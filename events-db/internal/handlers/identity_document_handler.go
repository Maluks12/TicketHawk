package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ─────────────────────────────────────────────────────────────────────────────
// Identity document access
//
// This is the ONLY path to an organiser's ID photograph or proof of address.
// There is no GET route, no CDN URL, no signed link that outlives the request.
//
// THE RULE, in one sentence: a document is visible to the staff member doing
// the first KYC review, and after that review completes it belongs to the
// master admin alone.
//
// Expressed as the two phases the gate below implements:
//
//	kyc_first_verified_at IS NULL      the review window. Any staff role holding
//	                                   kyc.verify (access, editor, admin) may
//	                                   look — refusing here would mean approving
//	                                   an identity on faith
//
//	kyc_first_verified_at IS NOT NULL  closed. MASTER ADMIN ONLY. The document
//	                                   has done its job; every later read is a
//	                                   dispute, a regulator request or a
//	                                   retention audit, not routine work
//
// The break-glass exception, and the only one: an editor or admin with an
// approved, unexpired, organiser-scoped grant (migration 000073) may read a
// closed document. A master admin approves each grant individually and it
// expires on its own, so it never becomes standing access.
//
// THE GATE KEYS ON THE SUBJECT'S STATE, NOT THE CALLER'S RANK. That is why it
// cannot live in the capability matrix — no value of console_role answers
// "has this organiser been through review yet".
//
// Every read is written to kyc_document_access_log BEFORE the bytes leave. If
// the log write fails, the read does not happen. An unlogged look at someone's
// identity document is the exact thing this file exists to prevent, so the log
// is a precondition, not a side effect.
// ─────────────────────────────────────────────────────────────────────────────

const (
	ctxDocIsMaster  = "doc_is_master"
	ctxDocInReview  = "doc_in_review_window"
	ctxDocOrganizer = "doc_organizer_id"
)

// DocumentAccessState is the per-organiser answer the KYC screen renders from.
//
// Computed server-side and sent down, so the console never derives policy from
// a role — the same reason capabilities are fetched rather than inferred.
type DocumentAccessState struct {
	// Locked is true once the first review completed. Informational: it
	// explains WHY a document is not shown, so a reviewer who expects a photo
	// does not conclude the upload failed and reject a valid submission.
	Locked bool `json:"document_locked"`
	// CanView is the actual answer for THIS caller and THIS organiser.
	CanView bool `json:"can_view_document"`
	// ViaGrant means access came from an approved grant rather than from being
	// a master admin or being inside the review window. Worth surfacing,
	// because it expires.
	ViaGrant bool `json:"document_access_via_grant"`
	// GrantExpiresAt, when ViaGrant.
	GrantExpiresAt *time.Time `json:"document_grant_expires_at,omitempty"`
	// Reason is shown to staff who cannot view, in plain words.
	Reason string `json:"document_lock_reason,omitempty"`
}

// EvaluateDocumentAccess answers "may this caller see this organiser's
// documents right now" without serving anything.
//
// Shared by the gate and by the KYC detail endpoint, so the panel the console
// draws and the answer the server gives can never disagree — one function, two
// callers.
func EvaluateDocumentAccess(
	ctx context.Context, pool *pgxpool.Pool, userID, organizerID string,
) (DocumentAccessState, error) {
	var st DocumentAccessState

	var isMaster bool
	var staffRole *string
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(is_master_admin, FALSE), staff_role::text
		FROM users WHERE id = $1
	`, userID).Scan(&isMaster, &staffRole); err != nil {
		return st, err
	}

	var firstVerified *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT kyc_first_verified_at FROM organizers WHERE id = $1
	`, organizerID).Scan(&firstVerified); err != nil {
		return st, err
	}
	st.Locked = firstVerified != nil

	// Master admin passes in both phases. Checked first so everything below is
	// only ever reasoning about everyone else.
	if isMaster {
		st.CanView = true
		return st, nil
	}

	// A legacy platform admin with no staff_role gets the admin capability set
	// everywhere else (see RequireStaff). Not here. Document access is the one
	// place where "nobody got round to assigning you a role" must not read as
	// "you may see identity documents".
	var role models.StaffRole
	if staffRole != nil {
		if parsed, err := models.ParseStaffRole(*staffRole); err == nil {
			role = parsed
		}
	}

	// ── Phase 1: the review window ──────────────────────────────────────────
	if !st.Locked {
		if models.Can(&role, models.CapVerifyKYC) {
			st.CanView = true
			return st, nil
		}
		st.Reason = "Your role cannot review identity submissions."
		return st, nil
	}

	// ── Phase 2: closed ─────────────────────────────────────────────────────
	// read_only and access can hold no grant, so they are done here. Saying so
	// beats a generic refusal: it tells them not to bother asking.
	if role != models.StaffAdmin && role != models.StaffEditor {
		st.Reason = "This organiser has completed KYC. Identity documents are " +
			"readable by a master admin only."
		return st, nil
	}

	var grantID string
	var expires time.Time
	err := pool.QueryRow(ctx, `
		SELECT id::text, expires_at
		FROM kyc_document_grants
		WHERE granted_to = $1 AND organizer_id = $2
		  AND approved_at IS NOT NULL AND revoked_at IS NULL
		  AND expires_at > now()
		ORDER BY expires_at DESC
		LIMIT 1
	`, userID, organizerID).Scan(&grantID, &expires)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		st.Reason = "This organiser has completed KYC. Viewing their identity " +
			"document now needs a master admin to approve a time-limited grant."
		return st, nil
	case err != nil:
		return st, err
	}

	st.CanView = true
	st.ViaGrant = true
	st.GrantExpiresAt = &expires
	return st, nil
}

// RequireDocumentAccess enforces the rule above.
//
// Fails CLOSED on every error path. A database hiccup must never be the reason
// an identity document is served.
func RequireDocumentAccess(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.Param("id")

		userIDVal, ok := c.Get("user_id")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		userID, _ := userIDVal.(string)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		// Email is needed for the log whether this succeeds or fails, so it is
		// fetched before the decision rather than inside the happy path.
		var email string
		var isMaster bool
		if err := pool.QueryRow(ctx,
			`SELECT email, COALESCE(is_master_admin, FALSE) FROM users WHERE id = $1`, userID,
		).Scan(&email, &isMaster); err != nil {
			log.Error().Err(err).Msg("document access: actor lookup failed")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "could not verify access"})
			return
		}
		c.Set("actor_email", email)
		c.Set(ctxDocIsMaster, isMaster)
		c.Set(ctxDocOrganizer, organizerID)

		st, err := EvaluateDocumentAccess(ctx, pool, userID, organizerID)
		if errors.Is(err, pgx.ErrNoRows) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "organiser not found"})
			return
		}
		if err != nil {
			log.Error().Err(err).Msg("document access: evaluation failed")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "could not verify access"})
			return
		}

		if !st.CanView {
			// A refused attempt on an identity document belongs in the record a
			// regulator would read, not only in the console's own audit table.
			logDocAccess(pool, c, &organizerID, email, "view_denied",
				"Identity document requested without the right to see it", nil)
			resp := gin.H{"error": st.Reason}
			if st.Locked {
				resp["locked"] = true
				resp["hint"] = "Request access for this organiser, then ask a master admin to approve it."
			}
			c.AbortWithStatusJSON(http.StatusForbidden, resp)
			return
		}

		c.Set(ctxDocInReview, !st.Locked)
		c.Next()
	}
}

// RequireMasterAdmin gates the grant queue and the access log.
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
			logDocAccess(pool, c, nil, email, "view_denied",
				"Attempted access without master admin rights", nil)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "identity documents require master admin access",
			})
			return
		}

		c.Set("actor_email", email)
		c.Set(ctxDocIsMaster, true)
		c.Next()
	}
}

// GetIdentityDocumentHandler serves one document, once, to a caller the gate
// has already cleared.
//
//	POST /admin/kyc/:id/document          { "reason": "…", "category": "…" }
//	POST /admin/kyc/:id/proof-of-address  { "reason": "…", "category": "…" }
//
// A POST, not a GET, deliberately: the reason is mandatory for a closed
// document, and a body carries it better than a query string that would end up
// in nginx logs and browser history next to the organiser's id.
//
// kind is "document" (the ID photo) or "proof-of-address". Both go through the
// same gate. Splitting them would mean two rules to keep in step, and the
// weaker one would quietly become the way in.
func GetIdentityDocumentHandler(pool *pgxpool.Pool, kind string) gin.HandlerFunc {
	validCategories := map[string]bool{
		"dispute":             true,
		"regulator_request":   true,
		"fraud_investigation": true,
		"retention_audit":     true,
		"other":               true,
	}

	// Column pair per kind, chosen from this fixed map rather than built from
	// the request — the caller never names a column.
	type columns struct{ blob, mime, key, store string }
	cols := map[string]columns{
		"document":         {"id_document", "id_document_mime", "id_document_key", "id_document_store"},
		"proof-of-address": {"proof_of_address", "proof_of_address_mime", "", ""},
	}

	return func(c *gin.Context) {
		col, known := cols[kind]
		if !known {
			utils.InternalError(c, errors.New("unknown document kind "+kind), "unknown document type")
			return
		}

		organizerID := c.Param("id")
		email, _ := c.Get("actor_email")
		emailStr, _ := email.(string)

		inReviewVal, _ := c.Get(ctxDocInReview)
		inReview, _ := inReviewVal.(bool)

		var body struct {
			Reason   string `json:"reason"`
			Category string `json:"category"`
		}
		// An empty body is fine inside the review window, where a reason is
		// supplied rather than demanded. Outside it, the check below rejects.
		_ = c.ShouldBindJSON(&body)

		// A first-time reviewer looking at a submission still in its review
		// window is doing the job the document was uploaded for, so a default
		// reason is supplied rather than demanded — friction there just
		// produces "checking" typed a hundred times, which is a worse record
		// than an honest generated one.
		//
		// Reads AFTER the window are the ones that need justifying, and those
		// only reach here as a master admin or on an approved grant.
		if len(trimmed(body.Reason)) < 10 {
			if !inReview {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "a reason of at least 10 characters is required and is recorded permanently",
				})
				return
			}
			body.Reason = "First-time KYC review of a pending submission"
			body.Category = "kyc_review"
		}
		if inReview && body.Category == "" {
			body.Category = "kyc_review"
		}
		if body.Category != "kyc_review" && !validCategories[body.Category] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "category must be dispute, regulator_request, fraud_investigation, retention_audit or other",
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()

		// Where the document lives depends on whether the export has run.
		// id_document_key is set once it is in the external store; until then
		// the bytes are still inline (migration 000070). Proof of address was
		// never part of that offload, so it is always inline.
		var docKey, docStore, mime *string
		var hasInline bool

		if col.key != "" {
			err := pool.QueryRow(ctx, `
				SELECT `+col.key+`, `+col.store+`, `+col.mime+`,
				       (`+col.blob+` IS NOT NULL)
				FROM organizers WHERE id = $1
			`, organizerID).Scan(&docKey, &docStore, &mime, &hasInline)
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser not found"})
				return
			}
			if err != nil {
				utils.InternalError(c, err, "could not locate document")
				return
			}
		} else {
			err := pool.QueryRow(ctx, `
				SELECT `+col.mime+`, (`+col.blob+` IS NOT NULL)
				FROM organizers WHERE id = $1
			`, organizerID).Scan(&mime, &hasInline)
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser not found"})
				return
			}
			if err != nil {
				utils.InternalError(c, err, "could not locate document")
				return
			}
		}

		if docKey == nil && !hasInline {
			c.JSON(http.StatusNotFound, gin.H{"error": "no document of this type on file"})
			return
		}

		// LOG FIRST. If this fails, the read does not happen.
		action := "view"
		if kind == "proof-of-address" {
			action = "view_proof_of_address"
		}
		if err := logDocAccessSync(ctx, pool, c, &organizerID, emailStr,
			action, body.Reason, body.Category, docKey); err != nil {
			utils.InternalError(c, err, "could not record access — read refused")
			return
		}

		if docKey != nil {
			// Exported: the console never proxies the bytes. It returns the key
			// so a short-lived signed URL can be minted against the object
			// store, which applies its own access policy and writes its own
			// log — a store-side log survives a compromise of this one.
			c.JSON(http.StatusOK, gin.H{
				"location":   "external",
				"store":      docStore,
				"key":        docKey,
				"note":       "Generate a short-lived signed URL from the document store using this key.",
				"expires_in": 300,
			})
			return
		}

		var data []byte
		if err := pool.QueryRow(ctx,
			`SELECT `+col.blob+` FROM organizers WHERE id = $1`, organizerID,
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
			SELECT l.at, l.actor_email, l.action, l.reason,
			       l.organizer_id::text, o.business_name, l.document_key, l.ip::text
			FROM kyc_document_access_log l
			LEFT JOIN organizers o ON o.id = l.organizer_id
			ORDER BY l.at DESC
			LIMIT 200
		`)
		if err != nil {
			utils.InternalError(c, err, "could not load access log")
			return
		}
		defer rows.Close()

		type entry struct {
			At           time.Time `json:"at"`
			ActorEmail   string    `json:"actor_email"`
			Action       string    `json:"action"`
			Reason       string    `json:"reason"`
			OrganizerID  *string   `json:"organizer_id"`
			BusinessName *string   `json:"business_name"`
			DocumentKey  *string   `json:"document_key"`
			IP           *string   `json:"ip"`
		}
		out := []entry{}
		for rows.Next() {
			var e entry
			if err := rows.Scan(&e.At, &e.ActorEmail, &e.Action, &e.Reason,
				&e.OrganizerID, &e.BusinessName, &e.DocumentKey, &e.IP); err != nil {
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
	// kyc_document_access_log CHECKs reason at 10+ characters, so a short
	// denial reason would silently fail to write. Padded here rather than at
	// each call site.
	if len(trimmed(reason)) < 10 {
		reason = "Access denied: " + reason
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := logDocAccessSync(ctx, pool, c, organizerID, email, action, reason, "", key); err != nil {
			log.Warn().Err(err).Msg("kyc access log write failed")
		}
	}()
}