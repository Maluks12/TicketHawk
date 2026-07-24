package models

// Refund policy and reserve-sizing tests.
//
// These are pure functions with no database and no network, so they run
// everywhere and fast:
//
//	go test ./internal/models/... -run TestRefund -v
//
// They cover the money decisions that are easiest to get wrong and hardest to
// notice: band boundaries, the direction flooring rounds, whether the service
// fee escapes, and — most importantly — the invariant that ties the instant-tier
// reserve to the refund schedule. If that invariant ever breaks, the platform
// starts settling organisers money it may have to give back.

import (
	"testing"
	"time"
)

func at(daysFromNow float64) (eventStart, now time.Time) {
	now = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	eventStart = now.Add(time.Duration(daysFromNow * float64(24*time.Hour)))
	return
}

// ── Bands ─────────────────────────────────────────────────────────────────────

func TestRefundRateForDays_AllBands(t *testing.T) {
	cases := []struct {
		days int
		want float64
		why  string
	}{
		{365, 1.00, "far future"},
		{31, 1.00, "comfortably in the top band"},
		{30, 1.00, "30 is INCLUSIVE — the boundary the spec originally left in a gap"},
		{29, 0.75, "first day of the 75% band"},
		{21, 0.75, "last day of the 75% band"},
		{20, 0.50, "first day of the 50% band"},
		{14, 0.50, "last day of the 50% band"},
		{13, 0.25, "first day of the 25% band"},
		{7, 0.25, "7 still earns 25% — the cutoff is UNDER 7"},
		{6, 0.00, "inside the no-refund window"},
		{1, 0.00, "day before"},
		{0, 0.00, "same day"},
	}
	for _, c := range cases {
		if got := RefundRateForDays(c.days); got != c.want {
			t.Errorf("RefundRateForDays(%d) = %v, want %v (%s)", c.days, got, c.want, c.why)
		}
	}
}

func TestRefundSchedule_IsDescending(t *testing.T) {
	// RefundRateForDays returns the first band whose threshold is met, so an
	// out-of-order schedule silently returns the wrong rate rather than failing.
	for i := 1; i < len(RefundSchedule); i++ {
		if RefundSchedule[i].MinDaysBefore >= RefundSchedule[i-1].MinDaysBefore {
			t.Fatalf("RefundSchedule must be sorted by MinDaysBefore DESC; "+
				"index %d (%d days) is not below index %d (%d days)",
				i, RefundSchedule[i].MinDaysBefore, i-1, RefundSchedule[i-1].MinDaysBefore)
		}
	}
	if last := RefundSchedule[len(RefundSchedule)-1]; last.MinDaysBefore != 0 || last.Rate != 0 {
		t.Errorf("schedule must terminate at {0 days, 0%%}, got %+v", last)
	}
}

// ── Day arithmetic ────────────────────────────────────────────────────────────

func TestDaysUntil_FloorsAndClamps(t *testing.T) {
	cases := []struct {
		offset float64
		want   int
		why    string
	}{
		{30.0, 30, "exactly 30 days"},
		{30.99, 30, "floors down, does not round up"},
		{29.999, 29, "just under 30 lands in the 75% band, not 100%"},
		{7.5, 7, "half day floors to 7, still refundable"},
		{0.5, 0, "same-day"},
		{-1, 0, "event already started clamps to 0, never negative"},
		{-500, 0, "long past clamps to 0"},
	}
	for _, c := range cases {
		start, now := at(c.offset)
		if got := DaysUntil(start, now); got != c.want {
			t.Errorf("DaysUntil(+%.3f days) = %d, want %d (%s)", c.offset, got, c.want, c.why)
		}
	}
}

func TestDaysUntil_TimezoneIndependent(t *testing.T) {
	// The same instant expressed in SAST and UTC must produce the same band.
	// Calendar-day arithmetic would not guarantee this and would make a buyer's
	// refund depend on the wall-clock hour they happened to click.
	sast := time.FixedZone("SAST", 2*60*60)
	now := time.Date(2026, 3, 1, 23, 30, 0, 0, sast)
	start := now.Add(25 * 24 * time.Hour)

	inSAST := DaysUntil(start, now)
	inUTC := DaysUntil(start.UTC(), now.UTC())
	if inSAST != inUTC {
		t.Errorf("timezone changed the answer: SAST=%d UTC=%d", inSAST, inUTC)
	}
}

// ── Buyer quotes ──────────────────────────────────────────────────────────────

func TestQuoteBuyerRefund_ServiceFeeNeverReturned(t *testing.T) {
	// R400 subtotal + R20 service fee = R420 paid. At every refundable band the
	// buyer gets a share of the SUBTOTAL only; the fee is always retained.
	for _, days := range []float64{40, 25, 17, 10} {
		start, now := at(days)
		q := QuoteBuyerRefund(400, 20, start, now)
		if !q.Eligible {
			t.Fatalf("%v days should be refundable", days)
		}
		if q.RefundAmount > 400 {
			t.Errorf("at %v days refund %.2f exceeds subtotal — service fee leaked", days, q.RefundAmount)
		}
		if q.ServiceFeeKept != 20 {
			t.Errorf("at %v days ServiceFeeKept = %.2f, want 20", days, q.ServiceFeeKept)
		}
		if got := q.RefundAmount + q.Forfeited; got != 400 {
			t.Errorf("at %v days refund+forfeited = %.2f, want subtotal 400", days, got)
		}
	}
}

func TestQuoteBuyerRefund_AmountsPerBand(t *testing.T) {
	cases := []struct {
		days   float64
		refund float64
	}{
		{40, 400}, {30, 400}, {25, 300}, {21, 300},
		{20, 200}, {14, 200}, {13, 100}, {7, 100},
	}
	for _, c := range cases {
		start, now := at(c.days)
		q := QuoteBuyerRefund(400, 20, start, now)
		if q.RefundAmount != c.refund {
			t.Errorf("at %.0f days refund = %.2f, want %.2f", c.days, q.RefundAmount, c.refund)
		}
	}
}

func TestQuoteBuyerRefund_IneligibleCases(t *testing.T) {
	for _, days := range []float64{6, 1, 0.1} {
		start, now := at(days)
		q := QuoteBuyerRefund(400, 20, start, now)
		if q.Eligible {
			t.Errorf("at %v days quote should be ineligible", days)
		}
		if q.RefundAmount != 0 {
			t.Errorf("ineligible quote must carry zero amount, got %.2f", q.RefundAmount)
		}
		if q.Reason == "" {
			t.Error("ineligible quote must explain why")
		}
	}

	// Event already started.
	start, now := at(-2)
	q := QuoteBuyerRefund(400, 20, start, now)
	if q.Eligible || q.RefundAmount != 0 {
		t.Errorf("past event must be ineligible with zero refund, got %+v", q)
	}
}

func TestQuoteCancellationRefund_ReturnsEverything(t *testing.T) {
	// An organiser cancelling is not the buyer's fault, so the service fee comes
	// back too — the one case where the platform absorbs its own fee.
	q := QuoteCancellationRefund(400, 20)
	if !q.Eligible {
		t.Fatal("cancellation refunds are always eligible")
	}
	if q.RefundAmount != 420 {
		t.Errorf("cancellation refund = %.2f, want 420 (subtotal + service fee)", q.RefundAmount)
	}
	if q.ServiceFeeKept != 0 {
		t.Errorf("cancellation must not keep the service fee, kept %.2f", q.ServiceFeeKept)
	}
	if q.Forfeited != 0 {
		t.Errorf("buyer forfeits nothing on cancellation, got %.2f", q.Forfeited)
	}
}

// ── The invariant that protects the platform ──────────────────────────────────

func TestReserveAlwaysCoversBuyerRefund(t *testing.T) {
	// THE load-bearing property of the instant tier.
	//
	// At sale time the platform withholds ReserveRateForSale. Later the buyer
	// may refund at RefundRateAt. Because the schedule only ever falls as the
	// event nears, the reserve must be >= any refund that ticket can trigger.
	// If this ever fails, instant-tier settlement is handing out money the
	// platform may have to return from its own pocket.
	const subtotal = 400.0

	for saleDays := 0; saleDays <= 120; saleDays++ {
		saleStart, saleNow := at(float64(saleDays))
		reserve := ReserveRateForSale(saleStart, saleNow) * subtotal

		// Every later moment the buyer could choose to refund.
		for refundDays := 0; refundDays <= saleDays; refundDays++ {
			refundStart, refundNow := at(float64(refundDays))
			refund := QuoteBuyerRefund(subtotal, 20, refundStart, refundNow).RefundAmount

			if refund > reserve+0.005 {
				t.Fatalf("EXPOSURE: sold %d days out (reserve R%.2f) but a refund "+
					"%d days out returns R%.2f — platform is short R%.2f",
					saleDays, reserve, refundDays, refund, refund-reserve)
			}
		}
	}
}

func TestReserveRate_FullHoldMeansNoSplit(t *testing.T) {
	// A rate of 1.0 must be reachable, because splitForBooking treats it as the
	// signal to decline the split entirely. If the top band ever drops below
	// 1.0, far-out sales would be split while still fully refundable.
	start, now := at(45)
	if got := ReserveRateForSale(start, now); got != 1.0 {
		t.Errorf("sale 45 days out should reserve 100%%, got %v", got)
	}
	start, now = at(3)
	if got := ReserveRateForSale(start, now); got != 0.0 {
		t.Errorf("sale 3 days out is unrefundable and should reserve nothing, got %v", got)
	}
}

func TestRoundRand_HalfAwayFromZero(t *testing.T) {
	// Cent rounding shows up in every amount that reaches a gateway.
	cases := []struct{ in, want float64 }{
		{2.005, 2.01}, {2.004, 2.00}, {0.125, 0.13},
		{100.0, 100.0}, {33.333333, 33.33}, {66.666666, 66.67},
	}
	for _, c := range cases {
		if got := roundRand(c.in); got != c.want {
			t.Errorf("roundRand(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}