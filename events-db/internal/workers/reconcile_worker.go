package workers

import (
	"context"
	"time"

	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// StartReconcileWorker closes two gaps that both ended the same way: the buyer
// pays, and gets nothing.
//
//  1. Unsent tickets. PaymentService.enqueue drops the ticket-generation job on
//     a full channel — deliberately, so a slow worker can never block a webhook.
//     The only recovery was repository.GetUnsentConfirmedBookingIDs, called once
//     at startup with a 24-hour lookback. On a box that goes days between
//     deploys a dropped job was therefore lost permanently: booking confirmed,
//     tickets_sent FALSE, no QR, no email, and a single warn line as the only
//     trace. Running the same sweep on a ticker recovers it within one interval,
//     and also covers a crash between "payment applied" and "job consumed".
//
//  2. Stuck payments. The Paystack webhook returns 200 even when Verify or Apply
//     fails, deliberately, so the gateway stops retrying — its comment defers to
//     "a reconciliation job". There wasn't one. The only callers of
//     ReconcilePendingPayment were the two client-driven status polls, so if the
//     buyer closed the tab or lost signal nothing reconciled, and expireBookings
//     cancelled the booking ~15 minutes later with their money already taken.
//
// Both sweeps are safe to repeat. ReconcilePendingPayment is idempotent, and the
// booking worker no-ops on a booking whose tickets_sent is already TRUE.
func StartReconcileWorker(
	pool *pgxpool.Pool,
	paymentSvc *services.PaymentService,
	jobs chan<- string,
	ctx context.Context,
	interval time.Duration,
) {
	if interval <= 0 {
		interval = 2 * time.Minute
	}

	go func() {
		// A panic on a non-handler goroutine is not covered by gin.Recovery()
		// and would take the whole process down. Log it and stop this worker
		// rather than the API.
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).
					Msg("reconcile worker: panicked, worker stopped")
			}
		}()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		log.Info().Dur("interval", interval).Msg("reconcile worker: started")

		for {
			select {
			case <-ticker.C:
				runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
				reconcileUnsentTickets(runCtx, pool, jobs)
				reconcileStuckPayments(runCtx, pool, paymentSvc)
				cancel()
			case <-ctx.Done():
				log.Info().Msg("reconcile worker: stopped")
				return
			}
		}
	}()
}

// reconcileUnsentTickets re-queues confirmed bookings whose tickets never went
// out.
//
// The send is non-blocking on purpose: if the channel is full the remaining
// bookings stay unsent and are picked up next tick. A blocking send here would
// reproduce the startup deadlock this worker exists to backstop.
func reconcileUnsentTickets(ctx context.Context, pool *pgxpool.Pool, jobs chan<- string) {
	ids, err := repository.GetUnsentConfirmedBookingIDs(pool)
	if err != nil {
		log.Error().Err(err).Msg("reconcile: querying unsent bookings")
		return
	}
	if len(ids) == 0 {
		return
	}

	queued := 0
	for _, id := range ids {
		select {
		case jobs <- id:
			queued++
		default:
			log.Warn().Int("queued", queued).Int("total", len(ids)).
				Msg("reconcile: job channel full, remaining bookings deferred to next tick")
			return
		}
	}
	log.Info().Int("count", queued).Msg("reconcile: re-queued unsent booking jobs")
}

// reconcileStuckPayments asks the gateway for the truth about bookings still
// pending past the point a webhook should have landed.
//
// The lower bound (2 minutes) avoids racing a webhook that is simply in flight.
// The upper bound (24 hours) keeps the sweep off long-dead rows that
// expireBookings has already cancelled — those need a human, not a retry.
// LIMIT caps the work per tick so a backlog degrades throughput rather than
// blowing the run timeout.
func reconcileStuckPayments(ctx context.Context, pool *pgxpool.Pool, paymentSvc *services.PaymentService) {
	rows, err := pool.Query(ctx, `
		SELECT id
		FROM   bookings
		WHERE  payment_status = 'pending'
		AND    booking_status IN ('pending', 'confirmed')
		AND    created_at < NOW() - INTERVAL '2 minutes'
		AND    created_at > NOW() - INTERVAL '24 hours'
		LIMIT  50
	`)
	if err != nil {
		log.Error().Err(err).Msg("reconcile: querying stuck payments")
		return
	}

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			log.Error().Err(err).Msg("reconcile: scanning stuck payment row")
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Msg("reconcile: iterating stuck payments")
		return
	}
	if len(ids) == 0 {
		return
	}

	resolved := 0
	for _, id := range ids {
		if _, err := paymentSvc.ReconcilePendingPayment(ctx, id); err != nil {
			// Expected for genuinely abandoned checkouts — the buyer never paid.
			// Warn rather than error so a real gateway outage still stands out.
			log.Warn().Err(err).Str("booking_id", id).
				Msg("reconcile: payment still unresolved")
			continue
		}
		resolved++
	}
	log.Info().Int("checked", len(ids)).Int("resolved", resolved).
		Msg("reconcile: swept pending payments")
}