package models

import "time"

type CartItemType string

const (
	CartItemTypeTicket CartItemType = "ticket"
	CartItemTypeMerch  CartItemType = "merchandise"
)

type Cart struct {
	ID             string     `json:"id" db:"id"`
	UserID         *string    `json:"user_id,omitempty" db:"user_id"`
	SessionID      *string    `json:"session_id,omitempty" db:"session_id"`
	LastActivityAt time.Time  `json:"last_activity_at" db:"last_activity_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	Items          []CartItem `json:"items,omitempty" db:"-"`
}

type CartItem struct {
	ID                   string       `json:"id" db:"id"`
	CartID               string       `json:"cart_id" db:"cart_id"`
	ItemType             CartItemType `json:"item_type" db:"item_type"`
	EventID              string       `json:"event_id" db:"event_id"`
	MerchandiseVariantID *string      `json:"merchandise_variant_id,omitempty" db:"merchandise_variant_id"`
	Quantity             int          `json:"quantity" db:"quantity"`
	UnitPriceSnapshot    float64      `json:"unit_price_snapshot" db:"unit_price_snapshot"`
	CreatedAt            time.Time    `json:"created_at" db:"created_at"`

	// Joined display fields — populated by GetCartWithItems, not stored columns
	EventTitle   *string `json:"event_title,omitempty" db:"event_title"`
	VariantSKU   *string `json:"variant_sku,omitempty" db:"variant_sku"`
	MerchName    *string `json:"merch_name,omitempty" db:"merch_name"`
	VariantAttrs *string `json:"variant_attributes,omitempty" db:"variant_attributes"`
}

// DTOs — json binding only, no db tags needed

type AddCartItemDTO struct {
	ItemType             CartItemType `json:"item_type" binding:"required,oneof=ticket merchandise"`
	EventID              string       `json:"event_id" binding:"required,uuid"`
	MerchandiseVariantID *string      `json:"merchandise_variant_id" binding:"omitempty,uuid"`
	Quantity             int          `json:"quantity" binding:"required,min=1,max=10"`
}

type UpdateCartItemDTO struct {
	Quantity int `json:"quantity" binding:"required,min=1,max=10"`
}

type MergeCartDTO struct {
	SessionID string `json:"session_id" binding:"required,uuid"`
}

// CartSummary is returned by GET /cart, grouped by event

type CartSummary struct {
	TotalItems   int          `json:"total_items"`
	TotalAmount  float64      `json:"total_amount"`
	Currency     string       `json:"currency"`
	ItemsByEvent []EventGroup `json:"items_by_event"`
}

type EventGroup struct {
	EventID    string     `json:"event_id"`
	EventTitle string     `json:"event_title"`
	Items      []CartItem `json:"items"`
	Subtotal   float64    `json:"subtotal"`
}