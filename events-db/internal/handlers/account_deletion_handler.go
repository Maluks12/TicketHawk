package handlers

import (
	"errors"
	"net/http"
	"strings"

	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// ─────────────────────────────────────────────────────────────────────────────
// Self-service account deletion
//
// Required by Apple App Store Guideline 5.1.1(v): an app that lets a user
// create an account must let them delete it from inside the app, without being
// sent to a website or told to email support.
//
// Two endpoints rather than one, because a destructive action that cannot be
// previewed is a bad destructive action:
//
//	GET    /v1/user/me/deletion-preview  — what will happen if I do this?
//	DELETE /v1/user/me                   — do it
//
// The preview exists so the confirmation screen can say "you have 3 upcoming
// tickets worth R1,480" instead of a generic "are you sure?". People do not
// read generic warnings; they do read specific ones about their own money.
// ─────────────────────────────────────────────────────────────────────────────

// DeleteAccountRequest is the body of DELETE /v1/user/me.
type DeleteAccountRequest struct {
	// Password re-authentication. A valid access token is not sufficient
	// authority to destroy an account — the same reasoning that already applies
	// to changing the email address in UpdateMyProfileHandler. Someone holding a
	// stolen token should not be able to erase the victim's account (and with it
	// their ability to reach their own tickets) in a single request.
	Password string `json:"password" binding:"required"`

	// AcknowledgeForfeit must be true when the account holds confirmed tickets
	// for events that have not happened yet. The first call returns 409 with the
	// details so the client can show a second, specific confirmation; the client
	// then repeats the call with this set. It is a deliberate speed bump, not a
	// refusal — the deletion always remains completable in-app, which is what
	// the guideline requires.
	AcknowledgeForfeit bool `json:"acknowledge_forfeit"`

	// Optional product feedback. Stored on the stub row; never anything the user
	// wrote about themselves elsewhere.
	Reason string `json:"reason"`
}

// GetAccountDeletionPreviewHandler reports what deletion would affect.
//
//	GET /v1/user/me/deletion-preview
//
// Read-only and idempotent; safe to call every time the screen opens.
func GetAccountDeletionPreviewHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)

		blockers, err := repository.GetAccountDeletionBlockers(c.Request.Context(), pool, userID)
		if err != nil {
			utils.InternalError(c, err, "checking account deletion state")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"can_delete":        !blockers.HasOrganiserHold(),
			"organiser_hold":    blockers.HasOrganiserHold(),
			"upcoming_tickets":  blockers.UpcomingTickets,
			"upcoming_value":    blockers.UpcomingValue,
			"live_events":       blockers.LiveEvents,
			"unsettled_payouts": blockers.UnsettledPayouts,
		})
	}
}

// DeleteMyAccountHandler deletes the authenticated user's own account.
//
//	DELETE /v1/user/me
//	Body: { "password": "...", "acknowledge_forfeit": false, "reason": "..." }
//
// Responses the client must handle:
//
//	200 — done; drop local tokens and return to a signed-out state
//	401 — wrong password
//	409 organiser_hold        — cannot proceed; live events or unsettled payouts
//	409 tickets_will_be_lost  — retry with acknowledge_forfeit: true
func DeleteMyAccountHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)

		var req DeleteAccountRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Enter your password to delete your account",
				"code":  "password_required",
			})
			return
		}

		// ── Re-authenticate ──────────────────────────────────────────────────
		hash, err := repository.GetPasswordHash(pool, c.Request.Context(), userID)
		if err != nil {
			if errors.Is(err, repository.ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}
			utils.InternalError(c, err, "verifying password")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "That password is incorrect",
				"code":  "password_incorrect",
			})
			return
		}

		// ── Check what this would disturb ────────────────────────────────────
		blockers, err := repository.GetAccountDeletionBlockers(c.Request.Context(), pool, userID)
		if err != nil {
			utils.InternalError(c, err, "checking account deletion state")
			return
		}

		// Organiser hold. This is the one case that genuinely cannot proceed
		// unattended, and it is not about protecting the platform — other people
		// hold tickets to those events, and money may still be owed. Deleting the
		// organiser behind a live event would leave buyers with an event nobody
		// is accountable for, and an in-flight payout with nowhere to land.
		//
		// Apple accepts a documented hold of this kind provided the path forward
		// is clear and in-app, which is why the message says exactly what to do
		// rather than "contact support".
		if blockers.HasOrganiserHold() {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Your organiser account is still active. Cancel or complete your " +
					"live events and wait for any outstanding payouts to settle, then delete your account.",
				"code":              "organiser_hold",
				"live_events":       blockers.LiveEvents,
				"unsettled_payouts": blockers.UnsettledPayouts,
				"support_email":     "support@ticket-hawk.com",
			})
			return
		}

		// Upcoming tickets. Warn once, proceed on the second call.
		if blockers.UpcomingTickets > 0 && !req.AcknowledgeForfeit {
			c.JSON(http.StatusConflict, gin.H{
				"error":            "You still have tickets to events that haven't happened yet.",
				"code":             "tickets_will_be_lost",
				"upcoming_tickets": blockers.UpcomingTickets,
				"upcoming_value":   blockers.UpcomingValue,
			})
			return
		}

		// ── Do it ────────────────────────────────────────────────────────────
		var reason *string
		if r := strings.TrimSpace(req.Reason); r != "" {
			if len([]rune(r)) > 500 {
				r = string([]rune(r)[:500])
			}
			reason = &r
		}

		if err := repository.SoftDeleteUser(c.Request.Context(), pool, userID, reason); err != nil {
			if errors.Is(err, repository.ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}
			utils.InternalError(c, err, "deleting account")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"deleted": true,
			"message": "Your account has been deleted.",
		})
	}
}