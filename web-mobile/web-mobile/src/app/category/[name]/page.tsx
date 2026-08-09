'use client';

import { useState, useEffect, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { useEventStore } from '@/store/eventsStore';
import { useAppTheme } from '@/themes/colors';
import { getEventsByCategory, type Event } from '@/lib/api';
import { WarningCircle, Ticket } from '@phosphor-icons/react';

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
  const { colors, isDark }  = useAppTheme();
  const { dbCategories, setSelectedEvent } = useEventStore();

  const [events,  setEvents]  = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [error,   setError]   = useState<string | null>(null);

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

  const handlePress = (item: Event) => {
    setSelectedEvent(item);
    router.push(`/events/${item.id}`);
  };

  return (
    <div className="min-h-screen" style={{ backgroundColor: colors.background }}>

      {/* Header */}
      <div
        className="sticky top-0 z-10 flex items-center px-4 py-3 border-b"
        style={{ backgroundColor: colors.background, borderColor: colors.dividerColor }}
      >
        <button onClick={() => router.back()} className="w-10 h-10 flex items-center justify-center">
          <span style={{ color: colors.text }} className="text-3xl leading-none font-light">‹</span>
        </button>
        <div className="flex-1 text-center md:text-left md:ml-2">
          <p className="font-bold text-base" style={{ color: colors.text }}>{categoryLabel}</p>
          <p className="text-xs" style={{ color: colors.subText }}>
            {loading ? 'Loading…' : `${events.length} event${events.length !== 1 ? 's' : ''}`}
          </p>
        </div>
        <div className="w-10" />
      </div>

      {/* Loading */}
      {loading ? (
        <div className="flex items-center justify-center py-32">
          <div
            className="w-8 h-8 border-2 border-t-transparent rounded-full animate-spin"
            style={{ borderColor: colors.text }}
          />
        </div>

      /* Error */
      ) : error ? (
        <div className="flex flex-col items-center justify-center py-32 px-8 text-center gap-4">
          <WarningCircle size={40} weight="regular" color={colors.subText} />
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

      /* Empty */
      ) : events.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-32 px-8 text-center gap-3">
          <Ticket size={40} weight="regular" color={colors.subText} />
          <p className="font-bold text-lg" style={{ color: colors.text }}>No events found</p>
          <p className="text-sm" style={{ color: colors.subText }}>
            There are no {categoryLabel.toLowerCase()} events right now. Check back soon.
          </p>
        </div>

      /* Events grid */
      ) : (
        <div className="px-4 md:px-8 py-4">
          {/* ── MOBILE: single-column stacked cards with image overlay (unchanged) ── */}
          <div className="flex flex-col gap-4 max-w-2xl mx-auto md:hidden">
            {events.map((item) => (
              <button
                key={item.id}
                onClick={() => handlePress(item)}
                className="relative rounded-xl overflow-hidden w-full text-left shadow-lg"
                style={{ aspectRatio: '1.92' }}
              >
                {item.cover_image_url ? (
                  <img src={item.cover_image_url} alt={item.title}
                       className="absolute inset-0 w-full h-full object-cover" />
                ) : (
                  <div className="absolute inset-0 bg-neutral-800" />
                )}
                <div
                  className="absolute inset-0"
                  style={{ background: 'linear-gradient(to bottom, transparent 30%, rgba(0,0,0,0.35) 62%, rgba(0,0,0,0.86))' }}
                />
                <div className="absolute top-3 right-3 bg-black/60 rounded px-2.5 py-1">
                  <span className="text-white text-xs font-bold">{formatPrice(item.price)}</span>
                </div>
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

          {/* ── DESKTOP: YouTube Explore-style grid ── */}
          <div className="hidden md:grid grid-cols-3 lg:grid-cols-4 gap-x-5 gap-y-10">
            {events.map((item) => (
              <button
                key={item.id}
                onClick={() => handlePress(item)}
                className="text-left w-full group"
              >
                {/* Thumbnail */}
                <div className="relative w-full aspect-video rounded-xl overflow-hidden mb-3"
                     style={{ backgroundColor: colors.sectionCardBg }}>
                  {item.cover_image_url ? (
                    <img
                      src={item.cover_image_url}
                      alt={item.title}
                      className="absolute inset-0 w-full h-full object-cover transition-transform duration-200 group-hover:scale-105"
                    />
                  ) : (
                    <div className="absolute inset-0 bg-neutral-800" />
                  )}
                  {/* Price badge */}
                  <div className="absolute bottom-2 right-2 bg-black/80 rounded px-2 py-0.5">
                    <span className="text-white text-[11px] font-bold">{formatPrice(item.price)}</span>
                  </div>
                </div>

                {/* Text below thumbnail — YouTube style */}
                <div className="px-0.5">
                  <p
                    className="font-semibold text-[15px] leading-[21px] line-clamp-2 mb-1"
                    style={{ color: colors.text }}
                  >
                    {item.title}
                  </p>
                  <p className="text-[13px] leading-[19px]" style={{ color: colors.subText }}>
                    {formatCardDate(item.start_time)}
                    {item.start_time ? ` · ${formatTime(item.start_time)}` : ''}
                  </p>
                  {item.venue_name && (
                    <p className="text-[13px] leading-[19px] truncate" style={{ color: colors.subText }}>
                      {item.venue_name}{item.city ? `, ${item.city}` : ''}
                    </p>
                  )}
                </div>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}