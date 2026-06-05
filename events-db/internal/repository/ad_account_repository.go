package repository

import (
	"context"
	"errors"
	"events-db/internal/models"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAdAccountNotFound     = errors.New("ad account not found")
	ErrAdAccountUnauthorized = errors.New("ad account belongs to a different organizer")
)

// CreateAdAccount creates a new ad account for an organizer.
// Newly created accounts have no payment card — the organizer adds one
// separately via RegisterAdAccountPaymentMethod.
func CreateAdAccount(pool *pgxpool.Pool, organizerID string, dto *models.CreateAdAccountDTO) (*models.AdAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var a models.AdAccount
	err := pool.QueryRow(ctx, `
		INSERT INTO ad_accounts (organizer_id, name)
		VALUES ($1, $2)
		RETURNING id, organizer_id, name, status,
		          card_name, card_last4, card_brand, card_expiry,
		          ad_payment_email,
		          currency, created_at, updated_at
	`, organizerID, dto.Name).Scan(
		&a.ID, &a.OrganizerID, &a.Name, &a.Status,
		&a.CardName, &a.CardLast4, &a.CardBrand, &a.CardExpiry,
		&a.AdPaymentEmail,
		&a.Currency, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateAdAccount: %w", err)
	}
	a.HasPaymentMethod = a.CardLast4 != nil
	return &a, nil
}

// GetAdAccounts returns all ad accounts for an organizer, with denormalised
// stats (total lifetime spend, count of active campaigns) joined in.
func GetAdAccounts(pool *pgxpool.Pool, organizerID string) ([]models.AdAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT
		    aa.id, aa.organizer_id, aa.name, aa.status,
		    aa.card_name, aa.card_last4, aa.card_brand, aa.card_expiry,
		    aa.ad_payment_email,
		    aa.currency, aa.created_at, aa.updated_at,
		    COALESCE(SUM(ac.total_spent), 0)                                    AS total_spent,
		    COUNT(ac.id) FILTER (WHERE ac.status = 'active')                    AS active_campaigns
		FROM   ad_accounts aa
		LEFT JOIN ad_campaigns ac ON ac.ad_account_id = aa.id
		WHERE  aa.organizer_id = $1
		GROUP BY aa.id
		ORDER BY aa.created_at ASC
	`, organizerID)
	if err != nil {
		return nil, fmt.Errorf("GetAdAccounts: %w", err)
	}
	defer rows.Close()

	var out []models.AdAccount
	for rows.Next() {
		var a models.AdAccount
		if err := rows.Scan(
			&a.ID, &a.OrganizerID, &a.Name, &a.Status,
			&a.CardName, &a.CardLast4, &a.CardBrand, &a.CardExpiry,
			&a.AdPaymentEmail,
			&a.Currency, &a.CreatedAt, &a.UpdatedAt,
			&a.TotalSpent, &a.ActiveCampaigns,
		); err != nil {
			return nil, err
		}
		a.HasPaymentMethod = a.CardLast4 != nil
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAdAccount returns a single ad account with ownership check.
func GetAdAccount(pool *pgxpool.Pool, accountID, organizerID string) (*models.AdAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var a models.AdAccount
	err := pool.QueryRow(ctx, `
		SELECT
		    aa.id, aa.organizer_id, aa.name, aa.status,
		    aa.card_name, aa.card_last4, aa.card_brand, aa.card_expiry,
		    aa.ad_payment_email,
		    aa.currency, aa.created_at, aa.updated_at,
		    COALESCE(SUM(ac.total_spent), 0)                                    AS total_spent,
		    COUNT(ac.id) FILTER (WHERE ac.status = 'active')                    AS active_campaigns
		FROM   ad_accounts aa
		LEFT JOIN ad_campaigns ac ON ac.ad_account_id = aa.id
		WHERE  aa.id = $1
		GROUP BY aa.id
	`, accountID).Scan(
		&a.ID, &a.OrganizerID, &a.Name, &a.Status,
		&a.CardName, &a.CardLast4, &a.CardBrand, &a.CardExpiry,
		&a.AdPaymentEmail,
		&a.Currency, &a.CreatedAt, &a.UpdatedAt,
		&a.TotalSpent, &a.ActiveCampaigns,
	)
	if err != nil {
		return nil, ErrAdAccountNotFound
	}
	if a.OrganizerID != organizerID {
		return nil, ErrAdAccountUnauthorized
	}
	a.HasPaymentMethod = a.CardLast4 != nil
	return &a, nil
}

// UpdateAdAccount updates name and/or status of an ad account.
func UpdateAdAccount(pool *pgxpool.Pool, accountID, organizerID string, dto *models.UpdateAdAccountDTO) (*models.AdAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	a, err := GetAdAccount(pool, accountID, organizerID)
	if err != nil {
		return nil, err
	}
	if dto.Name != nil {
		a.Name = *dto.Name
	}
	if dto.Status != nil {
		a.Status = *dto.Status
	}

	if _, err = pool.Exec(ctx, `
		UPDATE ad_accounts
		SET    name = $1, status = $2, updated_at = NOW()
		WHERE  id = $3
	`, a.Name, a.Status, accountID); err != nil {
		return nil, fmt.Errorf("UpdateAdAccount: %w", err)
	}
	return a, nil
}

// RegisterAdAccountPaymentMethod stores card display metadata against a specific
// ad account. Only the safe display fields are stored — raw card numbers and CVV
// are handled client-side and never reach the server.
// Also re-activates any payment_failed campaigns under this account.
func RegisterAdAccountPaymentMethod(pool *pgxpool.Pool, accountID, organizerID string, dto *models.RegisterAdAccountPaymentMethodDTO) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE ad_accounts
		SET    card_name       = $1,
		       card_last4      = $2,
		       card_brand      = $3,
		       card_expiry     = $4,
		       ad_payment_email = $5,
		       updated_at      = NOW()
		WHERE  id = $6 AND organizer_id = $7
	`, dto.CardName, dto.CardLast4, dto.CardBrand, dto.CardExpiry,
		dto.Email, accountID, organizerID)
	if err != nil {
		return fmt.Errorf("RegisterAdAccountPaymentMethod: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrAdAccountNotFound
	}

	// Re-activate payment_failed campaigns under this account.
	_, _ = pool.Exec(ctx, `
		UPDATE ad_campaigns
		SET    status = 'active', updated_at = NOW()
		WHERE  ad_account_id = $1
		  AND  status = 'payment_failed'
		  AND  (end_date IS NULL OR end_date >= CURRENT_DATE)
	`, accountID)

	return nil
}

// GetAdAccountPaymentMethodStatus returns the card-on-file status for one account.
func GetAdAccountPaymentMethodStatus(pool *pgxpool.Pool, accountID, organizerID string) (string, *string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cardLast4 *string
	var email *string
	err := pool.QueryRow(ctx, `
		SELECT card_last4, ad_payment_email
		FROM   ad_accounts
		WHERE  id = $1 AND organizer_id = $2
	`, accountID, organizerID).Scan(&cardLast4, &email)
	if err != nil {
		return "none", nil, ErrAdAccountNotFound
	}

	status := "none"
	if cardLast4 != nil {
		status = "active"
	}
	return status, email, nil
}