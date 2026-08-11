package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"events-db/internal/middleware"
	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Staff capabilities and the request lifecycle
//
// Routes are registered in cmd/api/main.go on a group that already carries
// AuthMiddleware + RequireStaff.
//
// The lifecycle a dangerous action moves through:
//
//	POST   /admin/requests              editor+ logs it        → pending_approval
//	POST   /admin/requests/:id/approve  a DIFFERENT admin      → awaiting_subject | ready
//	POST   /admin/requests/:id/confirm  the organiser's OTP    → ready
//	POST   /admin/requests/:id/execute  after the cooling-off  → executed
//
// Every transition is a separate call by a separate party. Nothing here can
// short-circuit that, because the database constraints from migration 000068
// reject the write rather than trusting this code to be correct.
// ─────────────────────────────────────────────────────────────────────────────

// GET /admin/me/capabilities
//
// The console calls this on load and renders from the answer. Returning the
// list rather than letting the client derive it from the role is what stops the
// two drifting — the UI can be wrong about presentation but never about policy.
func GetMyCapabilitiesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := middleware.StaffRoleFrom(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"staff_role": nil, "capabilities": []string{}})
			return
		}
		caps := models.Capabilities(&role)
		out := make([]string, len(caps))
		for i, cp := range caps {
			out[i] = string(cp)
		}
		c.JSON(http.StatusOK, gin.H{
			"staff_role":   string(role),
			"capabilities": out,
		})
	}
}

// ── Create ───────────────────────────────────────────────────────────────────

type createRequestBody struct {
	Kind          string         `json:"kind"`
	Reason        string         `json:"reason"`
	TargetUserID  *string        `json:"target_user_id"`
	TargetEventID *string        `json:"target_event_id"`
	Payload       map[string]any `json:"payload"`
}

// POST /admin/requests — requires CapRequestDangerous (editor+).
func CreateStaffRequestHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID := c.MustGet("user_id").(string)

		var body createRequestBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		kind := models.StaffRequestKind(body.Kind)
		policy, known := models.RequestPolicies[kind]
		if !known {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown request kind"})
			return
		}
		// Mirrors the DB constraint. Checked here too so the caller gets a
		// useful message instead of a constraint violation.
		if len(trimmed(body.Reason)) < 10 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "give a reason of at least 10 characters — the approver has to judge this",
			})
			return
		}
		if body.TargetUserID == nil && body.TargetEventID == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a target user or event is required"})
			return
		}

		// Secrets must never be parked in a pending row: it stays readable for
		// as long as the request is open. Bank additions carry the masked tail
		// and a provider reference, never a full account number.
		if body.Payload != nil {
			for _, banned := range []string{"account_number", "password", "id_number", "code"} {
				if _, present := body.Payload[banned]; present {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": fmt.Sprintf("payload must not contain %q", banned),
					})
					return
				}
			}
		}
		payloadJSON, _ := json.Marshal(body.Payload)
		if body.Payload == nil {
			payloadJSON = []byte("{}")
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var id string
		err := pool.QueryRow(ctx, `
			INSERT INTO staff_requests
				(kind, requested_by, reason, target_user_id, target_event_id,
				 payload, subject_required)
			VALUES ($1::staff_request_kind, $2, $3, $4, $5, $6::jsonb, $7)
			RETURNING id::text
		`, string(kind), actorID, body.Reason,
			body.TargetUserID, body.TargetEventID, payloadJSON,
			policy.SubjectMustConfirm,
		).Scan(&id)
		if err != nil {
			utils.InternalError(c, err, "could not create request")
			return
		}

		middleware.AuditAllowed(pool, c, "request.create", "staff_request", id, map[string]any{
			"kind": string(kind),
		})

		c.JSON(http.StatusCreated, gin.H{
			"id":     id,
			"status": "pending_approval",
			"policy": policy,
			"note":   "A different admin must approve this. You cannot approve your own request.",
		})
	}
}

// ── Approve ──────────────────────────────────────────────────────────────────

// POST /admin/requests/:id/approve — requires CapApproveRequests (admin).
//
// The four-eyes rule is enforced THREE times over: by the WHERE clause below,
// by the CHECK constraint in migration 000068, and by the friendly message on
// the 409. The first two are the controls; the third is courtesy.
func ApproveStaffRequestHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID := c.MustGet("user_id").(string)
		reqID := c.Param("id")
		if _, err := uuid.Parse(reqID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var kindStr string
		var requestedBy string
		err := pool.QueryRow(ctx, `
			SELECT kind::text, requested_by::text
			FROM staff_requests
			WHERE id = $1 AND status = 'pending_approval' AND expires_at > now()
		`, reqID).Scan(&kindStr, &requestedBy)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "request not found, already actioned, or expired"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "could not load request")
			return
		}

		if requestedBy == actorID {
			middleware.AuditAllowed(pool, c, "request.self_approve_blocked", "staff_request", reqID, nil)
			c.JSON(http.StatusConflict, gin.H{
				"error": "you cannot approve your own request — a different admin must review it",
			})
			return
		}

		policy := models.RequestPolicies[models.StaffRequestKind(kindStr)]
		// Cooling-off starts at APPROVAL, not at creation: the clock should run
		// from when the action became real, not from when it was typed.
		notBefore := time.Now().Add(time.Duration(policy.CoolingOffMinutes) * time.Minute)

		nextStatus := "ready"
		if policy.SubjectMustConfirm {
			nextStatus = "awaiting_subject"
		}

		tag, err := pool.Exec(ctx, `
			UPDATE staff_requests
			SET status = $3::staff_request_status,
			    approved_by = $2,
			    approved_at = now(),
			    execute_not_before = $4
			WHERE id = $1
			  AND status = 'pending_approval'
			  AND requested_by <> $2
		`, reqID, actorID, nextStatus, notBefore)
		if err != nil {
			utils.InternalError(c, err, "could not approve request")
			return
		}
		if tag.RowsAffected() == 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "request was actioned by someone else"})
			return
		}

		middleware.AuditAllowed(pool, c, "request.approve", "staff_request", reqID, map[string]any{
			"kind": kindStr,
		})

		c.JSON(http.StatusOK, gin.H{
			"status":             nextStatus,
			"execute_not_before": notBefore,
			"subject_required":   policy.SubjectMustConfirm,
		})
	}
}

// POST /admin/requests/:id/reject — requires CapApproveRequests.
func RejectStaffRequestHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID := c.MustGet("user_id").(string)
		reqID := c.Param("id")

		var body struct {
			Reason string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || len(trimmed(body.Reason)) < 5 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a rejection reason is required — the requester sees this"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		tag, err := pool.Exec(ctx, `
			UPDATE staff_requests
			SET status = 'rejected', rejected_by = $2, rejected_at = now(), rejection_reason = $3
			WHERE id = $1 AND status IN ('pending_approval', 'awaiting_subject')
		`, reqID, actorID, body.Reason)
		if err != nil {
			utils.InternalError(c, err, "could not reject request")
			return
		}
		if tag.RowsAffected() == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "request not found or already actioned"})
			return
		}

		middleware.AuditAllowed(pool, c, "request.reject", "staff_request", reqID, nil)
		c.JSON(http.StatusOK, gin.H{"status": "rejected"})
	}
}

// ── Subject confirmation (layer 3) ───────────────────────────────────────────

// POST /admin/requests/:id/send-code — requires CapRequestDangerous.
//
// Generates the organiser's confirmation code and stores only its hash. The
// plaintext exists for exactly as long as it takes to email it.
func SendConfirmationCodeHandler(pool *pgxpool.Pool, sendEmail func(to, code string) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.Param("id")

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		var email string
		err := pool.QueryRow(ctx, `
			SELECT u.email
			FROM staff_requests r
			JOIN users u ON u.id = r.target_user_id
			WHERE r.id = $1 AND r.status = 'awaiting_subject'
		`, reqID).Scan(&email)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "request not awaiting confirmation"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "could not load request")
			return
		}

		code, err := secureCode(6)
		if err != nil {
			utils.InternalError(c, err, "could not generate code")
			return
		}
		sum := sha256.Sum256([]byte(code))

		// Upsert so re-sending replaces the old code AND resets attempts —
		// otherwise a resend would inherit a nearly-exhausted attempt counter
		// and lock out the legitimate organiser.
		_, err = pool.Exec(ctx, `
			INSERT INTO staff_request_confirmations
				(request_id, code_hash, sent_to, expires_at, attempts)
			VALUES ($1, $2, $3, now() + INTERVAL '15 minutes', 0)
			ON CONFLICT (request_id) DO UPDATE SET
				code_hash = EXCLUDED.code_hash,
				sent_to = EXCLUDED.sent_to,
				sent_at = now(),
				expires_at = EXCLUDED.expires_at,
				attempts = 0
		`, reqID, hex.EncodeToString(sum[:]), email)
		if err != nil {
			utils.InternalError(c, err, "could not store confirmation")
			return
		}

		if sendEmail != nil {
			if mailErr := sendEmail(email, code); mailErr != nil {
				utils.InternalError(c, mailErr, "could not send confirmation email")
				return
			}
		}

		middleware.AuditAllowed(pool, c, "request.send_code", "staff_request", reqID, nil)
		// The code is never returned in the response — it must travel to the
		// organiser, not back to the staff member who triggered it. Returning
		// it would collapse layer 3 into layer 1.
		c.JSON(http.StatusOK, gin.H{"sent": true, "expires_in_minutes": 15})
	}
}

// POST /admin/requests/:id/confirm  { "code": "123456" }
//
// Called on the ORGANISER's behalf. Deliberately verifies and increments the
// attempt counter in one statement so parallel guesses can't race past the
// ceiling the DB constraint imposes.
func ConfirmStaffRequestHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.Param("id")
		var body struct {
			Code string `json:"code"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Code == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code required"})
			return
		}
		sum := sha256.Sum256([]byte(body.Code))
		hash := hex.EncodeToString(sum[:])

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		tx, err := pool.Begin(ctx)
		if err != nil {
			utils.InternalError(c, err, "could not verify code")
			return
		}
		defer tx.Rollback(ctx)

		var storedHash string
		var attempts int
		err = tx.QueryRow(ctx, `
			UPDATE staff_request_confirmations
			SET attempts = attempts + 1
			WHERE request_id = $1 AND expires_at > now() AND attempts < 5
			RETURNING code_hash, attempts
		`, reqID).Scan(&storedHash, &attempts)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusGone, gin.H{"error": "code expired or too many attempts — request a new one"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "could not verify code")
			return
		}

		if storedHash != hash {
			// Commit so the incremented attempt count survives a wrong guess —
			// rolling back here would make the counter meaningless.
			_ = tx.Commit(ctx)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "incorrect code", "attempts_remaining": 5 - attempts,
			})
			return
		}

		if _, err := tx.Exec(ctx, `
			UPDATE staff_requests
			SET subject_confirmed_at = now(), status = 'ready'
			WHERE id = $1 AND status = 'awaiting_subject'
		`, reqID); err != nil {
			utils.InternalError(c, err, "could not confirm request")
			return
		}
		// The hash has done its job; keeping it serves no purpose and is one
		// more secret sitting at rest.
		if _, err := tx.Exec(ctx, `DELETE FROM staff_request_confirmations WHERE request_id = $1`, reqID); err != nil {
			utils.InternalError(c, err, "could not clear confirmation")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			utils.InternalError(c, err, "could not confirm request")
			return
		}

		middleware.AuditAllowed(pool, c, "request.subject_confirmed", "staff_request", reqID, nil)
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}

// ── List ─────────────────────────────────────────────────────────────────────

// GET /admin/requests?status=pending_approval
func ListStaffRequestsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := c.Query("status")

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		rows, err := pool.Query(ctx, `
			SELECT r.id::text, r.kind::text, r.status::text, r.reason,
			       r.requested_at, r.expires_at, r.execute_not_before,
			       r.subject_required, r.subject_confirmed_at,
			       req.email AS requested_by_email,
			       app.email AS approved_by_email,
			       r.target_user_id::text, r.target_event_id::text, r.payload
			FROM staff_requests r
			JOIN users req ON req.id = r.requested_by
			LEFT JOIN users app ON app.id = r.approved_by
			WHERE ($1::text = '' OR r.status::text = $1::text)
			ORDER BY r.requested_at DESC
			LIMIT 200
		`, status)
		if err != nil {
			utils.InternalError(c, err, "could not list requests")
			return
		}
		defer rows.Close()

		type item struct {
			ID                 string     `json:"id"`
			Kind               string     `json:"kind"`
			Status             string     `json:"status"`
			Reason             string     `json:"reason"`
			RequestedAt        time.Time  `json:"requested_at"`
			ExpiresAt          time.Time  `json:"expires_at"`
			ExecuteNotBefore   *time.Time `json:"execute_not_before"`
			SubjectRequired    bool       `json:"subject_required"`
			SubjectConfirmedAt *time.Time `json:"subject_confirmed_at"`
			RequestedByEmail   string     `json:"requested_by_email"`
			ApprovedByEmail    *string    `json:"approved_by_email"`
			TargetUserID       *string    `json:"target_user_id"`
			TargetEventID      *string    `json:"target_event_id"`
			Payload            any        `json:"payload"`
		}

		out := []item{}
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.ID, &it.Kind, &it.Status, &it.Reason,
				&it.RequestedAt, &it.ExpiresAt, &it.ExecuteNotBefore,
				&it.SubjectRequired, &it.SubjectConfirmedAt,
				&it.RequestedByEmail, &it.ApprovedByEmail,
				&it.TargetUserID, &it.TargetEventID, &it.Payload); err != nil {
				utils.InternalError(c, err, "could not read requests")
				return
			}
			out = append(out, it)
		}
		if err := rows.Err(); err != nil {
			utils.InternalError(c, err, "could not read requests")
			return
		}

		c.JSON(http.StatusOK, gin.H{"requests": out})
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// secureCode returns an n-digit numeric code from crypto/rand.
//
// crypto/rand, not math/rand: this code is the second factor protecting a
// payout redirect, and a predictable one is no factor at all.
func secureCode(n int) (string, error) {
	const digits = "0123456789"
	out := make([]byte, n)
	for i := range out {
		v, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		out[i] = digits[v.Int64()]
	}
	return string(out), nil
}

func trimmed(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}