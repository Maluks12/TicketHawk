package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// applyEventInfoEdit writes an event's information block from an APPROVED edit
// request, inside the executor's existing transaction.
//
// ── Why this lives here and not in the repository ───────────────────────────
//
// The executor already holds a transaction and a row lock on the request.
// Calling into a repository function that opens its own pool transaction would
// deadlock against it. Everything applied by request_execute_handler.go takes
// a pgx.Tx for the same reason — see applyTicketTypeEdits, which this mirrors.
//
// ── Why it does not re-validate ─────────────────────────────────────────────
//
// The payload was validated by ValidateEventInfoItems when the request was
// FILED. Re-validating here, against a catalogue that may have changed in the
// meantime, would strand a request an admin has already approved — the
// organiser would see "approved" and nothing would happen, with the failure
// visible only in the logs. The allow-list below is a safety net against a
// malformed payload, not a second validation pass.
//
// ── Why it replaces rather than merges ──────────────────────────────────────
//
// The information block is edited as a whole, so the payload IS the intended
// final state. Merging would make it impossible to remove a field through the
// review flow, which matters: removing an age restriction from a live event is
// exactly as material as changing it, and is one of the things review exists to
// catch.
func applyEventInfoEdit(ctx context.Context, tx pgx.Tx, eventID string, raw []any) error {
	type pending struct {
		key   string
		value string
	}

	items := make([]pending, 0, len(raw))
	seen := make(map[string]bool, len(raw))

	for _, entry := range raw {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}

		key, _ := m["key"].(string)
		key = strings.ToLower(strings.TrimSpace(key))
		value, _ := m["value"].(string)
		value = strings.TrimSpace(value)

		if key == "" || value == "" {
			// An empty value means the field was not chosen. It gets no row,
			// which is what makes an unchosen field impossible to render.
			continue
		}
		if seen[key] {
			return fmt.Errorf("event info: duplicate field %q", key)
		}
		seen[key] = true

		if len([]rune(value)) > 600 {
			return fmt.Errorf("event info: %q is longer than 600 characters", key)
		}

		items = append(items, pending{key: key, value: value})
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM event_info_items WHERE event_id = $1`, eventID,
	); err != nil {
		return fmt.Errorf("event info: clear existing: %w", err)
	}

	for i, it := range items {
		// info_key is a foreign key onto event_info_catalogue, so an
		// unrecognised key fails here with 23503 rather than being silently
		// dropped. That is the behaviour we want at execution time: a payload
		// naming a field that no longer exists is a real problem an admin
		// should hear about, not something to quietly skip.
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_info_items (event_id, info_key, value, sort_order)
			VALUES ($1, $2, $3, $4)
		`, eventID, it.key, it.value, i); err != nil {
			return fmt.Errorf("event info: write %q: %w", it.key, err)
		}
	}

	return nil
}