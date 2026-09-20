package models

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// ── Event detail change notifications ────────────────────────────────────────
//
// A live event's details change by several routes: the organiser's direct
// Information edit (PUT /organiser/events/:id/info — no review, on purpose),
// an approved edit request (title, description, times; venue and date from the
// admin console), and a draft edit while an event is taken down.
//
// None of those told the people holding tickets. This file decides WHAT counts
// as a change worth telling them about and how to say it. The worker in
// workers/event_details_change_worker.go decides WHEN.
//
// Deliberately NOT watched:
//   - the line-up — it has its own notice (event_lineup_change.go)
//   - images and tags — presentation, not something anyone acts on
//   - ticket tiers — price and capacity edits apply to future sales only; an
//     existing booking keeps what it paid (see applyTicketTypeEdits)

// NotifEventDetailsChanged goes to every account holding a confirmed ticket
// when a live event's date, time, venue, name, description or Information
// changes. Guests, and people holding a ticket someone assigned to them, get
// the same notice by email.
const NotifEventDetailsChanged NotificationType = "event_details_changed"

// EventDetailsSnapshot is what ticket holders were last told about an event.
//
// Built in SQL (repository.detailsSnapshotSQL and the 000092 backfill), never
// in Go, so a stored snapshot can be compared with a fresh one by plain jsonb
// equality before any of the judgement below runs.
//
// Times are unix seconds rather than timestamps: jsonb renders a timestamp in
// the session's TimeZone, so two sessions with different settings would read
// the same instant as a change.
//
// The description is a hash of its whitespace- and case-normalised text. It is
// never quoted to anyone, only detected, and a hash keeps a long description
// out of a row the worker reads every cycle.
type EventDetailsSnapshot struct {
	Title    string            `json:"title"`
	DescHash string            `json:"desc"`
	Start    int64             `json:"start"`
	End      int64             `json:"end"`
	Timezone string            `json:"tz"`
	Venue    string            `json:"venue"`
	Address  string            `json:"address"`
	City     string            `json:"city"`
	Info     map[string]string `json:"info"`
}

// Kinds of change.
const (
	DetailChanged = "changed"
	DetailAdded   = "added"
	DetailRemoved = "removed"
)

// Field keys. Information fields are DetailFieldInfoPrefix + catalogue key.
const (
	DetailFieldWhen        = "when"
	DetailFieldVenue       = "venue"
	DetailFieldTitle       = "title"
	DetailFieldDescription = "description"
	DetailFieldInfoPrefix  = "info:"
)

// EventDetailChange is one line of "what changed".
type EventDetailChange struct {
	Field string `json:"field"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	// Old and New are display text. Both empty for the description, which is
	// only ever reported as updated.
	Old string `json:"old,omitempty"`
	New string `json:"new,omitempty"`
	// Short is the compact form a push body quotes: "18+", "19:00", "The
	// Warehouse", "Sat 14 Nov, 19:00". Empty means the value is too long to
	// quote, so the push says the field was updated and the email carries the
	// full text.
	Short string `json:"short,omitempty"`
}

// EventDetailChanges is ordered most consequential first — date and time,
// venue, name, Information in catalogue order, description last — because an
// OS may cut a push body off, and what gets cut should be the least important.
type EventDetailChanges []EventDetailChange

// WhenFormatter renders an event's start and end in the event's own timezone:
// full (start and end) for the email, short (start only) for a push body.
// Injected so this package stays free of utils and tests can pin the output.
type WhenFormatter func(start, end time.Time, tz string) (full, short string)

// maxQuotableRunes — longer values are reported as "updated" in a push body
// rather than quoted, and read in full in the email.
const maxQuotableRunes = 32

// maxBodyParts — how many changes a push body names before summarising.
const maxBodyParts = 3

// DiffEventDetails reports what changed between announced and current.
//
// Text comparisons ignore case and runs of whitespace. Correcting "the
// warehouse" to "The Warehouse", or fixing a double space in a parking note, is
// not worth a push to every ticket holder — that is the kind of notification
// that gets notifications switched off, which costs us the one about the venue
// moving.
//
// Date and time compare as the formatted local text, minute precision, not as
// raw instants: a seconds-level difference is invisible to everyone, while a
// corrected timezone that moves the local clock time is exactly what a ticket
// holder needs to hear.
func DiffEventDetails(
	announced, current EventDetailsSnapshot,
	catalogue map[string]EventInfoCatalogueEntry,
	formatWhen WhenFormatter,
) EventDetailChanges {
	var out EventDetailChanges

	// ── Date & time ─────────────────────────────────────────────────────────
	oldFull, oldShort := formatWhen(time.Unix(announced.Start, 0), time.Unix(announced.End, 0), announced.Timezone)
	newFull, newShort := formatWhen(time.Unix(current.Start, 0), time.Unix(current.End, 0), current.Timezone)
	if oldFull != newFull {
		c := EventDetailChange{
			Field: DetailFieldWhen, Label: "Date & time", Kind: DetailChanged,
			Old: oldFull, New: newFull,
		}
		// The short form is the START only. If just the end moved, quoting an
		// unchanged start as "now …" would read as though nothing changed, so
		// the push says the time was updated instead.
		if newShort != oldShort {
			c.Short = newShort
		}
		out = append(out, c)
	}

	// ── Venue ───────────────────────────────────────────────────────────────
	oldVenue := VenueText(announced.Venue, announced.Address, announced.City)
	newVenue := VenueText(current.Venue, current.Address, current.City)
	if normDetailText(oldVenue) != normDetailText(newVenue) {
		c := EventDetailChange{
			Field: DetailFieldVenue, Label: "Venue", Kind: DetailChanged,
			Old: oldVenue, New: newVenue,
		}
		// Quote the venue NAME only when the name itself moved. An address
		// correction under the same name is "Venue updated" — quoting the
		// unchanged name as "now X" would hide the fact that anything moved.
		if normDetailText(announced.Venue) != normDetailText(current.Venue) {
			c.Short = quotable(current.Venue)
		}
		out = append(out, c)
	}

	// ── Name ────────────────────────────────────────────────────────────────
	if normDetailText(announced.Title) != normDetailText(current.Title) {
		out = append(out, EventDetailChange{
			Field: DetailFieldTitle, Label: "Event name", Kind: DetailChanged,
			Old: strings.TrimSpace(announced.Title), New: strings.TrimSpace(current.Title),
		})
	}

	// ── Information ─────────────────────────────────────────────────────────
	keys := make([]string, 0, len(announced.Info)+len(current.Info))
	seen := make(map[string]bool, len(announced.Info)+len(current.Info))
	for k := range announced.Info {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for k := range current.Info {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	// Catalogue order puts age requirement first — the change most likely to
	// turn someone away at the door.
	sort.Slice(keys, func(i, j int) bool {
		oi, oj := infoOrder(keys[i], catalogue), infoOrder(keys[j], catalogue)
		if oi != oj {
			return oi < oj
		}
		return keys[i] < keys[j]
	})

	for _, k := range keys {
		oldV := strings.TrimSpace(announced.Info[k])
		newV := strings.TrimSpace(current.Info[k])
		had, has := oldV != "", newV != ""

		c := EventDetailChange{Field: DetailFieldInfoPrefix + k, Label: infoLabel(k, catalogue)}
		switch {
		case had && has:
			if normDetailText(oldV) == normDetailText(newV) {
				continue
			}
			c.Kind, c.Old, c.New, c.Short = DetailChanged, oldV, newV, quotable(newV)
		case has:
			c.Kind, c.New, c.Short = DetailAdded, newV, quotable(newV)
		case had:
			c.Kind, c.Old = DetailRemoved, oldV
		default:
			continue
		}
		out = append(out, c)
	}

	// ── Description ─────────────────────────────────────────────────────────
	//
	// Last, and never quoted. On a live event it only changes through an
	// approved edit request, so it is a deliberate edit rather than autosave
	// churn — worth a line, not worth leading with.
	if announced.DescHash != current.DescHash {
		out = append(out, EventDetailChange{
			Field: DetailFieldDescription, Label: "Description", Kind: DetailChanged,
		})
	}

	return out
}

// VenueText renders venue, address and city as one line, skipping a part the
// line already contains. Addresses usually end in the city already, and
// "The Warehouse, 12 Main Rd, Braamfontein, Braamfontein" helps nobody.
func VenueText(venue, address, city string) string {
	parts := make([]string, 0, 3)
	sofar := ""
	for _, p := range []string{venue, address, city} {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lp := strings.ToLower(p)
		if strings.Contains(sofar, lp) {
			continue
		}
		parts = append(parts, p)
		sofar += "\n" + lp
	}
	return strings.Join(parts, ", ")
}

// ── Reading a change list ────────────────────────────────────────────────────

func (cs EventDetailChanges) has(field string) bool {
	for _, c := range cs {
		if c.Field == field {
			return true
		}
	}
	return false
}

// TimeChanged reports whether the date or time moved.
func (cs EventDetailChanges) TimeChanged() bool { return cs.has(DetailFieldWhen) }

// VenueChanged reports whether the venue, address or city moved.
func (cs EventDetailChanges) VenueChanged() bool { return cs.has(DetailFieldVenue) }

// Fields lists the changed field keys, in order, for the notification payload.
func (cs EventDetailChanges) Fields() []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Field)
	}
	return out
}

// NotificationTitle is the push / in-app title.
//
// Says WHAT kind of change when it is one that can send someone to the wrong
// place at the wrong time; those are the ones worth reading on a lock screen.
func (cs EventDetailChanges) NotificationTitle() string {
	switch {
	case cs.TimeChanged() && cs.VenueChanged():
		return "Date, time & venue changed"
	case cs.TimeChanged():
		return "Date & time changed"
	case cs.VenueChanged():
		return "Venue changed"
	default:
		return "Event update"
	}
}

// EmailSubject is the email subject line.
func (cs EventDetailChanges) EmailSubject(eventTitle string) string {
	t := strings.TrimSpace(eventTitle)
	switch {
	case cs.TimeChanged() && cs.VenueChanged():
		return "Date, time & venue changed: " + t
	case cs.TimeChanged():
		return "Date & time changed: " + t
	case cs.VenueChanged():
		return "Venue changed: " + t
	default:
		return "Update to " + t
	}
}

// NotificationBody writes the push / in-app body.
//
// Names the changes rather than saying "details have been updated", for the
// same reason as the line-up notice: a vague message makes the ticket holder
// open the page and compare, which is the work this exists to save them.
//
// Up to four changes are named; beyond that the first three are named and the
// rest counted — "and 1 more" in the fourth slot would say less than the
// fourth change itself.
func (cs EventDetailChanges) NotificationBody(eventTitle string) string {
	t := strings.TrimSpace(eventTitle)
	if len(cs) == 0 {
		return fmt.Sprintf("The details for %s have been updated.", t)
	}

	shown := len(cs)
	if shown > maxBodyParts+1 {
		shown = maxBodyParts
	}

	parts := make([]string, 0, shown+1)
	for _, c := range cs[:shown] {
		parts = append(parts, c.phrase())
	}
	if rest := len(cs) - shown; rest > 0 {
		parts = append(parts, fmt.Sprintf("%d more updates", rest))
	}
	return fmt.Sprintf("%s: %s.", t, strings.Join(parts, "; "))
}

// phrase is one change as a push body fragment.
func (c EventDetailChange) phrase() string {
	switch c.Field {
	case DetailFieldTitle:
		return fmt.Sprintf("Renamed from “%s”", c.Old)
	case DetailFieldDescription:
		return "Description updated"
	}

	switch c.Kind {
	case DetailAdded:
		if c.Short != "" {
			return fmt.Sprintf("%s added: %s", c.Label, c.Short)
		}
		return c.Label + " added"
	case DetailRemoved:
		return c.Label + " removed"
	default:
		if c.Short != "" {
			return fmt.Sprintf("%s now %s", c.Label, c.Short)
		}
		return c.Label + " updated"
	}
}

// EventDetailsChangePayload is the Data on a NotifEventDetailsChanged row.
//
// event_id is what the app routes a tap on (lib/push.ts falls through to
// /event/:id for any type carrying one), so this works in the current app
// build without a release.
type EventDetailsChangePayload struct {
	EventID    string `json:"event_id"`
	EventSlug  string `json:"event_slug"`
	EventTitle string `json:"event_title"`
	// NoticeID points at the event_update_notices row holding the full change
	// list, for a client that later wants to show "what changed".
	NoticeID string   `json:"notice_id,omitempty"`
	Fields   []string `json:"fields"`
	// TimeChanged / VenueChanged let a client style the two changes that can
	// send someone to the wrong place or time, without re-deriving them from
	// Fields.
	TimeChanged  bool `json:"time_changed"`
	VenueChanged bool `json:"venue_changed"`
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// normDetailText is the comparison form: case-insensitive, whitespace runs
// collapsed, ends trimmed.
func normDetailText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// quotable returns v if it is short enough to quote in a push body, else "".
func quotable(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.ContainsAny(v, "\r\n") || utf8.RuneCountInString(v) > maxQuotableRunes {
		return ""
	}
	return v
}

// infoLabel is the catalogue label, or a readable form of the key for one
// that has since left the catalogue ("prohibited_items" → "Prohibited items").
func infoLabel(key string, catalogue map[string]EventInfoCatalogueEntry) string {
	if e, ok := catalogue[key]; ok && strings.TrimSpace(e.Label) != "" {
		return strings.TrimSpace(e.Label)
	}
	s := strings.TrimSpace(strings.ReplaceAll(key, "_", " "))
	if s == "" {
		return "Information"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// infoOrder is the catalogue display order; unknown keys sort last.
func infoOrder(key string, catalogue map[string]EventInfoCatalogueEntry) int {
	if e, ok := catalogue[key]; ok {
		return e.DisplayOrder
	}
	return 1 << 20
}