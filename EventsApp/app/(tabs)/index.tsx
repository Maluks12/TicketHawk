import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  Modal,
  FlatList,
  Image,
  ActivityIndicator,
  RefreshControl,
  Animated,
  StyleSheet,
} from 'react-native';
import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useRouter } from 'expo-router';
import { LinearGradient } from 'expo-linear-gradient';
import { Ionicons } from '@expo/vector-icons';
import * as SecureStore from 'expo-secure-store';
import { getHomeSections, loadMoreSectionEvents, type Event, type HomeSection, api } from '../../lib/api';
import { useEventStore } from '../../store/eventStore';
import { s, sheet } from '../../styles/home';
import FilterModal, { FilterParams, buildFilterQueryString,formatDateRangeLabel } from '../../components/FilterModal';
import { useAppTheme } from '../../theme/colors';

// ─── Cart header button ───────────────────────────────────────────────────────

function CartHeaderButton() {
  const router = useRouter();
  const { colors } = useAppTheme();
  const [count, setCount] = useState(0);
  const prevCount = useRef(0);
  const scaleAnim = useRef(new Animated.Value(1)).current;

  useEffect(() => {
    let mounted = true;
    const refresh = async () => {
      try {
        const sid = await SecureStore.getItemAsync('session_id').catch(() => null);
        const headers: Record<string, string> = {};
        if (sid) headers['X-Session-ID'] = sid;
        const res = await api.get<{ total_items: number }>('/v1/cart', { headers });
        const n = res.data?.total_items ?? 0;
        if (mounted) {
          if (n !== prevCount.current && n > 0) {
            Animated.sequence([
              Animated.spring(scaleAnim, { toValue: 1.4, useNativeDriver: true, bounciness: 14 }),
              Animated.spring(scaleAnim, { toValue: 1,   useNativeDriver: true, bounciness: 6  }),
            ]).start();
          }
          prevCount.current = n;
          setCount(n);
        }
      } catch { /* guest with no cart yet */ }
    };
    refresh();
    const interval = setInterval(refresh, 30_000);
    return () => { mounted = false; clearInterval(interval); };
  }, []);

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
// One of these is picked at random on every app load.
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

// ─── Cards ────────────────────────────────────────────────────────────────────

const fmtCardDate = (iso?: string | null) => {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short', year: 'numeric' });
};

const EventCard = ({ item, onPress, colors }: { item: Event; onPress: () => void; colors: any }) => (
  <TouchableOpacity className={s.hCard} onPress={onPress}>
    <View className={s.hCardImageArea} style={{ backgroundColor: colors.sectionCardBg }}>
      {item.cover_image_url && (
        <Image source={{ uri: item.cover_image_url }} style={sheet.absoluteFill} resizeMode="cover" />
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
);

const Top10Card = ({ item, rank, onPress, colors }: { item: Event; rank: number; onPress: () => void; colors: any }) => (
  <TouchableOpacity className={s.hCard} onPress={onPress}>
    <View className={s.hCardImageArea}>
      {item.cover_image_url && (
        <Image source={{ uri: item.cover_image_url }} style={sheet.absoluteFill} resizeMode="cover" />
      )}
      {/* Dark gradient on the left so the number is always readable */}
      <LinearGradient
        colors={['rgba(0,0,0,0.72)', 'rgba(0,0,0,0.18)', 'transparent']}
        start={{ x: 0, y: 0 }}
        end={{ x: 0.7, y: 0 }}
        style={sheet.absoluteFill}
      />
      {/* Apple-style rank number — top-left, ON the card */}
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
);

// ─── Section row (paginated) ──────────────────────────────────────────────────

const SectionRow = ({
  section, isTop10 = false, marginTop = false, onEventPress, colors,
}: {
  section: HomeSection; isTop10?: boolean; marginTop?: boolean;
  onEventPress: (event: Event) => void; colors: any;
}) => {
  const [events,     setEvents]     = useState<Event[]>(section.events ?? []);
  const [nextCursor, setNextCursor] = useState<string>(section.next_cursor ?? '');
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
        <View style={{ height: 1, backgroundColor: colors.dividerColor, marginVertical: 4 }} />
      )}
      <View className={marginTop ? s.sectionMt : s.section}>
        <View className={s.sectionHeader}>
          <Text className={s.sectionTitle} style={{ color: colors.text }}>{section.title.toUpperCase()}</Text>
          {/* Only show "VIEW ALL" indicator when there are more pages */}
          {nextCursor ? (
            <TouchableOpacity onPress={handleLoadMore} disabled={loadingMore}>
              <Text className={s.sectionLink}>{loadingMore ? '…' : 'MORE'}</Text>
            </TouchableOpacity>
          ) : (
            <TouchableOpacity><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity>
          )}
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
        ListFooterComponent={
          loadingMore ? (
            <View style={{ width: 56, alignItems: 'center', justifyContent: 'center' }}>
              <ActivityIndicator size="small" color="#000000" />
            </View>
          ) : null
        }
        renderItem={({ item, index }) =>
          isTop10
          ? <Top10Card item={item} rank={index + 1} onPress={() => onEventPress(item)} colors={colors} />
          : <EventCard item={item} onPress={() => onEventPress(item)} colors={colors} />
        }
      />
    </>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function Index() {
  const router = useRouter();
  const setSelectedEvent = useEventStore((state) => state.setSelectedEvent);
  const setAllEvents     = useEventStore((state) => state.setAllEvents);   // ← category page filter
  const { colors } = useAppTheme();

  // ── Refs ─────────────────────────────────────────────────────────────────
  const lastScrollY  = useRef(0);
  const headerOffset = useRef(0);
  const headerAnim   = useRef(new Animated.Value(0)).current;
  const headerHeight = useRef(120);

  // Stable random spotlight config — picked once on mount
  const [spotlightConfig, setSpotlightConfig] = useState(
  () => SPOTLIGHT_CONFIGS[Math.floor(Math.random() * SPOTLIGHT_CONFIGS.length)]
  );

  const [activeCategory, setActiveCategory] = useState('All');
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);

  // ── Filter state ─────────────────────────────────────────────────────────
  const [filterOpen, setFilterOpen] = useState(false);
  const [filterParams, setFilterParams] = useState<FilterParams>({ date: 'any' });

  const [sections, setSections]     = useState<HomeSection[]>([]);
  const [loading, setLoading]       = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError]           = useState<string | null>(null);

  const featuredEvent: Event | null =
    sections.find((sec) => sec.id === 'featured')?.events?.[0] ?? null;

  // All non-featured events (deduped)
  const allEvents = sections
    .filter((sec) => sec.id !== 'featured')
    .flatMap((sec) => sec.events ?? [])
    .filter((e, i, arr) => arr.findIndex((x) => x.id === e.id) === i);

  // Keep the store in sync so the category page can filter without an extra API call.
  useEffect(() => {
    if (allEvents.length > 0) setAllEvents(allEvents);
  }, [allEvents.length]);

  // Derive unique categories from the loaded events — always in sync with the DB.
  const categories = useMemo(() => {
    const seen = new Set<string>();
    const cats: string[] = [];
    for (const event of allEvents) {
      const raw = event.category?.trim();
      if (!raw) continue;
      const key = raw.toLowerCase();
      if (!seen.has(key)) {
        seen.add(key);
        cats.push(raw.replace(/\b\w/g, (c) => c.toUpperCase()));
      }
    }
    cats.sort((a, b) => a.localeCompare(b));
    return ['All', ...cats];
  }, [allEvents]);

  // Reset active category if it no longer exists (e.g. after a filter refresh).
  useEffect(() => {
    if (activeCategory !== 'All' && !categories.includes(activeCategory)) {
      setActiveCategory('All');
    }
  }, [categories, activeCategory]);

  // If the current spotlightConfig has no matching events, switch to one that does.
  useEffect(() => {
    if (!allEvents.length) return;
    const hasEvents = (cfg: typeof SPOTLIGHT_CONFIGS[0]) =>
      allEvents.some((e) => e.category?.toLowerCase().includes(cfg.category));
    if (hasEvents(spotlightConfig)) return;
    const fallback = SPOTLIGHT_CONFIGS.find(
      (cfg) => cfg.category !== spotlightConfig.category && hasEvents(cfg)
    );
    if (fallback) setSpotlightConfig(fallback);
  }, [allEvents, spotlightConfig]);

  // Pick events for the spotlight section — pure computation, no hooks.
  const spotlightEvents = (() => {
    const preferred = allEvents.filter((e) =>
      e.category?.toLowerCase().includes(spotlightConfig.category)
    ).slice(0, 4);
    if (preferred.length > 0) return preferred;
    for (const cfg of SPOTLIGHT_CONFIGS) {
      if (cfg.category === spotlightConfig.category) continue;
      const fallback = allEvents.filter((e) =>
        e.category?.toLowerCase().includes(cfg.category)
      ).slice(0, 4);
      if (fallback.length > 0) return fallback;
    }
    return allEvents.slice(0, 4);
  })();

  const sportsBannerEvent = spotlightEvents[0] ?? null;
  const sportsListEvents  = spotlightEvents.slice(1);
  const feedSections      = sections.filter((sec) => sec.id !== 'featured');

  const fetchSections = useCallback(async () => {
    try {
      setError(null);
      const { data } = await getHomeSections();
      setSections(data.sections ?? []);
    } catch (err: any) {
      setError(err?.message ?? 'Failed to load. Check your API URL and that the backend is running.');
    }
  }, []);

  useEffect(() => {
    fetchSections().finally(() => setLoading(false));
  }, [fetchSections]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await fetchSections();
    setRefreshing(false);
  }, [fetchSections]);

  const handleSelectCategory = (cat: string) => {
    setDropdownOpen(false);
    if (cat === 'All') {
      setActiveCategory('All');
      return;
    }
    router.push(`/category/${encodeURIComponent(cat)}`);
  };

  const handleEventPress = (event: Event) => {
    setSelectedEvent(event);
    router.push(`/event/${event.id}`);
  };

  // ── Filter apply ─────────────────────────────────────────────────────────
  const handleFilterApply = async (params: FilterParams) => {
    setFilterParams(params);
    setFilterOpen(false);

    headerOffset.current = 0;
    Animated.spring(headerAnim, {
      toValue: 0,
      useNativeDriver: true,
      bounciness: 0,
      speed: 14,
    }).start();

    try {
      setLoading(true);
      setError(null);
      const qs = buildFilterQueryString(params);
      const apiUrl = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';
      const res = await fetch(`${apiUrl}/v1/events/filter?${qs}`);
      if (!res.ok) throw new Error('Filter request failed');
      const data = await res.json();
      if (data.data) {
        setSections([{
          id: 'filtered', title: 'Results', events: data.data,
          next_cursor: ''
        }]);
      }
    } catch (err: any) {
      await fetchSections();
    } finally {
      setLoading(false);
    }
  };

  // ── Location / date label for header ────────────────────────────────────
  const locationLabel =
    filterParams.city     ??
    filterParams.province ??
    (filterParams.lat ? 'Near me' : 'Near me');

  const dateLabelText =
    filterParams.date === 'any'      ? 'All dates'    :
    filterParams.date === 'tomorrow' ? 'Tomorrow'     :
    filterParams.date === 'weekend'  ? 'This Weekend' :
    filterParams.date === 'custom' && filterParams.date_from && filterParams.date_to
      ? formatDateRangeLabel(filterParams.date_from, filterParams.date_to)
      : 'All dates';

  // ── Snap helper ──────────────────────────────────────────────────────────
  const snapHeader = () => {
    const target = headerOffset.current < -(headerHeight.current / 2) ? -headerHeight.current : 0;
    Animated.spring(headerAnim, {
      toValue: target,
      useNativeDriver: true,
      bounciness: 0,
      speed: 14,
    }).start(() => {
      headerOffset.current = target;
    });
  };

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
            headerOffset.current = Math.min(
              0,
              Math.max(-headerHeight.current, headerOffset.current - delta)
            );
            headerAnim.setValue(headerOffset.current);
          }
          setScrolled(currentY > 10);
        }}
        onScrollEndDrag={snapHeader}
        onMomentumScrollEnd={snapHeader}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor="#000000" />
        }
      >
        {/* Hero Banner */}
        <TouchableOpacity
          activeOpacity={0.95}
          onPress={() => featuredEvent && handleEventPress(featuredEvent)}
        >
          <View className={s.hero}>
            {(featuredEvent?.banner_image_url ?? featuredEvent?.cover_image_url) ? (
              <Image
                source={{ uri: featuredEvent!.banner_image_url ?? featuredEvent!.cover_image_url! }}
                style={sheet.absoluteFill}
                resizeMode="cover"
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
              <Text className={s.heroBadge}>✦ {featuredEvent?.category ?? 'Featured'}</Text>
              <Text className={s.heroTitle} numberOfLines={2}>
                {featuredEvent?.title.toUpperCase() ?? 'DISCOVER\nTOP EVENTS'}
              </Text>
              <Text className={s.heroSub} numberOfLines={3}>
                {featuredEvent?.description ?? 'Find and book the best events happening near you.'}
              </Text>
              <View className={s.heroActions}>
                <TouchableOpacity
                  className={s.btnPrimary}
                  onPress={() => featuredEvent && handleEventPress(featuredEvent)}
                >
                  <Text className={s.btnPrimaryText}>🎟 GET TICKETS</Text>
                </TouchableOpacity>
                <TouchableOpacity
                  className={s.btnOutline}
                  onPress={() => featuredEvent && handleEventPress(featuredEvent)}
                >
                  <Text className={s.btnOutlineText}>VIEW DETAILS</Text>
                </TouchableOpacity>
              </View>
            </View>
          </View>
        </TouchableOpacity>

        {/* Category Filter */}
        <View className={s.filterRow}>
  {activeCategory !== 'All' && (
      <TouchableOpacity
        className={s.filterClearBtn}
        style={{ borderColor: colors.dividerColor }}
        onPress={() => setActiveCategory('All')}
      >
        <Text className={s.filterClearText} style={{ color: colors.text }}>✕</Text>
      </TouchableOpacity>
    )}
    <TouchableOpacity
      className={`${s.filterChip} ${activeCategory !== 'All' ? s.filterChipActive : ''}`}
      style={activeCategory === 'All' ? { borderColor: colors.dividerColor, backgroundColor: colors.sectionCardBg } : undefined}
      onPress={() => setDropdownOpen(true)}
    >
      <Text
        className={`${s.filterChipText} ${activeCategory !== 'All' ? s.filterChipTextActive : ''}`}
        style={activeCategory === 'All' ? { color: colors.subText } : undefined}
      >
        {activeCategory === 'All' ? 'CATEGORIES' : activeCategory.toUpperCase()}
      </Text>
      <Text
        className={`${s.filterChipArrow} ${activeCategory !== 'All' ? s.filterChipTextActive : ''}`}
        style={activeCategory === 'All' ? { color: colors.subText } : undefined}
      > ▾</Text>
    </TouchableOpacity>
  </View>

       

        {/* Category Dropdown Modal */}
        <Modal visible={dropdownOpen} transparent animationType="fade" onRequestClose={() => setDropdownOpen(false)}>
          <View className={s.modalBackdrop}>
            <View className={s.modalSheet} style={{ backgroundColor: colors.cardBg, borderColor: colors.dividerColor }}>
              <FlatList
                data={categories}
                keyExtractor={(item) => item}
                renderItem={({ item }) => (
                  <TouchableOpacity className={s.modalItem} onPress={() => handleSelectCategory(item)}>
                    <Text
                      className={`${s.modalItemText} ${activeCategory === item ? s.modalItemTextActive : ''}`}
                      style={{ color: activeCategory === item ? colors.text : colors.subText }}
                      >
                      {item}
                    </Text>
                  </TouchableOpacity>
                )}
              />
              <TouchableOpacity className={s.modalCloseBtn} style={{ backgroundColor: colors.sectionCardBg }} onPress={() => setDropdownOpen(false)}>
             <Text className={s.modalCloseBtnText} style={{ color: colors.text }}>✕</Text>
                <Text className={s.modalCloseBtnText}>✕</Text>
              </TouchableOpacity>
            </View>
          </View>
        </Modal>

        {/* Error banner */}
        {error && (
          <View style={{
            margin: 20,
            backgroundColor: 'rgba(255,60,110,0.12)',
            borderWidth: 1,
            borderColor: colors.dividerColor,
            borderRadius: 3,
            padding: 14,
          }}>
            <Text style={{ color: colors.text, fontWeight: '800', fontSize: 12, marginBottom: 4, letterSpacing: 1 }}>
              ⚠ COULD NOT LOAD SECTIONS
            </Text>
            <Text style={{ color: colors.subText, fontSize: 11 }}>{error}</Text>
            <TouchableOpacity
              onPress={onRefresh}
              style={{
                marginTop: 10,
                alignSelf: 'flex-start',
                borderWidth: 1,
                borderColor: '#0a0a0a',
                paddingHorizontal: 14,
                paddingVertical: 6,
                borderRadius: 2,
              }}
            >
              <Text style={{ color: '#0a0a0a', fontWeight: '800', fontSize: 10, letterSpacing: 1.5 }}>RETRY</Text>
            </TouchableOpacity>
          </View>
        )}

        {/* Dynamic Feed Sections */}
        {loading ? (
          <View className={s.loadingRow}>
           <ActivityIndicator size="small" color={colors.text} />
          </View>
        ) : (
          feedSections.map((section, index) => (
            <SectionRow
              key={section.id}
              section={section}
              isTop10={section.id === 'trending'}
              marginTop={true}
              onEventPress={handleEventPress}
              colors={colors}
            />
          ))
        )}

        {/* Dynamic Spotlight Section */}
        {!loading && sportsBannerEvent && (
          <>
            <View style={{ height: 1, backgroundColor: colors.dividerColor, marginVertical: 4 }} />
            <View className={s.sectionMt}>
              <View className={s.sectionHeader}>
                <Text className={s.sectionTitle} style={{ color: spotlightConfig.accentColor }}>{spotlightConfig.title}</Text>
                <TouchableOpacity><Text className={s.sectionLink} style={{ color: spotlightConfig.accentColor }}>EXPLORE</Text></TouchableOpacity>
              </View>
            </View>
            <TouchableOpacity
              activeOpacity={0.92}
              onPress={() => handleEventPress(sportsBannerEvent)}
            >
              <View className={s.sportsBanner} style={{ backgroundColor: spotlightConfig.bgColor }}>
                {(sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url) ? (
                  <Image
                    source={{ uri: sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url! }}
                    style={sheet.absoluteFill}
                    resizeMode="cover"
                  />
                ) : null}
                {/* Tinted gradient — colour comes from the active spotlight config */}
                <LinearGradient
                  colors={[spotlightConfig.gradientLight, spotlightConfig.gradientDark]}
                  style={sheet.absoluteFill}
                />
                {/* Accent colour strip at the bottom edge */}
                <View style={{
                  position: 'absolute', bottom: 0, left: 0, right: 0,
                  height: 3, backgroundColor: spotlightConfig.accentColor, opacity: 0.85,
                }} />
                <View className={s.sportsBannerContent}>
                  <Text className={s.sportsBadge} style={{ color: spotlightConfig.accentColor }}>
                    {spotlightConfig.badge}
                  </Text>
                  <Text className={s.sportsTitle}>{sportsBannerEvent.title.toUpperCase()}</Text>
                  <Text className={s.sportsSub} numberOfLines={2}>{sportsBannerEvent.description}</Text>
                  <TouchableOpacity
                    className={s.btnOutline}
                    onPress={() => handleEventPress(sportsBannerEvent)}
                  >
                    <Text className={s.btnOutlineText}>VIEW TICKETS</Text>
                  </TouchableOpacity>
                </View>
              </View>
            </TouchableOpacity>
            {sportsListEvents.length > 0 && (
              <View className={s.sportsListContainer}>
                {sportsListEvents.map((sport, index) => (
                  <TouchableOpacity
                    key={sport.id}
                    className={index === sportsListEvents.length - 1 ? s.sportRowLast : s.sportRow}
                    onPress={() => handleEventPress(sport)}
                  >
                    <View className={s.sportThumb}>
                      {sport.cover_image_url && (
                        <Image source={{ uri: sport.cover_image_url }} style={sheet.absoluteFill} resizeMode="cover" />
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

        {/* ── Free Events ─────────────────────────────────────────── */}
        {!loading && (() => {
          const freeEvents = allEvents.filter((e) => e.price === 0);
          if (!freeEvents.length) return null;
          return (
            <>
              <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginVertical: 4 }} />
              <View className={s.sectionMt}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>FREE EVENTS</Text>
                  <TouchableOpacity><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity>
                </View>
              </View>
              <FlatList
                horizontal
                data={freeEvents}
                keyExtractor={(e) => e.id}
                showsHorizontalScrollIndicator={false}
                contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }}
                renderItem={({ item }) => (
                  <EventCard item={item} onPress={() => handleEventPress(item)} colors={colors} />
                )}
              />
            </>
          );
        })()}

        {/* ── Premium & Exclusive ──────────────────────────────────── */}
        {!loading && (() => {
          const premiumEvents = allEvents.filter((e) => e.price >= 3000).slice(0, 10);
          if (!premiumEvents.length) return null;
          return (
            <>
              <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginVertical: 4 }} />
              <View className={s.sectionMt}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>PREMIUM & EXCLUSIVE</Text>
                  <TouchableOpacity><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity>
                </View>
              </View>
              <FlatList
                horizontal
                data={premiumEvents}
                keyExtractor={(e) => e.id}
                showsHorizontalScrollIndicator={false}
                contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }}
                renderItem={({ item }) => (
                  <EventCard item={item} onPress={() => handleEventPress(item)} colors={colors} />
                )}
              />
            </>
          );
        })()}

        {/* ── Coming Up (15–45 days out) ───────────────────────────── */}
        {!loading && (() => {
          const now = Date.now();
          const comingUp = allEvents
            .filter((e) => {
              const ms = new Date(e.start_time).getTime() - now;
              const days = ms / 86400000;
              return days >= 15 && days <= 45;
            })
            .slice(0, 10);
          if (!comingUp.length) return null;
          return (
            <>
              <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginVertical: 4 }} />
              <View className={s.sectionMt}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>COMING NEXT MONTH</Text>
                  <TouchableOpacity><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity>
                </View>
              </View>
              <FlatList
                horizontal
                data={comingUp}
                keyExtractor={(e) => e.id}
                showsHorizontalScrollIndicator={false}
                contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }}
                renderItem={({ item }) => (
                 <EventCard item={item} onPress={() => handleEventPress(item)} colors={colors} />
                )}
              />
            </>
          );
        })()}

        {/* ── Around the Country ───────────────────────────────────── */}
        {!loading && (() => {
          const otherCities = allEvents
            .filter((e) => !['Johannesburg','johannesburg'].includes(e.city ?? ''))
            .slice(0, 10);
          if (!otherCities.length) return null;
          return (
            <>
              <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginVertical: 4 }} />
              <View className={s.sectionMt}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>AROUND THE COUNTRY</Text>
                  <TouchableOpacity><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity>
                </View>
              </View>
              <FlatList
                horizontal
                data={otherCities}
                keyExtractor={(e) => e.id}
                showsHorizontalScrollIndicator={false}
                contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }}
                renderItem={({ item }) => (
                  <EventCard item={item} onPress={() => handleEventPress(item)} colors={colors} />
                )}
              />
            </>
          );
        })()}

        {/* ── Music & Concerts ─────────────────────────────────────── */}
        {!loading && (() => {
          const musicEvents = allEvents
            .filter((e) => e.category?.toLowerCase().includes('concert') || e.category?.toLowerCase().includes('music'))
            .slice(0, 10);
          if (!musicEvents.length) return null;
          return (
            <>
              <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginVertical: 4 }} />
              <View className={s.sectionMt}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>MUSIC & CONCERTS</Text>
                  <TouchableOpacity><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity>
                </View>
              </View>
              <FlatList
                horizontal
                data={musicEvents}
                keyExtractor={(e) => e.id}
                showsHorizontalScrollIndicator={false}
                contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }}
                renderItem={({ item }) => <EventCard item={item} onPress={() => handleEventPress(item)} colors={colors} />}
              />
            </>
          );
        })()}

        {/* ── Gala & Black Tie ─────────────────────────────────────── */}
        {!loading && (() => {
          const galaEvents = allEvents
            .filter((e) => e.category?.toLowerCase().includes('gala') || e.category?.toLowerCase().includes('exclusive'))
            .slice(0, 10);
          if (!galaEvents.length) return null;
          return (
            <>
              <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginVertical: 4 }} />
              <View className={s.sectionMt}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>GALA & BLACK TIE</Text>
                  <TouchableOpacity><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity>
                </View>
              </View>
              <FlatList
                horizontal
                data={galaEvents}
                keyExtractor={(e) => e.id}
                showsHorizontalScrollIndicator={false}
                contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }}
                renderItem={({ item }) => <EventCard item={item} onPress={() => handleEventPress(item)} colors={colors} />}
              />
            </>
          );
        })()}

        {/* ── Tech & Business ──────────────────────────────────────── */}
        {!loading && (() => {
          const techEvents = allEvents
            .filter((e) => e.category?.toLowerCase().includes('tech'))
            .slice(0, 10);
          if (!techEvents.length) return null;
          return (
            <>
              <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginVertical: 4 }} />
              <View className={s.sectionMt}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>TECH & BUSINESS</Text>
                  <TouchableOpacity><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity>
                </View>
              </View>
              <FlatList
                horizontal
                data={techEvents}
                keyExtractor={(e) => e.id}
                showsHorizontalScrollIndicator={false}
                contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }}
                renderItem={({ item }) => <EventCard item={item} onPress={() => handleEventPress(item)} colors={colors} />}
              />
            </>
          );
        })()}

        {/* ── Food & Lifestyle ─────────────────────────────────────── */}
        {!loading && (() => {
          const foodEvents = allEvents
            .filter((e) => e.category?.toLowerCase().includes('food'))
            .slice(0, 10);
          if (!foodEvents.length) return null;
          return (
            <>
              <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginVertical: 4 }} />
              <View className={s.sectionMt}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>FOOD & LIFESTYLE</Text>
                  <TouchableOpacity><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity>
                </View>
              </View>
              <FlatList
                horizontal
                data={foodEvents}
                keyExtractor={(e) => e.id}
                showsHorizontalScrollIndicator={false}
                contentContainerStyle={{ paddingHorizontal: 16, gap: 12, paddingBottom: 4 }}
                renderItem={({ item }) => <EventCard item={item} onPress={() => handleEventPress(item)} colors={colors} />}
              />
            </>
          );
        })()}

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
      <FilterModal
        visible={filterOpen}
        onClose={() => setFilterOpen(false)}
        initialParams={filterParams}
        onApply={handleFilterApply}
      />
    </View>
  );
}