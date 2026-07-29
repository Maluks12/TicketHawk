// ─────────────────────────────────────────────────────────────────────────────
// Organiser Studio — API layer
//
// Read-only views into the organiser dashboard (stats, events, campaigns) plus
// the live ticket-scanning endpoint. Mirrors the events-organizer web client but
// scoped to what a mobile organiser needs on the door: see their numbers and
// admit attendees. No create / edit / publish / cancel calls live here on
// purpose — the Studio is a window onto the data, not a control panel.
//
// All routes are prefixed with /v1 to match the rest of the mobile API surface.
// ─────────────────────────────────────────────────────────────────────────────
import { api } from './api';

// ── Stats ─────────────────────────────────────────────────────────────────────
// Shape of GET /v1/organiser/dashboard/stats (repository.OrganizerStats).
export interface OrgStats {
  total_events: number;
  published_events: number;
  pending_review_events: number;
  rejected_events: number;
  draft_events: number;
  cancelled_events: number;

  total_bookings: number;
  total_tickets_sold: number;
  gross_ticket_revenue: number;
  platform_fees_paid: number;
  net_ticket_earnings: number;

  bookings_today: number;
  revenue_today: number;

  merch_units_sold: number;
  gross_merch_revenue: number;

  total_net_earnings: number;
}

// ── Event rows ────────────────────────────────────────────────────────────────
// Shape of each row in GET /v1/organiser/dashboard/events.
export interface OrgEventRow {
  id: string;
  slug: string;
  title: string;
  status: string;
  start_time: string;
  city: string;
  price: number;
  currency: string;
  max_capacity?: number | null;
  tickets_sold: number;

  total_bookings: number;
  gross_ticket_revenue: number;
  net_ticket_earnings: number;

  merch_units_sold: number;
  gross_merch_revenue: number;

  total_net_earnings: number;
  created_at: string;
}

// ── Campaigns ─────────────────────────────────────────────────────────────────
// Shape of each row in GET /v1/organiser/campaigns (models.AdCampaign).
export type CampaignStatus =
  | 'active' | 'paused' | 'exhausted' | 'ended' | 'payment_failed';

export interface OrgCampaign {
  id: string;
  event_id: string;
  ad_account_id?: string | null;
  daily_budget: number;
  cpm_rate: number;
  total_spent: number;
  total_impressions: number;
  total_clicks: number;
  today_spent: number;
  today_impressions: number;
  today_clicks: number;
  start_date: string;
  end_date?: string | null;
  status: CampaignStatus;
  created_at: string;
  updated_at: string;
  event?: { title?: string } | null;
}

// ── Single-event stats ────────────────────────────────────────────────────────
export interface OrgEventStats {
  event: {
    id: string;
    title: string;
    status: string;
    start_time: string;
    end_time: string;
    city: string;
    venue_name: string;
    price: number;
    currency: string;
    tickets_sold: number;
    max_capacity: number | null;
    views: number;
    cover_image_url: string | null;
  };
  total_revenue: number;
  total_bookings: number;
  confirmed_bookings: number;
  pending_bookings: number;
}

export interface OrgBooking {
  id: string;
  booking_number: string;
  ticket_quantity: number;
  subtotal: number;
  platform_fee: number;
  net_earning: number;
  currency: string;
  booking_status: string;
  attendee_name: string;
  confirmed_at: string | null;
}

export interface OrgMerchSale {
  merchandise_id: string;
  merch_name: string;
  variant_id: string | null;
  variant_sku: string | null;
  attributes: string | null;
  units_sold: number;
  unit_price: number;
  gross_revenue: number;
  oversold_quantity: number;
}

// ── Scan result ───────────────────────────────────────────────────────────────
export interface ScanSuccess {
  message: string;
  assignment_id: string;
  slot_number?: number;
  booking_id?: string;
  scanned_at?: string;
}

/**
 * Unified result from POST /v1/tickets/scan. The endpoint accepts any rotating
 * token and reports its type:
 *   - individual → admitted:1 with slot_number
 *   - pool + peek → remaining (nothing admitted yet; show the headcount picker)
 *   - pool admit → admitted (count) + remaining (after)
 */
export interface ScanResult {
  type: 'individual' | 'pool';
  message?: string;
  peek?: boolean;
  idempotent?: boolean;
  admitted?: number;
  remaining?: number;
  slot_number?: number;
  slot_numbers?: number[];
  assignment_id?: string;
  booking_id?: string;
  booking_number?: string;
  event_title?: string;
}

// ── API calls (read-only + scan) ──────────────────────────────────────────────
export const getOrgStats = () =>
  api.get<OrgStats>('/v1/organiser/dashboard/stats').then((r) => r.data);

export const getOrgEvents = (status?: string) =>
  api
    .get<{ events: OrgEventRow[]; count: number; next_cursor: string | null }>(
      '/v1/organiser/dashboard/events',
      {
        params: {
          limit: 100,
          ...(status && status !== 'all' ? { status } : {}),
        },
      },
    )
    .then((r) => r.data.events ?? []);

export const getOrgCampaigns = () =>
  api
    .get<{ campaigns: OrgCampaign[]; count: number }>('/v1/organiser/campaigns')
    .then((r) => r.data.campaigns ?? []);

export const getOrgEventStats = (eventId: string) =>
  api
    .get<OrgEventStats>(`/v1/organiser/dashboard/events/${eventId}/stats`)
    .then((r) => r.data);

export const getOrgEventBookings = (eventId: string) =>
  api
    .get<{ bookings: OrgBooking[] }>(
      `/v1/organiser/dashboard/events/${eventId}/bookings`,
    )
    .then((r) => r.data.bookings ?? []);

export const getOrgEventMerch = (eventId: string) =>
  api
    .get<{ sales: OrgMerchSale[]; has_oversold: boolean; total_oversold: number }>(
      `/v1/organiser/dashboard/events/${eventId}/merch`,
    )
    .then((r) => r.data.sales ?? []);

/**
 * Admit a ticket at the door. The rotating QR encodes a signed token; the
 * server verifies the HMAC, confirms this organiser owns the event, and marks
 * the assignment scanned (single-entry, race-safe).
 */
export const scanTicket = (assignmentId: string, token: string) =>
  api
    .post<ScanSuccess>(`/v1/tickets/${assignmentId}/scan`, { token })
    .then((r) => r.data);

/**
 * Unified door scan. Send the raw token from the QR:
 *   - no count        → admit an individual ticket, or PEEK a pool (returns remaining)
 *   - count + key     → admit `count` heads from a pool (idempotent on key)
 * The backend dispatches on the token type, so the scanner doesn't need to parse it.
 */
export const scanToken = (
  token: string,
  opts?: { count?: number; idempotencyKey?: string },
) =>
  api
    .post<ScanResult>('/v1/tickets/scan', {
      token,
      count: opts?.count,
      idempotency_key: opts?.idempotencyKey,
    })
    .then((r) => r.data);

/**
 * Fresh UUID v4 for a pool-admit idempotency key (one per physical "Admit"
 * action). Math.random is fine here — this is a dedupe token, not a secret.
 * The backend stores it as a UUID, so it must be a valid v4 string.
 */
export function newScanKey(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}

// ── Token parsing ─────────────────────────────────────────────────────────────
// Rotating QR token format: base64url(jsonPayload).<hmacSig>
// The payload carries "aid" — the ticket assignment id we POST the scan to.
// We decode base64url ourselves rather than relying on atob, which is not
// guaranteed across every RN runtime.
const B64 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

function base64Decode(input: string): string {
  let output = '';
  let bc = 0;
  let bs = 0;
  for (let i = 0; i < input.length; i++) {
    const idx = B64.indexOf(input.charAt(i));
    if (idx === -1) continue;
    bs = bc % 4 ? bs * 64 + idx : idx;
    if (bc++ % 4) {
      output += String.fromCharCode(255 & (bs >> ((-2 * bc) & 6)));
    }
  }
  return output;
}

export function extractAssignmentId(token: string): string {
  const lastDot = token.lastIndexOf('.');
  if (lastDot < 0) throw new Error('Malformed token');

  const b64url = token.substring(0, lastDot);
  const b64 = b64url.replace(/-/g, '+').replace(/_/g, '/');
  const padded = b64 + '='.repeat((4 - (b64.length % 4)) % 4);

  let payload: Record<string, unknown>;
  try {
    payload = JSON.parse(base64Decode(padded));
  } catch {
    throw new Error('Token payload is not valid JSON');
  }

  if (typeof payload.aid !== 'string' || !payload.aid) {
    throw new Error('Token missing assignment ID');
  }
  return payload.aid;
}

// ── Formatting helpers ────────────────────────────────────────────────────────
export function fmtMoney(v: number | null | undefined, cur = 'ZAR'): string {
  if (v == null) return '—';
  return new Intl.NumberFormat('en-ZA', {
    style: 'currency',
    currency: cur,
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(v);
}

/** Compact money for hero numbers: R 1.2k / R 3.4M */
export function fmtMoneyCompact(v: number | null | undefined, cur = 'R'): string {
  if (v == null) return '—';
  const abs = Math.abs(v);
  if (abs >= 1_000_000) return `${cur} ${(v / 1_000_000).toFixed(1)}M`;
  if (abs >= 1_000) return `${cur} ${(v / 1_000).toFixed(1)}k`;
  return `${cur} ${Math.round(v).toLocaleString('en-ZA')}`;
}

export function fmtNum(v: number | null | undefined): string {
  if (v == null) return '0';
  return v.toLocaleString('en-ZA');
}

export function fmtDate(d: string | null | undefined): string {
  if (!d) return '—';
  return new Date(d).toLocaleDateString('en-ZA', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
  });
}

export function fmtDateTime(d: string | null | undefined): string {
  if (!d) return '—';
  return new Date(d).toLocaleDateString('en-ZA', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function fmtTime(d: string | null | undefined): string {
  if (!d) return '—';
  return new Date(d).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

export function ctr(clicks: number, impressions: number): string {
  if (!impressions) return '0.00%';
  return `${((clicks / impressions) * 100).toFixed(2)}%`;
}

export function capacityPct(sold: number, cap?: number | null): number | null {
  if (!cap) return null;
  return Math.min((sold / cap) * 100, 100);
}