package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// OFFLINE TAG RECONCILIATION TESTS — Phase 2a gate
// =============================================================================
//
// Unit only, no database, no hardware:
//
//	go test ./internal/repository/... -v -run TestClassifyTagScan
//
// This is the whole safety argument for manifest-only offline NFC. The device
// cannot verify a cryptogram offline, so everything rests on the server
// refusing, at sync, the mappings the device could not know had changed. Each
// case below is one of those.

func strptr(s string) *string { return &s }

const (
	assignA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	assignB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	bookC   = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	uid     = "04A1B2C3D4E580"
)

func TestClassifyTagScanAdmits(t *testing.T) {
	// The happy path: active tag, and the device admitted against the same
	// assignment the server has bound. Everything else in ingestOne then
	// applies as normal — including the already-scanned collision guard, which
	// this function deliberately does NOT duplicate.
	v, reason := ClassifyTagScan(
		TagScanState{Found: true, Status: "active", BoundAssignmentID: strptr(assignA)},
		TagScanClaim{TagUID: uid, AssignmentID: strptr(assignA)},
	)
	assert.Equal(t, TagScanOK, v)
	assert.Empty(t, reason)
}

func TestClassifyTagScanIgnoresNonTagScans(t *testing.T) {
	// A QR scan carries no UID and must pass straight through untouched —
	// otherwise adding NFC would change the verdict of every existing scan.
	for _, uid := range []string{"", "   "} {
		v, reason := ClassifyTagScan(
			TagScanState{Found: false},
			TagScanClaim{TagUID: uid, AssignmentID: strptr(assignA)},
		)
		assert.Equal(t, TagScanOK, v)
		assert.Empty(t, reason)
	}
}

func TestClassifyTagScanRevokedBands(t *testing.T) {
	// The case the whole control exists for: a band deactivated, lost, or
	// destroyed AFTER the handset's last sync. The device let them in; the
	// server must not agree.
	for _, status := range []string{"deactivated", "lost", "destroyed"} {
		t.Run(status, func(t *testing.T) {
			v, reason := ClassifyTagScan(
				TagScanState{Found: true, Status: status, BoundAssignmentID: strptr(assignA)},
				TagScanClaim{TagUID: uid, AssignmentID: strptr(assignA)},
			)
			assert.Equal(t, TagScanRejected, v)
			assert.Equal(t, TagReasonNotActive, reason)
		})
	}
}

func TestClassifyTagScanNeverIssuedBands(t *testing.T) {
	// Registered but never handed to anybody. A tap on one of these means a
	// band left the encoding table without being bound — worth refusing and
	// worth a distinct reason so it is not read as a revocation.
	for _, status := range []string{"provisioned", "encoded"} {
		v, reason := ClassifyTagScan(
			TagScanState{Found: true, Status: status},
			TagScanClaim{TagUID: uid, AssignmentID: strptr(assignA)},
		)
		assert.Equal(t, TagScanRejected, v)
		assert.Equal(t, TagReasonNotActive, reason)
	}
}

func TestClassifyTagScanUnknownStatusFailsClosed(t *testing.T) {
	// A value added to the tag_status enum later must refuse entry until
	// somebody updates this function, rather than falling through to admit.
	// If this test ever fails, a new status was added without a decision.
	v, reason := ClassifyTagScan(
		TagScanState{Found: true, Status: "suspended_for_review", BoundAssignmentID: strptr(assignA)},
		TagScanClaim{TagUID: uid, AssignmentID: strptr(assignA)},
	)
	assert.Equal(t, TagScanRejected, v)
	assert.Equal(t, TagReasonNotActive, reason)
}

func TestClassifyTagScanRebound(t *testing.T) {
	// Re-issue: the attendee lost band A, staff deactivated it and bound band B
	// to the same ticket. A handset still holding the old manifest maps band A
	// to that assignment. Server says the binding moved, so the lost band's
	// admission does not stand.
	v, reason := ClassifyTagScan(
		TagScanState{Found: true, Status: "active", BoundAssignmentID: strptr(assignB)},
		TagScanClaim{TagUID: uid, AssignmentID: strptr(assignA)},
	)
	assert.Equal(t, TagScanRejected, v)
	assert.Equal(t, TagReasonRebound, reason)
}

func TestClassifyTagScanMissingClaim(t *testing.T) {
	// A device that sends a UID but no assignment has not resolved the band
	// against its manifest at all. There is nothing to agree with.
	v, reason := ClassifyTagScan(
		TagScanState{Found: true, Status: "active", BoundAssignmentID: strptr(assignA)},
		TagScanClaim{TagUID: uid},
	)
	assert.Equal(t, TagScanRejected, v)
	assert.Equal(t, TagReasonRebound, reason)
}

func TestClassifyTagScanUnknownTagIsProvisional(t *testing.T) {
	// Deliberately NOT rejected. A band encoded after this handset last synced
	// is indistinguishable here from a forged one, and treating a legitimate
	// late registration as fraud is the worse error. Provisional admits
	// nothing and puts it in front of a human.
	v, reason := ClassifyTagScan(
		TagScanState{Found: false},
		TagScanClaim{TagUID: uid, AssignmentID: strptr(assignA)},
	)
	assert.Equal(t, TagScanProvisional, v)
	assert.Equal(t, TagReasonUnknownTag, reason)
}

func TestClassifyTagScanPoolBoundIsRefused(t *testing.T) {
	// Pool-bound bands are refused offline for the same reason TapTagHandler
	// refuses them online. The two paths must not disagree.
	v, reason := ClassifyTagScan(
		TagScanState{Found: true, Status: "active", BoundBookingID: strptr(bookC)},
		TagScanClaim{TagUID: uid, AssignmentID: strptr(assignA)},
	)
	assert.Equal(t, TagScanRejected, v)
	assert.Equal(t, TagReasonPoolUnsupported, reason)
}

func TestClassifyTagScanActiveButUnbound(t *testing.T) {
	// Unreachable while chk_rfid_active_is_bound holds. Tested anyway, because
	// "unreachable" states are exactly what a dropped constraint produces, and
	// the safe answer is refusal rather than an assumption.
	v, reason := ClassifyTagScan(
		TagScanState{Found: true, Status: "active"},
		TagScanClaim{TagUID: uid, AssignmentID: strptr(assignA)},
	)
	assert.Equal(t, TagScanRejected, v)
	assert.Equal(t, TagReasonUnboundTag, reason)
}