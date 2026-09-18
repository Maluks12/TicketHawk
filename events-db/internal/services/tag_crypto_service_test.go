package services

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"strings"
	"testing"

	"events-db/internal/config"
	"events-db/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TAG CRYPTO TESTS — Phase 1 gate
// =============================================================================
//
// All unit, no database, no hardware. Run with:
//
//	go test ./internal/services/... -v -run TestTag
//
// WHAT THESE PROVE, AND WHAT THEY DO NOT.
//
// TestTagCMACVectors is the only test here with external authority: it checks
// our AES-CMAC against RFC 4493's published vectors. If that passes, the
// primitive under everything else is correct.
//
// TestTagSDMRoundTrip encodes a tap with the same derivation the verifier uses,
// so it proves the plumbing — decrypt, derive, MAC, compare — and it proves the
// tamper cases fail. It does NOT prove conformance with real NXP silicon,
// because both sides share our assumptions about sdmSV2Prefix and the PICC
// layout. A round-trip test cannot catch a shared wrong constant.
//
// TestTagSDMAgainstRealTag is where that gets closed, and it is SKIPPED until
// somebody taps an actual encoded wristband and pastes the capture in. Phase 1
// is not done until it runs green. This is stated in three places on purpose.

const testMasterKeyHex = "404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f"

func testTagService(t *testing.T) *TagCryptoService {
	t.Helper()
	svc, err := NewTagCryptoService(&config.Config{TagMasterKey: testMasterKeyHex})
	require.NoError(t, err)
	return svc
}

// ─── AES-CMAC: RFC 4493 §4 ───────────────────────────────────────────────────

func TestTagCMACVectors(t *testing.T) {
	key, err := hex.DecodeString("2b7e151628aed2a6abf7158809cf4f3c")
	require.NoError(t, err)

	cases := []struct{ name, msg, want string }{
		{
			// The empty message is the case implementations get wrong: it is an
			// INCOMPLETE final block (pad, XOR K2), not a complete one.
			name: "empty",
			msg:  "",
			want: "bb1d6929e95937287fa37d129b756746",
		},
		{
			name: "one full block",
			msg:  "6bc1bee22e409f96e93d7e117393172a",
			want: "070a16b46b4d4144f79bdd9dd04a287c",
		},
		{
			name: "partial final block",
			msg:  "6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411",
			want: "dfa66747de9ae63030ca32611497c827",
		},
		{
			name: "four full blocks",
			msg: "6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e51" +
				"30c81c46a35ce411e5fbc1191a0a52eff69f2445df4f9b17ad2b417be66c3710",
			want: "51f0bebf7e3b9d92fc49741779363cfe",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := hex.DecodeString(tc.msg)
			require.NoError(t, err)
			got, err := AESCMAC(key, msg)
			require.NoError(t, err)
			assert.Equal(t, tc.want, hex.EncodeToString(got))
		})
	}
}

func TestTagSDMMACTruncation(t *testing.T) {
	// The SDM MAC is the ODD-indexed bytes of the full CMAC, not the first
	// eight. Swapping the two passes any round-trip test that uses the same
	// helper on both sides and fails against every real tag, so it is pinned.
	full := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	assert.Equal(t, []byte{1, 3, 5, 7, 9, 11, 13, 15}, TruncateSDMMAC(full))
}

// ─── Key derivation ──────────────────────────────────────────────────────────

func TestTagKeyDerivation(t *testing.T) {
	svc := testTagService(t)
	uidA := []byte{0x04, 0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0x80}
	uidB := []byte{0x04, 0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0x81} // one bit apart

	const evt1 = "11111111-1111-1111-1111-111111111111"
	const evt2 = "22222222-2222-2222-2222-222222222222"

	k, err := svc.TagKey(evt1, 1, uidA, tagPurposeMAC)
	require.NoError(t, err)
	assert.Len(t, k, 16, "derived keys must be AES-128 — both chip families use 128-bit keys")

	t.Run("deterministic", func(t *testing.T) {
		again, err := svc.TagKey(evt1, 1, uidA, tagPurposeMAC)
		require.NoError(t, err)
		assert.Equal(t, k, again)
	})

	// Each of these is a containment boundary. If any two of them collide, a key
	// recovered in one context unlocks another.
	t.Run("scoped per event", func(t *testing.T) {
		other, err := svc.TagKey(evt2, 1, uidA, tagPurposeMAC)
		require.NoError(t, err)
		assert.NotEqual(t, k, other, "a key from one event must not open another")
	})

	t.Run("scoped per key version", func(t *testing.T) {
		other, err := svc.TagKey(evt1, 2, uidA, tagPurposeMAC)
		require.NoError(t, err)
		assert.NotEqual(t, k, other, "rotation must actually change the key")
	})

	t.Run("scoped per tag", func(t *testing.T) {
		other, err := svc.TagKey(evt1, 1, uidB, tagPurposeMAC)
		require.NoError(t, err)
		assert.NotEqual(t, k, other, "adjacent UIDs must not share a key")
	})

	t.Run("scoped per purpose", func(t *testing.T) {
		meta, err := svc.TagKey(evt1, 1, uidA, tagPurposeMeta)
		require.NoError(t, err)
		app, err := svc.TagKey(evt1, 1, uidA, tagPurposeAppMK)
		require.NoError(t, err)
		assert.NotEqual(t, k, meta)
		assert.NotEqual(t, k, app)
		assert.NotEqual(t, meta, app)
	})

	t.Run("master key is not recoverable from a derived key", func(t *testing.T) {
		// Not a proof, just a canary: a refactor that accidentally returns the
		// master (or a slice of it) instead of a derivation should fail loudly.
		assert.NotEqual(t, testMasterKeyHex[:32], hex.EncodeToString(k))
	})
}

func TestTagMasterKeyValidation(t *testing.T) {
	// Boot-time failure is the point: a bad key must stop the process, not
	// surface as refused wristbands at a gate.
	for _, bad := range []struct{ name, key string }{
		{"empty", ""},
		{"not hex", "zzzz"},
		{"too short", "0011223344556677"}, // 8 bytes
		{"odd length", "0011223344556677889900112233445566"},
	} {
		t.Run(bad.name, func(t *testing.T) {
			_, err := NewTagCryptoService(&config.Config{TagMasterKey: bad.key})
			assert.Error(t, err)
		})
	}

	t.Run("accepts 128/192/256-bit masters", func(t *testing.T) {
		for _, n := range []int{16, 24, 32} {
			_, err := NewTagCryptoService(&config.Config{
				TagMasterKey: strings.Repeat("ab", n),
			})
			assert.NoError(t, err, "%d-byte master should be accepted", n)
		}
	})
}

// ─── SDM round-trip (NTAG 424 DNA) ───────────────────────────────────────────

// encodeSDMTap plays the part of the tag: builds the PICC block, encrypts it
// with the meta key, and produces the truncated tap MAC. It mirrors verifySDM
// and therefore shares its assumptions — see the header note on what this can
// and cannot prove.
func encodeSDMTap(t *testing.T, svc *TagCryptoService, eventID string, keyVersion int, uid []byte, ctr int64) *SDMPayload {
	t.Helper()
	require.Len(t, uid, 7)

	plain := make([]byte, aes.BlockSize)
	plain[0] = sdmPICCTagByte
	copy(plain[1:8], uid)
	plain[8] = byte(ctr)
	plain[9] = byte(ctr >> 8)
	plain[10] = byte(ctr >> 16)
	// bytes 11..15 stay zero — padding, ignored by the verifier

	metaKey, err := svc.TagKey(eventID, keyVersion, nil, tagPurposeMeta)
	require.NoError(t, err)
	block, err := aes.NewCipher(metaKey)
	require.NoError(t, err)
	ct := make([]byte, aes.BlockSize)
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(ct, plain)

	macKey, err := svc.TagKey(eventID, keyVersion, uid, tagPurposeMAC)
	require.NoError(t, err)
	sv := append(append([]byte{}, sdmSV2Prefix...), uid...)
	sv = append(sv, plain[8], plain[9], plain[10])
	sessionKey, err := AESCMAC(macKey, sv)
	require.NoError(t, err)
	full, err := AESCMAC(sessionKey, nil)
	require.NoError(t, err)

	return &SDMPayload{
		PICCData: hex.EncodeToString(ct),
		CMAC:     hex.EncodeToString(TruncateSDMMAC(full)),
	}
}

func TestTagSDMRoundTrip(t *testing.T) {
	svc := testTagService(t)
	const evt = "11111111-1111-1111-1111-111111111111"
	uid := []byte{0x04, 0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0x80}

	sdm := encodeSDMTap(t, svc, evt, 1, uid, 42)

	v, err := svc.VerifyTap(evt, 1, &TapRequest{ChipType: models.ChipNTAG424, SDM: sdm})
	require.NoError(t, err)

	assert.Equal(t, "04A1B2C3D4E580", v.UID, "UID must come out of the encrypted block, uppercase hex")
	assert.Equal(t, models.VerifyServer, v.Class)
	require.NotNil(t, v.ReadCounter)
	assert.Equal(t, int64(42), *v.ReadCounter)
	assert.Empty(t, v.Warnings)
}

func TestTagSDMCounterIsReadBack(t *testing.T) {
	// The counter is the anti-replay control, so it has to survive the 3-byte
	// little-endian round trip exactly — including the top of its range, where
	// a byte-order slip would still pass at small values.
	svc := testTagService(t)
	const evt = "11111111-1111-1111-1111-111111111111"
	uid := []byte{0x04, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66}

	for _, ctr := range []int64{0, 1, 255, 256, 65535, 65536, 16777215} {
		sdm := encodeSDMTap(t, svc, evt, 1, uid, ctr)
		v, err := svc.VerifyTap(evt, 1, &TapRequest{ChipType: models.ChipNTAG424, SDM: sdm})
		require.NoError(t, err, "counter %d", ctr)
		require.NotNil(t, v.ReadCounter)
		assert.Equal(t, ctr, *v.ReadCounter)
	}
}

func TestTagSDMRejections(t *testing.T) {
	svc := testTagService(t)
	const evt = "11111111-1111-1111-1111-111111111111"
	const otherEvt = "22222222-2222-2222-2222-222222222222"
	uid := []byte{0x04, 0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0x80}

	t.Run("tampered MAC", func(t *testing.T) {
		sdm := encodeSDMTap(t, svc, evt, 1, uid, 7)
		mac, _ := hex.DecodeString(sdm.CMAC)
		mac[0] ^= 0x01
		sdm.CMAC = hex.EncodeToString(mac)

		_, err := svc.VerifyTap(evt, 1, &TapRequest{ChipType: models.ChipNTAG424, SDM: sdm})
		assert.ErrorIs(t, err, ErrTagBadMAC)
	})

	t.Run("tampered PICC data", func(t *testing.T) {
		// Flipping ciphertext yields garbage plaintext; the tag byte check
		// catches it before the MAC does.
		sdm := encodeSDMTap(t, svc, evt, 1, uid, 7)
		picc, _ := hex.DecodeString(sdm.PICCData)
		picc[3] ^= 0xFF
		sdm.PICCData = hex.EncodeToString(picc)

		_, err := svc.VerifyTap(evt, 1, &TapRequest{ChipType: models.ChipNTAG424, SDM: sdm})
		assert.Error(t, err)
	})

	t.Run("tag from another event", func(t *testing.T) {
		// The containment property, end to end: a perfectly valid tap from one
		// festival must not verify at another.
		sdm := encodeSDMTap(t, svc, otherEvt, 1, uid, 7)
		_, err := svc.VerifyTap(evt, 1, &TapRequest{ChipType: models.ChipNTAG424, SDM: sdm})
		assert.Error(t, err)
	})

	t.Run("tag encoded under a rotated-away key version", func(t *testing.T) {
		sdm := encodeSDMTap(t, svc, evt, 1, uid, 7)
		_, err := svc.VerifyTap(evt, 2, &TapRequest{ChipType: models.ChipNTAG424, SDM: sdm})
		assert.Error(t, err)
	})

	t.Run("cloned UID with no valid cryptogram", func(t *testing.T) {
		// The whole anti-cloning claim in one case: knowing the UID buys an
		// attacker nothing without the key.
		_, err := svc.VerifyTap(evt, 1, &TapRequest{
			ChipType: models.ChipNTAG424,
			TagUID:   "04A1B2C3D4E580",
			SDM:      &SDMPayload{PICCData: strings.Repeat("00", 16), CMAC: strings.Repeat("00", 8)},
		})
		assert.Error(t, err)
	})

	t.Run("malformed payloads", func(t *testing.T) {
		for _, bad := range []*SDMPayload{
			{PICCData: "", CMAC: strings.Repeat("00", 8)},
			{PICCData: strings.Repeat("00", 15), CMAC: strings.Repeat("00", 8)}, // short block
			{PICCData: strings.Repeat("00", 16), CMAC: strings.Repeat("00", 4)}, // short MAC
			{PICCData: "nothex" + strings.Repeat("0", 26), CMAC: strings.Repeat("00", 8)},
		} {
			_, err := svc.VerifyTap(evt, 1, &TapRequest{ChipType: models.ChipNTAG424, SDM: bad})
			assert.ErrorIs(t, err, ErrTagBadSDM)
		}
	})

	t.Run("missing sdm block", func(t *testing.T) {
		_, err := svc.VerifyTap(evt, 1, &TapRequest{ChipType: models.ChipNTAG424, TagUID: "04A1B2C3D4E580"})
		assert.ErrorIs(t, err, ErrTagBadSDM)
	})

	t.Run("caller-asserted UID is ignored", func(t *testing.T) {
		// A reader must not be able to name the tag. The UID comes out of the
		// decrypted block or not at all.
		sdm := encodeSDMTap(t, svc, evt, 1, uid, 9)
		v, err := svc.VerifyTap(evt, 1, &TapRequest{
			ChipType: models.ChipNTAG424,
			TagUID:   "DEADBEEFDEADBE", // a lie
			SDM:      sdm,
		})
		require.NoError(t, err)
		assert.Equal(t, "04A1B2C3D4E580", v.UID)
	})
}

// ─── DESFire and UID-only ────────────────────────────────────────────────────

func TestTagDESFireIsReaderVerifiedOnly(t *testing.T) {
	svc := testTagService(t)
	const evt = "11111111-1111-1111-1111-111111111111"
	ctr := int64(5)

	t.Run("successful claim is accepted but downgraded", func(t *testing.T) {
		for _, chip := range []models.ChipType{models.ChipDESFireEV2, models.ChipDESFireEV3} {
			v, err := svc.VerifyTap(evt, 1, &TapRequest{
				ChipType: chip,
				TagUID:   "04:A1:B2:C3:D4:E5:80",
				Reader:   &ReaderClaim{AuthOK: true, OriginalityOK: true, Counter: &ctr},
			})
			require.NoError(t, err)
			assert.Equal(t, "04A1B2C3D4E580", v.UID, "separators must be normalised away")
			assert.Equal(t, models.VerifyReader, v.Class,
				"DESFire can never reach server_verified — the server sees only the reader's claim")
			assert.Empty(t, v.Warnings)
		}
	})

	t.Run("failed or absent auth is refused", func(t *testing.T) {
		for _, r := range []*ReaderClaim{nil, {AuthOK: false, OriginalityOK: true}} {
			_, err := svc.VerifyTap(evt, 1, &TapRequest{
				ChipType: models.ChipDESFireEV3,
				TagUID:   "04A1B2C3D4E580",
				Reader:   r,
			})
			assert.ErrorIs(t, err, ErrTagReaderClaimOnly)
		}
	})

	t.Run("originality failure warns rather than denies entry", func(t *testing.T) {
		v, err := svc.VerifyTap(evt, 1, &TapRequest{
			ChipType: models.ChipDESFireEV2,
			TagUID:   "04A1B2C3D4E580",
			Reader:   &ReaderClaim{AuthOK: true, OriginalityOK: false},
		})
		require.NoError(t, err)
		assert.Contains(t, v.Warnings, "originality_signature_not_confirmed")
	})
}

func TestTagUIDOnlyIsAlwaysUnverified(t *testing.T) {
	svc := testTagService(t)
	v, err := svc.VerifyTap("11111111-1111-1111-1111-111111111111", 1, &TapRequest{
		ChipType: models.ChipUIDOnly,
		TagUID:   "04a1b2c3d4e580",
	})
	require.NoError(t, err)
	assert.Equal(t, models.VerifyNone, v.Class)
	assert.Nil(t, v.ReadCounter, "a UID-only chip offers no replay evidence")
	assert.Contains(t, v.Warnings, "uid_only_chip_is_cloneable")
}

func TestTagUnsupportedChip(t *testing.T) {
	svc := testTagService(t)
	_, err := svc.VerifyTap("11111111-1111-1111-1111-111111111111", 1,
		&TapRequest{ChipType: models.ChipType("mifare_classic"), TagUID: "04A1B2C3D4E580"})
	assert.ErrorIs(t, err, ErrTagUnsupportedChip)
}

// ─── UID normalisation ───────────────────────────────────────────────────────

func TestTagNormaliseUID(t *testing.T) {
	// Readers disagree about case and separators. Without canonicalisation the
	// same physical chip registers as several unknown tags.
	ok := []struct{ in, want string }{
		{"04a1b2c3d4e580", "04A1B2C3D4E580"},
		{"04:A1:B2:C3:D4:E5:80", "04A1B2C3D4E580"},
		{"04-a1-b2-c3-d4-e5-80", "04A1B2C3D4E580"},
		{"  04A1B2C3D4E580  ", "04A1B2C3D4E580"},
		{"04A1B2C3", "04A1B2C3"}, // 4-byte UID
	}
	for _, tc := range ok {
		got, err := NormaliseUID(tc.in)
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.want, got)
	}

	bad := []string{"", "   ", "04A1B2", "zzzzzzzz", "04A1B2C3D", strings.Repeat("AB", 12)}
	for _, in := range bad {
		_, err := NormaliseUID(in)
		assert.Error(t, err, "expected %q to be rejected", in)
	}
}

// ─── Conformance harness (hardware required) ─────────────────────────────────

// TestTagSDMAgainstRealTag is the only test that can catch a wrong protocol
// constant. Everything above shares the verifier's assumptions; this does not.
//
// To run it: encode one NTAG 424 DNA wristband with the platform encoder, tap
// it, capture the picc_data and cmac query parameters the tag emits, and fill
// in the four constants below along with the event ID and key version used at
// encoding time.
//
// PHASE 1 IS NOT COMPLETE UNTIL THIS PASSES. If it fails, the likely culprits
// are sdmSV2Prefix, sdmPICCTagByte, or the SDM encoding profile written to the
// tag — in that order.
func TestTagSDMAgainstRealTag(t *testing.T) {
	const (
		realEventID    = "" // event the tag was encoded for
		realKeyVersion = 1
		realPICCData   = "" // hex from the tag's picc_data parameter
		realCMAC       = "" // hex from the tag's cmac parameter
		expectedUID    = "" // UID printed on / read from the chip, uppercase hex
	)

	if realPICCData == "" || realCMAC == "" {
		t.Skip("no real-tag capture provided — see the comment above; Phase 1 sign-off requires this test")
	}

	svc := testTagService(t) // replace with the real master when capturing
	v, err := svc.VerifyTap(realEventID, realKeyVersion, &TapRequest{
		ChipType: models.ChipNTAG424,
		SDM:      &SDMPayload{PICCData: realPICCData, CMAC: realCMAC},
	})
	require.NoError(t, err, "real tag failed verification — check sdmSV2Prefix and the encoding profile")
	assert.Equal(t, expectedUID, v.UID)
	require.NotNil(t, v.ReadCounter)
	t.Logf("real tag verified: uid=%s counter=%d", v.UID, *v.ReadCounter)
}