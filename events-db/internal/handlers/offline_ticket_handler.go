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

// offlineAdmissionGrace mirrors the scan gate: entry is allowed up to 2h after
// the event ends, so offline tokens stay valid across the whole admission window.
const offlineAdmissionGrace = 2 * time.Hour

// offlineAssignment is one individually-assigned ticket in the offline bundle.
type offlineAssignment struct {
	AssignmentID string `json:"assignment_id"`
	SlotNumber   int    `json:"slot_number"`
	Token        string `json:"token"`
}

// offlineBundle is everything the app needs to render a booking's entry passes
// with no network: a group pool token (if the buyer still holds unassigned
// tickets) plus one token per claimed individual assignment. The app caches
// this at purchase time and falls back to it whenever the live rotating QR
// can't be fetched.
type offlineBundle struct {
	BookingID     string              `json:"booking_id"`
	GeneratedAt   time.Time           `json:"generated_at"`
	ExpiresAt     time.Time           `json:"expires_at"`
	PoolToken     *string             `json:"pool_token"`     // nil when nothing is left in the pool
	PoolRemaining int                 `json:"pool_remaining"` // heads still admissible from the pool
	Assignments   []offlineAssignment `json:"assignments"`
}

// GetOfflineTicketsHandler returns the offline token bundle for an authed
// buyer's own booking.
//
//	GET /v1/bookings/:id/offline
//
// Auth: user JWT (booking owner).
func GetOfflineTicketsHandler(
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

		var (
			eventID       *string
			bookingStatus string
			startTime     *time.Time
			endTime       *time.Time
		)
		err := pool.QueryRow(ctx, `
			SELECT b.event_id, b.booking_status, e.start_time, e.end_time
			FROM bookings b
			LEFT JOIN events e ON b.event_id = e.id
			WHERE b.id = $1 AND b.user_id = $2
		`, bookingID, userID).Scan(&eventID, &bookingStatus, &startTime, &endTime)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		respondOfflineBundle(c, ctx, pool, rotatingQR, bookingID, eventID, bookingStatus, endTime)
	}
}

// GetGuestOfflineTicketsHandler returns the offline token bundle for a guest
// booking, authenticated by booking number + email (same scheme as the guest
// rotating-QR endpoints).
//
//	GET /v1/guest/bookings/:bookingNumber/offline?email=
func GetGuestOfflineTicketsHandler(
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

		var (
			bookingID     string
			eventID       *string
			bookingStatus string
			startTime     *time.Time
			endTime       *time.Time
		)
		err := pool.QueryRow(ctx, `
			SELECT b.id, b.event_id, b.booking_status, e.start_time, e.end_time
			FROM bookings b
			LEFT JOIN events e ON b.event_id = e.id
			WHERE b.booking_number = $1 AND b.is_guest = true AND lower(b.attendee_email) = lower($2)
		`, bookingNumber, email).Scan(&bookingID, &eventID, &bookingStatus, &startTime, &endTime)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
			return
		}

		respondOfflineBundle(c, ctx, pool, rotatingQR, bookingID, eventID, bookingStatus, endTime)
	}
}

// respondOfflineBundle is the shared tail for both offline endpoints. It
// validates the booking, computes the admission-window expiry, mints an offline
// pool token (if anything remains in the pool) plus one offline token per
// claimed assignment, and returns the bundle.
func respondOfflineBundle(
	c *gin.Context,
	ctx context.Context,
	pool *pgxpool.Pool,
	rotatingQR *services.RotatingQRService,
	bookingID string,
	eventID *string,
	bookingStatus string,
	endTime *time.Time,
) {
	// Only confirmed bookings have valid entry passes.
	if bookingStatus != string(models.BookingConfirmed) {
		c.JSON(http.StatusConflict, gin.H{
			"error":          "Booking is not confirmed",
			"booking_status": bookingStatus,
		})
		return
	}

	// Offline entry passes only make sense for event bookings with a known end
	// time — merch-only bookings have neither an event nor a gate.
	if eventID == nil || endTime == nil {
		c.JSON(http.StatusOK, offlineBundle{
			BookingID:     bookingID,
			GeneratedAt:   time.Now(),
			ExpiresAt:     time.Now(),
			PoolToken:     nil,
			PoolRemaining: 0,
			Assignments:   []offlineAssignment{},
		})
		return
	}

	eid := *eventID
	exp := endTime.Add(offlineAdmissionGrace)

	bundle := offlineBundle{
		BookingID:   bookingID,
		GeneratedAt: time.Now(),
		ExpiresAt:   exp,
		Assignments: []offlineAssignment{},
	}

	// ── Pool token — covers every head still with the buyer ───────────────────
	remaining, _ := repository.CountPoolRemaining(ctx, pool, bookingID)
	bundle.PoolRemaining = remaining
	if remaining > 0 {
		poolTok, err := rotatingQR.GenerateOfflinePoolToken(bookingID, eid, exp)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate offline pool token"})
			return
		}
		bundle.PoolToken = &poolTok.Token
	}

	// ── Individual tokens — one per claimed assignment ────────────────────────
	rows, err := pool.Query(ctx, `
		SELECT id, slot_number
		FROM ticket_assignments
		WHERE booking_id = $1 AND status = 'claimed'
		ORDER BY slot_number
	`, bookingID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load assignments"})
		return
	}
	defer rows.Close()

	type claimed struct {
		id   string
		slot int
	}
	var claimedList []claimed
	for rows.Next() {
		var cl claimed
		if err := rows.Scan(&cl.id, &cl.slot); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read assignments"})
			return
		}
		claimedList = append(claimedList, cl)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read assignments"})
		return
	}

	for _, cl := range claimedList {
		tok, err := rotatingQR.GenerateOfflineToken(cl.id, bookingID, eid, cl.slot, exp)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate offline token"})
			return
		}
		bundle.Assignments = append(bundle.Assignments, offlineAssignment{
			AssignmentID: cl.id,
			SlotNumber:   cl.slot,
			Token:        tok.Token,
		})
	}

	c.JSON(http.StatusOK, bundle)
}