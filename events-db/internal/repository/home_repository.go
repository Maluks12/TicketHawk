package repository

import (
	"context"
	"events-db/internal/models"
	"events-db/internal/utils"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type HomeSection struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Events []models.Event `json:"events"`
	NextCursor string `json:"next_cursor"` // potentially might need to add * for empty strings
}

type HomeSectionsResult struct {
	Sections  []HomeSection `json:"sections"`
}

func GetHomeSections(pool *pgxpool.Pool) (*HomeSectionsResult, error) {
	type sectionFetcher struct {
		id    string
		title string
		fetch func() ([]models.Event, *utils.Cursor, error)
	}

	fetchers := []sectionFetcher{
		{
			id:    "trending",
			title: "Trending Now",
			fetch: func() ([]models.Event, *utils.Cursor, error) {
				return TrendingEvents(pool, 10, nil)
			},
		},
		{
			id:    "featured",
			title: "Featured Events",
			fetch: func() ([]models.Event, *utils.Cursor, error) {
				return FeaturedEvents(pool, 10, nil)
			},
		},
		{
			id:    "happening_soon",
			title: "Happening Soon",
			fetch: func() ([]models.Event, *utils.Cursor, error) {
				return HappeningSoon(pool, 10, nil)
			},
		},
		{
			id:    "recently_added",
			title: "Recently Added",
			fetch: func() ([]models.Event, *utils.Cursor, error) {
				return RecentlyAdded(pool, 10, nil)
			},
		},
		{
			id:    "this_weekend",
			title: "This Weekend",
			fetch: func() ([]models.Event, *utils.Cursor, error) {
				return ThisWeekend(pool, 10, nil)
			},
		},
	}

	type result struct {
	section HomeSection
	err  error
}

	results := make([]result, len(fetchers))
	var wg sync.WaitGroup

	for i, f := range fetchers {
		wg.Add(1)
		go func(idx int, sf sectionFetcher) {
			defer wg.Done()
			events, nextCursor, err := sf.fetch()
			results[idx] = result{
				err: err,
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

	out := make([]HomeSection, 0, len(fetchers))
	for _, r := range results {
		if r.err != nil || len(r.section.Events) == 0 {
			continue
		}
		out = append(out, r.section)
	}

	return &HomeSectionsResult{Sections: out}, nil
}

func RecentlyAdded(pool *pgxpool.Pool, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished}
	where := "WHERE e.status = $1"

	if cursor != nil {
		where += `
			AND (
				e.created_at < $2
				OR (e.created_at = $2 AND e.id < $3)
			)
		`
		args = append(args, cursor.CreatedAt, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			e.id, e.title, e.description, e.cover_image_url,
			e.start_time, e.end_time, e.price, e.currency,
			e.city, e.venue_name, e.max_capacity, e.tickets_sold,
			e.is_featured, e.status, e.views, e.created_at,
			c.name AS category
		FROM events e
		JOIN categories c ON e.category_id = c.id
		%s
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $%d
	`, where, limitPlaceholder)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0 , limit+1)
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Title, &e.Description, &e.CoverImageURL,
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
		nextCursor = &utils.Cursor{
			CreatedAt: last.CreatedAt,
			ID:        last.ID,
		}
		events = events[:limit]
	}

	return events, nextCursor, nil
}

func HappeningSoon(pool *pgxpool.Pool, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished, time.Now()}
	where := `
		WHERE e.status = $1
		AND e.start_time > $2
	`

	if cursor != nil {
		// ASC order: next page means start_time is greater than the last seen,
		// or equal start_time with a greater id
		where += `
			AND (
				e.start_time > $3
				OR (e.start_time = $3 AND e.id > $4)
			)
		`
		args = append(args, cursor.StartTime, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			e.id, e.title, e.description, e.cover_image_url,
			e.start_time, e.end_time, e.price, e.currency,
			e.city, e.venue_name, e.max_capacity, e.tickets_sold,
			e.is_featured, e.status, e.views, e.created_at,
			c.name AS category
		FROM events e
		JOIN categories c ON e.category_id = c.id
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
			&e.ID, &e.Title, &e.Description, &e.CoverImageURL,
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
		nextCursor = &utils.Cursor{
			StartTime: last.StartTime,
			ID:        last.ID,
		}
		events = events[:limit]
	}

	return events, nextCursor, nil
}

func ThisWeekend(pool *pgxpool.Pool, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	daysUntilSat := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSat == 0 {
		daysUntilSat = 7 // already Saturday — skip to next weekend
	}
	weekendStart := time.Date(now.Year(), now.Month(), now.Day()+daysUntilSat, 0, 0, 0, 0, now.Location())
	weekendEnd := weekendStart.AddDate(0, 0, 2) // midnight Monday

	args := []interface{}{models.EventPublished, weekendStart, weekendEnd}
	where := `
		WHERE e.status = $1
		AND e.start_time >= $2
		AND e.start_time <  $3
	`

	if cursor != nil {
		where += `
			AND (
				e.start_time > $4
				OR (e.start_time = $4 AND e.id > $5)
			)
		`
		args = append(args, cursor.StartTime, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			e.id, e.title, e.description, e.cover_image_url,
			e.start_time, e.end_time, e.price, e.currency,
			e.city, e.venue_name, e.max_capacity, e.tickets_sold,
			e.is_featured, e.status, e.views, e.created_at,
			c.name AS category
		FROM events e
		JOIN categories c ON e.category_id = c.id
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
			&e.ID, &e.Title, &e.Description, &e.CoverImageURL,
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
		nextCursor = &utils.Cursor{
			StartTime: last.StartTime,
			ID:        last.ID,
		}
		events = events[:limit]
	}

	return events, nextCursor, nil
} // look at this more, there might be an issue that it skips the weekend if it is a saturday

