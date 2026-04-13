package repository

import (
	"context"
	"errors"
	"events-db/internal/models"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrMerchNotFound = errors.New("merchandise not found")

// saleWindowClause is the SQL condition that enforces the sale window.
// Appended to every active-merch read query.
// Items with no sale_start / sale_end set pass through unchanged (existing behaviour).
const saleWindowClause = `
    AND (sale_start IS NULL OR sale_start <= NOW())
    AND (sale_end   IS NULL OR sale_end   >  NOW())`

// scanMerch scans a merchandise row including the two new sale-window columns.
// Column order must match every SELECT that calls this helper.
func scanMerch(row interface {
	Scan(...any) error
}, m *models.Merchandise) error {
	return row.Scan(
		&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
		&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
		&m.SaleStart, &m.SaleEnd,
		&m.CreatedAt, &m.UpdatedAt,
	)
}

// merchSelectCols is the canonical column list used in every merch SELECT.
// Keeping it in one place prevents drift when columns are added.
const merchSelectCols = `id, event_id, name, description, base_price, category,
       display_order, is_active, images::text,
       sale_start, sale_end,
       created_at, updated_at`

// GetMerchandiseByEvent returns merch with variants in 2 queries (not N+1).
// Only items that are active AND within their sale window are returned.
func GetMerchandiseByEvent(pool *pgxpool.Pool, eventID string, limit int) ([]models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q := fmt.Sprintf(`
        SELECT %s
        FROM merchandise
        WHERE event_id = $1 AND is_active = TRUE %s
        ORDER BY display_order ASC, created_at ASC`,
		merchSelectCols, saleWindowClause)
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := pool.Query(ctx, q, eventID)
	if err != nil {
		return nil, err
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
		return items, nil
	}

	varRows, err := pool.Query(ctx, `
        SELECT id, merchandise_id, sku, attributes::text,
               stock_quantity, price_override, is_active, created_at, updated_at
        FROM merchandise_variants
        WHERE merchandise_id = ANY($1) AND is_active = TRUE
        ORDER BY created_at ASC
    `, ids)
	if err != nil {
		return items, nil // variants failing shouldn't break merch display
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

	for i, item := range items {
		items[i].Variants = variantMap[item.ID]
	}

	return items, nil
}

// GetVariantsByMerchandise returns all active variants for a merchandise item.
func GetVariantsByMerchandise(pool *pgxpool.Pool, merchID string) ([]models.MerchandiseVariant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
        SELECT id, merchandise_id, sku, attributes::text,
               stock_quantity, price_override, is_active, created_at, updated_at
        FROM merchandise_variants
        WHERE merchandise_id = $1 AND is_active = TRUE
        ORDER BY created_at ASC
    `, merchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var variants []models.MerchandiseVariant
	for rows.Next() {
		var v models.MerchandiseVariant
		if err := rows.Scan(
			&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
			&v.StockQuantity, &v.PriceOverride, &v.IsActive,
			&v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, err
		}
		variants = append(variants, v)
	}
	return variants, nil
}

// GetVariantByID fetches a single variant with its parent merchandise.
// Also enforces the parent's sale window — you can't buy a variant whose
// item's sale period has not started yet or has already ended.
func GetVariantByID(pool *pgxpool.Pool, variantID string) (*models.MerchandiseVariant, float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var v models.MerchandiseVariant
	var basePrice float64
	err := pool.QueryRow(ctx, fmt.Sprintf(`
        SELECT mv.id, mv.merchandise_id, mv.sku, mv.attributes::text,
               mv.stock_quantity, mv.price_override, mv.is_active,
               mv.created_at, mv.updated_at, m.base_price
        FROM merchandise_variants mv
        JOIN merchandise m ON m.id = mv.merchandise_id
        WHERE mv.id = $1
          AND mv.is_active = TRUE
          AND m.is_active  = TRUE %s
    `, saleWindowClause), variantID).Scan(
		&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
		&v.StockQuantity, &v.PriceOverride, &v.IsActive,
		&v.CreatedAt, &v.UpdatedAt, &basePrice,
	)
	if err != nil {
		return nil, 0, ErrMerchNotFound
	}
	return &v, basePrice, nil
}

// CreateMerchandise inserts a new merch item for an event.
func CreateMerchandise(pool *pgxpool.Pool, eventID string, dto *models.CreateMerchandiseDTO) (*models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var m models.Merchandise
	err := pool.QueryRow(ctx, fmt.Sprintf(`
        INSERT INTO merchandise
          (event_id, name, description, base_price, category, display_order, images, sale_start, sale_end)
        VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9)
        RETURNING %s
    `, merchSelectCols),
		eventID, dto.Name, dto.Description, dto.BasePrice,
		dto.Category, dto.DisplayOrder, dto.Images,
		dto.SaleStart, dto.SaleEnd,
	).Scan(
		&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
		&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
		&m.SaleStart, &m.SaleEnd,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create merchandise: %w", err)
	}
	return &m, nil
}

// CreateVariant inserts a new variant for a merchandise item.
func CreateVariant(pool *pgxpool.Pool, merchID string, dto *models.CreateVariantDTO) (*models.MerchandiseVariant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var v models.MerchandiseVariant
	err := pool.QueryRow(ctx, `
        INSERT INTO merchandise_variants
          (merchandise_id, sku, attributes, stock_quantity, price_override)
        VALUES ($1, $2, $3::jsonb, $4, $5)
        RETURNING id, merchandise_id, sku, attributes::text,
                  stock_quantity, price_override, is_active, created_at, updated_at
    `, merchID, dto.SKU, dto.Attributes, dto.StockQuantity, dto.PriceOverride,
	).Scan(
		&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
		&v.StockQuantity, &v.PriceOverride, &v.IsActive,
		&v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create variant: %w", err)
	}
	return &v, nil
}

// DecrementVariantStock atomically decrements stock. Returns error if insufficient.
// Also enforces the parent's sale window — can't decrement stock for an expired item.
func DecrementVariantStock(pool *pgxpool.Pool, variantID string, qty int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, fmt.Sprintf(`
        UPDATE merchandise_variants mv
        SET stock_quantity = mv.stock_quantity - $1, updated_at = NOW()
        FROM merchandise m
        WHERE mv.id = $2
          AND mv.merchandise_id = m.id
          AND mv.stock_quantity >= $1
          AND m.is_active = TRUE %s
    `, saleWindowClause), qty, variantID)
	if err != nil {
		return fmt.Errorf("stock decrement failed: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("insufficient stock or item no longer on sale")
	}
	return nil
}

// GetMerchandiseByID returns a single merch item with all its variants.
// Returns ErrMerchNotFound if the item doesn't exist, is inactive, or outside
// its sale window.
func GetMerchandiseByID(pool *pgxpool.Pool, merchID string) (*models.MerchandiseDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var m models.Merchandise
	err := pool.QueryRow(ctx, fmt.Sprintf(`
        SELECT %s
        FROM merchandise
        WHERE id = $1 AND is_active = TRUE %s
    `, merchSelectCols, saleWindowClause), merchID).Scan(
		&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
		&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
		&m.SaleStart, &m.SaleEnd,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, ErrMerchNotFound
	}

	variants, err := GetVariantsByMerchandise(pool, merchID)
	if err != nil {
		return nil, fmt.Errorf("failed to load variants: %w", err)
	}
	m.Variants = variants

	detail := &models.MerchandiseDetail{Merchandise: m}
	detail.ComputeStockSummary()
	return detail, nil
}

// GetMerchandiseByEventFiltered returns merch for an event with optional
// category and attribute filters. Attribute filter is a JSON key-value pair
// e.g. filterKey="size", filterValue="L".
// Sale window is always enforced.
func GetMerchandiseByEventFiltered(
	pool *pgxpool.Pool,
	eventID string,
	limit int,
	category *string,
	filterKey *string,
	filterValue *string,
) ([]models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []any{eventID}
	where := fmt.Sprintf("WHERE event_id = $1 AND is_active = TRUE %s", saleWindowClause)

	if category != nil && *category != "" {
		args = append(args, *category)
		where += fmt.Sprintf(" AND category = $%d", len(args))
	}

	variantJoin := ""
	if filterKey != nil && filterValue != nil && *filterKey != "" {
		args = append(args, *filterKey, *filterValue)
		variantJoin = fmt.Sprintf(`
            AND id IN (
                SELECT merchandise_id FROM merchandise_variants
                WHERE is_active = TRUE
                AND attributes->>$%d = $%d
            )`, len(args)-1, len(args))
	}

	limitClause := ""
	if limit > 0 {
		args = append(args, limit)
		limitClause = fmt.Sprintf("LIMIT $%d", len(args))
	}

	q := fmt.Sprintf(`
        SELECT %s
        FROM merchandise
        %s %s
        ORDER BY display_order ASC, created_at ASC
        %s
    `, merchSelectCols, where, variantJoin, limitClause)

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.Merchandise
	var ids []string
	for rows.Next() {
		var m models.Merchandise
		if err := rows.Scan(
			&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
			&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
			&m.SaleStart, &m.SaleEnd,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, m)
		ids = append(ids, m.ID)
	}

	if len(items) == 0 {
		return items, nil
	}

	varRows, err := pool.Query(ctx, `
        SELECT id, merchandise_id, sku, attributes::text,
               stock_quantity, price_override, is_active, created_at, updated_at
        FROM merchandise_variants
        WHERE merchandise_id = ANY($1) AND is_active = TRUE
        ORDER BY created_at ASC
    `, ids)
	if err != nil {
		return items, nil
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

	for i, item := range items {
		items[i].Variants = variantMap[item.ID]
	}

	return items, nil
}

// UpdateMerchandise updates allowed fields on a merch item.
// Verifies the item belongs to an event owned by organizerID.
func UpdateMerchandise(pool *pgxpool.Pool, merchID string, dto *models.UpdateMerchandiseDTO, organizerID string) (*models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify ownership — organiser must own the event this merch belongs to.
	// No sale-window check here: organisers can update expired items to extend them.
	var eventID string
	err := pool.QueryRow(ctx, `
        SELECT m.event_id FROM merchandise m
        JOIN events e ON e.id = m.event_id
        WHERE m.id = $1 AND e.organizer_id = $2 AND m.is_active = TRUE
    `, merchID, organizerID).Scan(&eventID)
	if err != nil {
		return nil, ErrMerchNotFound
	}

	sets := []string{"updated_at = NOW()"}
	args := []any{}

	if dto.Name != nil {
		args = append(args, *dto.Name)
		sets = append(sets, fmt.Sprintf("name = $%d", len(args)))
	}
	if dto.Description != nil {
		args = append(args, *dto.Description)
		sets = append(sets, fmt.Sprintf("description = $%d", len(args)))
	}
	if dto.BasePrice != nil {
		args = append(args, *dto.BasePrice)
		sets = append(sets, fmt.Sprintf("base_price = $%d", len(args)))
	}
	if dto.Category != nil {
		args = append(args, *dto.Category)
		sets = append(sets, fmt.Sprintf("category = $%d", len(args)))
	}
	if dto.DisplayOrder != nil {
		args = append(args, *dto.DisplayOrder)
		sets = append(sets, fmt.Sprintf("display_order = $%d", len(args)))
	}
	if dto.IsActive != nil {
		args = append(args, *dto.IsActive)
		sets = append(sets, fmt.Sprintf("is_active = $%d", len(args)))
	}
	if dto.Images != nil {
		args = append(args, *dto.Images)
		sets = append(sets, fmt.Sprintf("images = $%d::jsonb", len(args)))
	}
	if dto.SaleStart != nil {
		args = append(args, *dto.SaleStart)
		sets = append(sets, fmt.Sprintf("sale_start = $%d", len(args)))
	}
	if dto.SaleEnd != nil {
		args = append(args, *dto.SaleEnd)
		sets = append(sets, fmt.Sprintf("sale_end = $%d", len(args)))
	}

	if len(args) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	args = append(args, merchID)
	query := fmt.Sprintf(`
        UPDATE merchandise SET %s
        WHERE id = $%d
        RETURNING %s
    `, joinSets(sets), len(args), merchSelectCols)

	var m models.Merchandise
	err = pool.QueryRow(ctx, query, args...).Scan(
		&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
		&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
		&m.SaleStart, &m.SaleEnd,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update merchandise: %w", err)
	}
	return &m, nil
}

// DeactivateMerchandise soft-deletes a merch item (sets is_active = false).
// Verifies organiser ownership before deactivating.
func DeactivateMerchandise(pool *pgxpool.Pool, merchID string, organizerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
        UPDATE merchandise m
        SET is_active = FALSE, updated_at = NOW()
        FROM events e
        WHERE m.event_id = e.id
        AND m.id = $1
        AND e.organizer_id = $2
        AND m.is_active = TRUE
    `, merchID, organizerID)
	if err != nil {
		return fmt.Errorf("failed to deactivate merchandise: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrMerchNotFound
	}
	return nil
}

// UpdateVariant updates stock quantity, price override, or active status.
// Verifies organiser ownership through the merchandise → event chain.
func UpdateVariant(pool *pgxpool.Pool, variantID string, dto *models.UpdateVariantDTO, organizerID string) (*models.MerchandiseVariant, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var eventID string
	err := pool.QueryRow(ctx, `
        SELECT e.id FROM merchandise_variants mv
        JOIN merchandise m ON m.id = mv.merchandise_id
        JOIN events e ON e.id = m.event_id
        WHERE mv.id = $1 AND e.organizer_id = $2
    `, variantID, organizerID).Scan(&eventID)
	if err != nil {
		return nil, "", ErrMerchNotFound
	}

	sets := []string{"updated_at = NOW()"}
	args := []any{}

	if dto.StockQuantity != nil {
		args = append(args, *dto.StockQuantity)
		sets = append(sets, fmt.Sprintf("stock_quantity = $%d", len(args)))
	}
	if dto.PriceOverride != nil {
		args = append(args, *dto.PriceOverride)
		sets = append(sets, fmt.Sprintf("price_override = $%d", len(args)))
	}
	if dto.IsActive != nil {
		args = append(args, *dto.IsActive)
		sets = append(sets, fmt.Sprintf("is_active = $%d", len(args)))
	}

	if len(args) == 0 {
		return nil, "", fmt.Errorf("no fields to update")
	}

	args = append(args, variantID)
	query := fmt.Sprintf(`
        UPDATE merchandise_variants SET %s
        WHERE id = $%d
        RETURNING id, merchandise_id, sku, attributes::text,
                  stock_quantity, price_override, is_active, created_at, updated_at
    `, joinSets(sets), len(args))

	var v models.MerchandiseVariant
	err = pool.QueryRow(ctx, query, args...).Scan(
		&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
		&v.StockQuantity, &v.PriceOverride, &v.IsActive,
		&v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to update variant: %w", err)
	}
	return &v, eventID, nil
}

// ExpireMerchListings deactivates merchandise items whose sale window has closed.
// Returns the IDs of affected events so callers can bust the merch cache.
// Called by the expiry worker on every tick.
func ExpireMerchListings(pool *pgxpool.Pool, ctx context.Context) ([]string, error) {
	rows, err := pool.Query(ctx, `
        UPDATE merchandise
        SET is_active = FALSE, updated_at = NOW()
        WHERE is_active = TRUE
          AND sale_end IS NOT NULL
          AND sale_end < NOW()
        RETURNING event_id
    `)
	if err != nil {
		return nil, fmt.Errorf("ExpireMerchListings: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	var eventIDs []string
	for rows.Next() {
		var eid string
		if err := rows.Scan(&eid); err != nil {
			continue
		}
		if _, ok := seen[eid]; !ok {
			seen[eid] = struct{}{}
			eventIDs = append(eventIDs, eid)
		}
	}
	return eventIDs, nil
}

// joinSets is a small helper to comma-join SET clauses.
func joinSets(sets []string) string {
	result := ""
	for i, s := range sets {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}