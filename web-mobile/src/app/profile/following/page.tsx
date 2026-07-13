'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';
import { api, type FollowedEvent, type FollowingResponse, fmtFollowDate, fmtFollowPrice, followAvailPct } from '@/lib/api';

const PALETTES = [
  { top: '#0d1f37', bottom: '#092e20', accent: '#00e5a0' },
  { top: '#1a0a2e', bottom: '#200a3d', accent: '#b57bee' },
  { top: '#1a1000', bottom: '#3a2400', accent: '#ffb830' },
  { top: '#001828', bottom: '#003050', accent: '#29c5f6' },
  { top: '#1a0008', bottom: '#3a0012', accent: '#ff4d6d' },
  { top: '#0a1a00', bottom: '#163800', accent: '#8fff00' },
];

export default function FollowingPage() {
  const router      = useRouter();
  const { user }    = useAuthStore();
  const { colors }  = useAppTheme();

  const [events,      setEvents]      = useState<FollowedEvent[]>([]);
  const [loading,     setLoading]     = useState(true);
  const [cursor,      setCursor]      = useState<string | null>(null);
  const [hasMore,     setHasMore]     = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [unfollowing, setUnfollowing] = useState<Set<string>>(new Set());

  const load = useCallback(async (replace = false) => {
    if (!user) { setLoading(false); return; }
    try {
      replace ? setLoading(true) : setLoadingMore(true);
      const params: Record<string, string> = { limit: '20' };
      if (!replace && cursor) params.cursor = cursor;
      const res = await api.get<FollowingResponse>('/v1/user/following', { params });
      const { data: items, next_cursor } = res.data;
      setEvents(prev => replace ? items : [...prev, ...items]);
      setCursor(next_cursor ?? null);
      setHasMore(!!next_cursor);
    } catch { /* show empty state */ }
    finally { setLoading(false); setLoadingMore(false); }
  }, [user, cursor]);

  useEffect(() => { load(true); }, []);

  const handleUnfollow = useCallback(async (eventId: string) => {
    setUnfollowing(prev => new Set(prev).add(eventId));
    try {
      await api.post(`/v1/events/${eventId}/follow`);
      setEvents(prev => prev.filter(e => e.id !== eventId));
    } catch { /* ignore */ }
    finally {
      setUnfollowing(prev => { const n = new Set(prev); n.delete(eventId); return n; });
    }
  }, []);

  const renderEmpty = (title: string, sub: string) => (
    <div className="flex flex-col items-center justify-center py-32 px-10 gap-4 text-center">
      <div className="w-20 h-20 rounded-2xl flex items-center justify-center"
           style={{ backgroundColor: 'rgba(255,255,255,0.04)' }}>
        <span className="text-3xl">🤍</span>
      </div>
      <p className="text-white text-xl font-extrabold">{title}</p>
      <p className="text-sm" style={{ color: colors.subText }}>{sub}</p>
    </div>
  );

  return (
    <div className="min-h-screen" style={{ backgroundColor: colors.background }}>
      {/* Header */}
      <div className="sticky top-0 z-10 flex items-center px-4 py-3 border-b"
           style={{ backgroundColor: colors.background, borderColor: colors.dividerColor }}>
        <button onClick={() => router.back()} className="w-10 h-10 flex items-center justify-center rounded-full">
          <span style={{ color: colors.text }}>←</span>
        </button>
        <div className="flex-1 text-center">
          <p className="font-extrabold text-lg" style={{ color: colors.text }}>Following</p>
          {events.length > 0 && !loading && (
            <p className="text-xs" style={{ color: colors.subText }}>
              {events.length} event{events.length !== 1 ? 's' : ''}
            </p>
          )}
        </div>
        <div className="w-10" />
      </div>

      {!user
        ? renderEmpty('Sign in to see followed events', "Follow events you're excited about and we'll remind you as the date gets close.")
        : loading
        ? <div className="flex items-center justify-center py-32"><div className="w-8 h-8 border-2 border-[#ff2d55] border-t-transparent rounded-full animate-spin" /></div>
        : events.length === 0
        ? renderEmpty('No followed events yet', "Tap Follow on any event to track it. You'll get reminders as the date approaches and alerts when tickets sell out.")
        : (
          <div className="flex flex-col gap-3 px-4 py-4 max-w-2xl mx-auto">
            {events.map((item, index) => {
              const pal = PALETTES[index % PALETTES.length];
              const pct = followAvailPct(item.tickets_sold, item.max_capacity);
              const isSellingFast = pct !== null && pct >= 80 && !item.is_sold_out;
              const pill = item.is_sold_out
                ? { bg: 'rgba(255,77,77,0.2)', border: 'rgba(255,77,77,0.38)', color: '#ff6b6b', label: 'SOLD OUT' }
                : isSellingFast
                  ? { bg: 'rgba(255,184,48,0.16)', border: 'rgba(255,184,48,0.32)', color: '#ffb830', label: '🔥 SELLING FAST' }
                  : { bg: 'rgba(0,229,160,0.1)', border: 'rgba(0,229,160,0.22)', color: '#00e5a0', label: item.category?.toUpperCase() || 'EVENT' };

              return (
                <div key={item.follow_id} className="relative rounded-[18px] overflow-hidden h-48 shadow-xl cursor-pointer"
                     onClick={() => router.push(`/events/${item.slug || item.id}`)}>
                  {item.cover_image_url
                    ? <img src={item.cover_image_url} alt={item.title} className="absolute inset-0 w-full h-full object-cover" />
                    : <div className="absolute inset-0" style={{ background: `linear-gradient(135deg, ${pal.top}, ${pal.bottom})` }} />
                  }
                  <div className="absolute inset-0" style={{ background: 'linear-gradient(to bottom, rgba(0,0,0,0.06), rgba(0,0,0,0.62), rgba(0,0,0,0.92))' }} />

                  <div className="absolute inset-0 p-3.5 flex flex-col justify-between">
                    {/* Top row */}
                    <div className="flex items-start justify-between">
                      <span className="px-2 py-1 rounded text-[9px] font-extrabold tracking-widest border"
                            style={{ backgroundColor: pill.bg, borderColor: pill.border, color: pill.color }}>
                        {pill.label}
                      </span>
                      <button
                        onClick={(e) => { e.stopPropagation(); handleUnfollow(item.id); }}
                        className="w-8 h-8 rounded-full flex items-center justify-center border"
                        style={{ backgroundColor: 'rgba(0,0,0,0.45)', borderColor: 'rgba(255,255,255,0.1)' }}
                      >
                        {unfollowing.has(item.id)
                          ? <div className="w-4 h-4 border-2 border-[#ff2d55] border-t-transparent rounded-full animate-spin" />
                          : <span style={{ color: '#ff2d55' }}>♥</span>
                        }
                      </button>
                    </div>
                    {/* Bottom info */}
                    <div className="flex flex-col gap-1">
                      <p className="text-[11px] font-semibold" style={{ color: 'rgba(255,255,255,0.55)' }}>
                        📅 {fmtFollowDate(item.start_time)}
                      </p>
                      <p className="text-white text-xl font-extrabold leading-tight line-clamp-2">{item.title}</p>
                      <div className="flex items-center gap-1.5">
                        <p className="text-[11px] flex-1 truncate" style={{ color: 'rgba(255,255,255,0.42)' }}>
                          📍 {item.venue_name}, {item.city}
                        </p>
                        <p className="text-xs font-extrabold" style={{ color: pal.accent }}>{fmtFollowPrice(item.price)}</p>
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}

            {hasMore && (
              <button
                onClick={() => load(false)}
                disabled={loadingMore}
                className="w-full py-3 text-xs font-bold tracking-widest border rounded-lg"
                style={{ borderColor: colors.dividerColor, color: colors.subText }}
              >
                {loadingMore ? 'Loading…' : 'LOAD MORE'}
              </button>
            )}
          </div>
        )
      }
    </div>
  );
}