package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAdAccountNotFound     = errors.New("ad account not found")
	ErrAdAccountUnauthorized = errors.New("ad account belongs to a different organizer")
)

// CreateAdAccount creates a new ad account for an organizer. Ad accounts now
// group campaigns; funding is the per-organiser wallet, so there is no card on
// the account.
func CreateAdAccount(pool *pgxpool.Pool, organizerID string, dto *models.CreateAdAccountDTO) (*models.AdAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var a models.AdAccount
	err := pool.QueryRow(ctx, `
		INSERT INTO ad_accounts (organizer_id, name)
		VALUES ($1, $2)
		RETURNING id, organizer_id, name, status, currency, created_at, updated_at
	`, organizerID, dto.Name).Scan(
		&a.ID, &a.OrganizerID, &a.Name, &a.Status, &a.Currency, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateAdAccount: %w", err)
	}
	return &a, nil
}

// GetAdAccounts returns all ad accounts for an organizer, with denormalised
// stats (total lifetime spend, count of active campaigns) joined in.
func GetAdAccounts(pool *pgxpool.Pool, organizerID string) ([]models.AdAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT
		    aa.id, aa.organizer_id, aa.name, aa.status, aa.currency, aa.created_at, aa.updated_at,
		    COALESCE(SUM(ac.total_spent), 0)                 AS total_spent,
		    COUNT(ac.id) FILTER (WHERE ac.status = 'active') AS active_campaigns
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
			&a.ID, &a.OrganizerID, &a.Name, &a.Status, &a.Currency, &a.CreatedAt, &a.UpdatedAt,
			&a.TotalSpent, &a.ActiveCampaigns,
		); err != nil {
			return nil, err
		}
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
		    aa.id, aa.organizer_id, aa.name, aa.status, aa.currency, aa.created_at, aa.updated_at,
		    COALESCE(SUM(ac.total_spent), 0)                 AS total_spent,
		    COUNT(ac.id) FILTER (WHERE ac.status = 'active') AS active_campaigns
		FROM   ad_accounts aa
		LEFT JOIN ad_campaigns ac ON ac.ad_account_id = aa.id
		WHERE  aa.id = $1
		GROUP BY aa.id
	`, accountID).Scan(
		&a.ID, &a.OrganizerID, &a.Name, &a.Status, &a.Currency, &a.CreatedAt, &a.UpdatedAt,
		&a.TotalSpent, &a.ActiveCampaigns,
	)
	if err != nil {
		return nil, ErrAdAccountNotFound
	}
	if a.OrganizerID != organizerID {
		return nil, ErrAdAccountUnauthorized
	}
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