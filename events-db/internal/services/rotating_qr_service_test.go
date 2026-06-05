package services

// // =============================================================================
// // ROTATING QR SERVICE TESTS
// // =============================================================================
// //
// // UNIT TESTS (no DB) — test HMAC generation, signature verification, window
// //   expiry, and tamper detection. These run without any infrastructure.
// //
// // INTEGRATION TESTS (DB-backed) — require a running Postgres container:
// //   docker run -d --name test-postgres \
// //     -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
// //     -e POSTGRES_DB=test_db -p 5433:5432 postgres:15
// //
// //   Tests skip automatically when the container is unreachable.
// //   They exercise the full scan flow: token generation → DB status checks →
// //   atomic single-entry mark, matching what ScanRotatingQRHandler does.
// //
// // Run all:        go test ./internal/services/... -v -run TestRotatingQR
// // Run unit only:  go test ./internal/services/... -v -run TestRotatingQRUnit
// // Run DB only:    go test ./internal/services/... -v -run TestRotatingQRDB
// // =============================================================================

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"events-db/internal/config"
	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Shared fixtures ──────────────────────────────────────────────────────────

func newTestRotatingQR() *RotatingQRService {
	return NewRotatingQRService(&config.Config{
		QRHMACSecret: "test-secret-must-be-at-least-32-chars-long-padding",
	})
}

// buildTokenWithWindow constructs a correctly-signed token for any window value.
// Used to test time-boundary logic without waiting for the clock to advance.
func buildTokenWithWindow(svc *RotatingQRService, assignmentID string, window int64) string {
	p := RotatingQRPayload{
		AssignmentID: assignmentID,
		BookingID:    "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		EventID:      "cccccccc-cccc-cccc-cccc-cccccccccccc",
		SlotNumber:   1,
		Window:       window,
		Version:      1,
	}
	data, _ := json.Marshal(p)
	payloadB64 := base64.RawURLEncoding.EncodeToString(data)
	sig := svc.sign(assignmentID, window)
	return payloadB64 + "." + sig
}

// ─── UNIT TESTS (no DB) ───────────────────────────────────────────────────────

func TestRotatingQRUnit_GenerateAndVerify(t *testing.T) {
	svc := newTestRotatingQR()

	tok, err := svc.GenerateToken("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "cccccccc-cccc-cccc-cccc-cccccccccccc", 1)
	require.NoError(t, err)
	assert.NotEmpty(t, tok.Token)
	assert.Equal(t, QRWindowSeconds, tok.WindowSec)
	assert.True(t, time.Until(tok.ExpiresAt) > 0, "ExpiresAt must be in the future")

	payload, err := svc.VerifyToken(tok.Token)
	require.NoError(t, err)
	assert.Equal(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", payload.AssignmentID)
	assert.Equal(t, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", payload.BookingID)
	assert.Equal(t, "cccccccc-cccc-cccc-cccc-cccccccccccc", payload.EventID)
	assert.Equal(t, 1, payload.SlotNumber)
	assert.Equal(t, 1, payload.Version)
}

func TestRotatingQRUnit_TokenFormat(t *testing.T) {
	svc := newTestRotatingQR()
	tok, _ := svc.GenerateToken("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "cccccccc-cccc-cccc-cccc-cccccccccccc", 1)

	parts := strings.Split(tok.Token, ".")
	require.Len(t, parts, 2, "token must have exactly two dot-separated segments")
	for _, p := range parts {
		assert.NotEmpty(t, p, "neither segment may be empty")
	}
}

func TestRotatingQRUnit_TamperedPayloadRejected(t *testing.T) {
	svc := newTestRotatingQR()
	tok, _ := svc.GenerateToken("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "cccccccc-cccc-cccc-cccc-cccccccccccc", 1)

	dot := strings.LastIndex(tok.Token, ".")
	tampered := "dGFtcGVyZWQ" + tok.Token[dot:]

	_, err := svc.VerifyToken(tampered)
	assert.Error(t, err, "tampered payload must be rejected")
}

func TestRotatingQRUnit_TamperedSignatureRejected(t *testing.T) {
	svc := newTestRotatingQR()
	tok, _ := svc.GenerateToken("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "cccccccc-cccc-cccc-cccc-cccccccccccc", 1)

	dot := strings.LastIndex(tok.Token, ".")
	tampered := tok.Token[:dot+1] + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	_, err := svc.VerifyToken(tampered)
	assert.Error(t, err, "tampered signature must be rejected")
}

func TestRotatingQRUnit_WrongSecretRejected(t *testing.T) {
	svc1 := NewRotatingQRService(&config.Config{QRHMACSecret: "secret-one-must-be-at-least-32-chars-padding"})
	svc2 := NewRotatingQRService(&config.Config{QRHMACSecret: "secret-two-must-be-at-least-32-chars-padding"})

	tok, _ := svc1.GenerateToken("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "cccccccc-cccc-cccc-cccc-cccccccccccc", 1)
	_, err := svc2.VerifyToken(tok.Token)
	assert.Error(t, err, "token signed with a different secret must be rejected")
}

func TestRotatingQRUnit_CurrentWindowAccepted(t *testing.T) {
	svc := newTestRotatingQR()
	token := buildTokenWithWindow(svc, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", currentWindow())
	_, err := svc.VerifyToken(token)
	assert.NoError(t, err, "current window must be accepted")
}

func TestRotatingQRUnit_PreviousWindowAccepted(t *testing.T) {
	svc := newTestRotatingQR()
	token := buildTokenWithWindow(svc, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", currentWindow()-1)
	_, err := svc.VerifyToken(token)
	assert.NoError(t, err, "previous window must still be accepted (grace for slow networks)")
}

func TestRotatingQRUnit_ExpiredWindowRejected(t *testing.T) {
	svc := newTestRotatingQR()
	staleWindow := currentWindow() - (qrWindowTolerance + 2)
	token := buildTokenWithWindow(svc, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", staleWindow)
	_, err := svc.VerifyToken(token)
	assert.Error(t, err, "token beyond tolerance window must be rejected")
}

func TestRotatingQRUnit_FutureWindowRejected(t *testing.T) {
	svc := newTestRotatingQR()
	token := buildTokenWithWindow(svc, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", currentWindow()+5)
	_, err := svc.VerifyToken(token)
	assert.Error(t, err, "future window must be rejected — prevents pre-generating tokens")
}

func TestRotatingQRUnit_MalformedInputsRejected(t *testing.T) {
	svc := newTestRotatingQR()

	cases := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"no dot separator", "onlyonepart"},
		{"dot only", "."},
		{"invalid base64 payload", "!!!.AAAA"},
		{"valid b64 but not JSON", base64.RawURLEncoding.EncodeToString([]byte("notjson")) + ".AAAA"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.VerifyToken(tc.token)
			assert.Error(t, err, "malformed token must not succeed")
		})
	}
}

func TestRotatingQRUnit_AssignmentIDBoundToSignature(t *testing.T) {
	svc := newTestRotatingQR()
	w := currentWindow()

	// Build valid token for assignment A, then attach its signature to a payload
	// for assignment B. The HMAC input includes the assignmentID so this must fail.
	tokenA := buildTokenWithWindow(svc, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", w)

	pB := RotatingQRPayload{
		AssignmentID: "dddddddd-dddd-dddd-dddd-dddddddddddd",
		BookingID:    "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		EventID:      "cccccccc-cccc-cccc-cccc-cccccccccccc",
		SlotNumber:   2,
		Window:       w,
		Version:      1,
	}
	dataB, _ := json.Marshal(pB)
	payloadB := base64.RawURLEncoding.EncodeToString(dataB)
	sigA := tokenA[strings.LastIndex(tokenA, ".")+1:]
	crossToken := payloadB + "." + sigA

	_, err := svc.VerifyToken(crossToken)
	assert.Error(t, err, "signature from a different assignment must not verify")
}

// ─── DB SETUP ─────────────────────────────────────────────────────────────────

func setupRotatingQRTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgresql://testuser:testpass@localhost:5433/test_db")
	if err != nil {
		t.Skipf("rotating QR DB tests skipped — could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("rotating QR DB tests skipped — DB not reachable (start with: docker run -d --name test-postgres -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass -e POSTGRES_DB=test_db -p 5433:5432 postgres:15): %v", pingErr)
	}
	createRotatingQRTestTables(t, pool)
	cleanupRotatingQRTestData(pool)
	return pool
}

func createRotatingQRTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
		"DROP TABLE IF EXISTS ticket_assignments CASCADE",
		"DROP TABLE IF EXISTS bookings           CASCADE",
		"DROP TABLE IF EXISTS events             CASCADE",
		"DROP TABLE IF EXISTS organizers         CASCADE",
		"DROP TYPE  IF EXISTS assignment_status",
	}
	for _, s := range drops {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		`DO $$ BEGIN
			CREATE TYPE assignment_status AS ENUM
				('unassigned','pending','claimed','revoked','cancelled');
		EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		`CREATE TABLE organizers (
			id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id  UUID NOT NULL UNIQUE,
			name     VARCHAR(255) NOT NULL DEFAULT 'Test Organizer'
		)`,

		`CREATE TABLE events (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			organizer_id UUID REFERENCES organizers(id),
			title        VARCHAR(255) NOT NULL DEFAULT 'Test Event',
			start_time   TIMESTAMPTZ NOT NULL,
			end_time     TIMESTAMPTZ NOT NULL,
			venue_name   VARCHAR(255) DEFAULT '',
			currency     VARCHAR(10) DEFAULT 'ZAR'
		)`,

		`CREATE TABLE bookings (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_number   VARCHAR(50) UNIQUE NOT NULL DEFAULT 'BK-' || substr(gen_random_uuid()::text,1,8),
			event_id         UUID REFERENCES events(id) ON DELETE CASCADE,
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

		`CREATE TABLE ticket_assignments (
			id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_id              UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
			slot_number             INTEGER NOT NULL CHECK (slot_number >= 1),
			status                  assignment_status NOT NULL DEFAULT 'unassigned',
			assignee_name           VARCHAR(200),
			assignee_email          VARCHAR(255),
			claim_token             UUID UNIQUE,
			assigned_at             TIMESTAMPTZ,
			expires_at              TIMESTAMPTZ,
			claimed_at              TIMESTAMPTZ,
			revoked_at              TIMESTAMPTZ,
			qr_code_data            TEXT,
			qr_code_url             TEXT,
			qr_scanned_at           TIMESTAMPTZ,
			scanned_by_organizer_id UUID REFERENCES organizers(id),
			created_at              TIMESTAMPTZ DEFAULT NOW(),
			updated_at              TIMESTAMPTZ DEFAULT NOW(),
			CONSTRAINT uq_booking_slot UNIQUE (booking_id, slot_number)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Skipf("rotating QR DB tests skipped — table setup failed: %v", err)
		}
	}
}

func cleanupRotatingQRTestData(pool *pgxpool.Pool) {
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE TABLE ticket_assignments CASCADE")
	pool.Exec(ctx, "TRUNCATE TABLE bookings           CASCADE")
	pool.Exec(ctx, "TRUNCATE TABLE events             CASCADE")
	pool.Exec(ctx, "TRUNCATE TABLE organizers         CASCADE")
}

// ─── DB fixture helpers ───────────────────────────────────────────────────────

func insertRQROrganizer(t *testing.T, pool *pgxpool.Pool) (organizerID, userID string) {
	t.Helper()
	userID = "00000000-0000-0000-0000-000000000001"
	err := pool.QueryRow(context.Background(),
		`INSERT INTO organizers (user_id, name) VALUES ($1, 'Test Org') RETURNING id`,
		userID,
	).Scan(&organizerID)
	require.NoError(t, err)
	return organizerID, userID
}

func insertRQREvent(t *testing.T, pool *pgxpool.Pool, organizerID string, start, end time.Time) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO events (organizer_id, title, start_time, end_time)
		 VALUES ($1, 'Rotating QR Test Event', $2, $3) RETURNING id`,
		organizerID, start, end,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertRQRBooking(t *testing.T, pool *pgxpool.Pool, eventID, status string) string {
	t.Helper()
	bn := fmt.Sprintf("BK-RQR-%d", time.Now().UnixNano())
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO bookings (booking_number, event_id, ticket_quantity, total_amount, booking_status, payment_status)
		VALUES ($1, $2, 1, 100.00, $3, 'completed') RETURNING id
	`, bn, eventID, status).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertRQRAssignment(t *testing.T, pool *pgxpool.Pool, bookingID, status string) string {
	t.Helper()
	var claimedAt *time.Time
	if status == string(models.AssignmentClaimed) {
		now := time.Now()
		claimedAt = &now
	}
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO ticket_assignments (booking_id, slot_number, status, claimed_at)
		VALUES ($1, 1, $2, $3) RETURNING id
	`, bookingID, status, claimedAt).Scan(&id)
	require.NoError(t, err)
	return id
}

// scanToken replicates the logic in ScanRotatingQRHandler:
//  1. Crypto-verify the token.
//  2. Check DB: assignment status, booking status, event window, organizer ownership.
//  3. Atomically mark qr_scanned_at WHERE qr_scanned_at IS NULL.
func scanToken(t *testing.T, pool *pgxpool.Pool, svc *RotatingQRService, token, scannerUserID string) error {
	t.Helper()
	ctx := context.Background()

	payload, err := svc.VerifyToken(token)
	if err != nil {
		return fmt.Errorf("token invalid: %w", err)
	}

	var (
		assignStatus    string
		scannedAt       *time.Time
		bookingStatus   string
		startTime       time.Time
		endTime         time.Time
		organizerUserID string
	)
	err = pool.QueryRow(ctx, `
		SELECT
			ta.status,
			ta.qr_scanned_at,
			b.booking_status,
			e.start_time,
			e.end_time,
			o.user_id AS organizer_user_id
		FROM ticket_assignments ta
		JOIN bookings   b ON ta.booking_id   = b.id
		JOIN events     e ON b.event_id      = e.id
		JOIN organizers o ON e.organizer_id  = o.id
		WHERE ta.id = $1
	`, payload.AssignmentID).Scan(
		&assignStatus, &scannedAt, &bookingStatus,
		&startTime, &endTime, &organizerUserID,
	)
	if err != nil {
		return fmt.Errorf("ticket not found: %w", err)
	}
	if organizerUserID != scannerUserID {
		return fmt.Errorf("access denied: not the event organizer")
	}
	if assignStatus != string(models.AssignmentClaimed) {
		return fmt.Errorf("ticket not valid (status: %s)", assignStatus)
	}
	if bookingStatus != string(models.BookingConfirmed) {
		return fmt.Errorf("booking no longer valid (status: %s)", bookingStatus)
	}
	now := time.Now()
	if now.Before(startTime.Add(-2*time.Hour)) || now.After(endTime.Add(2*time.Hour)) {
		return fmt.Errorf("outside event admission window")
	}
	tag, err := pool.Exec(ctx,
		`UPDATE ticket_assignments SET qr_scanned_at = NOW() WHERE id = $1 AND qr_scanned_at IS NULL`,
		payload.AssignmentID,
	)
	if err != nil {
		return fmt.Errorf("DB update failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ticket already scanned")
	}
	return nil
}

// ─── DB INTEGRATION TESTS ─────────────────────────────────────────────────────

func TestRotatingQRDB_ScanValidToken(t *testing.T) {
	pool := setupRotatingQRTestDB(t)
	defer cleanupRotatingQRTestData(pool)

	svc := newTestRotatingQR()
	orgID, orgUserID := insertRQROrganizer(t, pool)
	eventID := insertRQREvent(t, pool, orgID, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertRQRBooking(t, pool, eventID, string(models.BookingConfirmed))
	assignmentID := insertRQRAssignment(t, pool, bookingID, string(models.AssignmentClaimed))

	tok, err := svc.GenerateToken(assignmentID, bookingID, eventID, 1)
	require.NoError(t, err)

	require.NoError(t, scanToken(t, pool, svc, tok.Token, orgUserID))

	var scannedAt *time.Time
	err = pool.QueryRow(context.Background(),
		`SELECT qr_scanned_at FROM ticket_assignments WHERE id = $1`, assignmentID,
	).Scan(&scannedAt)
	require.NoError(t, err)
	assert.NotNil(t, scannedAt, "qr_scanned_at must be set after a valid scan")
}

func TestRotatingQRDB_ScanEnforcesSingleEntry(t *testing.T) {
	pool := setupRotatingQRTestDB(t)
	defer cleanupRotatingQRTestData(pool)

	svc := newTestRotatingQR()
	orgID, orgUserID := insertRQROrganizer(t, pool)
	eventID := insertRQREvent(t, pool, orgID, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertRQRBooking(t, pool, eventID, string(models.BookingConfirmed))
	assignmentID := insertRQRAssignment(t, pool, bookingID, string(models.AssignmentClaimed))

	tok, err := svc.GenerateToken(assignmentID, bookingID, eventID, 1)
	require.NoError(t, err)

	// First scan — must succeed
	require.NoError(t, scanToken(t, pool, svc, tok.Token, orgUserID))

	// Second scan with the same token — must be rejected
	err = scanToken(t, pool, svc, tok.Token, orgUserID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already scanned", "second scan must report duplicate entry")
}

func TestRotatingQRDB_ScanRejectsCancelledBooking(t *testing.T) {
	pool := setupRotatingQRTestDB(t)
	defer cleanupRotatingQRTestData(pool)

	svc := newTestRotatingQR()
	orgID, orgUserID := insertRQROrganizer(t, pool)
	eventID := insertRQREvent(t, pool, orgID, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertRQRBooking(t, pool, eventID, string(models.BookingCancelled))
	assignmentID := insertRQRAssignment(t, pool, bookingID, string(models.AssignmentClaimed))

	tok, err := svc.GenerateToken(assignmentID, bookingID, eventID, 1)
	require.NoError(t, err)

	err = scanToken(t, pool, svc, tok.Token, orgUserID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled", "cancelled booking must be rejected at the door")
}

func TestRotatingQRDB_ScanRejectsRefundedBooking(t *testing.T) {
	pool := setupRotatingQRTestDB(t)
	defer cleanupRotatingQRTestData(pool)

	svc := newTestRotatingQR()
	orgID, orgUserID := insertRQROrganizer(t, pool)
	eventID := insertRQREvent(t, pool, orgID, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertRQRBooking(t, pool, eventID, string(models.BookingRefunded))
	assignmentID := insertRQRAssignment(t, pool, bookingID, string(models.AssignmentClaimed))

	tok, err := svc.GenerateToken(assignmentID, bookingID, eventID, 1)
	require.NoError(t, err)

	err = scanToken(t, pool, svc, tok.Token, orgUserID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refunded", "refunded booking must be rejected at the door")
}

func TestRotatingQRDB_ScanRejectsUnclaimedSlot(t *testing.T) {
	pool := setupRotatingQRTestDB(t)
	defer cleanupRotatingQRTestData(pool)

	svc := newTestRotatingQR()
	orgID, orgUserID := insertRQROrganizer(t, pool)
	eventID := insertRQREvent(t, pool, orgID, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertRQRBooking(t, pool, eventID, string(models.BookingConfirmed))
	assignmentID := insertRQRAssignment(t, pool, bookingID, string(models.AssignmentUnassigned))

	tok, err := svc.GenerateToken(assignmentID, bookingID, eventID, 1)
	require.NoError(t, err)

	err = scanToken(t, pool, svc, tok.Token, orgUserID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status", "unclaimed slot must be rejected")
}

func TestRotatingQRDB_ScanRejectsOutsideEventWindow(t *testing.T) {
	pool := setupRotatingQRTestDB(t)
	defer cleanupRotatingQRTestData(pool)

	svc := newTestRotatingQR()
	orgID, orgUserID := insertRQROrganizer(t, pool)
	// Event ended 5 hours ago — outside the 2-hour post-event grace window
	eventID := insertRQREvent(t, pool, orgID, time.Now().Add(-8*time.Hour), time.Now().Add(-5*time.Hour))
	bookingID := insertRQRBooking(t, pool, eventID, string(models.BookingConfirmed))
	assignmentID := insertRQRAssignment(t, pool, bookingID, string(models.AssignmentClaimed))

	tok, err := svc.GenerateToken(assignmentID, bookingID, eventID, 1)
	require.NoError(t, err)

	err = scanToken(t, pool, svc, tok.Token, orgUserID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "window", "scan outside event window must be rejected")
}

func TestRotatingQRDB_ScanRejectsWrongOrganizer(t *testing.T) {
	pool := setupRotatingQRTestDB(t)
	defer cleanupRotatingQRTestData(pool)

	svc := newTestRotatingQR()
	orgID, _ := insertRQROrganizer(t, pool)
	eventID := insertRQREvent(t, pool, orgID, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertRQRBooking(t, pool, eventID, string(models.BookingConfirmed))
	assignmentID := insertRQRAssignment(t, pool, bookingID, string(models.AssignmentClaimed))

	tok, err := svc.GenerateToken(assignmentID, bookingID, eventID, 1)
	require.NoError(t, err)

	wrongUserID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	err = scanToken(t, pool, svc, tok.Token, wrongUserID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied", "non-organizer must be rejected")
}

func TestRotatingQRDB_ScanRejectsExpiredToken(t *testing.T) {
	pool := setupRotatingQRTestDB(t)
	defer cleanupRotatingQRTestData(pool)

	svc := newTestRotatingQR()
	orgID, orgUserID := insertRQROrganizer(t, pool)
	eventID := insertRQREvent(t, pool, orgID, time.Now().Add(-time.Hour), time.Now().Add(3*time.Hour))
	bookingID := insertRQRBooking(t, pool, eventID, string(models.BookingConfirmed))
	assignmentID := insertRQRAssignment(t, pool, bookingID, string(models.AssignmentClaimed))

	// Build a token with a valid HMAC but for a stale window
	staleWindow := currentWindow() - (qrWindowTolerance + 2)
	p := RotatingQRPayload{
		AssignmentID: assignmentID,
		BookingID:    bookingID,
		EventID:      eventID,
		SlotNumber:   1,
		Window:       staleWindow,
		Version:      1,
	}
	data, _ := json.Marshal(p)
	payloadB64 := base64.RawURLEncoding.EncodeToString(data)
	expiredToken := payloadB64 + "." + svc.sign(assignmentID, staleWindow)

	err := scanToken(t, pool, svc, expiredToken, orgUserID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token invalid", "expired token must be rejected before DB is checked")
}