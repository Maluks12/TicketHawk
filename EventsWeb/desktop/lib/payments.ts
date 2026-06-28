/**
 * lib/payments.ts
 *
 * Payment client for the Paystack + PayPal (gateway-neutral) backend,
 * adapted for web (Next.js desktop).
 *
 * Web vs mobile differences:
 *  - Uses window.open() popup instead of expo-web-browser
 *  - No Apple Pay (requires Safari-backed surfaces; not available on web desktop)
 *  - Supports: card, paypal, instant_eft
 *  - Same polling loop and API routes as the mobile client
 *
 * Flow per booking:
 *   1. startBookingPayment(bookingId, method) → { authorization_url, ... }
 *      For card: the booking creation already returns a Paystack URL in
 *      client_token — reuse it to skip a round-trip (see purchase/page.tsx).
 *   2. openGatewayCheckout(authorization_url) opens a centered popup and
 *      resolves once the popup closes.
 *   3. pollPayment(bookingId) reads the authoritative server-side status.
 *
 * If the popup is blocked by the browser, the caller is given a 'blocked'
 * outcome and should fall back to window.location redirect.
 */

import { api } from './api';

export type PaymentMethod = 'card' | 'paypal' | 'instant_eft';

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

export type CheckoutOutcome =
  | { type: 'returned' }   // popup closed (gateway redirected or user closed after paying)
  | { type: 'dismissed' }  // user explicitly closed the popup
  | { type: 'blocked' };   // browser blocked the popup — caller should redirect instead

/**
 * openGatewayCheckout opens the gateway's hosted-checkout URL in a centred
 * popup window and resolves once the popup closes.
 *
 * 'returned' — popup closed. Could be a successful gateway redirect OR a
 *              manual close. Always follow up with pollPayment() to get the
 *              authoritative status from the server.
 * 'blocked'  — the browser blocked the popup (common with user-gesture missing).
 *              Fall back to a full-page redirect (window.location.href).
 *
 * Note: We cannot reliably distinguish between "gateway redirected back" and
 * "user closed the popup" on web, because the gateway redirects to the backend
 * return URL which then bounces to a mobile deep link (eventsapp://). A browser
 * can't navigate to that scheme, so it shows an error page inside the popup
 * rather than closing it. Polling is the authoritative way to confirm the outcome.
 */
export function openGatewayCheckout(authorizationUrl: string): Promise<CheckoutOutcome> {
  return new Promise((resolve) => {
    const w    = 720;
    const h    = 700;
    const left = Math.max(0, Math.round((window.screen.width  - w) / 2));
    const top  = Math.max(0, Math.round((window.screen.height - h) / 2));
    const features = [
      `width=${w}`, `height=${h}`,
      `left=${left}`, `top=${top}`,
      'scrollbars=yes', 'resizable=yes',
      'toolbar=no', 'menubar=no', 'location=no', 'status=no',
    ].join(',');

    const popup = window.open(authorizationUrl, 'eventspay_checkout', features);

    if (!popup || popup.closed) {
      // Popup was blocked immediately.
      resolve({ type: 'blocked' });
      return;
    }

    // Poll every 500 ms — cheap and reliable across all browsers.
    const timer = setInterval(() => {
      try {
        if (popup.closed) {
          clearInterval(timer);
          resolve({ type: 'returned' });
        }
      } catch {
        // Cross-origin access check can throw; ignore — popup is still open.
      }
    }, 500);
  });
}

/**
 * startBookingPayment initialises (or re-initialises) a payment for a booking
 * with the chosen method and returns the gateway's hosted-checkout URL.
 *
 * For card payments on a fresh booking, prefer reusing the authorization_url
 * (client_token) that the booking creation already returned — it saves a
 * round-trip. Call this explicitly for PayPal / Instant EFT or when retrying.
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
  const res = await api.post(
    `/v1/bookings/${bookingId}/payment/retry`,
    {},
    { params: { method } },
  );
  return normaliseStart(res.data);
}

export interface GuestRef {
  bookingNumber: string;
  email: string;
}

/**
 * fetchPaymentStatus reads the current payment state for a booking.
 * Guest bookings use a separate route keyed by booking number + email.
 */
export async function fetchPaymentStatus(
  bookingId: string,
  guest?: GuestRef,
): Promise<PaymentStatus> {
  if (guest) {
    const res = await api.get(
      `/v1/guest/bookings/${encodeURIComponent(guest.bookingNumber)}/payment`,
      { params: { email: guest.email } },
    );
    // The guest endpoint returns booking_number instead of booking_id — backfill it.
    return { ...(res.data as PaymentStatus), booking_id: bookingId };
  }
  const res = await api.get(`/v1/bookings/${bookingId}/payment`);
  return res.data as PaymentStatus;
}

/** retryGuestPayment re-initialises a guest's card payment by booking number. */
export async function retryGuestPayment(
  bookingNumber: string,
  email: string,
): Promise<StartPaymentResult> {
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
  timeoutMs  = 150_000,
  guest?: GuestRef,
): Promise<PaymentStatus> {
  const deadline = Date.now() + timeoutMs;
  let last: PaymentStatus | null = null;

  while (Date.now() < deadline) {
    try {
      last = await fetchPaymentStatus(bookingId, guest);
      if (
        last.payment_status === 'completed' ||
        last.payment_status === 'failed'    ||
        last.payment_status === 'refunded'
      ) {
        return last;
      }
    } catch {
      // Transient network / 5xx — keep polling until the deadline.
    }
    await sleep(intervalMs);
  }

  if (last) return last;
  throw new Error('payment status poll timed out');
}

// ── helpers ───────────────────────────────────────────────────────────────────

function normaliseStart(data: any): StartPaymentResult {
  return {
    authorization_url: data?.authorization_url ?? data?.client_token ?? '',
    reference:         data?.reference ?? data?.payment_request_id ?? '',
    payment_method:    data?.payment_method ?? '',
    amount:            Number(data?.amount ?? 0),
    currency:          data?.currency ?? 'ZAR',
  };
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}