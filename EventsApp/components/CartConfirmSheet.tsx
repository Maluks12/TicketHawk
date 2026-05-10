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
  TextInput,
  KeyboardAvoidingView,
  Platform,
  Alert,
  LayoutAnimation,
  UIManager,
} from 'react-native';
import { useRef, useEffect, useState, useCallback } from 'react';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { type MerchandiseItem } from '../lib/api';

if (Platform.OS === 'android' && UIManager.setLayoutAnimationEnabledExperimental) {
  UIManager.setLayoutAnimationEnabledExperimental(true);
}

// ─── Types ────────────────────────────────────────────────────────────────────

type VariantItem = NonNullable<MerchandiseItem['variants']>[number];

export interface DeliveryAddress {
  street:      string;
  suburb:      string;
  city:        string;
  province:    string;
  postal_code: string;
}

export type FulfillmentMethod = 'pickup' | 'delivery';

interface CartConfirmSheetProps {
  item:            MerchandiseItem;
  selectedVariant: VariantItem | null;
  onClose:         () => void;
  onAddToCart: (
    item:              MerchandiseItem,
    variant:           VariantItem | null,
    fulfillmentMethod: FulfillmentMethod,
    deliveryAddress?:  DeliveryAddress,
  ) => void;
}

// ─── Palette ──────────────────────────────────────────────────────────────────

const PALETTE: Record<string, { bg: string; text: string }> = {
  sneakers:    { bg: '#0d0d0d', text: '#ffffff' },
  apparel:     { bg: '#1a3a5c', text: '#ffffff' },
  culture:     { bg: '#1a6b3c', text: '#ffffff' },
  drops:       { bg: '#1a8fe8', text: '#000000' },
  accessories: { bg: '#4a1a6b', text: '#ffffff' },
  jersey:      { bg: '#7b1c2a', text: '#ffffff' },
  footwear:    { bg: '#2d2d2d', text: '#ffffff' },
  collectible: { bg: '#cc2200', text: '#ffffff' },
  default:     { bg: '#111111', text: '#ffffff' },
};

function paletteFor(item: MerchandiseItem) {
  const key = (item.category ?? 'default').toLowerCase();
  return PALETTE[key] ?? PALETTE.default;
}

function fmtPrice(price: number) {
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
    const attrs = JSON.parse(v.attributes);
    return Object.values(attrs as Record<string, string>).join(' / ') || v.sku;
  } catch {
    return v.sku;
  }
}

const { height: SCREEN_H, width: SCREEN_W } = Dimensions.get('window');
const SHEET_HEIGHT = Math.round(SCREEN_H * 0.88);

// ─── Image carousel ───────────────────────────────────────────────────────────

function ImageCarousel({ images, tc }: { images: string[]; tc: string }) {
  const [page, setPage] = useState(0);
  const IMG_H = Math.round(SCREEN_W * 0.48);
  if (!images.length) return null;
  return (
    <View style={{ height: IMG_H }}>
      <ScrollView
        horizontal pagingEnabled showsHorizontalScrollIndicator={false}
        onScroll={(e) => setPage(Math.round(e.nativeEvent.contentOffset.x / SCREEN_W))}
        scrollEventThrottle={16} decelerationRate="fast" style={{ height: IMG_H }}
      >
        {images.map((uri, i) => (
          <Image key={i} source={{ uri }} style={{ width: SCREEN_W, height: IMG_H }} resizeMode="cover" />
        ))}
      </ScrollView>
      {images.length > 1 && (
        <View style={{ position: 'absolute', bottom: 8, left: 0, right: 0, flexDirection: 'row', justifyContent: 'center', gap: 5 }}>
          {images.map((_, i) => (
            <View key={i} style={{ width: i === page ? 14 : 5, height: 5, borderRadius: 3, backgroundColor: i === page ? tc : tc + '40' }} />
          ))}
        </View>
      )}
    </View>
  );
}

// ─── Other variants ───────────────────────────────────────────────────────────

function OtherVariants({ item, selectedVariant, tc }: { item: MerchandiseItem; selectedVariant: VariantItem | null; tc: string }) {
  const others = (item.variants ?? []).filter((v) => v.id !== selectedVariant?.id && v.is_active);
  if (!others.length) return null;
  return (
    <View style={{ marginHorizontal: 20, marginTop: 20 }}>
      <Text style={{ fontSize: 9, fontWeight: '800', letterSpacing: 2, color: tc + '50', marginBottom: 10 }}>OTHER SIZES AVAILABLE</Text>
      <ScrollView horizontal showsHorizontalScrollIndicator={false}>
        {others.map((v) => (
          <View key={v.id} style={{ borderWidth: 1, borderColor: tc + '22', borderRadius: 3, paddingHorizontal: 14, paddingVertical: 10, marginRight: 8, opacity: v.stock_quantity === 0 ? 0.38 : 1, backgroundColor: tc + '07' }}>
            <Text style={{ fontSize: 11, fontWeight: '700', letterSpacing: 0.5, color: tc }}>{getVariantLabel(v).toUpperCase()}</Text>
          </View>
        ))}
      </ScrollView>
    </View>
  );
}

// ─── Fulfillment section — inline dropdown, no nested Modal ──────────────────

function FulfillmentSection({
  item, method, onMethod, address, onAddress, tc, bg,
}: {
  item: MerchandiseItem; method: FulfillmentMethod; onMethod: (m: FulfillmentMethod) => void;
  address: DeliveryAddress; onAddress: (a: DeliveryAddress) => void; tc: string; bg: string;
}) {
  const mode       = (item.fulfillment_mode ?? 'delivery') as string;
  const canPickup  = mode === 'pickup'   || mode === 'both';
  const canDeliver = mode === 'delivery' || mode === 'both';
  const canChoose  = mode === 'both';
  const deliveryFee = item.delivery_fee ?? 0;
  const pickupLoc   = item.pickup_location;

  const [open, setOpen] = useState(false);

  const toggle = () => {
    if (!canChoose) return;
    LayoutAnimation.configureNext(LayoutAnimation.Presets.easeInEaseOut);
    setOpen((o) => !o);
  };

  const pick = (m: FulfillmentMethod) => {
    LayoutAnimation.configureNext(LayoutAnimation.Presets.easeInEaseOut);
    onMethod(m);
    setOpen(false);
  };

  const methodLabel = method === 'pickup'
    ? `\u{1F3EA}  Pickup${pickupLoc ? ` \u00B7 ${pickupLoc}` : ''}`
    : `\u{1F4E6}  Delivery${deliveryFee > 0 ? ` \u00B7 ${fmtPrice(deliveryFee)}` : ' \u00B7 Free'}`;

  const inputStyle = {
    borderWidth: 1, borderColor: tc + '30', borderRadius: 6,
    paddingHorizontal: 12, paddingVertical: 11,
    color: tc, fontSize: 14, fontWeight: '500' as const,
    backgroundColor: tc + '08', marginBottom: 10,
  };

  return (
    <View style={{ marginHorizontal: 20, marginTop: 18 }}>
      <Text style={{ fontSize: 9, fontWeight: '800', letterSpacing: 2, color: tc + '50', marginBottom: 8 }}>
        FULFILLMENT
      </Text>

      {/* Tappable row */}
      <TouchableOpacity
        onPress={toggle}
        activeOpacity={canChoose ? 0.7 : 1}
        style={{
          borderWidth: 1, borderColor: open ? tc + '60' : tc + '25',
          borderRadius: 8, paddingHorizontal: 14, paddingVertical: 14,
          backgroundColor: tc + '07', flexDirection: 'row', alignItems: 'center',
        }}
      >
        <Text style={{ fontSize: 14, fontWeight: '600', color: tc, flex: 1 }}>{methodLabel}</Text>
        {canChoose && (
          <Text style={{ fontSize: 18, color: tc + '60', fontWeight: '300', transform: [{ rotate: open ? '90deg' : '0deg' }] }}>
            {'>'}
          </Text>
        )}
      </TouchableOpacity>

      {/* Inline dropdown */}
      {open && canChoose && (
        <View style={{ borderWidth: 1, borderColor: tc + '20', borderRadius: 8, marginTop: 6, overflow: 'hidden', backgroundColor: tc + '05' }}>
          {canPickup && (
            <TouchableOpacity
              onPress={() => pick('pickup')}
              style={{
                flexDirection: 'row', alignItems: 'center', gap: 12,
                paddingVertical: 14, paddingHorizontal: 14,
                backgroundColor: method === 'pickup' ? tc + '18' : 'transparent',
                borderBottomWidth: canDeliver ? 1 : 0, borderBottomColor: tc + '15',
              }}
              activeOpacity={0.7}
            >
              <Text style={{ fontSize: 20 }}>🏪</Text>
              <View style={{ flex: 1 }}>
                <Text style={{ fontSize: 14, fontWeight: '800', color: tc }}>Pickup</Text>
                <Text style={{ fontSize: 12, color: tc + '65', marginTop: 2 }}>
                  {pickupLoc ?? 'At the event — details confirmed via email'}
                </Text>
              </View>
              {method === 'pickup' && (
                <View style={{ width: 20, height: 20, borderRadius: 10, backgroundColor: tc, alignItems: 'center', justifyContent: 'center' }}>
                  <Text style={{ fontSize: 11, color: bg, fontWeight: '900' }}>{'✓'}</Text>
                </View>
              )}
            </TouchableOpacity>
          )}
          {canDeliver && (
            <TouchableOpacity
              onPress={() => pick('delivery')}
              style={{
                flexDirection: 'row', alignItems: 'center', gap: 12,
                paddingVertical: 14, paddingHorizontal: 14,
                backgroundColor: method === 'delivery' ? tc + '18' : 'transparent',
              }}
              activeOpacity={0.7}
            >
              <Text style={{ fontSize: 20 }}>📦</Text>
              <View style={{ flex: 1 }}>
                <Text style={{ fontSize: 14, fontWeight: '800', color: tc }}>Delivery</Text>
                <Text style={{ fontSize: 12, color: tc + '65', marginTop: 2 }}>
                  {deliveryFee > 0 ? `${fmtPrice(deliveryFee)} delivery fee` : 'Free delivery'}
                </Text>
              </View>
              {method === 'delivery' && (
                <View style={{ width: 20, height: 20, borderRadius: 10, backgroundColor: tc, alignItems: 'center', justifyContent: 'center' }}>
                  <Text style={{ fontSize: 11, color: bg, fontWeight: '900' }}>{'✓'}</Text>
                </View>
              )}
            </TouchableOpacity>
          )}
        </View>
      )}

      {/* Delivery address form */}
      {method === 'delivery' && (
        <View style={{ marginTop: 14 }}>
          <Text style={{ fontSize: 9, fontWeight: '800', letterSpacing: 1.5, color: tc + '50', marginBottom: 10 }}>DELIVERY ADDRESS</Text>
          <TextInput placeholder="Street address *" placeholderTextColor={tc + '45'} value={address.street} onChangeText={(v) => onAddress({ ...address, street: v })} style={inputStyle} />
          <TextInput placeholder="Suburb" placeholderTextColor={tc + '45'} value={address.suburb} onChangeText={(v) => onAddress({ ...address, suburb: v })} style={inputStyle} />
          <View style={{ flexDirection: 'row', gap: 8 }}>
            <TextInput placeholder="City *" placeholderTextColor={tc + '45'} value={address.city} onChangeText={(v) => onAddress({ ...address, city: v })} style={[inputStyle, { flex: 1 }]} />
            <TextInput placeholder="Province *" placeholderTextColor={tc + '45'} value={address.province} onChangeText={(v) => onAddress({ ...address, province: v })} style={[inputStyle, { flex: 1 }]} />
          </View>
          <TextInput placeholder="Postal code" placeholderTextColor={tc + '45'} value={address.postal_code} onChangeText={(v) => onAddress({ ...address, postal_code: v })} keyboardType="numeric" style={inputStyle} />
        </View>
      )}
    </View>
  );
}

// ─── CartConfirmSheet ─────────────────────────────────────────────────────────

export const CartConfirmSheet = ({ item, selectedVariant, onClose, onAddToCart }: CartConfirmSheetProps) => {
  const { bottom }       = useSafeAreaInsets();
  const { bg, text: tc } = paletteFor(item);
  const images           = parseImages(item.images);

  const displayPrice =
    selectedVariant?.price_override != null ? selectedVariant.price_override : item.effective_price;

  const mode = (item.fulfillment_mode ?? 'delivery') as string;
  const defaultMethod: FulfillmentMethod =
    mode === 'pickup' ? 'pickup' : mode === 'delivery' ? 'delivery' : 'pickup';

  const [fulfillmentMethod, setFulfillmentMethod] = useState<FulfillmentMethod>(defaultMethod);
  const [deliveryAddress,   setDeliveryAddress]   = useState<DeliveryAddress>({
    street: '', suburb: '', city: '', province: '', postal_code: '',
  });

  const translateY      = useRef(new Animated.Value(SHEET_HEIGHT)).current;
  const backdropOpacity = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    Animated.parallel([
      Animated.spring(translateY,      { toValue: 0, damping: 22, stiffness: 260, useNativeDriver: true }),
      Animated.timing(backdropOpacity, { toValue: 1, duration: 260, useNativeDriver: true }),
    ]).start();
  }, []);

  const dismiss = useCallback(() => {
    Animated.parallel([
      Animated.timing(translateY,      { toValue: SHEET_HEIGHT, duration: 220, useNativeDriver: true }),
      Animated.timing(backdropOpacity, { toValue: 0,            duration: 200, useNativeDriver: true }),
    ]).start(onClose);
  }, [onClose]);

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

  const [added,   setAdded]   = useState(false);
  const btnScale              = useRef(new Animated.Value(1)).current;

  const handleAddToCart = useCallback(() => {
    if (fulfillmentMethod === 'delivery') {
      if (!deliveryAddress.street.trim() || !deliveryAddress.city.trim() || !deliveryAddress.province.trim()) {
        Alert.alert('Address required', 'Please fill in your street, city, and province for delivery.');
        return;
      }
    }
    Animated.sequence([
      Animated.timing(btnScale, { toValue: 0.94, duration: 80,  useNativeDriver: true }),
      Animated.spring(btnScale, { toValue: 1,    damping: 10,   stiffness: 300, useNativeDriver: true }),
    ]).start();
    setAdded(true);
    onAddToCart(item, selectedVariant, fulfillmentMethod,
      fulfillmentMethod === 'delivery' ? deliveryAddress : undefined);
    setTimeout(dismiss, 1100);
  }, [item, selectedVariant, fulfillmentMethod, deliveryAddress, onAddToCart, dismiss]);

  const stockLabel =
    !selectedVariant ? null :
    selectedVariant.stock_quantity === 0  ? { text: 'SOLD OUT',  color: '#ff3c6e' } :
    selectedVariant.stock_quantity <= 5   ? { text: 'LOW STOCK', color: '#ff9500' } :
                                            { text: 'IN STOCK',  color: '#34c759' };

  const deliveryFeeAmount = fulfillmentMethod === 'delivery' ? (item.delivery_fee ?? 0) : 0;
  const totalAmount       = displayPrice + deliveryFeeAmount;

  return (
    <Modal visible transparent animationType="none" onRequestClose={dismiss} statusBarTranslucent>
      <StatusBar barStyle="light-content" />
      <Animated.View
        style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, backgroundColor: '#000000c0', opacity: backdropOpacity }}
        onTouchEnd={dismiss}
      />
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
        style={{ position: 'absolute', bottom: 0, left: 0, right: 0, height: SHEET_HEIGHT }}
      >
        <Animated.View style={{
          flex: 1, backgroundColor: bg,
          borderTopLeftRadius: 16, borderTopRightRadius: 16,
          overflow: 'hidden', transform: [{ translateY }],
        }}>
          {/* Drag handle */}
          <View style={{ alignItems: 'center', paddingTop: 14, paddingBottom: 10 }} {...dragPan.panHandlers}>
            <View style={{ width: 44, height: 5, borderRadius: 3, backgroundColor: tc + '35' }} />
          </View>

          {/* Header */}
          <View style={{ flexDirection: 'row', alignItems: 'center', paddingHorizontal: 20, paddingBottom: 14, borderBottomWidth: 1, borderBottomColor: tc + '12' }}>
            <Text style={{ fontSize: 10, fontWeight: '900', letterSpacing: 2.5, color: tc + '55', flex: 1 }}>CONFIRM ORDER</Text>
            <TouchableOpacity onPress={dismiss} hitSlop={{ top: 12, bottom: 12, left: 12, right: 12 }}>
              <Text style={{ fontSize: 17, color: tc + '45' }}>{'✕'}</Text>
            </TouchableOpacity>
          </View>

          {/* Scrollable body */}
          <ScrollView
            showsVerticalScrollIndicator={false}
            contentContainerStyle={{ paddingBottom: (bottom > 0 ? bottom : 24) + 90 }}
            keyboardShouldPersistTaps="handled"
          >
            <ImageCarousel images={images} tc={tc} />

            {/* Name + price */}
            <View style={{ paddingHorizontal: 20, paddingTop: 18, paddingBottom: 6, flexDirection: 'row', alignItems: 'flex-start', gap: 12 }}>
              <View style={{ flex: 1 }}>
                <Text style={{ fontSize: 9, fontWeight: '800', letterSpacing: 1.8, color: tc + '50', marginBottom: 5 }}>
                  {(item.category ?? 'MERCHANDISE').toUpperCase()}
                </Text>
                <Text numberOfLines={2} style={{ fontSize: 17, fontWeight: '700', lineHeight: 22, color: tc }}>{item.name}</Text>
              </View>
              <Text style={{ fontSize: 22, fontWeight: '900', color: tc, letterSpacing: -0.5, paddingTop: 18, flexShrink: 0 }}>
                {fmtPrice(displayPrice)}
              </Text>
            </View>

            {/* Selected variant */}
            {selectedVariant && (
              <View style={{
                marginHorizontal: 20, marginTop: 12,
                borderWidth: 1, borderColor: tc + '22', borderRadius: 8,
                paddingHorizontal: 16, paddingVertical: 14,
                flexDirection: 'row', alignItems: 'center', backgroundColor: tc + '07',
              }}>
                <View style={{ flex: 1 }}>
                  <Text style={{ fontSize: 9, fontWeight: '800', letterSpacing: 1.8, color: tc + '50', marginBottom: 4 }}>SELECTED</Text>
                  <Text style={{ fontSize: 15, fontWeight: '800', letterSpacing: 0.3, color: tc }}>
                    {getVariantLabel(selectedVariant).toUpperCase()}
                  </Text>
                </View>
                {stockLabel && (
                  <Text style={{ fontSize: 8, fontWeight: '900', letterSpacing: 1, color: stockLabel.color }}>{stockLabel.text}</Text>
                )}
              </View>
            )}

            {/* Fulfillment — inline dropdown */}
            <FulfillmentSection
              item={item} method={fulfillmentMethod} onMethod={setFulfillmentMethod}
              address={deliveryAddress} onAddress={setDeliveryAddress} tc={tc} bg={bg}
            />

            {/* Other sizes */}
            <OtherVariants item={item} selectedVariant={selectedVariant} tc={tc} />

            {/* Order summary */}
            <View style={{ marginHorizontal: 20, marginTop: 22, borderTopWidth: 1, borderTopColor: tc + '12', paddingTop: 16 }}>
              <Text style={{ fontSize: 9, fontWeight: '800', letterSpacing: 2, color: tc + '50', marginBottom: 12 }}>ORDER SUMMARY</Text>
              <View style={{ flexDirection: 'row', justifyContent: 'space-between', paddingVertical: 9, borderBottomWidth: 1, borderBottomColor: tc + '0e' }}>
                <Text style={{ fontSize: 11, color: tc + '65' }}>Item</Text>
                <Text style={{ fontSize: 11, color: tc + '65' }}>{fmtPrice(displayPrice)}</Text>
              </View>
              <View style={{ flexDirection: 'row', justifyContent: 'space-between', paddingVertical: 9, borderBottomWidth: 1, borderBottomColor: tc + '0e' }}>
                <Text style={{ fontSize: 11, color: tc + '65' }}>Fulfilment</Text>
                <Text style={{ fontSize: 11, color: tc + '65' }}>
                  {fulfillmentMethod === 'delivery'
                    ? deliveryFeeAmount > 0 ? `Delivery · ${fmtPrice(deliveryFeeAmount)}` : 'Delivery · Free'
                    : 'Pickup'}
                </Text>
              </View>
              <View style={{ flexDirection: 'row', justifyContent: 'space-between', paddingVertical: 9 }}>
                <Text style={{ fontSize: 13, fontWeight: '800', color: tc }}>Total</Text>
                <Text style={{ fontSize: 16, fontWeight: '900', color: tc }}>{fmtPrice(totalAmount)}</Text>
              </View>
            </View>
          </ScrollView>

          {/* Pinned CTA */}
          <View style={{
            position: 'absolute', bottom: 0, left: 0, right: 0,
            paddingHorizontal: 20, paddingTop: 12,
            paddingBottom: bottom > 0 ? bottom : 24,
            backgroundColor: bg, borderTopWidth: 1, borderTopColor: tc + '12',
          }}>
            <Animated.View style={{ transform: [{ scale: btnScale }] }}>
              <TouchableOpacity
                onPress={added ? undefined : handleAddToCart}
                activeOpacity={0.85}
                style={{
                  backgroundColor: added ? '#34c759' : tc,
                  borderRadius: 8, paddingVertical: 17,
                  alignItems: 'center', justifyContent: 'center',
                  flexDirection: 'row', gap: 10,
                }}
              >
                {added
                  ? <Text style={{ fontSize: 12, fontWeight: '900', letterSpacing: 2, color: bg }}>{'✓'}  ADDED TO CART</Text>
                  : <Text style={{ fontSize: 12, fontWeight: '900', letterSpacing: 2.5, color: bg }}>ADD TO CART  {'→'}</Text>
                }
              </TouchableOpacity>
            </Animated.View>
          </View>
        </Animated.View>
      </KeyboardAvoidingView>
    </Modal>
  );
};