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

type PaymentService struct {
	pool        *pgxpool.Pool
	cfg         *config.Config
	httpClient  *http.Client
	tokenMu     sync.Mutex
	cachedToken *stitchToken
	jobChan     chan<- string
}

func NewPaymentService(pool *pgxpool.Pool, cfg *config.Config, jobChan chan<- string) *PaymentService {
	return &PaymentService{
		pool: pool,
		cfg:  cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		jobChan: jobChan,
	}
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stitchTokenURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching stitch token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("stitch token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tok stitchToken
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	tok.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	s.cachedToken = &tok
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stitchGraphQLURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating graphql request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("graphql request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stitch graphql returned %d: %s", resp.StatusCode, string(respBody))
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return fmt.Errorf("decoding graphql response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("graphql errors: %s", strings.Join(msgs, "; "))
	}

	if out != nil {
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
				"currency": "ZAR",
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