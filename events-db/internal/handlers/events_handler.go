package handlers

import (
	"context"
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
	"time"

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

		// Same omission as the organiser path: dto.TicketTypes was accepted and
		// never written, so an admin creating a tiered event got a flat-priced one.
		// Not fatal — the event is valid without tiers — so this logs and continues.
		if len(dto.TicketTypes) > 0 {
			if _, ttErr := repository.CreateTicketTypes(pool, event.ID, dto.TicketTypes, userID.(string)); ttErr != nil {
				log.Error().Err(ttErr).Str("event_id", event.ID).
					Msg("admin create: event created but ticket types failed to persist")
			} else if tts, loadErr := repository.GetTicketTypesByEventID(pool, event.ID); loadErr == nil {
				event.TicketTypes = tts
				event.ComputeAvailableTickets()
			}
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
			} else {
				// The handover UPDATE ran after CreateEvent returned, so the
				// in-memory struct is stale. Mirror the DB so the response
				// doesn't contradict the handover block.
				if newOrg, ok := handover["new_organizer"].(gin.H); ok {
					if newID, ok := newOrg["user_id"].(string); ok {
						event.OrganizerID = newID
					}
				}
				event.ListingFeePaid = true
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
			// Same event, second counter: IncrementView feeds the lifetime
			// events.views that trending and the public page read, this one
			// feeds the dashboard's time series. Both are written from the
			// same request so they stay in agreement.
			ec.RecordViewBucket(c.Request.Context(), eventID)
		} else {
			_ = repository.IncrementViews(pool, eventID)
			_ = repository.RecordMetricsDirect(pool, eventID, 1, 0, 0)
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
			ec.RecordViewBucket(c.Request.Context(), event.ID)
		} else {
			_ = repository.IncrementViews(pool, event.ID)
			_ = repository.RecordMetricsDirect(pool, event.ID, 1, 0, 0)
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

		// Optional body: older drafts can accept the rights declaration here
		// instead of at creation. Absent/invalid body = not accepted.
		var body struct {
			RightsDeclarationAccepted bool `json:"rights_declaration_accepted"`
		}
		_ = c.ShouldBindJSON(&body)

		outcome, err := repository.PublishEvent(pool, eventID, userID.(string), body.RightsDeclarationAccepted)
		if err != nil {
			if errors.Is(err, repository.ErrEventNotPublishable) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Event not found or not in a submittable state (must be draft or rejected)"})
				return
			}
			if errors.Is(err, repository.ErrKYCRequired) {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "Identity verification is required before publishing a paid event.",
					"hint":  "Submit your ID via POST /organiser/kyc, then try again once verified.",
					"code":  "kyc_required",
				})
				return
			}
			if errors.Is(err, repository.ErrPayoutDetailsRequired) {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "Payout bank details are required before publishing a paid event, so we know where to send your money.",
					"hint":  "Add them via POST /organiser/payouts/details.",
					"code":  "payout_details_required",
				})
				return
			}
			if errors.Is(err, repository.ErrDeclarationRequired) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Please confirm you hold the rights to host this event and sell tickets for it.",
					"hint":  "Resend this request with {\"rights_declaration_accepted\": true}.",
					"code":  "declaration_required",
				})
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

		if outcome.LiveImmediately {
			c.JSON(http.StatusOK, gin.H{
				"message": "Your private event is live. It won't appear in the feed or search — share the link to invite people.",
				"slug":    outcome.Slug,
			})
			return
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

		if len([]rune(query)) < 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "search query must be at least 2 characters"})
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

func CancelEventHandler(pool *pgxpool.Pool, ec *cache.EventCache, refundService *services.RefundService) gin.HandlerFunc {
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

		// Actually move the money. CancelEvent above only flips statuses and
		// notifies; until this call existed the response below promised a refund
		// that never happened. Buyers are made whole INCLUDING the service fee —
		// they did not choose this.
		//
		// Runs with its own timeout rather than the request context: a
		// cancellation of a large event issues one gateway call per booking and
		// must not be killed halfway by a client disconnect, which would leave
		// some buyers repaid and others not.
		refundCtx, cancelRefunds := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancelRefunds()

		outcome, refundErr := refundService.RefundEventCancellation(refundCtx, eventID, input.Reason)
		if refundErr != nil {
			log.Error().Err(refundErr).Str("event_id", eventID).
				Msg("cancel event: cancelled but refunds could not be processed")
			c.JSON(http.StatusOK, gin.H{
				"message": "Event cancelled, but refunds could not be started",
				"note":    "Support has been alerted. Attendees have not been refunded yet.",
				"error":   "refund_processing_failed",
			})
			return
		}

		note := "All attendees have been refunded in full, including the service fee."
		if outcome.BookingsFailed > 0 {
			note = "Some refunds could not be completed and need manual review."
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Event cancelled successfully",
			"note":    note,
			"refunds": outcome,
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

		// Persist the pricing tiers submitted with the event.
		//
		// CreateEventDTO has carried a TicketTypes field for a long time and the
		// creation wizard has been sending it (body.ticket_types) — but nothing here
		// ever read it, so every tier an organiser defined was silently dropped. The
		// event came out flat-priced, the buyer got a single synthesised "General
		// Admission" row, and the organiser had no way to tell anything was wrong
		// because the request returned 201.
		//
		// Failure here does NOT roll back the event. The event is valid without
		// tiers (that is just flat pricing), and destroying a successfully created
		// event because one tier had a bad capacity would be worse than reporting
		// the problem. The response carries ticket_types_error so the client can
		// surface it, and the organiser can add the tiers from the event page.
		var ticketTypeErr string
		if len(dto.TicketTypes) > 0 {
			if _, ttErr := repository.CreateTicketTypes(pool, event.ID, dto.TicketTypes, userID.(string)); ttErr != nil {
				ticketTypeErr = ttErr.Error()
				log.Error().Err(ttErr).
					Str("event_id", event.ID).
					Int("tier_count", len(dto.TicketTypes)).
					Msg("event created but ticket types failed to persist")
			} else {
				// Reload so the response carries the tiers with their generated ids.
				if tts, loadErr := repository.GetTicketTypesByEventID(pool, event.ID); loadErr == nil {
					event.TicketTypes = tts
					event.ComputeAvailableTickets()
				}
			}
		}

		resp := gin.H{
			"message": "Event created as draft — pending admin approval before it can be published.",
			"event":   event,
		}
		if ticketTypeErr != "" {
			resp["ticket_types_error"] = ticketTypeErr
		}
		c.JSON(http.StatusCreated, resp)
	}
}