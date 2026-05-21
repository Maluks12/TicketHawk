package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	// ListingFeePerTicket is the platform listing fee in ZAR charged to the
	// organizer per ticket when the event is FREE (price = 0) and capacity
	// reaches the threshold. The organizer absorbs this cost — ticket buyers
	// are never charged.
	ListingFeePerTicket float64 = 5.00

	// ListingFeeThreshold is the minimum ticket capacity at which the listing
	// fee kicks in for free events. Below this, free events are published at
	// no cost (but still excluded from home-feed sections).
	ListingFeeThreshold int = 20
)

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	// ErrListingFeeRequired is returned by PublishEvent when a FREE event has
	// capacity >= 20 but the listing fee has not been paid.
	ErrListingFeeRequired = errors.New(
		"listing fee must be paid before publishing a free event with 20+ ticket capacity",
	)

	// ErrListingFeeAlreadyPaid is returned when the organizer tries to
	// initiate a payment for a fee that has already been settled.
	ErrListingFeeAlreadyPaid = errors.New("listing fee has already been paid for this event")

	// ErrListingFeeNotApplicable is returned when the organizer tries to pay
	// a listing fee for a paid-ticket event (not needed).
	ErrListingFeeNotApplicable = errors.New(
		"listing fee only applies to free events (price = 0); paid-ticket events are not charged",
	)
)

// ── Types ─────────────────────────────────────────────────────────────────────

// ListingFeeInfo is the read-model returned to the organizer dashboard.
type ListingFeeInfo struct {
	EventID           string  `json:"event_id"`
	IsFreeEvent       bool    `json:"is_free_event"`
	EffectiveCapacity int     `json:"effective_capacity"`
	FeeAmount         float64 `json:"fee_amount"`
	FeePaid           bool    `json:"fee_paid"`
	FeeRequired       bool    `json:"fee_required"`
}

// ── Read ──────────────────────────────────────────────────────────────────────

// GetListingFeeInfo computes the current listing fee status for an event.
//
// The listing fee ONLY applies to free events (events.price = 0).
// Paid-ticket events are never charged and always eligible for home feed.
//
// Effective capacity is events.max_capacity when set; otherwise the sum of all
// visible ticket type capacities.
func GetListingFeeInfo(pool *pgxpool.Pool, eventID, organizerID string) (*ListingFeeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		eventPrice    float64
		maxCapacity   *int
		feeAmount     float64
		feePaid       bool
		ttCapacitySum *int
	)

	err := pool.QueryRow(ctx, `
		SELECT e.price,
		       e.max_capacity,
		       e.listing_fee_amount,
		       e.listing_fee_paid,
		       (SELECT SUM(t.capacity)
		        FROM   ticket_types t
		        WHERE  t.event_id = e.id AND t.is_visible = TRUE) AS tt_cap_sum
		FROM   events e
		WHERE  e.id = $1 AND e.organizer_id = $2
	`, eventID, organizerID).Scan(&eventPrice, &maxCapacity, &feeAmount, &feePaid, &ttCapacitySum)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, fmt.Errorf("GetListingFeeInfo: %w", err)
	}

	isFreeEvent := eventPrice == 0
	effectiveCapacity := computeEffectiveCapacity(maxCapacity, ttCapacitySum)
	requiredFee := computeListingFee(isFreeEvent, effectiveCapacity)

	return &ListingFeeInfo{
		EventID:           eventID,
		IsFreeEvent:       isFreeEvent,
		EffectiveCapacity: effectiveCapacity,
		FeeAmount:         requiredFee,
		FeePaid:           feePaid,
		FeeRequired:       requiredFee > 0,
	}, nil
}

// ── Write ─────────────────────────────────────────────────────────────────────

// SetListingFeePaymentRequest records the Stitch payment request ID and stores
// the fee amount against the event before the Stitch payment flow starts.
//
// Returns ErrListingFeeNotApplicable for paid-ticket events.
// Returns ErrListingFeeAlreadyPaid when the fee is already settled.
// Returns ErrEventNotFound when the event doesn't belong to the organizer.
func SetListingFeePaymentRequest(pool *pgxpool.Pool, eventID, organizerID, paymentRequestID string, feeAmount float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		eventPrice float64
		feePaid    bool
	)
	err := pool.QueryRow(ctx, `
		SELECT price, listing_fee_paid
		FROM   events
		WHERE  id = $1 AND organizer_id = $2
	`, eventID, organizerID).Scan(&eventPrice, &feePaid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEventNotFound
		}
		return fmt.Errorf("SetListingFeePaymentRequest check: %w", err)
	}

	// Paid-ticket events are never charged a listing fee
	if eventPrice > 0 {
		return ErrListingFeeNotApplicable
	}
	if feePaid {
		return ErrListingFeeAlreadyPaid
	}

	cmd, err := pool.Exec(ctx, `
		UPDATE events
		SET    listing_fee_payment_request_id = $1,
		       listing_fee_amount             = $2,
		       updated_at                     = NOW()
		WHERE  id              = $3
		  AND  organizer_id    = $4
		  AND  price           = 0
		  AND  listing_fee_paid = FALSE
	`, paymentRequestID, feeAmount, eventID, organizerID)
	if err != nil {
		return fmt.Errorf("SetListingFeePaymentRequest: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrListingFeeAlreadyPaid
	}
	return nil
}

// ConfirmListingFeeByPaymentRequest marks listing_fee_paid = TRUE on the event
// identified by its Stitch payment request ID. Called exclusively from the
// webhook handler.
//
// Returns (eventID, true, nil) on success.
// Returns ("", false, nil) when no event matches — safe to ignore, it's a
// different payment type.
func ConfirmListingFeeByPaymentRequest(pool *pgxpool.Pool, paymentRequestID string) (eventID string, matched bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = pool.QueryRow(ctx, `
		UPDATE events
		SET    listing_fee_paid = TRUE,
		       updated_at       = NOW()
		WHERE  listing_fee_payment_request_id = $1
		  AND  listing_fee_paid               = FALSE
		  AND  price                          = 0
		RETURNING id
	`, paymentRequestID).Scan(&eventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("ConfirmListingFeeByPaymentRequest: %w", err)
	}
	return eventID, true, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// computeEffectiveCapacity resolves total ticket capacity for the fee check.
//   - max_capacity wins when set (organizer-level hard cap).
//   - Falls back to the sum of visible ticket type capacities.
//   - Returns 0 when neither is set (treated as below threshold → no fee).
func computeEffectiveCapacity(maxCapacity *int, ttCapacitySum *int) int {
	if maxCapacity != nil {
		return *maxCapacity
	}
	if ttCapacitySum != nil {
		return *ttCapacitySum
	}
	return 0
}

// computeListingFee returns the ZAR fee owed.
//   - Returns 0 for paid-ticket events (listing fee does not apply).
//   - Returns 0 for free events below the capacity threshold.
//   - Returns capacity × ListingFeePerTicket otherwise.
func computeListingFee(isFreeEvent bool, capacity int) float64 {
	if !isFreeEvent {
		return 0 // paid-ticket events are never charged
	}
	if capacity < ListingFeeThreshold {
		return 0 // free events under 20 capacity publish at no cost
	}
	return float64(capacity) * ListingFeePerTicket
}