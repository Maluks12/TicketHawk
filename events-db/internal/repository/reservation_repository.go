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

// CreateReservations atomically creates reservations for all ticket items in
// the cart. It checks capacity at both the event level AND the ticket type
// level (when present) inside a single transaction, rolling back everything if
// any check fails.
func CreateReservations(
	pool *pgxpool.Pool,
	cartID string,
	userID, sessionID *string,
) ([]models.TicketReservation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Fetch all ticket items grouped by event + ticket_type (NULLs included)
	rows, err := tx.Query(ctx, `
		SELECT event_id, ticket_type_id, SUM(quantity)::int AS total_qty
		FROM cart_items
		WHERE cart_id = $1 AND item_type = 'ticket'
		GROUP BY event_id, ticket_type_id
	`, cartID)
	if err != nil {
		return nil, fmt.Errorf("failed to query ticket items: %w", err)
	}
	defer rows.Close()

	type itemGroup struct {
		eventID      string
		ticketTypeID *string
		qty          int
	}
	var ticketGroups []itemGroup
	for rows.Next() {
		var eg itemGroup
		if err := rows.Scan(&eg.eventID, &eg.ticketTypeID, &eg.qty); err != nil {
			return nil, err
		}
		ticketGroups = append(ticketGroups, eg)
	}
	rows.Close()

	if len(ticketGroups) == 0 {
		return nil, ErrNoTicketItems
	}

	// Release any existing reservations for this cart before re-creating them.
	//
	// Two things this MUST get right, both of which the previous version did not:
	//
	//  1. ticket_types.reserved_count has to come back too. The loop below
	//     increments it, so releasing only events.reserved_count made the type
	//     counter ratchet upward on every re-entry into checkout until the type
	//     reported sold out with no real demand behind it.
	//
	//  2. The decrement must be pre-aggregated per target row. A cart holding
	//     two ticket types for the SAME event produces two ticket_reservations
	//     rows joining one events row; Postgres applies only one arbitrary match
	//     in UPDATE ... FROM, silently stranding the other quantity forever.
	//     Grouping first guarantees exactly one source row per target row.
	if _, err := tx.Exec(ctx, `
		UPDATE ticket_types tt
		SET reserved_count = GREATEST(0, tt.reserved_count - agg.qty),
		    updated_at     = NOW()
		FROM (
			SELECT ticket_type_id, SUM(quantity)::int AS qty
			FROM ticket_reservations
			WHERE cart_id = $1 AND ticket_type_id IS NOT NULL
			GROUP BY ticket_type_id
		) agg
		WHERE tt.id = agg.ticket_type_id
	`, cartID); err != nil {
		return nil, fmt.Errorf("failed to release prior ticket type reservations: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE events e
		SET reserved_count = GREATEST(0, e.reserved_count - agg.qty),
		    updated_at     = NOW()
		FROM (
			SELECT event_id, SUM(quantity)::int AS qty
			FROM ticket_reservations
			WHERE cart_id = $1
			GROUP BY event_id
		) agg
		WHERE e.id = agg.event_id
	`, cartID); err != nil {
		return nil, fmt.Errorf("failed to release prior event reservations: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM ticket_reservations WHERE cart_id = $1`, cartID); err != nil {
		return nil, fmt.Errorf("failed to delete prior reservations: %w", err)
	}

	var reservations []models.TicketReservation

	for _, eg := range ticketGroups {
		// ---------------------------------------------------------------
		// 1. Event-level capacity check
		// ---------------------------------------------------------------
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

		// ---------------------------------------------------------------
		// 2. Ticket-type-level capacity check (when a type was chosen)
		// ---------------------------------------------------------------
		if eg.ticketTypeID != nil {
			var ttCapacity *int
			var ttSold, ttReserved int

			err := tx.QueryRow(ctx, `
				SELECT capacity, tickets_sold, reserved_count
				FROM ticket_types
				WHERE id = $1
				FOR UPDATE
			`, *eg.ticketTypeID).Scan(&ttCapacity, &ttSold, &ttReserved)
			if err != nil {
				return nil, fmt.Errorf("ticket type %s not found", *eg.ticketTypeID)
			}

			if ttCapacity != nil && ttSold+ttReserved+eg.qty > *ttCapacity {
				return nil, fmt.Errorf("%w: ticket type %s is sold out", ErrInsufficientReservationCapacity, *eg.ticketTypeID)
			}

			// Increment ticket type's reserved_count
			_, err = tx.Exec(ctx,
				`UPDATE ticket_types
				 SET reserved_count = reserved_count + $1, updated_at = NOW()
				 WHERE id = $2`, eg.qty, *eg.ticketTypeID)
			if err != nil {
				return nil, fmt.Errorf("failed to update ticket type reserved_count: %w", err)
			}
		}

		// ---------------------------------------------------------------
		// 3. Increment event reserved_count
		// ---------------------------------------------------------------
		_, err = tx.Exec(ctx,
			`UPDATE events SET reserved_count = reserved_count + $1, updated_at = NOW()
			 WHERE id = $2`, eg.qty, eg.eventID)
		if err != nil {
			return nil, fmt.Errorf("failed to update event reserved_count: %w", err)
		}

		// ---------------------------------------------------------------
		// 4. Insert reservation row (now carries ticket_type_id)
		// ---------------------------------------------------------------
		var r models.TicketReservation
		err = tx.QueryRow(ctx, `
			INSERT INTO ticket_reservations
				(cart_id, event_id, user_id, session_id, quantity, ticket_type_id)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, cart_id, event_id, user_id, session_id,
			          booking_number, quantity, ticket_type_id, expires_at, created_at
		`, cartID, eg.eventID, userID, sessionID, eg.qty, eg.ticketTypeID,
		).Scan(
			&r.ID, &r.CartID, &r.EventID, &r.UserID, &r.SessionID,
			&r.BookingNumber, &r.Quantity, &r.TicketTypeID, &r.ExpiresAt, &r.CreatedAt,
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

// RefreshReservations resets expires_at to NOW() + 10 minutes for all active
// reservations belonging to the given cart.
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
// reserved_count on both events and ticket types.
func AbandonReservations(pool *pgxpool.Pool, cartID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Decrement reserved_count on ticket types.
	//
	// Pre-aggregated per ticket type: a cart can legitimately hold more than one
	// reservation row per type, and UPDATE ... FROM only applies one arbitrary
	// matching source row per target row.
	_, err = tx.Exec(ctx, `
		UPDATE ticket_types tt
		SET reserved_count = GREATEST(0, tt.reserved_count - agg.qty),
		    updated_at     = NOW()
		FROM (
			SELECT ticket_type_id, SUM(quantity)::int AS qty
			FROM ticket_reservations
			WHERE cart_id = $1 AND ticket_type_id IS NOT NULL
			GROUP BY ticket_type_id
		) agg
		WHERE tt.id = agg.ticket_type_id
	`, cartID)
	if err != nil {
		return fmt.Errorf("AbandonReservations: failed to decrement ticket type reserved_count: %w", err)
	}

	// Decrement reserved_count on events.
	//
	// Same aggregation, and this is the case that actually bit: a mixed GA + VIP
	// cart for one event released only ONE of the two quantities, permanently
	// stranding the rest as phantom reserved capacity on every abandon.
	_, err = tx.Exec(ctx, `
		UPDATE events e
		SET reserved_count = GREATEST(0, e.reserved_count - agg.qty),
		    updated_at     = NOW()
		FROM (
			SELECT event_id, SUM(quantity)::int AS qty
			FROM ticket_reservations
			WHERE cart_id = $1
			GROUP BY event_id
		) agg
		WHERE e.id = agg.event_id
	`, cartID)
	if err != nil {
		return fmt.Errorf("AbandonReservations: failed to decrement event reserved_count: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM ticket_reservations WHERE cart_id = $1`, cartID); err != nil {
		return fmt.Errorf("AbandonReservations: failed to delete reservation rows: %w", err)
	}

	return tx.Commit(ctx)
}

// ConvertReservationsToBookings atomically converts every active reservation
// for a cart into a real pending booking row, then cleans up reservation rows.
// Now carries ticket_type_id and snapshots the type name into bookings for
// receipts and emails.
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

	// Fetch active reservations including ticket_type_id
	rows, err := tx.Query(ctx, `
		SELECT id, event_id, booking_number, quantity, ticket_type_id
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
		ticketTypeID               *string
	}
	var reservations []res
	for rows.Next() {
		var r res
		if err := rows.Scan(&r.id, &r.eventID, &r.bookingNumber, &r.qty, &r.ticketTypeID); err != nil {
			rows.Close()
			return nil, err
		}
		reservations = append(reservations, r)
	}
	rows.Close()

	if len(reservations) == 0 {
		return nil, errors.New("no active reservations found for this cart")
	}

	// Fetch unit prices from cart_items, keyed by event_id + ticket_type_id
	type cartKey struct{ eventID, ticketTypeID string }
	priceRows, err := tx.Query(ctx, `
		SELECT event_id, COALESCE(ticket_type_id::text, ''), unit_price_snapshot
		FROM cart_items
		WHERE cart_id = $1 AND item_type = 'ticket'
	`, cartID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cart prices: %w", err)
	}
	cartPrices := map[cartKey]float64{}
	for priceRows.Next() {
		var eID, ttID string
		var price float64
		if err := priceRows.Scan(&eID, &ttID, &price); err != nil {
			priceRows.Close()
			return nil, err
		}
		cartPrices[cartKey{eID, ttID}] = price
	}
	priceRows.Close()

	var bookingIDs []string

	for _, r := range reservations {
		// Resolve price: prefer event+type key, fall back to event-only key
		ttKeyStr := ""
		if r.ticketTypeID != nil {
			ttKeyStr = *r.ticketTypeID
		}
		unitPrice, ok := cartPrices[cartKey{r.eventID, ttKeyStr}]
		if !ok {
			unitPrice = cartPrices[cartKey{r.eventID, ""}]
		}

		// Each reservation can belong to a different organiser, so the fee
		// terms are resolved per line rather than once per cart.
		terms, err := GetFeeTermsForEvent(ctx, tx, r.eventID)
		if err != nil {
			return nil, err
		}

		subtotal := unitPrice * float64(r.qty)
		serviceFee, commission := CalculateFees(subtotal, r.qty, terms)
		platformFee := serviceFee  // buyer-paid service fee (per ticket × qty, capped)
		organizerFee := commission // organizer-paid commission (rate × subtotal)
		// Buyer pays ticket price + service fee; commission is deducted from the
		// organizer's payout, so it is not part of total_amount.
		totalAmount := subtotal + platformFee

		isGuest := userID == nil

		// Snapshot ticket type name for receipts/emails
		var ticketTypeName *string
		if r.ticketTypeID != nil {
			var name string
			tx.QueryRow(ctx, `SELECT name FROM ticket_types WHERE id = $1`, *r.ticketTypeID).Scan(&name)
			if name != "" {
				ticketTypeName = &name
			}
		}

		// Attempt to fetch guest email from cart — best-effort, NULL is fine
		var attendeeEmail *string
		if sessionID != nil {
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
				ticket_type_id, ticket_type_name,
				ticket_quantity, unit_price, subtotal,
				platform_fee, organizer_fee, total_amount, currency,
				attendee_email,
				booking_status, payment_status,
				cart_id
			) VALUES (
				$1, $2, $3, $4,
				$5, $6,
				$7, $8, $9,
				$10, $11, $12, 'ZAR',
				$13,
				'pending', 'pending',
				$14
			)
			RETURNING id
		`,
			r.bookingNumber, r.eventID, userID, isGuest,
			r.ticketTypeID, ticketTypeName,
			r.qty, unitPrice, subtotal,
			platformFee, organizerFee, totalAmount,
			attendeeEmail,
			cartID,
		).Scan(&bookingID)
		if err != nil {
			return nil, fmt.Errorf("failed to create booking for event %s: %w", r.eventID, err)
		}
		bookingIDs = append(bookingIDs, bookingID)

		// Move capacity from reserved_count → tickets_sold on the event
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

		// Move capacity on ticket type too (when present)
		if r.ticketTypeID != nil {
			_, err = tx.Exec(ctx, `
				UPDATE ticket_types
				SET tickets_sold   = tickets_sold   + $1,
				    reserved_count = GREATEST(0, reserved_count - $1),
				    updated_at     = NOW()
				WHERE id = $2
			`, r.qty, *r.ticketTypeID)
			if err != nil {
				return nil, fmt.Errorf("failed to update ticket type capacity for %s: %w", *r.ticketTypeID, err)
			}
		}

		// Delete reservation row
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
		       booking_number, quantity, ticket_type_id, expires_at, created_at
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
			&r.BookingNumber, &r.Quantity, &r.TicketTypeID, &r.ExpiresAt, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		reservations = append(reservations, r)
	}
	return reservations, nil
}