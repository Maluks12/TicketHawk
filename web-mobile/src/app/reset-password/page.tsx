'use client';

import { Suspense, useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { api } from '@/lib/api';
import { useAppTheme } from '@/themes/colors';

/**
 * /reset-password
 *
 * Two modes, decided by the URL:
 *   with ?token=…   → "set a new password" form (arrived from an email link:
 *                     forgot-password OR an organiser handover invite)
 *   without a token → "request a reset link" form (typed the URL / linked
 *                     from login)
 *
 * Backend contract:
 *   POST /v1/auth/reset_password_request { email }
 *   POST /v1/auth/reset_password         { token, new_password }
 *   new_password: ≥8 chars, 1 uppercase, 1 digit, 1 special (strong_password)
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

/** Mirrors the backend's strong_password validator so users get instant feedback. */
function passwordProblem(pw: string): string | null {
  if (pw.length < 8) return 'Must be at least 8 characters';
  if (!/[A-Z]/.test(pw)) return 'Must contain an uppercase letter';
  if (!/[0-9]/.test(pw)) return 'Must contain a digit';
  if (!/[^A-Za-z0-9]/.test(pw)) return 'Must contain a special character';
  if (pw.length > 128) return 'Must be at most 128 characters';
  return null;
}

function ResetPasswordInner() {
  const router = useRouter();
  const params = useSearchParams();
  const { isDark, colors } = useAppTheme();

  const token = useMemo(() => params.get('token')?.trim() ?? '', [params]);
  const mode: 'set' | 'request' = token ? 'set' : 'request';

  const [loading, setLoading] = useState(false);
  const [globalErr, setGlobalErr] = useState<string | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [done, setDone] = useState(false);

  // set mode
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  // request mode
  const [email, setEmail] = useState('');

  const clearErr = (k: string) =>
    setErrors((e) => { const n = { ...e }; delete n[k]; return n; });

  const handleSet = async () => {
    const errs: Record<string, string> = {};
    const pwErr = passwordProblem(password);
    if (pwErr) errs.password = pwErr;
    if (confirm !== password) errs.confirm = 'Passwords do not match';
    setErrors(errs);
    setGlobalErr(null);
    if (Object.keys(errs).length) return;

    setLoading(true);
    try {
      await api.post('/v1/auth/reset_password', { token, new_password: password });
      setDone(true);
    } catch (err: any) {
      const data = err?.response?.data;
      if (data?.details?.new_password) {
        setErrors({ password: data.details.new_password });
      } else if (data?.details?.token || err?.response?.status === 400 || err?.response?.status === 401) {
        setGlobalErr(data?.error && data.error !== 'Validation failed'
          ? data.error
          : 'This reset link is invalid or has expired. Request a new one below.');
      } else {
        setGlobalErr('Something went wrong. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  };

  const handleRequest = async () => {
    const errs: Record<string, string> = {};
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) errs.email = 'Enter a valid email address';
    setErrors(errs);
    setGlobalErr(null);
    if (Object.keys(errs).length) return;

    setLoading(true);
    try {
      await api.post('/v1/auth/reset_password_request', { email });
      setDone(true); // backend always answers the same way — so do we
    } catch (err: any) {
      // Rate limited (3/IP) or transient failure
      setGlobalErr(err?.response?.status === 429
        ? 'Too many requests. Please wait a few minutes and try again.'
        : 'Something went wrong. Please try again.');
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

        {/* ── Success states ─────────────────────────────────────────────── */}
        {done && mode === 'set' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14, alignItems: 'center', textAlign: 'center' }}>
            <div style={{ width: 56, height: 56, borderRadius: 28, background: 'rgba(46,204,113,0.15)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 26 }}>✓</div>
            <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>Password updated</h1>
            <p style={{ margin: 0, fontSize: 14, color: colors.subText }}>
              Your password has been changed. You can now log in with it.
            </p>
            <button onClick={() => router.push('/auth/login')} style={primaryBtn(false)}>Go to login</button>
          </div>
        )}

        {done && mode === 'request' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14, alignItems: 'center', textAlign: 'center' }}>
            <div style={{ width: 56, height: 56, borderRadius: 28, background: 'rgba(46,204,113,0.15)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 26 }}>✉️</div>
            <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>Check your email</h1>
            <p style={{ margin: 0, fontSize: 14, color: colors.subText }}>
              If an account exists for <strong style={{ color: colors.text }}>{email}</strong>, we&apos;ve sent a link to reset your password. The link expires in 30 minutes.
            </p>
            <button onClick={() => router.push('/auth/login')} style={primaryBtn(false)}>Back to login</button>
          </div>
        )}

        {/* ── Set-new-password form ──────────────────────────────────────── */}
        {!done && mode === 'set' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>Set your password</h1>
            <p style={{ margin: '0 0 4px', fontSize: 13, color: colors.subText, lineHeight: 1.5 }}>
              Choose a new password for your TicketHawk account. It needs at least 8 characters, an uppercase letter, a digit, and a special character.
            </p>

            {globalErr && (
              <div style={{ background: 'rgba(255,60,110,0.1)', border: '1px solid rgba(255,60,110,0.3)', borderRadius: 10, padding: '10px 14px', display: 'flex', alignItems: 'center', gap: 8 }}>
                <span style={{ fontSize: 15 }}>⚠️</span>
                <span style={{ color: '#ff3c6e', fontSize: 13, fontWeight: 600 }}>{globalErr}</span>
              </div>
            )}

            <Field placeholder="New password" value={password} onChange={(v) => { setPassword(v); clearErr('password'); }} type="password" error={errors.password} autoComplete="new-password" />
            <Field placeholder="Confirm new password" value={confirm} onChange={(v) => { setConfirm(v); clearErr('confirm'); }} type="password" error={errors.confirm} autoComplete="new-password" />

            <button onClick={handleSet} disabled={loading} style={primaryBtn(loading)}>
              {loading ? 'Saving…' : 'Set password'}
            </button>

            {globalErr && (
              <p style={{ textAlign: 'center', fontSize: 13, color: colors.subText, margin: 0 }}>
                Link expired?{' '}
                <span onClick={() => router.push('/reset-password')} style={{ color: '#ff2d55', fontWeight: 700, cursor: 'pointer' }}>Request a new one</span>
              </p>
            )}
          </div>
        )}

        {/* ── Request-a-link form ────────────────────────────────────────── */}
        {!done && mode === 'request' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>Reset your password</h1>
            <p style={{ margin: '0 0 4px', fontSize: 13, color: colors.subText, lineHeight: 1.5 }}>
              Enter the email address for your account and we&apos;ll send you a link to reset your password.
            </p>

            {globalErr && (
              <div style={{ background: 'rgba(255,60,110,0.1)', border: '1px solid rgba(255,60,110,0.3)', borderRadius: 10, padding: '10px 14px', display: 'flex', alignItems: 'center', gap: 8 }}>
                <span style={{ fontSize: 15 }}>⚠️</span>
                <span style={{ color: '#ff3c6e', fontSize: 13, fontWeight: 600 }}>{globalErr}</span>
              </div>
            )}

            <Field placeholder="Email address" value={email} onChange={(v) => { setEmail(v); clearErr('email'); }} type="email" error={errors.email} autoComplete="email" />

            <button onClick={handleRequest} disabled={loading} style={primaryBtn(loading)}>
              {loading ? 'Sending…' : 'Send reset link'}
            </button>

            <p style={{ textAlign: 'center', fontSize: 13, color: colors.subText, margin: 0 }}>
              Remembered it?{' '}
              <span onClick={() => router.push('/auth/login')} style={{ color: '#ff2d55', fontWeight: 700, cursor: 'pointer' }}>Log in</span>
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

// useSearchParams requires a Suspense boundary in the App Router.
export default function ResetPasswordPage() {
  return (
    <Suspense fallback={null}>
      <ResetPasswordInner />
    </Suspense>
  );
}