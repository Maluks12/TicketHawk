package workers

import (
	"context"
	"fmt"
	"time"

	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// StartAdWorker runs nightly ad maintenance at midnight:
//
//  1. Bill each organiser for yesterday's actual spend via their Stitch mandate.
//  2. Resume campaigns that hit their daily budget (exhausted -> active).
//  3. End campaigns whose end_date has passed.
//
// Billing runs BEFORE resume so a payment_failed organiser doesn't get campaigns
// re-activated at midnight before the billing attempt has been made.
func StartAdWorker(pool *pgxpool.Pool, paymentSvc *services.PaymentService, ctx context.Context) {
	go func() {
		log.Info().Msg("ad worker: started (post-pay billing mode)")

		now := time.Now()
		nextMidnight := time.Date(
			now.Year(), now.Month(), now.Day()+1,
			0, 1, 0, 0, // 1 minute past midnight — avoids clock edge cases
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

		runNightlyAdTasks(pool, paymentSvc)

		for {
			select {
			case <-ticker.C:
				runNightlyAdTasks(pool, paymentSvc)
			case <-ctx.Done():
				log.Info().Msg("ad worker: stopped")
				return
			}
		}
	}()
}

func runNightlyAdTasks(pool *pgxpool.Pool, paymentSvc *services.PaymentService) {
	log.Info().Msg("ad worker: running nightly maintenance")

	// 1. Bill yesterday's actual spend FIRST — before resuming any campaigns.
	yesterday := time.Now().AddDate(0, 0, -1)
	billed, failed := billDailySpend(pool, paymentSvc, yesterday)
	log.Info().
		Int("billed", billed).
		Int("failed", failed).
		Str("billing_date", yesterday.Format("2006-01-02")).
		Msg("ad worker: daily billing complete")

	// 2. Resume campaigns that hit their daily budget (exhausted -> active).
	//    payment_failed campaigns stay paused — organiser must fix their mandate.
	if err := repository.ResumeExhaustedCampaigns(pool); err != nil {
		log.Error().Err(err).Msg("ad worker: failed to resume exhausted campaigns")
	} else {
		log.Info().Msg("ad worker: exhausted campaigns resumed for new day")
	}

	// 3. End campaigns whose end_date has passed.
	if err := repository.EndExpiredCampaigns(pool); err != nil {
		log.Error().Err(err).Msg("ad worker: failed to end expired campaigns")
	} else {
		log.Info().Msg("ad worker: expired campaigns ended")
	}
}

// billDailySpend bills every organiser for yesterday's actual spend.
// Returns the number of successful and failed billing attempts.
func billDailySpend(pool *pgxpool.Pool, paymentSvc *services.PaymentService, forDate time.Time) (billed, failed int) {
	spends, err := repository.GetUnbilledDailySpend(pool, forDate)
	if err != nil {
		log.Error().Err(err).Msg("ad worker: failed to fetch unbilled spend")
		return
	}

	if len(spends) == 0 {
		log.Info().Str("date", forDate.Format("2006-01-02")).Msg("ad worker: no unbilled spend for date")
		return
	}

	for _, spend := range spends {
		if err := billOrganizer(pool, paymentSvc, spend, forDate); err != nil {
			log.Error().
				Err(err).
				Str("organizer_id", spend.OrganizerID).
				Float64("amount_zar", spend.TotalSpend).
				Msg("ad worker: billing failed for organizer")
			failed++
		} else {
			billed++
		}
	}
	return
}

func billOrganizer(pool *pgxpool.Pool, paymentSvc *services.PaymentService, spend repository.OrganizerDaySpend, forDate time.Time) error {
	if spend.MandateID == "" {
		return fmt.Errorf("organizer %s has no mandate ID", spend.OrganizerID)
	}

	ref := fmt.Sprintf("ADS-%s-%s", forDate.Format("20060102"), spend.OrganizerID[:8])

	requestID, err := paymentSvc.InitiateDebitMandate(
		spend.MandateID,
		spend.TotalSpend,
		ref,
		spend.PaymentEmail,
	)
	if err != nil {
		// Record the failed attempt so admin can see it in the dashboard.
		// Record a failed billing row so admin can see it in the dashboard.
		// We pass empty requestID since the Stitch call never succeeded.
		_, _ = repository.CreateBillingRecord(pool, spend.OrganizerID, forDate, spend.TotalSpend, "")
		// Pause campaigns immediately since we couldn't even initiate the debit.
		pauseCampaignsForFailedBilling(pool, spend.OrganizerID)
		return fmt.Errorf("initiate debit mandate: %w", err)
	}

	// Record the pending billing row — Stitch webhook will confirm or fail it.
	if _, err := repository.CreateBillingRecord(pool, spend.OrganizerID, forDate, spend.TotalSpend, requestID); err != nil {
		log.Error().Err(err).Str("stitch_request_id", requestID).
			Msg("ad worker: failed to record billing row (debit already initiated)")
	}

	log.Info().
		Str("organizer_id", spend.OrganizerID).
		Float64("amount_zar", spend.TotalSpend).
		Str("stitch_request_id", requestID).
		Msg("ad worker: debit mandate initiated successfully")

	return nil
}

// pauseCampaignsForFailedBilling sets campaigns to payment_failed when we
// couldn't even initiate the debit (e.g. Stitch API down, invalid mandate).
func pauseCampaignsForFailedBilling(pool *pgxpool.Pool, organizerID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ad_campaigns
		SET    status = 'payment_failed', updated_at = NOW()
		WHERE  organizer_id = $1
		  AND  status IN ('active', 'exhausted')
	`, organizerID)
	if err != nil {
		log.Error().Err(err).Str("organizer_id", organizerID).
			Msg("ad worker: failed to pause campaigns after billing error")
	}
}