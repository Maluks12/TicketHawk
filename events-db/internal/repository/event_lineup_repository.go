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

// ── Read ─────────────────────────────────────────────────────────────────────

// GetEventLineup returns an event's line-up in display order.
//
// Ordered by kind first so teams group together ahead of performers — a fixture
// reads as "A vs B", and interleaving a support DJ between the two sides would
// be nonsense. Within a kind, the organiser's arrangement wins.
func GetEventLineup(pool *pgxpool.Pool, eventID string) ([]models.LineupEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT id, kind, name, image_url, subtitle, stage, set_time, external_url, sort_order
		FROM   event_lineup
		WHERE  event_id = $1
		ORDER  BY kind DESC, sort_order, name
	`, eventID)
	if err != nil {
		return nil, fmt.Errorf("GetEventLineup: %w", err)
	}
	defer rows.Close()

	out := make([]models.LineupEntry, 0, 16)
	for rows.Next() {
		var e models.LineupEntry
		if err := rows.Scan(
			&e.ID, &e.Kind, &e.Name, &e.ImageURL, &e.Subtitle,
			&e.Stage, &e.SetTime, &e.ExternalURL, &e.SortOrder,
		); err != nil {
			return nil, fmt.Errorf("GetEventLineup scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetEventLineupBulk loads line-ups for several events at once, so the detail
// payload never becomes N+1 if line-up is added to a list response.
func GetEventLineupBulk(pool *pgxpool.Pool, eventIDs []string) (map[string][]models.LineupEntry, error) {
	out := make(map[string][]models.LineupEntry, len(eventIDs))
	if len(eventIDs) == 0 {
		return out, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT event_id, id, kind, name, image_url, subtitle, stage, set_time, external_url, sort_order
		FROM   event_lineup
		WHERE  event_id = ANY($1::uuid[])
		ORDER  BY event_id, kind DESC, sort_order, name
	`, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("GetEventLineupBulk: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var eventID string
		var e models.LineupEntry
		if err := rows.Scan(
			&eventID, &e.ID, &e.Kind, &e.Name, &e.ImageURL, &e.Subtitle,
			&e.Stage, &e.SetTime, &e.ExternalURL, &e.SortOrder,
		); err != nil {
			return nil, fmt.Errorf("GetEventLineupBulk scan: %w", err)
		}
		out[eventID] = append(out[eventID], e)
	}
	return out, rows.Err()
}

// GetEventTimes returns an event's start and end, for the set-time range check.
func GetEventTimes(pool *pgxpool.Pool, eventID string) (start, end time.Time, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = pool.QueryRow(ctx,
		`SELECT start_time, end_time FROM events WHERE id = $1`, eventID,
	).Scan(&start, &end)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return start, end, ErrEventNotOwned
		}
		return start, end, fmt.Errorf("GetEventTimes: %w", err)
	}
	return start, end, nil
}

// ── Write ────────────────────────────────────────────────────────────────────

// ReplaceEventLineup swaps an event's whole line-up atomically.
//
// ── The dual write ──────────────────────────────────────────────────────────
//
// This writes the line-up TWICE: once as rows in event_lineup, and once as flat
// name arrays back into events.performers and events.teams.
//
// That is not redundancy left over from the migration. Shipped mobile builds
// read those arrays directly — EventsApp/app/event/[id].tsx lines 401-402 —
// and a user on an old build does not update because we deployed. Writing only
// the new table would empty the line-up section on every phone that has not
// been updated, including phones belonging to people holding tickets to the
// event whose line-up just changed.
//
// So both are written until the app's minimum supported version reads the
// line-up from the payload. At that point this function loses its second half
// and migration 000081's columns can be dropped. Not before.
//
// Both writes are in ONE transaction, so the arrays can never disagree with the
// table.
func ReplaceEventLineup(pool *pgxpool.Pool, eventID string, entries []models.LineupEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ReplaceEventLineup: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM event_lineup WHERE event_id = $1`, eventID,
	); err != nil {
		return fmt.Errorf("ReplaceEventLineup: delete: %w", err)
	}

	for _, e := range entries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_lineup
				(event_id, kind, name, image_url, subtitle, stage, set_time, external_url, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, eventID, string(e.Kind), e.Name, e.ImageURL, e.Subtitle,
			e.Stage, e.SetTime, e.ExternalURL, e.SortOrder,
		); err != nil {
			return fmt.Errorf("ReplaceEventLineup: insert %q: %w", e.Name, err)
		}
	}

	performers, teams := models.LineupNamesByKind(entries)
	if _, err := tx.Exec(ctx, `
		UPDATE events SET performers = $2, teams = $3, updated_at = NOW()
		WHERE id = $1
	`, eventID, performers, teams); err != nil {
		return fmt.Errorf("ReplaceEventLineup: legacy array sync: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ReplaceEventLineup: commit: %w", err)
	}
	return nil
}

// ApplyEventLineupFromEdit writes a line-up from an APPROVED edit request,
// inside the executor's transaction.
//
// Mirrors ApplyEventInfoFromEdit: the executor already holds a transaction and
// a row lock, so opening another here would deadlock against it. The legacy
// array sync happens here too — an approved line-up change must reach old app
// builds the same way a direct one does.
func ApplyEventLineupFromEdit(ctx context.Context, tx pgx.Tx, eventID string, entries []models.LineupEntry) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM event_lineup WHERE event_id = $1`, eventID,
	); err != nil {
		return fmt.Errorf("apply lineup: delete: %w", err)
	}

	for _, e := range entries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_lineup
				(event_id, kind, name, image_url, subtitle, stage, set_time, external_url, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, eventID, string(e.Kind), e.Name, e.ImageURL, e.Subtitle,
			e.Stage, e.SetTime, e.ExternalURL, e.SortOrder,
		); err != nil {
			return fmt.Errorf("apply lineup: insert %q: %w", e.Name, err)
		}
	}

	performers, teams := models.LineupNamesByKind(entries)
	if _, err := tx.Exec(ctx, `
		UPDATE events SET performers = $2, teams = $3 WHERE id = $1
	`, eventID, performers, teams); err != nil {
		return fmt.Errorf("apply lineup: legacy array sync: %w", err)
	}
	return nil
}