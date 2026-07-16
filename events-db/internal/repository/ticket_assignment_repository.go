package repository

import (
	"context"
	"errors"
	"events-db/internal/models"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAssignmentNotFound    = errors.New("ticket assignment not found")
var ErrAlreadyClaimed        = errors.New("ticket has already been claimed")
var ErrAssignmentNotPending  = errors.New("ticket is not in a pending state")
var ErrClaimTokenExpired     = errors.New("claim link has expired")
var ErrClaimEmailMismatch    = errors.New("this ticket was assigned to a different email address")
var ErrNotAssignmentOwner    = errors.New("you do not own this booking")
var ErrAlreadyAdmitted       = errors.New("ticket has already been admitted at the gate")

const assignmentRevocationWindow = 24 * time.Hour

// CreateTicketAssignments inserts one row per ticket in the booking.
// Slot 1 is pre-claimed by the buyer; all others start as unassigned.
// Called by the booking worker immediately after payment is confirmed.
func CreateTicketAssignments(
	pool *pgxpool.Pool,
	bookingID string,
	quantity int,
	buyerName string,
	buyerEmail string,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Pool model: every ticket starts UNASSIGNED, forming the buyer's group
	// pool. The buyer admits themselves and anyone with them straight from the
	// pool (their group QR), and assigns a ticket only to peel one off for
	// someone arriving separately. No slot is auto-claimed. (buyerName/buyerEmail
	// are retained on the signature for callers but no longer used here.)
	_ = buyerName
	_ = buyerEmail

	for slot := 1; slot <= quantity; slot++ {
		_, err = tx.Exec(ctx, `
			INSERT INTO ticket_assignments
				(booking_id, slot_number, status)
			VALUES ($1, $2, 'unassigned')
			ON CONFLICT (booking_id, slot_number) DO NOTHING
		`, bookingID, slot)

		if err != nil {
			return fmt.Errorf("insert slot %d: %w", slot, err)
		}
	}

	return tx.Commit(ctx)
}

// GetAssignmentsByBooking returns all assignment rows for a booking,
// verifying the booking belongs to the requesting user.
func GetAssignmentsByBooking(
	pool *pgxpool.Pool,
	bookingID string,
	userID string,
) ([]models.TicketAssignment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify ownership first.
	var count int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(1) FROM bookings WHERE id = $1 AND user_id = $2`,
		bookingID, userID,
	).Scan(&count)
	if err != nil || count == 0 {
		return nil, ErrNotAssignmentOwner
	}

	rows, err := pool.Query(ctx, `
		SELECT id, booking_id, slot_number, status,
		       assignee_name, assignee_email,
		       assigned_at, expires_at, claimed_at, revoked_at,
		       qr_scanned_at,
		       qr_code_url, created_at, updated_at
		FROM ticket_assignments
		WHERE booking_id = $1
		ORDER BY slot_number
	`, bookingID)
	if err != nil {
		return nil, fmt.Errorf("query assignments: %w", err)
	}
	defer rows.Close()

	var assignments []models.TicketAssignment
	for rows.Next() {
		var a models.TicketAssignment
		if err := rows.Scan(
			&a.ID, &a.BookingID, &a.SlotNumber, &a.Status,
			&a.AssigneeName, &a.AssigneeEmail,
			&a.AssignedAt, &a.ExpiresAt, &a.ClaimedAt, &a.RevokedAt,
			&a.QRScannedAt,
			&a.QRCodeURL, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}

// AssignTicket sets a slot to pending, generates a claim token, and sets expiry.
// Returns the claim token so the caller can email it.
func AssignTicket(
	pool *pgxpool.Pool,
	assignmentID string,
	bookingID string,
	userID string,
	dto *models.AssignTicketDTO,
) (claimToken string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ownership check + fetch current status (and whether it's already been
	// admitted at the gate) in one query.
	var status models.AssignmentStatus
	var ownerCount int
	var scannedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT ta.status, ta.qr_scanned_at,
		       (SELECT COUNT(1) FROM bookings WHERE id = ta.booking_id AND user_id = $3)
		FROM ticket_assignments ta
		WHERE ta.id = $1 AND ta.booking_id = $2
	`, assignmentID, bookingID, userID).Scan(&status, &scannedAt, &ownerCount)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAssignmentNotFound
	}
	if err != nil {
		return "", fmt.Errorf("fetch assignment: %w", err)
	}
	if ownerCount == 0 {
		return "", ErrNotAssignmentOwner
	}
	if status == models.AssignmentClaimed {
		return "", ErrAlreadyClaimed
	}
	// Can't peel a ticket out of the pool if it's already walked someone in.
	if scannedAt != nil {
		return "", ErrAlreadyAdmitted
	}

	now := time.Now()
	expiresAt := now.Add(assignmentRevocationWindow)

	var newToken string
	err = pool.QueryRow(ctx, `
		UPDATE ticket_assignments
		SET status         = $1,
		    assignee_name  = $2,
		    assignee_email = $3,
		    claim_token    = gen_random_uuid(),
		    assigned_at    = $4,
		    expires_at     = $5,
		    revoked_at     = NULL,
		    claimed_at     = NULL
		WHERE id = $6
		RETURNING claim_token
	`,
		models.AssignmentPending,
		dto.AssigneeName,
		dto.AssigneeEmail,
		now,
		expiresAt,
		assignmentID,
	).Scan(&newToken)

	if err != nil {
		return "", fmt.Errorf("assign ticket: %w", err)
	}
	return newToken, nil
}

// RevokeAssignment resets a pending assignment back to unassigned.
// Only the booking owner can revoke, and only while the ticket is still unclaimed.
func RevokeAssignment(
	pool *pgxpool.Pool,
	assignmentID string,
	bookingID string,
	userID string,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE ticket_assignments ta
		SET status         = $1,
		    assignee_name  = NULL,
		    assignee_email = NULL,
		    claim_token    = NULL,
		    assigned_at    = NULL,
		    expires_at     = NULL,
		    revoked_at     = NOW()
		FROM bookings b
		WHERE ta.id         = $2
		  AND ta.booking_id = $3
		  AND b.id          = ta.booking_id
		  AND b.user_id     = $4
		  AND ta.status     = $5
	`,
		models.AssignmentUnassigned,
		assignmentID,
		bookingID,
		userID,
		models.AssignmentPending,
	)
	if err != nil {
		return fmt.Errorf("revoke: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrAssignmentNotPending
	}
	return nil
}

// ClaimTicket is the public endpoint — the recipient clicks the link in their email.
// Returns the assignment so the handler can confirm details to the claimant.
func ClaimTicket(
	pool *pgxpool.Pool,
	claimToken string,
	claimantName string,
	claimantEmail string,
) (*models.TicketAssignment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var a models.TicketAssignment
	err = tx.QueryRow(ctx, `
		SELECT id, booking_id, slot_number, status, expires_at, assignee_email
		FROM ticket_assignments
		WHERE claim_token = $1
		FOR UPDATE
	`, claimToken).Scan(
		&a.ID, &a.BookingID, &a.SlotNumber, &a.Status,
		&a.ExpiresAt, &a.AssigneeEmail,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAssignmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("fetch for claim: %w", err)
	}
	if a.Status != models.AssignmentPending {
		return nil, ErrAssignmentNotPending
	}
	if a.ExpiresAt != nil && time.Now().After(*a.ExpiresAt) {
		return nil, ErrClaimTokenExpired
	}

	// The claim token proves the person has the invite LINK, but links get
	// forwarded and inboxes get shared — the claimant must also confirm the
	// email address the invitation was actually sent to. Without this check
	// anyone holding the link could claim the ticket under any identity.
	// (Nil/empty stored email would mean a legacy row assigned before the
	// email was recorded; only then do we accept the claimant's input as-is.)
	if a.AssigneeEmail != nil && *a.AssigneeEmail != "" &&
		!strings.EqualFold(strings.TrimSpace(*a.AssigneeEmail), strings.TrimSpace(claimantEmail)) {
		return nil, ErrClaimEmailMismatch
	}

	now := time.Now()
	err = tx.QueryRow(ctx, `
		UPDATE ticket_assignments
		SET status         = $1,
		    assignee_name  = $2,
		    assignee_email = $3,
		    claimed_at     = $4,
		    claim_token    = NULL,
		    expires_at     = NULL
		WHERE id = $5
		RETURNING id, booking_id, slot_number, status, assignee_name, assignee_email,
		          claimed_at, qr_code_url, created_at, updated_at
	`,
		models.AssignmentClaimed,
		claimantName,
		claimantEmail,
		now,
		a.ID,
	).Scan(
		&a.ID, &a.BookingID, &a.SlotNumber, &a.Status,
		&a.AssigneeName, &a.AssigneeEmail,
		&a.ClaimedAt, &a.QRCodeURL, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("claim update: %w", err)
	}

	return &a, tx.Commit(ctx)
}

// GetAssignmentForQR fetches the raw qr_code_data for a claimed ticket.
// The claimant must match the stored email.
func GetAssignmentForQR(
	pool *pgxpool.Pool,
	assignmentID string,
	userEmail string,
) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var qrData *string
	var status models.AssignmentStatus
	err := pool.QueryRow(ctx, `
		SELECT qr_code_data, status
		FROM ticket_assignments
		WHERE id = $1 AND lower(assignee_email) = lower($2)
	`, assignmentID, userEmail).Scan(&qrData, &status)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAssignmentNotFound
	}
	if err != nil {
		return "", err
	}
	if status != models.AssignmentClaimed {
		return "", ErrAssignmentNotPending
	}
	if qrData == nil {
		return "", nil
	}
	return *qrData, nil
}

// ExpirePendingAssignments resets any pending assignment rows whose 24-hour
// window has closed without being claimed. Called from the expiry worker.
func ExpirePendingAssignments(pool *pgxpool.Pool, ctx context.Context) error {
	cmd, err := pool.Exec(ctx, `
		UPDATE ticket_assignments
		SET status         = 'unassigned',
		    assignee_name  = NULL,
		    assignee_email = NULL,
		    claim_token    = NULL,
		    assigned_at    = NULL,
		    expires_at     = NULL
		WHERE status     = 'pending'
		  AND expires_at < NOW()
	`)
	if err != nil {
		return fmt.Errorf("expire assignments: %w", err)
	}
	if cmd.RowsAffected() > 0 {
		fmt.Printf("expiry worker: reset %d expired pending assignments\n", cmd.RowsAffected())
	}
	return nil
}