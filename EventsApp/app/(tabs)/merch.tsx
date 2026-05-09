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
  StyleSheet,
  Dimensions,
} from 'react-native';
import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { SafeAreaView, useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { getDropsFeed, addMerchToCart, getCachedSessionId, type MerchandiseItem } from '../../lib/api';
import { s, sheet } from '../../styles/merch';
import { CollectionModal } from '../../components/CollectionModal';
import { CartConfirmSheet } from '../../components/CartConfirmSheet';
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

// ─── Variant grouping ─────────────────────────────────────────────────────────

type VariantItem = NonNullable<MerchandiseItem['variants']>[number];

interface VariantGroup {
  key:      string;
  label:    string;
  variants: VariantItem[];
}

function groupVariantsByAttr(variants: VariantItem[]): VariantGroup[] {
  const ordered: string[] = [];
  const map = new Map<string, VariantItem[]>();

  for (const v of variants) {
    try {
      const attr = JSON.parse(v.attributes) as Record<string, string>;
      const primaryKey = Object.keys(attr)[0] ?? 'variant';
      if (!map.has(primaryKey)) {
        ordered.push(primaryKey);
        map.set(primaryKey, []);
      }
      map.get(primaryKey)!.push(v);
    } catch {
      if (!map.has('variant')) {
        ordered.push('variant');
        map.set('variant', []);
      }
      map.get('variant')!.push(v);
    }
  }

  return ordered.map((key) => ({
    key,
    label:
      key === 'size'   ? 'SIZE'   :
      key === 'colour' ? 'COLOUR' :
      key === 'fit'    ? 'FIT'    :
      key === 'type'   ? 'TYPE'   :
      key.toUpperCase(),
    variants: map.get(key)!,
  }));
}

function getVariantLabel(v: VariantItem): string {
  try {
    const attr = JSON.parse(v.attributes) as Record<string, string>;
    return Object.values(attr).join(' / ');
  } catch {
    return v.sku;
  }
}

// ─── Collection grouping ──────────────────────────────────────────────────────

interface CollectionEntry {
  representativeItem: MerchandiseItem;
  items: MerchandiseItem[];
  isCollection: boolean;
  collectionName?: string; // set when items are in a named merch_collection
}

function buildCollections(items: MerchandiseItem[]): CollectionEntry[] {
  // Items with collection_id → group by collection (uses the name you set in admin)
  // Items without collection_id → group by event_id (original behaviour, preserves collage)
  const collectionMap = new Map<string, MerchandiseItem[]>();
  const eventMap      = new Map<string, MerchandiseItem[]>();

  for (const item of items) {
    if (item.collection_id) {
      if (!collectionMap.has(item.collection_id)) collectionMap.set(item.collection_id, []);
      collectionMap.get(item.collection_id)!.push(item);
    } else {
      const key = item.event_id || item.id;
      if (!eventMap.has(key)) eventMap.set(key, []);
      eventMap.get(key)!.push(item);
    }
  }

  const entries: CollectionEntry[] = [];

  // Named collection cards — title = collection_name from admin
  collectionMap.forEach((group) => {
    const sorted = [...group].sort((a, b) => a.display_order - b.display_order);
    entries.push({
      representativeItem: sorted[0],
      items:              sorted,
      isCollection:       true,
      collectionName:     sorted[0].collection_name || undefined,
    });
  });

  // Event-grouped cards — original behaviour for items not in a named collection
  eventMap.forEach((group) => {
    const sorted = [...group].sort((a, b) => a.display_order - b.display_order);
    entries.push({
      representativeItem: sorted[0],
      items:              sorted,
      isCollection:       sorted.length > 1,
    });
  });

  entries.sort((a, b) => a.representativeItem.display_order - b.representativeItem.display_order);
  return entries;
}

// ─── Image collage (main feed cards — unchanged) ──────────────────────────────

const Collage = ({ images }: { images: string[] }) => {
  if (images.length === 0) {
    return <View style={sheet.collagePlaceholderBox} />;
  }
  if (images.length === 1) {
    return <Image source={{ uri: images[0] }} style={sheet.collageSingleImg} resizeMode="cover" />;
  }
  return (
    <View style={sheet.collageGridContainer}>
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

// ─── Detail image carousel (detail modal only) ────────────────────────────────

const { width: SCREEN_W } = Dimensions.get('window');
const CAROUSEL_SIDE   = 16;
const CAROUSEL_W      = SCREEN_W - CAROUSEL_SIDE * 2;
const CAROUSEL_RADIUS = 12;
const CAROUSEL_H      = 300;

const DetailImageCarousel = ({ images, tc }: { images: string[]; tc: string }) => {
  const scrollX = useRef(new Animated.Value(0)).current;

  if (images.length === 0) {
    return (
      <View style={{
        width: CAROUSEL_W, alignSelf: 'center', height: CAROUSEL_H,
        borderRadius: CAROUSEL_RADIUS, overflow: 'hidden',
        backgroundColor: tc + '10', alignItems: 'center', justifyContent: 'center',
      }}>
        <Text style={{ fontSize: 40 }}>📦</Text>
      </View>
    );
  }

  return (
    <View>
      <View collapsable={false} style={{
        width: CAROUSEL_W, alignSelf: 'center', height: CAROUSEL_H,
        borderRadius: CAROUSEL_RADIUS, overflow: 'hidden',
      }}>
        <Animated.ScrollView
          horizontal showsHorizontalScrollIndicator={false}
          snapToInterval={CAROUSEL_W} snapToAlignment="start"
          decelerationRate="fast" disableIntervalMomentum scrollEventThrottle={16}
          onScroll={Animated.event(
            [{ nativeEvent: { contentOffset: { x: scrollX } } }],
            { useNativeDriver: true },
          )}
        >
          {images.map((uri, i) => (
            <Image key={i} source={{ uri }} style={{ width: CAROUSEL_W, height: CAROUSEL_H }} resizeMode="cover" />
          ))}
        </Animated.ScrollView>
      </View>

      {images.length > 1 && (
        <View style={{ flexDirection: 'row', justifyContent: 'center', alignItems: 'center', gap: 6, paddingTop: 10 }}>
          {images.map((_, i) => {
            const dotScale = scrollX.interpolate({
              inputRange:  [(i - 1) * CAROUSEL_W, i * CAROUSEL_W, (i + 1) * CAROUSEL_W],
              outputRange: [6 / 22, 1, 6 / 22], extrapolate: 'clamp',
            });
            const dotOpacity = scrollX.interpolate({
              inputRange:  [(i - 1) * CAROUSEL_W, i * CAROUSEL_W, (i + 1) * CAROUSEL_W],
              outputRange: [0.3, 1, 0.3], extrapolate: 'clamp',
            });
            return (
              <Animated.View key={i} style={{
                width: 22, height: 4, borderRadius: 2, backgroundColor: tc,
                opacity: dotOpacity, transform: [{ scaleX: dotScale }],
              }} />
            );
          })}
        </View>
      )}
    </View>
  );
};

// ─── Collection badge ─────────────────────────────────────────────────────────

const CollectionBadge = ({ count, tc }: { count: number; tc: string }) => (
  <View style={{
    borderWidth: 1, borderColor: tc + '50', borderRadius: 2,
    paddingHorizontal: 8, paddingVertical: 4, alignItems: 'center', marginLeft: 8,
  }}>
    <Text style={{ fontSize: 13, fontWeight: '900', color: tc, lineHeight: 15 }}>{count}</Text>
    <Text style={{ fontSize: 7, fontWeight: '700', letterSpacing: 1.2, color: tc + '60' }}>ITEMS</Text>
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

  // Title priority: named collection name → event name → item name
  const title = isCollection
    ? (entry.collectionName || item.event_name || item.name)
    : item.name;

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
              <Text className={s.expiryLabel} style={{ color: urgent ? '#ff3c6e' : tc + '80' }}>ENDS IN</Text>
              <Text className={s.expiryTimer} style={{ color: urgent ? '#ff3c6e' : tc }}>{formatCountdown(remainSecs)}</Text>
            </View>
          )}
        </View>
        <Collage images={images} />
        <Text className={s.cardTitle} style={{ color: tc }} numberOfLines={3}>{title}</Text>
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

  // Title priority: named collection name → event name → item name
  const title = isCollection
    ? (entry.collectionName || item.event_name || item.name)
    : item.name;

  return (
    <TouchableOpacity activeOpacity={0.95} onPress={onPress}>
      <View className={s.card} style={{ backgroundColor: bg }}>
        <View className={s.cardTop}>
          <Text className={s.brandName} style={{ color: tc }}>
            {(item.category ?? 'DROP').toUpperCase()}
          </Text>
          {isCollection && <CollectionBadge count={entry.items.length} tc={tc} />}
          <View style={{ flex: 1 }} />
          <Text className={s.upcomingLabel} style={{ color: tc + '80' }}>DROPS IN</Text>
        </View>
        <View className={s.countdownRow} style={{ borderTopColor: tc + '15', borderBottomColor: tc + '15' }}>
          <Text className={s.countdownText} style={{ color: tc }}>{formatCountdown(untilSecs)}</Text>
        </View>
        <Collage images={images} />
        <Text className={s.cardTitle} style={{ color: tc }} numberOfLines={3}>{title}</Text>
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

// ─── Size selector ────────────────────────────────────────────────────────────

const SizeSelector = ({
  groups,
  selectedVariant,
  onSelect,
  tc,
}: {
  groups: VariantGroup[];
  selectedVariant: VariantItem | null;
  onSelect: (v: VariantItem | null) => void;
  tc: string;
}) => (
  <>
    {groups.map((group) => {
      const groupSelected = group.variants.find((v) => v.id === selectedVariant?.id) ?? null;
      return (
        <View
          key={group.key}
          className={s.variantsRow}
          style={{ borderTopColor: tc + '20', borderBottomColor: tc + '20', marginTop: 2 }}
        >
          <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
            <Text className={s.variantsLabel} style={{ color: tc + '70' }}>{group.label}</Text>
            {groupSelected && (
              <Text style={{ fontSize: 10, fontWeight: '700', letterSpacing: 1, color: tc + 'bb' }}>
                {getVariantLabel(groupSelected).toUpperCase()}
                {groupSelected.stock_quantity > 0 && groupSelected.stock_quantity <= 5
                  ? `  —  ONLY ${groupSelected.stock_quantity} LEFT` : ''}
              </Text>
            )}
          </View>
          <ScrollView horizontal showsHorizontalScrollIndicator={false}>
            {group.variants.map((v) => {
              const label      = getVariantLabel(v);
              const outOfStock = v.stock_quantity === 0;
              const lowStock   = !outOfStock && v.stock_quantity <= 5;
              const isSelected = selectedVariant?.id === v.id;
              return (
                <TouchableOpacity
                  key={v.id}
                  onPress={() => onSelect(isSelected ? null : v)}
                  disabled={outOfStock}
                  style={{
                    borderWidth: isSelected ? 2 : 1,
                    borderColor: outOfStock ? tc + '20' : isSelected ? tc : tc + '50',
                    backgroundColor: isSelected ? tc + '18' : 'transparent',
                    paddingHorizontal: 14, paddingVertical: 9, borderRadius: 2,
                    marginRight: 8, alignItems: 'center', minWidth: 54,
                  }}
                >
                  <Text style={{
                    fontSize: 11, fontWeight: isSelected ? '900' : '700', letterSpacing: 0.8,
                    color: outOfStock ? tc + '28' : isSelected ? tc : tc + 'cc',
                  }}>
                    {label.toUpperCase()}
                  </Text>
                  {lowStock && <Text style={{ fontSize: 7, color: '#ff9500', fontWeight: '800', marginTop: 2, letterSpacing: 0.5 }}>LOW</Text>}
                  {outOfStock && <Text className={s.variantSoldOut} style={{ color: tc + '30' }}>SOLD OUT</Text>}
                  {v.price_override != null && <Text style={{ fontSize: 8, color: tc + '80', fontWeight: '700', marginTop: 2 }}>{formatPrice(v.price_override)}</Text>}
                </TouchableOpacity>
              );
            })}
          </ScrollView>
        </View>
      );
    })}
  </>
);

// ─── Detail modal (for direct feed card taps) ─────────────────────────────────

const DetailModal = ({ item, onClose }: { item: MerchandiseItem; onClose: () => void }) => {
  const { bg, text: tc } = paletteFor(item);
  const images = parseImages(item.images);
  const { top } = useSafeAreaInsets();
  const router = useRouter();
  const translateY = useRef(new Animated.Value(800)).current;
  useEffect(() => {
    Animated.timing(translateY, { toValue: 0, duration: 380, useNativeDriver: true }).start();
  }, []);
  const scrollOffset = useRef(0);

  const [selectedVariant, setSelectedVariant] = useState<VariantItem | null>(null);
  const [showCart, setShowCart] = useState(false);
  const variantGroups = useMemo(
    () => (item.variants && item.variants.length > 0 ? groupVariantsByAttr(item.variants) : []),
    [item.variants],
  );
  const hasVariants = variantGroups.length > 0;
  const displayPrice =
    selectedVariant?.price_override != null ? selectedVariant.price_override : item.effective_price;

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
      onMoveShouldSetPanResponderCapture: (_, g) =>
        scrollOffset.current <= 0 && g.dy > 8 && g.dy > Math.abs(g.dx),
      onPanResponderMove: (_, g) => { if (g.dy > 0) translateY.setValue(g.dy); },
      onPanResponderRelease: (_, g) => {
        if (g.dy > 100 || g.vy > 0.6) {
          Animated.timing(translateY, { toValue: 800, duration: 140, useNativeDriver: true }).start(onClose);
        } else {
          Animated.spring(translateY, { toValue: 0, useNativeDriver: true }).start();
        }
      },
      onPanResponderTerminate: () => {
        Animated.spring(translateY, { toValue: 0, useNativeDriver: true }).start();
      },
    })
  ).current;

  const isAvailable = item.sale_status === 'live';
  const remainSecs = useTickingSeconds(item.seconds_remain ?? 0);
  const untilSecs  = useTickingSeconds(item.seconds_until ?? 0);
  const buyDisabled = isAvailable && hasVariants && !selectedVariant;

  return (
    <Modal visible transparent animationType="none" onRequestClose={onClose} statusBarTranslucent>
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
              <Text className={s.modalHeat} style={{ color: tc + 'aa' }}>ENDS {formatCountdown(remainSecs)}</Text>
            )}
            {!isAvailable && item.seconds_until != null && (
              <Text className={s.modalHeat} style={{ color: tc + 'aa' }}>IN {formatCountdown(untilSecs)}</Text>
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
            <Text className={s.heatScoreBoxScore} style={{ color: tc }}>{formatPrice(displayPrice)}</Text>
            <Text className={s.heatScoreBoxName} style={{ color: tc + '90' }} numberOfLines={2}>{item.name}</Text>
          </View>

          <View style={{ marginTop: 16 }}>
            <DetailImageCarousel images={images} tc={tc} />
          </View>

          <Text className={s.detailTitle} style={{ color: tc }}>{item.name}</Text>

          {hasVariants && (
            <SizeSelector groups={variantGroups} selectedVariant={selectedVariant} onSelect={setSelectedVariant} tc={tc} />
          )}

          <View className={s.detailGrid}>
            {[
              ['PRICE',    formatPrice(displayPrice)],
              ['CATEGORY', (item.category ?? '—').toUpperCase()],
              ['STATUS',   item.sale_status.toUpperCase()],
              ['RELEASE',  item.sale_start
                ? new Date(item.sale_start).toLocaleDateString('en-ZA', { day: '2-digit', month: 'short', year: 'numeric' }).toUpperCase()
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
            <Text className={s.modalBottomTitle} style={{ color: tc }} numberOfLines={1}>{item.name}</Text>
            <Text className={s.modalBottomDate} style={{ color: tc + '80' }}>{formatPrice(displayPrice)}</Text>
          </View>
          <TouchableOpacity
            className={s.buyNowBtn}
            style={{ borderColor: buyDisabled ? tc + '25' : tc + '60', opacity: buyDisabled ? 0.45 : 1 }}
            disabled={buyDisabled}
            onPress={isAvailable && !buyDisabled ? () => setShowCart(true) : undefined}
          >
            <Text className={s.buyNowText} style={{ color: tc }}>
              {isAvailable ? buyDisabled ? 'SELECT SIZE' : 'BUY' : 'ALERT'}
            </Text>
          </TouchableOpacity>
        </View>
      </Animated.View>

      {showCart && (
        <CartConfirmSheet
          item={item}
          selectedVariant={selectedVariant}
          onClose={() => setShowCart(false)}
          onAddToCart={async (cartItem, variant) => {
            try {
              const sid = await getCachedSessionId();
              await addMerchToCart(
                {
                  item_type:              'merchandise',
                  event_id:               cartItem.event_id,
                  merchandise_variant_id: variant?.id,
                  quantity:               1,
                },
                sid,
              );
            } catch {
              // silently ignore — cart screen will show latest state on next visit
            }
          }}
        />
      )}
    </Modal>
  );
};

// ─── Empty state ──────────────────────────────────────────────────────────────

const EmptyState = ({ tab }: { tab: 'available' | 'upcoming' }) => {
  const { colors } = useAppTheme();
  return (
    <View className={s.empty}>
      <Text className={s.emptyIcon}>{tab === 'available' ? '🛍' : '⏳'}</Text>
      <Text className={s.emptyTitle} style={{ color: colors.text }}>
        {tab === 'available' ? 'No drops live right now' : 'Nothing dropping soon'}
      </Text>
      <Text className={s.emptySub} style={{ color: colors.subText }}>
        Check back later — drops are time-limited and rotate often.
      </Text>
    </View>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

type Tab = 'available' | 'upcoming';

interface ActiveCollection {
  name: string;
  items: MerchandiseItem[];
}

export default function Merch() {
  const router                                  = useRouter();
  const [selected,         setSelected]         = useState<MerchandiseItem | null>(null);
  const [activeCollection, setActiveCollection] = useState<ActiveCollection | null>(null);
  const [tab,              setTab]              = useState<Tab>('available');
  const [available,        setAvailable]        = useState<MerchandiseItem[]>([]);
  const [upcoming,         setUpcoming]         = useState<MerchandiseItem[]>([]);
  const [loading,          setLoading]          = useState(true);
  const [refreshing,       setRefreshing]       = useState(false);
  const [error,            setError]            = useState<string | null>(null);
  const { colors }                              = useAppTheme();
  const insets                                  = useSafeAreaInsets();

  const lastScrollY  = useRef(0);
  const headerAnim   = useRef(new Animated.Value(0)).current;
  const headerHidden = useRef(false);
  const [headerH, setHeaderH] = useState(0);

  const showHeader = useCallback(() => {
    if (headerHidden.current) {
      headerHidden.current = false;
      Animated.spring(headerAnim, { toValue: 0, useNativeDriver: true, bounciness: 0, speed: 18 }).start();
    }
  }, [headerAnim]);

  const hideHeader = useCallback(() => {
    if (!headerHidden.current && headerH > 0) {
      headerHidden.current = true;
      Animated.spring(headerAnim, { toValue: -headerH, useNativeDriver: true, bounciness: 0, speed: 18 }).start();
    }
  }, [headerAnim, headerH]);

  const activeBgRef = useRef('#0a0a0a');
  const [bgColors, setBgColors] = useState({ active: '#0a0a0a', prev: '#0a0a0a' });
  const bgAnim = useRef(new Animated.Value(1)).current;

  const onViewableItemsChanged = useCallback(({ viewableItems }: any) => {
    if (!viewableItems.length) return;
    const entry: CollectionEntry = viewableItems[0].item;
    const newBg = paletteFor(entry.representativeItem).bg;
    if (newBg === activeBgRef.current) return;
    const prev = activeBgRef.current;
    activeBgRef.current = newBg;
    setBgColors({ active: newBg, prev });
    bgAnim.setValue(0);
    Animated.timing(bgAnim, { toValue: 1, duration: 500, useNativeDriver: false }).start();
  }, [bgAnim]);

  const viewabilityConfig = useRef({ itemVisiblePercentThreshold: 50 }).current;

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
        // Named collection → use collection name; event-grouped → use event name
        name:  entry.collectionName || entry.representativeItem.event_name || entry.representativeItem.name,
        items: entry.items,
      });
    } else {
      setSelected(entry.representativeItem);
    }
  }, []);

  return (
    <View className={s.screen}>
      <View style={[StyleSheet.absoluteFill, { backgroundColor: bgColors.prev }]} />
      <Animated.View style={[StyleSheet.absoluteFill, { backgroundColor: bgColors.active, opacity: bgAnim }]} />

      {/* ── Floating animated header ── */}
      <Animated.View
        style={{
          position: 'absolute', top: 0, left: 0, right: 0, zIndex: 10,
          backgroundColor: 'transparent',
          transform: [{ translateY: headerAnim }],
        }}
        onLayout={(e) => setHeaderH(e.nativeEvent.layout.height)}
      >
        <SafeAreaView edges={['top', 'left', 'right']}>
          <View className={s.header}>
            <Text className={s.headerTitle} style={{ color: '#ffffff' }}>DROPS</Text>
            <View className={s.headerRight}>
              <TouchableOpacity className={s.cartBtn} onPress={() => router.push('/cart' as any)}>
                <Ionicons name="cart-outline" size={26} color="#ffffff" />
              </TouchableOpacity>
            </View>
          </View>

          <View className={s.toggle} style={{ backgroundColor: '#ffffff18' }}>
            <TouchableOpacity
              className={`${s.toggleBtn} ${tab === 'available' ? s.toggleBtnActive : ''}`}
              onPress={() => setTab('available')}
            >
              <Text
                className={`${s.toggleText} ${tab === 'available' ? s.toggleTextActive : ''}`}
                style={{ color: tab === 'available' ? '#fff' : '#ffffff60' }}
              >
                AVAILABLE NOW
              </Text>
            </TouchableOpacity>
            <TouchableOpacity
              className={`${s.toggleBtn} ${tab === 'upcoming' ? s.toggleBtnActive : ''}`}
              onPress={() => setTab('upcoming')}
            >
              <Text
                className={`${s.toggleText} ${tab === 'upcoming' ? s.toggleTextActive : ''}`}
                style={{ color: tab === 'upcoming' ? '#fff' : '#ffffff60' }}
              >
                DROPPING SOON
              </Text>
            </TouchableOpacity>
          </View>
        </SafeAreaView>
      </Animated.View>

      {error && (
        <View className={s.errorBanner} style={{ marginTop: headerH }}>
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
          onViewableItemsChanged={onViewableItemsChanged}
          viewabilityConfig={viewabilityConfig}
          data={visibleCollections}
          keyExtractor={(entry) =>
            `${entry.representativeItem.collection_id ?? entry.representativeItem.event_id ?? entry.representativeItem.id}-${tab}`
          }
          renderItem={({ item: entry }) =>
            entry.representativeItem.sale_status === 'live'
              ? <AvailableCard entry={entry} onPress={() => handleEntryPress(entry)} />
              : <UpcomingCard  entry={entry} onPress={() => handleEntryPress(entry)} />
          }
          ListEmptyComponent={<EmptyState tab={tab} />}
          contentContainerStyle={{ paddingTop: headerH, paddingBottom: 110, flexGrow: 1 }}
          showsVerticalScrollIndicator={false}
          scrollEventThrottle={16}
          onScroll={(e) => {
            const y = e.nativeEvent.contentOffset.y;
            const delta = y - lastScrollY.current;
            lastScrollY.current = y;
            if (y <= 10) showHeader();
            else if (delta > 4) hideHeader();
          }}
          refreshControl={
            <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor="#ff3c6e" />
          }
        />
      )}

      {selected && (
        <DetailModal item={selected} onClose={() => setSelected(null)} />
      )}

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