package handlers

import (
	"errors"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func CreateGuestBookingHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var dto models.CreateGuestBookingDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		event, err := repository.GetEventByID(pool, dto.EventID, nil)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		if event.Status != models.EventPublished {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Event is not available for booking"})
			return
		}

		if event.MaxTicketsPerUser != nil && dto.TicketQuantity > *event.MaxTicketsPerUser {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Exceeds per-person ticket limit",
				"max_tickets_per_user": *event.MaxTicketsPerUser,
			})
			return
		}

		booking, err := repository.CreateGuestBooking(pool, &dto, event)
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

		c.JSON(http.StatusCreated, models.BookingResponse{
			BookingID:        booking.ID,
			BookingNumber:    booking.BookingNumber,
			TotalAmount:      booking.TotalAmount,
			Currency:         booking.Currency,
			ClientToken:      clientToken,
			PaymentRequestID: paymentRequestID,
			ExpiresAt:        booking.CreatedAt.Add(15 * time.Minute),
			IsGuest:          true,
		})
	}
}

func GetGuestBookingHandler(pool *pgxpool.Pool, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookingNumber := c.Param("bookingNumber")
		email := c.Query("email")

		// if bookingNumber == "" {
		// 	c.JSON(http.StatusBadRequest, gin.H{"error": "Booking number is required"})
		// 	return
		// }
		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
			return
		}

		lockKey := "guest:lockout:" + bookingNumber
		count, _ := rdb.Get(c.Request.Context(), lockKey).Int()
		if count >= 5 {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many failed attempts. Try again later"})
			return
		}

		booking, err := repository.GetGuestBooking(pool, bookingNumber, email)
		if err != nil {
			pipe :=rdb.Pipeline()
			pipe.Incr(c.Request.Context(), lockKey)
			pipe.Expire(c.Request.Context(), lockKey, time.Hour)
			pipe.Exec(c.Request.Context())

			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		rdb.Del(c.Request.Context(), lockKey)

		c.JSON(http.StatusOK, booking)
	}
}
func GuestPaymentStatusHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookingNumber := c.Param("bookingNumber")
		email := c.Query("email")

		if bookingNumber == "" || email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Booking number and email are required"})
			return
		}

		booking, err := repository.GetGuestBooking(pool, bookingNumber, email)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		payment, err := paymentService.GetPaymentStatusReconciled(booking.ID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Payment record not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"booking_number":   booking.BookingNumber,
			"payment_status":   payment.Status,
			"payment_method":   payment.PaymentMethod,
			"amount":           payment.Amount,
			"currency":         payment.Currency,
			"transaction_id":   payment.TransactionID,
			"completed_at":     payment.CompletedAt,
			"webhook_received": payment.WebhookReceived,
		})
	}
}

// GET /guest/bookings/:bookingNumber/assignments?email=...
//
// Returns all ticket assignment slots for a guest booking.
// Identity is verified by bookingNumber + email, matching the pattern used by
// GetGuestBookingHandler. The same Redis lockout (5 failed attempts → 1 hour
// block) is applied to prevent enumeration.
func GetGuestTicketAssignmentsHandler(pool *pgxpool.Pool, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookingNumber := c.Param("bookingNumber")
		email := c.Query("email")

		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
			return
		}

		// Redis lockout — same key as GetGuestBookingHandler so failed attempts
		// on either endpoint count toward the same limit.
		lockKey := "guest:lockout:" + bookingNumber
		if rdb != nil {
			count, _ := rdb.Get(c.Request.Context(), lockKey).Int()
			if count >= 5 {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many failed attempts. Try again later"})
				return
			}
		}

		// Verify identity: bookingNumber + email must match a guest booking.
		booking, err := repository.GetGuestBooking(pool, bookingNumber, email)
		if err != nil {
			if rdb != nil {
				pipe := rdb.Pipeline()
				pipe.Incr(c.Request.Context(), lockKey)
				pipe.Expire(c.Request.Context(), lockKey, time.Hour)
				pipe.Exec(c.Request.Context())
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		// Clear lockout on success.
		if rdb != nil {
			rdb.Del(c.Request.Context(), lockKey)
		}

		assignments, err := repository.GetAssignmentsByBookingID(pool, booking.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load assignments"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"data": assignments})
	}
}

func GuestRetryPaymentHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookingNumber := c.Param("bookingNumber")
		email := c.Query("email")

		if bookingNumber == "" || email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Booking number and email are required"})
			return
		}

		booking, err := repository.GetGuestBooking(pool, bookingNumber, email)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		payment, err := paymentService.GetPaymentStatus(booking.ID)
		if err == nil {
			switch payment.Status {
			case "completed", "refunded":
				c.JSON(http.StatusConflict, gin.H{"error": "Cannot retry a completed or refunded payment"})
				return
			}
		}

		clientToken, paymentRequestID, err := paymentService.CreatePaymentIntent(booking)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payment"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"booking_id":         booking.ID,
			"booking_number":     booking.BookingNumber,
			"client_token":       clientToken,
			"payment_request_id": paymentRequestID,
			"amount":             booking.TotalAmount,
			"currency":           booking.Currency,
		})
	}
}