package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"events-db/internal/cache"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Organiser analytics endpoints
//
// Register in main.go — the first three inside the existing orgRoutes group
// (AuthMiddleware + OrganizerOnlyMiddleware already applied), the beacon on the
// public router:
//
//	orgRoutes.GET("/dashboard/analytics/overview", handlers.GetAnalyticsOverviewHandler(pool))
//	orgRoutes.GET("/dashboard/analytics/events",   handlers.ListEventAnalyticsHandler(pool))
//	orgRoutes.GET("/dashboard/analytics/series",   handlers.GetAnalyticsSeriesHandler(pool))
//	orgRoutes.GET("/dashboard/analytics/events/:id", handlers.GetEventAnalyticsHandler(pool))
//
//	v1.POST("/events/:id/dwell", handlers.RecordDwellHandler(pool, eventCache))
// ─────────────────────────────────────────────────────────────────────────────

// defaultAnalyticsTZ is the display zone every organiser sees unless they ask
// for another. Bucketing in SAST rather than UTC is what makes "by day" mean a
// South African day — on UTC days, every sale after 22:00 local lands on
// tomorrow, which for a product full of evening events misfiles most of the
// dataset. Same reasoning that produced internal/utils/timefmt.go.
const defaultAnalyticsTZ = "Africa/Johannesburg"

// ── GET /organiser/dashboard/analytics/overview ──────────────────────────────
//
// The two rows at the top of the dashboard.
//
//	{
//	  "upcoming": { "title": "…", "days_to_go": 12.4, "metrics": { … } } | null,
//	  "averages": { "events_counted": 7, "avg_tickets_sold": 84.3, … } | null
//	}
//
// Both halves are null for a brand-new organiser. The dashboard shows a prompt
// for the null case rather than a row of zeroes, because "no data yet" and
// "zero reach" are different things and must not look the same.
func GetAnalyticsOverviewHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.MustGet("user_id").(string)

		out, err := repository.GetAnalyticsOverview(pool, orgID)
		if err != nil {
			utils.InternalError(c, err, "failed to load analytics overview")
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

// ── GET /organiser/dashboard/analytics/events ────────────────────────────────
//
// Every event the organiser owns with its full metric set. Powers the advanced
// page's breakdown table and its event picker in one call — the picker needs
// titles and the table needs numbers, and fetching them separately would make
// the two disagree during a refresh.
func ListEventAnalyticsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.MustGet("user_id").(string)

		rows, err := repository.ListEventAnalytics(pool, orgID)
		if err != nil {
			utils.InternalError(c, err, "failed to load event analytics")
			return
		}
		c.JSON(http.StatusOK, gin.H{"events": rows})
	}
}

// ── GET /organiser/dashboard/analytics/events/:id ────────────────────────────
//
// Deep analytics for one event.
func GetEventAnalyticsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.MustGet("user_id").(string)
		eventID := c.Param("id")

		if _, err := uuid.Parse(eventID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
			return
		}

		out, err := repository.GetEventAnalytics(pool, orgID, eventID)
		if errors.Is(err, repository.ErrEventNotOwned) {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "failed to load event analytics")
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

// ── GET /organiser/dashboard/analytics/series ────────────────────────────────
//
// The time series behind every chart on the advanced page.
//
// Query params (all optional):
//
//	granularity  minute | hour | day       (default: hour)
//	event_id     repeatable, or comma-separated. Omitted ⇒ all your events.
//	from, to     RFC3339. Defaults to a sensible window for the grain.
//	tz           IANA zone name (default Africa/Johannesburg)
//
// The response echoes the granularity, window and timezone ACTUALLY served.
// A request for a year at minute grain is clamped, not rejected, and says so in
// clamp_note — the organiser gets the recent detail they were after instead of
// a validation error explaining they asked wrong.
func GetAnalyticsSeriesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.MustGet("user_id").(string)

		gran, err := repository.ParseGranularity(c.Query("granularity"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		tz := c.DefaultQuery("tz", defaultAnalyticsTZ)
		// Validate before it reaches SQL: an unknown zone name makes
		// `AT TIME ZONE` throw, which would surface as a 500 for what is really
		// a bad request.
		if _, lErr := time.LoadLocation(tz); lErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown timezone"})
			return
		}

		eventIDs, bad := parseEventIDs(c)
		if bad != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": bad})
			return
		}

		to := time.Now()
		if raw := c.Query("to"); raw != "" {
			t, pErr := time.Parse(time.RFC3339, raw)
			if pErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "to must be RFC3339"})
				return
			}
			to = t
		}

		from := to.Add(-defaultWindow(gran))
		if raw := c.Query("from"); raw != "" {
			t, pErr := time.Parse(time.RFC3339, raw)
			if pErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "from must be RFC3339"})
				return
			}
			from = t
		}
		if !from.Before(to) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from must be before to"})
			return
		}

		res, err := repository.GetEventSeries(pool, orgID, eventIDs, gran, from, to, tz)
		if errors.Is(err, repository.ErrEventNotOwned) {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "failed to load analytics series")
			return
		}
		c.JSON(http.StatusOK, res)
	}
}

// defaultWindow picks a range that fills a chart without clamping when the
// caller doesn't specify one. Each is comfortably under MaxSeriesPoints.
func defaultWindow(g repository.Granularity) time.Duration {
	switch g {
	case repository.GranMinute:
		return 6 * time.Hour // 360 points
	case repository.GranDay:
		return 90 * 24 * time.Hour // 90 points
	default:
		return 14 * 24 * time.Hour // 336 points
	}
}

// parseEventIDs accepts either repeated ?event_id= params or one
// comma-separated list, because both are natural to write and neither is
// obviously correct to a caller reading the docs.
func parseEventIDs(c *gin.Context) ([]string, string) {
	raw := c.QueryArray("event_id")
	var flat []string
	for _, r := range raw {
		for _, part := range strings.Split(r, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, err := uuid.Parse(part); err != nil {
				return nil, "event_id must be a UUID"
			}
			flat = append(flat, part)
		}
	}
	return flat, ""
}

// ── POST /events/:id/dwell ───────────────────────────────────────────────────
//
// The retention beacon. Clients POST how long the viewer had the event detail
// screen open, once, when it closes.
//
//	{ "ms": 8400 }
//
// Unauthenticated on purpose — the metric describes anonymous browsing, and
// requiring a token would silently exclude every logged-out visitor, which is
// most of the traffic an organiser is trying to measure.
//
// That makes it writable by anyone, so it is deliberately boring to abuse:
//
//   - The event must exist AND be publicly visible, so it can't be used to
//     create rows for drafts or invented ids.
//   - ms is clamped to a sane ceiling. Without it, one curl with
//     ms=9999999999 would move the organiser's average retention to "16 weeks
//     per view" permanently, since these counters are additive and there is no
//     delete path.
//   - It returns 204 with no body whatever happens. There is nothing to learn
//     from the response, and beacons fired during page-unload can't read one.
//
// It is NOT a fraud-proof metric and shouldn't be presented as one. What it
// gives an organiser is a directional read on whether people linger, and the
// dashboard shows dwell COVERAGE next to it so a thin sample is visible rather
// than implied.
func RecordDwellHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	// maxDwellMs caps a single report at 30 minutes. Longer than any real
	// reading session, short enough that abuse can't dominate the average.
	const maxDwellMs int64 = 30 * 60 * 1000

	return func(c *gin.Context) {
		eventID := c.Param("id")
		if _, err := uuid.Parse(eventID); err != nil {
			c.Status(http.StatusNoContent)
			return
		}

		var body struct {
			Ms int64 `json:"ms"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Ms <= 0 {
			c.Status(http.StatusNoContent)
			return
		}
		if body.Ms > maxDwellMs {
			body.Ms = maxDwellMs
		}

		if !repository.EventIsVisible(pool, eventID) {
			c.Status(http.StatusNoContent)
			return
		}

		// Redis first; fall through to Postgres when it's down. Unlike
		// impressions, a dwell report is rare and unrepeatable — the client
		// sends each one exactly once and never retries — so it's worth a
		// direct write rather than dropping it.
		if ec == nil || !ec.RecordDwell(c.Request.Context(), eventID, body.Ms) {
			_ = repository.RecordMetricsDirect(pool, eventID, 0, body.Ms, 1)
		}

		c.Status(http.StatusNoContent)
	}
}