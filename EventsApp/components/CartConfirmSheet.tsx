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

// ─── Helpers ──────────────────────────────────────────────────────────────────

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
    const entries = Object.entries(attr);
    return entries.map(([k, val]) =>
      `${k.charAt(0).toUpperCase() + k.slice(1)}: ${val}`
    ).join('  ·  ');
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
const SHEET_HEIGHT = H * 0.72;

// ─── Add-to-cart confirmation feedback ───────────────────────────────────────

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
      <Text style={{ fontSize: 26 }}>✓</Text>
    </Animated.View>
  );
};

// ─── Other available variants row ─────────────────────────────────────────────

const OtherVariants = ({
  item,
  selectedVariant,
  tc,
  bg,
}: {
  item:            MerchandiseItem;
  selectedVariant: VariantItem | null;
  tc:              string;
  bg:              string;
}) => {
  const others = (item.variants ?? [])
    .filter((v) => v.id !== selectedVariant?.id && v.stock_quantity > 0)
    .slice(0, 6);

  if (others.length === 0) return null;

  return (
    <View style={{ marginTop: 20 }}>
      <Text style={{
        fontSize: 9, fontWeight: '800', letterSpacing: 2,
        color: tc + '55', marginBottom: 10, paddingHorizontal: 20,
      }}>
        OTHER SIZES AVAILABLE
      </Text>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: 20 }}>
        {others.map((v) => {
          const label    = getVariantShortLabel(v);
          const lowStock = v.stock_quantity <= 5;
          return (
            <View
              key={v.id}
              style={{
                borderWidth: 1,
                borderColor: tc + '30',
                paddingHorizontal: 12,
                paddingVertical: 7,
                borderRadius: 2,
                marginRight: 8,
                alignItems: 'center',
              }}
            >
              <Text style={{ fontSize: 10, fontWeight: '700', letterSpacing: 0.8, color: tc + 'aa' }}>
                {label.toUpperCase()}
              </Text>
              {lowStock && (
                <Text style={{ fontSize: 7, color: '#ff9500', fontWeight: '800', marginTop: 2 }}>
                  {v.stock_quantity} LEFT
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
  const heroImg = images[0];

  const displayPrice =
    selectedVariant?.price_override != null
      ? selectedVariant.price_override
      : item.effective_price;

  // ── Sheet animation ────────────────────────────────────────────────────────
  const translateY = useRef(new Animated.Value(SHEET_HEIGHT)).current;
  const backdropOpacity = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    Animated.parallel([
      Animated.spring(translateY, {
        toValue: 0,
        damping: 22,
        stiffness: 260,
        useNativeDriver: true,
      }),
      Animated.timing(backdropOpacity, {
        toValue: 1,
        duration: 260,
        useNativeDriver: true,
      }),
    ]).start();
  }, []);

  const dismiss = useCallback(() => {
    Animated.parallel([
      Animated.timing(translateY, { toValue: SHEET_HEIGHT, duration: 220, useNativeDriver: true }),
      Animated.timing(backdropOpacity, { toValue: 0, duration: 200, useNativeDriver: true }),
    ]).start(onClose);
  }, [onClose]);

  // ── Drag to dismiss ────────────────────────────────────────────────────────
  const panResponder = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => true,
      onMoveShouldSetPanResponder:  (_, g) => g.dy > 5,
      onPanResponderMove: (_, g) => {
        if (g.dy > 0) translateY.setValue(g.dy);
      },
      onPanResponderRelease: (_, g) => {
        if (g.dy > 100 || g.vy > 0.6) {
          dismiss();
        } else {
          Animated.spring(translateY, { toValue: 0, damping: 20, stiffness: 300, useNativeDriver: true }).start();
        }
      },
    })
  ).current;

  // ── Add to cart state ──────────────────────────────────────────────────────
  const [added, setAdded] = useState(false);
  const btnScale = useRef(new Animated.Value(1)).current;

  const handleAddToCart = useCallback(() => {
    // Haptic press-down animation
    Animated.sequence([
      Animated.timing(btnScale, { toValue: 0.94, duration: 80, useNativeDriver: true }),
      Animated.spring(btnScale,  { toValue: 1,    damping: 10, stiffness: 300, useNativeDriver: true }),
    ]).start();

    setAdded(true);
    onAddToCart(item, selectedVariant);

    // Auto-dismiss after brief confirmation
    setTimeout(() => {
      dismiss();
    }, 1100);
  }, [item, selectedVariant, onAddToCart, dismiss]);

  return (
    <Modal
      visible
      transparent
      animationType="none"
      onRequestClose={dismiss}
      statusBarTranslucent
    >
      <StatusBar barStyle="light-content" />

      {/* ── Backdrop ──────────────────────────────────────────────────────── */}
      <Animated.View
        style={{
          position: 'absolute', top: 0, left: 0, right: 0, bottom: 0,
          backgroundColor: '#000000cc',
          opacity: backdropOpacity,
        }}
        onTouchEnd={dismiss}
      />

      {/* ── Sheet ─────────────────────────────────────────────────────────── */}
      <Animated.View
        style={{
          position: 'absolute',
          bottom: 0,
          left: 0,
          right: 0,
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
          {...panResponder.panHandlers}
        >
          <View style={{
            width: 44, height: 5, borderRadius: 3,
            backgroundColor: tc + '40',
          }} />
        </View>

        {/* ── Header row ──────────────────────────────────────────────────── */}
        <View style={{
          flexDirection: 'row',
          alignItems: 'center',
          paddingHorizontal: 20,
          paddingBottom: 14,
          borderBottomWidth: 1,
          borderBottomColor: tc + '15',
        }}>
          <Text style={{
            fontSize: 10, fontWeight: '900', letterSpacing: 2.5,
            color: tc + '60', flex: 1,
          }}>
            CONFIRM ORDER
          </Text>
          <TouchableOpacity onPress={dismiss} hitSlop={{ top: 10, bottom: 10, left: 10, right: 10 }}>
            <Text style={{ fontSize: 18, color: tc + '50', lineHeight: 20 }}>✕</Text>
          </TouchableOpacity>
        </View>

        <ScrollView
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingBottom: (bottom > 0 ? bottom : 24) + 100 }}
        >
          {/* ── Product row ───────────────────────────────────────────────── */}
          <View style={{
            flexDirection: 'row',
            alignItems: 'flex-start',
            paddingHorizontal: 20,
            paddingVertical: 18,
            gap: 14,
          }}>
            {/* Thumbnail */}
            <View style={{
              width: 88, height: 88,
              borderRadius: 4,
              overflow: 'hidden',
              backgroundColor: tc + '10',
              flexShrink: 0,
            }}>
              {heroImg ? (
                <Image
                  source={{ uri: heroImg }}
                  style={{ width: '100%', height: '100%' }}
                  resizeMode="cover"
                />
              ) : (
                <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center' }}>
                  <Text style={{ fontSize: 28 }}>📦</Text>
                </View>
              )}
            </View>

            {/* Name + meta */}
            <View style={{ flex: 1 }}>
              <Text style={{
                fontSize: 9, fontWeight: '800', letterSpacing: 1.8,
                color: tc + '55', marginBottom: 5,
              }}>
                {(item.category ?? 'MERCHANDISE').toUpperCase()}
              </Text>
              <Text
                numberOfLines={3}
                style={{
                  fontSize: 15, fontWeight: '700', lineHeight: 20,
                  color: tc, marginBottom: 8,
                }}
              >
                {item.name}
              </Text>
              <Text style={{ fontSize: 22, fontWeight: '900', color: tc, letterSpacing: -0.5 }}>
                {formatPrice(displayPrice)}
              </Text>
            </View>
          </View>

          {/* ── Selected size / variant ────────────────────────────────────── */}
          {selectedVariant && (
            <View style={{
              marginHorizontal: 20,
              borderWidth: 1,
              borderColor: tc + '25',
              borderRadius: 3,
              paddingHorizontal: 16,
              paddingVertical: 14,
              flexDirection: 'row',
              alignItems: 'center',
              backgroundColor: tc + '08',
            }}>
              <View style={{ flex: 1 }}>
                <Text style={{
                  fontSize: 9, fontWeight: '800', letterSpacing: 1.8,
                  color: tc + '55', marginBottom: 4,
                }}>
                  SELECTED
                </Text>
                <Text style={{
                  fontSize: 15, fontWeight: '800', letterSpacing: 0.3,
                  color: tc,
                }}>
                  {getVariantLabel(selectedVariant).toUpperCase()}
                </Text>
              </View>

              {/* Stock indicator */}
              <View style={{ alignItems: 'flex-end' }}>
                {selectedVariant.stock_quantity <= 5 ? (
                  <>
                    <Text style={{ fontSize: 8, fontWeight: '900', color: '#ff9500', letterSpacing: 1 }}>
                      LOW STOCK
                    </Text>
                    <Text style={{ fontSize: 13, fontWeight: '700', color: '#ff9500', marginTop: 2 }}>
                      {selectedVariant.stock_quantity} left
                    </Text>
                  </>
                ) : (
                  <>
                    <Text style={{ fontSize: 8, fontWeight: '900', color: '#34c759', letterSpacing: 1 }}>
                      IN STOCK
                    </Text>
                    <Text style={{ fontSize: 13, fontWeight: '700', color: '#34c759', marginTop: 2 }}>
                      {selectedVariant.stock_quantity} units
                    </Text>
                  </>
                )}
              </View>
            </View>
          )}

          {/* ── Divider + order summary ────────────────────────────────────── */}
          <View style={{
            marginHorizontal: 20,
            marginTop: 20,
            borderTopWidth: 1,
            borderTopColor: tc + '15',
            paddingTop: 16,
          }}>
            <Text style={{
              fontSize: 9, fontWeight: '800', letterSpacing: 2,
              color: tc + '55', marginBottom: 12,
            }}>
              ORDER SUMMARY
            </Text>

            {[
              ['Item',     formatPrice(displayPrice)],
              ['Delivery', 'Calculated at checkout'],
              ['Total',    formatPrice(displayPrice)],
            ].map(([label, value], i, arr) => (
              <View
                key={label}
                style={{
                  flexDirection: 'row',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  paddingVertical: 8,
                  borderBottomWidth: i < arr.length - 1 ? 1 : 0,
                  borderBottomColor: tc + '10',
                }}
              >
                <Text style={{
                  fontSize: i === arr.length - 1 ? 13 : 11,
                  fontWeight: i === arr.length - 1 ? '800' : '500',
                  color: tc + (i === arr.length - 1 ? 'ff' : '70'),
                  letterSpacing: 0.3,
                }}>
                  {label}
                </Text>
                <Text style={{
                  fontSize: i === arr.length - 1 ? 15 : 11,
                  fontWeight: i === arr.length - 1 ? '900' : '500',
                  color: tc + (i === arr.length - 1 ? 'ff' : '70'),
                  letterSpacing: i === arr.length - 1 ? -0.3 : 0,
                }}>
                  {value}
                </Text>
              </View>
            ))}
          </View>

          {/* ── Other available sizes ──────────────────────────────────────── */}
          <OtherVariants
            item={item}
            selectedVariant={selectedVariant}
            tc={tc}
            bg={bg}
          />
        </ScrollView>

        {/* ── Add to Cart CTA — pinned to bottom ────────────────────────────── */}
        <View style={{
          position: 'absolute',
          bottom: 0, left: 0, right: 0,
          paddingHorizontal: 20,
          paddingTop: 14,
          paddingBottom: bottom > 0 ? bottom : 24,
          backgroundColor: bg,
          borderTopWidth: 1,
          borderTopColor: tc + '15',
        }}>
          <Animated.View style={{ transform: [{ scale: btnScale }] }}>
            <TouchableOpacity
              onPress={added ? undefined : handleAddToCart}
              activeOpacity={0.85}
              style={{
                backgroundColor: added ? '#34c759' : tc,
                borderRadius: 3,
                paddingVertical: 16,
                alignItems: 'center',
                justifyContent: 'center',
                flexDirection: 'row',
                gap: 10,
              }}
            >
              {added ? (
                <>
                  <SuccessTick tc={bg} />
                  <Text style={{
                    fontSize: 12, fontWeight: '900', letterSpacing: 2,
                    color: bg,
                  }}>
                    ADDED TO CART
                  </Text>
                </>
              ) : (
                <Text style={{
                  fontSize: 12, fontWeight: '900', letterSpacing: 2.5,
                  color: bg,
                }}>
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