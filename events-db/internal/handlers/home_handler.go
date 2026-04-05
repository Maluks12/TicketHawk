package handlers

import (
	"net/http"
	"strconv"

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

		// Resolve optional coordinates from ?lat=X&lng=Y
		var lat, lng *float64
		if latStr := c.Query("lat"); latStr != "" {
			if v, err := strconv.ParseFloat(latStr, 64); err == nil {
				lat = &v
			}
		}
		if lngStr := c.Query("lng"); lngStr != "" {
			if v, err := strconv.ParseFloat(lngStr, 64); err == nil {
				lng = &v
			}
		}

		// Only cache fully anonymous, location-free requests
		isPersonalized := userID != nil || (lat != nil && lng != nil)

		if !isPersonalized && ec != nil {
			if result, ok := ec.GetHome(ctx); ok {
				c.JSON(http.StatusOK, result)
				return
			}
		}

		result, err := repository.GetHomeSections(pool, userID, lat, lng)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load home sections"})
			return
		}

		if !isPersonalized && ec != nil {
			ec.SetHome(ctx, result)
		}

		c.JSON(http.StatusOK, result)
	}
}