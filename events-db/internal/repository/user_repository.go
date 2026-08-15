package repository

import (
	"context"
	"errors"
	"events-db/internal/models"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user not found")

// ErrRefreshTokenReused signals that a client presented a refresh token that
// was already rotated (revoked). This is the classic signature of a stolen
// token being replayed, so all of the user's sessions have been revoked.
var ErrRefreshTokenReused = errors.New("refresh token reuse detected")

func CreateUser(pool *pgxpool.Pool, user *models.User) (*models.User, error) {
	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		INSERT INTO users (email, password, role, first_name, last_name, phone_number)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, email, created_at
	`

	err := pool.QueryRow(ctx, query,
		user.Email, user.Password, user.Role,
		user.FirstName, user.LastName, user.PhoneNumber,
	).Scan(
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

	// deleted_at IS NULL is belt-and-braces. Deletion already overwrites the
	// email with a 'deleted-<uuid>@deleted.invalid' address, so no real address
	// can match a deleted row — but this is the single query every login goes
	// through, and it should not be relying on a side effect of the anonymiser
	// to keep deleted accounts out. If a future migration ever preserves the
	// original address, this predicate is what stops that becoming a
	// login-as-deleted-user bug.
	var query string = `
		SELECT id, email, password, role, created_at
		FROM users
		WHERE email = $1
		  AND deleted_at IS NULL
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

func DeleteUserByID(pool *pgxpool.Pool, id string) error {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	query := `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`
	_, err := pool.Exec(ctx, query, userID, tokenHash, expiresAt)

	return err
}

// GetAndRevokeRefreshToken rotates a refresh token: it atomically revokes the
// presented token and returns the owning user so a fresh pair can be issued.
//
// FIX 2 (reuse detection): if the presented token exists but was ALREADY
// revoked, someone is replaying an old token — almost certainly a stolen one,
// because the legitimate client received a new token at rotation time. In
// that case we revoke every token the user holds and return
// ErrRefreshTokenReused so the handler can log it. The attacker AND the
// legitimate user are both logged out; the legitimate user simply logs in
// again, the attacker is locked out.
// rotationGraceWindow is how long after a rotation the OLD token is still
// forgiven if presented again. This exists because refresh-token rotation +
// strict reuse detection self-destructs on innocent concurrency: two browser
// tabs share one refresh token in localStorage; when the access token expires
// they both refresh, the winner rotates the token, and the loser presents the
// just-rotated one — which used to be read as theft, revoking EVERY session
// the user had (all tabs, phone, app). Within this window a rotated token is
// treated as a concurrent-tab race and granted a fresh session instead.
// Genuine replays (long after rotation, or of tokens revoked by logout,
// password reset, or a previous reuse detection) still nuke the family.
// Industry-standard leeway (Auth0 et al. use similar reuse intervals).
// 60s was too tight for a phone. The app can start a refresh, get
// backgrounded by the OS mid-flight, and only finish (or retry) minutes later
// — and a retry after a lost response necessarily re-presents the SAME token
// the server already rotated. At 60s that was read as theft and revoked every
// session the user had, on every device. Five minutes covers a backgrounded
// retry, a tunnel hiccup and a slow mobile connection without meaningfully
// helping an attacker: a stolen token is still only usable inside a five
// minute window that the legitimate client is actively rotating out from
// under it.
const rotationGraceWindow = 5 * time.Minute

func GetAndRevokeRefreshToken(pool *pgxpool.Pool, tokenHash string) (userID string, role string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Happy path: token is live → revoke it (reason: rotated) and hand back
	// the user.
	query := `
		UPDATE refresh_tokens rt
		SET revoked = TRUE,
		    revoked_at = NOW(),
		    revoked_reason = 'rotated'
		FROM users u
		WHERE rt.user_id = u.id
		AND rt.token_hash = $1
		AND rt.revoked = FALSE
		AND rt.expires_at > NOW()
		RETURNING rt.user_id, u.role
	`
	err = pool.QueryRow(ctx, query, tokenHash).Scan(&userID, &role)
	if err == nil {
		return userID, role, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}

	// No live token matched. Check whether this hash belongs to a token that
	// was already revoked — and if so, when and why.
	var (
		reusedUserID string
		reusedRole   string
		revokedAt    *time.Time
		reason       *string
	)
	checkErr := pool.QueryRow(ctx, `
		SELECT rt.user_id, u.role, rt.revoked_at, rt.revoked_reason
		FROM refresh_tokens rt
		JOIN users u ON u.id = rt.user_id
		WHERE rt.token_hash = $1
		  AND rt.revoked = TRUE
		LIMIT 1
	`, tokenHash).Scan(&reusedUserID, &reusedRole, &revokedAt, &reason)

	if checkErr == nil {
		// Innocent concurrent-refresh race: the token was rotated moments
		// ago. Grant this caller its own fresh session (the handler mints a
		// new refresh token); the presented token stays revoked.
		if reason != nil && *reason == "rotated" &&
			revokedAt != nil && time.Since(*revokedAt) <= rotationGraceWindow {
			return reusedUserID, reusedRole, nil
		}

		// Anything else — a rotated token replayed long after, or a token
		// killed by logout/password-reset/previous-reuse — is treated as
		// theft: nuke the whole session family.
		_ = RevokeAllUserTokens(pool, reusedUserID, "reuse_detected")
		return "", "", ErrRefreshTokenReused
	}

	// Token simply doesn't exist (or is expired) — plain invalid token.
	return "", "", err
}

// RevokeAllUserTokens revokes every refresh token the user holds, recording
// why. Reasons in use: 'logout', 'password_reset', 'reuse_detected'.
func RevokeAllUserTokens(pool *pgxpool.Pool, userID string, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		UPDATE refresh_tokens
		SET revoked = TRUE,
		    revoked_at = COALESCE(revoked_at, NOW()),
		    revoked_reason = COALESCE(revoked_reason, $2)
		WHERE user_id = $1
	`
	_, err := pool.Exec(ctx, query, userID, reason)

	return err
}

// RevokeRefreshTokenByHash revokes a single session (used by logout so that
// signing out on one device no longer kills every other device and tab).
// Scoped to the user so a stray hash can't touch anyone else's session.
// Returns true if a live token was revoked.
func RevokeRefreshTokenByHash(pool *pgxpool.Pool, userID string, tokenHash string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := pool.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked = TRUE,
		    revoked_at = NOW(),
		    revoked_reason = 'logout'
		WHERE user_id = $1
		  AND token_hash = $2
		  AND revoked = FALSE
	`, userID, tokenHash)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
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