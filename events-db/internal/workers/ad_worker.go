package workers

import (
	"context"
	"time"

	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// StartAdWorker runs two nightly maintenance tasks:
//
//  1. Resume exhausted campaigns at the start of each new calendar day.
//     Campaigns that hit their daily_budget are automatically paused ('exhausted');
//     this worker reactivates them at midnight so they serve again.
//
//  2. End campaigns whose end_date has passed.
//
// The worker sleeps until the next midnight from startup, then fires every
// 24 hours. This is simpler than a cron and works cleanly with the graceful
// shutdown context already used by the booking and expiry workers.
func StartAdWorker(pool *pgxpool.Pool, ctx context.Context) {
	go func() {
		log.Info().Msg("ad worker: started")

		now := time.Now()
		nextMidnight := time.Date(
			now.Year(), now.Month(), now.Day()+1,
			0, 0, 30, 0, // 30 seconds past midnight avoids clock edge cases
			now.Location(),
		)

		log.Info().
			Str("first_reset_at", nextMidnight.Format(time.RFC3339)).
			Msg("ad worker: sleeping until midnight")

		select {
		case <-time.After(time.Until(nextMidnight)):
		case <-ctx.Done():
			log.Info().Msg("ad worker: shutdown before first reset")
			return
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		runNightlyAdTasks(pool)

		for {
			select {
			case <-ticker.C:
				runNightlyAdTasks(pool)
			case <-ctx.Done():
				log.Info().Msg("ad worker: stopped")
				return
			}
		}
	}()
}

func runNightlyAdTasks(pool *pgxpool.Pool) {
	log.Info().Msg("ad worker: running nightly maintenance")

	if err := repository.ResumeExhaustedCampaigns(pool); err != nil {
		log.Error().Err(err).Msg("ad worker: failed to resume exhausted campaigns")
	} else {
		log.Info().Msg("ad worker: exhausted campaigns resumed for new day")
	}

	if err := repository.EndExpiredCampaigns(pool); err != nil {
		log.Error().Err(err).Msg("ad worker: failed to end expired campaigns")
	} else {
		log.Info().Msg("ad worker: expired campaigns ended")
	}
}