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

// GetEventMerchHandler returns the full merch catalogue for an event.
// Supports optional ?category= and ?attribute[key]=value filters.
// Filters bypass the cache since they produce user-specific subsets.
func GetEventMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
    return func(c *gin.Context) {
        eventID := c.Param("id")
        ctx := c.Request.Context()

        // Check for filters — filtered requests skip cache
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
            if attrKey != ""        { keyPtr = &attrKey; valPtr = &attrVal }
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

// GetEventMerchPreviewHandler returns the first 4 items for the event detail page.
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

// GetMerchByIDHandler returns a single merch item with full variant detail
// and a stock summary. This powers the dedicated merch item page.
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

// OrgCreateMerchHandler creates a new merch item for an event the organiser owns.
func OrgCreateMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
    return func(c *gin.Context) {
        eventID := c.Param("id")
        var dto models.CreateMerchandiseDTO
        if err := c.ShouldBindJSON(&dto); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }

        item, err := repository.CreateMerchandise(pool, eventID, &dto)
        if err != nil {
            utils.InternalError(c, err, "create merchandise")
            return
        }

        if ec != nil {
            ec.BustMerch(c.Request.Context(), eventID)
        }

        c.JSON(http.StatusCreated, item)
    }
}

// OrgUpdateMerchHandler updates an existing merch item.
// Only fields provided in the request body are updated.
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

        if ec != nil {
            ec.BustMerch(c.Request.Context(), item.EventID, merchID)
        }

        c.JSON(http.StatusOK, gin.H{
            "message": "Merchandise updated successfully",
            "item":    item,
        })
    }
}

// OrgDeactivateMerchHandler soft-deletes a merch item (sets is_active = false).
// Does not delete the record — keeps order history intact.
func OrgDeactivateMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
    return func(c *gin.Context) {
        organizerID := c.GetString("user_id")
        merchID := c.Param("id")

        // Need eventID before deactivating for cache bust
        // GetMerchandiseByID gives us this cheaply (will be cached)
        var eventID string
        if item, err := repository.GetMerchandiseByID(pool, merchID); err == nil {
            eventID = item.EventID
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

// OrgCreateVariantHandler adds a new size/colour variant to a merch item.
func OrgCreateVariantHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
    return func(c *gin.Context) {
        merchID := c.Param("id")
        var dto models.CreateVariantDTO
        if err := c.ShouldBindJSON(&dto); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }

        variant, err := repository.CreateVariant(pool, merchID, &dto)
        if err != nil {
            utils.InternalError(c, err, "create variant")
            return
        }

        // Bust cache — fetch eventID to build the right key
        if ec != nil {
            var eventID string
            pool.QueryRow(c.Request.Context(),
                `SELECT event_id FROM merchandise WHERE id = $1`, merchID,
            ).Scan(&eventID)
            if eventID != "" {
                ec.BustMerch(c.Request.Context(), eventID, merchID)
            }
        }

        c.JSON(http.StatusCreated, variant)
    }
}

// OrgUpdateVariantHandler updates stock, price override, or active status
// on a specific variant. This is how organisers restock or reprice SKUs.
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

        // Bust both the event merch list and the specific item cache
        if ec != nil && eventID != "" {
            ec.BustMerch(c.Request.Context(), eventID, variant.MerchandiseID)
        }

        c.JSON(http.StatusOK, gin.H{
            "message": "Variant updated successfully",
            "variant": variant,
            "stock_status": variant.StockStatus(),
        })
    }
}