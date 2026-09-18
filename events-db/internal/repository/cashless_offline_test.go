package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// OFFLINE SPEND TESTS — Phase 4a gate
// =============================================================================
//
//	go test ./internal/repository/... -v -run TestOffline
//	go test ./internal/repository/... -v -run TestShortfall
//
// This is the most consequential test file in the project. The code it covers
// runs on a vendor's handset during an outage, where nothing can be checked
// against a server and no decision can be undone. A gap here is money.
//
// The scenario tests at the bottom are the ones to read first: they walk the
// attacks the v4.0 revision was written to close, in the order the doc raises
// them.

func basePolicy() OfflinePolicy {
	return OfflinePolicy{
		Enabled:              true,
		MaxOfflineTaps:       3,
		MaxOfflineSpendCents: 5000, // R50
		MaxDistinctVendors:   2,
		DecaySteps: []DecayStep{
			{AfterMinutes: 10, MaxSpendCents: 2500},
			{AfterMinutes: 20, MaxSpendCents: 0},
		},
		UnknownTagAllowanceCents: 500, // R5
	}
}

func goodDevice() OfflineDeviceState {
	return OfflineDeviceState{Attested: true, OfflineEnabled: true}
}

func goodTag() OfflineTagState {
	return OfflineTagState{
		Known:              true,
		TagStatus:          "active",
		VerificationClass:  "server_verified",
		CachedBalanceCents: 20000, // R200
		WalletFunding:      "card",
		CardOnFile:         true,
	}
}

// ─── The safe default ────────────────────────────────────────────────────────

func TestOfflineZeroPolicyDeniesEverything(t *testing.T) {
	// The single most important behaviour in this file. A caller that fails to
	// load event_cashless_config and passes an empty struct must DENY, not fall
	// back to built-in caps. An organiser who never thinks about offline spend
	// gets an event where no till ever holds a key.
	assert.Equal(t, OfflineDisabled,
		AuthorizeOfflineSpend(OfflinePolicy{}, goodDevice(), goodTag(), 1000))
}

func TestOfflineDisabledPolicyDeniesEvenPerfectRequests(t *testing.T) {
	p := basePolicy()
	p.Enabled = false
	assert.Equal(t, OfflineDisabled, AuthorizeOfflineSpend(p, goodDevice(), goodTag(), 100))
}

// ─── Attestation gate ────────────────────────────────────────────────────────

func TestOfflineRequiresAttestedDevice(t *testing.T) {
	// Checked BEFORE anything about the band, because an unattested device's
	// report of the band cannot be trusted either.
	for _, d := range []OfflineDeviceState{
		{Attested: false, OfflineEnabled: true},
		{Attested: true, OfflineEnabled: false},
		{},
	} {
		assert.Equal(t, OfflineDeviceNotAttested,
			AuthorizeOfflineSpend(basePolicy(), d, goodTag(), 1000))
	}
}

func TestOfflineUnattestedDeviceCannotBypassViaBandState(t *testing.T) {
	// A compromised till reporting a perfect band still gets nowhere.
	tag := goodTag()
	tag.CachedBalanceCents = 1_000_000
	assert.Equal(t, OfflineDeviceNotAttested,
		AuthorizeOfflineSpend(basePolicy(), OfflineDeviceState{}, tag, 100))
}

// ─── The cash-wallet zero cap ────────────────────────────────────────────────

func TestOfflineCashWalletHasZeroCeiling(t *testing.T) {
	// §5. No card behind the wallet means nothing to auto-charge if it goes
	// negative, so the exposure is unrecoverable by definition.
	tag := goodTag()
	tag.CardOnFile = false
	tag.WalletFunding = "cash"

	assert.Equal(t, OfflineCashWalletZeroCap,
		AuthorizeOfflineSpend(basePolicy(), goodDevice(), tag, 100))

	t.Run("even for one cent, even with a huge balance", func(t *testing.T) {
		tag.CachedBalanceCents = 5_000_000
		assert.Equal(t, OfflineCashWalletZeroCap,
			AuthorizeOfflineSpend(basePolicy(), goodDevice(), tag, 1))
	})

	t.Run("a mixed wallet with a card may spend", func(t *testing.T) {
		tag.WalletFunding = "mixed"
		tag.CardOnFile = true
		assert.Equal(t, SpendOK,
			AuthorizeOfflineSpend(basePolicy(), goodDevice(), tag, 100))
	})
}

// ─── The decaying cap ────────────────────────────────────────────────────────

func TestOfflineDecayingCap(t *testing.T) {
	p := basePolicy()
	for _, tc := range []struct {
		minutes int
		want    int64
	}{
		{0, 5000},
		{9, 5000},
		{10, 2500}, // first rung
		{19, 2500},
		{20, 0}, // second rung — dark too long, approve nothing
		{120, 0},
	} {
		assert.Equal(t, tc.want, EffectiveDeviceCapCents(p, tc.minutes),
			"at %d minutes offline", tc.minutes)
	}
}

func TestOfflineDecayStepsSortedDefensively(t *testing.T) {
	// decay_steps is editable JSONB, so out-of-order rungs are a plausible
	// operator mistake. Supplying them backwards must not disable the decay.
	p := basePolicy()
	p.DecaySteps = []DecayStep{
		{AfterMinutes: 20, MaxSpendCents: 0},
		{AfterMinutes: 10, MaxSpendCents: 2500},
	}
	assert.Equal(t, int64(2500), EffectiveDeviceCapCents(p, 15))
	assert.Equal(t, int64(0), EffectiveDeviceCapCents(p, 25))
}

func TestOfflineStaleDeviceApprovesNothing(t *testing.T) {
	d := goodDevice()
	d.MinutesSinceSync = 30
	assert.Equal(t, OfflineStaleTooLong,
		AuthorizeOfflineSpend(basePolicy(), d, goodTag(), 100))
}

func TestOfflineDegradedConnectivityTightensRatherThanEquals(t *testing.T) {
	// The §5 v4.0 point: a device that has been dark 15 minutes must have a
	// TIGHTER ceiling than one that synced a moment ago, not an equal one.
	fresh, stale := goodDevice(), goodDevice()
	stale.MinutesSinceSync = 15

	fresh.OfflineSpentCents, stale.OfflineSpentCents = 2400, 2400
	assert.Equal(t, SpendOK, AuthorizeOfflineSpend(basePolicy(), fresh, goodTag(), 500))
	assert.Equal(t, OfflineDeviceCapReached, AuthorizeOfflineSpend(basePolicy(), stale, goodTag(), 500))
}

// ─── Per-device velocity ─────────────────────────────────────────────────────

func TestOfflineDeviceVelocityCaps(t *testing.T) {
	t.Run("tap cap", func(t *testing.T) {
		d := goodDevice()
		d.OfflineTaps = 3
		assert.Equal(t, OfflineTapCapReached,
			AuthorizeOfflineSpend(basePolicy(), d, goodTag(), 100))

		d.OfflineTaps = 2
		assert.Equal(t, SpendOK, AuthorizeOfflineSpend(basePolicy(), d, goodTag(), 100))
	})

	t.Run("spend cap is inclusive at the boundary", func(t *testing.T) {
		d := goodDevice()
		d.OfflineSpentCents = 4000
		assert.Equal(t, SpendOK, AuthorizeOfflineSpend(basePolicy(), d, goodTag(), 1000))
		assert.Equal(t, OfflineDeviceCapReached,
			AuthorizeOfflineSpend(basePolicy(), d, goodTag(), 1001))
	})
}

// ─── The on-tag accumulator: the v3.0 gap ────────────────────────────────────

func TestOfflineAccumulatorBoundsSpendAcrossDevices(t *testing.T) {
	// THE headline fix of the v4.0 revision. Per-device caps bounded spend per
	// device, so one band tapped at eight stalls during an outage multiplied the
	// ceiling by eight. The accumulator lives on the BAND, so a fresh till with
	// an empty device budget still sees what the band has already spent.
	fresh := goodDevice() // no taps, no spend — this till has seen nothing

	tag := goodTag()
	tag.CachedBalanceCents = 6000
	tag.AccumulatorCents = 5500 // already spent R55 elsewhere this outage
	tag.DistinctVendorsSoFar = 1

	assert.Equal(t, SpendInsufficient,
		AuthorizeOfflineSpend(basePolicy(), fresh, tag, 1000),
		"a fresh device must still respect what the BAND has already spent")

	// What genuinely remains is still spendable.
	assert.Equal(t, SpendOK, AuthorizeOfflineSpend(basePolicy(), fresh, tag, 500))
}

func TestOfflineDistinctVendorCap(t *testing.T) {
	tag := goodTag()
	tag.DistinctVendorsSoFar = 2

	assert.Equal(t, OfflineVendorCapReached,
		AuthorizeOfflineSpend(basePolicy(), goodDevice(), tag, 100))

	t.Run("a vendor already used does not count again", func(t *testing.T) {
		// A bar serving the same person twice is ordinary trade and must not
		// exhaust the cap.
		tag.AlreadySpentAtVendor = true
		assert.Equal(t, SpendOK, AuthorizeOfflineSpend(basePolicy(), goodDevice(), tag, 100))
	})
}

// ─── Unknown bands ───────────────────────────────────────────────────────────

func TestOfflineUnknownTagAllowance(t *testing.T) {
	tag := goodTag()
	tag.Known = false

	t.Run("small purchase within the allowance", func(t *testing.T) {
		assert.Equal(t, SpendOK, AuthorizeOfflineSpend(basePolicy(), goodDevice(), tag, 400))
	})

	t.Run("over the allowance", func(t *testing.T) {
		assert.Equal(t, OfflineUnknownTagCap,
			AuthorizeOfflineSpend(basePolicy(), goodDevice(), tag, 600))
	})

	t.Run("accumulator counts toward it", func(t *testing.T) {
		tag.AccumulatorCents = 400
		assert.Equal(t, OfflineUnknownTagCap,
			AuthorizeOfflineSpend(basePolicy(), goodDevice(), tag, 200))
	})

	t.Run("allowance of zero disables unknown bands entirely", func(t *testing.T) {
		p := basePolicy()
		p.UnknownTagAllowanceCents = 0
		tag.AccumulatorCents = 0
		assert.Equal(t, OfflineUnknownTagCap,
			AuthorizeOfflineSpend(p, goodDevice(), tag, 1))
	})
}

// ─── Band eligibility ────────────────────────────────────────────────────────

func TestOfflineBandChecks(t *testing.T) {
	t.Run("uid_only chips can never spend offline", func(t *testing.T) {
		tag := goodTag()
		tag.VerificationClass = "unverified"
		assert.Equal(t, SpendChipNotEligible,
			AuthorizeOfflineSpend(basePolicy(), goodDevice(), tag, 100))
	})

	t.Run("revoked bands", func(t *testing.T) {
		for _, s := range []string{"lost", "deactivated", "destroyed"} {
			tag := goodTag()
			tag.TagStatus = s
			assert.Equal(t, SpendTagNotActive,
				AuthorizeOfflineSpend(basePolicy(), goodDevice(), tag, 100), s)
		}
	})

	t.Run("invalid amounts", func(t *testing.T) {
		assert.Equal(t, SpendInvalidAmount,
			AuthorizeOfflineSpend(basePolicy(), goodDevice(), goodTag(), 0))
		assert.Equal(t, SpendAmountTooLarge,
			AuthorizeOfflineSpend(basePolicy(), goodDevice(), goodTag(), MaxPurchaseCents+1))
	})
}

// ─── Scenario walks ──────────────────────────────────────────────────────────

func TestOfflineScenarioRunnerAcrossVendors(t *testing.T) {
	// The "Runner" attack from §5: one band, walked from stall to stall during a
	// venue-wide outage, before any greylisting propagates. Under v3.0 this
	// multiplied the R50 ceiling by the number of stalls reached. Here the
	// accumulator and the vendor cap stop it at the third stall.
	policy := basePolicy()
	tag := goodTag()
	tag.CachedBalanceCents = 50000 // a well-funded band

	// A FRESH till each time — that is the attack: each stall's device has its own
	// empty budget, so only band-scoped state can stop the walk.
	atNextStall := func(amount int64) string {
		return AuthorizeOfflineSpend(policy, goodDevice(), tag, amount)
	}

	// Stall 1
	assert.Equal(t, SpendOK, atNextStall(2000))
	tag.AccumulatorCents += 2000
	tag.DistinctVendorsSoFar = 1

	// Stall 2
	assert.Equal(t, SpendOK, atNextStall(2000))
	tag.AccumulatorCents += 2000
	tag.DistinctVendorsSoFar = 2

	// Stall 3 — refused, despite a fresh device with an empty budget and a band
	// holding R500. The bound is on the BAND.
	assert.Equal(t, OfflineVendorCapReached, atNextStall(2000))

	assert.Equal(t, int64(4000), tag.AccumulatorCents,
		"total offline exposure for this band is bounded regardless of stall count")
}

func TestOfflineScenarioLongOutage(t *testing.T) {
	// A single till through a lengthening outage. The ceiling should tighten as
	// it goes dark, not stay flat.
	policy := basePolicy()
	device := goodDevice()
	tag := goodTag()

	// Minute 0 — full budget.
	assert.Equal(t, SpendOK, AuthorizeOfflineSpend(policy, device, tag, 2000))
	device.OfflineSpentCents = 2000
	device.OfflineTaps = 1

	// Minute 12 — cap has decayed to R25, and R20 is already spent.
	device.MinutesSinceSync = 12
	assert.Equal(t, SpendOK, AuthorizeOfflineSpend(policy, device, tag, 500))
	assert.Equal(t, OfflineDeviceCapReached, AuthorizeOfflineSpend(policy, device, tag, 600))

	// Minute 25 — dark too long, approve nothing.
	device.MinutesSinceSync = 25
	assert.Equal(t, OfflineStaleTooLong, AuthorizeOfflineSpend(policy, device, tag, 1))
}

// ─── Shortfall classification ────────────────────────────────────────────────
//
// Moved to cashless_mandate_test.go (TestRecovery*), which covers the same
// ground against ClassifyRecovery — the mandate-gated ladder that replaced
// ClassifyShortfall. See the note in cashless_offline.go.

// ─── Server/device agreement ─────────────────────────────────────────────────

func TestOfflineRecheckMatchesDeviceDecision(t *testing.T) {
	// The device and the server run the SAME function, so a disagreement means
	// the device lied or is compromised — not that two implementations drifted.
	policy, device, tag := basePolicy(), goodDevice(), goodTag()

	for _, amount := range []int64{100, 2000, 5000, 6000} {
		assert.Equal(t,
			AuthorizeOfflineSpend(policy, device, tag, amount),
			RecheckOfflineApproval(policy, device, tag, amount),
			"amount %d", amount)
	}
}