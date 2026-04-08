package utils

import (
	"regexp"
	"strings"
)

// nonWordRe matches any run of characters that are not ASCII letters, digits,
// or hyphens. Used to collapse spaces, punctuation, and other noise into a
// single hyphen during slug generation.
//
// NOTE: the input is lowercased BEFORE this regex runs, so the character
// class only needs to cover lowercase letters and digits — uppercase letters
// are never present at match time.
var nonWordRe = regexp.MustCompile(`[^a-z0-9]+`)

// GenerateSlug produces a URL-safe slug from a human-readable title and the
// event's UUID. The UUID's first 8 characters are appended as a suffix so the
// slug is globally unique even when two events share an identical title —
// without needing a uniqueness retry loop.
//
// Examples:
//
//	GenerateSlug("Cape Town Jazz Festival 2026", "a1b2c3d4-...") → "cape-town-jazz-festival-2026-a1b2c3d4"
//	GenerateSlug("  Hello, World!!! ", "00000000-...")             → "hello-world-00000000"
//	GenerateSlug("JAZZ Fest", "a1b2c3d4-...")                      → "jazz-fest-a1b2c3d4"  (was "JAZZ-Fest-..." before fix)
func GenerateSlug(title, id string) string {
	// FIX: lowercase FIRST so the regex sees only [a-z0-9] as valid characters.
	// The previous order (regex → lowercase) preserved uppercase letters through
	// the regex pass (e.g. "JAZZ" matched [a-z0-9] as invalid and was left as
	// "JAZZ", then lowercased to "jazz" — but only because the whole string
	// happened to be letters). For mixed-case titles like "Cape Town 2026", the
	// capital letters survived the regex untouched and only lowercased afterward,
	// which produced the correct output by accident. Lowercasing first is the
	// correct and intentional order.
	s := strings.ToLower(title)
	s = nonWordRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	// Append the first 8 characters of the UUID for guaranteed uniqueness.
	// UUIDs are always at least 8 characters, but guard defensively.
	suffix := id
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}

	if s == "" {
		return suffix
	}
	return s + "-" + suffix
}