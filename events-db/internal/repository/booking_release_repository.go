package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// Seat release.
//
// events.tickets_sold and ticket_types.tickets_sold are incremented when a
// booking is created. Returning those seats is this file's job, and it happens
// on two occasions:
//
//   - payment could not be started at all (ReleaseFailedBooking)
//   - the booking was refunded or its event cancelled (ReleaseRefundedSeats)
//
// Both go through releaseSeatsTx, which is gated on bookings.seats_released_at
// being NULL and stamps it in the same transaction. That gate matters: refunds
// are retryable, and a second decrement would push tickets_sold below the true
// figure and oversell the event — a worse failure than the one being fixed.

// releaseSeatsTx returns a booking's seats to the event and tier counters
// inside an existing transaction. Reports whether it released anything: false
// means the booking had already been released, which is not an error.
//
// The row is locked FOR UPDATE so a concurrent expiry-worker pass or a
// duplicate refund cannot both observe seats_released_at as NULL.
func releaseSeatsTx(tx pgx.Tx, ctx context.Context, bookingID string) (bool, error) {
	var eventID string
	var ticketTypeID *string
	var quantity int

	err := tx.QueryRow(ctx, `
		SELECT event_id, ticket_type_id, ticket_quantity
		FROM bookings
		WHERE id = $1 AND seats_released_at IS NULL
		FOR UPDATE
	`, bookingID).Scan(&eventID, &ticketTypeID, &quantity)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // already released, or no such booking
		}
		return false, fmt.Errorf("locking booking for seat release: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE bookings SET seats_released_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, bookingID); err != nil {
		return false, fmt.Errorf("stamping seat release: %w", err)
	}

	// GREATEST guards against a counter that has already drifted low for some
	// other reason — better to floor at zero than to go negative and make
	// availability arithmetic nonsense.
	if _, err := tx.Exec(ctx, `
		UPDATE events
		SET tickets_sold = GREATEST(0, tickets_sold - $1), updated_at = NOW()
		WHERE id = $2
	`, quantity, eventID); err != nil {
		return false, fmt.Errorf("releasing event capacity: %w", err)
	}

	if ticketTypeID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE ticket_types
			SET tickets_sold = GREATEST(0, tickets_sold - $1), updated_at = NOW()
			WHERE id = $2
		`, quantity, *ticketTypeID); err != nil {
			return false, fmt.Errorf("releasing tier capacity: %w", err)
		}
	}

	return true, nil
}

// ReleaseRefundedSeats puts a refunded booking's tickets back on sale.
//
// Called after a buyer refund, a guest refund, and each booking refunded by an
// event cancellation. Before this existed, nothing decremented tickets_sold on
// any of those paths — repository.DecrementTicketsSold was written but never
// wired up — so every refund permanently consumed a seat and events drifted
// towards a false sold-out.
//
// Safe to call more than once for the same booking; subsequent calls are no-ops.
// Safe to call for a cancelled event too, where resale is moot but the counter
// should still reflect reality for reporting.
func ReleaseRefundedSeats(pool *pgxpool.Pool, ctx context.Context, bookingID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin seat release tx: %w", err)
	}
	defer tx.Rollback(ctx)

	released, err := releaseSeatsTx(tx, ctx, bookingID)
	if err != nil {
		return err
	}
	if !released {
		return tx.Commit(ctx)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit seat release: %w", err)
	}

	log.Info().Str("booking_id", bookingID).
		Msg("booking: refunded seats returned to the pool")
	return nil
}

// ReleaseFailedBooking cancels a booking whose payment could not be started and
// returns its tickets immediately.
//
// The booking row commits BEFORE payment initialisation, so when initialisation
// fails — a payment channel that is not live on the Paystack account, a gateway
// outage, a rejected subaccount — the booking is left pending and still holding
// its seats. The expiry worker does eventually clear it, but its TTL is 15
// minutes. During a busy on-sale that is fifteen minutes of inventory locked up
// by a sale that can never complete, and a buyer retrying burns more seats with
// every attempt.
//
// Deliberately conservative: it only touches bookings that are still pending AND
// have no payment row at all, so it can never cancel a sale that actually
// reached the gateway. Errors are returned for logging, but the caller should
// still fail the request either way — the buyer needs to know the payment did
// not start, whether or not the seats came back cleanly.
func ReleaseFailedBooking(pool *pgxpool.Pool, ctx context.Context, bookingID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin release tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT TRUE FROM bookings b
		WHERE b.id = $1
		  AND b.booking_status = 'pending'
		  AND b.payment_status = 'pending'
		  AND NOT EXISTS (SELECT 1 FROM payments p WHERE p.booking_id = b.id)
	`, bookingID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Already released, already paid, or a payment row exists after all.
			return nil
		}
		return fmt.Errorf("checking booking for release: %w", err)
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

	released, err := releaseSeatsTx(tx, ctx, bookingID)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit release: %w", err)
	}

	if released {
		log.Info().Str("booking_id", bookingID).
			Msg("booking: payment could not be started, seats returned to the pool")
	}
	return nil
}