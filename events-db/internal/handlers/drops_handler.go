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

const dropsMerchCols = `m.id, m.event_id, COALESCE(e.title, '') AS event_name,
       m.name, m.description, m.base_price, m.category,
       m.display_order, m.is_active, m.images::text,
       m.sale_start, m.sale_end, m.created_at, m.updated_at,
       m.collection_id, COALESCE(mc.name, '') AS collection_name,
       mc.cover_image_url AS collection_cover_image_url,
       mc.description AS collection_description,
       m.fulfillment_mode, m.pickup_location, m.delivery_fee`

type DropsResponse struct {
	Available []DropItem `json:"available"`
	Upcoming  []DropItem `json:"upcoming"`
}

type DropItem struct {
	models.Merchandise
	EventName             string  `json:"event_name"`
	CollectionID          *string `json:"collection_id"`
	CollectionName        string  `json:"collection_name"`
	CollectionCoverImage  *string `json:"collection_cover_image_url,omitempty"`
	CollectionDescription *string `json:"collection_description,omitempty"`
	SaleStatus            string  `json:"sale_status"`
	SecondsUntil          *int64  `json:"seconds_until"`
	SecondsRemain         *int64  `json:"seconds_remain"`
	EffectivePrice        float64 `json:"effective_price"`
}

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
		c.JSON(http.StatusOK, DropsResponse{Available: available, Upcoming: upcoming})
	}
}

func queryDrops(ctx context.Context, pool *pgxpool.Pool, scheduled bool) ([]DropItem, error) {
	var where string
	if scheduled {
		where = `WHERE m.is_active = TRUE
		           AND m.sale_start IS NOT NULL
		           AND m.sale_start > NOW()`
	} else {
		where = `WHERE m.is_active = TRUE
		           AND (m.sale_start IS NULL OR m.sale_start <= NOW())
		           AND (m.sale_end   IS NULL OR m.sale_end   >  NOW())`
	}

	q := fmt.Sprintf(`
		SELECT %s
		FROM merchandise m
		LEFT JOIN events e  ON e.id  = m.event_id
		LEFT JOIN merch_collections mc ON mc.id = m.collection_id AND mc.is_active = TRUE
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
		merch                 models.Merchandise
		eventName             string
		collectionID          *string
		collectionName        string
		collectionCoverImage  *string
		collectionDescription *string
	}
	var rowsData []rowData
	var ids []string

	for rows.Next() {
		var m models.Merchandise
		var eventName, collectionName string
		var collectionID *string
		var collectionCoverImage  *string
		var collectionDescription *string

		if err := rows.Scan(
			&m.ID, &m.EventID, &eventName,
			&m.Name, &m.Description, &m.BasePrice,
			&m.Category, &m.DisplayOrder, &m.IsActive, &m.Images,
			&m.SaleStart, &m.SaleEnd, &m.CreatedAt, &m.UpdatedAt,
			&collectionID, &collectionName, &collectionCoverImage,
			&collectionDescription,
			&m.FulfillmentMode, &m.PickupLocation, &m.DeliveryFee,
		); err != nil {
			return nil, err
		}
		rowsData = append(rowsData, rowData{
			merch: m, eventName: eventName,
			collectionID: collectionID, collectionName: collectionName,
			collectionCoverImage:  collectionCoverImage,
			collectionDescription: collectionDescription,
		})
		ids = append(ids, m.ID)
	}

	if len(rowsData) == 0 {
		return []DropItem{}, nil
	}

	// Batch-load variants — avoids N+1
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

	now := time.Now()
	result := make([]DropItem, 0, len(rowsData))
	for _, rd := range rowsData {
		m := rd.merch
		m.Variants = variantMap[m.ID]

		di := DropItem{
			Merchandise:           m,
			EventName:             rd.eventName,
			CollectionID:          rd.collectionID,
			CollectionName:        rd.collectionName,
			CollectionCoverImage:  rd.collectionCoverImage,
			CollectionDescription: rd.collectionDescription,
			SaleStatus:            m.SaleStatus(),
			EffectivePrice:        m.BasePrice,
		}

		if m.SaleStart != nil && m.SaleStart.After(now) {
			secs := int64(m.SaleStart.Sub(now).Seconds())
			di.SecondsUntil = &secs
		}
		if m.SaleEnd != nil && m.SaleEnd.After(now) {
			secs := int64(m.SaleEnd.Sub(now).Seconds())
			di.SecondsRemain = &secs
		}

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