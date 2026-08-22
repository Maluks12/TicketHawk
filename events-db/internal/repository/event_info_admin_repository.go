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

var (
	ErrInfoKeyExists   = errors.New("information field key already exists")
	ErrInfoKeyNotFound = errors.New("information field not found")
)

// GetInfoCatalogueEntry loads one field, active or not.
func GetInfoCatalogueEntry(pool *pgxpool.Pool, key string) (*models.EventInfoCatalogueEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var e models.EventInfoCatalogueEntry
	err := pool.QueryRow(ctx, `
		SELECT key, label, hint, value_type, enum_options,
		       display_order, review_required_when_published, is_active
		FROM   event_info_catalogue
		WHERE  key = $1
	`, key).Scan(
		&e.Key, &e.Label, &e.Hint, &e.ValueType, &e.EnumOptions,
		&e.DisplayOrder, &e.ReviewRequiredWhenPublished, &e.IsActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInfoKeyNotFound
		}
		return nil, fmt.Errorf("GetInfoCatalogueEntry: %w", err)
	}
	return &e, nil
}

// CreateInfoCatalogueEntry adds a field.
func CreateInfoCatalogueEntry(pool *pgxpool.Pool, e *models.EventInfoCatalogueEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var enumOpts any
	if len(e.EnumOptions) > 0 {
		enumOpts = e.EnumOptions
	}

	tag, err := pool.Exec(ctx, `
		INSERT INTO event_info_catalogue
			(key, label, hint, value_type, enum_options,
			 display_order, review_required_when_published, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE)
		ON CONFLICT (key) DO NOTHING
	`, e.Key, e.Label, e.Hint, string(e.ValueType), enumOpts,
		e.DisplayOrder, e.ReviewRequiredWhenPublished)
	if err != nil {
		return fmt.Errorf("CreateInfoCatalogueEntry: %w", err)
	}
	// ON CONFLICT DO NOTHING rather than letting the unique violation surface:
	// the handler needs to tell a 409 apart from a 500, and matching on
	// SQLSTATE 23505 is more fragile than checking whether a row landed.
	if tag.RowsAffected() == 0 {
		return ErrInfoKeyExists
	}
	return nil
}

// UpdateInfoCatalogueEntry writes presentation changes.
//
// key and value_type are absent from the SET list on purpose — renaming a key
// would orphan every event_info_items row pointing at it, and changing a
// value_type would leave existing values failing validation in organiser forms
// nobody has touched. See admin_event_info_handler.go.
func UpdateInfoCatalogueEntry(pool *pgxpool.Pool, e *models.EventInfoCatalogueEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var enumOpts any
	if len(e.EnumOptions) > 0 {
		enumOpts = e.EnumOptions
	}

	tag, err := pool.Exec(ctx, `
		UPDATE event_info_catalogue
		SET    label = $2,
		       hint = $3,
		       enum_options = $4,
		       display_order = $5,
		       review_required_when_published = $6
		WHERE  key = $1
	`, e.Key, e.Label, e.Hint, enumOpts, e.DisplayOrder, e.ReviewRequiredWhenPublished)
	if err != nil {
		return fmt.Errorf("UpdateInfoCatalogueEntry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInfoKeyNotFound
	}
	return nil
}

// SetInfoCatalogueActive retires or restores a field.
//
// Never deletes. event_info_items.info_key is ON DELETE RESTRICT, so a delete
// would fail anyway once any event uses the field — and that restriction exists
// precisely so information organisers entered cannot vanish because somebody
// tidied a list.
func SetInfoCatalogueActive(pool *pgxpool.Pool, key string, active bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := pool.Exec(ctx,
		`UPDATE event_info_catalogue SET is_active = $2 WHERE key = $1`, key, active)
	if err != nil {
		return fmt.Errorf("SetInfoCatalogueActive: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInfoKeyNotFound
	}
	return nil
}

// ReorderInfoCatalogue sets display_order from array position.
//
// One transaction for the whole list: the console reorders by drag, which moves
// several fields at once, and a partial apply would leave the catalogue
// visibly half-sorted with no way for the admin to tell which half took.
//
// Keys not in the list keep their current order, and are pushed below the
// supplied ones by the *10 spacing — so a partial list is a safe operation
// rather than a way to accidentally scramble everything omitted.
func ReorderInfoCatalogue(pool *pgxpool.Pool, keys []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ReorderInfoCatalogue: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	for i, key := range keys {
		if _, err := tx.Exec(ctx,
			`UPDATE event_info_catalogue SET display_order = $2 WHERE key = $1`,
			key, (i+1)*10,
		); err != nil {
			return fmt.Errorf("ReorderInfoCatalogue: %s: %w", key, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ReorderInfoCatalogue: commit: %w", err)
	}
	return nil
}

// ── Usage ────────────────────────────────────────────────────────────────────

// GetInfoCatalogueUsage counts how many events use each field.
//
// This is what turns "retire this field" from a guess into a decision: an admin
// can see that `camping` is used by 2 events and `parking` by 400 before
// hiding either.
func GetInfoCatalogueUsage(pool *pgxpool.Pool) (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT info_key, COUNT(DISTINCT event_id)
		FROM   event_info_items
		GROUP  BY info_key
	`)
	if err != nil {
		return nil, fmt.Errorf("GetInfoCatalogueUsage: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int, 32)
	for rows.Next() {
		var key string
		var n int
		if err := rows.Scan(&key, &n); err != nil {
			return nil, fmt.Errorf("GetInfoCatalogueUsage scan: %w", err)
		}
		out[key] = n
	}
	return out, rows.Err()
}

// GetInfoFieldUsageCount counts events using one field.
func GetInfoFieldUsageCount(pool *pgxpool.Pool, key string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var n int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT event_id) FROM event_info_items WHERE info_key = $1`, key,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("GetInfoFieldUsageCount: %w", err)
	}
	return n, nil
}

// GetInfoValuesInUse returns the distinct values events hold for a field.
//
// Used to refuse an enum edit that would orphan existing data. Capped: the
// caller only needs to know WHICH options are still in use, and an unbounded
// scan of a text field with thousands of distinct free-prose values would be
// both slow and pointless — that check only runs for enum fields, where the
// distinct count is small by construction.
func GetInfoValuesInUse(pool *pgxpool.Pool, key string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT DISTINCT value
		FROM   event_info_items
		WHERE  info_key = $1
		LIMIT  200
	`, key)
	if err != nil {
		return nil, fmt.Errorf("GetInfoValuesInUse: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, 16)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("GetInfoValuesInUse scan: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}