package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// MANDATE & RECOVERY TESTS — Phase 4b gate
// =============================================================================
//
//	go test ./internal/repository/... -v -run "TestMandate|TestRecovery"
//
// The property being defended: there is NO path from a negative wallet to a
// card charge that does not pass through a recorded mandate. Several tests
// below exist only to prove that path stays closed.

var testNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func activeMandate() *Mandate {
	return &Mandate{
		Status:            MandateActive,
		AuthorizationCode: "AUTH_abc123",
		MaxChargeCents:    10000, // R100 ceiling
		ChargedCents:      0,
		ExpiresAt:         testNow.AddDate(0, 3, 0),
	}
}

// ─── Mandate authorisation ───────────────────────────────────────────────────

func TestMandateAllowsChargeWithinCeiling(t *testing.T) {
	assert.Equal(t, MandateOK, AuthorizeMandateCharge(activeMandate(), 5000, testNow))
	assert.Equal(t, MandateOK, AuthorizeMandateCharge(activeMandate(), 10000, testNow),
		"exactly the ceiling is allowed")
}

func TestMandateRefusesOverCeiling(t *testing.T) {
	assert.Equal(t, MandateOverCeiling,
		AuthorizeMandateCharge(activeMandate(), 10001, testNow))
}

func TestMandateCeilingIsCumulative(t *testing.T) {
	// The ceiling is what the attendee agreed to across the WHOLE mandate, not
	// per charge. Without subtracting what has already been charged, three R40
	// recoveries against a R50 ceiling would each pass on their own.
	m := activeMandate()
	m.ChargedCents = 8000

	assert.Equal(t, MandateOK, AuthorizeMandateCharge(m, 2000, testNow))
	assert.Equal(t, MandateOverCeiling, AuthorizeMandateCharge(m, 2001, testNow))
	assert.Equal(t, int64(2000), RemainingMandateCents(m))
}

func TestMandateExpiryIsDecidedByTimeNotStatus(t *testing.T) {
	// A mandate that lapsed since the last sweep still reads 'active' in its
	// row. Time decides, not the column — otherwise a stale sweeper is the only
	// thing standing between a lapsed consent and a charge.
	m := activeMandate()
	m.ExpiresAt = testNow.Add(-time.Hour)
	assert.Equal(t, MandateHasExpired, AuthorizeMandateCharge(m, 100, testNow))

	m.ExpiresAt = testNow
	assert.Equal(t, MandateHasExpired, AuthorizeMandateCharge(m, 100, testNow),
		"expiry is exclusive — a mandate is dead at its expiry instant")
}

func TestMandateRefusals(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		assert.Equal(t, MandateNone, AuthorizeMandateCharge(nil, 100, testNow))
	})

	t.Run("revoked or exhausted", func(t *testing.T) {
		for _, s := range []string{MandateRevoked, MandateExhausted, "something_new"} {
			m := activeMandate()
			m.Status = s
			assert.Equal(t, MandateNotActive, AuthorizeMandateCharge(m, 100, testNow), s)
		}
	})

	t.Run("consent recorded but nothing to charge against", func(t *testing.T) {
		// Possible when the gateway returned no reusable authorization for the
		// card. The consent is real; it is simply not actionable.
		m := activeMandate()
		m.AuthorizationCode = ""
		assert.Equal(t, MandateNoAuthCode, AuthorizeMandateCharge(m, 100, testNow))
	})

	t.Run("invalid amounts", func(t *testing.T) {
		assert.Equal(t, MandateInvalidAmount, AuthorizeMandateCharge(activeMandate(), 0, testNow))
		assert.Equal(t, MandateInvalidAmount, AuthorizeMandateCharge(activeMandate(), -100, testNow))
	})
}

func TestRemainingMandateCents(t *testing.T) {
	assert.Equal(t, int64(0), RemainingMandateCents(nil))

	m := activeMandate()
	m.Status = MandateRevoked
	assert.Equal(t, int64(0), RemainingMandateCents(m), "a dead mandate has no headroom")

	m = activeMandate()
	m.ChargedCents = 10000
	assert.Equal(t, int64(0), RemainingMandateCents(m))
}

// ─── The recovery ladder ─────────────────────────────────────────────────────

func TestRecoveryHealthyWallet(t *testing.T) {
	for _, bal := range []int64{0, 1, 50000} {
		action, _ := ClassifyRecovery(RecoveryInput{
			BalanceCents: bal, CardOnFile: true, Mandate: activeMandate(),
		}, testNow)
		assert.Equal(t, RecoveryNone, action)
	}
}

func TestRecoveryNeverChargesWithoutAMandate(t *testing.T) {
	// THE property of this file. A card on file is not consent, so a wallet
	// with a card and no mandate must be ASKED, never charged.
	action, _ := ClassifyRecovery(RecoveryInput{
		BalanceCents: -4000, CardOnFile: true, Mandate: nil, MaxAttempts: 3,
	}, testNow)
	assert.Equal(t, RecoveryAskToSettle, action)

	t.Run("nor with a dead one", func(t *testing.T) {
		for _, s := range []string{MandateRevoked, MandateExpired, MandateExhausted} {
			m := activeMandate()
			m.Status = s
			action, _ := ClassifyRecovery(RecoveryInput{
				BalanceCents: -4000, CardOnFile: true, Mandate: m, MaxAttempts: 3,
			}, testNow)
			assert.Equal(t, RecoveryAskToSettle, action, s)
		}
	})

	t.Run("nor with an expired one that still reads active", func(t *testing.T) {
		m := activeMandate()
		m.ExpiresAt = testNow.Add(-time.Minute)
		action, _ := ClassifyRecovery(RecoveryInput{
			BalanceCents: -4000, CardOnFile: true, Mandate: m, MaxAttempts: 3,
		}, testNow)
		assert.Equal(t, RecoveryAskToSettle, action)
	})

	t.Run("nor when the amount exceeds the agreed ceiling", func(t *testing.T) {
		m := activeMandate()
		m.MaxChargeCents = 1000
		m.ChargedCents = 1000 // no headroom left at all
		action, _ := ClassifyRecovery(RecoveryInput{
			BalanceCents: -4000, CardOnFile: true, Mandate: m, MaxAttempts: 3,
		}, testNow)
		assert.Equal(t, RecoveryAskToSettle, action)
	})
}

func TestRecoveryChargesUnderAValidMandate(t *testing.T) {
	action, _ := ClassifyRecovery(RecoveryInput{
		BalanceCents: -4000, CardOnFile: true, Mandate: activeMandate(), MaxAttempts: 3,
	}, testNow)
	assert.Equal(t, RecoveryCharge, action)
}

func TestRecoveryTakesPartialHeadroom(t *testing.T) {
	// Recovering R30 of a R40 shortfall under a R30 ceiling beats recovering
	// nothing, and stays inside what was agreed.
	m := activeMandate()
	m.MaxChargeCents = 3000
	action, _ := ClassifyRecovery(RecoveryInput{
		BalanceCents: -4000, CardOnFile: true, Mandate: m, MaxAttempts: 3,
	}, testNow)
	assert.Equal(t, RecoveryCharge, action)
	assert.Equal(t, int64(3000), RemainingMandateCents(m))
}

func TestRecoveryCashWalletIsAlwaysInvestigated(t *testing.T) {
	// §5. A correctly enforced zero offline cap on a cash-funded wallet cannot
	// produce a negative balance, so one existing proves a control was
	// bypassed. It is a signal, not a debt — and nothing below may convert it
	// into an ordinary collection.
	action, reason := ClassifyRecovery(RecoveryInput{
		BalanceCents: -100, CardOnFile: false, MaxAttempts: 3,
	}, testNow)
	assert.Equal(t, RecoveryInvestigate, action)
	assert.Contains(t, reason, "bypassed")

	t.Run("a mandate does not override it", func(t *testing.T) {
		// Belt and braces: even if a mandate somehow exists on a cash wallet,
		// the investigation wins.
		action, _ := ClassifyRecovery(RecoveryInput{
			BalanceCents: -100, CardOnFile: false, Mandate: activeMandate(), MaxAttempts: 3,
		}, testNow)
		assert.Equal(t, RecoveryInvestigate, action)
	})

	t.Run("no number of attempts turns it into anything else", func(t *testing.T) {
		for _, n := range []int{0, 3, 99} {
			action, _ := ClassifyRecovery(RecoveryInput{
				BalanceCents: -100, CardOnFile: false, Attempts: n, MaxAttempts: 3,
			}, testNow)
			assert.Equal(t, RecoveryInvestigate, action)
		}
	})
}

func TestRecoveryExhaustedAttemptsGoToAHuman(t *testing.T) {
	// Not an automatic write-off, and not endless retrying either.
	action, _ := ClassifyRecovery(RecoveryInput{
		BalanceCents: -4000, CardOnFile: true, Mandate: activeMandate(),
		Attempts: 3, MaxAttempts: 3,
	}, testNow)
	assert.Equal(t, RecoveryManual, action)
}

func TestRecoverySettlementWindow(t *testing.T) {
	sent := testNow.AddDate(0, 0, -3)

	t.Run("inside the grace period we wait", func(t *testing.T) {
		action, _ := ClassifyRecovery(RecoveryInput{
			BalanceCents: -4000, CardOnFile: true, Mandate: nil,
			SettleLinkSentAt: &sent, GraceDays: 14, MaxAttempts: 3,
		}, testNow)
		assert.Equal(t, RecoveryNone, action)
	})

	t.Run("after it, write off rather than pursue", func(t *testing.T) {
		old := testNow.AddDate(0, 0, -20)
		action, reason := ClassifyRecovery(RecoveryInput{
			BalanceCents: -4000, CardOnFile: true, Mandate: nil,
			SettleLinkSentAt: &old, GraceDays: 14, MaxAttempts: 3,
		}, testNow)
		assert.Equal(t, RecoveryWriteOff, action)
		assert.Contains(t, reason, "grace period")
	})

	t.Run("zero grace days means we keep waiting rather than write off instantly", func(t *testing.T) {
		action, _ := ClassifyRecovery(RecoveryInput{
			BalanceCents: -4000, CardOnFile: true, Mandate: nil,
			SettleLinkSentAt: &sent, GraceDays: 0, MaxAttempts: 3,
		}, testNow)
		assert.Equal(t, RecoveryAskToSettle, action,
			"an unconfigured grace period must not silently forgive balances")
	})
}

func TestRecoveryLadderOrdering(t *testing.T) {
	// The ladder as one walk, showing that each rung is reached only when the
	// one above genuinely does not apply.
	m := activeMandate()

	// Mandate present and sufficient → charge, without ever asking.
	a, _ := ClassifyRecovery(RecoveryInput{
		BalanceCents: -2000, CardOnFile: true, Mandate: m, MaxAttempts: 3,
	}, testNow)
	assert.Equal(t, RecoveryCharge, a)

	// Same wallet, mandate now exhausted → ask, do not charge.
	m.ChargedCents = m.MaxChargeCents
	a, _ = ClassifyRecovery(RecoveryInput{
		BalanceCents: -2000, CardOnFile: true, Mandate: m, MaxAttempts: 3,
	}, testNow)
	assert.Equal(t, RecoveryAskToSettle, a)

	// Asked, grace elapsed → write off.
	old := testNow.AddDate(0, 0, -30)
	a, _ = ClassifyRecovery(RecoveryInput{
		BalanceCents: -2000, CardOnFile: true, Mandate: m,
		SettleLinkSentAt: &old, GraceDays: 14, MaxAttempts: 3,
	}, testNow)
	assert.Equal(t, RecoveryWriteOff, a)
}