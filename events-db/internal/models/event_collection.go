package models

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// Event collections (migration 000096)
//
// A group of one organiser's events — the dates of a tour, a festival's
// satellite nights — shown on the home feed as ONE card instead of one card
// per date. Unrelated to MerchCollection, which groups merchandise.
// ─────────────────────────────────────────────────────────────────────────────

// EventCollection is the organiser-facing record.
type EventCollection struct {
	ID            string    `json:"id"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Description   *string   `json:"description,omitempty"`
	CoverImageURL *string   `json:"cover_image_url,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Every event in the collection, whatever its status — the organiser sees
	// drafts and past dates too, because they put them there.
	EventCount int `json:"event_count"`
	// How many of those the public can actually see right now (published,
	// public, not yet ended). The feed only folds a collection into one card
	// when this is 2 or more; the dashboard shows it so the organiser can tell
	// why a collection is or is not appearing as a card.
	LiveEventCount int `json:"live_event_count"`
	// Member event ids, soonest first. The dashboard uses these to tick the
	// right boxes and to warn when an event is about to move from another
	// collection.
	EventIDs []string `json:"event_ids"`
}

// EventCollectionRef is the summary attached to an event in the home feed and
// on the event detail page.
//
// ── Why the feed carries a ref ON AN EVENT, not a new card type ──────────────
//
// Home sections are arrays of events, and the shipped mobile builds render
// every element as an event. A new element shape would break those builds the
// day the API deployed. Instead the feed keeps ONE representative event per
// collection and attaches this ref to it: an old build shows that one event (a
// graceful, uncluttered fallback), a new build sees `collection` and renders
// the collection card. Nothing has to update in lockstep.
type EventCollectionRef struct {
	ID            string  `json:"id"`
	Slug          string  `json:"slug"`
	Title         string  `json:"title"`
	CoverImageURL *string `json:"cover_image_url,omitempty"`
	// Live (published, public, not ended) events in the collection.
	EventCount int `json:"event_count"`
	// Span of those live events, for the card's date line ("21 Nov – 23 Dec").
	FirstStart time.Time `json:"first_start"`
	LastStart  time.Time `json:"last_start"`
	// Cheapest live event, for the card's "From R50". 0 means at least one
	// date is free.
	MinPrice float64 `json:"min_price"`
	Currency string  `json:"currency"`
	// Distinct cities, in date order, for "East London · Cape Town · +4".
	Cities []string `json:"cities"`
}

// PublicEventCollection is what GET /v1/event-collections/:slug returns.
type PublicEventCollection struct {
	ID            string           `json:"id"`
	Slug          string           `json:"slug"`
	Title         string           `json:"title"`
	Description   *string          `json:"description,omitempty"`
	CoverImageURL *string          `json:"cover_image_url,omitempty"`
	Organizer     *PublicOrganizer `json:"organizer,omitempty"`
	// Upcoming public dates, soonest first. May be empty once the last date
	// has passed — the page says so rather than 404ing a shared link.
	Events []Event `json:"events"`
}

// CreateEventCollectionDTO — POST /organiser/event-collections.
type CreateEventCollectionDTO struct {
	Title       string   `json:"title" binding:"required,min=2,max=120"`
	Description *string  `json:"description" binding:"omitempty,max=2000"`
	EventIDs    []string `json:"event_ids" binding:"omitempty,max=100,dive,uuid"`
}

// UpdateEventCollectionDTO — PATCH /organiser/event-collections/:id. Sparse:
// an omitted field is left alone. The cover is changed through its own upload
// route, never by URL, so a collection can only ever show an image that went
// through our upload pipeline.
type UpdateEventCollectionDTO struct {
	Title       *string `json:"title" binding:"omitempty,min=2,max=120"`
	Description *string `json:"description" binding:"omitempty,max=2000"`
}

// SetEventCollectionEventsDTO — PUT /organiser/event-collections/:id/events.
// The full membership list: anything not in it is released from the
// collection.
type SetEventCollectionEventsDTO struct {
	EventIDs []string `json:"event_ids" binding:"max=100,dive,uuid"`
}
