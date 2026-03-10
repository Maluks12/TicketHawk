package services

// // Integration tests for EmailService and QRService.
// //
// // EMAIL TESTS — actually deliver mail via SendGrid to ribnoma@gmail.com.
// // QR TESTS    — unit-test encrypt/decrypt without a DB; DB-backed tests
// //               require the same test-postgres container used by payment tests.
// //
// // Prerequisites
// // ─────────────
// // Set the following environment variables before running:
// //
// //   export SENDGRID_API_KEY="SG.xxxx"
// //   export EMAIL_FROM_ADDRESS="you@yourdomain.com"
// //   export EMAIL_FROM_NAME="Events Platform"
// //   export FRONTEND_URL="http://localhost:3000"
// //   export QR_ENCRYPTION_KEY="0123456789abcdef0123456789abcdef"  # exactly 32 bytes
// //
// // Optional (only needed for QR DB tests):
// //   export TEST_DB_URL="postgresql://testuser:testpass@localhost:5433/test_db"
// //
// // Run all tests in this file:
// //
// //   go test ./internal/services/... -v -run "TestEmail|TestQR"
// //
// // Run only the email send tests (requires real SendGrid key):
// //
// //   go test ./internal/services/... -v -run TestEmailSend

// import (
// 	"context"
// 	"fmt"
// 	"os"
// 	"path/filepath"
// 	"strings"
// 	"testing"
// 	"time"

// 	"events-db/internal/config"
// 	"events-db/internal/models"
// 	"github.com/joho/godotenv"

// 	"github.com/jackc/pgx/v5/pgxpool"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// )

// // ============================================================
// // Shared test fixtures
// // ============================================================

// const testRecipient = "ribnoma@gmail.com"

// // loadRootEnv loads the .env from the project root regardless of which
// // sub-package directory go test runs from. It walks up from the current
// // working directory (which go test sets to the package folder) until it
// // finds go.mod, then loads .env from there.
// func loadRootEnv() {
// 	dir, err := os.Getwd()
// 	if err != nil {
// 		return
// 	}
// 	for {
// 		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
// 			_ = godotenv.Load(filepath.Join(dir, ".env"))
// 			return
// 		}
// 		parent := filepath.Dir(dir)
// 		if parent == dir {
// 			break
// 		}
// 		dir = parent
// 	}
// }

// func emailTestConfig(t *testing.T) *config.Config {
// 	t.Helper()
// 	loadRootEnv()

// 	cfg, err := config.Load()
// 	if err != nil {
// 		t.Fatalf("failed to load config: %v", err)
// 	}

// 	if cfg.SendGridAPIKey == "" {
// 		t.Skip("SENDGRID_API_KEY not set — skipping live email tests")
// 	}
// 	if cfg.EmailFromAddress == "" {
// 		t.Skip("EMAIL_FROM_ADDRESS not set — skipping live email tests")
// 	}

// 	// Apply fallbacks for optional fields not set in .env
// 	if cfg.EmailFromName == "" {
// 		cfg.EmailFromName = "Events Platform Test"
// 	}
// 	if cfg.FrontendURL == "" {
// 		cfg.FrontendURL = "http://localhost:3000"
// 	}

// 	// Ensure QR key is valid for tests that need it
// 	cfg.QREncryptionKey = qrKeyOrSkip(t)

// 	return cfg
// }

// func qrTestConfig(t *testing.T) *config.Config {
// 	t.Helper()
// 	loadRootEnv()

// 	cfg, err := config.Load()
// 	if err != nil {
// 		t.Fatalf("failed to load config: %v", err)
// 	}

// 	cfg.QREncryptionKey = qrKeyOrSkip(t)
// 	return cfg
// }

// func qrKeyOrSkip(t *testing.T) string {
// 	t.Helper()
// 	key := os.Getenv("QR_ENCRYPTION_KEY")
// 	if key == "" {
// 		// Use a deterministic 32-byte key so unit tests work without env vars.
// 		return "testkeyfortesting1234567890abcde"
// 	}
// 	if len(key) != 32 {
// 		t.Fatalf("QR_ENCRYPTION_KEY must be exactly 32 bytes, got %d", len(key))
// 	}
// 	return key
// }

// func envOrDefault(key, fallback string) string {
// 	if v := os.Getenv(key); v != "" {
// 		return v
// 	}
// 	return fallback
// }

// // ── sample model builders ────────────────────────────────────

// func sampleUser() *models.User {
// 	first := "Maluks"
// 	last := "test"

// 	return &models.User{
// 		ID:        "user-test-001",
// 		FirstName: &first,
// 		LastName:  &last,
// 		Email:     testRecipient,
// 	}
// }

// func sampleEvent() *models.Event {
// 	reason := "Venue unavailable due to unforeseen circumstances"
// 	return &models.Event{
// 		ID:                 "event-test-001",
// 		Title:              "Go & Payments — Engineering Night",
// 		StartTime:          time.Now().Add(7 * 24 * time.Hour),
// 		EndTime:            time.Now().Add(7*24*time.Hour + 3*time.Hour),
// 		VenueName:          "The Workshop JHB",
// 		Address:            "1 Jan Smuts Ave",
// 		City:               "Johannesburg",
// 		Currency:           "ZAR",
// 		Price:              250.00,
// 		CancellationReason: &reason,
// 	}
// }

// func sampleBooking() *models.Booking {
// 	qrURL := "https://api.qrserver.com/v1/create-qr-code/?size=256x256&data=BK-20260305-001"
// 	qrData := "booking-test-001:BK-20260305-001"
// 	var refunded float64 = 250.00
// 	return &models.Booking{
// 		ID:             "booking-test-001",
// 		BookingNumber:  "BK-20260305-001",
// 		TicketQuantity: 2,
// 		UnitPrice:      250.00,
// 		TotalAmount:    500.00,
// 		Currency:       "ZAR",
// 		QRCodeURL:      &qrURL,
// 		QRCodeData:     &qrData,
// 		RefundedAmount: &refunded,
// 		CreatedAt:      time.Now().Add(-48 * time.Hour),
// 	}
// }

// func sampleOrganizer() *models.Organizer {
// 	return &models.Organizer{
// 		ID:           "org-test-001",
// 		BusinessName: "Maluk Events Co.",
// 	}
// }

// // ============================================================
// // EMAIL TESTS — SendBookingConfirmation
// // ============================================================

// func TestEmailSendBookingConfirmation(t *testing.T) {
// 	cfg := emailTestConfig(t)
// 	svc := NewEmailService(cfg)

// 	err := svc.SendBookingConfirmation(sampleBooking(), sampleEvent(), sampleUser())

// 	require.NoError(t, err, "SendBookingConfirmation should not return an error")
// 	t.Logf("✅ Booking confirmation sent to %s", testRecipient)
// }

// // ============================================================
// // EMAIL TESTS — SendRefundConfirmation
// // ============================================================

// func TestEmailSendRefundConfirmation(t *testing.T) {
// 	cfg := emailTestConfig(t)
// 	svc := NewEmailService(cfg)

// 	err := svc.SendRefundConfirmation(sampleBooking(), sampleEvent(), sampleUser())

// 	require.NoError(t, err)
// 	t.Logf("✅ Refund confirmation sent to %s", testRecipient)
// }

// // ============================================================
// // EMAIL TESTS — SendEventCancellation
// // ============================================================

// func TestEmailSendEventCancellation(t *testing.T) {
// 	cfg := emailTestConfig(t)
// 	svc := NewEmailService(cfg)

// 	err := svc.SendEventCancellation(sampleEvent(), sampleUser(), sampleBooking())

// 	require.NoError(t, err)
// 	t.Logf("✅ Event cancellation sent to %s", testRecipient)
// }

// // ============================================================
// // EMAIL TESTS — SendWaitlistNotification
// // ============================================================

// func TestEmailSendWaitlistNotification(t *testing.T) {
// 	cfg := emailTestConfig(t)
// 	svc := NewEmailService(cfg)

// 	err := svc.SendWaitlistNotification(sampleEvent(), sampleUser())

// 	require.NoError(t, err)
// 	t.Logf("✅ Waitlist notification sent to %s", testRecipient)
// }

// // ============================================================
// // EMAIL TESTS — SendPasswordReset
// // ============================================================

// func TestEmailSendPasswordReset(t *testing.T) {
// 	cfg := emailTestConfig(t)
// 	svc := NewEmailService(cfg)

// 	err := svc.SendPasswordReset(testRecipient, "test-reset-token-abc123")

// 	require.NoError(t, err)
// 	t.Logf("✅ Password reset sent to %s", testRecipient)
// }

// // ============================================================
// // EMAIL TESTS — SendOrganizerApproval
// // ============================================================

// func TestEmailSendOrganizerApproval(t *testing.T) {
// 	cfg := emailTestConfig(t)
// 	svc := NewEmailService(cfg)

// 	err := svc.SendOrganizerApproval(sampleUser(), sampleOrganizer())

// 	require.NoError(t, err)
// 	t.Logf("✅ Organizer approval sent to %s", testRecipient)
// }

// // ============================================================
// // EMAIL TESTS — SendOrganizerRejection
// // ============================================================

// func TestEmailSendOrganizerRejection(t *testing.T) {
// 	cfg := emailTestConfig(t)
// 	svc := NewEmailService(cfg)

// 	err := svc.SendOrganizerRejection(sampleUser(), "Application incomplete — missing business registration number")

// 	require.NoError(t, err)
// 	t.Logf("✅ Organizer rejection sent to %s", testRecipient)
// }

// // ============================================================
// // EMAIL TESTS — full suite in one shot (convenient smoke test)
// // ============================================================

// func TestEmailSendAll(t *testing.T) {
// 	cfg := emailTestConfig(t)
// 	svc := NewEmailService(cfg)

// 	cases := []struct {
// 		name string
// 		fn   func() error
// 	}{
// 		{"BookingConfirmation", func() error {
// 			return svc.SendBookingConfirmation(sampleBooking(), sampleEvent(), sampleUser())
// 		}},
// 		{"RefundConfirmation", func() error {
// 			return svc.SendRefundConfirmation(sampleBooking(), sampleEvent(), sampleUser())
// 		}},
// 		{"EventCancellation", func() error {
// 			return svc.SendEventCancellation(sampleEvent(), sampleUser(), sampleBooking())
// 		}},
// 		{"WaitlistNotification", func() error {
// 			return svc.SendWaitlistNotification(sampleEvent(), sampleUser())
// 		}},
// 		{"PasswordReset", func() error {
// 			return svc.SendPasswordReset(testRecipient, "smoke-test-token-xyz")
// 		}},
// 		{"OrganizerApproval", func() error {
// 			return svc.SendOrganizerApproval(sampleUser(), sampleOrganizer())
// 		}},
// 		{"OrganizerRejection", func() error {
// 			return svc.SendOrganizerRejection(sampleUser(), "Missing documentation")
// 		}},
// 	}

// 	for _, tc := range cases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			err := tc.fn()
// 			require.NoError(t, err)
// 			t.Logf("✅ %s → %s", tc.name, testRecipient)
// 			// Small pause so SendGrid doesn't rate-limit the smoke test.
// 			time.Sleep(300 * time.Millisecond)
// 		})
// 	}
// }

// // ============================================================
// // EMAIL TESTS — renderBookingConfirmation (unit, no network)
// // ============================================================

// func TestEmailRenderBookingConfirmation(t *testing.T) {
// 	cfg := &config.Config{
// 		FrontendURL:      "http://localhost:3000",
// 		QREncryptionKey:  qrKeyOrSkip(t),
// 	}
// 	svc := NewEmailService(cfg)

// 	html, err := svc.renderBookingConfirmation(sampleBooking(), sampleEvent(), sampleUser())

// 	require.NoError(t, err)
// 	assert.Contains(t, html, "BK-20260305-001", "booking number should appear in rendered HTML")
// 	assert.Contains(t, html, "Go &amp; Payments", "event title should be HTML-escaped and present")
// 	assert.Contains(t, html, "500.00", "total amount should appear")
// 	assert.Contains(t, html, "256x256", "QR code URL should be embedded")
// 	assert.True(t, strings.HasPrefix(strings.TrimSpace(html), "<!DOCTYPE"), "output should be valid HTML")
// }

// // ============================================================
// // QR TESTS — encrypt / decrypt round-trip (unit, no DB)
// // ============================================================

// func TestQREncryptDecryptRoundTrip(t *testing.T) {
// 	svc := NewQRService(nil, qrTestConfig(t))

// 	original := QRPayload{
// 		BookingID:     "booking-abc",
// 		BookingNumber: "BK-ROUND-001",
// 		EventID:       "event-xyz",
// 		UserID:        "user-123",
// 		Quantity:      3,
// 		IssuedAt:      time.Now().UTC().Truncate(time.Second),
// 	}

// 	encrypted, err := svc.encryptPayload(original)
// 	require.NoError(t, err)
// 	assert.NotEmpty(t, encrypted)
// 	assert.NotContains(t, encrypted, original.BookingID, "payload must not be plaintext")

// 	decrypted, err := svc.decryptPayload(encrypted)
// 	require.NoError(t, err)
// 	assert.Equal(t, original.BookingID, decrypted.BookingID)
// 	assert.Equal(t, original.BookingNumber, decrypted.BookingNumber)
// 	assert.Equal(t, original.EventID, decrypted.EventID)
// 	assert.Equal(t, original.UserID, decrypted.UserID)
// 	assert.Equal(t, original.Quantity, decrypted.Quantity)
// 	assert.Equal(t, original.IssuedAt.Unix(), decrypted.IssuedAt.Unix())
// }

// func TestQREncryptProducesUniqueOutputs(t *testing.T) {
// 	svc := NewQRService(nil, qrTestConfig(t))

// 	payload := QRPayload{BookingID: "same-booking", BookingNumber: "BK-001", Quantity: 1, IssuedAt: time.Now()}

// 	enc1, err := svc.encryptPayload(payload)
// 	require.NoError(t, err)

// 	enc2, err := svc.encryptPayload(payload)
// 	require.NoError(t, err)

// 	assert.NotEqual(t, enc1, enc2, "each encryption should produce a unique ciphertext (random nonce)")
// }

// func TestQRDecryptRejectsInvalidData(t *testing.T) {
// 	svc := NewQRService(nil, qrTestConfig(t))

// 	cases := []struct {
// 		name  string
// 		input string
// 	}{
// 		{"garbage string",     "not-base64!!!"},
// 		{"valid base64 garbage", "aGVsbG8gd29ybGQ="},   // "hello world" — too short / invalid tag
// 		{"empty string",       ""},
// 	}

// 	for _, tc := range cases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			_, err := svc.decryptPayload(tc.input)
// 			assert.Error(t, err, "invalid ciphertext must return an error")
// 		})
// 	}
// }

// func TestQRDecryptRejectsTamperedData(t *testing.T) {
// 	svc := NewQRService(nil, qrTestConfig(t))

// 	encrypted, err := svc.encryptPayload(QRPayload{
// 		BookingID: "tamper-test",
// 		IssuedAt:  time.Now(),
// 	})
// 	require.NoError(t, err)

// 	// Flip the last character to simulate tampering.
// 	tampered := encrypted[:len(encrypted)-1] + "X"
// 	if tampered == encrypted {
// 		tampered = encrypted[:len(encrypted)-1] + "Y"
// 	}

// 	_, err = svc.decryptPayload(tampered)
// 	assert.Error(t, err, "tampered ciphertext must fail authentication")
// }

// // ============================================================
// // QR TESTS — DB-backed: GenerateQRCode & VerifyQRCode
// // (requires TEST_DB_URL and a running postgres container)
// // ============================================================

// func setupQRTestDB(t *testing.T) *pgxpool.Pool {
// 	t.Helper()
// 	dbURL := os.Getenv("TEST_DB_URL")
// 	if dbURL == "" {
// 		dbURL = "postgresql://testuser:testpass@localhost:5433/test_db"
// 	}

// 	pool, err := pgxpool.New(context.Background(), dbURL)
// 	if err != nil {
// 		t.Skipf("QR DB tests skipped — could not connect to test DB: %v", err)
// 	}

// 	createQRTestTables(t, pool)
// 	cleanupQRTestData(t, pool)
// 	return pool
// }

// func createQRTestTables(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	stmts := []string{
// 		`CREATE TABLE IF NOT EXISTS events (
// 			id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			title               VARCHAR(255) NOT NULL DEFAULT 'Test Event',
// 			start_time          TIMESTAMPTZ NOT NULL,
// 			end_time            TIMESTAMPTZ NOT NULL,
// 			venue_name          VARCHAR(255) DEFAULT '',
// 			address             TEXT DEFAULT '',
// 			city                VARCHAR(100) DEFAULT '',
// 			currency            VARCHAR(10) DEFAULT 'ZAR',
// 			price               NUMERIC(10,2) DEFAULT 0,
// 			cancellation_reason TEXT
// 		)`,
// 		`CREATE TABLE IF NOT EXISTS bookings (
// 			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
// 			booking_number   VARCHAR(50) UNIQUE NOT NULL,
// 			event_id         UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
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
// 			qr_code_data     TEXT,
// 			qr_code_url      TEXT,
// 			tickets_sent     BOOLEAN DEFAULT FALSE,
// 			confirmed_at     TIMESTAMPTZ,
// 			cancelled_at     TIMESTAMPTZ,
// 			updated_at       TIMESTAMPTZ DEFAULT NOW(),
// 			created_at       TIMESTAMPTZ DEFAULT NOW()
// 		)`,
// 	}
// 	for _, s := range stmts {
// 		if _, err := pool.Exec(context.Background(), s); err != nil {
// 			t.Logf("Warning creating QR test table: %v", err)
// 		}
// 	}
// }

// func cleanupQRTestData(t *testing.T, pool *pgxpool.Pool) {
// 	t.Helper()
// 	pool.Exec(context.Background(), "TRUNCATE TABLE bookings CASCADE")
// 	pool.Exec(context.Background(), "TRUNCATE TABLE events CASCADE")
// }

// func insertQRTestEvent(t *testing.T, pool *pgxpool.Pool, start, end time.Time) string {
// 	t.Helper()
// 	var id string
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO events (title, start_time, end_time)
// 		VALUES ('QR Test Event', $1, $2)
// 		RETURNING id
// 	`, start, end).Scan(&id)
// 	require.NoError(t, err)
// 	return id
// }

// func insertQRTestBooking(t *testing.T, pool *pgxpool.Pool, eventID string) string {
// 	t.Helper()
// 	bn := fmt.Sprintf("BK-QR-%d", time.Now().UnixNano())
// 	var id string
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO bookings (booking_number, event_id, ticket_quantity, total_amount, booking_status)
// 		VALUES ($1, $2, 2, 100.00, 'confirmed')
// 		RETURNING id
// 	`, bn, eventID).Scan(&id)
// 	require.NoError(t, err)
// 	return id
// }

// func TestQRGenerateCode(t *testing.T) {
// 	pool := setupQRTestDB(t)
// 	defer cleanupQRTestData(t, pool)

// 	cfg, err := config.Load()
// 	if err != nil {
// 		t.Fatalf("failed to load config: %v", err)
// 	}

// 	// QR upload to R2 requires real credentials; skip if not set.
// 	if cfg.R2AccessKey == "" {
// 		t.Skip("R2_ACCESS_KEY not set — skipping QR upload test")
// 	}

// 	cfg.QREncryptionKey = qrKeyOrSkip(t)
// 	svc := NewQRService(pool, cfg)

// 	eventID   := insertQRTestEvent(t, pool, time.Now().Add(time.Hour), time.Now().Add(4*time.Hour))
// 	bookingID := insertQRTestBooking(t, pool, eventID)

// 	err = svc.GenerateQRCode(bookingID)

// 	require.NoError(t, err)

// 	// Verify DB was updated.
// 	var qrData, qrURL string
// 	var ticketsSent bool
// 	err = pool.QueryRow(context.Background(),
// 		"SELECT qr_code_data, qr_code_url, tickets_sent FROM bookings WHERE id = $1", bookingID,
// 	).Scan(&qrData, &qrURL, &ticketsSent)
// 	require.NoError(t, err)
// 	assert.NotEmpty(t, qrData, "qr_code_data should be set")
// 	assert.NotEmpty(t, qrURL, "qr_code_url should be set")
// 	assert.True(t, ticketsSent)
// }

// func TestQRVerifyCode(t *testing.T) {
// 	pool := setupQRTestDB(t)
// 	defer cleanupQRTestData(t, pool)

// 	cfg := &config.Config{QREncryptionKey: qrKeyOrSkip(t)}
// 	svc := NewQRService(pool, cfg)

// 	eventID   := insertQRTestEvent(t, pool, time.Now().Add(-1*time.Hour), time.Now().Add(3*time.Hour))
// 	bookingID := insertQRTestBooking(t, pool, eventID)

// 	// Encrypt a payload manually so we can test Verify without a real QR image.
// 	payload := QRPayload{
// 		BookingID:     bookingID,
// 		BookingNumber: "BK-QR-VERIFY",
// 		EventID:       eventID,
// 		UserID:        "user-verify-001",
// 		Quantity:      2,
// 		IssuedAt:      time.Now(),
// 	}
// 	encrypted, err := svc.encryptPayload(payload)
// 	require.NoError(t, err)

// 	result, err := svc.VerifyQRCode(encrypted)

// 	require.NoError(t, err)
// 	assert.Equal(t, bookingID, result.BookingID)
// 	assert.Equal(t, payload.BookingNumber, result.BookingNumber)
// }

// func TestQRVerifyRejectsOutsideEventWindow(t *testing.T) {
// 	pool := setupQRTestDB(t)
// 	defer cleanupQRTestData(t, pool)

// 	cfg := &config.Config{QREncryptionKey: qrKeyOrSkip(t)}
// 	svc := NewQRService(pool, cfg)

// 	// Event that ended 5 hours ago — well outside the 2-hour grace window.
// 	eventID   := insertQRTestEvent(t, pool, time.Now().Add(-8*time.Hour), time.Now().Add(-5*time.Hour))
// 	bookingID := insertQRTestBooking(t, pool, eventID)

// 	payload := QRPayload{
// 		BookingID: bookingID,
// 		EventID:   eventID,
// 		IssuedAt:  time.Now(),
// 	}
// 	encrypted, err := svc.encryptPayload(payload)
// 	require.NoError(t, err)

// 	_, err = svc.VerifyQRCode(encrypted)
// 	assert.Error(t, err, "QR code should be rejected outside the event time window")
// }

// func TestQRVerifyRejectsUnconfirmedBooking(t *testing.T) {
// 	pool := setupQRTestDB(t)
// 	defer cleanupQRTestData(t, pool)

// 	cfg := &config.Config{QREncryptionKey: qrKeyOrSkip(t)}
// 	svc := NewQRService(pool, cfg)

// 	eventID := insertQRTestEvent(t, pool, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))

// 	// Insert a PENDING (not confirmed) booking.
// 	bn := fmt.Sprintf("BK-QR-PEND-%d", time.Now().UnixNano())
// 	var bookingID string
// 	err := pool.QueryRow(context.Background(), `
// 		INSERT INTO bookings (booking_number, event_id, ticket_quantity, total_amount, booking_status)
// 		VALUES ($1, $2, 1, 50.00, 'pending')
// 		RETURNING id
// 	`, bn, eventID).Scan(&bookingID)
// 	require.NoError(t, err)

// 	payload := QRPayload{BookingID: bookingID, EventID: eventID, IssuedAt: time.Now()}
// 	encrypted, err := svc.encryptPayload(payload)
// 	require.NoError(t, err)

// 	_, err = svc.VerifyQRCode(encrypted)
// 	assert.Error(t, err, "unconfirmed booking should be rejected")
// 	assert.Contains(t, err.Error(), "not confirmed")
// }