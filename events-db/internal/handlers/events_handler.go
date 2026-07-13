package handlers

import (
	"errors"
	"events-db/internal/cache"
	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FIX 5: renamed CreateEventHandlers → CreateEventHandler (singular, consistent naming)
func CreateEventHandler(pool *pgxpool.Pool, ec *cache.EventCache, emailService *services.EmailService, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")

		var dto models.CreateEventDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		if dto.EndTime.Before(dto.StartTime) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "End time must be after start time"})
			return
		}

		// Admin-created events go live immediately and are exempt from the
		// listing fee, so they appear on the home feed like any normal event.
		event, err := repository.CreateEvent(pool, &dto, userID.(string), models.EventPublished, true)
		if err != nil {
			utils.InternalError(c, err, "failed to create event")
			return
		}

		// Path 4: created on behalf of an organiser — hand it over right now.
		// The event exists and is published either way; a handover failure is
		// reported but doesn't roll back creation (the admin can retransfer
		// via PUT /admin/events/:id/organizer).
		var handover gin.H
		if dto.OrganizerEmail != nil && *dto.OrganizerEmail != "" {
			businessName := ""
			if dto.OrganizerBusinessName != nil {
				businessName = *dto.OrganizerBusinessName
			}
			info := &repository.EventOwnershipInfo{
				ID:          event.ID,
				Title:       event.Title,
				OrganizerID: userID.(string),
			}
			handover, err = handoverEventToOrganizer(pool, emailService, cfg, info, *dto.OrganizerEmail, businessName)
			if err != nil {
				log.Warn().Err(err).Str("event_id", event.ID).
					Msg("admin create: event created but handover failed — retransfer via PUT /admin/events/:id/organizer")
				handover = gin.H{"error": "Event was created, but the handover failed. Use the transfer endpoint to retry."}
			}
		}

		if ec != nil {
			ec.BustOnPublish(c.Request.Context())
		}

		resp := gin.H{
			"message": "Event created and published to the home page",
			"event":   event,
		}
		if handover != nil {
			resp["handover"] = handover
		}
		c.JSON(http.StatusCreated, resp)
	}
}


func DeleteEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")
		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		err := repository.AdminDeleteEvent(pool, eventID)
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrEventNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			case errors.Is(err, repository.ErrEventHasBookings):
				c.JSON(http.StatusConflict, gin.H{
					"error": "This event has bookings or orders and can't be deleted. Cancel the event instead.",
				})
			default:
				utils.InternalError(c, err, "failed to delete event")
			}
			return
		}

		// Remove the event from the home/feed caches immediately.
		if ec != nil {
			ec.BustOnEventUpdate(c.Request.Context())
		}

		c.JSON(http.StatusOK, gin.H{"message": "Event deleted successfully"})
	}
}

// FIX 5: renamed TrendingEventHandlers → TrendingEventHandler
func TrendingEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		cursorStr := c.Query("cursor")

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		var cursor *utils.Cursor
		if cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		if cursorStr == "" && ec != nil {
			if page, ok := ec.GetTrending(c.Request.Context(), limit); ok {
				c.JSON(http.StatusOK, gin.H{
					"data":        page.Events,
					"next_cursor": utils.EncodeCursor(page.NextCursor),
				})
				return
			}
		}

		events, nextCursor, err := repository.TrendingEvents(pool, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch trending events")
			return
		}

		if cursorStr == "" && ec != nil {
			ec.SetTrending(c.Request.Context(), limit, cache.EventPage{
				Events:     events,
				NextCursor: nextCursor,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

func UpdateEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		eventID := c.Param("id")

		var dto models.UpdateEventDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		if ec != nil {
			ec.BustOnEventUpdate(c.Request.Context())
		}

		event, err := repository.UpdateEvents(pool, eventID, &dto, userID.(string))
		if err != nil {
			utils.InternalError(c, err, "failed to update event")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Event updated successfully",
			"event":   event,
		})
	}
}

// AdminUpdateEventHandler is the admin-only variant of UpdateEventHandler.
// Unlike the organizer version, it does NOT enforce organizer_id ownership —
// admins can edit any event regardless of who created it.
func AdminUpdateEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")

		var dto models.UpdateEventDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		if ec != nil {
			ec.BustOnEventUpdate(c.Request.Context())
		}

		event, err := repository.AdminUpdateEvent(pool, eventID, &dto)
		if err != nil {
			utils.InternalError(c, err, "failed to update event")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Event updated successfully",
			"event":   event,
		})
	}
}

// FIX 5: renamed GetEventByIDHandlers → GetEventByIDHandler
func GetEventByIDHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")

		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		var userID *string
		if uid, exists := c.Get("user_id"); exists {
			uidStr := uid.(string)
			userID = &uidStr
		}

		if ec != nil {
			ec.IncrementView(c.Request.Context(), eventID)
		} else {
			_ = repository.IncrementViews(pool, eventID)
		}

		event, err := repository.GetEventByID(pool, eventID, userID)
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
				return
			}
			utils.InternalError(c, err, "failed to fetch event")
			return
		}

		// Add Redis-buffered views not yet flushed to Postgres so the client
		// always sees an up-to-date count without waiting 60s for the flush.
		if ec != nil {
			event.Views += int(ec.GetPendingViewCount(c.Request.Context(), eventID))
		}

		c.JSON(http.StatusOK, event)
	}
}

// AdminGetEventByIDHandler is the admin-only variant. It returns any event
// regardless of status (draft, pending, published, cancelled) so admins can
// view and edit events in all states.
func AdminGetEventByIDHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")

		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		event, err := repository.AdminGetEventByID(pool, eventID)
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
				return
			}
			utils.InternalError(c, err, "failed to fetch event")
			return
		}

		if ec != nil {
			event.Views += int(ec.GetPendingViewCount(c.Request.Context(), eventID))
		}

		c.JSON(http.StatusOK, event)
	}
}

// GetEventBySlugHandler fetches a single published event by its URL slug.
// It also accepts a bare UUID so the mobile app's GET /events/:id call
// (which passes event.id) resolves correctly alongside slug-based links.
//
//	GET /events/:id
func GetEventBySlugHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		slugOrID := c.Param("id")

		if slugOrID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "event id or slug is required"})
			return
		}

		var event *models.Event
		var err error

		// If it looks like a UUID, try GetEventByID first so we don't force
		// callers to know (or construct) the slug.
		if _, uuidErr := uuid.Parse(slugOrID); uuidErr == nil {
			event, err = repository.GetEventByID(pool, slugOrID, nil)
		} else {
			event, err = repository.GetEventBySlug(pool, slugOrID)
		}

		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
				return
			}
			utils.InternalError(c, err, "failed to fetch event")
			return
		}

		// Add any Redis-buffered (not yet flushed) views so the client always
		// sees an up-to-date count without waiting 60s for the flush.
		if ec != nil {
			event.Views += int(ec.GetPendingViewCount(c.Request.Context(), event.ID))
		}

		c.JSON(http.StatusOK, event)
	}
}

// TrackEventViewHandler is the single place where a view is counted.
// Called by the frontend once when the user opens the event detail screen.
// Separating this from GetEventBySlugHandler means background fetches, admin
// checks, and ticket lookups no longer inflate the view count.
//
//	POST /v1/events/:id/view
func TrackEventViewHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		slugOrID := c.Param("id")
		if slugOrID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "event id or slug is required"})
			return
		}

		// Resolve the event so views are always stored against the stable UUID.
		// The :id param may be a bare UUID (mobile app passes event.id) or a
		// slug (web links), so handle both — mirroring GetEventBySlugHandler.
		var event *models.Event
		var err error
		if _, uuidErr := uuid.Parse(slugOrID); uuidErr == nil {
			event, err = repository.GetEventByID(pool, slugOrID, nil)
		} else {
			event, err = repository.GetEventBySlug(pool, slugOrID)
		}
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
				return
			}
			utils.InternalError(c, err, "failed to resolve event")
			return
		}

		if ec != nil {
			ec.IncrementView(c.Request.Context(), event.ID)
		} else {
			_ = repository.IncrementViews(pool, event.ID)
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// FIX 5: renamed GetAllEventsHandlers → GetAllEventsHandler
func GetAllEventsHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		cursorStr := c.Query("cursor")

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		var cursor *utils.Cursor
		if cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		if cursorStr == "" && ec != nil {
			if page, ok := ec.GetRecentlyAdded(c.Request.Context(), limit); ok {
				c.JSON(http.StatusOK, gin.H{
					"data":        page.Events,
					"next_cursor": utils.EncodeCursor(page.NextCursor),
				})
				return
			}
		}

		events, nextCursor, err := repository.GetAllEvents(pool, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "failed to get events")
			return
		}

		if cursorStr == "" && ec != nil {
			ec.SetRecentlyAdded(c.Request.Context(), limit, cache.EventPage{
				Events:     events,
				NextCursor: nextCursor,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

// FIX 5: renamed FeaturedEventsHandlers → FeaturedEventsHandler
func FeaturedEventsHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		cursorStr := c.Query("cursor")

		limit := 12
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		var cursor *utils.Cursor
		if cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		if cursorStr == "" && ec != nil {
			if page, ok := ec.GetFeatured(c.Request.Context(), limit); ok {
				c.JSON(http.StatusOK, gin.H{
					"data":        page.Events,
					"next_cursor": utils.EncodeCursor(page.NextCursor),
				})
				return
			}
		}

		events, nextCursor, err := repository.FeaturedEvents(pool, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch featured events")
			return
		}

		if cursorStr == "" && ec != nil {
			ec.SetFeatured(c.Request.Context(), limit, cache.EventPage{
				Events:     events,
				NextCursor: nextCursor,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

// FIX 5: renamed ActiveEventsHandlers → ActiveEventsHandler
func ActiveEventsHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		cursorStr := c.Query("cursor")

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		var cursor *utils.Cursor
		if cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		if cursorStr == "" && ec != nil {
			if page, ok := ec.GetHappeningSoon(c.Request.Context(), limit); ok {
				c.JSON(http.StatusOK, gin.H{
					"data":        page.Events,
					"next_cursor": utils.EncodeCursor(page.NextCursor),
				})
				return
			}
		}

		events, nextCursor, err := repository.ActiveEvents(pool, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "failed to get active events")
			return
		}

		if cursorStr == "" && ec != nil {
			ec.SetHappeningSoon(c.Request.Context(), limit, cache.EventPage{
				Events:     events,
				NextCursor: nextCursor,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

// FIX 5: renamed GetEventCategory → GetEventCategoryHandler (now consistent)
func GetEventCategoryHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		cursorStr := c.Query("cursor")

		categoryID, err := strconv.Atoi(id)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Category ID"})
			return
		}

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		var cursor *utils.Cursor
		if cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		if cursorStr == "" && ec != nil {
			if page, ok := ec.GetCategory(c.Request.Context(), categoryID, limit); ok {
				c.JSON(http.StatusOK, gin.H{
					"data":        page.Events,
					"next_cursor": utils.EncodeCursor(page.NextCursor),
				})
				return
			}
		}

		events, nextCursor, err := repository.GetEventBycategory(pool, categoryID, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "failed to get category")
			return
		}

		if cursorStr == "" && ec != nil {
			ec.SetCategory(c.Request.Context(), categoryID, limit, cache.EventPage{
				Events:     events,
				NextCursor: nextCursor,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

func PublishEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		eventID := c.Param("id")

		err := repository.PublishEvent(pool, eventID, userID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotPublishable) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Event not found or not in a submittable state (must be draft or rejected)"})
				return
			}
			if errors.Is(err, repository.ErrListingFeeRequired) {
				c.JSON(http.StatusPaymentRequired, gin.H{
					"error": "Listing fee required. This is a free event with 20+ ticket capacity. " +
						"As the organiser you must pay R5 per ticket upfront (ticket holders are not charged). " +
						"Starting at R100 for 20 tickets, going up with capacity.",
					"hint": "Call GET /organiser/events/:id/listing-fee to see the exact amount, " +
						"then POST /organiser/events/:id/listing-fee/initiate to pay.",
				})
				return
			}
			utils.InternalError(c, err, "failed to publish event")
			return
		}

		if ec != nil {
			ec.BustOnPublish(c.Request.Context())
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Event submitted for admin review. It will go live once an admin approves it.",
			"note":    "Event is now visible to all users",
		})
	}
}

func SearchEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		cursorStr := c.Query("cursor")

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "search query is required"})
			return
		}

		if len([]rune(query)) < 3 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "search query must be at least 3 characters"})
			return
		}

		var cursor *utils.Cursor
		if cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
				return
			}
		}

		events, nextCursor, err := repository.SearchEvents(pool, query, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "failed to search events")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

func CancelEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		eventID := c.Param("id")

		var input struct {
			Reason string `json:"reason" binding:"required,min=10"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cancellation reason required (min 10 characters)"})
			return
		}

		err := repository.CancelEvent(pool, eventID, input.Reason, userID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotCancellable) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Event not found or cannot be cancelled"})
				return
			}
			utils.InternalError(c, err, "failed to cancel event")
			return
		}

		if ec != nil {
			ec.BustOnEventUpdate(c.Request.Context())
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Event cancelled successfully",
			"note":    "All attendees will be automatically refunded",
		})
	}
}

func GetOrganizerEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID, _ := c.Get("user_id")
		status := c.DefaultQuery("status", "all")

		events, err := repository.GetOrganizerEvents(pool, organizerID.(string), status)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch organizer events")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"events": events,
			"total":  len(events),
		})
	}
}

// GetCategoriesHandler returns all active categories ordered by display_order.
//
//	GET /v1/categories
func GetCategoriesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cats, err := repository.GetCategories(pool)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch categories")
			return
		}
		c.JSON(http.StatusOK, gin.H{"categories": cats})
	}
}

// GetEventsByIDsHandler returns full event details for a comma-separated list
// of event IDs supplied by the client (e.g. from a locally-stored recently-
// viewed list). At most 20 IDs are accepted per request.
//
//	GET /v1/events/by-ids?ids=<uuid1>,<uuid2>,...
func GetEventsByIDsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := strings.TrimSpace(c.Query("ids"))
		if raw == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ids query parameter is required"})
			return
		}

		parts := strings.Split(raw, ",")
		ids := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				ids = append(ids, p)
			}
		}
		if len(ids) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no valid IDs provided"})
			return
		}
		if len(ids) > 20 {
			ids = ids[:20]
		}

		events, err := repository.GetEventsByIDs(pool, ids)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch events by IDs")
			return
		}

		c.JSON(http.StatusOK, gin.H{"data": events})
	}
}

// The event is created with status='draft' and must go through admin
// approval before it can be published.
//
//	POST /v1/organiser/events/create
func OrgCreateEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
 
		var dto models.CreateEventDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}
 
		if dto.EndTime.Before(dto.StartTime) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "End time must be after start time"})
			return
		}
 
		event, err := repository.CreateEvent(pool, &dto, userID.(string), models.EventDraft, false)
		if err != nil {
			utils.InternalError(c, err, "failed to create event")
			return
		}
 
		c.JSON(http.StatusCreated, gin.H{
			"message": "Event created as draft — pending admin approval before it can be published.",
			"event":   event,
		})
	}
}