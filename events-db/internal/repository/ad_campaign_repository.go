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
	ErrWalletInsufficient   = errors.New("wallet balance insufficient — please top up")
	ErrTopupNotFound        = errors.New("topup not found")
)

// DefaultCPMRate is ZAR per 1 000 impressions.
// R20 CPM: R50/day → 2 500 impressions; R500/day → 25 000 impressions.
const DefaultCPMRate = 20.00

// ── Wallet ────────────────────────────────────────────────────────────────────

// GetOrCreateWallet returns the organiser's ad wallet, creating it on first call.
func GetOrCreateWallet(pool *pgxpool.Pool, organizerID string) (*models.AdWallet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var w models.AdWallet
	err := pool.QueryRow(ctx, `
		INSERT INTO ad_wallets (organizer_id)
		VALUES ($1)
		ON CONFLICT (organizer_id) DO UPDATE
		    SET updated_at = ad_wallets.updated_at
		RETURNING id, organizer_id, balance, total_topups, total_spent, created_at, updated_at
	`, organizerID).Scan(
		&w.ID, &w.OrganizerID, &w.Balance,
		&w.TotalTopups, &w.TotalSpent,
		&w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrCreateWallet: %w", err)
	}
	return &w, nil
}

// ── Top-ups ───────────────────────────────────────────────────────────────────

// CreateTopup records a pending wallet top-up row after the Stitch payment
// intent has been created. The Stitch request ID is stored so the webhook
// can match the incoming confirmation to the right topup.
func CreateTopup(pool *pgxpool.Pool, organizerID string, amount float64, stitchRequestID string) (*models.AdWalletTopup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ensure wallet exists
	if _, err := GetOrCreateWallet(pool, organizerID); err != nil {
		return nil, err
	}

	var t models.AdWalletTopup
	err := pool.QueryRow(ctx, `
		INSERT INTO ad_wallet_topups
		    (organizer_id, amount, stitch_payment_request_id, reference_number)
		VALUES (
		    $1, $2, $3,
		    'TUP-' || LPAD(nextval('ad_topup_ref_seq')::text, 5, '0')
		)
		RETURNING id, organizer_id, amount, stitch_payment_request_id,
		          reference_number, status, created_at, completed_at
	`, organizerID, amount, stitchRequestID).Scan(
		&t.ID, &t.OrganizerID, &t.Amount, &t.StitchPaymentRequestID,
		&t.ReferenceNumber, &t.Status, &t.CreatedAt, &t.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateTopup: %w", err)
	}
	return &t, nil
}

// ConfirmTopup is called by the Stitch ad webhook when payment completes.
// It atomically:
//  1. Marks the topup as completed
//  2. Credits the wallet
//  3. Re-activates any campaigns that were paused due to empty wallet
func ConfirmTopup(pool *pgxpool.Pool, stitchRequestID string) (*models.AdWalletTopup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var t models.AdWalletTopup
	err = tx.QueryRow(ctx, `
		UPDATE ad_wallet_topups
		SET    status       = 'completed',
		       completed_at = NOW()
		WHERE  stitch_payment_request_id = $1
		  AND  status = 'pending'
		RETURNING id, organizer_id, amount, stitch_payment_request_id,
		          reference_number, status, created_at, completed_at
	`, stitchRequestID).Scan(
		&t.ID, &t.OrganizerID, &t.Amount, &t.StitchPaymentRequestID,
		&t.ReferenceNumber, &t.Status, &t.CreatedAt, &t.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("ConfirmTopup: topup not found or already processed: %w", err)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE ad_wallets
		SET    balance      = balance + $1,
		       total_topups = total_topups + $1,
		       updated_at   = NOW()
		WHERE  organizer_id = $2
	`, t.Amount, t.OrganizerID); err != nil {
		return nil, fmt.Errorf("ConfirmTopup: credit wallet: %w", err)
	}

	// Re-activate campaigns that were exhausted due to empty wallet
	if _, err = tx.Exec(ctx, `
		UPDATE ad_campaigns
		SET    status     = 'active',
		       updated_at = NOW()
		WHERE  organizer_id = $1
		  AND  status       = 'exhausted'
		  AND  (end_date IS NULL OR end_date >= CURRENT_DATE)
	`, t.OrganizerID); err != nil {
		return nil, fmt.Errorf("ConfirmTopup: reactivate campaigns: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &t, nil
}

// ── Campaigns ─────────────────────────────────────────────────────────────────

// CreateCampaign creates a new boost campaign for an event.
// Starts as 'active' if the wallet has sufficient balance; 'draft' otherwise.
func CreateCampaign(pool *pgxpool.Pool, eventID, organizerID string, dto *models.CreateCampaignDTO) (*models.AdCampaign, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Verify organiser owns the event
	var ownerID string
	if err := tx.QueryRow(ctx,
		`SELECT organizer_id FROM events WHERE id = $1`, eventID,
	).Scan(&ownerID); err != nil || ownerID != organizerID {
		return nil, ErrCampaignUnauthorized
	}

	// Block duplicate active campaigns for the same event
	var existing int
	tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM ad_campaigns
		WHERE event_id = $1
		  AND status IN ('active','paused','draft','exhausted')
	`, eventID).Scan(&existing)
	if existing > 0 {
		return nil, ErrEventAlreadyBoosted
	}

	wallet, _ := GetOrCreateWallet(pool, organizerID)
	initialStatus := models.CampaignActive
	if wallet == nil || wallet.Balance < dto.DailyBudget {
		initialStatus = models.CampaignDraft
	}

	startDate := time.Now()
	if dto.StartDate != nil {
		startDate = *dto.StartDate
	}

	var c models.AdCampaign
	err = tx.QueryRow(ctx, `
		INSERT INTO ad_campaigns
		    (event_id, organizer_id, daily_budget, cpm_rate, start_date, end_date, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, event_id, organizer_id, daily_budget, cpm_rate,
		          total_spent, total_impressions, total_clicks,
		          start_date, end_date, status, created_at, updated_at
	`, eventID, organizerID, dto.DailyBudget, DefaultCPMRate,
		startDate.Format("2006-01-02"), dto.EndDate, string(initialStatus),
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
// Only paused ↔ active transitions are permitted via this endpoint.
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
		SET    daily_budget = $1,
		       end_date     = $2,
		       status       = $3,
		       updated_at   = NOW()
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
		  AND  status != 'ended'
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

// SponsoredEvent wraps an Event with the campaign metadata needed to record
// an impression after the feed is served. These fields are never serialised
// to the client — they are internal to the feed assembly layer.
type SponsoredEvent struct {
	models.Event
	CampaignID        string  `json:"-"`
	CostPerImpression float64 `json:"-"`
}

// GetSponsoredForFeed returns the top N campaigns eligible to serve right now.
//
// Campaigns are ranked by:
//  1. daily_budget × affinity_multiplier  (primary: more spend = higher rank)
//  2. affinity_multiplier = 1 + min(prior bookings in category, 3) × 0.2
//     (a user who has booked 2 music events gets a 1.4× boost for music campaigns)
//  3. views DESC (tiebreaker)
//
// Campaigns are excluded when:
//   - status != 'active'
//   - today's amount_spent >= daily_budget
//   - wallet balance is 0
//   - event start_time has passed
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
		JOIN   ad_wallets    w  ON w.organizer_id = ac.organizer_id
		LEFT JOIN ad_daily_spend ds
		          ON ds.campaign_id = ac.id AND ds.spend_date = CURRENT_DATE
		LEFT JOIN user_cats uc ON uc.category_id = ev.category_id
		WHERE  ac.status     = 'active'
		  AND  ev.status     = 'published'
		  AND  ev.start_time > NOW()
		  AND  (ac.end_date IS NULL OR ac.end_date >= CURRENT_DATE)
		  AND  COALESCE(ds.amount_spent, 0) < ac.daily_budget
		  AND  w.balance > 0
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

// RecordImpressions charges each shown campaign one impression and deducts
// the cost from the organiser's wallet. Called asynchronously after feed
// assembly so it never blocks the HTTP response.
//
// Each call handles one impression per campaign — deduplicated per feed render.
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
			),
			update_campaign AS (
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
			),
			deduct AS (
				UPDATE ad_wallets w
				SET    balance     = balance     - $2,
				       total_spent = total_spent + $2,
				       updated_at  = NOW()
				FROM   ad_campaigns ac
				WHERE  ac.id = $1
				  AND  w.organizer_id = ac.organizer_id
				  AND  w.balance >= $2
				RETURNING w.balance, w.organizer_id
			)
			UPDATE ad_campaigns
			SET    status = 'exhausted', updated_at = NOW()
			WHERE  organizer_id = (SELECT organizer_id FROM deduct)
			  AND  status = 'active'
			  AND  (SELECT balance FROM deduct) <= 0
		`, s.CampaignID, cost)

		if err != nil {
			log.Error().Err(err).Str("campaign_id", s.CampaignID).
				Msg("ad: failed to record impression")
		}
	}
}

// RecordClick increments click count for a campaign when a user navigates
// from a sponsored card to the event detail page.
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

// InjectPreFetched weaves already-fetched sponsored events into an organic
// slice without touching the database. Use this when the caller has already
// called GetSponsoredForFeed once and wants to reuse the result across
// multiple sections (e.g. GetHomeSections) to avoid redundant queries.
//
// Ad positions and interleaving logic are identical to InjectSponsored.
// Impression recording is the caller's responsibility.
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
//
// Insertion positions (0-indexed): 2, 6  (maxAds=2)
// This mirrors Facebook's approach: never front-load ads, place them at
// predictable intervals within the feed.
//
// Impressions are recorded asynchronously — this function never blocks.
func InjectSponsored(pool *pgxpool.Pool, organic []models.Event, userID *string, maxAds int) []models.Event {
	if len(organic) == 0 || maxAds == 0 {
		return organic
	}

	sponsored, err := GetSponsoredForFeed(pool, userID, maxAds)
	if err != nil || len(sponsored) == 0 {
		return organic
	}

	// Ad positions (0-indexed): 2, 6, 10 ...
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

// ── Nightly maintenance ───────────────────────────────────────────────────────

// ResumeExhaustedCampaigns reactivates campaigns that paused because they
// hit their daily budget. Called at midnight by the ad_worker.
// Campaigns paused because the wallet was empty remain paused.
func ResumeExhaustedCampaigns(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ad_campaigns ac
		SET    status     = 'active',
		       updated_at = NOW()
		FROM   ad_wallets w
		WHERE  w.organizer_id = ac.organizer_id
		  AND  ac.status      = 'exhausted'
		  AND  (ac.end_date IS NULL OR ac.end_date >= CURRENT_DATE)
		  AND  w.balance > 0
	`)
	return err
}

// EndExpiredCampaigns closes campaigns whose end_date has passed.
func EndExpiredCampaigns(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ad_campaigns
		SET    status     = 'ended',
		       updated_at = NOW()
		WHERE  status  IN ('active','paused','exhausted','draft')
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