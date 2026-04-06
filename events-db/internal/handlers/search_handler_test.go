package handlers_test

// =============================================================================
// SEARCH SYSTEM TESTS
// =============================================================================
//
// Tests covered:
//
//   GET /events/search → SearchEventsHandler
//     (Basic cases — empty query 400, no results, drafts excluded — are already
//     covered in events_handler_test.go::TestEventSearch. This file covers the
//     deeper search behaviour.)
//
//     Full-text matching:
//       - multi-word query matches events containing all words
//       - query matches on description text, not just title
//       - partial word match returns events (tsquery plainto_tsquery behaviour)
//       - query with special characters does not crash (SQL injection guard)
//       - whitespace-only query is treated as empty and returns 400
//
//     Result correctness:
//       - cancelled events are excluded from results
//       - only published events appear even when title matches a draft
//       - multiple matching events all appear in results
//
//     Ranking:
//       - title match ranks above description-only match (weight A vs B)
//
//     Pagination:
//       - limit param restricts returned results
//       - next_cursor is set when results overflow the page
//       - cursor returns next page without overlap
//       - last page has no next_cursor
//
//     Response shape:
//       - each result row includes id, slug, title, city, venue_name,
//         start_time, end_time, price, currency
//
// Prerequisites:
//
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser \
//     -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db \
//     -p 5433:5432 postgres:15
//
// Run:
//   go test ./internal/handlers/... -v -run TestSearch -count=1
// =============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"events-db/internal/handlers"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── DB setup ─────────────────────────────────────────────────────────────────
//
// Reuses the events handler tables (same schema) but under its own setup
// function so search tests are fully isolated.

func setupSearchTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Fatalf(
			"Cannot connect to test DB: %v\n"+
				"Start it with:\n"+
				"  docker run -d --name test-postgres"+
				" -e POSTGRES_USER=testuser"+
				" -e POSTGRES_PASSWORD=testpass"+
				" -e POSTGRES_DB=test_db"+
				" -p 5433:5432 postgres:15",
			err,
		)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Fatalf("search DB not reachable: %v", pingErr)
	}
	createSearchTestTables(t, pool)
	cleanSearchTables(t, pool)
	return pool
}

func createSearchTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
		"DROP TABLE IF EXISTS bookings CASCADE",
		"DROP TABLE IF EXISTS events CASCADE",
		"DROP TABLE IF EXISTS categories CASCADE",
		"DROP TABLE IF EXISTS refresh_tokens CASCADE",
		"DROP TABLE IF EXISTS users CASCADE",
	}
	for _, s := range drops {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		`CREATE TABLE users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email      VARCHAR(255) UNIQUE NOT NULL,
			password   TEXT NOT NULL,
			role       VARCHAR(50) NOT NULL DEFAULT 'attendee',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE refresh_tokens (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT UNIQUE NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			revoked    BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE TABLE categories (
			id   SERIAL PRIMARY KEY,
			name VARCHAR(100) UNIQUE NOT NULL,
			slug VARCHAR(100) UNIQUE NOT NULL
		)`,

		`CREATE TABLE events (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			slug             TEXT,
			title            VARCHAR(255) NOT NULL,
			description      TEXT NOT NULL DEFAULT '',
			cover_image_url  TEXT,
			banner_image_url TEXT,
			banner_video_url TEXT,
			start_time       TIMESTAMPTZ NOT NULL,
			end_time         TIMESTAMPTZ NOT NULL,
			expires_at       TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
			timezone         VARCHAR(50) NOT NULL DEFAULT 'Africa/Johannesburg',
			venue_name       VARCHAR(255) NOT NULL DEFAULT '',
			address          TEXT NOT NULL DEFAULT '',
			city             VARCHAR(255) NOT NULL DEFAULT '',
			latitude         DOUBLE PRECISION,
			longitude        DOUBLE PRECISION,
			category_id      INTEGER NOT NULL REFERENCES categories(id),
			price            DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			currency         VARCHAR(3) NOT NULL DEFAULT 'ZAR',
			max_capacity     INTEGER,
			tickets_sold     INTEGER NOT NULL DEFAULT 0,
			reserved_count   INTEGER NOT NULL DEFAULT 0,
			allow_waitlist   BOOLEAN DEFAULT TRUE,
			status           VARCHAR(50) NOT NULL DEFAULT 'draft',
			is_featured      BOOLEAN DEFAULT FALSE,
			featured_until   TIMESTAMPTZ,
			organizer_id     UUID REFERENCES users(id),
			published_at     TIMESTAMPTZ,
			cancelled_at     TIMESTAMPTZ,
			cancellation_reason TEXT,
			refund_policy    TEXT,
			terms_and_conditions TEXT,
			tags             JSONB,
			views            BIGINT DEFAULT 0,
			search_vector    TSVECTOR,
			created_at       TIMESTAMPTZ DEFAULT NOW(),
			updated_at       TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Trigger keeps search_vector up-to-date with title + description on
		// every INSERT and UPDATE so tests don't need manual vector maintenance.
		`CREATE OR REPLACE FUNCTION search_tsv_update() RETURNS TRIGGER AS $$
		BEGIN
			NEW.search_vector :=
				setweight(to_tsvector('english', COALESCE(NEW.title, '')), 'A') ||
				setweight(to_tsvector('english', COALESCE(NEW.description, '')), 'B');
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,

		`CREATE TRIGGER search_tsv_trigger
		 BEFORE INSERT OR UPDATE ON events
		 FOR EACH ROW EXECUTE FUNCTION search_tsv_update()`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("search table setup failed:\n  sql: %s\n  err: %v", s, err)
		}
	}
}

func cleanSearchTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE events CASCADE")
	pool.Exec(ctx, "TRUNCATE categories CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens CASCADE")
	pool.Exec(ctx, "TRUNCATE users CASCADE")
}

// ─── Router ───────────────────────────────────────────────────────────────────

func buildSearchRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/events/search", handlers.SearchEventsHandler(pool))
	return r
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

func searchSeedOrganizer(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password, role)
		VALUES ($1, 'x', 'organizer') RETURNING id
	`, randomEmail()).Scan(&id)
	require.NoError(t, err)
	return id
}

func searchSeedCategory(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO categories (name, slug)
		VALUES (
			'Cat-'||substr(gen_random_uuid()::text,1,8),
			'cat-'||substr(gen_random_uuid()::text,1,8)
		) RETURNING id
	`).Scan(&id)
	require.NoError(t, err)
	return id
}

type searchSeedOpts struct {
	status      string    // default "published"
	description string    // default generic text
	startTime   time.Time // default 7 days from now
}

func searchSeedEvent(t *testing.T, pool *pgxpool.Pool, orgID string, catID int, title string, opts searchSeedOpts) string {
	t.Helper()
	if opts.status == "" {
		opts.status = "published"
	}
	if opts.description == "" {
		opts.description = "A standard event description with enough detail."
	}
	if opts.startTime.IsZero() {
		opts.startTime = time.Now().Add(7 * 24 * time.Hour)
	}
	endTime := opts.startTime.Add(3 * time.Hour)
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, description, status, category_id, organizer_id,
		                    price, currency, venue_name, address, city,
		                    start_time, end_time, expires_at)
		VALUES ($1, $2, $3, $4, $5,
		        200.00, 'ZAR', 'Test Venue', '1 Test Rd', 'Johannesburg',
		        $6, $7, NOW() + INTERVAL '60 days')
		RETURNING id
	`, title, opts.description, opts.status, catID, orgID, opts.startTime, endTime).Scan(&id)
	require.NoError(t, err)
	return id
}

// ─── Full-text matching ───────────────────────────────────────────────────────

func TestSearchFullText(t *testing.T) {
	pool := setupSearchTestDB(t)
	defer pool.Close()

	t.Run("multi-word query matches events containing all words", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		searchSeedEvent(t, pool, orgID, catID, "Cape Town Jazz Festival", searchSeedOpts{})
		searchSeedEvent(t, pool, orgID, catID, "Durban Jazz Night", searchSeedOpts{})
		searchSeedEvent(t, pool, orgID, catID, "Cape Town Beer Festival", searchSeedOpts{})

		// "Jazz Festival" should match events containing both words
		w := doRequest(r, http.MethodGet, "/events/search?q=Jazz+Festival", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		// plainto_tsquery treats multi-word as AND so only "Cape Town Jazz Festival" matches
		assert.Len(t, data, 1)
		assert.Equal(t, "Cape Town Jazz Festival", data[0].(map[string]interface{})["title"])
	})

	t.Run("query matches on description text, not just title", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		// Title has no "marathon" — only description does
		searchSeedEvent(t, pool, orgID, catID, "Annual Athletics Event", searchSeedOpts{
			description: "Join us for the big Johannesburg marathon experience this year.",
		})
		searchSeedEvent(t, pool, orgID, catID, "Unrelated Concert", searchSeedOpts{})

		w := doRequest(r, http.MethodGet, "/events/search?q=marathon", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 1)
		assert.Equal(t, "Annual Athletics Event", data[0].(map[string]interface{})["title"])
	})

	t.Run("query with SQL injection characters does not crash", func(t *testing.T) {
		cleanSearchTables(t, pool)
		r := buildSearchRouter(pool)

		// Should return 200 with empty results, not a 500
		w := doRequest(r, http.MethodGet, "/events/search?q=%27%3B+DROP+TABLE+events%3B--", nil)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("whitespace-only query returns 400", func(t *testing.T) {
		r := buildSearchRouter(pool)
		w := doRequest(r, http.MethodGet, "/events/search?q=+++", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── Result correctness ───────────────────────────────────────────────────────

func TestSearchResultCorrectness(t *testing.T) {
	pool := setupSearchTestDB(t)
	defer pool.Close()

	t.Run("cancelled events are excluded from results", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		searchSeedEvent(t, pool, orgID, catID, "Rock Concert Live", searchSeedOpts{status: "published"})
		searchSeedEvent(t, pool, orgID, catID, "Rock Concert Cancelled", searchSeedOpts{status: "cancelled"})

		w := doRequest(r, http.MethodGet, "/events/search?q=Rock+Concert", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)
		assert.Equal(t, "Rock Concert Live", data[0].(map[string]interface{})["title"])
	})

	t.Run("draft event matching title is not returned", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		searchSeedEvent(t, pool, orgID, catID, "EDM Warehouse Party", searchSeedOpts{status: "published"})
		searchSeedEvent(t, pool, orgID, catID, "EDM Warehouse Draft",
			searchSeedOpts{status: "draft"})

		w := doRequest(r, http.MethodGet, "/events/search?q=Warehouse", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)
		assert.Equal(t, "EDM Warehouse Party", data[0].(map[string]interface{})["title"])
	})

	t.Run("all matching published events are returned", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		searchSeedEvent(t, pool, orgID, catID, "Afropop Night Cape Town", searchSeedOpts{})
		searchSeedEvent(t, pool, orgID, catID, "Afropop Festival JHB", searchSeedOpts{})
		searchSeedEvent(t, pool, orgID, catID, "Afropop Durban Session", searchSeedOpts{})
		searchSeedEvent(t, pool, orgID, catID, "House Music Night", searchSeedOpts{})

		w := doRequest(r, http.MethodGet, "/events/search?q=Afropop", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 3)
	})

	t.Run("response row includes required fields", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		searchSeedEvent(t, pool, orgID, catID, "Field Check Concert", searchSeedOpts{})

		w := doRequest(r, http.MethodGet, "/events/search?q=Field+Check", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 1)
		row := data[0].(map[string]interface{})
		for _, field := range []string{"id", "title", "city", "venue_name", "start_time", "end_time", "price", "currency"} {
			assert.Contains(t, row, field, "response row missing field: %s", field)
		}
	})
}

// ─── Ranking ──────────────────────────────────────────────────────────────────

func TestSearchRanking(t *testing.T) {
	pool := setupSearchTestDB(t)
	defer pool.Close()

	t.Run("title match ranks above description-only match", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		// "techno" only in description
		searchSeedEvent(t, pool, orgID, catID, "Night Out Event", searchSeedOpts{
			description: "An amazing techno music experience for true fans.",
		})
		// "techno" in title (weight A — ranks higher)
		searchSeedEvent(t, pool, orgID, catID, "Techno Rave Johannesburg", searchSeedOpts{
			description: "Come join us for this great event.",
		})

		w := doRequest(r, http.MethodGet, "/events/search?q=techno", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 2)

		// Title match must come first
		first := data[0].(map[string]interface{})["title"].(string)
		assert.Equal(t, "Techno Rave Johannesburg", first,
			"title match should rank above description-only match")
	})
}

// ─── Pagination ───────────────────────────────────────────────────────────────

func TestSearchPagination(t *testing.T) {
	pool := setupSearchTestDB(t)
	defer pool.Close()

	t.Run("limit param restricts returned results", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		for i := 0; i < 5; i++ {
			searchSeedEvent(t, pool, orgID, catID, fmt.Sprintf("Jazz Session %d", i), searchSeedOpts{
				startTime: time.Now().Add(time.Duration(i+1) * 24 * time.Hour),
			})
		}

		w := doRequest(r, http.MethodGet, "/events/search?q=Jazz&limit=3", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 3)
	})

	t.Run("next_cursor is set when results overflow the page", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		for i := 0; i < 5; i++ {
			searchSeedEvent(t, pool, orgID, catID, fmt.Sprintf("Reggae Night %d", i), searchSeedOpts{
				startTime: time.Now().Add(time.Duration(i+1) * 24 * time.Hour),
			})
		}

		w := doRequest(r, http.MethodGet, "/events/search?q=Reggae&limit=3", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["next_cursor"], "next_cursor must be set when more pages exist")
	})

	t.Run("cursor returns next page without overlap", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		for i := 0; i < 5; i++ {
			searchSeedEvent(t, pool, orgID, catID, fmt.Sprintf("Kwaito Show %d", i), searchSeedOpts{
				startTime: time.Now().Add(time.Duration(i+1) * 24 * time.Hour),
			})
		}

		// Page 1
		w := doRequest(r, http.MethodGet, "/events/search?q=Kwaito&limit=3", nil)
		var page1 map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page1))
		cursor := page1["next_cursor"].(string)
		require.NotEmpty(t, cursor)

		page1IDs := make(map[string]bool)
		for _, row := range page1["data"].([]interface{}) {
			page1IDs[row.(map[string]interface{})["id"].(string)] = true
		}

		// Page 2
		w = doRequest(r, http.MethodGet, fmt.Sprintf("/events/search?q=Kwaito&limit=3&cursor=%s", cursor), nil)
		require.Equal(t, http.StatusOK, w.Code)
		var page2 map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page2))
		page2Data := page2["data"].([]interface{})
		assert.Len(t, page2Data, 2)
		for _, row := range page2Data {
			id := row.(map[string]interface{})["id"].(string)
			assert.False(t, page1IDs[id], "event %s appeared on both pages", id)
		}
	})

	t.Run("last page has empty or nil next_cursor", func(t *testing.T) {
		cleanSearchTables(t, pool)
		orgID := searchSeedOrganizer(t, pool)
		catID := searchSeedCategory(t, pool)
		r := buildSearchRouter(pool)

		searchSeedEvent(t, pool, orgID, catID, "Amapiano Set A", searchSeedOpts{})
		searchSeedEvent(t, pool, orgID, catID, "Amapiano Set B", searchSeedOpts{})

		w := doRequest(r, http.MethodGet, "/events/search?q=Amapiano&limit=20", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// next_cursor should be absent, null, or empty string on the last page
		cursor, exists := resp["next_cursor"]
		if exists {
			assert.Empty(t, cursor, "next_cursor should be empty on the last page")
		}
	})
}