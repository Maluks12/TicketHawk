package services

import (
	"context"
	"fmt"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// RefundService handles refunds that span more than one booking and touch the
// payout ledger — principally event cancellation.
//
// Buyer-initiated single-booking refunds do NOT go through here; they run in
// the booking handler against models.QuoteBuyerRefund, because they are simple
// and need no clawback (they are always priced so the reserve covers them).
type RefundService struct {
	pool     *pgxpool.Pool
	payments *PaymentService
}

func NewRefundService(pool *pgxpool.Pool, payments *PaymentService) *RefundService {
	return &RefundService{pool: pool, payments: payments}
}

// CancellationOutcome summarises what a cancellation actually moved, so the
// handler can report it and admin can reconcile it.
type CancellationOutcome struct {
	BookingsRefunded int     `json:"bookings_refunded"`
	BookingsFailed   int     `json:"bookings_failed"`
	TotalRefunded    float64 `json:"total_refunded"`
	ClawbackAccrued  float64 `json:"clawback_accrued"`
	OrganizerDemoted bool    `json:"organizer_demoted"`
}

// cancellationTarget is one booking that needs money returned.
type cancellationTarget struct {
	BookingID     string
	BookingNumber string
	Subtotal      float64
	PlatformFee   float64
	TotalAmount   float64
	// PaidToOrganizer is what Paystack settled directly to the organiser's bank
	// at sale time — zero unless the payment carried a subaccount split. This is
	// precisely the amount the platform cannot recover from its own balance, and
	// therefore precisely the clawback.
	PaidToOrganizer float64
}

// RefundEventCancellation returns every confirmed buyer on a cancelled event to
// whole — subtotal AND service fee, regardless of how close the event is. The
// buyer did nothing wrong, so they do not eat a fee for someone else's
// decision.
//
// This must be called AFTER repository.CancelEvent has flipped the event and
// its bookings, and it is deliberately idempotent at the booking level: a
// booking whose payment is already refunded is skipped, so a retry after a
// partial failure finishes the job rather than double-refunding.
//
// Money that was already settled to an instant-tier organiser cannot be pulled
// back out of their bank account, so it is recorded as debt on
// organizers.payout_debt and netted off their next payout. The organiser is
// also dropped to held tier — an organiser who has just cancelled an event is
// exactly who should not be receiving instant settlement.
func (s *RefundService) RefundEventCancellation(
	ctx context.Context,
	eventID string,
	reason string,
) (CancellationOutcome, error) {
	var out CancellationOutcome

	targets, organizerID, err := s.loadCancellationTargets(ctx, eventID)
	if err != nil {
		return out, err
	}
	if len(targets) == 0 {
		log.Info().Str("event_id", eventID).Msg("cancellation: no paid bookings to refund")
		return out, nil
	}

	for _, t := range targets {
		quote := models.QuoteCancellationRefund(t.Subtotal, t.PlatformFee)

		// ProcessRefund is the gateway call; it also stamps payments.status and
		// refunded_amount. A failure here is logged and counted rather than
		// aborting the loop — one dead gateway reference must not strand every
		// other buyer on the event.
		if err := s.payments.ProcessRefund(t.BookingID, quote.RefundAmount); err != nil {
			out.BookingsFailed++
			log.Error().Err(err).
				Str("booking_number", t.BookingNumber).
				Str("event_id", eventID).
				Float64("amount", quote.RefundAmount).
				Msg("cancellation: refund failed — buyer NOT repaid, needs manual action")
			continue
		}

		if err := s.markBookingRefunded(ctx, t.BookingID, quote.RefundAmount, reason); err != nil {
			// The money is already back with the buyer at this point, so this is
			// a bookkeeping problem, not a financial one. Log loudly and carry on.
			log.Error().Err(err).Str("booking_number", t.BookingNumber).
				Msg("cancellation: refunded at gateway but booking row not updated")
		}

		// Return the seats. Resale is moot on a cancelled event, but the counter
		// should still reflect reality for reporting and for the organiser's
		// dashboard. Idempotent, so a re-run cancellation sweep is harmless.
		if relErr := repository.ReleaseRefundedSeats(s.pool, ctx, t.BookingID); relErr != nil {
			log.Error().Err(relErr).Str("booking_number", t.BookingNumber).
				Msg("cancellation: seats not returned to the pool")
		}

		out.BookingsRefunded++
		out.TotalRefunded += quote.RefundAmount
		out.ClawbackAccrued += t.PaidToOrganizer
	}

	out.TotalRefunded = roundMoney(out.TotalRefunded)
	out.ClawbackAccrued = roundMoney(out.ClawbackAccrued)

	if out.ClawbackAccrued > 0 && organizerID != "" {
		if err := s.accrueClawback(ctx, organizerID, eventID, out.ClawbackAccrued); err != nil {
			log.Error().Err(err).Str("organizer_id", organizerID).
				Float64("amount", out.ClawbackAccrued).
				Msg("cancellation: refunds issued but clawback NOT recorded — platform is short")
		} else {
			out.OrganizerDemoted = true
		}
	}

	log.Info().
		Str("event_id", eventID).
		Int("refunded", out.BookingsRefunded).
		Int("failed", out.BookingsFailed).
		Float64("total_refunded", out.TotalRefunded).
		Float64("clawback", out.ClawbackAccrued).
		Msg("cancellation: refunds processed")

	return out, nil
}

// loadCancellationTargets finds every booking on the event that has money to
// return, and works out how much of each already left for the organiser.
//
// The status filter accepts 'refunded' as well as 'confirmed' because
// repository.CancelEvent flips booking_status BEFORE this runs. What actually
// gates a refund is the PAYMENT still being 'completed' — that is the
// idempotency anchor, and it is why a retry is safe.
func (s *RefundService) loadCancellationTargets(
	ctx context.Context,
	eventID string,
) ([]cancellationTarget, string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.booking_number, b.subtotal, b.platform_fee, b.total_amount,
		       CASE WHEN p.subaccount_code IS NOT NULL
		            THEN GREATEST(p.amount - COALESCE(p.transaction_charge, 0), 0)
		            ELSE 0 END AS paid_to_organizer,
		       COALESCE(o.id::text, '')
		FROM bookings b
		JOIN payments p   ON p.booking_id = b.id
		JOIN events   ev  ON ev.id = b.event_id
		LEFT JOIN organizers o ON o.user_id = ev.organizer_id
		WHERE b.event_id = $1
		  AND b.booking_status IN ('confirmed', 'refunded')
		  AND p.status = 'completed'
		  AND b.total_amount > 0
	`, eventID)
	if err != nil {
		return nil, "", fmt.Errorf("loading cancellation targets: %w", err)
	}
	defer rows.Close()

	var targets []cancellationTarget
	var organizerID string
	for rows.Next() {
		var t cancellationTarget
		var orgID string
		if err := rows.Scan(&t.BookingID, &t.BookingNumber, &t.Subtotal,
			&t.PlatformFee, &t.TotalAmount, &t.PaidToOrganizer, &orgID); err != nil {
			return nil, "", fmt.Errorf("scanning cancellation target: %w", err)
		}
		if orgID != "" {
			organizerID = orgID
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterating cancellation targets: %w", err)
	}
	return targets, organizerID, nil
}

func (s *RefundService) markBookingRefunded(
	ctx context.Context,
	bookingID string,
	amount float64,
	reason string,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE bookings
		SET booking_status      = 'refunded',
		    refund_amount       = $1,
		    refund_rate         = 1.0000,
		    refunded_at         = CURRENT_TIMESTAMP,
		    refund_reason       = COALESCE(refund_reason, $2),
		    refund_requested_at = COALESCE(refund_requested_at, CURRENT_TIMESTAMP),
		    updated_at          = CURRENT_TIMESTAMP
		WHERE id = $3
	`, amount, reason, bookingID)
	if err != nil {
		return err
	}

	// Notify the buyer that money is on its way back.
	//
	// models.NotifRefundProcessed was declared when the notification system was
	// built and then never emitted by anything — refunds sent email and wrote
	// no feed row, so buyers had no in-app record and (once push shipped) no
	// notification either. A refund is one of the things people most reliably
	// open an app to check on.
	//
	// This sits here rather than in RefundEventCancellation because
	// markBookingRefunded is the single choke point every refund passes
	// through, so a future refund path gets the notification for free.
	//
	// Best-effort by design: a notification failure must never surface as a
	// failed refund. The money has already moved; the worst case is a missing
	// feed row, and the buyer still has the email.
	if userID, evID, ok := s.bookingNotifyTarget(ctx, bookingID); ok {
		data := map[string]any{"booking_id": bookingID}
		if evID != "" {
			data["event_id"] = evID
		}
		if amount > 0 {
			data["refund_amount"] = amount
		}
		if nErr := repository.CreateNotification(
			s.pool, ctx, userID,
			models.NotifRefundProcessed,
			"Refund Processed",
			fmt.Sprintf("Your refund of R%.2f is on its way back to you. Allow a few working days for it to reflect.", amount),
			data,
		); nErr != nil {
			log.Error().Err(nErr).Str("booking_id", bookingID).
				Msg("refund service: refund notification failed")
		}
	}

	return nil
}

// bookingNotifyTarget resolves the account to notify for a booking.
//
// Returns ok=false for GUEST bookings (user_id IS NULL). Guests have no
// account, so there is no feed to write to and no device to push — they are
// served by the refund email instead. That is a deliberate skip, not an
// oversight, and it is why this returns a bool rather than an error.
func (s *RefundService) bookingNotifyTarget(ctx context.Context, bookingID string) (userID, eventID string, ok bool) {
	var uid, evID *string
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, event_id FROM bookings WHERE id = $1`, bookingID,
	).Scan(&uid, &evID)
	if err != nil || uid == nil || *uid == "" {
		return "", "", false
	}
	if evID != nil {
		eventID = *evID
	}
	return *uid, eventID, true
}

// accrueClawback records the shortfall as organiser debt, writes the audit row,
// and drops the organiser to held tier so no further sales settle instantly
// while they owe money.
//
// All three happen in one transaction: a debt without the demotion would let
// the organiser keep accruing instant settlements against a negative balance.
func (s *RefundService) accrueClawback(
	ctx context.Context,
	organizerID, eventID string,
	amount float64,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin clawback tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// ADDITIVE on conflict, not DO NOTHING.
	//
	// The previous comment claimed "a retry re-derives the same total". It does
	// not. loadCancellationTargets filters on p.status = 'completed', so every
	// retry returns only the bookings NOT yet refunded — the target set shrinks.
	// With DO NOTHING, a cancellation where some refunds failed accrued clawback
	// for the successful ones only, and the retry that refunded the rest hit the
	// conflict and skipped the debt entirely. The platform silently absorbed the
	// organiser's share of every booking refunded on a later attempt.
	//
	// Adding is safe precisely BECAUSE the target set shrinks: each run's
	// ClawbackAccrued is a non-overlapping delta, and the caller only reaches
	// here when it is > 0, so a clean re-run with nothing left to refund adds
	// nothing.
	if _, err = tx.Exec(ctx, `
		INSERT INTO organizer_payouts (organizer_id, event_id, kind, amount, currency, status)
		VALUES ($1, $2, 'refund_clawback', $3, 'ZAR', 'processing')
		ON CONFLICT (event_id, kind) DO UPDATE
		SET amount     = organizer_payouts.amount + EXCLUDED.amount,
		    updated_at = CURRENT_TIMESTAMP
	`, organizerID, eventID, amount); err != nil {
		return fmt.Errorf("accruing clawback row: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE organizers
		SET payout_debt      = payout_debt + $1,
		    payout_tier      = 'held',
		    tier_upgraded_at = NULL,
		    tier_upgraded_by = NULL,
		    updated_at       = CURRENT_TIMESTAMP
		WHERE id = $2
	`, amount, organizerID); err != nil {
		return fmt.Errorf("accruing organizer debt: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit clawback: %w", err)
	}

	log.Warn().
		Str("organizer_id", organizerID).
		Str("event_id", eventID).
		Float64("amount", amount).
		Msg("cancellation: organiser demoted to held tier and debited for settled refunds")
	return nil
}

// ── Dispute handling ──────────────────────────────────────────────────────────

// RecordDispute persists a chargeback notification. Disputes previously never
// reached the database at all, which meant the payout ledger would settle an
// organiser in full on money the bank had already reversed.
//
// Recording is deliberately separate from acting: a dispute being OPENED is not
// yet a loss, and auto-clawing back on every notification would punish
// organisers for disputes they go on to win. The row makes it visible to admin,
// and the reconciliation query below shows what is at risk.
func (s *RefundService) RecordDispute(
	ctx context.Context,
	gatewayReference, disputeReference, status, reason string,
	amount float64,
	rawPayload []byte,
) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO payment_disputes (
		    payment_id, booking_id, gateway, gateway_reference,
		    dispute_reference, amount, currency, status, reason, raw_payload
		)
		SELECT p.id, p.booking_id, 'paystack', $1, $2, $3, COALESCE(p.currency,'ZAR'), $4, $5, $6
		FROM payments p
		WHERE p.gateway_reference = $1
		LIMIT 1
		ON CONFLICT (gateway, gateway_reference)
		DO UPDATE SET status     = EXCLUDED.status,
		              reason     = EXCLUDED.reason,
		              amount     = EXCLUDED.amount,
		              updated_at = CURRENT_TIMESTAMP
	`, gatewayReference, disputeReference, amount, status, reason, rawPayload)
	if err != nil {
		return fmt.Errorf("recording dispute %s: %w", gatewayReference, err)
	}
	return nil
}

// SyncGatewayRefund reconciles a refund that was issued OUTSIDE the app — most
// commonly an admin clicking refund in the Paystack dashboard. Without this the
// database and Paystack diverge silently and the organiser gets settled on
// money the buyer already has back.
func (s *RefundService) SyncGatewayRefund(
	ctx context.Context,
	gatewayReference string,
	amount float64,
) error {
	tag := fmt.Sprintf("Refunded at gateway on %s", time.Now().UTC().Format("2006-01-02"))

	res, err := s.pool.Exec(ctx, `
		UPDATE bookings b
		SET booking_status = 'refunded',
		    refund_amount  = COALESCE(b.refund_amount, $2),
		    refund_rate    = CASE WHEN b.subtotal > 0
		                          THEN LEAST($2 / b.subtotal, 1.0)
		                          ELSE 0 END,
		    refunded_at    = COALESCE(b.refunded_at, CURRENT_TIMESTAMP),
		    refund_reason  = COALESCE(b.refund_reason, $3),
		    updated_at     = CURRENT_TIMESTAMP
		FROM payments p
		WHERE p.booking_id = b.id
		  AND p.gateway_reference = $1
		  AND b.booking_status <> 'refunded'
	`, gatewayReference, amount, tag)
	if err != nil {
		return fmt.Errorf("syncing gateway refund %s: %w", gatewayReference, err)
	}

	if res.RowsAffected() > 0 {
		log.Warn().Str("reference", gatewayReference).Float64("amount", amount).
			Msg("refund reconciled from gateway — issued outside the app")
	}
	return nil
}