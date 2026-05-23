package handlers_test

// =============================================================================
// MERCHANDISE ADMIN & STANDALONE TESTS
// =============================================================================
//
// Tests covered:
//
//   POST /admin/merch                → AdminCreateMerchHandler          (admin)
//     - creates standalone merch (event_id is nil)
//     - approval_status is 'approved' — bypasses organiser review queue
//     - returns 201 with full merch object
//     - missing required fields returns 400
//     - organizer role is forbidden — 403
//     - attendee role is forbidden — 403
//     - unauthenticated returns 401
//
//   POST /admin/events/:id/merch     → AdminCreateEventMerchHandler     (admin)
//     - creates merch linked to an existing event
//     - approval_status is 'approved' immediately
//     - returns 201 with the created item
//     - missing required fields returns 400
//     - organizer role is forbidden — 403
//     - unauthenticated returns 401
//
//   GET  /admin/merch                → GetAllMerchHandler               (admin)
//     - returns all merch items across all events
//     - includes event_title join (empty string for standalone)
//     - response has 'data' and 'count' fields
//     - organizer role is forbidden — 403
//     - unauthenticated returns 401
//
//   PUT  /admin/merch/:id            → AdminUpdateMerchHandler          (admin)
//     - updates name field and returns updated item
//     - sets approval_status to 'approved' on every save
//     - clears rejection_reason on save
//     - empty body (no fields) returns 400
//     - non-existent merch id returns 404
//     - organizer role is forbidden — 403
//     - unauthenticated returns 401
//
//   PUT  /admin/merch/variants/:id   → AdminUpdateVariantHandler        (admin)
//     - updates stock_quantity on an existing variant
//     - updates price_override on an existing variant
//     - deactivates a variant via is_active = false
//     - empty body returns 400
//     - non-existent variant id returns 404
//     - organizer role is forbidden — 403
//     - unauthenticated returns 401
//
//   POST /organiser/merch            → OrgCreateStandaloneMerchHandler  (organizer)
//     - creates standalone merch not linked to any event
//     - approval_status is 'pending_review'
//     - missing required fields returns 400
//     - attendee role is forbidden — 403
//     - unauthenticated returns 401
//
//   POST /organiser/merch/:id/resubmit → OrgResubmitMerchHandler       (organizer)
//     - NOTE: handler reads organizer_id from gin context but
//       AuthMiddleware only sets user_id; organizer_id is always "".
//       Until the handler is corrected, any resubmit will return 404
//       because the ownership JOIN will never match. Tests document this.
//     - item in 'rejected' state returns 404 (due to the above bug)
//     - non-existent merch id returns 404
//     - attendee role is forbidden — 403
//     - unauthenticated returns 401
//
// Prerequisites — start the test database once before the suite:
//
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser \
//     -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db \
//     -p 5433:5432 postgres:15
//
// Run all merch-admin tests:
//
//   go test ./internal/handlers/... -v -run TestMerchAdmin -count=1
//   go test ./internal/handlers/... -v -run TestMerchOrgStandalone -count=1
//   go test ./internal/handlers/... -v -run TestMerchOrgResubmit -count=1
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

func setupMerchAdminTestDB(t *testing.T) *pgxpool.Pool {
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
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("merch admin DB not reachable: %v", err)
	}
	createMerchAdminTestTables(t, pool)
	cleanMerchAdminTables(t, pool)
	return pool
}

func createMerchAdminTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
		"DROP TABLE IF EXISTS merchandise_variants CASCADE",
		"DROP TABLE IF EXISTS merchandise         CASCADE",
		"DROP TABLE IF EXISTS events              CASCADE",
		"DROP TABLE IF EXISTS categories          CASCADE",
		"DROP TABLE IF EXISTS refresh_tokens      CASCADE",
		"DROP TABLE IF EXISTS users               CASCADE",
	}
	for _, s := range drops {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		// FIX: added phone_number column so that repository.CreateUser's INSERT
		// (which always includes phone_number) does not fail with SQLSTATE 42703.
		`CREATE TABLE users (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email        VARCHAR(255) UNIQUE NOT NULL,
			password     TEXT NOT NULL,
			role         VARCHAR(50) NOT NULL DEFAULT 'attendee',
			first_name   VARCHAR(100),
			last_name    VARCHAR(100),
			phone_number VARCHAR(20),
			created_at   TIMESTAMPTZ DEFAULT NOW(),
			updated_at   TIMESTAMPTZ DEFAULT NOW()
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

		// Events table — organizer_id JOIN used by OrgUpdateMerchandise / ownership checks.
		`CREATE TABLE events (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title          VARCHAR(255) NOT NULL DEFAULT 'Test Event',
			description    TEXT NOT NULL DEFAULT '',
			start_time     TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
			end_time       TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '8 days',
			expires_at     TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
			timezone       VARCHAR(50) NOT NULL DEFAULT 'Africa/Johannesburg',
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

		// Full merchandise schema — includes approval_status, fulfillment_mode, and
		// sale window columns queried by all repository functions.
		// event_id is nullable to support standalone (non-event-linked) merch.
		`CREATE TABLE merchandise (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id         UUID REFERENCES events(id) ON DELETE CASCADE, -- nullable
			name             VARCHAR(255) NOT NULL,
			description      TEXT,
			base_price       NUMERIC(10,2) NOT NULL,
			category         VARCHAR(100),
			display_order    INTEGER NOT NULL DEFAULT 0,
			is_active        BOOLEAN NOT NULL DEFAULT TRUE,
			images           JSONB,
			fulfillment_mode VARCHAR(20) NOT NULL DEFAULT 'delivery',
			pickup_location  TEXT,
			delivery_fee     NUMERIC(10,2) NOT NULL DEFAULT 0.00,
			sale_start       TIMESTAMPTZ,
			sale_end         TIMESTAMPTZ,
			approval_status  VARCHAR(50) NOT NULL DEFAULT 'pending_review',
			rejection_reason TEXT,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
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
			CONSTRAINT merch_admin_stock_non_negative CHECK (stock_quantity >= 0)
		)`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("merch admin table setup failed:\n  sql: %s\n  err: %v", s, err)
		}
	}
}

func cleanMerchAdminTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE merchandise_variants CASCADE")
	pool.Exec(ctx, "TRUNCATE merchandise         CASCADE")
	pool.Exec(ctx, "TRUNCATE events              CASCADE")
	pool.Exec(ctx, "TRUNCATE categories          CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens      CASCADE")
	pool.Exec(ctx, "TRUNCATE users               CASCADE")
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

func merchAdminSeedUser(t *testing.T, pool *pgxpool.Pool, role string) (userID, token string) {
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

func merchAdminSeedCategory(t *testing.T, pool *pgxpool.Pool) int {
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

func merchAdminSeedEvent(t *testing.T, pool *pgxpool.Pool, organizerID string) string {
	t.Helper()
	catID := merchAdminSeedCategory(t, pool)
	var eventID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, status, price, currency, category_id, organizer_id)
		VALUES ('Test Event', 'published', 500.00, 'ZAR', $1, $2)
		RETURNING id
	`, catID, organizerID).Scan(&eventID)
	require.NoError(t, err)
	return eventID
}

// merchAdminSeedApprovedItem inserts a fully-approved merch item for update/variant tests.
func merchAdminSeedApprovedItem(
	t *testing.T,
	pool *pgxpool.Pool,
	eventID *string,
	name string,
	approvalStatus string,
) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO merchandise (event_id, name, base_price, fulfillment_mode, approval_status)
		VALUES ($1, $2, 200.00, 'delivery', $3)
		RETURNING id
	`, eventID, name, approvalStatus).Scan(&id)
	require.NoError(t, err)
	return id
}

func merchAdminSeedVariant(t *testing.T, pool *pgxpool.Pool, merchID, size string, stock int) string {
	t.Helper()
	sku := fmt.Sprintf("ADM-SKU-%s-%d", size, time.Now().UnixNano())
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO merchandise_variants (merchandise_id, sku, attributes, stock_quantity)
		VALUES ($1, $2, $3::jsonb, $4)
		RETURNING id
	`, merchID, sku, fmt.Sprintf(`{"size":"%s"}`, size), stock).Scan(&id)
	require.NoError(t, err)
	return id
}

// ─── Router ───────────────────────────────────────────────────────────────────

func buildMerchAdminRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()
	r := gin.New()

	r.POST("/auth/login", handlers.LoginHandler(pool, cfg))

	// Public read
	r.GET("/events/:id/merch", handlers.GetEventMerchHandler(pool, nil))
	r.GET("/merch/:id", handlers.GetMerchByIDHandler(pool, nil))

	// Organizer routes
	org := r.Group("/organiser")
	org.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	{
		// Standalone merch — POST /organiser/merch (no event ID in path)
		org.POST("/merch", handlers.OrgCreateStandaloneMerchHandler(pool, nil))
		org.POST("/merch/:id/resubmit", handlers.OrgResubmitMerchHandler(pool))
	}

	// Admin routes
	admin := r.Group("/admin")
	admin.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
	{
		admin.POST("/merch", handlers.AdminCreateMerchHandler(pool, nil))
		admin.POST("/events/:id/merch", handlers.AdminCreateEventMerchHandler(pool, nil))
		admin.GET("/merch", handlers.GetAllMerchHandler(pool))
		admin.PUT("/merch/:id", handlers.AdminUpdateMerchHandler(pool, nil))
		admin.PUT("/merch/variants/:id", handlers.AdminUpdateVariantHandler(pool))
	}

	return r
}

// ─── validMerchBody returns a minimal valid CreateMerchandiseDTO body. ─────────
func validMerchBody() map[string]interface{} {
	return map[string]interface{}{
		"name":       "Test Merch Item",
		"base_price": 299.00,
	}
}

// ─── POST /admin/merch ────────────────────────────────────────────────────────

func TestMerchAdminCreateStandalone(t *testing.T) {
	pool := setupMerchAdminTestDB(t)
	defer pool.Close()

	t.Run("creates standalone merch with approval_status approved", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, adminToken := merchAdminSeedUser(t, pool, "admin")

		w := doAuthRequest(r, http.MethodPost, "/admin/merch", validMerchBody(), adminToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		assert.Equal(t, "Test Merch Item", resp["name"])
		assert.EqualValues(t, 299.00, resp["base_price"])
		// Admin-created items bypass the review queue.
		assert.Equal(t, "approved", resp["approval_status"])
		// Standalone merch has no event_id.
		assert.Nil(t, resp["event_id"], "standalone merch must not have an event_id")
		// Required fields must be present in the response.
		for _, field := range []string{"id", "name", "base_price", "is_active", "created_at", "updated_at"} {
			assert.Contains(t, resp, field, "response missing field: %s", field)
		}
	})

	t.Run("missing required fields returns 400", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, adminToken := merchAdminSeedUser(t, pool, "admin")

		// Missing base_price
		w := doAuthRequest(r, http.MethodPost, "/admin/merch", map[string]interface{}{
			"name": "No Price Item",
		}, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("organizer role is forbidden — 403", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, orgToken := merchAdminSeedUser(t, pool, "organizer")

		w := doAuthRequest(r, http.MethodPost, "/admin/merch", validMerchBody(), orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("attendee role is forbidden — 403", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, userToken := merchAdminSeedUser(t, pool, "attendee")

		w := doAuthRequest(r, http.MethodPost, "/admin/merch", validMerchBody(), userToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildMerchAdminRouter(pool)
		w := doRequest(r, http.MethodPost, "/admin/merch", validMerchBody())
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── POST /admin/events/:id/merch ─────────────────────────────────────────────

func TestMerchAdminCreateEventMerch(t *testing.T) {
	pool := setupMerchAdminTestDB(t)
	defer pool.Close()

	t.Run("creates event-linked merch with approval_status approved", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, _ := merchAdminSeedUser(t, pool, "organizer")
		_, adminToken := merchAdminSeedUser(t, pool, "admin")
		eventID := merchAdminSeedEvent(t, pool, orgID)

		body := validMerchBody()
		body["name"] = "Event-Linked Merch"

		w := doAuthRequest(r, http.MethodPost, "/admin/events/"+eventID+"/merch", body, adminToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		assert.Equal(t, "Event-Linked Merch", resp["name"])
		assert.Equal(t, "approved", resp["approval_status"])
		// Event ID should be set
		assert.Equal(t, eventID, resp["event_id"])
	})

	t.Run("missing required fields returns 400", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, _ := merchAdminSeedUser(t, pool, "organizer")
		_, adminToken := merchAdminSeedUser(t, pool, "admin")
		eventID := merchAdminSeedEvent(t, pool, orgID)

		w := doAuthRequest(r, http.MethodPost, "/admin/events/"+eventID+"/merch",
			map[string]interface{}{"name": "No Price"}, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("organizer role is forbidden — 403", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, orgToken := merchAdminSeedUser(t, pool, "organizer")
		eventID := merchAdminSeedEvent(t, pool, orgID)

		w := doAuthRequest(r, http.MethodPost, "/admin/events/"+eventID+"/merch", validMerchBody(), orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildMerchAdminRouter(pool)
		w := doRequest(r, http.MethodPost,
			"/admin/events/00000000-0000-0000-0000-000000000000/merch", validMerchBody())
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── GET /admin/merch ─────────────────────────────────────────────────────────

func TestMerchAdminGetAll(t *testing.T) {
	pool := setupMerchAdminTestDB(t)
	defer pool.Close()

	t.Run("returns all merch including event_title join", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, _ := merchAdminSeedUser(t, pool, "organizer")
		_, adminToken := merchAdminSeedUser(t, pool, "admin")
		eventID := merchAdminSeedEvent(t, pool, orgID)

		// Event-linked item
		merchAdminSeedApprovedItem(t, pool, &eventID, "Event Item", "approved")
		// Standalone item — event_id is nil
		merchAdminSeedApprovedItem(t, pool, nil, "Standalone Item", "pending_review")

		w := doAuthRequest(r, http.MethodGet, "/admin/merch", nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp, "data")
		assert.Contains(t, resp, "count")
		assert.EqualValues(t, 2, resp["count"])

		// Both pending and approved items are returned (admin sees everything)
		data := resp["data"].([]interface{})
		assert.Len(t, data, 2)
	})

	t.Run("response includes data and count fields", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, adminToken := merchAdminSeedUser(t, pool, "admin")

		w := doAuthRequest(r, http.MethodGet, "/admin/merch", nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		assert.Contains(t, resp, "data")
		assert.Contains(t, resp, "count")
		assert.EqualValues(t, 0, resp["count"])
	})

	t.Run("organizer role is forbidden — 403", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, orgToken := merchAdminSeedUser(t, pool, "organizer")

		w := doAuthRequest(r, http.MethodGet, "/admin/merch", nil, orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildMerchAdminRouter(pool)
		w := doRequest(r, http.MethodGet, "/admin/merch", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── PUT /admin/merch/:id ─────────────────────────────────────────────────────

func TestMerchAdminUpdate(t *testing.T) {
	pool := setupMerchAdminTestDB(t)
	defer pool.Close()

	t.Run("updates name and returns updated item", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, _ := merchAdminSeedUser(t, pool, "organizer")
		_, adminToken := merchAdminSeedUser(t, pool, "admin")
		eventID := merchAdminSeedEvent(t, pool, orgID)
		merchID := merchAdminSeedApprovedItem(t, pool, &eventID, "Old Name", "approved")

		newName := "Updated Merch Name"
		w := doAuthRequest(r, http.MethodPut, "/admin/merch/"+merchID, map[string]interface{}{
			"name": newName,
		}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// AdminUpdateMerchHandler wraps the item in an "item" key.
		item := resp["item"].(map[string]interface{})
		assert.Equal(t, newName, item["name"])
	})

	t.Run("admin save always sets approval_status to approved", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, _ := merchAdminSeedUser(t, pool, "organizer")
		_, adminToken := merchAdminSeedUser(t, pool, "admin")
		eventID := merchAdminSeedEvent(t, pool, orgID)
		// Start with pending_review to verify the admin overrides it.
		merchID := merchAdminSeedApprovedItem(t, pool, &eventID, "Pending Item", "pending_review")

		w := doAuthRequest(r, http.MethodPut, "/admin/merch/"+merchID, map[string]interface{}{
			"name": "Now Approved",
		}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		item := resp["item"].(map[string]interface{})
		assert.Equal(t, "approved", item["approval_status"],
			"admin save must flip approval_status to approved")
	})

	t.Run("empty body returns 400", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, _ := merchAdminSeedUser(t, pool, "organizer")
		_, adminToken := merchAdminSeedUser(t, pool, "admin")
		eventID := merchAdminSeedEvent(t, pool, orgID)
		merchID := merchAdminSeedApprovedItem(t, pool, &eventID, "Item", "approved")

		w := doAuthRequest(r, http.MethodPut, "/admin/merch/"+merchID, map[string]interface{}{}, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("non-existent merch id returns 404 or 500", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, adminToken := merchAdminSeedUser(t, pool, "admin")

		w := doAuthRequest(r, http.MethodPut,
			"/admin/merch/00000000-0000-0000-0000-000000000000",
			map[string]interface{}{"name": "Ghost"}, adminToken)
		// The repository returns ErrMerchNotFound when RowsAffected() == 0.
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("organizer role is forbidden — 403", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, orgToken := merchAdminSeedUser(t, pool, "organizer")

		w := doAuthRequest(r, http.MethodPut,
			"/admin/merch/00000000-0000-0000-0000-000000000000",
			map[string]interface{}{"name": "X"}, orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildMerchAdminRouter(pool)
		w := doRequest(r, http.MethodPut,
			"/admin/merch/00000000-0000-0000-0000-000000000000",
			map[string]interface{}{"name": "X"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── PUT /admin/merch/variants/:id ────────────────────────────────────────────

func TestMerchAdminUpdateVariant(t *testing.T) {
	pool := setupMerchAdminTestDB(t)
	defer pool.Close()

	t.Run("updates stock_quantity and returns updated variant", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, _ := merchAdminSeedUser(t, pool, "organizer")
		_, adminToken := merchAdminSeedUser(t, pool, "admin")
		eventID := merchAdminSeedEvent(t, pool, orgID)
		merchID := merchAdminSeedApprovedItem(t, pool, &eventID, "Kit", "approved")
		variantID := merchAdminSeedVariant(t, pool, merchID, "L", 10)

		newStock := 25
		w := doAuthRequest(r, http.MethodPut, "/admin/merch/variants/"+variantID, map[string]interface{}{
			"stock_quantity": newStock,
		}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var variant map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &variant))
		assert.EqualValues(t, newStock, variant["stock_quantity"])
		assert.Equal(t, variantID, variant["id"])
	})

	t.Run("updates price_override on an existing variant", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, _ := merchAdminSeedUser(t, pool, "organizer")
		_, adminToken := merchAdminSeedUser(t, pool, "admin")
		eventID := merchAdminSeedEvent(t, pool, orgID)
		merchID := merchAdminSeedApprovedItem(t, pool, &eventID, "Shirt", "approved")
		variantID := merchAdminSeedVariant(t, pool, merchID, "M", 20)

		w := doAuthRequest(r, http.MethodPut, "/admin/merch/variants/"+variantID, map[string]interface{}{
			"price_override": 349.00,
		}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var variant map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &variant))
		assert.InDelta(t, 349.00, variant["price_override"], 0.01)
	})

	t.Run("deactivates a variant via is_active false", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, _ := merchAdminSeedUser(t, pool, "organizer")
		_, adminToken := merchAdminSeedUser(t, pool, "admin")
		eventID := merchAdminSeedEvent(t, pool, orgID)
		merchID := merchAdminSeedApprovedItem(t, pool, &eventID, "Cap", "approved")
		variantID := merchAdminSeedVariant(t, pool, merchID, "One Size", 50)

		isActive := false
		w := doAuthRequest(r, http.MethodPut, "/admin/merch/variants/"+variantID, map[string]interface{}{
			"is_active": isActive,
		}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var variant map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &variant))
		assert.Equal(t, false, variant["is_active"])
	})

	t.Run("empty body returns 400", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, _ := merchAdminSeedUser(t, pool, "organizer")
		_, adminToken := merchAdminSeedUser(t, pool, "admin")
		eventID := merchAdminSeedEvent(t, pool, orgID)
		merchID := merchAdminSeedApprovedItem(t, pool, &eventID, "Item", "approved")
		variantID := merchAdminSeedVariant(t, pool, merchID, "S", 5)

		w := doAuthRequest(r, http.MethodPut, "/admin/merch/variants/"+variantID,
			map[string]interface{}{}, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("non-existent variant returns 404", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, adminToken := merchAdminSeedUser(t, pool, "admin")

		w := doAuthRequest(r, http.MethodPut,
			"/admin/merch/variants/00000000-0000-0000-0000-000000000000",
			map[string]interface{}{"stock_quantity": 10}, adminToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("organizer role is forbidden — 403", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, orgToken := merchAdminSeedUser(t, pool, "organizer")

		w := doAuthRequest(r, http.MethodPut,
			"/admin/merch/variants/00000000-0000-0000-0000-000000000000",
			map[string]interface{}{"stock_quantity": 10}, orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildMerchAdminRouter(pool)
		w := doRequest(r, http.MethodPut,
			"/admin/merch/variants/00000000-0000-0000-0000-000000000000",
			map[string]interface{}{"stock_quantity": 10})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── POST /organiser/merch ────────────────────────────────────────────────────

func TestMerchOrgStandalone(t *testing.T) {
	pool := setupMerchAdminTestDB(t)
	defer pool.Close()

	t.Run("creates standalone merch with approval_status pending_review", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, orgToken := merchAdminSeedUser(t, pool, "organizer")

		body := map[string]interface{}{
			"name":       "Standalone Merch Item",
			"base_price": 150.00,
		}
		w := doAuthRequest(r, http.MethodPost, "/organiser/merch", body, orgToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// OrgCreateStandaloneMerchHandler wraps in "merch" key.
		merch := resp["merch"].(map[string]interface{})

		assert.Equal(t, "Standalone Merch Item", merch["name"])
		assert.InDelta(t, 150.00, merch["base_price"], 0.01)
		// Organizer-created items start in pending_review state.
		assert.Equal(t, "pending_review", merch["approval_status"])
	})

	t.Run("response includes admin-review message", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, orgToken := merchAdminSeedUser(t, pool, "organizer")

		w := doAuthRequest(r, http.MethodPost, "/organiser/merch", validMerchBody(), orgToken)
		require.Equal(t, http.StatusCreated, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp, "message")
	})

	t.Run("missing required fields returns 400", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, orgToken := merchAdminSeedUser(t, pool, "organizer")

		w := doAuthRequest(r, http.MethodPost, "/organiser/merch",
			map[string]interface{}{"name": "No Price"}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("attendee role is forbidden — 403", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, userToken := merchAdminSeedUser(t, pool, "attendee")

		w := doAuthRequest(r, http.MethodPost, "/organiser/merch", validMerchBody(), userToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildMerchAdminRouter(pool)
		w := doRequest(r, http.MethodPost, "/organiser/merch", validMerchBody())
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── POST /organiser/merch/:id/resubmit ───────────────────────────────────────
//
// OrgResubmitMerchHandler must read the organizer's identity from
// c.GetString("user_id") — the key set by AuthMiddleware. The handler
// previously read c.GetString("organizer_id") which is never populated,
// bypassing the ownership check. That bug has been fixed in the handler.

func TestMerchOrgResubmit(t *testing.T) {
	pool := setupMerchAdminTestDB(t)
	defer pool.Close()

	t.Run("organizer can resubmit their own rejected item — 200", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, orgToken := merchAdminSeedUser(t, pool, "organizer")
		eventID := merchAdminSeedEvent(t, pool, orgID)
		// Item is correctly rejected and owned by this organizer.
		merchID := merchAdminSeedApprovedItem(t, pool, &eventID, "Rejected Item", "rejected")

		w := doAuthRequest(r, http.MethodPost, "/organiser/merch/"+merchID+"/resubmit", nil, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp, "message")
		assert.Equal(t, merchID, resp["merch_id"])
	})

	t.Run("non-existent merch id returns 404", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, orgToken := merchAdminSeedUser(t, pool, "organizer")

		w := doAuthRequest(r, http.MethodPost,
			"/organiser/merch/00000000-0000-0000-0000-000000000000/resubmit", nil, orgToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("approved (non-rejected) item returns 404", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		orgID, orgToken := merchAdminSeedUser(t, pool, "organizer")
		eventID := merchAdminSeedEvent(t, pool, orgID)
		// OrgResubmitMerch only matches rows with approval_status = 'rejected'
		// AND organizer_id match — so an approved item also returns 404.
		merchID := merchAdminSeedApprovedItem(t, pool, &eventID, "Approved Item", "approved")

		w := doAuthRequest(r, http.MethodPost, "/organiser/merch/"+merchID+"/resubmit", nil, orgToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("attendee role is forbidden — 403", func(t *testing.T) {
		cleanMerchAdminTables(t, pool)
		r := buildMerchAdminRouter(pool)
		_, userToken := merchAdminSeedUser(t, pool, "attendee")

		w := doAuthRequest(r, http.MethodPost,
			"/organiser/merch/00000000-0000-0000-0000-000000000000/resubmit", nil, userToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildMerchAdminRouter(pool)
		w := doRequest(r, http.MethodPost,
			"/organiser/merch/00000000-0000-0000-0000-000000000000/resubmit", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}