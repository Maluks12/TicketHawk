package repository

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CASHLESS LEDGER TESTS — Phase 3 gate
// =============================================================================
//
// Unit only, no database:
//
//	go test ./internal/repository/... -v -run TestCashless
//
// These cover the parts where a bug costs money rather than an admission: cent
// rounding, sign direction, the spend decision, and chain integrity.

const walletX = "dddddddd-dddd-dddd-dddd-dddddddddddd"

// ─── Money conversion ────────────────────────────────────────────────────────

func TestCashlessToCents(t *testing.T) {
	// The float64 cases are the point of the whole int64-cents decision. 37.50
	// and 0.29 in particular truncate DOWN to 3749 and 28 under int64(x*100),
	// which is a cent lost per transaction, always in the same direction.
	cases := []struct {
		in   float64
		want int64
	}{
		{0.01, 1},
		{0.29, 29},
		// Three decimals is not a price ZAR can express, and the float64 that
		// arrives is 1.00499999999999989..., so it rounds DOWN. Pinned to
		// document that ToCents cannot recover precision the caller's float64
		// never carried — the guarantee below is about 2-decimal amounts.
		{1.005, 100},
		{37.50, 3750},
		{99.99, 9999},
		{100, 10000},
		{1234.56, 123456},
		{-50.25, -5025}, // adjustments may be negative
	}
	for _, tc := range cases {
		got, err := ToCents(tc.in)
		require.NoError(t, err, "%v", tc.in)
		assert.Equal(t, tc.want, got, "ToCents(%v)", tc.in)
	}
}

func TestCashlessToCentsIsExactForEveryRealPrice(t *testing.T) {
	// The guarantee that actually matters: every two-decimal rand amount from
	// R0.01 to R20,000.00 converts to exactly the right number of cents.
	//
	// This is also the case AGAINST the obvious int64(rands*100). That
	// truncation is wrong for roughly one value in twenty-two — always losing a
	// cent, always in the same direction — so a festival's takings would drift
	// steadily short. Both are checked here so a future "simplification" of
	// ToCents fails loudly instead of silently costing money.
	var truncationErrors int
	for i := int64(1); i <= 2_000_000; i++ {
		rands := float64(i) / 100.0

		got, err := ToCents(rands)
		if err != nil {
			t.Fatalf("ToCents(%v) errored: %v", rands, err)
		}
		if got != i {
			t.Fatalf("ToCents(%v) = %d, want %d", rands, got, i)
		}

		if int64(rands*100) != i {
			truncationErrors++
		}
	}
	assert.Greater(t, truncationErrors, 0,
		"if truncation is now exact, this comment and the int64-cents rationale need revisiting")
	t.Logf("naive int64(rands*100) would have been wrong %d times in 2,000,000", truncationErrors)
}

func TestCashlessToCentsRejectsGarbage(t *testing.T) {
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1e12} {
		_, err := ToCents(bad)
		assert.Error(t, err, "%v should be rejected", bad)
	}
}

func TestCashlessCentsRoundTrip(t *testing.T) {
	for _, c := range []int64{0, 1, 99, 100, 3750, 123456, -5025} {
		assert.Equal(t, c, mustCents(t, FromCents(c)))
	}
}

func mustCents(t *testing.T, r float64) int64 {
	t.Helper()
	c, err := ToCents(r)
	require.NoError(t, err)
	return c
}

func TestCashlessSummingCentsIsExact(t *testing.T) {
	// The reason this package does not accumulate money in float64. Ten
	// thousand R0.10 charges is exactly R1000.00 in cents; in float64 it is
	// not, and the drift grows with the size of the event.
	var cents int64
	var flt float64
	for i := 0; i < 10000; i++ {
		cents += 10
		flt += 0.10
	}
	assert.Equal(t, int64(100000), cents)
	assert.NotEqual(t, 1000.0, flt, "if this ever passes, float64 got exact and this comment is wrong")
	assert.Equal(t, 1000.0, FromCents(cents))
}

// ─── Sign rules ──────────────────────────────────────────────────────────────

func TestCashlessSignedCents(t *testing.T) {
	// Credits positive, debits negative, so balance is a plain SUM.
	for _, tc := range []struct {
		kind string
		in   int64
		want int64
	}{
		{TxnTopup, 5000, 5000},
		{TxnRefund, 2500, 2500},
		{TxnPurchase, 3750, -3750},
		{TxnPayout, 1000, -1000},
		{TxnAdjustment, 500, 500},   // corrections may go either way
		{TxnAdjustment, -500, -500},
	} {
		got, err := SignedCents(tc.kind, tc.in)
		require.NoError(t, err, tc.kind)
		assert.Equal(t, tc.want, got, "%s(%d)", tc.kind, tc.in)
	}
}

func TestCashlessSignedCentsRejections(t *testing.T) {
	// A purchase passed as a negative would otherwise become a CREDIT, and the
	// ledger's own sum would agree with it.
	for _, tc := range []struct{ kind string; in int64 }{
		{TxnPurchase, -100},
		{TxnTopup, -100},
		{TxnRefund, -100},
		{TxnPayout, -100},
		{TxnTopup, 0},
		{TxnPurchase, 0},
		{"transfer", 100},
		{"", 100},
	} {
		_, err := SignedCents(tc.kind, tc.in)
		assert.Error(t, err, "%s(%d) should be rejected", tc.kind, tc.in)
	}
}

// ─── Spend authorisation ─────────────────────────────────────────────────────

func okContext() SpendContext {
	w := walletX
	return SpendContext{
		TagStatus:         "active",
		VerificationClass: "server_verified",
		WalletID:          &w,
		WalletStatus:      WalletActive,
		BalanceCents:      10000, // R100.00
	}
}

func TestCashlessAuthorizeSpendAllows(t *testing.T) {
	assert.Equal(t, SpendOK, AuthorizeSpend(okContext(), 3750))

	t.Run("exact balance spends to zero", func(t *testing.T) {
		// The case float64 would eventually get wrong: a wallet holding exactly
		// the price must be able to pay it.
		assert.Equal(t, SpendOK, AuthorizeSpend(okContext(), 10000))
	})

	t.Run("reader_verified chips may spend", func(t *testing.T) {
		c := okContext()
		c.VerificationClass = "reader_verified"
		assert.Equal(t, SpendOK, AuthorizeSpend(c, 100))
	})
}

func TestCashlessAuthorizeSpendRefuses(t *testing.T) {
	t.Run("one cent over balance", func(t *testing.T) {
		assert.Equal(t, SpendInsufficient, AuthorizeSpend(okContext(), 10001))
	})

	t.Run("uid_only chip can never spend", func(t *testing.T) {
		// The database makes this binding unrepresentable; reaching here means
		// the constraint was dropped or the row predates it. Refuse anyway.
		c := okContext()
		c.VerificationClass = "unverified"
		assert.Equal(t, SpendChipNotEligible, AuthorizeSpend(c, 100))
	})

	t.Run("revoked band", func(t *testing.T) {
		for _, s := range []string{"lost", "deactivated", "destroyed", "provisioned", "encoded"} {
			c := okContext()
			c.TagStatus = s
			assert.Equal(t, SpendTagNotActive, AuthorizeSpend(c, 100), s)
		}
	})

	t.Run("band with no wallet", func(t *testing.T) {
		c := okContext()
		c.WalletID = nil
		assert.Equal(t, SpendNoWallet, AuthorizeSpend(c, 100))

		empty := ""
		c.WalletID = &empty
		assert.Equal(t, SpendNoWallet, AuthorizeSpend(c, 100))
	})

	t.Run("frozen or closed wallet", func(t *testing.T) {
		for _, s := range []string{WalletFrozen, WalletClosed, "something_new"} {
			c := okContext()
			c.WalletStatus = s
			assert.Equal(t, SpendWalletNotActive, AuthorizeSpend(c, 100), s)
		}
	})

	t.Run("invalid amounts", func(t *testing.T) {
		assert.Equal(t, SpendInvalidAmount, AuthorizeSpend(okContext(), 0))
		assert.Equal(t, SpendInvalidAmount, AuthorizeSpend(okContext(), -100))
	})

	t.Run("fat-finger cap", func(t *testing.T) {
		// 5000 typed instead of 50.00 must be stopped at the till, not found in
		// reconciliation the next morning.
		c := okContext()
		c.BalanceCents = 99_999_999
		assert.Equal(t, SpendAmountTooLarge, AuthorizeSpend(c, MaxPurchaseCents+1))
		assert.Equal(t, SpendOK, AuthorizeSpend(c, MaxPurchaseCents))
	})
}

func TestCashlessAuthorizeSpendChecksBandBeforeMoney(t *testing.T) {
	// A lost band with an empty wallet must report the band, not the balance —
	// the cashier can act on one of those and not the other.
	c := okContext()
	c.TagStatus = "lost"
	c.BalanceCents = 0
	assert.Equal(t, SpendTagNotActive, AuthorizeSpend(c, 100))
}

// ─── Hash chain ──────────────────────────────────────────────────────────────

func TestCashlessChainHashIsDeterministicAndSensitive(t *testing.T) {
	base := ChainHash("", walletX, TxnPurchase, 1, -3750, "vend-1", "04A1B2C3D4E580")
	assert.Len(t, base, 64)
	assert.Equal(t, base, ChainHash("", walletX, TxnPurchase, 1, -3750, "vend-1", "04A1B2C3D4E580"))

	// Every field must move the hash, or an edit to it would be invisible.
	assert.NotEqual(t, base, ChainHash("x", walletX, TxnPurchase, 1, -3750, "vend-1", "04A1B2C3D4E580"))
	assert.NotEqual(t, base, ChainHash("", "other", TxnPurchase, 1, -3750, "vend-1", "04A1B2C3D4E580"))
	assert.NotEqual(t, base, ChainHash("", walletX, TxnRefund, 1, -3750, "vend-1", "04A1B2C3D4E580"))
	assert.NotEqual(t, base, ChainHash("", walletX, TxnPurchase, 2, -3750, "vend-1", "04A1B2C3D4E580"))
	assert.NotEqual(t, base, ChainHash("", walletX, TxnPurchase, 1, -3751, "vend-1", "04A1B2C3D4E580"))
	assert.NotEqual(t, base, ChainHash("", walletX, TxnPurchase, 1, -3750, "vend-2", "04A1B2C3D4E580"))
	assert.NotEqual(t, base, ChainHash("", walletX, TxnPurchase, 1, -3750, "vend-1", "04A1B2C3D4E581"))
}

func TestCashlessChainHashFieldsCannotBeRearranged(t *testing.T) {
	// Length-prefixing exists so adjacent fields cannot be shifted across the
	// boundary between them and produce the same digest.
	a := ChainHash("", walletX, TxnPurchase, 1, -100, "ab", "cd")
	b := ChainHash("", walletX, TxnPurchase, 1, -100, "a", "bcd")
	assert.NotEqual(t, a, b)
}

func buildChain(t *testing.T, specs []ChainEntry) []ChainEntry {
	t.Helper()
	prev := ""
	out := make([]ChainEntry, 0, len(specs))
	for _, e := range specs {
		e.WalletID = walletX
		e.PrevHash = prev
		e.Hash = ChainHash(prev, e.WalletID, e.Kind, e.Seq, e.SignedCents, e.VendorID, e.TagUID)
		prev = e.Hash
		out = append(out, e)
	}
	return out
}

func TestCashlessVerifyChain(t *testing.T) {
	chain := buildChain(t, []ChainEntry{
		{Kind: TxnTopup, Seq: 1, SignedCents: 20000},
		{Kind: TxnPurchase, Seq: 2, SignedCents: -3750, VendorID: "bar1"},
		{Kind: TxnPurchase, Seq: 3, SignedCents: -6000, VendorID: "food2"},
	})
	assert.Equal(t, -1, VerifyChain(chain), "an untouched chain must verify")

	t.Run("edited amount is caught at that row", func(t *testing.T) {
		// The ordinary tampering case: someone patches one row to hide a charge.
		tampered := append([]ChainEntry(nil), chain...)
		tampered[1].SignedCents = -100
		assert.Equal(t, 1, VerifyChain(tampered))
	})

	t.Run("deleted row breaks the link", func(t *testing.T) {
		tampered := []ChainEntry{chain[0], chain[2]}
		assert.Equal(t, 1, VerifyChain(tampered))
	})

	t.Run("reordered rows break the link", func(t *testing.T) {
		tampered := []ChainEntry{chain[0], chain[2], chain[1]}
		assert.Equal(t, 1, VerifyChain(tampered))
	})

	t.Run("empty chain is intact", func(t *testing.T) {
		assert.Equal(t, -1, VerifyChain(nil))
	})
}

func TestCashlessChainBalanceAgrees(t *testing.T) {
	// The ledger's whole promise: balance is the sum of its entries, and the
	// chain says the entries have not been touched.
	chain := buildChain(t, []ChainEntry{
		{Kind: TxnTopup, Seq: 1, SignedCents: 20000},
		{Kind: TxnPurchase, Seq: 2, SignedCents: -3750, VendorID: "bar1"},
		{Kind: TxnRefund, Seq: 3, SignedCents: 3750, VendorID: "bar1"},
		{Kind: TxnPurchase, Seq: 4, SignedCents: -6000, VendorID: "food2"},
	})
	require.Equal(t, -1, VerifyChain(chain))

	var balance int64
	for _, e := range chain {
		balance += e.SignedCents
	}
	assert.Equal(t, int64(14000), balance)
	assert.Equal(t, 140.0, FromCents(balance))
}