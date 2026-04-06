package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FilterEventsHandler handles the unified location + date filter endpoint.
//
//	GET /events/filter
//
// All query parameters are optional — omitting everything returns all
// upcoming published events across South Africa.
//
// Location params (mutually exclusive — city takes precedence over lat/lng):
//
//	city=Cape+Town         filter by city name (case-insensitive)
//	lat=X&lng=Y            filter by proximity to coordinates
//	radius_km=50           radius used with lat/lng (default 50 km)
//
// Date params:
//
//	date=any               no date constraint (default)
//	date=tomorrow          events starting tomorrow
//	date=weekend           events starting this Sat–Sun
//	date=custom            requires date_from and date_to
//	date_from=2025-08-01   ISO 8601 date (required for date=custom)
//	date_to=2025-08-31     ISO 8601 date (required for date=custom)
//
// Pagination:
//
//	limit=20               max 50
//	cursor=...             opaque cursor from previous response
//
// Response:
//
//	{
//	  "data":        [ ...events... ],
//	  "next_cursor": "...",
//	  "filter": {
//	    "city":        "Cape Town",
//	    "date":        "weekend",
//	    "total_shown": 12
//	  }
//	}
func FilterEventsHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── Pagination ────────────────────────────────────────────────────────

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		var cursor *utils.Cursor
		if cursorStr := c.Query("cursor"); cursorStr != "" {
			var err error
			cursor, err = utils.DecodeCursor(cursorStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
				return
			}
		}

		// ── Location ──────────────────────────────────────────────────────────

		var filter repository.EventFilter

		city := strings.TrimSpace(c.Query("city"))
		if city != "" {
			filter.City = city
		} else {
			// Only use lat/lng if no city is provided
			if latStr := c.Query("lat"); latStr != "" {
				if v, err := strconv.ParseFloat(latStr, 64); err == nil {
					filter.Lat = &v
				}
			}
			if lngStr := c.Query("lng"); lngStr != "" {
				if v, err := strconv.ParseFloat(lngStr, 64); err == nil {
					filter.Lng = &v
				}
			}
			if radiusStr := c.Query("radius_km"); radiusStr != "" {
				if v, err := strconv.ParseFloat(radiusStr, 64); err == nil && v > 0 {
					filter.RadiusKm = v
				}
			}
			// lat without lng (or vice versa) — ignore both
			if filter.Lat != nil && filter.Lng == nil {
				filter.Lat = nil
			}
			if filter.Lng != nil && filter.Lat == nil {
				filter.Lng = nil
			}
		}

		// ── Date ──────────────────────────────────────────────────────────────

		datePreset := strings.ToLower(strings.TrimSpace(c.Query("date")))
		filter.DatePreset = datePreset

		if datePreset == "custom" {
			const dateFmt = "2006-01-02"
			if fromStr := c.Query("date_from"); fromStr != "" {
				if t, err := time.Parse(dateFmt, fromStr); err == nil {
					filter.DateFrom = &t
				} else {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date_from — use YYYY-MM-DD"})
					return
				}
			}
			if toStr := c.Query("date_to"); toStr != "" {
				if t, err := time.Parse(dateFmt, toStr); err == nil {
					filter.DateTo = &t
				} else {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date_to — use YYYY-MM-DD"})
					return
				}
			}
			if filter.DateFrom == nil || filter.DateTo == nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "date=custom requires both date_from and date_to (YYYY-MM-DD)",
				})
				return
			}
			if filter.DateTo.Before(*filter.DateFrom) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "date_to must be on or after date_from"})
				return
			}
		}

		// ── Execute ───────────────────────────────────────────────────────────

		events, nextCursor, err := repository.FilterEvents(pool, filter, limit, cursor)
		if err != nil {
			utils.InternalError(c, err, "filter events")
			return
		}

		// Build a human-readable summary of the active filters so the client
		// can render "Showing X events in Cape Town · This Weekend" etc.
		filterSummary := buildFilterSummary(filter, len(events))

		c.JSON(http.StatusOK, gin.H{
			"data":        events,
			"next_cursor": utils.EncodeCursor(nextCursor),
			"filter":      filterSummary,
		})
	}
}

// CitiesHandler returns a list of South African cities for the location picker.
//
//	GET /cities
//	GET /cities?q=cape
//
// Cities with active upcoming events are returned first (sorted by event count),
// followed by well-known SA cities that currently have no events.
//
// Response:
//
//	{
//	  "cities": [
//	    { "city": "Cape Town",     "event_count": 14 },
//	    { "city": "Johannesburg",  "event_count": 9  },
//	    { "city": "Durban",        "event_count": 0  }
//	  ]
//	}
func CitiesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		search := strings.TrimSpace(c.Query("q"))

		cities, err := repository.GetCitiesWithEvents(pool, search)
		if err != nil {
			utils.InternalError(c, err, "get cities")
			return
		}

		c.JSON(http.StatusOK, gin.H{"cities": cities})
	}
}

// filterSummary is the JSON shape returned in the "filter" key of the response
// so clients can render a summary of what filters are active.
type filterSummary struct {
	City        string `json:"city,omitempty"`
	Lat         *float64 `json:"lat,omitempty"`
	Lng         *float64 `json:"lng,omitempty"`
	RadiusKm    float64  `json:"radius_km,omitempty"`
	Date        string `json:"date,omitempty"`
	DateFrom    string `json:"date_from,omitempty"`
	DateTo      string `json:"date_to,omitempty"`
	TotalShown  int    `json:"total_shown"`
}

func buildFilterSummary(f repository.EventFilter, count int) filterSummary {
	s := filterSummary{
		City:       f.City,
		Lat:        f.Lat,
		Lng:        f.Lng,
		RadiusKm:   f.RadiusKm,
		Date:       f.DatePreset,
		TotalShown: count,
	}
	if f.DateFrom != nil {
		s.DateFrom = f.DateFrom.Format("2006-01-02")
	}
	if f.DateTo != nil {
		s.DateTo = f.DateTo.Format("2006-01-02")
	}
	if s.Date == "" {
		s.Date = "any"
	}
	return s
}