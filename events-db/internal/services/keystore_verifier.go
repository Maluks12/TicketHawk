package services

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// ── Android Keystore attestation verifier ─────────────────────────────────────
//
// Proves that a key lives in the device's secure hardware and was created for
// the challenge we issued — as opposed to being a file the app claims is
// hardware-backed.
//
// The evidence is a certificate chain. The leaf carries the attested public key
// and a Google-defined extension (OID 1.3.6.1.4.1.11129.2.1.17) describing how
// the key was created; the chain runs back to a Google hardware attestation
// root that is burned into the device at manufacture. Verifying to that root is
// what makes the extension trustworthy — parsed on its own it is just bytes the
// app supplied.
//
// WHAT YOU MUST SUPPLY. The Google hardware attestation root certificates are
// published at https://developer.android.com/privacy-and-security/security-key-attestation
// and cannot be fetched from this environment. Download them, save as PEM, and
// point ATTESTATION_ROOTS_PATH at the file. NewKeystoreVerifier REFUSES to
// construct without them — an unpinned verifier would accept a chain rooted in
// anything, including a self-signed root an attacker generates in a second,
// which is strictly worse than no verification because it looks like security.
//
// ALSO CHECK REVOCATION. Google publishes a revoked-key list at
// https://android.googleapis.com/attestation/status. Devices with extracted
// attestation keys appear there. Not fetched here (no network access in this
// environment, and it needs a refresh loop rather than a per-request call);
// wire it in before enabling offline spend at scale — see PHASE4C notes.

const keystoreAttestationOID = "1.3.6.1.4.1.11129.2.1.17"

// Security levels as encoded in the attestation extension.
const (
	kmSecurityLevelSoftware   = 0
	kmSecurityLevelTEE        = 1
	kmSecurityLevelStrongBox  = 2
)

// Verified boot states from the RootOfTrust structure.
const (
	verifiedBootVerified = 0 // green — the only state we accept
)

// keyDescription mirrors the ASN.1 KeyDescription in the attestation extension.
//
// Only the fields this verifier actually uses are typed; the rest are captured
// as RawValue so parsing does not break when Google adds a field, which they
// do with each KeyMint version. A verifier that fails closed on an unknown
// future field would reject every new handset on the market.
type keyDescription struct {
	AttestationVersion       int
	AttestationSecurityLevel asn1.Enumerated
	KeymasterVersion         int
	KeymasterSecurityLevel   asn1.Enumerated
	AttestationChallenge     []byte
	UniqueID                 []byte
	SoftwareEnforced         asn1.RawValue
	TeeEnforced              asn1.RawValue
}

// rootOfTrust is tag [704] inside the TEE-enforced authorization list.
type rootOfTrust struct {
	VerifiedBootKey   []byte
	DeviceLocked      bool
	VerifiedBootState asn1.Enumerated
	VerifiedBootHash  []byte `asn1:"optional"`
}

// KeystoreVerifier verifies attestation chains against pinned Google roots.
type KeystoreVerifier struct {
	roots *x509.CertPool
}

// NewKeystoreVerifier loads the pinned roots. Refuses to construct without
// them: see the note above about why an unpinned verifier is worse than none.
func NewKeystoreVerifier(rootsPath string) (*KeystoreVerifier, error) {
	if rootsPath == "" {
		return nil, fmt.Errorf("%w: ATTESTATION_ROOTS_PATH is not set", ErrAttestationNotConfigured)
	}
	pemBytes, err := os.ReadFile(rootsPath)
	if err != nil {
		return nil, fmt.Errorf("reading attestation roots: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("attestation roots file contained no usable certificates")
	}
	return &KeystoreVerifier{roots: pool}, nil
}

// Verify checks a DER certificate chain and extracts what the attestation says.
//
// Order matters and is not cosmetic: the chain is verified to a pinned root
// BEFORE the extension is read. Parsing first and trusting the contents of an
// unverified leaf is the classic way to get this wrong — the extension is
// attacker-supplied until the chain says otherwise.
func (v *KeystoreVerifier) Verify(ctx context.Context, chain [][]byte) (*HardwareKeyClaims, error) {
	certs, err := ParseAttestationChain(chain)
	if err != nil {
		return nil, err
	}
	leaf := certs[0]

	// ── Chain verification ───────────────────────────────────────────────────
	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}

	claims := &HardwareKeyClaims{}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         v.roots,
		Intermediates: intermediates,
		// Attestation certs are not TLS certs and carry no EKU we care about.
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		// Returned as a claims struct rather than an error so the caller can
		// record the verdict; a failed chain is an expected outcome, not a
		// server fault.
		claims.ChainVerified = false
		return claims, nil
	}
	claims.ChainVerified = true

	// ── Attested public key ──────────────────────────────────────────────────
	//
	// Extract WHATEVER the device generated and record its algorithm. Only RSA
	// is usable — Android's SECURE_KEY_IMPORT requires an RSA wrapping key —
	// but refusing to extract an EC key here would leave the enrolment handler
	// unable to tell an EC leaf from a certificate with no attestation
	// extension at all, and the device would get an opaque failure rather than
	// "regenerate the key as RSA".
	//
	// Note the root and intermediates may legitimately be EC — Key Attestation
	// CA1, the Remote Key Provisioning root, is EC P-384. Root key type says
	// nothing about leaf key type; the leaf is whatever the app asked Keystore
	// to generate.
	switch pub := leaf.PublicKey.(type) {
	case *rsa.PublicKey:
		claims.PublicKey = pub
		claims.KeyAlgorithm = "rsa"
	case *ecdsa.PublicKey:
		claims.PublicKey = pub
		claims.KeyAlgorithm = "ec"
	default:
		// Unknown algorithm: leave KeyAlgorithm empty so EvaluateAttestation
		// refuses rather than proceeding with a key nothing can use.
		return claims, nil
	}

	// ── The extension ────────────────────────────────────────────────────────
	var raw []byte
	for _, ext := range leaf.Extensions {
		if ext.Id.String() == keystoreAttestationOID {
			raw = ext.Value
			break
		}
	}
	if raw == nil {
		// No attestation extension: an ordinary certificate, not an attestation.
		return claims, nil
	}

	var kd keyDescription
	if _, err := asn1.Unmarshal(raw, &kd); err != nil {
		return claims, nil // malformed extension — treated as no evidence
	}

	claims.Challenge = string(kd.AttestationChallenge)

	switch int(kd.AttestationSecurityLevel) {
	case kmSecurityLevelStrongBox:
		claims.SecurityLevel = SecurityStrongBox
	case kmSecurityLevelTEE:
		claims.SecurityLevel = SecurityTEE
	default:
		claims.SecurityLevel = SecuritySoftware
	}

	claims.RootOfTrustOK = rootOfTrustGreen(kd.TeeEnforced)
	return claims, nil
}

// rootOfTrustGreen pulls RootOfTrust (tag [704]) out of the TEE-enforced
// authorization list and reports whether verified boot is green AND the
// bootloader is locked.
//
// Both conditions matter. A green boot state on an UNLOCKED bootloader means
// the device booted a signed image but the owner can replace it at will, which
// is not a basis for handing over an event key.
//
// Deliberately fails CLOSED: anything it cannot parse returns false. The cost
// of a false negative is a vendor restricted to online-only trading; the cost
// of a false positive is a rooted till holding a forgery key.
func rootOfTrustGreen(teeEnforced asn1.RawValue) bool {
	if len(teeEnforced.Bytes) == 0 {
		return false
	}

	rest := teeEnforced.Bytes
	for len(rest) > 0 {
		var field asn1.RawValue
		var err error
		rest, err = asn1.Unmarshal(rest, &field)
		if err != nil {
			return false
		}
		// Tag 704 is RootOfTrust within the authorization list.
		if field.Class != asn1.ClassContextSpecific || field.Tag != 704 {
			continue
		}

		var rot rootOfTrust
		if _, err := asn1.Unmarshal(field.Bytes, &rot); err != nil {
			return false
		}
		return rot.DeviceLocked && int(rot.VerifiedBootState) == verifiedBootVerified
	}
	return false
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// DecodeChainB64 converts the base64 chain a device posts into DER.
func DecodeChainB64(b64 []string) ([][]byte, error) {
	if len(b64) == 0 {
		return nil, errors.New("empty attestation chain")
	}
	out := make([][]byte, 0, len(b64))
	for i, s := range b64 {
		der, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("attestation chain entry %d is not base64: %w", i, err)
		}
		out = append(out, der)
	}
	return out, nil
}

// EncodeRootsPEM is a test/ops helper for writing a roots file.
func EncodeRootsPEM(certs []*x509.Certificate) []byte {
	var out []byte
	for _, c := range certs {
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})...)
	}
	return out
}

// ChainSerials extracts every certificate serial from a DER chain, normalised
// to the form Google's revocation list publishes.
//
// EVERY certificate, not just the leaf. Google lists intermediates too, and a
// compromised intermediate taints every device beneath it — checking only the
// leaf would miss a whole batch revoked together.
func ChainSerials(chain [][]byte) ([]string, error) {
	certs, err := ParseAttestationChain(chain)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(certs))
	for _, c := range certs {
		// big.Int renders without leading zeros, which is what Google uses, but
		// NormaliseSerial is applied anyway so there is exactly one place that
		// decides the canonical form.
		out = append(out, NormaliseSerial(c.SerialNumber.Text(16)))
	}
	return out, nil
}

// EncodePublicKeyB64 serialises a device's attested public key for storage in
// vendor_pos_devices.signing_pubkey, as base64 PKIX DER.
//
// Stored so an offline transaction's device signature can be verified later.
// That verification is what makes tampering DETECTABLE after the fact — it
// cannot be prevented on hardware a vendor physically controls.
func EncodePublicKeyB64(pub any) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(der)
}