package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ─────────────────────────────────────────────────────────────────────────────
// Identity document access — master admin only
//
// This is the answer to "where do I find the ID images". Nowhere in the ordinary
// console: no staff role can reach them, including admin. They come out here,
// and only here.
//
// The three conditions, all mandatory:
//
//   1. users.is_master_admin — a flag set by hand in the database, orthogonal to
//      console_role, so growing the admin team never grants document access.
//   2. A stated reason, at least 10 characters, enforced by a CHECK on
//      kyc_document_access_log. A read with no purpose is precisely the read
//      worth asking about later.
//   3. A category, so the log is aggregatable — "how many regulator requests
//      last quarter" should be answerable without reading free text.
//
// The access is logged BEFORE the bytes are returned. If the log write fails,
// the read does not happen. That ordering is the whole point: an unlogged look
// at someone's identity document is the thing this exists to prevent, so the
// log is a precondition rather than a side effect.
// ─────────────────────────────────────────────────────────────────────────────

// RequireMasterAdmin gates the routes below.
//
// Separate from the staff middleware on purpose — it reads a different column
// and answers a different question. A master admin who is not console staff
// still passes here; a console admin who is not a master admin does not.
func RequireMasterAdmin(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		var isMaster bool
		var email string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(is_master_admin, FALSE), email FROM users WHERE id = $1`,
			userID,
		).Scan(&isMaster, &email)
		if err != nil {
			// Fail closed. A database hiccup must not hand out identity
			// documents.
			log.Error().Err(err).Msg("master admin check failed")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "could not verify access"})
			return
		}
		if !isMaster {
			// Logged as a denial in the document access log, not just the staff
			// log: an attempt to reach identity documents belongs in the record
			// that a regulator would read.
			logDocAccess(pool, c, nil, email, "view_denied",
				"Attempted access without master admin rights", nil)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "identity documents require master admin access",
			})
			return
		}

		c.Set("actor_email", email)
		c.Next()
	}
}

// POST /admin/kyc/:id/document
//
//	{ "reason": "…", "category": "regulator_request" }
//
// A POST, not a GET, deliberately: the reason is required, and a body carries it
// better than a query string that would end up in access logs and browser
// history alongside the document id.
func GetIdentityDocumentHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	validCategories := map[string]bool{
		"dispute":             true,
		"regulator_request":   true,
		"fraud_investigation": true,
		"other":               true,
	}

	return func(c *gin.Context) {
		organizerID := c.Param("id")
		email, _ := c.Get("actor_email")
		emailStr, _ := email.(string)

		var body struct {
			Reason   string `json:"reason"`
			Category string `json:"category"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		if len(trimmed(body.Reason)) < 10 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "a reason of at least 10 characters is required and is recorded permanently",
			})
			return
		}
		if !validCategories[body.Category] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "category must be dispute, regulator_request, fraud_investigation or other",
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()

		// Where the document lives depends on whether the export has run.
		// id_document_key is set once it is in the external store; until then
		// the bytes are still in id_document (see migration 000070).
		var docKey, docStore, mime *string
		var hasInline bool
		err := pool.QueryRow(ctx, `
			SELECT id_document_key, id_document_store, id_document_mime,
			       (id_document IS NOT NULL)
			FROM organizers
			WHERE id = $1
		`, organizerID).Scan(&docKey, &docStore, &mime, &hasInline)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "organiser not found"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "could not locate document")
			return
		}
		if docKey == nil && !hasInline {
			c.JSON(http.StatusNotFound, gin.H{"error": "no identity document on file"})
			return
		}

		// LOG FIRST. If this fails, the read does not happen — an unlogged look
		// at an identity document is the exact thing this endpoint exists to
		// prevent, so the log is a precondition, not a side effect.
		if err := logDocAccessSync(ctx, pool, c, &organizerID, emailStr,
			"view", body.Reason, body.Category, docKey); err != nil {
			utils.InternalError(c, err, "could not record access — read refused")
			return
		}

		if docKey != nil {
			// Exported: the console never proxies the bytes. It returns a
			// short-lived signed URL so the object store applies its own access
			// policy and writes its own log, independently of this application.
			// A store-side log survives a compromise of this one.
			c.JSON(http.StatusOK, gin.H{
				"location":   "external",
				"store":      docStore,
				"key":        docKey,
				"note":       "Generate a short-lived signed URL from the document store using this key.",
				"expires_in": 300,
			})
			return
		}

		// Not yet exported. The bytes are still in the operational database —
		// which is the situation migration 000070 exists to end. Served, with
		// the state made explicit so it is visible rather than silent.
		var data []byte
		if err := pool.QueryRow(ctx,
			`SELECT id_document FROM organizers WHERE id = $1`, organizerID,
		).Scan(&data); err != nil {
			utils.InternalError(c, err, "could not read document")
			return
		}

		contentType := "image/jpeg"
		if mime != nil && *mime != "" {
			contentType = *mime
		}
		// inline, not attachment: the reviewer looks at it, they do not need a
		// copy of someone's ID sitting in their Downloads folder.
		c.Header("Content-Disposition", "inline")
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
		c.Header("X-Document-Location", "operational-database-pending-export")
		c.Data(http.StatusOK, contentType, data)
	}
}

// GET /admin/kyc/access-log — master admin only.
//
// Who looked at what, and why. A log nobody can read is a log nobody checks.
func GetDocumentAccessLogHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		rows, err := pool.Query(ctx, `
			SELECT at, actor_email, action, reason, organizer_id::text, document_key, ip::text
			FROM kyc_document_access_log
			ORDER BY at DESC
			LIMIT 200
		`)
		if err != nil {
			utils.InternalError(c, err, "could not load access log")
			return
		}
		defer rows.Close()

		type entry struct {
			At          time.Time `json:"at"`
			ActorEmail  string    `json:"actor_email"`
			Action      string    `json:"action"`
			Reason      string    `json:"reason"`
			OrganizerID *string   `json:"organizer_id"`
			DocumentKey *string   `json:"document_key"`
			IP          *string   `json:"ip"`
		}
		out := []entry{}
		for rows.Next() {
			var e entry
			if err := rows.Scan(&e.At, &e.ActorEmail, &e.Action, &e.Reason,
				&e.OrganizerID, &e.DocumentKey, &e.IP); err != nil {
				utils.InternalError(c, err, "could not read access log")
				return
			}
			out = append(out, e)
		}
		c.JSON(http.StatusOK, gin.H{"entries": out})
	}
}

// ── Logging ──────────────────────────────────────────────────────────────────

// logDocAccessSync writes the access record and returns the error, so a caller
// can refuse to proceed when it fails.
func logDocAccessSync(
	ctx context.Context, pool *pgxpool.Pool, c *gin.Context,
	organizerID *string, email, action, reason, category string, key *string,
) error {
	full := reason
	if category != "" {
		full = "[" + category + "] " + reason
	}
	userID, _ := c.Get("user_id")
	_, err := pool.Exec(ctx, `
		INSERT INTO kyc_document_access_log
			(organizer_id, actor_id, actor_email, action, reason, document_key, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, organizerID, userID, email, action, full, key, c.ClientIP(), c.Request.UserAgent())
	return err
}

// logDocAccess is the fire-and-forget variant, for denials — the request is
// already being refused, so a slow log must not hold it open.
func logDocAccess(
	pool *pgxpool.Pool, c *gin.Context,
	organizerID *string, email, action, reason string, key *string,
) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := logDocAccessSync(ctx, pool, c, organizerID, email, action, reason, "", key); err != nil {
			log.Warn().Err(err).Msg("kyc access log write failed")
		}
	}()
}