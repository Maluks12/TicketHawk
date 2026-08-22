package models

import (
	"fmt"
	"sort"
	"strings"
)

// ── Line-up change notifications ─────────────────────────────────────────────
//
// Phase 4 lets an organiser change a live event's line-up immediately, without
// review. That is deliberate — see ReplaceEventLineupHandler — but it means an
// organiser can drop a headliner silently, and somebody who paid to see that
// act finds out by arriving.
//
// This closes it by telling the people with a stake, rather than by blocking
// the change.

// NotifEventLineupChanged goes to ticket holders and, when acts are added or
// removed, to the organiser's followers.
const NotifEventLineupChanged NotificationType = "event_lineup_changed"

// LineupSnapshot is the set of names last announced, keyed by kind.
//
// Names ONLY. A stage move, a set-time shift, a new photo or a corrected link
// are real edits, but they are not "the act you paid to see is gone" — and
// notifying on them would train people to swipe the notification away, which
// costs us the one that matters.
type LineupSnapshot map[string][]string

// SnapshotFromEntries builds a normalised, sorted snapshot.
//
// Sorted so that reordering the line-up in the dashboard — dragging the
// headliner above the support act — is NOT a change anyone gets told about.
// Order is presentation; who is playing is the fact.
func SnapshotFromEntries(entries []LineupEntry) LineupSnapshot {
	byKind := make(map[string][]string, 2)
	for _, e := range entries {
		k := string(e.Kind)
		byKind[k] = append(byKind[k], strings.TrimSpace(e.Name))
	}
	for k := range byKind {
		sort.Strings(byKind[k])
	}
	return byKind
}

// LineupDiff is what changed between two snapshots.
type LineupDiff struct {
	Added   []string
	Removed []string
}

// IsMaterial reports whether anyone should be told.
//
// An empty diff means names are identical — the organiser edited a stage, a
// set time or an image, or simply re-saved. Nothing to announce.
func (d LineupDiff) IsMaterial() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0
}

// DiffSnapshots compares announced against current.
//
// Case-insensitive: correcting "drae sativ" to "Drae Sativ" is a typo fix, not
// a line-up change, and firing a "the line-up has changed" push at every ticket
// holder over a capitalisation edit is exactly the kind of thing that gets
// notifications turned off.
func DiffSnapshots(announced, current LineupSnapshot) LineupDiff {
	var d LineupDiff

	seen := map[string]string{} // lowered -> display name
	for _, names := range announced {
		for _, n := range names {
			seen[strings.ToLower(n)] = n
		}
	}

	now := map[string]string{}
	for _, names := range current {
		for _, n := range names {
			now[strings.ToLower(n)] = n
		}
	}

	for lower, display := range now {
		if _, had := seen[lower]; !had {
			d.Added = append(d.Added, display)
		}
	}
	for lower, display := range seen {
		if _, still := now[lower]; !still {
			d.Removed = append(d.Removed, display)
		}
	}

	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	return d
}

// NotificationBody writes the message a ticket holder reads.
//
// Names the acts rather than saying "the line-up has been updated". The whole
// point is that someone who bought a ticket to see a specific act learns that
// act is gone WITHOUT having to open the app and compare — a vague message
// makes them do the work we are trying to save them.
//
// Removals lead. That is the bad news and the reason for the notification; an
// addition is nice, a removal may mean they want a refund.
func (d LineupDiff) NotificationBody(eventTitle string) string {
	var parts []string

	if len(d.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("%s no longer playing", humanList(d.Removed)))
	}
	if len(d.Added) > 0 {
		parts = append(parts, fmt.Sprintf("%s added", humanList(d.Added)))
	}

	if len(parts) == 0 {
		return fmt.Sprintf("The line-up for %s has been updated.", eventTitle)
	}
	return fmt.Sprintf("%s: %s.", eventTitle, strings.Join(parts, "; "))
}

// humanList renders up to three names, then falls back to a count.
//
// A festival dropping eleven acts should not produce a notification body that
// is truncated mid-name by the OS — at that point the count IS the message and
// the detail belongs on the page.
func humanList(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	case 3:
		return names[0] + ", " + names[1] + " and " + names[2]
	default:
		return fmt.Sprintf("%s, %s and %d others", names[0], names[1], len(names)-2)
	}
}

// LineupChangePayload is the Data on a NotifEventLineupChanged row.
type LineupChangePayload struct {
	EventID    string   `json:"event_id"`
	EventSlug  string   `json:"event_slug"`
	EventTitle string   `json:"event_title"`
	Added      []string `json:"added,omitempty"`
	Removed    []string `json:"removed,omitempty"`
	// HasRemovals lets the client style a removal differently from an addition
	// without re-deriving it from the arrays. A dropped headliner may warrant a
	// refund prompt; a new support act does not.
	HasRemovals bool `json:"has_removals"`
}