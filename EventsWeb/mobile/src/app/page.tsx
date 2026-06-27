'use client';

import { useState, useEffect, useCallback, useRef, useMemo, memo } from 'react';
import { useRouter } from 'next/navigation';
import { useFilterStore } from '@/store/filterStore';
import {
  getHomeSections, loadMoreSectionEvents, getCategories,
  type Event, type HomeSection,
} from '@/lib/api';
import { useEventStore } from '@/store/eventsStore';
import { useAuthStore } from '@/store/authStore';
import FilterModal, {
  type FilterParams, buildFilterQueryString, formatDateRangeLabel, getBaseUrl,
} from '@/components/filterModal';
import { useAppTheme } from '@/themes/colors';
import { CoverImage } from '@/components/coverImage';

// ── Spotlight configs (matches native app exactly) ────────────────────────────
const SPOTLIGHT_CONFIGS = [
  { title: 'SPORTS ELITE',   category: 'sports',   badge: '⚡ SPORTS',   accentColor: '#e8c14a', bgColor: '#1a1200', gradientDark: 'rgba(26,18,0,0.82)',  gradientLight: 'rgba(26,18,0,0.25)'  },
  { title: 'BEST FESTIVALS', category: 'music',    badge: '🎪 FESTIVAL', accentColor: '#ff3c6e', bgColor: '#1a0010', gradientDark: 'rgba(26,0,16,0.82)',  gradientLight: 'rgba(26,0,16,0.25)'  },
  { title: 'GALA NIGHTS',    category: 'gala',     badge: '✨ GALA',     accentColor: '#c084fc', bgColor: '#0e0018', gradientDark: 'rgba(14,0,24,0.82)',  gradientLight: 'rgba(14,0,24,0.25)'  },
  { title: 'TECH SUMMIT',    category: 'tech',     badge: '💻 TECH',     accentColor: '#38bdf8', bgColor: '#00101a', gradientDark: 'rgba(0,16,26,0.82)',  gradientLight: 'rgba(0,16,26,0.25)'  },
  { title: 'FOOD & CULTURE', category: 'food',     badge: '🍽 FOOD',     accentColor: '#fb923c', bgColor: '#1a0800', gradientDark: 'rgba(26,8,0,0.82)',   gradientLight: 'rgba(26,8,0,0.25)'   },
  { title: 'TOP CONCERTS',   category: 'concerts', badge: '🎵 MUSIC',    accentColor: '#34d399', bgColor: '#001a10', gradientDark: 'rgba(0,26,16,0.82)',  gradientLight: 'rgba(0,26,16,0.25)'  },
];

function formatPrice(price: number) {
  if (price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function groupEventsIntoSections(events: Event[]): HomeSection[] {
  const order: string[] = [];
  const groups: Record<string, Event[]> = {};
  for (const e of events) {
    const cat = (e.category?.trim() || 'Other').replace(/\b\w/g, (c) => c.toUpperCase());
    if (!groups[cat]) { groups[cat] = []; order.push(cat); }
    if (groups[cat].length < 10) groups[cat].push(e);
  }
  const sections: HomeSection[] = order
    .filter((cat) => groups[cat].length >= 3)
    .map((cat) => ({ id: `cat-${cat.toLowerCase().replace(/\s+/g, '-')}`, title: cat.toUpperCase(), events: groups[cat], next_cursor: '' }))
    .sort((a, b) => b.events.length - a.events.length);
  const free = events.filter((e) => e.price === 0).slice(0, 10);
  if (free.length >= 2) sections.push({ id: 'free', title: 'FREE EVENTS', events: free, next_cursor: '' });
  if (!sections.length && events.length) sections.push({ id: 'results', title: 'RESULTS', events: events.slice(0, 10), next_cursor: '' });
  return sections;
}

const fmtCardDate = (iso?: string | null) => {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short', year: 'numeric' });
};

// ── Skeleton — matches native app shimmer style ───────────────────────────────
function SkeletonCard() {
  return (
    <div className="shrink-0 w-[200px] animate-pulse">
      <div className="w-full h-[130px] rounded-[14px] bg-white/10 mb-[10px]" />
      <div className="h-[9px] w-20 rounded bg-white/10 mb-1.5" />
      <div className="h-[14px] w-full rounded bg-white/10 mb-1" />
      <div className="h-[11px] w-3/4 rounded bg-white/10" />
    </div>
  );
}
function SkeletonSection() {
  return (
    <div className="mt-7 px-5">
      <div className="h-[11px] w-36 rounded bg-white/10 mb-4 animate-pulse" />
      <div className="flex gap-3 overflow-hidden pb-1">
        {[0, 1, 2].map((i) => <SkeletonCard key={i} />)}
      </div>
    </div>
  );
}
function SkeletonFeed() {
  return (
    <>
      <div className="w-full h-[380px] bg-white/6 animate-pulse mt-[130px] mx-4 rounded-[14px]" style={{ width: 'calc(100% - 32px)' }} />
      <SkeletonSection /><SkeletonSection /><SkeletonSection />
    </>
  );
}

// ── Event Card — 200×130px matching native exactly ────────────────────────────
const EventCard = memo(({ item, onPress, colors, isTop10 = false, rank }: {
  item: Event; onPress: () => void; colors: any; isTop10?: boolean; rank?: number;
}) => (
  <button onClick={onPress} className="shrink-0 w-[200px] text-left">
    <div className="relative w-full h-[130px] rounded-[14px] overflow-hidden" style={{ backgroundColor: colors.sectionCardBg }}>
      {item.cover_image_url && (
        <CoverImage uri={item.cover_image_url} alt={item.title} className="absolute inset-0 w-full h-full object-cover" />
      )}
      {/* Top 10 rank number overlay */}
      {isTop10 && rank !== undefined && (
        <>
          <div className="absolute inset-0" style={{ background: 'linear-gradient(to right, rgba(0,0,0,0.72), rgba(0,0,0,0.18), transparent)' }} />
          <span className="absolute top-[6px] left-[10px] text-[52px] font-black text-white leading-none z-10"
            style={{ letterSpacing: -2, textShadow: '0 2px 6px rgba(0,0,0,0.4)' }}>
            {rank}
          </span>
        </>
      )}
      {/* LIVE badge */}
      {!isTop10 && (item.category?.toLowerCase().includes('music') || item.category?.toLowerCase().includes('concert')) && (
        <div className="absolute top-[10px] left-[10px] bg-black/70 rounded-[5px] px-[10px] py-1 flex items-center gap-1">
          <div className="w-[5px] h-[5px] rounded-full bg-white" />
          <span className="text-white text-[9px] font-extrabold tracking-[1.2px]">LIVE</span>
        </div>
      )}
      {/* Price badge */}
      <div className="absolute top-[10px] right-[10px] bg-black/60 rounded-[5px] px-[10px] py-1">
        <span className="text-white text-[9px] font-bold">{formatPrice(item.price)}</span>
      </div>
    </div>
    <div className="pt-[10px] px-[2px]">
      <p className="text-[10px] font-semibold tracking-[0.5px] mb-[2px] truncate" style={{ color: colors.subText }}>{fmtCardDate(item.start_time)}</p>
      <p className="text-[14px] font-bold leading-[18px] mb-1 line-clamp-2" style={{ color: colors.text }}>{item.title}</p>
      <p className="text-[11px] truncate" style={{ color: colors.subText }}>{item.venue_name}</p>
    </div>
  </button>
));
EventCard.displayName = 'EventCard';

// ── Section Row — horizontal scroll with load more ────────────────────────────
const SectionRow = memo(({
  section, isTop10 = false, onEventPress, onViewAll, colors,
}: {
  section: HomeSection; isTop10?: boolean;
  onEventPress: (e: Event) => void; onViewAll: (id: string, title: string, events: Event[]) => void; colors: any;
}) => {
  const [events,      setEvents]      = useState<Event[]>(section.events ?? []);
  const [nextCursor,  setNextCursor]  = useState(section.next_cursor ?? '');
  const [loadingMore, setLoadingMore] = useState(false);

  useEffect(() => {
    setEvents(section.events ?? []);
    setNextCursor(section.next_cursor ?? '');
  }, [section]);

  const handleLoadMore = useCallback(async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    const result = await loadMoreSectionEvents(section.id, nextCursor);
    if (result && result.data.length > 0) {
      setEvents((prev) => {
        const ids = new Set(prev.map((e) => e.id));
        return [...prev, ...result.data.filter((e) => !ids.has(e.id))];
      });
      setNextCursor(result.next_cursor ?? '');
    } else {
      setNextCursor('');
    }
    setLoadingMore(false);
  }, [nextCursor, loadingMore, section.id]);

  if (!events.length) return null;

  return (
    <div className="mt-7">
      {/* Section header */}
      <div className="px-5 mb-4 flex items-center justify-between">
        <h2 className="text-[22px] font-black tracking-[0.5px]" style={{ color: colors.text }}>
          {section.title.toUpperCase()}
        </h2>
        <button
          onClick={() => onViewAll(section.id, section.title, events)}
          className="text-[10px] font-extrabold text-[#FF2D55] tracking-[1.5px]"
        >
          VIEW ALL
        </button>
      </div>
      {/* Horizontal scroll — matches native FlatList contentContainerStyle */}
      <div className="flex overflow-x-auto gap-3 px-5 pb-1 scrollbar-hide">
        {events.map((item, index) => (
          <EventCard
            key={item.id}
            item={item}
            isTop10={isTop10}
            rank={isTop10 ? index + 1 : undefined}
            onPress={() => onEventPress(item)}
            colors={colors}
          />
        ))}
        {nextCursor && (
          <button
            onClick={handleLoadMore}
            disabled={loadingMore}
            className="shrink-0 w-14 flex items-center justify-center text-xs font-bold"
            style={{ color: colors.subText }}
          >
            {loadingMore ? '...' : 'More'}
          </button>
        )}
      </div>
    </div>
  );
});
SectionRow.displayName = 'SectionRow';

// ── Editorial Section — same layout as SectionRow ─────────────────────────────
function EditorialSection({ title, events, filterKey, colors, onViewAll, onEventPress }: {
  title: string; events: Event[]; filterKey: string; colors: any;
  onViewAll: (filterKey: string, title: string) => void;
  onEventPress: (event: Event) => void;
}) {
  if (!events.length) return null;
  return (
    <div className="mt-7">
      <div className="px-5 mb-4 flex items-center justify-between">
        <h2 className="text-[22px] font-black tracking-[0.5px]" style={{ color: colors.text }}>{title}</h2>
        <button onClick={() => onViewAll(filterKey, title)} className="text-[10px] font-extrabold text-[#FF2D55] tracking-[1.5px]">VIEW ALL</button>
      </div>
      <div className="flex overflow-x-auto gap-3 px-5 pb-1 scrollbar-hide">
        {events.map((item) => <EventCard key={item.id} item={item} onPress={() => onEventPress(item)} colors={colors} />)}
      </div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────
export default function HomePage() {
  const router = useRouter();
  const setSelectedEvent    = useEventStore((s) => s.setSelectedEvent);
  const setAllEventsStore   = useEventStore((s) => s.setAllEvents);
  const setDbCategories     = useEventStore((s) => s.setDbCategories);
  const setSectionMeta      = useEventStore((s) => s.setSectionMeta);
  const recentlyViewedIds   = useEventStore((s) => s.recentlyViewedIds);
  const recentlyViewedCache = useEventStore((s) => s.recentlyViewedCache);
  const loadRecentlyViewed  = useEventStore((s) => s.loadRecentlyViewed);
  const clearRecentlyViewed = useEventStore((s) => s.clearRecentlyViewed);
  const authUserId          = useAuthStore((s) => s.user?.id ?? null);
  const authReady           = useAuthStore((s) => s.authReady);
  const { colors, isDark }  = useAppTheme();

  const [spotlightConfig, setSpotlightConfig] = useState(
    () => SPOTLIGHT_CONFIGS[Math.floor(Math.random() * SPOTLIGHT_CONFIGS.length)]
  );
  const [activeCategory, setActiveCategory] = useState('All');
  const [dropdownOpen,   setDropdownOpen]   = useState(false);
  const [filterParams,   setFilterParams]   = useState<FilterParams>({ date: 'any' });
  const [sections,       setSections]       = useState<HomeSection[]>([]);
  const [homeSections,   setHomeSections]   = useState<HomeSection[]>([]);
  const [loading,        setLoading]        = useState(true);
  const [error,          setError]          = useState<string | null>(null);
  const [featuredIndex,  setFeaturedIndex]  = useState(0);
  const featuredIndexRef = useRef(0);
  const didInitialFetch  = useRef(false);
  const prevAuthUserId   = useRef<string | null>(null);

  const { filterOpen, setFilterOpen, setLocationLabel, setDateLabelText } = useFilterStore();

  const isFiltered = !!(filterParams.city || filterParams.province || filterParams.lat || (filterParams.date && filterParams.date !== 'any'));

  const carouselEvents = useMemo<Event[]>(() => {
    const rawFeatured  = homeSections.find((s) => s.id === 'featured')?.events ?? [];
    const trendingPool = homeSections.find((s) => s.id === 'trending')?.events ?? [];
    return rawFeatured.length >= 2
      ? rawFeatured.slice(0, 6)
      : [...rawFeatured, ...trendingPool.filter((e) => !rawFeatured.some((f) => f.id === e.id))].slice(0, 5);
  }, [homeSections]);

  const allEvents = useMemo(() => {
    const seen = new Set<string>();
    const result: Event[] = [];
    for (const sec of sections) {
      if (sec.id === 'featured') continue;
      for (const e of sec.events ?? []) {
        if (!seen.has(e.id)) { seen.add(e.id); result.push(e); }
      }
    }
    return result;
  }, [sections]);

  const feedSections       = useMemo(() => sections.filter((s) => s.id !== 'featured'), [sections]);
  const freeEvents         = useMemo(() => allEvents.filter((e) => e.price === 0).slice(0, 10), [allEvents]);
  const premiumEvents      = useMemo(() => allEvents.filter((e) => e.price >= 3000).slice(0, 10), [allEvents]);
  const comingUp           = useMemo(() => { const now = Date.now(); return allEvents.filter((e) => { const d = (new Date(e.start_time).getTime() - now) / 86400000; return d >= 15 && d <= 45; }).slice(0, 10); }, [allEvents]);
  const otherCities        = useMemo(() => allEvents.filter((e) => !['Johannesburg', 'johannesburg'].includes(e.city ?? '')).slice(0, 10), [allEvents]);
  const musicEvents        = useMemo(() => allEvents.filter((e) => e.category?.toLowerCase().includes('concert') || e.category?.toLowerCase().includes('music')).slice(0, 10), [allEvents]);
  const galaEvents         = useMemo(() => allEvents.filter((e) => e.category?.toLowerCase().includes('gala') || e.category?.toLowerCase().includes('exclusive')).slice(0, 10), [allEvents]);
  const techEvents         = useMemo(() => allEvents.filter((e) => e.category?.toLowerCase().includes('tech')).slice(0, 10), [allEvents]);
  const foodEvents         = useMemo(() => allEvents.filter((e) => e.category?.toLowerCase().includes('food')).slice(0, 10), [allEvents]);

  const spotlightEvents = useMemo(() => {
    const p = allEvents.filter((e) => e.category?.toLowerCase().includes(spotlightConfig.category)).slice(0, 4);
    if (p.length) return p;
    for (const cfg of SPOTLIGHT_CONFIGS) {
      if (cfg.category === spotlightConfig.category) continue;
      const f = allEvents.filter((e) => e.category?.toLowerCase().includes(cfg.category)).slice(0, 4);
      if (f.length) return f;
    }
    return allEvents.slice(0, 4);
  }, [allEvents, spotlightConfig]);

  const sportsBannerEvent = spotlightEvents[0] ?? null;
  const sportsListEvents  = spotlightEvents.slice(1);

  const recentlyViewedEvents = useMemo<Event[]>(() => {
    if (!authUserId || !recentlyViewedIds.length) return [];
    const map = new Map(allEvents.map((e) => [e.id, e]));
    return recentlyViewedIds.flatMap((id) => { const e = recentlyViewedCache[id] ?? map.get(id); return e ? [e] : []; });
  }, [recentlyViewedIds, recentlyViewedCache, allEvents, authUserId]);

  const categories = useMemo(() => {
    const seen = new Set<string>();
    const cats: string[] = [];
    for (const e of allEvents) {
      const raw = e.category?.trim();
      if (!raw) continue;
      const key = raw.toLowerCase();
      if (!seen.has(key)) { seen.add(key); cats.push(raw.replace(/\b\w/g, (c) => c.toUpperCase())); }
    }
    return ['All', ...cats.sort((a, b) => a.localeCompare(b))];
  }, [allEvents]);

  // Auto-advance hero carousel
  useEffect(() => {
    if (carouselEvents.length <= 1) return;
    const t = setInterval(() => {
      const next = (featuredIndexRef.current + 1) % carouselEvents.length;
      featuredIndexRef.current = next;
      setFeaturedIndex(next);
    }, 5000);
    return () => clearInterval(t);
  }, [carouselEvents.length]);

  useEffect(() => { if (allEvents.length > 0) setAllEventsStore(allEvents); }, [allEvents]);

  useEffect(() => {
    if (!allEvents.length) return;
    const hasEvents = (cfg: typeof SPOTLIGHT_CONFIGS[0]) => allEvents.some((e) => e.category?.toLowerCase().includes(cfg.category));
    if (hasEvents(spotlightConfig)) return;
    const fallback = SPOTLIGHT_CONFIGS.find((cfg) => cfg.category !== spotlightConfig.category && hasEvents(cfg));
    if (fallback) setSpotlightConfig(fallback);
  }, [allEvents, spotlightConfig]);

  const fetchSections = useCallback(async (city?: string) => {
    try {
      setError(null);
      const [sectionsRes, categoriesRes] = await Promise.all([
        getHomeSections(city),
        getCategories().catch(() => null),
      ]);
      const loaded = sectionsRes.data.sections ?? [];
      setSections(loaded);
      setHomeSections(loaded);
      const cats = categoriesRes?.data?.categories ?? [];
      if (cats.length > 0) setDbCategories(cats);
    } catch (err: any) {
      setError(err?.message ?? 'Failed to load. Check your API URL.');
    }
  }, []);

  useEffect(() => {
    if (!authReady || didInitialFetch.current) return;
    didInitialFetch.current = true;
    prevAuthUserId.current = authUserId;
    if (authUserId) loadRecentlyViewed(authUserId);
    fetchSections(filterParams.city).finally(() => setLoading(false));
  }, [authReady, authUserId, fetchSections, filterParams.city, loadRecentlyViewed]);

  useEffect(() => {
    if (!didInitialFetch.current) return;
    if (authUserId === prevAuthUserId.current) return;
    prevAuthUserId.current = authUserId;
    if (authUserId) loadRecentlyViewed(authUserId);
    else clearRecentlyViewed();
    fetchSections(filterParams.city);
  }, [authUserId, fetchSections, filterParams.city, loadRecentlyViewed, clearRecentlyViewed]);

  const handleFilterApply = useCallback(async (params: FilterParams) => {
    setFilterParams(params);
    setFilterOpen(false);
    if (params.city) fetchSections(params.city);
    else if (!params.city && !params.province && !params.lat) fetchSections();
    const hasLocation = !!(params.city || params.province || params.lat);
    const hasDate     = !!(params.date && params.date !== 'any');
    if (!hasLocation && !hasDate) { setSections(homeSections); return; }
    try {
      setLoading(true); setError(null);
      const qs  = buildFilterQueryString(params);
      const res = await fetch(`${getBaseUrl()}/v1/events/filter?${qs}`);
      if (!res.ok) throw new Error(`Filter failed: ${res.status}`);
      const data = await res.json();
      if (Array.isArray(data.sections) && data.sections.length > 0) setSections(data.sections);
      else if (Array.isArray(data.data) && data.data.length > 0) setSections(groupEventsIntoSections(data.data));
      else { setSections(homeSections); setError('No events found. Showing all events.'); }
    } catch { setSections(homeSections); }
    finally { setLoading(false); }
  }, [homeSections, fetchSections]);

  const handleEventPress = useCallback((event: Event) => {
    setSelectedEvent(event);
    router.push(`/events/${event.id}`);
  }, [setSelectedEvent, router]);

  const handleViewAll = useCallback((sectionId: string, title: string, events: Event[]) => {
    setSectionMeta({ title, sectionId, events });
    router.push(`/section/${encodeURIComponent(title)}`);
  }, [setSectionMeta, router]);

  const handleViewAllEditorial = useCallback((filterKey: string, title: string) => {
    setSectionMeta({ title, filterKey });
    router.push(`/section/${encodeURIComponent(title)}`);
  }, [setSectionMeta, router]);

  const locationLabel  = filterParams.city ?? filterParams.province ?? (filterParams.lat ? 'Near me' : 'Anywhere');
  const dateLabelText  =
    filterParams.date === 'any'      ? 'All dates' :
    filterParams.date === 'tomorrow' ? 'Tomorrow'  :
    filterParams.date === 'weekend'  ? 'This Weekend' :
    filterParams.date === 'custom' && filterParams.date_from && filterParams.date_to
      ? formatDateRangeLabel(filterParams.date_from, filterParams.date_to)
      : 'All dates';

  useEffect(() => {
    setLocationLabel(locationLabel);
    setDateLabelText(dateLabelText);
  }, [locationLabel, dateLabelText]);

  return (
    <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>

      {/* ── Floating location header (matches native Animated.View) ─────────── */}
      <div
        className="sticky top-16 z-30 flex items-center justify-between px-5 pt-6 pb-6"
        style={{ backgroundColor: colors.background }}
      >
        <button onClick={() => setFilterOpen(true)} className="text-left">
          <p className="text-[22px] font-extrabold tracking-[0.3px]" style={{ color: colors.text }}>{locationLabel}</p>
          <p className="text-[17px] font-extrabold mt-[2px]" style={{ color: colors.subText }}>{dateLabelText}</p>
        </button>
      </div>

      {/* ── Hero carousel (mx-4 rounded-[14px] matching native) ─────────────── */}
      <div className="mx-4 rounded-[14px] overflow-hidden relative" style={{ height: 380 }}>
        {carouselEvents.length === 0 ? (
          <div className="absolute inset-0 bg-white/10 animate-pulse rounded-[14px]" />
        ) : (
          <>
            {carouselEvents.map((evt, idx) => (
              <div
                key={evt.id}
                className="absolute inset-0 w-full h-full transition-opacity duration-500"
                style={{ opacity: idx === featuredIndex ? 1 : 0, pointerEvents: idx === featuredIndex ? 'auto' : 'none' }}
              >
                {(evt.banner_image_url ?? evt.cover_image_url) && (
                  <img
                    src={evt.banner_image_url ?? evt.cover_image_url!}
                    alt={evt.title}
                    className="absolute inset-0 w-full h-full object-cover"
                  />
                )}
                {/* Gradient: transparent → dark — matches native LinearGradient locations [0.3, 0.7, 1] */}
                <div
                  className="absolute inset-0 cursor-pointer"
                  onClick={() => handleEventPress(evt)}
                  style={{ background: 'linear-gradient(to top, #0a0a0a 0%, rgba(10,10,10,0.85) 30%, transparent 70%)' }}
                />
                {/* Content — bottom left, matches native heroContent */}
                <div className="absolute bottom-0 left-0 right-0 p-5 pb-8 pointer-events-none">
                  <p className="text-[11px] font-bold text-white/70 tracking-[2px] uppercase mb-[10px]">
                    ✦ {evt.category ?? 'Featured'}
                  </p>
                  <h1 className="text-[42px] font-black text-white leading-[42px] mb-3 line-clamp-2"
                    style={{ letterSpacing: 2 }}>
                    {evt.title.toUpperCase()}
                  </h1>
                  <p className="text-[13px] text-white/80 leading-5 mb-5 max-w-[300px] line-clamp-3">
                    {evt.description}
                  </p>
                  <div className="flex items-center gap-3 pointer-events-auto">
                    <button
                      onClick={() => handleEventPress(evt)}
                      className="bg-[#FF2D55] text-white font-extrabold text-[11px] tracking-[1.5px] px-[22px] py-3 rounded-[2px]"
                    >
                      🎟 GET TICKETS
                    </button>
                    <button
                      onClick={() => handleEventPress(evt)}
                      className="bg-[#1c2e61] text-white font-bold text-[11px] tracking-[1px] px-[22px] py-3 rounded-[2px]"
                    >
                      VIEW DETAILS
                    </button>
                  </div>
                </div>
              </div>
            ))}
            {/* Dot indicators — bottom right */}
            {carouselEvents.length > 1 && (
              <div className="absolute bottom-[14px] right-4 flex items-center gap-[5px] z-10">
                {carouselEvents.map((_, i) => (
                  <button
                    key={i}
                    onClick={() => { featuredIndexRef.current = i; setFeaturedIndex(i); }}
                    className="h-[6px] rounded-full transition-all duration-300"
                    style={{
                      width: i === featuredIndex ? 18 : 6,
                      backgroundColor: i === featuredIndex ? '#FF2D55' : 'rgba(255,255,255,0.4)',
                    }}
                  />
                ))}
              </div>
            )}
          </>
        )}
      </div>

      {/* ── Category filter row (matches native filterRow) ───────────────────── */}
      <div className="flex items-center px-5 py-[14px] gap-[10px]">
        {activeCategory !== 'All' && (
          <button
            onClick={() => setActiveCategory('All')}
            className="w-[34px] h-[34px] rounded-[2px] border flex items-center justify-center text-xs"
            style={{ borderColor: colors.dividerColor, color: colors.text }}
          >
            ✕
          </button>
        )}
        <button
          onClick={() => setDropdownOpen(true)}
          className="flex items-center px-4 py-2 rounded-[14px] border"
          style={{ borderColor: colors.text }}
        >
          <span className="text-[10px] font-extrabold tracking-[1.2px]" style={{ color: colors.text }}>
            {activeCategory === 'All' ? 'CATEGORIES' : activeCategory.toUpperCase()}
          </span>
          <span className="text-[11px] ml-[6px]" style={{ color: colors.text }}> ▾</span>
        </button>
      </div>

      {/* ── Category dropdown overlay (matches native Modal + BlurView) ──────── */}
      {dropdownOpen && (
        <div
          className="fixed inset-0 z-50 flex flex-col"
          style={{ backdropFilter: 'blur(24px)', backgroundColor: isDark ? 'rgba(0,0,0,0.75)' : 'rgba(255,255,255,0.75)' }}
          onClick={() => setDropdownOpen(false)}
        >
          <div className="flex-1 w-full pt-16 pb-10 flex flex-col items-center overflow-y-auto" onClick={(e) => e.stopPropagation()}>
            {categories.map((item) => (
              <button
                key={item}
                onClick={() => {
                  setDropdownOpen(false);
                  if (item === 'All') setActiveCategory('All');
                  else router.push(`/category/${encodeURIComponent(item)}`);
                }}
                className="py-[18px] px-10 w-full text-left"
              >
                <span
                  className={activeCategory === item ? 'text-[26px] font-black tracking-[1px]' : 'text-[26px] font-normal'}
                  style={{ color: activeCategory === item ? colors.text : colors.subText }}
                >
                  {item}
                </span>
              </button>
            ))}
          </div>
          <div className="pb-10 flex justify-center">
            <button
              onClick={() => setDropdownOpen(false)}
              className="w-12 h-12 rounded-full flex items-center justify-center font-bold text-base"
              style={{ backgroundColor: colors.sectionCardBg, color: colors.text }}
            >
              ✕
            </button>
          </div>
        </div>
      )}

      {/* ── Error banner ─────────────────────────────────────────────────────── */}
      {error && (
        <div className="mx-5 mt-5 p-[14px] rounded-[3px] border" style={{ backgroundColor: 'rgba(255,60,110,0.12)', borderColor: colors.dividerColor }}>
          <p className="font-extrabold text-[12px] tracking-[1px] mb-1" style={{ color: colors.text }}>⚠ COULD NOT LOAD SECTIONS</p>
          <p className="text-[11px]" style={{ color: colors.subText }}>{error}</p>
          <button onClick={() => fetchSections()} className="mt-[10px] border px-[14px] py-[6px] rounded-[2px] text-[10px] font-extrabold tracking-[1.5px]" style={{ borderColor: colors.text, color: colors.text }}>RETRY</button>
        </div>
      )}

      {/* ── Recently viewed (logged-in only) ─────────────────────────────────── */}
      {authUserId && recentlyViewedEvents.length > 0 && (
        <div className="mt-7">
          <div className="px-5 mb-4">
            <h2 className="text-[22px] font-black tracking-[0.5px]" style={{ color: colors.text }}>RECENTLY VIEWED</h2>
          </div>
          <div className="flex overflow-x-auto gap-3 px-5 pb-1 scrollbar-hide">
            {recentlyViewedEvents.map((item) => (
              <EventCard key={item.id} item={item} onPress={() => handleEventPress(item)} colors={colors} />
            ))}
          </div>
        </div>
      )}

      {/* ── Feed sections ─────────────────────────────────────────────────────── */}
      {loading ? <SkeletonFeed /> : feedSections.map((section) => (
        <SectionRow key={section.id} section={section} isTop10={section.id === 'trending'} onEventPress={handleEventPress} onViewAll={handleViewAll} colors={colors} />
      ))}

      {/* ── Spotlight section (matches native sportsBanner + sportsListEvents) ── */}
      {!loading && !isFiltered && sportsBannerEvent && (
        <>
          <div className="mt-7 px-5 mb-4 flex items-center justify-between">
            <h2 className="text-[22px] font-black tracking-[0.5px]" style={{ color: spotlightConfig.accentColor }}>{spotlightConfig.title}</h2>
            <button className="text-[10px] font-extrabold tracking-[1.5px]" style={{ color: spotlightConfig.accentColor }}>EXPLORE</button>
          </div>
          {/* Banner */}
          <button
            onClick={() => handleEventPress(sportsBannerEvent)}
            className="relative mx-5 rounded-[3px] overflow-hidden text-left w-[calc(100%-40px)]"
            style={{ minHeight: 200, backgroundColor: spotlightConfig.bgColor }}
          >
            {(sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url) && (
              <img
                src={sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url!}
                alt={sportsBannerEvent.title}
                className="absolute inset-0 w-full h-full object-cover"
              />
            )}
            <div className="absolute inset-0" style={{ background: `linear-gradient(to bottom, ${spotlightConfig.gradientLight}, ${spotlightConfig.gradientDark})` }} />
            <div className="absolute bottom-0 left-0 right-0 h-[3px]" style={{ backgroundColor: spotlightConfig.accentColor, opacity: 0.85 }} />
            <div className="relative p-6 z-10">
              <p className="text-[10px] font-extrabold tracking-[2px] uppercase mb-[6px]" style={{ color: spotlightConfig.accentColor }}>{spotlightConfig.badge}</p>
              <h2 className="text-[28px] font-black text-white tracking-[1px] mb-2 leading-[32px]">{sportsBannerEvent.title.toUpperCase()}</h2>
              <p className="text-xs text-white/50 mb-4 line-clamp-2">{sportsBannerEvent.description}</p>
              <span className="border border-white/50 text-white font-bold text-[11px] tracking-[1px] px-[22px] py-3 rounded-[2px] inline-block">
                VIEW TICKETS
              </span>
            </div>
          </button>
          {/* Sport list rows — matches native sportsListContainer */}
          {sportsListEvents.length > 0 && (
            <div className="mt-4 px-5">
              {sportsListEvents.map((sport, index) => (
                <button
                  key={sport.id}
                  onClick={() => handleEventPress(sport)}
                  className="flex items-center gap-[14px] py-[14px] w-full text-left"
                  style={{ borderBottom: index < sportsListEvents.length - 1 ? `1px solid ${colors.dividerColor}` : 'none' }}
                >
                  <div className="w-32 h-16 rounded-[8px] shrink-0 overflow-hidden relative" style={{ backgroundColor: colors.sectionCardBg }}>
                    {sport.cover_image_url && (
                      <img src={sport.cover_image_url} alt={sport.title} className="absolute inset-0 w-full h-full object-cover" />
                    )}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-[10px] font-semibold tracking-[0.5px] mb-[2px]" style={{ color: colors.subText }}>{sport.category}</p>
                    <p className="text-[14px] font-semibold mb-[2px] truncate" style={{ color: colors.text }}>{sport.title}</p>
                    <p className="text-[11px]" style={{ color: colors.subText }}>{sport.city}</p>
                  </div>
                  <span className="text-[20px] shrink-0" style={{ color: spotlightConfig.accentColor }}>›</span>
                </button>
              ))}
            </div>
          )}
        </>
      )}

      {/* ── Editorial sections ───────────────────────────────────────────────── */}
      {!loading && !isFiltered && (
        <>
          <EditorialSection title="FREE EVENTS"         events={freeEvents}    filterKey="free"         colors={colors} onViewAll={handleViewAllEditorial} onEventPress={handleEventPress} />
          <EditorialSection title="PREMIUM & EXCLUSIVE" events={premiumEvents} filterKey="premium"      colors={colors} onViewAll={handleViewAllEditorial} onEventPress={handleEventPress} />
          <EditorialSection title="COMING NEXT MONTH"   events={comingUp}      filterKey="coming_up"    colors={colors} onViewAll={handleViewAllEditorial} onEventPress={handleEventPress} />
          <EditorialSection title="AROUND THE COUNTRY"  events={otherCities}   filterKey="other_cities" colors={colors} onViewAll={handleViewAllEditorial} onEventPress={handleEventPress} />
          <EditorialSection title="MUSIC & CONCERTS"    events={musicEvents}   filterKey="music"        colors={colors} onViewAll={handleViewAllEditorial} onEventPress={handleEventPress} />
          <EditorialSection title="GALA & BLACK TIE"    events={galaEvents}    filterKey="gala"         colors={colors} onViewAll={handleViewAllEditorial} onEventPress={handleEventPress} />
          <EditorialSection title="TECH & BUSINESS"     events={techEvents}    filterKey="tech"         colors={colors} onViewAll={handleViewAllEditorial} onEventPress={handleEventPress} />
          <EditorialSection title="FOOD & LIFESTYLE"    events={foodEvents}    filterKey="food"         colors={colors} onViewAll={handleViewAllEditorial} onEventPress={handleEventPress} />
        </>
      )}

      <div className="h-24" />

      <FilterModal visible={filterOpen} onClose={() => setFilterOpen(false)} initialParams={filterParams} onApply={handleFilterApply} />
    </div>
  );
}