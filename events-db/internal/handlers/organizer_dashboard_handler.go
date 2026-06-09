package handlers

import (
	"net/http"
	"strconv"

	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// All routes in this file are registered under /organiser and protected by
// AuthMiddleware + OrganizerOnlyMiddleware. Register them in main.go inside
// the existing orgMerch group:
//
//   orgMerch.GET("/dashboard/stats",               handlers.GetOrganizerDashboardStatsHandler(pool))
//   orgMerch.GET("/dashboard/events",              handlers.GetOrganizerDashboardEventsHandler(pool))
//   orgMerch.GET("/dashboard/events/:id/stats",    handlers.GetOrganizerEventStatsHandler(pool))
//   orgMerch.GET("/dashboard/events/:id/bookings", handlers.GetOrganizerEventBookingsHandler(pool))
//   orgMerch.GET("/dashboard/events/:id/merch",    handlers.GetOrganizerEventMerchHandler(pool))
//   orgMerch.GET("/dashboard/payouts",             handlers.GetOrganizerPayoutSummaryHandler(pool))

// ── GET /organiser/dashboard/stats ───────────────────────────────────────────
//
// Returns the top-level overview panel for the organiser dashboard:
//
//   {
//     "total_events": 12,
//     "published_events": 8,
//     "total_bookings": 430,
//     "gross_ticket_revenue": 215000.00,
//     "platform_fees_paid":    6272.50,
//     "net_ticket_earnings":  208727.50,
//     "merch_units_sold": 95,
//     "gross_merch_revenue": 14250.00,
//     "total_net_earnings": 222977.50,
//     "bookings_today": 7,
//     "revenue_today": 3500.00
//   }
func GetOrganizerDashboardStatsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.MustGet("user_id").(string)

		stats, err := repository.GetOrganizerStats(pool, orgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load dashboard stats"})
			return
		}
		c.JSON(http.StatusOK, stats)
	}
}

// ── GET /organiser/dashboard/events ──────────────────────────────────────────
//
// Returns a cursor-paginated table of the organiser's events with full revenue
// figures attached to each row.
//
// Query params:
//   status  — filter by event status: draft | published | cancelled | completed
//   limit   — page size, default 20, max 100
//   cursor  — opaque token from previous response
//
// Example row:
//   {
//     "id": "...",
//     "title": "Chelsea vs Arsenal",
//     "status": "published",
//     "tickets_sold": 342,
//     "total_bookings": 342,
//     "gross_ticket_revenue": 171000.00,
//     "platform_fees_paid":    4989.30,
//     "net_ticket_earnings":  166010.70,
//     "merch_units_sold": 67,
//     "gross_merch_revenue": 8040.00,
//     "total_net_earnings": 174050.70
//   }
func GetOrganizerDashboardEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.MustGet("user_id").(string)

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

		events, nextCursor, err := repository.GetOrganizerEventRows(
			pool, orgID,
			c.Query("status"),
			limit, cursor,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load events"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"events":      events,
			"count":       len(events),
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

// ── GET /organiser/dashboard/events/:id/stats ─────────────────────────────────
//
// Returns the complete financial breakdown for a single event owned by this
// organiser. Returns 404 if the event belongs to a different organiser.
//
// Example response:
//   {
//     "event_id": "...",
//     "title": "Chelsea vs Arsenal",
//     "total_bookings": 342,
//     "pending_bookings": 5,
//     "cancelled_bookings": 12,
//     "gross_ticket_revenue": 171000.00,
//     "platform_fees_paid":    4989.30,
//     "net_ticket_earnings":  166010.70,
//     "total_refunded": 1200.00,
//     "merch_units_sold": 67,
//     "gross_merch_revenue": 8040.00,
//     "oversold_merch_orders": 0,
//     "total_net_earnings": 174050.70,
//     "capacity_used_pct": 68.4
//   }
func GetOrganizerEventStatsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.MustGet("user_id").(string)
		eventID := c.Param("id")

		stats, err := repository.GetOrganizerEventStats(pool, eventID, orgID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}

		// Reshape to match the frontend EventStats interface:
		// { event: { id, title, ... }, total_bookings, confirmed_bookings, ... }
		c.JSON(http.StatusOK, gin.H{
			"event": gin.H{
				"id":              stats.EventID,
				"title":           stats.Title,
				"status":          stats.Status,
				"start_time":      stats.StartTime,
				"end_time":        stats.EndTime,
				"city":            stats.City,
				"venue_name":      stats.VenueName,
				"price":           stats.Price,
				"currency":        stats.Currency,
				"tickets_sold":    stats.TicketsSold,
				"max_capacity":    stats.MaxCapacity,
				"views":           stats.Views,
				"cover_image_url": stats.CoverImageURL,
			},
			"total_revenue":      stats.TotalNetEarnings,
			"total_bookings":     stats.TotalBookings + stats.PendingBookings,
			"confirmed_bookings": stats.TotalBookings,
			"pending_bookings":   stats.PendingBookings,
			"active_campaign":    nil,
		})
	}
}

// ── GET /organiser/dashboard/events/:id/bookings ──────────────────────────────
//
// Returns a cursor-paginated attendee/booking list for one of the organiser's
// events. Each row includes the attendee name, email, ticket quantity, and net
// earning to the organiser from that booking.
//
// Query params:
//   status  — filter: pending | confirmed | cancelled | refunded
//   limit   — default 50, max 200
//   cursor  — opaque pagination token
//
// Example row:
//   {
//     "id": "...",
//     "booking_number": "BK-20250401-A3F7B2",
//     "ticket_quantity": 2,
//     "subtotal": 1000.00,
//     "platform_fee": 29.30,
//     "net_earning": 970.70,
//     "currency": "ZAR",
//     "booking_status": "confirmed",
//     "attendee_name": "John Doe",
//     "attendee_email": "john@example.com",
//     "confirmed_at": "2025-04-01T14:22:00Z"
//   }
func GetOrganizerEventBookingsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.MustGet("user_id").(string)
		eventID := c.Param("id")

		limit := 50
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
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

		bookings, nextCursor, err := repository.GetOrganizerEventBookings(
			pool, eventID, orgID,
			c.Query("status"),
			limit, cursor,
		)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"bookings":    bookings,
			"count":       len(bookings),
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

// ── GET /organiser/dashboard/events/:id/merch ─────────────────────────────────
//
// Returns merch sales grouped by variant for one of the organiser's events.
// Each row shows units sold, unit price, and gross revenue for that SKU.
// Any oversold quantities are surfaced so the organiser knows action is needed.
//
// Example row:
//   {
//     "merchandise_id": "...",
//     "merch_name": "Chelsea Home Kit",
//     "variant_id": "...",
//     "variant_sku": "KIT-L-HOME-2526",
//     "attributes": "{\"size\":\"L\",\"colour\":\"home\"}",
//     "units_sold": 18,
//     "unit_price": 1200.00,
//     "gross_revenue": 21600.00,
//     "oversold_quantity": 0
//   }
func GetOrganizerEventMerchHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.MustGet("user_id").(string)
		eventID := c.Param("id")

		sales, err := repository.GetOrganizerEventMerchSales(pool, eventID, orgID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}

		// Surface a top-level flag so the frontend can show an alert banner
		// without having to scan every row.
		totalOversold := 0
		for _, s := range sales {
			totalOversold += s.Oversold
		}

		c.JSON(http.StatusOK, gin.H{
			"sales":          sales,
			"count":          len(sales),
			"has_oversold":   totalOversold > 0,
			"total_oversold": totalOversold,
		})
	}
}

// ── GET /organiser/dashboard/payouts ──────────────────────────────────────────
//
// Returns a lifetime payout summary across all of the organiser's events.
// This tells the organiser exactly what they have earned after platform fees,
// and how much has been refunded back to buyers.
//
// Response:
//   {
//     "currency": "ZAR",
//     "total_gross_revenue":  229250.00,
//     "total_platform_fees":    6669.30,
//     "total_ticket_earnings": 222580.70,
//     "total_merch_earnings":   14250.00,
//     "total_earnings":        236830.70,
//     "total_refunds_issued":    2400.00
//   }
func GetOrganizerPayoutSummaryHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.MustGet("user_id").(string)

		summary, err := repository.GetOrganizerPayoutSummary(pool, orgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load payout summary"})
			return
		}
		c.JSON(http.StatusOK, summary)
	}
}