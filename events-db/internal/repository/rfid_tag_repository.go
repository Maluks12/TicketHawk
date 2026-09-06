package repository

import (
	"context"
	"errors"
	"time"

	"events-db/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── RFID tag registry ─────────────────────────────────────────────────────────
//
// Reads and writes for migration 000085. Two things here are load-bearing and
// worth reading before changing anything:
//
//   - BumpReadCounter is the anti-replay control, and it is a conditional
//     UPDATE rather than a read-then-write. A gate with four handsets taps the
//     same wristband concurrently often enough that a check-then-set would let
//     a replayed capture through on the losing branch. The database decides.
//
//   - AppendTagEvent is best-effort at every call site, for the same reason
//     RecordOnlineScan is: the audit row must never be what turns somebody away
//     at a gate. Callers log failures and carry on.

// ScanVerdictRejected completes the set alongside ScanVerdictAdmitted and
// ScanVerdictDuplicate in scan_ledger_repository.go.
//
// It exists in the scan_verdict enum (migration 000051) but had no Go constant,
// because the QR path has no way to produce it: a forged QR token fails
// verification before any ticket is resolved, so there is nothing to ledger
// against. NFC changes that. A refused tap — bad cryptogram, replayed counter,
// unregistered chip — is a physical person at a physical gate being turned
// away, and it is the single most useful row in a fraud investigation.
//
// Like 'duplicate', it is excluded from the analytics attendance sum and from
// ListScanExceptions (which filters on collision/provisional), so adding it
// moves no existing figure.
const ScanVerdictRejected = "rejected"

var (
	ErrTagNotFound      = errors.New("tag not found")
	ErrTagAlreadyBound  = errors.New("tag already bound")
	ErrTagUIDRegistered = errors.New("tag uid already registered for this event")
)

// ── Batches ──────────────────────────────────────────────────────────────────

func CreateTagBatch(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, label string,
	chip models.ChipType,
	keyVersion, expected int,
	createdBy string,
) (*models.RFIDTagBatch, error) {
	b := &models.RFIDTagBatch{}
	err := pool.QueryRow(ctx, `
		INSERT INTO rfid_tag_batches
			(event_id, label, chip_type, key_version, expected_count, created_by)
		VALUES ($1, $2, $3::tag_chip_type, $4, $5, $6)
		RETURNING id, event_id, label, chip_type::text, key_version,
		          expected_count, activated_count, created_by, created_at, updated_at
	`, eventID, label, string(chip), keyVersion, expected, createdBy).Scan(
		&b.ID, &b.EventID, &b.Label, &b.ChipType, &b.KeyVersion,
		&b.ExpectedCount, &b.ActivatedCount, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func ListTagBatches(ctx context.Context, pool *pgxpool.Pool, eventID string) ([]models.RFIDTagBatch, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, event_id, label, chip_type::text, key_version,
		       expected_count, activated_count, created_by, created_at, updated_at
		FROM rfid_tag_batches
		WHERE event_id = $1
		ORDER BY created_at DESC
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.RFIDTagBatch{}
	for rows.Next() {
		var b models.RFIDTagBatch
		if err := rows.Scan(&b.ID, &b.EventID, &b.Label, &b.ChipType, &b.KeyVersion,
			&b.ExpectedCount, &b.ActivatedCount, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ── Registration ─────────────────────────────────────────────────────────────

// RegisterTagResult reports the outcome for one UID in a bulk registration.
// Per-UID rather than all-or-nothing because an operator importing 20,000 rows
// from an encoder export will have a handful of duplicates from a re-run, and
// failing the whole batch on those means they cannot make progress.
type RegisterTagResult struct {
	TagUID string `json:"tag_uid"`
	TagID  string `json:"tag_id,omitempty"`
	Status string `json:"status"` // registered | duplicate | error
	Error  string `json:"error,omitempty"`
}

// RegisterTags inserts a batch of provisioned tags. UIDs must already be
// normalised by the caller (services.NormaliseUID) — this function does not
// canonicalise, because a UID that reached here in the wrong form indicates a
// bug upstream that silently rewriting would hide.
func RegisterTags(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, batchID string,
	chip models.ChipType,
	class models.VerificationClass,
	keyVersion int,
	uids []string,
	actorID string,
) ([]RegisterTagResult, error) {
	results := make([]RegisterTagResult, 0, len(uids))

	for _, uid := range uids {
		var tagID string
		err := pool.QueryRow(ctx, `
			INSERT INTO rfid_tags
				(event_id, batch_id, tag_uid, chip_type, verification_class, key_version, status)
			VALUES ($1, $2, $3, $4::tag_chip_type, $5::tag_verification_class, $6, 'provisioned')
			ON CONFLICT (event_id, tag_uid) DO NOTHING
			RETURNING id
		`, eventID, batchID, uid, string(chip), string(class), keyVersion).Scan(&tagID)

		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// ON CONFLICT DO NOTHING returns no row — the UID is already here.
			results = append(results, RegisterTagResult{TagUID: uid, Status: "duplicate"})
		case err != nil:
			results = append(results, RegisterTagResult{TagUID: uid, Status: "error", Error: err.Error()})
		default:
			results = append(results, RegisterTagResult{TagUID: uid, TagID: tagID, Status: "registered"})
			AppendTagEvent(ctx, pool, tagID, eventID, models.TagEventProvisioned, &actorID, nil, nil)
		}
	}
	return results, nil
}

// ── Lookup ───────────────────────────────────────────────────────────────────

// TagWithBinding is a tag plus everything the tap path needs to decide
// admission, in one round trip. The joined columns are nullable because a tag
// may legitimately be unbound, and because ON DELETE SET NULL can leave a
// binding pointing at nothing after a booking is purged.
type TagWithBinding struct {
	models.RFIDTag

	AssignmentStatus *string `json:"assignment_status,omitempty"`
	AssignmentSlot   *int    `json:"assignment_slot,omitempty"`
	AssigneeName     *string `json:"assignee_name,omitempty"`
	BookingStatus    *string `json:"booking_status,omitempty"`
	QRScannedAt      *time.Time `json:"qr_scanned_at,omitempty"`
	TierName         *string `json:"tier_name,omitempty"`
}

// GetTagByUID resolves a tap. Scoped by event on purpose: the same physical
// chip may be registered to several events over its life, and the gate must
// only ever see the registration for the event it is working.
func GetTagByUID(ctx context.Context, pool *pgxpool.Pool, eventID, uid string) (*TagWithBinding, error) {
	t := &TagWithBinding{}
	err := pool.QueryRow(ctx, `
		SELECT
			rt.id, rt.event_id, rt.batch_id, rt.tag_uid,
			rt.chip_type::text, rt.verification_class::text, rt.key_version, rt.status::text,
			rt.assignment_id, rt.booking_id, rt.last_read_ctr,
			rt.bound_at, rt.activated_at, rt.deactivated_at, rt.created_at, rt.updated_at,
			ta.status::text, ta.slot_number, ta.assignee_name, ta.qr_scanned_at,
			b.booking_status::text,
			tt.name
		FROM rfid_tags rt
		LEFT JOIN ticket_assignments ta ON ta.id = rt.assignment_id
		LEFT JOIN bookings b            ON b.id  = COALESCE(ta.booking_id, rt.booking_id)
		LEFT JOIN ticket_types tt       ON tt.id = b.ticket_type_id
		WHERE rt.event_id = $1 AND rt.tag_uid = $2
	`, eventID, uid).Scan(
		&t.ID, &t.EventID, &t.BatchID, &t.TagUID,
		&t.ChipType, &t.VerificationClass, &t.KeyVersion, &t.Status,
		&t.AssignmentID, &t.BookingID, &t.LastReadCtr,
		&t.BoundAt, &t.ActivatedAt, &t.DeactivatedAt, &t.CreatedAt, &t.UpdatedAt,
		&t.AssignmentStatus, &t.AssignmentSlot, &t.AssigneeName, &t.QRScannedAt,
		&t.BookingStatus,
		&t.TierName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTagNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ListTags powers the organiser console and (from Phase 2) manifest sync.
// `since` non-zero returns only rows touched after it — the same delta-cursor
// shape BuildManifest already uses, so the Studio sync loop needs no new idea.
func ListTags(ctx context.Context, pool *pgxpool.Pool, eventID string, since time.Time, limit int) ([]models.RFIDTag, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := pool.Query(ctx, `
		SELECT id, event_id, batch_id, tag_uid,
		       chip_type::text, verification_class::text, key_version, status::text,
		       assignment_id, booking_id, last_read_ctr,
		       bound_at, activated_at, deactivated_at, created_at, updated_at
		FROM rfid_tags
		WHERE event_id = $1
		  AND ($2::timestamptz IS NULL OR updated_at > $2)
		ORDER BY updated_at
		LIMIT $3
	`, eventID, nullTime(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.RFIDTag{}
	for rows.Next() {
		var t models.RFIDTag
		if err := rows.Scan(&t.ID, &t.EventID, &t.BatchID, &t.TagUID,
			&t.ChipType, &t.VerificationClass, &t.KeyVersion, &t.Status,
			&t.AssignmentID, &t.BookingID, &t.LastReadCtr,
			&t.BoundAt, &t.ActivatedAt, &t.DeactivatedAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// EventKeyVersions returns the distinct key versions in use at an event,
// newest first.
//
// The SDM tap path needs this because of a genuine chicken-and-egg: an NTAG 424
// tap does not reveal its UID until the PICC block is decrypted, and the
// decryption key depends on the key version — which is recorded against a tag
// we cannot identify yet. The reader cannot supply it either, since the version
// is not written to the chip in any readable form.
//
// So the tap tries the versions the event actually has. In practice that is
// one; it is only ever more than one mid-rotation, and it is bounded by the
// number of encoding runs rather than by anything an attacker controls. A tag
// from another event still fails every candidate, because the event ID is
// baked into the derivation before the version is.
func EventKeyVersions(ctx context.Context, pool *pgxpool.Pool, eventID string) ([]int, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT key_version
		FROM rfid_tags
		WHERE event_id = $1
		UNION
		SELECT DISTINCT key_version
		FROM rfid_tag_batches
		WHERE event_id = $1
		ORDER BY key_version DESC
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []int{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		out = []int{1} // no tags registered yet — the default the encoder starts at
	}
	return out, nil
}

// ── Binding ──────────────────────────────────────────────────────────────────

// BindTagToAssignment binds a tag to one ticket and activates it in a single
// statement.
//
// The guard clause is doing real work. uq_rfid_active_assignment already stops
// two live tags sharing an assignment, but the status predicate here is what
// makes re-issuing a lost wristband safe: the operator must deactivate the old
// chip first, so there is never a window where both open the gate. A partial
// unique index cannot express that ordering on its own.
func BindTagToAssignment(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, tagUID, assignmentID, actorID string,
) error {
	var tagID string
	err := pool.QueryRow(ctx, `
		UPDATE rfid_tags
		SET assignment_id = $3,
		    booking_id    = NULL,
		    status        = 'active',
		    bound_at      = now(),
		    activated_at  = COALESCE(activated_at, now())
		WHERE event_id = $1
		  AND tag_uid  = $2
		  AND status IN ('provisioned', 'encoded')
		  AND assignment_id IS NULL
		  AND booking_id    IS NULL
		RETURNING id
	`, eventID, tagUID, assignmentID).Scan(&tagID)

	if errors.Is(err, pgx.ErrNoRows) {
		// Distinguish "no such tag" from "not bindable" so the operator at the
		// desk gets an instruction rather than a shrug.
		var status string
		if e := pool.QueryRow(ctx,
			`SELECT status::text FROM rfid_tags WHERE event_id = $1 AND tag_uid = $2`,
			eventID, tagUID).Scan(&status); errors.Is(e, pgx.ErrNoRows) {
			return ErrTagNotFound
		}
		return ErrTagAlreadyBound
	}
	if err != nil {
		return err
	}

	AppendTagEvent(ctx, pool, tagID, eventID, models.TagEventBound, &actorID, nil,
		map[string]any{"assignment_id": assignmentID})
	return nil
}

// BindTagToBooking binds a tag to a booking POOL — the group-pass equivalent,
// mirroring the pool token family on the QR path.
func BindTagToBooking(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, tagUID, bookingID, actorID string,
) error {
	var tagID string
	err := pool.QueryRow(ctx, `
		UPDATE rfid_tags
		SET booking_id    = $3,
		    assignment_id = NULL,
		    status        = 'active',
		    bound_at      = now(),
		    activated_at  = COALESCE(activated_at, now())
		WHERE event_id = $1
		  AND tag_uid  = $2
		  AND status IN ('provisioned', 'encoded')
		  AND assignment_id IS NULL
		  AND booking_id    IS NULL
		RETURNING id
	`, eventID, tagUID, bookingID).Scan(&tagID)

	if errors.Is(err, pgx.ErrNoRows) {
		var status string
		if e := pool.QueryRow(ctx,
			`SELECT status::text FROM rfid_tags WHERE event_id = $1 AND tag_uid = $2`,
			eventID, tagUID).Scan(&status); errors.Is(e, pgx.ErrNoRows) {
			return ErrTagNotFound
		}
		return ErrTagAlreadyBound
	}
	if err != nil {
		return err
	}

	AppendTagEvent(ctx, pool, tagID, eventID, models.TagEventBound, &actorID, nil,
		map[string]any{"booking_id": bookingID})
	return nil
}

// SetTagStatus moves a tag through its lifecycle — deactivation, loss reports,
// write-offs. Unbinding on the way out is deliberate: a lost tag that keeps its
// assignment_id would hold the uq_rfid_active_assignment slot and block the
// replacement wristband, which is the exact moment the operator needs one.
func SetTagStatus(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID, tagUID string,
	status models.TagStatus,
	kind models.TagEventKind,
	actorID string,
	reason string,
) error {
	var tagID string
	err := pool.QueryRow(ctx, `
		UPDATE rfid_tags
		SET status         = $3::tag_status,
		    assignment_id  = CASE WHEN $3 IN ('deactivated','lost','destroyed') THEN NULL ELSE assignment_id END,
		    booking_id     = CASE WHEN $3 IN ('deactivated','lost','destroyed') THEN NULL ELSE booking_id END,
		    deactivated_at = CASE WHEN $3 IN ('deactivated','lost','destroyed') THEN now() ELSE deactivated_at END
		WHERE event_id = $1 AND tag_uid = $2
		RETURNING id
	`, eventID, tagUID, string(status)).Scan(&tagID)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTagNotFound
	}
	if err != nil {
		return err
	}

	AppendTagEvent(ctx, pool, tagID, eventID, kind, &actorID, nil,
		map[string]any{"reason": reason, "status": string(status)})
	return nil
}

// ── Replay guard ─────────────────────────────────────────────────────────────

// BumpReadCounter advances a tag's SDM read counter high-water mark, and
// reports whether the counter was actually fresh.
//
// A false return is a REPLAY: the tap carried a counter we have already seen,
// which means the payload was captured and replayed rather than produced by the
// chip just now. The comparison and the write are one statement so concurrent
// taps at neighbouring gates cannot both win.
//
// Only meaningful for chips that emit a counter. Callers must not invoke it for
// DESFire or UID-only tags — the column carries a CHECK constraint restricting
// it to ntag424 and the write would fail.
func BumpReadCounter(ctx context.Context, pool *pgxpool.Pool, tagID string, ctr int64) (bool, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE rfid_tags
		SET last_read_ctr = $2
		WHERE id = $1
		  AND (last_read_ctr IS NULL OR last_read_ctr < $2)
	`, tagID, ctr)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ── Audit ────────────────────────────────────────────────────────────────────

// AppendTagEvent writes one lifecycle fact. Best-effort by design — it returns
// an error for logging, and no caller should refuse an operation because of it.
func AppendTagEvent(
	ctx context.Context,
	pool *pgxpool.Pool,
	tagID, eventID string,
	kind models.TagEventKind,
	actorID *string,
	deviceID *string,
	detail map[string]any,
) error {
	if detail == nil {
		detail = map[string]any{}
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO rfid_tag_events (tag_id, event_id, kind, actor_id, device_id, detail)
		VALUES ($1, $2, $3::tag_event_kind, $4, $5, $6)
	`, tagID, eventID, string(kind), actorID, deviceID, detail)
	return err
}

// ListTagEvents returns a tag's history, newest first — the record that answers
// a disputed admission (and, from Phase 3, a disputed charge).
func ListTagEvents(ctx context.Context, pool *pgxpool.Pool, tagID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := pool.Query(ctx, `
		SELECT id, kind::text, actor_id, device_id, detail, created_at
		FROM rfid_tag_events
		WHERE tag_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, tagID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var (
			id, kind  string
			actorID   *string
			deviceID  *string
			detail    map[string]any
			createdAt time.Time
		)
		if err := rows.Scan(&id, &kind, &actorID, &deviceID, &detail, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "kind": kind, "actor_id": actorID,
			"device_id": deviceID, "detail": detail, "created_at": createdAt,
		})
	}
	return out, rows.Err()
}

// RecordTagScan appends an NFC admission to the scan ledger.
//
// Phase 1 shipped this as a wrapper around RecordOnlineScan followed by an
// UPDATE that re-found the row by raw_ref. That was fragile — it matched the
// most recent row with the same raw_ref, which is ambiguous the moment one band
// is tapped twice in a second — and it put the tag UID in raw_ref, where every
// other scan path puts the assignment id. Both are fixed here: one INSERT,
// raw_ref carrying the assignment like the QR path, and the UID in its own
// column. The offline path (ingestOne) writes the same shape.
func RecordTagScan(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID string,
	assignmentID *string,
	bookingID *string,
	tagUID string,
	admitCount int,
	scannedByUserID string,
	verdict string,
) error {
	if admitCount < 1 {
		admitCount = 1
	}

	// raw_ref is the reference AS CLAIMED, and is VARCHAR(64). For a tap that
	// resolved to a ticket that is the assignment; for a refused tap that
	// resolved to nothing, the UID is all there is, and keeping it is the whole
	// point of ledgering refusals.
	rawRef := tagUID
	if assignmentID != nil {
		rawRef = *assignmentID
	} else if bookingID != nil {
		rawRef = *bookingID
	}
	if len(rawRef) > 64 {
		rawRef = rawRef[:64]
	}

	var uid *string
	if tagUID != "" {
		u := tagUID
		uid = &u
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO scan_events (
			id, event_id, assignment_id, booking_id, raw_ref,
			admit_count, scanned_by, device_id, mode, verdict, scanned_at,
			medium, tag_uid
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'online', $9::scan_verdict, now(), 'nfc', $10)
	`,
		uuid.NewString(), eventID, assignmentID, bookingID, rawRef,
		admitCount, scannedByUserID, OnlineScanDeviceID, verdict, uid,
	)
	return err
}