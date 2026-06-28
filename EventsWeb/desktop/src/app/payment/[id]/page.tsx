'use client';

/**
 * app/payment/[id]/page.tsx  — EventsWeb Desktop
 *
 * Drives the hosted-checkout step via a centred popup window, mirroring the
 * mobile app's expo-web-browser flow.
 *
 * URL params (pushed from purchase/page.tsx or cart/checkout/page.tsx):
 *   id            — bookingId
 *   url           — authorization_url (URL-encoded) from startBookingPayment
 *   amount        — total, for display only
 *   currency      — e.g. "ZAR"
 *   method        — card | paypal | instant_eft (display label)
 *   bookingNumber — guest only: booking number for polling via guest routes
 *   email         — guest only: email for polling via guest routes
 *
 * Backwards-compat: pages that haven't been updated yet may pass ?token= (the
 * old Stitch client_token). We fall back to it if ?url= is missing.
 *
 * Flow:
 *   1. On mount, open authorization_url in a centred popup.
 *      • If the popup is blocked → fall back to full-page redirect.
 *   2. Poll GET /v1/bookings/:id/payment every 2.5 s while the popup is open.
 *      If the poll reaches a terminal state first, the popup is closed
 *      programmatically and the result is shown immediately.
 *   3. When the popup closes (user closed it, or the gateway redirected and
 *      the browser error page was dismissed), do a brief confirmatory poll to
 *      decide: completed → success, pending → EFT state, else → cancelled.
 */

import { useState, useEffect, useCallback, useRef, use } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import {
  openGatewayCheckout,
  retryBookingPayment,
  retryGuestPayment,
  pollPayment,
  fetchPaymentStatus,
  type PaymentStatus,
  type PaymentMethod,
  type GuestRef,
} from '@/lib/payments';
import { useAppTheme } from '@/themes/colors';

type Phase =
  | 'opening'     // spinner — resolving URL / starting
  | 'in_popup'    // popup is open, waiting for user
  | 'confirming'  // popup closed, polling for status
  | 'done'        // payment completed ✅
  | 'failed'      // payment failed or unknown ⚠️
  | 'cancelled'   // user closed popup without paying ↩️
  | 'pending';    // EFT initiated, bank hasn't confirmed yet ⏳

const METHOD_LABEL: Record<string, string> = {
  card:        'Credit / Debit Card',
  paypal:      'PayPal',
  instant_eft: 'Instant EFT',
};

const METHOD_ICON: Record<string, string> = {
  card:        '💳',
  paypal:      '🅿️',
  instant_eft: '🏦',
};

export default function PaymentPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const router       = useRouter();
  const searchParams = useSearchParams();
  const { colors, isDark } = useAppTheme();
  const { id: bookingId } = use(params);

  // ── Read URL params ──────────────────────────────────────────────────────
  const rawUrl        = searchParams.get('url') ?? '';
  const amount        = searchParams.get('amount') ?? '0';
  const currency      = searchParams.get('currency') ?? 'ZAR';
  const method        = searchParams.get('method') ?? 'card';
  const bookingNumber = searchParams.get('bookingNumber') ?? '';
  const email         = searchParams.get('email') ?? '';
  // Backwards-compat: older pages pass ?token= (Stitch client_token / Paystack URL)
  const legacyToken   = searchParams.get('token') ?? '';

  const authorizationUrl = rawUrl ? decodeURIComponent(rawUrl) : legacyToken;

  const guest: GuestRef | undefined =
    bookingNumber && email ? { bookingNumber, email } : undefined;

  // ── State ────────────────────────────────────────────────────────────────
  const [phase,   setPhase]   = useState<Phase>('opening');
  const [message, setMessage] = useState('Opening secure checkout…');

  const startedRef   = useRef(false);
  const popupRef     = useRef<Window | null>(null);
  const doneTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ── Navigation helpers ───────────────────────────────────────────────────
  const goToTickets = useCallback(() => {
    if (doneTimerRef.current) { clearTimeout(doneTimerRef.current); doneTimerRef.current = null; }
    router.replace('/tickets');
  }, [router]);

  const leaveToCheckout = useCallback(() => {
    if (doneTimerRef.current) { clearTimeout(doneTimerRef.current); doneTimerRef.current = null; }
    try { router.back(); } catch { router.replace('/'); }
  }, [router]);

  // Cleanup on unmount — clear timer and close any lingering popup
  useEffect(() => () => {
    if (doneTimerRef.current) clearTimeout(doneTimerRef.current);
    try { popupRef.current?.close(); } catch {}
  }, []);

  // ── finishFromStatus ─────────────────────────────────────────────────────
  const finishFromStatus = useCallback(
    (status: PaymentStatus | null, cancelledFallback: boolean) => {
      try { popupRef.current?.close(); } catch {}

      if (status?.payment_status === 'completed') {
        setPhase('done');
        setMessage('Payment confirmed! 🎟');
        doneTimerRef.current = setTimeout(goToTickets, 1500);

      } else if (status?.payment_status === 'failed') {
        setPhase('failed');
        setMessage("Your payment didn't go through. You can try again.");

      } else if (status?.payment_status === 'pending' && !cancelledFallback) {
        // EFT and similar bank-rail methods: the transfer is initiated but the
        // bank hasn't confirmed yet. This is NOT a failure. When the bank settles,
        // Paystack's webhook completes the booking and issues tickets.
        setPhase('pending');
        setMessage(
          "Your EFT is processing. We'll email your tickets once your bank confirms — usually within a few minutes. No need to pay again.",
        );

      } else if (cancelledFallback) {
        setPhase('cancelled');
        setMessage("Payment cancelled. You can try again whenever you're ready.");

      } else {
        setPhase('failed');
        setMessage(
          "We're still confirming your payment. Your ticket may arrive shortly — check the Tickets tab.",
        );
      }
    },
    [goToTickets],
  );

  // ── Core checkout runner ─────────────────────────────────────────────────
  const run = useCallback(
    async (checkoutUrl: string) => {
      if (!checkoutUrl) {
        setPhase('failed');
        setMessage('Missing checkout link. Please go back and try again.');
        return;
      }

      setPhase('in_popup');
      setMessage('Complete your payment in the secure window…');

      const outcome = await openGatewayCheckout(checkoutUrl);

      if (outcome.type === 'blocked') {
        // Popup was blocked — redirect the current tab to the payment page.
        // The backend's /payments/return will redirect back here when done.
        setMessage('Popup blocked — redirecting to payment page…');
        window.location.href = checkoutUrl;
        return;
      }

      // Popup closed (either the gateway redirected or the user closed it).
      setPhase('confirming');
      setMessage('Confirming your payment…');

      // On web we treat both popup-close reasons identically: poll and let the
      // server tell us the real outcome. We use a brief poll (9 s) first and
      // fall through to a final direct read if it times out — same as mobile.
      try {
        const status = await pollPayment(bookingId, 2500, 150_000, guest);
        finishFromStatus(status, false);
      } catch {
        // Timed out — do one last direct read before giving up.
        try {
          const status = await fetchPaymentStatus(bookingId, guest);
          finishFromStatus(status, true);
        } catch {
          finishFromStatus(null, true);
        }
      }
    },
    [bookingId, finishFromStatus, guest],
  );

  // Mount once
  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    run(authorizationUrl);
  }, [run, authorizationUrl]);

  // ── Retry ────────────────────────────────────────────────────────────────
  const retry = useCallback(async () => {
    setPhase('opening');
    setMessage('Restarting secure checkout…');
    try {
      const started = guest
        ? await retryGuestPayment(guest.bookingNumber, guest.email)
        : await retryBookingPayment(bookingId, (method as PaymentMethod) || 'card');
      startedRef.current = false; // allow run() to fire again
      run(started.authorization_url);
    } catch {
      setPhase('failed');
      setMessage("Couldn't restart checkout. Please go back and start over.");
    }
  }, [bookingId, method, guest, run]);

  // ── Helpers ──────────────────────────────────────────────────────────────
  const fmtAmount = () => {
    const n = parseFloat(amount);
    return Number.isNaN(n)
      ? (currency || 'ZAR')
      : `${currency || 'ZAR'} ${Math.round(n).toLocaleString('en-ZA')}`;
  };

  const methodLabel = METHOD_LABEL[method] ?? 'Card';
  const methodIcon  = METHOD_ICON[method]  ?? '💳';
  const spinning    = phase === 'opening' || phase === 'confirming';
  const inPopup     = phase === 'in_popup';
  const canRetry    = phase === 'failed' || phase === 'cancelled';

  const bg     = isDark ? '#0a0a0a' : '#f5f5f7';
  const cardBg = isDark ? '#111'    : '#fff';
  const border = isDark ? '#222'    : '#e5e5e5';

  // ── Render ───────────────────────────────────────────────────────────────
  return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', flexDirection: 'column' }}>
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>

      {/* ── Header ── */}
      <div style={{
        display: 'flex', alignItems: 'center',
        padding: '14px 24px',
        borderBottom: `1px solid ${border}`,
        background: cardBg,
      }}>
        <div style={{ flex: 1 }}>
          <p style={{ margin: 0, fontSize: 16, fontWeight: 700, color: colors.text }}>
            {methodIcon} Pay with {methodLabel}
          </p>
        </div>
        <p style={{ margin: 0, fontSize: 14, color: colors.subText }}>{fmtAmount()}</p>
      </div>

      {/* ── Body ── */}
      <div style={{
        flex: 1, display: 'flex',
        alignItems: 'center', justifyContent: 'center',
        padding: '32px 16px',
      }}>
        <div style={{
          background: cardBg,
          border: `1px solid ${border}`,
          borderRadius: 20,
          padding: '48px 52px',
          maxWidth: 480, width: '100%',
          textAlign: 'center',
          display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 22,
        }}>

          {/* ── Spinner / Icon ── */}
          {spinning && (
            <div style={{
              width: 52, height: 52,
              border: '3px solid #ff3c6e', borderTopColor: 'transparent',
              borderRadius: '50%', animation: 'spin 0.8s linear infinite',
            }} />
          )}
          {inPopup    && <span style={{ fontSize: 56 }}>🔐</span>}
          {phase === 'done'      && <span style={{ fontSize: 56 }}>✅</span>}
          {phase === 'failed'    && <span style={{ fontSize: 56 }}>⚠️</span>}
          {phase === 'cancelled' && <span style={{ fontSize: 56 }}>↩️</span>}
          {phase === 'pending'   && <span style={{ fontSize: 56 }}>⏳</span>}

          {/* ── Message ── */}
          <p style={{
            margin: 0, fontSize: 16, lineHeight: 1.55,
            color: phase === 'done' ? '#00a843' : colors.text,
            fontWeight: phase === 'done' ? 700 : 400,
          }}>
            {message}
          </p>

          {/* ── In-popup hint ── */}
          {inPopup && (
            <div style={{
              background: isDark ? '#1a1a1a' : '#f9f9fb',
              border: `1px solid ${border}`,
              borderRadius: 12, padding: '14px 16px',
              fontSize: 13, color: colors.subText, textAlign: 'left',
              lineHeight: 1.6,
            }}>
              <strong style={{ color: colors.text }}>A secure payment window has opened.</strong>
              <br />
              Complete your {methodLabel} payment there — this page will update
              automatically once your payment is confirmed. Do not close this tab.
            </div>
          )}

          {/* ── Paystack / PayPal badge ── */}
          {inPopup && (
            <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
              <span style={{ fontSize: 12, color: colors.subText }}>Powered by</span>
              {method === 'paypal' ? (
                <span style={{ fontSize: 13, fontWeight: 700, color: '#003087' }}>PayPal</span>
              ) : (
                <span style={{ fontSize: 13, fontWeight: 700, color: '#00c3f7' }}>Paystack</span>
              )}
              <span style={{ fontSize: 12, color: colors.subText }}>· 🔒 SSL encrypted</span>
            </div>
          )}

          {/* ── Pending EFT ── */}
          {phase === 'pending' && (
            <button
              onClick={goToTickets}
              style={{
                background: '#ff3c6e', color: '#fff', border: 'none',
                borderRadius: 12, padding: '13px 0', fontWeight: 700,
                fontSize: 15, cursor: 'pointer', width: '100%',
              }}
            >
              Go to my tickets
            </button>
          )}

          {/* ── Retry flow ── */}
          {canRetry && (
            <>
              <button
                onClick={retry}
                style={{
                  background: '#ff3c6e', color: '#fff', border: 'none',
                  borderRadius: 12, padding: '13px 0', fontWeight: 700,
                  fontSize: 15, cursor: 'pointer', width: '100%',
                }}
              >
                Try again
              </button>
              <button
                onClick={leaveToCheckout}
                style={{
                  background: 'none',
                  border: `1px solid ${isDark ? '#444' : '#1a1a2e'}`,
                  borderRadius: 12, padding: '12px 0', fontWeight: 600,
                  fontSize: 15, cursor: 'pointer',
                  color: colors.text, width: '100%',
                }}
              >
                Back to checkout
              </button>
              <button
                onClick={goToTickets}
                style={{
                  background: 'none', border: 'none', padding: '8px',
                  fontSize: 14, color: colors.subText, cursor: 'pointer',
                  textDecoration: 'underline',
                }}
              >
                Check Tickets tab instead
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}