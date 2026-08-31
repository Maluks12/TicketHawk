package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"events-db/internal/cache"
	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ─────────────────────────────────────────────────────────────────────────────
// PATCH /organiser/events/:id/ticket-limit
//
// Edits events.max_tickets_per_user (migration 000043) on its own, so the
// number can be changed from the event detail page without the organiser
// re-opening the create wizard and re-submitting every other field with it.
//
// The existing routes cannot do this:
//
//	PUT  /organiser/events/:id            draft only, and a FULL CreateEventDTO
//	                                      replace — sending it to change one
//	                                      integer means every other column is
//	                                      rewritten from whatever the caller
//	                                      happened to have loaded
//	POST /organiser/events/:id/edit-request
//	                                      published only, and max_tickets_user
//	                                      is not in editableWhenPublished, so
//	                                      it comes back in rejected_fields
//
// Lifecycle split is the same one the rest of organiser editing uses: a draft
// has no buyers and is not discoverable, so it is written directly; a live
// event has people who paid against its current terms, so the change is filed
// as an event_edit staff_request and applied by applyEventEdit on approval.
// The wire key there is "max_tickets_user" — see editableEventFields in
// request_execute_handler.go, where that name is mapped to the real column.
// ─────────────────────────────────────────────────────────────────────────────

// Mirrors the CHECK on events.max_tickets_per_user from migration 000043.
//
// Enforced here rather than left to the constraint because a violation on the
// review path does not surface to the organiser at all: the request is filed
// happily, sits in the admin queue, and only dies at execution with a 23514
// long after anyone can connect it to what was typed.
const (
	minTicketsPerUser = 1
	maxTicketsPerUser = 10
)

type ticketLimitDTO struct {
	// Pointer, and absent is treated the same as explicit null: both mean
	// "no limit", which is what a blank input sends.
	MaxTicketsPerUser *int `json:"max_tickets_per_user"`

	// Optional. Only reaches staff_requests.reason on the review path, and
	// only when it satisfies the >= 10 character CHECK; otherwise the
	// generated sentence below is used, which is more use to an approver
	// than a one-word reason anyway.
	Reason string `json:"reason"`
}

func describeTicketLimit(v *int) string {
	if v == nil {
		return "no limit"
	}
	return fmt.Sprintf("%d per attendee", *v)
}

func UpdateEventTicketLimitHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)
		eventID := c.Param("id")
		ctx := c.Request.Context()

		var dto ticketLimitDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		if dto.MaxTicketsPerUser != nil {
			if n := *dto.MaxTicketsPerUser; n < minTicketsPerUser || n > maxTicketsPerUser {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf(
						"Max per attendee must be between %d and %d, or blank for no limit",
						minTicketsPerUser, maxTicketsPerUser),
				})
				return
			}
		}

		// organizer_id on events holds the USER id, which is why ownership is
		// checked against user_id directly rather than through organizers.
		var status string
		var current *int
		err := pool.QueryRow(ctx, `
			SELECT status::text, max_tickets_per_user
			FROM events
			WHERE id = $1 AND organizer_id = $2
		`, eventID, userID).Scan(&status, &current)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "failed to load event")
			return
		}

		// A no-op must not reach the review queue. Re-saving the same number
		// is an easy thing to do from a form, and an admin opening a request
		// whose diff says 4 -> 4 has been given work that means nothing.
		unchanged := (current == nil && dto.MaxTicketsPerUser == nil) ||
			(current != nil && dto.MaxTicketsPerUser != nil && *current == *dto.MaxTicketsPerUser)
		if unchanged {
			c.JSON(http.StatusOK, gin.H{
				"applied":              true,
				"unchanged":            true,
				"max_tickets_per_user": dto.MaxTicketsPerUser,
			})
			return
		}

		switch models.EventStatus(status) {

		// ── Draft: straight through ──────────────────────────────────────────
		case models.EventDraft:
			// status stays in the WHERE clause rather than being trusted from
			// the read above, so a publish landing in between cannot be
			// overwritten by a write that believed it was still a draft.
			tag, uerr := pool.Exec(ctx, `
				UPDATE events
				SET max_tickets_per_user = $3,
				    updated_at           = NOW()
				WHERE id = $1 AND organizer_id = $2 AND status = 'draft'
			`, eventID, userID, dto.MaxTicketsPerUser)
			if uerr != nil {
				utils.InternalError(c, uerr, "failed to update ticket limit")
				return
			}
			if tag.RowsAffected() == 0 {
				c.JSON(http.StatusConflict, gin.H{
					"error": "This event is no longer a draft — the change has to go through review",
				})
				return
			}

			if ec != nil {
				ec.BustOnEventUpdate(ctx)
			}
			log.Info().
				Str("event_id", eventID).
				Str("organizer_id", userID).
				Str("from", describeTicketLimit(current)).
				Str("to", describeTicketLimit(dto.MaxTicketsPerUser)).
				Msg("organiser updated draft ticket limit")

			c.JSON(http.StatusOK, gin.H{
				"applied":              true,
				"max_tickets_per_user": dto.MaxTicketsPerUser,
			})

		// ── Live: filed for review ───────────────────────────────────────────
		case models.EventPublished, models.EventPublishedPrivate:
			reason := strings.TrimSpace(dto.Reason)
			if len(reason) < 10 {
				reason = fmt.Sprintf(
					"Organiser changed the per-attendee ticket limit from %s to %s",
					describeTicketLimit(current), describeTicketLimit(dto.MaxTicketsPerUser))
			}

			payload, merr := json.Marshal(map[string]any{
				"changes": map[string]any{
					// Wire key, not the column name. applyEventEdit maps it.
					"max_tickets_user": dto.MaxTicketsPerUser,
				},
			})
			if merr != nil {
				utils.InternalError(c, merr, "failed to encode edit payload")
				return
			}

			// subject_required is set explicitly, NOT left to the column
			// default of TRUE. RequestPolicies[ReqEventEdit] has
			// SubjectMustConfirm false, and the execution CHECK constraint
			// refuses to run any row where subject_required is TRUE and
			// subject_confirmed_at is NULL — so a row that takes the default
			// reaches 'ready' on approval and then fails at execute time with
			// a constraint violation nobody can clear from the console.
			var requestID string
			ierr := pool.QueryRow(ctx, `
				INSERT INTO staff_requests
					(kind, status, requested_by, reason, target_event_id, payload, subject_required)
				VALUES ('event_edit', 'pending_approval', $1, $2, $3, $4, FALSE)
				RETURNING id
			`, userID, reason, eventID, payload).Scan(&requestID)
			if ierr != nil {
				utils.InternalError(c, ierr, "failed to file edit request")
				return
			}

			log.Info().
				Str("event_id", eventID).
				Str("organizer_id", userID).
				Str("request_id", requestID).
				Str("from", describeTicketLimit(current)).
				Str("to", describeTicketLimit(dto.MaxTicketsPerUser)).
				Msg("organiser filed ticket limit edit request")

			c.JSON(http.StatusAccepted, gin.H{
				"applied":    false,
				"request_id": requestID,
				"status":     "pending_approval",
				"message":    "Sent for review. Your event stays live and on sale until an admin approves the change.",
			})

		// ── Everything else ──────────────────────────────────────────────────
		default:
			// pending_review, rejected, cancelled, completed. The first two
			// match what PUT /organiser/events/:id already refuses; the last
			// two have no sensible meaning for a purchase limit.
			c.JSON(http.StatusConflict, gin.H{
				"error":  "The per-attendee limit cannot be changed while the event is in this state",
				"status": status,
			})
		}
	}
}