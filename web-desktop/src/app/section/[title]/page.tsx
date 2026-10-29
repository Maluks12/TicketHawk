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

function SkeletonCard({ large = false }: { large?: boolean }) {
  return (
    <div
      className="animate-pulse rounded-2xl overflow-hidden"
      style={{ aspectRatio: large ? '16 / 4' : '16 / 9', backgroundColor: 'rgba(255,255,255,0.05)' }}
    />
  );
}

function HeroCard({ item: raw, onPress, colors, label }: { item: Event; onPress: (e: Event) => void; colors: any; label?: string }) {
  const [hovered, setHovered] = useState(false);
  // A collection's stand-in event renders as the collection (see asCardEvent).
  const item = asCardEvent(raw);
  return (
    <button
      onClick={() => onPress(item)}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="relative w-full text-left overflow-hidden rounded-2xl"
      style={{ aspectRatio: '16 / 4' }}
    >
      {item.cover_image_url ? (
        <img
          src={item.cover_image_url}
          alt={item.title}
          className="absolute inset-0 w-full h-full object-cover transition-transform duration-700"
          style={{ transform: hovered ? 'scale(1.04)' : 'scale(1)' }}
        />
      ) : (
        <div className="absolute inset-0 rounded-2xl" style={{ backgroundColor: 'rgba(255,255,255,0.05)' }} />
      )}
      <div
        className="absolute inset-0"
        style={{ background: 'linear-gradient(to bottom, rgba(0,0,0,0.04) 0%, rgba(0,0,0,0.18) 45%, rgba(0,0,0,0.88) 100%)' }}
      />
      <div
        className="absolute top-4 right-4 rounded-lg px-3 py-1.5"
        style={{ backgroundColor: formatPrice(item.price) === 'Free' ? '#22c55e' : '#ff2d55' }}
      >
        <span className="text-white text-xs font-black tracking-wide">{formatPrice(item.price)}</span>
      </div>
      {/* Top-left: the title block fills the bottom of this card. */}
      {raw.collection && <CollectionBadge collection={raw.collection} position="top-left" />}
      <div className="absolute bottom-0 left-0 right-0 p-4">
        <div className="flex items-center gap-2 mb-2">
          {label && (
            <div
              className="flex items-center gap-1 text-[10px] font-black tracking-[0.12em] uppercase px-2.5 py-1 rounded"
              style={{ backgroundColor: 'rgba(255,45,85,0.9)', color: '#fff' }}
            >
              🔥 {label}
            </div>
          )}
          {item.category && (
            <div
              className="text-[10px] font-black tracking-[0.12em] uppercase px-2.5 py-1 rounded"
              style={{ backgroundColor: 'rgba(0,0,0,0.5)', backdropFilter: 'blur(6px)', color: 'rgba(255,255,255,0.85)' }}
            >
              {item.category}
            </div>
          )}
        </div>
        <p
          className="text-white font-black text-lg leading-tight mb-1"
          style={{ textShadow: '0 2px 16px rgba(0,0,0,0.7)' }}
        >
          {item.title}
        </p>
        <p className="text-white/70 text-sm font-medium">
          {raw.collection
            ? collectionDateRange(raw.collection)
            : <>{formatCardDate(item.start_time)}{item.start_time ? ` · ${formatTime(item.start_time)}` : ''}</>}
        </p>
        {item.venue_name && (
          <p className="text-white/45 text-xs mt-1 truncate">
            {item.venue_name}{item.city ? `, ${item.city}` : ''}
          </p>
        )}
      </div>
    </button>
  );
}

function GridCard({
  item, onPress, colors, isDark, rank,
}: {
  item: Event; onPress: (e: Event) => void; colors: any; isDark: boolean; rank?: number;
}) {
  const [hovered, setHovered] = useState(false);
  const raw = item;
  item = asCardEvent(raw);
  return (
    <button
      onClick={() => onPress(item)}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="text-left w-full relative"
    >
      {/* Netflix-style rank number */}
      {rank !== undefined && (
        <div
          className="absolute -left-2 bottom-[calc(100%-68px)] z-10 font-black select-none pointer-events-none leading-none"
          style={{
            fontSize: '5.5rem',
            color: 'transparent',
            WebkitTextStroke: isDark ? '1.5px rgba(255,255,255,0.13)' : '1.5px rgba(0,0,0,0.1)',
          }}
        >
          {rank}
        </div>
      )}
      <div
        className="relative w-full aspect-video rounded-xl overflow-hidden mb-2.5"
        style={{ backgroundColor: 'rgba(255,255,255,0.05)' }}
      >
        {item.cover_image_url ? (
          <img
            src={item.cover_image_url}
            alt={item.title}
            className="absolute inset-0 w-full h-full object-cover transition-transform duration-300"
            style={{ transform: hovered ? 'scale(1.06)' : 'scale(1)' }}
          />
        ) : (
          <div className="absolute inset-0" style={{ backgroundColor: 'rgba(255,255,255,0.05)' }} />
        )}
        <div
          className="absolute inset-0 transition-opacity duration-200"
          style={{ backgroundColor: 'rgba(0,0,0,0.28)', opacity: hovered ? 1 : 0 }}
        />
        <div
          className="absolute bottom-2 left-2 rounded-md px-2 py-0.5"
          style={{ backgroundColor: formatPrice(item.price) === 'Free' ? '#22c55e' : 'rgba(0,0,0,0.82)', backdropFilter: 'blur(4px)' }}
        >
          <span className="text-white text-[10px] font-black tracking-wide">{formatPrice(item.price)}</span>
        </div>
        {/* Price sits bottom-left on this card, so the badge goes bottom-right. */}
        {raw.collection && <CollectionBadge collection={raw.collection} position="bottom-right" />}
      </div>
      <div style={{ paddingLeft: rank !== undefined ? '1.25rem' : 0 }}>
        <p
          className="font-semibold text-[14px] leading-[20px] line-clamp-2 mb-1 transition-colors duration-150"
          style={{ color: hovered ? '#ff2d55' : colors.text }}
        >
          {item.title}
        </p>
        <p className="text-[12px] leading-[18px]" style={{ color: colors.subText }}>
          {raw.collection
            ? collectionDateRange(raw.collection)
            : <>{formatCardDate(item.start_time)}{item.start_time ? ` · ${formatTime(item.start_time)}` : ''}</>}
        </p>
        {item.venue_name && (
          <p className="text-[12px] leading-[18px] truncate" style={{ color: colors.subText }}>
            {item.venue_name}{item.city ? `, ${item.city}` : ''}
          </p>
        )}
      </div>
    </button>
  );
}

export default function SectionPage() {
  const router  = useRouter();
  const { colors, isDark } = useAppTheme();
  const { sectionMeta, allEvents, dbCategories, setSelectedEvent } = useEventStore();

  const [events,  setEvents]  = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [scrolled, setScrolled] = useState(false);

  const label      = sectionMeta?.title ?? '';
  const isTrending = sectionMeta?.sectionId === 'trending';

  useEffect(() => {
    window.scrollTo({ top: 0, behavior: 'instant' });
    const onScroll = () => setScrolled(window.scrollY > 10);
    window.addEventListener('scroll', onScroll);
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

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
      const slug  = sectionId.replace(/^cat-/, '');
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

  const handlePress = (item: Event) => {
    // A folded collection opens its own page, not the standing-in date.
    if (item.collection) { router.push(collectionHref(item.collection)); return; }
    setSelectedEvent(item); router.push(`/events/${item.id}`);
  };

  const [hero, ...rest] = events;

  return (
    <div className="min-h-screen" style={{ backgroundColor: colors.background }}>

      {/* Header */}
      <div
        className="sticky top-0 z-20 flex items-center px-6 py-3 transition-all duration-200"
        style={{
          backgroundColor: scrolled
            ? (isDark ? 'rgba(0,0,0,0.85)' : 'rgba(255,255,255,0.85)')
            : colors.background,
          backdropFilter: scrolled ? 'blur(16px)' : 'none',
          borderBottom: `1px solid ${scrolled ? colors.dividerColor : 'transparent'}`,
        }}
      >
        <button onClick={() => router.back()} className="flex items-center gap-1.5" style={{ color: colors.text }}>
          <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M12.707 5.293a1 1 0 010 1.414L9.414 10l3.293 3.293a1 1 0 01-1.414 1.414l-4-4a1 1 0 010-1.414l4-4a1 1 0 011.414 0z" clipRule="evenodd" />
          </svg>
          <span className="text-sm font-medium" style={{ color: colors.subText }}>Back</span>
        </button>
        <div className="flex-1 text-center">
          <p className="font-black text-base tracking-tight" style={{ color: colors.text }}>{label}</p>
        </div>
        <span className="text-xs font-semibold w-16 text-right" style={{ color: colors.subText }}>
          {!loading && `${events.length} events`}
        </span>
      </div>

      {loading ? (
        <div className="px-6 pt-6 pb-12">
          <div className="mb-6"><SkeletonCard large /></div>
          <div className="grid grid-cols-4 gap-x-5 gap-y-8">
            {Array.from({ length: 8 }).map((_, i) => <SkeletonCard key={i} />)}
          </div>
        </div>

      ) : events.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-40 px-8 text-center gap-3">
          <span className="text-5xl">🎟</span>
          <p className="font-bold text-xl" style={{ color: colors.text }}>No events found</p>
          <p className="text-sm" style={{ color: colors.subText }}>There are no events in this section right now.</p>
        </div>

      ) : (
        <div className="px-6 pt-6 pb-16">
          {hero && <div className="mb-8"><HeroCard item={hero} onPress={handlePress} colors={colors} label={label} /></div>}
          <div className="grid grid-cols-4 gap-x-5 gap-y-8">
            {rest.map((item, i) => (
              <GridCard
                key={item.id}
                item={item}
                rank={undefined}
                onPress={handlePress}
                colors={colors}
                isDark={isDark}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}