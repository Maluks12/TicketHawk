package middleware

import (
	"events-db/internal/accountguard"
	"events-db/internal/config"
	"events-db/internal/models"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// jwtClaims holds the fields extracted from a valid token.
type jwtClaims struct {
	UserID string
	Role   string
}

// FIX 2: parseJWTClaims extracts and validates claims from a raw Bearer token
// string. Previously this logic was duplicated verbatim in both AuthMiddleware
// and OptionalAuth (~40 lines each). Now it lives in one place.
//
// Returns an error if the token is missing, malformed, uses a wrong signing
// algorithm, is expired, or is missing required claims.
func parseJWTClaims(authHeader string, secret string) (*jwtClaims, error) {
	if authHeader == "" {
		return nil, fmt.Errorf("authorization header required")
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == "" || tokenString == authHeader {
		return nil, fmt.Errorf("invalid authorization header format")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid or expired token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().After(time.Unix(int64(exp), 0)) {
			return nil, fmt.Errorf("token has expired")
		}
	}

	userID, ok := claims["user_id"].(string)
	if !ok || userID == "" {
		return nil, fmt.Errorf("missing user_id claim")
	}

	role, ok := claims["role"].(string)
	if !ok || role == "" {
		return nil, fmt.Errorf("missing role claim")
	}

	return &jwtClaims{UserID: userID, Role: role}, nil
}

// AuthMiddleware requires a valid JWT. Aborts with 401 if missing or invalid,
// or if a master admin has disabled or banned the account.
func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := parseJWTClaims(c.GetHeader("Authorization"), cfg.JWTSecret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		// A disabled or banned account is signed out on its very next request,
		// not whenever its 15-minute token happens to expire. The check is an
		// in-memory lookup (see internal/accountguard), not a query.
		//
		// 401, not 403, on purpose. Every client answers a 401 by refreshing,
		// the refresh is refused too (the restriction revoked every refresh
		// token), and a refused refresh is how every app already ends a session.
		// So app builds that predate the account_status payload still sign the
		// account out.
		if lock, locked := accountguard.LockFor(claims.UserID); locked {
			accountguard.AbortLocked(c, http.StatusUnauthorized, lock)
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// OrganizerOnlyMiddleware requires the caller to hold the organizer or admin role.
func OrganizerOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		role, ok := roleVal.(string)
		if !ok || (role != string(models.RoleOrganizer) && role != string(models.RoleAdmin)) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Organizer access required",
				"note":  "Apply to become an organizer at /api/v1/user/apply-organizer",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminOnlyMiddleware requires the caller to hold the admin role.
func AdminOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}

		role, ok := roleVal.(string)
		if !ok || role != string(models.RoleAdmin) {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// OptionalAuth runs the JWT check but never aborts.
// If a valid token is present, user_id and role are set in the context exactly
// as AuthMiddleware does. If the token is absent or invalid the request
// continues as a guest — handlers check c.Get("user_id") to distinguish.
func OptionalAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := parseJWTClaims(c.GetHeader("Authorization"), cfg.JWTSecret)
		if err != nil {
			// Not an error for optional auth — just continue as guest.
			c.Next()
			return
		}

		// The one case where optional auth DOES refuse: a valid token for an
		// account that has been disabled or banned. Carrying on as a guest would
		// leave the app showing a signed-in user who can no longer do anything.
		// The 401 makes it sign out, and it browses as a guest from then on.
		if lock, locked := accountguard.LockFor(claims.UserID); locked {
			accountguard.AbortLocked(c, http.StatusUnauthorized, lock)
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}