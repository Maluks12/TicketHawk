package services

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// PaymentService orchestrates payments across two gateways (Paystack for cards +
// Apple Pay, PayPal for PayPal) while keeping all DB side-effects — payment
// records, booking/cart sync, merch fulfilment, listing-fee and wallet top-up
// confirmation — gateway-agnostic. Callers never branch on provider.
type PaymentService struct {
	pool    *pgxpool.Pool
	cfg     *config.Config
	jobChan chan<- string

	paystack *PaystackGateway
	paypal   *PayPalGateway
}

func NewPaymentService(pool *pgxpool.Pool, cfg *config.Config, jobChan chan<- string) *PaymentService {
	return &PaymentService{
		pool:     pool,
		cfg:      cfg,
		jobChan:  jobChan,
		paystack: NewPaystackGateway(cfg.PaystackSecretKey),
		paypal:   NewPayPalGateway(cfg.PayPalClientID, cfg.PayPalClientSecret, cfg.PayPalWebhookID, cfg.PayPalBaseURL()),
	}
}

// Paystack / PayPal expose the gateways to the webhook handlers for signature
// verification and authoritative status re-fetch.
func (s *PaymentService) Paystack() *PaystackGateway { return s.paystack }
func (s *PaymentService) PayPal() *PayPalGateway     { return s.paypal }

func (s *PaymentService) gatewayForMethod(m models.PaymentMethod) PaymentGateway {
	if m == models.PaymentPayPal {
		return s.paypal
	}
	return s.paystack // card + apple_pay
}

func (s *PaymentService) gatewayByName(name string) PaymentGateway {
	if name == string(GatewayPayPal) {
		return s.paypal
	}
	return s.paystack
}

// newRef builds a globally-unique reference with a routing prefix.
func newRef(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// ChannelWeb marks a payment as initiated from the web frontend. The channel is
// appended to the gateway callback URL so PaymentReturnHandler can respond with
// a self-closing HTML page (web popups) instead of the eventsapp:// deep-link
// redirect (mobile app in-app browser).
const ChannelWeb = "web"

// callbackURLFor returns the gateway callback URL, tagged with ?channel=web
// when the payment was initiated from the web frontend. An empty or unknown
// channel returns the plain callback URL (mobile-app behaviour, unchanged).
func (s *PaymentService) callbackURLFor(channel string) string {
	base := s.cfg.PaymentCallbackURL
	if channel != ChannelWeb || base == "" {
		return base
	}
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	q.Set("channel", ChannelWeb)
	u.RawQuery = q.Encode()
	return u.String()
}

// resolvePayerEmail finds the best email to bill against. Paystack requires one.
func (s *PaymentService) resolvePayerEmail(ctx context.Context, booking *models.Booking) string {
	if e := booking.GuestEmail(); e != "" {
		return e
	}
	if booking.UserID != nil {
		var email string
		if err := s.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, *booking.UserID).Scan(&email); err == nil && email != "" {
			return email
		}
	}
	return fmt.Sprintf("guest+%s@checkout.local", booking.BookingNumber)
}

func (s *PaymentService) userEmail(ctx context.Context, userID string) string {
	var email string
	if err := s.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email); err == nil {
		return email
	}
	return fmt.Sprintf("organiser+%s@checkout.local", userID)
}

// ── Ticket / booking payments ─────────────────────────────────────────────────

// CreatePaymentIntent starts a card (Paystack) payment for a booking — the
// back-compat default used by booking creation, guest, and retry flows.
func (s *PaymentService) CreatePaymentIntent(booking *models.Booking, channel string) (redirectURL string, reference string, err error) {
	return s.CreatePaymentIntentWithMethod(booking, models.PaymentCreditCard, channel)
}

// CreatePaymentIntentWithMethod starts a payment for a booking using the chosen
// method: card/apple_pay route to Paystack, paypal routes to PayPal. Returns the
// hosted-checkout URL the client opens, plus the provider reference.
func (s *PaymentService) CreatePaymentIntentWithMethod(booking *models.Booking, method models.PaymentMethod, channel string) (redirectURL string, reference string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gw := s.gatewayForMethod(method)
	res, err := gw.Initialize(ctx, InitInput{
		Reference:   newRef("bk_"),
		AmountCents: int64(booking.TotalAmount * 100),
		Currency:    booking.Currency,
		Email:       s.resolvePayerEmail(ctx, booking),
		Description: "Booking " + booking.BookingNumber,
		CallbackURL: s.callbackURLFor(channel),
		Channels:    method.PaystackChannels(),
		Metadata:    map[string]string{"booking_id": booking.ID, "booking_number": booking.BookingNumber},
	})
	if err != nil {
		return "", "", fmt.Errorf("initializing %s payment: %w", gw.Name(), err)
	}

	if err := s.createPaymentRecord(ctx, booking, gw.Name(), res.Reference, &res.RedirectURL, method, nil); err != nil {
		return "", "", fmt.Errorf("persisting payment record for booking %s: %w", booking.ID, err)
	}
	return res.RedirectURL, res.Reference, nil
}

// CreateCartPaymentIntent starts a card (Paystack) payment for a multi-item cart.
func (s *PaymentService) CreateCartPaymentIntent(primaryBooking *models.Booking, cartID string, totalAmount float64, channel string) (redirectURL string, reference string, err error) {
	return s.CreateCartPaymentIntentWithMethod(primaryBooking, cartID, totalAmount, models.PaymentCreditCard, channel)
}

// CreateCartPaymentIntentWithMethod starts a cart payment using the chosen method.
func (s *PaymentService) CreateCartPaymentIntentWithMethod(primaryBooking *models.Booking, cartID string, totalAmount float64, method models.PaymentMethod, channel string) (redirectURL string, reference string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gw := s.gatewayForMethod(method)
	res, err := gw.Initialize(ctx, InitInput{
		Reference:   newRef("cart_"),
		AmountCents: int64(totalAmount * 100),
		Currency:    primaryBooking.Currency,
		Email:       s.resolvePayerEmail(ctx, primaryBooking),
		Description: "Cart " + cartID,
		CallbackURL: s.callbackURLFor(channel),
		Channels:    method.PaystackChannels(),
		Metadata:    map[string]string{"cart_id": cartID},
	})
	if err != nil {
		return "", "", fmt.Errorf("initializing cart %s payment: %w", gw.Name(), err)
	}

	if err := s.createPaymentRecord(ctx, primaryBooking, gw.Name(), res.Reference, &res.RedirectURL, method, &cartID); err != nil {
		return "", "", fmt.Errorf("persisting payment record for cart %s: %w", cartID, err)
	}
	return res.RedirectURL, res.Reference, nil
}

// CreateListingFeeIntent starts a card (Paystack) payment for the platform
// listing fee. The returned reference is persisted into events by the handler so
// the webhook can match it.
func (s *PaymentService) CreateListingFeeIntent(eventID, organizerID string, amount float64) (redirectURL string, reference string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := s.paystack.Initialize(ctx, InitInput{
		Reference:   newRef("lf_"),
		AmountCents: int64(amount * 100),
		Currency:    models.DefaultCurrency,
		Email:       s.userEmail(ctx, organizerID),
		Description: "Listing fee " + eventID,
		CallbackURL: s.cfg.PaymentCallbackURL,
		Channels:    []string{"card"},
		Metadata:    map[string]string{"event_id": eventID, "kind": "listing_fee"},
	})
	if err != nil {
		return "", "", fmt.Errorf("CreateListingFeeIntent: %w", err)
	}
	return res.RedirectURL, res.Reference, nil
}

// CreateAdTopupIntent starts a card (Paystack) payment to top up an organiser's
// ad wallet, and records a pending top-up keyed by the Paystack reference.
func (s *PaymentService) CreateAdTopupIntent(organizerID string, amount float64) (redirectURL string, reference string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ref := newRef("topup_")
	res, err := s.paystack.Initialize(ctx, InitInput{
		Reference:   ref,
		AmountCents: int64(amount * 100),
		Currency:    models.DefaultCurrency,
		Email:       s.userEmail(ctx, organizerID),
		Description: "Ad wallet top-up",
		CallbackURL: s.cfg.PaymentCallbackURL,
		Channels:    []string{"card"},
		Metadata:    map[string]string{"organizer_id": organizerID, "kind": "ad_topup"},
	})
	if err != nil {
		return "", "", fmt.Errorf("CreateAdTopupIntent: %w", err)
	}

	if err := repository.CreateAdWalletTopup(s.pool, organizerID, amount, res.Reference); err != nil {
		return "", "", fmt.Errorf("recording pending top-up: %w", err)
	}
	return res.RedirectURL, res.Reference, nil
}

// createPaymentRecord upserts the payments row for a booking/cart.
func (s *PaymentService) createPaymentRecord(ctx context.Context, booking *models.Booking, gateway PaymentGatewayName, reference string, redirectURL *string, method models.PaymentMethod, cartID *string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO payments (
			booking_id, payment_gateway, payment_intent_id,
			amount, currency, payment_method, status,
			gateway_reference, gateway_redirect_url, cart_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (booking_id) DO UPDATE SET
			payment_gateway      = EXCLUDED.payment_gateway,
			payment_intent_id    = EXCLUDED.payment_intent_id,
			payment_method       = EXCLUDED.payment_method,
			gateway_reference    = EXCLUDED.gateway_reference,
			gateway_redirect_url = EXCLUDED.gateway_redirect_url,
			cart_id              = EXCLUDED.cart_id,
			webhook_received     = FALSE,
			status               = 'pending',
			updated_at           = CURRENT_TIMESTAMP
	`,
		booking.ID,
		string(gateway),
		reference,
		booking.TotalAmount,
		booking.Currency,
		string(method),
		string(models.PaymentPending),
		reference,
		redirectURL,
		cartID,
	)
	return err
}

// ── Refunds ───────────────────────────────────────────────────────────────────

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
	if payment.GatewayReference == nil {
		return fmt.Errorf("no gateway reference for booking %s", bookingID)
	}

	txnID := ""
	if payment.TransactionID != nil {
		txnID = *payment.TransactionID
	}

	gw := s.gatewayByName(payment.PaymentGateway)
	res, err := gw.Refund(ctx, *payment.GatewayReference, txnID, int64(payment.Amount*100), payment.Currency, "Customer requested refund")
	if err != nil {
		return fmt.Errorf("%s refund: %w", gw.Name(), err)
	}

	if _, err := s.pool.Exec(ctx, `
		UPDATE payments
		SET refund_id       = $1,
		    status          = $2::payment_status,
		    refunded_amount = amount,
		    refunded_at     = CURRENT_TIMESTAMP,
		    updated_at      = CURRENT_TIMESTAMP
		WHERE booking_id = $3
	`, res.RefundID, string(res.Status), bookingID); err != nil {
		log.Warn().Err(err).Str("booking_id", bookingID).Msg("payment: refund OK but DB update failed")
	}
	return nil
}

// ── Webhook application (shared by both gateways) ─────────────────────────────

// ApplyVerifiedPayment applies an authoritative, gateway-verified status to the
// correct destination: wallet top-up, listing fee, or booking/cart. Called by
// the Paystack and PayPal webhook handlers after they re-fetch the truth from
// the provider.
func (s *PaymentService) ApplyVerifiedPayment(ctx context.Context, v *VerifyResult) error {
	// Wallet top-ups (Paystack only) are routed by reference prefix.
	if strings.HasPrefix(v.Reference, "topup_") {
		switch v.Status {
		case models.PaymentCompleted:
			return repository.ConfirmAdWalletTopup(s.pool, v.Reference)
		case models.PaymentFailed:
			return repository.FailAdWalletTopup(s.pool, v.Reference)
		default:
			return nil
		}
	}

	// Listing fees (Paystack only): a completed payment whose reference matches
	// an event's stored listing-fee reference confirms that event.
	if v.Gateway == GatewayPaystack && v.Status == models.PaymentCompleted {
		eventID, matched, lfErr := repository.ConfirmListingFeeByPaymentRequest(s.pool, v.Reference)
		if lfErr != nil {
			log.Warn().Err(lfErr).Str("reference", v.Reference).Msg("webhook: listing fee confirmation failed")
		} else if matched {
			log.Info().Str("reference", v.Reference).Str("event_id", eventID).
				Msg("webhook: listing fee confirmed")
			return nil
		}
	}

	return s.applyBookingPayment(ctx, v)
}

func (s *PaymentService) applyBookingPayment(ctx context.Context, v *VerifyResult) error {
	var currentStatus, bookingID string
	var cartID *string

	err := s.pool.QueryRow(ctx, `
		SELECT status, booking_id, cart_id FROM payments WHERE gateway_reference = $1
	`, v.Reference).Scan(&currentStatus, &bookingID, &cartID)
	if err != nil {
		log.Warn().Str("reference", v.Reference).Msg("webhook: no payment record found, ignoring")
		return nil
	}

	if currentStatus == string(models.PaymentCompleted) || currentStatus == string(models.PaymentRefunded) {
		log.Info().Str("reference", v.Reference).Str("status", currentStatus).Msg("webhook: duplicate delivery, ignoring")
		return nil
	}

	var transactionID *string
	if v.TransactionID != "" {
		transactionID = &v.TransactionID
	}

	if err := s.pool.QueryRow(ctx, `
		UPDATE payments
		SET status         = $1::payment_status,
		    payment_method = $2::payment_method_type,
		    transaction_id = COALESCE($3, transaction_id),
		    webhook_received = TRUE,
		    completed_at   = CASE WHEN $1::text = 'completed' THEN CURRENT_TIMESTAMP ELSE completed_at END,
		    updated_at     = CURRENT_TIMESTAMP
		WHERE gateway_reference = $4
		RETURNING booking_id
	`, string(v.Status), string(v.Method), transactionID, v.Reference).Scan(&bookingID); err != nil {
		return fmt.Errorf("updating payment for reference %s: %w", v.Reference, err)
	}

	if err := s.syncBookingStatus(ctx, bookingID, v.Status); err != nil {
		log.Warn().Err(err).Str("booking_id", bookingID).Msg("webhook: payment updated but booking sync failed")
	}

	log.Info().Str("reference", v.Reference).Str("status", string(v.Status)).
		Str("method", string(v.Method)).Str("booking_id", bookingID).
		Msg("webhook: payment updated")

	if cartID != nil {
		additionalIDs, syncErr := s.syncAllCartBookings(ctx, *cartID, bookingID, v.Status)
		if syncErr != nil {
			log.Warn().Err(syncErr).Str("cart_id", *cartID).Msg("webhook: failed to sync cart bookings")
		}

		if v.Status == models.PaymentCompleted {
			if err := repository.FulfillMerchOrders(ctx, s.pool, *cartID, v.Reference); err != nil {
				log.Error().Err(err).Str("cart_id", *cartID).Str("reference", v.Reference).
					Msg("webhook: merch fulfillment failed — manual reconciliation required")
			}
			for _, id := range append([]string{bookingID}, additionalIDs...) {
				s.enqueue(id)
			}
		}
	} else if v.Status == models.PaymentCompleted {
		s.enqueue(bookingID)
	}

	return nil
}

func (s *PaymentService) enqueue(bookingID string) {
	select {
	case s.jobChan <- bookingID:
		log.Info().Str("booking_id", bookingID).Msg("webhook: enqueued booking job")
	default:
		log.Warn().Str("booking_id", bookingID).Msg("webhook: booking job channel full, job dropped")
	}
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
		SET booking_status = $1::booking_status,
		    payment_status = $2::payment_status,
		    confirmed_at   = CASE WHEN $1::text = 'confirmed' THEN CURRENT_TIMESTAMP ELSE confirmed_at END,
		    updated_at     = CURRENT_TIMESTAMP
		WHERE cart_id = $3
		  AND id      != $4
		  AND booking_status IN ('pending', 'cancelled')
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
	}
	return additionalIDs, nil
}

func (s *PaymentService) syncBookingStatus(ctx context.Context, bookingID string, status models.PaymentStatus) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE bookings
		SET payment_status = $1::payment_status, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, string(status), bookingID); err != nil {
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

	_, err := s.pool.Exec(ctx, `
		UPDATE bookings
		SET booking_status = $1::booking_status,
		    confirmed_at   = CASE WHEN $1::text = 'confirmed' THEN CURRENT_TIMESTAMP ELSE confirmed_at END,
		    updated_at     = CURRENT_TIMESTAMP
		WHERE id = $2
	`, string(bookingStatus), bookingID)
	return err
}

// ── Reads ─────────────────────────────────────────────────────────────────────

func (s *PaymentService) GetPaymentStatus(bookingID string) (*models.Payment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.GetPaymentByBookingID(ctx, bookingID)
}

func (s *PaymentService) GetPaymentByBookingID(ctx context.Context, bookingID string) (*models.Payment, error) {
	var p models.Payment
	err := s.pool.QueryRow(ctx, `
		SELECT id, booking_id, payment_gateway, payment_intent_id, transaction_id,
		       amount, currency, payment_method, status,
		       gateway_reference, gateway_redirect_url,
		       refund_id, refunded_amount, refunded_at,
		       webhook_received, created_at, updated_at, completed_at
		FROM payments
		WHERE booking_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, bookingID).Scan(
		&p.ID, &p.BookingID, &p.PaymentGateway, &p.PaymentIntentID, &p.TransactionID,
		&p.Amount, &p.Currency, &p.PaymentMethod, &p.Status,
		&p.GatewayReference, &p.GatewayRedirectURL,
		&p.RefundID, &p.RefundedAmount, &p.RefundedAt,
		&p.WebhookReceived, &p.CreatedAt, &p.UpdatedAt, &p.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("payment not found for booking %s: %w", bookingID, err)
	}
	return &p, nil
}

// MarkPayPalCancelled flips a PayPal payment (and its booking) to failed when
// the payer abandons the approval and PayPal redirects to the cancel URL.
// Without it, the order stays CREATED/non-terminal and the app would poll until
// timeout. Idempotent: a later webhook/verify with the same result is harmless.
func (s *PaymentService) MarkPayPalCancelled(ctx context.Context, orderID string) error {
	return s.ApplyVerifiedPayment(ctx, &VerifyResult{
		Gateway:   GatewayPayPal,
		Reference: orderID,
		Status:    models.PaymentFailed,
		Method:    models.PaymentPayPal,
	})
}

// ReconcilePendingPayment returns the booking's payment, but if it is still
// pending/processing it first asks the gateway for the authoritative status
// and applies it. This makes the status endpoint self-healing: when a user
// declines and closes the hosted checkout, the gateway never redirects to
// /payments/return, so without this the stored status would stay 'pending'
// forever. A gateway Verify() reports 'failed'/'abandoned' for such attempts,
// which maps to PaymentFailed — so the app learns the payment didn't go through.
func (s *PaymentService) ReconcilePendingPayment(ctx context.Context, bookingID string) (*models.Payment, error) {
	p, err := s.GetPaymentByBookingID(ctx, bookingID)
	if err != nil {
		return nil, err
	}

	// Already settled — nothing to reconcile.
	if p.Status == models.PaymentCompleted || p.Status == models.PaymentFailed || p.Status == models.PaymentRefunded {
		return p, nil
	}
	if p.GatewayReference == nil || *p.GatewayReference == "" {
		return p, nil
	}

	res, vErr := s.gatewayByName(p.PaymentGateway).Verify(ctx, *p.GatewayReference)
	if vErr != nil {
		log.Warn().Err(vErr).Str("booking_id", bookingID).Str("reference", *p.GatewayReference).
			Msg("payment status: gateway verify failed; returning stored status")
		return p, nil
	}

	// Only a terminal verdict flips the record; an in-progress verify is a no-op.
	if res.Status == models.PaymentCompleted || res.Status == models.PaymentFailed {
		if aErr := s.ApplyVerifiedPayment(ctx, res); aErr != nil {
			log.Warn().Err(aErr).Str("booking_id", bookingID).Msg("payment status: apply after verify failed")
		}
		if fresh, rErr := s.GetPaymentByBookingID(ctx, bookingID); rErr == nil {
			return fresh, nil
		}
	}
	return p, nil
}

// GetPaymentStatusReconciled is GetPaymentStatus plus an on-demand gateway
// reconciliation for non-terminal payments. Used by the status endpoint.
func (s *PaymentService) GetPaymentStatusReconciled(bookingID string) (*models.Payment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.ReconcilePendingPayment(ctx, bookingID)
}

// EnqueueJob sends a confirmed booking ID to the booking worker pipeline.
// Used for free bookings that bypass the payment flow.
func (s *PaymentService) EnqueueJob(bookingID string) {
	s.enqueue(bookingID)
}