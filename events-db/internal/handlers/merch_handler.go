package handlers

import (
	"net/http"

	"events-db/internal/cache"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Public read handlers ───────────────────────────────────────────────────────

func GetEventMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")
		ctx := c.Request.Context()

		categoryFilter := c.Query("category")
		attrKey := c.Query("attribute_key")
		attrVal := c.Query("attribute_value")
		hasFilters := categoryFilter != "" || attrKey != ""

		if !hasFilters && ec != nil {
			if items, ok := ec.GetMerchFull(ctx, eventID); ok {
				c.JSON(http.StatusOK, gin.H{"data": items, "count": len(items), "cached": true})
				return
			}
		}

		var items []models.Merchandise
		var err error

		if hasFilters {
			var catPtr, keyPtr, valPtr *string
			if categoryFilter != "" { catPtr = &categoryFilter }
			if attrKey != "" { keyPtr = &attrKey; valPtr = &attrVal }
			items, err = repository.GetMerchandiseByEventFiltered(pool, eventID, 0, catPtr, keyPtr, valPtr)
		} else {
			items, err = repository.GetMerchandiseByEvent(pool, eventID, 0)
		}

		if err != nil {
			utils.InternalError(c, err, "get event merch")
			return
		}

		if !hasFilters && ec != nil {
			ec.SetMerchFull(ctx, eventID, items)
		}

		c.JSON(http.StatusOK, gin.H{"data": items, "count": len(items)})
	}
}

func GetEventMerchPreviewHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")
		ctx := c.Request.Context()

		if ec != nil {
			if items, ok := ec.GetMerchPreview(ctx, eventID); ok {
				c.JSON(http.StatusOK, gin.H{"data": items})
				return
			}
		}

		items, err := repository.GetMerchandiseByEvent(pool, eventID, 4)
		if err != nil {
			utils.InternalError(c, err, "get merch preview")
			return
		}

		if ec != nil {
			ec.SetMerchPreview(ctx, eventID, items)
		}

		c.JSON(http.StatusOK, gin.H{"data": items})
	}
}

func GetMerchByIDHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		merchID := c.Param("id")
		ctx := c.Request.Context()

		if ec != nil {
			if item, ok := ec.GetMerchItem(ctx, merchID); ok {
				c.JSON(http.StatusOK, item)
				return
			}
		}

		item, err := repository.GetMerchandiseByID(pool, merchID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Merchandise not found"})
			return
		}

		if ec != nil {
			ec.SetMerchItem(ctx, merchID, item)
		}

		c.JSON(http.StatusOK, item)
	}
}

// ── Organiser write handlers ───────────────────────────────────────────────────

func OrgCreateMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.GetString("user_id")
		eventID := c.Param("id")
		var dto models.CreateMerchandiseDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		item, err := repository.CreateMerchandise(pool, &eventID, &dto, organizerID)
		if err != nil {
			if err == repository.ErrMerchNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found or access denied"})
				return
			}
			utils.InternalError(c, err, "create merchandise")
			return
		}

		if ec != nil {
			ec.BustMerch(c.Request.Context(), eventID)
		}

		c.JSON(http.StatusCreated, gin.H{
			"merch":   item,
			"message": "Merch item created and submitted for admin review. It will be visible to buyers once approved.",
		})
	}
}

func OrgCreateStandaloneMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		var dto models.CreateMerchandiseDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Standalone merch has no event, so the ownership guard inside
		// CreateMerchandise is a no-op here — organizerID is passed for
		// signature parity only.
		item, err := repository.CreateMerchandise(pool, nil, &dto, c.GetString("user_id"))
		if err != nil {
			utils.InternalError(c, err, "create standalone merchandise")
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"merch":   item,
			"message": "Merch item created and submitted for admin review. It will be visible to buyers once approved.",
		})
	}
}

func OrgUpdateMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.GetString("user_id")
		merchID := c.Param("id")

		var dto models.UpdateMerchandiseDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		item, err := repository.UpdateMerchandise(pool, merchID, &dto, organizerID)
		if err != nil {
			if err == repository.ErrMerchNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Merchandise not found or access denied"})
				return
			}
			if err.Error() == "no fields to update" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No fields provided to update"})
				return
			}
			utils.InternalError(c, err, "update merchandise")
			return
		}

		if ec != nil && item.EventID != nil {
			ec.BustMerch(c.Request.Context(), *item.EventID, merchID)
		}

		c.JSON(http.StatusOK, gin.H{"message": "Merchandise updated successfully", "item": item})
	}
}

func OrgDeactivateMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.GetString("user_id")
		merchID := c.Param("id")

		var eventID string
		if item, err := repository.GetMerchandiseByID(pool, merchID); err == nil {
			if item.EventID != nil {
				eventID = *item.EventID
			}
		}

		err := repository.DeactivateMerchandise(pool, merchID, organizerID)
		if err != nil {
			if err == repository.ErrMerchNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Merchandise not found or access denied"})
				return
			}
			utils.InternalError(c, err, "deactivate merchandise")
			return
		}

		if ec != nil && eventID != "" {
			ec.BustMerch(c.Request.Context(), eventID, merchID)
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Merchandise removed from listing",
			"note":    "Item deactivated — existing orders are not affected",
		})
	}
}

func OrgCreateVariantHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.GetString("user_id")
		merchID := c.Param("id")
		var dto models.CreateVariantDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		variant, err := repository.CreateVariant(pool, merchID, &dto, organizerID)
		if err != nil {
			if err == repository.ErrMerchNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Merch item not found or access denied"})
				return
			}
			utils.InternalError(c, err, "create variant")
			return
		}

		if ec != nil {
			var eventID *string
			pool.QueryRow(c.Request.Context(),
				`SELECT event_id FROM merchandise WHERE id = $1`, merchID,
			).Scan(&eventID)
			if eventID != nil {
				ec.BustMerch(c.Request.Context(), *eventID, merchID)
			}
		}

		c.JSON(http.StatusCreated, variant)
	}
}

func OrgUpdateVariantHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.GetString("user_id")
		variantID := c.Param("id")

		var dto models.UpdateVariantDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		variant, eventID, err := repository.UpdateVariant(pool, variantID, &dto, organizerID)
		if err != nil {
			if err == repository.ErrMerchNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Variant not found or access denied"})
				return
			}
			if err.Error() == "no fields to update" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No fields provided to update"})
				return
			}
			utils.InternalError(c, err, "update variant")
			return
		}

		if ec != nil && eventID != nil {
			ec.BustMerch(c.Request.Context(), *eventID, variant.MerchandiseID)
		}

		c.JSON(http.StatusOK, gin.H{
			"message":      "Variant updated successfully",
			"variant":      variant,
			"stock_status": variant.StockStatus(),
		})
	}
}

// ── Admin handlers ─────────────────────────────────────────────────────────────

// AdminCreateMerchHandler creates a standalone merch item, pre-approved.
// Admin-only. Route: POST /admin/merch
// Admin-created items bypass the organiser review queue and are visible immediately.
func AdminCreateMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		var dto models.CreateMerchandiseDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		item, err := repository.AdminCreateMerchandise(pool, nil, &dto)
		if err != nil {
			utils.InternalError(c, err, "admin create standalone merchandise")
			return
		}

		c.JSON(http.StatusCreated, item)
	}
}

// AdminCreateEventMerchHandler creates event-linked merch, pre-approved.
// Admin-only. Route: POST /admin/events/:id/merch
func AdminCreateEventMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")
		var dto models.CreateMerchandiseDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		item, err := repository.AdminCreateMerchandise(pool, &eventID, &dto)
		if err != nil {
			utils.InternalError(c, err, "admin create event merchandise")
			return
		}

		if ec != nil && item.EventID != nil {
			ec.BustMerch(c.Request.Context(), *item.EventID, item.ID)
		}

		c.JSON(http.StatusCreated, item)
	}
}

func GetAllMerchHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := repository.GetAllMerchandise(pool, 500)
		if err != nil {
			utils.InternalError(c, err, "get all merchandise")
			return
		}
		if items == nil {
			items = []models.Merchandise{}
		}
		c.JSON(http.StatusOK, gin.H{"data": items, "count": len(items)})
	}
}

func AdminUpdateMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		merchID := c.Param("id")
		var dto models.UpdateMerchandiseDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		item, err := repository.AdminUpdateMerchandise(pool, merchID, &dto)
		if err != nil {
			if err.Error() == "no fields to update" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No fields provided to update"})
				return
			}
			utils.InternalError(c, err, "admin update merchandise")
			return
		}
		if ec != nil && item.EventID != nil {
			ec.BustMerch(c.Request.Context(), *item.EventID, merchID)
		}
		c.JSON(http.StatusOK, gin.H{"message": "Merchandise updated", "item": item})
	}
}

func OrgResubmitMerchHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		merchID := c.Param("id")
		organizerID := c.GetString("user_id")

		if err := repository.OrgResubmitMerch(pool, merchID, organizerID); err != nil {
			if err == repository.ErrMerchNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Merch item not found or not in rejected state"})
				return
			}
			utils.InternalError(c, err, "resubmit merch")
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Merch item resubmitted for admin review", "merch_id": merchID})
	}
}

func AdminUpdateVariantHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		variantID := c.Param("id")

		var dto models.UpdateVariantDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		variant, err := repository.AdminUpdateVariant(pool, variantID, &dto)
		if err != nil {
			if err == repository.ErrMerchNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Variant not found"})
				return
			}
			if err.Error() == "no fields to update" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No fields provided to update"})
				return
			}
			utils.InternalError(c, err, "admin update variant")
			return
		}

		c.JSON(http.StatusOK, variant)
	}
}