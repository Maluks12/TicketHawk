package handlers_test

// // =============================================================================
// TICKET TYPE FLOW TESTS
// =============================================================================
//
// Tests covered:
//   Organiser CRUD:
//   - POST /organiser/events/:id/ticket-types   → OrgCreateTicketTypesHandler
//   - GET  /organiser/events/:id/ticket-types   → OrgGetTicketTypesHandler
//   - PUT  /organiser/events/:id/ticket-types   → OrgReplaceTicketTypesHandler
//   - PATCH /organiser/ticket-types/:typeId     → OrgUpdateTicketTypeHandler
//   - DELETE /organiser/ticket-types/:typeId    → OrgDeleteTicketTypeHandler
//
//   Public:
//   - GET /events/:id/ticket-types              → GetTicketTypesHandler
//
//   Cart integration:
//   - POST /cart/items with ticket_type_id      → price resolved from type
//   - POST /cart/items without ticket_type_id   → rejected when event has types
//   - Sold-out type                             → 409
//
// Prerequisites:
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db -p 5433:5432 postgres:15
//
// Run:
//   go test ./internal/handlers/... -v -run TestTicketType
// =============================================================================

import (
	"bytes"
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

func setupTicketTypeTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Skipf("ticket type DB tests skipped — could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("ticket type DB tests skipped — DB not reachable: %v", pingErr)
	}
	createTicketTypeTestTables(t, pool)
	cleanTicketTypeTables(t, pool)
	return pool
}

func createTicketTypeTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Drop in dependency order
	drops := []string{
		"DROP TABLE IF EXISTS cart_items CASCADE",
		"DROP TABLE IF EXISTS carts CASCADE",
		"DROP TABLE IF EXISTS ticket_types CASCADE",
		"DROP TABLE IF EXISTS merchandise_variants CASCADE",
		"DROP TABLE IF EXISTS merchandise CASCADE",
		"DROP TABLE IF EXISTS events CASCADE",
		"DROP TABLE IF EXISTS categories CASCADE",
		"DROP TABLE IF EXISTS refresh_tokens CASCADE",
		"DROP TABLE IF EXISTS users CASCADE",
		"DROP TYPE  IF EXISTS cart_item_type",
	}
	for _, s := range drops {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		// cart_item_type enum
		`DO $$ BEGIN CREATE TYPE cart_item_type AS ENUM ('ticket','merchandise');
		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		// users — VARCHAR(50) for role avoids enum dependency; OrganizerOnlyMiddleware
		// casts to string before comparing so "organizer" value works correctly.
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

		// Events — includes all columns referenced by GetEventByID and
		// the ticket_types trigger (price column must exist for the sync trigger).
		`CREATE TABLE events (
			id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title                VARCHAR(255) NOT NULL DEFAULT 'Test Event',
			description          TEXT NOT NULL DEFAULT '',
			cover_image_url      TEXT,
			banner_image_url     TEXT,
			banner_video_url     TEXT,
			start_time           TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
			end_time             TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '8 days',
			expires_at           TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
			timezone             VARCHAR(50) NOT NULL DEFAULT 'Africa/Johannesburg',
			venue_name           VARCHAR(255) NOT NULL DEFAULT 'Test Venue',
			address              TEXT NOT NULL DEFAULT '1 Test St',
			city                 VARCHAR(255) NOT NULL DEFAULT 'Johannesburg',
			latitude             DOUBLE PRECISION,
			longitude            DOUBLE PRECISION,
			price                DECIMAL(10,2) NOT NULL DEFAULT 100.00,
			currency             VARCHAR(3) NOT NULL DEFAULT 'ZAR',
			max_capacity         INTEGER DEFAULT 200,
			tickets_sold         INTEGER NOT NULL DEFAULT 0,
			reserved_count       INTEGER NOT NULL DEFAULT 0,
			allow_waitlist       BOOLEAN DEFAULT TRUE,
			is_featured          BOOLEAN DEFAULT FALSE,
			featured_until       TIMESTAMPTZ,
			status               VARCHAR(50) NOT NULL DEFAULT 'published',
			refund_policy        TEXT,
			terms_and_conditions TEXT,
			tags                 JSONB DEFAULT '[]',
			performers           JSONB DEFAULT '[]',
			teams                JSONB DEFAULT '[]',
			views                BIGINT DEFAULT 0,
			organizer_id         UUID REFERENCES users(id),
			category_id          INTEGER NOT NULL REFERENCES categories(id),
			created_at           TIMESTAMPTZ DEFAULT NOW(),
			updated_at           TIMESTAMPTZ DEFAULT NOW()
		)`,

		// ticket_types — migration 025 columns (no trigger needed in tests;
		// the price-sync trigger is a production-DB concern).
		`CREATE TABLE ticket_types (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id       UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			name           VARCHAR(100) NOT NULL,
			description    TEXT,
			price          DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			capacity       INTEGER,
			tickets_sold   INTEGER NOT NULL DEFAULT 0,
			reserved_count INTEGER NOT NULL DEFAULT 0,
			sort_order     INTEGER NOT NULL DEFAULT 0,
			is_visible     BOOLEAN NOT NULL DEFAULT TRUE,
			sale_starts_at TIMESTAMPTZ,
			sale_ends_at   TIMESTAMPTZ,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT chk_tt_price_positive    CHECK (price >= 0),
			CONSTRAINT chk_tt_capacity_positive CHECK (capacity IS NULL OR capacity > 0),
			CONSTRAINT chk_tt_sold_valid        CHECK (tickets_sold >= 0),
			CONSTRAINT chk_tt_reserved_valid    CHECK (reserved_count >= 0)
		)`,

		`CREATE TABLE carts (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id          UUID REFERENCES users(id) ON DELETE CASCADE,
			session_id       UUID,
			last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT carts_owner_check CHECK (
				(user_id IS NOT NULL AND session_id IS NULL) OR
				(user_id IS NULL     AND session_id IS NOT NULL)
			)
		)`,

		// cart_items — includes ticket_type_id (migration 026) and
		// fulfillment columns (migration 027).
		`CREATE TABLE cart_items (
			id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			cart_id                UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
			item_type              cart_item_type NOT NULL,
			event_id               UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			merchandise_variant_id UUID,
			ticket_type_id         UUID REFERENCES ticket_types(id) ON DELETE SET NULL,
			quantity               INTEGER NOT NULL DEFAULT 1,
			unit_price_snapshot    NUMERIC(10,2) NOT NULL,
			fulfillment_method     VARCHAR(20),
			delivery_address       JSONB,
			delivery_fee_snapshot  NUMERIC(10,2) NOT NULL DEFAULT 0.00,
			created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT quantity_positive CHECK (quantity > 0)
		)`,
		`CREATE TABLE merchandise (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    price           DECIMAL(10,2) NOT NULL DEFAULT 0.00,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    pickup_location TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`,

// Required: GetCartWithItems LEFT JOINs merchandise_variants and selects
// mv.sku and mv.attributes::text — both must exist even with no rows.
`CREATE TABLE merchandise_variants (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchandise_id UUID NOT NULL REFERENCES merchandise(id) ON DELETE CASCADE,
    sku            VARCHAR(100),
    attributes     JSONB NOT NULL DEFAULT '{}',
    stock_count    INTEGER NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("ticket type table setup failed:\n  sql: %s\n  err: %v", s, err)
		}
	}
}

func cleanTicketTypeTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE cart_items CASCADE")
	pool.Exec(ctx, "TRUNCATE merchandise_variants CASCADE")
	pool.Exec(ctx, "TRUNCATE merchandise CASCADE")
	pool.Exec(ctx, "TRUNCATE carts CASCADE")
	pool.Exec(ctx, "TRUNCATE ticket_types CASCADE")
	pool.Exec(ctx, "TRUNCATE events CASCADE")
	pool.Exec(ctx, "TRUNCATE categories CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens CASCADE")
	pool.Exec(ctx, "TRUNCATE users CASCADE")
}

// ─── seed helpers ─────────────────────────────────────────────────────────────

func ttSeedUserWithRole(t *testing.T, pool *pgxpool.Pool, role string) (userID, token string) {
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

func ttSeedCategory(t *testing.T, pool *pgxpool.Pool) int {
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

func ttSeedEvent(t *testing.T, pool *pgxpool.Pool, organizerID string) string {
	t.Helper()
	catID := ttSeedCategory(t, pool)
	var eventID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, status, price, currency, category_id, organizer_id)
		VALUES ('Test Concert', 'published', 100.00, 'ZAR', $1, $2)
		RETURNING id
	`, catID, organizerID).Scan(&eventID)
	require.NoError(t, err)
	return eventID
}

// ttSeedTicketType inserts a ticket type directly into the DB.
// Use for tests that need a pre-existing type without going through the handler.
func ttSeedTicketType(t *testing.T, pool *pgxpool.Pool, eventID, name string, price float64, capacity *int) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO ticket_types (event_id, name, price, capacity, sort_order)
		VALUES ($1, $2, $3, $4, 0)
		RETURNING id
	`, eventID, name, price, capacity).Scan(&id)
	require.NoError(t, err)
	return id
}

// ttSeedSoldOutTicketType seeds a ticket type that is fully sold out.
func ttSeedSoldOutTicketType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
	t.Helper()
	cap := 10
	typeID := ttSeedTicketType(t, pool, eventID, "Sold Out Tier", 150.00, &cap)
	// Mark it as fully sold
	_, err := pool.Exec(context.Background(),
		`UPDATE ticket_types SET tickets_sold = 10 WHERE id = $1`, typeID)
	require.NoError(t, err)
	return typeID
}

// ─── router builder ───────────────────────────────────────────────────────────

func buildTicketTypeRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()
	r := gin.New()

	auth := r.Group("/auth")
	auth.POST("/login", handlers.LoginHandler(pool, cfg))

	// Public routes
	events := r.Group("/events")
	{
		events.GET("/:id/ticket-types", handlers.GetTicketTypesHandler(pool))
	}

	// Cart routes (optional auth — covers both authenticated and session flows)
	cart := r.Group("/cart")
	cart.Use(middleware.OptionalAuth(cfg))
	{
		cart.GET("", handlers.GetCartHandler(pool))
		cart.POST("/items", handlers.AddCartItemHandler(pool))
	}

	// Organiser routes
	org := r.Group("/organiser")
	org.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	// {
	// 	org.GET("/events/:id/ticket-types", handlers.OrgGetTicketTypesHandler(pool))
	// 	org.POST("/events/:id/ticket-types", handlers.OrgCreateTicketTypesHandler(pool))
	// 	org.PUT("/events/:id/ticket-types", handlers.OrgReplaceTicketTypesHandler(pool))
	// 	org.PATCH("/ticket-types/:typeId", handlers.OrgUpdateTicketTypeHandler(pool))
	// 	org.DELETE("/ticket-types/:typeId", handlers.OrgDeleteTicketTypeHandler(pool))
	// }

	return r
}

// ─── Organiser — Create ───────────────────────────────────────────────────────

func TestTicketTypeOrgCreate(t *testing.T) {
	pool := setupTicketTypeTestDB(t)
	defer pool.Close()

	t.Run("organiser creates ticket types for their event", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/ticket-types",
			map[string]interface{}{
				"types": []map[string]interface{}{
					{"name": "General Admission", "price": 150.00, "capacity": 500},
					{"name": "VIP Pit", "price": 450.00, "capacity": 50, "sort_order": 1},
					{"name": "Early Bird", "price": 99.00, "capacity": 100, "sort_order": 2},
				},
			}, token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		types := resp["ticket_types"].([]interface{})
		assert.Len(t, types, 3)

		// Verify names are present
		names := make(map[string]bool)
		for _, tt := range types {
			names[tt.(map[string]interface{})["name"].(string)] = true
		}
		assert.True(t, names["General Admission"])
		assert.True(t, names["VIP Pit"])
		assert.True(t, names["Early Bird"])
	})

	t.Run("non-organiser (attendee) is rejected with 403", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, _ := ttSeedUserWithRole(t, pool, "organizer")
		_, attendeeToken := ttSeedUserWithRole(t, pool, "attendee")
		eventID := ttSeedEvent(t, pool, orgID)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/ticket-types",
			map[string]interface{}{
				"types": []map[string]interface{}{
					{"name": "GA", "price": 100.00},
				},
			}, attendeeToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, _ := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		r := buildTicketTypeRouter(pool)

		w := doRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/ticket-types",
			map[string]interface{}{
				"types": []map[string]interface{}{{"name": "GA", "price": 100.00}},
			})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("organiser cannot create types for another organiser's event", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		ownerID, _ := ttSeedUserWithRole(t, pool, "organizer")
		_, otherToken := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, ownerID)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/ticket-types",
			map[string]interface{}{
				"types": []map[string]interface{}{{"name": "GA", "price": 100.00}},
			}, otherToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("empty types array returns 400 validation error", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/ticket-types",
			map[string]interface{}{"types": []interface{}{}}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── Public — GET /events/:id/ticket-types ───────────────────────────────────

func TestTicketTypePublicGet(t *testing.T) {
	pool := setupTicketTypeTestDB(t)
	defer pool.Close()

	t.Run("returns visible on-sale types for a public event", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, _ := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		ttSeedTicketType(t, pool, eventID, "GA", 150.00, nil)
		ttSeedTicketType(t, pool, eventID, "VIP", 450.00, nil)
		// Hidden type — should NOT appear in public response
		_, err := pool.Exec(context.Background(), `
			INSERT INTO ticket_types (event_id, name, price, is_visible)
			VALUES ($1, 'Staff Comp', 0.00, FALSE)
		`, eventID)
		require.NoError(t, err)
		r := buildTicketTypeRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/ticket-types", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		types := resp["ticket_types"].([]interface{})
		assert.Len(t, types, 2, "only visible types should be returned")
	})

	t.Run("returns empty list when event has no types (flat-priced)", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, _ := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		r := buildTicketTypeRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/ticket-types", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		types := resp["ticket_types"].([]interface{})
		assert.Len(t, types, 0)
	})

	t.Run("does not return types whose sale window has ended", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, _ := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)

		// Active type
		ttSeedTicketType(t, pool, eventID, "GA", 150.00, nil)
		// Expired type — sale_ends_at in the past
		past := time.Now().Add(-1 * time.Hour)
		_, err := pool.Exec(context.Background(), `
			INSERT INTO ticket_types (event_id, name, price, sale_ends_at)
			VALUES ($1, 'Expired Early Bird', 99.00, $2)
		`, eventID, past)
		require.NoError(t, err)
		r := buildTicketTypeRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/"+eventID+"/ticket-types", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		types := resp["ticket_types"].([]interface{})
		assert.Len(t, types, 1, "expired type should be filtered out")
		assert.Equal(t, "GA", types[0].(map[string]interface{})["name"])
	})
}

// ─── Organiser — GET (all types including hidden) ─────────────────────────────

func TestTicketTypeOrgGetAll(t *testing.T) {
	pool := setupTicketTypeTestDB(t)
	defer pool.Close()

	t.Run("organiser sees all types including hidden ones", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		ttSeedTicketType(t, pool, eventID, "GA", 150.00, nil)
		_, err := pool.Exec(context.Background(), `
			INSERT INTO ticket_types (event_id, name, price, is_visible)
			VALUES ($1, 'Staff Comp', 0.00, FALSE)
		`, eventID)
		require.NoError(t, err)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/organiser/events/"+eventID+"/ticket-types", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		types := resp["ticket_types"].([]interface{})
		assert.Len(t, types, 2, "organiser should see all types including hidden")
	})
}

// ─── Organiser — Update (PATCH) ───────────────────────────────────────────────

func TestTicketTypeOrgUpdate(t *testing.T) {
	pool := setupTicketTypeTestDB(t)
	defer pool.Close()

	t.Run("organiser can update price and name of a type", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		typeID := ttSeedTicketType(t, pool, eventID, "GA", 150.00, nil)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPatch, "/organiser/ticket-types/"+typeID,
			map[string]interface{}{
				"name":  "General Admission (Updated)",
				"price": 175.00,
			}, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		tt := resp["ticket_type"].(map[string]interface{})
		assert.Equal(t, "General Admission (Updated)", tt["name"])
		assert.EqualValues(t, 175.00, tt["price"])
	})

	t.Run("organiser can hide a type by setting is_visible=false", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		typeID := ttSeedTicketType(t, pool, eventID, "Early Bird", 99.00, nil)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPatch, "/organiser/ticket-types/"+typeID,
			map[string]interface{}{"is_visible": false}, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// Verify public GET no longer shows it
		getW := doRequest(r, http.MethodGet, "/events/"+eventID+"/ticket-types", nil)
		var getResp map[string]interface{}
		require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &getResp))
		assert.Len(t, getResp["ticket_types"].([]interface{}), 0)
	})

	t.Run("another organiser cannot update a type they don't own", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		ownerID, _ := ttSeedUserWithRole(t, pool, "organizer")
		_, otherToken := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, ownerID)
		typeID := ttSeedTicketType(t, pool, eventID, "GA", 150.00, nil)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPatch, "/organiser/ticket-types/"+typeID,
			map[string]interface{}{"price": 1.00}, otherToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("updating non-existent type returns 404", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		_, token := ttSeedUserWithRole(t, pool, "organizer")
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPatch, "/organiser/ticket-types/00000000-0000-0000-0000-000000000000",
			map[string]interface{}{"price": 100.00}, token)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ─── Organiser — Replace (PUT) ────────────────────────────────────────────────

func TestTicketTypeOrgReplace(t *testing.T) {
	pool := setupTicketTypeTestDB(t)
	defer pool.Close()

	t.Run("replaces all types with a new set when no sales exist", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		// Create initial types
		ttSeedTicketType(t, pool, eventID, "Old GA", 100.00, nil)
		ttSeedTicketType(t, pool, eventID, "Old VIP", 300.00, nil)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/events/"+eventID+"/ticket-types",
			map[string]interface{}{
				"types": []map[string]interface{}{
					{"name": "New Standard", "price": 120.00},
					{"name": "New VIP", "price": 500.00, "capacity": 20},
				},
			}, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		types := resp["ticket_types"].([]interface{})
		assert.Len(t, types, 2)

		// Old type names must be gone
		names := make(map[string]bool)
		for _, tt := range types {
			names[tt.(map[string]interface{})["name"].(string)] = true
		}
		assert.False(t, names["Old GA"])
		assert.True(t, names["New Standard"])
	})

	t.Run("replace is blocked when any type has existing sales", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		typeID := ttSeedTicketType(t, pool, eventID, "GA", 100.00, nil)

		// Simulate a ticket having been sold
		pool.Exec(context.Background(),
			`UPDATE ticket_types SET tickets_sold = 1 WHERE id = $1`, typeID)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/organiser/events/"+eventID+"/ticket-types",
			map[string]interface{}{
				"types": []map[string]interface{}{{"name": "New Type", "price": 200.00}},
			}, token)
		assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	})
}

// ─── Organiser — Delete ───────────────────────────────────────────────────────

func TestTicketTypeOrgDelete(t *testing.T) {
	pool := setupTicketTypeTestDB(t)
	defer pool.Close()

	t.Run("organiser can delete a type with no sales", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		typeID := ttSeedTicketType(t, pool, eventID, "GA", 150.00, nil)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/organiser/ticket-types/"+typeID, nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// Verify it's gone
		var count int
		pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM ticket_types WHERE id = $1`, typeID).Scan(&count)
		assert.Equal(t, 0, count)
	})

	t.Run("delete is blocked when type has existing bookings (tickets_sold > 0)", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		typeID := ttSeedTicketType(t, pool, eventID, "VIP", 450.00, nil)
		pool.Exec(context.Background(),
			`UPDATE ticket_types SET tickets_sold = 5 WHERE id = $1`, typeID)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/organiser/ticket-types/"+typeID, nil, token)
		assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	})

	t.Run("another organiser cannot delete a type they don't own", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		ownerID, _ := ttSeedUserWithRole(t, pool, "organizer")
		_, otherToken := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, ownerID)
		typeID := ttSeedTicketType(t, pool, eventID, "GA", 150.00, nil)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/organiser/ticket-types/"+typeID, nil, otherToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

// ─── Cart integration ─────────────────────────────────────────────────────────

func TestTicketTypeCartIntegration(t *testing.T) {
	pool := setupTicketTypeTestDB(t)
	defer pool.Close()

	t.Run("price is resolved from ticket_type not from events.price", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		// Event price is 100.00 but the VIP type is 450.00
		typeID := ttSeedTicketType(t, pool, eventID, "VIP Pit", 450.00, nil)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type":      "ticket",
			"event_id":       eventID,
			"ticket_type_id": typeID,
			"quantity":       2,
		}, token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// Price must be from the ticket type (450.00) not the event (100.00)
		assert.EqualValues(t, 450.00, resp["unit_price_snapshot"],
			"unit price should be from the ticket type, not events.price")
		assert.Equal(t, typeID, resp["ticket_type_id"])
	})

	t.Run("ticket_type_id from a different event is rejected", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		event1ID := ttSeedEvent(t, pool, orgID)
		event2ID := ttSeedEvent(t, pool, orgID)
		typeFromEvent2 := ttSeedTicketType(t, pool, event2ID, "GA", 150.00, nil)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type":      "ticket",
			"event_id":       event1ID,
			"ticket_type_id": typeFromEvent2,
			"quantity":       1,
		}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("adding a sold-out ticket type returns 409", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		soldOutTypeID := ttSeedSoldOutTicketType(t, pool, eventID)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type":      "ticket",
			"event_id":       eventID,
			"ticket_type_id": soldOutTypeID,
			"quantity":       1,
		}, token)
		assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	})

	t.Run("ticket type with expired sale window is rejected", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)

		// Insert type with sale_ends_at in the past
		past := time.Now().Add(-1 * time.Hour)
		var expiredTypeID string
		pool.QueryRow(context.Background(), `
			INSERT INTO ticket_types (event_id, name, price, sale_ends_at)
			VALUES ($1, 'Expired Early Bird', 99.00, $2)
			RETURNING id
		`, eventID, past).Scan(&expiredTypeID)

		r := buildTicketTypeRouter(pool)
		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type":      "ticket",
			"event_id":       eventID,
			"ticket_type_id": expiredTypeID,
			"quantity":       1,
		}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("cart total reflects ticket type price × quantity", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		gaTypeID := ttSeedTicketType(t, pool, eventID, "GA", 150.00, nil)
		vipTypeID := ttSeedTicketType(t, pool, eventID, "VIP", 450.00, nil)
		r := buildTicketTypeRouter(pool)

		// Add 2× GA (300) + 1× VIP (450) = 750
		doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket", "event_id": eventID,
			"ticket_type_id": gaTypeID, "quantity": 2,
		}, token)
		doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket", "event_id": eventID,
			"ticket_type_id": vipTypeID, "quantity": 1,
		}, token)

		w := doAuthRequest(r, http.MethodGet, "/cart", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.EqualValues(t, 750.00, resp["total_amount"])
	})

	t.Run("non-existent ticket_type_id returns 404", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, token := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		r := buildTicketTypeRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type":      "ticket",
			"event_id":       eventID,
			"ticket_type_id": "00000000-0000-0000-0000-000000000099",
			"quantity":       1,
		}, token)
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	})

	t.Run("guest can add ticket with ticket_type_id using session ID", func(t *testing.T) {
		cleanTicketTypeTables(t, pool)
		orgID, _ := ttSeedUserWithRole(t, pool, "organizer")
		eventID := ttSeedEvent(t, pool, orgID)
		typeID := ttSeedTicketType(t, pool, eventID, "GA", 150.00, nil)
		r := buildTicketTypeRouter(pool)

		body, _ := json.Marshal(map[string]interface{}{
			"item_type": "ticket", "event_id": eventID,
			"ticket_type_id": typeID, "quantity": 1,
		})
		req := httptest.NewRequest(http.MethodPost, "/cart/items", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Session-ID", fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", time.Now().UnixNano()%999999999999))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.EqualValues(t, 150.00, resp["unit_price_snapshot"])
	})
}