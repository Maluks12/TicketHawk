package repository

import "strings"

// ── Offline tag reconciliation ────────────────────────────────────────────────
//
// Under the manifest-authoritative trust model (the same one ScannerCredentialService
// states for QR: no verification keys ever leave the server), an offline handset
// cannot check a wristband's cryptogram. It can only check what its last synced
// manifest says: this UID maps to that assignment, and that assignment has not
// been admitted yet.
//
// A manifest is a snapshot, so the device's mapping can be WRONG by the time it
// syncs — and the two ways it goes wrong are the ones that matter most:
//
//   - The tag was deactivated, reported lost, or written off after the device
//     last synced. The handset still has it as active and lets the holder in.
//     This is precisely the "attendee reported their band stolen at gate 3,
//     three other gates are offline" case, and it is the reason tag status is
//     re-checked here rather than trusted from the device.
//
//   - The tag was RE-BOUND after the device last synced. Re-issuing a lost
//     wristband deactivates the old chip and binds a new one; a stale handset
//     may still map the old UID to that assignment. Admitting on that mapping
//     would let the lost band work after it was replaced.
//
// Migration 000051 already states the governing principle — verdict is
// "SERVER-decided, not the device's claim" — and ingestOne already applies it
// to booking status. This is the same idea extended to the tag, and it is the
// control that makes manifest-only offline NFC acceptable for ticketing: a
// cloned or revoked band may get someone through a gate during an outage, but
// it cannot survive reconciliation, and it lands in the exceptions report the
// organiser actually reads.
//
// Kept pure — no database, no models import, no clock — so the whole decision
// table is unit-testable without infrastructure, which is what the Phase 2 test
// gate rests on. scan_repository.go deliberately avoids depending on models for
// the same reason, so the status strings here are local literals fixed by the
// tag_status enum in migration 000085.

// Tag statuses, mirroring the tag_status enum. Local literals rather than a
// models import, matching bookingStatusConfirmed in scan_repository.go.
const (
	tagStatusActive      = "active"
	tagStatusProvisioned = "provisioned"
	tagStatusEncoded     = "encoded"
)

// Reconciliation verdicts for a tag-backed scan. An empty verdict means "no
// tag-specific objection — carry on with the normal admission logic".
const (
	TagScanOK          = ""
	TagScanRejected    = "rejected"
	TagScanProvisional = "provisional"
)

// Reasons, surfaced in the exceptions report so an organiser sees WHY rather
// than a bare verdict. Stable strings — the organiser dashboard may map them.
const (
	TagReasonUnknownTag     = "tag_not_registered"
	TagReasonNotActive      = "tag_not_active"
	TagReasonRebound        = "tag_rebound_since_sync"
	TagReasonUnboundTag     = "tag_not_bound"
	TagReasonPoolUnsupported = "tag_pool_binding_unsupported"
)

// TagScanState is the SERVER's view of a tag at reconciliation time.
type TagScanState struct {
	Found             bool
	Status            string
	BoundAssignmentID *string
	BoundBookingID    *string
}

// TagScanClaim is what the DEVICE asserted when it admitted offline.
type TagScanClaim struct {
	TagUID       string
	AssignmentID *string
}

// ClassifyTagScan decides whether a tag-backed scan may proceed to the normal
// admission path, and if not, what to record instead.
//
// Returns (TagScanOK, "") to proceed. Otherwise a terminal verdict and a reason.
//
// The provisional/rejected split follows the convention ingestOne already uses:
// 'provisional' means "we cannot tell yet, a human should look" and does not
// admit; 'rejected' means "we can tell, and the answer is no". An unregistered
// UID is provisional rather than rejected because late encoding is a normal
// operational event — a wristband registered after this handset last synced is
// indistinguishable here from a forged one, and calling a legitimate late
// registration fraud would be worse than flagging it.
func ClassifyTagScan(state TagScanState, claim TagScanClaim) (verdict, reason string) {
	if strings.TrimSpace(claim.TagUID) == "" {
		// Not a tag-backed scan at all — nothing for this function to say.
		return TagScanOK, ""
	}

	if !state.Found {
		return TagScanProvisional, TagReasonUnknownTag
	}

	// Status first: a lost or deactivated band is refused regardless of what it
	// is bound to, and this is the case that must never fall through.
	switch state.Status {
	case tagStatusActive:
		// carry on
	case tagStatusProvisioned, tagStatusEncoded:
		// Registered but never handed out, so it should not have admitted
		// anyone. Distinguished from deactivated only in the reason string.
		return TagScanRejected, TagReasonNotActive
	default:
		// deactivated, lost, destroyed, or any value added to the enum later.
		// Defaulting to rejection means a new status is safe-by-default: a
		// future 'suspended' refuses entry until somebody teaches this function
		// otherwise, rather than silently admitting.
		return TagScanRejected, TagReasonNotActive
	}

	// Pool-bound tags are not admitted offline, matching TapTagHandler's online
	// behaviour. One physical band admitting several people is a product we
	// have not designed, and quietly admitting one head here would make the
	// two paths disagree.
	if state.BoundBookingID != nil && state.BoundAssignmentID == nil {
		return TagScanRejected, TagReasonPoolUnsupported
	}

	if state.BoundAssignmentID == nil {
		// Active but unbound should be unreachable — chk_rfid_active_is_bound
		// forbids it — so reaching here means the constraint was dropped or
		// bypassed. Refuse rather than assume.
		return TagScanRejected, TagReasonUnboundTag
	}

	// The device must have admitted against the binding the server holds.
	if claim.AssignmentID == nil || *claim.AssignmentID != *state.BoundAssignmentID {
		return TagScanRejected, TagReasonRebound
	}

	return TagScanOK, ""
}