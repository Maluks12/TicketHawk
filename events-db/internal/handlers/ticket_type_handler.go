package handlers

import (
	"errors"
	"events-db/internal/cache"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// Public
// ---------------------------------------------------------------------------

// GetTicketTypesHandler returns all visible ticket types for an event.
// Used by the event detail page to render the tier picker.
//
//	GET /events/:id/ticket-types
func GetTicketTypesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// The public event routes accept a SLUG or a UUID — GetEventBySlugHandler
		// resolves either, and the buyer web apps build links from the slug
		// (/events/new-event-2d694e55...). This handler did not: it passed the raw
		// param into a `WHERE event_id = $1` lookup, so every slug-based request
		// matched nothing and returned an empty list.
		//
		// The purchase page reads an empty list as "no tiers on this event" and
		// substitutes a synthetic GENERAL ADMISSION row priced at events.price, so
		// the failure was silent and looked like the tiers had been deleted: the
		// organiser saw both tiers On sale while buyers were offered a single
		// ticket that does not exist.
		//
		// ResolveVisibleEventID runs the same slug-or-UUID branch the analytics
		// beacons use and limits to publicly visible statuses — including
		// published_private, which is exactly what a private link is.
		eventID, ok := repository.ResolveVisibleEventID(pool, c.Param("id"))
		if !ok {
			// Same shape as a genuinely empty result, so probing ids cannot
			// distinguish a missing event from one that has no tiers.
			c.JSON(http.StatusOK, gin.H{"ticket_types": []*models.TicketType{}, "count": 0})
			return
		}

		types, err := repository.GetTicketTypesByEventID(pool, eventID)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch ticket types")
			return
		}

		// Filter to only visible + on-sale types for the public endpoint
		public := make([]*models.TicketType, 0, len(types))
		for _, tt := range types {
			if tt.IsOnSale() {
				public = append(public, tt)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"ticket_types": public,
			"count":        len(public),
		})
	}
}

// ---------------------------------------------------------------------------
// Organiser — all routes require AuthMiddleware + OrganizerOnlyMiddleware
// ---------------------------------------------------------------------------

// OrgGetTicketTypesHandler returns ALL types for an event (including hidden /
// not-yet-on-sale ones) so the organiser's dashboard can manage them.
//
//	GET /organiser/events/:id/ticket-types
func OrgGetTicketTypesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")

		types, err := repository.GetTicketTypesByEventID(pool, eventID)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch ticket types")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"ticket_types": types,
			"count":        len(types),
		})
	}
}

// OrgCreateTicketTypesHandler adds one or more ticket types to an existing event.
// The organiser can call this at any time — before or after publishing.
//
//	POST /organiser/events/:id/ticket-types
//
// Body: { "types": [ { "name": "VIP", "price": 450.00, "capacity": 50 }, … ] }
func OrgCreateTicketTypesHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		eventID := c.Param("id")

		var dto struct {
			Types []models.CreateTicketTypeDTO `json:"types" binding:"required,min=1,dive"`
		}
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		types, err := repository.CreateTicketTypes(pool, eventID, dto.Types, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrTicketTypeForbidden) {
				c.JSON(http.StatusForbidden, gin.H{"error": "event not found or access denied"})
				return
			}
			utils.InternalError(c, err, "failed to create ticket types")
			return
		}

		// Bust the cached event payload. Tier writes are the one organiser
		// mutation that never did this, so a new or edited tier — and the
		// max_capacity raise that comes with it — stayed invisible to buyers
		// reading the cached event until the entry aged out.
		if ec != nil {
			ec.BustOnEventUpdate(c.Request.Context())
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":      "Ticket types created",
			"ticket_types": types,
		})
	}
}

// OrgReplaceTicketTypesHandler atomically swaps all ticket types for an event
// with a new set. Blocked when any type already has bookings, to protect
// booking history. Use PATCH on individual types to update live events.
//
//	PUT /organiser/events/:id/ticket-types
func OrgReplaceTicketTypesHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		eventID := c.Param("id")

		var dto models.ReplaceTicketTypesDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		types, err := repository.ReplaceTicketTypes(pool, eventID, dto.Types, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrTicketTypeForbidden) {
				c.JSON(http.StatusForbidden, gin.H{"error": "event not found or access denied"})
				return
			}
			if errors.Is(err, repository.ErrTicketTypeHasSales) {
				c.JSON(http.StatusConflict, gin.H{
					"error": "Cannot replace ticket types after sales have begun. Update individual types with PATCH instead.",
				})
				return
			}
			utils.InternalError(c, err, "failed to replace ticket types")
			return
		}

		// Bust the cached event payload. Tier writes are the one organiser
		// mutation that never did this, so a new or edited tier — and the
		// max_capacity raise that comes with it — stayed invisible to buyers
		// reading the cached event until the entry aged out.
		if ec != nil {
			ec.BustOnEventUpdate(c.Request.Context())
		}

		c.JSON(http.StatusOK, gin.H{
			"message":      "Ticket types replaced",
			"ticket_types": types,
		})
	}
}

// OrgUpdateTicketTypeHandler applies a partial update to a single ticket type.
// Safe to call on live events — only updates the fields you send.
//
//	PATCH /organiser/ticket-types/:typeId
func OrgUpdateTicketTypeHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		typeID := c.Param("typeId")

		var dto models.UpdateTicketTypeDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		tt, err := repository.UpdateTicketType(pool, typeID, &dto, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrTicketTypeNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ticket type not found"})
				return
			}
			if errors.Is(err, repository.ErrTicketTypeForbidden) {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
			utils.InternalError(c, err, "failed to update ticket type")
			return
		}

		// Bust the cached event payload. Tier writes are the one organiser
		// mutation that never did this, so a new or edited tier — and the
		// max_capacity raise that comes with it — stayed invisible to buyers
		// reading the cached event until the entry aged out.
		if ec != nil {
			ec.BustOnEventUpdate(c.Request.Context())
		}

		c.JSON(http.StatusOK, gin.H{
			"message":     "Ticket type updated",
			"ticket_type": tt,
		})
	}
}

// OrgDeleteTicketTypeHandler removes a ticket type. Blocked if any bookings
// already exist for it — set is_visible = false instead to hide it.
//
//	DELETE /organiser/ticket-types/:typeId
func OrgDeleteTicketTypeHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		typeID := c.Param("typeId")

		err := repository.DeleteTicketType(pool, typeID, organizerID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrTicketTypeNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ticket type not found"})
				return
			}
			if errors.Is(err, repository.ErrTicketTypeForbidden) {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
			if errors.Is(err, repository.ErrTicketTypeHasSales) {
				c.JSON(http.StatusConflict, gin.H{
					"error": "Cannot delete a ticket type that has existing bookings. Set is_visible = false to hide it instead.",
				})
				return
			}
			utils.InternalError(c, err, "failed to delete ticket type")
			return
		}

		// Bust the cached event payload. Tier writes are the one organiser
		// mutation that never did this, so a new or edited tier — and the
		// max_capacity raise that comes with it — stayed invisible to buyers
		// reading the cached event until the entry aged out.
		if ec != nil {
			ec.BustOnEventUpdate(c.Request.Context())
		}

		c.JSON(http.StatusOK, gin.H{"message": "Ticket type deleted"})
	}
}