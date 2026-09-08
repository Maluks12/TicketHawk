package repository

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// SETTLEMENT TESTS
// =============================================================================
//
//	go test ./internal/repository/... -v -run "TestSettlement|TestFloat"
//
// These figures leave the building as payments. The invariant tests matter more
// than the example ones: a rounding rule that loses a cent produces payouts
// that do not add up, and a vendor's accountant finds that before you do.

func ratePtr(f float64) *float64 { return &f }

// ─── The invariant ───────────────────────────────────────────────────────────

func TestSettlementNeverCreatesOrLosesACent(t *testing.T) {
	// gross = commission + net, exactly, across thousands of random inputs.
	// The database enforces the same rule in chk_settlement_amounts; this
	// catches it before a write is attempted.
	r := rand.New(rand.NewSource(42))

	for i := 0; i < 20000; i++ {
		gross := r.Int63n(50_000_00) // up to R50,000
		rate := math.Round(r.Float64()*10000) / 100

		res, err := CalculateSettlement(SettlementInput{
			GrossCents: gross, CommissionRate: ratePtr(rate),
		})
		require.NoError(t, err, "gross=%d rate=%v", gross, rate)

		assert.Equal(t, gross, res.CommissionCents+res.NetCents,
			"gross=%d rate=%v commission=%d net=%d",
			gross, rate, res.CommissionCents, res.NetCents)
		assert.GreaterOrEqual(t, res.NetCents, int64(0), "a vendor can never be asked to pay us")
		assert.GreaterOrEqual(t, res.CommissionCents, int64(0))
	}
}

// ─── The rounding decision ───────────────────────────────────────────────────

func TestSettlementCommissionIsComputedOnTheTotal(t *testing.T) {
	// The case the whole approach exists for. Per-transaction rounding drifts
	// from on-total rounding, and the drift is ARBITRARY — direction depends on
	// the rate and the price mix — so a payout computed per transaction cannot
	// be reproduced by a vendor adding up their own sales.
	sales := []int64{}
	for i := 0; i < 3000; i++ {
		sales = append(sales, []int64{3750, 4500, 2500, 6000, 8750, 12000, 1750}[i%7])
	}

	var gross, perTxn int64
	const rate = 5.0
	for _, s := range sales {
		gross += s
		perTxn += int64(math.Round(float64(s) * rate / 100))
	}

	res, err := CalculateSettlement(SettlementInput{
		GrossCents: gross, CommissionRate: ratePtr(rate),
	})
	require.NoError(t, err)

	assert.NotEqual(t, perTxn, res.CommissionCents,
		"if these ever match, the drift has gone and this rationale needs revisiting")
	t.Logf("gross R%.2f | on-total R%.2f | per-txn R%.2f | drift R%.2f",
		FromCents(gross), FromCents(res.CommissionCents), FromCents(perTxn),
		FromCents(perTxn-res.CommissionCents))
}

func TestSettlementIsReproducible(t *testing.T) {
	// A vendor adding up their own sales and applying the rate must get exactly
	// what we paid. That is the practical point of on-total rounding.
	for _, gross := range []int64{1, 99, 100, 3750, 167_940_00, 999_999_99} {
		a, err := CalculateSettlement(SettlementInput{GrossCents: gross, CommissionRate: ratePtr(5)})
		require.NoError(t, err)
		b, err := CalculateSettlement(SettlementInput{GrossCents: gross, CommissionRate: ratePtr(5)})
		require.NoError(t, err)
		assert.Equal(t, a.CommissionCents, b.CommissionCents)
	}
}

// ─── Ordinary cases ──────────────────────────────────────────────────────────

func TestSettlementExamples(t *testing.T) {
	for _, tc := range []struct {
		name             string
		gross            int64
		rate             *float64
		commission, net  int64
	}{
		{"5% of R1000", 100_000, ratePtr(5), 5_000, 95_000},
		{"no arrangement", 100_000, nil, 0, 100_000},
		{"zero rate agreed", 100_000, ratePtr(0), 0, 100_000},
		{"100% rate", 100_000, ratePtr(100), 100_000, 0},
		{"nothing sold", 0, ratePtr(5), 0, 0},
		{"one cent", 1, ratePtr(5), 0, 1},
		{"rounds to a cent", 10, ratePtr(5), 1, 9}, // 0.5c rounds up
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := CalculateSettlement(SettlementInput{
				GrossCents: tc.gross, CommissionRate: tc.rate,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.commission, res.CommissionCents)
			assert.Equal(t, tc.net, res.NetCents)
		})
	}
}

func TestSettlementNoArrangementIsNotZeroPercent(t *testing.T) {
	// Both pay the vendor everything, but they mean different things: nil is
	// "no commission was ever negotiated" (an event billed on the per-tag fee),
	// 0 is "we agreed nothing". The applied rate is recorded so a statement can
	// tell them apart.
	none, err := CalculateSettlement(SettlementInput{GrossCents: 100_000})
	require.NoError(t, err)
	zero, err := CalculateSettlement(SettlementInput{GrossCents: 100_000, CommissionRate: ratePtr(0)})
	require.NoError(t, err)

	assert.Equal(t, none.NetCents, zero.NetCents)
	assert.Nil(t, none.AppliedRate)
	assert.NotNil(t, zero.AppliedRate)
}

func TestSettlementRejectsNonsense(t *testing.T) {
	// Errors rather than clamping: a bad rate is a configuration mistake, and
	// silently correcting it hides the mistake behind a plausible number.
	for _, tc := range []struct {
		name  string
		gross int64
		rate  *float64
	}{
		{"negative gross", -1, ratePtr(5)},
		{"negative rate", 1000, ratePtr(-5)},
		{"over 100%", 1000, ratePtr(101)},
		{"NaN", 1000, ratePtr(math.NaN())},
		{"Inf", 1000, ratePtr(math.Inf(1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CalculateSettlement(SettlementInput{
				GrossCents: tc.gross, CommissionRate: tc.rate,
			})
			assert.Error(t, err)
		})
	}
}

// ─── Float reconciliation ────────────────────────────────────────────────────

func TestFloatBalancesOnACleanEvent(t *testing.T) {
	// R10,000 topped up, R6,000 spent, R2,000 already paid to vendors.
	f := CalculateFloat(FloatInput{
		TopupsCents: 1_000_000, PurchasesCents: 600_000, SettledCents: 200_000,
	})

	assert.Equal(t, int64(400_000), f.OutstandingLiabilityCents, "attendees are still owed R4,000")
	assert.Equal(t, int64(400_000), f.VendorsOwedCents, "vendors are still owed R4,000")
	assert.Equal(t, int64(800_000), f.HeldCents)
	assert.True(t, f.Balanced)
}

func TestFloatDoesNotNetLiabilitiesAgainstPayables(t *testing.T) {
	// The misreading that matters. An event holding R10,000 of unspent attendee
	// balance while owing R10,000 to vendors is NOT square — it is two debts to
	// different people that happen to be the same size, and it needs R20,000 in
	// the bank rather than nothing.
	f := CalculateFloat(FloatInput{
		TopupsCents: 2_000_000, PurchasesCents: 1_000_000,
	})
	assert.Equal(t, int64(1_000_000), f.OutstandingLiabilityCents)
	assert.Equal(t, int64(1_000_000), f.VendorsOwedCents)
	assert.Equal(t, int64(2_000_000), f.HeldCents,
		"held must be the SUM of both obligations, never the difference")
}

func TestFloatRefundsReturnValueToAttendees(t *testing.T) {
	// A vendor refund puts spendable value back in the wallet, so it increases
	// what attendees are owed and decreases what the vendor is owed.
	base := CalculateFloat(FloatInput{TopupsCents: 100_000, PurchasesCents: 50_000})
	withRefund := CalculateFloat(FloatInput{
		TopupsCents: 100_000, PurchasesCents: 50_000, RefundsCents: 10_000,
	})

	assert.Equal(t, base.OutstandingLiabilityCents+10_000, withRefund.OutstandingLiabilityCents)
	assert.Equal(t, base.VendorsOwedCents-10_000, withRefund.VendorsOwedCents)
	assert.True(t, withRefund.Balanced)
}

func TestFloatPayoutsReduceLiability(t *testing.T) {
	// Returning unspent balance to an attendee discharges the debt.
	f := CalculateFloat(FloatInput{
		TopupsCents: 100_000, PurchasesCents: 40_000,
		PayoutsCents: 60_000, SettledCents: 40_000,
	})
	assert.Equal(t, int64(0), f.OutstandingLiabilityCents)
	assert.Equal(t, int64(0), f.VendorsOwedCents)
	assert.Equal(t, int64(0), f.HeldCents, "a fully closed-out event holds nothing")
	assert.True(t, f.Balanced)
}

func TestFloatDetectsOverSettlement(t *testing.T) {
	// The one real cross-source check: SettledCents comes from
	// vendor_settlements, PurchasesCents from cashless_transactions. Paying
	// vendors R9,999.99 when only R400 was spent means money left for sales
	// nobody rang up.
	f := CalculateFloat(FloatInput{
		TopupsCents: 100_000, PurchasesCents: 40_000, SettledCents: 999_999,
	})
	assert.False(t, f.Balanced)
	assert.Equal(t, int64(959_999), f.OverSettledCents)
}

func TestFloatBalancedIsNotATautology(t *testing.T) {
	// This test exists because the original Balanced check WAS one. It summed
	// figures all derived from the same expression and asserted the sum equalled
	// itself, so it was true for every input including nonsense — 200,000
	// random cases, zero failures. A check that cannot fail is not a check.
	//
	// Guards against a future "simplification" back to that shape.
	sawTrue, sawFalse := false, false
	for _, in := range []FloatInput{
		{TopupsCents: 100_000, PurchasesCents: 40_000, SettledCents: 10_000},
		{TopupsCents: 100_000, PurchasesCents: 40_000, SettledCents: 90_000},
		{TopupsCents: 0, PurchasesCents: 0, SettledCents: 1},
	} {
		if CalculateFloat(in).Balanced {
			sawTrue = true
		} else {
			sawFalse = true
		}
	}
	assert.True(t, sawTrue, "Balanced must be able to be true")
	assert.True(t, sawFalse, "Balanced must be able to be FALSE — otherwise it checks nothing")
}

func TestFloatShortfallIsNotAnImbalance(t *testing.T) {
	// The asymmetry worth pinning. A negative attendee liability is the
	// offline shortfall — expected, owned by the recovery ladder, and NOT a
	// reconciliation failure. Only the vendor side signals inconsistency.
	f := CalculateFloat(FloatInput{TopupsCents: 100_000, PurchasesCents: 120_000})
	assert.Negative(t, f.OutstandingLiabilityCents)
	assert.True(t, f.Balanced, "a shortfall must not be reported as books that do not reconcile")
	assert.Zero(t, f.OverSettledCents)
}

func TestFloatShortfallShowsAsNegativeLiability(t *testing.T) {
	// Offline overspend: attendees spent more than they put in, so the
	// liability goes negative. Not an error — it is the shortfall, and the
	// recovery ladder owns it. Worth asserting so nobody "fixes" it by
	// clamping at zero, which would hide the loss.
	f := CalculateFloat(FloatInput{TopupsCents: 100_000, PurchasesCents: 120_000})
	assert.Equal(t, int64(-20_000), f.OutstandingLiabilityCents)
	assert.Equal(t, int64(120_000), f.VendorsOwedCents,
		"vendors are owed for real drinks regardless of whether the attendee could pay")
}