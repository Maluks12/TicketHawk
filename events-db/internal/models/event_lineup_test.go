package models

import (
	"testing"
	"time"
)

var (
	evStart = time.Date(2026, 9, 8, 18, 0, 0, 0, time.UTC)
	evEnd   = time.Date(2026, 9, 9, 2, 0, 0, 0, time.UTC)
)

func TestValidateLineupEntries_Accepts(t *testing.T) {
	img := "https://cdn.example.com/a.jpg"
	sub := "  Main stage  "
	set := time.Date(2026, 9, 8, 21, 30, 0, 0, time.UTC)
	link := "open.spotify.com/artist/x"

	entries, err := ValidateLineupEntries([]LineupEntryDTO{
		{Name: "  Drae Sativ  ", ImageURL: &img, Subtitle: &sub, SetTime: &set, ExternalURL: &link},
		{Kind: "team", Name: "Sundowns"},
	}, evStart, evEnd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "Drae Sativ" {
		t.Errorf("name not trimmed: %q", entries[0].Name)
	}
	if entries[0].Subtitle == nil || *entries[0].Subtitle != "Main stage" {
		t.Errorf("subtitle not trimmed: %v", entries[0].Subtitle)
	}
	// Kind defaults to performer when omitted — the common case, and the one
	// the old performers[] column held.
	if entries[0].Kind != LineupPerformer {
		t.Errorf("kind should default to performer, got %q", entries[0].Kind)
	}
	if entries[1].Kind != LineupTeam {
		t.Errorf("explicit team kind lost: %q", entries[1].Kind)
	}
	// Scheme-less URL upgraded, matching the organiser-link validator.
	if entries[0].ExternalURL == nil || *entries[0].ExternalURL != "https://open.spotify.com/artist/x" {
		t.Errorf("external url not normalised: %v", entries[0].ExternalURL)
	}
	if entries[0].SortOrder != 0 || entries[1].SortOrder != 1 {
		t.Errorf("sort order not taken from position")
	}
}

// A blank optional field must become NULL, not an empty string. The two render
// differently and only one of them means "not set".
func TestValidateLineupEntries_BlankOptionalsBecomeNil(t *testing.T) {
	blank := "   "
	entries, err := ValidateLineupEntries([]LineupEntryDTO{
		{Name: "Solo Act", Subtitle: &blank, Stage: &blank, ExternalURL: &blank},
	}, evStart, evEnd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	e := entries[0]
	if e.Subtitle != nil || e.Stage != nil || e.ExternalURL != nil {
		t.Errorf("blank optionals should be nil, got %v %v %v", e.Subtitle, e.Stage, e.ExternalURL)
	}
}

func TestValidateLineupEntries_Rejects(t *testing.T) {
	shortened := "https://bit.ly/xyz"
	ipLink := "https://192.168.0.1/"
	httpLink := "http://example.com"
	credLink := "https://evil.example@spotify.com/x"

	cases := []struct {
		name string
		in   []LineupEntryDTO
	}{
		{"empty name", []LineupEntryDTO{{Name: "   "}}},
		{"bad kind", []LineupEntryDTO{{Kind: "headliner", Name: "X"}}},
		{"duplicate performer", []LineupEntryDTO{{Name: "DJ A"}, {Name: "dj a"}}},
		{"shortener link", []LineupEntryDTO{{Name: "X", ExternalURL: &shortened}}},
		{"raw ip link", []LineupEntryDTO{{Name: "X", ExternalURL: &ipLink}}},
		{"plain http link", []LineupEntryDTO{{Name: "X", ExternalURL: &httpLink}}},
		{"credentials in link", []LineupEntryDTO{{Name: "X", ExternalURL: &credLink}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateLineupEntries(tc.in, evStart, evEnd); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

// Duplicates are scoped to kind: a charity match can field a team and book a DJ
// sharing a name without that being a mistake.
func TestValidateLineupEntries_SameNameDifferentKindIsFine(t *testing.T) {
	if _, err := ValidateLineupEntries([]LineupEntryDTO{
		{Kind: "team", Name: "Sundowns"},
		{Kind: "performer", Name: "Sundowns"},
	}, evStart, evEnd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Set time ─────────────────────────────────────────────────────────────────
//
// The guard is against a mistyped DATE, which puts an act a day out and is
// invisible until somebody arrives at an empty stage.

func TestSetTime_AllowsSpillOutsideTheWindow(t *testing.T) {
	// An after-party act two hours past the advertised end.
	late := evEnd.Add(2 * time.Hour)
	if _, err := ValidateLineupEntries(
		[]LineupEntryDTO{{Name: "Closing DJ", SetTime: &late}}, evStart, evEnd,
	); err != nil {
		t.Fatalf("a set shortly after the end must be allowed: %v", err)
	}

	// A doors-open acoustic set before the advertised start.
	early := evStart.Add(-90 * time.Minute)
	if _, err := ValidateLineupEntries(
		[]LineupEntryDTO{{Name: "Opener", SetTime: &early}}, evStart, evEnd,
	); err != nil {
		t.Fatalf("a set shortly before the start must be allowed: %v", err)
	}
}

func TestSetTime_RejectsWrongDate(t *testing.T) {
	wrongDay := evStart.AddDate(0, 0, -1) // right time, day early
	if _, err := ValidateLineupEntries(
		[]LineupEntryDTO{{Name: "Headliner", SetTime: &wrongDay}}, evStart, evEnd,
	); err == nil {
		t.Fatal("a set time a full day out must be rejected")
	}
}

// The draft-creation path has no event times yet, so zero bounds skip the check
// rather than rejecting everything.
func TestSetTime_ZeroBoundsSkipTheCheck(t *testing.T) {
	anyTime := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := ValidateLineupEntries(
		[]LineupEntryDTO{{Name: "X", SetTime: &anyTime}}, time.Time{}, time.Time{},
	); err != nil {
		t.Fatalf("zero bounds must skip the range check: %v", err)
	}
}

// ── Legacy array projection ──────────────────────────────────────────────────
//
// This is what keeps shipped mobile builds working. If it breaks, old app
// installs show an empty line-up.

func TestLineupNamesByKind(t *testing.T) {
	performers, teams := LineupNamesByKind([]LineupEntry{
		{Kind: LineupTeam, Name: "Sundowns"},
		{Kind: LineupPerformer, Name: "Drae Sativ"},
		{Kind: LineupTeam, Name: "Chiefs"},
		{Kind: LineupPerformer, Name: "Kali Freshman"},
	})

	if len(teams) != 2 || teams[0] != "Sundowns" || teams[1] != "Chiefs" {
		t.Errorf("teams wrong: %v", teams)
	}
	if len(performers) != 2 || performers[0] != "Drae Sativ" {
		t.Errorf("performers wrong: %v", performers)
	}
}

// An empty line-up must produce empty slices, not nil — a nil slice would write
// NULL to the arrays, and the app does `ev.teams ?? []`, so NULL and [] happen
// to render the same. Empty slices keep the column shape honest either way.
func TestLineupNamesByKind_EmptyIsNotNil(t *testing.T) {
	performers, teams := LineupNamesByKind(nil)
	if performers == nil || teams == nil {
		t.Error("expected empty slices, not nil")
	}
	if len(performers) != 0 || len(teams) != 0 {
		t.Error("expected both to be empty")
	}
}