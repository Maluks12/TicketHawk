import React, { useEffect, useState, useCallback, useRef } from 'react';
import {
  View, Text, ScrollView, TouchableOpacity, RefreshControl,
 ActivityIndicator, Image, TextInput, KeyboardAvoidingView, Platform, Animated,
  useWindowDimensions, InteractionManager,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter, useFocusEffect } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { StyleSheet } from 'react-native';
// expo-image rather than react-native's Image: the two keep separate caches, so an image already fetched elsewhere in the app was being re-downloaded here. expo-image also persists to disk across restarts.
import { Image as ExpoImage } from 'expo-image';
import { api } from '../../lib/api';
import { useAuthStore } from '../../store/authStore';
import { cacheBookingsList, getCachedBookingsList, syncManyOffline, syncBookingOffline, cacheBookingDetail, saveGuestTicket, getGuestTickets } from '../../lib/offline';
import { s, sheet, setTicketsTheme } from '../../styles/tickets';
import { useAppTheme } from '../../theme/colors'; 

const CARD_HEIGHT = 210;
// Vertical space between cards. This used to be a negative margin of
// -(CARD_HEIGHT - CARD_PEEK), which overlapped every card onto the one
// above so only a 72px sliver showed. It looked like a wallet, but it meant
// the only fully readable ticket was the last one — the event name, date and
// venue of everything above it were covered, and scrolling never revealed
// them because the overlap is fixed rather than scroll-driven.
//
// Now they simply stack vertically with a gap, matching the category screen.
const CARD_GAP = 14;

// ─── Types ────────────────────────────────────────────────────────────────────

interface BookingEvent {
  title: string;
  start_time: string;
  venue_name: string;
  city: string;
  cover_image_url?: string | null;
}

export interface Booking {
  id: string;
  booking_number: string;
  event_id: string;
  ticket_quantity: number;
  total_amount: number;
  currency: string;
  booking_status: 'pending' | 'confirmed' | 'cancelled' | 'refunded';
  payment_status: 'pending' | 'processing' | 'completed' | 'failed' | 'refunded';
  qr_code_url?: string | null;
  created_at: string;
  confirmed_at?: string | null;
  ticket_type_name?: string | null;
  event?: BookingEvent;
}

// ─── Palettes ─────────────────────────────────────────────────────────────────

const PALETTES = [
  { top: '#0d2137', bottom: '#0a3d2e', accent: '#00e5a0' },
  { top: '#1a0a2e', bottom: '#2d0a3d', accent: '#b57bee' },
  { top: '#1a1200', bottom: '#3d2600', accent: '#ffb830' },
  { top: '#001a2e', bottom: '#003d5c', accent: '#29c5f6' },
  { top: '#1a0008', bottom: '#3d0015', accent: '#ff4d6d' },
  { top: '#0a1a00', bottom: '#1a3d00', accent: '#8fff00' },
];

// ─── Helpers ──────────────────────────────────────────────────────────────────

const fmt = (iso: string) => new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
const fmtTime = (iso: string) => new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
const isUpcoming = (b: Booking) => !!b.event?.start_time && new Date(b.event.start_time) > new Date();

// A "real" ticket is one the buyer actually holds: booking confirmed/refunded,
// or payment completed/refunded. Pending and failed bookings are NOT shown.
const isRealTicket = (b: Booking) =>
  b.booking_status === 'confirmed' || b.booking_status === 'refunded' ||
  b.payment_status === 'completed' || b.payment_status === 'refunded';

// Upcoming = future-dated AND still active. Cancelled/refunded bookings drop to
// the Past tab even when the event date is still ahead.
const isUpcomingActive = (b: Booking) =>
  isUpcoming(b) && b.booking_status !== 'cancelled' && b.booking_status !== 'refunded';
const daysUntil  = (iso: string) => Math.ceil((new Date(iso).getTime() - Date.now()) / 86400000);
const dedup = (list: Booking[]) => { const seen = new Set<string>(); return list.filter(b => { if (seen.has(b.booking_number)) return false; seen.add(b.booking_number); return true; }); };

// ─── Ticket Card ──────────────────────────────────────────────────────────────

// React.memo so a parent re-render doesn't re-run every card's ~15 reads off
// the `s`/`sheet` Proxy objects in styles/tickets.ts. The comparator ignores
// `onPress`: TicketStack builds that closure inline on every render, so a
// default shallow compare would never match and the memo would do nothing.
const TicketCard = React.memo(({ booking, index, onPress }: { booking: Booking; index: number; onPress: () => void }) => {
  const pal  = PALETTES[index % PALETTES.length];
  const up   = isUpcoming(booking);
  const days = booking.event?.start_time ? daysUntil(booking.event.start_time) : null;

  const statusLabel =
    booking.booking_status === 'confirmed' && up
      ? days === 0 ? 'TODAY' : days === 1 ? 'TOMORROW' : `IN ${days} DAYS`
      : booking.booking_status === 'confirmed' ? 'ATTENDED'
      : booking.booking_status.toUpperCase();

  const statusColor =
    up && booking.booking_status === 'confirmed' ? pal.accent
    : booking.booking_status === 'cancelled' || booking.booking_status === 'refunded' ? '#ff4d4d'
    : '#888';

 return (
  <TouchableOpacity activeOpacity={0.93} onPress={onPress} style={sheet.cardTouchable}>
    <LinearGradient colors={[pal.top, pal.bottom]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={sheet.card}>
      {booking.event?.cover_image_url && (
        <ExpoImage
          source={{ uri: booking.event.cover_image_url }}
          style={StyleSheet.absoluteFill}
          contentFit="cover"
          blurRadius={3}
          cachePolicy="memory-disk"
          transition={150}
          recyclingKey={booking.id}
        />
      )}
      {booking.event?.cover_image_url && (
        <LinearGradient colors={[`${pal.top}d8`, `${pal.bottom}f0`]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={StyleSheet.absoluteFill} />
      )}

      {/* Top — location only */}
      {booking.event && (
        <View className={s.metaRow}>
          <Ionicons name="location-outline" size={10} color="rgba(255,255,255,0.70)" />
          <Text className={s.metaText} style={{ color: 'rgba(255,255,255,0.70)' }} numberOfLines={1}>
            {booking.event.venue_name}, {booking.event.city}
          </Text>
        </View>
      )}

      {/* Middle — vertically centered, left-aligned */}
      <View style={{ flex: 1, justifyContent: 'center' }}>
        <Text className={s.eventTitle} numberOfLines={3}>{booking.event?.title ?? 'Event'}</Text>
        {booking.event?.start_time && (
          <View className={s.metaRow} style={{ marginTop: 5 }}>
            <Ionicons name="calendar-outline" size={10} color="rgba(255,255,255,0.70)" />
            <Text className={s.metaText} style={{ color: 'rgba(255,255,255,0.70)' }}>
              {fmt(booking.event.start_time)} · {fmtTime(booking.event.start_time)}
            </Text>
          </View>
        )}
      </View>

    
      {/* Bottom — quantity left, status indicator right */}
      <View className={s.cardBottom}>
        <View>
          <Text className={s.bottomLabel}>{booking.ticket_type_name ?? 'GENERAL'}</Text>
          <Text className={s.bottomValue} style={{ color: pal.accent }}>
            {booking.ticket_quantity}x {booking.ticket_quantity === 1 ? 'ticket' : 'tickets'}
          </Text>
        </View>
        <View style={{ flex: 1 }} />
        <View className={s.statusPill} style={{ borderColor: `${statusColor}55`, backgroundColor: `${statusColor}22` }}>
          <View className={s.statusDot} style={{ backgroundColor: statusColor }} />
          <Text className={s.statusText} style={{ color: statusColor }}>{statusLabel}</Text>
        </View>
      </View>

    </LinearGradient>
  </TouchableOpacity>
);
}, (a, b) =>
  a.booking.id === b.booking.id &&
  a.booking.booking_status === b.booking.booking_status &&
  a.index === b.index);

// ─── Stack ────────────────────────────────────────────────────────────────────

// Memoised: TicketPane re-renders whenever the screen does, and without this
// the stack re-maps every booking into fresh elements each time. `bookings` is
// a stable reference from the useMemo upstream, so the compare holds.
const TicketStack = React.memo(({ bookings, offset = 0 }: { bookings: Booking[]; offset?: number }) => {
  const router = useRouter();
  return (
    <View>
      {bookings.map((b, i) => (
        <View key={b.id} style={i > 0 ? { marginTop: CARD_GAP } : undefined}>
          <TicketCard booking={b} index={offset + i} onPress={() => {
            // Guest bookings have no auth token — route with booking number +
            // email so the detail screen fetches via the /guest/ endpoints.
            const em = (b as any).attendee_email;
            if ((b as any).is_guest && em) {
              router.push({
                pathname: '/booking/[id]',
                params: { id: b.id, bookingNumber: (b as any).booking_number, email: em },
              } as any);
            } else {
              router.push(`/booking/${b.id}` as any);
            }
          }} />
        </View>
      ))}
    </View>
  );
});

// ─── Guest Lookup ─────────────────────────────────────────────────────────────

const GuestLookup = () => {
  const { isDark } = useAppTheme();
  setTicketsTheme(isDark);
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const [bookingNumber, setBookingNumber] = useState('');
  const [email, setEmail]               = useState('');
  const [loading, setLoading]           = useState(false);
  const [error, setError]               = useState<string | null>(null);
  const [result, setResult]             = useState<Booking | null>(null);
  // Previously looked-up guest tickets, persisted on-device so they can be
  // re-opened offline without re-entering (or re-fetching) anything.
  const [saved, setSaved]               = useState<Booking[]>([]);
  useEffect(() => { getGuestTickets<Booking>().then(setSaved).catch(() => {}); }, [result]);

  const lookup = async () => {
    const bn = bookingNumber.trim().toUpperCase();
    const em = email.trim().toLowerCase();
    if (!bn || !em) { setError('Enter both your booking number and email.'); return; }
    setLoading(true); setError(null); setResult(null);
    try {
      const { data } = await api.get<Booking>(`/v1/guest/bookings/${encodeURIComponent(bn)}`, { params: { email: em } });
      const rec = { ...data, is_guest: true, attendee_email: em } as Booking;
      setResult(rec);
      // One successful lookup = fully offline-capable from here on: persist the
      // ticket, cache it as screen data, and pull its offline entry tokens now.
      saveGuestTicket(rec, em);
      cacheBookingDetail(rec, []);
      if ((rec as any).booking_status === 'confirmed') {
        syncBookingOffline((rec as any).id, { bookingNumber: bn, email: em });
      }
    } catch (e: any) {
      const status = e?.response?.status;
      // No response at all = offline. If this ticket was found on this device
      // before, serve the saved copy instead of an error.
      if (!e?.response) {
        const savedTickets = await getGuestTickets<Booking>().catch(() => [] as Booking[]);
        const hit = savedTickets.find(t =>
          (t as any).booking_number?.toUpperCase() === bn &&
          (t as any).attendee_email?.toLowerCase() === em);
        if (hit) { setResult(hit); setLoading(false); return; }
        setError("You're offline. Tickets previously viewed on this device appear below.");
        setLoading(false); return;
      }
      setError(status === 404 ? 'No booking found. Check your booking number and email.'
        : status === 429 ? 'Too many attempts. Wait a moment and try again.'
        : e?.response?.data?.error ?? 'Something went wrong.');
    } finally { setLoading(false); }
  };

  if (result) {
    return (
      <ScrollView style={{ flex: 1 }} contentContainerStyle={[sheet.guestScroll, { paddingBottom: insets.bottom + 40 }]} showsVerticalScrollIndicator={false}>
        <Text className={s.sectionLabel}>YOUR TICKET</Text>
        <TicketStack bookings={[result]} />
        <TouchableOpacity style={sheet.lookupAgainBtn} onPress={() => setResult(null)}>
          {/* Themed: a white glyph on the light sheet's rgba(0,0,0,0.06) pill was
              invisible, so the button looked like a text-only row. */}
          <Ionicons name="search-outline" size={15} color={isDark ? 'rgba(255,255,255,0.6)' : 'rgba(0,0,0,0.55)'} />
          <Text className={s.lookupAgainText}>Look up another ticket</Text>
        </TouchableOpacity>
      </ScrollView>
    );
  }

  return (
    <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : 'height'}>
      <ScrollView contentContainerStyle={[sheet.guestScroll, { paddingBottom: insets.bottom + 40 }]} keyboardShouldPersistTaps="handled" showsVerticalScrollIndicator={false}>
        <View style={sheet.guestForm}>
          <View style={sheet.guestIconWrap}>
            <Ionicons name="ticket-outline" size={36} color={isDark ? 'rgba(255,255,255,0.18)' : 'rgba(0,0,0,0.18)'} />
          </View>
          <Text className={s.guestTitle}>Find your ticket</Text>
          <Text className={s.guestSub}>Enter your booking number and email from your confirmation.</Text>

          <View style={sheet.fieldGroup}>
            <Text className={s.fieldLabel}>BOOKING NUMBER</Text>
            <TextInput style={sheet.input} value={bookingNumber} onChangeText={setBookingNumber}
              placeholder="" placeholderTextColor={isDark ? 'rgba(255,255,255,0.2)' : 'rgba(0,0,0,0.25)'}
              autoCapitalize="characters" autoCorrect={false} returnKeyType="next" />
          </View>
          <View style={sheet.fieldGroup}>
            <Text className={s.fieldLabel}>EMAIL ADDRESS</Text>
            <TextInput style={sheet.input} value={email} onChangeText={setEmail}
              placeholder="" placeholderTextColor={isDark ? 'rgba(255,255,255,0.2)' : 'rgba(0,0,0,0.25)'}
              autoCapitalize="none" keyboardType="email-address" autoCorrect={false}
              returnKeyType="search" onSubmitEditing={lookup} />
          </View>

          {error && (
            <View style={sheet.errorBox}>
              <Ionicons name="alert-circle-outline" size={15} color="#ff6b6b" />
              <Text className={s.errorText}>{error}</Text>
            </View>
          )}

          <TouchableOpacity style={[sheet.lookupBtn, loading && { opacity: 0.6 }]} onPress={lookup} disabled={loading} activeOpacity={0.85}>
            {loading ? <ActivityIndicator color={isDark ? '#0a0a0a' : '#ffffff'} size="small" /> : <Text className={s.lookupBtnText}>Find Ticket</Text>}
          </TouchableOpacity>

          <TouchableOpacity style={sheet.signInPrompt} onPress={() => router.push('/auth/login' as any)}>
            <Text className={s.signInPromptText}>
              Have an account? <Text style={{ color: '#ff3c6e', fontWeight: '700' }}>Sign in</Text>
            </Text>
          </TouchableOpacity>
        </View>

        {saved.length > 0 && (
          <View style={{ marginTop: 28 }}>
            <Text className={s.sectionLabel}>YOUR SAVED TICKETS</Text>
            <TicketStack bookings={saved} />
          </View>
        )}
      </ScrollView>
    </KeyboardAvoidingView>
  );
};

// ─── Tickets View (auth + local merged) ───────────────────────────────────────

// One tab's content, rendered at a fixed width inside the sliding row below.
//
// It does NOT hide itself. The previous version toggled display:'none', which
// looked free but isn't: Yoga drops a display:'none' subtree out of layout
// entirely, so restoring it forces a full synchronous re-layout of ~20 cards
// (each with nested views, two gradients and an image) on the tap. That was the
// remaining stall after the remount fix — cheaper than remounting, still a
// frame-blocking layout pass.
//
// Now both panes stay laid out permanently and the parent translates between
// them on the native thread. Layout is computed once; switching costs nothing
// on the JS thread at all.
//
// React.memo holds here because every prop is reference-stable: `colors` is a
// module constant from useAppTheme, `insets` comes from the safe-area context,
// `bookings` is memoised upstream, and `onRefresh` is a useCallback.
const TicketPane = React.memo(({
  kind, bookings, insets, colors, subTextColor, iconTint, refreshing, onRefresh,
}: {
  kind: 'upcoming' | 'past';
  bookings: Booking[];
  insets: any;
  colors: any;
  subTextColor: string;
  iconTint: string;
  refreshing: boolean;
  onRefresh: () => void;
}) => {
  if (bookings.length === 0) {
    return (
      <View style={{ flex: 1 }}>
        <View className={s.centred}>
          <View style={sheet.emptyIcon}>
            <Ionicons name={kind === 'upcoming' ? 'calendar-outline' : 'time-outline'} size={44} color={iconTint} />
          </View>
          <Text className={s.emptyTitle} style={{ color: colors.text }}>
            {kind === 'upcoming' ? 'No upcoming events' : 'No past events'}
          </Text>
          <Text className={s.emptySub} style={{ color: subTextColor }}>
            {kind === 'upcoming'
              ? 'Your future bookings will show up here.'
              : "Events you've attended will appear here."}
          </Text>
        </View>
      </View>
    );
  }

  return (
    <ScrollView
      className={s.scroll}
      contentContainerStyle={{ paddingBottom: insets.bottom + 40 }}
      showsVerticalScrollIndicator={false}
      refreshControl={
        <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={subTextColor} />
      }
    >
      <TicketStack bookings={bookings} offset={0} />
    </ScrollView>
  );
});

const TicketsView = ({ apiBookings, loading, refreshing, onRefresh, error, onRetry }: {
  apiBookings: Booking[]; loading: boolean; refreshing: boolean;
  onRefresh: () => void; error: string | null; onRetry: () => void;
}) => {
  const insets             = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();
  setTicketsTheme(isDark);

  // useMemo, not bare expressions. These recomputed on EVERY render — so each
  // tab switch redid the dedup, three filters, and a `new Date()` parse per
  // booking, synchronously, before React could even start the list swap. That
  // work on the JS thread is what made the tab transition stall halfway: the
  // indicator itself animates natively, but the content underneath it froze.
  // `all` is returned too — the "no tickets at all" empty state below
  // distinguishes an empty account from an empty tab, so it needs the
  // unfiltered count, not just the per-tab lists.
  const { all, upcoming, past } = React.useMemo(() => {
    const list = dedup([...apiBookings]).filter(isRealTicket);
    return {
      all:      list,
      upcoming: list.filter(isUpcomingActive),
      past:     list.filter(b => !isUpcomingActive(b)),
    };
  }, [apiBookings]);

  const [activeTab,   setActiveTab]   = useState<'upcoming' | 'past'>('upcoming');
  const [tabBarWidth, setTabBarWidth] = useState(0);
  const { width: winWidth } = useWindowDimensions();
  const paneAnim = React.useRef(new Animated.Value(0)).current;

  // The Past pane is mounted once, AFTER the initial render settles, rather
  // than on first tap. Mounting ~20 image-backed cards during the switch
  // animation would hitch the very transition this is meant to smooth;
  // runAfterInteractions defers it to idle time so it's ready and warm before
  // the user ever reaches for the tab.
  const [pastEverShown, setPastEverShown] = useState(false);
  useEffect(() => {
    const task = InteractionManager.runAfterInteractions(() => setPastEverShown(true));
    return () => task.cancel();
  }, []);
  const indicatorAnim = React.useRef(new Animated.Value(0)).current;

  const switchTab = React.useCallback((tab: 'upcoming' | 'past') => {
    // Re-tapping the active tab used to restart the spring and force a full
    // re-render for no visual change.
    if (tab === activeTab) return;

    // Animation FIRST, state second. setActiveTab synchronously schedules the
    // list swap; starting the spring after it meant the animation was queued
    // behind that render work and visibly began late. Kicking it off first
    // hands it to the UI thread immediately, so the underline slides smoothly
    // even while JS is busy mounting the new cards.
    // Underline and content slide together, both on the native driver. Because
    // the panes are already laid out, this animation never touches the JS
    // thread — it stays smooth even if React is busy behind it.
    Animated.parallel([
      Animated.spring(indicatorAnim, {
        toValue: tab === 'past' ? tabBarWidth / 2 : 0,
        useNativeDriver: true,
        tension: 180,
        friction: 22,
      }),
      Animated.spring(paneAnim, {
        toValue: tab === 'past' ? -winWidth : 0,
        useNativeDriver: true,
        tension: 180,
        friction: 22,
      }),
    ]).start();

    setActiveTab(tab);
  }, [activeTab, tabBarWidth, winWidth, indicatorAnim, paneAnim]);

  const subTextColor  = colors.subText;
  const badgeBg       = isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)';
  const badgeBgActive = isDark ? 'rgba(255,255,255,0.15)' : 'rgba(0,0,0,0.12)';
  const iconTint      = isDark ? 'rgba(255,255,255,0.1)'  : 'rgba(0,0,0,0.1)';

  if (loading) return (
    <View className={s.centred}>
      <ActivityIndicator color={subTextColor} />
    </View>
  );

  if (error && all.length === 0) return (
    <View className={s.centred}>
      <Ionicons name="alert-circle-outline" size={36} color="rgba(255,80,80,0.4)" />
      <Text className={s.emptyTitle} style={{ color: colors.text }}>Something went wrong</Text>
      <Text className={s.emptySub}   style={{ color: subTextColor }}>{error}</Text>
      <TouchableOpacity style={sheet.retryBtn} onPress={onRetry}>
        <Text className={s.retryText} style={{ color: colors.text }}>Try again</Text>
      </TouchableOpacity>
    </View>
  );

  if (all.length === 0) return (
    <View className={s.centred}>
      <View style={sheet.emptyIcon}>
        <Ionicons name="ticket-outline" size={44} color={iconTint} />
      </View>
      <Text className={s.emptyTitle} style={{ color: colors.text }}>No tickets yet</Text>
      <Text className={s.emptySub}   style={{ color: subTextColor }}>
        Events you book will appear here as digital tickets.
      </Text>
    </View>
  );


  return (
    <View style={{ flex: 1 }}>
      {/* ── Tab bar ────────────────────────────────────────────────────── */}
      <View style={sheet.tabBarWrapper}>
        <View
          style={[sheet.tabBar, { borderBottomColor: colors.dividerColor }]}
          onLayout={(e) => {
            // Guard: onLayout fires on every layout pass, and an unconditional
            // setState here re-rendered the whole screen mid-transition.
            const w = e.nativeEvent.layout.width;
            setTabBarWidth((prev) => (prev === w ? prev : w));
          }}
        >
          <TouchableOpacity style={sheet.tabItem} activeOpacity={0.75} onPress={() => switchTab('upcoming')}>
            <Text
              className={activeTab === 'upcoming' ? s.tabLabelActive : s.tabLabelInactive}
              style={{ color: activeTab === 'upcoming' ? colors.text : subTextColor }}
            >
              Upcoming
            </Text>
          </TouchableOpacity>

          <TouchableOpacity style={sheet.tabItem} activeOpacity={0.75} onPress={() => switchTab('past')}>
            <Text
              className={activeTab === 'past' ? s.tabLabelActive : s.tabLabelInactive}
              style={{ color: activeTab === 'past' ? colors.text : subTextColor }}
            >
              Past
            </Text>
          </TouchableOpacity>
        </View>

        {/* Sliding underline — pixel values so useNativeDriver: true works */}
        <Animated.View
          style={[
            sheet.tabIndicator,
            {
              width: tabBarWidth / 2,
              backgroundColor: colors.text,
              transform: [{ translateX: indicatorAnim }],
            },
          ]}
        />
      </View>

      {/* ── Content ────────────────────────────────────────────────────────
          BOTH panes stay mounted; switching tabs only flips `display`.

          Previously the active tab's list was the only thing rendered, so every
          switch unmounted every card and mounted a fresh set. Each card carries
          a full-size ExpoImage with blurRadius, two LinearGradients, and ~15
          reads off the `s`/`sheet` Proxy objects in styles/tickets.ts (every
          property access there is a proxy trap, which Hermes handles poorly).
          Twenty cards made that a visible stall — the tab appeared to hang
          halfway through the transition.

          Keeping both mounted makes a switch a style change: no remount, no
          image reload, no re-blur, no proxy traps. The Past pane is mounted
          lazily on first visit so the initial screen load isn't paying for
          cards the user may never look at, and stays mounted afterwards. */}
      <View style={{ flex: 1, overflow: 'hidden' }}>
        <Animated.View
          style={{
            flex: 1,
            flexDirection: 'row',
            width: winWidth * 2,
            transform: [{ translateX: paneAnim }],
          }}
        >
          <View style={{ width: winWidth }}>
            <TicketPane
              kind="upcoming"
              bookings={upcoming}
              insets={insets}
              colors={colors}
              subTextColor={subTextColor}
              iconTint={iconTint}
              refreshing={refreshing}
              onRefresh={onRefresh}
            />
          </View>

          {/* Placeholder keeps the row exactly 2x wide before the Past pane
              mounts, so the translate lands in the right place regardless. */}
          <View style={{ width: winWidth }}>
            {pastEverShown && (
              <TicketPane
                kind="past"
                bookings={past}
                insets={insets}
                colors={colors}
                subTextColor={subTextColor}
                iconTint={iconTint}
                refreshing={refreshing}
                onRefresh={onRefresh}
              />
            )}
          </View>
        </Animated.View>
      </View>
    </View>
  );
};

 

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function TicketsScreen() {
  const insets = useSafeAreaInsets();
  const { user } = useAuthStore();
  const { colors, isDark } = useAppTheme();
  setTicketsTheme(isDark);

  const [apiBookings, setApiBookings] = useState<Booking[]>([]);
  const [loading, setLoading]         = useState(false);
  const [refreshing, setRefreshing]   = useState(false);
  const [error, setError]             = useState<string | null>(null);

  useEffect(() => {}, []);

  const fetchApiBookings = useCallback(async () => {
    if (!user) return;
    setError(null);
    try {
      const { data } = await api.get<{ data: Booking[]; next_cursor: string | null }>('/v1/bookings', { params: { limit: 50 } });
      const now = Date.now();
      const sorted = (data.data ?? []).slice().sort((a, b) => {
        const aT = a.event?.start_time ? new Date(a.event.start_time).getTime() : 0;
        const bT = b.event?.start_time ? new Date(b.event.start_time).getTime() : 0;
        const aUp = aT > now, bUp = bT > now;
        if (aUp && !bUp) return -1; if (!aUp && bUp) return 1;
        return aUp ? aT - bT : bT - aT;
      });
      setApiBookings(sorted);
      // Cache the list for offline viewing + pull each confirmed booking's
      // offline entry tokens in the background ("saved as soon as bought").
      cacheBookingsList(sorted);
      syncManyOffline(sorted as any);
    } catch (e: any) {
      // Offline / network down — fall back to the last cached list instead of erroring.
      const cached = await getCachedBookingsList<Booking>();
      if (cached.length) setApiBookings(cached);
      else setError(e?.response?.data?.error ?? 'Failed to load tickets');
    }
  }, [user]);

  const load    = useCallback(async () => { setLoading(true); await fetchApiBookings(); setLoading(false); }, [fetchApiBookings]);
  const refresh = useCallback(async () => { setRefreshing(true); await fetchApiBookings(); setRefreshing(false); }, [fetchApiBookings]);

  useEffect(() => { load(); }, [user]);

  // Re-fetch when the tab regains focus (e.g. returning here right after a
  // purchase) so a freshly-confirmed booking shows up immediately. The first
  // focus is skipped because the mount effect above already does the spinner
  // load; subsequent focuses refetch silently (no spinner flash).
  const focusedOnce = useRef(false);
  useFocusEffect(
    useCallback(() => {
      if (!focusedOnce.current) { focusedOnce.current = true; return; }
      if (user) fetchApiBookings();
    }, [user, fetchApiBookings]),
  );

  const showGuestLookup = !user;

  return (
    <View className={s.root} style={{ backgroundColor: colors.background }}>
      <View className={s.header} style={{ paddingTop: insets.top + 10, backgroundColor: colors.background }}>
        <Text className={s.headerTitle} style={{ color: colors.text }}>My Tickets</Text>
      </View>

      {showGuestLookup ? (
        <GuestLookup />
      ) : (
        <TicketsView
          apiBookings={apiBookings}
          loading={loading}
          refreshing={refreshing}
          onRefresh={refresh}
          error={error}
          onRetry={load}
        />
      )}
    </View>
  );
}