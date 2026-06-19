package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrTopupNotFound is returned when a top-up reference can't be matched.
var ErrTopupNotFound = errors.New("top-up not found")

// AdWallet is one organiser's pre-paid ad balance (keyed by users.id, matching
// ad_campaigns.organizer_id).
type AdWallet struct {
	ID          string    `json:"id"`
	OrganizerID string    `json:"organizer_id"`
	Balance     float64   `json:"balance"`
	TotalTopups float64   `json:"total_topups"`
	TotalSpent  float64   `json:"total_spent"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AdWalletTopup is one card top-up paid via Paystack.
type AdWalletTopup struct {
	ID                string     `json:"id"`
	OrganizerID       string     `json:"organizer_id"`
	Amount            float64    `json:"amount"`
	PaystackReference string     `json:"paystack_reference"`
	ReferenceNumber   *string    `json:"reference_number,omitempty"`
	Status            string     `json:"status"`
	CreatedAt         time.Time  `json:"created_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
}

// GetOrCreateAdWallet returns the organiser's wallet, creating an empty one on
// first use so balance reads and top-ups always have a row to target.
func GetOrCreateAdWallet(pool *pgxpool.Pool, organizerID string) (*AdWallet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var w AdWallet
	err := pool.QueryRow(ctx, `
		INSERT INTO ad_wallets (organizer_id)
		VALUES ($1)
		ON CONFLICT (organizer_id) DO UPDATE SET updated_at = ad_wallets.updated_at
		RETURNING id, organizer_id, balance, total_topups, total_spent, created_at, updated_at
	`, organizerID).Scan(
		&w.ID, &w.OrganizerID, &w.Balance, &w.TotalTopups, &w.TotalSpent, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrCreateAdWallet: %w", err)
	}
	return &w, nil
}

// CreateAdWalletTopup records a pending top-up keyed by its Paystack reference.
// The balance is only credited once the payment webhook confirms it.
func CreateAdWalletTopup(pool *pgxpool.Pool, organizerID string, amount float64, paystackReference string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ensure the wallet row exists so confirmation can credit it.
	if _, err := GetOrCreateAdWallet(pool, organizerID); err != nil {
		return err
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO ad_wallet_topups (organizer_id, amount, paystack_reference, reference_number, status)
		VALUES ($1, $2, $3, 'TUP-' || lpad(nextval('ad_topup_ref_seq')::text, 5, '0'), 'pending')
	`, organizerID, amount, paystackReference)
	if err != nil {
		return fmt.Errorf("CreateAdWalletTopup: %w", err)
	}
	return nil
}

// ConfirmAdWalletTopup credits the wallet for a successful top-up and resumes any
// campaigns that were paused for a zero balance. Idempotent: a top-up already
// marked completed is a no-op.
func ConfirmAdWalletTopup(pool *pgxpool.Pool, paystackReference string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var organizerID string
	var amount float64
	err = tx.QueryRow(ctx, `
		UPDATE ad_wallet_topups
		SET    status = 'completed', completed_at = NOW()
		WHERE  paystack_reference = $1 AND status = 'pending'
		RETURNING organizer_id, amount
	`, paystackReference).Scan(&organizerID, &amount)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already processed or unknown — treat as a no-op so the webhook 200s.
		return nil
	}
	if err != nil {
		return fmt.Errorf("ConfirmAdWalletTopup: %w", err)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE ad_wallets
		SET    balance      = balance + $2,
		       total_topups = total_topups + $2,
		       updated_at   = NOW()
		WHERE  organizer_id = $1
	`, organizerID, amount); err != nil {
		return fmt.Errorf("ConfirmAdWalletTopup credit: %w", err)
	}

	// No campaign status flip is needed: serving is gated on wallet balance > 0
	// (see GetSponsoredForFeed), so a topped-up wallet immediately resumes
	// serving without touching campaign status. Manually-paused campaigns stay
	// paused, as the organiser intended.

	return tx.Commit(ctx)
}

// FailAdWalletTopup marks a top-up failed. The balance is untouched.
func FailAdWalletTopup(pool *pgxpool.Pool, paystackReference string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ad_wallet_topups
		SET    status = 'failed'
		WHERE  paystack_reference = $1 AND status = 'pending'
	`, paystackReference)
	return err
}

// GetAdWalletTopups returns an organiser's top-up history, most recent first.
func GetAdWalletTopups(pool *pgxpool.Pool, organizerID string, limit int) ([]AdWalletTopup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT id, organizer_id, amount, paystack_reference, reference_number, status, created_at, completed_at
		FROM   ad_wallet_topups
		WHERE  organizer_id = $1
		ORDER BY created_at DESC
		LIMIT  $2
	`, organizerID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetAdWalletTopups: %w", err)
	}
	defer rows.Close()

	var out []AdWalletTopup
	for rows.Next() {
		var t AdWalletTopup
		if err := rows.Scan(
			&t.ID, &t.OrganizerID, &t.Amount, &t.PaystackReference,
			&t.ReferenceNumber, &t.Status, &t.CreatedAt, &t.CompletedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}