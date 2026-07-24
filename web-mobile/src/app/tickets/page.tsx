'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { MapPin, Calendar, Ticket, AlertCircle } from 'lucide-react';
import { api } from '@/lib/api';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';

// ── Types ─────────────────────────────────────────────────────────────────────
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

// ── Palettes ──────────────────────────────────────────────────────────────────
const PALETTES = [
  { top: '#0d2137', bottom: '#0a3d2e', accent: '#00e5a0' },
  { top: '#1a0a2e', bottom: '#2d0a3d', accent: '#b57bee' },
  { top: '#1a1200', bottom: '#3d2600', accent: '#ffb830' },
  { top: '#001a2e', bottom: '#003d5c', accent: '#29c5f6' },
  { top: '#1a0008', bottom: '#3d0015', accent: '#ff4d6d' },
  { top: '#0a1a00', bottom: '#1a3d00', accent: '#8fff00' },
];

// ── Helpers ───────────────────────────────────────────────────────────────────
const fmt = (iso: string) =>
  new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
const fmtTime = (iso: string) =>
  new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
const isUpcoming = (b: Booking) =>
  !!b.event?.start_time && new Date(b.event.start_time) > new Date();

// A "real" ticket is one the buyer actually holds: booking confirmed/refunded,
// or payment completed/refunded. Pending and failed bookings are NOT shown.
// (Mirrors EventsApp/app/(tabs)/tickets.tsx so web and app agree.)
const isRealTicket = (b: Booking) =>
  b.booking_status === 'confirmed' || b.booking_status === 'refunded' ||
  b.payment_status === 'completed' || b.payment_status === 'refunded';

// Upcoming = future-dated AND still active. Cancelled/refunded bookings drop to
// the Past tab even when the event date is still ahead — a cancelled booking is
// not an upcoming ticket. This is the fix for cancelled events showing under
// "Upcoming"; the app already does exactly this.
const isUpcomingActive = (b: Booking) =>
  isUpcoming(b) && b.booking_status !== 'cancelled' && b.booking_status !== 'refunded';
const daysUntil = (iso: string) =>
  Math.ceil((new Date(iso).getTime() - Date.now()) / 86400000);
const dedup = (list: Booking[]) => {
  const seen = new Set<string>();
  return list.filter((b) => { if (seen.has(b.booking_number)) return false; seen.add(b.booking_number); return true; });
};
const fmtZAR = (n: number) => `R${Math.round(n).toLocaleString('en-ZA')}`;

// ── Ticket Card ───────────────────────────────────────────────────────────────
function TicketCard({ booking, index, onPress }: { booking: Booking; index: number; onPress: () => void }) {
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


  const stackCount = Math.min(booking.ticket_quantity - 1, 2);
  return (
    <div className="relative" style={{ paddingBottom: stackCount * 5 }}>
      {/* Stack shadow cards */}
      {Array.from({ length: stackCount }).map((_, i) => (
        <div
          key={i}
          className="absolute inset-x-0 rounded-[16px]"
          style={{
            bottom: -(i + 1) * 5,
            left: (i + 1) * 4,
            right: (i + 1) * 4,
            height: 40,
            background: `linear-gradient(135deg, ${PALETTES[(index + i + 1) % PALETTES.length].top} 0%, ${PALETTES[(index + i + 1) % PALETTES.length].bottom} 100%)`,
            opacity: 0.45 - i * 0.15,
            borderRadius: 16,
          }}
        />
      ))}
    <button
      onClick={onPress}
      className="w-full text-left rounded-[16px] overflow-hidden relative transition-transform hover:scale-[1.01] active:scale-[0.99] z-10"
    >
      {/* Blurred background image */}
      {booking.event?.cover_image_url && (
        <>
          <img
            src={booking.event.cover_image_url}
            alt=""
            className="absolute inset-0 w-full h-full object-cover"
            style={{ filter: 'blur(3px)', opacity: 0.35 }}
          />
          {/* Re-apply gradient on top of blurred image */}
          <div
            className="absolute inset-0"
            style={{ background: `linear-gradient(135deg, ${pal.top}d8 0%, ${pal.bottom}f0 100%)` }}
          />
        </>
      )}

      <div className="relative z-10 p-5 flex flex-col h-full" style={{ minHeight: 210 }}>
        {/* Location */}
        {booking.event && (
          <div className="flex items-center gap-1.5 mb-2">
            <MapPin size={10} color="rgba(255,255,255,0.70)" />
            <span className="text-[11px] font-medium text-white/70 truncate">
              {booking.event.venue_name}, {booking.event.city}
            </span>
          </div>
        )}

        {/* Title */}
        <div className="flex-1 flex flex-col justify-center">
          <p className="text-xl font-black text-white leading-snug line-clamp-3 mb-2">
            {booking.event?.title ?? 'Event'}
          </p>
          {booking.event?.start_time && (
            <div className="flex items-center gap-1.5">
              <Calendar size={10} color="rgba(255,255,255,0.70)" />
              <span className="text-[11px] text-white/70">
                {fmt(booking.event.start_time)} · {fmtTime(booking.event.start_time)}
              </span>
            </div>
          )}
        </div>

        {/* Bottom row */}
        <div className="flex items-end justify-between mt-4 pt-4 border-t border-white/10">
          <div>
            <p className="text-[9px] font-black tracking-[1.5px] text-white/50 mb-0.5">
              {booking.ticket_type_name ?? 'GENERAL'}
            </p>
            <p className="text-sm font-black" style={{ color: pal.accent }}>
              {booking.ticket_quantity}× {booking.ticket_quantity === 1 ? 'ticket' : 'tickets'}
            </p>
          </div>

          <div>
            <p className="text-[9px] font-black tracking-[1.5px] text-white/50 mb-0.5 text-right">TOTAL</p>
            <p className="text-sm font-black text-white">{fmtZAR(booking.total_amount)}</p>
          </div>

          {/* Status pill */}
          <div
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-full border"
            style={{ borderColor: `${statusColor}55`, backgroundColor: `${statusColor}22` }}
          >
            <div className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: statusColor }} />
            <span className="text-[9px] font-black tracking-[1px]" style={{ color: statusColor }}>
              {statusLabel}
            </span>
          </div>
        </div>
      </div>
    </button>
    </div>
  );
}

// ── Guest lookup ──────────────────────────────────────────────────────────────
function GuestLookup() {
  const router = useRouter();
  const { colors } = useAppTheme();
  const [bookingNumber, setBookingNumber] = useState('');
  const [email,   setEmail]   = useState('');
  const [loading, setLoading] = useState(false);
  const [error,   setError]   = useState<string | null>(null);
  const [result,  setResult]  = useState<Booking | null>(null);

  const lookup = async () => {
    const bn = bookingNumber.trim().toUpperCase();
    const em = email.trim().toLowerCase();
    if (!bn || !em) { setError('Enter both your booking number and email.'); return; }
    setLoading(true); setError(null); setResult(null);
    try {
      const { data } = await api.get<Booking>(
        `/v1/guest/bookings/${encodeURIComponent(bn)}`,
        { params: { email: em } },
      );
      setResult(data);
    } catch (e: any) {
      const status = e?.response?.status;
      setError(
        status === 404 ? 'No booking found. Check your number and email.'
        : status === 429 ? 'Too many attempts. Please wait.'
        : e?.response?.data?.error ?? 'Something went wrong.',
      );
    } finally { setLoading(false); }
  };

  if (result) {
    return (
      <div className="max-w-lg mx-auto px-4 py-8">
        <p className="text-[11px] font-black tracking-[1.5px] mb-4" style={{ color: colors.subText }}>YOUR TICKET</p>
        <TicketCard booking={result} index={0} onPress={() => router.push(`/booking/${result.id}?bn=${encodeURIComponent(result.booking_number)}&email=${encodeURIComponent(email.trim().toLowerCase())}`)} />
        <button onClick={() => setResult(null)} className="mt-6 flex items-center gap-2 text-sm font-bold" style={{ color: colors.subText }}>
          ← Look up another ticket
        </button>
      </div>
    );
  }

  return (
    <div className="max-w-md mx-auto px-6 py-10">
      <div className="text-center mb-8">
        <Ticket size={48} color={colors.iconColor} strokeWidth={1.2} className="mx-auto mb-4" />
        <h2 className="text-2xl font-black mb-2" style={{ color: colors.text }}>Find your ticket</h2>
        <p className="text-sm" style={{ color: colors.subText }}>
          Enter your booking number and email from your confirmation.
        </p>
      </div>

      <div className="space-y-4">
        <div>
          <label className="block text-[10px] font-black tracking-[1.5px] mb-1.5" style={{ color: colors.subText }}>
            BOOKING NUMBER
          </label>
          <input
            value={bookingNumber}
            onChange={(e) => setBookingNumber(e.target.value)}
            placeholder="e.g. BK-A3F7K2"
            className="w-full px-4 py-3 rounded-sm border bg-transparent outline-none text-sm font-medium"
            style={{ borderColor: colors.dividerColor, color: colors.text }}
            onKeyDown={(e) => e.key === 'Enter' && lookup()}
          />
        </div>
        <div>
          <label className="block text-[10px] font-black tracking-[1.5px] mb-1.5" style={{ color: colors.subText }}>
            EMAIL ADDRESS
          </label>
          <input
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            type="email"
            placeholder="you@example.com"
            className="w-full px-4 py-3 rounded-sm border bg-transparent outline-none text-sm font-medium"
            style={{ borderColor: colors.dividerColor, color: colors.text }}
            onKeyDown={(e) => e.key === 'Enter' && lookup()}
          />
        </div>

        {error && (
          <div className="flex items-center gap-2 p-3 rounded-sm" style={{ backgroundColor: 'rgba(255,75,75,0.12)' }}>
            <AlertCircle size={15} color="#ff6b6b" />
            <p className="text-sm" style={{ color: '#ff6b6b' }}>{error}</p>
          </div>
        )}

        <button
          onClick={lookup}
          disabled={loading}
          className="w-full py-3.5 rounded-sm text-sm font-black tracking-wide transition-opacity"
          style={{ backgroundColor: colors.text, color: colors.background, opacity: loading ? 0.6 : 1 }}
        >
          {loading ? 'Searching…' : 'Find Ticket'}
        </button>

        <button
          onClick={() => router.push('/auth/login')}
          className="w-full text-sm text-center"
          style={{ color: colors.subText }}
        >
          Have an account? <span style={{ color: '#ff3c6e', fontWeight: 700 }}>Sign in</span>
        </button>
      </div>
    </div>
  );
}

// ── Tickets list view ─────────────────────────────────────────────────────────
function TicketsView({
  bookings, loading, error, onRetry,
}: {
  bookings: Booking[]; loading: boolean;
  error: string | null; onRetry: () => void;
}) {
  const router = useRouter();
  const { colors } = useAppTheme();
  const [activeTab, setActiveTab] = useState<'upcoming' | 'past'>('upcoming');

  // Only real tickets, and cancelled/refunded never count as "upcoming".
  const all      = dedup(bookings).filter(isRealTicket);
  const upcoming = all.filter(isUpcomingActive);
  const past     = all.filter((b) => !isUpcomingActive(b));

  if (loading) {
    return (
      <div className="flex items-center justify-center py-24">
        <div className="w-10 h-10 border-2 rounded-full animate-spin" style={{ borderColor: `${colors.dividerColor} transparent transparent transparent` }} />
      </div>
    );
  }

  if (error && all.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-24 px-8 text-center">
        <AlertCircle size={40} color="rgba(255,80,80,0.4)" />
        <p className="mt-3 text-lg font-black" style={{ color: colors.text }}>Something went wrong</p>
        <p className="mt-1 text-sm" style={{ color: colors.subText }}>{error}</p>
        <button onClick={onRetry} className="mt-4 border px-5 py-2 rounded-sm text-[11px] font-black tracking-wide" style={{ borderColor: colors.dividerColor, color: colors.text }}>
          Try again
        </button>
      </div>
    );
  }

  if (all.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-24 px-8 text-center">
        <Ticket size={52} color={colors.iconColor} strokeWidth={1.2} />
        <p className="mt-4 text-xl font-black" style={{ color: colors.text }}>No tickets yet</p>
        <p className="mt-1 text-sm" style={{ color: colors.subText }}>
          Events you book will appear here as digital tickets.
        </p>
        <button
          onClick={() => router.push('/')}
          className="mt-6 px-6 py-3 rounded-sm text-[11px] font-black tracking-wide"
          style={{ backgroundColor: '#ff2d55', color: '#fff' }}
        >
          BROWSE EVENTS
        </button>
      </div>
    );
  }

  const visible  = activeTab === 'upcoming' ? upcoming : past;
  const tabEmpty = visible.length === 0;

  return (
    <div>
      {/* Tab bar */}
      <div
        className="flex border-b relative"
        style={{ borderColor: colors.dividerColor }}
      >
        {(['upcoming', 'past'] as const).map((t) => {
          const active = activeTab === t;
          return (
            <button
              key={t}
              onClick={() => setActiveTab(t)}
              className="flex-1 flex items-center justify-center gap-2 py-3.5 text-sm font-bold relative"
              style={{ color: active ? colors.text : colors.subText }}
            >
              {t.charAt(0).toUpperCase() + t.slice(1)}
              {active && (
                <span
                  className="absolute bottom-0 left-0 right-0 h-0.5"
                  style={{ backgroundColor: colors.text }}
                />
              )}
            </button>
          );
        })}
      </div>

      {tabEmpty ? (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <p className="text-4xl mb-4">{activeTab === 'upcoming' ? '📅' : '🕐'}</p>
          <p className="text-lg font-black" style={{ color: colors.text }}>
            {activeTab === 'upcoming' ? 'No upcoming events' : 'No past events'}
          </p>
          <p className="mt-1 text-sm" style={{ color: colors.subText }}>
            {activeTab === 'upcoming' ? 'Your future bookings will show up here.' : "Events you've attended will appear here."}
          </p>
        </div>
      ) : (
        <div className="px-4 py-4 space-y-4 max-w-2xl mx-auto">
          {visible.map((b, i) => (
            <TicketCard
              key={b.id}
              booking={b}
              index={i}
              onPress={() => router.push(`/booking/${b.id}`)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────
export default function TicketsPage() {
  const router = useRouter();
  const { user } = useAuthStore();
  const { colors } = useAppTheme();

  const [bookings,   setBookings]   = useState<Booking[]>([]);
  const [loading,    setLoading]    = useState(false);
  const [error,      setError]      = useState<string | null>(null);

  const fetchBookings = useCallback(async () => {
    if (!user) return;
    setError(null);
    try {
      const { data } = await api.get<{ data: Booking[] }>('/v1/bookings', { params: { limit: 50 } });
      const now = Date.now();
      const sorted = (data.data ?? []).sort((a, b) => {
        const aT = a.event?.start_time ? new Date(a.event.start_time).getTime() : 0;
        const bT = b.event?.start_time ? new Date(b.event.start_time).getTime() : 0;
        const aUp = aT > now, bUp = bT > now;
        if (aUp && !bUp) return -1; if (!aUp && bUp) return 1;
        return aUp ? aT - bT : bT - aT;
      });
      setBookings(sorted);
    } catch (e: any) { setError(e?.response?.data?.error ?? 'Failed to load tickets'); }
  }, [user]);

  const load    = useCallback(async () => { setLoading(true); await fetchBookings(); setLoading(false); }, [fetchBookings]);
  const refresh = useCallback(async () => { await fetchBookings(); }, [fetchBookings]);

  useEffect(() => { load(); }, [user]);

  // Silent background refresh whenever the tab/window regains focus, so a
  // booking made elsewhere shows up without a manual button (the app does the
  // equivalent via useFocusEffect). No spinner — this is invisible.
  useEffect(() => {
    if (!user) return;
    const onFocus = () => { refresh(); };
    window.addEventListener('focus', onFocus);
    return () => window.removeEventListener('focus', onFocus);
  }, [user, refresh]);

  return (
    <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>
      {/* Header */}
      <div
        className="sticky top-0 md:top-16 z-40 flex items-center gap-3 px-5 py-4"
        style={{ backgroundColor: colors.background, borderBottom: `1px solid ${colors.dividerColor}` }}
      >
        <h1 className="text-2xl font-black" style={{ color: colors.text }}>My Tickets</h1>
      </div>

      {/* Not logged in */}
      {!user ? (
        <GuestLookup />
      ) : (
        <TicketsView
          bookings={bookings}
          loading={loading}
          error={error}
          onRetry={load}
        />
      )}
    </div>
  );
}