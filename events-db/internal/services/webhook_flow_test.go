package services

// // Run with:
// //   go test ./internal/services/... -v -run "TestWH"
// //
// // Requires test DB:
// //   docker run -d --name test-postgres \
// //     -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
// //     -e POSTGRES_DB=test_db -p 5433:5432 postgres:15

// import (
// 	"bytes"
// 	"context"
// 	"crypto/hmac"
// 	"crypto/sha256"
// 	"encoding/base64"
// 	"encoding/json"
// 	"fmt"
// 	"io"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"
// 	"time"

// 	"events-db/internal/config"
// 	"events-db/internal/models"

// 	"github.com/gin-gonic/gin"
// 	"github.com/jackc/pgx/v5/pgxpool"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// 	svix "github.com/svix/svix-webhooks/go"
// )

// // ─── constants ────────────────────────────────────────────────────────────────

// const (
// 	whTestDB    = "postgresql://testuser:testpass@localhost:5433/test_db"
// 	whRawSecret = "super-secret-webhook-key-32chars!!"
// )

// func whWebhookSecret() string {
// 	return "whsec_" + base64.StdEncoding.EncodeToString([]byte(whRawSecret))
// }

// // ─── DB setup ─────────────────────────────────────────────────────────────────

// func whDB(t *testing.T) *pgxpool.Pool {
// 	t.Helper()
// 	pool, err := pgxpool.New(context.Background(), whTestDB)
// 	require.NoError(t, err, "connect to test DB — is docker running?")
// 	whMigrate(t, pool)
// 	whTruncate(t, pool)
// 	return pool
// }

// func whMigrate(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	ctx := context.Background()

// 	// Drop first so a leftover schema from another test suite never bleeds in.
// 	for _, s := range []string{
// 		"DROP TABLE IF EXISTS payments  CASCADE",
// 		"DROP TABLE IF EXISTS bookings  CASCADE",
// 	} {
// 		pool.Exec(ctx, s)
// 	}

// 	for _, s := range []string{
// 		// payment_method_type enum — idempotent
// 		`DO $$ BEGIN
// 		   CREATE TYPE payment_method_type AS ENUM (
// 		     'credit_card','paypal','apple_pay','google_pay','samsung_pay','instant_eft');
// 		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

// 		// bookings — event_id / user_id have defaults so inserts stay minimal
// 		`CREATE TABLE bookings (
// 			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			booking_number  VARCHAR(50) UNIQUE NOT NULL
// 			                DEFAULT 'BK-' || substr(gen_random_uuid()::text,1,8),
// 			event_id        UUID NOT NULL DEFAULT gen_random_uuid(),
// 			user_id         UUID          DEFAULT gen_random_uuid(),
// 			ticket_quantity INT  NOT NULL DEFAULT 1,
// 			unit_price      NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			subtotal        NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			platform_fee    NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			organizer_fee   NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			total_amount    NUMERIC(10,2) NOT NULL DEFAULT 0,
// 			currency        VARCHAR(10)   NOT NULL DEFAULT 'ZAR',
// 			booking_status  VARCHAR(50)   NOT NULL DEFAULT 'pending',
// 			payment_status  VARCHAR(50)   NOT NULL DEFAULT 'pending',
// 			confirmed_at    TIMESTAMPTZ,
// 			cancelled_at    TIMESTAMPTZ,
// 			updated_at      TIMESTAMPTZ   DEFAULT NOW(),
// 			created_at      TIMESTAMPTZ   DEFAULT NOW()
// 		)`,

// 		`CREATE TABLE payments (
// 			id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			booking_id                UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
// 			payment_gateway           VARCHAR(50)   NOT NULL DEFAULT 'stitch',
// 			payment_intent_id         TEXT,
// 			transaction_id            TEXT,
// 			amount                    NUMERIC(10,2) NOT NULL,
// 			currency                  VARCHAR(10)   NOT NULL DEFAULT 'ZAR',
// 			payment_method            payment_method_type NOT NULL DEFAULT 'credit_card',
// 			card_last_4               VARCHAR(4),
// 			card_brand                VARCHAR(50),
// 			status                    VARCHAR(50)   NOT NULL DEFAULT 'pending',
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
// 			webhook_received          BOOLEAN     DEFAULT FALSE,
// 			completed_at              TIMESTAMPTZ,
// 			created_at                TIMESTAMPTZ DEFAULT NOW(),
// 			updated_at                TIMESTAMPTZ DEFAULT NOW(),
// 			UNIQUE(booking_id)
// 		)`,
// 	} {
// 		_, err := pool.Exec(ctx, s)
// 		require.NoError(t, err, "migrate failed")
// 	}
// }

// func whTruncate(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	pool.Exec(context.Background(), "TRUNCATE payments CASCADE")
// 	pool.Exec(context.Background(), "TRUNCATE bookings CASCADE")
// }

// // ─── config ───────────────────────────────────────────────────────────────────

// func whCfg() *config.Config {
// 	return &config.Config{
// 		JWTSecret:           "test-jwt-secret-that-is-32chars!!",
// 		FrontendURL:         "http://localhost:3000",
// 		SendGridAPIKey:      "SG.test",
// 		StitchClientID:      "test-client",
// 		StitchClientSecret:  "test-secret",
// 		StitchWebhookSecret: whWebhookSecret(),
// 		EmailFromAddress:    "noreply@test.com",
// 		EmailFromName:       "Test",
// 	}
// }

// // ─── seed helpers ─────────────────────────────────────────────────────────────

// func whSeedBooking(t *testing.T, pool *pgxpool.Pool) string {
// 	t.Helper()
// 	var id string
// 	err := pool.QueryRow(context.Background(),
// 		`INSERT INTO bookings (total_amount, currency) VALUES (100.00, 'ZAR') RETURNING id`,
// 	).Scan(&id)
// 	require.NoError(t, err)
// 	return id
// }

// func whSeedPayment(t *testing.T, pool *pgxpool.Pool, bookingID, reqID string, status models.PaymentStatus) {
// 	t.Helper()
// 	_, err := pool.Exec(context.Background(), `
// 		INSERT INTO payments
// 			(booking_id, payment_gateway, payment_intent_id, amount, currency,
// 			 payment_method, status, stitch_payment_request_id)
// 		VALUES ($1, 'stitch', $2, 100.00, 'ZAR', 'credit_card', $3, $4)
// 	`, bookingID, reqID, string(status), reqID)
// 	require.NoError(t, err)
// }

// func whMakeBooking(bookingID string) *models.Booking {
// 	return &models.Booking{
// 		ID:            bookingID,
// 		BookingNumber: fmt.Sprintf("BK-%d", time.Now().UnixNano()),
// 		TotalAmount:   100.00,
// 		Currency:      "ZAR",
// 	}
// }

// func whGetStatuses(t *testing.T, pool *pgxpool.Pool, bookingID string) (bookingSt, paymentSt string) {
// 	t.Helper()
// 	err := pool.QueryRow(context.Background(),
// 		`SELECT booking_status, payment_status FROM bookings WHERE id = $1`, bookingID,
// 	).Scan(&bookingSt, &paymentSt)
// 	require.NoError(t, err)
// 	return
// }

// // ─── mock Stitch server ───────────────────────────────────────────────────────

// // whMockServer starts a mock Stitch server.
// // If gqlData != nil it returns it as a successful GraphQL response.
// // If gqlData == nil it returns a GraphQL error with errMsg.
// func whMockServer(t *testing.T, gqlData interface{}, errMsg string) *httptest.Server {
// 	t.Helper()
// 	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		w.Header().Set("Content-Type", "application/json")
// 		if r.URL.Path == "/connect/token" {
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"access_token": "test_token",
// 				"token_type":   "Bearer",
// 				"expires_in":   3600,
// 			})
// 			return
// 		}
// 		if gqlData != nil {
// 			dataBytes, _ := json.Marshal(gqlData)
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"data": json.RawMessage(dataBytes), "errors": nil,
// 			})
// 		} else {
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"data":   nil,
// 				"errors": []map[string]string{{"message": errMsg}},
// 			})
// 		}
// 	}))
// }

// // whRedirect overrides the package-level URL vars so all Stitch HTTP calls
// // hit the mock server. Requires payment_service.go to declare the block as var.
// func whRedirect(t *testing.T, serverURL string) {
// 	t.Helper()
// 	origToken, origGQL, origAud := stitchTokenURL, stitchGraphQLURL, stitchTokenAudience
// 	stitchTokenURL = serverURL + "/connect/token"
// 	stitchGraphQLURL = serverURL + "/graphql"
// 	stitchTokenAudience = serverURL + "/connect/token"
// 	t.Cleanup(func() {
// 		stitchTokenURL, stitchGraphQLURL, stitchTokenAudience = origToken, origGQL, origAud
// 	})
// }

// // whEvent builds a card_payment.updated StitchWebhookEvent.
// func whEvent(reqID, status, method, txnID string) *StitchWebhookEvent {
// 	txns := []map[string]string{}
// 	if txnID != "" {
// 		txns = append(txns, map[string]string{"id": txnID})
// 	}
// 	data, _ := json.Marshal(map[string]interface{}{
// 		"paymentInitiation": map[string]interface{}{
// 			"id":           reqID,
// 			"status":       status,
// 			"payer":        map[string]interface{}{"paymentMethod": method},
// 			"transactions": txns,
// 		},
// 	})
// 	return &StitchWebhookEvent{Type: "card_payment.updated", Data: json.RawMessage(data)}
// }

// // ─── HTTP handler helpers (inlined — handlers imports services, not the reverse) ─

// func whRouter(svc *PaymentService) *gin.Engine {
// 	gin.SetMode(gin.TestMode)
// 	r := gin.New()
// 	wh, err := svix.NewWebhook(whWebhookSecret())
// 	if err != nil {
// 		panic(err)
// 	}
// 	r.POST("/webhooks/stitch", func(c *gin.Context) {
// 		body, err := io.ReadAll(c.Request.Body)
// 		if err != nil {
// 			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read body"})
// 			return
// 		}
// 		if err := wh.Verify(body, c.Request.Header); err != nil {
// 			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
// 			return
// 		}
// 		var event StitchWebhookEvent
// 		if err := json.Unmarshal(body, &event); err != nil {
// 			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
// 			return
// 		}
// 		if err := svc.HandleWebhook(&event); err != nil {
// 			c.JSON(http.StatusOK, gin.H{"received": true, "warning": err.Error()})
// 			return
// 		}
// 		c.JSON(http.StatusOK, gin.H{"received": true})
// 	})
// 	return r
// }

// func whSignedReq(t *testing.T, r *gin.Engine, body []byte) *httptest.ResponseRecorder {
// 	t.Helper()
// 	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
// 	ts := fmt.Sprintf("%d", time.Now().Unix())
// 	mac := hmac.New(sha256.New, []byte(whRawSecret))
// 	mac.Write([]byte(fmt.Sprintf("%s.%s.%s", msgID, ts, string(body))))
// 	sig := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
// 	req := httptest.NewRequest(http.MethodPost, "/webhooks/stitch", bytes.NewReader(body))
// 	req.Header.Set("Content-Type", "application/json")
// 	req.Header.Set("svix-id", msgID)
// 	req.Header.Set("svix-timestamp", ts)
// 	req.Header.Set("svix-signature", sig)
// 	w := httptest.NewRecorder()
// 	r.ServeHTTP(w, req)
// 	return w
// }

// // =============================================================================
// // TEST: HandleWebhook
// // =============================================================================

// func TestWHHandleWebhook(t *testing.T) {
// 	pool := whDB(t)
// 	defer pool.Close()
// 	svc := NewPaymentService(pool, whCfg())

// 	t.Run("unknown event type returns no error", func(t *testing.T) {
// 		err := svc.HandleWebhook(&StitchWebhookEvent{
// 			Type: "some.unknown.event",
// 			Data: json.RawMessage(`{}`),
// 		})
// 		assert.NoError(t, err)
// 	})

// 	t.Run("malformed JSON returns parse error", func(t *testing.T) {
// 		err := svc.HandleWebhook(&StitchWebhookEvent{
// 			Type: "card_payment.updated",
// 			Data: json.RawMessage(`{bad json`),
// 		})
// 		require.Error(t, err)
// 		assert.Contains(t, err.Error(), "decoding payment update payload")
// 	})

// 	t.Run("empty payment request ID returns error", func(t *testing.T) {
// 		err := svc.HandleWebhook(&StitchWebhookEvent{
// 			Type: "card_payment.updated",
// 			Data: json.RawMessage(`{"paymentInitiation":{"id":"","status":"completed"}}`),
// 		})
// 		require.Error(t, err)
// 		assert.Contains(t, err.Error(), "webhook missing payment request ID")
// 	})

// 	t.Run("unknown stitch ID is gracefully ignored", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		err := svc.HandleWebhook(whEvent("req_no_match", "completed", "credit_card", ""))
// 		assert.NoError(t, err)
// 	})

// 	t.Run("completed payment updates row and confirms booking", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_complete", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(whEvent("req_complete", "completed", "credit_card", "txn_001")))

// 		p, err := svc.GetPaymentByBookingID(context.Background(), bID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentCompleted, p.Status)
// 		assert.True(t, p.WebhookReceived)
// 		require.NotNil(t, p.TransactionID)
// 		assert.Equal(t, "txn_001", *p.TransactionID)
// 		assert.NotNil(t, p.CompletedAt)

// 		bs, ps := whGetStatuses(t, pool, bID)
// 		assert.Equal(t, "confirmed", bs)
// 		assert.Equal(t, "completed", ps)
// 	})

// 	t.Run("duplicate webhook is idempotent", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_idem", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(whEvent("req_idem", "completed", "credit_card", "txn_idem")))
// 		require.NoError(t, svc.HandleWebhook(whEvent("req_idem", "completed", "credit_card", "txn_idem")))

// 		bs, _ := whGetStatuses(t, pool, bID)
// 		assert.Equal(t, "confirmed", bs)
// 	})

// 	t.Run("failed payment cancels booking", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_fail", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(whEvent("req_fail", "failed", "credit_card", "")))

// 		bs, ps := whGetStatuses(t, pool, bID)
// 		assert.Equal(t, "cancelled", bs)
// 		assert.Equal(t, "failed", ps)
// 	})

// 	t.Run("processing status leaves booking pending", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_proc", models.PaymentPending)

// 		require.NoError(t, svc.HandleWebhook(whEvent("req_proc", "processing", "credit_card", "")))

// 		bs, ps := whGetStatuses(t, pool, bID)
// 		assert.Equal(t, "pending", bs)
// 		assert.Equal(t, "processing", ps)
// 	})

// 	t.Run("apple_pay mapped correctly", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_apple", models.PaymentPending)
// 		require.NoError(t, svc.HandleWebhook(whEvent("req_apple", "completed", "apple_pay", "txn_apple")))
// 		p, err := svc.GetPaymentByBookingID(context.Background(), bID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentApplePay, p.PaymentMethod)
// 	})

// 	t.Run("google_pay mapped correctly", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_gpay", models.PaymentPending)
// 		require.NoError(t, svc.HandleWebhook(whEvent("req_gpay", "completed", "google_pay", "txn_gpay")))
// 		p, err := svc.GetPaymentByBookingID(context.Background(), bID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentGooglePay, p.PaymentMethod)
// 	})

// 	t.Run("instant_eft mapped correctly", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_eft", models.PaymentPending)
// 		require.NoError(t, svc.HandleWebhook(whEvent("req_eft", "completed", "instant_eft", "txn_eft")))
// 		p, err := svc.GetPaymentByBookingID(context.Background(), bID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentInstantEFT, p.PaymentMethod)
// 	})

// 	t.Run("payment_initiation_request.updated is handled", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_pir", models.PaymentPending)
// 		ev := whEvent("req_pir", "completed", "credit_card", "txn_pir")
// 		ev.Type = "payment_initiation_request.updated"
// 		require.NoError(t, svc.HandleWebhook(ev))
// 		bs, _ := whGetStatuses(t, pool, bID)
// 		assert.Equal(t, "confirmed", bs)
// 	})
// }

// // =============================================================================
// // TEST: CreatePaymentIntent
// // =============================================================================

// func TestWHCreatePaymentIntent(t *testing.T) {
// 	pool := whDB(t)
// 	defer pool.Close()

// 	t.Run("success returns client token and request ID", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		srv := whMockServer(t, map[string]interface{}{
// 			"clientCardPaymentInitiate": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{
// 					"id": "req_001", "clientToken": "token_xyz",
// 				},
// 			},
// 		}, "")
// 		defer srv.Close()
// 		whRedirect(t, srv.URL)

// 		svc := NewPaymentService(pool, whCfg())
// 		bID := whSeedBooking(t, pool)

// 		clientToken, requestID, err := svc.CreatePaymentIntent(whMakeBooking(bID))
// 		require.NoError(t, err)
// 		assert.Equal(t, "token_xyz", clientToken)
// 		assert.Equal(t, "req_001", requestID)

// 		p, err := svc.GetPaymentByBookingID(context.Background(), bID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentPending, p.Status)
// 		require.NotNil(t, p.StitchPaymentRequestID)
// 		assert.Equal(t, "req_001", *p.StitchPaymentRequestID)
// 	})

// 	t.Run("GraphQL error propagates", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		srv := whMockServer(t, nil, "Secure Fields not enabled")
// 		defer srv.Close()
// 		whRedirect(t, srv.URL)

// 		_, _, err := NewPaymentService(pool, whCfg()).CreatePaymentIntent(
// 			whMakeBooking(whSeedBooking(t, pool)),
// 		)
// 		require.Error(t, err)
// 		assert.Contains(t, err.Error(), "creating stitch secure fields session")
// 	})

// 	t.Run("empty token from Stitch returns error", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		srv := whMockServer(t, map[string]interface{}{
// 			"clientCardPaymentInitiate": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{"id": "", "clientToken": ""},
// 			},
// 		}, "")
// 		defer srv.Close()
// 		whRedirect(t, srv.URL)

// 		_, _, err := NewPaymentService(pool, whCfg()).CreatePaymentIntent(
// 			whMakeBooking(whSeedBooking(t, pool)),
// 		)
// 		require.Error(t, err)
// 		assert.Contains(t, err.Error(), "empty payment ID or client token")
// 	})

// 	t.Run("second call upserts with new request ID", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		call := 0
// 		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			w.Header().Set("Content-Type", "application/json")
// 			if r.URL.Path == "/connect/token" {
// 				json.NewEncoder(w).Encode(map[string]interface{}{
// 					"access_token": "tok", "token_type": "Bearer", "expires_in": 3600,
// 				})
// 				return
// 			}
// 			call++
// 			data, _ := json.Marshal(map[string]interface{}{
// 				"clientCardPaymentInitiate": map[string]interface{}{
// 					"paymentInitiation": map[string]interface{}{
// 						"id":          fmt.Sprintf("req_%d", call),
// 						"clientToken": fmt.Sprintf("token_%d", call),
// 					},
// 				},
// 			})
// 			json.NewEncoder(w).Encode(map[string]interface{}{"data": json.RawMessage(data), "errors": nil})
// 		}))
// 		defer srv.Close()
// 		whRedirect(t, srv.URL)

// 		svc := NewPaymentService(pool, whCfg())
// 		bID := whSeedBooking(t, pool)
// 		b := whMakeBooking(bID)

// 		_, _, err := svc.CreatePaymentIntent(b)
// 		require.NoError(t, err)
// 		_, _, err = svc.CreatePaymentIntent(b)
// 		require.NoError(t, err)

// 		p, err := svc.GetPaymentByBookingID(context.Background(), bID)
// 		require.NoError(t, err)
// 		require.NotNil(t, p.StitchPaymentRequestID)
// 		assert.Equal(t, "req_2", *p.StitchPaymentRequestID)
// 	})
// }

// // =============================================================================
// // TEST: ProcessRefund
// // =============================================================================

// func TestWHProcessRefund(t *testing.T) {
// 	pool := whDB(t)
// 	defer pool.Close()

// 	t.Run("no payment record returns error", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		err := NewPaymentService(pool, whCfg()).ProcessRefund("00000000-0000-0000-0000-000000000000")
// 		require.Error(t, err)
// 		assert.Contains(t, err.Error(), "fetching payment")
// 	})

// 	t.Run("non-completed payment returns error", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_pend", models.PaymentPending)
// 		err := NewPaymentService(pool, whCfg()).ProcessRefund(bID)
// 		require.Error(t, err)
// 		assert.Contains(t, err.Error(), "not completed")
// 	})

// 	t.Run("missing stitch request ID returns error", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		_, err := pool.Exec(context.Background(), `
// 			INSERT INTO payments (booking_id, payment_gateway, amount, currency, payment_method, status)
// 			VALUES ($1, 'stitch', 100.00, 'ZAR', 'credit_card', 'completed')
// 		`, bID)
// 		require.NoError(t, err)
// 		err = NewPaymentService(pool, whCfg()).ProcessRefund(bID)
// 		require.Error(t, err)
// 		assert.Contains(t, err.Error(), "no stitch payment request ID")
// 	})

// 	t.Run("Stitch API error returns error", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		srv := whMockServer(t, nil, "refund not available")
// 		defer srv.Close()
// 		whRedirect(t, srv.URL)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_rfail", models.PaymentCompleted)
// 		err := NewPaymentService(pool, whCfg()).ProcessRefund(bID)
// 		require.Error(t, err)
// 		assert.Contains(t, err.Error(), "stitch refund")
// 	})

// 	t.Run("successful refund marks payment as refunded", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		srv := whMockServer(t, map[string]interface{}{
// 			"clientRefundInitiate": map[string]interface{}{
// 				"id": "refund_abc", "status": "completed",
// 			},
// 		}, "")
// 		defer srv.Close()
// 		whRedirect(t, srv.URL)

// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_rok", models.PaymentCompleted)
// 		require.NoError(t, NewPaymentService(pool, whCfg()).ProcessRefund(bID))

// 		p, err := NewPaymentService(pool, whCfg()).GetPaymentByBookingID(context.Background(), bID)
// 		require.NoError(t, err)
// 		assert.Equal(t, models.PaymentRefunded, p.Status)
// 		require.NotNil(t, p.RefundID)
// 		assert.Equal(t, "refund_abc", *p.RefundID)
// 		assert.NotNil(t, p.RefundedAt)
// 		assert.NotNil(t, p.RefundedAmount)
// 	})
// }

// // =============================================================================
// // TEST: Status & method mapping
// // =============================================================================

// func TestWHStatusMapping(t *testing.T) {
// 	cases := []struct {
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
// 	for _, tc := range cases {
// 		tc := tc
// 		t.Run(tc.input, func(t *testing.T) {
// 			assert.Equal(t, tc.expected, stitchStatusToInternal(tc.input))
// 		})
// 	}
// }

// func TestWHMethodMapping(t *testing.T) {
// 	cases := []struct {
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
// 	for _, tc := range cases {
// 		tc := tc
// 		t.Run(tc.input, func(t *testing.T) {
// 			assert.Equal(t, tc.expected, stitchMethodToInternal(tc.input))
// 		})
// 	}
// }

// // =============================================================================
// // TEST: Token caching
// // =============================================================================

// func TestWHTokenCaching(t *testing.T) {
// 	pool := whDB(t)
// 	defer pool.Close()

// 	calls := 0
// 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		calls++
// 		w.Header().Set("Content-Type", "application/json")
// 		json.NewEncoder(w).Encode(map[string]interface{}{
// 			"access_token": "cached_tok", "token_type": "Bearer", "expires_in": 3600,
// 		})
// 	}))
// 	defer srv.Close()
// 	whRedirect(t, srv.URL)

// 	svc := NewPaymentService(pool, whCfg())
// 	ctx := context.Background()

// 	t.Run("first call fetches token", func(t *testing.T) {
// 		tok, err := svc.getAccessToken(ctx)
// 		require.NoError(t, err)
// 		assert.Equal(t, "cached_tok", tok)
// 		assert.Equal(t, 1, calls)
// 	})

// 	t.Run("second call uses cache", func(t *testing.T) {
// 		tok, err := svc.getAccessToken(ctx)
// 		require.NoError(t, err)
// 		assert.Equal(t, "cached_tok", tok)
// 		assert.Equal(t, 1, calls, "no additional HTTP call expected")
// 	})

// 	t.Run("expired token triggers re-fetch", func(t *testing.T) {
// 		svc.cachedToken.ExpiresAt = time.Now().Add(-2 * time.Minute)
// 		tok, err := svc.getAccessToken(ctx)
// 		require.NoError(t, err)
// 		assert.Equal(t, "cached_tok", tok)
// 		assert.Equal(t, 2, calls)
// 	})
// }

// // =============================================================================
// // TEST: HTTP handler
// // =============================================================================

// func TestWHHTTPHandler(t *testing.T) {
// 	pool := whDB(t)
// 	defer pool.Close()
// 	svc := NewPaymentService(pool, whCfg())

// 	t.Run("valid signed payload returns 200", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_http_ok", models.PaymentPending)

// 		body, _ := json.Marshal(map[string]interface{}{
// 			"type": "card_payment.updated",
// 			"data": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{
// 					"id":           "req_http_ok",
// 					"status":       "completed",
// 					"payer":        map[string]interface{}{"paymentMethod": "credit_card"},
// 					"transactions": []map[string]string{{"id": "txn_http"}},
// 				},
// 			},
// 		})

// 		w := whSignedReq(t, whRouter(svc), body)
// 		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

// 		var resp map[string]interface{}
// 		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
// 		assert.Equal(t, true, resp["received"])
// 	})

// 	t.Run("wrong signature returns 401", func(t *testing.T) {
// 		body := []byte(`{"type":"card_payment.updated","data":{}}`)
// 		req := httptest.NewRequest(http.MethodPost, "/webhooks/stitch", bytes.NewReader(body))
// 		req.Header.Set("Content-Type", "application/json")
// 		req.Header.Set("svix-id", "msg_fake")
// 		req.Header.Set("svix-timestamp", fmt.Sprintf("%d", time.Now().Unix()))
// 		req.Header.Set("svix-signature", "v1,bm90YXJlYWxzaWduYXR1cmU=")
// 		w := httptest.NewRecorder()
// 		whRouter(svc).ServeHTTP(w, req)
// 		assert.Equal(t, http.StatusUnauthorized, w.Code)
// 	})

// 	t.Run("missing svix headers returns 401", func(t *testing.T) {
// 		req := httptest.NewRequest(http.MethodPost, "/webhooks/stitch",
// 			bytes.NewReader([]byte(`{"type":"card_payment.updated","data":{}}`)))
// 		req.Header.Set("Content-Type", "application/json")
// 		w := httptest.NewRecorder()
// 		whRouter(svc).ServeHTTP(w, req)
// 		assert.Equal(t, http.StatusUnauthorized, w.Code)
// 	})

// 	t.Run("malformed JSON returns 400", func(t *testing.T) {
// 		w := whSignedReq(t, whRouter(svc), []byte(`{not valid json}`))
// 		assert.Equal(t, http.StatusBadRequest, w.Code)
// 	})

// 	t.Run("unknown event type returns 200", func(t *testing.T) {
// 		body, _ := json.Marshal(map[string]interface{}{
// 			"type": "some.unknown.event",
// 			"data": map[string]interface{}{},
// 		})
// 		w := whSignedReq(t, whRouter(svc), body)
// 		assert.Equal(t, http.StatusOK, w.Code)
// 	})

// 	t.Run("payment_initiation_request.updated confirms booking", func(t *testing.T) {
// 		whTruncate(t, pool)
// 		bID := whSeedBooking(t, pool)
// 		whSeedPayment(t, pool, bID, "req_pir_http", models.PaymentPending)

// 		body, _ := json.Marshal(map[string]interface{}{
// 			"type": "payment_initiation_request.updated",
// 			"data": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{
// 					"id":           "req_pir_http",
// 					"status":       "completed",
// 					"payer":        map[string]interface{}{"paymentMethod": "instant_eft"},
// 					"transactions": []map[string]string{{"id": "txn_pir"}},
// 				},
// 			},
// 		})

// 		w := whSignedReq(t, whRouter(svc), body)
// 		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

// 		bs, ps := whGetStatuses(t, pool, bID)
// 		assert.Equal(t, "confirmed", bs)
// 		assert.Equal(t, "completed", ps)
// 	})
// }

// // =============================================================================
// // TEST: Integration
// // =============================================================================

// func TestWHFullLifecycle(t *testing.T) {
// 	pool := whDB(t)
// 	defer pool.Close()
// 	whTruncate(t, pool)

// 	srv := whMockServer(t, map[string]interface{}{
// 		"clientCardPaymentInitiate": map[string]interface{}{
// 			"paymentInitiation": map[string]interface{}{
// 				"id": "lifecycle_req", "clientToken": "lifecycle_token",
// 			},
// 		},
// 	}, "")
// 	defer srv.Close()
// 	whRedirect(t, srv.URL)

// 	svc := NewPaymentService(pool, whCfg())
// 	bID := whSeedBooking(t, pool)

// 	// 1. Create intent
// 	clientToken, requestID, err := svc.CreatePaymentIntent(whMakeBooking(bID))
// 	require.NoError(t, err)
// 	assert.NotEmpty(t, clientToken)
// 	assert.NotEmpty(t, requestID)

// 	// 2. Starts pending
// 	p, err := svc.GetPaymentStatus(bID)
// 	require.NoError(t, err)
// 	assert.Equal(t, models.PaymentPending, p.Status)
// 	assert.False(t, p.WebhookReceived)

// 	// 3. Stitch fires webhook
// 	require.NoError(t, svc.HandleWebhook(whEvent(requestID, "completed", "credit_card", "txn_life")))

// 	// 4. Confirmed
// 	p, err = svc.GetPaymentByBookingID(context.Background(), bID)
// 	require.NoError(t, err)
// 	assert.Equal(t, models.PaymentCompleted, p.Status)
// 	assert.True(t, p.WebhookReceived)
// 	bs, _ := whGetStatuses(t, pool, bID)
// 	assert.Equal(t, "confirmed", bs)
// }

// func TestWHRetryAfterFailure(t *testing.T) {
// 	pool := whDB(t)
// 	defer pool.Close()
// 	whTruncate(t, pool)

// 	call := 0
// 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		w.Header().Set("Content-Type", "application/json")
// 		if r.URL.Path == "/connect/token" {
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"access_token": "tok", "token_type": "Bearer", "expires_in": 3600,
// 			})
// 			return
// 		}
// 		call++
// 		data, _ := json.Marshal(map[string]interface{}{
// 			"clientCardPaymentInitiate": map[string]interface{}{
// 				"paymentInitiation": map[string]interface{}{
// 					"id":          fmt.Sprintf("retry_req_%d", call),
// 					"clientToken": fmt.Sprintf("retry_tok_%d", call),
// 				},
// 			},
// 		})
// 		json.NewEncoder(w).Encode(map[string]interface{}{"data": json.RawMessage(data), "errors": nil})
// 	}))
// 	defer srv.Close()
// 	whRedirect(t, srv.URL)

// 	svc := NewPaymentService(pool, whCfg())
// 	bID := whSeedBooking(t, pool)
// 	b := whMakeBooking(bID)

// 	// First attempt fails
// 	tok1, req1, err := svc.CreatePaymentIntent(b)
// 	require.NoError(t, err)
// 	assert.Equal(t, "retry_tok_1", tok1)
// 	require.NoError(t, svc.HandleWebhook(whEvent(req1, "failed", "credit_card", "")))
// 	bs, _ := whGetStatuses(t, pool, bID)
// 	assert.Equal(t, "cancelled", bs)

// 	// Retry succeeds
// 	tok2, req2, err := svc.CreatePaymentIntent(b)
// 	require.NoError(t, err)
// 	assert.Equal(t, "retry_tok_2", tok2)
// 	assert.NotEqual(t, req1, req2)
// 	require.NoError(t, svc.HandleWebhook(whEvent(req2, "completed", "credit_card", "txn_retry")))
// 	bs, _ = whGetStatuses(t, pool, bID)
// 	assert.Equal(t, "confirmed", bs)
// }