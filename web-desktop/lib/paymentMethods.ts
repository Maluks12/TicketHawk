/**
 * Which payment methods the checkout is allowed to offer.
 *
 * ── Why this exists ──────────────────────────────────────────────────────────
 *
 * Paystack enables payment channels per-account, and NOT identically between
 * test and live mode. Instant EFT (Ozow) and Capitec Pay both require a
 * separate KYC review on the live account — typically 2–3 business days after
 * you enable them under Settings → Preferences → Accept payments via.
 *
 * Until that review clears, `/transaction/initialize` with channel "eft"
 * fails. And because the booking row is created BEFORE payment initialisation,
 * every failed attempt used to hold its tickets until the expiry worker
 * released them. Offering a method the account cannot process therefore did not
 * just annoy the buyer, it silently burned inventory.
 *
 * So: never offer a channel that is not live on the Paystack account. Flip the
 * flag here once Paystack confirms activation — one line, both checkout
 * surfaces, no other code changes.
 *
 * ── Checking before you flip ─────────────────────────────────────────────────
 *
 * Paystack Dashboard → Settings → Preferences → Accept payments via. EFT must
 * read as active, not pending review. A test-mode success proves nothing about
 * live; the two are reviewed separately.
 *
 * Note also that Capitec customers cannot pay through the Ozow EFT channel —
 * they need Capitec Pay, which is a distinct method and a separate activation.
 */

export type PaymentMethodId = 'card' | 'instant_eft';

/**
 * Set INSTANT_EFT to true only once Paystack has activated EFT on the LIVE
 * account. Leaving it true while the channel is inactive breaks checkout for
 * anyone who selects it.
 */
export const PAYMENT_METHODS_ENABLED: Record<PaymentMethodId, boolean> = {
  card: true,
  instant_eft: false, // ← awaiting Paystack live-mode EFT approval
};

/** True when a method may be shown in the picker. */
export function isPaymentMethodEnabled(id: PaymentMethodId): boolean {
  return PAYMENT_METHODS_ENABLED[id] === true;
}

/**
 * Filters a picker's option list down to the methods that are actually
 * available. Keeps the option objects intact so each surface can carry its own
 * icons, labels and styling.
 */
export function enabledPaymentMethods<T extends { id: string }>(options: readonly T[]): T[] {
  return options.filter((o) => isPaymentMethodEnabled(o.id as PaymentMethodId));
}

/**
 * The method to select by default. Falls back to the first enabled option so
 * disabling card (unlikely, but possible during an outage) cannot leave the
 * checkout with nothing selected.
 */
export function defaultPaymentMethod(): PaymentMethodId {
  if (isPaymentMethodEnabled('card')) return 'card';
  const first = (Object.keys(PAYMENT_METHODS_ENABLED) as PaymentMethodId[])
    .find(isPaymentMethodEnabled);
  return first ?? 'card';
}