package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreatePasswordReset(pool *pgxpool.Pool, user_id string, token_hash string, expires_at time.Time) error {
	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		INSERT INTO password_resets (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`

	_, err := pool.Exec(ctx, query, user_id, token_hash, expires_at)
	return err
}

func ResetPassword(pool *pgxpool.Pool, token_hash string, new_password_hash string) error {
	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx)

	query := `
		UPDATE users
		SET password = $2
		FROM password_resets
		WHERE users.id = password_resets.user_id
		  AND password_resets.token_hash = $1
		  AND password_resets.expires_at > NOW()
		  AND password_resets.used_at IS NULL
	`
	tag, err := tx.Exec(ctx, query, token_hash, new_password_hash)
	if err != nil {
		return  err
	}

	if tag.RowsAffected() == 0 {
		return errors.New("invalid or expired token")
	}

	_, _ = pool.Exec(ctx, `
		UPDATE password_resets
		SET used_at = NOW()
		WHERE token_hash = $1
	`, token_hash)

	return tx.Commit(ctx)
}