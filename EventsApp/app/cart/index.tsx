import React, { useState, useEffect, useCallback, useRef } from 'react';
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  ActivityIndicator,
  Alert,
  Animated,
  StyleSheet,
  RefreshControl,
  TextInput,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { Feather, Ionicons } from '@expo/vector-icons';
import { useAppTheme } from '../../theme/colors';
import {
  type CartItem,
  type CartSummary,
  type CartEventGroup,
  getCart,
  updateCartItem,
  updateCartItemAddress,
  removeCartItem,
  clearCart,
  cartItemLabel,
  cartItemSubtitle,
  fmtZAR,
  getCachedSessionId,
} from '../../lib/api';

// ─── Delivery address state per item ─────────────────────────────────────────

interface AddressForm {
  street:      string;
  suburb:      string;
  city:        string;
  province:    string;
  postal_code: string;
}

const emptyAddress = (): AddressForm => ({
  street: '', suburb: '', city: '', province: '', postal_code: '',
});

function parseStoredAddress(raw?: string | null): AddressForm | null {
  if (!raw) return null;
  try {
    const p = JSON.parse(raw);
    return {
      street:      p.street      ?? '',
      suburb:      p.suburb      ?? '',
      city:        p.city        ?? '',
      province:    p.province    ?? '',
      postal_code: p.postal_code ?? '',
    };
  } catch {
    return null;
  }
}

function isAddressComplete(a: AddressForm) {
  return a.street.trim() !== '' && a.city.trim() !== '' && a.province.trim() !== '';
}

// ─── DeliveryAddressRow ───────────────────────────────────────────────────────

interface DeliveryAddressRowProps {
  item:      CartItem;
  sessionId: string | null;
  colors:    any;
  isDark:    boolean;
  onSaved:   () => void;
}

function DeliveryAddressRow({ item, sessionId, colors, isDark, onSaved }: DeliveryAddressRowProps) {
  const stored   = parseStoredAddress(item.delivery_address);
  const [form,   setForm]   = useState<AddressForm>(stored ?? emptyAddress());
  const [open,   setOpen]   = useState(!stored || !isAddressComplete(stored));
  const [saving, setSaving] = useState(false);
  const heightAnim = useRef(new Animated.Value(open ? 1 : 0)).current;

  const toggle = () => {
    const next = !open;
    setOpen(next);
    Animated.spring(heightAnim, {
      toValue: next ? 1 : 0,
      damping: 20, stiffness: 220, useNativeDriver: false,
    }).start();
  };

  const handleSave = async () => {
    if (!isAddressComplete(form)) {
      Alert.alert('Missing fields', 'Street, city, and province are required.');
      return;
    }
    setSaving(true);
    try {
      await updateCartItemAddress(item.id, {
        street:      form.street.trim(),
        suburb:      form.suburb.trim() || undefined,
        city:        form.city.trim(),
        province:    form.province.trim(),
        postal_code: form.postal_code.trim() || undefined,
      }, sessionId);
      setOpen(false);
      Animated.spring(heightAnim, { toValue: 0, damping: 20, stiffness: 220, useNativeDriver: false }).start();
      onSaved();
    } catch (err: any) {
      Alert.alert('Error', err?.response?.data?.error ?? 'Could not save address.');
    } finally {
      setSaving(false);
    }
  };

  const complete = isAddressComplete(form);
  const cardBg   = isDark ? '#161616' : '#f9f9f9';
  const border   = isDark ? '#2a2a2a' : '#e5e5e5';
  const inputBg  = isDark ? '#0f0f0f' : '#fff';
  const label    = isDark ? '#888' : '#999';

  const summaryLine = stored && isAddressComplete(stored)
    ? [stored.street, stored.suburb, stored.city, stored.province].filter(Boolean).join(', ')
    : null;

  return (
    <View style={{ marginHorizontal: 16, marginBottom: 4 }}>
      <TouchableOpacity
        onPress={toggle}
        activeOpacity={0.75}
        style={[styles.addressHeader, { backgroundColor: cardBg, borderColor: border }]}
      >
        <View style={styles.addressHeaderLeft}>
          <Text style={{ fontSize: 16 }}>📦</Text>
          <View style={{ flex: 1 }}>
            <Text style={[styles.addressHeaderTitle, { color: colors.text }]}>
              Delivery address
            </Text>
            {summaryLine ? (
              <Text style={[styles.addressHeaderSub, { color: colors.subText }]} numberOfLines={1}>
                {summaryLine}
              </Text>
            ) : (
              <Text style={[styles.addressHeaderSub, { color: '#ff9500' }]}>
                Required before checkout
              </Text>
            )}
          </View>
        </View>
        <View style={styles.addressHeaderRight}>
          {summaryLine && (
            <View style={styles.savedPill}>
              <Text style={styles.savedPillText}>✓</Text>
            </View>
          )}
          <Text style={[styles.chevron, { color: colors.subText }]}>{open ? '▲' : '▼'}</Text>
        </View>
      </TouchableOpacity>

      <Animated.View style={{
        overflow: 'hidden',
        maxHeight: heightAnim.interpolate({ inputRange: [0, 1], outputRange: [0, 520] }),
        opacity: heightAnim,
      }}>
        <View style={[styles.addressForm, { backgroundColor: cardBg, borderColor: border }]}>
          {[
            { key: 'street',      label: 'Street address *',  multi: false },
            { key: 'suburb',      label: 'Suburb',            multi: false },
          ].map(({ key, label: ph }) => (
            <TextInput
              key={key}
              placeholder={ph}
              placeholderTextColor={label}
              value={(form as any)[key]}
              onChangeText={(v) => setForm(f => ({ ...f, [key]: v }))}
              style={[styles.input, { color: colors.text, backgroundColor: inputBg, borderColor: border }]}
            />
          ))}

          <View style={{ flexDirection: 'row', gap: 8 }}>
            {[
              { key: 'city',     label: 'City *' },
              { key: 'province', label: 'Province *' },
            ].map(({ key, label: ph }) => (
              <TextInput
                key={key}
                placeholder={ph}
                placeholderTextColor={label}
                value={(form as any)[key]}
                onChangeText={(v) => setForm(f => ({ ...f, [key]: v }))}
                style={[styles.input, { flex: 1, color: colors.text, backgroundColor: inputBg, borderColor: border }]}
              />
            ))}
          </View>

          <TextInput
            placeholder="Postal code"
            placeholderTextColor={label}
            value={form.postal_code}
            onChangeText={(v) => setForm(f => ({ ...f, postal_code: v }))}
            keyboardType="numeric"
            style={[styles.input, { color: colors.text, backgroundColor: inputBg, borderColor: border }]}
          />

          <TouchableOpacity
            onPress={handleSave}
            disabled={saving || !complete}
            style={[
              styles.saveBtn,
              { backgroundColor: complete ? '#ff2d55' : '#ff2d5530', opacity: saving ? 0.6 : 1 },
            ]}
          >
            {saving
              ? <ActivityIndicator size="small" color="#fff" />
              : <Text style={styles.saveBtnText}>{summaryLine ? 'Update address' : 'Save address'}</Text>
            }
          </TouchableOpacity>
        </View>
      </Animated.View>
    </View>
  );
}

// ─── CartItemRow ──────────────────────────────────────────────────────────────

interface CartItemRowProps {
  item:             CartItem;
  onQuantityChange: (id: string, qty: number) => void;
  onRemove:         (id: string) => void;
  updating:         boolean;
  colors:           any;
}

function CartItemRow({ item, onQuantityChange, onRemove, updating, colors }: CartItemRowProps) {
  const fadeAnim = useRef(new Animated.Value(1)).current;
  const lineTotal = item.unit_price_snapshot * item.quantity + item.delivery_fee_snapshot;
  const sub       = cartItemSubtitle(item);
  const isTicket  = item.item_type === 'ticket';

  const handleRemove = () => {
    Alert.alert(
      'Remove item',
      `Remove "${cartItemLabel(item)}" from your cart?`,
      [
        { text: 'Cancel', style: 'cancel' },
        {
          text: 'Remove', style: 'destructive',
          onPress: () => {
            Animated.timing(fadeAnim, { toValue: 0, duration: 220, useNativeDriver: true }).start(() => {
              onRemove(item.id);
            });
          },
        },
      ]
    );
  };

  return (
    <Animated.View style={[styles.itemRow, { opacity: fadeAnim, backgroundColor: colors.cardBg, borderColor: colors.dividerColor }]}>
      <View style={[styles.typeBadge, isTicket ? styles.typeBadgeTicket : styles.typeBadgeMerch]}>
        {isTicket
          ? <Ionicons name="ticket-outline" size={14} color="#fff" />
          : <Feather name="shopping-bag" size={13} color="#fff" />
        }
      </View>

      <View style={styles.itemContent}>
        <Text style={[styles.itemName, { color: colors.text }]} numberOfLines={2}>
          {cartItemLabel(item)}
        </Text>
        {sub ? (
          <Text style={[styles.itemSub, { color: colors.subText }]} numberOfLines={1}>{sub}</Text>
        ) : null}
        {item.delivery_fee_snapshot > 0 && (
          <Text style={[styles.itemFeeNote, { color: colors.subText }]}>
            + {fmtZAR(item.delivery_fee_snapshot)} delivery
          </Text>
        )}

        <View style={styles.itemBottom}>
          <View style={[styles.stepper, { borderColor: colors.dividerColor, backgroundColor: colors.sectionCardBg }]}>
            <TouchableOpacity
              style={styles.stepperBtn}
              onPress={() => item.quantity > 1 ? onQuantityChange(item.id, item.quantity - 1) : handleRemove()}
              disabled={updating}
              hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}
            >
              <Text style={[styles.stepperIcon, { color: colors.text }]}>
                {item.quantity === 1 ? '🗑' : '−'}
              </Text>
            </TouchableOpacity>
            <View style={styles.stepperCount}>
              {updating
                ? <ActivityIndicator size="small" color="#ff2d55" />
                : <Text style={[styles.stepperCountText, { color: colors.text }]}>{item.quantity}</Text>
              }
            </View>
            <TouchableOpacity
              style={styles.stepperBtn}
              onPress={() => onQuantityChange(item.id, item.quantity + 1)}
              disabled={updating || item.quantity >= 10}
              hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}
            >
              <Text style={[styles.stepperIcon, { color: item.quantity >= 10 ? colors.subText : colors.text }]}>＋</Text>
            </TouchableOpacity>
          </View>
          <Text style={styles.lineTotal}>{fmtZAR(lineTotal)}</Text>
        </View>
      </View>

      <TouchableOpacity style={styles.deleteBtn} onPress={handleRemove} hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}>
        <Feather name="trash-2" size={16} color={colors.subText} />
      </TouchableOpacity>
    </Animated.View>
  );
}

// ─── Empty State ──────────────────────────────────────────────────────────────

function EmptyCart({ colors, onBrowse }: { colors: any; onBrowse: () => void }) {
  return (
    <View style={styles.emptyWrap}>
      <View style={[styles.emptyIconCircle, { backgroundColor: colors.sectionCardBg }]}>
        <Ionicons name="cart-outline" size={48} color={colors.subText} />
      </View>
      <Text style={[styles.emptyTitle, { color: colors.text }]}>Your cart is empty</Text>
      <Text style={[styles.emptySub, { color: colors.subText }]}>
        Browse events and merch drops to add items here
      </Text>
      <TouchableOpacity style={styles.emptyBtn} onPress={onBrowse} activeOpacity={0.85}>
        <Text style={styles.emptyBtnText}>Explore Events</Text>
      </TouchableOpacity>
    </View>
  );
}

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function CartScreen() {
  const router             = useRouter();
  const insets             = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();

  const [cart, setCart]             = useState<CartSummary | null>(null);
  const [loading, setLoading]       = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [updatingId, setUpdatingId] = useState<string | null>(null);
  const sidRef                      = useRef<string | null>(null);

  // ── Fetch ──────────────────────────────────────────────────────────────────

  const fetchCart = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      if (sidRef.current === null) sidRef.current = await getCachedSessionId();
      const data = await getCart(sidRef.current);
      setCart(data);
    } catch {
      setCart(null);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => { fetchCart(); }, [fetchCart]);

  // ── Actions ────────────────────────────────────────────────────────────────

  const handleQuantityChange = async (itemId: string, qty: number) => {
    setUpdatingId(itemId);
    try {
      await updateCartItem(itemId, qty, sidRef.current);
      await fetchCart(true);
    } catch (err: any) {
      Alert.alert('Error', err?.response?.data?.error ?? 'Could not update quantity');
    } finally {
      setUpdatingId(null);
    }
  };

  const handleRemove = async (itemId: string) => {
    setUpdatingId(itemId);
    try {
      await removeCartItem(itemId, sidRef.current);
      await fetchCart(true);
    } catch {
      Alert.alert('Error', 'Could not remove item');
    } finally {
      setUpdatingId(null);
    }
  };

  const handleClearCart = () => {
    Alert.alert('Clear cart', 'Remove all items from your cart?', [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'Clear', style: 'destructive',
        onPress: async () => {
          try {
            await clearCart(sidRef.current);
            setCart(null);
          } catch {
            Alert.alert('Error', 'Could not clear cart');
          }
        },
      },
    ]);
  };

  // ── Render ─────────────────────────────────────────────────────────────────

  const bg          = isDark ? '#0a0a0a' : '#f5f5f7';
  const totalItems  = cart?.total_items ?? 0;
  const totalAmount = cart?.total_amount ?? 0;
  const hasItems    = totalItems > 0;
  const allItems    = cart?.items_by_event?.flatMap(g => g.items) ?? [];

  // Warn if any delivery merch item is missing an address
  const missingAddress = allItems.some(
    i => i.item_type === 'merchandise' && i.fulfillment_method === 'delivery' && !i.delivery_address,
  );

  return (
    <View style={[styles.screen, { backgroundColor: bg, paddingBottom: insets.bottom }]}>

      {/* ── Header ─────────────────────────────────────────────────── */}
      <View style={[styles.header, { paddingTop: insets.top + 8, backgroundColor: isDark ? '#0a0a0a' : '#fff', borderBottomColor: colors.dividerColor }]}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn} hitSlop={{ top: 10, bottom: 10, left: 10, right: 10 }}>
          <Feather name="arrow-left" size={22} color={colors.text} />
        </TouchableOpacity>
        <Text style={[styles.headerTitle, { color: colors.text }]}>Cart</Text>
        <View style={styles.headerRight}>
          {hasItems && (
            <View style={styles.countBubble}>
              <Text style={styles.countBubbleText}>{totalItems}</Text>
            </View>
          )}
          {hasItems && (
            <TouchableOpacity onPress={handleClearCart} style={styles.clearBtn} hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}>
              <Text style={[styles.clearBtnText, { color: colors.subText }]}>Clear</Text>
            </TouchableOpacity>
          )}
        </View>
      </View>

      {/* ── Body ───────────────────────────────────────────────────── */}
      {loading ? (
        <View style={styles.centered}>
          <ActivityIndicator size="large" color="#ff2d55" />
          <Text style={[styles.loadingText, { color: colors.subText }]}>Loading cart…</Text>
        </View>
      ) : !hasItems ? (
        <EmptyCart colors={colors} onBrowse={() => router.back()} />
      ) : (
        <ScrollView
          style={styles.scroll}
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingBottom: 180, paddingTop: 12 }}
          refreshControl={
            <RefreshControl refreshing={refreshing} onRefresh={() => { setRefreshing(true); fetchCart(); }} tintColor="#ff2d55" />
          }
        >
          {cart!.items_by_event.map((group: CartEventGroup) => (
            <View key={group.event_id} style={styles.group}>
              <View style={[styles.groupHeader, { backgroundColor: isDark ? '#111' : '#fff' }]}>
                <View style={styles.groupHeaderLeft}>
                  <View style={styles.groupDot} />
                  <Text style={[styles.groupTitle, { color: colors.text }]} numberOfLines={1}>
                    {group.event_title || 'Items'}
                  </Text>
                </View>
                <Text style={[styles.groupSubtotal, { color: colors.subText }]}>{fmtZAR(group.subtotal)}</Text>
              </View>

              {group.items.map((item: CartItem) => (
                <React.Fragment key={item.id}>
                  <CartItemRow
                    item={item}
                    onQuantityChange={handleQuantityChange}
                    onRemove={handleRemove}
                    updating={updatingId === item.id}
                    colors={colors}
                  />
                  {/* Delivery address form for delivery merch items */}
                  {item.item_type === 'merchandise' && item.fulfillment_method === 'delivery' && (
                    <DeliveryAddressRow
                      item={item}
                      sessionId={sidRef.current}
                      colors={colors}
                      isDark={isDark}
                      onSaved={() => fetchCart(true)}
                    />
                  )}
                </React.Fragment>
              ))}
            </View>
          ))}

          {/* Address-required banner */}
          {missingAddress && (
            <View style={[styles.warningPill, { backgroundColor: isDark ? '#2a1a00' : '#fff8ec' }]}>
              <Text style={{ fontSize: 14 }}>⚠️</Text>
              <Text style={[styles.warningText, { color: isDark ? '#ffb340' : '#a05000' }]}>
                Add a delivery address above before you can check out
              </Text>
            </View>
          )}

          {/* Order summary */}
          <View style={[styles.summaryCard, { backgroundColor: isDark ? '#111' : '#fff', borderColor: colors.dividerColor }]}>
            <Text style={[styles.summaryTitle, { color: colors.subText }]}>ORDER SUMMARY</Text>
            {allItems.map((item: CartItem) => (
              <View key={item.id} style={styles.summaryRow}>
                <Text style={[styles.summaryLabel, { color: colors.text }]} numberOfLines={1}>
                  {cartItemLabel(item)} × {item.quantity}
                </Text>
                <Text style={[styles.summaryValue, { color: colors.text }]}>
                  {fmtZAR(item.unit_price_snapshot * item.quantity)}
                </Text>
              </View>
            ))}
            {allItems.some(i => i.delivery_fee_snapshot > 0) && (
              <View style={styles.summaryRow}>
                <Text style={[styles.summaryLabel, { color: colors.subText }]}>Delivery fees</Text>
                <Text style={[styles.summaryValue, { color: colors.subText }]}>
                  {fmtZAR(allItems.reduce((s, i) => s + i.delivery_fee_snapshot, 0))}
                </Text>
              </View>
            )}
            <View style={[styles.summaryDivider, { backgroundColor: colors.dividerColor }]} />
            <View style={styles.summaryTotalRow}>
              <Text style={[styles.summaryTotalLabel, { color: colors.text }]}>Total</Text>
              <Text style={styles.summaryTotalValue}>{fmtZAR(totalAmount)}</Text>
            </View>
          </View>

          <View style={[styles.infoPill, { backgroundColor: colors.sectionCardBg }]}>
            <Feather name="lock" size={13} color={colors.subText} />
            <Text style={[styles.infoPillText, { color: colors.subText }]}>
              Tickets are reserved for 10 minutes once you begin checkout
            </Text>
          </View>
        </ScrollView>
      )}

      {/* ── Checkout CTA ────────────────────────────────────────────── */}
      {hasItems && !loading && (
        <View style={[styles.ctaBar, { paddingBottom: insets.bottom > 0 ? insets.bottom + 4 : 20, backgroundColor: isDark ? '#0a0a0a' : '#fff', borderTopColor: colors.dividerColor }]}>
          <View style={styles.ctaAmountWrap}>
            <Text style={[styles.ctaAmountLabel, { color: colors.subText }]}>Total</Text>
            <Text style={[styles.ctaAmount, { color: colors.text }]}>{fmtZAR(totalAmount)}</Text>
          </View>
          <TouchableOpacity
            style={[styles.checkoutBtn, missingAddress && styles.checkoutBtnDisabled]}
            onPress={() => {
              if (missingAddress) {
                Alert.alert('Address required', 'Please add a delivery address for all delivery items before checking out.');
                return;
              }
              router.push('/cart/checkout' as any);
            }}
            activeOpacity={0.88}
          >
            <Text style={styles.checkoutBtnText}>Checkout</Text>
            <Feather name="arrow-right" size={18} color="#fff" />
          </TouchableOpacity>
        </View>
      )}
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  screen:           { flex: 1 },
  centered:         { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 12 },
  scroll:           { flex: 1 },
  loadingText:      { fontSize: 13, fontWeight: '600', marginTop: 8 },

  header:           { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 20, paddingBottom: 14, borderBottomWidth: StyleSheet.hairlineWidth, gap: 10 },
  backBtn:          { width: 32 },
  headerTitle:      { fontSize: 28, fontWeight: '800', letterSpacing: -0.5, flex: 1 },
  headerRight:      { flexDirection: 'row', alignItems: 'center', gap: 10 },
  countBubble:      { minWidth: 22, height: 22, borderRadius: 11, backgroundColor: '#ff2d55', alignItems: 'center', justifyContent: 'center', paddingHorizontal: 6 },
  countBubbleText:  { fontSize: 12, fontWeight: '800', color: '#fff' },
  clearBtn:         { paddingHorizontal: 6, paddingVertical: 4 },
  clearBtnText:     { fontSize: 14, fontWeight: '600' },

  group:            { marginBottom: 4 },
  groupHeader:      { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 20, paddingVertical: 10, marginHorizontal: 16, marginBottom: 4, borderRadius: 12 },
  groupHeaderLeft:  { flexDirection: 'row', alignItems: 'center', gap: 8, flex: 1, marginRight: 12 },
  groupDot:         { width: 8, height: 8, borderRadius: 4, backgroundColor: '#ff2d55' },
  groupTitle:       { fontSize: 13, fontWeight: '700', letterSpacing: 0.2, flex: 1 },
  groupSubtotal:    { fontSize: 13, fontWeight: '600' },

  itemRow: {
    flexDirection: 'row', alignItems: 'flex-start',
    paddingHorizontal: 16, paddingVertical: 14,
    marginHorizontal: 16, marginBottom: 4,
    borderRadius: 14, borderWidth: StyleSheet.hairlineWidth, gap: 12,
  },
  typeBadge:        { width: 28, height: 28, borderRadius: 8, alignItems: 'center', justifyContent: 'center', marginTop: 2 },
  typeBadgeTicket:  { backgroundColor: '#ff2d55' },
  typeBadgeMerch:   { backgroundColor: '#1c2e61' },
  itemContent:      { flex: 1, gap: 3 },
  itemName:         { fontSize: 15, fontWeight: '700', lineHeight: 20 },
  itemSub:          { fontSize: 12, fontWeight: '500' },
  itemFeeNote:      { fontSize: 11, fontWeight: '500' },
  itemBottom:       { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginTop: 8 },
  deleteBtn:        { padding: 4, marginTop: 2 },

  stepper:          { flexDirection: 'row', alignItems: 'center', borderRadius: 10, borderWidth: StyleSheet.hairlineWidth, overflow: 'hidden' },
  stepperBtn:       { width: 32, height: 32, alignItems: 'center', justifyContent: 'center' },
  stepperIcon:      { fontSize: 16, fontWeight: '600' },
  stepperCount:     { width: 36, alignItems: 'center', justifyContent: 'center', height: 32 },
  stepperCountText: { fontSize: 15, fontWeight: '700' },
  lineTotal:        { fontSize: 15, fontWeight: '800', color: '#ff2d55' },

  // ── Delivery address ───────────────────────────────────────────────────────
  addressHeader: {
    flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between',
    paddingHorizontal: 14, paddingVertical: 12,
    borderRadius: 12, borderWidth: StyleSheet.hairlineWidth,
    marginBottom: 2,
  },
  addressHeaderLeft:  { flexDirection: 'row', alignItems: 'center', gap: 10, flex: 1 },
  addressHeaderTitle: { fontSize: 13, fontWeight: '700' },
  addressHeaderSub:   { fontSize: 11, fontWeight: '500', marginTop: 1 },
  addressHeaderRight: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  savedPill: {
    width: 20, height: 20, borderRadius: 10,
    backgroundColor: '#34c759', alignItems: 'center', justifyContent: 'center',
  },
  savedPillText:    { fontSize: 10, fontWeight: '900', color: '#fff' },
  chevron:          { fontSize: 10, fontWeight: '700' },

  addressForm: {
    borderRadius: 12, borderWidth: StyleSheet.hairlineWidth,
    padding: 14, gap: 10, marginBottom: 8,
  },
  input: {
    borderWidth: 1, borderRadius: 8,
    paddingHorizontal: 12, paddingVertical: 11,
    fontSize: 14, fontWeight: '500',
  },
  saveBtn: {
    borderRadius: 8, paddingVertical: 13,
    alignItems: 'center', justifyContent: 'center',
    marginTop: 4,
  },
  saveBtnText:      { fontSize: 13, fontWeight: '800', color: '#fff', letterSpacing: 0.3 },

  // ── Warning ────────────────────────────────────────────────────────────────
  warningPill: {
    flexDirection: 'row', alignItems: 'center', gap: 8,
    marginHorizontal: 16, marginBottom: 8, padding: 14, borderRadius: 10,
  },
  warningText:      { fontSize: 13, fontWeight: '600', flex: 1 },

  // ── Summary ────────────────────────────────────────────────────────────────
  summaryCard: {
    marginHorizontal: 16, marginTop: 8, marginBottom: 8,
    borderRadius: 16, borderWidth: StyleSheet.hairlineWidth, padding: 18, gap: 10,
  },
  summaryTitle:     { fontSize: 11, fontWeight: '800', letterSpacing: 1.2, marginBottom: 2 },
  summaryRow:       { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  summaryLabel:     { fontSize: 14, fontWeight: '500', flex: 1, marginRight: 12 },
  summaryValue:     { fontSize: 14, fontWeight: '600' },
  summaryDivider:   { height: StyleSheet.hairlineWidth, marginVertical: 4 },
  summaryTotalRow:  { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  summaryTotalLabel:{ fontSize: 16, fontWeight: '700' },
  summaryTotalValue:{ fontSize: 18, fontWeight: '800', color: '#ff2d55' },

  infoPill:         { flexDirection: 'row', alignItems: 'center', gap: 8, marginHorizontal: 16, padding: 12, borderRadius: 10, marginBottom: 8 },
  infoPillText:     { fontSize: 12, fontWeight: '500', flex: 1 },

  // ── CTA bar ────────────────────────────────────────────────────────────────
  ctaBar: {
    position: 'absolute', bottom: 0, left: 0, right: 0,
    flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between',
    paddingHorizontal: 20, paddingTop: 14, borderTopWidth: StyleSheet.hairlineWidth,
  },
  ctaAmountWrap:       { gap: 2 },
  ctaAmountLabel:      { fontSize: 12, fontWeight: '600' },
  ctaAmount:           { fontSize: 22, fontWeight: '800', letterSpacing: -0.5 },
  checkoutBtn:         { flexDirection: 'row', alignItems: 'center', gap: 8, backgroundColor: '#ff2d55', paddingHorizontal: 24, paddingVertical: 14, borderRadius: 14 },
  checkoutBtnDisabled: { backgroundColor: '#ff2d5570' },
  checkoutBtnText:     { fontSize: 16, fontWeight: '800', color: '#fff', letterSpacing: 0.2 },

  emptyWrap:        { flex: 1, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 40, gap: 14 },
  emptyIconCircle:  { width: 96, height: 96, borderRadius: 48, alignItems: 'center', justifyContent: 'center', marginBottom: 4 },
  emptyTitle:       { fontSize: 22, fontWeight: '800', letterSpacing: -0.3 },
  emptySub:         { fontSize: 14, fontWeight: '500', textAlign: 'center', lineHeight: 20 },
  emptyBtn:         { backgroundColor: '#ff2d55', paddingHorizontal: 28, paddingVertical: 13, borderRadius: 12, marginTop: 6 },
  emptyBtnText:     { fontSize: 15, fontWeight: '800', color: '#fff' },
});