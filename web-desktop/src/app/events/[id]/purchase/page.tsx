'use client';

import { useState, useEffect, useCallback, useRef, use } from 'react';
import { BadgeCheck, ShieldCheck, CreditCard, Landmark, Check, AlertCircle, X } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { api, getTicketTypes, type TicketType, type Event } from '@/lib/api';
import { startBookingPayment, startGuestPayment, preopenGatewayPopup, discardPendingPopup, type PaymentMethod } from '@/lib/payments';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';
import SeatingMap, { detectVenueType, MapLegend } from '@/components/seatingMap';

// ── Helpers ───────────────────────────────────────────────────────────────────
function fmtPrice(p: number) {
  if (p === 0) return 'Free';
  return `R${Math.round(p).toLocaleString('en-ZA')}`;
}
function fmtDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
}
function fmtTime(iso: string) {
  return new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
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

// ── Page ──────────────────────────────────────────────────────────────────────
export default function EventPurchasePage({ params }: { params: Promise<{ id: string }> }) {
  const router = useRouter();
  const { user } = useAuthStore();
  const { colors, isDark } = useAppTheme();
  const { id: eventId } = use(params);

  const [event,          setEvent]          = useState<Event | null>(null);
  const [ticketTypes,    setTicketTypes]    = useState<TicketType[]>([]);
  const [loading,        setLoading]        = useState(true);
  const [selectedId,     setSelectedId]     = useState<string | null>(null);
  const [quantity,       setQuantity]       = useState(1);
  const [step,              setStep]        = useState<'select' | 'map' | 'summary'>('select');
  const [adding,         setAdding]         = useState(false);
  const [paymentMethod,  setPaymentMethod]  = useState<PaymentMethod>('card');
  const [paySheetOpen,   setPaySheetOpen]   = useState(false);
  const [contactOpen,    setContactOpen]    = useState(false);
  const [guestName,      setGuestName]      = useState('');
  const [guestSurname,   setGuestSurname]   = useState('');
  const [guestEmail,     setGuestEmail]     = useState('');
  const [error,          setError]          = useState('');

  // Errors are surfaced as a floating toast (see below) so they're visible
  // no matter where the person is scrolled to, or whether a sheet is open.
  // Auto-dismiss after a few seconds; a fresh error resets the timer.
  useEffect(() => {
    if (!error) return;
    const t = setTimeout(() => setError(''), 5000);
    return () => clearTimeout(t);
  }, [error]);

  // ── Gate: redirect to details if not navigated from there ─────────────
  // Use a ref so this check only runs once, even if the component re-renders
  // (e.g. due to React.use() Suspense cycles unwrapping params).
  const gateChecked = useRef(false);
  useEffect(() => {
    if (gateChecked.current) return;
    gateChecked.current = true;
    const key = `event_detail_visited_${eventId}`;
    const hasVisited = sessionStorage.getItem(key);
    if (!hasVisited) {
      router.replace(`/events/${eventId}`);
      return;
    }
    sessionStorage.removeItem(key);
  }, [eventId, router]);

    useEffect(() => {
    const load = async () => {
      try {
        const { data: evRes } = await api.get(`/v1/events/${eventId}`);
        const ev: Event = evRes.data ?? evRes;
        setEvent(ev);

        const types = await getTicketTypes(eventId);
        if (types.length === 0 && ev?.price != null) {
          const fallback: TicketType = {
            id: 'ga-fallback', event_id: eventId, name: 'GENERAL ADMISSION',
            description: null, price: ev.price, capacity: null, tickets_sold: 0,
            reserved_count: 0, sort_order: 0, is_visible: true,
            sale_starts_at: null, sale_ends_at: null, created_at: '', updated_at: '',
          };
          setTicketTypes([fallback]);
          setSelectedId('ga-fallback');
        } else {
          setTicketTypes(types);
          const first = types.find((t) => !isSoldOut(t));
          if (first) setSelectedId(first.id);
        }
      } catch {
        setError('Failed to load event.');
      } finally {
        setLoading(false);
      }
    };
    load();
  }, [eventId]);

  const selectedType = ticketTypes.find((t) => t.id === selectedId) ?? null;
  const totalAmount  = selectedType ? selectedType.price * quantity : 0;
  // Mirror the backend fee model (events-db repository.CalculateFees): a R10
  // buyer service fee PER TICKET, capped at 20% of the ticket's face value,
  // rounded to cents. The old flat 5% estimate showed a total that didn't
  // match what Paystack actually charged.
  const platformFee  = totalAmount > 0
    ? Math.round(Math.min(10, 0.2 * (selectedType?.price ?? 0)) * quantity * 100) / 100
    : 0;
  const grandTotal   = totalAmount + platformFee;
  // NOTE: Seating map UI is temporarily disabled (feature-flagged off) rather
  // than removed — flip SEATING_MAP_ENABLED back to true to restore it.
  // All the original detection/rendering logic below is left untouched for
  // future reference.
  const SEATING_MAP_ENABLED = false;
  const venueType = (() => {
  if (!SEATING_MAP_ENABLED) return 'none' as const;
  if (!event) return 'none' as const;
  const override = (event as any).venue_type as string | undefined;
  if (override && override !== 'auto') return override as 'stadium' | 'auditorium' | 'none';
  return detectVenueType(event.category ?? '', event.venue_name);
})();

  const handleBuyNow = async () => {
    if (!selectedType || !event) return;
    if (!user) {
      if (!guestName.trim() || !guestSurname.trim()) { setError('Please enter your name and surname.'); return; }
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(guestEmail.trim())) { setError('Please enter a valid email.'); return; }
    }
    setAdding(true);
    setError('');
    preopenGatewayPopup();
    try {
      const nameParts  = [guestName.trim(), guestSurname.trim()];
      const firstName  = user ? String((user as any).first_name ?? '') : (nameParts[0] ?? '');
      const lastRaw    = user ? String((user as any).last_name  ?? '') : nameParts.slice(1).join(' ');
      const lastName   = lastRaw.trim() || firstName;
      const email      = user ? String((user as any).email ?? '') : guestEmail.trim();
      const endpoint   = user ? '/v1/bookings' : '/v1/guest/bookings';

      const payload = {
        event_id:            event.id,
        ticket_type_id:      selectedId !== 'ga-fallback' ? selectedId : undefined,
        ticket_quantity:     quantity,
        attendee_first_name: firstName,
        attendee_last_name:  lastName,
        attendee_email:      email,
      };

      // channel=web tags the payment so the gateway's return page is a
      // self-closing web page instead of the mobile-app deep link.
      const { data: booking } = await api.post(endpoint, payload, { params: { channel: 'web' } });
      const bookingId = (booking as any).booking_id ?? booking.id;

      // Free ticket — booking is already confirmed server-side.
      if ((booking as any).total_amount === 0) {
        discardPendingPopup();
        router.replace('/tickets');
        return;
      }

      // Resolve the gateway checkout URL.
      // • Card (default): the booking creation already initialised a Paystack
      //   card checkout and returned its URL in client_token. Reuse it to skip
      //   a round-trip (same optimisation as the mobile app).
      // • Instant EFT: call startBookingPayment to get a
      //   method-specific hosted-checkout URL.
      // • Guests: no method-aware start route — always use the card URL from
      //   booking creation.
      let authorizationUrl: string;
      let usedMethod: PaymentMethod = 'card';

      if (paymentMethod === 'card' && (booking as any).client_token) {
        // Card: reuse the checkout URL already returned by booking creation.
        authorizationUrl = (booking as any).client_token;
        usedMethod = 'card';
      } else if (paymentMethod === 'instant_eft') {
        // Instant EFT: initialise a method-specific checkout.
        if (user) {
          const started = await startBookingPayment(bookingId, paymentMethod);
          authorizationUrl = started.authorization_url;
        } else {
          const bn = (booking as any).booking_number;
          const started = await startGuestPayment(bn, guestEmail.trim(), paymentMethod);
          authorizationUrl = started.authorization_url;
        }
        usedMethod = paymentMethod;
      } else {
        authorizationUrl = (booking as any).client_token ?? '';
        usedMethod = 'card';
      }

      if (!authorizationUrl) {
        discardPendingPopup();
        setError('Payment unavailable. Please try again.');
        return;
      }

      const params = new URLSearchParams({
        url:      encodeURIComponent(authorizationUrl),
        method:   usedMethod,
        amount:   String((booking as any).total_amount ?? 0),
        currency: (booking as any).currency ?? 'ZAR',
      });
      // Pass booking number + email so the payment page can poll via guest routes.
      if (!user) {
        params.set('bookingNumber', String((booking as any).booking_number ?? ''));
        params.set('email', email);
      }
      router.push(`/payment/${bookingId}?${params.toString()}`);
    } catch (e: any) {
      discardPendingPopup();
      setError(e?.response?.data?.error ?? 'Failed to create booking. Please try again.');
    } finally {
      setAdding(false);
    }
  };

  const bg     = isDark ? '#0a0a0a' : '#f5f5f7';
  const cardBg = isDark ? '#111'    : '#fff';
  const border = isDark ? '#222'    : '#e5e5e5';

  if (loading) return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ width: 32, height: 32, border: '3px solid #ff2d55', borderTopColor: 'transparent', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  );

  if (!event) return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 12, padding: 40 }}>
      <span style={{ fontSize: 40 }}>⚠️</span>
      <h2 style={{ color: colors.text, margin: 0 }}>Event not found</h2>
      <button onClick={() => router.back()} style={{ background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 12, padding: '12px 24px', fontWeight: 800, cursor: 'pointer' }}>Go back</button>
    </div>
  );

const steps = venueType !== 'none'
  ? (['select', 'map', 'summary'] as const)
  : (['select', 'summary'] as const);
const stepIndex = (steps as readonly string[]).indexOf(step);

return (
  <div style={{ minHeight: '100vh', background: bg }}>
    <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>

    {/* Header */}
    <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '20px 16px 12px', borderBottom: `1px solid ${border}`, background: bg, position: 'sticky', top: 0, zIndex: 40 }}>
      <button
        onClick={() => {
          if (step === 'summary') setStep(venueType !== 'none' ? 'map' : 'select');
          else if (step === 'map') setStep('select');
          else router.back();
        }}
        style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: 22, color: colors.text, padding: 0 }}>←</button>
      <h1 style={{ flex: 1, margin: 0, fontSize: 18, fontWeight: 800, color: colors.text }}>
        {step === 'select' ? 'Select Tickets' : step === 'map' ? 'Choose Seats' : 'Checkout'}
      </h1>
      <div style={{ display: 'flex', gap: 5 }}>
        {steps.map((_, i) => (
          <div key={i} style={{ width: i === stepIndex ? 18 : 6, height: 6, borderRadius: 3, background: i === stepIndex ? '#ff2d55' : (isDark ? '#444' : '#ccc'), transition: 'width 0.2s' }} />
        ))}
      </div>
    </div>

    <div style={{ maxWidth: 640, margin: '0 auto', padding: '0 16px 100px' }}>

      {/* Event title strip — unchanged */}
      <div style={{ padding: '16px 0 12px', borderBottom: `1px solid ${border}` }}>
        <p style={{ margin: 0, fontSize: 20, fontWeight: 800, color: colors.text }}>{event.title}</p>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: colors.subText }}>
          {fmtDate(event.start_time)} · {fmtTime(event.start_time)}
        </p>
        {event.venue_name && (
          <p style={{ margin: '2px 0 0', fontSize: 13, color: colors.subText }}>
            {event.venue_name}{event.city ? `, ${event.city}` : ''}
          </p>
        )}
      </div>

        {/* ══ STEP: Select ticket type ══════════════════════════════════ */}
        {step === 'select' && (
          <div style={{ paddingTop: 16 }}>
            <p style={{ fontSize: 11, fontWeight: 800, letterSpacing: 1.4, color: colors.subText, margin: '0 0 12px' }}>AVAILABLE TICKETS</p>

            {ticketTypes.length === 0 ? (
              <div style={{ textAlign: 'center', padding: '48px 0', color: colors.subText }}>
                <p style={{ fontSize: 40, margin: '0 0 8px' }}>🎫</p>
                <p style={{ margin: 0 }}>No tickets currently on sale</p>
              </div>
            ) : (
              ticketTypes.map((tt, idx) => {
                const sold      = isSoldOut(tt);
                const selected  = tt.id === selectedId;
                const isPremium = idx === ticketTypes.length - 1 && ticketTypes.length > 1;
                return (
                  <div
                    key={tt.id}
                    onClick={() => !sold && setSelectedId(tt.id)}
                    style={{
                      display: 'flex', alignItems: 'center', gap: 14, padding: 16, marginBottom: 10,
                      background: cardBg, border: `${selected ? 2 : 1}px solid ${selected ? '#ff2d55' : border}`,
                      borderRadius: 14, cursor: sold ? 'default' : 'pointer', opacity: sold ? 0.5 : 1,
                      transition: 'border-color 0.15s',
                    }}
                  >
                    {/* Radio */}
                    <div style={{ width: 20, height: 20, borderRadius: '50%', border: `2px solid ${selected ? '#ff2d55' : (isDark ? '#555' : '#ccc')}`, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                      {selected && <div style={{ width: 10, height: 10, borderRadius: '50%', background: '#ff2d55' }} />}
                    </div>

                    {/* Info */}
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 2 }}>
                        <p style={{ margin: 0, fontSize: 14, fontWeight: 700, color: colors.text }}>{tt.name}</p>
                        {isPremium && !sold && <span style={{ fontSize: 10, fontWeight: 800, color: '#ff9500', background: 'rgba(255,149,0,0.12)', padding: '2px 7px', borderRadius: 4 }}>⭐ VIP</span>}
                      </div>
                      <p style={{ margin: 0, fontSize: 12, color: colors.subText }}>
                        {sold ? 'Sold out' : (tt.description ?? availLabel(tt))}
                      </p>
                    </div>

                    {/* Price */}
                    <div style={{ textAlign: 'right' }}>
                      <p style={{ margin: 0, fontSize: 17, fontWeight: 800, color: sold ? colors.subText : colors.text }}>{fmtPrice(tt.price)}</p>
                      <p style={{ margin: 0, fontSize: 11, color: colors.subText }}>incl. fees</p>
                    </div>
                  </div>
                );
              })
            )}

            {/* Quantity picker */}
            {selectedType && !isSoldOut(selectedType) && (() => {
              const maxPer = typeof (event as any)?.max_tickets_per_user === 'number'
                ? (event as any).max_tickets_per_user : Infinity;
              if (maxPer === 1) return null;
              const maxVisible = Math.min(isFinite(maxPer) ? maxPer : 6, 6);
              return (
                <div style={{ marginTop: 8 }}>
                  <p style={{ fontSize: 11, fontWeight: 800, letterSpacing: 1.2, color: colors.subText, margin: '0 0 10px' }}>HOW MANY TICKETS?</p>
                  <div style={{ display: 'flex', gap: 8 }}>
                    {Array.from({ length: maxVisible }, (_, i) => i + 1).map((n) => (
                      <button
                        key={n}
                        onClick={() => setQuantity(n)}
                        style={{
                          width: 44, height: 44, borderRadius: 10, border: `${quantity === n ? 2 : 1}px solid ${quantity === n ? '#ff2d55' : border}`,
                          background: quantity === n ? 'rgba(255,45,85,0.1)' : cardBg,
                          color: quantity === n ? '#ff2d55' : colors.text,
                          fontWeight: 800, fontSize: 15, cursor: 'pointer',
                        }}
                      >
                        {n === maxVisible && !isFinite(maxPer) ? '6+' : n}
                      </button>
                    ))}
                  </div>
                </div>
              );
            })()}
          </div>
        )}

              {/* ══ STEP: Seating Map ══ */}
      {step === 'map' && venueType !== 'none' && (
        <div style={{ paddingTop: 16 }}>
          <p style={{ fontSize: 11, fontWeight: 800, letterSpacing: 1.4, color: colors.subText, margin: '0 0 8px' }}>CHOOSE YOUR SECTION</p>
          <p style={{ fontSize: 13, color: colors.subText, margin: '0 0 16px' }}>Tap a section to select your tier</p>
          <div style={{ marginBottom: 16 }}>
            <MapLegend ticketTypes={ticketTypes} selectedTypeId={selectedId} />
          </div>
          <div style={{ display: 'flex', justifyContent: 'center', overflowX: 'auto' }}>
            <SeatingMap
              venueType={venueType}
              ticketTypes={ticketTypes}
              selectedTypeId={selectedId}
              onSelectType={setSelectedId}
            />
          </div>
          {selectedType && (
            <div style={{ marginTop: 16, background: cardBg, border: `1px solid ${border}`, borderRadius: 14, padding: '14px 16px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <p style={{ margin: 0, fontSize: 14, fontWeight: 700, color: colors.text }}>{selectedType.name.toUpperCase()}</p>
                <p style={{ margin: '2px 0 0', fontSize: 12, color: colors.subText }}>{quantity} ticket{quantity !== 1 ? 's' : ''} · {selectedType.price === 0 ? 'Free' : `R${Math.round(selectedType.price).toLocaleString('en-ZA')}`} each</p>
              </div>
              <p style={{ margin: 0, fontSize: 17, fontWeight: 800, color: colors.text }}>
                {totalAmount === 0 ? 'Free' : `R${Math.round(totalAmount).toLocaleString('en-ZA')}`}
              </p>
            </div>
          )}
        </div>
      )}

        {/* ══ STEP: Checkout summary ════════════════════════════════════ */}
        {step === 'summary' && selectedType && (
          <div style={{ paddingTop: 16, display: 'flex', flexDirection: 'column', gap: 10 }}>

            {/* Contact row */}
            <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 14, overflow: 'hidden' }}>
              <div
                onClick={() => !user && setContactOpen(true)}
                style={{ display: 'flex', alignItems: 'center', padding: '14px 16px', cursor: !user ? 'pointer' : 'default', borderBottom: `1px solid ${border}` }}
              >
                <p style={{ margin: 0, fontSize: 13, color: colors.subText, width: 100 }}>Contact</p>
                <div style={{ flex: 1 }}>
                  {!user && !guestName && !guestEmail ? (
                    <p style={{ margin: 0, fontSize: 13, color: '#ff2d55', fontWeight: 600 }}>Add contact info ›</p>
                  ) : (
                    <p style={{ margin: 0, fontSize: 13, color: colors.text, fontWeight: 600 }}>
                      {user ? (`${(user as any).first_name ?? ''} ${(user as any).last_name ?? ''}`.trim() || (user as any).email) : (guestName || guestEmail)}
                    </p>
                  )}
                </div>
              </div>

              <div
                onClick={() => setPaySheetOpen(true)}
                style={{ display: 'flex', alignItems: 'center', padding: '14px 16px', cursor: 'pointer' }}
              >
                <p style={{ margin: 0, fontSize: 13, color: colors.subText, width: 100 }}>Payment</p>
                <p style={{ margin: 0, flex: 1, fontSize: 13, color: colors.text, fontWeight: 600 }}>
                  {paymentMethod === 'instant_eft' ? 'Instant EFT' : 'Credit / Debit Card'} ›
                </p>
              </div>
            </div>

            {/* Order details */}
            <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 14, padding: 16 }}>
              <p style={{ margin: '0 0 12px', fontSize: 11, fontWeight: 800, letterSpacing: 1.2, color: colors.subText }}>ORDER DETAILS</p>
              {[
                ['Ticket type', selectedType.name],
                ['Quantity', `${quantity} ticket${quantity > 1 ? 's' : ''}`],
                ['Delivery', 'Mobile ticket · Access in app'],
              ].map(([label, val]) => (
                <div key={label} style={{ display: 'flex', justifyContent: 'space-between', padding: '8px 0', borderBottom: `1px solid ${border}` }}>
                  <p style={{ margin: 0, fontSize: 13, color: colors.subText }}>{label}</p>
                  <p style={{ margin: 0, fontSize: 13, fontWeight: 600, color: colors.text }}>{val}</p>
                </div>
              ))}
            </div>

            {/* Trust badges */}
            <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 14, padding: 16, display: 'flex', flexDirection: 'column', gap: 10 }}>
              {[
                { Icon: BadgeCheck, title: 'Official Ticket Marketplace', body: 'Tickets are reviewed and verified by the venue.' },
                { Icon: ShieldCheck, title: 'Every ticket protected',      body: 'Secure payment processing.' },
              ].map(({ Icon, title, body }) => (
                <div key={title} style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
                  <Icon size={18} color="#2ecc71" style={{ flexShrink: 0, marginTop: 1 }} />
                  <div>
                    <p style={{ margin: 0, fontSize: 13, fontWeight: 700, color: colors.text }}>{title}</p>
                    <p style={{ margin: 0, fontSize: 12, color: colors.subText }}>{body}</p>
                  </div>
                </div>
              ))}
            </div>

            {/* Price breakdown */}
            <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 14, padding: 16 }}>
              <p style={{ margin: '0 0 8px', fontSize: 11, fontWeight: 800, letterSpacing: 1.2, color: colors.subText }}>PRICE BREAKDOWN</p>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0' }}>
                <p style={{ margin: 0, fontSize: 14, color: colors.text }}>{selectedType.name} × {quantity}</p>
                <p style={{ margin: 0, fontSize: 14, color: colors.text }}>{fmtPrice(totalAmount)}</p>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0' }}>
                <p style={{ margin: 0, fontSize: 14, color: colors.subText }}>Service Fee</p>
                <p style={{ margin: 0, fontSize: 14, color: colors.subText }}>{fmtPrice(platformFee)}</p>
              </div>
              <div style={{ height: 1, background: border, margin: '8px 0' }} />
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0' }}>
                <p style={{ margin: 0, fontSize: 16, fontWeight: 800, color: colors.text }}>Total Amount</p>
                <p style={{ margin: 0, fontSize: 16, fontWeight: 800, color: colors.text }}>{fmtPrice(grandTotal)}</p>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* ── Error toast ── fixed to the top of the viewport so it's visible
           no matter where the person has scrolled to, and it sits above the
           contact/payment sheets (z-index 200 > 100) instead of being hidden
           behind them. */}
      {error && (
        <div
          style={{
            position: 'fixed', top: 16, left: 16, right: 16, zIndex: 200,
            maxWidth: 520, margin: '0 auto',
            display: 'flex', alignItems: 'flex-start', gap: 10,
            background: '#2a0e14', border: '1px solid #ff3c6e',
            borderRadius: 12, padding: '12px 14px',
            boxShadow: '0 8px 24px rgba(0,0,0,0.35)',
          }}
          role="alert"
        >
          <AlertCircle size={18} color="#ff3c6e" style={{ flexShrink: 0, marginTop: 1 }} />
          <p style={{ flex: 1, margin: 0, fontSize: 13, fontWeight: 600, color: '#fff', lineHeight: 1.4 }}>{error}</p>
          <button
            onClick={() => setError('')}
            aria-label="Dismiss"
            style={{ background: 'none', border: 'none', padding: 2, cursor: 'pointer', flexShrink: 0, lineHeight: 0 }}
          >
            <X size={16} color="#ffb3c2" />
          </button>
        </div>
      )}

      {/* CTA bar */}
    {selectedType && !isSoldOut(selectedType) && (
      <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0, background: bg, borderTop: `1px solid ${border}`, padding: '14px 16px 28px', display: 'flex', alignItems: 'center', gap: 14, maxWidth: 640, margin: '0 auto' }}>
        <div>
          <p style={{ margin: 0, fontSize: 11, color: colors.subText }}>Total</p>
          <p style={{ margin: 0, fontSize: 18, fontWeight: 800, color: colors.text }}>
            {(step === 'summary' ? grandTotal : totalAmount) === 0 ? 'Free' : `R${Math.round(step === 'summary' ? grandTotal : totalAmount).toLocaleString('en-ZA')}`}
          </p>
        </div>
        <button
          onClick={() => {
            if (step === 'select') {
              if (!selectedId) { setError('Please choose a ticket type to continue.'); return; }
              setStep(venueType !== 'none' ? 'map' : 'summary');
            } else if (step === 'map') {
              setStep('summary');
            } else {
              handleBuyNow();
            }
          }}
          disabled={adding}
          style={{ flex: 1, background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 14, padding: '14px', fontSize: 15, fontWeight: 800, cursor: 'pointer', opacity: adding ? 0.6 : 1 }}
        >
          {adding ? 'Processing…' : step === 'select' ? (venueType !== 'none' ? 'Choose Seats' : 'Review Order') : step === 'map' ? 'Review Order' : 'Pay Now'}
        </button>
      </div>
    )}

      {/* ── Guest contact sheet ──────────────────────────────────────── */}
      {contactOpen && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', zIndex: 100, display: 'flex', alignItems: 'flex-end', justifyContent: 'center' }} onClick={() => setContactOpen(false)}>
          <div onClick={(e) => e.stopPropagation()} style={{ background: isDark ? '#1a1a1a' : '#fff', borderRadius: '20px 20px 0 0', padding: '20px 20px 40px', width: '100%', maxWidth: 520 }}>
            <h3 style={{ margin: '0 0 16px', color: colors.text }}>Contact info</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <input
                placeholder="Name"
                value={guestName}
                onChange={(e) => setGuestName(e.target.value)}
                style={{ background: isDark ? '#111' : '#f5f5f5', color: colors.text, border: `1px solid ${border}`, borderRadius: 10, padding: '12px 14px', fontSize: 14, outline: 'none', width: '100%', boxSizing: 'border-box' }}
              />
              <input
                placeholder="Surname"
                value={guestSurname}
                onChange={(e) => setGuestSurname(e.target.value)}
                style={{ background: isDark ? '#111' : '#f5f5f5', color: colors.text, border: `1px solid ${border}`, borderRadius: 10, padding: '12px 14px', fontSize: 14, outline: 'none', width: '100%', boxSizing: 'border-box' }}
              />
              <input
                placeholder="Email address"
                type="email"
                value={guestEmail}
                onChange={(e) => setGuestEmail(e.target.value)}
                style={{ background: isDark ? '#111' : '#f5f5f5', color: colors.text, border: `1px solid ${border}`, borderRadius: 10, padding: '12px 14px', fontSize: 14, outline: 'none', width: '100%', boxSizing: 'border-box' }}
              />
            </div>
            <p style={{ margin: '12px 0', fontSize: 12, color: colors.subText }}>Your email will be used for ticket delivery and purchase confirmation.</p>
            <button
              onClick={() => {
                if (!guestName.trim() || !guestSurname.trim()) { setError('Please enter your name and surname.'); return; }
                if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(guestEmail.trim())) { setError('Please enter a valid email.'); return; }
                setContactOpen(false);
              }}
              style={{ width: '100%', background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 12, padding: '13px', fontSize: 15, fontWeight: 800, cursor: 'pointer' }}
            >
              Done
            </button>
          </div>
        </div>
      )}

      {/* ── Payment method sheet ──────────────────────────────────────── */}
      {paySheetOpen && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', zIndex: 100, display: 'flex', alignItems: 'flex-end', justifyContent: 'center' }} onClick={() => setPaySheetOpen(false)}>
          <div onClick={(e) => e.stopPropagation()} style={{ background: isDark ? '#1a1a1a' : '#fff', borderRadius: '20px 20px 0 0', padding: '20px 20px 40px', width: '100%', maxWidth: 520 }}>
            <h3 style={{ margin: '0 0 4px', color: colors.text }}>Payment Method</h3>
            {([
              { id: 'card',        Icon: CreditCard, label: 'Credit / Debit Card' },
              { id: 'instant_eft', Icon: Landmark,   label: 'Instant EFT' },
            ] as const).map(({ id, Icon, label }) => (
              <div
                key={id}
                onClick={() => { setPaymentMethod(id); setPaySheetOpen(false); }}
                style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '14px 0', borderTop: `1px solid ${border}`, cursor: 'pointer' }}
              >
                <Icon size={22} color={colors.subText} />
                <p style={{ flex: 1, margin: 0, fontSize: 15, fontWeight: 500, color: colors.text }}>{label}</p>
                {paymentMethod === id && <Check size={18} color="#ff2d55" strokeWidth={3} />}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}