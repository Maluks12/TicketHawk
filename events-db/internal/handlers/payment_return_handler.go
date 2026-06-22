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
// PayPal returns carry ?token=<orderID>&PayerID=...; on approval we verify and
// capture the order here synchronously (the webhook/app poll remain idempotent
// backups), and on cancel we mark it failed so the app stops polling.
func PaymentReturnHandler(paymentService *services.PaymentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Paystack: ?reference=...&trxref=...   PayPal: ?token=...&PayerID=...
		reference := c.Query("reference")
		if reference == "" {
			reference = c.Query("trxref")
		}
		paypalToken := c.Query("token")
		paypalCancelled := c.Query("pp_cancel") == "1"

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
		} else if paypalCancelled && paypalToken != "" {
			// PayPal redirected to the cancel URL — the payer abandoned the
			// approval. Fail it now so the app stops polling a CREATED order.
			ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
			defer cancel()

			if err := paymentService.MarkPayPalCancelled(ctx, paypalToken); err != nil {
				log.Warn().Err(err).Str("token", paypalToken).
					Msg("payment return: paypal cancel mark failed")
			} else {
				log.Info().Str("token", paypalToken).
					Msg("payment return: paypal payment cancelled by payer")
			}
		} else if paypalToken != "" {
			// PayPal approval return — capture the order SYNCHRONOUSLY here, the
			// same way Paystack is verified above. PayPal.Verify() GETs the order
			// and, if APPROVED, captures it; ApplyVerifiedPayment is idempotent,
			// so a later poll or webhook applying the same result is harmless.
			// This removes the dependency on the app's status poll (which can be
			// rate-limited or skipped if the user leaves the screen), so the
			// booking confirms and tickets issue even if the poll never runs.
			ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
			defer cancel()

			if vr, err := paymentService.PayPal().Verify(ctx, paypalToken); err != nil {
				log.Warn().Err(err).Str("token", paypalToken).
					Msg("payment return: paypal verify/capture failed (app poll will reconcile)")
			} else if applyErr := paymentService.ApplyVerifiedPayment(ctx, vr); applyErr != nil {
				log.Warn().Err(applyErr).Str("token", paypalToken).
					Msg("payment return: paypal apply failed (app poll will reconcile)")
			} else {
				log.Info().Str("token", paypalToken).Str("status", string(vr.Status)).
					Msg("payment return: paypal captured synchronously")
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