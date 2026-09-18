package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidSettlementStatus = errors.New("unknown settlement status")
	ErrSettlementTransition    = errors.New("settlement is not in a state that allows this transition")
)

// ── Settlement queries ────────────────────────────────────────────────────────
//
// Every figure is DERIVED from cashless_transactions at settlement time. There
// is no running per-vendor total anywhere, deliberately: a total maintained by
// triggers drifts the first time an offline batch syncs late, and drift in a
// payout figure is money somebody is short.

// VendorPeriodTotals is one vendor's activity in a window.
type VendorPeriodTotals struct {
	VendorID       string
	VendorName     string
	VendorCode     string
	CommissionRate *float64

	GrossCents   int64 // purchases minus refunds
	RefundCents  int64
	TxnCount     int
	OfflineCount int

	// Transactions flagged at ingest — bad signature, cap violation, sequence
	// gap. Reported alongside the payout, NOT deducted from it: whether a
	// flagged sale is real is a judgement, and quietly withholding a vendor's
	// money on a heuristic is the wrong default.
	FlaggedCount int
	FlaggedCents int64
}

// VendorTotalsForPeriod computes every vendor's position over [start, end).
//
// Half-open on purpose. With offline batches syncing in bursts, a transaction
// landing exactly on a boundary is not rare, and an inclusive end would let two
// consecutive settlement runs both claim it.
func VendorTotalsForPeriod(
	ctx context.Context, pool *pgxpool.Pool,
	eventID string, start, end time.Time,
) ([]VendorPeriodTotals, error) {
	rows, err := pool.Query(ctx, `
		SELECT v.id, v.name, v.code, v.commission_rate,
		       -- amount is SIGNED: purchases negative, refunds positive. Gross
		       -- is therefore the negated sum, which nets refunds off sales in
		       -- one expression rather than two subqueries that could disagree.
		       COALESCE(-SUM(t.amount), 0)::bigint AS gross_rands,
		       COALESCE(SUM(t.amount) FILTER (WHERE t.kind = 'refund'), 0) AS refund_rands,
		       COUNT(t.id) AS txn_count,
		       COUNT(t.id) FILTER (WHERE t.mode = 'offline') AS offline_count,
		       COUNT(t.id) FILTER (WHERE t.detail ? 'recheck'
		                             AND t.detail->>'recheck' <> '') AS flagged_count,
		       COALESCE(-SUM(t.amount) FILTER (WHERE t.detail ? 'recheck'
		                             AND t.detail->>'recheck' <> ''), 0) AS flagged_rands
		FROM event_vendors v
		LEFT JOIN cashless_transactions t
		       ON t.vendor_id = v.id
		      AND t.kind IN ('purchase', 'refund')
		      AND t.created_at >= $2
		      AND t.created_at <  $3
		WHERE v.event_id = $1
		GROUP BY v.id, v.name, v.code, v.commission_rate
		ORDER BY v.code
	`, eventID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []VendorPeriodTotals{}
	for rows.Next() {
		var (
			v                            VendorPeriodTotals
			gross, refund, flaggedAmount float64
		)
		if err := rows.Scan(&v.VendorID, &v.VendorName, &v.VendorCode, &v.CommissionRate,
			&gross, &refund, &v.TxnCount, &v.OfflineCount,
			&v.FlaggedCount, &flaggedAmount); err != nil {
			return nil, err
		}
		// Converted to cents at the boundary, once. Everything downstream is
		// integer arithmetic — see the header of cashless_ledger.go.
		if c, err := ToCents(gross); err == nil {
			v.GrossCents = c
		}
		if c, err := ToCents(refund); err == nil {
			v.RefundCents = c
		}
		if c, err := ToCents(flaggedAmount); err == nil {
			v.FlaggedCents = c
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpsertSettlement stores a calculated settlement as a draft.
//
// Updates an existing draft for the same period rather than inserting a second
// one somebody might also pay. Refuses to touch a settlement that has been
// approved or paid — recalculating a payout after the money left would silently
// rewrite what both sides agreed.
func UpsertSettlement(
	ctx context.Context, pool *pgxpool.Pool,
	eventID, vendorID string, start, end time.Time,
	totals VendorPeriodTotals, calc SettlementResult, createdBy string,
) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO vendor_settlements
			(event_id, vendor_id, period_start, period_end,
			 gross_cents, refund_cents, commission_cents, net_cents, commission_rate,
			 txn_count, offline_count, flagged_count, flagged_cents, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (vendor_id, period_start, period_end) DO UPDATE SET
			gross_cents      = EXCLUDED.gross_cents,
			refund_cents     = EXCLUDED.refund_cents,
			commission_cents = EXCLUDED.commission_cents,
			net_cents        = EXCLUDED.net_cents,
			commission_rate  = EXCLUDED.commission_rate,
			txn_count        = EXCLUDED.txn_count,
			offline_count    = EXCLUDED.offline_count,
			flagged_count    = EXCLUDED.flagged_count,
			flagged_cents    = EXCLUDED.flagged_cents,
			calculated_at    = now()
		WHERE vendor_settlements.status = 'draft'
		RETURNING id
	`, eventID, vendorID, start, end,
		calc.GrossCents, totals.RefundCents, calc.CommissionCents, calc.NetCents,
		calc.AppliedRate, totals.TxnCount, totals.OfflineCount,
		totals.FlaggedCount, totals.FlaggedCents, createdBy).Scan(&id)
	return id, err
}

// SettlementRow is a stored settlement for display.
type SettlementRow struct {
	ID              string     `json:"id"`
	VendorID        string     `json:"vendor_id"`
	VendorName      string     `json:"vendor_name"`
	VendorCode      string     `json:"vendor_code"`
	PeriodStart     time.Time  `json:"period_start"`
	PeriodEnd       time.Time  `json:"period_end"`
	Gross           float64    `json:"gross"`
	Commission      float64    `json:"commission"`
	Net             float64    `json:"net"`
	CommissionRate  *float64   `json:"commission_rate,omitempty"`
	TxnCount        int        `json:"txn_count"`
	OfflineCount    int        `json:"offline_count"`
	FlaggedCount    int        `json:"flagged_count"`
	Flagged         float64    `json:"flagged_amount"`
	Status          string     `json:"status"`
	Reference       *string    `json:"reference,omitempty"`
	CalculatedAt    time.Time  `json:"calculated_at"`
	PaidAt          *time.Time `json:"paid_at,omitempty"`
}

func ListSettlements(ctx context.Context, pool *pgxpool.Pool, eventID string) ([]SettlementRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.id, s.vendor_id, v.name, v.code, s.period_start, s.period_end,
		       s.gross_cents, s.commission_cents, s.net_cents, s.commission_rate,
		       s.txn_count, s.offline_count, s.flagged_count, s.flagged_cents,
		       s.status::text, s.reference, s.calculated_at, s.paid_at
		FROM vendor_settlements s
		JOIN event_vendors v ON v.id = s.vendor_id
		WHERE s.event_id = $1
		ORDER BY s.period_end DESC, v.code
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SettlementRow{}
	for rows.Next() {
		var (
			r                                    SettlementRow
			gross, commission, net, flaggedCents int64
		)
		if err := rows.Scan(&r.ID, &r.VendorID, &r.VendorName, &r.VendorCode,
			&r.PeriodStart, &r.PeriodEnd, &gross, &commission, &net, &r.CommissionRate,
			&r.TxnCount, &r.OfflineCount, &r.FlaggedCount, &flaggedCents,
			&r.Status, &r.Reference, &r.CalculatedAt, &r.PaidAt); err != nil {
			return nil, err
		}
		r.Gross, r.Commission, r.Net = FromCents(gross), FromCents(commission), FromCents(net)
		r.Flagged = FromCents(flaggedCents)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetSettlementStatus moves a settlement through its lifecycle.
//
// The WHERE clause is the guard: a settlement can only be approved from draft
// and paid from approved. Marking something paid that was never approved would
// mean money leaving without anyone signing it off.
func SetSettlementStatus(
	ctx context.Context, pool *pgxpool.Pool,
	settlementID, newStatus, reference, note string,
) error {
	var from string
	switch newStatus {
	case "approved":
		from = "draft"
	case "paid":
		from = "approved"
	case "disputed", "cancelled":
		from = "" // reachable from anywhere except paid
	default:
		return ErrInvalidSettlementStatus
	}

	q := `
		UPDATE vendor_settlements
		SET status      = $2::settlement_status,
		    reference   = COALESCE(NULLIF($3,''), reference),
		    note        = COALESCE(NULLIF($4,''), note),
		    approved_at = CASE WHEN $2 = 'approved' THEN now() ELSE approved_at END,
		    paid_at     = CASE WHEN $2 = 'paid'     THEN now() ELSE paid_at END
		WHERE id = $1 AND status <> 'paid'`
	args := []any{settlementID, newStatus, reference, note}
	if from != "" {
		q += ` AND status = $5`
		args = append(args, from)
	}

	res, err := pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrSettlementTransition
	}
	return nil
}

// ── Float reconciliation ─────────────────────────────────────────────────────

// EventFloatTotals gathers everything CalculateFloat needs.
func EventFloatTotals(ctx context.Context, pool *pgxpool.Pool, eventID string) (FloatInput, error) {
	var in FloatInput
	var topups, purchases, refunds, payouts float64

	err := pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount) FILTER (WHERE kind = 'topup'), 0),
			COALESCE(-SUM(amount) FILTER (WHERE kind = 'purchase'), 0),
			COALESCE(SUM(amount) FILTER (WHERE kind = 'refund'), 0),
			COALESCE(-SUM(amount) FILTER (WHERE kind = 'payout'), 0)
		FROM cashless_transactions WHERE event_id = $1
	`, eventID).Scan(&topups, &purchases, &refunds, &payouts)
	if err != nil {
		return in, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(net_cents), 0) FROM vendor_settlements
		WHERE event_id = $1 AND status = 'paid'
	`, eventID).Scan(&in.SettledCents); err != nil {
		return in, err
	}

	for _, conv := range []struct {
		src float64
		dst *int64
	}{
		{topups, &in.TopupsCents},
		{purchases, &in.PurchasesCents},
		{refunds, &in.RefundsCents},
		{payouts, &in.PayoutsCents},
	} {
		c, err := ToCents(conv.src)
		if err != nil {
			return in, err
		}
		*conv.dst = c
	}
	return in, nil
}

// EventShortfallTotals counts wallets currently negative.
func EventShortfallTotals(ctx context.Context, pool *pgxpool.Pool, eventID string) (int, int64, error) {
	var count int
	var cents int64
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(amount_cents - recovered_cents), 0)
		FROM wallet_shortfalls
		WHERE event_id = $1 AND state NOT IN ('recovered', 'written_off')
	`, eventID).Scan(&count, &cents)
	return count, cents, err
}

// SaveFloatReconciliation records the figures an organiser acted on, so they
// remain recoverable as the ledger keeps moving.
func SaveFloatReconciliation(
	ctx context.Context, pool *pgxpool.Pool,
	eventID string, in FloatInput, res FloatResult,
	shortfallCount int, shortfallCents int64, unsettledCents int64, by string,
) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO event_float_reconciliations
			(event_id, topups_cents, purchases_cents, refunds_cents, payouts_cents,
			 outstanding_liability_cents, shortfall_cents, shortfall_count,
			 settled_cents, unsettled_cents, reconciled_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`, eventID, in.TopupsCents, in.PurchasesCents, in.RefundsCents, in.PayoutsCents,
		res.OutstandingLiabilityCents, shortfallCents, shortfallCount,
		in.SettledCents, unsettledCents, by).Scan(&id)
	return id, err
}