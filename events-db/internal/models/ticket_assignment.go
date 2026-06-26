package models

import "time"

type AssignmentStatus string

const (
	AssignmentUnassigned AssignmentStatus = "unassigned"
	AssignmentPending    AssignmentStatus = "pending"  // invite sent, not yet claimed
	AssignmentClaimed    AssignmentStatus = "claimed"
	AssignmentRevoked    AssignmentStatus = "revoked"
	AssignmentCancelled  AssignmentStatus = "cancelled"
)

type TicketAssignment struct {
	ID         string           `json:"id"          db:"id"`
	BookingID  string           `json:"booking_id"  db:"booking_id"`
	SlotNumber int              `json:"slot_number" db:"slot_number"`
	Status     AssignmentStatus `json:"status"      db:"status"`

	AssigneeName  *string `json:"assignee_name,omitempty"  db:"assignee_name"`
	AssigneeEmail *string `json:"assignee_email,omitempty" db:"assignee_email"`

	// ClaimToken is the secret sent in the claim email — never serialised to JSON.
	ClaimToken *string `json:"-" db:"claim_token"`

	AssignedAt *time.Time `json:"assigned_at,omitempty" db:"assigned_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"  db:"expires_at"`
	ClaimedAt  *time.Time `json:"claimed_at,omitempty"  db:"claimed_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"  db:"revoked_at"`

	// QRScannedAt is set once this ticket has been admitted at the gate. Exposed
	// so the app can tell an assignable pool ticket from one already walked in.
	QRScannedAt *time.Time `json:"qr_scanned_at,omitempty" db:"qr_scanned_at"`

	// Per-ticket QR — data is never serialised; URL is.
	QRCodeData *string `json:"-" db:"qr_code_data"`
	QRCodeURL  *string `json:"qr_code_url,omitempty" db:"qr_code_url"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// ── DTOs ──────────────────────────────────────────────────────────────────────

// AssignTicketDTO is sent by the buyer when assigning a slot to someone.
type AssignTicketDTO struct {
	AssigneeEmail string `json:"assignee_email" binding:"required,email,max=255"`
	AssigneeName  string `json:"assignee_name"  binding:"required,min=1,max=200"`
}