package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GetUnsentConfirmedBookingIDs returns the IDs of confirmed bookings from the
// last 24 hours whose tickets have not yet been sent. Called once at startup to
// re-enqueue any jobs that were in-flight when the previous server process died.
//
// Previously this query lived inline in main.go. Keeping raw SQL out of main.go
// makes it testable, reusable, and visible to the rest of the data layer.
func GetUnsentConfirmedBookingIDs(pool *pgxpool.Pool) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT id
		FROM   bookings
		WHERE  booking_status = 'confirmed'
		AND    tickets_sent   = FALSE
		AND    created_at     > NOW() - INTERVAL '24 hours'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}