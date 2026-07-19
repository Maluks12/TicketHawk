package repository

import (
	"context"
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
	tag, err := pool.Exec(ctx, `
		UPDATE organizers
		SET payout_tier      = $1,
		    tier_upgraded_at = CASE WHEN $1 = 'instant' THEN CURRENT_TIMESTAMP ELSE tier_upgraded_at END,
		    tier_upgraded_by = CASE WHEN $1 = 'instant' THEN $2::uuid ELSE tier_upgraded_by END,
		    updated_at       = CURRENT_TIMESTAMP
		WHERE id = $3
	`, string(tier), adminID, organizerID)
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

	settled, err := pool.Exec(ctx, `
		INSERT INTO organizer_payouts (organizer_id, event_id, kind, amount, currency)
		SELECT o.id, ev.id, 'event_settlement',
		       SUM(b.subtotal - b.organizer_fee), MAX(b.currency)
		FROM events ev
		JOIN organizers o ON o.user_id = ev.organizer_id
		JOIN bookings b   ON b.event_id = ev.id
		WHERE ev.status = 'completed'
		  AND ev.end_time <= $1
		  AND b.booking_status = 'confirmed'
		  AND b.subtotal > 0
		  AND NOT EXISTS (
		      SELECT 1 FROM payments p
		      WHERE p.booking_id = b.id AND p.subaccount_code IS NOT NULL
		  )
		GROUP BY o.id, ev.id
		HAVING SUM(b.subtotal - b.organizer_fee) > 0
		ON CONFLICT (event_id, kind) DO NOTHING
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("CreatePayoutsForCompletedEvents settlements: %w", err)
	}

	reserves, err := pool.Exec(ctx, `
		INSERT INTO organizer_payouts (organizer_id, event_id, kind, amount, currency)
		SELECT o.id, ev.id, 'reserve_release',
		       SUM(p.reserve_amount), MAX(p.currency)
		FROM events ev
		JOIN organizers o ON o.user_id = ev.organizer_id
		JOIN bookings b   ON b.event_id = ev.id
		JOIN payments p   ON p.booking_id = b.id
		WHERE ev.status = 'completed'
		  AND ev.end_time <= $1
		  AND b.booking_status = 'confirmed'
		  AND p.status = 'completed'
		  AND p.subaccount_code IS NOT NULL
		  AND p.reserve_amount > 0
		GROUP BY o.id, ev.id
		HAVING SUM(p.reserve_amount) > 0
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

func MarkPayoutPaid(pool *pgxpool.Pool, ctx context.Context, payoutID, transferCode string) error {
	_, err := pool.Exec(ctx, `
		UPDATE organizer_payouts
		SET status = 'paid', paystack_transfer_code = $1,
		    failure_reason = NULL, paid_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, transferCode, payoutID)
	return err
}

func MarkPayoutFailed(pool *pgxpool.Pool, ctx context.Context, payoutID, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	_, err := pool.Exec(ctx, `
		UPDATE organizer_payouts
		SET status = 'failed', failure_reason = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, reason, payoutID)
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
		       o.paystack_recipient_code, o.business_name,
		       o.settlement_bank_code, o.bank_account_last_4, o.bank_account_name
		FROM organizer_payouts op
		JOIN organizers o ON o.id = op.organizer_id
		WHERE op.status IN ('pending', 'processing')
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
			&r.TransferRef, &r.RecipientCode, &r.BusinessName,
			&r.BankCode, &r.AccountLast4, &r.AccountName); err != nil {
			return nil, fmt.Errorf("FetchPayableRows scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}