'use client';

import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useStatusStore, startStatusMonitor, probeNow, type ServiceState } from '@/store/statusStore';

// ─────────────────────────────────────────────────────────────────────────────
// The banner every page shows when TicketHawk is not itself — dashboard edition.
//
// Same store and same states as the consumer site; two things differ here.
//
// ── 1. The shell is exactly one viewport tall ────────────────────────────────
//
// The dashboard shell is `height: 100dvh; overflow: hidden`, with the sidebar
// and the main column scrolling INSIDE it. A banner simply placed above it would
// make the document one banner taller than the viewport: the body would gain a
// second scrollbar and the bottom of the sidebar would sit below the fold.
//
// So this component measures itself and publishes its height as
// --service-banner-h on <html>, and the shell subtracts it:
//
//   height: calc(100dvh - var(--service-banner-h, 0px))
//
// The fallback is 0px, so the shell is byte-for-byte the old height whenever
// the banner is not showing — including on the server, and on any page this
// component never mounts on.
//
// ── 2. Colour comes from the design system, not from here ────────────────────
//
// Both dashboards define --warning*, --danger*, --success* for dark AND light
// themes, so the banner follows the theme switch with no code of its own. The
// tint is layered over var(--bg) rather than left to composite over whatever is
// behind it, so it reads the same on every page regardless of body background.
//
// The same file serves events-organizer and events-admin; the two share token
// names by design (admin's globals.css was ported from the organiser's).
// ─────────────────────────────────────────────────────────────────────────────

type Tone = { dot: string; tint: string; border: string };

const TONES: Record<Exclude<ServiceState, 'ok'> | 'recovered', Tone> = {
  maintenance: { dot: 'var(--warning)', tint: 'var(--warning-bg)',     border: 'var(--warning-border)' },
  degraded:    { dot: 'var(--warning)', tint: 'var(--warning-bg)',     border: 'var(--warning-border)' },
  down:        { dot: 'var(--danger)',  tint: 'var(--danger-bg)',      border: 'var(--danger-border)'  },
  offline:     { dot: 'var(--text-3)',  tint: 'var(--surface-strong)', border: 'var(--border)'         },
  recovered:   { dot: 'var(--success)', tint: 'var(--success-bg)',     border: 'var(--success-border)' },
};

/** "Until 02:00." — rendered against the SERVER's clock, not the device's. */
function endsAtLabel(iso: string | null): string {
  if (!iso) return '';
  const t = new Date(iso);
  if (Number.isNaN(t.getTime()) || t.getTime() < Date.now()) return '';
  return `Until ${t.toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit', hour12: false })}.`;
}

export default function ServiceBanner() {
  const state     = useStatusStore((s) => s.state);
  const message   = useStatusStore((s) => s.message);
  const endsAt    = useStatusStore((s) => s.endsAt);
  const revision  = useStatusStore((s) => s.revision);
  const checking  = useStatusStore((s) => s.checking);
  const recovered = useStatusStore((s) => s.recovered);

  // Keyed to the NOTICE: dismissing today's maintenance notice must not also
  // silence tomorrow's outage.
  const [dismissed, setDismissed] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  const showRecovered = state === 'ok' && recovered;
  const key = `${state}:${revision}`;
  const visible = showRecovered || (state !== 'ok' && dismissed !== key);

  useEffect(() => { startStatusMonitor(); }, []);

  // Publish our height for the shell to subtract. Layout effect, so the shell
  // resizes in the same frame the banner appears rather than one frame later.
  // A ResizeObserver, not a one-off measurement, because the message wraps to
  // two lines on a narrow window and the height has to follow it.
  useLayoutEffect(() => {
    const root = document.documentElement;
    const el = ref.current;
    if (!visible || !el) {
      root.style.setProperty('--service-banner-h', '0px');
      return;
    }
    const publish = () => root.style.setProperty('--service-banner-h', `${el.offsetHeight}px`);
    publish();
    const ro = new ResizeObserver(publish);
    ro.observe(el);
    return () => {
      ro.disconnect();
      root.style.setProperty('--service-banner-h', '0px');
    };
  }, [visible]);

  if (!visible) return null;

  const tone = TONES[showRecovered ? 'recovered' : (state as Exclude<ServiceState, 'ok'>)];

  let text: string;
  let detail = '';

  if (showRecovered) {
    text = 'Back online.';
  } else if (state === 'maintenance') {
    text = message || "TicketHawk is under maintenance. Some actions won't work until it's done.";
    detail = endsAtLabel(endsAt);
  } else if (state === 'degraded') {
    text = "Some parts of TicketHawk aren't loading. We're working on it.";
  } else if (state === 'offline') {
    text = "You're offline. Changes won't save until you reconnect.";
  } else {
    text = "Can't reach TicketHawk right now. Changes you make may not save.";
  }

  const canRetry = state === 'down' || state === 'offline' || state === 'degraded';

  return (
    <div
      ref={ref}
      role="status"
      aria-live="polite"
      style={{
        // Static on purpose: no z-index, so modal scrims (position: fixed)
        // cover the banner like the rest of the page instead of it showing
        // through a dimmed dialog.
        background: `linear-gradient(${tone.tint}, ${tone.tint}), var(--bg)`,
        borderBottom: `1px solid ${tone.border}`,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '9px 20px' }}>
        <span
          aria-hidden="true"
          style={{ width: 8, height: 8, borderRadius: 99, background: tone.dot, flexShrink: 0 }}
        />

        <p style={{ flex: 1, fontSize: 13, lineHeight: 1.45, color: 'var(--text)' }}>
          {text}
          {detail && <span style={{ color: 'var(--text-2)' }}> {detail}</span>}
        </p>

        {canRetry && (
          <button
            type="button"
            onClick={() => { void probeNow(); }}
            disabled={checking}
            style={{
              flexShrink: 0,
              padding: '4px 12px',
              borderRadius: 99,
              border: '1px solid var(--border)',
              background: 'var(--surface-strong)',
              color: 'var(--text)',
              fontSize: 12,
              fontWeight: 600,
              cursor: checking ? 'default' : 'pointer',
              opacity: checking ? 0.6 : 1,
            }}
          >
            {checking ? 'Checking…' : 'Retry'}
          </button>
        )}

        {/* No Dismiss on "Back online" — it removes itself in a few seconds. */}
        {!showRecovered && (
        <button
          type="button"
          onClick={() => setDismissed(key)}
          aria-label="Dismiss"
          style={{
            flexShrink: 0,
            display: 'grid',
            placeItems: 'center',
            width: 26,
            height: 26,
            borderRadius: 99,
            border: 'none',
            background: 'transparent',
            color: 'var(--text-2)',
            cursor: 'pointer',
          }}
        >
          {/* Inline, not an icon import: events-admin has no icon library. */}
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
               strokeWidth="2.2" strokeLinecap="round" aria-hidden="true">
            <path d="M18 6 6 18M6 6l12 12" />
          </svg>
        </button>
        )}
      </div>
    </div>
  );
}