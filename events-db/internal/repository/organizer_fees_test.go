package repository

import (
	"errors"
	"testing"
)

func f64(v float64) *float64 { return &v }

func TestCalculateFees_Defaults(t *testing.T) {
	cases := []struct {
		name              string
		subtotal          float64
		qty               int
		wantSvc, wantComm float64
	}{
		{"R200 x1", 200, 1, 10, 10},      // 5% of 200
		{"R150 x2", 300, 2, 20, 15},      // R10 each, 5% of 300
		{"R30 x1 capped", 30, 1, 6, 1.5}, // 20% cap bites below R50
		{"free", 0, 3, 0, 0},
	}
	for _, c := range cases {
		svc, comm := CalculateFees(c.subtotal, c.qty, DefaultFeeTerms())
		if svc != c.wantSvc || comm != c.wantComm {
			t.Errorf("%s: got svc=%.2f comm=%.2f, want svc=%.2f comm=%.2f", c.name, svc, comm, c.wantSvc, c.wantComm)
		}
	}
}

func TestCalculateFees_CustomTerms(t *testing.T) {
	terms := FeeTerms{ServiceFeePerTicket: 5, CommissionRate: 0.045}
	svc, comm := CalculateFees(200, 1, terms)
	if svc != 5 || comm != 9 {
		t.Fatalf("got svc=%.2f comm=%.2f, want 5 and 9", svc, comm)
	}
	// The 20% cap still applies to a custom service fee.
	svc, _ = CalculateFees(20, 1, terms)
	if svc != 4 {
		t.Fatalf("cap: got svc=%.2f, want 4", svc)
	}
}

func TestValidateFeeOverride(t *testing.T) {
	cases := []struct {
		name string
		in   FeeOverrideInput
		want error
	}{
		{"reset to defaults", FeeOverrideInput{}, nil},
		{"valid deal", FeeOverrideInput{CommissionPct: f64(4.5), ServiceFee: f64(5), Note: "launch deal"}, nil},
		{"floor ok", FeeOverrideInput{CommissionPct: f64(4), Note: "floor"}, nil},
		{"below floor", FeeOverrideInput{CommissionPct: f64(3.99), Note: "too low"}, ErrFeeCommissionRange},
		{"above ceiling", FeeOverrideInput{CommissionPct: f64(10.01), Note: "too high"}, ErrFeeCommissionRange},
		{"service at cap", FeeOverrideInput{ServiceFee: f64(10), Note: "cap"}, nil},
		{"service over cap", FeeOverrideInput{ServiceFee: f64(10.5), Note: "over"}, ErrFeeServiceFeeRange},
		{"service negative", FeeOverrideInput{ServiceFee: f64(-1), Note: "neg"}, ErrFeeServiceFeeRange},
		{"missing note", FeeOverrideInput{CommissionPct: f64(4.5)}, ErrFeeNoteRequired},
	}
	for _, c := range cases {
		if got := ValidateFeeOverride(c.in); !errors.Is(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}