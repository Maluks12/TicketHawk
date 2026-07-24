// ── Organiser payout (bank) details client ────────────────────────────────────
// Paid events are gated on TWO things server-side, not one: full KYC (tier 2)
// AND payout bank details on file. PublishEvent returns ErrPayoutDetailsRequired
// when the bank half is missing. These endpoints are the only way to satisfy
// it, and they were unreachable from the dashboard while the old /payouts page
// (an ad-billing surface) stayed retired — so the bank form now lives on the
// full-verification screen, where the organiser is already standing when they
// hit the paid-event gate.
import { api } from './api';

export interface Bank {
  name: string;
  code: string;
  slug: string;
}

export interface PayoutProfile {
  organizer_id: string;
  business_name: string;
  payout_tier: 'held' | 'instant';
  payout_enabled: boolean;
  has_subaccount: boolean;
  settlement_bank_name?: string | null;
  bank_account_name?: string | null;
  bank_account_last_4?: string | null;
}

export interface SetPayoutDetailsDTO {
  bank_code: string;
  bank_name: string;
  account_number: string;
  account_name: string;
  account_type: string;
}

/** GET /organiser/payouts/banks — ZA settlement banks, cached 24h server-side. */
export async function getBanks(): Promise<Bank[]> {
  const { data } = await api.get('/organiser/payouts/banks');
  return data.banks ?? [];
}

/** GET /organiser/payouts/profile — current bank setup, if any. */
export async function getPayoutProfile(): Promise<PayoutProfile | null> {
  try {
    const { data } = await api.get('/organiser/payouts/profile');
    return data.profile ?? null;
  } catch {
    // No organiser row yet / transient failure — the form still renders.
    return null;
  }
}

/** POST /organiser/payouts/details — creates the Paystack subaccount + recipient. */
export async function setPayoutDetails(dto: SetPayoutDetailsDTO) {
  return api.post('/organiser/payouts/details', dto);
}

/** SA account numbers are 6–20 digits; the server enforces the same bounds. */
export function looksLikeAccountNumber(n: string): boolean {
  return /^\d{6,20}$/.test(n.trim());
}