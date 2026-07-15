package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
func PaymentReturnHandler(paymentService *services.PaymentService, webBaseURL string) gin.HandlerFunc {
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

		// Web-channel returns: the checkout ran in a browser popup/tab opened by
		// the web frontend (payments were initialised with ?channel=web). A
		// deep-link redirect would be rejected by the browser ("address is
		// invalid" on iOS Safari, where no app scheme handler exists in a plain
		// web context). Serve a tiny self-closing page instead: it notifies the
		// opener tab via postMessage, closes itself (allowed — the window was
		// script-opened), and falls back to the web tickets page if closing is
		// blocked. The opener's status watcher independently confirms the
		// payment, so this page carries no state of its own.
		if strings.EqualFold(strings.TrimSpace(c.Query("channel")), services.ChannelWeb) {
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(webReturnHTML(webBaseURL, reference, paypalToken)))
			return
		}

		c.Redirect(http.StatusFound, buildAppReturnURL(reference, paypalToken))
	}
}

// webReturnHTML renders the self-closing landing for web-channel checkouts.
func webReturnHTML(webBaseURL, reference, paypalToken string) string {
	fallback := strings.TrimRight(webBaseURL, "/") + "/tickets"
	ref := url.QueryEscape(reference)
	tok := url.QueryEscape(paypalToken)
	return fmt.Sprintf(`<!doctype html>
<html><head><meta name="viewport" content="width=device-width, initial-scale=1"><title>Payment received</title></head>
<body style="margin:0;height:100vh;display:flex;flex-direction:column;align-items:center;justify-content:center;font-family:sans-serif;background:#0a0a0a;color:#eee;gap:10px">
<div style="font-size:34px">&#10003;</div>
<div style="font-weight:700">Payment received</div>
<div style="font-size:13px;color:#999">Returning to TicketHawk&#8230;</div>
<script>
(function () {
  try {
    if (window.opener && !window.opener.closed) {
      window.opener.postMessage({ source: 'tickethawk', type: 'payment_return', reference: '%s', token: '%s' }, '*');
    }
  } catch (e) {}
  // Close this popup/tab; the opener's watcher takes it from here.
  setTimeout(function () { try { window.close(); } catch (e) {} }, 400);
  // If the browser refused to close us, land somewhere useful.
  setTimeout(function () { window.location.replace('%s'); }, 1600);
})();
</script>
</body></html>`, ref, tok, fallback)
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