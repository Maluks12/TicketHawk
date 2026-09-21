'use client';

import { useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { useAppTheme } from '@/themes/colors';
import { useStatusStore, startStatusMonitor, probeNow, type ServiceState } from '@/store/statusStore';

// ─────────────────────────────────────────────────────────────────────────────
// The banner every page shows when TicketHawk is not itself.
//
// Mounted once in the root layout, ABOVE <NavBar />, in normal document flow.
// That placement is deliberate: the nav is `sticky top-0`, so a banner in flow
// above it pushes the page down while you are at the top and then scrolls away,
// leaving the nav to stick to the top edge as it always did. Making the banner
// itself sticky would have both elements fighting for top: 0.
//
// It renders nothing at all when there is nothing to say, so it costs an empty
// <></> on every healthy page load.
//
// ── Colour ───────────────────────────────────────────────────────────────────
//
// themes/colors.ts has no semantic status colours (the consumer palette is
// black, white and the pink CTA), so the four tones live here. They are alpha
// tints, which composite correctly over both the black and the white theme
// background, and the TEXT comes from the theme — so this stays legible in both
// without a second set of values. The pink brand colour is deliberately not
// used for the error state: it is the "buy" colour everywhere else on the site.
// ─────────────────────────────────────────────────────────────────────────────

type Tone = { dot: string; bg: string; border: string };

const TONES: Record<Exclude<ServiceState, 'ok'> | 'recovered', Tone> = {
  maintenance: { dot: '#FFB020', bg: 'rgba(255,176,32,0.13)', border: 'rgba(255,176,32,0.32)' },
  degraded:    { dot: '#FFB020', bg: 'rgba(255,176,32,0.13)', border: 'rgba(255,176,32,0.32)' },
  down:        { dot: '#FF453A', bg: 'rgba(255,69,58,0.12)',  border: 'rgba(255,69,58,0.30)'  },
  offline:     { dot: '#8E8E93', bg: 'rgba(142,142,147,0.15)', border: 'rgba(142,142,147,0.30)' },
  recovered:   { dot: '#30D158', bg: 'rgba(48,209,88,0.13)',  border: 'rgba(48,209,88,0.30)'  },
};

/** "until 02:00" — rendered against the SERVER's clock, not the device's. */
function endsAtLabel(iso: string | null): string {
  if (!iso) return '';
  const t = new Date(iso);
  if (Number.isNaN(t.getTime()) || t.getTime() < Date.now()) return '';
  return `Until ${t.toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit', hour12: false })}.`;
}

export default function ServiceBanner() {
  const { colors } = useAppTheme();

  const state     = useStatusStore((s) => s.state);
  const message   = useStatusStore((s) => s.message);
  const endsAt    = useStatusStore((s) => s.endsAt);
  const revision  = useStatusStore((s) => s.revision);
  const checking  = useStatusStore((s) => s.checking);
  const recovered = useStatusStore((s) => s.recovered);

  // Dismissal is keyed to the NOTICE, not the component: dismissing today's
  // maintenance notice must not also silence tomorrow's outage.
  const [dismissed, setDismissed] = useState<string | null>(null);

  useEffect(() => { startStatusMonitor(); }, []);

  const showRecovered = state === 'ok' && recovered;
  if (state === 'ok' && !showRecovered) return null;

  const key = `${state}:${revision}`;
  if (!showRecovered && dismissed === key) return null;

  const tone = TONES[showRecovered ? 'recovered' : (state as Exclude<ServiceState, 'ok'>)];

  let text: string;
  let detail = '';

  if (showRecovered) {
    text = 'Back online.';
  } else if (state === 'maintenance') {
    text = message || "TicketHawk is under maintenance. Some things won't work until it's done.";
    detail = endsAtLabel(endsAt);
  } else if (state === 'degraded') {
    text = "Some parts of TicketHawk aren't loading. We're working on it.";
  } else if (state === 'offline') {
    text = "You're offline. Reconnect to keep browsing.";
  } else {
    text = "Can't reach TicketHawk right now. Pages and bookings may not load.";
  }

  const canRetry = state === 'down' || state === 'offline' || state === 'degraded';

  return (
    <div
      role="status"
      aria-live="polite"
      className="w-full"
      style={{ backgroundColor: tone.bg, borderBottom: `1px solid ${tone.border}` }}
    >
      <div className="flex items-center gap-3 px-4 md:px-6 py-2.5">
        <span
          aria-hidden="true"
          className="shrink-0 rounded-full"
          style={{ width: 8, height: 8, backgroundColor: tone.dot }}
        />

        <p className="flex-1 text-[13px] leading-snug" style={{ color: colors.text }}>
          {text}
          {detail && (
            <span style={{ color: colors.subText }}> {detail}</span>
          )}
        </p>

        {canRetry && (
          <button
            type="button"
            onClick={() => { void probeNow(); }}
            disabled={checking}
            className="shrink-0 rounded-full px-3 py-1 text-[12px] font-bold tracking-[0.3px] disabled:opacity-60"
            style={{ backgroundColor: colors.sectionCardBg, color: colors.text }}
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
            className="shrink-0 rounded-full p-1"
            style={{ color: colors.iconColor }}
          >
            <X size={15} />
          </button>
        )}
      </div>
    </div>
  );
}