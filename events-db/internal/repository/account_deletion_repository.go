package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Account deletion
//
// See migrations/000065 for why this anonymises in place rather than issuing a
// DELETE. The short version: bookings.user_id and events.organizer_id are both
// ON DELETE RESTRICT, and those rows are financial records that must outlive
// the account.
// ─────────────────────────────────────────────────────────────────────────────

// AccountDeletionBlockers describes the things standing between a user and a
// clean deletion. Nothing here is a hard "no" from the platform's point of view
// — Apple requires deletion to be initiable in-app — but two of them are worth
// stopping to explain before the account goes.
type AccountDeletionBlockers struct {
	// UpcomingTickets: confirmed bookings for events that have not yet ended.
	// These are not a blocker so much as something the user must be told they
	// are throwing away, since the tickets become unreachable afterwards.
	UpcomingTickets int     `json:"upcoming_tickets"`
	UpcomingValue   float64 `json:"upcoming_value"`

	// LiveEvents: events this user is the ORGANISER of that are published and
	// have not yet ended. This one is a genuine hold. Other people hold tickets
	// to those events; anonymising the organiser behind them would leave
	// ticket-holders with an event nobody is accountable for.
	LiveEvents int `json:"live_events"`

	// UnsettledPayouts: money the platform still owes this organiser, or is
	// still trying to pay them. Deleting the account mid-settlement would strand
	// the funds.
	UnsettledPayouts int `json:"unsettled_payouts"`
}

// HasOrganiserHold reports whether deletion must be refused outright rather
// than merely warned about.
func (b AccountDeletionBlockers) HasOrganiserHold() bool {
	return b.LiveEvents > 0 || b.UnsettledPayouts > 0
}

// GetAccountDeletionBlockers inspects everything that deletion would disturb.
//
// Deliberately three narrow queries rather than one join: they touch unrelated
// tables with different indexes, and a user with no organiser row (the
// overwhelming majority) short-circuits the last two entirely.
func GetAccountDeletionBlockers(ctx context.Context, pool *pgxpool.Pool, userID string) (AccountDeletionBlockers, error) {
	var b AccountDeletionBlockers

	// Upcoming confirmed tickets. COALESCE because SUM over zero rows is NULL.
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(bk.total_amount), 0)
		FROM bookings bk
		JOIN events ev ON ev.id = bk.event_id
		WHERE bk.user_id = $1
		  AND bk.booking_status = 'confirmed'
		  AND COALESCE(ev.end_time, ev.start_time + INTERVAL '4 hours') > NOW()
	`, userID).Scan(&b.UpcomingTickets, &b.UpcomingValue)
	if err != nil {
		return b, fmt.Errorf("GetAccountDeletionBlockers/tickets: %w", err)
	}

	// Events they run that are still live.
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM events
		WHERE organizer_id = $1
		  AND status IN ('published', 'published_private')
		  AND COALESCE(end_time, start_time + INTERVAL '4 hours') > NOW()
	`, userID).Scan(&b.LiveEvents)
	if err != nil {
		return b, fmt.Errorf("GetAccountDeletionBlockers/events: %w", err)
	}

	// Money still in flight. Joined through organizers because payouts are keyed
	// on the organiser profile, not the user.
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM organizer_payouts op
		JOIN organizers o ON o.id = op.organizer_id
		WHERE o.user_id = $1
		  AND op.status IN ('pending', 'processing', 'failed')
	`, userID).Scan(&b.UnsettledPayouts)
	if err != nil {
		return b, fmt.Errorf("GetAccountDeletionBlockers/payouts: %w", err)
	}

	return b, nil
}

// SoftDeleteUser anonymises the account and tears down everything that could
// still reach the person afterwards.
//
// All of it runs in ONE transaction. A partial deletion is the worst possible
// outcome: an account whose email is destroyed (so the owner can never sign in
// or recover it) but whose push tokens survive (so their old phone keeps buzzing
// with notifications for an account that no longer exists).
//
// What is destroyed here, and why each one matters:
//
//   - users row        — every identifying column overwritten in place. The
//     password is set to a literal that is not a valid bcrypt
//     hash, so CompareHashAndPassword can only ever fail; there
//     is no "correct" password for the stub.
//   - refresh_tokens   — revoked, so no device can mint a new access token.
//   - push_tokens      — deleted, so notifications stop immediately.
//   - notifications    — deleted; the inbox is personal content.
//   - event_follows    — deleted; a follow list is a taste profile.
//   - carts            — deleted; abandoned-cart rows hold intent data.
//   - password_resets  — deleted, so an in-flight reset email cannot be used to
//     interact with the stub after the fact.
//
// What is deliberately KEPT: bookings, payments, tickets and payouts. Those are
// financial records, they are what the ON DELETE RESTRICT constraints protect,
// and after this runs they point at an anonymous id rather than a person.
//
// Note on the access token: this revokes refresh tokens, but a JWT already
// issued stays cryptographically valid until it expires. Access tokens are
// minted with a 15-minute lifetime (see LoginHandler), so the residual window is
// bounded and small. Closing it entirely would mean a database lookup on every
// authenticated request, which is a large cost to pay for 15 minutes.
func SoftDeleteUser(ctx context.Context, pool *pgxpool.Pool, userID string, reason *string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("SoftDeleteUser/begin: %w", err)
	}
	// Rollback is a no-op once the tx has committed, so this is safe to defer
	// unconditionally and is what guarantees no partial state on any early return.
	defer func() { _ = tx.Rollback(ctx) }()

	// Guard against double-deletion. Without this, a client that retries the
	// request would overwrite the already-anonymised row a second time —
	// harmless in itself, but it would move deleted_at forward and make the
	// audit trail lie about when the account actually went.
	var alreadyDeleted bool
	err = tx.QueryRow(ctx,
		`SELECT deleted_at IS NOT NULL FROM users WHERE id = $1`, userID,
	).Scan(&alreadyDeleted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("SoftDeleteUser/lookup: %w", err)
	}
	if alreadyDeleted {
		return nil // idempotent
	}

	// ── Anonymise in place ───────────────────────────────────────────────────
	// The new email is derived from the row's own UUID, which is already random
	// and carries nothing about the person. '.invalid' is reserved by RFC 2606,
	// so the address can never be registered or delivered to, and the user's
	// real address is freed up for a fresh signup immediately.
	if _, err = tx.Exec(ctx, `
		UPDATE users
		SET email                      = 'deleted-' || id::text || '@deleted.invalid',
		    -- NULL rather than a sentinel string now that 000075 allows it.
		    -- LoginHandler rejects a NULL password before bcrypt, so this is
		    -- strictly stronger than a placeholder that a future refactor
		    -- might accidentally compare against.
		    password                   = NULL,
		    first_name                 = 'Deleted',
		    last_name                  = 'User',
		    phone_number               = NULL,
		    date_of_birth              = NULL,
		    email_verified             = FALSE,
		    email_verification_token   = NULL,
		    email_verification_expires  = NULL,
		    deleted_at                 = NOW(),
		    deletion_reason            = $2,
		    updated_at                 = NOW()
		WHERE id = $1
	`, userID, reason); err != nil {
		return fmt.Errorf("SoftDeleteUser/anonymise: %w", err)
	}

	// ── Kill every live session ──────────────────────────────────────────────
	if _, err = tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked        = TRUE,
		    revoked_at     = COALESCE(revoked_at, NOW()),
		    revoked_reason = COALESCE(revoked_reason, 'account_deleted')
		WHERE user_id = $1
	`, userID); err != nil {
		return fmt.Errorf("SoftDeleteUser/tokens: %w", err)
	}

	// ── Stop anything that could still reach them ────────────────────────────
	// Ordered least-to-most likely to fail on a schema drift; each is scoped to
	// the one user. Tables are removed rather than anonymised because none of
	// them is referenced by a financial record.
	for _, stmt := range []string{
		// Social identities MUST go, and this is load-bearing rather than
		// hygiene. The row is anonymised, not deleted, so it survives with a
		// deleted-<uuid>@deleted.invalid address — and (provider, subject) on
		// user_identities would still point straight at it. Leaving them means
		// tapping "Sign in with Google" after deleting your account walks you
		// back into the anonymised stub, with the deletion silently undone.
		`DELETE FROM user_identities WHERE user_id = $1`,
		`DELETE FROM push_tokens     WHERE user_id = $1`,
		`DELETE FROM notifications   WHERE user_id = $1`,
		`DELETE FROM event_follows   WHERE user_id = $1`,
		`DELETE FROM password_resets WHERE user_id = $1`,
		// cart_items cascade from carts, so the parent row is enough.
		`DELETE FROM carts           WHERE user_id = $1`,
	} {
		if _, err = tx.Exec(ctx, stmt, userID); err != nil {
			return fmt.Errorf("SoftDeleteUser/purge (%s): %w", stmt, err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("SoftDeleteUser/commit: %w", err)
	}
	return nil
}