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

// ResetPassword atomically:
//  1. sets the new password hash (only if the token is valid, unexpired, unused)
//  2. marks the reset token as used
//  3. revokes ALL of the user's refresh tokens
//
// FIX 1 (session revocation): previously an attacker holding a stolen refresh
// token could keep minting access tokens for up to 7 days after the victim
// reset their password. Now a password reset kills every session.
//
// FIX 5 (transaction correctness): previously the used_at update ran on the
// pool (outside the transaction) before Commit, so a failed commit burned the
// token without changing the password. All three statements now run inside
// the same transaction and succeed or fail together.
func ResetPassword(pool *pgxpool.Pool, token_hash string, new_password_hash string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Update the password and capture which user it belongs to.
	var userID string
	err = tx.QueryRow(ctx, `
		UPDATE users
		SET password = $2
		FROM password_resets
		WHERE users.id = password_resets.user_id
		  AND password_resets.token_hash = $1
		  AND password_resets.expires_at > NOW()
		  AND password_resets.used_at IS NULL
		RETURNING users.id
	`, token_hash, new_password_hash).Scan(&userID)
	if err != nil {
		// pgx.ErrNoRows lands here too — invalid, expired, or already-used token.
		return errors.New("invalid or expired token")
	}

	// 2. Burn the reset token (single use).
	if _, err := tx.Exec(ctx, `
		UPDATE password_resets
		SET used_at = NOW()
		WHERE token_hash = $1
	`, token_hash); err != nil {
		return err
	}

	// 3. Revoke every refresh token the user holds, on every device.
	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked = TRUE
		WHERE user_id = $1
		  AND revoked = FALSE
	`, userID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}