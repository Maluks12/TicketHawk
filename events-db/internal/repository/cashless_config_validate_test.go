package repository

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// OFFLINE CONFIG VALIDATION TESTS
// =============================================================================
//
//	go test ./internal/repository/... -v -run TestValidateOffline
//
// A misconfigured cap ladder does not fail loudly — it silently widens exposure
// while every dashboard stays green. These tests are the only thing standing
// between a plausible-looking config and a disabled control.

func validPolicy() OfflinePolicy {
	return OfflinePolicy{
		Enabled:              true,
		MaxOfflineTaps:       3,
		MaxOfflineSpendCents: 5000,
		MaxDistinctVendors:   2,
		DecaySteps: []DecayStep{
			{AfterMinutes: 10, MaxSpendCents: 2500},
			{AfterMinutes: 20, MaxSpendCents: 0},
		},
		UnknownTagAllowanceCents: 500,
	}
}

func TestValidateOfflinePolicyAcceptsTheDocumentedDefaults(t *testing.T) {
	// The §5 defaults must pass. If this ever fails, the validator has drifted
	// from the architecture it is meant to enforce.
	assert.Empty(t, ValidateOfflinePolicy(validPolicy()))
}

func TestValidateOfflinePolicyAcceptsNoDecay(t *testing.T) {
	// Legal: the base cap simply applies throughout. Tighter events may not
	// want a ladder at all.
	p := validPolicy()
	p.DecaySteps = nil
	assert.Empty(t, ValidateOfflinePolicy(p))
}

func TestValidateOfflinePolicyAcceptsAllZeros(t *testing.T) {
	// The most conservative configuration possible: offline mode on, but
	// nothing may be approved. Must be expressible — it is the setting an
	// operator reaches for when something has gone wrong mid-event.
	p := OfflinePolicy{Enabled: true}
	assert.Empty(t, ValidateOfflinePolicy(p))
}

// ─── The decay ladder ────────────────────────────────────────────────────────

func TestValidateOfflineRejectsRisingDecayLadder(t *testing.T) {
	// THE case this file exists for. A ladder that goes up inverts the entire
	// point — a device offline longer would be allowed to approve MORE — and it
	// looks perfectly deliberate sitting in a config table.
	p := validPolicy()
	p.DecaySteps = []DecayStep{
		{AfterMinutes: 10, MaxSpendCents: 2500},
		{AfterMinutes: 20, MaxSpendCents: 9000},
	}
	problems := ValidateOfflinePolicy(p)
	require.NotEmpty(t, problems)
	assert.True(t, containsSubstring(problems, "RAISES the cap"),
		"got: %v", problems)
}

func TestValidateOfflineRejectsFirstRungAboveBaseCap(t *testing.T) {
	// Subtler version: each rung descends, but the first is higher than the
	// base cap, so going offline for ten minutes INCREASES what a till may
	// approve.
	p := validPolicy()
	p.MaxOfflineSpendCents = 2000
	p.DecaySteps = []DecayStep{
		{AfterMinutes: 10, MaxSpendCents: 5000},
		{AfterMinutes: 20, MaxSpendCents: 1000},
	}
	problems := ValidateOfflinePolicy(p)
	assert.True(t, containsSubstring(problems, "RAISES the cap"), "got: %v", problems)
}

func TestValidateOfflineCatchesRiseEvenWhenStepsAreOutOfOrder(t *testing.T) {
	// decay_steps is editable JSONB, so out-of-order rungs are a plausible
	// operator mistake. Sorting before checking means a rising ladder cannot be
	// hidden by shuffling it — which would otherwise pass validation and then
	// be silently re-sorted at enforcement time.
	p := validPolicy()
	p.DecaySteps = []DecayStep{
		{AfterMinutes: 20, MaxSpendCents: 9000},
		{AfterMinutes: 10, MaxSpendCents: 2500},
	}
	assert.True(t, containsSubstring(ValidateOfflinePolicy(p), "RAISES the cap"))
}

func TestValidateOfflineRejectsDuplicateAndZeroRungs(t *testing.T) {
	p := validPolicy()
	p.DecaySteps = []DecayStep{
		{AfterMinutes: 10, MaxSpendCents: 2500},
		{AfterMinutes: 10, MaxSpendCents: 1000},
	}
	assert.True(t, containsSubstring(ValidateOfflinePolicy(p), "two rungs"))

	p.DecaySteps = []DecayStep{{AfterMinutes: 0, MaxSpendCents: 1000}}
	assert.True(t, containsSubstring(ValidateOfflinePolicy(p), "at or before zero minutes"))

	p.DecaySteps = []DecayStep{{AfterMinutes: -5, MaxSpendCents: 1000}}
	assert.True(t, containsSubstring(ValidateOfflinePolicy(p), "at or before zero minutes"))
}

func TestValidateOfflineRejectsTooManyRungs(t *testing.T) {
	p := validPolicy()
	p.MaxOfflineSpendCents = 10000
	steps := []DecayStep{}
	for i := 1; i <= 8; i++ {
		steps = append(steps, DecayStep{AfterMinutes: i * 5, MaxSpendCents: int64(10000 - i*1000)})
	}
	p.DecaySteps = steps
	assert.True(t, containsSubstring(ValidateOfflinePolicy(p), "rungs"))
}

// ─── Ceilings ────────────────────────────────────────────────────────────────

func TestValidateOfflineCeilings(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*OfflinePolicy)
		expect string
	}{
		"spend cap too high": {
			func(p *OfflinePolicy) { p.MaxOfflineSpendCents = 500_000; p.DecaySteps = nil },
			"max_offline_spend_cents",
		},
		"too many taps": {
			func(p *OfflinePolicy) { p.MaxOfflineTaps = 100 },
			"max_offline_taps",
		},
		"too many vendors": {
			func(p *OfflinePolicy) { p.MaxDistinctVendors = 50 },
			"max_distinct_vendors",
		},
		"unknown allowance too high": {
			func(p *OfflinePolicy) { p.UnknownTagAllowanceCents = 100_000 },
			"unknown_tag_allowance_cents",
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := validPolicy()
			tc.mutate(&p)
			assert.True(t, containsSubstring(ValidateOfflinePolicy(p), tc.expect),
				"got: %v", ValidateOfflinePolicy(p))
		})
	}
}

func TestValidateOfflineRejectsNegatives(t *testing.T) {
	for name, mutate := range map[string]func(*OfflinePolicy){
		"spend":     func(p *OfflinePolicy) { p.MaxOfflineSpendCents = -1 },
		"taps":      func(p *OfflinePolicy) { p.MaxOfflineTaps = -1 },
		"vendors":   func(p *OfflinePolicy) { p.MaxDistinctVendors = -1 },
		"allowance": func(p *OfflinePolicy) { p.UnknownTagAllowanceCents = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			p := validPolicy()
			mutate(&p)
			assert.NotEmpty(t, ValidateOfflinePolicy(p))
		})
	}
}

func TestValidateOfflineReportsEveryProblemAtOnce(t *testing.T) {
	// An organiser fixing one error at a time through a form is an organiser
	// who gives up and asks for the caps to be raised wholesale.
	p := OfflinePolicy{
		Enabled:                  true,
		MaxOfflineTaps:           100,
		MaxOfflineSpendCents:     500_000,
		MaxDistinctVendors:       50,
		UnknownTagAllowanceCents: 100_000,
	}
	assert.GreaterOrEqual(t, len(ValidateOfflinePolicy(p)), 4)
}

func containsSubstring(items []string, want string) bool {
	for _, s := range items {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}