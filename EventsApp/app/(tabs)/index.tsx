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
} from 'react-native';
import { useState, useEffect, useCallback, useRef } from 'react';
import { useRouter } from 'expo-router';
import { LinearGradient } from 'expo-linear-gradient';
import { getHomeSections, loadMoreSectionEvents, type Event, type HomeSection } from '../../lib/api';
import { useEventStore } from '../../store/eventStore';
import { s, sheet } from '../../styles/home';
import FilterModal, { FilterParams, buildFilterQueryString,formatDateRangeLabel } from '../../components/FilterModal';


const CATEGORIES = ['All', 'Concerts', 'Sports', 'Gala', 'Tech', 'Food'];
const HEADER_HEIGHT = 120;
const BASE_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';

// ─── Helpers ─────────────────────────────────────────────────────────────────

function formatPrice(price: number, currency: string) {
  if (price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

// ─── Cards ────────────────────────────────────────────────────────────────────

const EventCard = ({ item, onPress }: { item: Event; onPress: () => void }) => (
  <TouchableOpacity className={s.hCard} onPress={onPress}>
    <View className={s.hCardImageArea}>
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
      <Text className={s.hCardCat} numberOfLines={1}>{item.category}</Text>
      <Text className={s.hCardTitle} numberOfLines={2}>{item.title}</Text>
      <Text className={s.hCardMeta} numberOfLines={1}>📍 {item.venue_name}</Text>
    </View>
  </TouchableOpacity>
);

const Top10Card = ({ item, rank, onPress }: { item: Event; rank: number; onPress: () => void }) => (
  <TouchableOpacity className={s.hCard} onPress={onPress}>
    <View className={s.hCardImageArea}>
      {item.cover_image_url && (
        <Image source={{ uri: item.cover_image_url }} style={sheet.absoluteFill} resizeMode="cover" />
      )}
      <View className={s.rankBadge}>
        <Text className={s.rankText}>{rank}</Text>
      </View>
      <View className={s.hCardPriceBadge}>
        <Text className={s.hCardPriceBadgeText}>{formatPrice(item.price, item.currency)}</Text>
      </View>
    </View>
    <View className={s.hCardBody}>
      <Text className={s.hCardCat} numberOfLines={1}>{item.category}</Text>
      <Text className={s.hCardTitle} numberOfLines={2}>{item.title}</Text>
      <Text className={s.hCardMeta} numberOfLines={1}>📍 {item.venue_name}</Text>
    </View>
  </TouchableOpacity>
);

// ─── Section row (paginated) ──────────────────────────────────────────────────
//
// Converts the horizontal ScrollView into a FlatList so we can call
// loadMoreSectionEvents when the user scrolls near the end.

const SectionRow = ({
  section,
  isTop10 = false,
  marginTop = false,
  onEventPress,
}: {
  section: HomeSection;
  isTop10?: boolean;
  marginTop?: boolean;
  onEventPress: (event: Event) => void;
}) => {
  const [events,     setEvents]     = useState<Event[]>(section.events ?? []);
  const [nextCursor, setNextCursor] = useState<string>(section.next_cursor ?? '');
  const [loadingMore, setLoadingMore] = useState(false);

  // Re-sync when the parent section refreshes (e.g. pull-to-refresh)
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
      setNextCursor(''); // nothing left to load
    }
    setLoadingMore(false);
  }, [nextCursor, loadingMore, section.id]);

  if (!events.length) return null;

  return (
    <>
      {marginTop && (
        <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginVertical: 4 }} />
      )}
      <View className={marginTop ? s.sectionMt : s.section}>
        <View className={s.sectionHeader}>
          <Text className={s.sectionTitle}>{section.title.toUpperCase()}</Text>
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
              <ActivityIndicator size="small" color="#ff3c6e" />
            </View>
          ) : null
        }
        renderItem={({ item, index }) =>
          isTop10
            ? <Top10Card item={item} rank={index + 1} onPress={() => onEventPress(item)} />
            : <EventCard item={item} onPress={() => onEventPress(item)} />
        }
      />
    </>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function Index() {
  const router = useRouter();
  const setSelectedEvent = useEventStore((state) => state.setSelectedEvent);

  // ── Refs ─────────────────────────────────────────────────────────────────
  const lastScrollY  = useRef(0);
  const headerOffset = useRef(0);
  const headerAnim   = useRef(new Animated.Value(0)).current;

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

  const sportsEvents = sections
    .filter((sec) => sec.id !== 'featured')
    .flatMap((sec) => sec.events ?? [])
    .filter((e) => e.category?.toLowerCase() === 'sports')
    .filter((e, i, arr) => arr.findIndex((x) => x.id === e.id) === i)
    .slice(0, 4);

  const sportsBannerEvent = sportsEvents[0] ?? null;
  const sportsListEvents  = sportsEvents.slice(1);
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
    setActiveCategory(cat);
    setDropdownOpen(false);
  };

  const handleEventPress = (event: Event) => {
    setSelectedEvent(event);
    router.push(`/event/${event.id}`);
  };

  // ── Filter apply ─────────────────────────────────────────────────────────
  const handleFilterApply = async (params: FilterParams) => {
    setFilterParams(params);
    setFilterOpen(false);

    // Always snap the header back into view so the updated city / date label is visible
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
      const res = await fetch(`${BASE_URL}/v1/events/filter?${qs}`);
      if (!res.ok) throw new Error('Filter request failed');
      const data = await res.json();
      // data.data is the filtered events array — wrap it into a section for display
      if (data.data) {
        setSections([{
          id: 'filtered', title: 'Results', events: data.data,
          next_cursor: ''
        }]);
      }
    } catch (err: any) {
      // If the filter endpoint isn't ready yet, fall back to the full home feed
      await fetchSections();
    } finally {
      setLoading(false);
    }
  };

  // ── Location / date label for header ────────────────────────────────────
  const locationLabel =
    filterParams.city ?? (filterParams.lat ? 'Near me' : 'Near me');

  const dateLabelText =
    filterParams.date === 'any'      ? 'All dates'    :
    filterParams.date === 'tomorrow' ? 'Tomorrow'     :
    filterParams.date === 'weekend'  ? 'This Weekend' :
    filterParams.date === 'custom' && filterParams.date_from && filterParams.date_to
      ? formatDateRangeLabel(filterParams.date_from, filterParams.date_to)
      : 'All dates';

  // ── Snap helper ──────────────────────────────────────────────────────────
  const snapHeader = () => {
    const target = headerOffset.current < -(HEADER_HEIGHT / 2) ? -HEADER_HEIGHT : 0;
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
    <View className={s.screen}>

      {/* Animated Header */}
      <Animated.View
        style={{ transform: [{ translateY: headerAnim }] }}
        className={s.locationHeader}
      >
        {/* Tapping the location/date row opens the filter sheet */}
        <TouchableOpacity onPress={() => setFilterOpen(true)}>
          <Text className={s.locationCity}>{locationLabel}</Text>
          <Text className={s.locationSub}>{dateLabelText}</Text>
        </TouchableOpacity>
        <TouchableOpacity className={s.menuBtn}>
          <Text className={s.menuIcon}>🛒</Text>
        </TouchableOpacity>
      </Animated.View>

      <ScrollView
        className={s.scroll}
        showsVerticalScrollIndicator={false}
        scrollEventThrottle={16}
        onScroll={(e) => {
          const currentY = e.nativeEvent.contentOffset.y;
          const delta    = currentY - lastScrollY.current;
          lastScrollY.current = currentY;

          headerOffset.current = Math.min(
            0,
            Math.max(-HEADER_HEIGHT, headerOffset.current - delta)
          );
          headerAnim.setValue(headerOffset.current);
          setScrolled(currentY > 10);
        }}
        onScrollEndDrag={snapHeader}
        onMomentumScrollEnd={snapHeader}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor="#ff3c6e" />
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
            <TouchableOpacity className={s.filterClearBtn} onPress={() => setActiveCategory('All')}>
              <Text className={s.filterClearText}>✕</Text>
            </TouchableOpacity>
          )}
          <TouchableOpacity
            className={`${s.filterChip} ${activeCategory !== 'All' ? s.filterChipActive : ''}`}
            onPress={() => setDropdownOpen(true)}
          >
            <Text className={`${s.filterChipText} ${activeCategory !== 'All' ? s.filterChipTextActive : ''}`}>
              {activeCategory === 'All' ? 'CATEGORIES' : activeCategory.toUpperCase()}
            </Text>
            <Text className={`${s.filterChipArrow} ${activeCategory !== 'All' ? s.filterChipTextActive : ''}`}> ▾</Text>
          </TouchableOpacity>
        </View>

        {/* Thin divider below filter */}
        <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginBottom: 4 }} />

        {/* Category Dropdown Modal */}
        <Modal visible={dropdownOpen} transparent animationType="fade" onRequestClose={() => setDropdownOpen(false)}>
          <View className={s.modalBackdrop}>
            <View className={s.modalSheet}>
              <FlatList
                data={CATEGORIES}
                keyExtractor={(item) => item}
                renderItem={({ item }) => (
                  <TouchableOpacity className={s.modalItem} onPress={() => handleSelectCategory(item)}>
                    <Text className={`${s.modalItemText} ${activeCategory === item ? s.modalItemTextActive : ''}`}>
                      {item}
                    </Text>
                  </TouchableOpacity>
                )}
              />
              <TouchableOpacity className={s.modalCloseBtn} onPress={() => setDropdownOpen(false)}>
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
            borderColor: '#ff3c6e',
            borderRadius: 3,
            padding: 14,
          }}>
            <Text style={{ color: '#ff3c6e', fontWeight: '800', fontSize: 12, marginBottom: 4, letterSpacing: 1 }}>
              ⚠ COULD NOT LOAD SECTIONS
            </Text>
            <Text style={{ color: 'rgba(255,255,255,0.5)', fontSize: 11 }}>{error}</Text>
            <TouchableOpacity
              onPress={onRefresh}
              style={{
                marginTop: 10,
                alignSelf: 'flex-start',
                borderWidth: 1,
                borderColor: '#ff3c6e',
                paddingHorizontal: 14,
                paddingVertical: 6,
                borderRadius: 2,
              }}
            >
              <Text style={{ color: '#ff3c6e', fontWeight: '800', fontSize: 10, letterSpacing: 1.5 }}>RETRY</Text>
            </TouchableOpacity>
          </View>
        )}

        {/* Dynamic Feed Sections */}
        {loading ? (
          <View className={s.loadingRow}>
            <ActivityIndicator size="small" color="#ff3c6e" />
          </View>
        ) : (
          feedSections.map((section, index) => (
            <SectionRow
              key={section.id}
              section={section}
              isTop10={section.id === 'trending'}
              marginTop={index > 0}
              onEventPress={handleEventPress}
            />
          ))
        )}

        {/* Sports Elite */}
        {!loading && sportsBannerEvent && (
          <>
            <View style={{ height: 1, backgroundColor: 'rgba(255,255,255,0.06)', marginVertical: 4 }} />
            <View className={s.sectionMt}>
              <View className={s.sectionHeader}>
                <Text className={s.sectionTitle}>SPORTS ELITE</Text>
                <TouchableOpacity><Text className={s.sectionLink}>EXPLORE</Text></TouchableOpacity>
              </View>
            </View>
            <TouchableOpacity
              activeOpacity={0.92}
              onPress={() => handleEventPress(sportsBannerEvent)}
            >
              <View className={s.sportsBanner}>
                {(sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url) ? (
                  <Image
                    source={{ uri: sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url! }}
                    style={sheet.absoluteFill}
                    resizeMode="cover"
                  />
                ) : null}
                <View className={s.sportsBannerBg} />
                <LinearGradient
                  colors={['rgba(10,10,10,0.3)', 'rgba(10,10,10,0.75)']}
                  style={sheet.absoluteFill}
                />
                <View className={s.sportsBannerContent}>
                  <Text className={s.sportsBadge}>{sportsBannerEvent.category}</Text>
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
                      <Text className={s.sportCat}>{sport.category}</Text>
                      <Text className={s.sportTitle}>{sport.title}</Text>
                      <Text className={s.sportLoc}>{sport.city}</Text>
                    </View>
                    <Text className={s.sportArrow}>›</Text>
                  </TouchableOpacity>
                ))}
              </View>
            )}
          </>
        )}

        <View style={{ height: 100 }} />
      </ScrollView>

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