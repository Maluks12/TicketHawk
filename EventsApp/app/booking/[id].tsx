import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
  View, Text, ScrollView, TouchableOpacity,
  ActivityIndicator, TextInput, Modal, Animated,
  KeyboardAvoidingView, Platform, Alert, StyleSheet,
  Dimensions,
} from 'react-native';
// expo-image, not react-native's Image.
//
// This screen was the last one still on the built-in Image after the rest of
// the app migrated. The two libraries keep SEPARATE caches, so a cover image
// already downloaded and cached by the event screen counted for nothing here —
// every visit re-fetched the full-size original over the network, which is why
// the ticket detail felt slow while the screens either side of it were instant.
// expo-image also has a real disk cache, so it survives an app restart.
import { Image as ExpoImage } from 'expo-image';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { Feather, Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import {
  BookingDetail,
  TicketAssignment,
  getBookingDetail,
  getTicketAssignments,
  assignTicket,
  revokeAssignment,
  api,
} from '../../lib/api';
import { useAuthStore } from '../../store/authStore';
import { syncBookingOffline, cacheBookingDetail, getCachedBookingDetail } from '../../lib/offline';
import { s, sheet, t, setBookingTheme } from '../../styles/booking';
import { useAppTheme } from '../../theme/colors';
import { RotatingQR } from '../../components/RotatingQR';
import {
  getRefundQuote,
  submitRefund,
  fmtRand,
  fmtRate,
  refundWindowLabel,
  daysLeftInBand,
  type RefundQuote,
  type RefundQuoteResponse,
} from '../../lib/refunds';

const { width: SW } = Dimensions.get('window');

// Lets the hero scrim stay light. A shadow tight to the glyphs guarantees
// contrast against whatever pixel happens to sit behind a given letter, which a
// blanket overlay can only do by darkening the entire image.
const HERO_TEXT_SHADOW = {
  textShadowColor:  'rgba(0,0,0,0.55)',
  textShadowOffset: { width: 0, height: 1 },
  textShadowRadius: 6,
} as const;

// ─── Helpers ──────────────────────────────────────────────────────────────────

const fmt       = (iso: string) => new Date(iso).toLocaleDateString('en-ZA', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });
const fmtTime   = (iso: string) => new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
// lib/money — was Math.round, which overstated any amount carrying cents.
const fmtMoney  = (n: number)   => fmtRand(n);
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
// Shows the buyer what they will actually be paid BEFORE they commit, then
// submits. The amount is never computed here — see lib/refunds.ts for why.

// ── Free bookings are cancellations, not refunds ─────────────────────────────
//
// A R0 ticket still holds a seat, so the action has to exist — the organiser's
// headcount is wrong until the seat goes back. But every word of refund
// language is false for it: there is no money to return, "R0 back" is
// meaningless, and the sliding-scale line ("25% of your ticket price") is noise
// computed from zero.
//
// So the SAME endpoint and the same seat-release path (seats_released_at) are
// reused, and only the wording changes. This is deliberately a presentation
// switch rather than a second code path: a separate cancel flow would be a
// second place for seat release to drift out of sync with refunds.
const RefundModal = ({ visible, bookingId, guest, quote, totalPaid, onClose, onSuccess }: {
  visible: boolean;
  bookingId: string;
  guest?: { bookingNumber: string; email: string };
  quote: RefundQuote | null;
  totalPaid: number;
  onClose: () => void;
  onSuccess: (amount: number) => void;
}) => {
  const isFree = totalPaid <= 0;
  const [reason, setReason]   = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError]     = useState('');

  // Clear the form each time the sheet opens, so a cancelled attempt doesn't
  // leave a stale reason (or a stale error) sitting there next time.
  useEffect(() => {
    if (visible) { setReason(''); setError(''); }
  }, [visible]);

  const submit = async () => {
    // A free booking still needs releasing even though there is no refund quote
    // to be "eligible" — gating on the quote would leave the button permanently
    // dead for exactly the bookings this sheet now exists to cancel.
    if (!isFree && !quote?.eligible) { setError('This booking is no longer refundable.'); return; }
    if (reason.trim().length < 10) { setError('Please provide at least 10 characters.'); return; }
    setLoading(true); setError('');
    try {
      await submitRefund(bookingId, reason.trim(), guest);
      onSuccess(quote?.refund_amount ?? 0);
      onClose();
    } catch (e: any) {
      // A 400 here means the server re-priced the refund and it no longer
      // qualifies — almost always because the buyer sat on the sheet across a
      // band boundary. Surface the server's reason verbatim rather than the
      // now-stale figure this sheet was showing.
      // The server's message is refund-shaped ("Failed to process payment
      // refund"), which is meaningless on a booking that cost nothing. For a
      // free booking the server error is suppressed in favour of language that
      // matches what the user was actually trying to do.
      setError(
        isFree
          ? 'Couldn\u2019t cancel your booking. Please try again.'
          : e?.response?.data?.error ??
            e?.response?.data?.quote?.reason ??
            'Failed to submit. Please try again.',
      );
    } finally { setLoading(false); }
  };

  const bandDays  = quote ? daysLeftInBand(quote.days_until_event) : null;
  const nextRate  = quote && quote.rate >= 1 ? '75%'
                  : quote && quote.rate >= 0.75 ? '50%'
                  : quote && quote.rate >= 0.5  ? '25%'
                  : '0%';

  return (
    <Modal visible={visible} animationType="slide" transparent presentationStyle="overFullScreen" onRequestClose={onClose}>
      {/* ── Keyboard handling ────────────────────────────────────────────────
          The sheet used to be a fixed-height View, so once the keyboard opened
          over the reason field there was no way to reach the submit button:
          the content could not scroll and the sheet could not move. The user
          could type and then had no way to finish.

          Three things fix it together, and all three are needed:
            • maxHeight caps the sheet so it can never exceed the space left
              above the keyboard,
            • the ScrollView lets the content move within that space,
            • keyboardShouldPersistTaps="handled" means the FIRST tap on the
              submit button submits, instead of being swallowed as a
              dismiss-the-keyboard tap — which on its own looks like a dead
              button and is exactly as frustrating as not scrolling.

          behavior differs by platform: iOS pads the container, Android resizes
          the window itself and double-handling it makes the sheet jump. */}
      <KeyboardAvoidingView
        style={{ flex: 1 }}
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      >
        <TouchableOpacity style={{ flex: 1 }} activeOpacity={1} onPress={onClose} />
        <View style={[sheet.modalSheet, { maxHeight: '85%', paddingBottom: 0 }]}>
          <View className={s.modalHandle} />
          <ScrollView
            keyboardShouldPersistTaps="handled"
            showsVerticalScrollIndicator={false}
            contentContainerStyle={{ gap: 14, paddingBottom: 40 }}
          >
          <Text className={s.modalTitle}>{isFree ? 'Cancel booking' : 'Request a refund'}</Text>

          {/* ── What you get back ──────────────────────────────────────────
              Suppressed entirely for a free booking. Every row in it is a
              refund figure derived from zero — "You'll get back R0",
              "Refunded at 25% of the ticket price R0", and a 7-day refund
              policy that has nothing to do with releasing a free seat. Showing
              a card whose every line reads R0 is worse than showing none. */}
          {isFree ? null : quote ? (
            <View style={sheet.quoteCard}>
              <View style={sheet.quoteHeadRow}>
                <Text style={sheet.quoteHeadLabel}>You&apos;ll get back</Text>
                <Text style={sheet.quoteHeadValue}>{fmtRand(quote.refund_amount)}</Text>
              </View>
              <View style={sheet.quoteDivider} />

              <View style={sheet.quoteRow}>
                <Text style={sheet.quoteLabel}>Total paid</Text>
                <Text style={sheet.quoteValue}>{fmtRand(totalPaid)}</Text>
              </View>
              <View style={sheet.quoteRow}>
                <Text style={sheet.quoteLabel}>
                  Refunded at {fmtRate(quote.rate)} of the ticket price
                </Text>
                <Text style={sheet.quoteValue}>{fmtRand(quote.refund_amount)}</Text>
              </View>
              {quote.forfeited > 0 && (
                <View style={sheet.quoteRow}>
                  <Text style={sheet.quoteLabel}>Not refunded</Text>
                  <Text style={sheet.quoteValue}>−{fmtRand(quote.forfeited)}</Text>
                </View>
              )}
              {quote.service_fee_kept > 0 && (
                <View style={sheet.quoteRow}>
                  <Text style={sheet.quoteLabel}>Service fee (non-refundable)</Text>
                  <Text style={sheet.quoteValue}>−{fmtRand(quote.service_fee_kept)}</Text>
                </View>
              )}

              <Text style={sheet.quoteFoot}>
                Based on {refundWindowLabel(quote.days_until_event).toLowerCase()}.
                {bandDays != null
                  ? ` Refunds drop to ${nextRate} in ${bandDays} day${bandDays === 1 ? '' : 's'}.`
                  : ''}
                {' '}Refunds are not available within 7 days of the event.
              </Text>
            </View>
          ) : (
            <View style={sheet.quoteLoading}>
              <ActivityIndicator color={t.spinner} />
            </View>
          )}

          {/* The trailing "original payment method within 3-5 business days"
              is part of the PAID sentence and has to live inside that branch —
              left outside, it appended itself to the cancellation copy and
              produced "...so someone else can take it.original payment method
              within 3-5 business days." */}
          <Text className={s.modalSub}>
            {isFree
              ? 'Tell us why you\u2019re cancelling. Your seat is released straight away so someone else can take it.'
              : 'Tell us why you\u2019d like a refund. Once approved, the money is returned to your original payment method within 3\u20135 business days.'}
          </Text>
          <Text className={s.modalFieldLabel}>
            {isFree ? 'REASON FOR CANCELLING' : 'REASON FOR REFUND'}
          </Text>
          <TextInput
            style={[sheet.modalInput, error ? sheet.modalInputError : undefined]}
            value={reason} onChangeText={v => { setReason(v); setError(''); }}
            placeholder="e.g. I can no longer attend this event due to..."
            placeholderTextColor={t.placeholder}
            multiline numberOfLines={4} maxLength={500} textAlignVertical="top"
          />
          <Text className={s.charCount}>{reason.length}/500</Text>
          {error ? (
            <View style={sheet.modalError}>
              <Ionicons name="alert-circle-outline" size={14} color="#ff6b6b" />
              <Text style={sheet.modalErrorText}>{error}</Text>
            </View>
          ) : null}
          <TouchableOpacity
            style={[sheet.modalBtn, (loading || (!isFree && !quote?.eligible)) && { opacity: 0.6 }]}
            onPress={submit}
            disabled={loading || (!isFree && !quote?.eligible)}
            activeOpacity={0.85}
          >
            {loading
              ? <ActivityIndicator color="#fff" size="small" />
              : <Text style={{ color: '#fff', fontSize: 15, fontWeight: '800' }}>
                  {isFree
                    ? 'Cancel booking'
                    : quote ? `Refund ${fmtRand(quote.refund_amount)}` : 'Submit refund request'}
                </Text>}
          </TouchableOpacity>
          <TouchableOpacity className={s.modalCancel} onPress={onClose}>
            <Text className={s.modalCancelText}>Cancel</Text>
          </TouchableOpacity>
          </ScrollView>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
};

// ─── Assign Sheet ─────────────────────────────────────────────────────────────
// Bottom sheet allowing the buyer to assign an unassigned slot to someone else.
// The backend sends that person a claim-link email on success.

interface AssignSheetProps {
  visible: boolean;
  bookingId: string;
  assignment: TicketAssignment | null;
  totalTickets: number;
  onClose: () => void;
  onSuccess: () => void;
}

const AssignSheet = ({ visible, bookingId, assignment, totalTickets, onClose, onSuccess }: AssignSheetProps) => {
  const insets = useSafeAreaInsets();
  const [name, setName]       = useState('');
  const [email, setEmail]     = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError]     = useState('');

  // Reset fields whenever the sheet opens for a (possibly different) slot.
  useEffect(() => {
    if (visible) { setName(''); setEmail(''); setError(''); }
  }, [visible, assignment?.id]);

  const submit = async () => {
    if (!assignment) return;
    if (!name.trim())  { setError('Please enter the recipient\'s name.');  return; }
    if (!email.trim()) { setError('Please enter the recipient\'s email.'); return; }
    // Basic email shape check — the backend validates properly.
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) {
      setError('Please enter a valid email address.');
      return;
    }
    setLoading(true); setError('');
    try {
      await assignTicket(bookingId, assignment.id, name.trim(), email.trim().toLowerCase());
      onSuccess();
      onClose();
    } catch (e: any) {
      const msg = e?.response?.data?.error ?? e?.response?.data?.details ?? 'Failed to send invite. Please try again.';
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal visible={visible} animationType="slide" transparent presentationStyle="overFullScreen">
      <KeyboardAvoidingView
        style={{ flex: 1 }}
        behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
      >
        <TouchableOpacity style={{ flex: 1 }} activeOpacity={1} onPress={onClose} />
        <View style={[sheet.assignSheet, { paddingBottom: insets.bottom + 16 }]}>
          {/* Handle */}
          <View style={sheet.sheetHandle} />

          <Text style={sheet.sheetTitle}>Send ticket to someone</Text>
          {assignment && totalTickets > 1 && (
            <Text style={sheet.sheetSub}>
              Ticket {assignment.slot_number} of {totalTickets} · They'll receive a claim link by email.
            </Text>
          )}
          {!assignment || totalTickets === 1 ? (
            <Text style={sheet.sheetSub}>
              They'll receive a claim link by email and can pick it up at the door.
            </Text>
          ) : null}

          {/* Name */}
          <Text style={sheet.fieldLabel}>RECIPIENT NAME</Text>
          <TextInput
            style={[sheet.sheetInput, error && !email ? sheet.sheetInputError : undefined]}
            value={name}
            onChangeText={v => { setName(v); setError(''); }}
            placeholder="e.g. Jane Smith"
            placeholderTextColor={t.placeholder}
            autoCapitalize="words"
            autoCorrect={false}
            returnKeyType="next"
          />

          {/* Email */}
          <Text style={[sheet.fieldLabel, { marginTop: 12 }]}>RECIPIENT EMAIL</Text>
          <TextInput
            style={[sheet.sheetInput, error && email ? sheet.sheetInputError : undefined]}
            value={email}
            onChangeText={v => { setEmail(v); setError(''); }}
            placeholder="jane@example.com"
            placeholderTextColor={t.placeholder}
            autoCapitalize="none"
            keyboardType="email-address"
            autoCorrect={false}
            returnKeyType="send"
            onSubmitEditing={submit}
          />

          {/* Error */}
          {!!error && (
            <View style={sheet.sheetErrorBox}>
              <Ionicons name="alert-circle-outline" size={14} color="#ff6b6b" />
              <Text style={sheet.sheetErrorText}>{error}</Text>
            </View>
          )}

          {/* Submit */}
          <TouchableOpacity
            style={[sheet.sendBtn, loading && { opacity: 0.6 }]}
            onPress={submit}
            disabled={loading}
            activeOpacity={0.85}
          >
            {loading
              ? <ActivityIndicator color={t.onAccent} size="small" />
              : (
                <>
                  <Feather name="send" size={15} color={t.onAccent} />
                  <Text style={sheet.sendBtnText}>Send claim link</Text>
                </>
              )
            }
          </TouchableOpacity>

          <TouchableOpacity style={sheet.cancelBtn} onPress={onClose}>
            <Text style={sheet.cancelBtnText}>Cancel</Text>
          </TouchableOpacity>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function BookingDetailScreen() {
  const { isDark } = useAppTheme();
  setBookingTheme(isDark);
  const insets = useSafeAreaInsets();
  const router = useRouter();
  // `bn` is the parameter name used by the confirmation email's link
  // (…/booking/<id>?bn=<number>&email=<addr>, see events-db email_service.go),
  // `bookingNumber` is what the Tickets tab passes when it pushes this route
  // internally. Both mean the same thing and both have to be accepted — the
  // screen only read `bookingNumber`, so a guest arriving from their own
  // confirmation email had no way to authenticate and got "Booking not found".
  const params = useLocalSearchParams<{
    id: string; bookingNumber?: string; bn?: string; email?: string;
  }>();
  const { id, email } = params;
  const bookingNumber = params.bookingNumber ?? params.bn;

  // A guest reaches this screen from the Tickets-tab lookup or from the link in
  // their confirmation email. With no auth token their booking is addressed by
  // booking number + email via the /guest/ routes.
  const guest = bookingNumber && email ? { bookingNumber, email } : undefined;
  const { user, authReady } = useAuthStore();
  const [booking, setBooking]               = useState<BookingDetail | null>(null);
  const [assignments, setAssignments]       = useState<TicketAssignment[]>([]);
  const [loading, setLoading]               = useState(true);
  const [refundOpen, setRefundOpen]         = useState(false);
  const [refunded, setRefunded]             = useState(false);
  const [refundedAmount, setRefundedAmount] = useState<number | null>(null);
  const [refundQuote, setRefundQuote]       = useState<RefundQuoteResponse | null>(null);
  const [quoteLoading, setQuoteLoading]     = useState(false);
  const [assignTarget, setAssignTarget]     = useState<TicketAssignment | null>(null);
  const [revoking, setRevoking]             = useState<string | null>(null); // assignmentId being revoked
  const [poolRemaining, setPoolRemaining]   = useState<number | null>(null); // live remaining from the pool QR
  const scrollY = useRef(new Animated.Value(0)).current;

  const refreshAssignments = useCallback(async () => {
    if (!id) return;
    try {
      const data = await getTicketAssignments(id as string);
      setAssignments(data);
    } catch {}
  }, [id]);

  const handleRevoke = async (assignmentId: string) => {
    Alert.alert(
      'Revoke invite',
      'This will cancel the pending invite and make the slot available to assign again.',
      [
        { text: 'Cancel', style: 'cancel' },
        {
          text: 'Revoke',
          style: 'destructive',
          onPress: async () => {
            setRevoking(assignmentId);
            try {
              await revokeAssignment(id as string, assignmentId);
              await refreshAssignments();
            } catch (e: any) {
              Alert.alert('Error', e?.response?.data?.error ?? 'Failed to revoke. Please try again.');
            } finally {
              setRevoking(null);
            }
          },
        },
      ],
    );
  };

  useEffect(() => {
    const load = async () => {
      // WAIT FOR THE SESSION TO SETTLE.
      //
      // This screen is a universal-link landing pad: tapping "View My Tickets"
      // in the confirmation email cold-starts the app straight onto this route.
      // At that moment loadUser() is still in flight, so `user` is null — and
      // the old code took that to mean "not signed in", set loading=false and
      // rendered "Booking not found". `user` was not in the dependency array
      // either, so when the session finished restoring a moment later this
      // effect never re-ran. The ticket was permanently unreachable from the
      // email even though opening it from the Tickets tab worked fine.
      //
      // Guests skip the wait: they authenticate with booking number + email,
      // not with a session.
      if (!guest && !authReady) return;
      if (!user && !guest) { setLoading(false); return; }
      try {
        // Retry transient failures (esp. 429 rate-limit / network blips) with
        // backoff so a momentary limit doesn't render as "Booking not found".
        // A genuine 404 bails immediately. Guests use the /guest/ routes.
        let bookingDetail: any = null;
        for (let attempt = 0; attempt < 4; attempt++) {
          try {
            bookingDetail = guest
              ? (await api.get(`/v1/guest/bookings/${encodeURIComponent(guest.bookingNumber)}`,
                  { params: { email: guest.email } })).data
              : await getBookingDetail(id as string);
            break;
          } catch (e: any) {
            if (e?.response?.status === 404) throw e;
            if (attempt === 3) throw e;
            await new Promise(r => setTimeout(r, 500 * (attempt + 1)));
          }
        }
        const assignments = guest
          ? await api.get(`/v1/guest/bookings/${encodeURIComponent(guest.bookingNumber)}/assignments`,
              { params: { email: guest.email } })
              .then(r => (r.data?.data ?? r.data ?? []) as TicketAssignment[])
              .catch(() => [])
          : await getTicketAssignments(id as string).catch(() => []);
        setBooking(bookingDetail);
        setAssignments(assignments);
        // Cache the screen data itself so this ticket still renders offline.
        cacheBookingDetail(bookingDetail, assignments);

        // Cache offline entry tokens the moment the ticket is viewed online, so
        // it still scans at the gate with no signal. Best-effort; never blocks.
        if (bookingDetail?.booking_status === 'confirmed') {
          const realId = bookingDetail.id ?? (id as string);
          syncBookingOffline(
            realId,
            guest ? { bookingNumber: guest.bookingNumber, email: guest.email } : undefined,
          );
        }
      } catch {
        // Offline / network failure — fall back to the cached copy so the
        // ticket (and its offline QR) still renders with no signal.
        const cached = await getCachedBookingDetail(
          guest ? { bookingNumber: guest.bookingNumber } : { bookingId: id as string },
        );
        if (cached) {
          setBooking(cached.booking);
          setAssignments((cached.assignments ?? []) as TicketAssignment[]);
        }
      } finally { setLoading(false); }
    };
    load();
  }, [id, bookingNumber, email, authReady, user?.id]);

  // ── Refund quote ──────────────────────────────────────────────────────────
  // Fetched separately from the booking, and deliberately AFTER it: the quote is
  // additive (it decides whether a refund button exists), so it must never be
  // able to delay or fail the ticket itself. Only asked for when a refund could
  // conceivably apply — a confirmed booking for an event that hasn't started.
  // Mock bookings are skipped since there is no server row behind them.
  const eventStarted = booking?.event?.start_time
    ? new Date(booking.event.start_time).getTime() <= Date.now()
    : false;
  const refundPossible =
    !!booking &&
    !refunded &&
    booking.booking_status === 'confirmed' &&
    !eventStarted;

  // Nothing was paid, so there is nothing to refund — but the seat is still
  // held and still needs releasing. Drives the wording only; the action and the
  // endpoint behind it are identical.
  const isFreeBooking = (booking?.total_amount ?? 0) <= 0;

  useEffect(() => {
    if (!refundPossible || !booking) { setRefundQuote(null); return; }
    let cancelled = false;
    setQuoteLoading(true);
    getRefundQuote(booking.id ?? (id as string), guest)
      .then(q => { if (!cancelled) setRefundQuote(q); })
      .finally(() => { if (!cancelled) setQuoteLoading(false); });
    return () => { cancelled = true; };
  }, [refundPossible, booking?.id, bookingNumber, email]);

  const heroOpacity = scrollY.interpolate({ inputRange: [0, 180], outputRange: [1, 0.4], extrapolate: 'clamp' });
  // Themed: this was pinned to rgba(10,10,10,…), so scrolling in light mode slid
  // a black bar down over a white page. Built from the theme's page colour, and
  // the title/glyph fade from white (over the hero art) to the theme's text
  // colour (over the solid header) so they stay legible through the transition.
  const headerBg = scrollY.interpolate({
    inputRange:  [140, 200],
    outputRange: [`${t.headerScrim}0)`, `${t.headerScrim}1)`],
    extrapolate: 'clamp',
  });
  const headerFg = scrollY.interpolate({
    inputRange:  [140, 200],
    outputRange: ['#ffffff', isDark ? '#ffffff' : '#0a0a0a'],
    extrapolate: 'clamp',
  });

  // Still restoring the session (cold start from the email link) — a spinner,
  // never an error. `loading` starts true and the load effect returns early
  // without touching it until auth settles, so this covers that window too.
  if (loading || (!guest && !authReady)) return (
    <View className={s.root} style={{ alignItems: 'center', justifyContent: 'center' }}>
      <ActivityIndicator color={t.spinner} />
    </View>
  );

  // Session settled and there genuinely is nobody signed in. The ticket exists,
  // we just can't prove it belongs to whoever is holding the phone — so ask,
  // rather than claiming the booking doesn't exist. The login screen is a modal
  // that pops back here on success, and the load effect re-runs on `user`.
  if (!booking && !user && !guest) return (
    <View className={s.root} style={{ alignItems: 'center', justifyContent: 'center', gap: 12, paddingHorizontal: 40 }}>
      <Ionicons name="lock-closed-outline" size={40} color={t.iconFaint} />
      <Text className={s.errTitle}>Sign in to view this ticket</Text>
      <TouchableOpacity className={s.backBtnFull} onPress={() => router.push('/auth/login' as any)}>
        <Text className={s.backBtnFullText}>Sign in</Text>
      </TouchableOpacity>
      <TouchableOpacity onPress={() => router.replace('/(tabs)' as any)}>
        <Text className={s.qrHint}>Go to home</Text>
      </TouchableOpacity>
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

  // Determine which slots to show QR codes for.
  // Claimed slots have a real QR; others show a pending placeholder.
  // Fall back to booking-level qr_code_url for older bookings that were
  // not migrated to per-assignment QR codes.
  const hasAssignments = assignments.length > 0;
  const claimedSlots   = assignments.filter(a => a.status === 'claimed');
  const showQRs        = status === 'confirmed' || status === 'pending';

  // Pool model: unassigned AND not-yet-admitted slots are the buyer's assignable
  // group. A slot that's already been scanned at the gate stays unassigned but
  // can't be assigned, so it's excluded here (qr_scanned_at comes from the API).
  const poolSlots      = assignments.filter(a => a.status === 'unassigned' && !(a as any).qr_scanned_at);
  const assignedSlots  = assignments.filter(a => a.status === 'pending' || a.status === 'claimed');
  const poolCount      = poolSlots.length;
  const firstUnassigned = poolSlots[0] ?? null;

  return (
    <View className={s.root}>
      {/* Floating header */}
      <Animated.View className={s.floatingHeader} style={{ backgroundColor: headerBg, paddingTop: insets.top }}>
        <TouchableOpacity className={s.headerBack} onPress={() => router.back()}>
          <Feather name="arrow-left" size={20} color="#fff" />
        </TouchableOpacity>
        {/* Animated.Text so the title can cross-fade from white (over the hero
            artwork) to the theme's text colour (over the solid header). The
            className supplies size/weight; only the colour is animated. */}
        <Animated.Text
          className={s.headerTitle}
          style={{ color: headerFg }}
          numberOfLines={1}
        >
          {booking.event?.title ?? 'Ticket'}
        </Animated.Text>
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
            ? <ExpoImage
                source={{ uri: booking.event.cover_image_url }}
                style={StyleSheet.absoluteFillObject}
                contentFit="cover"
                // memory-disk: reuse whatever the event and purchase screens
                // already pulled down, and keep it across restarts.
                cachePolicy="memory-disk"
                transition={180}
                recyclingKey={booking.event.cover_image_url}
              />
            : <View className={s.heroBg} />
          }
          {/* The scrim exists to keep the status badge, title and date legible —
              nothing more. The old three-stop ramp
              ['transparent','rgba(10,10,10,0.85)','#0a0a0a'] reached 85% black by
              the MIDPOINT of a 320px hero and solid black at the bottom, so it
              was covering the top half of the artwork on its way to covering the
              rest. On a photo-led screen that is most of the reason to have a
              hero at all.
              Now: a whisper at the very top (so the back button reads against a
              bright sky), untouched through the middle where the image actually
              lives, and the weight pushed down into the last third where the text
              sits. Same legibility, roughly half the coverage — and the text
              carries its own shadow (heroTitle/heroDate below) so the ramp no
              longer has to do that job on its own. */}
          <LinearGradient
            colors={[
              'rgba(0,0,0,0.28)',
              'transparent',
              'rgba(0,0,0,0.30)',
              'rgba(0,0,0,0.78)',
            ]}
            locations={[0, 0.30, 0.66, 1]}
            style={StyleSheet.absoluteFillObject}
          />
          <View className={s.heroContent} style={{ paddingTop: insets.top + 56 }}>
            <View className={s.statusBadge} style={{ borderColor: `${statusCol}44`, backgroundColor: `${statusCol}18` }}>
              <View className={s.statusDot} style={{ backgroundColor: statusCol }} />
              <Text className={s.statusText} style={{ color: statusCol }}>{status.toUpperCase()}</Text>
            </View>
            <Text className={s.heroTitle} style={HERO_TEXT_SHADOW}>{booking.event?.title}</Text>
            {booking.event?.start_time && (
              <Text className={s.heroDate} style={HERO_TEXT_SHADOW}>
                {fmt(booking.event.start_time)} · {fmtTime(booking.event.start_time)}
              </Text>
            )}
          </View>
        </Animated.View>

        {/* Entry passes — group pool pass + assigned tickets */}
        <View className={s.qrSection}>
          <Text className={s.sectionLabel}>
            {booking.ticket_quantity > 1
              ? `ENTRY PASSES (${booking.ticket_quantity} tickets)`
              : 'ENTRY PASS'}
          </Text>

          {status === 'refunded' ? (
            <View style={[sheet.qrCard, { opacity: 0.4 }]}>
              <Ionicons name="close-circle-outline" size={64} color={t.chevron} />
              <Text className={s.qrNumber}>#{booking.booking_number}</Text>
              <Text className={s.qrHint}>
                {isFreeBooking ? 'This booking has been cancelled' : 'This ticket has been refunded'}
              </Text>
            </View>
          ) : hasAssignments ? (
            <>
              {/* Group pool pass — one QR covering everyone still with the buyer */}
              {poolCount > 0 && showQRs && (
                <View style={[sheet.qrCard, { marginBottom: 12 }]}>
                  {booking.ticket_quantity > 1 && (
                    <View style={sheet.slotBadge}>
                      <Text style={sheet.slotBadgeText}>GROUP PASS</Text>
                    </View>
                  )}
                  <RotatingQR
                    size={SW - 96}
                    pool={{ bookingId: guest ? undefined : (id as string) }}
                    guest={guest}
                    onRemaining={setPoolRemaining}
                  />
                  <Text style={sheet.assigneeName}>
                    {(() => {
                      const n = poolRemaining != null ? poolRemaining : poolCount;
                      return `Covers your group · ${n} admission${n === 1 ? '' : 's'}`;
                    })()}
                  </Text>
                  <Text className={s.qrNumber}>#{booking.booking_number}</Text>
                  <Text className={s.qrHint}>Live QR — one scan admits your group at the entrance</Text>
                </View>
              )}

              {/* Pool fully assigned away */}
              {poolCount === 0 && assignedSlots.length > 0 && showQRs && (
                <View style={[sheet.qrCard, { marginBottom: 12 }]}>
                  <View style={sheet.pendingQR}>
                    <Ionicons name="people-outline" size={48} color={t.iconFaint} />
                    <Text style={sheet.pendingText}>You&apos;ve assigned all your tickets</Text>
                  </View>
                  <Text className={s.qrHint}>Everyone below has their own pass</Text>
                </View>
              )}

              {/* Assigned tickets list */}
              {assignedSlots.length > 0 && (
                <>
                  <Text style={sheet.assignedHeader}>ASSIGNED ({assignedSlots.length})</Text>
                  {assignedSlots.map((slot) => {
                    const isClaimed  = slot.status === 'claimed';
                    const isPending  = slot.status === 'pending';
                    const isRevoking = revoking === slot.id;
                    return (
                      <View key={slot.id} style={sheet.assignedRow}>
                        <View
                          style={[
                            sheet.assignedIcon,
                            { backgroundColor: isClaimed ? 'rgba(0,229,160,0.12)' : 'rgba(255,184,48,0.14)' },
                          ]}
                        >
                          <Ionicons
                            name={isClaimed ? 'checkmark' : 'mail-outline'}
                            size={16}
                            color={isClaimed ? '#00e5a0' : '#ffb830'}
                          />
                        </View>
                        <View style={{ flex: 1 }}>
                          <Text style={sheet.assignedName} numberOfLines={1}>
                            {slot.assignee_name ?? 'Guest'}
                          </Text>
                          <Text
                            style={[sheet.assignedStatus, { color: isClaimed ? '#00e5a0' : '#c79249' }]}
                            numberOfLines={1}
                          >
                            {isClaimed
                              ? 'Claimed · has their ticket'
                              : `Invite sent · ${slot.assignee_email ?? 'awaiting claim'}`}
                          </Text>
                        </View>
                        {isPending && status === 'confirmed' && !guest && (
                          <TouchableOpacity onPress={() => handleRevoke(slot.id)} disabled={isRevoking} hitSlop={8}>
                            {isRevoking
                              ? <ActivityIndicator size="small" color="#ff6b6b" />
                              : <Feather name="x" size={18} color={t.icon} />}
                          </TouchableOpacity>
                        )}
                      </View>
                    );
                  })}
                </>
              )}

              {/* Assign a ticket — peel one off the pool (authed buyer only) */}
              {poolCount > 0 && status === 'confirmed' && !guest && firstUnassigned && (
                <TouchableOpacity
                  style={sheet.assignRow}
                  onPress={() => setAssignTarget(firstUnassigned)}
                  activeOpacity={0.8}
                >
                  <View style={sheet.assignRowIcon}>
                    <Feather name="send" size={15} color={t.iconStrong} />
                  </View>
                  <View style={{ flex: 1 }}>
                    <Text style={sheet.assignRowTitle}>Assign a ticket</Text>
                    <Text style={sheet.assignRowSub}>Send one to someone arriving separately</Text>
                  </View>
                  <Feather name="chevron-right" size={18} color={t.chevron} />
                </TouchableOpacity>
              )}
            </>
          ) : (
            /* Fallback for older bookings without per-assignment rows */
            <View style={sheet.qrCard}>
              {booking.qr_code_url ? (
                <ExpoImage
                  source={{ uri: booking.qr_code_url }}
                  style={sheet.qrImage}
                  contentFit="contain"
                  // A QR must never render blurred or half-decoded, so no
                  // transition and no placeholder here — it appears when it is
                  // readable, or not at all.
                  cachePolicy="memory-disk"
                  transition={0}
                />
              ) : (
                <View style={sheet.pendingQR}>
                  <Ionicons name="time-outline" size={48} color={t.iconFaint} />
                  <Text style={sheet.pendingText}>
                    {status === 'confirmed' ? 'QR code is being generated…' : 'Ticket not yet confirmed'}
                  </Text>
                </View>
              )}
              <Text className={s.qrNumber}>#{booking.booking_number}</Text>
              <Text className={s.qrHint}>
                {status === 'confirmed'
                  ? 'Present this at the venue entrance'
                  : 'Complete payment to activate your ticket'}
              </Text>
            </View>
          )}
        </View>

        {/* Event Details */}
        <View style={sheet.card}>
          <Text className={s.sectionLabel}>EVENT DETAILS</Text>
          {booking.event?.start_time && (
            <InfoRow icon={<Ionicons name="calendar-outline" size={15} color={t.icon} />}
              label="Date & time" value={`${fmt(booking.event.start_time)} at ${fmtTime(booking.event.start_time)}`} />
          )}
          {booking.event && (
            <InfoRow icon={<Ionicons name="location-outline" size={15} color={t.icon} />}
              label="Venue" value={`${booking.event.venue_name}, ${booking.event.city}`} />
          )}
          {booking.event?.address && (
            <InfoRow icon={<Feather name="map-pin" size={15} color={t.icon} />}
              label="Address" value={booking.event.address} />
          )}
        </View>

        {/* Ticket Info */}
        <View style={sheet.card}>
          <Text className={s.sectionLabel}>TICKET INFO</Text>
          <InfoRow icon={<Ionicons name="ticket-outline" size={15} color={t.icon} />}
            label="Type" value={booking.ticket_type_name ?? 'General Admission'} />
          <InfoRow icon={<Feather name="users" size={15} color={t.icon} />}
            label="Quantity" value={`${booking.ticket_quantity} ticket${booking.ticket_quantity > 1 ? 's' : ''}`} />
          <InfoRow icon={<Feather name="check-circle" size={15} color={t.icon} />}
            label="Status" value={status.charAt(0).toUpperCase() + status.slice(1)} valueColor={statusCol} />
          <InfoRow icon={<Feather name="clock" size={15} color={t.icon} />}
            label="Booked on" value={fmtDT(booking.created_at)} />
          {booking.confirmed_at && (
            <InfoRow icon={<Feather name="check" size={15} color={t.icon} />}
              label="Confirmed" value={fmtDT(booking.confirmed_at)} />
          )}
        </View>

        {/* Attendee */}
        {(booking.attendee_first_name || booking.attendee_email) && (
          <View style={sheet.card}>
            <Text className={s.sectionLabel}>ATTENDEE</Text>
            {(booking.attendee_first_name || booking.attendee_last_name) && (
              <InfoRow icon={<Feather name="user" size={15} color={t.icon} />}
                label="Name" value={[booking.attendee_first_name, booking.attendee_last_name].filter(Boolean).join(' ')} />
            )}
            {booking.attendee_email && (
              <InfoRow icon={<Feather name="mail" size={15} color={t.icon} />}
                label="Email" value={booking.attendee_email} />
            )}
          </View>
        )}

        {/* Payment */}
        <View style={sheet.card}>
          <Text className={s.sectionLabel}>PAYMENT BREAKDOWN</Text>
          {booking.unit_price != null && (
            <PriceLine label={`Unit price × ${booking.ticket_quantity}`} value={fmtMoney((booking.unit_price ?? 0) * booking.ticket_quantity)} />
          )}
          {(booking.platform_fee ?? 0) > 0 && (
            <PriceLine label="Service fee" value={fmtMoney(booking.platform_fee ?? 0)} />
          )}
          {(booking.discount_amount ?? 0) > 0 && (
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
                  {/* refundedAmount is what THIS session's request was quoted at;
                      booking.refunded_amount is what the server recorded on an
                      earlier one. Prefer the fresh figure so the buyer sees the
                      exact amount they just agreed to, not a stale field that the
                      cached booking may not carry at all. */}
                  {isFreeBooking
                    ? 'Booking cancelled — your seat has been released'
                    : (refundedAmount ?? booking.refunded_amount) != null
                    ? `Refund of ${fmtMoney((refundedAmount ?? booking.refunded_amount) as number)} is being processed — 3–5 business days`
                    : 'Refund request submitted — processing in 3–5 business days'}
                </Text>
              </View>
              {booking.refunded_reason && <Text className={s.refundReason}>Reason: {booking.refunded_reason}</Text>}
            </>
          )}
        </View>

        {/* ── Refund ────────────────────────────────────────────────────────
            Restored, and now driven entirely by the server's quote rather than
            being unconditionally present.

            The rule the button follows (events-db/internal/models/refund_policy.go):
              30+ days  → 100% of the ticket price
              21–29     →  75%
              14–20     →  50%
               7–13     →  25%
              under 7   →   0%  — NO BUTTON AT ALL

            Inside 7 days the action is not disabled or greyed out, it is gone,
            replaced by a line explaining why. A visible-but-dead refund button is
            worse than no button: buyers tap it, get a 400 back, and open a support
            ticket. The eligibility decision comes from quote.eligible, so the app
            can never disagree with what the refund endpoint will actually do.

            The amount is on the button face on purpose. "Request a refund" alone
            invites the buyer to assume they get everything back, which after day 30
            is never true — putting the real figure up front is the difference
            between an informed cancellation and a chargeback. */}
        {refundPossible && (
          <View style={sheet.refundWrap}>
            {quoteLoading && !refundQuote ? (
              <View style={sheet.quoteLoading}>
                <ActivityIndicator color={t.spinner} size="small" />
              </View>
            ) : refundQuote?.quote?.eligible ? (
              <>
                <TouchableOpacity
                  className={s.refundBtn}
                  onPress={() => setRefundOpen(true)}
                  activeOpacity={0.85}
                >
                  <Ionicons
                    name={isFreeBooking ? 'close-circle-outline' : 'arrow-undo-outline'}
                    size={16}
                    color="#ff4d4d"
                  />
                  <Text className={s.refundBtnText}>
                    {isFreeBooking
                      ? 'Cancel booking'
                      : `Request a refund · ${fmtRand(refundQuote.quote!.refund_amount)} back`}
                  </Text>
                </TouchableOpacity>
                {/* The sliding-scale line is refund pricing. On a free ticket
                    every number in it is zero, so it says nothing true — the
                    seat still gets released, which is the part that matters. */}
                <Text className={s.refundBtnSub}>
                  {isFreeBooking
                    ? 'Releases your seat so someone else can book it.'
                    : `${refundWindowLabel(refundQuote.quote!.days_until_event)} · ${fmtRate(refundQuote.quote!.rate)} of your ticket price${(() => {
                        const left = daysLeftInBand(refundQuote.quote!.days_until_event);
                        return left != null
                          ? ` · rate drops in ${left} day${left === 1 ? '' : 's'}`
                          : '';
                      })()}`}
                </Text>
              </>
            ) : refundQuote?.quote ? (
              <View style={sheet.refundNote}>
                <Ionicons name="information-circle-outline" size={17} color={t.icon} />
                <View style={{ flex: 1 }}>
                  <Text className={s.refundNoteTitle}>Refunds are closed for this booking</Text>
                  <Text className={s.refundNoteBody}>
                    {refundQuote.quote!.reason ??
                      'Refunds are not available within 7 days of the event.'}
                  </Text>
                </View>
              </View>
            ) : null}
          </View>
        )}
      </Animated.ScrollView>

      <RefundModal
        visible={refundOpen}
        bookingId={booking.id}
        guest={guest}
        quote={refundQuote?.quote ?? null}
        totalPaid={refundQuote?.total_paid ?? booking.total_amount}
        onClose={() => setRefundOpen(false)}
        onSuccess={(amount) => { setRefunded(true); setRefundedAmount(amount); }}
      />

      <AssignSheet
        visible={!!assignTarget}
        bookingId={booking.id}
        assignment={assignTarget}
        totalTickets={booking.ticket_quantity}
        onClose={() => setAssignTarget(null)}
        onSuccess={refreshAssignments}
      />
    </View>
  );
}
// ─── Pool model styles ────────────────────────────────────────────────────────
// Moved to styles/booking.ts (sheet.assignRow, sheet.assignedRow, …) so they
// follow the theme. They were hardcoded #141414 panels with white text here,
// which is why the "Assign a ticket" row and the assigned-ticket list stayed
// dark on a light screen.