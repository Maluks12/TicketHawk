'use client';

import { useState, useEffect, useCallback, useRef, useMemo, memo } from 'react';
import { useRouter } from 'next/navigation';
import { ChevronLeft, ChevronRight } from 'lucide-react';
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

function isEventLive(e: { start_time?: string; end_time?: string | null }): boolean {
  if (!e?.start_time) return false;
  const now = Date.now();
  const start = new Date(e.start_time).getTime();
  if (Number.isNaN(start) || now < start) return false;
  const end = e.end_time ? new Date(e.end_time).getTime() : start + 4 * 60 * 60 * 1000;
  return !Number.isNaN(end) && now <= end;
}

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

// ── Skeleton ──────────────────────────────────────────────────────────────────
// Card width formula: ~15% smaller than the original baseline overall
// (3.15 → 3.5 → 3.68, the last step being a further 5% reduction).
// Breakdown of the 56px offset: 20px left padding (px-5) + 2 gaps × 16px + 4px buffer = 56px.
const CARD_W = 'calc((100vw - 56px) / 3.68)';

function SkeletonCard() {
  return (
    <div className="shrink-0 animate-pulse" style={{ width: CARD_W }}>
      <div className="w-full aspect-[16/10] rounded-[16px] bg-white/10 mb-3" />
      <div className="h-2.5 w-20 rounded bg-white/10 mb-2" />
      <div className="h-3.5 w-full rounded bg-white/10 mb-1.5" />
      <div className="h-2.5 w-3/4 rounded bg-white/10" />
    </div>
  );
}
function SkeletonSection() {
  return (
    <div className="mt-8 px-5">
      <div className="h-3 w-36 rounded bg-white/10 mb-1.5 animate-pulse" />
      <div className="flex gap-4 overflow-hidden pb-4">
        {[0, 1, 2].map((i) => <SkeletonCard key={i} />)}
      </div>
    </div>
  );
}
function SkeletonFeed() {
  return (
    <>
      <div className="w-full h-[420px] bg-white/6 animate-pulse" />
      <SkeletonSection /><SkeletonSection /><SkeletonSection />
    </>
  );
}

// ── CHANGE B: Arrow scroll hook — shared by SectionRow + EditorialSection ────
function useScrollArrows() {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [canScrollLeft,  setCanScrollLeft]  = useState(false);
  const [canScrollRight, setCanScrollRight] = useState(false);

  const checkScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    setCanScrollLeft(el.scrollLeft > 4);
    setCanScrollRight(el.scrollLeft < el.scrollWidth - el.clientWidth - 4);
  }, []);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    checkScroll();
    el.addEventListener('scroll', checkScroll, { passive: true });
    const ro = new ResizeObserver(checkScroll);
    ro.observe(el);
    return () => { el.removeEventListener('scroll', checkScroll); ro.disconnect(); };
  }, [checkScroll]);

  const scrollLeft  = () => {
    const el = scrollRef.current;
    if (el) el.scrollBy({ left: -(el.clientWidth * 0.92), behavior: 'smooth' });
  };
  const scrollRight = () => {
    const el = scrollRef.current;
    if (el) el.scrollBy({ left: el.clientWidth * 0.92, behavior: 'smooth' });
  };

  return { scrollRef, canScrollLeft, canScrollRight, scrollLeft, scrollRight };
}

// ── CHANGE C: Arrow button component ─────────────────────────────────────────
function ScrollArrow({ dir, onClick, colors }: { dir: 'left' | 'right'; onClick: () => void; colors: any }) {
  return (
    <button
      onClick={onClick}
      className="absolute top-1/2 -translate-y-1/2 z-20 w-10 h-10 rounded-full flex items-center justify-center shadow-lg transition-opacity duration-200 hover:opacity-90"
      style={{
        [dir === 'left' ? 'left' : 'right']: '8px',
        backgroundColor: 'rgba(0,0,0,0.75)',
        border: `1px solid rgba(255,255,255,0.15)`,
      }}
      aria-label={dir === 'left' ? 'Scroll left' : 'Scroll right'}
    >
      {dir === 'left'
        ? <ChevronLeft size={20} color="#ffffff" />
        : <ChevronRight size={20} color="#ffffff" />
      }
    </button>
  );
}

// ── CHANGE D: EventCard — larger dimensions (200→260 wide, 130→170 tall) ─────
// Cards ignore onPress in their memo comparison: the home page's carousel
// intervals re-render the page every few seconds, recreating the inline
// onPress closures. onPress only closes over `item`, so comparing item
// identity (+ colors / rank) is safe and stops the every-tick card
// re-render that users saw as flicker.
const eventCardEqual = (a: any, b: any) =>
  a.item === b.item && a.colors === b.colors && a.rank === b.rank;

const EventCard = memo(({ item, onPress, colors }: { item: Event; onPress: () => void; colors: any }) => (
  <button onClick={onPress} className="shrink-0 text-left" style={{ width: CARD_W }}>
    <div className="relative w-full aspect-[16/10] rounded-[16px] overflow-hidden" style={{ backgroundColor: colors.sectionCardBg }}>
      {item.cover_image_url && (
        <CoverImage uri={item.cover_image_url} alt={item.title} className="absolute inset-0 w-full h-full object-cover" />
      )}
      {isEventLive(item) && (
       <div className="absolute top-3 left-3 bg-black/70 rounded-[6px] px-2.5 py-1 flex items-center gap-1">
          <div className="w-1.5 h-1.5 rounded-full bg-white" />
          <span className="text-white text-[9px] font-extrabold tracking-[1.2px]">LIVE</span>
        </div>
      )}
      <div className="absolute top-3 right-3 min-w-[34px] h-[22px] bg-black/60 rounded-[6px] px-2 flex items-center justify-center">
        <span className="text-white text-[10px] font-bold leading-none">{formatPrice(item.price)}</span>
      </div>
    </div>
    <div className="pt-3">
      <p className="text-[11px] font-semibold tracking-[0.3px] mb-1 truncate" style={{ color: colors.subText }}>{fmtCardDate(item.start_time)}</p>
      <p className="text-[16px] font-bold leading-[22px] mb-1 line-clamp-2" style={{ color: colors.text }}>{item.title}</p>
      <p className="text-[12.5px] truncate" style={{ color: colors.subText }}>{item.venue_name}</p>
    </div>
  </button>
), eventCardEqual);
EventCard.displayName = 'EventCard';
// See eventCardEqual note above the memo wrappers.

// ── CHANGE E: Top10Card — larger dimensions ───────────────────────────────────
const Top10Card = memo(({ item, rank, onPress, colors }: { item: Event; rank: number; onPress: () => void; colors: any }) => (
  <button onClick={onPress} className="shrink-0 text-left" style={{ width: CARD_W }}>
    <div className="relative w-full aspect-[16/10] rounded-[16px] overflow-hidden">
      {item.cover_image_url && (
        <CoverImage uri={item.cover_image_url} alt={item.title} className="absolute inset-0 w-full h-full object-cover" />
      )}
      <div className="absolute inset-0" style={{ background: 'linear-gradient(to right, rgba(0,0,0,0.72), rgba(0,0,0,0.18), transparent)' }} />
      <span className="absolute top-1.5 left-3 text-[60px] font-black text-white leading-none" style={{ textShadow: '0 2px 10px rgba(0,0,0,0.5)', letterSpacing: -2 }}>{rank}</span>
      <div className="absolute top-3 right-3 min-w-[34px] h-[22px] bg-black/60 rounded-[6px] px-2 flex items-center justify-center">
        <span className="text-white text-[10px] font-bold leading-none">{formatPrice(item.price)}</span>
      </div>
    </div>
    <div className="pt-3">
      <p className="text-[11px] font-semibold tracking-[0.3px] mb-1 truncate" style={{ color: colors.subText }}>{fmtCardDate(item.start_time)}</p>
      <p className="text-[16px] font-bold leading-[22px] mb-1 line-clamp-2" style={{ color: colors.text }}>{item.title}</p>
      <p className="text-[12.5px] truncate" style={{ color: colors.subText }}>{item.venue_name}</p>
    </div>
  </button>
), eventCardEqual);
Top10Card.displayName = 'Top10Card';

// HeroCarousel — owns its own auto-advance timer so the 5s tick re-renders
// ONLY this component. Previously the interval's setState lived in HomePage,
// re-rendering the entire page every tick; combined with any non-memoized
// child that meant visible card churn. Module scope + self-contained state
// makes tick-driven flicker structurally impossible.
const HeroCarousel = memo(function HeroCarousel({ events: carouselEvents, colors, onEventPress, onBuyTickets }: {
  events: Event[]; colors: any; onEventPress: (e: Event) => void; onBuyTickets: (e: Event) => void;
}) {
  const [featuredIndex, setFeaturedIndex] = useState(0);
  const featuredIndexRef = useRef(0);

  useEffect(() => {
    if (carouselEvents.length <= 1) return;
    const t = setInterval(() => {
      const next = (featuredIndexRef.current + 1) % carouselEvents.length;
      featuredIndexRef.current = next;
      setFeaturedIndex(next);
    }, 5000);
    return () => clearInterval(t);
  }, [carouselEvents.length]);

  return (
    <>
    {/* Hero carousel */}
    <div className="relative mb-2 md:mx-4 rounded-none md:rounded-[18px] overflow-hidden" style={{ height: "clamp(320px, 45vw, 560px)" }}>
      {carouselEvents.length === 0 ? (
        <div className="absolute inset-0 rounded-[18px] bg-white/10 animate-pulse" />
      ) : (
        <>
          {carouselEvents.map((evt, idx) => (
            <div
              key={evt.id}
              className="absolute inset-0 w-full h-full transition-opacity duration-500"
              style={{ opacity: idx === featuredIndex ? 1 : 0, pointerEvents: idx === featuredIndex ? 'auto' : 'none', zIndex: idx === featuredIndex ? 1 : 0 }}
            >
              {/* Background image */}
              {(evt.banner_image_url ?? evt.cover_image_url) && (
                <img
                  src={evt.banner_image_url ?? evt.cover_image_url!}
                  alt={evt.title}
                  className="absolute inset-0 w-full h-full object-cover"
                />
              )}
              {/* Gradient overlay — visual only, no click handler */}
              <div
                className="absolute inset-0"
                style={{ background: 'linear-gradient(to top, rgba(0,0,0,0.96) 0%, rgba(0,0,0,0.75) 35%, rgba(0,0,0,0.1) 65%, transparent 100%)' }}
              />
              {/* Content — category badge, title, description, then buttons+dots on same row */}
              <div className="absolute bottom-0 left-0 right-0 px-5 pb-5 pointer-events-none">
                {/* Category badge */}
                <span
                  className="inline-flex items-center gap-1.5 mb-3 px-2.5 py-1 rounded-[5px] text-[9px] font-extrabold tracking-[1.8px] uppercase"
                  style={{ backgroundColor: 'rgba(255,45,85,0.85)', color: '#fff' }}
                >
                  ✦ {evt.category ?? 'Featured'}
                </span>
                {/* Title — 28px keeps it tight even at 2 lines */}
                <h1
                  className="text-[28px] font-black text-white leading-[32px] mb-2 line-clamp-2"
                  style={{ letterSpacing: '0.5px', textShadow: '0 1px 4px rgba(0,0,0,0.5)' }}
                >
                  {evt.title.toUpperCase()}
                </h1>
                {/* Description — 1 line only to keep it clean */}
                <p className="text-[12px] text-white/65 leading-[17px] mb-4 line-clamp-1">
                  {evt.description}
                </p>
                {/* Bottom row: buttons pinned left, dots pinned right — in the same row so they never collide */}
                <div className="flex items-end justify-between pointer-events-auto">
                  <div className="flex items-center gap-3">
                    <button
                      onClick={(e) => { e.stopPropagation(); onBuyTickets(evt); }}
                      className="bg-[#FF2D55] text-white font-extrabold tracking-[1.5px] rounded-[5px]"
                      style={{ padding: '12px 26px', fontSize: 13 }}
                    >
                      🎟 GET TICKETS
                    </button>
                    <button
                      onClick={(e) => { e.stopPropagation(); onEventPress(evt); }}
                      className="bg-[#1c2e61] text-white font-bold tracking-[1px] rounded-[5px]"
                      style={{ padding: '12px 26px', fontSize: 13 }}
                    >
                      VIEW DETAILS
                    </button>
                  </div>
                  {/* Dots — bottom-right corner, same row as buttons */}
                  {carouselEvents.length > 1 && (
                    <div className="flex items-center gap-2 pb-[3px]">
                      {carouselEvents.map((_, i) => (
                        <button
                          key={i}
                          onClick={() => { featuredIndexRef.current = i; setFeaturedIndex(i); }}
                          className="h-[6px] rounded-full transition-all duration-300"
                          style={{
                            width: i === featuredIndex ? 20 : 6,
                            backgroundColor: i === featuredIndex ? '#FF2D55' : 'rgba(255,255,255,0.4)',
                          }}
                        />
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </div>
          ))}
        </>
      )}
    </div>
    </>
  );
}, (a, b) => a.events === b.events && a.colors === b.colors);


// EditorialSection — MUST live at module scope. It was previously defined
// inside Home, so every carousel tick (4.5–5s setState) created a new
// component type and React remounted every section — images re-requested,
// cards visibly flickered, and in-flight image loads were aborted.
const EditorialSection = memo(function EditorialSection({ title, events, filterKey, colors, onViewAll, onEventPress }: {
  title: string; events: Event[]; filterKey: string; colors: any;
  onViewAll: (filterKey: string, title: string) => void;
  onEventPress: (event: Event) => void;
}) {
  const { scrollRef, canScrollLeft, canScrollRight, scrollLeft, scrollRight } = useScrollArrows();
  if (!events.length) return null;
  return (
    <div className="mt-8">
      <div className="px-5 mb-1.5 flex items-center justify-between">
        <h2 className="text-[17px] font-black tracking-[0.8px] uppercase" style={{ color: colors.text }}>{title}</h2>
        <button onClick={() => onViewAll(filterKey, title)} className="text-[10px] font-extrabold text-[#FF2D55] tracking-[1.5px]">VIEW ALL</button>
      </div>
      <div className="relative">
        {canScrollLeft  && <ScrollArrow dir="left"  onClick={scrollLeft}  colors={colors} />}
        {canScrollRight && <ScrollArrow dir="right" onClick={scrollRight} colors={colors} />}
        <div ref={scrollRef} className="flex overflow-x-auto gap-7 px-5 pb-4 scrollbar-hide">
          {events.map((item) => <EventCard key={item.id} item={item} onPress={() => onEventPress(item)} colors={colors} />)}
        </div>
      </div>
    </div>
  );
}, (a, b) => a.events === b.events && a.colors === b.colors && a.title === b.title);


// ── CHANGE F: SectionRow — now has left/right arrows, no visible scrollbar ───
const SectionRow = memo(({
  section, isTop10 = false, onEventPress, onViewAll, colors,
}: {
  section: HomeSection; isTop10?: boolean;
  onEventPress: (e: Event) => void; onViewAll: (id: string, title: string, events: Event[]) => void; colors: any;
}) => {
  const [events,      setEvents]      = useState<Event[]>(section.events ?? []);
  const [nextCursor,  setNextCursor]  = useState(section.next_cursor ?? '');
  const [loadingMore, setLoadingMore] = useState(false);
  const { scrollRef, canScrollLeft, canScrollRight, scrollLeft, scrollRight } = useScrollArrows();

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
    <div className="mt-8">
      {/* Section title + VIEW ALL */}
      <div className="px-5 md:px-8 mb-1.5 flex items-center justify-between">
        <h2 className="text-[17px] font-black tracking-[0.8px] uppercase" style={{ color: colors.text }}>
          {section.title}
        </h2>
        <button
          onClick={() => onViewAll(section.id, section.title, events)}
          className="text-[10px] font-extrabold text-[#FF2D55] tracking-[1.5px]"
        >
          VIEW ALL
        </button>
      </div>
      {/* Scroll track + arrows */}
      <div className="relative">
        {canScrollLeft  && <ScrollArrow dir="left"  onClick={scrollLeft}  colors={colors} />}
        {canScrollRight && <ScrollArrow dir="right" onClick={scrollRight} colors={colors} />}
        <div ref={scrollRef} className="flex overflow-x-auto gap-5 md:gap-7 px-5 md:px-8 pb-4 scrollbar-hide">
          {events.map((item, index) =>
            isTop10
              ? <Top10Card key={item.id} item={item} rank={index + 1} onPress={() => onEventPress(item)} colors={colors} />
              : <EventCard  key={item.id} item={item}                  onPress={() => onEventPress(item)} colors={colors} />
          )}
          {nextCursor && (
            <button onClick={handleLoadMore} disabled={loadingMore} className="shrink-0 w-14 flex items-center justify-center text-xs font-bold" style={{ color: colors.subText }}>
              {loadingMore ? '...' : 'More'}
            </button>
          )}
        </div>
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

  const [activeCategory, setActiveCategory] = useState('All');
  const [dropdownOpen,   setDropdownOpen]   = useState(false);

  useEffect(() => {
    if (!dropdownOpen) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = prev; };
  }, [dropdownOpen]);
  const [filterParams,   setFilterParams]   = useState<FilterParams>({ date: 'any' });
  const [sections,       setSections]       = useState<HomeSection[]>([]);
  const [homeSections,   setHomeSections]   = useState<HomeSection[]>([]);
  const [loading,        setLoading]        = useState(true);
  const [error,          setError]          = useState<string | null>(null);

  // filterOpen lives in the shared store so the navbar can open it too
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

  useEffect(() => { if (allEvents.length > 0) setAllEventsStore(allEvents); }, [allEvents]);


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
    router.push(`/events/${event.id}`);
  }, [setSelectedEvent, router]);

  const handleBuyTickets = useCallback((event: Event) => {
    setSelectedEvent(event);
    sessionStorage.setItem(`event_detail_visited_${event.id}`, '1');
    router.push(`/events/${event.id}/purchase`);
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

  // Keep the navbar filter pill in sync
  useEffect(() => {
    setLocationLabel(locationLabel);
    setDateLabelText(dateLabelText);
  }, [locationLabel, dateLabelText]);


  return (
    <div style={{ backgroundColor: colors.background, minHeight: '100vh' }} className="pt-4">
      

      <HeroCarousel events={carouselEvents} colors={colors} onEventPress={handleEventPress} onBuyTickets={handleBuyTickets} />

      {/* ── Category bar: pill tabs on desktop, dropdown on mobile ── */}
      {/* Desktop pill tabs */}
      <div className="hidden md:flex items-center gap-2 px-8 pt-3 pb-4 overflow-x-auto scrollbar-hide">
        {categories.map((item) => {
          const isActive = activeCategory === item;
          return (
            <button
              key={item}
              onClick={() => {
                if (item === 'All') { setActiveCategory('All'); }
                else { setActiveCategory(item); router.push(`/category/${encodeURIComponent(item)}`); }
              }}
              className="shrink-0 px-5 h-[40px] rounded-full text-[13px] font-bold tracking-[0.3px] transition-all duration-150 whitespace-nowrap"
              style={{
                backgroundColor: isActive ? '#FF2D55' : (isDark ? 'rgba(255,255,255,0.07)' : 'rgba(0,0,0,0.06)'),
                color: isActive ? '#fff' : colors.text,
                border: isActive ? 'none' : `1px solid ${isDark ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.1)'}`,
              }}
            >
              {item}
            </button>
          );
        })}
      </div>

      {/* Mobile dropdown trigger */}
      <div className="flex md:hidden items-center px-5 pt-1 pb-3 gap-3 overflow-x-auto scrollbar-hide">
        {activeCategory !== 'All' && (
          <button
            onClick={() => setActiveCategory('All')}
            className="h-[42px] w-[42px] shrink-0 rounded-[10px] border flex items-center justify-center text-sm font-bold"
            style={{ borderColor: colors.dividerColor, color: colors.text }}
          >
            ✕
          </button>
        )}
        <button
          onClick={() => setDropdownOpen(true)}
          className="flex items-center px-5 h-[42px] rounded-[12px] border shrink-0"
          style={{ borderColor: colors.text }}
        >
          <span className="text-[11px] font-extrabold tracking-[1.2px]" style={{ color: colors.text }}>
            {activeCategory === 'All' ? 'CATEGORIES' : activeCategory.toUpperCase()}
          </span>
          <span className="text-[13px] ml-1.5" style={{ color: colors.text }}> ▾</span>
        </button>
      </div>

      {/* Mobile category dropdown overlay */}
      {dropdownOpen && (
        <div className="fixed inset-0 z-50 flex flex-col md:hidden" onClick={() => setDropdownOpen(false)}>
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
        <div className="mt-8">
          <div className="px-5 mb-1.5 flex items-center justify-between">
            <h2 className="text-[17px] font-black tracking-[0.8px] uppercase" style={{ color: colors.text }}>RECENTLY VIEWED</h2>
          </div>
          <div className="flex overflow-x-auto gap-7 px-5 pb-4 scrollbar-hide">
            {recentlyViewedEvents.map((item) => <EventCard key={item.id} item={item} onPress={() => handleEventPress(item)} colors={colors} />)}
          </div>
        </div>
      )}

      {/* Feed sections */}
      {loading ? <SkeletonFeed /> : feedSections.map((section) => (
        <SectionRow key={section.id} section={section} isTop10={section.id === 'trending'} onEventPress={handleEventPress} onViewAll={handleViewAll} colors={colors} />
      ))}

      {/* Editorial sections — each carries its own top margin (mt-8) */}
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