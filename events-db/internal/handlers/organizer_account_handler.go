package handlers

import (
	"errors"
	"net/http"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UpdateOrganizerProfileHandler lets an organiser edit their own account.
//
//	PATCH /v1/organiser/profile
//
// Pairs with the existing GET on the same path. This is the ACCOUNT — business
// name, contact email and phone, website. The public page (description, cover,
// socials, visibility) is a different resource at /organiser/public-profile.
//
// Nothing here was editable before: UpdateOrganizerDTO existed in the model
// with no repository function and no route behind it, so an organiser who
// mistyped their business name at sign-up had no way to correct it.
//
// ── The name change does NOT move the public URL ────────────────────────────
//
// organizers.slug is assigned once by a trigger on insert. A rename leaves it
// alone, so links already shared keep resolving. The response carries the slug
// so the dashboard can say so plainly rather than letting the organiser guess.
func UpdateOrganizerProfileHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		var dto models.UpdateOrganizerAccountDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": utils.FormatValidationErrors(err)})
			return
		}

		org, err := repository.UpdateOrganizerAccount(pool, userID, &dto)
		if err != nil {
			if errors.Is(err, repository.ErrOrganizerNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser profile not found"})
				return
			}
			utils.InternalError(c, err, "failed to update organiser profile")
			return
		}

		c.JSON(http.StatusOK, org)
	}
}