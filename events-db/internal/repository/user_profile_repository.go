package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUserNotFound is returned when an update targets a user id that no longer
// exists — a valid token for a deleted account, essentially.
//var ErrUserNotFound = errors.New("user not found")

// ProfileUpdate is a sparse patch: a nil field means "leave it alone", which is
// what lets the client submit only what changed.
type ProfileUpdate struct {
	FirstName   *string
	LastName    *string
	PhoneNumber *string
	Email       *string
}

func (p ProfileUpdate) IsEmpty() bool {
	return p.FirstName == nil && p.LastName == nil && p.PhoneNumber == nil && p.Email == nil
}

// EmailTakenByAnotherUser reports whether an email belongs to somebody else.
//
// Checked before the UPDATE rather than relying on the unique index, because a
// constraint violation surfaces as an opaque 500 and the user deserves to be
// told plainly that the address is in use.
func EmailTakenByAnotherUser(pool *pgxpool.Pool, ctx context.Context, email, userID string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users
			WHERE LOWER(email) = LOWER($1) AND id <> $2
		)
	`, email, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("EmailTakenByAnotherUser: %w", err)
	}
	return exists, nil
}

// GetPasswordHash returns the stored bcrypt hash for re-authentication.
//
// Deliberately narrow: it fetches the hash and nothing else, so a caller that
// only needs to verify a password never pulls a whole user row into memory and
// cannot accidentally serialise the hash into a response.
func GetPasswordHash(pool *pgxpool.Pool, ctx context.Context, userID string) (string, error) {
	var hash string
	err := pool.QueryRow(ctx, `SELECT password FROM users WHERE id = $1`, userID).Scan(&hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrUserNotFound
		}
		return "", fmt.Errorf("GetPasswordHash: %w", err)
	}
	return hash, nil
}

// UpdateUserProfile applies a sparse patch and returns the updated row.
//
// The SQL is built dynamically so that untouched columns are never written —
// a blanket UPDATE with COALESCE would still bump updated_at on every column
// and, worse, would silently clear a field the client omitted.
func UpdateUserProfile(pool *pgxpool.Pool, ctx context.Context, userID string, upd ProfileUpdate) (*models.User, error) {
	sets := []string{}
	args := []any{}
	n := 1

	add := func(col string, val any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, n))
		args = append(args, val)
		n++
	}

	if upd.FirstName != nil {
		add("first_name", *upd.FirstName)
	}
	if upd.LastName != nil {
		add("last_name", *upd.LastName)
	}
	if upd.PhoneNumber != nil {
		add("phone_number", *upd.PhoneNumber)
	}
	if upd.Email != nil {
		add("email", *upd.Email)
		// Changing the login address invalidates the previous verification.
		// Leaving email_verified true here would let someone move an account to
		// an address they have not proven they control, while everything
		// downstream still treats it as confirmed.
		sets = append(sets, "email_verified = FALSE")
	}

	if len(sets) == 0 {
		return nil, errors.New("UpdateUserProfile: nothing to update")
	}

	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, userID)

	query := fmt.Sprintf(`
		UPDATE users
		SET %s
		WHERE id = $%d
		RETURNING id, email, first_name, last_name, phone_number,
		          date_of_birth, role, email_verified, created_at, updated_at
	`, strings.Join(sets, ", "), n)

	var u models.User
	err := pool.QueryRow(ctx, query, args...).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.PhoneNumber,
		&u.DateOfBirth, &u.Role, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("UpdateUserProfile: %w", err)
	}
	return &u, nil
}