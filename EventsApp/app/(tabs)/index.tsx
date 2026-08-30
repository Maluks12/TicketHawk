import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  Modal,
  FlatList,
  ActivityIndicator,
  RefreshControl,
  Animated,
  StyleSheet,
  AppState,
  useWindowDimensions,
  type AppStateStatus,
} from 'react-native';
import { Image } from 'expo-image';
import { prefetchImages, prefetchFullImages } from '../../lib/img';
import ComingSoon from '../../components/ComingSoon';
import { CoverImage } from '../../components/CoverImage';
import { useState, useEffect, useCallback, useRef, useMemo, memo, forwardRef, useImperativeHandle } from 'react';
import { useRouter, useFocusEffect } from 'expo-router';
import { LinearGradient } from 'expo-linear-gradient';
import { Ionicons } from '@expo/vector-icons';
import {
  getHomeSections,
  loadMoreSectionEvents,
  getCategories,
  getCart,
  getCachedSessionId,
  type Event,
  type HomeSection,
} from '../../lib/api';
import { useEventStore } from '../../store/eventStore';
import { Ticket, CaretDown, MagnifyingGlass } from 'phosphor-react-native';
import { useAuthStore } from '../../store/authStore';
import { s, sheet } from '../../styles/home';
import FilterModal, { FilterParams, buildFilterQueryString, formatDateRangeLabel, getBaseUrl } from '../../components/FilterModal';
import { useAppTheme } from '../../theme/colors';
import { BlurView } from 'expo-blur';

// ─── Live window check ────────────────────────────────────────────────────────
// LIVE only while the event is actually running: start <= now <= end.
// Events without an end_time are assumed live for 4 hours after start.
function isEventLive(e: { start_time?: string; end_time?: string | null }): boolean {
  if (!e?.start_time) return false;
  const now   = Date.now();
  const start = new Date(e.start_time).getTime();
  if (Number.isNaN(start) || now < start) return false;
  const end = e.end_time ? new Date(e.end_time).getTime() : start + 4 * 60 * 60 * 1000;
  return !Number.isNaN(end) && now <= end;
}

// ─── Cart header button ───────────────────────────────────────────────────────
function CartHeaderButton() {
  const router             = useRouter();
  const { colors }         = useAppTheme();
  const [count, setCount]  = useState(0);
  const prevCount          = useRef(0);
  const scaleAnim          = useRef(new Animated.Value(1)).current;
  const isMounted          = useRef(true);

  const refreshCount = useCallback(async () => {
    try {
      const sid  = await getCachedSessionId();
      const data = await getCart(sid);
      const n    = data?.total_items ?? 0;
      if (!isMounted.current) return;
      if (n !== prevCount.current) {
        if (n > prevCount.current) {
          Animated.sequence([
            Animated.spring(scaleAnim, { toValue: 1.4, useNativeDriver: true, bounciness: 14 }),
            Animated.spring(scaleAnim, { toValue: 1,   useNativeDriver: true, bounciness: 6  }),
          ]).start();
        }
        prevCount.current = n;
        setCount(n);
      }
    } catch { /* guest with no cart yet */ }
  }, []);

  useEffect(() => {
    isMounted.current = true;
    refreshCount();
    const sub = AppState.addEventListener('change', (state: AppStateStatus) => {
      if (state === 'active') refreshCount();
    });
    return () => {
      isMounted.current = false;
      sub.remove();
    };
  }, [refreshCount]);

  return (
    <TouchableOpacity
      onPress={() => router.push('/cart' as any)}
      style={cartStyles.btn}
      hitSlop={{ top: 10, bottom: 10, left: 10, right: 10 }}
      activeOpacity={0.75}
    >
      <Ionicons name="cart-outline" size={28} color={colors.text} />
      {count > 0 && (
        <Animated.View style={[cartStyles.badge, { transform: [{ scale: scaleAnim }] }]}>
          <Text style={cartStyles.badgeText}>{count > 9 ? '9+' : count}</Text>
        </Animated.View>
      )}
    </TouchableOpacity>
  );
}

const cartStyles = StyleSheet.create({
  btn:       { position: 'relative', padding: 4 },
  badge:     { position: 'absolute', top: 0, right: 0, minWidth: 17, height: 17, borderRadius: 9, backgroundColor: '#ff2d55', alignItems: 'center', justifyContent: 'center', paddingHorizontal: 3, borderWidth: 1.5, borderColor: '#fff' },
  badgeText: { fontSize: 9, fontWeight: '900', color: '#fff', lineHeight: 11 },
});

// ─── Dynamic spotlight section configs ────────────────────────────────────────
const SPOTLIGHT_CONFIGS = [
  { title: 'SPORTS ELITE',    category: 'sports',   badge: 'SPORTS',    accentColor: '#e8c14a', bgColor: '#1a1200', gradientDark: 'rgba(26,18,0,0.82)',   gradientLight: 'rgba(26,18,0,0.25)'   },
  { title: 'BEST FESTIVALS',  category: 'music',    badge: 'FESTIVAL',  accentColor: '#ff3c6e', bgColor: '#1a0010', gradientDark: 'rgba(26,0,16,0.82)',   gradientLight: 'rgba(26,0,16,0.25)'   },
  { title: 'GALA NIGHTS',     category: 'gala',     badge: 'GALA',      accentColor: '#c084fc', bgColor: '#0e0018', gradientDark: 'rgba(14,0,24,0.82)',   gradientLight: 'rgba(14,0,24,0.25)'   },
  { title: 'TECH SUMMIT',     category: 'tech',     badge: 'TECH',      accentColor: '#38bdf8', bgColor: '#00101a', gradientDark: 'rgba(0,16,26,0.82)',   gradientLight: 'rgba(0,16,26,0.25)'   },
  { title: 'FOOD & CULTURE',  category: 'food',     badge: 'FOOD',      accentColor: '#fb923c', bgColor: '#1a0800', gradientDark: 'rgba(26,8,0,0.82)',    gradientLight: 'rgba(26,8,0,0.25)'    },
  { title: 'TOP CONCERTS',    category: 'concerts', badge: 'MUSIC',     accentColor: '#34d399', bgColor: '#001a10', gradientDark: 'rgba(0,26,16,0.82)',   gradientLight: 'rgba(0,26,16,0.25)'   },
];

// ─── Helpers ─────────────────────────────────────────────────────────────────

// Which events feed the hero carousel. Lifted out of the component so
// fetchSections() can warm exactly the artwork the hero is about to request --
// if these two ever computed a different pool, the prefetch would be warming
// the wrong images.
function pickCarouselEvents(sections: HomeSection[]): Event[] {
  const rawFeatured  = sections.find((sec) => sec.id === 'featured')?.events ?? [];
  const trendingPool = sections.find((sec) => sec.id === 'trending')?.events ?? [];
  return rawFeatured.length >= 2
    ? rawFeatured.slice(0, 6)
    : [
        ...rawFeatured,
        ...trendingPool.filter((e) => !rawFeatured.some((f) => f.id === e.id)),
      ].slice(0, 5);
}

function formatPrice(price: number, currency: string) {
  if (price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function groupEventsIntoSections(events: Event[]): HomeSection[] {
  const order: string[] = [];
  const groups: Record<string, Event[]> = {};
  for (const e of events) {
    const cat = (e.category?.trim() || 'Other')
      .replace(/\b\w/g, (c) => c.toUpperCase());
    if (!groups[cat]) { groups[cat] = []; order.push(cat); }
    if (groups[cat].length < 10) groups[cat].push(e);
  }
  const sections: HomeSection[] = order
    .filter((cat) => groups[cat].length >= 3)
    .map((cat) => ({
      id: `cat-${cat.toLowerCase().replace(/\s+/g, '-')}`,
      title: cat.toUpperCase(),
      events: groups[cat],
      next_cursor: '',
    }))
    .sort((a, b) => b.events.length - a.events.length);
  const free = events.filter((e) => e.price === 0).slice(0, 10);
  if (free.length >= 2) sections.push({ id: 'free', title: 'FREE EVENTS', events: free, next_cursor: '' });
  if (!sections.length && events.length)
    sections.push({ id: 'results', title: 'RESULTS', events: events.slice(0, 10), next_cursor: '' });
  return sections;
}

// ─── Skeleton loader ──────────────────────────────────────────────────────────
function SkeletonPulse({ style }: { style: any }) {
  const anim = useRef(new Animated.Value(0.35)).current;
  useEffect(() => {
    Animated.loop(
      Animated.sequence([
        Animated.timing(anim, { toValue: 1,    duration: 750, useNativeDriver: true }),
        Animated.timing(anim, { toValue: 0.35, duration: 750, useNativeDriver: true }),
      ])
    ).start();
  }, [anim]);
  return <Animated.View style={[style, { opacity: anim }]} />;
}

function SkeletonSection() {
  const shimmer = 'rgba(255,255,255,0.08)';
  return (
    <View style={{ marginTop: 20 }}>
      <View style={{ paddingHorizontal: 16, marginBottom: 12 }}>
        <SkeletonPulse style={{ width: 140, height: 11, borderRadius: 3, backgroundColor: shimmer }} />
      </View>
      <View style={{ flexDirection: 'row', paddingHorizontal: 16, gap: 12 }}>
        {[0, 1, 2].map((i) => (
          <View key={i} style={{ width: 160 }}>
            <SkeletonPulse style={{ width: 160, height: 110, borderRadius: 4, backgroundColor: shimmer, marginBottom: 8 }} />
            <SkeletonPulse style={{ width: 80,  height: 9,   borderRadius: 3, backgroundColor: shimmer, marginBottom: 6 }} />
            <SkeletonPulse style={{ width: 140, height: 11,  borderRadius: 3, backgroundColor: shimmer, marginBottom: 5 }} />
            <SkeletonPulse style={{ width: 100, height: 9,   borderRadius: 3, backgroundColor: shimmer }} />
          </View>
        ))}
      </View>
    </View>
  );
}

function SkeletonFeed() {
  return (
    <>
      <SkeletonPulse style={{ width: '100%', height: 420, backgroundColor: 'rgba(255,255,255,0.06)' }} />
      <SkeletonSection />
      <SkeletonSection />
      <SkeletonSection />
    </>
  );
}

const fmtCardDate = (iso?: string | null) => {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short', year: 'numeric' });
};

// ─── PERF: React.memo + expo-image (memory-disk cache, blur-up transition) ───
const EventCard = memo(({ item, onPress, colors, priority }: { item: Event; onPress: (e: Event) => void; colors: any; priority?: 'low' | 'normal' | 'high' }) => (
  <TouchableOpacity className={s.hCard} onPress={() => onPress(item)}>
    <View className={s.hCardImageArea} style={{ backgroundColor: colors.sectionCardBg }}>
      {item.cover_image_url && (
        <CoverImage
          uri={item.cover_image_url}
          style={sheet.absoluteFill}
          contentFit="cover"
          priority={priority ?? 'normal'}
          recyclingKey={item.id}
        />
      )}
      {isEventLive(item) && (
        <View className={s.hCardBadge}>
          <View style={{ width: 5, height: 5, borderRadius: 3, backgroundColor: '#fff' }} />
          <Text className={s.hCardBadgeText}>LIVE</Text>
        </View>
      )}
      <View className={s.hCardPriceBadge}>
        <Text className={s.hCardPriceBadgeText}>{formatPrice(item.price, item.currency)}</Text>
      </View>
    </View>
    <View className={s.hCardBody}>
      <Text className={s.hCardCat} style={{ color: colors.subText }} numberOfLines={1}>{fmtCardDate(item.start_time)}</Text>
      <Text className={s.hCardTitle} style={{ color: colors.text }} numberOfLines={2}>{item.title}</Text>
      <Text className={s.hCardMeta} style={{ color: colors.subText }} numberOfLines={1}>{item.venue_name}</Text>
    </View>
  </TouchableOpacity>
));

const Top10Card = memo(({ item, rank, onPress, colors }: { item: Event; rank: number; onPress: (e: Event) => void; colors: any }) => (
  <TouchableOpacity className={s.hCard} onPress={() => onPress(item)}>
    <View className={s.hCardImageArea}>
      {item.cover_image_url && (
        <CoverImage
          uri={item.cover_image_url}
          style={sheet.absoluteFill}
          contentFit="cover"
          recyclingKey={item.id}
        />
      )}
      <LinearGradient
        colors={['rgba(0,0,0,0.72)', 'rgba(0,0,0,0.18)', 'transparent']}
        start={{ x: 0, y: 0 }}
        end={{ x: 0.7, y: 0 }}
        style={sheet.absoluteFill}
      />
      <Text style={sheet.top10Rank}>{rank}</Text>
      <View className={s.hCardPriceBadge}>
        <Text className={s.hCardPriceBadgeText}>{formatPrice(item.price, item.currency)}</Text>
      </View>
    </View>
    <View className={s.hCardBody}>
      {/* colors.subText is required, not optional: hCardCat carries no colour
          of its own, so without it the date falls back to RN's default black
          and vanishes on the dark theme. EventCard has always had this; the
          Top 10 variant was the one that missed it. */}
      <Text className={s.hCardCat} style={{ color: colors.subText }} numberOfLines={1}>{fmtCardDate(item.start_time)}</Text>
      <Text className={s.hCardTitle} style={{ color: colors.text }} numberOfLines={2}>{item.title}</Text>
      <Text className={s.hCardMeta} style={{ color: colors.subText }} numberOfLines={1}>{item.venue_name}</Text>
    </View>
  </TouchableOpacity>
));

// ─── PERF: React.memo — section rows skip re-render unless section data changes
// Removes entries with a duplicate `id`, keeping first occurrence and order.
// React keys must be unique within a list; backend feeds can occasionally repeat
// an event (organic + sponsored copy of the same id), which would otherwise warn.
function dedupeById<T extends { id: string }>(items: T[]): T[] {
  const seen = new Set<string>();
  const out: T[] = [];
  for (const it of items) {
    if (!seen.has(it.id)) { seen.add(it.id); out.push(it); }
  }
  return out;
}

const SectionRow = memo(({
  section, isTop10 = false, marginTop = false, onEventPress, onViewAll, colors,
}: {
  section: HomeSection; isTop10?: boolean; marginTop?: boolean;
  onEventPress: (event: Event) => void; onViewAll: (sectionId: string, title: string, events: Event[]) => void; colors: any;
}) => {
  // Guard against a section arriving with the same event id twice (e.g. an event
  // present organically AND woven in as its sponsored card). React requires keys
  // to be unique within a list, so de-dupe before rendering.
  const [events,      setEvents]      = useState<Event[]>(dedupeById(section.events ?? []));
  const [nextCursor,  setNextCursor]  = useState<string>(section.next_cursor ?? '');
  const [loadingMore, setLoadingMore] = useState(false);

  useEffect(() => {
    setEvents(dedupeById(section.events ?? []));
    setNextCursor(section.next_cursor ?? '');
  }, [section]);

  const handleLoadMore = useCallback(async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    const result = await loadMoreSectionEvents(section.id, nextCursor);
    if (result && result.data.length > 0) {
      setEvents((prev) => {
        const existingIds = new Set(prev.map((e) => e.id));
        const fresh = result.data.filter((e) => !existingIds.has(e.id));
        return [...prev, ...fresh];
      });
      setNextCursor(result.next_cursor ?? '');
    } else {
      setNextCursor('');
    }
    setLoadingMore(false);
  }, [nextCursor, loadingMore, section.id]);

  if (!events.length) return null;

  return (
    <>
      {marginTop && (
        <View style={{ height: 1, backgroundColor: 'transparent', marginVertical: 4 }} />
      )}
      <View className={marginTop ? s.sectionMt : s.section}>
        <View className={s.sectionHeader}>
          <Text className={s.sectionTitle} style={{ color: colors.text }}>{section.title.toUpperCase()}</Text>
          <TouchableOpacity onPress={() => onViewAll(section.id, section.title, events)}>
            <Text className={s.sectionLink}>VIEW ALL</Text>
          </TouchableOpacity>
        </View>
      </View>
      <FlatList
        horizontal
        data={events}
        keyExtractor={(e) => e.id}
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }}
        onEndReachedThreshold={0.4}
        onEndReached={handleLoadMore}
        windowSize={5}
        maxToRenderPerBatch={5}
        initialNumToRender={4}
        ListFooterComponent={
          loadingMore ? (
            <View style={{ width: 56, alignItems: 'center', justifyContent: 'center' }}>
              <ActivityIndicator size="small" color={colors.text} />
            </View>
          ) : null
        }
        renderItem={({ item, index }) =>
          isTop10
          ? <Top10Card item={item} rank={index + 1} onPress={onEventPress} colors={colors} />
          : <EventCard item={item} onPress={onEventPress} colors={colors} priority={index < 3 ? 'high' : 'normal'} />
        }
      />
    </>
  );
});

// ─── Hero dot indicators ──────────────────────────────────────────────────────
// The active-dot index used to be state on the screen itself. The carousel
// advances every 5 seconds, so every 5 seconds the entire home feed -- hero,
// every section row, every editorial list -- re-rendered just to recolour five
// 6px dots. That render is long enough to swallow a tap on the tab bar or a
// card. The index now lives here, and the screen pushes it in imperatively.
type HeroDotsHandle = { setIndex: (i: number) => void };

const HeroDots = memo(forwardRef<HeroDotsHandle, { count: number; onPress: (i: number) => void }>(
  ({ count, onPress }, ref) => {
    const [index, setIndex] = useState(0);
    useImperativeHandle(ref, () => ({ setIndex }), []);
    if (count <= 1) return null;
    return (
      <View style={{ position: 'absolute', bottom: 14, right: 16, flexDirection: 'row', gap: 5, zIndex: 10 }}>
        {Array.from({ length: count }).map((_, idx) => (
          <TouchableOpacity key={idx} onPress={() => onPress(idx)} hitSlop={{ top: 10, bottom: 10, left: 8, right: 8 }}>
            <View style={{
              width: idx === index ? 18 : 6,
              height: 6,
              borderRadius: 3,
              backgroundColor: idx === index ? '#FF2D55' : 'rgba(255,255,255,0.4)',
            }} />
          </TouchableOpacity>
        ))}
      </View>
    );
  },
));
HeroDots.displayName = 'HeroDots';

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function Index() {
  const router = useRouter();
  const setSelectedEvent     = useEventStore((state) => state.setSelectedEvent);
  const setAllEvents         = useEventStore((state) => state.setAllEvents);
  const setDbCategories      = useEventStore((state) => state.setDbCategories);
  const setSectionMeta       = useEventStore((state) => state.setSectionMeta);
  const recentlyViewedIds    = useEventStore((state) => state.recentlyViewedIds);
  const recentlyViewedCache  = useEventStore((state) => state.recentlyViewedCache);
  const loadRecentlyViewed   = useEventStore((state) => state.loadRecentlyViewed);
  const clearRecentlyViewed  = useEventStore((state) => state.clearRecentlyViewed);
  // Subscribe to the auth user so we can re-fetch when it transitions from
  // null → known. _layout.tsx calls loadUser() on startup; if that validates
  // or refreshes the token, user becomes non-null and we need a fresh home
  // fetch so the personalised "For You" section appears without a manual pull.
  const authUserId = useAuthStore((state) => state.user?.id ?? null);
  const { colors, isDark } = useAppTheme();
  const { width: screenWidth } = useWindowDimensions();
  const heroWidth = screenWidth - 32; // hero has mx-4 (16px each side)

  const lastScrollY      = useRef(0);
  const headerOffset     = useRef(0);
  const headerAnim       = useRef(new Animated.Value(0)).current;
  const headerHeight     = useRef(120);
  const featuredIndexRef = useRef(0);
  const heroBannerRef    = useRef<FlatList<Event>>(null);
  const heroDotsRef      = useRef<HeroDotsHandle>(null);
  // Carousel only ticks while this tab is actually on screen.
  const screenFocused    = useRef(true);

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
  const [refreshing,     setRefreshing]     = useState(false);
  const [error,          setError]          = useState<string | null>(null);

  const isFiltered = !!(
    filterParams.city     ||
    filterParams.province ||
    filterParams.lat      ||
    (filterParams.date && filterParams.date !== 'any')
  );

  // PERF: memoize carousel pool — only recomputes when homeSections changes
  const carouselEvents = useMemo<Event[]>(() => pickCarouselEvents(homeSections), [homeSections]);

  // PERF: useMemo + O(n) Set-based dedup — was O(n²) findIndex inline every render
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

  // PERF: memoize feedSections so filter() doesn't run every render
  const feedSections = useMemo(
    () => sections.filter((sec) => sec.id !== 'featured'),
    [sections]
  );

  // Recently viewed -- resolved instantly from the in-memory/persisted cache.
  // No API call needed: the cache is populated when the user opens an event.
  const recentlyViewedEvents = useMemo<Event[]>(() => {
  if (!authUserId || !recentlyViewedIds.length) return [];
  const allEventsMap = new Map(allEvents.map((e) => [e.id, e]));
  return recentlyViewedIds.flatMap((id) => {
    const e = recentlyViewedCache[id] ?? allEventsMap.get(id);
    return e ? [e] : [];
  });
}, [recentlyViewedIds, recentlyViewedCache, allEvents, authUserId]);

  // PERF: memoize all 8 editorial section datasets
  const freeEvents    = useMemo(() => allEvents.filter((e) => e.price === 0).slice(0, 10), [allEvents]);
  const premiumEvents = useMemo(() => allEvents.filter((e) => e.price >= 3000).slice(0, 10), [allEvents]);
  const comingUp      = useMemo(() => {
    const nowMs = Date.now();
    return allEvents.filter((e) => { const d = (new Date(e.start_time).getTime() - nowMs) / 86400000; return d >= 15 && d <= 45; }).slice(0, 10);
  }, [allEvents]);
  const otherCities   = useMemo(() => allEvents.filter((e) => !['Johannesburg', 'johannesburg'].includes(e.city ?? '')).slice(0, 10), [allEvents]);
  const musicEvents   = useMemo(() => allEvents.filter((e) => e.category?.toLowerCase().includes('concert') || e.category?.toLowerCase().includes('music')).slice(0, 10), [allEvents]);
  const galaEvents    = useMemo(() => allEvents.filter((e) => e.category?.toLowerCase().includes('gala') || e.category?.toLowerCase().includes('exclusive')).slice(0, 10), [allEvents]);
  const techEvents    = useMemo(() => allEvents.filter((e) => e.category?.toLowerCase().includes('tech')).slice(0, 10), [allEvents]);
  const foodEvents    = useMemo(() => allEvents.filter((e) => e.category?.toLowerCase().includes('food')).slice(0, 10), [allEvents]);

  // PERF: memoize spotlight events
  const spotlightEvents = useMemo(() => {
    const preferred = allEvents.filter((e) => e.category?.toLowerCase().includes(spotlightConfig.category)).slice(0, 4);
    if (preferred.length > 0) return preferred;
    for (const cfg of SPOTLIGHT_CONFIGS) {
      if (cfg.category === spotlightConfig.category) continue;
      const fallback = allEvents.filter((e) => e.category?.toLowerCase().includes(cfg.category)).slice(0, 4);
      if (fallback.length > 0) return fallback;
    }
    return allEvents.slice(0, 4);
  }, [allEvents, spotlightConfig]);

  const sportsBannerEvent = spotlightEvents[0] ?? null;
  const sportsListEvents  = spotlightEvents.slice(1);

  useEffect(() => {
    if (allEvents.length > 0) setAllEvents(allEvents);
  }, [allEvents]);

  // ── Featured carousel (FlatList-based, no fade-race glitch) ──────────────
  const advanceTo = useCallback((nextIdx: number) => {
    featuredIndexRef.current = nextIdx;
    heroDotsRef.current?.setIndex(nextIdx);
    // Guard: scrollToIndex crashes if list is empty or index out of range
    try {
      heroBannerRef.current?.scrollToIndex({ index: nextIdx, animated: true });
    } catch {}
  }, []);

  const heroViewabilityConfig = useRef({ viewAreaCoveragePercentThreshold: 50 });
  const onHeroViewableItemsChanged = useRef(({ viewableItems }: { viewableItems: Array<{ index: number | null }> }) => {
    if (viewableItems[0] && viewableItems[0].index != null) {
      const idx = viewableItems[0].index;
      featuredIndexRef.current = idx;
      heroDotsRef.current?.setIndex(idx);
    }
  });

  useFocusEffect(
    useCallback(() => {
      screenFocused.current = true;
      return () => { screenFocused.current = false; };
    }, []),
  );

  useEffect(() => {
    if (carouselEvents.length <= 1) return;
    const timer = setInterval(() => {
      // Don't animate (or scroll) a carousel nobody is looking at.
      if (!screenFocused.current) return;
      const next = (featuredIndexRef.current + 1) % carouselEvents.length;
      advanceTo(next);
    }, 5000);
    return () => clearInterval(timer);
  }, [carouselEvents.length, advanceTo]);

  useEffect(() => {
    featuredIndexRef.current = 0;
    heroDotsRef.current?.setIndex(0);
    // Only scroll if the list already has items — scrollToIndex throws when length is 0
    if ((heroBannerRef.current as any)?._listRef?._getItemCount?.() > 0) {
      heroBannerRef.current?.scrollToIndex({ index: 0, animated: false });
    }
  }, [homeSections]);

  // ── Category derivation ───────────────────────────────────────────────────
  const categories = useMemo(() => {
    const seen = new Set<string>();
    const cats: string[] = [];
    for (const event of allEvents) {
      const raw = event.category?.trim();
      if (!raw) continue;
      const key = raw.toLowerCase();
      if (!seen.has(key)) { seen.add(key); cats.push(raw.replace(/\b\w/g, (c) => c.toUpperCase())); }
    }
    cats.sort((a, b) => a.localeCompare(b));
    return ['All', ...cats];
  }, [allEvents]);

  useEffect(() => {
    if (activeCategory !== 'All' && !categories.includes(activeCategory)) setActiveCategory('All');
  }, [categories, activeCategory]);

  // ── Spotlight config selection ────────────────────────────────────────────
  useEffect(() => {
    if (!allEvents.length) return;
    const hasEvents = (cfg: typeof SPOTLIGHT_CONFIGS[0]) =>
      allEvents.some((e) => e.category?.toLowerCase().includes(cfg.category));
    if (hasEvents(spotlightConfig)) return;
    const fallback = SPOTLIGHT_CONFIGS.find((cfg) => cfg.category !== spotlightConfig.category && hasEvents(cfg));
    if (fallback) setSpotlightConfig(fallback);
  }, [allEvents, spotlightConfig]);

  // ── Data fetching ─────────────────────────────────────────────────────────
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
      // Warm the cache for the first few covers of each section so they render
      // instantly as the user scrolls instead of downloading on first sight.
      prefetchImages(
        loaded.flatMap((sec) => (sec.events ?? []).slice(0, 6)).map((e) => e.cover_image_url),
      );
      // The line above warms the _thumb.jpg variant, which is right for cards
      // but wrong for the hero: it renders banner_image_url at full size, so it
      // never touches the thumbnail and starts its download cold. Warm the
      // first two slides at full resolution instead. Only two -- the rest of
      // the carousel is a long way behind a 5-second auto-advance, and full-size
      // banners are not something to be pulling down six at a time on mobile
      // data.
      prefetchFullImages(
        pickCarouselEvents(loaded).slice(0, 2).map((e) => e.banner_image_url ?? e.cover_image_url),
      );
      const cats = categoriesRes?.data?.categories ?? [];
      if (cats.length > 0) setDbCategories(cats);
    } catch (err: any) {
      setError(err?.message ?? 'Failed to load. Check your API URL and that the backend is running.');
    }
  }, []);

  // Single startup fetch: wait until session restore has settled (authReady),
  // then load the feed once with the correct guest/signed-in state. This avoids
  // the old "fetch as guest on mount, then re-fetch when auth resolves" double
  // request.
  const authReady = useAuthStore((state) => state.authReady);
  const didInitialFetch = useRef(false);
  const prevAuthUserId = useRef<string | null>(null);
  // Monotonic token: only the most recent filter apply is allowed to write to
  // `sections`, so a slow earlier request can never clobber a newer one.
  const filterReqId = useRef(0);

  useEffect(() => {
    if (!authReady || didInitialFetch.current) return;
    didInitialFetch.current = true;
    prevAuthUserId.current = authUserId;
    if (authUserId) loadRecentlyViewed(authUserId);
    fetchSections(filterParams.city).finally(() => setLoading(false));
  }, [authReady, authUserId, fetchSections, filterParams.city, loadRecentlyViewed]);

  // Later auth changes (logging in or out while the app is open) swap the
  // personalised sections in or out. The initial load is handled above, so this
  // only runs on a genuine post-startup transition.
  useEffect(() => {
    if (!didInitialFetch.current) return;
    if (authUserId === prevAuthUserId.current) return;
    prevAuthUserId.current = authUserId;
    if (authUserId) {
      loadRecentlyViewed(authUserId);
    } else {
      clearRecentlyViewed();
    }
    fetchSections(filterParams.city);
  }, [authUserId, fetchSections, filterParams.city, loadRecentlyViewed, clearRecentlyViewed]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await fetchSections(filterParams.city);
    setRefreshing(false);
  }, [fetchSections, filterParams.city]);

  // ── Empty-state action ──────────────────────────────────────────────────────
  //
  // "Try again" used to call onRefresh, which loads the UNFILTERED home feed —
  // so the button appeared to reset the location while the header still showed
  // the old one, and the feed no longer matched what the header claimed.
  //
  // Retrying is also the wrong verb here. The usual reason this state shows is
  // that a filter matched nothing, and re-running the same filter returns the
  // same nothing. Clearing it is the action that actually helps.
  const hasActiveFilter = !!(
    filterParams.city ||
    filterParams.province ||
    filterParams.lat ||
    (filterParams.date && filterParams.date !== 'any')
  );

  // Mirrors the no-filter branch of handleFilterApply: bump filterReqId so any
  // in-flight filtered request cannot land after this and repopulate the list.
  const clearFilters = useCallback(() => {
    filterReqId.current += 1;
    setFilterParams({ date: 'any' });
    setError(null);
    setSections(homeSections);
    headerOffset.current = 0;
    Animated.spring(headerAnim, { toValue: 0, useNativeDriver: true, bounciness: 0, speed: 14 }).start();
  }, [homeSections, headerAnim]);

  // PERF: useCallback so handleFilterApply reference is stable across renders
  const handleFilterApply = useCallback(async (params: FilterParams) => {
    setFilterParams(params);
    setFilterOpen(false);

    headerOffset.current = 0;
    Animated.spring(headerAnim, { toValue: 0, useNativeDriver: true, bounciness: 0, speed: 14 }).start();

    const hasLocation = !!(params.city || params.province || params.lat);
    const hasDate     = !!(params.date && params.date !== 'any');

    // No filter at all → restore the baseline (Anywhere / Any date) home feed.
    if (!hasLocation && !hasDate) {
      filterReqId.current += 1;        // invalidate any in-flight filter request
      setError(null);
      setSections(homeSections);
      return;
    }

    // Active filter → /v1/events/filter is the SINGLE source of truth for what's
    // shown. We deliberately do NOT also call fetchSections() here: it writes to
    // the same `sections` state, and whichever request happened to resolve last
    // would win — so the unfiltered home feed could overwrite the filtered list,
    // making the location/date filter look like it did nothing.
    const reqId = ++filterReqId.current;
    try {
      setLoading(true);
      setError(null);
      const qs  = buildFilterQueryString(params);
      const res = await fetch(`${getBaseUrl()}/v1/events/filter?${qs}`);
      if (!res.ok) throw new Error(`Filter failed: ${res.status}`);
      const data = await res.json();

      // A newer apply started while we were awaiting — discard this stale result.
      if (reqId !== filterReqId.current) return;

      if (Array.isArray(data.sections) && data.sections.length > 0) {
        setSections(data.sections);
      } else {
        setSections([]);
        setError('No events found for that filter.');
      }
    } catch {
      if (reqId !== filterReqId.current) return;
      // Surface the failure instead of silently falling back to the unfiltered
      // feed, which previously made a failed filter look like a working-but-
      // ignored one.
      setSections([]);
      setError('Could not load filtered events. Pull down to refresh and try again.');
    } finally {
      if (reqId === filterReqId.current) setLoading(false);
    }
  }, [homeSections, headerAnim]);

  const handleSelectCategory = (cat: string) => {
    setDropdownOpen(false);
    if (cat === 'All') { setActiveCategory('All'); return; }
    router.push(`/category/${encodeURIComponent(cat)}`);
  };

  // PERF: stable reference so EventCard/SectionRow don't re-render unnecessarily
  const handleEventPress = useCallback((event: Event) => {
    setSelectedEvent(event);
    // The detail screen renders the full-size banner, but only the thumbnail was
    // warmed for this card -- so kick the real download off now and let it run
    // under the push animation instead of starting it when the screen lands.
    prefetchFullImages([event.banner_image_url ?? event.cover_image_url]);
    router.push(`/event/${event.id}`);
  }, [setSelectedEvent, router]);

  // GET TICKETS skips the details page and lands directly on ticket selection.
  const handleGetTickets = useCallback((event: Event) => {
    setSelectedEvent(event);
    prefetchFullImages([event.banner_image_url ?? event.cover_image_url]);
    router.push('/event/purchase');
  }, [setSelectedEvent, router]);

  const handleViewAll = useCallback((sectionId: string, title: string, events: Event[]) => {
    setSectionMeta({ title, sectionId, events });
    router.push(`/section/${encodeURIComponent(title)}`);
  }, [setSectionMeta, router]);

  const handleViewAllEditorial = useCallback((filterKey: string, title: string) => {
    setSectionMeta({ title, filterKey });
    router.push(`/section/${encodeURIComponent(title)}`);
  }, [setSectionMeta, router]);

  const locationLabel =
    filterParams.city     ??
    filterParams.province ??
    (filterParams.lat ? 'Near me' : 'Anywhere');

  const dateLabelText =
    filterParams.date === 'any'      ? 'All dates'    :
    filterParams.date === 'tomorrow' ? 'Tomorrow'     :
    filterParams.date === 'weekend'  ? 'This Weekend' :
    filterParams.date === 'custom' && filterParams.date_from && filterParams.date_to
      ? formatDateRangeLabel(filterParams.date_from, filterParams.date_to)
      : 'All dates';

  // PERF: useCallback so snapHeader reference is stable
  const snapHeader = useCallback(() => {
    const target = headerOffset.current < -(headerHeight.current / 2) ? -headerHeight.current : 0;
    Animated.spring(headerAnim, { toValue: target, useNativeDriver: true, bounciness: 0, speed: 14 })
      .start(() => { headerOffset.current = target; });
  }, [headerAnim]);

  // ── Offline empty state ─────────────────────────────────────────────────────
  // If the very first load fails with a network error and we have nothing cached
  // to show, replace the whole page with a single "connect to network" state
  // instead of rendering the hero + an error banner.
  const isNetworkError = !!error && /network/i.test(error);
  if (!loading && isNetworkError && sections.length === 0) {
    return (
      <View style={{ flex: 1, backgroundColor: colors.background, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 40, gap: 14 }}>
        <View style={{ width: 84, height: 84, borderRadius: 26, backgroundColor: colors.sectionCardBg, alignItems: 'center', justifyContent: 'center', marginBottom: 4 }}>
          <Ionicons name="wifi-outline" size={40} color={colors.iconColor} />
        </View>
        <Text style={{ color: colors.text, fontSize: 20, fontWeight: '800', textAlign: 'center', letterSpacing: -0.3 }}>
          Connect to Network
        </Text>
        <Text style={{ color: colors.subText, fontSize: 13, textAlign: 'center', lineHeight: 19 }}>
          You're not connected to the internet. Check your connection to browse events.
        </Text>
        <TouchableOpacity
          onPress={async () => { setLoading(true); await fetchSections(filterParams.city); setLoading(false); }}
          activeOpacity={0.85}
          style={{ marginTop: 10, paddingHorizontal: 28, paddingVertical: 13, borderRadius: 12, backgroundColor: colors.ctaBg }}
        >
          <Text style={{ color: colors.ctaText, fontSize: 14, fontWeight: '800', letterSpacing: 0.3 }}>Try Again</Text>
        </TouchableOpacity>
      </View>
    );
  }

  // ── Nothing to show at all ──────────────────────────────────────────────────
  // Counts events across EVERY section, featured included: `allEvents` skips
  // the featured section deliberately, so it would report empty for a feed that
  // has a carousel and nothing else.
  //
  // Only when the count is zero — a filter that matched nothing already falls
  // back to the unfiltered feed and lands in the existing empty state below,
  // which offers to clear the filter. That is a different problem with a
  // different fix, and this cover would hide it.
  //
  // Placed with the offline state above: after every hook, before the main
  // return, so hook order is identical on all three paths.
  const totalEventCount = sections.reduce((n, sec) => n + (sec.events?.length ?? 0), 0);
  if (!loading && !error && totalEventCount === 0) {
    return (
      <ComingSoon
        colors={colors}
        onRefresh={() => fetchSections(filterParams.city)}
      />
    );
  }

  return (
    <View className={s.screen} style={{ backgroundColor: colors.background }}>
      <ScrollView
        className={s.scroll}
        style={{ backgroundColor: colors.background }}
        showsVerticalScrollIndicator={false}
        // A horizontal swipe across a row is never perfectly horizontal. Without
        // a directional lock the page also scrolls by the few vertical pixels in
        // the gesture, so everything under the row you're swiping drifts up and
        // down with your thumb.
        directionalLockEnabled
        scrollEventThrottle={16}
        onScroll={(e) => {
          const currentY = e.nativeEvent.contentOffset.y;
          const delta    = currentY - lastScrollY.current;
          lastScrollY.current = currentY;
          if (currentY <= 0) {
            headerOffset.current = 0;
            headerAnim.setValue(0);
          } else {
            headerOffset.current = Math.min(0, Math.max(-headerHeight.current, headerOffset.current - delta));
            headerAnim.setValue(headerOffset.current);
          }
          // PERF: removed setScrolled(currentY > 10) — state was computed but
          // never consumed in JSX, causing a re-render every 16ms during scroll.
        }}
        onScrollEndDrag={snapHeader}
        onMomentumScrollEnd={snapHeader}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={colors.text} />
        }
      >
        {/* Hero Banner — horizontal FlatList swiper (no fade-race glitch, swipe-enabled) */}
        <View className={s.hero}>
          <FlatList
            ref={heroBannerRef}
            data={carouselEvents}
            keyExtractor={(item) => item.id}
            ListEmptyComponent={
              <View style={{ width: heroWidth, height: 380, borderRadius: 14, overflow: 'hidden' }}>
                <View className={s.heroImagePlaceholder} />
                <LinearGradient
                  colors={['transparent', 'rgba(10,10,10,0.85)', '#0a0a0a']}
                  locations={[0.3, 0.7, 1]}
                  style={sheet.absoluteFill}
                />
                <View className={s.heroContent}>
                  <Text className={s.heroBadge}>✦ Featured</Text>
                  <Text className={s.heroTitle}>{'DISCOVER\nTOP EVENTS'}</Text>
                  <Text className={s.heroSub}>{'Find and book the best events happening near you.'}</Text>
                </View>
              </View>
            }
            horizontal
            pagingEnabled
            showsHorizontalScrollIndicator={false}
            scrollEnabled={carouselEvents.length > 1}
            onViewableItemsChanged={onHeroViewableItemsChanged.current}
            viewabilityConfig={heroViewabilityConfig.current}
            getItemLayout={(_data, index) => ({ length: heroWidth, offset: heroWidth * index, index })}
            style={{ width: heroWidth, height: 380 }}
            renderItem={({ item: evt }) => (
              <TouchableOpacity
                activeOpacity={0.95}
                onPress={() => evt && handleEventPress(evt)}
                style={{ width: heroWidth, height: 380, borderRadius: 14, overflow: 'hidden' }}
              >
                {(evt?.banner_image_url ?? evt?.cover_image_url) ? (
                  <Image
                    source={{ uri: evt!.banner_image_url ?? evt!.cover_image_url! }}
                    style={sheet.absoluteFill}
                    contentFit="cover"
                    cachePolicy="memory-disk"
                    priority="high"
                    transition={200}
                    recyclingKey={evt!.id}
                  />
                ) : (
                  <View className={s.heroImagePlaceholder} />
                )}
                <LinearGradient
                  colors={['transparent', 'rgba(10,10,10,0.85)', '#0a0a0a']}
                  locations={[0.3, 0.7, 1]}
                  style={sheet.absoluteFill}
                />
                <View className={s.heroContent}>
                  <Text className={s.heroBadge}>✦ {evt?.category ?? 'Featured'}</Text>
                  <Text className={s.heroTitle} numberOfLines={2}>
                    {evt?.title.toUpperCase() ?? 'DISCOVER\nTOP EVENTS'}
                  </Text>
                  <Text className={s.heroSub} numberOfLines={3}>
                    {evt?.description ?? 'Find and book the best events happening near you.'}
                  </Text>
                  <View className={s.heroActions}>
                    <TouchableOpacity className={s.btnPrimary} style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 6 }} onPress={() => evt && handleGetTickets(evt)}>
                      <Ticket size={15} color="#FFFFFF" weight="fill" />
                      <Text className={s.btnPrimaryText}>GET TICKETS</Text>
                    </TouchableOpacity>
                    <TouchableOpacity className={s.btnBlue} onPress={() => evt && handleEventPress(evt)}>
                      <Text className={s.btnBlueText}>VIEW DETAILS</Text>
                    </TouchableOpacity>
                  </View>
                </View>
              </TouchableOpacity>
            )}
          />
          {/* Dot indicators — tap to jump, auto-updates on swipe */}
          <HeroDots ref={heroDotsRef} count={carouselEvents.length} onPress={advanceTo} />
        </View>

        {/* Category Filter */}
        <View className={s.filterRow}>
          {activeCategory !== 'All' && (
            <TouchableOpacity className={s.filterClearBtn} style={{ borderColor: colors.dividerColor }} onPress={() => setActiveCategory('All')}>
              <Text className={s.filterClearText} style={{ color: colors.text }}>✕</Text>
            </TouchableOpacity>
          )}
          <TouchableOpacity
            className={`${s.filterChip} ${activeCategory !== 'All' ? s.filterChipActive : ''}`}
            style={activeCategory === 'All'
              ? { borderColor: colors.text, backgroundColor: 'transparent' }
              : { borderColor: isDark ? '#ffffff' : '#000000', backgroundColor: 'transparent' }
            }
            onPress={() => setDropdownOpen(true)}
          >
            <Text className={`${s.filterChipText} ${activeCategory !== 'All' ? s.filterChipTextActive : ''}`} style={{ color: colors.text }}>
              {activeCategory === 'All' ? 'CATEGORIES' : activeCategory.toUpperCase()}
            </Text>
            <Text className={`${s.filterChipArrow} ${activeCategory !== 'All' ? s.filterChipTextActive : ''}`} style={{ color: colors.text }}> ▾</Text>
          </TouchableOpacity>
        </View>

        {/* Category Dropdown Modal */}
        <Modal visible={dropdownOpen} transparent animationType="slide" onRequestClose={() => setDropdownOpen(false)}>
          <View className={s.modalBackdrop}>
            <BlurView intensity={130} tint={isDark ? 'dark' : 'light'} style={{ flex: 1, width: '100%', paddingTop: 64, paddingBottom: 40, alignItems: 'center' }}>
              <FlatList
                data={categories}
                keyExtractor={(item) => item}
                showsVerticalScrollIndicator={false}
                renderItem={({ item }) => (
                  <TouchableOpacity className={s.modalItem} onPress={() => handleSelectCategory(item)}>
                    <Text
                      className={`${s.modalItemText} ${activeCategory === item ? s.modalItemTextActive : ''}`}
                      style={{ color: activeCategory === item ? colors.text : colors.subText }}
                    >{item}</Text>
                  </TouchableOpacity>
                )}
              />
              <TouchableOpacity className={s.modalCloseBtn} style={{ backgroundColor: colors.sectionCardBg }} onPress={() => setDropdownOpen(false)}>
                <Text className={s.modalCloseBtnText} style={{ color: colors.text }}>✕</Text>
              </TouchableOpacity>
            </BlurView>
          </View>
        </Modal>

        {/* Empty / error state.
            Restyled to match the rest of the feed: a centred block on the page
            background, using the same section-card surface as every other card,
            instead of a tinted red slab with a warning glyph and a shouting
            all-caps heading. The pink wash and the ⚠ read as "something is
            broken" even when the copy underneath says the filter simply matched
            nothing, which is the most common reason this shows at all. */}
        {error && (
          <View style={{ alignItems: 'center', paddingHorizontal: 40, paddingTop: 32, paddingBottom: 40 }}>
            <View style={{
              width: 64, height: 64, borderRadius: 22,
              backgroundColor: colors.sectionCardBg,
              alignItems: 'center', justifyContent: 'center', marginBottom: 16,
            }}>
              <MagnifyingGlass size={26} color={colors.iconColor} weight="regular" />
            </View>

            <Text style={{
              color: colors.text, fontSize: 17, fontWeight: '800',
              letterSpacing: -0.3, textAlign: 'center', marginBottom: 6,
            }}>
              Nothing here yet
            </Text>

            <Text style={{
              color: colors.subText, fontSize: 13, lineHeight: 19, textAlign: 'center',
            }}>
              {error}
            </Text>

            <TouchableOpacity
              onPress={hasActiveFilter ? clearFilters : onRefresh}
              activeOpacity={0.85}
              style={{
                marginTop: 18, paddingHorizontal: 22, paddingVertical: 11,
                borderRadius: 999, backgroundColor: colors.sectionCardBg,
              }}
            >
              <Text style={{ color: colors.text, fontSize: 13, fontWeight: '700' }}>
                {hasActiveFilter ? 'Clear filters' : 'Try again'}
              </Text>
            </TouchableOpacity>
          </View>
        )}

        {/* ===== RECENTLY VIEWED - logged-in users only, always the first section ===== */}
        {authUserId && recentlyViewedEvents.length > 0 && (
          <>
            <View className={s.sectionMt}>
              <View className={s.sectionHeader}>
                <Text className={s.sectionTitle} style={{ color: colors.text }}>RECENTLY VIEWED</Text>
              </View>
            </View>
            <FlatList
              horizontal
              data={recentlyViewedEvents}
              keyExtractor={(e) => e.id}
              showsHorizontalScrollIndicator={false}
              windowSize={5}
              maxToRenderPerBatch={5}
              initialNumToRender={4}
              contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }}
              renderItem={({ item }) => (
                <EventCard item={item} onPress={handleEventPress} colors={colors} />
              )}
            />
          </>
        )}

        {/* Dynamic Feed Sections */}
        {loading ? <SkeletonFeed /> : feedSections.map((section) => (
          <SectionRow key={section.id} section={section} isTop10={section.id === 'trending'} marginTop={true} onEventPress={handleEventPress} onViewAll={handleViewAll} colors={colors} />
        ))}

        {/* Spotlight Section */}
        {!loading && !isFiltered && sportsBannerEvent && (
          <>
            <View className={s.sectionMt}>
              <View className={s.sectionHeader}>
                <Text className={s.sectionTitle} style={{ color: spotlightConfig.accentColor }}>{spotlightConfig.title}</Text>
              </View>
            </View>
            <TouchableOpacity activeOpacity={0.92} onPress={() => handleEventPress(sportsBannerEvent)}>
              <View className={s.sportsBanner} style={{ backgroundColor: spotlightConfig.bgColor }}>
                {(sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url) ? (
                  <Image source={{ uri: sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url! }} style={sheet.absoluteFill} contentFit="cover" cachePolicy="memory-disk" recyclingKey={sportsBannerEvent.id} />
                ) : null}
                <LinearGradient colors={[spotlightConfig.gradientLight, spotlightConfig.gradientDark]} style={sheet.absoluteFill} />
                <View style={{ position: 'absolute', bottom: 0, left: 0, right: 0, height: 3, backgroundColor: spotlightConfig.accentColor, opacity: 0.85 }} />
                <View className={s.sportsBannerContent}>
                  <Text className={s.sportsBadge} style={{ color: spotlightConfig.accentColor }}>{spotlightConfig.badge}</Text>
                  <Text className={s.sportsTitle}>{sportsBannerEvent.title.toUpperCase()}</Text>
                  <Text className={s.sportsSub} numberOfLines={2}>{sportsBannerEvent.description}</Text>
                  <TouchableOpacity className={s.btnOutline} onPress={() => handleEventPress(sportsBannerEvent)}>
                    <Text className={s.btnOutlineText}>VIEW TICKETS</Text>
                  </TouchableOpacity>
                </View>
              </View>
            </TouchableOpacity>
            {sportsListEvents.length > 0 && (
              <View className={s.sportsListContainer}>
                {sportsListEvents.map((sport, index) => (
                  <TouchableOpacity key={sport.id} className={index === sportsListEvents.length - 1 ? s.sportRowLast : s.sportRow} onPress={() => handleEventPress(sport)}>
                    <View className={s.sportThumb}>
                      {sport.cover_image_url && (
                        <Image source={{ uri: sport.cover_image_url }} style={sheet.absoluteFill} contentFit="cover" cachePolicy="memory-disk" recyclingKey={sport.id} />
                      )}
                    </View>
                    <View className={s.sportInfo}>
                      <Text className={s.sportCat} style={{ color: colors.subText }}>{sport.category}</Text>
                      <Text className={s.sportTitle} style={{ color: colors.text }}>{sport.title}</Text>
                      <Text className={s.sportLoc} style={{ color: colors.subText }}>{sport.city}</Text>
                    </View>
                    <Text className={s.sportArrow} style={{ color: colors.iconColor }}>›</Text>
                  </TouchableOpacity>
                ))}
              </View>
            )}
          </>
        )}

        {/* Editorial Sections — hidden when filtered */}
        {!loading && !isFiltered && freeEvents.length > 0 && (
          <><View className={s.sectionMt}><View className={s.sectionHeader}><Text className={s.sectionTitle} style={{ color: colors.text }}>FREE EVENTS</Text><TouchableOpacity onPress={() => handleViewAllEditorial('free', 'Free Events')}><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity></View></View>
          <FlatList horizontal data={freeEvents} keyExtractor={(e) => e.id} showsHorizontalScrollIndicator={false} windowSize={5} maxToRenderPerBatch={5} initialNumToRender={4} contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }} renderItem={({ item }) => <EventCard item={item} onPress={handleEventPress} colors={colors} />} /></>
        )}

        {!loading && !isFiltered && premiumEvents.length > 0 && (
          <><View className={s.sectionMt}><View className={s.sectionHeader}><Text className={s.sectionTitle} style={{ color: colors.text }}>PREMIUM & EXCLUSIVE</Text><TouchableOpacity onPress={() => handleViewAllEditorial('premium', 'Premium & Exclusive')}><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity></View></View>
          <FlatList horizontal data={premiumEvents} keyExtractor={(e) => e.id} showsHorizontalScrollIndicator={false} windowSize={5} maxToRenderPerBatch={5} initialNumToRender={4} contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }} renderItem={({ item }) => <EventCard item={item} onPress={handleEventPress} colors={colors} />} /></>
        )}

        {!loading && !isFiltered && comingUp.length > 0 && (
          <><View className={s.sectionMt}><View className={s.sectionHeader}><Text className={s.sectionTitle} style={{ color: colors.text }}>COMING NEXT MONTH</Text><TouchableOpacity onPress={() => handleViewAllEditorial('coming_up', 'Coming Next Month')}><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity></View></View>
          <FlatList horizontal data={comingUp} keyExtractor={(e) => e.id} showsHorizontalScrollIndicator={false} windowSize={5} maxToRenderPerBatch={5} initialNumToRender={4} contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }} renderItem={({ item }) => <EventCard item={item} onPress={handleEventPress} colors={colors} />} /></>
        )}

        {!loading && !isFiltered && otherCities.length > 0 && (
          <><View className={s.sectionMt}><View className={s.sectionHeader}><Text className={s.sectionTitle} style={{ color: colors.text }}>AROUND THE COUNTRY</Text><TouchableOpacity onPress={() => handleViewAllEditorial('other_cities', 'Around The Country')}><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity></View></View>
          <FlatList horizontal data={otherCities} keyExtractor={(e) => e.id} showsHorizontalScrollIndicator={false} windowSize={5} maxToRenderPerBatch={5} initialNumToRender={4} contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }} renderItem={({ item }) => <EventCard item={item} onPress={handleEventPress} colors={colors} />} /></>
        )}

        {!loading && !isFiltered && musicEvents.length > 0 && (
          <><View className={s.sectionMt}><View className={s.sectionHeader}><Text className={s.sectionTitle} style={{ color: colors.text }}>MUSIC & CONCERTS</Text><TouchableOpacity onPress={() => handleViewAllEditorial('music', 'Music & Concerts')}><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity></View></View>
          <FlatList horizontal data={musicEvents} keyExtractor={(e) => e.id} showsHorizontalScrollIndicator={false} windowSize={5} maxToRenderPerBatch={5} initialNumToRender={4} contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }} renderItem={({ item }) => <EventCard item={item} onPress={handleEventPress} colors={colors} />} /></>
        )}

        {!loading && !isFiltered && galaEvents.length > 0 && (
          <><View className={s.sectionMt}><View className={s.sectionHeader}><Text className={s.sectionTitle} style={{ color: colors.text }}>GALA & BLACK TIE</Text><TouchableOpacity onPress={() => handleViewAllEditorial('gala', 'Gala & Black Tie')}><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity></View></View>
          <FlatList horizontal data={galaEvents} keyExtractor={(e) => e.id} showsHorizontalScrollIndicator={false} windowSize={5} maxToRenderPerBatch={5} initialNumToRender={4} contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }} renderItem={({ item }) => <EventCard item={item} onPress={handleEventPress} colors={colors} />} /></>
        )}

        {!loading && !isFiltered && techEvents.length > 0 && (
          <><View className={s.sectionMt}><View className={s.sectionHeader}><Text className={s.sectionTitle} style={{ color: colors.text }}>TECH & BUSINESS</Text><TouchableOpacity onPress={() => handleViewAllEditorial('tech', 'Tech & Business')}><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity></View></View>
          <FlatList horizontal data={techEvents} keyExtractor={(e) => e.id} showsHorizontalScrollIndicator={false} windowSize={5} maxToRenderPerBatch={5} initialNumToRender={4} contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }} renderItem={({ item }) => <EventCard item={item} onPress={handleEventPress} colors={colors} />} /></>
        )}

        {!loading && !isFiltered && foodEvents.length > 0 && (
          <><View className={s.sectionMt}><View className={s.sectionHeader}><Text className={s.sectionTitle} style={{ color: colors.text }}>FOOD & LIFESTYLE</Text><TouchableOpacity onPress={() => handleViewAllEditorial('food', 'Food & Lifestyle')}><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity></View></View>
          <FlatList horizontal data={foodEvents} keyExtractor={(e) => e.id} showsHorizontalScrollIndicator={false} windowSize={5} maxToRenderPerBatch={5} initialNumToRender={4} contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }} renderItem={({ item }) => <EventCard item={item} onPress={handleEventPress} colors={colors} />} /></>
        )}

        <View style={{ height: 100 }} />
      </ScrollView>

      {/* Animated Header */}
      <Animated.View
        style={[{ transform: [{ translateY: headerAnim }], backgroundColor: colors.headerBg }]}
        className={s.locationHeader}
        onLayout={(e) => { headerHeight.current = e.nativeEvent.layout.height; }}
      >
        {/* The caret is the only thing marking this as tappable. Without it the
            location and date read as a static page title, so the filter sheet
            behind them went undiscovered. hitSlop widens the touch target
            without changing the layout. */}
        <TouchableOpacity
          onPress={() => setFilterOpen(true)}
          activeOpacity={0.7}
          accessibilityRole="button"
          accessibilityLabel={`Filter events. Currently ${locationLabel}, ${dateLabelText}`}
          hitSlop={{ top: 8, bottom: 8, left: 8, right: 16 }}
        >
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 6 }}>
            <Text className={s.locationCity} style={{ color: colors.text }}>{locationLabel}</Text>
            <CaretDown size={16} weight="bold" color={colors.text} />
          </View>
          <Text className={s.locationSub} style={{ color: colors.subText }}>{dateLabelText}</Text>
        </TouchableOpacity>
      </Animated.View>

      {/* Filter bottom sheet */}
      <FilterModal visible={filterOpen} onClose={() => setFilterOpen(false)} initialParams={filterParams} onApply={handleFilterApply} />
    </View>
  );
}