package handlers

import (
	"errors"
	"events-db/internal/cache"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateEventHandlers(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		roleVal, _ := c.Get("role")

		role, _ := roleVal.(string)
		if role != string(models.RoleOrganizer) && role != string(models.RoleAdmin) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Only organizers can create events",
				"note":  "Apply to become an organizer at /api/v1/user/apply-organizer",
			})
			return
		}

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

		if ec != nil {
			ec.BustOnPublish(c.Request.Context())
		}

		event, err := repository.CreateEvent(pool, &dto, userID.(string))
		if err != nil {
			utils.InternalError(c, err, "failed to create event")
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Event created as draft",
			"event":   event,
			"note":    "Publish the event to make it visible to attendees",
		})
	}
}

func DeleteEventHandlers(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		eventID := c.Param("id")

		err := repository.DeleteEvents(pool, eventID, userID.(string))
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
				return
			}
			utils.InternalError(c, err, "failed to delete event")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Event deleted successfully"})
	}
}

func TrendingEventHandlers(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
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

func UpdateEventHandlers(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
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

func GetEventByIDHandlers(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
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
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		c.JSON(http.StatusOK, event)
	}
}

// GetEventBySlugHandler fetches a single published event by its URL slug.
//
//	GET /events/:slug
//
// This is the primary lookup path for the public-facing event detail page.
// The slug is more stable than a UUID for sharing links because it is readable
// and survives copy-paste. The response is identical to GetEventByIDHandlers so
// frontend clients can use either interchangeably.
func GetEventBySlugHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")

		if slug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "slug is required"})
			return
		}

		event, err := repository.GetEventBySlug(pool, slug)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		// Increment view count — same behaviour as the UUID lookup.
		if ec != nil {
			ec.IncrementView(c.Request.Context(), event.ID)
		} else {
			_ = repository.IncrementViews(pool, event.ID)
		}

		c.JSON(http.StatusOK, event)
	}
}

func GetAllEventsHandlers(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
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

func FeaturedEventsHandlers(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
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

func ActiveEventsHandlers(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
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

func GetEventCategory(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if ec != nil {
			ec.BustOnPublish(c.Request.Context())
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Event published successfully",
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cancellation reason required"})
			return
		}

		err := repository.CancelEvent(pool, eventID, input.Reason, userID.(string))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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