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
	CreatedAt          time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at" db:"updated_at"`

	// Joined display fields — populated by dashboard queries, never stored columns.
	MerchName  *string `json:"merch_name,omitempty" db:"-"`
	VariantSKU *string `json:"variant_sku,omitempty" db:"-"`
	EventTitle *string `json:"event_title,omitempty" db:"-"`
}