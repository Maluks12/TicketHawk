'use client';
// ── Paystack inline checkout (InlineJS v2, resumeTransaction) ─────────────────
// Completes a payment on our own page instead of redirecting to
// checkout.paystack.com. Apple Pay appears inside the sheet on Safari and Apple
// hardware; card is there for everyone else.
//
// ── Why resumeTransaction and not paymentRequest ─────────────────────────────
//
// paymentRequest() mounts a standalone Apple Pay button, which looks better —
// but its options are the GENERAL transaction options, where key, amount and
// email are all required, and it has no accessCode parameter. That means the
// browser creates the transaction, so amount, subaccountCode and
// transactionCharge all become client-supplied and client-editable.
//
// For a deposit product that is harmless: tamper the amount and you get less
// credit. For tickets it is not, because amount and value are decoupled — pay R1
// and receive a R400 admission. It would also bypass createPaymentRecord
// entirely, so webhooks would arrive bearing references the backend has never
// seen, with nothing to reconcile against.
//
// resumeTransaction takes an access code for a transaction our server already
// created, with the split already attached. Nothing about the money is
// decidable from the browser. Paystack confirmed (DevRel, Jul 2026) that the
// access code retains the full configuration including split details.
//
// The cost is one extra tap: Apple Pay is a channel inside Paystack's sheet
// rather than a button on our page. That is the trade, and it is the right way
// round for a platform holding other people's money.
//
// NOTE: the transaction must have been initialised with apple_pay in its
// channels list, or the sheet won't offer it. See models.PaystackChannels.
import { useCallback, useEffect, useRef, useState } from 'react';

const INLINE_JS = 'https://js.paystack.co/v2/inline.js';
const SCRIPT_ID = 'paystack-inline-v2';

interface PaystackPopInstance {
  resumeTransaction: (accessCode: string, opts?: Record<string, unknown>) => unknown;
}

/** Load InlineJS v2 once, shared across mounts. */
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
      existing.addEventListener('error', () => reject(new Error('inline.js failed to load')));
      return;
    }
    const el = document.createElement('script');
    el.id = SCRIPT_ID;
    el.src = INLINE_JS;
    el.async = true;
    el.onload = () => resolve();
    el.onerror = () => reject(new Error('inline.js failed to load'));
    document.head.appendChild(el);
  });

  return w.__psInlinePromise;
}

export interface InlineCheckoutHandle {
  /** Opens the sheet. Must be called from a user gesture. */
  open: () => void;
  /** False until InlineJS has loaded, so the caller can disable its button. */
  ready: boolean;
}

interface Props {
  accessCode: string;
  /** Fired after Paystack reports success. Verify server-side regardless. */
  onSuccess: (reference: string) => void;
  onCancel?: () => void;
  /** Called when the sheet cannot be used at all, so the caller can redirect. */
  onUnavailable?: (reason: string) => void;
  children: (handle: InlineCheckoutHandle) => React.ReactNode;
}

export default function InlineCheckout({
  accessCode,
  onSuccess,
  onCancel,
  onUnavailable,
  children,
}: Props) {
  const [ready, setReady] = useState(false);
  const popRef = useRef<PaystackPopInstance | null>(null);
  // Paystack's own StrictMode-safe guard: loading twice is harmless, but
  // constructing two popups and resuming the same access code on both is not.
  const loading = useRef(false);

  useEffect(() => {
    if (loading.current || !accessCode) return;
    loading.current = true;

    let cancelled = false;
    (async () => {
      try {
        await loadInlineJS();
        if (cancelled) return;
        const w = window as unknown as { PaystackPop: new () => PaystackPopInstance };
        popRef.current = new w.PaystackPop();
        setReady(true);
      } catch (e) {
        if (!cancelled) onUnavailable?.((e as Error).message || 'inline checkout unavailable');
      }
    })();

    return () => { cancelled = true; };
  }, [accessCode, onUnavailable]);

  const open = useCallback(() => {
    const pop = popRef.current;
    if (!pop) {
      onUnavailable?.('inline checkout not ready');
      return;
    }
    try {
      pop.resumeTransaction(accessCode, {
        onSuccess: (t: { reference?: string } | undefined) => {
          // Not proof of payment — the webhook is authoritative. The caller
          // routes somewhere that reads server-side status.
          onSuccess(t?.reference ?? '');
        },
        onCancel: () => onCancel?.(),
        onError: (err: { message?: string } | undefined) => {
          onUnavailable?.(err?.message ?? 'checkout error');
        },
      });
    } catch (e) {
      onUnavailable?.((e as Error).message || 'could not open checkout');
    }
  }, [accessCode, onSuccess, onCancel, onUnavailable]);

  return <>{children({ open, ready })}</>;
}