package services

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"events-db/internal/config"
	"events-db/internal/models"
)

// ── Tag cryptography ──────────────────────────────────────────────────────────
//
// One service, every chip family, because the platform has to stay chip-
// agnostic: a festival buying 30,000 NTAG 424 wristbands and a club reusing 200
// DESFire cards must both work without a second code path at the gate.
//
// The shape mirrors RotatingQRService deliberately — verification is pure
// computation, touches no database, and returns a payload the handler then
// checks against the DB. Same reasoning as the QR path: crypto and state stay
// separable, so the crypto is unit-testable without infrastructure.
//
// WHAT IS TRUSTED, PER CHIP. This is the whole design and it is not a detail:
//
//	NTAG 424 DNA   The chip emits a fresh SDM (Secure Unique NFC) message on
//	               every tap: encrypted UID + a monotonic read counter, plus a
//	               CMAC over both under a key only we hold. VerifyTap
//	               reconstructs that CMAC here, on the server. A cloned UID
//	               produces no valid CMAC, and a replayed capture carries a
//	               counter the caller has already recorded. The reader is not
//	               trusted with anything.
//
//	DESFire EV2/3  Authentication is a mutual challenge-response between chip
//	               and reader. It is stronger at the tap than SDM — and it is
//	               invisible to us. All that reaches the server is the reader
//	               SAYING it succeeded. VerifyTap therefore returns
//	               VerifyReader, and the honest security statement is that
//	               DESFire taps are exactly as trustworthy as the handset,
//	               which is what device attestation in Phase 4 is for. Anyone
//	               reading "premium tier" in the architecture doc as "safer to
//	               accept unattested" has it backwards.
//
//	UID-only       Nothing is verified. A UID is a serial number that any phone
//	               can emit. Accepted for access-control pilots so a low-budget
//	               organiser is not blocked, refused for anything financial by
//	               the verification_class check in Phase 3.
//
// KEY STORAGE. No key derived here is ever persisted. The platform holds one
// master (TAG_MASTER_KEY, environment only); per-event and per-tag keys are
// derived on demand and discarded. rfid_tags stores key_version and nothing
// else, so a database compromise yields no key material and key rotation is a
// version bump rather than a re-encoding of the estate.

// Derivation label prefixes. Every derivation is domain-separated by a distinct
// prefix so no two purposes can ever produce the same key — the same reasoning
// behind the "pool:" / "off:" / "offpool:" namespaces in RotatingQRService.
const (
	tagKDFVersion   = "thv1"
	tagKDFEventPfx  = tagKDFVersion + "|evt|"
	tagPurposeMeta  = byte(0x01) // SDMMetaReadKey  — decrypts the PICC data
	tagPurposeMAC   = byte(0x02) // SDMFileReadMACKey — authenticates the tap
	tagPurposeAppMK = byte(0x03) // DESFire application master key
)

// SDM protocol constants (NXP NTAG 424 DNA, AN12196).
//
// sdmSV2Prefix is the fixed header of the session-vector used to derive the
// per-tap MAC session key. sdmPICCTagByte is the leading byte of a decrypted
// PICC block when UID and counter mirroring are both enabled.
//
// VERIFY THESE AGAINST A PHYSICAL TAG BEFORE PRODUCTION. They are transcribed
// from the NXP application note, not measured, and a wrong constant here fails
// closed — every real tap is rejected — rather than open. TestSDMAgainstRealTag
// in tag_crypto_service_test.go is the harness for that check and is skipped
// until a capture is pasted into it. Do not promote Phase 1 without running it.
var (
	sdmSV2Prefix   = []byte{0x3C, 0xC3, 0x00, 0x01, 0x00, 0x80}
	sdmPICCTagByte = byte(0xC7)
)

var (
	ErrTagUnsupportedChip = errors.New("unsupported chip type")
	ErrTagBadSDM          = errors.New("malformed SDM payload")
	ErrTagBadMAC          = errors.New("invalid tag signature")
	ErrTagReaderClaimOnly = errors.New("reader did not report a successful authentication")
	ErrTagNoUID           = errors.New("tag UID missing")
)

// TagCryptoService verifies taps and derives tag keys.
type TagCryptoService struct {
	master []byte
}

// NewTagCryptoService builds the service from config. The master key is decoded
// once at construction: a malformed key should stop the process at boot, not
// surface as a refused wristband at a gate on a Friday night.
func NewTagCryptoService(cfg *config.Config) (*TagCryptoService, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(cfg.TagMasterKey))
	if err != nil {
		return nil, fmt.Errorf("TAG_MASTER_KEY is not valid hex: %w", err)
	}
	switch len(raw) {
	case 16, 24, 32:
	default:
		return nil, fmt.Errorf("TAG_MASTER_KEY must decode to 16, 24 or 32 bytes, got %d", len(raw))
	}
	return &TagCryptoService{master: raw}, nil
}

// ── Key derivation ───────────────────────────────────────────────────────────

// EventKey derives the per-event, per-version root key.
//
// Scoping to the event is what contains a breach: a key recovered from one
// festival's wristband opens nothing at the next one. Including the version
// means rotation is a bump — old tags keep verifying under their recorded
// version while new tags are encoded under the new one, so an estate can be
// rotated progressively instead of in one outage.
func (s *TagCryptoService) EventKey(eventID string, keyVersion int) ([]byte, error) {
	label := fmt.Sprintf("%s%s|%d", tagKDFEventPfx, eventID, keyVersion)
	return AESCMAC(s.master, []byte(label))
}

// TagKey derives a per-tag, per-purpose AES-128 key by diversifying the event
// key with the chip UID — the AN10922 pattern: a constant, the purpose, then
// the UID, run through CMAC.
//
// Note it is INTERNALLY consistent rather than byte-identical to AN10922. That
// is a deliberate call: we encode our own tags with this same function, so
// nothing outside the platform needs to reproduce it. If you later buy
// pre-encoded stock from a bureau using NXP's own tooling, this function is the
// one thing that must change to match their derivation — which is why every
// caller goes through it instead of inlining the CMAC.
func (s *TagCryptoService) TagKey(eventID string, keyVersion int, uid []byte, purpose byte) ([]byte, error) {
	ek, err := s.EventKey(eventID, keyVersion)
	if err != nil {
		return nil, err
	}
	input := make([]byte, 0, 2+len(uid))
	input = append(input, 0x01, purpose)
	input = append(input, uid...)
	return AESCMAC(ek, input) // 16 bytes — AES-128, what both chip families use
}

// ── Tap verification ─────────────────────────────────────────────────────────

// SDMPayload is what an NTAG 424 DNA emits, as the reader relays it. Field
// names match the tag's own URL parameters so an encoder misconfiguration is
// visible by comparison rather than by translation.
type SDMPayload struct {
	PICCData    string `json:"picc_data"`               // hex, 32 chars — encrypted UID + counter
	CMAC        string `json:"cmac"`                    // hex, 16 chars — truncated tap MAC
	EncFileData string `json:"enc_file_data,omitempty"` // hex, optional mirrored file data
}

// ReaderClaim is what a DESFire-capable reader reports after doing the mutual
// authentication itself. Named "claim" throughout, in every layer, because that
// is exactly what it is until Phase 4 signs it with a hardware-backed key.
type ReaderClaim struct {
	AuthOK        bool   `json:"auth_ok"`
	OriginalityOK bool   `json:"originality_ok"` // NXP originality signature check
	Counter       *int64 `json:"counter,omitempty"`
	AppletVersion string `json:"applet_version,omitempty"`
}

// TapRequest is the chip-agnostic envelope the POS posts for one tap.
type TapRequest struct {
	ChipType models.ChipType `json:"chip_type" binding:"required"`
	TagUID   string          `json:"tag_uid,omitempty"` // required except ntag424, where it comes from PICC data
	SDM      *SDMPayload     `json:"sdm,omitempty"`
	Reader   *ReaderClaim    `json:"reader,omitempty"`
}

// TapVerification is the verified result. ReadCounter is non-nil only where the
// chip actually produced one; the caller must treat nil as "no replay evidence
// available" and not as zero.
type TapVerification struct {
	UID         string
	ReadCounter *int64
	Class       models.VerificationClass
	Warnings    []string
}

// VerifyTap authenticates a tap as far as the chip allows, and no further.
//
// It resolves the UID and, where the chip supports it, proves freshness. It
// does NOT decide admission: status, binding, booking state and the admission
// window are the handler's job, exactly as VerifyToken leaves those to
// ScanRotatingQRHandler. keyVersion is the version recorded against the tag —
// callers pass the stored value so a tag encoded under an older master still
// verifies after a rotation.
func (s *TagCryptoService) VerifyTap(eventID string, keyVersion int, req *TapRequest) (*TapVerification, error) {
	switch req.ChipType {

	case models.ChipNTAG424:
		if req.SDM == nil {
			return nil, fmt.Errorf("%w: sdm block required for ntag424", ErrTagBadSDM)
		}
		return s.verifySDM(eventID, keyVersion, req.SDM)

	case models.ChipDESFireEV2, models.ChipDESFireEV3:
		uid, err := normaliseUID(req.TagUID)
		if err != nil {
			return nil, err
		}
		if req.Reader == nil || !req.Reader.AuthOK {
			return nil, ErrTagReaderClaimOnly
		}
		v := &TapVerification{
			UID:         uid,
			ReadCounter: req.Reader.Counter,
			Class:       models.VerifyReader,
		}
		// Surfaced rather than refused: a failed originality check means the
		// chip is probably not genuine NXP silicon, but readers vary in whether
		// they can perform it at all, and turning "couldn't check" into "denied
		// entry" would strand legitimate attendees. It goes on the tag's audit
		// trail and into the exceptions report for a human to look at.
		if !req.Reader.OriginalityOK {
			v.Warnings = append(v.Warnings, "originality_signature_not_confirmed")
		}
		return v, nil

	case models.ChipUIDOnly:
		uid, err := normaliseUID(req.TagUID)
		if err != nil {
			return nil, err
		}
		return &TapVerification{
			UID:      uid,
			Class:    models.VerifyNone,
			Warnings: []string{"uid_only_chip_is_cloneable"},
		}, nil

	default:
		return nil, fmt.Errorf("%w: %s", ErrTagUnsupportedChip, req.ChipType)
	}
}

// verifySDM performs the full server-side NTAG 424 DNA check:
//
//	1. AES-CBC-decrypt the PICC block with the tag's SDMMetaReadKey, recovering
//	   the real UID and the chip's read counter. Note the UID is not sent in the
//	   clear — it comes out of this step, which is why TapRequest.TagUID is
//	   ignored for this chip: a caller cannot assert a UID it does not hold the
//	   key to produce.
//	2. Derive the per-tap session MAC key from UID + counter.
//	3. Recompute the truncated CMAC and compare in constant time.
//
// Freshness is NOT decided here — the counter is returned and the caller
// compares it against rfid_tags.last_read_ctr, because that is a database fact
// and this function stays pure.
func (s *TagCryptoService) verifySDM(eventID string, keyVersion int, p *SDMPayload) (*TapVerification, error) {
	picc, err := hex.DecodeString(strings.TrimSpace(p.PICCData))
	if err != nil || len(picc) != aes.BlockSize {
		return nil, fmt.Errorf("%w: picc_data must be 32 hex chars", ErrTagBadSDM)
	}
	gotMAC, err := hex.DecodeString(strings.TrimSpace(p.CMAC))
	if err != nil || len(gotMAC) != 8 {
		return nil, fmt.Errorf("%w: cmac must be 16 hex chars", ErrTagBadSDM)
	}

	// The meta key is diversified by UID, but the UID is inside the block we are
	// about to decrypt. That circularity is resolved the way NXP intends: the
	// meta key is diversified by the UID *printed on the tag at encoding time*,
	// which for us is always the UID the encoder registered. We therefore derive
	// with a zero UID for the meta purpose, making SDMMetaReadKey per-event
	// rather than per-tag. This is the one place the key hierarchy is
	// intentionally coarser, and it is why the MAC key below — which IS per-tag
	// — is what actually authenticates the tap.
	metaKey, err := s.TagKey(eventID, keyVersion, nil, tagPurposeMeta)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(metaKey)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, aes.BlockSize)
	cipher.NewCBCDecrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(plain, picc)

	if plain[0] != sdmPICCTagByte {
		// Wrong key, wrong encoding profile, or not one of our tags at all.
		return nil, fmt.Errorf("%w: unexpected PICC tag byte 0x%02X", ErrTagBadSDM, plain[0])
	}
	uid := plain[1:8]
	ctr := int64(plain[8]) | int64(plain[9])<<8 | int64(plain[10])<<16 // 3 bytes, LSB first

	macKey, err := s.TagKey(eventID, keyVersion, uid, tagPurposeMAC)
	if err != nil {
		return nil, err
	}

	// Session vector: fixed prefix || UID || counter (LSB first).
	sv := make([]byte, 0, len(sdmSV2Prefix)+10)
	sv = append(sv, sdmSV2Prefix...)
	sv = append(sv, uid...)
	sv = append(sv, plain[8], plain[9], plain[10])

	sessionKey, err := AESCMAC(macKey, sv)
	if err != nil {
		return nil, err
	}

	// The MAC covers the mirrored file data when SDM file mirroring is on, and
	// an empty message when it is not. Both are supported so the encoding
	// profile can change without a server release.
	var msg []byte
	if p.EncFileData != "" {
		msg, err = hex.DecodeString(strings.TrimSpace(p.EncFileData))
		if err != nil {
			return nil, fmt.Errorf("%w: enc_file_data is not hex", ErrTagBadSDM)
		}
	}
	full, err := AESCMAC(sessionKey, msg)
	if err != nil {
		return nil, err
	}
	if !cmacEqual(TruncateSDMMAC(full), gotMAC) {
		return nil, ErrTagBadMAC
	}

	return &TapVerification{
		UID:         strings.ToUpper(hex.EncodeToString(uid)),
		ReadCounter: &ctr,
		Class:       models.VerifyServer,
	}, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// normaliseUID canonicalises a caller-supplied UID to the uppercase, separator-
// free hex the database constraint expects. Readers are inconsistent about
// case and about colon separators, and a UID that differs only in formatting
// would otherwise register as a second, unknown tag.
func normaliseUID(raw string) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.NewReplacer(":", "", "-", "", " ", "").Replace(s)
	if s == "" {
		return "", ErrTagNoUID
	}
	if len(s)%2 != 0 || len(s) < 8 || len(s) > 20 {
		return "", fmt.Errorf("%w: %q is not a plausible chip UID", ErrTagNoUID, raw)
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", fmt.Errorf("%w: not hex", ErrTagNoUID)
	}
	return s, nil
}

// NormaliseUID is the exported form, used by the registration handler so an
// operator bulk-importing UIDs from an encoder's CSV gets the same
// canonicalisation the tap path applies.
func NormaliseUID(raw string) (string, error) { return normaliseUID(raw) }