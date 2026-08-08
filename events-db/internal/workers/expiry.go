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

func expireBookings(pool *pgxpool.Pool, ctx context.Context, ttl time.Duration) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, event_id, ticket_type_id, ticket_quantity
		FROM bookings
		WHERE booking_status = 'pending'
		AND payment_status = 'pending'
		AND created_at < NOW() - $1::interval
		FOR UPDATE SKIP LOCKED
	`, ttl.String())
	if err != nil {
		return err
	}
	defer rows.Close()

	type expiredBooking struct {
		id           string
		eventID      string
		ticketTypeID *string
		quantity     int
	}

	var expired []expiredBooking
	for rows.Next() {
		var b expiredBooking
		if err := rows.Scan(&b.id, &b.eventID, &b.ticketTypeID, &b.quantity); err != nil {
			return err
		}
		expired = append(expired, b)
	}
	rows.Close()

	if len(expired) == 0 {
		return nil
	}

	for _, b := range expired {
		if _, err = tx.Exec(ctx, `
			UPDATE bookings
			SET booking_status = 'cancelled', updated_at = NOW()
			WHERE id = $1
		`, b.id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `
			UPDATE events
			SET tickets_sold = GREATEST(0, tickets_sold - $1), updated_at = NOW()
			WHERE id = $2
		`, b.quantity, b.eventID); err != nil {
			return err
		}
		// Release the tier's capacity too — createBookingTx incremented
		// ticket_types.tickets_sold when the booking targeted a tier.
		if b.ticketTypeID != nil {
			if _, err = tx.Exec(ctx, `
				UPDATE ticket_types
				SET tickets_sold = GREATEST(0, tickets_sold - $1), updated_at = NOW()
				WHERE id = $2
			`, b.quantity, *b.ticketTypeID); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	log.Info().Int("count", len(expired)).Msg("expiry worker: cancelled expired bookings")
	return nil
}

func expireReservations(pool *pgxpool.Pool, ctx context.Context) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, event_id, quantity FROM ticket_reservations
		WHERE expires_at < NOW()
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type expiredRes struct {
		id, eventID string
		qty         int
	}
	var expired []expiredRes
	for rows.Next() {
		var r expiredRes
		if err := rows.Scan(&r.id, &r.eventID, &r.qty); err != nil {
			return err
		}
		expired = append(expired, r)
	}
	rows.Close()
	if len(expired) == 0 {
		return nil
	}

	for _, r := range expired {
		if _, err := tx.Exec(ctx,
			`UPDATE events SET reserved_count = GREATEST(0, reserved_count - $1), updated_at = NOW()
			 WHERE id = $2`, r.qty, r.eventID); err != nil {
			return fmt.Errorf("expireReservations: decrement reserved_count for event %s: %w", r.eventID, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM ticket_reservations WHERE id = $1`, r.id); err != nil {
			return fmt.Errorf("expireReservations: delete reservation %s: %w", r.id, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Info().Int("count", len(expired)).Msg("expiry worker: released expired reservations")
	return nil
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