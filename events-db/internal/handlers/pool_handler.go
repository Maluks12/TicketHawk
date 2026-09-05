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
	"github.com/rs/zerolog/log"
)

// withinAdmissionWindow defers to models.ScanWindow, the single source of
// truth for the gate.
//
// It used to hardcode "start-2h to end+2h" and read neither offset column, so
// this endpoint silently ignored every custom window an organiser set — the
// rotating-QR gate honoured them and this one did not, on the same event.
// Taking the offsets as arguments is what keeps the two paths in step.
func withinAdmissionWindow(start, end time.Time, openOffsetMin, closeOffsetMin *int) bool {
	open, close := models.ScanWindow(start, end, openOffsetMin, closeOffsetMin)
	now := time.Now()
	return !now.Before(open) && !now.After(close)
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
		eventID         string
		assignStatus    string
		bookingID       string
		bookingStatus   string
		startTime       time.Time
		endTime         time.Time
		scanOpenOff     *int
		scanCloseOff    *int
		organizerUserID string
		isStaff         bool
		// Tier the holder bought. Read from bookings.ticket_type_name, which is
		// denormalised at booking time on purpose — the door needs the name the
		// buyer actually paid for, and it must survive the organiser later
		// renaming or deleting the tier.
		ticketTypeName *string
	)
	// e.id and ta.booking_id are selected for the scan_events row written
	// below; the ledger foreign-keys both.
	err := pool.QueryRow(ctx, `
		SELECT e.id, ta.status, ta.booking_id, b.booking_status,
		       e.start_time, e.end_time,
		       e.scan_open_offset_min, e.scan_close_offset_min,
		       e.organizer_id,
		       (es.user_id IS NOT NULL) AS is_staff,
		       b.ticket_type_name
		FROM ticket_assignments ta
		JOIN bookings b ON ta.booking_id = b.id
		JOIN events   e ON b.event_id    = e.id
		LEFT JOIN event_staff es
		       ON es.event_id = e.id AND es.user_id = $2 AND es.status = 'active'
		WHERE ta.id = $1
	`, payload.AssignmentID, scannerUserID).Scan(&eventID, &assignStatus, &bookingID, &bookingStatus,
		&startTime, &endTime, &scanOpenOff, &scanCloseOff,
		&organizerUserID, &isStaff, &ticketTypeName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Ticket not found"})
		return
	}
	if organizerUserID != scannerUserID && !isStaff && c.GetString("role") != string(models.RoleAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have scanning access for this event"})
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
	if !withinAdmissionWindow(startTime, endTime, scanOpenOff, scanCloseOff) {
		c.JSON(http.StatusConflict, gin.H{"error": "Outside event admission window"})
		return
	}

	// Scanner columns are set in the same atomic statement rather than in a
	// fire-and-forget goroutine — see the note in ScanRotatingQRHandler.
	tag, err := pool.Exec(ctx, `
		UPDATE ticket_assignments
		SET qr_scanned_at           = NOW(),
		    scanned_by_user_id      = $2,
		    scanned_by_organizer_id = (SELECT id FROM organizers WHERE user_id = $2 LIMIT 1)
		WHERE id = $1 AND qr_scanned_at IS NULL
	`, payload.AssignmentID, scannerUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record scan"})
		return
	}
	if tag.RowsAffected() == 0 {
		var firstScan *time.Time
		_ = pool.QueryRow(ctx, `SELECT qr_scanned_at FROM ticket_assignments WHERE id = $1`, payload.AssignmentID).Scan(&firstScan)

		if lerr := repository.RecordOnlineScan(ctx, pool, eventID,
			&payload.AssignmentID, &bookingID, payload.AssignmentID,
			1, scannerUserID, repository.ScanVerdictDuplicate,
		); lerr != nil {
			log.Warn().Err(lerr).Str("assignment_id", payload.AssignmentID).
				Msg("scan: duplicate not written to scan ledger")
		}

		resp := gin.H{"error": "Ticket already scanned"}
		if firstScan != nil {
			resp["first_scan_at"] = *firstScan
		}
		c.JSON(http.StatusConflict, resp)
		return
	}

	// Best-effort ledger append — the attendee is already admitted, and an
	// audit failure must never become a refusal at the door.
	if lerr := repository.RecordOnlineScan(ctx, pool, eventID,
		&payload.AssignmentID, &bookingID, payload.AssignmentID,
		1, scannerUserID, repository.ScanVerdictAdmitted,
	); lerr != nil {
		log.Error().Err(lerr).Str("assignment_id", payload.AssignmentID).
			Str("event_id", eventID).
			Msg("scan: admitted but not written to scan ledger — attendance and payout eligibility will under-report")
	}

	// "General Admission" rather than an empty string when the event is flat-
	// priced: the door staff need to read something definite off the screen, and
	// a blank field looks like a failed lookup.
	tier := "General Admission"
	if ticketTypeName != nil && *ticketTypeName != "" {
		tier = *ticketTypeName
	}

	c.JSON(http.StatusOK, gin.H{
		"type":             "individual",
		"message":          "Admitted",
		"admitted":         1,
		"assignment_id":    payload.AssignmentID,
		"slot_number":      payload.SlotNumber,
		"booking_id":       payload.BookingID,
		"ticket_type_name": tier,
	})
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
		eventID         string
		bookingStatus   string
		bookingNumber   string
		eventTitle      string
		startTime       time.Time
		endTime         time.Time
		scanOpenOff     *int
		scanCloseOff    *int
		organizerUserID string
		isStaff         bool
		// A pool covers one booking, so every head under it shares one tier.
		ticketTypeName *string
	)
	// e.id is selected for the scan_events row written after a successful
	// admit — the ledger foreign-keys it.
	err := pool.QueryRow(ctx, `
		SELECT e.id, b.booking_status, b.booking_number, e.title,
		       e.start_time, e.end_time,
		       e.scan_open_offset_min, e.scan_close_offset_min,
		       e.organizer_id,
		       (es.user_id IS NOT NULL) AS is_staff,
		       b.ticket_type_name
		FROM bookings b
		JOIN events  e ON b.event_id = e.id
		LEFT JOIN event_staff es
		       ON es.event_id = e.id AND es.user_id = $2 AND es.status = 'active'
		WHERE b.id = $1
	`, payload.BookingID, scannerUserID).Scan(&eventID, &bookingStatus, &bookingNumber, &eventTitle,
		&startTime, &endTime, &scanOpenOff, &scanCloseOff,
		&organizerUserID, &isStaff, &ticketTypeName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
		return
	}
	if organizerUserID != scannerUserID && !isStaff && c.GetString("role") != string(models.RoleAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have scanning access for this event"})
		return
	}
	// Same fallback as the individual path — door staff need a definite label.
	poolTier := "General Admission"
	if ticketTypeName != nil && *ticketTypeName != "" {
		poolTier = *ticketTypeName
	}
	if bookingStatus != string(models.BookingConfirmed) {
		c.JSON(http.StatusConflict, gin.H{"error": "Booking is no longer valid", "booking_status": bookingStatus})
		return
	}
	if !withinAdmissionWindow(startTime, endTime, scanOpenOff, scanCloseOff) {
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
	// Idempotent replay: this returns the ORIGINAL admission, which is already
	// in the scan ledger. Deliberately does NOT write another ledger row —
	// doing so would double-count those heads in the analytics door figure.
	if found, prevAdmitted, ierr := repository.GetScanIdempotency(ctx, pool, idemKey); ierr == nil && found {
		c.JSON(http.StatusOK, gin.H{
			"type":             "pool",
			"message":          "Admitted",
			"admitted":         prevAdmitted,
			"remaining":        remaining,
			"idempotent":       true,
			"booking_number":   bookingNumber,
			"event_title":      eventTitle,
			"ticket_type_name": poolTier,
		})
		return
	}

	if remaining == 0 {
		// Every head on this pass is already inside. Ledger it so an online
		// gate leaves the same trail an offline one would.
		if lerr := repository.RecordOnlineScan(ctx, pool, eventID,
			nil, &payload.BookingID, payload.BookingID,
			count, scannerUserID, repository.ScanVerdictDuplicate,
		); lerr != nil {
			log.Warn().Err(lerr).Str("booking_id", payload.BookingID).
				Msg("scan: pool duplicate not written to scan ledger")
		}
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

	// One ledger row for the whole pool admit, carrying the number of heads
	// ACTUALLY admitted. The analytics door figure is SUM(admit_count), so
	// writing the REQUESTED count here would over-report a partial admit.
	// assignment_id is nil: a pool scan admits N slots, not one named
	// assignment — the same shape ingestOne writes on the offline path.
	if admitted > 0 {
		if lerr := repository.RecordOnlineScan(ctx, pool, eventID,
			nil, &payload.BookingID, payload.BookingID,
			admitted, scannerUserID, repository.ScanVerdictAdmitted,
		); lerr != nil {
			log.Error().Err(lerr).Str("booking_id", payload.BookingID).
				Str("event_id", eventID).Int("admitted", admitted).
				Msg("scan: pool admitted but not written to scan ledger — attendance and payout eligibility will under-report")
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"type":             "pool",
		"message":          "Admitted",
		"admitted":         admitted,
		"remaining":        remaining - admitted,
		"slot_numbers":     slots,
		"booking_number":   bookingNumber,
		"event_title":      eventTitle,
		"ticket_type_name": poolTier,
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