package services

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ── POS device attestation & key delivery ─────────────────────────────────────
//
// The gate that decides whether an event's tag key may be handed to a physical
// device somebody else is holding.
//
// WHY A KEY GOES TO THE DEVICE AT ALL. Offline spend needs the till to verify a
// band's cryptogram with no server to ask, which means holding the event key.
// With symmetric crypto a verification key IS a forgery key — there is no
// version where the device can check a band without also being able to mint
// one. Attestation does not remove that; it reduces how often a compromised
// device is the one holding it. The 4a caps are what bound the loss when this
// gate is beaten.
//
// WHAT THIS DOES AND DOES NOT PROTECT AGAINST. Worth being exact, because the
// architecture doc blurs it:
//
//	Cloned band at an HONEST till   — on-device verification stops it. This is
//	                                  the entire reason keys are delivered.
//	COMPROMISED till                — on-device verification does nothing. The
//	                                  device authors the transaction; it can
//	                                  report a sale that never happened whether
//	                                  or not it holds a key. Only attestation,
//	                                  the caps, and after-the-fact signature
//	                                  checking touch this case.
//
// So key delivery buys exactly one thing, and the caps remain the load-bearing
// control.

// AttestationVerdict mirrors the attestation_verdict enum in 000088.
type AttestationVerdict string

const (
	AttestPassed          AttestationVerdict = "passed"
	AttestFailedIntegrity AttestationVerdict = "failed_integrity"
	AttestFailedHardware  AttestationVerdict = "failed_hardware"
	AttestFailedChain     AttestationVerdict = "failed_chain"
	AttestFailedStale     AttestationVerdict = "failed_stale"
	AttestError           AttestationVerdict = "error"
)

// Security levels from the Keystore attestation extension.
const (
	SecurityStrongBox = "strongbox"
	SecurityTEE       = "tee"
	SecuritySoftware  = "software"
)

// IntegrityClaims is the normalised result of verifying a Play Integrity token.
//
// An interface result rather than Google's raw response so the decision logic
// below is testable without a network call — and so a future move to a
// different attestation provider does not touch the policy.
type IntegrityClaims struct {
	// Nonce the device bound into its request. Must match what we issued, or
	// this is a replay of an attestation captured from a clean handset — the
	// first thing anyone rooting a till would try.
	Nonce string

	AppRecognised   bool   // PLAY_RECOGNIZED — unmodified APK from Play
	DeviceIntegrity bool   // MEETS_DEVICE_INTEGRITY — not rooted/emulated
	BasicIntegrity  bool   // MEETS_BASIC_INTEGRITY
	PackageName     string
	IssuedAt        time.Time
}

// HardwareKeyClaims is the normalised result of verifying an Android Keystore
// attestation certificate chain.
type HardwareKeyClaims struct {
	// Challenge the device bound into the key's attestation. Bound separately
	// from the Play Integrity nonce because they prove different things: one
	// that the app is genuine, the other that THIS key lives in hardware.
	Challenge string

	SecurityLevel  string
	ChainVerified  bool // verified to a pinned Google hardware attestation root
	PublicKey      *rsa.PublicKey
	RootOfTrustOK  bool // verified boot state is green
}

// IntegrityVerifier verifies a Play Integrity token.
//
// An interface because the real implementation calls Google's API, and this
// package must remain unit-testable. See the note on GoogleIntegrityVerifier
// below about what still has to be built.
type IntegrityVerifier interface {
	Verify(ctx context.Context, token string) (*IntegrityClaims, error)
}

// KeyAttestationVerifier verifies an Android Keystore attestation chain.
type KeyAttestationVerifier interface {
	Verify(ctx context.Context, chain [][]byte) (*HardwareKeyClaims, error)
}

// AttestationPolicy is what this deployment insists on.
//
// The zero value REFUSES everything, matching OfflinePolicy in Phase 4a: a
// caller that fails to load configuration denies rather than falling back to
// something permissive.
type AttestationPolicy struct {
	// RequireStrongBox insists the key lives in a discrete secure element
	// rather than the TEE. Stronger, but excludes a lot of mid-range Android
	// hardware — which at a South African festival is most of what vendors
	// actually own. Default false, and the level is recorded either way so an
	// estate that drifts is visible.
	RequireStrongBox bool

	// ExpectedPackage is the Studio app's package name. A valid integrity
	// verdict for a DIFFERENT app is not a valid verdict for this one.
	ExpectedPackage string

	// MaxTokenAge bounds replay beyond the nonce check.
	MaxTokenAge time.Duration
}

var (
	ErrAttestationNonceMismatch = errors.New("attestation nonce does not match the issued challenge")
	ErrAttestationNotConfigured = errors.New("attestation is not configured for this deployment")
)

// EvaluateAttestation applies policy to already-verified claims.
//
// Pure — no network, no clock beyond the `now` parameter — so every branch is
// testable. The cryptographic verification happens in the verifiers; this
// decides what the results mean, which is the part that encodes judgement and
// therefore the part most worth pinning in tests.
func EvaluateAttestation(
	policy AttestationPolicy,
	expectedNonce string,
	integrity *IntegrityClaims,
	hardware *HardwareKeyClaims,
	now time.Time,
) (AttestationVerdict, string) {
	if policy.ExpectedPackage == "" || policy.MaxTokenAge <= 0 {
		// Unconfigured means refuse. A permissive default here would hand out
		// event keys on a misconfiguration.
		return AttestError, "attestation policy is not configured"
	}
	if integrity == nil || hardware == nil {
		return AttestError, "missing attestation evidence"
	}

	// Nonce binding first. Both halves must carry the challenge we issued, or
	// this is a replay of evidence captured elsewhere.
	if expectedNonce == "" ||
		integrity.Nonce != expectedNonce ||
		hardware.Challenge != expectedNonce {
		return AttestFailedStale, "nonce mismatch — evidence was not produced for this request"
	}
	if integrity.IssuedAt.IsZero() || now.Sub(integrity.IssuedAt) > policy.MaxTokenAge {
		return AttestFailedStale, "integrity token is too old"
	}

	// App identity. A genuine verdict for a different package proves nothing
	// about ours.
	if integrity.PackageName != policy.ExpectedPackage {
		return AttestFailedIntegrity, "integrity verdict is for a different application"
	}
	if !integrity.AppRecognised || !integrity.DeviceIntegrity || !integrity.BasicIntegrity {
		return AttestFailedIntegrity, "device or application did not meet integrity requirements"
	}

	// Hardware backing. This is what distinguishes "the app says the key is
	// safe" from "the silicon says so".
	if !hardware.ChainVerified {
		return AttestFailedChain, "attestation chain did not verify to a pinned root"
	}
	if !hardware.RootOfTrustOK {
		return AttestFailedHardware, "verified boot state is not green"
	}
	if hardware.PublicKey == nil {
		return AttestFailedHardware, "no attested public key in the chain"
	}
	switch hardware.SecurityLevel {
	case SecurityStrongBox:
		// always acceptable
	case SecurityTEE:
		if policy.RequireStrongBox {
			return AttestFailedHardware, "key is TEE-backed but this event requires StrongBox"
		}
	default:
		// Software-backed means the "hardware" key is a file. That is the exact
		// thing attestation exists to detect.
		return AttestFailedHardware, "key is not hardware-backed"
	}

	return AttestPassed, ""
}

// ── Nonce issuing ────────────────────────────────────────────────────────────

// NewAttestationNonce returns a 32-byte challenge as hex, to be stored against
// the device and bound into BOTH the Play Integrity request and the Keystore
// attestation challenge.
func NewAttestationNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating attestation nonce: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ── Key wrapping ─────────────────────────────────────────────────────────────

// WrapKeyForDevice encrypts an event tag key to a device's attested public key
// using RSA-OAEP(SHA-256), so the plaintext never crosses the wire and can only
// be recovered inside the keystore that generated the private half.
//
// HONEST LIMITATION, worth stating rather than burying. Unwrapping with a
// Keystore private key means the AES key is decrypted inside the TEE but is
// then returned to application memory. The strictly stronger approach is
// Android's SECURE_KEY_IMPORT, where an ASN.1 SecureKeyWrapper is imported
// directly into StrongBox and the key material never exists outside the secure
// element at all.
//
// I have not built SECURE_KEY_IMPORT here because it cannot be verified without
// a physical device, and shipping untested ASN.1 in the key-delivery path is
// worse than shipping a simpler mechanism whose limits are documented. The
// difference matters only against an attacker with app-memory access on an
// ATTESTED device — precisely the case attestation is supposed to have
// excluded. Treat it as the hardening step once real hardware is in hand.
func WrapKeyForDevice(pub *rsa.PublicKey, key []byte) (string, error) {
	if pub == nil {
		return "", errors.New("no device public key")
	}
	if len(key) == 0 {
		return "", errors.New("refusing to wrap an empty key")
	}
	// Label binds the ciphertext to its purpose, so a wrapped tag key cannot be
	// replayed into a context expecting a different kind of key.
	ct, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, key, []byte("tickethawk-tag-key-v1"))
	if err != nil {
		return "", fmt.Errorf("wrapping key for device: %w", err)
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

// ParseAttestationChain decodes DER certificates into x509 values.
//
// Deliberately does NOT verify them — that is KeyAttestationVerifier's job, and
// separating parse from verify keeps it impossible to accidentally treat a
// well-formed chain as a trusted one.
func ParseAttestationChain(der [][]byte) ([]*x509.Certificate, error) {
	if len(der) == 0 {
		return nil, errors.New("empty attestation chain")
	}
	out := make([]*x509.Certificate, 0, len(der))
	for i, d := range der {
		c, err := x509.ParseCertificate(d)
		if err != nil {
			return nil, fmt.Errorf("attestation chain cert %d: %w", i, err)
		}
		out = append(out, c)
	}
	return out, nil
}