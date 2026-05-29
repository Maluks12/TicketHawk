import { api } from './api';

// ── Shared types ───────────────────────────────────────────────────────────────
export type CampaignStatus = 'active' | 'paused' | 'exhausted' | 'ended' | 'payment_failed';
export type AccountStatus  = 'active' | 'disabled';

export interface AdAccount {
  id: string;
  name: string;
  status: AccountStatus;
  card_last4: string | null;
  card_brand: string | null;
  card_expiry: string | null;
  has_payment_method: boolean;
  total_spent: number;
  active_campaigns: number;
  currency: string;
  created_at: string;
}

export interface Campaign {
  id: string;
  event_id: string;
  ad_account_id: string | null;
  daily_budget: number;
  total_spent: number;
  today_spent: number;
  total_impressions: number;
  today_impressions: number;
  total_clicks: number;
  today_clicks: number;
  start_date: string;
  end_date: string | null;
  status: CampaignStatus;
  created_at: string;
  event?: { title: string };
}

export interface BillingRecord {
  id: string;
  billing_date: string;
  amount: number;
  status: string;
  failure_reason: string | null;
  completed_at: string | null;
}

// ── DTOs ──────────────────────────────────────────────────────────────────────
export interface CreateAdAccountDTO {
  name: string;
}

export interface RegisterPaymentMethodDTO {
  card_name: string;
  card_last4: string;
  card_brand: string;
  card_expiry: string; // MM/YY
  email: string;
}

export interface CreateCampaignDTO {
  ad_account_id: string;
  daily_budget: number;
  end_date?: string;
}

// ── Ad accounts ───────────────────────────────────────────────────────────────
export const getAdAccounts = (): Promise<AdAccount[]> =>
  api.get('/organiser/ad-accounts').then(r => r.data.accounts ?? []);

export const getAdAccount = (id: string): Promise<AdAccount> =>
  api.get(`/organiser/ad-accounts/${id}`).then(r => r.data);

export const createAdAccount = (dto: CreateAdAccountDTO): Promise<AdAccount> =>
  api.post('/organiser/ad-accounts', dto).then(r => r.data.account);

export const updateAdAccount = (
  id: string,
  dto: Partial<{ name: string; status: AccountStatus }>,
): Promise<AdAccount> =>
  api.patch(`/organiser/ad-accounts/${id}`, dto).then(r => r.data.account);

// ── Payment methods ───────────────────────────────────────────────────────────

// Only safe display fields are sent — raw card number and CVV never leave
// the browser (they are used client-side only to derive last4 / brand).
export const registerPaymentMethod = (
  accountId: string,
  dto: RegisterPaymentMethodDTO,
): Promise<void> =>
  api
    .post(`/organiser/ad-accounts/${accountId}/payment-method`, dto)
    .then(() => undefined);

// ── Campaigns ─────────────────────────────────────────────────────────────────
export const getCampaigns = (): Promise<Campaign[]> =>
  api.get('/organiser/campaigns').then(r => r.data.campaigns ?? []);

export const createCampaign = (eventId: string, dto: CreateCampaignDTO): Promise<Campaign> =>
  api.post(`/organiser/events/${eventId}/campaigns`, dto).then(r => r.data.campaign);

export const pauseCampaign = (id: string): Promise<void> =>
  api.patch(`/organiser/campaigns/${id}`, { status: 'paused' }).then(() => undefined);

export const resumeCampaign = (id: string): Promise<void> =>
  api.patch(`/organiser/campaigns/${id}`, { status: 'active' }).then(() => undefined);

export const endCampaign = (id: string): Promise<void> =>
  api.patch(`/organiser/campaigns/${id}`, { status: 'ended' }).then(() => undefined);

// ── Billing ───────────────────────────────────────────────────────────────────
export const getBillingRecords = (): Promise<BillingRecord[]> =>
  api.get('/organiser/ad-billing').then(r => r.data.records ?? []);