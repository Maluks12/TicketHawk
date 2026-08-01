package handlers

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

// ── Profile editing ──────────────────────────────────────────────────────────
//
// Before this the app had no way to change a name, phone number or email —
// GET /v1/user/me was the only user route, and the "Edit profile" row on the
// profile screen was wired to an empty onPress.
//
// ── On changing the email address ────────────────────────────────────────────
//
// Email is the login identifier, so changing it is a security-relevant action
// rather than a cosmetic one. Two protections are applied here:
//
//   1. The new address must be unique. Without that check the UPDATE would fail
//      on the unique index and surface as a 500.
//
//   2. email_verified is reset to FALSE and a fresh verification is required.
//      Otherwise a user could move their account onto an address they do not
//      control, and any future "verified email" assumption elsewhere in the
//      system would be wrong.
//
//   3. The current password is required before the address can change. Without
//      it, anyone holding a stolen access token could quietly move the account
//      onto their own email and then use the ordinary "forgot password" flow to
//      take it over permanently — the session theft becomes account theft, and
//      the real owner finds out when they can no longer sign in.
//
//      Only an EMAIL change demands it. Requiring a password to correct a typo
//      in a surname is friction with no threat behind it, and friction that
//      buys nothing trains people to distrust the prompt when it does matter.

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// UpdateProfileRequest carries only the fields a user may change about
// themselves. Role, email_verified and id are deliberately absent — accepting
// them from the client is how privilege escalation bugs happen.
type UpdateProfileRequest struct {
	FirstName   *string `json:"first_name"`
	LastName    *string `json:"last_name"`
	PhoneNumber *string `json:"phone_number"`
	Email       *string `json:"email"`
	// CurrentPassword is required only when Email is present. See the note above.
	CurrentPassword *string `json:"current_password"`
}

// UpdateMyProfileHandler updates the authenticated user's own profile.
//
//	PATCH /v1/user/me
//
// Every field is optional; only those present are written. That makes the
// endpoint safe to call from a form that submits one changed field.
//
// emailService may be nil (tests, or a deployment with email disabled); the
// change-notification below is skipped rather than panicking in that case.
func UpdateMyProfileHandler(pool *pgxpool.Pool, emailService *services.EmailService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)

		var req UpdateProfileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		upd := repository.ProfileUpdate{}

		if req.FirstName != nil {
			v := strings.TrimSpace(*req.FirstName)
			if len([]rune(v)) < 1 || len([]rune(v)) > 80 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "First name must be between 1 and 80 characters"})
				return
			}
			upd.FirstName = &v
		}

		if req.LastName != nil {
			v := strings.TrimSpace(*req.LastName)
			if len([]rune(v)) < 1 || len([]rune(v)) > 80 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Last name must be between 1 and 80 characters"})
				return
			}
			upd.LastName = &v
		}

		if req.PhoneNumber != nil {
			// Normalised the same way the app does: strip a +27 / 27 / leading 0
			// prefix down to the 9 national digits, then store as 0XXXXXXXXX.
			// Doing it here as well means a client that skips normalisation —
			// or a future one — still writes a consistent value.
			digits := onlyDigits(*req.PhoneNumber)
			digits = strings.TrimPrefix(digits, "27")
			digits = strings.TrimPrefix(digits, "0")
			if len(digits) != 9 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a valid South African mobile number"})
				return
			}
			local := "0" + digits
			upd.PhoneNumber = &local
		}

		if req.Email != nil {
			v := strings.ToLower(strings.TrimSpace(*req.Email))
			if !emailRe.MatchString(v) || len(v) > 255 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a valid email address"})
				return
			}

			// Re-authenticate. A valid access token is not sufficient authority
			// to change the login identifier.
			if req.CurrentPassword == nil || *req.CurrentPassword == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Enter your current password to change your email address",
					"code":  "password_required",
				})
				return
			}
			hash, err := repository.GetPasswordHash(pool, c.Request.Context(), userID)
			if err != nil {
				if errors.Is(err, repository.ErrUserNotFound) {
					c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
					return
				}
				utils.InternalError(c, err, "verifying password")
				return
			}
			if bcrypt.CompareHashAndPassword([]byte(hash), []byte(*req.CurrentPassword)) != nil {
				// 401 rather than 403: the credential supplied was wrong, which
				// is what the client needs to distinguish from "not allowed".
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "That password is incorrect",
					"code":  "password_incorrect",
				})
				return
			}

			taken, err := repository.EmailTakenByAnotherUser(pool, c.Request.Context(), v, userID)
			if err != nil {
				utils.InternalError(c, err, "checking email availability")
				return
			}
			if taken {
				c.JSON(http.StatusConflict, gin.H{"error": "That email address is already in use"})
				return
			}
			upd.Email = &v
		}

		// Capture the address the account had BEFORE the write. The security
		// alert has to reach the person who owned the account until a moment
		// ago, and after UpdateUserProfile runs that address is gone from the
		// row. Read once, here, only when it will actually be needed.
		var previousEmail, previousFirstName string
		if upd.Email != nil {
			if em, fn, err := repository.GetUserContact(pool, c.Request.Context(), userID); err == nil {
				previousEmail, previousFirstName = em, fn
			}
		}

		if upd.IsEmpty() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No changes supplied"})
			return
		}

		user, err := repository.UpdateUserProfile(pool, c.Request.Context(), userID, upd)
		if err != nil {
			if errors.Is(err, repository.ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}
			utils.InternalError(c, err, "updating profile")
			return
		}

		// Tell both addresses that the login email moved. Fire-and-forget on a
		// background context: SES latency must not be charged to the user's
		// request, and a mail failure must not fail a profile update that has
		// already been committed. Errors are logged, not surfaced.
		if upd.Email != nil && emailService != nil && previousEmail != "" && previousEmail != *upd.Email {
			go func(first, oldAddr, newAddr string) {
				if err := emailService.SendEmailChangeConfirmation(first, oldAddr, newAddr, time.Now()); err != nil {
					log.Error().Err(err).
						Str("user_id", userID).
						Msg("failed to send email-change notification")
				}
			}(previousFirstName, previousEmail, *upd.Email)
		}

		c.JSON(http.StatusOK, gin.H{
			"user": user,
			// The client uses this to decide whether to tell the user to go and
			// confirm a new address.
			"email_changed": upd.Email != nil,
		})
	}
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var _ = models.User{} // keep the models import meaningful if fields move here later