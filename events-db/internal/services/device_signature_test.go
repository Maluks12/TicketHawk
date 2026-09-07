package services

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// DEVICE SIGNATURE TESTS — Phase 4d gate
// =============================================================================
//
//	go test ./internal/services/... -v -run TestDeviceSignature
//
// The property: a till cannot disown a sale it signed, and nobody else can
// forge one in its name. Every field in the payload must be covered, or a
// vendor could edit that field in their queue before syncing and still verify.

func signPayload(t *testing.T, priv *rsa.PrivateKey, p OfflineTxnPayload) string {
	t.Helper()
	digest := sha256.Sum256(p.CanonicalBytes())
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, digest[:])
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(sig)
}

func samplePayload() OfflineTxnPayload {
	return OfflineTxnPayload{
		TxnID:        "11111111-1111-1111-1111-111111111111",
		EventID:      "22222222-2222-2222-2222-222222222222",
		TagUID:       "04A1B2C3D4E580",
		VendorID:     "33333333-3333-3333-3333-333333333333",
		DeviceID:     "44444444-4444-4444-4444-444444444444",
		AmountCents:  3750,
		DeviceSeq:    7,
		AccumBefore:  1000,
		AccumAfter:   4750,
		ApprovedAtMs: 1757246400000,
	}
}

func TestDeviceSignatureRoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubB64 := EncodePublicKeyB64(&priv.PublicKey)
	require.NotEmpty(t, pubB64)

	p := samplePayload()
	assert.NoError(t, VerifyOfflineTxnSignature(pubB64, signPayload(t, priv, p), p))
}

func TestDeviceSignatureCoversEveryField(t *testing.T) {
	// If any of these passes verification after being changed, that field is
	// not in the digest — and a vendor could edit it in their queue before
	// syncing while keeping a valid signature.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubB64 := EncodePublicKeyB64(&priv.PublicKey)

	base := samplePayload()
	sig := signPayload(t, priv, base)

	mutations := map[string]func(*OfflineTxnPayload){
		"txn id":       func(p *OfflineTxnPayload) { p.TxnID = "99999999-9999-9999-9999-999999999999" },
		"event id":     func(p *OfflineTxnPayload) { p.EventID = "99999999-9999-9999-9999-999999999999" },
		"tag uid":      func(p *OfflineTxnPayload) { p.TagUID = "04A1B2C3D4E581" },
		"vendor id":    func(p *OfflineTxnPayload) { p.VendorID = "99999999-9999-9999-9999-999999999999" },
		"device id":    func(p *OfflineTxnPayload) { p.DeviceID = "99999999-9999-9999-9999-999999999999" },
		"amount":       func(p *OfflineTxnPayload) { p.AmountCents = 100 },
		"device seq":   func(p *OfflineTxnPayload) { p.DeviceSeq = 8 },
		"accum before": func(p *OfflineTxnPayload) { p.AccumBefore = 0 },
		"accum after":  func(p *OfflineTxnPayload) { p.AccumAfter = 0 },
		"approved at":  func(p *OfflineTxnPayload) { p.ApprovedAtMs = 1757246400001 },
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			tampered := base
			mutate(&tampered)
			assert.ErrorIs(t, VerifyOfflineTxnSignature(pubB64, sig, tampered), ErrSignatureInvalid)
		})
	}
}

func TestDeviceSignatureFieldsCannotBeRearranged(t *testing.T) {
	// Length-prefixing exists so adjacent fields cannot be shifted across the
	// boundary between them and produce the same digest.
	a := samplePayload()
	a.VendorID, a.DeviceID = "ab", "cd"
	b := samplePayload()
	b.VendorID, b.DeviceID = "a", "bcd"
	assert.NotEqual(t, a.CanonicalBytes(), b.CanonicalBytes())
}

func TestDeviceSignatureWrongDeviceRejected(t *testing.T) {
	// The attribution property: one till's signature must not verify as
	// another's, or "which device rang this up" stops meaning anything.
	a, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	b, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	p := samplePayload()
	sig := signPayload(t, a, p)

	assert.ErrorIs(t, VerifyOfflineTxnSignature(EncodePublicKeyB64(&b.PublicKey), sig, p),
		ErrSignatureInvalid)
}

func TestDeviceSignatureMissingInputs(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubB64 := EncodePublicKeyB64(&priv.PublicKey)
	p := samplePayload()

	t.Run("unsigned transaction", func(t *testing.T) {
		// Distinguished from an INVALID signature on purpose: a missing one is
		// a client that never signed, an invalid one is tampering or the wrong
		// device. Ingestion treats them differently.
		assert.ErrorIs(t, VerifyOfflineTxnSignature(pubB64, "", p), ErrSignatureMissing)
		assert.ErrorIs(t, VerifyOfflineTxnSignature(pubB64, "   ", p), ErrSignatureMissing)
	})

	t.Run("device never enrolled", func(t *testing.T) {
		assert.ErrorIs(t, VerifyOfflineTxnSignature("", "c2ln", p), ErrDeviceKeyMissing)
	})

	t.Run("stored key is garbage", func(t *testing.T) {
		assert.ErrorIs(t, VerifyOfflineTxnSignature("not-base64!!", "c2ln", p), ErrDeviceKeyMissing)
	})

	t.Run("signature is not base64", func(t *testing.T) {
		assert.ErrorIs(t, VerifyOfflineTxnSignature(pubB64, "not base64!!", p), ErrSignatureInvalid)
	})
}

func TestDeviceSignatureKeyEncodingRoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	b64 := EncodePublicKeyB64(&priv.PublicKey)
	decoded, err := DecodePublicKeyB64(b64)
	require.NoError(t, err)

	rsaPub, ok := decoded.(*rsa.PublicKey)
	require.True(t, ok)
	assert.Equal(t, priv.PublicKey.N, rsaPub.N)
	assert.Equal(t, priv.PublicKey.E, rsaPub.E)
}