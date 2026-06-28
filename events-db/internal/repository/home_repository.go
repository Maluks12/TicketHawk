package repository

import (
	"context"
	"events-db/internal/models"
	"events-db/internal/utils"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type HomeSection struct {
	ID         string         `json:"id"`
	Title      string         `json:"title"`
	Events     []models.Event `json:"events"`
	NextCursor string         `json:"next_cursor"`
}

type HomeSectionsResult struct {
	Sections []HomeSection `json:"sections"`
}

// GetHomeSections assembles the full homepage feed in parallel and injects
// sponsored events into every section before returning.
//
// Sections always present:
//   - Trending Now    (time-decayed view score)
//   - Happening Soon  (soonest upcoming)
//   - This Weekend    (this Sat/Sun)
//   - Recently Added  (newest published)
//   - Featured Events (is_featured = true)
//
// Conditional sections:
//   - For You   → injected first when userID is provided (auth users)
//   - Near You  → injected last when lat/lng are provided
//
// Each section receives up to 2 sponsored cards at positions 3 and 7
// (1-indexed). Sponsored ranking weights daily_budget × category affinity,
// so ads relevant to the user rank higher regardless of raw budget alone.
func GetHomeSections(pool *pgxpool.Pool, userID *string, city *string) (*HomeSectionsResult, error) {
	// Composed of the shared (cacheable) sections plus the personalised ones.
	// Retained for any direct callers; the home handler caches the shared part
	// separately and merges the personalised sections on top.
	shared, err := SharedHomeSections(pool)
	if err != nil {
		return nil, err
	}
	forYou, nearYou := PersonalHomeSections(pool, userID, city)

	out := make([]HomeSection, 0, len(forYou)+len(shared.Sections)+len(nearYou))
	out = append(out, forYou...)
	out = append(out, shared.Sections...)
	out = append(out, nearYou...)
	return &HomeSectionsResult{Sections: out}, nil
}

// sectionFetcher pairs a section's identity with the query that fills it.
type sectionFetcher struct {
	id    string
	title string
	fetch func() ([]models.Event, *utils.Cursor, error)
}

// runSections executes the given section fetchers in parallel, weaves in
// sponsored cards (fetched once, ranked for sponsoredUserID), drops sections
// that errored or came back empty, and preserves the input order. Sponsored
// impressions are recorded once for the cards actually shown.
func runSections(pool *pgxpool.Pool, fetchers []sectionFetcher, sponsoredUserID *string) []HomeSection {
	if len(fetchers) == 0 {
		return nil
	}

	type result struct {
		section HomeSection
		err     error
	}
	results := make([]result, len(fetchers))
	var wg sync.WaitGroup

	// Pre-fetch sponsored (paid) ad cards ONCE and share across all sections.
	// Admin-promoted events are NOT injected here — they appear organically in
	// the normal rows (via the admin_promoted eligibility gate), not as cards.
	const maxAdsPerSection = 2
	sponsored, _ := GetSponsoredForFeed(pool, sponsoredUserID, maxAdsPerSection)

	for i, f := range fetchers {
		wg.Add(1)
		go func(idx int, sf sectionFetcher) {
			defer wg.Done()
			events, nextCursor, err := sf.fetch()
			if err != nil {
				results[idx] = result{err: err}
				return
			}
			events = InjectPreFetched(events, sponsored, maxAdsPerSection)
			results[idx] = result{
				section: HomeSection{
					ID:         sf.id,
					Title:      sf.title,
					Events:     events,
					NextCursor: utils.EncodeCursor(nextCursor),
				},
			}
		}(i, f)
	}
	wg.Wait()

	if len(sponsored) > 0 {
		go RecordImpressions(pool, sponsored)
	}

	out := make([]HomeSection, 0, len(fetchers))
	for _, r := range results {
		if r.err != nil || len(r.section.Events) == 0 {
			continue
		}
		out = append(out, r.section)
	}
	return out
}

// SharedHomeSections returns the non-personalised feed sections. These are
// identical for every visitor, so the handler caches them globally and reuses
// them for signed-in users too — only the personalised sections are computed
// fresh per request.
func SharedHomeSections(pool *pgxpool.Pool) (*HomeSectionsResult, error) {
	fetchers := []sectionFetcher{
		{
			id:    "trending",
			title: "Trending Now",
			fetch: func() ([]models.Event, *utils.Cursor, error) { return TrendingNow(pool, 10) },
		},
		{
			id:    "happening_soon",
			title: "Happening Soon",
			fetch: func() ([]models.Event, *utils.Cursor, error) { return HappeningSoon(pool, 10, nil) },
		},
		{
			id:    "this_weekend",
			title: "This Weekend",
			fetch: func() ([]models.Event, *utils.Cursor, error) { return ThisWeekend(pool, 10, nil) },
		},
		{
			id:    "recently_added",
			title: "Recently Added",
			fetch: func() ([]models.Event, *utils.Cursor, error) { return RecentlyAdded(pool, 10, nil) },
		},
		{
			id:    "featured",
			title: "Featured Events",
			fetch: func() ([]models.Event, *utils.Cursor, error) { return FeaturedEvents(pool, 10, nil) },
		},
	}
	return &HomeSectionsResult{Sections: runSections(pool, fetchers, nil)}, nil
}

// PersonalHomeSections returns only the user-specific sections:
//   - forYou  (0 or 1 section) when userID is set
//   - nearYou (0 or 1 section) when city is set
//
// They are returned separately so the caller can place "For You" first and
// "Near You" last, around the shared sections. Each is dropped if it comes back
// empty, matching the shared-section behaviour.
func PersonalHomeSections(pool *pgxpool.Pool, userID *string, city *string) (forYou, nearYou []HomeSection) {
	var fetchers []sectionFetcher

	if userID != nil {
		uid := *userID
		fetchers = append(fetchers, sectionFetcher{
			id:    "for_you",
			title: "For You",
			fetch: func() ([]models.Event, *utils.Cursor, error) {
				events, err := ForYou(pool, uid, 10)
				return events, nil, err
			},
		})
	}
	if city != nil && *city != "" {
		ct := *city
		fetchers = append(fetchers, sectionFetcher{
			id:    "near_you",
			title: "Near You",
			fetch: func() ([]models.Event, *utils.Cursor, error) {
				events, err := NearYou(pool, ct, 10)
				return events, nil, err
			},
		})
	}
	if len(fetchers) == 0 {
		return nil, nil
	}

	for _, sec := range runSections(pool, fetchers, userID) {
		switch sec.ID {
		case "for_you":
			forYou = append(forYou, sec)
		case "near_you":
			nearYou = append(nearYou, sec)
		}
	}
	return forYou, nearYou
}

// ── For You ───────────────────────────────────────────────────────────────────
//
// Personalised ranking formula per event:
//
//	score = (bookings_in_this_category × 50)
//	      + (min(views, 10000) × 0.05)
//	      + soon_bonus (30 if within 7 days, 10 if within 30 days)
//
// Events already purchased (non-cancelled/refunded) are excluded.
// For users with no booking history the formula degrades to popularity+recency.
func ForYou(pool *pgxpool.Pool, userID string, limit int) ([]models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		WITH user_cats AS (
			SELECT e.category_id, COUNT(*) AS cnt
			FROM   bookings b
			JOIN   events   e ON b.event_id = e.id
			WHERE  b.user_id        = $1
			  AND  b.booking_status IN ('confirmed','pending')
			GROUP BY e.category_id
		)
		SELECT
			ev.id, COALESCE(ev.slug, '') AS slug, ev.title, ev.description,
			ev.cover_image_url, ev.banner_image_url,
			ev.start_time, ev.end_time, ev.price, ev.currency,
			ev.city, ev.venue_name, ev.max_capacity, ev.tickets_sold,
			ev.is_featured, ev.status, ev.views, ev.created_at,
			c.name AS category
		FROM   events     ev
		JOIN   categories c   ON c.id  = ev.category_id
		LEFT JOIN user_cats uc ON uc.category_id = ev.category_id
		WHERE  ev.status          = 'published'
		  AND  (ev.price > 0 OR ev.listing_fee_paid = TRUE OR ev.admin_promoted = TRUE)
		  AND  ev.start_time > NOW()
		  AND NOT EXISTS (
			  SELECT 1 FROM bookings b2
			  WHERE  b2.event_id       = ev.id
			    AND  b2.user_id        = $1
			    AND  b2.booking_status NOT IN ('cancelled','refunded')
		  )
		ORDER BY (
			COALESCE(uc.cnt, 0) * 50
			+ LEAST(ev.views, 10000) * 0.05
			+ CASE
				WHEN ev.start_time < NOW() + INTERVAL '7 days'  THEN 30
				WHEN ev.start_time < NOW() + INTERVAL '30 days' THEN 10
				ELSE 0
			  END
		) DESC,
		ev.start_time ASC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("ForYou: %w", err)
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit)
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Slug, &e.Title, &e.Description,
			&e.CoverImageURL, &e.BannerImageURL,
			&e.StartTime, &e.EndTime, &e.Price, &e.Currency,
			&e.City, &e.VenueName, &e.MaxCapacity, &e.TicketsSold,
			&e.IsFeatured, &e.Status, &e.Views, &e.CreatedAt,
			&e.Category,
		); err != nil {
			return nil, fmt.Errorf("ForYou scan: %w", err)
		}
		e.ComputeAvailableTickets()
		events = append(events, e)
	}
	return events, rows.Err()
}

// ── Near You (city-based) ─────────────────────────────────────────────────────
//
// Returns events whose city matches the user's selected city or any known
// suburb/district in the same metro. No lat/lng or PostGIS required.
func NearYou(pool *pgxpool.Pool, city string, limit int) ([]models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cities := cityAndNeighbours(city)

	rows, err := pool.Query(ctx, `
		SELECT
			ev.id, COALESCE(ev.slug, '') AS slug, ev.title, ev.description,
			ev.cover_image_url, ev.banner_image_url,
			ev.start_time, ev.end_time, ev.price, ev.currency,
			ev.city, ev.venue_name, ev.max_capacity, ev.tickets_sold,
			ev.is_featured, ev.status, ev.views, ev.created_at,
			c.name AS category
		FROM events ev
		JOIN categories c ON c.id = ev.category_id
		WHERE ev.status          = 'published'
		  AND (ev.price > 0 OR ev.listing_fee_paid = TRUE OR ev.admin_promoted = TRUE)
		  AND ev.start_time > NOW()
		  AND LOWER(ev.city) = ANY($1)
		ORDER BY ev.start_time ASC, ev.views DESC
		LIMIT $2
	`, cities, limit)
	if err != nil {
		return nil, fmt.Errorf("NearYou: %w", err)
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit)
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Slug, &e.Title, &e.Description,
			&e.CoverImageURL, &e.BannerImageURL,
			&e.StartTime, &e.EndTime, &e.Price, &e.Currency,
			&e.City, &e.VenueName, &e.MaxCapacity, &e.TicketsSold,
			&e.IsFeatured, &e.Status, &e.Views, &e.CreatedAt,
			&e.Category,
		); err != nil {
			return nil, fmt.Errorf("NearYou scan: %w", err)
		}
		e.ComputeAvailableTickets()
		events = append(events, e)
	}
	return events, rows.Err()
}

// cityAndNeighbours returns lowercase city + known suburb names for SA metros.
func cityAndNeighbours(city string) []string {
	lower := strings.ToLower(strings.TrimSpace(city))
	metros := map[string][]string{
		"johannesburg": {
			"johannesburg", "sandton", "midrand", "soweto", "roodepoort",
			"randburg", "fourways", "maboneng", "braamfontein", "melville",
			"rosebank", "illovo", "newtown", "doornfontein", "sun city",
		},
		"cape town": {
			"cape town", "sea point", "green point", "waterfront",
			"newlands", "claremont", "stellenbosch", "franschhoek", "paarl",
		},
		"durban": {
			"durban", "umhlanga", "ballito", "pinetown", "westville",
		},
		"pretoria": {
			"pretoria", "centurion", "hatfield", "arcadia", "menlyn",
		},
	}
	if neighbours, ok := metros[lower]; ok {
		return neighbours
	}
	return []string{lower}
}

// ── Trending Now ──────────────────────────────────────────────────────────────
//
// Hacker News-style time-decay: score = views / (age_hours + 2)^1.5
// Prevents stale events from permanently holding top slots due to high
// accumulated views. Used only in the home feed; the standalone
// /events/trending endpoint keeps its existing raw-views sort.
func TrendingNow(pool *pgxpool.Pool, limit int) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT
			e.id, COALESCE(e.slug, '') AS slug, e.title, e.description,
			e.cover_image_url, e.banner_image_url,
			e.start_time, e.end_time, e.price, e.currency,
			e.city, e.venue_name, e.max_capacity, e.tickets_sold,
			e.is_featured, e.status, e.views, e.created_at,
			c.name AS category
		FROM   events     e
		JOIN   categories c ON c.id = e.category_id
		WHERE  e.status          = 'published'
		  AND  (e.price > 0 OR e.listing_fee_paid = TRUE OR e.admin_promoted = TRUE)
		  AND  e.start_time > NOW()
		ORDER BY (
			e.views::float
			/ POWER(
				GREATEST(EXTRACT(EPOCH FROM (NOW() - e.created_at)) / 3600.0, 1.0) + 2.0,
				1.5
			)
		) DESC,
		e.start_time ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("TrendingNow: %w", err)
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit)
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Slug, &e.Title, &e.Description,
			&e.CoverImageURL, &e.BannerImageURL,
			&e.StartTime, &e.EndTime, &e.Price, &e.Currency,
			&e.City, &e.VenueName, &e.MaxCapacity, &e.TicketsSold,
			&e.IsFeatured, &e.Status, &e.Views, &e.CreatedAt,
			&e.Category,
		); err != nil {
			return nil, nil, fmt.Errorf("TrendingNow scan: %w", err)
		}
		e.ComputeAvailableTickets()
		events = append(events, e)
	}
	return events, nil, rows.Err()
}

// ── Recently Added ────────────────────────────────────────────────────────────

func RecentlyAdded(pool *pgxpool.Pool, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished}
	where := "WHERE e.status = $1 AND (e.price > 0 OR e.listing_fee_paid = TRUE OR e.admin_promoted = TRUE)"

	if cursor != nil {
		where += ` AND (e.created_at < $2 OR (e.created_at = $2 AND e.id < $3))`
		args = append(args, cursor.CreatedAt, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			e.id, COALESCE(e.slug, '') AS slug, e.title, e.description,
			e.cover_image_url, e.banner_image_url,
			e.start_time, e.end_time, e.price, e.currency,
			e.city, e.venue_name, e.max_capacity, e.tickets_sold,
			e.is_featured, e.status, e.views, e.created_at,
			c.name AS category
		FROM   events     e
		JOIN   categories c ON c.id = e.category_id
		%s
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $%d
	`, where, limitPlaceholder)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit+1)
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Slug, &e.Title, &e.Description,
			&e.CoverImageURL, &e.BannerImageURL,
			&e.StartTime, &e.EndTime, &e.Price, &e.Currency,
			&e.City, &e.VenueName, &e.MaxCapacity, &e.TicketsSold,
			&e.IsFeatured, &e.Status, &e.Views, &e.CreatedAt,
			&e.Category,
		); err != nil {
			return nil, nil, err
		}
		e.ComputeAvailableTickets()
		events = append(events, e)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		events = events[:limit]
	}
	return events, nextCursor, nil
}

// ── Happening Soon ────────────────────────────────────────────────────────────

func HappeningSoon(pool *pgxpool.Pool, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished, time.Now()}
	where := `WHERE e.status = $1 AND (e.price > 0 OR e.listing_fee_paid = TRUE OR e.admin_promoted = TRUE) AND e.start_time > $2`

	if cursor != nil {
		where += ` AND (e.start_time > $3 OR (e.start_time = $3 AND e.id > $4))`
		args = append(args, cursor.StartTime, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			e.id, COALESCE(e.slug, '') AS slug, e.title, e.description,
			e.cover_image_url, e.banner_image_url,
			e.start_time, e.end_time, e.price, e.currency,
			e.city, e.venue_name, e.max_capacity, e.tickets_sold,
			e.is_featured, e.status, e.views, e.created_at,
			c.name AS category
		FROM   events     e
		JOIN   categories c ON c.id = e.category_id
		%s
		ORDER BY e.start_time ASC, e.id ASC
		LIMIT $%d
	`, where, limitPlaceholder)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit+1)
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Slug, &e.Title, &e.Description,
			&e.CoverImageURL, &e.BannerImageURL,
			&e.StartTime, &e.EndTime, &e.Price, &e.Currency,
			&e.City, &e.VenueName, &e.MaxCapacity, &e.TicketsSold,
			&e.IsFeatured, &e.Status, &e.Views, &e.CreatedAt,
			&e.Category,
		); err != nil {
			return nil, nil, err
		}
		e.ComputeAvailableTickets()
		events = append(events, e)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{StartTime: last.StartTime, ID: last.ID}
		events = events[:limit]
	}
	return events, nextCursor, nil
}

// ── This Weekend ──────────────────────────────────────────────────────────────
// Fixed: Saturday no longer skips to the next weekend.
//
//	Sat → window = today..Monday midnight
//	Sun → window = today..Monday midnight
//	Mon-Fri → window = coming Saturday..Monday midnight
func ThisWeekend(pool *pgxpool.Pool, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := saNow()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var weekendStart, weekendEnd time.Time
	switch now.Weekday() {
	case time.Saturday:
		weekendStart = today
		weekendEnd = today.AddDate(0, 0, 2)
	case time.Sunday:
		weekendStart = today
		weekendEnd = today.AddDate(0, 0, 1)
	default:
		daysUntilSat := int(time.Saturday) - int(now.Weekday())
		weekendStart = today.AddDate(0, 0, daysUntilSat)
		weekendEnd = weekendStart.AddDate(0, 0, 2)
	}

	args := []interface{}{models.EventPublished, weekendStart, weekendEnd}
	where := `
		WHERE e.status          = $1
		  AND (e.price > 0 OR e.listing_fee_paid = TRUE OR e.admin_promoted = TRUE)
		  AND e.start_time >= $2
		  AND e.start_time <  $3
	`

	if cursor != nil {
		where += ` AND (e.start_time > $4 OR (e.start_time = $4 AND e.id > $5))`
		args = append(args, cursor.StartTime, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			e.id, COALESCE(e.slug, '') AS slug, e.title, e.description,
			e.cover_image_url, e.banner_image_url,
			e.start_time, e.end_time, e.price, e.currency,
			e.city, e.venue_name, e.max_capacity, e.tickets_sold,
			e.is_featured, e.status, e.views, e.created_at,
			c.name AS category
		FROM   events     e
		JOIN   categories c ON c.id = e.category_id
		%s
		ORDER BY e.start_time ASC, e.id ASC
		LIMIT $%d
	`, where, limitPlaceholder)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit+1)
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Slug, &e.Title, &e.Description,
			&e.CoverImageURL, &e.BannerImageURL,
			&e.StartTime, &e.EndTime, &e.Price, &e.Currency,
			&e.City, &e.VenueName, &e.MaxCapacity, &e.TicketsSold,
			&e.IsFeatured, &e.Status, &e.Views, &e.CreatedAt,
			&e.Category,
		); err != nil {
			return nil, nil, err
		}
		e.ComputeAvailableTickets()
		events = append(events, e)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{StartTime: last.StartTime, ID: last.ID}
		events = events[:limit]
	}
	return events, nextCursor, nil
}