package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"events-db/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dropsMerchCols is the shared SELECT list for drops queries.
const dropsMerchCols = `id, event_id, name, description, base_price, category,
       display_order, is_active, images::text,
       sale_start, sale_end, created_at, updated_at`

// DropsResponse is the payload returned by GET /v1/drops.
// Two buckets — available now and dropping soon — so the frontend
// can render them in separate toggle views without filtering client-side.
type DropsResponse struct {
	Available []DropItem `json:"available"`
	Upcoming  []DropItem `json:"upcoming"`
}

// DropItem is a flattened merch item enriched with computed fields
// the frontend needs to render the card and detail modal.
type DropItem struct {
	models.Merchandise
	SaleStatus     string  `json:"sale_status"`      // "live" | "scheduled"
	SecondsUntil   *int64  `json:"seconds_until"`    // nil when live, countdown when scheduled
	SecondsRemain  *int64  `json:"seconds_remain"`   // nil when no sale_end, else seconds until expiry
	EffectivePrice float64 `json:"effective_price"`  // lowest active variant price, or base_price
}

// GetDropsFeedHandler returns all globally active and upcoming merch drops.
//
// Route: GET /v1/drops
//
// Response shape:
//
//	{
//	  "available": [ ...DropItem ],   -- live right now
//	  "upcoming":  [ ...DropItem ]    -- sale_start in the future
//	}
//
// Available items: is_active=true, sale window open (or no window set).
// Upcoming items:  is_active=true, sale_start is in the future.
// Items with no sale_start and no sale_end are always in "available".
func GetDropsFeedHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		available, err := queryDrops(ctx, pool, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load available drops"})
			return
		}

		upcoming, err := queryDrops(ctx, pool, true)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load upcoming drops"})
			return
		}

		c.JSON(http.StatusOK, DropsResponse{
			Available: available,
			Upcoming:  upcoming,
		})
	}
}

// queryDrops fetches either live or scheduled merch across all events.
// scheduled=false → within sale window (available now)
// scheduled=true  → sale_start is in the future (not yet live)
func queryDrops(ctx context.Context, pool *pgxpool.Pool, scheduled bool) ([]DropItem, error) {
	var where string
	if scheduled {
		// Upcoming: active, sale_start exists and is in the future
		where = `WHERE is_active = TRUE
		           AND sale_start IS NOT NULL
		           AND sale_start > NOW()`
	} else {
		// Available: active, within sale window (or no window boundaries set)
		where = `WHERE is_active = TRUE
		           AND (sale_start IS NULL OR sale_start <= NOW())
		           AND (sale_end   IS NULL OR sale_end   >  NOW())`
	}

	q := fmt.Sprintf(`
		SELECT %s
		FROM merchandise
		%s
		ORDER BY display_order ASC, created_at DESC
		LIMIT 50
	`, dropsMerchCols, where)

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("queryDrops scheduled=%v: %w", scheduled, err)
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
		return []DropItem{}, nil
	}

	// Batch-load all active variants (avoids N+1)
	varRows, err := pool.Query(ctx, `
		SELECT id, merchandise_id, sku, attributes::text,
		       stock_quantity, price_override, is_active, created_at, updated_at
		FROM merchandise_variants
		WHERE merchandise_id = ANY($1) AND is_active = TRUE
		ORDER BY created_at ASC
	`, ids)
	if err == nil {
		defer varRows.Close()
		variantMap := make(map[string][]models.MerchandiseVariant)
		for varRows.Next() {
			var v models.MerchandiseVariant
			if err := varRows.Scan(
				&v.ID, &v.MerchandiseID, &v.SKU, &v.Attributes,
				&v.StockQuantity, &v.PriceOverride, &v.IsActive,
				&v.CreatedAt, &v.UpdatedAt,
			); err == nil {
				variantMap[v.MerchandiseID] = append(variantMap[v.MerchandiseID], v)
			}
		}
		for i := range items {
			items[i].Variants = variantMap[items[i].ID]
		}
	}

	// Build DropItems with computed fields
	now := time.Now()
	result := make([]DropItem, 0, len(items))
	for _, m := range items {
		di := DropItem{
			Merchandise:    m,
			SaleStatus:     m.SaleStatus(),
			EffectivePrice: m.BasePrice,
		}

		// Countdown until drop goes live
		if m.SaleStart != nil && m.SaleStart.After(now) {
			secs := int64(m.SaleStart.Sub(now).Seconds())
			di.SecondsUntil = &secs
		}

		// Countdown until drop expires
		if m.SaleEnd != nil && m.SaleEnd.After(now) {
			secs := int64(m.SaleEnd.Sub(now).Seconds())
			di.SecondsRemain = &secs
		}

		// Lowest active variant price (or base_price if no variants)
		lowestPrice := m.BasePrice
		for _, v := range m.Variants {
			if v.PriceOverride != nil && *v.PriceOverride < lowestPrice {
				lowestPrice = *v.PriceOverride
			}
		}
		di.EffectivePrice = lowestPrice

		result = append(result, di)
	}

	return result, nil
}