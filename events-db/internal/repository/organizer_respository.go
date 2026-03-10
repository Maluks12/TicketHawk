package repository

import (
	"context"
	"errors"
	"events-db/internal/models"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOrganizerNotFound = errors.New("organizer not found")

func CreateOrganizer(pool *pgxpool.Pool, dto *models.CreateOrganizerDTO, userID string) (*models.Organizer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		INSERT INTO organizers (
			user_id, business_name, business_email, business_phone, 
			website_url, description, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, business_name, status, created_at
	`
	 var org models.Organizer
    err := pool.QueryRow(ctx, query,
        userID, dto.BusinessName, dto.BusinessEmail, dto.BusinessPhone,
        dto.WebsiteURL, dto.Description, models.OrganizerPending,
    ).Scan(&org.ID, &org.UserID, &org.BusinessName, &org.Status, &org.CreatedAt)

    if err != nil {
        return nil, fmt.Errorf("failed to create organizer: %w", err)
    }

    return &org, nil
}

func ApproveOrganizer(pool *pgxpool.Pool, organizerID string, adminID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	query := `
		UPDATE organizers
		SET status = $1, verified = TRUE, verified_at = CURRENT_TIMESTAMP, verified_by = $2
		WHERE id = $3 AND status = $4
		RETURNING user_id
	`

	var userID string

	err = tx.QueryRow(ctx, query, models.OrganizerApproved, adminID, organizerID, models.OrganizerPending).Scan(&userID)
	if err != nil {
		return fmt.Errorf("failed to approve organizer: %w", err)
	}

	updateUser := `
		UPDATE users
		SET role = $1
		WHERE id = $2
	`

	_, err = tx.Exec(ctx, updateUser, models.RoleOrganizer, userID)
	if err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}

	return tx.Commit(ctx)
}

func RejectOrganizer(pool *pgxpool.Pool, organizerID string, adminID string, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		UPDATE organizers
		SET status = $1, rejection_reason = $2, verified_by = $3
		WHERE id = $4 AND status = $5
	`

	cmd, err := pool.Exec(ctx, query, models.OrganizerRejected, reason, adminID, organizerID,models.OrganizerPending)
	if err != nil {
		return fmt.Errorf("failed to reject organizer: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ErrOrganizerNotFound
	}

	return nil
}

func GetOrganizerByUserID(pool *pgxpool.Pool, userID string) (*models.Organizer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT id, user_id, business_name, business_email, business_phone,
               website_url, description, logo_url, verified, status,
               commission_rate, created_at, updated_at
        FROM organizers
        WHERE user_id = $1
	`
	 var org models.Organizer
    err := pool.QueryRow(ctx, query, userID).Scan(
        &org.ID, &org.UserID, &org.BusinessName, &org.BusinessEmail, &org.BusinessPhone,
        &org.WebsiteURL, &org.Description, &org.LogoURL, &org.Verified, &org.Status,
        &org.CommissionRate, &org.CreatedAt, &org.UpdatedAt,
    )

    if err != nil {
        return nil, fmt.Errorf("failed to get organizer: %w", err)
    }

	return &org, nil
}

func GetPendingOrganizers(pool *pgxpool.Pool) ([]models.Organizer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT o.id, o.user_id, o.business_name, o.business_email,
               o.description, o.status, o.created_at,
               u.email, u.first_name, u.last_name
        FROM organizers o
        JOIN users u ON o.user_id = u.id
        WHERE o.status = $1
        ORDER BY o.created_at ASC
	`

	rows, err := pool.Query(ctx, query, models.OrganizerPending)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	organizers := make([]models.Organizer, 0)
	for rows.Next() {
		var org models.Organizer
		var userEmail, userFirstName, userLastName string

		err = rows.Scan(
			&org.ID, 
			&org.UserID, 
			&org.BusinessName, 
			&org.BusinessEmail,
            &org.Description, 
			&org.Status, 
			&org.CreatedAt,
            &userEmail, 
			&userFirstName, 
			&userLastName,
		)
		if err != nil {
			return nil, err
		}
		org.BusinessEmail = userEmail
		organizers = append(organizers, org)
	}

	return organizers, nil
}