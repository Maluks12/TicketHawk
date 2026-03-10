package services

// // BEFORE RUNNING THESE TESTS make two small changes to payment_service.go:
// //
// // 1. Change const → var for the URL block so tests can redirect to a mock server:
// //
// //    var (
// //        stitchTokenURL      = "https://secure.stitch.money/connect/token"
// //        stitchGraphQLURL    = "https://api.stitch.money/graphql"
// //        stitchTokenAudience = "https://secure.stitch.money/connect/token"
// //        stitchGateway       = "stitch"
// //    )
// //
// // 2. Add ::text cast in syncBookingStatus to fix pgx type ambiguity:
// //
// //    confirmed_at = CASE WHEN $1::text = 'confirmed' THEN CURRENT_TIMESTAMP ELSE confirmed_at END,
// //
// // Start the test database (once):
// //
// //    docker run -d --name test-postgres \
// //      -e POSTGRES_USER=testuser \
// //      -e POSTGRES_PASSWORD=testpass \
// //      -e POSTGRES_DB=test_db \
// //      -p 5433:5432 postgres:15
// //
// // Run all payment tests:
// //
// //    go test ./internal/services/... -v -run TestPayment

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
// 	dbURL := "postgresql://testuser:testpass@localhost:5433/test_db"
// 	pool, err := pgxpool.New(context.Background(), dbURL)
// 	if err != nil {
// 		t.Fatalf(
// 			"Failed to connect to test database: %v\n"+
// 				"Start the test DB with:\n"+
// 				"  docker run -d --name test-postgres"+
// 				" -e POSTGRES_USER=testuser"+
// 				" -e POSTGRES_PASSWORD=testpass"+
// 				" -e POSTGRES_DB=test_db"+
// 				" -p 5433:5432 postgres:15",
// 			err,
// 		)
// 	}
// 	createPaymentTestTables(t, pool)
// 	cleanupPaymentTestData(pool)
// 	return pool
// }

// func createPaymentTestTables(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	statements := []string{
// 		// Enums must be created before the tables that reference them.
// 		// EXCEPTION WHEN duplicate_object means re-running tests never fails.
// 		`DO $$ BEGIN
// 			CREATE TYPE payment_method_type AS ENUM (
// 				'credit_card','paypal','apple_pay',
// 				'google_pay','samsung_pay','instant_eft'
// 			);
// 		EXCEPTION WHEN duplicate_object THEN NULL;
// 		END $$`,

// 		`DO $$ BEGIN
// 			CREATE TYPE payment_status_type AS ENUM (
// 				'pending','processing','completed','failed','refunded'
// 			);
// 		EXCEPTION WHEN duplicate_object THEN NULL;
// 		END $$`,

// 		`DO $$ BEGIN
// 			CREATE TYPE booking_status_type AS ENUM (
// 				'pending','confirmed','cancelled','refunded'
// 			);
// 		EXCEPTION WHEN duplicate_object THEN NULL;
// 		END $$`,

// 		// bookings — only columns needed by the payment service
// 		`CREATE TABLE IF NOT EXISTS bookings (
// 			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			booking_number   VARCHAR(50) UNIQUE NOT NULL,
// 			event_id         UUID NOT NULL DEFAULT gen_random_uuid(),
// 			user_id          UUID NOT NULL DEFAULT gen_random_uuid(),
// 			ticket_quantity  INT NOT NULL DEFAULT 1,
// 			unit_price       NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			subtotal         NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			platform_fee     NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			organizer_fee    NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			total_amount     NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			currency         VARCHAR(10) NOT NULL DEFAULT 'ZAR',
// 			booking_status   VARCHAR(50) NOT NULL DEFAULT 'pending',
// 			payment_status   VARCHAR(50) NOT NULL DEFAULT 'pending',
// 			confirmed_at     TIMESTAMPTZ,
// 			cancelled_at     TIMESTAMPTZ,
// 			updated_at       TIMESTAMPTZ DEFAULT NOW(),
// 			created_at       TIMESTAMPTZ DEFAULT NOW()
// 		)`,

// 		// payments — payment_method uses the enum so the ::payment_method_type
// 		// cast in handlePaymentUpdate resolves correctly against the test DB.
// 		`CREATE TABLE IF NOT EXISTS payments (
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
// 		if _, err := pool.Exec(context.Background(), stmt); err != nil {
// 			t.Logf("Warning creating test table/type: %v", err)
// 		}
// 	}
// }

// func cleanupPaymentTestDB(pool *pgxpool.Pool) {
// 	cleanupPaymentTestData(pool)
// 	pool.Close()
// }

// func cleanupPaymentTestData(pool *pgxpool.Pool) {
// 	pool.Exec(context.Background(), "TRUNCATE TABLE payments CASCADE")
// 	pool.Exec(context.Background(), "TRUNCATE TABLE bookings CASCADE")
// }

// // ==========================================
// // Helpers
// // ==========================================

// // insertTestBooking inserts a minimal booking row and returns its UUID.
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

// // insertTestPayment inserts a payment row for the given booking.
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

// // makeTestBooking returns a *models.Booking suitable for service method calls.
// func makeTestBooking(bookingID string) *models.Booking {
// 	return &models.Booking{
// 		ID:            bookingID,
// 		BookingNumber: fmt.Sprintf("BK-%d", time.Now().UnixNano()),
// 		TotalAmount:   100.00,
// 		Currency:      "ZAR",
// 	}
// }

// // testConfig returns a minimal config with blank Stitch credentials.
// // Tests that need the Stitch API use a mock server via pointServiceAtMock.
// func testConfig() *config.Config {
// 	return &config.Config{
// 		StitchClientID:     "test-client-id",
// 		StitchClientSecret: "test-client-secret",
// 		StitchEnvironment:  "sandbox",
// 		FrontendURL:        "http://localhost:3000",
// 	}
// }

// // newMockStitchServer starts a local HTTP server that handles both
// // the token endpoint and the GraphQL endpoint.
// // graphQLData is marshalled into the `data` field of the GraphQL response.
// func newMockStitchServer(t *testing.T, graphQLData interface{}) *httptest.Server {
// 	t.Helper()
// 	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		w.Header().Set("Content-Type", "application/json")
// 		switch r.URL.Path {
// 		case "/connect/token":
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"access_token": "test_token_abc",
// 				"token_type":   "Bearer",
// 				"expires_in":   3600,
// 			})
// 		default:
// 			dataBytes, _ := json.Marshal(graphQLData)
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"data":   json.RawMessage(dataBytes),
// 				"errors": nil,
// 			})
// 		}
// 	}))
// }

// // newMockStitchServerWithError returns a mock that always returns a GraphQL error.
// func newMockStitchServerWithError(t *testing.T, errMsg string) *httptest.Server {
// 	t.Helper()
// 	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		w.Header().Set("Content-Type", "application/json")
// 		if r.URL.Path == "/connect/token" {
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"access_token": "test_token_abc",
// 				"token_type":   "Bearer",
// 				"expires_in":   3600,
// 			})
// 			return
// 		}
// 		json.NewEncoder(w).Encode(map[string]interface{}{
// 			"data":   nil,
// 			"errors": []map[string]string{{"message": errMsg}},
// 		})
// 	}))
// }

// // pointServiceAtMock overrides the package-level URL vars so the service
// // calls go to the mock server. t.Cleanup restores the originals automatically.
// func pointServiceAtMock(t *testing.T, mockURL string) {
// 	t.Helper()
// 	origToken := stitchTokenURL
// 	origGQL   := stitchGraphQLURL
// 	origAud   := stitchTokenAudience

// 	stitchTokenURL      = mockURL + "/connect/token"
// 	stitchGraphQLURL    = mockURL + "/graphql"
// 	stitchTokenAudience = mockURL + "/connect/token"

// 	t.Cleanup(func() {
// 		stitchTokenURL      = origToken
// 		stitchGraphQLURL    = origGQL
// 		stitchTokenAudience = origAud
// 	})
// }

// // getBookingStatus fetches booking_status and payment_status from the DB.
// func getBookingStatus(t *testing.T, pool *pgxpool.Pool, bookingID string) (bookingStatus, paymentStatus string) {
// 	t.Helper()
// 	err := pool.QueryRow(context.Background(),
// 		"SELECT booking_status, payment_status FROM bookings WHERE id = $1",
// 		bookingID,
// 	).Scan(&bookingStatus, &paymentStatus)
// 	require.NoError(t, err)
// 	return
// }

// // buildWebhookEvent constructs a StitchWebhookEvent for card_payment.updated.
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
// 	svc := NewPaymentService(pool, testConfig())
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
// 	svc := NewPaymentService(pool, testConfig())
// 	ctx := context.Background()

// 	t.Run("inserts new payment record", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		booking   := makeTestBooking(bookingID)

// 		err := svc.createPaymentRecord(ctx, booking, "req_new", "")

// 		require.NoError(t, err)
// 		payment, err := svc.GetPaymentByBookingID(ctx, bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, "stitch", payment.PaymentGateway)
// 		assert.Equal(t, models.PaymentPending, payment.Status)
// 		assert.Equal(t, models.PaymentCreditCard, payment.PaymentMethod)
// 		assert.Equal(t, "req_new", *payment.StitchPaymentRequestID)
// 	})

// 	t.Run("upserts on duplicate booking — updates to latest request ID", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		booking   := makeTestBooking(bookingID)

// 		require.NoError(t, svc.createPaymentRecord(ctx, booking, "req_first", ""))
// 		require.NoError(t, svc.createPaymentRecord(ctx, booking, "req_second", ""))

// 		payment, err := svc.GetPaymentByBookingID(ctx, bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, "req_second", *payment.StitchPaymentRequestID)
// 	})

// 	t.Run("stitch_payment_url is empty in no-redirect flow", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		booking   := makeTestBooking(bookingID)

// 		require.NoError(t, svc.createPaymentRecord(ctx, booking, "req_nourl", ""))

// 		payment, err := svc.GetPaymentByBookingID(ctx, bookingID)
// 		require.NoError(t, err)
// 		if payment.StitchPaymentURL != nil {
// 			assert.Empty(t, *payment.StitchPaymentURL, "no redirect URL should be stored in Secure Fields flow")
// 		}
// 	})
// }

// // ==========================================
// // Tests: CreatePaymentIntent
// // ==========================================

// func TestPaymentCreatePaymentIntent(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)

// 	t.Run("returns client token and request ID on success", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)

// 		srv := newMockStitchServer(t, map[string]interface{}{
// 			"clientCardPaymentInitiate": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{
// 					"id":          "stitch_req_001",
// 					"clientToken": "secure_fields_token_xyz",
// 				},
// 			},
// 		})
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		svc      := NewPaymentService(pool, testConfig())
// 		bookingID := insertTestBooking(t, pool)

// 		clientToken, requestID, err := svc.CreatePaymentIntent(makeTestBooking(bookingID))

// 		require.NoError(t, err)
// 		assert.Equal(t, "secure_fields_token_xyz", clientToken)
// 		assert.Equal(t, "stitch_req_001", requestID)

// 		payment, err := svc.GetPaymentByBookingID(context.Background(), bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentPending, payment.Status)
// 		assert.Equal(t, "stitch_req_001", *payment.StitchPaymentRequestID)
// 	})

// 	t.Run("returns error when Stitch GraphQL returns an error", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)

// 		srv := newMockStitchServerWithError(t, "Secure Fields not enabled on this client")
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		_, _, err := NewPaymentService(pool, testConfig()).CreatePaymentIntent(
// 			makeTestBooking(insertTestBooking(t, pool)),
// 		)

// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "creating stitch secure fields session")
// 	})

// 	t.Run("returns error when response has empty client token", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)

// 		srv := newMockStitchServer(t, map[string]interface{}{
// 			"clientCardPaymentInitiate": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{
// 					"id":          "",
// 					"clientToken": "",
// 				},
// 			},
// 		})
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		_, _, err := NewPaymentService(pool, testConfig()).CreatePaymentIntent(
// 			makeTestBooking(insertTestBooking(t, pool)),
// 		)

// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "empty payment ID or client token")
// 	})
// }

// // ==========================================
// // Tests: HandleWebhook
// // ==========================================

// func TestPaymentHandleWebhook(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	svc := NewPaymentService(pool, testConfig())

// 	t.Run("unknown event type is silently ignored", func(t *testing.T) {
// 		err := svc.HandleWebhook(&StitchWebhookEvent{
// 			Type: "some_unknown_event.type",
// 			Data: json.RawMessage(`{}`),
// 		})
// 		assert.NoError(t, err)
// 	})

// 	t.Run("completed payment updates payment row and confirms booking", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_complete", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(
// 			buildWebhookEvent("req_complete", "completed", "credit_card", "txn_001"),
// 		))

// 		payment, err := svc.GetPaymentByBookingID(context.Background(), bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentCompleted, payment.Status)
// 		assert.True(t, payment.WebhookReceived)
// 		assert.NotNil(t, payment.TransactionID)
// 		assert.Equal(t, "txn_001", *payment.TransactionID)
// 		assert.NotNil(t, payment.CompletedAt)

// 		bs, ps := getBookingStatus(t, pool, bookingID)
// 		assert.Equal(t, "confirmed", bs)
// 		assert.Equal(t, "completed", ps)
// 	})

// 	t.Run("failed payment cancels the booking", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_fail", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(
// 			buildWebhookEvent("req_fail", "failed", "credit_card", ""),
// 		))

// 		bs, ps := getBookingStatus(t, pool, bookingID)
// 		assert.Equal(t, "cancelled", bs)
// 		assert.Equal(t, "failed", ps)
// 	})

// 	t.Run("processing status does not change booking status", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_proc", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(
// 			buildWebhookEvent("req_proc", "processing", "credit_card", ""),
// 		))

// 		bs, ps := getBookingStatus(t, pool, bookingID)
// 		assert.Equal(t, "pending", bs)
// 		assert.Equal(t, "processing", ps)
// 	})

// 	t.Run("apple pay method mapped correctly", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_apple", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(
// 			buildWebhookEvent("req_apple", "completed", "apple_pay", "txn_apple"),
// 		))

// 		payment, err := svc.GetPaymentByBookingID(context.Background(), bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentApplePay, payment.PaymentMethod)
// 	})

// 	t.Run("google pay method mapped correctly", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_gpay", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(
// 			buildWebhookEvent("req_gpay", "completed", "google_pay", "txn_gpay"),
// 		))

// 		payment, err := svc.GetPaymentByBookingID(context.Background(), bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentGooglePay, payment.PaymentMethod)
// 	})

// 	t.Run("replay attack — second identical webhook is rejected", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_replay", models.PaymentPending)

// 		event := buildWebhookEvent("req_replay", "completed", "credit_card", "txn_r")

// 		require.NoError(t, svc.HandleWebhook(event), "first webhook should succeed")
// 		assert.Error(t, svc.HandleWebhook(event), "replayed webhook should be rejected")
// 	})

// 	t.Run("unknown payment request ID returns error", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)

// 		err := svc.HandleWebhook(buildWebhookEvent("req_does_not_exist", "completed", "credit_card", ""))
// 		assert.Error(t, err)
// 	})

// 	t.Run("malformed JSON returns error", func(t *testing.T) {
// 		err := svc.HandleWebhook(&StitchWebhookEvent{
// 			Type: "card_payment.updated",
// 			Data: json.RawMessage(`{bad json`),
// 		})
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "decoding payment update payload")
// 	})

// 	t.Run("missing payment request ID in payload returns error", func(t *testing.T) {
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

// 		err := NewPaymentService(pool, testConfig()).ProcessRefund("00000000-0000-0000-0000-000000000000")
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "fetching payment")
// 	})

// 	t.Run("returns error when payment is not completed", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc      := NewPaymentService(pool, testConfig())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_pending", models.PaymentPending)

// 		err := svc.ProcessRefund(bookingID)
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "not completed")
// 	})

// 	t.Run("returns error when stitch payment request ID is missing", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)
// 		svc      := NewPaymentService(pool, testConfig())
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

// 		svc      := NewPaymentService(pool, testConfig())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_refund_fail", models.PaymentCompleted)

// 		err := svc.ProcessRefund(bookingID)
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "stitch refund")
// 	})

// 	t.Run("successful refund updates payment to refunded", func(t *testing.T) {
// 		cleanupPaymentTestData(pool)

// 		srv := newMockStitchServer(t, map[string]interface{}{
// 			"clientRefundInitiate": map[string]interface{}{
// 				"id":     "refund_abc",
// 				"status": "completed",
// 			},
// 		})
// 		defer srv.Close()
// 		pointServiceAtMock(t, srv.URL)

// 		svc      := NewPaymentService(pool, testConfig())
// 		bookingID := insertTestBooking(t, pool)
// 		insertTestPayment(t, pool, bookingID, "req_refund_ok", models.PaymentCompleted)

// 		require.NoError(t, svc.ProcessRefund(bookingID))

// 		payment, err := svc.GetPaymentByBookingID(context.Background(), bookingID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentRefunded, payment.Status)
// 		assert.NotNil(t, payment.RefundID)
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
// 			"access_token": "cached_token",
// 			"token_type":   "Bearer",
// 			"expires_in":   3600,
// 		})
// 	}))
// 	defer srv.Close()
// 	pointServiceAtMock(t, srv.URL)

// 	svc := NewPaymentService(pool, testConfig())
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
// 		assert.Equal(t, 1, callCount, "cached token should not trigger another HTTP call")
// 	})

// 	t.Run("re-fetches when token has expired", func(t *testing.T) {
// 		svc.cachedToken.ExpiresAt = time.Now().Add(-2 * time.Minute)

// 		token, err := svc.getAccessToken(ctx)
// 		require.NoError(t, err)
// 		assert.Equal(t, "cached_token", token)
// 		assert.Equal(t, 2, callCount, "expired token should trigger a new fetch")
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
// 		{"completed",  models.PaymentCompleted},
// 		{"COMPLETED",  models.PaymentCompleted},
// 		{"succeeded",  models.PaymentCompleted},
// 		{"processing", models.PaymentProcessing},
// 		{"failed",     models.PaymentFailed},
// 		{"error",      models.PaymentFailed},
// 		{"cancelled",  models.PaymentFailed},
// 		{"canceled",   models.PaymentFailed},
// 		{"pending",    models.PaymentPending},
// 		{"unknown",    models.PaymentPending},
// 		{"",           models.PaymentPending},
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
// 		{"applepay",    models.PaymentApplePay},
// 		{"apple_pay",   models.PaymentApplePay},
// 		{"APPLEPAY",    models.PaymentApplePay},
// 		{"googlepay",   models.PaymentGooglePay},
// 		{"google_pay",  models.PaymentGooglePay},
// 		{"samsungpay",  models.PaymentSamsungPay},
// 		{"samsung_pay", models.PaymentSamsungPay},
// 		{"paypal",      models.PaymentPayPal},
// 		{"instanteft",  models.PaymentInstantEFT},
// 		{"instant_eft", models.PaymentInstantEFT},
// 		{"eft",         models.PaymentInstantEFT},
// 		{"unknown",     models.PaymentCreditCard},
// 		{"",            models.PaymentCreditCard},
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
// 	svc := NewPaymentService(pool, testConfig())
// 	ctx := context.Background()

// 	tests := []struct {
// 		name                  string
// 		paymentStatus         models.PaymentStatus
// 		expectedBookingStatus string
// 	}{
// 		{"completed confirms booking",        models.PaymentCompleted,  "confirmed"},
// 		{"failed cancels booking",            models.PaymentFailed,     "cancelled"},
// 		{"pending leaves booking pending",    models.PaymentPending,    "pending"},
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
// 	svc := NewPaymentService(pool, testConfig())

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
// // Integration: full happy path lifecycle
// // ==========================================

// func TestPaymentFullLifecycle(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	cleanupPaymentTestData(pool)

// 	srv := newMockStitchServer(t, map[string]interface{}{
// 		"clientCardPaymentInitiate": map[string]interface{}{
// 			"paymentInitiation": map[string]interface{}{
// 				"id":          "lifecycle_req",
// 				"clientToken": "lifecycle_client_token",
// 			},
// 		},
// 	})
// 	defer srv.Close()
// 	pointServiceAtMock(t, srv.URL)

// 	svc      := NewPaymentService(pool, testConfig())
// 	ctx      := context.Background()
// 	bookingID := insertTestBooking(t, pool)

// 	// 1. Create payment intent — must return client token, NOT a redirect URL
// 	clientToken, requestID, err := svc.CreatePaymentIntent(makeTestBooking(bookingID))
// 	require.NoError(t, err)
// 	assert.NotEmpty(t, clientToken, "Secure Fields must return a client token, not a redirect URL")
// 	assert.NotEmpty(t, requestID)

// 	// 2. Payment starts as pending
// 	payment, err := svc.GetPaymentStatus(bookingID)
// 	require.NoError(t, err)
// 	assert.Equal(t, models.PaymentPending, payment.Status)
// 	assert.False(t, payment.WebhookReceived)

// 	// 3. Stitch fires completed webhook
// 	require.NoError(t, svc.HandleWebhook(
// 		buildWebhookEvent(requestID, "completed", "credit_card", "txn_lifecycle"),
// 	))

// 	// 4. Payment and booking are confirmed
// 	payment, err = svc.GetPaymentByBookingID(ctx, bookingID)
// 	require.NoError(t, err)
// 	assert.Equal(t, models.PaymentCompleted, payment.Status)
// 	assert.True(t, payment.WebhookReceived)

// 	bs, _ := getBookingStatus(t, pool, bookingID)
// 	assert.Equal(t, "confirmed", bs)

// 	// 5. Replayed webhook must be rejected (idempotency guard)
// 	err = svc.HandleWebhook(buildWebhookEvent(requestID, "completed", "credit_card", "txn_lifecycle"))
// 	assert.Error(t, err, "replayed webhook must be rejected — webhook_received is already TRUE")
// }

// // ==========================================
// // Integration: failed payment lifecycle
// // ==========================================

// func TestPaymentFailedLifecycle(t *testing.T) {
// 	pool := setupPaymentTestDB(t)
// 	defer cleanupPaymentTestDB(pool)
// 	cleanupPaymentTestData(pool)

// 	srv := newMockStitchServer(t, map[string]interface{}{
// 		"clientCardPaymentInitiate": map[string]interface{}{
// 			"paymentInitiation": map[string]interface{}{
// 				"id":          "failed_req",
// 				"clientToken": "failed_client_token",
// 			},
// 		},
// 	})
// 	defer srv.Close()
// 	pointServiceAtMock(t, srv.URL)

// 	svc      := NewPaymentService(pool, testConfig())
// 	bookingID := insertTestBooking(t, pool)

// 	clientToken, requestID, err := svc.CreatePaymentIntent(makeTestBooking(bookingID))
// 	require.NoError(t, err)
// 	assert.NotEmpty(t, clientToken)

// 	require.NoError(t, svc.HandleWebhook(
// 		buildWebhookEvent(requestID, "failed", "credit_card", ""),
// 	))

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
// 				"access_token": "test_token",
// 				"token_type":   "Bearer",
// 				"expires_in":   3600,
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
// 		json.NewEncoder(w).Encode(map[string]interface{}{
// 			"data":   json.RawMessage(data),
// 			"errors": nil,
// 		})
// 	}))
// 	defer srv.Close()
// 	pointServiceAtMock(t, srv.URL)

// 	svc      := NewPaymentService(pool, testConfig())
// 	bookingID := insertTestBooking(t, pool)
// 	booking   := makeTestBooking(bookingID)

// 	// First attempt
// 	token1, req1, err := svc.CreatePaymentIntent(booking)
// 	require.NoError(t, err)
// 	assert.Equal(t, "retry_token_1", token1)

// 	// First attempt fails
// 	require.NoError(t, svc.HandleWebhook(buildWebhookEvent(req1, "failed", "credit_card", "")))

// 	// Retry — must produce a fresh client token and new request ID
// 	token2, req2, err := svc.CreatePaymentIntent(booking)
// 	require.NoError(t, err)
// 	assert.Equal(t, "retry_token_2", token2)
// 	assert.NotEqual(t, req1, req2, "retry must produce a new payment request ID")

// 	// Second attempt succeeds
// 	require.NoError(t, svc.HandleWebhook(buildWebhookEvent(req2, "completed", "credit_card", "txn_retry")))

// 	bs, _ := getBookingStatus(t, pool, bookingID)
// 	assert.Equal(t, "confirmed", bs)
// }