package models

import "time"

// ── Scan (admission) window ───────────────────────────────────────────────────
//
// BOTH sides of the window are anchored to the event's START time:
//
//	open  = start_time - scan_open_offset_min    (default 2h)
//	close = start_time + scan_close_offset_min   (default: see below)
//
// The close side used to be anchored to end_time (end + 2h), which meant a
// nine-hour festival was still admitting people seven hours after the headliner
// and a 45-minute set held its doors open longer than the set itself ran.
//
// The DEFAULT close is deliberately not a flat offset. It lands at whichever
// comes FIRST:
//
//	start_time + 2h        — the ceiling; nobody is admitted more than two
//	                         hours into an event, however long it runs
//	end_time   - 30min     — for short events, doors shut half an hour before
//	                         the event itself is over
//
// So a 20:00–23:00 show closes at 22:00 (the ceiling), while a 20:00–22:00 show
// closes at 21:30 (30 min before the end). The crossover is a duration of
// exactly 2h30m: at or above that, the ceiling always wins.
//
// A NON-NULL scan_close_offset_min is an explicit organiser decision and is
// taken LITERALLY: 840 means "close 14 hours after the start", full stop. The
// 30-minutes-before-the-end clamp is there to pick a sensible default, not to
// overrule someone who set the window on purpose.
//
// NULL on either column means "use the default", which is why every check below
// is `!= nil` and not a truthiness test: an organiser who sets 0 means zero
// grace, not two hours.
//
// One source of truth for every scan path — rotating QR gate, unified pool/
// individual gate, offline credential mint, attendee offline pass, staff-code
// expiry. EventsStudio/lib/organiser.ts mirrors this file; change both together.

const (
	// DefaultScanGrace is the fallback offset on either side when the
	// organiser hasn't set one: scanning opens 2h before start_time, and the
	// default close can never be later than 2h after start_time.
	DefaultScanGrace = 2 * time.Hour

	// DefaultScanCloseLead is how far before end_time the default close lands
	// when the event is too short for the 2h ceiling to bite.
	DefaultScanCloseLead = 30 * time.Minute
)

// ScanCloses returns the effective admission-close time for an event.
//
// This is the single implementation of the close rule; ScanWindow and
// StaffCodeExpiry both defer to it, so there is one place to change it.
func ScanCloses(start, end time.Time, closeOffsetMin *int) time.Time {
	// Explicit organiser override — anchored at start, taken at face value.
	if closeOffsetMin != nil {
		return start.Add(time.Duration(*closeOffsetMin) * time.Minute)
	}

	// Default: 30 min before the end, capped at 2h after the start.
	ceiling := start.Add(DefaultScanGrace)
	lead := end.Add(-DefaultScanCloseLead)
	if lead.Before(ceiling) {
		return lead
	}
	return ceiling
}

// ScanWindow returns the effective [open, close] admission window for an event,
// given its start/end and the (nullable) per-side offset overrides in MINUTES.
// A nil offset falls back to the default on that side.
func ScanWindow(start, end time.Time, openOffsetMin, closeOffsetMin *int) (open, close time.Time) {
	openGrace := DefaultScanGrace
	if openOffsetMin != nil {
		openGrace = time.Duration(*openOffsetMin) * time.Minute
	}
	return start.Add(-openGrace), ScanCloses(start, end, closeOffsetMin)
}

// StaffCodeExpiry is the close time, but never earlier than the event's own end.
//
// Join codes are staff ONBOARDING, not admission. Under the start-anchored close
// a six-hour festival stops scanning at start+2h, and tying join codes to that
// would lock out a staffer arriving at hour three to work the manifest or the
// exceptions report — nothing to do with the gate. Codes therefore live at least
// as long as the event, and longer when the organiser set a long custom window.
func StaffCodeExpiry(start, end time.Time, closeOffsetMin *int) time.Time {
	close := ScanCloses(start, end, closeOffsetMin)
	if close.Before(end) {
		return end
	}
	return close
}