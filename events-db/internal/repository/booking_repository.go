package repository

import (
	"context"
	"crypto/subtle"
	"errors"
	"events-db/internal/models"
	"events-db/internal/utils"
	"fmt"
	"math"
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
	serviceFee, commission := CalculateFees(subtotal, quantity)
	platformFee := serviceFee  // buyer-paid service fee (R10 × qty)
	organizerFee := commission // organizer-paid commission (6% × subtotal)
	discountAmount := 0.0
	// Buyer is charged the ticket price plus the service fee only. The 6%
	// commission is NOT added here — it comes out of the organizer's payout.
	totalAmount := subtotal + platformFee - discountAmount

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

	booking.EventID = &eventID
	booking.UserID = userID
	booking.IsGuest = isGuest
	booking.TicketQuantity = quantity
	booking.UnitPrice = unitPrice
	booking.Subtotal = subtotal
	booking.PlatformFee = platformFee
	booking.OrganizerFee = organizerFee
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

// ── Fee model ────────────────────────────────────────────────────────────────
//
// Per ticket the platform takes two distinct fees with two distinct bearers:
//
//   • Service fee — R10.00 PER TICKET, paid by the BUYER, added on top of the
//     ticket price (so it lands in total_amount). To avoid gouging cheap
//     tickets, the per-ticket fee is CAPPED at MaxServiceFeeRate of the ticket's
//     face value. The cap only ever bites below R50 (20% × R50 = R10), so any
//     ticket R50+ pays the full flat R10. Stored in the `platform_fee` column.
//
//        R200 → R10 (5%)    R50 → R10 (20%)    R30 → R6 (20%)    R10 → R2 (20%)
//
//   • Commission — 6% of the ticket subtotal, borne by the ORGANIZER. A
//     percentage already scales with price, so it needs no cap. It is NOT added
//     to total_amount (the buyer never pays it); instead it is deducted from the
//     organizer's payout. Stored in the `organizer_fee` column.
//
// So:  buyer pays      = subtotal + service_fee
//      organizer nets  = subtotal − commission
//      platform earns  = service_fee + commission
const (
	ServiceFeePerTicket     = 10.00 // ZAR, buyer-paid, charged once per ticket (before the cap)
	MaxServiceFeeRate       = 0.20  // service fee never exceeds this share of a ticket's face value
	OrganizerCommissionRate = 0.06  // organizer-paid, fraction of the ticket subtotal
)

// round2 rounds a money amount to whole cents (2 decimal places).
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// CalculateFees returns the buyer-paid service fee and the organizer-paid
// commission for a single booking line of `quantity` tickets totalling
// `subtotal`. The per-ticket service fee is the flat ServiceFeePerTicket, but
// never more than MaxServiceFeeRate of the ticket's face value — so a R50 ticket
// pays the full R10 while a R20 ticket pays R4 (20%) instead. Free lines
// (subtotal == 0) incur neither fee.
func CalculateFees(subtotal float64, quantity int) (serviceFee, commission float64) {
	if subtotal <= 0 || quantity <= 0 {
		return 0, 0
	}
	unitPrice := subtotal / float64(quantity)
	perTicket := math.Min(ServiceFeePerTicket, MaxServiceFeeRate*unitPrice)
	serviceFee = round2(perTicket * float64(quantity))
	commission = round2(OrganizerCommissionRate * subtotal)
	return serviceFee, commission
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
		LEFT JOIN events e ON b.event_id = e.id
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
			e.title, e.start_time, e.venue_name, e.city, e.address, e.cover_image_url
		FROM bookings b
		LEFT JOIN events e ON b.event_id = e.id
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
		&event.Title, &event.StartTime, &event.VenueName, &event.City, &event.Address, &event.CoverImageURL,
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
		LEFT JOIN events e ON b.event_id = e.id
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
// CreateMerchOnlyBooking inserts a placeholder booking row for a cart that
// contains only merchandise items (no ticket items). The booking has a NULL
// event_id, zero ticket quantity, and booking_type = 'merch_only' so that
// downstream workers can distinguish it from regular ticket bookings.
func CreateMerchOnlyBooking(
	pool *pgxpool.Pool,
	cartID string,
	userID, sessionID *string,
	total float64,
	currency string,
) (*models.Booking, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var booking models.Booking
	err := pool.QueryRow(ctx, `
		INSERT INTO bookings (
			event_id, user_id, session_id,
			is_guest, ticket_quantity, unit_price, subtotal,
			platform_fee, organizer_fee, discount_amount,
			total_amount, currency,
			booking_status, payment_status,
			booking_type
		)
		VALUES (
			NULL,    -- no event for merch-only
			$1, $2,
			$3, 0, 0, 0,
			0, 0, 0,
			$4, $5,
			$6, $7,
			'merch_only'
		)
		RETURNING id, booking_number, created_at
	`,
		userID, sessionID,
		userID == nil, // is_guest = true when there is no user_id
		total, currency,
		models.BookingPending, models.PaymentPending,
	).Scan(&booking.ID, &booking.BookingNumber, &booking.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create merch-only booking: %w", err)
	}

	booking.TotalAmount = total
	booking.Currency = currency
	return &booking, nil
}