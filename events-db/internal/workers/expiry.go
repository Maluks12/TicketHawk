package workers

import (
	"context"
	"events-db/internal/cache"
	"events-db/internal/repository"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// StartExpiryWorker runs periodic maintenance tasks:
//
//  1. Expire pending bookings older than ttl.
//  2. Release expired ticket reservations.
//  3. Clean up stale carts (inactive > 24 h).
//  4. Expire pending ticket assignments.
//  5. Deactivate merch listings whose sale_end has passed.
//
// It also flushes Redis view counts to Postgres every 60 s.
//
// It no longer sends event-reminder notifications — that duplicated
// workers/reminder_worker.go, which main.go also starts. See the note further
// down where sendFollowReminders used to be.
func StartExpiryWorker(pool *pgxpool.Pool, ec *cache.EventCache, ctx context.Context, ttl time.Duration, interval time.Duration) {
	viewTicker := time.NewTicker(60 * time.Second)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer viewTicker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := expireBookings(pool, ctx, ttl); err != nil {
					log.Error().Err(err).Msg("expiry worker: bookings")
				}
				if err := expireReservations(pool, ctx); err != nil {
					log.Error().Err(err).Msg("expiry worker: reservations")
				}
				if err := cleanupStaleCarts(pool, ctx); err != nil {
					log.Error().Err(err).Msg("expiry worker: carts")
				}
				if err := expirePendingAssignments(pool, ctx); err != nil {
					log.Error().Err(err).Msg("expiry worker: assignments")
				}
				// Deactivate merch items whose sale window has closed, then
				// bust the cache for every affected event so fresh reads kick in.
				expireMerchListings(pool, ec, ctx)

				// Mark events whose end_time has passed as completed so they
				// drop out of every public listing.
				completeEndedEvents(pool, ec, ctx)

			case <-viewTicker.C:
				flushViewCounts(pool, ec, ctx)
				flushMetricBuckets(pool, ec, ctx)

				case <-ctx.Done():
				return
			}
		}
	}()
}

// completeEndedEvents transitions published events whose end_time has passed to
// the 'completed' status, removing them from every public listing, then busts
// the feed caches so the change is reflected on the next read.
func completeEndedEvents(pool *pgxpool.Pool, ec *cache.EventCache, ctx context.Context) {
	ids, err := repository.CompleteEndedEvents(pool, ctx)
	if err != nil {
		log.Error().Err(err).Msg("expiry worker: complete ended events")
		return
	}
	if len(ids) == 0 {
		return
	}

	log.Info().
		Int("count", len(ids)).
		Msg("expiry worker: marked ended events as completed")

	if ec != nil {
		ec.BustOnEventUpdate(ctx)
	}
}

// expireMerchListings deactivates any merchandise items whose sale_end has
// passed, then busts the event-merch cache for all affected events so the
// next read reflects the removal immediately.
func expireMerchListings(pool *pgxpool.Pool, ec *cache.EventCache, ctx context.Context) {
	eventIDs, err := repository.ExpireMerchListings(pool, ctx)
	if err != nil {
		log.Error().Err(err).Msg("expiry worker: merch listings")
		return
	}
	if len(eventIDs) == 0 {
		return
	}

	log.Info().
		Int("count", len(eventIDs)).
		Msg("expiry worker: deactivated expired merch listings")

	if ec != nil {
		for _, eid := range eventIDs {
			ec.BustMerch(ctx, eid)
		}
	}
}

// NOTE: follow-reminder sending used to live here as sendFollowReminders(),
// running on its own hourly ticker inside this worker. It has been REMOVED —
// workers/reminder_worker.go does exactly the same job (same
// GetFollowsNeedingReminder query, same MarkReminderSent flag flip) and main.go
// starts BOTH.
//
// Two hourly tickers racing over the same candidate set meant a follower could
// be notified twice: both workers read the rows before either flipped the
// reminded_7_days / reminded_1_day flag. That was survivable while the only
// symptom was a duplicate row in the in-app feed. With push notifications live
// it is two buzzes on the phone for the same reminder, which is exactly the
// kind of thing that gets an app's notifications switched off for good.
//
// reminder_worker.go was kept over this copy because it runs a cycle
// immediately on startup (so reminders missed during a restart go out without
// waiting an hour) and because its message copy names the event and its date
// rather than the generic "Event in 7 days".

func flushViewCounts(pool *pgxpool.Pool, ec *cache.EventCache, ctx context.Context) {
	if ec == nil {
		return
	}
	counts := ec.FlushViews(ctx)
	if len(counts) == 0 {
		return
	}

	for eventID, count := range counts {
		_, err := pool.Exec(ctx,
			`UPDATE events SET views = COALESCE(views, 0) + $1, updated_at = NOW()
			 WHERE id = $2`, count, eventID)
		if err != nil {
			log.Error().Err(err).Str("event_id", eventID).Msg("view flush: failed to update event")
		}
	}
	log.Info().Int("events_updated", len(counts)).Msg("view flush: complete")
}

// expiryBatchSize caps how many rows one expiry statement touches.
//
// expireBookings used to select EVERY expired pending booking in one
// transaction under FOR UPDATE SKIP LOCKED and then issue two or three
// sequential tx.Exec round-trips per row inside it. After any period of
// downtime that became a very long transaction holding locks across the whole
// backlog, on the one path that frees seats after an abandoned checkout.
// reconcileStuckPayments already caps itself at 50; this one did not.
const expiryBatchSize = 200

// expiryMaxBatches bounds a single pass so a pathological backlog cannot spin
// the worker forever. 200 x 50 = 10,000 rows per tick, and whatever is left is
// picked up on the next one.
const expiryMaxBatches = 50

// expireBookings cancels abandoned pending bookings and returns their seats to
// inventory, in bounded batches.
func expireBookings(pool *pgxpool.Pool, ctx context.Context, ttl time.Duration) error {
	total := 0
	for batch := 0; batch < expiryMaxBatches; batch++ {
		n, err := expireBookingsBatch(pool, ctx, ttl, expiryBatchSize)
		if err != nil {
			if total > 0 {
				log.Info().Int("count", total).Msg("expiry worker: cancelled expired bookings before error")
			}
			return err
		}
		total += n
		if n < expiryBatchSize {
			break
		}
	}
	if total > 0 {
		log.Info().Int("count", total).Msg("expiry worker: cancelled expired bookings")
	}
	return nil
}

// expireBookingsBatch handles one bounded batch.
//
// It is a SINGLE statement rather than a read loop plus N updates inside an
// explicit transaction. Three things follow from that:
//
//   - The implicit transaction is exactly one statement long, so the locks
//     taken by FOR UPDATE SKIP LOCKED are held for as short a time as possible.
//   - The counter decrements are aggregated per event and per tier by SQL, so
//     two expired bookings on the same event are one UPDATE, not two. The old
//     per-row loop could not do this: a single UPDATE cannot touch the same row
//     twice, which is exactly why the aggregation has to happen in the CTE.
//   - The row loop that dropped errors on the floor is gone, and with it the
//     missing rows.Err() check that let a mid-stream failure look like an
//     empty backlog.
//
// Postgres runs data-modifying CTEs exactly once and always to completion,
// whether or not the primary query reads their output, so ev and tt both apply
// even though only cancelled is selected from.
func expireBookingsBatch(pool *pgxpool.Pool, ctx context.Context, ttl time.Duration, limit int) (int, error) {
	// payment_status IN ('pending', 'failed') — not 'pending' alone.
	//
	// syncBookingStatus no longer cancels a booking when an attempt is declined,
	// because Paystack keeps the transaction open and the buyer often pays by
	// another method on the same reference. That leaves declined bookings
	// sitting pending with payment_status 'failed', and with the old filter this
	// worker never collected them — their seats would have been held forever.
	// This is now the ONLY thing that frees a seat after an abandoned checkout.
	//
	// seats_released_at IS NULL keeps this consistent with releaseSeatsTx: the
	// stamp is the single source of truth for whether a booking currently holds
	// inventory, and ReacquireSeats reads it to decide whether a late
	// confirmation needs its seat back. It is set in the same statement here, so
	// a later refund cannot release the same seats a second time.
	var cancelled int
	err := pool.QueryRow(ctx, `
		WITH victims AS (
			SELECT id, event_id, ticket_type_id, ticket_quantity
			FROM bookings
			WHERE booking_status = 'pending'
			  AND payment_status IN ('pending', 'failed')
			  AND seats_released_at IS NULL
			  AND created_at < NOW() - $1::interval
			ORDER BY created_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		),
		ev AS (
			UPDATE events e
			SET tickets_sold = GREATEST(0, e.tickets_sold - agg.qty),
			    updated_at   = NOW()
			FROM (
				SELECT event_id, SUM(ticket_quantity)::int AS qty
				FROM victims GROUP BY event_id
			) agg
			WHERE e.id = agg.event_id
		),
		tt AS (
			UPDATE ticket_types t
			SET tickets_sold = GREATEST(0, t.tickets_sold - agg.qty),
			    updated_at   = NOW()
			FROM (
				SELECT ticket_type_id, SUM(ticket_quantity)::int AS qty
				FROM victims
				WHERE ticket_type_id IS NOT NULL
				GROUP BY ticket_type_id
			) agg
			WHERE t.id = agg.ticket_type_id
		),
		cancelled AS (
			UPDATE bookings b
			SET booking_status    = 'cancelled',
			    seats_released_at = NOW(),
			    updated_at        = NOW()
			FROM victims v
			WHERE b.id = v.id
			RETURNING b.id
		)
		SELECT count(*)::int FROM cancelled
	`, ttl.String(), limit).Scan(&cancelled)
	if err != nil {
		return 0, fmt.Errorf("expireBookingsBatch: %w", err)
	}
	return cancelled, nil
}

// expireReservations releases expired ticket reservations in bounded batches,
// for the same reasons expireBookings does — see the note above it.
func expireReservations(pool *pgxpool.Pool, ctx context.Context) error {
	total := 0
	for batch := 0; batch < expiryMaxBatches; batch++ {
		n, err := expireReservationsBatch(pool, ctx, expiryBatchSize)
		if err != nil {
			if total > 0 {
				log.Info().Int("count", total).Msg("expiry worker: released expired reservations before error")
			}
			return err
		}
		total += n
		if n < expiryBatchSize {
			break
		}
	}
	if total > 0 {
		log.Info().Int("count", total).Msg("expiry worker: released expired reservations")
	}
	return nil
}

// expireReservationsBatch handles one bounded batch. Single statement, so the
// reserved_count decrement and the DELETE are atomic per batch without an
// explicit transaction.
func expireReservationsBatch(pool *pgxpool.Pool, ctx context.Context, limit int) (int, error) {
	var released int
	err := pool.QueryRow(ctx, `
		WITH victims AS (
			SELECT id, event_id, quantity
			FROM ticket_reservations
			WHERE expires_at < NOW()
			ORDER BY expires_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		),
		ev AS (
			UPDATE events e
			SET reserved_count = GREATEST(0, e.reserved_count - agg.qty),
			    updated_at     = NOW()
			FROM (
				SELECT event_id, SUM(quantity)::int AS qty
				FROM victims GROUP BY event_id
			) agg
			WHERE e.id = agg.event_id
		),
		del AS (
			DELETE FROM ticket_reservations r
			USING victims v
			WHERE r.id = v.id
			RETURNING r.id
		)
		SELECT count(*)::int FROM del
	`, limit).Scan(&released)
	if err != nil {
		return 0, fmt.Errorf("expireReservationsBatch: %w", err)
	}
	return released, nil
}

func cleanupStaleCarts(pool *pgxpool.Pool, ctx context.Context) error {
	cmd, err := pool.Exec(ctx,
		`DELETE FROM carts WHERE last_activity_at < NOW() - INTERVAL '24 hours'`)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() > 0 {
		log.Info().Int64("count", cmd.RowsAffected()).Msg("expiry worker: cleaned up stale carts")
	}
	return nil
}

func expirePendingAssignments(pool *pgxpool.Pool, ctx context.Context) error {
	return repository.ExpirePendingAssignments(pool, ctx)
}