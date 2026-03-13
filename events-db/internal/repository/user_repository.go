package repository

import (
	"context"
	"errors"
	"events-db/internal/models"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user not found")

func CreateUser(pool *pgxpool.Pool, user *models.User) (*models.User, error) {
	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query:= `
		INSERT INTO users (email, password)
		Values ($1, $2)
		RETURNING id, email, created_at
	`

	err := pool.QueryRow(ctx, query, user.Email, user.Password).Scan(
		&user.ID,
		&user.Email,
		&user.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return user, nil
}

func GetUserByEmail(pool *pgxpool.Pool, email string) (*models.User, error) {
	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var query string = `
		SELECT id, email, password, role, created_at
		FROM users
		WHERE email = $1
	`

	var user models.User

	err := pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Password,
		&user.Role,
		&user.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func GetUserByID(pool *pgxpool.Pool, id string) (*models.User, error) {
	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var query string = `
		SELECT id, email, password, created_at
		FROM users
		WHERE id = $1
	`

	var user models.User

	err := pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.Password,
		&user.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func DeleteUserByID(pool *pgxpool.Pool, id string)  error {
	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var query string = `
		DELETE FROM users
		WHERE id = $1
	`

	commandTag, err := pool.Exec(ctx, query, id)

	if err != nil {
		return err
	}

	if commandTag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func CreateRefreshToken(pool *pgxpool.Pool, userID string, tokenHash string, expiresAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	query := `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`
	_, err := pool.Exec(ctx, query, userID, tokenHash, expiresAt)

	return err
}

func GetAndRevokeRefreshToken(pool *pgxpool.Pool, tokenHash string) (userID string, role string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	query := `
		UPDATE refresh_tokens rt
		SET revoked = TRUE
		FROM users u
		WHERE rt.user_id = u.id
		AND rt.token_hash = $1
		AND rt.revoked = FALSE
		AND rt.expires_at > NOW()
		RETURNING rt.user_id, u.role
	`
	err =  pool.QueryRow(ctx, query, tokenHash).Scan(&userID, &role)
	return
}

func RevokeAllUserTokens(pool *pgxpool.Pool, userID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	query := `
		UPDATE refresh_tokens SET revoked = TRUE WHERE user_id = $1
	`
	_, err := pool.Exec(ctx, query, userID)

	return err
}

func GetUserByIDForEmail(pool *pgxpool.Pool, id string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
 
	query := `
		SELECT id, email, first_name, last_name, role, created_at
		FROM users
		WHERE id = $1
	`
 
	var user models.User
	err := pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.FirstName,
		&user.LastName,
		&user.Role,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetUserByIDForEmail: %w", err)
	}
	return &user, nil
}
 