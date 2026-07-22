'use client';
import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useOrgAuth } from '../../../store/authStore';
import { api } from '../../../lib/api';
import {
  validateEmail,
  validatePassword,
  validatePasswordMatch,
  validateRequired,
  firstError,
} from '../../../lib/validation';

type Mode = 'login' | 'register';

export default function LoginPage() {
  const router = useRouter();
  const { login, user, hydrate, isLoading } = useOrgAuth();
  const [mode, setMode] = useState<Mode>('login');

  // Login
  const [email,       setEmail]       = useState('');
  const [password,    setPassword]    = useState('');
  const [showPass,    setShowPass]    = useState(false);

  // Register
  const [firstName,        setFirstName]        = useState('');
  const [lastName,         setLastName]          = useState('');
  const [regEmail,         setRegEmail]          = useState('');
  const [regPassword,      setRegPassword]       = useState('');
  const [confirmPassword,  setConfirmPassword]   = useState('');
  const [phoneNumber,      setPhoneNumber]       = useState('');
  const [dob,              setDob]               = useState('');
  const [showRegPass,      setShowRegPass]       = useState(false);
  const [showConfirmPass,  setShowConfirmPass]   = useState(false);

  const [error,      setError]      = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => { hydrate(); }, [hydrate]);
  useEffect(() => {
    if (isLoading || !user) return;
    if (user.role === 'organizer' || user.role === 'admin') {
      router.replace('/dashboard');
    } else {
      router.replace('/onboarding');
    }
  }, [user, isLoading, router]);

  // ── Login ──────────────────────────────────────────────────────────────────
  async function handleLogin(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const err = firstError({
      email:    validateEmail(email),
      password: validateRequired(password, 'Password'),
    });
    if (err) { setError(err); return; }

    setError(''); setSubmitting(true);
    try {
      const { role } = await login(email, password);
      // Organisers/admins go straight to the dashboard. Everyone else goes to
      // onboarding, where basic KYC (name + SA ID) grants the organiser role
      // and unlocks private free events. No more auto-register-organizer.
      if (role === 'organizer' || role === 'admin') {
        router.replace('/dashboard');
      } else {
        router.replace('/onboarding');
      }
    } catch (e: unknown) {
      setError(
        (e as { response?: { data?: { error?: string } } })?.response?.data?.error
        ?? (e instanceof Error ? e.message : 'Login failed')
      );
    } finally { setSubmitting(false); }
  }

  // ── Register ───────────────────────────────────────────────────────────────
  async function handleRegister(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const err = firstError({
      firstName: validateRequired(firstName, 'First name'),
      lastName:  validateRequired(lastName, 'Last name'),
      email:     validateEmail(regEmail),
      phone:     validateRequired(phoneNumber, 'Phone number'),
      dob:       validateRequired(dob, 'Date of birth'),
      password:  validatePassword(regPassword),
      confirm:   validatePasswordMatch(regPassword, confirmPassword),
    });
    if (err) { setError(err); return; }

    setError(''); setSubmitting(true);
    try {
      await api.post('/auth/register', {
        first_name: firstName, last_name: lastName,
        email: regEmail, password: regPassword,
        phone_number: phoneNumber.trim(),
        date_of_birth: new Date(dob).toISOString(),
      });
      await login(regEmail, regPassword);
      // New account is a plain user — onboarding turns them into an organiser.
      router.replace('/onboarding');
    } catch (e: unknown) {
      setError(
        (e as { response?: { data?: { error?: string } } })?.response?.data?.error
        ?? (e instanceof Error ? e.message : 'Registration failed')
      );
    } finally { setSubmitting(false); }
  }

  function switchMode(m: Mode) { setMode(m); setError(''); }

  return (
    <div style={{
      minHeight: '100vh', background: '#f0f2f5',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      padding: '24px 16px',
      fontFamily: "'Figtree', -apple-system, BlinkMacSystemFont, sans-serif",
    }}>
      <style>{`
        .org-input {
          width: 100%; padding: 10px 12px; border: 1px solid #ccd0d5;
          border-radius: 6px; font-size: 15px; color: #1c1e21;
          background: #fff; outline: none; box-sizing: border-box;
          font-family: inherit; transition: border-color .15s, box-shadow .15s;
        }
        .org-input:focus {
          border-color: #1877f2;
          box-shadow: 0 0 0 2px rgba(24,119,242,.15);
        }
        .org-input::placeholder { color: #bec3c9; }
        .pass-wrap { position: relative; }
        .pass-wrap .org-input { padding-right: 42px; }
        .eye-btn {
          position: absolute; right: 0; top: 0; bottom: 0;
          width: 40px; background: none; border: none; cursor: pointer;
          display: flex; align-items: center; justify-content: center;
          color: #8a8d91; border-radius: 0 6px 6px 0;
        }
        .eye-btn:hover { color: #1c1e21; }
      `}</style>

      <div style={{ width: '100%', maxWidth: 400 }}>

        {/* Logo */}
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <div style={{
            width: 60, height: 60, borderRadius: 14, background: '#0a0a0a',
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center', marginBottom: 12,
          }}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="/brand-logo.png" alt="TicketHawk" width={44} height={44} />
          </div>
          <div style={{ fontSize: 24, fontWeight: 800, letterSpacing: 0.5, color: '#1c1e21' }}>
            TICKET<span style={{ color: '#FF2D55' }}>HAWK</span>
          </div>
          <div style={{ fontSize: 14, color: '#606770', marginTop: 4 }}>Organiser Dashboard</div>
        </div>

        {/* Card */}
        <div style={{ background: '#fff', borderRadius: 8, border: '1px solid #dddfe2', overflow: 'hidden' }}>

          {/* Tab bar */}
          <div style={{ display: 'flex', borderBottom: '1px solid #dddfe2', paddingLeft: 8 }}>
            {(['login', 'register'] as const).map(m => (
              <button key={m} onClick={() => switchMode(m)} style={{
                padding: '12px 16px', fontSize: 14, fontWeight: 700,
                border: 'none', background: 'none', cursor: 'pointer',
                color: mode === m ? '#1877f2' : '#606770',
                borderBottom: `2px solid ${mode === m ? '#1877f2' : 'transparent'}`,
                marginBottom: -1, fontFamily: 'inherit',
              }}>
                {m === 'login' ? 'Log in' : 'Create account'}
              </button>
            ))}
          </div>

          <div style={{ padding: '20px 24px 24px' }}>
            {error && (
              <div style={{
                background: '#fff5f5', border: '1px solid #fca5a5', borderRadius: 6,
                padding: '10px 12px', color: '#b91c1c', fontSize: 13, marginBottom: 16,
              }}>
                {error}
              </div>
            )}

            {/* ── Log in ── */}
            {mode === 'login' && (
              <form onSubmit={handleLogin} noValidate>
                <Field label="Email address">
                  <input className="org-input" type="email" value={email}
                    autoFocus autoComplete="email"
                    onChange={e => setEmail(e.target.value)}
                    placeholder="you@example.com" />
                </Field>
                <Field label="Password">
                  <div className="pass-wrap">
                    <input className="org-input"
                      type={showPass ? 'text' : 'password'}
                      value={password}
                      autoComplete="current-password"
                      onChange={e => setPassword(e.target.value)}
                      placeholder="••••••••" />
                    <button type="button" className="eye-btn"
                      onClick={() => setShowPass(p => !p)}
                      aria-label={showPass ? 'Hide password' : 'Show password'}>
                      <EyeIcon show={showPass} />
                    </button>
                  </div>
                </Field>
                <SubmitBtn loading={submitting} label="Log in" loadingLabel="Logging in…" />
                <div style={{ textAlign: 'center', marginTop: 16, fontSize: 13, color: '#606770' }}>
                  Don't have an account?{' '}
                  <InlineLink onClick={() => switchMode('register')} label="Create one" />
                </div>
              </form>
            )}

            {/* ── Register ── */}
            {mode === 'register' && (
              <form onSubmit={handleRegister} noValidate>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                  <Field label="First name">
                    <input className="org-input" value={firstName}
                      autoFocus autoComplete="given-name"
                      onChange={e => setFirstName(e.target.value)}
                      placeholder="Jane" />
                  </Field>
                  <Field label="Last name">
                    <input className="org-input" value={lastName}
                      autoComplete="family-name"
                      onChange={e => setLastName(e.target.value)}
                      placeholder="Doe" />
                  </Field>
                </div>
                <Field label="Email address">
                  <input className="org-input" type="email" value={regEmail}
                    autoComplete="email"
                    onChange={e => setRegEmail(e.target.value)}
                    placeholder="you@example.com" />
                </Field>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                  <Field label="Phone number">
                    <input className="org-input" type="tel" value={phoneNumber}
                      autoComplete="tel"
                      onChange={e => setPhoneNumber(e.target.value)}
                      placeholder="+27 82 123 4567" />
                  </Field>
                  <Field label="Date of birth">
                    <input className="org-input" type="date" value={dob}
                      max={new Date().toISOString().split('T')[0]}
                      onChange={e => setDob(e.target.value)} />
                  </Field>
                </div>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                  <Field label="Password">
                    <div className="pass-wrap">
                      <input className="org-input"
                        type={showRegPass ? 'text' : 'password'}
                        value={regPassword}
                        autoComplete="new-password"
                        onChange={e => setRegPassword(e.target.value)}
                        placeholder="Min 8 chars" />
                      <button type="button" className="eye-btn"
                        onClick={() => setShowRegPass(p => !p)}
                        aria-label={showRegPass ? 'Hide' : 'Show'}>
                        <EyeIcon show={showRegPass} />
                      </button>
                    </div>
                  </Field>
                  <Field label="Confirm">
                    <div className="pass-wrap">
                      <input className="org-input"
                        type={showConfirmPass ? 'text' : 'password'}
                        value={confirmPassword}
                        autoComplete="new-password"
                        onChange={e => setConfirmPassword(e.target.value)}
                        placeholder="••••••••" />
                      <button type="button" className="eye-btn"
                        onClick={() => setShowConfirmPass(p => !p)}
                        aria-label={showConfirmPass ? 'Hide' : 'Show'}>
                        <EyeIcon show={showConfirmPass} />
                      </button>
                    </div>
                  </Field>
                </div>
                <SubmitBtn loading={submitting} label="Create account" loadingLabel="Creating…" />
                <div style={{ textAlign: 'center', marginTop: 16, fontSize: 13, color: '#606770' }}>
                  Already have an account?{' '}
                  <InlineLink onClick={() => switchMode('login')} label="Log in" />
                </div>
              </form>
            )}
          </div>
        </div>

        <p style={{ textAlign: 'center', fontSize: 12, color: '#8a8d91', marginTop: 14 }}>
          Use the same email and password as your EventsApp account.
        </p>
      </div>
    </div>
  );
}

// ── Eye icon — open = visible, closed = hidden ─────────────────────────────────
function EyeIcon({ show }: { show: boolean }) {
  return show ? (
    <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
      <path d="M10 12a2 2 0 100-4 2 2 0 000 4z"/>
      <path fillRule="evenodd" d="M.458 10C1.732 5.943 5.522 3 10 3s8.268 2.943 9.542 7c-1.274 4.057-5.064 7-9.542 7S1.732 14.057.458 10zM14 10a4 4 0 11-8 0 4 4 0 018 0z" clipRule="evenodd"/>
    </svg>
  ) : (
    <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
      <path fillRule="evenodd" d="M3.707 2.293a1 1 0 00-1.414 1.414l14 14a1 1 0 001.414-1.414l-1.473-1.473A10.014 10.014 0 0019.542 10C18.268 5.943 14.478 3 10 3a9.958 9.958 0 00-4.512 1.074l-1.78-1.781zm4.261 4.26l1.514 1.515a2.003 2.003 0 012.45 2.450l1.514 1.514a4 4 0 00-5.478-5.478z" clipRule="evenodd"/>
      <path d="M12.454 16.697L9.75 13.992a4 4 0 01-3.742-3.741L2.335 6.578A9.98 9.98 0 00.458 10c1.274 4.057 5.064 7 9.542 7 .847 0 1.669-.105 2.454-.303z"/>
    </svg>
  );
}

// ── Helpers ────────────────────────────────────────────────────────────────────
function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ marginBottom: 12 }}>
      <label style={{ display: 'block', fontSize: 13, fontWeight: 600, color: '#1c1e21', marginBottom: 5 }}>
        {label}
      </label>
      {children}
    </div>
  );
}

function SubmitBtn({ loading, label, loadingLabel }: {
  loading: boolean; label: string; loadingLabel: string;
}) {
  return (
    <button type="submit" disabled={loading} style={{
      width: '100%', padding: '11px', borderRadius: 6, marginTop: 4,
      background: loading ? '#b8d9ff' : '#1877f2',
      border: 'none', color: '#fff', fontSize: 15, fontWeight: 700,
      cursor: loading ? 'not-allowed' : 'pointer',
      fontFamily: 'inherit', transition: 'background .15s',
    }}>
      {loading ? loadingLabel : label}
    </button>
  );
}

function InlineLink({ onClick, label }: { onClick: () => void; label: string }) {
  return (
    <button type="button" onClick={onClick} style={{
      background: 'none', border: 'none', color: '#1877f2', fontWeight: 700,
      cursor: 'pointer', fontSize: 13, padding: 0, fontFamily: 'inherit',
    }}>
      {label}
    </button>
  );
}