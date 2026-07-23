package models

import "time"

// ── Refund policy ─────────────────────────────────────────────────────────────
//
// A buyer-initiated refund returns a percentage of the ticket SUBTOTAL, decided
// by how far away the event still is at the moment the refund is REQUESTED:
//
//	30 days or more ....... 100%
//	21 – 29 days .......... 75%
//	14 – 20 days .......... 50%
//	 7 – 13 days .......... 25%
//	under 7 days ..........  0%   (not refundable)
//
// The buyer service fee (bookings.platform_fee) is never returned on a
// buyer-initiated refund — it pays for a transaction that already happened and
// cannot be undone. The organiser commission (bookings.organizer_fee) is
// charged on GROSS SOLD, so it is not reduced when a partial refund occurs.
//
// An organiser-initiated CANCELLATION is a different thing entirely: the buyer
// did nothing wrong, so they get 100% back INCLUDING the service fee,
// regardless of how close the event is. See QuoteCancellationRefund.
//
// ── Why this file also decides the instant-tier reserve ───────────────────────
//
// For an instant-tier organiser, Paystack settles their share directly at the
// time of sale — that money is gone from the platform's control within ~2 days.
// If a buyer later refunds, the platform must fund it alone. So the amount
// withheld at sale (the "reserve") has to cover the largest refund that ticket
// could ever trigger.
//
// The refund rate only ever DECREASES as the event approaches, so a ticket's
// maximum future liability is fixed the moment it is sold: it is exactly the
// refund rate applicable on that day. ReserveRateForSale therefore returns the
// same schedule, which bounds platform exposure at zero without any clawback
// machinery. A ticket sold 60 days out reserves 100% (so it is not split at
// all and settles through the ledger like held tier); a ticket sold 3 days out
// reserves nothing and settles to the organiser instantly and in full.

// RefundBand is one row of the schedule. MinDaysBefore is an inclusive lower
// bound: a booking exactly 30 days from the event start gets the 30-day rate.
type RefundBand struct {
	MinDaysBefore int
	Rate          float64
}

// RefundSchedule MUST stay sorted by MinDaysBefore descending — RefundRateForDays
// returns the first band whose threshold is met and relies on that ordering.
var RefundSchedule = []RefundBand{
	{MinDaysBefore: 30, Rate: 1.00},
	{MinDaysBefore: 21, Rate: 0.75},
	{MinDaysBefore: 14, Rate: 0.50},
	{MinDaysBefore: 7, Rate: 0.25},
	{MinDaysBefore: 0, Rate: 0.00},
}

// DaysUntil returns whole days between now and start, floored, never negative.
//
// Both instants are normalised to UTC and compared as absolute durations rather
// than calendar dates. That is deliberate: calendar-day arithmetic in SAST would
// make the band a buyer lands in depend on the wall-clock hour they happened to
// click, and would drift if the platform ever serves a second timezone. Absolute
// duration is deterministic, matches how the database stores timestamps, and is
// trivially testable.
//
// Flooring favours the platform at the boundary by at most a few hours, which is
// why the bands themselves are inclusive at the bottom — 30 days and 1 minute
// and 30 days and 23 hours both floor to 30 and both get 100%.
func DaysUntil(start, now time.Time) int {
	remaining := start.UTC().Sub(now.UTC())
	if remaining <= 0 {
		return 0
	}
	return int(remaining / (24 * time.Hour))
}

// RefundRateForDays maps whole days-until-event to the fraction of subtotal a
// buyer gets back. Returns 0 for anything under the lowest band.
func RefundRateForDays(days int) float64 {
	for _, band := range RefundSchedule {
		if days >= band.MinDaysBefore {
			return band.Rate
		}
	}
	return 0
}

// RefundRateAt is the convenience form: how much of the subtotal a buyer would
// get back if they requested a refund at `now` for an event starting at
// `eventStart`.
func RefundRateAt(eventStart, now time.Time) float64 {
	return RefundRateForDays(DaysUntil(eventStart, now))
}

// ReserveRateForSale returns the fraction of subtotal that must be withheld from
// an instant-tier organiser for a ticket sold at `saleTime` for an event
// starting at `eventStart`.
//
// This is the same schedule as the refund rate, and that is the whole point —
// see the file header. A rate of 1.0 means the sale must not be split at all.
func ReserveRateForSale(eventStart, saleTime time.Time) float64 {
	return RefundRateAt(eventStart, saleTime)
}

// ── Quotes ────────────────────────────────────────────────────────────────────

// RefundQuote is what a buyer is told before they confirm, and what the refund
// path then executes. Amounts are in the booking's currency (ZAR).
type RefundQuote struct {
	Eligible       bool    `json:"eligible"`
	Rate           float64 `json:"rate"`             // fraction of subtotal, 0.0–1.0
	DaysUntilEvent int     `json:"days_until_event"` // whole days, floored
	RefundAmount   float64 `json:"refund_amount"`    // actually returned to the buyer
	ServiceFeeKept float64 `json:"service_fee_kept"` // non-refundable, buyer-initiated only
	Forfeited      float64 `json:"forfeited"`        // subtotal the buyer does not get back
	Reason         string  `json:"reason,omitempty"` // set when Eligible is false
}

// QuoteBuyerRefund prices a buyer-initiated refund. It never returns the service
// fee. A quote with Eligible false must not be executed — RefundAmount is zero
// and Reason explains why.
func QuoteBuyerRefund(subtotal, serviceFee float64, eventStart, now time.Time) RefundQuote {
	days := DaysUntil(eventStart, now)
	rate := RefundRateForDays(days)

	q := RefundQuote{
		Rate:           rate,
		DaysUntilEvent: days,
		ServiceFeeKept: roundRand(serviceFee),
	}

	if !eventStart.UTC().After(now.UTC()) {
		q.Reason = "The event has already started, so this booking can no longer be refunded."
		q.Forfeited = roundRand(subtotal)
		return q
	}
	if rate <= 0 {
		q.Reason = "Refunds are not available within 7 days of the event."
		q.Forfeited = roundRand(subtotal)
		return q
	}

	q.Eligible = true
	q.RefundAmount = roundRand(rate * subtotal)
	q.Forfeited = roundRand(subtotal - q.RefundAmount)
	return q
}

// QuoteCancellationRefund prices an organiser-initiated cancellation: the buyer
// is made whole, service fee included, whatever the timing. The platform absorbs
// its own fee here rather than charging a buyer for someone else's decision.
func QuoteCancellationRefund(subtotal, serviceFee float64) RefundQuote {
	return RefundQuote{
		Eligible:       true,
		Rate:           1.0,
		DaysUntilEvent: 0,
		RefundAmount:   roundRand(subtotal + serviceFee),
		ServiceFeeKept: 0,
		Forfeited:      0,
	}
}

// roundRand rounds to whole cents, half away from zero. Kept private and local
// so models has no dependency on the services package.
func roundRand(v float64) float64 {
	if v < 0 {
		return -float64(int64(-v*100+0.5)) / 100
	}
	return float64(int64(v*100+0.5)) / 100
}