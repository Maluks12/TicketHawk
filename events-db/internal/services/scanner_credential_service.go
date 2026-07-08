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

// ── Scanner credentials ───────────────────────────────────────────────────────
//
// The staff-side twin of the offline ticket token: an HMAC-signed blob in its
// own "scancred:" namespace authorizing {user, event, role, device} until the
// event's admission window ends. It lives in the scanner device's SecureStore
// and gates the scanner LOCALLY while offline.
//
// Deliberately NOT a ticket-verification key: the master QRHMACSecret never
// leaves the server. Offline ticket validity is manifest-authoritative (the
// device checks membership in the synced attendee list), so the device never
// needs to verify ticket signatures at all. Sync endpoints authenticate with
// the normal JWT + live canScan check — which is what makes staff revocation
// cut off a device's sync within one interval.

type ScannerCredentialPayload struct {
	UserID   string `json:"uid"`
	EventID  string `json:"eid"`
	Role     string `json:"role"` // "owner" | "manager" | "scanner" | "admin"
	DeviceID string `json:"did"`
	Exp      int64  `json:"exp"`
	Version  int    `json:"v"`
}

type ScannerCredentialService struct {
	cfg *config.Config
}

func NewScannerCredentialService(cfg *config.Config) *ScannerCredentialService {
	return &ScannerCredentialService{cfg: cfg}
}

// Mint creates a signed credential bound to (user, event, device) with an
// absolute expiry — callers set exp to the event's admission-window end so a
// lost phone's exposure self-limits to hours.
func (s *ScannerCredentialService) Mint(
	userID, eventID, role, deviceID string,
	exp time.Time,
) (string, error) {
	p := ScannerCredentialPayload{
		UserID: userID, EventID: eventID, Role: role,
		DeviceID: deviceID, Exp: exp.Unix(), Version: 1,
	}
	pj, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshal credential: %w", err)
	}
	b64 := base64.RawURLEncoding.EncodeToString(pj)
	return b64 + "." + s.sign(p), nil
}

// Verify parses and checks a credential (signature + expiry). The device uses
// only the embedded expiry locally; this server-side verifier exists for any
// future endpoint that accepts credential auth directly.
func (s *ScannerCredentialService) Verify(cred string) (*ScannerCredentialPayload, error) {
	dot := strings.LastIndex(cred, ".")
	if dot < 0 {
		return nil, fmt.Errorf("malformed credential")
	}
	pj, err := base64.RawURLEncoding.DecodeString(cred[:dot])
	if err != nil {
		return nil, fmt.Errorf("invalid credential encoding")
	}
	var p ScannerCredentialPayload
	if err := json.Unmarshal(pj, &p); err != nil {
		return nil, fmt.Errorf("invalid credential payload")
	}
	if time.Now().Unix() > p.Exp {
		return nil, fmt.Errorf("credential expired")
	}
	if !hmac.Equal([]byte(cred[dot+1:]), []byte(s.sign(p))) {
		return nil, fmt.Errorf("invalid credential signature")
	}
	return &p, nil
}

func (s *ScannerCredentialService) sign(p ScannerCredentialPayload) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.QRHMACSecret))
	mac.Write([]byte(fmt.Sprintf("scancred:%s:%s:%s:%d", p.UserID, p.EventID, p.DeviceID, p.Exp)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}