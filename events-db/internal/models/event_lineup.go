package models

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ── Line-up ──────────────────────────────────────────────────────────────────
//
// Replaces the events.performers and events.teams text[] columns, which could
// hold a name and nothing else. Those columns still exist and are still written
// on every save — see the dual-write note in migration 000081 — because shipped
// mobile builds read them directly.

// LineupKind separates the two layouts the client already branches on.
type LineupKind string

const (
	// LineupPerformer is an artist, act or speaker. Rendered as a list.
	LineupPerformer LineupKind = "performer"
	// LineupTeam is one side of a fixture. Rendered as a matchup.
	LineupTeam LineupKind = "team"
)

func IsValidLineupKind(s string) bool {
	return LineupKind(s) == LineupPerformer || LineupKind(s) == LineupTeam
}

// LineupEntry is one act or team on an event.
type LineupEntry struct {
	ID       string     `json:"id"                  db:"id"`
	Kind     LineupKind `json:"kind"                db:"kind"`
	Name     string     `json:"name"                db:"name"`
	ImageURL *string    `json:"image_url,omitempty" db:"image_url"`
	Subtitle *string    `json:"subtitle,omitempty"  db:"subtitle"`
	Stage    *string    `json:"stage,omitempty"     db:"stage"`
	// SetTime is a full timestamp rather than a time of day. Most festivals
	// cross midnight, so a bare 01:30 is ambiguous about which day it belongs
	// to — and getting that wrong puts the headliner 24 hours out.
	SetTime     *time.Time `json:"set_time,omitempty"     db:"set_time"`
	ExternalURL *string    `json:"external_url,omitempty" db:"external_url"`
	SortOrder   int        `json:"sort_order"             db:"sort_order"`
}

// ── DTOs ─────────────────────────────────────────────────────────────────────

type LineupEntryDTO struct {
	Kind        string     `json:"kind"         binding:"omitempty,oneof=performer team"`
	Name        string     `json:"name"         binding:"required,max=120"`
	ImageURL    *string    `json:"image_url"    binding:"omitempty,url,max=500"`
	Subtitle    *string    `json:"subtitle"     binding:"omitempty,max=80"`
	Stage       *string    `json:"stage"        binding:"omitempty,max=80"`
	SetTime     *time.Time `json:"set_time"`
	ExternalURL *string    `json:"external_url" binding:"omitempty,max=500"`
}

// ReplaceEventLineupDTO replaces an event's whole line-up.
//
// Replacement rather than per-entry CRUD, matching organiser links and event
// information: the organiser edits the line-up as one list, order is part of
// the content, and reconciling adds/edits/removes/reorders across four
// endpoints would be more code on both sides for no gain.
//
// An empty array is valid and means "remove the whole line-up".
type ReplaceEventLineupDTO struct {
	Entries []LineupEntryDTO `json:"entries" binding:"omitempty,max=200,dive"`
}

// ── Validation ───────────────────────────────────────────────────────────────

var (
	ErrLineupNoName          = errors.New("every line-up entry needs a name")
	ErrLineupDuplicateName   = errors.New("duplicate name in line-up")
	ErrLineupBadKind         = errors.New("kind must be performer or team")
	ErrLineupBadExternalURL  = errors.New("link must be a valid https:// URL")
	ErrLineupSetTimeOutOfRange = errors.New("set time is not close to the event")
)

// LineupSetTimeSlack is how far outside the event window a set time may fall.
//
// Not zero, because real schedules spill: a doors-open acoustic set can precede
// the advertised start, and an after-party act runs past the advertised end.
// Twelve hours is loose enough for both while still catching the mistake this
// guards against — a set time typed with the wrong DATE, which lands a headliner
// a day out and is otherwise invisible until someone turns up to an empty stage.
const LineupSetTimeSlack = 12 * time.Hour

// ValidateLineupEntries checks a replacement set.
//
// eventStart and eventEnd bound the set-time check. Pass zero values to skip it
// — the draft-creation path has no times yet at the point the line-up is
// entered.
func ValidateLineupEntries(
	in []LineupEntryDTO,
	eventStart, eventEnd time.Time,
) ([]LineupEntry, error) {
	out := make([]LineupEntry, 0, len(in))

	// Duplicate detection is per KIND. A charity match can field a team called
	// "Sundowns" and book a DJ called "Sundowns" without that being a mistake;
	// the same act listed twice in one list almost always is.
	seen := make(map[string]bool, len(in))

	for i, dto := range in {
		name := strings.TrimSpace(dto.Name)
		if name == "" {
			return nil, ErrLineupNoName
		}

		kind := strings.TrimSpace(dto.Kind)
		if kind == "" {
			kind = string(LineupPerformer)
		}
		if !IsValidLineupKind(kind) {
			return nil, fmt.Errorf("%q: %w", dto.Kind, ErrLineupBadKind)
		}

		dupKey := kind + "\x00" + strings.ToLower(name)
		if seen[dupKey] {
			return nil, fmt.Errorf("%q: %w", name, ErrLineupDuplicateName)
		}
		seen[dupKey] = true

		entry := LineupEntry{
			Kind:      LineupKind(kind),
			Name:      name,
			ImageURL:  trimPtr(dto.ImageURL),
			Subtitle:  trimPtr(dto.Subtitle),
			Stage:     trimPtr(dto.Stage),
			SortOrder: i,
		}

		if dto.ExternalURL != nil {
			raw := strings.TrimSpace(*dto.ExternalURL)
			if raw != "" {
				normalised, err := validateLineupURL(raw)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", name, err)
				}
				entry.ExternalURL = &normalised
			}
		}

		if dto.SetTime != nil && !dto.SetTime.IsZero() {
			if !eventStart.IsZero() && !eventEnd.IsZero() {
				lo := eventStart.Add(-LineupSetTimeSlack)
				hi := eventEnd.Add(LineupSetTimeSlack)
				if dto.SetTime.Before(lo) || dto.SetTime.After(hi) {
					return nil, fmt.Errorf("%s: %w (check the date, not just the time)",
						name, ErrLineupSetTimeOutOfRange)
				}
			}
			t := *dto.SetTime
			entry.SetTime = &t
		}

		out = append(out, entry)
	}

	return out, nil
}

// validateLineupURL applies the generic protections from the organiser-link
// validator, minus the per-platform host allow-list.
//
// There is no allow-list here because a line-up link legitimately points
// anywhere — an artist's own site, a Spotify page, a Bandcamp, a label. What
// still holds is that it must be https, must not carry credentials, must not be
// a raw IP, and must not be a shortener whose destination can be changed after
// we approved it.
func validateLineupURL(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrLineupBadExternalURL, err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", ErrLineupBadExternalURL
	}
	if u.User != nil {
		// https://evil.example@spotify.com/… renders as a spotify.com link in
		// some clients while resolving elsewhere.
		return "", fmt.Errorf("%w: it must not contain a username or password", ErrLineupBadExternalURL)
	}

	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host == "" || !strings.Contains(host, ".") {
		return "", fmt.Errorf("%w: %q is not a public domain", ErrLineupBadExternalURL, host)
	}
	if isIPLiteral(host) {
		return "", fmt.Errorf("%w: raw IP addresses are not allowed", ErrLineupBadExternalURL)
	}
	if shortenerHosts[host] {
		return "", fmt.Errorf("%w: %s is a link shortener", ErrLineupBadExternalURL, host)
	}

	u.Fragment = ""
	return u.String(), nil
}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// ── Legacy array projection ──────────────────────────────────────────────────

// LineupNamesByKind splits a line-up into the two flat name arrays that
// events.performers and events.teams still hold.
//
// This is what keeps shipped mobile builds working. The app reads those arrays
// directly (EventsApp/app/event/[id].tsx lines 401-402) and a user on an old
// build does not update because we deployed — so every line-up save writes the
// new table AND refreshes the arrays. See ReplaceEventLineup.
//
// Delete this, and the dual write, once the app's minimum supported version
// reads the line-up from the payload instead.
func LineupNamesByKind(entries []LineupEntry) (performers, teams []string) {
	performers = make([]string, 0, len(entries))
	teams = make([]string, 0, 4)
	for _, e := range entries {
		if e.Kind == LineupTeam {
			teams = append(teams, e.Name)
		} else {
			performers = append(performers, e.Name)
		}
	}
	return performers, teams
}