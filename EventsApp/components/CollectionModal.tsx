import {
  View,
  Text,
  Modal,
  ScrollView,
  TouchableOpacity,
  Animated,
  PanResponder,
  Dimensions,
  StatusBar,
  StyleSheet,
  NativeSyntheticEvent,
  NativeScrollEvent,
  Alert,
} from 'react-native';
// expo-image rather than react-native's Image: the two keep separate caches, so an image already fetched elsewhere in the app was being re-downloaded here. expo-image also persists to disk across restarts.
import { Image as ExpoImage } from 'expo-image';
import { useRef, useState, useCallback, useEffect, useMemo } from 'react';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { type MerchandiseItem, addMerchToCart, getCachedSessionId } from '../lib/api';
import { CartConfirmSheet } from './CartConfirmSheet';
import { useAuthStore } from '../store/authStore';

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

// ─── Variant grouping ─────────────────────────────────────────────────────────
// Variants carry JSONB attributes e.g. {"size":"UK 8"} or {"size":"M","fit":"player"}.
// We group by the primary attribute key so each sizing dimension gets its own row.

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

// ─── Carousel dimensions ──────────────────────────────────────────────────────

const { width: W } = Dimensions.get('window');
const CARD_W    = W * 0.72;
const CARD_GAP  = 14;
const SNAP_W    = CARD_W + CARD_GAP;
const SIDE_PAD  = (W - CARD_W) / 2;
const IMAGE_H   = 260;

// ─── Image carousel (used inside DetailOverlay) ───────────────────────────────

const ImageCarousel = ({ images, tc }: { images: string[]; tc: string }) => {
  const [activeIndex, setActiveIndex] = useState(0);
  const scrollRef = useRef<ScrollView>(null);

  const onMomentumScrollEnd = useCallback((e: NativeSyntheticEvent<NativeScrollEvent>) => {
    const index = Math.round(e.nativeEvent.contentOffset.x / W);
    setActiveIndex(Math.max(0, Math.min(index, images.length - 1)));
  }, [images.length]);

  if (images.length === 0) {
    return (
      <View style={{ width: W, height: IMAGE_H, alignItems: 'center', justifyContent: 'center' }}>
        <Text style={{ fontSize: 48 }}>📦</Text>
      </View>
    );
  }

  return (
    <View>
      <ScrollView
        ref={scrollRef}
        horizontal
        pagingEnabled
        showsHorizontalScrollIndicator={false}
        onMomentumScrollEnd={onMomentumScrollEnd}
        decelerationRate="fast"
        scrollEventThrottle={16}
        style={{ width: W, height: IMAGE_H }}
      >
        {images.map((uri, i) => (
          <ExpoImage
            key={i}
            source={{ uri }}
            style={{ width: W, height: IMAGE_H }}
            contentFit="cover" cachePolicy="memory-disk" transition={150}
          />
        ))}
      </ScrollView>

      {images.length > 1 && (
        <View style={{
          flexDirection: 'row', justifyContent: 'center',
          alignItems: 'center', gap: 5, paddingTop: 10,
        }}>
          {images.map((_, i) => (
            <TouchableOpacity
              key={i}
              onPress={() => {
                scrollRef.current?.scrollTo({ x: i * W, animated: true });
                setActiveIndex(i);
              }}
              hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}
            >
              <View style={{
                width:           i === activeIndex ? 22 : 6,
                height:          4,
                borderRadius:    2,
                backgroundColor: i === activeIndex ? tc : tc + '30',
              }} />
            </TouchableOpacity>
          ))}
        </View>
      )}
    </View>
  );
};

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
    outputRange: [0.91, 1, 0.91],
    extrapolate: 'clamp',
  });

  const opacity = scrollX.interpolate({
    inputRange,
    outputRange: [0.55, 1, 0.55],
    extrapolate: 'clamp',
  });

  const translateY = scrollX.interpolate({
    inputRange,
    outputRange: [14, 0, 14],
    extrapolate: 'clamp',
  });

  // Show up to 4 in-stock size labels as preview chips on the card
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
        style={{ backgroundColor: bg, borderRadius: 20, overflow: 'hidden' }}
      >
        {/* ── Hero image ───────────────────────────────────────────────── */}
        <View style={{ width: '100%', height: CARD_W * 1.0 }}>
          {heroImage ? (
            <ExpoImage
              source={{ uri: heroImage }}
              style={StyleSheet.absoluteFillObject}
              contentFit="cover" cachePolicy="memory-disk" transition={150}
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

          {/* Status / timer pill */}
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

          {/* Bottom scrim */}
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

          {/* Available size preview chips */}
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
  <View style={{ flexDirection: 'row', justifyContent: 'center', alignItems: 'center', marginTop: 18, gap: 7 }}>
    {Array.from({ length: count }).map((_, i) => {
      const scale = scrollX.interpolate({
        inputRange: [(i - 1) * SNAP_W, i * SNAP_W, (i + 1) * SNAP_W],
        outputRange: [0.7, 1.25, 0.7],
        extrapolate: 'clamp',
      });
      const opacity = scrollX.interpolate({
        inputRange: [(i - 1) * SNAP_W, i * SNAP_W, (i + 1) * SNAP_W],
        outputRange: [0.22, 1, 0.22],
        extrapolate: 'clamp',
      });
      return (
        <Animated.View
          key={i}
          style={{
            width: 6,
            height: 6,
            borderRadius: 3,
            backgroundColor: tc,
            opacity,
            transform: [{ scale }],
          }}
        />
      );
    })}
  </View>
);

// ─── Detail overlay ───────────────────────────────────────────────────────────
// Extracted from an inline IIFE into a proper component so it can hold its own
// selectedVariant state without lifting it into CollectionModal.

interface DetailOverlayProps {
  item:        MerchandiseItem;
  detailX:     Animated.Value;
  detailY:     Animated.Value;
  detailOpacity: Animated.Value;
  onClose:     () => void;
  bottom:      number;
  top:         number;
}

const DetailOverlay = ({
  item,
  detailX,
  detailY,
  detailOpacity,
  onClose,
  bottom,
  top,
}: DetailOverlayProps) => {
  const { bg: dBg, text: dtc } = paletteFor(item);
  const imgs = parseImages(item.images);
  const isDetailLive = item.sale_status === 'live';
  const remainSecs   = useTickingSeconds(item.seconds_remain ?? 0);
  const untilSecs    = useTickingSeconds(item.seconds_until  ?? 0);
  const router = useRouter();
  const { user } = useAuthStore();

  // ── Variant / size selection ─────────────────────────────────────────────
  const [selectedVariant, setSelectedVariant] = useState<VariantItem | null>(null);
  const [showCart, setShowCart] = useState(false);

  const handleBuyPress = () => {
    if (!user) {
      Alert.alert(
        'Sign in required',
        'Log in or sign up to add items to your cart.',
        [
          { text: 'Cancel', style: 'cancel' },
          { text: 'Log in / Sign up', onPress: () => { onClose(); router.push('/auth/login' as any); } },
        ],
      );
      return;
    }
    setShowCart(true);
  };
  const variantGroups = useMemo(
    () => (item.variants && item.variants.length > 0 ? groupVariantsByAttr(item.variants) : []),
    [item.variants],
  );
  const hasVariants = variantGroups.length > 0;
  const displayPrice =
    selectedVariant?.price_override != null ? selectedVariant.price_override : item.effective_price;

  // Reset selection whenever the item changes (user swipes to different card)
  useEffect(() => { setSelectedVariant(null); }, [item.id]);

  const buyDisabled = isDetailLive && hasVariants && !selectedVariant;

  const dismissPan = useRef(
    PanResponder.create({
      onMoveShouldSetPanResponder: (_, g) => g.dy > 12 && g.dy > Math.abs(g.dx),
      onPanResponderMove:   (_, g) => { if (g.dy > 0) detailY.setValue(g.dy); },
      onPanResponderRelease: (_, g) => {
        if (g.dy > 80 || g.vy > 0.5) onClose();
        else Animated.spring(detailY, { toValue: 0, useNativeDriver: true }).start();
      },
    })
  ).current;

  return (
    <Animated.View
      style={{
        position: 'absolute', top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: dBg,
        transform: [{ translateX: detailX }, { translateY: detailY }],
        opacity: detailOpacity,
      }}
      {...dismissPan.panHandlers}
    >
      {/* ── Header ──────────────────────────────────────────────────── */}
      <View style={{ paddingTop: top + 12, alignItems: 'center', paddingBottom: 4 }}>
        <View style={{ width: 44, height: 5, borderRadius: 3, backgroundColor: dtc + '55' }} />
        <View style={{
          flexDirection: 'row', alignItems: 'center',
          paddingHorizontal: 20, width: '100%', marginTop: 12,
        }}>
          <Text style={{ fontSize: 10, fontWeight: '800', letterSpacing: 1.5, color: dtc + 'aa' }}>
            {isDetailLive ? '● AVAILABLE NOW' : '◎ DROPPING SOON'}
          </Text>
          <View style={{ flex: 1 }} />
          {isDetailLive && remainSecs > 0 && (
            <Text style={{ fontSize: 10, fontWeight: '800', letterSpacing: 1.5, color: dtc + 'aa' }}>
              ENDS {formatCountdown(remainSecs)}
            </Text>
          )}
          {!isDetailLive && untilSecs > 0 && (
            <Text style={{ fontSize: 10, fontWeight: '800', letterSpacing: 1.5, color: dtc + 'aa' }}>
              IN {formatCountdown(untilSecs)}
            </Text>
          )}
        </View>
      </View>

      {/* ── Scrollable body ─────────────────────────────────────────── */}
      <ScrollView
        showsVerticalScrollIndicator={false}
        contentContainerStyle={{ paddingBottom: 120 }}
      >
        {/* Price box — updates live when a variant is selected */}
        <View style={{
          borderWidth: 1, borderColor: dtc + '40',
          margin: 16, padding: 16, borderRadius: 2,
        }}>
          <Text style={{ fontSize: 10, fontWeight: '800', letterSpacing: 1.5, color: dtc + '90' }}>
            {(item.category ?? 'MERCHANDISE').toUpperCase()}
          </Text>
          <Text style={{ fontSize: 36, fontWeight: '900', color: dtc, marginTop: 4 }}>
            {formatPrice(displayPrice)}
          </Text>
          <Text style={{ fontSize: 11, fontWeight: '500', color: dtc + '90', marginTop: 2 }}>
            {item.name}
          </Text>
        </View>

        {/* Image carousel */}
        <ImageCarousel images={imgs} tc={dtc} />

        {/* Title */}
        <Text style={{
          fontSize: 28, fontWeight: '800', color: dtc,
          paddingHorizontal: 20, paddingTop: 16,
        }}>
          {item.name}
        </Text>

        {/* ── Size / variant selector ─────────────────────────────── */}
        {hasVariants && variantGroups.map((group) => {
          const groupSelected = group.variants.find((v) => v.id === selectedVariant?.id) ?? null;
          return (
            <View
              key={group.key}
              style={{
                paddingHorizontal: 20,
                paddingVertical: 14,
                borderTopWidth: 1,
                borderBottomWidth: 1,
                borderTopColor: dtc + '20',
                borderBottomColor: dtc + '20',
                marginTop: 2,
              }}
            >
              {/* Row header */}
              <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
                <Text style={{ fontSize: 10, fontWeight: '800', letterSpacing: 1.5, color: dtc + '70' }}>
                  {group.label}
                </Text>
                {groupSelected && (
                  <Text style={{ fontSize: 10, fontWeight: '700', letterSpacing: 1, color: dtc + 'bb' }}>
                    {getVariantLabel(groupSelected).toUpperCase()}
                    {groupSelected.stock_quantity > 0 && groupSelected.stock_quantity <= 5
                      ? `  —  ONLY ${groupSelected.stock_quantity} LEFT`
                      : ''}
                  </Text>
                )}
              </View>

              {/* Chips */}
              <ScrollView horizontal showsHorizontalScrollIndicator={false}>
                {group.variants.map((v) => {
                  const label      = getVariantLabel(v);
                  const outOfStock = v.stock_quantity === 0;
                  const lowStock   = !outOfStock && v.stock_quantity <= 5;
                  const isSelected = selectedVariant?.id === v.id;

                  return (
                    <TouchableOpacity
                      key={v.id}
                      onPress={() => setSelectedVariant(isSelected ? null : v)}
                      disabled={outOfStock}
                      style={{
                        borderWidth: isSelected ? 2 : 1,
                        borderColor: outOfStock
                          ? dtc + '20'
                          : isSelected
                          ? dtc
                          : dtc + '50',
                        backgroundColor: isSelected ? dtc + '18' : 'transparent',
                        paddingHorizontal: 14,
                        paddingVertical: 9,
                        borderRadius: 2,
                        marginRight: 8,
                        alignItems: 'center',
                        minWidth: 54,
                      }}
                    >
                      <Text style={{
                        fontSize: 11,
                        fontWeight: isSelected ? '900' : '700',
                        letterSpacing: 0.8,
                        color: outOfStock ? dtc + '28' : isSelected ? dtc : dtc + 'cc',
                      }}>
                        {label.toUpperCase()}
                      </Text>

                      {lowStock && (
                        <Text style={{ fontSize: 7, color: '#ff9500', fontWeight: '800', marginTop: 2, letterSpacing: 0.5 }}>
                          LOW
                        </Text>
                      )}

                      {outOfStock && (
                        <Text style={{ fontSize: 8, color: dtc + '30', fontWeight: '600', marginTop: 2, letterSpacing: 0.5 }}>
                          SOLD OUT
                        </Text>
                      )}

                      {v.price_override != null && (
                        <Text style={{ fontSize: 8, color: dtc + '80', fontWeight: '700', marginTop: 2 }}>
                          {formatPrice(v.price_override)}
                        </Text>
                      )}
                    </TouchableOpacity>
                  );
                })}
              </ScrollView>
            </View>
          );
        })}

        {/* Description */}
        {item.description && (
          <Text style={{
            fontSize: 15, lineHeight: 24, color: dtc + 'cc',
            paddingHorizontal: 20, paddingTop: 12,
          }}>
            {item.description}
          </Text>
        )}
      </ScrollView>

      {/* ── Bottom bar ──────────────────────────────────────────────── */}
      <View style={{
        position: 'absolute', bottom: 0, left: 0, right: 0,
        flexDirection: 'row', alignItems: 'center',
        paddingHorizontal: 20, paddingTop: 14,
        paddingBottom: bottom > 0 ? bottom : 24,
        borderTopWidth: 1, borderTopColor: dtc + '20',
        backgroundColor: dBg, gap: 10,
      }}>
        <TouchableOpacity onPress={onClose}>
          <Text style={{ fontSize: 10, fontWeight: '800', letterSpacing: 1.5, color: dtc + 'aa' }}>
            BACK
          </Text>
        </TouchableOpacity>

        <View style={{ flex: 1, alignItems: 'center' }}>
          <Text numberOfLines={1} style={{ fontSize: 11, fontWeight: '700', color: dtc }}>
            {item.name}
          </Text>
          <Text style={{ fontSize: 10, color: dtc + '80', marginTop: 2 }}>
            {formatPrice(displayPrice)}
          </Text>
        </View>

        <TouchableOpacity
          disabled={buyDisabled}
          onPress={isDetailLive && !buyDisabled ? handleBuyPress : undefined}
          style={{
            borderWidth: 1,
            borderColor: buyDisabled ? dtc + '25' : dtc + '60',
            paddingHorizontal: 18,
            paddingVertical: 8,
            borderRadius: 2,
            opacity: buyDisabled ? 0.45 : 1,
          }}
        >
          <Text style={{ fontSize: 10, fontWeight: '800', letterSpacing: 1.5, color: dtc }}>
            {isDetailLive
              ? buyDisabled ? 'SELECT SIZE' : 'BUY'
              : 'ALERT'}
          </Text>
        </TouchableOpacity>
      </View>

      {/* ── Cart confirmation sheet ──────────────────────────────────────── */}
      {showCart && (
        <CartConfirmSheet
          item={item}
          selectedVariant={selectedVariant}
          onClose={() => setShowCart(false)}
          onAddToCart={async (cartItem, variant, fulfillmentMethod) => {
            try {
              const sid = await getCachedSessionId();
              await addMerchToCart(
                {
                  item_type:              'merchandise',
                  event_id:               cartItem.event_id,
                  merchandise_variant_id: variant?.id,
                  quantity:               1,
                  fulfillment_method:     fulfillmentMethod,
                },
                sid,
              );
            } catch {
              // silently ignore — cart screen will show latest state on next visit
            }
          }}
        />
      )}
    </Animated.View>
  );
};

// ─── CollectionModal ──────────────────────────────────────────────────────────

interface CollectionModalProps {
  collectionName: string;
  description?: string;
  items: MerchandiseItem[];
  onSelectItem: (item: MerchandiseItem) => void;
  onClose: () => void;
}

export const CollectionModal = ({
  collectionName,
  description,
  items,
  onSelectItem,
  onClose,
}: CollectionModalProps) => {
  const { top, bottom } = useSafeAreaInsets();

  const { bg, text: tc } = items.length > 0 ? paletteFor(items[0]) : PALETTE.default;

  // ── Drag-to-dismiss (collection sheet) ───────────────────────────────────
  const translateY = useRef(new Animated.Value(800)).current;
  useEffect(() => {
    Animated.timing(translateY, { toValue: 0, duration: 280, useNativeDriver: true }).start();
  }, []);

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

  // ── Internal detail state + animated values ───────────────────────────────
  const [selectedItem, setSelectedItem] = useState<MerchandiseItem | null>(null);
  const detailX       = useRef(new Animated.Value(400)).current;
  const detailOpacity = useRef(new Animated.Value(0)).current;
  const detailY       = useRef(new Animated.Value(0)).current;

  const openDetail = useCallback((item: MerchandiseItem) => {
    detailX.setValue(400);
    detailY.setValue(0);
    detailOpacity.setValue(0);
    setSelectedItem(item);
    Animated.parallel([
      Animated.spring(detailX,       { toValue: 0, damping: 20, stiffness: 300, useNativeDriver: true }),
      Animated.timing(detailOpacity, { toValue: 1, duration: 100, useNativeDriver: true }),
    ]).start();
  }, []);

  const closeDetail = useCallback(() => {
    Animated.parallel([
      Animated.timing(detailX,       { toValue: 400, duration: 140, useNativeDriver: true }),
      Animated.timing(detailOpacity, { toValue: 0,   duration: 140, useNativeDriver: true }),
    ]).start(() => setSelectedItem(null));
  }, []);

  // ── Carousel tracking ─────────────────────────────────────────────────────
  const scrollX    = useRef(new Animated.Value(0)).current;
  const [activeIndex, setActiveIndex] = useState(0);

  const onMomentumScrollEnd = useCallback((e: any) => {
    const offset = e.nativeEvent.contentOffset.x;
    const index  = Math.round(offset / SNAP_W);
    setActiveIndex(Math.max(0, Math.min(index, items.length - 1)));
  }, [items.length]);

  const activeItem = items[activeIndex] ?? items[0];
  const isLive = activeItem?.sale_status === 'live';

  return (
    <Modal visible transparent animationType="none" onRequestClose={onClose} statusBarTranslucent>
      <StatusBar barStyle={tc === '#ffffff' ? 'light-content' : 'dark-content'} />

      <Animated.View style={{ flex: 1, backgroundColor: bg, transform: [{ translateY }] }}>

        {/* ── Drag handle + collection header ─────────────────────────── */}
        <View
          style={{ paddingTop: top + 8, alignItems: 'center', paddingBottom: 2 }}
          {...pilePan.panHandlers}
        >
          <View style={{
            width: 44, height: 5, borderRadius: 3,
            backgroundColor: tc + '55', marginBottom: 18,
          }} />

          <View style={{
            flexDirection: 'row', alignItems: 'flex-start',
            paddingHorizontal: 20, width: '100%', marginBottom: 16,
          }}>
            <View style={{ flex: 1 }}>
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

          <View style={{ width: '100%', height: 1, backgroundColor: tc + '15' }} />
        </View>

        {/* ── Carousel + description (scrollable) ──────────────────── */}
        <ScrollView
          style={{ flex: 1 }}
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingTop: 8, paddingBottom: 16 }}
          bounces={description ? true : false}
          scrollEnabled={!!description}
        >
          {/* Carousel */}
          <View style={{ justifyContent: 'center' }}>
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
                  onPress={() => openDetail(item)}
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

          {/* Description — only rendered when present */}
          {!!description && (
            <View style={{
              marginHorizontal: 20,
              marginTop: 28,
              paddingTop: 20,
              borderTopWidth: 1,
              borderTopColor: tc + '18',
            }}>
              <Text style={{
                fontSize: 9, fontWeight: '800', letterSpacing: 2,
                color: tc + '45', marginBottom: 10,
              }}>
                ABOUT THIS COLLECTION
              </Text>
              <Text style={{
                fontSize: 15, lineHeight: 24,
                color: tc + 'cc', fontWeight: '400',
              }}>
                {description}
              </Text>
            </View>
          )}
        </ScrollView>

        {/* ── Bottom action bar ─────────────────────────────────────────── */}
        {activeItem && (
          <View style={{
            flexDirection: 'row', alignItems: 'center',
            paddingHorizontal: 20, paddingTop: 14,
            paddingBottom: bottom > 0 ? bottom : 24,
            borderTopWidth: 1, borderTopColor: tc + '20',
            backgroundColor: bg, gap: 10,
          }}>
            <Text style={{
              fontSize: 10, fontWeight: '800', letterSpacing: 1.5,
              color: tc + 'aa', minWidth: 36,
            }}>
              {String(activeIndex + 1).padStart(2, '0')}/{String(items.length).padStart(2, '0')}
            </Text>

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

            <TouchableOpacity
              onPress={() => openDetail(activeItem)}
              style={{
                borderWidth: 1, borderColor: tc + '60',
                paddingHorizontal: 18, paddingVertical: 8, borderRadius: 10,
              }}
            >
              <Text style={{ fontSize: 10, fontWeight: '800', letterSpacing: 1.5, color: tc }}>
                {isLive ? 'VIEW' : 'ALERT'}
              </Text>
            </TouchableOpacity>
          </View>
        )}

        {/* ── Detail overlay ────────────────────────────────────────────── */}
        {selectedItem && (
          <DetailOverlay
            item={selectedItem}
            detailX={detailX}
            detailY={detailY}
            detailOpacity={detailOpacity}
            onClose={closeDetail}
            bottom={bottom}
            top={top}
          />
        )}

      </Animated.View>
    </Modal>
  );
};