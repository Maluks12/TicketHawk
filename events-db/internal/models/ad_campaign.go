package models

import "time"

// ── Campaign status ───────────────────────────────────────────────────────────

type CampaignStatus string

const (
	CampaignDraft     CampaignStatus = "draft"      // created, wallet too low to activate
	CampaignActive    CampaignStatus = "active"      // running and serving impressions
	CampaignPaused    CampaignStatus = "paused"      // manually paused by organiser
	CampaignExhausted CampaignStatus = "exhausted"   // daily budget hit; auto-resumes at midnight
	CampaignEnded     CampaignStatus = "ended"       // end_date passed or manually ended
)

// ── Top-up status ─────────────────────────────────────────────────────────────

type TopupStatus string

const (
	TopupPending   TopupStatus = "pending"
	TopupCompleted TopupStatus = "completed"
	TopupFailed    TopupStatus = "failed"
)

// ── Ad Wallet ─────────────────────────────────────────────────────────────────
// One wallet per organiser. All campaigns for that organiser draw from the
// same balance.

type AdWallet struct {
	ID          string    `json:"id" db:"id"`
	OrganizerID string    `json:"organizer_id" db:"organizer_id"`
	Balance     float64   `json:"balance" db:"balance"`            // current spendable ZAR
	TotalTopups float64   `json:"total_topups" db:"total_topups"`  // lifetime deposits
	TotalSpent  float64   `json:"total_spent" db:"total_spent"`    // lifetime spend
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// ── Wallet Top-up ─────────────────────────────────────────────────────────────

type AdWalletTopup struct {
	ID                     string      `json:"id" db:"id"`
	OrganizerID            string      `json:"organizer_id" db:"organizer_id"`
	Amount                 float64     `json:"amount" db:"amount"`
	StitchPaymentRequestID *string     `json:"stitch_payment_request_id,omitempty" db:"stitch_payment_request_id"`
	ReferenceNumber        string      `json:"reference_number" db:"reference_number"` // e.g. TUP-00042
	Status                 TopupStatus `json:"status" db:"status"`
	CreatedAt              time.Time   `json:"created_at" db:"created_at"`
	CompletedAt            *time.Time  `json:"completed_at,omitempty" db:"completed_at"`
}

// ── Ad Campaign ───────────────────────────────────────────────────────────────

type AdCampaign struct {
	ID          string         `json:"id" db:"id"`
	EventID     string         `json:"event_id" db:"event_id"`
	OrganizerID string         `json:"organizer_id" db:"organizer_id"`

	// Spending controls
	DailyBudget float64 `json:"daily_budget" db:"daily_budget"` // ZAR cap per calendar day (min R50)
	CPMRate     float64 `json:"cpm_rate" db:"cpm_rate"`         // ZAR per 1 000 impressions

	// Lifetime totals — updated incrementally on each impression
	TotalSpent       float64 `json:"total_spent" db:"total_spent"`
	TotalImpressions int64   `json:"total_impressions" db:"total_impressions"`
	TotalClicks      int64   `json:"total_clicks" db:"total_clicks"`

	// Today's numbers — joined from ad_daily_spend at query time, not stored on
	// the campaigns row itself, so db:"-" is correct here.
	TodaySpent       float64 `json:"today_spent" db:"-"`
	TodayImpressions int64   `json:"today_impressions" db:"-"`
	TodayClicks      int64   `json:"today_clicks" db:"-"`

	StartDate time.Time  `json:"start_date" db:"start_date"`
	EndDate   *time.Time `json:"end_date,omitempty" db:"end_date"`

	Status    CampaignStatus `json:"status" db:"status"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`

	// Populated on demand via JOIN — never a stored column.
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
	// Minimum R50/day. Higher budget = higher ad rank = more impressions per day.
	DailyBudget float64    `json:"daily_budget" binding:"required,min=50"`
	StartDate   *time.Time `json:"start_date"`  // defaults to today if nil
	EndDate     *time.Time `json:"end_date"`    // nil = run until wallet is empty
}

type UpdateCampaignDTO struct {
	DailyBudget *float64        `json:"daily_budget" binding:"omitempty,min=50"`
	EndDate     *time.Time      `json:"end_date"`
	// Allowed transitions: active ↔ paused only. exhausted → active is automatic at midnight.
	Status      *CampaignStatus `json:"status"`
}

type TopupWalletDTO struct {
	// Minimum R50 top-up.
	Amount float64 `json:"amount" binding:"required,min=50"`
}