import React, { useEffect, useState, useCallback } from 'react';
import {
  View, Text, ScrollView, TouchableOpacity, RefreshControl,
  ActivityIndicator, Image, TextInput, KeyboardAvoidingView, Platform,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { StyleSheet } from 'react-native';
import { api, clearCart } from '../../lib/api';
import { useAuthStore } from '../../store/authStore';
import { s, sheet } from '../../styles/tickets';
import { useAppTheme } from '../../theme/colors'; 

const CARD_HEIGHT = 200;
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
          <Image source={{ uri: booking.event.cover_image_url }} style={StyleSheet.absoluteFillObject} blurRadius={2} />
        )}
        {booking.event?.cover_image_url && (
          <LinearGradient colors={[`${pal.top}e0`, `${pal.bottom}f5`]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={StyleSheet.absoluteFillObject} />
        )}

        {/* Top row */}
        <View className={s.cardTopRow}>
          <View style={[sheet.logoMark, { borderColor: `${pal.accent}50` }]}>
            <Ionicons name="ticket" size={12} color={pal.accent} />
          </View>
          <View className={s.statusPill} style={{ borderColor: `${statusColor}40`, backgroundColor: `${statusColor}18` }}>
            <View className={s.statusDot} style={{ backgroundColor: statusColor }} />
            <Text className={s.statusText} style={{ color: statusColor }}>{statusLabel}</Text>
          </View>
        </View>

        <Text className={s.eventTitle} numberOfLines={2}>{booking.event?.title ?? 'Event'}</Text>

        <View className={s.metaGroup}>
          {booking.event?.start_time && (
            <View className={s.metaRow}>
              <Ionicons name="calendar-outline" size={10} color="rgba(255,255,255,0.45)" />
              <Text className={s.metaText}>{fmt(booking.event.start_time)} · {fmtTime(booking.event.start_time)}</Text>
            </View>
          )}
          {booking.event && (
            <View className={s.metaRow}>
              <Ionicons name="location-outline" size={10} color="rgba(255,255,255,0.45)" />
              <Text className={s.metaText} numberOfLines={1}>{booking.event.venue_name}, {booking.event.city}</Text>
            </View>
          )}
        </View>

        {/* Perforation */}
        <View style={sheet.perfRow}>
          <View style={[sheet.perfCircle, { left: -10 }]} />
          <View style={sheet.perfLine}>
            {Array.from({ length: 20 }).map((_, i) => (
              <View key={i} style={[sheet.perfDash, { backgroundColor: `${pal.accent}28` }]} />
            ))}
          </View>
          <View style={[sheet.perfCircle, { right: -10 }]} />
        </View>

        {/* Bottom */}
        <View className={s.cardBottom}>
          <View>
            <Text className={s.bottomLabel}>{booking.ticket_type_name ?? 'GENERAL'}</Text>
            <Text className={s.bottomValue} style={{ color: pal.accent }}>
              {booking.ticket_quantity}× {booking.ticket_quantity === 1 ? 'ticket' : 'tickets'}
            </Text>
          </View>
          <View className={s.bottomCenter}>
            <Text className={s.bookingNum}>#{booking.booking_number}</Text>
          </View>
          <View className={s.bottomRight}>
            <Text className={s.bottomLabel}>TOTAL</Text>
            <Text className={s.bottomValue} style={{ color: pal.accent }}>
              R{Math.round(booking.total_amount).toLocaleString('en-ZA')}
            </Text>
          </View>
          {booking.booking_status === 'confirmed' && (
            <View style={[sheet.qrBadge, { backgroundColor: `${pal.accent}18`, borderColor: `${pal.accent}40` }]}>
              <Ionicons name="qr-code" size={20} color={pal.accent} />
            </View>
          )}
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
          <TicketCard booking={b} index={offset + i} onPress={() => router.push(`/booking/${b.id}` as any)} />
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

  const lookup = async () => {
    const bn = bookingNumber.trim().toUpperCase();
    const em = email.trim().toLowerCase();
    if (!bn || !em) { setError('Enter both your booking number and email.'); return; }
    setLoading(true); setError(null); setResult(null);
    try {
      const { data } = await api.get<Booking>(`/v1/guest/bookings/${encodeURIComponent(bn)}`, { params: { email: em } });
      setResult(data);
    } catch (e: any) {
      const status = e?.response?.status;
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
      </ScrollView>
    </KeyboardAvoidingView>
  );
};

// ─── Tickets View (auth + local merged) ───────────────────────────────────────

const TicketsView = ({ apiBookings, loading, refreshing, onRefresh, error, onRetry }: {
  apiBookings: Booking[]; loading: boolean; refreshing: boolean;
  onRefresh: () => void; error: string | null; onRetry: () => void;
}) => {
  const insets       = useSafeAreaInsets();
  const all      = dedup([...apiBookings]);
  const upcoming = all.filter(isUpcoming);
  const past     = all.filter(b => !isUpcoming(b));

  if (loading) return <View className={s.centred}><ActivityIndicator color="rgba(255,255,255,0.5)" /></View>;

  if (error && all.length === 0) return (
    <View className={s.centred}>
      <Ionicons name="alert-circle-outline" size={36} color="rgba(255,80,80,0.4)" />
      <Text className={s.emptyTitle}>Something went wrong</Text>
      <Text className={s.emptySub}>{error}</Text>
      <TouchableOpacity style={sheet.retryBtn} onPress={onRetry}><Text className={s.retryText}>Try again</Text></TouchableOpacity>
    </View>
  );

  if (all.length === 0) return (
    <View className={s.centred}>
      <View style={sheet.emptyIcon}><Ionicons name="ticket-outline" size={44} color="rgba(255,255,255,0.1)" /></View>
      <Text className={s.emptyTitle}>No tickets yet</Text>
      <Text className={s.emptySub}>Events you book will appear here as digital tickets.</Text>
    </View>
  );

  return (
    <ScrollView className={s.scroll} contentContainerStyle={{ paddingBottom: insets.bottom + 40 }}
      showsVerticalScrollIndicator={false}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor="rgba(255,255,255,0.5)" />}>
      {upcoming.length > 0 && (
        <View>
          <Text className={s.sectionLabel}>UPCOMING</Text>
          <TicketStack bookings={upcoming} offset={0} />
        </View>
      )}
      {past.length > 0 && (
        <View style={upcoming.length > 0 ? { marginTop: 36 } : undefined}>
          <Text className={s.sectionLabel}>PAST</Text>
          <TicketStack bookings={past} offset={upcoming.length} />
        </View>
      )}
    </ScrollView>
  );
};

// ─── Mock tickets (remove when real data is flowing) ──────────────────────────

const MOCK_BOOKINGS: Booking[] = [
  {
    id: 'mock-1',
    booking_number: 'BK-A3F7K2',
    event_id: 'evt-1',
    ticket_quantity: 2,
    total_amount: 1400,
    currency: 'ZAR',
    booking_status: 'confirmed',
    payment_status: 'completed',
    qr_code_url: null,
    created_at: new Date().toISOString(),
    ticket_type_name: 'VIP',
    event: {
      title: 'Sun Met 2026 — Cape Town',
      start_time: new Date(Date.now() + 7 * 86400000).toISOString(),
      venue_name: 'Kenilworth Racecourse',
      city: 'Cape Town',
      cover_image_url: 'https://images.unsplash.com/photo-1551698618-1dfe5d97d256?w=800',
    },
  },
  {
    id: 'mock-2',
    booking_number: 'BK-R9Q2M1',
    event_id: 'evt-2',
    ticket_quantity: 1,
    total_amount: 650,
    currency: 'ZAR',
    booking_status: 'confirmed',
    payment_status: 'completed',
    qr_code_url: null,
    created_at: new Date().toISOString(),
    ticket_type_name: 'GENERAL ADMISSION',
    event: {
      title: 'Afro Nation Johannesburg',
      start_time: new Date(Date.now() + 1 * 86400000).toISOString(),
      venue_name: 'FNB Stadium',
      city: 'Johannesburg',
      cover_image_url: 'https://images.unsplash.com/photo-1493225457124-a3eb161ffa5f?w=800',
    },
  },
  {
    id: 'mock-3',
    booking_number: 'BK-T5X8N4',
    event_id: 'evt-3',
    ticket_quantity: 3,
    total_amount: 2100,
    currency: 'ZAR',
    booking_status: 'confirmed',
    payment_status: 'completed',
    qr_code_url: null,
    created_at: new Date(Date.now() - 30 * 86400000).toISOString(),
    ticket_type_name: 'PREMIUM',
    event: {
      title: 'Ultra South Africa — Johannesburg',
      start_time: new Date(Date.now() - 10 * 86400000).toISOString(),
      venue_name: 'Expo Centre',
      city: 'Johannesburg',
      cover_image_url: 'https://images.unsplash.com/photo-1470225620780-dba8ba36b745?w=800',
    },
  },
  {
    id: 'mock-4',
    booking_number: 'BK-D2W6P9',
    event_id: 'evt-4',
    ticket_quantity: 1,
    total_amount: 350,
    currency: 'ZAR',
    booking_status: 'cancelled',
    payment_status: 'refunded',
    qr_code_url: null,
    created_at: new Date(Date.now() - 60 * 86400000).toISOString(),
    ticket_type_name: 'GENERAL ADMISSION',
    event: {
      title: 'Design Indaba 2025',
      start_time: new Date(Date.now() - 45 * 86400000).toISOString(),
      venue_name: 'Cape Town International Convention Centre',
      city: 'Cape Town',
      cover_image_url: 'https://images.unsplash.com/photo-1540575467063-178a50c2df87?w=800',
    },
  },
];

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function TicketsScreen() {
  const insets = useSafeAreaInsets();
  const { user } = useAuthStore();
  const { colors } = useAppTheme(); 

  const [apiBookings, setApiBookings] = useState<Booking[]>(MOCK_BOOKINGS); // remove mock and keep it like this:   const [apiBookings, setApiBookings] = useState<Booking[]>([]);
  const [loading, setLoading]         = useState(false);
  const [refreshing, setRefreshing]   = useState(false);
  const [error, setError]             = useState<string | null>(null);
  const [resetting, setResetting]     = useState(false);

  useEffect(() => {}, []);

  const handleReset = async () => {
    setResetting(true);
    try { await clearCart(); } catch (_) {}
    setApiBookings([]);
    await load();
    setResetting(false);
  };

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
    } catch (e: any) { setError(e?.response?.data?.error ?? 'Failed to load tickets'); }
  }, [user]);

  const load    = useCallback(async () => { setLoading(true); await fetchApiBookings(); setLoading(false); }, [fetchApiBookings]);
  const refresh = useCallback(async () => { setRefreshing(true); await fetchApiBookings(); setRefreshing(false); }, [fetchApiBookings]);

  useEffect(() => { load(); }, [user]);

  const totalCount = dedup([...apiBookings]).length;
  const showGuestLookup = false; // hardcoded mock mode — restore to: !user

  return (
    <View className={s.root} style={{ backgroundColor: colors.background }}>
      <View className={s.header} style={{ paddingTop: insets.top + 10, backgroundColor: colors.background }}>
        <Text className={s.headerTitle} style={{ color: colors.text }}>My Tickets</Text>
        {totalCount > 0 && (
          <View style={sheet.countPill}><Text style={sheet.countText}>{totalCount}</Text></View>
        )}
        <TouchableOpacity onPress={handleReset} disabled={resetting} style={{ marginLeft: 'auto', paddingHorizontal: 12, paddingVertical: 6, borderRadius: 20, backgroundColor: 'rgba(255,255,255,0.08)', marginRight: 4 }}>
          {resetting
            ? <ActivityIndicator size='small' color='rgba(255,255,255,0.5)' />
            : <Text style={{ color: 'rgba(255,255,255,0.5)', fontSize: 12, fontWeight: '600', letterSpacing: 0.5 }}>RESET</Text>
          }
        </TouchableOpacity>
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