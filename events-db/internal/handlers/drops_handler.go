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
// We JOIN events to pull event_name so the frontend can group items
// from the same event into a "collection" carousel without a separate API call.
const dropsMerchCols = `m.id, m.event_id, COALESCE(e.title, '') AS event_name,
       m.name, m.description, m.base_price, m.category,
       m.display_order, m.is_active, m.images::text,
       m.sale_start, m.sale_end, m.created_at, m.updated_at`

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
	EventName      string  `json:"event_name"`       // parent event title — used for collection grouping
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
		where = `WHERE m.is_active = TRUE
		           AND m.sale_start IS NOT NULL
		           AND m.sale_start > NOW()`
	} else {
		// Available: active, within sale window (or no window boundaries set)
		where = `WHERE m.is_active = TRUE
		           AND (m.sale_start IS NULL OR m.sale_start <= NOW())
		           AND (m.sale_end   IS NULL OR m.sale_end   >  NOW())`
	}

	q := fmt.Sprintf(`
		SELECT %s
		FROM merchandise m
		LEFT JOIN events e ON e.id = m.event_id
		%s
		ORDER BY m.display_order ASC, m.created_at DESC
		LIMIT 50
	`, dropsMerchCols, where)

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("queryDrops scheduled=%v: %w", scheduled, err)
	}
	defer rows.Close()

	type rowData struct {
		merch     models.Merchandise
		eventName string
	}
	var rowsData []rowData
	var ids []string
	for rows.Next() {
		var m models.Merchandise
		var eventName string
		if err := rows.Scan(
			&m.ID, &m.EventID, &eventName, &m.Name, &m.Description, &m.BasePrice,
			&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
			&m.SaleStart, &m.SaleEnd,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rowsData = append(rowsData, rowData{merch: m, eventName: eventName})
		ids = append(ids, m.ID)
	}

	if len(rowsData) == 0 {
		return []DropItem{}, nil
	}

	// Batch-load all active variants (avoids N+1)
	variantMap := make(map[string][]models.MerchandiseVariant)
	varRows, err := pool.Query(ctx, `
		SELECT id, merchandise_id, sku, attributes::text,
		       stock_quantity, price_override, is_active, created_at, updated_at
		FROM merchandise_variants
		WHERE merchandise_id = ANY($1) AND is_active = TRUE
		ORDER BY created_at ASC
	`, ids)
	if err == nil {
		defer varRows.Close()
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
	}

	// Build DropItems with computed fields
	now := time.Now()
	result := make([]DropItem, 0, len(rowsData))
	for _, rd := range rowsData {
		m := rd.merch
		m.Variants = variantMap[m.ID]

		di := DropItem{
			Merchandise:    m,
			EventName:      rd.eventName,
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