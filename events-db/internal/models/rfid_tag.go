package models

import "time"

// ── RFID/NFC tags ─────────────────────────────────────────────────────────────
//
// Mirrors migration 000085. The enums here are the Go side of the Postgres
// enums and must stay in step with them — a value added on one side and not the
// other fails at the driver, not at compile time.

// ChipType is the physical chip family. Kept separate from VerificationClass
// because procurement thinks in chips and the security model thinks in trust.
type ChipType string

const (
	ChipNTAG424    ChipType = "ntag424"
	ChipDESFireEV2 ChipType = "desfire_ev2"
	ChipDESFireEV3 ChipType = "desfire_ev3"
	ChipUIDOnly    ChipType = "uid_only"
)

// VerificationClass says WHO can vouch for a tap.
//
//	VerifyServer — the backend verifies the tap itself, from the chip's own
//	               cryptogram. The reader is a relay and does not need trusting.
//	VerifyReader — the reader authenticated the chip and is reporting the
//	               result. Only as trustworthy as the reader, which is why
//	               device attestation (Phase 4) exists.
//	VerifyNone   — the tap carries nothing but a UID, which anything can claim.
//
// Phase 3 gates wallet binding on this. Do not add a class without deciding
// what it may spend.
type VerificationClass string

const (
	VerifyServer VerificationClass = "server_verified"
	VerifyReader VerificationClass = "reader_verified"
	VerifyNone   VerificationClass = "unverified"
)

// VerificationClassFor returns the class a chip type is allowed to hold. The
// same pairing is enforced by chk_rfid_class_matches_chip in the database; this
// is the single place the application derives it so the two cannot drift.
func VerificationClassFor(chip ChipType) (VerificationClass, bool) {
	switch chip {
	case ChipNTAG424:
		return VerifyServer, true
	case ChipDESFireEV2, ChipDESFireEV3:
		return VerifyReader, true
	case ChipUIDOnly:
		return VerifyNone, true
	default:
		return "", false
	}
}

// TagStatus is the tag lifecycle.
type TagStatus string

const (
	TagProvisioned TagStatus = "provisioned"
	TagEncoded     TagStatus = "encoded"
	TagActive      TagStatus = "active"
	TagDeactivated TagStatus = "deactivated"
	TagLost        TagStatus = "lost"
	TagDestroyed   TagStatus = "destroyed"
)

// Admissible reports whether a tag in this status may be considered at a gate
// at all. Everything else is refused before any crypto runs — a lost tag should
// cost one indexed lookup to reject, not a CMAC.
func (s TagStatus) Admissible() bool {
	return s == TagActive
}

// TagEventKind is the lifecycle audit vocabulary (rfid_tag_events.kind).
type TagEventKind string

const (
	TagEventProvisioned  TagEventKind = "provisioned"
	TagEventEncoded      TagEventKind = "encoded"
	TagEventBound        TagEventKind = "bound"
	TagEventUnbound      TagEventKind = "unbound"
	TagEventActivated    TagEventKind = "activated"
	TagEventDeactivated  TagEventKind = "deactivated"
	TagEventReportedLost TagEventKind = "reported_lost"
	TagEventKeyRotated   TagEventKind = "key_rotated"
	TagEventTapRefused   TagEventKind = "tap_refused"
)

// ScanMedium is how a credential was presented. Orthogonal to scan mode, which
// is transport (online/offline) — see the note in migration 000085.
type ScanMedium string

const (
	MediumQR  ScanMedium = "qr"
	MediumNFC ScanMedium = "nfc"
)

// RFIDTag is one physical chip as the platform knows it.
type RFIDTag struct {
	ID                string            `json:"id"`
	EventID           string            `json:"event_id"`
	BatchID           *string           `json:"batch_id,omitempty"`
	TagUID            string            `json:"tag_uid"`
	ChipType          ChipType          `json:"chip_type"`
	VerificationClass VerificationClass `json:"verification_class"`
	KeyVersion        int               `json:"key_version"`
	Status            TagStatus         `json:"status"`
	AssignmentID      *string           `json:"assignment_id,omitempty"`
	BookingID         *string           `json:"booking_id,omitempty"`
	LastReadCtr       *int64            `json:"last_read_ctr,omitempty"`
	BoundAt           *time.Time        `json:"bound_at,omitempty"`
	ActivatedAt       *time.Time        `json:"activated_at,omitempty"`
	DeactivatedAt     *time.Time        `json:"deactivated_at,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// RFIDTagBatch is an encoding run — the unit an operator handles and the unit
// the per-tag software fee is invoiced against.
type RFIDTagBatch struct {
	ID             string    `json:"id"`
	EventID        string    `json:"event_id"`
	Label          string    `json:"label"`
	ChipType       ChipType  `json:"chip_type"`
	KeyVersion     int       `json:"key_version"`
	ExpectedCount  int       `json:"expected_count"`
	ActivatedCount int       `json:"activated_count"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}