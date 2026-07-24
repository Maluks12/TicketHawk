package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ReleaseFailedBooking cancels a booking whose payment could not be started and
// returns its tickets to the pool immediately.
//
// ── Why this exists ──────────────────────────────────────────────────────────
//
// The booking row commits BEFORE payment initialisation. When initialisation
// fails — a payment channel that is not live on the Paystack account, a gateway
// outage, a rejected subaccount — the booking is left pending and still holding
// its seats. The expiry worker does eventually clear it, but its TTL is 15
// minutes. During a busy on-sale that is fifteen minutes of inventory locked up
// by a sale that can never complete, and a buyer retrying burns more seats with
// every attempt.
//
// This performs the same release the expiry worker does, at once rather than on
// a timer: cancel the booking, decrement the event counter, and decrement the
// ticket tier counter when the booking targeted one.
//
// Deliberately conservative. It only touches bookings that are still pending
// AND have no payment row at all, so it can never cancel a sale that actually
// reached the gateway. Errors are returned for logging, but the caller should
// still fail the request either way — the buyer needs to know the payment did
// not start, whether or not the seats came back cleanly.
func ReleaseFailedBooking(pool *pgxpool.Pool, ctx context.Context, bookingID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin release tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var eventID string
	var ticketTypeID *string
	var quantity int

	// FOR UPDATE so a concurrent expiry-worker pass cannot release the same
	// seats twice and drive tickets_sold below the true figure.
	err = tx.QueryRow(ctx, `
		SELECT b.event_id, b.ticket_type_id, b.ticket_quantity
		FROM bookings b
		WHERE b.id = $1
		  AND b.booking_status = 'pending'
		  AND b.payment_status = 'pending'
		  AND NOT EXISTS (SELECT 1 FROM payments p WHERE p.booking_id = b.id)
		FOR UPDATE
	`, bookingID).Scan(&eventID, &ticketTypeID, &quantity)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Already released, already paid, or a payment row exists after all.
			// Nothing to do, and nothing wrong.
			return nil
		}
		return fmt.Errorf("locking booking for release: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE bookings
		SET booking_status = 'cancelled',
		    payment_status = 'failed',
		    updated_at     = NOW()
		WHERE id = $1
	`, bookingID); err != nil {
		return fmt.Errorf("cancelling failed booking: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE events
		SET tickets_sold = GREATEST(0, tickets_sold - $1), updated_at = NOW()
		WHERE id = $2
	`, quantity, eventID); err != nil {
		return fmt.Errorf("releasing event capacity: %w", err)
	}

	if ticketTypeID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE ticket_types
			SET tickets_sold = GREATEST(0, tickets_sold - $1), updated_at = NOW()
			WHERE id = $2
		`, quantity, *ticketTypeID); err != nil {
			return fmt.Errorf("releasing tier capacity: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit release: %w", err)
	}

	log.Info().
		Str("booking_id", bookingID).
		Int("tickets_released", quantity).
		Msg("booking: payment could not be started, seats returned to the pool")
	return nil
}