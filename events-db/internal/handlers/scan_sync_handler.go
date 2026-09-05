package handlers

import (
	"events-db/internal/models"
	"context"
	"errors"
	"net/http"
	"time"

	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Offline scanning sync endpoints ───────────────────────────────────────────
//
// All authenticate with the normal JWT and re-check LIVE canScan membership on
// every call — that's what makes revoking a staffer cut off their device's
// sync within one interval. The scanner credential these mint is for the
// device's LOCAL offline gating only; it never substitutes for auth here.

// Credential expiry derives from models.ScanCloses — the same start-anchored
// admission close every scan path uses — so a lost scanner phone is bounded by
// exactly the window the organiser set, and never outlives the gate.

// requireCanScan resolves the event and enforces owner/staff/admin scanning
// access. Returns (eventID, role) or writes the error and returns "".
func requireCanScan(c *gin.Context, ctx context.Context, pool *pgxpool.Pool) (string, string) {
	eventID := c.Param("id")
	if _, err := uuid.Parse(eventID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return "", ""
	}
	ok, role, err := repository.CanScanEvent(ctx, pool, eventID,
		c.MustGet("user_id").(string), c.GetString("role"))
	if err != nil {
		if errors.Is(err, repository.ErrEventNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check permissions"})
		}
		return "", ""
	}
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have scanning access for this event"})
		return "", ""
	}
	return eventID, role
}

// MintScannerCredentialHandler issues the device's offline credential.
//
//	POST /v1/staff/events/:id/scanner-credential   { device_id }
func MintScannerCredentialHandler(
	pool *pgxpool.Pool,
	creds *services.ScannerCredentialService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID, role := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		var input struct {
			DeviceID string `json:"device_id" binding:"required,max=64"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "device_id is required"})
			return
		}

		var startTime, endTime time.Time
		var scanCloseOff *int
		if err := pool.QueryRow(ctx,
			`SELECT start_time, end_time, scan_close_offset_min FROM events WHERE id = $1`, eventID,
		).Scan(&startTime, &endTime, &scanCloseOff); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}
		// Credential lifetime tracks the (possibly custom) admission-close time.
		// start_time is needed too now that the close is anchored to the start.
		exp := models.ScanCloses(startTime, endTime, scanCloseOff)
		if time.Now().After(exp) {
			c.JSON(http.StatusConflict, gin.H{"error": "This event's admission window has ended"})
			return
		}

		cred, err := creds.Mint(c.MustGet("user_id").(string), eventID, role, input.DeviceID, exp)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mint credential"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"credential": cred,
			"role":       role,
			"expires_at": exp,
		})
	}
}

// GetManifestHandler returns the attendee manifest (full, or delta with ?since=).
//
//	GET /v1/staff/events/:id/manifest?since=RFC3339
func GetManifestHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		var since time.Time
		if s := c.Query("since"); s != "" {
			t, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "since must be RFC3339"})
				return
			}
			since = t
		}

		m, err := repository.BuildManifest(ctx, pool, eventID, since)
		if err != nil {
			if errors.Is(err, repository.ErrEventNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build manifest"})
			}
			return
		}
		c.JSON(http.StatusOK, m)
	}
}

// IngestScansHandler accepts a device's batch of scan events, reconciles them,
// and returns per-scan verdicts. Idempotent by client-generated scan id.
//
//	POST /v1/staff/events/:id/scans   { device_id, scans: [...] }
func IngestScansHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		var input struct {
			DeviceID string                    `json:"device_id" binding:"required,max=64"`
			Scans    []repository.IncomingScan `json:"scans" binding:"required"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "device_id and scans are required"})
			return
		}
		if len(input.Scans) == 0 {
			c.JSON(http.StatusOK, gin.H{"results": []repository.ScanOutcome{}})
			return
		}
		if len(input.Scans) > 200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Max 200 scans per batch"})
			return
		}

		results, err := repository.IngestScans(ctx, pool, eventID,
			c.MustGet("user_id").(string), input.DeviceID, input.Scans)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to ingest scans"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
	}
}

// ScanExceptionsHandler returns the collisions + provisionals report.
// Manager-level: owner, manager-role staff, or admin.
//
//	GET /v1/events/:id/scan-exceptions?since=RFC3339
func ScanExceptionsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		var since time.Time
		if s := c.Query("since"); s != "" {
			t, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "since must be RFC3339"})
				return
			}
			since = t
		}

		exceptions, err := repository.ListScanExceptions(ctx, pool, eventID, since)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load exceptions"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"exceptions": exceptions, "cursor": time.Now()})
	}
}