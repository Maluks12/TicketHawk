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

type OrganizerEventSummary struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
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
            refund_policy, terms_and_conditions, tags,
            organizer_id, status
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
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
		dto.RefundPolicy, dto.TermsAndConditions, dto.Tags,
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

func PublishEvent(pool *pgxpool.Pool, eventID string, organizerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := pool.Exec(ctx, `
		UPDATE events
		SET status = $1, published_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND organizer_id = $3 AND status = $4
	`, models.EventPublished, eventID, organizerID, models.EventDraft)
	if err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("event not found or already published")
	}
	return nil
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

	var bookingCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM bookings WHERE event_id = $1`, eventID,
	).Scan(&bookingCount); err != nil {
		return fmt.Errorf("failed to check bookings: %w", err)
	}
	if bookingCount > 0 {
		return fmt.Errorf("cannot delete event with existing bookings")
	}

	cmd, err := pool.Exec(ctx, `
		DELETE FROM events WHERE id = $1 AND organizer_id = $2 AND status = $3
	`, eventID, organizerID, models.EventDraft)
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
		return fmt.Errorf("event not found or cannot be cancelled")
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
				fmt.Sprintf("Unfortunately, %s has been cancelled. You will be refunded automatically.", reason),
				map[string]string{"event_id": eventID},
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
            e.id, e.slug, e.title, e.description, e.cover_image_url,
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
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL,
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
            e.allow_waitlist, e.is_featured, e.featured_until, e.status,
            e.refund_policy, e.terms_and_conditions, e.tags, e.views,
            e.organizer_id, e.created_at, e.updated_at,
            c.name as category
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
        &event.AllowWaitlist, &event.IsFeatured, &event.FeaturedUntil, &event.Status,
        &event.RefundPolicy, &event.TermsAndConditions, &event.Tags, &event.Views,
        &event.OrganizerID, &event.CreatedAt, &event.UpdatedAt,
        &event.Category,
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
            e.allow_waitlist, e.is_featured, e.featured_until, e.status,
            e.refund_policy, e.terms_and_conditions, e.tags, e.views,
            e.organizer_id, e.created_at, e.updated_at,
            c.name as category
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
		&event.AllowWaitlist, &event.IsFeatured, &event.FeaturedUntil, &event.Status,
		&event.RefundPolicy, &event.TermsAndConditions, &event.Tags, &event.Views,
		&event.OrganizerID, &event.CreatedAt, &event.UpdatedAt,
		&event.Category,
	)
	if err != nil {
		return nil, ErrEventNotFound
	}

	event.ComputeAvailableTickets()
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
            e.id, e.slug, e.title, e.description, e.cover_image_url,
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
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL,
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
            e.id, e.slug, e.title, e.description, e.cover_image_url,
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
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL,
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
            e.id, e.slug, e.title, e.description, e.cover_image_url,
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
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL,
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
            e.id, e.slug, e.title, e.description, e.cover_image_url,
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
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL,
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

func SearchEvents(pool *pgxpool.Pool, query string, limit int, cursor *utils.Cursor) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{query, models.EventPublished}
	whereClauses := []string{
		"e.search_vector @@ plainto_tsquery('english', $1)",
		"e.status = $2",
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
            e.id, e.slug, e.title, e.description, e.cover_image_url,
            e.start_time, e.end_time, e.price, e.currency,
            e.city, e.venue_name, c.name as category
        FROM events e
        JOIN categories c ON e.category_id = c.id
        %s
        ORDER BY
            ts_rank(e.search_vector, plainto_tsquery('english', $1)) DESC,
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
			&event.ID, &event.Slug, &event.Title, &event.Description, &event.CoverImageURL,
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
		SELECT id, slug, title, description, start_time, status, tickets_sold,
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