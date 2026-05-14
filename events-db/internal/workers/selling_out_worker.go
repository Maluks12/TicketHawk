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

// StartSellOutWorker polls the database every 15 minutes for published events
// that have crossed a sell-out threshold without a notification having been
// dispatched yet.
//
// Two thresholds are tracked per event (via columns added in migration 034):
//
//   - 80 % sold  → "Selling fast" push to all followers
//   - 100 % sold → "Sold out" push to all followers
//
// For each qualifying event the worker:
//  1. Fetches all follower user IDs.
//  2. Bulk-inserts a notification row for every follower.
//  3. Marks the event-level flag so the notification fires exactly once.
//
// The event-level flag (not per-follow) means we do a single UPDATE per event
// regardless of how many followers it has, keeping the critical path short.
func StartSellOutWorker(pool *pgxpool.Pool, ctx context.Context) {
	go func() {
		log.Info().Msg("sell-out worker: started")

		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		// Run once on startup to catch any events that crossed a threshold
		// while the server was down.
		runSellOutCycle(pool, ctx)

		for {
			select {
			case <-ticker.C:
				runSellOutCycle(pool, ctx)
			case <-ctx.Done():
				log.Info().Msg("sell-out worker: stopped")
				return
			}
		}
	}()
}

func runSellOutCycle(pool *pgxpool.Pool, ctx context.Context) {
	candidates, err := repository.GetSellOutCandidates(pool)
	if err != nil {
		log.Error().Err(err).Msg("sell-out worker: failed to query candidates")
		return
	}

	if len(candidates) == 0 {
		return
	}

	log.Info().Int("events", len(candidates)).Msg("sell-out worker: processing events")

	for _, c := range candidates {
		// Sold-out takes priority: if an event is both at 80 % and 100 % in
		// the same cycle (e.g. the first time the worker sees it) we send the
		// sold-out alert only — "selling fast" after sold-out is confusing.
		if c.NotifySoldOut {
			if err := notifyFollowersSoldOut(pool, ctx, c); err != nil {
				log.Error().
					Err(err).
					Str("event_id", c.EventID).
					Msg("sell-out worker: sold-out notifications failed")
				continue
			}
			if err := repository.MarkEventSoldOutNotified(pool, c.EventID); err != nil {
				log.Error().
					Err(err).
					Str("event_id", c.EventID).
					Msg("sell-out worker: failed to mark sold-out notified — may resend")
			}
			// Also flip the 80-pct flag so we don't send a "selling fast"
			// alert later for an event that is already sold out.
			if c.Notify80Pct {
				if err := repository.MarkEvent80PctNotified(pool, c.EventID); err != nil {
					log.Error().
						Err(err).
						Str("event_id", c.EventID).
						Msg("sell-out worker: failed to mark 80pct notified alongside sold-out")
				}
			}
			continue
		}

		// 80 % threshold only.
		if c.Notify80Pct {
			if err := notifyFollowers80Pct(pool, ctx, c); err != nil {
				log.Error().
					Err(err).
					Str("event_id", c.EventID).
					Msg("sell-out worker: 80pct notifications failed")
				continue
			}
			if err := repository.MarkEvent80PctNotified(pool, c.EventID); err != nil {
				log.Error().
					Err(err).
					Str("event_id", c.EventID).
					Msg("sell-out worker: failed to mark 80pct notified — may resend")
			}
		}
	}
}

// notifyFollowers80Pct sends a "selling fast" notification to every follower
// of the event. It fetches the follower list and creates one notification per
// user using the shared CreateNotification helper.
func notifyFollowers80Pct(pool *pgxpool.Pool, ctx context.Context, c repository.SellOutCandidate) error {
	followerIDs, err := repository.GetEventFollowerIDs(pool, c.EventID)
	if err != nil {
		return fmt.Errorf("get followers: %w", err)
	}
	if len(followerIDs) == 0 {
		return nil
	}

	pctSold := int(float64(c.TicketsSold) / float64(c.MaxCapacity) * 100)
	remaining := c.MaxCapacity - c.TicketsSold

	title := fmt.Sprintf("🔥 %s is selling fast!", c.EventTitle)
	body := fmt.Sprintf(
		"%d%% of tickets for %s are gone — only %d left. Grab yours before they sell out.",
		pctSold, c.EventTitle, remaining,
	)
	data := map[string]interface{}{
		"event_id":    c.EventID,
		"event_slug":  c.EventSlug,
		"tickets_sold": c.TicketsSold,
		"max_capacity": c.MaxCapacity,
	}

	for _, userID := range followerIDs {
		if err := repository.CreateNotification(
			pool, ctx, userID,
			models.NotifTicketsSellingFast,
			title, body, data,
		); err != nil {
			// Log but continue — one follower failing shouldn't block others.
			log.Error().
				Err(err).
				Str("user_id", userID).
				Str("event_id", c.EventID).
				Msg("sell-out worker: failed to create selling-fast notification")
		}
	}

	log.Info().
		Str("event_id", c.EventID).
		Str("event_title", c.EventTitle).
		Int("followers_notified", len(followerIDs)).
		Msg("sell-out worker: selling-fast notifications sent")

	return nil
}

// notifyFollowersSoldOut sends a "sold out" notification to every follower
// of the event.
func notifyFollowersSoldOut(pool *pgxpool.Pool, ctx context.Context, c repository.SellOutCandidate) error {
	followerIDs, err := repository.GetEventFollowerIDs(pool, c.EventID)
	if err != nil {
		return fmt.Errorf("get followers: %w", err)
	}
	if len(followerIDs) == 0 {
		return nil
	}

	title := fmt.Sprintf("😔 %s is sold out", c.EventTitle)
	body := fmt.Sprintf(
		"All tickets for %s have been sold. Keep an eye out — tickets sometimes become available again.",
		c.EventTitle,
	)
	data := map[string]interface{}{
		"event_id":   c.EventID,
		"event_slug": c.EventSlug,
	}

	for _, userID := range followerIDs {
		if err := repository.CreateNotification(
			pool, ctx, userID,
			models.NotifEventSoldOut,
			title, body, data,
		); err != nil {
			log.Error().
				Err(err).
				Str("user_id", userID).
				Str("event_id", c.EventID).
				Msg("sell-out worker: failed to create sold-out notification")
		}
	}

	log.Info().
		Str("event_id", c.EventID).
		Str("event_title", c.EventTitle).
		Int("followers_notified", len(followerIDs)).
		Msg("sell-out worker: sold-out notifications sent")

	return nil
}