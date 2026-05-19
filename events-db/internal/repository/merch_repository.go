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

const saleWindowClause = `
    AND (sale_start IS NULL OR sale_start <= NOW())
    AND (sale_end   IS NULL OR sale_end   >  NOW())`

const approvedMerchClause = `
    AND approval_status = 'approved'`

const merchSelectCols = `id, event_id, name, description, base_price, category,
       display_order, is_active, images::text,
       fulfillment_mode, pickup_location, delivery_fee,
       sale_start, sale_end,
       approval_status, rejection_reason,
       created_at, updated_at`

// scanMerch scans a merchandise row into m.
// Column order must match every SELECT that uses merchSelectCols.
func scanMerch(row interface {
	Scan(...any) error
}, m *models.Merchandise) error {
	return row.Scan(
		&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
		&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
		&m.FulfillmentMode, &m.PickupLocation, &m.DeliveryFee,
		&m.SaleStart, &m.SaleEnd,
		&m.ApprovalStatus, &m.RejectionReason,
		&m.CreatedAt, &m.UpdatedAt,
	)
}

func GetMerchandiseByEvent(pool *pgxpool.Pool, eventID string, limit int) ([]models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q := fmt.Sprintf(`
        SELECT %s
        FROM merchandise
        WHERE event_id = $1 AND is_active = TRUE %s %s
        ORDER BY display_order ASC, created_at ASC`,
		merchSelectCols, saleWindowClause, approvedMerchClause)
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
		return items, nil
	}
	defer varRows.Close()

	variantMap := make(map[string][]models.MerchandiseVariant)
	for varRows.Next() {
		var v models.MerchandiseVariant
		if err := varRows.Scan(&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
			&v.StockQuantity, &v.PriceOverride, &v.IsActive, &v.CreatedAt, &v.UpdatedAt,
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

func GetAllMerchandise(pool *pgxpool.Pool, limit int) ([]models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q := `
        SELECT m.id, m.event_id, m.name, m.description, m.base_price, m.category,
               m.display_order, m.is_active, m.images::text,
               m.fulfillment_mode, m.pickup_location, m.delivery_fee,
               m.sale_start, m.sale_end,
               m.approval_status, m.rejection_reason,
               m.created_at, m.updated_at,
               COALESCE(e.title, '') as event_title
        FROM merchandise m
        LEFT JOIN events e ON e.id = m.event_id
        ORDER BY m.created_at DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := pool.Query(ctx, q)
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
			&m.FulfillmentMode, &m.PickupLocation, &m.DeliveryFee,
			&m.SaleStart, &m.SaleEnd,
			&m.ApprovalStatus, &m.RejectionReason,
			&m.CreatedAt, &m.UpdatedAt,
			&m.EventTitle,
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
        WHERE merchandise_id = ANY($1)
        ORDER BY created_at ASC
    `, ids)
	if err != nil {
		return items, nil
	}
	defer varRows.Close()

	variantMap := make(map[string][]models.MerchandiseVariant)
	for varRows.Next() {
		var v models.MerchandiseVariant
		if err := varRows.Scan(&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
			&v.StockQuantity, &v.PriceOverride, &v.IsActive, &v.CreatedAt, &v.UpdatedAt,
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
		if err := rows.Scan(&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
			&v.StockQuantity, &v.PriceOverride, &v.IsActive, &v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, err
		}
		variants = append(variants, v)
	}
	return variants, nil
}

func GetVariantByID(pool *pgxpool.Pool, variantID string) (*models.MerchandiseVariant, float64, *models.MerchFulfillmentInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var v models.MerchandiseVariant
	var basePrice float64
	var fi models.MerchFulfillmentInfo

	err := pool.QueryRow(ctx, fmt.Sprintf(`
        SELECT mv.id, mv.merchandise_id, mv.sku, mv.attributes::text,
               mv.stock_quantity, mv.price_override, mv.is_active,
               mv.created_at, mv.updated_at,
               m.base_price,
               m.fulfillment_mode, m.pickup_location, m.delivery_fee
        FROM merchandise_variants mv
        JOIN merchandise m ON m.id = mv.merchandise_id
        WHERE mv.id = $1
          AND mv.is_active = TRUE
          AND m.is_active  = TRUE %s
    `, saleWindowClause), variantID).Scan(
		&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
		&v.StockQuantity, &v.PriceOverride, &v.IsActive,
		&v.CreatedAt, &v.UpdatedAt, &basePrice,
		&fi.FulfillmentMode, &fi.PickupLocation, &fi.DeliveryFee,
	)
	if err != nil {
		return nil, 0, nil, ErrMerchNotFound
	}
	return &v, basePrice, &fi, nil
}

func CreateMerchandise(pool *pgxpool.Pool, eventID *string, dto *models.CreateMerchandiseDTO) (*models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mode := dto.FulfillmentMode
	if mode == "" {
		mode = string(models.FulfillmentModeDelivery)
	}

	var m models.Merchandise
	err := pool.QueryRow(ctx, fmt.Sprintf(`
        INSERT INTO merchandise
          (event_id, name, description, base_price, category, display_order, images,
           fulfillment_mode, pickup_location, delivery_fee,
           sale_start, sale_end, approval_status)
        VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, 'pending_review')
        RETURNING %s
    `, merchSelectCols),
		eventID, dto.Name, dto.Description, dto.BasePrice,
		dto.Category, dto.DisplayOrder, dto.Images,
		mode, dto.PickupLocation, dto.DeliveryFee,
		dto.SaleStart, dto.SaleEnd,
	).Scan(
		&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
		&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
		&m.FulfillmentMode, &m.PickupLocation, &m.DeliveryFee,
		&m.SaleStart, &m.SaleEnd,
		&m.ApprovalStatus, &m.RejectionReason,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create merchandise: %w", err)
	}
	return &m, nil
}

// AdminCreateMerchandise inserts a new merch item pre-approved.
// Admin-created items bypass the organiser review queue and are
// visible to buyers immediately (subject to is_active and sale window).
func AdminCreateMerchandise(pool *pgxpool.Pool, eventID *string, dto *models.CreateMerchandiseDTO) (*models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mode := dto.FulfillmentMode
	if mode == "" {
		mode = string(models.FulfillmentModeDelivery)
	}

	var m models.Merchandise
	err := pool.QueryRow(ctx, fmt.Sprintf(`
        INSERT INTO merchandise
          (event_id, name, description, base_price, category, display_order, images,
           fulfillment_mode, pickup_location, delivery_fee,
           sale_start, sale_end, approval_status)
        VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, 'approved')
        RETURNING %s
    `, merchSelectCols),
		eventID, dto.Name, dto.Description, dto.BasePrice,
		dto.Category, dto.DisplayOrder, dto.Images,
		mode, dto.PickupLocation, dto.DeliveryFee,
		dto.SaleStart, dto.SaleEnd,
	).Scan(
		&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
		&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
		&m.FulfillmentMode, &m.PickupLocation, &m.DeliveryFee,
		&m.SaleStart, &m.SaleEnd,
		&m.ApprovalStatus, &m.RejectionReason,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create merchandise: %w", err)
	}
	return &m, nil
}

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
          AND m.approval_status = 'approved'
    `, saleWindowClause), qty, variantID)
	if err != nil {
		return fmt.Errorf("stock decrement failed: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("insufficient stock or item no longer on sale")
	}
	return nil
}

// GetMerchandiseByID — FIX: Scan now includes ApprovalStatus + RejectionReason
// to match the 18-column merchSelectCols (previously only scanned 16).
func GetMerchandiseByID(pool *pgxpool.Pool, merchID string) (*models.MerchandiseDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var m models.Merchandise
	err := pool.QueryRow(ctx, fmt.Sprintf(`
        SELECT %s
        FROM merchandise
        WHERE id = $1 AND is_active = TRUE %s
          AND approval_status = 'approved'
    `, merchSelectCols, saleWindowClause), merchID).Scan(
		&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
		&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
		&m.FulfillmentMode, &m.PickupLocation, &m.DeliveryFee,
		&m.SaleStart, &m.SaleEnd,
		&m.ApprovalStatus, &m.RejectionReason,
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

func GetMerchandiseByEventFiltered(pool *pgxpool.Pool, eventID string, limit int, category *string, filterKey *string, filterValue *string) ([]models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []any{eventID}
	where := fmt.Sprintf("WHERE event_id = $1 AND is_active = TRUE %s %s", saleWindowClause, approvedMerchClause)

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
        SELECT %s FROM merchandise %s %s
        ORDER BY display_order ASC, created_at ASC %s
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
		return items, nil
	}
	defer varRows.Close()

	variantMap := make(map[string][]models.MerchandiseVariant)
	for varRows.Next() {
		var v models.MerchandiseVariant
		if err := varRows.Scan(&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
			&v.StockQuantity, &v.PriceOverride, &v.IsActive, &v.CreatedAt, &v.UpdatedAt,
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

// UpdateMerchandise — FIX: Scan now includes ApprovalStatus + RejectionReason
// to match the 18-column mechSelectCols (previously only scanned 16).
func UpdateMerchandise(pool *pgxpool.Pool, merchID string, dto *models.UpdateMerchandiseDTO, organizerID string) (*models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var eventID *string
	err := pool.QueryRow(ctx, `SELECT event_id FROM merchandise WHERE id = $1 AND is_active = TRUE`, merchID).Scan(&eventID)
	if err != nil {
		return nil, ErrMerchNotFound
	}

	if eventID != nil {
		var verifyID string
		err = pool.QueryRow(ctx, `
            SELECT m.id FROM merchandise m
            JOIN events e ON e.id = m.event_id
            WHERE m.id = $1 AND e.organizer_id = $2 AND m.is_active = TRUE
        `, merchID, organizerID).Scan(&verifyID)
		if err != nil {
			return nil, ErrMerchNotFound
		}
	}

	sets := []string{"updated_at = NOW()", "approval_status = 'pending_review'", "rejection_reason = NULL"}
	args := []any{}

	if dto.Name != nil { args = append(args, *dto.Name); sets = append(sets, fmt.Sprintf("name = $%d", len(args))) }
	if dto.Description != nil { args = append(args, *dto.Description); sets = append(sets, fmt.Sprintf("description = $%d", len(args))) }
	if dto.BasePrice != nil { args = append(args, *dto.BasePrice); sets = append(sets, fmt.Sprintf("base_price = $%d", len(args))) }
	if dto.Category != nil { args = append(args, *dto.Category); sets = append(sets, fmt.Sprintf("category = $%d", len(args))) }
	if dto.DisplayOrder != nil { args = append(args, *dto.DisplayOrder); sets = append(sets, fmt.Sprintf("display_order = $%d", len(args))) }
	if dto.IsActive != nil { args = append(args, *dto.IsActive); sets = append(sets, fmt.Sprintf("is_active = $%d", len(args))) }
	if dto.Images != nil { args = append(args, *dto.Images); sets = append(sets, fmt.Sprintf("images = $%d::jsonb", len(args))) }
	if dto.FulfillmentMode != nil { args = append(args, *dto.FulfillmentMode); sets = append(sets, fmt.Sprintf("fulfillment_mode = $%d", len(args))) }
	if dto.PickupLocation != nil { args = append(args, *dto.PickupLocation); sets = append(sets, fmt.Sprintf("pickup_location = $%d", len(args))) }
	if dto.DeliveryFee != nil { args = append(args, *dto.DeliveryFee); sets = append(sets, fmt.Sprintf("delivery_fee = $%d", len(args))) }
	if dto.SaleStart != nil { args = append(args, *dto.SaleStart); sets = append(sets, fmt.Sprintf("sale_start = $%d", len(args))) }
	if dto.SaleEnd != nil { args = append(args, *dto.SaleEnd); sets = append(sets, fmt.Sprintf("sale_end = $%d", len(args))) }

	if len(args) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	args = append(args, merchID)
	var m models.Merchandise
	err = pool.QueryRow(ctx, fmt.Sprintf(`
        UPDATE merchandise SET %s WHERE id = $%d RETURNING %s
    `, joinSets(sets), len(args), merchSelectCols), args...).Scan(
		&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
		&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
		&m.FulfillmentMode, &m.PickupLocation, &m.DeliveryFee,
		&m.SaleStart, &m.SaleEnd,
		&m.ApprovalStatus, &m.RejectionReason,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update merchandise: %w", err)
	}
	return &m, nil
}

func DeactivateMerchandise(pool *pgxpool.Pool, merchID string, organizerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var eventID *string
	err := pool.QueryRow(ctx, `SELECT event_id FROM merchandise WHERE id = $1 AND is_active = TRUE`, merchID).Scan(&eventID)
	if err != nil {
		return ErrMerchNotFound
	}

	if eventID != nil {
		cmd, err := pool.Exec(ctx, `
            UPDATE merchandise m SET is_active = FALSE, updated_at = NOW()
            FROM events e WHERE m.event_id = e.id AND m.id = $1
            AND e.organizer_id = $2 AND m.is_active = TRUE
        `, merchID, organizerID)
		if err != nil { return fmt.Errorf("failed to deactivate merchandise: %w", err) }
		if cmd.RowsAffected() == 0 { return ErrMerchNotFound }
	} else {
		cmd, err := pool.Exec(ctx, `UPDATE merchandise SET is_active = FALSE, updated_at = NOW() WHERE id = $1 AND is_active = TRUE`, merchID)
		if err != nil { return fmt.Errorf("failed to deactivate merchandise: %w", err) }
		if cmd.RowsAffected() == 0 { return ErrMerchNotFound }
	}
	return nil
}

func UpdateVariant(pool *pgxpool.Pool, variantID string, dto *models.UpdateVariantDTO, organizerID string) (*models.MerchandiseVariant, *string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var eventID *string
	err := pool.QueryRow(ctx, `
        SELECT m.event_id FROM merchandise_variants mv
        JOIN merchandise m ON m.id = mv.merchandise_id WHERE mv.id = $1
    `, variantID).Scan(&eventID)
	if err != nil {
		return nil, nil, ErrMerchNotFound
	}

	if eventID != nil {
		var verifyID string
		err = pool.QueryRow(ctx, `
            SELECT e.id FROM merchandise_variants mv
            JOIN merchandise m ON m.id = mv.merchandise_id
            JOIN events e ON e.id = m.event_id
            WHERE mv.id = $1 AND e.organizer_id = $2
        `, variantID, organizerID).Scan(&verifyID)
		if err != nil { return nil, nil, ErrMerchNotFound }
	}

	sets := []string{"updated_at = NOW()"}
	args := []any{}
	if dto.StockQuantity != nil { args = append(args, *dto.StockQuantity); sets = append(sets, fmt.Sprintf("stock_quantity = $%d", len(args))) }
	if dto.PriceOverride != nil { args = append(args, *dto.PriceOverride); sets = append(sets, fmt.Sprintf("price_override = $%d", len(args))) }
	if dto.IsActive != nil { args = append(args, *dto.IsActive); sets = append(sets, fmt.Sprintf("is_active = $%d", len(args))) }
	if len(args) == 0 { return nil, nil, fmt.Errorf("no fields to update") }

	args = append(args, variantID)
	var v models.MerchandiseVariant
	err = pool.QueryRow(ctx, fmt.Sprintf(`
        UPDATE merchandise_variants SET %s WHERE id = $%d
        RETURNING id, merchandise_id, sku, attributes::text,
                  stock_quantity, price_override, is_active, created_at, updated_at
    `, joinSets(sets), len(args)), args...).Scan(
		&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
		&v.StockQuantity, &v.PriceOverride, &v.IsActive, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil { return nil, nil, fmt.Errorf("failed to update variant: %w", err) }
	return &v, eventID, nil
}

func ExpireMerchListings(pool *pgxpool.Pool, ctx context.Context) ([]string, error) {
	rows, err := pool.Query(ctx, `
        UPDATE merchandise SET is_active = FALSE, updated_at = NOW()
        WHERE is_active = TRUE AND sale_end IS NOT NULL AND sale_end < NOW()
        RETURNING event_id
    `)
	if err != nil {
		return nil, fmt.Errorf("ExpireMerchListings: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	var eventIDs []string
	for rows.Next() {
		var eid *string
		if err := rows.Scan(&eid); err != nil { continue }
		if eid == nil { continue }
		if _, ok := seen[*eid]; !ok {
			seen[*eid] = struct{}{}
			eventIDs = append(eventIDs, *eid)
		}
	}
	return eventIDs, nil
}

func joinSets(sets []string) string {
	result := ""
	for i, s := range sets {
		if i > 0 { result += ", " }
		result += s
	}
	return result
}

// AdminUpdateMerchandise — uses UPDATE then SELECT (not RETURNING) so it stays
// correct even if merchSelectCols grows in future. No ownership check.
func AdminUpdateMerchandise(pool *pgxpool.Pool, merchID string, dto *models.UpdateMerchandiseDTO) (*models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Admin is the approval authority — any admin save auto-approves the item
	// so it becomes visible in buyer-facing drops/collections immediately.
	sets := []string{"updated_at = NOW()", "approval_status = 'approved'", "rejection_reason = NULL"}
	args := []any{}

	if dto.Name != nil { args = append(args, *dto.Name); sets = append(sets, fmt.Sprintf("name = $%d", len(args))) }
	if dto.Description != nil { args = append(args, *dto.Description); sets = append(sets, fmt.Sprintf("description = $%d", len(args))) }
	if dto.BasePrice != nil { args = append(args, *dto.BasePrice); sets = append(sets, fmt.Sprintf("base_price = $%d", len(args))) }
	if dto.Category != nil { args = append(args, *dto.Category); sets = append(sets, fmt.Sprintf("category = $%d", len(args))) }
	if dto.DisplayOrder != nil { args = append(args, *dto.DisplayOrder); sets = append(sets, fmt.Sprintf("display_order = $%d", len(args))) }
	if dto.IsActive != nil {
		args = append(args, *dto.IsActive)
		sets = append(sets, fmt.Sprintf("is_active = $%d", len(args)))
	}
	if dto.Images != nil { args = append(args, *dto.Images); sets = append(sets, fmt.Sprintf("images = $%d::jsonb", len(args))) }
	if dto.FulfillmentMode != nil { args = append(args, *dto.FulfillmentMode); sets = append(sets, fmt.Sprintf("fulfillment_mode = $%d", len(args))) }
	if dto.PickupLocation != nil { args = append(args, *dto.PickupLocation); sets = append(sets, fmt.Sprintf("pickup_location = $%d", len(args))) }
	if dto.DeliveryFee != nil { args = append(args, *dto.DeliveryFee); sets = append(sets, fmt.Sprintf("delivery_fee = $%d", len(args))) }
	if dto.SaleStart != nil { args = append(args, *dto.SaleStart); sets = append(sets, fmt.Sprintf("sale_start = $%d", len(args))) }
	if dto.SaleEnd != nil { args = append(args, *dto.SaleEnd); sets = append(sets, fmt.Sprintf("sale_end = $%d", len(args))) }

	if len(args) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	args = append(args, merchID)
	res, err := pool.Exec(ctx,
		fmt.Sprintf("UPDATE merchandise SET %s WHERE id = $%d", joinSets(sets), len(args)),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update merchandise: %w", err)
	}
	if res.RowsAffected() == 0 {
		return nil, ErrMerchNotFound
	}

	var m models.Merchandise
	err = pool.QueryRow(ctx, `
		SELECT id, event_id, name, description, base_price, category,
		       display_order, is_active, images::text,
		       fulfillment_mode, pickup_location, delivery_fee,
		       sale_start, sale_end,
		       approval_status, rejection_reason,
		       created_at, updated_at
		FROM merchandise WHERE id = $1
	`, merchID).Scan(
		&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
		&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
		&m.FulfillmentMode, &m.PickupLocation, &m.DeliveryFee,
		&m.SaleStart, &m.SaleEnd,
		&m.ApprovalStatus, &m.RejectionReason,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated merchandise: %w", err)
	}
	return &m, nil
}

// ── Admin merch approval ──────────────────────────────────────────────────────

func AdminGetPendingMerch(pool *pgxpool.Pool) ([]models.Merchandise, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT m.id, m.event_id, m.name, m.description, m.base_price, m.category,
		       m.display_order, m.is_active, m.images::text,
		       m.fulfillment_mode, m.pickup_location, m.delivery_fee,
		       m.sale_start, m.sale_end,
		       m.approval_status, m.rejection_reason,
		       m.created_at, m.updated_at,
		       COALESCE(e.title, '') AS event_title
		FROM merchandise m
		LEFT JOIN events e ON e.id = m.event_id
		WHERE m.approval_status = 'pending_review'
		ORDER BY m.created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("AdminGetPendingMerch: %w", err)
	}
	defer rows.Close()

	var items []models.Merchandise
	for rows.Next() {
		var m models.Merchandise
		if err := rows.Scan(
			&m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice, &m.Category,
			&m.DisplayOrder, &m.IsActive, &m.Images,
			&m.FulfillmentMode, &m.PickupLocation, &m.DeliveryFee,
			&m.SaleStart, &m.SaleEnd,
			&m.ApprovalStatus, &m.RejectionReason,
			&m.CreatedAt, &m.UpdatedAt,
			&m.EventTitle,
		); err != nil {
			return nil, fmt.Errorf("AdminGetPendingMerch scan: %w", err)
		}
		items = append(items, m)
	}
	if items == nil { items = []models.Merchandise{} }
	return items, rows.Err()
}

func AdminApproveMerch(pool *pgxpool.Pool, merchID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd, err := pool.Exec(ctx, `
		UPDATE merchandise SET approval_status = 'approved', rejection_reason = NULL
		WHERE id = $1 AND approval_status = 'pending_review'
	`, merchID)
	if err != nil { return fmt.Errorf("AdminApproveMerch: %w", err) }
	if cmd.RowsAffected() == 0 { return ErrMerchNotFound }
	return nil
}

func AdminRejectMerch(pool *pgxpool.Pool, merchID string, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd, err := pool.Exec(ctx, `
		UPDATE merchandise SET approval_status = 'rejected', rejection_reason = $1
		WHERE id = $2 AND approval_status = 'pending_review'
	`, reason, merchID)
	if err != nil { return fmt.Errorf("AdminRejectMerch: %w", err) }
	if cmd.RowsAffected() == 0 { return ErrMerchNotFound }
	return nil
}

func OrgResubmitMerch(pool *pgxpool.Pool, merchID string, organizerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd, err := pool.Exec(ctx, `
		UPDATE merchandise m
		SET    approval_status = 'pending_review', rejection_reason = NULL, updated_at = NOW()
		FROM   events e
		WHERE  m.id = $1 AND m.approval_status = 'rejected'
		  AND  e.id = m.event_id AND e.organizer_id = $2
	`, merchID, organizerID)
	if err != nil { return fmt.Errorf("OrgResubmitMerch: %w", err) }
	if cmd.RowsAffected() == 0 { return ErrMerchNotFound }
	return nil
}

func AdminUpdateVariant(pool *pgxpool.Pool, variantID string, dto *models.UpdateVariantDTO) (*models.MerchandiseVariant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sets := []string{"updated_at = NOW()"}
	args := []any{}
	if dto.StockQuantity != nil { args = append(args, *dto.StockQuantity); sets = append(sets, fmt.Sprintf("stock_quantity = $%d", len(args))) }
	if dto.PriceOverride != nil { args = append(args, *dto.PriceOverride); sets = append(sets, fmt.Sprintf("price_override = $%d", len(args))) }
	if dto.IsActive != nil { args = append(args, *dto.IsActive); sets = append(sets, fmt.Sprintf("is_active = $%d", len(args))) }
	if len(args) == 0 { return nil, fmt.Errorf("no fields to update") }

	args = append(args, variantID)
	var v models.MerchandiseVariant
	err := pool.QueryRow(ctx, fmt.Sprintf(`
        UPDATE merchandise_variants SET %s WHERE id = $%d
        RETURNING id, merchandise_id, sku, attributes::text,
                  stock_quantity, price_override, is_active, created_at, updated_at
    `, joinSets(sets), len(args)), args...).Scan(
		&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
		&v.StockQuantity, &v.PriceOverride, &v.IsActive, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil { return nil, ErrMerchNotFound }
	return &v, nil
}