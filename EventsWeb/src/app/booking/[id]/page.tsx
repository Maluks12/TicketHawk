'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';
import { api } from '@/lib/api';
import { RotatingQR } from '@/components/rotatingQR';

interface BookingDetail {
  id: string; booking_number: string; event_id: string; ticket_quantity: number;
  unit_price?: number; subtotal?: number; platform_fee?: number; total_amount: number;
  discount_amount?: number; currency: string; booking_status: string; payment_status: string;
  qr_code_url?: string | null; tickets_sent?: boolean; ticket_type_name?: string;
  attendee_first_name?: string; attendee_last_name?: string; attendee_email?: string;
  created_at: string; confirmed_at?: string; refunded_at?: string;
  refunded_reason?: string; refunded_amount?: number;
  event?: { title: string; start_time: string; venue_name: string; city: string; cover_image_url?: string; address?: string };
}

interface TicketAssignment { id: string; slot_number: number; status: string; assignee_name?: string; assignee_email?: string; }

const STATUS_COLOR: Record<string, string> = { confirmed: '#00e5a0', pending: '#ffb830', cancelled: '#ff4d4d', refunded: '#888' };
const fmt = (iso: string) => new Date(iso).toLocaleDateString('en-ZA', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });
const fmtTime = (iso: string) => new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
const fmtMoney = (n: number) => `R${Math.round(n).toLocaleString('en-ZA')}`;
const fmtDT = (iso: string) => new Date(iso).toLocaleString('en-ZA', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });

function InfoRow({ icon, label, value, valueColor }: { icon: string; label: string; value: string; valueColor?: string }) {
  const { colors } = useAppTheme();
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', padding: '10px 0', borderBottom: `1px solid ${colors.dividerColor}` }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: colors.subText, fontSize: 13 }}>
        <span>{icon}</span><span>{label}</span>
      </div>
      <span style={{ fontSize: 13, fontWeight: 600, color: valueColor ?? colors.text, textAlign: 'right', maxWidth: '60%' }}>{value}</span>
    </div>
  );
}

function PriceLine({ label, value, bold, accent }: { label: string; value: string; bold?: boolean; accent?: boolean }) {
  const { colors } = useAppTheme();
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0' }}>
      <span style={{ fontSize: bold ? 16 : 14, fontWeight: bold ? 700 : 500, color: colors.text }}>{label}</span>
      <span style={{ fontSize: bold ? 16 : 14, fontWeight: bold ? 800 : 600, color: accent ? '#ff3c6e' : colors.text }}>{value}</span>
    </div>
  );
}

export default function BookingDetailPage({ params }: { params: { id: string } }) {
  const router = useRouter();
  const { user } = useAuthStore();
  const { colors, isDark } = useAppTheme();
  const { id } = params;

  const [booking, setBooking] = useState<BookingDetail | null>(null);
  const [assignments, setAssignments] = useState<TicketAssignment[]>([]);
  const [loading, setLoading] = useState(true);
  const [refundReason, setRefundReason] = useState('');
  const [refundOpen, setRefundOpen] = useState(false);
  const [refundLoading, setRefundLoading] = useState(false);
  const [refundErr, setRefundErr] = useState('');
  const [refunded, setRefunded] = useState(false);
  const [assignTarget, setAssignTarget] = useState<TicketAssignment | null>(null);
  const [assignName, setAssignName] = useState('');
  const [assignEmail, setAssignEmail] = useState('');
  const [assignLoading, setAssignLoading] = useState(false);
  const [assignErr, setAssignErr] = useState('');

  const refreshAssignments = useCallback(async () => {
    if (!id) return;
    try { const res = await api.get<{ data: TicketAssignment[] }>(`/v1/bookings/${id}/assignments`); setAssignments(res.data.data ?? []); } catch {}
  }, [id]);

  useEffect(() => {
    const load = async () => {
      if (!user) { setLoading(false); return; }
      try {
        const [bRes, aRes] = await Promise.all([
          api.get<{ data: BookingDetail }>(`/v1/bookings/${id}`),
          api.get<{ data: TicketAssignment[] }>(`/v1/bookings/${id}/assignments`).catch(() => ({ data: { data: [] } })),
        ]);
        setBooking(bRes.data.data ?? (bRes.data as any));
        setAssignments((aRes.data as any).data ?? []);
      } catch {} finally { setLoading(false); }
    };
    load();
  }, [id, user]);

  const handleRefund = async () => {
    if (refundReason.trim().length < 10) { setRefundErr('Please provide at least 10 characters.'); return; }
    setRefundLoading(true); setRefundErr('');
    try { await api.post(`/v1/bookings/${id}/refund`, { reason: refundReason.trim() }); setRefunded(true); setRefundOpen(false); }
    catch (e: any) { setRefundErr(e?.response?.data?.error ?? 'Failed to submit. Please try again.'); }
    finally { setRefundLoading(false); }
  };

  const handleAssign = async () => {
    if (!assignTarget) return;
    if (!assignName.trim()) { setAssignErr("Please enter the recipient's name."); return; }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(assignEmail.trim())) { setAssignErr('Please enter a valid email address.'); return; }
    setAssignLoading(true); setAssignErr('');
    try {
      await api.post(`/v1/bookings/${id}/assignments/${assignTarget.id}`, { assignee_name: assignName.trim(), assignee_email: assignEmail.trim().toLowerCase() });
      await refreshAssignments(); setAssignTarget(null); setAssignName(''); setAssignEmail('');
    } catch (e: any) { setAssignErr(e?.response?.data?.error ?? 'Failed to send invite.'); }
    finally { setAssignLoading(false); }
  };

  const bg = isDark ? '#0a0a0a' : '#f5f5f7';
  const cardBg = isDark ? '#111' : '#fff';
  const border = isDark ? '#222' : '#e5e5e5';
  const inputStyle = { background: isDark ? '#1c1c1c' : '#f5f5f5', color: colors.text, border: `1px solid ${border}`, borderRadius: 10, padding: '11px 14px', fontSize: 14, width: '100%', boxSizing: 'border-box' as const, outline: 'none' };

  if (loading) return <div style={{ minHeight: '100vh', background: bg, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><div style={{ width: 32, height: 32, border: '3px solid #ff2d55', borderTopColor: 'transparent', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} /></div>;
  if (!user) return <div style={{ minHeight: '100vh', background: bg, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 12, padding: 40 }}><h2 style={{ color: colors.text }}>Sign in to view your booking</h2><button onClick={() => router.push('/auth/login')} style={{ background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 12, padding: '13px 28px', fontWeight: 800, cursor: 'pointer', fontSize: 15 }}>Log in</button></div>;
  if (!booking) return <div style={{ minHeight: '100vh', background: bg, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 12, padding: 40 }}><span style={{ fontSize: 40 }}>⚠️</span><h2 style={{ color: colors.text }}>Booking not found</h2><button onClick={() => router.back()} style={{ background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 12, padding: '13px 28px', fontWeight: 800, cursor: 'pointer', fontSize: 15 }}>Go back</button></div>;

  const status = refunded ? 'refunded' : booking.booking_status;
  const statusCol = STATUS_COLOR[status] ?? '#888';
  const canRefund = status === 'confirmed' && !refunded && !!user;
  const hasAssignments = assignments.length > 0;

  return (
    <div style={{ minHeight: '100vh', background: bg }}>
      <div style={{ maxWidth: 680, margin: '0 auto', padding: '0 16px 60px' }}>

        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '24px 0 16px', borderBottom: `1px solid ${border}` }}>
          <button onClick={() => router.back()} style={{ background: 'none', border: 'none', cursor: 'pointer', color: colors.text, fontSize: 22 }}>←</button>
          <h1 style={{ margin: 0, fontSize: 20, fontWeight: 800, color: colors.text, flex: 1 }} className="truncate">{booking.event?.title ?? 'Ticket'}</h1>
          <span style={{ background: `${statusCol}18`, border: `1px solid ${statusCol}44`, color: statusCol, borderRadius: 8, padding: '4px 10px', fontSize: 11, fontWeight: 800 }}>{status.toUpperCase()}</span>
        </div>

        {/* Hero */}
        {booking.event?.cover_image_url && (
          <div style={{ height: 200, borderRadius: 16, overflow: 'hidden', margin: '16px 0', position: 'relative' }}>
            <img src={booking.event.cover_image_url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
            <div style={{ position: 'absolute', inset: 0, background: 'linear-gradient(to bottom, transparent 40%, rgba(0,0,0,0.8))' }} />
            <div style={{ position: 'absolute', bottom: 16, left: 16, right: 16 }}>
              <p style={{ margin: 0, fontSize: 18, fontWeight: 800, color: '#fff' }}>{booking.event.title}</p>
              {booking.event.start_time && <p style={{ margin: '4px 0 0', fontSize: 13, color: 'rgba(255,255,255,0.75)' }}>{fmt(booking.event.start_time)} · {fmtTime(booking.event.start_time)}</p>}
            </div>
          </div>
        )}

        {/* QR / Entry passes — one card per ticket slot */}
<div style={{ marginTop: 24 }}>
  <p style={{ fontSize: 11, fontWeight: 800, letterSpacing: 1.4, color: 'rgba(128,128,128,0.6)', margin: '0 0 12px' }}>ENTRY PASSES</p>

  {booking.status === 'refunded' || booking.status === 'cancelled' ? (
    <div style={{ textAlign: 'center', padding: 24, background: 'rgba(255,255,255,0.03)', borderRadius: 12, opacity: 0.4 }}>
      <p style={{ margin: 0, fontSize: 13, color: '#888' }}>This ticket has been {booking.status}</p>
    </div>
  ) : assignments.length > 0 ? (
    assignments.map((slot: any) => {
      const isClaimed  = slot.status === 'claimed';
      const isPending  = slot.status === 'pending';
      const isInactive = slot.status === 'inactive' || slot.status === 'revoked';
      const status     = booking.status;
      const showQR     = (status === 'confirmed' || status === 'pending') && !isInactive;

      return (
        <div key={slot.id} style={{ marginBottom: 12, borderRadius: 12, overflow: 'hidden', border: '1px solid rgba(255,255,255,0.08)', opacity: isInactive ? 0.35 : 1 }}>
          <div style={{ background: 'rgba(255,255,255,0.04)', padding: '12px 16px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
            <p style={{ margin: 0, fontSize: 12, fontWeight: 700, color: '#aaa' }}>
              {isPending ? `Invite sent to ${slot.assignee_email ?? 'recipient'}` : isClaimed ? 'Assigned' : isInactive ? 'Invalidated' : 'Unassigned'}
            </p>
            <span style={{ fontSize: 10, fontWeight: 800, color: STATUS_COLOR[booking.status] ?? '#888', letterSpacing: 1 }}>
              #{booking.booking_number}
            </span>
          </div>
          <div style={{ padding: 16, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8 }}>
            {showQR && isClaimed ? (
              <RotatingQR assignmentId={slot.id} size={200} />
            ) : (
              <div style={{ width: 200, height: 200, borderRadius: 12, background: 'rgba(255,255,255,0.04)', border: '1px solid rgba(255,255,255,0.06)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8 }}>
                <span style={{ fontSize: 40 }}>{isPending ? '📧' : isInactive ? '🚫' : '⏳'}</span>
                <p style={{ margin: 0, fontSize: 12, color: '#888', textAlign: 'center', maxWidth: 160 }}>
                  {isPending ? `Invite sent to ${slot.assignee_email ?? 'recipient'}` : isInactive ? 'Ticket invalidated' : status === 'confirmed' ? 'Ticket not yet claimed' : 'Complete payment to activate'}
                </p>
              </div>
            )}
            <p style={{ margin: 0, fontSize: 11, color: '#666', textAlign: 'center' }}>
              {isClaimed ? 'Live QR — present at venue entrance' : isPending ? 'Waiting for recipient to claim' : isInactive ? 'This slot has been invalidated' : status === 'confirmed' ? 'QR will appear once claimed' : 'Complete payment to activate your ticket'}
            </p>
          </div>
        </div>
      );
    })
  ) : (
    /* Fallback for older bookings without per-assignment QR */
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', padding: 16 }}>
      {booking.qr_code_url
        ? <img src={booking.qr_code_url} alt="QR Code" style={{ width: 200, height: 200, borderRadius: 8 }} />
        : <span style={{ fontSize: 48 }}>⏳</span>
      }
      <p style={{ margin: '8px 0 0', fontSize: 12, color: '#888', textAlign: 'center' }}>
        {booking.status === 'confirmed'
          ? (booking.qr_code_url ? 'Present this at the venue entrance' : 'QR code is being generated…')
          : 'Complete payment to activate your ticket'}
      </p>
    </div>
  )}
</div>

        {/* Event Details */}
        <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 16, padding: 16, marginTop: 10 }}>
          <p style={{ margin: '0 0 4px', fontSize: 11, fontWeight: 800, letterSpacing: 1.1, color: colors.subText }}>EVENT DETAILS</p>
          {booking.event?.start_time && <InfoRow icon="📅" label="Date & time" value={`${fmt(booking.event.start_time)} at ${fmtTime(booking.event.start_time)}`} />}
          {booking.event && <InfoRow icon="📍" label="Venue" value={`${booking.event.venue_name}, ${booking.event.city}`} />}
          {booking.event?.address && <InfoRow icon="🗺" label="Address" value={booking.event.address} />}
        </div>

        {/* Ticket Info */}
        <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 16, padding: 16, marginTop: 10 }}>
          <p style={{ margin: '0 0 4px', fontSize: 11, fontWeight: 800, letterSpacing: 1.1, color: colors.subText }}>TICKET INFO</p>
          <InfoRow icon="🎟" label="Type" value={booking.ticket_type_name ?? 'General Admission'} />
          <InfoRow icon="👥" label="Quantity" value={`${booking.ticket_quantity} ticket${booking.ticket_quantity > 1 ? 's' : ''}`} />
          <InfoRow icon="✅" label="Status" value={status.charAt(0).toUpperCase() + status.slice(1)} valueColor={statusCol} />
          <InfoRow icon="🕐" label="Booked on" value={fmtDT(booking.created_at)} />
          {booking.confirmed_at && <InfoRow icon="✓" label="Confirmed" value={fmtDT(booking.confirmed_at)} />}
        </div>

        {/* Attendee */}
        {(booking.attendee_first_name || booking.attendee_email) && (
          <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 16, padding: 16, marginTop: 10 }}>
            <p style={{ margin: '0 0 4px', fontSize: 11, fontWeight: 800, letterSpacing: 1.1, color: colors.subText }}>ATTENDEE</p>
            {(booking.attendee_first_name || booking.attendee_last_name) && <InfoRow icon="👤" label="Name" value={[booking.attendee_first_name, booking.attendee_last_name].filter(Boolean).join(' ')} />}
            {booking.attendee_email && <InfoRow icon="✉️" label="Email" value={booking.attendee_email} />}
          </div>
        )}

        {/* Payment */}
        <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 16, padding: 16, marginTop: 10 }}>
          <p style={{ margin: '0 0 8px', fontSize: 11, fontWeight: 800, letterSpacing: 1.1, color: colors.subText }}>PAYMENT BREAKDOWN</p>
          {booking.unit_price != null && <PriceLine label={`Unit price × ${booking.ticket_quantity}`} value={fmtMoney((booking.unit_price ?? 0) * booking.ticket_quantity)} />}
          {(booking.platform_fee ?? 0) > 0 && <PriceLine label="Service fee" value={fmtMoney(booking.platform_fee ?? 0)} />}
          {(booking.discount_amount ?? 0) > 0 && <PriceLine label="Discount" value={`-${fmtMoney(booking.discount_amount ?? 0)}`} accent />}
          <div style={{ height: 1, background: border, margin: '8px 0' }} />
          <PriceLine label="Total paid" value={fmtMoney(booking.total_amount)} bold />
          {(status === 'refunded' || refunded) && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, background: 'rgba(0,229,160,0.1)', borderRadius: 8, padding: '10px 12px', marginTop: 8 }}>
              <span style={{ color: '#00e5a0' }}>✓</span>
              <span style={{ fontSize: 13, color: '#00e5a0', fontWeight: 600 }}>
                {booking.refunded_amount != null ? `Refund of ${fmtMoney(booking.refunded_amount)} is being processed` : 'Refund request submitted — processing in 3–5 business days'}
              </span>
            </div>
          )}
        </div>

        {canRefund && (
          <button onClick={() => setRefundOpen(true)} style={{ display: 'flex', alignItems: 'center', gap: 8, background: 'transparent', border: '1px solid #ff4d4d', color: '#ff4d4d', borderRadius: 12, padding: '12px 20px', fontSize: 14, fontWeight: 700, cursor: 'pointer', marginTop: 16 }}>
            ↩ Request refund
          </button>
        )}
      </div>

      {/* Refund modal */}
      {refundOpen && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', zIndex: 100, display: 'flex', alignItems: 'flex-end', justifyContent: 'center' }} onClick={() => setRefundOpen(false)}>
          <div onClick={(e) => e.stopPropagation()} style={{ background: isDark ? '#1a1a1a' : '#fff', borderRadius: '20px 20px 0 0', padding: '20px 20px 40px', width: '100%', maxWidth: 520 }}>
            <h3 style={{ margin: '0 0 8px', color: colors.text }}>Request a refund</h3>
            <p style={{ margin: '0 0 16px', fontSize: 13, color: colors.subText }}>Tell us why you'd like a refund. Processed within 3–5 business days.</p>
            <textarea value={refundReason} onChange={(e) => { setRefundReason(e.target.value); setRefundErr(''); }} placeholder="e.g. I can no longer attend this event due to..." rows={4} style={{ width: '100%', background: isDark ? '#111' : '#f5f5f5', color: colors.text, border: `1px solid ${refundErr ? '#ff3c6e' : border}`, borderRadius: 10, padding: '12px', fontSize: 14, boxSizing: 'border-box', outline: 'none', resize: 'vertical' }} />
            <p style={{ margin: '4px 0 12px', fontSize: 12, color: colors.subText, textAlign: 'right' }}>{refundReason.length}/500</p>
            {refundErr && <p style={{ color: '#ff3c6e', fontSize: 13, margin: '0 0 12px' }}>{refundErr}</p>}
            <button onClick={handleRefund} disabled={refundLoading} style={{ width: '100%', background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 12, padding: '14px', fontSize: 15, fontWeight: 800, cursor: 'pointer', opacity: refundLoading ? 0.6 : 1 }}>
              {refundLoading ? 'Submitting…' : 'Submit refund request'}
            </button>
            <button onClick={() => setRefundOpen(false)} style={{ width: '100%', background: 'none', border: 'none', color: colors.subText, padding: '12px', fontSize: 14, cursor: 'pointer', marginTop: 8 }}>Cancel</button>
          </div>
        </div>
      )}

      {/* Assign ticket modal */}
      {assignTarget && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', zIndex: 100, display: 'flex', alignItems: 'flex-end', justifyContent: 'center' }} onClick={() => setAssignTarget(null)}>
          <div onClick={(e) => e.stopPropagation()} style={{ background: isDark ? '#1a1a1a' : '#fff', borderRadius: '20px 20px 0 0', padding: '20px 20px 40px', width: '100%', maxWidth: 520 }}>
            <h3 style={{ margin: '0 0 8px', color: colors.text }}>Send ticket to someone</h3>
            <p style={{ margin: '0 0 16px', fontSize: 13, color: colors.subText }}>They'll receive a claim link by email and can pick it up at the door.</p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <input placeholder="Recipient name" value={assignName} onChange={(e) => { setAssignName(e.target.value); setAssignErr(''); }} style={{ background: isDark ? '#111' : '#f5f5f5', color: colors.text, border: `1px solid ${border}`, borderRadius: 10, padding: '12px 14px', fontSize: 14, outline: 'none', width: '100%', boxSizing: 'border-box' as const }} />
              <input placeholder="Recipient email" value={assignEmail} onChange={(e) => { setAssignEmail(e.target.value); setAssignErr(''); }} type="email" style={{ background: isDark ? '#111' : '#f5f5f5', color: colors.text, border: `1px solid ${border}`, borderRadius: 10, padding: '12px 14px', fontSize: 14, outline: 'none', width: '100%', boxSizing: 'border-box' as const }} />
            </div>
            {assignErr && <p style={{ color: '#ff3c6e', fontSize: 13, margin: '8px 0' }}>{assignErr}</p>}
            <button onClick={handleAssign} disabled={assignLoading} style={{ width: '100%', background: '#fff', color: '#0a0a0a', border: 'none', borderRadius: 12, padding: '14px', fontSize: 15, fontWeight: 800, cursor: 'pointer', opacity: assignLoading ? 0.6 : 1, marginTop: 12, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8 }}>
              📤 {assignLoading ? 'Sending…' : 'Send claim link'}
            </button>
            <button onClick={() => setAssignTarget(null)} style={{ width: '100%', background: 'none', border: 'none', color: colors.subText, padding: '12px', fontSize: 14, cursor: 'pointer', marginTop: 4 }}>Cancel</button>
          </div>
        </div>
      )}

      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  );
}