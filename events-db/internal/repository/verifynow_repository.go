package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SaveBankVerification records the outcome of the bank-account ownership check
// run at bank-detail entry. Only the result is stored — never the account
// number (we keep only last 4 elsewhere).
func SaveBankVerification(pool *pgxpool.Pool, ctx context.Context, organizerID string, verified bool, ref string, raw []byte) error {
	_, err := pool.Exec(ctx, `
		UPDATE organizers
		SET bank_account_verified    = $1,
		    bank_account_verified_at = CASE WHEN $1 THEN CURRENT_TIMESTAMP ELSE NULL END,
		    bank_verification_ref    = $2,
		    updated_at               = CURRENT_TIMESTAMP
		WHERE id = $3
	`, verified, nullIfEmpty(ref), organizerID)
	if err != nil {
		return fmt.Errorf("SaveBankVerification: %w", err)
	}
	return nil
}

// SaveDHAIDVerification records the said_verification outcome run at promotion.
func SaveDHAIDVerification(pool *pgxpool.Pool, ctx context.Context, organizerID string, verified bool, raw []byte) error {
	_, err := pool.Exec(ctx, `
		UPDATE organizers
		SET dha_id_verified    = $1,
		    dha_id_verified_at = CASE WHEN $1 THEN CURRENT_TIMESTAMP ELSE dha_id_verified_at END,
		    dha_id_check       = $2,
		    dha_id_checked_at  = CURRENT_TIMESTAMP,
		    updated_at         = CURRENT_TIMESTAMP
		WHERE id = $3
	`, verified, raw, organizerID)
	if err != nil {
		return fmt.Errorf("SaveDHAIDVerification: %w", err)
	}
	return nil
}

// PromotionIdentity holds the fields needed to run the DHA ID check for one
// organizer at promotion time: the (encrypted) ID plus legal names, and the
// current bank-verified flag so the caller can enforce both gates.
type PromotionIdentity struct {
	LegalFirstName      string
	LegalLastName       string
	IDEncrypted         []byte
	KYCStatus           string
	BankAccountVerified bool
}

// GetPromotionIdentity fetches promotion inputs by the organizer's row id.
func GetPromotionIdentity(pool *pgxpool.Pool, ctx context.Context, organizerID string) (*PromotionIdentity, error) {
	var p PromotionIdentity
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(legal_first_name, ''), COALESCE(legal_last_name, ''),
		       sa_id_encrypted, kyc_status, bank_account_verified
		FROM organizers WHERE id = $1
	`, organizerID).Scan(&p.LegalFirstName, &p.LegalLastName, &p.IDEncrypted, &p.KYCStatus, &p.BankAccountVerified)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("organizer not found")
		}
		return nil, fmt.Errorf("GetPromotionIdentity: %w", err)
	}
	return &p, nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}