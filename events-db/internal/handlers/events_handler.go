package handlers

import (
	// "context"
	"context"
	// "encoding/base64"
	// "encoding/json"
	"errors"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/utils"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	//"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// type CreateEventInput struct {
// 	Title         string    `json:"title" binding:"required"`
// 	Descript      string    `json:"descript"`
// 	CoverImageURL string    `json:"cover_image_url"`
// 	StartTime     time.Time `json:"start_time" binding:"required"`
// 	EndTime       time.Time `json:"end_time" binding:"required"`
// 	Views         int       `json:"views"`
// 	IsFeatured    bool      `json:"is_featured"`
// 	ExpiresAt     time.Time `json:"expires_at" binding:"required"`
// 	VenueName     string    `json:"venue_name" binding:"required"`
// 	City          string    `json:"city" binding:"required"`
// 	Address       string    `json:"address" binding:"required"`
// 	Category      string    `json:"category" binding:"required"`
// 	FeaturedUntil *time.Time `json:"featured_until"`
// }

// type UpdateEventInput struct {
// 	Title         *string    `json:"title"`
// 	Descript      *string    `json:"descript"`
// 	CoverImageURL *string    `json:"cover_image_url"`
// 	StartTime     *time.Time `json:"start_time"`
// 	EndTime       *time.Time `json:"end_time"`
// 	Views         *int       `json:"views"`
// 	IsFeatured    *bool      `json:"is_featured"`
// 	ExpiresAt     *time.Time `json:"expires_at"`
// 	VenueName     *string    `json:"venue_name"`
// 	City          *string    `json:"city"`
// 	Address       *string    `json:"address"`
// 	Category      *string    `json:"category"`
// 	FeaturedUntil *time.Time `json:"featured_until"`
// }

func CreateEventHandlers(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _:= c.Get("user_id")
		role, _:= c.Get("role")

		if role != models.RoleOrganizer && role != models.RoleAdmin {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Only organizers can create events",
				"note": "Apply to become an organizer at /api/v1/user/apply-organizer",
			})
			return
		}

		var dto models.CreateEventDTO

		if err := c.ShouldBindJSON(&dto); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{
				"error": "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}


		if dto.EndTime.Before(dto.StartTime) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "End time must be after start time"})
			return 
		}

		// if dto. && input.FeaturedUntil == nil {
		// 	c.JSON(http.StatusBadRequest, gin.H{
		// 		"error": "featured_until is required when is_featured is true",
		// 	})
		// 	return
		// }

		// var categoryID int
		// err := pool.QueryRow(
		// 	context.Background(),
		// 	"SELECT id FROM categories WHERE name = $1",
		// 	input.Category,
		// ).Scan(&categoryID)
		// if err != nil {
		// 	c.JSON(http.StatusBadRequest, gin.H{
		// 		"error": "Invalid category",
		// 	})
		// 	return
		// }

		event, err := repository.CreateEvent(pool, &dto, userID.(string))

		if err != nil {
			utils.InternalError(c, err, "descriptive-log-message")
		}
		 c.JSON(http.StatusCreated, gin.H{
			"message": "Event created as draft",
			"event": event,
			"note": "Publish the event to make it visible to attendees",
		})
	}
} // fixed

func DeleteEventHandlers(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID,_ := c.Get("user_id")
		eventID := c.Param("id")

		err := repository.DeleteEvents(pool, eventID, userID.(string))

		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
				return
			}
			utils.InternalError(c, err, "descriptive-log-message")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Event deleted successfully"})

	}
} // fixed

func TrendingEventHandlers(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
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
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		events, nextCursor, err := repository.TrendingEvents(pool, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "descriptive-log-message")
			return
		}
		c.JSON(200, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
} // fixed

func UpdateEventHandlers(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID ,_ := c.Get("user_id")
		eventID := c.Param("id")

		var dto models.UpdateEventDTO

		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		// var categoryID *int
		// if input.Category != nil {
		// 	var id int
		// 	err := pool.QueryRow(
		// 		context.Background(),
		// 		"SELECT id FROM categories WHERE name = $1",
		// 		*input.Category,
		// 	).Scan(&id)

		// 	if err != nil {
		// 		c.JSON(http.StatusBadRequest, gin.H{
		// 			"error": "invalid category",
		// 		})
		// 		return
		// 	}
		// 	categoryID = &id
		// }

		// if input.IsFeatured != nil {
		// 	if *input.IsFeatured {
				
		// 		if input.FeaturedUntil == nil {
		// 			c.JSON(http.StatusBadRequest, gin.H{
		// 				"error": "featured_until is required when is_featured is true",
		// 			})
		// 			return
		// 		}
		// 	} else {
				
		// 		input.FeaturedUntil = nil
		// 	}
		// }

		// // fix this as i can update without changing anything
		// if input.Title == nil && input.Descript == nil && input.StartTime == nil && input.EndTime == nil && input.CoverImageURL == nil && input.Views == nil && input.IsFeatured == nil && input.ExpiresAt == nil && input.VenueName == nil && input.City == nil && input.Address == nil && input.Category == nil && input.FeaturedUntil == nil {
		// 	c.JSON(http.StatusBadRequest, gin.H{"error": "At least one field must be provided"})
		// 	return
		// }

		event, err := repository.UpdateEvents(pool, eventID, &dto, userID.(string))

		if err != nil {
			utils.InternalError(c, err, "descriptive-log-message")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Event updated successfully",
			"event": event,
		})
	}
} // fixed

func GetEventByIDHandlers(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")

		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		var userID *string 
		if uid, exists := c.Get("user_id"); exists {
			uidStr := uid.(string)
			userID = &uidStr
		}

	_ = repository.IncrementViews(pool, eventID)

	event,err := repository.GetEventByID(pool, eventID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	c.JSON(http.StatusOK, event)
	}
} // fixed

func GetAllEventsHandlers(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
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
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		events, nextCursor, err := repository.GetAllEvents(pool, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "descriptive-log-message")
			return
		}
		c.JSON(200, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
} // fixed

func FeaturedEventsHandlers(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cursorStr := c.Query("cursor")

		limit := 12
        if l := c.Query("limit"); l != "" {
            if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
                limit = parsed
            }
        }

		var cursor *utils.Cursor
		if cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		events, nextCursor, err := repository.FeaturedEvents(pool, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "descriptive-log-message")
			return
		}
		c.JSON(200, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

func ActiveEventsHandlers(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
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
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		events, nextCursor, err := repository.ActiveEvents(pool, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "descriptive-log-message")
			return
		}
		c.JSON(200, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
} // potentially fixed

func GetEventCategory(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
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
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		CategoryID, err := strconv.Atoi(id)

		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Category ID"})
			return
		}

		events, nextCursor, err := repository.GetEventBycategory(pool, CategoryID, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "descriptive-log-message")
			return
		}
		c.JSON(200, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
} // potentially fixed

func PublishEventHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		eventID := c.Param("id")

		err := repository.PublishEvent(pool, eventID, userID.(string))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Event published successfully",
			"note": "Event is now visible to all users",
		})
	}
} // new

func SearchEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		cursorStr := c.Query("cursor")

		limit := 20
        if l := c.Query("limit"); l != "" {
            if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
                limit = parsed
            }
        }

		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "search query is required",
			})
			return
		}

		var cursor *utils.Cursor
		if cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "invalid cursor",
				})
				return
			}
		}

		events, nextCursor, err := repository.SearchEvents(pool, query, limit, cursor)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
} // correct

func CancelEventHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _:= c.Get("user_id")
		eventID := c.Param("id")

		var input struct {
			Reason string `json:"reason" binding:"required,min=10"`
		}

		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cancellation reason required"})
			return
		}

		err := repository.CancelEvent(pool, eventID, input.Reason, userID.(string))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Event cancelled successfully",
			"note": "All attendees will be automatically refunded",
		})
	}
} // new

func GetOrganizerEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 5 *time.Second)
		defer cancel()

		organizerID, _ := c.Get("user_id")
		status := c.DefaultQuery("status", "all")

		query := `
			SELECT id, title, description, start_time, status, tickets_sold,
			max_capacity, price, currency, created_at
			FROM events
			WHERE organizer_id = $1
		`

		args := []interface{}{organizerID}
		if status != "all" {
			query += `AND status = $2`
			args = append(args, status)
		}

		query += `ORDER BY start_time DESC`

		rows, err := pool.Query(ctx, query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch events"})
			return
		}

		defer rows.Close()

		events := []map[string]interface{}{}
		for rows.Next() {
			var id, title, description, status, currency string
			var startTime, createdAt time.Time
			var ticketsSold int
			var maxCapacity *int
			var price float64

			err = rows.Scan(
				&id, &title, 
				&description, 
				&startTime, 
				&status, 
				&ticketsSold, 
				&maxCapacity, 
				&price, 
				&currency, 
				&createdAt,
			)
			if err != nil {
				continue
			}

			event := map[string]interface{}{
				"id": id,
				"title": title,
				"description": description,
				"start_time": startTime,
				"status": status,
				"tickets_sold": ticketsSold,
				"max_capacity": maxCapacity,
				"price": price,
				"currency": currency,
				"created_at": createdAt,
			}
			events = append(events, event)
		}

		 c.JSON(http.StatusOK, gin.H{
            "events": events,
            "total": len(events),
        })
	}
} // Need to test and add cusor pagination potentially