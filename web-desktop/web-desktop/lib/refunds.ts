// ─────────────────────────────────────────────────────────────────────────────
// PORTED FROM EventsApp/lib/refunds.ts — keep the three copies in sync.
//
// The web apps previously had NO refund quote at all. booking/[id]/page.tsx
// decided eligibility with:
//
//     const canRefund = status === 'confirmed' && !refunded && !!user;
//
// which showed a refund button for every confirmed booking regardless of how
// close the event was. A buyer three days out saw the button, tapped it, and
// got a 400 from the server — a visible-but-dead refund button, which is the
// single worst version of this feature: it promises money the server will
// refuse, and the buyer's next move is a support ticket.
//
// The app was rebuilt against GET /refund-quote months ago. The two web apps
// were not. This closes that gap.
// ─────────────────────────────────────────────────────────────────────────────
// ─────────────────────────────────────────────────────────────────────────────
// Buyer refund quotes.
//
// The refund amount is decided by the SERVER, from events-db/internal/models/
// refund_policy.go, keyed off how far away the event still is at the moment the
// refund is requested:
//
//     30 days or more ....... 100% of subtotal
//     21 – 29 days ..........  75%
//     14 – 20 days ..........  50%
//      7 – 13 days ..........  25%
//     under 7 days ..........   0%  — not refundable
//
// The buyer service fee (bookings.platform_fee) is NEVER returned on a
// buyer-initiated refund. An organiser-initiated cancellation is a different
// path entirely and refunds 100% including the fee.
//
// ── Why the app never computes the amount itself ──────────────────────────────
//
// It would be trivial to reimplement the schedule here from event.start_time,
// and it would be wrong. The amount shown to the buyer before they confirm has
// to be the amount they are actually paid, and only the server knows the
// booking's true subtotal and platform_fee (both of which the booking-detail
// payload can omit for older bookings). Quoting locally would also silently
// drift the day a band changes on the backend, so the buyer would be promised
// one figure and paid another. So: always ask, never guess.
//
// GET /v1/bookings/:id/refund-quote                            (authenticated)
// GET /v1/guest/bookings/:bookingNumber/refund-quote?email=…   (guest)
// ─────────────────────────────────────────────────────────────────────────────

import { api } from './api';

/** Mirrors models.RefundQuote. */
export interface RefundQuote {
  eligible:         boolean;
  rate:             number;  // fraction of subtotal, 0.0–1.0
  days_until_event: number;  // whole days, floored
  refund_amount:    number;  // what actually lands back with the buyer
  service_fee_kept: number;  // non-refundable
  forfeited:        number;  // subtotal the buyer does not get back
  reason?:          string;  // set when eligible is false
}

export interface RefundQuoteResponse {
  booking_number: string;
  total_paid:     number;
  quote:          RefundQuote;
}

export interface GuestRef {
  bookingNumber: string;
  email:         string;
}

/**
 * Fetches the live refund quote for a booking.
 *
 * Returns null rather than throwing: the quote is decoration on a screen whose
 * primary job is showing a working ticket, so a failed quote must degrade to
 * "no refund button" and never to an error state or a crash.
 */
export async function getRefundQuote(
  bookingId: string,
  guest?: GuestRef,
): Promise<RefundQuoteResponse | null> {
  try {
    if (guest) {
      const { data } = await api.get<RefundQuoteResponse>(
        `/v1/guest/bookings/${encodeURIComponent(guest.bookingNumber)}/refund-quote`,
        { params: { email: guest.email } },
      );
      return data ?? null;
    }
    const { data } = await api.get<RefundQuoteResponse>(`/v1/bookings/${bookingId}/refund-quote`);
    return data ?? null;
  } catch {
    return null;
  }
}

/** Submits the refund. Throws on failure so the caller can surface the reason. */
export async function submitRefund(
  bookingId: string,
  reason: string,
  guest?: GuestRef,
): Promise<void> {
  if (guest) {
    await api.post(
      `/v1/guest/bookings/${encodeURIComponent(guest.bookingNumber)}/refund`,
      { reason },
      { params: { email: guest.email } },
    );
    return;
  }
  await api.post(`/v1/bookings/${bookingId}/refund`, { reason });
}

// ─── Presentation helpers ────────────────────────────────────────────────────

/**
 * Money for display. Shows cents whenever the amount has them.
 *
 * This used to Math.round(), which quietly overstated refunds: a 75% quote on
 * a R10 ticket is R7.50 and rendered as "R8". The buyer is promised a number
 * on the button face and then receives 50c less — small in rand, but it is a
 * broken promise about money, and the kind of discrepancy that produces a
 * support ticket rather than a shrug. The server's quote is authoritative and
 * carries cents (see models.roundRand), so the display must too.
 *
 * Whole amounts still render clean — R10, not R10.00 — so the common case is
 * unchanged.
 */
export const fmtRand = (n: number): string => {
  const hasCents = Math.abs(n * 100 - Math.round(n * 100)) < 0.5
    ? Math.round(n * 100) % 100 !== 0
    : true;
  return `R${n.toLocaleString('en-ZA', {
    minimumFractionDigits: hasCents ? 2 : 0,
    maximumFractionDigits: 2,
  })}`;
};

/** "100%", "75%" — the band the buyer currently falls in. */
export const fmtRate = (rate: number): string => `${Math.round(rate * 100)}%`;

/**
 * Human-readable window, used both on the button and in the sheet.
 * Deliberately phrased around the DEADLINE rather than the band, because the
 * only thing a buyer can act on is "decide before X".
 */
export function refundWindowLabel(days: number): string {
  if (days <= 0)  return 'Event has started';
  if (days === 1) return '1 day until the event';
  return `${days} days until the event`;
}

/**
 * How many days remain in the buyer's CURRENT band before the rate drops.
 * Bands are 30 / 21 / 14 / 7, so a buyer 23 days out has 2 days left at 75%.
 * Returns null when there is no lower band to fall into (already at 0%).
 */
export function daysLeftInBand(days: number): number | null {
  const thresholds = [30, 21, 14, 7];
  for (const t of thresholds) {
    if (days >= t) return days - t + 1;
  }
  return null;
}