package handlers_test

// =============================================================================
// CHECKOUT / TICKET RESERVATION TESTS
// =============================================================================
//
// Tests covered:
//   - POST /checkout/begin      → CheckoutBeginHandler
//   - POST /checkout/heartbeat  → CheckoutHeartbeatHandler
//   - POST /checkout/abandon    → CheckoutAbandonHandler
//   - POST /checkout/confirm    → CheckoutConfirmHandler
//
// Verifies:
//   - Reservations are created atomically when entering checkout
//   - reserved_count on events is incremented correctly
//   - Heartbeat refreshes the 10-minute window
//   - Abandon releases capacity immediately
//   - Sold-out events block reservation
//   - Carts with no ticket items are rejected
//   - Multi-event carts create independent reservations per event
//   - Confirm converts reservations → bookings and creates Stitch payment intent
//   - Webhook confirms all bookings from the same cart
//   - Guest users can complete the full confirm flow
//
// Run:
//   go test ./internal/handlers/... -v -run TestCheckout
// =============================================================================

import (
	//"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"events-db/internal/handlers"
	"events-db/internal/middleware"
	"events-db/internal/services"

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
		"DROP TABLE IF EXISTS payments CASCADE",
		"DROP TABLE IF EXISTS bookings CASCADE",
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

		// bookings — needed by CheckoutConfirmHandler (ConvertReservationsToBookings)
		`DO $$ BEGIN CREATE TYPE payment_method_type AS ENUM
			('credit_card','paypal','apple_pay','google_pay','samsung_pay','instant_eft');
		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		`CREATE TABLE bookings (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_number   VARCHAR(50) UNIQUE NOT NULL DEFAULT 'BK-' || substr(gen_random_uuid()::text,1,8),
			event_id         UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			user_id          UUID REFERENCES users(id) ON DELETE SET NULL,
			is_guest         BOOLEAN NOT NULL DEFAULT FALSE,
			ticket_quantity  INT NOT NULL DEFAULT 1,
			unit_price       NUMERIC(10,2) NOT NULL DEFAULT 0,
			subtotal         NUMERIC(10,2) NOT NULL DEFAULT 0,
			platform_fee     NUMERIC(10,2) NOT NULL DEFAULT 0,
			organizer_fee    NUMERIC(10,2) NOT NULL DEFAULT 0,
			total_amount     NUMERIC(10,2) NOT NULL DEFAULT 0,
			currency         VARCHAR(10) NOT NULL DEFAULT 'ZAR',
			booking_status   VARCHAR(50) NOT NULL DEFAULT 'pending',
			payment_status   VARCHAR(50) NOT NULL DEFAULT 'pending',
			tickets_sent     BOOLEAN NOT NULL DEFAULT FALSE,
			attendee_email   VARCHAR(255),
			cart_id          UUID REFERENCES carts(id) ON DELETE SET NULL,
			confirmed_at     TIMESTAMPTZ,
			cancelled_at     TIMESTAMPTZ,
			updated_at       TIMESTAMPTZ DEFAULT NOW(),
			created_at       TIMESTAMPTZ DEFAULT NOW()
		)`,

		// payments — needed by CreateCartPaymentIntent (called from CheckoutConfirmHandler)
		`CREATE TABLE payments (
			id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_id                UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
			payment_gateway           VARCHAR(50) NOT NULL DEFAULT 'stitch',
			payment_intent_id         TEXT,
			transaction_id            TEXT,
			amount                    NUMERIC(10,2) NOT NULL,
			currency                  VARCHAR(10) NOT NULL DEFAULT 'ZAR',
			payment_method            payment_method_type NOT NULL DEFAULT 'credit_card',
			status                    VARCHAR(50) NOT NULL DEFAULT 'pending',
			stitch_payment_request_id TEXT,
			stitch_payment_url        TEXT,
			cart_id                   UUID REFERENCES carts(id) ON DELETE SET NULL,
			webhook_received          BOOLEAN DEFAULT FALSE,
			completed_at              TIMESTAMPTZ,
			created_at                TIMESTAMPTZ DEFAULT NOW(),
			updated_at                TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(booking_id)
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
	pool.Exec(ctx, "TRUNCATE payments           CASCADE")
	pool.Exec(ctx, "TRUNCATE bookings           CASCADE")
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
	return buildCheckoutRouterWithMock(pool, "")
}

// buildCheckoutRouterWithMock builds the checkout router and optionally points the
// payment service at a mock Stitch server (for /checkout/confirm tests).
func buildCheckoutRouterWithMock(pool *pgxpool.Pool, mockStitchURL string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()

	jobChan := make(chan string, 256)
	paymentSvc := services.NewPaymentService(pool, cfg, jobChan)
	if mockStitchURL != "" {
		// Redirect Stitch calls to the mock server for confirm tests
		// (relies on stitchTokenURL / stitchGraphQLURL being package-level vars in payment_service.go)
		_ = mockStitchURL // injected via pointCheckoutMock helper below
	}

	r := gin.New()

	auth := r.Group("/auth")
	auth.POST("/login", handlers.LoginHandler(pool, cfg))

	checkout := r.Group("/checkout")
	{
		checkout.POST("/begin",     middleware.OptionalAuth(cfg), handlers.CheckoutBeginHandler(pool))
		checkout.POST("/heartbeat", middleware.OptionalAuth(cfg), handlers.CheckoutHeartbeatHandler(pool))
		checkout.POST("/abandon",   middleware.OptionalAuth(cfg), handlers.CheckoutAbandonHandler(pool))
		checkout.POST("/confirm",   middleware.OptionalAuth(cfg), handlers.CheckoutConfirmHandler(pool, paymentSvc))
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
// ─── POST /checkout/confirm ───────────────────────────────────────────────────
//
// Confirm converts every active reservation into real bookings, then creates a
// single Stitch payment intent for the full cart total. Tests cover:
//   - Registered user single-event cart
//   - Registered user multi-event cart (two events, one payment)
//   - Guest user single-event cart (X-Session-ID only)
//   - Guest user multi-event cart
//   - Capacity moves from reserved_count → tickets_sold on confirm
//   - Reservations are deleted after confirm
//   - booking_ids returned in response
//   - No active reservations returns 400
//   - Expired reservations return 400
//   - No session or token returns 400
//   - Stitch failure returns 500 (bookings stay pending, cleaned up by expiry worker)

// checkoutMockStitch starts a mock Stitch server that returns a successful
// payment initiation response and redirects the payment service to it.
// Returns a cleanup function — call it (or t.Cleanup) to restore the real URLs.
func checkoutMockStitch(t *testing.T, paymentRequestID, clientToken string) (cleanup func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/connect/token" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "mock_token", "token_type": "Bearer", "expires_in": 3600,
			})
			return
		}
		data, _ := json.Marshal(map[string]interface{}{
			"clientCardPaymentInitiate": map[string]interface{}{
				"paymentInitiation": map[string]interface{}{
					"id": paymentRequestID, "clientToken": clientToken,
				},
			},
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": json.RawMessage(data), "errors": nil,
		})
	}))
	restore := services.SetStitchBaseURLs(
		srv.URL+"/connect/token",
		srv.URL+"/graphql",
		srv.URL+"/connect/token",
	)
	return func() {
		restore()
		srv.Close()
	}
}

// checkoutMockStitchError starts a mock Stitch server that always returns a
// GraphQL error. Use to test confirm-time Stitch failures.
func checkoutMockStitchError(t *testing.T) (cleanup func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/connect/token" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "mock_token", "token_type": "Bearer", "expires_in": 3600,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":   nil,
			"errors": []map[string]string{{"message": "stitch service unavailable"}},
		})
	}))
	restore := services.SetStitchBaseURLs(
		srv.URL+"/connect/token",
		srv.URL+"/graphql",
		srv.URL+"/connect/token",
	)
	return func() {
		restore()
		srv.Close()
	}
}

// checkoutSeedGuestCart creates a session-based cart with ticket items.
func checkoutSeedGuestCart(t *testing.T, pool *pgxpool.Pool, sessionID, eventID string, qty int) string {
	t.Helper()
	var cartID string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO carts (session_id) VALUES ($1::uuid) RETURNING id`, sessionID,
	).Scan(&cartID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO cart_items (cart_id, item_type, event_id, quantity, unit_price_snapshot)
		VALUES ($1, 'ticket', $2, $3, 100.00)
	`, cartID, eventID, qty)
	require.NoError(t, err)
	return cartID
}

// getBookingsByCartID returns all booking IDs created from a cart.
func getBookingsByCartID(t *testing.T, pool *pgxpool.Pool, cartID string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT id FROM bookings WHERE cart_id = $1 ORDER BY created_at ASC`, cartID)
	require.NoError(t, err)
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	return ids
}

// getTicketsSold returns tickets_sold for an event.
func getTicketsSold(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	pool.QueryRow(context.Background(),
		`SELECT tickets_sold FROM events WHERE id = $1`, eventID,
	).Scan(&count)
	return count
}

func TestCheckoutConfirm(t *testing.T) {
	pool := setupCheckoutTestDB(t)
	defer pool.Close()

	// ─── registered user, single-event cart ───────────────────────────────────

	t.Run("registered user — converts reservation to booking and returns client token", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		cleanup := checkoutMockStitch(t, "req_confirm_001", "token_confirm_001")
		defer cleanup()

		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		cartID := checkoutSeedCart(t, pool, userID, eventID, 2)
		r := buildCheckoutRouter(pool)

		// Begin to create reservation
		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)
		assert.Equal(t, 2, getReservedCount(t, pool, eventID))

		// Confirm
		w := doAuthRequest(r, http.MethodPost, "/checkout/confirm", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "token_confirm_001", resp["client_token"])
		assert.Equal(t, "req_confirm_001", resp["payment_request_id"])
		assert.EqualValues(t, 200.00, resp["total_amount"])
		assert.NotEmpty(t, resp["booking_ids"])
		assert.NotEmpty(t, resp["primary_booking_id"])

		// Reservation must be deleted
		assert.Equal(t, 0, getReservationCount(t, pool, cartID), "reservation must be deleted after confirm")

		// Booking created with cart_id
		bookingIDs := getBookingsByCartID(t, pool, cartID)
		require.Len(t, bookingIDs, 1)

		// Capacity: reserved_count=0, tickets_sold=2
		assert.Equal(t, 0, getReservedCount(t, pool, eventID))
		assert.Equal(t, 2, getTicketsSold(t, pool, eventID))
	})

	// ─── registered user, multi-event cart ────────────────────────────────────

	t.Run("registered user — multi-event cart creates one booking per event", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		cleanup := checkoutMockStitch(t, "req_multi_confirm", "token_multi_confirm")
		defer cleanup()

		userID, token := checkoutSeedUser(t, pool)
		event1ID := checkoutSeedEvent(t, pool, userID, 50)
		event2ID := checkoutSeedEvent(t, pool, userID, 50)

		var cartID string
		pool.QueryRow(context.Background(),
			`INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID,
		).Scan(&cartID)
		pool.Exec(context.Background(), `
			INSERT INTO cart_items (cart_id, item_type, event_id, quantity, unit_price_snapshot)
			VALUES ($1, 'ticket', $2, 2, 100.00), ($1, 'ticket', $3, 1, 100.00)
		`, cartID, event1ID, event2ID)

		r := buildCheckoutRouter(pool)
		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)

		w := doAuthRequest(r, http.MethodPost, "/checkout/confirm", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		// Two booking IDs in response — one per event
		bookingIDsRaw := resp["booking_ids"].([]interface{})
		assert.Len(t, bookingIDsRaw, 2, "one booking per event")
		assert.EqualValues(t, 300.00, resp["total_amount"], "2×100 + 1×100 = 300")

		// Both bookings in DB
		dbBookings := getBookingsByCartID(t, pool, cartID)
		assert.Len(t, dbBookings, 2)

		// Each event: reserved_count=0, tickets_sold correct
		assert.Equal(t, 0, getReservedCount(t, pool, event1ID))
		assert.Equal(t, 0, getReservedCount(t, pool, event2ID))
		assert.Equal(t, 2, getTicketsSold(t, pool, event1ID))
		assert.Equal(t, 1, getTicketsSold(t, pool, event2ID))

		// Reservations deleted
		assert.Equal(t, 0, getReservationCount(t, pool, cartID))
	})

	// ─── guest user, single-event cart ────────────────────────────────────────

	t.Run("guest user — confirm with X-Session-ID, single event", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		cleanup := checkoutMockStitch(t, "req_guest_confirm", "token_guest_confirm")
		defer cleanup()

		// Seed an organiser so the event has a valid organizer_id
		organizerID, _ := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, organizerID, 50)

		sessionID := fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", 30)
		cartID := checkoutSeedGuestCart(t, pool, sessionID, eventID, 1)

		r := buildCheckoutRouter(pool)

		// Guest begin
		beginReq := httptest.NewRequest(http.MethodPost, "/checkout/begin", nil)
		beginReq.Header.Set("X-Session-ID", sessionID)
		beginW := httptest.NewRecorder()
		r.ServeHTTP(beginW, beginReq)
		require.Equal(t, http.StatusOK, beginW.Code, beginW.Body.String())

		// Guest confirm
		confirmReq := httptest.NewRequest(http.MethodPost, "/checkout/confirm", nil)
		confirmReq.Header.Set("X-Session-ID", sessionID)
		confirmW := httptest.NewRecorder()
		r.ServeHTTP(confirmW, confirmReq)
		require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(confirmW.Body.Bytes(), &resp))
		assert.Equal(t, "token_guest_confirm", resp["client_token"])
		assert.NotEmpty(t, resp["booking_ids"])

		// Booking created with is_guest = true (userID was nil)
		bookingIDs := getBookingsByCartID(t, pool, cartID)
		require.Len(t, bookingIDs, 1)
		var isGuest bool
		pool.QueryRow(context.Background(),
			`SELECT is_guest FROM bookings WHERE id = $1`, bookingIDs[0],
		).Scan(&isGuest)
		assert.True(t, isGuest, "booking created from guest cart must have is_guest=true")

		assert.Equal(t, 0, getReservedCount(t, pool, eventID))
		assert.Equal(t, 1, getTicketsSold(t, pool, eventID))
	})

	// ─── guest user, multi-event cart ─────────────────────────────────────────

	t.Run("guest user — multi-event cart, two bookings created", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		cleanup := checkoutMockStitch(t, "req_guest_multi_confirm", "token_guest_multi_confirm")
		defer cleanup()

		organizerID, _ := checkoutSeedUser(t, pool)
		event1ID := checkoutSeedEvent(t, pool, organizerID, 50)
		event2ID := checkoutSeedEvent(t, pool, organizerID, 50)

		sessionID := fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", 40)
		var cartID string
		pool.QueryRow(context.Background(),
			`INSERT INTO carts (session_id) VALUES ($1::uuid) RETURNING id`, sessionID,
		).Scan(&cartID)
		pool.Exec(context.Background(), `
			INSERT INTO cart_items (cart_id, item_type, event_id, quantity, unit_price_snapshot)
			VALUES ($1, 'ticket', $2, 1, 100.00), ($1, 'ticket', $3, 1, 100.00)
		`, cartID, event1ID, event2ID)

		r := buildCheckoutRouter(pool)

		beginReq := httptest.NewRequest(http.MethodPost, "/checkout/begin", nil)
		beginReq.Header.Set("X-Session-ID", sessionID)
		beginW := httptest.NewRecorder()
		r.ServeHTTP(beginW, beginReq)
		require.Equal(t, http.StatusOK, beginW.Code, beginW.Body.String())

		confirmReq := httptest.NewRequest(http.MethodPost, "/checkout/confirm", nil)
		confirmReq.Header.Set("X-Session-ID", sessionID)
		confirmW := httptest.NewRecorder()
		r.ServeHTTP(confirmW, confirmReq)
		require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(confirmW.Body.Bytes(), &resp))
		bookingIDsRaw := resp["booking_ids"].([]interface{})
		assert.Len(t, bookingIDsRaw, 2)

		assert.Equal(t, 0, getReservedCount(t, pool, event1ID))
		assert.Equal(t, 0, getReservedCount(t, pool, event2ID))
		assert.Equal(t, 0, getReservationCount(t, pool, cartID))
	})

	// ─── cart also contains merch items ───────────────────────────────────────

	t.Run("cart with tickets and merch — merch included in total, only ticket bookings created", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		cleanup := checkoutMockStitch(t, "req_mixed_confirm", "token_mixed_confirm")
		defer cleanup()

		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)

		var cartID string
		pool.QueryRow(context.Background(),
			`INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID,
		).Scan(&cartID)

		// 2 tickets
		pool.Exec(context.Background(), `
			INSERT INTO cart_items (cart_id, item_type, event_id, quantity, unit_price_snapshot)
			VALUES ($1, 'ticket', $2, 2, 100.00)
		`, cartID, eventID)

		// 1 merch item
		var merchID, variantID string
		pool.QueryRow(context.Background(),
			`INSERT INTO merchandise (event_id, name, base_price) VALUES ($1, 'Scarf', 250.00) RETURNING id`,
			eventID).Scan(&merchID)
		pool.QueryRow(context.Background(),
			`INSERT INTO merchandise_variants (merchandise_id, sku, attributes) VALUES ($1, 'SKU-CT', '{}') RETURNING id`,
			merchID).Scan(&variantID)
		pool.Exec(context.Background(), `
			INSERT INTO cart_items (cart_id, item_type, event_id, merchandise_variant_id, quantity, unit_price_snapshot)
			VALUES ($1, 'merchandise', $2, $3, 1, 250.00)
		`, cartID, eventID, variantID)

		r := buildCheckoutRouter(pool)
		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)

		w := doAuthRequest(r, http.MethodPost, "/checkout/confirm", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// Total = 2×100 tickets + 1×250 merch = 450
		assert.EqualValues(t, 450.00, resp["total_amount"], "merch must be included in cart total")
		// Only one booking (for the ticket event)
		bookingIDsRaw := resp["booking_ids"].([]interface{})
		assert.Len(t, bookingIDsRaw, 1, "merch does not produce a booking")
	})

	// ─── error cases ──────────────────────────────────────────────────────────

	t.Run("no active reservations returns 400", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		checkoutSeedCart(t, pool, userID, eventID, 1)
		r := buildCheckoutRouter(pool)

		// Skip begin — no reservations exist
		w := doAuthRequest(r, http.MethodPost, "/checkout/confirm", nil, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["error"], "no active reservations")
	})

	t.Run("expired reservations return 400", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		checkoutSeedCart(t, pool, userID, eventID, 1)
		r := buildCheckoutRouter(pool)

		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)

		// Force-expire all reservations
		pool.Exec(context.Background(),
			`UPDATE ticket_reservations SET expires_at = NOW() - INTERVAL '1 second'`)

		w := doAuthRequest(r, http.MethodPost, "/checkout/confirm", nil, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no session and no token returns 400", func(t *testing.T) {
		r := buildCheckoutRouter(pool)
		req := httptest.NewRequest(http.MethodPost, "/checkout/confirm", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Stitch failure after bookings created returns 500 — bookings remain pending", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		cleanup := checkoutMockStitchError(t)
		defer cleanup()

		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		cartID := checkoutSeedCart(t, pool, userID, eventID, 1)
		r := buildCheckoutRouter(pool)

		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)

		w := doAuthRequest(r, http.MethodPost, "/checkout/confirm", nil, token)
		assert.Equal(t, http.StatusInternalServerError, w.Code)

		// Bookings were created by ConvertReservationsToBookings before Stitch was called
		// They sit as pending and will be expired by the expiry worker after 15 minutes
		bookings := getBookingsByCartID(t, pool, cartID)
		assert.Len(t, bookings, 1, "booking must exist even when Stitch fails")

		var bookingStatus string
		pool.QueryRow(context.Background(),
			`SELECT booking_status FROM bookings WHERE id = $1`, bookings[0],
		).Scan(&bookingStatus)
		assert.Equal(t, "pending", bookingStatus, "booking should remain pending until expiry worker cleans it up")
	})

	// ─── booking_ids in response match DB ─────────────────────────────────────

	t.Run("booking_ids in response match bookings in DB", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		cleanup := checkoutMockStitch(t, "req_id_check", "token_id_check")
		defer cleanup()

		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		cartID := checkoutSeedCart(t, pool, userID, eventID, 1)
		r := buildCheckoutRouter(pool)

		doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		w := doAuthRequest(r, http.MethodPost, "/checkout/confirm", nil, token)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		bookingIDsRaw := resp["booking_ids"].([]interface{})
		responseIDs := make(map[string]bool)
		for _, id := range bookingIDsRaw {
			responseIDs[id.(string)] = true
		}

		dbIDs := getBookingsByCartID(t, pool, cartID)
		for _, id := range dbIDs {
			assert.True(t, responseIDs[id], "every DB booking ID must appear in the response")
		}
	})

	// ─── confirm is idempotent when re-attempted ───────────────────────────────

	t.Run("re-calling confirm after reservations consumed returns 400", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		cleanup := checkoutMockStitch(t, "req_idem_confirm", "token_idem_confirm")
		defer cleanup()

		userID, token := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, userID, 50)
		checkoutSeedCart(t, pool, userID, eventID, 1)
		r := buildCheckoutRouter(pool)

		doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		w1 := doAuthRequest(r, http.MethodPost, "/checkout/confirm", nil, token)
		require.Equal(t, http.StatusOK, w1.Code)

		// Second confirm — no reservations left
		w2 := doAuthRequest(r, http.MethodPost, "/checkout/confirm", nil, token)
		assert.Equal(t, http.StatusBadRequest, w2.Code,
			"second confirm must fail — reservations were already consumed")
	})
}

// ─── Full checkout integration: begin → confirm → webhook ─────────────────────
//
// End-to-end test that simulates the complete cart checkout journey for both
// registered and guest users: enter checkout, confirm payment, receive webhook,
// verify all bookings are confirmed.

func TestCheckoutFullFlow(t *testing.T) {
	pool := setupCheckoutTestDB(t)
	defer pool.Close()

	t.Run("registered user: begin → confirm → webhook confirms all bookings", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		cleanup := checkoutMockStitch(t, "req_full_flow", "token_full_flow")
		defer cleanup()

		userID, token := checkoutSeedUser(t, pool)
		event1ID := checkoutSeedEvent(t, pool, userID, 50)
		event2ID := checkoutSeedEvent(t, pool, userID, 50)

		var cartID string
		pool.QueryRow(context.Background(),
			`INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID,
		).Scan(&cartID)
		pool.Exec(context.Background(), `
			INSERT INTO cart_items (cart_id, item_type, event_id, quantity, unit_price_snapshot)
			VALUES ($1, 'ticket', $2, 2, 100.00), ($1, 'ticket', $3, 1, 100.00)
		`, cartID, event1ID, event2ID)

		r := buildCheckoutRouter(pool)

		// 1. Enter checkout
		beginW := doAuthRequest(r, http.MethodPost, "/checkout/begin", nil, token)
		require.Equal(t, http.StatusOK, beginW.Code)
		assert.Equal(t, 2, getReservedCount(t, pool, event1ID))
		assert.Equal(t, 1, getReservedCount(t, pool, event2ID))

		// 2. Confirm — converts reservations to bookings, creates payment intent
		confirmW := doAuthRequest(r, http.MethodPost, "/checkout/confirm", nil, token)
		require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())

		var confirmResp map[string]interface{}
		require.NoError(t, json.Unmarshal(confirmW.Body.Bytes(), &confirmResp))
		paymentRequestID := confirmResp["payment_request_id"].(string)
		assert.Equal(t, "req_full_flow", paymentRequestID)

		bookings := getBookingsByCartID(t, pool, cartID)
		require.Len(t, bookings, 2)

		// After confirm: reserved_count=0, tickets_sold correct
		assert.Equal(t, 0, getReservedCount(t, pool, event1ID))
		assert.Equal(t, 0, getReservedCount(t, pool, event2ID))
		assert.Equal(t, 2, getTicketsSold(t, pool, event1ID))
		assert.Equal(t, 1, getTicketsSold(t, pool, event2ID))

		// Bookings start as pending
		for _, id := range bookings {
			var bs string
			pool.QueryRow(context.Background(),
				`SELECT booking_status FROM bookings WHERE id = $1`, id).Scan(&bs)
			assert.Equal(t, "pending", bs)
		}

		// 3. Stitch fires webhook — confirm all bookings
		jobChan := make(chan string, 8)
		paymentSvc := services.NewPaymentService(pool, authTestConfig(), jobChan)
		txns := []map[string]string{{"id": "txn_full_flow"}}
		data, _ := json.Marshal(map[string]interface{}{
			"paymentInitiation": map[string]interface{}{
				"id": paymentRequestID, "status": "completed",
				"payer":        map[string]interface{}{"paymentMethod": "credit_card"},
				"transactions": txns,
			},
		})
		webhookEvent := &services.StitchWebhookEvent{
			Type: "card_payment.updated",
			Data: json.RawMessage(data),
		}
		require.NoError(t, paymentSvc.HandleWebhook(webhookEvent))

		// All bookings confirmed
		for _, id := range bookings {
			var bs, ps string
			pool.QueryRow(context.Background(),
				`SELECT booking_status, payment_status FROM bookings WHERE id = $1`, id,
			).Scan(&bs, &ps)
			assert.Equal(t, "confirmed", bs, "booking %s must be confirmed after webhook", id)
			assert.Equal(t, "completed", ps)
		}

		// Both jobs enqueued for QR + email
		enqueued := make(map[string]bool)
		for i := 0; i < 2; i++ {
			select {
			case id := <-jobChan:
				enqueued[id] = true
			case <-time.After(200 * time.Millisecond):
				t.Errorf("booking job %d not enqueued", i+1)
			}
		}
		for _, id := range bookings {
			assert.True(t, enqueued[id], "booking %s must be enqueued for processing", id)
		}
	})

	t.Run("guest user: begin → confirm → webhook confirms booking", func(t *testing.T) {
		cleanCheckoutTables(t, pool)
		cleanup := checkoutMockStitch(t, "req_guest_full", "token_guest_full")
		defer cleanup()

		organizerID, _ := checkoutSeedUser(t, pool)
		eventID := checkoutSeedEvent(t, pool, organizerID, 50)

		sessionID := fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", 99)
		cartID := checkoutSeedGuestCart(t, pool, sessionID, eventID, 1)

		r := buildCheckoutRouter(pool)

		// Begin
		beginReq := httptest.NewRequest(http.MethodPost, "/checkout/begin", nil)
		beginReq.Header.Set("X-Session-ID", sessionID)
		beginW := httptest.NewRecorder()
		r.ServeHTTP(beginW, beginReq)
		require.Equal(t, http.StatusOK, beginW.Code)

		// Confirm
		confirmReq := httptest.NewRequest(http.MethodPost, "/checkout/confirm", nil)
		confirmReq.Header.Set("X-Session-ID", sessionID)
		confirmW := httptest.NewRecorder()
		r.ServeHTTP(confirmW, confirmReq)
		require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())

		var confirmResp map[string]interface{}
		require.NoError(t, json.Unmarshal(confirmW.Body.Bytes(), &confirmResp))
		paymentRequestID := confirmResp["payment_request_id"].(string)

		bookings := getBookingsByCartID(t, pool, cartID)
		require.Len(t, bookings, 1)

		assert.Equal(t, 0, getReservedCount(t, pool, eventID))
		assert.Equal(t, 1, getTicketsSold(t, pool, eventID))

		// Webhook
		jobChan := make(chan string, 4)
		paymentSvc := services.NewPaymentService(pool, authTestConfig(), jobChan)
		data, _ := json.Marshal(map[string]interface{}{
			"paymentInitiation": map[string]interface{}{
				"id": paymentRequestID, "status": "completed",
				"payer": map[string]interface{}{"paymentMethod": "instant_eft"},
				"transactions": []map[string]string{{"id": "txn_guest_full"}},
			},
		})
		require.NoError(t, paymentSvc.HandleWebhook(&services.StitchWebhookEvent{
			Type: "card_payment.updated",
			Data: json.RawMessage(data),
		}))

		var bs string
		pool.QueryRow(context.Background(),
			`SELECT booking_status FROM bookings WHERE id = $1`, bookings[0],
		).Scan(&bs)
		assert.Equal(t, "confirmed", bs, "guest booking must be confirmed after webhook")

		select {
		case id := <-jobChan:
			assert.Equal(t, bookings[0], id)
		case <-time.After(200 * time.Millisecond):
			t.Error("guest booking job not enqueued after webhook")
		}
	})
}