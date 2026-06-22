package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"events-db/internal/models"

	"github.com/sony/gobreaker"
)

// PayPalGateway implements PaymentGateway against PayPal Orders v2. It is used
// for consumer ticket/merch payments only — never for organiser wallet top-ups.
// The order id is treated as our reconciliation reference; the capture id is the
// transaction id used for refunds.
type PayPalGateway struct {
	clientID     string
	clientSecret string
	webhookID    string
	baseURL      string

	httpClient *http.Client
	breaker    *gobreaker.CircuitBreaker

	tokenMu     sync.Mutex
	cachedToken *paypalToken
}

type paypalToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ExpiresAt   time.Time
}

func NewPayPalGateway(clientID, clientSecret, webhookID, baseURL string) *PayPalGateway {
	return &PayPalGateway{
		clientID:     clientID,
		clientSecret: clientSecret,
		webhookID:    webhookID,
		baseURL:      strings.TrimRight(baseURL, "/"),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		breaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        "paypal",
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

func (g *PayPalGateway) Name() PaymentGatewayName { return GatewayPayPal }

// appendCancelMarker tags the PayPal cancel_url so PaymentReturnHandler can
// distinguish an abandoned approval from a successful return.
func appendCancelMarker(rawURL string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&pp_cancel=1"
	}
	return rawURL + "?pp_cancel=1"
}

// PayPal cannot transact in ZAR (the app's pricing currency). For ZAR we present
// the PayPal order in USD, converted at PAYPAL_USD_ZAR_RATE (ZAR per 1 USD;
// default 18.5). The booking and our payment record stay in the original
// currency — only the PayPal order and its capture happen in USD. Any other
// currency is passed through unchanged.
func paypalPresentmentAmount(currency string, amountCents int64) (string, int64) {
	if !strings.EqualFold(currency, "ZAR") {
		return strings.ToUpper(currency), amountCents
	}
	rate := 18.5
	if v := strings.TrimSpace(os.Getenv("PAYPAL_USD_ZAR_RATE")); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 {
			rate = parsed
		}
	}
	usdCents := int64(math.Round(float64(amountCents) / rate))
	if usdCents < 1 {
		usdCents = 1
	}
	return "USD", usdCents
}

func (g *PayPalGateway) token(ctx context.Context) (string, error) {
	g.tokenMu.Lock()
	defer g.tokenMu.Unlock()

	if g.cachedToken != nil && time.Now().Before(g.cachedToken.ExpiresAt.Add(-60*time.Second)) {
		return g.cachedToken.AccessToken, nil
	}

	form := strings.NewReader("grant_type=client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/v1/oauth2/token", form)
	if err != nil {
		return "", fmt.Errorf("creating paypal token request: %w", err)
	}
	req.SetBasicAuth(g.clientID, g.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching paypal token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("paypal token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tok paypalToken
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("decoding paypal token: %w", err)
	}
	tok.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	g.cachedToken = &tok
	return tok.AccessToken, nil
}

// do performs an authenticated PayPal REST call through the breaker.
func (g *PayPalGateway) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	token, err := g.token(ctx)
	if err != nil {
		return fmt.Errorf("paypal auth: %w", err)
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshalling paypal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	result, err := g.breaker.Execute(func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, reqBody)
		if err != nil {
			return nil, fmt.Errorf("creating paypal request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("paypal request: %w", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("paypal %s returned %d: %s", path, resp.StatusCode, string(respBody))
		}
		if resp.StatusCode >= 400 {
			return nil, &clientError{err: fmt.Errorf("paypal %s returned %d: %s", path, resp.StatusCode, string(respBody))}
		}
		return respBody, nil
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
		if err := json.Unmarshal(result.([]byte), out); err != nil {
			return fmt.Errorf("decoding paypal data: %w", err)
		}
	}
	return nil
}

// ── Initialize (create order) ─────────────────────────────────────────────────

type paypalAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type paypalOrderRequest struct {
	Intent        string `json:"intent"`
	PurchaseUnits []struct {
		ReferenceID string       `json:"reference_id"`
		CustomID    string       `json:"custom_id"`
		Description string       `json:"description,omitempty"`
		Amount      paypalAmount `json:"amount"`
	} `json:"purchase_units"`
	ApplicationContext struct {
		ReturnURL  string `json:"return_url"`
		CancelURL  string `json:"cancel_url"`
		UserAction string `json:"user_action"`
	} `json:"application_context"`
}

type paypalOrder struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Links  []struct {
		Href string `json:"href"`
		Rel  string `json:"rel"`
	} `json:"links"`
	PurchaseUnits []struct {
		Amount   paypalAmount `json:"amount"`
		Payments struct {
			Captures []struct {
				ID     string       `json:"id"`
				Status string       `json:"status"`
				Amount paypalAmount `json:"amount"`
			} `json:"captures"`
		} `json:"payments"`
	} `json:"purchase_units"`
}

func centsToDecimal(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func (g *PayPalGateway) Initialize(ctx context.Context, in InitInput) (*InitResult, error) {
	var body paypalOrderRequest
	body.Intent = "CAPTURE"
	body.PurchaseUnits = make([]struct {
		ReferenceID string       `json:"reference_id"`
		CustomID    string       `json:"custom_id"`
		Description string       `json:"description,omitempty"`
		Amount      paypalAmount `json:"amount"`
	}, 1)
	body.PurchaseUnits[0].ReferenceID = in.Reference
	body.PurchaseUnits[0].CustomID = in.Reference
	body.PurchaseUnits[0].Description = in.Description
	curr, amountCents := paypalPresentmentAmount(in.Currency, in.AmountCents)
	body.PurchaseUnits[0].Amount = paypalAmount{
		CurrencyCode: curr,
		Value:        centsToDecimal(amountCents),
	}
	body.ApplicationContext.ReturnURL = in.CallbackURL
	// Distinct cancel URL so the return handler can tell an abandoned approval
	// from a completed one and fail it immediately — otherwise the app polls a
	// never-terminal CREATED order until timeout.
	body.ApplicationContext.CancelURL = appendCancelMarker(in.CallbackURL)
	body.ApplicationContext.UserAction = "PAY_NOW"

	var order paypalOrder
	if err := g.do(ctx, http.MethodPost, "/v2/checkout/orders", body, &order); err != nil {
		return nil, fmt.Errorf("paypal create order: %w", err)
	}
	if order.ID == "" {
		return nil, errors.New("paypal returned empty order id")
	}

	approveURL := ""
	for _, l := range order.Links {
		if strings.EqualFold(l.Rel, "approve") || strings.EqualFold(l.Rel, "payer-action") {
			approveURL = l.Href
			break
		}
	}
	if approveURL == "" {
		return nil, errors.New("paypal order has no approve link")
	}

	return &InitResult{
		Gateway:     GatewayPayPal,
		Reference:   order.ID,
		RedirectURL: approveURL,
	}, nil
}

// ── Verify (capture if approved) ──────────────────────────────────────────────

func (g *PayPalGateway) Verify(ctx context.Context, reference string) (*VerifyResult, error) {
	var order paypalOrder
	if err := g.do(ctx, http.MethodGet, "/v2/checkout/orders/"+reference, nil, &order); err != nil {
		return nil, fmt.Errorf("paypal get order: %w", err)
	}

	// APPROVED means the payer authorised it but funds aren't taken until capture.
	if strings.EqualFold(order.Status, "APPROVED") {
		var captured paypalOrder
		if err := g.do(ctx, http.MethodPost, "/v2/checkout/orders/"+reference+"/capture", struct{}{}, &captured); err != nil {
			return nil, fmt.Errorf("paypal capture: %w", err)
		}
		order = captured
	}

	res := &VerifyResult{
		Gateway:   GatewayPayPal,
		Reference: order.ID,
		Status:    paypalStatusToInternal(order.Status),
		Method:    models.PaymentPayPal,
		Currency:  in0Currency(order),
	}

	if len(order.PurchaseUnits) > 0 && len(order.PurchaseUnits[0].Payments.Captures) > 0 {
		cpt := order.PurchaseUnits[0].Payments.Captures[0]
		res.TransactionID = cpt.ID
		res.AmountCents = decimalToCents(cpt.Amount.Value)
		if res.Currency == "" {
			res.Currency = cpt.Amount.CurrencyCode
		}
		if strings.EqualFold(cpt.Status, "COMPLETED") {
			res.Status = models.PaymentCompleted
		}
	}
	return res, nil
}

func in0Currency(o paypalOrder) string {
	if len(o.PurchaseUnits) > 0 {
		return o.PurchaseUnits[0].Amount.CurrencyCode
	}
	return ""
}

// ── Refund ──────────────────────────────────────────────────────────────────

type paypalRefundRequest struct {
	Amount      *paypalAmount `json:"amount,omitempty"`
	NoteToPayer string        `json:"note_to_payer,omitempty"`
}

type paypalRefundResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (g *PayPalGateway) Refund(ctx context.Context, reference, transactionID string, amountCents int64, currency, reason string) (*RefundResult, error) {
	if transactionID == "" {
		return nil, errors.New("paypal refund requires a capture id")
	}

	req := paypalRefundRequest{NoteToPayer: reason}
	if amountCents > 0 {
		req.Amount = &paypalAmount{CurrencyCode: currency, Value: centsToDecimal(amountCents)}
	}

	var data paypalRefundResponse
	if err := g.do(ctx, http.MethodPost, "/v2/payments/captures/"+transactionID+"/refund", req, &data); err != nil {
		return nil, fmt.Errorf("paypal refund: %w", err)
	}

	status := models.PaymentRefunded
	if !strings.EqualFold(data.Status, "COMPLETED") && !strings.EqualFold(data.Status, "PENDING") {
		status = models.PaymentFailed
	}
	return &RefundResult{RefundID: data.ID, Status: status}, nil
}

// ── Webhook verification ──────────────────────────────────────────────────────

// PayPalWebhookHeaders carries the transmission headers PayPal sends with every
// webhook; they're required to verify authenticity.
type PayPalWebhookHeaders struct {
	AuthAlgo         string
	CertURL          string
	TransmissionID   string
	TransmissionSig  string
	TransmissionTime string
}

type paypalVerifyRequest struct {
	AuthAlgo         string          `json:"auth_algo"`
	CertURL          string          `json:"cert_url"`
	TransmissionID   string          `json:"transmission_id"`
	TransmissionSig  string          `json:"transmission_sig"`
	TransmissionTime string          `json:"transmission_time"`
	WebhookID        string          `json:"webhook_id"`
	WebhookEvent     json.RawMessage `json:"webhook_event"`
}

type paypalVerifyResponse struct {
	VerificationStatus string `json:"verification_status"`
}

// VerifyWebhook asks PayPal to confirm a webhook's signature. rawEvent is the
// exact JSON body received.
func (g *PayPalGateway) VerifyWebhook(ctx context.Context, h PayPalWebhookHeaders, rawEvent []byte) (bool, error) {
	var resp paypalVerifyResponse
	err := g.do(ctx, http.MethodPost, "/v1/notifications/verify-webhook-signature", paypalVerifyRequest{
		AuthAlgo:         h.AuthAlgo,
		CertURL:          h.CertURL,
		TransmissionID:   h.TransmissionID,
		TransmissionSig:  h.TransmissionSig,
		TransmissionTime: h.TransmissionTime,
		WebhookID:        g.webhookID,
		WebhookEvent:     json.RawMessage(rawEvent),
	}, &resp)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(resp.VerificationStatus, "SUCCESS"), nil
}

// ── Status mapping ────────────────────────────────────────────────────────────

func paypalStatusToInternal(s string) models.PaymentStatus {
	switch strings.ToUpper(s) {
	case "COMPLETED", "CAPTURED":
		return models.PaymentCompleted
	case "APPROVED", "PENDING", "SAVED", "PROCESSING":
		return models.PaymentProcessing
	case "VOIDED", "DECLINED", "FAILED":
		return models.PaymentFailed
	default:
		return models.PaymentPending
	}
}

// decimalToCents converts a PayPal decimal string ("123.45") to minor units.
func decimalToCents(v string) int64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	parts := strings.SplitN(v, ".", 2)
	var cents int64
	for _, ch := range parts[0] {
		if ch < '0' || ch > '9' {
			return 0
		}
		cents = cents*10 + int64(ch-'0')
	}
	cents *= 100
	if len(parts) == 2 {
		frac := parts[1]
		for len(frac) < 2 {
			frac += "0"
		}
		frac = frac[:2]
		cents += int64(frac[0]-'0')*10 + int64(frac[1]-'0')
	}
	return cents
}