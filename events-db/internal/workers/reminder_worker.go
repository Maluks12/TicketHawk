package workers

import (
	"context"
	"fmt"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// StartReminderWorker polls the database every hour for event_follows rows
// where the event starts in ~7 days or ~1 day and the corresponding reminder
// notification has not yet been sent. For each candidate it:
//
//  1. Inserts a notification row for the follower.
//  2. Flips the reminded_7_days / reminded_1_day flag so the notification is
//     never sent again, even across server restarts.
//
// The ±6-hour query window in GetFollowsNeedingReminder means hourly polling
// has ample overlap; no reminder will be skipped due to cron drift.
func StartReminderWorker(pool *pgxpool.Pool, ctx context.Context) {
	go func() {
		log.Info().Msg("reminder worker: started")

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// Run once immediately on startup so any reminders missed during a
		// restart are dispatched without waiting a full hour.
		runReminderCycle(pool, ctx)

		for {
			select {
			case <-ticker.C:
				runReminderCycle(pool, ctx)
			case <-ctx.Done():
				log.Info().Msg("reminder worker: stopped")
				return
			}
		}
	}()
}

func runReminderCycle(pool *pgxpool.Pool, ctx context.Context) {
	candidates, err := repository.GetFollowsNeedingReminder(pool)
	if err != nil {
		log.Error().Err(err).Msg("reminder worker: failed to query candidates")
		return
	}

	if len(candidates) == 0 {
		return
	}

	log.Info().Int("count", len(candidates)).Msg("reminder worker: dispatching reminders")

	sent := 0
	for _, c := range candidates {
		if err := sendReminderNotification(pool, ctx, c); err != nil {
			log.Error().
				Err(err).
				Str("follow_id", c.FollowID).
				Str("event_id", c.EventID).
				Int("days_until", c.DaysUntil).
				Msg("reminder worker: failed to send reminder")
			continue
		}
		// Only mark the flag after the notification was persisted successfully.
		if err := repository.MarkReminderSent(pool, c.FollowID, c.ReminderField); err != nil {
			log.Error().
				Err(err).
				Str("follow_id", c.FollowID).
				Str("field", c.ReminderField).
				Msg("reminder worker: failed to mark reminder sent — may resend on next cycle")
		}
		sent++
	}

	log.Info().
		Int("sent", sent).
		Int("total", len(candidates)).
		Msg("reminder worker: cycle complete")
}

func sendReminderNotification(
	pool *pgxpool.Pool,
	ctx context.Context,
	c repository.ReminderCandidate,
) error {
	var title, body string

	switch c.DaysUntil {
	case 7:
		title = fmt.Sprintf("⏰ %s is in 1 week!", c.EventTitle)
		body = fmt.Sprintf(
			"%s is coming up in 7 days on %s. Make sure you're ready!",
			c.EventTitle,
			c.StartTime.Format("Monday, 2 January"),
		)
	case 1:
		title = fmt.Sprintf("🔔 %s is tomorrow!", c.EventTitle)
		body = fmt.Sprintf(
			"%s is happening tomorrow, %s. Get excited!",
			c.EventTitle,
			c.StartTime.Format("Monday, 2 January"),
		)
	default:
		return fmt.Errorf("unexpected DaysUntil value %d", c.DaysUntil)
	}

	return repository.CreateNotification(
		pool,
		ctx,
		c.UserID,
		models.NotifEventReminder,
		title,
		body,
		map[string]interface{}{
			"event_id":   c.EventID,
			"event_slug": c.EventSlug,
			"days_until": c.DaysUntil,
			"start_time": c.StartTime.Format(time.RFC3339),
		},
	)
}