package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Group pool admissions ─────────────────────────────────────────────────────
//
// A booking's "pool" is the set of tickets the buyer still holds: unassigned
// rows that haven't been scanned in yet. The buyer's group QR admits from this
// pool. Assigned tickets (pending/claimed) leave the pool and carry their own
// individual QR, so they are never touched here.

// CountPoolRemaining returns how many admissions are still available in a
// booking's pool (unassigned AND not yet scanned).
func CountPoolRemaining(ctx context.Context, pool *pgxpool.Pool, bookingID string) (int, error) {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM ticket_assignments
		WHERE booking_id = $1
		  AND status = 'unassigned'
		  AND qr_scanned_at IS NULL
	`, bookingID).Scan(&n)
	return n, err
}

// AdmitFromPool atomically admits up to `count` heads from a booking's pool and
// returns the slot numbers actually admitted (fewer than `count` if the pool ran
// low; empty if it was already drained).
//
// Race-safety: the inner SELECT ... FOR UPDATE SKIP LOCKED makes concurrent
// scanners lock DISJOINT rows — two simultaneous admits can never mark the same
// slot, and neither blocks the other. This is the pool analogue of the single
// ticket's "WHERE qr_scanned_at IS NULL" guard.
func AdmitFromPool(
	ctx context.Context,
	pool *pgxpool.Pool,
	bookingID string,
	count int,
	scannerUserID string,
) ([]int, error) {
	if count <= 0 {
		return []int{}, nil
	}

	rows, err := pool.Query(ctx, `
		WITH picked AS (
			SELECT id
			FROM ticket_assignments
			WHERE booking_id = $1
			  AND status = 'unassigned'
			  AND qr_scanned_at IS NULL
			ORDER BY slot_number
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE ticket_assignments t
		SET qr_scanned_at           = NOW(),
		    scanned_by_organizer_id = (SELECT id FROM organizers WHERE user_id = $3 LIMIT 1)
		FROM picked
		WHERE t.id = picked.id
		RETURNING t.slot_number
	`, bookingID, count, scannerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slots := make([]int, 0, count)
	for rows.Next() {
		var sn int
		if err := rows.Scan(&sn); err != nil {
			return nil, err
		}
		slots = append(slots, sn)
	}
	return slots, rows.Err()
}

// ── Scan idempotency ──────────────────────────────────────────────────────────

// GetScanIdempotency returns a previously-recorded admission for an idempotency
// key. found=false means this is a first-time key (caller should perform the
// admit and then SaveScanIdempotency).
func GetScanIdempotency(ctx context.Context, pool *pgxpool.Pool, key string) (found bool, admitted int, err error) {
	err = pool.QueryRow(ctx, `
		SELECT admitted_count
		FROM scan_idempotency
		WHERE idempotency_key = $1
	`, key).Scan(&admitted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	return true, admitted, nil
}

// SaveScanIdempotency records the result of a pool admission so a retried
// request with the same key returns the same result instead of re-admitting.
// ON CONFLICT DO NOTHING keeps it safe under a genuine race on the same key.
func SaveScanIdempotency(
	ctx context.Context,
	pool *pgxpool.Pool,
	key, bookingID string,
	admitted int,
	scannerUserID string,
) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO scan_idempotency (idempotency_key, booking_id, admitted_count, scanned_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (idempotency_key) DO NOTHING
	`, key, bookingID, admitted, scannerUserID)
	return err
}