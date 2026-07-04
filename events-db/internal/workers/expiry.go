package workers

import (
	"context"
	"events-db/internal/cache"
	"events-db/internal/models"
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
// It also:
//   - Flushes Redis view counts to Postgres every 60 s.
//   - Sends event-reminder notifications to followers every hour.
func StartExpiryWorker(pool *pgxpool.Pool, ec *cache.EventCache, ctx context.Context, ttl time.Duration, interval time.Duration) {
	viewTicker := time.NewTicker(60 * time.Second)

	// Reminder notifications run once per hour — fine-grained enough to catch
	// the ±6-hour window used in GetFollowsNeedingReminder without hammering
	// the DB more than necessary.
	reminderTicker := time.NewTicker(1 * time.Hour)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer viewTicker.Stop()
		defer reminderTicker.Stop()

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

			case <-reminderTicker.C:
				sendFollowReminders(pool, ctx)

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

// sendFollowReminders queries all event follows where the event starts in ~7
// days or ~1 day and the relevant reminder has not been sent yet, then creates
// a notification for each user and marks the flag as sent.
func sendFollowReminders(pool *pgxpool.Pool, ctx context.Context) {
	candidates, err := repository.GetFollowsNeedingReminder(pool)
	if err != nil {
		log.Error().Err(err).Msg("reminder worker: query failed")
		return
	}
	if len(candidates) == 0 {
		return
	}

	log.Info().Int("count", len(candidates)).Msg("reminder worker: sending follow reminders")

	for _, c := range candidates {
		var title, body string
		if c.DaysUntil == 7 {
			title = "Event in 7 days 🗓"
			body = fmt.Sprintf(
				"%s is starting in 7 days. Make sure you're ready!",
				c.EventTitle,
			)
		} else {
			title = "Event tomorrow ⏰"
			body = fmt.Sprintf(
				"%s is starting tomorrow. Don't forget!",
				c.EventTitle,
			)
		}

		notifErr := repository.CreateNotification(
			pool, ctx,
			c.UserID,
			models.NotifEventReminder,
			title,
			body,
			map[string]interface{}{
				"event_id":   c.EventID,
				"event_slug": c.EventSlug,
				"days_until": c.DaysUntil,
			},
		)
		if notifErr != nil {
			log.Error().
				Err(notifErr).
				Str("follow_id", c.FollowID).
				Str("user_id", c.UserID).
				Msg("reminder worker: failed to create notification")
			// Don't mark the reminder as sent — it will retry next hour.
			continue
		}

		// Only flip the flag after the notification is confirmed written.
		if markErr := repository.MarkReminderSent(pool, c.FollowID, c.ReminderField); markErr != nil {
			log.Error().
				Err(markErr).
				Str("follow_id", c.FollowID).
				Str("field", c.ReminderField).
				Msg("reminder worker: failed to mark reminder sent")
			// Non-fatal — the notification was already written, so a duplicate
			// on the next run is preferable to a crash.
		}
	}
}

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