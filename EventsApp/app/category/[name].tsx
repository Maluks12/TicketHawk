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
import { useMemo } from 'react';
import { LinearGradient } from 'expo-linear-gradient';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useEventStore } from '../../store/eventStore';
import { useAppTheme } from '../../theme/colors';
import type { Event } from '../../lib/api';

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

// ─── Category Event Card ───────────────────────────────────────────────────────

function CategoryEventCard({ item, onPress }: { item: Event; onPress: () => void }) {
  return (
    <TouchableOpacity onPress={onPress} style={styles.card} activeOpacity={0.88}>

      {/* Image — only TOP corners rounded, matching home-page card image style */}
      <View style={styles.cardImageWrap}>
        {item.cover_image_url ? (
          <Image
            source={{ uri: item.cover_image_url }}
            style={StyleSheet.absoluteFillObject}
            resizeMode="cover"
          />
        ) : (
          <View style={[StyleSheet.absoluteFillObject, styles.cardImagePlaceholder]} />
        )}

        <LinearGradient
          colors={['transparent', 'rgba(0,0,0,0.55)']}
          style={styles.cardGradient}
        />

        <View style={styles.priceBadge}>
          <Text style={styles.priceBadgeText}>{formatPrice(item.price)}</Text>
        </View>
      </View>

      {/* Text body */}
      <View style={styles.cardBody}>
        <Text style={styles.cardTitle} numberOfLines={2}>{item.title}</Text>
        <Text style={styles.cardMeta} numberOfLines={1}>
          {formatCardDate(item.start_time)}
          {item.start_time ? `  ·  ${formatTime(item.start_time)}` : ''}
        </Text>
        {item.venue_name ? (
          <Text style={styles.cardVenue} numberOfLines={1}>
            {item.venue_name}{item.city ? `, ${item.city}` : ''}
          </Text>
        ) : null}
      </View>
    </TouchableOpacity>
  );
}

// ─── Empty state ──────────────────────────────────────────────────────────────

function EmptyState({ category }: { category: string }) {
  return (
    <View style={styles.emptyWrap}>
      <Text style={styles.emptyIcon}>🎟</Text>
      <Text style={styles.emptyTitle}>No events found</Text>
      <Text style={styles.emptySub}>
        There are no {category.toLowerCase()} events right now. Check back soon.
      </Text>
    </View>
  );
}

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function CategoryScreen() {
  const { name } = useLocalSearchParams<{ name: string }>();
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { colors } = useAppTheme();
  const { allEvents, setSelectedEvent } = useEventStore();

  const categoryLabel = name
    ? decodeURIComponent(name).replace(/\b\w/g, (c) => c.toUpperCase())
    : 'Events';

  // Client-side filter — instant, no extra API call.
  // Case-insensitive substring match so "Sports", "sports", "SPORTS" all match.
  const filtered = useMemo(() => {
    if (!name) return allEvents;
    const needle = decodeURIComponent(name).toLowerCase();
    return allEvents.filter((e) => e.category?.toLowerCase().includes(needle));
  }, [allEvents, name]);

  const handleEventPress = (event: Event) => {
    setSelectedEvent(event);
    router.push(`/event/${event.id}`);
  };

  return (
    <View style={[styles.screen, { backgroundColor: colors.background }]}>
      <StatusBar barStyle="dark-content" />

      {/* ── Header ─────────────────────────────────────────────────────────── */}
      <View style={[styles.header, { paddingTop: insets.top + 8, backgroundColor: colors.background }]}>
        <TouchableOpacity style={styles.backBtn} onPress={() => router.back()} hitSlop={12}>
          <Text style={[styles.backIcon, { color: colors.text }]}>‹</Text>
        </TouchableOpacity>

        <View style={styles.headerCenter}>
          <Text style={[styles.headerTitle, { color: colors.text }]}>{categoryLabel}</Text>
          <Text style={[styles.headerSub, { color: colors.subText }]}>
            {filtered.length} event{filtered.length !== 1 ? 's' : ''}
          </Text>
        </View>

        {/* Spacer to keep title centred */}
        <View style={styles.backBtn} />
      </View>

      <View style={{ height: 1, backgroundColor: colors.dividerColor }} />

      {/* ── Content ────────────────────────────────────────────────────────── */}
      {allEvents.length === 0 ? (
        <View style={styles.loadingWrap}>
          <ActivityIndicator size="large" color="#0a0a0a" />
          <Text style={[styles.loadingText, { color: colors.subText }]}>Loading events…</Text>
        </View>
      ) : (
        <FlatList
          data={filtered}
          keyExtractor={(e) => e.id}
          contentContainerStyle={[styles.listContent, { paddingBottom: insets.bottom + 24 }]}
          showsVerticalScrollIndicator={false}
          ListEmptyComponent={<EmptyState category={categoryLabel} />}
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
  backBtn: {
    width: 40,
    alignItems: 'center',
    justifyContent: 'center',
  },
  backIcon: {
    fontSize: 32,
    fontWeight: '300',
    lineHeight: 36,
    marginTop: -2,
  },
  headerCenter: {
    flex: 1,
    alignItems: 'center',
  },
  headerTitle: {
    fontSize: 17,
    fontWeight: '700',
    letterSpacing: 0.2,
  },
  headerSub: {
    fontSize: 12,
    fontWeight: '400',
    marginTop: 1,
  },

  listContent: {
    paddingHorizontal: 16,
    paddingTop: 16,
    gap: 16,
  },

  // Outer card — NO overflow:hidden so bottom corners are handled by borderRadius alone
  card: {
    backgroundColor: '#ffffff',
    borderRadius: CARD_RADIUS,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.08,
    shadowRadius: 8,
    elevation: 3,
  },

  // Image wrap — overflow:hidden only here, only top corners rounded
  cardImageWrap: {
    width: '100%',
    height: CARD_IMAGE_H,
    backgroundColor: '#e5e5e5',
    borderTopLeftRadius: CARD_RADIUS,
    borderTopRightRadius: CARD_RADIUS,
    overflow: 'hidden',
  },
  cardImagePlaceholder: {
    backgroundColor: '#d0d0d0',
  },
  cardGradient: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    height: CARD_IMAGE_H * 0.45,
  },
  priceBadge: {
    position: 'absolute',
    bottom: 12,
    left: 12,
    backgroundColor: 'rgba(0,0,0,0.65)',
    borderRadius: 6,
    paddingHorizontal: 10,
    paddingVertical: 5,
  },
  priceBadgeText: {
    color: '#ffffff',
    fontSize: 12,
    fontWeight: '700',
    letterSpacing: 0.3,
  },

  cardBody: {
    paddingHorizontal: 14,
    paddingTop: 12,
    paddingBottom: 14,
  },
  cardTitle: {
    fontSize: 16,
    fontWeight: '700',
    color: '#0a0a0a',
    marginBottom: 5,
    lineHeight: 21,
  },
  cardMeta: {
    fontSize: 12,
    color: 'rgba(0,0,0,0.45)',
    fontWeight: '500',
    marginBottom: 2,
  },
  cardVenue: {
    fontSize: 12,
    color: 'rgba(0,0,0,0.35)',
    fontWeight: '400',
  },

  loadingWrap: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  loadingText: {
    fontSize: 13,
    marginTop: 12,
  },
  emptyWrap: {
    alignItems: 'center',
    paddingTop: 80,
    paddingHorizontal: 40,
  },
  emptyIcon:  { fontSize: 40, marginBottom: 16 },
  emptyTitle: { fontSize: 18, fontWeight: '700', color: '#0a0a0a', marginBottom: 8 },
  emptySub:   { fontSize: 13, color: 'rgba(0,0,0,0.4)', textAlign: 'center', lineHeight: 19 },
});