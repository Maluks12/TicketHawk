package handlers

import (
	"errors"
	"net/http"

	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ToggleOrganizerFollowHandler follows or unfollows an organiser.
//
//	POST /v1/organisers/:slug/follow
//
// One endpoint for both directions, matching the event follow route: the client
// calls it on every tap without needing to know the current state first.
//
// The slug is resolved through the SAME public gate as the profile, so an
// organiser whose page does not resolve cannot be followed. Otherwise follows
// would accumulate against suspended accounts and surface the moment a
// suspension lifted.
func ToggleOrganizerFollowHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		slug := c.Param("slug")

		organizerID, err := repository.ResolveFollowableOrganizer(pool, slug)
		if err != nil {
			if errors.Is(err, repository.ErrOrganizerProfileNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser not found"})
				return
			}
			utils.InternalError(c, err, "failed to resolve organiser")
			return
		}

		result, err := repository.ToggleOrganizerFollow(pool, userID, organizerID)
		if err != nil {
			utils.InternalError(c, err, "toggle organiser follow")
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// GetOrganizerFollowStatusHandler reports follow state and follower count.
//
//	GET /v1/organisers/:slug/follow
//
// Called on page load so the button renders in the right state. Kept separate
// from the profile endpoint even though the profile now carries both fields,
// because the profile response is cacheable across users and this one is not.
func GetOrganizerFollowStatusHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		slug := c.Param("slug")

		organizerID, err := repository.ResolveFollowableOrganizer(pool, slug)
		if err != nil {
			if errors.Is(err, repository.ErrOrganizerProfileNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "organiser not found"})
				return
			}
			utils.InternalError(c, err, "failed to resolve organiser")
			return
		}

		following, err := repository.IsFollowingOrganizer(pool, userID, organizerID)
		if err != nil {
			utils.InternalError(c, err, "check organiser follow status")
			return
		}

		count, err := repository.GetOrganizerFollowerCount(pool, organizerID)
		if err != nil {
			utils.InternalError(c, err, "organiser follower count")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"following":      following,
			"follower_count": count,
			"organizer_id":   organizerID,
		})
	}
}

// GetFollowedOrganizersHandler lists the organisers the caller follows.
//
//	GET /v1/user/following-organisers
//
// Not paged. A user who follows more than a few hundred organisers does not
// exist yet, and adding a cursor to a list nobody scrolls would be shape
// nobody needs. If that changes it is a keyset on followed_at, same as the
// followed-events feed.
func GetFollowedOrganizersHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		orgs, err := repository.GetFollowedOrganizers(pool, userID)
		if err != nil {
			utils.InternalError(c, err, "failed to list followed organisers")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":  orgs,
			"count": len(orgs),
		})
	}
}