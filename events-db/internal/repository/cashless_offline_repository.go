package repository

import "sort"

// ── Offline spend decision logic ──────────────────────────────────────────────
//
// Pure. No database, no clock, no network — the same shape as AuthorizeSpend
// and ClassifyTagScan, and here it matters most: this code runs on a vendor's
// handset during an outage, where nothing can be checked and nothing can be
// undone. Every rule it enforces has to be provable in a test, because the
// first time it runs for real there will be a queue and no signal.
//
// The same functions run in BOTH places. The device calls them to decide, and
// the server calls them again at reconciliation to check the device decided
// correctly. That is the point of keeping them pure: if a till approves
// something these rules forbid, the server sees the disagreement and knows the
// device is lying or compromised, rather than having two subtly different
// implementations that disagree for boring reasons.
//
// WHY CAPS ARE THE LAYER THAT MATTERS. Offline spend requires the event's tag
// key on the device, because verifying a chip with no server means holding the
// key, and with symmetric crypto a verification key is a forgery key. Policy
// gating and attestation reduce the chance a device is compromised; only the
// caps below bound what a compromised device can take. Assurance versus
// arithmetic — prefer the arithmetic.

// Offline refusal reasons, in addition to the Spend* reasons in
// cashless_ledger.go, which still apply offline.
const (
	OfflineDisabled        = "offline_spend_disabled"
	OfflineDeviceNotAttested = "device_not_attested"
	OfflineCashWalletZeroCap = "cash_wallet_no_offline_credit"
	OfflineTapCapReached   = "device_tap_cap_reached"
	OfflineDeviceCapReached = "device_spend_cap_reached"
	OfflineTagCapReached   = "tag_offline_cap_reached"
	OfflineVendorCapReached = "tag_vendor_cap_reached"
	OfflineUnknownTagCap   = "unknown_tag_allowance_exceeded"
	OfflineStaleTooLong    = "device_offline_too_long"
)

// DecayStep is one rung of the decaying ceiling: after AfterMinutes without a
// successful sync, a device may approve no more than MaxSpendCents in total.
type DecayStep struct {
	AfterMinutes  int   `json:"after_minutes"`
	MaxSpendCents int64 `json:"max_spend_cents"`
}

// OfflinePolicy is the event's configuration (event_cashless_config).
//
// The zero value is SAFE: Enabled false refuses everything. A caller that fails
// to load config and passes an empty struct denies rather than falls back to
// built-in defaults, which is why there is no NewDefaultPolicy() helper — a
// default that permits spending is exactly the mistake this shape prevents.
type OfflinePolicy struct {
	Enabled                  bool
	MaxOfflineTaps           int
	MaxOfflineSpendCents     int64
	MaxDistinctVendors       int
	DecaySteps               []DecayStep
	UnknownTagAllowanceCents int64
}

// OfflineDeviceState is what the till knows about itself.
type OfflineDeviceState struct {
	Attested          bool
	OfflineEnabled    bool
	OfflineTaps       int   // approvals since the last successful sync
	OfflineSpentCents int64 // cumulative, since the last successful sync
	MinutesSinceSync  int
}

// OfflineTagState is what the till knows about the band, from its cached
// manifest plus whatever the chip itself reports.
type OfflineTagState struct {
	Known             bool
	TagStatus         string
	VerificationClass string

	// CachedBalanceCents is the balance as of the last sync. It can only be
	// STALE-HIGH in the dangerous direction — the holder may have spent
	// elsewhere since — which is exactly what AccumulatorCents corrects for.
	CachedBalanceCents int64

	// AccumulatorCents is the §5 on-tag accumulator: cumulative offline spend
	// since the band's last online sync, read from the chip itself.
	//
	// This is the control that closes the v3.0 gap. Per-device caps bounded
	// spend per device, so one band tapped at eight stalls during an outage
	// multiplied the ceiling by eight. The accumulator lives on the BAND, so
	// every till sees the band's real offline history no matter which device
	// produced it.
	AccumulatorCents int64

	// DistinctVendorsSoFar is read from the same on-tag record.
	DistinctVendorsSoFar int
	AlreadySpentAtVendor bool

	WalletFunding string // none / cash / card / mixed
	CardOnFile    bool
}

// EffectiveDeviceCapCents applies the decaying ceiling.
//
// Steps are sorted and the LAST matching step wins, so callers cannot break the
// decay by supplying them out of order — a plausible mistake given they come
// from editable JSONB. A device inside every threshold gets the base cap.
func EffectiveDeviceCapCents(policy OfflinePolicy, minutesSinceSync int) int64 {
	cap := policy.MaxOfflineSpendCents

	steps := make([]DecayStep, len(policy.DecaySteps))
	copy(steps, policy.DecaySteps)
	sort.Slice(steps, func(i, j int) bool { return steps[i].AfterMinutes < steps[j].AfterMinutes })

	for _, s := range steps {
		if minutesSinceSync >= s.AfterMinutes {
			cap = s.MaxSpendCents
		}
	}
	if cap < 0 {
		return 0
	}
	return cap
}

// AuthorizeOfflineSpend decides one sale with no server available.
//
// Returns SpendOK ("") or a refusal reason. Order is deliberate and is not
// merely cosmetic: the categorical refusals come first so a compromised or
// ineligible device is stopped before any arithmetic is trusted, and the
// cashier sees the most actionable reason rather than the first one that
// happens to fail.
func AuthorizeOfflineSpend(
	policy OfflinePolicy,
	device OfflineDeviceState,
	tag OfflineTagState,
	amountCents int64,
) string {
	// ── Layer 1: is offline spend permitted here at all ──────────────────────
	if !policy.Enabled {
		return OfflineDisabled
	}
	if amountCents <= 0 {
		return SpendInvalidAmount
	}
	if amountCents > MaxPurchaseCents {
		return SpendAmountTooLarge
	}

	// ── Layer 2: is this device allowed to be making the decision ────────────
	// Checked before anything about the band, because an unattested device's
	// report of the band cannot be trusted either.
	if !device.Attested || !device.OfflineEnabled {
		return OfflineDeviceNotAttested
	}

	// ── Layer 3: the band ────────────────────────────────────────────────────
	if tag.VerificationClass == verifyClassUnverified {
		// A uid_only chip cannot be verified offline any more than online, and
		// offline there is no ledger to catch it afterwards.
		return SpendChipNotEligible
	}

	if !tag.Known {
		// §5 unrecognised-tag allowance. A band issued after this device's last
		// sync is indistinguishable from a forged one, so blanket rejection
		// strands legitimate attendees and blind trust is unbounded. A small
		// fixed allowance is the middle; zero disables it.
		if policy.UnknownTagAllowanceCents <= 0 ||
			tag.AccumulatorCents+amountCents > policy.UnknownTagAllowanceCents {
			return OfflineUnknownTagCap
		}
	} else {
		if tag.TagStatus != tagStatusActive {
			return SpendTagNotActive
		}
	}

	// ── Layer 4: the cash-wallet zero cap ────────────────────────────────────
	// §5. A wallet with no card behind it has nothing to auto-charge if it goes
	// negative, so its offline exposure is unrecoverable by definition. Zero is
	// the only defensible ceiling.
	//
	// This is also why a negative cash-funded wallet is treated as fraud rather
	// than as a routine shortfall later: correctly enforced, this branch makes
	// it impossible.
	if !tag.CardOnFile {
		return OfflineCashWalletZeroCap
	}

	// ── Layer 5: the band's own offline history ──────────────────────────────
	// Cached balance minus what the band has already spent offline elsewhere.
	// Using the accumulator here is what makes the cap per-BAND rather than
	// per-device.
	available := tag.CachedBalanceCents - tag.AccumulatorCents
	if available < amountCents {
		return SpendInsufficient
	}

	// The distinct-vendor cap only bites for a vendor this band has NOT already
	// used during this outage — otherwise a single stall serving the same person
	// twice would exhaust it, which is ordinary behaviour at a bar.
	if !tag.AlreadySpentAtVendor && tag.DistinctVendorsSoFar >= policy.MaxDistinctVendors {
		return OfflineVendorCapReached
	}

	// ── Layer 6: this device's own budget ────────────────────────────────────
	deviceCap := EffectiveDeviceCapCents(policy, device.MinutesSinceSync)
	if deviceCap <= 0 {
		// The decay has run to zero: this till has been dark long enough that it
		// should not be approving anything.
		return OfflineStaleTooLong
	}
	if policy.MaxOfflineTaps > 0 && device.OfflineTaps >= policy.MaxOfflineTaps {
		return OfflineTapCapReached
	}
	if device.OfflineSpentCents+amountCents > deviceCap {
		return OfflineDeviceCapReached
	}

	return SpendOK
}

// ── Shortfall reconciliation ─────────────────────────────────────────────────

// Shortfall actions, mapping to the shortfall_state enum in 000087.
const (
	ShortfallNone          = "none"
	ShortfallAutoCharge    = "auto_charge"
	ShortfallManualReview  = "manual_review"
	ShortfallInvestigate   = "investigate"
)

// ShortfallInput is a wallet found negative at reconciliation.
type ShortfallInput struct {
	BalanceCents  int64 // negative when there is a shortfall
	WalletFunding string
	CardOnFile    bool
	Attempts      int
	MaxAttempts   int
}

// ClassifyShortfall decides what to do about a negative wallet.
//
// The cash-only branch is the one worth understanding. §5 spells it out: a
// correctly enforced zero offline cap on a cash-funded wallet CANNOT produce a
// negative balance. So one existing is not a routine shortfall to retry and
// write off — it is proof that a control was bypassed, by a compromised device
// or by a bug, and the useful response is an investigation rather than a
// write-off. Treating it as ordinary would mean the platform quietly absorbs
// exactly the losses that indicate its defences are broken.
func ClassifyShortfall(in ShortfallInput) (action, reason string) {
	if in.BalanceCents >= 0 {
		return ShortfallNone, ""
	}

	// No card behind the wallet: nothing to charge, and its presence is itself
	// the signal.
	if !in.CardOnFile {
		return ShortfallInvestigate, "negative balance on a wallet with no card — offline cap was bypassed"
	}

	if in.Attempts < in.MaxAttempts {
		return ShortfallAutoCharge, "attempting recovery against the card on file"
	}

	return ShortfallManualReview, "card recovery exhausted"
}

// ── Cross-check ──────────────────────────────────────────────────────────────

// RecheckOfflineApproval re-runs a device's decision server-side at sync.
//
// A disagreement does NOT reverse the sale — the attendee already has their
// drink, and clawing back a settled transaction on the strength of a
// recomputation would punish attendees for a vendor's device. It marks the
// transaction for review instead, which is the same philosophy as accepting an
// offline transaction with a missing signature: preserve the evidence, flag the
// anomaly, let a human decide.
//
// Returns "" when the device decided as these rules would have.
func RecheckOfflineApproval(
	policy OfflinePolicy,
	device OfflineDeviceState,
	tag OfflineTagState,
	amountCents int64,
) string {
	return AuthorizeOfflineSpend(policy, device, tag, amountCents)
}