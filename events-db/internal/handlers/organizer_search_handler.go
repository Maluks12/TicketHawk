package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SearchOrganizersHandler finds organisers by name.
//
//	GET /v1/organisers/search?q=soweto&limit=8
//
// Registered BEFORE /organisers/:slug so "search" is matched as a literal
// segment rather than being captured as a slug — otherwise this endpoint would
// resolve as "the organiser whose slug is 'search'" and 404 forever.
//
// A short query returns an EMPTY LIST, not a 400. The app calls this on every
// keystroke alongside the event search; a 400 at one and two characters would
// mean an error state flashing on screen while someone types normally. The
// event search endpoint returns 400 there, but it is called on a debounce and
// only once the user has committed to a query — different call pattern,
// different right answer.
func SearchOrganizersHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))

		limit := 8
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 20 {
				limit = parsed
			}
		}

		if len([]rune(query)) < 2 {
			c.JSON(http.StatusOK, gin.H{"data": []any{}, "count": 0})
			return
		}

		results, err := repository.SearchPublicOrganizers(pool, query, limit)
		if err != nil {
			utils.InternalError(c, err, "organiser search failed")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":  results,
			"count": len(results),
		})
	}
}