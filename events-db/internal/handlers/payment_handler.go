package handlers

import (
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateStitchPayment(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDValue, exists := c.Get("user_id")

		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		userID := userIDValue.(string)

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

		if booking.UserID == nil || *booking.UserID != userID {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		if  booking.PaymentStatus == "completed" {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Payment already completed",
			})
			return
		}

		expiresAt := booking.CreatedAt.Add(15 * time.Minute)
		if time.Now().After(expiresAt) && booking.BookingStatus == models.BookingPending {
			c.JSON(http.StatusGone, gin.H {
				"error":      "Booking has expired",
				"expired_at": expiresAt,
			})
			return
		}

		clientToken, paymentRequestID, err := paymentService.CreatePaymentIntent(booking)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to create payment",
			})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"booking_id":         booking.ID,
			"booking_number":     booking.BookingNumber,
			"client_token":       clientToken,
			"payment_request_id": paymentRequestID,
			"amount":             booking.TotalAmount,
			"currency":           booking.Currency,
		})
	}
}

func GetPaymentStatusHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDValue, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		userID := userIDValue.(string)
		bookingID := c.Param("id")

		if _, err := uuid.Parse(bookingID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid booking ID"})
			return
		}

		// Verify the booking belongs to the authenticated user
		booking, err := repository.GetBookingByID(pool, bookingID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		if booking.UserID == nil || *booking.UserID != userID {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		payment, err := paymentService.GetPaymentStatus(bookingID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Payment record not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"booking_id":                bookingID,
			"booking_number":            booking.BookingNumber,
			"payment_status":            payment.Status,
			"payment_method":            payment.PaymentMethod,
			"amount":                    payment.Amount,
			"currency":                  payment.Currency,
			"stitch_payment_url":        payment.StitchPaymentURL,
			"stitch_payment_request_id": payment.StitchPaymentRequestID,
			"transaction_id":            payment.TransactionID,
			"completed_at":              payment.CompletedAt,
			"webhook_received":          payment.WebhookReceived,
		})
	}
}

func RetryPaymentHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDValue, exists := c.Get("user_id")

		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		userID := userIDValue.(string)

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

		if booking.UserID == nil || *booking.UserID != userID {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		// Only allow retrying pending/failed bookings
		payment, err := paymentService.GetPaymentStatus(bookingID)
		if err == nil {
			switch payment.Status {
			case "completed", "refunded":
				c.JSON(http.StatusConflict, gin.H{
					"error": "Cannot retry a completed or refunded payment",
				})
				return
			}
		}

		expiresAt := booking.CreatedAt.Add(15 * time.Minute)
		if time.Now().After(expiresAt) && booking.BookingStatus == models.BookingPending {
			c.JSON(http.StatusGone, gin.H {
				"error":      "Booking has expired",
				"expired_at": expiresAt,
			})
			return
		}

		clientToken, paymentRequestID, err := paymentService.CreatePaymentIntent(booking)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payment"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"booking_id":         bookingID,
			"booking_number":     booking.BookingNumber,
			"client_token":       clientToken,
			"payment_request_id": paymentRequestID,
			"amount":             booking.TotalAmount,
			"currency":           booking.Currency,
		})
	}
}
