package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── RFID/NFC tag handlers (Phase 1: online) ───────────────────────────────────
//
// Authorisation reuses requireCanScan / requireStaffManager from the existing
// scan handlers rather than introducing a parallel notion of "tag staff": a
// person who may admit an attendee with a QR code may admit the same attendee
// with a wristband, and revoking them cuts off both at once. A separate
// permission axis here would be one more thing to remember to revoke.
//
// A note on route shapes. Every path below is static under /tags — there is no
// /tags/:uid. Sibling static and wildcard segments at the same level are a
// known source of router-tree conflicts, and it costs nothing to sidestep the
// question: single-tag lookup is GET /tags?uid=..., which is also friendlier to
// UIDs that would otherwise need escaping.

// TapMaxSkewedClock bounds how far a device's clock may sit from the server's
// before its reported scan time is replaced. The device clock is display-only
// in scan_events (received_at is the ordering authority), but a wildly wrong
// value still pollutes the exceptions report a human reads.
const TapMaxSkewedClock = 15 * time.Minute

// ── Batches ──────────────────────────────────────────────────────────────────

// CreateTagBatchHandler opens an encoding run.
//
//	POST /v1/staff/events/:id/tag-batches
//	{ "label": "Main gate wristbands", "chip_type": "ntag424",
//	  "key_version": 1, "expected_count": 5000 }
func CreateTagBatchHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			Label         string `json:"label" binding:"required,max=120"`
			ChipType      string `json:"chip_type" binding:"required"`
			KeyVersion    int    `json:"key_version"`
			ExpectedCount int    `json:"expected_count"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if in.KeyVersion <= 0 {
			in.KeyVersion = 1
		}
		if in.ExpectedCount < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "expected_count cannot be negative"})
			return
		}

		chip := models.ChipType(in.ChipType)
		if _, ok := models.VerificationClassFor(chip); !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Unknown chip_type",
				"supported": []string{
					string(models.ChipNTAG424), string(models.ChipDESFireEV2),
					string(models.ChipDESFireEV3), string(models.ChipUIDOnly),
				},
			})
			return
		}

		batch, err := repository.CreateTagBatch(ctx, pool, eventID, in.Label, chip,
			in.KeyVersion, in.ExpectedCount, c.MustGet("user_id").(string))
		if err != nil {
			log.Error().Err(err).Str("event_id", eventID).Msg("rfid: create batch failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create batch"})
			return
		}

		resp := gin.H{"batch": batch}
		// Said plainly at the moment of choosing, not buried in a doc: a UID-only
		// batch is an access-control decision that permanently excludes these
		// tags from the wallet, and the exclusion is enforced in the database.
		if chip == models.ChipUIDOnly {
			resp["warning"] = "UID-only chips are cloneable. Permitted for access control only — " +
				"these tags can never be bound to a wallet."
		}
		c.JSON(http.StatusCreated, resp)
	}
}

// ListTagBatchesHandler
//
//	GET /v1/staff/events/:id/tag-batches
func ListTagBatchesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}
		batches, err := repository.ListTagBatches(ctx, pool, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load batches"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"batches": batches})
	}
}

// ── Registration ─────────────────────────────────────────────────────────────

// RegisterTagsHandler imports UIDs from an encoding run.
//
//	POST /v1/staff/events/:id/tags/register
//	{ "batch_id": "...", "tag_uids": ["04A1B2C3D4E580", ...] }
//
// Per-UID verdicts rather than all-or-nothing: an operator re-running a partly
// completed import of twenty thousand rows must be able to make progress
// instead of being blocked by the rows that already landed.
func RegisterTagsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		eventID := requireStaffManager(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			BatchID string   `json:"batch_id" binding:"required,uuid"`
			TagUIDs []string `json:"tag_uids" binding:"required"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if len(in.TagUIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tag_uids cannot be empty"})
			return
		}
		if len(in.TagUIDs) > 2000 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Max 2000 UIDs per request — split the import into pages",
			})
			return
		}

		// The batch fixes chip type and key version. Taking them from the batch
		// rather than the request body means a mistyped chip_type on page 7 of
		// an import cannot produce a batch encoded two different ways.
		var (
			chipType   string
			keyVersion int
			batchEvent string
		)
		err := pool.QueryRow(ctx, `
			SELECT chip_type::text, key_version, event_id
			FROM rfid_tag_batches WHERE id = $1
		`, in.BatchID).Scan(&chipType, &keyVersion, &batchEvent)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Batch not found"})
			return
		}
		if batchEvent != eventID {
			c.JSON(http.StatusForbidden, gin.H{"error": "Batch belongs to a different event"})
			return
		}

		chip := models.ChipType(chipType)
		class, ok := models.VerificationClassFor(chip)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Batch has an unknown chip type"})
			return
		}

		// Canonicalise before touching the database so a formatting variant
		// cannot register as a second, unknown tag. Rejects are reported rather
		// than silently dropped — a malformed UID in an encoder export is a
		// problem with the export.
		normalised := make([]string, 0, len(in.TagUIDs))
		results := make([]repository.RegisterTagResult, 0, len(in.TagUIDs))
		for _, raw := range in.TagUIDs {
			uid, err := services.NormaliseUID(raw)
			if err != nil {
				results = append(results, repository.RegisterTagResult{
					TagUID: raw, Status: "error", Error: "malformed UID",
				})
				continue
			}
			normalised = append(normalised, uid)
		}

		inserted, err := repository.RegisterTags(ctx, pool, eventID, in.BatchID,
			chip, class, keyVersion, normalised, c.MustGet("user_id").(string))
		if err != nil {
			log.Error().Err(err).Str("event_id", eventID).Msg("rfid: register tags failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register tags"})
			return
		}
		results = append(results, inserted...)

		var registered, duplicate, failed int
		for _, r := range results {
			switch r.Status {
			case "registered":
				registered++
			case "duplicate":
				duplicate++
			default:
				failed++
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"results": results,
			"summary": gin.H{"registered": registered, "duplicate": duplicate, "failed": failed},
		})
	}
}

// ── Lookup ───────────────────────────────────────────────────────────────────

// ListTagsHandler lists tags, or resolves one by UID.
//
//	GET /v1/staff/events/:id/tags
//	GET /v1/staff/events/:id/tags?uid=04A1B2C3D4E580
//	GET /v1/staff/events/:id/tags?since=RFC3339      (delta — Phase 2 sync)
func ListTagsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		if raw := c.Query("uid"); raw != "" {
			uid, err := services.NormaliseUID(raw)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Malformed tag UID"})
				return
			}
			tag, err := repository.GetTagByUID(ctx, pool, eventID, uid)
			if errors.Is(err, repository.ErrTagNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Tag not registered for this event"})
				return
			}
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tag"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"tag": tag})
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

		tags, err := repository.ListTags(ctx, pool, eventID, since, 1000)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tags"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"tags": tags, "cursor": time.Now()})
	}
}

// ── Binding & lifecycle ──────────────────────────────────────────────────────

// BindTagHandler attaches a tag to a ticket or a booking pool and activates it.
//
//	POST /v1/staff/events/:id/tags/bind
//	{ "tag_uid": "...", "assignment_id": "..." }
//	{ "tag_uid": "...", "booking_id": "..." }
func BindTagHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			TagUID       string `json:"tag_uid" binding:"required"`
			AssignmentID string `json:"assignment_id"`
			BookingID    string `json:"booking_id"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if (in.AssignmentID == "") == (in.BookingID == "") {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Provide exactly one of assignment_id or booking_id",
			})
			return
		}

		uid, err := services.NormaliseUID(in.TagUID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Malformed tag UID"})
			return
		}
		actor := c.MustGet("user_id").(string)

		// The target must belong to THIS event. Without this check a staffer at
		// event A could bind a wristband to a ticket for event B, and the tap
		// path — which resolves strictly within its own event — would then
		// refuse it with a confusing "not registered" at the gate.
		if in.AssignmentID != "" {
			if _, err := uuid.Parse(in.AssignmentID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment_id"})
				return
			}
			var owns bool
			if err := pool.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM ticket_assignments ta
					JOIN bookings b ON b.id = ta.booking_id
					WHERE ta.id = $1 AND b.event_id = $2
				)`, in.AssignmentID, eventID).Scan(&owns); err != nil || !owns {
				c.JSON(http.StatusNotFound, gin.H{"error": "Assignment not found for this event"})
				return
			}
			err = repository.BindTagToAssignment(ctx, pool, eventID, uid, in.AssignmentID, actor)
		} else {
			if _, err := uuid.Parse(in.BookingID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid booking_id"})
				return
			}
			var owns bool
			if err := pool.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM bookings WHERE id = $1 AND event_id = $2)`,
				in.BookingID, eventID).Scan(&owns); err != nil || !owns {
				c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found for this event"})
				return
			}
			err = repository.BindTagToBooking(ctx, pool, eventID, uid, in.BookingID, actor)
		}

		switch {
		case errors.Is(err, repository.ErrTagNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Tag not registered for this event"})
		case errors.Is(err, repository.ErrTagAlreadyBound):
			c.JSON(http.StatusConflict, gin.H{
				"error": "Tag is already bound or is not in a bindable state. " +
					"Deactivate it first if you are re-issuing.",
			})
		case err != nil:
			log.Error().Err(err).Str("event_id", eventID).Msg("rfid: bind failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to bind tag"})
		default:
			c.JSON(http.StatusOK, gin.H{"message": "Tag bound", "tag_uid": uid})
		}
	}
}

// SetTagStatusHandler retires, reports lost, or writes off a tag.
//
//	POST /v1/staff/events/:id/tags/status
//	{ "tag_uid": "...", "status": "lost", "reason": "attendee reported at gate 3" }
func SetTagStatusHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}

		var in struct {
			TagUID string `json:"tag_uid" binding:"required"`
			Status string `json:"status" binding:"required"`
			Reason string `json:"reason" binding:"max=500"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Only the transitions gate staff should be making. 'active' is
		// deliberately absent: activation happens through bind, so a tag can
		// never be switched on without something to admit.
		var kind models.TagEventKind
		switch models.TagStatus(in.Status) {
		case models.TagDeactivated:
			kind = models.TagEventDeactivated
		case models.TagLost:
			kind = models.TagEventReportedLost
		case models.TagDestroyed:
			kind = models.TagEventDeactivated
		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Unsupported status transition",
				"allowed": []string{"deactivated", "lost", "destroyed"},
			})
			return
		}

		uid, err := services.NormaliseUID(in.TagUID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Malformed tag UID"})
			return
		}

		err = repository.SetTagStatus(ctx, pool, eventID, uid,
			models.TagStatus(in.Status), kind, c.MustGet("user_id").(string), in.Reason)
		switch {
		case errors.Is(err, repository.ErrTagNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Tag not registered for this event"})
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update tag"})
		default:
			c.JSON(http.StatusOK, gin.H{"message": "Tag updated", "tag_uid": uid, "status": in.Status})
		}
	}
}

// ── The tap ──────────────────────────────────────────────────────────────────

// TapTagHandler admits an attendee from an NFC tap.
//
//	POST /v1/staff/events/:id/tags/tap
//	{ "device_id": "...", "chip_type": "ntag424",
//	  "sdm": { "picc_data": "...", "cmac": "..." } }
//
// The sequence deliberately mirrors ScanRotatingQRHandler step for step — chip
// crypto, DB status, admission window, atomic single-entry mark, ledger append
// — because the two paths must be able to disagree about the medium and about
// nothing else. A wristband and a phone screen presenting the same ticket have
// to produce the same verdict, or the exceptions report becomes fiction.
//
// The one genuinely new step is the replay guard between crypto and status.
func TapTagHandler(
	pool *pgxpool.Pool,
	tagCrypto *services.TagCryptoService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		eventID, _ := requireCanScan(c, ctx, pool)
		if eventID == "" {
			return
		}
		scannerUserID := c.MustGet("user_id").(string)

		var in struct {
			DeviceID string `json:"device_id" binding:"required,max=64"`
			services.TapRequest
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// ── Step 1: chip verification ─────────────────────────────────────────
		// 422 rather than 401 for the same reason the QR path uses it: the
		// bearer JWT is fine, it is the credential the attendee presented that
		// is not. A 401 here would trip the client's auto-logout interceptor and
		// throw the staffer back to a login screen mid-queue.
		versions, err := repository.EventKeyVersions(ctx, pool, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load key versions"})
			return
		}

		var (
			verified   *services.TapVerification
			verifyErr  error
			keyVersion int
		)
		for _, v := range versions {
			verified, verifyErr = tagCrypto.VerifyTap(eventID, v, &in.TapRequest)
			if verifyErr == nil {
				keyVersion = v
				break
			}
		}
		if verifyErr != nil {
			// Refusals are ledgered even when no tag row resolves — an
			// unrecognised or forged tap is exactly the event an investigation
			// later needs, and it is the one most easily lost.
			uid := in.TagUID
			if uid == "" {
				uid = "unresolved"
			}
			if lerr := repository.RecordTagScan(ctx, pool, eventID, nil, nil, uid,
				1, scannerUserID, repository.ScanVerdictRejected); lerr != nil {
				log.Warn().Err(lerr).Str("event_id", eventID).
					Msg("rfid: refused tap not written to scan ledger")
			}
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":  "Tag could not be verified",
				"detail": verifyErr.Error(),
			})
			return
		}

		// ── Step 2: resolve the tag ───────────────────────────────────────────
		tag, err := repository.GetTagByUID(ctx, pool, eventID, verified.UID)
		if errors.Is(err, repository.ErrTagNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Tag is not registered for this event",
				"tag_uid": verified.UID,
			})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tag"})
			return
		}

		// The stored key version is authoritative once the tag is known. A
		// mismatch means the tag verified under a version other than the one on
		// record — recoverable, but it belongs in the audit trail.
		if keyVersion != tag.KeyVersion {
			log.Warn().Str("tag_uid", tag.TagUID).
				Int("verified_with", keyVersion).Int("recorded", tag.KeyVersion).
				Msg("rfid: tag verified under an unexpected key version")
		}

		if !tag.Status.Admissible() {
			repository.AppendTagEvent(ctx, pool, tag.ID, eventID, models.TagEventTapRefused,
				&scannerUserID, &in.DeviceID, map[string]any{"reason": "status", "status": tag.Status})
			c.JSON(http.StatusConflict, gin.H{
				"error":  "This tag is not active",
				"status": tag.Status,
			})
			return
		}

		// ── Step 3: replay guard ──────────────────────────────────────────────
		// Only server-verified chips carry a counter. For the other classes
		// there is simply no freshness evidence, and pretending otherwise would
		// be worse than admitting the gap: the honest position is that a
		// reader-verified tap is trusted because the reader is, which is what
		// attestation in Phase 4 addresses.
		if verified.ReadCounter != nil && tag.ChipType == models.ChipNTAG424 {
			fresh, err := repository.BumpReadCounter(ctx, pool, tag.ID, *verified.ReadCounter)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record tap counter"})
				return
			}
			if !fresh {
				repository.AppendTagEvent(ctx, pool, tag.ID, eventID, models.TagEventTapRefused,
					&scannerUserID, &in.DeviceID, map[string]any{
						"reason": "replayed_counter", "counter": *verified.ReadCounter,
					})
				if lerr := repository.RecordTagScan(ctx, pool, eventID,
					tag.AssignmentID, tag.BookingID, tag.TagUID,
					1, scannerUserID, repository.ScanVerdictRejected); lerr != nil {
					log.Warn().Err(lerr).Msg("rfid: replay refusal not ledgered")
				}
				c.JSON(http.StatusConflict, gin.H{
					"error": "Replayed tap — this exact tag message has been seen before",
				})
				return
			}
		}

		// ── Step 4: binding and booking state ─────────────────────────────────
		if tag.AssignmentID == nil {
			// Pool-bound tags admit N heads and need the live remaining count.
			// That is the same resolution ScanHandler already performs, so
			// Phase 1 stops here rather than reimplementing it badly; pool
			// wristbands land with the Phase 2 manifest work.
			c.JSON(http.StatusNotImplemented, gin.H{
				"error": "Pool-bound tags are not admitted in Phase 1 — bind to an assignment",
			})
			return
		}
		if tag.AssignmentStatus == nil || *tag.AssignmentStatus != string(models.AssignmentClaimed) {
			c.JSON(http.StatusConflict, gin.H{
				"error":  "Ticket is not valid",
				"status": tag.AssignmentStatus,
			})
			return
		}
		if tag.BookingStatus == nil || *tag.BookingStatus != string(models.BookingConfirmed) {
			c.JSON(http.StatusConflict, gin.H{
				"error":          "Booking is no longer valid",
				"booking_status": tag.BookingStatus,
			})
			return
		}

		// ── Step 5: admission window ──────────────────────────────────────────
		var (
			startTime, endTime       time.Time
			scanOpenOff, scanCloseOff *int
		)
		if err := pool.QueryRow(ctx, `
			SELECT start_time, end_time, scan_open_offset_min, scan_close_offset_min
			FROM events WHERE id = $1
		`, eventID).Scan(&startTime, &endTime, &scanOpenOff, &scanCloseOff); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}
		now := time.Now()
		scanOpen, scanClose := models.ScanWindow(startTime, endTime, scanOpenOff, scanCloseOff)
		if now.Before(scanOpen) || now.After(scanClose) {
			c.JSON(http.StatusConflict, gin.H{"error": "Outside event admission window"})
			return
		}

		// ── Step 6: atomic single-entry mark ──────────────────────────────────
		// Identical guard to the QR path, and that is the point: a ticket
		// already admitted by QR cannot be admitted again by wristband, because
		// both compete for the same qr_scanned_at IS NULL.
		res, err := pool.Exec(ctx, `
			UPDATE ticket_assignments
			SET qr_scanned_at           = NOW(),
			    scanned_by_user_id      = $2,
			    scanned_by_organizer_id = (SELECT id FROM organizers WHERE user_id = $2 LIMIT 1)
			WHERE id = $1
			  AND qr_scanned_at IS NULL
		`, *tag.AssignmentID, scannerUserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record scan"})
			return
		}

		if res.RowsAffected() == 0 {
			if lerr := repository.RecordTagScan(ctx, pool, eventID,
				tag.AssignmentID, tag.BookingID, tag.TagUID,
				1, scannerUserID, repository.ScanVerdictDuplicate); lerr != nil {
				log.Warn().Err(lerr).Str("tag_uid", tag.TagUID).
					Msg("rfid: duplicate not written to scan ledger")
			}
			resp := gin.H{"error": "Ticket already scanned"}
			if tag.QRScannedAt != nil {
				resp["first_scan_at"] = *tag.QRScannedAt
			}
			c.JSON(http.StatusConflict, resp)
			return
		}

		// ── Step 7: ledger ────────────────────────────────────────────────────
		// Best-effort, exactly as on the QR path: the attendee is already
		// through the gate, and a failed audit write must not become a refusal.
		// Logged at error level because a persistent failure here silently
		// under-reports attendance and blocks instant-tier payout promotion.
		if lerr := repository.RecordTagScan(ctx, pool, eventID,
			tag.AssignmentID, tag.BookingID, tag.TagUID,
			1, scannerUserID, repository.ScanVerdictAdmitted); lerr != nil {
			log.Error().Err(lerr).
				Str("tag_uid", tag.TagUID).Str("event_id", eventID).
				Msg("rfid: admitted but not written to scan ledger — attendance will under-report")
		}

		repository.AppendTagEvent(ctx, pool, tag.ID, eventID, models.TagEventActivated,
			&scannerUserID, &in.DeviceID, map[string]any{
				"verification_class": string(verified.Class),
				"warnings":           verified.Warnings,
			})

		c.JSON(http.StatusOK, gin.H{
			"message":            "Admitted",
			"tag_uid":            tag.TagUID,
			"assignment_id":      *tag.AssignmentID,
			"slot_number":        tag.AssignmentSlot,
			"attendee":           tag.AssigneeName,
			"tier":               tag.TierName,
			"verification_class": verified.Class,
			"warnings":           verified.Warnings,
			"scanned_at":         now,
		})
	}
}