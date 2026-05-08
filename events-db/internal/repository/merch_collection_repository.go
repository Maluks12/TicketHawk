package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCollectionNotFound = errors.New("collection not found")

const collectionSelectCols = `id, organizer_id, name, description, display_order, is_active, created_at, updated_at`

func scanCollection(row interface {
	Scan(...any) error
}, c *models.MerchCollection) error {
	return row.Scan(
		&c.ID, &c.OrganizerID, &c.Name, &c.Description,
		&c.DisplayOrder, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
	)
}

// ── Organiser / Admin write ────────────────────────────────────────────────────

// CreateCollection inserts a new merch collection.
// organizerID is nil for admin-created collections.
func CreateCollection(pool *pgxpool.Pool, organizerID *string, dto *models.CreateCollectionDTO) (*models.MerchCollection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var c models.MerchCollection
	err := pool.QueryRow(ctx, fmt.Sprintf(`
        INSERT INTO merch_collections (organizer_id, name, description, display_order)
        VALUES ($1, $2, $3, $4)
        RETURNING %s
    `, collectionSelectCols),
		organizerID, dto.Name, dto.Description, dto.DisplayOrder,
	).Scan(
		&c.ID, &c.OrganizerID, &c.Name, &c.Description,
		&c.DisplayOrder, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}
	return &c, nil
}

// UpdateCollection updates a collection.
// If organizerID is non-nil, the row must be owned by that organizer.
// Pass nil for organizerID on admin calls to skip the ownership check.
func UpdateCollection(pool *pgxpool.Pool, collectionID string, organizerID *string, dto *models.UpdateCollectionDTO) (*models.MerchCollection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []any{}
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
	if dto.DisplayOrder != nil {
		setClauses = append(setClauses, fmt.Sprintf("display_order = $%d", argIdx))
		args = append(args, *dto.DisplayOrder)
		argIdx++
	}
	if dto.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *dto.IsActive)
		argIdx++
	}

	if len(setClauses) == 1 {
		return nil, errors.New("no fields to update")
	}

	args = append(args, collectionID)
	whereClause := fmt.Sprintf("id = $%d", argIdx)
	argIdx++

	if organizerID != nil {
		whereClause += fmt.Sprintf(" AND organizer_id = $%d", argIdx)
		args = append(args, *organizerID)
	}

	q := fmt.Sprintf(`
        UPDATE merch_collections
        SET %s
        WHERE %s
        RETURNING %s
    `, joinClauses(setClauses), whereClause, collectionSelectCols)

	var c models.MerchCollection
	err := pool.QueryRow(ctx, q, args...).Scan(
		&c.ID, &c.OrganizerID, &c.Name, &c.Description,
		&c.DisplayOrder, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, ErrCollectionNotFound
	}
	return &c, nil
}

// DeactivateCollection soft-deletes a collection (is_active = false) AND
// un-assigns all merchandise items that belonged to it (collection_id → NULL).
// ON DELETE SET NULL only fires on a hard DELETE, so we do the un-assign
// explicitly before marking the collection inactive.
// Pass nil organizerID for admin calls (no ownership check).
func DeactivateCollection(pool *pgxpool.Pool, collectionID string, organizerID *string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify the collection exists (and is owned by the organiser, if applicable)
	// before touching anything.
	checkQ := `SELECT EXISTS(SELECT 1 FROM merch_collections WHERE id = $1 AND is_active = TRUE`
	checkArgs := []any{collectionID}
	if organizerID != nil {
		checkQ += ` AND organizer_id = $2`
		checkArgs = append(checkArgs, *organizerID)
	}
	checkQ += `)`
	var exists bool
	if err := pool.QueryRow(ctx, checkQ, checkArgs...).Scan(&exists); err != nil || !exists {
		return ErrCollectionNotFound
	}

	// Un-assign all merch items first.
	if _, err := pool.Exec(ctx,
		`UPDATE merchandise SET collection_id = NULL, updated_at = NOW() WHERE collection_id = $1`,
		collectionID,
	); err != nil {
		return fmt.Errorf("un-assign merch from collection: %w", err)
	}

	// Soft-delete the collection.
	tag, err := pool.Exec(ctx,
		`UPDATE merch_collections SET is_active = FALSE, updated_at = NOW() WHERE id = $1`,
		collectionID,
	)
	if err != nil {
		return fmt.Errorf("deactivate collection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCollectionNotFound
	}
	return nil
}

// AssignMerchToCollection sets or clears the collection_id on a merchandise row.
// Pass nil collectionID to remove the item from its current collection.
// If organizerID is non-nil, ownership is verified via the merchandise → event → organizer chain.
// Pass nil organizerID for admin calls.
func AssignMerchToCollection(pool *pgxpool.Pool, merchID string, collectionID *string, organizerID *string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// If assigning (not clearing), verify the collection exists and is accessible.
	if collectionID != nil {
		var exists bool
		checkQ := `SELECT EXISTS(SELECT 1 FROM merch_collections WHERE id = $1 AND is_active = TRUE`
		checkArgs := []any{*collectionID}
		if organizerID != nil {
			checkQ += ` AND (organizer_id = $2 OR organizer_id IS NULL)`
			checkArgs = append(checkArgs, *organizerID)
		}
		checkQ += `)`
		if err := pool.QueryRow(ctx, checkQ, checkArgs...).Scan(&exists); err != nil || !exists {
			return ErrCollectionNotFound
		}
	}

	updateQ := `UPDATE merchandise SET collection_id = $1, updated_at = NOW() WHERE id = $2`
	updateArgs := []any{collectionID, merchID}

	if organizerID != nil {
		// Organiser ownership check — mirrors the pattern used by DeactivateMerchandise:
		// events.organizer_id stores the user_id directly, no separate organizers table.
		updateQ = `
            UPDATE merchandise m
            SET    collection_id = $1, updated_at = NOW()
            FROM   events e
            WHERE  m.id = $2
              AND  e.id = m.event_id
              AND  e.organizer_id = $3
        `
		updateArgs = append(updateArgs, *organizerID)
	}

	tag, err := pool.Exec(ctx, updateQ, updateArgs...)
	if err != nil {
		return fmt.Errorf("assign merch to collection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMerchNotFound
	}
	return nil
}

// ── Reads ──────────────────────────────────────────────────────────────────────

// GetCollectionByID returns a single collection with all its active merchandise items.
// Returns ErrCollectionNotFound if the collection does not exist or is inactive.
func GetCollectionByID(pool *pgxpool.Pool, collectionID string) (*models.MerchCollection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var c models.MerchCollection
	err := pool.QueryRow(ctx, fmt.Sprintf(`
        SELECT %s FROM merch_collections WHERE id = $1 AND is_active = TRUE
    `, collectionSelectCols), collectionID).Scan(
		&c.ID, &c.OrganizerID, &c.Name, &c.Description,
		&c.DisplayOrder, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, ErrCollectionNotFound
	}

	// Fetch items — active and within their sale window.
	items, err := getMerchByCollection(ctx, pool, collectionID)
	if err != nil {
		return nil, err
	}
	c.Items = items
	return &c, nil
}

// GetCollectionsByOrganizer returns all active collections owned by the organizer.
// Items are NOT pre-loaded — use GetCollectionByID for item detail.
func GetCollectionsByOrganizer(pool *pgxpool.Pool, organizerID string) ([]models.MerchCollection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, fmt.Sprintf(`
        SELECT %s FROM merch_collections
        WHERE organizer_id = $1 AND is_active = TRUE
        ORDER BY display_order ASC, created_at DESC
    `, collectionSelectCols), organizerID)
	if err != nil {
		return nil, fmt.Errorf("get collections by organizer: %w", err)
	}
	defer rows.Close()

	var result []models.MerchCollection
	for rows.Next() {
		var c models.MerchCollection
		if err := scanCollection(rows, &c); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	if result == nil {
		result = []models.MerchCollection{}
	}
	return result, nil
}

// GetAllCollections returns every collection regardless of status — for the admin dashboard.
// Items are NOT pre-loaded.
func GetAllCollections(pool *pgxpool.Pool) ([]models.MerchCollection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, fmt.Sprintf(`
        SELECT %s FROM merch_collections
        ORDER BY created_at DESC
    `, collectionSelectCols))
	if err != nil {
		return nil, fmt.Errorf("get all collections: %w", err)
	}
	defer rows.Close()

	var result []models.MerchCollection
	for rows.Next() {
		var c models.MerchCollection
		if err := scanCollection(rows, &c); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	if result == nil {
		result = []models.MerchCollection{}
	}
	return result, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// getMerchByCollection fetches active, in-window merch for a collection.
// Variants are loaded in a second query (not N+1).
func getMerchByCollection(ctx context.Context, pool *pgxpool.Pool, collectionID string) ([]models.Merchandise, error) {
	rows, err := pool.Query(ctx, fmt.Sprintf(`
        SELECT %s
        FROM   merchandise
        WHERE  collection_id = $1
          AND  is_active = TRUE
          %s
        ORDER BY display_order ASC, created_at ASC
    `, merchSelectCols, saleWindowClause), collectionID)
	if err != nil {
		return nil, fmt.Errorf("get merch by collection: %w", err)
	}
	defer rows.Close()

	var items []models.Merchandise
	var ids []string
	for rows.Next() {
		var m models.Merchandise
		if err := scanMerch(rows, &m); err != nil {
			return nil, err
		}
		items = append(items, m)
		ids = append(ids, m.ID)
	}
	if len(items) == 0 {
		return []models.Merchandise{}, nil
	}

	varRows, err := pool.Query(ctx, `
        SELECT id, merchandise_id, sku, attributes::text,
               stock_quantity, price_override, is_active, created_at, updated_at
        FROM   merchandise_variants
        WHERE  merchandise_id = ANY($1) AND is_active = TRUE
        ORDER BY created_at ASC
    `, ids)
	if err != nil {
		return items, nil // variant failure should not break collection display
	}
	defer varRows.Close()

	variantMap := make(map[string][]models.MerchandiseVariant)
	for varRows.Next() {
		var v models.MerchandiseVariant
		if err := varRows.Scan(
			&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
			&v.StockQuantity, &v.PriceOverride, &v.IsActive,
			&v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			continue
		}
		variantMap[v.MerchandiseID] = append(variantMap[v.MerchandiseID], v)
	}
	for i := range items {
		items[i].Variants = variantMap[items[i].ID]
	}
	return items, nil
}

// joinClauses joins SET clause fragments with ", ".
func joinClauses(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}