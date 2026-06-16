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
import { useState, useEffect, useCallback } from 'react';
import { LinearGradient } from 'expo-linear-gradient';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { getEventsByCategory, type Event } from '../../lib/api';
import { useEventStore } from '../../store/eventStore';
import { useAppTheme } from '../../theme/colors';
import { CoverImage } from '../../components/CoverImage';

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

// ─── Card ─────────────────────────────────────────────────────────────────────

function CategoryEventCard({ item, onPress }: { item: Event; onPress: () => void }) {
  return (
    <TouchableOpacity onPress={onPress} style={styles.card} activeOpacity={0.88}>
      <View style={styles.cardImageWrap}>
        {item.cover_image_url ? (
          <CoverImage uri={item.cover_image_url} style={StyleSheet.absoluteFillObject} />
        ) : (
          <View style={[StyleSheet.absoluteFillObject, { backgroundColor: '#2a2a2a' }]} />
        )}
        {/* Strong bottom gradient ensures text is always readable over any image */}
        <LinearGradient
          colors={['transparent', 'rgba(0,0,0,0.35)', 'rgba(0,0,0,0.86)']}
          locations={[0.3, 0.62, 1]}
          style={StyleSheet.absoluteFillObject}
        />
        {/* Price badge — top right */}
        <View style={styles.priceBadge}>
          <Text style={styles.priceBadgeText}>{formatPrice(item.price)}</Text>
        </View>
        {/* Text overlaid at the bottom of the image */}
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

export default function CategoryScreen() {
  const { name }   = useLocalSearchParams<{ name: string }>();
  const router     = useRouter();
  const insets     = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();
  const { dbCategories, setSelectedEvent } = useEventStore();

  const [events,     setEvents]     = useState<Event[]>([]);
  const [loading,    setLoading]    = useState(true);
  const [error,      setError]      = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const decodedName    = name ? decodeURIComponent(name) : '';
  const categoryLabel  = decodedName.replace(/\b\w/g, (c) => c.toUpperCase());

  const fetchEvents = useCallback(async () => {
    if (!decodedName) return;
    setError(null);

    // Look up the numeric category ID that was fetched from GET /v1/categories
    // on app load and stored in the event store by index.tsx.
    const match = dbCategories.find(
      (c) =>
        c.name.toLowerCase() === decodedName.toLowerCase() ||
        c.slug?.toLowerCase() === decodedName.toLowerCase()
    );

    if (!match) {
      // This only happens if the user navigates directly to this screen before
      // the home page has loaded. Show a helpful error rather than silently
      // returning all events.
      setError(
        `Category "${categoryLabel}" not found. Go back to the home screen and try again.`
      );
      return;
    }

    try {
      // Uses the existing GET /v1/events/category/:id endpoint with the
      // correct numeric ID — guaranteed to return only events for this category.
      const res  = await getEventsByCategory(match.id, 50);
      const data = res.data?.data ?? [];
      setEvents(data);
    } catch (e: any) {
      setError(e?.message ?? 'Failed to load events.');
    }
  }, [decodedName, dbCategories]);

  useEffect(() => {
    setLoading(true);
    fetchEvents().finally(() => setLoading(false));
  }, [fetchEvents]);

  const onRefresh = async () => {
    setRefreshing(true);
    await fetchEvents();
    setRefreshing(false);
  };

  const handleEventPress = (event: Event) => {
    setSelectedEvent(event);
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
          <Text style={[styles.headerTitle, { color: colors.text }]}>{categoryLabel}</Text>
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
      ) : error ? (
        <View style={styles.centreWrap}>
          <Text style={styles.emptyIcon}>⚠️</Text>
          <Text style={[styles.emptyTitle, { color: colors.text }]}>Something went wrong</Text>
          <Text style={[styles.emptySub, { color: colors.subText }]}>{error}</Text>
          <TouchableOpacity style={[styles.retryBtn, { borderColor: colors.text }]} onPress={() => { setLoading(true); fetchEvents().finally(() => setLoading(false)); }}>
             <Text style={[styles.retryText, { color: colors.text }]}>RETRY</Text>
            </TouchableOpacity>
        </View>
      ) : (
        <FlatList
          data={events}
          keyExtractor={(e) => e.id}
          contentContainerStyle={[styles.listContent, { paddingBottom: insets.bottom + 24 }]}
          showsVerticalScrollIndicator={false}
          onRefresh={onRefresh}
          refreshing={refreshing}
          ListEmptyComponent={
            <View style={styles.centreWrap}>
              <Text style={styles.emptyIcon}>🎟</Text>
              <Text style={[styles.emptyTitle, { color: colors.text }]}>No events found</Text>
              <Text style={[styles.emptySub, { color: colors.subText }]}>There are no {categoryLabel.toLowerCase()} events right now. Check back soon.
              </Text>
            </View>
          }
          renderItem={({ item }) => (
            <CategoryEventCard item={item} onPress={() => handleEventPress(item)} />
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
    bottom: 0,
    left: 0,
    right: 0,
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

  centreWrap: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: 32 },
  emptyIcon:  { fontSize: 40, marginBottom: 16 },
  emptyTitle: { fontSize: 18, fontWeight: '700', marginBottom: 8, textAlign: 'center' },
  emptySub:  { fontSize: 13, textAlign: 'center', lineHeight: 19, marginBottom: 20 },
  retryBtn:  { borderWidth: 1, paddingHorizontal: 20, paddingVertical: 8, borderRadius: 2 },
  retryText: { fontSize: 11, fontWeight: '800', letterSpacing: 1.5 },
});