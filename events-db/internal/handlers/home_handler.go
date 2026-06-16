package handlers

import (
	"net/http"

	"events-db/internal/cache"
	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HomeSectionsHandler assembles the homepage feed.
//
// Query parameters (all optional):
//
//	lat  float  — user latitude  (from browser Geolocation API)
//	lng  float  — user longitude (from browser Geolocation API)
//
// Auth:
//
//	Requires OptionalAuth middleware. If a valid Bearer token is present,
//	the "For You" section is injected at the top and sponsored ads are
//	personalised by category affinity. Without a token, the feed is
//	purely algorithmic (trending, soon, weekend, recent, featured).
//
// Cache:
//
//	Anonymous requests with no coordinates hit the Redis home cache (60s TTL).
//	Requests with a userID or lat/lng bypass the cache — the result is
//	specific to that person/location and must not be shared across users.
//
// Route in main.go:
//
//	router.GET("/home", middleware.OptionalAuth(cfg), handlers.HomeSectionsHandler(pool, eventCache))
func HomeSectionsHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Resolve optional userID from JWT (set by OptionalAuth)
		var userID *string
		if uid, exists := c.Get("user_id"); exists {
			s := uid.(string)
			userID = &s
		}

		// Resolve optional city from ?city=Johannesburg
		var city *string
		if cityStr := c.Query("city"); cityStr != "" {
			city = &cityStr
		}

		isPersonalized := userID != nil || city != nil

		// The shared sections (trending, happening soon, this weekend, recently
		// added, featured) are identical for every visitor, so we cache them
		// globally and reuse them for signed-in users too. Only the personalised
		// "For You" / "Near You" sections are computed fresh per request.
		var shared *repository.HomeSectionsResult
		if ec != nil {
			if cached, ok := ec.GetHome(ctx); ok {
				shared = cached
			}
		}
		if shared == nil {
			result, err := repository.SharedHomeSections(pool)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load home sections"})
				return
			}
			shared = result
			if ec != nil {
				ec.SetHome(ctx, shared)
			}
		}

		// Anonymous, location-free request → the cached shared feed is the whole
		// response.
		if !isPersonalized {
			c.JSON(http.StatusOK, shared)
			return
		}

		// Personalised request → run only the cheap user-specific sections and
		// merge them around the cached shared ones (For You first, Near You last).
		forYou, nearYou := repository.PersonalHomeSections(pool, userID, city)

		merged := make([]repository.HomeSection, 0, len(forYou)+len(shared.Sections)+len(nearYou))
		merged = append(merged, forYou...)
		merged = append(merged, shared.Sections...)
		merged = append(merged, nearYou...)

		c.JSON(http.StatusOK, repository.HomeSectionsResult{Sections: merged})
	}
}