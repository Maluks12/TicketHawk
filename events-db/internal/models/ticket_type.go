package models

import "time"

// TicketType represents a named pricing tier within an event (e.g. "General
// Admission", "VIP Pit", "Early Bird"). An event may have zero types (flat
// price) or many. When types exist, events.price is kept as the cheapest
// visible type by a DB trigger.
type TicketType struct {
	ID            string     `json:"id" db:"id"`
	EventID       string     `json:"event_id" db:"event_id"`
	Name          string     `json:"name" db:"name"`
	Description   *string    `json:"description,omitempty" db:"description"`
	Price         float64    `json:"price" db:"price"`
	Capacity      *int       `json:"capacity,omitempty" db:"capacity"`
	TicketsSold   int        `json:"tickets_sold" db:"tickets_sold"`
	ReservedCount int        `json:"reserved_count" db:"reserved_count"`
	// Computed fields — not stored, populated by ComputeAvailable()
	AvailableTickets int  `json:"available_tickets" db:"-"`
	IsSoldOut        bool `json:"is_sold_out" db:"-"`
	SortOrder        int  `json:"sort_order" db:"sort_order"`
	IsVisible        bool `json:"is_visible" db:"is_visible"`
	// Optional sale window; nil means always on sale while visible
	SaleStartsAt *time.Time `json:"sale_starts_at,omitempty" db:"sale_starts_at"`
	SaleEndsAt   *time.Time `json:"sale_ends_at,omitempty" db:"sale_ends_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

// ComputeAvailable fills AvailableTickets and IsSoldOut from capacity counters.
func (t *TicketType) ComputeAvailable() {
	if t.Capacity != nil {
		t.AvailableTickets = *t.Capacity - t.TicketsSold - t.ReservedCount
		if t.AvailableTickets < 0 {
			t.AvailableTickets = 0
		}
		t.IsSoldOut = t.AvailableTickets == 0
	} else {
		t.AvailableTickets = UnlimitedTickets // -1 sentinel from events.go
		t.IsSoldOut = false
	}
}

// IsOnSale returns true when this type is visible and within its sale window.
func (t *TicketType) IsOnSale() bool {
	if !t.IsVisible {
		return false
	}
	now := time.Now()
	if t.SaleStartsAt != nil && now.Before(*t.SaleStartsAt) {
		return false
	}
	if t.SaleEndsAt != nil && now.After(*t.SaleEndsAt) {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

// CreateTicketTypeDTO is used both in the standalone POST and inside
// ReplaceTicketTypesDTO when redefining all types at once.
type CreateTicketTypeDTO struct {
	Name         string     `json:"name" binding:"required,min=1,max=100"`
	Description  *string    `json:"description"`
	Price        float64    `json:"price" binding:"min=0"` // 0 = free tier
	Capacity     *int       `json:"capacity" binding:"omitempty,min=1"`
	SortOrder    int        `json:"sort_order"`
	IsVisible    *bool      `json:"is_visible"` // nil → defaults to TRUE in DB
	SaleStartsAt *time.Time `json:"sale_starts_at"`
	SaleEndsAt   *time.Time `json:"sale_ends_at"`
}

// UpdateTicketTypeDTO allows partial updates on a single type.
type UpdateTicketTypeDTO struct {
	Name         *string    `json:"name" binding:"omitempty,min=1,max=100"`
	Description  *string    `json:"description"`
	Price        *float64   `json:"price" binding:"omitempty,min=0"`
	Capacity     *int       `json:"capacity" binding:"omitempty,min=1"`
	SortOrder    *int       `json:"sort_order"`
	IsVisible    *bool      `json:"is_visible"`
	SaleStartsAt *time.Time `json:"sale_starts_at"`
	SaleEndsAt   *time.Time `json:"sale_ends_at"`
}

// ReplaceTicketTypesDTO is used in PUT /organiser/events/:id/ticket-types to
// atomically swap out all types for an event in one request.
type ReplaceTicketTypesDTO struct {
	Types []CreateTicketTypeDTO `json:"types" binding:"required,min=1,dive"`
}