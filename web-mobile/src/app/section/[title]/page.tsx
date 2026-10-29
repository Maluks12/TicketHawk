'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useEventStore } from '@/store/eventsStore';
import { useAppTheme } from '@/themes/colors';
import { CollectionBadge } from '@/components/collectionBadge';
import { asCardEvent, collectionDateRange, collectionHref } from '@/lib/eventCollections';
import {
  api,
  getEventsByCategory,
  getTrendingEvents,
  getActiveEvents,
  type Event,
} from '@/lib/api';

function formatPrice(price: number) {
  if (!price || price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}
function formatCardDate(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
}
function formatTime(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

async function fetchSectionEvents(sectionId: string): Promise<Event[]> {
  try {
    switch (sectionId) {
      case 'trending':
        return (await getTrendingEvents(100)).data.data ?? [];
      case 'happening_soon':
      case 'this_weekend':
        return (await getActiveEvents(100)).data.data ?? [];
      case 'recently_added': {
        const res = await api.get<{ data: Event[] }>('/v1/events', { params: { limit: 100 } });
        return res.data.data ?? [];
      }
      case 'featured': {
        const res = await api.get<{ data: Event[] }>('/v1/events/featured', { params: { limit: 100 } });
        return res.data.data ?? [];
      }
      default:
        return [];
    }
  } catch {
    return [];
  }
}

const EDITORIAL_FILTERS: Record<string, (e: Event) => boolean> = {
  free:         (e) => e.price === 0,
  premium:      (e) => e.price >= 3000,
  coming_up:    (e) => { const d = (new Date(e.start_time).getTime() - Date.now()) / 86400000; return d >= 15 && d <= 45; },
  other_cities: (e) => !['Johannesburg', 'johannesburg'].includes(e.city ?? ''),
  music:        (e) => !!(e.category?.toLowerCase().includes('concert') || e.category?.toLowerCase().includes('music')),
  gala:         (e) => !!(e.category?.toLowerCase().includes('gala') || e.category?.toLowerCase().includes('exclusive')),
  tech:         (e) => !!(e.category?.toLowerCase().includes('tech')),
  food:         (e) => !!(e.category?.toLowerCase().includes('food')),
};

export default function SectionPage() {
  const router  = useRouter();
  const { colors, isDark } = useAppTheme();
  const { sectionMeta, allEvents, dbCategories, setSelectedEvent } = useEventStore();

  const [events,  setEvents]  = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);

  const label = sectionMeta?.title ?? '';

  useEffect(() => {
    if (!sectionMeta) { setLoading(false); return; }
    const { sectionId, filterKey, events: fallbackEvents } = sectionMeta;

    if (filterKey) {
      const fn = EDITORIAL_FILTERS[filterKey];
      setEvents(fn ? allEvents.filter(fn) : (fallbackEvents ?? []));
      setLoading(false);
      return;
    }

    if (sectionId?.startsWith('cat-')) {
      const slug = sectionId.replace(/^cat-/, '');
      const match = dbCategories.find(
        (c) => c.name.toLowerCase().replace(/\s+/g, '-') === slug || c.slug?.toLowerCase() === slug
      );
      if (match) {
        getEventsByCategory(match.id, 100)
          .then((res) => setEvents(res.data?.data ?? []))
          .catch(() => setEvents(fallbackEvents ?? []))
          .finally(() => setLoading(false));
      } else {
        setEvents(fallbackEvents ?? []);
        setLoading(false);
      }
      return;
    }

    if (sectionId) {
      fetchSectionEvents(sectionId)
        .then((data) => setEvents(data.length > 0 ? data : (fallbackEvents ?? [])))
        .finally(() => setLoading(false));
      return;
    }

    setEvents(fallbackEvents ?? []);
    setLoading(false);
  }, [sectionMeta, allEvents, dbCategories]);

  return (
    <div className="min-h-screen" style={{ backgroundColor: colors.background }}>
      {/* Header */}
      <div className="sticky top-0 z-10 flex items-center px-4 py-3 border-b"
           style={{ backgroundColor: colors.background, borderColor: colors.dividerColor }}>
        <button onClick={() => router.back()} className="w-10 h-10 flex items-center justify-center">
          <span style={{ color: colors.text }} className="text-3xl leading-none font-light">‹</span>
        </button>
        <div className="flex-1 text-center md:text-left md:ml-2">
          <p className="font-bold text-base" style={{ color: colors.text }}>{label}</p>
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
      ) : events.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-32 px-8 text-center gap-3">
          <span className="text-4xl">🎟</span>
          <p className="font-bold text-lg" style={{ color: colors.text }}>No events found</p>
          <p className="text-sm" style={{ color: colors.subText }}>There are no events in this section right now.</p>
        </div>
      ) : (
        <div className="px-4 md:px-8 py-4">

          {/* ── MOBILE: single-column stacked cards with image overlay ── */}
          <div className="flex flex-col gap-4 max-w-2xl mx-auto md:hidden">
            {events.map((raw) => { const item = asCardEvent(raw); return (
              <button
                key={item.id}
                onClick={() => {
                  // A folded collection opens its own page, not the standing-in date.
                  if (raw.collection) { router.push(collectionHref(raw.collection)); return; }
                  setSelectedEvent(raw); router.push(`/events/${raw.id}`);
                }}
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
                {/* Top-left here: the title and date fill the bottom of this card. */}
                {raw.collection && <CollectionBadge collection={raw.collection} compact position="top-left" />}
                <div className="absolute bottom-0 left-0 right-0 p-3.5">
                  <p className="text-white font-extrabold text-base leading-snug mb-1 line-clamp-2">{item.title}</p>
                  <p className="text-white/80 text-xs font-medium mb-0.5">
                    {raw.collection
                      ? collectionDateRange(raw.collection)
                      : <>{formatCardDate(item.start_time)}{item.start_time ? `  ·  ${formatTime(item.start_time)}` : ''}</>}
                  </p>
                  {item.venue_name && (
                    <p className="text-white/65 text-xs truncate">
                      {item.venue_name}{item.city ? `, ${item.city}` : ''}
                    </p>
                  )}
                </div>
              </button>
            ); })}
          </div>

          {/* ── DESKTOP: YouTube Explore-style grid ── */}
          <div className="hidden md:grid grid-cols-3 lg:grid-cols-4 gap-x-5 gap-y-10">
            {events.map((raw) => { const item = asCardEvent(raw); return (
              <button
                key={item.id}
                onClick={() => {
                  // A folded collection opens its own page, not the standing-in date.
                  if (raw.collection) { router.push(collectionHref(raw.collection)); return; }
                  setSelectedEvent(raw); router.push(`/events/${raw.id}`);
                }}
                className="text-left w-full group"
              >
                {/* Thumbnail */}
                <div
                  className="relative w-full aspect-video rounded-xl overflow-hidden mb-3"
                  style={{ backgroundColor: colors.sectionCardBg }}
                >
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
                  {/* Price sits bottom-right on this card; the badge takes bottom-left. */}
                  {raw.collection && <CollectionBadge collection={raw.collection} compact />}
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
                    {raw.collection
                      ? collectionDateRange(raw.collection)
                      : <>{formatCardDate(item.start_time)}{item.start_time ? ` · ${formatTime(item.start_time)}` : ''}</>}
                  </p>
                  {item.venue_name && (
                    <p className="text-[13px] leading-[19px] truncate" style={{ color: colors.subText }}>
                      {item.venue_name}{item.city ? `, ${item.city}` : ''}
                    </p>
                  )}
                </div>
              </button>
            ); })}
          </div>

        </div>
      )}
    </div>
  );
}