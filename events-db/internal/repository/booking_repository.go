package repository

import (
	"context"
	"crypto/subtle"
	"errors"
	"events-db/internal/models"
	"events-db/internal/utils"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrBookingNotFound = errors.New("booking not found")
var ErrInsufficientCapacity = errors.New("insufficient tickets available")

type CancelledAssignee struct {
	Name  string
	Email string
	Slot  int
}

func createBookingTx(
	pool *pgxpool.Pool,
	eventID string,
	userID *string, 
	isGuest bool,
	quantity int,
	unitPrice float64,
	currency string,
	firstName *string,
	lastName *string,
	email *string,
	phone *string,
	specialReqs *string,
	promoCode *string,
) (*models.Booking, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	subtotal := unitPrice * float64(quantity)
	platformFee := CalculatePlatformFee(subtotal)
	organizerFee := 0.0
	discountAmount := 0.0
	totalAmount := subtotal + platformFee + organizerFee - discountAmount

	if quantity <= 0 {
    	return nil, errors.New("invalid ticket quantity")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the event row and verify availability atomically.
	var maxCapacity *int
	var ticketsSold int
	err = tx.QueryRow(ctx, `
		SELECT max_capacity, tickets_sold
		FROM events
		WHERE id = $1 AND status = $2
		FOR UPDATE
	`, eventID, models.EventPublished).Scan(&maxCapacity, &ticketsSold)
	if err != nil {
		return nil, fmt.Errorf("failed to lock event: %w", err)
	}

	if maxCapacity != nil && ticketsSold+quantity > *maxCapacity {
		return nil, ErrInsufficientCapacity
	}

	var booking models.Booking
	err = tx.QueryRow(ctx, `
		INSERT INTO bookings (
			event_id, user_id, is_guest, ticket_quantity, unit_price, subtotal,
			platform_fee, organizer_fee, total_amount, currency,
			attendee_first_name, attendee_last_name, attendee_email, attendee_phone,
			special_requirements, promo_code, discount_amount,
			booking_status, payment_status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING id, booking_number, created_at
	`,
		eventID, userID, isGuest, quantity, unitPrice, subtotal,
		platformFee, organizerFee, totalAmount, currency,
		firstName, lastName, email, phone,
		specialReqs, promoCode, discountAmount,
		models.BookingPending, models.PaymentPending,
	).Scan(&booking.ID, &booking.BookingNumber, &booking.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create booking: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE events SET tickets_sold = tickets_sold + $1 WHERE id = $2
	`, quantity, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to increment tickets sold: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit booking: %w", err)
	}

	booking.EventID = eventID
	booking.UserID = userID
	booking.IsGuest = isGuest
	booking.TicketQuantity = quantity
	booking.TotalAmount = totalAmount
	booking.Currency = currency
	booking.AttendeeFirstName = firstName
	booking.AttendeeLastName = lastName
	booking.AttendeeEmail = email
	booking.BookingStatus = models.BookingPending
	booking.PaymentStatus = models.PaymentPending

	return &booking, nil
}

func CreateBooking(pool *pgxpool.Pool, dto *models.CreateBookingDTO, userID string, event *models.Event) (*models.Booking, error) {
	uid := userID
	return createBookingTx(
		pool,
		dto.EventID,
		&uid,
		false, // not a guest
		dto.TicketQuantity,
		event.Price,
		event.Currency,
		dto.AttendeeFirstName,
		dto.AttendeeLastName,
		dto.AttendeeEmail,
		dto.AttendeePhone,
		dto.SpecialRequirements,
		dto.PromoCode,
	)
}

func CreateGuestBooking(pool *pgxpool.Pool, dto *models.CreateGuestBookingDTO, event *models.Event) (*models.Booking, error) {
	return createBookingTx(
		pool,
		dto.EventID,
		nil, // no user_id
		true, // is_guest = true
		dto.TicketQuantity,
		event.Price,
		event.Currency,
		&dto.AttendeeFirstName,
		&dto.AttendeeLastName,
		&dto.AttendeeEmail,
		dto.AttendeePhone,
		dto.SpecialRequirements,
		dto.PromoCode,
	)
}

func CalculatePlatformFee(subtotal float64) float64 {
	// example platform fees such as stipe
	percentage := subtotal * 0.029
	fixed := 0.30
	return  percentage + fixed
}

func ConfirmBooking(pool *pgxpool.Pool, bookingID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5 *time.Second)
	defer cancel()

	query := `
		UPDATE bookings
		SET booking_status = $1, payment_status = $2, confirmed_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`
	cmd, err := pool.Exec(ctx, query, models.BookingConfirmed, models.PaymentCompleted, bookingID)
	if err != nil {
		return fmt.Errorf("failed to confirm booking: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ErrBookingNotFound
	}
	return nil
}

func GetBookingByID(pool *pgxpool.Pool, bookingID string) (*models.Booking, error) {
	ctx,cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT
			b.id, b.booking_number, b.event_id, b.user_id, b.is_guest,
			b.ticket_quantity, b.unit_price, b.subtotal,
			b.platform_fee, b.organizer_fee, b.total_amount, b.currency,
			b.booking_status, b.payment_status, b.qr_code_url, b.tickets_sent,
			b.attendee_first_name, b.attendee_last_name, b.attendee_email,
			b.created_at, b.confirmed_at,
			e.title, e.start_time, e.venue_name, e.city, e.address, e.cover_image_url
		FROM bookings b
		JOIN events e ON b.event_id = e.id
		WHERE b.id = $1
	`

	var booking models.Booking
	var event models.Event

	err := pool.QueryRow(ctx, query, bookingID).Scan(
		&booking.ID, &booking.BookingNumber, &booking.EventID, &booking.UserID, &booking.IsGuest,
		&booking.TicketQuantity, &booking.UnitPrice, &booking.Subtotal,
		&booking.PlatformFee, &booking.OrganizerFee, &booking.TotalAmount, &booking.Currency,
		&booking.BookingStatus, &booking.PaymentStatus, &booking.QRCodeURL, &booking.TicketsSent,
		&booking.AttendeeFirstName, &booking.AttendeeLastName, &booking.AttendeeEmail,
		&booking.CreatedAt, &booking.ConfirmedAt,
		&event.Title, &event.StartTime, &event.VenueName, &event.City, &event.Address, &event.CoverImageURL,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get booking: %w", err)
	}

	booking.Event = &event
	return &booking, nil
}

func GetGuestBooking(pool *pgxpool.Pool, bookingNumber string, email string) (*models.Booking, error) {
	ctx,cancel := context.WithTimeout(context.Background(), 5 *time.Second)
	defer cancel()

	var booking models.Booking
	var event models.Event

	query := `
		SELECT
			b.id, b.booking_number, b.event_id, b.user_id, b.is_guest,
			b.ticket_quantity, b.unit_price, b.subtotal,
			b.platform_fee, b.organizer_fee, b.total_amount, b.currency,
			b.booking_status, b.payment_status, b.qr_code_url,
			b.attendee_first_name, b.attendee_last_name, b.attendee_email,
			b.created_at, b.confirmed_at,
			e.title, e.start_time
		FROM bookings b
		JOIN events e ON b.event_id = e.id
		WHERE b.booking_number = $1
		  AND lower(b.attendee_email) = lower($2)
		  AND b.is_guest = TRUE
		`

		err := pool.QueryRow(ctx, query, bookingNumber, email).Scan(
        &booking.ID, &booking.BookingNumber, &booking.EventID, &booking.UserID, &booking.IsGuest,
		&booking.TicketQuantity, &booking.UnitPrice, &booking.Subtotal,
		&booking.PlatformFee, &booking.OrganizerFee, &booking.TotalAmount, &booking.Currency,
		&booking.BookingStatus, &booking.PaymentStatus, &booking.QRCodeURL,
		&booking.AttendeeFirstName, &booking.AttendeeLastName, &booking.AttendeeEmail,
		&booking.CreatedAt, &booking.ConfirmedAt,
		&event.Title, &event.StartTime,
    )

	if err != nil {
		return nil, fmt.Errorf("booking not found: %w", err)
	}

	booking.Event = &event
	return  &booking, nil
}

func GetUserBookings(pool *pgxpool.Pool, userID string, limit int, cursor *utils.Cursor) ([]models.Booking, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{userID} 
	where := "WHERE b.user_id = $1"

	if cursor != nil {
		where += `
			AND (
				b.created_at < $2
				OR (b.created_at = $2 AND b.id < $3)
			)
		`
		args = append(args, cursor.CreatedAt, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT 
			b.id, b.booking_number, b.event_id,
			b.ticket_quantity, b.total_amount, b.currency,
			b.booking_status, b.payment_status, b.qr_code_url,
			b.created_at, b.confirmed_at,
			e.title, e.start_time, e.venue_name, e.city, e.cover_image_url
		FROM bookings b
		JOIN events e ON b.event_id = e.id
		%s
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT $%d
	`, where, limitPlaceholder)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get bookings: %w", err)
	}
	defer rows.Close()

	bookings := make([]models.Booking, 0, limit+1)

	for rows.Next() {
		var booking models.Booking
		var event models.Event

		err = rows.Scan(
			&booking.ID,
			&booking.BookingNumber,
			&booking.EventID,
			&booking.TicketQuantity,
			&booking.TotalAmount,
			&booking.Currency,
			&booking.BookingStatus,
			&booking.PaymentStatus,
			&booking.QRCodeURL,
			&booking.CreatedAt,
			&booking.ConfirmedAt,
			&event.Title,
			&event.StartTime,
			&event.VenueName,
			&event.City,
			&event.CoverImageURL,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan booking: %w", err)
		}

		booking.Event = &event
		bookings = append(bookings, booking)
	}

	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("row iteration error: %w", err)
	}

	var nextCursor *utils.Cursor
	if len(bookings) > limit {
		last := bookings[limit-1]
		nextCursor = &utils.Cursor{
			CreatedAt: last.CreatedAt,
			ID:        last.ID,
		}
		bookings = bookings[:limit]
	}

	return bookings, nextCursor, nil
}

func RequestRefund(pool *pgxpool.Pool, bookingID string, userID string, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		UPDATE bookings b
		SET refund_requested_at = CURRENT_TIMESTAMP, refund_reason = $1
		WHERE b.id = $2 AND b.user_id = $3
		AND b.booking_status = $4
		AND EXISTS (
			SELECT 1 FROM events e
			WHERE e.id = b.event_id AND e.start_time > CURRENT_TIMESTAMP
		)
	`

	cmd, err := pool.Exec(ctx, query, reason, bookingID, userID, models.BookingConfirmed)
	if err != nil {
		return fmt.Errorf("failed to request refund: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("booking not found, already refunded, or event has started")
	}
	return nil
}

func emailsMatch(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
 
// RequestRefundWithCascade updates the booking to refund-requested status and
// simultaneously cancels all ticket assignment slots. It returns the list of
// assignees who had already claimed their tickets so the caller can notify them.
func RequestRefundWithCascade(
	pool *pgxpool.Pool,
	bookingID string,
	userID string,
	reason string,
) ([]CancelledAssignee, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
 
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
 
	// ── Step 1: Update the booking (same conditions as before) ───────────────
	cmd, err := tx.Exec(ctx, `
		UPDATE bookings b
		SET refund_requested_at = CURRENT_TIMESTAMP,
		    refund_reason        = $1,
		    booking_status       = $2
		WHERE b.id        = $3
		  AND b.user_id   = $4
		  AND b.booking_status = $5
		  AND EXISTS (
		      SELECT 1 FROM events e
		      WHERE e.id = b.event_id
		        AND e.start_time > CURRENT_TIMESTAMP
		  )
	`, reason, models.BookingRefunded, bookingID, userID, models.BookingConfirmed)
 
	if err != nil {
		return nil, fmt.Errorf("update booking: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, fmt.Errorf("booking not found, already refunded, or event has already started")
	}
 
	// ── Step 2: Collect claimed assignees before wiping them ─────────────────
	// We fetch everyone who already claimed so we can email them after commit.
	// Slot 1 (the buyer) is excluded — they initiated the refund themselves.
	rows, err := tx.Query(ctx, `
		SELECT slot_number, assignee_name, assignee_email
		FROM ticket_assignments
		WHERE booking_id = $1
		  AND status     = $2
		  AND slot_number > 1
	`, bookingID, models.AssignmentClaimed)
	if err != nil {
		return nil, fmt.Errorf("fetch claimed assignees: %w", err)
	}
 
	var notifyList []CancelledAssignee
	for rows.Next() {
		var a CancelledAssignee
		var name, email *string
		if err := rows.Scan(&a.Slot, &name, &email); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan assignee: %w", err)
		}
		if name != nil {
			a.Name = *name
		}
		if email != nil {
			a.Email = *email
		}
		if a.Email != "" {
			notifyList = append(notifyList, a)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration: %w", err)
	}
 
	// ── Step 3: Cancel ALL assignment slots for this booking ─────────────────
	// This covers unassigned, pending, and claimed in one sweep.
	// Slot 1 (buyer) is included — their QR is also now invalid.
	_, err = tx.Exec(ctx, `
		UPDATE ticket_assignments
		SET status     = 'cancelled',
		    claim_token = NULL,
		    expires_at  = NULL
		WHERE booking_id = $1
		  AND status    != 'cancelled'
	`, bookingID)
	if err != nil {
		return nil, fmt.Errorf("cancel assignments: %w", err)
	}
 
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
 
	return notifyList, nil
}