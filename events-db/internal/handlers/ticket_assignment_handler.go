package handlers

import (
	"errors"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── GET /bookings/:id/assignments ─────────────────────────────────────────────
// Returns all ticket assignment slots for a booking the user owns.
func GetTicketAssignmentsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)
		bookingID := c.Param("id")

		if _, err := uuid.Parse(bookingID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid booking ID"})
			return
		}

		assignments, err := repository.GetAssignmentsByBooking(pool, bookingID, userID)
		if err != nil {
			if errors.Is(err, repository.ErrNotAssignmentOwner) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load assignments"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"data": assignments})
	}
}

// ── POST /bookings/:id/assignments/:assignmentId ───────────────────────────────
// Buyer assigns an unassigned slot to someone by email.
// Sends the recipient a claim invite email.
func AssignTicketHandler(
	pool *pgxpool.Pool,
	emailSvc *services.EmailService,
	frontendURL string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)
		bookingID := c.Param("id")
		assignmentID := c.Param("assignmentId")

		for _, id := range []string{bookingID, assignmentID} {
			if _, err := uuid.Parse(id); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
				return
			}
		}

		var dto models.AssignTicketDTO
		if err := c.ShouldBindJSON(&dto); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
			return
		}

		// Fetch booking details needed for the email.
		booking, err := repository.GetBookingByID(pool, bookingID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}
		if booking.UserID == nil || *booking.UserID != userID {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		claimToken, err := repository.AssignTicket(pool, assignmentID, bookingID, userID, &dto)
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrAssignmentNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "Assignment not found"})
			case errors.Is(err, repository.ErrAlreadyClaimed):
				c.JSON(http.StatusConflict, gin.H{"error": "This ticket has already been claimed"})
			case errors.Is(err, repository.ErrAlreadyAdmitted):
				c.JSON(http.StatusConflict, gin.H{"error": "This ticket has already been admitted at the gate and can't be assigned"})
			case errors.Is(err, repository.ErrNotAssignmentOwner):
				c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign ticket"})
			}
			return
		}

		claimURL := fmt.Sprintf("%s/claim-ticket?token=%s", frontendURL, claimToken)	
		expiresAt := time.Now().Add(24 * time.Hour)

		// Fetch buyer's name for the email.
		buyerName := "Someone"
		if booking.AttendeeFirstName != nil {
			buyerName = *booking.AttendeeFirstName
			if booking.AttendeeLastName != nil {
				buyerName += " " + *booking.AttendeeLastName
			}
		}

		if booking.Event == nil {
			booking.Event = &models.Event{Title: "the event"}
		}

		go func() {
			if err := emailSvc.SendTicketAssignmentInvite(
				dto.AssigneeEmail,
				dto.AssigneeName,
				buyerName,
				booking.Event.Title,
				claimURL,
				expiresAt,
			); err != nil {
				// Log but don't fail the HTTP response — the assignment is already saved.
				_ = err
			}
		}()

		c.JSON(http.StatusOK, gin.H{
			"message":    "Invite sent successfully",
			"expires_at": expiresAt,
		})
	}
}

// ── DELETE /bookings/:id/assignments/:assignmentId ─────────────────────────────
// Buyer revokes a pending (unclaimed) assignment, returning it to unassigned.
func RevokeAssignmentHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)
		bookingID := c.Param("id")
		assignmentID := c.Param("assignmentId")

		err := repository.RevokeAssignment(pool, assignmentID, bookingID, userID)
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrAssignmentNotPending):
				c.JSON(http.StatusConflict, gin.H{"error": "Ticket cannot be revoked — it may already be claimed or unassigned"})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke assignment"})
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Assignment revoked. Ticket is now unassigned."})
	}
}

// ── POST /tickets/claim ───────────────────────────────────────────────────────
// Public endpoint — no auth required. The recipient clicks the link in their
// email, which hits the frontend's /claim-ticket page, which calls this
// endpoint with the token and the claimant's confirmed name + email.
func ClaimTicketHandler(pool *pgxpool.Pool, emailSvc *services.EmailService, frontendURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			Token         string `json:"token"          binding:"required,uuid"`
			ClaimantName  string `json:"claimant_name"  binding:"required,min=1,max=200"`
			ClaimantEmail string `json:"claimant_email" binding:"required,email"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
			return
		}

		assignment, err := repository.ClaimTicket(pool, input.Token, input.ClaimantName, input.ClaimantEmail)
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrAssignmentNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "This claim link is invalid or has already been used"})
			case errors.Is(err, repository.ErrClaimTokenExpired):
				c.JSON(http.StatusGone, gin.H{"error": "This claim link has expired. Ask the ticket holder to resend the invitation."})
			case errors.Is(err, repository.ErrClaimEmailMismatch):
				c.JSON(http.StatusForbidden, gin.H{
					"error": "This ticket was assigned to a different email address. Enter the email address that received the invitation.",
					"field": "email",
				})
			case errors.Is(err, repository.ErrClaimNameMismatch):
				c.JSON(http.StatusForbidden, gin.H{
					"error": "The name doesn't match this invitation. Enter your name exactly as it appears in the invitation email.",
					"field": "name",
				})
			case errors.Is(err, repository.ErrAssignmentNotPending):
				c.JSON(http.StatusConflict, gin.H{"error": "This ticket has already been claimed or is no longer available"})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to claim ticket"})
			}
			return
		}

		// The claimant's permanent "my ticket" link — renders the live
		// rotating entry QR. Static QR images are no longer emailed: the
		// gate scanner only verifies rotating tokens, so an emailed image
		// could never be scanned anyway.
		ticketURL := fmt.Sprintf("%s/claim-ticket?assignment=%s&email=%s",
			frontendURL, assignment.ID, url.QueryEscape(input.ClaimantEmail))

		// Fetch event details for the confirmation email.
		booking, err := repository.GetBookingByID(pool, assignment.BookingID)
		if err == nil && booking.Event != nil {
			eventDate := booking.Event.StartTime.Format("Monday, 2 January 2006 at 15:04")
			venueName := ""
			if booking.Event.VenueName != "" {
				venueName = booking.Event.VenueName
			}

			go func() {
				_ = emailSvc.SendTicketClaimConfirmation(
					input.ClaimantEmail,
					input.ClaimantName,
					booking.Event.Title,
					eventDate,
					venueName,
					booking.BookingNumber,
					assignment.SlotNumber,
					ticketURL,
				)
			}()
		}

		c.JSON(http.StatusOK, gin.H{
			"message":       "Ticket claimed successfully!",
			"slot_number":   assignment.SlotNumber,
			"assignment_id": assignment.ID,
			"ticket_url":    ticketURL,
			"event_title": func() string {
				if booking != nil && booking.Event != nil {
					return booking.Event.Title
				}
				return ""
			}(),
		})
	}
}