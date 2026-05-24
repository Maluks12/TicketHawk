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
//  1. Bill each ad account for yesterday's actual spend via its Stitch mandate.
//  2. Resume campaigns that hit their daily budget (exhausted -> active).
//  3. End campaigns whose end_date has passed.
//
// Billing is now per ad account (each has its own mandate) rather than per
// organizer. An organizer with multiple accounts gets one billing debit per
// account, each charged against that account's own Stitch mandate.
func StartAdWorker(pool *pgxpool.Pool, paymentSvc *services.PaymentService, ctx context.Context) {
	go func() {
		log.Info().Msg("ad worker: started (post-pay billing, per-account mode)")

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

	yesterday := time.Now().AddDate(0, 0, -1)
	billed, failed := billDailySpend(pool, paymentSvc, yesterday)
	log.Info().
		Int("billed", billed).
		Int("failed", failed).
		Str("billing_date", yesterday.Format("2006-01-02")).
		Msg("ad worker: daily billing complete")

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

// billDailySpend bills every ad account for yesterday's actual spend.
// Each account is billed independently against its own Stitch mandate.
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
		if err := billAdAccount(pool, paymentSvc, spend, forDate); err != nil {
			log.Error().
				Err(err).
				Str("ad_account_id", spend.AdAccountID).
				Str("organizer_id", spend.OrganizerID).
				Float64("amount_zar", spend.TotalSpend).
				Msg("ad worker: billing failed for ad account")
			failed++
		} else {
			billed++
		}
	}
	return
}

// billAdAccount initiates a Stitch debit for one ad account's daily spend.
func billAdAccount(pool *pgxpool.Pool, paymentSvc *services.PaymentService, spend repository.AdAccountDaySpend, forDate time.Time) error {
	if spend.MandateID == "" {
		return fmt.Errorf("ad account %s has no mandate ID", spend.AdAccountID)
	}

	// Reference format: ADS-YYYYMMDD-<first 8 chars of account ID>
	ref := fmt.Sprintf("ADS-%s-%s", forDate.Format("20060102"), spend.AdAccountID[:8])

	requestID, err := paymentSvc.InitiateDebitMandate(
		spend.MandateID,
		spend.TotalSpend,
		ref,
		spend.PaymentEmail,
	)
	if err != nil {
		// Record the failed attempt so admin can see it. Pass empty requestID
		// since the Stitch call never succeeded.
		_, _ = repository.CreateBillingRecord(pool, spend.OrganizerID, forDate, spend.TotalSpend, "", spend.AdAccountID)
		// Pause campaigns under this account immediately.
		repository.PauseCampaignsForAdAccount(pool, spend.AdAccountID)
		return fmt.Errorf("initiate debit mandate: %w", err)
	}

	if _, err := repository.CreateBillingRecord(pool, spend.OrganizerID, forDate, spend.TotalSpend, requestID, spend.AdAccountID); err != nil {
		log.Error().Err(err).Str("stitch_request_id", requestID).
			Msg("ad worker: failed to record billing row (debit already initiated)")
	}

	log.Info().
		Str("ad_account_id", spend.AdAccountID).
		Str("organizer_id", spend.OrganizerID).
		Float64("amount_zar", spend.TotalSpend).
		Str("stitch_request_id", requestID).
		Msg("ad worker: debit mandate initiated successfully")

	return nil
}