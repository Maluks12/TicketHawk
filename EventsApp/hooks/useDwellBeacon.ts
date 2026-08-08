import { useEffect, useRef } from 'react';
import { AppState, AppStateStatus } from 'react-native';
import { API_BASE_URL } from '../lib/api';

// ─────────────────────────────────────────────────────────────────────────────
// Dwell beacon — the only client change the analytics feature needs.
//
// Impressions, views, tickets, PSR, scans and no-shows are all measured
// server-side and already work. Retention is the exception: nothing on the
// server can know how long someone looked at a screen, so the client has to
// say. Until this ships the organiser dashboard shows "—" for retention rather
// than a fabricated zero.
//
// WHY ON CLOSE, NOT ON A TIMER
// A heartbeat ("still here after 5s?") measures whether someone left, not how
// long they stayed, and multiplies the request count for the same information.
// Reporting once on unmount costs one request per view and reports the real
// figure.
//
// The trade-off: a force-quit reports nothing, so retention is measured over a
// SAMPLE of views. That's why the API tracks dwell_samples alongside dwell_ms
// and the dashboard shows coverage next to the number — a 4%-coverage figure is
// labelled indicative rather than presented as fact.
// ─────────────────────────────────────────────────────────────────────────────

// Anything longer is a phone left face-up on a table, not a reader. The server
// clamps at the same value; clamping here too saves a pointless round trip.
const MAX_DWELL_MS = 30 * 60 * 1000;

// Below this it's a mis-tap or a back-swipe, not a view worth measuring.
const MIN_DWELL_MS = 800;

/**
 * Reports how long the event detail screen was actually in the FOREGROUND.
 *
 * Backgrounded time is subtracted, not counted. Without that, an event screen
 * left open while the phone is in a pocket reports hours and single-handedly
 * moves the organiser's average retention — these counters are additive with no
 * delete path, so one bad sample is permanent.
 *
 * @param eventId  The event being viewed — UUID or slug, both are accepted by
 *                 the API. Pass null/undefined while loading; the timer doesn't
 *                 start until there's an id to report against.
 */
export function useDwellBeacon(eventId: string | null | undefined) {
  const activeSince = useRef<number | null>(null);
  const accumulated = useRef(0);
  const sent = useRef(false);

  useEffect(() => {
    if (!eventId) return;

    activeSince.current = Date.now();
    accumulated.current = 0;
    sent.current = false;

    const onAppState = (state: AppStateStatus) => {
      if (state === 'active') {
        activeSince.current = Date.now();
      } else if (activeSince.current !== null) {
        accumulated.current += Date.now() - activeSince.current;
        activeSince.current = null;
      }
    };

    const sub = AppState.addEventListener('change', onAppState);

    return () => {
      sub.remove();

      // Guards React StrictMode double-invoking effects in development, which
      // would otherwise report every view twice and halve apparent retention.
      if (sent.current) return;
      sent.current = true;

      if (activeSince.current !== null) {
        accumulated.current += Date.now() - activeSince.current;
      }
      const ms = Math.min(accumulated.current, MAX_DWELL_MS);
      if (ms < MIN_DWELL_MS) return;

      // Not awaited, errors swallowed: this fires during unmount, and a failed
      // analytics call must never surface to the user or delay the navigation
      // they just asked for.
      //
      // Plain fetch rather than the axios `api` instance on purpose — the
      // endpoint is unauthenticated and axios would attach the interceptor's
      // Authorization header and its 401 refresh logic, which could kick off a
      // token rotation from a screen that is already unmounting.
      fetch(`${API_BASE_URL}/v1/events/${encodeURIComponent(eventId)}/dwell`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ms }),
      }).catch(() => {});
    };
  }, [eventId]);
}