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
    return nil, errors.New("cart has no ticket items")
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
    WHERE cart_id = $1
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
