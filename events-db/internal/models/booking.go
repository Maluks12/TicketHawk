package models

import "time"

type BookingStatus string
type PaymentStatus string

const (
	BookingPending   BookingStatus = "pending"
	BookingConfirmed BookingStatus = "confirmed"
	BookingCancelled BookingStatus = "cancelled"
	BookingRefunded  BookingStatus = "refunded"
)

const (
	PaymentPending    PaymentStatus = "pending"
	PaymentProcessing PaymentStatus = "processing"
	PaymentCompleted  PaymentStatus = "completed"
	PaymentFailed     PaymentStatus = "failed"
	PaymentRefunded   PaymentStatus = "refunded"
)

type Booking struct {
	ID                string        `json:"id" db:"id"`
	BookingNumber     string        `json:"booking_number" db:"booking_number"`
	// EventID is nil for merch-only bookings that are not linked to any event.
	EventID           *string       `json:"event_id,omitempty" db:"event_id"`
	UserID            *string       `json:"user_id,omitempty" db:"user_id"`
	// SessionID identifies guest buyers on merch-only bookings.
	SessionID         *string       `json:"session_id,omitempty" db:"session_id"`
	IsGuest           bool          `json:"is_guest" db:"is_guest"`
	// BookingType distinguishes regular ticket bookings from merch-only ones.
	BookingType       string        `json:"booking_type" db:"booking_type"`
	// TicketTypeID links to the tier that was booked. Nil for flat-priced events.
	TicketTypeID      *string       `json:"ticket_type_id,omitempty" db:"ticket_type_id"`
	// TicketTypeName is a snapshot of the type name at booking time, so receipts
	// and emails show the correct tier even if the organiser later renames it.
	TicketTypeName    *string       `json:"ticket_type_name,omitempty" db:"ticket_type_name"`
	TicketQuantity    int           `json:"ticket_quantity" db:"ticket_quantity"`
	UnitPrice         float64       `json:"unit_price" db:"unit_price"`
	Subtotal          float64       `json:"subtotal" db:"subtotal"`
	PlatformFee       float64       `json:"platform_fee" db:"platform_fee"`
	OrganizerFee      float64       `json:"organizer_fee" db:"organizer_fee"`
	TotalAmount       float64       `json:"total_amount" db:"total_amount"`
	Currency          string        `json:"currency" db:"currency"`
	AttendeeFirstName *string       `json:"attendee_first_name,omitempty" db:"attendee_first_name"`
	AttendeeLastName  *string       `json:"attendee_last_name,omitempty" db:"attendee_last_name"`
	AttendeeEmail     *string       `json:"attendee_email,omitempty" db:"attendee_email"`
	AttendeePhone     *string       `json:"attendee_phone,omitempty" db:"attendee_phone"`
	BookingStatus     BookingStatus `json:"booking_status" db:"booking_status"`
	PaymentStatus     PaymentStatus `json:"payment_status" db:"payment_status"`
	TicketsSent       bool          `json:"tickets_sent" db:"tickets_sent"`
	QRCodeData        *string       `json:"-" db:"qr_code_data"`
	QRCodeURL         *string       `json:"qr_code_url" db:"qr_code_url"`
	RefundedRequestAt *time.Time    `json:"refunded_request_at,omitempty" db:"refunded_request_at"`
	RefundedReason    *string       `json:"refunded_reason,omitempty" db:"refunded_reason"`
	RefundedAt        *time.Time    `json:"refunded_at,omitempty" db:"refunded_at"`
	RefundedAmount    *float64      `json:"refunded_amount,omitempty" db:"refunded_amount"`
	BookingNotes      *string       `json:"booking_notes,omitempty" db:"booking_notes"`
	SpecialRequirements *string     `json:"special_requirements,omitempty" db:"special_requirements"`
	PromoCode         *string       `json:"promo_code,omitempty" db:"promo_code"`
	DiscountAmount    float64       `json:"discount_amount" db:"discount_amount"`
	CreatedAt         time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at" db:"updated_at"`
	ConfirmedAt       *time.Time    `json:"confirmed_at,omitempty" db:"confirmed_at"`
	CancelledAt       *time.Time    `json:"cancelled_at,omitempty" db:"cancelled_at"`

	Event *Event       `json:"event,omitempty"`
	User  *UserProfile `json:"user,omitempty"`
}

func (b *Booking) GuestEmail() string {
	if b.AttendeeEmail != nil {
		return *b.AttendeeEmail
	}
	return ""
}

func (b *Booking) GuestName() string {
	if b.AttendeeFirstName != nil && b.AttendeeLastName != nil {
		return *b.AttendeeFirstName + " " + *b.AttendeeLastName
	}
	if b.AttendeeFirstName != nil {
		return *b.AttendeeFirstName
	}
	return "Guest"
}

type CreateBookingDTO struct {
	EventID             string  `json:"event_id" binding:"required,uuid"`
	TicketQuantity      int     `json:"ticket_quantity" binding:"required,min=1,max=10"`
	AttendeeFirstName   *string `json:"attendee_first_name"`
	AttendeeLastName    *string `json:"attendee_last_name"`
	AttendeeEmail       *string `json:"attendee_email" binding:"omitempty,email"`
	AttendeePhone       *string `json:"attendee_phone"`
	SpecialRequirements *string `json:"special_requirements"`
	PromoCode           *string `json:"promo_code"`
}

type CreateGuestBookingDTO struct {
	EventID             string  `json:"event_id" binding:"required,uuid"`
	TicketQuantity      int     `json:"ticket_quantity" binding:"required,min=1,max=10"`
	AttendeeFirstName   string  `json:"attendee_first_name" binding:"required,max=100"`
	AttendeeLastName    string  `json:"attendee_last_name" binding:"max=100"`
	AttendeeEmail       string  `json:"attendee_email" binding:"required,email,max=255"`
	AttendeePhone       *string `json:"attendee_phone"`
	SpecialRequirements *string `json:"special_requirements"`
	PromoCode           *string `json:"promo_code"`
}

type BookingResponse struct {
	BookingID        string    `json:"booking_id"`
	BookingNumber    string    `json:"booking_number"`
	TotalAmount      float64   `json:"total_amount"`
	Currency         string    `json:"currency"`
	PaymentRequestID string    `json:"payment_request_id"`
	ClientToken      string    `json:"client_token"`
	ExpiresAt        time.Time `json:"expires_at"`
	IsGuest          bool      `json:"is_guest"`
}

func (b *Booking) QRCodeURLOrEmpty() string {
	if b.QRCodeURL == nil {
		return ""
	}
	return *b.QRCodeURL
}