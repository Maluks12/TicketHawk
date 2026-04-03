package handlers

import (
	//"crypto/sha256"
	//"encoding"
	//"encoding/hex"
	"errors"
	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"
	"fmt"
	"net/http"

	//"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required,min=8,max=128"`
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
	Token string `json:"token" binding:"required,uuid4"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=128"`
}

func CreateUserhandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var registerRequest RegisterRequest

		if err := c.ShouldBindJSON(&registerRequest); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": utils.FormatValidationErrors(err)})
			return
		}

		// if len(registerRequest.Password) < 8 {
		// 	c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 8 characters"})
		// 	return
		// }

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(registerRequest.Password), bcrypt.DefaultCost)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create account"})
			return
		}

		user := &models.User{
			Email:    registerRequest.Email,
			Password: string(hashedPassword),
			Role:     models.RoleAttendee,
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
		c.JSON(http.StatusCreated, createdUser)
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
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginRequest.Password))

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		claims := jwt.MapClaims{
			"user_id": user.ID,
			"email":   user.Email,
			"role":	user.Role,
			"exp":     time.Now().Add(15 * time.Minute).Unix(),
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

		accessToken, err := token.SignedString([]byte(cfg.JWTSecret))

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}

		rawRefresh, hashedRefresh, _ := utils.GenerateResetToken()
		expiresAt := time.Now().Add(7 * 24 * time.Hour)

		if err := repository.CreateRefreshToken(pool, user.ID, hashedRefresh, expiresAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}
		// add refresh tokens to this
		c.JSON(http.StatusOK, gin.H{
			"access_token": accessToken,
			"refresh_token": rawRefresh,
			"expires_in": 900,
		})
	}
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

		rawToken, hashedToken, _:= utils.GenerateResetToken()
		expiresAt := time.Now().Add(30 * time.Minute)

		err = repository.CreatePasswordReset(pool, user.ID, hashedToken, expiresAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create reset token"})
			return
		}

		// TODO: send email
		resetLink := fmt.Sprintf("%s/reset-password?token=%s", cfg.FrontendURL, rawToken)
		if err := emailService.SendPasswordReset(user.Email, resetLink); err != nil {
			// Log the error but don't tell the client — prevents email enumeration
			// and avoids breaking the UX on transient email failures
			log.Warn().Err(err).Str("email", user.Email).Msg("failed to send password reset email")
		}

		c.JSON(http.StatusOK, gin.H{"message": "If the email exists, a reset link was sent"})
	}
}

func ResetPasswordHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context){
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

		err = repository.ResetPassword(pool, tokenHash, passwordHash)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired token"})
			return
		}

			c.JSON(http.StatusOK, gin.H{"message": "password reset successful"})
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
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired refresh token"})
			return
		}

		claims := jwt.MapClaims {
			"user_id": userID,
			"role": role,
			"exp": time.Now().Add(15 * time.Minute).Unix(),		
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		accessToken, err := token.SignedString([]byte(cfg.JWTSecret))

		rawRefresh, hashedRefresh, _ := utils.GenerateResetToken()
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}

		if err := repository.CreateRefreshToken(pool, userID, hashedRefresh, expiresAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}


		c.JSON(http.StatusOK, gin.H{
			"access_tokens": accessToken,
			"refresh_token": rawRefresh,
			"expires_in": 900,
		})
	}
}

func LogoutHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context){
		userID, _ := c.Get("user_id")
		repository.RevokeAllUserTokens(pool, userID. (string))
		c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
	}
}