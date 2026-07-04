package handlers

import (
	"errors"
	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"events-db/internal/utils"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

		if event.Status != models.EventPublished {
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

		clientToken, paymentRequestID, err := paymentService.CreatePaymentIntent(booking)
		if err != nil {
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

func GetBookingDetailsHandler(bookings repository.BookingQuerier) gin.HandlerFunc {
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

		cancelledAssignees, err := bookings.RequestRefundWithCascade(bookingID, userID.(string), input.Reason)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := paymentService.ProcessRefund(bookingID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process payment refund"})
			return
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
				event.StartTime.Format("Monday, 2 January 2006"),
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

		c.JSON(http.StatusOK, gin.H{
			"message":            "Refund processed successfully",
			"refund_status":      "processing",
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