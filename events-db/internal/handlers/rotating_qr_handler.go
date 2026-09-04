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

// GetGuestRotatingQRTokenHandler issues a rotating QR token for a guest's
// claimed ticket, authenticated by booking number + email instead of a JWT.
// It returns the exact same token format as the authed endpoint so guest
// tickets scan identically at the gate.
//
//	GET /v1/guest/bookings/:bookingNumber/assignments/:assignmentId/qr?email=
func GetGuestRotatingQRTokenHandler(
	pool *pgxpool.Pool,
	rotatingQR *services.RotatingQRService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookingNumber := c.Param("bookingNumber")
		assignmentID := c.Param("assignmentId")
		email := strings.TrimSpace(c.Query("email"))

		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
			return
		}
		if _, err := uuid.Parse(assignmentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment ID"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var (
			bookingID       string
			eventID         *string // nullable — merch-only bookings have no event
			slotNumber      int
			assignStatus    string
			bookingStatus   string
			bookingNumberDB string
			attendeeEmail   *string
			isGuest         bool
		)

		err := pool.QueryRow(ctx, `
			SELECT
				ta.booking_id,
				b.event_id,
				ta.slot_number,
				ta.status,
				b.booking_status,
				b.booking_number,
				b.attendee_email,
				b.is_guest
			FROM ticket_assignments ta
			JOIN bookings b ON ta.booking_id = b.id
			WHERE ta.id = $1
		`, assignmentID).Scan(
			&bookingID,
			&eventID,
			&slotNumber,
			&assignStatus,
			&bookingStatus,
			&bookingNumberDB,
			&attendeeEmail,
			&isGuest,
		)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ticket not found"})
			return
		}

		// Guest auth: the assignment's booking must be a guest booking whose
		// number + email match what the caller supplied.
		if !isGuest || bookingNumberDB != bookingNumber ||
			attendeeEmail == nil || !strings.EqualFold(*attendeeEmail, email) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		if assignStatus != string(models.AssignmentClaimed) {
			c.JSON(http.StatusConflict, gin.H{"error": "Ticket is not yet active", "status": assignStatus})
			return
		}
		if bookingStatus != string(models.BookingConfirmed) {
			c.JSON(http.StatusConflict, gin.H{"error": "Booking is no longer valid", "booking_status": bookingStatus})
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
			// Use 422 (Unprocessable Entity), NOT 401 — the bearer JWT is valid;
			// it's the QR payload itself that's expired or forged.
			// 401 would fire the frontend's auto-logout interceptor which is wrong
			// here and silently redirects the organizer to the login page.
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Invalid or expired QR code"})
			return
		}

		// ── Step 1b: URL param must match the verified token payload ─────────
		// The handler resolves the assignment via payload.AssignmentID (HMAC-
		// verified), but we also validate the URL param for consistency — a
		// mismatch hints at a client bug or crafted request.
		if urlID := c.Param("assignmentId"); urlID != payload.AssignmentID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Assignment ID mismatch between URL and token"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		// ── Step 2: DB status checks ──────────────────────────────────────────
		// e.id and ta.booking_id are selected for the scan_events row written at
		// the end. The ledger foreign-keys both, and reading them costs nothing
		// because the query already joins both tables.
		//
		// Note: ta.qr_scanned_at is intentionally NOT selected here — the
		// duplicate-entry guard is handled atomically by the UPDATE below
		// (WHERE qr_scanned_at IS NULL). Fetching it upfront would add a
		// nullable TIMESTAMPTZ scan that serves no purpose in this path.
		var (
			eventID         string
			assignStatus    string
			bookingID       string
			bookingStatus   string
			startTime       time.Time
			endTime         time.Time
			scanOpenOff     *int   // custom scan-open offset (min before start); nil = default 2h
			scanCloseOff    *int   // custom scan-close offset (min after end);  nil = default 2h
			organizerUserID string // equals events.organizer_id which REFERENCES users(id)
			isStaff         bool   // active event_staff membership for the scanner
		)

		err = pool.QueryRow(ctx, `
			SELECT
				e.id,
				ta.status,
				ta.booking_id,
				b.booking_status,
				e.start_time,
				e.end_time,
				e.scan_open_offset_min,
				e.scan_close_offset_min,
				e.organizer_id AS organizer_user_id,
				(es.user_id IS NOT NULL) AS is_staff
			FROM ticket_assignments ta
			JOIN bookings  b ON ta.booking_id = b.id
			JOIN events    e ON b.event_id    = e.id
			LEFT JOIN event_staff es
			       ON es.event_id = e.id
			      AND es.user_id  = $2
			      AND es.status   = 'active'
			WHERE ta.id = $1
		`, payload.AssignmentID, scannerUserID).Scan(
			&eventID,
			&assignStatus,
			&bookingID,
			&bookingStatus,
			&startTime,
			&endTime,
			&scanOpenOff,
			&scanCloseOff,
			&organizerUserID,
			&isStaff,
		)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ticket not found"})
			return
		}

		// Owner, active event staff, or platform admin may scan at the door.
		if organizerUserID != scannerUserID && !isStaff && c.GetString("role") != string(models.RoleAdmin) {
			c.JSON(http.StatusForbidden, gin.H{"error": "You don't have scanning access for this event"})
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
		scanOpen, scanClose := models.ScanWindow(startTime, endTime, scanOpenOff, scanCloseOff)
		if now.Before(scanOpen) || now.After(scanClose) {
			c.JSON(http.StatusConflict, gin.H{"error": "Outside event admission window"})
			return
		}

		// ── Step 4: atomic single-entry mark ──────────────────────────────────
		// The WHERE qr_scanned_at IS NULL guard makes this race-condition safe:
		// if two staff members scan simultaneously, only one UPDATE wins.
		//
		// The scanner columns are set in the SAME statement. They used to be
		// written by a fire-and-forget goroutine after the response was sent,
		// which was wrong twice over: a panic on a non-handler goroutine is not
		// covered by gin.Recovery() and would take the whole process down, and
		// one goroutine per scan is unbounded on a busy gate. Folding them in
		// costs nothing extra, makes the audit atomic with the admission, and
		// matches what ingestOne already does on the offline path.
		tag, err := pool.Exec(ctx, `
			UPDATE ticket_assignments
			SET qr_scanned_at           = NOW(),
			    scanned_by_user_id      = $2,
			    scanned_by_organizer_id = (SELECT id FROM organizers WHERE user_id = $2 LIMIT 1)
			WHERE id = $1
			  AND qr_scanned_at IS NULL
		`, payload.AssignmentID, scannerUserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record scan"})
			return
		}

		if tag.RowsAffected() == 0 {
			// Ticket was already admitted — fetch when it was first scanned for the UI.
			// Use *time.Time so that a NULL (shouldn't happen, but defensive) or a
			// failed sub-query doesn't send "0001-01-01T00:00:00Z" to the client.
			var firstScan *time.Time
			_ = pool.QueryRow(ctx,
				`SELECT qr_scanned_at FROM ticket_assignments WHERE id = $1`,
				payload.AssignmentID,
			).Scan(&firstScan)

			// Ledger the refused re-scan too. 'duplicate' is excluded from both
			// the analytics sum and the exceptions report, so it moves no
			// figure — but without it a re-scan leaves no trace at all.
			if lerr := repository.RecordOnlineScan(ctx, pool, eventID,
				&payload.AssignmentID, &bookingID, payload.AssignmentID,
				1, scannerUserID, repository.ScanVerdictDuplicate,
			); lerr != nil {
				log.Warn().Err(lerr).
					Str("assignment_id", payload.AssignmentID).
					Msg("scan: duplicate not written to scan ledger")
			}

			resp := gin.H{"error": "Ticket already scanned"}
			if firstScan != nil {
				resp["first_scan_at"] = *firstScan
			}
			c.JSON(http.StatusConflict, resp)
			return
		}

		// ── Step 5: append to the scan ledger ─────────────────────────────────
		// Best-effort by design. The attendee is already admitted by the UPDATE
		// above, and refusing them because an audit row failed to write would be
		// the worse failure at a gate. Logged at error level because a persistent
		// failure here silently under-reports attendance and blocks instant-tier
		// promotion — which is exactly how this went unnoticed before.
		if lerr := repository.RecordOnlineScan(ctx, pool, eventID,
			&payload.AssignmentID, &bookingID, payload.AssignmentID,
			1, scannerUserID, repository.ScanVerdictAdmitted,
		); lerr != nil {
			log.Error().Err(lerr).
				Str("assignment_id", payload.AssignmentID).
				Str("event_id", eventID).
				Msg("scan: admitted but not written to scan ledger — attendance and payout eligibility will under-report")
		}

		c.JSON(http.StatusOK, gin.H{
			"message":       "Admitted",
			"assignment_id": payload.AssignmentID,
			"slot_number":   payload.SlotNumber,
			"booking_id":    payload.BookingID,
			"scanned_at":    now,
		})
	}
}

// GetClaimedRotatingQRTokenHandler godoc
//
//	GET /v1/tickets/claimed/:assignmentId/qr?email=<claimant email>
//
// Issues a rotating QR token for a CLAIMED ticket, authenticated by the
// assignment ID (a UUID the claimant received via their claim link/email)
// plus the email address they claimed the ticket with. This mirrors the
// guest endpoint's auth model (unguessable ID + email) and exists because:
//
//   - the authed endpoint only allows the BOOKING OWNER (buyer), and
//   - the guest endpoint only covers guest bookings authed by the BUYER's
//     email — a claimant invited from an account-holder's booking had no
//     way to mint a live entry QR at all.
//
// Returns the exact same token payload shape as the other QR endpoints so
// claimed tickets scan identically at the gate.
//
// Auth: none (public) — authorised by assignment UUID + claimant email match.
func GetClaimedRotatingQRTokenHandler(
	pool *pgxpool.Pool,
	rotatingQR *services.RotatingQRService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		assignmentID := c.Param("assignmentId")
		email := strings.TrimSpace(c.Query("email"))

		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
			return
		}
		if _, err := uuid.Parse(assignmentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment ID"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var (
			bookingID     string
			eventID       *string // nullable — merch-only bookings have no event
			slotNumber    int
			assignStatus  string
			bookingStatus string
			assigneeEmail *string
		)

		err := pool.QueryRow(ctx, `
			SELECT
				ta.booking_id,
				b.event_id,
				ta.slot_number,
				ta.status,
				b.booking_status,
				ta.assignee_email
			FROM ticket_assignments ta
			JOIN bookings b ON ta.booking_id = b.id
			WHERE ta.id = $1
		`, assignmentID).Scan(
			&bookingID,
			&eventID,
			&slotNumber,
			&assignStatus,
			&bookingStatus,
			&assigneeEmail,
		)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ticket not found"})
			return
		}

		// The email on file for this claimed slot must match the caller's.
		// Same 404-vs-403 shape as the guest endpoint: reveal nothing about
		// whether the assignment exists to a caller with the wrong email.
		if assigneeEmail == nil || !strings.EqualFold(*assigneeEmail, email) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		if assignStatus != string(models.AssignmentClaimed) {
			c.JSON(http.StatusConflict, gin.H{"error": "Ticket is not yet active", "status": assignStatus})
			return
		}
		if bookingStatus != string(models.BookingConfirmed) {
			c.JSON(http.StatusConflict, gin.H{"error": "Booking is no longer valid", "booking_status": bookingStatus})
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