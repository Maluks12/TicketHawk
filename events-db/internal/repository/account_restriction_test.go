package repository

import (
	"errors"
	"strings"
	"testing"

	"events-db/internal/models"
)

func intp(v int) *int { return &v }

func TestPlanRestrictionRules(t *testing.T) {
	ok := "Chargeback fraud on three events"
	cases := []struct {
		name string
		in   RestrictionInput
		err  error
	}{
		{"disable needs days", RestrictionInput{Action: "disable", Reason: ok}, ErrRestrictionDays},
		{"disable zero days", RestrictionInput{Action: "disable", Days: intp(0), Reason: ok}, ErrRestrictionDays},
		{"disable too long", RestrictionInput{Action: "disable", Days: intp(models.MaxRestrictionDays + 1), Reason: ok}, ErrRestrictionDays},
		{"disable ok", RestrictionInput{Action: "disable", Days: intp(7), Reason: ok}, nil},
		{"ban ignores days", RestrictionInput{Action: "ban", Days: intp(3), Reason: ok}, nil},
		{"block needs a feature", RestrictionInput{Action: "block", Reason: ok}, ErrRestrictionNoFeatures},
		{"block rejects a lockout kind", RestrictionInput{Action: "block", Features: []models.RestrictionKind{models.RestrictBan}, Reason: ok}, ErrRestrictionFeature},
		{"block rejects negative days", RestrictionInput{Action: "block", Days: intp(-1), Features: []models.RestrictionKind{models.RestrictPayouts}, Reason: ok}, ErrRestrictionDays},
		{"block indefinite", RestrictionInput{Action: "block", Features: []models.RestrictionKind{models.RestrictPayouts}, Reason: ok}, nil},
		{"reason too short", RestrictionInput{Action: "ban", Reason: " abc "}, ErrRestrictionReason},
		{"reason too long", RestrictionInput{Action: "ban", Reason: strings.Repeat("x", models.MaxRestrictionReasonLen+1)}, ErrRestrictionReason},
		{"unknown action", RestrictionInput{Action: "suspend", Reason: ok}, ErrRestrictionAction},
	}
	for _, c := range cases {
		_, err := planRestriction(c.in)
		if !errors.Is(err, c.err) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.err)
		}
	}
}

func TestPlanRestrictionShapes(t *testing.T) {
	p, err := planRestriction(RestrictionInput{Action: "ban", Days: intp(9), Reason: "Repeated ticket fraud"})
	if err != nil {
		t.Fatal(err)
	}
	if p.days != nil {
		t.Error("a ban is always indefinite, whatever days were sent")
	}
	if len(p.replaces) != 2 {
		t.Errorf("a ban replaces both lockout kinds, got %v", p.replaces)
	}

	p, err = planRestriction(RestrictionInput{
		Action:   "block",
		Days:     intp(0),
		Features: []models.RestrictionKind{models.RestrictPayouts, models.RestrictPayouts, models.RestrictBuyTickets},
		Reason:   "Disputed payouts under review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.days != nil {
		t.Error("days 0 on a block means indefinite")
	}
	if len(p.kinds) != 2 {
		t.Errorf("duplicate features collapse to one row each, got %v", p.kinds)
	}
	for _, k := range p.replaces {
		if k.IsLockout() {
			t.Errorf("a feature block must never replace a lockout, got %v", p.replaces)
		}
	}
}

func TestAccountStateSeverity(t *testing.T) {
	rec := func(k models.RestrictionKind) RestrictionRecord { return RestrictionRecord{Kind: string(k)} }
	if got := accountState(nil); got != "active" {
		t.Errorf("no restrictions: %q", got)
	}
	if got := accountState([]RestrictionRecord{rec(models.RestrictPayouts)}); got != "limited" {
		t.Errorf("feature block: %q", got)
	}
	if got := accountState([]RestrictionRecord{rec(models.RestrictPayouts), rec(models.RestrictDisable)}); got != "disabled" {
		t.Errorf("disable outranks a block: %q", got)
	}
	if got := accountState([]RestrictionRecord{rec(models.RestrictDisable), rec(models.RestrictBan)}); got != "banned" {
		t.Errorf("ban outranks everything: %q", got)
	}
}