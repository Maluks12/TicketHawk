package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Writes ───────────────────────────────────────────────────────────────────

type KYCRecord struct {
	OrganizerID    string
	LegalFirstName string
	LegalLastName  string
	IDEncrypted    []byte
	IDLast4        string
	DOB            time.Time
	Status         models.KYCStatus
	Method         string
	IDDocument     []byte
	IDDocumentMime string
	ProofOfAddress []byte
	ProofMime      string
	RegistryRaw    []byte
	RegistryAt     *time.Time
}

// SaveKYCSubmission persists a (re)submission. Allowed from 'none' and
// 'rejected'; a pending or verified organizer can't overwrite their identity
// without an admin reset — that path would let a verified account swap in a
// different person's details unnoticed.
func SaveKYCSubmission(pool *pgxpool.Pool, ctx context.Context, r KYCRecord) error {
	var proof []byte
	var proofMime *string
	if len(r.ProofOfAddress) > 0 {
		proof = r.ProofOfAddress
		proofMime = &r.ProofMime
	}
	var registryAt interface{}
	var registryRaw interface{}
	if r.RegistryAt != nil {
		registryAt = *r.RegistryAt
		registryRaw = r.RegistryRaw
	}

	verifiedAt := "NULL"
	if r.Status == models.KYCVerified {
		verifiedAt = "CURRENT_TIMESTAMP"
	}

	tag, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE organizers
		SET legal_first_name = $1, legal_last_name = $2,
		    sa_id_encrypted = $3, sa_id_last4 = $4, sa_id_dob = $5,
		    kyc_status = $6, kyc_method = $7,
		    kyc_submitted_at = CURRENT_TIMESTAMP,
		    kyc_verified_at = %s,
		    kyc_rejection_reason = NULL, kyc_reviewed_by = NULL,
		    id_document = $8, id_document_mime = $9,
		    proof_of_address = $10, proof_of_address_mime = $11,
		    registry_check = $12, registry_checked_at = $13,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $14 AND kyc_status IN ('none', 'rejected')
	`, verifiedAt),
		r.LegalFirstName, r.LegalLastName,
		r.IDEncrypted, r.IDLast4, r.DOB,
		string(r.Status), r.Method,
		r.IDDocument, r.IDDocumentMime,
		proof, proofMime,
		registryRaw, registryAt,
		r.OrganizerID)
	if err != nil {
		return fmt.Errorf("SaveKYCSubmission: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrKYCNotSubmittable
	}
	return nil
}

var ErrKYCNotSubmittable = errors.New("kyc already submitted or verified — contact support to change your identity details")

// AdminDecideKYC approves or rejects a pending submission.
func AdminDecideKYC(pool *pgxpool.Pool, ctx context.Context, organizerID, adminID string, approve bool, reason string) error {
	var query string
	args := []interface{}{adminID, organizerID}
	if approve {
		query = `
			UPDATE organizers
			SET kyc_status = 'verified', kyc_method = COALESCE(kyc_method, 'manual'),
			    kyc_verified_at = CURRENT_TIMESTAMP, kyc_reviewed_by = $1,
			    kyc_rejection_reason = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE id = $2 AND kyc_status = 'pending_review'`
	} else {
		query = `
			UPDATE organizers
			SET kyc_status = 'rejected', kyc_reviewed_by = $1,
			    kyc_rejection_reason = $3, updated_at = CURRENT_TIMESTAMP
			WHERE id = $2 AND kyc_status = 'pending_review'`
		args = append(args, reason)
	}
	tag, err := pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("AdminDecideKYC: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("submission is not pending review")
	}
	return nil
}

// SaveRegistryResult stores a re-run provider check; auto-verifies a pending
// submission on a full match.
func SaveRegistryResult(pool *pgxpool.Pool, ctx context.Context, organizerID string, raw []byte, fullMatch bool) error {
	_, err := pool.Exec(ctx, `
		UPDATE organizers
		SET registry_check = $1, registry_checked_at = CURRENT_TIMESTAMP,
		    kyc_status = CASE WHEN $2 AND kyc_status = 'pending_review' THEN 'verified' ELSE kyc_status END,
		    kyc_method = CASE WHEN $2 AND kyc_status = 'pending_review' THEN 'didit' ELSE kyc_method END,
		    kyc_verified_at = CASE WHEN $2 AND kyc_status = 'pending_review' THEN CURRENT_TIMESTAMP ELSE kyc_verified_at END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`, raw, fullMatch, organizerID)
	if err != nil {
		return fmt.Errorf("SaveRegistryResult: %w", err)
	}
	return nil
}

// ── Reads ────────────────────────────────────────────────────────────────────

// GetKYCProfile — organizer-facing view (no decrypted ID, no documents).
func GetKYCProfile(pool *pgxpool.Pool, ctx context.Context, organizerID string) (*models.KYCProfile, error) {
	var p models.KYCProfile
	var status string
	var hasDoc, hasProof bool
	err := pool.QueryRow(ctx, `
		SELECT kyc_status, kyc_method, legal_first_name, legal_last_name, sa_id_last4,
		       kyc_submitted_at, kyc_verified_at, kyc_rejection_reason,
		       id_document IS NOT NULL, proof_of_address IS NOT NULL
		FROM organizers WHERE id = $1
	`, organizerID).Scan(&status, &p.Method, &p.LegalFirstName, &p.LegalLastName, &p.IDLast4,
		&p.SubmittedAt, &p.VerifiedAt, &p.RejectionReason, &hasDoc, &hasProof)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("organizer not found")
		}
		return nil, fmt.Errorf("GetKYCProfile: %w", err)
	}
	p.Status = models.KYCStatus(status)
	p.HasIDDocument = hasDoc
	p.HasProofOfAddr = hasProof
	return &p, nil
}

// KYCIdentity — decryptable identity fields for provider re-checks.
type KYCIdentity struct {
	LegalFirstName string
	LegalLastName  string
	IDEncrypted    []byte
	DOB            time.Time
}

func GetKYCIdentity(pool *pgxpool.Pool, ctx context.Context, organizerID string) (*KYCIdentity, error) {
	var r KYCIdentity
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(legal_first_name, ''), COALESCE(legal_last_name, ''),
		       sa_id_encrypted, sa_id_dob
		FROM organizers
		WHERE id = $1 AND sa_id_encrypted IS NOT NULL
	`, organizerID).Scan(&r.LegalFirstName, &r.LegalLastName, &r.IDEncrypted, &r.DOB)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no KYC submission on file")
		}
		return nil, fmt.Errorf("GetKYCIdentity: %w", err)
	}
	return &r, nil
}

// AdminKYCQueueRow — one row of the admin review list.
type AdminKYCQueueRow struct {
	OrganizerID    string     `json:"organizer_id"`
	BusinessName   string     `json:"business_name"`
	BusinessEmail  string     `json:"business_email"`
	Status         string     `json:"status"`
	Method         *string    `json:"method,omitempty"`
	LegalFirstName *string    `json:"legal_first_name,omitempty"`
	LegalLastName  *string    `json:"legal_last_name,omitempty"`
	SubmittedAt    *time.Time `json:"submitted_at,omitempty"`
	HasRegistry    bool       `json:"has_registry_check"`
}

// ListKYCSubmissions lists submissions by status (default pending_review).
func ListKYCSubmissions(pool *pgxpool.Pool, ctx context.Context, status string) ([]AdminKYCQueueRow, error) {
	if status == "" {
		status = string(models.KYCPendingReview)
	}
	switch models.KYCStatus(status) {
	case models.KYCPendingReview, models.KYCVerified, models.KYCRejected:
	default:
		return nil, fmt.Errorf("invalid status filter")
	}

	rows, err := pool.Query(ctx, `
		SELECT id, business_name, business_email, kyc_status, kyc_method,
		       legal_first_name, legal_last_name, kyc_submitted_at,
		       registry_check IS NOT NULL
		FROM organizers
		WHERE kyc_status = $1
		ORDER BY kyc_submitted_at ASC NULLS LAST
		LIMIT 200
	`, status)
	if err != nil {
		return nil, fmt.Errorf("ListKYCSubmissions: %w", err)
	}
	defer rows.Close()

	var out []AdminKYCQueueRow
	for rows.Next() {
		var r AdminKYCQueueRow
		if err := rows.Scan(&r.OrganizerID, &r.BusinessName, &r.BusinessEmail, &r.Status,
			&r.Method, &r.LegalFirstName, &r.LegalLastName, &r.SubmittedAt, &r.HasRegistry); err != nil {
			return nil, fmt.Errorf("ListKYCSubmissions scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AdminKYCRaw — everything the admin detail view needs (ID still encrypted;
// the service layer decrypts).
type AdminKYCRaw struct {
	Detail      models.AdminKYCDetail
	IDEncrypted []byte
	RegistryRaw []byte
}

func GetAdminKYCDetail(pool *pgxpool.Pool, ctx context.Context, organizerID string) (*AdminKYCRaw, error) {
	var r AdminKYCRaw
	var status string
	var dob *time.Time
	var hasDoc, hasProof bool
	err := pool.QueryRow(ctx, `
		SELECT id, business_name, business_email, kyc_status, kyc_method,
		       legal_first_name, legal_last_name, sa_id_encrypted, sa_id_dob,
		       kyc_submitted_at, kyc_verified_at, kyc_rejection_reason,
		       id_document IS NOT NULL, proof_of_address IS NOT NULL,
		       bank_account_name, registry_check, registry_checked_at
		FROM organizers WHERE id = $1
	`, organizerID).Scan(
		&r.Detail.OrganizerID, &r.Detail.BusinessName, &r.Detail.BusinessEmail, &status, &r.Detail.Method,
		&r.Detail.LegalFirstName, &r.Detail.LegalLastName, &r.IDEncrypted, &dob,
		&r.Detail.SubmittedAt, &r.Detail.VerifiedAt, &r.Detail.RejectionReason,
		&hasDoc, &hasProof,
		&r.Detail.BankAccountName, &r.RegistryRaw, &r.Detail.RegistryAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("organizer not found")
		}
		return nil, fmt.Errorf("GetAdminKYCDetail: %w", err)
	}
	r.Detail.Status = models.KYCStatus(status)
	r.Detail.HasIDDocument = hasDoc
	r.Detail.HasProofOfAddr = hasProof
	if dob != nil {
		s := dob.Format("2006-01-02")
		r.Detail.DateOfBirth = &s
	}

	// Name match computed at read time so it never goes stale.
	first, last, bank := "", "", ""
	if r.Detail.LegalFirstName != nil {
		first = *r.Detail.LegalFirstName
	}
	if r.Detail.LegalLastName != nil {
		last = *r.Detail.LegalLastName
	}
	if r.Detail.BankAccountName != nil {
		bank = *r.Detail.BankAccountName
	}
	r.Detail.NameMatch = models.MatchNames(bank, first, last)
	return &r, nil
}

// GetKYCDocument streams one of the two stored documents.
func GetKYCDocument(pool *pgxpool.Pool, ctx context.Context, organizerID, kind string) (data []byte, mime string, err error) {
	col, mimeCol := "id_document", "id_document_mime"
	if kind == "proof-of-address" {
		col, mimeCol = "proof_of_address", "proof_of_address_mime"
	}
	err = pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s, COALESCE(%s, 'application/octet-stream') FROM organizers WHERE id = $1 AND %s IS NOT NULL`, col, mimeCol, col),
		organizerID).Scan(&data, &mime)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", fmt.Errorf("document not found")
		}
		return nil, "", fmt.Errorf("GetKYCDocument: %w", err)
	}
	return data, mime, nil
}

// ── Publish-gate lookups ─────────────────────────────────────────────────────

// OrganizerPublishGates — the organizer-level facts PublishEvent needs.
type OrganizerPublishGates struct {
	KYCVerified   bool
	PayoutEnabled bool
	KYCLevel       int
}

// GetOrganizerPublishGates fetches gate state by the events.organizer_id
// (which is a users.id). No organizer row at all means both gates fail.
func GetOrganizerPublishGates(pool *pgxpool.Pool, ctx context.Context, userID string) (OrganizerPublishGates, error) {
	var g OrganizerPublishGates
	err := pool.QueryRow(ctx, `
		SELECT kyc_status = 'verified', payout_enabled, kyc_level
		FROM organizers WHERE user_id = $1
	`, userID).Scan(&g.KYCVerified, &g.PayoutEnabled, &g.KYCLevel)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OrganizerPublishGates{}, nil
		}
		return g, fmt.Errorf("GetOrganizerPublishGates: %w", err)
	}
	return g, nil
}

// ── Basic KYC (tier 1) ───────────────────────────────────────────────────────

// BasicKYCRecord is the tier-1 identity: legal name + encrypted SA ID. No
// document, no registry call — validated locally by the service layer.
type BasicKYCRecord struct {
	OrganizerID    string
	LegalFirstName string
	LegalLastName  string
	IDEncrypted    []byte
	IDLast4        string
	DOB            time.Time
}

// SaveBasicKYC stores the tier-1 identity and raises kyc_level to at least 1.
// Same overwrite guard as full submissions: a pending or verified identity
// can never be silently replaced (that would let a verified account swap in a
// different person). kyc_status is left untouched — basic KYC does not verify.
func SaveBasicKYC(pool *pgxpool.Pool, ctx context.Context, r BasicKYCRecord) error {
	tag, err := pool.Exec(ctx, `
		UPDATE organizers
		SET legal_first_name = $2,
		    legal_last_name  = $3,
		    sa_id_encrypted  = $4,
		    sa_id_last4      = $5,
		    sa_id_dob        = $6,
		    kyc_level         = GREATEST(kyc_level, 1),
		    updated_at       = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND kyc_status IN ('none', 'rejected')
	`, r.OrganizerID, r.LegalFirstName, r.LegalLastName,
		r.IDEncrypted, r.IDLast4, r.DOB)
	if err != nil {
		return fmt.Errorf("SaveBasicKYC: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("identity already submitted or verified — contact support to change it")
	}
	return nil
}

// GetKYCLevel returns the organiser's current tier (0 if the row is missing).
func GetKYCLevel(pool *pgxpool.Pool, ctx context.Context, organizerID string) (int, error) {
	var tier int
	err := pool.QueryRow(ctx,
		`SELECT kyc_level FROM organizers WHERE id = $1`, organizerID).Scan(&tier)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return tier, err
}