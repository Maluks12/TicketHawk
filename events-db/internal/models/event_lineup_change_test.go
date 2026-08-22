package models

import (
	"strings"
	"testing"
)

func snap(performers ...string) LineupSnapshot {
	return LineupSnapshot{"performer": performers}
}

// ── What counts as a change ──────────────────────────────────────────────────
//
// These are the cases that decide whether a push lands on every ticket
// holder's phone. Over-firing trains people to swipe the notification away,
// which costs us the one that matters — a dropped headliner.

func TestDiff_ReorderIsNotAChange(t *testing.T) {
	d := DiffSnapshots(snap("Drae Sativ", "Kali Freshman"), snap("Kali Freshman", "Drae Sativ"))
	if d.IsMaterial() {
		t.Fatalf("dragging the headliner up the list is presentation, not news: %+v", d)
	}
}

func TestDiff_CapitalisationIsNotAChange(t *testing.T) {
	d := DiffSnapshots(snap("drae sativ"), snap("Drae Sativ"))
	if d.IsMaterial() {
		t.Fatalf("fixing a typo must not push every ticket holder: %+v", d)
	}
}

func TestDiff_IdenticalIsNotAChange(t *testing.T) {
	// An organiser editing a stage name, a set time or an image re-saves the
	// whole list. Names unchanged means nothing to announce.
	d := DiffSnapshots(snap("A", "B"), snap("A", "B"))
	if d.IsMaterial() {
		t.Fatalf("expected no change: %+v", d)
	}
}

func TestDiff_RemovalIsDetected(t *testing.T) {
	d := DiffSnapshots(snap("Drae Sativ", "Kali Freshman"), snap("Kali Freshman"))
	if !d.IsMaterial() {
		t.Fatal("a dropped act must be announced")
	}
	if len(d.Removed) != 1 || d.Removed[0] != "Drae Sativ" {
		t.Errorf("wrong removal: %+v", d.Removed)
	}
	if len(d.Added) != 0 {
		t.Errorf("unexpected additions: %+v", d.Added)
	}
}

func TestDiff_AdditionIsDetected(t *testing.T) {
	d := DiffSnapshots(snap("A"), snap("A", "B"))
	if len(d.Added) != 1 || d.Added[0] != "B" {
		t.Errorf("wrong addition: %+v", d.Added)
	}
}

// A swap is the common real case: one act drops out and a replacement is
// announced the same afternoon.
func TestDiff_Swap(t *testing.T) {
	d := DiffSnapshots(snap("Old Headliner", "Support"), snap("New Headliner", "Support"))
	if len(d.Removed) != 1 || d.Removed[0] != "Old Headliner" {
		t.Errorf("wrong removal: %+v", d.Removed)
	}
	if len(d.Added) != 1 || d.Added[0] != "New Headliner" {
		t.Errorf("wrong addition: %+v", d.Added)
	}
}

// Kinds are flattened for diffing: an act moving from team to performer is not
// a line-up change, it is a data correction.
func TestDiff_KindChangeIsNotAChange(t *testing.T) {
	before := LineupSnapshot{"team": {"Sundowns"}}
	after := LineupSnapshot{"performer": {"Sundowns"}}
	if DiffSnapshots(before, after).IsMaterial() {
		t.Fatal("reclassifying an entry must not notify anyone")
	}
}

func TestSnapshotFromEntries_SortsAndGroups(t *testing.T) {
	s := SnapshotFromEntries([]LineupEntry{
		{Kind: LineupPerformer, Name: "Zed"},
		{Kind: LineupTeam, Name: "Chiefs"},
		{Kind: LineupPerformer, Name: "Alpha"},
	})
	if len(s["performer"]) != 2 || s["performer"][0] != "Alpha" {
		t.Errorf("performers not sorted: %+v", s["performer"])
	}
	if len(s["team"]) != 1 || s["team"][0] != "Chiefs" {
		t.Errorf("teams wrong: %+v", s["team"])
	}
}

// ── Message copy ─────────────────────────────────────────────────────────────
//
// The body has to name the acts. A vague "the line-up has been updated" makes
// the reader open the app and compare — which is the work the notification
// exists to save.

func TestNotificationBody_NamesTheActs(t *testing.T) {
	d := LineupDiff{Removed: []string{"Drae Sativ"}, Added: []string{"Kali Freshman"}}
	body := d.NotificationBody("Winter Fest")

	if !strings.Contains(body, "Drae Sativ") || !strings.Contains(body, "Kali Freshman") {
		t.Fatalf("body must name both acts: %q", body)
	}
	// Removals lead — that is the bad news and the reason for the message.
	if strings.Index(body, "Drae Sativ") > strings.Index(body, "Kali Freshman") {
		t.Errorf("removal should come first: %q", body)
	}
}

func TestNotificationBody_LongListsCollapse(t *testing.T) {
	d := LineupDiff{Removed: []string{"A", "B", "C", "D", "E"}}
	body := d.NotificationBody("Big Fest")
	// Past three names the count IS the message — an OS-truncated body cut off
	// mid-name tells the reader nothing.
	if !strings.Contains(body, "3 others") {
		t.Errorf("expected a collapsed count: %q", body)
	}
}

func TestHumanList(t *testing.T) {
	cases := map[int]string{1: "A", 2: "A and B", 3: "A, B and C"}
	for n, want := range cases {
		names := []string{"A", "B", "C"}[:n]
		if got := humanList(names); got != want {
			t.Errorf("humanList(%d) = %q, want %q", n, got, want)
		}
	}
}

// ── Admin catalogue guard ────────────────────────────────────────────────────

// The check that stops an admin silently breaking organiser forms: removing an
// enum option that events still hold leaves those organisers unable to save,
// rejected on a field they never touched.
func TestOptionsRemovedFromUse(t *testing.T) {
	orphaned := OptionsRemovedFromUse(
		[]string{"All ages", "18+"},
		[]string{"All ages", "16+", "18+"},
	)
	if len(orphaned) != 1 || orphaned[0] != "16+" {
		t.Fatalf("expected 16+ to be flagged, got %+v", orphaned)
	}

	if got := OptionsRemovedFromUse([]string{"All ages", "16+", "18+"}, []string{"16+"}); len(got) != 0 {
		t.Errorf("adding options must always be safe, got %+v", got)
	}

	// Case-insensitive, matching how ValidateEventInfoItems compares.
	if got := OptionsRemovedFromUse([]string{"All Ages"}, []string{"all ages"}); len(got) != 0 {
		t.Errorf("comparison should ignore case, got %+v", got)
	}
}

func TestValidateNewInfoField_RejectsReservedKeys(t *testing.T) {
	// refund_policy is already a column on the event payload. A catalogue field
	// with the same key would render a second Refund Policy row from a
	// different source, and the two would drift apart.
	_, err := ValidateNewInfoField(&AdminCreateInfoFieldDTO{
		Key: "refund_policy", Label: "Refunds", ValueType: "text",
	})
	if err == nil {
		t.Fatal("reserved key must be rejected")
	}
}

func TestValidateNewInfoField_RejectsBadKeys(t *testing.T) {
	for _, k := range []string{"Shuttle Service", "shuttle-service", "1shuttle", "ab", ""} {
		if _, err := ValidateNewInfoField(&AdminCreateInfoFieldDTO{
			Key: k, Label: "X", ValueType: "text",
		}); err == nil {
			t.Errorf("key %q should have been rejected", k)
		}
	}
}

func TestValidateNewInfoField_EnumNeedsOptions(t *testing.T) {
	if _, err := ValidateNewInfoField(&AdminCreateInfoFieldDTO{
		Key: "shuttle", Label: "Shuttle", ValueType: "enum", EnumOptions: []string{"Yes"},
	}); err == nil {
		t.Fatal("an enum with one option is unfillable and must be rejected")
	}

	if _, err := ValidateNewInfoField(&AdminCreateInfoFieldDTO{
		Key: "shuttle", Label: "Shuttle", ValueType: "text", EnumOptions: []string{"a", "b"},
	}); err == nil {
		t.Fatal("options on a non-enum field signal a mistake and must be rejected")
	}
}