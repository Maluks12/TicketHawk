'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter, useParams } from 'next/navigation';
import { ArrowLeft, Share2, MapPin, Calendar, Clock, Users } from 'lucide-react';
import { useEventStore } from '@/store/eventsStore';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';
import { toggleFollow, getFollowStatus, api, type Event } from '@/lib/api';

// ── Helpers ───────────────────────────────────────────────────────────────────
const fmtPrice = (price: number) =>
  price === 0 ? 'Free' : `R${Math.round(price).toLocaleString('en-ZA')}`;

const fmtDate = (iso: string) =>
  new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'long', year: 'numeric' });

const fmtDateShort = (iso: string) =>
  new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });

const fmtTime = (iso: string) =>
  new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });

// ── Meta item ─────────────────────────────────────────────────────────────────
function MetaItem({ icon: Icon, label, value }: { icon: React.ElementType; label: string; value: string }) {
  const { colors } = useAppTheme();
  return (
    <div className="flex items-center gap-3 py-3">
      <div
        className="w-9 h-9 rounded-[10px] flex items-center justify-center shrink-0"
        style={{ backgroundColor: colors.sectionCardBg }}
      >
        <Icon size={16} color={colors.subText} />
      </div>
      <div>
        <p className="text-[10px] font-black tracking-[1px]" style={{ color: colors.subText }}>{label}</p>
        <p className="text-sm font-semibold mt-0.5" style={{ color: colors.text }}>{value}</p>
      </div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────
export default function EventDetailPage() {
  const router = useRouter();
  const { id } = useParams<{ id: string }>();
  const { colors, isDark } = useAppTheme();

  const selectedEvent    = useEventStore((s) => s.selectedEvent);
  const setSelectedEvent = useEventStore((s) => s.setSelectedEvent);
  const addRecentlyViewed = useEventStore((s) => s.addRecentlyViewed);
  const authUser         = useAuthStore((s) => s.user);

  const [event,        setEvent]        = useState<Event | null>(selectedEvent);
  const [expanded,     setExpanded]     = useState(false);
  const [isFollowing,  setIsFollowing]  = useState(false);
  const [followLoading, setFollowLoading] = useState(false);
  const [notFound,     setNotFound]     = useState(false);

  // ── Fetch event if not in store ───────────────────────────────────────────
  useEffect(() => {
    if (!id) return;

    // Track view
    fetch(`${process.env.NEXT_PUBLIC_API_URL}/v1/events/${id}/view`, { method: 'POST' }).catch(() => {});

    // Fetch fresh data
    fetch(`${process.env.NEXT_PUBLIC_API_URL}/v1/events/${id}`)
      .then((r) => { if (!r.ok) { setNotFound(true); return null; } return r.json(); })
      .then((fresh) => {
        if (fresh) {
          setEvent(fresh);
          setSelectedEvent(fresh);
        }
      })
      .catch(() => {
        if (!selectedEvent) setNotFound(true);
      });
  }, [id]);

  // ── Recently viewed ───────────────────────────────────────────────────────
  useEffect(() => {
    if (event?.id && authUser?.id) {
      addRecentlyViewed(event, authUser.id);
    }
  }, [event?.id, authUser?.id]);

  // ── Follow status ─────────────────────────────────────────────────────────
  useEffect(() => {
    if (!event?.id || !authUser) return;
    let cancelled = false;
    getFollowStatus(event.id)
      .then((res) => { if (!cancelled) setIsFollowing(res.following); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [event?.id, authUser]);

  const handleFollow = useCallback(async () => {
    if (!authUser) { router.push('/auth/login'); return; }
    if (!event?.id || followLoading) return;
    setIsFollowing((prev) => !prev);
    setFollowLoading(true);
    try {
      const res = await toggleFollow(event.id);
      setIsFollowing(res.following);
    } catch {
      setIsFollowing((prev) => !prev);
    } finally {
      setFollowLoading(false);
    }
  }, [authUser, event?.id, followLoading, router]);

  const handleShare = () => {
    if (navigator.share && event) {
      navigator.share({ title: event.title, url: window.location.href }).catch(() => {});
    } else {
      navigator.clipboard.writeText(window.location.href).then(() => alert('Link copied!')).catch(() => {});
    }
  };

  const handleDirections = () => {
    if (!event) return;
    const q = encodeURIComponent(`${event.venue_name}, ${event.city}`);
    window.open(`https://www.google.com/maps/search/?api=1&query=${q}`, '_blank');
  };

  // ── Loading / not found ───────────────────────────────────────────────────
  if (notFound && !event) {
    return (
      <div className="flex flex-col items-center justify-center min-h-screen" style={{ backgroundColor: colors.background }}>
        <p className="text-sm font-black tracking-[3px]" style={{ color: colors.subText }}>EVENT NOT FOUND</p>
        <button
          onClick={() => router.back()}
          className="mt-4 border px-5 py-2 rounded-sm text-[11px] font-black tracking-wide"
          style={{ borderColor: colors.dividerColor, color: colors.text }}
        >
          ← GO BACK
        </button>
      </div>
    );
  }

  if (!event) {
    return (
      <div className="flex items-center justify-center min-h-screen" style={{ backgroundColor: colors.background }}>
        <div className="w-10 h-10 border-2 border-[#ff2d55] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  const ev           = event as any;
  const heroImage    = event.banner_image_url ?? event.cover_image_url;
  const isSoldOut    = event.is_sold_out ?? false;
  const isASport     = event.category?.toLowerCase() === 'sports';
  const teams:      string[] = ev.teams      ?? [];
  const performers: string[] = ev.performers ?? [];
  const maxCap               = ev.max_capacity ?? 0;
  const organizerName        = ev.organizer_name ?? 'EVENT ORGANIZER';

  const shortDesc   = event.description?.slice(0, 280) ?? '';
  const hasMore     = (event.description?.length ?? 0) > 280;
  const displayDesc = expanded ? event.description : (shortDesc + (hasMore ? '…' : ''));

  return (
    <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>

      {/* ── Floating nav ──────────────────────────────────────────────────── */}
      <div className="fixed top-0 md:top-16 left-0 right-0 z-40 flex items-center justify-between px-4 pt-3 pb-2 pointer-events-none">
        <button
          onClick={() => router.back()}
          className="pointer-events-auto w-10 h-10 rounded-full flex items-center justify-center backdrop-blur-md"
          style={{ backgroundColor: 'rgba(0,0,0,0.55)' }}
        >
          <ArrowLeft size={18} color="#fff" />
        </button>
        <button
          onClick={handleShare}
          className="pointer-events-auto w-10 h-10 rounded-full flex items-center justify-center backdrop-blur-md"
          style={{ backgroundColor: 'rgba(0,0,0,0.55)' }}
        >
          <Share2 size={18} color="#fff" />
        </button>
      </div>

      {/* ── Hero ──────────────────────────────────────────────────────────── */}
      <div className="relative w-full" style={{ height: 420 }}>
        {heroImage ? (
          <img src={heroImage} alt={event.title} className="absolute inset-0 w-full h-full object-cover" />
        ) : (
          <div className="absolute inset-0" style={{ backgroundColor: '#222' }} />
        )}

        {/* Gradient overlays */}
        <div className="absolute inset-0" style={{ background: 'linear-gradient(to top, #0a0a0a 0%, rgba(10,10,10,0.85) 30%, transparent 70%)' }} />
        <div className="absolute inset-0" style={{ background: 'linear-gradient(to bottom, rgba(0,0,0,0.5) 0%, transparent 35%)' }} />

        {/* Sold out badge */}
        {isSoldOut && (
          <div className="absolute top-16 right-4 px-3 py-1.5 rounded-sm bg-black/70 border border-white/20">
            <span className="text-[10px] font-black tracking-[2px] text-white">SOLD OUT</span>
          </div>
        )}

        {/* Hero text */}
        <div className="absolute bottom-0 left-0 right-0 px-5 pb-6">
          <div className="flex flex-wrap items-center gap-3 mb-2.5">
            {event.is_featured && (
              <span className="bg-[#FF2D55] text-white text-[9px] font-black tracking-[1.5px] px-2.5 py-1 rounded-sm">
                TRENDING #1
              </span>
            )}
            {event.views != null && (
              <span className="text-white/70 text-[11px] font-bold">
                👁 {event.views.toLocaleString('en-ZA')} VIEWS
              </span>
            )}
          </div>
          <p className="text-[11px] font-bold tracking-[2px] text-white/70 uppercase mb-2">
            ✦ {event.category ?? 'Event'}
          </p>
          <h1 className="text-4xl font-black text-white tracking-[1.5px] leading-tight line-clamp-3 uppercase">
            {event.title}
          </h1>
        </div>
      </div>

      {/* ── Action card ───────────────────────────────────────────────────── */}
      <div
        className="mx-4 -mt-4 relative z-10 rounded-[16px] overflow-hidden border p-5 mb-6"
        style={{ backgroundColor: colors.cardBg, borderColor: colors.dividerColor }}
      >
        {/* Meta rows */}
        <div className="divide-y" style={{ '--tw-divide-opacity': 1, borderColor: colors.dividerColor } as any}>
          <MetaItem icon={Calendar} label="DATE"  value={fmtDateShort(event.start_time)} />
          <MetaItem icon={Clock}    label="TIME"  value={fmtTime(event.start_time)} />
          <MetaItem icon={MapPin}   label="VENUE" value={`${event.venue_name}, ${event.city}`} />
          {maxCap > 0 && (
            <MetaItem icon={Users} label="CAPACITY" value={`${maxCap.toLocaleString('en-ZA')} Seats`} />
          )}
        </div>

        <div className="mt-4 pt-4 border-t" style={{ borderColor: colors.dividerColor }}>
          {/* Price */}
          <p className="text-sm font-black mb-3" style={{ color: colors.subText }}>
            From <span style={{ color: colors.text, fontSize: 18 }}>{fmtPrice(event.price)}</span>
          </p>

          {/* Primary CTA */}
          {isSoldOut ? (
            <div className="w-full py-4 rounded-sm border text-center" style={{ borderColor: colors.dividerColor }}>
              <span className="text-sm font-black tracking-wide" style={{ color: colors.subText }}>🎟 SOLD OUT</span>
            </div>
          ) : (
            <button
              onClick={() => router.push(`/events/${event.id}/purchase`)}
              className="w-full py-4 rounded-sm text-white font-black text-sm tracking-[1.5px] mb-3"
              style={{ backgroundColor: '#FF2D55' }}
            >
              GET TICKETS
            </button>
          )}

          {/* Secondary row */}
          <div className="flex gap-3">
            <button
              onClick={handleFollow}
              disabled={followLoading}
              className="flex-1 flex items-center justify-center gap-2 py-3 rounded-sm border text-sm font-black tracking-wide transition-all"
              style={{
                borderColor: isFollowing ? '#b57bee' : colors.dividerColor,
                backgroundColor: isFollowing ? '#2d1c3a' : 'transparent',
                color: isFollowing ? '#b57bee' : colors.text,
              }}
            >
              {isFollowing ? '💜' : '🤍'}
              <span>{followLoading ? '...' : isFollowing ? 'FOLLOWING' : 'FOLLOW'}</span>
            </button>
            <button
              onClick={handleShare}
              className="flex-1 flex items-center justify-center gap-2 py-3 rounded-sm border text-sm font-black tracking-wide"
              style={{ borderColor: colors.dividerColor, color: colors.text }}
            >
              ↑ SHARE
            </button>
          </div>
        </div>
      </div>

      {/* ── Content ───────────────────────────────────────────────────────── */}
      <div className="px-5 space-y-8 pb-16">

        {/* About */}
        <div>
          <h2 className="text-lg font-black mb-3" style={{ color: colors.text }}>About This Event</h2>
          <p className="text-sm leading-relaxed" style={{ color: colors.subText }}>{displayDesc}</p>
          {hasMore && (
            <button
              onClick={() => setExpanded((v) => !v)}
              className="mt-2 text-[11px] font-black tracking-wide border-b"
              style={{ color: colors.text, borderColor: colors.text }}
            >
              {expanded ? 'SHOW LESS ▴' : 'READ MORE'}
            </button>
          )}
        </div>

        {/* Bento grid */}
        <div className="grid grid-cols-2 gap-3">
          {[
            ['🎟', 'STATUS', event.status?.toUpperCase() ?? '—'],
            ['✅', 'TYPE',   event.category ?? '—'],
          ].map(([icon, label, value]) => (
            <div
              key={label}
              className="rounded-[12px] p-4 border"
              style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }}
            >
              <p className="text-2xl mb-2">{icon}</p>
              <p className="text-[10px] font-black tracking-[1px] mb-0.5" style={{ color: colors.subText }}>{label}</p>
              <p className="text-sm font-black" style={{ color: colors.text }}>{value}</p>
            </div>
          ))}
        </div>

        {/* Teams */}
        {isASport && teams.length > 0 && (
          <div>
            <h2 className="text-lg font-black mb-3" style={{ color: colors.text }}>TEAMS</h2>
            <div className="rounded-[14px] overflow-hidden border divide-y" style={{ borderColor: colors.dividerColor }}>
              {teams.map((team, i) => (
                <div key={i} className="flex items-center gap-3 px-4 py-3">
                  <div
                    className="w-9 h-9 rounded-full flex items-center justify-center shrink-0 border"
                    style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }}
                  >
                    <span className="text-sm font-black" style={{ color: colors.text }}>
                      {team.trim().charAt(0).toUpperCase()}
                    </span>
                  </div>
                  <span className="flex-1 text-sm font-semibold" style={{ color: colors.text }}>{team}</span>
                  <span style={{ color: colors.subText }}>›</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Performers */}
        {!isASport && performers.length > 0 && (
          <div>
            <h2 className="text-lg font-black mb-3" style={{ color: colors.text }}>PERFORMERS</h2>
            <div className="rounded-[14px] overflow-hidden border divide-y" style={{ borderColor: colors.dividerColor }}>
              {performers.map((performer, i) => (
                <div key={i} className="flex items-center gap-3 px-4 py-3">
                  <div
                    className="w-9 h-9 rounded-full flex items-center justify-center shrink-0 border"
                    style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }}
                  >
                    <span className="text-sm font-black" style={{ color: colors.text }}>
                      {performer.trim().charAt(0).toUpperCase()}
                    </span>
                  </div>
                  <span className="flex-1 text-sm font-semibold" style={{ color: colors.text }}>{performer}</span>
                  <span style={{ color: colors.subText }}>›</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Location */}
        <div>
          <h2 className="text-lg font-black mb-3" style={{ color: colors.text }}>LOCATION</h2>
          <div
            className="rounded-[14px] overflow-hidden border"
            style={{ backgroundColor: colors.cardBg, borderColor: colors.dividerColor }}
          >
            {/* Map placeholder */}
            <div
              className="relative w-full flex items-center justify-center"
              style={{ height: 140, backgroundColor: isDark ? '#1a1a1a' : '#f0f0f0' }}
            >
              {/* Grid lines */}
              {[1,2,3,4,5,6].map((i) => (
                <div key={`h${i}`} className="absolute left-0 right-0 h-px opacity-20" style={{ top: `${i * 14}%`, backgroundColor: '#999' }} />
              ))}
              {[1,2,3,4,5,6].map((i) => (
                <div key={`v${i}`} className="absolute top-0 bottom-0 w-px opacity-20" style={{ left: `${i * 14}%`, backgroundColor: '#999' }} />
              ))}
              <div
                className="w-11 h-11 rounded-full flex items-center justify-center shadow-lg"
                style={{ backgroundColor: isDark ? '#ffffff' : '#1a1a1a' }}
              >
                <span className="text-xl">📍</span>
              </div>
            </div>
            {/* Address */}
            <div className="p-4">
              <p className="text-base font-black mb-0.5" style={{ color: colors.text }}>{event.venue_name}</p>
              {ev.address && <p className="text-sm mb-0.5" style={{ color: colors.subText }}>{ev.address}</p>}
              <p className="text-sm mb-4" style={{ color: colors.subText }}>{event.city}</p>
              <div className="flex gap-3">
                <button
                  onClick={handleDirections}
                  className="flex-1 py-2.5 rounded-sm text-white text-[11px] font-black tracking-wide"
                  style={{ backgroundColor: colors.text }}
                >
                  GET DIRECTIONS
                </button>
                <button
                  onClick={handleDirections}
                  className="flex-1 py-2.5 rounded-sm text-[11px] font-black tracking-wide border"
                  style={{ borderColor: colors.dividerColor, color: colors.text }}
                >
                  OPEN IN MAPS
                </button>
              </div>
            </div>
          </div>
        </div>

        {/* Organizer */}
        <div
          className="flex items-center gap-4 p-4 rounded-[14px] border"
          style={{ backgroundColor: colors.cardBg, borderColor: colors.dividerColor }}
        >
          <div
            className="w-14 h-14 rounded-full flex items-center justify-center shrink-0 border"
            style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }}
          >
            <span className="text-xl font-black" style={{ color: colors.text }}>
              {organizerName.charAt(0).toUpperCase()}
            </span>
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-[10px] font-bold tracking-wide mb-0.5" style={{ color: colors.subText }}>Organized by</p>
            <p className="text-sm font-black truncate" style={{ color: colors.text }}>{organizerName}</p>
          </div>
          <button className="text-2xl">💬</button>
        </div>

      </div>
    </div>
  );
}