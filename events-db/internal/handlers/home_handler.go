package handlers

import (
	"events-db/internal/repository"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func HomeSectionsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := repository.GetHomeSections(pool)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to load home sections",
			})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}