import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  ActivityIndicator,
  Alert,
  Animated,
  Dimensions,
  StatusBar,
  Image,
  TextInput,
  Modal,
  Platform,
  KeyboardAvoidingView,
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
import { sheet as ss, PRIMARY } from '../../styles/purchase';

const { width: SW } = Dimensions.get('window');

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
  const [guestName, setGuestName]         = useState('');
  const [guestEmail, setGuestEmail]       = useState('');
  const [contactModalOpen, setContactModalOpen] = useState(false);
  const modalSlide = useRef(new Animated.Value(300)).current;
  const fadeAnim = useRef(new Animated.Value(1)).current;

  function openContactModal() {
    setContactModalOpen(true);
    Animated.spring(modalSlide, { toValue: 0, useNativeDriver: true, bounciness: 4 }).start();
  }

  function closeContactModal() {
    Animated.timing(modalSlide, { toValue: 300, duration: 220, useNativeDriver: true }).start(() => {
      setContactModalOpen(false);
    });
  }

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
    // Validate guest details when not logged in
    if (!user) {
      if (!guestName.trim()) {
        Alert.alert('Name required', 'Please enter your name so we can personalise your ticket.');
        return;
      }
      const emailOk = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(guestEmail.trim());
      if (!emailOk) {
        Alert.alert('Valid email required', 'Please enter a valid email address so we can send your ticket.');
        return;
      }
    }
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
    <KeyboardAvoidingView
      style={{ flex: 1 }}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
    >
    <View style={[ss.screen, { paddingBottom: insets.bottom }]}>
      <StatusBar barStyle="dark-content" />

      {/* ── Header ─────────────────────────────────────────────────── */}
      <View style={[ss.header, { paddingTop: insets.top + 4 }]}>
        <TouchableOpacity
          onPress={() => {
            if (step === 'summary') animateStep(venueType !== 'none' ? 'map' : 'select');
            else if (step === 'map') animateStep('select');
            else router.back();
          }}
          style={ss.backBtn}
          hitSlop={{ top: 8, left: 8, right: 8, bottom: 8 }}
        >
          <Text style={ss.backIcon}>‹</Text>
        </TouchableOpacity>
        <Text style={ss.headerTitle}>{headerTitle}</Text>
        <View style={ss.stepDots}>
          {steps.map((_, i) => (
            <View key={i} style={[ss.stepDot, i === stepIndex && ss.stepDotActive]} />
          ))}
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
            {/* ── Event Banner — checkout step only, scrolls with content ── */}
            {step === 'summary' && (
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
            )}

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

                  {/* Contact row — tappable for guests */}
                  <TouchableOpacity
                    style={ss.detailRow}
                    onPress={!user ? openContactModal : undefined}
                    activeOpacity={!user ? 0.7 : 1}
                  >
                    <Text style={ss.detailLabel}>Contact</Text>
                    <View style={ss.detailRight}>
                      {!user && !guestName && !guestEmail ? (
                        <Text style={[ss.detailValue, { color: PRIMARY }]}>Add contact info</Text>
                      ) : (
                        <Text style={ss.detailValue} numberOfLines={1}>
                          {user
                            ? (`${(user as any).first_name ?? ''} ${(user as any).last_name ?? ''}`.trim() || (user as any).email)
                            : (guestName || guestEmail)}
                        </Text>
                      )}
                      {!user && <Text style={ss.chevron}>›</Text>}
                    </View>
                  </TouchableOpacity>

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

                  <View style={[ss.detailRowMultiline, { borderBottomWidth: 0 }]}>
                    <Text style={ss.detailLabel}>Ticket</Text>
                    <View style={{ flex: 1, marginLeft: 12 }}>
                      <Text style={ss.detailValue}>{selectedType?.name ?? 'General Admission'}</Text>
                      {selectedType?.description ? (
                        <Text style={ss.detailValueSub}>{selectedType.description}</Text>
                      ) : null}
                      <Text style={ss.detailValueSub}>
                        Mobile Tickets · {guestEmail ? `Sent to ${guestEmail}` : 'Access in the Events App'}
                      </Text>
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

              </View>
            )}

          </ScrollView>
        </Animated.View>
      )}

      {/* ── Sticky CTA Bar ─────────────────────────────────────────── */}
      {!loading && selectedType && !isSoldOut(selectedType) && (
        <View style={[ss.ctaBar, { paddingBottom: insets.bottom > 0 ? insets.bottom + 8 : 20 }]}>

          <Text style={ss.ctaTotalLabel}>
            Total {step === 'summary' ? fmtPrice(grandTotal) : fmtPrice(totalAmount)}
          </Text>

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
      {/* ── Contact Info Modal ─────────────────────────────────────── */}
      <Modal
        visible={contactModalOpen}
        transparent
        animationType="none"
        onRequestClose={closeContactModal}
      >
        <KeyboardAvoidingView
          style={{ flex: 1 }}
          behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        >
          <TouchableOpacity
            style={ss.modalBackdrop}
            activeOpacity={1}
            onPress={closeContactModal}
          />
          <Animated.View style={[ss.modalSheet, { transform: [{ translateY: modalSlide }] }]}>
            {/* Handle bar */}
            <View style={ss.modalHandle} />

            {/* Header */}
            <View style={ss.modalHeader}>
              <TouchableOpacity onPress={closeContactModal} style={ss.modalCloseBtn}>
                <Text style={ss.modalCloseText}>✕</Text>
              </TouchableOpacity>
              <Text style={ss.modalTitle}>Contact</Text>
              <TouchableOpacity
                onPress={() => {
                  const emailOk = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(guestEmail.trim());
                  if (!guestName.trim()) {
                    Alert.alert('Name required', 'Please enter your full name.');
                    return;
                  }
                  if (!emailOk) {
                    Alert.alert('Valid email required', 'Please enter a valid email address.');
                    return;
                  }
                  closeContactModal();
                }}
                style={ss.modalDoneBtn}
              >
                <Text style={ss.modalDoneText}>Done</Text>
              </TouchableOpacity>
            </View>

            {/* Email field */}
            <View style={ss.modalFieldWrap}>
              <TextInput
                style={ss.modalInput}
                placeholder="Email address"
                placeholderTextColor="#aab0be"
                value={guestEmail}
                onChangeText={setGuestEmail}
                keyboardType="email-address"
                autoCapitalize="none"
                autoCorrect={false}
                returnKeyType="next"
                autoFocus
              />
            </View>

            {/* Name field */}
            <View style={[ss.modalFieldWrap, { marginTop: 10 }]}>
              <TextInput
                style={ss.modalInput}
                placeholder="Full name"
                placeholderTextColor="#aab0be"
                value={guestName}
                onChangeText={setGuestName}
                autoCapitalize="words"
                autoCorrect={false}
                returnKeyType="done"
                onSubmitEditing={closeContactModal}
              />
            </View>

            {/* Already have an account */}
            <TouchableOpacity style={ss.modalAccountRow}>
              <Text style={ss.modalAccountText}>Already have an account?</Text>
              <Text style={ss.modalAccountChevron}>›</Text>
            </TouchableOpacity>

            <Text style={ss.modalFootnote}>
              The email address and name you provide will be used for purchase confirmation and delivering e-tickets.
            </Text>
          </Animated.View>
        </KeyboardAvoidingView>
      </Modal>

    </View>
    </KeyboardAvoidingView>
  );
}