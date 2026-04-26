package models

import "time"

// FulfillmentStatus represents whether a merch order was successfully fulfilled.
type FulfillmentStatus string

const (
	// FulfillmentFulfilled means stock was decremented and the order is complete.
	FulfillmentFulfilled FulfillmentStatus = "fulfilled"

	// FulfillmentOversold means payment was captured but stock was exhausted at
	// fulfillment time. The admin must resolve this manually (refund or restock).
	// This can occur because merch stock is not pre-reserved at checkout.
	FulfillmentOversold FulfillmentStatus = "oversold"
)

// MerchOrder is the permanent record of a merchandise sale. It is created
// atomically with the stock decrement inside the payment webhook handler,
// only after Stitch confirms a successful payment.
type MerchOrder struct {
	ID                 string            `json:"id" db:"id"`
	CartID             *string           `json:"cart_id,omitempty" db:"cart_id"`
	MerchandiseID      string            `json:"merchandise_id" db:"merchandise_id"`
	VariantID          string            `json:"variant_id" db:"variant_id"`
	EventID            string            `json:"event_id" db:"event_id"`
	UserID             *string           `json:"user_id,omitempty" db:"user_id"`
	SessionID          *string           `json:"session_id,omitempty" db:"session_id"`
	Quantity           int               `json:"quantity" db:"quantity"`
	UnitPrice          float64           `json:"unit_price" db:"unit_price"`
	TotalAmount        float64           `json:"total_amount" db:"total_amount"`
	StitchPaymentReqID string            `json:"stitch_payment_req_id" db:"stitch_payment_req_id"`
	FulfillmentStatus  FulfillmentStatus `json:"fulfillment_status" db:"fulfillment_status"`

	// Delivery / pickup snapshot — captured from cart_items at payment confirmation
	// so the organiser always knows how to fulfil even if the cart is later cleaned up.
	//
	// FulfillmentMethod: "delivery" or "pickup" (nil for pre-migration orders).
	// DeliveryAddress:   JSONB text snapshot of the buyer's shipping address.
	// PickupLocation:    Snapshot of merchandise.pickup_location at time of order.
	FulfillmentMethod *string `json:"fulfillment_method,omitempty" db:"fulfillment_method"`
	DeliveryAddress   *string `json:"delivery_address,omitempty" db:"delivery_address"` // JSONB::text
	PickupLocation    *string `json:"pickup_location,omitempty" db:"pickup_location"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`

	// Joined display fields — populated by dashboard queries, never stored columns.
	MerchName  *string `json:"merch_name,omitempty" db:"-"`
	VariantSKU *string `json:"variant_sku,omitempty" db:"-"`
	EventTitle *string `json:"event_title,omitempty" db:"-"`
}