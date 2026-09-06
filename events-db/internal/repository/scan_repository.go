package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Offline scanning: manifest + scan-event ledger ────────────────────────────

// CanScanEvent mirrors the per-scan authorization used by the scan handlers:
// event owner, platform admin, or active event_staff member. Returned role
// feeds the scanner credential.
func CanScanEvent(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, userID, globalRole string,
) (bool, string, error) {
	if globalRole == "admin" {
		var one int
		if err := pool.QueryRow(ctx, `SELECT 1 FROM events WHERE id = $1`, eventID).Scan(&one); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, "", ErrEventNotFound
			}
			return false, "", err
		}
		return true, "admin", nil
	}

	var isOwner bool
	var staffRole *string
	err := pool.QueryRow(ctx, `
		SELECT e.organizer_id = $2,
		       (SELECT es.role::text FROM event_staff es
		        WHERE es.event_id = e.id AND es.user_id = $2 AND es.status = 'active')
		FROM events e WHERE e.id = $1
	`, eventID, userID).Scan(&isOwner, &staffRole)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", ErrEventNotFound
		}
		return false, "", err
	}
	if isOwner {
		return true, "owner", nil
	}
	if staffRole != nil {
		return true, *staffRole, nil
	}
	return false, "", nil
}

// ── Manifest ──────────────────────────────────────────────────────────────────

type ManifestAssignment struct {
	AssignmentID string     `json:"assignment_id"`
	BookingID    string     `json:"booking_id"`
	SlotNumber   int        `json:"slot_number"`
	Name         string     `json:"name"`
	Tier         string     `json:"tier"`
	Status       string     `json:"status"`     // claimed / unassigned / ...
	BookingOK    bool       `json:"booking_ok"` // booking still confirmed (refunds kill this)
	ScannedAt    *time.Time `json:"scanned_at"` // set = someone already admitted it
}

type ManifestPool struct {
	BookingID     string `json:"booking_id"`
	BookingNumber string `json:"booking_number"`
	BuyerName     string `json:"buyer_name"`
	// Tier for the whole pool. Offline scanning must show the same label as an
	// online scan, or door staff get different answers depending on signal.
	Tier          string `json:"tier"`
	PoolTotal     int    `json:"pool_total"`    // unassigned slots on the booking
	PoolAdmitted  int    `json:"pool_admitted"` // of those, already admitted
	BookingOK     bool   `json:"booking_ok"`
}

type Manifest struct {
	EventID     string               `json:"event_id"`
	Title       string               `json:"title"`
	StartTime   time.Time            `json:"start_time"`
	EndTime     time.Time            `json:"end_time"`
	Cursor      time.Time            `json:"cursor"` // pass back as ?since= for deltas
	Assignments []ManifestAssignment `json:"assignments"`
	Pools       []ManifestPool       `json:"pools"`

	// Tags is the RFID/NFC wristband mapping. Empty for events with no tag
	// programme, so a scanner that ignores the field behaves exactly as before.
	Tags []ManifestTag `json:"tags"`
}

// ManifestTag maps one wristband to what it admits, for offline resolution on
// the handset.
//
// Under the manifest-authoritative trust model the device holds no verification
// keys, so this mapping IS the offline check: UID present, status active, and
// the assignment it points at not yet admitted. Everything the device cannot
// know — a band revoked or re-bound since this snapshot — is caught server-side
// at sync by ClassifyTagScan.
//
// Status ships as a plain string rather than a boolean because the device shows
// door staff WHY a band was refused, and "this band was reported lost" is a
// different conversation at the gate from "this ticket is already inside".
type ManifestTag struct {
	TagUID       string  `json:"tag_uid"`
	Status       string  `json:"status"`
	AssignmentID *string `json:"assignment_id"`
	BookingID    *string `json:"booking_id"`
	ChipType     string  `json:"chip_type"`
}

// BuildManifest returns the event's attendee manifest — full when since is
// zero, otherwise only rows whose assignment or booking changed after it.
// Other gates' admissions propagate through this same delta (their scans bump
// updated_at via qr_scanned_at), which is how cross-gate dedupe reaches every
// device without any device-to-device channel.
func BuildManifest(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID string,
	since time.Time,
) (*Manifest, error) {
	m := &Manifest{EventID: eventID, Assignments: []ManifestAssignment{},
		Pools: []ManifestPool{}, Tags: []ManifestTag{}}

	err := pool.QueryRow(ctx, `
		SELECT title, start_time, end_time FROM events WHERE id = $1
	`, eventID).Scan(&m.Title, &m.StartTime, &m.EndTime)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, err
	}

	// Cursor is server-now taken BEFORE the reads: a client passing it back can
	// re-receive a row changed mid-query but can never miss one. Client-side
	// application is an upsert, so overlap is harmless.
	m.Cursor = time.Now()

	rows, err := pool.Query(ctx, `
		SELECT ta.id, ta.booking_id, ta.slot_number,
		       COALESCE(ta.assignee_name, ''),
		       COALESCE(tt.name, 'General Admission'),
		       ta.status::text,
		       b.booking_status = 'confirmed',
		       ta.qr_scanned_at
		FROM ticket_assignments ta
		JOIN bookings b       ON b.id = ta.booking_id
		LEFT JOIN ticket_types tt ON tt.id = b.ticket_type_id
		WHERE b.event_id = $1
		  AND ($2::timestamptz IS NULL OR ta.updated_at > $2 OR b.updated_at > $2)
		ORDER BY ta.booking_id, ta.slot_number
	`, eventID, nullableTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a ManifestAssignment
		if err := rows.Scan(&a.AssignmentID, &a.BookingID, &a.SlotNumber,
			&a.Name, &a.Tier, &a.Status, &a.BookingOK, &a.ScannedAt); err != nil {
			return nil, err
		}
		m.Assignments = append(m.Assignments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Pool summaries: one row per booking that has unassigned slots. Buyer name
	// resolves to the account holder, or the guest email prefix for guest buys.
	prows, err := pool.Query(ctx, `
		SELECT b.id, b.booking_number,
		       COALESCE(NULLIF(TRIM(COALESCE(u.first_name,'') || ' ' || COALESCE(u.last_name,'')), ''),
		                split_part(COALESCE(b.attendee_email, ''), '@', 1),
		                'Guest'),
		       COUNT(*) FILTER (WHERE ta.status = 'unassigned'),
		       COUNT(*) FILTER (WHERE ta.status = 'unassigned' AND ta.qr_scanned_at IS NOT NULL),
		       b.booking_status = 'confirmed',
		       COALESCE(NULLIF(b.ticket_type_name, ''), 'General Admission')
		FROM bookings b
		JOIN ticket_assignments ta ON ta.booking_id = b.id
		LEFT JOIN users u ON u.id = b.user_id
		WHERE b.event_id = $1
		  AND ($2::timestamptz IS NULL OR ta.updated_at > $2 OR b.updated_at > $2)
		GROUP BY b.id, b.booking_number, u.first_name, u.last_name, b.attendee_email, b.booking_status,
		         b.ticket_type_name
		HAVING COUNT(*) FILTER (WHERE ta.status = 'unassigned') > 0
	`, eventID, nullableTime(since))
	if err != nil {
		return nil, err
	}
	defer prows.Close()
	for prows.Next() {
		var p ManifestPool
		if err := prows.Scan(&p.BookingID, &p.BookingNumber, &p.BuyerName,
			&p.PoolTotal, &p.PoolAdmitted, &p.BookingOK, &p.Tier); err != nil {
			return nil, err
		}
		m.Pools = append(m.Pools, p)
	}
	if err := prows.Err(); err != nil {
		return nil, err
	}

	// Tags ride the SAME cursor as assignments and pools, so the scanner keeps
	// one sync loop and one "since" value. Revocations propagate through this
	// delta the way other gates' admissions already do: SetTagStatus bumps
	// updated_at, so a band reported lost at the box office reaches every
	// handset on its next sync without any device-to-device channel.
	//
	// Deactivated and lost bands are shipped rather than filtered out. A device
	// that simply stopped seeing a UID would treat the next tap as an unknown
	// band and admit it as a provisional; shipping the revocation lets it
	// refuse at the gate and say why.
	trows, err := pool.Query(ctx, `
		SELECT rt.tag_uid, rt.status::text, rt.assignment_id, rt.booking_id, rt.chip_type::text
		FROM rfid_tags rt
		WHERE rt.event_id = $1
		  AND ($2::timestamptz IS NULL OR rt.updated_at > $2)
		ORDER BY rt.tag_uid
	`, eventID, nullableTime(since))
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var t ManifestTag
		if err := trows.Scan(&t.TagUID, &t.Status, &t.AssignmentID,
			&t.BookingID, &t.ChipType); err != nil {
			return nil, err
		}
		m.Tags = append(m.Tags, t)
	}
	return m, trows.Err()
}

func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// ── Ingest + reconciliation ───────────────────────────────────────────────────

// IncomingScan is one device-recorded scan event.
type IncomingScan struct {
	ID           string    `json:"id" binding:"required"` // client-generated UUID
	Kind         string    `json:"kind"`                  // "individual" | "pool"
	AssignmentID string    `json:"assignment_id"`         // individual
	BookingID    string    `json:"booking_id"`            // pool (and optional on individual)
	AdmitCount   int       `json:"admit_count"`           // pool: heads admitted at the gate
	Mode         string    `json:"mode"`                  // "online" | "offline"
	ScannedAt    time.Time `json:"scanned_at" binding:"required"`

	// Medium is how the credential was presented: "qr" (default) or "nfc".
	// Orthogonal to Mode, which is transport — a tap and a QR scan can each
	// arrive online or offline. Empty is treated as "qr" so devices running
	// older builds keep ingesting unchanged.
	Medium string `json:"medium,omitempty"`

	// TagUID is the wristband the device read, for "nfc" scans. It is the
	// device's CLAIM: the server re-resolves it against rfid_tags and may
	// override the admission entirely (see ClassifyTagScan).
	TagUID string `json:"tag_uid,omitempty"`
}

// ScanOutcome is the server's verdict for one incoming scan.
type ScanOutcome struct {
	ID           string     `json:"id"`
	Verdict      string     `json:"verdict"` // admitted / duplicate / collision / rejected / provisional
	AdmittedN    int        `json:"admitted_n,omitempty"`
	FirstScanAt  *time.Time `json:"first_scan_at,omitempty"`
	ConflictWith *string    `json:"conflict_with,omitempty"`

	// TagReason explains a tag-driven refusal ("tag_rebound_since_sync", ...)
	// so the handset can tell door staff why a band was turned away rather than
	// showing a bare verdict. Empty for every non-tag scan.
	TagReason string `json:"tag_reason,omitempty"`
}

// IngestScans applies a device's batch. Exactly-once per scan via the
// client-generated id (ON CONFLICT DO NOTHING); retried batches are free.
// First-received-at-server wins: a scan arriving for an already-admitted
// ticket is recorded as a collision pointing at the winner — the multi-gate
// exceptions report — never silently dropped.
func IngestScans(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, scannedBy, deviceID string,
	scans []IncomingScan,
) ([]ScanOutcome, error) {
	out := make([]ScanOutcome, 0, len(scans))

	for _, sc := range scans {
		o, err := ingestOne(ctx, pool, eventID, scannedBy, deviceID, sc)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, nil
}

// bookingStatusConfirmed mirrors models.BookingConfirmed. Kept as a local
// literal so this file does not take a dependency on the models package;
// the value is fixed by the booking_status enum in migration 000001.
const bookingStatusConfirmed = "confirmed"

// Scan media, mirroring the scan_medium enum added in migration 000085. Local
// literals for the same reason as bookingStatusConfirmed above.
const (
	scanMediumQR  = "qr"
	scanMediumNFC = "nfc"
)

func ingestOne(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, scannedBy, deviceID string,
	sc IncomingScan,
) (*ScanOutcome, error) {
	if _, err := uuid.Parse(sc.ID); err != nil {
		return &ScanOutcome{ID: sc.ID, Verdict: "rejected"}, nil
	}
	if sc.Mode != "online" {
		sc.Mode = "offline"
	}
	if sc.AdmitCount <= 0 {
		sc.AdmitCount = 1
	}
	// An absent medium means qr: devices on older builds send no field, and
	// their scans are exactly what they always were.
	if sc.Medium != scanMediumNFC {
		sc.Medium = scanMediumQR
		sc.TagUID = ""
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Resolve the claimed references against the DB so FKs hold; unresolved
	// individual refs become provisionals (ticket sold after our last sync, or
	// junk — the reconciler can't tell yet, the exceptions report can).
	var (
		assignID  *string
		bookingID *string
		rawRef    string
	)
	isPool := sc.Kind == "pool"

	// bookingConfirmed is resolved from the DB, never from the device.
	//
	// BuildManifest ships booking_status = 'confirmed' to the scanner as a
	// boolean and lets it gate locally, but a device runs on a manifest
	// snapshot: a booking cancelled or refunded after its last sync still
	// looks valid on that handset. Without this the server accepted the
	// device's verdict and recorded an 'admitted' scan against a refunded
	// ticket. Migration 000051 states the intent plainly -- verdict is
	// "SERVER-decided, not the device's claim" -- so re-check it here.
	bookingConfirmed := false

	if isPool {
		rawRef = sc.BookingID
		var status string
		if err := tx.QueryRow(ctx,
			`SELECT booking_status::text FROM bookings WHERE id = $1 AND event_id = $2`,
			sc.BookingID, eventID).Scan(&status); err == nil {
			bookingID = &sc.BookingID
			bookingConfirmed = status == bookingStatusConfirmed
		}
	} else {
		rawRef = sc.AssignmentID
		var bID, status string
		err := tx.QueryRow(ctx, `
			SELECT ta.booking_id, b.booking_status::text
			FROM ticket_assignments ta
			JOIN bookings b ON b.id = ta.booking_id
			WHERE ta.id = $1 AND b.event_id = $2
		`, sc.AssignmentID, eventID).Scan(&bID, &status)
		if err == nil {
			assignID = &sc.AssignmentID
			bookingID = &bID
			bookingConfirmed = status == bookingStatusConfirmed
		}
	}

	// Tag reconciliation, for nfc scans only.
	//
	// Resolved BEFORE the ledger insert so the verdict is settled by the time
	// the row is finalised, and read from the DB rather than trusted from the
	// device — the same rule migration 000051 sets out and that bookingConfirmed
	// above already follows. A handset offline through an outage can be holding
	// a mapping that was revoked hours ago.
	tagVerdict, tagReason := TagScanOK, ""
	var tagUID *string
	if sc.Medium == scanMediumNFC && sc.TagUID != "" {
		uid := sc.TagUID
		tagUID = &uid

		var st TagScanState
		err := tx.QueryRow(ctx, `
			SELECT status::text, assignment_id, booking_id
			FROM rfid_tags WHERE event_id = $1 AND tag_uid = $2
		`, eventID, uid).Scan(&st.Status, &st.BoundAssignmentID, &st.BoundBookingID)
		switch {
		case err == nil:
			st.Found = true
		case errors.Is(err, pgx.ErrNoRows):
			st.Found = false
		default:
			return nil, err
		}

		var claimed *string
		if !isPool && sc.AssignmentID != "" {
			claimed = &sc.AssignmentID
		}
		tagVerdict, tagReason = ClassifyTagScan(st, TagScanClaim{
			TagUID: uid, AssignmentID: claimed,
		})
	}

	// Idempotency gate: insert the ledger row first with a provisional verdict;
	// if the id already exists this batch item was processed before — return
	// the stored outcome and touch nothing.
	tag, err := tx.Exec(ctx, `
		INSERT INTO scan_events (id, event_id, assignment_id, booking_id, raw_ref,
		                         admit_count, scanned_by, device_id, mode, verdict, scanned_at,
		                         medium, tag_uid)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'provisional', $10, $11::scan_medium, $12)
		ON CONFLICT (id) DO NOTHING
	`, sc.ID, eventID, assignID, bookingID, rawRef,
		sc.AdmitCount, scannedBy, deviceID, sc.Mode, sc.ScannedAt,
		sc.Medium, tagUID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		var v string
		var cw *string
		if err := tx.QueryRow(ctx,
			`SELECT verdict::text, conflict_with::text FROM scan_events WHERE id = $1`, sc.ID,
		).Scan(&v, &cw); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &ScanOutcome{ID: sc.ID, Verdict: v, ConflictWith: cw}, nil
	}

	outcome := &ScanOutcome{ID: sc.ID}

	// The ledger row was inserted above with the CLAIMED admit_count. A pool
	// admit can go through for fewer heads than were claimed, and the analytics
	// door figure is SUM(admit_count) over admitted rows — so the claimed count
	// has to be corrected to the actual one before the row is finalised.
	admitCount := sc.AdmitCount

	switch {
	case tagVerdict != TagScanOK:
		// The band itself is the problem — revoked, re-bound, or unknown. Takes
		// precedence over every other check because it is the one the device
		// had no way to evaluate, and admitting nothing is the point.
		outcome.Verdict = tagVerdict
		outcome.TagReason = tagReason

	case bookingID != nil && !bookingConfirmed:
		// Resolved to a real booking that is cancelled, refunded, or still
		// pending. Record the attempt and admit nothing. 'rejected' is a
		// terminal verdict, so the device will not retry it, and the row stays
		// in the ledger for the organiser's exceptions report.
		outcome.Verdict = "rejected"

	case !isPool && assignID != nil:
		// Individual: apply iff still unscanned — the same atomic guard the
		// online scan path uses, now fed from the ledger.
		t2, err := tx.Exec(ctx, `
			UPDATE ticket_assignments
			SET qr_scanned_at = $2,
			    scanned_by_user_id = $3,
			    scanned_by_organizer_id = (SELECT id FROM organizers WHERE user_id = $3 LIMIT 1)
			WHERE id = $1 AND qr_scanned_at IS NULL
		`, *assignID, sc.ScannedAt, scannedBy)
		if err != nil {
			return nil, err
		}
		if t2.RowsAffected() == 1 {
			outcome.Verdict = "admitted"
			outcome.AdmittedN = 1
		} else {
			outcome.Verdict = "collision"
			var winner *string
			var first *time.Time
			_ = tx.QueryRow(ctx, `
				SELECT se.id::text, ta.qr_scanned_at
				FROM ticket_assignments ta
				LEFT JOIN scan_events se ON se.assignment_id = ta.id AND se.verdict = 'admitted'
				WHERE ta.id = $1
				ORDER BY se.received_at ASC NULLS LAST LIMIT 1
			`, *assignID).Scan(&winner, &first)
			outcome.ConflictWith = winner
			outcome.FirstScanAt = first
		}

	case isPool && bookingID != nil:
		// Pool: admit up to admit_count from the booking's unadmitted pool.
		//
		// FOR UPDATE, not FOR UPDATE SKIP LOCKED. SKIP LOCKED skips any row a
		// concurrent transaction merely holds a lock on, not only rows that are
		// genuinely spent — so two gates scanning the same pass in the same
		// instant produced a PHANTOM shortfall: the rows that did come back were
		// marked scanned and committed, while the verdict said 'collision'. The
		// party was told entry failed and their slots were burned anyway.
		//
		// ORDER BY slot_number gives every transaction the same lock order, so
		// blocking here cannot deadlock. Two scans of the SAME booking now
		// serialise, which is exactly the double-entry case that must serialise;
		// different bookings never contend for the same rows.
		rows, err := tx.Query(ctx, `
			WITH picked AS (
				SELECT id FROM ticket_assignments
				WHERE booking_id = $1 AND status = 'unassigned' AND qr_scanned_at IS NULL
				ORDER BY slot_number
				LIMIT $2
				FOR UPDATE
			)
			UPDATE ticket_assignments ta
			SET qr_scanned_at = $3,
			    scanned_by_user_id = $4,
			    scanned_by_organizer_id = (SELECT id FROM organizers WHERE user_id = $4 LIMIT 1)
			FROM picked WHERE ta.id = picked.id
			RETURNING ta.id
		`, *bookingID, sc.AdmitCount, sc.ScannedAt, scannedBy)
		if err != nil {
			return nil, err
		}
		n := 0
		for rows.Next() {
			n++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		outcome.AdmittedN = n

		if n == 0 {
			// Nothing left in the pool — every head on this pass is already
			// inside. THAT is the real exception, and it belongs in the
			// organiser's report. admit_count keeps the claimed value here
			// because scan_events carries CHECK (admit_count > 0).
			outcome.Verdict = "collision"
		} else {
			// n heads went through the door, so the ledger says n. A shortfall
			// (0 < n < claimed) is a pass that has been partly used already —
			// normal, not an exception. Calling it a collision under-reported
			// attendance by n while still burning the slots; calling it admitted
			// with the CLAIMED count would over-report it. The device still gets
			// AdmittedN, so the gate can show "3 of 5 admitted".
			outcome.Verdict = "admitted"
			admitCount = n
		}

	default:
		// Reference didn't resolve — sold after the device's last sync, wrong
		// event, or junk. Kept as provisional for the exceptions report.
		outcome.Verdict = "provisional"
	}

	if _, err := tx.Exec(ctx, `
		UPDATE scan_events SET verdict = $2, conflict_with = $3, admit_count = $4 WHERE id = $1
	`, sc.ID, outcome.Verdict, outcome.ConflictWith, admitCount); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return outcome, nil
}

// ── Exceptions report ─────────────────────────────────────────────────────────

type ScanException struct {
	ID           string     `json:"id"`
	Verdict      string     `json:"verdict"`
	AssignmentID *string    `json:"assignment_id,omitempty"`
	BookingID    *string    `json:"booking_id,omitempty"`
	RawRef       *string    `json:"raw_ref,omitempty"`
	Name         string     `json:"name"`
	SlotNumber   *int       `json:"slot_number,omitempty"`
	AdmitCount   int        `json:"admit_count"`
	ScannedBy    string     `json:"scanned_by"` // scanner's display name
	DeviceID     string     `json:"device_id"`
	ScannedAt    time.Time  `json:"scanned_at"`
	ReceivedAt   time.Time  `json:"received_at"`
	FirstScanAt  *time.Time `json:"first_scan_at,omitempty"`
}

// ListScanExceptions returns collisions + unresolved provisionals for the
// event — the multi-gate report — newest first, optionally after a cursor so
// devices can poll for fresh collisions cheaply.
func ListScanExceptions(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID string,
	since time.Time,
) ([]ScanException, error) {
	rows, err := pool.Query(ctx, `
		SELECT se.id, se.verdict::text, se.assignment_id, se.booking_id, se.raw_ref,
		       COALESCE(ta.assignee_name, ''),
		       ta.slot_number,
		       se.admit_count,
		       COALESCE(NULLIF(TRIM(COALESCE(u.first_name,'') || ' ' || COALESCE(u.last_name,'')), ''), u.email),
		       se.device_id, se.scanned_at, se.received_at,
		       ta.qr_scanned_at
		FROM scan_events se
		LEFT JOIN ticket_assignments ta ON ta.id = se.assignment_id
		JOIN users u ON u.id = se.scanned_by
		WHERE se.event_id = $1
		  AND se.verdict IN ('collision', 'provisional')
		  AND ($2::timestamptz IS NULL OR se.received_at > $2)
		ORDER BY se.received_at DESC
		LIMIT 200
	`, eventID, nullableTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ScanException{}
	for rows.Next() {
		var e ScanException
		if err := rows.Scan(&e.ID, &e.Verdict, &e.AssignmentID, &e.BookingID, &e.RawRef,
			&e.Name, &e.SlotNumber, &e.AdmitCount, &e.ScannedBy,
			&e.DeviceID, &e.ScannedAt, &e.ReceivedAt, &e.FirstScanAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}