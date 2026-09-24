import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  Image,
  ActivityIndicator,
  StatusBar,
  StyleSheet,
  Dimensions,
} from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState, useEffect } from 'react';
import { LinearGradient } from 'expo-linear-gradient';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import {
  api,
  getEventsByCategory,
  getTrendingEvents,
  getActiveEvents,
  type Event,
} from '../../lib/api';
import { useEventStore } from '../../store/eventStore';
import { useAppTheme } from '../../theme/colors';
import { CoverImage } from '../../components/CoverImage';
import { prefetchFullImages } from '../../lib/img';

const { width: SCREEN_W } = Dimensions.get('window');
const CARD_IMAGE_H = Math.round(SCREEN_W * 0.52);
const CARD_RADIUS  = 14;

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatPrice(price: number) {
  if (!price || price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function formatCardDate(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-ZA', {
    weekday: 'short', day: 'numeric', month: 'short', year: 'numeric',
  });
}

function formatTime(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

// Fetch function per named API section ID
async function fetchSectionEvents(sectionId: string): Promise<Event[]> {
  try {
    switch (sectionId) {
      case 'trending':
        return (await getTrendingEvents(100)).data.data ?? [];
      case 'happening_soon':
      case 'this_weekend':
        return (await getActiveEvents(100)).data.data ?? [];
      case 'recently_added': {
        const res = await api.get<{ data: Event[] }>('/v1/events', { params: { limit: 100 } });
        return res.data.data ?? [];
      }
      case 'featured': {
        const res = await api.get<{ data: Event[] }>('/v1/events/featured', { params: { limit: 100 } });
        return res.data.data ?? [];
      }
      default:
        return [];
    }
  } catch {
    return [];
  }
}

// Editorial filter functions — applied to the full allEvents store (no slice)
const EDITORIAL_FILTERS: Record<string, (e: Event) => boolean> = {
  free:         (e) => e.price === 0,
  premium:      (e) => e.price >= 3000,
  coming_up:    (e) => {
    const d = (new Date(e.start_time).getTime() - Date.now()) / 86400000;
    return d >= 15 && d <= 45;
  },
  other_cities: (e) => !['Johannesburg', 'johannesburg'].includes(e.city ?? ''),
  music:        (e) => !!(e.category?.toLowerCase().includes('concert') || e.category?.toLowerCase().includes('music')),
  gala:         (e) => !!(e.category?.toLowerCase().includes('gala') || e.category?.toLowerCase().includes('exclusive')),
  tech:         (e) => !!(e.category?.toLowerCase().includes('tech')),
  food:         (e) => !!(e.category?.toLowerCase().includes('food')),
};

// ─── Card ─────────────────────────────────────────────────────────────────────

function SectionEventCard({ item, onPress }: { item: Event; onPress: () => void }) {
  return (
    <TouchableOpacity onPress={onPress} style={styles.card} activeOpacity={0.88}>
      <View style={styles.cardImageWrap}>
        {item.cover_image_url ? (
          <CoverImage uri={item.cover_image_url} style={StyleSheet.absoluteFill} />
        ) : (
          <View style={[StyleSheet.absoluteFill, { backgroundColor: '#2a2a2a' }]} />
        )}
        <LinearGradient
          colors={['transparent', 'rgba(0,0,0,0.35)', 'rgba(0,0,0,0.86)']}
          locations={[0.3, 0.62, 1]}
          style={StyleSheet.absoluteFill}
        />
        <View style={styles.priceBadge}>
          <Text style={styles.priceBadgeText}>{formatPrice(item.price)}</Text>
        </View>
        <View style={styles.cardBody}>
          <Text style={styles.cardTitle} numberOfLines={2}>{item.title}</Text>
          <Text style={styles.cardMeta} numberOfLines={1}>
            {formatCardDate(item.start_time)}{item.start_time ? `  ·  ${formatTime(item.start_time)}` : ''}
          </Text>
          {item.venue_name ? (
            <Text style={styles.cardVenue} numberOfLines={1}>
              {item.venue_name}{item.city ? `, ${item.city}` : ''}
            </Text>
          ) : null}
        </View>
      </View>
    </TouchableOpacity>
  );
}

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function SectionScreen() {
  const { title }  = useLocalSearchParams<{ title: string }>();
  const router     = useRouter();
  const insets     = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();
  const { sectionMeta, allEvents, dbCategories, setSelectedEvent } = useEventStore();

  const [events,  setEvents]  = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);

  const label = title ? decodeURIComponent(title) : sectionMeta?.title ?? '';

  useEffect(() => {
    if (!sectionMeta) { setLoading(false); return; }

    const { sectionId, filterKey, events: fallbackEvents } = sectionMeta;

    // 1. Editorial sections — filter full allEvents, no slice cap
    if (filterKey) {
      const fn = EDITORIAL_FILTERS[filterKey];
      setEvents(fn ? allEvents.filter(fn) : (fallbackEvents ?? []));
      setLoading(false);
      return;
    }

    // 2. cat-* sections — fetch by matching category ID from dbCategories
    if (sectionId?.startsWith('cat-')) {
      const slug = sectionId.replace(/^cat-/, '');
      const match = dbCategories.find(
        (c) => c.name.toLowerCase().replace(/\s+/g, '-') === slug ||
               c.slug?.toLowerCase() === slug
      );
      if (match) {
        getEventsByCategory(match.id, 100)
          .then((res) => setEvents(res.data?.data ?? []))
          .catch(() => setEvents(fallbackEvents ?? []))
          .finally(() => setLoading(false));
      } else {
        setEvents(fallbackEvents ?? []);
        setLoading(false);
      }
      return;
    }

    // 3. Named API sections — fetch via dedicated typed functions
    if (sectionId) {
      fetchSectionEvents(sectionId)
        .then((data) => setEvents(data.length > 0 ? data : (fallbackEvents ?? [])))
        .finally(() => setLoading(false));
      return;
    }

    // 4. for_you / near_you / unknown — use the pre-loaded events passed in
    setEvents(fallbackEvents ?? []);
    setLoading(false);
  }, [sectionMeta, allEvents, dbCategories]);

  const handleEventPress = (event: Event) => {
    setSelectedEvent(event);
    // The detail screen renders the FULL-size banner, but only the card
    // thumbnail was warmed for this list — so start the real download now and
    // let it run under the push animation instead of beginning cold when the
    // screen lands. The home feed has always done this; these screens did not,
    // which is why arriving here felt slower than arriving from home.
    prefetchFullImages([event.banner_image_url ?? event.cover_image_url]);
    router.push(`/event/${event.id}`);
  };

  return (
    <View style={[styles.screen, { backgroundColor: colors.background }]}>
      <StatusBar barStyle={isDark ? 'light-content' : 'dark-content'} />

      {/* Header */}
      <View style={[styles.header, { paddingTop: insets.top + 8, backgroundColor: colors.background }]}>
        <TouchableOpacity style={styles.backBtn} onPress={() => router.back()} hitSlop={12}>
          <Text style={[styles.backIcon, { color: colors.text }]}>‹</Text>
        </TouchableOpacity>
        <View style={styles.headerCenter}>
          <Text style={[styles.headerTitle, { color: colors.text }]}>{label}</Text>
          <Text style={[styles.headerSub, { color: colors.subText }]}>
            {loading ? 'Loading…' : `${events.length} event${events.length !== 1 ? 's' : ''}`}
          </Text>
        </View>
        <View style={styles.backBtn} />
      </View>

      <View style={{ height: 1, backgroundColor: colors.dividerColor }} />

      {loading ? (
        <View style={styles.centreWrap}>
          <ActivityIndicator size="large" color={colors.text} />
        </View>
      ) : (
        <FlatList
          data={events}
          keyExtractor={(e) => e.id}
          contentContainerStyle={[styles.listContent, { paddingBottom: insets.bottom + 24 }]}
          showsVerticalScrollIndicator={false}
          ListEmptyComponent={
            <View style={styles.centreWrap}>
              <Text style={styles.emptyIcon}>🎟</Text>
              <Text style={[styles.emptyTitle, { color: colors.text }]}>No events found</Text>
              <Text style={[styles.emptySub, { color: colors.subText }]}>
                There are no events in this section right now.
              </Text>
            </View>
          }
          renderItem={({ item }) => (
            <SectionEventCard item={item} onPress={() => handleEventPress(item)} />
          )}
        />
      )}
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  screen: { flex: 1 },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingBottom: 12,
  },
  backBtn:      { width: 40, alignItems: 'center', justifyContent: 'center' },
  backIcon:     { fontSize: 32, fontWeight: '300', lineHeight: 36, marginTop: -2 },
  headerCenter: { flex: 1, alignItems: 'center' },
  headerTitle:  { fontSize: 17, fontWeight: '700', letterSpacing: 0.2 },
  headerSub:    { fontSize: 12, fontWeight: '400', marginTop: 1 },

  listContent: { paddingHorizontal: 16, paddingTop: 16, gap: 16 },

  card: {
    borderRadius: CARD_RADIUS,
    overflow: 'hidden',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 3 },
    shadowOpacity: 0.18,
    shadowRadius: 10,
    elevation: 5,
  },
  cardImageWrap: {
    width: '100%',
    height: CARD_IMAGE_H,
    backgroundColor: '#2a2a2a',
    borderRadius: CARD_RADIUS,
    overflow: 'hidden',
  },
  priceBadge: {
    position: 'absolute', top: 12, right: 12,
    backgroundColor: 'rgba(0,0,0,0.60)',
    borderRadius: 6, paddingHorizontal: 10, paddingVertical: 5,
  },
  priceBadgeText: { color: '#fff', fontSize: 12, fontWeight: '700', letterSpacing: 0.3 },

  cardBody: {
    position: 'absolute',
    bottom: 0, left: 0, right: 0,
    padding: 14,
    paddingBottom: 16,
  },
  cardTitle: {
    fontSize: 16,
    fontWeight: '800',
    marginBottom: 4,
    lineHeight: 21,
    color: '#ffffff',
    textShadowColor: 'rgba(0,0,0,0.55)',
    textShadowOffset: { width: 0, height: 1 },
    textShadowRadius: 4,
  },
  cardMeta: {
    fontSize: 12,
    fontWeight: '500',
    marginBottom: 2,
    color: 'rgba(255,255,255,0.82)',
    textShadowColor: 'rgba(0,0,0,0.55)',
    textShadowOffset: { width: 0, height: 1 },
    textShadowRadius: 3,
  },
  cardVenue: {
    fontSize: 12,
    fontWeight: '400',
    color: 'rgba(255,255,255,0.68)',
    textShadowColor: 'rgba(0,0,0,0.55)',
    textShadowOffset: { width: 0, height: 1 },
    textShadowRadius: 3,
  },

  centreWrap: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: 32, marginTop: 80 },
  emptyIcon:  { fontSize: 40, marginBottom: 16 },
  emptyTitle: { fontSize: 18, fontWeight: '700', marginBottom: 8, textAlign: 'center' },
  emptySub:   { fontSize: 13, textAlign: 'center', lineHeight: 19 },
});