package workers

import (
	"context"
	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log" 
)

// StartBookingWorker drains the jobs channel and, for each confirmed booking:
//  1. Generates + uploads the QR code to R2
//  2. Sends the booking confirmation email via SendGrid
//
// Buffer the channel generously (e.g. 256) so the webhook handler never blocks.
func StartBookingWorker(
	pool *pgxpool.Pool,
	cfg *config.Config,
	jobs <-chan string,
	ctx context.Context,
) {
	emailSvc := services.NewEmailService(cfg)
	qrSvc := services.NewQRService(pool, cfg)

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
	ctx context.Context, // FIX 5: accept ctx so notifications respect graceful shutdown
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
 
	event, err := repository.GetEventByID(pool, booking.EventID, nil)
	if err != nil {
		log.Error().Err(err).
			Str("event_id", booking.EventID).
			Str("booking_id", bookingID).
			Msg("booking worker: failed to fetch event")
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
 
	var buyerQRURL string
	for i, assignmentID := range assignments {
		if err := qrSvc.GenerateQRCodeForAssignment(assignmentID); err != nil {
			log.Error().Err(err).Str("assignment_id", assignmentID).Msg("booking worker: QR generation failed")
			continue
		}
		if i == 0 {
			buyerQRURL = fetchQRURLForAssignment(pool, assignmentID)
		}
	}
 
	if err := emailSvc.SendBookingConfirmationWithQR(booking, event, user, buyerQRURL); err != nil {
		log.Error().Err(err).Str("booking_id", bookingID).Msg("booking worker: email failed, retrying in 3s")
		time.Sleep(3 * time.Second)
		if retryErr := emailSvc.SendBookingConfirmationWithQR(booking, event, user, buyerQRURL); retryErr != nil {
			log.Error().Err(retryErr).Str("booking_id", bookingID).Msg("booking worker: email retry also failed")
		}
		return
	}
 
	// FIX 5: use the passed ctx instead of context.Background() so this
	// respects graceful shutdown and doesn't outlive the server.
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

	// Registered user — fetch from DB
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
 
func fetchQRURLForAssignment(pool *pgxpool.Pool, assignmentID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
 
	var url *string
	pool.QueryRow(ctx,
		`SELECT qr_code_url FROM ticket_assignments WHERE id = $1`,
		assignmentID,
	).Scan(&url)
 
	if url == nil {
		return ""
	}
	return *url
}