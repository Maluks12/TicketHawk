package models

import (
	"database/sql/driver"
	"encoding/json"
	//"fmt"
	"time"
)

type EventStatus string

const (
	EventDraft         EventStatus = "draft"
	EventPendingReview EventStatus = "pending_review" // submitted by organiser; awaiting admin approval
	EventPublished     EventStatus = "published"      // admin approved; visible to buyers
	EventRejected      EventStatus = "rejected"       // admin rejected; organiser can revise and resubmit
	EventCancelled     EventStatus = "cancelled"
	EventCompleted     EventStatus = "completed"
	// EventPublishedPrivate: live but link-only. Discovery queries filter on
	// EventPublished, so private events are excluded from feed/search/home
	// automatically; functional paths (booking, slug access, completion)
	// explicitly accept both. See IsLive().
	EventPublishedPrivate EventStatus = "published_private"
)

// IsLive reports whether a status accepts bookings/scans — published events
// (public) and published_private events (link-only) alike.
func (s EventStatus) IsLive() bool {
	return s == EventPublished || s == EventPublishedPrivate
}

type Event struct {
	ID             string      `json:"id" db:"id"`
	Slug           string      `json:"slug" db:"slug"`
	Visibility     string      `json:"visibility" db:"visibility"`
	RightsDeclaredAt *time.Time `json:"rights_declared_at,omitempty" db:"rights_declared_at"`
	Title          string      `json:"title" db:"title"`
	Description    string      `json:"description" db:"description"`
	OrganizerID    string      `json:"organizer_id" db:"organizer_id"`
	CoverImageURL  *string     `json:"cover_image_url,omitempty" db:"cover_image_url"`
	BannerImageURL *string     `json:"banner_image_url,omitempty" db:"banner_image_url"`
	BannerVideoURL *string     `json:"banner_video_url,omitempty" db:"banner_video_url"`
	GalleryImages  ImageArray  `json:"gallery_images,omitempty" db:"gallery_images"`
	StartTime      time.Time   `json:"start_time" db:"start_time"`
	EndTime        time.Time   `json:"end_time" db:"end_time"`
	ExpiresAt      time.Time   `json:"expires_at" db:"expires_at"`
	Timezone       string      `json:"timezone" db:"timezone"`
	VenueName      string      `json:"venue_name" db:"venue_name"`
	Address        string      `json:"address" db:"address"`
	City           string      `json:"city" db:"city"`
	Latitude       *float64    `json:"latitude,omitempty" db:"latitude"`
	Longitude      *float64    `json:"longitude,omitempty" db:"longitude"`
	// Google Places id captured by the organiser's address autocomplete.
	// Clients use it to build EXACT map links (&query_place_id=...) instead of
	// a fuzzy text search that lands on the wrong venue.
	PlaceID        *string     `json:"place_id,omitempty" db:"place_id"`
	CategoryID     int         `json:"category_id" db:"category_id"`
	Category       string      `json:"category" db:"category"`
	// Price reflects the cheapest visible ticket type (kept in sync by a DB
	// trigger). For flat-priced events with no types it is the organiser's
	// manually set price. Frontend should display this as "from R{price}".
	Price          float64     `json:"price" db:"price"`
	Currency       string      `json:"currency" db:"currency"`
	MaxCapacity    *int        `json:"max_capacity,omitempty" db:"max_capacity"`
	TicketsSold    int         `json:"tickets_sold" db:"tickets_sold"`
	AvailableTickets int       `json:"available_tickets" db:"-"`
	IsSoldOut      bool        `json:"is_sold_out" db:"-"`
	AllowWaitlist       bool        `json:"allow_waitlist" db:"allow_waitlist"`
	MaxTicketsPerUser   *int        `json:"max_tickets_per_user,omitempty" db:"max_tickets_per_user"`
	VenueType           *string     `json:"venue_type,omitempty" db:"venue_type"`
	IsFeatured     bool        `json:"is_featured" db:"is_featured"`
	FeaturedUntil  *time.Time  `json:"featured_until,omitempty" db:"featured_until"`
	// AdminPromoted is set by an admin to place the event on the home feed as a
	// zero-cost sponsored card, without the organiser running a paid campaign.
	AdminPromoted   bool       `json:"admin_promoted" db:"admin_promoted"`
	AdminPromotedAt *time.Time `json:"admin_promoted_at,omitempty" db:"admin_promoted_at"`
	Status         EventStatus `json:"status" db:"status"`
	PublishedAt      *time.Time  `json:"published_at,omitempty" db:"published_at"`
	RejectionReason  *string     `json:"rejection_reason,omitempty" db:"rejection_reason"`
	CancelledAt    *time.Time  `json:"cancelled_at,omitempty" db:"cancelled_at"`
	CancellationReason *string `json:"cancellation_reason,omitempty" db:"cancellation_reason"`
	RefundPolicy       *string `json:"refund_policy,omitempty" db:"refund_policy"`
	TermsAndConditions *string `json:"terms_and_conditions,omitempty" db:"terms_and_conditions"`
	Tags           TagArray    `json:"tags,omitempty" db:"tags"`
	Performers     TagArray    `json:"performers,omitempty" db:"performers"`
	Teams          TagArray    `json:"teams,omitempty" db:"teams"`
	Views          int         `json:"views" db:"views"`
	// ListingFeePaid is true once the organizer has settled the platform
	// listing fee (R5 × capacity). Only events with this set appear in the
	// home-feed algorithm. Events with capacity < 20 have a fee of 0 and are
	// published for free — but are also excluded from the home feed until the
	// organizer chooses to pay the listing fee or run an ad campaign.
	ListingFeePaid              bool    `json:"listing_fee_paid" db:"listing_fee_paid"`
	ListingFeeAmount            float64 `json:"listing_fee_amount" db:"listing_fee_amount"`
	ListingFeePaymentRequestID  *string `json:"-" db:"listing_fee_payment_request_id"`

	// IsSponsored is set at feed assembly time — never stored in DB.
	IsSponsored    bool        `json:"is_sponsored,omitempty" db:"-"`
	Organizer      *Organizer  `json:"organizer,omitempty" db:"-"`
	// TicketTypes lists all visible pricing tiers. Populated by GetEventByID /
	// GetEventBySlug. Nil (omitted) on list/feed endpoints for performance.
	TicketTypes    []*TicketType `json:"ticket_types,omitempty" db:"-"`
	CreatedAt      time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at" db:"updated_at"`
}

type ImageArray []string
type TagArray []string

func (a *ImageArray) Scan(value interface{}) error {
	if value == nil {
		*a = ImageArray{}
		return nil
	}
	return json.Unmarshal(value.([]byte), a)
}

func (a ImageArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	return json.Marshal(a)
}

// NOTE: TagArray deliberately has NO custom Scan/Value implementation.
// pgx v5 natively encodes/decodes Postgres text[] into any []string-derived
// type, in both text and binary wire formats. The previous custom Scan was
// written for the TEXT format ({a,b}) but pgx delivers arrays in BINARY
// format, whose payload always begins with a 0x00 dimensions header — the
// old "null bytes" guard (b[0] == 0) therefore treated EVERY array as empty
// and silently returned nothing, which is why tags/performers/teams never
// appeared in any API response despite being stored correctly.


const UnlimitedTickets = -1

// ComputeAvailableTickets fills AvailableTickets and IsSoldOut.
//
// ── Why this consults the tiers ──────────────────────────────────────────────
//
// This used to compare MaxCapacity against TicketsSold and nothing else, which
// is only the whole story for a flat-priced event. Once an event has tiers, the
// real supply is the sum of what the tiers still have, and the two numbers come
// apart in the ordinary case:
//
//   event max_capacity 1000, tickets_sold 120
//   tiers: GA  100/100 sold, VIP 20/20 sold   -> nothing left to buy
//
// The old computation reported is_sold_out=false (120 < 1000), so the event
// card and the detail screen offered a live "Get Tickets" button that led to a
// purchase screen where every tier was greyed out and no CTA rendered. The
// buyer got a dead end with no explanation.
//
// Tiers are only attached on the endpoints that load them (GetEventByID and the
// organiser dashboard); elsewhere TicketTypes is empty and the flat-price path
// below applies exactly as before, so this is additive.
//
// ── "Sold out" is not the same as "not on sale yet" ─────────────────────────
//
// A tier outside its sale window, or hidden, is deliberately NOT counted as
// sold out — an event whose Early Bird opens tomorrow has sold nothing and is
// not sold out. When no tier is currently on sale we report zero availability
// but leave IsSoldOut false, so the UI can say "not on sale" rather than
// telling the buyer they missed it.
func (e *Event) ComputeAvailableTickets() {
	// Event-level cap, computed first: it binds regardless of tiers.
	capRemaining := UnlimitedTickets
	if e.MaxCapacity != nil {
		capRemaining = *e.MaxCapacity - e.TicketsSold
		if capRemaining < 0 {
			capRemaining = 0
		}
	}

	if len(e.TicketTypes) > 0 {
		var (
			onSale       int
			soldOut      int
			sumAvailable int
			hasUnlimited bool
		)
		for _, tt := range e.TicketTypes {
			if tt == nil || !tt.IsOnSale() {
				continue
			}
			onSale++
			tt.ComputeAvailable()
			switch {
			case tt.IsSoldOut:
				soldOut++
			case tt.AvailableTickets == UnlimitedTickets:
				hasUnlimited = true
			default:
				sumAvailable += tt.AvailableTickets
			}
		}

		switch {
		case onSale == 0:
			// Nothing on sale right now. Not sold out — see the note above.
			e.AvailableTickets = 0
			e.IsSoldOut = false
		case soldOut == onSale:
			e.AvailableTickets = 0
			e.IsSoldOut = true
		case hasUnlimited:
			e.AvailableTickets = UnlimitedTickets
			e.IsSoldOut = false
		default:
			e.AvailableTickets = sumAvailable
			e.IsSoldOut = sumAvailable == 0
		}

		// The event cap still overrides: an organiser can cap the venue below the
		// sum of the tiers, and the smaller number is the one that binds.
		if capRemaining != UnlimitedTickets {
			if e.AvailableTickets == UnlimitedTickets || capRemaining < e.AvailableTickets {
				e.AvailableTickets = capRemaining
			}
			if capRemaining == 0 {
				e.IsSoldOut = true
			}
		}
		return
	}

	// Flat-priced event, or an endpoint that does not load tiers.
	if capRemaining != UnlimitedTickets {
		e.AvailableTickets = capRemaining
		e.IsSoldOut = capRemaining == 0
		return
	}
	e.AvailableTickets = UnlimitedTickets
	e.IsSoldOut = false
}

type CreateEventDTO struct {
	Title              string     `json:"title" binding:"required,min=5,max=255"`
	Description        string     `json:"description" binding:"required,min=50"`
	CoverImageURL      *string    `json:"cover_image_url" binding:"omitempty,url"`
	BannerImageURL     *string    `json:"banner_image_url" binding:"omitempty,url"`
	BannerVideoURL     *string    `json:"banner_video_url" binding:"omitempty,url"`
	GalleryImages      []string   `json:"gallery_images"`
	StartTime          time.Time  `json:"start_time" binding:"required"`
	EndTime            time.Time  `json:"end_time" binding:"required"`
	ExpiresAt          time.Time  `json:"expires_at" binding:"required"`
	Timezone           string     `json:"timezone" binding:"required"`
	VenueName          string     `json:"venue_name" binding:"required"`
	Address            string     `json:"address" binding:"required"`
	City               string     `json:"city" binding:"required"`
	Latitude           *float64   `json:"latitude"`
	Longitude          *float64   `json:"longitude"`
	PlaceID            *string    `json:"place_id"`
	CategoryID         int        `json:"category_id" binding:"required"`
	Price              float64    `json:"price" binding:"min=0"`
	Currency           string     `json:"currency" binding:"required"`
	MaxCapacity        *int       `json:"max_capicity" binding:"omitempty,min=1"`
	AllowWaitlist       bool       `json:"allow_waitlist"`
	MaxTicketsPerUser   *int       `json:"max_tickets_per_user" binding:"omitempty,min=1,max=20"`
	VenueType           *string    `json:"venue_type" binding:"omitempty,oneof=stadium auditorium none"`
	RefundPolicy       *string    `json:"refund_policy"`
	TermsAndConditions *string    `json:"terms_and_conditions"`
	Tags               []string   `json:"tags"`
	Performers         []string   `json:"performers"`
	Teams              []string   `json:"teams"`
	// Optional: create ticket types inline with the event. When provided the
	// cheapest type price overwrites the Price field above via DB trigger.
	TicketTypes        []CreateTicketTypeDTO `json:"ticket_types"`
	// Visibility: 'public' (default, discoverable, admin-reviewed) or
	// 'private_link' (link-only, skips review, hidden from feed/search).
	Visibility         *string `json:"visibility" binding:"omitempty,oneof=public private_link"`
	// Scan (admission) window overrides, in MINUTES. nil = default 2h grace on
	// that side. ScanOpenOffsetMin = minutes before start_time scanning opens;
	// ScanCloseOffsetMin = minutes after end_time it closes. Ceiling 7 days.
	ScanOpenOffsetMin  *int `json:"scan_open_offset_min" binding:"omitempty,min=0,max=10080"`
	ScanCloseOffsetMin *int `json:"scan_close_offset_min" binding:"omitempty,min=0,max=10080"`
	// RightsDeclarationAccepted: organizer confirms they hold the rights to
	// host/sell this event. Can also be accepted later at publish time.
	RightsDeclarationAccepted bool `json:"rights_declaration_accepted"`
	// Optional (admin create only): hand the event to an organiser at creation
	// time. The organiser is invited + auto-approved if needed, ownership is
	// set to them, and the handover email fires immediately.
	OrganizerEmail        *string `json:"organizer_email" binding:"omitempty,email,max=255"`
	OrganizerBusinessName *string `json:"organizer_business_name" binding:"omitempty,max=255"`
}

type UpdateEventDTO struct {
	Title              *string    `json:"title" binding:"omitempty,min=5,max=255"`
	Description        *string    `json:"description" binding:"omitempty,min=50"`
	CoverImageURL      *string    `json:"cover_image_url" binding:"omitempty,url"`
	BannerImageURL     *string    `json:"banner_image_url" binding:"omitempty,url"`
	BannerVideoURL     *string    `json:"banner_video_url" binding:"omitempty,url"`
	GalleryImages      *[]string  `json:"gallery_images"`
	StartTime          *time.Time `json:"start_time"`
	EndTime            *time.Time `json:"end_time"`
	ExpiresAt          *time.Time `json:"expires_at"`
	Timezone           *string    `json:"timezone"`
	VenueName          *string    `json:"venue_name"`
	Address            *string    `json:"address"`
	City               *string    `json:"city"`
	Latitude           *float64   `json:"latitude"`
	Longitude          *float64   `json:"longitude"`
	PlaceID            *string    `json:"place_id"`
	Price              *float64   `json:"price" binding:"omitempty,min=0"`
	MaxCapacity        *int       `json:"max_capicity" binding:"omitempty,min=1"`
	AllowWaitlist       *bool      `json:"allow_waitlist"`
	MaxTicketsPerUser   *int       `json:"max_tickets_per_user" binding:"omitempty,min=1,max=20"`
	VenueType           *string    `json:"venue_type" binding:"omitempty,oneof=stadium auditorium none"`
	IsFeatured         *bool      `json:"is_featured"`
	FeaturedUntil      *time.Time `json:"featured_until"`
	RefundPolicy       *string    `json:"refund_policy"`
	TermsAndConditions *string    `json:"terms_and_conditions"`
	Tags               *[]string  `json:"tags"`
	Performers         *[]string  `json:"performers"`
	Teams              *[]string  `json:"teams"`
}

// Category represents a row from the categories table.
type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}