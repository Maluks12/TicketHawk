package repository

import (
	"context"
	"errors"
	"events-db/internal/models"
	"events-db/internal/utils"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrEventNotFound = errors.New("event not found")
var ErrEventSoldOut = errors.New("event is sold out")
var ErrEventNotPublishable = errors.New("event not found or not in a submittable state")
var ErrEventNotCancellable = errors.New("event not found or cannot be cancelled")

type OrganizerEventSummary struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartTime   time.Time `json:"start_time"`
	Status      string    `json:"status"`
	TicketsSold int       `json:"tickets_sold"`
	MaxCapacity *int      `json:"max_capacity,omitempty"`
	Price       float64   `json:"price"`
	Currency    string    `json:"currency"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateEvent inserts a new event in draft status and returns the created row.
// The slug is generated client-side from the title and a pre-generated UUID so
// it can be included in the single INSERT — no second round-trip needed.
func CreateEvent(pool *pgxpool.Pool, dto *models.CreateEventDTO, organizerID string) (*models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id := uuid.New().String()
	slug := utils.GenerateSlug(dto.Title, id)

	query := `
        INSERT INTO events (
            id, slug,
            title, description, cover_image_url, banner_image_url, gallery_images,
            start_time, end_time, expires_at, timezone,
            venue_name, address, city, latitude, longitude,
            category_id, price, currency, max_capacity, allow_waitlist,
            max_tickets_per_user, venue_type,
            refund_policy, terms_and_conditions, tags, performers, teams,
            organizer_id, status
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30)
        RETURNING id, slug, title, description, cover_image_url, start_time, end_time,
                  price, currency, max_capacity, tickets_sold, status, organizer_id, created_at, updated_at
    `

	var event models.Event
	err := pool.QueryRow(ctx, query,
		id, slug,
		dto.Title, dto.Description, dto.CoverImageURL, dto.BannerImageURL, dto.GalleryImages,
		dto.StartTime, dto.EndTime, dto.ExpiresAt, dto.Timezone,
		dto.VenueName, dto.Address, dto.City, dto.Latitude, dto.Longitude,
		dto.CategoryID, dto.Price, dto.Currency, dto.MaxCapacity, dto.AllowWaitlist,
		dto.MaxTicketsPerUser, dto.VenueType,
		dto.RefundPolicy, dto.TermsAndConditions, dto.Tags, dto.Performers, dto.Teams,
		organizerID, models.EventDraft,
	).Scan(
		&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL,
		&event.StartTime, &event.EndTime, &event.Price, &event.Currency,
		&event.MaxCapacity, &event.TicketsSold, &event.Status, &event.OrganizerID,
		&event.CreatedAt, &event.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create event: %w", err)
	}

	event.ComputeAvailableTickets()
	return &event, nil
}

// PublishEvent submits a draft or rejected event for admin review.
// Status moves to pending_review; event goes live only after AdminApproveEvent.
func PublishEvent(pool *pgxpool.Pool, eventID string, organizerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Guard: free events (price = 0) with capacity >= 20 must pay the listing
	// fee before submitting for review. Paid-ticket events are never blocked.
	var (
		eventPrice  float64
		maxCap      *int
		feePaid     bool
		ttCapSum    *int
	)
	err := pool.QueryRow(ctx, `
		SELECT e.price, e.max_capacity, e.listing_fee_paid,
		       (SELECT SUM(t.capacity)
		        FROM   ticket_types t
		        WHERE  t.event_id = e.id AND t.is_visible = TRUE) AS tt_cap_sum
		FROM   events e
		WHERE  e.id = $1 AND e.organizer_id = $2
	`, eventID, organizerID).Scan(&eventPrice, &maxCap, &feePaid, &ttCapSum)
	if err != nil {
		return ErrEventNotPublishable
	}

	// Only free events are subject to the listing fee
	if eventPrice == 0 {
		effectiveCapacity := computeEffectiveCapacity(maxCap, ttCapSum)
		if effectiveCapacity >= ListingFeeThreshold && !feePaid {
			return ErrListingFeeRequired
		}
	}

	cmd, err := pool.Exec(ctx, `
		UPDATE events
		SET status = $1, rejection_reason = NULL
		WHERE id = $2 AND organizer_id = $3 AND status IN ($4, $5)
	`, models.EventPendingReview, eventID, organizerID, models.EventDraft, models.EventRejected)
	if err != nil {
		return fmt.Errorf("failed to submit event for review: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrEventNotPublishable
	}
	return nil
}

// AdminApproveEvent approves a pending_review event, making it publicly visible.
func AdminApproveEvent(pool *pgxpool.Pool, eventID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE events
		SET status = $1, published_at = CURRENT_TIMESTAMP, rejection_reason = NULL
		WHERE id = $2 AND status = $3
	`, models.EventPublished, eventID, models.EventPendingReview)
	if err != nil {
		return fmt.Errorf("failed to approve event: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrEventNotPublishable
	}
	return nil
}

// AdminRejectEvent rejects a pending_review event with a reason.
// The organiser can revise and resubmit (calls PublishEvent again).
func AdminRejectEvent(pool *pgxpool.Pool, eventID string, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE events
		SET status = $1, rejection_reason = $2
		WHERE id = $3 AND status = $4
	`, models.EventRejected, reason, eventID, models.EventPendingReview)
	if err != nil {
		return fmt.Errorf("failed to reject event: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrEventNotPublishable
	}
	return nil
}

// AdminGetPendingEvents returns all events awaiting admin review, oldest first.
func AdminGetPendingEvents(pool *pgxpool.Pool) ([]models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT e.id, e.slug, e.title, e.description, e.status,
		       e.start_time, e.end_time, e.price, e.currency,
		       e.venue_name, e.city, e.organizer_id, e.created_at
		FROM events e
		WHERE e.status = $1
		ORDER BY e.created_at ASC
	`, models.EventPendingReview)
	if err != nil {
		return nil, fmt.Errorf("AdminGetPendingEvents: %w", err)
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var ev models.Event
		if err := rows.Scan(
			&ev.ID, &ev.Slug, &ev.Title, &ev.Description, &ev.Status,
			&ev.StartTime, &ev.EndTime, &ev.Price, &ev.Currency,
			&ev.VenueName, &ev.City, &ev.OrganizerID, &ev.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("AdminGetPendingEvents scan: %w", err)
		}
		events = append(events, ev)
	}
	if events == nil {
		events = []models.Event{}
	}
	return events, rows.Err()
}

func CheckAvailability(pool *pgxpool.Pool, eventID string, quantity int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var maxCapacity *int
	var ticketsSold int

	err := pool.QueryRow(ctx, `
		SELECT max_capacity, tickets_sold
		FROM events
		WHERE id = $1 AND status = $2
	`, eventID, models.EventPublished).Scan(&maxCapacity, &ticketsSold)
	if err != nil {
		return false, fmt.Errorf("failed to check availability: %w", err)
	}

	if maxCapacity == nil {
		return true, nil
	}
	return *maxCapacity-ticketsSold >= quantity, nil
}

func IncrementTicketsSold(pool *pgxpool.Pool, eventID string, quantity int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var maxCapacity *int
	var ticketsSold int
	err = tx.QueryRow(ctx, `
		SELECT max_capacity, tickets_sold FROM events WHERE id = $1 FOR UPDATE
	`, eventID).Scan(&maxCapacity, &ticketsSold)
	if err != nil {
		return fmt.Errorf("failed to lock event: %w", err)
	}

	if maxCapacity != nil && ticketsSold+quantity > *maxCapacity {
		return ErrInsufficientCapacity
	}

	if _, err = tx.Exec(ctx, `
		UPDATE events SET tickets_sold = tickets_sold + $1 WHERE id = $2
	`, quantity, eventID); err != nil {
		return fmt.Errorf("failed to update tickets sold: %w", err)
	}

	return tx.Commit(ctx)
}

func DecrementTicketsSold(pool *pgxpool.Pool, eventID string, quantity int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE events SET tickets_sold = GREATEST(tickets_sold - $1, 0) WHERE id = $2
	`, quantity, eventID)
	return err
}

func DeleteEvents(pool *pgxpool.Pool, eventID string, organizerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		DELETE FROM events WHERE id = $1
		AND status IN ('draft', 'cancelled')
	`, eventID)
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}

func CancelEvent(pool *pgxpool.Pool, eventID string, reason string, organizerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
 
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)
 
	cmd, err := tx.Exec(ctx, `
		UPDATE events
		SET status = $1, cancelled_at = CURRENT_TIMESTAMP, cancellation_reason = $2
		WHERE id = $3 AND organizer_id = $4 AND status = $5
	`, models.EventCancelled, reason, eventID, organizerID, models.EventPublished)
	if err != nil {
		return fmt.Errorf("failed to cancel event: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrEventNotCancellable
	}
 
	if _, err = tx.Exec(ctx, `
		UPDATE bookings
		SET booking_status = $1, refund_requested_at = CURRENT_TIMESTAMP, refund_reason = $2
		WHERE event_id = $3 AND booking_status = $4
	`, models.BookingRefunded, "Event cancelled by organizer", eventID, models.BookingConfirmed); err != nil {
		return fmt.Errorf("failed to update bookings: %w", err)
	}
 
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit cancellation: %w", err)
	}
 
	// Notify attendees — runs outside the transaction so a notification failure
	// never rolls back the cancellation.
	notifRows, notifErr := pool.Query(ctx, `
		SELECT DISTINCT user_id FROM bookings WHERE event_id = $1 AND user_id IS NOT NULL
	`, eventID)
	if notifErr == nil {
		defer notifRows.Close()
		for notifRows.Next() {
			var uid string
			if err := notifRows.Scan(&uid); err != nil {
				continue
			}
			_ = CreateNotification(
				pool, ctx, uid,
				models.NotifEventCancelled,
				"Event Cancelled",
				"An event you booked has been cancelled. Your refund is being processed.",
				&eventID,
			)
		}
	}
 
	return nil
}

func TrendingEvents(pool *pgxpool.Pool, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished}
	where := "WHERE e.status = $1"

	if cursor != nil {
		where += `
			AND (e.views < $2 OR (e.views = $2 AND (e.start_time < $3 OR (e.start_time = $3 AND e.id < $4))))
		`
		args = append(args, cursor.Views, cursor.StartTime, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
        SELECT
            e.id, e.slug, e.title, e.description, e.cover_image_url, e.banner_image_url,
            e.start_time, e.end_time, e.price, e.currency,
            e.city, e.venue_name, e.views, c.name as category
        FROM events e
        JOIN categories c ON e.category_id = c.id
        %s
        ORDER BY e.views DESC, e.start_time DESC, e.id DESC
        LIMIT $%d
    `, where, limitPlaceholder)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit)
	for rows.Next() {
		var event models.Event
		err = rows.Scan(
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL, &event.BannerImageURL,
			&event.StartTime, &event.EndTime, &event.Price, &event.Currency,
			&event.City, &event.VenueName, &event.Views, &event.Category,
		)
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{Views: last.Views, StartTime: last.StartTime, ID: last.ID}
		events = events[:limit]
	}
	return events, nextCursor, nil
}

// UpdateEvents applies a partial update to an event. When the title changes the
// slug is automatically regenerated so it stays in sync.
func UpdateEvents(pool *pgxpool.Pool, eventID string, dto *models.UpdateEventDTO, organizerID string) (*models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if dto.Title != nil {
		updates = append(updates, fmt.Sprintf("title = $%d", argIndex))
		args = append(args, *dto.Title)
		argIndex++

		// Regenerate slug whenever the title changes.
		updates = append(updates, fmt.Sprintf("slug = $%d", argIndex))
		args = append(args, utils.GenerateSlug(*dto.Title, eventID))
		argIndex++
	}
	if dto.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argIndex))
		args = append(args, *dto.Description)
		argIndex++
	}
	if dto.CoverImageURL != nil {
		updates = append(updates, fmt.Sprintf("cover_image_url = $%d", argIndex))
		args = append(args, *dto.CoverImageURL)
		argIndex++
	}
	if dto.BannerImageURL != nil {
		updates = append(updates, fmt.Sprintf("banner_image_url = $%d", argIndex))
		args = append(args, *dto.BannerImageURL)
		argIndex++
	}
	if dto.BannerVideoURL != nil {
		updates = append(updates, fmt.Sprintf("banner_video_url = $%d", argIndex))
		args = append(args, *dto.BannerVideoURL)
		argIndex++
	}
	if dto.Price != nil {
		updates = append(updates, fmt.Sprintf("price = $%d", argIndex))
		args = append(args, *dto.Price)
		argIndex++
	}
	if dto.StartTime != nil {
		updates = append(updates, fmt.Sprintf("start_time = $%d", argIndex))
		args = append(args, *dto.StartTime)
		argIndex++
	}
	if dto.EndTime != nil {
		updates = append(updates, fmt.Sprintf("end_time = $%d", argIndex))
		args = append(args, *dto.EndTime)
		argIndex++
	}
	if dto.ExpiresAt != nil {
		updates = append(updates, fmt.Sprintf("expires_at = $%d", argIndex))
		args = append(args, *dto.ExpiresAt)
		argIndex++
	}
	if dto.Timezone != nil {
		updates = append(updates, fmt.Sprintf("timezone = $%d", argIndex))
		args = append(args, *dto.Timezone)
		argIndex++
	}
	if dto.VenueName != nil {
		updates = append(updates, fmt.Sprintf("venue_name = $%d", argIndex))
		args = append(args, *dto.VenueName)
		argIndex++
	}
	if dto.Address != nil {
		updates = append(updates, fmt.Sprintf("address = $%d", argIndex))
		args = append(args, *dto.Address)
		argIndex++
	}
	if dto.City != nil {
		updates = append(updates, fmt.Sprintf("city = $%d", argIndex))
		args = append(args, *dto.City)
		argIndex++
	}
	if dto.Latitude != nil {
		updates = append(updates, fmt.Sprintf("latitude = $%d", argIndex))
		args = append(args, *dto.Latitude)
		argIndex++
	}
	if dto.Longitude != nil {
		updates = append(updates, fmt.Sprintf("longitude = $%d", argIndex))
		args = append(args, *dto.Longitude)
		argIndex++
	}
	if dto.MaxCapacity != nil {
		updates = append(updates, fmt.Sprintf("max_capacity = $%d", argIndex))
		args = append(args, *dto.MaxCapacity)
		argIndex++
	}
	if dto.AllowWaitlist != nil {
		updates = append(updates, fmt.Sprintf("allow_waitlist = $%d", argIndex))
		args = append(args, *dto.AllowWaitlist)
		argIndex++
	}
	if dto.MaxTicketsPerUser != nil {
		updates = append(updates, fmt.Sprintf("max_tickets_per_user = $%d", argIndex))
		args = append(args, *dto.MaxTicketsPerUser)
		argIndex++
	}
	if dto.VenueType != nil {
		updates = append(updates, fmt.Sprintf("venue_type = $%d", argIndex))
		args = append(args, *dto.VenueType)
		argIndex++
	}
	if dto.IsFeatured != nil {
		updates = append(updates, fmt.Sprintf("is_featured = $%d", argIndex))
		args = append(args, *dto.IsFeatured)
		argIndex++
	}
	if dto.FeaturedUntil != nil {
		updates = append(updates, fmt.Sprintf("featured_until = $%d", argIndex))
		args = append(args, *dto.FeaturedUntil)
		argIndex++
	}
	if dto.RefundPolicy != nil {
		updates = append(updates, fmt.Sprintf("refund_policy = $%d", argIndex))
		args = append(args, *dto.RefundPolicy)
		argIndex++
	}
	if dto.TermsAndConditions != nil {
		updates = append(updates, fmt.Sprintf("terms_and_conditions = $%d", argIndex))
		args = append(args, *dto.TermsAndConditions)
		argIndex++
	}
	if dto.Tags != nil {
		updates = append(updates, fmt.Sprintf("tags = $%d", argIndex))
		args = append(args, *dto.Tags)
		argIndex++
	}
	if dto.Performers != nil {
		updates = append(updates, fmt.Sprintf("performers = $%d", argIndex))
		args = append(args, *dto.Performers)
		argIndex++
	}
	if dto.Teams != nil {
		updates = append(updates, fmt.Sprintf("teams = $%d", argIndex))
		args = append(args, *dto.Teams)
		argIndex++
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")

	query := fmt.Sprintf(`
        UPDATE events
        SET %s
        WHERE id = $%d AND organizer_id = $%d
        RETURNING id, slug, title, description, status, updated_at
    `, strings.Join(updates, ", "), argIndex, argIndex+1)

	args = append(args, eventID, organizerID)

	var event models.Event
	err := pool.QueryRow(ctx, query, args...).Scan(
		&event.ID, &event.Slug, &event.Title, &event.Description, &event.Status, &event.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update event: %w", err)
	}

	return GetEventByID(pool, event.ID, &organizerID)
}

// AdminUpdateEvent applies a partial update to any event without requiring
// organizer ownership. Used exclusively by the admin panel (PUT /admin/events/:id)
// so admins can edit events created by any organizer.
func AdminUpdateEvent(pool *pgxpool.Pool, eventID string, dto *models.UpdateEventDTO) (*models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if dto.Title != nil {
		updates = append(updates, fmt.Sprintf("title = $%d", argIndex))
		args = append(args, *dto.Title)
		argIndex++
		updates = append(updates, fmt.Sprintf("slug = $%d", argIndex))
		args = append(args, utils.GenerateSlug(*dto.Title, eventID))
		argIndex++
	}
	if dto.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argIndex))
		args = append(args, *dto.Description)
		argIndex++
	}
	if dto.CoverImageURL != nil {
		updates = append(updates, fmt.Sprintf("cover_image_url = $%d", argIndex))
		args = append(args, *dto.CoverImageURL)
		argIndex++
	}
	if dto.BannerImageURL != nil {
		updates = append(updates, fmt.Sprintf("banner_image_url = $%d", argIndex))
		args = append(args, *dto.BannerImageURL)
		argIndex++
	}
	if dto.BannerVideoURL != nil {
		updates = append(updates, fmt.Sprintf("banner_video_url = $%d", argIndex))
		args = append(args, *dto.BannerVideoURL)
		argIndex++
	}
	if dto.Price != nil {
		updates = append(updates, fmt.Sprintf("price = $%d", argIndex))
		args = append(args, *dto.Price)
		argIndex++
	}
	if dto.StartTime != nil {
		updates = append(updates, fmt.Sprintf("start_time = $%d", argIndex))
		args = append(args, *dto.StartTime)
		argIndex++
	}
	if dto.EndTime != nil {
		updates = append(updates, fmt.Sprintf("end_time = $%d", argIndex))
		args = append(args, *dto.EndTime)
		argIndex++
	}
	if dto.ExpiresAt != nil {
		updates = append(updates, fmt.Sprintf("expires_at = $%d", argIndex))
		args = append(args, *dto.ExpiresAt)
		argIndex++
	}
	if dto.Timezone != nil {
		updates = append(updates, fmt.Sprintf("timezone = $%d", argIndex))
		args = append(args, *dto.Timezone)
		argIndex++
	}
	if dto.VenueName != nil {
		updates = append(updates, fmt.Sprintf("venue_name = $%d", argIndex))
		args = append(args, *dto.VenueName)
		argIndex++
	}
	if dto.Address != nil {
		updates = append(updates, fmt.Sprintf("address = $%d", argIndex))
		args = append(args, *dto.Address)
		argIndex++
	}
	if dto.City != nil {
		updates = append(updates, fmt.Sprintf("city = $%d", argIndex))
		args = append(args, *dto.City)
		argIndex++
	}
	if dto.Latitude != nil {
		updates = append(updates, fmt.Sprintf("latitude = $%d", argIndex))
		args = append(args, *dto.Latitude)
		argIndex++
	}
	if dto.Longitude != nil {
		updates = append(updates, fmt.Sprintf("longitude = $%d", argIndex))
		args = append(args, *dto.Longitude)
		argIndex++
	}
	if dto.MaxCapacity != nil {
		updates = append(updates, fmt.Sprintf("max_capacity = $%d", argIndex))
		args = append(args, *dto.MaxCapacity)
		argIndex++
	}
	if dto.AllowWaitlist != nil {
		updates = append(updates, fmt.Sprintf("allow_waitlist = $%d", argIndex))
		args = append(args, *dto.AllowWaitlist)
		argIndex++
	}
	if dto.MaxTicketsPerUser != nil {
		updates = append(updates, fmt.Sprintf("max_tickets_per_user = $%d", argIndex))
		args = append(args, *dto.MaxTicketsPerUser)
		argIndex++
	}
	if dto.VenueType != nil {
		updates = append(updates, fmt.Sprintf("venue_type = $%d", argIndex))
		args = append(args, *dto.VenueType)
		argIndex++
	}
	if dto.IsFeatured != nil {
		updates = append(updates, fmt.Sprintf("is_featured = $%d", argIndex))
		args = append(args, *dto.IsFeatured)
		argIndex++
	}
	if dto.FeaturedUntil != nil {
		updates = append(updates, fmt.Sprintf("featured_until = $%d", argIndex))
		args = append(args, *dto.FeaturedUntil)
		argIndex++
	}
	if dto.RefundPolicy != nil {
		updates = append(updates, fmt.Sprintf("refund_policy = $%d", argIndex))
		args = append(args, *dto.RefundPolicy)
		argIndex++
	}
	if dto.TermsAndConditions != nil {
		updates = append(updates, fmt.Sprintf("terms_and_conditions = $%d", argIndex))
		args = append(args, *dto.TermsAndConditions)
		argIndex++
	}
	if dto.Tags != nil {
		updates = append(updates, fmt.Sprintf("tags = $%d", argIndex))
		args = append(args, *dto.Tags)
		argIndex++
	}
	if dto.Performers != nil {
		updates = append(updates, fmt.Sprintf("performers = $%d", argIndex))
		args = append(args, *dto.Performers)
		argIndex++
	}
	if dto.Teams != nil {
		updates = append(updates, fmt.Sprintf("teams = $%d", argIndex))
		args = append(args, *dto.Teams)
		argIndex++
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")

	// Admin variant: no organizer_id constraint — admins can edit any event.
	query := fmt.Sprintf(`
        UPDATE events
        SET %s
        WHERE id = $%d
        RETURNING id
    `, strings.Join(updates, ", "), argIndex)

	args = append(args, eventID)

	var returnedID string
	err := pool.QueryRow(ctx, query, args...).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, fmt.Errorf("AdminUpdateEvent: failed to update event: %w", err)
	}

	return AdminGetEventByID(pool, returnedID)
}

// AdminGetEventByID fetches a full event by UUID with no status restriction —
// admins can view events in any state (draft, pending, published, cancelled).
func AdminGetEventByID(pool *pgxpool.Pool, id string) (*models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var event models.Event
	err := pool.QueryRow(ctx, `
        SELECT
            e.id, e.slug, e.title, e.description, e.cover_image_url, e.banner_image_url,
            e.start_time, e.end_time, e.expires_at, e.timezone,
            e.venue_name, e.address, e.city, e.latitude, e.longitude,
            e.category_id, e.price, e.currency, e.max_capacity, e.tickets_sold,
            e.allow_waitlist, e.max_tickets_per_user, e.venue_type,
            e.is_featured, e.featured_until, e.status,
            e.refund_policy, e.terms_and_conditions, e.tags, e.views,
            e.organizer_id, e.created_at, e.updated_at,
            c.name as category,
            e.performers, e.teams
        FROM events e
        JOIN categories c ON e.category_id = c.id
        WHERE e.id = $1
    `, id).Scan(
		&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL, &event.BannerImageURL,
		&event.StartTime, &event.EndTime, &event.ExpiresAt, &event.Timezone,
		&event.VenueName, &event.Address, &event.City, &event.Latitude, &event.Longitude,
		&event.CategoryID, &event.Price, &event.Currency, &event.MaxCapacity, &event.TicketsSold,
		&event.AllowWaitlist, &event.MaxTicketsPerUser, &event.VenueType,
		&event.IsFeatured, &event.FeaturedUntil, &event.Status,
		&event.RefundPolicy, &event.TermsAndConditions, &event.Tags, &event.Views,
		&event.OrganizerID, &event.CreatedAt, &event.UpdatedAt,
		&event.Category,
		&event.Performers, &event.Teams,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, fmt.Errorf("AdminGetEventByID: %w", err)
	}

	event.ComputeAvailableTickets()

	ticketTypes, err := GetTicketTypesByEventID(pool, event.ID)
	if err == nil && len(ticketTypes) > 0 {
		event.TicketTypes = ticketTypes
	}

	return &event, nil
}

// GetEventByID fetches a full event by UUID. When requestingUserID is nil only
// published events are returned; when set, drafts owned by that user are also
// visible (supporting the organiser preview flow).
func GetEventByID(pool *pgxpool.Pool, id string, requestingUserID *string) (*models.Event, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    query := `
        SELECT
            e.id, e.slug, e.title, e.description, e.cover_image_url, e.banner_image_url,
            e.start_time, e.end_time, e.expires_at, e.timezone,
            e.venue_name, e.address, e.city, e.latitude, e.longitude,
            e.category_id, e.price, e.currency, e.max_capacity, e.tickets_sold,
            e.allow_waitlist, e.max_tickets_per_user, e.venue_type,
            e.is_featured, e.featured_until, e.status,
            e.refund_policy, e.terms_and_conditions, e.tags, e.views,
            e.organizer_id, e.created_at, e.updated_at,
            c.name as category,
            e.performers, e.teams
        FROM events e
        JOIN categories c ON e.category_id = c.id
        WHERE e.id = $1
    `

    args := []interface{}{id}
    if requestingUserID == nil {
        query += ` AND e.status = $2`
        args = append(args, models.EventPublished)
    } else {
        query += ` AND (e.status = $2 OR e.organizer_id = $3)`
        args = append(args, models.EventPublished, *requestingUserID)
    }

    var event models.Event
    err := pool.QueryRow(ctx, query, args...).Scan(
        &event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL, &event.BannerImageURL,
        &event.StartTime, &event.EndTime, &event.ExpiresAt, &event.Timezone,
        &event.VenueName, &event.Address, &event.City, &event.Latitude, &event.Longitude,
        &event.CategoryID, &event.Price, &event.Currency, &event.MaxCapacity, &event.TicketsSold,
        &event.AllowWaitlist, &event.MaxTicketsPerUser, &event.VenueType,
		&event.IsFeatured, &event.FeaturedUntil, &event.Status,
        &event.RefundPolicy, &event.TermsAndConditions, &event.Tags, &event.Views,
        &event.OrganizerID, &event.CreatedAt, &event.UpdatedAt,
        &event.Category,
        &event.Performers, &event.Teams,
    )
    if err != nil {
        // Fix 6: distinguish a missing row from a genuine DB failure so callers
        // can return 404 vs 500 correctly.
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrEventNotFound
        }
        return nil, fmt.Errorf("failed to get event: %w", err)
    }

    event.ComputeAvailableTickets()

    // Load ticket types — nil slice means the event uses flat pricing
    ticketTypes, err := GetTicketTypesByEventID(pool, event.ID)
    if err == nil && len(ticketTypes) > 0 {
        event.TicketTypes = ticketTypes
    }

    return &event, nil
}

// GetEventBySlug fetches a published event by its URL slug. This is the primary
// lookup path for the public-facing event detail page.
func GetEventBySlug(pool *pgxpool.Pool, slug string) (*models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
 
	var event models.Event
	err := pool.QueryRow(ctx, `
        SELECT
            e.id, e.slug, e.title, e.description, e.cover_image_url, e.banner_image_url,
            e.banner_video_url,
            e.start_time, e.end_time, e.expires_at, e.timezone,
            e.venue_name, e.address, e.city, e.latitude, e.longitude,
            e.category_id, e.price, e.currency, e.max_capacity, e.tickets_sold,
            e.allow_waitlist, e.max_tickets_per_user, e.venue_type,
            e.is_featured, e.featured_until, e.status,
            e.refund_policy, e.terms_and_conditions, e.tags, e.views,
            e.organizer_id, e.created_at, e.updated_at,
            c.name as category,
            e.performers, e.teams
        FROM events e
        JOIN categories c ON e.category_id = c.id
        WHERE e.slug = $1
          AND e.status = $2
    `, slug, models.EventPublished).Scan(
		&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL, &event.BannerImageURL,
		&event.BannerVideoURL,
		&event.StartTime, &event.EndTime, &event.ExpiresAt, &event.Timezone,
		&event.VenueName, &event.Address, &event.City, &event.Latitude, &event.Longitude,
		&event.CategoryID, &event.Price, &event.Currency, &event.MaxCapacity, &event.TicketsSold,
		&event.AllowWaitlist, &event.MaxTicketsPerUser, &event.VenueType,
		&event.IsFeatured, &event.FeaturedUntil, &event.Status,
		&event.RefundPolicy, &event.TermsAndConditions, &event.Tags, &event.Views,
		&event.OrganizerID, &event.CreatedAt, &event.UpdatedAt,
		&event.Category,
		&event.Performers, &event.Teams,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, fmt.Errorf("GetEventBySlug: %w", err)
	}
 
	event.ComputeAvailableTickets()

	// Load ticket types — nil slice means the event uses flat pricing
	ticketTypes, err := GetTicketTypesByEventID(pool, event.ID)
	if err == nil && len(ticketTypes) > 0 {
		event.TicketTypes = ticketTypes
	}

	return &event, nil
}

func GetAllEvents(pool *pgxpool.Pool, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished}
	where := "WHERE e.status = $1"

	if cursor != nil {
		where += `
			AND (
				e.start_time < $2
				OR (e.start_time = $2 AND e.id < $3)
			)
		`
		args = append(args, cursor.StartTime, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
        SELECT
            e.id, e.slug, e.title, e.description, e.cover_image_url, e.banner_image_url,
            e.start_time, e.end_time, e.price, e.currency,
            e.city, e.venue_name, e.max_capacity, e.tickets_sold,
            e.is_featured, e.status, e.views, e.created_at,
            c.name as category
        FROM events e
        JOIN categories c ON e.category_id = c.id
        %s
        ORDER BY e.start_time DESC, e.id DESC
        LIMIT $%d
    `, where, limitPlaceholder)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get events: %w", err)
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit)
	for rows.Next() {
		var event models.Event
		err = rows.Scan(
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL, &event.BannerImageURL,
			&event.StartTime, &event.EndTime, &event.Price, &event.Currency,
			&event.City, &event.VenueName, &event.MaxCapacity, &event.TicketsSold,
			&event.IsFeatured, &event.Status, &event.Views, &event.CreatedAt,
			&event.Category,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan event: %w", err)
		}
		event.ComputeAvailableTickets()
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{StartTime: last.StartTime, ID: last.ID}
		events = events[:limit]
	}
	return events, nextCursor, nil
}

func FeaturedEvents(pool *pgxpool.Pool, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished, true, time.Now()}
	where := `
	WHERE e.status = $1
	AND (e.price > 0 OR e.listing_fee_paid = TRUE)
	AND e.is_featured = $2
	AND e.featured_until > $3
	`

	if cursor != nil {
		where += `
			AND (
				featured_until < $4
				OR (
					featured_until = $4
					AND (
						start_time < $5
						OR (start_time = $5 AND id < $6)
					)
				)
			)
		`
		args = append(args, cursor.FeaturedUntil, cursor.StartTime, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := `
        SELECT
            e.id, e.slug, e.title, e.description, e.cover_image_url, e.banner_image_url,
            e.start_time, e.end_time, e.price, e.currency,
            e.city, e.venue_name, e.max_capacity, e.tickets_sold,
            e.is_featured, e.status, e.views, e.created_at,
            c.name as category
        FROM events e
        JOIN categories c ON e.category_id = c.id
    ` + where + `
		ORDER BY featured_until DESC, start_time DESC, id DESC
		LIMIT $` + strconv.Itoa(limitPlaceholder)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit+1)
	for rows.Next() {
		var event models.Event
		err = rows.Scan(
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL, &event.BannerImageURL,
			&event.StartTime, &event.EndTime, &event.Price, &event.Currency,
			&event.City, &event.VenueName, &event.MaxCapacity, &event.TicketsSold,
			&event.IsFeatured, &event.Status, &event.Views, &event.CreatedAt,
			&event.Category,
		)
		if err != nil {
			return nil, nil, err
		}
		event.ComputeAvailableTickets()
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{
			FeaturedUntil: *last.FeaturedUntil,
			StartTime:     last.StartTime,
			ID:            last.ID,
		}
		events = events[:limit]
	}
	return events, nextCursor, nil
}

func ActiveEvents(pool *pgxpool.Pool, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished, time.Now()}
	where := `
	WHERE e.status = $1
	AND end_time > $2
	`

	if cursor != nil {
		where += `
			AND (
				start_time < $3
				OR (start_time = $3 AND id < $4)
			)
		`
		args = append(args, cursor.StartTime, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := `
		SELECT
            e.id, e.slug, e.title, e.description, e.cover_image_url, e.banner_image_url,
            e.start_time, e.end_time, e.price, e.currency,
            e.city, e.venue_name, e.max_capacity, e.tickets_sold,
            e.is_featured, e.status, e.views, e.created_at,
            c.name as category
        FROM events e
		JOIN categories c ON e.category_id = c.id
	` + where + `
		ORDER BY start_time DESC, id DESC
		LIMIT $` + strconv.Itoa(limitPlaceholder)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit+1)
	for rows.Next() {
		var event models.Event
		err = rows.Scan(
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL, &event.BannerImageURL,
			&event.StartTime, &event.EndTime, &event.Price, &event.Currency,
			&event.City, &event.VenueName, &event.MaxCapacity, &event.TicketsSold,
			&event.IsFeatured, &event.Status, &event.Views, &event.CreatedAt,
			&event.Category,
		)
		if err != nil {
			return nil, nil, err
		}
		event.ComputeAvailableTickets()
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{StartTime: last.StartTime, ID: last.ID}
		events = events[:limit]
	}
	return events, nextCursor, nil
}

func IncrementViews(pool *pgxpool.Pool, eventID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx,
		`UPDATE events SET views = views + 1 WHERE id = $1 AND status = $2`,
		eventID, models.EventPublished,
	)
	if err != nil {
		return fmt.Errorf("failed to increment views: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}

func GetEventBycategory(pool *pgxpool.Pool, category_id int, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished, category_id}
	where := `
		WHERE e.status = $1
		AND category_id = $2
	`

	if cursor != nil {
		where += `
			AND (
				start_time < $3
				or (start_time = $3 AND id < $4)
			)
		`
		args = append(args, cursor.StartTime, cursor.ID)
	}

	limitPlaceholder := len(args) + 1
	args = append(args, limit+1)

	query := `
		SELECT
            e.id, e.slug, e.title, e.description, e.cover_image_url, e.banner_image_url,
            e.start_time, e.end_time, e.price, e.currency,
            e.city, e.venue_name, e.max_capacity, e.tickets_sold,
            e.is_featured, e.status, e.views, e.created_at,
            c.name as category
        FROM events e
		JOIN categories c ON e.category_id = c.id
	` + where + `
		ORDER BY start_time DESC, id DESC
		LIMIT $` + strconv.Itoa(limitPlaceholder)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit+1)
	for rows.Next() {
		var event models.Event
		err = rows.Scan(
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL, &event.BannerImageURL,
			&event.StartTime, &event.EndTime, &event.Price, &event.Currency,
			&event.City, &event.VenueName, &event.MaxCapacity, &event.TicketsSold,
			&event.IsFeatured, &event.Status, &event.Views, &event.CreatedAt,
			&event.Category,
		)
		if err != nil {
			return nil, nil, err
		}
		event.ComputeAvailableTickets()
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{StartTime: last.StartTime, ID: last.ID}
		events = events[:limit]
	}
	return events, nextCursor, nil
}

// GetCategories returns all active categories ordered by display_order.
func GetCategories(pool *pgxpool.Pool) ([]models.Category, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT id, name, slug
		FROM categories
		WHERE is_active = TRUE
		ORDER BY display_order ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cats := make([]models.Category, 0)
	for rows.Next() {
		var c models.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

func SearchEvents(pool *pgxpool.Pool, query string, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Combined search strategy:
	//   1. Prefix full-text: to_tsquery with :* matches words that START with the query
	//      e.g. "dur" -> matches "Durban", "curry" -> "Curry Mile Fest"
	//   2. Trigram similarity: word_similarity() catches typos and transpositions
	//      e.g. "joberg" -> matches "joburg", "captown" -> "Cape Town"
	// Results are ranked by the best trigram similarity score across title/venue/city.
	args := []interface{}{query, models.EventPublished}
	whereClauses := []string{
		`e.status = $2`,
		`(
			e.search_vector @@ to_tsquery('simple',
				regexp_replace(
					regexp_replace(trim($1), '[^a-zA-Z0-9\s]', '', 'g'),
					'\s+', ':* & ', 'g'
				) || ':*'
			)
			OR word_similarity($1, e.title)      > 0.2
			OR word_similarity($1, e.venue_name) > 0.2
			OR word_similarity($1, e.city)       > 0.2
		)`,
	}

	if cursor != nil {
		startTimeIndex := len(args) + 1
		idIndex := len(args) + 2
		whereClauses = append(whereClauses, fmt.Sprintf(
			"(e.start_time < $%d OR (e.start_time = $%d AND e.id < $%d))",
			startTimeIndex, startTimeIndex, idIndex,
		))
		args = append(args, cursor.StartTime, cursor.ID)
	}

	finalWhere := "WHERE " + strings.Join(whereClauses, " AND ")
	limitIndex := len(args) + 1
	args = append(args, limit+1)

	querySQL := fmt.Sprintf(`
        SELECT
            e.id, COALESCE(e.slug, '') AS slug, e.title, e.description, e.cover_image_url, e.banner_image_url,
            e.start_time, e.end_time, e.price, e.currency,
            e.city, e.venue_name, c.name as category
        FROM events e
        JOIN categories c ON e.category_id = c.id
        %s
        ORDER BY
            GREATEST(
                word_similarity($1, e.title),
                word_similarity($1, e.venue_name),
                word_similarity($1, e.city)
            ) DESC,
            e.start_time DESC,
            e.id DESC
        LIMIT $%d
    `, finalWhere, limitIndex)

	rows, err := pool.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit)
	for rows.Next() {
		var event models.Event
		err = rows.Scan(
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL, &event.BannerImageURL,
			&event.StartTime, &event.EndTime, &event.Price, &event.Currency,
			&event.City, &event.VenueName, &event.Category,
		)
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{StartTime: last.StartTime, ID: last.ID}
		events = events[:limit]
	}
	return events, nextCursor, nil
}

func GetEventTitlesByIDs(pool *pgxpool.Pool, ids []string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	rows, err := pool.Query(ctx, `SELECT id, title FROM events WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch event titles: %w", err)
	}
	defer rows.Close()

	titles := make(map[string]string, len(ids))
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, fmt.Errorf("failed to scan event title row: %w", err)
		}
		titles[id] = title
	}
	return titles, nil
}

func GetOrganizerEvents(pool *pgxpool.Pool, organizerID string, status string) ([]OrganizerEventSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const baseQuery = `
		SELECT id, COALESCE(slug, '') AS slug, title, description, start_time, status, tickets_sold,
       		max_capacity, price, currency, created_at
		FROM events
		WHERE organizer_id = $1
	`

	var (
		rows interface {
			Next() bool
			Scan(...any) error
			Close()
		}
		err error
	)

	if status != "" && status != "all" {
		rows, err = pool.Query(ctx, baseQuery+` AND status = $2 ORDER BY start_time DESC`, organizerID, status)
	} else {
		rows, err = pool.Query(ctx, baseQuery+` ORDER BY start_time DESC`, organizerID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query organizer events: %w", err)
	}
	defer rows.Close()

	events := make([]OrganizerEventSummary, 0)
	for rows.Next() {
		var e OrganizerEventSummary
		if err := rows.Scan(
			&e.ID, &e.Slug, &e.Title, &e.Description, &e.StartTime,
			&e.Status, &e.TicketsSold, &e.MaxCapacity,
			&e.Price, &e.Currency, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan organizer event row: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}
// GetEventsByIDs fetches full event details for an arbitrary slice of event
// IDs. Order is preserved to match the caller's ID ordering (e.g. most-recent-
// first from a recently-viewed list). Up to 20 IDs are accepted; the slice is
// silently truncated beyond that.
//
// Used by GET /v1/events/by-ids (recently-viewed, client-driven lists).
func GetEventsByIDs(pool *pgxpool.Pool, ids []string) ([]models.Event, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > 20 {
		ids = ids[:20]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Validate that every element is a proper UUID before passing to Postgres.
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return nil, fmt.Errorf("invalid event ID %q: %w", id, err)
		}
	}

	rows, err := pool.Query(ctx, `
		SELECT
			ev.id, ev.title, ev.description, ev.cover_image_url, ev.banner_image_url,
			ev.start_time, ev.end_time, ev.price, ev.currency,
			ev.city, ev.venue_name, ev.max_capacity, ev.tickets_sold,
			ev.is_featured, ev.status, ev.views, ev.created_at,
			c.name AS category
		FROM   events     ev
		JOIN   categories c ON c.id = ev.category_id
		WHERE  ev.id = ANY($1)
		  AND  ev.status = 'published'
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("GetEventsByIDs: %w", err)
	}
	defer rows.Close()

	// Build a map first so we can restore caller-requested ordering.
	byID := make(map[string]models.Event, len(ids))
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Title, &e.Description, &e.CoverImageURL, &e.BannerImageURL,
			&e.StartTime, &e.EndTime, &e.Price, &e.Currency,
			&e.City, &e.VenueName, &e.MaxCapacity, &e.TicketsSold,
			&e.IsFeatured, &e.Status, &e.Views, &e.CreatedAt,
			&e.Category,
		); err != nil {
			return nil, fmt.Errorf("GetEventsByIDs scan: %w", err)
		}
		e.ComputeAvailableTickets()
		byID[e.ID] = e
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Return events in the same order as the input IDs (most-recent-first).
	out := make([]models.Event, 0, len(ids))
	for _, id := range ids {
		if e, ok := byID[id]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}