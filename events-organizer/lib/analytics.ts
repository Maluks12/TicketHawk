// ─────────────────────────────────────────────────────────────────────────────
// Analytics client
//
// Types mirror internal/repository/analytics_repository.go exactly. Nullable
// fields are `number | null` on purpose and are NOT defaulted to 0 anywhere in
// this file: null means "we have no data", 0 means "we measured nothing", and
// the dashboard renders those differently ("—" vs "0"). Collapsing them would
// tell an organiser their event reached nobody when in fact collection hadn't
// started.
//
// No metric is recomputed here. Every rate (CTR, retention, attendance) is
// derived once, server-side, so the number in a chart tooltip can never
// disagree with the number in the table beside it.
// ─────────────────────────────────────────────────────────────────────────────
import { api } from './api';

// ── Types ────────────────────────────────────────────────────────────────────

export interface EventMetrics {
  impressions: number;
  views: number;
  ctr: number | null;
  retention_seconds: number | null;
  dwell_samples: number;
  dwell_coverage: number | null;

  tickets_sold: number;
  bookings: number;
  gross_revenue: number;
  platform_fees: number;
  psr: number;
  conversion: number | null;

  scanned: number;
  no_shows: number | null;
  attendance_rate: number | null;
}

export interface OverviewEvent {
  event_id: string;
  title: string;
  status: string;
  start_time: string;
  end_time: string;
  capacity: number | null;
  currency: string;
  days_to_go: number;
  metrics: EventMetrics;
}

export interface OverviewAverages {
  events_counted: number;
  avg_impressions: number;
  avg_views: number;
  avg_tickets_sold: number;
  avg_psr: number;
  avg_gross_revenue: number;
  avg_scanned: number;
  avg_no_shows: number | null;
  ctr: number | null;
  conversion: number | null;
  retention_seconds: number | null;
  attendance_rate: number | null;
  currency: string;
}

export interface AnalyticsOverview {
  upcoming: OverviewEvent | null;
  averages: OverviewAverages | null;
}

export interface EventAnalyticsRow {
  event_id: string;
  title: string;
  status: string;
  start_time: string;
  ended: boolean;
  currency: string;
  metrics: EventMetrics;
}

export interface EventAnalytics extends EventAnalyticsRow {
  end_time: string;
  capacity: number | null;
  metrics_since: string | null;
}

export type Granularity = 'minute' | 'hour' | 'day';

export interface SeriesPoint {
  bucket: string;
  impressions: number;
  views: number;
  tickets_sold: number;
  bookings: number;
  gross_revenue: number;
  psr: number;
  scanned: number;
  dwell_ms: number;
  dwell_samples: number;
}

export interface SeriesResult {
  granularity: Granularity;
  timezone: string;
  from: string;
  to: string;
  clamped: boolean;
  clamp_note?: string;
  points: SeriesPoint[];
  totals: EventMetrics;
}

// ── Calls ────────────────────────────────────────────────────────────────────

export async function getAnalyticsOverview(): Promise<AnalyticsOverview> {
  const { data } = await api.get('/organiser/dashboard/analytics/overview');
  return data;
}

export async function listEventAnalytics(): Promise<EventAnalyticsRow[]> {
  // /breakdown, not /events: URLs ending "/analytics/events" are on common
  // ad/tracker blocklists, so the browser cancelled this call before it left —
  // nothing in the server log, just a failed section. See main.go.
  const { data } = await api.get('/organiser/dashboard/analytics/breakdown');
  return data?.events ?? [];
}

export async function getEventAnalytics(eventId: string): Promise<EventAnalytics> {
  const { data } = await api.get(`/organiser/dashboard/analytics/breakdown/${eventId}`);
  return data;
}

export interface SeriesQuery {
  granularity: Granularity;
  eventIds?: string[];
  from?: Date;
  to?: Date;
  tz?: string;
}

export async function getSeries(q: SeriesQuery): Promise<SeriesResult> {
  const params: Record<string, string> = { granularity: q.granularity };
  if (q.eventIds?.length) params.event_id = q.eventIds.join(',');
  if (q.from) params.from = q.from.toISOString();
  if (q.to) params.to = q.to.toISOString();
  // The browser's own zone by default so "by day" means the organiser's day.
  // Falls back to SAST if the browser won't say (older Safari, locked-down
  // privacy modes), which matches the server default.
  params.tz = q.tz
    ?? Intl.DateTimeFormat().resolvedOptions().timeZone
    ?? 'Africa/Johannesburg';

  const { data } = await api.get('/organiser/dashboard/analytics/series', { params });
  return data;
}

// ── Windows ──────────────────────────────────────────────────────────────────

// Ranges offered per grain. Each keeps the point count under the server's
// 1500-point cap so nothing is silently clamped by picking a preset.
export const RANGES: Record<Granularity, Array<{ id: string; label: string; hours: number }>> = {
  minute: [
    { id: '1h',  label: 'Last hour',    hours: 1 },
    { id: '6h',  label: 'Last 6 hours', hours: 6 },
    { id: '24h', label: 'Last 24 hours', hours: 24 },
  ],
  hour: [
    { id: '24h', label: 'Last 24 hours', hours: 24 },
    { id: '7d',  label: 'Last 7 days',  hours: 24 * 7 },
    { id: '30d', label: 'Last 30 days', hours: 24 * 30 },
  ],
  day: [
    { id: '30d',  label: 'Last 30 days',  hours: 24 * 30 },
    { id: '90d',  label: 'Last 90 days',  hours: 24 * 90 },
    { id: '365d', label: 'Last 12 months', hours: 24 * 365 },
  ],
};

export function rangeToDates(hours: number): { from: Date; to: Date } {
  const to = new Date();
  const from = new Date(to.getTime() - hours * 3600 * 1000);
  return { from, to };
}

// ── Formatters ───────────────────────────────────────────────────────────────
//
// EVERY formatter takes `number | null` and returns EM DASH for null. This is
// the single place the "no data" convention is enforced; call these rather than
// interpolating a raw value, or a null silently renders as "null" on screen.

export const DASH = '—';

export function fmtNum(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) return DASH;
  return new Intl.NumberFormat('en-ZA').format(Math.round(n));
}

/** Compact form for headline tiles: 12 400 → 12.4k */
export function fmtCompact(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) return DASH;
  const abs = Math.abs(n);
  if (abs >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
  if (abs >= 10_000) return (n / 1_000).toFixed(1).replace(/\.0$/, '') + 'k';
  return fmtNum(n);
}

export function fmtMoney(n: number | null | undefined, currency = 'ZAR'): string {
  if (n === null || n === undefined || Number.isNaN(n)) return DASH;
  // Matches fmtMoney in lib/utils.tsx. maximumFractionDigits: 0 ROUNDED rather
  // than truncated, so PSR of R244.40 printed as "R 244" — a 40c discrepancy
  // against the dashboard on a card the organiser reads as their earnings. A
  // bare minimum of 0 is not the fix either: it drops a trailing zero, turning
  // R28.20 into "R 28,2". Whole rands print clean, cents print in full.
  //
  // fmtMoneyCompact delegates here below 10k, so the k/M abbreviations above
  // that threshold are unaffected.
  const rounded = Math.round(n * 100) / 100;
  const hasCents = Math.round(Math.abs(rounded) * 100) % 100 !== 0;
  return new Intl.NumberFormat('en-ZA', {
    style: 'currency',
    currency,
    minimumFractionDigits: hasCents ? 2 : 0,
    maximumFractionDigits: 2,
  }).format(rounded);
}

export function fmtMoneyCompact(n: number | null | undefined, currency = 'ZAR'): string {
  if (n === null || n === undefined || Number.isNaN(n)) return DASH;
  const sym = currency === 'ZAR' ? 'R' : '';
  const abs = Math.abs(n);
  if (abs >= 1_000_000) return `${sym}${(n / 1_000_000).toFixed(1)}M`;
  if (abs >= 10_000) return `${sym}${(n / 1_000).toFixed(1)}k`;
  return fmtMoney(n, currency);
}

/** 0.0432 → "4.3%" */
export function fmtPct(n: number | null | undefined, digits = 1): string {
  if (n === null || n === undefined || Number.isNaN(n)) return DASH;
  return `${(n * 100).toFixed(digits)}%`;
}

/** Retention: 8.4 → "8s", 95 → "1m 35s". Seconds in, human out. */
export function fmtDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || Number.isNaN(seconds)) return DASH;
  const s = Math.round(seconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const rem = s % 60;
  if (m < 60) return rem ? `${m}m ${rem}s` : `${m}m`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

/** Axis/tooltip labels, formatted for the grain being shown. */
export function fmtBucket(iso: string, g: Granularity): string {
  const d = new Date(iso);
  if (g === 'day') {
    return d.toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' });
  }
  if (g === 'hour') {
    return d.toLocaleString('en-ZA', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
  }
  return d.toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

/**
 * Whether a retention figure rests on enough samples to be worth stating
 * plainly. Below this the dashboard labels it a sample rather than a
 * measurement — without the caveat, a 3%-coverage number reads as fact.
 */
export const LOW_COVERAGE = 0.2;

export function coverageNote(m: Pick<EventMetrics, 'dwell_coverage' | 'dwell_samples'>): string | null {
  if (m.dwell_samples === 0) return 'No dwell data reported yet';
  if (m.dwell_coverage !== null && m.dwell_coverage < LOW_COVERAGE) {
    return `Based on ${fmtPct(m.dwell_coverage, 0)} of views — treat as indicative`;
  }
  return null;
}