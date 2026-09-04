package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Online scan ledger ────────────────────────────────────────────────────────
//
// The offline path (IngestScans) writes every admission into scan_events. The
// online handlers did not — they set ticket_assignments.qr_scanned_at and
// nothing else. Three things read the ledger rather than that column, and all
// three were therefore blind to any ticket scanned over the live API:
//
//   - analytics_repository: the door figure is SUM(admit_count) over
//     verdict = 'admitted'. Online scans counted as zero, so attendance and
//     no-show rates under-reported by however much of the gate ran online.
//
//   - payout_repository.CheckInstantEligibility: a "delivered paid event"
//     requires EXISTS (SELECT 1 FROM scan_events WHERE event_id = ev.id). An
//     organiser whose gates ran online had no rows at all, so the count never
//     incremented and instant-tier promotion was unreachable no matter how many
//     events they completed successfully.
//
//   - ListScanExceptions: the multi-gate report never saw online admissions, so
//     a collision between an online gate and an offline one only ever showed
//     one side of itself.
//
// RecordOnlineScan closes the gap. It is deliberately best-effort at every call
// site: the ticket_assignments UPDATE is what admits the attendee, and a failed
// audit row must never become a refusal at the door. Callers log the error
// rather than returning it.

// OnlineScanDeviceID fills scan_events.device_id (NOT NULL) for scans that
// arrived over the API rather than from a Studio handset. It is displayed
// verbatim in the exceptions report, so it reads as a source, not a device.
const OnlineScanDeviceID = "online-api"

// Verdicts the online path writes. 'admitted' is what the analytics sum counts;
// 'duplicate' is audit only — it is excluded from that sum AND from
// ListScanExceptions, which filters on ('collision', 'provisional').
const (
	ScanVerdictAdmitted  = "admitted"
	ScanVerdictDuplicate = "duplicate"
)

// RecordOnlineScan appends one row to the scan ledger for an admission applied
// through an online handler.
//
// assignmentID is nil for pool admits — a pool scan admits N slots rather than
// one named assignment, which is the same shape ingestOne writes offline.
// admitCount is the number of heads ACTUALLY admitted, not the number claimed;
// scan_events carries CHECK (admit_count > 0), so it is clamped to 1.
func RecordOnlineScan(
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID string,
	assignmentID *string,
	bookingID *string,
	rawRef string,
	admitCount int,
	scannedByUserID string,
	verdict string,
) error {
	if admitCount < 1 {
		admitCount = 1
	}
	if len(rawRef) > 64 { // raw_ref is VARCHAR(64)
		rawRef = rawRef[:64]
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO scan_events (
			id, event_id, assignment_id, booking_id, raw_ref,
			admit_count, scanned_by, device_id, mode, verdict, scanned_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'online', $9::scan_verdict, now())
	`,
		uuid.NewString(), eventID, assignmentID, bookingID, rawRef,
		admitCount, scannedByUserID, OnlineScanDeviceID, verdict,
	)
	return err
}