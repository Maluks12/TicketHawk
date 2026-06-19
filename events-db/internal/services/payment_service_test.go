package services

// Payment service tests (gateway-neutral architecture).
//
// These exercise the DB-orchestration core of PaymentService without any network
// calls: ApplyVerifiedPayment (booking/cart/top-up/listing-fee routing),
// createPaymentRecord, GetPaymentByBookingID, syncBookingStatus, the refund
// pre-flight validation, and the pure status/method/amount mappers plus the
// Paystack webhook-signature check.
//
// The actual provider HTTP calls (Paystack initialize/verify/refund, PayPal
// order create/capture, webhook re-verification) hit live endpoints and are not
// unit-tested here; their results are fed in as VerifyResult values instead.
//
// Prerequisites:
//
//	docker run -d --name test-postgres \
//	  -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
//	  -e POSTGRES_DB=test_db -p 5433:5432 postgres:15
//
// Run:
//
//	go test ./internal/services/... -v -run TestPayment

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"events-db/internal/config"
	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// paystackSignatureFor recomputes the x-paystack-signature header value
// (HMAC-SHA512 of the raw body, hex-encoded) for tests.
func paystackSignatureFor(body []byte, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

const paymentTestDBURL = "postgresql://testuser:testpass@localhost:5433/test_db"

func paymentTestConfig() *config.Config {
	return &config.Config{
		JWTSecret:          "test-jwt-secret-that-is-32chars!!",
		PaystackSecretKey:  "sk_test_paystack_secret_key",
		PaystackPublicKey:  "pk_test_paystack_public_key",
		PayPalClientID:     "paypal-client-id",
		PayPalClientSecret: "paypal-client-secret",
		PayPalWebhookID:    "paypal-webhook-id",
		PayPalEnvironment:  "sandbox",
		PaymentCallbackURL: "http://localhost:3000/payments/return",
	}
}

func paymentJobChan() chan string { return make(chan string, 256) }

// ── DB setup ──────────────────────────────────────────────────────────────────

func setupPaymentDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), paymentTestDBURL)
	if err != nil {
		t.Skipf("payment DB tests skipped — could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("payment DB tests skipped — DB not reachable: %v", pingErr)
	}
	createPaymentTables(t, pool)
	cleanPaymentTables(pool)
	return pool
}

func createPaymentTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	for _, s := range []string{
		"DROP TABLE IF EXISTS payments         CASCADE",
		"DROP TABLE IF EXISTS merch_orders     CASCADE",
		"DROP TABLE IF EXISTS cart_items       CASCADE",
		"DROP TABLE IF EXISTS merchandise_variants CASCADE",
		"DROP TABLE IF EXISTS merchandise      CASCADE",
		"DROP TABLE IF EXISTS bookings         CASCADE",
		"DROP TABLE IF EXISTS carts            CASCADE",
		"DROP TABLE IF EXISTS ad_wallet_topups CASCADE",
		"DROP TABLE IF EXISTS ad_wallets       CASCADE",
		"DROP TABLE IF EXISTS events           CASCADE",
		"DROP TYPE  IF EXISTS topup_status",
		"DROP SEQUENCE IF EXISTS ad_topup_ref_seq",
	} {
		pool.Exec(ctx, s)
	}

	for _, s := range []string{
		`DO $$ BEGIN
			CREATE TYPE payment_method_type AS ENUM (
				'credit_card','paypal','apple_pay','google_pay','samsung_pay','instant_eft');
		EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		`DO $$ BEGIN
			CREATE TYPE topup_status AS ENUM ('pending','completed','failed');
		EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		`CREATE SEQUENCE IF NOT EXISTS ad_topup_ref_seq START 1`,

		`CREATE TABLE carts (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id          UUID,
			session_id       UUID,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE TABLE bookings (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_number   VARCHAR(50) UNIQUE NOT NULL DEFAULT 'BK-' || substr(gen_random_uuid()::text,1,8),
			event_id         UUID NOT NULL DEFAULT gen_random_uuid(),
			user_id          UUID          DEFAULT gen_random_uuid(),
			total_amount     NUMERIC(10,2) NOT NULL DEFAULT 0,
			currency         VARCHAR(10) NOT NULL DEFAULT 'ZAR',
			booking_status   VARCHAR(50) NOT NULL DEFAULT 'pending',
			payment_status   VARCHAR(50) NOT NULL DEFAULT 'pending',
			cart_id          UUID REFERENCES carts(id) ON DELETE SET NULL,
			confirmed_at     TIMESTAMPTZ,
			cancelled_at     TIMESTAMPTZ,
			updated_at       TIMESTAMPTZ DEFAULT NOW(),
			created_at       TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE payments (
			id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_id           UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
			payment_gateway      VARCHAR(50) NOT NULL DEFAULT 'paystack',
			payment_intent_id    TEXT,
			transaction_id       TEXT,
			amount               NUMERIC(10,2) NOT NULL,
			currency             VARCHAR(10) NOT NULL DEFAULT 'ZAR',
			payment_method       payment_method_type NOT NULL DEFAULT 'credit_card',
			status               VARCHAR(50) NOT NULL DEFAULT 'pending',
			gateway_reference    TEXT,
			gateway_redirect_url TEXT,
			cart_id              UUID REFERENCES carts(id) ON DELETE SET NULL,
			refund_id            TEXT,
			refunded_amount      NUMERIC(10,2),
			refunded_at          TIMESTAMPTZ,
			webhook_received     BOOLEAN DEFAULT FALSE,
			completed_at         TIMESTAMPTZ,
			created_at           TIMESTAMPTZ DEFAULT NOW(),
			updated_at           TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(booking_id)
		)`,

		// Per-organiser wallet (for top-up application tests).
		`CREATE TABLE ad_wallets (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			organizer_id UUID UNIQUE NOT NULL,
			balance      NUMERIC(10,2) NOT NULL DEFAULT 0.00,
			total_topups NUMERIC(10,2) NOT NULL DEFAULT 0.00,
			total_spent  NUMERIC(10,2) NOT NULL DEFAULT 0.00,
			created_at   TIMESTAMPTZ DEFAULT NOW(),
			updated_at   TIMESTAMPTZ DEFAULT NOW(),
			CONSTRAINT chk_wallet_balance_non_negative CHECK (balance >= 0)
		)`,

		`CREATE TABLE ad_wallet_topups (
			id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			organizer_id       UUID NOT NULL,
			amount             NUMERIC(10,2) NOT NULL,
			paystack_reference VARCHAR(255) UNIQUE NOT NULL,
			reference_number   VARCHAR(20) UNIQUE,
			status             topup_status NOT NULL DEFAULT 'pending',
			created_at         TIMESTAMPTZ DEFAULT NOW(),
			completed_at       TIMESTAMPTZ
		)`,

		// Minimal events table so the listing-fee lookup runs cleanly.
		`CREATE TABLE events (
			id                             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			price                          NUMERIC(10,2) NOT NULL DEFAULT 0.00,
			listing_fee_paid               BOOLEAN NOT NULL DEFAULT FALSE,
			listing_fee_payment_request_id TEXT,
			updated_at                     TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Minimal merch tables so cart fulfilment runs cleanly (no rows → no-op).
		`CREATE TABLE merchandise (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			pickup_location TEXT
		)`,
		`CREATE TABLE merchandise_variants (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			merchandise_id UUID
		)`,
		`CREATE TABLE cart_items (
			id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			cart_id                UUID,
			item_type              TEXT,
			merchandise_variant_id UUID,
			event_id               UUID,
			quantity               INT,
			unit_price_snapshot    NUMERIC(10,2),
			fulfillment_method     TEXT,
			delivery_address       JSONB
		)`,
		`CREATE TABLE merch_orders (
			id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			cart_id               UUID,
			stitch_payment_req_id TEXT
		)`,
	} {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Skipf("payment DB tests skipped — table setup failed: %v", err)
		}
	}
}

func cleanPaymentTables(pool *pgxpool.Pool) {
	ctx := context.Background()
	for _, tbl := range []string{
		"payments", "merch_orders", "cart_items", "merchandise_variants",
		"merchandise", "bookings", "carts", "ad_wallet_topups", "ad_wallets", "events",
	} {
		pool.Exec(ctx, "TRUNCATE TABLE "+tbl+" CASCADE")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func seedBooking(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO bookings (total_amount, currency) VALUES (100.00, 'ZAR') RETURNING id`,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedCart(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO carts (session_id) VALUES (gen_random_uuid()) RETURNING id`,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedBookingForCart(t *testing.T, pool *pgxpool.Pool, cartID string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO bookings (total_amount, currency, cart_id) VALUES (100.00, 'ZAR', $1) RETURNING id`,
		cartID,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// seedPayment inserts a pending payment keyed by gateway_reference.
func seedPayment(t *testing.T, pool *pgxpool.Pool, bookingID, reference string, cartID *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO payments
			(booking_id, payment_gateway, payment_intent_id, amount, currency,
			 payment_method, status, gateway_reference, cart_id)
		VALUES ($1, 'paystack', $2, 100.00, 'ZAR', 'credit_card', 'pending', $2, $3)
	`, bookingID, reference, cartID)
	require.NoError(t, err)
}

func makeBooking(bookingID string) *models.Booking {
	return &models.Booking{
		ID:            bookingID,
		BookingNumber: fmt.Sprintf("BK-%d", time.Now().UnixNano()),
		TotalAmount:   100.00,
		Currency:      "ZAR",
	}
}

func bookingStatuses(t *testing.T, pool *pgxpool.Pool, bookingID string) (bookingSt, paymentSt string) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT booking_status, payment_status FROM bookings WHERE id = $1`, bookingID,
	).Scan(&bookingSt, &paymentSt)
	require.NoError(t, err)
	return
}

func verified(gw PaymentGatewayName, ref string, status models.PaymentStatus, method models.PaymentMethod, txn string) *VerifyResult {
	return &VerifyResult{
		Gateway:       gw,
		Reference:     ref,
		Status:        status,
		Method:        method,
		TransactionID: txn,
		AmountCents:   10000,
		Currency:      "ZAR",
	}
}

// ── createPaymentRecord ─────────────────────────────────────────────────────

func TestPaymentCreateRecord(t *testing.T) {
	pool := setupPaymentDB(t)
	defer pool.Close()
	svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())
	ctx := context.Background()

	t.Run("inserts a gateway-neutral record (no cart)", func(t *testing.T) {
		cleanPaymentTables(pool)
		bID := seedBooking(t, pool)

		err := svc.createPaymentRecord(ctx, makeBooking(bID), GatewayPaystack, "bk_new", nil, models.PaymentCreditCard, nil)
		require.NoError(t, err)

		p, err := svc.GetPaymentByBookingID(ctx, bID)
		require.NoError(t, err)
		assert.Equal(t, "paystack", p.PaymentGateway)
		assert.Equal(t, models.PaymentPending, p.Status)
		require.NotNil(t, p.GatewayReference)
		assert.Equal(t, "bk_new", *p.GatewayReference)
		assert.Nil(t, p.GatewayRedirectURL)
	})

	t.Run("stores cart_id and redirect URL when provided", func(t *testing.T) {
		cleanPaymentTables(pool)
		cartID := seedCart(t, pool)
		bID := seedBookingForCart(t, pool, cartID)
		redirect := "https://checkout.paystack.com/abc"

		err := svc.createPaymentRecord(ctx, makeBooking(bID), GatewayPaystack, "cart_x", &redirect, models.PaymentCreditCard, &cartID)
		require.NoError(t, err)

		var storedCart *string
		var storedRedirect *string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT cart_id, gateway_redirect_url FROM payments WHERE booking_id = $1`, bID,
		).Scan(&storedCart, &storedRedirect))
		require.NotNil(t, storedCart)
		assert.Equal(t, cartID, *storedCart)
		require.NotNil(t, storedRedirect)
		assert.Equal(t, redirect, *storedRedirect)
	})

	t.Run("upserts on duplicate booking — latest reference wins", func(t *testing.T) {
		cleanPaymentTables(pool)
		bID := seedBooking(t, pool)

		require.NoError(t, svc.createPaymentRecord(ctx, makeBooking(bID), GatewayPaystack, "bk_first", nil, models.PaymentCreditCard, nil))
		require.NoError(t, svc.createPaymentRecord(ctx, makeBooking(bID), GatewayPayPal, "bk_second", nil, models.PaymentPayPal, nil))

		p, err := svc.GetPaymentByBookingID(ctx, bID)
		require.NoError(t, err)
		assert.Equal(t, "bk_second", *p.GatewayReference)
		assert.Equal(t, "paypal", p.PaymentGateway)
	})
}

// ── GetPaymentByBookingID ───────────────────────────────────────────────────

func TestPaymentGetByBookingID(t *testing.T) {
	pool := setupPaymentDB(t)
	defer pool.Close()
	svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())
	ctx := context.Background()

	t.Run("returns the payment for an existing booking", func(t *testing.T) {
		cleanPaymentTables(pool)
		bID := seedBooking(t, pool)
		seedPayment(t, pool, bID, "bk_get", nil)

		p, err := svc.GetPaymentByBookingID(ctx, bID)
		require.NoError(t, err)
		assert.Equal(t, bID, p.BookingID)
		assert.Equal(t, models.PaymentPending, p.Status)
		require.NotNil(t, p.GatewayReference)
		assert.Equal(t, "bk_get", *p.GatewayReference)
		assert.False(t, p.WebhookReceived)
	})

	t.Run("errors for a non-existent booking", func(t *testing.T) {
		cleanPaymentTables(pool)
		_, err := svc.GetPaymentByBookingID(ctx, "00000000-0000-0000-0000-000000000000")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "payment not found")
	})
}

// ── ApplyVerifiedPayment: bookings ──────────────────────────────────────────

func TestPaymentApplyVerified_Booking(t *testing.T) {
	pool := setupPaymentDB(t)
	defer pool.Close()
	ctx := context.Background()

	t.Run("completed confirms booking and enqueues a job", func(t *testing.T) {
		cleanPaymentTables(pool)
		jobCh := make(chan string, 4)
		svc := NewPaymentService(pool, paymentTestConfig(), jobCh)
		bID := seedBooking(t, pool)
		seedPayment(t, pool, bID, "bk_done", nil)

		require.NoError(t, svc.ApplyVerifiedPayment(ctx,
			verified(GatewayPaystack, "bk_done", models.PaymentCompleted, models.PaymentCreditCard, "txn_1")))

		p, err := svc.GetPaymentByBookingID(ctx, bID)
		require.NoError(t, err)
		assert.Equal(t, models.PaymentCompleted, p.Status)
		assert.True(t, p.WebhookReceived)
		require.NotNil(t, p.TransactionID)
		assert.Equal(t, "txn_1", *p.TransactionID)
		assert.NotNil(t, p.CompletedAt)

		bs, ps := bookingStatuses(t, pool, bID)
		assert.Equal(t, "confirmed", bs)
		assert.Equal(t, "completed", ps)

		select {
		case id := <-jobCh:
			assert.Equal(t, bID, id)
		default:
			t.Error("expected a booking job to be enqueued")
		}
	})

	t.Run("paypal completed routes through the booking path", func(t *testing.T) {
		cleanPaymentTables(pool)
		jobCh := make(chan string, 4)
		svc := NewPaymentService(pool, paymentTestConfig(), jobCh)
		bID := seedBooking(t, pool)
		seedPayment(t, pool, bID, "ORDER-9GH", nil) // PayPal order id as reference

		require.NoError(t, svc.ApplyVerifiedPayment(ctx,
			verified(GatewayPayPal, "ORDER-9GH", models.PaymentCompleted, models.PaymentPayPal, "CAP-1")))

		p, err := svc.GetPaymentByBookingID(ctx, bID)
		require.NoError(t, err)
		assert.Equal(t, models.PaymentCompleted, p.Status)
		assert.Equal(t, models.PaymentPayPal, p.PaymentMethod)
	})

	t.Run("failed cancels the booking", func(t *testing.T) {
		cleanPaymentTables(pool)
		svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())
		bID := seedBooking(t, pool)
		seedPayment(t, pool, bID, "bk_fail", nil)

		require.NoError(t, svc.ApplyVerifiedPayment(ctx,
			verified(GatewayPaystack, "bk_fail", models.PaymentFailed, models.PaymentCreditCard, "")))

		bs, ps := bookingStatuses(t, pool, bID)
		assert.Equal(t, "cancelled", bs)
		assert.Equal(t, "failed", ps)
	})

	t.Run("processing leaves the booking pending", func(t *testing.T) {
		cleanPaymentTables(pool)
		svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())
		bID := seedBooking(t, pool)
		seedPayment(t, pool, bID, "bk_proc", nil)

		require.NoError(t, svc.ApplyVerifiedPayment(ctx,
			verified(GatewayPaystack, "bk_proc", models.PaymentProcessing, models.PaymentCreditCard, "")))

		bs, ps := bookingStatuses(t, pool, bID)
		assert.Equal(t, "pending", bs)
		assert.Equal(t, "processing", ps)
	})

	t.Run("apple pay method is recorded", func(t *testing.T) {
		cleanPaymentTables(pool)
		svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())
		bID := seedBooking(t, pool)
		seedPayment(t, pool, bID, "bk_apple", nil)

		require.NoError(t, svc.ApplyVerifiedPayment(ctx,
			verified(GatewayPaystack, "bk_apple", models.PaymentCompleted, models.PaymentApplePay, "txn_apple")))

		p, err := svc.GetPaymentByBookingID(ctx, bID)
		require.NoError(t, err)
		assert.Equal(t, models.PaymentApplePay, p.PaymentMethod)
	})

	t.Run("duplicate delivery is idempotent", func(t *testing.T) {
		cleanPaymentTables(pool)
		svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())
		bID := seedBooking(t, pool)
		seedPayment(t, pool, bID, "bk_idem", nil)

		v := verified(GatewayPaystack, "bk_idem", models.PaymentCompleted, models.PaymentCreditCard, "txn_idem")
		require.NoError(t, svc.ApplyVerifiedPayment(ctx, v))
		require.NoError(t, svc.ApplyVerifiedPayment(ctx, v))

		bs, _ := bookingStatuses(t, pool, bID)
		assert.Equal(t, "confirmed", bs)
	})

	t.Run("unknown reference is ignored without error", func(t *testing.T) {
		cleanPaymentTables(pool)
		svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())
		err := svc.ApplyVerifiedPayment(ctx,
			verified(GatewayPaystack, "bk_nope", models.PaymentCompleted, models.PaymentCreditCard, ""))
		assert.NoError(t, err)
	})
}

// ── ApplyVerifiedPayment: cart ──────────────────────────────────────────────

func TestPaymentApplyVerified_Cart(t *testing.T) {
	pool := setupPaymentDB(t)
	defer pool.Close()
	ctx := context.Background()

	t.Run("completed confirms ALL bookings in the cart and enqueues both", func(t *testing.T) {
		cleanPaymentTables(pool)
		jobCh := make(chan string, 8)
		svc := NewPaymentService(pool, paymentTestConfig(), jobCh)

		cartID := seedCart(t, pool)
		primary := seedBookingForCart(t, pool, cartID)
		secondary := seedBookingForCart(t, pool, cartID)
		seedPayment(t, pool, primary, "cart_done", &cartID)

		require.NoError(t, svc.ApplyVerifiedPayment(ctx,
			verified(GatewayPaystack, "cart_done", models.PaymentCompleted, models.PaymentCreditCard, "txn_cart")))

		bs1, _ := bookingStatuses(t, pool, primary)
		bs2, _ := bookingStatuses(t, pool, secondary)
		assert.Equal(t, "confirmed", bs1)
		assert.Equal(t, "confirmed", bs2, "secondary cart booking must also be confirmed")

		got := map[string]bool{}
		for i := 0; i < 2; i++ {
			select {
			case id := <-jobCh:
				got[id] = true
			case <-time.After(150 * time.Millisecond):
				t.Fatalf("expected 2 jobs, got %d", len(got))
			}
		}
		assert.True(t, got[primary])
		assert.True(t, got[secondary])
	})

	t.Run("failed cancels ALL bookings in the cart", func(t *testing.T) {
		cleanPaymentTables(pool)
		svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())

		cartID := seedCart(t, pool)
		primary := seedBookingForCart(t, pool, cartID)
		secondary := seedBookingForCart(t, pool, cartID)
		seedPayment(t, pool, primary, "cart_fail", &cartID)

		require.NoError(t, svc.ApplyVerifiedPayment(ctx,
			verified(GatewayPaystack, "cart_fail", models.PaymentFailed, models.PaymentCreditCard, "")))

		bs1, _ := bookingStatuses(t, pool, primary)
		bs2, _ := bookingStatuses(t, pool, secondary)
		assert.Equal(t, "cancelled", bs1)
		assert.Equal(t, "cancelled", bs2)
	})
}

// ── ApplyVerifiedPayment: ad-wallet top-ups ─────────────────────────────────

func TestPaymentApplyVerified_Topup(t *testing.T) {
	pool := setupPaymentDB(t)
	defer pool.Close()
	ctx := context.Background()
	svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())

	orgID := "11111111-1111-1111-1111-111111111111"

	t.Run("completed top-up credits the wallet", func(t *testing.T) {
		cleanPaymentTables(pool)
		_, err := pool.Exec(ctx, `INSERT INTO ad_wallets (organizer_id, balance) VALUES ($1, 0)`, orgID)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `
			INSERT INTO ad_wallet_topups (organizer_id, amount, paystack_reference, reference_number, status)
			VALUES ($1, 500.00, 'topup_ok', 'TUP-00001', 'pending')`, orgID)
		require.NoError(t, err)

		require.NoError(t, svc.ApplyVerifiedPayment(ctx,
			verified(GatewayPaystack, "topup_ok", models.PaymentCompleted, models.PaymentCreditCard, "")))

		var balance float64
		var status string
		require.NoError(t, pool.QueryRow(ctx, `SELECT balance FROM ad_wallets WHERE organizer_id = $1`, orgID).Scan(&balance))
		require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM ad_wallet_topups WHERE paystack_reference = 'topup_ok'`).Scan(&status))
		assert.Equal(t, 500.00, balance)
		assert.Equal(t, "completed", status)
	})

	t.Run("failed top-up leaves the balance untouched", func(t *testing.T) {
		cleanPaymentTables(pool)
		_, err := pool.Exec(ctx, `INSERT INTO ad_wallets (organizer_id, balance) VALUES ($1, 0)`, orgID)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `
			INSERT INTO ad_wallet_topups (organizer_id, amount, paystack_reference, reference_number, status)
			VALUES ($1, 500.00, 'topup_bad', 'TUP-00002', 'pending')`, orgID)
		require.NoError(t, err)

		require.NoError(t, svc.ApplyVerifiedPayment(ctx,
			verified(GatewayPaystack, "topup_bad", models.PaymentFailed, models.PaymentCreditCard, "")))

		var balance float64
		var status string
		require.NoError(t, pool.QueryRow(ctx, `SELECT balance FROM ad_wallets WHERE organizer_id = $1`, orgID).Scan(&balance))
		require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM ad_wallet_topups WHERE paystack_reference = 'topup_bad'`).Scan(&status))
		assert.Equal(t, 0.00, balance)
		assert.Equal(t, "failed", status)
	})
}

// ── ApplyVerifiedPayment: listing fee ───────────────────────────────────────

func TestPaymentApplyVerified_ListingFee(t *testing.T) {
	pool := setupPaymentDB(t)
	defer pool.Close()
	ctx := context.Background()
	svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())

	cleanPaymentTables(pool)
	var eventID string
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO events (price, listing_fee_paid, listing_fee_payment_request_id)
		VALUES (0, FALSE, 'lf_free_event') RETURNING id
	`).Scan(&eventID))

	require.NoError(t, svc.ApplyVerifiedPayment(ctx,
		verified(GatewayPaystack, "lf_free_event", models.PaymentCompleted, models.PaymentCreditCard, "")))

	var paid bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT listing_fee_paid FROM events WHERE id = $1`, eventID).Scan(&paid))
	assert.True(t, paid, "listing fee should be marked paid")
}

// ── syncBookingStatus ───────────────────────────────────────────────────────

func TestPaymentSyncBookingStatus(t *testing.T) {
	pool := setupPaymentDB(t)
	defer pool.Close()
	svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())
	ctx := context.Background()

	cases := []struct {
		name    string
		status  models.PaymentStatus
		booking string
	}{
		{"completed → confirmed", models.PaymentCompleted, "confirmed"},
		{"failed → cancelled", models.PaymentFailed, "cancelled"},
		{"pending → pending", models.PaymentPending, "pending"},
		{"processing → pending", models.PaymentProcessing, "pending"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleanPaymentTables(pool)
			bID := seedBooking(t, pool)
			require.NoError(t, svc.syncBookingStatus(ctx, bID, tc.status))
			bs, _ := bookingStatuses(t, pool, bID)
			assert.Equal(t, tc.booking, bs)
		})
	}
}

// ── ProcessRefund: pre-flight validation (no gateway HTTP) ──────────────────

func TestPaymentProcessRefund_Validation(t *testing.T) {
	pool := setupPaymentDB(t)
	defer pool.Close()
	svc := NewPaymentService(pool, paymentTestConfig(), paymentJobChan())

	t.Run("no payment record", func(t *testing.T) {
		cleanPaymentTables(pool)
		err := svc.ProcessRefund("00000000-0000-0000-0000-000000000000")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fetching payment")
	})

	t.Run("payment not completed", func(t *testing.T) {
		cleanPaymentTables(pool)
		bID := seedBooking(t, pool)
		seedPayment(t, pool, bID, "bk_pending", nil)
		err := svc.ProcessRefund(bID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not completed")
	})

	t.Run("completed but missing gateway reference", func(t *testing.T) {
		cleanPaymentTables(pool)
		bID := seedBooking(t, pool)
		_, err := pool.Exec(context.Background(), `
			INSERT INTO payments (booking_id, payment_gateway, amount, currency, payment_method, status)
			VALUES ($1, 'paystack', 100.00, 'ZAR', 'credit_card', 'completed')
		`, bID)
		require.NoError(t, err)
		err = svc.ProcessRefund(bID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no gateway reference")
	})
}

// ── Pure mappers ────────────────────────────────────────────────────────────

func TestPaystackStatusMapping(t *testing.T) {
	cases := map[string]models.PaymentStatus{
		"success":   models.PaymentCompleted,
		"SUCCESS":   models.PaymentCompleted,
		"ongoing":   models.PaymentProcessing,
		"pending":   models.PaymentProcessing,
		"failed":    models.PaymentFailed,
		"abandoned": models.PaymentFailed,
		"reversed":  models.PaymentFailed,
		"":          models.PaymentPending,
		"weird":     models.PaymentPending,
	}
	for in, want := range cases {
		assert.Equal(t, want, paystackStatusToInternal(in), "input %q", in)
	}
}

func TestPaystackChannelMapping(t *testing.T) {
	assert.Equal(t, models.PaymentApplePay, paystackChannelToMethod("apple_pay"))
	assert.Equal(t, models.PaymentApplePay, paystackChannelToMethod("applepay"))
	assert.Equal(t, models.PaymentCreditCard, paystackChannelToMethod("card"))
	assert.Equal(t, models.PaymentCreditCard, paystackChannelToMethod("eft"))
	assert.Equal(t, models.PaymentCreditCard, paystackChannelToMethod(""))
}

func TestPayPalStatusMapping(t *testing.T) {
	cases := map[string]models.PaymentStatus{
		"COMPLETED": models.PaymentCompleted,
		"CAPTURED":  models.PaymentCompleted,
		"APPROVED":  models.PaymentProcessing,
		"PENDING":   models.PaymentProcessing,
		"VOIDED":    models.PaymentFailed,
		"DECLINED":  models.PaymentFailed,
		"FAILED":    models.PaymentFailed,
		"":          models.PaymentPending,
		"weird":     models.PaymentPending,
	}
	for in, want := range cases {
		assert.Equal(t, want, paypalStatusToInternal(in), "input %q", in)
	}
}

func TestPayPalAmountConversion(t *testing.T) {
	assert.Equal(t, int64(10000), decimalToCents("100.00"))
	assert.Equal(t, int64(12345), decimalToCents("123.45"))
	assert.Equal(t, int64(0), decimalToCents(""))
	assert.Equal(t, "100.00", centsToDecimal(10000))
	assert.Equal(t, "123.45", centsToDecimal(12345))
}

// ── Paystack webhook signature ──────────────────────────────────────────────

func TestPaystackVerifySignature(t *testing.T) {
	gw := NewPaystackGateway("sk_test_secret")
	body := []byte(`{"event":"charge.success","data":{"reference":"bk_1"}}`)

	// Recompute the expected HMAC-SHA512 the same way the gateway does.
	good := paystackSignatureFor(body, "sk_test_secret")
	assert.True(t, gw.VerifySignature(body, good), "correct signature must verify")
	assert.False(t, gw.VerifySignature(body, "deadbeef"), "wrong signature must fail")
	assert.False(t, gw.VerifySignature(body, ""), "empty signature must fail")
}