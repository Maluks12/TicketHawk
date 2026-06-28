package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

var (
	ErrCampaignNotFound     = errors.New("campaign not found")
	ErrCampaignUnauthorized = errors.New("campaign belongs to a different organiser")
	ErrEventAlreadyBoosted  = errors.New("this event already has an active campaign")
)

// DefaultCPMRate is ZAR per 1 000 impressions.
// R20 CPM: R200/day → 10 000 impressions; R500/day → 25 000 impressions.
const DefaultCPMRate = 20.00

// ── Campaigns ─────────────────────────────────────────────────────────────────

// CreateCampaign creates a new boost campaign for a published event.
//
// Pre-paid wallet model: the campaign starts 'active' immediately. It only
// serves while the organiser's wallet has a positive balance (enforced in
// GetSponsoredForFeed), so no card/payment-method check is required at creation —
// the organiser tops up the per-organiser wallet to fund all their campaigns.
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

	// Verify the ad account belongs to this organizer and is active.
	// (Ad accounts now group campaigns; billing is the per-organiser wallet.)
	var accountOrgID, accountStatus string
	err = tx.QueryRow(ctx, `
		SELECT organizer_id, status FROM ad_accounts WHERE id = $1
	`, dto.AdAccountID).Scan(&accountOrgID, &accountStatus)
	if err != nil || accountOrgID != organizerID {
		return nil, ErrAdAccountUnauthorized
	}
	if accountStatus != string(models.AdAccountActive) {
		return nil, fmt.Errorf("ad account is disabled — re-enable it before creating campaigns")
	}

	// Block duplicate active campaigns for the same event.
	var existing int
	tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM ad_campaigns
		WHERE event_id = $1
		  AND status IN ('active','paused','exhausted')
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
		    (event_id, organizer_id, ad_account_id, daily_budget, cpm_rate, start_date, end_date, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'active')
		RETURNING id, event_id, organizer_id, ad_account_id, daily_budget, cpm_rate,
		          total_spent, total_impressions, total_clicks,
		          start_date, end_date, status, created_at, updated_at
	`, eventID, organizerID, dto.AdAccountID, dto.DailyBudget, DefaultCPMRate,
		startDate.Format("2006-01-02"), dto.EndDate,
	).Scan(
		&c.ID, &c.EventID, &c.OrganizerID, &c.AdAccountID, &c.DailyBudget, &c.CPMRate,
		&c.TotalSpent, &c.TotalImpressions, &c.TotalClicks,
		&c.StartDate, &c.EndDate, &c.Status, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateCampaign: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Make sure the organiser has a wallet to fund this campaign.
	if _, werr := GetOrCreateAdWallet(pool, organizerID); werr != nil {
		log.Warn().Err(werr).Str("organizer_id", organizerID).Msg("CreateCampaign: wallet ensure failed")
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
		WHERE  id = $1 AND organizer_id = $2
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
// Pre-paid wallet model: a campaign serves only while its organiser's wallet has
// a positive balance (INNER JOIN on ad_wallets … balance > 0) and today's spend
// hasn't hit the daily_budget cap. A drained wallet stops serving immediately;
// a top-up resumes serving with no status change.
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
		JOIN   ad_wallets    aw ON aw.organizer_id = ac.organizer_id AND aw.balance > 0
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

// RecordImpressions tracks spend per impression and draws the organiser's wallet
// down in the same statement.
//
// Pre-paid wallet model: each impression atomically debits the wallet (clamped
// at zero so the balance never goes negative), upserts ad_daily_spend, and marks
// the campaign 'exhausted' when today's spend hits daily_budget. Once the wallet
// reaches zero, GetSponsoredForFeed stops selecting the campaign, so no further
// impressions are recorded.
func RecordImpressions(pool *pgxpool.Pool, campaigns []SponsoredEvent) {
	if len(campaigns) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, s := range campaigns {
		cost := s.CostPerImpression

		_, err := pool.Exec(ctx, `
			WITH wallet AS (
				UPDATE ad_wallets
				SET    balance     = GREATEST(balance - $2, 0),
				       total_spent = total_spent + LEAST(balance, $2),
				       updated_at  = NOW()
				WHERE  organizer_id = (SELECT organizer_id FROM ad_campaigns WHERE id = $1)
				RETURNING balance
			),
			spend AS (
				INSERT INTO ad_daily_spend (campaign_id, spend_date, impressions, amount_spent)
				VALUES ($1, CURRENT_DATE, 1, $2)
				ON CONFLICT (campaign_id, spend_date) DO UPDATE
				    SET impressions  = ad_daily_spend.impressions + 1,
				        amount_spent = ad_daily_spend.amount_spent + $2
				RETURNING amount_spent
			)
			UPDATE ad_campaigns ac
			SET    total_impressions = total_impressions + 1,
			       total_spent       = total_spent + $2,
			       status = CASE
			           WHEN (SELECT amount_spent FROM spend) >= ac.daily_budget
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

// InjectPreFetched weaves already-fetched sponsored events into an organic slice.
func InjectPreFetched(organic []models.Event, sponsored []SponsoredEvent, maxAds int) []models.Event {
	if len(organic) == 0 || len(sponsored) == 0 || maxAds == 0 {
		return organic
	}

	// Only the first maxAds cards are actually woven in.
	if len(sponsored) > maxAds {
		sponsored = sponsored[:maxAds]
	}

	// Drop any organic copy of an event we're about to inject as a sponsored
	// card. Without this the same event id can appear twice in one section —
	// once organically and once as its sponsored card — which renders fine
	// server-side but trips React's "two children with the same key" guard on
	// the client.
	adIDs := make(map[string]bool, len(sponsored))
	for _, s := range sponsored {
		adIDs[s.ID] = true
	}
	filtered := make([]models.Event, 0, len(organic))
	for _, e := range organic {
		if !adIDs[e.ID] {
			filtered = append(filtered, e)
		}
	}
	organic = filtered

	adPositions := make([]int, 0, len(sponsored))
	for i := 0; i < len(sponsored); i++ {
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
			// Organic exhausted before every ad position was reached (short
			// section, or dedup removed organic items) — append the remaining
			// sponsored cards so a promoted/sponsored event is never dropped.
			result = append(result, sponsored[sponIdx].Event)
			sponIdx++
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

	// Reuse the shared weaving logic so the organic/sponsored de-duplication
	// (and the duplicate-key fix) lives in exactly one place.
	result := InjectPreFetched(organic, sponsored, maxAds)

	shown := sponsored
	if len(shown) > maxAds {
		shown = shown[:maxAds]
	}
	go RecordImpressions(pool, shown)
	return result
}

// ── Nightly maintenance ───────────────────────────────────────────────────────

// ResumeExhaustedCampaigns reactivates campaigns that hit their daily budget.
// Called at midnight by the ad_worker so a new day's budget is available.
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
		WHERE  status IN ('active','paused','exhausted')
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