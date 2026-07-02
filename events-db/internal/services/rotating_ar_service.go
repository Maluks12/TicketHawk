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

// OfflineTokenSkewSeconds is the grace period we add on top of an offline
// token's embedded expiry to absorb device/server clock drift at the gate.
// The expiry itself is already set to the event's admission window end
// (event end + 2h), so this is only slop, not a real extension.
const OfflineTokenSkewSeconds = 300 // 5 minutes

// RotatingQRService generates and verifies HMAC-signed QR tokens.
//
// It mints two families of token from the same shared secret:
//
//   - Rotating tokens: bound to a 30-second time window. The app polls for a
//     fresh one every ~28s. Screenshot-proof, but requires the holder to be
//     online. Used for the in-app live QR.
//   - Offline tokens: bound to an absolute expiry (the event admission window)
//     instead of a rotating window. Self-contained, so the app can render them
//     with no network — the fallback shown when the holder has no signal, and
//     the payload carried inside Apple/Google Wallet passes later on.
//
// Both are purely computational — no DB writes on generation, no per-ticket
// secrets. The only state needed for verification is the shared HMAC secret.
type RotatingQRService struct {
	cfg *config.Config
}

func NewRotatingQRService(cfg *config.Config) *RotatingQRService {
	return &RotatingQRService{cfg: cfg}
}

// RotatingQRPayload is the data embedded inside the QR code.
// Short JSON keys keep the QR code data density low so it scans faster.
type RotatingQRPayload struct {
	// Type distinguishes an individual ticket token ("" / "i") from a group
	// pool token ("p"). A pool token admits one-or-more heads from a booking's
	// unassigned pool; an individual token admits one specific assignment.
	Type         string `json:"t,omitempty"`
	AssignmentID string `json:"aid,omitempty"` // ticket_assignments.id (individual tokens)
	BookingID    string `json:"bid"`           // bookings.id
	EventID      string `json:"eid"`           // events.id
	SlotNumber   int    `json:"sn,omitempty"`  // seat/slot number (individual tokens)

	// Window is set on ROTATING tokens only: unix_time / QRWindowSeconds.
	Window int64 `json:"w,omitempty"`

	// Off marks an OFFLINE token. When true, verification checks Exp instead of
	// the rotating window, and the signature comes from the offline namespace so
	// an offline token can never be verified as (or forged from) a rotating one.
	Off bool `json:"off,omitempty"`
	// Exp is the absolute unix expiry for OFFLINE tokens (event admission end).
	Exp int64 `json:"exp,omitempty"`

	Version int `json:"v"` // token version for future migrations
}

// TokenTypePool marks a group pool token. Anything else (incl. empty) is treated
// as an individual ticket token, preserving backward compatibility.
const TokenTypePool = "p"

// IsPool reports whether the payload is a group pool token.
func (p *RotatingQRPayload) IsPool() bool { return p.Type == TokenTypePool }

// IsOffline reports whether the payload is a self-contained offline token.
func (p *RotatingQRPayload) IsOffline() bool { return p.Off }

// RotatingQRToken is returned to the app. The app renders Token as a QR code.
// For rotating tokens it refreshes when ExpiresAt is reached (WindowSec > 0);
// for offline tokens WindowSec is 0 and ExpiresAt is the event admission end.
type RotatingQRToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	WindowSec int       `json:"window_sec"`
	IssuedAt  time.Time `json:"issued_at"`
}

// GenerateToken creates a fresh HMAC-signed rotating token for the given
// assignment. Call this every time the app polls for a new QR code.
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

// GeneratePoolToken creates a fresh HMAC-signed GROUP token for a booking's
// unassigned pool. It carries the booking (not a specific assignment); the
// scanner resolves "how many remain" from the DB and admits a chosen headcount.
// The signature input is namespaced ("pool:…") so a pool token can never be
// mistaken for, or forged from, an individual ticket token.
func (s *RotatingQRService) GeneratePoolToken(
	bookingID, eventID string,
) (*RotatingQRToken, error) {
	w := currentWindow()

	payload := RotatingQRPayload{
		Type:      TokenTypePool,
		BookingID: bookingID,
		EventID:   eventID,
		Window:    w,
		Version:   1,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal pool payload: %w", err)
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := s.signPool(bookingID, w)
	token := payloadB64 + "." + sig

	now := time.Now()
	return &RotatingQRToken{
		Token:     token,
		ExpiresAt: windowExpiry(w),
		WindowSec: QRWindowSeconds,
		IssuedAt:  now,
	}, nil
}

// GenerateOfflineToken mints a self-contained INDIVIDUAL token that verifies
// without a live network round-trip on the holder's side. It is bound to an
// absolute expiry (typically the event admission window end) rather than a
// rotating window, so a phone with no signal can still render a valid QR that
// the (online) scanner admits through the exact same code path as a rotating
// token — VerifyToken returns the same payload shape.
//
// Trade-off: an offline token is static and therefore screenshot-shareable.
// While the scanner is online this is fully contained by the single-entry DB
// guard; it only becomes a real exposure once BOTH sides go offline.
func (s *RotatingQRService) GenerateOfflineToken(
	assignmentID, bookingID, eventID string,
	slotNumber int,
	exp time.Time,
) (*RotatingQRToken, error) {
	e := exp.Unix()

	payload := RotatingQRPayload{
		AssignmentID: assignmentID,
		BookingID:    bookingID,
		EventID:      eventID,
		SlotNumber:   slotNumber,
		Off:          true,
		Exp:          e,
		Version:      1,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal offline payload: %w", err)
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := s.signOffline(assignmentID, e)
	token := payloadB64 + "." + sig

	return &RotatingQRToken{
		Token:     token,
		ExpiresAt: exp,
		WindowSec: 0, // 0 signals to the app: static, do not poll for refresh
		IssuedAt:  time.Now(),
	}, nil
}

// GenerateOfflinePoolToken mints a self-contained GROUP token for a booking's
// pool. Like the online pool token it carries the booking rather than a slot;
// the scanner still resolves the live remaining count from the DB, so even a
// replayed offline pool token cannot over-admit while the scanner is online.
func (s *RotatingQRService) GenerateOfflinePoolToken(
	bookingID, eventID string,
	exp time.Time,
) (*RotatingQRToken, error) {
	e := exp.Unix()

	payload := RotatingQRPayload{
		Type:      TokenTypePool,
		BookingID: bookingID,
		EventID:   eventID,
		Off:       true,
		Exp:       e,
		Version:   1,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal offline pool payload: %w", err)
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := s.signOfflinePool(bookingID, e)
	token := payloadB64 + "." + sig

	return &RotatingQRToken{
		Token:     token,
		ExpiresAt: exp,
		WindowSec: 0,
		IssuedAt:  time.Now(),
	}, nil
}

// VerifyToken parses a QR token (rotating, pool, or offline) and returns the
// embedded payload if valid.
//
// Verification steps:
//  1. Split token into payload and HMAC signature.
//  2. Decode and unmarshal the payload.
//  3. Validity check — offline tokens are checked against their absolute Exp;
//     rotating/pool tokens against the current time window.
//  4. Recompute HMAC in the matching namespace and compare in constant time.
//
// This method does NOT touch the DB — the caller (handler) verifies
// booking/assignment status afterwards. Keeping crypto and DB concerns separate
// makes testing easier and lets the same call serve every token family.
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

	// ── Offline tokens: absolute-expiry path ──────────────────────────────────
	// Pool and individual offline tokens sign different, namespaced inputs, so
	// the signature itself enforces the type — a token can't be cross-cast, and
	// an offline token can never validate as a rotating one (different secret
	// namespace) or vice-versa.
	if payload.Off {
		now := time.Now().Unix()
		if payload.Exp == 0 || now > payload.Exp+OfflineTokenSkewSeconds {
			return nil, fmt.Errorf("offline ticket expired")
		}

		var expectedSig string
		if payload.IsPool() {
			expectedSig = s.signOfflinePool(payload.BookingID, payload.Exp)
		} else {
			expectedSig = s.signOffline(payload.AssignmentID, payload.Exp)
		}
		if !hmac.Equal([]byte(gotSig), []byte(expectedSig)) {
			return nil, fmt.Errorf("invalid token signature")
		}
		return &payload, nil
	}

	// ── Rotating / pool tokens: time-window path ──────────────────────────────
	now := currentWindow()
	age := now - payload.Window
	// age > qrWindowTolerance → token is too old (expired)
	// age < -qrWindowTolerance → token is too far in the future (clock skew / replay)
	if age > qrWindowTolerance || age < -qrWindowTolerance {
		return nil, fmt.Errorf("token expired (window age: %d)", age)
	}

	var expectedSig string
	if payload.IsPool() {
		expectedSig = s.signPool(payload.BookingID, payload.Window)
	} else {
		expectedSig = s.sign(payload.AssignmentID, payload.Window)
	}
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

// signPool computes the HMAC for a group pool token. The "pool:" namespace keeps
// it in a separate signature space from individual tokens, so the two can never
// collide or be forged from one another even at the same window.
func (s *RotatingQRService) signPool(bookingID string, window int64) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.QRHMACSecret))
	mac.Write([]byte(fmt.Sprintf("pool:%s:%d", bookingID, window)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// signOffline computes the HMAC for an individual offline token. The "off:"
// namespace binds the signature to the assignment and its absolute expiry and
// keeps it fully separate from rotating and pool signature spaces.
func (s *RotatingQRService) signOffline(assignmentID string, exp int64) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.QRHMACSecret))
	mac.Write([]byte(fmt.Sprintf("off:%s:%d", assignmentID, exp)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// signOfflinePool computes the HMAC for a group offline token, in its own
// "offpool:" namespace so it can't be confused with any other token family.
func (s *RotatingQRService) signOfflinePool(bookingID string, exp int64) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.QRHMACSecret))
	mac.Write([]byte(fmt.Sprintf("offpool:%s:%d", bookingID, exp)))
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