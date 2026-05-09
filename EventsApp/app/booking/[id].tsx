import React, { useState, useEffect, useRef } from 'react';
import {
  View, Text, ScrollView, TouchableOpacity, Image,
  ActivityIndicator, TextInput, Modal, Dimensions, Animated,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { Feather, Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { StyleSheet } from 'react-native';
import { api } from '../../lib/api';
import { useAuthStore } from '../../store/authStore';
import { s, sheet } from '../../styles/booking';

const { width: SW } = Dimensions.get('window');

// ─── Types ────────────────────────────────────────────────────────────────────

interface BookingDetail {
  id: string;
  booking_number: string;
  event_id: string;
  ticket_quantity: number;
  unit_price?: number;
  subtotal?: number;
  platform_fee?: number;
  organizer_fee?: number;
  total_amount: number;
  discount_amount?: number;
  currency: string;
  booking_status: string;
  payment_status: string;
  qr_code_url?: string | null;
  tickets_sent?: boolean;
  ticket_type_name?: string | null;
  attendee_first_name?: string | null;
  attendee_last_name?: string | null;
  attendee_email?: string | null;
  created_at: string;
  confirmed_at?: string | null;
  refunded_at?: string | null;
  refunded_reason?: string | null;
  refunded_amount?: number | null;
  event?: {
    title: string;
    start_time: string;
    venue_name: string;
    city: string;
    cover_image_url?: string | null;
    address?: string;
  };
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

const fmt       = (iso: string) => new Date(iso).toLocaleDateString('en-ZA', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });
const fmtTime   = (iso: string) => new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
const fmtMoney  = (n: number)   => `R${Math.round(n).toLocaleString('en-ZA')}`;
const fmtDT     = (iso: string) => new Date(iso).toLocaleString('en-ZA', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });

const STATUS_COLOR: Record<string, string> = {
  confirmed: '#00e5a0', pending: '#ffb830', cancelled: '#ff4d4d', refunded: '#888',
};

// ─── QR Placeholder ───────────────────────────────────────────────────────────

const QRPlaceholder = ({ value }: { value: string }) => {
  const cells = 17;
  const size  = Math.floor((SW - 96) / cells);
  const dots: { x: number; y: number }[] = [];
  let hash = 0;
  for (let i = 0; i < value.length; i++) hash = (hash * 31 + value.charCodeAt(i)) >>> 0;
  for (let r = 0; r < cells; r++) {
    for (let c = 0; c < cells; c++) {
      const isTL = r < 7 && c < 7, iTR = r < 7 && c >= cells - 7, iBL = r >= cells - 7 && c < 7;
      if (isTL || iTR || iBL) {
        const inB = r === 0 || r === 6 || c === 0 || c === 6 ||
          (r >= 2 && r <= 4 && c >= 2 && c <= 4) ||
          (r >= 2 && r <= 4 && c >= cells - 5 && c <= cells - 3) ||
          (r >= cells - 5 && r <= cells - 3 && c >= 2 && c <= 4);
        if (inB) dots.push({ x: c, y: r });
      } else {
        if (((hash >> ((r * cells + c) % 32)) ^ (r + c)) & 1) dots.push({ x: c, y: r });
      }
    }
  }
  return (
    <View style={{ width: cells * size, height: cells * size, backgroundColor: '#fff', borderRadius: 4 }}>
      {dots.map((d, i) => (
        <View key={i} style={{ position: 'absolute', left: d.x * size, top: d.y * size, width: size - 1, height: size - 1, backgroundColor: '#0a0a0a', borderRadius: 1 }} />
      ))}
    </View>
  );
};

// ─── Info Row ─────────────────────────────────────────────────────────────────

const InfoRow = ({ icon, label, value, valueColor }: { icon: React.ReactNode; label: string; value: string; valueColor?: string }) => (
  <View className={s.infoRow}>
    <View className={s.infoRowLeft}>{icon}<Text className={s.infoLabel}>{label}</Text></View>
    <Text className={s.infoValue} style={valueColor ? { color: valueColor } : undefined} numberOfLines={2}>{value}</Text>
  </View>
);

// ─── Price Line ───────────────────────────────────────────────────────────────

const PriceLine = ({ label, value, bold, accent }: { label: string; value: string; bold?: boolean; accent?: boolean }) => (
  <View className={s.priceLine}>
    <Text className={bold ? s.priceLabelBold : s.priceLabel}>{label}</Text>
    <Text className={bold ? s.priceValueBold : s.priceValue} style={accent ? { color: '#ff3c6e' } : undefined}>{value}</Text>
  </View>
);

// ─── Refund Modal ─────────────────────────────────────────────────────────────

const RefundModal = ({ visible, bookingId, onClose, onSuccess }: {
  visible: boolean; bookingId: string; onClose: () => void; onSuccess: () => void;
}) => {
  const [reason, setReason]   = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError]     = useState('');

  const submit = async () => {
    if (reason.trim().length < 10) { setError('Please provide at least 10 characters.'); return; }
    setLoading(true); setError('');
    try {
      await api.post(`/v1/bookings/${bookingId}/refund`, { reason: reason.trim() });
      onSuccess(); onClose();
    } catch (e: any) {
      setError(e?.response?.data?.error ?? 'Failed to submit. Please try again.');
    } finally { setLoading(false); }
  };

  return (
    <Modal visible={visible} animationType="slide" transparent presentationStyle="overFullScreen">
      <View className={s.modalOverlay}>
        <View style={sheet.modalSheet}>
          <View className={s.modalHandle} />
          <Text className={s.modalTitle}>Request a refund</Text>
          <Text className={s.modalSub}>Tell us why you'd like a refund. Processed within 3–5 business days.</Text>
          <Text className={s.modalFieldLabel}>REASON FOR REFUND</Text>
          <TextInput
            style={[sheet.modalInput, error ? sheet.modalInputError : undefined]}
            value={reason} onChangeText={v => { setReason(v); setError(''); }}
            placeholder="e.g. I can no longer attend this event due to..."
            placeholderTextColor="rgba(255,255,255,0.25)"
            multiline numberOfLines={4} maxLength={500} textAlignVertical="top"
          />
          <Text className={s.charCount}>{reason.length}/500</Text>
          {error ? (
            <View style={sheet.modalError}>
              <Ionicons name="alert-circle-outline" size={14} color="#ff6b6b" />
              <Text style={sheet.modalErrorText}>{error}</Text>
            </View>
          ) : null}
          <TouchableOpacity style={[sheet.modalBtn, loading && { opacity: 0.6 }]} onPress={submit} disabled={loading} activeOpacity={0.85}>
            {loading ? <ActivityIndicator color="#fff" size="small" /> : <Text className={s.modalBtnText}>Submit refund request</Text>}
          </TouchableOpacity>
          <TouchableOpacity className={s.modalCancel} onPress={onClose}>
            <Text className={s.modalCancelText}>Cancel</Text>
          </TouchableOpacity>
        </View>
      </View>
    </Modal>
  );
};

// ─── Mock booking details (mirrors MOCK_BOOKINGS in tickets.tsx) ──────────────

const MOCK_DETAILS: Record<string, BookingDetail> = {
  'mock-1': {
    id: 'mock-1',
    booking_number: 'BK-A3F7K2',
    event_id: 'evt-1',
    ticket_quantity: 2,
    unit_price: 700,
    subtotal: 1400,
    platform_fee: 80,
    organizer_fee: 0,
    total_amount: 1480,
    discount_amount: 0,
    currency: 'ZAR',
    booking_status: 'confirmed',
    payment_status: 'completed',
    qr_code_url: null,
    tickets_sent: true,
    ticket_type_name: 'VIP',
    attendee_first_name: 'Demo',
    attendee_last_name: 'User',
    attendee_email: 'demo@example.com',
    created_at: new Date(Date.now() - 3 * 86400000).toISOString(),
    confirmed_at: new Date(Date.now() - 3 * 86400000 + 60000).toISOString(),
    event: {
      title: 'Sun Met 2026 — Cape Town',
      start_time: new Date(Date.now() + 7 * 86400000).toISOString(),
      venue_name: 'Kenilworth Racecourse',
      city: 'Cape Town',
      cover_image_url: 'https://images.unsplash.com/photo-1551698618-1dfe5d97d256?w=800',
      address: 'Kenilworth Racecourse, Rosmead Ave, Kenilworth, Cape Town, 7708',
    },
  },
  'mock-2': {
    id: 'mock-2',
    booking_number: 'BK-R9Q2M1',
    event_id: 'evt-2',
    ticket_quantity: 1,
    unit_price: 650,
    subtotal: 650,
    platform_fee: 45,
    organizer_fee: 0,
    total_amount: 695,
    discount_amount: 0,
    currency: 'ZAR',
    booking_status: 'confirmed',
    payment_status: 'completed',
    qr_code_url: null,
    tickets_sent: true,
    ticket_type_name: 'GENERAL ADMISSION',
    attendee_first_name: 'Demo',
    attendee_last_name: 'User',
    attendee_email: 'demo@example.com',
    created_at: new Date(Date.now() - 1 * 86400000).toISOString(),
    confirmed_at: new Date(Date.now() - 1 * 86400000 + 60000).toISOString(),
    event: {
      title: 'Afro Nation Johannesburg',
      start_time: new Date(Date.now() + 1 * 86400000).toISOString(),
      venue_name: 'FNB Stadium',
      city: 'Johannesburg',
      cover_image_url: 'https://images.unsplash.com/photo-1493225457124-a3eb161ffa5f?w=800',
      address: 'FNB Stadium, Soccer City Ave, Nasrec, Johannesburg, 1809',
    },
  },
  'mock-3': {
    id: 'mock-3',
    booking_number: 'BK-T5X8N4',
    event_id: 'evt-3',
    ticket_quantity: 3,
    unit_price: 700,
    subtotal: 2100,
    platform_fee: 120,
    organizer_fee: 0,
    total_amount: 2220,
    discount_amount: 0,
    currency: 'ZAR',
    booking_status: 'confirmed',
    payment_status: 'completed',
    qr_code_url: null,
    tickets_sent: true,
    ticket_type_name: 'PREMIUM',
    attendee_first_name: 'Demo',
    attendee_last_name: 'User',
    attendee_email: 'demo@example.com',
    created_at: new Date(Date.now() - 40 * 86400000).toISOString(),
    confirmed_at: new Date(Date.now() - 40 * 86400000 + 60000).toISOString(),
    event: {
      title: 'Ultra South Africa — Johannesburg',
      start_time: new Date(Date.now() - 10 * 86400000).toISOString(),
      venue_name: 'Expo Centre',
      city: 'Johannesburg',
      cover_image_url: 'https://images.unsplash.com/photo-1470225620780-dba8ba36b745?w=800',
      address: 'Expo Centre, Nasrec Rd, Johannesburg, 2109',
    },
  },
  'mock-4': {
    id: 'mock-4',
    booking_number: 'BK-D2W6P9',
    event_id: 'evt-4',
    ticket_quantity: 1,
    unit_price: 350,
    subtotal: 350,
    platform_fee: 30,
    organizer_fee: 0,
    total_amount: 380,
    discount_amount: 0,
    currency: 'ZAR',
    booking_status: 'cancelled',
    payment_status: 'refunded',
    qr_code_url: null,
    tickets_sent: false,
    ticket_type_name: 'GENERAL ADMISSION',
    attendee_first_name: 'Demo',
    attendee_last_name: 'User',
    attendee_email: 'demo@example.com',
    created_at: new Date(Date.now() - 60 * 86400000).toISOString(),
    confirmed_at: new Date(Date.now() - 60 * 86400000 + 60000).toISOString(),
    refunded_at: new Date(Date.now() - 55 * 86400000).toISOString(),
    refunded_reason: 'Event cancelled by organiser.',
    refunded_amount: 380,
    event: {
      title: 'Design Indaba 2025',
      start_time: new Date(Date.now() - 45 * 86400000).toISOString(),
      venue_name: 'Cape Town International Convention Centre',
      city: 'Cape Town',
      cover_image_url: 'https://images.unsplash.com/photo-1540575467063-178a50c2df87?w=800',
      address: 'CTICC, Convention Square, 1 Lower Long St, Cape Town, 8001',
    },
  },
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function BookingDetailScreen() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { user } = useAuthStore();
  const [booking, setBooking]       = useState<BookingDetail | null>(null);
  const [loading, setLoading]       = useState(true);
  const [refundOpen, setRefundOpen] = useState(false);
  const [refunded, setRefunded]     = useState(false);
  const scrollY = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    const load = async () => {
      // Intercept mock IDs — no API call needed
      if (id?.startsWith('mock-')) {
        setBooking(MOCK_DETAILS[id] ?? null);
        setLoading(false);
        return;
      }
      if (!user) { setLoading(false); return; }
      try {
        const { data } = await api.get<BookingDetail>(`/v1/bookings/${id}`);
        setBooking(data);
      } catch {} finally { setLoading(false); }
    };
    load();
  }, [id]);

  const heroOpacity = scrollY.interpolate({ inputRange: [0, 180], outputRange: [1, 0.4], extrapolate: 'clamp' });
  const headerBg    = scrollY.interpolate({ inputRange: [140, 200], outputRange: ['rgba(10,10,10,0)', 'rgba(10,10,10,1)'], extrapolate: 'clamp' });

  if (loading) return (
    <View className={s.root} style={{ alignItems: 'center', justifyContent: 'center' }}>
      <ActivityIndicator color="rgba(255,255,255,0.5)" />
    </View>
  );

  if (!booking) return (
    <View className={s.root} style={{ alignItems: 'center', justifyContent: 'center', gap: 12, paddingHorizontal: 40 }}>
      <Ionicons name="alert-circle-outline" size={40} color="rgba(255,80,80,0.5)" />
      <Text className={s.errTitle}>Booking not found</Text>
      <TouchableOpacity className={s.backBtnFull} onPress={() => router.back()}>
        <Text className={s.backBtnFullText}>Go back</Text>
      </TouchableOpacity>
    </View>
  );

  const status    = refunded ? 'refunded' : booking.booking_status;
  const statusCol = STATUS_COLOR[status] ?? '#888';
  const canRefund = status === 'confirmed' && !refunded && !!user;
  const isMock    = !booking.unit_price;
  const qrValue   = booking.qr_code_url ?? booking.booking_number;

  return (
    <View className={s.root}>
      {/* Floating header */}
      <Animated.View className={s.floatingHeader} style={{ backgroundColor: headerBg, paddingTop: insets.top }}>
        <TouchableOpacity className={s.headerBack} onPress={() => router.back()}>
          <Feather name="arrow-left" size={20} color="#fff" />
        </TouchableOpacity>
        <Text className={s.headerTitle} numberOfLines={1}>{booking.event?.title ?? 'Ticket'}</Text>
        <View style={{ width: 36 }} />
      </Animated.View>

      <Animated.ScrollView
        onScroll={Animated.event([{ nativeEvent: { contentOffset: { y: scrollY } } }], { useNativeDriver: false })}
        scrollEventThrottle={16}
        contentContainerStyle={{ paddingBottom: insets.bottom + 48 }}
        showsVerticalScrollIndicator={false}
      >
        {/* Hero */}
        <Animated.View className={s.hero} style={{ opacity: heroOpacity }}>
          {booking.event?.cover_image_url
            ? <Image source={{ uri: booking.event.cover_image_url }} style={StyleSheet.absoluteFillObject} />
            : <View className={s.heroBg} />
          }
          <LinearGradient colors={['transparent', 'rgba(10,10,10,0.85)', '#0a0a0a']} style={StyleSheet.absoluteFillObject} />
          <View className={s.heroContent} style={{ paddingTop: insets.top + 56 }}>
            <View className={s.statusBadge} style={{ borderColor: `${statusCol}44`, backgroundColor: `${statusCol}18` }}>
              <View className={s.statusDot} style={{ backgroundColor: statusCol }} />
              <Text className={s.statusText} style={{ color: statusCol }}>{status.toUpperCase()}</Text>
            </View>
            <Text className={s.heroTitle}>{booking.event?.title}</Text>
            {booking.event?.start_time && (
              <Text className={s.heroDate}>{fmt(booking.event.start_time)} · {fmtTime(booking.event.start_time)}</Text>
            )}
          </View>
        </Animated.View>

        {/* QR */}
        <View className={s.qrSection}>
          <Text className={s.sectionLabel}>ENTRY PASS</Text>
          <View style={sheet.qrCard}>
            {booking.qr_code_url
              ? <Image source={{ uri: booking.qr_code_url }} style={sheet.qrImage} resizeMode="contain" />
              : <QRPlaceholder value={qrValue} />
            }
            <Text className={s.qrNumber}>#{booking.booking_number}</Text>
            <Text className={s.qrHint}>
              {status === 'confirmed' ? 'Present this at the venue entrance'
                : status === 'refunded' ? 'This ticket has been refunded'
                : 'Ticket not yet confirmed'}
            </Text>
          </View>
        </View>

        {/* Event Details */}
        <View style={sheet.card}>
          <Text className={s.sectionLabel}>EVENT DETAILS</Text>
          {booking.event?.start_time && (
            <InfoRow icon={<Ionicons name="calendar-outline" size={15} color="rgba(255,255,255,0.45)" />}
              label="Date & time" value={`${fmt(booking.event.start_time)} at ${fmtTime(booking.event.start_time)}`} />
          )}
          {booking.event && (
            <InfoRow icon={<Ionicons name="location-outline" size={15} color="rgba(255,255,255,0.45)" />}
              label="Venue" value={`${booking.event.venue_name}, ${booking.event.city}`} />
          )}
          {booking.event?.address && (
            <InfoRow icon={<Feather name="map-pin" size={15} color="rgba(255,255,255,0.45)" />}
              label="Address" value={booking.event.address} />
          )}
        </View>

        {/* Ticket Info */}
        <View style={sheet.card}>
          <Text className={s.sectionLabel}>TICKET INFO</Text>
          <InfoRow icon={<Ionicons name="ticket-outline" size={15} color="rgba(255,255,255,0.45)" />}
            label="Type" value={booking.ticket_type_name ?? 'General Admission'} />
          <InfoRow icon={<Feather name="users" size={15} color="rgba(255,255,255,0.45)" />}
            label="Quantity" value={`${booking.ticket_quantity} ticket${booking.ticket_quantity > 1 ? 's' : ''}`} />
          <InfoRow icon={<Feather name="check-circle" size={15} color="rgba(255,255,255,0.45)" />}
            label="Status" value={status.charAt(0).toUpperCase() + status.slice(1)} valueColor={statusCol} />
          <InfoRow icon={<Feather name="clock" size={15} color="rgba(255,255,255,0.45)" />}
            label="Booked on" value={fmtDT(booking.created_at)} />
          {booking.confirmed_at && (
            <InfoRow icon={<Feather name="check" size={15} color="rgba(255,255,255,0.45)" />}
              label="Confirmed" value={fmtDT(booking.confirmed_at)} />
          )}
        </View>

        {/* Attendee */}
        {(booking.attendee_first_name || booking.attendee_email) && (
          <View style={sheet.card}>
            <Text className={s.sectionLabel}>ATTENDEE</Text>
            {(booking.attendee_first_name || booking.attendee_last_name) && (
              <InfoRow icon={<Feather name="user" size={15} color="rgba(255,255,255,0.45)" />}
                label="Name" value={[booking.attendee_first_name, booking.attendee_last_name].filter(Boolean).join(' ')} />
            )}
            {booking.attendee_email && (
              <InfoRow icon={<Feather name="mail" size={15} color="rgba(255,255,255,0.45)" />}
                label="Email" value={booking.attendee_email} />
            )}
          </View>
        )}

        {/* Payment */}
        <View style={sheet.card}>
          <Text className={s.sectionLabel}>PAYMENT BREAKDOWN</Text>
          {!isMock && booking.unit_price != null && (
            <PriceLine label={`Unit price × ${booking.ticket_quantity}`} value={fmtMoney((booking.unit_price ?? 0) * booking.ticket_quantity)} />
          )}
          {!isMock && (booking.platform_fee ?? 0) > 0 && (
            <PriceLine label="Service fee" value={fmtMoney(booking.platform_fee ?? 0)} />
          )}
          {!isMock && (booking.discount_amount ?? 0) > 0 && (
            <PriceLine label="Discount" value={`-${fmtMoney(booking.discount_amount ?? 0)}`} accent />
          )}
          <View style={sheet.priceDivider} />
          <PriceLine label="Total paid" value={fmtMoney(booking.total_amount)} bold />
          {(status === 'refunded' || refunded) && (
            <>
              <View style={sheet.priceDivider} />
              <View className={s.refundedBanner}>
                <Ionicons name="checkmark-circle" size={15} color="#00e5a0" />
                <Text className={s.refundedText}>
                  {booking.refunded_amount != null
                    ? `Refund of ${fmtMoney(booking.refunded_amount)} is being processed`
                    : 'Refund request submitted — processing in 3–5 business days'}
                </Text>
              </View>
              {booking.refunded_reason && <Text className={s.refundReason}>Reason: {booking.refunded_reason}</Text>}
            </>
          )}
        </View>

        {/* Actions */}
        {canRefund && (
          <View className={s.actions}>
            <TouchableOpacity className={s.refundBtn} onPress={() => setRefundOpen(true)} activeOpacity={0.8}>
              <Feather name="rotate-ccw" size={15} color="#ff4d4d" />
              <Text className={s.refundBtnText}>Request refund</Text>
            </TouchableOpacity>
          </View>
        )}
      </Animated.ScrollView>

      <RefundModal visible={refundOpen} bookingId={booking.id}
        onClose={() => setRefundOpen(false)} onSuccess={() => setRefunded(true)} />
    </View>
  );
}