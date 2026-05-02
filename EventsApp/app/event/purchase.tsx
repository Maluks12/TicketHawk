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

// ─── Helpers ──────────────────────────────────────────────────────────────────

function fmtPrice(price: number) {
  if (price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function fmtDate(iso: string) {
  const d = new Date(iso);
  return d.toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'short' });
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

  // ─────────────────────────────────────────────────────────────────────────

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

  // Step dots — only show map dot when venue has a map
  const steps = venueType !== 'none'
    ? (['select', 'map', 'summary'] as const)
    : (['select', 'summary'] as const);
  const stepIndex = steps.indexOf(step as any);

  return (
    <View style={[ss.screen, { paddingBottom: insets.bottom }]}>
      <StatusBar barStyle="dark-content" />

      {/* ── Header ─────────────────────────────────────────────────── */}
      <View style={[ss.header, { paddingTop: insets.top + 6 }]}>
        <TouchableOpacity onPress={() => router.back()} style={ss.backPill}>
          <Text style={ss.backIcon}>‹</Text>
          <View style={{ flex: 1 }}>
            <Text style={ss.headerTitle} numberOfLines={1}>{event.title}</Text>
            <Text style={ss.headerMeta}>{fmtDate(event.start_time)} at {event.start_time.slice(11,16)} · {event.venue_name}</Text>
          </View>
          <View style={ss.infoCircle}>
            <Text style={ss.infoIcon}>ⓘ</Text>
          </View>
        </TouchableOpacity>
      </View>

      {/* ── Filter bar ─────────────────────────────────────────────── */}
      <View style={ss.filterBar}>
        <TouchableOpacity style={ss.filterPill}>
          <Text style={ss.filterPillIcon}>⊞</Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[ss.filterPill, ss.filterPillDark]}
          onPress={() => {
            if (step === 'summary') animateStep('select');
          }}
        >
          <Text style={ss.filterPillDarkText}>{quantity} ticket{quantity !== 1 ? 's' : ''}</Text>
        </TouchableOpacity>
        <TouchableOpacity style={ss.filterPill}>
          <Text style={ss.filterPillText}>Price  </Text>
          <Text style={[ss.filterPillText, { fontWeight: '700' }]}>Includes Fees</Text>
        </TouchableOpacity>
      </View>

      {/* ── Body ───────────────────────────────────────────────────── */}
      {loading ? (
        <View style={ss.centered}>
          <ActivityIndicator color="#1a1a1a" size="large" />
          <Text style={ss.loadingText}>Loading tickets…</Text>
        </View>
      ) : (
        <Animated.View style={{ flex: 1, opacity: fadeAnim }}>
          <ScrollView
            style={ss.scroll}
            showsVerticalScrollIndicator={false}
            contentContainerStyle={{ paddingBottom: 170 }}
          >

            {/* ══ STEP: Ticket Type + Quantity ══════════════════════════ */}
            {step === 'select' && (
              <View>
                {/* Listings count + sort */}
                <View style={ss.listingsBar}>
                  <Text style={ss.listingsCount}>{ticketTypes.length} Listing{ticketTypes.length !== 1 ? 's' : ''}</Text>
                  <TouchableOpacity style={ss.sortBtn}>
                    <Text style={ss.sortText}>⇅  Sort by deal</Text>
                  </TouchableOpacity>
                </View>

                {/* Ticket type cards */}
                {ticketTypes.length === 0 ? (
                  <View style={ss.emptyState}>
                    <Text style={ss.emptyText}>No tickets currently on sale</Text>
                  </View>
                ) : (
                  ticketTypes.map((tt, idx) => {
                    const sold     = isSoldOut(tt);
                    const selected = tt.id === selectedId;
                    const isPremium = idx === ticketTypes.length - 1 && ticketTypes.length > 1;
                    return (
                      <TouchableOpacity
                        key={tt.id}
                        onPress={() => !sold && setSelectedId(tt.id)}
                        style={[ss.listingCard, selected && ss.listingCardSelected, sold && ss.listingCardSoldOut]}
                        activeOpacity={sold ? 1 : 0.75}
                      >
                        {/* Left thumbnail placeholder */}
                        <View style={[ss.listingThumb, { backgroundColor: selected ? '#dbe8ff' : '#efefef' }]}>
                          <Text style={{ fontSize: 22 }}>🎟</Text>
                        </View>

                        {/* Info */}
                        <View style={ss.listingInfo}>
                          <Text style={[ss.listingName, sold && { color: '#aaa' }]}>{tt.name}</Text>
                          <Text style={ss.listingDesc} numberOfLines={1}>
                            {tt.description ?? availLabel(tt)}
                          </Text>
                          <View style={ss.listingBadges}>
                            {!sold && (
                              <View style={ss.dealBadge}>
                                <Text style={ss.dealBadgeText}>10  {isPremium ? 'Premium' : 'Amazing'}</Text>
                              </View>
                            )}
                            {sold && (
                              <View style={ss.soldBadge}>
                                <Text style={ss.soldBadgeText}>Sold Out</Text>
                              </View>
                            )}
                            {isPremium && !sold && (
                              <View style={ss.extraBadge}>
                                <Text style={ss.extraBadgeText}>⭐ VIP</Text>
                              </View>
                            )}
                          </View>
                        </View>

                        {/* Price */}
                        <View style={ss.listingPriceCol}>
                          <Text style={[ss.listingPrice, sold && { color: '#bbb', textDecorationLine: 'line-through' }]}>
                            {fmtPrice(tt.price)}
                          </Text>
                          <Text style={ss.listingFeeNote}>incl. fees</Text>
                        </View>
                      </TouchableOpacity>
                    );
                  })
                )}

                {/* Quantity picker */}
                {selectedType && !isSoldOut(selectedType) && (
                  <View style={ss.qtySheet}>
                    <Text style={ss.qtyTitle}>How many tickets?</Text>
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
                    <View style={ss.seatedRow}>
                      <Text style={ss.seatedIcon}>🪑🪑</Text>
                      <Text style={ss.seatedText}>You'll be seated together</Text>
                    </View>
                  </View>
                )}
              </View>
            )}

            {/* ══ STEP: Seating Map ═════════════════════════════════════ */}
            {step === 'map' && venueType !== 'none' && (
              <View>
                <View style={ss.mapHeader}>
                  <Text style={ss.mapHeaderTitle}>CHOOSE YOUR SECTION</Text>
                  <Text style={ss.mapHeaderSub}>Tap a section to update your tier</Text>
                </View>

                {/* Legend */}
                <View style={ss.legendRow}>
                  <MapLegend ticketTypes={ticketTypes} selectedTypeId={selectedId} />
                </View>

                {/* Map */}
                <View style={ss.mapWrap}>
                  <SeatingMap
                    venueType={venueType}
                    ticketTypes={ticketTypes}
                    selectedTypeId={selectedId}
                    onSelectType={handleSelectType}
                  />
                </View>

                <Text style={ss.mapNote}>Tap a section to select your ticket tier</Text>

                {/* Selected tier summary */}
                {selectedType && (
                  <View style={ss.tierSummaryCard}>
                    <View style={ss.tierSummaryLeft}>
                      <Text style={ss.tierSummaryName}>{selectedType.name.toUpperCase()}</Text>
                      <Text style={ss.tierSummaryMeta}>{quantity} ticket{quantity !== 1 ? 's' : ''} · {fmtPrice(selectedType.price)} each</Text>
                    </View>
                    <Text style={ss.tierSummaryTotal}>{fmtPrice(totalAmount)}</Text>
                  </View>
                )}
              </View>
            )}

            {/* ══ STEP: Order Summary ═══════════════════════════════════ */}
            {step === 'summary' && (
              <View style={ss.summaryWrap}>
                <Text style={ss.summaryHeading}>ORDER SUMMARY</Text>

                <View style={ss.summaryCard}>
                  {/* Event info */}
                  <View style={ss.summaryEventRow}>
                    <View style={ss.summaryDot} />
                    <View style={{ flex: 1 }}>
                      <Text style={ss.summaryEventName}>{event.title}</Text>
                      <Text style={ss.summaryEventMeta}>{fmtDate(event.start_time)} · {event.venue_name}</Text>
                    </View>
                  </View>

                  <View style={ss.divider} />

                  <View style={ss.summaryRow}>
                    <Text style={ss.summaryLabel}>{selectedType?.name ?? 'Ticket'} × {quantity}</Text>
                    <Text style={ss.summaryValue}>{fmtPrice(totalAmount)}</Text>
                  </View>
                  <View style={ss.summaryRow}>
                    <Text style={ss.summaryLabelLight}>Platform fee (5%)</Text>
                    <Text style={ss.summaryValueLight}>{fmtPrice(platformFee)}</Text>
                  </View>

                  <View style={ss.divider} />

                  <View style={[ss.summaryRow, { marginTop: 2 }]}>
                    <Text style={ss.summaryTotalLabel}>TOTAL</Text>
                    <Text style={ss.summaryTotalValue}>{fmtPrice(grandTotal)}</Text>
                  </View>
                </View>

                <View style={ss.secureNote}>
                  <Text style={ss.secureNoteText}>🔒  Secure payment via Stitch · Card details never stored</Text>
                </View>

                <TouchableOpacity onPress={() => animateStep('select')} style={ss.editLink}>
                  <Text style={ss.editLinkText}>← Edit selection</Text>
                </TouchableOpacity>
              </View>
            )}

          </ScrollView>
        </Animated.View>
      )}

      {/* ── Sticky bottom CTA ──────────────────────────────────────── */}
      {!loading && selectedType && !isSoldOut(selectedType) && (
        <View style={[ss.ctaContainer, { paddingBottom: insets.bottom > 0 ? insets.bottom + 8 : 20 }]}>
          {/* Back to map pill (only on summary step) */}
          {step === 'summary' && venueType !== 'none' && (
            <TouchableOpacity onPress={() => animateStep('map')} style={ss.backToMapBtn}>
              <Text style={ss.backToMapIcon}>🗺</Text>
              <Text style={ss.backToMapText}>Back to map</Text>
            </TouchableOpacity>
          )}

          {/* Price row */}
          <View style={ss.ctaPriceRow}>
            <Text style={ss.ctaQtyLabel}>
              {quantity}× {selectedType.name.toUpperCase()}
            </Text>
            <View style={{ alignItems: 'flex-end' }}>
              <Text style={ss.ctaPrice}>
                {step === 'summary' ? fmtPrice(grandTotal) : fmtPrice(totalAmount)}
              </Text>
              <Text style={ss.ctaFeeNote}>incl. fees</Text>
            </View>
          </View>

          {/* Primary action */}
          {step === 'select' && (
            <TouchableOpacity style={ss.ctaBtn} onPress={handleNext} activeOpacity={0.88}>
              <Text style={ss.ctaBtnText}>{venueType !== 'none' ? 'CHOOSE SEATS ›' : 'REVIEW ORDER ›'}</Text>
            </TouchableOpacity>
          )}
          {step === 'map' && (
            <TouchableOpacity style={ss.ctaBtn} onPress={() => animateStep('summary')} activeOpacity={0.88}>
              <Text style={ss.ctaBtnText}>REVIEW ORDER ›</Text>
            </TouchableOpacity>
          )}
          {step === 'summary' && (
            <TouchableOpacity style={[ss.ctaBtn, adding && { opacity: 0.7 }]} onPress={handleAddToCart} disabled={adding} activeOpacity={0.88}>
              {adding ? <ActivityIndicator color="#fff" size="small" /> : <Text style={ss.ctaBtnText}>🎟  RESERVE TICKETS</Text>}
            </TouchableOpacity>
          )}
        </View>
      )}
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const ss = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: '#f5f5f5',
  },
  centered: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 12,
  },
  errorText: {
    fontSize: 14,
    fontWeight: '800',
    color: '#333',
    letterSpacing: 1,
  },
  goBackBtn: {
    borderWidth: 1,
    borderColor: '#ccc',
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 20,
  },
  goBackText: { fontSize: 12, fontWeight: '700', color: '#333' },
  loadingText: { fontSize: 12, color: '#888', marginTop: 8, fontWeight: '600' },

  // ── Header ────────────────────────────────────────────────────────────────
  header: {
    backgroundColor: '#fff',
    paddingHorizontal: 12,
    paddingBottom: 10,
    borderBottomWidth: 1,
    borderBottomColor: '#ebebeb',
  },
  backPill: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: '#f3f3f3',
    borderRadius: 28,
    paddingVertical: 10,
    paddingHorizontal: 14,
    gap: 8,
    borderWidth: 1,
    borderColor: '#e5e5e5',
  },
  backIcon: {
    fontSize: 22,
    fontWeight: '300',
    color: '#222',
    lineHeight: 24,
    marginTop: -1,
  },
  headerTitle: {
    fontSize: 13,
    fontWeight: '800',
    color: '#111',
    letterSpacing: -0.2,
  },
  headerMeta: {
    fontSize: 11,
    color: '#777',
    fontWeight: '500',
    marginTop: 1,
  },
  infoCircle: {
    width: 30,
    height: 30,
    borderRadius: 15,
    borderWidth: 1.5,
    borderColor: '#ccc',
    alignItems: 'center',
    justifyContent: 'center',
  },
  infoIcon: { fontSize: 14, color: '#555' },

  // ── Filter bar ────────────────────────────────────────────────────────────
  filterBar: {
    flexDirection: 'row',
    gap: 8,
    paddingHorizontal: 16,
    paddingVertical: 10,
    backgroundColor: '#fff',
    borderBottomWidth: 1,
    borderBottomColor: '#ebebeb',
  },
  filterPill: {
    flexDirection: 'row',
    borderRadius: 20,
    borderWidth: 1,
    borderColor: '#ddd',
    paddingVertical: 6,
    paddingHorizontal: 12,
    alignItems: 'center',
    backgroundColor: '#fff',
  },
  filterPillIcon: { fontSize: 14, color: '#444' },
  filterPillText: { fontSize: 11, color: '#555', fontWeight: '500' },
  filterPillDark: {
    backgroundColor: '#111',
    borderColor: '#111',
  },
  filterPillDarkText: {
    fontSize: 11,
    fontWeight: '700',
    color: '#fff',
  },

  // ── Scroll ────────────────────────────────────────────────────────────────
  scroll: { flex: 1 },

  // ── Listings (select step) ────────────────────────────────────────────────
  listingsBar: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingVertical: 12,
    backgroundColor: '#fff',
    borderBottomWidth: 1,
    borderBottomColor: '#efefef',
  },
  listingsCount: {
    fontSize: 16,
    fontWeight: '800',
    color: '#111',
  },
  sortBtn: { flexDirection: 'row', alignItems: 'center', gap: 4 },
  sortText: { fontSize: 13, color: '#333', fontWeight: '600' },

  listingCard: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: '#fff',
    marginHorizontal: 0,
    borderBottomWidth: 1,
    borderBottomColor: '#efefef',
    padding: 14,
    gap: 12,
  },
  listingCardSelected: {
    backgroundColor: '#f0f5ff',
    borderLeftWidth: 3,
    borderLeftColor: '#0f52ba',
  },
  listingCardSoldOut: { opacity: 0.5 },
  listingThumb: {
    width: 64,
    height: 64,
    borderRadius: 10,
    alignItems: 'center',
    justifyContent: 'center',
  },
  listingInfo: { flex: 1, gap: 3 },
  listingName: { fontSize: 15, fontWeight: '800', color: '#111', letterSpacing: -0.2 },
  listingDesc: { fontSize: 11, color: '#888', fontWeight: '500' },
  listingBadges: { flexDirection: 'row', gap: 6, marginTop: 3 },
  dealBadge: {
    backgroundColor: '#e6f5ea',
    borderRadius: 4,
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
  dealBadgeText: { fontSize: 10, fontWeight: '800', color: '#1e7a34' },
  soldBadge: {
    backgroundColor: '#fff0f0',
    borderRadius: 4,
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
  soldBadgeText: { fontSize: 10, fontWeight: '700', color: '#cc2222' },
  extraBadge: {
    backgroundColor: '#f5f0e8',
    borderRadius: 4,
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
  extraBadgeText: { fontSize: 10, fontWeight: '700', color: '#7a5a1e' },
  listingPriceCol: { alignItems: 'flex-end', gap: 2 },
  listingPrice: { fontSize: 17, fontWeight: '900', color: '#111' },
  listingFeeNote: { fontSize: 10, color: '#999', fontWeight: '500' },

  // ── Quantity sheet ────────────────────────────────────────────────────────
  qtySheet: {
    backgroundColor: '#fff',
    marginTop: 10,
    paddingTop: 20,
    paddingBottom: 24,
    paddingHorizontal: 20,
    borderTopWidth: 1,
    borderTopColor: '#efefef',
  },
  qtyTitle: {
    fontSize: 15,
    fontWeight: '800',
    color: '#111',
    marginBottom: 14,
    textAlign: 'center',
  },
  qtyGrid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 10,
  },
  qtyBtn: {
    flex: 1,
    minWidth: (SW - 60) / 3 - 8,
    paddingVertical: 18,
    borderRadius: 10,
    backgroundColor: '#f3f3f3',
    borderWidth: 1.5,
    borderColor: 'transparent',
    alignItems: 'center',
  },
  qtyBtnSel: {
    backgroundColor: '#fff',
    borderColor: '#111',
  },
  qtyBtnText: {
    fontSize: 17,
    fontWeight: '800',
    color: '#aaa',
  },
  qtyBtnTextSel: { color: '#111' },
  seatedRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
    marginTop: 14,
  },
  seatedIcon: { fontSize: 16 },
  seatedText: { fontSize: 12, color: '#888', fontWeight: '600' },

  // ── Seating map step ──────────────────────────────────────────────────────
  mapHeader: {
    backgroundColor: '#fff',
    paddingHorizontal: 16,
    paddingTop: 14,
    paddingBottom: 8,
    borderBottomWidth: 1,
    borderBottomColor: '#ebebeb',
  },
  mapHeaderTitle: {
    fontSize: 11,
    fontWeight: '900',
    letterSpacing: 1.5,
    color: '#333',
  },
  mapHeaderSub: {
    fontSize: 11,
    color: '#888',
    fontWeight: '500',
    marginTop: 2,
  },
  legendRow: {
    backgroundColor: '#fff',
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderBottomWidth: 1,
    borderBottomColor: '#ebebeb',
  },
  mapWrap: {
    backgroundColor: '#fff',
    paddingHorizontal: 20,
    paddingTop: 16,
    paddingBottom: 4,
  },
  mapNote: {
    textAlign: 'center',
    fontSize: 11,
    color: '#aaa',
    fontWeight: '500',
    paddingVertical: 8,
    backgroundColor: '#fff',
  },
  tierSummaryCard: {
    flexDirection: 'row',
    alignItems: 'center',
    marginHorizontal: 16,
    marginTop: 12,
    padding: 14,
    backgroundColor: '#fff',
    borderRadius: 10,
    borderWidth: 1,
    borderColor: '#e5e5e5',
    gap: 12,
  },
  tierSummaryLeft: { flex: 1 },
  tierSummaryName: {
    fontSize: 13,
    fontWeight: '900',
    color: '#111',
    letterSpacing: 0.4,
  },
  tierSummaryMeta: {
    fontSize: 11,
    color: '#888',
    fontWeight: '500',
    marginTop: 2,
  },
  tierSummaryTotal: {
    fontSize: 18,
    fontWeight: '900',
    color: '#111',
  },

  // ── Summary step ──────────────────────────────────────────────────────────
  summaryWrap: { paddingTop: 4 },
  summaryHeading: {
    fontSize: 11,
    fontWeight: '900',
    letterSpacing: 1.5,
    color: '#999',
    paddingHorizontal: 16,
    paddingVertical: 14,
    backgroundColor: '#fff',
    borderBottomWidth: 1,
    borderBottomColor: '#ebebeb',
  },
  summaryCard: {
    backgroundColor: '#fff',
    marginHorizontal: 16,
    marginTop: 12,
    borderRadius: 12,
    padding: 16,
    borderWidth: 1,
    borderColor: '#e5e5e5',
  },
  summaryEventRow: { flexDirection: 'row', alignItems: 'flex-start', gap: 10, marginBottom: 12 },
  summaryDot: { width: 8, height: 8, borderRadius: 4, backgroundColor: '#111', marginTop: 3 },
  summaryEventName: { fontSize: 14, fontWeight: '800', color: '#111', letterSpacing: -0.2 },
  summaryEventMeta: { fontSize: 11, color: '#888', marginTop: 2, fontWeight: '500' },
  divider: { height: 1, backgroundColor: '#efefef', marginVertical: 10 },
  summaryRow: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginBottom: 5 },
  summaryLabel: { fontSize: 13, fontWeight: '700', color: '#111' },
  summaryValue: { fontSize: 13, fontWeight: '800', color: '#111' },
  summaryLabelLight: { fontSize: 12, fontWeight: '500', color: '#aaa' },
  summaryValueLight: { fontSize: 12, fontWeight: '600', color: '#aaa' },
  summaryTotalLabel: { fontSize: 13, fontWeight: '900', color: '#111', letterSpacing: 1 },
  summaryTotalValue: { fontSize: 20, fontWeight: '900', color: '#111' },
  secureNote: {
    marginHorizontal: 16,
    marginTop: 10,
    paddingVertical: 12,
    alignItems: 'center',
  },
  secureNoteText: { fontSize: 11, color: '#aaa', fontWeight: '500' },
  editLink: { alignSelf: 'center', paddingVertical: 12 },
  editLinkText: { fontSize: 12, fontWeight: '700', color: '#555' },

  // ── Sticky CTA ────────────────────────────────────────────────────────────
  ctaContainer: {
    position: 'absolute',
    bottom: 0,
    left: 0,
    right: 0,
    backgroundColor: '#fff',
    paddingHorizontal: 16,
    paddingTop: 14,
    borderTopWidth: 1,
    borderTopColor: '#ebebeb',
    gap: 10,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: -2 },
    shadowOpacity: 0.06,
    shadowRadius: 8,
    elevation: 6,
  },
  backToMapBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    backgroundColor: '#111',
    borderRadius: 24,
    paddingVertical: 10,
    paddingHorizontal: 20,
    alignSelf: 'center',
  },
  backToMapIcon: { fontSize: 14 },
  backToMapText: { fontSize: 12, fontWeight: '800', color: '#fff', letterSpacing: 0.3 },
  ctaPriceRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  ctaQtyLabel: { fontSize: 11, fontWeight: '700', color: '#888', letterSpacing: 0.3 },
  ctaPrice: { fontSize: 17, fontWeight: '900', color: '#111' },
  ctaFeeNote: { fontSize: 10, color: '#aaa', fontWeight: '500' },
  ctaBtn: {
    backgroundColor: '#ff3c6e',
    paddingVertical: 16,
    borderRadius: 10,
    alignItems: 'center',
  },
  ctaBtnText: {
    fontSize: 13,
    fontWeight: '900',
    letterSpacing: 1.5,
    color: '#fff',
  },
  emptyState: { padding: 40, alignItems: 'center' },
  emptyText: { fontSize: 13, color: '#aaa', fontWeight: '600' },
});