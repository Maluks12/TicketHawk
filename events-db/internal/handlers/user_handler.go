package handlers

import (
	"errors"
	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

// dummyPasswordHash is compared against when a login email doesn't exist.
//
// FIX 3 (timing side-channel): previously a login for an unknown email
// returned 401 in ~1ms while a known email took ~100ms (one bcrypt compare).
// An attacker could enumerate registered emails by timing responses. Now both
// paths cost one bcrypt compare, so they're indistinguishable.
var dummyPasswordHash []byte

func init() {
	var err error
	dummyPasswordHash, err = bcrypt.GenerateFromPassword(
		[]byte("dummy-password-for-timing-equalization"),
		bcrypt.DefaultCost,
	)
	if err != nil {
		panic("failed to generate dummy bcrypt hash: " + err.Error())
	}
}

type RegisterRequest struct {
	Email       string  `json:"email"        binding:"required,email,max=255"`
	Password    string  `json:"password"     binding:"required,strong_password,max=128"`
	FirstName   string  `json:"first_name"   binding:"required,max=100"`
	LastName    string  `json:"last_name"    binding:"required,max=100"`
	PhoneNumber string  `json:"phone_number" binding:"required,max=20"`

	// DateOfBirth is deliberately absent.
	//
	// It used to be here as an optional *string, and it was never read: neither
	// this handler nor repository.CreateUser ever touched it, so every signup
	// since launch has collected a date of birth and thrown it away. Removing
	// the field makes the API honest about that.
	//
	// Age now belongs where it is actually load-bearing — at checkout for
	// age-restricted events, alongside the phone number — rather than as a
	// seventh field on a signup form. users.date_of_birth stays nullable and
	// keeps working for the accounts that set it via profile edit.
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required,min=8,max=128"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type PasswordResetRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required,uuid4"`
	NewPassword string `json:"new_password" binding:"required,strong_password,max=128"`
}

func CreateUserhandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RegisterRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": utils.FormatValidationErrors(err)})
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create account"})
			return
		}

		user := &models.User{
			Email:       req.Email,
			Password:    string(hashedPassword),
			Role:        models.RoleAttendee,
			FirstName:   &req.FirstName,
			LastName:    &req.LastName,
			PhoneNumber: &req.PhoneNumber,
		}

		createdUser, err := repository.CreateUser(pool, user)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Email already registered"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create account"})
			return
		}

		// Return the created user (frontend will call login separately to get tokens)
		c.JSON(http.StatusCreated, gin.H{
			"id":         createdUser.ID,
			"email":      createdUser.Email,
			"first_name": req.FirstName,
			"last_name":  req.LastName,
			"role":       createdUser.Role,
		})
	}
}

func LoginHandler(pool *pgxpool.Pool, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var loginRequest LoginRequest

		if err := c.ShouldBindJSON(&loginRequest); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": utils.FormatValidationErrors(err)})
			return
		}

		user, err := repository.GetUserByEmail(pool, loginRequest.Email)

		if err != nil {
			// FIX 3: burn the same bcrypt cost as the known-email path so
			// response timing doesn't reveal whether the email exists.
			_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(loginRequest.Password))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		// A social-only account has password IS NULL (migration 000075). Guard
		// BEFORE bcrypt: an empty hash makes CompareHashAndPassword return an
		// error today, so this is defence in depth rather than a live bug — but
		// "the comparison happens to fail" is not something to rely on for an
		// authentication decision. Burn the dummy compare so the timing still
		// matches every other failure path.
		if user.Password == "" {
			_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(loginRequest.Password))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "This account uses Google or Apple sign-in. Use that button, or reset your password to set one.",
			})
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginRequest.Password))

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		session, err := issueSession(pool, cfg, user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"access_token":  session.AccessToken,
			"refresh_token": session.RefreshToken,
			"expires_in":    session.ExpiresIn,
		})
	}
}

// ─── Shared session issuance ────────────────────────────────────────────────
//
// Password login and social sign-in both land here, so there is exactly one
// place where an access token is minted and a refresh token is persisted.
// Before this existed the logic was inline in LoginHandler, and adding a second
// entry point would have meant a second copy — which is how a refresh-token TTL
// or a claim set silently drifts between two ways of signing in.

type issuedSession struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

const accessTokenTTL = 15 * time.Minute

func issueSession(pool *pgxpool.Pool, cfg *config.Config, user *models.User) (*issuedSession, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    user.Role,
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(accessTokenTTL).Unix(),
	}

	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return nil, fmt.Errorf("issueSession/sign: %w", err)
	}

	rawRefresh, hashedRefresh, err := utils.GenerateResetToken()
	if err != nil {
		return nil, fmt.Errorf("issueSession/generate: %w", err)
	}

	if err := repository.CreateRefreshToken(pool, user.ID, hashedRefresh, time.Now().Add(refreshTokenTTL)); err != nil {
		return nil, fmt.Errorf("issueSession/persist: %w", err)
	}

	return &issuedSession{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		ExpiresIn:    int(accessTokenTTL.Seconds()),
	}, nil
}

func RequestPasswordResetHandler(pool *pgxpool.Pool, emailService *services.EmailService, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input PasswordResetRequest

		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": utils.FormatValidationErrors(err)})
			return
		}

		user, err := repository.GetUserByEmail(pool, input.Email)

		if err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "If the email exists, a reset link has been sent"})
			return
		}

		rawToken, hashedToken, err := utils.GenerateResetToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create reset token"})
			return
		}
		expiresAt := time.Now().Add(30 * time.Minute)

		err = repository.CreatePasswordReset(pool, user.ID, hashedToken, expiresAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create reset token"})
			return
		}

		resetLink := fmt.Sprintf("%s/reset-password?token=%s", cfg.FrontendURLs[0], rawToken)
		if err := emailService.SendPasswordReset(user.Email, resetLink); err != nil {
			// Log the error but don't tell the client — prevents email enumeration
			// and avoids breaking the UX on transient email failures
			log.Warn().Err(err).Str("email", user.Email).Msg("failed to send password reset email")
		}

		c.JSON(http.StatusOK, gin.H{"message": "If the email exists, a reset link was sent"})
	}
}

func ResetPasswordHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ResetPasswordRequest

		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": utils.FormatValidationErrors(err)})
			return
		}

		tokenHash := utils.HashResetToken(input.Token)

		passwordHash, err := utils.HashPassword(input.NewPassword)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		// FIX 1: ResetPassword now also revokes every refresh token the user
		// holds (see repository.ResetPassword), so a compromised session
		// can't outlive a password reset.
		err = repository.ResetPassword(pool, tokenHash, passwordHash)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "password reset successful. please log in again"})
	}
}

func DeleteUserHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		if _, err := uuid.Parse(id); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ID"})
			return
		}

		err := repository.DeleteUserByID(pool, id)
		if err != nil {
			if errors.Is(err, repository.ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
	}
}

func TestProtectedHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")

		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "user_id not found in context"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "Protected route accessed successfully!",
			"user_id": userID,
		})
	}
}

// refreshTokenTTL is how long a session survives WITHOUT being used.
//
// It was 7 days. Because the TTL is reset on every rotation, that is fine for
// somebody who opens the app daily — and a guaranteed logout for everyone who
// buys a ticket and then doesn't open the app again until the event three
// weeks later. Which is the normal way people use a ticketing app.
//
// 90 days, still sliding: every refresh pushes the deadline out again, so an
// active user is never signed out at all, and an abandoned session still ages
// out on its own. Revocation is unaffected — logout, password reset and reuse
// detection all revoke immediately regardless of the TTL.
const refreshTokenTTL = 90 * 24 * time.Hour

func RefreshTokenHandler(pool *pgxpool.Pool, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			RefreshToken string `json:"refresh_token" binding:"required"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token required"})
			return
		}

		tokenHash := utils.HashResetToken(input.RefreshToken)
		userID, role, err := repository.GetAndRevokeRefreshToken(pool, tokenHash)
		if err != nil {
			// FIX 2: a replayed (already-rotated) token means the whole
			// session family was just revoked. Log it loudly — this is a
			// strong signal of a stolen token — but keep the client-facing
			// message generic.
			if errors.Is(err, repository.ErrRefreshTokenReused) {
				log.Warn().
					Str("ip", c.ClientIP()).
					Msg("refresh token reuse detected — all user sessions revoked")
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired refresh token"})
			return
		}

		claims := jwt.MapClaims{
			"user_id": userID,
			"role":    role,
			"iat":     time.Now().Unix(),
			"exp":     time.Now().Add(15 * time.Minute).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		accessToken, err := token.SignedString([]byte(cfg.JWTSecret))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}

		rawRefresh, hashedRefresh, err := utils.GenerateResetToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}
		expiresAt := time.Now().Add(refreshTokenTTL)

		if err := repository.CreateRefreshToken(pool, userID, hashedRefresh, expiresAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"access_token":  accessToken,
			"refresh_token": rawRefresh,
			"expires_in":    900,
		})
	}
}

func LogoutHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// FIX 5: safe type assertion + error handling. Previously the
		// assertion could panic and a failed revoke was silently reported
		// as a successful logout.
		userIDVal, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}
		userID, ok := userIDVal.(string)
		if !ok || userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		// SCOPED LOGOUT: if the client tells us which refresh token it holds,
		// revoke only that session — signing out on your phone should not
		// log out your laptop and every open tab (which is what the old
		// revoke-everything behaviour did, and a big source of "why do I
		// keep getting logged out" during multi-device use). Clients that
		// don't send a token (e.g. older app builds) keep the old
		// revoke-all behaviour as a safe fallback.
		var input struct {
			RefreshToken string `json:"refresh_token"`
		}
		_ = c.ShouldBindJSON(&input) // body is optional

		if input.RefreshToken != "" {
			revoked, err := repository.RevokeRefreshTokenByHash(pool, userID, utils.HashResetToken(input.RefreshToken))
			if err != nil {
				log.Error().Err(err).Str("user_id", userID).Msg("logout: failed to revoke refresh token")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to log out"})
				return
			}
			if revoked {
				c.JSON(http.StatusOK, gin.H{"message": "Logged out"})
				return
			}
			// Token didn't match a live session for this user (already
			// rotated, or stale) — fall through to revoke-all so logout
			// always leaves the account in a signed-out state.
		}

		if err := repository.RevokeAllUserTokens(pool, userID, "logout"); err != nil {
			log.Error().Err(err).Str("user_id", userID).Msg("logout: failed to revoke refresh tokens")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to log out"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
	}
}