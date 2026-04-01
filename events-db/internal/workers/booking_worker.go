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
	log.Printf("booking worker: processing job (booking_id=%s)", bookingID)
 
	booking, err := repository.GetBookingByID(pool, bookingID)
	if err != nil {
		log.Printf("booking worker: failed to fetch booking %s: %v", bookingID, err)
		return
	}
	if booking.BookingStatus != models.BookingConfirmed {
		log.Printf("booking worker: booking %s is not confirmed (status=%s), skipping",
			bookingID, booking.BookingStatus)
		return
	}
	if booking.TicketsSent {
		log.Printf("booking worker: tickets already sent for booking %s, skipping", bookingID)
		return
	}
 
	event, err := repository.GetEventByID(pool, booking.EventID, nil)
	if err != nil {
		log.Printf("booking worker: failed to fetch event %s for booking %s: %v",
			booking.EventID, bookingID, err)
		return
	}
 
	user, err := resolveEmailRecipient(pool, booking)
	if err != nil {
		log.Printf("booking worker: failed to resolve recipient for booking %s: %v",
			bookingID, err)
		return
	}
 
	// ── Step 1: Create assignment rows ────────────────────────────────────────
	// Slot 1 is auto-claimed by the buyer; remaining slots start as unassigned.
	if err := repository.CreateTicketAssignments(
		pool,
		bookingID,
		booking.TicketQuantity,
		user.GetFullName(),
		user.Email,
	); err != nil {
		log.Printf("booking worker: failed to create assignments for booking %s: %v",
			bookingID, err)
		// Non-fatal — carry on and attempt QR + email for the buyer's slot.
	}
 
	// ── Step 2: Generate a QR code for every assignment slot ─────────────────
	assignments, err := fetchAssignmentIDsForBooking(pool, bookingID)
	if err != nil {
		log.Printf("booking worker: failed to fetch assignment IDs for %s: %v", bookingID, err)
	}
 
	var buyerQRURL string
	for i, assignmentID := range assignments {
		if err := qrSvc.GenerateQRCodeForAssignment(assignmentID); err != nil {
			log.Printf("booking worker: QR generation failed for assignment %s: %v",
				assignmentID, err)
			continue
		}
		if i == 0 {
			// Slot 1 QR URL — used in the buyer's confirmation email.
			buyerQRURL = fetchQRURLForAssignment(pool, assignmentID)
		}
	}
 
	// ── Step 3: Send buyer confirmation email (slot 1 QR) ────────────────────
	if err := emailSvc.SendBookingConfirmationWithQR(booking, event, user, buyerQRURL); err != nil {
		log.Printf("booking worker: email failed for booking %s: %v", bookingID, err)
		time.Sleep(3 * time.Second)
		if retryErr := emailSvc.SendBookingConfirmationWithQR(booking, event, user, buyerQRURL); retryErr != nil {
			log.Printf("booking worker: email retry also failed for booking %s: %v",
				bookingID, retryErr)
		}
		return
	}
 
	// ── Step 4: In-app notification ───────────────────────────────────────────
	if booking.UserID != nil {
		notifErr := repository.CreateNotification(
			pool, context.Background(),
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
			log.Printf("booking worker: notification failed for %s: %v", booking.ID, notifErr)
		}
	}
 
	log.Printf("booking worker: successfully processed booking %s (%d ticket slots)",
		bookingID, booking.TicketQuantity)
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