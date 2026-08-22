package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// applyEventLineupEdit writes an event's line-up from an APPROVED edit request,
// inside the executor's existing transaction.
//
// The direct endpoint (PUT /organiser/events/:id/lineup) is NOT review-gated —
// line-ups change and the accurate version needs to reach the page the same
// day. This path exists for the OTHER route into the same data: an admin
// correcting a line-up through the console files an event_edit like any other
// change, and without this the request would execute with the line-up silently
// skipped while reporting success.
//
// Mirrors applyEventInfoEdit: takes a pgx.Tx because the executor already holds
// one and a row lock, and re-validating here against rules that may have moved
// would strand a request an admin already approved.
func applyEventLineupEdit(ctx context.Context, tx pgx.Tx, eventID string, raw []any) error {
	type pending struct {
		kind        string
		name        string
		imageURL    *string
		subtitle    *string
		stage       *string
		setTime     *time.Time
		externalURL *string
	}

	entries := make([]pending, 0, len(raw))
	seen := make(map[string]bool, len(raw))

	// Names for the legacy array sync. See ReplaceEventLineup for why the
	// arrays are still written.
	performers := make([]string, 0, len(raw))
	teams := make([]string, 0, 4)

	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		name, _ := m["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		kind, _ := m["kind"].(string)
		kind = strings.TrimSpace(kind)
		if kind != "team" {
			kind = "performer"
		}

		dupKey := kind + "\x00" + strings.ToLower(name)
		if seen[dupKey] {
			return fmt.Errorf("line-up: duplicate entry %q", name)
		}
		seen[dupKey] = true

		p := pending{kind: kind, name: name}
		p.imageURL = optString(m, "image_url")
		p.subtitle = optString(m, "subtitle")
		p.stage = optString(m, "stage")
		p.externalURL = optString(m, "external_url")

		// json.Unmarshal leaves timestamps as RFC3339 strings.
		if s, ok := m["set_time"].(string); ok && strings.TrimSpace(s) != "" {
			t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
			if err != nil {
				return fmt.Errorf("line-up: %q has an unreadable set time: %w", name, err)
			}
			p.setTime = &t
		}

		entries = append(entries, p)
		if kind == "team" {
			teams = append(teams, name)
		} else {
			performers = append(performers, name)
		}
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM event_lineup WHERE event_id = $1`, eventID,
	); err != nil {
		return fmt.Errorf("line-up: clear existing: %w", err)
	}

	for i, e := range entries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_lineup
				(event_id, kind, name, image_url, subtitle, stage, set_time, external_url, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, eventID, e.kind, e.name, e.imageURL, e.subtitle,
			e.stage, e.setTime, e.externalURL, i,
		); err != nil {
			return fmt.Errorf("line-up: write %q: %w", e.name, err)
		}
	}

	// Keeps shipped mobile builds in step — they read these arrays directly.
	if _, err := tx.Exec(ctx, `
		UPDATE events SET performers = $2, teams = $3 WHERE id = $1
	`, eventID, performers, teams); err != nil {
		return fmt.Errorf("line-up: legacy array sync: %w", err)
	}

	return nil
}

// optString pulls a trimmed, non-empty string out of a decoded JSON map.
// Returns nil for absent, wrong-typed, or blank values so the column stays
// NULL rather than holding an empty string — the two render differently and
// only one of them means "not set".
func optString(m map[string]any, key string) *string {
	v, ok := m[key].(string)
	if !ok {
		return nil
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}