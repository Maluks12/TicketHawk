package repository

import (
	"context"
	"fmt"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GetAssignmentsByBookingID returns all ticket assignment slots for a booking
// by booking ID alone, with no user_id ownership check.
//
// IMPORTANT: The caller is responsible for verifying the requester's identity
// before calling this function. For guests, use GetGuestBooking first to verify
// bookingNumber + email. For registered users, use GetAssignmentsByBooking which
// includes the ownership check internally.
func GetAssignmentsByBookingID(pool *pgxpool.Pool, bookingID string) ([]models.TicketAssignment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT id, booking_id, slot_number, status,
		       assignee_name, assignee_email,
		       assigned_at, expires_at, claimed_at, revoked_at,
		       qr_code_url, created_at, updated_at
		FROM ticket_assignments
		WHERE booking_id = $1
		ORDER BY slot_number
	`, bookingID)
	if err != nil {
		return nil, fmt.Errorf("GetAssignmentsByBookingID: %w", err)
	}
	defer rows.Close()

	var assignments []models.TicketAssignment
	for rows.Next() {
		var a models.TicketAssignment
		if err := rows.Scan(
			&a.ID, &a.BookingID, &a.SlotNumber, &a.Status,
			&a.AssigneeName, &a.AssigneeEmail,
			&a.AssignedAt, &a.ExpiresAt, &a.ClaimedAt, &a.RevokedAt,
			&a.QRCodeURL, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("GetAssignmentsByBookingID scan: %w", err)
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}