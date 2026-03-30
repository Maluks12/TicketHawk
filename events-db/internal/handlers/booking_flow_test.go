package handlers_test

// // =============================================================================
// // BOOKING FLOW TESTS
// // =============================================================================
// //
// // Tests covered:
// //   - POST /bookings              → CreateBookingHandler    (auth required)
// //   - GET  /bookings              → GetUserBookingsHandler  (auth required)
// //   - GET  /bookings/:id          → GetBookingDetailsHandler (auth required)
// //   - POST /bookings/:id/refund   → RequestRefundHandler   (auth required)
// //   - POST /bookings/:id/payment  → CreateStitchPayment    (auth required)
// //   - GET  /bookings/:id/payment  → GetPaymentStatusHandler (auth required)
// //   - POST /bookings/:id/payment/retry → RetryPaymentHandler (auth required)
// //   - POST /guest/bookings        → CreateGuestBookingHandler (no auth)
// //
// // Prerequisites: same Docker Postgres as auth_flow_test.go.
// // Run:  go test ./internal/handlers/... -v -run TestBooking
// // =============================================================================

// import (
// 	"context"
// 	"encoding/json"
// 	"net/http"
// 	"testing"

// 	"events-db/internal/handlers"
// 	"events-db/internal/middleware"
// 	"events-db/internal/models"
// 	"events-db/internal/repository"
// 	"events-db/internal/services"

// 	"github.com/gin-gonic/gin"
// 	"github.com/jackc/pgx/v5/pgxpool"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// 	"golang.org/x/crypto/bcrypt"
// )

// // ─── booking-specific test setup ─────────────────────────────────────────────

// func setupBookingTestDB(t *testing.T) *pgxpool.Pool {
// 	t.Helper()
// 	pool, err := pgxpool.New(context.Background(), authTestDBURL)
// 	if err != nil {
// 		t.Fatalf("Cannot connect to test DB: %v", err)
// 	}
// 	createBookingTestTables(t, pool)
// 	cleanBookingTables(t, pool)
// 	return pool
// }

// func createBookingTestTables(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	ctx := context.Background()

// 	// Drop in reverse dependency order so we always start from a clean slate.
// 	drops := []string{
// 		"DROP TABLE IF EXISTS payments  CASCADE",
// 		"DROP TABLE IF EXISTS bookings  CASCADE",
// 		"DROP TABLE IF EXISTS events    CASCADE",
// 		"DROP TABLE IF EXISTS categories CASCADE",
// 		"DROP TABLE IF EXISTS organizers CASCADE",
// 		"DROP TABLE IF EXISTS refresh_tokens CASCADE",
// 		"DROP TABLE IF EXISTS users     CASCADE",
// 	}
// 	for _, s := range drops {
// 		if _, err := pool.Exec(ctx, s); err != nil {
// 			t.Logf("drop warning: %v", err)
// 		}
// 	}

// 	stmts := []string{
// 		// Enums
// 		`DO $$ BEGIN CREATE TYPE event_status AS ENUM ('draft','published','cancelled','completed');
// 		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
// 		`DO $$ BEGIN CREATE TYPE booking_status_type AS ENUM ('pending','confirmed','cancelled','refunded');
// 		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
// 		`DO $$ BEGIN CREATE TYPE payment_status_type AS ENUM ('pending','processing','completed','failed','refunded');
// 		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
// 		`DO $$ BEGIN CREATE TYPE payment_method_type AS ENUM ('credit_card','paypal','apple_pay','google_pay','samsung_pay','instant_eft');
// 		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

// 		// Users
// 		`CREATE TABLE users (
// 			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			email      VARCHAR(255) UNIQUE NOT NULL,
// 			password   TEXT NOT NULL,
// 			role       VARCHAR(50) NOT NULL DEFAULT 'user',
// 			created_at TIMESTAMPTZ DEFAULT NOW(),
// 			updated_at TIMESTAMPTZ DEFAULT NOW()
// 		)`,

// 		// Refresh tokens
// 		`CREATE TABLE refresh_tokens (
// 			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
// 			token_hash TEXT UNIQUE NOT NULL,
// 			expires_at TIMESTAMPTZ NOT NULL,
// 			revoked    BOOLEAN NOT NULL DEFAULT FALSE,
// 			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
// 		)`,

// 		// Categories — SERIAL INTEGER to match real schema
// 		`CREATE TABLE categories (
// 			id           SERIAL PRIMARY KEY,
// 			name         VARCHAR(100) UNIQUE NOT NULL,
// 			slug         VARCHAR(100) UNIQUE NOT NULL,
// 			description  TEXT,
// 			image_url    TEXT,
// 			display_order INTEGER DEFAULT 0,
// 			is_active    BOOLEAN DEFAULT TRUE,
// 			created_at   TIMESTAMPTZ DEFAULT NOW()
// 		)`,

// 		// Organizers
// 		`CREATE TABLE organizers (
// 			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
// 			name       VARCHAR(255) NOT NULL,
// 			created_at TIMESTAMPTZ DEFAULT NOW()
// 		)`,

// 		// Events — matches all columns queried by GetEventByID and GetUserBookings
// 		`CREATE TABLE events (
// 			id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			title               VARCHAR(255) NOT NULL,
// 			description         TEXT NOT NULL DEFAULT '',
// 			cover_image_url     TEXT,
// 			banner_image_url    TEXT,
// 			gallery_images      JSONB,
// 			start_time          TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
// 			end_time            TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '8 days',
// 			expires_at          TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
// 			timezone            VARCHAR(50) NOT NULL DEFAULT 'SAST',
// 			venue_name          VARCHAR(255) NOT NULL DEFAULT '',
// 			address             TEXT NOT NULL DEFAULT '',
// 			city                VARCHAR(255) NOT NULL DEFAULT '',
// 			latitude            DECIMAL(10,8),
// 			longitude           DECIMAL(11,8),
// 			category_id         INTEGER NOT NULL REFERENCES categories(id),
// 			tags                TEXT[],
// 			price               DECIMAL(10,2) NOT NULL DEFAULT 0,
// 			currency            VARCHAR(3) NOT NULL DEFAULT 'ZAR',
// 			max_capacity        INTEGER,
// 			tickets_sold        INTEGER NOT NULL DEFAULT 0,
// 			allow_waitlist      BOOLEAN DEFAULT TRUE,
// 			status              VARCHAR(50) NOT NULL DEFAULT 'published',
// 			is_featured         BOOLEAN DEFAULT FALSE,
// 			featured_until      TIMESTAMPTZ,
// 			published_at        TIMESTAMPTZ,
// 			cancelled_at        TIMESTAMPTZ,
// 			cancellation_reason TEXT,
// 			refund_policy       TEXT,
// 			terms_and_conditions TEXT,
// 			organizer_id        UUID REFERENCES users(id),
// 			views               BIGINT DEFAULT 0,
// 			search_vector       tsvector,
// 			created_at          TIMESTAMPTZ DEFAULT NOW(),
// 			updated_at          TIMESTAMPTZ DEFAULT NOW(),
// 			CONSTRAINT chk_tickets CHECK (tickets_sold >= 0),
// 			CONSTRAINT chk_end_after_start CHECK (end_time > start_time)
// 		)`,

// 		// Bookings
// 		`CREATE TABLE bookings (
// 			id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			booking_number       VARCHAR(50) UNIQUE NOT NULL DEFAULT 'BK-' || substr(gen_random_uuid()::text,1,8),
// 			event_id             UUID NOT NULL REFERENCES events(id),
// 			user_id              UUID REFERENCES users(id),
// 			is_guest             BOOLEAN NOT NULL DEFAULT FALSE,
// 			ticket_quantity      INT NOT NULL,
// 			unit_price           NUMERIC(10,2) NOT NULL,
// 			subtotal             NUMERIC(10,2) NOT NULL,
// 			platform_fee         NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			organizer_fee        NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			total_amount         NUMERIC(10,2) NOT NULL,
// 			currency             VARCHAR(10) NOT NULL DEFAULT 'ZAR',
// 			attendee_first_name  VARCHAR(100),
// 			attendee_last_name   VARCHAR(100),
// 			attendee_email       VARCHAR(255),
// 			attendee_phone       VARCHAR(50),
// 			booking_status       VARCHAR(50) NOT NULL DEFAULT 'pending',
// 			payment_status       VARCHAR(50) NOT NULL DEFAULT 'pending',
// 			tickets_sent         BOOLEAN DEFAULT FALSE,
// 			qr_code_data         TEXT,
// 			qr_code_url          TEXT,
// 			special_requirements TEXT,
// 			promo_code           VARCHAR(100),
// 			discount_amount      NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			refunded_request_at  TIMESTAMPTZ,
// 			refunded_reason      TEXT,
// 			refunded_at          TIMESTAMPTZ,
// 			refunded_amount      NUMERIC(10,2),
// 			booking_notes        TEXT,
// 			confirmed_at         TIMESTAMPTZ,
// 			cancelled_at         TIMESTAMPTZ,
// 			created_at           TIMESTAMPTZ DEFAULT NOW(),
// 			updated_at           TIMESTAMPTZ DEFAULT NOW()
// 		)`,

// 		// Payments
// 		`CREATE TABLE payments (
// 			id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			booking_id                UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
// 			payment_gateway           VARCHAR(50) NOT NULL DEFAULT 'stitch',
// 			payment_intent_id         TEXT,
// 			transaction_id            TEXT,
// 			amount                    NUMERIC(10,2) NOT NULL,
// 			currency                  VARCHAR(10) NOT NULL DEFAULT 'ZAR',
// 			payment_method            payment_method_type NOT NULL DEFAULT 'credit_card',
// 			status                    VARCHAR(50) NOT NULL DEFAULT 'pending',
// 			stitch_payment_request_id TEXT,
// 			stitch_payment_url        TEXT,
// 			refund_id                 TEXT,
// 			refunded_amount           NUMERIC(10,2),
// 			refunded_at               TIMESTAMPTZ,
// 			webhook_received          BOOLEAN DEFAULT FALSE,
// 			completed_at              TIMESTAMPTZ,
// 			created_at                TIMESTAMPTZ DEFAULT NOW(),
// 			updated_at                TIMESTAMPTZ DEFAULT NOW(),
// 			UNIQUE(booking_id)
// 		)`,
// 	}

// 	for _, s := range stmts {
// 		if _, err := pool.Exec(ctx, s); err != nil {
// 			t.Fatalf("booking table create failed: %v\nSQL: %s", err, s)
// 		}
// 	}
// }

// func cleanBookingTables(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	ctx := context.Background()
// 	pool.Exec(ctx, "TRUNCATE payments    CASCADE")
// 	pool.Exec(ctx, "TRUNCATE bookings    CASCADE")
// 	pool.Exec(ctx, "TRUNCATE events      CASCADE")
// 	pool.Exec(ctx, "TRUNCATE categories  CASCADE")
// 	pool.Exec(ctx, "TRUNCATE organizers  CASCADE")
// 	pool.Exec(ctx, "TRUNCATE refresh_tokens CASCADE")
// 	pool.Exec(ctx, "TRUNCATE users       CASCADE")
// }

// // seedCategory inserts a category row and returns its INTEGER id.
// func seedCategory(t *testing.T, pool *pgxpool.Pool) int {
// 	t.Helper()
// 	var id int
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO categories (name, slug)
// 		VALUES ('Test', 'test-'||substr(gen_random_uuid()::text,1,8))
// 		RETURNING id
// 	`).Scan(&id)
// 	require.NoError(t, err)
// 	return id
// }

// // ─── seed helpers ─────────────────────────────────────────────────────────────

// // seedUser inserts a user and returns (userID, accessToken).
// func seedUser(t *testing.T, pool *pgxpool.Pool) (userID, accessToken string) {
// 	t.Helper()
// 	cfg := authTestConfig()
// 	email := randomEmail()
// 	hashed, _ := bcrypt.GenerateFromPassword([]byte("Pass1234!"), bcrypt.DefaultCost)
// 	u := &models.User{Email: email, Password: string(hashed), Role: "user"}
// 	created, err := repository.CreateUser(pool, u)
// 	require.NoError(t, err)
// 	userID = created.ID

// 	r := buildAuthRouter(pool, cfg)
// 	w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
// 		"email": email, "password": "Pass1234!",
// 	})
// 	require.Equal(t, http.StatusOK, w.Code)
// 	var resp map[string]interface{}
// 	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 	accessToken = resp["access_token"].(string)
// 	return
// }

// // seedPublishedEvent creates a published event and returns its ID.
// func seedPublishedEvent(t *testing.T, pool *pgxpool.Pool) string {
// 	t.Helper()
// 	categoryID := seedCategory(t, pool)
// 	// organizer_id must be non-NULL: OrganizerID is a non-pointer string in models.Event.
// 	organizerID, _ := seedUser(t, pool)
// 	var eventID string
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO events (
// 			title, description, status, price, currency,
// 			max_capacity, tickets_sold, category_id,
// 			venue_name, address, city,
// 			organizer_id,
// 			start_time, end_time, expires_at
// 		)
// 		VALUES (
// 			'Test Concert', '', 'published', 50.00, 'ZAR',
// 			100, 0, $1,
// 			'Test Venue', '123 Main St', 'Cape Town',
// 			$2,
// 			NOW() + INTERVAL '7 days',
// 			NOW() + INTERVAL '8 days',
// 			NOW() + INTERVAL '30 days'
// 		)
// 		RETURNING id
// 	`, categoryID, organizerID).Scan(&eventID)
// 	require.NoError(t, err)
// 	return eventID
// }

// // seedCompletedBooking creates a confirmed booking with a completed payment row.
// func seedCompletedBooking(t *testing.T, pool *pgxpool.Pool, userID, eventID string) string {
// 	t.Helper()
// 	var bookingID string
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO bookings (event_id, user_id, ticket_quantity, unit_price, subtotal,
// 		                      total_amount, currency, booking_status, payment_status)
// 		VALUES ($1, $2, 1, 50.00, 50.00, 52.50, 'ZAR', 'confirmed', 'completed')
// 		RETURNING id
// 	`, eventID, userID).Scan(&bookingID)
// 	require.NoError(t, err)

// 	_, err = pool.Exec(context.Background(), `
// 		INSERT INTO payments (booking_id, payment_gateway, amount, currency, payment_method, status,
// 		                      stitch_payment_request_id, webhook_received)
// 		VALUES ($1, 'stitch', 52.50, 'ZAR', 'credit_card', 'completed', 'req_completed_test', TRUE)
// 	`, bookingID)
// 	require.NoError(t, err)
// 	return bookingID
// }

// // ─── router builder ───────────────────────────────────────────────────────────

// // buildBookingRouter assembles the full booking + auth router.
// //
// // PaymentService now requires a jobChan. We pass a buffered channel so the
// // handler can enqueue booking jobs without blocking. Tests that need to assert
// // on enqueued jobs should construct their own channel via services.NewPaymentService.
// func buildBookingRouter(pool *pgxpool.Pool) *gin.Engine {
// 	gin.SetMode(gin.TestMode)
// 	cfg := authTestConfig()

// 	// Buffered job channel — the worker is not started in tests; the channel
// 	// simply absorbs any enqueued jobs so the webhook handler never blocks.
// 	jobChan := make(chan string, 256)
// 	paymentSvc := services.NewPaymentService(pool, cfg, jobChan)

// 	r := gin.New()

// 	// Auth routes (needed to obtain tokens)
// 	auth := r.Group("/auth")
// 	auth.POST("/login", handlers.LoginHandler(pool, cfg))

// 	// Booking routes (auth required)
// 	bookings := r.Group("/bookings")
// 	bookings.Use(middleware.AuthMiddleware(cfg))
// 	bookings.POST("", handlers.CreateBookingHandler(pool, paymentSvc))
// 	bookings.GET("", handlers.GetUserBookingsHandler(pool))
// 	bookings.GET("/:id", handlers.GetBookingDetailsHandler(pool))
// 	bookings.POST("/:id/refund", handlers.RequestRefundHandler(pool,paymentSvc))
// 	bookings.POST("/:id/payment", handlers.CreateStitchPayment(pool, paymentSvc))
// 	bookings.GET("/:id/payment", handlers.GetPaymentStatusHandler(pool, paymentSvc))
// 	bookings.POST("/:id/payment/retry", handlers.RetryPaymentHandler(pool, paymentSvc))

// 	// Guest routes (no auth)
// 	guest := r.Group("/guest")
// 	guest.POST("/bookings", handlers.CreateGuestBookingHandler(pool, paymentSvc))

// 	return r
// }

// // ─── CreateBooking tests ──────────────────────────────────────────────────────

// func TestBookingCreate(t *testing.T) {
// 	pool := setupBookingTestDB(t)
// 	defer pool.Close()

// 	t.Run("success — authenticated user creates booking and gets client_token", func(t *testing.T) {
// 		// This test calls Stitch. Without a mock server it will return 500.
// 		// To run: change const→var for the URL block in payment_service.go,
// 		// then call pointServiceAtMock() to redirect to a local mock.
// 		t.Skip("Requires Stitch mock server — see payment_service_test.go for setup")
// 		cleanBookingTables(t, pool)
// 		_, token := seedUser(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
// 			"event_id": eventID, "ticket_quantity": 2,
// 		}, token)
// 		assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		assert.NotEmpty(t, resp["booking_id"])
// 		assert.NotEmpty(t, resp["booking_number"])
// 		assert.NotEmpty(t, resp["client_token"])
// 	})

// 	t.Run("unauthenticated request returns 401", func(t *testing.T) {
// 		r := buildBookingRouter(pool)
// 		w := doRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
// 			"event_id": "00000000-0000-0000-0000-000000000001", "ticket_quantity": 1,
// 		})
// 		assert.Equal(t, http.StatusUnauthorized, w.Code)
// 	})

// 	t.Run("non-existent event returns 404", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		_, token := seedUser(t, pool)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
// 			"event_id": "00000000-0000-0000-0000-000000000099", "ticket_quantity": 1,
// 		}, token)
// 		assert.Equal(t, http.StatusNotFound, w.Code)
// 	})

// 	t.Run("ticket_quantity=0 returns 400 validation error", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		_, token := seedUser(t, pool)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
// 			"event_id": "00000000-0000-0000-0000-000000000001", "ticket_quantity": 0,
// 		}, token)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("ticket_quantity exceeds max (>10) returns 400", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		_, token := seedUser(t, pool)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
// 			"event_id": "00000000-0000-0000-0000-000000000001", "ticket_quantity": 11,
// 		}, token)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("sold-out event returns 409", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		organizerID, token := seedUser(t, pool)
// 		categoryID := seedCategory(t, pool)
// 		var fullEventID string
// 		err := pool.QueryRow(context.Background(), `
// 			INSERT INTO events (
// 				title, description, status, price, currency,
// 				max_capacity, tickets_sold, category_id,
// 				venue_name, address, city,
// 				organizer_id,
// 				start_time, end_time, expires_at
// 			)
// 			VALUES (
// 				'Sold Out Show', '', 'published', 100.00, 'ZAR',
// 				1, 1, $1,
// 				'Test Venue', '1 Main St', 'Cape Town',
// 				$2,
// 				NOW()+INTERVAL '7 days',
// 				NOW()+INTERVAL '8 days',
// 				NOW()+INTERVAL '30 days'
// 			)
// 			RETURNING id
// 		`, categoryID, organizerID).Scan(&fullEventID)
// 		require.NoError(t, err)

// 		r := buildBookingRouter(pool)
// 		w := doAuthRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
// 			"event_id": fullEventID, "ticket_quantity": 1,
// 		}, token)
// 		assert.Equal(t, http.StatusConflict, w.Code)
// 	})
// }

// // ─── GetUserBookings tests ────────────────────────────────────────────────────

// func TestBookingGetUserBookings(t *testing.T) {
// 	pool := setupBookingTestDB(t)
// 	defer pool.Close()

// 	t.Run("returns paginated list of user's bookings", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		userID, token := seedUser(t, pool)
// 		eventID := seedPublishedEvent(t, pool)

// 		for i := 0; i < 2; i++ {
// 			pool.Exec(context.Background(), `
// 				INSERT INTO bookings (event_id, user_id, ticket_quantity, unit_price, subtotal,
// 				                      total_amount, currency, booking_status, payment_status)
// 				VALUES ($1, $2, 1, 50.00, 50.00, 52.50, 'ZAR', 'confirmed', 'completed')
// 			`, eventID, userID)
// 		}

// 		r := buildBookingRouter(pool)
// 		w := doAuthRequest(r, http.MethodGet, "/bookings", nil, token)
// 		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		require.NotNil(t, resp["data"])
// 		data := resp["data"].([]interface{})
// 		assert.GreaterOrEqual(t, len(data), 2)
// 	})

// 	t.Run("empty list when user has no bookings", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		_, token := seedUser(t, pool)
// 		r := buildBookingRouter(pool)
// 		w := doAuthRequest(r, http.MethodGet, "/bookings", nil, token)
// 		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		if resp["data"] != nil {
// 			data := resp["data"].([]interface{})
// 			assert.Len(t, data, 0)
// 		}
// 	})

// 	t.Run("unauthenticated request returns 401", func(t *testing.T) {
// 		r := buildBookingRouter(pool)
// 		w := doRequest(r, http.MethodGet, "/bookings", nil)
// 		assert.Equal(t, http.StatusUnauthorized, w.Code)
// 	})
// }

// // ─── GetBookingDetails tests ──────────────────────────────────────────────────

// func TestBookingGetDetails(t *testing.T) {
// 	pool := setupBookingTestDB(t)
// 	defer pool.Close()

// 	t.Run("owner can fetch their booking", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		userID, token := seedUser(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		bookingID := seedCompletedBooking(t, pool, userID, eventID)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodGet, "/bookings/"+bookingID, nil, token)
// 		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		assert.Equal(t, bookingID, resp["id"])
// 	})

// 	t.Run("another user cannot see someone else's booking (returns 404)", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		userID, _ := seedUser(t, pool)
// 		_, otherToken := seedUser(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		bookingID := seedCompletedBooking(t, pool, userID, eventID)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodGet, "/bookings/"+bookingID, nil, otherToken)
// 		assert.Equal(t, http.StatusNotFound, w.Code)
// 	})

// 	t.Run("invalid UUID returns 400", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		_, token := seedUser(t, pool)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodGet, "/bookings/not-a-uuid", nil, token)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("non-existent booking UUID returns 404", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		_, token := seedUser(t, pool)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodGet, "/bookings/00000000-0000-0000-0000-000000000000", nil, token)
// 		assert.Equal(t, http.StatusNotFound, w.Code)
// 	})
// }

// // ─── RequestRefund tests ──────────────────────────────────────────────────────

// func TestBookingRequestRefund(t *testing.T) {
// 	pool := setupBookingTestDB(t)
// 	defer pool.Close()

// 	t.Run("refund reason too short returns 400", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		userID, token := seedUser(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		bookingID := seedCompletedBooking(t, pool, userID, eventID)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodPost, "/bookings/"+bookingID+"/refund", map[string]string{
// 			"reason": "too short",
// 		}, token)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("missing reason returns 400", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		userID, token := seedUser(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		bookingID := seedCompletedBooking(t, pool, userID, eventID)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodPost, "/bookings/"+bookingID+"/refund", map[string]string{}, token)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("another user cannot refund someone else's booking", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		userID, _ := seedUser(t, pool)
// 		_, otherToken := seedUser(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		bookingID := seedCompletedBooking(t, pool, userID, eventID)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodPost, "/bookings/"+bookingID+"/refund", map[string]string{
// 			"reason": "I want a refund because the event was cancelled",
// 		}, otherToken)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})
// }

// // ─── Payment flow tests ───────────────────────────────────────────────────────

// func TestBookingPaymentFlow(t *testing.T) {
// 	pool := setupBookingTestDB(t)
// 	defer pool.Close()

// 	t.Run("get payment status for owned booking", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		userID, token := seedUser(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		bookingID := seedCompletedBooking(t, pool, userID, eventID)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodGet, "/bookings/"+bookingID+"/payment", nil, token)
// 		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		assert.Equal(t, "completed", resp["payment_status"])
// 	})

// 	t.Run("payment status for another user's booking returns 404", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		userID, _ := seedUser(t, pool)
// 		_, otherToken := seedUser(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		bookingID := seedCompletedBooking(t, pool, userID, eventID)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodGet, "/bookings/"+bookingID+"/payment", nil, otherToken)
// 		assert.Equal(t, http.StatusNotFound, w.Code)
// 	})

// 	t.Run("retry on already-completed payment returns 409", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		userID, token := seedUser(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		bookingID := seedCompletedBooking(t, pool, userID, eventID)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodPost, "/bookings/"+bookingID+"/payment/retry", nil, token)
// 		assert.Equal(t, http.StatusConflict, w.Code)
// 	})

// 	t.Run("payment on invalid UUID returns 400", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		_, token := seedUser(t, pool)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodPost, "/bookings/not-a-uuid/payment", nil, token)
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("payment on non-existent booking returns 404", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		_, token := seedUser(t, pool)
// 		r := buildBookingRouter(pool)

// 		w := doAuthRequest(r, http.MethodPost, "/bookings/00000000-0000-0000-0000-000000000000/payment", nil, token)
// 		assert.Equal(t, http.StatusNotFound, w.Code)
// 	})
// }

// // ─── Guest booking tests ──────────────────────────────────────────────────────

// func TestBookingGuest(t *testing.T) {
// 	pool := setupBookingTestDB(t)
// 	defer pool.Close()

// 	t.Run("guest booking created successfully without auth", func(t *testing.T) {
// 		// CreateGuestBookingHandler calls paymentService.CreatePaymentIntent.
// 		// Without a Stitch mock server this returns 500.
// 		// To run: follow const→var instructions in payment_service_test.go,
// 		// then point stitchTokenURL / stitchGraphQLURL at a local mock server.
// 		t.Skip("Requires Stitch mock server — payment intent cannot be created without one")
// 		cleanBookingTables(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		r := buildBookingRouter(pool)

// 		w := doRequest(r, http.MethodPost, "/guest/bookings", map[string]interface{}{
// 			"event_id":            eventID,
// 			"ticket_quantity":     1,
// 			"attendee_first_name": "Jane",
// 			"attendee_last_name":  "Doe",
// 			"attendee_email":      "jane@guest.com",
// 		})
// 		assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		assert.NotEmpty(t, resp["booking_id"])
// 		assert.True(t, resp["is_guest"].(bool))
// 	})

// 	t.Run("guest booking missing attendee_email returns 400", func(t *testing.T) {
// 		cleanBookingTables(t, pool)
// 		eventID := seedPublishedEvent(t, pool)
// 		r := buildBookingRouter(pool)

// 		w := doRequest(r, http.MethodPost, "/guest/bookings", map[string]interface{}{
// 			"event_id":            eventID,
// 			"ticket_quantity":     1,
// 			"attendee_first_name": "Jane",
// 			"attendee_last_name":  "Doe",
// 		})
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("guest booking with invalid email returns 400", func(t *testing.T) {
// 		r := buildBookingRouter(pool)
// 		w := doRequest(r, http.MethodPost, "/guest/bookings", map[string]interface{}{
// 			"event_id":            "00000000-0000-0000-0000-000000000001",
// 			"ticket_quantity":     1,
// 			"attendee_first_name": "Jane",
// 			"attendee_last_name":  "Doe",
// 			"attendee_email":      "not-an-email",
// 		})
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("guest booking missing attendee_first_name returns 400", func(t *testing.T) {
// 		r := buildBookingRouter(pool)
// 		w := doRequest(r, http.MethodPost, "/guest/bookings", map[string]interface{}{
// 			"event_id":           "00000000-0000-0000-0000-000000000001",
// 			"ticket_quantity":    1,
// 			"attendee_last_name": "Doe",
// 			"attendee_email":     "jane@guest.com",
// 		})
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})
// }