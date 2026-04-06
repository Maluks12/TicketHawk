package repository

import (
	"context"
	"events-db/internal/models"
	"events-db/internal/utils"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EventFilter holds all optional filter parameters. Zero-values mean "no filter".
// DatePreset and DateFrom/DateTo are mutually exclusive — if DatePreset is set,
// DateFrom and DateTo are ignored.
type EventFilter struct {
	// Location — one of City OR (Lat+Lng), never both.
	// If both are empty, events from all cities are returned.
	City string

	// Lat/Lng — when provided, events within RadiusKm are returned.
	// If RadiusKm is 0 the default 50 km radius is used.
	Lat      *float64
	Lng      *float64
	RadiusKm float64

	// DatePreset is one of: "any", "tomorrow", "weekend", "custom".
	// "any" (or "") means no date constraint.
	DatePreset string

	// DateFrom/DateTo are used only when DatePreset == "custom".
	// Both must be provided together.
	DateFrom *time.Time
	DateTo   *time.Time
}

// FilterEvents returns a cursor-paginated list of published events matching
// the given filter. All filter parameters are optional — passing an empty
// EventFilter is equivalent to GetAllEvents.
//
// Query parameters map:
//
//	city=Cape+Town        → events in that city (case-insensitive ILIKE)
//	lat=X&lng=Y           → events within radius_km (default 50 km)
//	date=any              → no date constraint (default)
//	date=tomorrow         → events starting between midnight and end of tomorrow
//	date=weekend          → events starting this Sat–Sun
//	date=custom           → requires date_from and date_to (ISO 8601 date strings)
func FilterEvents(
	pool *pgxpool.Pool,
	f EventFilter,
	limit int,
	cursor *utils.Cursor,
) ([]models.Event, *utils.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished}
	clauses := []string{"e.status = $1"}

	// ── Location filter ───────────────────────────────────────────────────────

	if f.City != "" {
		args = append(args, f.City)
		clauses = append(clauses, fmt.Sprintf("e.city ILIKE $%d", len(args)))
	} else if f.Lat != nil && f.Lng != nil {
		radiusKm := f.RadiusKm
		if radiusKm <= 0 {
			radiusKm = 50
		}
		args = append(args, *f.Lat, *f.Lng, radiusKm)
		latIdx := len(args) - 2
		lngIdx := len(args) - 1
		radIdx := len(args)
		clauses = append(clauses, fmt.Sprintf(`
			e.latitude IS NOT NULL AND e.longitude IS NOT NULL
			AND (
				6371 * acos(LEAST(1.0,
					cos(radians($%d)) * cos(radians(e.latitude))
					* cos(radians(e.longitude) - radians($%d))
					+ sin(radians($%d)) * sin(radians(e.latitude))
				))
			) < $%d
		`, latIdx, lngIdx, latIdx, radIdx))
	}

	// ── Date filter ───────────────────────────────────────────────────────────

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	switch strings.ToLower(f.DatePreset) {
	case "tomorrow":
		tomorrow := today.AddDate(0, 0, 1)
		dayAfter := today.AddDate(0, 0, 2)
		args = append(args, tomorrow, dayAfter)
		fromIdx := len(args) - 1
		toIdx := len(args)
		clauses = append(clauses,
			fmt.Sprintf("e.start_time >= $%d AND e.start_time < $%d", fromIdx, toIdx),
		)

	case "weekend":
		var weekendStart, weekendEnd time.Time
		switch now.Weekday() {
		case time.Saturday:
			weekendStart = today
			weekendEnd = today.AddDate(0, 0, 2)
		case time.Sunday:
			weekendStart = today
			weekendEnd = today.AddDate(0, 0, 1)
		default:
			daysUntilSat := int(time.Saturday) - int(now.Weekday())
			weekendStart = today.AddDate(0, 0, daysUntilSat)
			weekendEnd = weekendStart.AddDate(0, 0, 2)
		}
		args = append(args, weekendStart, weekendEnd)
		fromIdx := len(args) - 1
		toIdx := len(args)
		clauses = append(clauses,
			fmt.Sprintf("e.start_time >= $%d AND e.start_time < $%d", fromIdx, toIdx),
		)

	case "custom":
		if f.DateFrom != nil && f.DateTo != nil {
			dateToEndOfDay := time.Date(
				f.DateTo.Year(), f.DateTo.Month(), f.DateTo.Day(),
				23, 59, 59, 0, f.DateTo.Location(),
			)
			args = append(args, *f.DateFrom, dateToEndOfDay)
			fromIdx := len(args) - 1
			toIdx := len(args)
			clauses = append(clauses,
				fmt.Sprintf("e.start_time >= $%d AND e.start_time <= $%d", fromIdx, toIdx),
			)
		}
	}

	// ── Cursor ────────────────────────────────────────────────────────────────
	// ORDER BY is ASC so the next page needs rows strictly greater than the
	// last item on the current page — use > not <.

	if cursor != nil {
		args = append(args, cursor.StartTime, cursor.ID)
		startIdx := len(args) - 1
		idIdx := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(e.start_time > $%d OR (e.start_time = $%d AND e.id > $%d))",
			startIdx, startIdx, idIdx,
		))
	}

	// ── Build query ───────────────────────────────────────────────────────────

	limitIdx := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			e.id, COALESCE(e.slug, '') AS slug, e.title, e.description, e.cover_image_url,
			e.start_time, e.end_time, e.price, e.currency,
			e.city, e.venue_name, e.max_capacity, e.tickets_sold,
			e.is_featured, e.status, e.views, e.created_at,
			c.name AS category
		FROM events e
		JOIN categories c ON c.id = e.category_id
		WHERE %s
		ORDER BY e.start_time ASC, e.id ASC
		LIMIT $%d
	`, strings.Join(clauses, " AND "), limitIdx)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("FilterEvents: %w", err)
	}
	defer rows.Close()

	events := make([]models.Event, 0, limit+1)
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.Slug, &e.Title, &e.Description, &e.CoverImageURL,
			&e.StartTime, &e.EndTime, &e.Price, &e.Currency,
			&e.City, &e.VenueName, &e.MaxCapacity, &e.TicketsSold,
			&e.IsFeatured, &e.Status, &e.Views, &e.CreatedAt,
			&e.Category,
		); err != nil {
			return nil, nil, fmt.Errorf("FilterEvents scan: %w", err)
		}
		e.ComputeAvailableTickets()
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
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

// GetCitiesWithEvents returns all distinct cities that have at least one
// published upcoming event, optionally filtered by a search query.
// Results are sorted by event count (busiest cities first) so the most
// relevant cities surface at the top of the picker.
//
// The static SA cities list is merged in so cities appear even before
// any events are created there — they just sort to the bottom.
func GetCitiesWithEvents(pool *pgxpool.Pool, search string) ([]CityResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []interface{}{models.EventPublished, time.Now()}
	whereSearch := ""

	if search != "" {
		args = append(args, "%"+search+"%")
		whereSearch = fmt.Sprintf("AND city ILIKE $%d", len(args))
	}

	query := fmt.Sprintf(`
		SELECT city, COUNT(*) AS event_count
		FROM events
		WHERE status = $1
		  AND start_time > $2
		  AND city IS NOT NULL
		  AND city <> ''
		  %s
		GROUP BY city
		ORDER BY event_count DESC, city ASC
		LIMIT 50
	`, whereSearch)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("GetCitiesWithEvents: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	results := make([]CityResult, 0, 20)

	for rows.Next() {
		var r CityResult
		if err := rows.Scan(&r.City, &r.EventCount); err != nil {
			return nil, fmt.Errorf("GetCitiesWithEvents scan: %w", err)
		}
		seen[strings.ToLower(r.City)] = true
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, city := range southAfricanCities {
		if search != "" && !strings.Contains(strings.ToLower(city), strings.ToLower(search)) {
			continue
		}
		if seen[strings.ToLower(city)] {
			continue
		}
		results = append(results, CityResult{City: city, EventCount: 0})
	}

	return results, nil
}

// CityResult is returned by GetCitiesWithEvents.
type CityResult struct {
	City       string `json:"city"`
	EventCount int64  `json:"event_count"`
}

// southAfricanCities is the canonical list of SA cities shown in the location
// picker. Cities with active events will already appear at the top from the DB
// query; these fill in the remainder so the picker is never empty.
var southAfricanCities = []string{
	// Gauteng
	"Johannesburg",
	"Pretoria",
	"Sandton",
	"Soweto",
	"Midrand",
	"Centurion",
	"Randburg",
	"Roodepoort",
	"Boksburg",
	"Benoni",
	"Germiston",
	"Krugersdorp",
	// Western Cape
	"Cape Town",
	"Stellenbosch",
	"Paarl",
	"George",
	"Knysna",
	"Mossel Bay",
	"Hermanus",
	"Worcester",
	// KwaZulu-Natal
	"Durban",
	"Pietermaritzburg",
	"Richards Bay",
	"Newcastle",
	"Umhlanga",
	"Ballito",
	// Eastern Cape
	"Port Elizabeth",
	"East London",
	"Grahamstown",
	"Mthatha",
	"Uitenhage",
	// Free State
	"Bloemfontein",
	"Welkom",
	"Bethlehem",
	// North West
	"Rustenburg",
	"Mahikeng",
	"Klerksdorp",
	// Limpopo
	"Polokwane",
	"Tzaneen",
	"Mokopane",
	// Mpumalanga
	"Nelspruit",
	"Witbank",
	"Secunda",
	// Northern Cape
	"Kimberley",
	"Upington",
	"Springbok",
}