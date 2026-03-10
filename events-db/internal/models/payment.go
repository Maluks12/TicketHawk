package models

import "time"

type PaymentMethod string

const (
	PaymentCreditCard PaymentMethod = "credit_card"
	PaymentPayPal     PaymentMethod = "paypal"
	PaymentApplePay   PaymentMethod = "apple_pay"
	PaymentGooglePay  PaymentMethod = "google_pay"
	PaymentSamsungPay PaymentMethod = "samsung_pay"
	PaymentInstantEFT PaymentMethod = "instant_eft"
)

type Payment struct {
	ID                    string        `json:"id" db:"id"`
	BookingID             string        `json:"booking_id"`
	PaymentGateway        string        `json:"payment_gateway" db:"payment_gateway"`
	PaymentIntentID       *string       `json:"payment_intent_id,omitempty" db:"payment_intent_id"`
	TransactionID         *string       `json:"transaction_id,omitempty" db:"transaction_id"`
	Amount                float64       `json:"amount" db:"amount"`
	Currency              string        `json:"currency" db:"currency"`
	PaymentMethod         PaymentMethod `json:"payment_method" db:"payment_method"`
	CardLast4             *string       `json:"card_last_4,omitempty" db:"card_last_4"`
	CardBrand             *string       `json:"card_brand,omitempty" db:"card_brand"`
	Status                PaymentStatus `json:"status" db:"status"`
	FailureReason         *string       `json:"failure_reason,omitempty" db:"failure_reason"`
	FailureCode           *string       `json:"failure_code,omitempty" db:"failure_code"`
	// Stripe-specific (legacy, kept for backwards compatibility)
	StripeCustomerID      *string       `json:"-" db:"stripe_customer"`
	StripePaymentMethodID *string       `json:"-" db:"stripe_payment_method_id"`
	// Stitch-specific fields
	StitchPaymentRequestID *string       `json:"stitch_payment_request_id,omitempty" db:"stitch_payment_request_id"`
	StitchPaymentURL       *string       `json:"stitch_payment_url,omitempty" db:"stitch_payment_url"`
	StitchLinkedAccountID  *string       `json:"-" db:"stitch_linked_account_id"`
	// Refund tracking
	RefundID              *string       `json:"refund_id,omitempty" db:"refund_id"`
	RefundedAmount        *float64      `json:"refunded_amount,omitempty" db:"refunded_amount"`
	RefundedAt            *time.Time	`json:"refunded_at,omitempty" db:"refunded_at"`
	WebhookReceived  	  bool			`json:"webhook_received" db:"webhook_received"`
	CreatedAt			  time.Time		`json:"created_at" db:"created_at"`
	UpdatedAt  			  time.Time		`json:"updated_at" db:"updated_at"`
	CompletedAt			  *time.Time	`json:"completed_at,omitempty" db:"completed_at"`
}

func (pm PaymentMethod) ToStitchMethod() string {
	switch pm {
	case PaymentApplePay:
		return "applepay"
	case PaymentGooglePay:
		return "googlepay"
	case PaymentSamsungPay:
		return "samsungpay"
	case PaymentPayPal:
		return "paypal"
	case PaymentInstantEFT:
		return "instanteft"
	default:
		return ""
	}
}