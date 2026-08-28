'use client';

import { useState, useEffect } from 'react';
import { useRouter, useParams } from 'next/navigation';
import { api, toggleFollow, getFollowStatus, type Event, type EventInfoItem } from '@/lib/api';
import { useAppTheme } from '@/themes/colors';
import { useAuthStore } from '@/store/authStore';

// ── Information: the three summary fields ────────────────────────────────────
// Keys are catalogue keys, in the catalogue's display_order (10, 20, 30).
// Labels are hardcoded rather than read from the item because a field the
// organiser SKIPPED has no item to read a label from — which is the whole
// reason the N/A tile exists.
const SUMMARY_INFO_KEYS = [
  { key: 'age_requirement', label: 'Ages' },
  { key: 'doors_open',      label: 'Doors open' },
  { key: 'id_required',     label: 'ID required' },
] as const;

// Shown in a summary tile the organiser left blank.
const INFO_NOT_GIVEN = 'N/A';

function fmtPrice(p: number) {
  if (p === 0) return 'Free';
  return `R${Math.round(p).toLocaleString('en-ZA')}`;
}
function fmtDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
}
function fmtTime(iso: string) {
  return new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

/* ─── Inline SVG icons ─────────────────────────────────────────────────── */
const IconCalendar = () => (
  <svg width="15" height="15" viewBox="0 0 15 15" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
    <rect x="1.5" y="2.5" width="12" height="11" rx="2"/>
    <path d="M1.5 6h12M5 1.5v2M10 1.5v2"/>
  </svg>
);
const IconClock = () => (
  <svg width="15" height="15" viewBox="0 0 15 15" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
    <circle cx="7.5" cy="7.5" r="6"/>
    <path d="M7.5 4.5v3.25l2 2"/>
  </svg>
);
const IconPin = () => (
  <svg width="15" height="15" viewBox="0 0 15 15" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
    <path d="M7.5 1.5C5.29 1.5 3.5 3.29 3.5 5.5c0 3.25 4 8 4 8s4-4.75 4-8c0-2.21-1.79-4-4-4z"/>
    <circle cx="7.5" cy="5.5" r="1.5" fill="currentColor" stroke="none"/>
  </svg>
);
const IconMap = () => (
  <svg width="15" height="15" viewBox="0 0 15 15" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
    <polygon points="1.5,2 5.5,4 9.5,2 13.5,4 13.5,13 9.5,11 5.5,13 1.5,11"/>
    <line x1="5.5" y1="4" x2="5.5" y2="13"/>
    <line x1="9.5" y1="2" x2="9.5" y2="11"/>
  </svg>
);
const IconArrowLeft = () => (
  <svg width="13" height="13" viewBox="0 0 13 13" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M8.5 2L4 6.5l4.5 4.5"/>
  </svg>
);
const IconEye = () => (
  <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
    <path d="M.5 6C.5 6 2.5 2 6 2S11.5 6 11.5 6 9.5 10 6 10 .5 6 .5 6z"/>
    <circle cx="6" cy="6" r="1.75" fill="currentColor" stroke="none"/>
  </svg>
);
const IconShield = () => (
  <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="#22c55e" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
    <path d="M7 1.5L2 3.5v3.75C2 10.25 4.25 12 7 12.5c2.75-.5 5-2.25 5-5.25V3.5L7 1.5z"/>
    <path d="M4.5 7l2 2 3-3"/>
  </svg>
);
const IconLock = () => (
  <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="#f59e0b" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
    <rect x="2" y="6.5" width="10" height="6.5" rx="1.5"/>
    <path d="M4.5 6.5V4.5a2.5 2.5 0 015 0v2"/>
  </svg>
);
const IconExternalLink = () => (
  <svg width="11" height="11" viewBox="0 0 11 11" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
    <path d="M4.5 2H2a1 1 0 00-1 1v6a1 1 0 001 1h6a1 1 0 001-1V6.5"/>
    <path d="M7 1h3v3M10 1L5.5 5.5"/>
  </svg>
);
const IconHeart = ({ filled }: { filled?: boolean }) => (
  <svg width="15" height="15" viewBox="0 0 15 15" fill={filled ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
    <path d="M7.5 13S1.5 9.5 1.5 5.25C1.5 3.18 3.18 1.5 5.25 1.5c1.02 0 1.94.4 2.25 1.09.31-.69 1.23-1.09 2.25-1.09 2.07 0 3.75 1.68 3.75 3.75C13.5 9.5 7.5 13 7.5 13z"/>
  </svg>
);

/* ─── Component ─────────────────────────────────────────────────────────── */
/**
 * Build a map URL that opens the EXACT venue.
 *
 * Order matters:
 *  1. place_id  — Google's own id for the place the organiser picked. Exact.
 *  2. lat/lng   — exact coordinates, captured with the place.
 *  3. text      — last resort. This is what used to run for EVERY event, and
 *                 it's why buyers landed somewhere else: a fuzzy search for
 *                 "<venue>, <city>" lets Google guess, and a mismatched pair
 *                 like "FNB Stadium, Sandton" can resolve anywhere. The text
 *                 fallback now includes the street address, which it wrongly
 *                 omitted before.
 */
function buildMapsUrl(ev: { venue_name?: string; address?: string; city?: string; latitude?: number | null; longitude?: number | null; place_id?: string | null; }): string {
  const label = [ev.venue_name, ev.address, ev.city].filter(Boolean).join(', ');
  if (ev.place_id) {
    return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(label || 'venue')}&query_place_id=${encodeURIComponent(ev.place_id)}`;
  }
  if (typeof ev.latitude === 'number' && typeof ev.longitude === 'number') {
    return `https://www.google.com/maps/search/?api=1&query=${ev.latitude},${ev.longitude}`;
  }
  return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(label)}`;
}

export default function EventDetailPage() {
  const router = useRouter();
  const { id: eventId } = useParams<{ id: string }>();
  const { colors, isDark } = useAppTheme();

  const { user } = useAuthStore();

  const [event,    setEvent]   = useState<Event | null>(null);
  const [loading,  setLoading] = useState(true);
  const [imgErr,   setImgErr]  = useState(false);
  const [expanded, setExpanded] = useState(false);

  // ── Follow state ──────────────────────────────────────────────────────────
  const [isFollowing,   setIsFollowing]   = useState(false);
  const [followLoading, setFollowLoading] = useState(false);
  const [showSignIn,    setShowSignIn]    = useState(false);
  const [followErr,     setFollowErr]     = useState('');

  useEffect(() => {
    if (!eventId) return;
    api.get(`/v1/events/${eventId}`)
      .then(({ data: res }) => setEvent(res.data ?? res))
      .catch(() => setEvent(null))
      .finally(() => setLoading(false));
    api.post(`/v1/events/${eventId}/view`).catch(() => {});
  }, [eventId]);

  // ── Fetch current follow status once the event and a logged-in user are
  // both available, so the button always reflects reality (e.g. the person
  // followed this event on another device/app). Mirrors EventsApp exactly.
  useEffect(() => {
    if (!event?.id || !user) return;
    let cancelled = false;
    getFollowStatus(event.id)
      .then((res) => { if (!cancelled) setIsFollowing(res.following); })
      .catch(() => { /* default to not-following — no disruption to the UI */ });
    return () => { cancelled = true; };
  }, [event?.id, user]);

  /* Design tokens */
  const bg      = isDark ? '#0a0a0a' : '#f4f4f7';
  const cardBg  = isDark ? '#141414' : '#ffffff';
  const border  = isDark ? '#222222' : '#e6e6ea';
  const subtle  = isDark ? '#1c1c1c' : '#f0f0f4';
  const accent  = '#ff2d55';
  const hdrBg   = isDark ? 'rgba(10,10,10,0.88)' : 'rgba(244,244,247,0.88)';

  if (loading) return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ width: 26, height: 26, border: `2.5px solid ${accent}`, borderTopColor: 'transparent', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  );

  if (!event) return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 14, padding: 40 }}>
      <span style={{ fontSize: 32, opacity: 0.35 }}>🔍</span>
      <h2 style={{ color: colors.text, margin: 0, fontSize: 17, fontWeight: 600 }}>Event not found</h2>
      <button onClick={() => router.back()} style={{ background: accent, color: '#fff', border: 'none', borderRadius: 10, padding: '10px 20px', fontWeight: 700, cursor: 'pointer', fontSize: 14 }}>Go back</button>
    </div>
  );

  const heroImg   = event.banner_image_url ?? event.cover_image_url;
  const isSoldOut = event.is_sold_out ?? false;
  const tags      = [...(((event as any).teams) ?? []), ...(((event as any).performers) ?? [])];
  const desc      = event.description ?? '';
  const shortDesc = desc.slice(0, 280);
  const hasMore   = desc.length > 280;
  const avail     = (event as any).available_tickets;
  // available_tickets is not always a count. The API uses -1 as a SENTINEL for
  // "unlimited" (models.UnlimitedTickets), which is what an event with no
  // max_capacity and no ticket tiers returns -- a perfectly ordinary
  // configuration, not an edge case. Without the >= 0 guard, -1 < 20 is true,
  // so an event with UNLIMITED availability rendered the low-stock badge with
  // the sentinel printed straight into it: "Only -1 left".
  //
  // That is worse than a cosmetic glitch: it is false scarcity messaging shown
  // to a real buyer, which is exactly what the CPA prohibits.
  //
  // The tier-level readers in ./purchase already do this (`< 0 ? null : ...`);
  // this event-level one was simply missed.
  const isLow     = avail != null && avail >= 0 && avail < 20;
  const organiser = (event as any).organiser ?? (event as any).organizer ?? (event as any).hosted_by ?? (event as any).organisation ?? event.venue_name;

  const handleGetTickets = () => {
    sessionStorage.setItem(`event_detail_visited_${eventId}`, '1');
    router.push(`/events/${eventId}/purchase`);
  };

  // ── Toggle follow ─────────────────────────────────────────────────────────
  // Same flow as EventsApp: optimistic flip so the button snaps immediately,
  // reconcile with the server response, roll back + surface an error on
  // failure. Signed-out users are prompted to log in instead of firing the
  // request.
  const handleFollow = async () => {
    if (!user) { setShowSignIn(true); return; }
    if (!event?.id || followLoading) return;

    setIsFollowing((prev) => !prev);
    setFollowLoading(true);
    try {
      const res = await toggleFollow(event.id);
      setIsFollowing(res.following);
    } catch {
      setIsFollowing((prev) => !prev);
      setFollowErr('Could not update follow status. Please try again.');
    } finally {
      setFollowLoading(false);
    }
  };

  // ── Event information ───────────────────────────────────────────────────────
  // Only fields the organiser filled in have items, so the section is hidden
  // entirely when there are none. Age, doors and ID are lifted into summary
  // tiles; the rest keep their stated order and only appear when present.
  const evInfo: EventInfoItem[] = Array.isArray(event.info_items) ? event.info_items : [];
  const evInfoTiles = SUMMARY_INFO_KEYS.map(t => {
    const value = String(evInfo.find(i => i?.key === t.key)?.value ?? '').trim();
    return { key: t.key, label: t.label, value: value || INFO_NOT_GIVEN, given: !!value };
  });
  // Filtered by key, not position, so this stays correct if display_order moves.
  const evInfoRows = evInfo.filter(i => !SUMMARY_INFO_KEYS.some(t => t.key === i?.key));

  const venueStr  = `${event.venue_name ?? ''}${event.city ? `, ${event.city}` : ''}`;
  const mapsUrl   = buildMapsUrl(event);
  const infoItems = [
    { icon: <IconCalendar />, label: 'Date',    value: fmtDate(event.start_time), mapsUrl: null },
    { icon: <IconClock />,    label: 'Time',    value: fmtTime(event.start_time), mapsUrl: null },
    { icon: <IconPin />,      label: 'Venue',   value: venueStr,                  mapsUrl },
    ...(event.address ? [{ icon: <IconMap />, label: 'Address', value: event.address, mapsUrl }] : []),
  ];

  return (
    <div style={{ minHeight: '100vh', background: bg }}>
      <style>{`
        @keyframes spin { to { transform: rotate(360deg); } }
        * { box-sizing: border-box; }

        /* ── Header ── */
        .ed-header {
          position: sticky; top: 0; z-index: 50;
          display: flex; align-items: center; gap: 14px;
          padding: 0 40px; height: 54px;
          background: ${hdrBg};
          border-bottom: 1px solid ${border};
          backdrop-filter: blur(20px);
          -webkit-backdrop-filter: blur(20px);
        }
        .back-btn {
          width: 32px; height: 32px; border-radius: 50%;
          border: 1px solid ${border}; background: ${subtle};
          display: flex; align-items: center; justify-content: center;
          cursor: pointer; color: ${colors.text}; flex-shrink: 0;
          transition: background 0.12s;
        }
        .back-btn:hover { background: ${border}; }

        /* ── Hero ── */
        .ed-hero {
          position: relative; width: 100%;
          height: 440px; overflow: hidden; background: #111;
        }
        .hero-scrim {
          position: absolute; inset: 0;
          background: linear-gradient(to top,
            rgba(0,0,0,0.88) 0%,
            rgba(0,0,0,0.35) 50%,
            rgba(0,0,0,0.05) 100%);
          pointer-events: none;
        }
        .hero-foot {
          position: absolute; bottom: 0; left: 0; right: 0;
          padding: 0 40px 30px;
        }

        /* ── Body ── */
        .ed-body {
          padding: 24px 32px 80px;
          display: grid; grid-template-columns: 1fr 340px; gap: 20px;
          align-items: stretch;
        }
        .ed-left    { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
        .ed-sidebar { display: flex; flex-direction: column; }

        /* ── Shared card ── */
        .ed-card {
          background: ${cardBg};
          border: 1px solid ${border};
          border-radius: 16px;
          overflow: hidden;
        }

        /* ── Info grid ── */
        .info-grid { display: grid; grid-template-columns: 1fr 1fr; }
        .info-cell { display: flex; align-items: center; gap: 12px; padding: 16px 20px; }
        .info-grid > .info-cell:nth-child(odd) { border-right: 1px solid ${border}; }
        @media (max-width: 860px) {
          .info-grid > .info-cell:nth-child(odd) { border-right: none; }
        }
        .info-cell-link {
          text-decoration: none; cursor: pointer;
          transition: background 0.12s;
        }
        .info-cell-link:hover { background: ${subtle}; }
        .info-cell-link:hover .info-cell-arrow { opacity: 1; }
        .info-cell-arrow { opacity: 0; transition: opacity 0.12s; color: ${colors.subText}; flex-shrink: 0; margin-left: auto; }
        /* ── Event information ──
           Summary tiles carry the three questions asked before every other one:
           age, doors, ID. They are ALL-OR-NOTHING — once the organiser has
           filled in any information field, all three render, and one they
           skipped reads N/A. A strip that changed shape per event would leave
           "no ID line" ambiguous between "no ID needed" and "organiser did not
           say"; N/A says which.
           Three across on desktop, where the left column has the room. */
        .evinfo-tiles {
          display: grid; grid-template-columns: repeat(3, 1fr);
          gap: 8px; margin-bottom: 20px;
        }
        @media (max-width: 620px) { .evinfo-tiles { grid-template-columns: 1fr 1fr; } }
        .evinfo-tile { background: ${subtle}; border-radius: 12px; padding: 13px 14px; min-width: 0; }
        .evinfo-tile-lbl {
          margin: 0; font-size: 10px; font-weight: 700; letter-spacing: 0.9px;
          text-transform: uppercase; color: ${colors.subText};
        }
        .evinfo-tile-val { margin: 5px 0 0; font-size: 14px; font-weight: 600; line-height: 1.3; }
        /* Rows below repeat the About card's rhythm rather than the info-grid's
           dense cells: these values run to full sentences. */
        .evinfo-row { padding: 18px 0; border-bottom: 1px solid ${border}; }
        .evinfo-row:last-child { padding-bottom: 0; border-bottom: none; }
        .evinfo-row:first-of-type { padding-top: 0; }
        .evinfo-lbl {
          margin: 0; font-size: 10px; font-weight: 700; letter-spacing: 1px;
          text-transform: uppercase; color: ${colors.subText};
        }
        .evinfo-val { margin: 9px 0 0; font-size: 15px; line-height: 1.7; color: ${colors.text}; }

        .info-icon {
          width: 34px; height: 34px; border-radius: 9px;
          background: ${subtle}; display: flex; align-items: center;
          justify-content: center; flex-shrink: 0; color: ${accent};
        }

        /* ── Section labels ── */
        .sec-lbl {
          margin: 0 0 10px;
          font-size: 10px; font-weight: 700; letter-spacing: 1px;
          text-transform: uppercase; color: ${colors.subText};
        }

        /* ── Tag chips ── */
        .tag-chip {
          font-size: 12px; font-weight: 600; color: ${colors.text};
          background: ${subtle}; border: 1px solid ${border};
          border-radius: 20px; padding: 5px 13px;
        }

        /* ── Sidebar ── */
        .sb-price {
          padding: 22px 22px 18px;
          background: ${isDark ? 'rgba(255,45,85,0.08)' : 'rgba(255,45,85,0.04)'};
          border-bottom: 1px solid ${border};
        }
        .sb-org      { padding: 16px 22px; border-bottom: 1px solid ${border}; display: flex; align-items: center; gap: 11px; }
        .sb-cta      { padding: 14px 18px 10px; }
        .sb-payment  { padding: 0 18px 16px; }
        .sb-trust    { padding: 12px 22px; border-top: 1px solid ${border}; display: flex; gap: 18px; }
        .org-avatar {
          width: 38px; height: 38px; border-radius: 10px;
          background: ${subtle}; border: 1px solid ${border};
          display: flex; align-items: center; justify-content: center;
          font-size: 15px; flex-shrink: 0;
        }

        /* ── CTA ── */
        .get-tickets-btn {
          width: 100%; background: ${accent}; color: #fff;
          border: none; border-radius: 12px; padding: 14px;
          font-size: 15px; font-weight: 700; cursor: pointer; letter-spacing: 0.15px;
          transition: opacity 0.15s, transform 0.12s;
        }
        .get-tickets-btn:hover  { opacity: 0.87; transform: translateY(-1px); }
        .get-tickets-btn:active { opacity: 1; transform: none; }
        .sold-out-btn {
          width: 100%; background: ${subtle}; color: ${colors.subText};
          border: 1px solid ${border}; border-radius: 12px; padding: 14px;
          font-size: 14px; font-weight: 700; cursor: not-allowed;
        }

        /* ── Follow (desktop: full-width button under Get Tickets,
             matching EventsApp's outline FOLLOW button 1:1 in behaviour) ── */
        .follow-btn {
          width: 100%; margin-top: 8px;
          display: flex; align-items: center; justify-content: center; gap: 7px;
          background: ${subtle}; color: ${colors.text};
          border: 1px solid ${border}; border-radius: 12px; padding: 12px;
          font-size: 13px; font-weight: 700; letter-spacing: 0.2px; cursor: pointer;
          transition: background 0.15s, border-color 0.15s, color 0.15s;
        }
        .follow-btn:hover  { background: ${border}; }
        .follow-btn:disabled { cursor: default; opacity: 0.75; }
        .follow-btn.is-following {
          background: ${isDark ? 'rgba(181,123,238,0.12)' : 'rgba(181,123,238,0.08)'};
          border-color: #b57bee; color: #b57bee;
        }

        /* ── Follow (mobile: icon-only circular button — adapted for the
             fixed bottom bar, which has no room for the full text label
             once price + Get Tickets are in it) ── */
        .follow-btn-mobile {
          width: 44px; height: 44px; border-radius: 50%; flex-shrink: 0;
          display: flex; align-items: center; justify-content: center;
          background: ${subtle}; border: 1px solid ${border}; color: ${colors.text};
          cursor: pointer; transition: background 0.15s, border-color 0.15s, color 0.15s;
        }
        .follow-btn-mobile:disabled { cursor: default; opacity: 0.75; }
        .follow-btn-mobile.is-following {
          background: ${isDark ? 'rgba(181,123,238,0.14)' : 'rgba(181,123,238,0.08)'};
          border-color: #b57bee; color: #b57bee;
        }

        /* ── Trust items ── */
        .trust-item  { display: flex; align-items: center; gap: 6px; }
        .trust-txt   { font-size: 11px; font-weight: 600; color: ${colors.subText}; }
        .trust-row   { display: flex; align-items: center; gap: 13px; padding: 14px 20px; }

        /* ── Mobile CTA ── */
        .ed-mobile-cta { display: none; }

        /* ── Responsive ── */
        @media (max-width: 860px) {
          .ed-header  { padding: 0 20px; height: 50px; }
          .ed-hero    { height: 300px; }
          .hero-foot  { padding: 0 20px 22px; }
          .ed-body    { grid-template-columns: 1fr; padding: 16px 20px 120px; gap: 10px; }
          .info-grid  { grid-template-columns: 1fr; }
          .ed-sidebar { display: none; }
          .ed-mobile-cta {
            display: flex; position: fixed; bottom: 0; left: 0; right: 0;
            background: ${hdrBg}; border-top: 1px solid ${border};
            backdrop-filter: blur(20px); -webkit-backdrop-filter: blur(20px);
            padding: 12px 20px calc(12px + env(safe-area-inset-bottom, 0px));
            align-items: center; gap: 12px; z-index: 40;
          }
        }
      `}</style>

      {/* ── Header ─────────────────────────────────────────────────────────── */}
      <header className="ed-header">
        <button className="back-btn" onClick={() => router.back()} aria-label="Back">
          <IconArrowLeft />
        </button>
        <span style={{ flex: 1, fontSize: 14, fontWeight: 600, color: colors.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {event.title}
        </span>
        {event.is_featured && (
          <span style={{ fontSize: 10, fontWeight: 700, letterSpacing: 0.6, color: accent, background: isDark ? 'rgba(255,45,85,0.12)' : 'rgba(255,45,85,0.08)', padding: '4px 9px', borderRadius: 20, flexShrink: 0 }}>
            TRENDING
          </span>
        )}
      </header>

      {/* ── Hero ───────────────────────────────────────────────────────────── */}
      <div className="ed-hero">
        {heroImg && !imgErr ? (
          <img src={heroImg} alt={event.title} onError={() => setImgErr(true)}
            style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }} />
        ) : (
          <div style={{
            width: '100%', height: '100%',
            background: isDark
              ? 'linear-gradient(150deg, #1a070f 0%, #1c1c1f 50%, #0d1320 100%)'
              : 'linear-gradient(150deg, #f5e5ea 0%, #eeeef2 50%, #e5eaf5 100%)',
          }} />
        )}
        <div className="hero-scrim" />

        {/* Top-right badges */}
        <div style={{ position: 'absolute', top: 18, right: 20, display: 'flex', gap: 8 }}>
          {event.category && (
            <span style={{ fontSize: 10, fontWeight: 700, letterSpacing: 0.7, color: '#fff', background: 'rgba(255,45,85,0.9)', backdropFilter: 'blur(8px)', padding: '5px 10px', borderRadius: 20 }}>
              {event.category.toUpperCase()}
            </span>
          )}
          {isSoldOut && (
            <span style={{ fontSize: 10, fontWeight: 700, letterSpacing: 0.7, color: '#fff', background: 'rgba(0,0,0,0.65)', backdropFilter: 'blur(8px)', padding: '5px 10px', borderRadius: 20 }}>
              SOLD OUT
            </span>
          )}
        </div>

        {/* Bottom: views + title */}
        <div className="hero-foot">
          {event.views > 0 && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 5, marginBottom: 10, color: 'rgba(255,255,255,0.55)' }}>
              <IconEye />
              <span style={{ fontSize: 11, fontWeight: 500 }}>{event.views.toLocaleString('en-ZA')} views</span>
            </div>
          )}
          <h1 style={{ margin: 0, fontSize: 28, fontWeight: 800, color: '#fff', lineHeight: 1.2, letterSpacing: -0.5, maxWidth: 700 }}>
            {event.title}
          </h1>
        </div>
      </div>

      {/* ── Body ───────────────────────────────────────────────────────────── */}
      <div className="ed-body">

        {/* Left column ────────────────────────────────────── */}
        <div className="ed-left">

          {/* Info grid */}
          <div className="ed-card">
            <div className="info-grid">
              {infoItems.map(({ icon, label, value, mapsUrl }, i) => {
                const isLastRow = Math.floor(i / 2) === Math.ceil(infoItems.length / 2) - 1;
                const cellStyle = {
                  borderBottom: !isLastRow ? `1px solid ${border}` : 'none',
                };
                const inner = (
                  <>
                    <div className="info-icon">{icon}</div>
                    <div style={{ minWidth: 0, flex: 1 }}>
                      <p style={{ margin: 0, fontSize: 10, fontWeight: 700, letterSpacing: 0.9, textTransform: 'uppercase', color: colors.subText }}>{label}</p>
                      <p style={{ margin: '3px 0 0', fontSize: 13, fontWeight: 600, color: colors.text, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{value}</p>
                    </div>
                    {mapsUrl && <span className="info-cell-arrow"><IconExternalLink /></span>}
                  </>
                );
                return mapsUrl ? (
                  <a key={label} href={mapsUrl} target="_blank" rel="noopener noreferrer"
                    className="info-cell info-cell-link" style={cellStyle}>
                    {inner}
                  </a>
                ) : (
                  <div key={label} className="info-cell" style={cellStyle}>
                    {inner}
                  </div>
                );
              })}
            </div>
          </div>

          {/* About */}
          {desc && (
            <div className="ed-card" style={{ padding: '20px 22px' }}>
              <p className="sec-lbl">About this event</p>
              <p style={{ margin: 0, fontSize: 15, color: colors.text, lineHeight: 1.75 }}>
                {expanded ? desc : shortDesc + (hasMore ? '…' : '')}
              </p>
              {hasMore && (
                <button onClick={() => setExpanded(e => !e)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: accent, fontSize: 13, fontWeight: 700, padding: '10px 0 0', display: 'block' }}>
                  {expanded ? 'Show less' : 'Read more'}
                </button>
              )}
            </div>
          )}

          {/* Event information */}
          {evInfo.length > 0 && (
            <div className="ed-card" style={{ padding: '20px 22px' }}>
              <p className="sec-lbl">Information</p>

              <div className="evinfo-tiles">
                {evInfoTiles.map(t => (
                  <div key={t.key} className="evinfo-tile">
                    <p className="evinfo-tile-lbl">{t.label}</p>
                    {/* N/A is dimmed to subText so it reads as a gap the
                        organiser left, not an answer they gave. */}
                    <p className="evinfo-tile-val" style={{ color: t.given ? colors.text : colors.subText }}>
                      {t.value}
                    </p>
                  </div>
                ))}
              </div>

              {evInfoRows.map(item => (
                <div key={item.key} className="evinfo-row">
                  <p className="evinfo-lbl">{item.label}</p>
                  <p className="evinfo-val">{item.value}</p>
                </div>
              ))}
            </div>
          )}

          {/* Performers / Teams */}
          {tags.length > 0 && (
            <div className="ed-card" style={{ padding: '20px 22px' }}>
              <p className="sec-lbl">{event.category?.toLowerCase() === 'sports' ? 'Teams' : 'Performers'}</p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 7 }}>
                {tags.map((tag: string) => <span key={tag} className="tag-chip">{tag}</span>)}
              </div>
            </div>
          )}

          {/* Trust */}
          <div className="ed-card">
            {[
              { icon: <IconShield />, label: 'Official Ticket Marketplace', sub: 'Reviewed and verified by the venue' },
              { icon: <IconLock />,   label: 'Secure Checkout',             sub: 'Your payment and personal data are protected' },
            ].map(({ icon, label, sub }, i) => (
              <div key={label} className="trust-row" style={{ borderTop: i > 0 ? `1px solid ${border}` : 'none' }}>
                <div style={{ width: 32, height: 32, borderRadius: 9, background: subtle, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                  {icon}
                </div>
                <div>
                  <p style={{ margin: 0, fontSize: 13, fontWeight: 600, color: colors.text }}>{label}</p>
                  <p style={{ margin: '2px 0 0', fontSize: 12, color: colors.subText }}>{sub}</p>
                </div>
              </div>
            ))}
          </div>

        </div>{/* /ed-left */}

        {/* Sidebar ─────────────────────────────────────────── */}
        <div className="ed-sidebar">
          <div className="ed-card" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>

            {/* Price */}
            <div className="sb-price">
              <p style={{ margin: '0 0 2px', fontSize: 10, fontWeight: 700, letterSpacing: 1, textTransform: 'uppercase', color: isDark ? 'rgba(255,255,255,0.38)' : 'rgba(0,0,0,0.38)' }}>
                Tickets from
              </p>
              <p style={{ margin: 0, fontSize: 38, fontWeight: 800, color: colors.text, letterSpacing: -1.5, lineHeight: 1.1 }}>
                {fmtPrice(event.price)}
              </p>
              {/* avail >= 0, not just != null: -1 means unlimited, and the
                  else-branch below would otherwise print "-1 available". An
                  unlimited event shows no count, which is right -- there is no
                  number worth telling the buyer. */}
              {avail != null && avail >= 0 && !isSoldOut && (
                <p style={{ margin: '6px 0 0', fontSize: 12, fontWeight: 600, color: isLow ? '#ff9500' : colors.subText }}>
                  {isLow ? `⚡ Only ${avail} left` : `${avail.toLocaleString()} available`}
                </p>
              )}
            </div>

            {/* Organiser */}
            {organiser && (
              <div className="sb-org">
                <div className="org-avatar">🏢</div>
                <div>
                  <p style={{ margin: 0, fontSize: 10, fontWeight: 700, letterSpacing: 0.8, color: colors.subText, textTransform: 'uppercase' }}>Organised by</p>
                  <p style={{ margin: '2px 0 0', fontSize: 14, fontWeight: 700, color: colors.text }}>{organiser}</p>
                </div>
              </div>
            )}

            {/* Spacer */}
            <div style={{ flex: 1, minHeight: 20 }} />

            {/* CTA */}
            <div className="sb-cta">
              {isSoldOut
                ? <button disabled className="sold-out-btn">Sold Out</button>
                : <button className="get-tickets-btn" onClick={handleGetTickets}>Get Tickets</button>
              }
              <button
                className={`follow-btn${isFollowing ? ' is-following' : ''}`}
                onClick={handleFollow}
                disabled={followLoading}
              >
                <IconHeart filled={isFollowing} />
                {followLoading ? 'Updating…' : isFollowing ? 'Following' : 'Follow'}
              </button>
            </div>

            {/* Payment methods */}
            <div className="sb-payment">
              <p style={{ margin: '0 0 10px', fontSize: 10, fontWeight: 700, letterSpacing: 0.9, textTransform: 'uppercase', color: colors.subText }}>
                Accepted payment methods
              </p>
              <div style={{ display: 'flex', alignItems: 'center', gap: 7, flexWrap: 'wrap' }}>
                {/* Visa */}
                <div style={{ background: '#1a1f71', borderRadius: 5, padding: '6px 9px', display: 'flex', alignItems: 'center' }}>
                  <svg width="42" height="13.5" viewBox="0 8.124 24 7.751" role="img" aria-label="Visa">
                    <path fill="#fff" d="M9.112 8.262L5.97 15.758H3.92L2.374 9.775c-.094-.368-.175-.503-.461-.658C1.447 8.864.677 8.627 0 8.479l.046-.217h3.3a.904.904 0 01.894.764l.817 4.338 2.018-5.102zm8.033 5.049c.008-1.979-2.736-2.088-2.717-2.972.006-.269.262-.555.822-.628a3.66 3.66 0 011.913.336l.34-1.59a5.207 5.207 0 00-1.814-.333c-1.917 0-3.266 1.02-3.278 2.479-.012 1.079.963 1.68 1.698 2.04.756.367 1.01.603 1.006.931-.005.504-.602.725-1.16.734-.975.015-1.54-.263-1.992-.473l-.351 1.642c.453.208 1.289.39 2.156.398 2.037 0 3.37-1.006 3.377-2.564m5.061 2.447H24l-1.565-7.496h-1.656a.883.883 0 00-.826.55l-2.909 6.946h2.036l.405-1.12h2.488zm-2.163-2.656l1.02-2.815.588 2.815zm-8.16-4.84l-1.603 7.496H8.34l1.605-7.496z"/>
                  </svg>
                </div>
                {/* Mastercard */}
                <div style={{ background: isDark ? '#2a2a2a' : '#f0f0f0', borderRadius: 5, padding: '6px 8px', display: 'flex', alignItems: 'center' }}>
                  <div style={{ width: 15, height: 15, borderRadius: '50%', background: '#eb001b', flexShrink: 0 }} />
                  <div style={{ width: 15, height: 15, borderRadius: '50%', background: '#f79e1b', flexShrink: 0, marginLeft: -6 }} />
                </div>
                {/* Amex */}
                <div style={{ background: '#2e77bc', borderRadius: 5, padding: '6px 8px', display: 'flex', alignItems: 'center' }}>
                  <span style={{ fontSize: 11, fontWeight: 800, color: '#fff', letterSpacing: 0.6 }}>AMEX</span>
                </div>
                {/* Ozow */}
                <div style={{ background: '#1e2330', borderRadius: 5, padding: '5px 9px', display: 'flex', alignItems: 'center', gap: 4 }}>
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none">
                    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 15.5c-3.04 0-5.5-2.46-5.5-5.5S8.96 6.5 12 6.5s5.5 2.46 5.5 5.5-2.46 5.5-5.5 5.5z" fill="#00e587"/>
                  </svg>
                  <span style={{ fontSize: 11, fontWeight: 800, color: '#fff', letterSpacing: 0.2 }}>Ozow</span>
                </div>
              </div>
            </div>

            {/* Trust footer */}
            <div className="sb-trust">
              <div className="trust-item"><IconShield /><span className="trust-txt">Official</span></div>
              <div className="trust-item"><IconLock /><span className="trust-txt">Secure</span></div>
            </div>

          </div>
        </div>{/* /ed-sidebar */}

      </div>

      {/* ── Mobile CTA ─────────────────────────────────────────────────────── */}
      <div className="ed-mobile-cta">
        <div>
          <p style={{ margin: 0, fontSize: 10, color: colors.subText, textTransform: 'uppercase', letterSpacing: 0.8, fontWeight: 600 }}>From</p>
          <p style={{ margin: 0, fontSize: 19, fontWeight: 800, color: colors.text, letterSpacing: -0.5 }}>{fmtPrice(event.price)}</p>
        </div>
        {/* Icon-only on mobile — same follow logic as the desktop button,
            just adapted to fit the fixed bar alongside price + Get Tickets. */}
        <button
          className={`follow-btn-mobile${isFollowing ? ' is-following' : ''}`}
          onClick={handleFollow}
          disabled={followLoading}
          aria-label={isFollowing ? 'Following this event' : 'Follow this event'}
        >
          <IconHeart filled={isFollowing} />
        </button>
        {isSoldOut
          ? <button disabled className="sold-out-btn" style={{ flex: 1 }}>Sold Out</button>
          : <button className="get-tickets-btn" style={{ flex: 1 }} onClick={handleGetTickets}>Get Tickets</button>
        }
      </div>

      {/* ── Sign-in prompt (unauthenticated Follow tap) ───────────────────────
           Same trigger as EventsApp's Alert.alert('Sign in required', …) —
           adapted to a dismissible web toast with a direct Log in action
           instead of a native alert dialog. */}
      {showSignIn && (
        <div
          style={{
            position: 'fixed', top: 16, left: 16, right: 16, zIndex: 200,
            maxWidth: 520, margin: '0 auto',
            display: 'flex', alignItems: 'center', gap: 10,
            background: cardBg, border: `1px solid ${border}`,
            borderRadius: 12, padding: '12px 14px',
            boxShadow: '0 8px 24px rgba(0,0,0,0.35)',
          }}
          role="alert"
        >
          <div style={{ color: accent, flexShrink: 0 }}><IconHeart /></div>
          <p style={{ flex: 1, margin: 0, fontSize: 13, fontWeight: 600, color: colors.text, lineHeight: 1.4 }}>
            Log in or sign up to follow events and get reminders.
          </p>
          <button
            onClick={() => router.push('/auth/login')}
            style={{ background: accent, color: '#fff', border: 'none', borderRadius: 8, padding: '8px 14px', fontSize: 12, fontWeight: 700, cursor: 'pointer', flexShrink: 0 }}
          >
            Log in
          </button>
          <button
            onClick={() => setShowSignIn(false)}
            aria-label="Dismiss"
            style={{ background: 'none', border: 'none', padding: 2, cursor: 'pointer', flexShrink: 0, color: colors.subText, fontSize: 14 }}
          >
            ✕
          </button>
        </div>
      )}

      {/* ── Follow error toast — same visual pattern as the purchase page's
           error toast, reused here so failures don't rely on a native alert. */}
      {followErr && (
        <div
          style={{
            position: 'fixed', top: 16, left: 16, right: 16, zIndex: 200,
            maxWidth: 520, margin: '0 auto',
            display: 'flex', alignItems: 'center', gap: 10,
            background: '#2a0e14', border: '1px solid #ff3c6e',
            borderRadius: 12, padding: '12px 14px',
            boxShadow: '0 8px 24px rgba(0,0,0,0.35)',
          }}
          role="alert"
        >
          <p style={{ flex: 1, margin: 0, fontSize: 13, fontWeight: 600, color: '#fff', lineHeight: 1.4 }}>{followErr}</p>
          <button
            onClick={() => setFollowErr('')}
            aria-label="Dismiss"
            style={{ background: 'none', border: 'none', padding: 2, cursor: 'pointer', flexShrink: 0 }}
          >
            <span style={{ color: '#ffb3c2' }}>✕</span>
          </button>
        </div>
      )}
    </div>
  );
}