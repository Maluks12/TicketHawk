package services

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ── Offline transaction signatures ────────────────────────────────────────────
//
// Every offline sale is signed by the till's hardware-backed key — the same key
// pair attested during enrolment, whose private half never leaves the secure
// element.
//
// WHAT THIS BUYS, precisely, because §5 of the architecture doc overstates it.
// A signature does NOT prevent a compromised till from inventing transactions:
// that device holds the key and can sign whatever it likes. What it does is
// make the alternative impossible — nobody ELSE can forge a transaction and
// attribute it to that till, and a till cannot later disown a sale it signed.
// So the fraud surface narrows from "anyone on the network" to "this specific
// device, provably", which is what turns a reconciliation dispute from an
// argument into a finding.
//
// It is detection, not prevention, and the Phase 4a caps remain what bounds the
// loss.

var (
	ErrSignatureMissing = errors.New("offline transaction is not signed")
	ErrSignatureInvalid = errors.New("offline transaction signature does not verify")
	ErrDeviceKeyMissing = errors.New("device has no signing key on file")
)

// OfflineTxnPayload is the canonical set of fields a device signs.
//
// FROZEN. Changing which fields are included, or their order, invalidates every
// signature ever produced — so a new field means a new version prefix, never an
// edit to this struct's contribution to the digest.
//
// Note what IS covered: the amount, the wallet's band, the vendor, the device's
// own sequence number, and the accumulator readings. A vendor who edits any of
// those in their queue before syncing produces a signature that no longer
// verifies, which is precisely the tampering §5's monotonic counter is meant to
// catch.
type OfflineTxnPayload struct {
	TxnID        string
	EventID      string
	TagUID       string
	VendorID     string
	DeviceID     string
	AmountCents  int64
	DeviceSeq    int64
	AccumBefore  int64
	AccumAfter   int64
	ApprovedAtMs int64
}

// CanonicalBytes renders a payload deterministically for signing.
//
// Length-prefixed, exactly as ChainHash is and for the same reason: without it,
// a vendor id ending in digits and a device id starting with them could be
// rearranged into the same byte stream, letting one signed transaction stand in
// for a different one.
func (p OfflineTxnPayload) CanonicalBytes() []byte {
	var b strings.Builder
	for _, f := range []string{
		"otxv1",
		p.TxnID,
		p.EventID,
		p.TagUID,
		p.VendorID,
		p.DeviceID,
		fmt.Sprintf("%d", p.AmountCents),
		fmt.Sprintf("%d", p.DeviceSeq),
		fmt.Sprintf("%d", p.AccumBefore),
		fmt.Sprintf("%d", p.AccumAfter),
		fmt.Sprintf("%d", p.ApprovedAtMs),
	} {
		fmt.Fprintf(&b, "%d:%s|", len(f), f)
	}
	return []byte(b.String())
}

// VerifyOfflineTxnSignature checks a base64 RSA-PKCS1v15-SHA256 signature
// against the device's stored public key.
//
// PKCS#1 v1.5 rather than PSS because Android Keystore's
// SHA256withRSA is v1.5, and a till signing with the platform default is what
// we want — the alternative is a custom signing path on the device, which is
// more code in the place hardest to debug.
func VerifyOfflineTxnSignature(pubKeyB64, signatureB64 string, payload OfflineTxnPayload) error {
	if strings.TrimSpace(signatureB64) == "" {
		return ErrSignatureMissing
	}
	if strings.TrimSpace(pubKeyB64) == "" {
		return ErrDeviceKeyMissing
	}

	pub, err := DecodePublicKeyB64(pubKeyB64)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDeviceKeyMissing, err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok || rsaPub == nil {
		return fmt.Errorf("%w: stored key is not RSA", ErrDeviceKeyMissing)
	}

	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("%w: signature is not base64", ErrSignatureInvalid)
	}

	digest := sha256.Sum256(payload.CanonicalBytes())
	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, digest[:], sig); err != nil {
		return ErrSignatureInvalid
	}
	return nil
}

// DecodePublicKeyB64 reverses EncodePublicKeyB64.
func DecodePublicKeyB64(b64 string) (crypto.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("public key is not base64: %w", err)
	}
	return x509.ParsePKIXPublicKey(der)
}