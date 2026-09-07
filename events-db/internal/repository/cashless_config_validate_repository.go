package repository

import (
	"fmt"
	"sort"
)

// ── Offline policy validation ─────────────────────────────────────────────────
//
// Pure. Runs before an organiser's configuration is stored.
//
// WHY THIS IS NOT JUST INPUT VALIDATION. The caps in Phase 4a are the only
// thing that bounds loss when attestation is beaten — arithmetic rather than
// assurance. A misconfigured ladder does not fail loudly; it silently widens
// exposure while every dashboard stays green. So the checks below are less
// about rejecting nonsense and more about refusing configurations that LOOK
// deliberate but disable a control:
//
//   - a decay ladder that goes UP over time, which inverts the whole idea
//   - a per-device cap larger than any wallet could plausibly hold
//   - an unknown-tag allowance big enough to be worth farming
//
// Every limit here is a policy judgement, not a law. They are stated as
// constants with reasons so an operator who disagrees can argue with the
// reasoning rather than guessing at the intent.

const (
	// MaxConfigurableOfflineSpendCents caps the per-device offline ceiling at
	// R500. The doc's example is R50 per device. Someone setting R5,000 has
	// almost certainly typed an extra zero, and the cost of being wrong is
	// exactly the unbounded exposure v4.0 was written to close.
	MaxConfigurableOfflineSpendCents int64 = 50_000

	// MaxConfigurableTaps before a forced sync. The doc says 3. Twenty is
	// already generous; beyond it, the velocity cap has stopped meaning
	// anything.
	MaxConfigurableTaps = 20

	// MaxConfigurableVendors a single band may spend at during one outage. The
	// doc says 2. Ten distinct stalls during a single outage is not a pattern
	// any honest attendee produces.
	MaxConfigurableVendors = 10

	// MaxConfigurableUnknownAllowanceCents for a band the till has never seen.
	// R50. This is spending against a band nobody can verify, so it should
	// cover one drink and no more.
	MaxConfigurableUnknownAllowanceCents int64 = 5_000

	// MaxConfigurableDecaySteps. More than six rungs is a ladder nobody can
	// reason about, and unreasonable configuration is its own risk.
	MaxConfigurableDecaySteps = 6
)

// ValidateOfflinePolicy checks a proposed configuration and returns every
// problem found, not just the first.
//
// All of them, because an organiser fixing one error at a time through a form
// is an organiser who gives up and asks for the caps to be raised wholesale.
func ValidateOfflinePolicy(p OfflinePolicy) []string {
	var problems []string

	if p.MaxOfflineSpendCents < 0 {
		problems = append(problems, "max_offline_spend_cents cannot be negative")
	}
	if p.MaxOfflineSpendCents > MaxConfigurableOfflineSpendCents {
		problems = append(problems, fmt.Sprintf(
			"max_offline_spend_cents of %d exceeds the %d ceiling (R%.2f) — this is the amount a single till may approve with no server, and a larger figure is usually a typo",
			p.MaxOfflineSpendCents, MaxConfigurableOfflineSpendCents,
			FromCents(MaxConfigurableOfflineSpendCents)))
	}

	if p.MaxOfflineTaps < 0 {
		problems = append(problems, "max_offline_taps cannot be negative")
	}
	if p.MaxOfflineTaps > MaxConfigurableTaps {
		problems = append(problems, fmt.Sprintf(
			"max_offline_taps of %d exceeds the %d ceiling — beyond this the velocity cap stops bounding anything",
			p.MaxOfflineTaps, MaxConfigurableTaps))
	}

	if p.MaxDistinctVendors < 0 {
		problems = append(problems, "max_distinct_vendors cannot be negative")
	}
	if p.MaxDistinctVendors > MaxConfigurableVendors {
		problems = append(problems, fmt.Sprintf(
			"max_distinct_vendors of %d exceeds the %d ceiling — this is the control that stops one band being walked from stall to stall during an outage",
			p.MaxDistinctVendors, MaxConfigurableVendors))
	}

	if p.UnknownTagAllowanceCents < 0 {
		problems = append(problems, "unknown_tag_allowance_cents cannot be negative")
	}
	if p.UnknownTagAllowanceCents > MaxConfigurableUnknownAllowanceCents {
		problems = append(problems, fmt.Sprintf(
			"unknown_tag_allowance_cents of %d exceeds the %d ceiling (R%.2f) — this is spend against a band nobody can verify, so it should cover one drink at most",
			p.UnknownTagAllowanceCents, MaxConfigurableUnknownAllowanceCents,
			FromCents(MaxConfigurableUnknownAllowanceCents)))
	}

	problems = append(problems, validateDecayLadder(p)...)
	return problems
}

// validateDecayLadder is the check that matters most.
//
// The decaying cap exists so that degraded connectivity produces a TIGHTER risk
// ceiling, not an equal one. A ladder that rises over time, or that starts
// above the base cap, inverts that — and it would look perfectly deliberate in
// a config table while quietly removing the control.
func validateDecayLadder(p OfflinePolicy) []string {
	var problems []string
	if len(p.DecaySteps) == 0 {
		return nil // no decay configured is legal; the base cap simply applies throughout
	}
	if len(p.DecaySteps) > MaxConfigurableDecaySteps {
		problems = append(problems, fmt.Sprintf(
			"decay_steps has %d rungs, more than the %d allowed — a ladder nobody can reason about is its own risk",
			len(p.DecaySteps), MaxConfigurableDecaySteps))
	}

	steps := make([]DecayStep, len(p.DecaySteps))
	copy(steps, p.DecaySteps)
	sort.Slice(steps, func(i, j int) bool { return steps[i].AfterMinutes < steps[j].AfterMinutes })

	seen := map[int]bool{}
	prev := p.MaxOfflineSpendCents

	for i, s := range steps {
		if s.AfterMinutes <= 0 {
			problems = append(problems, fmt.Sprintf(
				"decay step %d has after_minutes of %d — a rung at or before zero minutes would replace the base cap entirely",
				i+1, s.AfterMinutes))
		}
		if seen[s.AfterMinutes] {
			problems = append(problems, fmt.Sprintf(
				"decay_steps has two rungs at %d minutes — which applies is then arbitrary",
				s.AfterMinutes))
		}
		seen[s.AfterMinutes] = true

		if s.MaxSpendCents < 0 {
			problems = append(problems, fmt.Sprintf(
				"decay step at %d minutes has a negative cap", s.AfterMinutes))
		}

		// THE check. Each rung must be no higher than the one before it.
		if s.MaxSpendCents > prev {
			problems = append(problems, fmt.Sprintf(
				"decay step at %d minutes RAISES the cap from %d to %d — the decaying ceiling exists so that a device offline longer may approve LESS, not more",
				s.AfterMinutes, prev, s.MaxSpendCents))
		}
		prev = s.MaxSpendCents
	}
	return problems
}