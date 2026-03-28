package workers

import (
	"context"
	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
		log.Println("booking worker: started")
		for {
			select {
			case bookingID, ok := <-jobs:
				if !ok {
					log.Println("booking worker: channel closed, stopping")
					return
				}
				processBookingJob(pool, emailSvc, qrSvc, bookingID)

			case <-ctx.Done():
				log.Println("booking worker: context cancelled, stopping")
				return
			}
		}
	}()
}

func processBookingJob(
	pool *pgxpool.Pool,
	emailSvc *services.EmailService,
	qrSvc *services.QRService,
	bookingID string,
) {
	log.Printf("booking worker: processing job for booking %s", bookingID)

	// 1. Fetch the full booking record
	booking, err := repository.GetBookingByID(pool, bookingID)
	if err != nil {
		log.Printf("booking worker: failed to fetch booking %s: %v", bookingID, err)
		return
	}

	// Guard: only proceed for confirmed bookings
	if booking.BookingStatus != models.BookingConfirmed {
		log.Printf("booking worker: booking %s is not confirmed (status=%s), skipping",
			bookingID, booking.BookingStatus)
		return
	}

	// Guard: skip if tickets already sent (handles duplicate webhook delivery)
	if booking.TicketsSent {
		log.Printf("booking worker: tickets already sent for booking %s, skipping", bookingID)
		return
	}

	// 2. Fetch the event
	event, err := repository.GetEventByID(pool, booking.EventID, nil)
	if err != nil {
		log.Printf("booking worker: failed to fetch event %s for booking %s: %v",
			booking.EventID, bookingID, err)
		return
	}

	// 3. Resolve the email recipient — guest or registered user
	user, err := resolveEmailRecipient(pool, booking)
	if err != nil {
		log.Printf("booking worker: failed to resolve recipient for booking %s: %v",
			bookingID, err)
		return
	}

	// 4. Generate QR code (encrypts payload → uploads PNG to R2 → updates booking row)
	if err := qrSvc.GenerateQRCode(bookingID); err != nil {
		log.Printf("booking worker: QR generation failed for booking %s: %v", bookingID, err)
		// Don't return — still attempt to send the email even without the QR.
		// The email template gracefully handles an empty QRCodeURL.
	}

	// Re-fetch so the email template has the freshly written qr_code_url
	booking, err = repository.GetBookingByID(pool, bookingID)
	if err != nil {
		log.Printf("booking worker: re-fetch after QR failed for booking %s: %v", bookingID, err)
		return
	}

	// 5. Send the confirmation email, with one retry on transient failure
	if err := emailSvc.SendBookingConfirmation(booking, event, user); err != nil {
		log.Printf("booking worker: email failed for booking %s: %v", bookingID, err)
		time.Sleep(3 * time.Second)
		if retryErr := emailSvc.SendBookingConfirmation(booking, event, user); retryErr != nil {
			log.Printf("booking worker: email retry also failed for booking %s: %v",
				bookingID, retryErr)
		}
		return
	}

	// Non-fatal notification — guest bookings have no user_id so skip them
	if booking.UserID != nil {
		notifErr := repository.CreateNotification(
			pool, context.Background(),
			*booking.UserID,
			models.NotifBookingConfirmed,
			"Booking Confirmed!",
			fmt.Sprintf("Your booking for %s is confirmed. Check your email for your ticket.", event.Title),
			map[string]string{
				"booking_id":     booking.ID,
				"event_id":       event.ID,
				"booking_number": booking.BookingNumber,
			},
		)
		if notifErr != nil {
			log.Printf("booking worker: notification failed for %s: %v", booking.ID, notifErr)
		}
	}

	log.Printf("booking worker: successfully processed booking %s", bookingID)
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