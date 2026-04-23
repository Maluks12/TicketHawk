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
	// TicketTypeID is set when the buyer chose a specific pricing tier.
	// Nil for flat-priced events or merch items.
	TicketTypeID      *string `json:"ticket_type_id,omitempty" db:"ticket_type_id"`
	Quantity          int     `json:"quantity" db:"quantity"`
	UnitPriceSnapshot float64 `json:"unit_price_snapshot" db:"unit_price_snapshot"`

	// Fulfillment fields — only set for merch items.
	// FulfillmentMethod is the buyer's choice: "delivery" or "pickup".
	// DeliveryAddress is stored as JSON text (JSONB in DB) for delivery orders.
	// DeliveryFeeSnapshot is a flat fee captured at add-to-cart time; it is
	// charged once per line item regardless of quantity.
	FulfillmentMethod   *string `json:"fulfillment_method,omitempty" db:"fulfillment_method"`
	DeliveryAddress     *string `json:"delivery_address,omitempty" db:"delivery_address"` // JSONB::text
	DeliveryFeeSnapshot float64 `json:"delivery_fee_snapshot" db:"delivery_fee_snapshot"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`

	// Joined display fields — populated by GetCartWithItems, not stored columns
	EventTitle     *string `json:"event_title,omitempty" db:"event_title"`
	VariantSKU     *string `json:"variant_sku,omitempty" db:"variant_sku"`
	MerchName      *string `json:"merch_name,omitempty" db:"merch_name"`
	VariantAttrs   *string `json:"variant_attributes,omitempty" db:"variant_attributes"`
	TicketTypeName *string `json:"ticket_type_name,omitempty" db:"ticket_type_name"`
	// PickupLocation is joined from merchandise so the cart can display "pick up at X"
	PickupLocation *string `json:"pickup_location,omitempty" db:"pickup_location"`
}

// LineTotal returns the full cost of this cart item including any delivery fee.
func (ci *CartItem) LineTotal() float64 {
	return ci.UnitPriceSnapshot*float64(ci.Quantity) + ci.DeliveryFeeSnapshot
}

// DTOs — json binding only, no db tags needed

type AddCartItemDTO struct {
	ItemType             CartItemType `json:"item_type" binding:"required,oneof=ticket merchandise"`
	EventID              string       `json:"event_id" binding:"required,uuid"`
	MerchandiseVariantID *string      `json:"merchandise_variant_id" binding:"omitempty,uuid"`
	// TicketTypeID selects the pricing tier. Required when the event uses ticket
	// types; omit for flat-priced events or merch items.
	TicketTypeID *string `json:"ticket_type_id" binding:"omitempty,uuid"`
	Quantity     int     `json:"quantity" binding:"required,min=1,max=10"`

	// Fulfillment — required for merch items; ignored for tickets.
	// Must match the item's fulfillment_mode. For delivery, DeliveryAddress is
	// also required.
	FulfillmentMethod *string          `json:"fulfillment_method" binding:"omitempty,oneof=delivery pickup"`
	DeliveryAddress   *DeliveryAddress `json:"delivery_address"`
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