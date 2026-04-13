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
} from 'react-native';
import { useState, useEffect, useCallback } from 'react';
import { LinearGradient } from 'expo-linear-gradient';
import { getHomeSections, type Event, type HomeSection } from '../lib/api';
import { s, sheet } from '../styles/home';

const CATEGORIES = ['All', 'Concerts', 'Sports', 'Gala', 'Tech', 'Food'];

// ─── Helpers ─────────────────────────────────────────────────────────────────

function formatPrice(price: number, currency: string) {
  if (price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

// ─── Cards ────────────────────────────────────────────────────────────────────

const EventCard = ({ item }: { item: Event }) => (
  <TouchableOpacity className={s.hCard}>
    <View className={s.hCardImageArea}>
      {item.cover_image_url && (
        <Image source={{ uri: item.cover_image_url }} style={sheet.absoluteFill} resizeMode="cover" />
      )}
      {(item.category?.toLowerCase().includes('music') ||
        item.category?.toLowerCase().includes('concert')) && (
        <View className={s.hCardBadge}>
          <Text className={s.hCardBadgeText}>● LIVE</Text>
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

const Top10Card = ({ item, rank }: { item: Event; rank: number }) => (
  <TouchableOpacity className={s.hCard}>
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

// ─── Section row ──────────────────────────────────────────────────────────────

const SectionRow = ({
  section,
  isTop10 = false,
  marginTop = false,
}: {
  section: HomeSection;
  isTop10?: boolean;
  marginTop?: boolean;
}) => {
  if (!section.events?.length) return null;
  return (
    <>
      <View className={marginTop ? s.sectionMt : s.section}>
        <View className={s.sectionHeader}>
          <Text className={s.sectionTitle}>{section.title.toUpperCase()}</Text>
          <TouchableOpacity><Text className={s.sectionLink}>VIEW ALL</Text></TouchableOpacity>
        </View>
      </View>
      <ScrollView
        horizontal
        showsHorizontalScrollIndicator={false}
        contentContainerClassName={s.hScrollContent}
      >
        {section.events.map((event, index) =>
          isTop10
            ? <Top10Card key={event.id} item={event} rank={index + 1} />
            : <EventCard key={event.id} item={event} />
        )}
      </ScrollView>
    </>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function Index() {
  const [activeCategory, setActiveCategory] = useState('All');
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [location] = useState('Johannesburg, ZA');
  const [scrolled, setScrolled] = useState(false);

  const [sections, setSections]   = useState<HomeSection[]>([]);
  const [loading, setLoading]     = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError]         = useState<string | null>(null);

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

  return (
    <View className={s.screen}>

      {/* Fixed Location Header */}
      <View
        className={scrolled ? s.locationHeaderScrolled : s.locationHeader}
        style={scrolled ? sheet.locationHeaderShadow : undefined}
      >
        <View>
          <Text className={s.locationCity}>{location}</Text>
          <Text className={s.locationSub}>All dates</Text>
        </View>
        <TouchableOpacity className={s.menuBtn}>
          <Text className={s.menuIcon}>🛒</Text>
        </TouchableOpacity>
      </View>

      <ScrollView
        className={s.scroll}
        showsVerticalScrollIndicator={false}
        onScroll={(e) => setScrolled(e.nativeEvent.contentOffset.y > 10)}
        scrollEventThrottle={16}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor="#ff3c6e" />
        }
      >
        {/* Hero Banner */}
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
            colors={['rgba(0,0,47,0.75)', '#000000']}
            locations={[0.6, 1]}
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
              <TouchableOpacity className={s.btnPrimary}>
                <Text className={s.btnPrimaryText}>🎟 Get Tickets</Text>
              </TouchableOpacity>
              <TouchableOpacity className={s.btnOutline}>
                <Text className={s.btnOutlineText}>View Details</Text>
              </TouchableOpacity>
            </View>
          </View>
        </View>

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
              {activeCategory === 'All' ? 'Categories' : activeCategory}
            </Text>
            <Text className={`${s.filterChipArrow} ${activeCategory !== 'All' ? s.filterChipTextActive : ''}`}> ▾</Text>
          </TouchableOpacity>
        </View>

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

        {/* Error banner — visible so you can diagnose API issues */}
        {error && (
          <View className="mx-5 mt-4 bg-brand-pink/20 border border-brand-pink rounded-xl p-4">
            <Text className="text-brand-pink font-bold text-sm mb-1">⚠ Could not load sections</Text>
            <Text className="text-white/70 text-xs">{error}</Text>
            <TouchableOpacity onPress={onRefresh} className="mt-3 self-start bg-brand-pink px-4 py-2 rounded-full">
              <Text className="text-white font-bold text-xs">Retry</Text>
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
            />
          ))
        )}

        {/* Sports Elite */}
        {!loading && sportsBannerEvent && (
          <>
            <View className={s.sectionMt}>
              <View className={s.sectionHeader}>
                <Text className={s.sectionTitle}>SPORTS ELITE</Text>
                <TouchableOpacity><Text className={s.sectionLink}>EXPLORE</Text></TouchableOpacity>
              </View>
            </View>
            <View className={s.sportsBanner}>
              {(sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url) ? (
                <Image
                  source={{ uri: sportsBannerEvent.banner_image_url ?? sportsBannerEvent.cover_image_url! }}
                  style={sheet.absoluteFill}
                  resizeMode="cover"
                />
              ) : null}
              <View className={s.sportsBannerBg} />
              <View className={s.sportsBannerContent}>
                <Text className={s.sportsBadge}>{sportsBannerEvent.category}</Text>
                <Text className={s.sportsTitle}>{sportsBannerEvent.title.toUpperCase()}</Text>
                <Text className={s.sportsSub} numberOfLines={2}>{sportsBannerEvent.description}</Text>
                <TouchableOpacity className={s.btnOutline}>
                  <Text className={s.btnOutlineText}>View Tickets</Text>
                </TouchableOpacity>
              </View>
            </View>
            {sportsListEvents.length > 0 && (
              <View className={s.sportsListContainer}>
                {sportsListEvents.map((sport, index) => (
                  <TouchableOpacity
                    key={sport.id}
                    className={index === sportsListEvents.length - 1 ? s.sportRowLast : s.sportRow}
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
    </View>
  );
}