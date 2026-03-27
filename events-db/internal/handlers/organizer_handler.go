package handlers

import (
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ApplyOrganizerHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _:= c.Get("user_id")
		role, _:= c.Get("role")

		if role == models.RoleOrganizer || role == models.RoleAdmin {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Already an organizer"})
			return
		}

		var dto models.CreateOrganizerDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		organizer, err := repository.CreateOrganizer(pool, &dto, userID.(string))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit application"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Application submitted successfully",
			"organizer": organizer,
			"note": "Your application will be reviewed by our team within 2-3 business days",
		})
	}
}

func ApproveOrganizerHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		adminID, _ := c.Get("user_id")
		organizerID := c.Param("id")

		err := repository.ApproveOrganizer(pool, organizerID, adminID.(string))
		if err != nil {
			utils.InternalError(c, err, "descriptive-log-message")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Organizer approved successfully",
			"note": "User can now create events",
		})
	}
}

func RejectOrganizerHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		adminID, _:=c.Get("user_id")
		organizerID := c.Param("id")

		var input struct {
			Reason string `json:"reason" binding:"required,min=10"`
		}

		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Rejection reason required"})
			return
		}

		err := repository.RejectOrganizer(pool, organizerID, adminID.(string), input.Reason)
		if err != nil {
			utils.InternalError(c, err, "descriptive-log-message")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Organizer application rejected"})
	}
}

func GetOrganizerApplicationHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c * gin.Context) {
		organizers, err := repository.GetPendingOrganizers(pool)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch applications"})
            return
		}
		c.JSON(http.StatusOK, gin.H{
            "applications": organizers,
            "total": len(organizers),
        })
	}
}

func GetOrganizerProfileHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _:= c.Get("user_id")

		organizer, err := repository.GetOrganizerByUserID(pool, userID.(string))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Organizer profile not found"})
            return
		}

		c.JSON(http.StatusOK, organizer)
	}
}