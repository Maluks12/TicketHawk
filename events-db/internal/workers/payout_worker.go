package workers

import (
	"context"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// StartPayoutWorker runs the two-tier payout pipeline:
//
//  1. Ledger creation — for every paid event that completed at least
//     PayoutDelayAfterEventHours ago, insert its 'event_settlement' (held-tier
//     money the platform collected) and 'reserve_release' (instant-tier 10%
//     reserve) rows. Idempotent via UNIQUE(event_id, kind).
//  2. Transfer processing — pay pending rows from the platform Paystack
//     balance and reconcile any stuck 'processing' rows.
//
// Both steps are safe to run on every tick and across restarts; idempotency
// lives in the DB (unique constraints + claimed transfer references), not in
// worker state.
func StartPayoutWorker(pool *pgxpool.Pool, payoutService *services.PayoutService, ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Info().Dur("interval", interval).Msg("payout worker: started")

		for {
			select {
			case <-ticker.C:
				runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)

				delay := time.Duration(models.PayoutDelayAfterEventHours) * time.Hour
				created, err := repository.CreatePayoutsForCompletedEvents(pool, runCtx, delay)
				if err != nil {
					log.Error().Err(err).Msg("payout worker: ledger creation")
				} else if created > 0 {
					log.Info().Int64("rows", created).Msg("payout worker: new payout rows created")
				}

				payoutService.ProcessPendingPayouts(runCtx)
				cancel()

			case <-ctx.Done():
				log.Info().Msg("payout worker: stopped")
				return
			}
		}
	}()
}