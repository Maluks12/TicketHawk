// ── Slug addition to admin.repository.go ─────────────────────────────────────
//
// Two changes in this file:
//   1. AdminEventRow gains a Slug field.
//   2. GetAdminEvents includes e.slug in the SELECT and Scan.
//
// Only the changed structs and functions are shown below. The rest of the file
// (GetAdminStats, GetOversoldMerchOrders, etc.) is unchanged.
// ─────────────────────────────────────────────────────────────────────────────

package repository

import (
	"context"
	"fmt"
	"time"

	"events-db/internal/models"
	"events-db/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AdminStats is the payload returned by GET /admin/stats.
type AdminStats struct {
	TotalEvents        int     `json:"total_events"`
	ActiveEvents       int     `json:"active_events"`
	// Approval queue badge counts
	PendingEvents      int     `json:"pending_events"` // events awaiting review
	PendingMerch       int     `json:"pending_merch"`  // merch items awaiting review
	TotalUsers         int     `json:"total_users"`
	TotalBookings      int     `json:"total_bookings"`
	ConfirmedBookings  int     `json:"confirmed_bookings"`
	TicketRevenue      float64 `json:"ticket_revenue"`
	TicketsToday       int     `json:"tickets_today"`
	TicketRevenueToday float64 `json:"ticket_revenue_today"`

	MerchUnitsSold      int     `json:"merch_units_sold"`
	MerchRevenue        float64 `json:"merch_revenue"`
	MerchOrdersToday    int     `json:"merch_orders_today"`
	MerchRevenueToday   float64 `json:"merch_revenue_today"`
	OversoldMerchOrders int     `json:"oversold_merch_orders"`

	TotalRevenue      float64 `json:"total_revenue"`
	TotalRevenueToday float64 `json:"total_revenue_today"`
}

// GetAdminStats is unchanged — included here to keep the file self-contained.
func GetAdminStats(pool *pgxpool.Pool) (*AdminStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var s AdminStats

	err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM events)                                                AS total_events,
			(SELECT COUNT(*) FROM events
			   WHERE status = 'published' AND start_time > NOW())                       AS active_events,
			(SELECT COUNT(*) FROM events WHERE status = 'pending_review')               AS pending_events,
			(SELECT COUNT(*) FROM merchandise WHERE approval_status = 'pending_review') AS pending_merch,
			(SELECT COUNT(*) FROM users)                                                 AS total_users,
			(SELECT COUNT(*) FROM bookings)                                              AS total_bookings,
			(SELECT COUNT(*) FROM bookings WHERE booking_status = 'confirmed')           AS confirmed_bookings,
			(SELECT COALESCE(SUM(total_amount), 0) FROM bookings
			   WHERE booking_status = 'confirmed')                                      AS ticket_revenue,
			(SELECT COUNT(*) FROM bookings WHERE created_at >= CURRENT_DATE)            AS tickets_today,
			(SELECT COALESCE(SUM(total_amount), 0) FROM bookings
			   WHERE booking_status = 'confirmed'
			     AND created_at >= CURRENT_DATE)                                        AS ticket_revenue_today
	`).Scan(
		&s.TotalEvents, &s.ActiveEvents,
		&s.PendingEvents, &s.PendingMerch,
		&s.TotalUsers,
		&s.TotalBookings, &s.ConfirmedBookings,
		&s.TicketRevenue, &s.TicketsToday, &s.TicketRevenueToday,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAdminStats tickets: %w", err)
	}

	err = pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(quantity), 0)                                                  AS merch_units_sold,
			COALESCE(SUM(total_amount) FILTER (WHERE fulfillment_status = 'fulfilled'), 0)
			                                                                            AS merch_revenue,
			COUNT(*) FILTER (WHERE created_at >= CURRENT_DATE)                         AS merch_orders_today,
			COALESCE(SUM(total_amount) FILTER (
			   WHERE fulfillment_status = 'fulfilled'
			     AND created_at >= CURRENT_DATE), 0)                                   AS merch_revenue_today,
			COUNT(*) FILTER (WHERE fulfillment_status = 'oversold')                    AS oversold_merch_orders
		FROM merch_orders
	`).Scan(
		&s.MerchUnitsSold, &s.MerchRevenue,
		&s.MerchOrdersToday, &s.MerchRevenueToday,
		&s.OversoldMerchOrders,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAdminStats merch: %w", err)
	}

	s.TotalRevenue = s.TicketRevenue + s.MerchRevenue
	s.TotalRevenueToday = s.TicketRevenueToday + s.MerchRevenueToday

	return &s, nil
}

// AdminEventRow is a lightweight event summary for the admin events table.
// Slug is included so the admin UI can build direct event links.
type AdminEventRow struct {
	ID             string             `json:"id"`
	Slug           string             `json:"slug,omitempty"`           // ← added
	Title          string             `json:"title"`
	Status         models.EventStatus `json:"status"`
	StartTime      time.Time          `json:"start_time"`
	City           string             `json:"city"`
	Price          float64            `json:"price"`
	Currency       string             `json:"currency"`
	TicketsSold    int                `json:"tickets_sold"`
	MaxCapacity    *int               `json:"max_capacity,omitempty"`
	TicketRevenue  float64            `json:"ticket_revenue"`
	MerchRevenue   float64            `json:"merch_revenue"`
	MerchUnitsSold int                `json:"merch_units_sold"`
	TotalRevenue   float64            `json:"total_revenue"`
	CreatedAt      time.Time          `json:"created_at"`
}

// GetAdminEvents returns a cursor-paginated list of all events with full ticket
// and merch revenue per event. Filters: status, search (title ILIKE).
func GetAdminEvents(
	pool *pgxpool.Pool,
	limit int,
	cursor *utils.Cursor,
	status string,
	search string,
) ([]AdminEventRow, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := []interface{}{}
	where := "WHERE 1=1"
	argN := 1

	if status != "" {
		where += fmt.Sprintf(" AND e.status = $%d", argN)
		args = append(args, status)
		argN++
	}
	if search != "" {
		where += fmt.Sprintf(" AND e.title ILIKE $%d", argN)
		args = append(args, "%"+search+"%")
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
			e.id, COALESCE(e.slug, '') AS slug, e.title, e.status, e.start_time,
			e.city, e.price, e.currency,
			e.tickets_sold, e.max_capacity,
			COALESCE(SUM(b.total_amount) FILTER (
				WHERE b.booking_status = 'confirmed'), 0)           AS ticket_revenue,
			COALESCE(SUM(mo.total_amount) FILTER (
				WHERE mo.fulfillment_status = 'fulfilled'), 0)      AS merch_revenue,
			COALESCE(SUM(mo.quantity) FILTER (
				WHERE mo.fulfillment_status = 'fulfilled'), 0)::int AS merch_units_sold,
			e.created_at
		FROM events e
		LEFT JOIN bookings b      ON b.event_id  = e.id
		LEFT JOIN merch_orders mo ON mo.event_id = e.id
		%s
		GROUP BY e.id
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $%d
	`, where, argN)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("GetAdminEvents: %w", err)
	}
	defer rows.Close()

	var events []AdminEventRow
	for rows.Next() {
		var ev AdminEventRow
		if err := rows.Scan(
    &ev.ID, &ev.Slug, &ev.Title, &ev.Status, &ev.StartTime,
    &ev.City, &ev.Price, &ev.Currency,
    &ev.TicketsSold, &ev.MaxCapacity,
    &ev.TicketRevenue, &ev.MerchRevenue, &ev.MerchUnitsSold,
    &ev.CreatedAt,
	); err != nil {
		return nil, nil, fmt.Errorf("GetAdminEvents scan: %w", err)
	}
		ev.TotalRevenue = ev.TicketRevenue + ev.MerchRevenue
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *utils.Cursor
	if len(events) > limit {
		last := events[limit-1]
		nextCursor = &utils.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		events = events[:limit]
	}

	if events == nil {
		events = []AdminEventRow{}
	}
	return events, nextCursor, nil
}

// GetUserProfile returns the public profile for the authenticated user.
func GetUserProfile(pool *pgxpool.Pool, userID string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var u models.User

	// has_password and the linked provider list are computed here rather than
	// in a second round trip: the profile screen needs all three to decide what
	// to render (whether to demand a current password for an email change,
	// whether to offer "Set a password", which provider button to show), and
	// splitting them means the screen can render in a state that contradicts
	// itself while the second call is still in flight.
	//
	// password IS NOT NULL is a boolean ABOUT the hash — the hash itself never
	// leaves this query.
	var providers []string
	err := pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.first_name, u.last_name, u.phone_number, u.role, u.created_at,
		       (u.password IS NOT NULL AND u.password <> '') AS has_password,
		       COALESCE(
		           ARRAY(
		               SELECT ui.provider FROM user_identities ui
		               WHERE ui.user_id = u.id ORDER BY ui.created_at
		           ),
		           '{}'
		       ) AS linked_providers
		FROM users u WHERE u.id = $1
	`, userID).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName,
		&u.PhoneNumber, &u.Role, &u.CreatedAt,
		&u.HasPassword, &providers,
	)
	if err != nil {
		return nil, fmt.Errorf("GetUserProfile: %w", err)
	}
	if providers == nil {
		providers = []string{}
	}
	u.LinkedProviders = providers
	return &u, nil
}