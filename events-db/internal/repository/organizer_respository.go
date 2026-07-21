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

// CreateOrganizer records a user's APPLICATION to become an organiser.
//
// KYC-gated flow (changed from the old Meta-style instant self-service):
//  1. Inserts the organizer row with status = 'pending' and verified = FALSE
//  2. Does NOT touch users.role — the account stays an attendee
//
// The role upgrade happens exclusively in PromoteOrganizerAfterKYC, which
// fires when the applicant's identity verification passes (auto registry
// match, admin KYC approval, or admin registry re-run).
func CreateOrganizer(pool *pgxpool.Pool, dto *models.CreateOrganizerDTO, userID string) (*models.Organizer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Insert organizer — pending until identity verification passes.
	var org models.Organizer
	err = tx.QueryRow(ctx, `
		INSERT INTO organizers (
			user_id, business_name, business_email, business_phone,
			website_url, description, status, verified, verified_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE, NULL)
		RETURNING id, user_id, business_name, status, created_at
	`,
		userID, dto.BusinessName, dto.BusinessEmail, dto.BusinessPhone,
		dto.WebsiteURL, dto.Description, models.OrganizerPending,
	).Scan(&org.ID, &org.UserID, &org.BusinessName, &org.Status, &org.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create organizer: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit organizer creation: %w", err)
	}

	return &org, nil
}

// PromoteOrganizerAfterKYC activates an organiser whose identity verification
// has passed: flips users.role attendee → organizer and marks the organizer
// row approved/verified. Idempotent — safe to call from every place KYC can
// become 'verified' (submission auto-match, admin approval, registry re-run).
// Admin accounts are never downgraded: only 'attendee' is upgraded.
func PromoteOrganizerAfterKYC(pool *pgxpool.Pool, ctx context.Context, organizerID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("PromoteOrganizerAfterKYC: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `
		UPDATE users u
		SET role = $1, updated_at = NOW()
		FROM organizers o
		WHERE o.id = $2 AND u.id = o.user_id AND u.role = $3
	`, models.RoleOrganizer, organizerID, models.RoleAttendee); err != nil {
		return fmt.Errorf("PromoteOrganizerAfterKYC: role: %w", err)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE organizers
		SET status = $1, verified = TRUE,
		    verified_at = COALESCE(verified_at, CURRENT_TIMESTAMP),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, models.OrganizerApproved, organizerID); err != nil {
		return fmt.Errorf("PromoteOrganizerAfterKYC: organizer: %w", err)
	}

	return tx.Commit(ctx)
}

// GetOrganizerByUserID returns the organizer profile for an authenticated user.
func GetOrganizerByUserID(pool *pgxpool.Pool, userID string) (*models.Organizer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var org models.Organizer
	err := pool.QueryRow(ctx, `
		SELECT id, user_id, business_name, business_email, business_phone,
		       website_url, description, logo_url, verified, status,
		       commission_rate, created_at, updated_at
		FROM organizers
		WHERE user_id = $1
	`, userID).Scan(
		&org.ID, &org.UserID, &org.BusinessName, &org.BusinessEmail, &org.BusinessPhone,
		&org.WebsiteURL, &org.Description, &org.LogoURL, &org.Verified, &org.Status,
		&org.CommissionRate, &org.CreatedAt, &org.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get organizer: %w", err)
	}
	return &org, nil
}

// GetAllOrganizers returns all organizer accounts, optionally filtered by status.
func GetAllOrganizers(pool *pgxpool.Pool, statusFilter string) ([]models.Organizer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT o.id, o.user_id, o.business_name, o.business_email, o.business_phone,
		       o.description, o.status, o.verified, o.created_at, o.updated_at
		FROM organizers o
	`
	args := []interface{}{}
	if statusFilter != "" {
		query += " WHERE o.status = $1"
		args = append(args, statusFilter)
	}
	query += " ORDER BY o.created_at DESC"

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	organizers := make([]models.Organizer, 0)
	for rows.Next() {
		var org models.Organizer
		if err := rows.Scan(
			&org.ID, &org.UserID, &org.BusinessName, &org.BusinessEmail, &org.BusinessPhone,
			&org.Description, &org.Status, &org.Verified, &org.CreatedAt, &org.UpdatedAt,
		); err != nil {
			return nil, err
		}
		organizers = append(organizers, org)
	}
	return organizers, nil
}

// AdminUpdateOrganizerStatus allows admin to suspend, ban, or reinstate an organiser.
// Replaces the old Approve/Reject queue — there is no longer a registration queue.
func AdminUpdateOrganizerStatus(pool *pgxpool.Pool, organizerID string, status models.OrganizerStatus, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE organizers
		SET status = $1, rejection_reason = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`, status, reason, organizerID)
	if err != nil {
		return fmt.Errorf("failed to update organizer status: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrOrganizerNotFound
	}
	return nil
}

// ── Legacy functions kept for backwards compatibility with existing tests ──────

// ApproveOrganizer handles any old "pending" rows still in the DB.
// New accounts are auto-approved — this should only be hit by legacy data or tests.
func ApproveOrganizer(pool *pgxpool.Pool, organizerID string, adminID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `
		UPDATE organizers
		SET status = $1, verified = TRUE, verified_at = CURRENT_TIMESTAMP, verified_by = $2
		WHERE id = $3 AND status = $4
		RETURNING user_id
	`, models.OrganizerApproved, adminID, organizerID, models.OrganizerPending).Scan(&userID)
	if err != nil {
		return fmt.Errorf("failed to approve organizer: %w", err)
	}

	if _, err = tx.Exec(ctx, `UPDATE users SET role = $1 WHERE id = $2`,
		models.RoleOrganizer, userID); err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}

	return tx.Commit(ctx)
}

// RejectOrganizer handles any old "pending" rows still in the DB.
func RejectOrganizer(pool *pgxpool.Pool, organizerID string, adminID string, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE organizers
		SET status = $1, rejection_reason = $2, verified_by = $3
		WHERE id = $4 AND status = $5
	`, models.OrganizerRejected, reason, adminID, organizerID, models.OrganizerPending)
	if err != nil {
		return fmt.Errorf("failed to reject organizer: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrOrganizerNotFound
	}
	return nil
}

// GetPendingOrganizers returns any legacy "pending" rows still in the DB.
// Should be empty for all new accounts going forward.
func GetPendingOrganizers(pool *pgxpool.Pool) ([]models.Organizer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT o.id, o.user_id, o.business_name, o.business_email,
		       o.description, o.status, o.created_at,
		       u.email, u.first_name, u.last_name
		FROM organizers o
		JOIN users u ON o.user_id = u.id
		WHERE o.status = $1
		ORDER BY o.created_at ASC
	`, models.OrganizerPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	organizers := make([]models.Organizer, 0)
	for rows.Next() {
		var org models.Organizer
		var userEmail, userFirstName, userLastName string
		if err = rows.Scan(
			&org.ID, &org.UserID, &org.BusinessName, &org.BusinessEmail,
			&org.Description, &org.Status, &org.CreatedAt,
			&userEmail, &userFirstName, &userLastName,
		); err != nil {
			return nil, err
		}
		org.BusinessEmail = userEmail
		organizers = append(organizers, org)
	}
	return organizers, nil
}