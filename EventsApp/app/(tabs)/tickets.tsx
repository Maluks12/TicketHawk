import React, { useEffect, useState, useCallback } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  RefreshControl,
  ActivityIndicator,
  Image,
  TextInput,
  KeyboardAvoidingView,
  Platform,
  Dimensions,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { api } from '../../lib/api';
import { useAuthStore } from '../../store/authStore';

const CARD_HEIGHT = 200;
const CARD_PEEK = 72;

// ─── Types ────────────────────────────────────────────────────────────────────

interface BookingEvent {
  title: string;
  start_time: string;
  venue_name: string;
  city: string;
  cover_image_url?: string | null;
}

interface Booking {
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

// ─── Card palettes ────────────────────────────────────────────────────────────

const CARD_PALETTES: { top: string; bottom: string; accent: string }[] = [
  { top: '#0d2137', bottom: '#0a3d2e', accent: '#00e5a0' },
  { top: '#1a0a2e', bottom: '#2d0a3d', accent: '#b57bee' },
  { top: '#1a1200', bottom: '#3d2600', accent: '#ffb830' },
  { top: '#001a2e', bottom: '#003d5c', accent: '#29c5f6' },
  { top: '#1a0008', bottom: '#3d0015', accent: '#ff4d6d' },
  { top: '#0a1a00', bottom: '#1a3d00', accent: '#8fff00' },
];

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-ZA', {
    weekday: 'short', day: 'numeric', month: 'short', year: 'numeric',
  });
}
function formatTime(iso: string) {
  return new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}
function isUpcoming(b: Booking): boolean {
  return !!b.event?.start_time && new Date(b.event.start_time) > new Date();
}
function daysUntil(iso: string): number {
  return Math.ceil((new Date(iso).getTime() - Date.now()) / (1000 * 60 * 60 * 24));
}

// ─── Ticket Card ──────────────────────────────────────────────────────────────

const TicketCard = ({ booking, index, onPress }: { booking: Booking; index: number; onPress: () => void }) => {
  const pal = CARD_PALETTES[index % CARD_PALETTES.length];
  const upcoming = isUpcoming(booking);
  const days = booking.event?.start_time ? daysUntil(booking.event.start_time) : null;

  const statusLabel =
    booking.booking_status === 'confirmed' && upcoming
      ? days === 0 ? 'TODAY' : days === 1 ? 'TOMORROW' : `IN ${days} DAYS`
      : booking.booking_status === 'confirmed' ? 'ATTENDED'
      : booking.booking_status.toUpperCase();

  const statusColor =
    upcoming && booking.booking_status === 'confirmed' ? pal.accent
    : booking.booking_status === 'cancelled' || booking.booking_status === 'refunded' ? '#ff4d4d'
    : '#888';

  return (
    <TouchableOpacity activeOpacity={0.93} onPress={onPress} style={[styles.cardTouchable, { zIndex: 100 - index }]}>
      <LinearGradient colors={[pal.top, pal.bottom]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={styles.card}>
        {booking.event?.cover_image_url && (
          <Image source={{ uri: booking.event.cover_image_url }} style={StyleSheet.absoluteFillObject} blurRadius={2} />
        )}
        {booking.event?.cover_image_url && (
          <LinearGradient colors={[`${pal.top}e0`, `${pal.bottom}f5`]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={StyleSheet.absoluteFillObject} />
        )}
        <View style={styles.cardTopRow}>
          <View style={[styles.logoMark, { borderColor: `${pal.accent}50` }]}>
            <Ionicons name="ticket" size={12} color={pal.accent} />
          </View>
          <View style={[styles.statusPill, { borderColor: `${statusColor}40`, backgroundColor: `${statusColor}18` }]}>
            <View style={[styles.statusDot, { backgroundColor: statusColor }]} />
            <Text style={[styles.statusText, { color: statusColor }]}>{statusLabel}</Text>
          </View>
        </View>
        <Text style={styles.eventTitle} numberOfLines={2}>{booking.event?.title ?? 'Event'}</Text>
        <View style={styles.metaGroup}>
          {booking.event?.start_time && (
            <View style={styles.metaRow}>
              <Ionicons name="calendar-outline" size={10} color="rgba(255,255,255,0.45)" />
              <Text style={styles.metaText}>{formatDate(booking.event.start_time)} · {formatTime(booking.event.start_time)}</Text>
            </View>
          )}
          {booking.event && (
            <View style={styles.metaRow}>
              <Ionicons name="location-outline" size={10} color="rgba(255,255,255,0.45)" />
              <Text style={styles.metaText} numberOfLines={1}>{booking.event.venue_name}, {booking.event.city}</Text>
            </View>
          )}
        </View>
        <View style={styles.perfRow}>
          <View style={[styles.perfCircle, { left: -10 }]} />
          <View style={styles.perfLine}>
            {Array.from({ length: 20 }).map((_, i) => (
              <View key={i} style={[styles.perfDash, { backgroundColor: `${pal.accent}28` }]} />
            ))}
          </View>
          <View style={[styles.perfCircle, { right: -10 }]} />
        </View>
        <View style={styles.cardBottom}>
          <View>
            <Text style={styles.bottomLabel}>{booking.ticket_type_name ?? 'GENERAL'}</Text>
            <Text style={[styles.bottomValue, { color: pal.accent }]}>
              {booking.ticket_quantity}× {booking.ticket_quantity === 1 ? 'ticket' : 'tickets'}
            </Text>
          </View>
          <View style={styles.bottomCenter}>
            <Text style={styles.bookingNum}>#{booking.booking_number}</Text>
          </View>
          <View style={styles.bottomRight}>
            <Text style={styles.bottomLabel}>TOTAL</Text>
            <Text style={[styles.bottomValue, { color: pal.accent }]}>
              R{Math.round(booking.total_amount).toLocaleString('en-ZA')}
            </Text>
          </View>
          {booking.booking_status === 'confirmed' && (
            <View style={[styles.qrBadge, { backgroundColor: `${pal.accent}18`, borderColor: `${pal.accent}40` }]}>
              <Ionicons name="qr-code" size={20} color={pal.accent} />
            </View>
          )}
        </View>
      </LinearGradient>
    </TouchableOpacity>
  );
};

// ─── Guest Lookup ─────────────────────────────────────────────────────────────

const GuestLookup = () => {
  const insets = useSafeAreaInsets();
  const [bookingNumber, setBookingNumber] = useState('');
  const [email, setEmail] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<Booking | null>(null);

  const lookup = async () => {
    const bn = bookingNumber.trim().toUpperCase();
    const em = email.trim().toLowerCase();
    if (!bn || !em) {
      setError('Please enter both your booking number and email address.');
      return;
    }
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      const { data } = await api.get<Booking>(
        `/v1/guest/bookings/${encodeURIComponent(bn)}`,
        { params: { email: em } },
      );
      setResult(data);
    } catch (e: any) {
      const msg = e?.response?.data?.error;
      if (e?.response?.status === 404) {
        setError('No booking found. Double-check your booking number and email.');
      } else if (e?.response?.status === 429) {
        setError('Too many attempts. Please wait a while before trying again.');
      } else {
        setError(msg ?? 'Something went wrong. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  };

  const reset = () => {
    setResult(null);
    setBookingNumber('');
    setEmail('');
    setError(null);
  };

  return (
    <KeyboardAvoidingView
      style={{ flex: 1 }}
      behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
    >
      <ScrollView
        contentContainerStyle={[styles.guestScroll, { paddingBottom: insets.bottom + 40 }]}
        keyboardShouldPersistTaps="handled"
        showsVerticalScrollIndicator={false}
      >
        {result ? (
          <View style={{ gap: 20 }}>
            <Text style={styles.sectionLabel}>YOUR TICKET</Text>
            <TicketCard booking={result} index={0} onPress={() => {}} />
            <TouchableOpacity style={styles.lookupAgainBtn} onPress={reset}>
              <Ionicons name="search-outline" size={15} color="rgba(255,255,255,0.6)" />
              <Text style={styles.lookupAgainText}>Look up another ticket</Text>
            </TouchableOpacity>
          </View>
        ) : (
          <View style={styles.guestForm}>
            <View style={styles.guestIconWrap}>
              <Ionicons name="ticket-outline" size={36} color="rgba(255,255,255,0.18)" />
            </View>
            <Text style={styles.guestTitle}>Find your ticket</Text>
            <Text style={styles.guestSub}>
              Enter the booking number and email address from your confirmation email.
            </Text>

            <View style={styles.fieldGroup}>
              <Text style={styles.fieldLabel}>BOOKING NUMBER</Text>
              <TextInput
                style={styles.input}
                value={bookingNumber}
                onChangeText={t => setBookingNumber(t)}
                placeholder="e.g. BK-00123"
                placeholderTextColor="rgba(255,255,255,0.2)"
                autoCapitalize="characters"
                autoCorrect={false}
                returnKeyType="next"
              />
            </View>

            <View style={styles.fieldGroup}>
              <Text style={styles.fieldLabel}>EMAIL ADDRESS</Text>
              <TextInput
                style={styles.input}
                value={email}
                onChangeText={t => setEmail(t)}
                placeholder="you@example.com"
                placeholderTextColor="rgba(255,255,255,0.2)"
                autoCapitalize="none"
                keyboardType="email-address"
                autoCorrect={false}
                returnKeyType="search"
                onSubmitEditing={lookup}
              />
            </View>

            {error && (
              <View style={styles.errorBox}>
                <Ionicons name="alert-circle-outline" size={15} color="#ff6b6b" />
                <Text style={styles.errorText}>{error}</Text>
              </View>
            )}

            <TouchableOpacity
              style={[styles.lookupBtn, loading && { opacity: 0.6 }]}
              onPress={lookup}
              disabled={loading}
              activeOpacity={0.85}
            >
              {loading
                ? <ActivityIndicator color="#0a0a0a" size="small" />
                : <Text style={styles.lookupBtnText}>Find Ticket</Text>
              }
            </TouchableOpacity>
          </View>
        )}
      </ScrollView>
    </KeyboardAvoidingView>
  );
};

// ─── Authenticated Tickets ────────────────────────────────────────────────────

const AuthTickets = () => {
  const insets = useSafeAreaInsets();
  const [bookings, setBookings] = useState<Booking[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchBookings = useCallback(async () => {
    try {
      setError(null);
      const { data } = await api.get<{ data: Booking[]; next_cursor: string | null }>('/v1/bookings', {
        params: { limit: 50 },
      });
      const now = Date.now();
      const sorted = (data.data ?? []).slice().sort((a, b) => {
        const aT = a.event?.start_time ? new Date(a.event.start_time).getTime() : 0;
        const bT = b.event?.start_time ? new Date(b.event.start_time).getTime() : 0;
        const aUp = aT > now, bUp = bT > now;
        if (aUp && !bUp) return -1;
        if (!aUp && bUp) return 1;
        return aUp ? aT - bT : bT - aT;
      });
      setBookings(sorted);
    } catch (e: any) {
      setError(e?.response?.data?.error ?? 'Failed to load tickets');
    }
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    await fetchBookings();
    setLoading(false);
  }, [fetchBookings]);

  const refresh = useCallback(async () => {
    setRefreshing(true);
    await fetchBookings();
    setRefreshing(false);
  }, [fetchBookings]);

  useEffect(() => { load(); }, [load]);

  const upcoming = bookings.filter(isUpcoming);
  const past = bookings.filter(b => !isUpcoming(b));

  if (loading) {
    return <View style={styles.centred}><ActivityIndicator color="rgba(255,255,255,0.6)" /></View>;
  }

  if (error) {
    return (
      <View style={styles.centred}>
        <Ionicons name="alert-circle-outline" size={36} color="rgba(255,80,80,0.4)" />
        <Text style={styles.emptyTitle}>Something went wrong</Text>
        <Text style={styles.emptySub}>{error}</Text>
        <TouchableOpacity style={styles.retryBtn} onPress={load}>
          <Text style={styles.retryText}>Try again</Text>
        </TouchableOpacity>
      </View>
    );
  }

  if (bookings.length === 0) {
    return (
      <View style={styles.centred}>
        <View style={styles.emptyIcon}>
          <Ionicons name="ticket-outline" size={44} color="rgba(255,255,255,0.1)" />
        </View>
        <Text style={styles.emptyTitle}>No tickets yet</Text>
        <Text style={styles.emptySub}>Events you book will appear here as digital tickets.</Text>
      </View>
    );
  }

  return (
    <ScrollView
      contentContainerStyle={[styles.scroll, { paddingBottom: insets.bottom + 40 }]}
      showsVerticalScrollIndicator={false}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={refresh} tintColor="rgba(255,255,255,0.5)" />}
    >
      {upcoming.length > 0 && (
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>UPCOMING</Text>
          <View>
            {upcoming.map((b, i) => (
              <View key={b.id} style={i > 0 ? { marginTop: -(CARD_HEIGHT - CARD_PEEK) } : undefined}>
                <TicketCard booking={b} index={i} onPress={() => {}} />
              </View>
            ))}
          </View>
        </View>
      )}
      {past.length > 0 && (
        <View style={[styles.section, upcoming.length > 0 && { marginTop: 36 }]}>
          <Text style={styles.sectionLabel}>PAST</Text>
          <View>
            {past.map((b, i) => (
              <View key={b.id} style={i > 0 ? { marginTop: -(CARD_HEIGHT - CARD_PEEK) } : undefined}>
                <TicketCard booking={b} index={upcoming.length + i} onPress={() => {}} />
              </View>
            ))}
          </View>
        </View>
      )}
    </ScrollView>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function TicketsScreen() {
  const insets = useSafeAreaInsets();
  const { user } = useAuthStore();

  return (
    <View style={styles.root}>
      <View style={[styles.header, { paddingTop: insets.top + 10 }]}>
        <Text style={styles.headerTitle}>My Tickets</Text>
      </View>
      {user ? <AuthTickets /> : <GuestLookup />}
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: '#0a0a0a' },
  header: { flexDirection: 'row', alignItems: 'center', gap: 10, paddingHorizontal: 24, paddingBottom: 18 },
  headerTitle: { fontSize: 28, fontWeight: '800', color: '#fff', letterSpacing: -0.5 },
  centred: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 12, paddingHorizontal: 40 },
  scroll: { paddingHorizontal: 20, paddingTop: 4 },
  section: {},
  sectionLabel: { fontSize: 10, fontWeight: '800', letterSpacing: 2.5, color: 'rgba(255,255,255,0.28)', marginBottom: 14, marginLeft: 2 },

  // Card
  cardTouchable: {
    borderRadius: 20,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 14 },
    shadowOpacity: 0.6,
    shadowRadius: 22,
    elevation: 16,
  },
  card: {
    width: '100%', height: CARD_HEIGHT, borderRadius: 20, padding: 18,
    overflow: 'hidden', justifyContent: 'space-between',
  },
  cardTopRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  logoMark: { width: 26, height: 26, borderRadius: 7, borderWidth: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(255,255,255,0.05)' },
  statusPill: { flexDirection: 'row', alignItems: 'center', gap: 5, paddingHorizontal: 9, paddingVertical: 4, borderRadius: 20, borderWidth: 1 },
  statusDot: { width: 5, height: 5, borderRadius: 3 },
  statusText: { fontSize: 9, fontWeight: '800', letterSpacing: 1.5 },
  eventTitle: { fontSize: 18, fontWeight: '800', color: '#fff', letterSpacing: -0.2, lineHeight: 22, flex: 1, marginVertical: 2 },
  metaGroup: { gap: 3 },
  metaRow: { flexDirection: 'row', alignItems: 'center', gap: 5 },
  metaText: { fontSize: 11, color: 'rgba(255,255,255,0.5)', fontWeight: '500', flex: 1 },
  perfRow: { flexDirection: 'row', alignItems: 'center', marginHorizontal: -18, position: 'relative' },
  perfCircle: { width: 16, height: 16, borderRadius: 8, backgroundColor: '#0a0a0a', position: 'absolute', zIndex: 2 },
  perfLine: { flex: 1, flexDirection: 'row', gap: 4, overflow: 'hidden', marginHorizontal: 8 },
  perfDash: { flex: 1, height: 1 },
  cardBottom: { flexDirection: 'row', alignItems: 'flex-end', gap: 8 },
  bottomLabel: { fontSize: 8, fontWeight: '700', letterSpacing: 1.5, color: 'rgba(255,255,255,0.38)', marginBottom: 2 },
  bottomValue: { fontSize: 13, fontWeight: '800' },
  bottomCenter: { flex: 1, alignItems: 'center' },
  bookingNum: { fontSize: 10, color: 'rgba(255,255,255,0.25)', fontWeight: '600', letterSpacing: 0.5 },
  bottomRight: { alignItems: 'flex-end' },
  qrBadge: { width: 38, height: 38, borderRadius: 10, borderWidth: 1, alignItems: 'center', justifyContent: 'center' },

  // Empty / error
  emptyIcon: { width: 80, height: 80, borderRadius: 24, backgroundColor: 'rgba(255,255,255,0.04)', alignItems: 'center', justifyContent: 'center', marginBottom: 4 },
  emptyTitle: { fontSize: 19, fontWeight: '800', color: '#fff' },
  emptySub: { fontSize: 13, color: 'rgba(255,255,255,0.32)', textAlign: 'center', lineHeight: 20 },
  retryBtn: { marginTop: 6, paddingHorizontal: 20, paddingVertical: 9, borderRadius: 10, backgroundColor: 'rgba(255,255,255,0.07)' },
  retryText: { color: '#fff', fontSize: 13, fontWeight: '700' },

  // Guest lookup
  guestScroll: { flexGrow: 1, paddingHorizontal: 20, paddingTop: 4 },
  guestForm: { flex: 1, gap: 20 },
  guestIconWrap: { width: 72, height: 72, borderRadius: 20, backgroundColor: 'rgba(255,255,255,0.04)', alignItems: 'center', justifyContent: 'center', alignSelf: 'center', marginTop: 24 },
  guestTitle: { fontSize: 22, fontWeight: '800', color: '#fff', textAlign: 'center', letterSpacing: -0.3 },
  guestSub: { fontSize: 13, color: 'rgba(255,255,255,0.38)', textAlign: 'center', lineHeight: 20 },
  fieldGroup: { gap: 8 },
  fieldLabel: { fontSize: 10, fontWeight: '800', letterSpacing: 2, color: 'rgba(255,255,255,0.3)' },
  input: {
    backgroundColor: 'rgba(255,255,255,0.06)',
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.1)',
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 14,
    color: '#fff',
    fontSize: 15,
    fontWeight: '500',
  },
  errorBox: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: 8,
    backgroundColor: 'rgba(255,80,80,0.08)',
    borderWidth: 1,
    borderColor: 'rgba(255,80,80,0.2)',
    borderRadius: 10,
    padding: 12,
  },
  errorText: { color: '#ff8080', fontSize: 13, lineHeight: 18, flex: 1 },
  lookupBtn: {
    backgroundColor: '#fff',
    borderRadius: 12,
    paddingVertical: 15,
    alignItems: 'center',
    marginTop: 4,
  },
  lookupBtnText: { color: '#0a0a0a', fontSize: 15, fontWeight: '800', letterSpacing: 0.2 },
  lookupAgainBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 7,
    paddingVertical: 12,
    borderRadius: 12,
    backgroundColor: 'rgba(255,255,255,0.06)',
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.1)',
  },
  lookupAgainText: { color: 'rgba(255,255,255,0.6)', fontSize: 13, fontWeight: '600' },
});