package services

import (
	"context"
	"errors"

	"events-db/internal/models"

	"github.com/rs/zerolog/log"
	"github.com/sony/gobreaker"
)

// PaymentGatewayName identifies which provider handled a given payment. It is
// persisted on the payments row (payment_gateway column) so refunds and webhook
// reconciliation route back to the correct backend.
type PaymentGatewayName string

const (
	GatewayPaystack PaymentGatewayName = "paystack"
	GatewayPayPal   PaymentGatewayName = "paypal"
)

// ErrPaymentServiceUnavailable is returned when a gateway's circuit breaker is
// open or the provider is failing, so callers can surface a clean 503 instead
// of leaking provider internals.
var ErrPaymentServiceUnavailable = errors.New("payment service temporarily unavailable")

// clientError wraps a 4xx response from a gateway. The circuit breaker's
// IsSuccessful callback treats these as successes so they don't count toward
// tripping the breaker — a bad request is a client bug, not an outage.
type clientError struct{ err error }

func (e *clientError) Error() string { return e.err.Error() }
func (e *clientError) Unwrap() error { return e.err }

// InitInput is the normalized request to start a hosted payment. The same shape
// is used for ticket bookings, carts, merch, listing fees and wallet top-ups —
// only the Reference prefix and amount differ.
type InitInput struct {
	Reference   string            // our globally-unique reference (also used to reconcile webhooks)
	AmountCents int64             // amount in the minor unit (cents for ZAR)
	Currency    string            // ISO 4217, e.g. "ZAR"
	Email       string            // payer email — Paystack requires it; PayPal uses it for receipts
	Description string            // human-readable line item
	CallbackURL string            // where the gateway returns the customer after completion
	Channels    []string          // Paystack channels, e.g. ["card"]. Apple Pay surfaces under "card".
	Metadata    map[string]string // free-form; echoed back where the provider supports it
}

// InitResult is the normalized response from starting a payment. The client
// opens RedirectURL (Paystack authorization_url / PayPal approve link) to pay.
type InitResult struct {
	Gateway     PaymentGatewayName
	Reference   string // provider-side identifier to reconcile against (Paystack reference / PayPal order id)
	RedirectURL string // hosted checkout / approval URL
	AccessCode  string // Paystack inline access_code (empty for PayPal)
}

// VerifyResult is the normalized authoritative status of a payment, used both
// by the synchronous verify-after-redirect path and by webhook handling.
type VerifyResult struct {
	Gateway       PaymentGatewayName
	Reference     string
	Status        models.PaymentStatus
	Method        models.PaymentMethod
	TransactionID string // provider transaction/capture id, for refunds
	AmountCents   int64
	Currency      string
}

// RefundResult is the normalized result of a refund request.
type RefundResult struct {
	RefundID string
	Status   models.PaymentStatus
}

// PaymentGateway is implemented by the Paystack and PayPal backends. The
// PaymentService routes by the customer's chosen method and persists results in
// a gateway-agnostic way, so booking/cart/merch logic never branches on provider.
type PaymentGateway interface {
	Name() PaymentGatewayName

	// Initialize starts a hosted payment and returns a redirect URL + reference.
	Initialize(ctx context.Context, in InitInput) (*InitResult, error)

	// Verify fetches the authoritative status of a payment by its provider reference.
	// For PayPal this also captures an APPROVED order so funds are actually taken.
	Verify(ctx context.Context, reference string) (*VerifyResult, error)

	// Refund refunds a settled payment. transactionID is the value stored from
	// Verify (Paystack transaction id / PayPal capture id).
	Refund(ctx context.Context, reference, transactionID string, amountCents int64, currency, reason string) (*RefundResult, error)
}

// logBreakerStateChange logs circuit-breaker transitions for every gateway.
func logBreakerStateChange(name string, from, to gobreaker.State) {
	log.Warn().
		Str("breaker", name).
		Str("from", from.String()).
		Str("to", to.String()).
		Msg("payment circuit breaker state changed")
}