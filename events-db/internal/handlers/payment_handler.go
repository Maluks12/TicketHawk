package handlers

import (
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"fmt"
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

// PaymentPageHandler serves the Stitch Secure Fields host page.
// The client_token is injected server-side so the WebView can render
// the card form entirely in-app — no browser redirect occurs.
func PaymentPageHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        bookingID   := c.Param("id")
        clientToken := c.Query("client_token")
        if clientToken == "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": "client_token required"})
            return
        }
        html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
  <title>Payment</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: -apple-system, BlinkMacSystemFont, sans-serif;
           background: #f9f9f9; padding: 24px 16px; }
    h3   { font-size: 15px; color: #888; font-weight: 500; margin-bottom: 16px; }
    #stitch-secure-fields { background: #fff; border-radius: 14px;
           padding: 20px; box-shadow: 0 1px 8px rgba(0,0,0,0.07); min-height: 180px; }
    #pay-btn { display: block; width: 100%%; margin-top: 24px;
               background: #e8163c; color: #fff; border: none; border-radius: 14px;
               padding: 17px; font-size: 17px; font-weight: 700;
               letter-spacing: 0.3px; cursor: pointer; transition: opacity .2s; }
    #pay-btn:disabled { opacity: 0.45; cursor: not-allowed; }
    #status { margin-top: 14px; text-align: center; font-size: 13px; color: #666; min-height: 20px; }
  </style>
</head>
<body>
  <h3>Card Details</h3>
  <div id="stitch-secure-fields"></div>
  <button id="pay-btn" disabled>Pay Now</button>
  <p id="status"></p>

  <script src="https://secure.stitch.money/sdk/secure-fields/v1/index.js"></script>
  <script>
    var btn    = document.getElementById('pay-btn');
    var status = document.getElementById('status');

    var sf = new StitchSecureFields({
      clientToken: '%s',
      containerId: 'stitch-secure-fields',
      onReady: function() { btn.disabled = false; },
      onComplete: function() {
        status.textContent = 'Payment submitted — confirming…';
        btn.disabled = true;
        // Deep-link back into the app — WebView intercepts this URL
        window.location.href = 'eventsapp://payment/success?booking_id=%s';
      },
      onError: function(err) {
        status.textContent = err && err.message ? err.message : 'Payment failed. Please try again.';
        btn.disabled = false;
      }
    });

    btn.onclick = function() {
      btn.disabled = true;
      status.textContent = 'Processing…';
      sf.submit();
    };
  </script>
</body>
</html>`, clientToken, bookingID)

        c.Header("Content-Type", "text/html; charset=utf-8")
        c.String(http.StatusOK, html)
    }
}
