import {
  View,
  Text,
  Image,
  TouchableOpacity,
  Animated,
  PanResponder,
  ScrollView,
  Modal,
  StatusBar,
  Dimensions,
  NativeSyntheticEvent,
  NativeScrollEvent,
} from 'react-native';
import { useRef, useEffect, useState, useCallback } from 'react';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { type MerchandiseItem } from '../lib/api';

// ─── Types ────────────────────────────────────────────────────────────────────

type VariantItem = NonNullable<MerchandiseItem['variants']>[number];

interface CartConfirmSheetProps {
  item:            MerchandiseItem;
  selectedVariant: VariantItem | null;
  onClose:         () => void;
  onAddToCart:     (item: MerchandiseItem, variant: VariantItem | null) => void;
}

// ─── Palette ──────────────────────────────────────────────────────────────────

const PALETTE: Record<string, { bg: string; text: string }> = {
  sneakers:    { bg: '#0d0d0d',   text: '#ffffff' },
  apparel:     { bg: '#1a3a5c',   text: '#ffffff' },
  culture:     { bg: '#1a6b3c',   text: '#ffffff' },
  drops:       { bg: '#1a8fe8',   text: '#000000' },
  accessories: { bg: '#4a1a6b',   text: '#ffffff' },
  jersey:      { bg: '#7b1c2a',   text: '#ffffff' },
  footwear:    { bg: '#2d2d2d',   text: '#ffffff' },
  collectible: { bg: '#cc2200',   text: '#ffffff' },
  default:     { bg: '#111111',   text: '#ffffff' },
};

function paletteFor(item: MerchandiseItem) {
  const key = (item.category ?? 'default').toLowerCase();
  return PALETTE[key] ?? PALETTE.default;
}

function formatPrice(price: number) {
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function parseImages(raw?: string): string[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter(Boolean) : [];
  } catch {
    return [];
  }
}

function getVariantLabel(v: VariantItem): string {
  try {
    const attr = JSON.parse(v.attributes) as Record<string, string>;
    return Object.entries(attr)
      .map(([k, val]) => `${k.charAt(0).toUpperCase() + k.slice(1)}: ${val}`)
      .join('  ·  ');
  } catch {
    return v.sku;
  }
}

function getVariantShortLabel(v: VariantItem): string {
  try {
    const attr = JSON.parse(v.attributes) as Record<string, string>;
    return Object.values(attr).join(' / ');
  } catch {
    return v.sku;
  }
}

const { width: W, height: H } = Dimensions.get('window');
const SHEET_HEIGHT = H * 0.82;
const IMAGE_HEIGHT = 260;

// ─── Image carousel ───────────────────────────────────────────────────────────

const ImageCarousel = ({ images, tc }: { images: string[]; tc: string }) => {
  const [activeIndex, setActiveIndex] = useState(0);
  const scrollRef = useRef<ScrollView>(null);

  const onMomentumScrollEnd = useCallback((e: NativeSyntheticEvent<NativeScrollEvent>) => {
    const index = Math.round(e.nativeEvent.contentOffset.x / W);
    setActiveIndex(Math.max(0, Math.min(index, images.length - 1)));
  }, [images.length]);

  if (images.length === 0) {
    return (
      <View style={{ width: W, height: IMAGE_HEIGHT, alignItems: 'center', justifyContent: 'center' }}>
        <Text style={{ fontSize: 48 }}>📦</Text>
      </View>
    );
  }

  return (
    <View>
      {/* Swipeable image strip */}
      <ScrollView
        ref={scrollRef}
        horizontal
        pagingEnabled
        showsHorizontalScrollIndicator={false}
        onMomentumScrollEnd={onMomentumScrollEnd}
        decelerationRate="fast"
        scrollEventThrottle={16}
        style={{ width: W, height: IMAGE_HEIGHT }}
      >
        {images.map((uri, i) => (
          <Image
            key={i}
            source={{ uri }}
            style={{ width: W, height: IMAGE_HEIGHT }}
            resizeMode="cover"
          />
        ))}
      </ScrollView>

      {/* Adidas-style pill indicator — only when multiple images */}
      {images.length > 1 && (
        <View style={{
          flexDirection: 'row',
          justifyContent: 'center',
          alignItems: 'center',
          gap: 5,
          paddingTop: 10,
          paddingBottom: 2,
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

// ─── Success tick animation ───────────────────────────────────────────────────

const SuccessTick = ({ tc }: { tc: string }) => {
  const scale   = useRef(new Animated.Value(0)).current;
  const opacity = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    Animated.parallel([
      Animated.spring(scale,   { toValue: 1, damping: 12, stiffness: 300, useNativeDriver: true }),
      Animated.timing(opacity, { toValue: 1, duration: 80,               useNativeDriver: true }),
    ]).start();
  }, []);

  return (
    <Animated.View style={{ transform: [{ scale }], opacity }}>
      <Text style={{ fontSize: 22, color: tc }}>✓</Text>
    </Animated.View>
  );
};

// ─── Other available sizes strip ──────────────────────────────────────────────

const OtherVariants = ({
  item,
  selectedVariant,
  tc,
}: {
  item:            MerchandiseItem;
  selectedVariant: VariantItem | null;
  tc:              string;
}) => {
  const others = (item.variants ?? [])
    .filter((v) => v.id !== selectedVariant?.id && v.stock_quantity > 0)
    .slice(0, 8);

  if (others.length === 0) return null;

  return (
    <View style={{ marginTop: 22 }}>
      <Text style={{
        fontSize: 9, fontWeight: '800', letterSpacing: 2,
        color: tc + '55', marginBottom: 10, paddingHorizontal: 20,
      }}>
        OTHER SIZES AVAILABLE
      </Text>
      <ScrollView
        horizontal
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={{ paddingHorizontal: 20, gap: 8 }}
      >
        {others.map((v) => {
          const label    = getVariantShortLabel(v);
          const lowStock = v.stock_quantity <= 5;
          return (
            <View
              key={v.id}
              style={{
                borderWidth: 1,
                borderColor: tc + '28',
                paddingHorizontal: 14,
                paddingVertical: 9,
                borderRadius: 2,
                alignItems: 'center',
                minWidth: 52,
              }}
            >
              <Text style={{ fontSize: 11, fontWeight: '700', letterSpacing: 0.8, color: tc + '99' }}>
                {label.toUpperCase()}
              </Text>
              {lowStock && (
                <Text style={{ fontSize: 7, color: '#ff9500', fontWeight: '800', marginTop: 2, letterSpacing: 0.4 }}>
                  LOW
                </Text>
              )}
            </View>
          );
        })}
      </ScrollView>
    </View>
  );
};

// ─── CartConfirmSheet ─────────────────────────────────────────────────────────

export const CartConfirmSheet = ({
  item,
  selectedVariant,
  onClose,
  onAddToCart,
}: CartConfirmSheetProps) => {
  const { bottom } = useSafeAreaInsets();
  const { bg, text: tc } = paletteFor(item);
  const images = parseImages(item.images);

  const displayPrice =
    selectedVariant?.price_override != null
      ? selectedVariant.price_override
      : item.effective_price;

  // ── Sheet entrance/exit animation ─────────────────────────────────────────
  const translateY      = useRef(new Animated.Value(SHEET_HEIGHT)).current;
  const backdropOpacity = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    Animated.parallel([
      Animated.spring(translateY, { toValue: 0, damping: 22, stiffness: 260, useNativeDriver: true }),
      Animated.timing(backdropOpacity, { toValue: 1, duration: 260, useNativeDriver: true }),
    ]).start();
  }, []);

  const dismiss = useCallback(() => {
    Animated.parallel([
      Animated.timing(translateY,      { toValue: SHEET_HEIGHT, duration: 220, useNativeDriver: true }),
      Animated.timing(backdropOpacity, { toValue: 0,            duration: 200, useNativeDriver: true }),
    ]).start(onClose);
  }, [onClose]);

  // ── Drag handle pan ────────────────────────────────────────────────────────
  const dragPan = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => true,
      onMoveShouldSetPanResponder:  (_, g) => g.dy > 5,
      onPanResponderMove:   (_, g) => { if (g.dy > 0) translateY.setValue(g.dy); },
      onPanResponderRelease: (_, g) => {
        if (g.dy > 100 || g.vy > 0.6) {
          dismiss();
        } else {
          Animated.spring(translateY, { toValue: 0, damping: 20, stiffness: 300, useNativeDriver: true }).start();
        }
      },
    })
  ).current;

  // ── Add-to-cart state ──────────────────────────────────────────────────────
  const [added,    setAdded]    = useState(false);
  const btnScale                = useRef(new Animated.Value(1)).current;

  const handleAddToCart = useCallback(() => {
    Animated.sequence([
      Animated.timing(btnScale, { toValue: 0.94, duration: 80,  useNativeDriver: true }),
      Animated.spring(btnScale, { toValue: 1,    damping: 10,   stiffness: 300, useNativeDriver: true }),
    ]).start();

    setAdded(true);
    onAddToCart(item, selectedVariant);
    setTimeout(dismiss, 1100);
  }, [item, selectedVariant, onAddToCart, dismiss]);

  // ── Stock status label (no unit count) ────────────────────────────────────
  const stockLabel =
    !selectedVariant         ? null :
    selectedVariant.stock_quantity === 0  ? { text: 'SOLD OUT', color: '#ff3c6e' } :
    selectedVariant.stock_quantity <= 5   ? { text: 'LOW STOCK', color: '#ff9500' } :
                                            { text: 'IN STOCK',  color: '#34c759' };

  return (
    <Modal visible transparent animationType="none" onRequestClose={dismiss} statusBarTranslucent>
      <StatusBar barStyle="light-content" />

      {/* ── Backdrop ──────────────────────────────────────────────────────── */}
      <Animated.View
        style={{
          position: 'absolute', top: 0, left: 0, right: 0, bottom: 0,
          backgroundColor: '#000000c0',
          opacity: backdropOpacity,
        }}
        onTouchEnd={dismiss}
      />

      {/* ── Sheet ─────────────────────────────────────────────────────────── */}
      <Animated.View
        style={{
          position: 'absolute',
          bottom: 0, left: 0, right: 0,
          height: SHEET_HEIGHT,
          backgroundColor: bg,
          borderTopLeftRadius: 16,
          borderTopRightRadius: 16,
          overflow: 'hidden',
          transform: [{ translateY }],
        }}
      >
        {/* ── Drag handle ─────────────────────────────────────────────────── */}
        <View
          style={{ alignItems: 'center', paddingTop: 14, paddingBottom: 10 }}
          {...dragPan.panHandlers}
        >
          <View style={{ width: 44, height: 5, borderRadius: 3, backgroundColor: tc + '35' }} />
        </View>

        {/* ── Header ──────────────────────────────────────────────────────── */}
        <View style={{
          flexDirection: 'row', alignItems: 'center',
          paddingHorizontal: 20, paddingBottom: 14,
          borderBottomWidth: 1, borderBottomColor: tc + '12',
        }}>
          <Text style={{ fontSize: 10, fontWeight: '900', letterSpacing: 2.5, color: tc + '55', flex: 1 }}>
            CONFIRM ORDER
          </Text>
          <TouchableOpacity onPress={dismiss} hitSlop={{ top: 12, bottom: 12, left: 12, right: 12 }}>
            <Text style={{ fontSize: 17, color: tc + '45', lineHeight: 20 }}>✕</Text>
          </TouchableOpacity>
        </View>

        {/* ── Scrollable content ────────────────────────────────────────────── */}
        <ScrollView
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingBottom: (bottom > 0 ? bottom : 24) + 90 }}
        >
          {/* Image carousel */}
          <ImageCarousel images={images} tc={tc} />

          {/* Product name + price */}
          <View style={{
            paddingHorizontal: 20, paddingTop: 18, paddingBottom: 6,
            flexDirection: 'row', alignItems: 'flex-start', gap: 12,
          }}>
            <View style={{ flex: 1 }}>
              <Text style={{
                fontSize: 9, fontWeight: '800', letterSpacing: 1.8,
                color: tc + '50', marginBottom: 5,
              }}>
                {(item.category ?? 'MERCHANDISE').toUpperCase()}
              </Text>
              <Text numberOfLines={2} style={{
                fontSize: 17, fontWeight: '700', lineHeight: 22, color: tc,
              }}>
                {item.name}
              </Text>
            </View>
            <Text style={{
              fontSize: 22, fontWeight: '900', color: tc,
              letterSpacing: -0.5, paddingTop: 18, flexShrink: 0,
            }}>
              {formatPrice(displayPrice)}
            </Text>
          </View>

          {/* Selected size box */}
          {selectedVariant && (
            <View style={{
              marginHorizontal: 20, marginTop: 12,
              borderWidth: 1, borderColor: tc + '22',
              borderRadius: 3, paddingHorizontal: 16, paddingVertical: 14,
              flexDirection: 'row', alignItems: 'center',
              backgroundColor: tc + '07',
            }}>
              <View style={{ flex: 1 }}>
                <Text style={{
                  fontSize: 9, fontWeight: '800', letterSpacing: 1.8,
                  color: tc + '50', marginBottom: 4,
                }}>
                  SELECTED
                </Text>
                <Text style={{ fontSize: 15, fontWeight: '800', letterSpacing: 0.3, color: tc }}>
                  {getVariantLabel(selectedVariant).toUpperCase()}
                </Text>
              </View>
              {stockLabel && (
                <Text style={{
                  fontSize: 8, fontWeight: '900', letterSpacing: 1,
                  color: stockLabel.color,
                }}>
                  {stockLabel.text}
                </Text>
              )}
            </View>
          )}

          {/* Order summary */}
          <View style={{
            marginHorizontal: 20, marginTop: 22,
            borderTopWidth: 1, borderTopColor: tc + '12', paddingTop: 16,
          }}>
            <Text style={{
              fontSize: 9, fontWeight: '800', letterSpacing: 2,
              color: tc + '50', marginBottom: 12,
            }}>
              ORDER SUMMARY
            </Text>

            {([
              ['Item',     formatPrice(displayPrice)] as const,
              ['Delivery', 'Calculated at checkout']  as const,
              ['Total',    formatPrice(displayPrice)] as const,
            ]).map(([label, value], i, arr) => {
              const isTotal = i === arr.length - 1;
              return (
                <View
                  key={label}
                  style={{
                    flexDirection: 'row', justifyContent: 'space-between',
                    alignItems: 'center', paddingVertical: 9,
                    borderBottomWidth: isTotal ? 0 : 1,
                    borderBottomColor: tc + '0e',
                  }}
                >
                  <Text style={{
                    fontSize: isTotal ? 13 : 11,
                    fontWeight: isTotal ? '800' : '400',
                    color: isTotal ? tc : tc + '65',
                    letterSpacing: 0.2,
                  }}>
                    {label}
                  </Text>
                  <Text style={{
                    fontSize: isTotal ? 16 : 11,
                    fontWeight: isTotal ? '900' : '400',
                    color: isTotal ? tc : tc + '65',
                    letterSpacing: isTotal ? -0.4 : 0,
                  }}>
                    {value}
                  </Text>
                </View>
              );
            })}
          </View>

          {/* Other sizes */}
          <OtherVariants item={item} selectedVariant={selectedVariant} tc={tc} />
        </ScrollView>

        {/* ── Pinned ADD TO CART button ────────────────────────────────────── */}
        <View style={{
          position: 'absolute', bottom: 0, left: 0, right: 0,
          paddingHorizontal: 20, paddingTop: 12,
          paddingBottom: bottom > 0 ? bottom : 24,
          backgroundColor: bg,
          borderTopWidth: 1, borderTopColor: tc + '12',
        }}>
          <Animated.View style={{ transform: [{ scale: btnScale }] }}>
            <TouchableOpacity
              onPress={added ? undefined : handleAddToCart}
              activeOpacity={0.85}
              style={{
                backgroundColor: added ? '#34c759' : tc,
                borderRadius: 3,
                paddingVertical: 17,
                alignItems: 'center',
                justifyContent: 'center',
                flexDirection: 'row',
                gap: 10,
              }}
            >
              {added ? (
                <>
                  <SuccessTick tc={bg} />
                  <Text style={{ fontSize: 12, fontWeight: '900', letterSpacing: 2, color: bg }}>
                    ADDED TO CART
                  </Text>
                </>
              ) : (
                <Text style={{ fontSize: 12, fontWeight: '900', letterSpacing: 2.5, color: bg }}>
                  ADD TO CART  →
                </Text>
              )}
            </TouchableOpacity>
          </Animated.View>
        </View>

      </Animated.View>
    </Modal>
  );
};