package models

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ── Admin catalogue management ───────────────────────────────────────────────

var (
	ErrInfoBadKey          = errors.New("key must be lowercase letters, digits and underscores")
	ErrInfoKeyReserved     = errors.New("that key is reserved")
	ErrInfoEnumNeedsOptions = errors.New("an enum field needs at least two options")
	ErrInfoOptionsOnNonEnum = errors.New("only enum fields can have options")
	ErrInfoTypeLocked      = errors.New("value type cannot be changed once events use this field")
)

var infoKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,47}$`)

// reservedInfoKeys are names that would collide with fields the event payload
// already carries. An info item keyed `refund_policy` would render a second
// Refund Policy row beside the real one, sourced from a different column, and
// the two would drift apart the moment either was edited.
var reservedInfoKeys = map[string]bool{
	"refund_policy":        true,
	"terms_and_conditions": true,
	"title":                true,
	"description":          true,
	"venue":                true,
	"address":              true,
	"start_time":           true,
	"end_time":             true,
	"price":                true,
	"capacity":             true,
	"lineup":               true,
	"tags":                 true,
}

type AdminCreateInfoFieldDTO struct {
	Key                         string   `json:"key"           binding:"required,max=48"`
	Label                       string   `json:"label"         binding:"required,max=80"`
	Hint                        *string  `json:"hint"          binding:"omitempty,max=240"`
	ValueType                   string   `json:"value_type"    binding:"required,oneof=text enum time"`
	EnumOptions                 []string `json:"enum_options"  binding:"omitempty,max=20,dive,max=60"`
	DisplayOrder                int      `json:"display_order"`
	ReviewRequiredWhenPublished bool     `json:"review_required_when_published"`
}

// AdminUpdateInfoFieldDTO — every field is a pointer so "not sent" stays
// distinct from "sent as empty". Clearing a hint is a real action and must not
// be indistinguishable from a client that omitted the key.
//
// Note the absence of Key and ValueType. Neither is editable; see the header of
// admin_event_info_handler.go for why.
type AdminUpdateInfoFieldDTO struct {
	Label                       *string   `json:"label"         binding:"omitempty,max=80"`
	Hint                        *string   `json:"hint"          binding:"omitempty,max=240"`
	EnumOptions                 *[]string `json:"enum_options"  binding:"omitempty,max=20,dive,max=60"`
	DisplayOrder                *int      `json:"display_order"`
	ReviewRequiredWhenPublished *bool     `json:"review_required_when_published"`
}

// ValidateNewInfoField checks a proposed catalogue entry.
func ValidateNewInfoField(dto *AdminCreateInfoFieldDTO) (*EventInfoCatalogueEntry, error) {
	key := strings.ToLower(strings.TrimSpace(dto.Key))

	if !infoKeyPattern.MatchString(key) {
		return nil, fmt.Errorf("%q: %w", dto.Key, ErrInfoBadKey)
	}
	if reservedInfoKeys[key] {
		return nil, fmt.Errorf("%q: %w — the event payload already has this", key, ErrInfoKeyReserved)
	}

	vt := InfoValueType(dto.ValueType)
	opts := trimOptions(dto.EnumOptions)

	// Mirrors chk_info_enum_options in migration 000080, so the API rejects
	// this with a readable message rather than surfacing a constraint
	// violation as a 500.
	if vt == InfoEnum && len(opts) < 2 {
		return nil, ErrInfoEnumNeedsOptions
	}
	if vt != InfoEnum && len(opts) > 0 {
		return nil, ErrInfoOptionsOnNonEnum
	}

	order := dto.DisplayOrder
	if order == 0 {
		// Land new fields above "Additional information" (900) but below the
		// seeded set, rather than at 0, which would put an admin's ad-hoc
		// field ahead of the age requirement.
		order = 500
	}

	entry := &EventInfoCatalogueEntry{
		Key:                         key,
		Label:                       strings.TrimSpace(dto.Label),
		ValueType:                   vt,
		DisplayOrder:                order,
		ReviewRequiredWhenPublished: dto.ReviewRequiredWhenPublished,
		IsActive:                    true,
	}
	if vt == InfoEnum {
		entry.EnumOptions = opts
	}
	if dto.Hint != nil {
		if h := strings.TrimSpace(*dto.Hint); h != "" {
			entry.Hint = &h
		}
	}
	return entry, nil
}

// ValidateInfoFieldUpdate applies a sparse patch to an existing entry.
func ValidateInfoFieldUpdate(
	existing *EventInfoCatalogueEntry,
	dto *AdminUpdateInfoFieldDTO,
) (*EventInfoCatalogueEntry, error) {
	updated := *existing

	if dto.Label != nil {
		l := strings.TrimSpace(*dto.Label)
		if l == "" {
			return nil, errors.New("label cannot be empty")
		}
		updated.Label = l
	}
	if dto.Hint != nil {
		h := strings.TrimSpace(*dto.Hint)
		if h == "" {
			updated.Hint = nil
		} else {
			updated.Hint = &h
		}
	}
	if dto.DisplayOrder != nil {
		updated.DisplayOrder = *dto.DisplayOrder
	}
	if dto.ReviewRequiredWhenPublished != nil {
		updated.ReviewRequiredWhenPublished = *dto.ReviewRequiredWhenPublished
	}
	if dto.EnumOptions != nil {
		if updated.ValueType != InfoEnum {
			return nil, ErrInfoOptionsOnNonEnum
		}
		opts := trimOptions(*dto.EnumOptions)
		if len(opts) < 2 {
			return nil, ErrInfoEnumNeedsOptions
		}
		updated.EnumOptions = opts
	}

	return &updated, nil
}

// OptionsRemovedFromUse returns values that events currently hold but which the
// proposed option list no longer contains.
//
// This is the check that stops an admin quietly breaking organiser forms.
// Dropping "16+" from age_requirement while events hold that value means those
// organisers cannot save their own information block — the validator rejects a
// value they never chose to enter, naming a field they did not touch.
//
// Case-insensitive, matching how ValidateEventInfoItems compares enum values.
func OptionsRemovedFromUse(proposed, inUse []string) []string {
	allowed := make(map[string]bool, len(proposed))
	for _, p := range proposed {
		allowed[strings.ToLower(strings.TrimSpace(p))] = true
	}

	var orphaned []string
	seen := map[string]bool{}
	for _, v := range inUse {
		lower := strings.ToLower(strings.TrimSpace(v))
		if lower == "" || allowed[lower] || seen[lower] {
			continue
		}
		seen[lower] = true
		orphaned = append(orphaned, v)
	}
	return orphaned
}

func trimOptions(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		t := strings.TrimSpace(s)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}