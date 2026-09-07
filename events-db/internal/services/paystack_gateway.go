package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"events-db/internal/models"

	"github.com/sony/gobreaker"
)

const paystackBaseURL = "https://api.paystack.co"

// PaystackGateway implements PaymentGateway against Paystack's REST API.
// Auth is a single Bearer secret key (no OAuth/token refresh). Apple Pay is not
// a distinct server call — it is surfaced in the hosted checkout when enabled on
// the Paystack dashboard and settles through the card rail, so it arrives back
// as a normal card transaction whose authorization.channel we map to apple_pay.
type PaystackGateway struct {
	secretKey  string
	httpClient *http.Client
	breaker    *gobreaker.CircuitBreaker
}

func NewPaystackGateway(secretKey string) *PaystackGateway {
	return &PaystackGateway{
		secretKey:  secretKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		breaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        "paystack",
			MaxRequests: 2,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
			IsSuccessful: func(err error) bool {
				var ce *clientError
				return err == nil || errors.As(err, &ce)
			},
			ReadyToTrip:   func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= 5 },
			OnStateChange: logBreakerStateChange,
		}),
	}
}

func (g *PaystackGateway) Name() PaymentGatewayName { return GatewayPaystack }

// paystackEnvelope is the standard {status, message, data} wrapper Paystack
// returns on every endpoint.
type paystackEnvelope struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// isHTTPSuccess reports whether a status code is in the 2xx range. Paystack is
// not uniform about which success code it returns — creation endpoints answer
// 201, everything else 200 — so every check must span the range.
func isHTTPSuccess(code int) bool { return code >= 200 && code < 300 }

// do performs a Paystack REST call through the circuit breaker and unmarshals
// the data field into out. 4xx responses are wrapped as clientError so they do
// not trip the breaker; 5xx and network errors do.
func (g *PaystackGateway) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshalling paystack request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	result, err := g.breaker.Execute(func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, method, paystackBaseURL+path, reqBody)
		if err != nil {
			return nil, fmt.Errorf("creating paystack request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+g.secretKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("paystack request: %w", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("paystack %s returned %d: %s", path, resp.StatusCode, string(respBody))
		}

		var env paystackEnvelope
		if err := json.Unmarshal(respBody, &env); err != nil {
			if !isHTTPSuccess(resp.StatusCode) {
				return nil, &clientError{err: fmt.Errorf("paystack %s returned %d: %s", path, resp.StatusCode, string(respBody))}
			}
			return nil, fmt.Errorf("decoding paystack response: %w", err)
		}

		// Paystack returns 201 Created for POST /subaccount and
		// POST /transferrecipient, so success is the whole 2xx range, not just
		// 200. Comparing against http.StatusOK alone treated those two calls as
		// failures and surfaced Paystack's SUCCESS message ("Subaccount
		// created") as the error text, while the object really had been created
		// — orphaning one subaccount at Paystack per attempt and making payout
		// onboarding impossible. Only /transaction/initialize returns a plain
		// 200, which is why payments worked and this stayed latent.
		//
		// The status code is now included in the error so a future mismatch is
		// obvious from the log line instead of reading like a success.
		if !isHTTPSuccess(resp.StatusCode) || !env.Status {
			return nil, &clientError{err: fmt.Errorf("paystack %s returned %d: %s", path, resp.StatusCode, env.Message)}
		}
		return &env, nil
	})

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return ErrPaymentServiceUnavailable
		}
		var ce *clientError
		if errors.As(err, &ce) {
			return ce.err
		}
		return err
	}

	if out != nil {
		env := result.(*paystackEnvelope)
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decoding paystack data: %w", err)
		}
	}
	return nil
}

// ── Initialize ────────────────────────────────────────────────────────────────

type paystackInitRequest struct {
	Email       string            `json:"email"`
	Amount      int64             `json:"amount"` // minor unit (cents)
	Currency    string            `json:"currency"`
	Reference   string            `json:"reference"`
	CallbackURL string            `json:"callback_url,omitempty"`
	Channels    []string          `json:"channels,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`

	// Split-payment fields. Subaccount routes settlement to an organizer's
	// bank; TransactionCharge (minor units) is the flat platform cut that
	// overrides the subaccount's percentage_charge; Bearer picks who pays
	// Paystack's processing fee ("account" = platform main account).
	Subaccount        string `json:"subaccount,omitempty"`
	TransactionCharge int64  `json:"transaction_charge,omitempty"`
	Bearer            string `json:"bearer,omitempty"`
}

type paystackInitData struct {
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Reference        string `json:"reference"`
}

func (g *PaystackGateway) Initialize(ctx context.Context, in InitInput) (*InitResult, error) {
	channels := in.Channels
	if len(channels) == 0 {
		// Cards + Apple Pay both ride the card channel.
		channels = []string{"card"}
	}

	req := paystackInitRequest{
		Email:       in.Email,
		Amount:      in.AmountCents,
		Currency:    in.Currency,
		Reference:   in.Reference,
		CallbackURL: in.CallbackURL,
		Channels:    channels,
		Metadata:    in.Metadata,
	}
	// Attach the organizer split only when the caller resolved one. A zero
	// TransactionCharge with a subaccount would hand the platform cut to the
	// organizer, so both must be present together.
	if in.SubaccountCode != "" && in.TransactionChargeCents > 0 {
		req.Subaccount = in.SubaccountCode
		req.TransactionCharge = in.TransactionChargeCents
		req.Bearer = in.Bearer
	}

	var data paystackInitData
	err := g.do(ctx, http.MethodPost, "/transaction/initialize", req, &data)
	if err != nil {
		return nil, fmt.Errorf("paystack initialize: %w", err)
	}
	if data.AuthorizationURL == "" {
		return nil, errors.New("paystack returned empty authorization_url")
	}

	return &InitResult{
		Gateway:     GatewayPaystack,
		Reference:   data.Reference,
		RedirectURL: data.AuthorizationURL,
		AccessCode:  data.AccessCode,
	}, nil
}

// ── Verify ──────────────────────────────────────────────────────────────────

type paystackVerifyData struct {
	Status        string `json:"status"` // success | failed | abandoned | ongoing | pending
	Reference     string `json:"reference"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	ID            int64  `json:"id"`
	Authorization struct {
		Channel  string `json:"channel"`   // card | apple_pay | ...
		Brand    string `json:"brand"`     // visa | mastercard | ...
		CardType string `json:"card_type"` // debit | credit

		// Reusable authorization. Paystack returns these on every card
		// transaction; whether the code may actually be re-charged is what
		// Reusable says, and it is false for a good number of them.
		AuthorizationCode string `json:"authorization_code"`
		Last4             string `json:"last4"`
		Reusable          bool   `json:"reusable"`
	} `json:"authorization"`
}

func (g *PaystackGateway) Verify(ctx context.Context, reference string) (*VerifyResult, error) {
	var data paystackVerifyData
	if err := g.do(ctx, http.MethodGet, "/transaction/verify/"+reference, nil, &data); err != nil {
		return nil, fmt.Errorf("paystack verify: %w", err)
	}

	res := &VerifyResult{
		Gateway:       GatewayPaystack,
		Reference:     data.Reference,
		Status:        paystackStatusToInternal(data.Status),
		Method:        paystackChannelToMethod(data.Authorization.Channel),
		TransactionID: fmt.Sprintf("%d", data.ID),
		AmountCents:   data.Amount,
		Currency:      data.Currency,
		CardBrand:     data.Authorization.Brand,
		CardLast4:     data.Authorization.Last4,
		Reusable:      data.Authorization.Reusable,
	}

	// Only carry the code when it is genuinely re-chargeable. Paystack returns
	// one for single-use authorizations as well, and storing those would
	// produce mandates that look valid and fail at the moment they are needed —
	// which for shortfall recovery is weeks later, with an attendee already
	// gone.
	if data.Authorization.Reusable {
		res.AuthorizationCode = data.Authorization.AuthorizationCode
	}
	return res, nil
}

// ── Refund ──────────────────────────────────────────────────────────────────

type paystackRefundRequest struct {
	Transaction  string `json:"transaction"` // reference or transaction id
	Amount       int64  `json:"amount,omitempty"`
	Currency     string `json:"currency,omitempty"`
	MerchantNote string `json:"merchant_note,omitempty"`
}

type paystackRefundData struct {
	ID     int64  `json:"id"`
	Status string `json:"status"` // pending | processed | processing | failed
}

func (g *PaystackGateway) Refund(ctx context.Context, reference, transactionID string, amountCents int64, currency, reason string) (*RefundResult, error) {
	txn := transactionID
	if txn == "" {
		txn = reference
	}

	var data paystackRefundData
	err := g.do(ctx, http.MethodPost, "/refund", paystackRefundRequest{
		Transaction:  txn,
		Amount:       amountCents,
		Currency:     currency,
		MerchantNote: reason,
	}, &data)
	if err != nil {
		return nil, fmt.Errorf("paystack refund: %w", err)
	}

	status := models.PaymentRefunded
	if strings.EqualFold(data.Status, "failed") {
		status = models.PaymentFailed
	}
	return &RefundResult{
		RefundID: fmt.Sprintf("%d", data.ID),
		Status:   status,
	}, nil
}

// ── Webhook signature ─────────────────────────────────────────────────────────

// VerifySignature validates a Paystack webhook by recomputing the HMAC-SHA512
// of the raw body with the secret key and comparing it (constant-time) to the
// x-paystack-signature header value.
func (g *PaystackGateway) VerifySignature(rawBody []byte, signature string) bool {
	mac := hmac.New(sha512.New, []byte(g.secretKey))
	mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature)))
}

// ── Status mapping ────────────────────────────────────────────────────────────

func paystackStatusToInternal(s string) models.PaymentStatus {
	switch strings.ToLower(s) {
	case "success":
		return models.PaymentCompleted
	case "ongoing", "pending", "processing":
		return models.PaymentProcessing
	case "failed", "abandoned", "reversed":
		return models.PaymentFailed
	default:
		return models.PaymentPending
	}
}

func paystackChannelToMethod(channel string) models.PaymentMethod {
	switch strings.ToLower(channel) {
	case "apple_pay", "applepay":
		return models.PaymentApplePay
	case "card":
		return models.PaymentCreditCard
	case "eft":
		return models.PaymentInstantEFT
	default:
		return models.PaymentCreditCard
	}
}