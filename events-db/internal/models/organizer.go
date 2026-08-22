package models

import "time"

// OrganizerStatus reflects account standing, NOT a registration queue.
// Accounts are auto-approved on self-registration (Meta Business style).
type OrganizerStatus string

const (
	OrganizerActive    OrganizerStatus = "active"    // set automatically on self-registration
	OrganizerSuspended OrganizerStatus = "suspended" // temporarily blocked by admin
	OrganizerBanned    OrganizerStatus = "banned"    // permanently removed by admin

	// Legacy DB values - kept so existing rows don't break. Do not write for new accounts.
	OrganizerPending  OrganizerStatus = "pending"
	OrganizerApproved OrganizerStatus = "approved"
	OrganizerRejected OrganizerStatus = "rejected"
)

type Organizer struct {
	ID               string          `json:"id" db:"id"`
	UserID           string          `json:"user_id" db:"user_id"`
	// Slug is the organiser's public URL segment (/organiser/<slug>). Assigned
	// once by a DB trigger on insert and never regenerated on rename — see
	// migration 000077 for why.
	Slug             string          `json:"slug" db:"slug"`
	BusinessName     string          `json:"business_name" db:"business_name"`
	BusinessEmail    string          `json:"business_email" db:"business_email"`
	BusinessPhone    string          `json:"business_phone" db:"business_phone"`
	TaxID            *string         `json:"tax_id,omitempty" db:"tax_id"`
	WebsiteURL       *string         `json:"website_url,omitempty" db:"website_url"`
	Description      *string         `json:"description,omitempty" db:"description"`
	LogoURL          *string         `json:"logo_url,omitempty" db:"logo_url"`
	// CoverImageURL is the wide hero on the public organiser page. LogoURL
	// stays the small square mark used on event cards and in the sidebar.
	CoverImageURL    *string         `json:"cover_image_url,omitempty" db:"cover_image_url"`
	// PublicProfileEnabled is the organiser's own kill switch for their public
	// page. It is NOT the only gate — see CanHavePublicProfile.
	PublicProfileEnabled bool        `json:"public_profile_enabled" db:"public_profile_enabled"`
	Verified         bool            `json:"verified" db:"verified"`
	VerifiedAt       *time.Time      `json:"verified_at,omitempty" db:"verified_at"`
	VerifiedBy       *string         `json:"verified_by,omitempty" db:"verified_by"`
	// KYCLevel mirrors organizers.kyc_level. 0 = none, 1 = basic (private free
	// events only), 2 = full (paid and public events). Carried on the model
	// because the public-profile gate depends on it; see KYCLevelFull in
	// kyc.go.
	KYCLevel         int             `json:"kyc_level" db:"kyc_level"`
	CommissionRate   float64         `json:"commission_rate" db:"commission"`
	StripeAccountID  *string         `json:"-" db:"stripe_account_id"`
	PayoutEnabled    bool            `json:"payout_enabled" db:"payout_enabled"`
	BankAccountLast4 *string         `json:"bank_account_last_4,omitempty" db:"bank_account_last_4"`
	Status           OrganizerStatus `json:"status" db:"status"`
	RejectionReason  *string         `json:"rejection_reason,omitempty" db:"rejection_reason"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}

// CanHavePublicProfile reports whether this organiser's public page should
// resolve.
//
// Three independent conditions, all required:
//
//   - kyc_level >= 2. This is the same bar the platform already applies to
//     creating a PUBLIC event (migration 000057), so it excludes nobody who
//     could plausibly want the page — a level-1 organiser can only run private
//     free events and has nothing to list. What it does exclude is a brand-new
//     unverified account getting an indexable page with outbound links on day
//     one, which is the shape the abuse would take.
//
//     Deliberately NOT gated on payout_tier: that flips to 'instant' on a
//     settlement-risk judgement about volume and trust, and a perfectly
//     legitimate organiser running their first paid event sits at 'held'.
//     Hiding their page then would hurt exactly the person the page is for.
//
//   - The organiser has not switched it off themselves.
//
//   - The account is not suspended or banned. A suspended organiser's page
//     going dark is the point of a suspension.
func (o *Organizer) CanHavePublicProfile() bool {
	if o == nil {
		return false
	}
	if o.KYCLevel < KYCLevelFull {
		return false
	}
	if !o.PublicProfileEnabled {
		return false
	}
	switch o.Status {
	case OrganizerSuspended, OrganizerBanned, OrganizerRejected:
		return false
	}
	return true
}

// ── Public projection ────────────────────────────────────────────────────────

// PublicOrganizer is what an unauthenticated visitor is allowed to see.
//
// This exists as a SEPARATE STRUCT on purpose. Marshalling Organizer straight
// onto a public endpoint would ship business_email, business_phone,
// commission_rate, payout_enabled, bank_account_last_4, status and
// rejection_reason to anyone who loads an event page — several of which are
// commercially sensitive and one of which (the phone number) is personal data
// the organiser gave us for support contact, not for publication.
//
// Adding a field to Organizer must never silently widen what is public, so the
// projection is built by hand in the repository rather than by embedding.
type PublicOrganizer struct {
	ID            string  `json:"id"`
	Slug          string  `json:"slug"`
	BusinessName  string  `json:"business_name"`
	Description   *string `json:"description,omitempty"`
	LogoURL       *string `json:"logo_url,omitempty"`
	CoverImageURL *string `json:"cover_image_url,omitempty"`
	Verified      bool    `json:"verified"`
	// MemberSince is the year only. The exact registration timestamp is
	// operational detail with no reason to be public.
	MemberSince int `json:"member_since"`
	// EventsHosted counts PUBLISHED public events, past and upcoming. Ticket
	// and revenue figures are deliberately absent: they would let a competitor
	// size an organiser's business from a page we publish on their behalf.
	EventsHosted int `json:"events_hosted"`
	// FollowerCount is populated in phase 2 (organiser follows).
	FollowerCount int64 `json:"follower_count"`
	// IsFollowing is set only when the request carried a valid token. A
	// pointer, not a bool, so a logged-out visitor gets the key omitted rather
	// than a hard `false` — the client renders an unauthenticated follow button
	// differently from a "you are not following" one, and cannot tell those
	// apart from a plain false.
	IsFollowing *bool `json:"is_following,omitempty"`
	// Links is nil rather than empty when the organiser has set none, so the
	// client can distinguish "no socials" from "not loaded".
	Links []OrganizerLink `json:"links,omitempty"`
}

// ── Links ────────────────────────────────────────────────────────────────────

// OrganizerLinkPlatform is the closed set of platforms an organiser may link.
// Mirrored by chk_organizer_link_platform in migration 000077 — adding one
// means adding it in BOTH places, plus a host entry in the allow-list used by
// ValidateOrganizerLink.
type OrganizerLinkPlatform string

const (
	LinkInstagram OrganizerLinkPlatform = "instagram"
	LinkX         OrganizerLinkPlatform = "x"
	LinkFacebook  OrganizerLinkPlatform = "facebook"
	LinkTikTok    OrganizerLinkPlatform = "tiktok"
	LinkYouTube   OrganizerLinkPlatform = "youtube"
	LinkLinkedIn  OrganizerLinkPlatform = "linkedin"
	LinkWhatsApp  OrganizerLinkPlatform = "whatsapp"
	LinkSpotify   OrganizerLinkPlatform = "spotify"
	LinkWebsite   OrganizerLinkPlatform = "website"
)

// OrganizerLinkPlatforms is the display order used when an organiser has not
// set one explicitly, and the canonical list for client-side pickers.
var OrganizerLinkPlatforms = []OrganizerLinkPlatform{
	LinkInstagram, LinkX, LinkFacebook, LinkTikTok,
	LinkYouTube, LinkLinkedIn, LinkWhatsApp, LinkSpotify, LinkWebsite,
}

// IsValidLinkPlatform reports whether s names a platform we accept.
func IsValidLinkPlatform(s string) bool {
	for _, p := range OrganizerLinkPlatforms {
		if string(p) == s {
			return true
		}
	}
	return false
}

// OrganizerLink is one social or website link on an organiser's public page.
type OrganizerLink struct {
	Platform  OrganizerLinkPlatform `json:"platform"  db:"platform"`
	URL       string                `json:"url"       db:"url"`
	SortOrder int                   `json:"sort_order" db:"sort_order"`
}

// ── DTOs ─────────────────────────────────────────────────────────────────────

// CreateOrganizerDTO — user self-registers as an organiser with no admin gate.
type CreateOrganizerDTO struct {
	BusinessName  string  `json:"business_name" binding:"required,min=2,max=255"`
	BusinessEmail string  `json:"business_email" binding:"required,email"`
	BusinessPhone string  `json:"business_phone" binding:"required"`
	WebsiteURL    *string `json:"website_url" binding:"omitempty,url"`
	Description   string  `json:"description" binding:"required,min=50,max=2000"`
}

type UpdateOrganizerDTO struct {
	BusinessName  *string `json:"business_name" binding:"required,min=2,max=255"`
	BusinessEmail *string `json:"business_email" binding:"required,email"`
	BusinessPhone *string `json:"business_phone" binding:"required"`
	WebsiteURL    *string `json:"website_url" binding:"omitempty,url"`
	Description   *string `json:"description" binding:"required,min=50,max=2000"`
	LogoURL       *string `json:"logo_url"`
}

// UpdatePublicProfileDTO — the organiser editing their own public page.
//
// Every field is a pointer so "not sent" and "sent as empty" stay distinct:
// clearing a description is a real action and must not be indistinguishable
// from a client that simply didn't include the key.
type UpdatePublicProfileDTO struct {
	Description          *string `json:"description" binding:"omitempty,max=2000"`
	CoverImageURL        *string `json:"cover_image_url" binding:"omitempty,url,max=500"`
	PublicProfileEnabled *bool   `json:"public_profile_enabled"`
}

// OrganizerLinkDTO is one entry in a links replacement request.
//
// URL accepts either a full https URL or a bare handle ("@wildernessbeach").
// ValidateOrganizerLink normalises the handle into the platform's canonical
// URL and rejects a full URL whose host is not on that platform's allow-list —
// so an "Instagram" link cannot be made to point at a payment page.
type OrganizerLinkDTO struct {
	Platform string `json:"platform" binding:"required"`
	URL      string `json:"url" binding:"required,max=500"`
}

// ReplaceOrganizerLinksDTO replaces the organiser's entire link set in one
// call. Replacement rather than per-link CRUD because the client edits them as
// a single form and the order matters — reconciling adds, edits, removes and
// reorders across four endpoints would be far more code on both sides for no
// benefit.
//
// An empty Links array is a valid request meaning "remove all my links".
type ReplaceOrganizerLinksDTO struct {
	Links []OrganizerLinkDTO `json:"links" binding:"omitempty,max=9,dive"`
}

// AdminUpdateOrganizerStatusDTO — admin suspends or bans an organiser.
// No "approve" action needed — registration is self-service.
type AdminUpdateOrganizerStatusDTO struct {
	Status OrganizerStatus `json:"status" binding:"required"`
	Reason string          `json:"reason" binding:"required,min=10"`
}