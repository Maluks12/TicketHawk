package services_test

// Webhook handler HTTP-contract tests.
//
// These verify the Paystack webhook handler's signature gating and routing
// without any network calls:
//   - bad / missing signature            → 401
//   - valid signature, unhandled event   → 200 (no provider re-verification)
//   - valid signature, malformed JSON    → 400
//
// The "valid signature + charge.success" happy path is deliberately NOT exercised
// here: the handler re-verifies the charge against Paystack's live API, which is
// out of scope for a unit test. The DB side-effects of a verified charge are
// covered exhaustively in payment_service_test.go via ApplyVerifiedPayment, and
// PayPal application is covered there too (Gateway = PayPal).
//
// This file lives in the external services_test package so it can import the
// handlers package (which imports services) without creating an import cycle.
//
// Prerequisites:
//
//	docker run -d --name test-postgres \
//	  -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
//	  -e POSTGRES_DB=test_db -p 5433:5432 postgres:15
//
// Run:
//
//	go test ./internal/services/... -v -run TestWebhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"events-db/internal/config"
	"events-db/internal/handlers"
	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const whTestDBURL = "postgresql://testuser:testpass@localhost:5433/test_db"
const whPaystackSecret = "sk_test_webhook_secret"

func whConfig() *config.Config {
	return &config.Config{
		JWTSecret:          "test-jwt-secret-that-is-32chars!!",
		PaystackSecretKey:  whPaystackSecret,
		PaystackPublicKey:  "pk_test_webhook_public",
		PayPalClientID:     "paypal-client-id",
		PayPalClientSecret: "paypal-client-secret",
		PayPalWebhookID:    "paypal-webhook-id",
		PayPalEnvironment:  "sandbox",
		PaymentCallbackURL: "http://localhost:3000/payments/return",
	}
}

// whService builds a PaymentService against the test DB. The pool is only used
// for the cases that fall through to a DB read; the signature/routing cases
// short-circuit before touching it.
func whService(t *testing.T) (*services.PaymentService, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), whTestDBURL)
	if err != nil {
		t.Skipf("webhook tests skipped — could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("webhook tests skipped — DB not reachable: %v", pingErr)
	}
	return services.NewPaymentService(pool, whConfig(), make(chan string, 16)), pool
}

func whRouter(svc *services.PaymentService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/webhooks/paystack", handlers.PaystackWebhookHandler(svc))
	return r
}

// whSign computes the Paystack HMAC-SHA512 signature for a body.
func whSign(body []byte, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func whPost(r *gin.Engine, body []byte, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/paystack", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("x-paystack-signature", signature)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestWebhookPaystackSignature(t *testing.T) {
	svc, pool := whService(t)
	defer pool.Close()
	r := whRouter(svc)

	t.Run("missing signature returns 401", func(t *testing.T) {
		body := []byte(`{"event":"charge.success","data":{"reference":"bk_1"}}`)
		w := whPost(r, body, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("wrong signature returns 401", func(t *testing.T) {
		body := []byte(`{"event":"charge.success","data":{"reference":"bk_1"}}`)
		w := whPost(r, body, "not-the-right-hmac")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("signature computed with a different secret returns 401", func(t *testing.T) {
		body := []byte(`{"event":"charge.success","data":{"reference":"bk_1"}}`)
		w := whPost(r, body, whSign(body, "the-wrong-secret"))
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestWebhookPaystackRouting(t *testing.T) {
	svc, pool := whService(t)
	defer pool.Close()
	r := whRouter(svc)

	t.Run("valid signature + unhandled event returns 200 (no re-verify)", func(t *testing.T) {
		// A non-charge event is acknowledged without contacting Paystack.
		body := []byte(`{"event":"transfer.success","data":{"reference":"trf_1"}}`)
		w := whPost(r, body, whSign(body, whPaystackSecret))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, true, resp["received"])
	})

	t.Run("valid signature + missing reference is acknowledged with 200", func(t *testing.T) {
		body := []byte(`{"event":"charge.success","data":{"reference":""}}`)
		w := whPost(r, body, whSign(body, whPaystackSecret))
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("valid signature + malformed JSON returns 400", func(t *testing.T) {
		body := []byte(`{not valid json`)
		w := whPost(r, body, whSign(body, whPaystackSecret))
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})
}