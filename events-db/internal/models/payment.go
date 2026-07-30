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
	ID              string        `json:"id" db:"id"`
	BookingID       string        `json:"booking_id"`
	PaymentGateway  string        `json:"payment_gateway" db:"payment_gateway"`
	PaymentIntentID *string       `json:"payment_intent_id,omitempty" db:"payment_intent_id"`
	TransactionID   *string       `json:"transaction_id,omitempty" db:"transaction_id"`
	Amount          float64       `json:"amount" db:"amount"`
	Currency        string        `json:"currency" db:"currency"`
	PaymentMethod   PaymentMethod `json:"payment_method" db:"payment_method"`
	CardLast4       *string       `json:"card_last_4,omitempty" db:"card_last_4"`
	CardBrand       *string       `json:"card_brand,omitempty" db:"card_brand"`
	Status          PaymentStatus `json:"status" db:"status"`
	FailureReason   *string       `json:"failure_reason,omitempty" db:"failure_reason"`
	FailureCode     *string       `json:"failure_code,omitempty" db:"failure_code"`

	// ── Gateway-neutral fields (Paystack / PayPal) ────────────────────────────
	// GatewayReference is the provider-side identifier we reconcile webhooks
	// against: Paystack transaction reference, or PayPal order id.
	GatewayReference *string `json:"gateway_reference,omitempty" db:"gateway_reference"`
	// GatewayRedirectURL is the hosted checkout / approval URL the client opens.
	GatewayRedirectURL *string `json:"gateway_redirect_url,omitempty" db:"gateway_redirect_url"`

	// ── Legacy provider fields (kept until the drop migration) ────────────────
	StripeCustomerID       *string `json:"-" db:"stripe_customer"`
	StripePaymentMethodID  *string `json:"-" db:"stripe_payment_method_id"`
	StitchPaymentRequestID *string `json:"-" db:"stitch_payment_request_id"`
	StitchPaymentURL       *string `json:"-" db:"stitch_payment_url"`
	StitchLinkedAccountID  *string `json:"-" db:"stitch_linked_account_id"`

	// Refund tracking
	RefundID        *string    `json:"refund_id,omitempty" db:"refund_id"`
	RefundedAmount  *float64   `json:"refunded_amount,omitempty" db:"refunded_amount"`
	RefundedAt      *time.Time `json:"refunded_at,omitempty" db:"refunded_at"`
	WebhookReceived bool       `json:"webhook_received" db:"webhook_received"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty" db:"completed_at"`
}

// PaystackChannels maps a requested payment method to the Paystack channel list
// passed to /transaction/initialize.
//
// apple_pay is its OWN channel, not a variant of card. The previous comment
// here claimed Apple Pay surfaced under the card channel, and that was wrong:
// Paystack's channel enum lists card, bank, ussd, qr, eft, mobile_money,
// bank_transfer and apple_pay as siblings. Initialising with ["card"] alone
// produces a transaction that will not offer Apple Pay at checkout, however
// well configured the account and the frontend are — the channel list is a
// whitelist, so omitting apple_pay silently forbids it.
//
// Both card and apple_pay are requested together so a single transaction can
// serve either: the buyer picks at the Paystack sheet rather than committing
// on our checkout page. Apple Pay is card-funded and settles identically, so
// the subaccount split, transaction charge and reserve logic are unaffected.
func (pm PaymentMethod) PaystackChannels() []string {
	switch pm {
	case PaymentApplePay, PaymentCreditCard:
		return []string{"card", "apple_pay"}
	case PaymentInstantEFT:
		return []string{"eft"}
	default:
		return []string{"card", "apple_pay"}
	}
}