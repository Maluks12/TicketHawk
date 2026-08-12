package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Platform analytics time series
//
// The admin equivalent of the organiser's /analytics/series, and it reuses the
// same three decisions because they were right there too:
//
//   • DENSE output. Every bucket in range is present, zero-filled. A sparse
//     series makes a quiet Tuesday and a missing Tuesday identical, and any
//     line chart drawn from it connects across the gap and implies activity
//     that never happened.
//
//   • Bucketed in the DISPLAY zone, not UTC. On UTC days every sale after
//     22:00 SAST lands on tomorrow, which for a product full of evening events
//     misfiles a large share of the dataset.
//
//   • Point count is CLAMPED rather than rejected. Asking for a year by hour
//     returns the most recent slice with a note, instead of a validation error
//     that makes the caller guess a working range.
//
// What differs from the organiser version is scope and money. This one is
// platform-wide and separates GMV from revenue, because on the admin side the
// distinction is the whole point (see platform_stats_handler.go).
// ─────────────────────────────────────────────────────────────────────────────

const adminDefaultTZ = "Africa/Johannesburg"

// maxAdminPoints caps one response. 400 points is more than a chart of this
// width can resolve; past that the extra rows cost bytes and legibility with
// nothing gained.
const maxAdminPoints = 400

type adminSeriesPoint struct {
	Bucket time.Time `json:"bucket"`

	// Money
	TicketGMV  float64 `json:"ticket_gmv"`
	MerchGMV   float64 `json:"merch_gmv"`
	OurRevenue float64 `json:"our_revenue"` // booking fees + commission

	// Volume
	Bookings    int64 `json:"bookings"`
	TicketsSold int64 `json:"tickets_sold"`
	TicketsFree int64 `json:"tickets_free"`
	TicketsPaid int64 `json:"tickets_paid"`

	// Growth
	Signups       int64 `json:"signups"`
	EventsCreated int64 `json:"events_created"`
}

// GET /admin/analytics/series?granularity=day|hour&from=&to=&tz=
func GetAdminAnalyticsSeriesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		gran := c.DefaultQuery("granularity", "day")
		if gran != "day" && gran != "hour" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "granularity must be day or hour"})
			return
		}

		tz := c.DefaultQuery("tz", adminDefaultTZ)
		// Validated before it reaches SQL: an unknown zone makes AT TIME ZONE
		// throw, which would surface as a 500 for what is a bad request.
		if _, err := time.LoadLocation(tz); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown timezone"})
			return
		}

		to := time.Now()
		if raw := c.Query("to"); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "to must be RFC3339"})
				return
			}
			to = t
		}

		step := 24 * time.Hour
		if gran == "hour" {
			step = time.Hour
		}
		from := to.Add(-90 * 24 * time.Hour)
		if gran == "hour" {
			from = to.Add(-14 * 24 * time.Hour)
		}
		if raw := c.Query("from"); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "from must be RFC3339"})
				return
			}
			from = t
		}
		if !from.Before(to) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from must be before to"})
			return
		}

		// Clamp by moving the START forward, keeping the end. Asked for a year
		// by hour, what you want is the recent detail — not the first fortnight
		// of last year.
		var clamped bool
		var note string
		if maxSpan := step * time.Duration(maxAdminPoints); to.Sub(from) > maxSpan {
			from = to.Add(-maxSpan)
			clamped = true
			note = fmt.Sprintf("%s grain is capped at %d points — showing the most recent %s",
				gran, maxAdminPoints, maxSpan.Round(time.Hour))
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
		defer cancel()

		// One statement. `grid` generates every bucket in the display zone and
		// each source is aggregated to the same expression, then LEFT JOINed —
		// which is what makes the output dense without a second pass in Go.
		//
		// Four separate CTEs rather than joins between the source tables: a
		// booking and a signup have nothing to join ON, and forcing them
		// together would multiply rows before the GROUP BY.
		const q = `
			WITH grid AS (
				SELECT generate_series(
					date_trunc($1::text, $2::timestamptz AT TIME ZONE $4::text),
					date_trunc($1::text, $3::timestamptz AT TIME ZONE $4::text),
					('1 ' || $1::text)::interval
				) AS b
			),
			bk AS (
				SELECT date_trunc($1::text, confirmed_at AT TIME ZONE $4::text) AS b,
				       COUNT(*)                                   AS bookings,
				       SUM(subtotal)                              AS ticket_gmv,
				       SUM(platform_fee + organizer_fee)          AS our_revenue,
				       SUM(ticket_quantity)                       AS tickets_sold,
				       SUM(ticket_quantity) FILTER (WHERE unit_price = 0) AS tickets_free,
				       SUM(ticket_quantity) FILTER (WHERE unit_price > 0) AS tickets_paid
				FROM bookings
				WHERE booking_status = 'confirmed'
				  AND confirmed_at IS NOT NULL
				  AND confirmed_at >= $2::timestamptz AND confirmed_at <= $3::timestamptz
				GROUP BY 1
			),
			mo AS (
				SELECT date_trunc($1::text, created_at AT TIME ZONE $4::text) AS b,
				       SUM(total_amount) AS merch_gmv
				FROM merch_orders
				WHERE fulfillment_status = 'fulfilled'
				  AND created_at >= $2::timestamptz AND created_at <= $3::timestamptz
				GROUP BY 1
			),
			su AS (
				SELECT date_trunc($1::text, created_at AT TIME ZONE $4::text) AS b,
				       COUNT(*) AS signups
				FROM users
				WHERE created_at >= $2::timestamptz AND created_at <= $3::timestamptz
				GROUP BY 1
			),
			ev AS (
				SELECT date_trunc($1::text, created_at AT TIME ZONE $4::text) AS b,
				       COUNT(*) AS events_created
				FROM events
				WHERE created_at >= $2::timestamptz AND created_at <= $3::timestamptz
				GROUP BY 1
			)
			SELECT
				(grid.b AT TIME ZONE $4::text) AS bucket,
				COALESCE(bk.ticket_gmv, 0)::numeric,
				COALESCE(mo.merch_gmv, 0)::numeric,
				COALESCE(bk.our_revenue, 0)::numeric,
				COALESCE(bk.bookings, 0)::bigint,
				COALESCE(bk.tickets_sold, 0)::bigint,
				COALESCE(bk.tickets_free, 0)::bigint,
				COALESCE(bk.tickets_paid, 0)::bigint,
				COALESCE(su.signups, 0)::bigint,
				COALESCE(ev.events_created, 0)::bigint
			FROM grid
			LEFT JOIN bk ON bk.b = grid.b
			LEFT JOIN mo ON mo.b = grid.b
			LEFT JOIN su ON su.b = grid.b
			LEFT JOIN ev ON ev.b = grid.b
			ORDER BY grid.b
		`

		rows, err := pool.Query(ctx, q, gran, from, to, tz)
		if err != nil {
			utils.InternalError(c, err, "could not load analytics series")
			return
		}
		defer rows.Close()

		points := []adminSeriesPoint{}
		for rows.Next() {
			var p adminSeriesPoint
			if err := rows.Scan(&p.Bucket, &p.TicketGMV, &p.MerchGMV, &p.OurRevenue,
				&p.Bookings, &p.TicketsSold, &p.TicketsFree, &p.TicketsPaid,
				&p.Signups, &p.EventsCreated); err != nil {
				utils.InternalError(c, err, "could not read analytics series")
				return
			}
			points = append(points, p)
		}
		if err := rows.Err(); err != nil {
			utils.InternalError(c, err, "could not read analytics series")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"granularity": gran,
			"timezone":    tz,
			"from":        from,
			"to":          to,
			"clamped":     clamped,
			"clamp_note":  note,
			"points":      points,
		})
	}
}