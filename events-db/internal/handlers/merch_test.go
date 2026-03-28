package handlers_test

// =============================================================================
// MERCHANDISE TESTS
// =============================================================================
//
// Tests covered:
//   - GET  /events/:id/merch              → GetEventMerchHandler       (public)
//   - GET  /events/:id/merch/preview      → GetEventMerchPreviewHandler (public)
//   - GET  /merch/:id                     → GetMerchByIDHandler         (public)
//   - POST /organiser/events/:id/merch    → OrgCreateMerchHandler       (organiser)
//   - POST /organiser/merch/:id/variants  → OrgCreateVariantHandler     (organiser)
//   - PUT  /organiser/merch/:id           → OrgUpdateMerchHandler       (organiser)
//   - DELETE /organiser/merch/:id         → OrgDeactivateMerchHandler   (organiser)
//   - PUT  /organiser/merch/variants/:id  → OrgUpdateVariantHandler     (organiser)
//
// Run:
//   go test ./internal/handlers/... -v -run TestMerch
// =============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func setupMerchTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Skipf("merch DB tests skipped — could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("merch DB tests skipped — DB not reachable: %v", pingErr)
	}
	createMerchTestTables(t, pool)
	cleanMerchTables(t, pool)
	return pool
}

func createMerchTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
		"DROP TABLE IF EXISTS merchandise_variants CASCADE",
		"DROP TABLE IF EXISTS merchandise CASCADE",
		"DROP TABLE IF EXISTS events CASCADE",
		"DROP TABLE IF EXISTS categories CASCADE",
		"DROP TABLE IF EXISTS organizers CASCADE",
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
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title          VARCHAR(255) NOT NULL DEFAULT 'Test Event',
			description    TEXT NOT NULL DEFAULT '',
			start_time     TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
			end_time       TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '8 days',
			expires_at     TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
			timezone       VARCHAR(50) NOT NULL DEFAULT 'SAST',
			venue_name     VARCHAR(255) NOT NULL DEFAULT '',
			address        TEXT NOT NULL DEFAULT '',
			city           VARCHAR(255) NOT NULL DEFAULT '',
			price          DECIMAL(10,2) NOT NULL DEFAULT 100.00,
			currency       VARCHAR(3) NOT NULL DEFAULT 'ZAR',
			max_capacity   INTEGER DEFAULT 100,
			tickets_sold   INTEGER NOT NULL DEFAULT 0,
			reserved_count INTEGER NOT NULL DEFAULT 0,
			allow_waitlist BOOLEAN DEFAULT TRUE,
			status         VARCHAR(50) NOT NULL DEFAULT 'published',
			is_featured    BOOLEAN DEFAULT FALSE,
			organizer_id   UUID REFERENCES users(id),
			category_id    INTEGER NOT NULL REFERENCES categories(id),
			views          BIGINT DEFAULT 0,
			created_at     TIMESTAMPTZ DEFAULT NOW(),
			updated_at     TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE merchandise (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id      UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			name          VARCHAR(255) NOT NULL,
			description   TEXT,
			base_price    NUMERIC(10,2) NOT NULL,
			category      VARCHAR(100),
			display_order INTEGER NOT NULL DEFAULT 0,
			is_active     BOOLEAN NOT NULL DEFAULT TRUE,
			images        JSONB,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE TABLE merchandise_variants (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			merchandise_id UUID NOT NULL REFERENCES merchandise(id) ON DELETE CASCADE,
			sku            VARCHAR(100) UNIQUE NOT NULL,
			attributes     JSONB NOT NULL DEFAULT '{}',
			stock_quantity INTEGER NOT NULL DEFAULT 0,
			price_override NUMERIC(10,2),
			is_active      BOOLEAN NOT NULL DEFAULT TRUE,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT stock_non_negative CHECK (stock_quantity >= 0)
		)`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("merch table setup failed: %v\nSQL: %s", err, s)
		}
	}
}

func cleanMerchTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE merchandise_variants CASCADE")
	pool.Exec(ctx, "TRUNCATE merchandise CASCADE")
	pool.Exec(ctx, "TRUNCATE events CASCADE")
	pool.Exec(ctx, "TRUNCATE categories CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens CASCADE")
	pool.Exec(ctx, "TRUNCATE users CASCADE")
}

// ─── seed helpers ─────────────────────────────────────────────────────────────

func merchSeedUserWithRole(t *testing.T, pool *pgxpool.Pool, role string) (userID, token string) {
	t.Helper()
	cfg := authTestConfig()
	email := randomEmail()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("Pass1234!"), bcrypt.DefaultCost)
	u := &models.User{Email: email, Password: string(hashed), Role: models.UserRole(role)}
	created, err := repository.CreateUser(pool, u)
	require.NoError(t, err)
	userID = created.ID

	r := buildAuthRouter(pool, cfg)
	w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
		"email": email, "password": "Pass1234!",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token = resp["access_token"].(string)
	return
}

func merchSeedCategory(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO categories (name, slug)
		VALUES (
			'Test-'||substr(gen_random_uuid()::text,1,8),
			'test-'||substr(gen_random_uuid()::text,1,8)
		)
		RETURNING id
	`).Scan(&id)
	require.NoError(t, err)
	return id
}

func merchSeedEvent(t *testing.T, pool *pgxpool.Pool, organizerID string) string {
	t.Helper()
	catID := merchSeedCategory(t, pool)
	var eventID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, status, price, currency, category_id, organizer_id)
		VALUES ('Chelsea vs Arsenal', 'published', 500.00, 'ZAR', $1, $2)
		RETURNING id
	`, catID, organizerID).Scan(&eventID)
	require.NoError(t, err)
	return eventID
}

func merchSeedItems(t *testing.T, pool *pgxpool.Pool, eventID string, count int) []string {
	t.Helper()
	ids := make([]string, count)
	for i := 0; i < count; i++ {
		var id string
		err := pool.QueryRow(context.Background(), `
			INSERT INTO merchandise (event_id, name, base_price, display_order)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, eventID, fmt.Sprintf("Item %d", i+1), float64((i+1)*100), i).Scan(&id)
		require.NoError(t, err)
		ids[i] = id
	}
	return ids
}

func merchSeedVariant(t *testing.T, pool *pgxpool.Pool, merchID, size string, stock int) string {
	t.Helper()
	sku := fmt.Sprintf("SKU-%s-%d", size, time.Now().UnixNano())
	attrs := fmt.Sprintf(`{"size":"%s"}`, size)
	var variantID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO merchandise_variants (merchandise_id, sku, attributes, stock_quantity)
		VALUES ($1, $2, $3::jsonb, $4)
		RETURNING id
	`, merchID, sku, attrs, stock).Scan(&variantID)
	require.NoError(t, err)
	return variantID
}

// ─── router builder ───────────────────────────────────────────────────────────
// eventCache is nil throughout all DB-level merch tests.
// Cache behaviour is tested separately in internal/cache/event_cache_test.go.

func buildMerchRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()
	r := gin.New()

	auth := r.Group("/auth")
	auth.POST("/login", handlers.LoginHandler(pool, cfg))

	// Public merch routes — pass nil cache (no Redis in DB integration tests)
	events := r.Group("/events")
	{
		events.GET("/:id/merch",         handlers.GetEventMerchHandler(pool, nil))         // ← nil cache
		events.GET("/:id/merch/preview", handlers.GetEventMerchPreviewHandler(pool, nil))  // ← nil cache
	}

	// Single merch item route
	merch := r.Group("/merch")
	{
		merch.GET("/:id", handlers.GetMerchByIDHandler(pool, nil)) // ← new route, nil cache
	}

	// Organiser routes
	org := r.Group("/organiser")
	org.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	{
		org.POST("/events/:id/merch",     handlers.OrgCreateMerchHandler(pool, nil))        // ← nil cache
		org.POST("/merch/:id/variants",   handlers.OrgCreateVariantHandler(pool, nil))      // ← nil cache
		org.PUT("/merch/:id",             handlers.OrgUpdateMerchHandler(pool, nil))        // ← new route
		org.DELETE("/merch/:id",          handlers.OrgDeactivateMerchHandler(pool, nil))    // ← new route
		org.PUT("/merch/variants/:id",    handlers.OrgUpdateVariantHandler(pool, nil))      // ← new route
	}

	return r
}

// ─── GET /events/:id/merch ────────────────────────────────────────────────────

func TestMerchGetCatalogue(t *testing.T) {
	pool := setupMerchTestDB(t)
	defer pool.Close()

	t.Run("returns all active merch items with variants nested", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 3)

		merchSeedVariant(t, pool, ids[0], "S", 5)
		merchSeedVariant(t, pool, ids[0], "L", 10)

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 3)
		assert.EqualValues(t, 3, resp["count"])

		firstItem := data[0].(map[string]interface{})
		variants := firstItem["variants"].([]interface{})
		assert.Len(t, variants, 2, "variants should be nested in each merch item")
	})

	t.Run("items are ordered by display_order ascending", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		pool.Exec(context.Background(), `
			INSERT INTO merchandise (event_id, name, base_price, display_order)
			VALUES ($1, 'C', 100, 3), ($1, 'A', 100, 1), ($1, 'B', 100, 2)
		`, eventID)

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 3)
		assert.Equal(t, "A", data[0].(map[string]interface{})["name"])
		assert.Equal(t, "B", data[1].(map[string]interface{})["name"])
		assert.Equal(t, "C", data[2].(map[string]interface{})["name"])
	})

	t.Run("inactive items are excluded", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		pool.Exec(context.Background(), `
			INSERT INTO merchandise (event_id, name, base_price, is_active)
			VALUES ($1, 'Active Item', 100, TRUE),
			       ($1, 'Hidden Item', 100, FALSE)
		`, eventID)

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)
		assert.Equal(t, "Active Item", data[0].(map[string]interface{})["name"])
	})

	t.Run("event with no merch returns empty data array", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		r := buildMerchRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.EqualValues(t, 0, resp["count"])
	})

	t.Run("no auth required — public endpoint", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		r := buildMerchRouter(pool)

		req := httptest.NewRequest(http.MethodGet, "/events/"+eventID+"/merch", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ─── GET /events/:id/merch/preview ───────────────────────────────────────────

func TestMerchGetPreview(t *testing.T) {
	pool := setupMerchTestDB(t)
	defer pool.Close()

	t.Run("returns at most 4 items for event details page", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		merchSeedItems(t, pool, eventID, 6)

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch/preview", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 4, "preview must return at most 4 items")
	})

	t.Run("returns all items when fewer than 4 exist", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		merchSeedItems(t, pool, eventID, 2)

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch/preview", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Len(t, resp["data"].([]interface{}), 2)
	})

	t.Run("preview respects display_order — shows first 4 by order", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		for i := 1; i <= 5; i++ {
			pool.Exec(context.Background(), `
				INSERT INTO merchandise (event_id, name, base_price, display_order)
				VALUES ($1, $2, 100, $3)
			`, eventID, fmt.Sprintf("Item %d", i), i)
		}

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch/preview", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 4)
		assert.Equal(t, "Item 1", data[0].(map[string]interface{})["name"])
		assert.Equal(t, "Item 4", data[3].(map[string]interface{})["name"])
	})

	t.Run("inactive items excluded from preview", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		pool.Exec(context.Background(), `
			INSERT INTO merchandise (event_id, name, base_price, is_active, display_order)
			VALUES ($1, 'Visible', 100, TRUE, 1),
			       ($1, 'Hidden', 100, FALSE, 0)
		`, eventID)

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch/preview", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)
		assert.Equal(t, "Visible", data[0].(map[string]interface{})["name"])
	})
}

// ─── GET /merch/:id — single item detail page ─────────────────────────────────

func TestMerchGetByID(t *testing.T) {
	pool := setupMerchTestDB(t)
	defer pool.Close()

	t.Run("returns single item with variants and stock summary", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		merchSeedVariant(t, pool, ids[0], "S", 3)
		merchSeedVariant(t, pool, ids[0], "M", 10)
		merchSeedVariant(t, pool, ids[0], "L", 0) // out of stock

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/merch/"+ids[0], nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		// Item fields
		assert.Equal(t, ids[0], resp["id"])
		assert.Equal(t, "Item 1", resp["name"])

		// Variants nested
		variants := resp["variants"].([]interface{})
		assert.Len(t, variants, 3, "all 3 variants should be returned including out-of-stock")

		// Stock summary
		summary := resp["stock_summary"].(map[string]interface{})
		assert.EqualValues(t, 13, summary["total_stock"])    // 3 + 10 + 0
		assert.EqualValues(t, 2, summary["available_variants"]) // S and M have stock
		     // total 13 <= 10? No actually > 10
		// total = 13 which is > 10 so status should be "available"
		assert.Equal(t, "available", summary["status"])
	})

	t.Run("stock summary shows sold_out when all variants have zero stock", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		merchSeedVariant(t, pool, ids[0], "S", 0)
		merchSeedVariant(t, pool, ids[0], "M", 0)

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/merch/"+ids[0], nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		summary := resp["stock_summary"].(map[string]interface{})
		assert.Equal(t, "sold_out", summary["status"])
		assert.EqualValues(t, 0, summary["total_stock"])
		assert.EqualValues(t, 0, summary["available_variants"])
	})

	t.Run("stock summary shows low_stock when total is <= 10", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		merchSeedVariant(t, pool, ids[0], "S", 3)
		merchSeedVariant(t, pool, ids[0], "M", 4) // total = 7

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/merch/"+ids[0], nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		summary := resp["stock_summary"].(map[string]interface{})
		assert.Equal(t, "low_stock", summary["status"])
	})

	t.Run("returns 404 for non-existent merch ID", func(t *testing.T) {
		cleanMerchTables(t, pool)
		r := buildMerchRouter(pool)
		fakeID := "00000000-0000-0000-0000-000000000000"
		w := doRequest(r, http.MethodGet, "/merch/"+fakeID, nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 404 for inactive item", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		var inactiveID string
		pool.QueryRow(context.Background(), `
			INSERT INTO merchandise (event_id, name, base_price, is_active)
			VALUES ($1, 'Discontinued', 100, FALSE)
			RETURNING id
		`, eventID).Scan(&inactiveID)

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/merch/"+inactiveID, nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("no auth required — public endpoint", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)

		r := buildMerchRouter(pool)
		req := httptest.NewRequest(http.MethodGet, "/merch/"+ids[0], nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ─── POST /organiser/events/:id/merch ─────────────────────────────────────────

func TestMerchOrgCreate(t *testing.T) {
	pool := setupMerchTestDB(t)
	defer pool.Close()

	t.Run("organiser creates merch item successfully", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/merch",
			map[string]interface{}{
				"name":          "Chelsea Home Kit",
				"description":   "Official 2025/26 home kit",
				"base_price":    1200.00,
				"category":      "apparel",
				"display_order": 1,
			}, orgToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Chelsea Home Kit", resp["name"])
		assert.EqualValues(t, 1200.00, resp["base_price"])
		assert.Equal(t, eventID, resp["event_id"])
		assert.Equal(t, true, resp["is_active"])
		assert.NotEmpty(t, resp["id"])
	})

	t.Run("admin can also create merch", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		_, adminToken := merchSeedUserWithRole(t, pool, "admin")
		eventID := merchSeedEvent(t, pool, orgID)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/merch",
			map[string]interface{}{
				"name": "Admin Scarf", "base_price": 250.00,
			}, adminToken)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("regular user gets 403", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		_, userToken := merchSeedUserWithRole(t, pool, "attendee")
		eventID := merchSeedEvent(t, pool, orgID)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/merch",
			map[string]interface{}{"name": "Scarf", "base_price": 250.00}, userToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated request gets 401", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		r := buildMerchRouter(pool)

		w := doRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/merch",
			map[string]interface{}{"name": "Scarf", "base_price": 250.00})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("missing name returns 400", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/merch",
			map[string]interface{}{"base_price": 250.00}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("price of zero returns 400", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/merch",
			map[string]interface{}{"name": "Scarf", "base_price": 0}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── PUT /organiser/merch/:id ─────────────────────────────────────────────────

func TestMerchOrgUpdate(t *testing.T) {
	pool := setupMerchTestDB(t)
	defer pool.Close()

	t.Run("organiser updates name and price successfully", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/"+ids[0],
			map[string]interface{}{
				"name":       "Updated Kit Name",
				"base_price": 1500.00,
			}, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		item := resp["item"].(map[string]interface{})
		assert.Equal(t, "Updated Kit Name", item["name"])
		assert.EqualValues(t, 1500.00, item["base_price"])
	})

	t.Run("partial update — only provided fields change", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		// Only update category
		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/"+ids[0],
			map[string]interface{}{"category": "apparel"}, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		item := resp["item"].(map[string]interface{})
		// Name should be unchanged
		assert.Equal(t, "Item 1", item["name"])
		assert.Equal(t, "apparel", item["category"])
	})

	t.Run("organiser cannot update another organiser's merch — 404", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		_, otherToken := merchSeedUserWithRole(t, pool, "organizer") // different organiser
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/"+ids[0],
			map[string]interface{}{"name": "Hacked Name"}, otherToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("empty body returns 400 — no fields to update", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/"+ids[0],
			map[string]interface{}{}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unauthenticated gets 401", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		w := doRequest(r, http.MethodPut, "/organiser/merch/"+ids[0],
			map[string]interface{}{"name": "Hacked"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── DELETE /organiser/merch/:id ─────────────────────────────────────────────

func TestMerchOrgDeactivate(t *testing.T) {
	pool := setupMerchTestDB(t)
	defer pool.Close()

	t.Run("organiser deactivates their merch item successfully", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/organiser/merch/"+ids[0], nil, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["message"], "removed from listing")

		// Verify it no longer appears in public catalogue
		w2 := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch", nil)
		require.Equal(t, http.StatusOK, w2.Code)
		var catalogueResp map[string]interface{}
		require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &catalogueResp))
		assert.EqualValues(t, 0, catalogueResp["count"],
			"deactivated item must not appear in public catalogue")
	})

	t.Run("organiser cannot deactivate another organiser's merch — 404", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		_, otherToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/organiser/merch/"+ids[0], nil, otherToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("deactivating already-inactive item returns 404", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		var inactiveID string
		pool.QueryRow(context.Background(), `
			INSERT INTO merchandise (event_id, name, base_price, is_active)
			VALUES ($1, 'Already Gone', 100, FALSE)
			RETURNING id
		`, eventID).Scan(&inactiveID)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/organiser/merch/"+inactiveID, nil, orgToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unauthenticated gets 401", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		w := doRequest(r, http.MethodDelete, "/organiser/merch/"+ids[0], nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── PUT /organiser/merch/variants/:id ───────────────────────────────────────

func TestMerchOrgUpdateVariant(t *testing.T) {
	pool := setupMerchTestDB(t)
	defer pool.Close()

	t.Run("organiser restocks a variant", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		variantID := merchSeedVariant(t, pool, ids[0], "M", 5)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/variants/"+variantID,
			map[string]interface{}{"stock_quantity": 50}, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		variant := resp["variant"].(map[string]interface{})
		assert.EqualValues(t, 50, variant["stock_quantity"])
	})

	t.Run("organiser sets a price override on a variant", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		variantID := merchSeedVariant(t, pool, ids[0], "XL", 10)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/variants/"+variantID,
			map[string]interface{}{"price_override": 1800.00}, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		variant := resp["variant"].(map[string]interface{})
		assert.EqualValues(t, 1800.00, variant["price_override"])
	})

	t.Run("organiser deactivates a specific variant", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		variantID := merchSeedVariant(t, pool, ids[0], "S", 10)
		r := buildMerchRouter(pool)

		isActive := false
		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/variants/"+variantID,
			map[string]interface{}{"is_active": isActive}, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		variant := resp["variant"].(map[string]interface{})
		assert.Equal(t, false, variant["is_active"])
	})

	t.Run("stock_status field reflects low_stock correctly", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		variantID := merchSeedVariant(t, pool, ids[0], "L", 20)
		r := buildMerchRouter(pool)

		// Reduce stock to 3 — should become low_stock
		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/variants/"+variantID,
			map[string]interface{}{"stock_quantity": 3}, orgToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "low_stock", resp["stock_status"])
	})

	t.Run("organiser cannot update another organiser's variant — 404", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		_, otherToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		variantID := merchSeedVariant(t, pool, ids[0], "M", 10)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/variants/"+variantID,
			map[string]interface{}{"stock_quantity": 999}, otherToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("empty body returns 400", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		variantID := merchSeedVariant(t, pool, ids[0], "M", 10)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/variants/"+variantID,
			map[string]interface{}{}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unauthenticated gets 401", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		variantID := merchSeedVariant(t, pool, ids[0], "M", 10)
		r := buildMerchRouter(pool)

		w := doRequest(r, http.MethodPut, "/organiser/merch/variants/"+variantID,
			map[string]interface{}{"stock_quantity": 999})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── POST /organiser/merch/:id/variants ───────────────────────────────────────

func TestMerchOrgCreateVariant(t *testing.T) {
	pool := setupMerchTestDB(t)
	defer pool.Close()

	t.Run("organiser adds a size variant to merch item", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		sku := fmt.Sprintf("CHELSEA-KIT-M-%d", time.Now().UnixNano())
		w := doAuthRequest(r, http.MethodPost, "/organiser/merch/"+ids[0]+"/variants",
			map[string]interface{}{
				"sku":            sku,
				"attributes":     `{"size":"M"}`,
				"stock_quantity": 15,
			}, orgToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, sku, resp["sku"])
		assert.EqualValues(t, 15, resp["stock_quantity"])
		assert.Equal(t, ids[0], resp["merchandise_id"])
		assert.Equal(t, true, resp["is_active"])
	})

	t.Run("variant with size and colour attributes", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		sku := fmt.Sprintf("KIT-M-HOME-%d", time.Now().UnixNano())
		w := doAuthRequest(r, http.MethodPost, "/organiser/merch/"+ids[0]+"/variants",
			map[string]interface{}{
				"sku":            sku,
				"attributes":     `{"size":"M","colour":"home"}`,
				"stock_quantity": 20,
			}, orgToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		attrs := resp["attributes"].(string)
		assert.Contains(t, attrs, "M")
		assert.Contains(t, attrs, "home")
	})

	t.Run("variant with price_override", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		sku := fmt.Sprintf("XL-PREMIUM-%d", time.Now().UnixNano())
		w := doAuthRequest(r, http.MethodPost, "/organiser/merch/"+ids[0]+"/variants",
			map[string]interface{}{
				"sku":            sku,
				"attributes":     `{"size":"XL"}`,
				"stock_quantity": 5,
				"price_override": 150.00,
			}, orgToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.EqualValues(t, 150.00, resp["price_override"])
	})

	t.Run("duplicate SKU returns 500", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		const dupSKU = "DUPLICATE-SKU-001"
		pool.Exec(context.Background(), `
			INSERT INTO merchandise_variants (merchandise_id, sku, attributes)
			VALUES ($1, $2, '{}')
		`, ids[0], dupSKU)

		r := buildMerchRouter(pool)
		w := doAuthRequest(r, http.MethodPost, "/organiser/merch/"+ids[0]+"/variants",
			map[string]interface{}{
				"sku":            dupSKU,
				"attributes":     `{"size":"S"}`,
				"stock_quantity": 5,
			}, orgToken)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("missing SKU returns 400", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/merch/"+ids[0]+"/variants",
			map[string]interface{}{
				"attributes":     `{"size":"M"}`,
				"stock_quantity": 5,
			}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing attributes returns 400", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, orgToken := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		sku := fmt.Sprintf("NO-ATTRS-%d", time.Now().UnixNano())
		w := doAuthRequest(r, http.MethodPost, "/organiser/merch/"+ids[0]+"/variants",
			map[string]interface{}{
				"sku":            sku,
				"stock_quantity": 5,
			}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("non-organiser gets 403", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		_, userToken := merchSeedUserWithRole(t, pool, "attendee")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/merch/"+ids[0]+"/variants",
			map[string]interface{}{
				"sku":            "SKU-BLOCKED",
				"attributes":     `{"size":"S"}`,
				"stock_quantity": 1,
			}, userToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated gets 401", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)
		r := buildMerchRouter(pool)

		w := doRequest(r, http.MethodPost, "/organiser/merch/"+ids[0]+"/variants",
			map[string]interface{}{
				"sku":            "SKU-ANON",
				"attributes":     `{"size":"M"}`,
				"stock_quantity": 5,
			})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── Variant integration tests ────────────────────────────────────────────────

func TestMerchVariantSizeIntegration(t *testing.T) {
	pool := setupMerchTestDB(t)
	defer pool.Close()

	t.Run("kit with S, M, L, XL variants all returned in catalogue", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)

		sizes := []string{"S", "M", "L", "XL"}
		for _, size := range sizes {
			merchSeedVariant(t, pool, ids[0], size, 10)
		}

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 1)
		item := data[0].(map[string]interface{})
		variants := item["variants"].([]interface{})
		assert.Len(t, variants, 4, "all 4 size variants should be returned")
	})

	t.Run("out of stock variant still appears — frontend shows out-of-stock label", func(t *testing.T) {
		cleanMerchTables(t, pool)
		orgID, _ := merchSeedUserWithRole(t, pool, "organizer")
		eventID := merchSeedEvent(t, pool, orgID)
		ids := merchSeedItems(t, pool, eventID, 1)

		pool.Exec(context.Background(), `
			INSERT INTO merchandise_variants (merchandise_id, sku, attributes, stock_quantity)
			VALUES ($1, 'SIZE-S-OOS', '{"size":"S"}'::jsonb, 0),
			       ($1, 'SIZE-M-INS', '{"size":"M"}'::jsonb, 10)
		`, ids[0])

		r := buildMerchRouter(pool)
		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/merch", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		variants := data[0].(map[string]interface{})["variants"].([]interface{})
		assert.Len(t, variants, 2,
			"out-of-stock variants should still appear so frontend can show out-of-stock label")

		for _, v := range variants {
			variant := v.(map[string]interface{})
			if variant["sku"] == "SIZE-S-OOS" {
				assert.EqualValues(t, 0, variant["stock_quantity"])
			}
		}
	})
}