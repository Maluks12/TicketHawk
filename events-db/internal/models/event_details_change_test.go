package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// These are the rules that decide whether a push and an email land on every
// ticket holder for an event. Over-firing trains people to ignore them, which
// costs us the one about the venue moving; under-firing is the bug this
// feature exists to fix.

var dcSAST = time.FixedZone("SAST", 2*60*60)

// dcWhen mirrors the shape of the worker's formatter: minute precision,
// start and end in full, start only in short. "UTC" lets a test move the zone.
func dcWhen(start, end time.Time, tz string) (string, string) {
	loc := dcSAST
	if tz == "UTC" {
		loc = time.UTC
	}
	s, e := start.In(loc), end.In(loc)
	return s.Format("Mon 2 Jan 2006 15:04") + " – " + e.Format("Mon 2 Jan 2006 15:04"),
		s.Format("Mon 2 Jan, 15:04")
}

func dcAt(d, h, m int) int64 {
	return time.Date(2026, time.November, d, h, m, 0, 0, dcSAST).Unix()
}

func dcCatalogue() map[string]EventInfoCatalogueEntry {
	return map[string]EventInfoCatalogueEntry{
		"age_requirement": {Key: "age_requirement", Label: "Age requirement", DisplayOrder: 10},
		"doors_open":      {Key: "doors_open", Label: "Doors open", DisplayOrder: 20},
		"dress_code":      {Key: "dress_code", Label: "Dress code", DisplayOrder: 40},
		"parking":         {Key: "parking", Label: "Parking", DisplayOrder: 60},
	}
}

func dcSnap() EventDetailsSnapshot {
	return EventDetailsSnapshot{
		Title:    "Summer Fest",
		DescHash: "d1",
		Start:    dcAt(14, 18, 0),
		End:      dcAt(14, 23, 0),
		Timezone: "SAST",
		Venue:    "The Old Mill",
		Address:  "12 Mill Rd, Braamfontein",
		City:     "Braamfontein",
		Info: map[string]string{
			"parking":         "Street parking only",
			"age_requirement": "All ages",
		},
	}
}

// dcWithInfo returns a copy whose Info map can be edited without touching s.
func dcWithInfo(s EventDetailsSnapshot) EventDetailsSnapshot {
	info := make(map[string]string, len(s.Info))
	for k, v := range s.Info {
		info[k] = v
	}
	s.Info = info
	return s
}

func dcDiff(a, b EventDetailsSnapshot) EventDetailChanges {
	return DiffEventDetails(a, b, dcCatalogue(), dcWhen)
}

// ── What is NOT a change ─────────────────────────────────────────────────────

func TestDetails_IdenticalIsNotAChange(t *testing.T) {
	if got := dcDiff(dcSnap(), dcSnap()); len(got) != 0 {
		t.Fatalf("re-saving unchanged details must not notify anyone: %+v", got)
	}
}

func TestDetails_CaseAndWhitespaceAreNotChanges(t *testing.T) {
	cur := dcWithInfo(dcSnap())
	cur.Title = "  summer   FEST "
	cur.Venue = "the old mill"
	cur.Info["parking"] = "Street  parking   only"
	if got := dcDiff(dcSnap(), cur); len(got) != 0 {
		t.Fatalf("typo-level edits must not push every ticket holder: %+v", got)
	}
}

func TestDetails_SecondsAreNotAChange(t *testing.T) {
	cur := dcSnap()
	cur.Start += 20 // same minute, invisible to anyone reading the time
	if got := dcDiff(dcSnap(), cur); len(got) != 0 {
		t.Fatalf("a sub-minute shift is not a time change: %+v", got)
	}
}

func TestDetails_NilInfoMapsAreSafe(t *testing.T) {
	a, b := dcSnap(), dcSnap()
	a.Info, b.Info = nil, nil
	if got := DiffEventDetails(a, b, nil, dcWhen); len(got) != 0 {
		t.Fatalf("expected no change: %+v", got)
	}
}

// ── What IS a change ─────────────────────────────────────────────────────────

func TestDetails_StartMoveIsAnnouncedWithOldAndNew(t *testing.T) {
	cur := dcSnap()
	cur.Start = dcAt(14, 19, 0)
	got := dcDiff(dcSnap(), cur)
	if len(got) != 1 || got[0].Field != DetailFieldWhen {
		t.Fatalf("expected one date & time change: %+v", got)
	}
	if !strings.Contains(got[0].Old, "18:00") || !strings.Contains(got[0].New, "19:00") {
		t.Errorf("old/new should carry both times: %+v", got[0])
	}
	if got[0].Short != "Sat 14 Nov, 19:00" {
		t.Errorf("push should quote the new start, got %q", got[0].Short)
	}
	if !got.TimeChanged() || got.VenueChanged() {
		t.Errorf("flags wrong: time=%v venue=%v", got.TimeChanged(), got.VenueChanged())
	}
}

func TestDetails_EndOnlyMoveDoesNotQuoteUnchangedStart(t *testing.T) {
	cur := dcSnap()
	cur.End = dcAt(15, 2, 0)
	got := dcDiff(dcSnap(), cur)
	if len(got) != 1 || got[0].Field != DetailFieldWhen {
		t.Fatalf("expected one date & time change: %+v", got)
	}
	if got[0].Short != "" {
		t.Errorf("quoting the unchanged start as \"now …\" reads as no change; got %q", got[0].Short)
	}
	if body := got.NotificationBody("Summer Fest"); !strings.Contains(body, "Date & time updated") {
		t.Errorf("body should say the time was updated: %q", body)
	}
}

func TestDetails_TimezoneThatMovesTheClockIsAChange(t *testing.T) {
	cur := dcSnap()
	cur.Timezone = "UTC" // same instants, but the local clock time people read moves
	if got := dcDiff(dcSnap(), cur); len(got) != 1 || got[0].Field != DetailFieldWhen {
		t.Fatalf("a timezone correction that moves the displayed time must be announced: %+v", got)
	}
}

func TestDetails_VenueFieldsCollapseIntoOneChange(t *testing.T) {
	cur := dcSnap()
	cur.Venue, cur.Address, cur.City = "The Warehouse", "4 Dock Rd, Cape Town", "Cape Town"
	got := dcDiff(dcSnap(), cur)
	if len(got) != 1 || got[0].Field != DetailFieldVenue {
		t.Fatalf("name, address and city moving is ONE venue change: %+v", got)
	}
	if got[0].New != "The Warehouse, 4 Dock Rd, Cape Town" {
		t.Errorf("new venue line wrong: %q", got[0].New)
	}
	if got[0].Short != "The Warehouse" {
		t.Errorf("push should quote the new venue name, got %q", got[0].Short)
	}
}

func TestDetails_AddressOnlyChangeIsNotQuotedAsTheSameName(t *testing.T) {
	cur := dcSnap()
	cur.Address = "14 Mill Rd, Braamfontein"
	got := dcDiff(dcSnap(), cur)
	if len(got) != 1 || got[0].Field != DetailFieldVenue {
		t.Fatalf("an address move is a venue change: %+v", got)
	}
	if got[0].Short != "" {
		t.Errorf("\"Venue now The Old Mill\" would hide that the address moved; got %q", got[0].Short)
	}
}

func TestDetails_InfoAddedRemovedChangedInCatalogueOrder(t *testing.T) {
	cur := dcWithInfo(dcSnap())
	cur.Info["age_requirement"] = "18+"     // changed, quotable
	cur.Info["dress_code"] = "Smart casual" // added
	delete(cur.Info, "parking")             // removed
	cur.Info["doors_open"] = "17:30"        // added
	got := dcDiff(dcSnap(), cur)

	want := []struct{ field, kind string }{
		{"info:age_requirement", DetailChanged},
		{"info:doors_open", DetailAdded},
		{"info:dress_code", DetailAdded},
		{"info:parking", DetailRemoved},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d changes, got %+v", len(want), got)
	}
	for i, w := range want {
		if got[i].Field != w.field || got[i].Kind != w.kind {
			t.Errorf("change %d: want %s/%s, got %s/%s", i, w.field, w.kind, got[i].Field, got[i].Kind)
		}
	}
	if got[0].Label != "Age requirement" || got[0].Old != "All ages" || got[0].Short != "18+" {
		t.Errorf("age change wrong: %+v", got[0])
	}
}

func TestDetails_LongInfoIsNotQuotedInPush(t *testing.T) {
	cur := dcWithInfo(dcSnap())
	cur.Info["parking"] = "Use the north lot off Jan Smuts Ave; the south lot is closed for the weekend"
	got := dcDiff(dcSnap(), cur)
	if len(got) != 1 || got[0].Short != "" {
		t.Fatalf("long text belongs in the email, not the push: %+v", got)
	}
	if got[0].New == "" {
		t.Errorf("the email still needs the full new value")
	}
}

func TestDetails_UnknownInfoKeyGetsReadableLabel(t *testing.T) {
	cur := dcWithInfo(dcSnap())
	cur.Info["prohibited_items"] = "No glass"
	got := dcDiff(dcSnap(), cur)
	if len(got) != 1 || got[0].Label != "Prohibited items" {
		t.Fatalf("a key missing from the catalogue should still read well: %+v", got)
	}
}

func TestDetails_DescriptionChangeIsLastAndUnquoted(t *testing.T) {
	cur := dcWithInfo(dcSnap())
	cur.DescHash = "d2"
	cur.Info["age_requirement"] = "18+"
	got := dcDiff(dcSnap(), cur)
	if len(got) != 2 || got[1].Field != DetailFieldDescription {
		t.Fatalf("description should be reported, last: %+v", got)
	}
	if got[1].Old != "" || got[1].New != "" || got[1].Short != "" {
		t.Errorf("the description is never quoted: %+v", got[1])
	}
}

func TestDetails_ImportantChangesLead(t *testing.T) {
	cur := dcWithInfo(dcSnap())
	cur.DescHash = "d2"
	cur.Info["parking"] = "Paid parking R50"
	cur.Title = "Summer Fest 2026"
	cur.Venue = "The Warehouse"
	cur.Start = dcAt(15, 18, 0)
	got := dcDiff(dcSnap(), cur)
	order := []string{DetailFieldWhen, DetailFieldVenue, DetailFieldTitle, "info:parking", DetailFieldDescription}
	if len(got) != len(order) {
		t.Fatalf("expected %d changes, got %+v", len(order), got)
	}
	for i, f := range order {
		if got[i].Field != f {
			t.Errorf("position %d: want %s, got %s", i, f, got[i].Field)
		}
	}
}

// ── Wording ──────────────────────────────────────────────────────────────────

func TestBody_NamesChanges(t *testing.T) {
	cur := dcWithInfo(dcSnap())
	cur.Start = dcAt(14, 19, 0)
	cur.Info["parking"] = "Paid parking R50"
	body := dcDiff(dcSnap(), cur).NotificationBody("Summer Fest")
	want := "Summer Fest: Date & time now Sat 14 Nov, 19:00; Parking now Paid parking R50."
	if body != want {
		t.Errorf("body:\n got %q\nwant %q", body, want)
	}
}

func TestBody_FourChangesAreAllNamed(t *testing.T) {
	cs := EventDetailChanges{
		{Field: "info:a", Label: "A", Kind: DetailRemoved},
		{Field: "info:b", Label: "B", Kind: DetailRemoved},
		{Field: "info:c", Label: "C", Kind: DetailRemoved},
		{Field: "info:d", Label: "D", Kind: DetailRemoved},
	}
	if got := cs.NotificationBody("X"); got != "X: A removed; B removed; C removed; D removed." {
		t.Errorf("four changes fit; \"1 more\" says less than naming it: %q", got)
	}
}

func TestBody_OverflowIsCounted(t *testing.T) {
	cs := EventDetailChanges{
		{Field: "info:a", Label: "A", Kind: DetailRemoved},
		{Field: "info:b", Label: "B", Kind: DetailRemoved},
		{Field: "info:c", Label: "C", Kind: DetailRemoved},
		{Field: "info:d", Label: "D", Kind: DetailRemoved},
		{Field: "info:e", Label: "E", Kind: DetailRemoved},
	}
	if got := cs.NotificationBody("X"); got != "X: A removed; B removed; C removed; 2 more updates." {
		t.Errorf("overflow: %q", got)
	}
}

func TestBody_RenameUsesTheNewTitleAndNamesTheOld(t *testing.T) {
	cur := dcSnap()
	cur.Title = "Summer Fest: Encore"
	got := dcDiff(dcSnap(), cur)
	body := got.NotificationBody(cur.Title)
	if body != "Summer Fest: Encore: Renamed from “Summer Fest”." {
		t.Errorf("rename body: %q", body)
	}
}

func TestBody_AddedValueIsQuoted(t *testing.T) {
	cur := dcWithInfo(dcSnap())
	cur.Info["dress_code"] = "Smart casual"
	if body := dcDiff(dcSnap(), cur).NotificationBody("Summer Fest"); body != "Summer Fest: Dress code added: Smart casual." {
		t.Errorf("added body: %q", body)
	}
}

func TestTitlesAndSubjects(t *testing.T) {
	when := EventDetailChange{Field: DetailFieldWhen}
	venue := EventDetailChange{Field: DetailFieldVenue}
	info := EventDetailChange{Field: "info:parking"}

	cases := []struct {
		cs             EventDetailChanges
		title, subject string
	}{
		{EventDetailChanges{when, venue}, "Date, time & venue changed", "Date, time & venue changed: Fest"},
		{EventDetailChanges{when, info}, "Date & time changed", "Date & time changed: Fest"},
		{EventDetailChanges{venue}, "Venue changed", "Venue changed: Fest"},
		{EventDetailChanges{info}, "Event update", "Update to Fest"},
	}
	for i, c := range cases {
		if got := c.cs.NotificationTitle(); got != c.title {
			t.Errorf("case %d title: %q", i, got)
		}
		if got := c.cs.EmailSubject("Fest"); got != c.subject {
			t.Errorf("case %d subject: %q", i, got)
		}
	}
}

func TestVenueText_SkipsWhatTheLineAlreadySays(t *testing.T) {
	cases := []struct{ v, a, c, want string }{
		{"The Old Mill", "12 Mill Rd, Braamfontein", "Braamfontein", "The Old Mill, 12 Mill Rd, Braamfontein"},
		{"The Old Mill", "12 Mill Rd", "Braamfontein", "The Old Mill, 12 Mill Rd, Braamfontein"},
		{"", "12 Mill Rd", "", "12 Mill Rd"},
		{"  ", "", "Durban", "Durban"},
	}
	for _, c := range cases {
		if got := VenueText(c.v, c.a, c.c); got != c.want {
			t.Errorf("VenueText(%q,%q,%q) = %q, want %q", c.v, c.a, c.c, got, c.want)
		}
	}
}

// The stored snapshot is written by SQL (jsonb_build_object) and read here.
// This pins the key names and types the two sides agree on.
func TestSnapshot_ReadsTheShapeSQLWrites(t *testing.T) {
	raw := `{"tz": "SAST", "end": 1794693600, "city": "Braamfontein", "desc": "9e107d9d372bb6826bd81d3542a419d6",
	         "info": {"parking": "Street parking only"}, "start": 1794675600, "title": "Summer Fest",
	         "venue": "The Old Mill", "address": "12 Mill Rd"}`
	var s EventDetailsSnapshot
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.Start != 1794675600 || s.End != 1794693600 || s.Info["parking"] != "Street parking only" ||
		s.Venue != "The Old Mill" || s.DescHash == "" || s.Timezone != "SAST" {
		t.Errorf("snapshot fields not read: %+v", s)
	}
}

func TestPayload_CarriesEventIDForTapRouting(t *testing.T) {
	b, err := json.Marshal(EventDetailsChangePayload{EventID: "e1", Fields: []string{"when"}, TimeChanged: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"event_id":"e1"`) {
		t.Errorf("the app routes a tap on event_id: %s", b)
	}
}