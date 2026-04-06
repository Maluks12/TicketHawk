package models

import "time"

// ── Notification types ────────────────────────────────────────────────────────
// Add these constants alongside the existing ones in notification.go.

// NotifEventReminder already existed — the 7-day and 1-day reminders both use
// it, distinguished by the "days_until" field in the notification Data payload.
// NotifEventFollowed is sent to the organiser when someone follows their event.
const (
	NotifEventFollowed NotificationType = "event_followed"
)

// ── Event Follow ──────────────────────────────────────────────────────────────

// EventFollow represents a user's saved/liked event.
type EventFollow struct {
	ID            string    `json:"id"              db:"id"`
	UserID        string    `json:"user_id"         db:"user_id"`
	EventID       string    `json:"event_id"        db:"event_id"`
	Reminded7Days bool      `json:"reminded_7_days" db:"reminded_7_days"`
	Reminded1Day  bool      `json:"reminded_1_day"  db:"reminded_1_day"`
	FollowedAt    time.Time `json:"followed_at"     db:"followed_at"`
}

// FollowedEventRow is the denormalised shape returned by GetFollowedEvents —
// the full event card data joined with the follow metadata so the frontend can
// render the "Following" feed without a second round-trip.
//
// db tags map to the column aliases used in the GetFollowedEvents SELECT so
// any sqlx-style scanner can bind the row directly without manual Scan calls.
type FollowedEventRow struct {
	// Follow metadata
	FollowID   string    `json:"follow_id"   db:"follow_id"`
	FollowedAt time.Time `json:"followed_at" db:"followed_at"`

	// Event card fields (subset of Event — enough for a list card)
	EventID          string      `json:"id"                       db:"event_id"`
	Slug             string      `json:"slug"                     db:"slug"`
	Title            string      `json:"title"                    db:"title"`
	CoverImageURL    *string     `json:"cover_image_url,omitempty" db:"cover_image_url"`
	StartTime        time.Time   `json:"start_time"               db:"start_time"`
	EndTime          time.Time   `json:"end_time"                 db:"end_time"`
	VenueName        string      `json:"venue_name"               db:"venue_name"`
	City             string      `json:"city"                     db:"city"`
	Price            float64     `json:"price"                    db:"price"`
	Currency         string      `json:"currency"                 db:"currency"`
	Status           EventStatus `json:"status"                   db:"status"`
	Category         string      `json:"category"                 db:"category"`
	TicketsSold      int         `json:"tickets_sold"             db:"tickets_sold"`
	MaxCapacity      *int        `json:"max_capacity,omitempty"   db:"max_capacity"`
	// Computed fields — not columns in the DB, so no db tag.
	IsSoldOut        bool        `json:"is_sold_out"`
	AvailableTickets int         `json:"available_tickets"`
}

// FollowToggleResponse is returned by POST /events/:id/follow so the client
// knows the new state without needing a separate GET.
// Pure API response — no db tags needed.
type FollowToggleResponse struct {
	Following   bool   `json:"following"`
	FollowCount int64  `json:"follow_count"`
	EventID     string `json:"event_id"`
}