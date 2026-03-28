package handlers

import (
	"events-db/internal/cache"
	"events-db/internal/models"
	"events-db/internal/repository"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GetEventMerchHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")
		ctx := c.Request.Context()

		if ec != nil {
			if items, ok := ec.GetMerchFull(ctx, eventID); ok {
				c.JSON(http.StatusOK, gin.H{"data": items, "count": len(items)})
				return
			}
		}

		items, err := repository.GetMerchandiseByEvent(pool, eventID, 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load merchandise"})
			return
		}

		if ec != nil {
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load merchandise preview"})
			return
		}

		if ec != nil {
			ec.SetMerchPreview(ctx, eventID, items)
		}

		c.JSON(http.StatusOK, gin.H{"data": items})
	}
}

// Bust cache when organiser creates merch or a variant
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create merchandise"})
			return
		}

		// Bust both preview and full list for this event
		if ec != nil {
			ec.BustMerch(c.Request.Context(), eventID)
		}

		c.JSON(http.StatusCreated, item)
	}
}

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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create variant"})
			return
		}

		// Need the eventID to bust the right cache key
		// Fetch it from the merch record
		if ec != nil {
			var eventID string
			pool.QueryRow(c.Request.Context(),
				`SELECT event_id FROM merchandise WHERE id = $1`, merchID,
			).Scan(&eventID)
			if eventID != "" {
				ec.BustMerch(c.Request.Context(), eventID)
			}
		}

		c.JSON(http.StatusCreated, variant)
	}
}