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
  return new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
}
function formatTime(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

function SkeletonCard({ large = false }: { large?: boolean }) {
  return (
    <div
      className="animate-pulse rounded-2xl overflow-hidden"
      style={{ aspectRatio: large ? '16 / 4' : '16 / 9', backgroundColor: 'rgba(255,255,255,0.05)' }}
    />
  );
}

function HeroCard({ item, onPress, colors }: { item: Event; onPress: (e: Event) => void; colors: any }) {
  const [hovered, setHovered] = useState(false);
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
      {/* Price badge */}
      <div
        className="absolute top-4 right-4 rounded-lg px-3 py-1.5"
        style={{ backgroundColor: formatPrice(item.price) === 'Free' ? '#22c55e' : '#ff2d55' }}
      >
        <span className="text-white text-xs font-black tracking-wide">{formatPrice(item.price)}</span>
      </div>
      {/* Bottom content */}
      <div className="absolute bottom-0 left-0 right-0 p-4">
        <div className="flex items-center gap-2 mb-2">
          <div
            className="flex items-center gap-1 text-[10px] font-black tracking-[0.12em] uppercase px-2.5 py-1 rounded"
            style={{ backgroundColor: 'rgba(255,45,85,0.9)', color: '#fff' }}
          >
            🔥 Featured
          </div>
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
          {formatCardDate(item.start_time)}{item.start_time ? ` · ${formatTime(item.start_time)}` : ''}
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

function GridCard({ item, onPress, colors, isDark }: { item: Event; onPress: (e: Event) => void; colors: any; isDark: boolean }) {
  const [hovered, setHovered] = useState(false);
  return (
    <button
      onClick={() => onPress(item)}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="text-left w-full"
    >
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
        {/* Hover dim */}
        <div
          className="absolute inset-0 transition-opacity duration-200"
          style={{ backgroundColor: 'rgba(0,0,0,0.28)', opacity: hovered ? 1 : 0 }}
        />
        {/* Price */}
        <div
          className="absolute bottom-2 left-2 rounded-md px-2 py-0.5"
          style={{ backgroundColor: formatPrice(item.price) === 'Free' ? '#22c55e' : 'rgba(0,0,0,0.82)', backdropFilter: 'blur(4px)' }}
        >
          <span className="text-white text-[10px] font-black tracking-wide">{formatPrice(item.price)}</span>
        </div>
      </div>
      {/* Info */}
      <p
        className="font-semibold text-[14px] leading-[20px] line-clamp-2 mb-1 transition-colors duration-150"
        style={{ color: hovered ? '#ff2d55' : colors.text }}
      >
        {item.title}
      </p>
      <p className="text-[12px] leading-[18px]" style={{ color: colors.subText }}>
        {formatCardDate(item.start_time)}{item.start_time ? ` · ${formatTime(item.start_time)}` : ''}
      </p>
      {item.venue_name && (
        <p className="text-[12px] leading-[18px] truncate" style={{ color: colors.subText }}>
          {item.venue_name}{item.city ? `, ${item.city}` : ''}
        </p>
      )}
    </button>
  );
}

export default function CategoryPage() {
  const params     = useParams<{ name: string }>();
  const router     = useRouter();
  const { colors, isDark } = useAppTheme();
  const { dbCategories, setSelectedEvent } = useEventStore();

  const [events,  setEvents]  = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [error,   setError]   = useState<string | null>(null);
  const [scrolled, setScrolled] = useState(false);

  const decodedName   = params.name ? decodeURIComponent(params.name) : '';
  const categoryLabel = decodedName.replace(/\b\w/g, (c) => c.toUpperCase());

  useEffect(() => {
    window.scrollTo({ top: 0, behavior: 'instant' });
    const onScroll = () => setScrolled(window.scrollY > 10);
    window.addEventListener('scroll', onScroll);
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  const fetchEvents = useCallback(async () => {
    if (!decodedName) return;
    if (!dbCategories.length) return;
    setError(null);
    const match = dbCategories.find(
      (c) =>
        c.name.toLowerCase() === decodedName.toLowerCase() ||
        c.slug?.toLowerCase() === decodedName.toLowerCase()
    );
    if (!match) { setError(`Category "${categoryLabel}" not found.`); return; }
    try {
      const res  = await getEventsByCategory(match.id, 50);
      setEvents(res.data?.data ?? []);
    } catch (e: any) {
      setError(e?.message ?? 'Failed to load events.');
    }
  }, [decodedName, dbCategories, categoryLabel]);

  useEffect(() => {
    setLoading(true);
    fetchEvents().finally(() => setLoading(false));
  }, [fetchEvents]);

  const handlePress = (item: Event) => { setSelectedEvent(item); router.push(`/events/${item.id}`); };

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
        <button onClick={() => router.back()} className="flex items-center gap-1.5 group" style={{ color: colors.text }}>
          <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M12.707 5.293a1 1 0 010 1.414L9.414 10l3.293 3.293a1 1 0 01-1.414 1.414l-4-4a1 1 0 010-1.414l4-4a1 1 0 011.414 0z" clipRule="evenodd" />
          </svg>
          <span className="text-sm font-medium" style={{ color: colors.subText }}>Back</span>
        </button>
        <div className="flex-1 text-center">
          <p className="font-black text-base tracking-tight" style={{ color: colors.text }}>{categoryLabel}</p>
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

      ) : error ? (
        <div className="flex flex-col items-center justify-center py-40 px-8 text-center gap-4">
          <span className="text-5xl">⚠️</span>
          <p className="font-bold text-lg" style={{ color: colors.text }}>Something went wrong</p>
          <p className="text-sm" style={{ color: colors.subText }}>{error}</p>
          <button
            onClick={() => { setLoading(true); fetchEvents().finally(() => setLoading(false)); }}
            className="mt-2 px-6 py-2.5 rounded-full text-xs font-black tracking-widest uppercase"
            style={{ backgroundColor: '#ff2d55', color: '#fff' }}
          >
            Retry
          </button>
        </div>

      ) : events.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-40 px-8 text-center gap-3">
          <span className="text-5xl">🎟</span>
          <p className="font-bold text-xl" style={{ color: colors.text }}>No events yet</p>
          <p className="text-sm" style={{ color: colors.subText }}>
            Nothing in {categoryLabel.toLowerCase()} right now — check back soon.
          </p>
        </div>

      ) : (
        <div className="px-6 pt-6 pb-16">
          {hero && <div className="mb-8"><HeroCard item={hero} onPress={handlePress} colors={colors} /></div>}
          <div className="grid grid-cols-4 gap-x-5 gap-y-8">
            {rest.map((item) => (
              <GridCard key={item.id} item={item} onPress={handlePress} colors={colors} isDark={isDark} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}