package handlers_test

// =============================================================================
// BOOKING FLOW TESTS
// =============================================================================
//
// Tests covered:
//   - POST /bookings                                  → CreateBookingHandler
//   - GET  /bookings                                  → GetUserBookingsHandler
//   - GET  /bookings/:id                              → GetBookingDetailsHandler
//   - POST /bookings/:id/refund                       → RequestRefundHandler (cascade)
//   - POST /bookings/:id/payment                      → CreateStitchPayment
//   - GET  /bookings/:id/payment                      → GetPaymentStatusHandler
//   - POST /bookings/:id/payment/retry                → RetryPaymentHandler
//   - GET  /bookings/:id/assignments                  → GetTicketAssignmentsHandler
//   - POST /bookings/:id/assignments/:assignmentId    → AssignTicketHandler
//   - DELETE /bookings/:id/assignments/:assignmentId  → RevokeAssignmentHandler
//   - POST /tickets/claim                             → ClaimTicketHandler (public)
//   - POST /guest/bookings                            → CreateGuestBookingHandler (no auth)
//
// Prerequisites: same Docker Postgres as auth_flow_test.go.
// Run:  go test ./internal/handlers/... -v -run TestBooking
// =============================================================================

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"events-db/internal/handlers"
	"events-db/internal/middleware"
	"events-db/internal/models"
	"events-db/internal/repository"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// ─── DB setup ─────────────────────────────────────────────────────────────────

func setupBookingTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Fatalf("Cannot connect to test DB: %v", err)
	}
	createBookingTestTables(t, pool)
	cleanBookingTables(t, pool)
	return pool
}

func createBookingTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
		"DROP TABLE IF EXISTS ticket_assignments CASCADE",
		"DROP TABLE IF EXISTS payments           CASCADE",
		"DROP TABLE IF EXISTS bookings           CASCADE",
		"DROP TABLE IF EXISTS events             CASCADE",
		"DROP TABLE IF EXISTS categories         CASCADE",
		"DROP TABLE IF EXISTS organizers         CASCADE",
		"DROP TABLE IF EXISTS refresh_tokens     CASCADE",
		"DROP TABLE IF EXISTS users              CASCADE",
		"DROP TYPE  IF EXISTS assignment_status",
	}
	for _, s := range drops {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		// Enums
		`DO $$ BEGIN CREATE TYPE event_status AS ENUM ('draft','published','cancelled','completed');
		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
		`DO $$ BEGIN CREATE TYPE booking_status_type AS ENUM ('pending','confirmed','cancelled','refunded');
		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
		`DO $$ BEGIN CREATE TYPE payment_status_type AS ENUM ('pending','processing','completed','failed','refunded');
		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
		`DO $$ BEGIN CREATE TYPE payment_method_type AS ENUM ('credit_card','paypal','apple_pay','google_pay','samsung_pay','instant_eft');
		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
		`DO $$ BEGIN CREATE TYPE assignment_status AS ENUM ('unassigned','pending','claimed','revoked','cancelled');
		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		// Users
		`CREATE TABLE users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email      VARCHAR(255) UNIQUE NOT NULL,
			password   TEXT NOT NULL DEFAULT '',
			first_name VARCHAR(100),
			last_name  VARCHAR(100),
			role       VARCHAR(50) NOT NULL DEFAULT 'attendee',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Refresh tokens
		`CREATE TABLE refresh_tokens (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT UNIQUE NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			revoked    BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		// Categories
		`CREATE TABLE categories (
			id           SERIAL PRIMARY KEY,
			name         VARCHAR(100) UNIQUE NOT NULL,
			slug         VARCHAR(100) UNIQUE NOT NULL,
			display_order INTEGER DEFAULT 0,
			is_active    BOOLEAN DEFAULT TRUE,
			created_at   TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Organizers
		`CREATE TABLE organizers (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name       VARCHAR(255) NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Events
		`CREATE TABLE events (
			id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title               VARCHAR(255) NOT NULL,
			description         TEXT NOT NULL DEFAULT '',
			start_time          TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
			end_time            TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '8 days',
			expires_at          TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
			timezone            VARCHAR(50) NOT NULL DEFAULT 'SAST',
			venue_name          VARCHAR(255) NOT NULL DEFAULT '',
			address             TEXT NOT NULL DEFAULT '',
			city                VARCHAR(255) NOT NULL DEFAULT '',
			category_id         INTEGER NOT NULL REFERENCES categories(id),
			price               DECIMAL(10,2) NOT NULL DEFAULT 0,
			currency            VARCHAR(3) NOT NULL DEFAULT 'ZAR',
			max_capacity        INTEGER,
			tickets_sold        INTEGER NOT NULL DEFAULT 0,
			allow_waitlist      BOOLEAN DEFAULT TRUE,
			status              VARCHAR(50) NOT NULL DEFAULT 'published',
			is_featured         BOOLEAN DEFAULT FALSE,
			organizer_id        UUID REFERENCES users(id),
			views               BIGINT DEFAULT 0,
			created_at          TIMESTAMPTZ DEFAULT NOW(),
			updated_at          TIMESTAMPTZ DEFAULT NOW(),
			CONSTRAINT chk_tickets CHECK (tickets_sold >= 0)
		)`,

		// Bookings
		`CREATE TABLE bookings (
			id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_number       VARCHAR(50) UNIQUE NOT NULL DEFAULT 'BK-' || substr(gen_random_uuid()::text,1,8),
			event_id             UUID NOT NULL REFERENCES events(id),
			user_id              UUID REFERENCES users(id),
			is_guest             BOOLEAN NOT NULL DEFAULT FALSE,
			ticket_quantity      INT NOT NULL,
			unit_price           NUMERIC(10,2) NOT NULL,
			subtotal             NUMERIC(10,2) NOT NULL,
			platform_fee         NUMERIC(10,2) NOT NULL DEFAULT 0,
			organizer_fee        NUMERIC(10,2) NOT NULL DEFAULT 0,
			total_amount         NUMERIC(10,2) NOT NULL,
			currency             VARCHAR(10) NOT NULL DEFAULT 'ZAR',
			attendee_first_name  VARCHAR(100),
			attendee_last_name   VARCHAR(100),
			attendee_email       VARCHAR(255),
			attendee_phone       VARCHAR(50),
			booking_status       VARCHAR(50) NOT NULL DEFAULT 'pending',
			payment_status       VARCHAR(50) NOT NULL DEFAULT 'pending',
			tickets_sent         BOOLEAN DEFAULT FALSE,
			qr_code_data         TEXT,
			qr_code_url          TEXT,
			special_requirements TEXT,
			promo_code           VARCHAR(100),
			discount_amount      NUMERIC(10,2) NOT NULL DEFAULT 0,
			refunded_request_at  TIMESTAMPTZ,
			refunded_reason      TEXT,
			refunded_at          TIMESTAMPTZ,
			refunded_amount      NUMERIC(10,2),
			booking_notes        TEXT,
			confirmed_at         TIMESTAMPTZ,
			cancelled_at         TIMESTAMPTZ,
			created_at           TIMESTAMPTZ DEFAULT NOW(),
			updated_at           TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Ticket assignments — one row per individual ticket slot
		`CREATE TABLE ticket_assignments (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_id     UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
			slot_number    INTEGER NOT NULL CHECK (slot_number >= 1),
			status         assignment_status NOT NULL DEFAULT 'unassigned',
			assignee_name  VARCHAR(200),
			assignee_email VARCHAR(255),
			claim_token    UUID UNIQUE,
			assigned_at    TIMESTAMPTZ,
			expires_at     TIMESTAMPTZ,
			claimed_at     TIMESTAMPTZ,
			revoked_at     TIMESTAMPTZ,
			qr_code_data   TEXT,
			qr_code_url    TEXT,
			created_at     TIMESTAMPTZ DEFAULT NOW(),
			updated_at     TIMESTAMPTZ DEFAULT NOW(),
			CONSTRAINT uq_booking_slot UNIQUE (booking_id, slot_number)
		)`,

		// Payments
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
			refund_id                 TEXT,
			refunded_amount           NUMERIC(10,2),
			refunded_at               TIMESTAMPTZ,
			webhook_received          BOOLEAN DEFAULT FALSE,
			completed_at              TIMESTAMPTZ,
			created_at                TIMESTAMPTZ DEFAULT NOW(),
			updated_at                TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(booking_id)
		)`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("booking table create failed: %v\nSQL: %s", err, s)
		}
	}
}

func cleanBookingTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE ticket_assignments CASCADE")
	pool.Exec(ctx, "TRUNCATE payments           CASCADE")
	pool.Exec(ctx, "TRUNCATE bookings           CASCADE")
	pool.Exec(ctx, "TRUNCATE events             CASCADE")
	pool.Exec(ctx, "TRUNCATE categories         CASCADE")
	pool.Exec(ctx, "TRUNCATE organizers         CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens     CASCADE")
	pool.Exec(ctx, "TRUNCATE users              CASCADE")
}

// ─── seed helpers ─────────────────────────────────────────────────────────────

func seedCategory(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO categories (name, slug)
		VALUES ('Test', 'test-'||substr(gen_random_uuid()::text,1,8))
		RETURNING id
	`).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedUser(t *testing.T, pool *pgxpool.Pool) (userID, accessToken string) {
	t.Helper()
	cfg := authTestConfig()
	email := randomEmail()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("Pass1234!"), bcrypt.DefaultCost)
	u := &models.User{Email: email, Password: string(hashed), Role: "attendee"}
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
	accessToken = resp["access_token"].(string)
	return
}

func seedPublishedEvent(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	categoryID := seedCategory(t, pool)
	organizerID, _ := seedUser(t, pool)
	var eventID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (
			title, description, status, price, currency,
			max_capacity, tickets_sold, category_id,
			venue_name, address, city, organizer_id,
			start_time, end_time, expires_at
		)
		VALUES (
			'Test Concert', '', 'published', 50.00, 'ZAR',
			100, 0, $1, 'Test Venue', '123 Main St', 'Cape Town', $2,
			NOW() + INTERVAL '7 days', NOW() + INTERVAL '8 days', NOW() + INTERVAL '30 days'
		)
		RETURNING id
	`, categoryID, organizerID).Scan(&eventID)
	require.NoError(t, err)
	return eventID
}

// seedConfirmedBooking creates a confirmed booking with a payment row.
// quantity controls ticket_quantity — used for multi-ticket assignment tests.
func seedConfirmedBooking(t *testing.T, pool *pgxpool.Pool, userID, eventID string, quantity int) string {
	t.Helper()
	if quantity < 1 {
		quantity = 1
	}
	unitPrice := 50.00
	subtotal := unitPrice * float64(quantity)
	total := subtotal + 2.50

	var bookingID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO bookings (
			event_id, user_id, ticket_quantity, unit_price, subtotal,
			total_amount, currency, booking_status, payment_status,
			attendee_first_name, attendee_last_name, attendee_email
		)
		VALUES ($1, $2, $3, $4, $5, $6, 'ZAR', 'confirmed', 'completed', 'Test', 'User', 'test@test.com')
		RETURNING id
	`, eventID, userID, quantity, unitPrice, subtotal, total).Scan(&bookingID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO payments (
			booking_id, payment_gateway, amount, currency,
			payment_method, status, stitch_payment_request_id, webhook_received
		)
		VALUES ($1, 'stitch', $2, 'ZAR', 'credit_card', 'completed', 'req_test', TRUE)
	`, bookingID, total)
	require.NoError(t, err)
	return bookingID
}

// seedAssignments inserts ticket_assignment rows for a confirmed booking.
// Slot 1 is pre-claimed by the buyer; the rest are unassigned.
func seedAssignments(t *testing.T, pool *pgxpool.Pool, bookingID string, quantity int) []string {
	t.Helper()
	var ids []string
	for slot := 1; slot <= quantity; slot++ {
		status := "unassigned"
		var claimedAt interface{} = nil
		buyerEmail := interface{}(nil)
		buyerName := interface{}(nil)
		if slot == 1 {
			status = "claimed"
			claimedAt = time.Now()
			email := "test@test.com"
			name := "Test User"
			buyerEmail = email
			buyerName = name
		}
		var id string
		err := pool.QueryRow(context.Background(), `
			INSERT INTO ticket_assignments
				(booking_id, slot_number, status, assignee_name, assignee_email, claimed_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		`, bookingID, slot, status, buyerName, buyerEmail, claimedAt).Scan(&id)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	return ids
}

// seedPendingAssignment puts slot 2 in pending state with a known claim token.
// Returns (assignmentID, claimToken).
func seedPendingAssignment(t *testing.T, pool *pgxpool.Pool, bookingID string) (string, string) {
	t.Helper()
	var id, token string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO ticket_assignments
			(booking_id, slot_number, status, assignee_name, assignee_email,
			 claim_token, assigned_at, expires_at)
		VALUES ($1, 2, 'pending', 'Alice Smith', 'alice@test.com',
		        gen_random_uuid(), NOW(), NOW() + INTERVAL '24 hours')
		RETURNING id, claim_token::text
	`, bookingID).Scan(&id, &token)
	require.NoError(t, err)
	return id, token
}

// ─── router builder ───────────────────────────────────────────────────────────

func buildBookingRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()

	jobChan := make(chan string, 256)
	paymentSvc := services.NewPaymentService(pool, cfg, jobChan)
	emailSvc := services.NewEmailService(cfg)

	r := gin.New()

	auth := r.Group("/auth")
	auth.POST("/login", handlers.LoginHandler(pool, cfg))

	// Authenticated booking routes
	bookings := r.Group("/bookings")
	bookings.Use(middleware.AuthMiddleware(cfg))
	bookings.POST("", handlers.CreateBookingHandler(pool, paymentSvc))
	bookings.GET("", handlers.GetUserBookingsHandler(pool))
	bookings.GET("/:id", handlers.GetBookingDetailsHandler(pool))
	bookings.POST("/:id/refund", handlers.RequestRefundHandler(pool, paymentSvc, emailSvc))
	bookings.POST("/:id/payment", handlers.CreateStitchPayment(pool, paymentSvc))
	bookings.GET("/:id/payment", handlers.GetPaymentStatusHandler(pool, paymentSvc))
	bookings.POST("/:id/payment/retry", handlers.RetryPaymentHandler(pool, paymentSvc))

	// Ticket assignment routes
	bookings.GET("/:id/assignments", handlers.GetTicketAssignmentsHandler(pool))
	// bookings.POST("/:id/assignments/:assignmentId", handlers.AssignTicketHandler(pool, emailSvc, cfg))
	bookings.DELETE("/:id/assignments/:assignmentId", handlers.RevokeAssignmentHandler(pool))

	// Public routes
	public := r.Group("")
	public.POST("/tickets/claim", handlers.ClaimTicketHandler(pool, emailSvc))
	public.POST("/guest/bookings", handlers.CreateGuestBookingHandler(pool, paymentSvc))

	return r
}

// ─── CreateBooking ────────────────────────────────────────────────────────────

func TestBookingCreate(t *testing.T) {
	pool := setupBookingTestDB(t)
	defer pool.Close()

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		r := buildBookingRouter(pool)
		w := doRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
			"event_id": "00000000-0000-0000-0000-000000000001", "ticket_quantity": 1,
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("non-existent event returns 404", func(t *testing.T) {
		cleanBookingTables(t, pool)
		_, token := seedUser(t, pool)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
			"event_id": "00000000-0000-0000-0000-000000000099", "ticket_quantity": 1,
		}, token)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("ticket_quantity=0 returns 400", func(t *testing.T) {
		cleanBookingTables(t, pool)
		_, token := seedUser(t, pool)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
			"event_id": "00000000-0000-0000-0000-000000000001", "ticket_quantity": 0,
		}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ticket_quantity > 10 returns 400", func(t *testing.T) {
		cleanBookingTables(t, pool)
		_, token := seedUser(t, pool)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
			"event_id": "00000000-0000-0000-0000-000000000001", "ticket_quantity": 11,
		}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("sold-out event returns 409", func(t *testing.T) {
		cleanBookingTables(t, pool)
		organizerID, token := seedUser(t, pool)
		categoryID := seedCategory(t, pool)
		var fullEventID string
		err := pool.QueryRow(context.Background(), `
			INSERT INTO events (
				title, description, status, price, currency,
				max_capacity, tickets_sold, category_id,
				venue_name, address, city, organizer_id,
				start_time, end_time, expires_at
			)
			VALUES (
				'Sold Out', '', 'published', 100.00, 'ZAR',
				1, 1, $1, 'Venue', '1 St', 'JHB', $2,
				NOW()+INTERVAL '7 days', NOW()+INTERVAL '8 days', NOW()+INTERVAL '30 days'
			)
			RETURNING id
		`, categoryID, organizerID).Scan(&fullEventID)
		require.NoError(t, err)

		r := buildBookingRouter(pool)
		w := doAuthRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
			"event_id": fullEventID, "ticket_quantity": 1,
		}, token)
		assert.Equal(t, http.StatusConflict, w.Code)
	})
}

// ─── GetUserBookings ──────────────────────────────────────────────────────────

func TestBookingGetUserBookings(t *testing.T) {
	pool := setupBookingTestDB(t)
	defer pool.Close()

	t.Run("returns list of user bookings", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)

		for i := 0; i < 2; i++ {
			seedConfirmedBooking(t, pool, userID, eventID, 1)
		}

		r := buildBookingRouter(pool)
		w := doAuthRequest(r, http.MethodGet, "/bookings", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.GreaterOrEqual(t, len(data), 2)
	})

	t.Run("empty list when user has no bookings", func(t *testing.T) {
		cleanBookingTables(t, pool)
		_, token := seedUser(t, pool)
		r := buildBookingRouter(pool)
		w := doAuthRequest(r, http.MethodGet, "/bookings", nil, token)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		if resp["data"] != nil {
			assert.Len(t, resp["data"].([]interface{}), 0)
		}
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildBookingRouter(pool)
		w := doRequest(r, http.MethodGet, "/bookings", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ─── GetBookingDetails ────────────────────────────────────────────────────────

func TestBookingGetDetails(t *testing.T) {
	pool := setupBookingTestDB(t)
	defer pool.Close()

	t.Run("owner can fetch their booking", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 1)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/bookings/"+bookingID, nil, token)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, bookingID, resp["id"])
	})

	t.Run("another user cannot see someone else's booking", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, _ := seedUser(t, pool)
		_, otherToken := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 1)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/bookings/"+bookingID, nil, otherToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid UUID returns 400", func(t *testing.T) {
		cleanBookingTables(t, pool)
		_, token := seedUser(t, pool)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/bookings/not-a-uuid", nil, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("non-existent booking returns 404", func(t *testing.T) {
		cleanBookingTables(t, pool)
		_, token := seedUser(t, pool)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/bookings/00000000-0000-0000-0000-000000000000", nil, token)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ─── RequestRefund (cascade) ──────────────────────────────────────────────────

func TestBookingRequestRefund(t *testing.T) {
	pool := setupBookingTestDB(t)
	defer pool.Close()

	t.Run("refund cascades and cancels all assignment slots", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 3)
		seedAssignments(t, pool, bookingID, 3)

		// Verify we start with mixed statuses
		var claimedCount int
		pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM ticket_assignments WHERE booking_id = $1 AND status = 'claimed'`,
			bookingID,
		).Scan(&claimedCount)
		assert.Equal(t, 1, claimedCount, "slot 1 should be pre-claimed")

		r := buildBookingRouter(pool)
		// Note: refund will attempt Stitch payment — it may 500 without a mock.
		// We assert the assignment cascade happened regardless.
		doAuthRequest(r, http.MethodPost, "/bookings/"+bookingID+"/refund", map[string]string{
			"reason": "Plans changed and I can no longer attend this event",
		}, token)

		// Regardless of the Stitch outcome, all assignment rows must be cancelled.
		var cancelledCount int
		pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM ticket_assignments WHERE booking_id = $1 AND status = 'cancelled'`,
			bookingID,
		).Scan(&cancelledCount)
		assert.Equal(t, 3, cancelledCount, "all 3 assignment slots must be cancelled after refund")
	})

	t.Run("refund reason too short returns 400", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 1)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/bookings/"+bookingID+"/refund", map[string]string{
			"reason": "too short",
		}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing reason returns 400", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 1)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/bookings/"+bookingID+"/refund", map[string]string{}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("another user cannot refund someone else's booking", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, _ := seedUser(t, pool)
		_, otherToken := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 1)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/bookings/"+bookingID+"/refund", map[string]string{
			"reason": "I want a refund because the event was cancelled unexpectedly",
		}, otherToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── Ticket Assignments ───────────────────────────────────────────────────────

func TestTicketAssignmentGetList(t *testing.T) {
	pool := setupBookingTestDB(t)
	defer pool.Close()

	t.Run("owner sees all assignment slots", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 3)
		seedAssignments(t, pool, bookingID, 3)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/bookings/"+bookingID+"/assignments", nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].([]interface{})
		assert.Len(t, data, 3)

		// Slot 1 should be claimed by buyer
		slot1 := data[0].(map[string]interface{})
		assert.Equal(t, float64(1), slot1["slot_number"])
		assert.Equal(t, "claimed", slot1["status"])
	})

	t.Run("another user cannot see someone else's assignments", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, _ := seedUser(t, pool)
		_, otherToken := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 2)
		seedAssignments(t, pool, bookingID, 2)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/bookings/"+bookingID+"/assignments", nil, otherToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		r := buildBookingRouter(pool)
		w := doRequest(r, http.MethodGet, "/bookings/00000000-0000-0000-0000-000000000001/assignments", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestTicketAssign(t *testing.T) {
	pool := setupBookingTestDB(t)
	defer pool.Close()

	t.Run("assigning an unassigned slot transitions it to pending", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 2)
		assignmentIDs := seedAssignments(t, pool, bookingID, 2)
		slot2ID := assignmentIDs[1] // slot 2 is unassigned
		r := buildBookingRouter(pool)

		w := doAuthRequest(r,
			http.MethodPost,
			"/bookings/"+bookingID+"/assignments/"+slot2ID,
			map[string]string{
				"assignee_email": "friend@example.com",
				"assignee_name":  "Friend Person",
			},
			token,
		)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// Verify DB state
		var status, email string
		err := pool.QueryRow(context.Background(),
			`SELECT status, assignee_email FROM ticket_assignments WHERE id = $1`, slot2ID,
		).Scan(&status, &email)
		require.NoError(t, err)
		assert.Equal(t, "pending", status)
		assert.Equal(t, "friend@example.com", email)
	})

	t.Run("invalid email returns 400", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 2)
		assignmentIDs := seedAssignments(t, pool, bookingID, 2)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r,
			http.MethodPost,
			"/bookings/"+bookingID+"/assignments/"+assignmentIDs[1],
			map[string]string{
				"assignee_email": "not-an-email",
				"assignee_name":  "Someone",
			},
			token,
		)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("assigning slot 1 (already claimed) returns 409", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 2)
		assignmentIDs := seedAssignments(t, pool, bookingID, 2)
		slot1ID := assignmentIDs[0]
		r := buildBookingRouter(pool)

		w := doAuthRequest(r,
			http.MethodPost,
			"/bookings/"+bookingID+"/assignments/"+slot1ID,
			map[string]string{
				"assignee_email": "someone@example.com",
				"assignee_name":  "Someone Else",
			},
			token,
		)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("another user cannot assign slots on someone else's booking", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, _ := seedUser(t, pool)
		_, otherToken := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 2)
		assignmentIDs := seedAssignments(t, pool, bookingID, 2)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r,
			http.MethodPost,
			"/bookings/"+bookingID+"/assignments/"+assignmentIDs[1],
			map[string]string{
				"assignee_email": "thief@example.com",
				"assignee_name":  "Ticket Thief",
			},
			otherToken,
		)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestTicketRevoke(t *testing.T) {
	pool := setupBookingTestDB(t)
	defer pool.Close()

	t.Run("owner can revoke a pending assignment", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 2)
		seedAssignments(t, pool, bookingID, 2)
		pendingID, _ := seedPendingAssignment(t, pool, bookingID)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r,
			http.MethodDelete,
			"/bookings/"+bookingID+"/assignments/"+pendingID,
			nil,
			token,
		)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var status string
		pool.QueryRow(context.Background(),
			`SELECT status FROM ticket_assignments WHERE id = $1`, pendingID,
		).Scan(&status)
		assert.Equal(t, "unassigned", status)
	})

	t.Run("revoking a claimed ticket returns 409", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 2)
		assignmentIDs := seedAssignments(t, pool, bookingID, 2)
		slot1ID := assignmentIDs[0] // already claimed
		r := buildBookingRouter(pool)

		w := doAuthRequest(r,
			http.MethodDelete,
			"/bookings/"+bookingID+"/assignments/"+slot1ID,
			nil,
			token,
		)
		assert.Equal(t, http.StatusConflict, w.Code)
	})
}

func TestTicketClaim(t *testing.T) {
	pool := setupBookingTestDB(t)
	defer pool.Close()

	t.Run("valid claim token successfully claims the ticket", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, _ := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 2)
		seedAssignments(t, pool, bookingID, 2)
		_, claimToken := seedPendingAssignment(t, pool, bookingID)
		r := buildBookingRouter(pool)

		w := doRequest(r, http.MethodPost, "/tickets/claim", map[string]string{
			"token":          claimToken,
			"claimant_name":  "Alice Smith",
			"claimant_email": "alice@test.com",
		})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// Verify the slot is now claimed in DB
		var status string
		pool.QueryRow(context.Background(),
			`SELECT status FROM ticket_assignments WHERE claim_token = $1`, claimToken,
		).Scan(&status)
		// After claim, claim_token is set to NULL — check by email instead
		pool.QueryRow(context.Background(),
			`SELECT status FROM ticket_assignments
			 WHERE booking_id = $1 AND assignee_email = 'alice@test.com'`,
			bookingID,
		).Scan(&status)
		assert.Equal(t, "claimed", status)
	})

	t.Run("invalid token returns 404", func(t *testing.T) {
		r := buildBookingRouter(pool)
		w := doRequest(r, http.MethodPost, "/tickets/claim", map[string]string{
			"token":          "00000000-0000-0000-0000-000000000000",
			"claimant_name":  "Nobody",
			"claimant_email": "nobody@test.com",
		})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("missing token returns 400", func(t *testing.T) {
		r := buildBookingRouter(pool)
		w := doRequest(r, http.MethodPost, "/tickets/claim", map[string]string{
			"claimant_name":  "Nobody",
			"claimant_email": "nobody@test.com",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("expired claim token returns 410 Gone", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, _ := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 2)
		seedAssignments(t, pool, bookingID, 2)

		var expiredToken string
		pool.QueryRow(context.Background(), `
			INSERT INTO ticket_assignments
				(booking_id, slot_number, status, assignee_name, assignee_email,
				 claim_token, assigned_at, expires_at)
			VALUES ($1, 3, 'pending', 'Expired User', 'expired@test.com',
			        gen_random_uuid(), NOW() - INTERVAL '25 hours', NOW() - INTERVAL '1 hour')
			RETURNING claim_token::text
		`, bookingID).Scan(&expiredToken)

		r := buildBookingRouter(pool)
		w := doRequest(r, http.MethodPost, "/tickets/claim", map[string]string{
			"token":          expiredToken,
			"claimant_name":  "Late Claimer",
			"claimant_email": "late@test.com",
		})
		assert.Equal(t, http.StatusGone, w.Code)
	})
}

// ─── Payment flow ─────────────────────────────────────────────────────────────

func TestBookingPaymentFlow(t *testing.T) {
	pool := setupBookingTestDB(t)
	defer pool.Close()

	t.Run("get payment status for owned booking", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 1)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/bookings/"+bookingID+"/payment", nil, token)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "completed", resp["payment_status"])
	})

	t.Run("payment status for another user's booking returns 404", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, _ := seedUser(t, pool)
		_, otherToken := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 1)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodGet, "/bookings/"+bookingID+"/payment", nil, otherToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("retry on already-completed payment returns 409", func(t *testing.T) {
		cleanBookingTables(t, pool)
		userID, token := seedUser(t, pool)
		eventID := seedPublishedEvent(t, pool)
		bookingID := seedConfirmedBooking(t, pool, userID, eventID, 1)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/bookings/"+bookingID+"/payment/retry", nil, token)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("payment on invalid UUID returns 400", func(t *testing.T) {
		cleanBookingTables(t, pool)
		_, token := seedUser(t, pool)
		r := buildBookingRouter(pool)

		w := doAuthRequest(r, http.MethodPost, "/bookings/not-a-uuid/payment", nil, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ─── Guest booking ────────────────────────────────────────────────────────────

func TestBookingGuest(t *testing.T) {
	pool := setupBookingTestDB(t)
	defer pool.Close()

	t.Run("guest booking missing attendee_email returns 400", func(t *testing.T) {
		cleanBookingTables(t, pool)
		eventID := seedPublishedEvent(t, pool)
		r := buildBookingRouter(pool)

		w := doRequest(r, http.MethodPost, "/guest/bookings", map[string]interface{}{
			"event_id": eventID, "ticket_quantity": 1,
			"attendee_first_name": "Jane", "attendee_last_name": "Doe",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("guest booking with invalid email returns 400", func(t *testing.T) {
		r := buildBookingRouter(pool)
		w := doRequest(r, http.MethodPost, "/guest/bookings", map[string]interface{}{
			"event_id": "00000000-0000-0000-0000-000000000001", "ticket_quantity": 1,
			"attendee_first_name": "Jane", "attendee_last_name": "Doe",
			"attendee_email": "not-an-email",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("guest booking missing first name returns 400", func(t *testing.T) {
		r := buildBookingRouter(pool)
		w := doRequest(r, http.MethodPost, "/guest/bookings", map[string]interface{}{
			"event_id": "00000000-0000-0000-0000-000000000001", "ticket_quantity": 1,
			"attendee_last_name": "Doe", "attendee_email": "jane@guest.com",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}