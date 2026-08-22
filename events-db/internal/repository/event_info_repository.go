package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)


// ── Catalogue ────────────────────────────────────────────────────────────────

// GetInfoCatalogue returns the fields an organiser may choose from.
//
// includeInactive is true only for the read path that hydrates an EXISTING
// event's form: a field deactivated after an event was created must still
// render its stored value, or the organiser would see information silently
// vanish from their own event.
func GetInfoCatalogue(pool *pgxpool.Pool, includeInactive bool) ([]models.EventInfoCatalogueEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q := `
		SELECT key, label, hint, value_type, enum_options,
		       display_order, review_required_when_published, is_active
		FROM   event_info_catalogue`
	if !includeInactive {
		q += ` WHERE is_active = TRUE`
	}
	q += ` ORDER BY display_order, label`

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("GetInfoCatalogue: %w", err)
	}
	defer rows.Close()

	out := make([]models.EventInfoCatalogueEntry, 0, 24)
	for rows.Next() {
		var e models.EventInfoCatalogueEntry
		if err := rows.Scan(
			&e.Key, &e.Label, &e.Hint, &e.ValueType, &e.EnumOptions,
			&e.DisplayOrder, &e.ReviewRequiredWhenPublished, &e.IsActive,
		); err != nil {
			return nil, fmt.Errorf("GetInfoCatalogue scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetInfoCatalogueMap returns the catalogue keyed by key, for validation.
func GetInfoCatalogueMap(pool *pgxpool.Pool) (map[string]models.EventInfoCatalogueEntry, error) {
	list, err := GetInfoCatalogue(pool, true)
	if err != nil {
		return nil, err
	}
	m := make(map[string]models.EventInfoCatalogueEntry, len(list))
	for _, e := range list {
		m[e.Key] = e
	}
	return m, nil
}

// ── Per-event items ──────────────────────────────────────────────────────────

// GetEventInfoItems returns an event's information block in display order.
//
// Ordered by the ITEM's sort_order first and the catalogue's display_order
// second. The organiser's arrangement wins where they made one; the catalogue
// order is the tiebreaker, which matters for items written before the client
// supported reordering (they all carry sort_order 0).
func GetEventInfoItems(pool *pgxpool.Pool, eventID string) ([]models.EventInfoItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT i.info_key, c.label, i.value, c.value_type, i.sort_order
		FROM   event_info_items i
		JOIN   event_info_catalogue c ON c.key = i.info_key
		WHERE  i.event_id = $1
		ORDER  BY i.sort_order, c.display_order
	`, eventID)
	if err != nil {
		return nil, fmt.Errorf("GetEventInfoItems: %w", err)
	}
	defer rows.Close()

	out := make([]models.EventInfoItem, 0, 12)
	for rows.Next() {
		var it models.EventInfoItem
		if err := rows.Scan(&it.Key, &it.Label, &it.Value, &it.ValueType, &it.SortOrder); err != nil {
			return nil, fmt.Errorf("GetEventInfoItems scan: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetEventInfoItemsBulk loads information for several events at once.
//
// Exists so the event detail endpoint does not become N+1 if information is
// ever added to a list response. Returns a map keyed by event id.
func GetEventInfoItemsBulk(pool *pgxpool.Pool, eventIDs []string) (map[string][]models.EventInfoItem, error) {
	out := make(map[string][]models.EventInfoItem, len(eventIDs))
	if len(eventIDs) == 0 {
		return out, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT i.event_id, i.info_key, c.label, i.value, c.value_type, i.sort_order
		FROM   event_info_items i
		JOIN   event_info_catalogue c ON c.key = i.info_key
		WHERE  i.event_id = ANY($1::uuid[])
		ORDER  BY i.event_id, i.sort_order, c.display_order
	`, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("GetEventInfoItemsBulk: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var eventID string
		var it models.EventInfoItem
		if err := rows.Scan(&eventID, &it.Key, &it.Label, &it.Value, &it.ValueType, &it.SortOrder); err != nil {
			return nil, fmt.Errorf("GetEventInfoItemsBulk scan: %w", err)
		}
		out[eventID] = append(out[eventID], it)
	}
	return out, rows.Err()
}

// ── Ownership and status ─────────────────────────────────────────────────────

// EventEditContext is what the write path needs to know before applying a
// change: that the caller owns the event, and whether it is live.
type EventEditContext struct {
	EventID     string
	OrganizerID string
	Status      models.EventStatus
	// IsPublished drives the review lock. Both published states count: a
	// link-only event has real buyers even though it is not discoverable, and
	// their tickets were bought against the same stated age restriction.
	IsPublished bool
}

// GetEventEditContext resolves an event the caller owns.
//
// Ownership is matched on events.organizer_id = the caller's USER id, which is
// how event ownership is keyed throughout this codebase (see
// GetOrganizerEventsHandler and friends) — not on organizers.id. Those are
// different columns holding different values, and using the wrong one here
// would either match nothing or, worse, match somebody else's events.
func GetEventEditContext(pool *pgxpool.Pool, eventID, userID string) (*EventEditContext, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var c EventEditContext
	err := pool.QueryRow(ctx, `
		SELECT id, organizer_id, status
		FROM   events
		WHERE  id = $1 AND organizer_id = $2
	`, eventID, userID).Scan(&c.EventID, &c.OrganizerID, &c.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotOwned
		}
		return nil, fmt.Errorf("GetEventEditContext: %w", err)
	}

	c.IsPublished = c.Status == models.EventPublished ||
		c.Status == models.EventPublishedPrivate ||
		c.Status == models.EventCompleted

	return &c, nil
}

// ── Write ────────────────────────────────────────────────────────────────────

// ReplaceEventInfoItems swaps an event's whole information block atomically.
//
// Delete-then-insert in one transaction, matching ReplaceOrganizerLinks. A
// visitor never sees a half-applied block where the parking note has been
// removed but its replacement has not landed.
func ReplaceEventInfoItems(pool *pgxpool.Pool, eventID string, items []models.EventInfoItem) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ReplaceEventInfoItems: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM event_info_items WHERE event_id = $1`, eventID,
	); err != nil {
		return fmt.Errorf("ReplaceEventInfoItems: delete: %w", err)
	}

	for _, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_info_items (event_id, info_key, value, sort_order)
			VALUES ($1, $2, $3, $4)
		`, eventID, it.Key, it.Value, it.SortOrder); err != nil {
			return fmt.Errorf("ReplaceEventInfoItems: insert %s: %w", it.Key, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ReplaceEventInfoItems: commit: %w", err)
	}
	return nil
}

// ApplyEventInfoFromEdit writes an information block from an APPROVED edit
// request, inside the caller's transaction.
//
// Separate from ReplaceEventInfoItems because the edit-request executor already
// holds a transaction and a row lock — starting a second one here would deadlock
// against it. It also skips validation: the payload was validated when the
// request was FILED, and re-validating at execution against a catalogue that may
// have changed in the interim would strand an approved request that an admin has
// already signed off.
func ApplyEventInfoFromEdit(ctx context.Context, tx pgx.Tx, eventID string, items []models.EventInfoItem) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM event_info_items WHERE event_id = $1`, eventID,
	); err != nil {
		return fmt.Errorf("apply event info: delete: %w", err)
	}

	for _, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_info_items (event_id, info_key, value, sort_order)
			VALUES ($1, $2, $3, $4)
		`, eventID, it.Key, it.Value, it.SortOrder); err != nil {
			return fmt.Errorf("apply event info: insert %s: %w", it.Key, err)
		}
	}
	return nil
}