'use client';
// ── Onboarding: complete your profile ─────────────────────────────────────────
// The landing page for any signed-in account WITHOUT an organiser profile
// (KYC tier 0). One short form — legal name + SA ID number — validated by the
// backend (Luhn, DOB, 18+), stored encrypted, no document and no cost. On
// success the account is promoted to organiser (tier 1) and can create
// PRIVATE FREE events immediately. Paid/public events need full verification,
// offered later from the create-event screen.
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useOrgAuth } from '../../store/authStore';
import { refreshAccessToken } from '../../lib/api';
import { submitBasicKyc, looksLikeSaId, getKycStatus } from '../../lib/kyc';

export default function OnboardingPage() {
  const router = useRouter();
  const { user, hydrate, isLoading } = useOrgAuth();

  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [idNumber, setIdNumber] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => { hydrate(); }, [hydrate]);
  useEffect(() => {
    if (!isLoading && !user) router.replace('/login');
  }, [user, isLoading, router]);

  // Already onboarded (e.g. deep link back here)? Straight to the dashboard.
  // Guard on the token being present in this browser first — on a fresh
  // post-login navigation the request interceptor reads evt_token from
  // localStorage, so firing before it's written sends a bare request that the
  // API rejects with "authorization header required".
  useEffect(() => {
    if (!user) return;
    if (typeof window === 'undefined') return;
    if (!localStorage.getItem('evt_token')) return; // token not yet in this browser
    getKycStatus().then((s) => {
      if (s.kyc_level >= 1) router.replace('/dashboard');
    }).catch(() => {});
  }, [user, router]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (firstName.trim().length < 2 || lastName.trim().length < 2) {
      setError('Enter your legal first and last name as they appear on your ID.');
      return;
    }
    if (!looksLikeSaId(idNumber)) {
      setError('Your SA ID number should be exactly 13 digits.');
      return;
    }
    setBusy(true);
    try {
      await submitBasicKyc({
        legal_first_name: firstName.trim(),
        legal_last_name: lastName.trim(),
        sa_id_number: idNumber.trim(),
      });
      // The promotion changed our role server-side; refresh the session so the
      // new organizer token is in hand BEFORE the dashboard loads.
      try { await refreshAccessToken(); } catch { /* dashboard 401s will retry */ }
      router.replace('/dashboard');
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error;
      setError(msg ?? 'Something went wrong — check your details and try again.');
    } finally {
      setBusy(false);
    }
  }

  if (isLoading || !user) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', background: 'var(--bg)' }}>
        <div className="spinner spinner-lg" />
      </div>
    );
  }

  return (
    <div style={{ minHeight: '100vh', background: 'var(--bg)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}>
      <div style={{ width: '100%', maxWidth: 460 }}>
        {/* Brand */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, justifyContent: 'center', marginBottom: 20 }}>
          <div style={{ width: 36, height: 36, borderRadius: 8, background: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <svg viewBox="0 0 20 20" fill="white" width="18" height="18">
              <path fillRule="evenodd" d="M6 2a1 1 0 00-1 1v1H4a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V6a2 2 0 00-2-2h-1V3a1 1 0 10-2 0v1H7V3a1 1 0 00-1-1zm0 5a1 1 0 000 2h8a1 1 0 100-2H6z" clipRule="evenodd"/>
            </svg>
          </div>
          <div style={{ fontSize: 18, fontWeight: 800, letterSpacing: 0.4, color: 'var(--text)' }}>
            TICKET<span style={{ color: 'var(--brand)' }}>HAWK</span>
            <span style={{ fontSize: 12, color: 'var(--text-2)', fontWeight: 500, marginLeft: 6 }}>Organiser</span>
          </div>
        </div>

        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 10, padding: 24 }}>
          <h1 style={{ fontSize: 20, fontWeight: 700, margin: '0 0 6px' }}>Complete your profile</h1>
          <p style={{ fontSize: 13.5, color: 'var(--text-2)', margin: '0 0 18px', lineHeight: 1.5 }}>
            To create and publish events we need to know who you are. This quick step
            lets you create <strong>private free events</strong> right away — it&apos;s
            instant and free. Paid and public events need full verification, which you
            can do later.
          </p>

          <form onSubmit={submit}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginBottom: 12 }}>
              <div>
                <label style={lbl}>Legal first name</label>
                <input className="input" style={inp} value={firstName}
                  onChange={(e) => setFirstName(e.target.value)}
                  placeholder="As on your ID" autoComplete="given-name" />
              </div>
              <div>
                <label style={lbl}>Legal last name</label>
                <input className="input" style={inp} value={lastName}
                  onChange={(e) => setLastName(e.target.value)}
                  placeholder="As on your ID" autoComplete="family-name" />
              </div>
            </div>

            <div style={{ marginBottom: 6 }}>
              <label style={lbl}>SA ID number</label>
              <input className="input" style={inp} value={idNumber}
                onChange={(e) => setIdNumber(e.target.value.replace(/\D/g, '').slice(0, 13))}
                placeholder="13 digits" inputMode="numeric" />
            </div>
            <p style={{ fontSize: 12, color: 'var(--text-2)', margin: '0 0 14px' }}>
              Validated locally and stored encrypted. No documents needed for this step.
            </p>

            {error && (
              <div className="alert alert-error" style={{ fontSize: 13, marginBottom: 12 }}>{error}</div>
            )}

            <button type="submit" className="btn btn-primary" disabled={busy}
              style={{ width: '100%', padding: '10px 0', fontSize: 15 }}>
              {busy ? 'Checking…' : 'Complete profile'}
            </button>
          </form>

          <div style={{ height: 1, background: 'var(--bg)', margin: '16px 0' }} />
          <p style={{ fontSize: 12, color: 'var(--text-2)', margin: 0, lineHeight: 1.5 }}>
            <strong>What unlocks what:</strong> this step → private free events.
            Full verification (ID photo, checked against Home Affairs) → paid events
            and public listings in the TicketHawk feed.
          </p>
        </div>
      </div>
    </div>
  );
}

const lbl: React.CSSProperties = { display: 'block', fontSize: 12.5, fontWeight: 600, color: 'var(--text)', marginBottom: 4 };
const inp: React.CSSProperties = { width: '100%', padding: '9px 11px', border: '1px solid var(--border)', borderRadius: 6, fontSize: 14 };