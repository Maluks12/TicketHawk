import { create } from 'zustand';
import { api } from '../lib/api';

// The dashboards' lib/api.ts reads NEXT_PUBLIC_API_URL inline rather than
// exporting it, so it is read the same way here. Every deployed build sets it
// to the API root + /v1 (docker-compose.prod.yml, deploy.yml), which
// statusUrl() below accounts for. In a local run with no .env.local it is
// unset, the poll 404s against the Next dev server, and a 404 reads as
// "alive" — so the failure mode is no banner, never a false one.
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? '';

// ─────────────────────────────────────────────────────────────────────────────
// Service status — what the banner at the top of every page is reading.
//
// ── The thing to understand first ────────────────────────────────────────────
//
// A server that is down cannot tell you it is down. So "TicketHawk is not
// responding" is NEVER learned from the API; it is learned from our own failed
// requests. Two sources feed this store:
//
//   1. PASSIVE — an interceptor on the shared axios instance. Every real call
//      the app already makes is a health probe we get for free, so the banner
//      appears the moment something the user did actually failed, not up to a
//      minute later when a poll happens to fire.
//
//   2. ACTIVE — a poll of GET /v1/status. This is how we learn the two things
//      failed requests cannot tell us: that a maintenance window was DECLARED
//      (so we say "maintenance", not "something went wrong"), and that the API
//      is up but a dependency is not. It is also how we notice recovery while
//      the user is sitting on a page doing nothing.
//
// ── Why fetch and not axios for the poll ─────────────────────────────────────
//
// The axios instance attaches the bearer token and owns the refresh-rotation
// logic. A status poll must not be able to trigger a token rotation, and must
// work identically for a signed-out visitor, so it goes out as a bare,
// credential-free GET with no custom headers — which also keeps it a "simple"
// CORS request, so it never costs a preflight.
//
// ── A caveat worth knowing ───────────────────────────────────────────────────
//
// When the origin is down, Cloudflare answers with its own 5xx page, and that
// page carries no CORS headers — so the browser blocks it and the fetch throws
// exactly as it would with no network at all. In the browser we therefore
// cannot always tell "server down" from "device offline" by status code. That
// is what navigator.onLine is for below, and it is why the native apps (no CORS)
// can be more precise than the web can.
// ─────────────────────────────────────────────────────────────────────────────

export type ServiceState =
  | 'ok'           // everything normal — no banner
  | 'maintenance'  // operator declared a window; message comes from the server
  | 'degraded'     // API answering, a dependency it needs is not
  | 'down'         // our requests are failing and the device says it is online
  | 'offline';     // the device has no connection — not our fault, still worth saying

interface StatusState {
  state: ServiceState;
  /** Operator's words. Only ever set for `maintenance`. */
  message: string;
  /** RFC3339 end of the maintenance window, when one was declared. */
  endsAt: string | null;
  /** Identity of the current notice. Dismissals are keyed on it. */
  revision: string;
  /** A probe is in flight — drives the Retry button's label. */
  checking: boolean;
  /** True for a few seconds after recovery, so the banner can confirm it. */
  recovered: boolean;
}

export const useStatusStore = create<StatusState>(() => ({
  state: 'ok',
  message: '',
  endsAt: null,
  revision: '',
  checking: false,
  recovered: false,
}));

// ── Tuning ───────────────────────────────────────────────────────────────────

/** Healthy: one cheap request a minute, only while the tab is visible. */
const POLL_OK_MS = 60_000;
/** Unhealthy: check often, because the user is waiting to be told it is back. */
const POLL_BAD_MS = 10_000;
/** Long enough for a slow SA mobile connection, short enough to still be news. */
const PROBE_TIMEOUT_MS = 8_000;
/**
 * One failed request is not an outage — a single dropped connection on a train
 * would put a red bar on the page. Two in a row is.
 */
const FAILURES_BEFORE_DOWN = 2;
/** How long "Back online" stays up before the banner removes itself. */
const RECOVERED_MS = 4_000;

// ── Where to ask ─────────────────────────────────────────────────────────────
//
// Handles both shapes of base URL in this codebase: the consumer apps set
// NEXT_PUBLIC_API_URL to the API root, the dashboards set it to the root + /v1.
function statusUrl(): string {
  const base = (API_BASE_URL ?? '').replace(/\/+$/, '');
  return base.endsWith('/v1') ? `${base}/status` : `${base}/v1/status`;
}

// ── Transitions ──────────────────────────────────────────────────────────────

let consecutiveFailures = 0;
let recoveredTimer: ReturnType<typeof setTimeout> | null = null;

function isBad(s: ServiceState): boolean {
  return s === 'down' || s === 'offline' || s === 'degraded';
}

/**
 * Everything is fine. Announce it briefly if we had previously said otherwise.
 *
 * fromProbe matters: `maintenance` and `degraded` are facts only GET /v1/status
 * knows, and during a maintenance window the API keeps working normally. If a
 * successful ordinary request were allowed to clear those states, the banner
 * would blink out on every API call and return on the next poll.
 */
function noteHealthy(fromProbe = false) {
  consecutiveFailures = 0;
  const prev = useStatusStore.getState().state;

  if (!fromProbe && (prev === 'maintenance' || prev === 'degraded')) return;

  useStatusStore.setState({
    state: 'ok',
    message: '',
    endsAt: null,
    revision: '',
    recovered: isBad(prev),
  });

  if (isBad(prev)) {
    if (recoveredTimer) clearTimeout(recoveredTimer);
    recoveredTimer = setTimeout(() => {
      useStatusStore.setState({ recovered: false });
      recoveredTimer = null;
    }, RECOVERED_MS);
  }
}

/**
 * A request failed in a way that suggests the platform, not the request.
 * Offline is decided first: telling someone on a plane that TicketHawk is
 * broken is both wrong and the kind of thing people screenshot.
 */
function noteFailure() {
  if (typeof navigator !== 'undefined' && navigator.onLine === false) {
    consecutiveFailures = 0;
    useStatusStore.setState({ state: 'offline', message: '', endsAt: null, revision: 'offline', recovered: false });
    return;
  }

  consecutiveFailures += 1;
  if (consecutiveFailures < FAILURES_BEFORE_DOWN) return;

  // Never overwrite a declared maintenance window with a generic failure — a
  // deploy that takes the API down mid-window is the window, not a new fault.
  if (useStatusStore.getState().state === 'maintenance') return;

  useStatusStore.setState({ state: 'down', message: '', endsAt: null, revision: 'down', recovered: false });
}

interface StatusPayload {
  status?: string;
  message?: string;
  ends_at?: string;
  revision?: string;
}

function applyServerStatus(body: StatusPayload) {
  consecutiveFailures = 0;

  if (body?.status === 'maintenance') {
    useStatusStore.setState({
      state: 'maintenance',
      message: (body.message ?? '').trim(),
      endsAt: body.ends_at ?? null,
      revision: body.revision ?? 'maintenance',
      recovered: false,
    });
    return;
  }

  if (body?.status === 'degraded') {
    useStatusStore.setState({
      state: 'degraded',
      message: '',
      endsAt: null,
      revision: body.revision ?? 'degraded',
      recovered: false,
    });
    return;
  }

  noteHealthy(true);
}

// ── The probe ────────────────────────────────────────────────────────────────

export async function probeNow(): Promise<void> {
  if (typeof window === 'undefined') return;

  if (navigator.onLine === false) {
    useStatusStore.setState({
      state: 'offline', message: '', endsAt: null, revision: 'offline', recovered: false,
    });
    return;
  }

  useStatusStore.setState({ checking: true });

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), PROBE_TIMEOUT_MS);

  try {
    const res = await fetch(statusUrl(), {
      signal: controller.signal,
      cache: 'no-store',
      credentials: 'omit',
    });

    if (res.ok) {
      applyServerStatus((await res.json()) as StatusPayload);
    } else if (res.status >= 500) {
      noteFailure();
    } else {
      // 404 (this endpoint not deployed yet), 429 (throttled), any other 4xx:
      // something answered us in full, which is the only question being asked
      // here. Treat it as alive rather than inventing an outage.
      noteHealthy(true);
    }
  } catch {
    noteFailure();
  } finally {
    clearTimeout(timer);
    useStatusStore.setState({ checking: false });
  }
}

// ── Passive signal: every call the app already makes ─────────────────────────
//
// Registered at module scope so it attaches exactly once. It only observes —
// the error is re-thrown untouched, so the refresh/retry logic in lib/api.ts
// behaves exactly as it did before.
api.interceptors.response.use(
  (res) => {
    noteHealthy();
    return res;
  },
  (error) => {
    const status = error?.response?.status;
    if (status === undefined) {
      // No response at all: timeout, connection refused, DNS, or a CORS-blocked
      // edge error page. All of them mean we could not be served.
      noteFailure();
    } else if (status >= 500) {
      noteFailure();
    } else {
      // A 401/404/422 is the server working correctly and disagreeing with us.
      noteHealthy();
    }
    return Promise.reject(error);
  },
);

// ── Polling loop ─────────────────────────────────────────────────────────────

let started = false;
let pollTimer: ReturnType<typeof setTimeout> | null = null;

function schedule() {
  if (pollTimer) clearTimeout(pollTimer);
  const delay = useStatusStore.getState().state === 'ok' ? POLL_OK_MS : POLL_BAD_MS;
  pollTimer = setTimeout(tick, delay);
}

async function tick() {
  // A backgrounded tab is not watching a banner, and mobile browsers throttle
  // its timers anyway. Skip the request and let visibilitychange catch up.
  if (document.visibilityState === 'visible') await probeNow();
  schedule();
}

/**
 * Starts the monitor. Idempotent — safe to call from every mount; the banner
 * lives in the root layout, so in practice this runs once per page load.
 */
export function startStatusMonitor(): void {
  if (started || typeof window === 'undefined') return;
  started = true;

  window.addEventListener('online', () => { void probeNow(); });
  window.addEventListener('offline', () => {
    useStatusStore.setState({
      state: 'offline', message: '', endsAt: null, revision: 'offline', recovered: false,
    });
  });
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') { void probeNow(); schedule(); }
  });

  void probeNow();
  schedule();
}