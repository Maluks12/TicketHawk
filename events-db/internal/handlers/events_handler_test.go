package handlers_test

// =============================================================================
// EVENTS HANDLER TESTS
// =============================================================================
//
// Tests covered:
//   - POST /admin/events               → CreateEventHandlers    (admin only)
//   - GET  /admin/events/:id           → GetEventByIDHandlers   (admin only)
//   - PUT  /admin/events/:id           → UpdateEventHandlers    (admin only)
//   - DELETE /admin/events/:id         → DeleteEventHandlers    (admin only)
//   - POST /organiser/events/:id/publish → PublishEventHandler  (organiser)
//   - POST /organiser/events/:id/cancel  → CancelEventHandler   (organiser)
//   - GET  /events                     → GetAllEventsHandlers   (public)
//   - GET  /events/trending            → TrendingEventHandlers  (public)
//   - GET  /events/featured            → FeaturedEventsHandlers (public)
//   - GET  /events/active              → ActiveEventsHandlers   (public)
//   - GET  /events/search              → SearchEventsHandler    (public)
//   - GET  /events/category/:id        → GetEventCategory       (public)
//
// Prerequisites — start the test database once before running:
//
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser \
//     -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db \
//     -p 5433:5432 postgres:15
//
// Run all event tests:
//
//   go test ./internal/handlers/... -v -run TestEvent
//
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

func setupEventsHandlerTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Skipf("events DB tests skipped — could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("events DB tests skipped — DB not reachable: %v", pingErr)
	}
	createEventsHandlerTestTables(t, pool)
	cleanEventsHandlerTables(t, pool)
	return pool
}

func createEventsHandlerTestTables(t *testing.T, pool *pgxpool.Pool) {
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

		// Events table matches every column scanned by the repository functions.
		// VARCHAR(50) for status/role avoids any dependency on production enum types.
		`CREATE TABLE events (
			id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title                VARCHAR(255) NOT NULL,
			description          TEXT NOT NULL DEFAULT '',
			cover_image_url      TEXT,
			banner_image_url     TEXT,
			banner_video_url     TEXT,
			gallery_images       JSONB,
			start_time           TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
			end_time             TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '8 days',
			expires_at           TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
			timezone             VARCHAR(50) NOT NULL DEFAULT 'Africa/Johannesburg',
			venue_name           VARCHAR(255) NOT NULL DEFAULT '',
			address              TEXT NOT NULL DEFAULT '',
			city                 VARCHAR(255) NOT NULL DEFAULT '',
			latitude             DOUBLE PRECISION,
			longitude            DOUBLE PRECISION,
			category_id          INTEGER NOT NULL REFERENCES categories(id),
			price                DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			currency             VARCHAR(3) NOT NULL DEFAULT 'ZAR',
			max_capacity         INTEGER,
			tickets_sold         INTEGER NOT NULL DEFAULT 0,
			reserved_count       INTEGER NOT NULL DEFAULT 0,
			allow_waitlist       BOOLEAN DEFAULT TRUE,
			max_tickets_per_user INTEGER,
			venue_type           VARCHAR(20),
			status               VARCHAR(50) NOT NULL DEFAULT 'draft',
			is_featured          BOOLEAN DEFAULT FALSE,
			featured_until       TIMESTAMPTZ,
			organizer_id         UUID REFERENCES users(id),
			published_at         TIMESTAMPTZ,
			cancelled_at         TIMESTAMPTZ,
			cancellation_reason  TEXT,
			refund_policy        TEXT,
			terms_and_conditions TEXT,
			tags                 JSONB,
			views                BIGINT DEFAULT 0,
			search_vector        TSVECTOR,
			created_at           TIMESTAMPTZ DEFAULT NOW(),
			updated_at           TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Trigger keeps search_vector in sync with title + description automatically.
		// This means SearchEvents tests work without manual vector updates.
		`CREATE OR REPLACE FUNCTION events_tsv_update() RETURNS TRIGGER AS $$
		BEGIN
			NEW.search_vector :=
				to_tsvector('english',
					COALESCE(NEW.title, '') || ' ' || COALESCE(NEW.description, ''));
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,

		`CREATE TRIGGER events_tsv_trigger
		 BEFORE INSERT OR UPDATE ON events
		 FOR EACH ROW EXECUTE FUNCTION events_tsv_update()`,

		// Minimal bookings stub — DeleteEvents checks COUNT(*) and CancelEvent
		// updates booking_status. No other columns needed for events tests.
		`CREATE TABLE bookings (
			id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id             UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			booking_status       VARCHAR(50) NOT NULL DEFAULT 'pending',
			cancellation_reason  TEXT,
			refund_requested_at  TIMESTAMPTZ,
			refund_reason        TEXT,
			created_at           TIMESTAMPTZ DEFAULT NOW()
		)`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("events table setup failed:\n  sql: %s\n  err: %v", s, err)
		}
	}
}

func cleanEventsHandlerTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE bookings CASCADE")
	pool.Exec(ctx, "TRUNCATE events CASCADE")
	pool.Exec(ctx, "TRUNCATE categories CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens CASCADE")
	pool.Exec(ctx, "TRUNCATE users CASCADE")
}

// ─── seed helpers ─────────────────────────────────────────────────────────────

// eventsSeedUser creates a user with the given role and returns their ID and a
// fresh JWT obtained via the /auth/login endpoint.
func eventsSeedUser(t *testing.T, pool *pgxpool.Pool, role string) (userID, token string) {
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

func eventsSeedCategory(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO categories (name, slug)
		VALUES (
			'Cat-'||substr(gen_random_uuid()::text,1,8),
			'cat-'||substr(gen_random_uuid()::text,1,8)
		)
		RETURNING id
	`).Scan(&id)
	require.NoError(t, err)
	return id
}

// eventsSeedDraft directly inserts a draft event owned by orgID.
// Use this to set up state for CRUD and publish/cancel tests.
func eventsSeedDraft(t *testing.T, pool *pgxpool.Pool, orgID string, catID int, title string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, description, status, category_id, organizer_id, price, currency,
		                    venue_name, address, city, timezone,
		                    start_time, end_time, expires_at)
		VALUES ($1, 'A full event description that is long enough for testing.', 'draft', $2, $3,
		        500.00, 'ZAR', 'FNB Stadium', 'Nasrec Road', 'Johannesburg', 'Africa/Johannesburg',
		        NOW() + INTERVAL '7 days', NOW() + INTERVAL '8 days', NOW() + INTERVAL '30 days')
		RETURNING id
	`, title, catID, orgID).Scan(&id)
	require.NoError(t, err)
	return id
}

// eventsSeedPublished inserts a published event directly for use in listing tests.
func eventsSeedPublished(t *testing.T, pool *pgxpool.Pool, orgID string, catID int, title string, views int) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, description, status, category_id, organizer_id, price, currency,
		                    venue_name, address, city, timezone, views,
		                    start_time, end_time, expires_at)
		VALUES ($1, 'A published test event with enough detail for integration tests.', 'published', $2, $3,
		        500.00, 'ZAR', 'FNB Stadium', 'Nasrec Road', 'Johannesburg', 'Africa/Johannesburg', $4,
		        NOW() + INTERVAL '7 days', NOW() + INTERVAL '8 days', NOW() + INTERVAL '30 days')
		RETURNING id
	`, title, catID, orgID, views).Scan(&id)
	require.NoError(t, err)
	return id
}

// eventsSeedFeatured inserts a published featured event.
func eventsSeedFeatured(t *testing.T, pool *pgxpool.Pool, orgID string, catID int, title string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, description, status, is_featured, featured_until,
		                    category_id, organizer_id, price, currency,
		                    venue_name, address, city, timezone,
		                    start_time, end_time, expires_at)
		VALUES ($1, 'A featured event with enough description for integration tests.', 'published',
		        TRUE, NOW() + INTERVAL '14 days',
		        $2, $3, 500.00, 'ZAR', 'FNB Stadium', 'Nasrec Road', 'Johannesburg',
		        'Africa/Johannesburg',
		        NOW() + INTERVAL '7 days', NOW() + INTERVAL '8 days', NOW() + INTERVAL '30 days')
		RETURNING id
	`, title, catID, orgID).Scan(&id)
	require.NoError(t, err)
	return id
}

// validCreateEventBody returns a map with all required fields for CreateEventDTO.
// description must be ≥ 50 chars; title must be ≥ 5 chars.
func validCreateEventBody(catID int) map[string]interface{} {
	now := time.Now()
	return map[string]interface{}{
		"title":       "Grand Test Concert Event",
		"description": "This is a comprehensive test event description that meets the fifty character minimum.",
		"start_time":  now.Add(24 * time.Hour).Format(time.RFC3339),
		"end_time":    now.Add(48 * time.Hour).Format(time.RFC3339),
		"expires_at":  now.Add(72 * time.Hour).Format(time.RFC3339),
		"timezone":    "Africa/Johannesburg",
		"venue_name":  "FNB Stadium",
		"address":     "Nasrec Road, Soweto",
		"city":        "Johannesburg",
		"category_id": catID,
		"price":       500.00,
		"currency":    "ZAR",
	}
}

// ─── router builder ───────────────────────────────────────────────────────────

func buildEventsRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()
	r := gin.New()

	// Auth — needed to obtain tokens via login
	r.POST("/auth/login", handlers.LoginHandler(pool, cfg))

	// Public event listings — nil cache (no Redis in integration tests)
	public := r.Group("/events")
	{
		public.GET("", handlers.GetAllEventsHandler(pool, nil))
		public.GET("/trending", handlers.TrendingEventHandler(pool, nil))
		public.GET("/featured", handlers.FeaturedEventsHandler(pool, nil))
		public.GET("/active", handlers.ActiveEventsHandler(pool, nil))
		public.GET("/search", handlers.SearchEventsHandler(pool))
		public.GET("/category/:id", handlers.GetEventCategoryHandler(pool, nil))
	}

	// Admin routes — create, read, update, delete
	admin := r.Group("/admin")
	admin.Use(middleware.AuthMiddleware(cfg), middleware.AdminOnlyMiddleware())
	{
		admin.POST("/events", handlers.CreateEventHandler(pool, nil))
		admin.GET("/events/:id", handlers.GetEventByIDHandler(pool, nil))
		admin.PUT("/events/:id", handlers.UpdateEventHandler(pool, nil))
		admin.DELETE("/events/:id", handlers.DeleteEventHandler(pool, nil))
	}

	// Organiser routes — publish and cancel
	org := r.Group("/organiser")
	org.Use(middleware.AuthMiddleware(cfg), middleware.OrganizerOnlyMiddleware())
	{
		org.POST("/events/:id/publish", handlers.PublishEventHandler(pool, nil))
		org.POST("/events/:id/cancel", handlers.CancelEventHandler(pool, nil))
	}

	return r
}

// ─── POST /admin/events ───────────────────────────────────────────────────────

func TestEventCreate(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("admin creates event — returns 201 with draft status", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		_, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/admin/events", validCreateEventBody(catID), adminToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		event := resp["event"].(map[string]interface{})
		assert.Equal(t, "Grand Test Concert Event", event["title"])
		assert.Equal(t, "draft", event["status"])
		assert.NotEmpty(t, event["id"])
	})

	t.Run("title too short — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		_, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		r := buildEventsRouter(pool)

		body := validCreateEventBody(catID)
		body["title"] = "Hi" // only 2 chars, min is 5
		w := doAuthRequest(r, http.MethodPost, "/admin/events", body, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("description too short — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		_, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		r := buildEventsRouter(pool)

		body := validCreateEventBody(catID)
		body["description"] = "Too short." // under 50 chars
		w := doAuthRequest(r, http.MethodPost, "/admin/events", body, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("end_time before start_time — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		_, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		r := buildEventsRouter(pool)

		now := time.Now()
		body := validCreateEventBody(catID)
		body["start_time"] = now.Add(48 * time.Hour).Format(time.RFC3339)
		body["end_time"] = now.Add(24 * time.Hour).Format(time.RFC3339) // before start
		w := doAuthRequest(r, http.MethodPost, "/admin/events", body, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing required fields — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		_, adminToken := eventsSeedUser(t, pool, "admin")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/admin/events",
			map[string]interface{}{"title": "Only Title"}, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("organiser is not allowed on the admin route — 403", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		_, orgToken := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/admin/events", validCreateEventBody(catID), orgToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated — 401", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		catID := eventsSeedCategory(t, pool)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodPost, "/admin/events", validCreateEventBody(catID))
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── GET /admin/events/:id ────────────────────────────────────────────────────

func TestEventGetByID(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("admin can fetch a published event by ID", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedPublished(t, pool, orgID, catID, "Visible Event", 0)
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/admin/events/"+eventID, nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, eventID, resp["id"])
		assert.Equal(t, "Visible Event", resp["title"])
	})

	t.Run("admin can fetch their own draft event", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		draftID := eventsSeedDraft(t, pool, orgID, catID, "Draft Event")
		r := buildEventsRouter(pool)

		// GetEventByID shows drafts to the owner (user_id matches organizer_id)
		w := doAuthRequest(r, http.MethodGet, "/admin/events/"+draftID, nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "draft", resp["status"])
	})

	t.Run("invalid UUID format — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		_, adminToken := eventsSeedUser(t, pool, "admin")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/admin/events/not-a-uuid", nil, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("non-existent event — 404", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		_, adminToken := eventsSeedUser(t, pool, "admin")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/admin/events/00000000-0000-0000-0000-000000000000", nil, adminToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ─── PUT /admin/events/:id ────────────────────────────────────────────────────

func TestEventUpdate(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("owner updates title — 200", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Original Title")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/admin/events/"+eventID,
			map[string]interface{}{"title": "Updated Title"}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		event := resp["event"].(map[string]interface{})
		assert.Equal(t, "Updated Title", event["title"])
	})

	t.Run("update multiple fields at once — 200", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Multi Update Test")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/admin/events/"+eventID,
			map[string]interface{}{
				"venue_name": "Loftus Versfeld",
				"city":       "Pretoria",
				"price":      750.00,
			}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		event := resp["event"].(map[string]interface{})
		assert.Equal(t, "Loftus Versfeld", event["venue_name"])
		assert.Equal(t, "Pretoria", event["city"])
	})

	t.Run("set is_featured with featured_until — 200", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Feature Me")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPut, "/admin/events/"+eventID,
			map[string]interface{}{
				"is_featured":    true,
				"featured_until": time.Now().Add(120 * time.Hour).Format(time.RFC3339),
			}, adminToken)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("unauthenticated — 401", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Protected Event")
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodPut, "/admin/events/"+eventID,
			map[string]interface{}{"title": "Hacked"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── DELETE /admin/events/:id ─────────────────────────────────────────────────

func TestEventDelete(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("owner deletes their own draft — 200", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "To Be Deleted")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/admin/events/"+eventID, nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["message"], "deleted successfully")

		// Confirm it no longer exists
		w2 := doAuthRequest(r, http.MethodGet, "/admin/events/"+eventID, nil, adminToken)
		assert.Equal(t, http.StatusNotFound, w2.Code)
	})

	t.Run("cannot delete a published event — 404 (only drafts qualify)", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedPublished(t, pool, orgID, catID, "Live Event", 0)
		r := buildEventsRouter(pool)

		// DeleteEvents requires status = 'draft', so this returns ErrEventNotFound
		w := doAuthRequest(r, http.MethodDelete, "/admin/events/"+eventID, nil, adminToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("cannot delete event with existing bookings — 400/500", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, adminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Has Bookings")
		// Seed a booking against this event
		pool.Exec(context.Background(),
			`INSERT INTO bookings (event_id) VALUES ($1)`, eventID)
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/admin/events/"+eventID, nil, adminToken)
		// Handler returns 500 via utils.InternalError when delete fails due to booking constraint
		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("non-existent event — 404", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		_, adminToken := eventsSeedUser(t, pool, "admin")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/admin/events/00000000-0000-0000-0000-000000000000", nil, adminToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("different admin cannot delete another admin's draft — 404", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "admin")
		_, otherAdminToken := eventsSeedUser(t, pool, "admin")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Someone Else's Event")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodDelete, "/admin/events/"+eventID, nil, otherAdminToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ─── POST /organiser/events/:id/publish ──────────────────────────────────────

func TestEventPublish(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("organiser publishes their draft — 200", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, orgToken := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Ready to Publish")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/publish", nil, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["message"], "published successfully")

		// Event should now appear in public listings
		r2 := buildEventsRouter(pool)
		wList := doRequest(r2, http.MethodGet, "/events", nil)
		require.Equal(t, http.StatusOK, wList.Code)
		var listResp map[string]interface{}
		require.NoError(t, json.Unmarshal(wList.Body.Bytes(), &listResp))
		data := listResp["data"].([]interface{})
		require.Len(t, data, 1)
		assert.Equal(t, eventID, data[0].(map[string]interface{})["id"])
	})

	t.Run("publishing an already-published event — 400 (0 rows updated)", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, orgToken := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedPublished(t, pool, orgID, catID, "Already Live", 0)
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/publish", nil, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("organiser cannot publish another organiser's event — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		_, otherToken := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Not Mine")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/publish", nil, otherToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("attendee cannot publish — 403", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		_, attendeeToken := eventsSeedUser(t, pool, "attendee")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Private Draft")
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/publish", nil, attendeeToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unauthenticated — 401", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Anon Draft")
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/publish", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── POST /organiser/events/:id/cancel ───────────────────────────────────────

func TestEventCancel(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	cancelBody := map[string]interface{}{
		"reason": "Venue is no longer available for the scheduled date.",
	}

	t.Run("organiser cancels their published event — 200", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, orgToken := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedPublished(t, pool, orgID, catID, "Event To Cancel", 0)
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/cancel", cancelBody, orgToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["message"], "cancelled successfully")

		// Event must no longer appear in public listings
		wList := doRequest(r, http.MethodGet, "/events", nil)
		require.Equal(t, http.StatusOK, wList.Code)
		var listResp map[string]interface{}
		require.NoError(t, json.Unmarshal(wList.Body.Bytes(), &listResp))
		assert.EqualValues(t, 0, len(listResp["data"].([]interface{})))
	})

	t.Run("cannot cancel a draft event — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, orgToken := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedDraft(t, pool, orgID, catID, "Draft Not Published")
		r := buildEventsRouter(pool)

		// CancelEvent requires status = 'published'
		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/cancel", cancelBody, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing reason — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, orgToken := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedPublished(t, pool, orgID, catID, "Reason Missing", 0)
		r := buildEventsRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/cancel",
			map[string]interface{}{"reason": "short"}, orgToken) // min=10
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unauthenticated — 401", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventID := eventsSeedPublished(t, pool, orgID, catID, "Anon Cancel Try", 0)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/cancel", cancelBody)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── GET /events (GetAllEventsHandlers) ───────────────────────────────────────

func TestEventGetAll(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("returns only published events — drafts hidden", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventsSeedPublished(t, pool, orgID, catID, "Published A", 0)
		eventsSeedPublished(t, pool, orgID, catID, "Published B", 0)
		eventsSeedDraft(t, pool, orgID, catID, "Hidden Draft")
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 2, "only the 2 published events should appear")
	})

	t.Run("no events returns empty data", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotNil(t, resp["data"])
	})

	t.Run("response includes next_cursor field", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp, "next_cursor")
	})

	t.Run("invalid cursor — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events?cursor=not-valid-base64", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no auth required — public endpoint", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events", nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ─── GET /events/trending ─────────────────────────────────────────────────────

func TestEventTrending(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("returns events sorted by views DESC", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventsSeedPublished(t, pool, orgID, catID, "Low Views", 10)
		eventsSeedPublished(t, pool, orgID, catID, "High Views", 500)
		eventsSeedPublished(t, pool, orgID, catID, "Mid Views", 100)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/trending", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 3)

		first := data[0].(map[string]interface{})
		last := data[2].(map[string]interface{})
		assert.Equal(t, "High Views", first["title"])
		assert.Equal(t, "Low Views", last["title"])
	})

	t.Run("drafts are excluded from trending", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventsSeedPublished(t, pool, orgID, catID, "Published Trending", 200)
		eventsSeedDraft(t, pool, orgID, catID, "Draft High Views")
		// Manually set draft views high so it would appear if not filtered
		pool.Exec(context.Background(), `UPDATE events SET views = 9999 WHERE status = 'draft'`)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/trending", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)
		assert.Equal(t, "Published Trending", data[0].(map[string]interface{})["title"])
	})
}

// ─── GET /events/featured ─────────────────────────────────────────────────────

func TestEventFeatured(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("returns only featured published events", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventsSeedFeatured(t, pool, orgID, catID, "Featured One")
		eventsSeedFeatured(t, pool, orgID, catID, "Featured Two")
		eventsSeedPublished(t, pool, orgID, catID, "Regular Published", 0)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/featured", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 2, "only featured events should be returned")

		for _, item := range data {
			event := item.(map[string]interface{})
			assert.Equal(t, true, event["is_featured"])
		}
	})

	t.Run("no featured events returns empty slice", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventsSeedPublished(t, pool, orgID, catID, "Not Featured", 0)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/featured", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 0)
	})
}

// ─── GET /events/active ───────────────────────────────────────────────────────

func TestEventActive(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("returns events whose end_time is in the future", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)

		// Active event — end_time in future (default seeder puts it 8 days out)
		eventsSeedPublished(t, pool, orgID, catID, "Future Event", 0)

		// Past event — manually set end_time in the past
		pastID := eventsSeedPublished(t, pool, orgID, catID, "Past Event", 0)
		pool.Exec(context.Background(),
			`UPDATE events SET end_time = NOW() - INTERVAL '1 day' WHERE id = $1`, pastID)

		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/active", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		require.Len(t, data, 1)
		assert.Equal(t, "Future Event", data[0].(map[string]interface{})["title"])
	})
}

// ─── GET /events/search ───────────────────────────────────────────────────────

func TestEventSearch(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("finds published events matching query", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		// search_vector trigger fires on INSERT so no manual update needed
		eventsSeedPublished(t, pool, orgID, catID, "Coldplay Concert Johannesburg", 0)
		eventsSeedPublished(t, pool, orgID, catID, "Jazz Festival Cape Town", 0)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/search?q=Coldplay", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)
		assert.Equal(t, "Coldplay Concert Johannesburg", data[0].(map[string]interface{})["title"])
	})

	t.Run("empty query — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/search", nil) // no ?q=
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("query with no results returns empty data", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventsSeedPublished(t, pool, orgID, catID, "Rock Festival Durban", 0)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/search?q=xyznonexistent", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// data may be nil or empty slice — both are valid empty responses
		assert.NotContains(t, resp, "error")
	})

	t.Run("drafts are excluded from search results", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventsSeedPublished(t, pool, orgID, catID, "Public Marathon Event", 0)
		eventsSeedDraft(t, pool, orgID, catID, "Secret Marathon Draft")
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/search?q=Marathon", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)
		assert.Equal(t, "Public Marathon Event", data[0].(map[string]interface{})["title"])
	})
}

// ─── GET /events/category/:id ─────────────────────────────────────────────────

func TestEventCategory(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("returns published events for the given category only", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		musicCat := eventsSeedCategory(t, pool)
		sportsCat := eventsSeedCategory(t, pool)

		eventsSeedPublished(t, pool, orgID, musicCat, "Music Event 1", 0)
		eventsSeedPublished(t, pool, orgID, musicCat, "Music Event 2", 0)
		eventsSeedPublished(t, pool, orgID, sportsCat, "Sports Event", 0)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, fmt.Sprintf("/events/category/%d", musicCat), nil)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 2, "should return only music category events")
	})

	t.Run("unknown category returns empty slice", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/category/99999", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotNil(t, resp["data"])
	})

	t.Run("non-integer category ID — 400", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events/category/not-an-id", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("drafts are excluded even if they belong to the category", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		eventsSeedPublished(t, pool, orgID, catID, "Visible", 0)
		eventsSeedDraft(t, pool, orgID, catID, "Hidden Draft")
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, fmt.Sprintf("/events/category/%d", catID), nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)
		assert.Equal(t, "Visible", data[0].(map[string]interface{})["title"])
	})
}

// ─── Cursor pagination ────────────────────────────────────────────────────────

func TestEventPagination(t *testing.T) {
	pool := setupEventsHandlerTestDB(t)
	defer pool.Close()

	t.Run("cursor paginates through all published events without duplicates", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)

		const total = 25
		for i := 0; i < total; i++ {
			eventsSeedPublished(t, pool, orgID, catID, fmt.Sprintf("Paginated Event %02d", i), 0)
		}

		r := buildEventsRouter(pool)
		seenIDs := make(map[string]bool)
		cursor := ""
		pageCount := 0

		for {
			pageCount++
			url := "/events"
			if cursor != "" {
				url += "?cursor=" + cursor
			}

			w := doRequest(r, http.MethodGet, url, nil)
			require.Equal(t, http.StatusOK, w.Code, "page %d", pageCount)

			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			data := resp["data"].([]interface{})
			require.NotEmpty(t, data, "page %d should not be empty", pageCount)

			for _, item := range data {
				id := item.(map[string]interface{})["id"].(string)
				assert.False(t, seenIDs[id], "duplicate event %s on page %d", id, pageCount)
				seenIDs[id] = true
			}

			nextCursor, _ := resp["next_cursor"].(string)
			if nextCursor == "" {
				break
			}
			cursor = nextCursor

			require.Less(t, pageCount, 10, "infinite pagination loop detected")
		}

		assert.Equal(t, total, len(seenIDs), "should fetch all %d events across pages", total)
	})

	t.Run("limit query param is respected", func(t *testing.T) {
		cleanEventsHandlerTables(t, pool)
		orgID, _ := eventsSeedUser(t, pool, "organizer")
		catID := eventsSeedCategory(t, pool)
		for i := 0; i < 10; i++ {
			eventsSeedPublished(t, pool, orgID, catID, fmt.Sprintf("Limit Test %d", i), 0)
		}
		r := buildEventsRouter(pool)

		w := doRequest(r, http.MethodGet, "/events?limit=3", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 3)
		assert.NotEmpty(t, resp["next_cursor"])
	})
}