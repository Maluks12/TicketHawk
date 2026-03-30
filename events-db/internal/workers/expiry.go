package workers

import (
	"context"
	"events-db/internal/cache"
	"events-db/internal/repository"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartExpiryWorker(pool *pgxpool.Pool, ec *cache.EventCache, ctx context.Context, ttl time.Duration, interval time.Duration) {
	viewTicker := time.NewTicker(60 * time.Second) // flush views every 60s

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer viewTicker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := expireBookings(pool, ctx, ttl); err != nil {
					log.Printf("expiry worker: bookings: %v", err)
				}
				if err := expireReservations(pool, ctx); err != nil {
					log.Printf("expiry worker: reservations: %v", err)
				}
				if err := cleanupStaleCarts(pool, ctx); err != nil {
					log.Printf("expiry worker: carts: %v", err)
				}
				if err := expirePendingAssignments(pool, ctx); err != nil {   // ← add
		          log.Printf("expiry worker: assignments: %v", err)
     			}
			case <-viewTicker.C:
				flushViewCounts(pool, ec, ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func flushViewCounts(pool *pgxpool.Pool, ec *cache.EventCache, ctx context.Context) {
	if ec == nil {
		return
	}
	counts := ec.FlushViews(ctx)
	if len(counts) == 0 {
		return
	}

	for eventID, count := range counts {
		_, err := pool.Exec(ctx,
			`UPDATE events SET views = COALESCE(views, 0) + $1, updated_at = NOW()
			 WHERE id = $2`, count, eventID)
		if err != nil {
			log.Printf("view flush: failed to update event %s: %v", eventID, err)
		}
	}
	log.Printf("view flush: updated %d events", len(counts))
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

// expireReservations releases ticket_reservations past their expires_at,
// decrementing reserved_count on the corresponding event atomically.
func expireReservations(pool *pgxpool.Pool, ctx context.Context) error {
  tx, err := pool.Begin(ctx)
  if err != nil {
    return err
  }
  defer tx.Rollback(ctx)
 
  rows, err := tx.Query(ctx, `
    SELECT id, event_id, quantity FROM ticket_reservations
    WHERE expires_at < NOW()
    FOR UPDATE SKIP LOCKED
  `)
  if err != nil {
    return err
  }
  defer rows.Close()
 
  type expiredRes struct{ id, eventID string; qty int }
  var expired []expiredRes
  for rows.Next() {
    var r expiredRes
    if err := rows.Scan(&r.id, &r.eventID, &r.qty); err != nil {
      return err
    }
    expired = append(expired, r)
  }
  rows.Close()
  if len(expired) == 0 {
    return nil
  }
 
  for _, r := range expired {
    tx.Exec(ctx,
      `UPDATE events SET reserved_count = GREATEST(0, reserved_count - $1), updated_at = NOW()
       WHERE id = $2`, r.qty, r.eventID)
    tx.Exec(ctx, `DELETE FROM ticket_reservations WHERE id = $1`, r.id)
  }
 
  if err := tx.Commit(ctx); err != nil {
    return err
  }
  log.Printf("expiry worker: released %d expired reservations", len(expired))
  return nil
}
 
// cleanupStaleCarts deletes carts inactive for more than 24 hours.
func cleanupStaleCarts(pool *pgxpool.Pool, ctx context.Context) error {
  cmd, err := pool.Exec(ctx,
    `DELETE FROM carts WHERE last_activity_at < NOW() - INTERVAL '24 hours'`)
  if err != nil {
    return err
  }
  if cmd.RowsAffected() > 0 {
    log.Printf("expiry worker: cleaned up %d stale carts", cmd.RowsAffected())
  }
  return nil
}

func expirePendingAssignments(pool *pgxpool.Pool, ctx context.Context) error {
	return repository.ExpirePendingAssignments(pool, ctx)
}