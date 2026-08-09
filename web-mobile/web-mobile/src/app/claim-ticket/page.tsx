'use client';

import { Suspense, useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { api } from '@/lib/api';
import { useAppTheme } from '@/themes/colors';
import { RotatingQR } from '@/components/rotatingQR';
import { Ticket, HourglassMedium, WarningCircle, LinkBreak } from '@phosphor-icons/react';

/**
 * /claim-ticket
 *
 * Landing page for ticket-assignment emails. Three modes, decided by the URL:
 *
 *   ?token=<claim uuid>            → "claim your ticket" form. The recipient
 *                                    confirms their name + email, which POSTs
 *                                    to the backend and claims the slot.
 *   ?assignment=<id>&email=<email> → "my ticket" view: renders the LIVE
 *                                    rotating entry QR for a claimed slot.
 *                                    This is the permanent link sent in the
 *                                    claim-confirmation email.
 *   neither                        → invalid-link state.
 *
 * Backend contract:
 *   POST /v1/tickets/claim { token, claimant_name, claimant_email }
 *     → { assignment_id, slot_number, event_title, ticket_url }
 *     404 invalid/used · 410 expired · 409 already claimed
 *   GET  /v1/tickets/claimed/:assignmentId/qr?email=…
 *     → rotating QR token (same shape as the authed endpoint)
 */

interface FieldProps {
  placeholder: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
  error?: string;
  autoComplete?: string;
}

function Field({ placeholder, value, onChange, type = 'text', error, autoComplete }: FieldProps) {
  const [focused, setFocused] = useState(false);
  const { isDark } = useAppTheme();
  return (
    <div className="flex flex-col gap-1">
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoComplete={autoComplete}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        style={{
          background: isDark ? '#1c1c1c' : '#f5f5f5',
          color: isDark ? '#fff' : '#0a0a0a',
          border: `1.5px solid ${error ? '#ff3c6e' : focused ? (isDark ? '#555' : '#0a0a0a') : (isDark ? '#333' : '#e0e0e0')}`,
          borderRadius: 10,
          padding: '12px 14px',
          fontSize: 15,
          outline: 'none',
          width: '100%',
          boxSizing: 'border-box' as const,
        }}
      />
      {error && <span style={{ color: '#ff3c6e', fontSize: 12, fontWeight: 600 }}>{error}</span>}
    </div>
  );
}

interface ClaimedTicket {
  assignmentId: string;
  email: string;
  eventTitle?: string;
  slotNumber?: number;
}

function ClaimTicketInner() {
  const router = useRouter();
  const params = useSearchParams();
  const { isDark, colors } = useAppTheme();

  const token       = useMemo(() => params.get('token')?.trim() ?? '', [params]);
  const assignment  = useMemo(() => params.get('assignment')?.trim() ?? '', [params]);
  const emailParam  = useMemo(() => params.get('email')?.trim() ?? '', [params]);

  const [loading, setLoading]     = useState(false);
  const [globalErr, setGlobalErr] = useState<string | null>(null);
  const [errors, setErrors]       = useState<Record<string, string>>({});
  const [expired, setExpired]     = useState(false);
  const [ticket, setTicket]       = useState<ClaimedTicket | null>(
    assignment && emailParam ? { assignmentId: assignment, email: emailParam } : null,
  );

  const mode: 'ticket' | 'claim' | 'invalid' =
    ticket ? 'ticket' : token ? 'claim' : 'invalid';

  // claim-form state
  const [name, setName]   = useState('');
  const [email, setEmail] = useState('');

  const clearErr = (k: string) =>
    setErrors((e) => { const n = { ...e }; delete n[k]; return n; });

  const handleClaim = async () => {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = 'Enter your full name';
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) errs.email = 'Enter a valid email address';
    setErrors(errs);
    setGlobalErr(null);
    if (Object.keys(errs).length) return;

    setLoading(true);
    try {
      const res = await api.post<{
        assignment_id: string;
        slot_number: number;
        event_title: string;
      }>('/v1/tickets/claim', {
        token,
        claimant_name: name.trim(),
        claimant_email: email.trim(),
      });

      const t: ClaimedTicket = {
        assignmentId: res.data.assignment_id,
        email: email.trim(),
        eventTitle: res.data.event_title,
        slotNumber: res.data.slot_number,
      };
      setTicket(t);
      // Make the URL the permanent "my ticket" link so a refresh (or a
      // bookmark) keeps working after the single-use claim token is spent.
      router.replace(
        `/claim-ticket?assignment=${encodeURIComponent(t.assignmentId)}&email=${encodeURIComponent(t.email)}`,
      );
    } catch (err: any) {
      const status = err?.response?.status;
      const data   = err?.response?.data;
      if (status === 410) {
        setExpired(true);
      } else if (status === 404) {
        setGlobalErr('This claim link is invalid or has already been used.');
      } else if (status === 409) {
        setGlobalErr(data?.error ?? 'This ticket has already been claimed.');
      } else if (status === 403) {
        // Server enforces that the entered email matches the address the
        // invitation was sent to.
        setErrors((e) => ({ ...e, email: 'Use the email address that received the invitation' }));
        setGlobalErr(data?.error ?? 'This ticket was assigned to a different email address.');
      } else if (status === 429) {
        setGlobalErr('Too many attempts. Please wait a few minutes and try again.');
      } else if (data?.details) {
        setGlobalErr('Please check your details and try again.');
      } else {
        setGlobalErr('Something went wrong. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  };

  const card: React.CSSProperties = {
    width: '100%', maxWidth: 420, borderRadius: 18, padding: '28px 24px',
    background: isDark ? '#111' : '#fff',
    border: `1px solid ${isDark ? '#262626' : '#eee'}`,
    boxShadow: isDark ? 'none' : '0 8px 30px rgba(0,0,0,0.06)',
  };
  const primaryBtn = (disabled: boolean): React.CSSProperties => ({
    background: isDark ? '#fff' : '#0a0a0a', color: isDark ? '#0a0a0a' : '#fff',
    border: 'none', borderRadius: 12, padding: '14px', fontSize: 15, fontWeight: 800,
    cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? 0.6 : 1, marginTop: 4, width: '100%',
  });

  return (
    <div style={{ minHeight: '100dvh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20, background: colors.background }}>
      <div style={card}>

        {/* ── Live ticket view ───────────────────────────────────────────── */}
        {mode === 'ticket' && ticket && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14, alignItems: 'center', textAlign: 'center' }}>
            <div style={{ width: 56, height: 56, borderRadius: 28, background: 'rgba(46,204,113,0.15)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Ticket size={26} weight="fill" color="#2ecc71" /></div>
            <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>
              {ticket.eventTitle ? `You're going to ${ticket.eventTitle}!` : 'Your ticket'}
            </h1>
            {ticket.slotNumber != null && (
              <p style={{ margin: 0, fontSize: 13, color: colors.subText }}>Ticket slot {ticket.slotNumber}</p>
            )}

            {/* Dark "ticket" panel — the RotatingQR's loading/error states are
                styled for dark backgrounds, and it reads as a physical ticket. */}
            <div style={{ background: '#0a0a0a', borderRadius: 16, padding: '24px 28px', border: '1px solid #262626' }}>
              <RotatingQR
                assignmentId={ticket.assignmentId}
                size={220}
                endpoint={`/v1/tickets/claimed/${encodeURIComponent(ticket.assignmentId)}/qr?email=${encodeURIComponent(ticket.email)}`}
              />
            </div>

            <p style={{ margin: 0, fontSize: 13, color: colors.subText, lineHeight: 1.5 }}>
              Show this screen at the gate. The QR code refreshes automatically for
              security — screenshots and printouts won&apos;t scan, so keep this page
              open (it&apos;s also linked in your confirmation email).
            </p>
          </div>
        )}

        {/* ── Expired link ───────────────────────────────────────────────── */}
        {mode === 'claim' && expired && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14, alignItems: 'center', textAlign: 'center' }}>
            <div style={{ width: 56, height: 56, borderRadius: 28, background: 'rgba(255,60,110,0.12)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><HourglassMedium size={26} weight="regular" color="#ff3c6e" /></div>
            <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>This link has expired</h1>
            <p style={{ margin: 0, fontSize: 14, color: colors.subText, lineHeight: 1.5 }}>
              Claim links are valid for 24 hours. Ask the person who sent you the
              ticket to resend the invitation from their booking.
            </p>
          </div>
        )}

        {/* ── Claim form ─────────────────────────────────────────────────── */}
        {mode === 'claim' && !expired && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>Claim your ticket</h1>
            <p style={{ margin: '0 0 4px', fontSize: 13, color: colors.subText, lineHeight: 1.5 }}>
              Someone assigned you a ticket on TicketHawk. Confirm your name and
              the email address this invitation was sent to — your name will
              appear on the ticket.
            </p>

            {globalErr && (
              <div style={{ background: 'rgba(255,60,110,0.1)', border: '1px solid rgba(255,60,110,0.3)', borderRadius: 10, padding: '10px 14px', display: 'flex', alignItems: 'center', gap: 8 }}>
                <WarningCircle size={15} weight="fill" color="#ff3c6e" />
                <span style={{ color: '#ff3c6e', fontSize: 13, fontWeight: 600 }}>{globalErr}</span>
              </div>
            )}

            <Field placeholder="Full name" value={name} onChange={(v) => { setName(v); clearErr('name'); }} error={errors.name} autoComplete="name" />
            <Field placeholder="Email address" value={email} onChange={(v) => { setEmail(v); clearErr('email'); }} type="email" error={errors.email} autoComplete="email" />

            <button onClick={handleClaim} disabled={loading} style={primaryBtn(loading)}>
              {loading ? 'Claiming…' : 'Claim my ticket'}
            </button>

            <p style={{ textAlign: 'center', fontSize: 12, color: colors.subText, margin: 0, lineHeight: 1.5 }}>
              For security, this must be the email address the invitation was sent to.
              Your ticket link will be emailed there.
            </p>
          </div>
        )}

        {/* ── Invalid link ───────────────────────────────────────────────── */}
        {mode === 'invalid' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14, alignItems: 'center', textAlign: 'center' }}>
            <div style={{ width: 56, height: 56, borderRadius: 28, background: 'rgba(255,60,110,0.12)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><LinkBreak size={26} weight="regular" color="#ff3c6e" /></div>
            <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>Invalid link</h1>
            <p style={{ margin: 0, fontSize: 14, color: colors.subText, lineHeight: 1.5 }}>
              This ticket link is missing its details. Please open the exact link
              from your email, or ask the sender to resend the invitation.
            </p>
            <button onClick={() => router.push('/')} style={primaryBtn(false)}>Browse events</button>
          </div>
        )}
      </div>
    </div>
  );
}

// useSearchParams requires a Suspense boundary in the App Router.
export default function ClaimTicketPage() {
  return (
    <Suspense fallback={null}>
      <ClaimTicketInner />
    </Suspense>
  );
}