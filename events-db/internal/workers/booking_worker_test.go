package workers

// =============================================================================
// BOOKING WORKER TESTS
// =============================================================================
//
// Tests covered:
//   Lifecycle
//   - StartBookingWorker stops cleanly on context cancellation
//   - StartBookingWorker stops cleanly when channel is closed
//
//   processBookingJob guard conditions
//   - Skips non-confirmed bookings (nil services prove no QR/email calls made)
//   - Skips bookings where tickets_sent = TRUE (duplicate delivery guard)
//   - Handles missing booking ID gracefully (no panic)
//
//   Assignment creation
//   - Creates N assignment rows on a confirmed booking
//   - Slot 1 is auto-claimed by the buyer
//   - Remaining slots start as unassigned
//
//   fetchAssignmentIDsForBooking
//   - Returns IDs in slot_number order
//   - Returns empty slice for booking with no assignments
//
//   fetchQRURLForAssignment
//   - Returns empty string when qr_code_url is NULL
//   - Returns URL when set
//
//   resolveEmailRecipient
//   - Guest booking returns constructed user from attendee fields
//   - Guest booking with all-nil fields does not panic
//   - Non-guest with nil UserID returns fallback user with attendee email
//   - Non-guest with UserID fetches registered user from DB
//   - Non-guest with non-existent UserID propagates error
//
//   Worker channel behaviour
//   - Non-blocking send when channel is full
//
// Prerequisites:
//   docker run -d --name test-postgres \
//     -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
//     -e POSTGRES_DB=test_db -p 5433:5432 postgres:15
//
// Run:  go test ./internal/workers/... -v -run TestWorker
// =============================================================================

import (
	"context"
	"fmt"
	"testing"
	"time"

	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const workerTestDBURL = "postgresql://testuser:testpass@localhost:5433/test_db"

func minimalCfg() *config.Config { return &config.Config{} }

// ─── DB setup ─────────────────────────────────────────────────────────────────

func workerDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), workerTestDBURL)
	if err != nil {
		t.Skipf("worker DB tests skipped — could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("worker DB tests skipped — DB not reachable: %v", pingErr)
	}
	workerMigrate(t, pool)
	workerTruncate(t, pool)
	return pool
}

func workerMigrate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
		"DROP TABLE IF EXISTS ticket_assignments CASCADE",
		"DROP TABLE IF EXISTS bookings           CASCADE",
		"DROP TABLE IF EXISTS events             CASCADE",
		"DROP TABLE IF EXISTS users              CASCADE",
		"DROP TYPE  IF EXISTS assignment_status",
	}
	for _, s := range drops {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		`DO $$ BEGIN CREATE TYPE assignment_status AS ENUM
		 ('unassigned','pending','claimed','revoked','cancelled');
		 EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		`CREATE TABLE users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email      VARCHAR(255) UNIQUE NOT NULL,
			first_name VARCHAR(100),
			last_name  VARCHAR(100),
			password   TEXT NOT NULL DEFAULT '',
			role       VARCHAR(50) NOT NULL DEFAULT 'attendee',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE events (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title           VARCHAR(255) NOT NULL DEFAULT 'Test Event',
			description     TEXT NOT NULL DEFAULT '',
			start_time      TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
			end_time        TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '8 days',
			expires_at      TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
			timezone        VARCHAR(50) NOT NULL DEFAULT 'SAST',
			venue_name      VARCHAR(255) NOT NULL DEFAULT '',
			address         TEXT NOT NULL DEFAULT '',
			city            VARCHAR(100) NOT NULL DEFAULT '',
			price           NUMERIC(10,2) NOT NULL DEFAULT 0,
			currency        VARCHAR(10) NOT NULL DEFAULT 'ZAR',
			max_capacity    INTEGER,
			tickets_sold    INTEGER NOT NULL DEFAULT 0,
			allow_waitlist  BOOLEAN DEFAULT TRUE,
			status          VARCHAR(50) NOT NULL DEFAULT 'published',
			is_featured     BOOLEAN DEFAULT FALSE,
			organizer_id    UUID REFERENCES users(id),
			views           BIGINT DEFAULT 0,
			created_at      TIMESTAMPTZ DEFAULT NOW(),
			updated_at      TIMESTAMPTZ DEFAULT NOW()
		)`,

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
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Skipf("worker migration failed (DB not ready?): %v", err)
		}
	}
}

func workerTruncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE ticket_assignments CASCADE")
	pool.Exec(ctx, "TRUNCATE bookings           CASCADE")
	pool.Exec(ctx, "TRUNCATE events             CASCADE")
	pool.Exec(ctx, "TRUNCATE users              CASCADE")
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

func workerSeedBooking(
	t *testing.T,
	pool *pgxpool.Pool,
	eventID, userID string,
	status models.BookingStatus,
	ticketsSent bool,
	quantity int,
) string {
	t.Helper()
	if quantity < 1 {
		quantity = 1
	}
	bn := fmt.Sprintf("BK-WRK-%d", time.Now().UnixNano())
	var id string

	if userID == "" {
		email := fmt.Sprintf("guest_%d@test.com", time.Now().UnixNano())
		err := pool.QueryRow(context.Background(), `
			INSERT INTO bookings (
				booking_number, event_id, user_id, is_guest,
				ticket_quantity, unit_price, subtotal, total_amount,
				attendee_first_name, attendee_last_name, attendee_email,
				booking_status, tickets_sent
			) VALUES ($1, $2, NULL, TRUE, $3, 50.00, 50.00, 52.50,
			          'Guest', 'User', $4, $5, $6)
			RETURNING id
		`, bn, eventID, quantity, email, string(status), ticketsSent).Scan(&id)
		require.NoError(t, err)
	} else {
		err := pool.QueryRow(context.Background(), `
			INSERT INTO bookings (
				booking_number, event_id, user_id, is_guest,
				ticket_quantity, unit_price, subtotal, total_amount,
				attendee_first_name, attendee_last_name, attendee_email,
				booking_status, tickets_sent
			) VALUES ($1, $2, $3, FALSE, $4, 50.00, 50.00, 52.50,
			          'Test', 'User', 'test@test.com', $5, $6)
			RETURNING id
		`, bn, eventID, userID, quantity, string(status), ticketsSent).Scan(&id)
		require.NoError(t, err)
	}
	return id
}

// ─── Lifecycle tests ──────────────────────────────────────────────────────────

func TestWorkerStopsOnContextCancellation(t *testing.T) {
	jobs := make(chan string, 4)
	ctx, cancel := context.WithCancel(context.Background())
	StartBookingWorker(nil, minimalCfg(), jobs, ctx)
	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
	// No deadlock or panic = pass.
}

func TestWorkerStopsOnChannelClose(t *testing.T) {
	jobs := make(chan string, 4)
	StartBookingWorker(nil, minimalCfg(), jobs, context.Background())
	time.Sleep(20 * time.Millisecond)
	close(jobs)
	time.Sleep(20 * time.Millisecond)
	// No panic = pass.
}

// ─── processBookingJob guard conditions ──────────────────────────────────────

func TestWorkerSkipsNonConfirmedBooking(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingPending, false, 1)

	// nil services — any email/QR call would panic, proving the guard fires first.
	processBookingJob(pool, nil, nil, bookingID)

	var ticketsSent bool
	err := pool.QueryRow(context.Background(),
		"SELECT tickets_sent FROM bookings WHERE id = $1", bookingID,
	).Scan(&ticketsSent)
	require.NoError(t, err)
	assert.False(t, ticketsSent, "tickets_sent must remain FALSE for a non-confirmed booking")
}

func TestWorkerSkipsAlreadySentTickets(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingConfirmed, true, 2)

	processBookingJob(pool, nil, nil, bookingID)

	var ticketsSent bool
	err := pool.QueryRow(context.Background(),
		"SELECT tickets_sent FROM bookings WHERE id = $1", bookingID,
	).Scan(&ticketsSent)
	require.NoError(t, err)
	assert.True(t, ticketsSent, "tickets_sent must remain TRUE — duplicate delivery guard")
}

func TestWorkerHandlesMissingBookingGracefully(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	// A random UUID that doesn't exist in the DB.
	processBookingJob(pool, nil, nil, "00000000-0000-0000-0000-000000000000")
	// No panic = pass.
}

// ─── Assignment creation ──────────────────────────────────────────────────────

func TestWorkerCreatesAssignmentSlotsOnConfirmedBooking(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)

	const quantity = 4
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingConfirmed, false, quantity)

	err := repository.CreateTicketAssignments(pool, bookingID, quantity, "Test User", "test@test.com")
	require.NoError(t, err)

	var count int
	pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM ticket_assignments WHERE booking_id = $1`, bookingID,
	).Scan(&count)
	assert.Equal(t, quantity, count, "one assignment row must be created per ticket")
}

func TestWorkerSlot1IsAutoClaimedByBuyer(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingConfirmed, false, 3)

	err := repository.CreateTicketAssignments(pool, bookingID, 3, "Test User", "test@test.com")
	require.NoError(t, err)

	var slot1Status, slot1Email string
	pool.QueryRow(context.Background(), `
		SELECT status, assignee_email
		FROM ticket_assignments
		WHERE booking_id = $1 AND slot_number = 1
	`, bookingID).Scan(&slot1Status, &slot1Email)

	assert.Equal(t, "claimed", slot1Status, "slot 1 must be pre-claimed")
	assert.Equal(t, "test@test.com", slot1Email, "slot 1 must be assigned to the buyer's email")
}

func TestWorkerRemainingSlotStartUnassigned(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingConfirmed, false, 3)

	err := repository.CreateTicketAssignments(pool, bookingID, 3, "Test User", "test@test.com")
	require.NoError(t, err)

	var unassignedCount int
	pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM ticket_assignments
		WHERE booking_id = $1 AND slot_number > 1 AND status = 'unassigned'
	`, bookingID).Scan(&unassignedCount)

	assert.Equal(t, 2, unassignedCount, "slots 2 and 3 must start as unassigned")
}

func TestWorkerCreateAssignmentsIsIdempotent(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingConfirmed, false, 2)

	// Calling twice (e.g. after a worker restart) must not create duplicate rows.
	require.NoError(t, repository.CreateTicketAssignments(pool, bookingID, 2, "Test User", "test@test.com"))
	require.NoError(t, repository.CreateTicketAssignments(pool, bookingID, 2, "Test User", "test@test.com"))

	var count int
	pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM ticket_assignments WHERE booking_id = $1`, bookingID,
	).Scan(&count)
	assert.Equal(t, 2, count, "idempotent call must not create duplicate rows (ON CONFLICT DO NOTHING)")
}

// ─── fetchAssignmentIDsForBooking ─────────────────────────────────────────────

func TestFetchAssignmentIDsReturnsInSlotOrder(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingConfirmed, false, 3)
	require.NoError(t, repository.CreateTicketAssignments(pool, bookingID, 3, "Test User", "test@test.com"))

	ids, err := fetchAssignmentIDsForBooking(pool, bookingID)
	require.NoError(t, err)
	assert.Len(t, ids, 3, "should return one ID per slot")

	// Verify they are in slot_number order by checking the DB directly.
	for i, id := range ids {
		var slotNumber int
		pool.QueryRow(context.Background(),
			`SELECT slot_number FROM ticket_assignments WHERE id = $1`, id,
		).Scan(&slotNumber)
		assert.Equal(t, i+1, slotNumber, "IDs must be returned in ascending slot_number order")
	}
}

func TestFetchAssignmentIDsReturnsEmptySliceForNoAssignments(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingConfirmed, false, 1)

	ids, err := fetchAssignmentIDsForBooking(pool, bookingID)
	require.NoError(t, err)
	assert.Empty(t, ids, "no assignment rows = empty slice, not an error")
}

// ─── fetchQRURLForAssignment ──────────────────────────────────────────────────

func TestFetchQRURLReturnsEmptyWhenNull(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingConfirmed, false, 1)
	require.NoError(t, repository.CreateTicketAssignments(pool, bookingID, 1, "Test User", "test@test.com"))

	ids, err := fetchAssignmentIDsForBooking(pool, bookingID)
	require.NoError(t, err)
	require.Len(t, ids, 1)

	url := fetchQRURLForAssignment(pool, ids[0])
	assert.Empty(t, url, "qr_code_url is NULL after creation — must return empty string")
}

func TestFetchQRURLReturnsURLWhenSet(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	eventID := workerSeedEvent(t, pool)
	userID := workerSeedUser(t, pool)
	bookingID := workerSeedBooking(t, pool, eventID, userID, models.BookingConfirmed, false, 1)
	require.NoError(t, repository.CreateTicketAssignments(pool, bookingID, 1, "Test User", "test@test.com"))

	ids, err := fetchAssignmentIDsForBooking(pool, bookingID)
	require.NoError(t, err)

	expectedURL := "https://pub-test.r2.dev/qr-codes/test-slot.png"
	_, err = pool.Exec(context.Background(),
		`UPDATE ticket_assignments SET qr_code_url = $1 WHERE id = $2`, expectedURL, ids[0],
	)
	require.NoError(t, err)

	url := fetchQRURLForAssignment(pool, ids[0])
	assert.Equal(t, expectedURL, url)
}

// ─── resolveEmailRecipient ────────────────────────────────────────────────────

func TestResolveEmailRecipientGuestBooking(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()

	first, last, email := "Jane", "Doe", "jane@guest.com"
	user, err := resolveEmailRecipient(pool, &models.Booking{
		IsGuest:           true,
		AttendeeFirstName: &first,
		AttendeeLastName:  &last,
		AttendeeEmail:     &email,
	})

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, email, user.Email)
	assert.Equal(t, "Jane", *user.FirstName)
	assert.Equal(t, "Doe", *user.LastName)
}

func TestResolveEmailRecipientGuestWithNilFields(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()

	// All attendee fields nil — must not panic.
	user, err := resolveEmailRecipient(pool, &models.Booking{IsGuest: true})

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Empty(t, user.Email)
}

func TestResolveEmailRecipientNilUserIDFallback(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()

	email := "fallback@test.com"
	user, err := resolveEmailRecipient(pool, &models.Booking{
		IsGuest:       false,
		UserID:        nil,
		AttendeeEmail: &email,
	})

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, email, user.Email)
}

func TestResolveEmailRecipientRegisteredUser(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	email := fmt.Sprintf("registered_%d@test.com", time.Now().UnixNano())
	var userID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, first_name, last_name)
		VALUES ($1, 'Alice', 'Smith') RETURNING id
	`, email).Scan(&userID)
	require.NoError(t, err)

	user, err := resolveEmailRecipient(pool, &models.Booking{IsGuest: false, UserID: &userID})

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, email, user.Email)
	assert.Equal(t, "Alice", *user.FirstName)
}

func TestResolveEmailRecipientRegisteredUserNotFound(t *testing.T) {
	pool := workerDB(t)
	defer pool.Close()
	workerTruncate(t, pool)

	nonExistent := "00000000-0000-0000-0000-000000000000"
	_, err := resolveEmailRecipient(pool, &models.Booking{IsGuest: false, UserID: &nonExistent})
	assert.Error(t, err, "missing user must propagate the error")
}

// ─── Channel behaviour ────────────────────────────────────────────────────────

func TestWorkerJobChannelFullIsNonBlocking(t *testing.T) {
	jobs := make(chan string, 1)
	jobs <- "already-full"

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	go func() {
		close(started)
		select {
		case jobs <- "new-job":
		default:
			// Channel full — expected path.
		}
	}()
	<-started

	done := make(chan struct{})
	go func() {
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