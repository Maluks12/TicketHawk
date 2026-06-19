package workers

import (
	"context"
	"time"

	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// StartAdWorker runs nightly ad maintenance just after midnight:
//
//  1. Resume campaigns that hit their daily budget (exhausted -> active).
//  2. End campaigns whose end_date has passed.
//
// There is no billing step under the pre-paid wallet model — money is collected
// up front when organisers top up, and impressions draw the wallet down in real
// time (see repository.RecordImpressions). A drained wallet simply stops serving
// via the balance gate in GetSponsoredForFeed.
func StartAdWorker(pool *pgxpool.Pool, ctx context.Context) {
	go func() {
		log.Info().Msg("ad worker: started (pre-paid wallet mode)")

		now := time.Now()
		nextMidnight := time.Date(
			now.Year(), now.Month(), now.Day()+1,
			0, 1, 0, 0,
			now.Location(),
		)

		log.Info().
			Str("first_run_at", nextMidnight.Format(time.RFC3339)).
			Msg("ad worker: sleeping until midnight")

		select {
		case <-time.After(time.Until(nextMidnight)):
		case <-ctx.Done():
			log.Info().Msg("ad worker: shutdown before first run")
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