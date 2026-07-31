package workers

import (
	"context"
	"errors"
	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// isPermanentEmailErr reports whether an email send failure is a non-retryable
// auth/config problem (unverified SES sender or suspended/paused account). Retrying
// such a failure is pointless — it only churns the queue and the logs.
func isPermanentEmailErr(err error) bool {
	var apiErr *services.EmailAPIError
	return errors.As(err, &apiErr) && apiErr.IsPermanent()
}

// retryBackoffs defines the wait duration before each successive retry attempt.
// Attempt 0 → 30 s, attempt 1 → 2 min, attempt 2 → 10 min, attempt 3 → 30 min,
// attempt 4+ → 1 h. The slice is indexed by attempts already made; if attempts
// exceeds the slice length the last value is used.
var retryBackoffs = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	1 * time.Hour,
}

func backoffFor(attempts int) time.Duration {
	if attempts < len(retryBackoffs) {
		return retryBackoffs[attempts]
	}
	return retryBackoffs[len(retryBackoffs)-1]
}

// StartBookingWorker drains the jobs channel and, for each confirmed booking:
//  1. Generates + uploads the QR code to R2.
//  2. Sends the booking confirmation email via Amazon SES.
//
// FIX 4: On email failure the worker now persists a row to
// booking_email_retries instead of spawning a fire-and-forget goroutine that
// would be silently lost on process restart. A periodic drain goroutine retries
// those rows with exponential back-off. Rows that exhaust max_attempts are
// marked 'failed' and left for ops review rather than being silently dropped.
//
// Buffer the jobs channel generously (e.g. 256) so the webhook handler never
// blocks.
func StartBookingWorker(
	pool *pgxpool.Pool,
	cfg *config.Config,
	jobs <-chan string,
	ctx context.Context,
) {
	emailSvc := services.NewEmailService(cfg)
	qrSvc := services.NewQRService(pool, cfg)

	// Drain any retries that survived a previous crash before accepting new jobs.
	drainEmailRetries(ctx, pool, emailSvc)

	// Periodic retry drain — runs every 5 minutes while the server is up.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				drainEmailRetries(ctx, pool, emailSvc)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Main job loop.
	go func() {
		log.Info().Msg("booking worker: started")
		for {
			select {
			case bookingID, ok := <-jobs:
				if !ok {
					log.Info().Msg("booking worker: channel closed, stopping")
					return
				}
				processBookingJob(ctx, pool, emailSvc, qrSvc, bookingID)

			case <-ctx.Done():
				log.Info().Msg("booking worker: context cancelled, stopping")
				return
			}
		}
	}()
}

func processBookingJob(
	ctx context.Context,
	pool *pgxpool.Pool,
	emailSvc *services.EmailService,
	qrSvc *services.QRService,
	bookingID string,
) {
	log.Info().Str("booking_id", bookingID).Msg("booking worker: processing job")

	booking, err := repository.GetBookingByID(pool, bookingID)
	if err != nil {
		log.Error().Err(err).Str("booking_id", bookingID).Msg("booking worker: failed to fetch booking")
		return
	}
	if booking.BookingStatus != models.BookingConfirmed {
		log.Warn().
			Str("booking_id", bookingID).
			Str("status", string(booking.BookingStatus)).
			Msg("booking worker: booking not confirmed, skipping")
		return
	}
	if booking.TicketsSent {
		log.Info().Str("booking_id", bookingID).Msg("booking worker: tickets already sent, skipping")
		return
	}

	// Merch-only bookings have no tickets or QR codes to generate. Mark them
	// sent so the startup re-enqueue (confirmed + tickets_sent = FALSE, last
	// 24h) stops picking them up on every restart.
	if booking.BookingType == "merch_only" {
		log.Info().Str("booking_id", bookingID).Msg("booking worker: merch-only booking, nothing to send")
		markTicketsSent(pool, bookingID)
		return
	}

	eventID := func() string { if booking.EventID != nil { return *booking.EventID }; return "" }()

	// GetEventByIDInternal, NOT GetEventByID: the request-path lookup filters to
	// published/published_private, so a booking for an event that has since rolled
	// to 'completed' or been 'cancelled' looked like a missing event. Combined with
	// the branch below that treated every error as permanent, that marked real
	// paid bookings as delivered without ever sending a ticket.
	event, err := repository.GetEventByIDInternal(pool, eventID)
	if err != nil {
		if errors.Is(err, repository.ErrEventNotFound) {
			// The row is genuinely gone (deleted/purged). Nothing can ever be
			// sent, so mark it sent — otherwise the startup sweep re-enqueues it
			// on EVERY restart and logs this forever.
			log.Error().
				Str("event_id", eventID).
				Str("booking_id", bookingID).
				Msg("booking worker: event row missing — marking booking sent to stop requeue churn")
			markTicketsSent(pool, bookingID)
			return
		}

		// Transient failure (DB blip, timeout, RDS failover). Leave tickets_sent
		// FALSE so the next sweep retries. Marking it sent here would silently
		// deny a paying customer their ticket over a five-second outage.
		log.Error().Err(err).
			Str("event_id", eventID).
			Str("booking_id", bookingID).
			Msg("booking worker: transient event fetch failure — leaving booking for retry")
		return
	}

	user, err := resolveEmailRecipient(pool, booking)
	if err != nil {
		log.Error().Err(err).Str("booking_id", bookingID).Msg("booking worker: failed to resolve recipient")
		return
	}

	if err := repository.CreateTicketAssignments(
		pool,
		bookingID,
		booking.TicketQuantity,
		user.GetFullName(),
		user.Email,
	); err != nil {
		log.Error().Err(err).Str("booking_id", bookingID).Msg("booking worker: failed to create assignments")
		// Non-fatal — carry on and attempt QR + email for the buyer's slot.
	}

	assignments, err := fetchAssignmentIDsForBooking(pool, bookingID)
	if err != nil {
		log.Error().Err(err).Str("booking_id", bookingID).Msg("booking worker: failed to fetch assignment IDs")
	}

	// QR codes are still generated per assignment — they back the rotating
	// entry QR in the app/web — but they are no longer embedded in the
	// confirmation email.
	for _, assignmentID := range assignments {
		if err := qrSvc.GenerateQRCodeForAssignment(assignmentID); err != nil {
			log.Error().Err(err).Str("assignment_id", assignmentID).Msg("booking worker: QR generation failed")
			continue
		}
	}

	if err := emailSvc.SendBookingConfirmation(booking, event, user); err != nil {
		// A permanent SES rejection (unverified sender, suspended/paused
		// account, or malformed message) won't fix itself on retry. Tickets are already issued, so
		// just log it once and skip the retry queue to avoid endless churn.
		if isPermanentEmailErr(err) {
			log.Error().Err(err).Str("booking_id", bookingID).
				Msg("booking worker: email rejected by SES (permanent) — NOT queuing retry; verify the sender identity / check SES account status. Tickets were still issued.")
			return
		}

		log.Error().Err(err).Str("booking_id", bookingID).
			Msg("booking worker: email failed — persisting to retry queue")

		// FIX 4: persist to DB instead of launching a goroutine.
		// The retry drain will pick this up within 5 minutes (or immediately
		// on the next server start), so no work is lost on a crash.
		if persistErr := persistEmailRetry(pool, bookingID, err.Error()); persistErr != nil {
			log.Error().Err(persistErr).Str("booking_id", bookingID).
				Msg("booking worker: CRITICAL — could not persist retry row; email may be lost")
		}
		return
	}

	// Email delivered — record it. This was the missing piece behind the
	// "every rebuild resends today's booking emails" bug: the happy path
	// never flipped tickets_sent, so the startup re-enqueue (confirmed +
	// tickets_sent = FALSE, last 24h) reprocessed and re-emailed every
	// booking on each restart. Only the retry-drain path marked it.
	markTicketsSent(pool, bookingID)

	// Use the passed ctx instead of context.Background() so the notification
	// write respects graceful shutdown and doesn't outlive the server.
	if booking.UserID != nil {
		notifErr := repository.CreateNotification(
			pool, ctx,
			*booking.UserID,
			models.NotifBookingConfirmed,
			"Booking Confirmed!",
			fmt.Sprintf("Your booking for %s is confirmed. You have %d ticket(s) to manage.",
				event.Title, booking.TicketQuantity),
			map[string]string{
				"booking_id":     booking.ID,
				"event_id":       event.ID,
				"booking_number": booking.BookingNumber,
			},
		)
		if notifErr != nil {
			log.Error().Err(notifErr).Str("booking_id", booking.ID).Msg("booking worker: notification failed")
		}
	}

	log.Info().
		Str("booking_id", bookingID).
		Int("ticket_slots", booking.TicketQuantity).
		Msg("booking worker: successfully processed booking")
}

// persistEmailRetry inserts a new retry row for a failed email send.
func persistEmailRetry(pool *pgxpool.Pool, bookingID, lastError string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO booking_email_retries (booking_id, last_error, retry_after)
		VALUES ($1, $2, NOW() + $3::interval)
		ON CONFLICT DO NOTHING
	`, bookingID, lastError, backoffFor(0).String())
	return err
}

// drainEmailRetries fetches all pending retry rows whose back-off has elapsed
// and attempts to re-send the confirmation email for each one.
func drainEmailRetries(
	ctx context.Context,
	pool *pgxpool.Pool,
	emailSvc *services.EmailService,

) {
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	type retryRow struct {
		id        string
		bookingID string
		attempts  int
	}

	rows, err := pool.Query(queryCtx, `
		SELECT id, booking_id, attempts
		FROM   booking_email_retries
		WHERE  status = 'pending'
		AND    retry_after <= NOW()
		ORDER  BY retry_after
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		log.Error().Err(err).Msg("booking worker: retry drain query failed")
		return
	}

	var pending []retryRow
	for rows.Next() {
		var r retryRow
		if err := rows.Scan(&r.id, &r.bookingID, &r.attempts); err != nil {
			log.Error().Err(err).Msg("booking worker: retry drain scan failed")
			continue
		}
		pending = append(pending, r)
	}
	rows.Close()

	if len(pending) == 0 {
		return
	}

	log.Info().Int("count", len(pending)).Msg("booking worker: draining email retry queue")

	for _, r := range pending {
		if err := retryEmailSend(ctx, pool, emailSvc, r.id, r.bookingID, r.attempts); err != nil {
			log.Error().Err(err).
				Str("retry_id", r.id).
				Str("booking_id", r.bookingID).
				Msg("booking worker: retry attempt failed")
		}
	}
}

// retryEmailSend attempts a single email re-send and updates the retry row.
func retryEmailSend(
	ctx context.Context,
	pool *pgxpool.Pool,
	emailSvc *services.EmailService,
	retryID, bookingID string,
	attempts int,
) error {
	booking, err := repository.GetBookingByID(pool, bookingID)
	if err != nil {
		return markRetryFailed(pool, retryID, fmt.Sprintf("fetch booking: %v", err))
	}
	if booking.TicketsSent {
		// Another path already sent it — clean up.
		return markRetrySent(pool, retryID)
	}

	// Internal fetch, same reason as the main send path: this retry drain runs
	// long after the booking was made, so the event has very often rolled to
	// 'completed' by now. The request-path lookup would report it missing and
	// permanently fail the retry for an event that is sitting right there.
	event, err := repository.GetEventByIDInternal(pool, func() string { if booking.EventID != nil { return *booking.EventID }; return "" }())
	if err != nil {
		return markRetryFailed(pool, retryID, fmt.Sprintf("fetch event: %v", err))
	}

	user, err := resolveEmailRecipient(pool, booking)
	if err != nil {
		return markRetryFailed(pool, retryID, fmt.Sprintf("resolve recipient: %v", err))
	}

	newAttempts := attempts + 1

	if sendErr := emailSvc.SendBookingConfirmation(booking, event, user); sendErr != nil {
		// A permanent auth/config failure won't resolve on retry — mark it
		// failed now so it stops draining every cycle.
		if isPermanentEmailErr(sendErr) {
			log.Error().Err(sendErr).Str("booking_id", bookingID).Str("retry_id", retryID).
				Msg("booking worker: email permanently rejected (auth/config) — marking failed, no further retries")
			return markRetryFailed(pool, retryID, fmt.Sprintf("permanent: %v", sendErr))
		}

		// Check whether we've hit the attempt cap.
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var maxAttempts int
		pool.QueryRow(updateCtx, `SELECT max_attempts FROM booking_email_retries WHERE id = $1`, retryID).
			Scan(&maxAttempts)

		if newAttempts >= maxAttempts {
			log.Error().
				Str("retry_id", retryID).
				Str("booking_id", bookingID).
				Int("attempts", newAttempts).
				Msg("booking worker: retry exhausted max_attempts — marking failed")
			return markRetryFailed(pool, retryID, fmt.Sprintf("attempt %d: %v", newAttempts, sendErr))
		}

		// Schedule the next attempt with exponential back-off.
		nextRetry := time.Now().Add(backoffFor(newAttempts))
		_, err = pool.Exec(updateCtx, `
			UPDATE booking_email_retries
			SET    attempts    = $1,
			       last_error  = $2,
			       retry_after = $3,
			       updated_at  = NOW()
			WHERE  id = $4
		`, newAttempts, sendErr.Error(), nextRetry, retryID)
		return err
	}

	// Email sent — mark success and record tickets_sent on the booking.
	if err := markRetrySent(pool, retryID); err != nil {
		log.Error().Err(err).Str("retry_id", retryID).Msg("booking worker: failed to mark retry sent")
	}
	markTicketsSent(pool, bookingID)

	log.Info().
		Str("retry_id", retryID).
		Str("booking_id", bookingID).
		Int("attempt", newAttempts).
		Msg("booking worker: retry email succeeded")

	// Notification for user (best-effort).
	if booking.UserID != nil {
		_ = repository.CreateNotification(
			pool, ctx,
			*booking.UserID,
			models.NotifBookingConfirmed,
			"Booking Confirmed!",
			fmt.Sprintf("Your booking for %s is confirmed. You have %d ticket(s) to manage.",
				event.Title, booking.TicketQuantity),
			map[string]string{
				"booking_id":     booking.ID,
				"event_id":       event.ID,
				"booking_number": booking.BookingNumber,
			},
		)
	}
	return nil
}

func markRetrySent(pool *pgxpool.Pool, retryID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
		UPDATE booking_email_retries SET status = 'sent', updated_at = NOW() WHERE id = $1
	`, retryID)
	return err
}

func markRetryFailed(pool *pgxpool.Pool, retryID, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
		UPDATE booking_email_retries
		SET    status = 'failed', last_error = $1, updated_at = NOW()
		WHERE  id = $2
	`, reason, retryID)
	return err
}

func markTicketsSent(pool *pgxpool.Pool, bookingID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx,
		`UPDATE bookings SET tickets_sent = TRUE WHERE id = $1`, bookingID,
	); err != nil {
		log.Error().Err(err).Str("booking_id", bookingID).
			Msg("booking worker: failed to mark tickets_sent")
	}
}

// resolveEmailRecipient returns a *models.User suitable for use in email templates.
// Guest bookings use the attendee fields stored on the booking itself.
// Registered users are fetched from the DB.
func resolveEmailRecipient(pool *pgxpool.Pool, booking *models.Booking) (*models.User, error) {
	if booking.IsGuest {
		firstName := ""
		lastName := ""
		email := ""
		if booking.AttendeeFirstName != nil {
			firstName = *booking.AttendeeFirstName
		}
		if booking.AttendeeLastName != nil {
			lastName = *booking.AttendeeLastName
		}
		if booking.AttendeeEmail != nil {
			email = *booking.AttendeeEmail
		}
		return &models.User{
			Email:     email,
			FirstName: &firstName,
			LastName:  &lastName,
		}, nil
	}

	if booking.UserID == nil {
		email := ""
		if booking.AttendeeEmail != nil {
			email = *booking.AttendeeEmail
		}
		return &models.User{Email: email}, nil
	}

	return repository.GetUserByIDForEmail(pool, *booking.UserID)
}

func fetchAssignmentIDsForBooking(pool *pgxpool.Pool, bookingID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx,
		`SELECT id FROM ticket_assignments WHERE booking_id = $1 ORDER BY slot_number`,
		bookingID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}