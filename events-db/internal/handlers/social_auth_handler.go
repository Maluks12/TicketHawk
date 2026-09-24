package handlers

import (
	"errors"
	"net/http"
	"strings"

	"events-db/internal/accountguard"
	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// SocialSignInHandler handles Sign in with Google and Sign in with Apple.
//
// ── One endpoint for both signup and login, deliberately ─────────────────────
//
// There is no /social/register vs /social/login split because the client cannot
// know which one it is doing. The user taps one button; whether that provisions
// an account or signs into an existing one is a fact only the database knows.
// Making the client choose would mean a "you already have an account" error on
// what the user experienced as a single tap.
//
// The response reports which happened (`created`) so the UI can differ, but the
// request is identical either way.
func SocialSignInHandler(pool *pgxpool.Pool, cfg *config.Config, verifier *services.OIDCVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.SocialSignInDTO
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		provider := services.SocialProvider(req.Provider)

		// A missing env var should read as "not deployed yet", not as "your
		// account is wrong" — 501 rather than a confusing 401.
		if !verifier.Enabled(provider) {
			log.Error().Str("provider", req.Provider).Msg("social sign-in attempted but provider is not configured")
			c.JSON(http.StatusNotImplemented, gin.H{
				"error": "That sign-in method isn't available right now",
			})
			return
		}

		identity, err := verifier.Verify(c.Request.Context(), provider, req.IDToken, req.Nonce)
		if err != nil {
			// Deliberately vague to the client, detailed in the log. A verify
			// failure is either a bug in our config or someone probing; the
			// user can do nothing useful with the distinction.
			log.Warn().Err(err).Str("provider", req.Provider).Msg("social id token rejected")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Could not verify that sign-in"})
			return
		}

		// Apple returns the name once, outside the token. Use it only to fill
		// blanks — never to override what the token asserted.
		if identity.FirstName == "" {
			identity.FirstName = strings.TrimSpace(req.FirstName)
		}
		if identity.LastName == "" {
			identity.LastName = strings.TrimSpace(req.LastName)
		}

		result, err := repository.ResolveSocialIdentity(pool, repository.SocialIdentityInput{
			Provider:       string(identity.Provider),
			Subject:        identity.Subject,
			Email:          identity.Email,
			EmailVerified:  identity.EmailVerified,
			IsPrivateRelay: identity.IsPrivateRelay,
			FirstName:      identity.FirstName,
			LastName:       identity.LastName,
		})
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrEmailBelongsToDeletedAccount):
				c.JSON(http.StatusForbidden, gin.H{
					"error": "This account has been deleted. Sign up again to create a new one.",
				})
			default:
				log.Error().Err(err).Str("provider", req.Provider).Msg("social sign-in failed")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not complete sign-in"})
			}
			return
		}

		// Same rule as password login: the provider has just proved who this
		// is, so they may learn the account's standing, and a disabled or
		// banned account gets no session. A brand-new account cannot be
		// restricted, so this only ever refuses an existing one.
		if lock, locked := accountguard.LockFor(result.User.ID); locked {
			accountguard.AbortLocked(c, http.StatusForbidden, lock)
			return
		}

		// Same session machinery as password login — one issuer, so refresh
		// token rotation, TTL and revocation behave identically no matter how
		// the user got in.
		session, err := issueSession(pool, cfg, result.User)
		if err != nil {
			log.Error().Err(err).Msg("failed to issue session after social sign-in")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}

		status := http.StatusOK
		if result.Created {
			status = http.StatusCreated
		}

		resp := gin.H{
			"access_token":  session.AccessToken,
			"refresh_token": session.RefreshToken,
			"expires_in":    session.ExpiresIn,
			// Lets the client route a brand-new user differently and know that
			// checkout will need to collect phone number / date of birth.
			"created":    result.Created,
			"linked":     result.Linked,
			"needs_info": result.NeedsInfo,
			"user": gin.H{
				"id":         result.User.ID,
				"email":      result.User.Email,
				"first_name": result.User.FirstName,
				"last_name":  result.User.LastName,
				"role":       result.User.Role,
			},
		}
		// Feature blocks come back beside the tokens, exactly as they do for
		// password login, so the app can show "Your account has been limited".
		if acctStatus, limited := accountguard.LimitedStatus(result.User.ID); limited {
			resp["account_status"] = acctStatus
		}
		c.JSON(status, resp)
	}
}

// ListLinkedIdentitiesHandler backs the profile screen's connected-accounts
// list.
func ListLinkedIdentitiesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		identities, err := repository.ListUserIdentities(pool, userID)
		if err != nil {
			log.Error().Err(err).Msg("failed to list linked identities")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load connected accounts"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": identities})
	}
}

// UnlinkIdentityHandler removes a provider from the signed-in account.
//
// The repository refuses to remove the last sign-in method from a
// password-less account; that error is surfaced verbatim because it is
// actionable ("set a password first").
func UnlinkIdentityHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		provider := c.Param("provider")
		if provider != "google" && provider != "apple" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown provider"})
			return
		}

		if err := repository.UnlinkIdentity(pool, userID, provider); err != nil {
			if strings.Contains(err.Error(), "set a password") {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			log.Error().Err(err).Msg("failed to unlink identity")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not disconnect that account"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Disconnected"})
	}
}