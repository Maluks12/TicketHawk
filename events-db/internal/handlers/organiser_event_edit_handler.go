package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"events-db/internal/cache"
	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ─────────────────────────────────────────────────────────────────────────────
// Organiser event editing
//
// Three routes, split by what the event's lifecycle allows:
//
//	DELETE /organiser/events/:id        draft only — hard delete
//	GET    /organiser/events/:id/edit   full event, shaped for the create form
//	PUT    /organiser/events/:id        draft only — direct save, no review
//	POST   /organiser/events/:id/edit-request
//	                                    published — files a staff_request for
//	                                    admin approval; the event stays live
//
// A draft has never been sold and is not discoverable, so nothing is at stake
// in changing it — the organiser owns it outright. A published event has
// buyers who paid against its current terms, so every change goes through the
// existing event_edit review path instead.
// ─────────────────────────────────────────────────────────────────────────────

// editableWhenPublished is what an organiser may propose on a LIVE event.
//
// Narrower than the admin console's allow-list on purpose. Date, venue and
// city are absent: moving an event in space or time is not an edit, it is a
// different event, and a buyer who can no longer attend needs a refund route
// rather than a silently amended listing. The TIME of day is editable because
// a start pushed by an hour is the common, genuine case.
//
// Values are the wire keys understood by applyEventEdit in
// request_execute_handler.go.
var editableWhenPublished = map[string]bool{
	"title":            true,
	"description":      true,
	"start_time":       true, // time-of-day only; the date is pinned below
	"end_time":         true,
	"tags":             true,
	"cover_image_url":  true,
	"banner_image_url": true,
	"ticket_types":     true,

	// The Information block (parking, accessibility, age requirement, …).
	//
	// Most information fields are editable on a live event WITHOUT review, and
	// go through PUT /organiser/events/:id/info directly — see
	// ReplaceEventInfoHandler for why: a "the north gate is now closed" update
	// that waits in an admin queue arrives after the event.
	//
	// This key exists for the fields the catalogue marks
	// review_required_when_published — today only age_requirement. That
	// endpoint refuses those with a 409 pointing here, so without this entry
	// the refusal would be a dead end: the organiser is told to file a request
	// and the request would then be rejected as an uneditable field.
	//
	// The payload is the WHOLE block, not a diff of it. See
	// applyEventInfoEdit.
	"event_info":       true,

	// Line-up — performers and teams, with images, stages and set times.
	//
	// The organiser's own route (PUT /organiser/events/:id/lineup) applies
	// directly and is NOT review-gated: an act drops out three days before a
	// festival and the replacement is announced the same afternoon, so a
	// line-up sitting in an admin queue is wrong on exactly the days most
	// people are looking at it.
	//
	// This key is for the OTHER route into the same data — an admin correcting
	// a line-up from the console files an event_edit like any other change.
	// Without it that request would execute with the line-up silently skipped
	// while reporting success.
	//
	// Payload is the whole list, not a diff. See applyEventLineupEdit.
	"lineup":           true,
}

// ── DELETE /organiser/events/:id ─────────────────────────────────────────────

// DeleteDraftEventHandler hard-deletes one of the organiser's own DRAFT events.
//
// Two independent guards, because "it's only a draft" is an assumption worth
// checking twice:
//
//   - status must be 'draft', enforced in the DELETE's WHERE clause so a
//     concurrent publish cannot slip between a read and the delete.
//   - bookings.event_id is ON DELETE RESTRICT, so Postgres refuses outright if
//     anything has ever been sold, whatever the status says. That is a
//     backstop, not the primary control.
//
// Ticket types, media, analytics rows and the like cascade automatically.
func DeleteDraftEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)
		eventID := c.Param("id")

		ctx := c.Request.Context()

		var status, title string
		err := pool.QueryRow(ctx,
			`SELECT status::text, title FROM events WHERE id = $1 AND organizer_id = $2`,
			eventID, userID,
		).Scan(&status, &title)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "failed to load event")
			return
		}

		if status != string(models.EventDraft) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Only draft events can be deleted. Cancel the event instead.",
			})
			return
		}

		tag, err := pool.Exec(ctx,
			`DELETE FROM events WHERE id = $1 AND organizer_id = $2 AND status = 'draft'`,
			eventID, userID,
		)
		if err != nil {
			// 23503 = foreign_key_violation: something still references this
			// event (a booking, a payout). Report it as a conflict rather than
			// a 500 — the request is well formed, the state forbids it.
			if strings.Contains(err.Error(), "23503") {
				c.JSON(http.StatusConflict, gin.H{
					"error": "This event has bookings or payouts attached and cannot be deleted.",
				})
				return
			}
			utils.InternalError(c, err, "failed to delete event")
			return
		}
		if tag.RowsAffected() == 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "Event is no longer a draft"})
			return
		}

		if ec != nil {
			// A deleted draft was never in the public feed, but the organiser's
			// own list views are cached alongside it.
			ec.BustOnEventUpdate(ctx)
		}
		log.Info().Str("event_id", eventID).Str("organizer_id", userID).
			Str("title", title).Msg("organiser deleted draft event")

		c.JSON(http.StatusOK, gin.H{"deleted": true, "id": eventID})
	}
}

// ── GET /organiser/events/:id/edit ───────────────────────────────────────────

// GetOrganiserEventForEditHandler returns one of the organiser's own events
// with every field the create form needs to hydrate itself, plus the tiers.
//
// Separate from the public event route because it must return DRAFTS, which
// the public route deliberately hides.
func GetOrganiserEventForEditHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)
		eventID := c.Param("id")
		ctx := c.Request.Context()

		var (
			ev struct {
				ID                string     `json:"id"`
				Title             string     `json:"title"`
				Description       string     `json:"description"`
				Status            string     `json:"status"`
				CoverImageURL     *string    `json:"cover_image_url"`
				BannerImageURL    *string    `json:"banner_image_url"`
				StartTime         time.Time  `json:"start_time"`
				EndTime           time.Time  `json:"end_time"`
				Timezone          string     `json:"timezone"`
				VenueName         string     `json:"venue_name"`
				Address           string     `json:"address"`
				City              string     `json:"city"`
				Latitude          *float64   `json:"latitude"`
				Longitude         *float64   `json:"longitude"`
				PlaceID           *string    `json:"place_id"`
				CategoryID        int        `json:"category_id"`
				Price             float64    `json:"price"`
				Currency          string     `json:"currency"`
				MaxCapacity       *int       `json:"max_capacity"`
				MaxTicketsPerUser *int       `json:"max_tickets_per_user"`
				AllowWaitlist     bool       `json:"allow_waitlist"`
				VenueType         *string    `json:"venue_type"`
				Tags              []string   `json:"tags"`
				Performers        []string   `json:"performers"`
				Teams             []string   `json:"teams"`
				Visibility        *string    `json:"visibility"`
				CreatedAt         time.Time  `json:"created_at"`
				UpdatedAt         *time.Time `json:"updated_at"`
			}
		)

		err := pool.QueryRow(ctx, `
			SELECT id, title, description, status::text,
			       cover_image_url, banner_image_url,
			       start_time, end_time, timezone,
			       COALESCE(venue_name,''), COALESCE(address,''), COALESCE(city,''),
			       latitude, longitude, place_id,
			       category_id, price, currency,
			       max_capacity, max_tickets_per_user, COALESCE(allow_waitlist,false),
			       venue_type,
			       COALESCE(tags, '{}'), COALESCE(performers, '{}'), COALESCE(teams, '{}'),
			       visibility, created_at, updated_at
			FROM events
			WHERE id = $1 AND organizer_id = $2
		`, eventID, userID).Scan(
			&ev.ID, &ev.Title, &ev.Description, &ev.Status,
			&ev.CoverImageURL, &ev.BannerImageURL,
			&ev.StartTime, &ev.EndTime, &ev.Timezone,
			&ev.VenueName, &ev.Address, &ev.City,
			&ev.Latitude, &ev.Longitude, &ev.PlaceID,
			&ev.CategoryID, &ev.Price, &ev.Currency,
			&ev.MaxCapacity, &ev.MaxTicketsPerUser, &ev.AllowWaitlist,
			&ev.VenueType,
			&ev.Tags, &ev.Performers, &ev.Teams,
			&ev.Visibility, &ev.CreatedAt, &ev.UpdatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "failed to load event")
			return
		}

		tiers, err := loadTicketTypesForEdit(c, pool, eventID)
		if err != nil {
			utils.InternalError(c, err, "failed to load ticket types")
			return
		}

		// Tells the client which fields to enable. A draft is fully editable;
		// a published event is restricted to the review-able subset.
		editable := make([]string, 0, len(editableWhenPublished))
		if ev.Status == string(models.EventDraft) {
			editable = append(editable, "*")
		} else {
			for k := range editableWhenPublished {
				editable = append(editable, k)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"event":           ev,
			"ticket_types":    tiers,
			"editable_fields": editable,
			"requires_review": ev.Status != string(models.EventDraft),
		})
	}
}

type ticketTypeEditRow struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  *string  `json:"description"`
	Price        float64  `json:"price"`
	Capacity     *int     `json:"capacity"`
	TicketsSold  int      `json:"tickets_sold"`
	Reserved     int      `json:"reserved_count"`
	SortOrder    int      `json:"sort_order"`
	IsVisible    bool     `json:"is_visible"`
	MinCapacity  int      `json:"min_capacity"` // sold + reserved: the floor
}

func loadTicketTypesForEdit(c *gin.Context, pool *pgxpool.Pool, eventID string) ([]ticketTypeEditRow, error) {
	rows, err := pool.Query(c.Request.Context(), `
		SELECT id, name, description, price, capacity,
		       tickets_sold, reserved_count, sort_order, is_visible
		FROM ticket_types
		WHERE event_id = $1
		ORDER BY sort_order, created_at
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ticketTypeEditRow, 0)
	for rows.Next() {
		var t ticketTypeEditRow
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Price, &t.Capacity,
			&t.TicketsSold, &t.Reserved, &t.SortOrder, &t.IsVisible); err != nil {
			return nil, err
		}
		t.MinCapacity = t.TicketsSold + t.Reserved
		out = append(out, t)
	}
	return out, rows.Err()
}

// ── POST /organiser/events/:id/edit-request ──────────────────────────────────

type organiserEditRequestDTO struct {
	// Reason is shown to the approving admin. staff_requests enforces a
	// meaningful length at the DB level, so a blank one is rejected here with
	// a readable message rather than a constraint violation.
	Reason  string         `json:"reason" binding:"required,min=10"`
	Changes map[string]any `json:"changes" binding:"required"`
}

// SubmitOrganiserEventEditHandler files an event_edit staff_request for a
// PUBLISHED event owned by the caller.
//
// The event keeps selling throughout. Nothing is written to `events` here —
// the diff sits in the request row and only lands when an admin approves it,
// at which point applyEventEdit runs it inside the approval transaction.
func SubmitOrganiserEventEditHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)
		eventID := c.Param("id")
		ctx := c.Request.Context()

		var dto organiserEditRequestDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		var status string
		var currentStart time.Time
		err := pool.QueryRow(ctx,
			`SELECT status::text, start_time FROM events WHERE id = $1 AND organizer_id = $2`,
			eventID, userID,
		).Scan(&status, &currentStart)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "failed to load event")
			return
		}
		if status == string(models.EventDraft) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Drafts are edited directly — no review needed",
			})
			return
		}

		// Drop anything not organiser-editable rather than filing it. An admin
		// approving a diff should not be shown a field the organiser was never
		// allowed to propose.
		clean := make(map[string]any, len(dto.Changes))
		rejected := make([]string, 0)
		for k, v := range dto.Changes {
			if editableWhenPublished[k] {
				clean[k] = v
			} else {
				rejected = append(rejected, k)
			}
		}
		if len(clean) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":            "No editable fields in this request",
				"rejected_fields":  rejected,
			})
			return
		}

		// Date is pinned: only the clock time may move. A changed date is a
		// different event to everyone holding a ticket, so it belongs in a
		// cancel-and-rebook flow rather than an edit.
		if raw, ok := clean["start_time"]; ok {
			newStart, perr := parseEditTime(raw)
			if perr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "start_time is not a valid timestamp"})
				return
			}
			if !sameCalendarDay(newStart, currentStart) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "The event date cannot be changed here — only the time of day. Cancel and re-list if the date must move.",
				})
				return
			}
		}

		payload, err := json.Marshal(map[string]any{"changes": clean})
		if err != nil {
			utils.InternalError(c, err, "failed to encode edit payload")
			return
		}

		var requestID string
		err = pool.QueryRow(ctx, `
			INSERT INTO staff_requests (kind, status, requested_by, reason, target_event_id, payload)
			VALUES ('event_edit', 'pending_approval', $1, $2, $3, $4)
			RETURNING id
		`, userID, dto.Reason, eventID, payload).Scan(&requestID)
		if err != nil {
			utils.InternalError(c, err, "failed to file edit request")
			return
		}

		log.Info().
			Str("event_id", eventID).
			Str("organizer_id", userID).
			Str("request_id", requestID).
			Int("field_count", len(clean)).
			Msg("organiser filed event edit request")

		c.JSON(http.StatusAccepted, gin.H{
			"request_id":      requestID,
			"status":          "pending_approval",
			"fields":          keysOf(clean),
			"rejected_fields": rejected,
			"message":         "Sent for review. Your event stays live and on sale until an admin approves the change.",
		})
	}
}

// parseEditTime accepts the RFC3339 string a JSON body carries.
func parseEditTime(raw any) (time.Time, error) {
	s, ok := raw.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("not a string")
	}
	return time.Parse(time.RFC3339, s)
}

// sameCalendarDay compares in the event's own offset rather than UTC, so an
// evening event near midnight SAST is not treated as a date change.
func sameCalendarDay(a, b time.Time) bool {
	loc := b.Location()
	ay, am, ad := a.In(loc).Date()
	by, bm, bd := b.In(loc).Date()
	return ay == by && am == bm && ad == bd
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ── PUT /organiser/events/:id ────────────────────────────────────────────────

// UpdateDraftEventHandler saves edits to a DRAFT event directly, no review.
//
// Reuses CreateEventDTO so the create form can post the identical body it
// already builds — one payload shape for both create and edit, which is the
// whole reason the form can be reused as an editor.
//
// Ticket tiers are NOT handled here. The form posts them separately to the
// existing PUT /organiser/events/:id/ticket-types, which already knows how to
// refuse a tier that has sales behind it.
//
// status is deliberately absent from the SET list: publishing is its own
// action with its own rights declaration, and an edit must not smuggle a
// draft into the public feed.
func UpdateDraftEventHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)
		eventID := c.Param("id")
		ctx := c.Request.Context()

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

		// status='draft' lives in the WHERE clause, not in a prior read, so a
		// publish landing between check and write cannot be overwritten.
		tag, err := pool.Exec(ctx, `
			UPDATE events SET
				title                = $3,
				description          = $4,
				cover_image_url      = $5,
				banner_image_url     = $6,
				start_time           = $7,
				end_time             = $8,
				timezone             = $9,
				venue_name           = $10,
				address              = $11,
				city                 = $12,
				latitude             = $13,
				longitude            = $14,
				place_id             = $15,
				category_id          = $16,
				price                = $17,
				currency             = $18,
				max_capacity         = $19,
				max_tickets_per_user = $20,
				allow_waitlist       = $21,
				venue_type           = COALESCE($22, venue_type),
				tags                 = $23,
				performers           = $24,
				teams                = $25,
				updated_at           = NOW()
			WHERE id = $1 AND organizer_id = $2 AND status = 'draft'
		`,
			eventID, userID,
			dto.Title, dto.Description,
			dto.CoverImageURL, dto.BannerImageURL,
			dto.StartTime, dto.EndTime, dto.Timezone,
			dto.VenueName, dto.Address, dto.City,
			dto.Latitude, dto.Longitude, dto.PlaceID,
			dto.CategoryID, dto.Price, dto.Currency,
			dto.MaxCapacity, dto.MaxTicketsPerUser, dto.AllowWaitlist,
			dto.VenueType,
			dto.Tags, dto.Performers, dto.Teams,
		)
		if err != nil {
			utils.InternalError(c, err, "failed to update event")
			return
		}
		if tag.RowsAffected() == 0 {
			// Either not theirs, or no longer a draft. Both are a 409 rather
			// than a 404: telling the organiser "not found" for their own
			// event that just got published would be misleading.
			c.JSON(http.StatusConflict, gin.H{
				"error": "Event not found, or it is no longer a draft and must be edited through review",
			})
			return
		}

		if ec != nil {
			ec.BustOnEventUpdate(ctx)
		}
		log.Info().Str("event_id", eventID).Str("organizer_id", userID).
			Msg("organiser updated draft event")

		c.JSON(http.StatusOK, gin.H{"updated": true, "id": eventID})
	}
}