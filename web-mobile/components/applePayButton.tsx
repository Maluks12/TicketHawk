'use client';
// ── Apple Pay button (Paystack InlineJS v2) ──────────────────────────────────
// Mounts a real Apple Pay button on our own page. Tapping it raises the iOS /
// macOS system sheet — card, amount, Face ID or double-click the side button —
// and the buyer never leaves ticket-hawk.com.
//
// ── The one rule this file exists to enforce ─────────────────────────────────
//
// EVERY payment here is completed against a transaction that our SERVER already
// initialised, identified only by its access code. We never hand Paystack a raw
// amount and email from the browser.
//
// That is not a style preference. Instant-tier organisers are paid through a
// Paystack subaccount split that payment_service.go attaches server-side
// (subaccount + transaction_charge). Paystack's DevRel team confirmed in July
// 2026 that an access code retains that configuration, so a payment made
// against it preserves the split. A client-initialised transaction would not:
// it would succeed, look identical in every log, and quietly route the
// organiser's entire share to the platform account. We have already spent a day
// tracing one instance of that class of bug — it is invisible until an organiser
// asks where their money is.
//
// Hence: no `amount`, no `email`, no `key` fallback anywhere below. If the
// access code is missing we render nothing and let the existing redirect flow
// handle the sale. Failing closed costs one Apple Pay conversion; failing open
// costs an organiser their revenue.
//
// ── Availability ─────────────────────────────────────────────────────────────
//
// Apple Pay is Safari and Apple hardware only, so this renders nothing
// everywhere else — `onElementsMount` tells us whether Paystack actually
// mounted an Apple Pay element, which is more reliable than sniffing the user
// agent ourselves. The normal card flow always remains as the fallback.
//
// Minimum ZAR transaction for Apple Pay is R1.00 (Paystack, Jul 2026).
import { useEffect, useRef, useState } from 'react';

const INLINE_JS = 'https://js.paystack.co/v2/inline.js';
const SCRIPT_ID = 'paystack-inline-v2';
/** Apple Pay will reject anything under this; below it we don't even offer it. */
const MIN_ZAR = 1;

interface PaystackPopInstance {
  paymentRequest: (opts: Record<string, unknown>) => Promise<unknown>;
  resumeTransaction: (accessCode: string, opts?: Record<string, unknown>) => unknown;
}

/** Load InlineJS v2 once, shared across every mount. */
function loadInlineJS(): Promise<void> {
  if (typeof window === 'undefined') return Promise.reject(new Error('no window'));

  const w = window as unknown as {
    PaystackPop?: new () => PaystackPopInstance;
    __psInlinePromise?: Promise<void>;
  };

  if (w.PaystackPop) return Promise.resolve();
  if (w.__psInlinePromise) return w.__psInlinePromise;

  w.__psInlinePromise = new Promise<void>((resolve, reject) => {
    const existing = document.getElementById(SCRIPT_ID) as HTMLScriptElement | null;
    if (existing) {
      existing.addEventListener('load', () => resolve());
      existing.addEventListener('error', () => reject(new Error('inline.js failed')));
      return;
    }
    const el = document.createElement('script');
    el.id = SCRIPT_ID;
    el.src = INLINE_JS;
    el.async = true;
    el.onload = () => resolve();
    el.onerror = () => reject(new Error('inline.js failed'));
    document.head.appendChild(el);
  });

  return w.__psInlinePromise;
}

interface Props {
  /** Access code from our server's payment intent. Required — see header. */
  accessCode: string;
  /** Total in rand, used only to skip rendering below Apple Pay's floor. */
  amount: number;
  /** Paystack reference, passed back so the caller can verify server-side. */
  reference: string;
  /** Fired after Paystack reports success. MUST verify on the server. */
  onSuccess: (reference: string) => void;
  onCancel?: () => void;
  /** Called when Apple Pay isn't available, so the caller can show card UI. */
  onUnavailable?: () => void;
}

export default function ApplePayButton({
  accessCode,
  amount,
  reference,
  onSuccess,
  onCancel,
  onUnavailable,
}: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [mounted, setMounted] = useState(false);
  const [busy, setBusy] = useState(false);
  // Paystack mounts into a real DOM node, so guard against StrictMode's double
  // effect — the popup previously appeared and vanished for exactly this
  // reason, and mounting the button twice would leave a duplicate behind.
  const initialised = useRef(false);

  useEffect(() => {
    if (initialised.current) return;
    if (!accessCode || amount < MIN_ZAR) {
      onUnavailable?.();
      return;
    }
    initialised.current = true;

    let cancelled = false;

    (async () => {
      try {
        await loadInlineJS();
        if (cancelled || !containerRef.current) return;

        const w = window as unknown as { PaystackPop: new () => PaystackPopInstance };
        const pop = new w.PaystackPop();

        await pop.paymentRequest({
          // accessCode ONLY. No amount, no email, no key — see the header note.
          accessCode,
          container: containerRef.current.id,
          styles: {
            applePay: { width: '100%', height: '50px', borderRadius: '10px', type: 'buy', locale: 'en' },
          },
          onElementsMount: (elements: { applePay?: unknown } | null) => {
            if (cancelled) return;
            if (elements && elements.applePay) {
              setMounted(true);
            } else {
              // Not an Apple device or Apple Pay unconfigured — stay hidden.
              onUnavailable?.();
            }
          },
          onSuccess: () => {
            if (cancelled) return;
            setBusy(false);
            onSuccess(reference);
          },
          onCancel: () => {
            if (cancelled) return;
            setBusy(false);
            onCancel?.();
          },
          onError: () => {
            if (cancelled) return;
            setBusy(false);
            onUnavailable?.();
          },
        });
      } catch {
        if (!cancelled) onUnavailable?.();
      }
    })();

    return () => { cancelled = true; };
  }, [accessCode, amount, reference, onSuccess, onCancel, onUnavailable]);

  return (
    <div style={{ display: mounted ? 'block' : 'none' }}>
      <div
        id="apple-pay-container"
        ref={containerRef}
        aria-busy={busy}
        style={{ width: '100%', minHeight: 50 }}
      />
      <p style={{
        margin: '8px 0 0', fontSize: 12, color: '#8a8d91', textAlign: 'center',
      }}>
        Pay instantly with Face ID or Touch ID
      </p>
    </div>
  );
}