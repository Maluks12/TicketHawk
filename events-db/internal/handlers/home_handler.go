package handlers

import (
	"events-db/internal/cache"
	"events-db/internal/repository"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func HomeSectionsHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx := c.Request.Context()

        // Cache check
        if ec != nil {
            if result, ok := ec.GetHome(ctx); ok {
                c.JSON(http.StatusOK, result)
                return
            }
        }

        // Postgres fallback
        result, err := repository.GetHomeSections(pool)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load home sections"})
            return
        }

        // Populate cache
        if ec != nil {
            ec.SetHome(ctx, result)
        }

        c.JSON(http.StatusOK, result)
    }
}