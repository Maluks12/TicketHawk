package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Toggle ───────────────────────────────────────────────────────────────────

// ToggleOrganizerFollow adds or removes a follow and returns the new state plus
// the organiser's total follower count.
//
// Done in ONE statement rather than the check-then-write that ToggleFollow uses
// for events. That older shape has a race: two taps arriving together both read
// "not following", both insert, and ON CONFLICT DO NOTHING silently swallows
// the second — so the user has toggled twice and is still following. Here the
// DELETE reports whether a row existed, and the INSERT only runs if it did not,
// with the outcome decided by what the database actually did.
func ToggleOrganizerFollow(pool *pgxpool.Pool, userID, organizerID string) (*models.OrganizerFollowToggleResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("ToggleOrganizerFollow: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	del, err := tx.Exec(ctx,
		`DELETE FROM organizer_follows WHERE user_id = $1 AND organizer_id = $2`,
		userID, organizerID)
	if err != nil {
		return nil, fmt.Errorf("ToggleOrganizerFollow: delete: %w", err)
	}

	following := del.RowsAffected() == 0
	if following {
		// Nothing was deleted, so the user was not following. ON CONFLICT is
		// still present as a backstop against a concurrent transaction that
		// committed an insert between our DELETE and this INSERT.
		if _, err := tx.Exec(ctx, `
			INSERT INTO organizer_follows (user_id, organizer_id)
			VALUES ($1, $2)
			ON CONFLICT (user_id, organizer_id) DO NOTHING
		`, userID, organizerID); err != nil {
			return nil, fmt.Errorf("ToggleOrganizerFollow: insert: %w", err)
		}
	}

	var (
		count int64
		slug  string
	)
	if err := tx.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM organizer_follows WHERE organizer_id = $1),
		       (SELECT slug FROM organizers WHERE id = $1)
	`, organizerID).Scan(&count, &slug); err != nil {
		return nil, fmt.Errorf("ToggleOrganizerFollow: count: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("ToggleOrganizerFollow: commit: %w", err)
	}

	return &models.OrganizerFollowToggleResponse{
		Following:     following,
		FollowerCount: count,
		OrganizerID:   organizerID,
		OrganizerSlug: slug,
	}, nil
}

// IsFollowingOrganizer reports whether userID follows organizerID.
func IsFollowingOrganizer(pool *pgxpool.Pool, userID, organizerID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM organizer_follows WHERE user_id = $1 AND organizer_id = $2)`,
		userID, organizerID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("IsFollowingOrganizer: %w", err)
	}
	return exists, nil
}

// GetOrganizerFollowerCount returns the organiser's follower total.
//
// Index-only scan over idx_organizer_follows_organizer. See migration 000078
// for why this is not a denormalised counter.
func GetOrganizerFollowerCount(pool *pgxpool.Pool, organizerID string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var n int64
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM organizer_follows WHERE organizer_id = $1`, organizerID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("GetOrganizerFollowerCount: %w", err)
	}
	return n, nil
}

// GetFollowedOrganizers returns the organisers a user follows, newest first.
//
// The upcoming-events count is a correlated subquery rather than a LEFT JOIN
// with GROUP BY: the join form has to aggregate over every event row for every
// organiser in the list, while the subquery stops as soon as it has counted the
// matching index entries. It is also the version that returns 0 rather than
// dropping the organiser entirely when they have nothing coming up, which is
// the row the user most needs to see in order to unfollow.
func GetFollowedOrganizers(pool *pgxpool.Pool, userID string) ([]models.FollowedOrganizerRow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT o.id, o.slug, o.business_name, o.logo_url, o.verified, f.followed_at,
		       (SELECT COUNT(*) FROM events e
		         WHERE e.organizer_id = o.user_id
		           AND e.status = 'published'
		           AND e.visibility = 'public'
		           AND e.end_time >= NOW()) AS upcoming_events
		FROM organizer_follows f
		JOIN organizers o ON o.id = f.organizer_id
		WHERE f.user_id = $1
		ORDER BY f.followed_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("GetFollowedOrganizers: %w", err)
	}
	defer rows.Close()

	out := make([]models.FollowedOrganizerRow, 0, 16)
	for rows.Next() {
		var r models.FollowedOrganizerRow
		if err := rows.Scan(
			&r.OrganizerID, &r.Slug, &r.BusinessName, &r.LogoURL,
			&r.Verified, &r.FollowedAt, &r.UpcomingEvents,
		); err != nil {
			return nil, fmt.Errorf("GetFollowedOrganizers scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── New-event announcement ───────────────────────────────────────────────────

// ErrNoPendingAnnouncement means no event currently needs announcing.
var ErrNoPendingAnnouncement = errors.New("no event pending announcement")

// PendingAnnouncement is one event that has gone live and not yet been
// announced to its organiser's followers.
type PendingAnnouncement struct {
	EventID       string
	EventSlug     string
	EventTitle    string
	CoverImageURL *string
	OrganizerID   string
	OrganizerSlug string
	OrganizerName string
	// OrganizerUserID is excluded from the fan-out — see ListOrganizerFollowerIDs.
	OrganizerUserID string
}

// ClaimPendingAnnouncements finds events that are live, public, and not yet
// announced, and claims them by stamping followers_notified_at.
//
// ── Why the claim happens BEFORE the notifications are written ──────────────
//
// The alternative — notify first, stamp after — re-sends the whole batch to
// everyone if the process dies mid-fan-out, and a duplicate "X announced a new
// event" push is the kind of thing that gets an app's notification permission
// revoked. Stamping first means a crash costs at most one missed announcement
// for one event, which is recoverable by hand and invisible to users.
//
// FOR UPDATE SKIP LOCKED makes the claim safe with more than one API container
// running: two workers polling the same second take disjoint sets rather than
// both processing the same event. Without it, the horizontal scaling that
// already exists behind the Cloudflare Tunnel would double every announcement.
//
// The grace period exists because approval is reversible in the first minutes.
// An admin who approves the wrong event and takes it down within the window has
// not yet notified anybody.
func ClaimPendingAnnouncements(
	pool *pgxpool.Pool,
	ctx context.Context,
	grace time.Duration,
	limit int,
) ([]PendingAnnouncement, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := pool.Query(ctx, `
		WITH claimed AS (
			SELECT id
			FROM   events
			WHERE  followers_notified_at IS NULL
			  AND  status = 'published'
			  AND  visibility = 'public'
			  AND  published_at IS NOT NULL
			  AND  published_at < NOW() - $1::interval
			ORDER BY published_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE events e
		SET    followers_notified_at = NOW()
		FROM   claimed c, organizers o
		WHERE  e.id = c.id
		  AND  o.id = e.organizer_id
		RETURNING e.id, e.slug, e.title, e.cover_image_url,
		          o.id, o.slug, o.business_name, o.user_id
	`, fmt.Sprintf("%d seconds", int(grace.Seconds())), limit)
	if err != nil {
		return nil, fmt.Errorf("ClaimPendingAnnouncements: %w", err)
	}
	defer rows.Close()

	out := make([]PendingAnnouncement, 0, limit)
	for rows.Next() {
		var a PendingAnnouncement
		if err := rows.Scan(
			&a.EventID, &a.EventSlug, &a.EventTitle, &a.CoverImageURL,
			&a.OrganizerID, &a.OrganizerSlug, &a.OrganizerName, &a.OrganizerUserID,
		); err != nil {
			return nil, fmt.Errorf("ClaimPendingAnnouncements scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListOrganizerFollowerIDs returns follower user IDs in pages.
//
// Keyset paged on user_id rather than returning the whole set: an organiser
// with 200k followers would otherwise materialise 200k strings in memory before
// a single notification is written, and the worker would hold that for the
// whole fan-out.
//
// The organiser's own user account is excluded. An organiser following
// themselves is not hypothetical — it is what happens the first time they test
// the follow button on their own page — and being pushed "you announced a new
// event" reads as a bug.
func ListOrganizerFollowerIDs(
	pool *pgxpool.Pool,
	ctx context.Context,
	organizerID string,
	excludeUserID string,
	afterUserID *string,
	limit int,
) ([]string, error) {
	if limit <= 0 {
		limit = 500
	}

	// afterUserID is a *string, not a "" sentinel. The obvious form,
	// `($3 = '' OR user_id > $3::uuid)`, is a live bug: Postgres does not
	// guarantee OR short-circuits, so it may evaluate ''::uuid and fail the
	// whole query with 22P02 on the very first page. A NULL comparison has no
	// such hazard.
	rows, err := pool.Query(ctx, `
		SELECT user_id
		FROM   organizer_follows
		WHERE  organizer_id = $1
		  AND  user_id <> $2
		  AND  ($3::uuid IS NULL OR user_id > $3::uuid)
		ORDER BY user_id
		LIMIT $4
	`, organizerID, excludeUserID, afterUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("ListOrganizerFollowerIDs: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("ListOrganizerFollowerIDs scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ReleaseAnnouncementClaim clears the latch so the event is retried next cycle.
//
// Called only when the fan-out could not START — a follower query that failed,
// for instance. NOT called for a partial fan-out: re-running a batch that
// already delivered half its notifications would double-notify those people,
// which is the failure this design exists to avoid.
func ReleaseAnnouncementClaim(pool *pgxpool.Pool, ctx context.Context, eventID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE events SET followers_notified_at = NULL WHERE id = $1`, eventID)
	if err != nil {
		return fmt.Errorf("ReleaseAnnouncementClaim: %w", err)
	}
	return nil
}

// ── Slug resolution ──────────────────────────────────────────────────────────

// ResolveFollowableOrganizer maps a slug or UUID to an organiser id, applying
// the same public gate as the profile endpoint.
//
// Following is gated for the same reason the page is: an organiser whose page
// does not resolve has nothing for a follower to be notified about, and letting
// people accumulate follows against a suspended account would surface it again
// the moment the suspension lifted.
func ResolveFollowableOrganizer(pool *pgxpool.Pool, slugOrID string) (string, error) {
	profile, err := GetPublicOrganizerProfile(pool, slugOrID)
	if err != nil {
		return "", err
	}
	return profile.ID, nil
}