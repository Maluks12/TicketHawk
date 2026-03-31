package services

// =============================================================================
// EMAIL & QR SERVICE TESTS
// =============================================================================
//
// EMAIL TESTS — deliver real mail via SendGrid to the test recipient.
//   Requires: SENDGRID_API_KEY, EMAIL_FROM_ADDRESS, FRONTEND_URL env vars.
//   Tests skip automatically when those vars are absent.
//
// QR TESTS (unit) — encrypt/decrypt without a DB; no env vars required.
//
// QR TESTS (DB-backed) — require a running Postgres container:
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db -p 5433:5432 postgres:15
//   Requires: R2_ACCESS_KEY env var to exercise the upload path.
//
// Run all:                     go test ./internal/services/... -v -run "TestEmail|TestQR"
// Run only live email sends:   go test ./internal/services/... -v -run TestEmailSend
// Run only unit QR tests:      go test ./internal/services/... -v -run TestQRUnit
// =============================================================================

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"events-db/internal/config"
	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Config helpers ───────────────────────────────────────────────────────────

const testRecipient = "ribnoma@gmail.com"

func loadRootEnv() {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			_ = godotenv.Load(filepath.Join(dir, ".env"))
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
}

func emailTestConfig(t *testing.T) *config.Config {
	t.Helper()
	loadRootEnv()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.SendGridAPIKey == "" {
		t.Skip("SENDGRID_API_KEY not set — skipping live email tests")
	}
	if cfg.EmailFromAddress == "" {
		t.Skip("EMAIL_FROM_ADDRESS not set — skipping live email tests")
	}
	if cfg.EmailFromName == "" {
		cfg.EmailFromName = "Events Platform Test"
	}
	if cfg.FrontendURL == "" {
		cfg.FrontendURL = "http://localhost:3000"
	}
	cfg.QREncryptionKey = qrKeyOrSkip(t)
	return cfg
}

func qrTestConfig(t *testing.T) *config.Config {
	t.Helper()
	loadRootEnv()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	cfg.QREncryptionKey = qrKeyOrSkip(t)
	return cfg
}

func qrKeyOrSkip(t *testing.T) string {
	t.Helper()
	key := os.Getenv("QR_ENCRYPTION_KEY")
	if key == "" {
		return "testkeyfortesting1234567890abcde" // deterministic 32-byte fallback
	}
	if len(key) != 32 {
		t.Fatalf("QR_ENCRYPTION_KEY must be exactly 32 bytes, got %d", len(key))
	}
	return key
}

// ─── Model fixtures ───────────────────────────────────────────────────────────

func sampleUser() *models.User {
	first, last := "Sipho", "Dlamini"
	return &models.User{
		ID:        "user-test-001",
		FirstName: &first,
		LastName:  &last,
		Email:     testRecipient,
	}
}

func sampleEvent() *models.Event {
	reason := "Venue unavailable due to unforeseen circumstances"
	return &models.Event{
		ID:                 "event-test-001",
		Title:              "Afrobeats Summer Festival 2025",
		StartTime:          time.Now().Add(7 * 24 * time.Hour),
		EndTime:            time.Now().Add(7*24*time.Hour + 4*time.Hour),
		VenueName:          "Sandton Convention Centre",
		Address:            "161 Maude St, Sandown",
		City:               "Johannesburg",
		Currency:           "ZAR",
		Price:              340.00,
		CancellationReason: &reason,
	}
}

func sampleBooking() *models.Booking {
	// QR URL is a placeholder — in production this is an R2 public CDN URL.
	// See explanation at the bottom of this file for why emails need a public URL.
	qrURL := "https://pub-your-account.r2.dev/qr-codes/booking-test-001.png"
	qrData := "booking-test-001:BK-20250712-001"
	var refunded float64 = 680.00
	return &models.Booking{
		ID:             "booking-test-001",
		BookingNumber:  "BK-20250712-001",
		TicketQuantity: 2,
		UnitPrice:      340.00,
		TotalAmount:    680.00,
		Currency:       "ZAR",
		QRCodeURL:      &qrURL,
		QRCodeData:     &qrData,
		RefundedAmount: &refunded,
		CreatedAt:      time.Now().Add(-48 * time.Hour),
	}
}

func sampleOrganizer() *models.Organizer {
	return &models.Organizer{
		ID:           "org-test-001",
		BusinessName: "Maluk Events Co.",
	}
}

// ─── EMAIL TESTS ──────────────────────────────────────────────────────────────

func TestEmailSendBookingConfirmation(t *testing.T) {
	svc := NewEmailService(emailTestConfig(t))
	require.NoError(t, svc.SendBookingConfirmation(sampleBooking(), sampleEvent(), sampleUser()))
	t.Logf("✅ Booking confirmation sent to %s", testRecipient)
}

func TestEmailSendBookingConfirmationWithQR(t *testing.T) {
	cfg := emailTestConfig(t)
	svc := NewEmailService(cfg)

	// This is the method the booking worker now calls.
	// The QR URL must be a publicly accessible URL — see note below.
	qrURL := "https://pub-your-account.r2.dev/qr-codes/booking-test-001.png"
	err := svc.SendBookingConfirmationWithQR(sampleBooking(), sampleEvent(), sampleUser(), qrURL)
	require.NoError(t, err)
	t.Logf("✅ Booking confirmation with QR sent to %s", testRecipient)
}

func TestEmailSendBookingConfirmationMultiTicket(t *testing.T) {
	cfg := emailTestConfig(t)
	svc := NewEmailService(cfg)

	// Multi-ticket booking — should include "assign your tickets" note in body.
	booking := sampleBooking()
	booking.TicketQuantity = 4
	booking.TotalAmount = 1360.00

	qrURL := "https://pub-your-account.r2.dev/qr-codes/booking-test-001.png"
	err := svc.SendBookingConfirmationWithQR(booking, sampleEvent(), sampleUser(), qrURL)
	require.NoError(t, err)
	t.Logf("✅ Multi-ticket confirmation (4 tickets) sent to %s", testRecipient)
}

func TestEmailSendRefundConfirmation(t *testing.T) {
	svc := NewEmailService(emailTestConfig(t))
	require.NoError(t, svc.SendRefundConfirmation(sampleBooking(), sampleEvent(), sampleUser()))
	t.Logf("✅ Refund confirmation sent to %s", testRecipient)
}

func TestEmailSendEventCancellation(t *testing.T) {
	svc := NewEmailService(emailTestConfig(t))
	require.NoError(t, svc.SendEventCancellation(sampleEvent(), sampleUser(), sampleBooking()))
	t.Logf("✅ Event cancellation sent to %s", testRecipient)
}

func TestEmailSendWaitlistNotification(t *testing.T) {
	svc := NewEmailService(emailTestConfig(t))
	require.NoError(t, svc.SendWaitlistNotification(sampleEvent(), sampleUser()))
	t.Logf("✅ Waitlist notification sent to %s", testRecipient)
}

func TestEmailSendPasswordReset(t *testing.T) {
	svc := NewEmailService(emailTestConfig(t))
	require.NoError(t, svc.SendPasswordReset(testRecipient, "test-reset-token-abc123"))
	t.Logf("✅ Password reset sent to %s", testRecipient)
}

func TestEmailSendOrganizerApproval(t *testing.T) {
	svc := NewEmailService(emailTestConfig(t))
	require.NoError(t, svc.SendOrganizerApproval(sampleUser(), sampleOrganizer()))
	t.Logf("✅ Organizer approval sent to %s", testRecipient)
}

func TestEmailSendOrganizerRejection(t *testing.T) {
	svc := NewEmailService(emailTestConfig(t))
	require.NoError(t, svc.SendOrganizerRejection(sampleUser(), "Application incomplete — missing business registration number"))
	t.Logf("✅ Organizer rejection sent to %s", testRecipient)
}

func TestEmailSendTicketAssignmentInvite(t *testing.T) {
	cfg := emailTestConfig(t)
	svc := NewEmailService(cfg)

	claimURL := cfg.FrontendURL + "/claim-ticket?token=550e8400-e29b-41d4-a716-446655440000"
	expiresAt := time.Now().Add(24 * time.Hour)

	err := svc.SendTicketAssignmentInvite(
		testRecipient,
		"Sipho Dlamini",
		"Thabo Nkosi",             // buyer name
		"Afrobeats Summer Festival 2025",
		claimURL,
		expiresAt,
	)
	require.NoError(t, err)
	t.Logf("✅ Ticket assignment invite sent to %s", testRecipient)
}

func TestEmailSendTicketClaimConfirmation(t *testing.T) {
	cfg := emailTestConfig(t)
	svc := NewEmailService(cfg)

	qrURL := "https://pub-your-account.r2.dev/qr-codes/assignment-test-001.png"

	err := svc.SendTicketClaimConfirmation(
		testRecipient,
		"Sipho Dlamini",
		"Afrobeats Summer Festival 2025",
		"Saturday, 12 July 2025 at 16:00",
		"Sandton Convention Centre",
		"BK-20250712-001",
		2, // slot number
		qrURL,
	)
	require.NoError(t, err)
	t.Logf("✅ Ticket claim confirmation sent to %s", testRecipient)
}

func TestEmailSendTicketInvalidationNotice(t *testing.T) {
	cfg := emailTestConfig(t)
	svc := NewEmailService(cfg)

	err := svc.SendTicketInvalidationNotice(
		testRecipient,
		"Sipho Dlamini",
		"Afrobeats Summer Festival 2025",
		"Saturday, 12 July 2025",
		"BK-20250712-001",
	)
	require.NoError(t, err)
	t.Logf("✅ Ticket invalidation notice sent to %s", testRecipient)
}

// TestEmailSendAll fires every template as a smoke test.
func TestEmailSendAll(t *testing.T) {
	cfg := emailTestConfig(t)
	svc := NewEmailService(cfg)
	qrURL := "https://pub-your-account.r2.dev/qr-codes/test.png"
	claimURL := cfg.FrontendURL + "/claim-ticket?token=550e8400-e29b-41d4-a716-446655440000"

	cases := []struct {
		name string
		fn   func() error
	}{
		{"BookingConfirmation", func() error {
			return svc.SendBookingConfirmation(sampleBooking(), sampleEvent(), sampleUser())
		}},
		{"BookingConfirmationWithQR", func() error {
			return svc.SendBookingConfirmationWithQR(sampleBooking(), sampleEvent(), sampleUser(), qrURL)
		}},
		{"RefundConfirmation", func() error {
			return svc.SendRefundConfirmation(sampleBooking(), sampleEvent(), sampleUser())
		}},
		{"EventCancellation", func() error {
			return svc.SendEventCancellation(sampleEvent(), sampleUser(), sampleBooking())
		}},
		{"WaitlistNotification", func() error {
			return svc.SendWaitlistNotification(sampleEvent(), sampleUser())
		}},
		{"PasswordReset", func() error {
			return svc.SendPasswordReset(testRecipient, "smoke-test-token-xyz")
		}},
		{"OrganizerApproval", func() error {
			return svc.SendOrganizerApproval(sampleUser(), sampleOrganizer())
		}},
		{"OrganizerRejection", func() error {
			return svc.SendOrganizerRejection(sampleUser(), "Missing documentation")
		}},
		{"TicketAssignmentInvite", func() error {
			return svc.SendTicketAssignmentInvite(testRecipient, "Sipho Dlamini", "Thabo Nkosi",
				"Afrobeats Summer Festival 2025", claimURL, time.Now().Add(24*time.Hour))
		}},
		{"TicketClaimConfirmation", func() error {
			return svc.SendTicketClaimConfirmation(testRecipient, "Sipho Dlamini",
				"Afrobeats Summer Festival 2025", "Saturday, 12 July 2025 at 16:00",
				"Sandton Convention Centre", "BK-20250712-001", 2, qrURL)
		}},
		{"TicketInvalidationNotice", func() error {
			return svc.SendTicketInvalidationNotice(testRecipient, "Sipho Dlamini",
				"Afrobeats Summer Festival 2025", "Saturday, 12 July 2025", "BK-20250712-001")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, tc.fn())
			t.Logf("✅ %s → %s", tc.name, testRecipient)
			time.Sleep(300 * time.Millisecond) // avoid SendGrid rate limits
		})
	}
}

// ─── EMAIL RENDER UNIT TESTS (no network) ────────────────────────────────────

func TestEmailRenderBookingConfirmationWithQR(t *testing.T) {
	svc := NewEmailService(&config.Config{
		FrontendURL:     "http://localhost:3000",
		QREncryptionKey: qrKeyOrSkip(t),
	})

	qrURL := "https://pub-your-account.r2.dev/qr-codes/test.png"
	html, err := svc.renderBookingConfirmationWithQR(sampleBooking(), sampleEvent(), sampleUser(), qrURL)

	require.NoError(t, err)
	assert.Contains(t, html, "BK-20250712-001", "booking number must be in the rendered HTML")
	assert.Contains(t, html, "Afrobeats", "event title must appear")
	assert.Contains(t, html, "680.00", "total amount must appear")
	assert.Contains(t, html, qrURL, "QR URL must be embedded in the img tag")
	assert.True(t, strings.HasPrefix(strings.TrimSpace(html), "<!DOCTYPE"), "output must be valid HTML")
}

func TestEmailRenderBookingConfirmationMultiTicketNote(t *testing.T) {
	svc := NewEmailService(&config.Config{
		FrontendURL:     "http://localhost:3000",
		QREncryptionKey: qrKeyOrSkip(t),
	})

	booking := sampleBooking()
	booking.TicketQuantity = 4

	html, err := svc.renderBookingConfirmationWithQR(booking, sampleEvent(), sampleUser(), "")
	require.NoError(t, err)
	assert.Contains(t, html, "4 tickets", "multi-ticket note must mention quantity")
	assert.Contains(t, html, "manage tickets", "link to assignment page must appear")
}

func TestEmailRenderBookingConfirmationSingleTicketNoNote(t *testing.T) {
	svc := NewEmailService(&config.Config{
		FrontendURL:     "http://localhost:3000",
		QREncryptionKey: qrKeyOrSkip(t),
	})

	booking := sampleBooking()
	booking.TicketQuantity = 1

	html, err := svc.renderBookingConfirmationWithQR(booking, sampleEvent(), sampleUser(), "")
	require.NoError(t, err)
	// Single ticket bookings must NOT show the multi-ticket assignment note.
	assert.NotContains(t, html, "manage tickets", "single-ticket booking must not show assignment note")
}

// ─── QR UNIT TESTS (no DB, no network) ───────────────────────────────────────

func TestQRUnitEncryptDecryptRoundTrip(t *testing.T) {
	svc := NewQRService(nil, qrTestConfig(t))

	original := QRPayload{
		BookingID:     "booking-abc",
		BookingNumber: "BK-ROUND-001",
		EventID:       "event-xyz",
		UserID:        "user-123",
		AssignmentID:  "assign-456",
		SlotNumber:    2,
		Quantity:      3,
		IssuedAt:      time.Now().UTC().Truncate(time.Second),
	}

	encrypted, err := svc.encryptPayload(original)
	require.NoError(t, err)
	assert.NotEmpty(t, encrypted)
	assert.NotContains(t, encrypted, original.BookingID, "payload must not be plaintext in ciphertext")

	decrypted, err := svc.decryptPayload(encrypted)
	require.NoError(t, err)
	assert.Equal(t, original.BookingID, decrypted.BookingID)
	assert.Equal(t, original.BookingNumber, decrypted.BookingNumber)
	assert.Equal(t, original.EventID, decrypted.EventID)
	assert.Equal(t, original.UserID, decrypted.UserID)
	assert.Equal(t, original.AssignmentID, decrypted.AssignmentID)
	assert.Equal(t, original.SlotNumber, decrypted.SlotNumber)
	assert.Equal(t, original.Quantity, decrypted.Quantity)
	assert.Equal(t, original.IssuedAt.Unix(), decrypted.IssuedAt.Unix())
}

func TestQRUnitEncryptProducesUniqueOutputs(t *testing.T) {
	svc := NewQRService(nil, qrTestConfig(t))
	payload := QRPayload{
		BookingID: "same-booking", BookingNumber: "BK-001",
		AssignmentID: "assign-001", SlotNumber: 1, Quantity: 1,
		IssuedAt: time.Now(),
	}

	enc1, err := svc.encryptPayload(payload)
	require.NoError(t, err)
	enc2, err := svc.encryptPayload(payload)
	require.NoError(t, err)

	// AES-GCM uses a random nonce so same plaintext yields different ciphertext.
	assert.NotEqual(t, enc1, enc2, "each encryption must produce a unique ciphertext (random nonce)")
}

func TestQRUnitDecryptRejectsInvalidData(t *testing.T) {
	svc := NewQRService(nil, qrTestConfig(t))

	cases := []struct {
		name  string
		input string
	}{
		{"garbage string", "not-base64!!!"},
		{"valid base64 but garbage bytes", "aGVsbG8gd29ybGQ="}, // "hello world" — too short for GCM tag
		{"empty string", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.decryptPayload(tc.input)
			assert.Error(t, err, "invalid ciphertext must return an error")
		})
	}
}

func TestQRUnitDecryptRejectsTamperedData(t *testing.T) {
	svc := NewQRService(nil, qrTestConfig(t))

	encrypted, err := svc.encryptPayload(QRPayload{
		BookingID: "tamper-test", AssignmentID: "assign-tamper",
		SlotNumber: 1, IssuedAt: time.Now(),
	})
	require.NoError(t, err)

	tampered := encrypted[:len(encrypted)-1] + "X"
	if tampered == encrypted {
		tampered = encrypted[:len(encrypted)-1] + "Y"
	}

	_, err = svc.decryptPayload(tampered)
	assert.Error(t, err, "tampered ciphertext must fail GCM authentication")
}

// ─── QR DB TESTS ──────────────────────────────────────────────────────────────

func setupQRTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		dbURL = "postgresql://testuser:testpass@localhost:5433/test_db"
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Skipf("QR DB tests skipped — could not connect: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("QR DB tests skipped — DB not reachable: %v", pingErr)
	}
	createQRTestTables(t, pool)
	cleanupQRTestData(t, pool)
	return pool
}

func createQRTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	stmts := []string{
		`DO $$ BEGIN CREATE TYPE assignment_status AS ENUM
		 ('unassigned','pending','claimed','revoked','cancelled');
		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		`CREATE TABLE IF NOT EXISTS events (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title      VARCHAR(255) NOT NULL DEFAULT 'Test Event',
			start_time TIMESTAMPTZ NOT NULL,
			end_time   TIMESTAMPTZ NOT NULL,
			venue_name VARCHAR(255) DEFAULT '',
			currency   VARCHAR(10) DEFAULT 'ZAR'
		)`,

		`CREATE TABLE IF NOT EXISTS bookings (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_number   VARCHAR(50) UNIQUE NOT NULL,
			event_id         UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			user_id          UUID,
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
			qr_code_data     TEXT,
			qr_code_url      TEXT,
			tickets_sent     BOOLEAN DEFAULT FALSE,
			confirmed_at     TIMESTAMPTZ,
			updated_at       TIMESTAMPTZ DEFAULT NOW(),
			created_at       TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS ticket_assignments (
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
	}
	for _, s := range stmts {
		if _, err := pool.Exec(context.Background(), s); err != nil {
			t.Logf("Warning creating QR test table: %v", err)
		}
	}
}

func cleanupQRTestData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	pool.Exec(context.Background(), "TRUNCATE ticket_assignments CASCADE")
	pool.Exec(context.Background(), "TRUNCATE bookings            CASCADE")
	pool.Exec(context.Background(), "TRUNCATE events              CASCADE")
}

func insertQRTestEvent(t *testing.T, pool *pgxpool.Pool, start, end time.Time) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO events (title, start_time, end_time) VALUES ('QR Test Event', $1, $2) RETURNING id`,
		start, end,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertQRTestBooking(t *testing.T, pool *pgxpool.Pool, eventID string, status string) string {
	t.Helper()
	bn := fmt.Sprintf("BK-QR-%d", time.Now().UnixNano())
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO bookings (booking_number, event_id, ticket_quantity, total_amount, booking_status)
		VALUES ($1, $2, 2, 100.00, $3) RETURNING id
	`, bn, eventID, status).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertQRTestAssignment(t *testing.T, pool *pgxpool.Pool, bookingID string, slot int, status string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO ticket_assignments (booking_id, slot_number, status, claimed_at)
		VALUES ($1, $2, $3, CASE WHEN $3 = 'claimed' THEN NOW() ELSE NULL END)
		RETURNING id
	`, bookingID, slot, status).Scan(&id)
	require.NoError(t, err)
	return id
}

// TestQRGenerateCodeForAssignment verifies that GenerateQRCodeForAssignment
// writes qr_code_data and qr_code_url to the ticket_assignments row.
// Skipped when R2_ACCESS_KEY is absent.
func TestQRGenerateCodeForAssignment(t *testing.T) {
	pool := setupQRTestDB(t)
	defer cleanupQRTestData(t, pool)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.R2AccessKey == "" {
		t.Skip("R2_ACCESS_KEY not set — skipping QR upload test")
	}
	cfg.QREncryptionKey = qrKeyOrSkip(t)
	svc := NewQRService(pool, cfg)

	eventID := insertQRTestEvent(t, pool, time.Now().Add(time.Hour), time.Now().Add(4*time.Hour))
	bookingID := insertQRTestBooking(t, pool, eventID, "confirmed")
	assignmentID := insertQRTestAssignment(t, pool, bookingID, 1, "claimed")

	require.NoError(t, svc.GenerateQRCodeForAssignment(assignmentID))

	var qrData, qrURL string
	err = pool.QueryRow(context.Background(),
		`SELECT qr_code_data, qr_code_url FROM ticket_assignments WHERE id = $1`, assignmentID,
	).Scan(&qrData, &qrURL)
	require.NoError(t, err)
	assert.NotEmpty(t, qrData, "qr_code_data must be written")
	assert.NotEmpty(t, qrURL, "qr_code_url must be written")
	assert.Contains(t, qrURL, assignmentID, "QR URL should reference the assignment ID")
}

// TestQRVerifyAssignmentCode verifies that a freshly-encrypted QR payload
// passes VerifyAssignmentQRCode when the booking is confirmed.
func TestQRVerifyAssignmentCode(t *testing.T) {
	pool := setupQRTestDB(t)
	defer cleanupQRTestData(t, pool)

	svc := NewQRService(pool, &config.Config{QREncryptionKey: qrKeyOrSkip(t)})
	eventID := insertQRTestEvent(t, pool, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertQRTestBooking(t, pool, eventID, "confirmed")
	assignmentID := insertQRTestAssignment(t, pool, bookingID, 1, "claimed")

	payload := QRPayload{
		BookingID:     bookingID,
		BookingNumber: "BK-QR-VERIFY",
		EventID:       eventID,
		UserID:        "user-verify-001",
		AssignmentID:  assignmentID,
		SlotNumber:    1,
		Quantity:      2,
		IssuedAt:      time.Now(),
	}
	encrypted, err := svc.encryptPayload(payload)
	require.NoError(t, err)

	result, err := svc.VerifyAssignmentQRCode(encrypted)
	require.NoError(t, err)
	assert.Equal(t, bookingID, result.BookingID)
	assert.Equal(t, assignmentID, result.AssignmentID)
	assert.Equal(t, 1, result.SlotNumber)
}

// TestQRVerifyAssignmentRejectsRefundedBooking is the core fix test —
// a refunded booking's QR must be rejected even if the assignment row is claimed.
func TestQRVerifyAssignmentRejectsRefundedBooking(t *testing.T) {
	pool := setupQRTestDB(t)
	defer cleanupQRTestData(t, pool)

	svc := NewQRService(pool, &config.Config{QREncryptionKey: qrKeyOrSkip(t)})
	eventID := insertQRTestEvent(t, pool, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))

	// Booking is refunded
	bookingID := insertQRTestBooking(t, pool, eventID, "refunded")
	assignmentID := insertQRTestAssignment(t, pool, bookingID, 1, "claimed")

	encrypted, err := svc.encryptPayload(QRPayload{
		BookingID:    bookingID,
		AssignmentID: assignmentID,
		SlotNumber:   1,
		IssuedAt:     time.Now(),
	})
	require.NoError(t, err)

	_, err = svc.VerifyAssignmentQRCode(encrypted)
	assert.Error(t, err, "refunded booking QR must be rejected")
	assert.Contains(t, err.Error(), "refunded", "error must state the booking was refunded")
}

// TestQRVerifyAssignmentRejectsCancelledBooking covers the event cancellation path.
func TestQRVerifyAssignmentRejectsCancelledBooking(t *testing.T) {
	pool := setupQRTestDB(t)
	defer cleanupQRTestData(t, pool)

	svc := NewQRService(pool, &config.Config{QREncryptionKey: qrKeyOrSkip(t)})
	eventID := insertQRTestEvent(t, pool, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertQRTestBooking(t, pool, eventID, "cancelled")
	assignmentID := insertQRTestAssignment(t, pool, bookingID, 1, "claimed")

	encrypted, err := svc.encryptPayload(QRPayload{
		BookingID:    bookingID,
		AssignmentID: assignmentID,
		SlotNumber:   1,
		IssuedAt:     time.Now(),
	})
	require.NoError(t, err)

	_, err = svc.VerifyAssignmentQRCode(encrypted)
	assert.Error(t, err, "cancelled booking QR must be rejected")
}

// TestQRVerifyAssignmentRejectsUnclaimedSlot ensures a pending/unassigned slot
// cannot be used to enter even if someone intercepts the encrypted QR data.
func TestQRVerifyAssignmentRejectsUnclaimedSlot(t *testing.T) {
	pool := setupQRTestDB(t)
	defer cleanupQRTestData(t, pool)

	svc := NewQRService(pool, &config.Config{QREncryptionKey: qrKeyOrSkip(t)})
	eventID := insertQRTestEvent(t, pool, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertQRTestBooking(t, pool, eventID, "confirmed")
	assignmentID := insertQRTestAssignment(t, pool, bookingID, 2, "unassigned")

	encrypted, err := svc.encryptPayload(QRPayload{
		BookingID:    bookingID,
		AssignmentID: assignmentID,
		SlotNumber:   2,
		IssuedAt:     time.Now(),
	})
	require.NoError(t, err)

	_, err = svc.VerifyAssignmentQRCode(encrypted)
	assert.Error(t, err, "unclaimed slot must be rejected at the door")
}

// TestQRVerifyAssignmentRejectsOutsideEventWindow confirms the time guard still works.
func TestQRVerifyAssignmentRejectsOutsideEventWindow(t *testing.T) {
	pool := setupQRTestDB(t)
	defer cleanupQRTestData(t, pool)

	svc := NewQRService(pool, &config.Config{QREncryptionKey: qrKeyOrSkip(t)})
	// Event ended 5 hours ago — outside the 2-hour grace window.
	eventID := insertQRTestEvent(t, pool, time.Now().Add(-8*time.Hour), time.Now().Add(-5*time.Hour))
	bookingID := insertQRTestBooking(t, pool, eventID, "confirmed")
	assignmentID := insertQRTestAssignment(t, pool, bookingID, 1, "claimed")

	encrypted, err := svc.encryptPayload(QRPayload{
		BookingID:    bookingID,
		AssignmentID: assignmentID,
		SlotNumber:   1,
		IssuedAt:     time.Now(),
	})
	require.NoError(t, err)

	_, err = svc.VerifyAssignmentQRCode(encrypted)
	assert.Error(t, err, "QR code outside time window must be rejected")
}

// TestQRVerifyLegacyBookingLevelRejectsRefunded ensures the old booking-level
// VerifyQRCode (backward compat) also honours the booking status fix.
func TestQRVerifyLegacyBookingLevelRejectsRefunded(t *testing.T) {
	pool := setupQRTestDB(t)
	defer cleanupQRTestData(t, pool)

	svc := NewQRService(pool, &config.Config{QREncryptionKey: qrKeyOrSkip(t)})
	eventID := insertQRTestEvent(t, pool, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertQRTestBooking(t, pool, eventID, "refunded")

	encrypted, err := svc.encryptPayload(QRPayload{
		BookingID: bookingID, EventID: eventID, IssuedAt: time.Now(),
	})
	require.NoError(t, err)

	_, err = svc.VerifyQRCode(encrypted)
	assert.Error(t, err, "legacy verifier must also reject refunded bookings")
	assert.Contains(t, err.Error(), "refunded")
}