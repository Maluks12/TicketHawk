package handlers

import (
	"context"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterAsOrganizerHandler allows any authenticated user to self-register as
// an organiser with a single POST and an empty body. All required DTO fields are
// derived from the user's own account so no form is needed on the frontend.
//
//	POST /v1/register-organizer
//	Body: {} - all fields optional; backend fills in sensible defaults.
func RegisterAsOrganizerHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		role, _   := c.Get("role")

		if role == models.RoleOrganizer || role == models.RoleAdmin {
			c.JSON(http.StatusOK, gin.H{"message": "Already an organizer"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Fetch user details to build the DTO.
		var email, firstName, lastName string
		_ = pool.QueryRow(ctx,
			`SELECT email, COALESCE(first_name,''), COALESCE(last_name,'') FROM users WHERE id = $1`,
			userID.(string),
		).Scan(&email, &firstName, &lastName)

		// Parse optional overrides - nothing required.
		var body struct {
			BusinessName  string `json:"business_name"`
			BusinessPhone string `json:"business_phone"`
			Description   string `json:"description"`
		}
		_ = c.ShouldBindJSON(&body)

		// Derive business name from user's own name or email prefix.
		businessName := strings.TrimSpace(body.BusinessName)
		if businessName == "" {
			full := strings.TrimSpace(firstName + " " + lastName)
			if full != "" {
				businessName = full
			} else {
				parts := strings.SplitN(email, "@", 2)
				businessName = parts[0]
			}
		}

		businessPhone := strings.TrimSpace(body.BusinessPhone)
		if businessPhone == "" {
			businessPhone = "N/A"
		}

		// Description must be at least 50 chars per DTO validation.
		description := strings.TrimSpace(body.Description)
		if len(description) < 50 {
			description = "Organizer account for " + businessName +
				". Events are created and managed via the EventsProject organizer dashboard. " +
				"Profile details can be updated at any time from the dashboard settings."
		}

		dto := models.CreateOrganizerDTO{
			BusinessName:  businessName,
			BusinessEmail: email,
			BusinessPhone: businessPhone,
			Description:   description,
		}

		organizer, err := repository.CreateOrganizer(pool, &dto, userID.(string))
		if err != nil {
			// If the organizer row already exists, treat it as success so
			// the frontend can re-login and get the organizer role JWT.
			if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
				c.JSON(http.StatusOK, gin.H{"message": "Already an organizer"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create organizer account"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":   "Organizer account created successfully",
			"organizer": organizer,
		})
	}
}

// GetOrganizerProfileHandler returns the authenticated organiser's own profile.
//
//	GET /v1/organiser/profile
func GetOrganizerProfileHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")

		organizer, err := repository.GetOrganizerByUserID(pool, userID.(string))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Organizer profile not found"})
			return
		}

		c.JSON(http.StatusOK, organizer)
	}
}

// AdminGetOrganizersHandler returns all organiser accounts with optional status filter.
//
//	GET /v1/admin/organizers?status=suspended
func AdminGetOrganizersHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizers, err := repository.GetAllOrganizers(pool, c.Query("status"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch organizers"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"organizers": organizers,
			"total":      len(organizers),
		})
	}
}

// AdminUpdateOrganizerStatusHandler lets an admin suspend, ban, or reinstate an organiser.
//
//	PUT /v1/admin/organizers/:id/status
func AdminUpdateOrganizerStatusHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizerID := c.Param("id")

		var dto models.AdminUpdateOrganizerStatusDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		if dto.Status != models.OrganizerActive &&
			dto.Status != models.OrganizerSuspended &&
			dto.Status != models.OrganizerBanned {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "status must be one of: active, suspended, banned",
			})
			return
		}

		if err := repository.AdminUpdateOrganizerStatus(pool, organizerID, dto.Status, dto.Reason); err != nil {
			if err == repository.ErrOrganizerNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Organizer not found"})
				return
			}
			utils.InternalError(c, err, "failed to update organizer status")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Organizer status updated",
			"status":  dto.Status,
		})
	}
}