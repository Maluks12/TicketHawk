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
import { prefetchImages } from '../../lib/img';
import { CoverImage } from '../../components/CoverImage';
import { useState, useEffect, useCallback, useRef, useMemo, memo } from 'react';
import { useRouter } from 'expo-router';
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
import { useAuthStore } from '../../store/authStore';
import { s, sheet } from '../../styles/home';
import FilterModal, { FilterParams, buildFilterQueryString, formatDateRangeLabel, getBaseUrl } from '../../components/FilterModal';
import { useAppTheme } from '../../theme/colors';
import { BlurView } from 'expo-blur';

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
  { title: 'SPORTS ELITE',    category: 'sports',   badge: '⚡ SPORTS',    accentColor: '#e8c14a', bgColor: '#1a1200', gradientDark: 'rgba(26,18,0,0.82)',   gradientLight: 'rgba(26,18,0,0.25)'   },
  { title: 'BEST FESTIVALS',  category: 'music',    badge: '🎪 FESTIVAL',  accentColor: '#ff3c6e', bgColor: '#1a0010', gradientDark: 'rgba(26,0,16,0.82)',   gradientLight: 'rgba(26,0,16,0.25)'   },
  { title: 'GALA NIGHTS',     category: 'gala',     badge: '✨ GALA',      accentColor: '#c084fc', bgColor: '#0e0018', gradientDark: 'rgba(14,0,24,0.82)',   gradientLight: 'rgba(14,0,24,0.25)'   },
  { title: 'TECH SUMMIT',     category: 'tech',     badge: '💻 TECH',      accentColor: '#38bdf8', bgColor: '#00101a', gradientDark: 'rgba(0,16,26,0.82)',   gradientLight: 'rgba(0,16,26,0.25)'   },
  { title: 'FOOD & CULTURE',  category: 'food',     badge: '🍽 FOOD',      accentColor: '#fb923c', bgColor: '#1a0800', gradientDark: 'rgba(26,8,0,0.82)',    gradientLight: 'rgba(26,8,0,0.25)'    },
  { title: 'TOP CONCERTS',    category: 'concerts', badge: '🎵 MUSIC',     accentColor: '#34d399', bgColor: '#001a10', gradientDark: 'rgba(0,26,16,0.82)',   gradientLight: 'rgba(0,26,16,0.25)'   },
];

// ─── Helpers ─────────────────────────────────────────────────────────────────

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
      {(item.category?.toLowerCase().includes('music') ||
        item.category?.toLowerCase().includes('concert')) && (
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
      <Text className={s.hCardCat} numberOfLines={1}>{fmtCardDate(item.start_time)}</Text>
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
  const [featuredIndex,  setFeaturedIndex]  = useState(0);

  const isFiltered = !!(
    filterParams.city     ||
    filterParams.province ||
    filterParams.lat      ||
    (filterParams.date && filterParams.date !== 'any')
  );

  // PERF: memoize carousel pool — only recomputes when homeSections changes
  const carouselEvents = useMemo<Event[]>(() => {
    const rawFeatured = homeSections.find((sec) => sec.id === 'featured')?.events ?? [];
    const trendingPool = homeSections.find((sec) => sec.id === 'trending')?.events ?? [];
    return rawFeatured.length >= 2
      ? rawFeatured.slice(0, 6)
      : [
          ...rawFeatured,
          ...trendingPool.filter((e) => !rawFeatured.some((f) => f.id === e.id)),
        ].slice(0, 5);
  }, [homeSections]);

  const featuredEvent: Event | null = carouselEvents[featuredIndex] ?? null;

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
    setFeaturedIndex(nextIdx);
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
      setFeaturedIndex(idx);
    }
  });

  useEffect(() => {
    if (carouselEvents.length <= 1) return;
    const timer = setInterval(() => {
      const next = (featuredIndexRef.current + 1) % carouselEvents.length;
      advanceTo(next);
    }, 5000);
    return () => clearInterval(timer);
  }, [carouselEvents.length, advanceTo]);

  useEffect(() => {
    featuredIndexRef.current = 0;
    setFeaturedIndex(0);
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

  return (
    <View className={s.screen} style={{ backgroundColor: colors.background }}>
      <ScrollView
        className={s.scroll}
        style={{ backgroundColor: colors.background }}
        showsVerticalScrollIndicator={false}
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
          <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor="#000000" />
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
                    <TouchableOpacity className={s.btnPrimary} onPress={() => evt && handleEventPress(evt)}>
                      <Text className={s.btnPrimaryText}>🎟 GET TICKETS</Text>
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
          {carouselEvents.length > 1 && (
            <View style={{ position: 'absolute', bottom: 14, right: 16, flexDirection: 'row', gap: 5, zIndex: 10 }}>
              {carouselEvents.map((_, idx) => (
                <TouchableOpacity key={idx} onPress={() => advanceTo(idx)} hitSlop={{ top: 8, bottom: 8, left: 6, right: 6 }}>
                  <View style={{
                    width: idx === featuredIndex ? 18 : 6,
                    height: 6,
                    borderRadius: 3,
                    backgroundColor: idx === featuredIndex ? '#FF2D55' : 'rgba(255,255,255,0.4)',
                  }} />
                </TouchableOpacity>
              ))}
            </View>
          )}
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

        {/* Error banner */}
        {error && (
          <View style={{ margin: 20, backgroundColor: 'rgba(255,60,110,0.12)', borderWidth: 1, borderColor: colors.dividerColor, borderRadius: 3, padding: 14 }}>
            <Text style={{ color: colors.text, fontWeight: '800', fontSize: 12, marginBottom: 4, letterSpacing: 1 }}>⚠ COULD NOT LOAD SECTIONS</Text>
            <Text style={{ color: colors.subText, fontSize: 11 }}>{error}</Text>
            <TouchableOpacity onPress={onRefresh} style={{ marginTop: 10, alignSelf: 'flex-start', borderWidth: 1, borderColor: colors.text, paddingHorizontal: 14, paddingVertical: 6, borderRadius: 2 }}>
              <Text style={{ color: colors.text, fontWeight: '800', fontSize: 10, letterSpacing: 1.5 }}>RETRY</Text>
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
                <TouchableOpacity><Text className={s.sectionLink} style={{ color: spotlightConfig.accentColor }}>EXPLORE</Text></TouchableOpacity>
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
                    <Text className={s.sportArrow} style={{ color: spotlightConfig.accentColor }}>›</Text>
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
        <TouchableOpacity onPress={() => setFilterOpen(true)}>
          <Text className={s.locationCity} style={{ color: colors.text }}>{locationLabel}</Text>
          <Text className={s.locationSub} style={{ color: colors.subText }}>{dateLabelText}</Text>
        </TouchableOpacity>
        <CartHeaderButton />
      </Animated.View>

      {/* Filter bottom sheet */}
      <FilterModal visible={filterOpen} onClose={() => setFilterOpen(false)} initialParams={filterParams} onApply={handleFilterApply} />
    </View>
  );
}