package models

import "time"

type TicketReservation struct {
	ID           string    `json:"id" db:"id"`
	CartID       string    `json:"cart_id" db:"cart_id"`
	EventID      string    `json:"event_id" db:"event_id"`
	UserID       *string   `json:"user_id,omitempty" db:"user_id"`
	SessionID    *string   `json:"session_id,omitempty" db:"session_id"`
	BookingNumber string   `json:"booking_number" db:"booking_number"`
	Quantity     int       `json:"quantity" db:"quantity"`
	// TicketTypeID is set when the buyer chose a specific pricing tier.
	TicketTypeID *string   `json:"ticket_type_id,omitempty" db:"ticket_type_id"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// SecondsRemaining returns the number of seconds left on the reservation.
// Returns 0 if already expired.
func (r *TicketReservation) SecondsRemaining() int {
	remaining := time.Until(r.ExpiresAt).Seconds()
	if remaining < 0 {
		return 0
	}
	return int(remaining)
}

// Response types — no db tags needed, these are never scanned from the DB

// CheckoutBeginResponse is returned by POST /checkout/begin.
type CheckoutBeginResponse struct {
	Reservations []ReservationSummary `json:"reservations"`
	SecondsLeft  int                  `json:"seconds_left"` // shortest TTL across all reservations
	CartTotal    float64              `json:"cart_total"`
	Currency     string               `json:"currency"`
}

// ReservationSummary is one entry inside CheckoutBeginResponse.
type ReservationSummary struct {
	ReservationID  string  `json:"reservation_id"`
	EventID        string  `json:"event_id"`
	EventTitle     string  `json:"event_title"`
	TicketTypeID   *string `json:"ticket_type_id,omitempty"`
	TicketTypeName *string `json:"ticket_type_name,omitempty"`
	Quantity       int     `json:"quantity"`
	SecondsLeft    int     `json:"seconds_left"`
}

// HeartbeatResponse is returned by POST /checkout/heartbeat.
type HeartbeatResponse struct {
	SecondsLeft int    `json:"seconds_left"`
	Status      string `json:"status"` // "ok" or "expired"
}