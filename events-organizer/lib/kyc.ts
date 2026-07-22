// ── Tiered KYC client ─────────────────────────────────────────────────────────
// Tier 0: nothing        → onboarding gate (cannot create events)
// Tier 1: basic identity → private free events (name + SA ID, instant, free)
// Tier 2: full identity  → paid + public events (ID photo + VerifyNow / admin)
import { api } from './api';

export interface KycStatus {
  kyc_level: number;
  // Raw profile block from the backend — shape owned by the API; we read the
  // status defensively since it nests differently across states.
  kyc?: Record<string, unknown> | null;
}

/** GET /organiser/kyc — 404 means "no organizer profile yet" = tier 0. */
export async function getKycStatus(): Promise<KycStatus> {
  try {
    const { data } = await api.get('/organiser/kyc');
    return { kyc_level: data.kyc_level ?? 0, kyc: data.kyc ?? null };
  } catch (e: unknown) {
    const status = (e as { response?: { status?: number } })?.response?.status;
    // 404 = no organiser profile yet; 401 = token not settled on this call.
    // Either way, treat as level 0 rather than throwing — the gate stays
    // functional and simply routes to onboarding.
    if (status === 404 || status === 401) return { kyc_level: 0, kyc: null };
    throw e;
  }
}

/** Pull a displayable KYC status string out of the profile block. */
export function readKycState(s: KycStatus): string {
  const k = s.kyc as Record<string, unknown> | null;
  if (!k) return 'none';
  return String(k.kyc_status ?? k.status ?? 'none');
}

export interface BasicKycDTO {
  legal_first_name: string;
  legal_last_name: string;
  sa_id_number: string;
}

/** POST /organiser/kyc/basic — tier 1: creates the profile, grants the role. */
export function submitBasicKyc(dto: BasicKycDTO) {
  return api.post('/organiser/kyc/basic', dto);
}

/** POST /organiser/kyc — tier 2: multipart with the ID document. */
export function submitFullKyc(form: FormData) {
  return api.post('/organiser/kyc', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
}

/** Light client-side SA ID sanity check (server does the full validation). */
export function looksLikeSaId(id: string): boolean {
  return /^\d{13}$/.test(id.trim());
}