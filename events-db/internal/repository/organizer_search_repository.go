package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SearchPublicOrganizers finds organisers by name for the app's search screen.
//
// ── Why ILIKE and not the full-text machinery events use ────────────────────
//
// events.search_vector exists because an event has a title, a description, a
// venue and tags to weigh against each other. An organiser has one field worth
// searching: their business name. Building a tsvector over a single short
// column would add a trigger, an index and a migration to answer a question a
// prefix match answers correctly.
//
// The pattern is anchored two ways and ranked so the better match wins:
//   1. name starts with the query   — "sowe" → "Soweto Arts Trust"
//   2. name contains the query      — "arts" → "Soweto Arts Trust"
//
// A leading-wildcard ILIKE cannot use a btree index, but the organizers table
// is in the low thousands at most — this is a sequential scan over a few
// thousand short strings, which is microseconds. If it ever stops being that,
// pg_trgm with a GIN index is the fix, not tsvector.
//
// ── The gate ────────────────────────────────────────────────────────────────
//
// Identical to GetPublicOrganizerProfile: kyc_level >= 2, page enabled, account
// in good standing. An organiser whose page 404s must not be findable in search
// either — surfacing a result that leads to a dead end is worse than no result.
//
// status::text, NOT a bare enum comparison — organizer_status has no 'banned'
// or 'active' label despite both existing as Go constants. See migration
// 000077 and the warning on models.OrganizerStatus.
func SearchPublicOrganizers(
	pool *pgxpool.Pool,
	query string,
	limit int,
) ([]models.PublicOrganizer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return []models.PublicOrganizer{}, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 8
	}

	// Escape LIKE metacharacters so a query of "100%" searches for the literal
	// string rather than matching everything.
	esc := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query)

	rows, err := pool.Query(ctx, `
		SELECT o.id, o.slug, o.business_name, o.description,
		       o.logo_url, o.cover_image_url, o.verified, o.created_at,
		       (SELECT COUNT(*) FROM events e
		         WHERE e.organizer_id = o.user_id
		           AND e.status = 'published'
		           AND e.visibility = 'public'
		           AND e.end_time >= NOW())                      AS upcoming,
		       (SELECT COUNT(*) FROM organizer_follows f
		         WHERE f.organizer_id = o.id)                    AS followers
		FROM organizers o
		WHERE o.public_profile_enabled = TRUE
		  AND o.kyc_level >= 2
		  AND o.status::text NOT IN ('suspended', 'banned', 'rejected')
		  AND o.business_name ILIKE '%' || $1 || '%' ESCAPE '\'
		ORDER BY
		  -- Prefix matches first: someone typing "sowe" means Soweto, not an
		  -- organiser with "sowe" buried mid-name.
		  (o.business_name ILIKE $1 || '%' ESCAPE '\') DESC,
		  -- Then organisers with something actually on sale, because a result
		  -- you can buy from is more useful than one you cannot.
		  (SELECT COUNT(*) FROM events e
		     WHERE e.organizer_id = o.user_id
		       AND e.status = 'published'
		       AND e.visibility = 'public'
		       AND e.end_time >= NOW()) DESC,
		  o.business_name ASC
		LIMIT $2
	`, esc, limit)
	if err != nil {
		return nil, fmt.Errorf("SearchPublicOrganizers: %w", err)
	}
	defer rows.Close()

	out := make([]models.PublicOrganizer, 0, limit)
	for rows.Next() {
		var (
			o         models.PublicOrganizer
			createdAt time.Time
			upcoming  int
		)
		if err := rows.Scan(
			&o.ID, &o.Slug, &o.BusinessName, &o.Description,
			&o.LogoURL, &o.CoverImageURL, &o.Verified, &createdAt,
			&upcoming, &o.FollowerCount,
		); err != nil {
			return nil, fmt.Errorf("SearchPublicOrganizers scan: %w", err)
		}
		o.MemberSince = createdAt.Year()
		// EventsHosted carries the UPCOMING count here rather than the lifetime
		// total. On a search row "3 events on sale" is a reason to tap;
		// "24 events hosted" is trivia. The profile endpoint still returns the
		// lifetime figure.
		o.EventsHosted = upcoming
		// Links are deliberately not loaded — a search row does not render
		// them, and fetching them would be N+1 across the result set.
		out = append(out, o)
	}
	return out, rows.Err()
}