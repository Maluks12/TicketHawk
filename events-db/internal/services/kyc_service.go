package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// KYCService owns organizer identity verification:
//
//	Submit  — validates the SA ID (checksum, DOB, 18+), sniffs and size-caps
//	          the uploaded documents, AES-GCM encrypts the ID number, and
//	          persists the submission as pending_review. If the Didit verifier
//	          is configured, a DHA registry check runs inline: a full match
//	          auto-verifies; anything else stays in the manual queue with the
//	          registry result attached for the reviewer.
//	Decide  — admin approve/reject.
//
// The encryption key comes from KYC_ENCRYPTION_KEY, falling back to the
// existing QR_ENCRYPTION_KEY so no new env is strictly required (a dedicated
// key is still recommended — see the wiring doc).
type KYCService struct {
	pool     *pgxpool.Pool
	verifier IDVerifier
	key      []byte
}

func NewKYCService(pool *pgxpool.Pool, verifier IDVerifier, encryptionKey string) (*KYCService, error) {
	k := []byte(encryptionKey)
	switch len(k) {
	case 16, 24, 32:
	default:
		return nil, fmt.Errorf("KYC encryption key must be 16, 24, or 32 bytes (got %d)", len(k))
	}
	return &KYCService{pool: pool, verifier: verifier, key: k}, nil
}

func (s *KYCService) VerifierName() string { return s.verifier.Name() }

// ── Crypto ───────────────────────────────────────────────────────────────────

func (s *KYCService) encrypt(plaintext string) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt recovers the SA ID number for the admin review endpoint.
func (s *KYCService) Decrypt(ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, sealed := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// ── Document sniffing ────────────────────────────────────────────────────────

// SniffKYCDocument validates magic bytes and returns the real MIME type.
// ID photos: JPEG/PNG/WebP. Proof of address additionally allows PDF.
func SniffKYCDocument(data []byte, allowPDF bool) (string, error) {
	if len(data) < 12 {
		return "", fmt.Errorf("file is too small to be a valid document")
	}
	switch {
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg", nil
	case data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
		return "image/png", nil
	case string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp", nil
	case allowPDF && string(data[0:5]) == "%PDF-":
		return "application/pdf", nil
	}
	return "", fmt.Errorf("unsupported file type — upload a JPEG, PNG or WebP photo%s", pdfHint(allowPDF))
}

func pdfHint(allowPDF bool) string {
	if allowPDF {
		return " (or PDF)"
	}
	return ""
}

// ── Submission ───────────────────────────────────────────────────────────────

type KYCSubmission struct {
	OrganizerID    string
	LegalFirstName string
	LegalLastName  string
	SAIDNumber     string
	IDDocument     []byte // required
	IDDocumentMime string
	ProofOfAddress []byte // optional
	ProofMime      string
}

// Submit processes a KYC submission end to end. Returns the resulting status
// so the handler can tell the organizer whether they were auto-verified.
// SubmitBasic performs TIER-1 verification: validates the SA ID number
// locally (Luhn, DOB, 18+, citizen/PR digit) and stores the legal identity
// encrypted. Deliberately free and instant — no document, no VerifyNow call,
// no admin queue. The caller is responsible for the role promotion.
func (s *KYCService) SubmitBasic(ctx context.Context, organizerID, firstName, lastName, idNumber string) error {
	dob, err := models.ValidateSAIDNumber(idNumber, time.Now())
	if err != nil {
		return fmt.Errorf("invalid ID number: %w", err)
	}
	encrypted, err := s.encrypt(idNumber)
	if err != nil {
		return fmt.Errorf("securing ID number: %w", err)
	}
	return repository.SaveBasicKYC(s.pool, ctx, repository.BasicKYCRecord{
		OrganizerID:    organizerID,
		LegalFirstName: firstName,
		LegalLastName:  lastName,
		IDEncrypted:    encrypted,
		IDLast4:        idNumber[len(idNumber)-4:],
		DOB:            dob,
	})
}

func (s *KYCService) Submit(ctx context.Context, sub KYCSubmission) (models.KYCStatus, error) {
	// No ID number in the payload = the organiser already gave it at tier-1
	// onboarding and the tier-2 form deliberately doesn't ask again. Recover
	// the stored one, decrypt it, and carry on as if it had been typed. It
	// still goes through the full validation below, so a legacy row holding
	// something malformed can't sneak past.
	if strings.TrimSpace(sub.SAIDNumber) == "" {
		rec, rErr := repository.GetKYCIdentity(s.pool, ctx, sub.OrganizerID)
		if rErr != nil {
			return "", fmt.Errorf("we don't have an ID number on file for you — please enter your 13-digit SA ID number")
		}
		idNumber, dErr := s.Decrypt(rec.IDEncrypted)
		if dErr != nil {
			return "", fmt.Errorf("could not read the ID number on file — please enter your 13-digit SA ID number")
		}
		sub.SAIDNumber = idNumber
	}

	dob, err := models.ValidateSAIDNumber(sub.SAIDNumber, time.Now())
	if err != nil {
		return "", fmt.Errorf("invalid ID number: %w", err)
	}

	encrypted, err := s.encrypt(sub.SAIDNumber)
	if err != nil {
		return "", fmt.Errorf("securing ID number: %w", err)
	}
	last4 := sub.SAIDNumber[len(sub.SAIDNumber)-4:]

	// Registry check (Didit) — best effort, before persisting so the result
	// lands in the same write. Any failure degrades to manual review.
	status := models.KYCPendingReview
	method := models.KYCMethodManual
	var registryRaw []byte
	var registryAt *time.Time

	if s.verifier.Name() != models.KYCMethodManual {
		res, vErr := s.verifier.Verify(ctx, VerifyIdentityInput{
			FirstName:   sub.LegalFirstName,
			LastName:    sub.LegalLastName,
			DateOfBirth: dob,
			IDNumber:    sub.SAIDNumber,
		})
		now := time.Now()
		registryAt = &now
		if vErr != nil {
			log.Warn().Err(vErr).Str("organizer_id", sub.OrganizerID).
				Msg("kyc: registry check failed — routing to manual review")
		} else {
			registryRaw = res.Raw
			if res.FullMatch {
				status = models.KYCVerified
				method = models.KYCMethodDidit
			}
		}
	}

	err = repository.SaveKYCSubmission(s.pool, ctx, repository.KYCRecord{
		OrganizerID:    sub.OrganizerID,
		LegalFirstName: sub.LegalFirstName,
		LegalLastName:  sub.LegalLastName,
		IDEncrypted:    encrypted,
		IDLast4:        last4,
		DOB:            dob,
		Status:         status,
		Method:         method,
		IDDocument:     sub.IDDocument,
		IDDocumentMime: sub.IDDocumentMime,
		ProofOfAddress: sub.ProofOfAddress,
		ProofMime:      sub.ProofMime,
		RegistryRaw:    registryRaw,
		RegistryAt:     registryAt,
	})
	if err != nil {
		return "", err
	}

	log.Info().Str("organizer_id", sub.OrganizerID).Str("status", string(status)).
		Str("method", method).Msg("kyc: submission processed")

	// KYC-gated role: a full registry match verifies identity on the spot, so
	// activate the organiser immediately. Best-effort — the submission itself
	// is already saved as verified, and an admin registry re-run or manual
	// approval also promotes, so a transient failure here self-heals.
	if status == models.KYCVerified {
		if pErr := repository.PromoteOrganizerAfterKYC(s.pool, ctx, sub.OrganizerID); pErr != nil {
			log.Warn().Err(pErr).Str("organizer_id", sub.OrganizerID).
				Msg("kyc: verified but organiser promotion failed — will resolve on admin re-run")
		}
	}

	return status, nil
}

// RunRegistryCheck re-runs the provider check for an existing submission
// (admin-triggered — e.g. after configuring the Didit key). Auto-verifies on
// a full match if the submission is still pending.
func (s *KYCService) RunRegistryCheck(ctx context.Context, organizerID string) (*VerifyIdentityResult, error) {
	if s.verifier.Name() == models.KYCMethodManual {
		return nil, fmt.Errorf("no registry verifier configured — set DIDIT_API_KEY")
	}

	rec, err := repository.GetKYCIdentity(s.pool, ctx, organizerID)
	if err != nil {
		return nil, err
	}
	idNumber, err := s.Decrypt(rec.IDEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypting stored ID: %w", err)
	}

	res, err := s.verifier.Verify(ctx, VerifyIdentityInput{
		FirstName:   rec.LegalFirstName,
		LastName:    rec.LegalLastName,
		DateOfBirth: rec.DOB,
		IDNumber:    idNumber,
	})
	if err != nil {
		return nil, err
	}

	if err := repository.SaveRegistryResult(s.pool, ctx, organizerID, res.Raw, res.FullMatch); err != nil {
		return nil, err
	}
	// A full match verifies the pending submission — activate the organiser.
	// Idempotent, so calling it when the status was already verified is safe.
	if res.FullMatch {
		if err := repository.PromoteOrganizerAfterKYC(s.pool, ctx, organizerID); err != nil {
			return nil, fmt.Errorf("registry check verified but promotion failed: %w", err)
		}
	}
	return res, nil
}