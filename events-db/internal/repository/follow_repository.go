package repository

import (
	"context"
	"errors"
	"events-db/internal/models"
	"events-db/internal/utils"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAlreadyFollowing = errors.New("already following this event")

// ToggleFollow adds or removes a follow for the given user+event pair.
// Returns the new following state and the total follow count for the event.
// It is safe to call concurrently — the INSERT uses ON CONFLICT DO NOTHING and
// the DELETE is idempotent.
func ToggleFollow(pool *pgxpool.Pool, userID, eventID string) (*models.FollowToggleResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Determine current state first.
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM event_follows WHERE user_id = $1 AND event_id = $2)`,
		userID, eventID,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("ToggleFollow: check exists: %w", err)
	}

	following := !exists // will be the new state after the toggle

	if exists {
		// Unfollow.
		if _, err := pool.Exec(ctx,
			`DELETE FROM event_follows WHERE user_id = $1 AND event_id = $2`,
			userID, eventID,
		); err != nil {
			return nil, fmt.Errorf("ToggleFollow: delete: %w", err)
		}
	} else {
		// Follow — ON CONFLICT DO NOTHING makes this safe against races.
		if _, err := pool.Exec(ctx,
			`INSERT INTO event_follows (user_id, event_id)
			 VALUES ($1, $2)
			 ON CONFLICT (user_id, event_id) DO NOTHING`,
			userID, eventID,
		); err != nil {
			return nil, fmt.Errorf("ToggleFollow: insert: %w", err)
		}
	}

	// Return the total follow count so the frontend can update its counter
	// without an extra round-trip.
	var count int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM event_follows WHERE event_id = $1`, eventID,
	).Scan(&count); err != nil {
		return nil, fmt.Errorf("ToggleFollow: count: %w", err)
	}

	return &models.FollowToggleResponse{
		Following:   following,
		FollowCount: count,
		EventID:     eventID,
	}, nil
}

// IsFollowing reports whether userID is following eventID.
func IsFollowing(pool *pgxpool.Pool, userID, eventID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM event_follows WHERE user_id = $1 AND event_id = $2)`,
		userID, eventID,
	).Scan(&exists)
	return exists, err
}

// GetFollowCount returns the total number of follows for an event.
func GetFollowCount(pool *pgxpool.Pool, eventID string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var count int64
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM event_follows WHERE event_id = $1`, eventID,
	).Scan(&count)
	return count, err
}

// GetFollowedEvents returns a cursor-paginated list of events followed by
// userID, newest follow first. Each row is a full event card suitable for
// rendering the "Following" feed directly.
func GetFollowedEvents(
	pool *pgxpool.Pool,
	userID string,
	limit int,
	cursor *utils.Cursor,
) ([]models.FollowedEventRow, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{userID}
	where := "WHERE f.user_id = $1"

	if cursor != nil {
		where += `
			AND (
				f.followed_at < $2
				OR (f.followed_at = $2 AND f.id < $3)
			)
		`
		args = append(args, cursor.CreatedAt, cursor.ID)
	}

	limitN := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			f.id             AS follow_id,
			f.followed_at,
			e.id             AS event_id,
			e.slug,
			e.title,
			e.cover_image_url,
			e.start_time,
			e.end_time,
			e.venue_name,
			e.city,
			e.price,
			e.currency,
			e.status,
			e.tickets_sold,
			e.max_capacity,
			c.name           AS category
		FROM event_follows f
		JOIN events e     ON e.id = f.event_id
		JOIN categories c ON c.id = e.category_id
		%s
		ORDER BY f.followed_at DESC, f.id DESC
		LIMIT $%d
	`, where, limitN)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("GetFollowedEvents: %w", err)
	}
	defer rows.Close()

	result := make([]models.FollowedEventRow, 0, limit)
	for rows.Next() {
		var r models.FollowedEventRow
		if err := rows.Scan(
			&r.FollowID, &r.FollowedAt,
			&r.EventID, &r.Slug, &r.Title, &r.CoverImageURL,
			&r.StartTime, &r.EndTime,
			&r.VenueName, &r.City,
			&r.Price, &r.Currency,
			&r.Status, &r.TicketsSold, &r.MaxCapacity,
			&r.Category,
		); err != nil {
			return nil, nil, fmt.Errorf("GetFollowedEvents scan: %w", err)
		}

		// Compute availability inline — mirrors Event.ComputeAvailableTickets.
		if r.MaxCapacity != nil {
			r.AvailableTickets = *r.MaxCapacity - r.TicketsSold
			if r.AvailableTickets < 0 {
				r.AvailableTickets = 0
			}
			r.IsSoldOut = r.AvailableTickets == 0
		} else {
			r.AvailableTickets = -1 // unlimited
		}

		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *utils.Cursor
	if len(result) > limit {
		last := result[limit-1]
		nextCursor = &utils.Cursor{
			CreatedAt: last.FollowedAt,
			ID:        last.FollowID,
		}
		result = result[:limit]
	}

	return result, nextCursor, nil
}

// ── Reminder helpers (used by the reminder worker) ────────────────────────────

// ReminderCandidate is a minimal row returned by GetFollowsNeedingReminder.
type ReminderCandidate struct {
	FollowID      string
	UserID        string
	EventID       string
	EventTitle    string
	EventSlug     string
	StartTime     time.Time
	DaysUntil     int    // 7 or 1 — tells the worker which flag to flip
	ReminderField string // "reminded_7_days" or "reminded_1_day" — used in the UPDATE
}

// GetFollowsNeedingReminder returns all event_follows rows where the event
// starts in approximately 7 days or 1 day and the corresponding reminder has
// not yet been sent.
//
// "Approximately" means within a ±6-hour window around the exact threshold so
// the worker does not need sub-hour precision and cron-drift doesn't cause
// missed reminders.
//
// Only published, non-cancelled events are included.
func GetFollowsNeedingReminder(pool *pgxpool.Pool) ([]ReminderCandidate, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT
			f.id,
			f.user_id,
			e.id,
			e.title,
			e.slug,
			e.start_time,
			CASE
				WHEN NOT f.reminded_7_days
				     AND e.start_time BETWEEN NOW() + INTERVAL '6.5 days'
				                          AND NOW() + INTERVAL '7.5 days'
				THEN 7
				WHEN NOT f.reminded_1_day
				     AND e.start_time BETWEEN NOW() + INTERVAL '18 hours'
				                          AND NOW() + INTERVAL '30 hours'
				THEN 1
			END AS days_until
		FROM event_follows f
		JOIN events e ON e.id = f.event_id
		WHERE e.status = 'published'
		  AND (
		        ( NOT f.reminded_7_days
		          AND e.start_time BETWEEN NOW() + INTERVAL '6.5 days'
		                               AND NOW() + INTERVAL '7.5 days' )
		    OR  ( NOT f.reminded_1_day
		          AND e.start_time BETWEEN NOW() + INTERVAL '18 hours'
		                               AND NOW() + INTERVAL '30 hours' )
		  )
	`)
	if err != nil {
		return nil, fmt.Errorf("GetFollowsNeedingReminder: %w", err)
	}
	defer rows.Close()

	var candidates []ReminderCandidate
	for rows.Next() {
		var c ReminderCandidate
		if err := rows.Scan(
			&c.FollowID, &c.UserID, &c.EventID,
			&c.EventTitle, &c.EventSlug, &c.StartTime, &c.DaysUntil,
		); err != nil {
			return nil, fmt.Errorf("GetFollowsNeedingReminder scan: %w", err)
		}
		if c.DaysUntil == 0 {
			continue // neither CASE branch matched — shouldn't happen given WHERE, but guard
		}
		// Derive the column name from DaysUntil so callers don't need a switch.
		switch c.DaysUntil {
		case 7:
			c.ReminderField = "reminded_7_days"
		case 1:
			c.ReminderField = "reminded_1_day"
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// MarkReminderSent flips the appropriate reminded_* column to TRUE for a
// specific follow row. The column name is validated against a fixed allowlist
// to prevent any SQL injection risk from the ReminderField string.
func MarkReminderSent(pool *pgxpool.Pool, followID, field string) error {
	// Allowlist check — only these two column names are valid.
	if field != "reminded_7_days" && field != "reminded_1_day" {
		return fmt.Errorf("MarkReminderSent: invalid field %q", field)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := fmt.Sprintf(
		`UPDATE event_follows SET %s = TRUE WHERE id = $1`, field,
	)
	_, err := pool.Exec(ctx, query, followID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("MarkReminderSent: %w", err)
	}
	return nil
}

// ── Sell-out notification helpers (used by the sell-out worker) ───────────────

// SellOutCandidate describes a published event that has crossed a capacity
// threshold and whose followers have not yet been notified.
type SellOutCandidate struct {
	EventID        string
	EventTitle     string
	EventSlug      string
	TicketsSold    int
	MaxCapacity    int
	Notify80Pct    bool // true → send "selling fast" notifications
	NotifySoldOut  bool // true → send "sold out" notifications
}

// GetSellOutCandidates returns published events with a finite capacity that
// have crossed the 80 % or 100 % sold threshold without a notification having
// been dispatched yet.
//
// It only returns events where at least one threshold is newly crossed so the
// worker does no work when there is nothing to do.
func GetSellOutCandidates(pool *pgxpool.Pool) ([]SellOutCandidate, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT
			id,
			title,
			COALESCE(slug, ''),
			tickets_sold,
			max_capacity,
			-- 80 % threshold: not yet notified AND sold >= 80 % of capacity
			(NOT notified_80pct AND tickets_sold::float / max_capacity >= 0.8)  AS notify_80pct,
			-- Sold-out threshold: not yet notified AND sold >= capacity
			(NOT notified_sold_out AND tickets_sold >= max_capacity)             AS notify_sold_out
		FROM events
		WHERE status        = 'published'
		  AND max_capacity  IS NOT NULL
		  AND (
		        (NOT notified_80pct    AND tickets_sold::float / max_capacity >= 0.8)
		     OR (NOT notified_sold_out AND tickets_sold >= max_capacity)
		      )
	`)
	if err != nil {
		return nil, fmt.Errorf("GetSellOutCandidates: %w", err)
	}
	defer rows.Close()

	var candidates []SellOutCandidate
	for rows.Next() {
		var c SellOutCandidate
		if err := rows.Scan(
			&c.EventID, &c.EventTitle, &c.EventSlug,
			&c.TicketsSold, &c.MaxCapacity,
			&c.Notify80Pct, &c.NotifySoldOut,
		); err != nil {
			return nil, fmt.Errorf("GetSellOutCandidates scan: %w", err)
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// GetEventFollowerIDs returns the user IDs of everyone currently following
// eventID. Used by the sell-out worker to fan out notifications.
func GetEventFollowerIDs(pool *pgxpool.Pool, eventID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx,
		`SELECT user_id FROM event_follows WHERE event_id = $1`, eventID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetEventFollowerIDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("GetEventFollowerIDs scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MarkEvent80PctNotified sets notified_80pct = TRUE for the given event so the
// worker never sends a second "selling fast" alert for the same event.
func MarkEvent80PctNotified(pool *pgxpool.Pool, eventID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx,
		`UPDATE events SET notified_80pct = TRUE WHERE id = $1`, eventID,
	)
	return err
}

// MarkEventSoldOutNotified sets notified_sold_out = TRUE for the given event.
func MarkEventSoldOutNotified(pool *pgxpool.Pool, eventID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx,
		`UPDATE events SET notified_sold_out = TRUE WHERE id = $1`, eventID,
	)
	return err
}