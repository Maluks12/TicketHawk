package models

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ── Event information ────────────────────────────────────────────────────────
//
// A catalogue of fields the server owns, and a row per field the organiser
// chose to fill in. An unchosen field has NO ROW, which is what makes it
// impossible to render an empty "PARKING" heading — the bug visible on Howler's
// pages, where every field exists whether or not anyone answered it.

// InfoValueType constrains how a catalogue field is entered and rendered.
type InfoValueType string

const (
	// InfoText is free prose. The overwhelming majority.
	InfoText InfoValueType = "text"
	// InfoEnum is one of the catalogue entry's EnumOptions. Used where the
	// answer needs to be comparable across events rather than merely readable
	// — age_requirement is the only one today.
	InfoEnum InfoValueType = "enum"
	// InfoTime is HH:MM in the event's local time. Stored as a string rather
	// than a time column because it is a wall-clock time with no date, and
	// giving it a date would invent information (a festival's doors_open of
	// 23:30 belongs to whichever day the client decides, not to one we picked).
	InfoTime InfoValueType = "time"
)

// EventInfoCatalogueEntry is one field an organiser may choose to fill in.
type EventInfoCatalogueEntry struct {
	Key          string        `json:"key"           db:"key"`
	Label        string        `json:"label"         db:"label"`
	Hint         *string       `json:"hint,omitempty" db:"hint"`
	ValueType    InfoValueType `json:"value_type"    db:"value_type"`
	EnumOptions  []string      `json:"enum_options,omitempty" db:"enum_options"`
	DisplayOrder int           `json:"display_order" db:"display_order"`
	// ReviewRequiredWhenPublished tells the dashboard to lock this field on a
	// live event and route changes through the edit-request flow. Exposed to
	// the client so the form can explain WHY a field is locked rather than
	// just disabling it.
	ReviewRequiredWhenPublished bool `json:"review_required_when_published" db:"review_required_when_published"`
	IsActive                    bool `json:"is_active" db:"is_active"`
}

// EventInfoItem is one filled-in field on one event.
//
// Label and ValueType are denormalised from the catalogue at read time so the
// client can render the Information block from this alone, without also
// fetching the catalogue and joining in the app.
type EventInfoItem struct {
	Key       string        `json:"key"        db:"info_key"`
	Label     string        `json:"label"      db:"label"`
	Value     string        `json:"value"      db:"value"`
	ValueType InfoValueType `json:"value_type" db:"value_type"`
	SortOrder int           `json:"sort_order" db:"sort_order"`
}

// ── DTOs ─────────────────────────────────────────────────────────────────────

// EventInfoItemDTO is one entry in a replacement request.
type EventInfoItemDTO struct {
	Key   string `json:"key"   binding:"required,max=48"`
	Value string `json:"value" binding:"required,max=600"`
}

// ReplaceEventInfoDTO replaces an event's entire information block.
//
// Replacement rather than per-item CRUD for the same reason organiser links
// are: the organiser edits the whole block as one form, order matters, and
// reconciling adds/edits/removes/reorders across four endpoints would be more
// code on both sides for no gain.
//
// An empty Items array is valid and means "remove all information".
type ReplaceEventInfoDTO struct {
	Items []EventInfoItemDTO `json:"items" binding:"omitempty,max=25,dive"`
}

// ── Validation ───────────────────────────────────────────────────────────────

var (
	ErrInfoUnknownKey     = errors.New("not a recognised information field")
	ErrInfoInactiveKey    = errors.New("this information field is no longer available")
	ErrInfoDuplicateKey   = errors.New("duplicate information field")
	ErrInfoEmptyValue     = errors.New("value cannot be empty")
	ErrInfoValueTooLong   = errors.New("value is too long")
	ErrInfoBadEnumValue   = errors.New("not one of the allowed values")
	ErrInfoBadTimeValue   = errors.New("must be a time in HH:MM format")
	ErrInfoReviewRequired = errors.New("this field cannot be changed on a live event without review")
)

// infoTimePattern matches 24-hour HH:MM, rejecting 24:00 and 09:60.
var infoTimePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// ValidateEventInfoItems checks a replacement set against the catalogue.
//
// `catalogue` is keyed by info key and comes from the database, not from a
// hardcoded list — so an admin deactivating a field takes effect immediately
// rather than on the next deploy.
//
// `enforceReviewLock` is true when the event is PUBLISHED. It is passed in
// rather than inferred here because this package has no view of event status,
// and putting the status check in the caller keeps the rule visible at the
// point where it is decided.
//
// `existing` is the event's current items, used only when enforceReviewLock is
// true: a locked field may be RE-SUBMITTED UNCHANGED without error. Without
// that, an organiser updating their parking note would be blocked because the
// same request also carries the age restriction they did not touch — the form
// submits the whole block, so every save would look like an attempt to change
// a locked field.
func ValidateEventInfoItems(
	in []EventInfoItemDTO,
	catalogue map[string]EventInfoCatalogueEntry,
	enforceReviewLock bool,
	existing map[string]string,
) ([]EventInfoItem, error) {
	out := make([]EventInfoItem, 0, len(in))
	seen := make(map[string]bool, len(in))

	for i, dto := range in {
		key := strings.ToLower(strings.TrimSpace(dto.Key))

		if seen[key] {
			return nil, fmt.Errorf("%s: %w", key, ErrInfoDuplicateKey)
		}
		seen[key] = true

		entry, ok := catalogue[key]
		if !ok {
			return nil, fmt.Errorf("%q: %w", key, ErrInfoUnknownKey)
		}
		if !entry.IsActive {
			return nil, fmt.Errorf("%s: %w", entry.Label, ErrInfoInactiveKey)
		}

		value := strings.TrimSpace(dto.Value)
		if value == "" {
			// Not an error the organiser needs to see as a failure: an empty
			// field means "I did not choose this one". Skip it so the block
			// simply has no row, which is the whole design.
			continue
		}
		if len([]rune(value)) > 600 {
			return nil, fmt.Errorf("%s: %w (max 600 characters)", entry.Label, ErrInfoValueTooLong)
		}

		switch entry.ValueType {
		case InfoEnum:
			if !containsFold(entry.EnumOptions, value) {
				return nil, fmt.Errorf("%s: %w (%s)",
					entry.Label, ErrInfoBadEnumValue, strings.Join(entry.EnumOptions, ", "))
			}
		case InfoTime:
			if !infoTimePattern.MatchString(value) {
				return nil, fmt.Errorf("%s: %w", entry.Label, ErrInfoBadTimeValue)
			}
		}

		// The review lock. Re-submitting the same value is always allowed —
		// see the note on `existing` above.
		if enforceReviewLock && entry.ReviewRequiredWhenPublished {
			prev, had := existing[key]
			if !had || prev != value {
				return nil, fmt.Errorf("%s: %w", entry.Label, ErrInfoReviewRequired)
			}
		}

		out = append(out, EventInfoItem{
			Key:       key,
			Label:     entry.Label,
			Value:     value,
			ValueType: entry.ValueType,
			SortOrder: i,
		})
	}

	// Removing a locked field is a change too. An organiser who deletes the
	// age restriction from a live event has changed what a buyer agreed to
	// just as surely as one who edits it.
	if enforceReviewLock {
		for key, prev := range existing {
			if seen[key] || prev == "" {
				continue
			}
			if entry, ok := catalogue[key]; ok && entry.ReviewRequiredWhenPublished {
				return nil, fmt.Errorf("%s: %w (it cannot be removed)", entry.Label, ErrInfoReviewRequired)
			}
		}
	}

	return out, nil
}

func containsFold(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// InfoItemsByKey turns a list into a key→value map, for the `existing` argument
// of ValidateEventInfoItems.
func InfoItemsByKey(items []EventInfoItem) map[string]string {
	m := make(map[string]string, len(items))
	for _, it := range items {
		m[it.Key] = it.Value
	}
	return m
}