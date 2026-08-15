package handlers

import (
	"context"
	"net/http"
	"time"

	"events-db/internal/middleware"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Grants to view an organiser's ID document.
//
// Editors and admins ask; a master admin approves. Each grant covers ONE
// organiser and expires, so investigating one dispute does not open every other
// organiser's document, and revoking access never means demoting anyone.

// POST /admin/kyc/:id/request-access  { "reason": "..." }
func RequestDocumentAccessHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID := c.MustGet("user_id").(string)
		organizerID := c.Param("id")

		var body struct {
			Reason string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || len(trimmed(body.Reason)) < 10 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "a reason of at least 10 characters is required — a master admin has to judge this",
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var id string
		err := pool.QueryRow(ctx, `
			INSERT INTO kyc_document_grants (organizer_id, granted_to, reason)
			VALUES ($1, $2, $3)
			RETURNING id::text
		`, organizerID, actorID, body.Reason).Scan(&id)
		if err != nil {
			utils.InternalError(c, err, "could not log the request")
			return
		}

		middleware.AuditAllowed(pool, c, "kyc.access_requested", "organizer", organizerID,
			map[string]any{"grant_id": id})

		c.JSON(http.StatusCreated, gin.H{
			"id":     id,
			"status": "pending",
			"note":   "A master admin must approve this before you can view the document.",
		})
	}
}

// GET /admin/kyc/grants — master admin only.
func ListDocumentGrantsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		rows, err := pool.Query(ctx, `
			SELECT g.id::text, g.organizer_id::text, o.business_name, u.email,
			       g.reason, g.requested_at, g.approved_at, g.expires_at, g.denied_at
			FROM kyc_document_grants g
			JOIN users u ON u.id = g.granted_to
			LEFT JOIN organizers o ON o.id = g.organizer_id
			WHERE g.revoked_at IS NULL
			ORDER BY g.requested_at DESC
			LIMIT 100
		`)
		if err != nil {
			utils.InternalError(c, err, "could not list grants")
			return
		}
		defer rows.Close()

		type item struct {
			ID           string     `json:"id"`
			OrganizerID  string     `json:"organizer_id"`
			BusinessName *string    `json:"business_name"`
			RequestedBy  string     `json:"requested_by"`
			Reason       string     `json:"reason"`
			RequestedAt  time.Time  `json:"requested_at"`
			ApprovedAt   *time.Time `json:"approved_at"`
			ExpiresAt    *time.Time `json:"expires_at"`
			DeniedAt     *time.Time `json:"denied_at"`
		}
		out := []item{}
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.ID, &it.OrganizerID, &it.BusinessName, &it.RequestedBy,
				&it.Reason, &it.RequestedAt, &it.ApprovedAt, &it.ExpiresAt, &it.DeniedAt); err != nil {
				utils.InternalError(c, err, "could not read grants")
				return
			}
			out = append(out, it)
		}
		c.JSON(http.StatusOK, gin.H{"grants": out})
	}
}

// POST /admin/kyc/grants/:id/approve  { "hours": 24 } — master admin only.
func ApproveDocumentGrantHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	const defaultHours, maxHours = 24, 168

	return func(c *gin.Context) {
		actorID := c.MustGet("user_id").(string)

		var body struct {
			Hours int `json:"hours"`
		}
		_ = c.ShouldBindJSON(&body)
		hours := body.Hours
		if hours <= 0 {
			hours = defaultHours
		}
		if hours > maxHours {
			hours = maxHours
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// granted_to <> $2 mirrors the CHECK constraint: a master admin has no
		// use for a grant to themselves and should not be able to mint one.
		tag, err := pool.Exec(ctx, `
			UPDATE kyc_document_grants
			SET approved_by = $2, approved_at = now(),
			    expires_at = now() + ($3 || ' hours')::interval
			WHERE id = $1
			  AND approved_at IS NULL AND denied_at IS NULL AND revoked_at IS NULL
			  AND granted_to <> $2
		`, c.Param("id"), actorID, hours)
		if err != nil {
			utils.InternalError(c, err, "could not approve the grant")
			return
		}
		if tag.RowsAffected() == 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "request not found, already actioned, or your own"})
			return
		}

		middleware.AuditAllowed(pool, c, "kyc.access_granted", "kyc_grant", c.Param("id"),
			map[string]any{"hours": hours})

		c.JSON(http.StatusOK, gin.H{"status": "approved", "expires_in_hours": hours})
	}
}

// POST /admin/kyc/grants/:id/deny — master admin only. Also used to revoke an
// already-approved grant, since both mean "this access ends now".
func DenyDocumentGrantHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID := c.MustGet("user_id").(string)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		tag, err := pool.Exec(ctx, `
			UPDATE kyc_document_grants
			SET denied_by = COALESCE(denied_by, $2),
			    denied_at = COALESCE(denied_at, now()),
			    revoked_at = now()
			WHERE id = $1 AND revoked_at IS NULL
		`, c.Param("id"), actorID)
		if err != nil {
			utils.InternalError(c, err, "could not deny the grant")
			return
		}
		if tag.RowsAffected() == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
			return
		}

		middleware.AuditAllowed(pool, c, "kyc.access_denied", "kyc_grant", c.Param("id"), nil)
		c.JSON(http.StatusOK, gin.H{"status": "denied"})
	}
}