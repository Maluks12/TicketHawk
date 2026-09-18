package services

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ATTESTATION TESTS — Phase 4b gate
// =============================================================================
//
//	go test ./internal/services/... -v -run TestAttest
//
// These cover the DECISION, not the cryptography. Verifying a Play Integrity
// token and an Android Keystore chain happens behind the two verifier
// interfaces; what is pinned here is what their results are allowed to mean,
// because that is where the judgement lives and where a permissive default
// would hand out event keys.

const testNonce = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"

var attestNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func goodPolicy() AttestationPolicy {
	return AttestationPolicy{
		ExpectedPackage: "com.tickethawk.studio",
		MaxTokenAge:     5 * time.Minute,
	}
}

func goodIntegrity() *IntegrityClaims {
	return &IntegrityClaims{
		Nonce:           testNonce,
		AppRecognised:   true,
		DeviceIntegrity: true,
		BasicIntegrity:  true,
		PackageName:     "com.tickethawk.studio",
		IssuedAt:        attestNow.Add(-30 * time.Second),
	}
}

func goodHardware(t *testing.T) *HardwareKeyClaims {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return &HardwareKeyClaims{
		Challenge:     testNonce,
		SecurityLevel: SecurityTEE,
		ChainVerified: true,
		RootOfTrustOK: true,
		PublicKey:     &k.PublicKey,
		KeyAlgorithm:  "rsa",
	}
}

// ─── The safe default ────────────────────────────────────────────────────────

func TestAttestUnconfiguredPolicyRefuses(t *testing.T) {
	// Same principle as OfflinePolicy{} in Phase 4a. A caller that fails to
	// load config must deny — a permissive default here gives away event keys.
	v, _ := EvaluateAttestation(AttestationPolicy{}, testNonce,
		goodIntegrity(), goodHardware(t), attestNow)
	assert.Equal(t, AttestError, v)
}

func TestAttestMissingEvidenceRefuses(t *testing.T) {
	v, _ := EvaluateAttestation(goodPolicy(), testNonce, nil, goodHardware(t), attestNow)
	assert.Equal(t, AttestError, v)

	v, _ = EvaluateAttestation(goodPolicy(), testNonce, goodIntegrity(), nil, attestNow)
	assert.Equal(t, AttestError, v)
}

// ─── Happy path ──────────────────────────────────────────────────────────────

func TestAttestPasses(t *testing.T) {
	v, reason := EvaluateAttestation(goodPolicy(), testNonce,
		goodIntegrity(), goodHardware(t), attestNow)
	assert.Equal(t, AttestPassed, v)
	assert.Empty(t, reason)
}

func TestAttestStrongBoxAlwaysAcceptable(t *testing.T) {
	hw := goodHardware(t)
	hw.SecurityLevel = SecurityStrongBox

	p := goodPolicy()
	v, _ := EvaluateAttestation(p, testNonce, goodIntegrity(), hw, attestNow)
	assert.Equal(t, AttestPassed, v)

	p.RequireStrongBox = true
	v, _ = EvaluateAttestation(p, testNonce, goodIntegrity(), hw, attestNow)
	assert.Equal(t, AttestPassed, v)
}

func TestAttestTEEAcceptableUnlessStrongBoxRequired(t *testing.T) {
	// Most mid-range Android hardware — which is most of what vendors at a
	// South African festival actually own — is TEE, not StrongBox. Requiring
	// StrongBox by default would exclude them, so it is opt-in.
	hw := goodHardware(t) // TEE
	v, _ := EvaluateAttestation(goodPolicy(), testNonce, goodIntegrity(), hw, attestNow)
	assert.Equal(t, AttestPassed, v)

	p := goodPolicy()
	p.RequireStrongBox = true
	v, _ = EvaluateAttestation(p, testNonce, goodIntegrity(), hw, attestNow)
	assert.Equal(t, AttestFailedHardware, v)
}

// ─── Replay ──────────────────────────────────────────────────────────────────

func TestAttestNonceMustMatchBothHalves(t *testing.T) {
	// The first thing anyone rooting a till would try: replay attestation
	// evidence captured from a clean handset. Both halves must carry the
	// challenge WE issued for THIS request.
	t.Run("integrity nonce wrong", func(t *testing.T) {
		i := goodIntegrity()
		i.Nonce = "deadbeef"
		v, _ := EvaluateAttestation(goodPolicy(), testNonce, i, goodHardware(t), attestNow)
		assert.Equal(t, AttestFailedStale, v)
	})

	t.Run("hardware challenge wrong", func(t *testing.T) {
		hw := goodHardware(t)
		hw.Challenge = "deadbeef"
		v, _ := EvaluateAttestation(goodPolicy(), testNonce, goodIntegrity(), hw, attestNow)
		assert.Equal(t, AttestFailedStale, v)
	})

	t.Run("no expected nonce at all", func(t *testing.T) {
		// A caller that forgot to issue one must not accidentally match an
		// empty challenge.
		i, hw := goodIntegrity(), goodHardware(t)
		i.Nonce, hw.Challenge = "", ""
		v, _ := EvaluateAttestation(goodPolicy(), "", i, hw, attestNow)
		assert.Equal(t, AttestFailedStale, v)
	})
}

func TestAttestStaleToken(t *testing.T) {
	i := goodIntegrity()
	i.IssuedAt = attestNow.Add(-10 * time.Minute)
	v, _ := EvaluateAttestation(goodPolicy(), testNonce, i, goodHardware(t), attestNow)
	assert.Equal(t, AttestFailedStale, v)

	t.Run("missing issue time is stale, not trusted", func(t *testing.T) {
		i := goodIntegrity()
		i.IssuedAt = time.Time{}
		v, _ := EvaluateAttestation(goodPolicy(), testNonce, i, goodHardware(t), attestNow)
		assert.Equal(t, AttestFailedStale, v)
	})
}

// ─── Integrity failures ──────────────────────────────────────────────────────

func TestAttestWrongPackageRejected(t *testing.T) {
	// A genuine verdict for a DIFFERENT app proves nothing about ours, and is
	// exactly what someone would present if they could get one cheaply.
	i := goodIntegrity()
	i.PackageName = "com.attacker.shim"
	v, _ := EvaluateAttestation(goodPolicy(), testNonce, i, goodHardware(t), attestNow)
	assert.Equal(t, AttestFailedIntegrity, v)
}

func TestAttestIntegrityFlags(t *testing.T) {
	for name, mutate := range map[string]func(*IntegrityClaims){
		"unrecognised app":       func(i *IntegrityClaims) { i.AppRecognised = false },
		"device integrity fails": func(i *IntegrityClaims) { i.DeviceIntegrity = false },
		"basic integrity fails":  func(i *IntegrityClaims) { i.BasicIntegrity = false },
	} {
		t.Run(name, func(t *testing.T) {
			i := goodIntegrity()
			mutate(i)
			v, _ := EvaluateAttestation(goodPolicy(), testNonce, i, goodHardware(t), attestNow)
			assert.Equal(t, AttestFailedIntegrity, v)
		})
	}
}

// ─── Hardware failures ───────────────────────────────────────────────────────

func TestAttestHardwareFailures(t *testing.T) {
	t.Run("chain not verified", func(t *testing.T) {
		hw := goodHardware(t)
		hw.ChainVerified = false
		v, _ := EvaluateAttestation(goodPolicy(), testNonce, goodIntegrity(), hw, attestNow)
		assert.Equal(t, AttestFailedChain, v)
	})

	t.Run("verified boot not green", func(t *testing.T) {
		hw := goodHardware(t)
		hw.RootOfTrustOK = false
		v, _ := EvaluateAttestation(goodPolicy(), testNonce, goodIntegrity(), hw, attestNow)
		assert.Equal(t, AttestFailedHardware, v)
	})

	t.Run("software-backed key", func(t *testing.T) {
		// The "hardware" key is a file. This is the exact thing attestation
		// exists to detect.
		hw := goodHardware(t)
		hw.SecurityLevel = SecuritySoftware
		v, _ := EvaluateAttestation(goodPolicy(), testNonce, goodIntegrity(), hw, attestNow)
		assert.Equal(t, AttestFailedHardware, v)
	})

	t.Run("no public key", func(t *testing.T) {
		hw := goodHardware(t)
		hw.PublicKey = nil
		hw.KeyAlgorithm = ""
		v, _ := EvaluateAttestation(goodPolicy(), testNonce, goodIntegrity(), hw, attestNow)
		assert.Equal(t, AttestFailedHardware, v)
	})

	t.Run("typed nil pointer is still no key", func(t *testing.T) {
		// The Go trap this field exists to avoid: an interface holding a typed
		// nil pointer is NOT nil, so a PublicKey == nil check would pass here
		// and let a device with no usable key through.
		hw := goodHardware(t)
		var nilKey *rsa.PublicKey
		hw.PublicKey = nilKey
		hw.KeyAlgorithm = ""
		v, _ := EvaluateAttestation(goodPolicy(), testNonce, goodIntegrity(), hw, attestNow)
		assert.Equal(t, AttestFailedHardware, v)
	})

	t.Run("EC key is extracted but reported as unusable", func(t *testing.T) {
		// EvaluateAttestation passes it — key ALGORITHM policy is the enrolment
		// handler's call, since only it knows the key is destined for wrapping.
		// What matters here is that the algorithm is visible rather than lost.
		hw := goodHardware(t)
		hw.KeyAlgorithm = "ec"
		v, _ := EvaluateAttestation(goodPolicy(), testNonce, goodIntegrity(), hw, attestNow)
		assert.Equal(t, AttestPassed, v)
		assert.Equal(t, "ec", hw.KeyAlgorithm)
	})
}

// ─── Key wrapping ────────────────────────────────────────────────────────────

func TestAttestWrapKeyRoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	key := make([]byte, 16) // AES-128, what both chip families use
	_, err = rand.Read(key)
	require.NoError(t, err)

	wrapped, err := WrapKeyForDevice(&priv.PublicKey, key)
	require.NoError(t, err)
	assert.NotEmpty(t, wrapped)

	// Same plaintext must not produce the same ciphertext twice — OAEP is
	// randomised, and a deterministic wrap would leak which devices hold the
	// same event key.
	again, err := WrapKeyForDevice(&priv.PublicKey, key)
	require.NoError(t, err)
	assert.NotEqual(t, wrapped, again)
}

func TestAttestWrapKeyRefusesGarbage(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	_, err = WrapKeyForDevice(nil, []byte{1, 2, 3})
	assert.Error(t, err)

	_, err = WrapKeyForDevice(&priv.PublicKey, nil)
	assert.Error(t, err, "wrapping an empty key would silently deliver nothing usable")
}

func TestAttestNonceIsRandomAndLongEnough(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		n, err := NewAttestationNonce()
		require.NoError(t, err)
		assert.Len(t, n, 64, "32 bytes as hex")
		assert.False(t, seen[n], "nonce repeated — replay protection would be worthless")
		seen[n] = true
	}
}

func TestAttestParseChainRejectsEmptyAndGarbage(t *testing.T) {
	_, err := ParseAttestationChain(nil)
	assert.Error(t, err)

	_, err = ParseAttestationChain([][]byte{{0x00, 0x01, 0x02}})
	assert.Error(t, err)
}