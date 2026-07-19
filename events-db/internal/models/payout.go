package models

import "time"

// PayoutTier controls how an organizer gets paid.
//
//	held    — default. Platform collects everything; a transfers-based worker
//	          settles (subtotal − commission) after the event completes.
//	instant — trusted organizers. Transactions carry a Paystack subaccount
//	          split; Paystack settles them directly as tickets sell, minus the
//	          platform cut and a rolling reserve released post-event.
type PayoutTier string

const (
	PayoutTierHeld    PayoutTier = "held"
	PayoutTierInstant PayoutTier = "instant"
)

// ── Instant-tier eligibility policy ──────────────────────────────────────────
// The system computes eligibility; an admin makes the final call. Tune freely.
const (
	// InstantTierMinCompletedEvents — paid events that reached 'completed'
	// with at least one confirmed booking AND at least one real scan.
	// One event is too cheap to stage; two matches where Howler draws
	// its "trusted organiser with a history" line.
	InstantTierMinCompletedEvents = 2

	// InstantTierMinGrossZAR — lifetime confirmed ticket subtotal, so two
	// trivially small events don't qualify.
	InstantTierMinGrossZAR = 5000.00

	// InstantTierMinAccountAgeDays — days since organizer registration.
	InstantTierMinAccountAgeDays = 30
)

// ── Payout policy ────────────────────────────────────────────────────────────
const (
	// InstantReserveRate — fraction of the ticket subtotal withheld from every
	// instant-tier split and released after the event completes (Howler holds
	// 10% the same way). Taken via an inflated transaction_charge, so it lands
	// in the platform account without any extra Paystack machinery.
	InstantReserveRate = 0.10

	// PayoutDelayAfterEventHours — grace period between an event completing
	// and its payout rows being created. Buys a dispute/refund window before
	// money leaves the platform account. Quicket pays 2–3 business days after
	// the event; 48h keeps us competitive while retaining a buffer.
	PayoutDelayAfterEventHours = 48
)

// OrganizerPayout is one row of the payout ledger — either a tier-1 event
// settlement or a tier-2 reserve release.
type OrganizerPayout struct {
	ID                   string     `json:"id" db:"id"`
	OrganizerID          string     `json:"organizer_id" db:"organizer_id"`
	EventID              string     `json:"event_id" db:"event_id"`
	EventTitle           string     `json:"event_title,omitempty" db:"-"`
	Kind                 string     `json:"kind" db:"kind"` // event_settlement | reserve_release
	Amount               float64    `json:"amount" db:"amount"`
	Currency             string     `json:"currency" db:"currency"`
	Status               string     `json:"status" db:"status"` // pending | processing | paid | failed
	TransferReference    *string    `json:"transfer_reference,omitempty" db:"transfer_reference"`
	PaystackTransferCode *string    `json:"paystack_transfer_code,omitempty" db:"paystack_transfer_code"`
	FailureReason        *string    `json:"failure_reason,omitempty" db:"failure_reason"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
	PaidAt               *time.Time `json:"paid_at,omitempty" db:"paid_at"`
}

// PayoutProfile is the organizer-facing (and admin-facing) view of payout setup.
type PayoutProfile struct {
	OrganizerID        string     `json:"organizer_id"`
	BusinessName       string     `json:"business_name"`
	PayoutTier         PayoutTier `json:"payout_tier"`
	PayoutEnabled      bool       `json:"payout_enabled"`
	HasSubaccount      bool       `json:"has_subaccount"`
	SubaccountCode     *string    `json:"subaccount_code,omitempty"` // admin views only
	SettlementBankName *string    `json:"settlement_bank_name,omitempty"`
	BankAccountName    *string    `json:"bank_account_name,omitempty"`
	BankAccountLast4   *string    `json:"bank_account_last_4,omitempty"`
	TierUpgradedAt     *time.Time `json:"tier_upgraded_at,omitempty"`
}

// InstantTierEligibility reports where an organizer stands against the
// promotion policy. Surfaced to admins next to the promote button, and to
// organizers as a progress checklist.
type InstantTierEligibility struct {
	CompletedPaidEvents int     `json:"completed_paid_events"`
	RequiredEvents      int     `json:"required_events"`
	GrossProcessed      float64 `json:"gross_processed"`
	RequiredGross       float64 `json:"required_gross"`
	AccountAgeDays      int     `json:"account_age_days"`
	RequiredAgeDays     int     `json:"required_age_days"`
	OpenRefundRequests  int     `json:"open_refund_requests"`
	HasVerifiedBank     bool    `json:"has_verified_bank"`
	Eligible            bool    `json:"eligible"`
}

// SetPayoutDetailsDTO — organizer (or admin on their behalf) submits bank details.
type SetPayoutDetailsDTO struct {
	BankCode      string `json:"bank_code" binding:"required"`
	BankName      string `json:"bank_name" binding:"required,max=100"`
	AccountNumber string `json:"account_number" binding:"required,min=6,max=20"`
	AccountName   string `json:"account_name" binding:"required,min=2,max=255"`
	// AccountType is used by the bank-ownership check ("Savings"/"Current"/…).
	// Optional; defaults to "Savings" when omitted.
	AccountType   string `json:"account_type" binding:"omitempty,max=20"`
}

// AdminSetTierDTO — admin promotes/demotes an organizer's payout tier.
type AdminSetTierDTO struct {
	Tier PayoutTier `json:"tier" binding:"required"`
}