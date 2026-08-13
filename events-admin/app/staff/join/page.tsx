'use client';
import { Suspense, useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { api } from '../../../lib/api';

// Invite acceptance. Two factors: the emailed link, and a code that goes to the
// administrator who invited you — never to this page. You have to ask them.

function JoinInner() {
  const router = useRouter();
  const params = useSearchParams();
  const token = params.get('token') ?? '';

  const [invite, setInvite] = useState<{ email: string; staff_role: string } | null>(null);
  const [rawStep, setStep] = useState<'loading' | 'invalid' | 'profile' | 'code' | 'done'>('loading');
  const step = !token ? 'invalid' : rawStep;
  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [otp, setOtp] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    // No synchronous branch: `step` starts at 'loading' and a missing token is
    // derived below rather than written here.
    if (!token) return;
    let alive = true;
    api.get('/staff/invite', { params: { token } })
      .then(({ data }) => {
        if (!alive) return;
        setInvite(data);
        setStep('profile');
      })
      .catch(() => { if (alive) setStep('invalid'); });
    return () => { alive = false; };
  }, [token]);

  async function requestCode() {
    if (password.length < 8) { setError('Password must be at least 8 characters'); return; }
    if (password !== confirm) { setError('Passwords do not match'); return; }
    setError(''); setBusy(true);
    try {
      await api.post('/staff/invite/start', { token });
      setStep('code');
    } catch {
      setError('Could not request a code. The link may have expired.');
    } finally {
      setBusy(false);
    }
  }

  async function complete() {
    setError(''); setBusy(true);
    try {
      await api.post('/staff/invite/complete', {
        token, otp, first_name: firstName, last_name: lastName, password,
      });
      setStep('done');
    } catch (e) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      setError(msg ?? 'Could not complete sign-up');
    } finally {
      setBusy(false);
    }
  }

  const input: React.CSSProperties = {
    width: '100%', padding: '11px 13px', fontSize: 14,
    border: '1px solid var(--border-2)', borderRadius: 'var(--r)',
    background: 'var(--surface-alpha)', color: 'var(--text)',
    fontFamily: 'inherit', boxSizing: 'border-box', outline: 'none',
  };

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--bg)', padding: 24,
    }}>
      <div style={{ width: '100%', maxWidth: 460 }}>
        <div style={{ textAlign: 'center', marginBottom: 22 }}>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/brand-logo.png" alt="" width={56} height={56}
            style={{ display: 'block', margin: '0 auto 12px' }} />
          <div style={{ fontSize: 20, fontWeight: 800, color: 'var(--text)' }}>
            TicketHawk Admin
          </div>
        </div>

        <div className="card">
          <div className="card-body" style={{ padding: 24 }}>
            {step === 'loading' && (
              <div style={{ textAlign: 'center', padding: 24 }}>
                <div className="spinner spinner-lg" style={{ margin: '0 auto' }} />
              </div>
            )}

            {step === 'invalid' && (
              <div style={{ textAlign: 'center' }}>
                <div style={{ fontSize: 17, fontWeight: 800, marginBottom: 8 }}>
                  This invite is no longer valid
                </div>
                <div style={{ fontSize: 13.5, color: 'var(--text-2)', lineHeight: 1.6 }}>
                  It may have expired, been revoked, or already been used. Ask an
                  administrator to send a new one.
                </div>
              </div>
            )}

            {step === 'profile' && invite && (
              <>
                <div style={{ fontSize: 17, fontWeight: 800, marginBottom: 4 }}>
                  Set up your account
                </div>
                <div style={{ fontSize: 13, color: 'var(--text-2)', marginBottom: 18 }}>
                  {invite.email} · {invite.staff_role.replace(/_/g, ' ')} access
                </div>

                {error && <div className="alert alert-danger" style={{ fontSize: 13 }}>{error}</div>}

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                  <div className="field">
                    <label>First name</label>
                    <input value={firstName} onChange={(e) => setFirstName(e.target.value)} style={input} />
                  </div>
                  <div className="field">
                    <label>Last name</label>
                    <input value={lastName} onChange={(e) => setLastName(e.target.value)} style={input} />
                  </div>
                </div>
                <div className="field">
                  <label>Password</label>
                  <input type="password" value={password}
                    onChange={(e) => setPassword(e.target.value)} style={input} />
                  <div className="field-hint">At least 8 characters</div>
                </div>
                <div className="field" style={{ marginBottom: 18 }}>
                  <label>Confirm password</label>
                  <input type="password" value={confirm}
                    onChange={(e) => setConfirm(e.target.value)} style={input} />
                </div>

                <button className="btn btn-primary btn-full" disabled={busy} onClick={requestCode}>
                  {busy ? 'Please wait…' : 'Continue'}
                </button>
              </>
            )}

            {step === 'code' && (
              <>
                <div style={{ fontSize: 17, fontWeight: 800, marginBottom: 4 }}>
                  Ask your administrator for the code
                </div>
                <div style={{ fontSize: 13.5, color: 'var(--text-2)', lineHeight: 1.6, marginBottom: 18 }}>
                  A 6-digit code has been sent to the administrator who invited
                  you — not to your inbox. Contact them directly for it.
                </div>

                {error && <div className="alert alert-danger" style={{ fontSize: 13 }}>{error}</div>}

                <div className="field" style={{ marginBottom: 18 }}>
                  <label>6-digit code</label>
                  <input value={otp} inputMode="numeric" maxLength={6}
                    onChange={(e) => setOtp(e.target.value.replace(/\D/g, ''))}
                    style={{
                      ...input, textAlign: 'center', fontSize: 24, letterSpacing: '.4em',
                      fontFamily: 'var(--font-mono)',
                    }} />
                </div>

                <button className="btn btn-primary btn-full"
                  disabled={busy || otp.length !== 6} onClick={complete}>
                  {busy ? 'Verifying…' : 'Create my account'}
                </button>
              </>
            )}

            {step === 'done' && (
              <div style={{ textAlign: 'center' }}>
                <div style={{ fontSize: 17, fontWeight: 800, marginBottom: 8 }}>
                  You&apos;re all set
                </div>
                <div style={{ fontSize: 13.5, color: 'var(--text-2)', lineHeight: 1.6, marginBottom: 18 }}>
                  Your account is ready. Sign in with the email and password you
                  just set.
                </div>
                <button className="btn btn-primary btn-full" onClick={() => router.replace('/login')}>
                  Go to sign in
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

export default function StaffJoinPage() {
  return (
    <Suspense fallback={<div className="page-loader"><div className="spinner spinner-lg" /></div>}>
      <JoinInner />
    </Suspense>
  );
}