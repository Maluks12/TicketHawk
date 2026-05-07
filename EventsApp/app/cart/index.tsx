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
  Platform,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { Feather, Ionicons } from '@expo/vector-icons';
import * as SecureStore from 'expo-secure-store';
import { useAppTheme } from '../../theme/colors';
import { api } from '../../lib/api';
import { useAuthStore } from '../../store/authStore';

// ─── Types ──────────────────────────────────────────────────────────────────

interface BackendCartItem {
  id: string;
  cart_id: string;
  item_type: 'ticket' | 'merchandise';
  event_id: string;
  ticket_type_id?: string | null;
  ticket_type_name?: string | null;
  merchandise_variant_id?: string | null;
  quantity: number;
  unit_price_snapshot: number;
  delivery_fee_snapshot: number;
  fulfillment_method?: string | null;
  delivery_address?: string | null;
  event_title?: string | null;
  variant_sku?: string | null;
  merch_name?: string | null;
  variant_attributes?: string | null;
  pickup_location?: string | null;
  created_at: string;
}

interface EventGroup {
  event_id: string;
  event_title: string;
  items: BackendCartItem[];
  subtotal: number;
}

interface CartSummary {
  total_items: number;
  total_amount: number;
  currency: string;
  items_by_event: EventGroup[];
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function fmtPrice(amount: number): string {
  return `R${Math.round(amount).toLocaleString('en-ZA')}`;
}

function itemLabel(item: BackendCartItem): string {
  if (item.item_type === 'ticket') {
    return item.ticket_type_name ?? 'General Admission';
  }
  return item.merch_name ?? 'Merchandise';
}

function itemSub(item: BackendCartItem): string | null {
  if (item.item_type === 'merchandise') {
    const parts: string[] = [];
    if (item.variant_sku) parts.push(item.variant_sku);
    if (item.variant_attributes) {
      try {
        const attrs = JSON.parse(item.variant_attributes);
        Object.entries(attrs).forEach(([k, v]) => parts.push(`${k}: ${v}`));
      } catch {
        parts.push(item.variant_attributes);
      }
    }
    if (item.fulfillment_method === 'delivery') parts.push('Delivery');
    else if (item.fulfillment_method === 'pickup') {
      parts.push(item.pickup_location ? `Pickup · ${item.pickup_location}` : 'Pickup');
    }
    return parts.join(' · ') || null;
  }
  return null;
}

async function getSessionId(): Promise<string | null> {
  try {
    return await SecureStore.getItemAsync('session_id');
  } catch {
    return null;
  }
}

// ─── CartItemRow ─────────────────────────────────────────────────────────────

interface CartItemRowProps {
  item: BackendCartItem;
  onQuantityChange: (id: string, qty: number) => void;
  onRemove: (id: string) => void;
  updating: boolean;
  colors: any;
}

function CartItemRow({ item, onQuantityChange, onRemove, updating, colors }: CartItemRowProps) {
  const fadeAnim = useRef(new Animated.Value(1)).current;
  const lineTotal = item.unit_price_snapshot * item.quantity + item.delivery_fee_snapshot;
  const sub = itemSub(item);
  const isTicket = item.item_type === 'ticket';

  const handleRemove = () => {
    Alert.alert(
      'Remove item',
      `Remove "${itemLabel(item)}" from your cart?`,
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
      {/* Type badge */}
      <View style={[styles.typeBadge, isTicket ? styles.typeBadgeTicket : styles.typeBadgeMerch]}>
        {isTicket
          ? <Ionicons name="ticket-outline" size={14} color="#fff" />
          : <Feather name="shopping-bag" size={13} color="#fff" />
        }
      </View>

      {/* Content */}
      <View style={styles.itemContent}>
        <Text style={[styles.itemName, { color: colors.text }]} numberOfLines={2}>
          {itemLabel(item)}
        </Text>
        {sub ? (
          <Text style={[styles.itemSub, { color: colors.subText }]} numberOfLines={1}>
            {sub}
          </Text>
        ) : null}
        {item.delivery_fee_snapshot > 0 && (
          <Text style={[styles.itemFeeNote, { color: colors.subText }]}>
            + {fmtPrice(item.delivery_fee_snapshot)} delivery
          </Text>
        )}

        {/* Quantity stepper + price row */}
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

          <Text style={styles.lineTotal}>{fmtPrice(lineTotal)}</Text>
        </View>
      </View>

      {/* Delete button */}
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
  const router    = useRouter();
  const insets    = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();
  const { user }  = useAuthStore();

  const [cart, setCart]         = useState<CartSummary | null>(null);
  const [loading, setLoading]   = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [updatingId, setUpdatingId] = useState<string | null>(null);

  // ── Fetch ──────────────────────────────────────────────────────────────────

  const fetchCart = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      const sid = await getSessionId();
      const headers: Record<string, string> = {};
      if (sid) headers['X-Session-ID'] = sid;
      const res = await api.get<CartSummary>('/v1/cart', { headers });
      setCart(res.data);
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
      const sid = await getSessionId();
      const headers: Record<string, string> = {};
      if (sid) headers['X-Session-ID'] = sid;
      await api.patch(`/v1/cart/items/${itemId}`, { quantity: qty }, { headers });
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
      const sid = await getSessionId();
      const headers: Record<string, string> = {};
      if (sid) headers['X-Session-ID'] = sid;
      await api.delete(`/v1/cart/items/${itemId}`, { headers });
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
            const sid = await getSessionId();
            const headers: Record<string, string> = {};
            if (sid) headers['X-Session-ID'] = sid;
            await api.delete('/v1/cart', { headers });
            setCart(null);
          } catch {
            Alert.alert('Error', 'Could not clear cart');
          }
        },
      },
    ]);
  };

  const handleCheckout = () => {
    router.push('/cart/checkout' as any);
  };

  // ── Render ─────────────────────────────────────────────────────────────────

  const bg = isDark ? '#0a0a0a' : '#f5f5f7';
  const totalItems  = cart?.total_items ?? 0;
  const totalAmount = cart?.total_amount ?? 0;
  const hasItems    = totalItems > 0;

  const allItems: BackendCartItem[] = cart?.items_by_event?.flatMap(eg => eg.items) ?? [];

  return (
    <View style={[styles.screen, { backgroundColor: bg, paddingBottom: insets.bottom }]}>

      {/* ── Header ─────────────────────────────────────────────────── */}
      <View style={[styles.header, { paddingTop: insets.top + 8, backgroundColor: isDark ? '#0a0a0a' : '#fff', borderBottomColor: colors.dividerColor }]}>
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
        <EmptyCart colors={colors} onBrowse={() => router.push('/' as any)} />
      ) : (
        <ScrollView
          style={styles.scroll}
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingBottom: 180, paddingTop: 12 }}
          refreshControl={
            <RefreshControl
              refreshing={refreshing}
              onRefresh={() => { setRefreshing(true); fetchCart(); }}
              tintColor="#ff2d55"
            />
          }
        >
          {/* Items grouped by event */}
          {cart!.items_by_event.map((group) => (
            <View key={group.event_id} style={styles.group}>
              {/* Group header */}
              <View style={[styles.groupHeader, { backgroundColor: isDark ? '#111' : '#fff' }]}>
                <View style={styles.groupHeaderLeft}>
                  <View style={styles.groupDot} />
                  <Text style={[styles.groupTitle, { color: colors.text }]} numberOfLines={1}>
                    {group.event_title || 'Items'}
                  </Text>
                </View>
                <Text style={[styles.groupSubtotal, { color: colors.subText }]}>
                  {fmtPrice(group.subtotal)}
                </Text>
              </View>

              {/* Items */}
              {group.items.map((item) => (
                <CartItemRow
                  key={item.id}
                  item={item}
                  onQuantityChange={handleQuantityChange}
                  onRemove={handleRemove}
                  updating={updatingId === item.id}
                  colors={colors}
                />
              ))}
            </View>
          ))}

          {/* Order summary card */}
          <View style={[styles.summaryCard, { backgroundColor: isDark ? '#111' : '#fff', borderColor: colors.dividerColor }]}>
            <Text style={[styles.summaryTitle, { color: colors.subText }]}>ORDER SUMMARY</Text>
            {allItems.map((item) => (
              <View key={item.id} style={styles.summaryRow}>
                <Text style={[styles.summaryLabel, { color: colors.text }]} numberOfLines={1}>
                  {itemLabel(item)} × {item.quantity}
                </Text>
                <Text style={[styles.summaryValue, { color: colors.text }]}>
                  {fmtPrice(item.unit_price_snapshot * item.quantity)}
                </Text>
              </View>
            ))}
            {allItems.some(i => i.delivery_fee_snapshot > 0) && (
              <View style={styles.summaryRow}>
                <Text style={[styles.summaryLabel, { color: colors.subText }]}>Delivery fees</Text>
                <Text style={[styles.summaryValue, { color: colors.subText }]}>
                  {fmtPrice(allItems.reduce((s, i) => s + i.delivery_fee_snapshot, 0))}
                </Text>
              </View>
            )}
            <View style={[styles.summaryDivider, { backgroundColor: colors.dividerColor }]} />
            <View style={styles.summaryTotalRow}>
              <Text style={[styles.summaryTotalLabel, { color: colors.text }]}>Total</Text>
              <Text style={styles.summaryTotalValue}>{fmtPrice(totalAmount)}</Text>
            </View>
          </View>

          {/* Info pill */}
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
            <Text style={[styles.ctaAmount, { color: colors.text }]}>{fmtPrice(totalAmount)}</Text>
          </View>
          <TouchableOpacity style={styles.checkoutBtn} onPress={handleCheckout} activeOpacity={0.88}>
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

  // Header
  header:           { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 20, paddingBottom: 14, borderBottomWidth: StyleSheet.hairlineWidth },
  headerTitle:      { fontSize: 32, fontWeight: '800', letterSpacing: -0.5 },
  headerRight:      { flexDirection: 'row', alignItems: 'center', gap: 10 },
  countBubble:      { minWidth: 22, height: 22, borderRadius: 11, backgroundColor: '#ff2d55', alignItems: 'center', justifyContent: 'center', paddingHorizontal: 6 },
  countBubbleText:  { fontSize: 12, fontWeight: '800', color: '#fff' },
  clearBtn:         { paddingHorizontal: 6, paddingVertical: 4 },
  clearBtnText:     { fontSize: 14, fontWeight: '600' },

  // Group
  group:            { marginBottom: 4 },
  groupHeader:      { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 20, paddingVertical: 10, marginHorizontal: 16, marginBottom: 4, borderRadius: 12 },
  groupHeaderLeft:  { flexDirection: 'row', alignItems: 'center', gap: 8, flex: 1, marginRight: 12 },
  groupDot:         { width: 8, height: 8, borderRadius: 4, backgroundColor: '#ff2d55' },
  groupTitle:       { fontSize: 13, fontWeight: '700', letterSpacing: 0.2, flex: 1 },
  groupSubtotal:    { fontSize: 13, fontWeight: '600' },

  // Item row
  itemRow: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    paddingHorizontal: 16,
    paddingVertical: 14,
    marginHorizontal: 16,
    marginBottom: 8,
    borderRadius: 14,
    borderWidth: StyleSheet.hairlineWidth,
    gap: 12,
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

  // Stepper
  stepper:          { flexDirection: 'row', alignItems: 'center', borderRadius: 10, borderWidth: StyleSheet.hairlineWidth, overflow: 'hidden' },
  stepperBtn:       { width: 32, height: 32, alignItems: 'center', justifyContent: 'center' },
  stepperIcon:      { fontSize: 16, fontWeight: '600' },
  stepperCount:     { width: 36, alignItems: 'center', justifyContent: 'center', height: 32 },
  stepperCountText: { fontSize: 15, fontWeight: '700' },
  lineTotal:        { fontSize: 15, fontWeight: '800', color: '#ff2d55' },

  // Summary card
  summaryCard: {
    marginHorizontal: 16,
    marginTop: 8,
    marginBottom: 8,
    borderRadius: 16,
    borderWidth: StyleSheet.hairlineWidth,
    padding: 18,
    gap: 10,
  },
  summaryTitle:     { fontSize: 11, fontWeight: '800', letterSpacing: 1.2, marginBottom: 2 },
  summaryRow:       { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  summaryLabel:     { fontSize: 14, fontWeight: '500', flex: 1, marginRight: 12 },
  summaryValue:     { fontSize: 14, fontWeight: '600' },
  summaryDivider:   { height: StyleSheet.hairlineWidth, marginVertical: 4 },
  summaryTotalRow:  { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  summaryTotalLabel:{ fontSize: 16, fontWeight: '700' },
  summaryTotalValue:{ fontSize: 18, fontWeight: '800', color: '#ff2d55' },

  // Info pill
  infoPill:         { flexDirection: 'row', alignItems: 'center', gap: 8, marginHorizontal: 16, padding: 12, borderRadius: 10, marginBottom: 8 },
  infoPillText:     { fontSize: 12, fontWeight: '500', flex: 1 },

  // CTA
  ctaBar: {
    position: 'absolute', bottom: 0, left: 0, right: 0,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 20,
    paddingTop: 14,
    borderTopWidth: StyleSheet.hairlineWidth,
  },
  ctaAmountWrap:    { gap: 2 },
  ctaAmountLabel:   { fontSize: 12, fontWeight: '600' },
  ctaAmount:        { fontSize: 22, fontWeight: '800', letterSpacing: -0.5 },
  checkoutBtn:      { flexDirection: 'row', alignItems: 'center', gap: 8, backgroundColor: '#ff2d55', paddingHorizontal: 24, paddingVertical: 14, borderRadius: 14 },
  checkoutBtnText:  { fontSize: 16, fontWeight: '800', color: '#fff', letterSpacing: 0.2 },

  // Empty
  emptyWrap:        { flex: 1, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 40, gap: 14 },
  emptyIconCircle:  { width: 96, height: 96, borderRadius: 48, alignItems: 'center', justifyContent: 'center', marginBottom: 4 },
  emptyTitle:       { fontSize: 22, fontWeight: '800', letterSpacing: -0.3 },
  emptySub:         { fontSize: 14, fontWeight: '500', textAlign: 'center', lineHeight: 20 },
  emptyBtn:         { backgroundColor: '#ff2d55', paddingHorizontal: 28, paddingVertical: 13, borderRadius: 12, marginTop: 6 },
  emptyBtnText:     { fontSize: 15, fontWeight: '800', color: '#fff' },
});