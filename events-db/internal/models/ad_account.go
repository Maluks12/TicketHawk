package models

import "time"

type AdAccountStatus string

const (
	AdAccountActive   AdAccountStatus = "active"
	AdAccountDisabled AdAccountStatus = "disabled"
)

// AdAccount is a billing container that owns campaigns and a payment method.
// An organizer can have multiple ad accounts (e.g. one per brand or event
// portfolio). Mirrors Meta's per-account billing model.
type AdAccount struct {
	ID          string          `json:"id"           db:"id"`
	OrganizerID string          `json:"organizer_id" db:"organizer_id"`
	Name        string          `json:"name"         db:"name"`
	Status      AdAccountStatus `json:"status"       db:"status"`

	// Payment method — one Stitch mandate per account.
	// Campaigns under this account are billed against this mandate nightly.
	StitchMandateID     *string `json:"-"                              db:"stitch_mandate_id"`
	StitchMandateStatus string  `json:"stitch_mandate_status"          db:"stitch_mandate_status"`
	AdPaymentEmail      *string `json:"ad_payment_email,omitempty"     db:"ad_payment_email"`

	// Denormalised at query time — not stored.
	HasPaymentMethod bool    `json:"has_payment_method" db:"-"`
	TotalSpent       float64 `json:"total_spent"        db:"-"`
	ActiveCampaigns  int     `json:"active_campaigns"   db:"-"`

	Currency  string    `json:"currency"    db:"currency"`
	CreatedAt time.Time `json:"created_at"  db:"created_at"`
	UpdatedAt time.Time `json:"updated_at"  db:"updated_at"`
}

// ── DTOs ─────────────────────────────────────────────────────────────────────

type CreateAdAccountDTO struct {
	Name string `json:"name" binding:"required,min=2,max=100"`
}

type UpdateAdAccountDTO struct {
	Name   *string          `json:"name"   binding:"omitempty,min=2,max=100"`
	Status *AdAccountStatus `json:"status" binding:"omitempty"`
}

// RegisterAdAccountPaymentMethodDTO stores a Stitch mandate against a specific
// ad account. Replaces the old organizer-level RegisterPaymentMethodDTO for
// the ad-accounts flow.
type RegisterAdAccountPaymentMethodDTO struct {
	StitchMandateID string `json:"stitch_mandate_id" binding:"required"`
	Email           string `json:"email"             binding:"required,email"`
}