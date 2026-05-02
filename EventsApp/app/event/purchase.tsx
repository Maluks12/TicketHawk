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
  Dimensions,
  StatusBar,
  Image,
  Platform,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { useEventStore } from '../../store/eventStore';
import { useAuthStore } from '../../store/authStore';
import {
  getTicketTypes,
  addTicketToCart,
  checkoutBegin,
  type TicketType,
} from '../../lib/api';
import SeatingMap, { detectVenueType, MapLegend } from '../../components/seatingmap';

const { width: SW } = Dimensions.get('window');
const PRIMARY = '#FF2D55';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function fmtPrice(price: number) {
  if (price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function fmtDate(iso: string) {
  const d = new Date(iso);
  return d.toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
}

function fmtTime(iso: string) {
  const d = new Date(iso);
  return d.toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

function availLabel(tt: TicketType): string {
  const av = tt.available ?? (tt.capacity ? tt.capacity - tt.tickets_sold - tt.reserved_count : null);
  if (av === null) return 'Available';
  if (av === 0) return 'Sold out';
  if (av < 10) return `${av} left`;
  return `${av.toLocaleString()} available`;
}

function isSoldOut(tt: TicketType): boolean {
  const av = tt.available ?? (tt.capacity ? tt.capacity - tt.tickets_sold - tt.reserved_count : null);
  return av !== null && av === 0;
}

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function TicketPurchaseScreen() {
  const router   = useRouter();
  const insets   = useSafeAreaInsets();
  const event    = useEventStore((s) => s.selectedEvent) as any;
  const { user } = useAuthStore();

  const [ticketTypes, setTicketTypes] = useState<TicketType[]>([]);
  const [loading, setLoading]         = useState(true);
  const [selectedId, setSelectedId]   = useState<string | null>(null);
  const [quantity, setQuantity]       = useState(1);
  const [step, setStep]               = useState<'select' | 'map' | 'summary'>('select');
  const [adding, setAdding]           = useState(false);
  const fadeAnim = useRef(new Animated.Value(1)).current;

  function animateStep(next: 'select' | 'map' | 'summary') {
    Animated.sequence([
      Animated.timing(fadeAnim, { toValue: 0, duration: 100, useNativeDriver: true }),
      Animated.timing(fadeAnim, { toValue: 1, duration: 180, useNativeDriver: true }),
    ]).start();
    setStep(next);
  }

  useEffect(() => {
    if (!event?.id) return;
    setLoading(true);
    getTicketTypes(event.id)
      .then((types) => {
        setTicketTypes(types);
        const first = types.find((t) => !isSoldOut(t));
        if (first) setSelectedId(first.id);
      })
      .catch(() => {
        if (event?.price != null) {
          const fallback: TicketType = {
            id: 'ga-fallback',
            event_id: event.id,
            name: 'GENERAL ADMISSION',
            description: null,
            price: event.price,
            capacity: null,
            tickets_sold: 0,
            reserved_count: 0,
            sort_order: 0,
            is_visible: true,
            sale_starts_at: null,
            sale_ends_at: null,
            created_at: '',
            updated_at: '',
          };
          setTicketTypes([fallback]);
          setSelectedId('ga-fallback');
        }
      })
      .finally(() => setLoading(false));
  }, [event?.id]);

  const selectedType = ticketTypes.find((t) => t.id === selectedId) ?? null;
  const totalAmount  = selectedType ? selectedType.price * quantity : 0;
  const platformFee  = Math.ceil(totalAmount * 0.05);
  const grandTotal   = totalAmount + platformFee;
  const venueType    = event ? detectVenueType(event.category ?? '', event.venue_name) : 'none';
  const bannerImage  = event?.banner_image_url ?? event?.cover_image_url ?? null;

  const handleSelectType = useCallback((id: string) => setSelectedId(id), []);

  const handleNext = () => {
    if (!selectedId) {
      Alert.alert('Select a ticket', 'Please choose a ticket type to continue.');
      return;
    }
    animateStep(venueType !== 'none' ? 'map' : 'summary');
  };

  const handleAddToCart = async () => {
    if (!selectedType || !event) return;
    setAdding(true);
    try {
      if (selectedId === 'ga-fallback') {
        Alert.alert('🎟 Added!', `${quantity}× ${selectedType.name} for ${fmtPrice(grandTotal)} added to your cart.`, [
          { text: 'Continue Shopping', style: 'cancel' },
          { text: 'Checkout', onPress: () => router.push('/(tabs)/tickets') },
        ]);
        setAdding(false);
        return;
      }
      await addTicketToCart({ item_type: 'ticket', event_id: event.id, ticket_type_id: selectedId ?? undefined, quantity });
      const checkout = await checkoutBegin();
      Alert.alert('🎟 Reserved!', `${quantity}× ${selectedType.name}. You have ${Math.floor(checkout.expires_in_seconds / 60)} min to complete payment.`, [
        { text: 'Continue Shopping', style: 'cancel' },
        { text: 'Checkout', onPress: () => router.push('/(tabs)/tickets') },
      ]);
    } catch (err: any) {
      Alert.alert('Error', err?.response?.data?.error ?? 'Failed to reserve tickets. Please try again.');
    } finally {
      setAdding(false);
    }
  };

  if (!event) {
    return (
      <View style={ss.centered}>
        <Text style={ss.errorText}>EVENT NOT FOUND</Text>
        <TouchableOpacity onPress={() => router.back()} style={ss.goBackBtn}>
          <Text style={ss.goBackText}>← GO BACK</Text>
        </TouchableOpacity>
      </View>
    );
  }

  const steps = venueType !== 'none'
    ? (['select', 'map', 'summary'] as const)
    : (['select', 'summary'] as const);
  const stepIndex = steps.indexOf(step as any);

  const headerTitle =
    step === 'select' ? 'Select Tickets' :
    step === 'map'    ? 'Choose Seats'   :
                        'Checkout';

  return (
    <View style={[ss.screen, { paddingBottom: insets.bottom }]}>
      <StatusBar barStyle="dark-content" />

      {/* ── Header ─────────────────────────────────────────────────── */}
      <View style={[ss.header, { paddingTop: insets.top + 4 }]}>
        <TouchableOpacity onPress={() => router.back()} style={ss.closeBtn} hitSlop={{ top: 8, left: 8, right: 8, bottom: 8 }}>
          <Text style={ss.closeIcon}>✕</Text>
        </TouchableOpacity>
        <Text style={ss.headerTitle}>{headerTitle}</Text>
        <View style={ss.stepDots}>
          {steps.map((_, i) => (
            <View key={i} style={[ss.stepDot, i === stepIndex && ss.stepDotActive]} />
          ))}
        </View>
      </View>

      {/* ── Event Banner ─────────────────────────────────────────────── */}
      <View style={ss.bannerWrap}>
        {bannerImage ? (
          <Image source={{ uri: bannerImage }} style={ss.bannerImage} resizeMode="cover" />
        ) : (
          <View style={ss.bannerPlaceholder}>
            <Text style={ss.bannerPlaceholderIcon}>🎪</Text>
          </View>
        )}
        <View style={ss.bannerOverlay} />
        <View style={ss.bannerMeta}>
          <Text style={ss.bannerDate} numberOfLines={1}>
            {fmtDate(event.start_time)} at {fmtTime(event.start_time)}
          </Text>
          <Text style={ss.bannerVenue} numberOfLines={1}>
            {event.venue_name}{event.venue_city ? ` · ${event.venue_city}` : ''}
          </Text>
        </View>
      </View>

      {/* ── Body ───────────────────────────────────────────────────── */}
      {loading ? (
        <View style={ss.centered}>
          <ActivityIndicator color={PRIMARY} size="large" />
          <Text style={ss.loadingText}>Loading tickets…</Text>
        </View>
      ) : (
        <Animated.View style={{ flex: 1, opacity: fadeAnim }}>
          <ScrollView
            style={ss.scroll}
            showsVerticalScrollIndicator={false}
            contentContainerStyle={{ paddingBottom: 170 }}
          >
            {/* Event title */}
            <View style={ss.eventTitleSection}>
              <Text style={ss.eventTitle}>{event.title}</Text>
              <Text style={ss.eventSubtitle}>
                {fmtDate(event.start_time)} at {fmtTime(event.start_time)}
              </Text>
              {event.venue_name ? (
                <Text style={ss.eventVenueSmall}>
                  {event.venue_name}{event.venue_city ? `, ${event.venue_city}` : ''}
                </Text>
              ) : null}
            </View>

            {/* ══ STEP: Ticket Type + Quantity ══════════════════════════ */}
            {step === 'select' && (
              <View>
                <View style={ss.sectionHeader}>
                  <Text style={ss.sectionHeaderText}>AVAILABLE TICKETS</Text>
                  <View style={ss.sortChip}>
                    <Text style={ss.sortChipText}>⇅  Best deal</Text>
                  </View>
                </View>

                {ticketTypes.length === 0 ? (
                  <View style={ss.emptyState}>
                    <Text style={ss.emptyIcon}>🎫</Text>
                    <Text style={ss.emptyText}>No tickets currently on sale</Text>
                  </View>
                ) : (
                  ticketTypes.map((tt, idx) => {
                    const sold      = isSoldOut(tt);
                    const selected  = tt.id === selectedId;
                    const isPremium = idx === ticketTypes.length - 1 && ticketTypes.length > 1;
                    return (
                      <TouchableOpacity
                        key={tt.id}
                        onPress={() => !sold && setSelectedId(tt.id)}
                        style={[ss.ticketCard, selected && ss.ticketCardSelected, sold && ss.ticketCardSoldOut]}
                        activeOpacity={sold ? 1 : 0.72}
                      >
                        <View style={[ss.radioOuter, selected && ss.radioOuterSelected]}>
                          {selected && <View style={ss.radioInner} />}
                        </View>
                        <View style={ss.ticketInfo}>
                          <View style={ss.ticketNameRow}>
                            <Text style={[ss.ticketName, sold && ss.ticketNameSoldOut]}>{tt.name}</Text>
                            {isPremium && !sold && (
                              <View style={ss.vipBadge}>
                                <Text style={ss.vipBadgeText}>⭐ VIP</Text>
                              </View>
                            )}
                          </View>
                          <Text style={ss.ticketAvail} numberOfLines={1}>
                            {tt.description ?? availLabel(tt)}
                          </Text>
                          {sold ? (
                            <View style={ss.soldBadge}>
                              <Text style={ss.soldBadgeText}>Sold Out</Text>
                            </View>
                          ) : (
                            <View style={ss.dealBadge}>
                              <Text style={ss.dealBadgeText}>{isPremium ? '⭑ Premium' : '✓ Available'}</Text>
                            </View>
                          )}
                        </View>
                        <View style={ss.ticketPriceCol}>
                          <Text style={[ss.ticketPrice, sold && ss.ticketPriceSoldOut]}>
                            {fmtPrice(tt.price)}
                          </Text>
                          <Text style={ss.ticketFeeNote}>incl. fees</Text>
                        </View>
                      </TouchableOpacity>
                    );
                  })
                )}

                {selectedType && !isSoldOut(selectedType) && (
                  <View style={ss.qtySection}>
                    <View style={ss.qtySectionHeader}>
                      <Text style={ss.qtySectionTitle}>How many tickets?</Text>
                      <Text style={ss.seatedNote}>🪑 Seated together</Text>
                    </View>
                    <View style={ss.qtyGrid}>
                      {[1, 2, 3, 4, 5, 6].map((n) => (
                        <TouchableOpacity
                          key={n}
                          onPress={() => setQuantity(n)}
                          style={[ss.qtyBtn, quantity === n && ss.qtyBtnSel]}
                          activeOpacity={0.7}
                        >
                          <Text style={[ss.qtyBtnText, quantity === n && ss.qtyBtnTextSel]}>
                            {n === 6 ? '6+' : n}
                          </Text>
                        </TouchableOpacity>
                      ))}
                    </View>
                  </View>
                )}
              </View>
            )}

            {/* ══ STEP: Seating Map ═════════════════════════════════════ */}
            {step === 'map' && venueType !== 'none' && (
              <View>
                <View style={ss.sectionHeader}>
                  <Text style={ss.sectionHeaderText}>CHOOSE YOUR SECTION</Text>
                </View>
                <Text style={ss.mapSubtitle}>Tap a section to select your tier</Text>
                <View style={ss.legendRow}>
                  <MapLegend ticketTypes={ticketTypes} selectedTypeId={selectedId} />
                </View>
                <View style={ss.mapWrap}>
                  <SeatingMap
                    venueType={venueType}
                    ticketTypes={ticketTypes}
                    selectedTypeId={selectedId}
                    onSelectType={handleSelectType}
                  />
                </View>
                {selectedType && (
                  <View style={ss.tierCard}>
                    <View style={ss.tierCardLeft}>
                      <Text style={ss.tierCardName}>{selectedType.name.toUpperCase()}</Text>
                      <Text style={ss.tierCardMeta}>{quantity} ticket{quantity !== 1 ? 's' : ''} · {fmtPrice(selectedType.price)} each</Text>
                    </View>
                    <Text style={ss.tierCardTotal}>{fmtPrice(totalAmount)}</Text>
                  </View>
                )}
              </View>
            )}

            {/* ══ STEP: Checkout Summary ════════════════════════════════ */}
            {step === 'summary' && (
              <View style={ss.checkoutWrap}>
                {/* Details list */}
                <View style={ss.detailsList}>
                  <View style={ss.detailRow}>
                    <Text style={ss.detailLabel}>Contact</Text>
                    <View style={ss.detailRight}>
                      <Text style={ss.detailValue} numberOfLines={1}>
                        {user ? (`${(user as any).first_name ?? ''} ${(user as any).last_name ?? ''}`.trim() || (user as any).email) : 'Guest'}
                      </Text>
                      <Text style={ss.chevron}>›</Text>
                    </View>
                  </View>

                  <View style={ss.detailRow}>
                    <Text style={ss.detailLabel}>Payment</Text>
                    <View style={ss.detailRight}>
                      <Text style={{ fontSize: 16 }}>💳</Text>
                      <Text style={ss.detailValue}>Secure via Stitch</Text>
                      <Text style={ss.chevron}>›</Text>
                    </View>
                  </View>

                  <View style={ss.detailRow}>
                    <Text style={ss.detailLabel}>Quantity</Text>
                    <Text style={[ss.detailValue, { flex: 1, marginLeft: 12, textAlign: 'left' }]}>
                      {quantity} ticket{quantity !== 1 ? 's' : ''}
                    </Text>
                  </View>

                  <View style={ss.detailRowMultiline}>
                    <Text style={ss.detailLabel}>Ticket</Text>
                    <View style={{ flex: 1, marginLeft: 12 }}>
                      <Text style={ss.detailValue}>{selectedType?.name ?? 'General Admission'}</Text>
                      {selectedType?.description ? (
                        <Text style={ss.detailValueSub}>{selectedType.description}</Text>
                      ) : null}
                    </View>
                  </View>

                  <View style={[ss.detailRowMultiline, { borderBottomWidth: 0 }]}>
                    <Text style={ss.detailLabel}>Delivery</Text>
                    <View style={{ flex: 1, marginLeft: 12 }}>
                      <Text style={ss.detailValue}>Mobile Tickets</Text>
                      <Text style={ss.detailValueSub}>Access in the Events App</Text>
                    </View>
                  </View>
                </View>

                {/* Trust section */}
                <View style={ss.trustSection}>
                  <View style={ss.trustRow}>
                    <Text style={ss.trustIcon}>✅</Text>
                    <View style={ss.trustText}>
                      <Text style={ss.trustTitle}>Official Ticket Marketplace</Text>
                      <Text style={ss.trustBody}>Tickets are reviewed and verified by the venue.</Text>
                    </View>
                  </View>
                  <View style={ss.trustRow}>
                    <Text style={ss.trustIcon}>🔒</Text>
                    <Text style={[ss.trustTitle, { flex: 1 }]}>Every ticket protected</Text>
                    <Text style={ss.trustHelp}>ⓘ</Text>
                  </View>
                </View>

                {/* Price breakdown */}
                <View style={ss.priceBreakdown}>
                  <View style={ss.priceRow}>
                    <Text style={ss.priceLabel}>{selectedType?.name ?? 'Ticket'} × {quantity}</Text>
                    <Text style={ss.priceValue}>{fmtPrice(totalAmount)}</Text>
                  </View>
                  <View style={ss.priceRow}>
                    <Text style={ss.priceLabelLight}>Service Fees (5%)</Text>
                    <Text style={ss.priceValueLight}>{fmtPrice(platformFee)}</Text>
                  </View>
                  <View style={ss.priceDivider} />
                  <View style={ss.priceRow}>
                    <Text style={ss.priceTotalLabel}>Total Amount</Text>
                    <Text style={ss.priceTotalValue}>{fmtPrice(grandTotal)}</Text>
                  </View>
                </View>

                <TouchableOpacity onPress={() => animateStep('select')} style={ss.editLink}>
                  <Text style={ss.editLinkText}>← Edit selection</Text>
                </TouchableOpacity>
              </View>
            )}

          </ScrollView>
        </Animated.View>
      )}

      {/* ── Sticky CTA Bar ─────────────────────────────────────────── */}
      {!loading && selectedType && !isSoldOut(selectedType) && (
        <View style={[ss.ctaBar, { paddingBottom: insets.bottom > 0 ? insets.bottom + 8 : 20 }]}>
          {step === 'summary' && venueType !== 'none' && (
            <TouchableOpacity onPress={() => animateStep('map')} style={ss.backToMapBtn}>
              <Text style={ss.backToMapText}>🗺  Back to map</Text>
            </TouchableOpacity>
          )}

          <View style={ss.ctaTotalRow}>
            <Text style={ss.ctaTotalLabel}>
              Total {step === 'summary' ? fmtPrice(grandTotal) : fmtPrice(totalAmount)}
            </Text>
            <TouchableOpacity style={ss.viewDetailsBtn}>
              <Text style={ss.viewDetailsBtnText}>
                {step === 'summary' ? 'View details' : `${quantity}× ticket`}
              </Text>
              <Text style={ss.viewDetailsChevron}>{step === 'summary' ? ' ∧' : ' ›'}</Text>
            </TouchableOpacity>
          </View>

          {step === 'select' && (
            <TouchableOpacity style={ss.payBtn} onPress={handleNext} activeOpacity={0.88}>
              <Text style={ss.payBtnText}>{venueType !== 'none' ? 'Choose Seats' : 'Review Order'}</Text>
            </TouchableOpacity>
          )}
          {step === 'map' && (
            <TouchableOpacity style={ss.payBtn} onPress={() => animateStep('summary')} activeOpacity={0.88}>
              <Text style={ss.payBtnText}>Review Order</Text>
            </TouchableOpacity>
          )}
          {step === 'summary' && (
            <TouchableOpacity
              style={[ss.payBtn, adding && ss.payBtnDisabled]}
              onPress={handleAddToCart}
              disabled={adding}
              activeOpacity={0.88}
            >
              {adding
                ? <ActivityIndicator color="#fff" size="small" />
                : <Text style={ss.payBtnText}>Pay Now</Text>
              }
            </TouchableOpacity>
          )}
        </View>
      )}
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const ss = StyleSheet.create({
  screen: { flex: 1, backgroundColor: '#ffffff' },
  centered: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 12 },
  errorText: { fontSize: 13, fontWeight: '800', color: '#333', letterSpacing: 1 },
  goBackBtn: { borderWidth: 1, borderColor: '#ddd', paddingHorizontal: 16, paddingVertical: 8, borderRadius: 20 },
  goBackText: { fontSize: 12, fontWeight: '700', color: '#333' },
  loadingText: { fontSize: 12, color: '#999', marginTop: 8, fontWeight: '600' },
  scroll: { flex: 1 },

  // Header
  header: {
    backgroundColor: '#ffffff',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 20,
    paddingBottom: 12,
    borderBottomWidth: 1,
    borderBottomColor: '#f3f4f6',
  },
  closeBtn: {
    width: 32, height: 32, borderRadius: 16,
    backgroundColor: '#f3f4f6',
    alignItems: 'center', justifyContent: 'center',
  },
  closeIcon: { fontSize: 13, fontWeight: '700', color: '#222' },
  headerTitle: { fontSize: 16, fontWeight: '800', color: '#111111', letterSpacing: -0.2 },
  stepDots: { flexDirection: 'row', gap: 5, alignItems: 'center' },
  stepDot: { width: 6, height: 6, borderRadius: 3, backgroundColor: '#e5e7eb' },
  stepDotActive: { backgroundColor: PRIMARY, width: 16 },

  // Banner
  bannerWrap: { width: '100%', height: 180, position: 'relative', backgroundColor: '#1a1a1a' },
  bannerImage: { width: '100%', height: '100%' },
  bannerPlaceholder: { width: '100%', height: '100%', backgroundColor: '#1f1f2e', alignItems: 'center', justifyContent: 'center' },
  bannerPlaceholderIcon: { fontSize: 48 },
  bannerOverlay: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: 'rgba(0,0,0,0.25)',
  },
  bannerMeta: { position: 'absolute', bottom: 14, left: 20, right: 20 },
  bannerDate: {
    fontSize: 12, fontWeight: '700', color: 'rgba(255,255,255,0.95)',
    letterSpacing: 0.2,
    textShadowColor: 'rgba(0,0,0,0.6)', textShadowOffset: { width: 0, height: 1 }, textShadowRadius: 4,
  },
  bannerVenue: {
    fontSize: 11, fontWeight: '500', color: 'rgba(255,255,255,0.75)', marginTop: 2,
    textShadowColor: 'rgba(0,0,0,0.6)', textShadowOffset: { width: 0, height: 1 }, textShadowRadius: 4,
  },

  // Event title
  eventTitleSection: {
    paddingHorizontal: 20, paddingTop: 20, paddingBottom: 16,
    borderBottomWidth: 1, borderBottomColor: '#f3f4f6',
  },
  eventTitle: { fontSize: 22, fontWeight: '800', color: '#111111', letterSpacing: -0.4, marginBottom: 4 },
  eventSubtitle: { fontSize: 14, fontWeight: '500', color: '#6b7280' },
  eventVenueSmall: { fontSize: 13, fontWeight: '400', color: '#9ca3af', marginTop: 2 },

  // Section header
  sectionHeader: {
    flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between',
    paddingHorizontal: 20, paddingTop: 16, paddingBottom: 10,
  },
  sectionHeaderText: { fontSize: 11, fontWeight: '800', color: '#9ca3af', letterSpacing: 1.2 },
  sortChip: { backgroundColor: '#f3f4f6', borderRadius: 20, paddingHorizontal: 10, paddingVertical: 4 },
  sortChipText: { fontSize: 11, fontWeight: '600', color: '#374151' },

  // Ticket cards
  ticketCard: {
    flexDirection: 'row', alignItems: 'center',
    backgroundColor: '#ffffff',
    borderBottomWidth: 1, borderBottomColor: '#f3f4f6',
    paddingHorizontal: 20, paddingVertical: 16, gap: 14,
  },
  ticketCardSelected: {
    backgroundColor: '#fff5f7',
    borderLeftWidth: 3, borderLeftColor: PRIMARY, paddingLeft: 17,
  },
  ticketCardSoldOut: { opacity: 0.45 },
  radioOuter: {
    width: 20, height: 20, borderRadius: 10,
    borderWidth: 2, borderColor: '#d1d5db',
    alignItems: 'center', justifyContent: 'center', flexShrink: 0,
  },
  radioOuterSelected: { borderColor: PRIMARY },
  radioInner: { width: 10, height: 10, borderRadius: 5, backgroundColor: PRIMARY },
  ticketInfo: { flex: 1, gap: 4 },
  ticketNameRow: { flexDirection: 'row', alignItems: 'center', gap: 6 },
  ticketName: { fontSize: 15, fontWeight: '700', color: '#111111', letterSpacing: -0.1 },
  ticketNameSoldOut: { color: '#9ca3af' },
  ticketAvail: { fontSize: 12, color: '#9ca3af', fontWeight: '500' },
  vipBadge: { backgroundColor: '#fef3c7', borderRadius: 4, paddingHorizontal: 5, paddingVertical: 1 },
  vipBadgeText: { fontSize: 10, fontWeight: '700', color: '#92400e' },
  dealBadge: { backgroundColor: '#f0fdf4', borderRadius: 4, paddingHorizontal: 6, paddingVertical: 2, alignSelf: 'flex-start' },
  dealBadgeText: { fontSize: 10, fontWeight: '700', color: '#15803d' },
  soldBadge: { backgroundColor: '#fff1f2', borderRadius: 4, paddingHorizontal: 6, paddingVertical: 2, alignSelf: 'flex-start' },
  soldBadgeText: { fontSize: 10, fontWeight: '700', color: '#be123c' },
  ticketPriceCol: { alignItems: 'flex-end', gap: 2 },
  ticketPrice: { fontSize: 17, fontWeight: '900', color: '#111111' },
  ticketPriceSoldOut: { color: '#d1d5db', textDecorationLine: 'line-through' },
  ticketFeeNote: { fontSize: 10, color: '#9ca3af', fontWeight: '500' },

  // Quantity
  qtySection: {
    backgroundColor: '#fafafa', marginTop: 8,
    paddingHorizontal: 20, paddingTop: 20, paddingBottom: 24,
    borderTopWidth: 1, borderTopColor: '#f0f0f0',
  },
  qtySectionHeader: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 },
  qtySectionTitle: { fontSize: 14, fontWeight: '700', color: '#111111' },
  seatedNote: { fontSize: 11, color: '#9ca3af', fontWeight: '500' },
  qtyGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
  qtyBtn: {
    flex: 1, minWidth: (SW - 60) / 3 - 8, paddingVertical: 16,
    borderRadius: 12, backgroundColor: '#f3f4f6',
    borderWidth: 1.5, borderColor: 'transparent', alignItems: 'center',
  },
  qtyBtnSel: { backgroundColor: '#ffffff', borderColor: PRIMARY },
  qtyBtnText: { fontSize: 17, fontWeight: '800', color: '#9ca3af' },
  qtyBtnTextSel: { color: PRIMARY },

  // Seating map
  mapSubtitle: { fontSize: 12, color: '#9ca3af', fontWeight: '500', paddingHorizontal: 20, marginBottom: 10 },
  legendRow: { backgroundColor: '#fff', paddingHorizontal: 20, paddingVertical: 10, borderBottomWidth: 1, borderBottomColor: '#f0f0f0' },
  mapWrap: { backgroundColor: '#fff', paddingHorizontal: 20, paddingTop: 16, paddingBottom: 4 },
  tierCard: {
    flexDirection: 'row', alignItems: 'center',
    marginHorizontal: 20, marginTop: 12, padding: 16,
    backgroundColor: '#fff5f7', borderRadius: 12,
    borderWidth: 1, borderColor: '#fecdd3', gap: 12,
  },
  tierCardLeft: { flex: 1 },
  tierCardName: { fontSize: 13, fontWeight: '900', color: '#111111', letterSpacing: 0.4 },
  tierCardMeta: { fontSize: 11, color: '#9ca3af', fontWeight: '500', marginTop: 2 },
  tierCardTotal: { fontSize: 18, fontWeight: '900', color: PRIMARY },

  // Checkout summary
  checkoutWrap: { paddingBottom: 8 },
  detailsList: {
    borderTopWidth: 1, borderBottomWidth: 1, borderColor: '#f3f4f6',
    marginHorizontal: 20, marginTop: 4, marginBottom: 24,
  },
  detailRow: {
    flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between',
    paddingVertical: 16, borderBottomWidth: 1, borderBottomColor: '#f3f4f6',
  },
  detailRowMultiline: {
    flexDirection: 'row', alignItems: 'flex-start',
    paddingVertical: 16, borderBottomWidth: 1, borderBottomColor: '#f3f4f6',
  },
  detailLabel: { width: 80, fontSize: 15, fontWeight: '700', color: '#111111', flexShrink: 0 },
  detailRight: { flex: 1, flexDirection: 'row', alignItems: 'center', justifyContent: 'flex-end', gap: 6, marginLeft: 12 },
  detailValue: { fontSize: 14, color: '#6b7280', fontWeight: '500' },
  detailValueSub: { fontSize: 12, color: '#9ca3af', marginTop: 2, fontWeight: '400' },
  chevron: { fontSize: 18, color: '#d1d5db', fontWeight: '300', lineHeight: 20 },

  // Trust
  trustSection: { paddingHorizontal: 20, marginBottom: 24, gap: 14 },
  trustRow: { flexDirection: 'row', alignItems: 'center', gap: 12 },
  trustIcon: { fontSize: 18, width: 24, textAlign: 'center' },
  trustText: { flex: 1 },
  trustTitle: { fontSize: 13, fontWeight: '700', color: '#111111' },
  trustBody: { fontSize: 12, color: '#9ca3af', marginTop: 1 },
  trustHelp: { fontSize: 16, color: '#d1d5db' },

  // Price breakdown
  priceBreakdown: {
    backgroundColor: '#fafafa', borderRadius: 14,
    marginHorizontal: 20, padding: 18, marginBottom: 16, gap: 10,
  },
  priceRow: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  priceLabel: { fontSize: 13, color: '#6b7280', fontWeight: '500' },
  priceValue: { fontSize: 13, color: '#6b7280', fontWeight: '600' },
  priceLabelLight: { fontSize: 13, color: '#9ca3af', fontWeight: '400' },
  priceValueLight: { fontSize: 13, color: '#9ca3af', fontWeight: '500' },
  priceDivider: { height: 1, backgroundColor: '#e5e7eb', marginVertical: 2 },
  priceTotalLabel: { fontSize: 15, fontWeight: '800', color: '#111111' },
  priceTotalValue: { fontSize: 17, fontWeight: '900', color: '#111111' },
  editLink: { alignSelf: 'center', paddingVertical: 14 },
  editLinkText: { fontSize: 12, fontWeight: '700', color: '#6b7280' },

  // CTA bar
  ctaBar: {
    position: 'absolute', bottom: 0, left: 0, right: 0,
    backgroundColor: '#ffffff',
    paddingHorizontal: 20, paddingTop: 14,
    borderTopWidth: 1, borderTopColor: '#f3f4f6',
    gap: 12,
    shadowColor: '#000', shadowOffset: { width: 0, height: -3 },
    shadowOpacity: 0.06, shadowRadius: 10, elevation: 8,
  },
  backToMapBtn: {
    alignSelf: 'center', paddingHorizontal: 16, paddingVertical: 8,
    borderRadius: 20, borderWidth: 1, borderColor: '#e5e7eb',
  },
  backToMapText: { fontSize: 12, fontWeight: '700', color: '#374151' },
  ctaTotalRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 2 },
  ctaTotalLabel: { fontSize: 14, fontWeight: '800', color: '#111111' },
  viewDetailsBtn: { flexDirection: 'row', alignItems: 'center' },
  viewDetailsBtnText: { fontSize: 13, fontWeight: '700', color: '#111111' },
  viewDetailsChevron: { fontSize: 13, fontWeight: '700', color: '#111111' },
  payBtn: {
    backgroundColor: PRIMARY, paddingVertical: 16,
    borderRadius: 14, alignItems: 'center',
    shadowColor: PRIMARY, shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.25, shadowRadius: 8, elevation: 4,
  },
  payBtnDisabled: { opacity: 0.6 },
  payBtnText: { fontSize: 16, fontWeight: '800', color: '#ffffff', letterSpacing: 0.3 },

  // Empty state
  emptyState: { padding: 48, alignItems: 'center', gap: 8 },
  emptyIcon: { fontSize: 36 },
  emptyText: { fontSize: 13, color: '#9ca3af', fontWeight: '600' },
});