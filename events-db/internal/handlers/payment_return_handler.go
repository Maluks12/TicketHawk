package handlers

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"events-db/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// PaymentReturnHandler is the public landing the gateways redirect the customer
// to after a hosted-checkout attempt. Both Paystack (callback_url) and PayPal
// (return_url) are configured to PAYMENT_CALLBACK_URL, which should point here.
//
// It does two jobs:
//
//  1. Best-effort SYNCHRONOUS verification for Paystack, so the booking's
//     payment status flips immediately rather than waiting on the async webhook.
//     This matters in test mode, where Paystack can't reach a localhost webhook.
//     The webhook remains the source of truth and is idempotent, so applying the
//     verified result here and again from the webhook later is harmless.
//
//  2. Redirects the in-app browser back into the mobile app via the
//     eventsapp://payment/return deep link. Expo's openAuthSessionAsync watches
//     for that scheme, closes the browser, and hands control back to the app,
//     which then polls GET /v1/bookings/:id/payment for the authoritative state.
//
// PayPal returns carry ?token=<orderID>&PayerID=...; those are reconciled by the
// PayPal webhook, so here we simply bounce back and let the app poll.
func PaymentReturnHandler(paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Paystack: ?reference=...&trxref=...   PayPal: ?token=...&PayerID=...
		reference := c.Query("reference")
		if reference == "" {
			reference = c.Query("trxref")
		}
		paypalToken := c.Query("token")

		if reference != "" {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
			defer cancel()

			if vr, err := paymentService.Paystack().Verify(ctx, reference); err != nil {
				log.Warn().Err(err).Str("reference", reference).
					Msg("payment return: paystack verify failed (webhook will reconcile)")
			} else if applyErr := paymentService.ApplyVerifiedPayment(ctx, vr); applyErr != nil {
				log.Warn().Err(applyErr).Str("reference", reference).
					Msg("payment return: apply failed (webhook will reconcile)")
			} else {
				log.Info().Str("reference", reference).Str("status", string(vr.Status)).
					Msg("payment return: verified synchronously")
			}
		}

		c.Redirect(http.StatusFound, buildAppReturnURL(reference, paypalToken))
	}
}

// buildAppReturnURL constructs the eventsapp:// deep link the in-app browser is
// redirected to so Expo's AuthSession can detect the return and close.
func buildAppReturnURL(reference, paypalToken string) string {
	q := url.Values{}
	if reference != "" {
		q.Set("reference", reference)
	}
	if paypalToken != "" {
		q.Set("token", paypalToken)
	}
	deep := "eventsapp://payment/return"
	if enc := q.Encode(); enc != "" {
		deep += "?" + enc
	}
	return deep
}