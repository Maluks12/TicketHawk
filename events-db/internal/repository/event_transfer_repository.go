package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Admin event handover (Path 4)
//
// Admins physically enter events for organisers who aren't on the platform
// yet, then hand ownership over. This file provides the two DB primitives:
//
//   1. EnsureOrganizerAccount — resolve an email to a ready-to-own organiser
//      account, creating and/or elevating it as needed. The admin performing
//      the handover IS the vetting step, so the account is auto-approved.
//
//   2. TransferEventOwnership — repoint events.organizer_id. Handover marks
//      the listing fee as settled (admin-promoted standing): the platform
//      entered this event, so the new owner is never billed a listing fee
//      for it.
// ─────────────────────────────────────────────────────────────────────────────

// EventOwnershipInfo is the minimal event projection a handover needs.
type EventOwnershipInfo struct {
	ID          string
	Title       string
	OrganizerID string
}

// GetEventOwnership loads just enough of an event to transfer it.
func GetEventOwnership(pool *pgxpool.Pool, eventID string) (*EventOwnershipInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var info EventOwnershipInfo
	err := pool.QueryRow(ctx, `
		SELECT id, title, organizer_id
		FROM events
		WHERE id = $1
	`, eventID).Scan(&info.ID, &info.Title, &info.OrganizerID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load event for transfer: %w", err)
	}
	return &info, nil
}

// OrganizerAccountResult reports what EnsureOrganizerAccount did.
type OrganizerAccountResult struct {
	UserID     string
	Email      string
	NewAccount bool // true when the user record was created by this call
}

// EnsureOrganizerAccount resolves an email address to a user who is ready to
// own events, atomically:
//
//   - If no user exists for the email, one is created with the organizer role
//     and the supplied (already-hashed) placeholder password. The caller is
//     expected to issue a password-reset token so the organiser sets their own
//     password from the handover email — the placeholder is random and never
//     disclosed, so the account is unusable until they do.
//   - If the user exists as an attendee, their role is elevated to organizer.
//     Admins are left untouched (never downgraded).
//   - An organizers profile row is upserted as approved + verified. The admin
//     initiating the handover is the approval, so there is no pending queue.
//     An existing profile keeps its business details; only its status is
//     normalised to approved.
//
// businessName seeds the profile for brand-new organisers; existing profiles
// are never renamed by a handover.
func EnsureOrganizerAccount(pool *pgxpool.Pool, email, businessName, placeholderPasswordHash string) (*OrganizerAccountResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	result := &OrganizerAccountResult{Email: email}

	// 1. Find-or-create the user. FOR UPDATE serialises concurrent handovers
	//    to the same email.
	var role models.UserRole
	err = tx.QueryRow(ctx, `
		SELECT id, role FROM users WHERE email = $1 FOR UPDATE
	`, email).Scan(&result.UserID, &role)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Invite-on-handover: create the account now; the handover email
		// carries the set-password link.
		err = tx.QueryRow(ctx, `
			INSERT INTO users (email, password, role)
			VALUES ($1, $2, $3)
			RETURNING id
		`, email, placeholderPasswordHash, models.RoleOrganizer).Scan(&result.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to create organizer account: %w", err)
		}
		result.NewAccount = true

	case err != nil:
		return nil, fmt.Errorf("failed to look up user: %w", err)

	default:
		// Existing user: elevate attendees, never touch admins.
		if role == models.RoleAttendee {
			if _, err := tx.Exec(ctx, `
				UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2
			`, models.RoleOrganizer, result.UserID); err != nil {
				return nil, fmt.Errorf("failed to elevate user role: %w", err)
			}
		}
	}

	// 2. Upsert the organizers profile as approved + verified. The handover
	//    is the approval. business_phone is NOT NULL — a new invite starts
	//    with an empty placeholder they complete from the dashboard later.
	if _, err := tx.Exec(ctx, `
		INSERT INTO organizers (
			user_id, business_name, business_email, business_phone,
			status, verified, verified_at
		)
		VALUES ($1, $2, $3, '', $4, TRUE, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) DO UPDATE
		SET status      = $4,
		    verified    = TRUE,
		    verified_at = COALESCE(organizers.verified_at, CURRENT_TIMESTAMP),
		    updated_at  = CURRENT_TIMESTAMP
	`, result.UserID, businessName, email, models.OrganizerApproved); err != nil {
		return nil, fmt.Errorf("failed to upsert organizer profile: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit organizer account: %w", err)
	}
	return result, nil
}

// TransferEventOwnership repoints an event at a new owner. Their dashboard,
// revenue attribution, and scanning rights all follow organizer_id, so this
// single UPDATE is the whole transfer; event_staff rows attach to the event
// (not the owner) and persist across it.
//
// listing_fee_paid is forced TRUE: an admin handover carries admin-promoted
// standing, so the receiving organiser is never billed a listing fee for an
// event the platform entered on their behalf.
func TransferEventOwnership(pool *pgxpool.Pool, eventID, newOwnerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := pool.Exec(ctx, `
		UPDATE events
		SET organizer_id     = $1,
		    listing_fee_paid = TRUE,
		    updated_at       = NOW()
		WHERE id = $2
	`, newOwnerID, eventID)
	if err != nil {
		return fmt.Errorf("failed to transfer event ownership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}