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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// GetGuestBookingHandler no longer takes a Redis client.
//
// Brute-force lockout now lives in middleware.GuestBookingGuard on the whole
// /guest group. That removes the nil-client panic this handler used to have
// (main.go leaves rdb nil when Redis is down at startup, and this called
// rdb.Get unguarded), and it means every guest route is protected rather than
// just this one and /assignments.
func GetGuestBookingHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookingNumber := c.Param("bookingNumber")
		email := c.Query("email")

		if bookingNumber == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Booking number is required"})
			return
		}
		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
			return
		}

		// The 404 below is what the guard counts as a failed attempt.
		booking, err := repository.GetGuestBooking(pool, bookingNumber, email)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		// Same event details an account holder's booking screen gets.
		repository.HydrateBookingEvent(pool, booking)

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
func GetGuestTicketAssignmentsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookingNumber := c.Param("bookingNumber")
		email := c.Query("email")

		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
			return
		}

		// Lockout is handled by middleware.GuestBookingGuard on the /guest
		// group; the 404 below is what it counts as a failed attempt.
		booking, err := repository.GetGuestBooking(pool, bookingNumber, email)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
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

		// Take the seats back before starting a new payment — same reasoning as
		// RetryPaymentHandler. If anything released this booking's seats, the
		// retry's payment will confirm this same booking row regardless.
		if _, reErr := repository.ReacquireSeats(pool, c.Request.Context(), booking.ID); reErr != nil {
			log.Error().Err(reErr).Str("booking_id", booking.ID).
				Msg("guest retry payment: could not reacquire released seats")
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

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// ── Claim the refund BEFORE calling the gateway ───────────────────────
		//
		// This used to run gateway-first, on the reasoning that a failed refund
		// should leave the guest holding a valid ticket rather than a voided one.
		// The order is right in spirit and wrong in effect: the eligibility check
		// above is a plain read, and the write that followed the gateway call was
		// `WHERE id = $3` with no status condition. Two requests arriving together
		// — a double-tapped button, a client retry, a flaky mobile connection —
		// both read 'confirmed', both passed the check, and both issued a refund
		// at Paystack. Real money, twice, recoverable only by hand.
		//
		// The authed path already got this right: RequestRefundWithCascade flips
		// the row inside a transaction with AND b.booking_status = $5 and checks
		// RowsAffected() == 0, so a second caller can never reach the gateway.
		// This is the same latch. The claim is a compare-and-swap on
		// booking_status, so exactly one request can ever move a booking out of
		// 'confirmed', and every other request is turned away at the 409 below
		// without touching the gateway.
		//
		// The fail-safe direction is preserved by releasing the latch if the
		// gateway then fails (see below) — the booking goes back to 'confirmed',
		// the ticket stays valid, and the guest can retry. Trading a brief window
		// where a ticket is provisionally voided for the elimination of double
		// refunds is the right trade: the first is reversible in code, the second
		// is not.
		var claimedAt time.Time
		err = pool.QueryRow(ctx, `
			UPDATE bookings
			SET booking_status      = 'refunded',
			    refund_amount       = $1,
			    refund_rate         = $2,
			    refunded_at         = CURRENT_TIMESTAMP,
			    refund_requested_at = CURRENT_TIMESTAMP,
			    refund_reason       = 'Guest requested refund',
			    updated_at          = CURRENT_TIMESTAMP
			WHERE id = $3
			  AND booking_status = 'confirmed'
			RETURNING refunded_at
		`, quote.RefundAmount, quote.Rate, booking.ID).Scan(&claimedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Lost the race, or the booking moved on between the read above
				// and here. Either way it is no longer ours to refund.
				c.JSON(http.StatusConflict, gin.H{"error": "This booking is not eligible for a refund"})
				return
			}
			utils.InternalError(c, err, "guest refund: could not claim booking")
			return
		}

		// ── Gateway ───────────────────────────────────────────────────────────
		//
		// A zero-value booking skips the gateway entirely: there may be no
		// payments row at all, and the amount rounds to zero minor units, so
		// ProcessRefund fails on its own guards. Attempting it would block the
		// cancellation of a free ticket and leave the seat held — see the same
		// case in booking_handler.go.
		if quote.RefundAmount > 0 {
			if refErr := paymentService.ProcessRefund(booking.ID, quote.RefundAmount); refErr != nil {
				// Release the latch so the guest is not left with a voided
				// ticket and no money. Guarded on the exact refunded_at this
				// request wrote, so it can only ever undo its own claim.
				if _, rbErr := pool.Exec(ctx, `
					UPDATE bookings
					SET booking_status      = 'confirmed',
					    refund_amount       = NULL,
					    refund_rate         = NULL,
					    refunded_at         = NULL,
					    refund_requested_at = NULL,
					    refund_reason       = NULL,
					    updated_at          = CURRENT_TIMESTAMP
					WHERE id = $1
					  AND booking_status = 'refunded'
					  AND refunded_at = $2
				`, booking.ID, claimedAt); rbErr != nil {
					// The guest now holds a booking marked refunded that was
					// never paid out. Loud, because it needs a human.
					log.Error().Err(rbErr).
						Str("booking_id", booking.ID).
						Str("booking_number", booking.BookingNumber).
						Msg("guest refund: gateway failed AND latch release failed — booking is marked refunded with no payout")
				}
				utils.InternalError(c, refErr, "guest refund: gateway refund failed")
				return
			}
		}

		if _, err := pool.Exec(ctx, `
			UPDATE ticket_assignments SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP
			WHERE booking_id = $1 AND status <> 'cancelled'
		`, booking.ID); err != nil {
			// Money is already back with the guest; this is bookkeeping only.
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