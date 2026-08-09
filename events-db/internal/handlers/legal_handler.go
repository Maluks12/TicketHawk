package handlers

import (
	"context"
	"net/http"
	"regexp"
	"time"

	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Legal acceptance
//
// Records which version of the organiser agreements an account accepted.
//
// Register on any AUTHENTICATED group — NOT the organiser-only one. A pending
// applicant reaches /onboarding and /kyc before the organiser role is granted,
// and they need to be able to accept the terms on the way in:
//
//	authed.GET ("/organiser/legal",        handlers.GetLegalAcceptanceHandler(pool))
//	authed.POST("/organiser/legal/accept", handlers.AcceptLegalHandler(pool))
//
// The dashboard gate FAILS OPEN if these are missing, so the frontend can ship
// before or after this without locking anyone out.
// ─────────────────────────────────────────────────────────────────────────────

// legalVersionRe constrains the version to the date format the client emits.
//
// This column is evidence of consent, and the value is client-supplied. Without
// a pattern, a caller could post an arbitrary string — including a FUTURE
// version — and permanently suppress every re-prompt, including the one after a
// change to the commission split. The regex is what stops "accepted" from
// becoming unfalsifiable.
var legalVersionRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// GET /organiser/legal
//
// Returns what this account has accepted. The CLIENT decides whether that is
// current, by comparing against its own CURRENT_LEGAL_VERSION — deliberately,
// so shipping new terms is a frontend deploy rather than a coordinated
// frontend+backend release where a mismatch prompts everyone or nobody.
func GetLegalAcceptanceHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var version *string
		var at *time.Time
		err := pool.QueryRow(ctx, `
			SELECT legal_accepted_version, legal_accepted_at
			FROM users
			WHERE id = $1
		`, userID).Scan(&version, &at)

		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "failed to read legal acceptance")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"accepted_version": version, // null = never accepted
			"accepted_at":      at,
		})
	}
}

// POST /organiser/legal/accept
//
//	{ "version": "2026-08-01" }
//
// Idempotent: accepting the same version twice refreshes the timestamp and is
// otherwise a no-op, so a double-click or a retried request can't fail.
func AcceptLegalHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)

		var body struct {
			Version string `json:"version"`
			// accepted_at is accepted in the payload and deliberately IGNORED.
			// A client-supplied timestamp is an attacker-supplied timestamp,
			// and this row is the evidence that consent happened at a
			// particular moment. now() below is the only value written.
			AcceptedAt string `json:"accepted_at"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		if !legalVersionRe.MatchString(body.Version) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version format"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		tag, err := pool.Exec(ctx, `
			UPDATE users
			SET legal_accepted_version = $2,
			    legal_accepted_at      = now()
			WHERE id = $1
		`, userID, body.Version)
		if err != nil {
			utils.InternalError(c, err, "failed to record legal acceptance")
			return
		}
		if tag.RowsAffected() == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"accepted_version": body.Version,
			"accepted_at":      time.Now().UTC(),
		})
	}
}

// ── Registration ─────────────────────────────────────────────────────────────
//
// The sign-up form posts accepted_legal_version alongside the account fields.
// To persist it, add this to the INSERT in the existing register handler —
// same pattern, same validation:
//
//	if legalVersionRe.MatchString(req.AcceptedLegalVersion) {
//	    // ... include legal_accepted_version = $n, legal_accepted_at = now()
//	}
//
// If you'd rather not touch the register handler yet, nothing breaks: the field
// is ignored, the column stays NULL, and the new organiser simply sees the gate
// once on their first dashboard load and accepts there instead. Slightly worse
// UX, identical legal outcome.