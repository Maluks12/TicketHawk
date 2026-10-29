import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  ActivityIndicator,
  StatusBar,
  StyleSheet,
  Dimensions,
  ScrollView,
} from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState, useEffect, useCallback, useMemo } from 'react';
import { LinearGradient } from 'expo-linear-gradient';
import { Image as ExpoImage } from 'expo-image';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Ionicons } from '@expo/vector-icons';
import { CheckCircle } from 'phosphor-react-native';
import { IMG_PLACEHOLDER, prefetchFullImages } from '../../lib/img';
import { getEventCollection, type Event, type PublicEventCollection } from '../../lib/api';
import { collectionDateRange } from '../../lib/eventCollections';
import { useEventStore } from '../../store/eventStore';
import { useAppTheme } from '../../theme/colors';
import { fmtRand } from '../../lib/money';

/**
 * Event collection — where a collection card on the home feed leads.
 *
 * The collection's cover, title and description, the organiser, then every
 * upcoming public date as a list in date order: the shape of a tour poster's
 * calendar, which is what an organiser grouping a tour is reproducing. Each
 * date opens that event's own screen, where tickets are bought — a collection
 * is navigation, never a checkout of its own.
 *
 * Mirrors /collections/[slug] on web-desktop and web-mobile.
 */

const { width: SCREEN_W } = Dimensions.get('window');

function formatPrice(price: number) {
  if (!price || price === 0) return 'Free';
  return fmtRand(price);
}
const dayNum  = (iso: string) => new Date(iso).toLocaleDateString('en-ZA', { day: 'numeric' });
const monthAb = (iso: string) => new Date(iso).toLocaleDateString('en-ZA', { month: 'short' }).toUpperCase();
const weekday = (iso: string) => new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short' });
const timeOf  = (iso: string) => new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });

export default function CollectionScreen() {
  const { slug } = useLocalSearchParams<{ slug: string }>();
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();
  const { setSelectedEvent } = useEventStore();

  const [collection, setCollection] = useState<PublicEventCollection | null>(null);
  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [city, setCity] = useState<string>('all');

  const load = useCallback(async () => {
    if (!slug) return;
    try {
      const { data } = await getEventCollection(slug);
      setCollection(data);
    } catch {
      setNotFound(true);
    } finally {
      setLoading(false);
    }
  }, [slug]);

  useEffect(() => { load(); }, [load]);

  const events = useMemo(() => collection?.events ?? [], [collection]);

  // Cities in date order. The filter only shows when there are enough to be
  // worth filtering — two cities read at a glance.
  const cities = useMemo(() => {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const e of events) {
      const c = (e.city ?? '').trim();
      if (c && !seen.has(c.toLowerCase())) { seen.add(c.toLowerCase()); out.push(c); }
    }
    return out;
  }, [events]);

  const shown = city === 'all' ? events : events.filter((e) => (e.city ?? '').trim() === city);
  const prices = events.map((e) => e.price ?? 0);
  const minPrice = prices.length ? Math.min(...prices) : 0;
  const maxPrice = prices.length ? Math.max(...prices) : 0;
  // "R80", "Free", or a range: "Free – R150", "R80 – R150".
  const priceLabel = minPrice === maxPrice
    ? formatPrice(minPrice)
    : `${formatPrice(minPrice)} – ${formatPrice(maxPrice)}`;
  const subtle = isDark ? '#1c1c1c' : '#f0f0f4';

  function openEvent(e: Event) {
    setSelectedEvent(e);
    // Warm the full-size banner under the push animation, as the home feed does.
    prefetchFullImages([e.banner_image_url ?? e.cover_image_url]);
    router.push(`/event/${e.id}` as any);
  }

  if (loading) {
    return (
      <View style={[styles.centreWrap, { flex: 1, backgroundColor: colors.background }]}>
        <ActivityIndicator color={colors.text} />
      </View>
    );
  }

  if (notFound || !collection) {
    return (
      <View style={{ flex: 1, backgroundColor: colors.background, paddingTop: insets.top }}>
        <View style={styles.header}>
          <TouchableOpacity style={styles.backBtn} onPress={() => router.back()} hitSlop={12}>
            <Text style={[styles.backIcon, { color: colors.text }]}>‹</Text>
          </TouchableOpacity>
        </View>
        <View style={styles.centreWrap}>
          <Text style={[styles.emptyTitle, { color: colors.text }]}>Collection not found</Text>
          <Text style={[styles.emptySub, { color: colors.subText }]}>It may have been removed by the organiser.</Text>
        </View>
      </View>
    );
  }

  const cover = collection.cover_image_url || events[0]?.cover_image_url || null;
  const org = collection.organizer;
  const range = events.length
    ? collectionDateRange({ first_start: events[0].start_time, last_start: events[events.length - 1].start_time })
    : '';

  const ListHeader = (
    <View>
      <View style={styles.coverWrap}>
        {cover ? (
          <ExpoImage
            source={{ uri: cover }}
            style={StyleSheet.absoluteFill}
            contentFit="cover"
            cachePolicy="memory-disk"
            placeholder={IMG_PLACEHOLDER}
            placeholderContentFit="cover"
            recyclingKey={collection.id}
            allowDownscaling
            transition={160}
          />
        ) : (
          <View style={[StyleSheet.absoluteFill, { backgroundColor: isDark ? '#1a1a1a' : '#E8E8E8' }]} />
        )}
        <LinearGradient
          colors={['transparent', 'rgba(0,0,0,0.25)', colors.background]}
          locations={[0.3, 0.7, 1]}
          style={StyleSheet.absoluteFill}
        />
      </View>

      <View style={styles.identityWrap}>
        <View style={styles.kicker}>
          <Ionicons name="layers" size={12} color="#fff" />
          <Text style={styles.kickerText}>
            COLLECTION · {events.length} {events.length === 1 ? 'DATE' : 'DATES'}
          </Text>
        </View>

        <Text style={[styles.title, { color: colors.text }]}>{collection.title}</Text>

        {events.length > 0 && (
          <Text style={[styles.meta, { color: colors.subText }]}>
            {range}
            {cities.length > 0 ? ` · ${cities.length} ${cities.length === 1 ? 'city' : 'cities'}` : ''}
            {` · ${priceLabel}`}
          </Text>
        )}

        {org && (
          <TouchableOpacity
            style={[styles.orgRow, { backgroundColor: subtle }]}
            activeOpacity={0.8}
            onPress={() => router.push(`/organiser/${encodeURIComponent(org.slug || org.id)}` as any)}
          >
            {org.logo_url ? (
              <ExpoImage source={{ uri: org.logo_url }} style={styles.orgLogo} contentFit="cover" cachePolicy="memory-disk" />
            ) : null}
            <Text style={[styles.orgText, { color: colors.subText }]} numberOfLines={1}>
              By <Text style={{ color: colors.text, fontWeight: '700' }}>{org.business_name}</Text>
            </Text>
            {org.verified && <CheckCircle size={15} color="#2f7cf6" weight="fill" />}
            <Text style={[styles.chevron, { color: colors.subText }]}>›</Text>
          </TouchableOpacity>
        )}

        {collection.description ? (
          <Text style={[styles.description, { color: colors.subText }]}>{collection.description}</Text>
        ) : null}
      </View>

      <View style={[styles.datesHead, { borderBottomColor: colors.dividerColor }]}>
        <Text style={[styles.datesTitle, { color: colors.text }]}>DATES</Text>
      </View>

      {cities.length >= 3 && (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.chips}>
          {['all', ...cities].map((c) => {
            const on = city === c;
            return (
              <TouchableOpacity
                key={c}
                onPress={() => setCity(c)}
                activeOpacity={0.8}
                style={[
                  styles.chip,
                  on ? { backgroundColor: '#FF2D55', borderColor: '#FF2D55' } : { borderColor: colors.dividerColor },
                ]}
              >
                <Text style={[styles.chipText, { color: on ? '#fff' : colors.text }]}>
                  {c === 'all' ? 'All cities' : c}
                </Text>
              </TouchableOpacity>
            );
          })}
        </ScrollView>
      )}
    </View>
  );

  return (
    <View style={{ flex: 1, backgroundColor: colors.background }}>
      <StatusBar barStyle={isDark ? 'light-content' : 'dark-content'} />

      <TouchableOpacity
        style={[styles.floatingBack, { top: insets.top + 8 }]}
        onPress={() => router.back()}
        hitSlop={12}
      >
        <Text style={styles.floatingBackIcon}>‹</Text>
      </TouchableOpacity>

      <FlatList
        data={shown}
        keyExtractor={(e) => e.id}
        ListHeaderComponent={ListHeader}
        renderItem={({ item: e }) => (
          <TouchableOpacity
            onPress={() => openEvent(e)}
            activeOpacity={0.75}
            style={[styles.row, { borderBottomColor: colors.dividerColor }]}
          >
            <View style={[styles.dateTile, { backgroundColor: subtle }]}>
              <Text style={styles.dateMonth}>{monthAb(e.start_time)}</Text>
              <Text style={[styles.dateDay, { color: colors.text }]}>{dayNum(e.start_time)}</Text>
            </View>
            <View style={{ flex: 1, minWidth: 0 }}>
              <Text style={[styles.rowTitle, { color: colors.text }]} numberOfLines={1}>
                {e.city || e.title}
              </Text>
              <Text style={[styles.rowSub, { color: colors.subText }]} numberOfLines={1}>
                {e.city && e.title && e.title.trim() !== collection.title.trim()
                  ? `${e.title} · ${e.venue_name}`
                  : e.venue_name}
              </Text>
              <Text style={[styles.rowTime, { color: colors.subText }]} numberOfLines={1}>
                {weekday(e.start_time)} · {timeOf(e.start_time)}{e.is_sold_out ? ' · Sold out' : ''}
              </Text>
            </View>
            <View
              style={[
                styles.pricePill,
                { backgroundColor: e.price === 0 ? '#22c55e' : (isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)') },
              ]}
            >
              <Text style={[styles.pricePillText, { color: e.price === 0 ? '#fff' : colors.text }]}>
                {formatPrice(e.price)}
              </Text>
            </View>
            <Text style={[styles.chevron, { color: colors.subText }]}>›</Text>
          </TouchableOpacity>
        )}
        contentContainerStyle={{ paddingBottom: insets.bottom + 40 }}
        ListEmptyComponent={
          <View style={styles.centreWrap}>
            <Text style={[styles.emptyTitle, { color: colors.text }]}>
              {events.length === 0 ? 'All dates have passed' : 'No dates in this city'}
            </Text>
            {events.length === 0 && (
              <Text style={[styles.emptySub, { color: colors.subText }]}>
                {org ? 'Follow the organiser to hear about the next one.' : 'Check back for new dates.'}
              </Text>
            )}
          </View>
        }
        showsVerticalScrollIndicator={false}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  header: { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingBottom: 12 },
  backBtn:  { width: 40, alignItems: 'center', justifyContent: 'center' },
  backIcon: { fontSize: 32, fontWeight: '300', lineHeight: 36, marginTop: -2 },

  floatingBack: {
    position: 'absolute', left: 12, zIndex: 20,
    width: 38, height: 38, borderRadius: 19,
    backgroundColor: 'rgba(0,0,0,0.45)',
    alignItems: 'center', justifyContent: 'center',
  },
  floatingBackIcon: { color: '#fff', fontSize: 28, fontWeight: '300', lineHeight: 30, marginTop: -2 },

  coverWrap: { width: '100%', height: Math.round(SCREEN_W * 0.62) },

  identityWrap: { paddingHorizontal: 16, marginTop: -28 },
  kicker: {
    alignSelf: 'flex-start', flexDirection: 'row', alignItems: 'center', gap: 6,
    height: 24, paddingHorizontal: 9, borderRadius: 6, backgroundColor: '#FF2D55',
  },
  kickerText: { color: '#fff', fontSize: 10, fontWeight: '800', letterSpacing: 1.2 },
  title: { fontSize: 26, fontWeight: '900', letterSpacing: -0.5, lineHeight: 30, marginTop: 12 },
  meta: { fontSize: 13.5, marginTop: 6 },

  orgRow: {
    alignSelf: 'flex-start', flexDirection: 'row', alignItems: 'center', gap: 8,
    paddingHorizontal: 10, paddingVertical: 8, borderRadius: 12, marginTop: 14, maxWidth: '100%',
  },
  orgLogo: { width: 26, height: 26, borderRadius: 7 },
  orgText: { fontSize: 13, flexShrink: 1 },
  description: { fontSize: 14, lineHeight: 22, marginTop: 16 },

  datesHead: { marginTop: 26, marginHorizontal: 16, paddingBottom: 10, borderBottomWidth: StyleSheet.hairlineWidth },
  datesTitle: { fontSize: 12.5, fontWeight: '800', letterSpacing: 1.4 },

  chips: { paddingHorizontal: 16, paddingTop: 12, gap: 8 },
  chip: { height: 32, paddingHorizontal: 14, borderRadius: 16, borderWidth: 1, alignItems: 'center', justifyContent: 'center' },
  chipText: { fontSize: 12, fontWeight: '700' },

  row: {
    flexDirection: 'row', alignItems: 'center', gap: 12,
    marginHorizontal: 16, paddingVertical: 14, borderBottomWidth: StyleSheet.hairlineWidth,
  },
  dateTile: { width: 54, height: 58, borderRadius: 12, alignItems: 'center', justifyContent: 'center' },
  dateMonth: { color: '#FF2D55', fontSize: 10, fontWeight: '800', letterSpacing: 1 },
  dateDay: { fontSize: 21, fontWeight: '900', lineHeight: 24, marginTop: 1 },
  rowTitle: { fontSize: 15, fontWeight: '700' },
  rowSub: { fontSize: 12.5, marginTop: 2 },
  rowTime: { fontSize: 12, marginTop: 2 },
  pricePill: { borderRadius: 6, paddingHorizontal: 9, paddingVertical: 4 },
  pricePillText: { fontSize: 12, fontWeight: '800' },
  chevron: { fontSize: 22, fontWeight: '300', marginLeft: 2 },

  centreWrap: { alignItems: 'center', justifyContent: 'center', padding: 32 },
  emptyTitle: { fontSize: 17, fontWeight: '700', marginBottom: 8, textAlign: 'center' },
  emptySub:   { fontSize: 13, textAlign: 'center', lineHeight: 19 },
});