'use client';

import { useState, useEffect, useCallback, useRef, use } from 'react';

import { enabledPaymentMethods, defaultPaymentMethod, type PaymentMethodId } from '@/lib/paymentMethods';
import { BadgeCheck, ShieldCheck, CreditCard, Landmark, Check, X } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { api, getTicketTypes, type TicketType, type Event } from '@/lib/api';
import { startBookingPayment, startGuestPayment, preopenGatewayPopup, discardPendingPopup, shouldRedirectInsteadOfPopup, type PaymentMethod } from '@/lib/payments';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';
import SeatingMap, { detectVenueType, MapLegend } from '@/components/seatingMap';
import RiskAcknowledgement from '@/components/risk-acknowledgement';
import InlineCheckout from '@/components/inlineCheckout';

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
// ─── Availability ─────────────────────────────────────────────────
//
// The server already decides this. models.TicketType.ComputeAvailable() sets
// `available_tickets` and `is_sold_out` on every tier it returns, and those are
// the same numbers the booking transaction enforces under its row lock. So the
// job here is to READ that answer, not to recompute it.
//
// It used to recompute it, from a field that does not exist:
//
//     tt.available ?? (tt.capacity ? tt.capacity - tt.tickets_sold - ... : null)
//
// The API sends `available_tickets`, never `available`, so the left side was
// always undefined and the fallback always ran. That fallback disagreed with the
// server in two cases:
//
//   capacity 0   -> `tt.capacity ?` is falsy, so 0 was read as "unlimited" and a
//                   tier with no allocation showed as Available.
//   oversold     -> available of -2 is not `=== 0`, so an over-allocated tier
//                   showed "-2 left" instead of Sold Out.
//
// Reading the server field fixes both and removes the duplicate rule. The local
// computation is kept only as a fallback for older payloads, now with `!= null`
// instead of a truthiness test and `<= 0` instead of `=== 0`.
function availableOf(tt: TicketType): number | null {
  if (typeof tt.available_tickets === 'number') {
    // -1 is the UnlimitedTickets sentinel from the Go model.
    return tt.available_tickets < 0 ? null : tt.available_tickets;
  }
  if (tt.capacity != null) {
    return Math.max(0, tt.capacity - (tt.tickets_sold ?? 0) - (tt.reserved_count ?? 0));
  }
  return null; // no cap = unlimited
}

function isSoldOut(tt: TicketType): boolean {
  if (typeof tt.is_sold_out === 'boolean') return tt.is_sold_out;
  const av = availableOf(tt);
  return av !== null && av <= 0;
}

function availLabel(tt: TicketType): string {
  if (isSoldOut(tt)) return 'Sold out';
  const av = availableOf(tt);
  if (av === null) return 'Available';
  if (av < 10) return `${av} left`;
  return `${av.toLocaleString()} available`;
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

  // CPA s49 acknowledgement. Deliberately NOT persisted anywhere: each
  // purchase needs its own affirmative act, so a buyer who ticked this last
  // week has not thereby consented this week.
  // Every on-sale tier gone. Not the same as "no tiers on sale yet" — an
  // Early Bird that opens next week has sold nothing and is not sold out.
  const allTiersSoldOut = ticketTypes.length > 0 && ticketTypes.every(isSoldOut);

  const [riskAccepted,   setRiskAccepted]   = useState(false);
  const [riskNudge,      setRiskNudge]      = useState(false);
  const [paymentMethod,  setPaymentMethod]  = useState<PaymentMethodId>(defaultPaymentMethod());
  const [paySheetOpen,   setPaySheetOpen]   = useState(false);
  const [contactOpen,    setContactOpen]    = useState(false);
  const [guestName,      setGuestName]      = useState('');
  const [guestSurname,   setGuestSurname]   = useState('');
  const [guestEmail,     setGuestEmail]     = useState('');
  const [error,          setError]          = useState('');

  // ── Payment intent, once the booking exists ──────────────────────────
  //
  // Apple Pay's button is mounted by Paystack against a transaction that
  // already exists, because the sheet has to open inside a user gesture with
  // no async gap in between. So the booking must be created BEFORE the button
  // can render — which is why tapping Pay no longer navigates straight out.
  //
  // It still creates the booking at exactly the same moment it always did, so
  // inventory behaviour is unchanged. booking_repository.go increments
  // tickets_sold inside the booking transaction, so a booking created any
  // earlier than this would hold seats for everyone merely browsing.
  const [intent, setIntent] = useState<{
    bookingId: string;
    accessCode: string;
    reference: string;
    amount: number;
    payParams: string;
  } | null>(null);
  // Guards the automatic fall-through to the card page so it can only fire
  // once, even if Paystack reports unavailability more than once.
  const fellBack = useRef(false);

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
  // Service fee mirrors the backend: R10 per ticket capped at 20% of face
  // value, rounded to cents. The old flat 5% estimate showed a total that
  // didn't match what Paystack actually charged.
  const platformFee  = totalAmount > 0
    ? Math.round(Math.min(10, 0.2 * (selectedType?.price ?? 0)) * quantity * 100) / 100
    : 0;
  const grandTotal   = totalAmount + platformFee;
  // Seating maps are disabled for launch (like the merch tab). All events go
  // straight from ticket selection to the order summary. Flip this to true to
  // restore the Choose Seats step -- all its code below is intact.
  const SEATING_ENABLED = false;
  const rawVenueType = (() => {
  if (!event) return 'none' as const;
  const override = (event as any).venue_type as string | undefined;
  if (override && override !== 'auto') return override as 'stadium' | 'auditorium' | 'none';
  return detectVenueType(event.category ?? '', event.venue_name);
})();
  const venueType = SEATING_ENABLED ? rawVenueType : 'none';

  const handleBuyNow = async () => {
    if (!selectedType || !event) return;
    if (!user) {
      if (!guestName.trim() || !guestSurname.trim()) { setError('Please enter your name and surname.'); return; }
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(guestEmail.trim())) { setError('Please enter a valid email.'); return; }
    }
    setAdding(true);
    setError('');
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

      // Pre-open the gateway popup within the tap gesture so mobile browsers
      // don't block it; the payment page falls back to a full redirect if the
      // popup was blocked anyway.
      //
      // Skip it entirely for redirect-first browsers (Chrome/Firefox/Edge on
      // iOS, in-app webviews): there the placeholder popup either fails to open
      // or leaves an orphan blank tab, and the payment page redirects in-tab
      // anyway. Not pre-opening keeps those browsers clean.
      if (!shouldRedirectInsteadOfPopup()) {
        preopenGatewayPopup();
      }

      // channel=web makes the booking's pre-initialised card checkout use the
      // web return page. Without it the gateway redirected back to the
      // eventsapp:// deep link, which Safari rejects ("address is invalid").
      const { data: booking } = await api.post(endpoint, payload, { params: { channel: 'web' } });
      const bookingId = (booking as any).booking_id ?? booking.id;

      // Free booking: nothing to pay, tickets are issued directly.
      if (!booking.total_amount || Number(booking.total_amount) <= 0) {
        discardPendingPopup();
        router.replace('/tickets');
        return;
      }

      // Resolve the gateway checkout URL (same flow as desktop):
      // • Card: booking creation already initialised a Paystack checkout and
      //   returned its URL in client_token — reuse it.
      // • Instant EFT: start a method-specific checkout.
      let authorizationUrl: string;
      let usedMethod: PaymentMethod = 'card';
      let accessCode = '';
      let startedReference = '';
      // client_token carries a checkout URL but no access code, and without
      // the access code there is no Apple Pay. Signed-in buyers spend one
      // extra round-trip to get it; guests keep the shortcut, since the guest
      // route has no access code to give.
      if (paymentMethod === 'card' && user) {
        const started = await startBookingPayment(bookingId, 'card');
        authorizationUrl = started.authorization_url || (booking as any).client_token;
        accessCode       = started.access_code;
        startedReference = started.reference;
      } else if (paymentMethod === 'card' && (booking as any).client_token) {
        authorizationUrl = (booking as any).client_token;
      } else if (paymentMethod === 'instant_eft') {
        if (user) {
          const started = await startBookingPayment(bookingId, paymentMethod);
          authorizationUrl = started.authorization_url;
          accessCode       = started.access_code;
          startedReference = started.reference;
        } else {
          const started = await startGuestPayment((booking as any).booking_number, guestEmail.trim(), paymentMethod);
          authorizationUrl = started.authorization_url;
        }
        usedMethod = paymentMethod;
      } else {
        authorizationUrl = (booking as any).client_token ?? '';
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
      if (!user) {
        params.set('bookingNumber', (booking as any).booking_number ?? '');
        params.set('email', guestEmail.trim());
      }
      // Logged-in Paystack payments come back with an access code, the only
      // client-side path that preserves the organiser subaccount split. With
      // one, stay on this page and let Apple Pay mount. Guests and PayPal have
      // no access code and keep the redirect flow.
      // TEMPORARY: confirms the access code reaches the browser. Remove
      // once the inline sheet is verified working.
      console.log('[pay] accessCode:', accessCode || '(empty)', 'signedIn:', !!user);
      if (accessCode) {
        setIntent({
          bookingId,
          accessCode,
          reference: startedReference,
          amount: Number((booking as any).total_amount ?? 0),
          payParams: params.toString(),
        });
        return;
      }

      router.push(`/payment/${bookingId}?${params.toString()}`);
    } catch (e: any) {
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
            ) : allTiersSoldOut ? (
              /* Every on-sale tier is gone. Without this the page rendered a list
                 of greyed-out tiers and no CTA — nothing visibly broken, but the
                 buyer had no idea why they could not continue. Distinct from the
                 branch above, which is "not on sale yet" and is not sold out. */
              <div style={{ textAlign: 'center', padding: '40px 0', color: colors.subText }}>
                <p style={{ fontSize: 40, margin: '0 0 8px' }}>⚠️</p>
                <p style={{ margin: '0 0 6px', fontSize: 16, fontWeight: 800, color: colors.text }}>
                  Sold out
                </p>
                <p style={{ margin: 0, fontSize: 13, maxWidth: 320, marginLeft: 'auto', marginRight: 'auto' }}>
                  Every ticket type for this event has sold out. Follow the event to be
                  notified if more tickets are released.
                </p>
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
                        {/* Description AND availability, not one or the other. This was
                            `tt.description ?? availLabel(tt)`, so any tier carrying a
                            description — all of them in practice — never showed
                            "3 left" or "47 available". The scarcity signal existed in
                            code and never reached a buyer. */}
                        {sold
                          ? 'Sold out'
                          : [tt.description, availLabel(tt)].filter(Boolean).join(' · ')}
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
                <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 7 }}>
                  {paymentMethod === 'instant_eft'
                    ? <Landmark size={16} color={colors.subText} />
                    : <CreditCard size={16} color={colors.subText} />}
                  <p style={{ margin: 0, fontSize: 13, color: colors.text, fontWeight: 600 }}>
                    {paymentMethod === 'instant_eft' ? 'Instant EFT' : 'Credit / Debit Card'} ›
                  </p>
                </div>
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

            {/* CPA s49 acknowledgement. Lives INSIDE the summary content so it
                flows directly under the price breakdown — the wrapper above
                already reserves 100px at the bottom for the fixed CTA bar, and
                rendering this outside that wrapper put all 100px of it between
                the total and the checkbox. */}
            <RiskAcknowledgement
              checked={riskAccepted}
              onChange={(v) => { setRiskAccepted(v); if (v) setRiskNudge(false); }}
              highlight={riskNudge}
            />

            {error && <p style={{ color: '#ff3c6e', fontSize: 13, margin: 0 }}>{error}</p>}
          </div>
        )}
      </div>


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
              // Gate: no payment without the s49 acknowledgement.
              if (!riskAccepted) { setRiskNudge(true); return; }
              handleBuyNow();
            }
          }}
          disabled={adding || (step === 'summary' && !riskAccepted)}
          style={{ flex: 1, background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 14, padding: '14px', fontSize: 15, fontWeight: 800, cursor: (adding || (step === 'summary' && !riskAccepted)) ? 'not-allowed' : 'pointer', opacity: (adding || (step === 'summary' && !riskAccepted)) ? 0.55 : 1 }}
        >
          {adding ? 'Processing…' : step === 'select' ? (venueType !== 'none' ? 'Choose Seats' : 'Review Order') : step === 'map' ? 'Review Order' : 'Pay Now'}
        </button>
      </div>
    )}

      {/* ── Payment sheet ───────────────────────────────────────────────
          Resumes the server-initialised transaction in place. Apple Pay
          appears as a channel inside Paystack's sheet on Apple hardware;
          card for everyone else. Either way the split rides on the access
          code, so nothing about the money is decided in the browser. */}
      {intent && (
        <InlineCheckout
          accessCode={intent.accessCode}
          onSuccess={() => { discardPendingPopup(); router.replace('/tickets'); }}
          onCancel={() => { /* leave the sheet up so they can retry */ }}
          onUnavailable={() => {
            // InlineJS blocked or failed: fall back to the hosted page so
            // the sale still completes. Guarded — may fire more than once.
            if (fellBack.current) return;
            fellBack.current = true;
            router.push(`/payment/${intent.bookingId}?${intent.payParams}`);
          }}
        >
          {({ open, ready }) => (
            <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', zIndex: 110, display: 'flex', alignItems: 'flex-end', justifyContent: 'center' }}>
              <div style={{ background: isDark ? '#1a1a1a' : '#fff', borderRadius: '20px 20px 0 0', padding: '20px 20px 36px', width: '100%', maxWidth: 520 }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 4 }}>
                  <h3 style={{ margin: 0, fontSize: 17, fontWeight: 800, color: colors.text }}>Complete payment</h3>
                  <button onClick={() => { setIntent(null); discardPendingPopup(); }} aria-label="Close"
                    style={{ background: 'none', border: 'none', cursor: 'pointer', color: colors.subText, padding: 4 }}><X size={20} /></button>
                </div>
                <p style={{ margin: '0 0 16px', fontSize: 13, color: colors.subText }}>
                  {`R${Math.round(intent.amount).toLocaleString('en-ZA')} · ${quantity} ${quantity === 1 ? 'ticket' : 'tickets'}`}
                </p>
                <button
                  onClick={open}
                  disabled={!ready}
                  style={{ width: '100%', background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 12, padding: '15px', fontSize: 15, fontWeight: 800, cursor: ready ? 'pointer' : 'wait', opacity: ready ? 1 : 0.6 }}
                >
                  {ready ? 'Pay securely' : 'Loading…'}
                </button>
                <p style={{ margin: '10px 0 0', fontSize: 12, color: colors.subText, textAlign: 'center' }}>
                  Apple Pay, card and 3-D Secure
                </p>
              </div>
            </div>
          )}
        </InlineCheckout>
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
            <h3 style={{ margin: '0 0 4px', color: colors.text }}>Payments</h3>
            {(enabledPaymentMethods([
              { id: 'card',        Icon: CreditCard, label: 'Credit / Debit Card' },
              { id: 'instant_eft', Icon: Landmark,   label: 'Instant EFT' },
            ] as const)).map(({ id, Icon, label }) => (
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