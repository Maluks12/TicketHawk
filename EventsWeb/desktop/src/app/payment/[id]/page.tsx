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
  preopenGatewayPopup,
  discardPendingPopup,
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

  // Cleanup on unmount — clear timer, abort poll, close any lingering popup.
  //
  // The teardown is DEFERRED one macrotask and cancelled if the effect re-runs.
  // React Strict Mode (on by default in `next dev`) mounts, unmounts, and
  // remounts every component in development: with a plain cleanup, that
  // simulated unmount fired popupRef.current.close() + pollAbortRef.abort()
  // a moment after the checkout popup opened — the popup flashed on screen and
  // instantly vanished, and the aborted run() left the page stuck on
  // "Complete your payment…" forever. Strict Mode remounts synchronously, so
  // the remounted effect body always runs before a 0 ms timer can fire and
  // cancels the teardown; on a REAL unmount nothing cancels it, and the popup
  // and poll are cleaned up as intended.
  const teardownTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (teardownTimerRef.current) {
      clearTimeout(teardownTimerRef.current);
      teardownTimerRef.current = null;
    }
    return () => {
      teardownTimerRef.current = setTimeout(() => {
        if (doneTimerRef.current) clearTimeout(doneTimerRef.current);
        pollAbortRef.current?.abort();
        try { popupRef.current?.close(); } catch {}
      }, 0);
    };
  }, []);

  // Instant confirmation: when a payment is initiated with channel=web, the
  // backend's return page posts a message to its opener the moment the gateway
  // redirects back (see PaymentReturnHandler). The message is only a NUDGE —
  // the status shown to the buyer always comes from our own API, so a forged
  // message can't fake a successful payment.
  useEffect(() => {
    const onMsg = (event: MessageEvent) => {
      const d = event.data as { source?: string; type?: string } | null;
      if (!d || d.source !== 'tickethawk' || d.type !== 'payment-return') return;
      fetchPaymentStatus(bookingId, guest)
        .then((s) => {
          if (s.payment_status === 'completed') {
            pollAbortRef.current?.abort();
            finishFromStatus(s, false);
          }
        })
        .catch(() => {});
    };
    window.addEventListener('message', onMsg);
    return () => window.removeEventListener('message', onMsg);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bookingId, bookingNumber, email]);

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
  const pollAbortRef = useRef<AbortController | null>(null);

  const run = useCallback(
    async (checkoutUrl: string, isRetry = false) => {
      if (!checkoutUrl) {
        setPhase('failed');
        setMessage('Missing checkout link. Please go back and try again.');
        return;
      }

      // Cancel any previous in-flight poll
      pollAbortRef.current?.abort();
      const abortCtrl = new AbortController();
      pollAbortRef.current = abortCtrl;

      setPhase('in_popup');
      setMessage('Complete your payment in the secure window…');

      // Watch the server-side status WHILE the popup is open. The backend's
      // return handler verifies with the gateway synchronously the moment the
      // popup lands back on /payments/return, so 'completed' shows up here
      // within seconds of a successful charge. When it does, close the popup
      // programmatically and show success — this rescues every case where
      // popup-close detection can't work (COOP-severed window handles, the
      // popup parked on a dead redirect page, or the buyer simply not closing
      // it). It only ever acts on 'completed': a transient 'failed' while the
      // popup is open can still be retried inside the gateway's own page, so
      // slamming the popup shut on failure would be wrong.
      let watcherFinished = false;
      const watchCtrl = new AbortController();
      abortCtrl.signal.addEventListener('abort', () => watchCtrl.abort(), { once: true });
      (async () => {
        while (!watchCtrl.signal.aborted) {
          try {
            const s = await fetchPaymentStatus(bookingId, guest);
            if (watchCtrl.signal.aborted) return;
            if (s.payment_status === 'completed') {
              watcherFinished = true;
              abortCtrl.abort(); // stop everything else in this run
              finishFromStatus(s, false);
              return;
            }
          } catch {
            // transient — keep watching
          }
          await new Promise<void>((res) => {
            const t = setTimeout(res, 3000);
            watchCtrl.signal.addEventListener('abort', () => { clearTimeout(t); res(); }, { once: true });
          });
        }
      })();

      const outcome = await openGatewayCheckout(checkoutUrl, (popup) => {
        popupRef.current = popup;
      });

      if (watcherFinished) return; // success already handled by the watcher
      watchCtrl.abort();           // popup closed — hand over to the confirm poll below

      if (outcome.type === 'blocked') {
        setMessage('Popup blocked — redirecting to payment page…');
        window.location.href = checkoutUrl;
        return;
      }

      if (abortCtrl.signal.aborted) return;

      setPhase('confirming');
      setMessage('Confirming your payment…');

      // On retry, ignore stale 'failed' status for 20 s while webhook arrives
      const ignoreFailedMs = isRetry ? 20_000 : 0;

      try {
        const status = await pollPayment(bookingId, 2500, 15_000, guest, ignoreFailedMs, abortCtrl.signal);
        if (abortCtrl.signal.aborted) return;
        finishFromStatus(status, false);
      } catch {
        if (abortCtrl.signal.aborted) return;
        try {
          const status = await fetchPaymentStatus(bookingId, guest);
          finishFromStatus(status, status.payment_status !== 'completed');
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
    preopenGatewayPopup();
    if (doneTimerRef.current) { clearTimeout(doneTimerRef.current); doneTimerRef.current = null; }
    setPhase('opening');
    setMessage('Restarting secure checkout…');
    try {
      const started = guest
        ? await retryGuestPayment(guest.bookingNumber, guest.email)
        : await retryBookingPayment(bookingId, (method as PaymentMethod) || 'card');
      // Call run directly — don't reset startedRef or the mount effect re-fires
      run(started.authorization_url, true);
    } catch {
      discardPendingPopup();
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

          {/* ── Manual check button — fallback in case `closed` detection misses ── */}
          {inPopup && (
            <button
              onClick={() => {
                pollAbortRef.current?.abort();
                const abortCtrl = new AbortController();
                pollAbortRef.current = abortCtrl;
                setPhase('confirming');
                setMessage('Confirming your payment…');
                fetchPaymentStatus(bookingId, guest)
                  .then(s => finishFromStatus(s, s.payment_status !== 'completed'))
                  .catch(() => finishFromStatus(null, true));
              }}
              style={{
                background: 'none', border: `1px solid ${border}`,
                borderRadius: 12, padding: '11px 0', fontWeight: 600,
                fontSize: 14, cursor: 'pointer', color: colors.subText,
                width: '100%',
              }}
            >
              I've closed the payment window — check status
            </button>
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