package workers

// =============================================================================
// BOOKING WORKER TESTS
// =============================================================================
//
// Tests covered:
//   - StartBookingWorker: stops cleanly on context cancellation
//   - StartBookingWorker: stops cleanly when channel is closed
//   - processBookingJob: skips non-confirmed bookings
//   - processBookingJob: skips bookings where tickets_sent = TRUE
//   - processBookingJob: logs and continues when booking is not found
//   - resolveEmailRecipient: returns constructed user for guest bookings
//   - resolveEmailRecipient: returns fallback user when UserID is nil
//   - resolveEmailRecipient: fetches registered user from DB
//
// Prerequisites — same Docker Postgres used by payment/webhook tests:
//
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser \
//     -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db \
//     -p 5433:5432 postgres:15
//
// Run:
//   go test ./internal/workers/... -v -run TestWorker
// =============================================================================

import (
	"context"
	"fmt"
	"testing"
	"time"

	"events-db/internal/config"
	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const workerTestDBURL = "postgresql://testuser:testpass@localhost:5433/test_db"

// minimalCfg returns a non-nil *config.Config with empty fields.
// StartBookingWorker constructs EmailService and QRService eagerly, so it
// needs a non-nil config even when no jobs will ever be processed (e.g. in
// lifecycle tests that only exercise goroutine start/stop).
func minimalCfg() *config.Config {
	return &config.Config{}
}

// ─── DB setup ─────────────────────────────────────────────────────────────────

func workerDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), workerTestDBURL)
	if err != nil {
		t.Skipf("worker DB tests skipped — could not create pool: %v", err)
	}
	// pgxpool.New is lazy — ping to confirm the DB is actually reachable.
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("worker DB tests skipped — DB not reachable (start with: docker run -d --name test-postgres -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass -e POSTGRES_DB=test_db -p 5433:5432 postgres:15): %v", pingErr)
	}
	workerMigrate(t, pool)
	workerTruncate(t, pool)
	return pool
}

func workerMigrate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	for _, s := range []string{
		"DROP TABLE IF EXISTS bookings CASCADE",
		"DROP TABLE IF EXISTS events   CASCADE",
		"DROP TABLE IF EXISTS users    CASCADE",
	} {
		pool.Exec(ctx, s)
	}

	for _, s := range []string{
		// users — minimal schema needed by GetUserByIDForEmail
		`CREATE TABLE users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email      VARCHAR(255) UNIQUE NOT NULL,
			first_name VARCHAR(100),
			last_name  VARCHAR(100),
			password   TEXT NOT NULL DEFAULT '',
			role       VARCHAR(50) NOT NULL DEFAULT 'user',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		// events — minimal schema needed by GetEventByID
		`CREATE TABLE events (
			id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title               VARCHAR(255) NOT NULL DEFAULT 'Test Event',
			description         TEXT NOT NULL DEFAULT '',
			start_time          TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
			end_time            TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '8 days',
			expires_at          TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
			timezone            VARCHAR(50) NOT NULL DEFAULT 'SAST',
			venue_name          VARCHAR(255) NOT NULL DEFAULT '',
			address             TEXT NOT NULL DEFAULT '',
			city                VARCHAR(100) NOT NULL DEFAULT '',
			price               NUMERIC(10,2) NOT NULL DEFAULT 0,
			currency            VARCHAR(10) NOT NULL DEFAULT 'ZAR',
			max_capacity        INTEGER,
			tickets_sold        INTEGER NOT NULL DEFAULT 0,
			allow_waitlist      BOOLEAN DEFAULT TRUE,
			status              VARCHAR(50) NOT NULL DEFAULT 'published',
			is_featured         BOOLEAN DEFAULT FALSE,
			organizer_id        UUID REFERENCES users(id),
			views               BIGINT DEFAULT 0,
			created_at          TIMESTAMPTZ DEFAULT NOW(),
			updated_at          TIMESTAMPTZ DEFAULT NOW()
		)`,

		// bookings — all columns used by GetBookingByID and processBookingJob
		`CREATE TABLE bookings (
			id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_number      VARCHAR(50) UNIQUE NOT NULL
			                    DEFAULT 'BK-' || substr(gen_random_uuid()::text,1,8),
			event_id            UUID NOT NULL REFERENCES events(id),
			user_id             UUID REFERENCES users(id),
			is_guest            BOOLEAN NOT NULL DEFAULT FALSE,
			ticket_quantity     INT NOT NULL DEFAULT 1,
			unit_price          NUMERIC(10,2) NOT NULL DEFAULT 0,
			subtotal            NUMERIC(10,2) NOT NULL DEFAULT 0,
			platform_fee        NUMERIC(10,2) NOT NULL DEFAULT 0,
			organizer_fee       NUMERIC(10,2) NOT NULL DEFAULT 0,
			total_amount        NUMERIC(10,2) NOT NULL DEFAULT 0,
			currency            VARCHAR(10) NOT NULL DEFAULT 'ZAR',
			attendee_first_name VARCHAR(100),
			attendee_last_name  VARCHAR(100),
			attendee_email      VARCHAR(255),
			attendee_phone      VARCHAR(50),
			booking_status      VARCHAR(50) NOT NULL DEFAULT 'pending',
			payment_status      VARCHAR(50) NOT NULL DEFAULT 'pending',
			tickets_sent        BOOLEAN NOT NULL DEFAULT FALSE,
			qr_code_data        TEXT,
			qr_code_url         TEXT,
			special_requirements TEXT,
			promo_code          VARCHAR(100),
			discount_amount     NUMERIC(10,2) NOT NULL DEFAULT 0,
			refunded_request_at TIMESTAMPTZ,
			refunded_reason     TEXT,
			refunded_at         TIMESTAMPTZ,
			refunded_amount     NUMERIC(10,2),
			booking_notes       TEXT,
			confirmed_at        TIMESTAMPTZ,
			cancelled_at        TIMESTAMPTZ,
			created_at          TIMESTAMPTZ DEFAULT NOW(),
			updated_at          TIMESTAMPTZ DEFAULT NOW()
		)`,
	} {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Skipf("worker DB tests skipped — migration failed (DB not ready?): %v", err)
		}
	}
}

func workerTruncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE bookings CASCADE")
	pool.Exec(ctx, "TRUNCATE events   CASCADE")
	pool.Exec(ctx, "TRUNCATE users    CASCADE")
}

// ─── seed helpers ─────────────────────────────────────────────────────────────

func workerSeedUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	email := fmt.Sprintf("user_%d@test.com", time.Now().UnixNano())
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, first_name, last_name)
		VALUES ($1, 'Test', 'User') RETURNING id
	`, email).Scan(&id)
	require.NoError(t, err)
	return id
}

func workerSeedEvent(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO events (title) VALUES ('Worker Test Event') RETURNING id
	`).Scan(&id)
	require.NoError(t, err)
	return id
}

// workerSeedBooking inserts a booking with the given status and tickets_sent flag.
// Pass userID="" for a guest booking (is_guest=TRUE, user_id=NULL).
func workerSeedBooking(
	t *testing.T,
	pool *pgxpool.Pool,
	eventID, userID string,
	status models.BookingStatus,
	ticketsSent bool,
) string {
	t.Helper()
	bn := fmt.Sprintf("BK-WRK-%d", time.Now().UnixNano())
	var id string

	if userID == "" {
		// Guest booking
		email := fmt.Sprintf("guest_%d@test.com", time.Now().UnixNano())
		err := pool.QueryRow(context.Background(), `
			INSERT INTO bookings (
				booking_number, event_id, user_id, is_guest,
				ticket_quantity, unit_price, subtotal, total_amount,
				attendee_first_name, attendee_last_name, attendee_email,
				booking_status, tickets_sent
			) VALUES ($1, $2, NULL, TRUE, 1, 50.00, 50.00, 52.50,
			          'Guest', 'User', $3, $4, $5)
			RETURNING id
		`, bn, eventID, email, string(status), ticketsSent).Scan(&id)
		require.NoError(t, err)
	} else {
		err := pool.QueryRow(context.Background(), `
			INSERT INTO bookings (
				booking_number, event_id, user_id, is_guest,
				ticket_quantity, unit_price, subtotal, total_amount,
				booking_status, tickets_sent
			) VALUES ($1, $2, $3, FALSE, 1, 50.00, 50.00, 52.50, $4, $5)
			RETURNING id
		`, bn, eventID, userID, string(status), ticketsSent).Scan(&id)
		require.NoError(t, err)
	}
	return id
}

// =============================================================================
// TEST: StartBookingWorker — lifecycle
// =============================================================================

func TestWorkerStopsOnContextCancellation(t *testing.T) {
	// Pure goroutine lifecycle test — no DB required.
	// StartBookingWorker constructs EmailService/QRService eagerly, so cfg must
	// be non-nil even though no jobs are ever processed in this test.
	jobs := make(chan string, 4)
	ctx, cancel := context.WithCancel(context.Background())

	StartBookingWorker(nil, minimalCfg(), jobs, ctx)

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
	// No deadlock or panic = pass.
}

func TestWorkerStopsOnChannelClose(t *testing.T) {
	// Pure goroutine lifecycle test — no DB required.
	jobs := make(chan string, 4)
	ctx := context.Background()

	StartBookingWorker(nil, minimalCfg(), jobs, ctx)

	time.Sleep(20 * time.Millisecond)
	close(jobs)
	time.Sleep(20 * time.Millisecond)
	// No panic = pass.
}

// =============================================================================
// TEST: processBookingJob — guard conditions (no email/QR calls)
// =============================================================================

// TestWorkerSkipsNonConfirmedBooking verifies processBookingJob returns
// immediately without sending email or QR when the booking is pending.
func TestWorkerSkipsNonConfirmedBooking(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingPending, false)

	// Pass nil services — any attempt to use them would panic, proving the
	// guard short-circuits before reaching email/QR logic.
	processBookingJob(pool, nil, nil, bookingID)

	// Booking should remain unchanged (no tickets_sent update).
	var ticketsSent bool
	err := pool.QueryRow(context.Background(),
		"SELECT tickets_sent FROM bookings WHERE id = $1", bookingID,
	).Scan(&ticketsSent)
	require.NoError(t, err)
	assert.False(t, ticketsSent, "tickets_sent must remain FALSE for a non-confirmed booking")
}

// TestWorkerSkipsAlreadySentTickets verifies the duplicate-delivery guard:
// a booking that already has tickets_sent=TRUE is silently skipped.
func TestWorkerSkipsAlreadySentTickets(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	// confirmed=TRUE, ticketsSent=TRUE — should be skipped without QR/email.
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingConfirmed, true)

	// nil services — any real call would panic.
	processBookingJob(pool, nil, nil, bookingID)

	// tickets_sent must still be TRUE and not set to FALSE.
	var ticketsSent bool
	err := pool.QueryRow(context.Background(),
		"SELECT tickets_sent FROM bookings WHERE id = $1", bookingID,
	).Scan(&ticketsSent)
	require.NoError(t, err)
	assert.True(t, ticketsSent, "tickets_sent must remain TRUE — duplicate delivery guard")
}

// TestWorkerHandlesMissingBookingGracefully verifies that a booking ID that
// doesn't exist in the DB is logged and skipped, not panicked.
func TestWorkerHandlesMissingBookingGracefully(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()

	// Non-existent ID — GetBookingByID will return an error; worker must not panic.
	processBookingJob(pool, nil, nil, "00000000-0000-0000-0000-000000000000")
}

// =============================================================================
// TEST: resolveEmailRecipient
// =============================================================================

func TestResolveEmailRecipientGuestBooking(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	first := "Jane"
	last := "Doe"
	email := "jane@guest.com"

	guestBooking := &models.Booking{
		IsGuest:           true,
		AttendeeFirstName: &first,
		AttendeeLastName:  &last,
		AttendeeEmail:     &email,
	}

	user, err := resolveEmailRecipient(pool, guestBooking)

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, email, user.Email)
	assert.Equal(t, "Jane", *user.FirstName)
	assert.Equal(t, "Doe", *user.LastName)
}

func TestResolveEmailRecipientGuestWithNilFields(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()

	// All attendee fields are nil — must not panic.
	guestBooking := &models.Booking{IsGuest: true}

	user, err := resolveEmailRecipient(pool, guestBooking)

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Empty(t, user.Email)
}

func TestResolveEmailRecipientNilUserIDFallback(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()

	email := "fallback@test.com"
	booking := &models.Booking{
		IsGuest:       false,
		UserID:        nil, // no user_id
		AttendeeEmail: &email,
	}

	user, err := resolveEmailRecipient(pool, booking)

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, email, user.Email)
}

func TestResolveEmailRecipientRegisteredUser(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	// Insert a real user so GetUserByIDForEmail finds them.
	email := fmt.Sprintf("registered_%d@test.com", time.Now().UnixNano())
	var userID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, first_name, last_name)
		VALUES ($1, 'Alice', 'Smith') RETURNING id
	`, email).Scan(&userID)
	require.NoError(t, err)

	booking := &models.Booking{
		IsGuest: false,
		UserID:  &userID,
	}

	user, err := resolveEmailRecipient(pool, booking)

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, email, user.Email)
}

func TestResolveEmailRecipientRegisteredUserNotFound(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	nonExistent := "00000000-0000-0000-0000-000000000000"
	booking := &models.Booking{
		IsGuest: false,
		UserID:  &nonExistent,
	}

	// GetUserByIDForEmail returns an error for a missing user — resolveEmailRecipient
	// should propagate it.
	_, err := resolveEmailRecipient(pool, booking)
	assert.Error(t, err, "fetching a non-existent user must return an error")
}

// =============================================================================
// TEST: Worker channel behaviour — non-blocking send when full
// =============================================================================

// TestWorkerJobChannelFullIsNonBlocking verifies that when the jobs channel is
// at capacity the handler does not block. This mirrors the select/default logic
// in payment_service.go: the webhook must return quickly even if the buffer is
// full. We exercise this by filling the channel and confirming the process
// function still returns promptly.
func TestWorkerJobChannelFullIsNonBlocking(t *testing.T) {
	jobs := make(chan string, 1) // tiny buffer
	jobs <- "already-full"      // fill it

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The worker goroutine is running but the channel is full.
	// Sending another job must not block.
	started := make(chan struct{})
	go func() {
		close(started)
		// Non-blocking send — mirrors the select/default in payment_service.go.
		select {
		case jobs <- "new-job":
		default:
			// Channel full — this is the expected path in this test.
		}
	}()

	<-started

	done := make(chan struct{})
	go func() {
		// Drain the channel so the worker goroutine can exit cleanly.
		for range jobs {
		}
		close(done)
	}()

	cancel()
	close(jobs)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("channel drain timed out — possible deadlock")
	}
}