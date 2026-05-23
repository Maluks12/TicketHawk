'use client';

import { useState, useEffect, useCallback, useRef, useMemo, memo } from 'react';
import { useRouter } from 'next/navigation';
import { ShoppingCart } from 'lucide-react';
import {
  getHomeSections, loadMoreSectionEvents, getCategories, getCart, getCachedSessionId,
  type Event, type HomeSection,
} from '@/lib/api';
import { useEventStore } from '@/store/eventsStore';
import { useAuthStore } from '@/store/authStore';
import FilterModal, {
  type FilterParams, buildFilterQueryString, formatDateRangeLabel, getBaseUrl,
} from '@/components/filterModal';
import { useAppTheme } from '@/themes/colors';

// ── Spotlight configs ─────────────────────────────────────────────────────────
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

// ── Cart button ───────────────────────────────────────────────────────────────
function CartHeaderButton() {
  const router = useRouter();
  const { colors } = useAppTheme();
  const [count, setCount] = useState(0);

  const refreshCount = useCallback(async () => {
    try {
      const sid  = await getCachedSessionId();
      const data = await getCart(sid);
      setCount(data?.total_items ?? 0);
    } catch {}
  }, []);

  useEffect(() => {
    refreshCount();
    const onVisible = () => { if (document.visibilityState === 'visible') refreshCount(); };
    document.addEventListener('visibilitychange', onVisible);
    return () => document.removeEventListener('visibilitychange', onVisible);
  }, [refreshCount]);

  return (
    <button onClick={() => router.push('/cart')} className="relative p-1">
      <ShoppingCart size={26} color={colors.text} />
      {count > 0 && (
        <span className="absolute top-0 right-0 min-w-[17px] h-[17px] rounded-full bg-[#ff2d55] text-white text-[9px] font-black flex items-center justify-center border-[1.5px] border-white px-0.5">
          {count > 9 ? '9+' : count}
        </span>
      )}
    </button>
  );
}

// ── Skeleton ──────────────────────────────────────────────────────────────────
function SkeletonCard() {
  return (
    <div className="w-[200px] shrink-0 animate-pulse">
      <div className="w-full h-[130px] rounded-[14px] bg-white/10 mb-2" />
      <div className="h-2 w-20 rounded bg-white/10 mb-1.5" />
      <div className="h-3 w-36 rounded bg-white/10 mb-1.5" />
      <div className="h-2 w-24 rounded bg-white/10" />
    </div>
  );
}
function SkeletonSection() {
  return (
    <div className="mt-7 px-5">
      <div className="h-3 w-36 rounded bg-white/10 mb-4 animate-pulse" />
      <div className="flex gap-3 overflow-hidden">
        {[0, 1, 2].map((i) => <SkeletonCard key={i} />)}
      </div>
    </div>
  );
}
function SkeletonFeed() {
  return (
    <>
      <div className="w-full h-[380px] bg-white/6 animate-pulse" />
      <SkeletonSection /><SkeletonSection /><SkeletonSection />
    </>
  );
}

// ── Event Card ────────────────────────────────────────────────────────────────
const EventCard = memo(({ item, onPress, colors }: { item: Event; onPress: () => void; colors: any }) => (
  <button onClick={onPress} className="w-[200px] shrink-0 text-left">
    <div className="relative w-full h-[130px] rounded-[14px] overflow-hidden" style={{ backgroundColor: colors.sectionCardBg }}>
      {item.cover_image_url && (
        <img src={item.cover_image_url} alt={item.title} className="absolute inset-0 w-full h-full object-cover" />
      )}
      {(item.category?.toLowerCase().includes('music') || item.category?.toLowerCase().includes('concert')) && (
       <div className="absolute top-2.5 left-2.5 bg-black/70 rounded-[5px] px-2.5 py-1 flex items-center gap-1">
          <div className="w-1.5 h-1.5 rounded-full bg-white" />
          <span className="text-white text-[9px] font-extrabold tracking-[1.2px]">LIVE</span>
        </div>
      )}
      <div className="absolute top-2.5 right-2.5 bg-black/60 rounded-[5px] px-2.5 py-1">
        <span className="text-white text-[9px] font-bold">{formatPrice(item.price)}</span>
      </div>
    </div>
    <div className="pt-2.5">
      <p className="text-[10px] font-semibold tracking-[0.5px] mb-0.5 truncate" style={{ color: colors.subText }}>{fmtCardDate(item.start_time)}</p>
      <p className="text-sm font-bold mb-1 leading-[18px] line-clamp-2" style={{ color: colors.text }}>{item.title}</p>
      <p className="text-[11px] truncate" style={{ color: colors.subText }}>{item.venue_name}</p>
    </div>
  </button>
));
EventCard.displayName = 'EventCard';

const Top10Card = memo(({ item, rank, onPress, colors }: { item: Event; rank: number; onPress: () => void; colors: any }) => (
  <button onClick={onPress} className="w-[200px] shrink-0 text-left">
    <div className="relative w-full h-[130px] rounded-[14px] overflow-hidden">
      {item.cover_image_url && (
        <img src={item.cover_image_url} alt={item.title} className="absolute inset-0 w-full h-full object-cover" />
      )}
      <div className="absolute inset-0" style={{ background: 'linear-gradient(to right, rgba(0,0,0,0.72), rgba(0,0,0,0.18), transparent)' }} />
      <span className="absolute top-1.5 left-2.5 text-[52px] font-black text-white leading-none" style={{ textShadow: '0 2px 6px rgba(0,0,0,0.4)', letterSpacing: -2 }}>{rank}</span>
      <div className="absolute top-2.5 right-2.5 bg-black/60 rounded-[5px] px-2.5 py-1">
        <span className="text-white text-[9px] font-bold">{formatPrice(item.price)}</span>
      </div>
    </div>
    <div className="pt-2.5">
      <p className="text-[10px] font-semibold tracking-[0.5px] mb-0.5 truncate" style={{ color: colors.subText }}>{fmtCardDate(item.start_time)}</p>
      <p className="text-sm font-bold mb-1 leading-[18px] line-clamp-2" style={{ color: colors.text }}>{item.title}</p>
      <p className="text-[11px] truncate" style={{ color: colors.subText }}>{item.venue_name}</p>
    </div>
  </button>
));
Top10Card.displayName = 'Top10Card';

// ── Section Row ───────────────────────────────────────────────────────────────
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
      <div className="px-5 mb-4 flex items-center justify-between">
        <h2 className="text-[22px] font-black tracking-[0.5px]" style={{ color: colors.text }}>{section.title.toUpperCase()}</h2>
        <button onClick={() => onViewAll(section.id, section.title, events)} className="text-[10px] font-extrabold text-[#FF2D55] tracking-[1.5px]">VIEW ALL</button>
      </div>
      <div className="flex overflow-x-auto gap-3 px-5 pb-1 scrollbar-hide">
        {events.map((item, index) =>
          isTop10
            ? <Top10Card key={item.id} item={item} rank={index + 1} onPress={() => onEventPress(item)} colors={colors} />
            : <EventCard key={item.id} item={item} onPress={() => onEventPress(item)} colors={colors} />
        )}
        {nextCursor && (
          <button onClick={handleLoadMore} disabled={loadingMore} className="shrink-0 w-14 flex items-center justify-center text-xs font-bold" style={{ color: colors.subText }}>
            {loadingMore ? '...' : 'More'}
          </button>
        )}
      </div>
    </div>
  );
});
SectionRow.displayName = 'SectionRow';

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
  const { colors, isDark }  = useAppTheme();

  const [spotlightConfig, setSpotlightConfig] = useState(
    () => SPOTLIGHT_CONFIGS[Math.floor(Math.random() * SPOTLIGHT_CONFIGS.length)]
  );
  const [activeCategory, setActiveCategory] = useState('All');
  const [dropdownOpen,   setDropdownOpen]   = useState(false);
  const [filterOpen,     setFilterOpen]     = useState(false);
  const [filterParams,   setFilterParams]   = useState<FilterParams>({ date: 'any' });
  const [sections,       setSections]       = useState<HomeSection[]>([]);
  const [homeSections,   setHomeSections]   = useState<HomeSection[]>([]);
  const [loading,        setLoading]        = useState(true);
  const [error,          setError]          = useState<string | null>(null);
  const [featuredIndex,  setFeaturedIndex]  = useState(0);
  const featuredIndexRef = useRef(0);

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
  const spotlightEvents    = useMemo(() => { const p = allEvents.filter((e) => e.category?.toLowerCase().includes(spotlightConfig.category)).slice(0, 4); if (p.length) return p; for (const cfg of SPOTLIGHT_CONFIGS) { if (cfg.category === spotlightConfig.category) continue; const f = allEvents.filter((e) => e.category?.toLowerCase().includes(cfg.category)).slice(0, 4); if (f.length) return f; } return allEvents.slice(0, 4); }, [allEvents, spotlightConfig]);
  const sportsBannerEvent  = spotlightEvents[0] ?? null;
  const sportsListEvents   = spotlightEvents.slice(1);

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

  // Auto-advance carousel
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

  // Spotlight fallback
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

  useEffect(() => { fetchSections().finally(() => setLoading(false)); }, [fetchSections]);

  const prevAuthUserId = useRef<string | null>(null);
  useEffect(() => {
    if (authUserId !== null && prevAuthUserId.current !== authUserId) {
      prevAuthUserId.current = authUserId;
      loadRecentlyViewed(authUserId);
      fetchSections(filterParams.city);
    }
    if (authUserId === null) { prevAuthUserId.current = null; clearRecentlyViewed(); }
  }, [authUserId, fetchSections, loadRecentlyViewed, clearRecentlyViewed]);

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
    router.push(`/event/${event.id}`);
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

  return (
    <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>

      {/* Location + filter bar */}
      <div className="sticky top-16 z-40 flex items-center justify-between px-5 py-4" style={{ backgroundColor: colors.headerBg, borderBottom: `1px solid ${colors.dividerColor}` }}>
        <button onClick={() => setFilterOpen(true)} className="text-left">
          <p className="text-[22px] font-extrabold tracking-[0.3px]" style={{ color: colors.text }}>{locationLabel}</p>
          <p className="text-[13px] font-extrabold" style={{ color: colors.subText }}>{dateLabelText}</p>
        </button>
        <CartHeaderButton />
      </div>

      {/* Hero carousel */}
      <div className="relative mt-4 mx-4 h-[380px] rounded-[14px] overflow-hidden">
        {carouselEvents.length === 0 ? (
          <div className="absolute inset-0 bg-gray-200" />
        ) : (
          <>
            {carouselEvents.map((evt, idx) => (
              <div
                key={evt.id}
                className="absolute inset-0 w-full h-full text-left transition-opacity duration-500"
                style={{ opacity: idx === featuredIndex ? 1 : 0, pointerEvents: idx === featuredIndex ? 'auto' : 'none' }}
              >
                {(evt.banner_image_url ?? evt.cover_image_url) && (
                  <img src={evt.banner_image_url ?? evt.cover_image_url!} alt={evt.title} className="absolute inset-0 w-full h-full object-cover" />
                )}
                {/* Clickable overlay behind the content — not a button, so no nesting issue */}
                <div
                  className="absolute inset-0 cursor-pointer"
                  onClick={() => handleEventPress(evt)}
                  style={{ background: 'linear-gradient(to top, #0a0a0a 0%, rgba(10,10,10,0.85) 30%, transparent 70%)' }}
                />
                <div className="absolute bottom-0 left-0 right-0 p-5 pb-8 pointer-events-none">
                  <p className="text-[11px] font-bold text-white/70 tracking-[2px] uppercase mb-2.5">✦ {evt.category ?? 'Featured'}</p>
                  <h1 className="text-[42px] font-black text-white tracking-[2px] leading-[42px] mb-3 line-clamp-2">{evt.title.toUpperCase()}</h1>
                  <p className="text-[13px] text-white/80 leading-5 mb-5 max-w-[300px] line-clamp-3">{evt.description}</p>
                  <div className="flex gap-3 pointer-events-auto">
                    <button onClick={() => handleEventPress(evt)} className="bg-[#FF2D55] px-[22px] py-3 rounded-sm text-white font-extrabold text-[11px] tracking-[1.5px]">🎟 GET TICKETS</button>
                    <button onClick={() => handleEventPress(evt)} className="bg-[#1c2e61] px-[22px] py-3 rounded-sm text-white font-bold text-[11px] tracking-[1px]">VIEW DETAILS</button>
                  </div>
                </div>
              </div>
            ))}
            {carouselEvents.length > 1 && (
              <div className="absolute bottom-3.5 right-4 flex gap-1.5 z-10">
                {carouselEvents.map((_, idx) => (
                  <button key={idx} onClick={() => { featuredIndexRef.current = idx; setFeaturedIndex(idx); }}>
                    <div className="h-1.5 rounded-full transition-all duration-300" style={{ width: idx === featuredIndex ? 18 : 6, backgroundColor: idx === featuredIndex ? '#FF2D55' : 'rgba(255,255,255,0.4)' }} />
                  </button>
                ))}
              </div>
            )}
          </>
        )}
      </div>

      {/* Category filter */}
      <div className="flex items-center px-5 py-3.5 gap-2.5">
        {activeCategory !== 'All' && (
          <button onClick={() => setActiveCategory('All')} className="w-[34px] h-[34px] rounded-[2px] border flex items-center justify-center text-xs" style={{ borderColor: colors.dividerColor, color: colors.text }}>✕</button>
        )}
        <button
          onClick={() => setDropdownOpen(true)}
          className="flex items-center px-4 py-2 rounded-[14px] border"
          style={{ borderColor: colors.text }}
        >
          <span className="text-[10px] font-extrabold tracking-[1.2px]" style={{ color: colors.text }}>{activeCategory === 'All' ? 'CATEGORIES' : activeCategory.toUpperCase()}</span>
          <span className="text-[11px] ml-1" style={{ color: colors.text }}> ▾</span>
        </button>
      </div>

      {/* Category dropdown */}
      {dropdownOpen && (
        <div className="fixed inset-0 z-50 flex flex-col" onClick={() => setDropdownOpen(false)}>
          <div className="flex-1 w-full pt-16 pb-10 flex flex-col items-center backdrop-blur-2xl bg-black/60" onClick={(e) => e.stopPropagation()}>
            <div className="flex-1 w-full overflow-y-auto">
              {categories.map((item) => (
                <button key={item} onClick={() => { setDropdownOpen(false); if (item === 'All') { setActiveCategory('All'); } else { router.push(`/category/${encodeURIComponent(item)}`); } }} className="py-[18px] px-10 w-full text-left">
                  <span className={`text-[26px] ${activeCategory === item ? 'font-black tracking-[1px]' : 'font-normal'}`} style={{ color: activeCategory === item ? colors.text : colors.subText }}>{item}</span>
                </button>
              ))}
            </div>
            <button onClick={() => setDropdownOpen(false)} className="mt-6 w-12 h-12 rounded-full flex items-center justify-center font-bold text-base" style={{ backgroundColor: colors.sectionCardBg, color: colors.text }}>✕</button>
          </div>
        </div>
      )}

      {/* Error banner */}
      {error && (
        <div className="mx-5 mt-5 p-3.5 rounded-sm border" style={{ backgroundColor: 'rgba(255,60,110,0.12)', borderColor: colors.dividerColor }}>
          <p className="font-black text-[12px] tracking-[1px] mb-1" style={{ color: colors.text }}>⚠ COULD NOT LOAD SECTIONS</p>
          <p className="text-[11px]" style={{ color: colors.subText }}>{error}</p>
          <button onClick={() => fetchSections()} className="mt-2.5 border px-3.5 py-1.5 rounded-sm text-[10px] font-black tracking-[1.5px]" style={{ borderColor: colors.text, color: colors.text }}>RETRY</button>
        </div>
      )}

      {/* Recently viewed */}
      {authUserId && recentlyViewedEvents.length > 0 && (
        <div className="mt-7">
          <div className="px-5 mb-4"><h2 className="text-[22px] font-black tracking-[0.5px]" style={{ color: colors.text }}>RECENTLY VIEWED</h2></div>
          <div className="flex overflow-x-auto gap-3 px-5 pb-1 scrollbar-hide">
            {recentlyViewedEvents.map((item) => <EventCard key={item.id} item={item} onPress={() => handleEventPress(item)} colors={colors} />)}
          </div>
        </div>
      )}

      {/* Feed sections */}
      {loading ? <SkeletonFeed /> : feedSections.map((section) => (
        <SectionRow key={section.id} section={section} isTop10={section.id === 'trending'} onEventPress={handleEventPress} onViewAll={handleViewAll} colors={colors} />
      ))}

      {/* Spotlight section */}
      {!loading && !isFiltered && sportsBannerEvent && (
        <div className="mt-7">
          <div className="px-5 mb-4 flex items-center justify-between">
            <h2 className="text-[22px] font-black tracking-[0.5px]" style={{ color: spotlightConfig.accentColor }}>{spotlightConfig.title}</h2>
            <button className="text-[10px] font-extrabold tracking-[1.5px]" style={{ color: spotlightConfig.accentColor }}>EXPLORE</button>
          </div>
          <button onClick={() => handleEventPress(sportsBannerEvent)} className="relative mx-5 rounded-sm overflow-hidden min-h-[200px] w-[calc(100%-40px)] text-left" style={{ backgroundColor: spotlightConfig.bgColor }}>
            {(sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url) && (
              <img src={sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url!} alt={sportsBannerEvent.title} className="absolute inset-0 w-full h-full object-cover" />
            )}
            <div className="absolute inset-0" style={{ background: `linear-gradient(to right, ${spotlightConfig.gradientDark}, ${spotlightConfig.gradientLight})` }} />
            <div className="absolute bottom-0 left-0 right-0 h-[3px] opacity-85" style={{ backgroundColor: spotlightConfig.accentColor }} />
            <div className="relative z-10 p-6">
              <p className="text-[10px] font-extrabold tracking-[2px] uppercase mb-1.5" style={{ color: spotlightConfig.accentColor }}>{spotlightConfig.badge}</p>
              <p className="text-[28px] font-black text-white tracking-[1px] mb-2">{sportsBannerEvent.title.toUpperCase()}</p>
              <p className="text-xs text-white/50 mb-4 line-clamp-2">{sportsBannerEvent.description}</p>
              <div className="border border-white/50 px-[22px] py-3 rounded-sm inline-block">
                <span className="text-white font-bold text-[11px] tracking-[1px]">VIEW TICKETS</span>
              </div>
            </div>
          </button>
          {sportsListEvents.length > 0 && (
            <div className="mt-4 px-5">
              {sportsListEvents.map((sport) => (
                <button key={sport.id} onClick={() => handleEventPress(sport)} className="flex items-center gap-3.5 py-3.5 w-full border-b last:border-0" style={{ borderColor: colors.dividerColor }}>
                  <div className="relative w-32 h-16 rounded-[8px] shrink-0 overflow-hidden bg-gray-200">
                    {sport.cover_image_url && <img src={sport.cover_image_url} alt={sport.title} className="absolute inset-0 w-full h-full object-cover" />}
                  </div>
                  <div className="flex-1 text-left">
                    <p className="text-[10px] font-semibold tracking-[0.5px] mb-0.5" style={{ color: colors.subText }}>{sport.category}</p>
                    <p className="text-sm font-semibold mb-0.5" style={{ color: colors.text }}>{sport.title}</p>
                    <p className="text-[11px]" style={{ color: colors.subText }}>{sport.city}</p>
                  </div>
                  <span className="text-[20px]" style={{ color: spotlightConfig.accentColor }}>›</span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Editorial sections */}
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