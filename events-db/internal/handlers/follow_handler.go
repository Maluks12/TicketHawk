package handlers

import (
	"net/http"
	"strconv"

	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ToggleFollowHandler adds or removes a follow for the authenticated user on
// the given event. A single endpoint handles both directions so the client
// just calls it on every tap — no need to know the current state first.
//
//	POST /events/:id/follow
//
// Response:
//
//	{ "following": true, "follow_count": 42, "event_id": "..." }
func ToggleFollowHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		eventID := c.Param("id")

		if eventID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "event ID required"})
			return
		}

		result, err := repository.ToggleFollow(pool, userID, eventID)
		if err != nil {
			utils.InternalError(c, err, "toggle follow")
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// GetFollowStatusHandler reports whether the authenticated user is currently
// following an event, plus the total follow count. Called on initial page load
// so the heart/bookmark icon renders in the correct state.
//
//	GET /events/:id/follow
//
// Response:
//
//	{ "following": false, "follow_count": 41, "event_id": "..." }
func GetFollowStatusHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		eventID := c.Param("id")

		following, err := repository.IsFollowing(pool, userID, eventID)
		if err != nil {
			utils.InternalError(c, err, "check follow status")
			return
		}

		count, err := repository.GetFollowCount(pool, eventID)
		if err != nil {
			utils.InternalError(c, err, "get follow count")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"following":    following,
			"follow_count": count,
			"event_id":     eventID,
		})
	}
}

// GetFollowedEventsHandler returns a cursor-paginated list of events the
// authenticated user is following, most recently followed first. This powers
// the "Following" / "Saved Events" page.
//
//	GET /user/following?limit=20&cursor=...
//
// Response:
//
//	{
//	  "data": [ { FollowedEventRow... }, ... ],
//	  "next_cursor": "...",
//	  "total_following": 12
//	}
func GetFollowedEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		var cursor *utils.Cursor
		if cursorStr := c.Query("cursor"); cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
				return
			}
		}

		events, nextCursor, err := repository.GetFollowedEvents(pool, userID, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "get followed events")
			return
		}

		var encodedCursor interface{}
		if nextCursor != nil {
			encodedCursor = utils.EncodeCursor(nextCursor)
		} else {
			encodedCursor = nil
		}

		c.JSON(http.StatusOK, gin.H{
			"data":        events,
			"next_cursor": encodedCursor,
		})
	}
}