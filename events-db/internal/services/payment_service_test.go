 package services

// // Payment service tests.
// //
// // Prerequisites:
// //   docker run -d --name test-postgres \
// //     -e POSTGRES_USER=testuser \
// //     -e POSTGRES_PASSWORD=testpass \
// //     -e POSTGRES_DB=test_db \
// //     -p 5433:5432 postgres:15
// //
// // Run:
// //   go test ./internal/services/... -v -run TestPayment
// //
// // Changes from previous version
// // ─────────────────────────────
// // • createPaymentRecord now takes cartID *string as a 4th arg.
// //   All legacy callers pass nil; cart-based callers pass a cart UUID.
// // • payments and bookings DDL include cart_id.
// // • carts table added so the FK on bookings.cart_id resolves.
// // • New tests: CreateCartPaymentIntent, syncAllCartBookings,
// //   multi-booking webhook confirmation, guest user paths.
// // • createPaymentRecord paymentURL arg changed from string to *string.
// //   All test call sites updated to pass nil instead of "".

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"
// 	"time"

// 	"events-db/internal/config"
// 	"events-db/internal/models"

// 	"github.com/jackc/pgx/v5/pgxpool"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// )

// // ==========================================
// // Test Database Setup
// // ==========================================

// func setupPaymentTestDB(t *testing.T) *pgxpool.Pool {
// 	t.Helper()
// 	pool, err := pgxpool.New(context.Background(), "postgresql://testuser:testpass@localhost:5433/test_db")
// 	if err != nil {
// 		t.Skipf("payment DB tests skipped — could not create pool: %v", err)
// 	}
// 	if pingErr := pool.Ping(context.Background()); pingErr != nil {
// 		pool.Close()
// 		t.Skipf("payment DB tests skipped — DB not reachable (start with: docker run -d --name test-postgres -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass -e POSTGRES_DB=test_db -p 5433:5432 postgres:15): %v", pingErr)
// 	}
// 	createPaymentTestTables(t, pool)
// 	cleanupPaymentTestData(pool)
// 	return pool
// }

// func createPaymentTestTables(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	ctx := context.Background()

// 	for _, s := range []string{
// 		"DROP TABLE IF EXISTS payments CASCADE",
// 		"DROP TABLE IF EXISTS bookings CASCADE",
// 		"DROP TABLE IF EXISTS carts    CASCADE",
// 	} {
// 		pool.Exec(ctx, s)
// 	}

// 	statements := []string{
// 		`DO $$ BEGIN
// 			CREATE TYPE payment_method_type AS ENUM (
// 				'credit_card','paypal','apple_pay','google_pay','samsung_pay','instant_eft'
// 			);
// 		EXCEPTION WHEN duplicate_object THEN NULL;
// 		END $$`,

// 		// carts — needed so bookings.cart_id FK resolves
// 		`CREATE TABLE carts (
// 			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			user_id          UUID,
// 			session_id       UUID,
// 			last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
// 			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
// 		)`,

// 		// bookings — no FK to events; event_id uses DEFAULT gen_random_uuid().
// 		// cart_id added for multi-booking cart checkout support.
// 		`CREATE TABLE bookings (
// 			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			booking_number   VARCHAR(50) UNIQUE NOT NULL DEFAULT 'BK-' || substr(gen_random_uuid()::text,1,8),
// 			event_id         UUID NOT NULL DEFAULT gen_random_uuid(),
// 			user_id          UUID          DEFAULT gen_random_uuid(),
// 			is_guest         BOOLEAN NOT NULL DEFAULT FALSE,
// 			ticket_quantity  INT NOT NULL DEFAULT 1,
// 			unit_price       NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			subtotal         NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			platform_fee     NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			organizer_fee    NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			total_amount     NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			currency         VARCHAR(10) NOT NULL DEFAULT 'ZAR',
// 			booking_status   VARCHAR(50) NOT NULL DEFAULT 'pending',
// 			payment_status   VARCHAR(50) NOT NULL DEFAULT 'pending',
// 			tickets_sent     BOOLEAN NOT NULL DEFAULT FALSE,
// 			cart_id          UUID REFERENCES carts(id) ON DELETE SET NULL,
// 			confirmed_at     TIMESTAMPTZ,
// 			cancelled_at     TIMESTAMPTZ,
// 			updated_at       TIMESTAMPTZ DEFAULT NOW(),
// 			created_at       TIMESTAMPTZ DEFAULT NOW()
// 		)`,

// 		`CREATE TABLE payments (
// 			id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			booking_id                UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
// 			payment_gateway           VARCHAR(50) NOT NULL DEFAULT 'stitch',
// 			payment_intent_id         TEXT,
// 			transaction_id            TEXT,
// 			amount                    NUMERIC(10,2) NOT NULL,
// 			currency                  VARCHAR(10) NOT NULL DEFAULT 'ZAR',
// 			payment_method            payment_method_type NOT NULL DEFAULT 'credit_card',
// 			card_last_4               VARCHAR(4),
// 			card_brand                VARCHAR(50),
// 			status                    VARCHAR(50) NOT NULL DEFAULT 'pending',
// 			failure_reason            TEXT,
// 			failure_code              VARCHAR(100),
// 			stripe_customer           TEXT,
// 			stripe_payment_method_id  TEXT,
// 			stitch_payment_request_id TEXT,
// 			stitch_payment_url        TEXT,
// 			stitch_linked_account_id  TEXT,
// 			cart_id                   UUID REFERENCES carts(id) ON DELETE SET NULL,
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

// 	for _, stmt := range statements {
// 		if _, err := pool.Exec(ctx, stmt); err != nil {
// 			t.Skipf("payment DB tests skipped — table setup failed: %v", err)
// 		}
// 	}
// }

// func cleanupPaymentTestDB(pool *pgxpool.Pool) {
// 	cleanupPaymentTestData(pool)
// 	pool.Close()
// }

// func cleanupPaymentTestData(pool *pgxpool.Pool) {
// 	ctx := context.Background()
// 	pool.Exec(ctx, "TRUNCATE TABLE payments CASCADE")
// 	pool.Exec(ctx, "TRUNCATE TABLE bookings CASCADE")
// 	pool.Exec(ctx, "TRUNCATE TABLE carts    CASCADE")
// }

// // ==========================================
// // Helpers
// // ==========================================

// func insertTestBooking(t *testing.T, pool *pgxpool.Pool) string {
// 	t.Helper()
// 	bookingNumber := fmt.Sprintf("BK-%d", time.Now().UnixNano())
// 	var id string
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO bookings (booking_number, total_amount, currency)
// 		VALUES ($1, 100.00, 'ZAR')
// 		RETURNING id
// 	`, bookingNumber).Scan(&id)
// 	require.NoError(t, err, "failed to insert test booking")
// 	return id
// }

// // insertTestBookingForCart inserts a booking linked to a cart (for cart checkout tests).
// func insertTestBookingForCart(t *testing.T, pool *pgxpool.Pool, cartID string) string {
// 	t.Helper()
// 	bookingNumber := fmt.Sprintf("BK-%d", time.Now().UnixNano())
// 	var id string
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO bookings (booking_number, total_amount, currency, cart_id)
// 		VALUES ($1, 100.00, 'ZAR', $2)
// 		RETURNING id
// 	`, bookingNumber, cartID).Scan(&id)
// 	require.NoError(t, err, "failed to insert test booking for cart")
// 	return id
// }

// // insertTestCart creates a cart row and returns its ID.
// // Pass userID for authenticated carts, empty string for guest carts.
// func insertTestCart(t *testing.T, pool *pgxpool.Pool, userID string) string {
// 	t.Helper()
// 	var id string
// 	if userID != "" {
// 		err := pool.QueryRow(context.Background(),
// 			`INSERT INTO carts (user_id) VALUES ($1::uuid) RETURNING id`, userID,
// 		).Scan(&id)
// 		require.NoError(t, err)
// 	} else {
// 		err := pool.QueryRow(context.Background(),
// 			`INSERT INTO carts (session_id) VALUES (gen_random_uuid()) RETURNING id`,
// 		).Scan(&id)
// 		require.NoError(t, err)
// 	}
// 	return id
// }

// func insertTestPayment(t *testing.T, pool *pgxpool.Pool, bookingID, requestID string, status models.PaymentStatus) {
// 	t.Helper()
// 	_, err := pool.Exec(context.Background(), `
// 		INSERT INTO payments (
// 			booking_id, payment_gateway, payment_intent_id,
// 			amount, currency, payment_method, status,
// 			stitch_payment_request_id
// 		) VALUES ($1, 'stitch', $2, 100.00, 'ZAR', 'credit_card', $3, $4)
// 	`, bookingID, requestID, string(status), requestID)
// 	require.NoError(t, err, "failed to insert test payment")
// }

// // insertTestPaymentWithCart inserts a payment linked to both a booking and a cart.
// // Used to test multi-booking webhook confirmation.
// func insertTestPaymentWithCart(t *testing.T, pool *pgxpool.Pool, bookingID, requestID, cartID string, status models.PaymentStatus) {
// 	t.Helper()
// 	_, err := pool.Exec(context.Background(), `
// 		INSERT INTO payments (
// 			booking_id, payment_gateway, payment_intent_id,
// 			amount, currency, payment_method, status,
// 			stitch_payment_request_id, cart_id
// 		) VALUES ($1, 'stitch', $2, 200.00, 'ZAR', 'credit_card', $3, $4, $5)
// 	`, bookingID, requestID, string(status), requestID, cartID)
// 	require.NoError(t, err, "failed to insert test payment with cart")
// }

// func makeTestBooking(bookingID string) *models.Booking {
// 	return &models.Booking{
// 		ID:            bookingID,
// 		BookingNumber: fmt.Sprintf("BK-%d", time.Now().UnixNano()),
// 		TotalAmount:   100.00,
// 		Currency:      "ZAR",
// 	}
// }

// func testConfig() *config.Config {
// 	return &config.Config{
// 		StitchClientID:     "test-client-id",
// 		StitchClientSecret: "test-client-secret",
// 		StitchEnvironment:  "sandbox",
// 		FrontendURL:        "http://localhost:3000",
// 	}
// }

// func testJobChan() chan string {
// 	return make(chan string, 256)
// }

// func newMockStitchServer(t *testing.T, graphQLData interface{}) *httptest.Server {
// 	t.Helper()
// 	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		w.Header().Set("Content-Type", "application/json")
// 		switch r.URL.Path {
// 		case "/connect/token":
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"access_token": "test_token_abc", "token_type": "Bearer", "expires_in": 3600,
// 			})
// 		default:
// 			dataBytes, _ := json.Marshal(graphQLData)
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"data": json.RawMessage(dataBytes), "errors": nil,
// 			})
// 		}
// 	}))
// }

// func newMockStitchServerWithError(t *testing.T, errMsg string) *httptest.Server {
// 	t.Helper()
// 	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		w.Header().Set("Content-Type", "application/json")
// 		if r.URL.Path == "/connect/token" {
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"access_token": "test_token_abc", "token_type": "Bearer", "expires_in": 3600,
// 			})
// 			return
// 		}
// 		json.NewEncoder(w).Encode(map[string]interface{}{
// 			"data":   nil,
// 			"errors": []map[string]string{{"message": errMsg}},
// 		})
// 	}))
// }

// func pointServiceAtMock(t *testing.T, mockURL string) {
// 	t.Helper()
// 	origToken := stitchTokenURL
// 	origGQL := stitchGraphQLURL
// 	origAud := stitchTokenAudience
// 	stitchTokenURL = mockURL + "/connect/token"
// 	stitchGraphQLURL = mockURL + "/graphql"
// 	stitchTokenAudience = mockURL + "/connect/token"
// 	t.Cleanup(func() {
// 		stitchTokenURL = origToken
// 		stitchGraphQLURL = origGQL
// 		stitchTokenAudience = origAud
// 	})
// }

// func getBookingStatus(t *testing.T, pool *pgxpool.Pool, bookingID string) (bookingStatus, paymentStatus string) {
// 	t.Helper()
// 	err := pool.QueryRow(context.Background(),
// 		"SELECT booking_status, payment_status FROM bookings WHERE id = $1", bookingID,
// 	).Scan(&bookingStatus, &paymentStatus)
// 	require.NoError(t, err)
// 	return
// }

// func buildWebhookEvent(requestID, status, method, txnID string) *StitchWebhookEvent {
// 	txns := []map[string]string{}
// 	if txnID != "" {
// 		txns = append(txns, map[string]string{"id": txnID})
// 	}
// 	data, _ := json.Marshal(map[string]interface{}{
// 		"paymentInitiation": map[string]interface{}{
// 			"id":           requestID,
// 			"status":       status,
// 			"payer":        map[string]interface{}{"paymentMethod": method},
// 			"transactions": txns,
// 		},
// 	})
// 	return &StitchWebhookEvent{Type: "card_payment.updated", Data: json.RawMessage(data)}
// }

// // ==========================================
// // Tests: GetPaymentByBookingID
// // ==========================================

// func TestPaymentGetByBookingID(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	svc := NewPaymentService(pool, testConfig(), testJobChan())
// 	ctx := context.Background()

// 	t.Run("returns payment for existing booking", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_abc123", models.PaymentPending)

// 		payment, err := svc.GetPaymentByBookingID(ctx, bookingID)

// 		require.NoError(t, err)
// 		assert.Equal(t, bookingID, payment.BookingID)
// 		assert.Equal(t, models.PaymentPending, payment.Status)
// 		assert.NotNil(t, payment.StitchPaymentRequestID)
// 		assert.Equal(t, "req_abc123", *payment.StitchPaymentRequestID)
// 		assert.False(t, payment.WebhookReceived)
// 	})

// 	t.Run("returns error for non-existent booking", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		_, err := svc.GetPaymentByBookingID(ctx, "00000000-0000-0000-0000-000000000000")
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "payment not found")
// 	})
// }

// // ==========================================
// // Tests: createPaymentRecord
// // ==========================================

// func TestPaymentCreateRecord(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	svc := NewPaymentService(pool, testConfig(), testJobChan())
// 	ctx := context.Background()

// 	t.Run("inserts new payment record without cart_id (legacy flow)", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)

// 		// paymentURL is nil — must be stored as NULL, not empty string
// 		err := svc.createPaymentRecord(ctx, makeTestBooking(bookingID), "req_new", nil, nil)

// 		require.NoError(t, err)
// 		payment, err := svc.GetPaymentByBookingID(ctx, bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, "stitch", payment.PaymentGateway)
// 		assert.Equal(t, models.PaymentPending, payment.Status)
// 		assert.Equal(t, "req_new", *payment.StitchPaymentRequestID)
// 		assert.Nil(t, payment.StitchPaymentURL)
// 	})

// 	t.Run("inserts payment record with cart_id (cart checkout flow)", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		cartID := insertTestCart(t, pool, "")
// 		bookingID := insertTestBookingForCart(t, pool, cartID)

// 		err := svc.createPaymentRecord(ctx, makeTestBooking(bookingID), "req_cart", nil, &cartID)

// 		require.NoError(t, err)
// 		// Verify cart_id stored on the payment row
// 		var storedCartID *string
// 		pool.QueryRow(context.Background(),
// 			"SELECT cart_id FROM payments WHERE booking_id = $1", bookingID,
// 		).Scan(&storedCartID)
// 		require.NotNil(t, storedCartID)
// 		assert.Equal(t, cartID, *storedCartID)
// 	})

// 	t.Run("upserts on duplicate booking — updates to latest request ID", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)

// 		require.NoError(t, svc.createPaymentRecord(ctx, makeTestBooking(bookingID), "req_first", nil, nil))
// 		require.NoError(t, svc.createPaymentRecord(ctx, makeTestBooking(bookingID), "req_second", nil, nil))

// 		payment, err := svc.GetPaymentByBookingID(ctx, bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, "req_second", *payment.StitchPaymentRequestID)
// 	})
// }

// // ==========================================
// // Tests: CreatePaymentIntent (legacy — no cart)
// // ==========================================

// func TestPaymentCreatePaymentIntent(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)

// 	t.Run("returns client token and request ID on success", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		srv := newMockStitchServer(t, map[string]interface{}{
// 			"clientCardPaymentInitiate": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{
// 					"id": "stitch_req_001", "clientToken": "secure_fields_token_xyz",
// 				},
// 			},
// 		})
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		bookingID := insertTestBooking(t, pool)

// 		clientToken, requestID, err := svc.CreatePaymentIntent(makeTestBooking(bookingID))

// 		require.NoError(t, err)
// 		assert.Equal(t, "secure_fields_token_xyz", clientToken)
// 		assert.Equal(t, "stitch_req_001", requestID)

// 		payment, err := svc.GetPaymentByBookingID(context.Background(), bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentPending, payment.Status)
// 		assert.Equal(t, "stitch_req_001", *payment.StitchPaymentRequestID)
// 		// Legacy flow — no payment URL, must be NULL not empty string
// 		assert.Nil(t, payment.StitchPaymentURL)
// 	})

// 	t.Run("returns error when Stitch GraphQL returns an error", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		srv := newMockStitchServerWithError(t, "Secure Fields not enabled on this client")
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		_, _, err := NewPaymentService(pool, testConfig(), testJobChan()).CreatePaymentIntent(
// 			makeTestBooking(insertTestBooking(t, pool)),
// 		)
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "creating stitch secure fields session")
// 	})

// 	t.Run("returns error when response has empty client token", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		srv := newMockStitchServer(t, map[string]interface{}{
// 			"clientCardPaymentInitiate": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{"id": "", "clientToken": ""},
// 			},
// 		})
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		_, _, err := NewPaymentService(pool, testConfig(), testJobChan()).CreatePaymentIntent(
// 			makeTestBooking(insertTestBooking(t, pool)),
// 		)
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "empty payment ID or client token")
// 	})
// }

// // ==========================================
// // Tests: CreateCartPaymentIntent (new — cart checkout)
// // ==========================================

// func TestPaymentCreateCartPaymentIntent(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)

// 	t.Run("registered user — creates payment linked to cart_id", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		srv := newMockStitchServer(t, map[string]interface{}{
// 			"clientCardPaymentInitiate": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{
// 					"id": "cart_req_001", "clientToken": "cart_token_xyz",
// 				},
// 			},
// 		})
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		cartID := insertTestCart(t, pool, "")
// 		bookingID := insertTestBookingForCart(t, pool, cartID)
// 		primaryBooking := makeTestBooking(bookingID)

// 		clientToken, requestID, err := svc.CreateCartPaymentIntent(primaryBooking, cartID, 200.00)

// 		require.NoError(t, err)
// 		assert.Equal(t, "cart_token_xyz", clientToken)
// 		assert.Equal(t, "cart_req_001", requestID)

// 		// Payment row must have both booking_id and cart_id set
// 		var storedBookingID, storedCartID string
// 		err = pool.QueryRow(context.Background(),
// 			"SELECT booking_id, cart_id FROM payments WHERE stitch_payment_request_id = $1",
// 			requestID,
// 		).Scan(&storedBookingID, &storedCartID)
// 		require.NoError(t, err)
// 		assert.Equal(t, bookingID, storedBookingID)
// 		assert.Equal(t, cartID, storedCartID)
// 	})

// 	t.Run("guest user — same behaviour, session-based cart", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		srv := newMockStitchServer(t, map[string]interface{}{
// 			"clientCardPaymentInitiate": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{
// 					"id": "guest_cart_req", "clientToken": "guest_cart_token",
// 				},
// 			},
// 		})
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		// Guest cart (no user_id — session_id only)
// 		guestCartID := insertTestCart(t, pool, "")
// 		bookingID := insertTestBookingForCart(t, pool, guestCartID)
// 		primaryBooking := makeTestBooking(bookingID)

// 		clientToken, requestID, err := svc.CreateCartPaymentIntent(primaryBooking, guestCartID, 150.00)

// 		require.NoError(t, err)
// 		assert.Equal(t, "guest_cart_token", clientToken)
// 		assert.Equal(t, "guest_cart_req", requestID)

// 		var storedCartID string
// 		pool.QueryRow(context.Background(),
// 			"SELECT cart_id FROM payments WHERE stitch_payment_request_id = $1", requestID,
// 		).Scan(&storedCartID)
// 		assert.Equal(t, guestCartID, storedCartID)
// 	})

// 	t.Run("Stitch error propagates", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		srv := newMockStitchServerWithError(t, "payment service unavailable")
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		cartID := insertTestCart(t, pool, "")
// 		bookingID := insertTestBookingForCart(t, pool, cartID)

// 		_, _, err := NewPaymentService(pool, testConfig(), testJobChan()).
// 			CreateCartPaymentIntent(makeTestBooking(bookingID), cartID, 200.00)
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "creating cart payment intent")
// 	})
// }

// // ==========================================
// // Tests: HandleWebhook
// // ==========================================

// func TestPaymentHandleWebhook(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)

// 	t.Run("unknown event type is silently ignored", func(t *testing.T) {
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		err := svc.HandleWebhook(&StitchWebhookEvent{
// 			Type: "some_unknown_event.type",
// 			Data: json.RawMessage(`{}`),
// 		})
// 		assert.NoError(t, err)
// 	})

// 	t.Run("completed payment confirms booking and enqueues job (legacy — no cart)", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		jobCh := make(chan string, 4)
// 		svc := NewPaymentService(pool, testConfig(), jobCh)
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_complete", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(buildWebhookEvent("req_complete", "completed", "credit_card", "txn_001")))

// 		payment, err := svc.GetPaymentByBookingID(context.Background(), bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentCompleted, payment.Status)
// 		assert.True(t, payment.WebhookReceived)
// 		assert.Equal(t, "txn_001", *payment.TransactionID)
// 		assert.NotNil(t, payment.CompletedAt)

// 		bs, ps := getBookingStatus(t, pool, bookingID)
// 		assert.Equal(t, "confirmed", bs)
// 		assert.Equal(t, "completed", ps)

// 		select {
// 		case enqueued := <-jobCh:
// 			assert.Equal(t, bookingID, enqueued)
// 		default:
// 			t.Error("booking job channel was empty — worker not notified")
// 		}
// 	})

// 	t.Run("multi-booking cart — webhook confirms ALL bookings from the cart", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		jobCh := make(chan string, 8)
// 		svc := NewPaymentService(pool, testConfig(), jobCh)

// 		// One cart with two bookings (e.g. Chelsea + Coldplay)
// 		cartID := insertTestCart(t, pool, "")
// 		primaryID := insertTestBookingForCart(t, pool, cartID)
// 		secondaryID := insertTestBookingForCart(t, pool, cartID)

// 		// Payment linked to both the primary booking AND the cart
// 		insertTestPaymentWithCart(t, pool, primaryID, "req_multi", cartID, models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(buildWebhookEvent("req_multi", "completed", "credit_card", "txn_multi")))

// 		// Primary booking confirmed
// 		bs1, ps1 := getBookingStatus(t, pool, primaryID)
// 		assert.Equal(t, "confirmed", bs1, "primary booking must be confirmed")
// 		assert.Equal(t, "completed", ps1)

// 		// Secondary booking also confirmed via syncAllCartBookings
// 		bs2, ps2 := getBookingStatus(t, pool, secondaryID)
// 		assert.Equal(t, "confirmed", bs2, "secondary booking must be confirmed by cart sync")
// 		assert.Equal(t, "completed", ps2)

// 		// Both booking IDs enqueued for QR + email
// 		enqueued := make(map[string]bool)
// 		for i := 0; i < 2; i++ {
// 			select {
// 			case id := <-jobCh:
// 				enqueued[id] = true
// 			case <-time.After(100 * time.Millisecond):
// 				t.Errorf("expected job %d was not enqueued", i+1)
// 			}
// 		}
// 		assert.True(t, enqueued[primaryID], "primary booking job must be enqueued")
// 		assert.True(t, enqueued[secondaryID], "secondary booking job must be enqueued")
// 	})

// 	t.Run("multi-booking cart with guest user — confirms all bookings", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		jobCh := make(chan string, 8)
// 		svc := NewPaymentService(pool, testConfig(), jobCh)

// 		// Guest cart (session-based, no user_id)
// 		guestCartID := insertTestCart(t, pool, "")
// 		primaryID := insertTestBookingForCart(t, pool, guestCartID)
// 		secondaryID := insertTestBookingForCart(t, pool, guestCartID)
// 		insertTestPaymentWithCart(t, pool, primaryID, "req_guest_multi", guestCartID, models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(buildWebhookEvent("req_guest_multi", "completed", "credit_card", "txn_guest_multi")))

// 		bs1, _ := getBookingStatus(t, pool, primaryID)
// 		bs2, _ := getBookingStatus(t, pool, secondaryID)
// 		assert.Equal(t, "confirmed", bs1)
// 		assert.Equal(t, "confirmed", bs2, "guest secondary booking must also be confirmed")
// 	})

// 	t.Run("multi-booking cart — failed payment cancels ALL bookings", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())

// 		cartID := insertTestCart(t, pool, "")
// 		primaryID := insertTestBookingForCart(t, pool, cartID)
// 		secondaryID := insertTestBookingForCart(t, pool, cartID)
// 		insertTestPaymentWithCart(t, pool, primaryID, "req_multi_fail", cartID, models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(buildWebhookEvent("req_multi_fail", "failed", "credit_card", "")))

// 		bs1, _ := getBookingStatus(t, pool, primaryID)
// 		bs2, _ := getBookingStatus(t, pool, secondaryID)
// 		assert.Equal(t, "cancelled", bs1)
// 		assert.Equal(t, "cancelled", bs2, "all cart bookings must be cancelled on failure")
// 	})

// 	t.Run("failed payment cancels single booking", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_fail", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(buildWebhookEvent("req_fail", "failed", "credit_card", "")))

// 		bs, ps := getBookingStatus(t, pool, bookingID)
// 		assert.Equal(t, "cancelled", bs)
// 		assert.Equal(t, "failed", ps)
// 	})

// 	t.Run("processing status does not change booking status", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_proc", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(buildWebhookEvent("req_proc", "processing", "credit_card", "")))

// 		bs, ps := getBookingStatus(t, pool, bookingID)
// 		assert.Equal(t, "pending", bs)
// 		assert.Equal(t, "processing", ps)
// 	})

// 	t.Run("apple pay method mapped correctly", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_apple", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(buildWebhookEvent("req_apple", "completed", "apple_pay", "txn_apple")))

// 		payment, err := svc.GetPaymentByBookingID(context.Background(), bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentApplePay, payment.PaymentMethod)
// 	})

// 	t.Run("google pay method mapped correctly", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_gpay", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(buildWebhookEvent("req_gpay", "completed", "google_pay", "txn_gpay")))

// 		payment, err := svc.GetPaymentByBookingID(context.Background(), bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentGooglePay, payment.PaymentMethod)
// 	})

// 	t.Run("duplicate webhook is idempotent — second call silently ignored", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_idem", models.PaymentPending)

// 		event := buildWebhookEvent("req_idem", "completed", "credit_card", "txn_idem")
// 		require.NoError(t, svc.HandleWebhook(event), "first webhook should succeed")
// 		require.NoError(t, svc.HandleWebhook(event), "duplicate webhook must be silently ignored")

// 		bs, _ := getBookingStatus(t, pool, bookingID)
// 		assert.Equal(t, "confirmed", bs)
// 	})

// 	t.Run("unknown payment request ID is gracefully ignored", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		err := svc.HandleWebhook(buildWebhookEvent("req_does_not_exist", "completed", "credit_card", ""))
// 		assert.NoError(t, err)
// 	})

// 	t.Run("malformed JSON returns error", func(t *testing.T) {
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		err := svc.HandleWebhook(&StitchWebhookEvent{
// 			Type: "card_payment.updated",
// 			Data: json.RawMessage(`{bad json`),
// 		})
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "decoding payment update payload")
// 	})

// 	t.Run("missing payment request ID in payload returns error", func(t *testing.T) {
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		err := svc.HandleWebhook(&StitchWebhookEvent{
// 			Type: "card_payment.updated",
// 			Data: json.RawMessage(`{"paymentInitiation":{"id":"","status":"completed"}}`),
// 		})
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "webhook missing payment request ID")
// 	})
// }

// // ==========================================
// // Tests: ProcessRefund
// // ==========================================

// func TestPaymentProcessRefund(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)

// 	t.Run("returns error when no payment record exists", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		err := NewPaymentService(pool, testConfig(), testJobChan()).ProcessRefund("00000000-0000-0000-0000-000000000000")
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "fetching payment")
// 	})

// 	t.Run("returns error when payment is not completed", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_pending", models.PaymentPending)

// 		err := svc.ProcessRefund(bookingID)
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "not completed")
// 	})

// 	t.Run("returns error when stitch payment request ID is missing", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		bookingID := insertTestBooking(t, pool)
// 		_, err := pool.Exec(context.Background(), `
// 			INSERT INTO payments (booking_id, payment_gateway, amount, currency, payment_method, status)
// 			VALUES ($1, 'stitch', 100.00, 'ZAR', 'credit_card', 'completed')
// 		`, bookingID)
// 		require.NoError(t, err)

// 		err = svc.ProcessRefund(bookingID)
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "no stitch payment request ID")
// 	})

// 	t.Run("returns error when Stitch refund API fails", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		srv := newMockStitchServerWithError(t, "refund not available for this payment method")
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_refund_fail", models.PaymentCompleted)

// 		err := svc.ProcessRefund(bookingID)
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "stitch refund")
// 	})

// 	t.Run("successful refund updates payment to refunded", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		srv := newMockStitchServer(t, map[string]interface{}{
// 			"clientRefundInitiate": map[string]interface{}{"id": "refund_abc", "status": "completed"},
// 		})
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		svc := NewPaymentService(pool, testConfig(), testJobChan())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_refund_ok", models.PaymentCompleted)

// 		require.NoError(t, svc.ProcessRefund(bookingID))

// 		payment, err := svc.GetPaymentByBookingID(context.Background(), bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentRefunded, payment.Status)
// 		assert.Equal(t, "refund_abc", *payment.RefundID)
// 		assert.NotNil(t, payment.RefundedAt)
// 		assert.NotNil(t, payment.RefundedAmount)
// 	})
// }

// // ==========================================
// // Tests: getAccessToken caching
// // ==========================================

// func TestPaymentAccessTokenCaching(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)

// 	callCount := 0
// 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		callCount++
// 		w.Header().Set("Content-Type", "application/json")
// 		json.NewEncoder(w).Encode(map[string]interface{}{
// 			"access_token": "cached_token", "token_type": "Bearer", "expires_in": 3600,
// 		})
// 	}))
// 	defer srv.Close()
// 	pointServiceAtMock(t, srv.URL)

// 	svc := NewPaymentService(pool, testConfig(), testJobChan())
// 	ctx := context.Background()

// 	t.Run("fetches token on first call", func(t *testing.T) {
// 		token, err := svc.getAccessToken(ctx)
// 		require.NoError(t, err)
// 		assert.Equal(t, "cached_token", token)
// 		assert.Equal(t, 1, callCount)
// 	})

// 	t.Run("uses cached token on second call — no extra HTTP request", func(t *testing.T) {
// 		token, err := svc.getAccessToken(ctx)
// 		require.NoError(t, err)
// 		assert.Equal(t, "cached_token", token)
// 		assert.Equal(t, 1, callCount)
// 	})

// 	t.Run("re-fetches when token has expired", func(t *testing.T) {
// 		svc.cachedToken.ExpiresAt = time.Now().Add(-2 * time.Minute)
// 		token, err := svc.getAccessToken(ctx)
// 		require.NoError(t, err)
// 		assert.Equal(t, "cached_token", token)
// 		assert.Equal(t, 2, callCount)
// 	})
// }

// // ==========================================
// // Tests: stitchStatusToInternal
// // ==========================================

// func TestPaymentStitchStatusMapping(t *testing.T) {
// 	tests := []struct {
// 		input    string
// 		expected models.PaymentStatus
// 	}{
// 		{"completed", models.PaymentCompleted},
// 		{"COMPLETED", models.PaymentCompleted},
// 		{"succeeded", models.PaymentCompleted},
// 		{"processing", models.PaymentProcessing},
// 		{"failed", models.PaymentFailed},
// 		{"error", models.PaymentFailed},
// 		{"cancelled", models.PaymentFailed},
// 		{"canceled", models.PaymentFailed},
// 		{"pending", models.PaymentPending},
// 		{"unknown", models.PaymentPending},
// 		{"", models.PaymentPending},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.input, func(t *testing.T) {
// 			assert.Equal(t, tt.expected, stitchStatusToInternal(tt.input), "input: %q", tt.input)
// 		})
// 	}
// }

// // ==========================================
// // Tests: stitchMethodToInternal
// // ==========================================

// func TestPaymentStitchMethodMapping(t *testing.T) {
// 	tests := []struct {
// 		input    string
// 		expected models.PaymentMethod
// 	}{
// 		{"applepay", models.PaymentApplePay},
// 		{"apple_pay", models.PaymentApplePay},
// 		{"APPLEPAY", models.PaymentApplePay},
// 		{"googlepay", models.PaymentGooglePay},
// 		{"google_pay", models.PaymentGooglePay},
// 		{"samsungpay", models.PaymentSamsungPay},
// 		{"samsung_pay", models.PaymentSamsungPay},
// 		{"paypal", models.PaymentPayPal},
// 		{"instanteft", models.PaymentInstantEFT},
// 		{"instant_eft", models.PaymentInstantEFT},
// 		{"eft", models.PaymentInstantEFT},
// 		{"unknown", models.PaymentCreditCard},
// 		{"", models.PaymentCreditCard},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.input, func(t *testing.T) {
// 			assert.Equal(t, tt.expected, stitchMethodToInternal(tt.input), "input: %q", tt.input)
// 		})
// 	}
// }

// // ==========================================
// // Tests: syncBookingStatus
// // ==========================================

// func TestPaymentSyncBookingStatus(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	svc := NewPaymentService(pool, testConfig(), testJobChan())
// 	ctx := context.Background()

// 	tests := []struct {
// 		name                  string
// 		paymentStatus         models.PaymentStatus
// 		expectedBookingStatus string
// 	}{
// 		{"completed confirms booking", models.PaymentCompleted, "confirmed"},
// 		{"failed cancels booking", models.PaymentFailed, "cancelled"},
// 		{"pending leaves booking pending", models.PaymentPending, "pending"},
// 		{"processing leaves booking pending", models.PaymentProcessing, "pending"},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			cleanupPaymentTestData(pool)
// 			bookingID := insertTestBooking(t, pool)

// 			require.NoError(t, svc.syncBookingStatus(ctx, bookingID, tt.paymentStatus))

// 			bs, _ := getBookingStatus(t, pool, bookingID)
// 			assert.Equal(t, tt.expectedBookingStatus, bs)
// 		})
// 	}
// }

// // ==========================================
// // Tests: GetPaymentStatus
// // ==========================================

// func TestPaymentGetStatus(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	svc := NewPaymentService(pool, testConfig(), testJobChan())

// 	t.Run("returns payment for existing booking", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_status", models.PaymentCompleted)

// 		payment, err := svc.GetPaymentStatus(bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentCompleted, payment.Status)
// 	})

// 	t.Run("returns error when no payment exists", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		_, err := svc.GetPaymentStatus("00000000-0000-0000-0000-000000000000")
// 		assert.Error(t, err)
// 	})
// }

// // ==========================================
// // Integration: single-booking legacy lifecycle
// // ==========================================

// func TestPaymentFullLifecycle(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	cleanupPaymentTestData(pool)

// 	srv := newMockStitchServer(t, map[string]interface{}{
// 		"clientCardPaymentInitiate": map[string]interface{}{
// 			"paymentInitiation": map[string]interface{}{
// 				"id": "lifecycle_req", "clientToken": "lifecycle_client_token",
// 			},
// 		},
// 	})
// 	defer srv.Close()
// 	pointServiceAtMock(t, srv.URL)

// 	jobCh := make(chan string, 4)
// 	svc := NewPaymentService(pool, testConfig(), jobCh)
// 	ctx := context.Background()
// 	bookingID := insertTestBooking(t, pool)

// 	clientToken, requestID, err := svc.CreatePaymentIntent(makeTestBooking(bookingID))
// 	require.NoError(t, err)
// 	assert.NotEmpty(t, clientToken)
// 	assert.NotEmpty(t, requestID)

// 	payment, err := svc.GetPaymentStatus(bookingID)
// 	require.NoError(t, err)
// 	assert.Equal(t, models.PaymentPending, payment.Status)
// 	assert.False(t, payment.WebhookReceived)

// 	require.NoError(t, svc.HandleWebhook(buildWebhookEvent(requestID, "completed", "credit_card", "txn_lifecycle")))

// 	payment, err = svc.GetPaymentByBookingID(ctx, bookingID)
// 	require.NoError(t, err)
// 	assert.Equal(t, models.PaymentCompleted, payment.Status)
// 	assert.True(t, payment.WebhookReceived)

// 	bs, _ := getBookingStatus(t, pool, bookingID)
// 	assert.Equal(t, "confirmed", bs)

// 	select {
// 	case enqueued := <-jobCh:
// 		assert.Equal(t, bookingID, enqueued)
// 	default:
// 		t.Error("booking worker job not enqueued after payment completed")
// 	}

// 	// Duplicate webhook silently ignored
// 	require.NoError(t, svc.HandleWebhook(buildWebhookEvent(requestID, "completed", "credit_card", "txn_lifecycle")))
// }

// // ==========================================
// // Integration: multi-booking cart lifecycle
// // ==========================================

// func TestPaymentCartLifecycle(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	cleanupPaymentTestData(pool)

// 	srv := newMockStitchServer(t, map[string]interface{}{
// 		"clientCardPaymentInitiate": map[string]interface{}{
// 			"paymentInitiation": map[string]interface{}{
// 				"id": "cart_lifecycle_req", "clientToken": "cart_lifecycle_token",
// 			},
// 		},
// 	})
// 	defer srv.Close()
// 	pointServiceAtMock(t, srv.URL)

// 	jobCh := make(chan string, 8)
// 	svc := NewPaymentService(pool, testConfig(), jobCh)
// 	ctx := context.Background()

// 	// Cart with two bookings (registered user)
// 	cartID := insertTestCart(t, pool, "")
// 	booking1ID := insertTestBookingForCart(t, pool, cartID)
// 	booking2ID := insertTestBookingForCart(t, pool, cartID)
// 	primaryBooking := makeTestBooking(booking1ID)

// 	// 1. Create cart payment intent
// 	clientToken, requestID, err := svc.CreateCartPaymentIntent(primaryBooking, cartID, 400.00)
// 	require.NoError(t, err)
// 	assert.NotEmpty(t, clientToken)
// 	assert.NotEmpty(t, requestID)

// 	// 2. Both bookings start as pending
// 	bs1, _ := getBookingStatus(t, pool, booking1ID)
// 	bs2, _ := getBookingStatus(t, pool, booking2ID)
// 	assert.Equal(t, "pending", bs1)
// 	assert.Equal(t, "pending", bs2)

// 	// 3. Stitch fires completed webhook
// 	require.NoError(t, svc.HandleWebhook(buildWebhookEvent(requestID, "completed", "credit_card", "txn_cart")))

// 	// 4. Both bookings confirmed
// 	bs1, ps1 := getBookingStatus(t, pool, booking1ID)
// 	bs2, ps2 := getBookingStatus(t, pool, booking2ID)
// 	assert.Equal(t, "confirmed", bs1)
// 	assert.Equal(t, "completed", ps1)
// 	assert.Equal(t, "confirmed", bs2, "second booking must also be confirmed")
// 	assert.Equal(t, "completed", ps2)

// 	// 5. Payment record marked completed
// 	payment, err := svc.GetPaymentByBookingID(ctx, booking1ID)
// 	require.NoError(t, err)
// 	assert.Equal(t, models.PaymentCompleted, payment.Status)
// 	assert.True(t, payment.WebhookReceived)

// 	// 6. Both booking jobs enqueued
// 	enqueued := make(map[string]bool)
// 	for i := 0; i < 2; i++ {
// 		select {
// 		case id := <-jobCh:
// 			enqueued[id] = true
// 		case <-time.After(200 * time.Millisecond):
// 			t.Errorf("job %d not enqueued in time", i+1)
// 		}
// 	}
// 	assert.True(t, enqueued[booking1ID])
// 	assert.True(t, enqueued[booking2ID])
// }

// // ==========================================
// // Integration: guest user cart lifecycle
// // ==========================================

// func TestPaymentGuestCartLifecycle(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	cleanupPaymentTestData(pool)

// 	srv := newMockStitchServer(t, map[string]interface{}{
// 		"clientCardPaymentInitiate": map[string]interface{}{
// 			"paymentInitiation": map[string]interface{}{
// 				"id": "guest_lifecycle_req", "clientToken": "guest_lifecycle_token",
// 			},
// 		},
// 	})
// 	defer srv.Close()
// 	pointServiceAtMock(t, srv.URL)

// 	jobCh := make(chan string, 8)
// 	svc := NewPaymentService(pool, testConfig(), jobCh)

// 	// Guest cart — session-based, no user_id
// 	guestCartID := insertTestCart(t, pool, "")
// 	booking1ID := insertTestBookingForCart(t, pool, guestCartID)
// 	booking2ID := insertTestBookingForCart(t, pool, guestCartID)

// 	clientToken, requestID, err := svc.CreateCartPaymentIntent(makeTestBooking(booking1ID), guestCartID, 300.00)
// 	require.NoError(t, err)
// 	assert.NotEmpty(t, clientToken)

// 	require.NoError(t, svc.HandleWebhook(buildWebhookEvent(requestID, "completed", "instant_eft", "txn_guest_cart")))

// 	bs1, _ := getBookingStatus(t, pool, booking1ID)
// 	bs2, _ := getBookingStatus(t, pool, booking2ID)
// 	assert.Equal(t, "confirmed", bs1)
// 	assert.Equal(t, "confirmed", bs2, "guest secondary booking must be confirmed")

// 	// Verify instant_eft payment method mapped correctly
// 	payment, err := svc.GetPaymentByBookingID(context.Background(), booking1ID)
// 	require.NoError(t, err)
// 	assert.Equal(t, models.PaymentInstantEFT, payment.PaymentMethod)
// }

// // ==========================================
// // Integration: failed cart payment
// // ==========================================

// func TestPaymentFailedLifecycle(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	cleanupPaymentTestData(pool)

// 	srv := newMockStitchServer(t, map[string]interface{}{
// 		"clientCardPaymentInitiate": map[string]interface{}{
// 			"paymentInitiation": map[string]interface{}{
// 				"id": "failed_req", "clientToken": "failed_client_token",
// 			},
// 		},
// 	})
// 	defer srv.Close()
// 	pointServiceAtMock(t, srv.URL)

// 	svc := NewPaymentService(pool, testConfig(), testJobChan())
// 	bookingID := insertTestBooking(t, pool)

// 	clientToken, requestID, err := svc.CreatePaymentIntent(makeTestBooking(bookingID))
// 	require.NoError(t, err)
// 	assert.NotEmpty(t, clientToken)

// 	require.NoError(t, svc.HandleWebhook(buildWebhookEvent(requestID, "failed", "credit_card", "")))

// 	bs, ps := getBookingStatus(t, pool, bookingID)
// 	assert.Equal(t, "cancelled", bs)
// 	assert.Equal(t, "failed", ps)
// }

// // ==========================================
// // Integration: retry after failed payment
// // ==========================================

// func TestPaymentRetryAfterFailure(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	cleanupPaymentTestData(pool)

// 	callCount := 0
// 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		w.Header().Set("Content-Type", "application/json")
// 		if r.URL.Path == "/connect/token" {
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"access_token": "test_token", "token_type": "Bearer", "expires_in": 3600,
// 			})
// 			return
// 		}
// 		callCount++
// 		data, _ := json.Marshal(map[string]interface{}{
// 			"clientCardPaymentInitiate": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{
// 					"id":          fmt.Sprintf("retry_req_%d", callCount),
// 					"clientToken": fmt.Sprintf("retry_token_%d", callCount),
// 				},
// 			},
// 		})
// 		json.NewEncoder(w).Encode(map[string]interface{}{"data": json.RawMessage(data), "errors": nil})
// 	}))
// 	defer srv.Close()
// 	pointServiceAtMock(t, srv.URL)

// 	svc := NewPaymentService(pool, testConfig(), testJobChan())
// 	bookingID := insertTestBooking(t, pool)
// 	booking := makeTestBooking(bookingID)

// 	token1, req1, err := svc.CreatePaymentIntent(booking)
// 	require.NoError(t, err)
// 	assert.Equal(t, "retry_token_1", token1)

// 	require.NoError(t, svc.HandleWebhook(buildWebhookEvent(req1, "failed", "credit_card", "")))

// 	token2, req2, err := svc.CreatePaymentIntent(booking)
// 	require.NoError(t, err)
// 	assert.Equal(t, "retry_token_2", token2)
// 	assert.NotEqual(t, req1, req2)

// 	require.NoError(t, svc.HandleWebhook(buildWebhookEvent(req2, "completed", "credit_card", "txn_retry")))

// 	bs, _ := getBookingStatus(t, pool, bookingID)
// 	assert.Equal(t, "confirmed", bs)
// }