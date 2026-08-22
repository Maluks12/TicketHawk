package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Candidate detection ──────────────────────────────────────────────────────

// LineupWatchCandidate is a live event whose line-up may have moved since the
// last announcement.
type LineupWatchCandidate struct {
	EventID    string
	EventSlug  string
	EventTitle string
	Announced  models.LineupSnapshot
	Current    models.LineupSnapshot
	// NeverAnnounced is true when the event has no snapshot at all — it was
	// published after this feature shipped, so its line-up was assembled while
	// it was still a draft and nobody has been told anything about it yet.
	//
	// Without this flag such an event's entire line-up reads as "added", and
	// every follower gets a "these acts have been added" push moments after the
	// phase 2 "new event announced" push for the same event. The worker seeds
	// the snapshot silently instead.
	NeverAnnounced bool
}

// FindChangedLineups returns live, upcoming events whose line-up NAMES differ
// from what was last announced.
//
// ── Why the comparison happens in Go, not in SQL ────────────────────────────
//
// The snapshot is JSONB and a `!=` against a freshly aggregated one would work,
// but it would be comparing serialised forms: a different key order, a
// different sort, or a capitalisation edit would all read as "changed". The
// diff rules that matter — case-insensitive, order-insensitive, names-only —
// are the product decision, and they belong somewhere readable and testable
// rather than buried in a jsonb expression.
//
// So SQL does the cheap part (which events are live and upcoming, with their
// current names) and DiffSnapshots does the judgement.
//
// `settle` keeps a half-built line-up quiet. An organiser adding acts one at a
// time would otherwise fire a notification per save; requiring the line-up to
// have been still for a few minutes collapses that into one.
func FindChangedLineups(
	pool *pgxpool.Pool,
	ctx context.Context,
	settle time.Duration,
	limit int,
) ([]LineupWatchCandidate, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := pool.Query(ctx, `
		SELECT e.id, e.slug, e.title,
		       (e.lineup_announced_snapshot IS NULL) AS never_announced,
		       COALESCE(e.lineup_announced_snapshot, '{}'::jsonb),
		       COALESCE(
		         (
		           SELECT jsonb_object_agg(kind, names)
		           FROM (
		             SELECT l.kind, jsonb_agg(l.name ORDER BY l.name) AS names
		             FROM   event_lineup l
		             WHERE  l.event_id = e.id
		             GROUP  BY l.kind
		           ) g
		         ),
		         '{}'::jsonb
		       )
		FROM   events e
		WHERE  e.status IN ('published', 'published_private')
		  -- A line-up change on a finished event is a records correction.
		  -- Nobody needs a push about an act that did not play last month.
		  AND  e.end_time >= NOW()
		  -- Let the line-up settle before announcing, so building it entry by
		  -- entry produces one notification rather than eight.
		  AND  COALESCE(e.updated_at, e.created_at) < NOW() - $1::interval
		  -- Never more than one line-up notification per event per hour, no
		  -- matter how much editing happens.
		  AND  (e.lineup_notified_at IS NULL OR e.lineup_notified_at < NOW() - INTERVAL '1 hour')
		ORDER BY e.updated_at
		LIMIT $2
	`, fmt.Sprintf("%d seconds", int(settle.Seconds())), limit)
	if err != nil {
		return nil, fmt.Errorf("FindChangedLineups: %w", err)
	}
	defer rows.Close()

	out := make([]LineupWatchCandidate, 0, limit)
	for rows.Next() {
		var (
			c                 LineupWatchCandidate
			announcedRaw, cur []byte
		)
		if err := rows.Scan(&c.EventID, &c.EventSlug, &c.EventTitle, &c.NeverAnnounced, &announcedRaw, &cur); err != nil {
			return nil, fmt.Errorf("FindChangedLineups scan: %w", err)
		}
		if err := json.Unmarshal(announcedRaw, &c.Announced); err != nil {
			c.Announced = models.LineupSnapshot{}
		}
		if err := json.Unmarshal(cur, &c.Current); err != nil {
			c.Current = models.LineupSnapshot{}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MarkLineupAnnounced records what was just announced.
//
// Written AFTER the notifications go out, unlike the claim in
// ClaimPendingAnnouncements which is written before.
//
// The two are opposite on purpose. A new-event announcement is one shot: if it
// half-fails, re-sending duplicates a push to everyone who already got it, so
// claiming first and losing an announcement is the safer failure. A line-up
// change is a DIFF: if it half-fails, the snapshot still holds the old names,
// so the next cycle recomputes the same diff and finishes the job. Recording
// first would mark the change announced to people who were never told.
func MarkLineupAnnounced(
	pool *pgxpool.Pool,
	ctx context.Context,
	eventID string,
	snapshot models.LineupSnapshot,
) error {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("MarkLineupAnnounced: encode: %w", err)
	}

	// updated_at is deliberately NOT touched. Bumping it would restart the
	// settle window this event just cleared, and two events saved together
	// would keep pushing each other's windows forward.
	if _, err := pool.Exec(ctx, `
		UPDATE events
		SET    lineup_announced_snapshot = $2,
		       lineup_notified_at = NOW()
		WHERE  id = $1
	`, eventID, raw); err != nil {
		return fmt.Errorf("MarkLineupAnnounced: %w", err)
	}
	return nil
}

// SeedLineupSnapshot records the current line-up as already-announced WITHOUT
// notifying anyone.
//
// Called when an event is first published. Its line-up was assembled while the
// event was a draft, so nobody has been told anything about it yet and there is
// nothing to have changed — announcing it as a "change" on day one would be
// nonsense. The new-event announcement in phase 2 is what covers publication.
func SeedLineupSnapshot(pool *pgxpool.Pool, ctx context.Context, eventID string) error {
	if _, err := pool.Exec(ctx, `
		UPDATE events e
		SET    lineup_announced_snapshot = COALESCE(
		         (
		           SELECT jsonb_object_agg(kind, names)
		           FROM (
		             SELECT l.kind, jsonb_agg(l.name ORDER BY l.name) AS names
		             FROM   event_lineup l
		             WHERE  l.event_id = e.id
		             GROUP  BY l.kind
		           ) g
		         ),
		         '{}'::jsonb
		       ),
		       lineup_notified_at = NOW()
		WHERE  e.id = $1
	`, eventID); err != nil {
		return fmt.Errorf("SeedLineupSnapshot: %w", err)
	}
	return nil
}

// ── Audience ─────────────────────────────────────────────────────────────────

// ListEventTicketHolderIDs returns the users who hold a confirmed booking.
//
// Only CONFIRMED bookings. A pending booking is someone mid-checkout who has
// not paid, and a refunded or cancelled one is someone with no stake left — in
// both cases a "the line-up changed" push is noise about an event they are not
// going to.
//
// Guest bookings have a NULL user_id and are excluded: there is no account to
// notify. Reaching them means email, which is a bigger change than this — noted
// in the README as an open gap rather than papered over here.
func ListEventTicketHolderIDs(
	pool *pgxpool.Pool,
	ctx context.Context,
	eventID string,
	afterUserID *string,
	limit int,
) ([]string, error) {
	if limit <= 0 {
		limit = 500
	}

	rows, err := pool.Query(ctx, `
		SELECT DISTINCT user_id
		FROM   bookings
		WHERE  event_id = $1
		  AND  user_id IS NOT NULL
		  AND  booking_status = 'confirmed'
		  AND  ($2::uuid IS NULL OR user_id > $2::uuid)
		ORDER  BY user_id
		LIMIT  $3
	`, eventID, afterUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("ListEventTicketHolderIDs: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("ListEventTicketHolderIDs scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GetEventOrganizerUserID returns the user id that owns an event.
//
// events.organizer_id is a USER id — the column is `REFERENCES users(id)`
// despite its name. See GetPublicOrganizerProfileByUserID for the full warning.
func GetEventOrganizerUserID(pool *pgxpool.Pool, ctx context.Context, eventID string) (string, error) {
	var uid string
	if err := pool.QueryRow(ctx,
		`SELECT organizer_id FROM events WHERE id = $1`, eventID,
	).Scan(&uid); err != nil {
		return "", fmt.Errorf("GetEventOrganizerUserID: %w", err)
	}
	return uid, nil
}