package handlers_test

// =============================================================================
// FOLLOW SYSTEM TESTS
// =============================================================================
//
// Tests covered:
//   - POST /events/:id/follow        → ToggleFollowHandler
//       - follow an event (following becomes true)
//       - unfollow an event (following becomes false, idempotent toggle)
//       - follow count increments and decrements correctly
//       - unauthenticated request gets 401
//       - invalid event ID still toggles (no UUID validation in handler)
//
//   - GET  /events/:id/follow        → GetFollowStatusHandler
//       - returns following=false for a user who has not followed
//       - returns following=true after following
//       - returns correct follow_count
//       - unauthenticated request gets 401
//
//   - GET  /user/following           → GetFollowedEventsHandler
//       - returns empty list when user follows nothing
//       - returns followed events in descending followed_at order
//       - does not return events followed by other users
//       - respects limit query parameter
//       - returns a next_cursor when results overflow the page
//       - cursor-based pagination returns the next page correctly
//       - unauthenticated request gets 401
//
// Prerequisites (run once before the suite):
//
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser \
//     -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db \
//     -p 5433:5432 postgres:15
//
//   Apply all migrations to test_db first, OR let the helper below create
//   the minimal schema it needs.
//
// Run tests:
//
//   go test ./internal/handlers/... -v -run TestFollow -count=1
// =============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"events-db/internal/config"
	"events-db/internal/handlers"
	"events-db/internal/middleware"
	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// ─── DB setup ─────────────────────────────────────────────────────────────────

func setupFollowTestDB(t *testing.T) *pgxpool.Pool {
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
		t.Fatalf("follow DB tests skipped — DB not reachable: %v", pingErr)
	}
	createFollowTestTables(t, pool)
	cleanFollowTables(t, pool)
	return pool
}

func createFollowTestTables(t *testing.T, pool *pgxpool.Pool) {
    t.Helper()
    ctx := context.Background()

    // Drop in dependency order so stale schemas from other test suites
    // don't bleed in. The slug column was added in migration 000020 — if the
    // events table was created by an earlier test without it, IF NOT EXISTS
    // silently keeps the old schema and every seedEvent call fails.
    drops := []string{
        "DROP TABLE IF EXISTS event_follows  CASCADE",
        "DROP TABLE IF EXISTS events         CASCADE",
        "DROP TABLE IF EXISTS categories     CASCADE",
        "DROP TABLE IF EXISTS refresh_tokens CASCADE",
        "DROP TABLE IF EXISTS users          CASCADE",
        "DROP TYPE  IF EXISTS event_status",
    }
    for _, s := range drops {
        pool.Exec(ctx, s)
    }
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`,

		`CREATE TABLE IF NOT EXISTS users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email      VARCHAR(255) UNIQUE NOT NULL,
			password   TEXT NOT NULL,
			role       VARCHAR(50) NOT NULL DEFAULT 'user',
			first_name TEXT,
			last_name  TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT UNIQUE NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			revoked    BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS categories (
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(100) UNIQUE NOT NULL,
    slug       TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
)`,

		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'event_status') THEN
				CREATE TYPE event_status AS ENUM ('draft', 'published', 'cancelled', 'completed');
			END IF;
		END $$`,

		`CREATE TABLE IF NOT EXISTS events (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    slug             TEXT UNIQUE,
    title            VARCHAR(255) NOT NULL,
    description      TEXT NOT NULL,
    cover_image_url  TEXT,
    banner_image_url TEXT,
    start_time       TIMESTAMPTZ NOT NULL,
    end_time         TIMESTAMPTZ NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    timezone         VARCHAR(50) DEFAULT 'SAST' NOT NULL,
    venue_name       VARCHAR(255) NOT NULL,
    address          TEXT NOT NULL,
    city             VARCHAR(255) NOT NULL,
    latitude         DECIMAL(10,8),
    longitude        DECIMAL(11,8),
    category_id      INTEGER NOT NULL REFERENCES categories(id),
    tags             TEXT[],
    price            DECIMAL(10,2) DEFAULT 0.00 NOT NULL,
    currency         VARCHAR(3) DEFAULT 'ZAR' NOT NULL,
    max_capacity     INTEGER,
    tickets_sold     INTEGER DEFAULT 0 NOT NULL,
    reserved_count   INTEGER DEFAULT 0 NOT NULL,
    allow_waitlist   BOOLEAN DEFAULT TRUE,
    status           event_status DEFAULT 'draft' NOT NULL,
    is_featured      BOOLEAN DEFAULT FALSE,
    featured_until   TIMESTAMPTZ,
    published_at     TIMESTAMPTZ,
    cancelled_at     TIMESTAMPTZ,
    cancellation_reason TEXT,
    refund_policy    TEXT,
    terms_and_conditions TEXT,
    organizer_id     UUID NOT NULL REFERENCES users(id),
    views            BIGINT DEFAULT 0,
    search_vector    tsvector,
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    updated_at       TIMESTAMPTZ DEFAULT NOW()
)`,

		`CREATE TABLE IF NOT EXISTS event_follows (
			id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id         UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
			event_id        UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			reminded_7_days BOOLEAN NOT NULL DEFAULT FALSE,
			reminded_1_day  BOOLEAN NOT NULL DEFAULT FALSE,
			followed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT uq_event_follows UNIQUE (user_id, event_id)
		)`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Logf("table setup warning: %v", err)
		}
	}
}

func cleanFollowTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE event_follows CASCADE")
	pool.Exec(ctx, "TRUNCATE events CASCADE")
	pool.Exec(ctx, "TRUNCATE categories RESTART IDENTITY CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens CASCADE")
	pool.Exec(ctx, "TRUNCATE users CASCADE")
}

// ─── Router ───────────────────────────────────────────────────────────────────

func buildFollowRouter(pool *pgxpool.Pool, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Follow toggle + status — auth required
	events := r.Group("/events")
	events.Use(middleware.AuthMiddleware(cfg))
	{
		events.POST("/:id/follow", handlers.ToggleFollowHandler(pool))
		events.GET("/:id/follow", handlers.GetFollowStatusHandler(pool))
	}

	// Followed events feed — auth required
	user := r.Group("/user")
	user.Use(middleware.AuthMiddleware(cfg))
	{
		user.GET("/following", handlers.GetFollowedEventsHandler(pool))
	}

	// Auth routes — needed for loginAndGetTokens helper
	auth := r.Group("/auth")
	auth.POST("/login", handlers.LoginHandler(pool, cfg))
	auth.POST("/refresh", handlers.RefreshTokenHandler(pool, cfg))

	return r
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// createFollowUser registers a fresh user directly via the repository and
// returns their ID plus a valid access token.
func createFollowUser(t *testing.T, pool *pgxpool.Pool, cfg *config.Config) (userID, token string) {
	t.Helper()
	email := randomEmail()
	password := "Password1234!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	created, err := repository.CreateUser(pool, &models.User{
		Email:    email,
		Password: string(hashed),
		Role:     "user",
	})
	require.NoError(t, err)
	userID = created.ID

	r := buildFollowRouter(pool, cfg)
	w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
		"email": email, "password": password,
	})
	require.Equal(t, http.StatusOK, w.Code, "login failed: %s", w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token = resp["access_token"].(string)
	return
}

// seedCategory inserts a category and returns its ID.
func seedCategory(t *testing.T, pool *pgxpool.Pool, name string) int {
    t.Helper()
    var id int
    slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))

    err := pool.QueryRow(context.Background(),
        `INSERT INTO categories (name, slug)
         VALUES ($1, $2)
         ON CONFLICT (name)
         DO UPDATE SET name = EXCLUDED.name
         RETURNING id`,
        name, slug,
    ).Scan(&id)

    require.NoError(t, err)
    return id
}

// seedEvent inserts a published event owned by organizerID and returns its UUID.
func seedEvent(t *testing.T, pool *pgxpool.Pool, organizerID string, categoryID int, title string) string {
    t.Helper()
    now := time.Now()

    slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))

    var id string
    err := pool.QueryRow(context.Background(), `
        INSERT INTO events (
            slug, title, description,
            start_time, end_time, expires_at,
            venue_name, address, city, category_id,
            price, currency, status, organizer_id
        ) VALUES (
            $1, $2, 'Test description',
            $3, $4, $5,
            'Test Venue', '1 Test St', 'Cape Town', $6,
            0.00, 'ZAR', 'published', $7
        ) RETURNING id`,
        slug,
        title,
        now.Add(24*time.Hour),
        now.Add(48*time.Hour),
        now.Add(72*time.Hour),
        categoryID,
        organizerID,
    ).Scan(&id)

    require.NoError(t, err)
    return id
}

// parseFollowToggleResponse unmarshals a ToggleFollowHandler response body.
func parseFollowToggleResponse(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}

// ─── POST /events/:id/follow — toggle ─────────────────────────────────────────

func TestFollowToggle(t *testing.T) {
	pool := setupFollowTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()

	t.Run("follow an event — following becomes true, follow_count becomes 1", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		followerID, token := createFollowUser(t, pool, cfg)
		_ = followerID
		catID := seedCategory(t, pool, "Music")
		eventID := seedEvent(t, pool, organizerID, catID, "Live Concert")

		w := doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

		resp := parseFollowToggleResponse(t, w.Body.Bytes())
		assert.Equal(t, true, resp["following"])
		assert.EqualValues(t, 1, resp["follow_count"])
		assert.Equal(t, eventID, resp["event_id"])
	})

	t.Run("unfollow — second call toggles back, following becomes false", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")
		eventID := seedEvent(t, pool, organizerID, catID, "Live Concert")

		// First call — follow
		w := doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
		require.Equal(t, http.StatusOK, w.Code)

		// Second call — unfollow
		w = doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

		resp := parseFollowToggleResponse(t, w.Body.Bytes())
		assert.Equal(t, false, resp["following"])
		assert.EqualValues(t, 0, resp["follow_count"])
	})

	t.Run("follow count reflects multiple distinct followers", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token1 := createFollowUser(t, pool, cfg)
		_, token2 := createFollowUser(t, pool, cfg)
		_, token3 := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")
		eventID := seedEvent(t, pool, organizerID, catID, "Festival")

		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token1)
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token2)
		w := doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token3)

		resp := parseFollowToggleResponse(t, w.Body.Bytes())
		assert.EqualValues(t, 3, resp["follow_count"])
	})

	t.Run("follow count decrements correctly after unfollow", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token1 := createFollowUser(t, pool, cfg)
		_, token2 := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")
		eventID := seedEvent(t, pool, organizerID, catID, "Festival")

		// Two followers
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token1)
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token2)

		// token1 unfollows
		w := doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token1)
		resp := parseFollowToggleResponse(t, w.Body.Bytes())
		assert.EqualValues(t, 1, resp["follow_count"])
		assert.Equal(t, false, resp["following"])
	})

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		w := doRequest(r, http.MethodPost, "/events/some-event-id/follow", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("toggling an event the caller does not own still works — no ownership check on follow", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, followerToken := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")
		eventID := seedEvent(t, pool, organizerID, catID, "Other's Event")

		w := doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, followerToken)
		assert.Equal(t, http.StatusOK, w.Code)

		resp := parseFollowToggleResponse(t, w.Body.Bytes())
		assert.Equal(t, true, resp["following"])
	})
}

// ─── GET /events/:id/follow — status ──────────────────────────────────────────

func TestFollowStatus(t *testing.T) {
	pool := setupFollowTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()

	t.Run("returns following=false before any follow action", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")
		eventID := seedEvent(t, pool, organizerID, catID, "Concert")

		w := doAuthRequest(r, http.MethodGet, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, false, resp["following"])
		assert.EqualValues(t, 0, resp["follow_count"])
		assert.Equal(t, eventID, resp["event_id"])
	})

	t.Run("returns following=true after following the event", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")
		eventID := seedEvent(t, pool, organizerID, catID, "Concert")

		// Follow
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)

		// Check status
		w := doAuthRequest(r, http.MethodGet, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, true, resp["following"])
		assert.EqualValues(t, 1, resp["follow_count"])
	})

	t.Run("follow_count is shared across all users — one user's status does not hide others", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token1 := createFollowUser(t, pool, cfg)
		_, token2 := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")
		eventID := seedEvent(t, pool, organizerID, catID, "Festival")

		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token1)
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token2)

		// token1 checks status — they are following, count is 2
		w := doAuthRequest(r, http.MethodGet, fmt.Sprintf("/events/%s/follow", eventID), nil, token1)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, true, resp["following"])
		assert.EqualValues(t, 2, resp["follow_count"])
	})

	t.Run("returns following=false after unfollowing", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")
		eventID := seedEvent(t, pool, organizerID, catID, "Concert")

		// Follow then unfollow
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)

		w := doAuthRequest(r, http.MethodGet, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, false, resp["following"])
		assert.EqualValues(t, 0, resp["follow_count"])
	})

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		r := buildFollowRouter(pool, cfg)
		w := doRequest(r, http.MethodGet, "/events/some-id/follow", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── GET /user/following — followed events feed ────────────────────────────────

func TestFollowedEventsFeed(t *testing.T) {
	pool := setupFollowTestDB(t)
	defer pool.Close()
	cfg := authTestConfig()

	t.Run("returns empty list when user follows nothing", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		_, token := createFollowUser(t, pool, cfg)

		w := doAuthRequest(r, http.MethodGet, "/user/following", nil, token)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 0)
		assert.Nil(t, resp["next_cursor"])
	})

	t.Run("returns followed events most-recently-followed first", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")

		event1 := seedEvent(t, pool, organizerID, catID, "Alpha Event")
		event2 := seedEvent(t, pool, organizerID, catID, "Beta Event")
		event3 := seedEvent(t, pool, organizerID, catID, "Gamma Event")

		// Follow in order — event3 is the most recently followed
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", event1), nil, token)
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", event2), nil, token)
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", event3), nil, token)

		w := doAuthRequest(r, http.MethodGet, "/user/following", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 3)

		// Most recently followed is first
		first := data[0].(map[string]interface{})
		assert.Equal(t, event3, first["id"])
		last := data[2].(map[string]interface{})
		assert.Equal(t, event1, last["id"])
	})

	t.Run("does not return events followed by other users", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, tokenA := createFollowUser(t, pool, cfg)
		_, tokenB := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")

		eventA := seedEvent(t, pool, organizerID, catID, "User A's event")
		eventB := seedEvent(t, pool, organizerID, catID, "User B's event")

		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventA), nil, tokenA)
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventB), nil, tokenB)

		// User A's feed must only contain eventA
		w := doAuthRequest(r, http.MethodGet, "/user/following", nil, tokenA)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 1)
		row := data[0].(map[string]interface{})
		assert.Equal(t, eventA, row["id"])
	})

	t.Run("respects limit query parameter", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")

		for i := 0; i < 5; i++ {
			eventID := seedEvent(t, pool, organizerID, catID, fmt.Sprintf("Event %d", i))
			doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
		}

		w := doAuthRequest(r, http.MethodGet, "/user/following?limit=3", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 3)
	})

	t.Run("returns next_cursor when more results exist beyond the page", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")

		for i := 0; i < 5; i++ {
			eventID := seedEvent(t, pool, organizerID, catID, fmt.Sprintf("Page Event %d", i))
			doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
		}

		w := doAuthRequest(r, http.MethodGet, "/user/following?limit=3", nil, token)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		assert.NotEmpty(t, resp["next_cursor"], "next_cursor must be present when more pages exist")
	})

	t.Run("next_cursor is nil when all results fit on one page", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")

		for i := 0; i < 2; i++ {
			eventID := seedEvent(t, pool, organizerID, catID, fmt.Sprintf("Small Page Event %d", i))
			doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
		}

		w := doAuthRequest(r, http.MethodGet, "/user/following?limit=20", nil, token)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		assert.Nil(t, resp["next_cursor"])
	})

	t.Run("cursor-based pagination returns next page without overlap", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")

		var allEventIDs []string
		for i := 0; i < 5; i++ {
			eventID := seedEvent(t, pool, organizerID, catID, fmt.Sprintf("Cursor Event %d", i))
			doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)
			allEventIDs = append(allEventIDs, eventID)
		}

		// Page 1
		w := doAuthRequest(r, http.MethodGet, "/user/following?limit=3", nil, token)
		var page1 map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page1))
		page1Data := page1["data"].([]interface{})
		require.Len(t, page1Data, 3)
		cursor := page1["next_cursor"].(string)
		require.NotEmpty(t, cursor)

		// Collect page 1 IDs
		page1IDs := make(map[string]bool)
		for _, row := range page1Data {
			page1IDs[row.(map[string]interface{})["id"].(string)] = true
		}

		// Page 2
		w = doAuthRequest(r, http.MethodGet, fmt.Sprintf("/user/following?limit=3&cursor=%s", cursor), nil, token)
		assert.Equal(t, http.StatusOK, w.Code)

		var page2 map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page2))
		page2Data := page2["data"].([]interface{})
		require.Len(t, page2Data, 2, "page 2 should contain the remaining 2 events")

		// No overlap between pages
		for _, row := range page2Data {
			id := row.(map[string]interface{})["id"].(string)
			assert.False(t, page1IDs[id], "event %s appeared on both pages", id)
		}
	})

	t.Run("response includes expected event card fields", func(t *testing.T) {
		cleanFollowTables(t, pool)
		r := buildFollowRouter(pool, cfg)
		organizerID, _ := createFollowUser(t, pool, cfg)
		_, token := createFollowUser(t, pool, cfg)
		catID := seedCategory(t, pool, "Music")
		eventID := seedEvent(t, pool, organizerID, catID, "Field Check Event")
		doAuthRequest(r, http.MethodPost, fmt.Sprintf("/events/%s/follow", eventID), nil, token)

		w := doAuthRequest(r, http.MethodGet, "/user/following", nil, token)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 1)

		row := data[0].(map[string]interface{})
		assert.Equal(t, eventID, row["id"])
		assert.NotEmpty(t, row["title"])
		assert.NotEmpty(t, row["venue_name"])
		assert.NotEmpty(t, row["city"])
		assert.NotEmpty(t, row["start_time"])
		assert.NotEmpty(t, row["currency"])
		assert.NotEmpty(t, row["status"])
		assert.NotEmpty(t, row["category"])
		assert.NotEmpty(t, row["follow_id"])
		assert.NotEmpty(t, row["followed_at"])
		// Computed fields
		assert.Contains(t, row, "is_sold_out")
		assert.Contains(t, row, "available_tickets")
	})

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		r := buildFollowRouter(pool, cfg)
		w := doRequest(r, http.MethodGet, "/user/following", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}