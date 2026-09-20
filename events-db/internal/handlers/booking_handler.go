package handlers

import (
	"errors"
	"events-db/internal/config"
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
	"github.com/rs/zerolog/log"
)

func CreateBookingHandler(
	bookings repository.BookingQuerier,
	events repository.EventQuerier,
	paymentService *services.PaymentService,
	cfg *config.Config,
) gin.HandlerFunc {
	expiryDuration := time.Duration(cfg.BookingExpiryMinutes) * time.Minute
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		var dto models.CreateBookingDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation",
				"details": utils.FormatValidationErrors(err),
			})
			return
		}

		event, err := events.GetEventByID(dto.EventID, nil)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		if event.MaxCapacity != nil && event.TicketsSold >= *event.MaxCapacity {
			c.JSON(http.StatusConflict, gin.H{"error": "This event is sold out"})
			return
		}

		if event.MaxTicketsPerUser != nil && dto.TicketQuantity > *event.MaxTicketsPerUser {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Exceeds per-person ticket limit",
				"max_tickets_per_user": *event.MaxTicketsPerUser,
			})
			return
		}

		if !event.Status.IsLive() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Event is not available for booking"})
			return
		}

		booking, err := bookings.CreateBooking(&dto, userID.(string), event)
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create booking"})
			return
		}

		// Free event — skip Stitch entirely, confirm the booking immediately.
		if booking.TotalAmount == 0 {
			if err := bookings.ConfirmBooking(booking.ID); err != nil {
				log.Error().Err(err).Str("booking_id", booking.ID).Msg("failed to auto-confirm free booking")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to confirm free booking"})
				return
			}
			// Trigger the booking worker so it creates ticket assignments,
			// generates the QR code, and sends the confirmation email —
			// identical to the post-payment flow for paid bookings.
			paymentService.EnqueueJob(booking.ID)
			c.JSON(http.StatusCreated, models.BookingResponse{
				BookingID:     booking.ID,
				BookingNumber: booking.BookingNumber,
				TotalAmount:   0,
				Currency:      booking.Currency,
				ClientToken:   "",
				ExpiresAt:     booking.CreatedAt.Add(expiryDuration),
			})
			return
		}

		clientToken, paymentRequestID, err := paymentService.CreatePaymentIntent(booking, paymentChannel(c))
		if err != nil {
			// Seats were counted against capacity when the booking committed.
			// Release them now instead of leaving them locked for 15 minutes
			// until the expiry worker runs.
			if relErr := bookings.ReleaseFailedBooking(booking.ID); relErr != nil {
				log.Error().Err(relErr).Str("booking_id", booking.ID).
					Msg("checkout: could not release seats after payment init failure")
			}
			utils.InternalError(c, err, "checkout: failed to initialize payment")
			return
		}
		c.JSON(http.StatusCreated, models.BookingResponse{
			BookingID:        booking.ID,
			BookingNumber:    booking.BookingNumber,
			TotalAmount:      booking.TotalAmount,
			Currency:         booking.Currency,
			ClientToken:      clientToken,
			PaymentRequestID: paymentRequestID,
			ExpiresAt:        booking.CreatedAt.Add(expiryDuration),
		})
	}
}

func GetUserBookingsHandler(bookings repository.BookingQuerier) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")

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
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
				return
			}
		}

		result, nextCursor, err := bookings.GetUserBookings(userID.(string), limit, cursor)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get bookings"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":        result,
			"next_cursor": utils.EncodeCursor(nextCursor),
		})
	}
}

func GetBookingDetailsHandler(bookings repository.BookingQuerier, pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		bookingID := c.Param("id")

		if _, err := uuid.Parse(bookingID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid booking ID"})
			return
		}

		booking, err := bookings.GetBookingByID(bookingID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		if booking.UserID == nil || *booking.UserID != userID.(string) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		// End time, organiser, Information block and line-up — what the event
		// page shows, on the ticket the buyer carries to the door. Only after
		// the ownership check, so nothing extra is loaded for a refused request.
		repository.HydrateBookingEvent(pool, booking)

		c.JSON(http.StatusOK, booking)
	}
}

// RequestRefundHandler processes a refund request for a confirmed booking.
//
// FIX 4: Ticket invalidation notices are now sent synchronously before the
// response is returned, rather than in a fire-and-forget goroutine.
//
// The previous goroutine was silently lost on process restart, meaning
// assignees whose tickets were invalidated by the refund might never receive
// an email notification. The assignee count is bounded (one per ticket in the
// booking) so the latency increase is negligible in practice, and the caller
// now gets a response that accurately reflects whether emails were attempted.
func RequestRefundHandler(
	pool *pgxpool.Pool,
	bookings repository.BookingQuerier,
	events repository.EventQuerier,
	paymentService *services.PaymentService,
	emailService *services.EmailService,
) gin.HandlerFunc {
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

		booking, err := bookings.GetBookingByID(bookingID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}
		if booking.UserID == nil || *booking.UserID != userID.(string) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "You do not have permission to refund this booking"})
			return
		}

		event, err := events.GetEventByID(func() string { if booking.EventID != nil { return *booking.EventID }; return "" }(), nil)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		// Price the refund BEFORE touching the booking. The sliding scale keys
		// off how far the event still is right now, not off when the ticket was
		// bought, and the buyer service fee is never returned on a
		// buyer-initiated refund. An ineligible quote (inside 7 days, or the
		// event already started) is a 400 with the reason, not a silent no-op.
		quote := models.QuoteBuyerRefund(booking.Subtotal, booking.PlatformFee, event.StartTime, time.Now())
		if !quote.Eligible {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": quote.Reason,
				"quote": quote,
			})
			return
		}

		cancelledAssignees, err := bookings.RequestRefundWithCascade(
			bookingID, userID.(string), input.Reason, quote.RefundAmount, quote.Rate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// ── Free bookings never reach the payment gateway ────────────────────
		//
		// A R0 ticket may have no payments row at all, and even where one
		// exists the refund rounds to zero minor units. ProcessRefund fails on
		// any of its first three guards, and because this handler returned on
		// that error, RequestRefundWithCascade had ALREADY marked the booking
		// refunded while ReleaseRefundedSeats below never ran — so the seat
		// stayed consumed forever and the event drifted towards a false
		// sold-out. The user saw "Failed to process payment refund" on a
		// booking that had, from the database's point of view, been cancelled.
		//
		// There is genuinely nothing to refund here, so the gateway call is
		// skipped rather than attempted and forgiven. Everything after this
		// point — seat release, invalidation emails, the response — is
		// identical, because releasing the seat is the whole point of the
		// action on a free ticket.
		if quote.RefundAmount > 0 {
			if err := paymentService.ProcessRefund(bookingID, quote.RefundAmount); err != nil {
				log.Error().Err(err).
					Str("booking_id", bookingID).
					Float64("amount", quote.RefundAmount).
					Msg("refund: gateway call failed after booking was already marked refunded")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process payment refund"})
				return
			}
		} else {
			log.Info().Str("booking_id", bookingID).
				Msg("cancellation: zero-value booking, no gateway refund needed")
		}

		// Put the seats back on sale. Nothing decremented tickets_sold on a
		// refund before this, so every refunded ticket permanently consumed
		// capacity and events drifted towards a false sold-out. Idempotent, so
		// a retried refund cannot release the same seats twice and oversell.
		if relErr := bookings.ReleaseRefundedSeats(bookingID); relErr != nil {
			log.Error().Err(relErr).Str("booking_id", bookingID).
				Msg("refund: seats not returned to the pool — event capacity understated")
		}

		// FIX 4: send synchronously — no goroutine. The count is bounded by
		// ticket quantity so latency is negligible. Errors are logged but do
		// not fail the request since the refund itself has already succeeded.
		emailsFailed := 0
		for _, assignee := range cancelledAssignees {
			if err := emailService.SendTicketInvalidationNotice(
				assignee.Email,
				assignee.Name,
				event.Title,
				utils.FormatEventDate(event.StartTime, event.Timezone),
				booking.BookingNumber,
			); err != nil {
				emailsFailed++
				log.Error().
					Err(err).
					Str("booking_number", booking.BookingNumber).
					Str("assignee_email", assignee.Email).
					Msg("refund: failed to send ticket invalidation notice")
			}
		}

		// Feed row + push for the buyer.
		//
		// This path does NOT go through RefundService.markBookingRefunded — it
		// uses RequestRefundWithCascade — so the notification wired in there
		// covers organiser cancellations only. Buyer-initiated refunds are the
		// case where someone is actively waiting on an answer, so they need it
		// more, not less.
		//
		// Best-effort: the money has already moved and the response below has
		// to be a success regardless. A missing feed row is recoverable; a 500
		// after a completed refund is not.
		if nErr := repository.CreateNotification(
			pool, c.Request.Context(), userID.(string),
			models.NotifRefundProcessed,
			"Refund Processed",
			fmt.Sprintf("Your refund of R%.2f for %s is on its way. Allow a few working days for it to reflect.",
				quote.RefundAmount, event.Title),
			map[string]any{
				"booking_id":     bookingID,
				"booking_number": booking.BookingNumber,
				"event_id":       event.ID,
				"refund_amount":  quote.RefundAmount,
			},
		); nErr != nil {
			log.Error().Err(nErr).Str("booking_id", bookingID).
				Msg("refund: notification failed — refund itself succeeded")
		}

		c.JSON(http.StatusOK, gin.H{
			"message":            "Refund processed successfully",
			"refund_status":      "processing",
			"refund_amount":      quote.RefundAmount,
			"refund_rate":        quote.Rate,
			"service_fee_kept":   quote.ServiceFeeKept,
			"days_until_event":   quote.DaysUntilEvent,
			"assignees_notified": len(cancelledAssignees) - emailsFailed,
		})
	}
}

// VerifyQRCodeHandler verifies a scanned QR code. Only organisers and admins
// may call this endpoint, and organisers may only verify tickets for their own
// events.
func VerifyQRCodeHandler(
	bookings repository.BookingQuerier,
	events repository.EventQuerier,
	qrService *services.QRService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		role, _ := c.Get("role")

		if role != string(models.RoleOrganizer) && role != string(models.RoleAdmin) {
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

		payload, err := qrService.VerifyQRCode(input.QRData)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"valid": false,
				"error": err.Error(),
			})
			return
		}

		booking, err := bookings.GetBookingByID(payload.BookingID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"valid": false,
				"error": "Booking not found",
			})
			return
		}

		event, err := events.GetEventByID(func() string { if booking.EventID != nil { return *booking.EventID }; return "" }(), nil)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"valid": false,
				"error": "Event not found",
			})
			return
		}

		if role == string(models.RoleOrganizer) && event.OrganizerID != userID.(string) {
			c.JSON(http.StatusForbidden, gin.H{
				"valid": false,
				"error": "You can only verify tickets for your own events",
			})
			return
		}

		attendeeName := ""
		if booking.AttendeeFirstName != nil {
			attendeeName = *booking.AttendeeFirstName
		}
		if booking.AttendeeLastName != nil {
			if attendeeName != "" {
				attendeeName += " "
			}
			attendeeName += *booking.AttendeeLastName
		}

		c.JSON(http.StatusOK, gin.H{
			"valid":           true,
			"booking_number":  booking.BookingNumber,
			"attendee_name":   attendeeName,
			"ticket_quantity": booking.TicketQuantity,
			"event_title":     event.Title,
		})
	}
}
// RefundQuoteHandler previews a refund for a logged-in buyer without executing
// it, so the app can show "you will get R200 of your R420 back" before they
// commit. Read-only and safe to poll.
//
//	GET /v1/bookings/:id/refund-quote
func RefundQuoteHandler(bookings repository.BookingQuerier, events repository.EventQuerier) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		bookingID := c.Param("id")

		booking, err := bookings.GetBookingByID(bookingID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}
		if booking.UserID == nil || *booking.UserID != userID.(string) {
			c.JSON(http.StatusForbidden, gin.H{"error": "You do not have permission to view this booking"})
			return
		}

		eventID := ""
		if booking.EventID != nil {
			eventID = *booking.EventID
		}
		event, err := events.GetEventByID(eventID, nil)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		quote := models.QuoteBuyerRefund(booking.Subtotal, booking.PlatformFee, event.StartTime, time.Now())
		c.JSON(http.StatusOK, gin.H{
			"booking_number": booking.BookingNumber,
			"total_paid":     booking.TotalAmount,
			"quote":          quote,
			"schedule":       models.RefundSchedule,
		})
	}
}