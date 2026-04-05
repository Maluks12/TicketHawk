package models

import "time"

// ── Enums ─────────────────────────────────────────────────────────────────────

type CampaignStatus string

const (
	CampaignDraft     CampaignStatus = "draft"
	CampaignActive    CampaignStatus = "active"
	CampaignPaused    CampaignStatus = "paused"
	CampaignExhausted CampaignStatus = "exhausted" // daily budget hit; auto-resumes at midnight
	CampaignEnded     CampaignStatus = "ended"     // end_date passed or manually ended
)

type TopupStatus string

const (
	TopupPending   TopupStatus = "pending"
	TopupCompleted TopupStatus = "completed"
	TopupFailed    TopupStatus = "failed"
)

// ── Ad Wallet ─────────────────────────────────────────────────────────────────

type AdWallet struct {
	ID          string    `json:"id"`
	OrganizerID string    `json:"organizer_id"`
	Balance     float64   `json:"balance"`
	TotalTopups float64   `json:"total_topups"`
	TotalSpent  float64   `json:"total_spent"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ── Wallet Top-up ─────────────────────────────────────────────────────────────

type AdWalletTopup struct {
	ID                     string      `json:"id"`
	OrganizerID            string      `json:"organizer_id"`
	Amount                 float64     `json:"amount"`
	StitchPaymentRequestID *string     `json:"stitch_payment_request_id,omitempty"`
	ReferenceNumber        string      `json:"reference_number"`
	Status                 TopupStatus `json:"status"`
	CreatedAt              time.Time   `json:"created_at"`
	CompletedAt            *time.Time  `json:"completed_at,omitempty"`
}

// ── Ad Campaign ───────────────────────────────────────────────────────────────

type AdCampaign struct {
	ID          string         `json:"id"`
	EventID     string         `json:"event_id"`
	OrganizerID string         `json:"organizer_id"`

	DailyBudget float64 `json:"daily_budget"`
	CPMRate     float64 `json:"cpm_rate"`

	TotalSpent       float64 `json:"total_spent"`
	TotalImpressions int64   `json:"total_impressions"`
	TotalClicks      int64   `json:"total_clicks"`

	// Today's numbers — joined from ad_daily_spend at query time
	TodaySpent       float64 `json:"today_spent"`
	TodayImpressions int64   `json:"today_impressions"`
	TodayClicks      int64   `json:"today_clicks"`

	StartDate time.Time  `json:"start_date"`
	EndDate   *time.Time `json:"end_date,omitempty"`

	Status    CampaignStatus `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`

	Event *Event `json:"event,omitempty"`
}

// CostPerImpression returns the ZAR charged per individual impression.
func (c *AdCampaign) CostPerImpression() float64 {
	return c.CPMRate / 1000.0
}

// DailyImpressionCap returns how many impressions this campaign can serve
// today before hitting its daily budget.
func (c *AdCampaign) DailyImpressionCap() int64 {
	if c.CostPerImpression() == 0 {
		return 0
	}
	return int64(c.DailyBudget / c.CostPerImpression())
}

// ── DTOs ──────────────────────────────────────────────────────────────────────

type CreateCampaignDTO struct {
	DailyBudget float64    `json:"daily_budget" binding:"required,min=50"`
	StartDate   *time.Time `json:"start_date"`
	EndDate     *time.Time `json:"end_date"`
}

type UpdateCampaignDTO struct {
	DailyBudget *float64        `json:"daily_budget" binding:"omitempty,min=50"`
	EndDate     *time.Time      `json:"end_date"`
	Status      *CampaignStatus `json:"status"`
}

type TopupWalletDTO struct {
	Amount float64 `json:"amount" binding:"required,min=50"`
}