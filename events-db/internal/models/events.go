package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type EventStatus string

const (
	EventDraft     EventStatus = "draft"
	EventPublished EventStatus = "published"
	EventCancelled EventStatus = "cancelled"
	EventCompleted EventStatus = "completed"
)

type Event struct {
	ID             string     `json:"id"              db:"id"`
	Title          string     `json:"title"           db:"title"`
	Description    string     `json:"description"     db:"description"`
	OrganizerID    string     `json:"organizer_id"    db:"organizer_id"`
	CoverImageURL  *string    `json:"cover_image_url,omitempty"  db:"cover_image_url"`
	BannerImageURL *string    `json:"banner_image_url,omitempty" db:"banner_image_url"`
	// BannerVideoURL is optional — the frontend uses the video as primary hero
	// media and falls back to BannerImageURL as the poster frame.
	BannerVideoURL *string    `json:"banner_video_url,omitempty" db:"banner_video_url"`
	GalleryImages  ImageArray `json:"gallery_images,omitempty"   db:"gallery_images"`
	StartTime      time.Time  `json:"start_time"      db:"start_time"`
	EndTime        time.Time  `json:"end_time"        db:"end_time"`
	ExpiresAt      time.Time  `json:"expires_at"      db:"expires_at"`
	Timezone       string     `json:"timezone"        db:"timezone"`
	VenueName      string     `json:"venue_name"      db:"venue_name"`
	Address        string     `json:"address"         db:"address"`
	City           string     `json:"city"            db:"city"`
	Latitude       *float64   `json:"latitude,omitempty"  db:"latitude"`
	Longitude      *float64   `json:"longitude,omitempty" db:"longitude"`
	CategoryID     int        `json:"-"               db:"category_id"`
	Category       string     `json:"category"        db:"category"`
	Price          float64    `json:"price"           db:"price"`
	Currency       string     `json:"currency"        db:"currency"`
	MaxCapacity    *int       `json:"max_capacity,omitempty" db:"max_capacity"`
	TicketsSold    int        `json:"tickets_sold"    db:"tickets_sold"`
	AvailableTickets int      `json:"available_tickets"`
	IsSoldOut      bool       `json:"is_sold_out"`
	AllowWaitlist  bool       `json:"allow_waitlist"  db:"allow_waitlist"`
	IsFeatured     bool       `json:"is_featured"     db:"is_featured"`
	FeaturedUntil  *time.Time `json:"featured_until,omitempty"    db:"featured_until"`
	Status         EventStatus `json:"status"         db:"status"`
	PublishedAt    *time.Time `json:"published_at,omitempty"      db:"published_at"`
	CancelledAt    *time.Time `json:"cancelled_at,omitempty"      db:"cancelled_at"`
	CancellationReason *string `json:"cancellation_reason,omitempty" db:"cancellation_reason"`
	RefundPolicy   *string    `json:"refund_policy,omitempty"     db:"refund_policy"`
	TermsAndConditions *string `json:"terms_and_conditions,omitempty" db:"terms_and_conditions"`
	Tags           TagArray   `json:"tags,omitempty"  db:"tags"`
	Views          int        `json:"views"           db:"views"`
	IsSponsored    bool       `json:"is_sponsored,omitempty" db:"is_sponsored"`
	Organizer      *Organizer `json:"organizer,omitempty"`
	CreatedAt      time.Time  `json:"created_at"      db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"      db:"updated_at"`
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

func (a *TagArray) Scan(value interface{}) error {
	if value == nil {
		*a = TagArray{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into TagArray", value)
	}
	str := string(bytes)
	if str == "{}" || str == "" {
		*a = TagArray{}
		return nil
	}
	return json.Unmarshal(bytes, a)
}

func (a TagArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	return json.Marshal(a)
}

const UnlimitedTickets = -1

func (e *Event) ComputeAvailableTickets() {
	if e.MaxCapacity != nil {
		e.AvailableTickets = *e.MaxCapacity - e.TicketsSold
		if e.AvailableTickets < 0 {
			e.AvailableTickets = 0
		}
		e.IsSoldOut = e.AvailableTickets == 0
	} else {
		e.AvailableTickets = UnlimitedTickets
		e.IsSoldOut = false
	}
}

type CreateEventDTO struct {
	Title       string  `json:"title"       binding:"required,min=5,max=255"`
	Description string  `json:"description" binding:"required,min=50"`
	// Image URLs are set via the /media upload endpoints after event creation.
	// These fields accept pre-existing URLs only — do not use them to upload files.
	CoverImageURL  *string  `json:"cover_image_url"  binding:"omitempty,url"`
	BannerImageURL *string  `json:"banner_image_url" binding:"omitempty,url"`
	// FIX: was binding:"omitempty_url" (invalid — underscore makes it a single
	// unregistered custom validator name). Correct form is "omitempty,url".
	BannerVideoURL *string  `json:"banner_video_url" binding:"omitempty,url"`
	GalleryImages  []string `json:"gallery_images"`
	StartTime      time.Time `json:"start_time" binding:"required"`
	EndTime        time.Time `json:"end_time"   binding:"required"`
	ExpiresAt      time.Time `json:"expires_at" binding:"required"`
	Timezone       string    `json:"timezone"   binding:"required"`
	VenueName      string    `json:"venue_name" binding:"required"`
	Address        string    `json:"address"    binding:"required"`
	City           string    `json:"city"       binding:"required"`
	Latitude       *float64  `json:"latitude"`
	Longitude      *float64  `json:"longitude"`
	CategoryID     int       `json:"category_id" binding:"required"`
	Price          float64   `json:"price"       binding:"required"`
	Currency       string    `json:"currency"    binding:"required"`
	// FIX: json tag was "max_capicity" (typo). Corrected to "max_capacity".
	MaxCapacity    *int      `json:"max_capacity" binding:"omitempty,min=1"`
	AllowWaitlist  bool      `json:"allow_waitlist"`
	RefundPolicy   *string   `json:"refund_policy"`
	TermsAndConditions *string `json:"terms_and_conditions"`
	Tags           []string  `json:"tags"`
}

type UpdateEventDTO struct {
	Title          *string    `json:"title"           binding:"omitempty,min=5,max=255"`
	Description    *string    `json:"description"     binding:"omitempty,min=50"`
	CoverImageURL  *string    `json:"cover_image_url"  binding:"omitempty,url"`
	BannerImageURL *string    `json:"banner_image_url" binding:"omitempty,url"`
	// FIX: same omitempty_url → omitempty,url correction as CreateEventDTO.
	BannerVideoURL *string    `json:"banner_video_url" binding:"omitempty,url"`
	GalleryImages  *[]string  `json:"gallery_images"`
	StartTime      *time.Time `json:"start_time"`
	EndTime        *time.Time `json:"end_time"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Timezone       *string    `json:"timezone"`
	VenueName      *string    `json:"venue_name"`
	Address        *string    `json:"address"`
	City           *string    `json:"city"`
	Latitude       *float64   `json:"latitude"`
	Longitude      *float64   `json:"longitude"`
	Price          *float64   `json:"price"          binding:"omitempty,min=0"`
	// FIX: json tag was "max_capicity" (typo). Corrected to "max_capacity".
	MaxCapacity    *int       `json:"max_capacity"   binding:"omitempty,min=1"`
	AllowWaitlist  *bool      `json:"allow_waitlist"`
	IsFeatured     *bool      `json:"is_featured"`
	FeaturedUntil  *time.Time `json:"featured_until"`
	RefundPolicy   *string    `json:"refund_policy"`
	TermsAndConditions *string `json:"terms_and_conditions"`
	Tags           *[]string  `json:"tags"`
}