import {
  View,
  Text,
  FlatList,
  Image,
  TouchableOpacity,
  Modal,
  ScrollView,
  StatusBar,
  StyleSheet,
  Dimensions,
  SafeAreaView,
  Animated,
  PanResponder,
  ActivityIndicator,
  RefreshControl,
} from 'react-native';
import { useState, useRef, useEffect, useCallback } from 'react';
import { getDropsFeed, type MerchandiseItem } from '../lib/api';

const { width: W, height: H } = Dimensions.get('window');

// ─── Card colour palette ──────────────────────────────────────────────────────
// Derived from category since the DB doesn't store brand colours.
// Extend this map as new categories are added.

const PALETTE: Record<string, { bg: string; text: string }> = {
  sneakers:   { bg: '#0d0d0d',   text: '#ffffff' },
  apparel:    { bg: '#1a3a5c',   text: '#ffffff' },
  culture:    { bg: '#1a6b3c',   text: '#ffffff' },
  drops:      { bg: '#1a8fe8',   text: '#000000' },
  accessories:{ bg: '#4a1a6b',   text: '#ffffff' },
  jersey:     { bg: '#7b1c2a',   text: '#ffffff' },
  footwear:   { bg: '#2d2d2d',   text: '#ffffff' },
  collectible:{ bg: '#cc2200',   text: '#ffffff' },
  default:    { bg: '#1a1a1a',   text: '#ffffff' },
};

function paletteFor(item: MerchandiseItem) {
  const key = (item.category ?? 'default').toLowerCase();
  return PALETTE[key] ?? PALETTE.default;
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function parseImages(raw?: string): string[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter(Boolean) : [];
  } catch {
    return [];
  }
}

function formatPrice(price: number) {
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

// ─── Countdown hooks ──────────────────────────────────────────────────────────

function useTickingSeconds(initial: number) {
  const [secs, setSecs] = useState(initial);
  useEffect(() => {
    if (secs <= 0) return;
    const t = setInterval(() => setSecs((s) => Math.max(0, s - 1)), 1000);
    return () => clearInterval(t);
  }, []);
  return secs;
}

function formatCountdown(totalSecs: number): string {
  const d = Math.floor(totalSecs / 86400);
  const h = Math.floor((totalSecs % 86400) / 3600);
  const m = Math.floor((totalSecs % 3600) / 60);
  const s = totalSecs % 60;
  if (d > 0) return `${d}D  ${String(h).padStart(2, '0')}H  ${String(m).padStart(2, '0')}M`;
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

// ─── Image collage ────────────────────────────────────────────────────────────

const Collage = ({ images }: { images: string[] }) => {
  if (images.length === 0) {
    return <View style={st.collagePlaceholder} />;
  }
  if (images.length === 1) {
    return <Image source={{ uri: images[0] }} style={st.collageSingle} resizeMode="cover" />;
  }
  return (
    <View style={st.collageGrid}>
      <Image source={{ uri: images[0] }} style={st.collageLeft} resizeMode="cover" />
      <View style={st.collageRight}>
        {images.slice(1, 3).map((uri, i) => (
          <Image
            key={i}
            source={{ uri }}
            style={[st.collageSmall, i === 0 && { marginBottom: 2 }]}
            resizeMode="cover"
          />
        ))}
      </View>
    </View>
  );
};

// ─── Available card ───────────────────────────────────────────────────────────

const AvailableCard = ({ item, onPress }: { item: MerchandiseItem; onPress: () => void }) => {
  const { bg, text: tc } = paletteFor(item);
  const images = parseImages(item.images);
  const remainSecs = useTickingSeconds(item.seconds_remain ?? 0);
  const hasExpiry = !!item.sale_end;
  const urgent = hasExpiry && remainSecs < 86400;

  return (
    <TouchableOpacity activeOpacity={0.95} onPress={onPress}>
      <View style={[st.card, { backgroundColor: bg }]}>
        <View style={st.cardTop}>
          <Text style={[st.brandName, { color: tc }]}>
            {(item.category ?? 'DROP').toUpperCase()}
          </Text>
          <View style={{ flex: 1 }} />
          {hasExpiry && (
            <View style={[st.expiryBadge, { borderColor: urgent ? '#ff3c6e' : tc + '40' }]}>
              <Text style={[st.expiryLabel, { color: urgent ? '#ff3c6e' : tc + '80' }]}>ENDS IN</Text>
              <Text style={[st.expiryTimer, { color: urgent ? '#ff3c6e' : tc }]}>
                {formatCountdown(remainSecs)}
              </Text>
            </View>
          )}
        </View>
        <Collage images={images} />
        <Text style={[st.cardTitle, { color: tc }]} numberOfLines={3}>{item.name}</Text>
        <View style={[st.cardBottom, { borderTopColor: tc + '25' }]}>
          <Text style={[st.category, { color: tc + 'aa' }]}>
            {(item.category ?? 'MERCHANDISE').toUpperCase()}
          </Text>
          <View style={{ flex: 1 }} />
          <TouchableOpacity style={[st.actionBtn, { borderColor: tc + '60' }]}>
            <Text style={[st.actionBtnText, { color: tc }]}>
              BUY  {formatPrice(item.effective_price)}
            </Text>
          </TouchableOpacity>
        </View>
      </View>
    </TouchableOpacity>
  );
};

// ─── Upcoming card ────────────────────────────────────────────────────────────

const UpcomingCard = ({ item, onPress }: { item: MerchandiseItem; onPress: () => void }) => {
  const { bg, text: tc } = paletteFor(item);
  const images = parseImages(item.images);
  const untilSecs = useTickingSeconds(item.seconds_until ?? 0);

  return (
    <TouchableOpacity activeOpacity={0.95} onPress={onPress}>
      <View style={[st.card, { backgroundColor: bg }]}>
        <View style={st.cardTop}>
          <Text style={[st.brandName, { color: tc }]}>
            {(item.category ?? 'DROP').toUpperCase()}
          </Text>
          <View style={{ flex: 1 }} />
          <Text style={[st.upcomingLabel, { color: tc + '80' }]}>DROPS IN</Text>
        </View>
        {/* Live countdown */}
        <View style={[st.countdownRow, { borderTopColor: tc + '15', borderBottomColor: tc + '15' }]}>
          <Text style={[st.countdownText, { color: tc }]}>{formatCountdown(untilSecs)}</Text>
        </View>
        <Collage images={images} />
        <Text style={[st.cardTitle, { color: tc }]} numberOfLines={3}>{item.name}</Text>
        <View style={[st.cardBottom, { borderTopColor: tc + '25' }]}>
          <Text style={[st.category, { color: tc + 'aa' }]}>
            {(item.category ?? 'MERCHANDISE').toUpperCase()}
          </Text>
          <View style={{ flex: 1 }} />
          <Text style={[st.releaseDate, { color: tc + '80' }]}>
            {item.sale_start ? new Date(item.sale_start).toLocaleDateString('en-ZA', { day: '2-digit', month: 'short', year: 'numeric' }).toUpperCase() : 'TBC'}
          </Text>
          <View style={{ width: 12 }} />
          <TouchableOpacity style={[st.actionBtn, { borderColor: tc + '60' }]}>
            <Text style={[st.actionBtnText, { color: tc }]}>SET ALERT</Text>
          </TouchableOpacity>
        </View>
      </View>
    </TouchableOpacity>
  );
};

// ─── Detail modal ─────────────────────────────────────────────────────────────

const DetailModal = ({ item, onClose }: { item: MerchandiseItem; onClose: () => void }) => {
  const { bg, text: tc } = paletteFor(item);
  const images = parseImages(item.images);
  const translateY = useRef(new Animated.Value(0)).current;
  const scrollOffset = useRef(0);

  const pilePan = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => true,
      onMoveShouldSetPanResponder: (_, g) => Math.abs(g.dy) > 4,
      onPanResponderMove: (_, g) => { if (g.dy > 0) translateY.setValue(g.dy); },
      onPanResponderRelease: (_, g) => {
        if (g.dy > 80 || g.vy > 0.5) {
          Animated.timing(translateY, { toValue: H, duration: 220, useNativeDriver: true }).start(onClose);
        } else {
          Animated.spring(translateY, { toValue: 0, useNativeDriver: true }).start();
        }
      },
    })
  ).current;

  const scrollPan = useRef(
    PanResponder.create({
      onMoveShouldSetPanResponder: (_, g) =>
        scrollOffset.current <= 0 && g.dy > 12 && g.dy > Math.abs(g.dx),
      onPanResponderMove: (_, g) => { if (g.dy > 0) translateY.setValue(g.dy); },
      onPanResponderRelease: (_, g) => {
        if (g.dy > 100 || g.vy > 0.6) {
          Animated.timing(translateY, { toValue: H, duration: 220, useNativeDriver: true }).start(onClose);
        } else {
          Animated.spring(translateY, { toValue: 0, useNativeDriver: true }).start();
        }
      },
    })
  ).current;

  const isAvailable = item.sale_status === 'live';
  const remainSecs = useTickingSeconds(item.seconds_remain ?? 0);
  const untilSecs  = useTickingSeconds(item.seconds_until ?? 0);

  return (
    <Modal visible animationType="slide" onRequestClose={onClose} statusBarTranslucent>
      <StatusBar barStyle={tc === '#ffffff' ? 'light-content' : 'dark-content'} />
      <Animated.View style={[st.modal, { backgroundColor: bg, transform: [{ translateY }] }]}>

        {/* Drag pill */}
        <View style={st.dragHandleArea} {...pilePan.panHandlers}>
          <View style={[st.dragHandle, { backgroundColor: tc + '55' }]} />
          <View style={st.modalTopBar}>
            <Text style={[st.modalStatus, { color: tc + 'aa' }]}>
              {isAvailable ? '● AVAILABLE NOW' : '◎ DROPPING SOON'}
            </Text>
            <View style={{ flex: 1 }} />
            {isAvailable && item.seconds_remain != null && (
              <Text style={[st.modalHeat, { color: tc + 'aa' }]}>
                ENDS {formatCountdown(remainSecs)}
              </Text>
            )}
            {!isAvailable && item.seconds_until != null && (
              <Text style={[st.modalHeat, { color: tc + 'aa' }]}>
                IN {formatCountdown(untilSecs)}
              </Text>
            )}
          </View>
        </View>

        <ScrollView
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingBottom: 120 }}
          onScroll={(e) => { scrollOffset.current = e.nativeEvent.contentOffset.y; }}
          scrollEventThrottle={16}
          {...scrollPan.panHandlers}
        >
          {/* Price box */}
          <View style={[st.heatScoreBox, { borderColor: tc + '40' }]}>
            <Text style={[st.heatScoreBoxLabel, { color: tc + '90' }]}>
              {(item.category ?? 'MERCHANDISE').toUpperCase()}
            </Text>
            <Text style={[st.heatScoreBoxScore, { color: tc }]}>
              {formatPrice(item.effective_price)}
            </Text>
            <Text style={[st.heatScoreBoxName, { color: tc + '90' }]} numberOfLines={2}>
              {item.name}
            </Text>
          </View>

          <View style={{ marginTop: 16 }}>
            <Collage images={images} />
          </View>

          <Text style={[st.detailTitle, { color: tc }]}>{item.name}</Text>

          {/* Variants */}
          {item.variants && item.variants.length > 0 && (
            <View style={[st.variantsRow, { borderTopColor: tc + '20', borderBottomColor: tc + '20' }]}>
              <Text style={[st.variantsLabel, { color: tc + '70' }]}>VARIANTS</Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ marginTop: 10 }}>
                {item.variants.map((v) => {
                  let label = v.sku;
                  try {
                    const attr = JSON.parse(v.attributes);
                    label = Object.values(attr).join(' / ');
                  } catch {}
                  const outOfStock = v.stock_quantity === 0;
                  return (
                    <TouchableOpacity
                      key={v.id}
                      style={[st.variantChip, { borderColor: tc + (outOfStock ? '25' : '60') }]}
                      disabled={outOfStock}
                    >
                      <Text style={[st.variantChipText, { color: tc + (outOfStock ? '40' : 'ff') }]}>
                        {label.toUpperCase()}
                      </Text>
                      {outOfStock && (
                        <Text style={[st.variantSoldOut, { color: tc + '40' }]}>SOLD OUT</Text>
                      )}
                    </TouchableOpacity>
                  );
                })}
              </ScrollView>
            </View>
          )}

          {/* Details grid */}
          <View style={st.detailGrid}>
            {[
              ['PRICE',    formatPrice(item.effective_price)],
              ['CATEGORY', (item.category ?? '—').toUpperCase()],
              ['STATUS',   item.sale_status.toUpperCase()],
              ['RELEASE',  item.sale_start ? new Date(item.sale_start).toLocaleDateString('en-ZA', { day: '2-digit', month: 'short', year: 'numeric' }).toUpperCase() : 'NOW'],
            ].map(([label, value]) => (
              <View key={label} style={[st.detailRow, { borderBottomColor: tc + '20' }]}>
                <Text style={[st.detailLabel, { color: tc + '70' }]}>{label}</Text>
                <Text style={[st.detailValue, { color: tc }]}>{value}</Text>
              </View>
            ))}
          </View>

          {item.description && (
            <Text style={[st.detailDesc, { color: tc + 'cc' }]}>{item.description}</Text>
          )}
        </ScrollView>

        {/* Bottom bar */}
        <View style={[st.modalBottom, { borderTopColor: tc + '20', backgroundColor: bg }]}>
          <TouchableOpacity>
            <Text style={[st.modalBottomAlerts, { color: tc + 'aa' }]}>ALERTS</Text>
          </TouchableOpacity>
          <View style={st.modalBottomCenter}>
            <Text style={[st.modalBottomTitle, { color: tc }]} numberOfLines={1}>{item.name}</Text>
            <Text style={[st.modalBottomDate, { color: tc + '80' }]}>
              {formatPrice(item.effective_price)}
            </Text>
          </View>
          <TouchableOpacity style={[st.buyNowBtn, { borderColor: tc + '60' }]}>
            <Text style={[st.buyNowText, { color: tc }]}>
              {isAvailable ? 'BUY' : 'ALERT'}
            </Text>
          </TouchableOpacity>
        </View>

      </Animated.View>
    </Modal>
  );
};

// ─── Empty state ──────────────────────────────────────────────────────────────

const EmptyState = ({ tab }: { tab: 'available' | 'upcoming' }) => (
  <View style={st.empty}>
    <Text style={st.emptyIcon}>{tab === 'available' ? '🛍' : '⏳'}</Text>
    <Text style={st.emptyTitle}>
      {tab === 'available' ? 'No drops live right now' : 'Nothing dropping soon'}
    </Text>
    <Text style={st.emptySub}>Check back later — drops are time-limited and rotate often.</Text>
  </View>
);

// ─── Screen ───────────────────────────────────────────────────────────────────

type Tab = 'available' | 'upcoming';

export default function Merch() {
  const [selected, setSelected] = useState<MerchandiseItem | null>(null);
  const [tab, setTab]           = useState<Tab>('available');
  const [available, setAvailable] = useState<MerchandiseItem[]>([]);
  const [upcoming, setUpcoming]   = useState<MerchandiseItem[]>([]);
  const [loading, setLoading]     = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError]           = useState<string | null>(null);

  const fetchDrops = useCallback(async () => {
    try {
      setError(null);
      const { data } = await getDropsFeed();
      setAvailable(data.available ?? []);
      setUpcoming(data.upcoming ?? []);
    } catch (err: any) {
      setError(err?.message ?? 'Failed to load drops.');
    }
  }, []);

  useEffect(() => {
    fetchDrops().finally(() => setLoading(false));
  }, [fetchDrops]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await fetchDrops();
    setRefreshing(false);
  }, [fetchDrops]);

  const visible = tab === 'available' ? available : upcoming;

  return (
    <View style={st.screen}>
      <SafeAreaView>
        <View style={st.header}>
          <Text style={st.headerTitle}>DROPS</Text>
          <View style={st.headerRight}>
            <View style={st.liveIndicator} />
            <Text style={st.liveText}>LIVE</Text>
            <TouchableOpacity style={st.cartBtn}>
              <Text style={st.cartIcon}>🛒</Text>
            </TouchableOpacity>
          </View>
        </View>

        {/* Toggle */}
        <View style={st.toggle}>
          <TouchableOpacity
            style={[st.toggleBtn, tab === 'available' && st.toggleBtnActive]}
            onPress={() => setTab('available')}
          >
            <Text style={[st.toggleText, tab === 'available' && st.toggleTextActive]}>
              AVAILABLE NOW {available.length > 0 ? `(${available.length})` : ''}
            </Text>
          </TouchableOpacity>
          <TouchableOpacity
            style={[st.toggleBtn, tab === 'upcoming' && st.toggleBtnActive]}
            onPress={() => setTab('upcoming')}
          >
            <Text style={[st.toggleText, tab === 'upcoming' && st.toggleTextActive]}>
              DROPPING SOON {upcoming.length > 0 ? `(${upcoming.length})` : ''}
            </Text>
          </TouchableOpacity>
        </View>
      </SafeAreaView>

      {error && (
        <View style={st.errorBanner}>
          <Text style={st.errorText}>⚠ {error}</Text>
          <TouchableOpacity onPress={onRefresh} style={st.retryBtn}>
            <Text style={st.retryText}>RETRY</Text>
          </TouchableOpacity>
        </View>
      )}

      {loading ? (
        <View style={st.loadingRow}>
          <ActivityIndicator size="large" color="#ff3c6e" />
        </View>
      ) : (
        <FlatList
          data={visible}
          keyExtractor={(d) => d.id}
          renderItem={({ item }) =>
            item.sale_status === 'live'
              ? <AvailableCard item={item} onPress={() => setSelected(item)} />
              : <UpcomingCard  item={item} onPress={() => setSelected(item)} />
          }
          ListEmptyComponent={<EmptyState tab={tab} />}
          contentContainerStyle={{ paddingBottom: 110, flexGrow: 1 }}
          showsVerticalScrollIndicator={false}
          refreshControl={
            <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor="#ff3c6e" />
          }
        />
      )}

      {selected && (
        <DetailModal item={selected} onClose={() => setSelected(null)} />
      )}
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const st = StyleSheet.create({
  screen: { flex: 1, backgroundColor: '#0a0a0a' },

  // Header
  header:        { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 20, paddingTop: 8, paddingBottom: 4 },
  headerTitle:   { fontSize: 32, fontWeight: '900', color: '#fff', letterSpacing: 2 },
  headerRight:   { flexDirection: 'row', alignItems: 'center', gap: 10 },
  liveIndicator: { width: 8, height: 8, borderRadius: 4, backgroundColor: '#ff3c6e' },
  liveText:      { fontSize: 11, fontWeight: '700', color: '#ff3c6e', letterSpacing: 1.5 },
  cartBtn:       { padding: 4 },
  cartIcon:      { fontSize: 20 },

  // Toggle
  toggle:           { flexDirection: 'row', marginHorizontal: 20, marginTop: 10, marginBottom: 14, borderRadius: 8, backgroundColor: '#1a1a1a', padding: 3 },
  toggleBtn:        { flex: 1, paddingVertical: 10, alignItems: 'center', borderRadius: 6 },
  toggleBtnActive:  { backgroundColor: '#ff3c6e' },
  toggleText:       { fontSize: 10, fontWeight: '800', color: '#555', letterSpacing: 1.2 },
  toggleTextActive: { color: '#fff' },

  // Card
  card:       { marginBottom: 2 },
  cardTop:    { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 12, gap: 6 },
  brandName:  { fontSize: 11, fontWeight: '800', letterSpacing: 1.5 },
  cardTitle:  { fontSize: 22, fontWeight: '700', lineHeight: 28, paddingHorizontal: 16, paddingTop: 14, paddingBottom: 4 },
  cardBottom: { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 14, borderTopWidth: 1, marginTop: 8 },
  category:   { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },
  releaseDate:{ fontSize: 10, fontWeight: '700', letterSpacing: 1 },

  // Expiry badge
  expiryBadge: { borderWidth: 1, paddingHorizontal: 10, paddingVertical: 6, borderRadius: 3, alignItems: 'center' },
  expiryLabel: { fontSize: 8, fontWeight: '800', letterSpacing: 1.5, marginBottom: 2 },
  expiryTimer: { fontSize: 13, fontWeight: '900', letterSpacing: 1 },

  // Countdown
  upcomingLabel: { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },
  countdownRow:  { borderTopWidth: 1, borderBottomWidth: 1, paddingVertical: 14, paddingHorizontal: 16, marginBottom: 2 },
  countdownText: { fontSize: 36, fontWeight: '900', letterSpacing: 2 },

  // Buttons
  actionBtn:     { borderWidth: 1, paddingHorizontal: 14, paddingVertical: 7, borderRadius: 2 },
  actionBtnText: { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },

  // Collage
  collagePlaceholder: { width: '100%', height: 220, backgroundColor: '#1a1a1a' },
  collageSingle:      { width: '100%', height: 300 },
  collageGrid:        { flexDirection: 'row', height: 250 },
  collageLeft:        { flex: 1, marginRight: 2 },
  collageRight:       { width: W * 0.38, flexDirection: 'column' },
  collageSmall:       { flex: 1 },

  // Loading / error / empty
  loadingRow:  { flex: 1, alignItems: 'center', justifyContent: 'center' },
  errorBanner: { flexDirection: 'row', alignItems: 'center', backgroundColor: '#ff3c6e20', borderColor: '#ff3c6e', borderWidth: 1, margin: 16, padding: 12, borderRadius: 8 },
  errorText:   { flex: 1, color: '#ff3c6e', fontSize: 12 },
  retryBtn:    { borderWidth: 1, borderColor: '#ff3c6e', paddingHorizontal: 12, paddingVertical: 6, borderRadius: 20 },
  retryText:   { color: '#ff3c6e', fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },
  empty:       { flex: 1, alignItems: 'center', justifyContent: 'center', padding: 40, marginTop: 60 },
  emptyIcon:   { fontSize: 48, marginBottom: 16 },
  emptyTitle:  { fontSize: 18, fontWeight: '800', color: '#fff', textAlign: 'center', marginBottom: 8 },
  emptySub:    { fontSize: 13, color: '#666', textAlign: 'center', lineHeight: 20 },

  // Modal
  modal:          { flex: 1 },
  dragHandleArea: { paddingTop: 14, paddingBottom: 4, alignItems: 'center' },
  dragHandle:     { width: 44, height: 5, borderRadius: 3, marginBottom: 10 },
  modalTopBar:    { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingBottom: 8, width: '100%' },
  modalStatus:    { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },
  modalHeat:      { fontSize: 11, fontWeight: '700', letterSpacing: 1 },

  // Price box
  heatScoreBox:       { borderWidth: 1, marginHorizontal: 16, padding: 14, borderRadius: 2 },
  heatScoreBoxLabel:  { fontSize: 10, fontWeight: '700', letterSpacing: 1.5, marginBottom: 4 },
  heatScoreBoxScore:  { fontSize: 36, fontWeight: '900', letterSpacing: -1 },
  heatScoreBoxName:   { fontSize: 11, fontWeight: '500', marginTop: 4, lineHeight: 16 },

  // Variants
  variantsRow:     { paddingHorizontal: 16, paddingVertical: 14, borderTopWidth: 1, borderBottomWidth: 1, marginTop: 4 },
  variantsLabel:   { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },
  variantChip:     { borderWidth: 1, paddingHorizontal: 14, paddingVertical: 8, borderRadius: 2, marginRight: 8, alignItems: 'center' },
  variantChipText: { fontSize: 10, fontWeight: '700', letterSpacing: 1 },
  variantSoldOut:  { fontSize: 8, letterSpacing: 1, marginTop: 2 },

  // Detail
  detailTitle:    { fontSize: 28, fontWeight: '800', lineHeight: 34, paddingHorizontal: 16, paddingTop: 20, paddingBottom: 8 },
  detailGrid:     { paddingHorizontal: 16, marginTop: 16 },
  detailRow:      { flexDirection: 'row', justifyContent: 'space-between', paddingVertical: 12, borderBottomWidth: 1 },
  detailLabel:    { fontSize: 11, fontWeight: '600', letterSpacing: 1.5 },
  detailValue:    { fontSize: 13, fontWeight: '700', letterSpacing: 0.5 },
  detailDesc:     { fontSize: 15, lineHeight: 24, paddingHorizontal: 16, paddingTop: 20 },

  // Modal bottom
  modalBottom:       { position: 'absolute', bottom: 0, left: 0, right: 0, flexDirection: 'row', alignItems: 'center', paddingHorizontal: 20, paddingVertical: 16, paddingBottom: 36, borderTopWidth: 1, gap: 12 },
  modalBottomAlerts: { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },
  modalBottomCenter: { flex: 1, alignItems: 'center' },
  modalBottomTitle:  { fontSize: 11, fontWeight: '700', letterSpacing: 0.5 },
  modalBottomDate:   { fontSize: 10, fontWeight: '500', marginTop: 2 },
  buyNowBtn:         { borderWidth: 1, paddingHorizontal: 18, paddingVertical: 8, borderRadius: 2 },
  buyNowText:        { fontSize: 12, fontWeight: '800', letterSpacing: 1.5 },
});