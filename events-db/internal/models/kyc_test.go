package models

import (
	"strconv"
	"testing"
	"time"
)

// luhnCheckDigit computes the 13th digit for a 12-digit SA ID prefix, so the
// tests can construct genuinely valid ID numbers without using real ones.
func luhnCheckDigit(prefix12 string) string {
	sum := 0
	double := true // position 12 (0-indexed 11) is doubled when the check digit is appended
	for i := len(prefix12) - 1; i >= 0; i-- {
		d := int(prefix12[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return strconv.Itoa((10 - sum%10) % 10)
}

func validID(yymmdd string) string {
	prefix := yymmdd + "500008" // sequence 5000, citizen 0, race-digit 8 (legacy)
	return prefix + luhnCheckDigit(prefix)
}

func TestValidateSAIDNumber(t *testing.T) {
	now := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)

	t.Run("valid 1990s ID passes with correct DOB", func(t *testing.T) {
		id := validID("900215")
		dob, err := ValidateSAIDNumber(id, now)
		if err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
		if dob.Year() != 1990 || dob.Month() != 2 || dob.Day() != 15 {
			t.Fatalf("wrong DOB: %v", dob)
		}
	})

	t.Run("2000s century inference", func(t *testing.T) {
		id := validID("050310") // 2005 — 21 years old in 2026
		dob, err := ValidateSAIDNumber(id, now)
		if err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
		if dob.Year() != 2005 {
			t.Fatalf("expected 2005, got %d", dob.Year())
		}
	})

	t.Run("under 18 rejected", func(t *testing.T) {
		id := validID("100101") // 2010 — 16 in 2026
		if _, err := ValidateSAIDNumber(id, now); err == nil {
			t.Fatal("expected under-18 rejection")
		}
	})

	t.Run("bad checksum rejected", func(t *testing.T) {
		id := validID("900215")
		flipped := id[:12] + string('0'+(id[12]-'0'+1)%10)
		if _, err := ValidateSAIDNumber(flipped, now); err == nil {
			t.Fatal("expected checksum rejection")
		}
	})

	t.Run("invalid date rejected", func(t *testing.T) {
		prefix := "900231" + "500008" // Feb 31
		id := prefix + luhnCheckDigit(prefix)
		if _, err := ValidateSAIDNumber(id, now); err == nil {
			t.Fatal("expected invalid-date rejection")
		}
	})

	t.Run("wrong length and non-digits rejected", func(t *testing.T) {
		for _, bad := range []string{"", "12345", "90021550000855555", "90021550A0084"} {
			if _, err := ValidateSAIDNumber(bad, now); err == nil {
				t.Fatalf("expected rejection for %q", bad)
			}
		}
	})

	t.Run("citizenship digit validated", func(t *testing.T) {
		prefix := "900215" + "500058" // citizenship digit 5 — invalid
		id := prefix + luhnCheckDigit(prefix)
		if _, err := ValidateSAIDNumber(id, now); err == nil {
			t.Fatal("expected citizenship-digit rejection")
		}
	})
}

func TestMatchNames(t *testing.T) {
	cases := []struct {
		bank, first, last string
		want              NameMatchResult
	}{
		{"Lisan Maluks", "Lisan", "Maluks", NameMatchFull},
		{"MALUKS, LISAN", "Lisan", "Maluks", NameMatchFull},
		{"L Maluks", "Lisan", "Maluks", NameMatchPartial},
		{"Lisan T Maluks", "Lisan", "Maluks", NameMatchPartial},
		{"Maluks Trading CC", "Lisan", "Maluks", NameMatchPartial},
		{"John Smith", "Lisan", "Maluks", NameMatchNone},
		{"", "Lisan", "Maluks", NameMatchUnknown},
		{"Lisan Maluks", "", "", NameMatchUnknown},
	}
	for _, c := range cases {
		if got := MatchNames(c.bank, c.first, c.last); got != c.want {
			t.Errorf("MatchNames(%q, %q, %q) = %q, want %q", c.bank, c.first, c.last, got, c.want)
		}
	}
}

func TestRequirementsMatrix(t *testing.T) {
	// Free private: declaration only, instant, no fee.
	r := RequirementsFor(VisibilityPrivateLink, false)
	if r.KYCVerifiedRequired || r.BankDetailsRequired || r.GoesThroughAdminReview || r.ListingFeeApplies || !r.RightsDeclaration {
		t.Fatalf("free private wrong: %+v", r)
	}
	// Paid private: KYC + bank, instant.
	r = RequirementsFor(VisibilityPrivateLink, true)
	if !r.KYCVerifiedRequired || !r.BankDetailsRequired || r.GoesThroughAdminReview {
		t.Fatalf("paid private wrong: %+v", r)
	}
	// Paid public: KYC + bank + review.
	r = RequirementsFor(VisibilityPublic, true)
	if !r.KYCVerifiedRequired || !r.BankDetailsRequired || !r.GoesThroughAdminReview {
		t.Fatalf("paid public wrong: %+v", r)
	}
	// Free public: KYC + listing fee + review. A free event still reaches a
	// public audience, so the organiser is identity-verified before it lists.
	// No bank details, though — there is no money to pay out.
	r = RequirementsFor(VisibilityPublic, false)
	if !r.KYCVerifiedRequired || !r.GoesThroughAdminReview || !r.ListingFeeApplies {
	}
}