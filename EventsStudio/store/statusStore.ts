import { AppState } from 'react-native';
import NetInfo from '@react-native-community/netinfo';
import { create } from 'zustand';
import { api, API_BASE_URL } from '../lib/api';

// ─────────────────────────────────────────────────────────────────────────────
// Service status — what the banner at the top of every screen is reading.
//
// Native twin of store/statusStore.ts in the web apps; same states, same rules,
// same endpoint. The one file serves both EventsApp and EventsStudio: both
// export `api` and `API_BASE_URL` from lib/api.ts, and both already ship
// @react-native-community/netinfo — so this adds no native module and needs no
// new store build beyond the usual one.
//
// ── The thing to understand first ────────────────────────────────────────────
//
// A server that is down cannot tell you it is down. "TicketHawk is not
// responding" is learned from our OWN failed requests:
//
//   1. PASSIVE — an interceptor on the shared axios instance. Every call the app
//      already makes doubles as a health probe, so the banner appears the moment
//      something the user did actually failed.
//
//   2. ACTIVE — a poll of GET /v1/status, which is the only way to learn that a
//      maintenance window was DECLARED (say "maintenance", not "something broke")
//      and to notice recovery while the user is sitting still.
//
// ── Why native can be more precise than the web ──────────────────────────────
//
// No CORS. When the origin is down, Cloudflare's 502/530 reaches us as a real
// status code instead of an opaque network error, and NetInfo tells us whether
// the phone itself has a connection. So "offline" and "TicketHawk is down" are
// genuinely different states here, not a best guess.
// ─────────────────────────────────────────────────────────────────────────────

export type ServiceState =
  | 'ok'           // everything normal — no banner
  | 'maintenance'  // operator declared a window; message comes from the server
  | 'degraded'     // API answering, a dependency it needs is not
  | 'down'         // our requests are failing while the phone has a connection
  | 'offline';     // the phone has no usable connection

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

/** Healthy: one cheap request a minute, only while the app is in the foreground. */
const POLL_OK_MS = 60_000;
/** Unhealthy: check often — the user is waiting to be told it is back. */
const POLL_BAD_MS = 10_000;
/**
 * Half the api instance's 20s timeout. A status probe that takes longer than
 * this on SA mobile data is itself the answer.
 */
const PROBE_TIMEOUT_MS = 10_000;
/** One dropped request on a taxi ride is not an outage. Two in a row is. */
const FAILURES_BEFORE_DOWN = 2;
/** How long "Back online" stays up before the banner removes itself. */
const RECOVERED_MS = 4_000;

// ── Where to ask ─────────────────────────────────────────────────────────────
//
// EXPO_PUBLIC_API_URL is the API root in every eas.json profile; the /v1 check
// is there so this keeps working if that ever changes.
function statusUrl(): string {
  const base = (API_BASE_URL ?? '').replace(/\/+$/, '');
  return base.endsWith('/v1') ? `${base}/status` : `${base}/v1/status`;
}

// ── Device connectivity (NetInfo) ────────────────────────────────────────────
//
// null = not known yet. Only an explicit false counts as offline:
// isInternetReachable starts as null on every launch and a null must not paint
// an "offline" banner over a working app. An explicit false on it is worth
// honouring, though — it is what a captive portal (venue WiFi login page) looks
// like: connected to WiFi, no way out.
let deviceOnline: boolean | null = null;

function deviceLooksOffline(): boolean {
  return deviceOnline === false;
}

// ── Transitions ──────────────────────────────────────────────────────────────

let consecutiveFailures = 0;
let recoveredTimer: ReturnType<typeof setTimeout> | null = null;

function isBad(s: ServiceState): boolean {
  return s === 'down' || s === 'offline' || s === 'degraded';
}

function setOffline() {
  consecutiveFailures = 0;
  useStatusStore.setState({ state: 'offline', message: '', endsAt: null, revision: 'offline', recovered: false });
}

/**
 * Everything is fine. Announce it briefly if we had previously said otherwise.
 *
 * fromProbe matters: `maintenance` and `degraded` are facts only GET /v1/status
 * knows, and during a maintenance window the API keeps working normally. If a
 * successful ordinary request could clear those states, the banner would blink
 * out on every API call and return on the next poll.
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
 * A request failed in a way that points at the platform, not the request.
 * Offline is decided first: telling someone in a basement venue that
 * TicketHawk is broken is both wrong and the kind of thing people screenshot.
 */
function noteFailure() {
  if (deviceLooksOffline()) {
    setOffline();
    return;
  }

  consecutiveFailures += 1;
  if (consecutiveFailures < FAILURES_BEFORE_DOWN) return;

  // A deploy that takes the API down mid-window IS the window, not a new fault.
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

let probeInFlight = false;

export async function probeNow(): Promise<void> {
  if (deviceLooksOffline()) {
    setOffline();
    return;
  }
  // AppState 'active', the NetInfo listener and a tap on Retry can all land in
  // the same second; one request answers all of them.
  if (probeInFlight) return;
  probeInFlight = true;

  useStatusStore.setState({ checking: true });

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), PROBE_TIMEOUT_MS);

  try {
    // Plain fetch, not `api`: no bearer token, and no way for a status poll to
    // trigger a refresh-token rotation.
    const res = await fetch(statusUrl(), {
      signal: controller.signal,
      headers: { Accept: 'application/json', 'Cache-Control': 'no-cache' },
    });

    if (res.ok) {
      applyServerStatus((await res.json()) as StatusPayload);
    } else if (res.status >= 500) {
      // Includes Cloudflare's own 52x/530 when the tunnel has no origin.
      noteFailure();
    } else {
      // 404 (endpoint not deployed yet), 429 (throttled), any other 4xx:
      // something answered in full, which is the only question being asked.
      noteHealthy(true);
    }
  } catch {
    noteFailure();
  } finally {
    clearTimeout(timer);
    probeInFlight = false;
    useStatusStore.setState({ checking: false });
  }
}

// ── Passive signal: every call the app already makes ─────────────────────────
//
// Registered at module scope so it attaches exactly once. It only observes —
// the error is re-thrown untouched, so the refresh/retry logic in lib/api.ts
// behaves exactly as before.
api.interceptors.response.use(
  (res) => {
    noteHealthy();
    return res;
  },
  (error) => {
    const status = error?.response?.status;
    if (status === undefined) {
      // No response at all: timeout, refused, DNS, no route.
      noteFailure();
    } else if (status >= 500) {
      noteFailure();
    } else {
      // 401/404/422 is the server working correctly and disagreeing with us.
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
  // A backgrounded app is not showing a banner, and iOS suspends it anyway.
  // Skip the request; the AppState listener catches up on return.
  if (AppState.currentState === 'active') await probeNow();
  schedule();
}

/**
 * Starts the monitor. Idempotent — the banner lives in the root layout, so in
 * practice this runs once per launch. Deliberately never torn down: it is
 * app-lifetime state, like the auth session.
 */
export function startStatusMonitor(): void {
  if (started) return;
  started = true;

  NetInfo.addEventListener((net) => {
    const wasOffline = deviceOnline === false;
    deviceOnline =
      net.isConnected === false || net.isInternetReachable === false
        ? false
        : net.isConnected === true
          ? true
          : null;

    if (deviceOnline === false) {
      setOffline();
    } else if (wasOffline && deviceOnline === true) {
      // Back on a network. Do not declare victory from NetInfo alone — ask.
      void probeNow();
    }
  });

  AppState.addEventListener('change', (next) => {
    if (next === 'active') {
      void probeNow();
      schedule();
    }
  });

  void probeNow();
  schedule();
}