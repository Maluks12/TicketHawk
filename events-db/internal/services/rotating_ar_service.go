package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"events-db/internal/config"
)

// QRWindowSeconds is the size of each time window in seconds.
// Every 30 seconds the token changes, so a screenshotted QR is dead within a minute.
const QRWindowSeconds = 30

// qrWindowTolerance is how many previous windows we still accept.
// 1 means we accept the current window and the one before it (~60s total validity).
// This gives the app enough time to display the code before it expires.
const qrWindowTolerance = 1

// RotatingQRService generates and verifies HMAC-signed time-windowed QR tokens.
// Tokens are purely computational — no DB writes on generation, no stored secrets per-ticket.
// The only state needed for verification is the shared HMAC secret in config.
type RotatingQRService struct {
	cfg *config.Config
}

func NewRotatingQRService(cfg *config.Config) *RotatingQRService {
	return &RotatingQRService{cfg: cfg}
}

// RotatingQRPayload is the data embedded inside the QR code.
// Short JSON keys keep the QR code data density low so it scans faster.
type RotatingQRPayload struct {
	AssignmentID string `json:"aid"` // ticket_assignments.id
	BookingID    string `json:"bid"` // bookings.id
	EventID      string `json:"eid"` // events.id
	SlotNumber   int    `json:"sn"`  // seat/slot number
	Window       int64  `json:"w"`   // unix_time / QRWindowSeconds
	Version      int    `json:"v"`   // token version for future migrations
}

// RotatingQRToken is returned to the app. The app renders Token as a QR code
// and refreshes when ExpiresAt is reached (or every ~25s for smooth UX).
type RotatingQRToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	WindowSec int       `json:"window_sec"`
	IssuedAt  time.Time `json:"issued_at"`
}

// GenerateToken creates a fresh HMAC-signed token for the given assignment.
// Call this every time the app polls for a new QR code.
func (s *RotatingQRService) GenerateToken(
	assignmentID, bookingID, eventID string,
	slotNumber int,
) (*RotatingQRToken, error) {
	w := currentWindow()

	payload := RotatingQRPayload{
		AssignmentID: assignmentID,
		BookingID:    bookingID,
		EventID:      eventID,
		SlotNumber:   slotNumber,
		Window:       w,
		Version:      1,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := s.sign(assignmentID, w)
	token := payloadB64 + "." + sig

	now := time.Now()
	return &RotatingQRToken{
		Token:     token,
		ExpiresAt: windowExpiry(w),
		WindowSec: QRWindowSeconds,
		IssuedAt:  now,
	}, nil
}

// VerifyToken parses a rotating QR token and returns the embedded payload if valid.
//
// Verification steps:
//  1. Split token into payload and HMAC signature.
//  2. Decode and unmarshal the payload.
//  3. Check that the embedded window is current or at most qrWindowTolerance windows old.
//  4. Recompute HMAC and compare in constant time to prevent timing attacks.
//
// This method does NOT check the DB — the caller (handler) must verify booking/assignment
// status after calling this. Keeping crypto and DB concerns separate makes testing easier.
func (s *RotatingQRService) VerifyToken(token string) (*RotatingQRPayload, error) {
	// Find the last dot — everything before it is the payload, after is the signature.
	dotIdx := strings.LastIndex(token, ".")
	if dotIdx < 0 {
		return nil, fmt.Errorf("malformed token: missing signature separator")
	}

	payloadB64 := token[:dotIdx]
	gotSig := token[dotIdx+1:]

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("invalid token encoding")
	}

	var payload RotatingQRPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, fmt.Errorf("invalid token payload")
	}

	// ── Check time window ────────────────────────────────────────────────────
	now := currentWindow()
	age := now - payload.Window
	if age < 0 || age > qrWindowTolerance {
		// age < 0 means the token is from the future (clock skew / replay from another server)
		// age > tolerance means it's too old
		return nil, fmt.Errorf("token expired (window age: %d)", age)
	}

	// ── Verify HMAC in constant time ─────────────────────────────────────────
	expectedSig := s.sign(payload.AssignmentID, payload.Window)
	if !hmac.Equal([]byte(gotSig), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid token signature")
	}

	return &payload, nil
}

// sign computes the HMAC-SHA256 for a given assignment ID + time window.
// The input binds both the identity (assignment) and time (window) to the signature,
// so tokens from different windows or different assignments can't be swapped.
func (s *RotatingQRService) sign(assignmentID string, window int64) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.QRHMACSecret))
	mac.Write([]byte(fmt.Sprintf("%s:%d", assignmentID, window)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// currentWindow returns floor(now / QRWindowSeconds).
func currentWindow() int64 {
	return time.Now().Unix() / QRWindowSeconds
}

// windowExpiry returns the time at which a given window ends.
func windowExpiry(window int64) time.Time {
	return time.Unix((window+1)*QRWindowSeconds, 0)
}