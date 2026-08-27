package handlers

import (
	"errors"
	"net/http"

	"events-db/internal/cache"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Catalogue ────────────────────────────────────────────────────────────────

// GetEventInfoCatalogueHandler lists the information fields an organiser may
// choose from.
//
//	GET /v1/organiser/event-info/catalogue
//
// Served from the database rather than hardcoded in the dashboard so that
// deactivating a field, changing a label or adding a new one takes effect
// without a frontend deploy. The response carries value_type, enum_options,
// hint and review_required_when_published, which is everything the form needs
// to render itself — including WHY a field is locked on a live event, so the
// dashboard can explain rather than just disable.
func GetEventInfoCatalogueHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		entries, err := repository.GetInfoCatalogue(pool, false)
		if err != nil {
			utils.InternalError(c, err, "failed to load information catalogue")
			return
		}
		c.JSON(http.StatusOK, gin.H{"catalogue": entries})
	}
}

// ── Read ─────────────────────────────────────────────────────────────────────

// GetEventInfoHandler returns an event's information block to its organiser.
//
//	GET /v1/organiser/events/:id/info
//
// Uses the FULL catalogue including inactive entries, because an event created
// before a field was retired must still show its stored value — otherwise the
// organiser watches information disappear from their own event with no
// explanation.
func GetEventInfoHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		eventID := c.Param("id")

		editCtx, err := repository.GetEventEditContext(pool, eventID, userID)
		if err != nil {
			if errors.Is(err, repository.ErrEventNotOwned) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
				return
			}
			utils.InternalError(c, err, "failed to load event")
			return
		}

		items, err := repository.GetEventInfoItems(pool, eventID)
		if err != nil {
			utils.InternalError(c, err, "failed to load event information")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"items": items,
			// The client needs this to decide whether to lock the reviewed
			// fields. Sent explicitly rather than left for the dashboard to
			// infer from status, so the two cannot disagree.
			"is_published": editCtx.IsPublished,
			"status":       editCtx.Status,
		})
	}
}

// ── Write ────────────────────────────────────────────────────────────────────

// ReplaceEventInfoHandler replaces an event's whole information block.
//
//	PUT /v1/organiser/events/:id/info
//
// ── What can be changed on a LIVE event ─────────────────────────────────────
//
// Almost everything, directly and immediately. Parking arrangements, prohibited
// items, transport notes and accessibility details CHANGE in the days before an
// event, and routing "the north gate is now closed" through an admin queue
// would mean the accurate version arrives after the event has happened.
//
// The exception is any catalogue field carrying
// review_required_when_published — today only age_requirement. Someone who
// bought a ticket to an "all ages" event and arrives with a 15-year-old to an
// 18+ door has a refund dispute, and an organiser should not be able to create
// that silently after the sale. Those changes go through the existing
// event_edit request flow instead, and this endpoint returns 409 pointing at
// it.
//
// Re-submitting a locked field UNCHANGED is fine. The form posts the whole
// block on every save, so treating an untouched age restriction as an attempted
// change would block the organiser from editing anything at all on a live
// event.
func ReplaceEventInfoHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		eventID := c.Param("id")

		var dto models.ReplaceEventInfoDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": utils.FormatValidationErrors(err)})
			return
		}

		editCtx, err := repository.GetEventEditContext(pool, eventID, userID)
		if err != nil {
			if errors.Is(err, repository.ErrEventNotOwned) {
				c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
				return
			}
			utils.InternalError(c, err, "failed to load event")
			return
		}

		// A cancelled or completed event is history. Editing its stated terms
		// after the fact changes what people can be shown to have agreed to.
		if editCtx.Status == models.EventCancelled || editCtx.Status == models.EventCompleted {
			c.JSON(http.StatusConflict, gin.H{
				"error": "this event has ended or been cancelled and its information can no longer be changed",
				"code":  "event_closed",
			})
			return
		}

		catalogue, err := repository.GetInfoCatalogueMap(pool)
		if err != nil {
			utils.InternalError(c, err, "failed to load information catalogue")
			return
		}

		// Current values, needed so an unchanged locked field passes.
		current, err := repository.GetEventInfoItems(pool, eventID)
		if err != nil {
			utils.InternalError(c, err, "failed to load current information")
			return
		}

		items, err := models.ValidateEventInfoItems(
			dto.Items, catalogue, editCtx.IsPublished, models.InfoItemsByKey(current),
		)
		if err != nil {
			// A review-lock failure is a 409, not a 400: the request is
			// well-formed and the organiser is entitled to make the change —
			// just not through this door. The code and hint tell the dashboard
			// which door to open.
			if errors.Is(err, models.ErrInfoReviewRequired) {
				c.JSON(http.StatusConflict, gin.H{
					"error": err.Error(),
					"code":  "review_required",
					"hint":  "Submit this change via POST /organiser/events/:id/edit-request for admin approval.",
				})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := repository.ReplaceEventInfoItems(pool, eventID, items); err != nil {
			utils.InternalError(c, err, "failed to save event information")
			return
		}

		// The detail payload carries this block, so a stale cache would show
		// the old parking note for the cache lifetime — which is precisely the
		// information most likely to be corrected at the last minute.
		if ec != nil && editCtx.IsPublished {
			ec.BustOnPublish(c.Request.Context())
		}

		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}