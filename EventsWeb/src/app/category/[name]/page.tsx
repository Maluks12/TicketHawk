'use client';

import { useState, useEffect, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { useEventStore } from '@/store/eventsStore';
import { useAppTheme } from '@/themes/colors';
import { getEventsByCategory, type Event } from '@/lib/api';

function formatPrice(price: number) {
  if (!price || price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function formatCardDate(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-ZA', {
    weekday: 'short', day: 'numeric', month: 'short', year: 'numeric',
  });
}

function formatTime(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

export default function CategoryPage() {
  const params      = useParams<{ name: string }>();
  const router      = useRouter();
  const { colors }  = useAppTheme();
  const { dbCategories, setSelectedEvent } = useEventStore();

  const [events,     setEvents]     = useState<Event[]>([]);
  const [loading,    setLoading]    = useState(true);
  const [error,      setError]      = useState<string | null>(null);

  const decodedName   = params.name ? decodeURIComponent(params.name) : '';
  const categoryLabel = decodedName.replace(/\b\w/g, (c) => c.toUpperCase());

  const fetchEvents = useCallback(async () => {
    if (!decodedName) return;
    setError(null);

    const match = dbCategories.find(
      (c) =>
        c.name.toLowerCase() === decodedName.toLowerCase() ||
        c.slug?.toLowerCase() === decodedName.toLowerCase()
    );

    if (!match) {
      setError(`Category "${categoryLabel}" not found. Go back to the home screen and try again.`);
      return;
    }

    try {
      const res  = await getEventsByCategory(match.id, 50);
      const data = res.data?.data ?? [];
      setEvents(data);
    } catch (e: any) {
      setError(e?.message ?? 'Failed to load events.');
    }
  }, [decodedName, dbCategories, categoryLabel]);

  useEffect(() => {
    setLoading(true);
    fetchEvents().finally(() => setLoading(false));
  }, [fetchEvents]);

  return (
    <div className="min-h-screen" style={{ backgroundColor: colors.background }}>
      {/* Header */}
      <div className="sticky top-0 z-10 flex items-center px-4 py-3 border-b"
           style={{ backgroundColor: colors.background, borderColor: colors.dividerColor }}>
        <button onClick={() => router.back()} className="w-10 h-10 flex items-center justify-center">
          <span style={{ color: colors.text }} className="text-3xl leading-none font-light">‹</span>
        </button>
        <div className="flex-1 text-center">
          <p className="font-bold text-base" style={{ color: colors.text }}>{categoryLabel}</p>
          <p className="text-xs" style={{ color: colors.subText }}>
            {loading ? 'Loading…' : `${events.length} event${events.length !== 1 ? 's' : ''}`}
          </p>
        </div>
        <div className="w-10" />
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-32">
          <div className="w-8 h-8 border-2 border-t-transparent rounded-full animate-spin"
               style={{ borderColor: colors.text }} />
        </div>
      ) : error ? (
        <div className="flex flex-col items-center justify-center py-32 px-8 text-center gap-4">
          <span className="text-4xl">⚠️</span>
          <p className="font-bold text-lg" style={{ color: colors.text }}>Something went wrong</p>
          <p className="text-sm" style={{ color: colors.subText }}>{error}</p>
          <button
            onClick={() => { setLoading(true); fetchEvents().finally(() => setLoading(false)); }}
            className="border px-5 py-2 text-xs font-extrabold tracking-widest rounded"
            style={{ borderColor: colors.text, color: colors.text }}
          >
            RETRY
          </button>
        </div>
      ) : events.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-32 px-8 text-center gap-3">
          <span className="text-4xl">🎟</span>
          <p className="font-bold text-lg" style={{ color: colors.text }}>No events found</p>
          <p className="text-sm" style={{ color: colors.subText }}>
            There are no {categoryLabel.toLowerCase()} events right now. Check back soon.
          </p>
        </div>
      ) : (
        <div className="flex flex-col gap-4 px-4 py-4 max-w-2xl mx-auto">
          {events.map((item) => (
            <button
              key={item.id}
              onClick={() => { setSelectedEvent(item); router.push(`/events/${item.id}`); }}
              className="relative rounded-xl overflow-hidden w-full text-left shadow-lg"
              style={{ aspectRatio: '1.92' }}
            >
              {item.cover_image_url ? (
                <img src={item.cover_image_url} alt={item.title}
                     className="absolute inset-0 w-full h-full object-cover" />
              ) : (
                <div className="absolute inset-0 bg-neutral-800" />
              )}
              {/* gradient */}
              <div className="absolute inset-0"
                   style={{ background: 'linear-gradient(to bottom, transparent 30%, rgba(0,0,0,0.35) 62%, rgba(0,0,0,0.86))' }} />
              {/* price badge */}
              <div className="absolute top-3 right-3 bg-black/60 rounded px-2.5 py-1">
                <span className="text-white text-xs font-bold">{formatPrice(item.price)}</span>
              </div>
              {/* text */}
              <div className="absolute bottom-0 left-0 right-0 p-3.5">
                <p className="text-white font-extrabold text-base leading-snug mb-1 line-clamp-2">{item.title}</p>
                <p className="text-white/80 text-xs font-medium mb-0.5">
                  {formatCardDate(item.start_time)}{item.start_time ? `  ·  ${formatTime(item.start_time)}` : ''}
                </p>
                {item.venue_name && (
                  <p className="text-white/65 text-xs truncate">
                    {item.venue_name}{item.city ? `, ${item.city}` : ''}
                  </p>
                )}
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}