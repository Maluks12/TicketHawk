/**
 * lib/payments.ts
 *
 * Payment client for the Paystack + PayPal (gateway-neutral) backend, using an
 * in-app browser (expo-web-browser) for the hosted-checkout step.
 *
 * Why the in-app browser instead of a WebView: Apple Pay only renders in a
 * Safari-backed surface (SFSafariViewController / ASWebAuthenticationSession on
 * iOS, Chrome Custom Tabs on Android) — never in a react-native-webview. The
 * in-app browser keeps the user inside the app (no app-switch to Safari) while
 * still supporting Apple Pay, 3-D Secure, OTP, and PayPal reliably.
 *
 * Flow per booking:
 *   1. startBookingPayment(bookingId, method) → { authorization_url, ... }
 *   2. openGatewayCheckout(authorization_url) opens the in-app browser
 *   3. the gateway redirects to the backend /payments/return, which verifies
 *      (Paystack) and 302-redirects to eventsapp://payment/return. On Android
 *      the auth session detects that scheme and closes itself; on iOS we run
 *      SFSafariViewController and dismiss it from a Linking listener instead
 *      (see openGatewayCheckout for why they differ)
 *   4. pollPayment(bookingId) reads the authoritative status
 *
 * This module is ADDITIVE — it reuses the axios `api` instance from ./api.
 */

import * as WebBrowser from 'expo-web-browser';
import * as Linking from 'expo-linking';
import { Platform } from 'react-native';
import { api } from './api';

export type PaymentMethod = 'card' | 'paypal' | 'apple_pay' | 'instant_eft';

export type PaymentStatusValue =
  | 'pending'
  | 'processing'
  | 'completed'
  | 'failed'
  | 'refunded';

export interface StartPaymentResult {
  authorization_url: string;
  reference: string;
  payment_method: string;
  amount: number;
  currency: string;
}

export interface PaymentStatus {
  booking_id: string;
  payment_status: PaymentStatusValue;
  payment_method?: string;
  amount: number;
  currency: string;
  authorization_url?: string | null;
  reference?: string | null;
  transaction_id?: string | null;
  webhook_received?: boolean;
}

/**
 * The deep link the backend's /payments/return redirects to. Expo's
 * openAuthSessionAsync watches for this scheme to know the hosted flow finished
 * and to dismiss the in-app browser. Must match the app.json "scheme".
 */
export const PAYMENT_RETURN_URL = 'eventsapp://payment/return';

export type CheckoutOutcome =
  | { type: 'returned'; url?: string } // browser hit the return scheme (likely paid)
  | { type: 'dismissed' };             // user closed the browser themselves

/**
 * openGatewayCheckout opens the gateway's hosted-checkout page in an in-app
 * browser and resolves once it closes. 'returned' means the gateway redirected
 * back through our scheme; 'dismissed' means the user closed it manually.
 *
 * Note (iOS): openAuthSessionAsync shows a one-time "<App> wants to use
 * <domain>" system prompt. If you'd rather avoid it, swap to
 * WebBrowser.openBrowserAsync + a Linking listener (see app/payment/[id].tsx).
 */
export async function openGatewayCheckout(authorizationUrl: string): Promise<CheckoutOutcome> {
  // iOS gets SFSafariViewController; everything else keeps the auth session.
  //
  // WHY THE SPLIT. Apple Pay needs a Safari-backed surface, and
  // ASWebAuthenticationSession — especially with preferEphemeralSession, which
  // deliberately isolates it from Safari — is a restricted context intended for
  // OAuth. Whether Apple Pay renders inside it is genuinely unclear;
  // SFSafariViewController is the fuller context and the safer bet.
  //
  // Android is left untouched on purpose. There is no Apple Pay to gain there,
  // and the auth-session path is what the Android deep-link ghost guard in
  // app/payment/[id].tsx is tuned against — that guard exists because the
  // eventsapp:// redirect both resolves the session AND gets picked up by
  // expo-router, pushing a phantom screen. Changing Android's opener would
  // disturb a fix that took real effort to land, for no benefit.
  //
  // TO REVERT: return openViaAuthSession(authorizationUrl) unconditionally.
  if (Platform.OS === 'ios') {
    return openViaSafariView(authorizationUrl);
  }
  return openViaAuthSession(authorizationUrl);
}

/** The original path: the session itself detects the return scheme and closes. */
async function openViaAuthSession(authorizationUrl: string): Promise<CheckoutOutcome> {
  const result = await WebBrowser.openAuthSessionAsync(authorizationUrl, PAYMENT_RETURN_URL, {
    showInRecents: false,
    // Ephemeral session: no shared Safari cookies, so iOS does NOT show the
    // "<App> Wants to Use <domain> to Sign In" consent dialog.
    preferEphemeralSession: true,
  });
  if (result.type === 'success') {
    return { type: 'returned', url: (result as any).url };
  }
  // 'cancel' | 'dismiss' | 'locked'
  return { type: 'dismissed' };
}

/**
 * SFSafariViewController. Unlike the auth session it will NOT close itself when
 * the gateway redirects to our scheme, so we listen for the deep link, dismiss
 * the browser ourselves, and resolve.
 *
 * Two things can finish this: the deep link arriving, or the user tapping Done.
 * Both are wired, and `settled` makes sure whichever lands first wins — without
 * it a Done tap arriving just after the redirect would report 'dismissed' on a
 * payment that actually succeeded, and the caller would show a cancellation for
 * money already taken.
 */
async function openViaSafariView(authorizationUrl: string): Promise<CheckoutOutcome> {
  return new Promise<CheckoutOutcome>((resolve) => {
    let settled = false;
    const finish = (outcome: CheckoutOutcome) => {
      if (settled) return;
      settled = true;
      sub.remove();
      resolve(outcome);
    };

    const sub = Linking.addEventListener('url', ({ url }) => {
      if (!url || !url.startsWith(PAYMENT_RETURN_URL)) return;
      // Close the browser we opened; the caller then polls for the real status.
      WebBrowser.dismissBrowser();
      finish({ type: 'returned', url });
    });

    WebBrowser.openBrowserAsync(authorizationUrl, {
      showInRecents: false,
      // Keep the sheet visually part of the app rather than a full takeover.
      presentationStyle: WebBrowser.WebBrowserPresentationStyle.PAGE_SHEET,
      dismissButtonStyle: 'cancel',
    })
      .then(() => {
        // Resolves when the browser closes for ANY reason, including our own
        // dismissBrowser() above — hence the settled guard.
        finish({ type: 'dismissed' });
      })
      .catch(() => finish({ type: 'dismissed' }));
  });
}

/**
 * startBookingPayment initialises (or re-initialises) a payment for a booking
 * with the chosen method and returns the gateway's hosted-checkout URL.
 *
 * The method query param is honoured for authenticated bookings. Guest bookings
 * don't have a method-aware start route, so the guest path should fall back to
 * the authorization_url returned at booking-creation time (card).
 */
export async function startBookingPayment(
  bookingId: string,
  method: PaymentMethod = 'card',
): Promise<StartPaymentResult> {
  const res = await api.post(`/v1/bookings/${bookingId}/payment`, {}, { params: { method } });
  return normaliseStart(res.data);
}

/** retryBookingPayment re-initialises a previously failed/abandoned payment. */
export async function retryBookingPayment(
  bookingId: string,
  method: PaymentMethod = 'card',
): Promise<StartPaymentResult> {
  const res = await api.post(`/v1/bookings/${bookingId}/payment/retry`, {}, { params: { method } });
  return normaliseStart(res.data);
}

/**
 * A guest has no auth token, so their payment is addressed by booking number +
 * email against the /v1/guest/* routes instead of the signed-in /v1/bookings/*
 * routes.
 */
export interface GuestRef {
  bookingNumber: string;
  email: string;
}

/** fetchPaymentStatus reads the current payment state for a booking (guest-aware). */
export async function fetchPaymentStatus(bookingId: string, guest?: GuestRef): Promise<PaymentStatus> {
  if (guest) {
    const res = await api.get(
      `/v1/guest/bookings/${encodeURIComponent(guest.bookingNumber)}/payment`,
      { params: { email: guest.email } },
    );
    // The guest endpoint returns booking_number, not booking_id — backfill it.
    return { ...(res.data as PaymentStatus), booking_id: bookingId };
  }
  const res = await api.get(`/v1/bookings/${bookingId}/payment`);
  return res.data as PaymentStatus;
}

/** retryGuestPayment re-initialises a guest's (card) payment by booking number. */
export async function retryGuestPayment(bookingNumber: string, email: string): Promise<StartPaymentResult> {
  const res = await api.post(
    `/v1/guest/bookings/${encodeURIComponent(bookingNumber)}/payment/retry`,
    {},
    { params: { email } },
  );
  return normaliseStart(res.data);
}

/**
 * pollPayment polls until the payment reaches a terminal state (completed /
 * failed / refunded) or the timeout elapses. Returns the last status seen.
 */
export async function pollPayment(
  bookingId: string,
  intervalMs = 2500,
  timeoutMs = 150_000,
  guest?: GuestRef,
): Promise<PaymentStatus> {
  const deadline = Date.now() + timeoutMs;
  let last: PaymentStatus | null = null;
  while (Date.now() < deadline) {
    try {
      last = await fetchPaymentStatus(bookingId, guest);
      if (
        last.payment_status === 'completed' ||
        last.payment_status === 'failed' ||
        last.payment_status === 'refunded'
      ) {
        return last;
      }
    } catch {
      // transient network/5xx — keep polling until the deadline
    }
    await sleep(intervalMs);
  }
  if (last) return last;
  throw new Error('payment status poll timed out');
}

// ── helpers ──────────────────────────────────────────────────────────────────

function normaliseStart(data: any): StartPaymentResult {
  // Tolerate both the new keys and any legacy client_token/payment_request_id.
  return {
    authorization_url: data?.authorization_url ?? data?.client_token ?? '',
    reference: data?.reference ?? data?.payment_request_id ?? '',
    payment_method: data?.payment_method ?? '',
    amount: Number(data?.amount ?? 0),
    currency: data?.currency ?? 'ZAR',
  };
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}