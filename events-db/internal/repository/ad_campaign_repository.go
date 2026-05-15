package repository

import (
	"context"
	"errors"
	"events-db/internal/models"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

var (
	ErrCampaignNotFound     = errors.New("campaign not found")
	ErrCampaignUnauthorized = errors.New("campaign belongs to a different organiser")
	ErrEventAlreadyBoosted  = errors.New("this event already has an active campaign")
	ErrNoPaymentMethod      = errors.New("no payment method on file — register a Stitch debit mandate first")
)

// DefaultCPMRate is ZAR per 1 000 impressions.
// R20 CPM: R200/day → 10 000 impressions; R500/day → 25 000 impressions.
const DefaultCPMRate = 20.00

// ── Payment method ────────────────────────────────────────────────────────────

// RegisterPaymentMethod stores the organiser's Stitch debit mandate ID.
// Called once after the organiser completes the Stitch mandate OAuth flow.
// All future end-of-day ad bills are automatically debited against this mandate.
func RegisterPaymentMethod(pool *pgxpool.Pool, organizerID string, dto *models.RegisterPaymentMethodDTO) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE organizers
		SET    stitch_mandate_id     = $1,
		       stitch_mandate_status = 'active',
		       ad_payment_email      = $2,
		       updated_at            = NOW()
		WHERE  id = $3
	`, dto.StitchMandateID, dto.Email, organizerID)
	if err != nil {
		return fmt.Errorf("RegisterPaymentMethod: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrOrganizerNotFound
	}

	// Re-activate any campaigns that were paused due to payment failure.
	_, _ = pool.Exec(ctx, `
		UPDATE ad_campaigns
		SET    status = 'active', updated_at = NOW()
		WHERE  organizer_id = $1
		  AND  status = 'payment_failed'
		  AND  (end_date IS NULL OR end_date >= CURRENT_DATE)
	`, organizerID)

	return nil
}

// GetPaymentMethodStatus returns whether the organiser has an active Stitch mandate.
func GetPaymentMethodStatus(pool *pgxpool.Pool, organizerID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var status string
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(stitch_mandate_status, 'none')
		FROM   organizers WHERE id = $1
	`, organizerID).Scan(&status)
	if err != nil {
		return "none", err
	}
	return status, nil
}

// ── Campaigns ─────────────────────────────────────────────────────────────────

// CreateCampaign creates a new boost campaign for an event.
//
// Post-pay model: campaign starts as 'active' immediately — no wallet balance check.
// Two prerequisites:
//  1. The organiser must have a Stitch debit mandate registered.
//  2. The event must be published (admin-approved) — can't boost a draft.
func CreateCampaign(pool *pgxpool.Pool, eventID, organizerID string, dto *models.CreateCampaignDTO) (*models.AdCampaign, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Verify organiser owns the event AND it is published.
	var ownerID, eventStatus string
	if err := tx.QueryRow(ctx,
		`SELECT organizer_id, status FROM events WHERE id = $1`, eventID,
	).Scan(&ownerID, &eventStatus); err != nil || ownerID != organizerID {
		return nil, ErrCampaignUnauthorized
	}
	if eventStatus != string(models.EventPublished) {
		return nil, fmt.Errorf("ad campaigns can only be created for published events")
	}

	// Require a payment method before accepting a campaign.
	var mandateStatus string
	tx.QueryRow(ctx, `
		SELECT COALESCE(stitch_mandate_status, 'none') FROM organizers WHERE id = $1
	`, organizerID).Scan(&mandateStatus)
	if mandateStatus != "active" {
		return nil, ErrNoPaymentMethod
	}

	// Block duplicate active campaigns for the same event.
	var existing int
	tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM ad_campaigns
		WHERE event_id = $1
		  AND status IN ('active','paused','exhausted','payment_failed')
	`, eventID).Scan(&existing)
	if existing > 0 {
		return nil, ErrEventAlreadyBoosted
	}

	startDate := time.Now()
	if dto.StartDate != nil {
		startDate = *dto.StartDate
	}

	var c models.AdCampaign
	err = tx.QueryRow(ctx, `
		INSERT INTO ad_campaigns
		    (event_id, organizer_id, daily_budget, cpm_rate, start_date, end_date, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'active')
		RETURNING id, event_id, organizer_id, daily_budget, cpm_rate,
		          total_spent, total_impressions, total_clicks,
		          start_date, end_date, status, created_at, updated_at
	`, eventID, organizerID, dto.DailyBudget, DefaultCPMRate,
		startDate.Format("2006-01-02"), dto.EndDate,
	).Scan(
		&c.ID, &c.EventID, &c.OrganizerID, &c.DailyBudget, &c.CPMRate,
		&c.TotalSpent, &c.TotalImpressions, &c.TotalClicks,
		&c.StartDate, &c.EndDate, &c.Status, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateCampaign: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCampaign returns a single campaign with today's spend joined in.
func GetCampaign(pool *pgxpool.Pool, campaignID, organizerID string) (*models.AdCampaign, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var c models.AdCampaign
	err := pool.QueryRow(ctx, `
		SELECT
		    ac.id, ac.event_id, ac.organizer_id, ac.daily_budget, ac.cpm_rate,
		    ac.total_spent, ac.total_impressions, ac.total_clicks,
		    ac.start_date, ac.end_date, ac.status, ac.created_at, ac.updated_at,
		    COALESCE(ds.amount_spent, 0),
		    COALESCE(ds.impressions,  0),
		    COALESCE(ds.clicks,       0)
		FROM   ad_campaigns ac
		LEFT JOIN ad_daily_spend ds
		       ON ds.campaign_id = ac.id AND ds.spend_date = CURRENT_DATE
		WHERE  ac.id = $1
	`, campaignID).Scan(
		&c.ID, &c.EventID, &c.OrganizerID, &c.DailyBudget, &c.CPMRate,
		&c.TotalSpent, &c.TotalImpressions, &c.TotalClicks,
		&c.StartDate, &c.EndDate, &c.Status, &c.CreatedAt, &c.UpdatedAt,
		&c.TodaySpent, &c.TodayImpressions, &c.TodayClicks,
	)
	if err != nil {
		return nil, ErrCampaignNotFound
	}
	if organizerID != "" && c.OrganizerID != organizerID {
		return nil, ErrCampaignUnauthorized
	}
	return &c, nil
}

// GetOrganizerCampaigns returns all campaigns for an organiser with today's stats.
func GetOrganizerCampaigns(pool *pgxpool.Pool, organizerID string) ([]models.AdCampaign, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT
		    ac.id, ac.event_id, ac.organizer_id, ac.daily_budget, ac.cpm_rate,
		    ac.total_spent, ac.total_impressions, ac.total_clicks,
		    ac.start_date, ac.end_date, ac.status, ac.created_at, ac.updated_at,
		    COALESCE(ds.amount_spent, 0),
		    COALESCE(ds.impressions,  0),
		    COALESCE(ds.clicks,       0)
		FROM   ad_campaigns ac
		LEFT JOIN ad_daily_spend ds
		       ON ds.campaign_id = ac.id AND ds.spend_date = CURRENT_DATE
		WHERE  ac.organizer_id = $1
		ORDER BY ac.created_at DESC
	`, organizerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AdCampaign
	for rows.Next() {
		var c models.AdCampaign
		if err := rows.Scan(
			&c.ID, &c.EventID, &c.OrganizerID, &c.DailyBudget, &c.CPMRate,
			&c.TotalSpent, &c.TotalImpressions, &c.TotalClicks,
			&c.StartDate, &c.EndDate, &c.Status, &c.CreatedAt, &c.UpdatedAt,
			&c.TodaySpent, &c.TodayImpressions, &c.TodayClicks,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateCampaign applies organiser-allowed field changes to a campaign.
// Only active <-> paused transitions are permitted via this endpoint.
func UpdateCampaign(pool *pgxpool.Pool, campaignID, organizerID string, dto *models.UpdateCampaignDTO) (*models.AdCampaign, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := GetCampaign(pool, campaignID, organizerID)
	if err != nil {
		return nil, err
	}

	if dto.Status != nil {
		allowed := map[models.CampaignStatus]bool{
			models.CampaignActive: c.Status == models.CampaignPaused,
			models.CampaignPaused: c.Status == models.CampaignActive || c.Status == models.CampaignExhausted,
		}
		if !allowed[*dto.Status] {
			return nil, fmt.Errorf("cannot transition campaign from %s to %s", c.Status, *dto.Status)
		}
	}

	if dto.DailyBudget != nil {
		c.DailyBudget = *dto.DailyBudget
	}
	if dto.EndDate != nil {
		c.EndDate = dto.EndDate
	}
	newStatus := c.Status
	if dto.Status != nil {
		newStatus = *dto.Status
	}

	if _, err = pool.Exec(ctx, `
		UPDATE ad_campaigns
		SET    daily_budget = $1, end_date = $2, status = $3, updated_at = NOW()
		WHERE  id = $4
	`, c.DailyBudget, c.EndDate, string(newStatus), campaignID); err != nil {
		return nil, fmt.Errorf("UpdateCampaign: %w", err)
	}
	c.Status = newStatus
	return c, nil
}

// EndCampaign permanently ends a campaign (status = 'ended').
func EndCampaign(pool *pgxpool.Pool, campaignID, organizerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE ad_campaigns
		SET    status = 'ended', updated_at = NOW()
		WHERE  id = $1 AND organizer_id = $2 AND status != 'ended'
	`, campaignID, organizerID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrCampaignNotFound
	}
	return nil
}

// ── Feed injection ────────────────────────────────────────────────────────────

// SponsoredEvent wraps an Event with campaign metadata needed to record
// an impression after the feed is served. Never serialised to the client.
type SponsoredEvent struct {
	models.Event
	CampaignID        string  `json:"-"`
	CostPerImpression float64 `json:"-"`
}

// GetSponsoredForFeed returns the top N campaigns eligible to serve right now.
//
// Post-pay model: NO wallet balance check. Campaigns run as long as they are
// 'active' and today's spend hasn't hit the daily_budget cap.
// The only billing-related pause is 'payment_failed', which is already excluded
// by the status = 'active' filter.
//
// Campaigns are ranked by:
//  1. daily_budget × affinity_multiplier (more spend = higher rank)
//  2. affinity_multiplier = 1 + min(prior bookings in category, 3) × 0.2
//  3. views DESC (tiebreaker)
func GetSponsoredForFeed(pool *pgxpool.Pool, userID *string, limit int) ([]SponsoredEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	uid := ""
	if userID != nil {
		uid = *userID
	}

	rows, err := pool.Query(ctx, `
		WITH user_cats AS (
			SELECT e.category_id, COUNT(*) AS cnt
			FROM   bookings b
			JOIN   events   e ON e.id = b.event_id
			WHERE  $1 != ''
			  AND  b.user_id        = $1
			  AND  b.booking_status IN ('confirmed','pending')
			GROUP BY e.category_id
		)
		SELECT
		    ev.id, ev.title, ev.description, ev.cover_image_url,
		    ev.start_time, ev.end_time, ev.price, ev.currency,
		    ev.city, ev.venue_name, ev.max_capacity, ev.tickets_sold,
		    ev.is_featured, ev.status, ev.views, ev.created_at,
		    c.name AS category,
		    ac.id  AS campaign_id,
		    ac.cpm_rate / 1000.0 AS cost_per_impression
		FROM   ad_campaigns  ac
		JOIN   events        ev ON ev.id = ac.event_id
		JOIN   categories    c  ON c.id  = ev.category_id
		LEFT JOIN ad_daily_spend ds
		          ON ds.campaign_id = ac.id AND ds.spend_date = CURRENT_DATE
		LEFT JOIN user_cats uc ON uc.category_id = ev.category_id
		WHERE  ac.status     = 'active'
		  AND  ev.status     = 'published'
		  AND  ev.start_time > NOW()
		  AND  (ac.end_date IS NULL OR ac.end_date >= CURRENT_DATE)
		  AND  COALESCE(ds.amount_spent, 0) < ac.daily_budget
		ORDER BY (
		    ac.daily_budget
		    * (1.0 + LEAST(COALESCE(uc.cnt, 0), 3) * 0.2)
		) DESC,
		ev.views DESC
		LIMIT $2
	`, uid, limit)
	if err != nil {
		return nil, fmt.Errorf("GetSponsoredForFeed: %w", err)
	}
	defer rows.Close()

	var out []SponsoredEvent
	for rows.Next() {
		var s SponsoredEvent
		if err := rows.Scan(
			&s.ID, &s.Title, &s.Description, &s.CoverImageURL,
			&s.StartTime, &s.EndTime, &s.Price, &s.Currency,
			&s.City, &s.VenueName, &s.MaxCapacity, &s.TicketsSold,
			&s.IsFeatured, &s.Status, &s.Views, &s.CreatedAt,
			&s.Category,
			&s.CampaignID, &s.CostPerImpression,
		); err != nil {
			return nil, fmt.Errorf("GetSponsoredForFeed scan: %w", err)
		}
		s.IsSponsored = true
		s.ComputeAvailableTickets()
		out = append(out, s)
	}
	return out, rows.Err()
}

// RecordImpressions tracks spend per impression in ad_daily_spend.
//
// Post-pay: no wallet deduction here — actual money is collected at end of day
// by the nightly billing worker via Stitch debit mandate.
// The campaign is marked 'exhausted' only when today's spend hits daily_budget.
func RecordImpressions(pool *pgxpool.Pool, campaigns []SponsoredEvent) {
	if len(campaigns) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, s := range campaigns {
		cost := s.CostPerImpression

		_, err := pool.Exec(ctx, `
			WITH upsert AS (
				INSERT INTO ad_daily_spend (campaign_id, spend_date, impressions, amount_spent)
				VALUES ($1, CURRENT_DATE, 1, $2)
				ON CONFLICT (campaign_id, spend_date) DO UPDATE
				    SET impressions  = ad_daily_spend.impressions  + 1,
				        amount_spent = ad_daily_spend.amount_spent + $2
				RETURNING amount_spent
			)
			UPDATE ad_campaigns ac
			SET    total_impressions = total_impressions + 1,
			       total_spent       = total_spent + $2,
			       status = CASE
			           WHEN (SELECT amount_spent FROM upsert) >= ac.daily_budget
			           THEN 'exhausted'::campaign_status
			           ELSE ac.status
			       END,
			       updated_at = NOW()
			WHERE  ac.id = $1
		`, s.CampaignID, cost)

		if err != nil {
			log.Error().Err(err).Str("campaign_id", s.CampaignID).
				Msg("ad: failed to record impression")
		}
	}
}

// RecordClick increments click count when a user navigates to the event detail
// from a sponsored card.
func RecordClick(pool *pgxpool.Pool, campaignID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		WITH upsert AS (
			INSERT INTO ad_daily_spend (campaign_id, spend_date, clicks)
			VALUES ($1, CURRENT_DATE, 1)
			ON CONFLICT (campaign_id, spend_date) DO UPDATE
			    SET clicks = ad_daily_spend.clicks + 1
		)
		UPDATE ad_campaigns
		SET    total_clicks = total_clicks + 1, updated_at = NOW()
		WHERE  id = $1
	`, campaignID)
	if err != nil {
		log.Error().Err(err).Str("campaign_id", campaignID).
			Msg("ad: failed to record click")
	}
}

// ── Feed helpers ──────────────────────────────────────────────────────────────

// InjectPreFetched weaves already-fetched sponsored events into an organic slice.
// Use this when GetSponsoredForFeed was already called upstream.
func InjectPreFetched(organic []models.Event, sponsored []SponsoredEvent, maxAds int) []models.Event {
	if len(organic) == 0 || len(sponsored) == 0 || maxAds == 0 {
		return organic
	}

	adPositions := make([]int, 0, maxAds)
	for i := 0; i < maxAds; i++ {
		adPositions = append(adPositions, 2+i*4)
	}

	result := make([]models.Event, 0, len(organic)+len(sponsored))
	orgIdx, sponIdx, posIdx := 0, 0, 0

	for orgIdx < len(organic) || sponIdx < len(sponsored) {
		if sponIdx < len(sponsored) &&
			posIdx < len(adPositions) &&
			len(result) == adPositions[posIdx] {
			result = append(result, sponsored[sponIdx].Event)
			sponIdx++
			posIdx++
		} else if orgIdx < len(organic) {
			result = append(result, organic[orgIdx])
			orgIdx++
		} else {
			break
		}
	}
	return result
}

// InjectSponsored weaves sponsored event cards into an organic event slice.
// Impressions are recorded asynchronously — never blocks the HTTP response.
func InjectSponsored(pool *pgxpool.Pool, organic []models.Event, userID *string, maxAds int) []models.Event {
	if len(organic) == 0 || maxAds == 0 {
		return organic
	}

	sponsored, err := GetSponsoredForFeed(pool, userID, maxAds)
	if err != nil || len(sponsored) == 0 {
		return organic
	}

	adPositions := make([]int, 0, maxAds)
	for i := 0; i < maxAds; i++ {
		adPositions = append(adPositions, 2+i*4)
	}

	result := make([]models.Event, 0, len(organic)+len(sponsored))
	orgIdx, sponIdx, posIdx := 0, 0, 0

	for orgIdx < len(organic) || sponIdx < len(sponsored) {
		if sponIdx < len(sponsored) &&
			posIdx < len(adPositions) &&
			len(result) == adPositions[posIdx] {
			result = append(result, sponsored[sponIdx].Event)
			sponIdx++
			posIdx++
		} else if orgIdx < len(organic) {
			result = append(result, organic[orgIdx])
			orgIdx++
		} else {
			break
		}
	}

	shown := sponsored[:sponIdx]
	go RecordImpressions(pool, shown)
	return result
}

// ── Nightly billing ───────────────────────────────────────────────────────────

// OrganizerDaySpend is one row per organiser for nightly billing.
type OrganizerDaySpend struct {
	OrganizerID  string
	MandateID    string
	PaymentEmail string
	TotalSpend   float64
}

// GetUnbilledDailySpend returns each organiser's total unbilled spend for the
// given date. Called by the nightly billing worker.
// Only returns rows where spend >= R1.00 and a mandate exists.
func GetUnbilledDailySpend(pool *pgxpool.Pool, forDate time.Time) ([]OrganizerDaySpend, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT
		    o.id                             AS organizer_id,
		    o.stitch_mandate_id              AS mandate_id,
		    COALESCE(o.ad_payment_email, '') AS payment_email,
		    SUM(ds.amount_spent)             AS total_spend
		FROM   ad_daily_spend ds
		JOIN   ad_campaigns ac ON ac.id = ds.campaign_id
		JOIN   organizers   o  ON o.id  = ac.organizer_id
		WHERE  ds.spend_date = $1
		  AND  ds.billed     = FALSE
		  AND  ds.amount_spent > 0
		  AND  o.stitch_mandate_id IS NOT NULL
		  AND  o.stitch_mandate_status = 'active'
		GROUP BY o.id, o.stitch_mandate_id, o.ad_payment_email
		HAVING SUM(ds.amount_spent) >= 1.00
	`, forDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("GetUnbilledDailySpend: %w", err)
	}
	defer rows.Close()

	var out []OrganizerDaySpend
	for rows.Next() {
		var s OrganizerDaySpend
		if err := rows.Scan(&s.OrganizerID, &s.MandateID, &s.PaymentEmail, &s.TotalSpend); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CreateBillingRecord inserts a pending billing record for an organiser's daily spend.
func CreateBillingRecord(pool *pgxpool.Pool, organizerID string, billingDate time.Time, amount float64, stitchRequestID string) (*models.AdBillingRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var r models.AdBillingRecord
	err := pool.QueryRow(ctx, `
		INSERT INTO ad_billing_records
		    (organizer_id, billing_date, amount, stitch_payment_request_id, status)
		VALUES ($1, $2, $3, NULLIF($4,''), 'pending')
		ON CONFLICT (organizer_id, billing_date) DO UPDATE
		    SET amount = EXCLUDED.amount,
		        stitch_payment_request_id = EXCLUDED.stitch_payment_request_id,
		        status = 'pending'
		RETURNING id, organizer_id, billing_date, amount,
		          stitch_payment_request_id, status, created_at, completed_at
	`, organizerID, billingDate.Format("2006-01-02"), amount, stitchRequestID).Scan(
		&r.ID, &r.OrganizerID, &r.BillingDate, &r.Amount,
		&r.StitchPaymentRequestID, &r.Status, &r.CreatedAt, &r.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateBillingRecord: %w", err)
	}
	return &r, nil
}

// ConfirmBilling is called by the Stitch webhook when a daily debit succeeds.
// Atomically marks the billing record completed and flags all spend rows as billed.
// Re-activates any payment_failed campaigns for this organiser.
func ConfirmBilling(pool *pgxpool.Pool, stitchRequestID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var organizerID string
	var billingDate time.Time
	err = tx.QueryRow(ctx, `
		UPDATE ad_billing_records
		SET    status = 'completed', completed_at = NOW()
		WHERE  stitch_payment_request_id = $1 AND status = 'pending'
		RETURNING organizer_id, billing_date
	`, stitchRequestID).Scan(&organizerID, &billingDate)
	if err != nil {
		return fmt.Errorf("ConfirmBilling: record not found or already processed: %w", err)
	}

	// Mark all daily spend rows for this organiser on this date as billed.
	if _, err = tx.Exec(ctx, `
		UPDATE ad_daily_spend ds
		SET    billed = TRUE
		FROM   ad_campaigns ac
		WHERE  ds.campaign_id  = ac.id
		  AND  ac.organizer_id = $1
		  AND  ds.spend_date   = $2
	`, organizerID, billingDate.Format("2006-01-02")); err != nil {
		return fmt.Errorf("ConfirmBilling: mark billed: %w", err)
	}

	// Re-activate campaigns that were paused due to payment failure.
	_, _ = tx.Exec(ctx, `
		UPDATE ad_campaigns
		SET    status = 'active', updated_at = NOW()
		WHERE  organizer_id = $1
		  AND  status = 'payment_failed'
		  AND  (end_date IS NULL OR end_date >= CURRENT_DATE)
	`, organizerID)

	return tx.Commit(ctx)
}

// FailBilling marks a billing record as failed and pauses the organiser's campaigns.
// Called by the nightly worker or Stitch webhook when a debit is declined.
// Campaigns are paused with status 'payment_failed' until the mandate is fixed.
func FailBilling(pool *pgxpool.Pool, stitchRequestID string, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var organizerID string
	err = tx.QueryRow(ctx, `
		UPDATE ad_billing_records
		SET    status = 'failed', failure_reason = $2
		WHERE  stitch_payment_request_id = $1 AND status = 'pending'
		RETURNING organizer_id
	`, stitchRequestID, reason).Scan(&organizerID)
	if err != nil {
		return fmt.Errorf("FailBilling: record not found: %w", err)
	}

	// Pause all active/exhausted campaigns until organiser fixes their payment method.
	if _, err = tx.Exec(ctx, `
		UPDATE ad_campaigns
		SET    status = 'payment_failed', updated_at = NOW()
		WHERE  organizer_id = $1
		  AND  status IN ('active','exhausted')
	`, organizerID); err != nil {
		return fmt.Errorf("FailBilling: pause campaigns: %w", err)
	}

	return tx.Commit(ctx)
}

// GetBillingHistory returns billing records for an organiser, newest first.
func GetBillingHistory(pool *pgxpool.Pool, organizerID string, limit int) ([]models.AdBillingRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT id, organizer_id, billing_date, amount,
		       stitch_payment_request_id, status, failure_reason,
		       created_at, completed_at
		FROM   ad_billing_records
		WHERE  organizer_id = $1
		ORDER BY billing_date DESC
		LIMIT  $2
	`, organizerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AdBillingRecord
	for rows.Next() {
		var r models.AdBillingRecord
		if err := rows.Scan(
			&r.ID, &r.OrganizerID, &r.BillingDate, &r.Amount,
			&r.StitchPaymentRequestID, &r.Status, &r.FailureReason,
			&r.CreatedAt, &r.CompletedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── Nightly maintenance ───────────────────────────────────────────────────────

// ResumeExhaustedCampaigns reactivates campaigns that hit their daily budget.
// Called at midnight by the ad_worker AFTER billing has been processed.
// Does NOT resume payment_failed campaigns — those need the organiser to fix their mandate.
func ResumeExhaustedCampaigns(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ad_campaigns
		SET    status = 'active', updated_at = NOW()
		WHERE  status = 'exhausted'
		  AND  (end_date IS NULL OR end_date >= CURRENT_DATE)
	`)
	return err
}

// EndExpiredCampaigns closes campaigns whose end_date has passed.
func EndExpiredCampaigns(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ad_campaigns
		SET    status = 'ended', updated_at = NOW()
		WHERE  status IN ('active','paused','exhausted','payment_failed')
		  AND  end_date < CURRENT_DATE
	`)
	return err
}

// ── Admin ─────────────────────────────────────────────────────────────────────

// GetAllCampaigns returns every campaign across all organisers for the admin dashboard.
func GetAllCampaigns(pool *pgxpool.Pool, statusFilter string) ([]models.AdCampaign, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{}
	where := ""
	if statusFilter != "" {
		args = append(args, statusFilter)
		where = "WHERE ac.status = $1"
	}

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT
		    ac.id, ac.event_id, ac.organizer_id, ac.daily_budget, ac.cpm_rate,
		    ac.total_spent, ac.total_impressions, ac.total_clicks,
		    ac.start_date, ac.end_date, ac.status, ac.created_at, ac.updated_at,
		    COALESCE(ds.amount_spent, 0),
		    COALESCE(ds.impressions,  0),
		    COALESCE(ds.clicks,       0)
		FROM   ad_campaigns ac
		LEFT JOIN ad_daily_spend ds
		       ON ds.campaign_id = ac.id AND ds.spend_date = CURRENT_DATE
		%s
		ORDER BY ac.daily_budget DESC, ac.created_at DESC
	`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AdCampaign
	for rows.Next() {
		var c models.AdCampaign
		if err := rows.Scan(
			&c.ID, &c.EventID, &c.OrganizerID, &c.DailyBudget, &c.CPMRate,
			&c.TotalSpent, &c.TotalImpressions, &c.TotalClicks,
			&c.StartDate, &c.EndDate, &c.Status, &c.CreatedAt, &c.UpdatedAt,
			&c.TodaySpent, &c.TodayImpressions, &c.TodayClicks,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}