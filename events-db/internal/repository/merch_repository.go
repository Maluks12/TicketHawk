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
 
// GetMerchandiseByEvent returns all active merch for an event with variants.
func GetMerchandiseByEvent(pool *pgxpool.Pool, eventID string, limit int) ([]models.Merchandise, error) {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  q := `SELECT id, event_id, name, description, base_price, category,
              display_order, is_active, images::text, created_at, updated_at
         FROM merchandise
         WHERE event_id = $1 AND is_active = TRUE
         ORDER BY display_order ASC, created_at ASC`
  if limit > 0 {
    q += fmt.Sprintf(" LIMIT %d", limit)
  }
 
  rows, err := pool.Query(ctx, q, eventID)
  if err != nil {
    return nil, err
  }
  defer rows.Close()
 
  var items []models.Merchandise
  for rows.Next() {
    var m models.Merchandise
    if err := rows.Scan(
      &m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
      &m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
      &m.CreatedAt, &m.UpdatedAt,
    ); err != nil {
      return nil, err
    }
    items = append(items, m)
  }
 
  // Attach variants to each item
  for i, item := range items {
    variants, err := GetVariantsByMerchandise(pool, item.ID)
    if err == nil {
      items[i].Variants = variants
    }
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
func GetVariantByID(pool *pgxpool.Pool, variantID string) (*models.MerchandiseVariant, float64, error) {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  var v models.MerchandiseVariant
  var basePrice float64
  err := pool.QueryRow(ctx, `
    SELECT mv.id, mv.merchandise_id, mv.sku, mv.attributes::text,
           mv.stock_quantity, mv.price_override, mv.is_active,
           mv.created_at, mv.updated_at, m.base_price
    FROM merchandise_variants mv
    JOIN merchandise m ON m.id = mv.merchandise_id
    WHERE mv.id = $1 AND mv.is_active = TRUE AND m.is_active = TRUE
  `, variantID).Scan(
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
  err := pool.QueryRow(ctx, `
    INSERT INTO merchandise
      (event_id, name, description, base_price, category, display_order, images)
    VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
    RETURNING id, event_id, name, description, base_price, category,
              display_order, is_active, images::text, created_at, updated_at
  `, eventID, dto.Name, dto.Description, dto.BasePrice,
     dto.Category, dto.DisplayOrder, dto.Images,
  ).Scan(
    &m.ID, &m.EventID, &m.Name, &m.Description, &m.BasePrice,
    &m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
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
func DecrementVariantStock(pool *pgxpool.Pool, variantID string, qty int) error {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
 
  cmd, err := pool.Exec(ctx, `
    UPDATE merchandise_variants
    SET stock_quantity = stock_quantity - $1, updated_at = NOW()
    WHERE id = $2 AND stock_quantity >= $1
  `, qty, variantID)
  if err != nil {
    return fmt.Errorf("stock decrement failed: %w", err)
  }
  if cmd.RowsAffected() == 0 {
    return errors.New("insufficient stock")
  }
  return nil
}
