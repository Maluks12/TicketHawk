import React, { useState, useEffect, useRef } from 'react';
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity,
  Image, ActivityIndicator, Alert, TextInput, Modal,
  Dimensions, Animated, Platform,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { Feather, Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { api } from '../../lib/api';
import { useAuthStore } from '../../store/authStore';
import { useMockBookingStore } from '../../store/mockBookingStore';

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

const fmt = (iso: string) =>
  new Date(iso).toLocaleDateString('en-ZA', {
    weekday: 'long', day: 'numeric', month: 'long', year: 'numeric',
  });

const fmtTime = (iso: string) =>
  new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });

const fmtMoney = (n: number, currency = 'ZAR') =>
  `R${Math.round(n).toLocaleString('en-ZA')}`;

const fmtDateTime = (iso: string) =>
  new Date(iso).toLocaleString('en-ZA', {
    day: 'numeric', month: 'short', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  });

const STATUS_COLOR: Record<string, string> = {
  confirmed:  '#00e5a0',
  pending:    '#ffb830',
  cancelled:  '#ff4d4d',
  refunded:   '#888',
};

// ─── QR Code (SVG-based placeholder for mock, real image for API bookings) ───

const QRPlaceholder = ({ value }: { value: string }) => {
  // Simple deterministic dot grid from the booking number string
  const cells = 17;
  const size = Math.floor((SW - 96) / cells);
  const dots: { x: number; y: number }[] = [];
  let hash = 0;
  for (let i = 0; i < value.length; i++) hash = (hash * 31 + value.charCodeAt(i)) >>> 0;
  // Fixed finder patterns + data dots
  for (let r = 0; r < cells; r++) {
    for (let c = 0; c < cells; c++) {
      const isFinderTL = r < 7 && c < 7;
      const isFinderTR = r < 7 && c >= cells - 7;
      const isFinderBL = r >= cells - 7 && c < 7;
      if (isFinderTL || isFinderTR || isFinderBL) {
        const inBorder = r === 0 || r === 6 || c === 0 || c === 6 ||
          (r >= 2 && r <= 4 && c >= 2 && c <= 4) ||
          (r >= 2 && r <= 4 && c >= cells - 5 && c <= cells - 3) ||
          (r >= cells - 5 && r <= cells - 3 && c >= 2 && c <= 4);
        if (inBorder) dots.push({ x: c, y: r });
      } else {
        const bit = ((hash >> ((r * cells + c) % 32)) ^ (r + c)) & 1;
        if (bit) dots.push({ x: c, y: r });
      }
    }
  }
  const total = cells * size;
  return (
    <View style={{ width: total, height: total, backgroundColor: '#fff', borderRadius: 4 }}>
      {dots.map((d, i) => (
        <View key={i} style={{
          position: 'absolute',
          left: d.x * size, top: d.y * size,
          width: size - 1, height: size - 1,
          backgroundColor: '#0a0a0a',
          borderRadius: 1,
        }} />
      ))}
    </View>
  );
};

// ─── Row components ───────────────────────────────────────────────────────────

const InfoRow = ({ icon, label, value, valueColor }: {
  icon: React.ReactNode; label: string; value: string; valueColor?: string;
}) => (
  <View style={s.infoRow}>
    <View style={s.infoRowLeft}>
      {icon}
      <Text style={s.infoLabel}>{label}</Text>
    </View>
    <Text style={[s.infoValue, valueColor ? { color: valueColor } : undefined]} numberOfLines={2}>
      {value}
    </Text>
  </View>
);

const PriceLine = ({ label, value, bold, accent }: {
  label: string; value: string; bold?: boolean; accent?: boolean;
}) => (
  <View style={s.priceLine}>
    <Text style={[s.priceLabel, bold && s.priceLabelBold]}>{label}</Text>
    <Text style={[s.priceValue, bold && s.priceValueBold, accent && { color: '#ff3c6e' }]}>{value}</Text>
  </View>
);

// ─── Refund Modal ─────────────────────────────────────────────────────────────

const RefundModal = ({
  visible, bookingId, onClose, onSuccess,
}: {
  visible: boolean; bookingId: string; onClose: () => void; onSuccess: () => void;
}) => {
  const [reason, setReason] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const submit = async () => {
    if (reason.trim().length < 10) {
      setError('Please provide at least 10 characters explaining the reason.');
      return;
    }
    setLoading(true);
    setError('');
    try {
      await api.post(`/v1/bookings/${bookingId}/refund`, { reason: reason.trim() });
      onSuccess();
      onClose();
    } catch (e: any) {
      setError(e?.response?.data?.error ?? 'Failed to submit refund request. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal visible={visible} animationType="slide" transparent presentationStyle="overFullScreen">
      <View style={s.modalOverlay}>
        <View style={s.modalSheet}>
          <View style={s.modalHandle} />
          <Text style={s.modalTitle}>Request a refund</Text>
          <Text style={s.modalSub}>
            Tell us why you'd like a refund. Your request will be reviewed and processed within 3–5 business days.
          </Text>

          <Text style={s.modalFieldLabel}>REASON FOR REFUND</Text>
          <TextInput
            style={[s.modalInput, error ? s.modalInputError : undefined]}
            value={reason}
            onChangeText={v => { setReason(v); setError(''); }}
            placeholder="e.g. I can no longer attend this event due to..."
            placeholderTextColor="rgba(255,255,255,0.25)"
            multiline
            numberOfLines={4}
            maxLength={500}
            textAlignVertical="top"
          />
          <Text style={s.charCount}>{reason.length}/500</Text>

          {error ? (
            <View style={s.modalError}>
              <Ionicons name="alert-circle-outline" size={14} color="#ff6b6b" />
              <Text style={s.modalErrorText}>{error}</Text>
            </View>
          ) : null}

          <TouchableOpacity
            style={[s.modalBtn, loading && { opacity: 0.6 }]}
            onPress={submit}
            disabled={loading}
            activeOpacity={0.85}
          >
            {loading
              ? <ActivityIndicator color="#fff" size="small" />
              : <Text style={s.modalBtnText}>Submit refund request</Text>
            }
          </TouchableOpacity>

          <TouchableOpacity style={s.modalCancel} onPress={onClose}>
            <Text style={s.modalCancelText}>Cancel</Text>
          </TouchableOpacity>
        </View>
      </View>
    </Modal>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function BookingDetailScreen() {
  const insets = useSafeAreaInsets();
  const router  = useRouter();
  const { id }  = useLocalSearchParams<{ id: string }>();
  const { user } = useAuthStore();
  const mockBookings = useMockBookingStore(s => s.bookings);

  const [booking, setBooking]     = useState<BookingDetail | null>(null);
  const [loading, setLoading]     = useState(true);
  const [refundOpen, setRefundOpen] = useState(false);
  const [refunded, setRefunded]   = useState(false);
  const scrollY = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    const load = async () => {
      // Try mock store first (works offline / before login)
      const mock = mockBookings.find(b => b.id === id);
      if (mock) {
        setBooking(mock as unknown as BookingDetail);
        setLoading(false);
        return;
      }
      // Fetch from API
      if (!user) { setLoading(false); return; }
      try {
        const { data } = await api.get<BookingDetail>(`/v1/bookings/${id}`);
        setBooking(data);
      } catch {
        // handled in render
      } finally {
        setLoading(false);
      }
    };
    load();
  }, [id]);

  const heroOpacity = scrollY.interpolate({
    inputRange: [0, 180],
    outputRange: [1, 0.4],
    extrapolate: 'clamp',
  });

  const headerBg = scrollY.interpolate({
    inputRange: [140, 200],
    outputRange: ['rgba(10,10,10,0)', 'rgba(10,10,10,1)'],
    extrapolate: 'clamp',
  });

  if (loading) {
    return (
      <View style={[s.root, { alignItems: 'center', justifyContent: 'center' }]}>
        <ActivityIndicator color="rgba(255,255,255,0.5)" />
      </View>
    );
  }

  if (!booking) {
    return (
      <View style={[s.root, { alignItems: 'center', justifyContent: 'center', gap: 12, paddingHorizontal: 40 }]}>
        <Ionicons name="alert-circle-outline" size={40} color="rgba(255,80,80,0.5)" />
        <Text style={s.errTitle}>Booking not found</Text>
        <TouchableOpacity style={s.backBtnFull} onPress={() => router.back()}>
          <Text style={s.backBtnFullText}>Go back</Text>
        </TouchableOpacity>
      </View>
    );
  }

  const status    = refunded ? 'refunded' : booking.booking_status;
  const statusCol = STATUS_COLOR[status] ?? '#888';
  const canRefund = status === 'confirmed' && !refunded && !!user;
  const isMock    = !booking.unit_price; // mock bookings don't have unit_price
  const qrValue   = booking.qr_code_url ?? booking.booking_number;

  return (
    <View style={s.root}>
      {/* Floating header */}
      <Animated.View style={[s.floatingHeader, { backgroundColor: headerBg, paddingTop: insets.top }]}>
        <TouchableOpacity style={s.headerBack} onPress={() => router.back()}>
          <Feather name="arrow-left" size={20} color="#fff" />
        </TouchableOpacity>
        <Text style={s.headerTitle} numberOfLines={1}>{booking.event?.title ?? 'Ticket'}</Text>
        <View style={{ width: 36 }} />
      </Animated.View>

      <Animated.ScrollView
        onScroll={Animated.event([{ nativeEvent: { contentOffset: { y: scrollY } } }], { useNativeDriver: false })}
        scrollEventThrottle={16}
        contentContainerStyle={{ paddingBottom: insets.bottom + 48 }}
        showsVerticalScrollIndicator={false}
      >
        {/* Hero */}
        <Animated.View style={[s.hero, { opacity: heroOpacity }]}>
          {booking.event?.cover_image_url ? (
            <Image source={{ uri: booking.event.cover_image_url }} style={StyleSheet.absoluteFillObject} />
          ) : (
            <View style={[StyleSheet.absoluteFillObject, { backgroundColor: '#1a1a1a' }]} />
          )}
          <LinearGradient
            colors={['transparent', 'rgba(10,10,10,0.85)', '#0a0a0a']}
            style={StyleSheet.absoluteFillObject}
          />
          <View style={[s.heroContent, { paddingTop: insets.top + 56 }]}>
            <View style={[s.statusBadge, { borderColor: `${statusCol}44`, backgroundColor: `${statusCol}18` }]}>
              <View style={[s.statusDot, { backgroundColor: statusCol }]} />
              <Text style={[s.statusText, { color: statusCol }]}>{status.toUpperCase()}</Text>
            </View>
            <Text style={s.heroTitle}>{booking.event?.title}</Text>
            {booking.event?.start_time && (
              <Text style={s.heroDate}>
                {fmt(booking.event.start_time)} · {fmtTime(booking.event.start_time)}
              </Text>
            )}
          </View>
        </Animated.View>

        {/* ─── QR Code ─── */}
        <View style={s.qrSection}>
          <Text style={s.sectionLabel}>ENTRY PASS</Text>
          <View style={s.qrCard}>
            {booking.qr_code_url ? (
              <Image
                source={{ uri: booking.qr_code_url }}
                style={s.qrImage}
                resizeMode="contain"
              />
            ) : (
              <QRPlaceholder value={qrValue} />
            )}
            <Text style={s.qrNumber}>#{booking.booking_number}</Text>
            <Text style={s.qrHint}>
              {status === 'confirmed'
                ? 'Present this at the venue entrance'
                : status === 'refunded'
                ? 'This ticket has been refunded'
                : 'Ticket not yet confirmed'}
            </Text>
          </View>
        </View>

        {/* ─── Event Details ─── */}
        <View style={s.card}>
          <Text style={s.sectionLabel}>EVENT DETAILS</Text>
          {booking.event?.start_time && (
            <InfoRow
              icon={<Ionicons name="calendar-outline" size={15} color="rgba(255,255,255,0.45)" />}
              label="Date & time"
              value={`${fmt(booking.event.start_time)} at ${fmtTime(booking.event.start_time)}`}
            />
          )}
          {booking.event && (
            <InfoRow
              icon={<Ionicons name="location-outline" size={15} color="rgba(255,255,255,0.45)" />}
              label="Venue"
              value={`${booking.event.venue_name}, ${booking.event.city}`}
            />
          )}
          {booking.event?.address && (
            <InfoRow
              icon={<Feather name="map-pin" size={15} color="rgba(255,255,255,0.45)" />}
              label="Address"
              value={booking.event.address}
            />
          )}
        </View>

        {/* ─── Ticket Info ─── */}
        <View style={s.card}>
          <Text style={s.sectionLabel}>TICKET INFO</Text>
          <InfoRow
            icon={<Ionicons name="ticket-outline" size={15} color="rgba(255,255,255,0.45)" />}
            label="Type"
            value={booking.ticket_type_name ?? 'General Admission'}
          />
          <InfoRow
            icon={<Feather name="users" size={15} color="rgba(255,255,255,0.45)" />}
            label="Quantity"
            value={`${booking.ticket_quantity} ticket${booking.ticket_quantity > 1 ? 's' : ''}`}
          />
          <InfoRow
            icon={<Feather name="check-circle" size={15} color="rgba(255,255,255,0.45)" />}
            label="Status"
            value={status.charAt(0).toUpperCase() + status.slice(1)}
            valueColor={statusCol}
          />
          <InfoRow
            icon={<Feather name="clock" size={15} color="rgba(255,255,255,0.45)" />}
            label="Booked on"
            value={fmtDateTime(booking.created_at)}
          />
          {booking.confirmed_at && (
            <InfoRow
              icon={<Feather name="check" size={15} color="rgba(255,255,255,0.45)" />}
              label="Confirmed"
              value={fmtDateTime(booking.confirmed_at)}
            />
          )}
        </View>

        {/* ─── Attendee ─── */}
        {(booking.attendee_first_name || booking.attendee_email) && (
          <View style={s.card}>
            <Text style={s.sectionLabel}>ATTENDEE</Text>
            {(booking.attendee_first_name || booking.attendee_last_name) && (
              <InfoRow
                icon={<Feather name="user" size={15} color="rgba(255,255,255,0.45)" />}
                label="Name"
                value={[booking.attendee_first_name, booking.attendee_last_name].filter(Boolean).join(' ')}
              />
            )}
            {booking.attendee_email && (
              <InfoRow
                icon={<Feather name="mail" size={15} color="rgba(255,255,255,0.45)" />}
                label="Email"
                value={booking.attendee_email}
              />
            )}
          </View>
        )}

        {/* ─── Pricing ─── */}
        <View style={s.card}>
          <Text style={s.sectionLabel}>PAYMENT BREAKDOWN</Text>
          {!isMock && booking.unit_price != null && (
            <PriceLine
              label={`Unit price × ${booking.ticket_quantity}`}
              value={fmtMoney((booking.unit_price ?? 0) * booking.ticket_quantity)}
            />
          )}
          {!isMock && (booking.platform_fee ?? 0) > 0 && (
            <PriceLine label="Service fee" value={fmtMoney(booking.platform_fee ?? 0)} />
          )}
          {!isMock && (booking.discount_amount ?? 0) > 0 && (
            <PriceLine label="Discount" value={`-${fmtMoney(booking.discount_amount ?? 0)}`} accent />
          )}
          <View style={s.priceDivider} />
          <PriceLine label="Total paid" value={fmtMoney(booking.total_amount, booking.currency)} bold />

          {/* Refund info if refunded */}
          {(status === 'refunded' || refunded) && (
            <>
              <View style={s.priceDivider} />
              <View style={s.refundedBanner}>
                <Ionicons name="checkmark-circle" size={15} color="#00e5a0" />
                <Text style={s.refundedText}>
                  {booking.refunded_amount != null
                    ? `Refund of ${fmtMoney(booking.refunded_amount)} is being processed`
                    : 'Refund request submitted — processing in 3–5 business days'}
                </Text>
              </View>
              {booking.refunded_reason && (
                <Text style={s.refundReason}>Reason: {booking.refunded_reason}</Text>
              )}
            </>
          )}
        </View>

        {/* ─── Actions ─── */}
        <View style={s.actions}>
          {canRefund && (
            <TouchableOpacity style={s.refundBtn} onPress={() => setRefundOpen(true)} activeOpacity={0.8}>
              <Feather name="rotate-ccw" size={15} color="#ff4d4d" />
              <Text style={s.refundBtnText}>Request refund</Text>
            </TouchableOpacity>
          )}
        </View>
      </Animated.ScrollView>

      {/* Refund modal */}
      <RefundModal
        visible={refundOpen}
        bookingId={booking.id}
        onClose={() => setRefundOpen(false)}
        onSuccess={() => setRefunded(true)}
      />
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const s = StyleSheet.create({
  root: { flex: 1, backgroundColor: '#0a0a0a' },

  floatingHeader: {
    position: 'absolute', top: 0, left: 0, right: 0, zIndex: 50,
    flexDirection: 'row', alignItems: 'center',
    paddingHorizontal: 16, paddingBottom: 12,
  },
  headerBack: {
    width: 36, height: 36, borderRadius: 10,
    backgroundColor: 'rgba(255,255,255,0.12)',
    alignItems: 'center', justifyContent: 'center',
  },
  headerTitle: {
    flex: 1, textAlign: 'center', fontSize: 15, fontWeight: '700', color: '#fff',
    marginHorizontal: 8,
  },

  hero: { height: 320, position: 'relative' },
  heroContent: { position: 'absolute', bottom: 0, left: 0, right: 0, padding: 24, gap: 8 },
  heroTitle: { fontSize: 28, fontWeight: '800', color: '#fff', letterSpacing: -0.5, lineHeight: 33 },
  heroDate:  { fontSize: 13, color: 'rgba(255,255,255,0.55)', fontWeight: '500' },

  statusBadge: {
    flexDirection: 'row', alignItems: 'center', gap: 6,
    paddingHorizontal: 10, paddingVertical: 5, borderRadius: 20, borderWidth: 1,
    alignSelf: 'flex-start',
  },
  statusDot:  { width: 6, height: 6, borderRadius: 3 },
  statusText: { fontSize: 10, fontWeight: '800', letterSpacing: 1.5 },

  // QR
  qrSection: { alignItems: 'center', paddingHorizontal: 24, paddingTop: 8, paddingBottom: 4 },
  qrCard: {
    width: '100%', backgroundColor: 'rgba(255,255,255,0.05)',
    borderRadius: 20, borderWidth: 1, borderColor: 'rgba(255,255,255,0.08)',
    alignItems: 'center', padding: 28, gap: 16,
  },
  qrImage: { width: SW - 96, height: SW - 96, borderRadius: 8 },
  qrNumber: { fontSize: 17, fontWeight: '800', color: '#fff', letterSpacing: 2 },
  qrHint:   { fontSize: 12, color: 'rgba(255,255,255,0.35)', textAlign: 'center' },

  // Sections
  sectionLabel: {
    fontSize: 10, fontWeight: '800', letterSpacing: 2.5,
    color: 'rgba(255,255,255,0.3)', marginBottom: 14, marginLeft: 2,
  },
  card: {
    marginHorizontal: 16, marginTop: 20,
    backgroundColor: 'rgba(255,255,255,0.04)',
    borderRadius: 16, borderWidth: StyleSheet.hairlineWidth,
    borderColor: 'rgba(255,255,255,0.08)', padding: 18, gap: 14,
  },

  // Info rows
  infoRow: { flexDirection: 'row', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 },
  infoRowLeft: { flexDirection: 'row', alignItems: 'center', gap: 10, flex: 1 },
  infoLabel: { fontSize: 13, color: 'rgba(255,255,255,0.45)', fontWeight: '500' },
  infoValue: { fontSize: 13, color: '#fff', fontWeight: '600', textAlign: 'right', flex: 1.2 },

  // Pricing
  priceLine: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  priceLabel: { fontSize: 13, color: 'rgba(255,255,255,0.5)', fontWeight: '400' },
  priceValue: { fontSize: 13, color: 'rgba(255,255,255,0.7)', fontWeight: '500' },
  priceLabelBold: { fontSize: 15, color: '#fff', fontWeight: '700' },
  priceValueBold: { fontSize: 15, color: '#fff', fontWeight: '800' },
  priceDivider: { height: StyleSheet.hairlineWidth, backgroundColor: 'rgba(255,255,255,0.08)', marginVertical: 4 },

  refundedBanner: {
    flexDirection: 'row', alignItems: 'flex-start', gap: 8,
    backgroundColor: 'rgba(0,229,160,0.08)',
    borderRadius: 10, padding: 12,
  },
  refundedText: { flex: 1, fontSize: 13, color: '#00e5a0', lineHeight: 18 },
  refundReason: { fontSize: 12, color: 'rgba(255,255,255,0.3)', fontStyle: 'italic', marginTop: 4 },

  // Actions
  actions: { marginHorizontal: 16, marginTop: 20, gap: 12 },
  refundBtn: {
    flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 8,
    backgroundColor: 'rgba(255,77,77,0.08)', borderRadius: 14,
    borderWidth: 1, borderColor: 'rgba(255,77,77,0.2)', paddingVertical: 15,
  },
  refundBtnText: { color: '#ff4d4d', fontSize: 14, fontWeight: '700' },

  // Error
  errTitle: { fontSize: 18, fontWeight: '700', color: '#fff' },
  backBtnFull: {
    marginTop: 8, paddingHorizontal: 20, paddingVertical: 10,
    borderRadius: 10, backgroundColor: 'rgba(255,255,255,0.07)',
  },
  backBtnFullText: { color: '#fff', fontSize: 13, fontWeight: '700' },

  // Refund modal
  modalOverlay: {
    flex: 1, backgroundColor: 'rgba(0,0,0,0.7)',
    justifyContent: 'flex-end',
  },
  modalSheet: {
    backgroundColor: '#161616',
    borderTopLeftRadius: 24, borderTopRightRadius: 24,
    padding: 24, paddingBottom: 40, gap: 14,
  },
  modalHandle: {
    width: 40, height: 4, borderRadius: 2,
    backgroundColor: 'rgba(255,255,255,0.15)',
    alignSelf: 'center', marginBottom: 8,
  },
  modalTitle: { fontSize: 20, fontWeight: '800', color: '#fff' },
  modalSub:   { fontSize: 13, color: 'rgba(255,255,255,0.4)', lineHeight: 19 },
  modalFieldLabel: {
    fontSize: 10, fontWeight: '800', letterSpacing: 2,
    color: 'rgba(255,255,255,0.3)',
  },
  modalInput: {
    backgroundColor: 'rgba(255,255,255,0.06)',
    borderWidth: 1, borderColor: 'rgba(255,255,255,0.1)',
    borderRadius: 12, padding: 14,
    color: '#fff', fontSize: 14, lineHeight: 20,
    minHeight: 100,
  },
  modalInputError: { borderColor: 'rgba(255,77,77,0.5)' },
  charCount: { fontSize: 11, color: 'rgba(255,255,255,0.2)', textAlign: 'right', marginTop: -8 },
  modalError: {
    flexDirection: 'row', alignItems: 'flex-start', gap: 7,
    backgroundColor: 'rgba(255,80,80,0.08)',
    borderRadius: 10, padding: 11,
  },
  modalErrorText: { flex: 1, fontSize: 13, color: '#ff8080', lineHeight: 18 },
  modalBtn: {
    backgroundColor: '#ff3c6e', borderRadius: 14,
    paddingVertical: 16, alignItems: 'center',
  },
  modalBtnText: { color: '#fff', fontSize: 15, fontWeight: '800' },
  modalCancel: { alignItems: 'center', paddingVertical: 10 },
  modalCancelText: { color: 'rgba(255,255,255,0.4)', fontSize: 14, fontWeight: '600' },
});