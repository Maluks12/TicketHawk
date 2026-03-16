package repository
 
import (
  "context"
  "errors"
  "events-db/internal/models"
  "fmt"
  "time"
 
  "github.com/jackc/pgx/v5/pgxpool"
)
 
var ErrReservationNotFound = errors.New("reservation not found")
var ErrInsufficientReservationCapacity = errors.New("insufficient ticket capacity for reservation")
var ErrNoTicketItems = errors.New("cart has no ticket items")
 
// CreateReservations atomically creates reservations for all ticket items in the cart.
// It checks capacity (tickets_sold + reserved_count + qty <= max_capacity) inside a
// single transaction, rolling back everything if any event is sold out.
func CreateReservations(pool *pgxpool.Pool, cartID string, userID, sessionID *string) ([]models.TicketReservation, error) {
  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
 
  tx, err := pool.Begin(ctx)
  if err != nil {
    return nil, err
  }
  defer tx.Rollback(ctx)
 
  // Fetch all ticket items in the cart grouped by event
  rows, err := tx.Query(ctx, `
    SELECT event_id, SUM(quantity)::int AS total_qty
    FROM cart_items
  WHERE cart_id = $1 AND item_type = 'ticket'
    GROUP BY event_id
  `, cartID)
  if err != nil {
    return nil, fmt.Errorf("failed to query ticket items: %w", err)
  }
  defer rows.Close()
 
  type eventQty struct { eventID string; qty int }
  var ticketGroups []eventQty
  for rows.Next() {
    var eg eventQty
    if err := rows.Scan(&eg.eventID, &eg.qty); err != nil {
      return nil, err
    }
    ticketGroups = append(ticketGroups, eg)
  }
  rows.Close()
 
  if len(ticketGroups) == 0 {
    return nil, ErrNoTicketItems
  }
 
  // Delete any existing reservations for this cart (re-entering checkout)
  tx.Exec(ctx, `
    UPDATE events e SET reserved_count = GREATEST(0, reserved_count - tr.quantity)
    FROM ticket_reservations tr
    WHERE tr.cart_id = $1 AND e.id = tr.event_id
  `, cartID)
  tx.Exec(ctx, `DELETE FROM ticket_reservations WHERE cart_id = $1`, cartID)
 
  // For each event, check capacity and create reservation
  var reservations []models.TicketReservation
  for _, eg := range ticketGroups {
    var maxCapacity *int
    var ticketsSold, reservedCount int
 
    err := tx.QueryRow(ctx, `
      SELECT max_capacity, tickets_sold, reserved_count
      FROM events WHERE id = $1 AND status = 'published'
      FOR UPDATE
    `, eg.eventID).Scan(&maxCapacity, &ticketsSold, &reservedCount)
    if err != nil {
      return nil, fmt.Errorf("event %s not found or not published", eg.eventID)
    }
 
    if maxCapacity != nil && ticketsSold+reservedCount+eg.qty > *maxCapacity {
      return nil, fmt.Errorf("%w: event %s", ErrInsufficientReservationCapacity, eg.eventID)
    }
 
    // Increment reserved_count
    _, err = tx.Exec(ctx,
      `UPDATE events SET reserved_count = reserved_count + $1, updated_at = NOW()
       WHERE id = $2`, eg.qty, eg.eventID)
    if err != nil {
      return nil, fmt.Errorf("failed to update reserved_count: %w", err)
    }
 
    // Insert reservation
    var r models.TicketReservation
    err = tx.QueryRow(ctx, `
      INSERT INTO ticket_reservations
        (cart_id, event_id, user_id, session_id, quantity)
      VALUES ($1, $2, $3, $4, $5)
      RETURNING id, cart_id, event_id, user_id, session_id,
                booking_number, quantity, expires_at, created_at
    `, cartID, eg.eventID, userID, sessionID, eg.qty,
    ).Scan(
      &r.ID, &r.CartID, &r.EventID, &r.UserID, &r.SessionID,
      &r.BookingNumber, &r.Quantity, &r.ExpiresAt, &r.CreatedAt,
    )
    if err != nil {
      return nil, fmt.Errorf("failed to create reservation: %w", err)
    }
    reservations = append(reservations, r)
  }
 
  if err := tx.Commit(ctx); err != nil {
    return nil, fmt.Errorf("reservation transaction failed: %w", err)
  }
  return reservations, nil
}
 
// RefreshReservations resets expires_at to NOW() + 10 minutes for all reservations
// belonging to the given cart. Returns the shortest remaining TTL in seconds.
func RefreshReservations(pool *pgxpool.Pool, cartID string) (int, error) {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  rows, err := pool.Query(ctx, `
    UPDATE ticket_reservations
    SET expires_at = NOW() + INTERVAL '10 minutes'
    WHERE cart_id = $1 AND expires_at > NOW()
    RETURNING expires_at
  `, cartID)
  if err != nil {
    return 0, err
  }
  defer rows.Close()
 
  shortest := 600 // 10 minutes
  count := 0
  for rows.Next() {
    var expiresAt time.Time
    if err := rows.Scan(&expiresAt); err != nil {
      continue
    }
    secs := int(time.Until(expiresAt).Seconds())
    if secs < shortest {
      shortest = secs
    }
    count++
  }
 
  if count == 0 {
    return 0, errors.New("no active reservations found for this cart")
  }
  return shortest, nil
}
 
// AbandonReservations releases all reservations for a cart and decrements
// reserved_count on each event. Called on checkout page leave.
func AbandonReservations(pool *pgxpool.Pool, cartID string) error {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  tx, err := pool.Begin(ctx)
  if err != nil {
    return err
  }
  defer tx.Rollback(ctx)
 
  // Decrement reserved_count for each event
  _, err = tx.Exec(ctx, `
    UPDATE events e
    SET reserved_count = GREATEST(0, e.reserved_count - tr.quantity),
        updated_at = NOW()
    FROM ticket_reservations tr
    WHERE tr.cart_id = $1 AND e.id = tr.event_id
  `, cartID)
  if err != nil {
    return err
  }
 
  // Delete reservations
  tx.Exec(ctx, `DELETE FROM ticket_reservations WHERE cart_id = $1`, cartID)
 
  return tx.Commit(ctx)
}
 
// ConvertReservationsToBookings atomically converts every active reservation for
// a cart into a real pending booking row, then cleans up the reservation rows.
//
// What it does in one transaction:
//   - For each reservation: inserts a booking (status=pending) using the
//     reservation's booking_number, sets cart_id on the booking row.
//   - Moves capacity from reserved_count → tickets_sold on the event.
//   - Deletes the reservation rows.
//
// The bookings start as pending/pending. The payment webhook will confirm them
// once Stitch reports a completed payment.
//
// Returns the ordered list of new booking IDs (same order as reservations).
func ConvertReservationsToBookings(
	pool *pgxpool.Pool,
	cartID string,
	userID, sessionID *string,
) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Fetch active reservations for this cart
	rows, err := tx.Query(ctx, `
		SELECT id, event_id, booking_number, quantity
		FROM ticket_reservations
		WHERE cart_id = $1 AND expires_at > NOW()
		ORDER BY created_at ASC
		FOR UPDATE
	`, cartID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reservations: %w", err)
	}

	type res struct {
		id, eventID, bookingNumber string
		qty                        int
	}
	var reservations []res
	for rows.Next() {
		var r res
		if err := rows.Scan(&r.id, &r.eventID, &r.bookingNumber, &r.qty); err != nil {
			rows.Close()
			return nil, err
		}
		reservations = append(reservations, r)
	}
	rows.Close()

	if len(reservations) == 0 {
		return nil, errors.New("no active reservations found for this cart")
	}

	// Fetch unit prices from cart_items for each event
	priceRows, err := tx.Query(ctx, `
		SELECT event_id, unit_price_snapshot
		FROM cart_items
		WHERE cart_id = $1 AND item_type = 'ticket'
	`, cartID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cart prices: %w", err)
	}
	eventPrices := map[string]float64{}
	for priceRows.Next() {
		var eID string
		var price float64
		if err := priceRows.Scan(&eID, &price); err != nil {
			priceRows.Close()
			return nil, err
		}
		eventPrices[eID] = price
	}
	priceRows.Close()

	var bookingIDs []string

	for _, r := range reservations {
		unitPrice := eventPrices[r.eventID]
		subtotal := unitPrice * float64(r.qty)
		// Use same platform fee calculation as the existing booking flow
		platformFee := CalculatePlatformFee(subtotal)
		totalAmount := subtotal + platformFee

		// Determine guest flag and attendee email from cart
		isGuest := userID == nil
		var attendeeEmail *string
		if sessionID != nil {
			// Attempt to fetch guest email from cart — best-effort, NULL is fine
			var email string
			tx.QueryRow(ctx, `
				SELECT attendee_email FROM cart_items
				WHERE cart_id = $1 AND event_id = $2 AND item_type = 'ticket'
				LIMIT 1
			`, cartID, r.eventID).Scan(&email)
			if email != "" {
				attendeeEmail = &email
			}
		}

		var bookingID string
		err = tx.QueryRow(ctx, `
			INSERT INTO bookings (
				booking_number, event_id, user_id, is_guest,
				ticket_quantity, unit_price, subtotal,
				platform_fee, organizer_fee, total_amount, currency,
				attendee_email,
				booking_status, payment_status,
				cart_id
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7,
				$8, 0, $9, 'ZAR',
				$10,
				'pending', 'pending',
				$11
			)
			RETURNING id
		`,
			r.bookingNumber, r.eventID, userID, isGuest,
			r.qty, unitPrice, subtotal,
			platformFee, totalAmount,
			attendeeEmail,
			cartID,
		).Scan(&bookingID)
		if err != nil {
			return nil, fmt.Errorf("failed to create booking for event %s: %w", r.eventID, err)
		}
		bookingIDs = append(bookingIDs, bookingID)

		// Move quantity from reserved_count to tickets_sold atomically
		_, err = tx.Exec(ctx, `
			UPDATE events
			SET tickets_sold   = tickets_sold   + $1,
			    reserved_count = GREATEST(0, reserved_count - $1),
			    updated_at     = NOW()
			WHERE id = $2
		`, r.qty, r.eventID)
		if err != nil {
			return nil, fmt.Errorf("failed to update event capacity for %s: %w", r.eventID, err)
		}

		// Delete the reservation row
		tx.Exec(ctx, `DELETE FROM ticket_reservations WHERE id = $1`, r.id)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("ConvertReservationsToBookings commit failed: %w", err)
	}
	return bookingIDs, nil
}

// GetReservationsByCart returns all active (non-expired) reservations for a cart.
func GetReservationsByCart(pool *pgxpool.Pool, cartID string) ([]models.TicketReservation, error) {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  rows, err := pool.Query(ctx, `
    SELECT id, cart_id, event_id, user_id, session_id,
           booking_number, quantity, expires_at, created_at
    FROM ticket_reservations
    WHERE cart_id = $1 AND expires_at > NOW()
    ORDER BY created_at ASC
  `, cartID)
  if err != nil {
    return nil, err
  }
  defer rows.Close()
 
  var reservations []models.TicketReservation
  for rows.Next() {
    var r models.TicketReservation
    if err := rows.Scan(
      &r.ID, &r.CartID, &r.EventID, &r.UserID, &r.SessionID,
      &r.BookingNumber, &r.Quantity, &r.ExpiresAt, &r.CreatedAt,
    ); err != nil {
      return nil, err
    }
    reservations = append(reservations, r)
  }
  return reservations, nil
}