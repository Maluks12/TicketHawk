'use client';

import { useState, useEffect } from 'react';
import { useRouter, useParams } from 'next/navigation';
import { api, type Event } from '@/lib/api';
import { useAppTheme } from '@/themes/colors';

// ── Helpers ───────────────────────────────────────────────────────────────────
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

// ── Page ──────────────────────────────────────────────────────────────────────
export default function EventDetailPage() {
  const router = useRouter();
  const { id: eventId } = useParams<{ id: string }>();
  const { colors, isDark } = useAppTheme();

  const [event,    setEvent]   = useState<Event | null>(null);
  const [loading,  setLoading] = useState(true);
  const [imgErr,   setImgErr]  = useState(false);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    if (!eventId) return;
    api
      .get(`/v1/events/${eventId}`)
      .then(({ data: res }) => setEvent(res.data ?? res))
      .catch(() => setEvent(null))
      .finally(() => setLoading(false));
    // Track view — fire and forget
    api.post(`/v1/events/${eventId}/view`).catch(() => {});
  }, [eventId]);

  const bg     = isDark ? '#0a0a0a' : '#f5f5f7';
  const cardBg = isDark ? '#111'    : '#fff';
  const border = isDark ? '#222'    : '#e5e5e5';

  // ── Loading ──────────────────────────────────────────────────────────────────
  if (loading) return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ width: 32, height: 32, border: '3px solid #ff2d55', borderTopColor: 'transparent', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  );

  // ── Not found ────────────────────────────────────────────────────────────────
  if (!event) return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 12, padding: 40 }}>
      <span style={{ fontSize: 40 }}>⚠️</span>
      <h2 style={{ color: colors.text, margin: 0 }}>Event not found</h2>
      <button onClick={() => router.back()} style={{ background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 12, padding: '12px 24px', fontWeight: 800, cursor: 'pointer' }}>Go back</button>
    </div>
  );

  const heroImage  = event.banner_image_url ?? event.cover_image_url;
  const isSoldOut  = event.is_sold_out ?? false;
  const tags       = [...(((event as any).teams) ?? []), ...(((event as any).performers) ?? [])];
  const desc       = event.description ?? '';
  const shortDesc  = desc.slice(0, 220);
  const hasMore    = desc.length > 220;

  return (
    <div style={{ minHeight: '100vh', background: bg }}>
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>

      {/* ── Sticky header ────────────────────────────────────────────────── */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '20px 16px 12px', borderBottom: `1px solid ${border}`, background: bg, position: 'sticky', top: 0, zIndex: 40 }}>
        <button
          onClick={() => router.back()}
          style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: 22, color: colors.text, padding: 0 }}
        >←</button>
        <h1 style={{ flex: 1, margin: 0, fontSize: 18, fontWeight: 800, color: colors.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          Event Details
        </h1>
        {event.is_featured && (
          <span style={{ fontSize: 10, fontWeight: 800, letterSpacing: 1, color: '#ff2d55', background: 'rgba(255,45,85,0.12)', padding: '3px 8px', borderRadius: 5 }}>
            🔥 TRENDING
          </span>
        )}
      </div>

      <div style={{ maxWidth: 640, margin: '0 auto', padding: '0 16px 120px' }}>

        {/* ── Hero image ───────────────────────────────────────────────────── */}
        <div style={{ margin: '16px 0', borderRadius: 16, overflow: 'hidden', position: 'relative', height: 220, background: '#111' }}>
          {heroImage && !imgErr ? (
            <img
              src={heroImage}
              alt={event.title}
              onError={() => setImgErr(true)}
              style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
            />
          ) : (
            <div style={{ width: '100%', height: '100%', background: 'linear-gradient(135deg,#1a1a1a 0%,#2a2a2a 100%)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <span style={{ fontSize: 48, opacity: 0.3 }}>🎟️</span>
            </div>
          )}

          {/* Gradient overlay */}
          <div style={{ position: 'absolute', inset: 0, background: 'linear-gradient(to top, rgba(0,0,0,0.7) 0%, transparent 55%)', pointerEvents: 'none' }} />

          {/* Sold out badge */}
          {isSoldOut && (
            <div style={{ position: 'absolute', top: 12, right: 12, background: '#ff2d55', color: '#fff', fontSize: 10, fontWeight: 800, letterSpacing: 1.5, padding: '4px 10px', borderRadius: 6 }}>
              SOLD OUT
            </div>
          )}

          {/* Views */}
          {event.views > 0 && (
            <div style={{ position: 'absolute', bottom: 12, left: 14, display: 'flex', alignItems: 'center', gap: 5 }}>
              <span style={{ fontSize: 12 }}>👁</span>
              <span style={{ color: 'rgba(255,255,255,0.85)', fontSize: 11, fontWeight: 700, letterSpacing: 1 }}>
                {event.views.toLocaleString('en-ZA')} VIEWS
              </span>
            </div>
          )}

          {/* Category pill */}
          <div style={{ position: 'absolute', bottom: 12, right: 14 }}>
            <span style={{ fontSize: 10, fontWeight: 800, letterSpacing: 1, color: '#ff2d55', background: 'rgba(0,0,0,0.6)', padding: '3px 9px', borderRadius: 5, backdropFilter: 'blur(6px)' }}>
              ✦ {event.category?.toUpperCase()}
            </span>
          </div>
        </div>

        {/* ── Title strip ──────────────────────────────────────────────────── */}
        <div style={{ padding: '4px 0 16px', borderBottom: `1px solid ${border}` }}>
          <p style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text, lineHeight: 1.25 }}>{event.title}</p>
          <p style={{ margin: '6px 0 0', fontSize: 13, color: colors.subText }}>
            {fmtDate(event.start_time)} · {fmtTime(event.start_time)}
          </p>
          {event.venue_name && (
            <p style={{ margin: '2px 0 0', fontSize: 13, color: colors.subText }}>
              {event.venue_name}{event.city ? `, ${event.city}` : ''}
            </p>
          )}
        </div>

        {/* ── Date / Venue / Price meta cards ──────────────────────────────── */}
        <div style={{ paddingTop: 16, paddingBottom: 4 }}>
          <p style={{ fontSize: 11, fontWeight: 800, letterSpacing: 1.4, color: colors.subText, margin: '0 0 12px' }}>EVENT INFO</p>

          {[
            { icon: '📅', label: 'Date',  value: fmtDate(event.start_time) },
            { icon: '🕐', label: 'Time',  value: fmtTime(event.start_time) },
            { icon: '📍', label: 'Venue', value: `${event.venue_name}${event.city ? `, ${event.city}` : ''}` },
            ...(event.address ? [{ icon: '🗺️', label: 'Address', value: event.address }] : []),
          ].map(({ icon, label, value }) => (
            <div key={label} style={{ display: 'flex', alignItems: 'center', gap: 14, padding: 14, marginBottom: 10, background: cardBg, border: `1px solid ${border}`, borderRadius: 14 }}>
              <div style={{ width: 36, height: 36, borderRadius: 10, background: isDark ? '#1e1e1e' : '#f0f0f0', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 18, flexShrink: 0 }}>
                {icon}
              </div>
              <div style={{ minWidth: 0 }}>
                <p style={{ margin: 0, fontSize: 10, fontWeight: 800, letterSpacing: 1.2, color: colors.subText }}>{label.toUpperCase()}</p>
                <p style={{ margin: '2px 0 0', fontSize: 13, fontWeight: 600, color: colors.text }}>{value}</p>
              </div>
            </div>
          ))}
        </div>

        {/* ── Price card ───────────────────────────────────────────────────── */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 14, padding: 14, marginBottom: 10, background: cardBg, border: `1px solid ${border}`, borderRadius: 14 }}>
          <div style={{ width: 36, height: 36, borderRadius: 10, background: isDark ? '#1e1e1e' : '#f0f0f0', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 18, flexShrink: 0 }}>
            🎟️
          </div>
          <div style={{ flex: 1 }}>
            <p style={{ margin: 0, fontSize: 10, fontWeight: 800, letterSpacing: 1.2, color: colors.subText }}>TICKETS FROM</p>
            <p style={{ margin: '2px 0 0', fontSize: 20, fontWeight: 800, color: colors.text }}>{fmtPrice(event.price)}</p>
          </div>
          {(event as any).available_tickets != null && !isSoldOut && (
            <p style={{ margin: 0, fontSize: 12, fontWeight: 700, color: (event as any).available_tickets < 20 ? '#ff9500' : colors.subText }}>
              {(event as any).available_tickets < 20
                ? `⚡ Only ${(event as any).available_tickets} left`
                : `${(event as any).available_tickets.toLocaleString()} available`}
            </p>
          )}
        </div>

        {/* ── Description ──────────────────────────────────────────────────── */}
        {desc && (
          <div style={{ marginBottom: 10 }}>
            <p style={{ fontSize: 11, fontWeight: 800, letterSpacing: 1.4, color: colors.subText, margin: '0 0 12px' }}>ABOUT THIS EVENT</p>
            <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 14, padding: 16 }}>
              <p style={{ margin: 0, fontSize: 14, color: colors.text, lineHeight: 1.7 }}>
                {expanded ? desc : (shortDesc + (hasMore ? '…' : ''))}
              </p>
              {hasMore && (
                <button
                  onClick={() => setExpanded(e => !e)}
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#ff2d55', fontSize: 13, fontWeight: 700, padding: '8px 0 0', display: 'block' }}
                >
                  {expanded ? 'Show less' : 'Read more'}
                </button>
              )}
            </div>
          </div>
        )}

        {/* ── Performers / Teams ───────────────────────────────────────────── */}
        {tags.length > 0 && (
          <div style={{ marginBottom: 10 }}>
            <p style={{ fontSize: 11, fontWeight: 800, letterSpacing: 1.4, color: colors.subText, margin: '0 0 12px' }}>
              {event.category?.toLowerCase() === 'sports' ? 'TEAMS' : 'PERFORMERS'}
            </p>
            <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 14, padding: 16, display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              {tags.map((tag: string) => (
                <span key={tag} style={{ fontSize: 13, fontWeight: 600, color: colors.text, background: isDark ? '#1e1e1e' : '#f0f0f0', borderRadius: 20, padding: '6px 14px' }}>
                  {tag}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* ── Trust badges (matching purchase page style) ───────────────── */}
        <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 14, padding: 16, display: 'flex', flexDirection: 'column', gap: 10, marginBottom: 10 }}>
          {[
            ['✅', 'Official Ticket Marketplace', 'Tickets are reviewed and verified by the venue.'],
            ['🔒', 'Secure Checkout', 'Your payment and personal data are always protected.'],
          ].map(([icon, title, body]) => (
            <div key={title} style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
              <span>{icon}</span>
              <div>
                <p style={{ margin: 0, fontSize: 13, fontWeight: 700, color: colors.text }}>{title}</p>
                <p style={{ margin: 0, fontSize: 12, color: colors.subText }}>{body}</p>
              </div>
            </div>
          ))}
        </div>

      </div>

      {/* ── Sticky CTA bar (same as purchase page) ───────────────────────── */}
      <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0, background: bg, borderTop: `1px solid ${border}`, padding: '14px 16px 28px', display: 'flex', alignItems: 'center', gap: 14, zIndex: 40 }}>
        <div style={{ maxWidth: 640, margin: '0 auto', display: 'flex', alignItems: 'center', gap: 14, width: '100%' }}>
          <div>
            <p style={{ margin: 0, fontSize: 11, color: colors.subText }}>From</p>
            <p style={{ margin: 0, fontSize: 18, fontWeight: 800, color: colors.text }}>{fmtPrice(event.price)}</p>
          </div>
          {isSoldOut ? (
            <button disabled style={{ flex: 1, background: isDark ? '#222' : '#e5e5e5', color: isDark ? '#555' : '#aaa', border: 'none', borderRadius: 14, padding: '14px', fontSize: 15, fontWeight: 800, cursor: 'not-allowed' }}>
              SOLD OUT
            </button>
          ) : (
            <button
              onClick={() => {
                // ── Set referral token so purchase page knows we came from here ──
                sessionStorage.setItem(`event_detail_visited_${eventId}`, '1');
                router.push(`/events/${eventId}/purchase`);
              }}
              style={{ flex: 1, background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 14, padding: '14px', fontSize: 15, fontWeight: 800, cursor: 'pointer' }}
            >
              Get Tickets
            </button>
          )}
        </div>
      </div>
    </div>
  );
}