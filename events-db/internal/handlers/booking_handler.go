package handlers

import (
	"errors"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)



func CreateBookingHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
	return func (c *gin.Context) {
		userID, exists := c.Get("user_id")

		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		var dto models.CreateBookingDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Validation",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		event, err := repository.GetEventByID(pool, dto.EventID, nil)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		// Add this — sold-out check
		if event.MaxCapacity != nil && event.TicketsSold >= *event.MaxCapacity {
			c.JSON(http.StatusConflict, gin.H{"error": "This event is sold out"})
			return
		}

		if event.Status != models.EventPublished {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Event is not available for booking"})
			return
		}
		
		if event.Status != models.EventPublished {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Event is not available for booking"})
			return
		}

		booking, err := repository.CreateBooking(pool, &dto, userID.(string), event)
		if err != nil {
			if errors.Is(err, repository.ErrInsufficientCapacity) {
				c.JSON(http.StatusConflict, gin.H{
					"error":             "Insufficient tickets available",
					"available_tickets": event.AvailableTickets,
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create booking"})
			return
		}

		clientToken, paymentRequestID, err := paymentService.CreatePaymentIntent(booking)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize payment"})
			return
		}

		response := models.BookingResponse {
			BookingID: booking.ID,
			BookingNumber: booking.BookingNumber,
			TotalAmount: booking.TotalAmount,
			Currency: booking.Currency,
			ClientToken: clientToken,
			PaymentRequestID: paymentRequestID,
			ExpiresAt: booking.CreatedAt.Add(15*time.Minute),
		}

		c.JSON(http.StatusCreated, response)
	}
}

func GetUserBookingsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _:= c.Get("user_id")

		limit := 20
        if l := c.Query("limit"); l != "" {
            if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
                limit = parsed
            }
        }

		cursorStr := c.Query("cursor")

		var cursor *utils.Cursor
		if cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		bookings, nextCursor, err := repository.GetUserBookings(pool, userID.(string), limit, cursor,)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get bookings"})
			return
		}

		c.JSON(200, gin.H{
			"data":        bookings,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

func GetBookingDetailsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		bookingID := c.Param("id")

		if _, err := uuid.Parse(bookingID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid booking ID"})
			return
		}

		booking, err := repository.GetBookingByID(pool, bookingID)

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		if booking.UserID == nil || *booking.UserID != userID.(string) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		c.JSON(http.StatusOK, booking)
	}
}

func RequestRefundHandler(pool *pgxpool.Pool, paymentService *services.PaymentService, emailService *services.EmailService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		bookingID := c.Param("id")

		var input struct {
			Reason string `json:"reason" binding:"required,min=10,max=500"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed"})
			return
		}

		booking, err := repository.GetBookingByID(pool, bookingID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}
		if booking.UserID == nil || *booking.UserID != userID.(string) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "You do not have permission to refund this booking"})
			return
		}

		event, err := repository.GetEventByID(pool, booking.EventID, nil)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		cancelledAssignees, err := repository.RequestRefundWithCascade(pool, bookingID, userID.(string), input.Reason)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := paymentService.ProcessRefund(bookingID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process payment refund"})
			return
		}

		if len(cancelledAssignees) > 0 {
			go func() {
				for _, assignee := range cancelledAssignees {
					if err := emailService.SendTicketInvalidationNotice(
						assignee.Email,
						assignee.Name,
						event.Title,
						event.StartTime.Format("Monday, 2 January 2006"),
						booking.BookingNumber,
					); err != nil {
						_ = err
					}
				}
			}()
		}

		c.JSON(http.StatusOK, gin.H{
			"message":            "Refund processed successfully",
			"refund_status":      "processing",
			"assignees_notified": len(cancelledAssignees),
		})
	}
}

func VerifyQRCodeHandler(pool *pgxpool.Pool, qrService *services.QRService) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID, _ := c.Get("user_id")
        role, _ := c.Get("role")

        if role != models.RoleOrganizer && role != models.RoleAdmin {
            c.JSON(http.StatusForbidden, gin.H{"error": "Organizer access required"})
            return
        }

        var input struct {
            QRData string `json:"qr_data" binding:"required"`
        }

        if err := c.ShouldBindJSON(&input); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
            return
        }

        // Verify QR code
        payload, err := qrService.VerifyQRCode(input.QRData)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{
                "valid": false,
                "error": err.Error(),
            })
            return
        }

        // Get booking
        booking, err := repository.GetBookingByID(pool, payload.BookingID)
        if err != nil {
            c.JSON(http.StatusNotFound, gin.H{
                "valid": false,
                "error": "Booking not found",
            })
            return
        }

        // Get event (needed for both ownership check and response)
        event, err := repository.GetEventByID(pool, booking.EventID, nil)
        if err != nil {
            c.JSON(http.StatusNotFound, gin.H{
                "valid": false,
                "error": "Event not found",
            })
            return
        }

        // Organizers can only verify tickets for their own events
        // Admins can verify any event
        if role == models.RoleOrganizer {
            if event.OrganizerID != userID.(string) {
                c.JSON(http.StatusForbidden, gin.H{
                    "valid": false,
                    "error": "You can only verify tickets for your own events",
                })
                return
            }
        }

        // QR code is valid and organizer has permission
        c.JSON(http.StatusOK, gin.H{
            "valid":           true,
            "booking_number":  booking.BookingNumber,
            "attendee_name":   fmt.Sprintf("%s %s", *booking.AttendeeFirstName, *booking.AttendeeLastName),
            "ticket_quantity": booking.TicketQuantity,
            "event_title":     event.Title,  // ← Use event, not booking.Event
        })
    }
}