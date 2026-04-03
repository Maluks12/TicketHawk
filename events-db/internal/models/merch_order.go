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
	ID                   string            `json:"id"`
	CartID               *string           `json:"cart_id,omitempty"`
	MerchandiseID        string            `json:"merchandise_id"`
	VariantID            string            `json:"variant_id"`
	EventID              string            `json:"event_id"`
	UserID               *string           `json:"user_id,omitempty"`
	SessionID            *string           `json:"session_id,omitempty"`
	Quantity             int               `json:"quantity"`
	UnitPrice            float64           `json:"unit_price"`
	TotalAmount          float64           `json:"total_amount"`
	StitchPaymentReqID   string            `json:"stitch_payment_req_id"`
	FulfillmentStatus    FulfillmentStatus `json:"fulfillment_status"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`

	// Joined display fields — populated by dashboard queries, not stored columns.
	MerchName    *string `json:"merch_name,omitempty"`
	VariantSKU   *string `json:"variant_sku,omitempty"`
	EventTitle   *string `json:"event_title,omitempty"`
}