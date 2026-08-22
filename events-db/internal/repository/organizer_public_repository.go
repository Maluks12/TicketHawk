package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrOrganizerProfileNotFound is returned for BOTH "no such organiser" and
// "this organiser's page is not public".
//
// One error for two causes is deliberate. Distinguishing them would let anyone
// enumerate slugs to learn which organisers exist but are suspended or have
// hidden their page — the status of another business's account is not the
// visitor's to know. The handler maps this to a plain 404.
var ErrOrganizerProfileNotFound = errors.New("organizer profile not found")

// ── Public profile ───────────────────────────────────────────────────────────

// GetPublicOrganizerProfile resolves an organiser by slug or UUID and returns
// the visitor-safe projection, with links attached.
//
// The gate (kyc_level >= 2, public_profile_enabled, not suspended/banned) is
// applied in SQL rather than by loading the row and calling
// CanHavePublicProfile on it. Two reasons: the partial index added in 000077
// covers exactly this predicate, and a gate in the WHERE clause cannot be
// forgotten by a future caller the way a method call can.
//
// CanHavePublicProfile remains the single source of truth for the RULE — this
// query mirrors it, and the model test asserts they agree.
func GetPublicOrganizerProfile(pool *pgxpool.Pool, slugOrID string) (*models.PublicOrganizer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	slugOrID = strings.TrimSpace(slugOrID)
	if slugOrID == "" {
		return nil, ErrOrganizerProfileNotFound
	}

	// Accept either form, matching how the event detail route already behaves.
	// The UUID branch is guarded by a parse rather than by casting in SQL:
	// `id::text = $1` would defeat the primary key index, and passing a
	// non-UUID string to a uuid comparison is a 22P02 error, not a miss.
	var (
		org       models.PublicOrganizer
		createdAt time.Time
	)

	const cols = `
		o.id, o.slug, o.business_name, o.description,
		o.logo_url, o.cover_image_url, o.verified, o.created_at`

	const gate = `
		  AND o.public_profile_enabled = TRUE
		  AND o.kyc_level >= 2
		  AND o.status::text NOT IN ('suspended', 'banned', 'rejected')`

	var row pgx.Row
	if _, err := uuid.Parse(slugOrID); err == nil {
		row = pool.QueryRow(ctx,
			`SELECT `+cols+` FROM organizers o WHERE o.id = $1`+gate, slugOrID)
	} else {
		row = pool.QueryRow(ctx,
			`SELECT `+cols+` FROM organizers o WHERE o.slug = $1`+gate, slugOrID)
	}

	err := row.Scan(
		&org.ID, &org.Slug, &org.BusinessName, &org.Description,
		&org.LogoURL, &org.CoverImageURL, &org.Verified, &createdAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrganizerProfileNotFound
		}
		return nil, fmt.Errorf("GetPublicOrganizerProfile: %w", err)
	}

	org.MemberSince = createdAt.Year()

	// Counts and links are separate round-trips rather than subqueries in the
	// SELECT above. A failure to count events should not take down the whole
	// profile — the page is still worth rendering without the stat.
	count, err := CountPublicOrganizerEvents(pool, org.ID)
	if err == nil {
		org.EventsHosted = count
	}

	links, err := GetOrganizerLinks(pool, org.ID)
	if err == nil && len(links) > 0 {
		org.Links = links
	}

	// Phase 2. Same tolerance as the event count: a follower count that fails
	// to load should not take down the page it decorates.
	if n, err := GetOrganizerFollowerCount(pool, org.ID); err == nil {
		org.FollowerCount = n
	}

	return &org, nil
}

// CountPublicOrganizerEvents counts an organiser's publicly listable events —
// published and completed, past and future.
//
// published_private is excluded: those events are link-only by definition, and
// including them in a public count would leak both their existence and how many
// there are. Draft, pending_review, rejected and cancelled are excluded for the
// same reason they are excluded from the listing itself.
func CountPublicOrganizerEvents(pool *pgxpool.Pool, organizerID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM events
		WHERE organizer_id = $1
		  AND status = ANY($2::event_status[])
	`, organizerID, []string{
		string(models.EventPublished),
		string(models.EventCompleted),
	}).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("CountPublicOrganizerEvents: %w", err)
	}
	return n, nil
}

// ── Event listing ────────────────────────────────────────────────────────────

// OrganizerEventScope selects which half of an organiser's catalogue to list.
type OrganizerEventScope string

const (
	ScopeUpcoming OrganizerEventScope = "upcoming"
	ScopePast     OrganizerEventScope = "past"
)

// ListPublicOrganizerEvents returns one page of an organiser's events.
//
// ── Why the split is on TIME, not on status ─────────────────────────────────
//
// An event only becomes `completed` when CompleteEndedEvents sweeps it, so
// there is always a window — and, for anything the worker misses, indefinitely
// — where a finished event is still `published`. Splitting on status would put
// last night's show under "Upcoming" until a background job caught up. Splitting
// on end_time is what the visitor actually means, and it matches how the event
// detail screen already decides whether to show a Get Tickets button.
//
// Both scopes accept `completed` as well as `published` so that an event swept
// by the worker still appears under Past rather than vanishing.
//
// Ordering differs by scope on purpose: upcoming events read soonest-first,
// past events read most-recent-first. The keyset cursor mirrors whichever
// ordering is in play, so paging stays stable across both.
func ListPublicOrganizerEvents(
	pool *pgxpool.Pool,
	organizerID string,
	scope OrganizerEventScope,
	limit int,
	cursor *utils.Cursor,
) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if limit <= 0 || limit > 50 {
		limit = 20
	}

	liveOrDone := []string{
		string(models.EventPublished),
		string(models.EventCompleted),
	}

	args := []interface{}{organizerID, liveOrDone}
	where := []string{
		`e.organizer_id = $1`,
		`e.status = ANY($2::event_status[])`,
	}

	var order string
	if scope == ScopePast {
		where = append(where, `e.end_time < NOW()`)
		order = `e.end_time DESC, e.id DESC`
		if cursor != nil {
			where = append(where, fmt.Sprintf(
				`(e.end_time < $%d OR (e.end_time = $%d AND e.id < $%d))`,
				len(args)+1, len(args)+1, len(args)+2))
			args = append(args, cursor.StartTime, cursor.ID)
		}
	} else {
		where = append(where, `e.end_time >= NOW()`)
		order = `e.start_time ASC, e.id ASC`
		if cursor != nil {
			where = append(where, fmt.Sprintf(
				`(e.start_time > $%d OR (e.start_time = $%d AND e.id > $%d))`,
				len(args)+1, len(args)+1, len(args)+2))
			args = append(args, cursor.StartTime, cursor.ID)
		}
	}

	// limit+1 so we can tell whether another page exists without a COUNT.
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			e.id, e.slug, e.title, e.cover_image_url, e.banner_image_url,
			e.start_time, e.end_time,
			e.venue_name, e.city,
			e.price, e.currency,
			e.max_capacity, e.tickets_sold,
			e.status, c.name AS category
		FROM events e
		JOIN categories c ON e.category_id = c.id
		WHERE %s
		ORDER BY %s
		LIMIT $%d
	`, strings.Join(where, " AND "), order, len(args))

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("ListPublicOrganizerEvents: %w", err)
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit)
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Slug, &e.Title, &e.CoverImageURL, &e.BannerImageURL,
			&e.StartTime, &e.EndTime,
			&e.VenueName, &e.City,
			&e.Price, &e.Currency,
			&e.MaxCapacity, &e.TicketsSold,
			&e.Status, &e.Category,
		); err != nil {
			return nil, nil, fmt.Errorf("ListPublicOrganizerEvents scan: %w", err)
		}
		// No ticket tiers are loaded here, so this takes the flat-price branch
		// and reports capacity headroom. That is the right level of detail for
		// a card; the detail endpoint computes the authoritative figure.
		e.ComputeAvailableTickets()
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("ListPublicOrganizerEvents rows: %w", err)
	}

	var next *utils.Cursor
	if len(events) > limit {
		events = events[:limit]
		last := events[len(events)-1]
		// The cursor carries whichever timestamp the ORDER BY used, so the
		// predicate above compares like with like.
		ts := last.StartTime
		if scope == ScopePast {
			ts = last.EndTime
		}
		next = &utils.Cursor{StartTime: ts, ID: last.ID}
	}

	return events, next, nil
}

// ── Links ────────────────────────────────────────────────────────────────────

// GetOrganizerLinks returns an organiser's links in display order.
func GetOrganizerLinks(pool *pgxpool.Pool, organizerID string) ([]models.OrganizerLink, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT platform, url, sort_order
		FROM organizer_links
		WHERE organizer_id = $1
		ORDER BY sort_order ASC, platform ASC
	`, organizerID)
	if err != nil {
		return nil, fmt.Errorf("GetOrganizerLinks: %w", err)
	}
	defer rows.Close()

	out := make([]models.OrganizerLink, 0, 9)
	for rows.Next() {
		var l models.OrganizerLink
		if err := rows.Scan(&l.Platform, &l.URL, &l.SortOrder); err != nil {
			return nil, fmt.Errorf("GetOrganizerLinks scan: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ReplaceOrganizerLinks swaps an organiser's entire link set atomically.
//
// Delete-then-insert inside one transaction rather than an upsert-and-prune
// dance: the client sends the complete set, so reconciling which rows changed
// buys nothing, and doing it in a transaction means a visitor never sees a
// half-applied set where the Instagram link has been removed but the
// replacement has not landed.
func ReplaceOrganizerLinks(pool *pgxpool.Pool, organizerID string, links []models.OrganizerLink) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ReplaceOrganizerLinks: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM organizer_links WHERE organizer_id = $1`, organizerID,
	); err != nil {
		return fmt.Errorf("ReplaceOrganizerLinks: delete: %w", err)
	}

	for _, l := range links {
		if _, err := tx.Exec(ctx, `
			INSERT INTO organizer_links (organizer_id, platform, url, sort_order)
			VALUES ($1, $2, $3, $4)
		`, organizerID, string(l.Platform), l.URL, l.SortOrder); err != nil {
			return fmt.Errorf("ReplaceOrganizerLinks: insert %s: %w", l.Platform, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ReplaceOrganizerLinks: commit: %w", err)
	}
	return nil
}

// ── Owner-side profile ───────────────────────────────────────────────────────

// GetOrganizerIDForUser resolves the organizers.id owned by a user.
//
// Handlers under /organiser hold a user_id from the JWT, not an organizer_id.
// Several existing queries paper over this by treating events.organizer_id as
// a user id — that works only because event ownership happens to be keyed that
// way. organizer_links references organizers(id), so it needs the real thing.
func GetOrganizerIDForUser(pool *pgxpool.Pool, userID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var id string
	err := pool.QueryRow(ctx,
		`SELECT id FROM organizers WHERE user_id = $1`, userID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrOrganizerNotFound
		}
		return "", fmt.Errorf("GetOrganizerIDForUser: %w", err)
	}
	return id, nil
}

// OwnerProfileView is what an organiser sees of their OWN public page in the
// dashboard — the public projection plus the fields only they need: whether the
// page is currently live, and if not, why.
type OwnerProfileView struct {
	*models.PublicOrganizer
	PublicProfileEnabled bool   `json:"public_profile_enabled"`
	IsLive               bool   `json:"is_live"`
	// BlockedReason is empty when IsLive. Otherwise it names the one condition
	// the organiser can act on, so the dashboard can say "complete identity
	// verification to publish your page" rather than showing a dead toggle.
	BlockedReason string `json:"blocked_reason,omitempty"`
	PublicURLPath string `json:"public_url_path"`
}

// GetOwnerProfile loads an organiser's own profile REGARDLESS of the public
// gate, so the dashboard can render the page they are about to publish.
func GetOwnerProfile(pool *pgxpool.Pool, userID string) (*OwnerProfileView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		org       models.PublicOrganizer
		createdAt time.Time
		enabled   bool
		kycLevel  int
		status    models.OrganizerStatus
		orgID     string
	)

	err := pool.QueryRow(ctx, `
		SELECT id, slug, business_name, description, logo_url, cover_image_url,
		       verified, created_at, public_profile_enabled, kyc_level, status
		FROM organizers
		WHERE user_id = $1
	`, userID).Scan(
		&orgID, &org.Slug, &org.BusinessName, &org.Description,
		&org.LogoURL, &org.CoverImageURL, &org.Verified, &createdAt,
		&enabled, &kycLevel, &status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrganizerNotFound
		}
		return nil, fmt.Errorf("GetOwnerProfile: %w", err)
	}

	org.ID = orgID
	org.MemberSince = createdAt.Year()

	if n, err := CountPublicOrganizerEvents(pool, orgID); err == nil {
		org.EventsHosted = n
	}
	if links, err := GetOrganizerLinks(pool, orgID); err == nil && len(links) > 0 {
		org.Links = links
	}
	if n, err := GetOrganizerFollowerCount(pool, orgID); err == nil {
		org.FollowerCount = n
	}

	// Rebuild a minimal Organizer so the live/blocked decision comes from
	// CanHavePublicProfile rather than being reimplemented here.
	probe := &models.Organizer{
		KYCLevel:             kycLevel,
		PublicProfileEnabled: enabled,
		Status:               status,
	}

	view := &OwnerProfileView{
		PublicOrganizer:      &org,
		PublicProfileEnabled: enabled,
		IsLive:               probe.CanHavePublicProfile(),
		PublicURLPath:        "/organiser/" + org.Slug,
	}

	if !view.IsLive {
		switch {
		case status == models.OrganizerSuspended || status == models.OrganizerBanned:
			view.BlockedReason = "account_suspended"
		case kycLevel < models.KYCLevelFull:
			view.BlockedReason = "kyc_level_required"
		case !enabled:
			view.BlockedReason = "disabled_by_organiser"
		default:
			view.BlockedReason = "unavailable"
		}
	}

	return view, nil
}

// UpdatePublicProfile applies the organiser's own edits to their page.
//
// Built as a dynamic UPDATE over only the keys that were sent, so an
// unspecified field is left alone rather than nulled — which is what the
// pointer fields on the DTO exist to express.
func UpdatePublicProfile(pool *pgxpool.Pool, userID string, dto *models.UpdatePublicProfileDTO) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sets := []string{}
	args := []interface{}{}

	if dto.Description != nil {
		args = append(args, *dto.Description)
		sets = append(sets, fmt.Sprintf("description = $%d", len(args)))
	}
	if dto.CoverImageURL != nil {
		args = append(args, *dto.CoverImageURL)
		sets = append(sets, fmt.Sprintf("cover_image_url = $%d", len(args)))
	}
	if dto.PublicProfileEnabled != nil {
		args = append(args, *dto.PublicProfileEnabled)
		sets = append(sets, fmt.Sprintf("public_profile_enabled = $%d", len(args)))
	}

	if len(sets) == 0 {
		return nil // nothing sent; not an error
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, userID)

	tag, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE organizers SET %s WHERE user_id = $%d`,
		strings.Join(sets, ", "), len(args),
	), args...)
	if err != nil {
		return fmt.Errorf("UpdatePublicProfile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrOrganizerNotFound
	}
	return nil
}

// ── Profile media ────────────────────────────────────────────────────────────

// SetOrganizerCoverImage points the organiser's public page at a new cover.
func SetOrganizerCoverImage(pool *pgxpool.Pool, userID, url string) error {
	return setOrganizerImageColumn(pool, userID, "cover_image_url", url)
}

// SetOrganizerLogo points the organiser at a new logo. Note this image also
// appears on every event card they have published, so a change here is not
// scoped to the profile page.
func SetOrganizerLogo(pool *pgxpool.Pool, userID, url string) error {
	return setOrganizerImageColumn(pool, userID, "logo_url", url)
}

// setOrganizerImageColumn is the shared body. The column name is interpolated
// rather than parameterised because Postgres does not accept an identifier as a
// bind parameter — it is safe here only because both callers pass a compile-time
// constant. Do not export this or widen it to take user input.
func setOrganizerImageColumn(pool *pgxpool.Pool, userID, column, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE organizers SET %s = $1, updated_at = NOW() WHERE user_id = $2`, column,
	), url, userID)
	if err != nil {
		return fmt.Errorf("set %s: %w", column, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrOrganizerNotFound
	}
	return nil
}