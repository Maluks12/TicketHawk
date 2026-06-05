package handlers

import (
	"context"
	"net/http"
	"time"

	"events-db/internal/models"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GetRotatingQRTokenHandler godoc
//
//	GET /v1/tickets/:assignmentId/qr
//
// Returns a fresh short-lived HMAC-signed rotating QR token for the ticket holder.
// The app renders the "token" string as a QR code and polls this endpoint every ~25s
// to stay ahead of the 30-second rotation window.
//
// Auth: user JWT (booking owner)
func GetRotatingQRTokenHandler(
	pool *pgxpool.Pool,
	rotatingQR *services.RotatingQRService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)
		assignmentID := c.Param("assignmentId")

		if _, err := uuid.Parse(assignmentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment ID"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		// Fetch the assignment along with the booking and event context needed to
		// build the payload and verify the token is still valid.
		var (
			bookingID     string
			eventID       *string // nullable — merch-only bookings have no event
			slotNumber    int
			assignStatus  string
			bookingStatus string
			bookingUserID *string
		)

		err := pool.QueryRow(ctx, `
			SELECT
				ta.booking_id,
				b.event_id,
				ta.slot_number,
				ta.status,
				b.booking_status,
				b.user_id
			FROM ticket_assignments ta
			JOIN bookings b ON ta.booking_id = b.id
			WHERE ta.id = $1
		`, assignmentID).Scan(
			&bookingID,
			&eventID,
			&slotNumber,
			&assignStatus,
			&bookingStatus,
			&bookingUserID,
		)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ticket not found"})
			return
		}

		// Only the booking owner may request a QR token for their assignment.
		if bookingUserID == nil || *bookingUserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		// The assignment must be claimed (i.e. has an attendee) before a QR is useful.
		if assignStatus != string(models.AssignmentClaimed) {
			c.JSON(http.StatusConflict, gin.H{
				"error":  "Ticket is not yet active",
				"status": assignStatus,
			})
			return
		}

		// The parent booking must still be confirmed — cancellations/refunds invalidate QRs.
		if bookingStatus != string(models.BookingConfirmed) {
			c.JSON(http.StatusConflict, gin.H{
				"error":          "Booking is no longer valid",
				"booking_status": bookingStatus,
			})
			return
		}

		eid := ""
		if eventID != nil {
			eid = *eventID
		}

		token, err := rotatingQR.GenerateToken(assignmentID, bookingID, eid, slotNumber)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate QR token"})
			return
		}

		c.JSON(http.StatusOK, token)
	}
}

// ScanRotatingQRHandler godoc
//
//	POST /v1/tickets/:assignmentId/scan
//
// Organizer endpoint — verifies a rotating QR token and admits the attendee.
//
// Verification order:
//  1. HMAC + window check (crypto only, no DB).
//  2. Assignment and booking status checks (must be claimed/confirmed).
//  3. Event admission window check (±2h around event).
//  4. Atomic single-entry mark — sets qr_scanned_at WHERE qr_scanned_at IS NULL.
//     A race-safe 0-rows-affected means the ticket was already used.
//
// Auth: user JWT (must be the organizer for the event)
//
// Request body:  { "token": "<rotating-qr-token>" }
func ScanRotatingQRHandler(
	pool *pgxpool.Pool,
	rotatingQR *services.RotatingQRService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		scannerUserID := c.MustGet("user_id").(string)

		var input struct {
			Token string `json:"token" binding:"required"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
			return
		}

		// ── Step 1: crypto verification (stateless) ───────────────────────────
		payload, err := rotatingQR.VerifyToken(input.Token)
		if err != nil {
			// Keep the error generic — don't signal whether it was expired vs. forged.
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired QR code"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		// ── Step 2: DB status checks ──────────────────────────────────────────
		var (
			assignStatus  string
			scannedAt     *time.Time
			bookingStatus string
			startTime     time.Time
			endTime       time.Time
			organizerUserID string // user_id of the event's organizer
		)

		err = pool.QueryRow(ctx, `
			SELECT
				ta.status,
				ta.qr_scanned_at,
				b.booking_status,
				e.start_time,
				e.end_time,
				o.user_id AS organizer_user_id
			FROM ticket_assignments ta
			JOIN bookings  b ON ta.booking_id = b.id
			JOIN events    e ON b.event_id    = e.id
			JOIN organizers o ON o.user_id = e.organizer_id
			WHERE ta.id = $1
		`, payload.AssignmentID).Scan(
			&assignStatus,
			&scannedAt,
			&bookingStatus,
			&startTime,
			&endTime,
			&organizerUserID,
		)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ticket not found"})
			return
		}

		// Only the event organizer may scan tickets at the door.
		if organizerUserID != scannerUserID {
			c.JSON(http.StatusForbidden, gin.H{"error": "Only the event organizer can scan tickets"})
			return
		}

		if assignStatus != string(models.AssignmentClaimed) {
			c.JSON(http.StatusConflict, gin.H{
				"error":  "Ticket is not valid",
				"status": assignStatus,
			})
			return
		}

		if bookingStatus != string(models.BookingConfirmed) {
			c.JSON(http.StatusConflict, gin.H{
				"error":          "Booking is no longer valid",
				"booking_status": bookingStatus,
			})
			return
		}

		// ── Step 3: event admission window ────────────────────────────────────
		now := time.Now()
		if now.Before(startTime.Add(-2*time.Hour)) || now.After(endTime.Add(2*time.Hour)) {
			c.JSON(http.StatusConflict, gin.H{"error": "Outside event admission window"})
			return
		}

		// ── Step 4: atomic single-entry mark ──────────────────────────────────
		// The WHERE qr_scanned_at IS NULL guard makes this race-condition safe:
		// if two staff members scan simultaneously, only one UPDATE wins.
		tag, err := pool.Exec(ctx, `
			UPDATE ticket_assignments
			SET qr_scanned_at = NOW()
			WHERE id = $1
			  AND qr_scanned_at IS NULL
		`, payload.AssignmentID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record scan"})
			return
		}

		if tag.RowsAffected() == 0 {
			// Ticket was already admitted — fetch when it was first scanned for the error.
			var firstScan time.Time
			_ = pool.QueryRow(ctx,
				`SELECT qr_scanned_at FROM ticket_assignments WHERE id = $1`,
				payload.AssignmentID,
			).Scan(&firstScan)

			c.JSON(http.StatusConflict, gin.H{
				"error":         "Ticket already scanned",
				"first_scan_at": firstScan,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":       "Admitted",
			"assignment_id": payload.AssignmentID,
			"slot_number":   payload.SlotNumber,
			"booking_id":    payload.BookingID,
			"scanned_at":    now,
		})

		// Best-effort: log the scan with the scanner's user ID for audit trails.
		// We fire-and-forget since the response is already sent.
		go func() {
			auditCtx, auditCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer auditCancel()
			_, _ = pool.Exec(auditCtx, `
				UPDATE ticket_assignments
				SET scanned_by_organizer_id = (
					SELECT id FROM organizers WHERE user_id = $1 LIMIT 1
				)
				WHERE id = $2
			`, scannerUserID, payload.AssignmentID)
		}()
	}
}