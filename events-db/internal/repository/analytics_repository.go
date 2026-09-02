package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Organiser analytics
//
// METRIC DEFINITIONS (this is the contract — the dashboard renders exactly what
// is defined here and never recomputes a metric a second way on the client):
//
//	impressions  Reach. The event's card was returned in a feed, search or
//	             category response. Counted server-side and deduplicated per
//	             response, so an event appearing in two sections of one homepage
//	             load is ONE impression, not two.
//	views        Someone opened the event detail page.
//	retention    dwell_ms / views, in seconds — "how long a viewer spent on your
//	             event divided by the views".
//	psr          Post-split revenue: subtotal - refund - organizer_fee, over
//	             confirmed AND refunded bookings. What the organiser keeps
//	             after the platform cut — the same arithmetic the payout worker
//	             uses (CreatePayoutsForCompletedEvents), so this figure and the
//	             dashboard's Net earnings are the same number by construction.
//
//	             It used to be subtotal - organizer_fee over confirmed rows
//	             only, which silently dropped a part-refunded booking whole: a
//	             R10 ticket refunded at R2.50 removed all R10 instead of R2.50,
//	             hiding R6.90 the organiser was still owed. Two cards labelled
//	             revenue disagreeing by an unexplained amount is worse than
//	             either definition on its own.
//	scanned      Tickets admitted at the door (scan_events, verdict 'admitted',
//	             summing admit_count so a pool admit counts every head).
//	no_shows     tickets_sold - scanned. Only a fact once the event has ended,
//	             so it is nil until then rather than a large fake number.
//	attendance   scanned / tickets_sold.
//
// Derived rates that fall out of the above:
//	ctr          views / impressions      — did the card earn the click
//	conversion   tickets_sold / views     — did the page earn the sale
//
// OWNERSHIP: every exported function takes organizerID and enforces it in the
// same statement that returns data. There is no fetch-then-check path, so a
// valid event id belonging to someone else returns nothing rather than a row.
//
// PACKAGE PLACEMENT: MetricBucket lives here, not in internal/cache, because
// internal/cache already imports internal/repository — defining it the other
// way round would be an import cycle.
// ─────────────────────────────────────────────────────────────────────────────

// ErrEventNotOwned covers "doesn't exist" and "isn't yours" with one error on
// purpose: separate errors would let an organiser probe which ids exist.
var ErrEventNotOwned = errors.New("event not found for this organiser")

// MetricBucket is one drained minute of reach/view/dwell for one event.
type MetricBucket struct {
	EventID      string
	BucketAt     time.Time // truncated to the minute, UTC
	Impressions  int64
	Views        int64
	DwellMs      int64
	DwellSamples int64
}

// Granularity is the time grain of a series request.
type Granularity string

const (
	GranMinute Granularity = "minute"
	GranHour   Granularity = "hour"
	GranDay    Granularity = "day"
)

// ParseGranularity whitelists the grain. date_trunc takes its field as a bound
// text parameter, so this is not an injection guard — it rejects grains the
// dashboard has no UI for ('century', 'microsecond') before they produce an
// absurd number of points.
func ParseGranularity(s string) (Granularity, error) {
	switch Granularity(s) {
	case GranMinute, GranHour, GranDay:
		return Granularity(s), nil
	case "":
		return GranHour, nil
	default:
		return "", fmt.Errorf("granularity must be minute, hour or day")
	}
}

// MaxSeriesPoints caps how many buckets one response may contain.
//
// Minute grain over a month is 43 200 points: the query is fine, the JSON is
// megabytes and the chart is unreadable. The window is clamped rather than
// rejected, so "last 30 days by minute" gives a useful answer instead of a
// validation error.
const MaxSeriesPoints = 1500

// ── Ingest ───────────────────────────────────────────────────────────────────

// UpsertMetricBuckets writes drained Redis buckets to Postgres.
//
// ADDITIVE on conflict: two flushes touching the same minute sum rather than
// overwrite. That is what makes the direct-write fallback safe to interleave
// with the buffered path — the same minute can legitimately be written from
// both routes and the total stays correct.
// The whole drain goes in ONE statement via unnest, not a pgx.Batch.
//
// This matters for correctness, not just speed. An event deleted between the
// impression and the flush leaves a bucket whose FK no longer resolves, and in
// a batch that single failure aborts every queued statement after it — one
// deleted event would silently discard the entire minute's metrics for every
// other event. The JOIN to events below drops orphaned buckets and writes the
// rest, which is the behaviour the batch version only appeared to have.
func UpsertMetricBuckets(pool *pgxpool.Pool, buckets []MetricBucket) error {
	if len(buckets) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Plain []string / []time.Time / []int64 — NOT custom named slice types.
	// pgx v5 encodes these natively; a named slice type would need an explicit
	// codec and panics at encode time without one.
	ids := make([]string, len(buckets))
	at := make([]time.Time, len(buckets))
	imp := make([]int64, len(buckets))
	vw := make([]int64, len(buckets))
	dms := make([]int64, len(buckets))
	dn := make([]int64, len(buckets))
	for i, b := range buckets {
		ids[i] = b.EventID
		at[i] = b.BucketAt
		imp[i] = b.Impressions
		vw[i] = b.Views
		dms[i] = b.DwellMs
		dn[i] = b.DwellSamples
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO event_metric_buckets
			(event_id, bucket_at, impressions, views, dwell_ms, dwell_samples)
		SELECT v.event_id, v.bucket_at, v.impressions, v.views, v.dwell_ms, v.dwell_samples
		FROM unnest(
			$1::uuid[], $2::timestamptz[], $3::bigint[],
			$4::bigint[], $5::bigint[], $6::bigint[]
		) AS v(event_id, bucket_at, impressions, views, dwell_ms, dwell_samples)
		-- Drops buckets for events deleted since the impression was recorded,
		-- instead of failing the write on a foreign-key violation.
		JOIN events e ON e.id = v.event_id
		ON CONFLICT (event_id, bucket_at) DO UPDATE SET
			impressions   = event_metric_buckets.impressions   + EXCLUDED.impressions,
			views         = event_metric_buckets.views         + EXCLUDED.views,
			dwell_ms      = event_metric_buckets.dwell_ms      + EXCLUDED.dwell_ms,
			dwell_samples = event_metric_buckets.dwell_samples + EXCLUDED.dwell_samples
	`, ids, at, imp, vw, dms, dn)
	if err != nil {
		return fmt.Errorf("UpsertMetricBuckets (%d buckets): %w", len(buckets), err)
	}
	return nil
}

// RecordMetricsDirect writes a counter straight to Postgres, bypassing Redis.
//
// Used when Redis is unavailable so dwell reports aren't lost — a client sends
// each dwell exactly once and never retries, so there is no second chance.
//
// Impressions deliberately do NOT use this path: they are high-frequency and
// low-value, and hammering Postgres from the feed to save a metric would trade
// homepage latency for a number nobody would miss.
func RecordMetricsDirect(pool *pgxpool.Pool, eventID string, views, dwellMs, dwellSamples int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO event_metric_buckets
			(event_id, bucket_at, impressions, views, dwell_ms, dwell_samples)
		VALUES ($1, date_trunc('minute', now()), 0, $2, $3, $4)
		ON CONFLICT (event_id, bucket_at) DO UPDATE SET
			views         = event_metric_buckets.views         + EXCLUDED.views,
			dwell_ms      = event_metric_buckets.dwell_ms      + EXCLUDED.dwell_ms,
			dwell_samples = event_metric_buckets.dwell_samples + EXCLUDED.dwell_samples
	`, eventID, views, dwellMs, dwellSamples)
	if err != nil {
		return fmt.Errorf("RecordMetricsDirect: %w", err)
	}
	return nil
}

// ResolveVisibleEventID turns a slug OR a UUID into the canonical event UUID,
// and reports whether that event is publicly reachable.
//
// Accepting both is not optional: lib/shareLinks.ts PREFERS the slug when the
// backend supplies one, so most real web traffic arrives at /events/<slug>.
// A UUID-only check would have silently dropped every one of those beacons and
// left retention measured only over people who happened to arrive via a UUID
// URL — a badly biased sample that would still have looked like a working
// metric. TrackEventViewHandler resolves the same two forms for the same
// reason.
//
// The visibility predicate also stops the unauthenticated beacon manufacturing
// metric rows for drafts, other organisers' events, or invented ids.
func ResolveVisibleEventID(pool *pgxpool.Pool, slugOrID string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Branch rather than OR-ing both predicates: casting a slug to uuid throws
	// at parse time, so `id = $1::uuid OR slug = $1` errors on every slug.
	var query string
	if _, err := uuid.Parse(slugOrID); err == nil {
		query = `SELECT id::text FROM events
		         WHERE id = $1::uuid
		           AND status IN ('published', 'published_private', 'completed')`
	} else {
		query = `SELECT id::text FROM events
		         WHERE slug = $1
		           AND status IN ('published', 'published_private', 'completed')`
	}

	var id string
	if err := pool.QueryRow(ctx, query, slugOrID).Scan(&id); err != nil {
		return "", false
	}
	return id, true
}

// ── Core metric payload ──────────────────────────────────────────────────────

// EventMetrics is one event's full analytic picture.
//
// Pointer fields are the ones that can be genuinely UNKNOWN, and the dashboard
// renders those as "—" rather than 0. A brand-new event with no reach yet and
// an event that reached nobody are different facts; collapsing both to zero
// would tell an organiser their marketing failed when it simply hasn't run.
type EventMetrics struct {
	// Reach and interest
	Impressions      int64    `json:"impressions"`
	Views            int64    `json:"views"`
	CTR              *float64 `json:"ctr"`               // views / impressions
	RetentionSeconds *float64 `json:"retention_seconds"` // dwell / views
	DwellSamples     int64    `json:"dwell_samples"`
	// Share of views that reported a dwell. Below ~20% the retention figure is
	// a sample rather than a measurement, and the UI says so.
	DwellCoverage *float64 `json:"dwell_coverage"`

	// Raw dwell accumulator behind RetentionSeconds. Not serialised — the
	// dashboard wants seconds-per-view, not a millisecond sum — but needed
	// while aggregating, so it rides along excluded from JSON.
	DwellMsTotal int64 `json:"-"`

	// Sales
	TicketsSold  int64    `json:"tickets_sold"`
	Bookings     int64    `json:"bookings"`
	GrossRevenue float64  `json:"gross_revenue"`
	PlatformFees float64  `json:"platform_fees"`
	PSR          float64  `json:"psr"`        // post-split revenue
	Conversion   *float64 `json:"conversion"` // tickets_sold / views

	// Door
	Scanned        int64    `json:"scanned"`
	NoShows        *int64   `json:"no_shows"`        // nil until the event ended
	AttendanceRate *float64 `json:"attendance_rate"` // scanned / tickets_sold
}

func ratio(num, den float64) *float64 {
	if den <= 0 {
		return nil
	}
	v := num / den
	return &v
}

// finalise derives every rate from the raw counters. Kept in ONE place so the
// per-event view, the series totals and the averages can never drift apart.
func (m *EventMetrics) finalise(ended bool) {
	m.CTR = ratio(float64(m.Views), float64(m.Impressions))
	m.Conversion = ratio(float64(m.TicketsSold), float64(m.Views))
	m.AttendanceRate = ratio(float64(m.Scanned), float64(m.TicketsSold))
	m.DwellCoverage = ratio(float64(m.DwellSamples), float64(m.Views))

	if m.Views > 0 && m.DwellMsTotal > 0 {
		v := float64(m.DwellMsTotal) / float64(m.Views) / 1000.0
		m.RetentionSeconds = &v
	}

	// No-shows only exist after the doors closed. Before that, an unscanned
	// ticket is just someone who hasn't arrived yet.
	if ended {
		n := m.TicketsSold - m.Scanned
		if n < 0 {
			// Pool admits can exceed per-ticket assignments; never report a
			// negative no-show count.
			n = 0
		}
		m.NoShows = &n
	}
}

// ── Per-event analytics ──────────────────────────────────────────────────────

// EventAnalytics is the payload for the advanced per-event view.
type EventAnalytics struct {
	EventID   string    `json:"event_id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Ended     bool      `json:"ended"`
	Capacity  *int      `json:"capacity"`
	Currency  string    `json:"currency"`

	Metrics EventMetrics `json:"metrics"`

	// When reach collection began for this event. Everything before this is
	// unknowable (the table starts empty at migration time); the dashboard uses
	// it to avoid presenting a partial history as a complete one.
	MetricsSince *time.Time `json:"metrics_since"`
}

// GetEventAnalytics returns the deep picture for one owned event.
func GetEventAnalytics(pool *pgxpool.Pool, organizerID, eventID string) (*EventAnalytics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var a EventAnalytics
	err := pool.QueryRow(ctx, `
		SELECT e.id::text, e.title, e.status::text, e.start_time, e.end_time,
		       e.max_capacity, e.currency
		FROM events e
		WHERE e.id = $1 AND e.organizer_id = $2
	`, eventID, organizerID).Scan(
		&a.EventID, &a.Title, &a.Status, &a.StartTime, &a.EndTime,
		&a.Capacity, &a.Currency,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEventNotOwned
	}
	if err != nil {
		return nil, fmt.Errorf("GetEventAnalytics event: %w", err)
	}
	a.Ended = time.Now().After(a.EndTime)

	m, since, err := eventMetricsFor(ctx, pool, []string{eventID})
	if err != nil {
		return nil, err
	}
	m.finalise(a.Ended)
	a.Metrics = m
	a.MetricsSince = since
	return &a, nil
}

// eventMetricsFor aggregates every metric across a set of event ids.
//
// Three narrow queries rather than one joined query, on purpose: joining
// buckets to bookings to scans multiplies rows (60 buckets × 40 bookings = 2400
// rows before the GROUP BY) and every SUM would then need a DISTINCT
// correction. Three scans are both faster and impossible to get subtly wrong.
func eventMetricsFor(ctx context.Context, pool *pgxpool.Pool, eventIDs []string) (EventMetrics, *time.Time, error) {
	var m EventMetrics
	var since *time.Time

	// 1. Reach / views / dwell
	err := pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(impressions), 0)::bigint,
			COALESCE(SUM(views), 0)::bigint,
			COALESCE(SUM(dwell_ms), 0)::bigint,
			COALESCE(SUM(dwell_samples), 0)::bigint,
			MIN(bucket_at)
		FROM event_metric_buckets
		WHERE event_id = ANY($1::uuid[])
	`, eventIDs).Scan(&m.Impressions, &m.Views, &m.DwellMsTotal, &m.DwellSamples, &since)
	if err != nil {
		return m, nil, fmt.Errorf("eventMetricsFor buckets: %w", err)
	}

	// 2. Sales. COUNTS stay confirmed-only — a refunded booking is not a sale
	// and its seat went back to inventory — while MONEY spans confirmed and
	// refunded, because a part-refunded booking still owes the organiser its
	// remainder. Same split as the organiser dashboard.
	err = pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE b.booking_status = 'confirmed')::bigint,
			COALESCE(SUM(b.ticket_quantity)
				FILTER (WHERE b.booking_status = 'confirmed'), 0)::bigint,
			COALESCE(SUM(b.subtotal), 0)::numeric,
			COALESCE(SUM(b.organizer_fee), 0)::numeric,
			COALESCE(SUM(`+settlementAmountSQL+`), 0)::numeric
		FROM bookings b
		WHERE b.event_id = ANY($1::uuid[])
		  AND b.booking_status IN `+paidBookingStatuses+`
	`, eventIDs).Scan(&m.Bookings, &m.TicketsSold, &m.GrossRevenue, &m.PlatformFees, &m.PSR)
	if err != nil {
		return m, nil, fmt.Errorf("eventMetricsFor sales: %w", err)
	}

	// 3. Door — admitted scans only. admit_count is SUMMED rather than counting
	// rows, because one pool admit is a single scan_event that lets several
	// people through the door.
	err = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(admit_count), 0)::bigint
		FROM scan_events
		WHERE event_id = ANY($1::uuid[])
		  AND verdict  = 'admitted'
	`, eventIDs).Scan(&m.Scanned)
	if err != nil {
		return m, nil, fmt.Errorf("eventMetricsFor scans: %w", err)
	}

	return m, since, nil
}

// ── Time series ──────────────────────────────────────────────────────────────

// SeriesPoint is one bucket at the requested grain.
type SeriesPoint struct {
	// Bucket is the START of the period as an absolute instant. The client
	// formats it in the requested zone; sending an instant rather than a
	// pre-formatted string keeps zone handling in exactly one place.
	Bucket       time.Time `json:"bucket"`
	Impressions  int64     `json:"impressions"`
	Views        int64     `json:"views"`
	TicketsSold  int64     `json:"tickets_sold"`
	Bookings     int64     `json:"bookings"`
	GrossRevenue float64   `json:"gross_revenue"`
	PSR          float64   `json:"psr"`
	Scanned      int64     `json:"scanned"`
	DwellMs      int64     `json:"dwell_ms"`
	DwellSamples int64     `json:"dwell_samples"`
}

// SeriesResult wraps the points with what was ACTUALLY served, which may differ
// from what was asked for once clamping applies.
type SeriesResult struct {
	Granularity Granularity   `json:"granularity"`
	Timezone    string        `json:"timezone"`
	From        time.Time     `json:"from"`
	To          time.Time     `json:"to"`
	Clamped     bool          `json:"clamped"`
	ClampNote   string        `json:"clamp_note,omitempty"`
	Points      []SeriesPoint `json:"points"`
	Totals      EventMetrics  `json:"totals"`
}

// ClampRange shrinks a window so it can't exceed MaxSeriesPoints at the chosen
// grain.
//
// The END is kept and the START moves forward: asked for "the last year by
// minute", what someone wants to see is the recent detail, not the first 25
// hours of last year.
func ClampRange(g Granularity, from, to time.Time) (time.Time, time.Time, bool, string) {
	var step time.Duration
	switch g {
	case GranMinute:
		step = time.Minute
	case GranHour:
		step = time.Hour
	default:
		step = 24 * time.Hour
	}
	maxSpan := step * time.Duration(MaxSeriesPoints)
	if to.Sub(from) <= maxSpan {
		return from, to, false, ""
	}
	return to.Add(-maxSpan), to, true, fmt.Sprintf(
		"%s grain is capped at %d points — showing the most recent %s",
		g, MaxSeriesPoints, maxSpan.Round(time.Minute),
	)
}

// GetEventSeries returns a DENSE series: every bucket in range is present,
// zero-filled where nothing happened.
//
// Density is not cosmetic. In a sparse series a quiet Tuesday and a missing
// Tuesday are indistinguishable, and any line chart drawn from it connects
// straight across the gap, implying activity that never happened.
//
// eventIDs empty ⇒ every event the organiser owns ("All events" mode).
func GetEventSeries(
	pool *pgxpool.Pool,
	organizerID string,
	eventIDs []string,
	g Granularity,
	from, to time.Time,
	tz string,
) (*SeriesResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Ownership is enforced HERE, once. Everything downstream operates on the
	// scoped id list, so the query below needs no organiser predicate.
	scoped, err := scopeEventIDs(ctx, pool, organizerID, eventIDs)
	if err != nil {
		return nil, err
	}

	from, to, clamped, note := ClampRange(g, from, to)
	res := &SeriesResult{
		Granularity: g, Timezone: tz, From: from, To: to,
		Clamped: clamped, ClampNote: note,
		Points: []SeriesPoint{},
	}
	if len(scoped) == 0 {
		return res, nil
	}

	// One statement, one round trip. `grid` generates every bucket in the
	// window IN THE DISPLAY ZONE; each metric source is aggregated to the same
	// expression and LEFT JOINed on.
	//
	// Bucketing in the display zone is what makes "by day" mean a South African
	// day rather than a UTC one. On UTC days every sale made after 22:00 SAST
	// lands on tomorrow — which for a product whose events are overwhelmingly
	// evening events would misfile a large share of the dataset.
	const q = `
		WITH grid AS (
			SELECT generate_series(
				date_trunc($2::text, $3::timestamptz AT TIME ZONE $5::text),
				date_trunc($2::text, $4::timestamptz AT TIME ZONE $5::text),
				('1 ' || $2::text)::interval
			) AS b
		),
		m AS (
			SELECT date_trunc($2::text, bucket_at AT TIME ZONE $5::text) AS b,
			       SUM(impressions)   AS impressions,
			       SUM(views)         AS views,
			       SUM(dwell_ms)      AS dwell_ms,
			       SUM(dwell_samples) AS dwell_samples
			FROM event_metric_buckets
			WHERE event_id = ANY($1::uuid[])
			  AND bucket_at >= $3::timestamptz AND bucket_at <= $4::timestamptz
			GROUP BY 1
		),
		-- Bucketed by confirmed_at, and a refunded booking keeps the
		-- confirmed_at of the sale — so its remainder lands in the bucket where
		-- the money came in, not where the refund happened. That is what makes
		-- the daily series still sum to the totals above.
		s AS (
			SELECT date_trunc($2::text, b.confirmed_at AT TIME ZONE $5::text) AS b,
			       COUNT(*) FILTER (WHERE b.booking_status = 'confirmed')     AS bookings,
			       SUM(b.ticket_quantity)
			           FILTER (WHERE b.booking_status = 'confirmed')          AS tickets_sold,
			       SUM(b.subtotal)                                            AS gross_revenue,
			       SUM(`+settlementAmountSQL+`)                               AS psr
			FROM bookings b
			WHERE b.event_id = ANY($1::uuid[])
			  AND b.booking_status IN `+paidBookingStatuses+`
			  AND b.confirmed_at IS NOT NULL
			  AND b.confirmed_at >= $3::timestamptz AND b.confirmed_at <= $4::timestamptz
			GROUP BY 1
		),
		sc AS (
			SELECT date_trunc($2::text, received_at AT TIME ZONE $5::text) AS b,
			       SUM(admit_count) AS scanned
			FROM scan_events
			WHERE event_id = ANY($1::uuid[])
			  AND verdict  = 'admitted'
			  AND received_at >= $3::timestamptz AND received_at <= $4::timestamptz
			GROUP BY 1
		)
		SELECT
			(grid.b AT TIME ZONE $5::text) AS bucket,
			COALESCE(m.impressions, 0)::bigint,
			COALESCE(m.views, 0)::bigint,
			COALESCE(m.dwell_ms, 0)::bigint,
			COALESCE(m.dwell_samples, 0)::bigint,
			COALESCE(s.bookings, 0)::bigint,
			COALESCE(s.tickets_sold, 0)::bigint,
			COALESCE(s.gross_revenue, 0)::numeric,
			COALESCE(s.psr, 0)::numeric,
			COALESCE(sc.scanned, 0)::bigint
		FROM grid
		LEFT JOIN m  ON m.b  = grid.b
		LEFT JOIN s  ON s.b  = grid.b
		LEFT JOIN sc ON sc.b = grid.b
		ORDER BY grid.b
	`

	rows, err := pool.Query(ctx, q, scoped, string(g), from, to, tz)
	if err != nil {
		return nil, fmt.Errorf("GetEventSeries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p SeriesPoint
		if err := rows.Scan(
			&p.Bucket, &p.Impressions, &p.Views, &p.DwellMs, &p.DwellSamples,
			&p.Bookings, &p.TicketsSold, &p.GrossRevenue, &p.PSR, &p.Scanned,
		); err != nil {
			return nil, fmt.Errorf("GetEventSeries scan: %w", err)
		}
		res.Totals.Impressions += p.Impressions
		res.Totals.Views += p.Views
		res.Totals.DwellSamples += p.DwellSamples
		res.Totals.DwellMsTotal += p.DwellMs
		res.Totals.Bookings += p.Bookings
		res.Totals.TicketsSold += p.TicketsSold
		res.Totals.GrossRevenue += p.GrossRevenue
		res.Totals.PSR += p.PSR
		res.Totals.Scanned += p.Scanned
		res.Points = append(res.Points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	res.Totals.PlatformFees = res.Totals.GrossRevenue - res.Totals.PSR
	// Window totals describe a PERIOD, not an event lifecycle, so no-shows stay
	// nil here — they only mean something against a finished event.
	res.Totals.finalise(false)
	return res, nil
}

// scopeEventIDs reduces a requested id set to the subset the organiser owns.
// An empty request means "all my events".
func scopeEventIDs(ctx context.Context, pool *pgxpool.Pool, organizerID string, requested []string) ([]string, error) {
	var rows pgx.Rows
	var err error
	if len(requested) == 0 {
		rows, err = pool.Query(ctx,
			`SELECT id::text FROM events WHERE organizer_id = $1`, organizerID)
	} else {
		rows, err = pool.Query(ctx,
			`SELECT id::text FROM events WHERE organizer_id = $1 AND id = ANY($2::uuid[])`,
			organizerID, requested)
	}
	if err != nil {
		return nil, fmt.Errorf("scopeEventIDs: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Specific ids requested and none came back ⇒ none of them are theirs.
	if len(requested) > 0 && len(out) == 0 {
		return nil, ErrEventNotOwned
	}
	return out, nil
}

// ── Organiser overview (the two dashboard rows) ──────────────────────────────

// OverviewEvent is the "next event coming up" row.
type OverviewEvent struct {
	EventID   string       `json:"event_id"`
	Title     string       `json:"title"`
	Status    string       `json:"status"`
	StartTime time.Time    `json:"start_time"`
	EndTime   time.Time    `json:"end_time"`
	Capacity  *int         `json:"capacity"`
	Currency  string       `json:"currency"`
	DaysToGo  float64      `json:"days_to_go"`
	Metrics   EventMetrics `json:"metrics"`
}

// OverviewAverages is the "average across every event you've held" row.
//
// Averages are per EVENT, not per ticket: "my typical event sells 84 tickets"
// is the sentence an organiser reasons with.
//
// Rates (CTR, retention, attendance) are computed from POOLED totals rather
// than by averaging each event's rate. A mean of ratios lets an event with
// three views and one sale claim 33% conversion and drag the headline with it;
// pooling weights every event by its actual size.
type OverviewAverages struct {
	EventsCounted int `json:"events_counted"`

	AvgImpressions  float64  `json:"avg_impressions"`
	AvgViews        float64  `json:"avg_views"`
	AvgTicketsSold  float64  `json:"avg_tickets_sold"`
	AvgPSR          float64  `json:"avg_psr"`
	AvgGrossRevenue float64  `json:"avg_gross_revenue"`
	AvgScanned      float64  `json:"avg_scanned"`
	AvgNoShows      *float64 `json:"avg_no_shows"`

	// Pooled rates
	CTR              *float64 `json:"ctr"`
	Conversion       *float64 `json:"conversion"`
	RetentionSeconds *float64 `json:"retention_seconds"`
	AttendanceRate   *float64 `json:"attendance_rate"`

	Currency string `json:"currency"`
}

// AnalyticsOverview is the payload behind the two rows at the top of the
// dashboard. Both halves are nullable: a brand-new organiser has neither a next
// event nor a history, and the dashboard shows a prompt instead of zeroes.
type AnalyticsOverview struct {
	Upcoming *OverviewEvent    `json:"upcoming"`
	Averages *OverviewAverages `json:"averages"`
}

// GetAnalyticsOverview builds both dashboard rows in one call.
func GetAnalyticsOverview(pool *pgxpool.Pool, organizerID string) (*AnalyticsOverview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	out := &AnalyticsOverview{}

	// ── Row 1: the next event that hasn't finished ───────────────────────────
	// Ordered by start_time so "next up" means soonest. Restricted to live
	// statuses: a draft isn't coming up, it's unfinished.
	var ue OverviewEvent
	err := pool.QueryRow(ctx, `
		SELECT id::text, title, status::text, start_time, end_time, max_capacity, currency
		FROM events
		WHERE organizer_id = $1
		  AND status IN ('published', 'published_private')
		  AND end_time >= now()
		ORDER BY start_time ASC
		LIMIT 1
	`, organizerID).Scan(
		&ue.EventID, &ue.Title, &ue.Status, &ue.StartTime, &ue.EndTime,
		&ue.Capacity, &ue.Currency,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("GetAnalyticsOverview upcoming: %w", err)
	}
	if err == nil {
		ue.DaysToGo = time.Until(ue.StartTime).Hours() / 24
		m, _, mErr := eventMetricsFor(ctx, pool, []string{ue.EventID})
		if mErr != nil {
			return nil, mErr
		}
		m.finalise(time.Now().After(ue.EndTime))
		ue.Metrics = m
		out.Upcoming = &ue
	}

	// ── Row 2: averages across events actually HELD ──────────────────────────
	// "Held" means finished. Live and upcoming events are excluded because
	// their totals are still climbing — folding a half-sold event into the
	// average drags the organiser's own benchmark down for no reason.
	heldIDs := []string{}
	var currency string
	rows, err := pool.Query(ctx, `
		SELECT id::text, currency
		FROM events
		WHERE organizer_id = $1
		  AND end_time < now()
		  AND status IN ('published', 'published_private', 'completed')
	`, organizerID)
	if err != nil {
		return nil, fmt.Errorf("GetAnalyticsOverview held: %w", err)
	}
	for rows.Next() {
		var id, cur string
		if err := rows.Scan(&id, &cur); err != nil {
			rows.Close()
			return nil, err
		}
		heldIDs = append(heldIDs, id)
		if currency == "" {
			currency = cur
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(heldIDs) == 0 {
		return out, nil
	}

	pooled, _, err := eventMetricsFor(ctx, pool, heldIDs)
	if err != nil {
		return nil, err
	}
	pooled.finalise(true) // every event in this set has ended

	n := float64(len(heldIDs))
	avg := &OverviewAverages{
		EventsCounted:    len(heldIDs),
		AvgImpressions:   float64(pooled.Impressions) / n,
		AvgViews:         float64(pooled.Views) / n,
		AvgTicketsSold:   float64(pooled.TicketsSold) / n,
		AvgPSR:           pooled.PSR / n,
		AvgGrossRevenue:  pooled.GrossRevenue / n,
		AvgScanned:       float64(pooled.Scanned) / n,
		CTR:              pooled.CTR,
		Conversion:       pooled.Conversion,
		RetentionSeconds: pooled.RetentionSeconds,
		AttendanceRate:   pooled.AttendanceRate,
		Currency:         currency,
	}
	if pooled.NoShows != nil {
		v := float64(*pooled.NoShows) / n
		avg.AvgNoShows = &v
	}
	out.Averages = avg
	return out, nil
}

// ── Per-event breakdown list (advanced page table) ───────────────────────────

// EventAnalyticsRow is one row of the advanced page's all-events table.
type EventAnalyticsRow struct {
	EventID   string       `json:"event_id"`
	Title     string       `json:"title"`
	Status    string       `json:"status"`
	StartTime time.Time    `json:"start_time"`
	Ended     bool         `json:"ended"`
	Currency  string       `json:"currency"`
	Metrics   EventMetrics `json:"metrics"`
}

// ListEventAnalytics returns every event the organiser owns with its full
// metric set, newest start date first.
//
// N+1 is avoided by pulling each aggregate keyed by event_id — four queries
// total regardless of how many events exist — then stitching in Go. Same
// reasoning as eventMetricsFor, applied across events rather than within one.
func ListEventAnalytics(pool *pgxpool.Pool, organizerID string) ([]EventAnalyticsRow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT id::text, title, status::text, start_time, end_time, currency
		FROM events
		WHERE organizer_id = $1
		ORDER BY start_time DESC
	`, organizerID)
	if err != nil {
		return nil, fmt.Errorf("ListEventAnalytics events: %w", err)
	}
	defer rows.Close()

	out := []EventAnalyticsRow{}
	index := map[string]int{}
	ids := []string{}
	now := time.Now()
	for rows.Next() {
		var r EventAnalyticsRow
		var endTime time.Time
		if err := rows.Scan(&r.EventID, &r.Title, &r.Status, &r.StartTime, &endTime, &r.Currency); err != nil {
			return nil, err
		}
		r.Ended = now.After(endTime)
		index[r.EventID] = len(out)
		ids = append(ids, r.EventID)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return out, nil
	}

	// 1. Reach / views / dwell
	bRows, err := pool.Query(ctx, `
		SELECT event_id::text,
		       COALESCE(SUM(impressions), 0)::bigint,
		       COALESCE(SUM(views), 0)::bigint,
		       COALESCE(SUM(dwell_ms), 0)::bigint,
		       COALESCE(SUM(dwell_samples), 0)::bigint
		FROM event_metric_buckets
		WHERE event_id = ANY($1::uuid[])
		GROUP BY event_id
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("ListEventAnalytics buckets: %w", err)
	}
	for bRows.Next() {
		var id string
		var imp, vw, dms, dn int64
		if err := bRows.Scan(&id, &imp, &vw, &dms, &dn); err != nil {
			bRows.Close()
			return nil, err
		}
		if i, ok := index[id]; ok {
			out[i].Metrics.Impressions = imp
			out[i].Metrics.Views = vw
			out[i].Metrics.DwellMsTotal = dms
			out[i].Metrics.DwellSamples = dn
		}
	}
	bRows.Close()
	if err := bRows.Err(); err != nil {
		return nil, err
	}

	// 2. Sales
	sRows, err := pool.Query(ctx, `
		SELECT event_id::text,
		       COUNT(*)::bigint,
		       COALESCE(SUM(ticket_quantity), 0)::bigint,
		       COALESCE(SUM(subtotal), 0)::numeric,
		       COALESCE(SUM(organizer_fee), 0)::numeric
		FROM bookings
		WHERE event_id = ANY($1::uuid[]) AND booking_status = 'confirmed'
		GROUP BY event_id
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("ListEventAnalytics sales: %w", err)
	}
	for sRows.Next() {
		var id string
		var bk, tk int64
		var gross, fee float64
		if err := sRows.Scan(&id, &bk, &tk, &gross, &fee); err != nil {
			sRows.Close()
			return nil, err
		}
		if i, ok := index[id]; ok {
			out[i].Metrics.Bookings = bk
			out[i].Metrics.TicketsSold = tk
			out[i].Metrics.GrossRevenue = gross
			out[i].Metrics.PlatformFees = fee
			out[i].Metrics.PSR = gross - fee
		}
	}
	sRows.Close()
	if err := sRows.Err(); err != nil {
		return nil, err
	}

	// 3. Door
	cRows, err := pool.Query(ctx, `
		SELECT event_id::text, COALESCE(SUM(admit_count), 0)::bigint
		FROM scan_events
		WHERE event_id = ANY($1::uuid[]) AND verdict = 'admitted'
		GROUP BY event_id
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("ListEventAnalytics scans: %w", err)
	}
	for cRows.Next() {
		var id string
		var sc int64
		if err := cRows.Scan(&id, &sc); err != nil {
			cRows.Close()
			return nil, err
		}
		if i, ok := index[id]; ok {
			out[i].Metrics.Scanned = sc
		}
	}
	cRows.Close()
	if err := cRows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		out[i].Metrics.finalise(out[i].Ended)
	}
	return out, nil
}