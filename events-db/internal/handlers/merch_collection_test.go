package handlers_test

// =============================================================================
// MERCH COLLECTION TESTS
// =============================================================================
//
// Tests covered:
//   - POST   /organiser/collections          → OrgCreateCollectionHandler
//   - GET    /organiser/collections          → OrgGetCollectionsHandler
//   - PUT    /organiser/collections/:id      → OrgUpdateCollectionHandler
//   - DELETE /organiser/collections/:id      → OrgDeactivateCollectionHandler
//   - PUT    /organiser/merch/:id/collection → OrgAssignMerchToCollectionHandler
//   - GET    /collections/:id               → GetCollectionByIDHandler   (public)
//   - POST   /admin/collections             → AdminCreateCollectionHandler
//   - GET    /admin/collections             → AdminGetCollectionsHandler
//   - PUT    /admin/collections/:id         → AdminUpdateCollectionHandler
//   - DELETE /admin/collections/:id         → AdminDeactivateCollectionHandler
//   - PUT    /admin/merch/:id/collection    → AdminAssignMerchToCollectionHandler
//
// Prerequisites — start the test database once before running:
//
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser \
//     -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db \
//     -p 5433:5432 postgres:15
//
// Run all collection tests:
//
//   go test ./internal/handlers/... -v -run TestCollection
//
// =============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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

func setupCollectionTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Skipf("collection DB tests skipped — could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("collection DB tests skipped — DB not reachable: %v", pingErr)
	}
	createCollectionTestTables(t, pool)
	cleanCollectionTables(t, pool)
	return pool
}

func createCollectionTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
		"DROP TABLE IF EXISTS merchandise_variants CASCADE",
		"DROP TABLE IF EXISTS merchandise CASCADE",
		"DROP TABLE IF EXISTS merch_collections CASCADE",
		"DROP TABLE IF EXISTS events CASCADE",
		"DROP TABLE IF EXISTS categories CASCADE",
		"DROP TABLE IF EXISTS refresh_tokens CASCADE",
		"DROP TABLE IF EXISTS users CASCADE",
	}
	for _, s := range drops {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		// VARCHAR(50) for role — avoids dependency on the production user_role enum.
		// OrganizerOnlyMiddleware and AdminOnlyMiddleware compare role as a string.
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

		// events.organizer_id references users.id directly — matches the pattern
		// used by UpdateMerchandise and DeactivateMerchandise ownership checks.
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

		`CREATE TABLE merch_collections (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			organizer_id  UUID,
			name          VARCHAR(255) NOT NULL,
			description   TEXT,
			display_order INTEGER NOT NULL DEFAULT 0,
			is_active     BOOLEAN NOT NULL DEFAULT TRUE,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE TABLE merchandise (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id      UUID REFERENCES events(id) ON DELETE CASCADE,
			collection_id UUID REFERENCES merch_collections(id) ON DELETE SET NULL,
			name          VARCHAR(255) NOT NULL,
			description   TEXT,
			base_price    NUMERIC(10,2) NOT NULL,
			category      VARCHAR(100),
			display_order INTEGER NOT NULL DEFAULT 0,
			is_active     BOOLEAN NOT NULL DEFAULT TRUE,
			images        JSONB,
			fulfillment_mode VARCHAR(50),
			pickup_location  TEXT,
			delivery_fee     NUMERIC(10,2),
			sale_start    TIMESTAMPTZ,
			sale_end      TIMESTAMPTZ,
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
			CONSTRAINT col_stock_non_negative CHECK (stock_quantity >= 0)
		)`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("collection table setup failed:\n  sql: %s\n  err: %v", s, err)
		}
	}
}

func cleanCollectionTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE merchandise_variants CASCADE")
	pool.Exec(ctx, "TRUNCATE merchandise CASCADE")
	pool.Exec(ctx, "TRUNCATE merch_collections CASCADE")
	pool.Exec(ctx, "TRUNCATE events CASCADE")
	pool.Exec(ctx, "TRUNCATE categories CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens CASCADE")
	pool.Exec(ctx, "TRUNCATE users CASCADE")
}

// ─── seed helpers ─────────────────────────────────────────────────────────────

func colSeedUserWithRole(t *testing.T, pool *pgxpool.Pool, role string) (userID, token string) {
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

func colSeedCategory(t *testing.T, pool *pgxpool.Pool) int {
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

func colSeedEvent(t *testing.T, pool *pgxpool.Pool, organizerID string) string {
	t.Helper()
	catID := colSeedCategory(t, pool)
	var eventID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, status, price, currency, category_id, organizer_id)
		VALUES ('Test Event', 'published', 500.00, 'ZAR', $1, $2)
		RETURNING id
	`, catID, organizerID).Scan(&eventID)
	require.NoError(t, err)
	return eventID
}

// colSeedCollection inserts a collection directly into the DB and returns its ID.
// Pass organizerID as empty string for admin-owned collections (organizerID = NULL).
func colSeedCollection(t *testing.T, pool *pgxpool.Pool, organizerID, name string) string {
	t.Helper()
	var collectionID string
	var orgArg interface{}
	if organizerID != "" {
		orgArg = organizerID
	}
	err := pool.QueryRow(context.Background(), `
		INSERT INTO merch_collections (organizer_id, name, display_order)
		VALUES ($1, $2, 0)
		RETURNING id
	`, orgArg, name).Scan(&collectionID)
	require.NoError(t, err)
	return collectionID
}

// colSeedMerch inserts a merch item linked to an event and returns its ID.
func colSeedMerch(t *testing.T, pool *pgxpool.Pool, eventID, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO merchandise (event_id, name, base_price)
		VALUES ($1, $2, 100.00)
		RETURNING id
	`, eventID, name).Scan(&id)
	require.NoError(t, err)
	return id
}

// colSeedMerchInCollection inserts a merch item pre-assigned to a collection.
func colSeedMerchInCollection(t *testing.T, pool *pgxpool.Pool, eventID, collectionID, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO merchandise (event_id, collection_id, name, base_price)
		VALUES ($1, $2, $3, 200.00)
		RETURNING id
	`, eventID, collectionID, name).Scan(&id)
	require.NoError(t, err)
	return id
}

// ─── router builder ───────────────────────────────────────────────────────────

func buildCollectionRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()
	r := gin.New()

	// Auth — needed so seed helpers can obtain JWT tokens
	auth := r.Group("/auth")
	auth.POST("/login", handlers.LoginHandler(pool, cfg))

	// Public — collection detail with items
	public := r.Group("/collections")
	{
		public.GET("/:id", handlers.GetCollectionByIDHandler(pool))
	}

	// Organiser — full collection CRUD + merch assignment
	org := r.Group("/organiser")
	org.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	{
		org.POST("/collections",        handlers.OrgCreateCollectionHandler(pool))
		org.GET("/collections",         handlers.OrgGetCollectionsHandler(pool))
		org.PUT("/collections/:id",     handlers.OrgUpdateCollectionHandler(pool))
		org.DELETE("/collections/:id",  handlers.OrgDeactivateCollectionHandler(pool))
		org.PUT("/merch/:id/collection", handlers.OrgAssignMerchToCollectionHandler(pool))
	}

	// Admin — full access, no ownership checks
	admin := r.Group("/admin")
	admin.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
	{
		admin.POST("/collections",        handlers.AdminCreateCollectionHandler(pool))
		admin.GET("/collections",         handlers.AdminGetCollectionsHandler(pool))
		admin.PUT("/collections/:id",     handlers.AdminUpdateCollectionHandler(pool))
		admin.DELETE("/collections/:id",  handlers.AdminDeactivateCollectionHandler(pool))
		admin.PUT("/merch/:id/collection", handlers.AdminAssignMerchToCollectionHandler(pool))
	}

	return r
}

// ─── POST /organiser/collections ─────────────────────────────────────────────

func TestCollectionOrgCreate(t *testing.T) {
	pool := setupCollectionTestDB(t)
	defer pool.Close()

	t.Run("organiser creates collection successfully", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, orgToken := colSeedUserWithRole(t, pool, "organizer")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/collections",
			map[string]interface{}{
				"name":          "Summer Drop 2025",
				"description":   "Limited edition summer collection",
				"display_order": 1,
			}, orgToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Summer Drop 2025", resp["name"])
		assert.Equal(t, "Limited edition summer collection", resp["description"])
		assert.Equal(t, true, resp["is_active"])
		assert.NotEmpty(t, resp["id"])
		assert.NotEmpty(t, resp["organizer_id"])
	})

	t.Run("collection is tied to the correct organiser", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/collections",
			map[string]interface{}{"name": "My Drop"}, orgToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, orgID, resp["organizer_id"])
	})

	t.Run("missing name returns 400", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, orgToken := colSeedUserWithRole(t, pool, "organizer")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/collections",
			map[string]interface{}{"description": "No name provided"}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("attendee gets 403", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, attendeeToken := colSeedUserWithRole(t, pool, "attendee")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/collections",
			map[string]interface{}{"name": "Blocked Drop"}, attendeeToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated gets 401", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		r := buildCollectionRouter(pool)

		w := doRequest(r, http.MethodPost, "/organiser/collections",
			map[string]interface{}{"name": "Anonymous Drop"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── GET /organiser/collections ──────────────────────────────────────────────

func TestCollectionOrgGetAll(t *testing.T) {
	pool := setupCollectionTestDB(t)
	defer pool.Close()

	t.Run("returns only the organiser's own collections", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		otherID, _ := colSeedUserWithRole(t, pool, "organizer")

		colSeedCollection(t, pool, orgID, "My Collection A")
		colSeedCollection(t, pool, orgID, "My Collection B")
		colSeedCollection(t, pool, otherID, "Other Organiser's Collection")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/organiser/collections", nil, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 2, "should only see own 2 collections, not the other organiser's")
		assert.EqualValues(t, 2, resp["count"])
	})

	t.Run("returns empty array when organiser has no collections", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, orgToken := colSeedUserWithRole(t, pool, "organizer")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/organiser/collections", nil, orgToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.EqualValues(t, 0, resp["count"])
	})

	t.Run("deactivated collections are excluded", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		activeID := colSeedCollection(t, pool, orgID, "Active Drop")
		_ = activeID

		// Insert an inactive collection directly
		pool.Exec(context.Background(), `
			INSERT INTO merch_collections (organizer_id, name, is_active)
			VALUES ($1, 'Inactive Drop', FALSE)
		`, orgID)

		r := buildCollectionRouter(pool)
		w := doAuthRequest(r, http.MethodGet, "/organiser/collections", nil, orgToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)
		assert.Equal(t, "Active Drop", data[0].(map[string]interface{})["name"])
	})

	t.Run("collections ordered by display_order ascending", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")

		pool.Exec(context.Background(), `
			INSERT INTO merch_collections (organizer_id, name, display_order)
			VALUES ($1, 'Third',  3),
			       ($1, 'First',  1),
			       ($1, 'Second', 2)
		`, orgID)

		r := buildCollectionRouter(pool)
		w := doAuthRequest(r, http.MethodGet, "/organiser/collections", nil, orgToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 3)
		assert.Equal(t, "First", data[0].(map[string]interface{})["name"])
		assert.Equal(t, "Second", data[1].(map[string]interface{})["name"])
		assert.Equal(t, "Third", data[2].(map[string]interface{})["name"])
	})

	t.Run("unauthenticated gets 401", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		r := buildCollectionRouter(pool)

		w := doRequest(r, http.MethodGet, "/organiser/collections", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── GET /collections/:id (public) ───────────────────────────────────────────

func TestCollectionGetByID(t *testing.T) {
	pool := setupCollectionTestDB(t)
	defer pool.Close()

	t.Run("returns collection with nested merch items", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		collectionID := colSeedCollection(t, pool, orgID, "Spring Drop")
		colSeedMerchInCollection(t, pool, eventID, collectionID, "Jersey")
		colSeedMerchInCollection(t, pool, eventID, collectionID, "Scarf")
		r := buildCollectionRouter(pool)

		w := doRequest(r, http.MethodGet, "/collections/"+collectionID, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, collectionID, resp["id"])
		assert.Equal(t, "Spring Drop", resp["name"])
		items := resp["items"].([]interface{})
		assert.Len(t, items, 2)
	})

	t.Run("collection with no items returns empty items array", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Empty Collection")
		r := buildCollectionRouter(pool)

		w := doRequest(r, http.MethodGet, "/collections/"+collectionID, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// items should be nil/absent or an empty array — not an error
		if items, ok := resp["items"]; ok && items != nil {
			assert.Len(t, items.([]interface{}), 0)
		}
	})

	t.Run("inactive merch items are excluded from collection", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		collectionID := colSeedCollection(t, pool, orgID, "Mixed Drop")

		colSeedMerchInCollection(t, pool, eventID, collectionID, "Active Item")
		// Insert inactive item directly
		pool.Exec(context.Background(), `
			INSERT INTO merchandise (event_id, collection_id, name, base_price, is_active)
			VALUES ($1, $2, 'Inactive Item', 100, FALSE)
		`, eventID, collectionID)

		r := buildCollectionRouter(pool)
		w := doRequest(r, http.MethodGet, "/collections/"+collectionID, nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		items := resp["items"].([]interface{})
		assert.Len(t, items, 1)
		assert.Equal(t, "Active Item", items[0].(map[string]interface{})["name"])
	})

	t.Run("returns 404 for non-existent collection", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		r := buildCollectionRouter(pool)

		w := doRequest(r, http.MethodGet, "/collections/00000000-0000-0000-0000-000000000000", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 404 for inactive collection", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		var inactiveID string
		pool.QueryRow(context.Background(), `
			INSERT INTO merch_collections (organizer_id, name, is_active)
			VALUES ($1, 'Hidden Drop', FALSE)
			RETURNING id
		`, orgID).Scan(&inactiveID)
		r := buildCollectionRouter(pool)

		w := doRequest(r, http.MethodGet, "/collections/"+inactiveID, nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("no auth required — public endpoint", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Public Drop")
		r := buildCollectionRouter(pool)

		req := httptest.NewRequest(http.MethodGet, "/collections/"+collectionID, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ─── PUT /organiser/collections/:id ──────────────────────────────────────────

func TestCollectionOrgUpdate(t *testing.T) {
	pool := setupCollectionTestDB(t)
	defer pool.Close()

	t.Run("organiser updates name and description successfully", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Old Name")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/collections/"+collectionID,
			map[string]interface{}{
				"name":        "New Name",
				"description": "Updated description",
			}, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		collection := resp["collection"].(map[string]interface{})
		assert.Equal(t, "New Name", collection["name"])
		assert.Equal(t, "Updated description", collection["description"])
	})

	t.Run("partial update — only provided fields change", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Unchanged Name")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/collections/"+collectionID,
			map[string]interface{}{"display_order": 5}, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		collection := resp["collection"].(map[string]interface{})
		assert.Equal(t, "Unchanged Name", collection["name"])
		assert.EqualValues(t, 5, collection["display_order"])
	})

	t.Run("organiser cannot update another organiser's collection — 404", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		_, otherToken := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Owner's Collection")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/collections/"+collectionID,
			map[string]interface{}{"name": "Hijacked Name"}, otherToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("empty body returns 400 — no fields to update", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Some Collection")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/collections/"+collectionID,
			map[string]interface{}{}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("non-existent collection returns 404", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, orgToken := colSeedUserWithRole(t, pool, "organizer")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/collections/00000000-0000-0000-0000-000000000000",
			map[string]interface{}{"name": "Ghost Update"}, orgToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unauthenticated gets 401", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Some Collection")
		r := buildCollectionRouter(pool)

		w := doRequest(r, http.MethodPut, "/organiser/collections/"+collectionID,
			map[string]interface{}{"name": "Anon Update"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── DELETE /organiser/collections/:id ───────────────────────────────────────

func TestCollectionOrgDeactivate(t *testing.T) {
	pool := setupCollectionTestDB(t)
	defer pool.Close()

	t.Run("organiser deactivates their collection successfully", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Drop To Remove")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/organiser/collections/"+collectionID, nil, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["message"], "removed")

		// Collection should no longer appear in the list
		w2 := doAuthRequest(r, http.MethodGet, "/organiser/collections", nil, orgToken)
		require.Equal(t, http.StatusOK, w2.Code)
		var listResp map[string]interface{}
		require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &listResp))
		assert.EqualValues(t, 0, listResp["count"])
	})

	t.Run("deactivating a collection un-assigns merch items — does not delete them", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		collectionID := colSeedCollection(t, pool, orgID, "Drop With Items")
		merchID := colSeedMerchInCollection(t, pool, eventID, collectionID, "Kit")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/organiser/collections/"+collectionID, nil, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// Merch item itself must still exist, just with collection_id = NULL
		var colID *string
		var isActive bool
		err := pool.QueryRow(context.Background(),
			`SELECT collection_id, is_active FROM merchandise WHERE id = $1`, merchID,
		).Scan(&colID, &isActive)
		require.NoError(t, err)
		assert.Nil(t, colID, "collection_id should be NULL after collection is deleted (ON DELETE SET NULL)")
		assert.True(t, isActive, "merch item itself should still be active")
	})

	t.Run("organiser cannot deactivate another organiser's collection — 404", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		_, otherToken := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Owner's Drop")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/organiser/collections/"+collectionID, nil, otherToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("non-existent collection returns 404", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, orgToken := colSeedUserWithRole(t, pool, "organizer")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/organiser/collections/00000000-0000-0000-0000-000000000000", nil, orgToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unauthenticated gets 401", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Some Drop")
		r := buildCollectionRouter(pool)

		w := doRequest(r, http.MethodDelete, "/organiser/collections/"+collectionID, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── PUT /organiser/merch/:id/collection ─────────────────────────────────────

func TestCollectionOrgAssignMerch(t *testing.T) {
	pool := setupCollectionTestDB(t)
	defer pool.Close()

	t.Run("organiser assigns merch item to their collection", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		collectionID := colSeedCollection(t, pool, orgID, "Target Collection")
		merchID := colSeedMerch(t, pool, eventID, "Unassigned Kit")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/"+merchID+"/collection",
			map[string]interface{}{"collection_id": collectionID}, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["message"], "assigned")

		// Verify the collection_id is set in the DB
		var gotCollectionID *string
		pool.QueryRow(context.Background(),
			`SELECT collection_id FROM merchandise WHERE id = $1`, merchID,
		).Scan(&gotCollectionID)
		require.NotNil(t, gotCollectionID)
		assert.Equal(t, collectionID, *gotCollectionID)
	})

	t.Run("organiser un-assigns merch from collection using null collection_id", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		collectionID := colSeedCollection(t, pool, orgID, "Drop")
		merchID := colSeedMerchInCollection(t, pool, eventID, collectionID, "Kit")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/"+merchID+"/collection",
			map[string]interface{}{"collection_id": nil}, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["message"], "removed")

		var gotCollectionID *string
		pool.QueryRow(context.Background(),
			`SELECT collection_id FROM merchandise WHERE id = $1`, merchID,
		).Scan(&gotCollectionID)
		assert.Nil(t, gotCollectionID, "collection_id should be NULL after un-assignment")
	})

	t.Run("assigning to a non-existent collection returns 404", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		merchID := colSeedMerch(t, pool, eventID, "Kit")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/"+merchID+"/collection",
			map[string]interface{}{"collection_id": "00000000-0000-0000-0000-000000000000"}, orgToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("assigning non-existent merch returns 404", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Drop")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/00000000-0000-0000-0000-000000000000/collection",
			map[string]interface{}{"collection_id": collectionID}, orgToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("organiser cannot assign merch from another organiser's event", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		otherID, otherToken := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID) // event owned by orgID
		collectionID := colSeedCollection(t, pool, otherID, "Other's Collection")
		merchID := colSeedMerch(t, pool, eventID, "Kit owned by org1")
		r := buildCollectionRouter(pool)

		// otherID tries to assign org1's merch to their own collection
		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/"+merchID+"/collection",
			map[string]interface{}{"collection_id": collectionID}, otherToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unauthenticated gets 401", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		collectionID := colSeedCollection(t, pool, orgID, "Drop")
		merchID := colSeedMerch(t, pool, eventID, "Kit")
		r := buildCollectionRouter(pool)

		w := doRequest(r, http.MethodPut, "/organiser/merch/"+merchID+"/collection",
			map[string]interface{}{"collection_id": collectionID})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── Admin collection CRUD ───────────────────────────────────────────────────

func TestCollectionAdminCRUD(t *testing.T) {
	pool := setupCollectionTestDB(t)
	defer pool.Close()

	t.Run("admin creates a collection with no organiser ownership", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, adminToken := colSeedUserWithRole(t, pool, "admin")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/admin/collections",
			map[string]interface{}{
				"name":        "Platform-wide Drop",
				"description": "Created by admin, no organiser",
			}, adminToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Platform-wide Drop", resp["name"])
		// organizer_id should be absent or nil for admin-created collections
		assert.Empty(t, resp["organizer_id"])
	})

	t.Run("admin get all returns both active and inactive collections", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, adminToken := colSeedUserWithRole(t, pool, "admin")
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")

		colSeedCollection(t, pool, orgID, "Active Drop")
		pool.Exec(context.Background(), `
			INSERT INTO merch_collections (name, is_active)
			VALUES ('Inactive Drop', FALSE)
		`)
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/admin/collections", nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// Admin sees both active and inactive
		assert.EqualValues(t, 2, resp["count"])
	})

	t.Run("admin updates any collection regardless of organiser ownership", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, adminToken := colSeedUserWithRole(t, pool, "admin")
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Organiser's Drop")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/admin/collections/"+collectionID,
			map[string]interface{}{"name": "Admin Renamed It"}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		collection := resp["collection"].(map[string]interface{})
		assert.Equal(t, "Admin Renamed It", collection["name"])
	})

	t.Run("admin deactivates any collection", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, adminToken := colSeedUserWithRole(t, pool, "admin")
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Drop To Kill")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/admin/collections/"+collectionID, nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// Confirm it is now inactive in the DB
		var isActive bool
		pool.QueryRow(context.Background(),
			`SELECT is_active FROM merch_collections WHERE id = $1`, collectionID,
		).Scan(&isActive)
		assert.False(t, isActive)
	})

	t.Run("non-admin organiser cannot hit admin endpoints — 403", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, orgToken := colSeedUserWithRole(t, pool, "organizer")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/admin/collections",
			map[string]interface{}{"name": "Sneaky Drop"}, orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated cannot hit admin endpoints — 401", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		r := buildCollectionRouter(pool)

		w := doRequest(r, http.MethodPost, "/admin/collections",
			map[string]interface{}{"name": "Anonymous Drop"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── PUT /admin/merch/:id/collection ─────────────────────────────────────────

func TestCollectionAdminAssignMerch(t *testing.T) {
	pool := setupCollectionTestDB(t)
	defer pool.Close()

	t.Run("admin assigns merch to any collection regardless of ownership", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, adminToken := colSeedUserWithRole(t, pool, "admin")
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		collectionID := colSeedCollection(t, pool, orgID, "Target")
		merchID := colSeedMerch(t, pool, eventID, "Unassigned Item")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/admin/merch/"+merchID+"/collection",
			map[string]interface{}{"collection_id": collectionID}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["message"], "assigned")
	})

	t.Run("admin un-assigns merch using null collection_id", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, adminToken := colSeedUserWithRole(t, pool, "admin")
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		collectionID := colSeedCollection(t, pool, orgID, "Drop")
		merchID := colSeedMerchInCollection(t, pool, eventID, collectionID, "Kit")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/admin/merch/"+merchID+"/collection",
			map[string]interface{}{"collection_id": nil}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["message"], "removed")
	})

	t.Run("admin assigning to a non-existent collection returns 404", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, adminToken := colSeedUserWithRole(t, pool, "admin")
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		merchID := colSeedMerch(t, pool, eventID, "Kit")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/admin/merch/"+merchID+"/collection",
			map[string]interface{}{"collection_id": "00000000-0000-0000-0000-000000000000"}, adminToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("admin assigning non-existent merch returns 404", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, adminToken := colSeedUserWithRole(t, pool, "admin")
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		collectionID := colSeedCollection(t, pool, orgID, "Drop")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/admin/merch/00000000-0000-0000-0000-000000000000/collection",
			map[string]interface{}{"collection_id": collectionID}, adminToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("non-admin gets 403", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		_, orgToken := colSeedUserWithRole(t, pool, "organizer")
		r := buildCollectionRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/admin/merch/00000000-0000-0000-0000-000000000000/collection",
			map[string]interface{}{"collection_id": nil}, orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

// ─── Integration: full collection lifecycle ───────────────────────────────────

func TestCollectionFullLifecycle(t *testing.T) {
	pool := setupCollectionTestDB(t)
	defer pool.Close()

	t.Run("create collection → assign merch → fetch publicly → deactivate → items un-assigned", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		r := buildCollectionRouter(pool)

		// Step 1: organiser creates a collection
		w := doAuthRequest(r, http.MethodPost, "/organiser/collections",
			map[string]interface{}{"name": "Lifecycle Drop"}, orgToken)
		require.Equal(t, http.StatusCreated, w.Code)
		var created map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
		collectionID := created["id"].(string)

		// Step 2: seed a merch item and assign it to the collection
		merchID := colSeedMerch(t, pool, eventID, "Lifecycle Kit")
		w = doAuthRequest(r, http.MethodPut, "/organiser/merch/"+merchID+"/collection",
			map[string]interface{}{"collection_id": collectionID}, orgToken)
		require.Equal(t, http.StatusOK, w.Code)

		// Step 3: public user fetches the collection — should see the item
		w = doRequest(r, http.MethodGet, "/collections/"+collectionID, nil)
		require.Equal(t, http.StatusOK, w.Code)
		var public map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &public))
		assert.Len(t, public["items"].([]interface{}), 1)
		assert.Equal(t, "Lifecycle Kit", public["items"].([]interface{})[0].(map[string]interface{})["name"])

		// Step 4: organiser deactivates the collection
		w = doAuthRequest(r, http.MethodDelete, "/organiser/collections/"+collectionID, nil, orgToken)
		require.Equal(t, http.StatusOK, w.Code)

		// Step 5: collection is gone from public endpoint
		w = doRequest(r, http.MethodGet, "/collections/"+collectionID, nil)
		assert.Equal(t, http.StatusNotFound, w.Code)

		// Step 6: merch item still exists, collection_id is now NULL
		var colID *string
		pool.QueryRow(context.Background(),
			`SELECT collection_id FROM merchandise WHERE id = $1`, merchID,
		).Scan(&colID)
		assert.Nil(t, colID, "merch item must still exist with collection_id = NULL")
	})

	t.Run("multiple merch items from same event in one collection all appear in public fetch", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, _ := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		collectionID := colSeedCollection(t, pool, orgID, "Full Kit Drop")
		r := buildCollectionRouter(pool)

		names := []string{"Home Kit", "Away Kit", "Training Kit", "Goalkeeper Kit"}
		for _, name := range names {
			colSeedMerchInCollection(t, pool, eventID, collectionID, name)
		}

		w := doRequest(r, http.MethodGet, "/collections/"+collectionID, nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Len(t, resp["items"].([]interface{}), 4)
	})

	t.Run("merch item can be re-assigned from one collection to another", func(t *testing.T) {
		cleanCollectionTables(t, pool)
		orgID, orgToken := colSeedUserWithRole(t, pool, "organizer")
		eventID := colSeedEvent(t, pool, orgID)
		colA := colSeedCollection(t, pool, orgID, "Collection A")
		colB := colSeedCollection(t, pool, orgID, "Collection B")
		merchID := colSeedMerchInCollection(t, pool, eventID, colA, "Transferable Kit")
		r := buildCollectionRouter(pool)

		// Re-assign from A to B
		w := doAuthRequest(r, http.MethodPut, "/organiser/merch/"+merchID+"/collection",
			map[string]interface{}{"collection_id": colB}, orgToken)
		require.Equal(t, http.StatusOK, w.Code)

		var gotColID *string
		pool.QueryRow(context.Background(),
			`SELECT collection_id FROM merchandise WHERE id = $1`, merchID,
		).Scan(&gotColID)
		require.NotNil(t, gotColID)
		assert.Equal(t, colB, *gotColID, "merch should now belong to collection B")

		// Collection A should now be empty
		w = doRequest(r, http.MethodGet, fmt.Sprintf("/collections/%s", colA), nil)
		require.Equal(t, http.StatusOK, w.Code)
		var respA map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &respA))
		if respA["items"] != nil {
			assert.Len(t, respA["items"].([]interface{}), 0)
		}
	})
}