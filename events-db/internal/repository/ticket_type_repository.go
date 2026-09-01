package repository

import (
	"context"
	"errors"
	"events-db/internal/models"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTicketTypeNotFound = errors.New("ticket type not found")

// GeneralTierName is the tier a flat-priced event is converted into the first
// time a second tier is added. Fixed rather than configurable so the backfill
// and the organiser UI agree on what the original product was called.
const GeneralTierName = "General"

// raiseEventCapacityToTierSum grows events.max_capacity to at least the sum of
// its tiers' capacities. It NEVER shrinks it.
//
// Tier capacities are an allocation WITHIN max_capacity, not an addition to it:
// ComputeAvailableTickets clamps every tier to (max_capacity - tickets_sold),
// so adding a 5-seat VIP tier to a 10-seat event that has sold 10 sells nothing
// at all. Raising the ceiling here is what makes "add a tier to sell more"
// behave the way an organiser expects.
//
// One-directional on purpose. max_capacity stays independently editable — an
// organiser can cap a 15-seat allocation at 12 because the venue changed — and
// an auto-shrink would silently undo that the next time any tier was touched.
// Deleting a tier likewise leaves the ceiling where it is.
//
// Skipped entirely when any tier is uncapped, since an unlimited tier has no
// number to add and a sum of the rest would be meaninglessly low.
func raiseEventCapacityToTierSum(tx pgx.Tx, ctx context.Context, eventID string) error {
	var uncapped int
	var tierSum int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE capacity IS NULL),
		       COALESCE(SUM(capacity), 0)
		FROM ticket_types
		WHERE event_id = $1
	`, eventID).Scan(&uncapped, &tierSum); err != nil {
		return fmt.Errorf("raiseEventCapacityToTierSum reading tiers: %w", err)
	}
	if uncapped > 0 || tierSum == 0 {
		return nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE events
		SET max_capacity = GREATEST(COALESCE(max_capacity, 0), $2),
		    updated_at   = NOW()
		WHERE id = $1
		  AND (max_capacity IS NULL OR max_capacity < $2)
	`, eventID, tierSum); err != nil {
		return fmt.Errorf("raiseEventCapacityToTierSum: %w", err)
	}
	return nil
}
var ErrTicketTypeForbidden = errors.New("ticket type belongs to a different organiser")
var ErrTicketTypeHasSales = errors.New("cannot delete a ticket type that already has bookings")

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

// GetTicketTypesByEventID returns all ticket types for an event ordered by
// sort_order, then name. Called when serving the event detail page and when
// resolving prices in the cart flow. Returns an empty slice (not nil) when
// the event has no types so callers can safely check len().
func GetTicketTypesByEventID(pool *pgxpool.Pool, eventID string) ([]*models.TicketType, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT id, event_id, name, description, price,
		       capacity, tickets_sold, reserved_count,
		       sort_order, is_visible, sale_starts_at, sale_ends_at,
		       created_at, updated_at
		FROM ticket_types
		WHERE event_id = $1
		ORDER BY sort_order ASC, name ASC
	`, eventID)
	if err != nil {
		return nil, fmt.Errorf("GetTicketTypesByEventID: %w", err)
	}
	defer rows.Close()

	var types []*models.TicketType
	for rows.Next() {
		tt := &models.TicketType{}
		if err := rows.Scan(
			&tt.ID, &tt.EventID, &tt.Name, &tt.Description, &tt.Price,
			&tt.Capacity, &tt.TicketsSold, &tt.ReservedCount,
			&tt.SortOrder, &tt.IsVisible, &tt.SaleStartsAt, &tt.SaleEndsAt,
			&tt.CreatedAt, &tt.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("GetTicketTypesByEventID scan: %w", err)
		}
		tt.ComputeAvailable()
		types = append(types, tt)
	}
	if types == nil {
		types = []*models.TicketType{}
	}
	return types, nil
}

// GetTicketTypeByID fetches a single type. Returns ErrTicketTypeNotFound when
// the row does not exist.
func GetTicketTypeByID(pool *pgxpool.Pool, id string) (*models.TicketType, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tt := &models.TicketType{}
	err := pool.QueryRow(ctx, `
		SELECT id, event_id, name, description, price,
		       capacity, tickets_sold, reserved_count,
		       sort_order, is_visible, sale_starts_at, sale_ends_at,
		       created_at, updated_at
		FROM ticket_types WHERE id = $1
	`, id).Scan(
		&tt.ID, &tt.EventID, &tt.Name, &tt.Description, &tt.Price,
		&tt.Capacity, &tt.TicketsSold, &tt.ReservedCount,
		&tt.SortOrder, &tt.IsVisible, &tt.SaleStartsAt, &tt.SaleEndsAt,
		&tt.CreatedAt, &tt.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTicketTypeNotFound
		}
		return nil, fmt.Errorf("GetTicketTypeByID: %w", err)
	}
	tt.ComputeAvailable()
	return tt, nil
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateTicketTypes inserts one or more ticket types for an event in a single
// transaction. Verifies the event belongs to organizerID before inserting.
// After insert the DB trigger updates events.price automatically.
func CreateTicketTypes(
	pool *pgxpool.Pool,
	eventID string,
	dtos []models.CreateTicketTypeDTO,
	organizerID string,
) ([]*models.TicketType, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify ownership
	var count int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE id = $1 AND organizer_id = $2`,
		eventID, organizerID,
	).Scan(&count)
	if err != nil || count == 0 {
		return nil, ErrTicketTypeForbidden
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// ── Materialise the flat ticket before adding anything beside it ───────
	//
	// A flat-priced event sells events.price against events.max_capacity with no
	// ticket_types row at all; the apps show a synthetic "GENERAL ADMISSION"
	// entry only while the tier list is empty. Add one tier and that synthetic
	// entry disappears — so an organiser selling 10 at R10 who adds a VIP tier
	// silently loses the R10 product. It was never deleted, it was never real.
	//
	// So the first tier creation turns the flat product into an actual tier and
	// moves the existing sales onto it. After this an event is EITHER flat with
	// no tiers, or fully tiered — never half of each.
	var existingTiers int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM ticket_types WHERE event_id = $1`, eventID,
	).Scan(&existingTiers); err != nil {
		return nil, fmt.Errorf("CreateTicketTypes counting tiers: %w", err)
	}

	// FOR UPDATE: max_capacity is read here and written at the end, and a
	// concurrent capacity edit between the two would be lost.
	var eventPrice float64
	var maxCapacity *int
	if err := tx.QueryRow(ctx, `
		SELECT price, max_capacity FROM events WHERE id = $1 FOR UPDATE
	`, eventID).Scan(&eventPrice, &maxCapacity); err != nil {
		return nil, fmt.Errorf("CreateTicketTypes locking event: %w", err)
	}

	var types []*models.TicketType

	if existingTiers == 0 {
		// Seats currently held by flat bookings. seats_released_at IS NULL is the
		// same test the release/reacquire path uses for "holds inventory", so a
		// refunded or expired flat booking correctly contributes nothing.
		var heldByFlat int
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(ticket_quantity), 0)
			FROM bookings
			WHERE event_id = $1
			  AND ticket_type_id IS NULL
			  AND seats_released_at IS NULL
			  AND booking_status IN ('pending', 'confirmed')
		`, eventID).Scan(&heldByFlat); err != nil {
			return nil, fmt.Errorf("CreateTicketTypes counting flat sales: %w", err)
		}

		// The general tier inherits the event's current allocation. Floored at
		// what is already sold: chk_tt_not_oversold rejects tickets_sold above
		// capacity, and an event whose counter drifted above max_capacity would
		// otherwise make this insert fail outright. A NULL max_capacity means the
		// flat product was uncapped, and the tier stays uncapped too.
		var generalCapacity *int
		if maxCapacity != nil {
			c := *maxCapacity
			if heldByFlat > c {
				c = heldByFlat
			}
			generalCapacity = &c
		}

		general := &models.TicketType{}
		if err := tx.QueryRow(ctx, `
			INSERT INTO ticket_types
				(event_id, name, description, price, capacity,
				 sort_order, is_visible)
			VALUES ($1, $2, NULL, $3, $4, -1, TRUE)
			RETURNING id, event_id, name, description, price,
			          capacity, tickets_sold, reserved_count,
			          sort_order, is_visible, sale_starts_at, sale_ends_at,
			          created_at, updated_at
		`, eventID, GeneralTierName, eventPrice, generalCapacity).Scan(
			&general.ID, &general.EventID, &general.Name, &general.Description, &general.Price,
			&general.Capacity, &general.TicketsSold, &general.ReservedCount,
			&general.SortOrder, &general.IsVisible, &general.SaleStartsAt, &general.SaleEndsAt,
			&general.CreatedAt, &general.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("CreateTicketTypes materialising general tier: %w", err)
		}

		// Backfill history: every flat booking becomes a general-tier booking,
		// including cancelled and refunded ones, so the tier's sales figures and
		// the organiser's booking list agree about what was sold.
		if _, err := tx.Exec(ctx, `
			UPDATE bookings
			SET ticket_type_id   = $1,
			    ticket_type_name = $2,
			    updated_at       = NOW()
			WHERE event_id = $3 AND ticket_type_id IS NULL
		`, general.ID, GeneralTierName, eventID); err != nil {
			return nil, fmt.Errorf("CreateTicketTypes backfilling bookings: %w", err)
		}

		// Carry the seats across. Without this the tier reads 0 sold while the
		// event counter reads 10, and its full capacity goes back on sale.
		if heldByFlat > 0 {
			if err := tx.QueryRow(ctx, `
				UPDATE ticket_types
				SET tickets_sold = $1, updated_at = NOW()
				WHERE id = $2
				RETURNING tickets_sold
			`, heldByFlat, general.ID).Scan(&general.TicketsSold); err != nil {
				return nil, fmt.Errorf("CreateTicketTypes carrying flat sales: %w", err)
			}
		}

		general.ComputeAvailable()
		types = append(types, general)
	}

	for _, dto := range dtos {
		isVisible := true
		if dto.IsVisible != nil {
			isVisible = *dto.IsVisible
		}
		tt := &models.TicketType{}
		err := tx.QueryRow(ctx, `
			INSERT INTO ticket_types
				(event_id, name, description, price, capacity,
				 sort_order, is_visible, sale_starts_at, sale_ends_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, event_id, name, description, price,
			          capacity, tickets_sold, reserved_count,
			          sort_order, is_visible, sale_starts_at, sale_ends_at,
			          created_at, updated_at
		`, eventID, dto.Name, dto.Description, dto.Price, dto.Capacity,
			dto.SortOrder, isVisible, dto.SaleStartsAt, dto.SaleEndsAt,
		).Scan(
			&tt.ID, &tt.EventID, &tt.Name, &tt.Description, &tt.Price,
			&tt.Capacity, &tt.TicketsSold, &tt.ReservedCount,
			&tt.SortOrder, &tt.IsVisible, &tt.SaleStartsAt, &tt.SaleEndsAt,
			&tt.CreatedAt, &tt.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("CreateTicketTypes insert: %w", err)
		}
		tt.ComputeAvailable()
		types = append(types, tt)
	}

	if err := raiseEventCapacityToTierSum(tx, ctx, eventID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateTicketTypes commit: %w", err)
	}
	return types, nil
}

// ReplaceTicketTypes atomically deletes all existing ticket types for an event
// and inserts the new set in one transaction. Only safe to call when no active
// reservations exist for the event; the handler validates this first.
func ReplaceTicketTypes(
	pool *pgxpool.Pool,
	eventID string,
	dtos []models.CreateTicketTypeDTO,
	organizerID string,
) ([]*models.TicketType, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify ownership
	var count int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE id = $1 AND organizer_id = $2`,
		eventID, organizerID,
	).Scan(&count)
	if err != nil || count == 0 {
		return nil, ErrTicketTypeForbidden
	}

	// Block replace if any type has active bookings (tickets_sold > 0)
	var soldCount int
	pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(tickets_sold), 0) FROM ticket_types WHERE event_id = $1`,
		eventID,
	).Scan(&soldCount)
	if soldCount > 0 {
		return nil, ErrTicketTypeHasSales
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Delete all existing types (trigger will set events.price to 0 temporarily)
	if _, err := tx.Exec(ctx, `DELETE FROM ticket_types WHERE event_id = $1`, eventID); err != nil {
		return nil, fmt.Errorf("ReplaceTicketTypes delete: %w", err)
	}

	var types []*models.TicketType
	for _, dto := range dtos {
		isVisible := true
		if dto.IsVisible != nil {
			isVisible = *dto.IsVisible
		}
		tt := &models.TicketType{}
		err := tx.QueryRow(ctx, `
			INSERT INTO ticket_types
				(event_id, name, description, price, capacity,
				 sort_order, is_visible, sale_starts_at, sale_ends_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, event_id, name, description, price,
			          capacity, tickets_sold, reserved_count,
			          sort_order, is_visible, sale_starts_at, sale_ends_at,
			          created_at, updated_at
		`, eventID, dto.Name, dto.Description, dto.Price, dto.Capacity,
			dto.SortOrder, isVisible, dto.SaleStartsAt, dto.SaleEndsAt,
		).Scan(
			&tt.ID, &tt.EventID, &tt.Name, &tt.Description, &tt.Price,
			&tt.Capacity, &tt.TicketsSold, &tt.ReservedCount,
			&tt.SortOrder, &tt.IsVisible, &tt.SaleStartsAt, &tt.SaleEndsAt,
			&tt.CreatedAt, &tt.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("ReplaceTicketTypes insert: %w", err)
		}
		tt.ComputeAvailable()
		types = append(types, tt)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("ReplaceTicketTypes commit: %w", err)
	}
	return types, nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// UpdateTicketType applies a partial update to a single type.  Only non-nil
// fields in the DTO are written. Verifies organiser ownership via the event FK.
func UpdateTicketType(
	pool *pgxpool.Pool,
	typeID string,
	dto *models.UpdateTicketTypeDTO,
	organizerID string,
) (*models.TicketType, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ownership check: join through events
	var ownerID string
	err := pool.QueryRow(ctx, `
		SELECT e.organizer_id
		FROM ticket_types tt
		JOIN events e ON e.id = tt.event_id
		WHERE tt.id = $1
	`, typeID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTicketTypeNotFound
		}
		return nil, fmt.Errorf("UpdateTicketType ownership check: %w", err)
	}
	if ownerID != organizerID {
		return nil, ErrTicketTypeForbidden
	}

	// Build SET clause dynamically
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if dto.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *dto.Name)
		argIdx++
	}
	if dto.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *dto.Description)
		argIdx++
	}
	if dto.Price != nil {
		setClauses = append(setClauses, fmt.Sprintf("price = $%d", argIdx))
		args = append(args, *dto.Price)
		argIdx++
	}
	if dto.Capacity != nil {
		setClauses = append(setClauses, fmt.Sprintf("capacity = $%d", argIdx))
		args = append(args, *dto.Capacity)
		argIdx++
	}
	if dto.SortOrder != nil {
		setClauses = append(setClauses, fmt.Sprintf("sort_order = $%d", argIdx))
		args = append(args, *dto.SortOrder)
		argIdx++
	}
	if dto.IsVisible != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_visible = $%d", argIdx))
		args = append(args, *dto.IsVisible)
		argIdx++
	}
	if dto.SaleStartsAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("sale_starts_at = $%d", argIdx))
		args = append(args, *dto.SaleStartsAt)
		argIdx++
	}
	if dto.SaleEndsAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("sale_ends_at = $%d", argIdx))
		args = append(args, *dto.SaleEndsAt)
		argIdx++
	}

	if len(setClauses) == 0 {
		return GetTicketTypeByID(pool, typeID)
	}

	// updated_at is handled by trigger but we set it explicitly for immediate return
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, typeID)
	query := fmt.Sprintf(`
		UPDATE ticket_types
		SET %s
		WHERE id = $%d
		RETURNING id, event_id, name, description, price,
		          capacity, tickets_sold, reserved_count,
		          sort_order, is_visible, sale_starts_at, sale_ends_at,
		          created_at, updated_at
	`, strings.Join(setClauses, ", "), argIdx)

	// Transactional because a capacity change has to move the event ceiling with
	// it: raising a tier from 5 to 8 on a full event sells nothing until
	// max_capacity follows, and the two must not be separately observable.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("UpdateTicketType begin: %w", err)
	}
	defer tx.Rollback(ctx)

	tt := &models.TicketType{}
	err = tx.QueryRow(ctx, query, args...).Scan(
		&tt.ID, &tt.EventID, &tt.Name, &tt.Description, &tt.Price,
		&tt.Capacity, &tt.TicketsSold, &tt.ReservedCount,
		&tt.SortOrder, &tt.IsVisible, &tt.SaleStartsAt, &tt.SaleEndsAt,
		&tt.CreatedAt, &tt.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("UpdateTicketType update: %w", err)
	}

	// Only on a capacity edit. Every other field leaves the allocation alone,
	// and this never shrinks the ceiling, so lowering a tier is safe here too.
	if dto.Capacity != nil {
		if err := raiseEventCapacityToTierSum(tx, ctx, tt.EventID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("UpdateTicketType commit: %w", err)
	}

	tt.ComputeAvailable()
	return tt, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteTicketType removes a type. Blocks deletion if tickets have already been
// sold under this type to protect booking history integrity.
func DeleteTicketType(pool *pgxpool.Pool, typeID, organizerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ownership + sold check in one query
	var ownerID string
	var sold int
	err := pool.QueryRow(ctx, `
		SELECT e.organizer_id, tt.tickets_sold
		FROM ticket_types tt
		JOIN events e ON e.id = tt.event_id
		WHERE tt.id = $1
	`, typeID).Scan(&ownerID, &sold)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTicketTypeNotFound
		}
		return fmt.Errorf("DeleteTicketType: %w", err)
	}
	if ownerID != organizerID {
		return ErrTicketTypeForbidden
	}
	if sold > 0 {
		return ErrTicketTypeHasSales
	}

	_, err = pool.Exec(ctx, `DELETE FROM ticket_types WHERE id = $1`, typeID)
	return err
}