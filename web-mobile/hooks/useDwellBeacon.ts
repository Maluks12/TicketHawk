'use client';
import { useEffect, useRef } from 'react';
import { API_BASE_URL } from '@/lib/api';

// ─────────────────────────────────────────────────────────────────────────────
// Dwell beacon — the only client change the analytics feature needs.
//
// Everything else an organiser sees (impressions, views, tickets, PSR, scans,
// no-shows) is measured server-side and already works. Retention is the
// exception: nothing on the server knows how long a page stayed open, so the
// client has to say. Until this ships the dashboard shows "—" for retention
// rather than a fabricated zero.
// ─────────────────────────────────────────────────────────────────────────────

const MAX_DWELL_MS = 30 * 60 * 1000; // server clamps here too
const MIN_DWELL_MS = 800;            // below this it's a bounce, not a read

/**
 * Reports how long the event page was actually VISIBLE.
 *
 * @param eventId  UUID or slug — the API resolves both. Most real traffic
 *                 arrives with a slug because lib/shareLinks prefers it, which
 *                 is exactly why the endpoint accepts either.
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

    // Tab switches and minimises pause the clock. A pinned tab left open all
    // afternoon is not a four-hour read, and counting it as one would move the
    // organiser's average permanently — these counters are additive with no
    // delete path.
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        activeSince.current = Date.now();
      } else if (activeSince.current !== null) {
        accumulated.current += Date.now() - activeSince.current;
        activeSince.current = null;
      }
    };

    const send = () => {
      // Guards against firing twice — StrictMode in dev double-invokes effects,
      // and in production both `pagehide` and the cleanup can run for the same
      // view. Two reports would halve apparent retention by doubling samples.
      if (sent.current) return;
      sent.current = true;

      if (activeSince.current !== null) {
        accumulated.current += Date.now() - activeSince.current;
        activeSince.current = null;
      }
      const ms = Math.min(accumulated.current, MAX_DWELL_MS);
      if (ms < MIN_DWELL_MS) return;

      const url = `${API_BASE_URL}/v1/events/${encodeURIComponent(eventId)}/dwell`;
      const body = JSON.stringify({ ms });

      // sendBeacon is the only thing browsers reliably run during unload. A
      // normal fetch is cancelled when the document goes away, which would lose
      // precisely the reports from people who navigated straight off — biasing
      // retention upward by keeping only the lingerers.
      if (typeof navigator !== 'undefined' && navigator.sendBeacon) {
        navigator.sendBeacon(url, new Blob([body], { type: 'application/json' }));
      } else {
        fetch(url, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body,
          keepalive: true,
        }).catch(() => {});
      }
    };

    document.addEventListener('visibilitychange', onVisibility);
    // pagehide, not unload: unload never fires on iOS Safari, and merely
    // registering it disqualifies the page from the back/forward cache.
    window.addEventListener('pagehide', send);

    return () => {
      document.removeEventListener('visibilitychange', onVisibility);
      window.removeEventListener('pagehide', send);
      send(); // client-side route change — pagehide doesn't fire for those
    };
  }, [eventId]);
}