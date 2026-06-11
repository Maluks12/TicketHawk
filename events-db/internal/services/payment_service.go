package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"github.com/sony/gobreaker"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const (
	stitchTokenURL      = "https://secure.stitch.money/connect/token"
	stitchGraphQLURL    = "https://api.stitch.money/graphql"
	stitchTokenAudience = "https://secure.stitch.money/connect/token"
	stitchGateway       = "stitch"
) //change to var when testing

type stitchToken struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int       `json:"expires_in"`
	ExpiresAt   time.Time `json:"-"`
}

type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphQLError  `json:"errors"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type secureFieldsPaymentResponse struct {
	ClientCardPaymentInitiate struct {
		PaymentInitiation struct {
			ID          string `json:"id"`
			ClientToken string `json:"clientToken"`
		} `json:"paymentInitiation"`
	} `json:"clientCardPaymentInitiate"`
}

type refundPaymentResponse struct {
	ClientRefundInitiate struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"clientRefundInitiate"`
}

var ErrPaymentServiceUnavailable = errors.New("payment service temporarily unavailable")

// clientError wraps a 4xx response from Stitch. The circuit breaker's
// IsSuccessful callback treats these as successes so they don't count toward
// tripping the breaker — a bad request is a client bug, not an outage.
type clientError struct{ err error }

func (e *clientError) Error() string { return e.err.Error() }
func (e *clientError) Unwrap() error { return e.err }

type PaymentService struct {
	pool         *pgxpool.Pool
	cfg          *config.Config
	httpClient   *http.Client
	tokenMu      sync.Mutex
	cachedToken  *stitchToken
	jobChan      chan<- string
	breaker      *gobreaker.CircuitBreaker // guards GraphQL calls
	tokenBreaker *gobreaker.CircuitBreaker // guards OAuth token endpoint
}

func NewPaymentService(pool *pgxpool.Pool, cfg *config.Config, jobChan chan<- string) *PaymentService {
	// isSuccessful returns true for clientErrors (4xx responses) so they don't
	// count as failures toward tripping the breaker. A bad request is a client
	// bug; a 5xx or network error is an infrastructure problem.
	isSuccessful := func(err error) bool {
		var ce *clientError
		return err == nil || errors.As(err, &ce)
	}

	// GraphQL breaker — trips after 5 consecutive infrastructure failures.
	// Waits 30 s before allowing a single probe request (half-open).
	// Closes again once 2 consecutive probes succeed.
	graphQLBreaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:          "stitch-graphql",
		MaxRequests:   2,
		Interval:      60 * time.Second,
		Timeout:       30 * time.Second,
		IsSuccessful:  isSuccessful,
		ReadyToTrip:   func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= 5 },
		OnStateChange: logBreakerStateChange,
	})

	// Token breaker — separate so a token-endpoint outage doesn't conflate
	// with GraphQL failures, and vice versa. Slightly more lenient (3 failures)
	// because token fetches are less frequent than GraphQL calls.
	tokenBreaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:          "stitch-token",
		MaxRequests:   1,
		Interval:      60 * time.Second,
		Timeout:       30 * time.Second,
		ReadyToTrip:   func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= 3 },
		OnStateChange: logBreakerStateChange,
	})

	return &PaymentService{
		pool: pool,
		cfg:  cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		jobChan:      jobChan,
		breaker:      graphQLBreaker,
		tokenBreaker: tokenBreaker,
	}
}

func logBreakerStateChange(name string, from, to gobreaker.State) {
	log.Warn().
		Str("breaker", name).
		Str("from", from.String()).
		Str("to", to.String()).
		Msg("payment circuit breaker state changed")
}

func (s *PaymentService) getAccessToken(ctx context.Context) (string, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()

	if s.cachedToken != nil && time.Now().Before(s.cachedToken.ExpiresAt.Add(-60*time.Second)) {
		return s.cachedToken.AccessToken, nil
	}

	formData := url.Values{}
	formData.Set("grant_type", "client_credentials")
	formData.Set("client_id", s.cfg.StitchClientID)
	formData.Set("client_secret", s.cfg.StitchClientSecret)
	formData.Set("audience", stitchTokenAudience)
	formData.Set("scope", "client_paymentrequest")

	result, err := s.tokenBreaker.Execute(func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, stitchTokenURL, strings.NewReader(formData.Encode()))
		if err != nil {
			return nil, fmt.Errorf("creating token request: %w", err)
		}
		req.Header.Set("Content-type", "application/x-www-form-urlencoded")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching stitch token: %w", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("stitch token endpoint returned %d: %s", resp.StatusCode, string(body))
		}

		var tok stitchToken
		if err := json.Unmarshal(body, &tok); err != nil {
			return nil, fmt.Errorf("decoding token response: %w", err)
		}
		return &tok, nil
	})

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return "", ErrPaymentServiceUnavailable
		}
		return "", err
	}

	tok := result.(*stitchToken)
	tok.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	s.cachedToken = tok
	return tok.AccessToken, nil
}

func (s *PaymentService) graphQL(ctx context.Context, query string, variables map[string]interface{}, out interface{}) error {
	token, err := s.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
 
	payload := graphQLRequest{Query: query, Variables: variables}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling graphql request: %w", err)
	}
 
	result, err := s.breaker.Execute(func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, stitchGraphQLURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("creating graphql request: %w", err)
		}
 
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
 
		resp, err := s.httpClient.Do(req)
		if err != nil {
			// Network error — counts as a failure toward tripping the breaker.
			return nil, fmt.Errorf("graphql request: %w", err)
		}
		defer resp.Body.Close()
 
		respBody, _ := io.ReadAll(resp.Body)
 
		// 5xx responses are transient Stitch failures — count toward tripping.
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("stitch graphql returned %d: %s", resp.StatusCode, string(respBody))
		}

		// 4xx responses are client errors (bad input, auth) — wrap in
		// clientError so IsSuccessful treats them as non-failures and the
		// breaker doesn't count them. Surface the real error to the caller.
		if resp.StatusCode != http.StatusOK {
			return nil, &clientError{
				err: fmt.Errorf("stitch graphql returned %d: %s", resp.StatusCode, string(respBody)),
			}
		}
 
		var gqlResp graphQLResponse
		if err := json.Unmarshal(respBody, &gqlResp); err != nil {
			return nil, fmt.Errorf("decoding graphql response: %w", err)
		}
 
		if len(gqlResp.Errors) > 0 {
			msgs := make([]string, len(gqlResp.Errors))
			for i, e := range gqlResp.Errors {
				msgs[i] = e.Message
			}
			// GraphQL application errors are not infrastructure failures.
			return &gqlResp, &clientError{
				err: fmt.Errorf("graphql errors: %s", strings.Join(msgs, "; ")),
			}
		}
 
		return &gqlResp, nil
	})
 
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return ErrPaymentServiceUnavailable
		}
		// Unwrap clientError so the caller sees the underlying error, not the wrapper.
		var ce *clientError
		if errors.As(err, &ce) {
			return ce.err
		}
		return err
	}
 
	if out != nil {
		gqlResp := result.(*graphQLResponse)
		if err := json.Unmarshal(gqlResp.Data, out); err != nil {
			return fmt.Errorf("decoding graphql data: %w", err)
		}
	}
 
	return nil
}

func (s *PaymentService) CreatePaymentIntent(booking *models.Booking) (clientToken string, paymentRequestID string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	amountCents := int64(booking.TotalAmount * 100)

	mutation := `
		mutation InitiateCardPayment(
      		$input: ClientCardPaymentInitiateInput!
    		) {
      		clientCardPaymentInitiate(input: $input) {
        	paymentInitiation {
			id
			clientToken
        }
      }
    }
	`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"amount": map[string]interface{}{
				"quantity": amountCents,
				"currency": booking.Currency,
			},
			"payerReference":       booking.BookingNumber,
			"merchantReference":    booking.BookingNumber,
			"beneficiaryReference": booking.BookingNumber,
			"externalReference":    booking.ID,
		},
	}

	var result secureFieldsPaymentResponse
	if err := s.graphQL(ctx, mutation, variables, &result); err != nil {
		return "", "", fmt.Errorf("creating stitch secure fields session: %w", err)
	}

	pi := result.ClientCardPaymentInitiate.PaymentInitiation
	if pi.ID == "" || pi.ClientToken == "" {
		return "", "", errors.New("stitch returned empty payment ID or client token")
	}

	if err := s.createPaymentRecord(ctx, booking, pi.ID, nil, nil); err != nil {
		log.Warn().Err(err).Str("booking_id", booking.ID).Msg("payment: failed to persist payment record")
	}

	return pi.ClientToken, pi.ID, nil
}

func (s *PaymentService) createPaymentRecord(ctx context.Context, booking *models.Booking, requestID string, paymentURL *string, cartID *string) error {
	query := `
		INSERT INTO payments (
        booking_id, payment_gateway, payment_intent_id,
        amount, currency, payment_method, status,
        stitch_payment_request_id, stitch_payment_url,
        cart_id
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    ON CONFLICT (booking_id)
    DO UPDATE SET
        stitch_payment_request_id = EXCLUDED.stitch_payment_request_id,
        stitch_payment_url        = EXCLUDED.stitch_payment_url,
        cart_id                   = EXCLUDED.cart_id,
        webhook_received          = FALSE,
        status                    = 'pending',
        updated_at                = CURRENT_TIMESTAMP
	`
	_, err := s.pool.Exec(ctx, query,
		booking.ID,
		stitchGateway,
		requestID,
		booking.TotalAmount,
		booking.Currency,
		string(models.PaymentCreditCard),
		string(models.PaymentPending),
		requestID,
		paymentURL,
		cartID,
	)
	return err
}

func (s *PaymentService) CreateCartPaymentIntent(primaryBooking *models.Booking, cartID string, totalAmount float64) (clientToken string, paymentRequestID string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	amountCents := int64(totalAmount * 100)

	mutation := `
		mutation InitiateCardPayment($input: ClientCardPaymentInitiateInput!) {
			clientCardPaymentInitiate(input: $input) {
				paymentInitiation {
					id
					clientToken
				}
			}
		}
	`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"amount": map[string]interface{}{
				"quantity": amountCents,
				"currency": primaryBooking.Currency,
			},
			"payerReference":       primaryBooking.BookingNumber,
			"merchantReference":    primaryBooking.BookingNumber,
			"beneficiaryReference": primaryBooking.BookingNumber,
			"externalReference":    cartID,
		},
	}

	var result secureFieldsPaymentResponse
	if err := s.graphQL(ctx, mutation, variables, &result); err != nil {
		return "", "", fmt.Errorf("creating cart payment intent: %w", err)
	}

	pi := result.ClientCardPaymentInitiate.PaymentInitiation
	if pi.ID == "" || pi.ClientToken == "" {
		return "", "", errors.New("stitch returned empty payment ID or client token")
	}

	if err := s.createPaymentRecord(ctx, primaryBooking, pi.ID, nil, &cartID); err != nil {
		log.Warn().Err(err).Str("cart_id", cartID).Msg("payment: failed to persist cart payment record")
	}

	return pi.ClientToken, pi.ID, nil
}

// CreateAdTopupIntent creates a Stitch payment intent for an ad wallet top-up.
// The externalReference is prefixed with "topup_" so the /webhooks/stitch-ads
// handler can distinguish it from booking payments without touching the
// existing /webhooks/stitch handler.
func (s *PaymentService) CreateAdTopupIntent(organizerID string, amount float64) (clientToken string, requestID string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	amountCents := int64(amount * 100)
	externalRef := "topup_" + organizerID

	mutation := `
		mutation InitiateCardPayment($input: ClientCardPaymentInitiateInput!) {
			clientCardPaymentInitiate(input: $input) {
				paymentInitiation {
					id
					clientToken
				}
			}
		}
	`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"amount": map[string]interface{}{
				"quantity": amountCents,
				"currency": models.DefaultCurrency,
			},
			"payerReference":       "Ad Wallet Top-up",
			"merchantReference":    externalRef,
			"beneficiaryReference": externalRef,
			"externalReference":    externalRef,
		},
	}

	var result secureFieldsPaymentResponse
	if err := s.graphQL(ctx, mutation, variables, &result); err != nil {
		return "", "", fmt.Errorf("CreateAdTopupIntent: %w", err)
	}

	pi := result.ClientCardPaymentInitiate.PaymentInitiation
	if pi.ID == "" || pi.ClientToken == "" {
		return "", "", errors.New("stitch returned empty payment ID for ad topup")
	}
	return pi.ClientToken, pi.ID, nil
}

// CreateListingFeeIntent creates a Stitch card payment intent for the platform
// listing fee. The externalReference embeds the eventID so the webhook can
// resolve it even if the listing_fee_payment_request_id lookup fails.
func (s *PaymentService) CreateListingFeeIntent(eventID, organizerID string, amount float64) (clientToken string, requestID string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	amountCents := int64(amount * 100)
	externalRef := "listing_fee_" + eventID

	mutation := `
		mutation InitiateCardPayment($input: ClientCardPaymentInitiateInput!) {
			clientCardPaymentInitiate(input: $input) {
				paymentInitiation {
					id
					clientToken
				}
			}
		}
	`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"amount": map[string]interface{}{
				"quantity": amountCents,
				"currency": models.DefaultCurrency,
			},
			"payerReference":       "Platform Listing Fee",
			"merchantReference":    externalRef,
			"beneficiaryReference": externalRef,
			"externalReference":    externalRef,
		},
	}

	var result secureFieldsPaymentResponse
	if err := s.graphQL(ctx, mutation, variables, &result); err != nil {
		return "", "", fmt.Errorf("CreateListingFeeIntent: %w", err)
	}

	pi := result.ClientCardPaymentInitiate.PaymentInitiation
	if pi.ID == "" || pi.ClientToken == "" {
		return "", "", errors.New("stitch returned empty payment ID for listing fee")
	}
	return pi.ClientToken, pi.ID, nil
}

func (s *PaymentService) ProcessRefund(bookingID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	payment, err := s.GetPaymentByBookingID(ctx, bookingID)
	if err != nil {
		return fmt.Errorf("fetching payment for booking %s: %w", bookingID, err)
	}

	if payment.Status != models.PaymentCompleted {
		return fmt.Errorf("payment for booking %s is not completed (status: %s)", bookingID, payment.Status)
	}

	if payment.StitchPaymentRequestID == nil {
		return fmt.Errorf("no stitch payment request ID for booking %s", bookingID)
	}

	mutation := `
		mutation RefundPayment($input: ClientRefundInitiateInput!) {
			clientRefundInitiate(input: $input) {
				id
				status
			}
		}
	`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"paymentRequestId": *payment.StitchPaymentRequestID,
			"reason":           "Customer requested refund",
		},
	}

	var result refundPaymentResponse
	if err := s.graphQL(ctx, mutation, variables, &result); err != nil {
		return fmt.Errorf("stitch refund: %w", err)
	}

	refundedID := result.ClientRefundInitiate.ID
	query := `
		UPDATE payments
		SET refund_id       = $1,
		    status          = $2,
		    refunded_amount = amount,
		    refunded_at     = CURRENT_TIMESTAMP,
		    updated_at      = CURRENT_TIMESTAMP
		WHERE booking_id = $3
	`

	if _, err := s.pool.Exec(ctx, query, refundedID, string(models.PaymentRefunded), bookingID); err != nil {
		log.Warn().Err(err).Str("booking_id", bookingID).Msg("payment: stitch refund OK but DB update failed")
	}
	return nil
}

type StitchWebhookEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type stitchPaymentUpdatePayload struct {
	PaymentInitiation struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Payer  struct {
			PaymentMethod string `json:"paymentMethod"`
			Account       *struct {
				ID string `json:"id"`
			} `json:"account"`
		} `json:"payer"`
		Transactions []struct {
			ID string `json:"id"`
		} `json:"transactions"`
	} `json:"paymentInitiation"`
}

func (s *PaymentService) HandleWebhook(event *StitchWebhookEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch event.Type {
	case "payment_initiation_request.updated":
		return s.handlePaymentUpdate(ctx, event.Data)
	case "card_payment.updated":
		return s.handlePaymentUpdate(ctx, event.Data)
	default:
		log.Warn().Str("type", event.Type).Msg("stitch webhook: unhandled event type")
		return nil
	}
}

func (s *PaymentService) handlePaymentUpdate(ctx context.Context, data json.RawMessage) error {
	var payload stitchPaymentUpdatePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("decoding payment update payload: %w", err)
	}

	pr := payload.PaymentInitiation
	if pr.ID == "" {
		return errors.New("webhook missing payment request ID")
	}

	internalStatus := stitchStatusToInternal(pr.Status)
	paymentMethod := stitchMethodToInternal(pr.Payer.PaymentMethod)

	var transactionID *string
	if len(pr.Transactions) > 0 {
		txID := pr.Transactions[0].ID
		transactionID = &txID
	}

	// ── Listing fee shortcut ──────────────────────────────────────────────
	// Check if this request ID belongs to a listing fee payment. If it does,
	// mark the event paid and return — it is not a booking payment.
	if internalStatus == models.PaymentCompleted {
		eventID, matched, lfErr := repository.ConfirmListingFeeByPaymentRequest(s.pool, pr.ID)
		if lfErr != nil {
			log.Warn().Err(lfErr).Str("stitch_id", pr.ID).Msg("webhook: listing fee confirmation failed")
		} else if matched {
			log.Info().Str("stitch_id", pr.ID).Str("event_id", eventID).
				Msg("stitch webhook: listing fee confirmed, event will now surface in home feed")
			return nil
		}
	}
	// ─────────────────────────────────────────────────────────────────────

	var currentStatus string
	var bookingID string
	var cartID *string

	queryStatus := `
		SELECT status, booking_id, cart_id FROM payments
		WHERE stitch_payment_request_id = $1
	`
	err := s.pool.QueryRow(ctx, queryStatus, pr.ID).Scan(&currentStatus, &bookingID, &cartID)
	if err != nil {
		log.Warn().Str("stitch_id", pr.ID).Msg("webhook: no payment record found, ignoring")
		return nil
	}

	if currentStatus == string(models.PaymentCompleted) || currentStatus == string(models.PaymentRefunded) {
		log.Info().Str("stitch_id", pr.ID).Str("status", currentStatus).Msg("webhook: duplicate delivery, ignoring")
		return nil
	}

	query := `
		UPDATE payments
			SET status       = $1::text,
			    payment_method   = $2::payment_method_type,
			    transaction_id   = COALESCE($3, transaction_id),
			    webhook_received = TRUE,
			    completed_at     = CASE WHEN $1::text = 'completed' THEN CURRENT_TIMESTAMP ELSE completed_at END,
			    updated_at       = CURRENT_TIMESTAMP
		WHERE stitch_payment_request_id = $4
		RETURNING booking_id
	`

	err = s.pool.QueryRow(ctx, query,
		string(internalStatus),
		string(paymentMethod),
		transactionID,
		pr.ID,
	).Scan(&bookingID)
	if err != nil {
		return fmt.Errorf("updating payment for stitch request %s: %w", pr.ID, err)
	}

	if err := s.syncBookingStatus(ctx, bookingID, internalStatus); err != nil {
		log.Warn().Err(err).Str("booking_id", bookingID).Msg("webhook: payment updated but booking sync failed")
	}

	log.Info().Str("stitch_id", pr.ID).Str("status", string(internalStatus)).
		Str("method", string(paymentMethod)).Str("booking_id", bookingID).
		Msg("stitch webhook: payment updated")

	if cartID != nil {
		additionalIDs, syncErr := s.syncAllCartBookings(ctx, *cartID, bookingID, internalStatus)
		if syncErr != nil {
			log.Warn().Err(syncErr).Str("cart_id", *cartID).Msg("webhook: failed to sync cart bookings")
		}

		if internalStatus == models.PaymentCompleted {
			if err := repository.FulfillMerchOrders(ctx, s.pool, *cartID, pr.ID); err != nil {
				log.Error().Err(err).
					Str("cart_id", *cartID).
					Str("stitch_id", pr.ID).
					Msg("webhook: merch fulfillment failed — manual reconciliation required")
			}
		}

		if internalStatus == models.PaymentCompleted {
			allBookingIDs := append([]string{bookingID}, additionalIDs...)
			for _, id := range allBookingIDs {
				select {
				case s.jobChan <- id:
					log.Info().Str("booking_id", id).Msg("stitch webhook: enqueued booking job")
				default:
					log.Warn().Str("booking_id", id).Msg("webhook: booking job channel full, job dropped")
				}
			}
		}
	} else if internalStatus == models.PaymentCompleted {
		select {
		case s.jobChan <- bookingID:
			log.Info().Str("booking_id", bookingID).Msg("stitch webhook: enqueued booking job")
		default:
			log.Warn().Str("booking_id", bookingID).Msg("webhook: booking job channel full, job dropped")
		}
	}

	return nil
}

func (s *PaymentService) syncAllCartBookings(ctx context.Context, cartID, primaryBookingID string, status models.PaymentStatus) ([]string, error) {
	var bookingStatus models.BookingStatus
	switch status {
	case models.PaymentCompleted:
		bookingStatus = models.BookingConfirmed
	case models.PaymentFailed:
		bookingStatus = models.BookingCancelled
	default:
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
		UPDATE bookings
		SET booking_status = $1::text,
		    payment_status = $2::text,
		    confirmed_at   = CASE WHEN $1::text = 'confirmed' THEN CURRENT_TIMESTAMP ELSE confirmed_at END,
		    updated_at     = CURRENT_TIMESTAMP
		WHERE cart_id = $3
		  AND id      != $4
		  AND booking_status = 'pending'
		RETURNING id
	`, string(bookingStatus), string(status), cartID, primaryBookingID)
	if err != nil {
		return nil, fmt.Errorf("syncing cart bookings: %w", err)
	}
	defer rows.Close()

	var additionalIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		additionalIDs = append(additionalIDs, id)
		log.Info().Str("booking_id", id).Str("cart_id", cartID).Msg("stitch webhook: confirmed additional cart booking")
	}
	return additionalIDs, nil
}

func (s *PaymentService) syncBookingStatus(ctx context.Context, bookingID string, status models.PaymentStatus) error {
	_, err := s.pool.Exec(ctx, `
        UPDATE bookings
        SET payment_status = $1,
            updated_at     = CURRENT_TIMESTAMP
        WHERE id = $2
    `, string(status), bookingID)
	if err != nil {
		return err
	}

	var bookingStatus models.BookingStatus
	switch status {
	case models.PaymentCompleted:
		bookingStatus = models.BookingConfirmed
	case models.PaymentFailed:
		bookingStatus = models.BookingCancelled
	default:
		return nil
	}

	_, err = s.pool.Exec(ctx, `
        UPDATE bookings
        SET booking_status = $1::text,
            confirmed_at   = CASE WHEN $1::text = 'confirmed' THEN CURRENT_TIMESTAMP ELSE confirmed_at END,
            updated_at     = CURRENT_TIMESTAMP
        WHERE id = $2
    `, string(bookingStatus), bookingID)
	return err
}

func (s *PaymentService) GetPaymentStatus(bookingID string) (*models.Payment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.GetPaymentByBookingID(ctx, bookingID)
}

func (s *PaymentService) GetPaymentByBookingID(ctx context.Context, bookingID string) (*models.Payment, error) {
	query := `
		SELECT id, booking_id, payment_gateway, payment_intent_id, transaction_id,
		       amount, currency, payment_method, status,
		       stitch_payment_request_id, stitch_payment_url,
		       refund_id, refunded_amount, refunded_at,
		       webhook_received, created_at, updated_at, completed_at
		FROM payments
		WHERE booking_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var p models.Payment
	err := s.pool.QueryRow(ctx, query, bookingID).Scan(
		&p.ID, &p.BookingID, &p.PaymentGateway, &p.PaymentIntentID, &p.TransactionID,
		&p.Amount, &p.Currency, &p.PaymentMethod, &p.Status,
		&p.StitchPaymentRequestID, &p.StitchPaymentURL,
		&p.RefundID, &p.RefundedAmount, &p.RefundedAt,
		&p.WebhookReceived, &p.CreatedAt, &p.UpdatedAt, &p.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("payment not found for booking %s: %w", bookingID, err)
	}
	return &p, nil
}

func stitchStatusToInternal(s string) models.PaymentStatus {
	switch strings.ToLower(s) {
	case "completed", "succeeded":
		return models.PaymentCompleted
	case "processing":
		return models.PaymentProcessing
	case "failed", "error", "cancelled", "canceled":
		return models.PaymentFailed
	default:
		return models.PaymentPending
	}
}

func stitchMethodToInternal(s string) models.PaymentMethod {
	switch strings.ToLower(s) {
	case "applepay", "apple_pay":
		return models.PaymentApplePay
	case "googlepay", "google_pay":
		return models.PaymentGooglePay
	case "samsungpay", "samsung_pay":
		return models.PaymentSamsungPay
	case "paypal":
		return models.PaymentPayPal
	case "instanteft", "instant_eft", "eft":
		return models.PaymentInstantEFT
	default:
		return models.PaymentCreditCard
	}
}

// EnqueueJob sends a confirmed booking ID to the booking worker pipeline.
// Call this for free bookings that bypass the Stitch payment flow so the
// worker still creates ticket assignments, generates QR codes, and sends
// the confirmation email.
func (s *PaymentService) EnqueueJob(bookingID string) {
	select {
	case s.jobChan <- bookingID:
	default:
		log.Warn().Str("booking_id", bookingID).Msg("payment_service: job channel full, dropping booking job")
	}
}
// InitiateDebitMandate initiates a Stitch debit mandate payment to bill an
// organiser for their end-of-day ad spend.
//
// This replaces the old CreateAdTopupIntent flow. Instead of the organiser
// manually topping up a wallet, the platform automatically debits them each
// morning for the previous day's actual spend via their stored mandate.
//
// The requestID returned should be stored in ad_billing_records.stitch_payment_request_id
// so the /webhooks/stitch-ads handler can match the incoming confirmation.
func (s *PaymentService) InitiateDebitMandate(mandateID string, amount float64, reference string, email string) (requestID string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	amountCents := int64(amount * 100)

	mutation := `
		mutation InitiateDebitOrder($input: DebitOrderInitiateInput!) {
			debitOrderInitiate(input: $input) {
				debitOrder {
					id
					status
				}
			}
		}
	`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"mandate": map[string]interface{}{
				"id": mandateID,
			},
			"amount": map[string]interface{}{
				"quantity": amountCents,
				"currency": models.DefaultCurrency,
			},
			"payerReference":    "EventsPlatform Ad Billing",
			"merchantReference": reference,
			"externalReference": reference,
			"beneficiary": map[string]interface{}{
				"bankAccount": map[string]interface{}{
					"name": "Events Platform Ad Revenue",
				},
			},
		},
	}

	var result struct {
		DebitOrderInitiate struct {
			DebitOrder struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"debitOrder"`
		} `json:"debitOrderInitiate"`
	}

	if err := s.graphQL(ctx, mutation, variables, &result); err != nil {
		return "", fmt.Errorf("InitiateDebitMandate: %w", err)
	}

	id := result.DebitOrderInitiate.DebitOrder.ID
	if id == "" {
		return "", errors.New("stitch returned empty debit order ID")
	}

	return id, nil
}