package handlers_test

// =============================================================================
// ORGANIZER DASHBOARD TESTS
// =============================================================================
//
// Tests covered:
//
//   GET /organiser/events → GetOrganizerEventsHandler
//     - returns only events owned by the authenticated organizer
//     - returns events for all statuses when status=all (or omitted)
//     - status=published filters to published events only
//     - status=draft filters to draft events only
//     - status=cancelled filters to cancelled events only
//     - two organizers' events do not cross-contaminate
//     - unauthenticated request returns 401
//     - regular user (attendee) is forbidden — returns 403
//     - empty event list returns 200 with empty array
//     - each row includes id, slug, title, status, tickets_sold, price, created_at
//
//   GET /admin/stats → GetAdminStatsHandler
//     - returns 200 with all expected stat fields (admin only)
//     - regular user is forbidden — 403
//     - unauthenticated returns 401
//     - total_revenue equals ticket_revenue + merch_revenue
//
//   GET /admin/events → GetAdminEventsHandler
//     - returns all events across all organizers
//     - status= filter returns only matching status
//     - search= filter matches event title case-insensitively
//     - limit and cursor pagination work correctly
//     - unauthenticated returns 401
//     - regular user is forbidden — 403
//     - each row includes id, slug, title, status, city, tickets_sold, total_revenue
//
//   GET /admin/merch/oversold → GetOversoldMerchOrdersHandler
//     - returns 200 with empty list when no oversold orders exist
//     - unauthenticated returns 401
//     - regular user is forbidden — 403
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
//   go test ./internal/handlers/... -v -run TestOrgDashboard -count=1
//   go test ./internal/handlers/... -v -run TestAdminDashboard -count=1
// =============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

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

func setupOrgDashTestDB(t *testing.T) *pgxpool.Pool {
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
		t.Fatalf("org dashboard DB not reachable: %v", pingErr)
	}
	createOrgDashTestTables(t, pool)
	cleanOrgDashTables(t, pool)
	return pool
}

func createOrgDashTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
		"DROP TABLE IF EXISTS merch_orders        CASCADE",
		"DROP TABLE IF EXISTS merchandise_variants CASCADE",
		"DROP TABLE IF EXISTS merchandise         CASCADE",
		"DROP TABLE IF EXISTS bookings            CASCADE",
		"DROP TABLE IF EXISTS events              CASCADE",
		"DROP TABLE IF EXISTS categories          CASCADE",
		"DROP TABLE IF EXISTS refresh_tokens      CASCADE",
		"DROP TABLE IF EXISTS users               CASCADE",
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
			first_name TEXT,
			last_name  TEXT,
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
			venue_name       VARCHAR(255) NOT NULL DEFAULT 'Test Venue',
			address          TEXT NOT NULL DEFAULT '1 Test Rd',
			city             VARCHAR(255) NOT NULL DEFAULT 'Johannesburg',
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

		// Bookings stub — used by GetAdminStats ticket revenue query
		`CREATE TABLE bookings (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id       UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			booking_status VARCHAR(50) NOT NULL DEFAULT 'pending',
			total_amount   DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			currency       VARCHAR(3) NOT NULL DEFAULT 'ZAR',
			created_at     TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Merch orders stub — used by GetAdminStats merch revenue query
		`CREATE TABLE merchandise (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id   UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
)`,

`CREATE TABLE merchandise_variants (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchandise_id UUID NOT NULL REFERENCES merchandise(id) ON DELETE CASCADE,
    sku            TEXT NOT NULL DEFAULT '',
    stock_quantity INTEGER NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    updated_at     TIMESTAMPTZ DEFAULT NOW()
)`,

`CREATE TABLE merch_orders (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id                UUID,
    merchandise_id         UUID REFERENCES merchandise(id),
    variant_id             UUID REFERENCES merchandise_variants(id),
    event_id               UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id                UUID,
    session_id             TEXT,
    quantity               INTEGER NOT NULL DEFAULT 1,
    unit_price             DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    total_amount           DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    currency               VARCHAR(3) NOT NULL DEFAULT 'ZAR',
    stitch_payment_req_id  TEXT,
    fulfillment_status     VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at             TIMESTAMPTZ DEFAULT NOW(),
    updated_at             TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (stitch_payment_req_id, variant_id)
)`,
		
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("org dash table setup failed:\n  sql: %s\n  err: %v", s, err)
		}
	}
}

func cleanOrgDashTables(t *testing.T, pool *pgxpool.Pool) {
    t.Helper()
    ctx := context.Background()
    pool.Exec(ctx, "TRUNCATE merch_orders         CASCADE")
    pool.Exec(ctx, "TRUNCATE merchandise_variants  CASCADE")
    pool.Exec(ctx, "TRUNCATE merchandise           CASCADE")
    pool.Exec(ctx, "TRUNCATE bookings             CASCADE")
    pool.Exec(ctx, "TRUNCATE events               CASCADE")
    pool.Exec(ctx, "TRUNCATE categories           CASCADE")
    pool.Exec(ctx, "TRUNCATE refresh_tokens       CASCADE")
    pool.Exec(ctx, "TRUNCATE users                CASCADE")
}

// ─── Router ───────────────────────────────────────────────────────────────────

func buildOrgDashRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()
	r := gin.New()

	// Auth — needed for token acquisition
	r.POST("/auth/login", handlers.LoginHandler(pool, cfg))

	// Organizer routes
	org := r.Group("/organiser")
	org.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	{
		org.GET("/events", handlers.GetOrganizerEventsHandler(pool))
	}

	// Admin routes
	admin := r.Group("/admin")
	admin.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
	{
		admin.GET("/stats", handlers.GetAdminStatsHandler(pool))
		admin.GET("/events", handlers.GetAdminEventsHandler(pool))
		admin.GET("/merch/oversold", handlers.GetOversoldMerchOrdersHandler(pool))
	}

	return r
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// orgDashSeedUser creates a user with the given role, logs them in via the
// router and returns their ID plus a valid access token.
func orgDashSeedUser(t *testing.T, pool *pgxpool.Pool, role string) (userID, token string) {
	t.Helper()
	cfg := authTestConfig()
	email := randomEmail()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("Pass1234!"), bcrypt.DefaultCost)
	created, err := repository.CreateUser(pool, &models.User{
		Email:    email,
		Password: string(hashed),
		Role:     models.UserRole(role),
	})
	require.NoError(t, err)
	userID = created.ID

	r := buildAuthRouter(pool, cfg)
	w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
		"email": email, "password": "Pass1234!",
	})
	require.Equal(t, http.StatusOK, w.Code, "login failed: %s", w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token = resp["access_token"].(string)
	return
}

func orgDashSeedCategory(t *testing.T, pool *pgxpool.Pool) int {
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

type orgDashEventOpts struct {
	status    string  // default "draft"
	city      string  // default "Johannesburg"
	price     float64 // default 0
	startDays int     // days from now, default 7
}

func orgDashSeedEvent(t *testing.T, pool *pgxpool.Pool, orgID string, catID int, title string, opts orgDashEventOpts) string {
	t.Helper()
	if opts.status == "" {
		opts.status = "draft"
	}
	if opts.city == "" {
		opts.city = "Johannesburg"
	}
	if opts.startDays == 0 {
		opts.startDays = 7
	}
	start := time.Now().Add(time.Duration(opts.startDays) * 24 * time.Hour)
	end := start.Add(3 * time.Hour)
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, description, status, category_id, organizer_id,
		                    price, currency, venue_name, address, city,
		                    start_time, end_time, expires_at)
		VALUES ($1, 'Test description with enough text.', $2, $3, $4,
		        $5, 'ZAR', 'FNB Stadium', 'Nasrec Rd', $6,
		        $7, $8, NOW() + INTERVAL '60 days')
		RETURNING id
	`, title, opts.status, catID, orgID, opts.price, opts.city, start, end).Scan(&id)
	require.NoError(t, err)
	return id
}

func orgDashSeedConfirmedBooking(t *testing.T, pool *pgxpool.Pool, eventID string, amount float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO bookings (event_id, booking_status, total_amount)
		VALUES ($1, 'confirmed', $2)
	`, eventID, amount)
	require.NoError(t, err)
}

func orgDashSeedMerchOrder(t *testing.T, pool *pgxpool.Pool, eventID string, amount float64, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO merch_orders (event_id, quantity, total_amount, fulfillment_status)
		VALUES ($1, 1, $2, $3)
	`, eventID, amount, status)
	require.NoError(t, err)
}

// ─── GET /organiser/events ────────────────────────────────────────────────────

func TestOrgDashboardEvents(t *testing.T) {
	pool := setupOrgDashTestDB(t)
	defer pool.Close()

	t.Run("returns only events owned by the authenticated organizer", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID1, token1 := orgDashSeedUser(t, pool, "organizer")
		orgID2, _ := orgDashSeedUser(t, pool, "organizer")
		catID := orgDashSeedCategory(t, pool)

		orgDashSeedEvent(t, pool, orgID1, catID, "Org1 Event A", orgDashEventOpts{status: "published"})
		orgDashSeedEvent(t, pool, orgID1, catID, "Org1 Event B", orgDashEventOpts{status: "draft"})
		orgDashSeedEvent(t, pool, orgID2, catID, "Org2 Event", orgDashEventOpts{status: "published"})

		w := doAuthRequest(r, http.MethodGet, "/organiser/events", nil, token1)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		events := resp["events"].([]interface{})
		assert.Len(t, events, 2, "should only see own events")
		for _, ev := range events {
			title := ev.(map[string]interface{})["title"].(string)
			assert.Contains(t, title, "Org1", "should only contain organizer 1's events")
		}
	})

	t.Run("status filter — published returns only published events", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID, token := orgDashSeedUser(t, pool, "organizer")
		catID := orgDashSeedCategory(t, pool)

		orgDashSeedEvent(t, pool, orgID, catID, "Draft Event", orgDashEventOpts{status: "draft"})
		orgDashSeedEvent(t, pool, orgID, catID, "Published Event", orgDashEventOpts{status: "published"})
		orgDashSeedEvent(t, pool, orgID, catID, "Cancelled Event", orgDashEventOpts{status: "cancelled"})

		w := doAuthRequest(r, http.MethodGet, "/organiser/events?status=published", nil, token)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		events := resp["events"].([]interface{})
		assert.Len(t, events, 1)
		assert.Equal(t, "published", events[0].(map[string]interface{})["status"])
	})

	t.Run("status filter — draft returns only draft events", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID, token := orgDashSeedUser(t, pool, "organizer")
		catID := orgDashSeedCategory(t, pool)

		orgDashSeedEvent(t, pool, orgID, catID, "Draft One", orgDashEventOpts{status: "draft"})
		orgDashSeedEvent(t, pool, orgID, catID, "Draft Two", orgDashEventOpts{status: "draft"})
		orgDashSeedEvent(t, pool, orgID, catID, "Published", orgDashEventOpts{status: "published"})

		w := doAuthRequest(r, http.MethodGet, "/organiser/events?status=draft", nil, token)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		events := resp["events"].([]interface{})
		assert.Len(t, events, 2)
	})

	t.Run("status=all (or omitted) returns events of every status", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID, token := orgDashSeedUser(t, pool, "organizer")
		catID := orgDashSeedCategory(t, pool)

		orgDashSeedEvent(t, pool, orgID, catID, "Draft", orgDashEventOpts{status: "draft"})
		orgDashSeedEvent(t, pool, orgID, catID, "Published", orgDashEventOpts{status: "published"})
		orgDashSeedEvent(t, pool, orgID, catID, "Cancelled", orgDashEventOpts{status: "cancelled"})

		w := doAuthRequest(r, http.MethodGet, "/organiser/events?status=all", nil, token)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		events := resp["events"].([]interface{})
		assert.Len(t, events, 3)
	})

	t.Run("empty event list returns 200 with empty events array", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		_, token := orgDashSeedUser(t, pool, "organizer")

		w := doAuthRequest(r, http.MethodGet, "/organiser/events", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		events := resp["events"].([]interface{})
		assert.Len(t, events, 0)
	})

	t.Run("response row includes required fields", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID, token := orgDashSeedUser(t, pool, "organizer")
		catID := orgDashSeedCategory(t, pool)
		orgDashSeedEvent(t, pool, orgID, catID, "Field Test Event", orgDashEventOpts{status: "published"})

		w := doAuthRequest(r, http.MethodGet, "/organiser/events", nil, token)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		events := resp["events"].([]interface{})
		require.Len(t, events, 1)
		row := events[0].(map[string]interface{})
		for _, field := range []string{"id", "title", "status", "tickets_sold", "price", "created_at"} {
			assert.Contains(t, row, field, "missing field: %s", field)
		}
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildOrgDashRouter(pool)
		w := doRequest(r, http.MethodGet, "/organiser/events", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("attendee role is forbidden — 403", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		_, userToken := orgDashSeedUser(t, pool, "attendee")

		w := doAuthRequest(r, http.MethodGet, "/organiser/events", nil, userToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

// ─── GET /admin/stats ─────────────────────────────────────────────────────────

func TestAdminDashboardStats(t *testing.T) {
	pool := setupOrgDashTestDB(t)
	defer pool.Close()

	t.Run("returns all expected stat fields", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		_, adminToken := orgDashSeedUser(t, pool, "admin")

		w := doAuthRequest(r, http.MethodGet, "/admin/stats", nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var stats map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))

		expectedFields := []string{
			"total_events", "active_events", "total_users", "total_bookings",
			"confirmed_bookings", "ticket_revenue", "tickets_today",
			"ticket_revenue_today", "merch_units_sold", "merch_revenue",
			"merch_orders_today", "merch_revenue_today", "oversold_merch_orders",
			"total_revenue", "total_revenue_today",
		}
		for _, field := range expectedFields {
			assert.Contains(t, stats, field, "admin/stats missing field: %s", field)
		}
	})

	t.Run("total_revenue equals ticket_revenue + merch_revenue", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID, _ := orgDashSeedUser(t, pool, "organizer")
		_, adminToken := orgDashSeedUser(t, pool, "admin")
		catID := orgDashSeedCategory(t, pool)

		eventID := orgDashSeedEvent(t, pool, orgID, catID, "Revenue Test Event",
			orgDashEventOpts{status: "published"})
		orgDashSeedConfirmedBooking(t, pool, eventID, 500.00)
		orgDashSeedMerchOrder(t, pool, eventID, 150.00, "fulfilled")

		w := doAuthRequest(r, http.MethodGet, "/admin/stats", nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code)
		var stats map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))

		ticketRev := stats["ticket_revenue"].(float64)
		merchRev := stats["merch_revenue"].(float64)
		totalRev := stats["total_revenue"].(float64)

		assert.InDelta(t, 500.00, ticketRev, 0.01)
		assert.InDelta(t, 150.00, merchRev, 0.01)
		assert.InDelta(t, ticketRev+merchRev, totalRev, 0.01,
			"total_revenue must equal ticket + merch revenue")
	})

	t.Run("total_events count increases when events are added", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID, _ := orgDashSeedUser(t, pool, "organizer")
		_, adminToken := orgDashSeedUser(t, pool, "admin")
		catID := orgDashSeedCategory(t, pool)

		// Empty baseline
		w := doAuthRequest(r, http.MethodGet, "/admin/stats", nil, adminToken)
		var before map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &before))
		beforeCount := int(before["total_events"].(float64))

		orgDashSeedEvent(t, pool, orgID, catID, "Added Event", orgDashEventOpts{status: "published"})

		w = doAuthRequest(r, http.MethodGet, "/admin/stats", nil, adminToken)
		var after map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &after))
		assert.Equal(t, beforeCount+1, int(after["total_events"].(float64)))
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildOrgDashRouter(pool)
		w := doRequest(r, http.MethodGet, "/admin/stats", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("organizer role is forbidden — 403", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		_, orgToken := orgDashSeedUser(t, pool, "organizer")

		w := doAuthRequest(r, http.MethodGet, "/admin/stats", nil, orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

// ─── GET /admin/events ────────────────────────────────────────────────────────

func TestAdminDashboardEvents(t *testing.T) {
	pool := setupOrgDashTestDB(t)
	defer pool.Close()

	t.Run("returns events across all organizers", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		org1, _ := orgDashSeedUser(t, pool, "organizer")
		org2, _ := orgDashSeedUser(t, pool, "organizer")
		_, adminToken := orgDashSeedUser(t, pool, "admin")
		catID := orgDashSeedCategory(t, pool)

		orgDashSeedEvent(t, pool, org1, catID, "Org1 Event", orgDashEventOpts{status: "published"})
		orgDashSeedEvent(t, pool, org2, catID, "Org2 Event", orgDashEventOpts{status: "published"})

		w := doAuthRequest(r, http.MethodGet, "/admin/events", nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 2)
	})

	t.Run("status= filter returns only matching status", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID, _ := orgDashSeedUser(t, pool, "organizer")
		_, adminToken := orgDashSeedUser(t, pool, "admin")
		catID := orgDashSeedCategory(t, pool)

		orgDashSeedEvent(t, pool, orgID, catID, "Draft A", orgDashEventOpts{status: "draft"})
		orgDashSeedEvent(t, pool, orgID, catID, "Draft B", orgDashEventOpts{status: "draft"})
		orgDashSeedEvent(t, pool, orgID, catID, "Published", orgDashEventOpts{status: "published"})

		w := doAuthRequest(r, http.MethodGet, "/admin/events?status=draft", nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 2)
		for _, ev := range data {
			assert.Equal(t, "draft", ev.(map[string]interface{})["status"])
		}
	})

	t.Run("search= filter matches event title case-insensitively", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID, _ := orgDashSeedUser(t, pool, "organizer")
		_, adminToken := orgDashSeedUser(t, pool, "admin")
		catID := orgDashSeedCategory(t, pool)

		orgDashSeedEvent(t, pool, orgID, catID, "Kalahari Night Market", orgDashEventOpts{status: "published"})
		orgDashSeedEvent(t, pool, orgID, catID, "Cape Town Jazz Fest", orgDashEventOpts{status: "published"})

		w := doAuthRequest(r, http.MethodGet, "/admin/events?search=kalahari", nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)
		assert.Equal(t, "Kalahari Night Market", data[0].(map[string]interface{})["title"])
	})

	t.Run("response row includes required fields", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID, _ := orgDashSeedUser(t, pool, "organizer")
		_, adminToken := orgDashSeedUser(t, pool, "admin")
		catID := orgDashSeedCategory(t, pool)
		orgDashSeedEvent(t, pool, orgID, catID, "Admin Field Test", orgDashEventOpts{status: "published"})

		w := doAuthRequest(r, http.MethodGet, "/admin/events", nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 1)
		row := data[0].(map[string]interface{})
		for _, field := range []string{"id", "title", "status", "city", "tickets_sold", "total_revenue", "created_at"} {
			assert.Contains(t, row, field, "admin/events row missing field: %s", field)
		}
	})

	t.Run("limit and cursor pagination return correct pages without overlap", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		orgID, _ := orgDashSeedUser(t, pool, "organizer")
		_, adminToken := orgDashSeedUser(t, pool, "admin")
		catID := orgDashSeedCategory(t, pool)

		for i := 0; i < 5; i++ {
			orgDashSeedEvent(t, pool, orgID, catID, fmt.Sprintf("Paged Event %d", i),
				orgDashEventOpts{status: "published", startDays: i + 1})
		}

		// Page 1
		w := doAuthRequest(r, http.MethodGet, "/admin/events?limit=3", nil, adminToken)
		var page1 map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page1))
		data1 := page1["data"].([]interface{})
		assert.Len(t, data1, 3)
		cursor := page1["next_cursor"].(string)
		require.NotEmpty(t, cursor)

		page1IDs := make(map[string]bool)
		for _, ev := range data1 {
			page1IDs[ev.(map[string]interface{})["id"].(string)] = true
		}

		// Page 2
		w = doAuthRequest(r, http.MethodGet, fmt.Sprintf("/admin/events?limit=3&cursor=%s", cursor), nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code)
		var page2 map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page2))
		data2 := page2["data"].([]interface{})
		assert.Len(t, data2, 2)
		for _, ev := range data2 {
			id := ev.(map[string]interface{})["id"].(string)
			assert.False(t, page1IDs[id], "event %s appeared on both pages", id)
		}
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildOrgDashRouter(pool)
		w := doRequest(r, http.MethodGet, "/admin/events", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("organizer role is forbidden — 403", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		_, orgToken := orgDashSeedUser(t, pool, "organizer")
		w := doAuthRequest(r, http.MethodGet, "/admin/events", nil, orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

// ─── GET /admin/merch/oversold ────────────────────────────────────────────────

func TestAdminDashboardOversold(t *testing.T) {
	pool := setupOrgDashTestDB(t)
	defer pool.Close()

	t.Run("returns 200 with empty list when no oversold orders exist", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		_, adminToken := orgDashSeedUser(t, pool, "admin")

		w := doAuthRequest(r, http.MethodGet, "/admin/merch/oversold", nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp, "data")
		assert.Contains(t, resp, "count")
		assert.EqualValues(t, 0, resp["count"])
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildOrgDashRouter(pool)
		w := doRequest(r, http.MethodGet, "/admin/merch/oversold", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("organizer role is forbidden — 403", func(t *testing.T) {
		cleanOrgDashTables(t, pool)
		r := buildOrgDashRouter(pool)
		_, orgToken := orgDashSeedUser(t, pool, "organizer")
		w := doAuthRequest(r, http.MethodGet, "/admin/merch/oversold", nil, orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}