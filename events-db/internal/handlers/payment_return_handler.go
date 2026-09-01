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
//  2. Hands control back to whichever surface started the checkout. There are
//     three, and they need three different answers — see the dispatch at the
//     bottom of the handler.
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

		// ── Hand-back dispatch ───────────────────────────────────────────────
		// Three surfaces can have opened the gateway, and each needs a
		// different answer. Order matters: the explicit channel tag is checked
		// before the User-Agent heuristic, so a tagged request is never
		// second-guessed by sniffing.

		// 1. Web channel — checkout ran in a browser popup/tab opened by the web
		// frontend (payments initialised with ?channel=web). A deep-link
		// redirect would be rejected by the browser ("address is invalid" on
		// iOS Safari, where no app scheme handler exists in a plain web
		// context). Serve a tiny self-closing page instead: it notifies the
		// opener tab via postMessage, closes itself (allowed — the window was
		// script-opened), and falls back to the web tickets page if closing is
		// blocked. The opener's status watcher independently confirms the
		// payment, so this page carries no state of its own.
		if strings.EqualFold(strings.TrimSpace(c.Query("channel")), services.ChannelWeb) {
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(webReturnHTML(webBaseURL, reference, paypalToken)))
			return
		}

		// 2. iOS app — EventsApp opens checkout in SFSafariViewController
		// (lib/payments.ts openViaSafariView), chosen over
		// ASWebAuthenticationSession because Apple Pay needs a Safari-backed
		// surface. That surface will not launch another app from a custom URL
		// scheme AT ALL — not by 302, not by scripted navigation, not by tapped
		// link. Every one of those answers with "Safari cannot open the page
		// because the address is invalid". Only Universal Links can hand off
		// from here, and Apple ignores those when they arrive via redirect.
		//
		// So this response does not try. The app closes the sheet from its own
		// side once the payment row settles (watchUntilSettled) — that is the
		// actual hand-back. Something still has to answer this request, and the
		// only job of that answer is to not be an error dialog while the sheet
		// is on its way out.
		if isIOSBrowser(c.GetHeader("User-Agent")) {
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(iosAppReturnHTML()))
			return
		}

		// 3. Android app — openAuthSessionAsync is watching for this exact URL
		// and intercepts it before navigation, closing the browser itself.
		// Serving HTML here instead would leave the session open indefinitely.
		c.Redirect(http.StatusFound, buildAppReturnURL(reference, paypalToken))
	}
}

// isIOSBrowser reports whether the return request came from an iOS browser
// surface — which, once the web channel has been ruled out above, means
// EventsApp's SFSafariViewController.
//
// This is a heuristic, and worth being honest about: it fails soft in both
// directions. A missed detection lands on the redirect, i.e. today's behaviour.
// A false positive shows a return button to someone who does not need one — the
// deep link simply does nothing and the page still says the payment went
// through. Neither case can lose a payment, because verification already
// happened above and the webhook reconciles regardless.
//
// iPadOS is the known blind spot: it reports a desktop Macintosh UA by default,
// so an iPad buyer falls through to the redirect. Replacing this with an
// explicit ?channel=ios_app tag from the app removes the guesswork entirely,
// and this branch can stay as-is once that ships — a tagged request never
// reaches here.
func isIOSBrowser(ua string) bool {
	ua = strings.ToLower(ua)
	if ua == "" || strings.Contains(ua, "android") {
		return false
	}
	return strings.Contains(ua, "iphone") ||
		strings.Contains(ua, "ipad") ||
		strings.Contains(ua, "ipod")
}

// iosAppReturnHTML is a deliberately EMPTY page — a dark background and
// nothing else.
//
// It has had three shapes, and the reasoning matters more than the markup:
//
//   1. A 302 to eventsapp://. SFSafariViewController answered it with "Safari
//      cannot open the page because the address is invalid".
//   2. A "Return to TicketHawk" button pointing at the same deep link, on the
//      assumption that a TAPPED custom-scheme anchor would be handed to the
//      system. It is not. The tap produced the identical dialog, on a page
//      that had just told the customer their payment succeeded — which is a
//      worse outcome than the redirect, not a better one.
//   3. This. The sheet is closed by the app once watchUntilSettled sees the
//      payment settle, so nothing here needs to be read or tapped. Content
//      would only draw the eye to a screen that is about to disappear, and it
//      would have to be hedged: this response is served whatever the gateway
//      decided, declines included, so a tick and the word "complete" can be
//      wrong.
//
// The background matches the app's so the moment reads as a rendering pause
// rather than a page the customer was sent to. Keep it that way.
//
// On an older build with no watcher the customer closes the sheet with the X,
// as they do today — from a blank screen rather than an error dialog. Their
// payment is unaffected either way: it was verified above, before this was
// ever rendered, and the webhook reconciles regardless.
func iosAppReturnHTML() string {
	return `<!doctype html>
<html><head><meta name="viewport" content="width=device-width, initial-scale=1"><title></title></head>
<body style="margin:0;min-height:100vh;background:#0a0a0a"></body></html>`
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
      window.opener.postMessage({ source: 'tickethawk', type: 'payment-return', reference: '%s', token: '%s' }, '*');
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

// buildAppReturnURL constructs the eventsapp:// deep link the Android in-app
// browser is redirected to, so openAuthSessionAsync can detect the return and
// close itself. ANDROID ONLY — iOS cannot consume this scheme from a browser
// surface at all (see the dispatch above).
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