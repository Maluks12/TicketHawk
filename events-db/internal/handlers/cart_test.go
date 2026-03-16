package handlers_test

// =============================================================================
// CART TESTS
// =============================================================================
//
// Tests covered:
//   - GET  /cart                → GetCartHandler       (auth optional)
//   - POST /cart/items          → AddCartItemHandler   (auth optional)
//   - PATCH /cart/items/:id     → UpdateCartItemHandler (auth optional)
//   - DELETE /cart/items/:id    → DeleteCartItemHandler (auth optional)
//   - DELETE /cart              → ClearCartHandler     (auth optional)
//   - POST /cart/merge          → MergeCartHandler     (auth required)
//
// Prerequisites:
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db -p 5433:5432 postgres:15
//
// Run:
//   go test ./internal/handlers/... -v -run TestCart
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

func setupCartTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Skipf("cart DB tests skipped — could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("cart DB tests skipped — DB not reachable: %v", pingErr)
	}
	createCartTestTables(t, pool)
	cleanCartTables(t, pool)
	return pool
}

func createCartTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
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

		// FIX 1: added all columns that GetEventByID selects:
		//   cover_image_url, banner_image_url, latitude, longitude,
		//   featured_until, refund_policy, terms_and_conditions, tags
		`CREATE TABLE events (
			id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title                VARCHAR(255) NOT NULL DEFAULT 'Test Event',
			description          TEXT NOT NULL DEFAULT '',
			cover_image_url      TEXT,
			banner_image_url     TEXT,
			start_time           TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
			end_time             TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '8 days',
			expires_at           TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
			timezone             VARCHAR(50) NOT NULL DEFAULT 'SAST',
			venue_name           VARCHAR(255) NOT NULL DEFAULT '',
			address              TEXT NOT NULL DEFAULT '',
			city                 VARCHAR(255) NOT NULL DEFAULT '',
			latitude             DOUBLE PRECISION,
			longitude            DOUBLE PRECISION,
			price                DECIMAL(10,2) NOT NULL DEFAULT 100.00,
			currency             VARCHAR(3) NOT NULL DEFAULT 'ZAR',
			max_capacity         INTEGER DEFAULT 100,
			tickets_sold         INTEGER NOT NULL DEFAULT 0,
			reserved_count       INTEGER NOT NULL DEFAULT 0,
			allow_waitlist       BOOLEAN DEFAULT TRUE,
			status               VARCHAR(50) NOT NULL DEFAULT 'published',
			is_featured          BOOLEAN DEFAULT FALSE,
			featured_until       TIMESTAMPTZ,
			refund_policy        TEXT,
			terms_and_conditions TEXT,
			tags                 JSONB DEFAULT '[]',
			organizer_id         UUID REFERENCES users(id),
			category_id          INTEGER NOT NULL REFERENCES categories(id),
			views                BIGINT DEFAULT 0,
			created_at           TIMESTAMPTZ DEFAULT NOW(),
			updated_at           TIMESTAMPTZ DEFAULT NOW()
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
			CONSTRAINT quantity_positive CHECK (quantity > 0),
			CONSTRAINT merch_requires_variant CHECK (
				(item_type = 'ticket'      AND merchandise_variant_id IS NULL) OR
				(item_type = 'merchandise' AND merchandise_variant_id IS NOT NULL)
			)
		)`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("cart table setup failed: %v\nSQL: %s", err, s)
		}
	}
}

func cleanCartTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
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

func cartSeedUser(t *testing.T, pool *pgxpool.Pool) (userID, token string) {
	t.Helper()
	cfg := authTestConfig()
	email := randomEmail()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("Pass1234!"), bcrypt.DefaultCost)
	u := &models.User{Email: email, Password: string(hashed), Role: "user"}
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

// FIX 2: randomise the name as well as the slug to prevent unique constraint
// violations when cartSeedCategory is called multiple times in the same test.
func cartSeedCategory(t *testing.T, pool *pgxpool.Pool) int {
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

func cartSeedEvent(t *testing.T, pool *pgxpool.Pool, organizerID string) string {
	t.Helper()
	catID := cartSeedCategory(t, pool)
	var eventID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title, status, price, currency, max_capacity,
		                    category_id, organizer_id)
		VALUES ('Test Event', 'published', 100.00, 'ZAR', 50, $1, $2)
		RETURNING id
	`, catID, organizerID).Scan(&eventID)
	require.NoError(t, err)
	return eventID
}

func cartSeedMerchWithVariant(t *testing.T, pool *pgxpool.Pool, eventID string) (merchID, variantID string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		INSERT INTO merchandise (event_id, name, base_price)
		VALUES ($1, 'Chelsea Scarf', 250.00)
		RETURNING id
	`, eventID).Scan(&merchID)
	require.NoError(t, err)

	sku := fmt.Sprintf("SKU-%d", time.Now().UnixNano())
	err = pool.QueryRow(context.Background(), `
		INSERT INTO merchandise_variants (merchandise_id, sku, attributes, stock_quantity)
		VALUES ($1, $2, '{"size":"M"}'::jsonb, 10)
		RETURNING id
	`, merchID, sku).Scan(&variantID)
	require.NoError(t, err)
	return
}

// ─── router builder ───────────────────────────────────────────────────────────

func buildCartRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()
	r := gin.New()

	auth := r.Group("/auth")
	auth.POST("/login", handlers.LoginHandler(pool, cfg))

	cart := r.Group("/cart")
	{
		cart.GET("", middleware.OptionalAuth(cfg), handlers.GetCartHandler(pool))
		cart.POST("/items", middleware.OptionalAuth(cfg), handlers.AddCartItemHandler(pool))
		cart.PATCH("/items/:id", middleware.OptionalAuth(cfg), handlers.UpdateCartItemHandler(pool))
		cart.DELETE("/items/:id", middleware.OptionalAuth(cfg), handlers.DeleteCartItemHandler(pool))
		cart.DELETE("", middleware.OptionalAuth(cfg), handlers.ClearCartHandler(pool))
		cart.POST("/merge", middleware.AuthMiddleware(cfg), handlers.MergeCartHandler(pool))
	}
	return r
}

// doSessionRequest attaches an X-Session-ID header for guest cart requests.
func doSessionRequest(r *gin.Engine, method, path string, body interface{}, sessionID string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", sessionID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ─── GET /cart ────────────────────────────────────────────────────────────────

func TestCartGet(t *testing.T) {
	pool := setupCartTestDB(t)
	defer pool.Close()

	t.Run("authenticated user gets empty cart on first request", func(t *testing.T) {
		cleanCartTables(t, pool)
		_, token := cartSeedUser(t, pool)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/cart", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.EqualValues(t, 0, resp["total_items"])
		assert.EqualValues(t, 0, resp["total_amount"])
	})

	t.Run("guest gets empty cart using session ID", func(t *testing.T) {
		r := buildCartRouter(pool)
		req := httptest.NewRequest(http.MethodGet, "/cart", nil)
		req.Header.Set("X-Session-ID", "550e8400-e29b-41d4-a716-446655440001")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("no token and no session ID returns 400", func(t *testing.T) {
		r := buildCartRouter(pool)
		req := httptest.NewRequest(http.MethodGet, "/cart", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── POST /cart/items ─────────────────────────────────────────────────────────

func TestCartAddItem(t *testing.T) {
	pool := setupCartTestDB(t)
	defer pool.Close()

	t.Run("authenticated user adds ticket item", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket",
			"event_id":  eventID,
			"quantity":  2,
		}, token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "ticket", resp["item_type"])
		assert.EqualValues(t, 2, resp["quantity"])
		assert.EqualValues(t, 100.00, resp["unit_price_snapshot"])
	})

	t.Run("authenticated user adds merch item with variant", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		_, variantID := cartSeedMerchWithVariant(t, pool, eventID)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type":              "merchandise",
			"event_id":               eventID,
			"merchandise_variant_id": variantID,
			"quantity":               1,
		}, token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "merchandise", resp["item_type"])
		assert.EqualValues(t, 250.00, resp["unit_price_snapshot"])
	})

	t.Run("guest adds ticket item using session ID", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, _ := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		body, _ := json.Marshal(map[string]interface{}{
			"item_type": "ticket",
			"event_id":  eventID,
			"quantity":  1,
		})
		req := httptest.NewRequest(http.MethodPost, "/cart/items", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Session-ID", "550e8400-e29b-41d4-a716-446655440002")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	})

	t.Run("merch item without variant ID returns 400", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "merchandise",
			"event_id":  eventID,
			"quantity":  1,
			// missing merchandise_variant_id
		}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("non-existent event returns 404", func(t *testing.T) {
		cleanCartTables(t, pool)
		_, token := cartSeedUser(t, pool)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket",
			"event_id":  "00000000-0000-0000-0000-000000000099",
			"quantity":  1,
		}, token)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("quantity 0 returns 400 validation error", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket",
			"event_id":  eventID,
			"quantity":  0,
		}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("quantity over 10 returns 400 validation error", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket",
			"event_id":  eventID,
			"quantity":  11,
		}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid item_type returns 400", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "gift_card",
			"event_id":  eventID,
			"quantity":  1,
		}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no session or token returns 400", func(t *testing.T) {
		r := buildCartRouter(pool)
		w := doRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket",
			"event_id":  "00000000-0000-0000-0000-000000000001",
			"quantity":  1,
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── PATCH /cart/items/:id ────────────────────────────────────────────────────

func TestCartUpdateItem(t *testing.T) {
	pool := setupCartTestDB(t)
	defer pool.Close()

	t.Run("update quantity of existing item", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		// Add item
		addResp := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket", "event_id": eventID, "quantity": 1,
		}, token)
		require.Equal(t, http.StatusCreated, addResp.Code)
		var addBody map[string]interface{}
		require.NoError(t, json.Unmarshal(addResp.Body.Bytes(), &addBody))
		itemID := addBody["id"].(string)

		// Update quantity
		w := doAuthRequest(r, http.MethodPatch, "/cart/items/"+itemID, map[string]interface{}{
			"quantity": 3,
		}, token)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("update non-existent item returns 400", func(t *testing.T) {
		cleanCartTables(t, pool)
		_, token := cartSeedUser(t, pool)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodPatch, "/cart/items/00000000-0000-0000-0000-000000000000",
			map[string]interface{}{"quantity": 2}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("update with quantity 0 returns 400", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		addResp := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket", "event_id": eventID, "quantity": 1,
		}, token)
		require.Equal(t, http.StatusCreated, addResp.Code)
		var addBody map[string]interface{}
		require.NoError(t, json.Unmarshal(addResp.Body.Bytes(), &addBody))
		itemID := addBody["id"].(string)

		w := doAuthRequest(r, http.MethodPatch, "/cart/items/"+itemID,
			map[string]interface{}{"quantity": 0}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── DELETE /cart/items/:id ───────────────────────────────────────────────────

func TestCartDeleteItem(t *testing.T) {
	pool := setupCartTestDB(t)
	defer pool.Close()

	t.Run("removes an item from the cart", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		addResp := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket", "event_id": eventID, "quantity": 1,
		}, token)
		require.Equal(t, http.StatusCreated, addResp.Code)
		var addBody map[string]interface{}
		require.NoError(t, json.Unmarshal(addResp.Body.Bytes(), &addBody))
		itemID := addBody["id"].(string)

		w := doAuthRequest(r, http.MethodDelete, "/cart/items/"+itemID, nil, token)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// Verify cart is now empty
		getW := doAuthRequest(r, http.MethodGet, "/cart", nil, token)
		var cartResp map[string]interface{}
		require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &cartResp))
		assert.EqualValues(t, 0, cartResp["total_items"])
	})

	t.Run("delete item belonging to another user returns 404", func(t *testing.T) {
		cleanCartTables(t, pool)
		ownerID, ownerToken := cartSeedUser(t, pool)
		_, otherToken := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, ownerID)
		r := buildCartRouter(pool)

		// owner adds item
		addResp := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket", "event_id": eventID, "quantity": 1,
		}, ownerToken)
		require.Equal(t, http.StatusCreated, addResp.Code)
		var addBody map[string]interface{}
		require.NoError(t, json.Unmarshal(addResp.Body.Bytes(), &addBody))
		itemID := addBody["id"].(string)

		// other user tries to delete owner's item — their cart is different so item not found
		w := doAuthRequest(r, http.MethodDelete, "/cart/items/"+itemID, nil, otherToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ─── DELETE /cart ─────────────────────────────────────────────────────────────

func TestCartClear(t *testing.T) {
	pool := setupCartTestDB(t)
	defer pool.Close()

	t.Run("clears all items from the cart", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		// Add two items
		for i := 0; i < 2; i++ {
			w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
				"item_type": "ticket", "event_id": eventID, "quantity": 1,
			}, token)
			require.Equal(t, http.StatusCreated, w.Code)
		}

		// Clear cart
		clearW := doAuthRequest(r, http.MethodDelete, "/cart", nil, token)
		assert.Equal(t, http.StatusOK, clearW.Code, clearW.Body.String())

		// Verify empty
		getW := doAuthRequest(r, http.MethodGet, "/cart", nil, token)
		var cartResp map[string]interface{}
		require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &cartResp))
		assert.EqualValues(t, 0, cartResp["total_items"])
	})
}

// ─── POST /cart/merge ─────────────────────────────────────────────────────────

func TestCartMerge(t *testing.T) {
	pool := setupCartTestDB(t)
	defer pool.Close()

	t.Run("merges guest session cart into user cart on login", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		eventID := cartSeedEvent(t, pool, userID)
		r := buildCartRouter(pool)

		sessionID := "550e8400-e29b-41d4-a716-446655440010"

		// Guest adds a ticket to their session cart
		guestBody, _ := json.Marshal(map[string]interface{}{
			"item_type": "ticket", "event_id": eventID, "quantity": 2,
		})
		guestReq := httptest.NewRequest(http.MethodPost, "/cart/items", bytes.NewReader(guestBody))
		guestReq.Header.Set("Content-Type", "application/json")
		guestReq.Header.Set("X-Session-ID", sessionID)
		guestW := httptest.NewRecorder()
		r.ServeHTTP(guestW, guestReq)
		require.Equal(t, http.StatusCreated, guestW.Code, guestW.Body.String())

		// User logs in and merges
		mergeW := doAuthRequest(r, http.MethodPost, "/cart/merge", map[string]string{
			"session_id": sessionID,
		}, token)
		assert.Equal(t, http.StatusOK, mergeW.Code, mergeW.Body.String())

		// Verify user cart has the merged item
		getW := doAuthRequest(r, http.MethodGet, "/cart", nil, token)
		var cartResp map[string]interface{}
		require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &cartResp))
		assert.EqualValues(t, 1, cartResp["total_items"], "merged item should appear in user cart")
	})

	t.Run("merge without auth returns 401", func(t *testing.T) {
		r := buildCartRouter(pool)
		w := doRequest(r, http.MethodPost, "/cart/merge", map[string]string{
			"session_id": "550e8400-e29b-41d4-a716-446655440011",
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("merge with invalid session UUID returns 400", func(t *testing.T) {
		cleanCartTables(t, pool)
		_, token := cartSeedUser(t, pool)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/merge", map[string]string{
			"session_id": "not-a-uuid",
		}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("merge with no existing guest cart is a no-op", func(t *testing.T) {
		cleanCartTables(t, pool)
		_, token := cartSeedUser(t, pool)
		r := buildCartRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/cart/merge", map[string]string{
			"session_id": "550e8400-e29b-41d4-a716-446655440099",
		}, token)
		// No guest cart to merge — should succeed silently
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ─── Multi-event cart integration test ───────────────────────────────────────

func TestCartMultiEventBehaviour(t *testing.T) {
	pool := setupCartTestDB(t)
	defer pool.Close()

	t.Run("cart correctly holds tickets and merch from different events", func(t *testing.T) {
		cleanCartTables(t, pool)
		userID, token := cartSeedUser(t, pool)
		event1ID := cartSeedEvent(t, pool, userID)
		event2ID := cartSeedEvent(t, pool, userID)
		_, variant1ID := cartSeedMerchWithVariant(t, pool, event1ID)
		r := buildCartRouter(pool)

		// Tickets to event 1
		w := doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket", "event_id": event1ID, "quantity": 2,
		}, token)
		require.Equal(t, http.StatusCreated, w.Code)

		// Tickets to event 2
		w = doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type": "ticket", "event_id": event2ID, "quantity": 1,
		}, token)
		require.Equal(t, http.StatusCreated, w.Code)

		// Merch from event 1
		w = doAuthRequest(r, http.MethodPost, "/cart/items", map[string]interface{}{
			"item_type":              "merchandise",
			"event_id":               event1ID,
			"merchandise_variant_id": variant1ID,
			"quantity":               1,
		}, token)
		require.Equal(t, http.StatusCreated, w.Code)

		// Fetch cart and verify all 3 items present
		getW := doAuthRequest(r, http.MethodGet, "/cart", nil, token)
		require.Equal(t, http.StatusOK, getW.Code)
		var cartResp map[string]interface{}
		require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &cartResp))
		assert.EqualValues(t, 3, cartResp["total_items"], "cart should hold items from both events")
		// Total: 2×100 + 1×100 + 1×250 = 550
		assert.EqualValues(t, 550.00, cartResp["total_amount"])
		// Two event groups
		groups := cartResp["items_by_event"].([]interface{})
		assert.Len(t, groups, 2, "items should be grouped by event")
	})
}