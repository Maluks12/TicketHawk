package repository

import "time"

// ── Payment mandates and shortfall recovery ───────────────────────────────────
//
// Pure. No database, no gateway.
//
// THE RULE THIS FILE ENCODES: a stored card is not consent.
//
// cashless_wallets.card_on_file records only that a top-up once succeeded. It
// says nothing about whether the holder agreed that the same card could be
// charged again, weeks later, for an amount nobody quoted them. Charging on
// that basis invites the "I didn't authorise this" dispute — which 3D Secure
// does NOT protect against, because its liability shift covers fraud, not
// disputed authorisation. The platform would eat the amount plus the chargeback
// fee.
//
// So recovery requires a mandate: a specific, recorded permission with a
// ceiling, an expiry, and the exact wording the attendee saw.
//
// WHAT THE 4a CAPS ALREADY BOUGHT. A shortfall can now only arise when a
// device's cached balance was stale-high — the holder spent online elsewhere
// while that till was dark — and its size is bounded by the device cap. So
// these are small numbers on few wallets: tens of rands, not hundreds. That
// changes the economics decisively. A surprise charge that triggers a
// chargeback costs the disputed amount plus the fee plus the relationship,
// which on a R30 shortfall is plainly worse than absorbing it. The ladder below
// is ordered accordingly: ask first, charge only with permission, write off
// rather than fight.

// Mandate statuses, mirroring the mandate_status enum in 000088.
const (
	MandateActive    = "active"
	MandateRevoked   = "revoked"
	MandateExpired   = "expired"
	MandateExhausted = "exhausted"
)

// Mandate refusal reasons.
const (
	MandateOK            = ""
	MandateNone          = "no_mandate_on_file"
	MandateNotActive     = "mandate_not_active"
	MandateHasExpired    = "mandate_expired"
	MandateOverCeiling   = "amount_exceeds_mandate_ceiling"
	MandateNoAuthCode    = "no_reusable_authorization"
	MandateInvalidAmount = "invalid_amount"
)

// Mandate is a recorded permission to charge a stored card later.
type Mandate struct {
	Status            string
	AuthorizationCode string
	MaxChargeCents    int64
	ChargedCents      int64
	ExpiresAt         time.Time
}

// AuthorizeMandateCharge decides whether a recovery charge may be made.
//
// Returns MandateOK ("") or a refusal reason. `now` is a parameter rather than
// a call to time.Now so expiry is testable and so the server and any worker
// evaluate against the same instant.
//
// The remaining-headroom check is the important one: the ceiling is what the
// attendee agreed to across the whole mandate, not per charge. Without
// subtracting what has already been charged, three R40 recoveries against a R50
// ceiling would each pass individually.
func AuthorizeMandateCharge(m *Mandate, amountCents int64, now time.Time) string {
	if amountCents <= 0 {
		return MandateInvalidAmount
	}
	if m == nil {
		return MandateNone
	}
	if m.Status != MandateActive {
		if m.Status == MandateExpired {
			return MandateHasExpired
		}
		return MandateNotActive
	}
	if !m.ExpiresAt.IsZero() && !now.Before(m.ExpiresAt) {
		// Evaluated independently of Status because a mandate that lapsed since
		// the last sweep still reads 'active' in the row. Time decides, not the
		// column.
		return MandateHasExpired
	}
	if m.AuthorizationCode == "" {
		// A consent record with nothing to act on. Possible when the gateway
		// did not return a reusable authorization for that card.
		return MandateNoAuthCode
	}
	if m.ChargedCents+amountCents > m.MaxChargeCents {
		return MandateOverCeiling
	}
	return MandateOK
}

// RemainingMandateCents is the headroom left under a mandate's ceiling. Used to
// offer a PARTIAL recovery rather than refusing outright: recovering R30 of a
// R40 shortfall under a R30 ceiling is better than recovering nothing, and it
// stays inside what the attendee agreed to.
func RemainingMandateCents(m *Mandate) int64 {
	if m == nil || m.Status != MandateActive {
		return 0
	}
	if r := m.MaxChargeCents - m.ChargedCents; r > 0 {
		return r
	}
	return 0
}

// ── Shortfall recovery ladder ────────────────────────────────────────────────

// Recovery actions. Supersedes the placeholder set in cashless_offline.go —
// ShortfallAutoCharge there assumed card_on_file was sufficient, which it is not.
const (
	RecoveryNone        = "none"
	RecoveryInvestigate = "investigate"
	RecoveryCharge      = "charge_mandate"
	RecoveryAskToSettle = "ask_to_settle"
	RecoveryManual      = "manual_review"
	RecoveryWriteOff    = "write_off"
)

// RecoveryInput is a negative wallet at reconciliation time.
type RecoveryInput struct {
	BalanceCents int64 // negative when there is a shortfall
	CardOnFile   bool
	Mandate      *Mandate

	SettleLinkSentAt *time.Time
	GraceDays        int
	DetectedAt       time.Time

	Attempts    int
	MaxAttempts int
}

// ClassifyRecovery decides what to do about a negative wallet.
//
// Order is the policy, and it is deliberately not "charge as early as possible":
//
//  1. Nothing to do.
//  2. Cash-only wallet → INVESTIGATE, never recover. §5: a correctly enforced
//     zero offline cap on a cash-funded wallet cannot produce a negative
//     balance, so one existing proves a control was bypassed. Chasing the
//     attendee for R40 is both wrong and a distraction from the actual finding.
//     No number of retries turns this into a write-off.
//  3. Retries exhausted → a human owns it. Not an automatic write-off.
//  4. No usable mandate → ASK. A settle link recovers a decent share of small
//     friendly balances, generates no disputes, and reads as competent rather
//     than sharp.
//  5. Grace elapsed after asking → write off, per a published policy.
//  6. Mandate present and sufficient → charge.
//
// Note what is NOT here: charging without a mandate, at any step, for any
// amount. That path does not exist by construction rather than by discipline.
func ClassifyRecovery(in RecoveryInput, now time.Time) (action, reason string) {
	if in.BalanceCents >= 0 {
		return RecoveryNone, ""
	}
	owed := -in.BalanceCents

	// The signal, not a debt. Checked first so nothing below can convert it
	// into an ordinary collection.
	if !in.CardOnFile {
		return RecoveryInvestigate,
			"negative balance on a wallet with no card — the offline cap was bypassed"
	}

	if in.MaxAttempts > 0 && in.Attempts >= in.MaxAttempts {
		return RecoveryManual, "recovery attempts exhausted"
	}

	// A mandate that covers the whole amount: charge it.
	if AuthorizeMandateCharge(in.Mandate, owed, now) == MandateOK {
		return RecoveryCharge, "charging under the mandate on file"
	}

	// Partial headroom is still worth taking, and stays inside what was agreed.
	if RemainingMandateCents(in.Mandate) > 0 &&
		AuthorizeMandateCharge(in.Mandate, RemainingMandateCents(in.Mandate), now) == MandateOK {
		return RecoveryCharge, "charging the remaining mandate headroom"
	}

	// Asked already and the grace period has run: write it off rather than
	// pursue a small balance. §6 commits to a published unspent-balance policy;
	// the negative side deserves the same treatment.
	if in.SettleLinkSentAt != nil && in.GraceDays > 0 {
		if !now.Before(in.SettleLinkSentAt.AddDate(0, 0, in.GraceDays)) {
			return RecoveryWriteOff, "grace period elapsed after settlement request"
		}
		return RecoveryNone, "waiting for the attendee to settle"
	}

	return RecoveryAskToSettle, "no usable mandate — request voluntary settlement"
}