import {
  View,
  Text,
  FlatList,
  Image,
  TouchableOpacity,
  Modal,
  ScrollView,
  StatusBar,
  Animated,
  PanResponder,
  ActivityIndicator,
  RefreshControl,
} from 'react-native';
import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { SafeAreaView, useSafeAreaInsets } from 'react-native-safe-area-context';
import { getDropsFeed, type MerchandiseItem } from '../../lib/api';
import { s, sheet } from '../../styles/merch';
import { CollectionModal } from '../../components/CollectionModal';
import { useAppTheme } from '../../theme/colors';

// ─── Card colour palette ──────────────────────────────────────────────────────

const PALETTE: Record<string, { bg: string; text: string }> = {
  sneakers:    { bg: '#0d0d0d',   text: '#ffffff' },
  apparel:     { bg: '#1a3a5c',   text: '#ffffff' },
  culture:     { bg: '#1a6b3c',   text: '#ffffff' },
  drops:       { bg: '#1a8fe8',   text: '#000000' },
  accessories: { bg: '#4a1a6b',   text: '#ffffff' },
  jersey:      { bg: '#7b1c2a',   text: '#ffffff' },
  footwear:    { bg: '#2d2d2d',   text: '#ffffff' },
  collectible: { bg: '#cc2200',   text: '#ffffff' },
  default:     { bg: '#1a1a1a',   text: '#ffffff' },
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
  const sec = totalSecs % 60;
  if (d > 0) return `${d}D  ${String(h).padStart(2, '0')}H  ${String(m).padStart(2, '0')}M`;
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`;
}

// ─── Collection grouping ──────────────────────────────────────────────────────

interface CollectionEntry {
  representativeItem: MerchandiseItem;
  items: MerchandiseItem[];
  isCollection: boolean;
}

function buildCollections(items: MerchandiseItem[]): CollectionEntry[] {
  const map = new Map<string, MerchandiseItem[]>();
  for (const item of items) {
    const key = item.event_id;
    if (!map.has(key)) map.set(key, []);
    map.get(key)!.push(item);
  }

  const entries: CollectionEntry[] = [];
  map.forEach((group) => {
    const sorted = [...group].sort((a, b) => a.display_order - b.display_order);
    entries.push({
      representativeItem: sorted[0],
      items: sorted,
      isCollection: sorted.length > 1,
    });
  });

  entries.sort((a, b) => a.representativeItem.display_order - b.representativeItem.display_order);
  return entries;
}

// ─── Image collage ────────────────────────────────────────────────────────────

const Collage = ({ images }: { images: string[] }) => {
  if (images.length === 0) {
    return <View className={s.collagePlaceholder} />;
  }
  if (images.length === 1) {
    return <Image source={{ uri: images[0] }} className={s.collageSingle} resizeMode="cover" />;
  }
  return (
    <View className={s.collageGrid}>
      <Image source={{ uri: images[0] }} className={s.collageLeft} resizeMode="cover" />
      <View style={sheet.collageRight}>
        {images.slice(1, 3).map((uri, i) => (
          <Image
            key={i}
            source={{ uri }}
            className={s.collageSmall}
            style={i === 0 ? { marginBottom: 2 } : undefined}
            resizeMode="cover"
          />
        ))}
      </View>
    </View>
  );
};

// ─── Collection badge ─────────────────────────────────────────────────────────

const CollectionBadge = ({ count, tc }: { count: number; tc: string }) => (
  <View style={{
    borderWidth: 1, borderColor: tc + '50', borderRadius: 2,
    paddingHorizontal: 8, paddingVertical: 4, alignItems: 'center', marginLeft: 8,
  }}>
    <Text style={{ fontSize: 13, fontWeight: '900', color: tc, lineHeight: 15 }}>
      {count}
    </Text>
    <Text style={{ fontSize: 7, fontWeight: '700', letterSpacing: 1.2, color: tc + '60' }}>
      ITEMS
    </Text>
  </View>
);

// ─── Available card ───────────────────────────────────────────────────────────

const AvailableCard = ({ entry, onPress }: { entry: CollectionEntry; onPress: () => void }) => {
  const { item, isCollection } = { item: entry.representativeItem, isCollection: entry.isCollection };
  const { bg, text: tc } = paletteFor(item);
  const images = parseImages(item.images);
  const remainSecs = useTickingSeconds(item.seconds_remain ?? 0);
  const hasExpiry = !!item.sale_end;
  const urgent = hasExpiry && remainSecs < 86400;

  return (
    <TouchableOpacity activeOpacity={0.95} onPress={onPress}>
      <View className={s.card} style={{ backgroundColor: bg }}>
        <View className={s.cardTop}>
          <Text className={s.brandName} style={{ color: tc }}>
            {(item.category ?? 'DROP').toUpperCase()}
          </Text>
          {isCollection && <CollectionBadge count={entry.items.length} tc={tc} />}
          <View style={{ flex: 1 }} />
          {hasExpiry && (
            <View className={s.expiryBadge} style={{ borderColor: urgent ? '#ff3c6e' : tc + '40' }}>
              <Text className={s.expiryLabel} style={{ color: urgent ? '#ff3c6e' : tc + '80' }}>
                ENDS IN
              </Text>
              <Text className={s.expiryTimer} style={{ color: urgent ? '#ff3c6e' : tc }}>
                {formatCountdown(remainSecs)}
              </Text>
            </View>
          )}
        </View>
        <Collage images={images} />
        <Text className={s.cardTitle} style={{ color: tc }} numberOfLines={3}>
          {isCollection ? (item.event_name || item.name) : item.name}
        </Text>
        <View className={s.cardBottom} style={{ borderTopColor: tc + '25' }}>
          <Text className={s.category} style={{ color: tc + 'aa' }}>
            {isCollection ? `${entry.items.length} ITEMS` : (item.category ?? 'MERCHANDISE').toUpperCase()}
          </Text>
          <View style={{ flex: 1 }} />
          <TouchableOpacity className={s.actionBtn} style={{ borderColor: tc + '60' }} onPress={onPress}>
            <Text className={s.actionBtnText} style={{ color: tc }}>
              {isCollection ? 'VIEW ALL' : `BUY  ${formatPrice(item.effective_price)}`}
            </Text>
          </TouchableOpacity>
        </View>
      </View>
    </TouchableOpacity>
  );
};

// ─── Upcoming card ────────────────────────────────────────────────────────────

const UpcomingCard = ({ entry, onPress }: { entry: CollectionEntry; onPress: () => void }) => {
  const { item, isCollection } = { item: entry.representativeItem, isCollection: entry.isCollection };
  const { bg, text: tc } = paletteFor(item);
  const images = parseImages(item.images);
  const untilSecs = useTickingSeconds(item.seconds_until ?? 0);

  return (
    <TouchableOpacity activeOpacity={0.95} onPress={onPress}>
      <View className={s.card} style={{ backgroundColor: bg }}>
        <View className={s.cardTop}>
          <Text className={s.brandName} style={{ color: tc }}>
            {(item.category ?? 'DROP').toUpperCase()}
          </Text>
          {isCollection && <CollectionBadge count={entry.items.length} tc={tc} />}
          <View style={{ flex: 1 }} />
          <Text className={s.upcomingLabel} style={{ color: tc + '80' }}>
            DROPS IN
          </Text>
        </View>
        <View className={s.countdownRow} style={{ borderTopColor: tc + '15', borderBottomColor: tc + '15' }}>
          <Text className={s.countdownText} style={{ color: tc }}>
            {formatCountdown(untilSecs)}
          </Text>
        </View>
        <Collage images={images} />
        <Text className={s.cardTitle} style={{ color: tc }} numberOfLines={3}>
          {isCollection ? (item.event_name || item.name) : item.name}
        </Text>
        <View className={s.cardBottom} style={{ borderTopColor: tc + '25' }}>
          <Text className={s.category} style={{ color: tc + 'aa' }}>
            {isCollection ? `${entry.items.length} ITEMS` : (item.category ?? 'MERCHANDISE').toUpperCase()}
          </Text>
          <View style={{ flex: 1 }} />
          <Text className={s.releaseDate} style={{ color: tc + '80' }}>
            {item.sale_start
              ? new Date(item.sale_start)
                  .toLocaleDateString('en-ZA', { day: '2-digit', month: 'short', year: 'numeric' })
                  .toUpperCase()
              : 'TBC'}
          </Text>
          <View style={{ width: 12 }} />
          <TouchableOpacity className={s.actionBtn} style={{ borderColor: tc + '60' }} onPress={onPress}>
            <Text className={s.actionBtnText} style={{ color: tc }}>
              {isCollection ? 'VIEW ALL' : 'SET ALERT'}
            </Text>
          </TouchableOpacity>
        </View>
      </View>
    </TouchableOpacity>
  );
};

// ─── Detail modal (for direct feed card taps) ─────────────────────────────────

const DetailModal = ({ item, onClose }: { item: MerchandiseItem; onClose: () => void }) => {
  const { bg, text: tc } = paletteFor(item);
  const images = parseImages(item.images);
  const { top } = useSafeAreaInsets();
  const translateY = useRef(new Animated.Value(800)).current;
  useEffect(() => {
    Animated.timing(translateY, { toValue: 0, duration: 380, useNativeDriver: true }).start();
  }, []);
  const scrollOffset = useRef(0);

  const pilePan = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => true,
      onMoveShouldSetPanResponder: (_, g) => Math.abs(g.dy) > 4,
      onPanResponderMove: (_, g) => { if (g.dy > 0) translateY.setValue(g.dy); },
      onPanResponderRelease: (_, g) => {
        if (g.dy > 80 || g.vy > 0.5) {
          Animated.timing(translateY, { toValue: 800, duration: 140, useNativeDriver: true }).start(onClose);
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
          Animated.timing(translateY, { toValue: 800, duration: 140, useNativeDriver: true }).start(onClose);
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
    <Modal visible animationType="none" onRequestClose={onClose} statusBarTranslucent>
      <StatusBar barStyle={tc === '#ffffff' ? 'light-content' : 'dark-content'} />
      <Animated.View style={[sheet.modalFlex, { backgroundColor: bg, transform: [{ translateY }] }]}>

        <View className={s.dragHandleArea} style={{ paddingTop: top + 8 }} {...pilePan.panHandlers}>
          <View className={s.dragHandle} style={{ backgroundColor: tc + '55' }} />
          <View className={s.modalTopBar}>
            <Text className={s.modalStatus} style={{ color: tc + 'aa' }}>
              {isAvailable ? '● AVAILABLE NOW' : '◎ DROPPING SOON'}
            </Text>
            <View style={{ flex: 1 }} />
            {isAvailable && item.seconds_remain != null && (
              <Text className={s.modalHeat} style={{ color: tc + 'aa' }}>
                ENDS {formatCountdown(remainSecs)}
              </Text>
            )}
            {!isAvailable && item.seconds_until != null && (
              <Text className={s.modalHeat} style={{ color: tc + 'aa' }}>
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
          <View className={s.heatScoreBox} style={{ borderColor: tc + '40' }}>
            <Text className={s.heatScoreBoxLabel} style={{ color: tc + '90' }}>
              {(item.category ?? 'MERCHANDISE').toUpperCase()}
            </Text>
            <Text className={s.heatScoreBoxScore} style={{ color: tc }}>
              {formatPrice(item.effective_price)}
            </Text>
            <Text className={s.heatScoreBoxName} style={{ color: tc + '90' }} numberOfLines={2}>
              {item.name}
            </Text>
          </View>

          <View style={{ marginTop: 16 }}>
            <Collage images={images} />
          </View>

          <Text className={s.detailTitle} style={{ color: tc }}>{item.name}</Text>

          {item.variants && item.variants.length > 0 && (
            <View
              className={s.variantsRow}
              style={{ borderTopColor: tc + '20', borderBottomColor: tc + '20' }}
            >
              <Text className={s.variantsLabel} style={{ color: tc + '70' }}>VARIANTS</Text>
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
                      className={s.variantChip}
                      style={{ borderColor: tc + (outOfStock ? '25' : '60') }}
                      disabled={outOfStock}
                    >
                      <Text
                        className={s.variantChipText}
                        style={{ color: tc + (outOfStock ? '40' : 'ff') }}
                      >
                        {label.toUpperCase()}
                      </Text>
                      {outOfStock && (
                        <Text className={s.variantSoldOut} style={{ color: tc + '40' }}>
                          SOLD OUT
                        </Text>
                      )}
                    </TouchableOpacity>
                  );
                })}
              </ScrollView>
            </View>
          )}

          <View className={s.detailGrid}>
            {[
              ['PRICE',    formatPrice(item.effective_price)],
              ['CATEGORY', (item.category ?? '—').toUpperCase()],
              ['STATUS',   item.sale_status.toUpperCase()],
              ['RELEASE',  item.sale_start
                ? new Date(item.sale_start)
                    .toLocaleDateString('en-ZA', { day: '2-digit', month: 'short', year: 'numeric' })
                    .toUpperCase()
                : 'NOW'],
            ].map(([label, value]) => (
              <View key={label} className={s.detailRow} style={{ borderBottomColor: tc + '20' }}>
                <Text className={s.detailLabel} style={{ color: tc + '70' }}>{label}</Text>
                <Text className={s.detailValue} style={{ color: tc }}>{value}</Text>
              </View>
            ))}
          </View>

          {item.description && (
            <Text className={s.detailDesc} style={{ color: tc + 'cc' }}>{item.description}</Text>
          )}
        </ScrollView>

        <View className={s.modalBottom} style={{ borderTopColor: tc + '20', backgroundColor: bg }}>
          <TouchableOpacity>
            <Text className={s.modalBottomAlerts} style={{ color: tc + 'aa' }}>ALERTS</Text>
          </TouchableOpacity>
          <View className={s.modalBottomCenter}>
            <Text className={s.modalBottomTitle} style={{ color: tc }} numberOfLines={1}>
              {item.name}
            </Text>
            <Text className={s.modalBottomDate} style={{ color: tc + '80' }}>
              {formatPrice(item.effective_price)}
            </Text>
          </View>
          <TouchableOpacity className={s.buyNowBtn} style={{ borderColor: tc + '60' }}>
            <Text className={s.buyNowText} style={{ color: tc }}>
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
  <View className={s.empty}>
    <Text className={s.emptyIcon}>{tab === 'available' ? '🛍' : '⏳'}</Text>
    <Text className={s.emptyTitle}>
      {tab === 'available' ? 'No drops live right now' : 'Nothing dropping soon'}
    </Text>
    <Text className={s.emptySub}>
      Check back later — drops are time-limited and rotate often.
    </Text>
  </View>
);

// ─── Screen ───────────────────────────────────────────────────────────────────

type Tab = 'available' | 'upcoming';

interface ActiveCollection {
  name: string;
  items: MerchandiseItem[];
}

export default function Merch() {
  const [selected,         setSelected]         = useState<MerchandiseItem | null>(null);
  const [activeCollection, setActiveCollection] = useState<ActiveCollection | null>(null);
  const [tab,              setTab]              = useState<Tab>('available');
  const [available,        setAvailable]        = useState<MerchandiseItem[]>([]);
  const [upcoming,         setUpcoming]         = useState<MerchandiseItem[]>([]);
  const [loading,          setLoading]          = useState(true);
  const [refreshing,       setRefreshing]       = useState(false);
  const [error,            setError]            = useState<string | null>(null);
  const { colors }                              = useAppTheme();

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

  const availableCollections = useMemo(() => buildCollections(available), [available]);
  const upcomingCollections  = useMemo(() => buildCollections(upcoming),  [upcoming]);
  const visibleCollections   = tab === 'available' ? availableCollections : upcomingCollections;

  const handleEntryPress = useCallback((entry: CollectionEntry) => {
    if (entry.isCollection) {
      setActiveCollection({
        name:  entry.representativeItem.event_name || entry.representativeItem.name,
        items: entry.items,
      });
    } else {
      setSelected(entry.representativeItem);
    }
  }, []);

  return (
    <View className={s.screen} style={{ backgroundColor: colors.background }}>
      <SafeAreaView edges={['top', 'left', 'right']}>
        <View className={s.header}>
          <Text className={s.headerTitle}>DROPS</Text>
          <View className={s.headerRight}>
            <TouchableOpacity className={s.cartBtn}>
              <Text className={s.cartIcon}>🛒</Text>
            </TouchableOpacity>
          </View>
        </View>

        <View className={s.toggle}>
          <TouchableOpacity
            className={`${s.toggleBtn} ${tab === 'available' ? s.toggleBtnActive : ''}`}
            onPress={() => setTab('available')}
          >
            <Text className={`${s.toggleText} ${tab === 'available' ? s.toggleTextActive : ''}`}>
              AVAILABLE NOW
            </Text>
          </TouchableOpacity>
          <TouchableOpacity
            className={`${s.toggleBtn} ${tab === 'upcoming' ? s.toggleBtnActive : ''}`}
            onPress={() => setTab('upcoming')}
          >
            <Text className={`${s.toggleText} ${tab === 'upcoming' ? s.toggleTextActive : ''}`}>
              DROPPING SOON
            </Text>
          </TouchableOpacity>
        </View>
      </SafeAreaView>

      {error && (
        <View className={s.errorBanner}>
          <Text className={s.errorText}>⚠ {error}</Text>
          <TouchableOpacity onPress={onRefresh} className={s.retryBtn}>
            <Text className={s.retryText}>RETRY</Text>
          </TouchableOpacity>
        </View>
      )}

      {loading ? (
        <View className={s.loadingRow}>
          <ActivityIndicator size="large" color="#ff3c6e" />
        </View>
      ) : (
        <FlatList
          data={visibleCollections}
          keyExtractor={(entry) => `${entry.representativeItem.event_id}-${tab}`}
          renderItem={({ item: entry }) =>
            entry.representativeItem.sale_status === 'live'
              ? <AvailableCard entry={entry} onPress={() => handleEntryPress(entry)} />
              : <UpcomingCard  entry={entry} onPress={() => handleEntryPress(entry)} />
          }
          ListEmptyComponent={<EmptyState tab={tab} />}
          contentContainerStyle={{ paddingBottom: 110, flexGrow: 1 }}
          showsVerticalScrollIndicator={false}
          refreshControl={
            <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor="#ff3c6e" />
          }
        />
      )}

      {/* Single-item detail modal (direct feed card taps) */}
      {selected && (
        <DetailModal item={selected} onClose={() => setSelected(null)} />
      )}

      {/* Collection carousel — detail is handled internally by CollectionModal */}
      {activeCollection && (
        <CollectionModal
          collectionName={activeCollection.name}
          items={activeCollection.items}
          onSelectItem={(item) => {
            // handled internally by CollectionModal
          }}
          onClose={() => setActiveCollection(null)}
        />
      )}
    </View>
  );
}