'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useAppTheme } from '@/themes/colors';
import { useStatusStore, type ServiceState } from '@/store/statusStore';

// ── ComingSoon ────────────────────────────────────────────────────────────────
// Cover shown on the home feed when nothing loaded. Purely presentational:
// page.tsx mounts it when nothing loaded and unmounts it the moment the first
// event comes back, so nothing here needs to know about the feed.
//
// "Nothing loaded" has two very different causes, and page.tsx cannot tell
// them apart — a failed /v1/home leaves the feed empty exactly like a
// successful one with zero events. The first cause is a launch message. The
// second must never be one: telling buyers "nothing is on sale just yet" while
// events ARE on sale and the API is simply unreachable is the worst possible
// outage copy. So the wording comes from the service status store — the same
// one the banner at the top of the page reads — and the two always agree.
//
// Recovery needs nothing extra: page.tsx already re-fetches every 45s while
// this cover is up, and the first successful fetch both fills the feed and
// clears the banner.

type Copy = { headline: string; accent: string; body: string; live: string; dot: string };

function copyFor(state: ServiceState, message: string): Copy {
  switch (state) {
    case 'maintenance':
      return {
        headline: 'Down for maintenance',
        accent: '.',
        body: message || "We're upgrading TicketHawk. Events will be back shortly — this page loads them on its own when we're done.",
        live: 'Retrying automatically',
        dot: '#FFB020',
      };
    case 'offline':
      return {
        headline: "You're offline",
        accent: '.',
        body: "Events can't load without a connection. Reconnect and this page fills in on its own.",
        live: 'Waiting for a connection',
        dot: '#8E8E93',
      };
    case 'down':
    case 'degraded':
      return {
        headline: "Can't load events right now",
        accent: '.',
        body: "TicketHawk isn't responding at the moment. This page will load events on its own as soon as it's back.",
        live: 'Retrying automatically',
        dot: '#FF453A',
      };
    default:
      return {
        headline: 'Events coming soon',
        accent: '!',
        body: 'Nothing is on sale just yet. The first events land here as soon as organisers open their doors — this page updates on its own the moment they do.',
        live: 'Watching for new events',
        dot: '#FF2D55',
      };
  }
}

export default function ComingSoon({ onRefresh }: { onRefresh?: () => Promise<void> | void }) {
  const { colors, isDark } = useAppTheme();
  const [refreshing, setRefreshing] = useState(false);

  const state   = useStatusStore((s) => s.state);
  const message = useStatusStore((s) => s.message);
  const copy    = copyFor(state, message);

  const border = isDark ? 'rgba(255,255,255,0.12)' : 'rgba(0,0,0,0.12)';

  const handleRefresh = async () => {
    if (refreshing || !onRefresh) return;
    setRefreshing(true);
    try { await onRefresh(); } finally { setRefreshing(false); }
  };

  return (
    <div
      className="flex flex-col items-center justify-center text-center px-6 py-20 md:py-28"
      style={{ minHeight: 'calc(100dvh - 200px)' }}
    >
      <style>{`
        @keyframes th-cs-pulse { 0%, 100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.3; transform: scale(0.75); } }
        .th-cs-dot { animation: th-cs-pulse 2s ease-in-out infinite; }
        @media (prefers-reduced-motion: reduce) { .th-cs-dot { animation: none; } }
      `}</style>

      <img src="/brand-logo_v2.png" alt="" style={{ width: 76, height: 76, objectFit: 'contain' }} />

      <h1
        className="mt-7 font-black uppercase"
        style={{ color: colors.text, fontSize: 'clamp(30px, 5.4vw, 54px)', lineHeight: 1.02, letterSpacing: '-1.2px' }}
      >
        {copy.headline}<span style={{ color: copy.dot }}>{copy.accent}</span>
      </h1>

      <p className="mt-5 text-[14px] leading-[23px] max-w-[42ch]" style={{ color: colors.subText }}>
        {copy.body}
      </p>

      {/* Live status */}
      <div className="mt-8 flex items-center gap-2.5">
        <span className="th-cs-dot w-[7px] h-[7px] rounded-full shrink-0" style={{ backgroundColor: copy.dot }} />
        <span className="text-[10.5px] font-extrabold tracking-[1.6px] uppercase" style={{ color: colors.subText }}>
          {copy.live}
        </span>
      </div>

      {/* Actions */}
      <div className="mt-7 flex flex-wrap items-center justify-center gap-3">
        <button
          onClick={handleRefresh}
          disabled={refreshing}
          className="h-[46px] px-7 rounded-[10px] text-[12px] font-extrabold tracking-[1.5px] uppercase text-white transition-opacity duration-150 disabled:opacity-60"
          style={{ backgroundColor: '#FF2D55' }}
        >
          {refreshing ? 'Checking…' : 'Check again'}
        </button>
        <Link
          href="/tickets"
          className="h-[46px] px-7 rounded-[10px] text-[12px] font-extrabold tracking-[1.5px] uppercase flex items-center border"
          style={{ borderColor: border, color: colors.text }}
        >
          My tickets
        </Link>
      </div>
    </div>
  );
}