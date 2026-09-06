package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
)

// ── AES-CMAC (RFC 4493) ───────────────────────────────────────────────────────
//
// Every NXP secure-messaging primitive we need is built on AES-CMAC: the SDM
// session-key derivation and tap MAC on NTAG 424 DNA, the AN10922 key
// diversification used for both chip families, and (from Phase 4) the DESFire
// Transaction MAC that backs the on-tag offline accumulator.
//
// Go's standard library has no CMAC, and the third-party options would mean
// adding a module dependency to the payments trust boundary for roughly eighty
// lines of well-specified arithmetic. It is implemented here instead, against
// the RFC's own test vectors (see cmac_test.go). The implementation is
// deliberately boring: constant-time comparison in cmacEqual, no allocations
// that depend on secret data, and no early return inside the block loop.
//
// Subtlety worth stating, because getting it backwards is the classic CMAC bug:
// an EMPTY message is treated as an incomplete final block (padded, XOR K2),
// NOT as a complete one. RFC 4493's first vector — CMAC of the empty string —
// exists precisely to catch that, and it is the first case in the test file.

const cmacRb = 0x87 // the GF(2^128) reduction constant for a 128-bit block

// shiftLeft1 returns b << 1 across the whole 16-byte block, MSB first.
func shiftLeft1(b []byte) []byte {
	out := make([]byte, len(b))
	var carry byte
	for i := len(b) - 1; i >= 0; i-- {
		out[i] = b[i]<<1 | carry
		carry = b[i] >> 7
	}
	return out
}

// cmacSubkeys derives K1 and K2 from the block cipher, per RFC 4493 §2.3.
func cmacSubkeys(block cipher.Block) (k1, k2 []byte) {
	l := make([]byte, aes.BlockSize)
	block.Encrypt(l, make([]byte, aes.BlockSize))

	k1 = shiftLeft1(l)
	if l[0]&0x80 != 0 {
		k1[len(k1)-1] ^= cmacRb
	}

	k2 = shiftLeft1(k1)
	if k1[0]&0x80 != 0 {
		k2[len(k2)-1] ^= cmacRb
	}
	return k1, k2
}

// AESCMAC computes the full 16-byte AES-CMAC of msg under key.
// key must be a valid AES key length (16 bytes for every chip we support).
func AESCMAC(key, msg []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	k1, k2 := cmacSubkeys(block)

	// Number of blocks. An empty message still has one (padded) block.
	n := (len(msg) + aes.BlockSize - 1) / aes.BlockSize
	complete := n > 0 && len(msg)%aes.BlockSize == 0
	if n == 0 {
		n = 1
	}

	// Build the final block: XOR with K1 when the message divides evenly into
	// blocks, otherwise pad with 0x80 0x00... and XOR with K2.
	last := make([]byte, aes.BlockSize)
	tail := msg[(n-1)*aes.BlockSize:]
	if complete {
		copy(last, tail)
		for i := range last {
			last[i] ^= k1[i]
		}
	} else {
		copy(last, tail)
		last[len(tail)] = 0x80 // safe: len(tail) < BlockSize whenever !complete
		for i := range last {
			last[i] ^= k2[i]
		}
	}

	// CBC-MAC chain over the preceding blocks, then the prepared final block.
	x := make([]byte, aes.BlockSize)
	y := make([]byte, aes.BlockSize)
	for i := 0; i < n-1; i++ {
		blk := msg[i*aes.BlockSize : (i+1)*aes.BlockSize]
		for j := range x {
			y[j] = x[j] ^ blk[j]
		}
		block.Encrypt(x, y)
	}
	for j := range x {
		y[j] = x[j] ^ last[j]
	}
	block.Encrypt(x, y)
	return x, nil
}

// TruncateSDMMAC reduces a full 16-byte CMAC to the 8-byte MAC an NTAG 424 DNA
// puts in its SDM message: the odd-indexed bytes, 1,3,5,...,15.
//
// This is not a generic truncation and swapping it for "first 8 bytes" produces
// a MAC that fails against every real tag while passing any test that uses the
// same helper on both sides — so it lives here, named, with the layout spelled
// out, rather than inline at the call site.
func TruncateSDMMAC(full []byte) []byte {
	out := make([]byte, 0, 8)
	for i := 1; i < len(full); i += 2 {
		out = append(out, full[i])
	}
	return out
}

// cmacEqual compares two MACs in constant time. Using bytes.Equal here would
// leak the length of the matching prefix through timing, which over enough taps
// is a forgery oracle.
func cmacEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}