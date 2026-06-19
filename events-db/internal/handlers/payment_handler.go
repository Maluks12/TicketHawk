package handlers

import (
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// parsePaymentMethod reads the requested method from the `method` query param
// (or JSON body field), defaulting to card. Card and Apple Pay go to Paystack;
// PayPal goes to PayPal.
func parsePaymentMethod(c *gin.Context) models.PaymentMethod {
	m := c.Query("method")
	if m == "" {
		var body struct {
			Method string `json:"method"`
		}
		_ = c.ShouldBindJSON(&body)
		m = body.Method
	}
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "apple_pay", "applepay":
		return models.PaymentApplePay
	case "paypal":
		return models.PaymentPayPal
	default:
		return models.PaymentCreditCard
	}
}

// CreatePaymentHandler starts a hosted payment for a booking using the chosen
// method and returns the URL the client opens to complete it.
//
//	POST /bookings/:id/payment?method=card|apple_pay|paypal
func CreatePaymentHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
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
		if booking.PaymentStatus == "completed" {
			c.JSON(http.StatusConflict, gin.H{"error": "Payment already completed"})
			return
		}

		expiresAt := booking.CreatedAt.Add(15 * time.Minute)
		if time.Now().After(expiresAt) && booking.BookingStatus == models.BookingPending {
			c.JSON(http.StatusGone, gin.H{"error": "Booking has expired", "expired_at": expiresAt})
			return
		}

		method := parsePaymentMethod(c)
		redirectURL, reference, err := paymentService.CreatePaymentIntentWithMethod(booking, method)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payment"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"booking_id":        booking.ID,
			"booking_number":    booking.BookingNumber,
			"authorization_url": redirectURL,
			"reference":         reference,
			"payment_method":    method,
			"amount":            booking.TotalAmount,
			"currency":          booking.Currency,
		})
	}
}

// GetPaymentStatusHandler returns the current payment status for a booking.
//
//	GET /bookings/:id/payment
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
			"booking_id":        bookingID,
			"booking_number":    booking.BookingNumber,
			"payment_status":    payment.Status,
			"payment_method":    payment.PaymentMethod,
			"payment_gateway":   payment.PaymentGateway,
			"amount":            payment.Amount,
			"currency":          payment.Currency,
			"authorization_url": payment.GatewayRedirectURL,
			"reference":         payment.GatewayReference,
			"transaction_id":    payment.TransactionID,
			"completed_at":      payment.CompletedAt,
			"webhook_received":  payment.WebhookReceived,
		})
	}
}

// RetryPaymentHandler re-initialises a payment for a pending/failed booking.
//
//	POST /bookings/:id/payment/retry?method=card|apple_pay|paypal
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

		if payment, err := paymentService.GetPaymentStatus(bookingID); err == nil {
			switch payment.Status {
			case "completed", "refunded":
				c.JSON(http.StatusConflict, gin.H{"error": "Cannot retry a completed or refunded payment"})
				return
			}
		}

		expiresAt := booking.CreatedAt.Add(15 * time.Minute)
		if time.Now().After(expiresAt) && booking.BookingStatus == models.BookingPending {
			c.JSON(http.StatusGone, gin.H{"error": "Booking has expired", "expired_at": expiresAt})
			return
		}

		method := parsePaymentMethod(c)
		redirectURL, reference, err := paymentService.CreatePaymentIntentWithMethod(booking, method)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payment"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"booking_id":        bookingID,
			"booking_number":    booking.BookingNumber,
			"authorization_url": redirectURL,
			"reference":         reference,
			"payment_method":    method,
			"amount":            booking.TotalAmount,
			"currency":          booking.Currency,
		})
	}
}