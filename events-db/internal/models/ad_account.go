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

	// Card payment details — display metadata only (no raw numbers stored).
	CardName   *string `json:"card_name,omitempty"   db:"card_name"`
	CardLast4  *string `json:"card_last4,omitempty"  db:"card_last4"`
	CardBrand  *string `json:"card_brand,omitempty"  db:"card_brand"`
	CardExpiry *string `json:"card_expiry,omitempty" db:"card_expiry"`

	// Billing contact email.
	AdPaymentEmail *string `json:"ad_payment_email,omitempty" db:"ad_payment_email"`

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

// RegisterAdAccountPaymentMethodDTO stores card display metadata for an ad account.
// Raw card numbers and CVV are never sent here — only the safe display fields.
type RegisterAdAccountPaymentMethodDTO struct {
	CardName   string `json:"card_name"   binding:"required,min=2,max=100"`
	CardLast4  string `json:"card_last4"  binding:"required,len=4"`
	CardBrand  string `json:"card_brand"  binding:"required"`
	CardExpiry string `json:"card_expiry" binding:"required,len=5"` // MM/YY
	Email      string `json:"email"       binding:"required,email"`
}