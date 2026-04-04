package repository

// This file contains small utility repository functions used by handlers
// that need aggregated data without fetching full model structs.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SumBookingTotalsByIDs returns the sum of total_amount for the given booking
// IDs in a single query. This replaces the per-booking GetBookingByID loop
// that was the fallback path in CheckoutConfirmHandler.
func SumBookingTotalsByIDs(pool *pgxpool.Pool, ids []string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var total float64
	err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(total_amount), 0)
		 FROM bookings
		 WHERE id = ANY($1)`,
		ids,
	).Scan(&total)
	return total, err
}