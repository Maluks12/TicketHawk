package handlers

import (
	"context"
	"net/http"

	"events-db/internal/cache"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ════════════════════════════════════════════════════════════════════════════
// Admin: Event Approval
// ════════════════════════════════════════════════════════════════════════════

// AdminGetPendingEventsHandler returns all events awaiting admin review.
//
//	GET /admin/events/pending
func AdminGetPendingEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		events, err := repository.AdminGetPendingEvents(pool)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch pending events")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"events": events,
			"count":  len(events),
		})
	}
}

// AdminApproveEventHandler approves a pending_review event, making it publicly live.
// Busts the event feed cache so the newly published event appears immediately.
//
//	PUT /admin/events/:id/approve
func AdminApproveEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")

		if err := repository.AdminApproveEvent(pool, eventID); err != nil {
			if err == repository.ErrEventNotPublishable {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "Event not found or not in pending_review state",
				})
				return
			}
			utils.InternalError(c, err, "failed to approve event")
			return
		}

		if ec != nil {
			ec.BustOnPublish(context.Background())
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Event approved and published successfully",
			"event_id": eventID,
		})
	}
}

// AdminRejectEventHandler rejects a pending_review event with a reason.
// The organiser can see the reason in their dashboard and resubmit after revising.
//
//	PUT /admin/events/:id/reject
//	Body: { "reason": "Images are too low resolution" }
func AdminRejectEventHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")

		var input struct {
			Reason string `json:"reason" binding:"required,min=10"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "A rejection reason is required (min 10 characters)",
			})
			return
		}

		if err := repository.AdminRejectEvent(pool, eventID, input.Reason); err != nil {
			if err == repository.ErrEventNotPublishable {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "Event not found or not in pending_review state",
				})
				return
			}
			utils.InternalError(c, err, "failed to reject event")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Event rejected. The organiser has been notified and can revise and resubmit.",
			"event_id": eventID,
		})
	}
}

// AdminPromoteEventHandler places an event on the home feed as a free, admin-
// granted sponsored card. If the event is still pending review (or a draft), it
// is approved/published in the same action — so an admin can approve-and-promote
// a non-advertised event in one click. Busts the feed cache so it appears now.
//
//	PUT /admin/events/:id/promote
func AdminPromoteEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")

		if err := repository.AdminPromoteEvent(pool, eventID); err != nil {
			if err == repository.ErrEventNotPublishable {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "Event not found, or it is cancelled/completed and cannot be promoted",
				})
				return
			}
			utils.InternalError(c, err, "failed to promote event")
			return
		}

		if ec != nil {
			ec.BustOnPublish(context.Background())
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Event published and added to the home page",
			"event_id": eventID,
		})
	}
}

// AdminUnpromoteEventHandler removes an event's admin promotion. The event stays
// published; it just no longer appears organically on the home feed (for a free
// event with no listing fee paid).
//
//	PUT /admin/events/:id/unpromote
func AdminUnpromoteEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")

		if err := repository.AdminUnpromoteEvent(pool, eventID); err != nil {
			if err == repository.ErrEventNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
				return
			}
			utils.InternalError(c, err, "failed to unpromote event")
			return
		}

		if ec != nil {
			ec.BustOnPublish(context.Background())
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Event removed from the home page",
			"event_id": eventID,
		})
	}
}

// ════════════════════════════════════════════════════════════════════════════
// Admin: Merch Approval
// ════════════════════════════════════════════════════════════════════════════
// AdminGetPendingMerchHandler returns all merch items awaiting admin review.
//
//	GET /admin/merch/pending
func AdminGetPendingMerchHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		merch, err := repository.AdminGetPendingMerch(pool)
		if err != nil {
			utils.InternalError(c, err, "failed to fetch pending merch")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"merch": merch,
			"count": len(merch),
		})
	}
}

// AdminApproveMerchHandler approves a pending_review merch item.
// The item becomes immediately visible to buyers.
//
//	PUT /admin/merch/:id/approve
func AdminApproveMerchHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		merchID := c.Param("id")

		if err := repository.AdminApproveMerch(pool, merchID); err != nil {
			if err == repository.ErrMerchNotFound {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "Merch item not found or not in pending_review state",
				})
				return
			}
			utils.InternalError(c, err, "failed to approve merch")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Merch item approved and now visible to buyers",
			"merch_id": merchID,
		})
	}
}

// AdminRejectMerchHandler rejects a pending_review merch item with a reason.
// The organiser can edit the item and resubmit via POST /organiser/merch/:id/resubmit.
//
//	PUT /admin/merch/:id/reject
//	Body: { "reason": "Product images must show actual item clearly" }
func AdminRejectMerchHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		merchID := c.Param("id")

		var input struct {
			Reason string `json:"reason" binding:"required,min=10"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "A rejection reason is required (min 10 characters)",
			})
			return
		}

		if err := repository.AdminRejectMerch(pool, merchID, input.Reason); err != nil {
			if err == repository.ErrMerchNotFound {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "Merch item not found or not in pending_review state",
				})
				return
			}
			utils.InternalError(c, err, "failed to reject merch")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Merch item rejected. The organiser can edit and resubmit.",
			"merch_id": merchID,
		})
	}
}