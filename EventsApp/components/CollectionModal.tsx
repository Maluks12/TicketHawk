import {
  View,
  Text,
  Modal,
  ScrollView,
  Image,
  TouchableOpacity,
  Animated,
  PanResponder,
  Dimensions,
  StatusBar,
  StyleSheet,
} from 'react-native';
import { useRef, useState, useCallback, useEffect } from 'react';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { type MerchandiseItem } from '../lib/api';

// ─── Palette (mirrors merch.tsx — single source of truth for colours) ─────────

const PALETTE: Record<string, { bg: string; text: string }> = {
  sneakers:    { bg: '#0d0d0d', text: '#ffffff' },
  apparel:     { bg: '#1a3a5c', text: '#ffffff' },
  culture:     { bg: '#1a6b3c', text: '#ffffff' },
  drops:       { bg: '#1a8fe8', text: '#000000' },
  accessories: { bg: '#4a1a6b', text: '#ffffff' },
  jersey:      { bg: '#7b1c2a', text: '#ffffff' },
  footwear:    { bg: '#2d2d2d', text: '#ffffff' },
  collectible: { bg: '#cc2200', text: '#ffffff' },
  default:     { bg: '#1a1a1a', text: '#ffffff' },
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

function formatCountdown(totalSecs: number): string {
  const d = Math.floor(totalSecs / 86400);
  const h = Math.floor((totalSecs % 86400) / 3600);
  const m = Math.floor((totalSecs % 3600) / 60);
  const sec = totalSecs % 60;
  if (d > 0) return `${d}D  ${String(h).padStart(2, '0')}H  ${String(m).padStart(2, '0')}M`;
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`;
}

function useTickingSeconds(initial: number) {
  const [secs, setSecs] = useState(initial);
  useEffect(() => {
    if (secs <= 0) return;
    const t = setInterval(() => setSecs((s) => Math.max(0, s - 1)), 1000);
    return () => clearInterval(t);
  }, []);
  return secs;
}

// ─── Carousel dimensions ──────────────────────────────────────────────────────

const { width: W } = Dimensions.get('window');
const CARD_W   = W * 0.64;
const CARD_GAP = 12;
const SNAP_W   = CARD_W + CARD_GAP;
const SIDE_PAD = (W - CARD_W) / 2;

// ─── Carousel Card ────────────────────────────────────────────────────────────

const CarouselCard = ({
  item,
  index,
  scrollX,
  onPress,
}: {
  item: MerchandiseItem;
  index: number;
  scrollX: Animated.Value;
  onPress: () => void;
}) => {
  const { bg, text: tc } = paletteFor(item);
  const images = parseImages(item.images);
  const heroImage = images[0];
  const isLive = item.sale_status === 'live';
  const remainSecs = useTickingSeconds(item.seconds_remain ?? 0);
  const untilSecs  = useTickingSeconds(item.seconds_until ?? 0);

  const inputRange = [
    (index - 1) * SNAP_W,
    index       * SNAP_W,
    (index + 1) * SNAP_W,
  ];

  const scale = scrollX.interpolate({
    inputRange,
    outputRange: [0.86, 1, 0.86],
    extrapolate: 'clamp',
  });

  const opacity = scrollX.interpolate({
    inputRange,
    outputRange: [0.5, 1, 0.5],
    extrapolate: 'clamp',
  });

  const translateY = scrollX.interpolate({
    inputRange,
    outputRange: [20, 0, 20],
    extrapolate: 'clamp',
  });

  // Variant label for the side strip
  let variantLabels: string[] = [];
  if (item.variants && item.variants.length > 0) {
    variantLabels = item.variants
      .filter((v) => v.stock_quantity > 0)
      .slice(0, 4)
      .map((v) => {
        try {
          const attr = JSON.parse(v.attributes);
          return Object.values(attr).join('/');
        } catch {
          return v.sku;
        }
      });
  }

  return (
    <Animated.View
      style={{
        width: CARD_W,
        marginHorizontal: CARD_GAP / 2,
        transform: [{ scale }, { translateY }],
        opacity,
      }}
    >
      <TouchableOpacity
        activeOpacity={0.93}
        onPress={onPress}
        style={{ backgroundColor: bg, borderRadius: 3, overflow: 'hidden' }}
      >
        {/* ── Hero image ───────────────────────────────────────────────── */}
        <View style={{ width: '100%', height: CARD_W * 1.0 }}>
          {heroImage ? (
            <Image
              source={{ uri: heroImage }}
              style={StyleSheet.absoluteFillObject}
              resizeMode="cover"
            />
          ) : (
            <View
              style={[StyleSheet.absoluteFillObject, {
                backgroundColor: tc + '10',
                alignItems: 'center',
                justifyContent: 'center',
              }]}
            >
              <Text style={{ fontSize: 40 }}>📦</Text>
            </View>
          )}

          {/* Status/timer pill — top left, same pill style as feed cards */}
          <View
            style={{
              position: 'absolute',
              top: 10,
              left: 10,
              borderWidth: 1,
              borderColor: isLive
                ? (item.seconds_remain != null && remainSecs < 86400 ? '#ff3c6e' : tc + '40')
                : tc + '50',
              paddingHorizontal: 8,
              paddingVertical: 4,
              borderRadius: 2,
              backgroundColor: bg + 'd0',
            }}
          >
            <Text
              style={{
                fontSize: 7,
                fontWeight: '800',
                letterSpacing: 1.4,
                color: isLive
                  ? (item.seconds_remain != null && remainSecs < 86400 ? '#ff3c6e' : tc + '90')
                  : tc + '80',
              }}
            >
              {isLive ? 'ENDS IN' : 'DROPS IN'}
            </Text>
            <Text
              style={{
                fontSize: 11,
                fontWeight: '900',
                letterSpacing: 0.5,
                color: isLive
                  ? (item.seconds_remain != null && remainSecs < 86400 ? '#ff3c6e' : tc)
                  : tc,
                marginTop: 1,
              }}
            >
              {isLive
                ? (item.seconds_remain != null ? formatCountdown(remainSecs) : 'NOW')
                : (item.seconds_until  != null ? formatCountdown(untilSecs)  : 'TBC')}
            </Text>
          </View>

          {/* Gradient scrim at bottom of image for text legibility */}
          <View
            style={{
              position: 'absolute',
              bottom: 0,
              left: 0,
              right: 0,
              height: 60,
              backgroundColor: bg,
              opacity: 0.6,
            }}
          />
        </View>

        {/* ── Text + CTA ───────────────────────────────────────────────── */}
        <View style={{ padding: 14, paddingTop: 12 }}>
          <Text
            style={{
              fontSize: 9,
              fontWeight: '800',
              letterSpacing: 1.5,
              color: tc + '70',
              marginBottom: 4,
            }}
          >
            {(item.category ?? 'DROP').toUpperCase()}
          </Text>

          <Text
            numberOfLines={2}
            style={{
              fontSize: 15,
              fontWeight: '700',
              lineHeight: 20,
              color: tc,
              marginBottom: 12,
            }}
          >
            {item.name}
          </Text>

          {/* Available sizes strip */}
          {variantLabels.length > 0 && (
            <View style={{ flexDirection: 'row', gap: 5, marginBottom: 12, flexWrap: 'wrap' }}>
              {variantLabels.map((lbl, i) => (
                <View
                  key={i}
                  style={{
                    borderWidth: 1,
                    borderColor: tc + '35',
                    paddingHorizontal: 7,
                    paddingVertical: 3,
                    borderRadius: 2,
                  }}
                >
                  <Text style={{ fontSize: 8, fontWeight: '700', letterSpacing: 0.8, color: tc + 'aa' }}>
                    {String(lbl).toUpperCase()}
                  </Text>
                </View>
              ))}
            </View>
          )}

          {/* Price row + CTA */}
          <View style={{ flexDirection: 'row', alignItems: 'center' }}>
            <Text
              style={{
                fontSize: 20,
                fontWeight: '900',
                color: tc,
                flex: 1,
                letterSpacing: -0.5,
              }}
            >
              {formatPrice(item.effective_price)}
            </Text>
            <View
              style={{
                borderWidth: 1,
                borderColor: tc + '60',
                paddingHorizontal: 13,
                paddingVertical: 7,
                borderRadius: 2,
              }}
            >
              <Text style={{ fontSize: 9, fontWeight: '800', letterSpacing: 1.5, color: tc }}>
                {isLive ? 'BUY' : 'ALERT'}
              </Text>
            </View>
          </View>
        </View>
      </TouchableOpacity>
    </Animated.View>
  );
};

// ─── Pagination dots ──────────────────────────────────────────────────────────

const PaginationDots = ({
  count,
  scrollX,
  tc,
}: {
  count: number;
  scrollX: Animated.Value;
  tc: string;
}) => (
  <View style={{ flexDirection: 'row', justifyContent: 'center', alignItems: 'center', marginTop: 16, gap: 5 }}>
    {Array.from({ length: count }).map((_, i) => {
      const width = scrollX.interpolate({
        inputRange: [(i - 1) * SNAP_W, i * SNAP_W, (i + 1) * SNAP_W],
        outputRange: [5, 22, 5],
        extrapolate: 'clamp',
      });
      const opacity = scrollX.interpolate({
        inputRange: [(i - 1) * SNAP_W, i * SNAP_W, (i + 1) * SNAP_W],
        outputRange: [0.25, 1, 0.25],
        extrapolate: 'clamp',
      });
      return (
        <Animated.View
          key={i}
          style={{ height: 2, width, opacity, borderRadius: 1, backgroundColor: tc }}
        />
      );
    })}
  </View>
);

// ─── CollectionModal ──────────────────────────────────────────────────────────

interface CollectionModalProps {
  collectionName: string;
  items: MerchandiseItem[];
  onSelectItem: (item: MerchandiseItem) => void;
  onClose: () => void;
}

export const CollectionModal = ({
  collectionName,
  items,
  onSelectItem,
  onClose,
}: CollectionModalProps) => {
  const { top, bottom } = useSafeAreaInsets();

  // Derive overall modal tone from the first item's category
  const { bg, text: tc } = items.length > 0 ? paletteFor(items[0]) : PALETTE.default;

  // Drag-to-dismiss — same logic as DetailModal
  const translateY = useRef(new Animated.Value(0)).current;

  const pilePan = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => true,
      onMoveShouldSetPanResponder: (_, g) => Math.abs(g.dy) > 4,
      onPanResponderMove: (_, g) => { if (g.dy > 0) translateY.setValue(g.dy); },
      onPanResponderRelease: (_, g) => {
        if (g.dy > 80 || g.vy > 0.5) {
          Animated.timing(translateY, {
            toValue: 800, duration: 220, useNativeDriver: true,
          }).start(onClose);
        } else {
          Animated.spring(translateY, { toValue: 0, useNativeDriver: true }).start();
        }
      },
    })
  ).current;

  // Carousel tracking
  const scrollX   = useRef(new Animated.Value(0)).current;
  const [activeIndex, setActiveIndex] = useState(0);

  const onMomentumScrollEnd = useCallback((e: any) => {
    const offset = e.nativeEvent.contentOffset.x;
    const index  = Math.round(offset / SNAP_W);
    setActiveIndex(Math.max(0, Math.min(index, items.length - 1)));
  }, [items.length]);

  const activeItem = items[activeIndex] ?? items[0];
  const isLive = activeItem?.sale_status === 'live';

  return (
    <Modal visible animationType="slide" onRequestClose={onClose} statusBarTranslucent>
      <StatusBar barStyle={tc === '#ffffff' ? 'light-content' : 'dark-content'} />

      <Animated.View style={{ flex: 1, backgroundColor: bg, transform: [{ translateY }] }}>

        {/* ── Drag handle + collection header ─────────────────────────── */}
        <View
          style={{ paddingTop: top + 8, alignItems: 'center', paddingBottom: 2 }}
          {...pilePan.panHandlers}
        >
          {/* Pill */}
          <View style={{
            width: 44, height: 5, borderRadius: 3,
            backgroundColor: tc + '55', marginBottom: 18,
          }} />

          {/* Header row */}
          <View style={{
            flexDirection: 'row', alignItems: 'flex-start',
            paddingHorizontal: 20, width: '100%', marginBottom: 16,
          }}>
            <View style={{ flex: 1 }}>
              {/* "COLLECTION" eyebrow — same tracking style as card category labels */}
              <Text style={{
                fontSize: 9, fontWeight: '800', letterSpacing: 2,
                color: tc + '55', marginBottom: 4,
              }}>
                COLLECTION
              </Text>
              <Text
                numberOfLines={2}
                style={{
                  fontSize: 24, fontWeight: '900', letterSpacing: 0.4,
                  color: tc, lineHeight: 28,
                }}
              >
                {collectionName.toUpperCase()}
              </Text>
            </View>

            {/* Item count badge — same border/radius as expiry badge on feed cards */}
            <View style={{
              borderWidth: 1, borderColor: tc + '40',
              paddingHorizontal: 12, paddingVertical: 8,
              borderRadius: 2, alignItems: 'center', marginLeft: 12, marginTop: 4,
            }}>
              <Text style={{ fontSize: 20, fontWeight: '900', color: tc, lineHeight: 22 }}>
                {items.length}
              </Text>
              <Text style={{ fontSize: 8, fontWeight: '700', letterSpacing: 1.2, color: tc + '55' }}>
                ITEMS
              </Text>
            </View>
          </View>

          {/* Thin rule — same tone as card border dividers */}
          <View style={{ width: '100%', height: 1, backgroundColor: tc + '15' }} />
        </View>

        {/* ── Carousel ─────────────────────────────────────────────────── */}
        <View style={{ flex: 1, justifyContent: 'center', paddingTop: 8 }}>
          <Animated.ScrollView
            horizontal
            showsHorizontalScrollIndicator={false}
            snapToInterval={SNAP_W}
            decelerationRate="fast"
            contentContainerStyle={{
              paddingHorizontal: SIDE_PAD - CARD_GAP / 2,
              alignItems: 'center',
            }}
            onScroll={Animated.event(
              [{ nativeEvent: { contentOffset: { x: scrollX } } }],
              { useNativeDriver: false }
            )}
            scrollEventThrottle={16}
            onMomentumScrollEnd={onMomentumScrollEnd}
          >
            {items.map((item, index) => (
              <CarouselCard
                key={item.id}
                item={item}
                index={index}
                scrollX={scrollX}
                onPress={() => onSelectItem(item)}
              />
            ))}
          </Animated.ScrollView>

          <PaginationDots count={items.length} scrollX={scrollX} tc={tc} />

          <Text style={{
            textAlign: 'center', marginTop: 10,
            fontSize: 8, letterSpacing: 2.5, color: tc + '30', fontWeight: '700',
          }}>
            SWIPE TO EXPLORE
          </Text>
        </View>

        {/* ── Bottom action bar — mirrors DetailModal bottom bar exactly ── */}
        {activeItem && (
          <View style={{
            flexDirection: 'row', alignItems: 'center',
            paddingHorizontal: 20, paddingTop: 14,
            paddingBottom: bottom > 0 ? bottom : 24,
            borderTopWidth: 1, borderTopColor: tc + '20',
            backgroundColor: bg, gap: 10,
          }}>
            {/* Index counter */}
            <Text style={{
              fontSize: 10, fontWeight: '800', letterSpacing: 1.5,
              color: tc + 'aa', minWidth: 36,
            }}>
              {String(activeIndex + 1).padStart(2, '0')}/{String(items.length).padStart(2, '0')}
            </Text>

            {/* Name + price — same layout as modalBottomCenter */}
            <View style={{ flex: 1, alignItems: 'center' }}>
              <Text
                numberOfLines={1}
                style={{ fontSize: 11, fontWeight: '700', letterSpacing: 0.5, color: tc }}
              >
                {activeItem.name}
              </Text>
              <Text style={{ fontSize: 10, fontWeight: '500', color: tc + '80', marginTop: 2 }}>
                {formatPrice(activeItem.effective_price)}
              </Text>
            </View>

            {/* CTA — same as buyNowBtn */}
            <TouchableOpacity
              onPress={() => onSelectItem(activeItem)}
              style={{
                borderWidth: 1, borderColor: tc + '60',
                paddingHorizontal: 18, paddingVertical: 8, borderRadius: 2,
              }}
            >
              <Text style={{ fontSize: 10, fontWeight: '800', letterSpacing: 1.5, color: tc }}>
                {isLive ? 'BUY' : 'ALERT'}
              </Text>
            </TouchableOpacity>
          </View>
        )}
      </Animated.View>
    </Modal>
  );
};
