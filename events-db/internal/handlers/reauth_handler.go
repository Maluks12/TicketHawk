package handlers

import (
	"errors"
	"net/http"

	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// ─── Step-up re-authentication ───────────────────────────────────────────────
//
// Two actions in this codebase demand proof of identity beyond a valid access
// token: changing the email address, and deleting the account. Both did that by
// checking a bcrypt password.
//
// Social sign-in breaks that outright. A Google- or Apple-created account has
// password IS NULL, so there is no password to check and no way for the user to
// produce one. Before this helper existed, both endpoints scanned NULL into a
// string, got an opaque pgx error, and returned 500 — meaning a social-only
// user could not change their email and, more seriously, COULD NOT DELETE THEIR
// ACCOUNT. That is an App Store Guideline 5.1.1(v) failure, not just a bug.
//
// The fix is not to drop re-authentication for social accounts. That would make
// a stolen unlocked phone enough to delete someone's account and their tickets.
// The fix is to accept the equivalent proof: a FRESH ID token from a provider
// already linked to the account. Re-running the provider flow is one tap in
// both clients, and it proves the same thing a password does — that the person
// holding the device still controls the identity behind the account.
//
// So: password accounts prove with a password, social accounts prove with a
// fresh token, and an account holding both may use either.

// ReauthCredentials is embedded in the request bodies of step-up endpoints.
type ReauthCredentials struct {
	// Password is used when the account has one.
	Password string `json:"password"`

	// Provider + IDToken are the social equivalent. The token must be freshly
	// minted (the verifier enforces exp/iat) and its subject must already be
	// linked to THIS account — a token for some other Google account proves
	// nothing about this one.
	Provider string `json:"provider"`
	IDToken  string `json:"id_token"`
	Nonce    string `json:"nonce"`
}

// ReauthResult describes why a re-authentication failed, so callers can send
// the right status and code.
type ReauthError struct {
	Status int
	Code   string
	Msg    string
}

func (e *ReauthError) Error() string { return e.Msg }

// Reauthenticate verifies a step-up credential for userID.
//
// Returns nil on success. On failure returns a *ReauthError carrying the exact
// status/code/message to hand back — the codes are stable so the clients can
// branch on them rather than on message text.
func Reauthenticate(
	pool *pgxpool.Pool,
	verifier *services.OIDCVerifier,
	c *gin.Context,
	userID string,
	creds ReauthCredentials,
) error {
	hasPassword, err := repository.HasPassword(pool, c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return &ReauthError{http.StatusNotFound, "user_not_found", "User not found"}
		}
		return &ReauthError{http.StatusInternalServerError, "server_error", "Could not verify your identity"}
	}

	// ── Social proof ─────────────────────────────────────────────────────────
	// Tried first when offered, so an account with BOTH can use either.
	if creds.Provider != "" && creds.IDToken != "" {
		provider := services.SocialProvider(creds.Provider)
		if provider != services.ProviderGoogle && provider != services.ProviderApple {
			return &ReauthError{http.StatusBadRequest, "unknown_provider", "Unknown sign-in provider"}
		}
		if verifier == nil || !verifier.Enabled(provider) {
			return &ReauthError{http.StatusNotImplemented, "provider_unavailable", "That sign-in method isn't available right now"}
		}

		identity, verr := verifier.Verify(c.Request.Context(), provider, creds.IDToken, creds.Nonce)
		if verr != nil {
			return &ReauthError{http.StatusUnauthorized, "reauth_failed", "Could not verify that sign-in"}
		}

		// CRITICAL: the token must belong to an identity ALREADY linked to this
		// account. Without this check, any valid Google token from any account
		// would satisfy the step-up — which would make the whole gate
		// decorative.
		linked, lerr := repository.IdentityBelongsToUser(pool, userID, string(provider), identity.Subject)
		if lerr != nil {
			return &ReauthError{http.StatusInternalServerError, "server_error", "Could not verify your identity"}
		}
		if !linked {
			return &ReauthError{http.StatusUnauthorized, "reauth_wrong_account", "That account isn't connected to this profile"}
		}
		return nil
	}

	// ── Password ─────────────────────────────────────────────────────────────
	if !hasPassword {
		// No password AND no social proof offered. Tell the client exactly what
		// to ask for rather than rejecting a password it cannot obtain.
		return &ReauthError{
			http.StatusBadRequest, "reauth_social_required",
			"Confirm with Google or Apple to continue",
		}
	}

	if creds.Password == "" {
		return &ReauthError{http.StatusBadRequest, "password_required", "Enter your password to continue"}
	}

	hash, err := repository.GetPasswordHash(pool, c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return &ReauthError{http.StatusNotFound, "user_not_found", "User not found"}
		}
		if errors.Is(err, repository.ErrNoPasswordSet) {
			return &ReauthError{http.StatusBadRequest, "reauth_social_required", "Confirm with Google or Apple to continue"}
		}
		return &ReauthError{http.StatusInternalServerError, "server_error", "Could not verify your identity"}
	}

	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(creds.Password)) != nil {
		return &ReauthError{http.StatusUnauthorized, "password_incorrect", "That password is incorrect"}
	}
	return nil
}

// AbortWithReauthError writes a *ReauthError to the response.
func AbortWithReauthError(c *gin.Context, err error) {
	var re *ReauthError
	if errors.As(err, &re) {
		c.JSON(re.Status, gin.H{"error": re.Msg, "code": re.Code})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not verify your identity"})
}