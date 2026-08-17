package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEmailBelongsToDeletedAccount is returned when a provider identity resolves
// to an anonymised account. Signing in must not resurrect it.
var ErrEmailBelongsToDeletedAccount = errors.New("account has been deleted")

// SocialIdentityInput is the verified data from an ID token. Every field here
// has already been through OIDCVerifier — nothing user-supplied reaches this
// layer.
type SocialIdentityInput struct {
	Provider       string
	Subject        string
	Email          string
	EmailVerified  bool
	IsPrivateRelay bool
	FirstName      string
	LastName       string
}

// SocialSignInResult reports what happened, so the handler can tell the client
// whether to route to "welcome" or straight into the app.
type SocialSignInResult struct {
	User      *models.User
	Created   bool // a brand-new account was provisioned
	Linked    bool // an existing password account gained this provider
	NeedsInfo bool // phone number or DOB still missing (collected at checkout)
}

// ResolveSocialIdentity is the whole sign-in decision, in one transaction.
//
// ── The three cases, in strict order ─────────────────────────────────────────
//
//  1. (provider, subject) already known    → sign in. Nothing else is consulted;
//     the subject is the provider's permanent
//     handle for this person.
//
//  2. Email matches a live account         → LINK the identity to it. This is
//     the auto-link path (see below).
//
//  3. Neither                              → provision a new account.
//
// ── Why auto-linking on email is safe HERE and not in general ────────────────
//
// Auto-linking is a real account-takeover vector, and the reason it is
// acceptable in this specific shape is narrow: we only do it when the PROVIDER
// asserts email_verified. Google and Apple both control the mailbox they are
// vouching for — Google because it is a Google account, Apple because it either
// verified the address at signup or minted the relay itself. So "Google says
// this person controls lisan@example.com" is at least as strong a proof as
// "this person can read a password-reset link sent to lisan@example.com",
// which is the standard everything else in this codebase already relies on.
//
// The rule is enforced below: an UNVERIFIED provider email never links and
// never matches. It provisions a separate account instead, which is the safe
// failure — an extra account is a support ticket, a wrong link is a breach.
func ResolveSocialIdentity(pool *pgxpool.Pool, in SocialIdentityInput) (*SocialSignInResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("ResolveSocialIdentity/begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	email := strings.ToLower(strings.TrimSpace(in.Email))

	// ── Case 1: known identity ───────────────────────────────────────────────
	var userID string
	err = tx.QueryRow(ctx, `
		SELECT user_id FROM user_identities
		WHERE provider = $1 AND provider_subject = $2
	`, in.Provider, in.Subject).Scan(&userID)

	switch {
	case err == nil:
		user, derr := loadUserForSignIn(ctx, tx, userID)
		if derr != nil {
			return nil, derr
		}
		// Refresh what the provider told us this time. Apple in particular can
		// flip a relay address on and off.
		if _, uerr := tx.Exec(ctx, `
			UPDATE user_identities
			SET email = COALESCE(NULLIF($3, ''), email),
			    email_verified = $4,
			    is_private_relay = $5,
			    last_login_at = NOW()
			WHERE provider = $1 AND provider_subject = $2
		`, in.Provider, in.Subject, email, in.EmailVerified, in.IsPrivateRelay); uerr != nil {
			return nil, fmt.Errorf("ResolveSocialIdentity/touch: %w", uerr)
		}
		if cerr := tx.Commit(ctx); cerr != nil {
			return nil, cerr
		}
		return &SocialSignInResult{User: user, NeedsInfo: missingCheckoutInfo(user)}, nil

	case !errors.Is(err, pgx.ErrNoRows):
		return nil, fmt.Errorf("ResolveSocialIdentity/lookup: %w", err)
	}

	// ── Case 2: link to an existing account by verified email ────────────────
	//
	// The email_verified gate is the entire safety argument. Do not remove it.
	if email != "" && in.EmailVerified {
		var (
			existingID      string
			existingDeleted bool
		)
		err = tx.QueryRow(ctx, `
			SELECT id, deleted_at IS NOT NULL FROM users WHERE LOWER(email) = $1
		`, email).Scan(&existingID, &existingDeleted)

		switch {
		case err == nil:
			if existingDeleted {
				// 000065 anonymises rather than deletes, and frees the address
				// for reuse. A deleted row must never be signed back into.
				return nil, ErrEmailBelongsToDeletedAccount
			}
			if _, ierr := tx.Exec(ctx, `
				INSERT INTO user_identities
					(user_id, provider, provider_subject, email, email_verified, is_private_relay, last_login_at)
				VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, NOW())
				ON CONFLICT (provider, provider_subject) DO NOTHING
			`, existingID, in.Provider, in.Subject, email, in.EmailVerified, in.IsPrivateRelay); ierr != nil {
				return nil, fmt.Errorf("ResolveSocialIdentity/link: %w", ierr)
			}

			// Signing in with a provider that vouches for the address is proof
			// of control over it, so an unverified account becomes verified.
			if _, verr := tx.Exec(ctx,
				`UPDATE users SET email_verified = TRUE, updated_at = NOW() WHERE id = $1 AND email_verified = FALSE`,
				existingID,
			); verr != nil {
				return nil, fmt.Errorf("ResolveSocialIdentity/verify: %w", verr)
			}

			user, derr := loadUserForSignIn(ctx, tx, existingID)
			if derr != nil {
				return nil, derr
			}
			if cerr := tx.Commit(ctx); cerr != nil {
				return nil, cerr
			}
			return &SocialSignInResult{User: user, Linked: true, NeedsInfo: missingCheckoutInfo(user)}, nil

		case !errors.Is(err, pgx.ErrNoRows):
			return nil, fmt.Errorf("ResolveSocialIdentity/email: %w", err)
		}
	}

	// ── Case 3: provision ────────────────────────────────────────────────────
	//
	// password is left NULL (see 000075). phone_number and date_of_birth are
	// left NULL too — checkout collects them.
	//
	// A social account's email is marked verified ONLY if the provider said so.
	// An Apple relay address is verified in the sense that mail routes to the
	// user, so it counts.
	if email == "" {
		// Cannot provision without an address: ticket delivery is email-based
		// and users.email is NOT NULL UNIQUE.
		return nil, errors.New("provider returned no email address")
	}

	var newID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password, first_name, last_name, role, email_verified, signup_method)
		VALUES ($1, NULL, NULLIF($2, ''), NULLIF($3, ''), 'attendee', $4, $5)
		RETURNING id
	`, email, in.FirstName, in.LastName, in.EmailVerified, in.Provider).Scan(&newID)
	if err != nil {
		// A concurrent first sign-in for the same address. Rare, but two taps
		// on a slow connection will do it.
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return nil, fmt.Errorf("ResolveSocialIdentity/race: %w", err)
		}
		return nil, fmt.Errorf("ResolveSocialIdentity/create: %w", err)
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO user_identities
			(user_id, provider, provider_subject, email, email_verified, is_private_relay, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`, newID, in.Provider, in.Subject, email, in.EmailVerified, in.IsPrivateRelay); err != nil {
		return nil, fmt.Errorf("ResolveSocialIdentity/identity: %w", err)
	}

	user, err := loadUserForSignIn(ctx, tx, newID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &SocialSignInResult{User: user, Created: true, NeedsInfo: true}, nil
}

func loadUserForSignIn(ctx context.Context, tx pgx.Tx, userID string) (*models.User, error) {
	var (
		u         models.User
		deletedAt *time.Time
	)
	err := tx.QueryRow(ctx, `
		SELECT id, email, first_name, last_name, phone_number, date_of_birth,
		       role, email_verified, created_at, updated_at, deleted_at
		FROM users WHERE id = $1
	`, userID).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.PhoneNumber, &u.DateOfBirth,
		&u.Role, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt, &deletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("loadUserForSignIn: %w", err)
	}
	if deletedAt != nil {
		return nil, ErrEmailBelongsToDeletedAccount
	}
	return &u, nil
}

// missingCheckoutInfo reports whether the fields deferred to checkout are still
// blank. Social accounts start this way by design.
func missingCheckoutInfo(u *models.User) bool {
	if u.PhoneNumber == nil || strings.TrimSpace(*u.PhoneNumber) == "" {
		return true
	}
	return u.DateOfBirth == nil
}

// ListUserIdentities powers the profile screen's "connected accounts" section.
func ListUserIdentities(pool *pgxpool.Pool, userID string) ([]models.UserIdentity, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT provider, email, created_at, last_login_at
		FROM user_identities WHERE user_id = $1 ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.UserIdentity{}
	for rows.Next() {
		var it models.UserIdentity
		if err := rows.Scan(&it.Provider, &it.Email, &it.CreatedAt, &it.LastLoginAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UnlinkIdentity removes a provider from an account.
//
// Guarded so somebody cannot lock themselves out: an account with no password
// must keep at least one identity, or nothing can ever sign into it again.
func UnlinkIdentity(pool *pgxpool.Pool, userID, provider string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var hasPassword bool
	var identityCount int
	if err := tx.QueryRow(ctx, `
		SELECT u.password IS NOT NULL,
		       (SELECT COUNT(*) FROM user_identities WHERE user_id = u.id)
		FROM users u WHERE u.id = $1
	`, userID).Scan(&hasPassword, &identityCount); err != nil {
		return err
	}

	if !hasPassword && identityCount <= 1 {
		return errors.New("set a password before removing your last sign-in method")
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM user_identities WHERE user_id = $1 AND provider = $2`,
		userID, provider,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// IdentityBelongsToUser reports whether (provider, subject) is linked to this
// specific account.
//
// Used by step-up re-authentication. Without it, presenting ANY valid Google
// token would satisfy the gate on email change and account deletion — the
// token would verify fine, it just wouldn't be the right person's.
func IdentityBelongsToUser(pool *pgxpool.Pool, userID, provider, subject string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_identities
			WHERE user_id = $1 AND provider = $2 AND provider_subject = $3
		)
	`, userID, provider, subject).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("IdentityBelongsToUser: %w", err)
	}
	return exists, nil
}