package repository

import (
	"context"
	"math"
	"errors"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Payout profile ────────────────────────────────────────────────────────────

// GetPayoutProfileByUserID returns the payout setup for the organizer owned by
// userID (organizer routes authenticate the user, not the organizer row).
func GetPayoutProfileByUserID(pool *pgxpool.Pool, ctx context.Context, userID string) (*models.PayoutProfile, error) {
	return scanPayoutProfile(pool.QueryRow(ctx, payoutProfileSQL+` WHERE o.user_id = $1`, userID))
}

// GetPayoutProfileByOrganizerID is the admin-side lookup by organizers.id.
func GetPayoutProfileByOrganizerID(pool *pgxpool.Pool, ctx context.Context, organizerID string) (*models.PayoutProfile, error) {
	return scanPayoutProfile(pool.QueryRow(ctx, payoutProfileSQL+` WHERE o.id = $1`, organizerID))
}

const payoutProfileSQL = `
	SELECT o.id, o.business_name, o.payout_tier, o.payout_enabled,
	       o.paystack_subaccount_code, o.settlement_bank_name,
	       o.bank_account_name, o.bank_account_last_4, o.tier_upgraded_at
	FROM organizers o`

func scanPayoutProfile(row pgx.Row) (*models.PayoutProfile, error) {
	var p models.PayoutProfile
	var tier string
	var subCode *string
	err := row.Scan(&p.OrganizerID, &p.BusinessName, &tier, &p.PayoutEnabled,
		&subCode, &p.SettlementBankName, &p.BankAccountName, &p.BankAccountLast4, &p.TierUpgradedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("organizer not found")
		}
		return nil, fmt.Errorf("GetPayoutProfile: %w", err)
	}
	p.PayoutTier = models.PayoutTier(tier)
	p.HasSubaccount = subCode != nil && *subCode != ""
	p.SubaccountCode = subCode
	return &p, nil
}

// SavePayoutDetails persists the subaccount + transfer-recipient codes after
// they've been created at Paystack, flips payout_enabled, and stores only the
// last 4 digits of the account number.
func SavePayoutDetails(pool *pgxpool.Pool, ctx context.Context, organizerID, subaccountCode, recipientCode string, dto *models.SetPayoutDetailsDTO) error {
	last4 := dto.AccountNumber
	if len(last4) > 4 {
		last4 = last4[len(last4)-4:]
	}
	tag, err := pool.Exec(ctx, `
		UPDATE organizers
		SET paystack_subaccount_code = $1,
		    paystack_recipient_code  = $2,
		    settlement_bank_code     = $3,
		    settlement_bank_name     = $4,
		    bank_account_name        = $5,
		    bank_account_last_4      = $6,
		    payout_enabled           = TRUE,
		    subaccount_updated_at    = CURRENT_TIMESTAMP,
		    updated_at               = CURRENT_TIMESTAMP
		WHERE id = $7
	`, subaccountCode, recipientCode, dto.BankCode, dto.BankName, dto.AccountName, last4, organizerID)
	if err != nil {
		return fmt.Errorf("SavePayoutDetails: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("organizer %s not found", organizerID)
	}
	return nil
}

// ── Instant-tier eligibility ──────────────────────────────────────────────────

// CheckInstantEligibility evaluates the promotion policy for one organizer.
// "Completed paid event" = event reached 'completed', had at least one
// confirmed paid booking, and produced at least one scan (the scan_events
// ledger proves the event actually happened — our advantage over pure
// sales-based checks).
func CheckInstantEligibility(pool *pgxpool.Pool, ctx context.Context, organizerID string) (*models.InstantTierEligibility, error) {
	e := &models.InstantTierEligibility{
		RequiredEvents:  models.InstantTierMinCompletedEvents,
		RequiredGross:   models.InstantTierMinGrossZAR,
		RequiredAgeDays: models.InstantTierMinAccountAgeDays,
	}

	var createdAt time.Time
	var hasBank bool
	if err := pool.QueryRow(ctx, `
		SELECT created_at,
		       (paystack_subaccount_code IS NOT NULL AND payout_enabled)
		FROM organizers WHERE id = $1
	`, organizerID).Scan(&createdAt, &hasBank); err != nil {
		return nil, fmt.Errorf("CheckInstantEligibility organizer: %w", err)
	}
	e.HasVerifiedBank = hasBank
	e.AccountAgeDays = int(time.Since(createdAt).Hours() / 24)

	// Delivered paid events + lifetime gross, via the organizer's user id.
	// "Delivered paid event" = completed + ≥1 confirmed paid booking + ≥1 scan.
	if err := pool.QueryRow(ctx, `
		WITH org AS (SELECT user_id FROM organizers WHERE id = $1)
		SELECT
			COUNT(DISTINCT ev.id) FILTER (
				WHERE ev.status = 'completed'
				  AND EXISTS (SELECT 1 FROM scan_events se WHERE se.event_id = ev.id)
				  AND EXISTS (
				      SELECT 1 FROM bookings pb
				      WHERE pb.event_id = ev.id
				        AND pb.booking_status = 'confirmed'
				        AND pb.subtotal > 0
				  )
			),
			COALESCE(SUM(b.subtotal) FILTER (WHERE b.booking_status = 'confirmed'), 0)
		FROM events ev
		JOIN org ON ev.organizer_id = org.user_id
		LEFT JOIN bookings b ON b.event_id = ev.id
	`, organizerID).Scan(&e.CompletedPaidEvents, &e.GrossProcessed); err != nil {
		return nil, fmt.Errorf("CheckInstantEligibility events: %w", err)
	}

	// Open refund requests block promotion outright.
	if err := pool.QueryRow(ctx, `
		WITH org AS (SELECT user_id FROM organizers WHERE id = $1)
		SELECT COUNT(*)
		FROM bookings b
		JOIN events ev ON ev.id = b.event_id
		JOIN org ON ev.organizer_id = org.user_id
		WHERE b.refund_requested_at IS NOT NULL AND b.refunded_at IS NULL
	`, organizerID).Scan(&e.OpenRefundRequests); err != nil {
		return nil, fmt.Errorf("CheckInstantEligibility refunds: %w", err)
	}

	e.Eligible = e.CompletedPaidEvents >= e.RequiredEvents &&
		e.GrossProcessed >= e.RequiredGross &&
		e.AccountAgeDays >= e.RequiredAgeDays &&
		e.OpenRefundRequests == 0 &&
		e.HasVerifiedBank
	return e, nil
}

// SetOrganizerTier promotes or demotes an organizer. Promotion to 'instant'
// is admin-initiated only; the DB constraint additionally guarantees a
// subaccount exists. Demotion always succeeds (it's the safe direction).
func SetOrganizerTier(pool *pgxpool.Pool, ctx context.Context, organizerID string, tier models.PayoutTier, adminID string) error {
	// NOTE: the promotion condition is computed here and passed as $4 rather
	// than comparing $1 = 'instant' in SQL — reusing one parameter in both an
	// assignment (varchar) and a comparison (text) makes Postgres fail with
	// "inconsistent types deduced for parameter $1" (SQLSTATE 42P08).
	isInstant := tier == models.PayoutTierInstant
	tag, err := pool.Exec(ctx, `
		UPDATE organizers
		SET payout_tier      = $1,
		    tier_upgraded_at = CASE WHEN $4 THEN CURRENT_TIMESTAMP ELSE tier_upgraded_at END,
		    tier_upgraded_by = CASE WHEN $4 THEN $2::uuid ELSE tier_upgraded_by END,
		    updated_at       = CURRENT_TIMESTAMP
		WHERE id = $3
	`, string(tier), adminID, organizerID, isInstant)
	if err != nil {
		return fmt.Errorf("SetOrganizerTier: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("organizer %s not found", organizerID)
	}
	return nil
}

// ── Ledger creation (idempotent) ──────────────────────────────────────────────

// CreatePayoutsForCompletedEvents finds paid events that completed at least
// `delay` ago and inserts their payout ledger rows:
//
//	event_settlement — SUM(subtotal − organizer_fee) over confirmed bookings
//	                   the platform collected (no subaccount on the payment).
//	                   Cart bookings never have a split, so they're included
//	                   automatically. Bookings whose own payment row carried a
//	                   subaccount were already settled by Paystack — excluded.
//	reserve_release  — SUM(reserve_amount) over completed split payments.
//
// UNIQUE(event_id, kind) + ON CONFLICT DO NOTHING makes this safe to run
// every tick forever. Returns how many new rows were created.
func CreatePayoutsForCompletedEvents(pool *pgxpool.Pool, ctx context.Context, delay time.Duration) (int64, error) {
	cutoff := time.Now().Add(-delay)

	// Refunded bookings are now INCLUDED rather than filtered out, because a
	// partial refund still leaves the organiser owed the unrefunded remainder.
	// The organiser commission is charged on gross sold, so it is subtracted in
	// full regardless of how much came back.
	//
	// The CASE is a safety net for rows that predate migration 000058: a
	// 'refunded' booking with a NULL refund_amount is treated as fully refunded,
	// so a legacy row can never be settled as if it were still confirmed.
	// GREATEST(...,0) stops a fully-refunded booking contributing a negative
	// amount that would silently reduce what other bookings earned.
	const settlementAmount = `
		GREATEST(
		    b.subtotal
		    - CASE WHEN b.booking_status = 'refunded'
		           THEN COALESCE(b.refund_amount, b.subtotal)
		           ELSE COALESCE(b.refund_amount, 0) END
		    - b.organizer_fee,
		    0)`

	settled, err := pool.Exec(ctx, `
		INSERT INTO organizer_payouts (organizer_id, event_id, kind, amount, currency)
		SELECT o.id, ev.id, 'event_settlement',
		       SUM(`+settlementAmount+`), MAX(b.currency)
		FROM events ev
		JOIN organizers o ON o.user_id = ev.organizer_id
		JOIN bookings b   ON b.event_id = ev.id
		WHERE ev.status = 'completed'
		  AND ev.end_time <= $1
		  AND b.booking_status IN ('confirmed', 'refunded')
		  AND b.subtotal > 0
		  AND NOT EXISTS (
		      SELECT 1 FROM payments p
		      WHERE p.booking_id = b.id AND p.subaccount_code IS NOT NULL
		  )
		GROUP BY o.id, ev.id
		HAVING SUM(`+settlementAmount+`) > 0
		ON CONFLICT (event_id, kind) DO NOTHING
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("CreatePayoutsForCompletedEvents settlements: %w", err)
	}

	// The reserve is what FUNDS a refund on an instant-tier booking: Paystack
	// already settled the organiser's share at sale time, so the withheld
	// portion is the only money the platform still controls. Whatever the
	// refund did not consume belongs to the organiser and is released here.
	//
	// Because the reserve rate is sized to the refund rate applicable on the
	// day of sale (models.ReserveRateForSale), and the refund rate only falls
	// as the event nears, this subtraction can never go negative on a booking
	// priced under the current schedule — GREATEST is belt-and-braces for rows
	// created before migration 000058, where a flat 10% reserve may be smaller
	// than the refund that was issued against it.
	//
	// Refunded bookings must be INCLUDED (previously they were filtered out by
	// booking_status = 'confirmed'), otherwise a partial refund would strand
	// the unconsumed remainder of the reserve on the platform permanently.
	const releasableReserve = `
		GREATEST(
		    p.reserve_amount
		    - CASE WHEN b.booking_status = 'refunded'
		           THEN COALESCE(b.refund_amount, p.reserve_amount)
		           ELSE COALESCE(b.refund_amount, 0) END,
		    0)`

	reserves, err := pool.Exec(ctx, `
		INSERT INTO organizer_payouts (organizer_id, event_id, kind, amount, currency)
		SELECT o.id, ev.id, 'reserve_release',
		       SUM(`+releasableReserve+`), MAX(p.currency)
		FROM events ev
		JOIN organizers o ON o.user_id = ev.organizer_id
		JOIN bookings b   ON b.event_id = ev.id
		JOIN payments p   ON p.booking_id = b.id
		WHERE ev.status = 'completed'
		  AND ev.end_time <= $1
		  AND b.booking_status IN ('confirmed', 'refunded')
		  AND p.status IN ('completed', 'refunded')
		  AND p.subaccount_code IS NOT NULL
		  AND p.reserve_amount > 0
		GROUP BY o.id, ev.id
		HAVING SUM(`+releasableReserve+`) > 0
		ON CONFLICT (event_id, kind) DO NOTHING
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("CreatePayoutsForCompletedEvents reserves: %w", err)
	}

	return settled.RowsAffected() + reserves.RowsAffected(), nil
}

// ── Ledger reads / state transitions ──────────────────────────────────────────

// payoutStatusFilter guards against SQL injection on the admin list endpoint.
var validPayoutStatuses = map[string]bool{"pending": true, "processing": true, "paid": true, "failed": true}

// ListPayouts returns ledger rows, newest first, optionally filtered by status
// and/or organizer. Used by both the admin screen and the organizer history.
func ListPayouts(pool *pgxpool.Pool, ctx context.Context, status, organizerID string, limit int) ([]models.OrganizerPayout, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	q := `
		SELECT op.id, op.organizer_id, op.event_id, COALESCE(ev.title, ''),
		       op.kind, op.amount, op.currency, op.status,
		       op.transfer_reference, op.paystack_transfer_code, op.failure_reason,
		       op.created_at, op.updated_at, op.paid_at
		FROM organizer_payouts op
		LEFT JOIN events ev ON ev.id = op.event_id
		WHERE 1=1`
	args := []interface{}{}
	if status != "" && validPayoutStatuses[status] {
		args = append(args, status)
		q += fmt.Sprintf(" AND op.status = $%d", len(args))
	}
	if organizerID != "" {
		args = append(args, organizerID)
		q += fmt.Sprintf(" AND op.organizer_id = $%d", len(args))
	}
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY op.created_at DESC LIMIT $%d", len(args))

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListPayouts: %w", err)
	}
	defer rows.Close()

	var out []models.OrganizerPayout
	for rows.Next() {
		var p models.OrganizerPayout
		if err := rows.Scan(&p.ID, &p.OrganizerID, &p.EventID, &p.EventTitle,
			&p.Kind, &p.Amount, &p.Currency, &p.Status,
			&p.TransferReference, &p.PaystackTransferCode, &p.FailureReason,
			&p.CreatedAt, &p.UpdatedAt, &p.PaidAt); err != nil {
			return nil, fmt.Errorf("ListPayouts scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ClaimPayoutForProcessing atomically moves one pending (or retried-failed)
// payout to 'processing' and stamps its transfer_reference exactly once.
// The WHERE status guard means two worker ticks can never claim the same row,
// and COALESCE keeps the original reference on retries so Paystack-side
// idempotency holds across attempts.
func ClaimPayoutForProcessing(pool *pgxpool.Pool, ctx context.Context, payoutID, transferReference string) (claimed bool, reference string, err error) {
	err = pool.QueryRow(ctx, `
		UPDATE organizer_payouts
		SET status             = 'processing',
		    transfer_reference = COALESCE(transfer_reference, $1),
		    updated_at         = CURRENT_TIMESTAMP
		WHERE id = $2 AND status IN ('pending', 'failed')
		RETURNING transfer_reference
	`, transferReference, payoutID).Scan(&reference)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", nil // someone else claimed it, or it's already paid
		}
		return false, "", fmt.Errorf("ClaimPayoutForProcessing: %w", err)
	}
	return true, reference, nil
}

// ResetPayoutForManualRetry clears the backoff state on a failed row so the
// next ProcessPendingPayouts tick picks it up immediately.
//
// This exists because the automatic retry budget is deliberately finite: once
// MaxPayoutAttempts is spent, next_attempt_at goes NULL and the worker stops
// looking at the row. That is correct for unattended retries — a dead
// recipient code should not generate traffic forever — but it must not trap
// the money. An admin pressing "retry" has presumably fixed the underlying
// cause, so the budget is reset to zero rather than merely nudged: they get a
// full fresh schedule, not one last attempt.
//
// Scoped to 'failed' so it can never disturb a row that is pending, mid-flight
// or already paid.
func ResetPayoutForManualRetry(pool *pgxpool.Pool, ctx context.Context, payoutID string) error {
	_, err := pool.Exec(ctx, `
		UPDATE organizer_payouts
		SET attempts        = 0,
		    next_attempt_at = CURRENT_TIMESTAMP,
		    updated_at      = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = 'failed'
	`, payoutID)
	if err != nil {
		return fmt.Errorf("ResetPayoutForManualRetry: %w", err)
	}
	return nil
}

func MarkPayoutPaid(pool *pgxpool.Pool, ctx context.Context, payoutID, transferCode string) error {
	_, err := pool.Exec(ctx, `
		UPDATE organizer_payouts
		SET status = 'paid', paystack_transfer_code = $1,
		    failure_reason = NULL, next_attempt_at = NULL,
		    paid_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, transferCode, payoutID)
	return err
}

// MaxPayoutAttempts caps the retry budget for one payout row. Once a row has
// failed this many times it stays 'failed' with next_attempt_at NULL and the
// worker stops picking it up — the failure is no longer plausibly transient
// and needs a human.
//
// The schedule below spans roughly 3.5 days in total (15m, 1h, 4h, 12h, then
// daily). That is deliberately longer than any realistic balance top-up or
// Paystack incident, so a solvable problem resolves itself, while a dead
// recipient code stops generating traffic within the week.
const MaxPayoutAttempts = 8

// MarkPayoutFailed records a failed transfer attempt and schedules the next
// one with exponential backoff.
//
// Every failure path funnels through here — gateway errors, a transfer that
// vanished at Paystack, a row claimed but never sent — so backoff applies
// uniformly without each call site having to think about it.
func MarkPayoutFailed(pool *pgxpool.Pool, ctx context.Context, payoutID, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	// $3 is cast explicitly. An uncast parameter compared against `attempts + 1`
	// has bitten this codebase before: Postgres deduces inconsistent types for
	// a reused parameter and fails with SQLSTATE 42P08.
	_, err := pool.Exec(ctx, `
		UPDATE organizer_payouts
		SET status          = 'failed',
		    failure_reason  = $1,
		    attempts        = attempts + 1,
		    last_attempt_at = CURRENT_TIMESTAMP,
		    next_attempt_at = CASE
		        WHEN attempts + 1 >= $3::int THEN NULL
		        WHEN attempts + 1 = 1 THEN CURRENT_TIMESTAMP + INTERVAL '15 minutes'
		        WHEN attempts + 1 = 2 THEN CURRENT_TIMESTAMP + INTERVAL '1 hour'
		        WHEN attempts + 1 = 3 THEN CURRENT_TIMESTAMP + INTERVAL '4 hours'
		        WHEN attempts + 1 = 4 THEN CURRENT_TIMESTAMP + INTERVAL '12 hours'
		        ELSE CURRENT_TIMESTAMP + INTERVAL '24 hours'
		    END,
		    updated_at      = CURRENT_TIMESTAMP
		WHERE id = $2
	`, reason, payoutID, MaxPayoutAttempts)
	return err
}

// PendingPayoutRow is what the transfer worker needs to pay one ledger row.
type PendingPayoutRow struct {
	PayoutID      string
	OrganizerID   string
	Amount        float64
	Currency      string
	Status        string
	TransferRef   *string
	Attempts      int
	RecipientCode *string
	BusinessName  string
	BankCode      *string
	AccountLast4  *string
	AccountName   *string
}

// FetchPayableRows returns pending + stuck-processing payouts joined with the
// organizer's recipient details. Processing rows are included so the worker
// can reconcile transfers that were initiated but never confirmed (crash).
func FetchPayableRows(pool *pgxpool.Pool, ctx context.Context, limit int) ([]PendingPayoutRow, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := pool.Query(ctx, `
		SELECT op.id, op.organizer_id, op.amount, op.currency, op.status, op.transfer_reference,
		       op.attempts,
		       o.paystack_recipient_code, o.business_name,
		       o.settlement_bank_code, o.bank_account_last_4, o.bank_account_name
		FROM organizer_payouts op
		JOIN organizers o ON o.id = op.organizer_id
		WHERE (
		        op.status IN ('pending', 'processing')
		        -- Failed rows come back once their backoff has elapsed. A NULL
		        -- next_attempt_at on a failed row means the retry budget is
		        -- spent: leave it alone, it needs a human.
		        OR (op.status = 'failed'
		            AND op.next_attempt_at IS NOT NULL
		            AND op.next_attempt_at <= CURRENT_TIMESTAMP)
		      )
		  AND op.kind <> 'refund_clawback'
		  AND (
		        op.status = 'processing'
		        OR (
		            (o.payout_hold_until IS NULL OR o.payout_hold_until <= CURRENT_TIMESTAMP)
		            AND NOT EXISTS (
		                SELECT 1 FROM active_account_restrictions ar
		                WHERE ar.user_id = o.user_id
		                  AND ar.kind IN ('payouts', 'disable', 'ban')
		            )
		        )
		      )
		ORDER BY op.created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("FetchPayableRows: %w", err)
	}
	defer rows.Close()

	var out []PendingPayoutRow
	for rows.Next() {
		var r PendingPayoutRow
		if err := rows.Scan(&r.PayoutID, &r.OrganizerID, &r.Amount, &r.Currency, &r.Status,
			&r.TransferRef, &r.Attempts, &r.RecipientCode, &r.BusinessName,
			&r.BankCode, &r.AccountLast4, &r.AccountName); err != nil {
			return nil, fmt.Errorf("FetchPayableRows scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
// ApplyPayoutDebt deducts any outstanding organiser debt from a pending payout
// before it is transferred.
//
// Returns settled=true when the debt consumed the payout entirely — the row is
// closed out as paid with a zero transfer and the caller must not proceed to
// the gateway. Returns settled=false when there is either no debt or a positive
// remainder to send, with organizer_payouts.amount already reduced.
//
// The whole thing runs in one transaction with SELECT ... FOR UPDATE on the
// organiser, because two payout rows for the same organiser processing
// concurrently would otherwise both see the full debt and each deduct it.
// payable is the amount that must ACTUALLY be transferred after debt is
// deducted. Callers must use this return value rather than the amount they
// passed in: the in-memory PendingPayoutRow still holds the pre-deduction
// figure, and transferring that while the ledger records the reduced one sends
// the organiser too much money with every internal record looking correct.
func ApplyPayoutDebt(
	pool *pgxpool.Pool,
	ctx context.Context,
	organizerID string,
	payoutID string,
	amount float64,
) (payable float64, settled bool, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin debt tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var debt float64
	if err := tx.QueryRow(ctx, `
		SELECT payout_debt FROM organizers WHERE id = $1 FOR UPDATE
	`, organizerID).Scan(&debt); err != nil {
		return 0, false, fmt.Errorf("locking organizer for debt check: %w", err)
	}
	if debt <= 0 {
		return amount, false, tx.Commit(ctx)
	}

	applied := debt
	if applied > amount {
		applied = amount
	}
	remaining := roundZAR(amount - applied)

	if _, err := tx.Exec(ctx, `
		UPDATE organizers
		SET payout_debt = payout_debt - $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, applied, organizerID); err != nil {
		return 0, false, fmt.Errorf("reducing organizer debt: %w", err)
	}

	if remaining <= 0 {
		// Nothing left to send. Mark paid so the worker stops re-queuing it,
		// and record why the organiser saw no money arrive.
		if _, err := tx.Exec(ctx, `
			UPDATE organizer_payouts
			SET amount         = 0,
			    status         = 'paid',
			    paid_at        = CURRENT_TIMESTAMP,
			    failure_reason = $1,
			    updated_at     = CURRENT_TIMESTAMP
			WHERE id = $2
		`, fmt.Sprintf("Fully offset against outstanding balance of R%.2f", applied), payoutID); err != nil {
			return 0, false, fmt.Errorf("closing fully-offset payout: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, false, fmt.Errorf("commit full offset: %w", err)
		}
		return 0, true, nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE organizer_payouts
		SET amount = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, remaining, payoutID); err != nil {
		return 0, false, fmt.Errorf("reducing payout by debt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("commit partial offset: %w", err)
	}
	return remaining, false, nil
}

func roundZAR(v float64) float64 {
	return math.Round(v*100) / 100
}