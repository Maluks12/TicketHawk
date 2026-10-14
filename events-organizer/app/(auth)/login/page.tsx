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
import PasswordChecklist from '../../../components/passwordChecklist';
import { ThemeToggle } from '../../../lib/theme';
import { ORGANISER_LEGAL_DOCS, legalLinkProps, CURRENT_LEGAL_VERSION } from '../../../lib/legalLinks';

type Mode = 'login' | 'register';

// One canonical form, matching the consumer apps (web-mobile, web-desktop and
// EventsApp all trim + lowercase before register and login). The dashboard
// used to send the address exactly as typed, so "Jane@Gmail.com" was stored
// with capitals and every TicketHawk login — which sends "jane@gmail.com" —
// came back "Invalid credentials". The backend now matches case-insensitively
// too; this keeps new rows clean at the source.
const normEmail = (e: string) => e.trim().toLowerCase();

export default function LoginPage() {
  const router = useRouter();
  const { login, logout, user, hydrate, isLoading } = useOrgAuth();
  const [mode, setMode] = useState<Mode>('login');
  // Set the moment a login/register succeeds and we hand off to the router,
  // so the "you're already signed in" notice below doesn't flash up for the
  // account that was just signed in while the next page loads.
  const [navigating, setNavigating] = useState(false);

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
  const [showRegPass,      setShowRegPass]       = useState(false);
  const [showConfirmPass,  setShowConfirmPass]   = useState(false);
  // Explicit, unticked by default. A pre-ticked box is not consent in any
  // jurisdiction that matters here, and POPIA in particular expects the act to
  // be voluntary and specific.
  const [acceptedLegal,    setAcceptedLegal]     = useState(false);

  const [error,      setError]      = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => { hydrate(); }, [hydrate]);

  // The landing page's "Start selling" CTAs link to /login?mode=register so the
  // signup tab is already open on arrival — sending a would-be signup to a login
  // form is a pointless extra click. Read from window.location rather than
  // useSearchParams(): the latter forces this client component to need a
  // Suspense boundary at build time, and this runs client-side only anyway.
  useEffect(() => {
    if (typeof window === 'undefined') return;
    if (new URLSearchParams(window.location.search).get('mode') === 'register') {
      setMode('register');
    }
  }, []);
  // A signed-in organiser/admin has nothing to do here — straight to the
  // dashboard, as before.
  //
  // A signed-in account WITHOUT an organiser profile is no longer bounced to
  // /onboarding. That auto-redirect made /login unreachable: anyone who had
  // ever created an account in this browser was pinned to "Complete your
  // profile" with no way back to the login form to use a different account.
  // They now see the login form, with a notice offering to continue setup or
  // sign out (below).
  useEffect(() => {
    if (isLoading || !user) return;
    if (user.role === 'organizer' || user.role === 'admin') {
      router.replace('/dashboard');
    }
  }, [user, isLoading, router]);

  const pendingProfile =
    !isLoading && !!user && !navigating && !submitting
    && user.role !== 'organizer' && user.role !== 'admin';

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
      // Switching accounts: end the session already in this browser first, so
      // its refresh token is revoked server-side rather than orphaned.
      if (user) await logout();
      const { role } = await login(normEmail(email), password);
      setNavigating(true);
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
      password:  validatePassword(regPassword),
      confirm:   validatePasswordMatch(regPassword, confirmPassword),
    });
    if (err) { setError(err); return; }
    if (!acceptedLegal) {
      setError('Please accept the Terms of Service, Privacy Policy and Organiser Agreement to continue.');
      return;
    }

    setError(''); setSubmitting(true);
    try {
      if (user) await logout();
      const em = normEmail(regEmail);
      await api.post('/auth/register', {
        first_name: firstName.trim(), last_name: lastName.trim(),
        email: em, password: regPassword,
        phone_number: phoneNumber.trim(),
        // Recorded at the moment of consent, with the version accepted. The
        // version is what lets a later material change re-prompt: without it,
        // "accepted" is permanent and a new fee model could never be
        // re-consented. Ignored by older backends, which is harmless.
        accepted_legal_version: CURRENT_LEGAL_VERSION,
        accepted_legal_at: new Date().toISOString(),
      });
      await login(em, regPassword);
      setNavigating(true);
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
      minHeight: '100vh', background: 'var(--bg)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      padding: '24px 16px',
      fontFamily: 'var(--font)',
      position: 'relative',
    }}>
      {/* Theme switch — the dashboard's lives in the top bar, but someone who
          has pinned light mode still needs it before they're signed in. */}
      <div style={{ position: 'absolute', top: 20, right: 20 }}>
        <ThemeToggle />
      </div>
      <style>{`
        .org-input {
          width: 100%; padding: 10px 12px; border: 1px solid var(--border-2);
          border-radius: var(--r); font-size: 15px; color: var(--text);
          background: var(--surface); outline: none; box-sizing: border-box;
          font-family: inherit; transition: border-color .15s, box-shadow .15s;
        }
        .org-input:focus {
          border-color: var(--accent);
          box-shadow: 0 0 0 3px var(--accent-ring);
        }
        .org-input::placeholder { color: var(--text-faint); }
        .pass-wrap { position: relative; }
        .pass-wrap .org-input { padding-right: 42px; }
        .eye-btn {
          position: absolute; right: 0; top: 0; bottom: 0;
          width: 40px; background: none; border: none; cursor: pointer;
          display: flex; align-items: center; justify-content: center;
          color: var(--text-3); border-radius: 0 var(--r) var(--r) 0;
        }
        .eye-btn:hover { color: var(--text); }
      `}</style>

      <div style={{ width: '100%', maxWidth: 400 }}>

        {/* Logo — the mark stands on its own.
            It used to sit inside a 60x60 --accent-deep rounded square, which
            put a purple tile behind a logo that already has its own gold disc,
            reading as a second, competing container. Without the tile the mark
            needs to carry the space itself, so it steps up from 44px to 64px. */}
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <div style={{ marginBottom: 12 }}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="/brand-logo.png" alt="TicketHawk" width={64} height={64}
              style={{ display: 'inline-block' }} />
          </div>
          <div style={{ fontSize: 24, fontWeight: 800, letterSpacing: 0.5, color: 'var(--text)' }}>
            TICKET<span style={{ color: 'var(--brand)' }}>HAWK</span>
          </div>
          <div style={{ fontSize: 14, color: 'var(--text-2)', marginTop: 4 }}>Organiser Dashboard</div>
        </div>

        {/* ── Signed in, profile not finished ─────────────────────────────
            Replaces the old hard redirect to /onboarding. The person decides:
            carry on with this account, or sign out and use another — and the
            login form stays usable underneath either way (logging in or
            creating an account from here signs the old session out first). */}
        {pendingProfile && user && (
          <div style={{
            background: 'var(--surface)', border: '1px solid var(--border)',
            borderRadius: 'var(--r-xl)', padding: '14px 16px', marginBottom: 14,
          }}>
            <div style={{ fontSize: 13.5, color: 'var(--text)', lineHeight: 1.5 }}>
              You&apos;re signed in as{' '}
              <strong style={{ overflowWrap: 'anywhere' }}>{user.email || 'an account'}</strong>
              , but its organiser profile isn&apos;t complete yet.
            </div>
            <div style={{ display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' }}>
              <button type="button" onClick={() => router.push('/onboarding')} style={{
                flex: '1 1 140px', padding: '9px 12px', borderRadius: 'var(--r)',
                background: 'var(--accent)', color: 'var(--on-accent)', border: 'none',
                fontSize: 13.5, fontWeight: 700, cursor: 'pointer', fontFamily: 'inherit',
              }}>
                Complete profile
              </button>
              <button type="button" onClick={() => { void logout(); }} style={{
                flex: '1 1 140px', padding: '9px 12px', borderRadius: 'var(--r)',
                background: 'transparent', color: 'var(--text)', border: '1px solid var(--border-2)',
                fontSize: 13.5, fontWeight: 600, cursor: 'pointer', fontFamily: 'inherit',
              }}>
                Sign out
              </button>
            </div>
          </div>
        )}

        {/* Card */}
        <div style={{ background: 'var(--surface)', borderRadius: 'var(--r-xl)', border: '1px solid var(--border)', overflow: 'hidden' }}>

          {/* Tab bar */}
          <div style={{ display: 'flex', borderBottom: '1px solid var(--border)', paddingLeft: 8 }}>
            {(['login', 'register'] as const).map(m => (
              <button key={m} onClick={() => switchMode(m)} style={{
                padding: '12px 16px', fontSize: 14, fontWeight: 700,
                border: 'none', background: 'none', cursor: 'pointer',
                color: mode === m ? 'var(--accent)' : 'var(--text-2)',
                borderBottom: `2px solid ${mode === m ? 'var(--accent)' : 'transparent'}`,
                marginBottom: -1, fontFamily: 'inherit',
              }}>
                {m === 'login' ? 'Log in' : 'Create account'}
              </button>
            ))}
          </div>

          <div style={{ padding: '20px 24px 24px' }}>
            {error && (
              <div style={{
                background: 'var(--danger-bg)', border: '1px solid var(--danger-border)', borderRadius: 6,
                padding: '10px 12px', color: 'var(--danger-fg)', fontSize: 13, marginBottom: 16,
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
                {/* The /reset-password route already existed but nothing on
                    this screen linked to it, so a locked-out organiser had no
                    route back in short of guessing the URL. */}
                <div style={{ textAlign: 'right', marginTop: -6, marginBottom: 16 }}>
                  <a href="/reset-password" style={{
                    fontSize: 12.5, color: 'var(--text-3)', textDecoration: 'none',
                  }}>
                    Forgot password?
                  </a>
                </div>
                <SubmitBtn loading={submitting} label="Log in" loadingLabel="Logging in…" />
                <div style={{ textAlign: 'center', marginTop: 16, fontSize: 13, color: 'var(--text-2)' }}>
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
                {/* Date of birth removed. The backend dropped it from
                    CreateUserRequest deliberately — it was never read by the
                    handler or the repository, so every signup since launch
                    collected a birth date and discarded it. Age belongs at
                    checkout for age-restricted events, where it is actually
                    load-bearing, not as a seventh field on a signup form.
                    users.date_of_birth stays nullable and still works for
                    accounts that set it via profile edit. Phone now spans
                    the row on its own. */}
                <Field label="Phone number">
                  <input className="org-input" type="tel" value={phoneNumber}
                    autoComplete="tel"
                    onChange={e => setPhoneNumber(e.target.value)}
                    placeholder="+27 82 123 4567" />
                </Field>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                  <Field label="Password">
                    <div className="pass-wrap">
                      <input className="org-input"
                        type={showRegPass ? 'text' : 'password'}
                        value={regPassword}
                        autoComplete="new-password"
                        onChange={e => setRegPassword(e.target.value)}
                        placeholder="••••••••" />
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

                {/* ── Live password requirements ──────────────────────────
                    The backend enforces strong_password (8+, uppercase,
                    digit, special) but this form only ever checked length,
                    so a weak password sailed past the client and came back
                    as an opaque 400. Showing the rules up front — and
                    ticking them as they're met — means the organiser knows
                    what's wanted before they submit, not after. Same
                    behaviour as the consumer web-mobile signup. */}
                <PasswordChecklist password={regPassword} />

                {/* ── Legal acceptance ────────────────────────────────────
                    One checkbox covering all four documents, each individually
                    linked. Grouping them is what keeps the form usable; linking
                    each one separately is what keeps the consent informed —
                    "I agree to the terms" with no way to read them is not
                    meaningfully agreement. Unticked by default, and the submit
                    button is disabled until it's ticked, so the state of the
                    form always matches what will happen. */}
                <div style={{
                  display: 'flex', alignItems: 'flex-start', gap: 10,
                  padding: '12px 14px', marginBottom: 16,
                  background: 'var(--surface-alpha)',
                  border: `1px solid ${acceptedLegal ? 'var(--accent-border)' : 'var(--border)'}`,
                  borderRadius: 'var(--r-lg)',
                  transition: 'border-color .15s ease',
                }}>
                  <input
                    id="accept-legal"
                    type="checkbox"
                    checked={acceptedLegal}
                    onChange={e => setAcceptedLegal(e.target.checked)}
                    style={{
                      width: 16, height: 16, marginTop: 2,
                      accentColor: 'var(--accent-deep)', cursor: 'pointer', flexShrink: 0,
                    }}
                  />
                  <label htmlFor="accept-legal" style={{
                    fontSize: 12.5, lineHeight: 1.6, color: 'var(--text-2)', cursor: 'pointer',
                  }}>
                    I have read and agree to the{' '}
                    {ORGANISER_LEGAL_DOCS.map((d, i) => (
                      <span key={d.key}>
                        <a
                          {...legalLinkProps(d.key)}
                          onClick={e => e.stopPropagation()}
                          style={{ color: 'var(--accent)', textDecoration: 'underline' }}
                        >
                          {d.label}
                        </a>
                        {i < ORGANISER_LEGAL_DOCS.length - 2 ? ', '
                          : i === ORGANISER_LEGAL_DOCS.length - 2 ? ' and ' : ''}
                      </span>
                    ))}
                    .
                  </label>
                </div>

                <SubmitBtn
                  loading={submitting}
                  disabled={!acceptedLegal}
                  label="Create account"
                  loadingLabel="Creating…"
                />
                <div style={{ textAlign: 'center', marginTop: 16, fontSize: 13, color: 'var(--text-2)' }}>
                  Already have an account?{' '}
                  <InlineLink onClick={() => switchMode('login')} label="Log in" />
                </div>
              </form>
            )}
          </div>
        </div>

        <p style={{ textAlign: 'center', fontSize: 12, color: 'var(--text-3)', marginTop: 14 }}>
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
      <label style={{ display: 'block', fontSize: 13, fontWeight: 600, color: 'var(--text)', marginBottom: 5 }}>
        {label}
      </label>
      {children}
    </div>
  );
}

function SubmitBtn({ loading, label, loadingLabel, disabled = false }: {
  loading: boolean; label: string; loadingLabel: string;
  /** Blocked for a reason other than being in flight — e.g. legal not accepted. */
  disabled?: boolean;
}) {
  const off = loading || disabled;
  return (
    <button type="submit" disabled={off} style={{
      width: '100%', padding: '11px', borderRadius: 'var(--r)', marginTop: 4,
      background: off ? 'var(--accent-border)' : 'var(--accent)',
      // --on-accent, NOT '#fff'. The accent is near-black in the light theme
      // and near-white in dark, so a hardcoded white label rendered white on
      // a near-white button — the label vanished entirely in dark mode.
      // The token flips with the theme; the literal could not.
      border: 'none', color: 'var(--on-accent)', fontSize: 15, fontWeight: 700,
      cursor: off ? 'not-allowed' : 'pointer',
      opacity: disabled && !loading ? 0.7 : 1,
      fontFamily: 'inherit', transition: 'background .15s, opacity .15s',
    }}>
      {loading ? loadingLabel : label}
    </button>
  );
}

function InlineLink({ onClick, label }: { onClick: () => void; label: string }) {
  return (
    <button type="button" onClick={onClick} style={{
      background: 'none', border: 'none', color: 'var(--mark)', fontWeight: 700,
      cursor: 'pointer', fontSize: 13, padding: 0, fontFamily: 'inherit',
    }}>
      {label}
    </button>
  );
}