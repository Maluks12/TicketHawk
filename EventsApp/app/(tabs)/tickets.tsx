import React, { useEffect, useState, useCallback, useRef } from 'react';
import {
  View, Text, ScrollView, TouchableOpacity, RefreshControl,
 ActivityIndicator, Image, TextInput, KeyboardAvoidingView, Platform, Animated,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter, useFocusEffect } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { StyleSheet } from 'react-native';
import { api } from '../../lib/api';
import { useAuthStore } from '../../store/authStore';
import { cacheBookingsList, getCachedBookingsList, syncManyOffline, syncBookingOffline, cacheBookingDetail, saveGuestTicket, getGuestTickets } from '../../lib/offline';
import { s, sheet } from '../../styles/tickets';
import { useAppTheme } from '../../theme/colors'; 

const CARD_HEIGHT = 210;
const CARD_PEEK   = 72;

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

const TicketCard = ({ booking, index, onPress }: { booking: Booking; index: number; onPress: () => void }) => {
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
  <TouchableOpacity activeOpacity={0.93} onPress={onPress} style={[sheet.cardTouchable, { zIndex: 100 - index }]}>
    <LinearGradient colors={[pal.top, pal.bottom]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={sheet.card}>
      {booking.event?.cover_image_url && (
        <Image source={{ uri: booking.event.cover_image_url }} style={StyleSheet.absoluteFillObject} blurRadius={3} />
      )}
      {booking.event?.cover_image_url && (
        <LinearGradient colors={[`${pal.top}d8`, `${pal.bottom}f0`]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={StyleSheet.absoluteFillObject} />
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
};

// ─── Stack ────────────────────────────────────────────────────────────────────

const TicketStack = ({ bookings, offset = 0 }: { bookings: Booking[]; offset?: number }) => {
  const router = useRouter();
  return (
    <View>
      {bookings.map((b, i) => (
        <View key={b.id} style={i > 0 ? { marginTop: -(CARD_HEIGHT - CARD_PEEK) } : undefined}>
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
};

// ─── Guest Lookup ─────────────────────────────────────────────────────────────

const GuestLookup = () => {
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
          <Ionicons name="search-outline" size={15} color="rgba(255,255,255,0.6)" />
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
            <Ionicons name="ticket-outline" size={36} color="rgba(255,255,255,0.18)" />
          </View>
          <Text className={s.guestTitle}>Find your ticket</Text>
          <Text className={s.guestSub}>Enter your booking number and email from your confirmation.</Text>

          <View style={sheet.fieldGroup}>
            <Text className={s.fieldLabel}>BOOKING NUMBER</Text>
            <TextInput style={sheet.input} value={bookingNumber} onChangeText={setBookingNumber}
              placeholder="e.g. BK-A3F7K2" placeholderTextColor="rgba(255,255,255,0.2)"
              autoCapitalize="characters" autoCorrect={false} returnKeyType="next" />
          </View>
          <View style={sheet.fieldGroup}>
            <Text className={s.fieldLabel}>EMAIL ADDRESS</Text>
            <TextInput style={sheet.input} value={email} onChangeText={setEmail}
              placeholder="you@example.com" placeholderTextColor="rgba(255,255,255,0.2)"
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
            {loading ? <ActivityIndicator color="#0a0a0a" size="small" /> : <Text className={s.lookupBtnText}>Find Ticket</Text>}
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

const TicketsView = ({ apiBookings, loading, refreshing, onRefresh, error, onRetry }: {
  apiBookings: Booking[]; loading: boolean; refreshing: boolean;
  onRefresh: () => void; error: string | null; onRetry: () => void;
}) => {
  const insets             = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();

  const all      = dedup([...apiBookings]).filter(isRealTicket);
  const upcoming = all.filter(isUpcomingActive);
  const past     = all.filter(b => !isUpcomingActive(b));

  const [activeTab,   setActiveTab]   = useState<'upcoming' | 'past'>('upcoming');
  const [tabBarWidth, setTabBarWidth] = useState(0);
  const indicatorAnim = React.useRef(new Animated.Value(0)).current;

  const switchTab = (tab: 'upcoming' | 'past') => {
    setActiveTab(tab);
    Animated.spring(indicatorAnim, {
      toValue: tab === 'past' ? tabBarWidth / 2 : 0,
      useNativeDriver: true,
      tension: 180,
      friction: 22,
    }).start();
  };

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

  const visibleBookings = activeTab === 'upcoming' ? upcoming : past;
  const tabEmpty        = visibleBookings.length === 0;

  return (
    <View style={{ flex: 1 }}>
      {/* ── Tab bar ────────────────────────────────────────────────────── */}
      <View style={sheet.tabBarWrapper}>
        <View
          style={[sheet.tabBar, { borderBottomColor: colors.dividerColor }]}
          onLayout={(e) => setTabBarWidth(e.nativeEvent.layout.width)}
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

      {/* ── Content ────────────────────────────────────────────────────── */}
      {tabEmpty ? (
        <View className={s.centred}>
          <View style={sheet.emptyIcon}>
            <Ionicons
              name={activeTab === 'upcoming' ? 'calendar-outline' : 'time-outline'}
              size={44}
              color={iconTint}
            />
          </View>
          <Text className={s.emptyTitle} style={{ color: colors.text }}>
            {activeTab === 'upcoming' ? 'No upcoming events' : 'No past events'}
          </Text>
          <Text className={s.emptySub} style={{ color: subTextColor }}>
            {activeTab === 'upcoming'
              ? 'Your future bookings will show up here.'
              : "Events you've attended will appear here."}
          </Text>
        </View>
      ) : (
        <ScrollView
          className={s.scroll}
          contentContainerStyle={{ paddingBottom: insets.bottom + 40 }}
          showsVerticalScrollIndicator={false}
          refreshControl={
            <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={subTextColor} />
          }
        >
          <TicketStack bookings={visibleBookings} offset={0} />
        </ScrollView>
      )}
    </View>
  );
};

 

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function TicketsScreen() {
  const insets = useSafeAreaInsets();
  const { user } = useAuthStore();
  const { colors } = useAppTheme(); 

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