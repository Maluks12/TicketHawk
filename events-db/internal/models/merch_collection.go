package models

import "time"

// MerchCollection is a curated group of merchandise items.
// Organizers and admins create collections; the app uses them to drive the
// carousel / CollectionModal instead of relying on event_id grouping.
type MerchCollection struct {
	ID              string    `json:"id" db:"id"`
	// OrganizerID is the user_id from the JWT for organiser-created collections.
	// Nil for admin-created collections.
	OrganizerID     *string   `json:"organizer_id,omitempty" db:"organizer_id"`
	Name            string    `json:"name" db:"name"`
	Description     *string   `json:"description,omitempty" db:"description"`
	CoverImageURL   *string   `json:"cover_image_url,omitempty" db:"cover_image_url"`
	DisplayOrder    int       `json:"display_order" db:"display_order"`
	IsActive        bool      `json:"is_active" db:"is_active"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`

	// Items is populated when fetching a collection with its merchandise.
	// Not stored — joined by the repository layer.
	Items []Merchandise `json:"items,omitempty" db:"-"`
}

// ── DTOs ─────────────────────────────────────────────────────────────────────

type CreateCollectionDTO struct {
	Name          string  `json:"name"          binding:"required,min=1,max=255"`
	Description   *string `json:"description"`
	CoverImageURL *string `json:"cover_image_url"`
	DisplayOrder  int     `json:"display_order"`
}

type UpdateCollectionDTO struct {
	Name           *string `json:"name"         binding:"omitempty,min=1,max=255"`
	Description    *string `json:"description"`
	CoverImageURL  *string `json:"cover_image_url"`
	DisplayOrder   *int    `json:"display_order"`
	IsActive       *bool   `json:"is_active"`
}

// AssignCollectionDTO is the body for PUT /organiser/merch/:id/collection
// and PUT /admin/merch/:id/collection.
// Set CollectionID to a valid UUID to assign, or null/omit to un-assign.
type AssignCollectionDTO struct {
	CollectionID *string `json:"collection_id"`
}