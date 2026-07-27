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
  Linking,
  KeyboardAvoidingView,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { useEventStore } from '../../store/eventStore';
import { useAuthStore } from '../../store/authStore';
import {
  api,
  getTicketTypes,
  createBooking,
  type TicketType,
} from '../../lib/api';
import { startBookingPayment } from '../../lib/payments';
import SeatingMap, { detectVenueType, MapLegend } from '../../components/seatingmap';
import { makePurchaseSheet, purchaseTokens, PRIMARY } from '../../styles/purchase';
import { useAppTheme } from '../../theme/colors';
import { isPaymentMethodEnabled } from '../../lib/paymentMethods';
import { openLegal } from '../../lib/legalLinks';
import {
  CaretLeft, CaretDown, ArrowsDownUp, Star, CrownSimple, Check,
  CreditCard, AppleLogo, PaypalLogo, Bank,
  SealCheck, ShieldCheck,
  Ticket, X,
} from 'phosphor-react-native';

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
  const { isDark } = useAppTheme();
  const ss = React.useMemo(() => makePurchaseSheet(isDark), [isDark]);
  const T  = purchaseTokens(isDark);
  const router   = useRouter();
  const insets   = useSafeAreaInsets();
  const event    = useEventStore((s) => s.selectedEvent) as any;
  const { user } = useAuthStore();

  const [ticketTypes, setTicketTypes] = useState<TicketType[]>([]);
  const [loading, setLoading]         = useState(true);
  const [selectedId, setSelectedId]   = useState<string | null>(null);
  const [quantity, setQuantity]       = useState(1);
  const [step, setStep]               = useState<'select' | 'map' | 'summary'>('select');
  const [showBreakdown, setShowBreakdown] = useState(false);
  const [adding, setAdding]           = useState(false);

  // CPA s49 acknowledgement. Not persisted: each purchase needs its own
  // affirmative act, so a buyer who ticked this last week has not thereby
  // consented this week. See the note in styles/purchase.ts (riskBox).
  const [riskAccepted, setRiskAccepted] = useState(false);
  const [riskNudge, setRiskNudge] = useState(false);
  const [guestName, setGuestName]         = useState('');
  const [guestSurname, setGuestSurname]   = useState('');
  const [guestEmail, setGuestEmail]       = useState('');
  const [contactModalOpen, setContactModalOpen] = useState(false);
  const [paymentSheetOpen, setPaymentSheetOpen] = useState(false);
  const [paymentMethod, setPaymentMethod] = useState<'card' | 'paypal' | 'apple_pay' | 'instant_eft'>('card');
  const modalSlide = useRef(new Animated.Value(300)).current;
  const surnameRef = useRef<TextInput>(null);
  const emailRef   = useRef<TextInput>(null);
  const paymentSlide = useRef(new Animated.Value(400)).current;
  const fadeAnim = useRef(new Animated.Value(1)).current;

  function openContactModal() {
    setContactModalOpen(true);
    Animated.spring(modalSlide, { toValue: 0, useNativeDriver: true, bounciness: 4 }).start();
  }

  function handleContactDone() {
    const emailOk = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(guestEmail.trim());
    if (!guestName.trim() || !guestSurname.trim()) {
      Alert.alert('Name required', 'Please enter your first name and surname.');
      return;
    }
    if (!emailOk) {
      Alert.alert('Valid email required', 'Please enter a valid email address.');
      return;
    }
    closeContactModal();
  }

  function closeContactModal() {
    Animated.timing(modalSlide, { toValue: 300, duration: 220, useNativeDriver: true }).start(() => {
      setContactModalOpen(false);
    });
  }

  function openPaymentSheet() {
    setPaymentSheetOpen(true);
    Animated.spring(paymentSlide, { toValue: 0, useNativeDriver: true, bounciness: 4 }).start();
  }

  function closePaymentSheet() {
    Animated.timing(paymentSlide, { toValue: 400, duration: 220, useNativeDriver: true }).start(() => {
      setPaymentSheetOpen(false);
    });
  }

  function selectPaymentMethod(method: 'card' | 'paypal' | 'apple_pay' | 'instant_eft') {
    // Belt and braces: the row is hidden when a method is disabled, but a
    // stale selection must never reach the gateway as an inactive channel.
    if (method === 'instant_eft' && !isPaymentMethodEnabled('instant_eft')) {
      method = 'card';
    }
    setPaymentMethod(method);
    closePaymentSheet();
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
        if (types.length === 0 && event?.price != null) {
          // Flat-price event with no ticket types — show a GA fallback
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
        } else {
          setTicketTypes(types);
          const first = types.find((t) => !isSoldOut(t));
          if (first) setSelectedId(first.id);
        }
      })
      .catch(() => {
        // Network / HTTP error — still try the flat-price fallback
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
  // Mirror the backend fee model (CalculateFees in events-db): a R10 service fee
  // per ticket, capped at 20% of the ticket's face value. This must match the
  // server, because the real charge is the booking's total_amount from the API.
  const SERVICE_FEE_PER_TICKET = 10;
  const MAX_SERVICE_FEE_RATE   = 0.20;
  const unitPrice    = selectedType?.price ?? 0;
  const perTicketFee = unitPrice > 0 ? Math.min(SERVICE_FEE_PER_TICKET, MAX_SERVICE_FEE_RATE * unitPrice) : 0;
  const platformFee  = Math.round(perTicketFee * quantity * 100) / 100;
  const grandTotal   = totalAmount + platformFee;
  // Seating maps are disabled for launch (like the merch tab). All events go
  // straight from ticket selection to the order summary. Flip this to true to
  // restore the Choose Seats step -- all its code below is intact.
  const SEATING_ENABLED = false;

  const rawVenueType = (() => {
    if (!event) return 'none' as const;
    const override = event.venue_type as string | undefined;
    if (override && override !== 'auto') return override as 'stadium' | 'auditorium' | 'none';
    return detectVenueType(event.category ?? '', event.venue_name);
  })();
  const venueType = SEATING_ENABLED ? rawVenueType : 'none';
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

    // Guest validation — must have filled in contact details
    if (!user) {
      if (!guestName.trim() || !guestSurname.trim()) {
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
      // Guests provide first name and surname as separate fields — never
      // derive the surname by splitting or duplicating the first name.
      const firstName = user ? String((user as any).first_name ?? '') : guestName.trim();
      const lastName  = user ? String((user as any).last_name ?? '')  : guestSurname.trim();
      const email     = user ? String((user as any).email ?? '') : guestEmail.trim();

      const endpoint  = user ? '/v1/bookings' : '/v1/guest/bookings';
      const payload   = user
        ? {
            event_id:            event.id,
            ticket_type_id:      selectedId !== 'ga-fallback' ? (selectedId ?? undefined) : undefined,
            ticket_quantity:     quantity,
            attendee_first_name: firstName,
            attendee_last_name:  lastName,
            attendee_email:      email,
          }
        : {
            event_id:            event.id,
            // Same as the authenticated path: tell the server WHICH tier was
            // chosen — the server resolves the price from this ID. 'ga-fallback'
            // is a client-only placeholder for flat-priced events, so omit it.
            ticket_type_id:      selectedId !== 'ga-fallback' ? (selectedId ?? undefined) : undefined,
            ticket_quantity:     quantity,
            attendee_first_name: firstName,
            attendee_last_name:  lastName,
            attendee_email:      email,
          };

      console.log('[PAY] endpoint:', endpoint, 'payload:', JSON.stringify(payload));
      const { data: booking } = await api.post(endpoint, payload);

      // For guest checkouts, surface the booking number + email so you can look
      // the ticket up in the Tickets tab's guest lookup while testing.
      if (!user) {
        console.log(
          '[PAY] guest booking created →',
          'bookingNumber:', (booking as any).booking_number,
          '| email:', email,
        );
      }

      // Free ticket — booking is already confirmed server-side, no payment step needed.
      if ((booking as any).total_amount === 0) {
        try { if (router.canDismiss()) router.dismissAll(); } catch {}
        router.navigate('/(tabs)/tickets');
        Alert.alert("You're in!", 'Your free ticket has been confirmed. Check the Tickets tab.');
        return;
      }

      const bookingId = (booking as any).booking_id;

      // Resolve the gateway checkout URL.
      //  • Authenticated users: start a payment with the selected method so the
      //    backend returns a method-specific hosted-checkout URL (card / PayPal /
      //    Apple Pay).
      //  • Guests: there is no method-aware start route, so fall back to the
      //    card authorization_url returned at booking creation.
      let authorizationUrl: string;
      let usedMethod: 'card' | 'paypal' | 'apple_pay' | 'instant_eft' = 'card';
      if (user && paymentMethod === 'card' && (booking as any).client_token) {
        // PERF: POST /v1/bookings already initialised a Paystack card checkout
        // and returned its URL. For card (the default), reuse it instead of
        // initialising the gateway a second time — saves a full round-trip and
        // gets the user to the payment sheet noticeably faster.
        authorizationUrl = (booking as any).client_token;
        usedMethod = 'card';
      } else if (user) {
        // Non-card method (PayPal / Apple Pay) needs a fresh, method-specific
        // gateway init via the method-aware start route.
        const started = await startBookingPayment(bookingId, paymentMethod);
        authorizationUrl = started.authorization_url;
        usedMethod = paymentMethod;
      } else {
        // Guests: no method-aware start route — use the card authorization_url
        // returned at booking creation.
        authorizationUrl = (booking as any).client_token;
      }

      if (!authorizationUrl) {
        Alert.alert('Payment unavailable', 'We couldn’t start the payment. Please try again.');
        return;
      }

      // Navigate to the payment WebView screen
      router.push({
        pathname: '/payment/[id]',
        params: {
          id:       bookingId,
          url:      encodeURIComponent(authorizationUrl),
          amount:   String((booking as any).total_amount),
          currency: (booking as any).currency,
          method:   usedMethod,
          // Guests have no auth token — pass booking number + email so the
          // payment screen can poll status and retry via the /v1/guest/* routes.
          ...(user ? {} : {
            bookingNumber: String((booking as any).booking_number ?? ''),
            email,
          }),
        },
      });
    } catch (err: any) {
      const msg = err?.response?.data?.error ?? 'Failed to create booking. Please try again.';
      Alert.alert('Error', msg);
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
      <StatusBar barStyle={isDark ? 'light-content' : 'dark-content'} />

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
          <CaretLeft size={20} color={T.text} weight="bold" />
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
            contentContainerStyle={{ paddingBottom: 130 }}
          >
            {/* ── Event Banner — checkout step only, scrolls with content ── */}
            {step === 'summary' && (
              <View style={ss.bannerWrap}>
                {bannerImage ? (
                  <Image source={{ uri: bannerImage }} style={ss.bannerImage} resizeMode="cover" />
                ) : (
                  <View style={ss.bannerPlaceholder}>
                    <Ticket size={36} color={T.border3} weight="duotone" />
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
                  <View style={[ss.sortChip, { flexDirection: 'row', alignItems: 'center', gap: 5 }]}>
                    <ArrowsDownUp size={13} color={T.subStrong} weight="bold" />
                    <Text style={ss.sortChipText}>Best deal</Text>
                  </View>
                </View>

                {ticketTypes.length === 0 ? (
                  <View style={ss.emptyState}>
                    <Ticket size={34} color={T.border3} weight="duotone" />
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
                              <View style={[ss.vipBadge, { flexDirection: 'row', alignItems: 'center', gap: 3 }]}>
                                <Star size={11} color={T.vipText} weight="fill" />
                                <Text style={ss.vipBadgeText}>VIP</Text>
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
                            <View style={[ss.dealBadge, { flexDirection: 'row', alignItems: 'center', gap: 3 }]}>
                              {isPremium
                                ? <CrownSimple size={11} color={T.okText} weight="fill" />
                                : <Check size={11} color={T.okText} weight="bold" />}
                              <Text style={ss.dealBadgeText}>{isPremium ? 'Premium' : 'Available'}</Text>
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

                {selectedType && !isSoldOut(selectedType) && (() => {
                  const maxPer = typeof event?.max_tickets_per_user === 'number'
                    ? event.max_tickets_per_user
                    : Infinity;

                  // Admin locked to exactly 1 ticket — skip the picker entirely,
                  // quantity stays at its initial value of 1.
                  if (maxPer === 1) return null;

                  const maxVisible = Math.min(maxPer, 6);
                  const opts = Array.from({ length: maxVisible }, (_, i) => i + 1);

                  return (
                    <View style={ss.qtySection}>
                      <View style={ss.qtySectionHeader}>
                        <Text style={ss.qtySectionTitle}>How many tickets?</Text>
                      </View>
                      {isFinite(maxPer) && (
                        <Text style={{ fontSize: 12, color: T.faint, marginBottom: 8, marginLeft: 2 }}>
                          Max {maxPer} ticket{maxPer !== 1 ? 's' : ''} per person
                        </Text>
                      )}
                      <View style={ss.qtyGrid}>
                        {opts.map((n) => (
                          <TouchableOpacity
                            key={n}
                            onPress={() => setQuantity(n)}
                            style={[ss.qtyBtn, quantity === n && ss.qtyBtnSel]}
                            activeOpacity={0.7}
                          >
                            <Text style={[ss.qtyBtnText, quantity === n && ss.qtyBtnTextSel]}>
                              {String(n)}
                            </Text>
                          </TouchableOpacity>
                        ))}
                      </View>
                    </View>
                  );
                })()}
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
                            : (`${guestName} ${guestSurname}`.trim() || guestEmail)}
                        </Text>
                      )}
                      {!user && <Text style={ss.chevron}>›</Text>}
                    </View>
                  </TouchableOpacity>

                  <TouchableOpacity style={ss.detailRow} onPress={openPaymentSheet} activeOpacity={0.7}>
                    <Text style={ss.detailLabel}>Payment</Text>
                    <View style={ss.detailRight}>
                      {paymentMethod === 'apple_pay' ? <AppleLogo size={16} color={T.text} weight="fill" />
                        : paymentMethod === 'paypal' ? <PaypalLogo size={16} color="#003087" weight="fill" />
                        : paymentMethod === 'instant_eft' ? <Bank size={16} color="#1a7f5a" weight="fill" />
                        : <CreditCard size={16} color={T.subStrong} weight="regular" />}
                      <Text style={ss.detailValue}>
                        {paymentMethod === 'apple_pay' ? 'Apple Pay' : paymentMethod === 'paypal' ? 'PayPal' : paymentMethod === 'instant_eft' ? 'Instant EFT' : 'Credit / Debit Card'}
                      </Text>
                      <Text style={ss.chevron}>›</Text>
                    </View>
                  </TouchableOpacity>

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
                    </View>
                  </View>
                </View>

                {/* Trust badges — restored.
                    These two lines exist on the web checkout and were the one
                    piece of reassurance the app checkout was missing: the buyer
                    is one tap from a card form, and nothing on screen said the
                    tickets are legitimate or the payment is protected. Copy is
                    kept verbatim from web-mobile's purchase page so support
                    isn't answering questions about two different promises. */}
                <View style={ss.trustCard}>
                  <View style={ss.trustRow}>
                    <SealCheck size={18} color={T.trustIcon} weight="fill" />
                    <View style={ss.trustText}>
                      <Text style={ss.trustTitle}>Official Ticket Marketplace</Text>
                      <Text style={ss.trustBody}>Tickets are reviewed and verified by the venue.</Text>
                    </View>
                  </View>
                  <View style={ss.trustRow}>
                    <ShieldCheck size={18} color={T.trustIcon} weight="fill" />
                    <View style={ss.trustText}>
                      <Text style={ss.trustTitle}>Every ticket protected</Text>
                      <Text style={ss.trustBody}>Secure payment processing.</Text>
                    </View>
                  </View>
                </View>

                {/* Price breakdown — collapsed by default; "View details"
                    expands the per-line items (SeatGeek-style). */}
                <View style={ss.priceBreakdown}>
                  <TouchableOpacity
                    style={ss.priceRow}
                    onPress={() => setShowBreakdown(v => !v)}
                    activeOpacity={0.7}
                  >
                    <Text style={ss.priceTotalLabel}>Total Amount</Text>
                    <View style={{ flexDirection: 'row', alignItems: 'center', gap: 10 }}>
                      <Text style={ss.priceTotalValue}>{fmtPrice(grandTotal)}</Text>
                      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 2 }}>
                        <Text style={{ fontSize: 12, fontWeight: '600', color: T.sub }}>
                          {showBreakdown ? 'Hide details' : 'View details'}
                        </Text>
                        <CaretDown size={14} color={T.sub} weight="bold" style={{ transform: [{ rotate: showBreakdown ? '180deg' : '0deg' }] }} />
                      </View>
                    </View>
                  </TouchableOpacity>
                  {showBreakdown && (
                    <>
                      <View style={ss.priceDivider} />
                      <View style={ss.priceRow}>
                        <Text style={ss.priceLabel}>{selectedType?.name ?? 'Ticket'} × {quantity}</Text>
                        <Text style={ss.priceValue}>{fmtPrice(totalAmount)}</Text>
                      </View>
                      <View style={ss.priceRow}>
                        <Text style={ss.priceLabelLight}>Service fee</Text>
                        <Text style={ss.priceValueLight}>{fmtPrice(platformFee)}</Text>
                      </View>
                    </>
                  )}
                </View>

                {/* ── CPA s49 acknowledgement ─────────────────────────────
                    Section 49 governs HOW agreement is obtained, not what the
                    terms say. For terms that limit our liability or ask the
                    buyer to assume risk, their attention must be drawn to the
                    term conspicuously, and s49(5) requires assent by an act
                    "consistent with acknowledgement of the notice" where the
                    activity could cause serious injury or death.

                    A Pay button alone cannot be that act: it means "I want this
                    ticket" and "I accept the refund policy" at the same time, so
                    it is not referable to the risk notice specifically. One
                    checkbox, one clause, risks named. */}
                <TouchableOpacity
                  style={[
                    ss.riskBox,
                    riskAccepted && ss.riskBoxChecked,
                    riskNudge && !riskAccepted && ss.riskBoxError,
                  ]}
                  onPress={() => { setRiskAccepted(v => !v); setRiskNudge(false); }}
                  activeOpacity={0.75}
                  accessibilityRole="checkbox"
                  accessibilityState={{ checked: riskAccepted }}
                  accessibilityLabel="Accept the risks of attending a live event"
                >
                  <View style={[ss.riskCheck, riskAccepted && ss.riskCheckOn]}>
                    {riskAccepted && <Check size={13} color="#fff" weight="bold" />}
                  </View>
                  <Text style={ss.riskLabel}>
                    I accept that attending a live event carries risks including personal
                    injury, that the organiser and venue reserve the right of admission,
                    and that age restrictions may apply — as set out in{' '}
                    <Text
                      style={ss.termsLink}
                      onPress={() => openLegal('terms')}
                      suppressHighlighting
                    >
                      clause 7 of the Terms of Use
                    </Text>
                    .
                  </Text>
                </TouchableOpacity>
                {riskNudge && !riskAccepted && (
                  <Text style={ss.riskError}>Please confirm this before continuing.</Text>
                )}
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
              style={[ss.payBtn, (adding || !riskAccepted) && ss.payBtnDisabled]}
              onPress={() => {
                // Gate: no payment without the s49 acknowledgement.
                if (!riskAccepted) { setRiskNudge(true); return; }
                handleAddToCart();
              }}
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
                <X size={16} color={T.text} weight="bold" />
              </TouchableOpacity>
              <Text style={ss.modalTitle}>Contact</Text>
              <TouchableOpacity onPress={handleContactDone} style={ss.modalDoneBtn}>
                <Text style={ss.modalDoneText}>Done</Text>
              </TouchableOpacity>
            </View>

            {/* Name fields — email intentionally last. No autoFocus: focusing
                a field in the same frame the Modal mounts makes iOS's
                KeyboardAvoidingView miss the keyboard event, leaving the
                sheet stuck behind the keyboard. */}
            <View style={ss.modalFieldWrap}>
              <TextInput
                style={ss.modalInput}
                placeholder="First name"
                placeholderTextColor={T.faint}
                value={guestName}
                onChangeText={setGuestName}
                autoCapitalize="words"
                autoCorrect={false}
                returnKeyType="next"
                onSubmitEditing={() => surnameRef.current?.focus()}
              />
            </View>
            <View style={[ss.modalFieldWrap, { marginTop: 10 }]}>
              <TextInput
                ref={surnameRef}
                style={ss.modalInput}
                placeholder="Surname"
                placeholderTextColor={T.faint}
                value={guestSurname}
                onChangeText={setGuestSurname}
                autoCapitalize="words"
                autoCorrect={false}
                returnKeyType="next"
                onSubmitEditing={() => emailRef.current?.focus()}
              />
            </View>

            {/* Email field */}
            <View style={[ss.modalFieldWrap, { marginTop: 10 }]}>
              <TextInput
                ref={emailRef}
                style={ss.modalInput}
                placeholder="Email address"
                placeholderTextColor={T.faint}
                value={guestEmail}
                onChangeText={setGuestEmail}
                keyboardType="email-address"
                autoCapitalize="none"
                autoCorrect={false}
                returnKeyType="done"
                onSubmitEditing={handleContactDone}
              />
            </View>

            {/* Already have an account */}
            <TouchableOpacity
              style={ss.modalAccountRow}
              onPress={() => {
                closeContactModal();
                router.push('/auth/login');
              }}
            >
              <Text style={ss.modalAccountText}>Already have an account?</Text>
              <Text style={ss.modalAccountChevron}>›</Text>
            </TouchableOpacity>

            <Text style={ss.modalFootnote}>
              The email address and name you provide will be used for purchase confirmation and delivering e-tickets.
            </Text>
          </Animated.View>
        </KeyboardAvoidingView>
      </Modal>

    {/* ── Payment Method Sheet ─────────────────────────────────
        Fully themed. The sheet used to be a hardcoded '#fff' panel with
        '#f0f0f0' dividers while its labels and check marks used T.text — so in
        dark mode "Add credit card" was white-on-white and the only payment
        method available looked like an empty sheet. Styles now come from
        styles/purchase.ts like the rest of the screen, which also means adding a
        method later can't reintroduce the same bug. */}
    <Modal visible={paymentSheetOpen} transparent animationType="none" onRequestClose={closePaymentSheet}>
      <TouchableOpacity style={ss.paySheetBackdrop} activeOpacity={1} onPress={closePaymentSheet} />
      <Animated.View style={[ss.paySheet, { transform: [{ translateY: paymentSlide }] }]}>
        <View style={ss.paySheetHandleWrap}>
          <View style={ss.paySheetHandle} />
        </View>
        <Text style={ss.paySheetTitle}>Payment method</Text>

        {/* Instant EFT (Paystack/Ozow) — signed-in users only for now;
            guest checkout is card-only until a method-aware guest route exists.

            Also gated on isPaymentMethodEnabled: Paystack activates EFT
            per-account and separately for test and live, so a channel that
            works in test can still fail in production until the extra KYC
            review clears. Offering it while inactive fails the charge AND
            holds the buyer's seats until the booking expires, so an
            unavailable method must not be shown at all. */}
        {user && isPaymentMethodEnabled('instant_eft') && (
          <TouchableOpacity onPress={() => selectPaymentMethod('instant_eft')} activeOpacity={0.7} style={ss.payRow}>
            <View style={[ss.payRowIconFilled, { backgroundColor: '#1a7f5a' }]}>
              <Bank size={19} color="#ffffff" weight="fill" />
            </View>
            <View style={{ flex: 1 }}>
              <Text style={ss.payRowLabelTop}>Instant EFT</Text>
              <Text style={ss.payRowSub}>Pay from your bank · instant confirmation</Text>
            </View>
            {paymentMethod === 'instant_eft' && <Check size={18} color={PRIMARY} weight="bold" />}
          </TouchableOpacity>
        )}

        {/* Credit / debit card */}
        <TouchableOpacity onPress={() => selectPaymentMethod('card')} activeOpacity={0.7} style={ss.payRow}>
          <View style={ss.payRowIcon}>
            <CreditCard size={20} color={T.subStrong} weight="regular" />
          </View>
          <View style={{ flex: 1 }}>
            <Text style={ss.payRowLabelTop}>Credit / Debit Card</Text>
            <Text style={ss.payRowSub}>Visa, Mastercard · 3D Secure</Text>
          </View>
          {paymentMethod === 'card' && <Check size={18} color={PRIMARY} weight="bold" />}
        </TouchableOpacity>
      </Animated.View>
    </Modal>

    </View>
    </KeyboardAvoidingView>
  );
}