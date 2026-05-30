package services

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"events-db/internal/config"
)

func newTestRotatingQR() *RotatingQRService {
	return NewRotatingQRService(&config.Config{
		QRHMACSecret: "test-secret-must-be-at-least-32-chars-long-padding",
	})
}

const (
	testAssignmentID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	testBookingID    = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	testEventID      = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

// TestGenerateAndVerify is the happy-path end-to-end.
func TestGenerateAndVerify(t *testing.T) {
	svc := newTestRotatingQR()

	tok, err := svc.GenerateToken(testAssignmentID, testBookingID, testEventID, 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if tok.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if tok.WindowSec != QRWindowSeconds {
		t.Errorf("WindowSec = %d, want %d", tok.WindowSec, QRWindowSeconds)
	}
	if time.Until(tok.ExpiresAt) <= 0 {
		t.Error("ExpiresAt is in the past")
	}

	payload, err := svc.VerifyToken(tok.Token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if payload.AssignmentID != testAssignmentID {
		t.Errorf("AssignmentID = %q, want %q", payload.AssignmentID, testAssignmentID)
	}
	if payload.SlotNumber != 1 {
		t.Errorf("SlotNumber = %d, want 1", payload.SlotNumber)
	}
	if payload.Version != 1 {
		t.Errorf("Version = %d, want 1", payload.Version)
	}
}

// TestTokenFormat checks the token is two dot-separated base64url segments.
func TestTokenFormat(t *testing.T) {
	svc := newTestRotatingQR()
	tok, _ := svc.GenerateToken(testAssignmentID, testBookingID, testEventID, 2)

	parts := strings.Split(tok.Token, ".")
	if len(parts) != 2 {
		t.Fatalf("expected 2 dot-separated parts, got %d (token: %q)", len(parts), tok.Token)
	}
	for _, p := range parts {
		if p == "" {
			t.Fatal("token contains an empty segment")
		}
	}
}

// TestTamperedPayloadRejected verifies a modified payload is rejected.
func TestTamperedPayloadRejected(t *testing.T) {
	svc := newTestRotatingQR()
	tok, _ := svc.GenerateToken(testAssignmentID, testBookingID, testEventID, 1)

	dot := strings.LastIndex(tok.Token, ".")
	// Replace payload segment with different base64url data
	tampered := "dGFtcGVyZWQ" + tok.Token[dot:]

	_, err := svc.VerifyToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered payload, got nil")
	}
}

// TestTamperedSignatureRejected verifies a replaced signature is detected.
func TestTamperedSignatureRejected(t *testing.T) {
	svc := newTestRotatingQR()
	tok, _ := svc.GenerateToken(testAssignmentID, testBookingID, testEventID, 1)

	dot := strings.LastIndex(tok.Token, ".")
	tampered := tok.Token[:dot+1] + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	_, err := svc.VerifyToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered signature, got nil")
	}
}

// TestWrongSecretRejected verifies a token from one secret fails verification with another.
func TestWrongSecretRejected(t *testing.T) {
	svc1 := NewRotatingQRService(&config.Config{QRHMACSecret: "secret-one-must-be-at-least-32-chars-padding"})
	svc2 := NewRotatingQRService(&config.Config{QRHMACSecret: "secret-two-must-be-at-least-32-chars-padding"})

	tok, _ := svc1.GenerateToken(testAssignmentID, testBookingID, testEventID, 1)
	_, err := svc2.VerifyToken(tok.Token)
	if err == nil {
		t.Fatal("expected error when verifying with wrong secret")
	}
}

// buildTokenWithWindow constructs a valid-HMAC token for the given window.
// Used to test time-window boundary logic independently of wall clock.
func buildTokenWithWindow(svc *RotatingQRService, window int64) string {
	p := RotatingQRPayload{
		AssignmentID: testAssignmentID,
		BookingID:    testBookingID,
		EventID:      testEventID,
		SlotNumber:   1,
		Window:       window,
		Version:      1,
	}
	data, _ := json.Marshal(p)
	payloadB64 := base64.RawURLEncoding.EncodeToString(data)
	sig := svc.sign(testAssignmentID, window)
	return payloadB64 + "." + sig
}

// TestCurrentWindowAccepted confirms a token from the current window is valid.
func TestCurrentWindowAccepted(t *testing.T) {
	svc := newTestRotatingQR()
	token := buildTokenWithWindow(svc, currentWindow())
	_, err := svc.VerifyToken(token)
	if err != nil {
		t.Errorf("current window should be accepted, got: %v", err)
	}
}

// TestPreviousWindowAccepted confirms the immediately previous window is still valid
// (allows up to ~60s for the app to refresh before being rejected).
func TestPreviousWindowAccepted(t *testing.T) {
	svc := newTestRotatingQR()
	token := buildTokenWithWindow(svc, currentWindow()-1)
	_, err := svc.VerifyToken(token)
	if err != nil {
		t.Errorf("previous window should be accepted (tolerance=%d), got: %v", qrWindowTolerance, err)
	}
}

// TestExpiredWindowRejected confirms a token older than the tolerance is rejected.
func TestExpiredWindowRejected(t *testing.T) {
	svc := newTestRotatingQR()
	staleWindow := currentWindow() - (qrWindowTolerance + 2)
	token := buildTokenWithWindow(svc, staleWindow)
	_, err := svc.VerifyToken(token)
	if err == nil {
		t.Fatal("stale window should be rejected, got nil error")
	}
}

// TestFutureWindowRejected confirms a token from the future is rejected (prevents
// pre-generated tokens from being stockpiled for later use).
func TestFutureWindowRejected(t *testing.T) {
	svc := newTestRotatingQR()
	futureWindow := currentWindow() + 5
	token := buildTokenWithWindow(svc, futureWindow)
	_, err := svc.VerifyToken(token)
	if err == nil {
		t.Fatal("future window should be rejected, got nil error")
	}
}

// TestMalformedTokensRejected checks garbage inputs don't panic or succeed.
func TestMalformedTokensRejected(t *testing.T) {
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
			if err == nil {
				t.Errorf("expected error for token %q, got nil", tc.token)
			}
		})
	}
}

// TestDifferentAssignmentsProduceDifferentTokens confirms assignment IDs are bound to their tokens.
func TestDifferentAssignmentsProduceDifferentTokens(t *testing.T) {
	svc := newTestRotatingQR()
	w := currentWindow()

	tok1 := buildTokenWithWindow(svc, w)

	svc2assignment := NewRotatingQRService(&config.Config{QRHMACSecret: "test-secret-must-be-at-least-32-chars-long-padding"})
	// Manually sign a token for a different assignment using the same service
	p := RotatingQRPayload{
		AssignmentID: "dddddddd-dddd-dddd-dddd-dddddddddddd",
		BookingID:    testBookingID,
		EventID:      testEventID,
		SlotNumber:   2,
		Window:       w,
		Version:      1,
	}
	data, _ := json.Marshal(p)
	payloadB64 := base64.RawURLEncoding.EncodeToString(data)
	sig := svc2assignment.sign("dddddddd-dddd-dddd-dddd-dddddddddddd", w)
	tok2 := payloadB64 + "." + sig

	if tok1 == tok2 {
		t.Error("different assignment IDs produced identical tokens")
	}

	// Cross-verify: tok1's signature should NOT work for assignment2's payload
	dot := strings.LastIndex(tok1, ".")
	crossToken := tok2[:strings.LastIndex(tok2, ".")+1] + tok1[dot+1:]
	_, err := svc.VerifyToken(crossToken)
	if err == nil {
		t.Error("swapped signature should not verify — assignment ID must be bound to HMAC")
	}
}