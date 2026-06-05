package repository

import (
	"context"
	"events-db/internal/models"
	"events-db/internal/utils"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EventFilter holds all optional filter parameters. Zero-values mean "no filter".
// DatePreset and DateFrom/DateTo are mutually exclusive — if DatePreset is set,
// DateFrom and DateTo are ignored.
type EventFilter struct {
	// Location — one of City, Province, or (Lat+Lng), never more than one.
	// If all are empty, events from all cities are returned.
	City string

	// Province — filters by all cities belonging to this SA province.
	// Ignored when City is set.
	Province string

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

// provinceCities maps each South African province to the cities stored in the
// events table. This mirrors the PROVINCE_CITIES map in FilterModal.tsx so that
// selecting a province on the client filters to the same set of cities here.
var provinceCities = map[string][]string{
	"Gauteng":       {"Johannesburg", "Pretoria", "Sandton", "Soweto", "Midrand", "Centurion", "Randburg", "Roodepoort", "Boksburg", "Benoni", "Germiston", "Krugersdorp"},
	"Western Cape":  {"Cape Town", "Stellenbosch", "Paarl", "George", "Knysna", "Mossel Bay", "Hermanus", "Worcester"},
	"KwaZulu-Natal": {"Durban", "Pietermaritzburg", "Richards Bay", "Newcastle", "Umhlanga", "Ballito"},
	"Eastern Cape":  {"Port Elizabeth", "East London", "Mthatha", "Grahamstown", "Uitenhage"},
	"Limpopo":       {"Polokwane", "Tzaneen", "Phalaborwa", "Louis Trichardt"},
	"Mpumalanga":    {"Nelspruit", "Witbank", "Secunda", "Barberton"},
	"North West":    {"Rustenburg", "Klerksdorp", "Mafikeng", "Potchefstroom"},
	"Free State":    {"Bloemfontein", "Welkom", "Sasolburg", "Kroonstad"},
	"Northern Cape": {"Kimberley", "Upington", "Springbok"},
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
		args = append(args, strings.ToLower(f.City))
		clauses = append(clauses, fmt.Sprintf("LOWER(e.city) = $%d", len(args)))
	} else if f.Province != "" {
		// Map province name → list of cities (case-insensitive lookup).
		var cities []string
		for prov, cs := range provinceCities {
			if strings.EqualFold(prov, f.Province) {
				cities = cs
				break
			}
		}
		if len(cities) > 0 {
			// Build individual LOWER(e.city) = $N params — one per city.
			// This is more reliable than ILIKE ANY(ARRAY[$2,$3,...]) because
			// pgx/v5 can struggle with type inference on multi-param ARRAY
			// constructors, causing the clause to silently match nothing.
			orClauses := make([]string, len(cities))
			for i, city := range cities {
				args = append(args, strings.ToLower(city))
				orClauses[i] = fmt.Sprintf("LOWER(e.city) = $%d", len(args))
			}
			clauses = append(clauses, "("+strings.Join(orClauses, " OR ")+")")
		}
		// Unrecognised province → no clause → return all events (safe fallback).
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
			e.id, COALESCE(e.slug, '') AS slug, e.title, e.description,
			e.cover_image_url, e.banner_image_url,
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
			&e.ID, &e.Slug, &e.Title, &e.Description,
			&e.CoverImageURL, &e.BannerImageURL,
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
// ── Filtered Sections ─────────────────────────────────────────────────────────

// FilterEventsSections runs the same filter query as FilterEvents but returns
// results pre-organised into HomeSection rows — the same structure the home
// feed uses — so the frontend can render them directly without any grouping.
//
// Netflix-style section rules:
//   - One section per category, up to 10 events each
//   - A section only appears when it has ≥ 3 events (no thin rows)
//   - Sections sorted largest → smallest (most content-rich first)
//   - A "Free Events" section is appended when ≥ 2 free events are present
//   - If no category clears the threshold, a single "Results" fallback is used
//
// We fetch up to 200 matching events so the grouping is meaningful. Sections
// are the pagination unit — the client can load more per section via the
// existing /events/filter cursor endpoint if needed.
func FilterEventsSections(
	pool *pgxpool.Pool,
	f EventFilter,
) ([]HomeSection, error) {
	// Pull up to 200 events — enough to fill several category rows without
	// making the response unreasonably large.
	events, _, err := FilterEvents(pool, f, 200, nil)
	if err != nil {
		return nil, fmt.Errorf("FilterEventsSections: %w", err)
	}
	return buildFilteredSections(events), nil
}

// buildFilteredSections groups a flat event slice into HomeSection rows.
// It is a pure function — no DB access — so it is easy to unit-test.
func buildFilteredSections(events []models.Event) []HomeSection {
	type catGroup struct {
		name   string
		events []models.Event
	}

	// Preserve first-seen order while grouping by normalised category.
	order := make([]string, 0)
	groups := make(map[string]*catGroup)

	for i := range events {
		e := &events[i]
		cat := titleCase(strings.TrimSpace(e.Category))
		if cat == "" {
			cat = "Other"
		}
		if _, ok := groups[cat]; !ok {
			groups[cat] = &catGroup{name: cat}
			order = append(order, cat)
		}
		if len(groups[cat].events) < 10 {
			groups[cat].events = append(groups[cat].events, *e)
		}
	}

	// Build sections — only keep categories with ≥ 3 events.
	sections := make([]HomeSection, 0, len(order)+1)
	for _, cat := range order {
		g := groups[cat]
		if len(g.events) < 3 {
			continue
		}
		sections = append(sections, HomeSection{
			ID:         "cat-" + strings.ToLower(strings.ReplaceAll(cat, " ", "-")),
			Title:      strings.ToUpper(cat),
			Events:     g.events,
			NextCursor: "",
		})
	}

	// Sort: most events first.
	sort.Slice(sections, func(i, j int) bool {
		return len(sections[i].Events) > len(sections[j].Events)
	})

	// Free Events — collect up to 10 and inject after category rows.
	freeEvents := make([]models.Event, 0, 10)
	for _, e := range events {
		if e.Price == 0 {
			freeEvents = append(freeEvents, e)
			if len(freeEvents) == 10 {
				break
			}
		}
	}
	if len(freeEvents) >= 2 {
		sections = append(sections, HomeSection{
			ID:         "free",
			Title:      "FREE EVENTS",
			Events:     freeEvents,
			NextCursor: "",
		})
	}

	// Absolute fallback — never return a blank screen.
	if len(sections) == 0 && len(events) > 0 {
		cap := 10
		if len(events) < cap {
			cap = len(events)
		}
		sections = append(sections, HomeSection{
			ID:         "results",
			Title:      "RESULTS",
			Events:     events[:cap],
			NextCursor: "",
		})
	}

	return sections
}

// titleCase converts "music" → "Music", "tech & business" → "Tech & Business".
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}