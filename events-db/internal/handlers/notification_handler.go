package handlers

import (
	"net/http"
	"strconv"

	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GetNotificationsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		cursorStr := c.Query("cursor")
		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}
		var cursor *utils.Cursor
		if cursorStr != "" {
			var err error
			if cursor, err = utils.DecodeCursor(cursorStr); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
				return
			}
		}
		notifs, nextCursor, err := repository.GetNotifications(pool, userID, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "get notifications")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"data":        notifs,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

func GetUnreadCountHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		count, err := repository.GetUnreadCount(pool, c.Request.Context(), userID)
		if err != nil {
			utils.InternalError(c, err, "get unread count")
			return
		}
		c.JSON(http.StatusOK, gin.H{"unread_count": count})
	}
}

func MarkReadHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		notifID := c.Param("id")
		if err := repository.MarkAsRead(pool, c.Request.Context(), userID, notifID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "marked as read"})
	}
}

func MarkAllReadHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if err := repository.MarkAllRead(pool, c.Request.Context(), userID); err != nil {
			utils.InternalError(c, err, "mark all read")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "all notifications marked as read"})
	}
}