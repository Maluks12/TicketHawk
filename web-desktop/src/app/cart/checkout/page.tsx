'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import {
  type CartItem, type CartSummary,
  getCart, checkoutBegin, checkoutConfirm, checkoutHeartbeat, checkoutAbandon,
  cartItemLabel, cartItemSubtitle, fmtZAR, getCachedSessionId,
} from '@/lib/api';
import { startBookingPayment, preopenGatewayPopup, discardPendingPopup, type PaymentMethod } from '@/lib/payments';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';
import { ShieldCheck, Lock, Smartphone } from 'lucide-react';

type Phase = 'review' | 'reserving' | 'reserved' | 'confirming';

function useCountdown(initialSecs: number, active: boolean) {
  const [secs, setSecs] = useState(initialSecs);
  useEffect(() => { if (!active || secs <= 0) return; const t = setInterval(() => setSecs((s) => Math.max(0, s - 1)), 1000); return () => clearInterval(t); }, [active, secs]);
  useEffect(() => { if (active) setSecs(initialSecs); }, [initialSecs]);
  return secs;
}

function fmtCountdown(secs: number) { const m = Math.floor(secs / 60); const s = secs % 60; return `${m}:${s.toString().padStart(2, '0')}`; }

export default function CartCheckoutPage() {
  const router = useRouter();
  const { colors, isDark } = useAppTheme();
  const { user, authReady, loadUser } = useAuthStore();

  const [cart, setCart] = useState<CartSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [phase, setPhase] = useState<Phase>('review');
  const [reservations, setReservations] = useState<any[]>([]);
  const [initialSecs, setInitialSecs] = useState(600);
  const [payMethod, setPayMethod] = useState<'card' | 'paypal' | 'instant_eft'>('card');
  const [guestName, setGuestName] = useState('');
  useEffect(() => { loadUser(); }, [loadUser]);
  const [guestEmail, setGuestEmail] = useState('');

  const secondsLeft = useCountdown(initialSecs, phase === 'reserved');
  const heartbeatRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const sidRef = useRef<string | null>(null);
  const abandonedRef = useRef(false);

  useEffect(() => {
    (async () => {
      sidRef.current = await getCachedSessionId();
      try { const data = await getCart(sidRef.current); setCart(data); }
      catch { alert('Could not load your cart.'); router.back(); }
      finally { setLoading(false); }
    })();
    return () => {
      if (heartbeatRef.current) clearInterval(heartbeatRef.current);
      if (!abandonedRef.current) checkoutAbandon(sidRef.current).catch(() => {});
    };
  }, []);

  useEffect(() => {
    if (phase !== 'reserved') { if (heartbeatRef.current) { clearInterval(heartbeatRef.current); heartbeatRef.current = null; } return; }
    heartbeatRef.current = setInterval(() => { checkoutHeartbeat(sidRef.current).catch(() => {}); }, 30_000);
    return () => { if (heartbeatRef.current) clearInterval(heartbeatRef.current); };
  }, [phase]);

  useEffect(() => {
    if (phase === 'reserved' && secondsLeft === 0) {
      alert('Your seat reservation has expired. Your cart is intact — you can try again.');
      setPhase('review'); setReservations([]);
    }
  }, [secondsLeft, phase]);

  const allItems = (cart as any)?.items_by_event?.flatMap((g: any) => g.items) ?? [];
  const cartTotal = (cart as any)?.total_amount ?? 0;
  // Mirror the backend fee model (events-db repository.CalculateFees): a R10
  // buyer service fee per TICKET, capped at 20% of that ticket's face value,
  // per line, rounded to cents. Merch lines carry no service fee. The old
  // flat 5%-of-cart estimate didn't match what the gateway actually charges.
  const platformFee = allItems.reduce((sum: number, i: any) => {
    if (i.item_type !== 'ticket') return sum;
    const unit = Number(i.unit_price_snapshot ?? 0);
    const qty  = Number(i.quantity ?? 0);
    if (unit <= 0 || qty <= 0) return sum;
    return sum + Math.round(Math.min(10, 0.2 * unit) * qty * 100) / 100;
  }, 0);
  const grandTotal = cartTotal + platformFee;
  const hasTickets = allItems.some((i: CartItem) => i.item_type === 'ticket');
  const isBusy = phase === 'reserving' || phase === 'confirming';
  const bg = isDark ? '#0a0a0a' : '#f5f5f7';
  const cardBg = isDark ? '#111' : '#fff';
  const border = isDark ? '#222' : '#e5e5e5';
  const isUrgent = secondsLeft <= 120;

  const userDisplayName = user ? (`${(user as any).first_name ?? ''} ${(user as any).last_name ?? ''}`.trim() || (user as any).email) : (guestName || guestEmail);

  const handleBeginCheckout = async () => {
    if (!user) {
      if (!guestName.trim()) { alert('Please enter your name before checking out.'); return; }
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(guestEmail.trim())) { alert('Please enter a valid email address.'); return; }
    }
    if (!hasTickets) { handleConfirm(); return; }
    setPhase('reserving');
    try {
      const data: any = await checkoutBegin(sidRef.current);
      setReservations(data.reservations ?? []);
      setInitialSecs(data.seconds_left ?? 600);
      setPhase('reserved');
    } catch (err: any) {
      setPhase('review');
      alert(err?.response?.data?.error ?? 'Could not reserve your tickets. Please try again.');
    }
  };

  const handleConfirm = async () => {
    preopenGatewayPopup();
    setPhase('confirming');
    abandonedRef.current = true;
    try {
      const data: any = await checkoutConfirm(sidRef.current);
      const primaryId = data.primary_booking_id;

      // Resolve the gateway checkout URL.
      // • Card: booking creation already initialised a Paystack checkout and
      //   returned its URL in client_token. Reuse it to save a round-trip.
      // • PayPal / Instant EFT: initialise a method-specific checkout via the
      //   payment start route.
      let authorizationUrl: string;
      let usedMethod: PaymentMethod = payMethod as PaymentMethod;

      if (payMethod === 'card' && data.client_token) {
        authorizationUrl = data.client_token;
      } else {
        const started = await startBookingPayment(primaryId, payMethod as PaymentMethod);
        authorizationUrl = started.authorization_url;
      }

      if (!authorizationUrl) {
        discardPendingPopup();
        abandonedRef.current = false;
        setPhase('reserved');
        alert('Payment setup failed — no checkout URL returned. Please try again.');
        return;
      }

      const params = new URLSearchParams({
        url:      encodeURIComponent(authorizationUrl),
        method:   usedMethod,
        amount:   String(data.total_amount ?? 0),
        currency: data.currency ?? 'ZAR',
      });
      router.push(`/payment/${primaryId}?${params.toString()}`);
    } catch (err: any) {
      discardPendingPopup();
      abandonedRef.current = false;
      setPhase('reserved');
      alert(err?.response?.data?.error ?? 'Payment setup failed. Please try again.');
    }
  };

  const inputStyle = { background: isDark ? '#111' : '#f5f5f7', color: colors.text, border: `1px solid ${border}`, borderRadius: 12, padding: '12px 14px', fontSize: 15, width: '100%', boxSizing: 'border-box' as const, outline: 'none' };

  if (loading) return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', gap: 12 }}>
      <div style={{ width: 36, height: 36, border: '3px solid #ff2d55', borderTopColor: 'transparent', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
      <p style={{ color: colors.subText, margin: 0 }}>Loading checkout…</p>
    </div>
  );

  return (
    <div style={{ minHeight: '100vh', background: bg }}>
      <div style={{ maxWidth: 640, margin: '0 auto', padding: '0 16px 160px' }}>

        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '24px 0 16px', borderBottom: `1px solid ${border}` }}>
          <button onClick={() => { if (phase === 'reserved' || phase === 'confirming') { if (!confirm('Leave checkout? Your seat reservations will be released.')) return; } router.back(); }} style={{ background: 'none', border: 'none', cursor: 'pointer', color: colors.text, fontSize: 22, padding: 4 }}>←</button>
          <h1 style={{ margin: 0, fontSize: 20, fontWeight: 800, color: colors.text, flex: 1 }}>Checkout</h1>
          {phase === 'reserved' && <span style={{ background: '#ff2d55', color: '#fff', borderRadius: 10, padding: '3px 10px', fontSize: 13, fontWeight: 800, fontVariant: 'tabular-nums' }}>{fmtCountdown(secondsLeft)}</span>}
        </div>

        {/* Reservation banner */}
        {phase === 'reserved' && (
          <div style={{ display: 'flex', gap: 12, alignItems: 'center', background: isUrgent ? '#fff1f3' : '#f0f7ff', border: `1px solid ${isUrgent ? '#ff2d55' : '#3b82f6'}`, borderRadius: 14, padding: 14, margin: '16px 0 4px' }}>
            <span style={{ fontSize: 20 }}>{isUrgent ? '⏱' : '🛡'}</span>
            <div>
              <p style={{ margin: 0, fontSize: 14, fontWeight: 800, color: isUrgent ? '#ff2d55' : '#1d4ed8' }}>{isUrgent ? 'Seats reserved — hurry!' : 'Seats reserved for you'}</p>
              <p style={{ margin: 0, fontSize: 12, fontWeight: 600, color: isUrgent ? '#e11d48' : '#2563eb' }}>{fmtCountdown(secondsLeft)} remaining to complete payment</p>
            </div>
          </div>
        )}

        {/* Contact */}
        <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 16, padding: 16, marginTop: 16 }}>
          <p style={{ margin: '0 0 10px', fontSize: 11, fontWeight: 800, letterSpacing: 1.1, color: colors.subText }}>CONTACT</p>
          {user ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <span style={{ fontSize: 18 }}>👤</span>
              <span style={{ fontSize: 15, fontWeight: 600, color: colors.text }}>{userDisplayName}</span>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <input placeholder="Email address" value={guestEmail} onChange={(e) => setGuestEmail(e.target.value)} type="email" style={inputStyle} />
              <input placeholder="Full name" value={guestName} onChange={(e) => setGuestName(e.target.value)} style={inputStyle} />
            </div>
          )}
        </div>

        {/* Payment method */}
        <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 16, padding: 16, marginTop: 10 }}>
          <p style={{ margin: '0 0 10px', fontSize: 11, fontWeight: 800, letterSpacing: 1.1, color: colors.subText }}>PAYMENT</p>
          <div style={{ display: 'flex', gap: 10 }}>
            {[
              { id: 'card',        emoji: '💳', label: 'Credit / Debit Card' },
              { id: 'paypal',      emoji: '🅿️', label: 'PayPal'              },
              { id: 'instant_eft', emoji: '🏦', label: 'Instant EFT'         },
            ].filter((opt) => !authReady || user || opt.id === 'card').map((opt) => (
              <button key={opt.id} onClick={() => setPayMethod(opt.id as any)} style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 8, padding: '12px 14px', borderRadius: 12, border: `2px solid ${payMethod === opt.id ? '#ff2d55' : border}`, background: payMethod === opt.id ? (isDark ? '#1a0008' : '#fff0f3') : cardBg, cursor: 'pointer' }}>
                <span style={{ fontSize: 20 }}>{opt.emoji}</span>
                <span style={{ fontSize: 13, fontWeight: 600, color: colors.text }}>{opt.label}</span>
              </button>
            ))}
          </div>
        </div>

        {/* Order */}
        <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 16, padding: 16, marginTop: 10 }}>
          <p style={{ margin: '0 0 10px', fontSize: 11, fontWeight: 800, letterSpacing: 1.1, color: colors.subText }}>YOUR ORDER</p>
          {(cart as any)?.items_by_event?.map((group: any, idx: number) => (
            <div key={group.event_id ?? idx}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                <div style={{ width: 7, height: 7, borderRadius: 4, background: '#ff2d55' }} />
                <span style={{ fontSize: 12, fontWeight: 700, color: colors.text }}>{group.event_title || 'Store Items'}</span>
              </div>
              {group.items.map((item: CartItem) => {
                const sub = cartItemSubtitle(item);
                const lineTotal = (item as any).unit_price_snapshot * item.quantity + ((item as any).delivery_fee_snapshot ?? 0);
                return (
                  <div key={item.id} style={{ display: 'flex', gap: 10, borderTop: `1px solid ${border}`, padding: '8px 0', alignItems: 'flex-start' }}>
                    <div style={{ width: 26, height: 26, borderRadius: 7, background: item.item_type === 'ticket' ? '#ff2d55' : '#1c2e61', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, fontSize: 12 }}>{item.item_type === 'ticket' ? '🎟' : '👜'}</div>
                    <div style={{ flex: 1 }}>
                      <p style={{ margin: 0, fontSize: 14, fontWeight: 700, color: colors.text }}>{cartItemLabel(item)}</p>
                      {sub && <p style={{ margin: 0, fontSize: 12, color: colors.subText }}>{sub}</p>}
                      <p style={{ margin: 0, fontSize: 11, color: colors.subText }}>{fmtZAR((item as any).unit_price_snapshot)} × {item.quantity}</p>
                    </div>
                    <span style={{ fontSize: 14, fontWeight: 800, color: colors.text }}>{fmtZAR(lineTotal)}</span>
                  </div>
                );
              })}
            </div>
          ))}
        </div>

        {/* Reservations confirmed */}
        {phase === 'reserved' && reservations.length > 0 && (
          <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 16, padding: 16, marginTop: 10 }}>
            <p style={{ margin: '0 0 10px', fontSize: 11, fontWeight: 800, letterSpacing: 1.1, color: colors.subText }}>RESERVATIONS CONFIRMED</p>
            {reservations.map((r: any) => (
              <div key={r.reservation_id} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '4px 0' }}>
                <span style={{ color: '#22c55e' }}>✓</span>
                <span style={{ fontSize: 14, fontWeight: 600, color: colors.text }}>{r.event_title} — {r.quantity} ticket{r.quantity !== 1 ? 's' : ''}</span>
              </div>
            ))}
          </div>
        )}

        {/* Trust signals */}
        <div style={{ display: 'flex', justifyContent: 'space-around', border: `1px solid ${border}`, borderRadius: 12, padding: 14, marginTop: 10 }}>
          {[{ Icon: ShieldCheck, label: 'Official tickets' }, { Icon: Lock, label: 'Secure checkout' }, { Icon: Smartphone, label: 'Mobile tickets' }].map((t) => (
            <div key={t.label} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 5 }}>
              <t.Icon size={18} color={colors.subText} />
              <span style={{ fontSize: 11, fontWeight: 600, color: colors.subText, textAlign: 'center' }}>{t.label}</span>
            </div>
          ))}
        </div>

        {/* Price breakdown */}
        <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 16, padding: 16, marginTop: 10 }}>
          <p style={{ margin: '0 0 10px', fontSize: 11, fontWeight: 800, letterSpacing: 1.1, color: colors.subText }}>PRICE BREAKDOWN</p>
          {allItems.map((item: CartItem) => (
            <div key={item.id} style={{ display: 'flex', justifyContent: 'space-between', padding: '4px 0' }}>
              <span style={{ fontSize: 14, color: colors.text }}>{cartItemLabel(item)} × {item.quantity}</span>
              <span style={{ fontSize: 14, fontWeight: 600, color: colors.text }}>{fmtZAR((item as any).unit_price_snapshot * item.quantity)}</span>
            </div>
          ))}
          <div style={{ display: 'flex', justifyContent: 'space-between', padding: '4px 0' }}>
            <span style={{ fontSize: 14, color: colors.subText }}>Service fee</span>
            <span style={{ fontSize: 14, color: colors.subText }}>{fmtZAR(platformFee)}</span>
          </div>
          <div style={{ height: 1, background: border, margin: '8px 0' }} />
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <span style={{ fontSize: 17, fontWeight: 700, color: colors.text }}>Total</span>
            <span style={{ fontSize: 20, fontWeight: 800, color: '#ff2d55' }}>{fmtZAR(grandTotal)}</span>
          </div>
        </div>
      </div>

      {/* Sticky CTA */}
      <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0, background: isDark ? '#0a0a0a' : '#fff', borderTop: `1px solid ${border}`, padding: '14px 24px 24px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', zIndex: 50 }}>
        <div>
          <p style={{ margin: 0, fontSize: 12, fontWeight: 600, color: colors.subText }}>Total due</p>
          <p style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>{fmtZAR(grandTotal)}</p>
        </div>
        {phase === 'review' && (
          <button onClick={handleBeginCheckout} disabled={isBusy} style={{ background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 14, padding: '14px 22px', fontSize: 15, fontWeight: 800, cursor: isBusy ? 'not-allowed' : 'pointer', opacity: isBusy ? 0.5 : 1, display: 'flex', alignItems: 'center', gap: 8 }}>
            {hasTickets ? 'Reserve & Continue' : 'Continue to Pay'}
          </button>
        )}
        {phase === 'reserving' && <div style={{ background: '#ff2d55', color: '#fff', borderRadius: 14, padding: '14px 22px', fontSize: 15, fontWeight: 800, opacity: 0.5 }}>Reserving seats…</div>}
        {phase === 'reserved' && (
          <button onClick={handleConfirm} disabled={secondsLeft === 0} style={{ background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 14, padding: '14px 22px', fontSize: 15, fontWeight: 800, cursor: secondsLeft === 0 ? 'not-allowed' : 'pointer', opacity: secondsLeft === 0 ? 0.5 : 1, display: 'flex', alignItems: 'center', gap: 8 }}>
            💳 Pay Now
          </button>
        )}
        {phase === 'confirming' && <div style={{ background: '#ff2d55', color: '#fff', borderRadius: 14, padding: '14px 22px', fontSize: 15, fontWeight: 800, opacity: 0.5 }}>Setting up payment…</div>}
      </div>
    </div>
  );
}