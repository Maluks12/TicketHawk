package handlers

import (
	"net/http"
	"strconv"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GET /admin/stats
//
// Returns aggregate platform metrics for the admin dashboard overview strip.
// Includes ticket revenue, merch revenue, combined totals, and an
// oversold_merch_orders count — non-zero means admin action is required.
func GetAdminStatsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats, err := repository.GetAdminStats(pool)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load stats"})
			return
		}
		c.JSON(http.StatusOK, stats)
	}
}

// GET /admin/events?status=published&search=festival&limit=20&cursor=...
//
// Returns a cursor-paginated list of all events for the admin events table.
// Each row includes ticket_revenue, merch_revenue, merch_units_sold, and
// total_revenue so the table can show a complete financial picture per event.
//
// Query params:
//   - status: filter by event status (draft, published, cancelled, completed)
//   - search: case-insensitive title search
//   - limit:  page size, default 20, max 100
//   - cursor: opaque pagination token from the previous response
func GetAdminEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
				limit = parsed
			}
		}

		var cursor *utils.Cursor
		if cursorStr := c.Query("cursor"); cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		events, nextCursor, err := repository.GetAdminEvents(
			pool, limit, cursor,
			c.Query("status"),
			c.Query("search"),
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load events"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

// GET /admin/merch/oversold
//
// Returns all merch orders where payment was captured but stock was exhausted
// at fulfillment time. These require manual resolution — issue a refund via
// Stitch or source additional stock and mark fulfilled.
//
// In normal operation this list is empty. The stats strip surfaces the count
// as an alert badge so admins see it immediately on login.
func GetOversoldMerchOrdersHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orders, err := repository.GetOversoldMerchOrders(pool)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load oversold orders"})
			return
		}
		if orders == nil {
			orders = []models.MerchOrder{}
		}
		c.JSON(http.StatusOK, gin.H{
			"data":  orders,
			"count": len(orders),
		})
	}
}

// GET /user/me
//
// Returns the authenticated user's own profile (id, email, name, phone, role).
func GetMyProfileHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)

		user, err := repository.GetUserProfile(pool, userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusOK, user)
	}
}

// GET /user/merch-orders?limit=20
//
// Returns the authenticated user's merch purchase history, newest first.
// Includes merch name, variant SKU, event title, fulfillment status, and
// price breakdown per order. Guest purchases (session_id only) are excluded
// since guests have no account to authenticate with.
//
// Query params:
//   - limit: number of orders to return, default 20, max 100
func GetMyMerchOrdersHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
				limit = parsed
			}
		}

		orders, err := repository.GetMerchOrdersForUser(pool, userID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load merch orders"})
			return
		}
		if orders == nil {
			orders = []models.MerchOrder{}
		}
		c.JSON(http.StatusOK, gin.H{
			"data":  orders,
			"count": len(orders),
		})
	}
}