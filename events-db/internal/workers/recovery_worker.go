package workers

import (
	"context"
	"time"

	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Shortfall recovery worker ─────────────────────────────────────────────────
//
// Drives repository.ClassifyRecovery over open cases.
//
// THE ONE THING THIS WORKER MUST NOT DO is charge a card without a recorded
// mandate. That property lives in ClassifyRecovery, which never returns
// RecoveryCharge unless a valid mandate covers the amount — so this loop simply
// does what it is told and has no branch of its own that reaches a card.
//
// Runs every fifteen minutes rather than continuously. Shortfalls arrive when
// tills sync after an outage, in bursts, and nothing here is urgent: the
// attendee has gone home, and the difference between recovering at 20:00 and
// 20:15 is nothing against the risk of hammering a payment gateway in a loop.
//
// PaymentCharger is the seam to whatever actually charges the card. Left as an
// interface because it is the one part that must not be wired in casually —
// see the note on Start below.
type PaymentCharger interface {
	ChargeAuthorization(ctx context.Context, authCode string, amountCents int64, reference string) error
}

// StartRecoveryWorker begins the loop.
//
// charger MAY BE NIL, and nil is a safe and reasonable configuration: with no
// charger, cases that classify as RecoveryCharge are logged and left alone
// rather than acted on. That is the correct default until the Paystack
// charge-authorization path has been built and tested, and it means enabling
// offline spend does not implicitly enable charging anybody's card.
func StartRecoveryWorker(pool *pgxpool.Pool, ctx context.Context, charger PaymentCharger) {
	go func() {
		if charger == nil {
			log.Info().Msg("recovery worker: started in REPORT-ONLY mode (no charger configured)")
		} else {
			log.Info().Msg("recovery worker: started")
		}

		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				runRecoveryPass(ctx, pool, charger)
			case <-ctx.Done():
				log.Info().Msg("recovery worker: stopped")
				return
			}
		}
	}()
}

func runRecoveryPass(ctx context.Context, pool *pgxpool.Pool, charger PaymentCharger) {
	cases, err := repository.ListActionableShortfalls(ctx, pool, 100)
	if err != nil {
		log.Error().Err(err).Msg("recovery worker: could not list shortfalls")
		return
	}
	if len(cases) == 0 {
		return
	}

	now := time.Now()
	var charged, asked, manual, wroteOff, skipped int

	for _, s := range cases {
		mandate, err := repository.GetActiveMandate(ctx, pool, s.WalletID)
		if err != nil {
			log.Error().Err(err).Str("wallet_id", s.WalletID).
				Msg("recovery worker: mandate lookup failed")
			continue
		}

		action, reason := repository.ClassifyRecovery(repository.RecoveryInput{
			BalanceCents:     s.BalanceCents,
			CardOnFile:       s.CardOnFile,
			Mandate:          mandate,
			SettleLinkSentAt: s.SettleSentAt,
			GraceDays:        recoveryGraceDays,
			DetectedAt:       s.DetectedAt,
			Attempts:         s.Attempts,
			MaxAttempts:      maxRecoveryAttempts,
		}, now)

		switch action {
		case repository.RecoveryNone:
			skipped++

		case repository.RecoveryInvestigate:
			// Never recovered from, never retried. A negative cash wallet means
			// a control was bypassed, and chasing the attendee for R40 is both
			// wrong and a distraction from the actual finding.
			if err := repository.SetShortfallState(ctx, pool, s.ID, "investigating", reason); err != nil {
				log.Error().Err(err).Str("shortfall_id", s.ID).Msg("recovery worker: state update failed")
			}
			log.Warn().Str("shortfall_id", s.ID).Str("wallet_id", s.WalletID).
				Int64("cents", s.AmountCents).
				Msg("recovery worker: cash wallet went negative — offline cap was bypassed")

		case repository.RecoveryCharge:
			owed := s.AmountCents - s.RecoveredCents
			if r := repository.RemainingMandateCents(mandate); r < owed {
				owed = r
			}
			if charger == nil {
				// Report-only. Logged so the volume is visible before anyone
				// switches charging on for real.
				log.Info().Str("shortfall_id", s.ID).Int64("would_charge_cents", owed).
					Msg("recovery worker: mandate charge available (report-only mode)")
				skipped++
				continue
			}
			ref := "rec_" + s.ID
			if err := charger.ChargeAuthorization(ctx, mandate.AuthorizationCode, owed, ref); err != nil {
				_ = repository.RecordRecoveryFailure(ctx, pool, s.ID, err.Error())
				log.Warn().Err(err).Str("shortfall_id", s.ID).Msg("recovery worker: charge failed")
				continue
			}
			if err := repository.RecordRecoveryCharge(ctx, pool, s.ID, s.WalletID,
				s.EventID, derefMandateID(s.MandateID), owed); err != nil {
				// The card WAS charged but the ledger write failed. Logged at
				// error because it needs a human: the attendee has paid and the
				// system does not know it.
				log.Error().Err(err).Str("shortfall_id", s.ID).
					Msg("recovery worker: charged but not ledgered — RECONCILE MANUALLY")
				continue
			}
			charged++

		case repository.RecoveryAskToSettle:
			// The settle link itself is not built yet — see PHASE4D notes. The
			// case moves to awaiting_settlement with no token so it is visible
			// in the organiser view, rather than being retried forever.
			if err := repository.MarkSettleLinkSent(ctx, pool, s.ID, ""); err != nil {
				log.Error().Err(err).Str("shortfall_id", s.ID).Msg("recovery worker: state update failed")
			}
			asked++

		case repository.RecoveryManual:
			if err := repository.SetShortfallState(ctx, pool, s.ID, "manual_review", reason); err != nil {
				log.Error().Err(err).Str("shortfall_id", s.ID).Msg("recovery worker: state update failed")
			}
			manual++

		case repository.RecoveryWriteOff:
			if err := repository.SetShortfallState(ctx, pool, s.ID, "written_off", reason); err != nil {
				log.Error().Err(err).Str("shortfall_id", s.ID).Msg("recovery worker: state update failed")
			}
			wroteOff++
		}
	}

	log.Info().
		Int("cases", len(cases)).Int("charged", charged).Int("asked", asked).
		Int("manual", manual).Int("written_off", wroteOff).Int("skipped", skipped).
		Msg("recovery worker: pass complete")
}

const (
	// maxRecoveryAttempts before a case goes to a human. Not an automatic
	// write-off — somebody should look before money is forgiven.
	maxRecoveryAttempts = 3

	// recoveryGraceDays after a settlement request before writing off. §6
	// commits to a published unspent-balance policy; the negative side deserves
	// the same treatment, and this number should match whatever you publish.
	recoveryGraceDays = 14
)

func derefMandateID(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// StartShortfallDetector scans for wallets that have gone negative.
//
// Separate from the recovery loop and much more frequent: detection is a cheap
// query and a shortfall that goes unnoticed for an hour is an hour in which the
// band keeps spending. Recovery is the slow, careful half.
func StartShortfallDetector(pool *pgxpool.Pool, ctx context.Context) {
	go func() {
		log.Info().Msg("shortfall detector: started")
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				runShortfallDetection(ctx, pool)
			case <-ctx.Done():
				log.Info().Msg("shortfall detector: stopped")
				return
			}
		}
	}()
}

func runShortfallDetection(ctx context.Context, pool *pgxpool.Pool) {
	// Only events with offline spend enabled can produce a shortfall, so the
	// scan is scoped to them rather than every event ever created.
	rows, err := pool.Query(ctx, `
		SELECT event_id FROM event_cashless_config WHERE offline_policy = 'enabled'
	`)
	if err != nil {
		log.Error().Err(err).Msg("shortfall detector: could not list offline events")
		return
	}
	defer rows.Close()

	events := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return
		}
		events = append(events, id)
	}
	if err := rows.Err(); err != nil {
		return
	}

	total := 0
	for _, eventID := range events {
		n, err := repository.DetectShortfalls(ctx, pool, eventID)
		if err != nil {
			log.Error().Err(err).Str("event_id", eventID).
				Msg("shortfall detector: detection failed")
			continue
		}
		total += n
	}
	if total > 0 {
		log.Warn().Int("shortfalls", total).Int("events", len(events)).
			Msg("shortfall detector: negative wallets found")
	}
}