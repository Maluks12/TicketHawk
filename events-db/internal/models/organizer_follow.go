package models

import "time"

// ── Organiser follow ─────────────────────────────────────────────────────────
//
// Distinct from EventFollow in follow.go. Following an EVENT means "remind me
// before this happens" and is consumed by the reminder worker. Following an
// ORGANISER means "tell me when they announce something new" and is consumed
// by the announcement worker. They share no state and no lifecycle.

// NotifOrganizerNewEvent is sent to an organiser's followers when one of their
// events goes live publicly.
//
// Declared here rather than in notification.go so that phase 2 adds no lines to
// an existing file — the constant block there is untouched. Go does not care
// which file in the package a const lives in.
const NotifOrganizerNewEvent NotificationType = "organizer_new_event"

// OrganizerFollow is one user's follow of one organiser.
type OrganizerFollow struct {
	ID          string    `json:"id"           db:"id"`
	UserID      string    `json:"user_id"      db:"user_id"`
	OrganizerID string    `json:"organizer_id" db:"organizer_id"`
	FollowedAt  time.Time `json:"followed_at"  db:"followed_at"`
}

// OrganizerFollowToggleResponse is returned by POST /organisers/:slug/follow so
// the client can update the button and the counter without a second call.
//
// Mirrors FollowToggleResponse for events on purpose: the two follow buttons
// behave identically on screen, so giving them different response shapes would
// mean two code paths in the app for one interaction.
type OrganizerFollowToggleResponse struct {
	Following     bool   `json:"following"`
	FollowerCount int64  `json:"follower_count"`
	OrganizerID   string `json:"organizer_id"`
	OrganizerSlug string `json:"organizer_slug"`
}

// FollowedOrganizerRow is one card in the user's "Organisers you follow" list.
//
// Carries UpcomingEvents because a follow list where every row says only a name
// gives the user no reason to tap. The count is the single most useful thing to
// show next to an organiser they have already expressed interest in.
type FollowedOrganizerRow struct {
	OrganizerID   string    `json:"id"`
	Slug          string    `json:"slug"`
	BusinessName  string    `json:"business_name"`
	LogoURL       *string   `json:"logo_url,omitempty"`
	Verified      bool      `json:"verified"`
	FollowedAt    time.Time `json:"followed_at"`
	UpcomingEvents int      `json:"upcoming_events"`
}

// NewEventAnnouncement is the Data payload on a NotifOrganizerNewEvent row.
//
// The client deep-links on EventSlug rather than EventID because the app's
// event route accepts either and the slug is what appears in a shared URL —
// so a notification tap and a shared link land on the same screen through the
// same code path.
type NewEventAnnouncement struct {
	EventID       string `json:"event_id"`
	EventSlug     string `json:"event_slug"`
	EventTitle    string `json:"event_title"`
	OrganizerID   string `json:"organizer_id"`
	OrganizerSlug string `json:"organizer_slug"`
	OrganizerName string `json:"organizer_name"`
	CoverImageURL string `json:"cover_image_url,omitempty"`
}