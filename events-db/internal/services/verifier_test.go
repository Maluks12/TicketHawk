package services

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// VERIFIER & KEY-WRAPPING TESTS
// =============================================================================
//
//	go test ./internal/services/... -v -run "TestIntegrityClaims|TestSecureKeyWrapper|TestChainDecode"
//
// SCOPE, stated plainly. These cover the parts that can be tested without
// Google and without an Android device: verdict normalisation, ASN.1
// self-consistency, and input handling. They do NOT prove that Google's API
// returns what normaliseIntegrityClaims expects, nor that Android will accept a
// SecureKeyWrapper this encoder produces. Both need real hardware — see the
// warning at the top of secure_key_import.go.

// ─── Play Integrity verdict normalisation ────────────────────────────────────

func integrityResp(appVerdict string, device []string, pkg, hash, ms string) *integrityResponse {
	r := &integrityResponse{}
	r.TokenPayloadExternal.AppIntegrity.AppRecognitionVerdict = appVerdict
	r.TokenPayloadExternal.AppIntegrity.PackageName = pkg
	r.TokenPayloadExternal.DeviceIntegrity.DeviceRecognitionVerdict = device
	r.TokenPayloadExternal.RequestDetails.RequestHash = hash
	r.TokenPayloadExternal.RequestDetails.RequestPackageName = pkg
	r.TokenPayloadExternal.RequestDetails.TimestampMillis = ms
	return r
}

func TestIntegrityClaimsEmptyVerdictListIsAFailure(t *testing.T) {
	// THE case worth pinning. An empty deviceRecognitionVerdict is how Google
	// reports a device that failed every check — rooted, emulated, or
	// unrecognised. Reading "field present" as a pass would hand an event key
	// to exactly the devices attestation exists to exclude.
	c := normaliseIntegrityClaims(integrityResp(
		"PLAY_RECOGNIZED", []string{}, "com.tickethawk.studio", "nonce", "1757246400000"))

	assert.True(t, c.AppRecognised)
	assert.False(t, c.DeviceIntegrity)
	assert.False(t, c.BasicIntegrity)

	// And EvaluateAttestation must refuse on it.
	hw := goodHardware(t)
	hw.Challenge = "nonce"
	c.Nonce = "nonce"
	c.IssuedAt = attestNow.Add(-time.Second)
	v, _ := EvaluateAttestation(goodPolicy(), "nonce", c, hw, attestNow)
	assert.Equal(t, AttestFailedIntegrity, v)
}

func TestIntegrityClaimsVerdictMapping(t *testing.T) {
	for name, tc := range map[string]struct {
		verdicts            []string
		wantDevice, wantBasic bool
	}{
		"basic only":   {[]string{"MEETS_BASIC_INTEGRITY"}, false, true},
		"device tier":  {[]string{"MEETS_DEVICE_INTEGRITY"}, true, true},
		"strong tier":  {[]string{"MEETS_STRONG_INTEGRITY"}, true, true},
		"both listed":  {[]string{"MEETS_BASIC_INTEGRITY", "MEETS_DEVICE_INTEGRITY"}, true, true},
		"unknown only": {[]string{"MEETS_VIRTUAL_INTEGRITY"}, false, false},
		"none":         {nil, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			c := normaliseIntegrityClaims(integrityResp(
				"PLAY_RECOGNIZED", tc.verdicts, "com.tickethawk.studio", "n", "1757246400000"))
			assert.Equal(t, tc.wantDevice, c.DeviceIntegrity)
			assert.Equal(t, tc.wantBasic, c.BasicIntegrity)
		})
	}
}

func TestIntegrityClaimsAppRecognition(t *testing.T) {
	// Anything other than PLAY_RECOGNIZED is a modified or sideloaded APK.
	for _, v := range []string{"UNRECOGNIZED_VERSION", "UNEVALUATED", ""} {
		c := normaliseIntegrityClaims(integrityResp(
			v, []string{"MEETS_DEVICE_INTEGRITY"}, "com.tickethawk.studio", "n", "1757246400000"))
		assert.False(t, c.AppRecognised, v)
	}
}

func TestIntegrityClaimsMissingTimestampIsStale(t *testing.T) {
	// A zero IssuedAt must read as stale, not as "no expiry to check".
	for _, ms := range []string{"", "0", "not-a-number", "-5"} {
		c := normaliseIntegrityClaims(integrityResp(
			"PLAY_RECOGNIZED", []string{"MEETS_DEVICE_INTEGRITY"}, "com.tickethawk.studio", "n", ms))
		assert.True(t, c.IssuedAt.IsZero(), "ms=%q", ms)

		hw := goodHardware(t)
		hw.Challenge = "n"
		c.Nonce = "n"
		v, _ := EvaluateAttestation(goodPolicy(), "n", c, hw, attestNow)
		assert.Equal(t, AttestFailedStale, v, "ms=%q", ms)
	}
}

func TestIntegrityClaimsNonceIsCarriedFromRequestHash(t *testing.T) {
	c := normaliseIntegrityClaims(integrityResp(
		"PLAY_RECOGNIZED", []string{"MEETS_DEVICE_INTEGRITY"},
		"com.tickethawk.studio", testNonce, "1757246400000"))
	assert.Equal(t, testNonce, c.Nonce,
		"the nonce lives in requestHash — comparison belongs to EvaluateAttestation")
}

// ─── SECURE_KEY_IMPORT ───────────────────────────────────────────────────────

func TestSecureKeyWrapperRoundTrip(t *testing.T) {
	// Proves the encoder is SELF-consistent. It does not prove Android accepts
	// the blob — only a physical import does that.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	for _, size := range []int{16, 24, 32} {
		key := make([]byte, size)
		_, err := rand.Read(key)
		require.NoError(t, err)

		wrapped, err := BuildSecureKeyWrapper(&priv.PublicKey, key, nil)
		require.NoError(t, err, "size %d", size)

		got, err := UnwrapSecureKeyWrapperForTest(priv, wrapped, nil)
		require.NoError(t, err, "size %d", size)
		assert.Equal(t, key, got, "size %d", size)
	}
}

func TestSecureKeyWrapperWithMaskingKey(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	key := make([]byte, 16)
	_, _ = rand.Read(key)
	mask := []byte{0xAA, 0x55, 0xF0, 0x0F}

	wrapped, err := BuildSecureKeyWrapper(&priv.PublicKey, key, mask)
	require.NoError(t, err)

	got, err := UnwrapSecureKeyWrapperForTest(priv, wrapped, mask)
	require.NoError(t, err)
	assert.Equal(t, key, got)

	t.Run("wrong mask yields wrong key, not an error", func(t *testing.T) {
		// Worth knowing: the GCM tag covers the MASKED bytes, so a mask
		// mismatch is silent. If Android ever rejects an import for an opaque
		// reason, a mask disagreement is a candidate.
		bad, err := UnwrapSecureKeyWrapperForTest(priv, wrapped, []byte{0x01})
		require.NoError(t, err)
		assert.NotEqual(t, key, bad)
	})
}

func TestSecureKeyWrapperIsRandomised(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	key := make([]byte, 16)
	_, _ = rand.Read(key)

	a, err := BuildSecureKeyWrapper(&priv.PublicKey, key, nil)
	require.NoError(t, err)
	b, err := BuildSecureKeyWrapper(&priv.PublicKey, key, nil)
	require.NoError(t, err)

	// Fresh transport key and IV each time. A deterministic wrapper would leak
	// which devices hold the same event key.
	assert.NotEqual(t, a, b)
}

func TestSecureKeyWrapperTamperDetection(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	key := make([]byte, 16)
	_, _ = rand.Read(key)

	wrapped, err := BuildSecureKeyWrapper(&priv.PublicKey, key, nil)
	require.NoError(t, err)

	der, err := base64.StdEncoding.DecodeString(wrapped)
	require.NoError(t, err)

	// Flip a byte in the back half, which lands in the encrypted key material
	// or its tag. GCM must refuse rather than return garbage — this is what
	// stops a device importing our key under altered parameters.
	der[len(der)-8] ^= 0xFF
	_, err = UnwrapSecureKeyWrapperForTest(priv, base64.StdEncoding.EncodeToString(der), nil)
	assert.Error(t, err)
}

func TestSecureKeyWrapperRejectsBadInput(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	_, err = BuildSecureKeyWrapper(nil, make([]byte, 16), nil)
	assert.Error(t, err)

	for _, size := range []int{0, 1, 15, 17, 33} {
		_, err := BuildSecureKeyWrapper(&priv.PublicKey, make([]byte, size), nil)
		assert.ErrorIs(t, err, ErrWrapUnsupportedKeySize, "size %d", size)
	}
}

func TestSecureKeyWrapperNeedsTheRightPrivateKey(t *testing.T) {
	a, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	b, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	key := make([]byte, 16)
	_, _ = rand.Read(key)
	wrapped, err := BuildSecureKeyWrapper(&a.PublicKey, key, nil)
	require.NoError(t, err)

	// The containment property: a blob wrapped for one device is useless to
	// another, so a stolen key grant does not travel.
	_, err = UnwrapSecureKeyWrapperForTest(b, wrapped, nil)
	assert.Error(t, err)
}

// ─── Chain decoding ──────────────────────────────────────────────────────────

func TestChainDecodeB64(t *testing.T) {
	der := []byte{0x30, 0x82, 0x01, 0x02}
	out, err := DecodeChainB64([]string{base64.StdEncoding.EncodeToString(der)})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, der, out[0])

	_, err = DecodeChainB64(nil)
	assert.Error(t, err)

	_, err = DecodeChainB64([]string{"not base64!!"})
	assert.Error(t, err)
}

func TestKeystoreVerifierRefusesWithoutPinnedRoots(t *testing.T) {
	// An unpinned verifier accepts a chain rooted in anything, including a
	// self-signed root an attacker generates in a second. That is strictly
	// worse than no verification, because it looks like security.
	_, err := NewKeystoreVerifier("")
	assert.ErrorIs(t, err, ErrAttestationNotConfigured)

	_, err = NewKeystoreVerifier("/nonexistent/roots.pem")
	assert.Error(t, err)
}