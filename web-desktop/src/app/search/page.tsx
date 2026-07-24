'use client';

import { useState, useCallback, useRef, useEffect, memo } from 'react';
import { useRouter } from 'next/navigation';
import { Search, X, Star, Megaphone, Sparkles, ChevronRight, ChevronLeft } from 'lucide-react';
import { searchEvents, getSuggestionEvents, type Event } from '@/lib/api';
import { useEventStore } from '@/store/eventsStore';
import { useAppTheme } from '@/themes/colors';
import { CoverImage } from '@/components/coverImage';

const ACCENT = '#ff3c6e';
const CARD_W = 'clamp(148px, 22vw, 230px)';

type SuggestionKey = 'featured' | 'popular' | 'just_announced';

const RAILS: { key: SuggestionKey; label: string; Icon: any; iconColor: string; bgColor: string }[] = [
  { key: 'featured',       label: "Editor's Picks", Icon: Sparkles,  iconColor: '#eab308', bgColor: 'rgba(234,179,8,0.12)'  },
  { key: 'popular',        label: 'Popular Events',  Icon: Star,      iconColor: '#3b82f6', bgColor: 'rgba(59,130,246,0.12)' },
  { key: 'just_announced', label: 'Just Announced',  Icon: Megaphone, iconColor: '#a855f7', bgColor: 'rgba(168,85,247,0.12)' },
];

const fmt      = (iso: string) =>
  new Date(iso).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short', year: 'numeric' });
const fmtPrice = (p: number) =>
  p === 0 ? 'Free' : `R${Math.round(p).toLocaleString('en-ZA')}`;

// ── Horizontal scroll-arrow hook — same behaviour as the home feed rows ─────────
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

function ScrollArrow({ dir, onClick }: { dir: 'left' | 'right'; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="absolute top-1/2 -translate-y-1/2 z-20 w-9 h-9 rounded-full flex items-center justify-center shadow-lg transition-opacity duration-200 hover:opacity-90"
      style={{
        [dir === 'left' ? 'left' : 'right']: '4px',
        backgroundColor: 'rgba(0,0,0,0.75)',
        border: '1px solid rgba(255,255,255,0.15)',
      }}
      aria-label={dir === 'left' ? 'Scroll left' : 'Scroll right'}
    >
      {dir === 'left' ? <ChevronLeft size={18} color="#ffffff" /> : <ChevronRight size={18} color="#ffffff" />}
    </button>
  );
}

// ── Rail card — poster-style with hover lift, Netflix-esque ────────────────────
const RailCard = memo(({ item, onPress, colors }: { item: Event; onPress: () => void; colors: any }) => (
  <button onClick={onPress} className="shrink-0 text-left group" style={{ width: CARD_W }}>
    <div
      className="relative w-full aspect-[16/10] rounded-[14px] overflow-hidden transition-transform duration-200 ease-out group-hover:scale-[1.045] group-hover:shadow-2xl"
      style={{ backgroundColor: colors.sectionCardBg }}
    >
      {item.cover_image_url && (
        <CoverImage uri={item.cover_image_url} alt={item.title} className="absolute inset-0 w-full h-full object-cover" />
      )}
      {(item.category?.toLowerCase().includes('music') || item.category?.toLowerCase().includes('concert')) && (
        <div className="absolute top-2 left-2 bg-black/70 rounded-[6px] px-2 py-[3px] flex items-center gap-1">
          <div className="w-1.5 h-1.5 rounded-full bg-white" />
          <span className="text-white text-[8px] font-extrabold tracking-[1px]">LIVE</span>
        </div>
      )}
      <div className="absolute top-2 right-2 bg-black/60 rounded-[6px] px-2 py-[3px]">
        <span className="text-white text-[9.5px] font-bold">{fmtPrice(item.price)}</span>
      </div>
      <div
        className="absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity duration-200"
        style={{ background: 'linear-gradient(to top, rgba(0,0,0,0.55), transparent 55%)' }}
      />
    </div>
    <div className="pt-2.5">
      <p className="text-[10.5px] font-semibold mb-0.5 truncate" style={{ color: colors.subText }}>
        {item.start_time ? fmt(item.start_time) : ''}
      </p>
      <p className="text-[13px] font-bold leading-snug mb-0.5 line-clamp-2" style={{ color: colors.text }}>
        {item.title}
      </p>
      <p className="text-[11px] truncate" style={{ color: colors.subText }}>
        {item.venue_name}
      </p>
    </div>
  </button>
));
RailCard.displayName = 'RailCard';

function RailSkeletonCard() {
  return (
    <div className="shrink-0 animate-pulse" style={{ width: CARD_W }}>
      <div className="w-full aspect-[16/10] rounded-[14px] bg-white/10 mb-2.5" />
      <div className="h-2 w-14 rounded bg-white/10 mb-1.5" />
      <div className="h-3 w-full rounded bg-white/10 mb-1.5" />
      <div className="h-2 w-2/3 rounded bg-white/10" />
    </div>
  );
}

// ── Rail — Netflix-style horizontal row (icon badge + title + See all + cards) ─
function Rail({
  config, events, loading, onEventPress, onSeeAll, colors,
}: {
  config: typeof RAILS[number]; events: Event[]; loading: boolean;
  onEventPress: (e: Event) => void; onSeeAll: (key: SuggestionKey) => void; colors: any;
}) {
  const { scrollRef, canScrollLeft, canScrollRight, scrollLeft, scrollRight } = useScrollArrows();
  const { key, label, Icon, iconColor, bgColor } = config;

  if (!loading && events.length === 0) return null;

  return (
    <div className="mb-8">
      {/* Rail title + See all */}
      <div className="flex items-center justify-between px-5 md:px-8 mb-3">
        <div className="flex items-center gap-2.5">
          <div className="w-8 h-8 rounded-[9px] flex items-center justify-center shrink-0" style={{ backgroundColor: bgColor }}>
            <Icon size={16} color={iconColor} />
          </div>
          <h2 className="text-[15px] font-black tracking-[0.4px]" style={{ color: colors.text }}>{label}</h2>
        </div>
        {!loading && events.length > 0 && (
          <button
            onClick={() => onSeeAll(key)}
            className="flex items-center gap-0.5 text-[10.5px] font-extrabold tracking-[1.2px] shrink-0 transition-opacity hover:opacity-70"
            style={{ color: ACCENT }}
          >
            SEE ALL <ChevronRight size={13} />
          </button>
        )}
      </div>

      {/* Scroll track + arrows */}
      <div className="relative">
        {canScrollLeft  && <ScrollArrow dir="left"  onClick={scrollLeft} />}
        {canScrollRight && <ScrollArrow dir="right" onClick={scrollRight} />}
        <div ref={scrollRef} className="flex overflow-x-auto overflow-y-visible gap-4 md:gap-5 px-5 md:px-8 py-1 scrollbar-hide">
          {loading
            ? [0, 1, 2, 3, 4].map((i) => <RailSkeletonCard key={i} />)
            : events.map((item) => <RailCard key={item.id} item={item} onPress={() => onEventPress(item)} colors={colors} />)
          }
        </div>
      </div>
    </div>
  );
}

// ── Grid card — larger poster tile for the full "See all" explore grid ─────────
const GridCard = memo(({ item, onPress, colors }: { item: Event; onPress: () => void; colors: any }) => (
  <button onClick={onPress} className="w-full text-left group">
    <div
      className="relative w-full aspect-[16/10] rounded-[14px] overflow-hidden transition-transform duration-200 ease-out group-hover:scale-[1.035] group-hover:shadow-2xl"
      style={{ backgroundColor: colors.sectionCardBg }}
    >
      {item.cover_image_url && (
        <CoverImage uri={item.cover_image_url} alt={item.title} className="absolute inset-0 w-full h-full object-cover" />
      )}
      {(item.category?.toLowerCase().includes('music') || item.category?.toLowerCase().includes('concert')) && (
        <div className="absolute top-2 left-2 bg-black/70 rounded-[6px] px-2 py-[3px] flex items-center gap-1">
          <div className="w-1.5 h-1.5 rounded-full bg-white" />
          <span className="text-white text-[8px] font-extrabold tracking-[1px]">LIVE</span>
        </div>
      )}
      <div className="absolute top-2 right-2 bg-black/60 rounded-[6px] px-2 py-[3px]">
        <span className="text-white text-[9.5px] font-bold">{fmtPrice(item.price)}</span>
      </div>
      <div
        className="absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity duration-200"
        style={{ background: 'linear-gradient(to top, rgba(0,0,0,0.6), transparent 55%)' }}
      />
    </div>
    <div className="pt-2.5">
      <p className="text-[9px] font-black tracking-[1.2px] mb-0.5 uppercase" style={{ color: ACCENT }}>
        {item.category}
      </p>
      <p className="text-[13.5px] font-bold leading-snug mb-0.5 line-clamp-2" style={{ color: colors.text }}>
        {item.title}
      </p>
      <p className="text-[11px] truncate mb-0.5" style={{ color: colors.subText }}>
        {item.venue_name}{item.city ? `, ${item.city}` : ''}
      </p>
      {item.start_time && (
        <p className="text-[10.5px]" style={{ color: colors.subText }}>
          {fmt(item.start_time)}
        </p>
      )}
    </div>
  </button>
));
GridCard.displayName = 'GridCard';

function GridSkeletonCard() {
  return (
    <div className="w-full animate-pulse">
      <div className="w-full aspect-[16/10] rounded-[14px] bg-white/10 mb-2.5" />
      <div className="h-2 w-14 rounded bg-white/10 mb-1.5" />
      <div className="h-3 w-full rounded bg-white/10 mb-1.5" />
      <div className="h-2 w-2/3 rounded bg-white/10" />
    </div>
  );
}

export default function SearchPage() {
  const router = useRouter();
  const { colors } = useAppTheme();
  const setSelectedEvent = useEventStore((s) => s.setSelectedEvent);

  const [query,   setQuery]   = useState('');
  const [results, setResults] = useState<Event[]>([]);
  const [loading, setLoading] = useState(false);
  const [mode,    setMode]    = useState<'idle' | 'results'>('idle');
  const [activeSuggestion, setActiveSuggestion] = useState<SuggestionKey | null>(null);

  // Netflix-style browse rails — fetched once on mount, rendered directly on the idle screen.
  const [railData,    setRailData]    = useState<Record<SuggestionKey, Event[]>>({ featured: [], popular: [], just_announced: [] });
  const [railLoading, setRailLoading] = useState(true);

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    let active = true;
    (async () => {
      const [featured, popular, justAnnounced] = await Promise.all([
        getSuggestionEvents('featured'),
        getSuggestionEvents('popular'),
        getSuggestionEvents('just_announced'),
      ]);
      if (!active) return;
      setRailData({ featured, popular, just_announced: justAnnounced });
      setRailLoading(false);
    })();
    return () => { active = false; };
  }, []);

  const MIN_QUERY_LENGTH = 2;

  const runSearch = useCallback(async (q: string) => {
    if (q.trim().length < MIN_QUERY_LENGTH) { setMode('idle'); setResults([]); return; }
    setLoading(true);
    setMode('results');
    const data = await searchEvents(q);
    setResults(data);
    setLoading(false);
  }, []);

  const handleChange = (val: string) => {
    setQuery(val);
    setActiveSuggestion(null);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => runSearch(val), 350);
  };

  // "See all" reuses the rail data already in memory — instant, no extra round-trip.
  const handleSeeAll = useCallback((key: SuggestionKey) => {
    setActiveSuggestion(key);
    setQuery('');
    setMode('results');
    setResults(railData[key] ?? []);
  }, [railData]);

  const handleClear = () => {
    setQuery('');
    setMode('idle');
    setResults([]);
    setActiveSuggestion(null);
  };

  const handleEventPress = (event: Event) => {
    setSelectedEvent(event);
    router.push(`/events/${event.id}`);
  };

  const allRailsEmpty = !railLoading && RAILS.every((r) => railData[r.key].length === 0);

  return (
    <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>

      {/* ── Search bar ─────────────────────────────────────────────────────── */}
      <div
        className="sticky top-0 md:top-16 z-40 px-4 py-3"
        style={{ backgroundColor: colors.headerBg, borderBottom: `1px solid ${colors.dividerColor}` }}
      >
        <div
          className="flex items-center gap-3 px-4 py-2.5 rounded-[14px]"
          style={{ backgroundColor: colors.sectionCardBg }}
        >
          <Search size={18} color={ACCENT} />
          <input
            value={query}
            onChange={(e) => handleChange(e.target.value)}
            placeholder="Search events, artists, venues..."
            className="flex-1 bg-transparent outline-none text-sm font-medium placeholder:opacity-40"
            style={{ color: colors.text }}
            autoFocus
          />
          {(query.length > 0 || mode === 'results') && (
            <button onClick={handleClear} className="p-0.5">
              <X size={16} color={colors.subText} />
            </button>
          )}
        </div>

        {/* Active suggestion label */}
        {activeSuggestion && mode === 'results' && !query && (
          <p className="mt-2 text-[11px] font-black tracking-[1.5px] px-1" style={{ color: colors.subText }}>
            {RAILS.find((r) => r.key === activeSuggestion)?.label.toUpperCase()}
          </p>
        )}
      </div>

      {/* ── Idle state — Netflix-style browse rails ─────────────────────────── */}
      {mode === 'idle' && (
        <div className="pt-6 pb-10">
          {RAILS.map((rail) => (
            <Rail
              key={rail.key}
              config={rail}
              events={railData[rail.key]}
              loading={railLoading}
              onEventPress={handleEventPress}
              onSeeAll={handleSeeAll}
              colors={colors}
            />
          ))}

          {/* True empty-state fallback — only if every rail comes back empty */}
          {allRailsEmpty && (
            <div className="flex flex-col items-center justify-center pt-16 pb-10 px-8 text-center">
              <Search size={52} color={colors.iconColor} strokeWidth={1.2} />
              <p className="mt-5 text-[22px] font-black tracking-wide" style={{ color: colors.text }}>
                FIND YOUR NEXT EVENT
              </p>
              <p className="mt-2 text-sm leading-relaxed" style={{ color: colors.subText }}>
                Search by name, artist, venue, or city
              </p>
            </div>
          )}
        </div>
      )}

      {/* ── Loading skeletons — same poster-grid layout as the real results ──── */}
      {loading && (
        <div className="px-5 md:px-8 pt-4 pb-10">
          <div className="h-2.5 w-24 rounded bg-white/10 mb-4 animate-pulse" />
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-5 md:gap-6">
            {[0, 1, 2, 3, 4, 5, 6, 7, 8, 9].map((i) => <GridSkeletonCard key={i} />)}
          </div>
        </div>
      )}

      {/* ── Results — Netflix explore-style poster grid ─────────────────────── */}
      {mode === 'results' && !loading && (
        <div className="px-5 md:px-8 pt-4 pb-10">
          {results.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-24 text-center">
              <Search size={48} color={colors.iconColor} strokeWidth={1.2} />
              <p className="mt-4 text-lg font-black tracking-wide" style={{ color: colors.text }}>
                NO RESULTS
              </p>
              <p className="mt-1 text-sm" style={{ color: colors.subText }}>
                Try a different search term or browse suggestions above
              </p>
            </div>
          ) : (
            <>
              <p className="pb-4 text-[11px] font-black tracking-[1.5px]" style={{ color: colors.subText }}>
                {results.length} RESULT{results.length !== 1 ? 'S' : ''}
              </p>
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-5 md:gap-6">
                {results.map((item) => (
                  <GridCard key={item.id} item={item} onPress={() => handleEventPress(item)} colors={colors} />
                ))}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}