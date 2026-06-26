package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// withinAdmissionWindow mirrors the single-ticket scan gate: entry is allowed
// from 2h before the event starts to 2h after it ends.
func withinAdmissionWindow(start, end time.Time) bool {
	now := time.Now()
	return !now.Before(start.Add(-2*time.Hour)) && !now.After(end.Add(2*time.Hour))
}

// ScanHandler is the unified door-scan endpoint. It accepts any rotating QR
// token and dispatches on its type:
//
//   - individual token → admit that one assigned ticket (boolean; idempotent by
//     nature because a re-scan hits the same row's "already scanned" guard).
//   - pool token        → admit `count` heads from the booking's group pool. A
//     request with no `count` (or count<=0) is a PEEK that returns how many
//     remain WITHOUT admitting, so the scanner can show the headcount picker.
//     A mutating admit requires an `idempotency_key` so a network retry returns
//     the same result instead of admitting again.
//
// Auth: organizer JWT (must own the event).
//
//	POST /v1/tickets/scan   { token, count?, idempotency_key? }
func ScanHandler(
	pool *pgxpool.Pool,
	rotatingQR *services.RotatingQRService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		scannerUserID := c.MustGet("user_id").(string)

		var input struct {
			Token          string `json:"token" binding:"required"`
			Count          int    `json:"count"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
			return
		}

		// Stateless crypto check. 422 (not 401) so the organiser app's
		// auto-logout-on-401 interceptor doesn't fire on a bad QR.
		payload, err := rotatingQR.VerifyToken(input.Token)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Invalid or expired QR code"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		if payload.IsPool() {
			scanPool(c, ctx, pool, scannerUserID, payload, input.Count, input.IdempotencyKey)
			return
		}
		scanIndividual(c, ctx, pool, scannerUserID, payload)
	}
}

// scanIndividual admits a single assigned ticket — the existing boolean path.
func scanIndividual(
	c *gin.Context,
	ctx context.Context,
	pool *pgxpool.Pool,
	scannerUserID string,
	payload *services.RotatingQRPayload,
) {
	var (
		assignStatus    string
		bookingStatus   string
		startTime       time.Time
		endTime         time.Time
		organizerUserID string
	)
	err := pool.QueryRow(ctx, `
		SELECT ta.status, b.booking_status, e.start_time, e.end_time, e.organizer_id
		FROM ticket_assignments ta
		JOIN bookings b ON ta.booking_id = b.id
		JOIN events   e ON b.event_id    = e.id
		WHERE ta.id = $1
	`, payload.AssignmentID).Scan(&assignStatus, &bookingStatus, &startTime, &endTime, &organizerUserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Ticket not found"})
		return
	}
	if organizerUserID != scannerUserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the event organizer can scan tickets"})
		return
	}
	if assignStatus != string(models.AssignmentClaimed) {
		c.JSON(http.StatusConflict, gin.H{"error": "Ticket is not valid", "status": assignStatus})
		return
	}
	if bookingStatus != string(models.BookingConfirmed) {
		c.JSON(http.StatusConflict, gin.H{"error": "Booking is no longer valid", "booking_status": bookingStatus})
		return
	}
	if !withinAdmissionWindow(startTime, endTime) {
		c.JSON(http.StatusConflict, gin.H{"error": "Outside event admission window"})
		return
	}

	tag, err := pool.Exec(ctx, `
		UPDATE ticket_assignments SET qr_scanned_at = NOW()
		WHERE id = $1 AND qr_scanned_at IS NULL
	`, payload.AssignmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record scan"})
		return
	}
	if tag.RowsAffected() == 0 {
		var firstScan *time.Time
		_ = pool.QueryRow(ctx, `SELECT qr_scanned_at FROM ticket_assignments WHERE id = $1`, payload.AssignmentID).Scan(&firstScan)
		resp := gin.H{"error": "Ticket already scanned"}
		if firstScan != nil {
			resp["first_scan_at"] = *firstScan
		}
		c.JSON(http.StatusConflict, resp)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type":          "individual",
		"message":       "Admitted",
		"admitted":      1,
		"assignment_id": payload.AssignmentID,
		"slot_number":   payload.SlotNumber,
		"booking_id":    payload.BookingID,
	})

	// Best-effort audit (fire-and-forget; response already sent).
	go func() {
		auditCtx, auditCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer auditCancel()
		_, _ = pool.Exec(auditCtx, `
			UPDATE ticket_assignments
			SET scanned_by_organizer_id = (SELECT id FROM organizers WHERE user_id = $1 LIMIT 1)
			WHERE id = $2
		`, scannerUserID, payload.AssignmentID)
	}()
}

// scanPool peeks or admits N heads from a booking's group pool.
func scanPool(
	c *gin.Context,
	ctx context.Context,
	pool *pgxpool.Pool,
	scannerUserID string,
	payload *services.RotatingQRPayload,
	count int,
	idemKey string,
) {
	var (
		bookingStatus   string
		bookingNumber   string
		eventTitle      string
		startTime       time.Time
		endTime         time.Time
		organizerUserID string
	)
	err := pool.QueryRow(ctx, `
		SELECT b.booking_status, b.booking_number, e.title, e.start_time, e.end_time, e.organizer_id
		FROM bookings b
		JOIN events  e ON b.event_id = e.id
		WHERE b.id = $1
	`, payload.BookingID).Scan(&bookingStatus, &bookingNumber, &eventTitle, &startTime, &endTime, &organizerUserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
		return
	}
	if organizerUserID != scannerUserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the event organizer can scan tickets"})
		return
	}
	if bookingStatus != string(models.BookingConfirmed) {
		c.JSON(http.StatusConflict, gin.H{"error": "Booking is no longer valid", "booking_status": bookingStatus})
		return
	}
	if !withinAdmissionWindow(startTime, endTime) {
		c.JSON(http.StatusConflict, gin.H{"error": "Outside event admission window"})
		return
	}

	remaining, err := repository.CountPoolRemaining(ctx, pool, payload.BookingID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read pool"})
		return
	}

	// PEEK — no count requested. Tell the scanner how many remain; admit nothing.
	if count <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"type":           "pool",
			"peek":           true,
			"remaining":      remaining,
			"booking_number": bookingNumber,
			"event_title":    eventTitle,
		})
		return
	}

	// Mutating admit requires an idempotency key so retries don't double-admit.
	if idemKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "idempotency_key is required to admit"})
		return
	}
	if found, prevAdmitted, ierr := repository.GetScanIdempotency(ctx, pool, idemKey); ierr == nil && found {
		c.JSON(http.StatusOK, gin.H{
			"type":           "pool",
			"message":        "Admitted",
			"admitted":       prevAdmitted,
			"remaining":      remaining,
			"idempotent":     true,
			"booking_number": bookingNumber,
			"event_title":    eventTitle,
		})
		return
	}

	if remaining == 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":          "All tickets in this pass have already been admitted",
			"remaining":      0,
			"booking_number": bookingNumber,
		})
		return
	}

	toAdmit := count
	if toAdmit > remaining {
		toAdmit = remaining
	}
	slots, err := repository.AdmitFromPool(ctx, pool, payload.BookingID, toAdmit, scannerUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to admit"})
		return
	}
	admitted := len(slots)
	_ = repository.SaveScanIdempotency(ctx, pool, idemKey, payload.BookingID, admitted, scannerUserID)

	c.JSON(http.StatusOK, gin.H{
		"type":           "pool",
		"message":        "Admitted",
		"admitted":       admitted,
		"remaining":      remaining - admitted,
		"slot_numbers":   slots,
		"booking_number": bookingNumber,
		"event_title":    eventTitle,
	})
}

// ─── Pool QR token endpoints (the buyer's group pass) ─────────────────────────

// GetPoolQRTokenHandler issues a rotating GROUP token for the buyer's pool.
// GET /v1/bookings/:id/pool-qr
func GetPoolQRTokenHandler(
	pool *pgxpool.Pool,
	rotatingQR *services.RotatingQRService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet("user_id").(string)
		bookingID := c.Param("id")
		if _, err := uuid.Parse(bookingID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid booking ID"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var eventID *string
		var bookingStatus string
		err := pool.QueryRow(ctx, `
			SELECT event_id, booking_status FROM bookings WHERE id = $1 AND user_id = $2
		`, bookingID, userID).Scan(&eventID, &bookingStatus)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}
		poolQRRespond(c, ctx, pool, rotatingQR, bookingID, eventID, bookingStatus)
	}
}

// GetGuestPoolQRTokenHandler issues a rotating GROUP token for a guest's pool,
// authenticated by booking number + email.
// GET /v1/guest/bookings/:bookingNumber/pool-qr?email=
func GetGuestPoolQRTokenHandler(
	pool *pgxpool.Pool,
	rotatingQR *services.RotatingQRService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookingNumber := c.Param("bookingNumber")
		email := strings.TrimSpace(c.Query("email"))
		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var bookingID string
		var eventID *string
		var bookingStatus string
		err := pool.QueryRow(ctx, `
			SELECT id, event_id, booking_status
			FROM bookings
			WHERE booking_number = $1 AND is_guest = true AND lower(attendee_email) = lower($2)
		`, bookingNumber, email).Scan(&bookingID, &eventID, &bookingStatus)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}
		poolQRRespond(c, ctx, pool, rotatingQR, bookingID, eventID, bookingStatus)
	}
}

// poolQRRespond is the shared tail for both pool-QR endpoints: validate the
// booking is confirmed, mint a pool token, and attach the remaining count.
func poolQRRespond(
	c *gin.Context,
	ctx context.Context,
	pool *pgxpool.Pool,
	rotatingQR *services.RotatingQRService,
	bookingID string,
	eventID *string,
	bookingStatus string,
) {
	if bookingStatus != string(models.BookingConfirmed) {
		c.JSON(http.StatusConflict, gin.H{"error": "Booking is not confirmed", "booking_status": bookingStatus})
		return
	}
	eid := ""
	if eventID != nil {
		eid = *eventID
	}
	token, err := rotatingQR.GeneratePoolToken(bookingID, eid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate QR token"})
		return
	}
	remaining, _ := repository.CountPoolRemaining(ctx, pool, bookingID)
	c.JSON(http.StatusOK, gin.H{
		"token":      token.Token,
		"expires_at": token.ExpiresAt,
		"window_sec": token.WindowSec,
		"issued_at":  token.IssuedAt,
		"remaining":  remaining,
	})
}