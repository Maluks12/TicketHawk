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
			return nil, fmt.Errorf("CreateTicketTypes insert: %w", err)
		}
		tt.ComputeAvailable()
		types = append(types, tt)
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

	tt := &models.TicketType{}
	err = pool.QueryRow(ctx, query, args...).Scan(
		&tt.ID, &tt.EventID, &tt.Name, &tt.Description, &tt.Price,
		&tt.Capacity, &tt.TicketsSold, &tt.ReservedCount,
		&tt.SortOrder, &tt.IsVisible, &tt.SaleStartsAt, &tt.SaleEndsAt,
		&tt.CreatedAt, &tt.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("UpdateTicketType update: %w", err)
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