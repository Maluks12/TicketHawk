'use client';

import { useState, useEffect } from 'react';
import { useRouter, useParams } from 'next/navigation';
import { api, type Event } from '@/lib/api';
import { useAppTheme } from '@/themes/colors';

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

export default function EventDetailPage() {
  const router   = useRouter();
  const { id: eventId } = useParams<{ id: string }>();
  const { colors, isDark } = useAppTheme();

  const [event,    setEvent]   = useState<Event | null>(null);
  const [loading,  setLoading] = useState(true);
  const [imgErr,   setImgErr]  = useState(false);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    if (!eventId) return;
    api.get(`/v1/events/${eventId}`)
      .then(({ data: res }) => setEvent(res.data ?? res))
      .catch(() => setEvent(null))
      .finally(() => setLoading(false));
    api.post(`/v1/events/${eventId}/view`).catch(() => {});
  }, [eventId]);

  const bg      = isDark ? '#0a0a0a' : '#f2f2f5';
  const cardBg  = isDark ? '#161616' : '#fff';
  const border  = isDark ? '#242424' : '#e8e8ec';
  const subtle  = isDark ? '#1e1e1e' : '#f0f0f3';

  if (loading) return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ width: 32, height: 32, border: '3px solid #ff2d55', borderTopColor: 'transparent', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  );

  if (!event) return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 12, padding: 40 }}>
      <span style={{ fontSize: 40 }}>⚠️</span>
      <h2 style={{ color: colors.text, margin: 0 }}>Event not found</h2>
      <button onClick={() => router.back()} style={{ background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 12, padding: '12px 24px', fontWeight: 800, cursor: 'pointer' }}>Go back</button>
    </div>
  );

  const heroImage = event.banner_image_url ?? event.cover_image_url;
  const isSoldOut = event.is_sold_out ?? false;
  const tags      = [...(((event as any).teams) ?? []), ...(((event as any).performers) ?? [])];
  const desc      = event.description ?? '';
  const shortDesc = desc.slice(0, 280);
  const hasMore   = desc.length > 280;
  const avail     = (event as any).available_tickets;
  const isLow     = avail != null && avail < 20;

  const handleGetTickets = () => {
    sessionStorage.setItem(`event_detail_visited_${eventId}`, '1');
    router.push(`/events/${eventId}/purchase`);
  };

  return (
    <div style={{ minHeight: '100vh', background: bg }}>
      <style>{`
        @keyframes spin { to { transform: rotate(360deg); } }
        * { box-sizing: border-box; }

        .ed-header {
          display: flex; align-items: center; gap: 12px;
          padding: 0 32px; height: 60px;
          background: ${bg}; border-bottom: 1px solid ${border};
          position: sticky; top: 0; z-index: 40;
          backdrop-filter: blur(12px);
        }
        .ed-hero {
          width: 100%; aspect-ratio: 21/9;
          min-height: 240px; max-height: 460px;
          position: relative; background: #111; overflow: hidden;
        }
        .ed-body {
          max-width: 1320px; margin: 0 auto;
          padding: 28px 40px 80px;
          display: grid; grid-template-columns: 1fr 400px; gap: 0 28px; align-items: start;
        }
        .ed-top-row {
          grid-column: 1 / -1;
          display: grid; grid-template-columns: 1fr 400px; gap: 28px; align-items: start;
          margin-bottom: 0;
        }
        .ed-top-left { min-width: 0; display: flex; flex-direction: column; gap: 10px; }
        .ed-bottom-left { grid-column: 1; display: flex; flex-direction: column; gap: 16px; margin-top: 10px; }
        .ed-sidebar { display: flex; flex-direction: column; }
        .ed-sidebar > div { display: flex; flex-direction: column; }
        .ed-mobile-cta { display: none; }

        .info-row { display: flex; align-items: center; gap: 10px; }
        .info-row + .info-row { border-top: 1px solid ${border}; padding-top: 14px; margin-top: 0; }

        .tag-chip {
          font-size: 13px; font-weight: 600; color: ${colors.text};
          background: ${subtle}; border-radius: 20px; padding: 6px 14px;
        }

        .get-tickets-btn {
          width: 100%; background: #ff2d55; color: #fff;
          border: none; border-radius: 14px; padding: 17px;
          font-size: 16px; font-weight: 800; cursor: pointer;
          transition: opacity 0.15s, transform 0.1s;
          letter-spacing: 0.3px;
        }
        .get-tickets-btn:hover { opacity: 0.9; transform: translateY(-1px); }
        .get-tickets-btn:active { opacity: 1; transform: translateY(0); }

        @media (max-width: 900px) {
          .ed-header { padding: 0 16px; }
          .ed-hero { aspect-ratio: 16/9; max-height: 240px; }
          .ed-body { grid-template-columns: 1fr; padding: 20px 16px 120px; gap: 16px; }
          .ed-top-row { grid-template-columns: 1fr; gap: 16px; }
          .ed-sidebar { display: none; }
          .ed-bottom-left { grid-column: 1; }
          .ed-mobile-cta {
            display: flex; position: fixed; bottom: 0; left: 0; right: 0;
            background: ${bg}; border-top: 1px solid ${border};
            padding: 12px 16px 28px; align-items: center; gap: 14px; z-index: 40;
          }
        }
      `}</style>

      {/* ── Header ─────────────────────────────────────────────────────────── */}
      <div className="ed-header">
        <button
          onClick={() => router.back()}
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: colors.text, padding: '6px 8px 6px 0', fontSize: 20, lineHeight: 1, flexShrink: 0 }}
        >←</button>
        <span style={{ flex: 1, fontSize: 15, fontWeight: 700, color: colors.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {event.title}
        </span>
        {event.is_featured && (
          <span style={{ fontSize: 10, fontWeight: 800, letterSpacing: 1, color: '#ff2d55', background: 'rgba(255,45,85,0.1)', padding: '4px 10px', borderRadius: 20, flexShrink: 0 }}>
            🔥 TRENDING
          </span>
        )}
      </div>

      {/* ── Hero ───────────────────────────────────────────────────────────── */}
      <div className="ed-hero">
        {heroImage && !imgErr ? (
          <img src={heroImage} alt={event.title} onError={() => setImgErr(true)}
            style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }} />
        ) : (
          <div style={{ width: '100%', height: '100%', background: 'linear-gradient(135deg,#1c1c1e,#2c2c2e)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <span style={{ fontSize: 64, opacity: 0.2 }}>🎟️</span>
          </div>
        )}
        <div style={{ position: 'absolute', inset: 0, background: 'linear-gradient(to top, rgba(0,0,0,0.6) 0%, transparent 55%)', pointerEvents: 'none' }} />

        {isSoldOut && (
          <div style={{ position: 'absolute', top: 16, right: 16, background: '#ff2d55', color: '#fff', fontSize: 11, fontWeight: 800, letterSpacing: 1.5, padding: '5px 12px', borderRadius: 6 }}>
            SOLD OUT
          </div>
        )}
        {event.views > 0 && (
          <div style={{ position: 'absolute', bottom: 16, left: 20, display: 'flex', alignItems: 'center', gap: 5 }}>
            <span style={{ fontSize: 12 }}>👁</span>
            <span style={{ color: 'rgba(255,255,255,0.8)', fontSize: 11, fontWeight: 700, letterSpacing: 0.8 }}>
              {event.views.toLocaleString('en-ZA')} views
            </span>
          </div>
        )}
        {event.category && (
          <div style={{ position: 'absolute', bottom: 16, right: 20, fontSize: 11, fontWeight: 700, letterSpacing: 0.8, color: '#fff', background: 'rgba(255,45,85,0.85)', padding: '4px 10px', borderRadius: 20, backdropFilter: 'blur(8px)' }}>
            {event.category}
          </div>
        )}
      </div>

      {/* ── Body ───────────────────────────────────────────────────────────── */}
      <div className="ed-body">

        {/* ══ TOP ROW: info grid + sidebar side by side, matched height ══ */}
        <div className="ed-top-row">
          <div className="ed-top-left">

            {/* Title block */}
            <div>
              <h1 style={{ margin: 0, fontSize: 26, fontWeight: 800, color: colors.text, lineHeight: 1.25 }}>
                {event.title}
              </h1>
            </div>

            {/* Event details — 2×2 grid */}
          {(() => {
            const infoItems = [
              { icon: '📅', label: 'Date',    value: fmtDate(event.start_time) },
              { icon: '🕐', label: 'Time',    value: fmtTime(event.start_time) },
              { icon: '📍', label: 'Venue',   value: `${event.venue_name ?? ''}${event.city ? `, ${event.city}` : ''}` },
              ...(event.address ? [{ icon: '🗺️', label: 'Address', value: event.address }] : []),
            ];
            return (
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 0, background: cardBg, borderRadius: 16, border: `1px solid ${border}`, overflow: 'hidden' }}>
                {infoItems.map(({ icon, label, value }, i) => {
                  const isLeft   = i % 2 === 0;
                  const isLastRow = i >= infoItems.length - (infoItems.length % 2 === 0 ? 2 : 1);
                  return (
                    <div key={label} style={{
                      display: 'flex', alignItems: 'center', gap: 14,
                      padding: '13px 16px',
                      borderRight:  isLeft ? `1px solid ${border}` : 'none',
                      borderBottom: !isLastRow ? `1px solid ${border}` : 'none',
                      alignSelf: 'stretch',
                    }}>
                      <div style={{ width: 36, height: 36, borderRadius: 10, background: subtle, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 17, flexShrink: 0 }}>
                        {icon}
                      </div>
                      <div style={{ minWidth: 0 }}>
                        <p style={{ margin: 0, fontSize: 10, fontWeight: 700, letterSpacing: 1.1, color: colors.subText, textTransform: 'uppercase' }}>{label}</p>
                        <p style={{ margin: '3px 0 0', fontSize: 14, fontWeight: 600, color: colors.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{value}</p>
                      </div>
                    </div>
                  );
                })}
              </div>
            );
          })()}

          </div>{/* /ed-top-left */}

          {/* ══ SIDEBAR lives here, inside ed-top-row ══ */}

          <div className="ed-sidebar">
          <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 20, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>

            {/* Price header */}
            <div style={{ padding: '26px 26px 20px', borderBottom: `1px solid ${border}` }}>
              <p style={{ margin: '0 0 2px', fontSize: 10, fontWeight: 700, letterSpacing: 1.2, color: colors.subText, textTransform: 'uppercase' }}>Tickets from</p>
              <p style={{ margin: 0, fontSize: 42, fontWeight: 800, color: colors.text, letterSpacing: -1.5 }}>{fmtPrice(event.price)}</p>
              {avail != null && !isSoldOut && (
                <p style={{ margin: '6px 0 0', fontSize: 12, fontWeight: 700, color: isLow ? '#ff9500' : colors.subText }}>
                  {isLow ? `⚡ Only ${avail} left` : `${avail.toLocaleString()} tickets available`}
                </p>
              )}
            </div>

            {/* Organiser */}
            <div style={{ padding: '20px 26px', flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
              {(() => {
                const organiser = (event as any).organiser ?? (event as any).organizer ?? (event as any).hosted_by ?? (event as any).organisation ?? event.venue_name;
                return organiser ? (
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                    <div style={{ width: 44, height: 44, borderRadius: 13, background: subtle, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 18, flexShrink: 0 }}>
                      🏢
                    </div>
                    <div>
                      <p style={{ margin: 0, fontSize: 10, fontWeight: 700, letterSpacing: 1.1, color: colors.subText, textTransform: 'uppercase' }}>Organised by</p>
                      <p style={{ margin: '3px 0 0', fontSize: 15, fontWeight: 700, color: colors.text }}>{organiser}</p>
                    </div>
                  </div>
                ) : null;
              })()}
            </div>

            {/* CTA */}
            <div style={{ padding: '8px 20px 24px' }}>
              {isSoldOut ? (
                <button disabled style={{ width: '100%', background: subtle, color: colors.subText, border: 'none', borderRadius: 12, padding: '15px', fontSize: 15, fontWeight: 800, cursor: 'not-allowed' }}>
                  Sold Out
                </button>
              ) : (
                <button className="get-tickets-btn" onClick={handleGetTickets}>
                  Get Tickets
                </button>
              )}
            </div>

            {/* Trust footer inside card */}
            <div style={{ borderTop: `1px solid ${border}`, padding: '16px 26px', display: 'flex', gap: 20 }}>
              {[['✅', 'Official'], ['🔒', 'Secure']].map(([icon, label]) => (
                <div key={label} style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                  <span style={{ fontSize: 13 }}>{icon}</span>
                  <span style={{ fontSize: 12, fontWeight: 600, color: colors.subText }}>{label}</span>
                </div>
              ))}
            </div>
          </div>
          </div>{/* /ed-sidebar */}
        </div>{/* /ed-top-row */}

        {/* ══ BOTTOM LEFT: about, performers, trust ══ */}
        <div className="ed-bottom-left">

          {/* About */}
          {desc && (
            <div style={{ background: cardBg, borderRadius: 16, border: `1px solid ${border}`, padding: '20px 22px' }}>
              <p style={{ margin: '0 0 10px', fontSize: 11, fontWeight: 700, letterSpacing: 1.2, color: colors.subText, textTransform: 'uppercase' }}>About this event</p>
              <p style={{ margin: 0, fontSize: 14, color: colors.text, lineHeight: 1.8 }}>
                {expanded ? desc : shortDesc + (hasMore ? '…' : '')}
              </p>
              {hasMore && (
                <button onClick={() => setExpanded(e => !e)}
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#ff2d55', fontSize: 13, fontWeight: 700, padding: '10px 0 0', display: 'block' }}>
                  {expanded ? 'Show less ↑' : 'Read more ↓'}
                </button>
              )}
            </div>
          )}

          {/* Performers / Teams */}
          {tags.length > 0 && (
            <div style={{ background: cardBg, borderRadius: 16, border: `1px solid ${border}`, padding: '20px 22px' }}>
              <p style={{ margin: '0 0 12px', fontSize: 11, fontWeight: 700, letterSpacing: 1.2, color: colors.subText, textTransform: 'uppercase' }}>
                {event.category?.toLowerCase() === 'sports' ? 'Teams' : 'Performers'}
              </p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                {tags.map((tag: string) => (
                  <span key={tag} className="tag-chip">{tag}</span>
                ))}
              </div>
            </div>
          )}

          {/* Trust row */}
          <div style={{ background: cardBg, borderRadius: 16, border: `1px solid ${border}`, overflow: 'hidden' }}>
            {[
              { icon: '✅', label: 'Official Ticket Marketplace', sub: 'Reviewed and verified by the venue' },
              { icon: '🔒', label: 'Secure Checkout', sub: 'Your payment and personal data are protected' },
            ].map(({ icon, label, sub }, i) => (
              <div key={label} style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '14px 20px', borderBottom: i === 0 ? `1px solid ${border}` : 'none' }}>
                <span style={{ fontSize: 20, flexShrink: 0 }}>{icon}</span>
                <div>
                  <p style={{ margin: 0, fontSize: 13, fontWeight: 700, color: colors.text }}>{label}</p>
                  <p style={{ margin: '1px 0 0', fontSize: 12, color: colors.subText }}>{sub}</p>
                </div>
              </div>
            ))}
          </div>
        </div>{/* /ed-bottom-left */}
      </div>

      {/* ── Mobile CTA ─────────────────────────────────────────────────────── */}
      <div className="ed-mobile-cta">
        <div>
          <p style={{ margin: 0, fontSize: 11, color: colors.subText }}>From</p>
          <p style={{ margin: 0, fontSize: 18, fontWeight: 800, color: colors.text }}>{fmtPrice(event.price)}</p>
        </div>
        {isSoldOut ? (
          <button disabled style={{ flex: 1, background: subtle, color: colors.subText, border: 'none', borderRadius: 12, padding: '14px', fontSize: 15, fontWeight: 800, cursor: 'not-allowed' }}>
            Sold Out
          </button>
        ) : (
          <button className="get-tickets-btn" style={{ flex: 1 }} onClick={handleGetTickets}>
            Get Tickets
          </button>
        )}
      </div>
    </div>
  );
}