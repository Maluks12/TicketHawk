package models

import "time"

// ── Scan (admission) window ───────────────────────────────────────────────────
//
// The historical behaviour was a fixed 2-hour grace on each side of the event:
// scanning opened 2h before start_time and closed 2h after end_time. Organisers
// can now override each side independently (in minutes) via the events table's
// scan_open_offset_min / scan_close_offset_min columns.
//
// A NULL offset means "use the default" — so an event that sets neither column
// behaves exactly as it always did. This keeps one source of truth for the
// default and lets every scan path (rotating QR, offline credential mint,
// staff-code expiry) compute the same window.

// DefaultScanGrace is the fallback grace applied to either side of an event
// when the organiser hasn't set a custom offset. 2 hours, matching the
// original hardcoded value.
const DefaultScanGrace = 2 * time.Hour

// ScanWindow returns the effective [open, close] admission window for an event,
// given its start/end and the (nullable) per-side offset overrides in MINUTES.
// A nil offset falls back to DefaultScanGrace on that side.
func ScanWindow(start, end time.Time, openOffsetMin, closeOffsetMin *int) (open, close time.Time) {
	openGrace := DefaultScanGrace
	if openOffsetMin != nil {
		openGrace = time.Duration(*openOffsetMin) * time.Minute
	}
	closeGrace := DefaultScanGrace
	if closeOffsetMin != nil {
		closeGrace = time.Duration(*closeOffsetMin) * time.Minute
	}
	return start.Add(-openGrace), end.Add(closeGrace)
}

// ScanCloses returns just the close time (end + close grace). Used where only
// the upper bound matters — offline credential expiry and staff-code expiry,
// which are keyed off the end of the admission window.
func ScanCloses(end time.Time, closeOffsetMin *int) time.Time {
	closeGrace := DefaultScanGrace
	if closeOffsetMin != nil {
		closeGrace = time.Duration(*closeOffsetMin) * time.Minute
	}
	return end.Add(closeGrace)
}