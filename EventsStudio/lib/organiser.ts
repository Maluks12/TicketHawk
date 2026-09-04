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
    // The IANA zone the organiser entered the event in. Carried for
    // completeness; the Studio renders in DEVICE time, because the phone
    // holding it is standing at that door.
    timezone?: string | null;
    // Per-side admission-window overrides, in MINUTES, exactly as the events
    // table stores them. NULL on either side means "use the 2h default" —
    // see scanWindow() below, which mirrors models.ScanWindow on the backend.
    scan_open_offset_min?: number | null;
    scan_close_offset_min?: number | null;
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

// ── Event information ─────────────────────────────────────────────────────────
// The organiser's information block: a catalogue-keyed list they fill in on the
// web dashboard (age_requirement, doors_open, parking, …). Only the fields they
// actually filled in have rows, so a key being absent is normal, not an error.
export interface EventInfoItem {
  key: string;
  label: string;
  value: string;
  // 'text' | 'enum' | 'time'
  value_type?: string;
  sort_order?: number;
}

/** Catalogue key for the organiser-stated doors time. Stored as 'HH:mm'. */
export const INFO_KEY_DOORS = 'doors_open';

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

/**
 * GET /v1/organiser/events/:id/info — the event's information block.
 *
 * Separate endpoint, separate failure: the overview must still render if this
 * 404s or times out, so every caller treats it as optional.
 */
export const getOrgEventInfo = (eventId: string) =>
  api
    .get<{ items: EventInfoItem[]; is_published: boolean; status: string }>(
      `/v1/organiser/events/${eventId}/info`,
    )
    .then((r) => r.data.items ?? []);

/** Value for a catalogue key, or null when the organiser left it blank. */
export function infoValue(items: EventInfoItem[], key: string): string | null {
  const v = items.find((i) => i?.key === key)?.value;
  const trimmed = (v ?? '').trim();
  return trimmed.length ? trimmed : null;
}

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
// ── Admission (scan) window ───────────────────────────────────────────────────
//
// Mirrors events-db/internal/models/scan_window.go exactly, so the Studio shows
// the same window the server enforces. Getting this wrong is worse than not
// showing it at all: a door that believes it has 20 minutes left when the
// server thinks otherwise finds out at the gate.
//
//   open  = start_time - (scan_open_offset_min  ?? 120 minutes)
//   close = end_time   + (scan_close_offset_min ?? 120 minutes)
//
// A NULL offset means "use the default", which is why `?? DEFAULT` and not a
// falsy check: an organiser who sets 0 means zero grace, not two hours.

/** models.DefaultScanGrace — 2 hours, in minutes. */
export const DEFAULT_SCAN_GRACE_MIN = 120;

export interface ScanWindow {
  open: Date;
  close: Date;
  /** True when the organiser overrode the default on that side. */
  customOpen: boolean;
  customClose: boolean;
}

export function scanWindow(
  startTime: string,
  endTime: string,
  openOffsetMin?: number | null,
  closeOffsetMin?: number | null,
): ScanWindow | null {
  const start = new Date(startTime);
  const end = new Date(endTime);
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return null;

  const openMin = openOffsetMin ?? DEFAULT_SCAN_GRACE_MIN;
  const closeMin = closeOffsetMin ?? DEFAULT_SCAN_GRACE_MIN;

  return {
    open: new Date(start.getTime() - openMin * 60_000),
    close: new Date(end.getTime() + closeMin * 60_000),
    customOpen: openOffsetMin != null && openOffsetMin !== DEFAULT_SCAN_GRACE_MIN,
    customClose: closeOffsetMin != null && closeOffsetMin !== DEFAULT_SCAN_GRACE_MIN,
  };
}

export type ScanPhase = 'before' | 'open' | 'closed';

export interface ScanStatus {
  phase: ScanPhase;
  /** ms until the window opens ('before') or closes ('open'); 0 when closed. */
  msRemaining: number;
}

export function scanStatus(w: ScanWindow, now: Date = new Date()): ScanStatus {
  const t = now.getTime();
  if (t < w.open.getTime()) return { phase: 'before', msRemaining: w.open.getTime() - t };
  if (t > w.close.getTime()) return { phase: 'closed', msRemaining: 0 };
  return { phase: 'open', msRemaining: w.close.getTime() - t };
}

// ── More formatting helpers ──────────────────────────────────────────────────

/** 24-hour wall clock, no seconds: "20:00". Device timezone. */
export function fmtClock(d: string | Date | null | undefined): string {
  if (!d) return '—';
  const date = typeof d === 'string' ? new Date(d) : d;
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleTimeString('en-ZA', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

/** "Sat 05 Sep" — enough to tell two days apart without the year. */
export function fmtDayShort(d: string | Date | null | undefined): string {
  if (!d) return '—';
  const date = typeof d === 'string' ? new Date(d) : d;
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleDateString('en-ZA', {
    weekday: 'short',
    day: '2-digit',
    month: 'short',
  });
}

/** True when the two instants land on the same calendar day, device-local. */
export function sameDay(a: string | Date, b: string | Date): boolean {
  const x = typeof a === 'string' ? new Date(a) : a;
  const y = typeof b === 'string' ? new Date(b) : b;
  if (Number.isNaN(x.getTime()) || Number.isNaN(y.getTime())) return false;
  return (
    x.getFullYear() === y.getFullYear() &&
    x.getMonth() === y.getMonth() &&
    x.getDate() === y.getDate()
  );
}

/** Coarse duration for countdowns: "3d 4h", "5h 12m", "42m", "under a minute". */
export function fmtDuration(ms: number): string {
  if (ms <= 0) return 'under a minute';
  const mins = Math.floor(ms / 60_000);
  if (mins < 1) return 'under a minute';
  const days = Math.floor(mins / 1440);
  const hours = Math.floor((mins % 1440) / 60);
  const rem = mins % 60;
  if (days > 0) return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
  if (hours > 0) return rem > 0 ? `${hours}h ${rem}m` : `${hours}h`;
  return `${rem}m`;
}

/**
 * "19:30" from an organiser-entered 'HH:mm' (or 'HH:mm:ss') doors value.
 * Anything that isn't a clock time is passed through untouched — the field is
 * typed 'time' in the catalogue but the column is free text, and showing what
 * the organiser actually wrote beats showing "—".
 */
export function fmtDoors(raw: string | null | undefined): string | null {
  if (!raw) return null;
  const m = /^(\d{1,2}):(\d{2})/.exec(raw.trim());
  if (!m) return raw.trim();
  const h = Number(m[1]);
  const mi = Number(m[2]);
  if (h > 23 || mi > 59) return raw.trim();
  return `${String(h).padStart(2, '0')}:${String(mi).padStart(2, '0')}`;
}