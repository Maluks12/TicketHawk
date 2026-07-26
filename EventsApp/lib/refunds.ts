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

export const fmtRand = (n: number): string =>
  `R${Math.round(n).toLocaleString('en-ZA')}`;

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