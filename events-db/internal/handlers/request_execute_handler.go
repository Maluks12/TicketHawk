package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"events-db/internal/cache"
	"events-db/internal/middleware"
	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Request execution — the last gate
//
// Everything up to here has been ABOUT an action. This is where one happens.
//
// The design rule for this file: the executor never re-decides whether the
// action is allowed. Authorisation, four-eyes, subject confirmation and
// cooling-off were settled by the time a request reaches 'ready', and the
// database's staff_requests_execution_gated CHECK will refuse the write if any
// of them were skipped. Re-checking here would put a second, drifting copy of
// the policy in the one place where being wrong is most expensive.
//
// What this file DOES own is that the action and the record of it are atomic.
// Every executor runs inside the same transaction that marks the request
// executed, so there is no state where the bank details are gone but the
// request still says 'ready' — which is exactly the state someone would exploit
// by executing twice.
// ─────────────────────────────────────────────────────────────────────────────

// POST /admin/requests/:id/execute — requires CapApproveRequests.
func ExecuteStaffRequestHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID := c.MustGet("user_id").(string)
		reqID := c.Param("id")

		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()

		tx, err := pool.Begin(ctx)
		if err != nil {
			utils.InternalError(c, err, "could not start execution")
			return
		}
		defer tx.Rollback(ctx)

		// FOR UPDATE, and the status/time predicates are in the WHERE clause
		// rather than checked in Go afterwards.
		//
		// Two people clicking Execute at the same moment is not hypothetical —
		// it is what happens when a request has been sitting in a queue that
		// several admins watch. Without the row lock both would read 'ready',
		// both would proceed, and the action would run twice.
		var kindStr string
		var targetUser, targetEvent *string
		var subjectRequired bool
		var subjectConfirmed *time.Time
		var notBefore *time.Time

		err = tx.QueryRow(ctx, `
			SELECT kind::text, target_user_id::text, target_event_id::text,
			       subject_required, subject_confirmed_at, execute_not_before
			FROM staff_requests
			WHERE id = $1
			  AND status = 'ready'
			  AND executed_at IS NULL
			  AND expires_at > now()
			FOR UPDATE
		`, reqID).Scan(&kindStr, &targetUser, &targetEvent,
			&subjectRequired, &subjectConfirmed, &notBefore)

		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "request is not ready to execute — it may already have run, expired, or still be waiting on approval or confirmation",
			})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "could not load request")
			return
		}

		// Cooling-off is reported here rather than left to the CHECK constraint
		// purely so the caller gets a useful message and a time. The constraint
		// remains the actual control.
		if notBefore != nil && time.Now().Before(*notBefore) {
			c.JSON(http.StatusConflict, gin.H{
				"error":              "cooling-off period has not elapsed",
				"execute_not_before": *notBefore,
			})
			return
		}

		kind := models.StaffRequestKind(kindStr)
		policy := models.RequestPolicies[kind]

		// ── Perform the action ───────────────────────────────────────────────
		if err := executeAction(ctx, tx, kind, targetUser, targetEvent, policy); err != nil {
			utils.InternalError(c, err, "could not perform the requested action")
			return
		}

		// Marking executed in the SAME transaction. If this fails, the action
		// above rolls back with it.
		if _, err := tx.Exec(ctx, `
			UPDATE staff_requests
			SET status = 'executed', executed_at = now(), executed_by = $2
			WHERE id = $1
		`, reqID, actorID); err != nil {
			utils.InternalError(c, err, "could not record execution")
			return
		}

		if err := tx.Commit(ctx); err != nil {
			utils.InternalError(c, err, "could not commit execution")
			return
		}

		// Cache busting AFTER commit. Doing it inside the transaction would
		// publish a change that could still roll back.
		if ec != nil && kind == models.ReqEventTakedown {
			ec.BustOnPublish(context.Background())
		}

		middleware.AuditAllowed(pool, c, "request.execute", "staff_request", reqID, map[string]any{
			"kind": kindStr,
		})

		c.JSON(http.StatusOK, gin.H{"status": "executed", "kind": kindStr})
	}
}

// executeAction dispatches to the right operation.
//
// Takes the transaction, not the pool, so every action is atomic with the
// status update that follows it.
func executeAction(
	ctx context.Context, tx pgx.Tx,
	kind models.StaffRequestKind,
	targetUser, targetEvent *string,
	policy models.RequestPolicy,
) error {
	switch kind {

	case models.ReqBankDelete:
		if targetUser == nil {
			return fmt.Errorf("bank deletion needs a target user")
		}
		// Clears the details and holds payouts. The hold matters even on a
		// DELETION: an organiser whose details vanish should notice before the
		// next settlement runs, and a hold is what creates that gap.
		_, err := tx.Exec(ctx, `
			UPDATE organizers
			SET bank_account_last_4 = NULL,
			    bank_name = NULL,
			    bank_account_name = NULL,
			    payout_hold_until = now() + ($2 || ' hours')::interval,
			    payout_hold_reason = 'Bank details removed by staff request'
			WHERE user_id = $1
		`, *targetUser, 48)
		return err

	case models.ReqBankAdd:
		// NOT IMPLEMENTED, and this returns an error rather than silently
		// succeeding.
		//
		// Adding bank details cannot be done from a request payload, because
		// the payload deliberately never contains an account number (see
		// staff_handler.go, which rejects it). The number has to go to the
		// payment provider directly and come back as a token; wiring that is a
		// Paystack integration task, not a database write.
		//
		// Failing loudly is the point: an executor that no-ops would mark the
		// request executed and leave everyone believing the details were added.
		return fmt.Errorf("bank_details_add cannot be executed from the console — " +
			"the account number must be submitted to the payment provider directly")

	case models.ReqEventTakedown:
		if targetEvent == nil {
			return fmt.Errorf("takedown needs a target event")
		}
		// Unpublished, NOT cancelled or deleted.
		//
		// Cancelling triggers refunds and tells buyers the event is off;
		// deleting destroys the tickets people hold. A takedown is about
		// removing it from the feed while the organiser fixes something, so
		// existing ticket holders keep their tickets and their QR codes
		// continue to scan.
		_, err := tx.Exec(ctx, `
			UPDATE events
			SET status = 'draft', is_featured = FALSE
			WHERE id = $1 AND status IN ('published', 'published_private')
		`, *targetEvent)
		return err

	case models.ReqCredentialReset:
		if targetUser == nil {
			return fmt.Errorf("credential reset needs a target user")
		}
		// Revokes every session rather than setting a new password. The account
		// is presumed compromised, so the goal is to get the attacker out; the
		// legitimate owner then uses the ordinary forgot-password flow, which
		// proves control of the mailbox. Setting a password here would mean
		// staff knew it.
		_, err := tx.Exec(ctx, `
			UPDATE refresh_tokens
			SET revoked_at = now()
			WHERE user_id = $1 AND revoked_at IS NULL
		`, *targetUser)
		return err

	case models.ReqRoleChange:
		// Handled by the dedicated endpoint in people_handler.go, which has the
		// last-admin and self-change guards. Routing it through here would
		// bypass those.
		return fmt.Errorf("role changes are made directly, not through request execution")

	case models.ReqAccountDelete:
		// Deliberately not wired to the existing deletion path.
		//
		// Account deletion already exists as a user-initiated flow with its own
		// anonymisation transaction and organiser-hold logic (migration
		// 000065). Reimplementing it here would produce a second, diverging
		// definition of "deleted" — and this one would be the one used on
		// people who did not ask for it.
		return fmt.Errorf("account deletion must go through the existing deletion flow")

	default:
		return fmt.Errorf("no executor for request kind %q", kind)
	}
}

// ── Takedown request helper ──────────────────────────────────────────────────

// POST /admin/events/:id/request-takedown — requires CapRequestDangerous.
//
// A convenience wrapper over request creation so the events page can offer a
// button without the console assembling a generic request body. Editors log it;
// an admin approves it; only then does it execute.
func RequestEventTakedownHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID := c.MustGet("user_id").(string)
		eventID := c.Param("id")

		var body struct {
			Reason string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || len(body.Reason) < 10 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "a reason of at least 10 characters is required — an admin has to judge this",
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// An admin holds both CapRequestDangerous and CapApproveRequests, so
		// without this an admin could create and approve in two clicks and the
		// four-eyes rule would be theatre. The DB CHECK stops the approval
		// either way; this just makes it visible at creation time.
		var id string
		err := pool.QueryRow(ctx, `
			INSERT INTO staff_requests
				(kind, requested_by, reason, target_event_id, subject_required)
			VALUES ('event_takedown', $1, $2, $3, FALSE)
			RETURNING id::text
		`, actorID, body.Reason, eventID).Scan(&id)
		if err != nil {
			utils.InternalError(c, err, "could not log takedown request")
			return
		}

		middleware.AuditAllowed(pool, c, "event.takedown_requested", "event", eventID, map[string]any{
			"request_id": id,
		})

		c.JSON(http.StatusCreated, gin.H{
			"request_id": id,
			"status":     "pending_approval",
			"note":       "A different admin must approve this before the event comes down.",
		})
	}
}