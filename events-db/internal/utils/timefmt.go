package utils

import (
	"strings"
	"sync"
	"time"
)

// ── Human-facing time formatting ──────────────────────────────────────────────
//
// # WHY THIS EXISTS
//
// Every user-facing timestamp used to be rendered with a bare
// `t.Format("Monday, 2 January 2006 at 15:04")`. time.Format renders in
// whatever *time.Location the time.Time happens to carry, and in production
// that location is UTC:
//
//   - The API container is built FROM alpine with no ENV TZ set, so
//     time.Local == UTC.
//   - start_time is TIMESTAMPTZ. pgx decodes it into a time.Time built from
//     time.Unix(...), which inherits time.Local — i.e. UTC.
//
// The apps and web frontends, by contrast, render the same instant with the
// device's own locale, which for our users is SAST (UTC+2). The result was a
// booking confirmation email that stated an event start time two hours earlier
// than every other surface showed — an attendee reading only the email would
// arrive two hours early, or assume they'd missed the doors.
//
// These helpers make the display zone explicit at every call site. They operate
// on the absolute instant, so they are correct regardless of what location the
// driver, the container, or a test happens to attach to the time.Time.
//
// Rule of thumb:
//
//	Event times (start_time, end_time)  → FormatEvent*  (event's own timezone)
//	Everything else (created_at, expiry,
//	  security alerts)                  → FormatSA*     (SAST)
const (
	// EventDateTimeLayout — "Friday, 7 August 2026 at 13:23"
	EventDateTimeLayout = "Monday, 2 January 2006 at 15:04"
	// EventDateLayout — "Friday, 7 August 2026"
	EventDateLayout = "Monday, 2 January 2006"
	// EventDayLayout — "Friday, 7 August" (used in push/reminder copy)
	EventDayLayout = "Monday, 2 January"
	// ShortDateLayout — "7 August 2026"
	ShortDateLayout = "2 January 2006"
	// ShortDateTimeLayout — "7 August 2026 at 13:23"
	ShortDateTimeLayout = "2 January 2006 at 15:04"
)

var (
	saLocOnce sync.Once
	saLocVal  *time.Location

	tzCacheMu sync.RWMutex
	tzCache   = make(map[string]*time.Location)
)

// SALocation returns Africa/Johannesburg, the default display zone for
// TicketHawk. The alpine runtime image installs tzdata, so LoadLocation
// normally succeeds; the fallback exists so that a stripped image (or a test
// binary on a host without tzdata) degrades to the correct fixed offset rather
// than silently falling back to UTC. SAST is UTC+2 year-round with no DST, so
// the fixed zone is exact, not an approximation.
func SALocation() *time.Location {
	saLocOnce.Do(func() {
		if loc, err := time.LoadLocation("Africa/Johannesburg"); err == nil {
			saLocVal = loc
			return
		}
		saLocVal = time.FixedZone("SAST", 2*60*60)
	})
	return saLocVal
}

// EventLocation resolves an event's stored IANA timezone (events.timezone,
// e.g. "Africa/Johannesburg") to a *time.Location.
//
// It falls back to SAST when the name is empty or unrecognised. Empty is a
// real case, not a defensive nicety: several read queries do not select the
// timezone column, so callers legitimately pass "". Falling back to SAST is
// right for a South African platform — falling back to UTC would reintroduce
// exactly the two-hour skew this file exists to remove.
//
// Resolved locations are cached because LoadLocation reads and parses a file
// from the zoneinfo database on every call, and this runs inside per-recipient
// email loops.
func EventLocation(tz string) *time.Location {
	name := strings.TrimSpace(tz)
	if name == "" {
		return SALocation()
	}

	tzCacheMu.RLock()
	loc, ok := tzCache[name]
	tzCacheMu.RUnlock()
	if ok {
		return loc
	}

	resolved, err := time.LoadLocation(name)
	if err != nil || resolved == nil {
		resolved = SALocation()
	}

	tzCacheMu.Lock()
	tzCache[name] = resolved
	tzCacheMu.Unlock()

	return resolved
}

// FormatEventDateTime renders an event timestamp with date and time in the
// event's own timezone — "Friday, 7 August 2026 at 13:23".
func FormatEventDateTime(t time.Time, tz string) string {
	return t.In(EventLocation(tz)).Format(EventDateTimeLayout)
}

// FormatEventDate renders an event timestamp as a date in the event's own
// timezone — "Friday, 7 August 2026".
//
// Date-only output still needs the conversion: an event starting at 00:30 SAST
// is 22:30 UTC on the PREVIOUS day, so formatting in UTC reports the wrong
// calendar date entirely, not merely the wrong clock time.
func FormatEventDate(t time.Time, tz string) string {
	return t.In(EventLocation(tz)).Format(EventDateLayout)
}

// FormatEventDay renders an event timestamp without the year, for short
// notification copy — "Friday, 7 August".
func FormatEventDay(t time.Time, tz string) string {
	return t.In(EventLocation(tz)).Format(EventDayLayout)
}

// FormatSADateTime renders a non-event timestamp in SAST with date and time —
// "Friday, 7 August 2026 at 13:23". Use for claim deadlines and similar values
// that belong to the platform rather than to a specific event.
func FormatSADateTime(t time.Time) string {
	return t.In(SALocation()).Format(EventDateTimeLayout)
}

// FormatSADate renders a non-event timestamp in SAST as a date —
// "7 August 2026".
func FormatSADate(t time.Time) string {
	return t.In(SALocation()).Format(ShortDateLayout)
}

// FormatSAShortDateTime renders a non-event timestamp in SAST without the
// weekday — "7 August 2026 at 13:23". Used in security alerts, where the
// recipient is matching the moment against their own memory and a zone two
// hours out makes a legitimate change look unfamiliar.
func FormatSAShortDateTime(t time.Time) string {
	return t.In(SALocation()).Format(ShortDateTimeLayout)
}