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
import { LinearGradient } from 'expo-linear-gradient';
import { useEventStore } from '../../store/eventStore';
import { useAuthStore } from '../../store/authStore';
import {
  getTicketTypes,
  addTicketToCart,
  checkoutBegin,
  type TicketType,
} from '../../lib/api';
import SeatingMap, { detectVenueType } from '../../components/seatingmap';

const { width: SW } = Dimensions.get('window');

// ─── Helpers ──────────────────────────────────────────────────────────────────

function fmtPrice(price: number, currency = 'ZAR') {
  if (price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function fmtDate(iso: string) {
  const d = new Date(iso);
  return d.toLocaleDateString('en-ZA', {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
  });
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
  const router    = useRouter();
  const insets    = useSafeAreaInsets();
  const event     = useEventStore((s) => s.selectedEvent) as any;
  const { user }  = useAuthStore();

  const [ticketTypes, setTicketTypes]   = useState<TicketType[]>([]);
  const [loading, setLoading]           = useState(true);
  const [selectedId, setSelectedId]     = useState<string | null>(null);
  const [quantity, setQuantity]         = useState(1);
  const [step, setStep]                 = useState<'select' | 'map' | 'summary'>('select');
  const [adding, setAdding]             = useState(false);

  // Animation for step transitions
  const fadeAnim = useRef(new Animated.Value(1)).current;

  function animateStep(next: 'select' | 'map' | 'summary') {
    Animated.sequence([
      Animated.timing(fadeAnim, { toValue: 0, duration: 120, useNativeDriver: true }),
      Animated.timing(fadeAnim, { toValue: 1, duration: 200, useNativeDriver: true }),
    ]).start();
    setStep(next);
  }

  // Load ticket types
  useEffect(() => {
    if (!event?.id) return;
    setLoading(true);
    getTicketTypes(event.id)
      .then((types) => {
        setTicketTypes(types);
        // Auto-select first available type
        const first = types.find((t) => !isSoldOut(t));
        if (first) setSelectedId(first.id);
      })
      .catch((err) => {
        console.warn('Failed to load ticket types', err);
        // Fall back to generic GA type derived from event price
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
  const platformFee  = Math.ceil(totalAmount * 0.05);   // 5% platform fee display
  const grandTotal   = totalAmount + platformFee;

  // Venue map type detection
  const venueType = event
    ? detectVenueType(event.category ?? '', event.venue_name)
    : 'none';

  // ── Handlers ─────────────────────────────────────────────────────────────

  const handleSelectType = useCallback((id: string) => {
    setSelectedId(id);
  }, []);

  const handleNext = () => {
    if (!selectedId) {
      Alert.alert('Select a ticket', 'Please choose a ticket type to continue.');
      return;
    }
    if (venueType !== 'none') {
      animateStep('map');
    } else {
      animateStep('summary');
    }
  };

  const handleAddToCart = async () => {
    if (!selectedType || !event) return;
    setAdding(true);

    try {
      // If it's the fallback (no real ticket types), skip cart API
      if (selectedId === 'ga-fallback') {
        Alert.alert(
          '🎟 Added!',
          `${quantity}× ${selectedType.name} for ${fmtPrice(grandTotal)} added to your cart.`,
          [
            { text: 'Continue Shopping', style: 'cancel' },
            { text: 'Checkout', onPress: () => router.push('/(tabs)/tickets') },
          ]
        );
        setAdding(false);
        return;
      }

      // Add to cart
      await addTicketToCart({
        item_type: 'ticket',
        event_id: event.id,
        ticket_type_id: selectedId ?? undefined,
        quantity,
      });

      // Begin checkout (creates reservations)
      const checkout = await checkoutBegin();

      Alert.alert(
        '🎟 Reserved!',
        `${quantity}× ${selectedType.name}. You have ${Math.floor(checkout.expires_in_seconds / 60)} minutes to complete payment.`,
        [
          { text: 'Continue Shopping', style: 'cancel' },
          { text: 'Checkout', onPress: () => router.push('/(tabs)/tickets') },
        ]
      );
    } catch (err: any) {
      const msg = err?.response?.data?.error ?? 'Failed to reserve tickets. Please try again.';
      Alert.alert('Error', msg);
    } finally {
      setAdding(false);
    }
  };

  // ─────────────────────────────────────────────────────────────────────────

  if (!event) {
    return (
      <View style={ss.centered}>
        <Text style={ss.errorText}>EVENT NOT FOUND</Text>
        <TouchableOpacity onPress={() => router.back()} style={ss.backBtnSmall}>
          <Text style={ss.backBtnSmallText}>← GO BACK</Text>
        </TouchableOpacity>
      </View>
    );
  }

  return (
    <View style={[ss.screen, { paddingBottom: insets.bottom }]}>
      <StatusBar barStyle="dark-content" />

      {/* ── Top bar ───────────────────────────────────────────────── */}
      <View style={[ss.topBar, { paddingTop: insets.top + 8 }]}>
        <TouchableOpacity onPress={() => router.back()} style={ss.backBtn}>
          <Text style={ss.backIcon}>‹</Text>
        </TouchableOpacity>

        <View style={ss.topTitleBlock}>
          <Text style={ss.topCategory} numberOfLines={1}>
            ✦ {event.category?.toUpperCase()}
          </Text>
          <Text style={ss.topTitle} numberOfLines={1}>{event.title.toUpperCase()}</Text>
          <Text style={ss.topMeta}>
            {fmtDate(event.start_time)} · {event.venue_name}
          </Text>
        </View>

        {/* Step indicator */}
        <View style={ss.stepIndicator}>
          {(['select', 'map', 'summary'] as const)
            .filter((s) => venueType !== 'none' || s !== 'map')
            .map((s, i) => (
              <View
                key={s}
                style={[
                  ss.stepDot,
                  step === s && ss.stepDotActive,
                  (step === 'summary' ||
                    (step === 'map' && s === 'select') ||
                    (step !== 'select' && s === 'select')) && ss.stepDotDone,
                ]}
              />
            ))}
        </View>
      </View>

      {/* ── Scrollable body ───────────────────────────────────────── */}
      {loading ? (
        <View style={ss.centered}>
          <ActivityIndicator color="#ff3c6e" size="large" />
          <Text style={ss.loadingText}>LOADING TICKETS</Text>
        </View>
      ) : (
        <Animated.View style={{ flex: 1, opacity: fadeAnim }}>
          <ScrollView
            style={ss.scroll}
            showsVerticalScrollIndicator={false}
            contentContainerStyle={{ paddingBottom: 160 }}
          >

            {/* ── STEP: Ticket Type Selection ──────────────────────── */}
            {step === 'select' && (
              <View>
                <View style={ss.sectionHeader}>
                  <Text style={ss.sectionTitle}>SELECT TICKET TYPE</Text>
                  <Text style={ss.sectionSub}>{ticketTypes.length} tier{ticketTypes.length !== 1 ? 's' : ''} available</Text>
                </View>

                {ticketTypes.length === 0 ? (
                  <View style={ss.emptyState}>
                    <Text style={ss.emptyText}>No tickets currently on sale</Text>
                  </View>
                ) : (
                  ticketTypes.map((tt, idx) => {
                    const sold = isSoldOut(tt);
                    const selected = tt.id === selectedId;
                    return (
                      <TouchableOpacity
                        key={tt.id}
                        onPress={() => !sold && setSelectedId(tt.id)}
                        style={[
                          ss.tierCard,
                          selected && ss.tierCardSelected,
                          sold && ss.tierCardSoldOut,
                        ]}
                        activeOpacity={sold ? 1 : 0.8}
                      >
                        {/* Selection indicator */}
                        <View style={[ss.tierRadio, selected && ss.tierRadioSelected]}>
                          {selected && <View style={ss.tierRadioInner} />}
                        </View>

                        <View style={ss.tierInfo}>
                          <View style={ss.tierNameRow}>
                            <Text style={[ss.tierName, selected && ss.tierNameSelected, sold && ss.tierNameSoldOut]}>
                              {tt.name.toUpperCase()}
                            </Text>
                            {idx === ticketTypes.length - 1 && ticketTypes.length > 1 && !sold && (
                              <View style={ss.premiumBadge}>
                                <Text style={ss.premiumBadgeText}>PREMIUM</Text>
                              </View>
                            )}
                            {sold && (
                              <View style={ss.soldOutBadge}>
                                <Text style={ss.soldOutBadgeText}>SOLD OUT</Text>
                              </View>
                            )}
                          </View>
                          {tt.description ? (
                            <Text style={ss.tierDesc} numberOfLines={2}>{tt.description}</Text>
                          ) : null}
                          <Text style={[ss.tierAvail, sold && { color: '#ff3c6e' }]}>
                            {availLabel(tt)}
                          </Text>
                        </View>

                        <Text style={[ss.tierPrice, selected && ss.tierPriceSelected, sold && ss.tierPriceSoldOut]}>
                          {fmtPrice(tt.price)}
                        </Text>
                      </TouchableOpacity>
                    );
                  })
                )}

                {/* Quantity */}
                {selectedType && !isSoldOut(selectedType) && (
                  <View style={ss.qtySection}>
                    <View style={ss.sectionHeader}>
                      <Text style={ss.sectionTitle}>HOW MANY TICKETS?</Text>
                      <Text style={ss.sectionSub}>You'll be seated together</Text>
                    </View>
                    <View style={ss.qtyGrid}>
                      {[1, 2, 3, 4, 5, 6].map((n) => (
                        <TouchableOpacity
                          key={n}
                          onPress={() => setQuantity(n)}
                          style={[ss.qtyBtn, quantity === n && ss.qtyBtnSelected]}
                          activeOpacity={0.75}
                        >
                          <Text style={[ss.qtyBtnText, quantity === n && ss.qtyBtnTextSelected]}>
                            {n === 6 ? '6+' : n}
                          </Text>
                        </TouchableOpacity>
                      ))}
                    </View>
                    <View style={ss.seatedRow}>
                      <Text style={ss.seatedIcon}>🪑</Text>
                      <Text style={ss.seatedText}>Seats are guaranteed together</Text>
                    </View>
                  </View>
                )}
              </View>
            )}

            {/* ── STEP: Seating Map ────────────────────────────────── */}
            {step === 'map' && venueType !== 'none' && (
              <View>
                <View style={ss.sectionHeader}>
                  <Text style={ss.sectionTitle}>CHOOSE YOUR SECTION</Text>
                  <Text style={ss.sectionSub}>Tap a section to update your tier</Text>
                </View>

                <SeatingMap
                  venueType={venueType}
                  ticketTypes={ticketTypes}
                  selectedTypeId={selectedId}
                  onSelectType={handleSelectType}
                />

                {/* Mini summary while on map step */}
                {selectedType && (
                  <View style={ss.miniSummary}>
                    <View style={ss.miniSummaryLeft}>
                      <Text style={ss.miniSummaryTier}>{selectedType.name.toUpperCase()}</Text>
                      <Text style={ss.miniSummaryMeta}>{quantity} ticket{quantity !== 1 ? 's' : ''} · {fmtPrice(selectedType.price)} each</Text>
                    </View>
                    <Text style={ss.miniSummaryTotal}>{fmtPrice(totalAmount)}</Text>
                  </View>
                )}
              </View>
            )}

            {/* ── STEP: Order Summary ──────────────────────────────── */}
            {step === 'summary' && (
              <View>
                <View style={ss.sectionHeader}>
                  <Text style={ss.sectionTitle}>ORDER SUMMARY</Text>
                </View>

                <View style={ss.summaryCard}>
                  {/* Event row */}
                  <View style={ss.summaryEventRow}>
                    <View style={ss.summaryEventDot} />
                    <View style={{ flex: 1 }}>
                      <Text style={ss.summaryEventTitle}>{event.title.toUpperCase()}</Text>
                      <Text style={ss.summaryEventMeta}>
                        {fmtDate(event.start_time)} · {event.venue_name}
                      </Text>
                    </View>
                  </View>

                  <View style={ss.summaryDivider} />

                  {/* Line items */}
                  <View style={ss.summaryLine}>
                    <Text style={ss.summaryLineLabel}>
                      {selectedType?.name ?? 'Ticket'} × {quantity}
                    </Text>
                    <Text style={ss.summaryLineValue}>{fmtPrice(totalAmount)}</Text>
                  </View>
                  <View style={ss.summaryLine}>
                    <Text style={ss.summaryLineLabelLight}>Platform fee</Text>
                    <Text style={ss.summaryLineValueLight}>{fmtPrice(platformFee)}</Text>
                  </View>

                  <View style={ss.summaryDivider} />

                  <View style={[ss.summaryLine, { marginTop: 4 }]}>
                    <Text style={ss.summaryTotal}>TOTAL</Text>
                    <Text style={ss.summaryTotalValue}>{fmtPrice(grandTotal)}</Text>
                  </View>
                </View>

                {/* Payment note */}
                <View style={ss.paymentNote}>
                  <Text style={ss.paymentNoteText}>
                    🔒  Secure payment via Stitch. Your card details are never stored.
                  </Text>
                </View>

                {/* Edit link */}
                <TouchableOpacity
                  onPress={() => animateStep('select')}
                  style={ss.editBtn}
                >
                  <Text style={ss.editBtnText}>← EDIT SELECTION</Text>
                </TouchableOpacity>
              </View>
            )}

          </ScrollView>
        </Animated.View>
      )}

      {/* ── Sticky CTA ────────────────────────────────────────────── */}
      {!loading && selectedType && !isSoldOut(selectedType) && (
        <View style={ss.ctaContainer}>
          <LinearGradient
            colors={['rgba(255,255,255,0)', 'rgba(255,255,255,0.98)', '#ffffff']}
            style={StyleSheet.absoluteFillObject}
            pointerEvents="none"
          />

          {/* Price preview */}
          <View style={ss.ctaPriceRow}>
            <Text style={ss.ctaPriceLabel}>
              {quantity}× {selectedType.name.toUpperCase()}
            </Text>
            <Text style={ss.ctaPrice}>
              {step === 'summary' ? fmtPrice(grandTotal) : fmtPrice(totalAmount)}
              {step === 'summary' && <Text style={ss.ctaPriceInclFees}>  incl. fees</Text>}
            </Text>
          </View>

          {step === 'select' && (
            <TouchableOpacity style={ss.ctaBtn} onPress={handleNext} activeOpacity={0.85}>
              <Text style={ss.ctaBtnText}>
                {venueType !== 'none' ? 'CHOOSE SEATS ›' : 'REVIEW ORDER ›'}
              </Text>
            </TouchableOpacity>
          )}

          {step === 'map' && (
            <TouchableOpacity style={ss.ctaBtn} onPress={() => animateStep('summary')} activeOpacity={0.85}>
              <Text style={ss.ctaBtnText}>REVIEW ORDER ›</Text>
            </TouchableOpacity>
          )}

          {step === 'summary' && (
            <TouchableOpacity
              style={[ss.ctaBtn, adding && ss.ctaBtnLoading]}
              onPress={handleAddToCart}
              activeOpacity={0.85}
              disabled={adding}
            >
              {adding ? (
                <ActivityIndicator color="#fff" size="small" />
              ) : (
                <Text style={ss.ctaBtnText}>🎟  RESERVE TICKETS</Text>
              )}
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
    backgroundColor: '#ffffff',
  },
  scroll: {
    flex: 1,
  },
  centered: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  loadingText: {
    marginTop: 12,
    fontSize: 10,
    fontWeight: '800',
    letterSpacing: 2,
    color: 'rgba(0,0,0,0.3)',
  },
  errorText: {
    fontSize: 11,
    fontWeight: '900',
    letterSpacing: 2,
    color: 'rgba(0,0,0,0.3)',
    marginBottom: 12,
  },

  // ── Top Bar ─────────────────────────────────────────────────────────────
  topBar: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    paddingHorizontal: 16,
    paddingBottom: 14,
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(0,0,0,0.07)',
    gap: 10,
  },
  backBtn: {
    width: 36,
    height: 36,
    borderRadius: 2,
    backgroundColor: '#f5f5f5',
    alignItems: 'center',
    justifyContent: 'center',
    marginTop: 2,
  },
  backIcon: {
    fontSize: 20,
    fontWeight: '900',
    color: '#000',
    lineHeight: 24,
  },
  topTitleBlock: {
    flex: 1,
  },
  topCategory: {
    fontSize: 9,
    fontWeight: '900',
    letterSpacing: 2,
    color: '#ff3c6e',
    marginBottom: 2,
  },
  topTitle: {
    fontSize: 15,
    fontWeight: '900',
    letterSpacing: 0.5,
    color: '#000',
    marginBottom: 2,
  },
  topMeta: {
    fontSize: 11,
    fontWeight: '500',
    color: 'rgba(0,0,0,0.45)',
  },
  stepIndicator: {
    flexDirection: 'row',
    gap: 5,
    alignItems: 'center',
    paddingTop: 4,
  },
  stepDot: {
    width: 7,
    height: 7,
    borderRadius: 4,
    backgroundColor: 'rgba(0,0,0,0.12)',
  },
  stepDotActive: {
    backgroundColor: '#ff3c6e',
    width: 18,
    borderRadius: 4,
  },
  stepDotDone: {
    backgroundColor: 'rgba(255,60,110,0.3)',
  },

  // ── Section Headers ──────────────────────────────────────────────────────
  sectionHeader: {
    flexDirection: 'row',
    alignItems: 'baseline',
    justifyContent: 'space-between',
    paddingHorizontal: 20,
    paddingTop: 20,
    paddingBottom: 12,
  },
  sectionTitle: {
    fontSize: 11,
    fontWeight: '900',
    letterSpacing: 2,
    color: '#000',
  },
  sectionSub: {
    fontSize: 10,
    fontWeight: '600',
    color: 'rgba(0,0,0,0.35)',
    letterSpacing: 0.5,
  },

  // ── Tier Cards ──────────────────────────────────────────────────────────
  tierCard: {
    flexDirection: 'row',
    alignItems: 'center',
    marginHorizontal: 20,
    marginBottom: 10,
    padding: 14,
    borderRadius: 10,
    backgroundColor: '#f7f7f7',
    borderWidth: 1.5,
    borderColor: 'transparent',
    gap: 12,
  },
  tierCardSelected: {
    borderColor: '#ff3c6e',
    backgroundColor: '#fff5f7',
  },
  tierCardSoldOut: {
    opacity: 0.5,
  },
  tierRadio: {
    width: 20,
    height: 20,
    borderRadius: 10,
    borderWidth: 2,
    borderColor: 'rgba(0,0,0,0.2)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  tierRadioSelected: {
    borderColor: '#ff3c6e',
  },
  tierRadioInner: {
    width: 10,
    height: 10,
    borderRadius: 5,
    backgroundColor: '#ff3c6e',
  },
  tierInfo: {
    flex: 1,
    gap: 2,
  },
  tierNameRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  tierName: {
    fontSize: 13,
    fontWeight: '900',
    letterSpacing: 0.5,
    color: '#000',
  },
  tierNameSelected: {
    color: '#ff3c6e',
  },
  tierNameSoldOut: {
    color: 'rgba(0,0,0,0.35)',
  },
  tierDesc: {
    fontSize: 11,
    color: 'rgba(0,0,0,0.5)',
    lineHeight: 16,
  },
  tierAvail: {
    fontSize: 10,
    fontWeight: '700',
    color: 'rgba(0,0,0,0.4)',
    letterSpacing: 0.3,
    marginTop: 2,
  },
  tierPrice: {
    fontSize: 15,
    fontWeight: '900',
    color: '#000',
    letterSpacing: 0.3,
  },
  tierPriceSelected: {
    color: '#ff3c6e',
  },
  tierPriceSoldOut: {
    color: 'rgba(0,0,0,0.3)',
    textDecorationLine: 'line-through',
  },
  premiumBadge: {
    backgroundColor: '#c8962a',
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 2,
  },
  premiumBadgeText: {
    fontSize: 8,
    fontWeight: '900',
    letterSpacing: 1,
    color: '#fff',
  },
  soldOutBadge: {
    backgroundColor: 'rgba(255,60,110,0.12)',
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 2,
  },
  soldOutBadgeText: {
    fontSize: 8,
    fontWeight: '900',
    letterSpacing: 1,
    color: '#ff3c6e',
  },

  // ── Quantity ─────────────────────────────────────────────────────────────
  qtySection: {
    marginTop: 8,
  },
  qtyGrid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 10,
    paddingHorizontal: 20,
  },
  qtyBtn: {
    flex: 1,
    minWidth: (SW - 40 - 50) / 3 - 6,
    paddingVertical: 16,
    borderRadius: 8,
    backgroundColor: '#f5f5f5',
    borderWidth: 1.5,
    borderColor: 'transparent',
    alignItems: 'center',
  },
  qtyBtnSelected: {
    borderColor: '#ff3c6e',
    backgroundColor: '#fff5f7',
  },
  qtyBtnText: {
    fontSize: 16,
    fontWeight: '900',
    color: 'rgba(0,0,0,0.55)',
  },
  qtyBtnTextSelected: {
    color: '#ff3c6e',
  },
  seatedRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
    paddingVertical: 14,
    marginHorizontal: 20,
    marginTop: 6,
    backgroundColor: '#f5f5f5',
    borderRadius: 8,
  },
  seatedIcon: {
    fontSize: 16,
  },
  seatedText: {
    fontSize: 11,
    fontWeight: '700',
    color: 'rgba(0,0,0,0.5)',
    letterSpacing: 0.3,
  },

  // ── Seating map mini summary ──────────────────────────────────────────────
  miniSummary: {
    flexDirection: 'row',
    alignItems: 'center',
    marginHorizontal: 20,
    marginTop: 12,
    padding: 14,
    backgroundColor: '#f7f7f7',
    borderRadius: 10,
    borderLeftWidth: 3,
    borderLeftColor: '#ff3c6e',
  },
  miniSummaryLeft: {
    flex: 1,
    gap: 2,
  },
  miniSummaryTier: {
    fontSize: 12,
    fontWeight: '900',
    letterSpacing: 0.5,
    color: '#ff3c6e',
  },
  miniSummaryMeta: {
    fontSize: 11,
    color: 'rgba(0,0,0,0.45)',
    fontWeight: '600',
  },
  miniSummaryTotal: {
    fontSize: 18,
    fontWeight: '900',
    color: '#000',
  },

  // ── Summary Card ─────────────────────────────────────────────────────────
  summaryCard: {
    marginHorizontal: 20,
    backgroundColor: '#f7f7f7',
    borderRadius: 12,
    padding: 16,
  },
  summaryEventRow: {
    flexDirection: 'row',
    gap: 10,
    alignItems: 'flex-start',
    marginBottom: 12,
  },
  summaryEventDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: '#ff3c6e',
    marginTop: 4,
  },
  summaryEventTitle: {
    fontSize: 13,
    fontWeight: '900',
    letterSpacing: 0.5,
    color: '#000',
    marginBottom: 2,
  },
  summaryEventMeta: {
    fontSize: 11,
    color: 'rgba(0,0,0,0.45)',
    fontWeight: '500',
  },
  summaryDivider: {
    height: 1,
    backgroundColor: 'rgba(0,0,0,0.08)',
    marginVertical: 10,
  },
  summaryLine: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 6,
  },
  summaryLineLabel: {
    fontSize: 12,
    fontWeight: '700',
    color: '#000',
  },
  summaryLineValue: {
    fontSize: 12,
    fontWeight: '800',
    color: '#000',
  },
  summaryLineLabelLight: {
    fontSize: 12,
    fontWeight: '500',
    color: 'rgba(0,0,0,0.45)',
  },
  summaryLineValueLight: {
    fontSize: 12,
    fontWeight: '600',
    color: 'rgba(0,0,0,0.45)',
  },
  summaryTotal: {
    fontSize: 13,
    fontWeight: '900',
    letterSpacing: 1.5,
    color: '#000',
  },
  summaryTotalValue: {
    fontSize: 20,
    fontWeight: '900',
    color: '#ff3c6e',
  },
  paymentNote: {
    marginHorizontal: 20,
    marginTop: 12,
    padding: 12,
    backgroundColor: 'rgba(0,0,0,0.03)',
    borderRadius: 8,
  },
  paymentNoteText: {
    fontSize: 11,
    color: 'rgba(0,0,0,0.45)',
    lineHeight: 16,
    textAlign: 'center',
    fontWeight: '500',
  },
  editBtn: {
    alignSelf: 'center',
    paddingVertical: 10,
    paddingHorizontal: 16,
    marginTop: 8,
  },
  editBtnText: {
    fontSize: 10,
    fontWeight: '800',
    letterSpacing: 1.5,
    color: 'rgba(0,0,0,0.35)',
  },

  // ── Sticky CTA ───────────────────────────────────────────────────────────
  ctaContainer: {
    position: 'absolute',
    bottom: 0,
    left: 0,
    right: 0,
    paddingHorizontal: 20,
    paddingBottom: Platform.OS === 'ios' ? 32 : 20,
    paddingTop: 40,
  },
  ctaPriceRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'baseline',
    marginBottom: 10,
  },
  ctaPriceLabel: {
    fontSize: 11,
    fontWeight: '800',
    letterSpacing: 0.5,
    color: 'rgba(0,0,0,0.5)',
  },
  ctaPrice: {
    fontSize: 17,
    fontWeight: '900',
    color: '#000',
  },
  ctaPriceInclFees: {
    fontSize: 11,
    fontWeight: '600',
    color: 'rgba(0,0,0,0.4)',
  },
  ctaBtn: {
    backgroundColor: '#ff3c6e',
    paddingVertical: 17,
    borderRadius: 2,
    alignItems: 'center',
  },
  ctaBtnLoading: {
    opacity: 0.7,
  },
  ctaBtnText: {
    fontSize: 12,
    fontWeight: '900',
    letterSpacing: 2,
    color: '#fff',
  },

  // ── Other ────────────────────────────────────────────────────────────────
  emptyState: {
    padding: 40,
    alignItems: 'center',
  },
  emptyText: {
    fontSize: 13,
    color: 'rgba(0,0,0,0.35)',
    fontWeight: '600',
  },
  backBtnSmall: {
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.15)',
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 2,
  },
  backBtnSmallText: {
    fontSize: 10,
    fontWeight: '800',
    letterSpacing: 1.5,
    color: '#000',
  },
});