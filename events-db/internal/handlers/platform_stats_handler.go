package handlers

import (
	"context"
	"net/http"
	"time"

	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Platform GMV and revenue
//
// GMV AND REVENUE ARE DIFFERENT NUMBERS AND THE DASHBOARD MUST NOT CONFLATE THEM
//
// The old dashboard showed "Total Revenue R47 110", which was SUM(subtotal) —
// the face value of every ticket sold. That is not revenue. Almost all of it
// belongs to organisers; TicketHawk keeps the commission and the booking fee.
// Reporting it as revenue overstates what the business actually earns by
// roughly 16×, which is the kind of number that ends up in a pitch deck.
//
// The fee model, read out of booking_repository.go rather than assumed:
//
//	subtotal      ticket face value                     → organiser's money
//	platform_fee  buyer-paid service fee (R10 × qty)    → OURS
//	organizer_fee organiser-paid commission (6%)        → OURS
//	total_amount  subtotal + platform_fee - discount    → what the buyer pays
//
// Note organizer_fee is NOT part of total_amount: the buyer never sees it, it
// is deducted from the organiser's settlement. So:
//
//	Ticket GMV   = SUM(subtotal)                   value transacted
//	Buyer paid   = SUM(total_amount)               cash through the platform
//	Our revenue  = SUM(platform_fee + organizer_fee)   what we keep
//	Organiser PSR = SUM(subtotal - organizer_fee)      what they keep
//
// MERCH IS DELIBERATELY INCOMPLETE — see MerchRevenueKnown below.
// ─────────────────────────────────────────────────────────────────────────────

type gmvBreakdown struct {
	// ── Volume ───────────────────────────────────────────────────────────────
	TicketGMV float64 `json:"ticket_gmv"`
	MerchGMV  float64 `json:"merch_gmv"`
	TotalGMV  float64 `json:"total_gmv"`
	BuyerPaid float64 `json:"buyer_paid"`

	// ── Our cut ──────────────────────────────────────────────────────────────
	BookingFees   float64 `json:"booking_fees"`   // platform_fee — buyer-paid
	Commission    float64 `json:"commission"`     // organizer_fee — 6% of face
	MerchRevenue  float64 `json:"merch_revenue"`
	OurRevenue    float64 `json:"our_revenue"`
	// take_rate = our_revenue / ticket_gmv. The single number that says whether
	// the business model works; nil rather than 0 when there is no GMV yet,
	// because "we take 0%" and "nothing has sold" are different facts.
	TakeRate *float64 `json:"take_rate"`

	// MerchRevenueKnown is false, and that is a data-model gap rather than a
	// bug in this query.
	//
	// merch_orders stores unit_price and total_amount but NO commission split —
	// unlike bookings, which carry platform_fee and organizer_fee per row. So
	// there is nothing in the database that says what TicketHawk earns on a
	// merch sale, and any figure here would be invented. Merch GMV is reported;
	// merch revenue is reported as unknown until a rate is stored.
	MerchRevenueKnown bool `json:"merch_revenue_known"`

	// ── Organiser side ───────────────────────────────────────────────────────
	OrganiserPSR float64 `json:"organiser_psr"`

	// ── Tickets ──────────────────────────────────────────────────────────────
	TicketsSold int64 `json:"tickets_sold"`
	TicketsFree int64 `json:"tickets_free"`
	TicketsPaid int64 `json:"tickets_paid"`

	// ── Events, by the four kinds the organiser form offers ──────────────────
	EventsPublicFree   int64 `json:"events_public_free"`
	EventsPublicPaid   int64 `json:"events_public_paid"`
	EventsPrivateFree  int64 `json:"events_private_free"`
	EventsPrivatePaid  int64 `json:"events_private_paid"`
	EventsPendingCount int64 `json:"events_pending_count"`

	// ── People ───────────────────────────────────────────────────────────────
	TotalUsers  int64 `json:"total_users"`
	ActiveUsers int64 `json:"active_users"`
	Organisers  int64 `json:"organisers"`
}

// GET /admin/stats/gmv
func GetPlatformGMVHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()

		var b gmvBreakdown

		// ── Tickets ──────────────────────────────────────────────────────────
		// Confirmed bookings only, matching every other revenue surface in the
		// product so the admin console and the organiser dashboard agree.
		err := pool.QueryRow(ctx, `
			SELECT
				COALESCE(SUM(subtotal), 0)::numeric,
				COALESCE(SUM(total_amount), 0)::numeric,
				COALESCE(SUM(platform_fee), 0)::numeric,
				COALESCE(SUM(organizer_fee), 0)::numeric,
				COALESCE(SUM(ticket_quantity), 0)::bigint,
				-- Free vs paid is decided per BOOKING by its unit price, not by
				-- the event's headline price: an event with a free tier and a
				-- VIP tier is both, and bucketing by the event would put every
				-- VIP ticket in the wrong column.
				COALESCE(SUM(ticket_quantity) FILTER (WHERE unit_price = 0), 0)::bigint,
				COALESCE(SUM(ticket_quantity) FILTER (WHERE unit_price > 0), 0)::bigint
			FROM bookings
			WHERE booking_status = 'confirmed'
		`).Scan(&b.TicketGMV, &b.BuyerPaid, &b.BookingFees, &b.Commission,
			&b.TicketsSold, &b.TicketsFree, &b.TicketsPaid)
		if err != nil {
			utils.InternalError(c, err, "could not load ticket totals")
			return
		}

		// ── Merch ────────────────────────────────────────────────────────────
		// 'fulfilled' only. An oversold order took the buyer's money but never
		// shipped, so counting it as GMV inflates volume with sales that are
		// heading for a refund.
		if err := pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(total_amount), 0)::numeric
			FROM merch_orders
			WHERE status = 'fulfilled'
		`).Scan(&b.MerchGMV); err != nil {
			utils.InternalError(c, err, "could not load merch totals")
			return
		}
		b.MerchRevenueKnown = false
		b.MerchRevenue = 0

		// ── Derived ──────────────────────────────────────────────────────────
		b.TotalGMV = b.TicketGMV + b.MerchGMV
		b.OurRevenue = b.BookingFees + b.Commission + b.MerchRevenue
		b.OrganiserPSR = b.TicketGMV - b.Commission
		if b.TicketGMV > 0 {
			rate := b.OurRevenue / b.TicketGMV
			b.TakeRate = &rate
		}

		// ── Events by kind ───────────────────────────────────────────────────
		// published = listed publicly; published_private = shareable link only.
		// Crossed with price to give the four kinds the create form offers.
		if err := pool.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE status = 'published'         AND price = 0)::bigint,
				COUNT(*) FILTER (WHERE status = 'published'         AND price > 0)::bigint,
				COUNT(*) FILTER (WHERE status = 'published_private' AND price = 0)::bigint,
				COUNT(*) FILTER (WHERE status = 'published_private' AND price > 0)::bigint,
				COUNT(*) FILTER (WHERE status = 'pending_review')::bigint
			FROM events
		`).Scan(&b.EventsPublicFree, &b.EventsPublicPaid,
			&b.EventsPrivateFree, &b.EventsPrivatePaid, &b.EventsPendingCount); err != nil {
			utils.InternalError(c, err, "could not load event counts")
			return
		}

		// ── People ───────────────────────────────────────────────────────────
		// "Active" = has confirmed a booking in the last 90 days.
		//
		// Deliberately activity, not last_login: someone who signs in and buys
		// nothing is not an active customer of a ticketing platform, and
		// last_login would flatter the number with people who opened the app
		// once. 90 days because event-going is seasonal — a 30-day window would
		// mark a regular festival-goer inactive every winter.
		if err := pool.QueryRow(ctx, `
			SELECT
				(SELECT COUNT(*) FROM users)::bigint,
				(SELECT COUNT(DISTINCT user_id) FROM bookings
				  WHERE booking_status = 'confirmed'
				    AND confirmed_at > now() - INTERVAL '90 days'
				    AND user_id IS NOT NULL)::bigint,
				(SELECT COUNT(*) FROM organizers)::bigint
		`).Scan(&b.TotalUsers, &b.ActiveUsers, &b.Organisers); err != nil {
			utils.InternalError(c, err, "could not load user counts")
			return
		}

		c.JSON(http.StatusOK, b)
	}
}

// ── Dashboard lists ──────────────────────────────────────────────────────────

type dashEvent struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	StartTime   time.Time `json:"start_time"`
	City        *string   `json:"city"`
	Price       float64   `json:"price"`
	TicketsSold int64     `json:"tickets_sold"`
	Revenue     float64   `json:"revenue"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// GET /admin/dashboard/lists
//
// The two five-row lists under the stats. One call rather than two, because
// they render side by side and two requests means two loading states that
// finish at different moments.
func GetDashboardListsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		query := func(where, order string) ([]dashEvent, error) {
			rows, err := pool.Query(ctx, `
				SELECT e.id::text, e.title, e.status::text, e.start_time, e.city,
				       e.price, COALESCE(e.tickets_sold, 0)::bigint,
				       COALESCE((SELECT SUM(b.subtotal) FROM bookings b
				                 WHERE b.event_id = e.id AND b.booking_status = 'confirmed'), 0)::numeric,
				       e.created_at
				FROM events e
				WHERE `+where+`
				ORDER BY `+order+`
				LIMIT 5
			`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			out := []dashEvent{}
			for rows.Next() {
				var d dashEvent
				if err := rows.Scan(&d.ID, &d.Title, &d.Status, &d.StartTime, &d.City,
					&d.Price, &d.TicketsSold, &d.Revenue, &d.SubmittedAt); err != nil {
					return nil, err
				}
				out = append(out, d)
			}
			return out, rows.Err()
		}

		recent, err := query("TRUE", "e.created_at DESC")
		if err != nil {
			utils.InternalError(c, err, "could not load recent events")
			return
		}

		// Oldest first, not newest: an approval queue is a queue, and the event
		// that has been waiting longest is the one closest to its own start
		// date. Sorting newest-first buries exactly the ones that are urgent.
		pending, err := query("e.status = 'pending_review'", "e.created_at ASC")
		if err != nil {
			utils.InternalError(c, err, "could not load pending events")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"recent":  recent,
			"pending": pending,
		})
	}
}