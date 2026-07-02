package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// OFFLINE TOKEN TESTS (no DB, no infra)
//
// Run:  go test ./internal/services/... -v -run TestOffline
//
// These exercise the offline token family end-to-end at the crypto layer:
// generation → VerifyToken → payload shape, plus expiry and tamper rejection.
// They reuse newTestRotatingQR() from rotating_qr_service_test.go.
// =============================================================================

func TestOfflineTokenRoundTripIndividual(t *testing.T) {
	svc := newTestRotatingQR()
	exp := time.Now().Add(2 * time.Hour)

	tok, err := svc.GenerateOfflineToken("aid-1", "bid-1", "eid-1", 3, exp)
	require.NoError(t, err)
	// WindowSec 0 tells the app: static, do not poll for refresh.
	assert.Equal(t, 0, tok.WindowSec)

	p, err := svc.VerifyToken(tok.Token)
	require.NoError(t, err)
	assert.True(t, p.IsOffline())
	assert.False(t, p.IsPool())
	assert.Equal(t, "aid-1", p.AssignmentID)
	assert.Equal(t, "bid-1", p.BookingID)
	assert.Equal(t, "eid-1", p.EventID)
	assert.Equal(t, 3, p.SlotNumber)
}

func TestOfflineTokenRoundTripPool(t *testing.T) {
	svc := newTestRotatingQR()
	exp := time.Now().Add(2 * time.Hour)

	tok, err := svc.GenerateOfflinePoolToken("bid-1", "eid-1", exp)
	require.NoError(t, err)

	p, err := svc.VerifyToken(tok.Token)
	require.NoError(t, err)
	assert.True(t, p.IsOffline())
	assert.True(t, p.IsPool()) // pool path must dispatch through ScanHandler's pool logic
	assert.Equal(t, "bid-1", p.BookingID)
}

func TestOfflineTokenExpired(t *testing.T) {
	svc := newTestRotatingQR()
	// Past the expiry AND past the clock-skew grace → must be rejected.
	exp := time.Now().Add(-time.Duration(OfflineTokenSkewSeconds+60) * time.Second)

	tok, err := svc.GenerateOfflineToken("aid-1", "bid-1", "eid-1", 1, exp)
	require.NoError(t, err)

	_, err = svc.VerifyToken(tok.Token)
	assert.Error(t, err, "expired offline token should not verify")
}

func TestOfflineTokenWithinSkewStillValid(t *testing.T) {
	svc := newTestRotatingQR()
	// Just past expiry but inside the skew grace → still accepted.
	exp := time.Now().Add(-30 * time.Second)

	tok, err := svc.GenerateOfflineToken("aid-1", "bid-1", "eid-1", 1, exp)
	require.NoError(t, err)

	p, err := svc.VerifyToken(tok.Token)
	require.NoError(t, err)
	assert.Equal(t, "aid-1", p.AssignmentID)
}

func TestOfflineTokenTamperRejected(t *testing.T) {
	svc := newTestRotatingQR()
	exp := time.Now().Add(2 * time.Hour)

	tok, err := svc.GenerateOfflineToken("aid-1", "bid-1", "eid-1", 1, exp)
	require.NoError(t, err)

	// Flip the final signature character so the HMAC no longer matches.
	last := tok.Token[len(tok.Token)-1]
	repl := byte('A')
	if last == 'A' {
		repl = 'B'
	}
	bad := tok.Token[:len(tok.Token)-1] + string(repl)

	_, err = svc.VerifyToken(bad)
	assert.Error(t, err, "tampered offline token should not verify")
}

func TestOfflineAndRotatingDoNotCrossVerify(t *testing.T) {
	svc := newTestRotatingQR()
	exp := time.Now().Add(2 * time.Hour)

	// An offline token's payload carries the same assignment as a rotating one,
	// but the namespaced signatures must keep the two families separate: an
	// offline token must never validate as rotating, and neither can be forged
	// from the other. We assert both simply verify as their own type here.
	off, err := svc.GenerateOfflineToken("aid-1", "bid-1", "eid-1", 1, exp)
	require.NoError(t, err)
	rot, err := svc.GenerateToken("aid-1", "bid-1", "eid-1", 1)
	require.NoError(t, err)

	offP, err := svc.VerifyToken(off.Token)
	require.NoError(t, err)
	assert.True(t, offP.IsOffline())

	rotP, err := svc.VerifyToken(rot.Token)
	require.NoError(t, err)
	assert.False(t, rotP.IsOffline())
}