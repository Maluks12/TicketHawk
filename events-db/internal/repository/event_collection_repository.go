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

// ─────────────────────────────────────────────────────────────────────────────
// Event collections (migration 000096)
//
// Ownership: event_collections.organizer_id holds a USER id, exactly like
// events.organizer_id. Every organiser-side query here is scoped by it, so a
// collection id belonging to someone else behaves as "not found" — the same
// answer as an id that does not exist, which avoids confirming that it does.
// ─────────────────────────────────────────────────────────────────────────────

var ErrEventCollectionNotFound = errors.New("event collection not found")

// Membership reuses ErrEventNotOwned (analytics_repository.go): "doesn't exist"
// and "isn't yours" are one answer, for the whole request — membership is
// all-or-nothing, never a partial apply.

// MinEventsForCollectionCard is how many LIVE dates a collection needs before
// the feed folds it into one card. A "collection" of one live event is just an
// event, and dressing it up as a collection would hide its own title and date
// behind a wrapper for no benefit.
const MinEventsForCollectionCard = 2

// liveEventPredicate is "a member of the public can see and book this right
// now": published (not published_private, which is link-only and never in
// discovery), and not yet over.
const liveEventPredicate = `e.status = 'published' AND e.end_time >= NOW()`

// ── Organiser side ──────────────────────────────────────────────────────────

// eventCollectionSelect returns a collection with its counts and member ids.
const eventCollectionSelect = `
	SELECT c.id, c.slug, c.title, c.description, c.cover_image_url,
	       c.created_at, c.updated_at,
	       COUNT(e.id)                                       AS event_count,
	       COUNT(e.id) FILTER (WHERE ` + liveEventPredicate + `) AS live_event_count,
	       COALESCE(ARRAY_AGG(e.id::text ORDER BY e.start_time) FILTER (WHERE e.id IS NOT NULL), '{}') AS event_ids
	FROM event_collections c
	LEFT JOIN events e ON e.event_collection_id = c.id
`

func scanEventCollection(row pgx.Row) (*models.EventCollection, error) {
	var c models.EventCollection
	if err := row.Scan(
		&c.ID, &c.Slug, &c.Title, &c.Description, &c.CoverImageURL,
		&c.CreatedAt, &c.UpdatedAt,
		&c.EventCount, &c.LiveEventCount, &c.EventIDs,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// ListOrganizerEventCollections returns every collection the organiser owns,
// newest first.
func ListOrganizerEventCollections(ctx context.Context, pool *pgxpool.Pool, userID string) ([]models.EventCollection, error) {
	rows, err := pool.Query(ctx, eventCollectionSelect+`
		WHERE c.organizer_id = $1
		GROUP BY c.id
		ORDER BY c.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("ListOrganizerEventCollections: %w", err)
	}
	defer rows.Close()

	out := make([]models.EventCollection, 0)
	for rows.Next() {
		c, err := scanEventCollection(rows)
		if err != nil {
			return nil, fmt.Errorf("ListOrganizerEventCollections scan: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetOrganizerEventCollection returns one collection the organiser owns.
func GetOrganizerEventCollection(ctx context.Context, pool *pgxpool.Pool, userID, id string) (*models.EventCollection, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrEventCollectionNotFound
	}
	c, err := scanEventCollection(pool.QueryRow(ctx, eventCollectionSelect+`
		WHERE c.id = $1 AND c.organizer_id = $2
		GROUP BY c.id
	`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEventCollectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetOrganizerEventCollection: %w", err)
	}
	return c, nil
}

// CreateEventCollection creates a collection and, optionally, fills it — in one
// transaction, so a failed membership check leaves no empty collection behind.
func CreateEventCollection(ctx context.Context, pool *pgxpool.Pool, userID string, dto models.CreateEventCollectionDTO) (*models.EventCollection, error) {
	id := uuid.NewString()
	title := strings.TrimSpace(dto.Title)
	slug := utils.GenerateSlug(title, id)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("CreateEventCollection/begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO event_collections (id, organizer_id, slug, title, description)
		VALUES ($1, $2, $3, $4, $5)
	`, id, userID, slug, title, ecTrimmedOrNil(dto.Description)); err != nil {
		return nil, fmt.Errorf("CreateEventCollection/insert: %w", err)
	}

	if len(dto.EventIDs) > 0 {
		if err := setMembershipTx(ctx, tx, userID, id, dto.EventIDs); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateEventCollection/commit: %w", err)
	}
	return GetOrganizerEventCollection(ctx, pool, userID, id)
}

// UpdateEventCollection applies a sparse title/description patch.
func UpdateEventCollection(ctx context.Context, pool *pgxpool.Pool, userID, id string, dto models.UpdateEventCollectionDTO) (*models.EventCollection, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrEventCollectionNotFound
	}
	sets := []string{"updated_at = NOW()"}
	args := []interface{}{id, userID}
	if dto.Title != nil {
		args = append(args, strings.TrimSpace(*dto.Title))
		sets = append(sets, fmt.Sprintf("title = $%d", len(args)))
	}
	if dto.Description != nil {
		// An empty string clears it.
		args = append(args, ecTrimmedOrNil(dto.Description))
		sets = append(sets, fmt.Sprintf("description = $%d", len(args)))
	}
	tag, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE event_collections SET %s
		WHERE id = $1 AND organizer_id = $2
	`, strings.Join(sets, ", ")), args...)
	if err != nil {
		return nil, fmt.Errorf("UpdateEventCollection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrEventCollectionNotFound
	}
	return GetOrganizerEventCollection(ctx, pool, userID, id)
}

// SetEventCollectionCover points the collection at an uploaded image, or
// clears it when url is nil.
func SetEventCollectionCover(ctx context.Context, pool *pgxpool.Pool, userID, id string, url *string) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrEventCollectionNotFound
	}
	tag, err := pool.Exec(ctx, `
		UPDATE event_collections SET cover_image_url = $3, updated_at = NOW()
		WHERE id = $1 AND organizer_id = $2
	`, id, userID, url)
	if err != nil {
		return fmt.Errorf("SetEventCollectionCover: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEventCollectionNotFound
	}
	return nil
}

// EventCollectionOwned reports whether the organiser owns the collection —
// checked BEFORE an upload so a stranger's id cannot push bytes into storage.
func EventCollectionOwned(ctx context.Context, pool *pgxpool.Pool, userID, id string) (bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return false, nil
	}
	var ok bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM event_collections WHERE id = $1 AND organizer_id = $2)
	`, id, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("EventCollectionOwned: %w", err)
	}
	return ok, nil
}

// DeleteEventCollection removes the collection. Its events are released by
// ON DELETE SET NULL and go back to appearing as individual cards; the events
// themselves are untouched.
func DeleteEventCollection(ctx context.Context, pool *pgxpool.Pool, userID, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrEventCollectionNotFound
	}
	tag, err := pool.Exec(ctx, `
		DELETE FROM event_collections WHERE id = $1 AND organizer_id = $2
	`, id, userID)
	if err != nil {
		return fmt.Errorf("DeleteEventCollection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEventCollectionNotFound
	}
	return nil
}

// SetEventCollectionEvents replaces the collection's membership with eventIDs.
func SetEventCollectionEvents(ctx context.Context, pool *pgxpool.Pool, userID, id string, eventIDs []string) (*models.EventCollection, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrEventCollectionNotFound
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("SetEventCollectionEvents/begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the collection row: two tabs saving membership at once serialise
	// here instead of interleaving their release/assign statements.
	var owner string
	err = tx.QueryRow(ctx, `
		SELECT organizer_id::text FROM event_collections WHERE id = $1 FOR UPDATE
	`, id).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != userID) {
		return nil, ErrEventCollectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("SetEventCollectionEvents/lock: %w", err)
	}

	if err := setMembershipTx(ctx, tx, userID, id, eventIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("SetEventCollectionEvents/commit: %w", err)
	}
	return GetOrganizerEventCollection(ctx, pool, userID, id)
}

// setMembershipTx makes eventIDs exactly the collection's members.
//
// An event already in ANOTHER of this organiser's collections moves here —
// one collection per event is a schema rule (a single column), and the
// dashboard warns before it happens. Events that are not the organiser's are
// rejected as a whole: nothing is applied.
func setMembershipTx(ctx context.Context, tx pgx.Tx, userID, collectionID string, eventIDs []string) error {
	ids := dedupeIDs(eventIDs)
	for _, e := range ids {
		if _, err := uuid.Parse(e); err != nil {
			return ErrEventNotOwned
		}
	}

	if len(ids) > 0 {
		var owned int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM events
			WHERE id = ANY($1::uuid[]) AND organizer_id = $2
		`, ids, userID).Scan(&owned); err != nil {
			return fmt.Errorf("setMembership/check: %w", err)
		}
		if owned != len(ids) {
			return ErrEventNotOwned
		}
	}

	// Release anything no longer listed.
	if _, err := tx.Exec(ctx, `
		UPDATE events SET event_collection_id = NULL
		WHERE event_collection_id = $1 AND NOT (id = ANY($2::uuid[]))
	`, collectionID, ids); err != nil {
		return fmt.Errorf("setMembership/release: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	// Assign. IS DISTINCT FROM skips rows already in place, so re-saving an
	// unchanged list writes nothing to the events table.
	if _, err := tx.Exec(ctx, `
		UPDATE events SET event_collection_id = $1
		WHERE id = ANY($2::uuid[]) AND organizer_id = $3
		  AND event_collection_id IS DISTINCT FROM $1
	`, collectionID, ids, userID); err != nil {
		return fmt.Errorf("setMembership/assign: %w", err)
	}
	return nil
}

// ── Public side ─────────────────────────────────────────────────────────────

// GetPublicEventCollection resolves a collection by slug (or id) with its
// upcoming public dates.
func GetPublicEventCollection(ctx context.Context, pool *pgxpool.Pool, slugOrID string) (*models.PublicEventCollection, error) {
	slugOrID = strings.TrimSpace(slugOrID)
	if slugOrID == "" {
		return nil, ErrEventCollectionNotFound
	}

	var (
		c       models.PublicEventCollection
		ownerID string
	)
	err := pool.QueryRow(ctx, `
		SELECT id, slug, title, description, cover_image_url, organizer_id::text
		FROM event_collections
		WHERE slug = $1 OR id::text = $1
		LIMIT 1
	`, slugOrID).Scan(&c.ID, &c.Slug, &c.Title, &c.Description, &c.CoverImageURL, &ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEventCollectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetPublicEventCollection: %w", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT
			e.id, e.slug, e.title, e.cover_image_url, e.banner_image_url,
			e.start_time, e.end_time, e.timezone,
			e.venue_name, e.city,
			e.price, e.currency,
			e.max_capacity, e.tickets_sold,
			e.status, c.name AS category
		FROM events e
		JOIN categories c ON e.category_id = c.id
		WHERE e.event_collection_id = $1 AND `+liveEventPredicate+`
		ORDER BY e.start_time ASC, e.id ASC
		LIMIT 200
	`, c.ID)
	if err != nil {
		return nil, fmt.Errorf("GetPublicEventCollection/events: %w", err)
	}
	defer rows.Close()

	c.Events = make([]models.Event, 0)
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Slug, &e.Title, &e.CoverImageURL, &e.BannerImageURL,
			&e.StartTime, &e.EndTime, &e.Timezone,
			&e.VenueName, &e.City,
			&e.Price, &e.Currency,
			&e.MaxCapacity, &e.TicketsSold,
			&e.Status, &e.Category,
		); err != nil {
			return nil, fmt.Errorf("GetPublicEventCollection/scan: %w", err)
		}
		// Card-level availability, same as the organiser page's event list.
		e.ComputeAvailableTickets()
		c.Events = append(c.Events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetPublicEventCollection/rows: %w", err)
	}

	// Organiser block, gated exactly as on the event page (public page live,
	// verified, in good standing). Best effort: absent rather than an error.
	if org, err := GetPublicOrganizerProfileByUserID(pool, ownerID); err == nil {
		c.Organizer = org
	}
	return &c, nil
}

// CollectionRefsForEvents returns, for each given event that belongs to a
// collection with at least MinEventsForCollectionCard live dates, a summary of
// that collection. Events not in such a collection are absent from the map.
// Events sharing a collection share one *EventCollectionRef.
func CollectionRefsForEvents(ctx context.Context, pool *pgxpool.Pool, eventIDs []string) (map[string]*models.EventCollectionRef, error) {
	out := map[string]*models.EventCollectionRef{}
	ids := make([]string, 0, len(eventIDs))
	for _, id := range dedupeIDs(eventIDs) {
		if _, err := uuid.Parse(id); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}

	rows, err := pool.Query(ctx, `
		WITH member AS (
			SELECT id::text AS event_id, event_collection_id AS cid
			FROM events
			WHERE id = ANY($1::uuid[]) AND event_collection_id IS NOT NULL
		),
		live AS (
			SELECT e.event_collection_id AS cid,
			       COUNT(*)          AS n,
			       MIN(e.start_time) AS first_start,
			       MAX(e.start_time) AS last_start,
			       MIN(e.price)      AS min_price,
			       MIN(e.currency)   AS currency
			FROM events e
			WHERE e.event_collection_id IN (SELECT cid FROM member)
			  AND `+liveEventPredicate+`
			GROUP BY e.event_collection_id
			HAVING COUNT(*) >= $2
		)
		SELECT m.event_id, c.id::text, c.slug, c.title, c.cover_image_url,
		       l.n, l.first_start, l.last_start, l.min_price, l.currency,
		       ARRAY(
		           SELECT x.city FROM (
		               SELECT e.city, MIN(e.start_time) AS s
		               FROM events e
		               WHERE e.event_collection_id = c.id AND `+liveEventPredicate+`
		               GROUP BY e.city
		           ) x ORDER BY x.s
		       ) AS cities
		FROM member m
		JOIN event_collections c ON c.id = m.cid
		JOIN live l              ON l.cid = c.id
	`, ids, MinEventsForCollectionCard)
	if err != nil {
		return nil, fmt.Errorf("CollectionRefsForEvents: %w", err)
	}
	defer rows.Close()

	byCollection := map[string]*models.EventCollectionRef{}
	for rows.Next() {
		var (
			eventID string
			r       models.EventCollectionRef
		)
		if err := rows.Scan(
			&eventID, &r.ID, &r.Slug, &r.Title, &r.CoverImageURL,
			&r.EventCount, &r.FirstStart, &r.LastStart, &r.MinPrice, &r.Currency,
			&r.Cities,
		); err != nil {
			return nil, fmt.Errorf("CollectionRefsForEvents/scan: %w", err)
		}
		ref, ok := byCollection[r.ID]
		if !ok {
			cp := r
			ref = &cp
			byCollection[r.ID] = ref
		}
		out[eventID] = ref
	}
	return out, rows.Err()
}

// FoldCollections keeps the FIRST event of each collection (in the section's
// existing order, so the ranking that put it there still decides where the
// card sits), attaches the collection summary to it, and drops that
// collection's other events. Events outside a qualifying collection pass
// through untouched.
func FoldCollections(events []models.Event, refs map[string]*models.EventCollectionRef) []models.Event {
	if len(refs) == 0 || len(events) == 0 {
		return events
	}
	seen := map[string]bool{}
	out := make([]models.Event, 0, len(events))
	for _, e := range events {
		ref, ok := refs[e.ID]
		if !ok {
			out = append(out, e)
			continue
		}
		if seen[ref.ID] {
			continue
		}
		seen[ref.ID] = true
		e.Collection = ref
		out = append(out, e)
	}
	return out
}

// foldHomeSections folds collections across every section with ONE query for
// all their events. Best effort: if the lookup fails the feed is returned
// unfolded — a cluttered feed beats an empty one.
func foldHomeSections(pool *pgxpool.Pool, sections []HomeSection) []HomeSection {
	var ids []string
	for _, s := range sections {
		for _, e := range s.Events {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return sections
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	refs, err := CollectionRefsForEvents(ctx, pool, ids)
	if err != nil || len(refs) == 0 {
		return sections
	}
	for i := range sections {
		sections[i].Events = FoldCollections(sections[i].Events, refs)
	}
	return sections
}

// collectionRefForEvent is the single-event form used by the detail page.
func collectionRefForEvent(pool *pgxpool.Pool, eventID string) (*models.EventCollectionRef, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	refs, err := CollectionRefsForEvents(ctx, pool, []string{eventID})
	if err != nil {
		return nil, err
	}
	return refs[eventID], nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func ecTrimmedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

func dedupeIDs(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(strings.ToLower(s))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
