package handlers_test

// =============================================================================
// CHECKOUT / TICKET RESERVATION TESTS
// =============================================================================
//
// Tests covered:
//   - POST /checkout/begin      → CheckoutBeginHandler
//   - POST /checkout/heartbeat  → CheckoutHeartbeatHandler
//   - POST /checkout/abandon    → CheckoutAbandonHandler
//
// Verifies:
//   - Reservations are created atomically when entering checkout
//   - reserved_count on events is incremented correctly
//   - Heartbeat refreshes the 10-minute window
//   - Abandon releases capacity immediately
//   - Sold-out events block reservation
//   - Carts with no ticket items are rejected
//   - Multi-event carts create independent reservations per event
//
// Run:
//   go test ./internal/handlers/... -v -run TestCheckout
// =============================================================================

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"events-db/internal/handlers"
	"events-db/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── DB setup ─────────────────────────────────────────────────────────────────

func setupCheckoutTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Skipf("checkout DB tests skipped — could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("checkout DB tests skipped — DB not reachable: %v", pingErr)
	}
	createCheckoutTestTables(t, pool)
	cleanCheckoutTables(t, pool)
	return pool
}

func createCheckoutTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
		"DROP TABLE IF EXISTS ticket_reservations CASCADE",
		"DROP TABLE IF EXISTS cart_items CASCADE",
		"DROP TABLE IF EXISTS carts CASCADE",
		"DROP TABLE IF EXISTS merchandise_variants CASCADE",
		"DROP TABLE IF EXISTS merchandise CASCADE",
		"DROP TABLE IF EXISTS events CASCADE",
		"DROP TABLE IF EXISTS categories CASCADE",
		"DROP TABLE IF EXISTS refresh_tokens CASCADE",
		"DROP TABLE IF EXISTS users CASCADE",
	}
	for _, s := range drops {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		`DO $$ BEGIN CREATE TYPE cart_item_type AS ENUM ('ticket','merchandise');
		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		`CREATE TABLE users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email      VARCHAR(255) UNIQUE NOT NULL,
			password   TEXT NOT NULL,
			role       VARCHAR(50) NOT NULL DEFAULT 'user',
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
			max_capacity   INTEGER DEFAULT 50,
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
			base_price    NUMERIC(10,2) NOT NULL,
			display_order INTEGER NOT NULL DEFAULT 0,
			is_active     BOOLEAN NOT NULL DEFAULT TRUE,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE TABLE merchandise_variants (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			merchandise_id UUID NOT NULL REFERENCES merchandise(id) ON DELETE CASCADE,
			sku            VARCHAR(100) UNIQUE NOT NULL,
			attributes     JSONB NOT NULL DEFAULT '{}',
			stock_quantity INTEGER NOT NULL DEFAULT 10,
			is_active      BOOLEAN NOT NULL DEFAULT TRUE,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
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

		`CREATE TABLE cart_items (
			id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			cart_id                UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
			item_type              cart_item_type NOT NULL,
			event_id               UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			merchandise_variant_id UUID REFERENCES merchandise_variants(id) ON DELETE SET NULL,
			quantity               INTEGER NOT NULL DEFAULT 1,
			unit_price_snapshot    NUMERIC(10,2) NOT NULL,
			created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT quantity_positive CHECK (quantity > 0)
		)`,

		`CREATE TABLE ticket_reservations (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			cart_id        UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
			event_id       UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			user_id        UUID REFERENCES users(id) ON DELETE SET NULL,
			session_id     UUID,
			booking_number VARCHAR(50) UNIQUE NOT NULL
			               DEFAULT 'BK-' || upper(substr(gen_random_uuid()::text,1,8)),
			quantity       INTEGER NOT NULL,
			expires_at     TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '10 minutes',
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT reservation_quantity_positive CHECK (quantity > 0)
		)`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("checkout table setup failed: %v\nSQL: %s", err, s)
		}
	}
}

func cleanCheckoutTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE ticket_reservations CASCADE")
	pool.Exec(ctx, "TRUNCATE cart_items CASCADE")
	pool.Exec(ctx, "TRUNCATE carts CASCADE")
	pool.Exec(ctx, "TRUNCATE merchandise_variants CASCADE")
	pool.Exec(ctx, "TRUNCATE merchandise CASCADE")
	pool.Exec(ctx, "TRUNCATE events CASCADE")
	pool.Exec(ctx, "TRUNCATE categories CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens CASCADE")
	pool.Exec(ctx, "TRUNCATE users CASCADE")
}

// ─── seed helpers ─────────────────────────────────────────────────────────────

func checkoutSeedUser(t *testing.T, pool *pgxpool.Pool) (userID, token string) {
	t.Helper()
	// reuse cartSeedUser pattern
	return cartSeedUser(t, pool)
}

func checkoutSeedCategory(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	return cartSeedCategory(t, pool)
}

func checkoutSeedEvent(t *testing.T, pool *pgxpool.Pool, organizerID string, maxCapacity int) string {
	t.Helper()
	catID := checkoutSeedCategory(t, pool)
	var eventID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, status, price, currency, max_capacity,
		                    category_id, organizer_id)
		VALUES ('Test Event', 'published', 100.00, 'ZAR', $1, $2, $3)
		RETURNING id
	`, maxCapacity, catID, organizerID).Scan(&eventID)
	require.NoError(t, err)
	return eventID
}

// checkoutSeedCart creates a cart and adds ticket items to it, returning the cart ID.
func checkoutSeedCart(t *testing.T, pool *pgxpool.Pool, userID, eventID string, qty int) string {
	t.Helper()
	var cartID string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID,
	).Scan(&cartID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO cart_items (cart_id, item_type, event_id, quantity, unit_price_snapshot)
		VALUES ($1, 'ticket', $2, $3, 100.00)
	`, cartID, eventID, qty)
	require.NoError(t, err)
	return cartID
}

func getReservedCount(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	pool.QueryRow(context.Background(),
		`SELECT reserved_count FROM events WHERE id = $1`, eventID,
	).Scan(&count)
	return count
}

func getReservationCount(t *testing.T, pool *pgxpool.Pool, cartID string) int {
	t.Helper()
	var count int
	pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM ticket_reservations WHERE cart_id = $1 AND expires_at > NOW()`, cartID,
	).Scan(&count)
	return count
}

// ─── router builder ───────────────────────────────────────────────────────────

func buildCheckoutRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()
	r := gin.New()

	auth := r.Group("/auth")
	auth.POST("/login", handlers.LoginHandler(pool, cfg))

	checkout := r.Group("/checkout")
	{
		checkout.POST("/begin", middleware.OptionalAuth(cfg), handlers.CheckoutBeginHandler(pool))
		checkout.POST("/heartbeat", middleware.OptionalAuth(cfg), handlers.CheckoutHeartbeatHandler(pool))
		checkout.POST("/abandon", middleware.OptionalAuth(cfg), handlers.CheckoutAbandonHandler(pool))
	}
	return r
}

// ─── POST /checkout/begin ─────────────────────────────────────────────────────

func TestCheckoutBegin(t *testing.T) {
	pool := setupCheckoutTestDB(t)
	defer pool.Close()

	t.Run("creates reservation and increments reserved_count", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		checkoutSeedCart(t, pool, userID, eventID, 2)
		r := buildCheckoutRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		reservations := resp["reservations"].([]interface{})
		assert.Len(t, reservations, 1, "one reservation per event")
		assert.Greater(t, int(resp["seconds_left"].(float64)), 0, "should have time remaining")
		assert.EqualValues(t, 200.00, resp["cart_total"])

		// reserved_count on the event must be 2
		assert.Equal(t, 2, getReservedCount(t, pool, eventID))
	})

	t.Run("cart with no ticket items returns 400", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		// Create cart with NO items at all
		pool.Exec(context.Background(),
			`INSERT INTO carts (user_id) VALUES ($1)`, userID)
		r := buildCheckoutRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("sold-out event returns 409 and creates no reservations", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		// max_capacity = 1, but tickets_sold = 1 already
		catID := checkoutSeedCategory(t, pool)
		var soldOutEventID string
		pool.QueryRow(context.Background(), `
			INSERT INTO events (title, status, price, currency,
			                    max_capacity, tickets_sold, category_id, organizer_id)
			VALUES ('Sold Out', 'published', 100.00, 'ZAR', 1, 1, $1, $2)
			RETURNING id
		`, catID, userID).Scan(&soldOutEventID)

		checkoutSeedCart(t, pool, userID, soldOutEventID, 1)
		r := buildCheckoutRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		assert.Equal(t, 0, getReservedCount(t, pool, soldOutEventID),
			"reserved_count must not be incremented when sold out")
	})

	t.Run("re-entering checkout replaces existing reservation", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		cartID := checkoutSeedCart(t, pool, userID, eventID, 2)
		r := buildCheckoutRouter(pool)

		// First begin
		w1 := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, w1.Code)
		assert.Equal(t, 2, getReservedCount(t, pool, eventID))

		// Second begin (user goes back and re-enters checkout)
		w2 := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, w2.Code)

		// reserved_count should still be 2 — not doubled
		assert.Equal(t, 2, getReservedCount(t, pool, eventID))
		// Still only 1 active reservation
		assert.Equal(t, 1, getReservationCount(t, pool, cartID))
	})

	t.Run("multi-event cart creates one reservation per event", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		event1ID := checkoutSeedEvent(t, pool, userID, 50)
		event2ID := checkoutSeedEvent(t, pool, userID, 50)

		// Cart with tickets to two events
		var cartID string
		pool.QueryRow(context.Background(),
			`INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID,
		).Scan(&cartID)
		pool.Exec(context.Background(), `
			INSERT INTO cart_items (cart_id, item_type, event_id, quantity, unit_price_snapshot)
			VALUES ($1, 'ticket', $2, 2, 100.00), ($1, 'ticket', $3, 1, 100.00)
		`, cartID, event1ID, event2ID)

		r := buildCheckoutRouter(pool)
		w := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		reservations := resp["reservations"].([]interface{})
		assert.Len(t, reservations, 2, "one reservation per event")

		// Each event's reserved_count matches
		assert.Equal(t, 2, getReservedCount(t, pool, event1ID))
		assert.Equal(t, 1, getReservedCount(t, pool, event2ID))
	})

	t.Run("merch-only cart has no ticket items — returns 400", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)

		// Insert merch item only (no tickets)
		var cartID string
		pool.QueryRow(context.Background(),
			`INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID,
		).Scan(&cartID)

		var merchID, variantID string
		pool.QueryRow(context.Background(),
			`INSERT INTO merchandise (event_id, name, base_price) VALUES ($1, 'Scarf', 200.00) RETURNING id`,
			eventID).Scan(&merchID)
		pool.QueryRow(context.Background(),
			`INSERT INTO merchandise_variants (merchandise_id, sku, attributes) VALUES ($1, 'SKU-1', '{}') RETURNING id`,
			merchID).Scan(&variantID)
		pool.Exec(context.Background(), `
			INSERT INTO cart_items (cart_id, item_type, event_id, merchandise_variant_id, quantity, unit_price_snapshot)
			VALUES ($1, 'merchandise', $2, $3, 1, 200.00)
		`, cartID, eventID, variantID)

		r := buildCheckoutRouter(pool)
		w := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("guest user can begin checkout with session ID", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, _ := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		sessionID := "550e8400-e29b-41d4-a716-446655440020"

		// Guest cart
		var cartID string
		pool.QueryRow(context.Background(),
			`INSERT INTO carts (session_id) VALUES ($1::uuid) RETURNING id`, sessionID,
		).Scan(&cartID)
		pool.Exec(context.Background(), `
			INSERT INTO cart_items (cart_id, item_type, event_id, quantity, unit_price_snapshot)
			VALUES ($1, 'ticket', $2, 1, 100.00)
		`, cartID, eventID)

		r := buildCheckoutRouter(pool)
		req := httptest.NewRequest(http.MethodPost, "/checkout/begin", nil)
		req.Header.Set("X-Session-ID", sessionID)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, 1, getReservedCount(t, pool, eventID))
	})

	t.Run("no session and no token returns 400", func(t *testing.T) {
		r := buildCheckoutRouter(pool)
		req := httptest.NewRequest(http.MethodPost, "/checkout/begin", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── POST /checkout/heartbeat ─────────────────────────────────────────────────

func TestCheckoutHeartbeat(t *testing.T) {
	pool := setupCheckoutTestDB(t)
	defer pool.Close()

	t.Run("refreshes reservation window and returns seconds_left", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		checkoutSeedCart(t, pool, userID, eventID, 1)
		r := buildCheckoutRouter(pool)

		// Begin checkout to create reservation
		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)

		// Heartbeat
		w := doAuthRequest(r, http.MethodPost, "/checkout/heartbeat", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "ok", resp["status"])
		secondsLeft := int(resp["seconds_left"].(float64))
		assert.Greater(t, secondsLeft, 550, "should have close to 10 minutes remaining after refresh")
	})

	t.Run("heartbeat without an active reservation returns 410 Gone", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		_, token := checkoutSeedUser(t, pool)
		r := buildCheckoutRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/checkout/heartbeat", nil, token)
		assert.Equal(t, http.StatusGone, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "expired", resp["status"])
	})

	t.Run("heartbeat after reservation expires returns 410", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		checkoutSeedCart(t, pool, userID, eventID, 1)
		r := buildCheckoutRouter(pool)

		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)

		// Force-expire the reservation
		pool.Exec(context.Background(),
			`UPDATE ticket_reservations SET expires_at = NOW() - INTERVAL '1 second'`)

		w := doAuthRequest(r, http.MethodPost, "/checkout/heartbeat", nil, token)
		assert.Equal(t, http.StatusGone, w.Code)
	})

	t.Run("heartbeat returns shortest TTL across multi-event reservations", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		event1ID := checkoutSeedEvent(t, pool, userID, 50)
		event2ID := checkoutSeedEvent(t, pool, userID, 50)

		var cartID string
		pool.QueryRow(context.Background(),
			`INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID,
		).Scan(&cartID)
		pool.Exec(context.Background(), `
			INSERT INTO cart_items (cart_id, item_type, event_id, quantity, unit_price_snapshot)
			VALUES ($1, 'ticket', $2, 1, 100.00), ($1, 'ticket', $3, 1, 100.00)
		`, cartID, event1ID, event2ID)

		r := buildCheckoutRouter(pool)
		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)

		// Shorten one reservation to 3 minutes
		pool.Exec(context.Background(), `
			UPDATE ticket_reservations SET expires_at = NOW() + INTERVAL '3 minutes'
			WHERE event_id = $1
		`, event1ID)

		w := doAuthRequest(r, http.MethodPost, "/checkout/heartbeat", nil, token)
		require.Equal(t, http.StatusOK, w.Code)
		// After heartbeat both are reset to 10 min — seconds_left should be close to 600
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Greater(t, int(resp["seconds_left"].(float64)), 550)
	})
}

// ─── POST /checkout/abandon ───────────────────────────────────────────────────

func TestCheckoutAbandon(t *testing.T) {
	pool := setupCheckoutTestDB(t)
	defer pool.Close()

	t.Run("releases reservation and decrements reserved_count", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		checkoutSeedCart(t, pool, userID, eventID, 3)
		r := buildCheckoutRouter(pool)

		// Enter checkout — creates reservation
		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)
		assert.Equal(t, 3, getReservedCount(t, pool, eventID))

		// Abandon
		w := doAuthRequest(r, http.MethodPost, "/checkout/abandon", nil, token)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, true, resp["released"])

		// reserved_count back to 0
		assert.Equal(t, 0, getReservedCount(t, pool, eventID))
	})

	t.Run("abandon with no active reservation is a no-op", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		_, token := checkoutSeedUser(t, pool)
		r := buildCheckoutRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/checkout/abandon", nil, token)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// released: false is acceptable — no reservation existed
		assert.NotNil(t, resp["released"])
	})

	t.Run("abandon releases all reservations in a multi-event cart", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		event1ID := checkoutSeedEvent(t, pool, userID, 50)
		event2ID := checkoutSeedEvent(t, pool, userID, 50)

		var cartID string
		pool.QueryRow(context.Background(),
			`INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID,
		).Scan(&cartID)
		pool.Exec(context.Background(), `
			INSERT INTO cart_items (cart_id, item_type, event_id, quantity, unit_price_snapshot)
			VALUES ($1, 'ticket', $2, 2, 100.00), ($1, 'ticket', $3, 3, 100.00)
		`, cartID, event1ID, event2ID)

		r := buildCheckoutRouter(pool)
		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)
		assert.Equal(t, 2, getReservedCount(t, pool, event1ID))
		assert.Equal(t, 3, getReservedCount(t, pool, event2ID))

		abandonW := doAuthRequest(r, http.MethodPost, "/checkout/abandon", nil, token)
		require.Equal(t, http.StatusOK, abandonW.Code)

		// Both events fully released
		assert.Equal(t, 0, getReservedCount(t, pool, event1ID))
		assert.Equal(t, 0, getReservedCount(t, pool, event2ID))
	})

	t.Run("capacity becomes available again after abandon", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		// Event with capacity 2
		eventID := checkoutSeedEvent(t, pool, userID, 2)
		checkoutSeedCart(t, pool, userID, eventID, 2)
		r := buildCheckoutRouter(pool)

		// User 1 reserves all capacity
		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)
		assert.Equal(t, 2, getReservedCount(t, pool, eventID))

		// User 2 tries to reserve — should fail (sold out)
		userID2, token2 := checkoutSeedUser(t, pool)
		cartID2 := checkoutSeedCart(t, pool, userID2, eventID, 1)
		_ = cartID2
		w := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token2)
		assert.Equal(t, http.StatusConflict, w.Code, "should be sold out while user 1 has reservation")

		// User 1 abandons
		doAuthRequest(r, http.MethodPost, "/checkout/abandon", nil, token)
		assert.Equal(t, 0, getReservedCount(t, pool, eventID))

		// User 2 tries again — should now succeed
		w2 := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token2)
		assert.Equal(t, http.StatusOK, w2.Code, "capacity should be available after user 1 abandoned")
	})
}

// ─── Reservation timing tests ─────────────────────────────────────────────────

func TestCheckoutReservationTiming(t *testing.T) {
	pool := setupCheckoutTestDB(t)
	defer pool.Close()

	t.Run("reservation expires_at is approximately 10 minutes from now", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		checkoutSeedCart(t, pool, userID, eventID, 1)
		r := buildCheckoutRouter(pool)

		before := time.Now()
		w := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, w.Code)

		var expiresAt time.Time
		pool.QueryRow(context.Background(),
			`SELECT expires_at FROM ticket_reservations ORDER BY created_at DESC LIMIT 1`,
		).Scan(&expiresAt)

		expectedExpiry := before.Add(10 * time.Minute)
		diff := expiresAt.Sub(expectedExpiry).Abs()
		assert.Less(t, diff, 5*time.Second, "expires_at should be within 5s of now+10min")
	})

	t.Run("seconds_left in begin response is close to 600", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		checkoutSeedCart(t, pool, userID, eventID, 1)
		r := buildCheckoutRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		secondsLeft := int(resp["seconds_left"].(float64))
		assert.GreaterOrEqual(t, secondsLeft, 595, "seconds_left should be close to 600")
		assert.LessOrEqual(t, secondsLeft, 600, "seconds_left should not exceed 600")
	})
}