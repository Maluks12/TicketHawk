package models

import "time"

// ── Campaign status ───────────────────────────────────────────────────────────

type CampaignStatus string

const (
	CampaignActive        CampaignStatus = "active"         // running and serving impressions
	CampaignPaused        CampaignStatus = "paused"         // manually paused by organiser
	CampaignExhausted     CampaignStatus = "exhausted"      // daily budget hit; auto-resumes at midnight
	CampaignPaymentFailed CampaignStatus = "payment_failed" // end-of-day billing failed; paused until resolved
	CampaignEnded         CampaignStatus = "ended"          // end_date passed or manually ended
)

// ── Billing ───────────────────────────────────────────────────────────────────
// Post-pay model: campaigns run freely during the day; at midnight the nightly
// worker sums each organiser's spend and debits via their stored Stitch mandate.

type BillingStatus string

const (
	BillingPending   BillingStatus = "pending"
	BillingCompleted BillingStatus = "completed"
	BillingFailed    BillingStatus = "failed"
	BillingWaived    BillingStatus = "waived" // admin can waive micro-charges (e.g. < R1)
)

// AdBillingRecord is one end-of-day charge per organiser.
type AdBillingRecord struct {
	ID                     string        `json:"id" db:"id"`
	OrganizerID            string        `json:"organizer_id" db:"organizer_id"`
	BillingDate            time.Time     `json:"billing_date" db:"billing_date"`
	Amount                 float64       `json:"amount" db:"amount"`
	StitchPaymentRequestID *string       `json:"stitch_payment_request_id,omitempty" db:"stitch_payment_request_id"`
	Status                 BillingStatus `json:"status" db:"status"`
	FailureReason          *string       `json:"failure_reason,omitempty" db:"failure_reason"`
	CreatedAt              time.Time     `json:"created_at" db:"created_at"`
	CompletedAt            *time.Time    `json:"completed_at,omitempty" db:"completed_at"`
}

// ── Ad Campaign ───────────────────────────────────────────────────────────────

type AdCampaign struct {
	ID          string  `json:"id"           db:"id"`
	EventID     string  `json:"event_id"     db:"event_id"`
	OrganizerID string  `json:"organizer_id" db:"organizer_id"`
	AdAccountID *string `json:"ad_account_id,omitempty" db:"ad_account_id"` // NEW
 
	DailyBudget float64 `json:"daily_budget" db:"daily_budget"`
	CPMRate     float64 `json:"cpm_rate"     db:"cpm_rate"`
 
	TotalSpent       float64 `json:"total_spent"        db:"total_spent"`
	TotalImpressions int64   `json:"total_impressions"  db:"total_impressions"`
	TotalClicks      int64   `json:"total_clicks"       db:"total_clicks"`
 
	TodaySpent       float64 `json:"today_spent"        db:"-"`
	TodayImpressions int64   `json:"today_impressions"  db:"-"`
	TodayClicks      int64   `json:"today_clicks"       db:"-"`
 
	StartDate time.Time  `json:"start_date"         db:"start_date"`
	EndDate   *time.Time `json:"end_date,omitempty" db:"end_date"`
 
	Status    CampaignStatus `json:"status"     db:"status"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
 
	Event *Event `json:"event,omitempty" db:"-"`
}

// CostPerImpression returns the ZAR cost charged per individual impression.
func (c *AdCampaign) CostPerImpression() float64 {
	return c.CPMRate / 1000.0
}

// DailyImpressionCap returns how many impressions this campaign can serve
// today before exhausting its daily budget.
func (c *AdCampaign) DailyImpressionCap() int64 {
	if c.CostPerImpression() == 0 {
		return 0
	}
	return int64(c.DailyBudget / c.CostPerImpression())
}

// ── DTOs ──────────────────────────────────────────────────────────────────────

type CreateCampaignDTO struct {
	AdAccountID string     `json:"ad_account_id" binding:"required"` // NEW — which account to bill
	DailyBudget float64    `json:"daily_budget"  binding:"required,min=200"`
	StartDate   *time.Time `json:"start_date"`
	EndDate     *time.Time `json:"end_date"`
}

type UpdateCampaignDTO struct {
	DailyBudget *float64 `json:"daily_budget" binding:"omitempty,min=200"`
	EndDate     *time.Time `json:"end_date"`
	// Allowed organiser transitions: active <-> paused only.
	// exhausted -> active happens automatically at midnight.
	// payment_failed -> active happens automatically after billing succeeds.
	Status *CampaignStatus `json:"status"`
}

// RegisterPaymentMethodDTO stores the organiser's Stitch debit mandate.
// Obtained by the frontend via the Stitch mandate OAuth flow, then sent here once.
// All future end-of-day ad bills are debited against this mandate automatically.
type RegisterPaymentMethodDTO struct {
	StitchMandateID string `json:"stitch_mandate_id" binding:"required"`
	Email           string `json:"email" binding:"required,email"`
}