package models

import "testing"

func testCatalogue() map[string]EventInfoCatalogueEntry {
	return map[string]EventInfoCatalogueEntry{
		"age_requirement": {
			Key: "age_requirement", Label: "Age requirement",
			ValueType:   InfoEnum,
			EnumOptions: []string{"All ages", "13+", "16+", "18+", "21+"},
			IsActive:    true,
			ReviewRequiredWhenPublished: true,
		},
		"doors_open": {
			Key: "doors_open", Label: "Doors open",
			ValueType: InfoTime, IsActive: true,
		},
		"parking": {
			Key: "parking", Label: "Parking",
			ValueType: InfoText, IsActive: true,
		},
		"camping": {
			Key: "camping", Label: "Camping",
			ValueType: InfoText, IsActive: false, // retired
		},
	}
}

func TestValidateEventInfoItems_Accepts(t *testing.T) {
	cat := testCatalogue()

	items, err := ValidateEventInfoItems([]EventInfoItemDTO{
		{Key: "parking", Value: "  Free on-site parking, entrance off Merriman Ave.  "},
		{Key: "doors_open", Value: "18:00"},
		{Key: "age_requirement", Value: "18+"},
	}, cat, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// Whitespace trimmed, so the DB CHECK on btrim(value) can never be the
	// thing that rejects a value the user considers filled in.
	if items[0].Value != "Free on-site parking, entrance off Merriman Ave." {
		t.Errorf("value not trimmed: %q", items[0].Value)
	}
	// sort_order follows the submitted order — the form is the source of truth.
	for i, it := range items {
		if it.SortOrder != i {
			t.Errorf("item %d has sort_order %d", i, it.SortOrder)
		}
	}
	// Label denormalised so the client can render without the catalogue.
	if items[2].Label != "Age requirement" {
		t.Errorf("label not attached: %q", items[2].Label)
	}
}

// An empty value is "I did not choose this field", not an error. It must
// produce NO ROW — that is what makes an unchosen field impossible to render
// as an empty heading.
func TestValidateEventInfoItems_EmptyValueIsSkippedNotRejected(t *testing.T) {
	items, err := ValidateEventInfoItems([]EventInfoItemDTO{
		{Key: "parking", Value: "Free"},
		{Key: "doors_open", Value: "   "},
	}, testCatalogue(), false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected the blank field to be dropped, got %d items", len(items))
	}
	if items[0].Key != "parking" {
		t.Errorf("wrong item survived: %q", items[0].Key)
	}
}

func TestValidateEventInfoItems_Rejects(t *testing.T) {
	cat := testCatalogue()
	long := make([]rune, 601)
	for i := range long {
		long[i] = 'a'
	}

	cases := []struct {
		name string
		in   []EventInfoItemDTO
	}{
		{"unknown key", []EventInfoItemDTO{{Key: "wifi_password", Value: "hunter2"}}},
		{"retired key", []EventInfoItemDTO{{Key: "camping", Value: "Yes"}}},
		{"duplicate key", []EventInfoItemDTO{
			{Key: "parking", Value: "A"}, {Key: "parking", Value: "B"},
		}},
		{"enum not in options", []EventInfoItemDTO{{Key: "age_requirement", Value: "17+"}}},
		{"time not HH:MM", []EventInfoItemDTO{{Key: "doors_open", Value: "6pm"}}},
		{"time 24:00", []EventInfoItemDTO{{Key: "doors_open", Value: "24:00"}}},
		{"time 09:60", []EventInfoItemDTO{{Key: "doors_open", Value: "09:60"}}},
		{"value too long", []EventInfoItemDTO{{Key: "parking", Value: string(long)}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateEventInfoItems(tc.in, cat, false, nil); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

// Enum matching is case-insensitive: an organiser typing "all ages" should not
// be rejected because the catalogue says "All ages".
func TestValidateEventInfoItems_EnumIsCaseInsensitive(t *testing.T) {
	if _, err := ValidateEventInfoItems(
		[]EventInfoItemDTO{{Key: "age_requirement", Value: "all ages"}},
		testCatalogue(), false, nil,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── The review lock ──────────────────────────────────────────────────────────
//
// These are the cases that matter most. The lock has to stop a silent change to
// what a buyer agreed to, WITHOUT blocking every other edit on a live event —
// the form posts the whole block on every save, so an untouched age restriction
// arrives on every request.

func TestReviewLock_UnchangedLockedFieldIsAllowed(t *testing.T) {
	existing := map[string]string{"age_requirement": "18+", "parking": "Free"}

	_, err := ValidateEventInfoItems([]EventInfoItemDTO{
		{Key: "age_requirement", Value: "18+"},          // untouched
		{Key: "parking", Value: "R20 at the north gate"}, // the actual edit
	}, testCatalogue(), true, existing)
	if err != nil {
		t.Fatalf("editing parking on a live event must not be blocked by an untouched age field: %v", err)
	}
}

func TestReviewLock_ChangedLockedFieldIsRefused(t *testing.T) {
	existing := map[string]string{"age_requirement": "All ages"}

	_, err := ValidateEventInfoItems([]EventInfoItemDTO{
		{Key: "age_requirement", Value: "18+"},
	}, testCatalogue(), true, existing)
	if err == nil {
		t.Fatal("changing the age requirement on a live event must require review")
	}
}

// Removing a locked field changes what a buyer agreed to just as surely as
// editing it, so it goes through review too.
func TestReviewLock_RemovingLockedFieldIsRefused(t *testing.T) {
	existing := map[string]string{"age_requirement": "18+", "parking": "Free"}

	_, err := ValidateEventInfoItems([]EventInfoItemDTO{
		{Key: "parking", Value: "Free"},
	}, testCatalogue(), true, existing)
	if err == nil {
		t.Fatal("removing the age requirement from a live event must require review")
	}
}

// A DRAFT has no buyers, so nothing is locked.
func TestReviewLock_DraftIsUnrestricted(t *testing.T) {
	if _, err := ValidateEventInfoItems(
		[]EventInfoItemDTO{{Key: "age_requirement", Value: "18+"}},
		testCatalogue(), false, map[string]string{"age_requirement": "All ages"},
	); err != nil {
		t.Fatalf("a draft must be freely editable: %v", err)
	}
}

// Adding a locked field to a live event that never had one is still a change.
func TestReviewLock_AddingLockedFieldIsRefused(t *testing.T) {
	_, err := ValidateEventInfoItems(
		[]EventInfoItemDTO{{Key: "age_requirement", Value: "18+"}},
		testCatalogue(), true, map[string]string{},
	)
	if err == nil {
		t.Fatal("adding an age requirement to a live event must require review")
	}
}