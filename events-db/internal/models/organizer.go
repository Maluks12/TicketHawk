package models

import "time"

// OrganizerStatus reflects account standing, NOT a registration queue.
// Accounts are auto-approved on self-registration (Meta Business style).
type OrganizerStatus string

const (
	OrganizerActive    OrganizerStatus = "active"    // set automatically on self-registration
	OrganizerSuspended OrganizerStatus = "suspended" // temporarily blocked by admin
	OrganizerBanned    OrganizerStatus = "banned"    // permanently removed by admin

	// Legacy DB values - kept so existing rows don't break. Do not write for new accounts.
	OrganizerPending  OrganizerStatus = "pending"
	OrganizerApproved OrganizerStatus = "approved"
	OrganizerRejected OrganizerStatus = "rejected"
)

type Organizer struct {
	ID               string          `json:"id" db:"id"`
	UserID           string          `json:"user_id" db:"user_id"`
	BusinessName     string          `json:"business_name" db:"business_name"`
	BusinessEmail    string          `json:"business_email" db:"business_email"`
	BusinessPhone    string          `json:"business_phone" db:"business_phone"`
	TaxID            *string         `json:"tax_id,omitempty" db:"tax_id"`
	WebsiteURL       *string         `json:"website_url,omitempty" db:"website_url"`
	Description      *string         `json:"description,omitempty" db:"description"`
	LogoURL          *string         `json:"logo_url,omitempty" db:"logo_url"`
	Verified         bool            `json:"verified" db:"verified"`
	VerifiedAt       *time.Time      `json:"verified_at,omitempty" db:"verified_at"`
	VerifiedBy       *string         `json:"verified_by,omitempty" db:"verified_by"`
	CommissionRate   float64         `json:"commission_rate" db:"commission"`
	StripeAccountID  *string         `json:"-" db:"stripe_account_id"`
	PayoutEnabled    bool            `json:"payout_enabled" db:"payout_enabled"`
	BankAccountLast4 *string         `json:"bank_account_last_4,omitempty" db:"bank_account_last_4"`
	Status           OrganizerStatus `json:"status" db:"status"`
	RejectionReason  *string         `json:"rejection_reason,omitempty" db:"rejection_reason"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}

// CreateOrganizerDTO — user self-registers as an organiser with no admin gate.
type CreateOrganizerDTO struct {
	BusinessName  string  `json:"business_name" binding:"required,min=2,max=255"`
	BusinessEmail string  `json:"business_email" binding:"required,email"`
	BusinessPhone string  `json:"business_phone" binding:"required"`
	WebsiteURL    *string `json:"website_url" binding:"omitempty,url"`
	Description   string  `json:"description" binding:"required,min=50,max=2000"`
}

type UpdateOrganizerDTO struct {
	BusinessName  *string `json:"business_name" binding:"required,min=2,max=255"`
	BusinessEmail *string `json:"business_email" binding:"required,email"`
	BusinessPhone *string `json:"business_phone" binding:"required"`
	WebsiteURL    *string `json:"website_url" binding:"omitempty,url"`
	Description   *string `json:"description" binding:"required,min=50,max=2000"`
	LogoURL       *string `json:"logo_url"`
}

// AdminUpdateOrganizerStatusDTO — admin suspends or bans an organiser.
// No "approve" action needed — registration is self-service.
type AdminUpdateOrganizerStatusDTO struct {
	Status OrganizerStatus `json:"status" binding:"required"`
	Reason string          `json:"reason" binding:"required,min=10"`
}