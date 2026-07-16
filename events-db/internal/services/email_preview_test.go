package services

// =============================================================================
// EMAIL PREVIEW SUITE
// =============================================================================
//
// Sends ONE of every email type the platform produces to a real inbox, so the
// branded templates can be eyeballed in actual email clients (Gmail, Outlook,
// phone). These deliver REAL mail via SendGrid — they are previews, not unit
// tests — and skip automatically when SENDGRID_API_KEY is absent.
//
// Recipient:   EMAIL_TEST_RECIPIENT env var (default below).
// Links/logo:  EMAIL_TEST_FRONTEND_URL env var (default https://ticket-hawk.com).
//              The brand logo is loaded from {frontend}/brand-logo.png, so this
//              MUST be a publicly reachable URL — if it pointed at
//              http://localhost:3000 the logo would never render in Gmail.
//
// ── Run natively (Go is installed, .env at events-db/ supplies the key) ──────
//
//   cd events-db
//   go test ./internal/services/ -run TestEmailPreviewAll -v
//
// ── Run via local Docker (no local Go needed; PowerShell) ────────────────────
//
//   cd events-db
//   docker run --rm -v "${PWD}:/app" -w /app --env-file .env `
//     -e EMAIL_TEST_RECIPIENT=you@example.com `
//     golang:1.25 go test ./internal/services/ -run TestEmailPreviewAll -v
//
// ── Send just one template ───────────────────────────────────────────────────
//
//   go test ./internal/services/ -run "TestEmailPreviewAll/password_reset" -v
//
// =============================================================================

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"events-db/internal/config"
	"events-db/internal/models"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
)

const defaultTestRecipient = "ribnoma@gmail.com"

func testRecipient() string {
	if v := os.Getenv("EMAIL_TEST_RECIPIENT"); v != "" {
		return v
	}
	return defaultTestRecipient
}

// loadRootEnv walks up from the test's working directory to the module root
// (where go.mod lives) and loads the .env found there, so the suite works no
// matter which directory `go test` is invoked from.
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

func previewConfig(t *testing.T) *config.Config {
	t.Helper()
	loadRootEnv()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.SendGridAPIKey == "" {
		t.Skip("SENDGRID_API_KEY not set — skipping live email previews")
	}
	if cfg.EmailFromAddress == "" {
		t.Skip("EMAIL_FROM_ADDRESS not set — skipping live email previews")
	}
	if cfg.EmailFromName == "" {
		cfg.EmailFromName = "TicketHawk"
	}

	// Force a PUBLIC frontend origin for the preview: the header logo is an
	// <img> pointing at {frontend}/brand-logo.png, and buttons deep-link into
	// the web app. A localhost origin from the dev .env would leave the logo
	// broken and the links dead in your inbox.
	frontend := os.Getenv("EMAIL_TEST_FRONTEND_URL")
	if frontend == "" {
		frontend = "https://ticket-hawk.com"
	}
	cfg.FrontendURLs = []string{frontend}

	return cfg
}

// ─── Fixtures ─────────────────────────────────────────────────────────────────

func previewUser() *models.User {
	first, last := "Sipho", "Dlamini"
	return &models.User{
		ID:        "user-test-001",
		FirstName: &first,
		LastName:  &last,
		Email:     testRecipient(),
	}
}

func previewEvent() *models.Event {
	reason := "Venue unavailable due to unforeseen circumstances"
	return &models.Event{
		ID:                 "event-test-001",
		Title:              "Afrobeats Summer Festival 2026",
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

func previewBooking() *models.Booking {
	userID := "user-test-001"
	var refunded float64 = 680.00
	return &models.Booking{
		ID:             "booking-test-001",
		UserID:         &userID, // account holder → plain /booking/:id link
		BookingNumber:  "BK-20260716-001",
		TicketQuantity: 2,
		UnitPrice:      340.00,
		TotalAmount:    680.00,
		Currency:       "ZAR",
		RefundedAmount: &refunded,
		CreatedAt:      time.Now().Add(-48 * time.Hour),
	}
}

func previewOrganizer() *models.Organizer {
	return &models.Organizer{
		ID:           "org-test-001",
		BusinessName: "Maluk Events Co.",
	}
}

// ─── The suite ────────────────────────────────────────────────────────────────

// TestEmailPreviewAll sends every email template, in a stable order, with a
// short pause between sends so they arrive in your inbox in roughly the same
// order they appear here.
func TestEmailPreviewAll(t *testing.T) {
	cfg := previewConfig(t)
	svc := NewEmailService(cfg)
	frontend := cfg.FrontendURLs[0]

	pause := func() { time.Sleep(400 * time.Millisecond) }

	t.Run("booking_confirmation_single", func(t *testing.T) {
		require.NoError(t, svc.SendBookingConfirmation(previewBooking(), previewEvent(), previewUser()))
		t.Logf("booking confirmation (1 ticket type view) sent to %s", testRecipient())
		pause()
	})

	t.Run("booking_confirmation_multi", func(t *testing.T) {
		b := previewBooking()
		b.TicketQuantity = 4
		b.TotalAmount = 1360.00
		require.NoError(t, svc.SendBookingConfirmation(b, previewEvent(), previewUser()))
		t.Logf("booking confirmation (multi-ticket, shows assign-your-group note) sent to %s", testRecipient())
		pause()
	})

	t.Run("booking_confirmation_guest", func(t *testing.T) {
		b := previewBooking()
		b.UserID = nil // guest booking → link carries ?bn=&email= credentials
		require.NoError(t, svc.SendBookingConfirmation(b, previewEvent(), previewUser()))
		t.Logf("booking confirmation (guest link variant) sent to %s", testRecipient())
		pause()
	})

	t.Run("refund_confirmation", func(t *testing.T) {
		require.NoError(t, svc.SendRefundConfirmation(previewBooking(), previewEvent(), previewUser()))
		t.Logf("refund confirmation sent to %s", testRecipient())
		pause()
	})

	t.Run("event_cancellation", func(t *testing.T) {
		require.NoError(t, svc.SendEventCancellation(previewEvent(), previewUser(), previewBooking()))
		t.Logf("event cancellation sent to %s", testRecipient())
		pause()
	})

	t.Run("waitlist_notification", func(t *testing.T) {
		require.NoError(t, svc.SendWaitlistNotification(previewEvent(), previewUser()))
		t.Logf("waitlist notification sent to %s", testRecipient())
		pause()
	})

	t.Run("password_reset", func(t *testing.T) {
		resetLink := frontend + "/reset-password?token=550e8400-e29b-41d4-a716-446655440000"
		require.NoError(t, svc.SendPasswordReset(testRecipient(), resetLink))
		t.Logf("password reset sent to %s", testRecipient())
		pause()
	})

	t.Run("organizer_approval", func(t *testing.T) {
		require.NoError(t, svc.SendOrganizerApproval(previewUser(), previewOrganizer()))
		t.Logf("organizer approval sent to %s", testRecipient())
		pause()
	})

	t.Run("organizer_rejection", func(t *testing.T) {
		require.NoError(t, svc.SendOrganizerRejection(previewUser(), "Application incomplete — missing business registration number"))
		t.Logf("organizer rejection sent to %s", testRecipient())
		pause()
	})

	t.Run("ticket_assignment_invite", func(t *testing.T) {
		claimURL := frontend + "/claim-ticket?token=550e8400-e29b-41d4-a716-446655440000"
		require.NoError(t, svc.SendTicketAssignmentInvite(
			testRecipient(),
			"Thandi Nkosi",
			"Sipho Dlamini",
			previewEvent().Title,
			claimURL,
			time.Now().Add(24*time.Hour),
		))
		t.Logf("ticket assignment invite sent to %s", testRecipient())
		pause()
	})

	t.Run("ticket_claim_confirmation", func(t *testing.T) {
		ticketURL := frontend + "/claim-ticket?assignment=8b1e2c34-0000-4000-8000-000000000001&email=" + testRecipient()
		require.NoError(t, svc.SendTicketClaimConfirmation(
			testRecipient(),
			"Thandi Nkosi",
			previewEvent().Title,
			previewEvent().StartTime.Format("Monday, 2 January 2006 at 15:04"),
			previewEvent().VenueName,
			"BK-20260716-001",
			2,
			ticketURL,
		))
		t.Logf("ticket claim confirmation sent to %s", testRecipient())
		pause()
	})

	t.Run("ticket_invalidation_notice", func(t *testing.T) {
		require.NoError(t, svc.SendTicketInvalidationNotice(
			testRecipient(),
			"Thandi Nkosi",
			previewEvent().Title,
			previewEvent().StartTime.Format("Monday, 2 January 2006 at 15:04"),
			"BK-20260716-001",
		))
		t.Logf("ticket invalidation notice sent to %s", testRecipient())
		pause()
	})

	t.Run("organizer_handover_existing_account", func(t *testing.T) {
		resetLink := frontend + "/reset-password?token=550e8400-e29b-41d4-a716-446655440000"
		require.NoError(t, svc.SendOrganizerHandover(
			testRecipient(),
			"Maluk Events Co.",
			previewEvent().Title,
			resetLink,
			cfg.OrganizerDashboardURL,
			false,
		))
		t.Logf("organizer handover (existing account) sent to %s", testRecipient())
		pause()
	})

	t.Run("organizer_handover_new_account", func(t *testing.T) {
		resetLink := frontend + "/reset-password?token=550e8400-e29b-41d4-a716-446655440000"
		require.NoError(t, svc.SendOrganizerHandover(
			testRecipient(),
			"Maluk Events Co.",
			previewEvent().Title,
			resetLink,
			cfg.OrganizerDashboardURL,
			true,
		))
		t.Logf("organizer handover (new account / set password) sent to %s", testRecipient())
	})
}