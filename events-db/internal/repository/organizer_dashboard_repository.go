package repository

import (
	"context"
	"fmt"
	"time"

	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Overview stats ────────────────────────────────────────────────────────────

// OrganizerStats is the payload for GET /organiser/dashboard/stats.
// All figures are scoped to events owned by the organiser.
type OrganizerStats struct {
	// Event counts
	TotalEvents         int `json:"total_events"`
	PublishedEvents     int `json:"published_events"`
	PendingReviewEvents int `json:"pending_review_events"` // awaiting admin approval
	RejectedEvents      int `json:"rejected_events"`       // admin rejected; can be revised
	DraftEvents         int `json:"draft_events"`
	CancelledEvents     int `json:"cancelled_events"`

	// Ticket sales
	TotalBookings     int     `json:"total_bookings"`      // confirmed only
	TotalTicketsSold  int     `json:"total_tickets_sold"`  // sum of ticket_quantity
	GrossTicketRevenue float64 `json:"gross_ticket_revenue"` // sum of subtotal (before fees)
	PlatformFeesPaid  float64 `json:"platform_fees_paid"`   // sum of platform_fee
	NetTicketEarnings float64 `json:"net_ticket_earnings"`  // gross - platform_fee

	// Today's ticket sales
	BookingsToday     int     `json:"bookings_today"`
	RevenueToday      float64 `json:"revenue_today"` // net today

	// Merch sales
	MerchUnitsSold  int     `json:"merch_units_sold"`
	GrossMerchRevenue float64 `json:"gross_merch_revenue"`
	// Merch has no platform fee in the current schema — the full amount is
	// the organiser's. We still surface gross so they can see the full picture.

	// Combined net (what the organiser earns after platform fees)
	TotalNetEarnings float64 `json:"total_net_earnings"`
}

// GetOrganizerStats returns the aggregated dashboard overview for one organiser.
func GetOrganizerStats(pool *pgxpool.Pool, organizerID string) (*OrganizerStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var s OrganizerStats

	// Event counts
	err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*)                                                  AS total_events,
			COUNT(*) FILTER (WHERE status = 'published')             AS published_events,
			COUNT(*) FILTER (WHERE status = 'pending_review')        AS pending_review_events,
			COUNT(*) FILTER (WHERE status = 'rejected')              AS rejected_events,
			COUNT(*) FILTER (WHERE status = 'draft')                 AS draft_events,
			COUNT(*) FILTER (WHERE status = 'cancelled')             AS cancelled_events
		FROM events
		WHERE organizer_id = $1
	`, organizerID).Scan(
		&s.TotalEvents, &s.PublishedEvents,
		&s.PendingReviewEvents, &s.RejectedEvents,
		&s.DraftEvents, &s.CancelledEvents,
	)
	if err != nil {
		return nil, fmt.Errorf("OrganizerStats events: %w", err)
	}

	// Ticket revenue — confirmed bookings only, scoped to this organiser's events
	err = pool.QueryRow(ctx, `
		SELECT
			COUNT(b.id)                         AS total_bookings,
			COALESCE(SUM(b.ticket_quantity), 0) AS total_tickets_sold,
			COALESCE(SUM(b.subtotal), 0)        AS gross_ticket_revenue,
			COALESCE(SUM(b.organizer_fee), 0)   AS platform_fees_paid,
			COUNT(b.id) FILTER (WHERE b.confirmed_at >= CURRENT_DATE)   AS bookings_today,
			COALESCE(SUM(b.subtotal - b.organizer_fee)
				FILTER (WHERE b.confirmed_at >= CURRENT_DATE), 0)       AS revenue_today
		FROM bookings b
		JOIN events e ON e.id = b.event_id
		WHERE e.organizer_id   = $1
		  AND b.booking_status = 'confirmed'
	`, organizerID).Scan(
		&s.TotalBookings,
		&s.TotalTicketsSold,
		&s.GrossTicketRevenue,
		&s.PlatformFeesPaid,
		&s.BookingsToday,
		&s.RevenueToday,
	)
	if err != nil {
		return nil, fmt.Errorf("OrganizerStats bookings: %w", err)
	}
	s.NetTicketEarnings = s.GrossTicketRevenue - s.PlatformFeesPaid

	// Merch revenue — fulfilled orders only
	err = pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(mo.quantity), 0)      AS merch_units_sold,
			COALESCE(SUM(mo.total_amount), 0)  AS gross_merch_revenue
		FROM merch_orders mo
		JOIN events e ON e.id = mo.event_id
		WHERE e.organizer_id          = $1
		  AND mo.fulfillment_status   = 'fulfilled'
	`, organizerID).Scan(&s.MerchUnitsSold, &s.GrossMerchRevenue)
	if err != nil {
		return nil, fmt.Errorf("OrganizerStats merch: %w", err)
	}

	s.TotalNetEarnings = s.NetTicketEarnings + s.GrossMerchRevenue
	return &s, nil
}

// ── Event list with revenue ───────────────────────────────────────────────────

// OrganizerEventRow is a richer version of OrganizerEventSummary that adds
// per-event revenue figures. Replaces the basic list returned by
// GetOrganizerEvents.
type OrganizerEventRow struct {
	ID          string             `json:"id"`
	Slug        string             `json:"slug"`
	Title       string             `json:"title"`
	Status      models.EventStatus `json:"status"`
	StartTime   time.Time          `json:"start_time"`
	EndTime     *time.Time         `json:"end_time"`
	City        string             `json:"city"`
	VenueName   string             `json:"venue_name"`
	Views       int                `json:"views"`
	Price       float64            `json:"price"`
	Currency    string             `json:"currency"`
	MaxCapacity *int               `json:"max_capacity,omitempty"`
	TicketsSold int                `json:"tickets_sold"`

	// Revenue (confirmed bookings only)
	TotalBookings      int     `json:"total_bookings"`
	GrossTicketRevenue float64 `json:"gross_ticket_revenue"`
	PlatformFeesPaid   float64 `json:"platform_fees_paid"`
	NetTicketEarnings  float64 `json:"net_ticket_earnings"`

	// Merch (fulfilled orders only)
	MerchUnitsSold    int     `json:"merch_units_sold"`
	GrossMerchRevenue float64 `json:"gross_merch_revenue"`

	// Combined
	TotalNetEarnings float64 `json:"total_net_earnings"`

	CreatedAt time.Time `json:"created_at"`
}

// GetOrganizerEventRows returns all events for an organiser with full revenue
// attached via LEFT JOIN aggregates. Filterable by status.
func GetOrganizerEventRows(pool *pgxpool.Pool, organizerID, status string, limit int, cursor *utils.Cursor) ([]OrganizerEventRow, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := []interface{}{organizerID}
	argN := 2
	where := "WHERE e.organizer_id = $1"

	if status != "" && status != "all" {
		where += fmt.Sprintf(" AND e.status = $%d", argN)
		args = append(args, status)
		argN++
	}
	if cursor != nil {
		where += fmt.Sprintf(
			" AND (e.created_at < $%d OR (e.created_at = $%d AND e.id < $%d))",
			argN, argN+1, argN+2,
		)
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
		argN += 3
	}

	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			e.id, e.slug, e.title, e.status, e.start_time, e.end_time,
			e.city, COALESCE(e.venue_name, ''), e.price, e.currency,
			e.max_capacity, e.tickets_sold, COALESCE(e.views, 0),
			COALESCE(COUNT(b.id)  FILTER (WHERE b.booking_status = 'confirmed'), 0)::int,
			COALESCE(SUM(b.subtotal)     FILTER (WHERE b.booking_status = 'confirmed'), 0),
			COALESCE(SUM(b.organizer_fee) FILTER (WHERE b.booking_status = 'confirmed'), 0),
			COALESCE(SUM(mo.quantity)    FILTER (WHERE mo.fulfillment_status = 'fulfilled'), 0)::int,
			COALESCE(SUM(mo.total_amount)FILTER (WHERE mo.fulfillment_status = 'fulfilled'), 0),
			e.created_at
		FROM events e
		LEFT JOIN bookings    b  ON b.event_id  = e.id
		LEFT JOIN merch_orders mo ON mo.event_id = e.id
		%s
		GROUP BY e.id
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $%d
	`, where, argN)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("GetOrganizerEventRows: %w", err)
	}
	defer rows.Close()

	var events []OrganizerEventRow
	for rows.Next() {
		var ev OrganizerEventRow
		var grossTicket, platformFee, grossMerch float64
		if err := rows.Scan(
			&ev.ID, &ev.Slug, &ev.Title, &ev.Status, &ev.StartTime, &ev.EndTime,
			&ev.City, &ev.VenueName, &ev.Price, &ev.Currency,
			&ev.MaxCapacity, &ev.TicketsSold, &ev.Views,
			&ev.TotalBookings,
			&grossTicket, &platformFee,
			&ev.MerchUnitsSold, &grossMerch,
			&ev.CreatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("GetOrganizerEventRows scan: %w", err)
		}
		ev.GrossTicketRevenue = grossTicket
		ev.PlatformFeesPaid = platformFee
		ev.NetTicketEarnings = grossTicket - platformFee
		ev.GrossMerchRevenue = grossMerch
		ev.TotalNetEarnings = ev.NetTicketEarnings + grossMerch
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	if events == nil {
		events = []OrganizerEventRow{}
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		events = events[:limit]
	}
	return events, nextCursor, nil
}

// ── Single-event stats ────────────────────────────────────────────────────────

// OrganizerEventStats is the full financial breakdown for one event.
type OrganizerEventStats struct {
	EventID       string             `json:"event_id"`
	Slug          string             `json:"slug"`
	Visibility    string             `json:"visibility"`
	Title         string             `json:"title"`
	Status        models.EventStatus `json:"status"`
	StartTime     time.Time          `json:"start_time"`
	EndTime       time.Time          `json:"end_time"`
	City          string             `json:"city"`
	VenueName     string             `json:"venue_name"`
	Price         float64            `json:"price"`
	Currency      string             `json:"currency"`
	Views         int                `json:"views"`
	CoverImageURL *string            `json:"cover_image_url"`
	BannerImageURL *string           `json:"banner_image_url"`

	// ── Full event record ────────────────────────────────────────────────
	// The organiser dashboard's event page is the one place an organiser can
	// review everything they entered, so this endpoint returns the complete
	// row rather than the handful of headline fields it used to.
	Description        string     `json:"description"`
	BannerVideoURL     *string    `json:"banner_video_url"`
	GalleryImages      []string   `json:"gallery_images"`
	Address            string     `json:"address"`
	Timezone           string     `json:"timezone"`
	Latitude           *float64   `json:"latitude"`
	Longitude          *float64   `json:"longitude"`
	VenueType          *string    `json:"venue_type"`
	CategoryID         int        `json:"category_id"`
	Category           string     `json:"category"`
	ExpiresAt          time.Time  `json:"expires_at"`
	AllowWaitlist      bool       `json:"allow_waitlist"`
	MaxTicketsPerUser  *int       `json:"max_tickets_per_user"`
	ScanOpenOffsetMin  *int       `json:"scan_open_offset_min"`
	ScanCloseOffsetMin *int       `json:"scan_close_offset_min"`
	RefundPolicy       *string    `json:"refund_policy"`
	TermsAndConditions *string    `json:"terms_and_conditions"`
	Tags               models.TagArray `json:"tags"`
	Performers         models.TagArray `json:"performers"`
	Teams              models.TagArray `json:"teams"`
	ListingFeePaid     bool       `json:"listing_fee_paid"`
	ListingFeeAmount   float64    `json:"listing_fee_amount"`
	IsFeatured         bool       `json:"is_featured"`
	AdminPromoted      bool       `json:"admin_promoted"`
	RightsDeclaredAt   *time.Time `json:"rights_declared_at"`
	PublishedAt        *time.Time `json:"published_at"`
	RejectionReason    *string    `json:"rejection_reason"`
	CancelledAt        *time.Time `json:"cancelled_at"`
	CancellationReason *string    `json:"cancellation_reason"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	TicketTypes        []*models.TicketType `json:"ticket_types"`

	// Capacity
	MaxCapacity  *int    `json:"max_capacity,omitempty"`
	TicketsSold  int     `json:"tickets_sold"`
	Availability float64 `json:"capacity_used_pct"` // 0–100, -1 if unlimited

	// Ticket financials
	TotalBookings      int     `json:"total_bookings"`       // confirmed
	PendingBookings    int     `json:"pending_bookings"`
	CancelledBookings  int     `json:"cancelled_bookings"`
	RefundedBookings   int     `json:"refunded_bookings"`
	GrossTicketRevenue float64 `json:"gross_ticket_revenue"`
	PlatformFeesPaid   float64 `json:"platform_fees_paid"`
	NetTicketEarnings  float64 `json:"net_ticket_earnings"`
	TotalRefunded      float64 `json:"total_refunded"`

	// Merch financials
	MerchItemsSold    int     `json:"merch_items_sold"`     // distinct SKUs with sales
	MerchUnitsSold    int     `json:"merch_units_sold"`
	GrossMerchRevenue float64 `json:"gross_merch_revenue"`
	OversoldMerchOrders int   `json:"oversold_merch_orders"` // > 0 = action needed

	// Combined
	TotalNetEarnings float64 `json:"total_net_earnings"`
}

// GetOrganizerEventStats returns the full financial breakdown for one event,
// verified to belong to the requesting organiser.
func GetOrganizerEventStats(pool *pgxpool.Pool, eventID, organizerID string) (*OrganizerEventStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var s OrganizerEventStats

	// gallery_images is JSONB. Scan it as raw bytes and parse with the shared
	// helper rather than binding straight to a custom Scanner — pgx v5 delivers
	// these in binary and a hand-rolled Scan is how the tags/performers/teams
	// columns silently returned empty for months.
	var rawGallery []byte

	// Verify ownership and pull the full event record. The organiser owns this
	// data and the dashboard is where they review it, so select everything
	// rather than a headline subset.
	err := pool.QueryRow(ctx, `
		SELECT e.id, e.slug, e.visibility, e.title, e.status, e.start_time, e.end_time,
		       e.city, e.venue_name, e.price, e.currency, e.max_capacity, e.tickets_sold,
		       COALESCE(e.views, 0), e.cover_image_url, e.banner_image_url,
		       e.description, e.banner_video_url, e.gallery_images,
		       e.address, e.timezone, e.latitude, e.longitude, e.venue_type,
		       e.category_id, COALESCE(c.name, ''), e.expires_at,
		       e.allow_waitlist, e.max_tickets_per_user,
		       e.scan_open_offset_min, e.scan_close_offset_min,
		       e.refund_policy, e.terms_and_conditions,
		       e.tags, e.performers, e.teams,
		       e.listing_fee_paid, e.listing_fee_amount, e.is_featured, e.admin_promoted,
		       e.rights_declared_at, e.published_at, e.rejection_reason,
		       e.cancelled_at, e.cancellation_reason,
		       e.created_at, e.updated_at
		FROM   events e
		LEFT   JOIN categories c ON e.category_id = c.id
		WHERE  e.id = $1 AND e.organizer_id = $2
	`, eventID, organizerID).Scan(
		&s.EventID, &s.Slug, &s.Visibility, &s.Title, &s.Status, &s.StartTime, &s.EndTime,
		&s.City, &s.VenueName, &s.Price, &s.Currency,
		&s.MaxCapacity, &s.TicketsSold, &s.Views, &s.CoverImageURL, &s.BannerImageURL,
		&s.Description, &s.BannerVideoURL, &rawGallery,
		&s.Address, &s.Timezone, &s.Latitude, &s.Longitude, &s.VenueType,
		&s.CategoryID, &s.Category, &s.ExpiresAt,
		&s.AllowWaitlist, &s.MaxTicketsPerUser,
		&s.ScanOpenOffsetMin, &s.ScanCloseOffsetMin,
		&s.RefundPolicy, &s.TermsAndConditions,
		&s.Tags, &s.Performers, &s.Teams,
		&s.ListingFeePaid, &s.ListingFeeAmount, &s.IsFeatured, &s.AdminPromoted,
		&s.RightsDeclaredAt, &s.PublishedAt, &s.RejectionReason,
		&s.CancelledAt, &s.CancellationReason,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, ErrEventNotFound
	}

	s.GalleryImages = parseJSONBStrings(rawGallery)

	// Pricing tiers. Non-fatal: an event may legitimately have none (flat
	// price), and a tier-loading hiccup shouldn't blank the whole page.
	if tts, ttErr := GetTicketTypesByEventID(pool, eventID); ttErr == nil {
		s.TicketTypes = tts
	}
	if s.TicketTypes == nil {
		s.TicketTypes = []*models.TicketType{}
	}

	if s.MaxCapacity != nil && *s.MaxCapacity > 0 {
		s.Availability = float64(s.TicketsSold) / float64(*s.MaxCapacity) * 100
	} else {
		s.Availability = -1 // unlimited
	}

	// Booking breakdown
	// Note: refunded_amount lives on the payments table, not bookings.
	// Pull it via a subquery to avoid a column-not-found error.
	err = pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE booking_status = 'confirmed')                       AS confirmed,
			COUNT(*) FILTER (WHERE booking_status = 'pending')                         AS pending,
			COUNT(*) FILTER (WHERE booking_status = 'cancelled')                       AS cancelled,
			COUNT(*) FILTER (WHERE booking_status = 'refunded')                        AS refunded,
			COALESCE(SUM(subtotal)     FILTER (WHERE booking_status = 'confirmed'), 0) AS gross_revenue,
			COALESCE(SUM(organizer_fee) FILTER (WHERE booking_status = 'confirmed'), 0) AS platform_fees,
			COALESCE((
				SELECT SUM(p.refunded_amount)
				FROM payments p
				JOIN bookings b2 ON p.booking_id = b2.id
				WHERE b2.event_id = $1 AND p.refunded_amount IS NOT NULL
			), 0)                                                                    AS total_refunded
		FROM bookings
		WHERE event_id = $1
	`, eventID).Scan(
		&s.TotalBookings, &s.PendingBookings, &s.CancelledBookings, &s.RefundedBookings,
		&s.GrossTicketRevenue, &s.PlatformFeesPaid,
		&s.TotalRefunded,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrganizerEventStats bookings: %w", err)
	}
	s.NetTicketEarnings = s.GrossTicketRevenue - s.PlatformFeesPaid

	// Merch breakdown
	err = pool.QueryRow(ctx, `
		SELECT
			COUNT(DISTINCT merchandise_id) FILTER (WHERE fulfillment_status = 'fulfilled')  AS items_sold,
			COALESCE(SUM(quantity)          FILTER (WHERE fulfillment_status = 'fulfilled'), 0) AS units_sold,
			COALESCE(SUM(total_amount)      FILTER (WHERE fulfillment_status = 'fulfilled'), 0) AS gross_merch,
			COUNT(*)                        FILTER (WHERE fulfillment_status = 'oversold')  AS oversold
		FROM merch_orders
		WHERE event_id = $1
	`, eventID).Scan(
		&s.MerchItemsSold, &s.MerchUnitsSold, &s.GrossMerchRevenue, &s.OversoldMerchOrders,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrganizerEventStats merch: %w", err)
	}

	s.TotalNetEarnings = s.NetTicketEarnings + s.GrossMerchRevenue
	return &s, nil
}

// ── Booking list for an event ─────────────────────────────────────────────────

// OrganizerBookingRow is one row in the attendee/booking list for an event.
// Deliberately omits sensitive payment fields (card details, raw stitch IDs).
type OrganizerBookingRow struct {
	ID            string              `json:"id"`
	BookingNumber string              `json:"booking_number"`
	TicketQty     int                 `json:"ticket_quantity"`
	UnitPrice     float64             `json:"unit_price"`
	Subtotal      float64             `json:"subtotal"`
	PlatformFee   float64             `json:"platform_fee"`
	NetEarning    float64             `json:"net_earning"` // subtotal - platform_fee
	Currency      string              `json:"currency"`
	BookingStatus models.BookingStatus `json:"booking_status"`
	PaymentStatus models.PaymentStatus `json:"payment_status"`
	IsGuest       bool                `json:"is_guest"`

	// Attendee info. Email is intentionally not exposed to organisers —
	// it's attendee PII, not something organisers should have access to.
	AttendeeName  string `json:"attendee_name"`
	AttendeeEmail string `json:"-"`

	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// GetOrganizerEventBookings returns a cursor-paginated booking list for one of
// the organiser's events. Only events owned by the organiser are accessible.
func GetOrganizerEventBookings(
	pool *pgxpool.Pool,
	eventID, organizerID string,
	statusFilter string,
	limit int,
	cursor *utils.Cursor,
) ([]OrganizerBookingRow, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ownership check
	var ownerID string
	if err := pool.QueryRow(ctx,
		`SELECT organizer_id FROM events WHERE id = $1`, eventID,
	).Scan(&ownerID); err != nil || ownerID != organizerID {
		return nil, nil, ErrEventNotFound
	}

	args := []interface{}{eventID}
	argN := 2
	where := "WHERE b.event_id = $1"

	if statusFilter != "" && statusFilter != "all" {
		where += fmt.Sprintf(" AND b.booking_status = $%d", argN)
		args = append(args, statusFilter)
		argN++
	} else {
		// Cancelled bookings aren't attendees — organisers shouldn't see them
		// in the attendee list by default. Explicit status filters (e.g.
		// ?status=cancelled) still work for any future admin/refund tooling.
		where += " AND b.booking_status != 'cancelled'"
	}
	if cursor != nil {
		where += fmt.Sprintf(
			" AND (b.created_at < $%d OR (b.created_at = $%d AND b.id < $%d))",
			argN, argN+1, argN+2,
		)
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
		argN += 3
	}

	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			b.id,
			b.booking_number,
			b.ticket_quantity,
			b.unit_price,
			b.subtotal,
			b.organizer_fee,
			b.currency,
			b.booking_status,
			b.payment_status,
			b.is_guest,
			COALESCE(
				NULLIF(TRIM(COALESCE(u.first_name,'') || ' ' || COALESCE(u.last_name,'')), ''),
				COALESCE(b.attendee_first_name || ' ' || b.attendee_last_name, ''),
				'Guest'
			) AS attendee_name,
			COALESCE(u.email, b.attendee_email, '')  AS attendee_email,
			b.confirmed_at,
			b.created_at
		FROM bookings b
		LEFT JOIN users u ON u.id = b.user_id
		%s
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT $%d
	`, where, argN)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("GetOrganizerEventBookings: %w", err)
	}
	defer rows.Close()

	var bookings []OrganizerBookingRow
	for rows.Next() {
		var bk OrganizerBookingRow
		if err := rows.Scan(
			&bk.ID, &bk.BookingNumber,
			&bk.TicketQty, &bk.UnitPrice, &bk.Subtotal, &bk.PlatformFee,
			&bk.Currency, &bk.BookingStatus, &bk.PaymentStatus, &bk.IsGuest,
			&bk.AttendeeName, &bk.AttendeeEmail,
			&bk.ConfirmedAt, &bk.CreatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("GetOrganizerEventBookings scan: %w", err)
		}
		bk.NetEarning = bk.Subtotal - bk.PlatformFee
		bookings = append(bookings, bk)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	if bookings == nil {
		bookings = []OrganizerBookingRow{}
	}

	var nextCursor *utils.Cursor
	if len(bookings) > limit {
		last := bookings[limit-1]
		nextCursor = &utils.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		bookings = bookings[:limit]
	}
	return bookings, nextCursor, nil
}

// ── Merch sales for an event ──────────────────────────────────────────────────

// OrganizerMerchSaleRow is one row in the per-event merch sales breakdown.
// Grouped by merch item so the organiser sees totals per product, not raw orders.
type OrganizerMerchSaleRow struct {
	MerchandiseID string  `json:"merchandise_id"`
	MerchName     string  `json:"merch_name"`
	VariantID     string  `json:"variant_id"`
	VariantSKU    string  `json:"variant_sku"`
	Attributes    string  `json:"attributes"` // e.g. {"size":"L","colour":"home"}
	UnitsSold     int     `json:"units_sold"`
	UnitPrice     float64 `json:"unit_price"`
	GrossRevenue  float64 `json:"gross_revenue"`
	Oversold      int     `json:"oversold_quantity"` // > 0 means admin action needed
}

// GetOrganizerEventMerchSales returns merch sales grouped by variant for one
// event, verified to belong to the requesting organiser.
func GetOrganizerEventMerchSales(pool *pgxpool.Pool, eventID, organizerID string) ([]OrganizerMerchSaleRow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ownership check
	var ownerID string
	if err := pool.QueryRow(ctx,
		`SELECT organizer_id FROM events WHERE id = $1`, eventID,
	).Scan(&ownerID); err != nil || ownerID != organizerID {
		return nil, ErrEventNotFound
	}

	rows, err := pool.Query(ctx, `
		SELECT
			mo.merchandise_id,
			m.name                                                                  AS merch_name,
			mo.variant_id,
			mv.sku                                                                  AS variant_sku,
			mv.attributes::text                                                     AS attributes,
			COALESCE(SUM(mo.quantity)     FILTER (WHERE mo.fulfillment_status = 'fulfilled'), 0)::int AS units_sold,
			mo.unit_price,
			COALESCE(SUM(mo.total_amount) FILTER (WHERE mo.fulfillment_status = 'fulfilled'), 0)      AS gross_revenue,
			COALESCE(SUM(mo.quantity)     FILTER (WHERE mo.fulfillment_status = 'oversold'),  0)::int AS oversold
		FROM merch_orders mo
		JOIN merchandise         m  ON m.id  = mo.merchandise_id
		JOIN merchandise_variants mv ON mv.id = mo.variant_id
		WHERE mo.event_id = $1
		GROUP BY mo.merchandise_id, m.name, mo.variant_id, mv.sku, mv.attributes, mo.unit_price
		ORDER BY gross_revenue DESC, m.name ASC
	`, eventID)
	if err != nil {
		return nil, fmt.Errorf("GetOrganizerEventMerchSales: %w", err)
	}
	defer rows.Close()

	var sales []OrganizerMerchSaleRow
	for rows.Next() {
		var r OrganizerMerchSaleRow
		if err := rows.Scan(
			&r.MerchandiseID, &r.MerchName,
			&r.VariantID, &r.VariantSKU, &r.Attributes,
			&r.UnitsSold, &r.UnitPrice, &r.GrossRevenue, &r.Oversold,
		); err != nil {
			return nil, fmt.Errorf("GetOrganizerEventMerchSales scan: %w", err)
		}
		sales = append(sales, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if sales == nil {
		sales = []OrganizerMerchSaleRow{}
	}
	return sales, nil
}

// ── Payout summary ────────────────────────────────────────────────────────────

// OrganizerPayoutSummary gives the organiser a clear breakdown of what they
// are owed across all their events, split by settlement state. This is purely
// informational — actual transfers happen outside this system (e.g. via Stitch
// bank transfer or manual payout), but surfacing the numbers here is the
// minimum a dashboard needs.
type OrganizerPayoutSummary struct {
	Currency string `json:"currency"`

	// Total net earnings from confirmed, non-refunded bookings
	TotalTicketEarnings float64 `json:"total_ticket_earnings"` // subtotal - platform_fee
	TotalMerchEarnings  float64 `json:"total_merch_earnings"`
	TotalEarnings       float64 `json:"total_earnings"`

	// Breakdowns
	TotalGrossRevenue  float64 `json:"total_gross_revenue"`  // before platform fees
	TotalPlatformFees  float64 `json:"total_platform_fees"`  // deducted by platform
	TotalRefundsIssued float64 `json:"total_refunds_issued"` // money returned to buyers
}

// GetOrganizerPayoutSummary returns lifetime payout-relevant figures for an organiser.
func GetOrganizerPayoutSummary(pool *pgxpool.Pool, organizerID string) (*OrganizerPayoutSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var p OrganizerPayoutSummary
	p.Currency = models.DefaultCurrency

	err := pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(b.subtotal)      FILTER (WHERE b.booking_status = 'confirmed'), 0) AS gross_revenue,
			COALESCE(SUM(b.organizer_fee)  FILTER (WHERE b.booking_status = 'confirmed'), 0) AS platform_fees,
			COALESCE(SUM(b.refunded_amount) FILTER (
				WHERE b.booking_status IN ('refunded','cancelled')
				  AND b.refunded_amount IS NOT NULL), 0)                                    AS refunds_issued
		FROM bookings b
		JOIN events e ON e.id = b.event_id
		WHERE e.organizer_id = $1
	`, organizerID).Scan(&p.TotalGrossRevenue, &p.TotalPlatformFees, &p.TotalRefundsIssued)
	if err != nil {
		return nil, fmt.Errorf("GetOrganizerPayoutSummary tickets: %w", err)
	}
	p.TotalTicketEarnings = p.TotalGrossRevenue - p.TotalPlatformFees

	err = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(mo.total_amount), 0)
		FROM merch_orders mo
		JOIN events e ON e.id = mo.event_id
		WHERE e.organizer_id        = $1
		  AND mo.fulfillment_status = 'fulfilled'
	`, organizerID).Scan(&p.TotalMerchEarnings)
	if err != nil {
		return nil, fmt.Errorf("GetOrganizerPayoutSummary merch: %w", err)
	}

	p.TotalEarnings = p.TotalTicketEarnings + p.TotalMerchEarnings
	return &p, nil
}