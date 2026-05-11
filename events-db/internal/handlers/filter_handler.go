package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"events-db/internal/cache"
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
// upcoming published events across South Africa, organised into sections.
//
// Location params (mutually exclusive — city takes precedence):
//
//	city=Cape+Town         filter by city name (case-insensitive)
//	province=Gauteng       filter by all cities in that province
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
// Response (same shape as /v1/home):
//
//	{
//	  "sections": [
//	    { "id": "cat-music", "title": "MUSIC", "events": [...], "next_cursor": "" },
//	    { "id": "free",      "title": "FREE EVENTS", "events": [...] }
//	  ],
//	  "filter": { "city": "Cape Town", "date": "weekend", "total_shown": 24 }
//	}
func FilterEventsHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── Location ──────────────────────────────────────────────────────────

		var filter repository.EventFilter

		city := strings.TrimSpace(c.Query("city"))
		if city != "" {
			filter.City = city
		} else if province := strings.TrimSpace(c.Query("province")); province != "" {
			filter.Province = province
		} else {
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

		// ── Cache (date-only, no location) ────────────────────────────────────
		// Location-scoped results are user/session-specific and must not be
		// shared. But date-only presets (tomorrow, weekend, any) are global and
		// identical for every anonymous request — cache them to avoid hitting
		// the DB on every date change in the filter modal.
		hasLocation := filter.City != "" || filter.Province != "" || filter.Lat != nil

		if !hasLocation && ec != nil && datePreset != "custom" {
			if cached, ok := ec.GetFilter(c.Request.Context(), datePreset); ok {
				totalShown := 0
				for _, sec := range cached {
					totalShown += len(sec.Events)
				}
				c.JSON(http.StatusOK, gin.H{
					"sections": cached,
					"filter":   buildFilterSummary(filter, totalShown),
				})
				return
			}
		}

		// ── Execute ───────────────────────────────────────────────────────────

		sections, err := repository.FilterEventsSections(pool, filter)
		if err != nil {
			utils.InternalError(c, err, "filter events sections")
			return
		}

		// Store date-only results in cache for subsequent requests
		if !hasLocation && ec != nil && datePreset != "custom" {
			ec.SetFilter(c.Request.Context(), datePreset, sections)
		}

		totalShown := 0
		for _, sec := range sections {
			totalShown += len(sec.Events)
		}

		filterSummary := buildFilterSummary(filter, totalShown)

		c.JSON(http.StatusOK, gin.H{
			"sections": sections,
			"filter":   filterSummary,
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
	City       string   `json:"city,omitempty"`
	Province   string   `json:"province,omitempty"`
	Lat        *float64 `json:"lat,omitempty"`
	Lng        *float64 `json:"lng,omitempty"`
	RadiusKm   float64  `json:"radius_km,omitempty"`
	Date       string   `json:"date,omitempty"`
	DateFrom   string   `json:"date_from,omitempty"`
	DateTo     string   `json:"date_to,omitempty"`
	TotalShown int      `json:"total_shown"`
}

func buildFilterSummary(f repository.EventFilter, count int) filterSummary {
	s := filterSummary{
		City:       f.City,
		Province:   f.Province,
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