package handlers

import (
	"context"
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
	"github.com/rs/zerolog/log"
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

		if !event.Status.IsLive() {
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
			if errors.Is(err, repository.ErrBookingTicketTypeNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Ticket type not found for this event"})
				return
			}
			if errors.Is(err, repository.ErrTicketTypeNotOnSale) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "This ticket type is not currently on sale"})
				return
			}
			if errors.Is(err, repository.ErrTicketTypeSoldOut) {
				c.JSON(http.StatusConflict, gin.H{"error": "This ticket type is sold out"})
				return
			}
			utils.InternalError(c, err, "guest checkout: failed to create booking")
			return
		}

		// Free event — skip the gateway entirely. Paystack rejects a R0 amount
		// ("Invalid Amount Sent"), so confirm the booking immediately and let the
		// worker create ticket assignments / QR / email, identical to the
		// post-payment flow for paid bookings.
		if booking.TotalAmount == 0 {
			if err := repository.ConfirmBooking(pool, booking.ID); err != nil {
				utils.InternalError(c, err, "guest checkout: failed to confirm free booking")
				return
			}
			paymentService.EnqueueJob(booking.ID)
			c.JSON(http.StatusCreated, models.BookingResponse{
				BookingID:     booking.ID,
				BookingNumber: booking.BookingNumber,
				TotalAmount:   0,
				Currency:      booking.Currency,
				ClientToken:   "",
				ExpiresAt:     booking.CreatedAt.Add(15 * time.Minute),
				IsGuest:       true,
			})
			return
		}

		method := dto.PaymentMethod
		if method == "" {
			method = models.PaymentCreditCard
		}
		clientToken, paymentRequestID, err := paymentService.CreatePaymentIntentWithMethod(booking, method, paymentChannel(c))
		if err != nil {
			// The booking committed before this call, so its seats already count
			// against capacity. Hand them straight back rather than waiting 15
			// minutes for the expiry worker — a buyer retrying a failed method
			// would otherwise burn the event's inventory one attempt at a time.
			if relErr := repository.ReleaseFailedBooking(pool, c.Request.Context(), booking.ID); relErr != nil {
				log.Error().Err(relErr).Str("booking_id", booking.ID).
					Msg("guest checkout: could not release seats after payment init failure")
			}
			utils.InternalError(c, err, "guest checkout: failed to initialize payment")
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
			utils.InternalError(c, err, "guest assignments: failed to load assignments")
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

		retryMethod := models.PaymentMethod(c.Query("method"))
		if retryMethod == "" {
			retryMethod = models.PaymentCreditCard
		}
		clientToken, paymentRequestID, err := paymentService.CreatePaymentIntentWithMethod(booking, retryMethod, paymentChannel(c))
		if err != nil {
			// The booking committed before this call, so its seats already count
			// against capacity. Hand them straight back rather than waiting 15
			// minutes for the expiry worker — a buyer retrying a failed method
			// would otherwise burn the event's inventory one attempt at a time.
			if relErr := repository.ReleaseFailedBooking(pool, c.Request.Context(), booking.ID); relErr != nil {
				log.Error().Err(relErr).Str("booking_id", booking.ID).
					Msg("guest retry payment: could not release seats after payment init failure")
			}
			utils.InternalError(c, err, "guest retry payment: failed to initialize payment")
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
// GuestStartPaymentHandler initialises (or re-initialises) a payment for a
// guest booking with the requested method (card, paypal, instant_eft).
// Identity is verified by bookingNumber + email query params.
//
//	POST /v1/guest/bookings/:bookingNumber/payment/start?email=&method=
func GuestStartPaymentHandler(pool *pgxpool.Pool, paymentService *services.PaymentService) gin.HandlerFunc {
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

		if booking.PaymentStatus == "completed" || booking.PaymentStatus == "refunded" {
			c.JSON(http.StatusConflict, gin.H{"error": "Payment already completed"})
			return
		}

		method := models.PaymentMethod(c.Query("method"))
		if method == "" {
			method = models.PaymentCreditCard
		}

		clientToken, paymentRequestID, err := paymentService.CreatePaymentIntentWithMethod(booking, method, paymentChannel(c))
		if err != nil {
			utils.InternalError(c, err, "guest start payment: failed to initialize payment")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"booking_id":         booking.ID,
			"booking_number":     booking.BookingNumber,
			"authorization_url":  clientToken,
			"reference":          paymentRequestID,
			"payment_method":     method,
			"amount":             booking.TotalAmount,
			"currency":           booking.Currency,
		})
	}
}
// GuestRefundQuoteHandler tells a guest what they would get back BEFORE they
// commit to cancelling. Read-only.
//
//	GET /v1/guest/bookings/:bookingNumber/refund-quote?email=...
func GuestRefundQuoteHandler(pool *pgxpool.Pool) gin.HandlerFunc {
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

		var startTime time.Time
		if err := pool.QueryRow(c.Request.Context(),
			`SELECT start_time FROM events WHERE id = $1`, booking.EventID).Scan(&startTime); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		quote := models.QuoteBuyerRefund(booking.Subtotal, booking.PlatformFee, startTime, time.Now())
		c.JSON(http.StatusOK, gin.H{
			"booking_number": booking.BookingNumber,
			"total_paid":     booking.TotalAmount,
			"quote":          quote,
		})
	}
}

// GuestRefundHandler lets a guest cancel and be refunded on the sliding scale.
//
// Guests could not refund at all before this: RequestRefundHandler requires an
// authenticated user and compares booking.UserID, which is NULL for every guest
// booking. That left a complete purchase path with no way out of it.
//
//	POST /v1/guest/bookings/:bookingNumber/refund?email=...
func GuestRefundHandler(
	pool *pgxpool.Pool,
	paymentService *services.PaymentService,
	emailService *services.EmailService,
) gin.HandlerFunc {
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
		if booking.BookingStatus != models.BookingConfirmed {
			c.JSON(http.StatusConflict, gin.H{"error": "This booking is not eligible for a refund"})
			return
		}

		var startTime time.Time
		var eventTitle string
		if err := pool.QueryRow(c.Request.Context(),
			`SELECT start_time, title FROM events WHERE id = $1`,
			booking.EventID).Scan(&startTime, &eventTitle); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		quote := models.QuoteBuyerRefund(booking.Subtotal, booking.PlatformFee, startTime, time.Now())
		if !quote.Eligible {
			c.JSON(http.StatusBadRequest, gin.H{"error": quote.Reason, "quote": quote})
			return
		}

		// Gateway first, database second. If the refund fails the booking stays
		// confirmed and the guest keeps a valid ticket — the safe direction to
		// fail in. The reverse order would void their ticket without paying them.
		if err := paymentService.ProcessRefund(booking.ID, quote.RefundAmount); err != nil {
			utils.InternalError(c, err, "guest refund: gateway refund failed")
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `
			UPDATE bookings
			SET booking_status      = 'refunded',
			    refund_amount       = $1,
			    refund_rate         = $2,
			    refunded_at         = CURRENT_TIMESTAMP,
			    refund_requested_at = CURRENT_TIMESTAMP,
			    refund_reason       = 'Guest requested refund',
			    updated_at          = CURRENT_TIMESTAMP
			WHERE id = $3
		`, quote.RefundAmount, quote.Rate, booking.ID); err != nil {
			// Money is already back with the guest; this is bookkeeping only.
			utils.InternalError(c, err, "guest refund: refunded at gateway but booking not updated")
			return
		}

		if _, err := pool.Exec(ctx, `
			UPDATE ticket_assignments SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP
			WHERE booking_id = $1 AND status <> 'cancelled'
		`, booking.ID); err != nil {
			utils.InternalError(c, err, "guest refund: could not void ticket assignments")
			return
		}

		// Return the seats to the pool. Nothing decremented tickets_sold on a
		// refund before this, so refunded tickets permanently consumed capacity.
		// Idempotent — a retried refund cannot release the same seats twice.
		if relErr := repository.ReleaseRefundedSeats(pool, ctx, booking.ID); relErr != nil {
			log.Error().Err(relErr).Str("booking_id", booking.ID).
				Msg("guest refund: seats not returned to the pool — event capacity understated")
		}

		c.JSON(http.StatusOK, gin.H{
			"message":          "Refund processed successfully",
			"refund_amount":    quote.RefundAmount,
			"refund_rate":      quote.Rate,
			"service_fee_kept": quote.ServiceFeeKept,
			"days_until_event": quote.DaysUntilEvent,
		})
	}
}