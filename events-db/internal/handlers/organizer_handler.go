package handlers

import (
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterAsOrganizerHandler allows any authenticated user to self-register as
// an organiser — no admin approval queue, exactly like Meta Business Manager.
// The user's role is upgraded to 'organizer' immediately and they can create
// events and ad campaigns straight away.
//
//	POST /v1/register-organizer
func RegisterAsOrganizerHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		role, _ := c.Get("role")

		if role == models.RoleOrganizer || role == models.RoleAdmin {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Already an organizer or admin"})
			return
		}

		var dto models.CreateOrganizerDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		organizer, err := repository.CreateOrganizer(pool, &dto, userID.(string))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create organizer account"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":   "Organizer account created successfully",
			"organizer": organizer,
			"note":      "You can now create events and ad campaigns. Events require admin approval before going live.",
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

// ── Admin: organizer account management ──────────────────────────────────────
// There is no admin "approval" queue — organizers self-register and are active
// immediately. Admin can suspend or ban an account after the fact.

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
//	Body: { "status": "suspended", "reason": "Repeated policy violations" }
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