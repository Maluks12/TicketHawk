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
function popupFeatures() {
  const w    = 720;
  const h    = 700;
  const left = Math.max(0, Math.round((window.screen.width  - w) / 2));
  const top  = Math.max(0, Math.round((window.screen.height - h) / 2));
  return [
    `width=${w}`, `height=${h}`,
    `left=${left}`, `top=${top}`,
    'scrollbars=yes', 'resizable=yes',
    'toolbar=no', 'menubar=no', 'location=no', 'status=no',
  ].join(',');
}

// Holds a popup opened by preopenGatewayPopup() until openGatewayCheckout()
// claims it. Module-level (not component state) because Next.js client-side
// navigation (router.push) does not reload the JS runtime, so this survives
// the trip from the checkout/purchase page into the /payment/[id] page.
let pendingPopup: Window | null = null;

/**
 * Opens a blank placeholder popup immediately. MUST be called synchronously
 * inside the "Pay Now" click handler, before any `await`.
 *
 * Why this exists: the real checkout URL is only known after one or two
 * network round-trips (checkoutConfirm / startBookingPayment) and a
 * router.push() into /payment/[id], where openGatewayCheckout() used to call
 * window.open() for the first time. By that point several `await`s and a
 * route change separate the call from the original click, so browsers treat
 * it as not user-initiated and silently block it — intermittently, since
 * some browsers' "sticky activation" occasionally survives a short async gap
 * and sometimes doesn't. Opening the popup up front, synchronously, keeps it
 * tied to the actual click every time; openGatewayCheckout() then just points
 * this already-open window at the real URL once it has one.
 */
export function preopenGatewayPopup(): boolean {
  if (typeof window === 'undefined') return false;
  const popup = window.open('about:blank', 'eventspay_checkout', popupFeatures());
  if (!popup) return false;
  try {
    popup.document.write(
      '<!doctype html><title>Loading…</title><body style="margin:0;height:100vh;display:flex;' +
      'align-items:center;justify-content:center;font-family:sans-serif;color:#888">' +
      'Loading secure checkout…</body>',
    );
  } catch {}
  pendingPopup = popup;
  return true;
}

/** Closes and discards any popup opened by preopenGatewayPopup() that ended up unused
 *  (e.g. the booking turned out to be free, or checkout failed before reaching the gateway). */
export function discardPendingPopup() {
  try { pendingPopup?.close(); } catch {}
  pendingPopup = null;
}

export function openGatewayCheckout(
  authorizationUrl: string,
  onOpen?: (popup: Window) => void,
): Promise<CheckoutOutcome> {
  return new Promise((resolve) => {
    let popup: Window | null = null;
    let reused = false;

    // Reuse a popup opened synchronously on click, if there is one.
    if (pendingPopup) {
      popup = pendingPopup;
      pendingPopup = null;
      reused = true;
    }

    // Fallback: no pre-opened popup (e.g. page was loaded directly via a
    // shared link). This is still subject to the browser's popup blocker.
    if (!popup) {
      popup = window.open(authorizationUrl, 'eventspay_checkout', popupFeatures());
    }

    if (!popup) {
      resolve({ type: 'blocked' });
      return;
    }
    const gwPopup = popup; // narrowed, non-null alias for use inside closures below

    onOpen?.(gwPopup);

    let resolved = false;
    const done = () => {
      if (resolved) return;
      resolved = true;
      clearInterval(timer);
      try { gwPopup.close(); } catch {}
      resolve({ type: 'returned' });
    };

    // openedAt marks when we actually point the popup at the real gateway
    // URL — see the grace period below.
    let openedAt = Date.now();

    if (reused) {
      // The placeholder ("Loading secure checkout…") is already on screen.
      // Wait a frame so it's actually painted before swapping in the real,
      // cross-origin gateway URL, then give it focus.
      requestAnimationFrame(() => {
        try {
          gwPopup.location.href = authorizationUrl;
          gwPopup.focus();
        } catch {}
        openedAt = Date.now();
      });
    }

    // Detection is deliberately based on ONE signal only: whether the popup
    // is really closed. We used to also treat "the main window regained
    // focus" as a close signal, on the theory that the user had clicked back
    // here after finishing in the popup. That turned out to be too fragile —
    // exactly when a popup takes/gives back focus varies by browser and by
    // the timing of the network calls before the popup navigates — and it
    // was closing the popup out from under people while they were still
    // trying to pay. `closed` is the only signal that's actually true when
    // it fires, so it's the only one allowed to end the flow automatically.
    //
    // `closed` itself has two known quirks around the cross-origin navigation
    // from our placeholder to the real gateway URL:
    //  - it can keep reporting false for a while after a real close, and
    //  - right after the navigation it can briefly and *incorrectly* flip to
    //    true for a tick while the browser swaps the window to a new
    //    process, before settling back to false.
    // A short grace period after navigation, plus requiring two consecutive
    // positive reads before acting, filters out that transient blip without
    // meaningfully delaying detection of a real close.
    const CLOSE_CHECK_GRACE_MS = 1500;
    let consecutiveClosedReads = 0;
    const timer = setInterval(() => {
      let isClosed = false;
      try { isClosed = gwPopup.closed; } catch { isClosed = false; }

      if (!isClosed || Date.now() - openedAt <= CLOSE_CHECK_GRACE_MS) {
        consecutiveClosedReads = 0;
        return;
      }
      consecutiveClosedReads += 1;
      if (consecutiveClosedReads >= 2) done();
    }, 500);

    // If detection fails (e.g. `closed` never reliably flips true in this
    // browser once the popup has gone cross-origin), the page's own "I've
    // closed the payment window" fallback button covers that case — it
    // doesn't depend on any of the above.
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
  const res = await api.post(`/v1/bookings/${bookingId}/payment`, {}, { params: { method, channel: 'web' } });
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
    { params: { method, channel: 'web' } },
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

/**
 * startGuestPayment initialises (or re-initialises) a payment for a guest
 * booking with the chosen method (card, paypal, instant_eft).
 * Identity is verified by bookingNumber + email.
 */
export async function startGuestPayment(
  bookingNumber: string,
  email: string,
  method: PaymentMethod = 'card',
): Promise<{ authorization_url: string; reference: string; payment_method: string }> {
  const res = await api.post(
    `/v1/guest/bookings/${encodeURIComponent(bookingNumber)}/payment/start`,
    {},
    { params: { email, method, channel: 'web' } },
  );
  const data = res.data as any;
  return {
    authorization_url: data?.authorization_url ?? data?.client_token ?? '',
    reference:         data?.reference ?? data?.payment_request_id ?? '',
    payment_method:    data?.payment_method ?? method,
  };
}

/** retryGuestPayment re-initialises a guest's card payment by booking number. */
export async function retryGuestPayment(
  bookingNumber: string,
  email: string,
): Promise<StartPaymentResult> {
  const res = await api.post(
    `/v1/guest/bookings/${encodeURIComponent(bookingNumber)}/payment/retry`,
    {},
    { params: { email, channel: 'web' } },
  );
  return normaliseStart(res.data);
}

/**
 * pollPayment polls until the payment reaches a terminal state (completed /
 * failed / refunded) or the timeout elapses. Returns the last status seen.
 */
export async function pollPayment(
  bookingId      : string,
  intervalMs     = 2500,
  timeoutMs      = 150_000,
  guest?         : GuestRef,
  ignoreFailedMs = 0,
  signal?        : AbortSignal,
): Promise<PaymentStatus> {
  const deadline          = Date.now() + timeoutMs;
  const ignoreFailedUntil = Date.now() + ignoreFailedMs;
  let last: PaymentStatus | null = null;

  while (Date.now() < deadline) {
    if (signal?.aborted) break;
    try {
      last = await fetchPaymentStatus(bookingId, guest);
      const isTerminalFailed =
        (last.payment_status === 'failed' || last.payment_status === 'refunded') &&
        Date.now() >= ignoreFailedUntil;
      if (last.payment_status === 'completed' || isTerminalFailed) {
        return last;
      }
    } catch {
      // Transient network / 5xx — keep polling until the deadline.
    }
    await new Promise<void>((res) => {
      const t = setTimeout(res, intervalMs);
      signal?.addEventListener('abort', () => { clearTimeout(t); res(); }, { once: true });
    });
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