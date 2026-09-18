package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"fmt"
)

// ── SECURE_KEY_IMPORT wrapping ────────────────────────────────────────────────
//
// Builds the ASN.1 SecureKeyWrapper Android expects for
// KeyProperties.PURPOSE_WRAP_KEY / SECURE_KEY_IMPORT (API 28+).
//
// WHY THIS EXISTS ALONGSIDE WrapKeyForDevice. Plain RSA-OAEP works, but
// unwrapping with a Keystore private key decrypts inside the TEE and then
// hands the AES key back to APPLICATION memory. From that moment the event tag
// key is an ordinary byte slice in a process an attacker with code execution
// can read.
//
// SECURE_KEY_IMPORT closes that: the wrapped blob is imported directly into the
// keystore, unwrapped inside the secure element, and installed as a key handle.
// The application can ask the key to do AES operations but can never read it.
// For a key that grants offline spend authority, that is the difference worth
// having.
//
// STRUCTURE (per Android's keystore documentation):
//
//	SecureKeyWrapper ::= SEQUENCE {
//	    version                   INTEGER,          -- 0
//	    encryptedTransportKey     OCTET_STRING,     -- RSA-OAEP(device pub, transportKey)
//	    initializationVector      OCTET_STRING,     -- 12 bytes, AES-GCM
//	    keyDescription            KeyDescription,
//	    encryptedKey              OCTET_STRING,     -- AES-GCM(transportKey, keyMaterial)
//	    tag                       OCTET_STRING      -- 16-byte GCM tag
//	}
//
//	KeyDescription ::= SEQUENCE {
//	    keyFormat                 INTEGER,          -- 3 = RAW
//	    keyParams                 AuthorizationList
//	}
//
// The KeyDescription is authenticated as GCM additional data, so the device
// cannot import our key material under parameters we did not authorise — it
// could otherwise import the event key as, say, exportable, and read it out
// through the front door.
//
// ⚠️ NOT VERIFIED AGAINST A DEVICE. Every byte here follows the specification,
// and the round-trip test proves the crypto is self-consistent, but nothing has
// unwrapped one of these blobs on real Android hardware. The failure mode is
// loud — importKey throws and the till cannot enrol — rather than silent, so it
// fails closed. Treat it exactly like the SDM constants in Phase 1: keep
// WrapKeyForDevice as the working path until a physical device has imported one
// of these successfully, then switch.

// Android keymaster tag numbers used in the authorization list. The encoding is
// tag | type, which is why these are not small sequential integers.
const (
	kmTagAlgorithm   = 2   // ENUM
	kmTagKeySize     = 3   // INTEGER
	kmTagBlockMode   = 4   // ENUM_REP
	kmTagDigest      = 5   // ENUM_REP
	kmTagPadding     = 6   // ENUM_REP
	kmTagPurpose     = 1   // ENUM_REP
	kmTagNoAuthReq   = 503 // BOOL

	kmAlgorithmAES = 32
	kmBlockModeGCM = 32
	kmDigestNone   = 0
	kmPaddingNone  = 1
	kmPurposeEncrypt = 0
	kmPurposeDecrypt = 1

	keyFormatRaw = 3
)

// ErrWrapUnsupportedKeySize guards against wrapping something that is not an
// AES key, which would import successfully and then fail every operation.
var ErrWrapUnsupportedKeySize = errors.New("wrapped key must be 16, 24 or 32 bytes")

// BuildSecureKeyWrapper produces a base64 SecureKeyWrapper for Android's
// SECURE_KEY_IMPORT.
//
// maskingKey is XORed by Android during import and is conventionally all zeros;
// it is a parameter rather than a constant so a future policy can use it
// without a signature change.
func BuildSecureKeyWrapper(devicePub *rsa.PublicKey, keyMaterial []byte, maskingKey []byte) (string, error) {
	if devicePub == nil {
		return "", errors.New("no device public key")
	}
	switch len(keyMaterial) {
	case 16, 24, 32:
	default:
		return "", ErrWrapUnsupportedKeySize
	}

	// 1. Ephemeral AES-256 transport key, used once for this blob.
	transportKey := make([]byte, 32)
	if _, err := rand.Read(transportKey); err != nil {
		return "", fmt.Errorf("generating transport key: %w", err)
	}

	// 2. Wrap the transport key to the device's attested public key.
	//    Android specifies OAEP with SHA-256 and an empty label here — unlike
	//    WrapKeyForDevice, which uses a purpose label. The label is fixed by the
	//    platform, not by us, so it cannot be changed.
	encTransport, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, devicePub, transportKey, nil)
	if err != nil {
		return "", fmt.Errorf("wrapping transport key: %w", err)
	}

	// 3. The key description, authenticated as GCM AAD so the device cannot
	//    import our key under parameters we did not authorise.
	desc, err := buildKeyDescription(len(keyMaterial) * 8)
	if err != nil {
		return "", err
	}

	// 4. Encrypt the key material under the transport key.
	block, err := aes.NewCipher(transportKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	iv := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generating IV: %w", err)
	}

	masked := keyMaterial
	if len(maskingKey) > 0 {
		masked = make([]byte, len(keyMaterial))
		for i := range keyMaterial {
			masked[i] = keyMaterial[i] ^ maskingKey[i%len(maskingKey)]
		}
	}

	sealed := gcm.Seal(nil, iv, masked, desc)
	// Go appends the tag to the ciphertext; Android wants them as separate
	// fields. Splitting them wrongly is the most likely place for this to
	// break, so it is done explicitly rather than by slicing inline.
	tagLen := gcm.Overhead() // 16
	if len(sealed) < tagLen {
		return "", errors.New("GCM output shorter than its tag")
	}
	encryptedKey := sealed[:len(sealed)-tagLen]
	gcmTag := sealed[len(sealed)-tagLen:]

	wrapper := struct {
		Version               int
		EncryptedTransportKey []byte
		InitializationVector  []byte
		KeyDescription        asn1.RawValue
		EncryptedKey          []byte
		Tag                   []byte
	}{
		Version:               0,
		EncryptedTransportKey: encTransport,
		InitializationVector:  iv,
		KeyDescription:        asn1.RawValue{FullBytes: desc},
		EncryptedKey:          encryptedKey,
		Tag:                   gcmTag,
	}

	der, err := asn1.Marshal(wrapper)
	if err != nil {
		return "", fmt.Errorf("encoding SecureKeyWrapper: %w", err)
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// buildKeyDescription encodes KeyDescription for a RAW AES key restricted to
// GCM encrypt/decrypt with no padding.
//
// The restrictions are the point. Importing the tag key as AES/GCM
// encrypt+decrypt only, with no digest and no padding, means the keystore will
// refuse to use it for anything else — so even a compromised app cannot coax
// the secure element into an operation that would leak key material.
func buildKeyDescription(keySizeBits int) ([]byte, error) {
	authList, err := buildAuthorizationList(keySizeBits)
	if err != nil {
		return nil, err
	}

	desc := struct {
		KeyFormat int
		KeyParams asn1.RawValue
	}{
		KeyFormat: keyFormatRaw,
		KeyParams: asn1.RawValue{FullBytes: authList},
	}
	return asn1.Marshal(desc)
}

// buildAuthorizationList encodes the keymaster AuthorizationList.
//
// Every entry is context-specific and explicitly tagged by its keymaster tag
// number. Order matters — the list is DER SEQUENCE and Android parses it
// positionally by tag, so entries are emitted in ascending tag order.
func buildAuthorizationList(keySizeBits int) ([]byte, error) {
	var body []byte

	appendEnumSet := func(tag int, values ...int) error {
		var setBody []byte
		for _, v := range values {
			b, err := asn1.Marshal(asn1.Enumerated(v))
			if err != nil {
				return err
			}
			setBody = append(setBody, b...)
		}
		wrapped, err := asn1.Marshal(asn1.RawValue{
			Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: setBody,
		})
		if err != nil {
			return err
		}
		el, err := asn1.Marshal(asn1.RawValue{
			Class: asn1.ClassContextSpecific, Tag: tag, IsCompound: true, Bytes: wrapped,
		})
		if err != nil {
			return err
		}
		body = append(body, el...)
		return nil
	}

	appendTagged := func(tag int, inner any) error {
		b, err := asn1.Marshal(inner)
		if err != nil {
			return err
		}
		el, err := asn1.Marshal(asn1.RawValue{
			Class: asn1.ClassContextSpecific, Tag: tag, IsCompound: true, Bytes: b,
		})
		if err != nil {
			return err
		}
		body = append(body, el...)
		return nil
	}

	// Ascending tag order: purpose(1), algorithm(2), keySize(3), blockMode(4),
	// digest(5), padding(6), noAuthRequired(503).
	if err := appendEnumSet(kmTagPurpose, kmPurposeEncrypt, kmPurposeDecrypt); err != nil {
		return nil, err
	}
	if err := appendTagged(kmTagAlgorithm, asn1.Enumerated(kmAlgorithmAES)); err != nil {
		return nil, err
	}
	if err := appendTagged(kmTagKeySize, keySizeBits); err != nil {
		return nil, err
	}
	if err := appendEnumSet(kmTagBlockMode, kmBlockModeGCM); err != nil {
		return nil, err
	}
	if err := appendEnumSet(kmTagDigest, kmDigestNone); err != nil {
		return nil, err
	}
	if err := appendEnumSet(kmTagPadding, kmPaddingNone); err != nil {
		return nil, err
	}
	// No user authentication: a till operates with a queue in front of it and
	// cannot prompt for a fingerprint on every tap.
	if err := appendTagged(kmTagNoAuthReq, true); err != nil {
		return nil, err
	}

	return asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: body,
	})
}

// UnwrapSecureKeyWrapperForTest reverses BuildSecureKeyWrapper using the
// device's PRIVATE key.
//
// Test-only, and named so. Real devices never expose the private half — that is
// the entire point — so this exists solely to prove the encoder is
// self-consistent. It does NOT prove Android will accept the blob; only a
// physical import does that.
func UnwrapSecureKeyWrapperForTest(priv *rsa.PrivateKey, b64 string, maskingKey []byte) ([]byte, error) {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Version               int
		EncryptedTransportKey []byte
		InitializationVector  []byte
		KeyDescription        asn1.RawValue
		EncryptedKey          []byte
		Tag                   []byte
	}
	if _, err := asn1.Unmarshal(der, &wrapper); err != nil {
		return nil, fmt.Errorf("decoding SecureKeyWrapper: %w", err)
	}

	transportKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, wrapper.EncryptedTransportKey, nil)
	if err != nil {
		return nil, fmt.Errorf("unwrapping transport key: %w", err)
	}

	block, err := aes.NewCipher(transportKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	sealed := append(append([]byte{}, wrapper.EncryptedKey...), wrapper.Tag...)
	masked, err := gcm.Open(nil, wrapper.InitializationVector, sealed, wrapper.KeyDescription.FullBytes)
	if err != nil {
		return nil, fmt.Errorf("decrypting key material: %w", err)
	}

	if len(maskingKey) > 0 {
		out := make([]byte, len(masked))
		for i := range masked {
			out[i] = masked[i] ^ maskingKey[i%len(maskingKey)]
		}
		return out, nil
	}
	return masked, nil
}