 package handlers_test

// // =============================================================================
// // FILTER & CITIES TESTS
// // =============================================================================
// //
// // Tests covered:
// //
// //   GET /events/filter → FilterEventsHandler
// //     Location filters:
// //       - no filter returns all published events across all cities
// //       - city filter (case-insensitive) returns only matching city events
// //       - unknown city returns empty results
// //       - lat/lng proximity filter returns only nearby events
// //       - city takes precedence over lat/lng when both are supplied
// //       - lat without lng (and vice-versa) is ignored — no filter applied
// //       - drafts are never returned regardless of filter
// //
// //     Date filters:
// //       - date=any (or omitted) returns all upcoming published events
// //       - date=tomorrow returns only events starting tomorrow
// //       - date=weekend returns only events starting this Sat-Sun
// //       - date=custom with date_from and date_to returns events in range
// //       - date=custom without date_from or date_to returns 400
// //       - date_to before date_from returns 400
// //       - invalid date_from format returns 400
// //
// //     Combined:
// //       - city + date=weekend returns intersection
// //       - limit and cursor pagination work correctly
// //
// //     Response shape:
// //       - response includes "data", "next_cursor" and "filter" summary key
// //       - filter summary reflects active city, date preset and total_shown
// //
// //   GET /cities → CitiesHandler
// //     - returns at least the static SA city list when no events exist
// //     - cities with events sort above cities with zero events
// //     - q= search filters to matching cities (case-insensitive)
// //     - q= with no matches returns an empty list
// //
// // Prerequisites:
// //
// //   docker run -d --name test-postgres \
// //     -e POSTGRES_USER=testuser \
// //     -e POSTGRES_PASSWORD=testpass \
// //     -e POSTGRES_DB=test_db \
// //     -p 5433:5432 postgres:15
// //
// // Run:
// //   go test ./internal/handlers/... -v -run TestFilter -count=1
// //   go test ./internal/handlers/... -v -run TestCities -count=1
// // =============================================================================

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"net/http"
// 	"testing"
// 	"time"

// 	"events-db/internal/handlers"

// 	"github.com/gin-gonic/gin"
// 	"github.com/jackc/pgx/v5/pgxpool"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// )

// // ─── DB setup ─────────────────────────────────────────────────────────────────

// func setupFilterTestDB(t *testing.T) *pgxpool.Pool {
// 	t.Helper()
// 	pool, err := pgxpool.New(context.Background(), authTestDBURL)
// 	if err != nil {
// 		t.Fatalf(
// 			"Cannot connect to test DB: %v\n"+
// 				"Start it with:\n"+
// 				"  docker run -d --name test-postgres"+
// 				" -e POSTGRES_USER=testuser"+
// 				" -e POSTGRES_PASSWORD=testpass"+
// 				" -e POSTGRES_DB=test_db"+
// 				" -p 5433:5432 postgres:15",
// 			err,
// 		)
// 	}
// 	if pingErr := pool.Ping(context.Background()); pingErr != nil {
// 		pool.Close()
// 		t.Fatalf("filter DB not reachable: %v", pingErr)
// 	}
// 	createFilterTestTables(t, pool)
// 	cleanFilterTables(t, pool)
// 	return pool
// }

// func createFilterTestTables(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	ctx := context.Background()

// 	drops := []string{
// 		"DROP TABLE IF EXISTS bookings CASCADE",
// 		"DROP TABLE IF EXISTS events CASCADE",
// 		"DROP TABLE IF EXISTS categories CASCADE",
// 		"DROP TABLE IF EXISTS refresh_tokens CASCADE",
// 		"DROP TABLE IF EXISTS users CASCADE",
// 	}
// 	for _, s := range drops {
// 		pool.Exec(ctx, s)
// 	}

// 	stmts := []string{
// 		`CREATE TABLE users (
// 			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			email      VARCHAR(255) UNIQUE NOT NULL,
// 			password   TEXT NOT NULL,
// 			role       VARCHAR(50) NOT NULL DEFAULT 'attendee',
// 			created_at TIMESTAMPTZ DEFAULT NOW(),
// 			updated_at TIMESTAMPTZ DEFAULT NOW()
// 		)`,

// 		`CREATE TABLE refresh_tokens (
// 			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
// 			token_hash TEXT UNIQUE NOT NULL,
// 			expires_at TIMESTAMPTZ NOT NULL,
// 			revoked    BOOLEAN NOT NULL DEFAULT FALSE,
// 			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
// 		)`,

// 		`CREATE TABLE categories (
// 			id   SERIAL PRIMARY KEY,
// 			name VARCHAR(100) UNIQUE NOT NULL,
// 			slug VARCHAR(100) UNIQUE NOT NULL
// 		)`,

// 		`CREATE TABLE events (
// 			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			slug             TEXT,
// 			title            VARCHAR(255) NOT NULL,
// 			description      TEXT NOT NULL DEFAULT '',
// 			cover_image_url  TEXT,
// 			banner_image_url TEXT,
// 			banner_video_url TEXT,
// 			start_time       TIMESTAMPTZ NOT NULL,
// 			end_time         TIMESTAMPTZ NOT NULL,
// 			expires_at       TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
// 			timezone         VARCHAR(50) NOT NULL DEFAULT 'Africa/Johannesburg',
// 			venue_name       VARCHAR(255) NOT NULL DEFAULT '',
// 			address          TEXT NOT NULL DEFAULT '',
// 			city             VARCHAR(255) NOT NULL DEFAULT '',
// 			latitude         DOUBLE PRECISION,
// 			longitude        DOUBLE PRECISION,
// 			category_id      INTEGER NOT NULL REFERENCES categories(id),
// 			price            DECIMAL(10,2) NOT NULL DEFAULT 0.00,
// 			currency         VARCHAR(3) NOT NULL DEFAULT 'ZAR',
// 			max_capacity     INTEGER,
// 			tickets_sold     INTEGER NOT NULL DEFAULT 0,
// 			reserved_count   INTEGER NOT NULL DEFAULT 0,
// 			allow_waitlist   BOOLEAN DEFAULT TRUE,
// 			status           VARCHAR(50) NOT NULL DEFAULT 'draft',
// 			is_featured      BOOLEAN DEFAULT FALSE,
// 			featured_until   TIMESTAMPTZ,
// 			organizer_id     UUID REFERENCES users(id),
// 			published_at     TIMESTAMPTZ,
// 			cancelled_at     TIMESTAMPTZ,
// 			cancellation_reason TEXT,
// 			refund_policy    TEXT,
// 			terms_and_conditions TEXT,
// 			tags             JSONB,
// 			views            BIGINT DEFAULT 0,
// 			search_vector    TSVECTOR,
// 			created_at       TIMESTAMPTZ DEFAULT NOW(),
// 			updated_at       TIMESTAMPTZ DEFAULT NOW()
// 		)`,
// 	}

// 	for _, s := range stmts {
// 		if _, err := pool.Exec(ctx, s); err != nil {
// 			t.Fatalf("filter table setup failed:\n  sql: %s\n  err: %v", s, err)
// 		}
// 	}
// }

// func cleanFilterTables(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	ctx := context.Background()
// 	pool.Exec(ctx, "TRUNCATE events CASCADE")
// 	pool.Exec(ctx, "TRUNCATE categories CASCADE")
// 	pool.Exec(ctx, "TRUNCATE refresh_tokens CASCADE")
// 	pool.Exec(ctx, "TRUNCATE users CASCADE")
// }

// // ─── Router ───────────────────────────────────────────────────────────────────

// func buildFilterRouter(pool *pgxpool.Pool) *gin.Engine {
// 	gin.SetMode(gin.TestMode)
// 	r := gin.New()

// 	r.GET("/events/filter", handlers.FilterEventsHandler(pool))
// 	r.GET("/cities", handlers.CitiesHandler(pool))
// 	return r
// }

// // ─── Seed helpers ─────────────────────────────────────────────────────────────

// func filterSeedCategory(t *testing.T, pool *pgxpool.Pool) int {
// 	t.Helper()
// 	var id int
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO categories (name, slug)
// 		VALUES (
// 			'Cat-'||substr(gen_random_uuid()::text,1,8),
// 			'cat-'||substr(gen_random_uuid()::text,1,8)
// 		) RETURNING id
// 	`).Scan(&id)
// 	require.NoError(t, err)
// 	return id
// }

// // filterSeedOrganizer inserts a bare organizer user row and returns its ID.
// func filterSeedOrganizer(t *testing.T, pool *pgxpool.Pool) string {
// 	t.Helper()
// 	var id string
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO users (email, password, role)
// 		VALUES ($1, 'x', 'organizer') RETURNING id
// 	`, randomEmail()).Scan(&id)
// 	require.NoError(t, err)
// 	return id
// }

// type filterSeedOpts struct {
// 	city      string
// 	lat       *float64
// 	lng       *float64
// 	startTime time.Time
// }

// func filterSeedEvent(t *testing.T, pool *pgxpool.Pool, orgID string, catID int, title string, opts filterSeedOpts) string {
// 	t.Helper()
// 	if opts.startTime.IsZero() {
// 		opts.startTime = time.Now().Add(7 * 24 * time.Hour)
// 	}
// 	endTime := opts.startTime.Add(2 * time.Hour)
// 	var id string
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO events (title, description, status, category_id, organizer_id,
// 		                    price, currency, venue_name, address, city,
// 		                    latitude, longitude,
// 		                    start_time, end_time, expires_at)
// 		VALUES ($1, 'Enough description text here.', 'published', $2, $3,
// 		        100.00, 'ZAR', 'Test Venue', '1 Test Rd', $4,
// 		        $5, $6,
// 		        $7, $8, NOW() + INTERVAL '60 days')
// 		RETURNING id
// 	`, title, catID, orgID, opts.city, opts.lat, opts.lng, opts.startTime, endTime).Scan(&id)
// 	require.NoError(t, err)
// 	return id
// }

// func filterSeedDraft(t *testing.T, pool *pgxpool.Pool, orgID string, catID int, city string) string {
// 	t.Helper()
// 	var id string
// 	pool.QueryRow(context.Background(), `
// 		INSERT INTO events (title, description, status, category_id, organizer_id,
// 		                    price, currency, venue_name, address, city,
// 		                    start_time, end_time, expires_at)
// 		VALUES ('Draft Event', 'description', 'draft', $1, $2,
// 		        0, 'ZAR', 'Venue', 'Addr', $3,
// 		        NOW() + INTERVAL '7 days', NOW() + INTERVAL '8 days', NOW() + INTERVAL '30 days')
// 		RETURNING id
// 	`, catID, orgID, city).Scan(&id)
// 	return id
// }

// // lat/lng pairs for SA cities used across tests
// var (
// 	latCapeTown = ptr(-33.9249)
// 	lngCapeTown = ptr(18.4241)
// 	latJHB      = ptr(-26.2041)
// 	lngJHB      = ptr(28.0473)
// 	latDurban   = ptr(-29.8587)
// 	lngDurban   = ptr(31.0218)
// )

// func ptr(v float64) *float64 { return &v }

// // nextSaturday returns the date of the next coming Saturday (or today if already Sat).
// func nextSaturday() time.Time {
// 	now := time.Now()
// 	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
// 	daysUntil := (int(time.Saturday) - int(today.Weekday()) + 7) % 7
// 	if daysUntil == 0 {
// 		daysUntil = 0 // already Saturday
// 	}
// 	return today.AddDate(0, 0, daysUntil)
// }

// func tomorrow() time.Time {
// 	now := time.Now()
// 	return time.Date(now.Year(), now.Month(), now.Day()+1, 12, 0, 0, 0, now.Location())
// }

// // ─── GET /events/filter — location ────────────────────────────────────────────

// func TestFilterLocation(t *testing.T) {
// 	pool := setupFilterTestDB(t)
// 	defer pool.Close()

// 	t.Run("no filter returns all published events", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		filterSeedEvent(t, pool, orgID, catID, "Cape Town Event", filterSeedOpts{city: "Cape Town"})
// 		filterSeedEvent(t, pool, orgID, catID, "Durban Event", filterSeedOpts{city: "Durban"})
// 		filterSeedEvent(t, pool, orgID, catID, "Johannesburg Event", filterSeedOpts{city: "Johannesburg"})

// 		w := doRequest(r, http.MethodGet, "/events/filter", nil)
// 		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 3)
// 	})

// 	t.Run("city filter returns only events in that city", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		filterSeedEvent(t, pool, orgID, catID, "Cape Event 1", filterSeedOpts{city: "Cape Town"})
// 		filterSeedEvent(t, pool, orgID, catID, "Cape Event 2", filterSeedOpts{city: "Cape Town"})
// 		filterSeedEvent(t, pool, orgID, catID, "JHB Event", filterSeedOpts{city: "Johannesburg"})

// 		w := doRequest(r, http.MethodGet, "/events/filter?city=Cape+Town", nil)
// 		require.Equal(t, http.StatusOK, w.Code)

// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 2)
// 		for _, row := range data {
// 			assert.Equal(t, "Cape Town", row.(map[string]interface{})["city"])
// 		}
// 	})

// 	t.Run("city filter is case-insensitive", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		filterSeedEvent(t, pool, orgID, catID, "Durban Jazz", filterSeedOpts{city: "Durban"})

// 		w := doRequest(r, http.MethodGet, "/events/filter?city=durban", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 1)
// 	})

// 	t.Run("unknown city returns empty results", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		filterSeedEvent(t, pool, orgID, catID, "JHB Festival", filterSeedOpts{city: "Johannesburg"})

// 		w := doRequest(r, http.MethodGet, "/events/filter?city=Timbuktu", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 0)
// 	})

// 	t.Run("lat/lng proximity filter returns only nearby events", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		// Cape Town coords — should match Cape Town event, not Durban event (>1000 km away)
// 		filterSeedEvent(t, pool, orgID, catID, "Near Cape Town", filterSeedOpts{
// 			city: "Cape Town", lat: latCapeTown, lng: lngCapeTown,
// 		})
// 		filterSeedEvent(t, pool, orgID, catID, "Far Durban", filterSeedOpts{
// 			city: "Durban", lat: latDurban, lng: lngDurban,
// 		})

// 		url := fmt.Sprintf("/events/filter?lat=%f&lng=%f&radius_km=100", *latCapeTown, *lngCapeTown)
// 		w := doRequest(r, http.MethodGet, url, nil)
// 		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		require.Len(t, data, 1)
// 		assert.Equal(t, "Near Cape Town", data[0].(map[string]interface{})["title"])
// 	})

// 	t.Run("city takes precedence over lat/lng when both supplied", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		filterSeedEvent(t, pool, orgID, catID, "JHB Event", filterSeedOpts{
// 			city: "Johannesburg", lat: latJHB, lng: lngJHB,
// 		})
// 		filterSeedEvent(t, pool, orgID, catID, "Cape Town Event", filterSeedOpts{
// 			city: "Cape Town", lat: latCapeTown, lng: lngCapeTown,
// 		})

// 		// Cape Town coords + city=Johannesburg — city wins, only JHB event returned
// 		url := fmt.Sprintf("/events/filter?city=Johannesburg&lat=%f&lng=%f", *latCapeTown, *lngCapeTown)
// 		w := doRequest(r, http.MethodGet, url, nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		require.Len(t, data, 1)
// 		assert.Equal(t, "Johannesburg", data[0].(map[string]interface{})["city"])
// 	})

// 	t.Run("lat without lng is ignored — all events returned", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		filterSeedEvent(t, pool, orgID, catID, "Event A", filterSeedOpts{city: "Johannesburg"})
// 		filterSeedEvent(t, pool, orgID, catID, "Event B", filterSeedOpts{city: "Cape Town"})

// 		url := fmt.Sprintf("/events/filter?lat=%f", *latCapeTown) // no lng
// 		w := doRequest(r, http.MethodGet, url, nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 2, "incomplete lat/lng should not filter")
// 	})

// 	t.Run("drafts are never returned even when city matches", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		filterSeedEvent(t, pool, orgID, catID, "Published Cape", filterSeedOpts{city: "Cape Town"})
// 		filterSeedDraft(t, pool, orgID, catID, "Cape Town")

// 		w := doRequest(r, http.MethodGet, "/events/filter?city=Cape+Town", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 1)
// 		assert.Equal(t, "Published Cape", data[0].(map[string]interface{})["title"])
// 	})
// }

// // ─── GET /events/filter — date ────────────────────────────────────────────────

// func TestFilterDate(t *testing.T) {
// 	pool := setupFilterTestDB(t)
// 	defer pool.Close()

// 	t.Run("date=any (or omitted) returns all published upcoming events", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		filterSeedEvent(t, pool, orgID, catID, "Event A", filterSeedOpts{startTime: time.Now().Add(48 * time.Hour)})
// 		filterSeedEvent(t, pool, orgID, catID, "Event B", filterSeedOpts{startTime: time.Now().Add(72 * time.Hour)})

// 		w := doRequest(r, http.MethodGet, "/events/filter?date=any", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 2)
// 	})

// 	t.Run("date=tomorrow returns only events starting tomorrow", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		tomorrowStart := tomorrow()
// 		filterSeedEvent(t, pool, orgID, catID, "Tomorrow Event", filterSeedOpts{startTime: tomorrowStart})
// 		filterSeedEvent(t, pool, orgID, catID, "Next Week Event", filterSeedOpts{startTime: time.Now().Add(7 * 24 * time.Hour)})

// 		w := doRequest(r, http.MethodGet, "/events/filter?date=tomorrow", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 1)
// 		assert.Equal(t, "Tomorrow Event", data[0].(map[string]interface{})["title"])
// 	})

// 	t.Run("date=weekend returns only events on the coming Saturday/Sunday", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		sat := nextSaturday()
// 		sun := sat.AddDate(0, 0, 1)

// 		filterSeedEvent(t, pool, orgID, catID, "Saturday Gig", filterSeedOpts{startTime: sat})
// 		filterSeedEvent(t, pool, orgID, catID, "Sunday Market", filterSeedOpts{startTime: sun})
// 		filterSeedEvent(t, pool, orgID, catID, "Monday Meeting", filterSeedOpts{startTime: sun.AddDate(0, 0, 1)})

// 		w := doRequest(r, http.MethodGet, "/events/filter?date=weekend", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 2, "should contain only Saturday and Sunday events")
// 		titles := []string{
// 			data[0].(map[string]interface{})["title"].(string),
// 			data[1].(map[string]interface{})["title"].(string),
// 		}
// 		assert.Contains(t, titles, "Saturday Gig")
// 		assert.Contains(t, titles, "Sunday Market")
// 	})

// 	t.Run("date=custom with valid range returns events within range", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		base := time.Now()
// 		inRange := base.AddDate(0, 0, 10)
// 		outRange := base.AddDate(0, 0, 40)

// 		filterSeedEvent(t, pool, orgID, catID, "In Range", filterSeedOpts{startTime: inRange})
// 		filterSeedEvent(t, pool, orgID, catID, "Out Of Range", filterSeedOpts{startTime: outRange})

// 		from := base.AddDate(0, 0, 5).Format("2006-01-02")
// 		to := base.AddDate(0, 0, 20).Format("2006-01-02")
// 		w := doRequest(r, http.MethodGet, fmt.Sprintf("/events/filter?date=custom&date_from=%s&date_to=%s", from, to), nil)
// 		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 1)
// 		assert.Equal(t, "In Range", data[0].(map[string]interface{})["title"])
// 	})

// 	t.Run("date=custom without date_from or date_to returns 400", func(t *testing.T) {
// 		r := buildFilterRouter(pool)
// 		w := doRequest(r, http.MethodGet, "/events/filter?date=custom", nil)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("date=custom missing date_to returns 400", func(t *testing.T) {
// 		r := buildFilterRouter(pool)
// 		w := doRequest(r, http.MethodGet, "/events/filter?date=custom&date_from=2025-08-01", nil)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("date_to before date_from returns 400", func(t *testing.T) {
// 		r := buildFilterRouter(pool)
// 		w := doRequest(r, http.MethodGet, "/events/filter?date=custom&date_from=2025-08-20&date_to=2025-08-01", nil)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("invalid date_from format returns 400", func(t *testing.T) {
// 		r := buildFilterRouter(pool)
// 		w := doRequest(r, http.MethodGet, "/events/filter?date=custom&date_from=01-08-2025&date_to=2025-08-31", nil)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})
// }

// // ─── GET /events/filter — combined + response shape ───────────────────────────

// func TestFilterCombined(t *testing.T) {
// 	pool := setupFilterTestDB(t)
// 	defer pool.Close()

// 	t.Run("city + date=weekend returns intersection only", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		sat := nextSaturday()
// 		sun := sat.AddDate(0, 0, 1)

// 		// Only this should match: Cape Town AND weekend
// 		filterSeedEvent(t, pool, orgID, catID, "Cape Town Saturday", filterSeedOpts{
// 			city: "Cape Town", startTime: sat,
// 		})
// 		// Wrong city
// 		filterSeedEvent(t, pool, orgID, catID, "JHB Saturday", filterSeedOpts{
// 			city: "Johannesburg", startTime: sat,
// 		})
// 		// Wrong day (Sunday is still weekend)
// 		filterSeedEvent(t, pool, orgID, catID, "Cape Town Sunday", filterSeedOpts{
// 			city: "Cape Town", startTime: sun,
// 		})
// 		// Wrong city AND wrong day
// 		filterSeedEvent(t, pool, orgID, catID, "JHB Monday", filterSeedOpts{
// 			city: "Johannesburg", startTime: sun.AddDate(0, 0, 1),
// 		})

// 		w := doRequest(r, http.MethodGet, "/events/filter?city=Cape+Town&date=weekend", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		// Saturday + Sunday Cape Town events
// 		assert.Len(t, data, 2)
// 	})

// 	t.Run("response includes filter summary with active city and date preset", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		r := buildFilterRouter(pool)

// 		w := doRequest(r, http.MethodGet, "/events/filter?city=Pretoria&date=tomorrow", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

// 		require.Contains(t, resp, "filter", "response must include a filter summary key")
// 		filter := resp["filter"].(map[string]interface{})
// 		assert.Equal(t, "Pretoria", filter["city"])
// 		assert.Equal(t, "tomorrow", filter["date"])
// 		assert.Contains(t, filter, "total_shown")
// 	})

// 	t.Run("response includes data, next_cursor and filter keys", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		r := buildFilterRouter(pool)

// 		w := doRequest(r, http.MethodGet, "/events/filter", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		assert.Contains(t, resp, "data")
// 		assert.Contains(t, resp, "next_cursor")
// 		assert.Contains(t, resp, "filter")
// 	})

// 	t.Run("limit param is respected", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		for i := 0; i < 5; i++ {
// 			filterSeedEvent(t, pool, orgID, catID, fmt.Sprintf("Event %d", i), filterSeedOpts{
// 				city:      "Johannesburg",
// 				startTime: time.Now().Add(time.Duration(i+1) * 24 * time.Hour),
// 			})
// 		}

// 		w := doRequest(r, http.MethodGet, "/events/filter?limit=3", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		data := resp["data"].([]interface{})
// 		assert.Len(t, data, 3)
// 		assert.NotEmpty(t, resp["next_cursor"], "next_cursor must be set when more pages exist")
// 	})

// 	t.Run("cursor pagination returns next page without overlap", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		for i := 0; i < 5; i++ {
// 			filterSeedEvent(t, pool, orgID, catID, fmt.Sprintf("Pg Event %d", i), filterSeedOpts{
// 				city:      "Durban",
// 				startTime: time.Now().Add(time.Duration(i+1) * 24 * time.Hour),
// 			})
// 		}

// 		// Page 1
// 		w := doRequest(r, http.MethodGet, "/events/filter?city=Durban&limit=3", nil)
// 		var page1 map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page1))
// 		cursor := page1["next_cursor"].(string)
// 		require.NotEmpty(t, cursor)

// 		page1IDs := make(map[string]bool)
// 		for _, row := range page1["data"].([]interface{}) {
// 			page1IDs[row.(map[string]interface{})["id"].(string)] = true
// 		}

// 		// Page 2
// 		w = doRequest(r, http.MethodGet, fmt.Sprintf("/events/filter?city=Durban&limit=3&cursor=%s", cursor), nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var page2 map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page2))
// 		page2Data := page2["data"].([]interface{})
// 		assert.Len(t, page2Data, 2)
// 		for _, row := range page2Data {
// 			id := row.(map[string]interface{})["id"].(string)
// 			assert.False(t, page1IDs[id], "event %s appeared on both pages", id)
// 		}
// 	})

// 	t.Run("invalid cursor returns 400", func(t *testing.T) {
// 		r := buildFilterRouter(pool)
// 		w := doRequest(r, http.MethodGet, "/events/filter?cursor=notbase64!!", nil)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})
// }

// // ─── GET /cities ──────────────────────────────────────────────────────────────

// func TestCities(t *testing.T) {
// 	pool := setupFilterTestDB(t)
// 	defer pool.Close()

// 	t.Run("returns at least the static SA city list when no events exist", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		r := buildFilterRouter(pool)

// 		w := doRequest(r, http.MethodGet, "/cities", nil)
// 		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		cities := resp["cities"].([]interface{})
// 		assert.NotEmpty(t, cities, "static SA city list must always be returned")

// 		// Confirm at least one well-known SA city is present
// 		names := make([]string, 0, len(cities))
// 		for _, c := range cities {
// 			names = append(names, c.(map[string]interface{})["city"].(string))
// 		}
// 		assert.Contains(t, names, "Johannesburg")
// 		assert.Contains(t, names, "Cape Town")
// 		assert.Contains(t, names, "Durban")
// 	})

// 	t.Run("city with events surfaces above cities with zero events", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		orgID := filterSeedOrganizer(t, pool)
// 		catID := filterSeedCategory(t, pool)
// 		r := buildFilterRouter(pool)

// 		// Seed two Cape Town events — no other city has events
// 		filterSeedEvent(t, pool, orgID, catID, "Cape A", filterSeedOpts{city: "Cape Town"})
// 		filterSeedEvent(t, pool, orgID, catID, "Cape B", filterSeedOpts{city: "Cape Town"})

// 		w := doRequest(r, http.MethodGet, "/cities", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		cities := resp["cities"].([]interface{})
// 		require.NotEmpty(t, cities)

// 		// Cape Town must be first (highest event count)
// 		first := cities[0].(map[string]interface{})
// 		assert.Equal(t, "Cape Town", first["city"])
// 		assert.EqualValues(t, 2, first["event_count"])
// 	})

// 	t.Run("q= search filters cities by name case-insensitively", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		r := buildFilterRouter(pool)

// 		w := doRequest(r, http.MethodGet, "/cities?q=cape", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		cities := resp["cities"].([]interface{})
// 		require.NotEmpty(t, cities)

// 		for _, c := range cities {
// 			name := c.(map[string]interface{})["city"].(string)
// 			assert.Contains(t, name, "Cape", "all returned cities should contain 'Cape'")
// 		}
// 	})

// 	t.Run("q= search with no matches returns empty list", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		r := buildFilterRouter(pool)

// 		w := doRequest(r, http.MethodGet, "/cities?q=xyznonexistent", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		cities := resp["cities"].([]interface{})
// 		assert.Len(t, cities, 0)
// 	})

// 	t.Run("event_count is 0 for static cities with no active events", func(t *testing.T) {
// 		cleanFilterTables(t, pool)
// 		r := buildFilterRouter(pool)

// 		w := doRequest(r, http.MethodGet, "/cities?q=Polokwane", nil)
// 		require.Equal(t, http.StatusOK, w.Code)
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		cities := resp["cities"].([]interface{})
// 		require.Len(t, cities, 1)
// 		assert.EqualValues(t, 0, cities[0].(map[string]interface{})["event_count"])
// 	})
// }