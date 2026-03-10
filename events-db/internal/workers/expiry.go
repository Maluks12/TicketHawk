package workers

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartExpiryWorker(pool *pgxpool.Pool, ctx context.Context, ttl time.Duration, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select{
			case <-ticker.C:
				if err := expireBookings(pool, ctx, ttl); err != nil {
					log.Printf("expiry worker: %v", err)
				}
			case <- ctx.Done():
				return
			}
		}
	} ()
}

func expireBookings(pool *pgxpool.Pool, ctx context.Context, ttl time.Duration) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	query := `
		SELECT id, event_id, ticket_quantity
		FROM bookings
		WHERE booking_status = 'pending'
		AND payment_status = 'pending'
		AND created_at < NOW() - $1::interval
		FOR UPDATE SKIP LOCKED
	`

	rows, err := tx.Query(ctx,query, ttl.String())
	if err != nil {
		return err
	}
	defer rows.Close()

	type expiredBooking struct {
		id 		string
		eventID string
		quantity int
	}

	var expired []expiredBooking
	for rows.Next() {
		var b expiredBooking
		if err := rows.Scan(&b.id, &b.eventID, &b.quantity); err != nil {
			return err
		}
		expired = append(expired, b)
	}
	rows.Close()

	if len(expired) == 0 {
		return nil
	}

	queryCancel := `
		UPDATE bookings
		SET booking_status = 'cancelled',
		updated_at     = NOW()
		WHERE id = $1
	`
	

	queryRestore := `
		UPDATE events
		SET tickets_sold = GREATEST(0, tickets_sold - $1),
		updated_at = NOW()
		WHERE id = $2
	`

	for _, b := range expired {
		// Cancel booking
		_, err = tx.Exec(ctx, queryCancel,b.id)
		if err != nil {
			return err
		}
		// Restore ticket count
		_, err = tx.Exec(ctx, queryRestore, b.quantity, b.eventID)
		if err != nil {
			return err
		}
	}


	if err := tx.Commit(ctx); err != nil {
		return err
	}

	log.Printf("expiry worker: cancelled %d expired bookings", len(expired))
	return nil
}