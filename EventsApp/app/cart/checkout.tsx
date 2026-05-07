/**
 * app/cart/checkout.tsx
 *
 * Cart checkout screen. Flow:
 *  1. Loads cart summary from GET /v1/cart
 *  2. On "Pay Now" → POST /v1/checkout/begin  (creates ticket reservations,
 *     returns seconds_left + cart_total)
 *  3. POST /v1/checkout/confirm  (converts reservations → bookings, creates
 *     Stitch payment intent, returns client_token)
 *  4. Navigates to /payment/[id] with the token — same WebView path as the
 *     single-event purchase flow.
 *  5. Sends a heartbeat every 30 s while the user is on this screen so the
 *     reservation window stays open.
 *  6. On unmount (back / close) → POST /v1/checkout/abandon to release seats.
 */

import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  ActivityIndicator,
  Alert,
  Animated,
  StyleSheet,
  Platform,
  Modal,
  TextInput,
  KeyboardAvoidingView,
  StatusBar,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { Feather, Ionicons } from '@expo/vector-icons';
import * as SecureStore from 'expo-secure-store';
import { useAppTheme } from '../../theme/colors';
import { useAuthStore } from '../../store/authStore';
import { api } from '../../lib/api';

// ─── Types ────────────────────────────────────────────────────────────────────

interface BackendCartItem {
  id: string;
  item_type: 'ticket' | 'merchandise';
  event_id: string;
  ticket_type_id?: string | null;
  ticket_type_name?: string | null;
  merchandise_variant_id?: string | null;
  quantity: number;
  unit_price_snapshot: number;
  delivery_fee_snapshot: number;
  fulfillment_method?: string | null;
  event_title?: string | null;
  merch_name?: string | null;
  variant_sku?: string | null;
  variant_attributes?: string | null;
  pickup_location?: string | null;
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

interface ReservationSummary {
  reservation_id: string;
  event_id: string;
  event_title: string;
  quantity: number;
  seconds_left: number;
}

interface CheckoutBeginResponse {
  reservations: ReservationSummary[];
  seconds_left: number;
  cart_total: number;
  currency: string;
}

type Phase = 'review' | 'reserving' | 'reserved' | 'confirming';
type PaymentMethod = 'card' | 'eft';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function fmtPrice(n: number): string {
  return `R${Math.round(n).toLocaleString('en-ZA')}`;
}

function fmtCountdown(secs: number): string {
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  return `${m}:${s.toString().padStart(2, '0')}`;
}

function itemLabel(item: BackendCartItem): string {
  if (item.item_type === 'ticket') return item.ticket_type_name ?? 'General Admission';
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
    if (item.fulfillment_method === 'delivery') parts.push('📦 Delivery');
    else if (item.fulfillment_method === 'pickup') {
      parts.push(item.pickup_location ? `🏪 Pickup · ${item.pickup_location}` : '🏪 Pickup');
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

function makeAuthHeaders(sid: string | null): Record<string, string> {
  return sid ? { 'X-Session-ID': sid } : {};
}

// ─── Countdown Timer ─────────────────────────────────────────────────────────

function useCountdown(initialSecs: number, active: boolean) {
  const [secs, setSecs] = useState(initialSecs);
  useEffect(() => {
    if (!active || secs <= 0) return;
    const t = setInterval(() => setSecs((s) => Math.max(0, s - 1)), 1000);
    return () => clearInterval(t);
  }, [active, secs]);
  useEffect(() => { if (active) setSecs(initialSecs); }, [initialSecs]);
  return secs;
}

// ─── Section: Order Line Items ────────────────────────────────────────────────

function OrderLine({ item, colors }: { item: BackendCartItem; colors: any }) {
  const isTicket = item.item_type === 'ticket';
  const lineTotal = item.unit_price_snapshot * item.quantity + item.delivery_fee_snapshot;
  const sub = itemSub(item);

  return (
    <View style={styles.orderLine}>
      <View style={[styles.orderLineIcon, isTicket ? styles.iconTicket : styles.iconMerch]}>
        {isTicket
          ? <Ionicons name="ticket-outline" size={13} color="#fff" />
          : <Feather name="shopping-bag" size={12} color="#fff" />
        }
      </View>
      <View style={styles.orderLineBody}>
        <Text style={[styles.orderLineName, { color: colors.text }]} numberOfLines={2}>
          {itemLabel(item)}
        </Text>
        {sub ? (
          <Text style={[styles.orderLineSub, { color: colors.subText }]} numberOfLines={1}>
            {sub}
          </Text>
        ) : null}
        <Text style={[styles.orderLineQty, { color: colors.subText }]}>
          {fmtPrice(item.unit_price_snapshot)} × {item.quantity}
          {item.delivery_fee_snapshot > 0 ? `  +  ${fmtPrice(item.delivery_fee_snapshot)} delivery` : ''}
        </Text>
      </View>
      <Text style={[styles.orderLineTotal, { color: colors.text }]}>
        {fmtPrice(lineTotal)}
      </Text>
    </View>
  );
}

// ─── Reservation Timer Banner ─────────────────────────────────────────────────

function ReservationBanner({ secondsLeft, colors }: { secondsLeft: number; colors: any }) {
  const isUrgent = secondsLeft <= 120;
  const pulseAnim = useRef(new Animated.Value(1)).current;

  useEffect(() => {
    if (!isUrgent) return;
    Animated.loop(
      Animated.sequence([
        Animated.timing(pulseAnim, { toValue: 0.6, duration: 600, useNativeDriver: true }),
        Animated.timing(pulseAnim, { toValue: 1, duration: 600, useNativeDriver: true }),
      ])
    ).start();
    return () => pulseAnim.stopAnimation();
  }, [isUrgent]);

  return (
    <View style={[styles.resBanner, { backgroundColor: isUrgent ? '#fff1f3' : '#f0f7ff', borderColor: isUrgent ? '#ff2d55' : '#3b82f6' }]}>
      <Animated.View style={{ opacity: isUrgent ? pulseAnim : 1 }}>
        <Ionicons
          name={isUrgent ? 'timer-outline' : 'shield-checkmark-outline'}
          size={20}
          color={isUrgent ? '#ff2d55' : '#3b82f6'}
        />
      </Animated.View>
      <View style={styles.resBannerText}>
        <Text style={[styles.resBannerTitle, { color: isUrgent ? '#ff2d55' : '#1d4ed8' }]}>
          {isUrgent ? 'Seats reserved — hurry!' : 'Seats reserved for you'}
        </Text>
        <Text style={[styles.resBannerSub, { color: isUrgent ? '#e11d48' : '#2563eb' }]}>
          {fmtCountdown(secondsLeft)} remaining to complete payment
        </Text>
      </View>
    </View>
  );
}

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function CartCheckoutScreen() {
  const router          = useRouter();
  const insets          = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();
  const { user }        = useAuthStore();

  // Cart state
  const [cart, setCart]           = useState<CartSummary | null>(null);
  const [loading, setLoading]     = useState(true);

  // Checkout phase
  const [phase, setPhase]         = useState<Phase>('review');
  const [reservations, setReservations] = useState<ReservationSummary[]>([]);
  const [reservedTotal, setReservedTotal] = useState(0);
  const [reservedCurrency, setReservedCurrency] = useState('ZAR');
  const [initialSecs, setInitialSecs] = useState(600);

  // Countdown (only ticks when phase === 'reserved')
  const secondsLeft = useCountdown(initialSecs, phase === 'reserved');

  // Heartbeat interval ref
  const heartbeatRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Contact for guest users
  const [guestName, setGuestName]   = useState('');
  const [guestEmail, setGuestEmail] = useState('');
  const [contactOpen, setContactOpen] = useState(false);
  const contactSlide = useRef(new Animated.Value(400)).current;

  // Payment method
  const [payMethod, setPayMethod]   = useState<PaymentMethod>('card');
  const [paySheetOpen, setPaySheetOpen] = useState(false);
  const paySlide = useRef(new Animated.Value(400)).current;

  const sessionIdRef = useRef<string | null>(null);

  // ── Load ──────────────────────────────────────────────────────────────────

  useEffect(() => {
    (async () => {
      sessionIdRef.current = await getSessionId();
      try {
        const headers = makeAuthHeaders(sessionIdRef.current);
        const res = await api.get<CartSummary>('/v1/cart', { headers });
        setCart(res.data);
      } catch {
        Alert.alert('Error', 'Could not load your cart.');
        router.back();
      } finally {
        setLoading(false);
      }
    })();

    // Abandon reservations when leaving screen
    return () => {
      if (heartbeatRef.current) clearInterval(heartbeatRef.current);
      const sid = sessionIdRef.current;
      api.post('/v1/checkout/abandon', {}, { headers: makeAuthHeaders(sid) }).catch(() => {});
    };
  }, []);

  // ── Heartbeat ─────────────────────────────────────────────────────────────

  useEffect(() => {
    if (phase !== 'reserved') {
      if (heartbeatRef.current) { clearInterval(heartbeatRef.current); heartbeatRef.current = null; }
      return;
    }
    heartbeatRef.current = setInterval(async () => {
      try {
        await api.post('/v1/checkout/heartbeat', {}, { headers: makeAuthHeaders(sessionIdRef.current) });
      } catch {
        // heartbeat failed — reservation may have expired
      }
    }, 30_000);
    return () => { if (heartbeatRef.current) clearInterval(heartbeatRef.current); };
  }, [phase]);

  // ── Reservation expiry ────────────────────────────────────────────────────

  useEffect(() => {
    if (phase === 'reserved' && secondsLeft === 0) {
      Alert.alert(
        'Reservation expired',
        'Your seat reservation has expired. Your cart is intact — you can try again.',
        [{ text: 'OK', onPress: () => { setPhase('review'); setReservations([]); } }]
      );
    }
  }, [secondsLeft, phase]);

  // ── Modal helpers ─────────────────────────────────────────────────────────

  const openContact = () => {
    setContactOpen(true);
    Animated.spring(contactSlide, { toValue: 0, useNativeDriver: true, bounciness: 4 }).start();
  };
  const closeContact = () => {
    Animated.timing(contactSlide, { toValue: 400, duration: 220, useNativeDriver: true }).start(() => setContactOpen(false));
  };
  const openPay = () => {
    setPaySheetOpen(true);
    Animated.spring(paySlide, { toValue: 0, useNativeDriver: true, bounciness: 4 }).start();
  };
  const closePay = () => {
    Animated.timing(paySlide, { toValue: 400, duration: 220, useNativeDriver: true }).start(() => setPaySheetOpen(false));
  };

  // ── Begin checkout (reserves tickets) ─────────────────────────────────────

  const handleBeginCheckout = async () => {
    // Guest validation
    if (!user) {
      if (!guestName.trim()) { Alert.alert('Name required', 'Please add your contact info before checking out.'); return; }
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(guestEmail.trim())) { Alert.alert('Valid email required', 'Please enter a valid email address.'); return; }
    }

    setPhase('reserving');
    try {
      const headers = makeAuthHeaders(sessionIdRef.current);
      const res = await api.post<CheckoutBeginResponse>('/v1/checkout/begin', {}, { headers });
      const data = res.data;
      setReservations(data.reservations ?? []);
      setReservedTotal(data.cart_total);
      setReservedCurrency(data.currency ?? 'ZAR');
      setInitialSecs(data.seconds_left ?? 600);
      setPhase('reserved');
    } catch (err: any) {
      setPhase('review');
      const msg = err?.response?.data?.error ?? 'Could not reserve your tickets. Please try again.';
      Alert.alert('Checkout failed', msg);
    }
  };

  // ── Confirm + pay ──────────────────────────────────────────────────────────

  const handleConfirm = async () => {
    setPhase('confirming');
    try {
      const headers = makeAuthHeaders(sessionIdRef.current);
      const res = await api.post<{
        client_token: string;
        payment_request_id: string;
        total_amount: number;
        currency: string;
        primary_booking_id: string;
        booking_ids: string[];
      }>('/v1/checkout/confirm', {}, { headers });

      const { client_token, total_amount, currency, primary_booking_id } = res.data;

      // Navigate to payment WebView — same screen as direct purchase flow
      router.push({
        pathname: '/payment/[id]',
        params: {
          id:       primary_booking_id,
          token:    client_token,
          amount:   String(total_amount),
          currency: currency,
        },
      } as any);
    } catch (err: any) {
      setPhase('reserved');
      const msg = err?.response?.data?.error ?? 'Payment setup failed. Please try again.';
      Alert.alert('Error', msg);
    }
  };

  // ── Derived ───────────────────────────────────────────────────────────────

  const allItems   = cart?.items_by_event?.flatMap((g) => g.items) ?? [];
  const ticketItems = allItems.filter((i) => i.item_type === 'ticket');
  const merchItems  = allItems.filter((i) => i.item_type === 'merchandise');
  const cartTotal  = cart?.total_amount ?? 0;
  const platformFee = Math.ceil(cartTotal * 0.05);
  const grandTotal  = cartTotal + platformFee;

  const hasTickets  = ticketItems.length > 0;
  const hasMerch    = merchItems.length > 0;

  const isReserved  = phase === 'reserved';
  const isReserving = phase === 'reserving';
  const isConfirming = phase === 'confirming';
  const isBusy      = isReserving || isConfirming;

  const bg = isDark ? '#0a0a0a' : '#f5f5f7';

  // ── Render ────────────────────────────────────────────────────────────────

  if (loading) {
    return (
      <View style={[styles.screen, { backgroundColor: bg, paddingTop: insets.top, alignItems: 'center', justifyContent: 'center', gap: 12 }]}>
        <ActivityIndicator size="large" color="#ff2d55" />
        <Text style={[styles.loadingText, { color: colors.subText }]}>Loading checkout…</Text>
      </View>
    );
  }

  const userDisplayName = user
    ? (`${(user as any).first_name ?? ''} ${(user as any).last_name ?? ''}`.trim() || (user as any).email)
    : (guestName || guestEmail);

  return (
    <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <View style={[styles.screen, { backgroundColor: bg }]}>
        <StatusBar barStyle={isDark ? 'light-content' : 'dark-content'} />

        {/* ── Header ─────────────────────────────────────────────────── */}
        <View style={[styles.header, { paddingTop: insets.top + 8, backgroundColor: isDark ? '#0a0a0a' : '#fff', borderBottomColor: colors.dividerColor }]}>
          <TouchableOpacity
            onPress={() => {
              if (isReserved || isConfirming) {
                Alert.alert('Leave checkout?', 'Your seat reservations will be released.', [
                  { text: 'Stay', style: 'cancel' },
                  { text: 'Leave', style: 'destructive', onPress: () => router.back() },
                ]);
              } else {
                router.back();
              }
            }}
            style={styles.backBtn}
            hitSlop={{ top: 10, bottom: 10, left: 10, right: 10 }}
          >
            <Feather name="arrow-left" size={22} color={colors.text} />
          </TouchableOpacity>

          <View style={styles.headerCenter}>
            <Text style={[styles.headerTitle, { color: colors.text }]}>Checkout</Text>
            {isReserved && (
              <View style={styles.headerTimerPill}>
                <Text style={styles.headerTimerText}>{fmtCountdown(secondsLeft)}</Text>
              </View>
            )}
          </View>

          {/* Phase indicator dots */}
          <View style={styles.phaseDots}>
            {(['review', 'reserved', 'confirming'] as Phase[]).map((p, i) => (
              <View
                key={p}
                style={[
                  styles.phaseDot,
                  (phase === p || (p === 'reserved' && phase === 'reserving')) && styles.phaseDotActive,
                  i < (['review', 'reserved', 'confirming'] as Phase[]).indexOf(phase) && styles.phaseDotDone,
                ]}
              />
            ))}
          </View>
        </View>

        {/* ── Scroll Body ─────────────────────────────────────────────── */}
        <ScrollView
          style={styles.scroll}
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingBottom: 160, paddingTop: 16 }}
          keyboardShouldPersistTaps="handled"
        >

          {/* ── Reservation banner (shows after begin) ─────────── */}
          {isReserved && (
            <View style={styles.sectionPad}>
              <ReservationBanner secondsLeft={secondsLeft} colors={colors} />
            </View>
          )}

          {/* ── Contact details ────────────────────────────────── */}
          <View style={[styles.card, { backgroundColor: isDark ? '#111' : '#fff', borderColor: colors.dividerColor }]}>
            <Text style={[styles.cardLabel, { color: colors.subText }]}>CONTACT</Text>
            <TouchableOpacity
              style={styles.detailRow}
              onPress={!user ? openContact : undefined}
              activeOpacity={!user ? 0.7 : 1}
            >
              <Feather name="user" size={17} color={colors.subText} />
              <View style={styles.detailBody}>
                {userDisplayName ? (
                  <Text style={[styles.detailValue, { color: colors.text }]}>{userDisplayName}</Text>
                ) : (
                  <Text style={[styles.detailValue, { color: '#ff2d55' }]}>Add contact info</Text>
                )}
              </View>
              {!user && <Feather name="chevron-right" size={17} color={colors.subText} />}
            </TouchableOpacity>
          </View>

          {/* ── Payment method ─────────────────────────────────── */}
          <View style={[styles.card, { backgroundColor: isDark ? '#111' : '#fff', borderColor: colors.dividerColor }]}>
            <Text style={[styles.cardLabel, { color: colors.subText }]}>PAYMENT</Text>
            <TouchableOpacity style={styles.detailRow} onPress={openPay} activeOpacity={0.7}>
              <Text style={styles.payMethodEmoji}>
                {payMethod === 'card' ? '💳' : '🏦'}
              </Text>
              <View style={styles.detailBody}>
                <Text style={[styles.detailValue, { color: colors.text }]}>
                  {payMethod === 'card' ? 'Credit / Debit Card' : 'EFT / Instant Pay'}
                </Text>
              </View>
              <Feather name="chevron-right" size={17} color={colors.subText} />
            </TouchableOpacity>
          </View>

          {/* ── Order items ────────────────────────────────────── */}
          <View style={[styles.card, { backgroundColor: isDark ? '#111' : '#fff', borderColor: colors.dividerColor }]}>
            <Text style={[styles.cardLabel, { color: colors.subText }]}>YOUR ORDER</Text>

            {cart?.items_by_event.map((group) => (
              <View key={group.event_id}>
                <View style={styles.groupHeaderInCard}>
                  <View style={styles.groupDot} />
                  <Text style={[styles.groupTitleInCard, { color: colors.text }]} numberOfLines={1}>
                    {group.event_title}
                  </Text>
                </View>
                {group.items.map((item) => (
                  <OrderLine key={item.id} item={item} colors={colors} />
                ))}
              </View>
            ))}
          </View>

          {/* ── Reservations summary (after begin) ─────────────── */}
          {isReserved && reservations.length > 0 && (
            <View style={[styles.card, { backgroundColor: isDark ? '#111' : '#fff', borderColor: colors.dividerColor }]}>
              <Text style={[styles.cardLabel, { color: colors.subText }]}>RESERVATIONS CONFIRMED</Text>
              {reservations.map((r) => (
                <View key={r.reservation_id} style={styles.reservationRow}>
                  <Ionicons name="shield-checkmark-outline" size={16} color="#22c55e" />
                  <Text style={[styles.reservationText, { color: colors.text }]} numberOfLines={1}>
                    {r.event_title} — {r.quantity} ticket{r.quantity !== 1 ? 's' : ''}
                  </Text>
                </View>
              ))}
            </View>
          )}

          {/* ── Trust signals ──────────────────────────────────── */}
          <View style={[styles.trustRow, { borderColor: colors.dividerColor }]}>
            <View style={styles.trustItem}>
              <Ionicons name="shield-checkmark-outline" size={18} color={colors.subText} />
              <Text style={[styles.trustText, { color: colors.subText }]}>Official tickets</Text>
            </View>
            <View style={[styles.trustDivider, { backgroundColor: colors.dividerColor }]} />
            <View style={styles.trustItem}>
              <Feather name="lock" size={16} color={colors.subText} />
              <Text style={[styles.trustText, { color: colors.subText }]}>Secure checkout</Text>
            </View>
            <View style={[styles.trustDivider, { backgroundColor: colors.dividerColor }]} />
            <View style={styles.trustItem}>
              <Feather name="smartphone" size={16} color={colors.subText} />
              <Text style={[styles.trustText, { color: colors.subText }]}>Mobile tickets</Text>
            </View>
          </View>

          {/* ── Price breakdown ────────────────────────────────── */}
          <View style={[styles.card, { backgroundColor: isDark ? '#111' : '#fff', borderColor: colors.dividerColor }]}>
            <Text style={[styles.cardLabel, { color: colors.subText }]}>PRICE BREAKDOWN</Text>

            {allItems.map((item) => (
              <View key={item.id} style={styles.priceRow}>
                <Text style={[styles.priceLabel, { color: colors.text }]} numberOfLines={1}>
                  {itemLabel(item)} × {item.quantity}
                </Text>
                <Text style={[styles.priceValue, { color: colors.text }]}>
                  {fmtPrice(item.unit_price_snapshot * item.quantity)}
                </Text>
              </View>
            ))}

            {allItems.some((i) => i.delivery_fee_snapshot > 0) && (
              <View style={styles.priceRow}>
                <Text style={[styles.priceLabel, { color: colors.subText }]}>Delivery fees</Text>
                <Text style={[styles.priceValue, { color: colors.subText }]}>
                  {fmtPrice(allItems.reduce((s, i) => s + i.delivery_fee_snapshot, 0))}
                </Text>
              </View>
            )}

            <View style={styles.priceRow}>
              <Text style={[styles.priceLabel, { color: colors.subText }]}>Service fee (5%)</Text>
              <Text style={[styles.priceValue, { color: colors.subText }]}>{fmtPrice(platformFee)}</Text>
            </View>

            <View style={[styles.priceDivider, { backgroundColor: colors.dividerColor }]} />

            <View style={styles.priceTotalRow}>
              <Text style={[styles.priceTotalLabel, { color: colors.text }]}>Total</Text>
              <Text style={styles.priceTotalValue}>{fmtPrice(grandTotal)}</Text>
            </View>
          </View>

        </ScrollView>

        {/* ── Sticky CTA ─────────────────────────────────────────────── */}
        <View style={[styles.ctaBar, {
          paddingBottom: insets.bottom > 0 ? insets.bottom + 4 : 20,
          backgroundColor: isDark ? '#0a0a0a' : '#fff',
          borderTopColor: colors.dividerColor,
        }]}>

          <View style={styles.ctaAmountWrap}>
            <Text style={[styles.ctaLabel, { color: colors.subText }]}>Total due</Text>
            <Text style={[styles.ctaAmount, { color: colors.text }]}>{fmtPrice(grandTotal)}</Text>
          </View>

          {/* Review → Reserve tickets */}
          {phase === 'review' && (
            <TouchableOpacity
              style={[styles.ctaBtn, isBusy && styles.ctaBtnDisabled]}
              onPress={handleBeginCheckout}
              disabled={isBusy}
              activeOpacity={0.88}
            >
              {isReserving
                ? <ActivityIndicator color="#fff" size="small" />
                : <>
                    <Feather name="lock" size={16} color="#fff" />
                    <Text style={styles.ctaBtnText}>
                      {hasTickets ? 'Reserve & Continue' : 'Continue to Pay'}
                    </Text>
                  </>
              }
            </TouchableOpacity>
          )}

          {/* Reserved → Confirm payment */}
          {phase === 'reserved' && (
            <TouchableOpacity
              style={[styles.ctaBtn, secondsLeft === 0 && styles.ctaBtnDisabled]}
              onPress={handleConfirm}
              disabled={secondsLeft === 0}
              activeOpacity={0.88}
            >
              <>
                <Feather name="credit-card" size={16} color="#fff" />
                <Text style={styles.ctaBtnText}>Pay Now</Text>
              </>
            </TouchableOpacity>
          )}

          {/* Confirming */}
          {phase === 'confirming' && (
            <View style={[styles.ctaBtn, styles.ctaBtnDisabled]}>
              <ActivityIndicator color="#fff" size="small" />
              <Text style={styles.ctaBtnText}>Setting up payment…</Text>
            </View>
          )}
        </View>

        {/* ── Contact modal ────────────────────────────────────────────── */}
        <Modal visible={contactOpen} transparent animationType="none" onRequestClose={closeContact}>
          <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
            <TouchableOpacity style={styles.modalBackdrop} activeOpacity={1} onPress={closeContact} />
            <Animated.View style={[styles.modalSheet, { backgroundColor: isDark ? '#1a1a1a' : '#fff', transform: [{ translateY: contactSlide }] }]}>
              <View style={styles.modalHandle} />
              <View style={styles.modalHeader}>
                <TouchableOpacity onPress={closeContact}><Text style={[styles.modalClose, { color: colors.subText }]}>✕</Text></TouchableOpacity>
                <Text style={[styles.modalTitle, { color: colors.text }]}>Contact Info</Text>
                <TouchableOpacity onPress={() => {
                  if (!guestName.trim()) { Alert.alert('Name required', 'Please enter your full name.'); return; }
                  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(guestEmail.trim())) { Alert.alert('Email required', 'Please enter a valid email address.'); return; }
                  closeContact();
                }}>
                  <Text style={styles.modalDone}>Done</Text>
                </TouchableOpacity>
              </View>

              <View style={[styles.modalField, { borderColor: colors.dividerColor, backgroundColor: isDark ? '#111' : '#f5f5f7' }]}>
                <TextInput
                  style={[styles.modalInput, { color: colors.text }]}
                  placeholder="Email address"
                  placeholderTextColor={colors.subText}
                  value={guestEmail}
                  onChangeText={setGuestEmail}
                  keyboardType="email-address"
                  autoCapitalize="none"
                  autoCorrect={false}
                  autoFocus
                />
              </View>
              <View style={[styles.modalField, { borderColor: colors.dividerColor, backgroundColor: isDark ? '#111' : '#f5f5f7', marginTop: 10 }]}>
                <TextInput
                  style={[styles.modalInput, { color: colors.text }]}
                  placeholder="Full name"
                  placeholderTextColor={colors.subText}
                  value={guestName}
                  onChangeText={setGuestName}
                  autoCapitalize="words"
                  returnKeyType="done"
                  onSubmitEditing={closeContact}
                />
              </View>
              <Text style={[styles.modalFootnote, { color: colors.subText }]}>
                Used to personalise and deliver your tickets.
              </Text>
            </Animated.View>
          </KeyboardAvoidingView>
        </Modal>

        {/* ── Payment method sheet ─────────────────────────────────────── */}
        <Modal visible={paySheetOpen} transparent animationType="none" onRequestClose={closePay}>
          <TouchableOpacity style={styles.modalBackdrop} activeOpacity={1} onPress={closePay} />
          <Animated.View style={[styles.modalSheet, { backgroundColor: isDark ? '#1a1a1a' : '#fff', transform: [{ translateY: paySlide }] }]}>
            <View style={styles.modalHandle} />
            <Text style={[styles.paySheetTitle, { color: colors.text }]}>Payment Method</Text>

            {([
              { id: 'card' as PaymentMethod, emoji: '💳', label: 'Credit / Debit Card', sub: 'Visa, Mastercard, Amex' },
              { id: 'eft' as PaymentMethod, emoji: '🏦', label: 'EFT / Instant Pay', sub: 'Stitch instant bank transfer' },
            ] as const).map((opt) => (
              <TouchableOpacity
                key={opt.id}
                style={[styles.payOption, { borderTopColor: colors.dividerColor }]}
                onPress={() => { setPayMethod(opt.id); closePay(); }}
                activeOpacity={0.7}
              >
                <Text style={styles.payOptionEmoji}>{opt.emoji}</Text>
                <View style={styles.payOptionBody}>
                  <Text style={[styles.payOptionLabel, { color: colors.text }]}>{opt.label}</Text>
                  <Text style={[styles.payOptionSub, { color: colors.subText }]}>{opt.sub}</Text>
                </View>
                {payMethod === opt.id && (
                  <Ionicons name="checkmark-circle" size={22} color="#ff2d55" />
                )}
              </TouchableOpacity>
            ))}
          </Animated.View>
        </Modal>

      </View>
    </KeyboardAvoidingView>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  screen:       { flex: 1 },
  scroll:       { flex: 1 },
  loadingText:  { fontSize: 13, fontWeight: '600' },
  sectionPad:   { paddingHorizontal: 16, marginBottom: 4 },

  // Header
  header: {
    flexDirection: 'row', alignItems: 'center', paddingHorizontal: 20,
    paddingBottom: 14, borderBottomWidth: StyleSheet.hairlineWidth,
  },
  backBtn:         { width: 36, alignItems: 'flex-start' },
  headerCenter:    { flex: 1, flexDirection: 'row', alignItems: 'center', gap: 10 },
  headerTitle:     { fontSize: 20, fontWeight: '800', letterSpacing: -0.3 },
  headerTimerPill: { backgroundColor: '#ff2d55', borderRadius: 10, paddingHorizontal: 10, paddingVertical: 3 },
  headerTimerText: { fontSize: 13, fontWeight: '800', color: '#fff', fontVariant: ['tabular-nums'] },
  phaseDots:       { flexDirection: 'row', gap: 5 },
  phaseDot:        { width: 6, height: 6, borderRadius: 3, backgroundColor: '#e0e0e0' },
  phaseDotActive:  { backgroundColor: '#ff2d55', width: 16 },
  phaseDotDone:    { backgroundColor: '#22c55e' },

  // Reservation banner
  resBanner: {
    flexDirection: 'row', alignItems: 'center', gap: 12,
    padding: 14, borderRadius: 14, borderWidth: 1, marginBottom: 12,
  },
  resBannerText:   { flex: 1 },
  resBannerTitle:  { fontSize: 14, fontWeight: '800' },
  resBannerSub:    { fontSize: 12, fontWeight: '600', marginTop: 2, fontVariant: ['tabular-nums'] },

  // Card
  card: {
    marginHorizontal: 16, marginBottom: 10, borderRadius: 16,
    borderWidth: StyleSheet.hairlineWidth, padding: 16, gap: 10,
  },
  cardLabel:    { fontSize: 11, fontWeight: '800', letterSpacing: 1.1, marginBottom: 2 },

  // Detail row (contact / payment)
  detailRow:    { flexDirection: 'row', alignItems: 'center', gap: 12 },
  detailBody:   { flex: 1 },
  detailValue:  { fontSize: 15, fontWeight: '600' },
  payMethodEmoji: { fontSize: 20, width: 26, textAlign: 'center' },

  // Group within card
  groupHeaderInCard: { flexDirection: 'row', alignItems: 'center', gap: 8, marginBottom: 6, marginTop: 4 },
  groupDot:          { width: 7, height: 7, borderRadius: 3.5, backgroundColor: '#ff2d55' },
  groupTitleInCard:  { fontSize: 12, fontWeight: '700', flex: 1, letterSpacing: 0.2 },

  // Order line
  orderLine:     { flexDirection: 'row', alignItems: 'flex-start', gap: 10, paddingVertical: 8, borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: 'rgba(0,0,0,0.06)' },
  orderLineIcon: { width: 26, height: 26, borderRadius: 7, alignItems: 'center', justifyContent: 'center', marginTop: 1 },
  iconTicket:    { backgroundColor: '#ff2d55' },
  iconMerch:     { backgroundColor: '#1c2e61' },
  orderLineBody: { flex: 1, gap: 2 },
  orderLineName: { fontSize: 14, fontWeight: '700', lineHeight: 19 },
  orderLineSub:  { fontSize: 12, fontWeight: '500' },
  orderLineQty:  { fontSize: 11, fontWeight: '500' },
  orderLineTotal:{ fontSize: 14, fontWeight: '800', minWidth: 60, textAlign: 'right' },

  // Reservations
  reservationRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  reservationText:{ fontSize: 14, fontWeight: '600', flex: 1 },

  // Trust
  trustRow: {
    flexDirection: 'row', alignItems: 'center', marginHorizontal: 16,
    marginBottom: 10, borderRadius: 12, borderWidth: StyleSheet.hairlineWidth,
    padding: 14,
  },
  trustItem:    { flex: 1, alignItems: 'center', gap: 5 },
  trustDivider: { width: StyleSheet.hairlineWidth, height: 28 },
  trustText:    { fontSize: 11, fontWeight: '600', textAlign: 'center' },

  // Price
  priceRow:       { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  priceLabel:     { fontSize: 14, fontWeight: '500', flex: 1, marginRight: 12 },
  priceValue:     { fontSize: 14, fontWeight: '600' },
  priceDivider:   { height: StyleSheet.hairlineWidth, marginVertical: 8 },
  priceTotalRow:  { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  priceTotalLabel:{ fontSize: 17, fontWeight: '700' },
  priceTotalValue:{ fontSize: 20, fontWeight: '800', color: '#ff2d55' },

  // CTA
  ctaBar: {
    position: 'absolute', bottom: 0, left: 0, right: 0,
    flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between',
    paddingHorizontal: 20, paddingTop: 14,
    borderTopWidth: StyleSheet.hairlineWidth,
  },
  ctaAmountWrap: { gap: 2 },
  ctaLabel:      { fontSize: 12, fontWeight: '600' },
  ctaAmount:     { fontSize: 22, fontWeight: '800', letterSpacing: -0.5 },
  ctaBtn:        { flexDirection: 'row', alignItems: 'center', gap: 8, backgroundColor: '#ff2d55', paddingHorizontal: 22, paddingVertical: 14, borderRadius: 14 },
  ctaBtnDisabled:{ opacity: 0.5 },
  ctaBtnText:    { fontSize: 15, fontWeight: '800', color: '#fff', letterSpacing: 0.2 },

  // Modal
  modalBackdrop: { ...StyleSheet.absoluteFillObject, backgroundColor: 'rgba(0,0,0,0.5)' },
  modalSheet: {
    position: 'absolute', bottom: 0, left: 0, right: 0,
    borderTopLeftRadius: 22, borderTopRightRadius: 22,
    paddingBottom: 40, paddingHorizontal: 20,
  },
  modalHandle:   { width: 36, height: 4, borderRadius: 2, backgroundColor: '#ddd', alignSelf: 'center', marginTop: 12, marginBottom: 8 },
  modalHeader:   { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingVertical: 12 },
  modalClose:    { fontSize: 17, fontWeight: '600', width: 32 },
  modalTitle:    { fontSize: 17, fontWeight: '800' },
  modalDone:     { fontSize: 16, fontWeight: '700', color: '#ff2d55' },
  modalField:    { borderRadius: 12, borderWidth: StyleSheet.hairlineWidth, paddingHorizontal: 14 },
  modalInput:    { height: 48, fontSize: 15, fontWeight: '500' },
  modalFootnote: { fontSize: 12, fontWeight: '500', marginTop: 14, lineHeight: 17 },

  // Pay sheet
  paySheetTitle: { fontSize: 18, fontWeight: '800', marginTop: 8, marginBottom: 8 },
  payOption:     { flexDirection: 'row', alignItems: 'center', paddingVertical: 16, borderTopWidth: StyleSheet.hairlineWidth, gap: 14 },
  payOptionEmoji:{ fontSize: 22, width: 28, textAlign: 'center' },
  payOptionBody: { flex: 1 },
  payOptionLabel:{ fontSize: 16, fontWeight: '600' },
  payOptionSub:  { fontSize: 12, fontWeight: '500', marginTop: 2 },
});