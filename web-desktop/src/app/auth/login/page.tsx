'use client';

import { useState, useCallback, useEffect, useRef } from 'react';
import { CheckCircle2, Calendar, ChevronLeft, ChevronRight, AlertCircle, X } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';

type Mode = 'login' | 'register';

// Friendly labels for the error summary toast — `errors` is keyed by field
// name internally, but "regPassword: Required" reads better as "Password: Required".
const FIELD_LABELS: Record<string, string> = {
  loginEmail: 'Email', loginPassword: 'Password',
  firstName: 'First name', lastName: 'Last name',
  regEmail: 'Email', phone: 'Phone number', dob: 'Date of birth',
  regPassword: 'Password', confirmPass: 'Confirm password', agreed: 'Terms',
};

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

// ── DOB helpers: auto-slash formatting + calendar popover ────────────────────
// As the person types digits, slashes get inserted automatically after the
// day and month groups (DD/MM/YYYY). Deleting a slash also removes the digit
// right before it, so backspace feels natural instead of getting "stuck".
function formatDobInput(raw: string, prev: string): string {
  let digits = raw.replace(/\D/g, '').slice(0, 8);
  const prevDigits = prev.replace(/\D/g, '');
  if (raw.length < prev.length && digits.length === prevDigits.length) {
    digits = digits.slice(0, -1);
  }
  let out = digits.slice(0, 2);
  if (digits.length > 2) out += '/' + digits.slice(2, 4);
  if (digits.length > 4) out += '/' + digits.slice(4, 8);
  return out;
}

const DOB_WEEKDAY_LABELS = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'];
const DOB_MONTH_NAMES = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
];

function DobCalendarPopover({
  value, isDark, onSelect,
}: {
  value: string; isDark: boolean;
  onSelect: (day: number, month: number, year: number) => void;
}) {
  const [dd, mm, yyyy] = value.split('/');
  const parsedDay   = /^\d{1,2}$/.test(dd ?? '')   ? parseInt(dd, 10)   : null;
  const parsedMonth = /^\d{1,2}$/.test(mm ?? '')   ? parseInt(mm, 10) - 1 : null;
  const parsedYear  = /^\d{4}$/.test(yyyy ?? '')   ? parseInt(yyyy, 10) : null;

  const now = new Date();
  const [viewMonth, setViewMonth] = useState(parsedMonth ?? now.getMonth());
  const [viewYear,  setViewYear]  = useState(parsedYear  ?? now.getFullYear() - 18);

  const todayStr = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;

  const goPrev = () => { if (viewMonth === 0) { setViewYear((y) => y - 1); setViewMonth(11); } else setViewMonth((m) => m - 1); };
  const goNext = () => { if (viewMonth === 11) { setViewYear((y) => y + 1); setViewMonth(0); } else setViewMonth((m) => m + 1); };

  const daysInMonth     = new Date(viewYear, viewMonth + 1, 0).getDate();
  const firstDayOfMonth = new Date(viewYear, viewMonth, 1).getDay();
  const cells: (number | null)[] = Array(firstDayOfMonth).fill(null);
  for (let d = 1; d <= daysInMonth; d++) cells.push(d);

  const currentYear = now.getFullYear();
  const years = Array.from({ length: 101 }, (_, i) => currentYear - i);

  const selectStyle = {
    flex: 1, fontSize: 12.5, fontWeight: 700, borderRadius: 8, padding: '6px 6px',
    background: isDark ? '#1c1c1c' : '#f5f5f5', color: isDark ? '#fff' : '#0a0a0a',
    border: `1px solid ${isDark ? '#333' : '#e0e0e0'}`, outline: 'none', cursor: 'pointer',
  };

  return (
    <div
      style={{
        position: 'absolute', top: 'calc(100% + 8px)', left: 0, right: 0, zIndex: 50,
        background: isDark ? '#161616' : '#fff', border: `1px solid ${isDark ? '#2a2a2a' : '#e5e5e5'}`,
        borderRadius: 14, padding: 14, boxShadow: '0 12px 32px rgba(0,0,0,0.35)',
      }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      {/* Quick month / year jump — a plain month-by-month calendar would take
          forever to reach a birth year decades back. */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 10 }}>
        <select value={viewMonth} onChange={(e) => setViewMonth(Number(e.target.value))} style={selectStyle}>
          {DOB_MONTH_NAMES.map((m, i) => <option key={m} value={i}>{m}</option>)}
        </select>
        <select value={viewYear} onChange={(e) => setViewYear(Number(e.target.value))} style={selectStyle}>
          {years.map((y) => <option key={y} value={y}>{y}</option>)}
        </select>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
        <button type="button" onClick={goPrev} style={{ width: 26, height: 26, borderRadius: '50%', border: 'none', background: 'transparent', display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', color: isDark ? '#999' : '#666' }}>
          <ChevronLeft size={14} />
        </button>
        <span style={{ fontSize: 13, fontWeight: 700, color: isDark ? '#fff' : '#0a0a0a' }}>{DOB_MONTH_NAMES[viewMonth]} {viewYear}</span>
        <button type="button" onClick={goNext} style={{ width: 26, height: 26, borderRadius: '50%', border: 'none', background: 'transparent', display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', color: isDark ? '#999' : '#666' }}>
          <ChevronRight size={14} />
        </button>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', marginBottom: 2 }}>
        {DOB_WEEKDAY_LABELS.map((d) => (
          <div key={d} style={{ textAlign: 'center', fontSize: 9, fontWeight: 900, letterSpacing: 0.4, color: isDark ? '#777' : '#999', padding: '2px 0' }}>{d}</div>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)' }}>
        {cells.map((day, idx) => {
          if (!day) return <div key={`pad-${idx}`} style={{ height: 30 }} />;
          const dateStr    = `${viewYear}-${String(viewMonth + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
          const isFuture   = dateStr > todayStr;
          const isSelected = parsedDay === day && parsedMonth === viewMonth && parsedYear === viewYear;
          return (
            <div key={day} style={{ height: 30, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <button
                type="button"
                disabled={isFuture}
                onClick={() => onSelect(day, viewMonth, viewYear)}
                style={{
                  width: 26, height: 26, borderRadius: '50%', fontSize: 12, border: 'none',
                  cursor: isFuture ? 'not-allowed' : 'pointer',
                  background: isSelected ? '#ff2d55' : 'transparent',
                  color: isFuture ? (isDark ? '#3a3a3a' : '#d5d5d5') : isSelected ? '#fff' : (isDark ? '#fff' : '#0a0a0a'),
                  fontWeight: isSelected ? 700 : 400,
                }}
              >
                {day}
              </button>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function DobField({ value, onChange, error }: { value: string; onChange: (v: string) => void; error?: string }) {
  const [focused, setFocused]           = useState(false);
  const [calendarOpen, setCalendarOpen] = useState(false);
  const { isDark } = useAppTheme();
  const wrapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!calendarOpen) return;
    const onClickOutside = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setCalendarOpen(false);
    };
    document.addEventListener('mousedown', onClickOutside);
    return () => document.removeEventListener('mousedown', onClickOutside);
  }, [calendarOpen]);

  return (
    <div ref={wrapRef} style={{ position: 'relative' }}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <div style={{ position: 'relative' }}>
          <input
            type="text"
            inputMode="numeric"
            value={value}
            onChange={(e) => onChange(formatDobInput(e.target.value, value))}
            placeholder="Date of birth (DD/MM/YYYY)"
            autoComplete="bday"
            maxLength={10}
            onFocus={() => setFocused(true)}
            onBlur={() => setFocused(false)}
            style={{
              background: isDark ? '#1c1c1c' : '#f5f5f5',
              color: isDark ? '#fff' : '#0a0a0a',
              border: `1.5px solid ${error ? '#ff3c6e' : focused ? (isDark ? '#555' : '#0a0a0a') : (isDark ? '#333' : '#e0e0e0')}`,
              borderRadius: 10,
              padding: '12px 44px 12px 14px',
              fontSize: 15,
              outline: 'none',
              width: '100%',
              boxSizing: 'border-box' as const,
            }}
          />
          <button
            type="button"
            onClick={() => setCalendarOpen((o) => !o)}
            aria-label="Choose date from calendar"
            style={{
              position: 'absolute', right: 7, top: '50%', transform: 'translateY(-50%)',
              width: 30, height: 30, borderRadius: 8, border: 'none', cursor: 'pointer',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              background: calendarOpen ? '#ff2d55' : 'transparent',
            }}
          >
            <Calendar size={17} color={calendarOpen ? '#fff' : (isDark ? '#999' : '#666')} />
          </button>
        </div>
        {error && <span style={{ color: '#ff3c6e', fontSize: 12, fontWeight: 600 }}>{error}</span>}
      </div>
      {calendarOpen && (
        <DobCalendarPopover
          value={value}
          isDark={isDark}
          onSelect={(day, month, year) => {
            onChange(`${String(day).padStart(2, '0')}/${String(month + 1).padStart(2, '0')}/${year}`);
            setCalendarOpen(false);
          }}
        />
      )}
    </div>
  );
}

export default function AuthPage() {
  const router = useRouter();
  const { login, register } = useAuthStore();
  const { isDark, colors } = useAppTheme();

  const [mode, setMode] = useState<Mode>('login');
  const [loading, setLoading] = useState(false);
  const [globalErr, setGlobalErr] = useState<string | null>(null);
  const [agreed, setAgreed] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [showErrorToast, setShowErrorToast] = useState(false);

  // The toast lists whatever is currently in `errors`, so it shrinks live as
  // the person fixes each field, and disappears once nothing's left. It also
  // auto-dismisses ~5s after last changing — long enough to read a short
  // list without lingering once they've moved on.
  const activeErrors = Object.entries(errors).filter(([, v]) => v);
  useEffect(() => {
    if (activeErrors.length === 0) { setShowErrorToast(false); return; }
    if (!showErrorToast) return;
    const t = setTimeout(() => setShowErrorToast(false), 5000);
    return () => clearTimeout(t);
  }, [errors, showErrorToast]);

  const [loginEmail, setLoginEmail] = useState('');
  const [loginPassword, setLoginPassword] = useState('');
  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [regEmail, setRegEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [dob, setDob] = useState('');
  const [regPassword, setRegPassword] = useState('');
  const [confirmPass, setConfirmPass] = useState('');

  const clearErr = (key: string) => setErrors((p) => ({ ...p, [key]: '' }));

  const switchMode = (m: Mode) => { setMode(m); setGlobalErr(null); setErrors({}); setShowErrorToast(false); };

  const parseDobToISO = (f: string) => {
    const [dd, mm, yyyy] = f.split('/');
    if (!dd || !mm || !yyyy || yyyy.length < 4) return null;
    const d = new Date(`${yyyy}-${mm}-${dd}`);
    return isNaN(d.getTime()) ? null : d.toISOString();
  };

  // South African national numbers are 10 digits (0XX XXX XXXX) — the +27
  // box next to this field is just the country code display, so cap the
  // typed value at 10 digits, matching the >=10 check already used below.
  const formatPhoneInput = (raw: string) => raw.replace(/\D/g, '').slice(0, 10);

  // Two valid shapes: 10 digits starting with the trunk 0 (0821234567),
  // or 9 digits with the leading 0 dropped (821234567) — the latter is how
  // the number reads once the +27 country code stands in for it.
  const isValidSaPhone = (raw: string) => {
    const digits = raw.replace(/\D/g, '');
    return (digits.length === 10 && digits.startsWith('0')) || (digits.length === 9 && !digits.startsWith('0'));
  };

  const handleLogin = useCallback(async () => {
    const errs: Record<string, string> = {};
    if (!loginEmail.trim()) errs.loginEmail = 'Please enter your email';
    if (Object.keys(errs).length) { setErrors(errs); setShowErrorToast(true); return; }
    setLoading(true); setGlobalErr(null);
    try {
      await login(loginEmail.trim().toLowerCase(), loginPassword);
      router.back();
    } catch (e: any) {
      const msg = e?.response?.data?.error ?? '';
      setGlobalErr(msg.includes('credentials') ? 'Incorrect email or password.' : msg || 'Something went wrong.');
    } finally { setLoading(false); }
  }, [loginEmail, loginPassword, login, router]);

  const handleRegister = useCallback(async () => {
    const errs: Record<string, string> = {};
    if (!firstName.trim()) errs.firstName = 'Required';
    if (!lastName.trim()) errs.lastName = 'Required';
    if (!regEmail.trim()) errs.regEmail = 'Required';
    else if (!/\S+@\S+\.\S+/.test(regEmail)) errs.regEmail = 'Enter a valid email';
    if (!phone.trim()) errs.phone = 'Required';
    else if (!isValidSaPhone(phone)) errs.phone = 'Enter a valid number';
    if (!dob) errs.dob = 'Required';
    else if (!parseDobToISO(dob)) errs.dob = 'Use DD/MM/YYYY';
    if (!regPassword) errs.regPassword = 'Required';
    else if (!(regPassword.length >= 8 && /[A-Z]/.test(regPassword) && /[0-9]/.test(regPassword) && /[^A-Za-z0-9\s]/.test(regPassword))) errs.regPassword = 'Password does not meet all requirements';
    if (confirmPass !== regPassword) errs.confirmPass = "Passwords don't match";
    if (!agreed) errs.agreed = 'You must agree to continue';
    if (Object.keys(errs).length) { setErrors(errs); setShowErrorToast(true); return; }
    setLoading(true); setGlobalErr(null);
    try {
      await register({
        email: regEmail.trim().toLowerCase(), password: regPassword,
        first_name: firstName.trim(), last_name: lastName.trim(),
        phone_number: phone.trim(), date_of_birth: parseDobToISO(dob)!,
      });
      router.back();
    } catch (e: any) {
      const msg = e?.response?.data?.error ?? '';
      setGlobalErr(msg.toLowerCase().includes('email') ? 'An account with this email already exists.' : msg || 'Something went wrong.');
    } finally { setLoading(false); }
  }, [firstName, lastName, regEmail, phone, dob, regPassword, confirmPass, agreed, register, router]);

  const bg = isDark ? '#0a0a0a' : '#f5f5f7';
  const cardBg = isDark ? '#111' : '#fff';
  const border = isDark ? '#222' : '#e5e5e5';

  return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '24px 16px' }}>

      {/* ── Error summary toast ── fixed to the top of the viewport so it's
           visible no matter how far down the form the person has scrolled.
           Lists every currently-invalid field at once, and shrinks live as
           each one gets fixed. */}
      {showErrorToast && activeErrors.length > 0 && (
        <div
          style={{
            position: 'fixed', top: 16, left: 16, right: 16, zIndex: 200,
            maxWidth: 420, margin: '0 auto',
            display: 'flex', alignItems: 'flex-start', gap: 10,
            background: '#2a0e14', border: '1px solid #ff3c6e',
            borderRadius: 12, padding: '12px 14px',
            boxShadow: '0 8px 24px rgba(0,0,0,0.35)',
          }}
          role="alert"
        >
          <AlertCircle size={18} color="#ff3c6e" style={{ flexShrink: 0, marginTop: 1 }} />
          <div style={{ flex: 1 }}>
            <p style={{ margin: '0 0 4px', fontSize: 13, fontWeight: 800, color: '#fff' }}>
              Please fix the following:
            </p>
            <ul style={{ margin: 0, padding: 0, listStyle: 'none', display: 'flex', flexDirection: 'column', gap: 2 }}>
              {activeErrors.map(([key, message]) => (
                <li key={key} style={{ fontSize: 12.5, color: '#ffd7de', lineHeight: 1.5 }}>
                  <span style={{ fontWeight: 700, color: '#fff' }}>{FIELD_LABELS[key] ?? key}:</span> {message}
                </li>
              ))}
            </ul>
          </div>
          <button
            onClick={() => setShowErrorToast(false)}
            aria-label="Dismiss"
            style={{ background: 'none', border: 'none', padding: 2, cursor: 'pointer', flexShrink: 0, lineHeight: 0 }}
          >
            <X size={16} color="#ffb3c2" />
          </button>
        </div>
      )}

      <div style={{ width: '100%', maxWidth: 440, background: cardBg, borderRadius: 20, border: `1px solid ${border}`, padding: '32px 28px', boxShadow: isDark ? '0 20px 60px rgba(0,0,0,0.5)' : '0 8px 40px rgba(0,0,0,0.08)' }}>

        {/* Brand */}
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10, marginBottom: 28 }}>
          <img src="/brand-logo_v2.png" alt="TicketHawk" style={{ width: 64, height: 64, objectFit: 'contain' }} />
          <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>
            {mode === 'login' ? 'Welcome back' : 'Create account'}
          </h1>
          <p style={{ margin: 0, fontSize: 13, color: colors.subText, textAlign: 'center' }}>
            {mode === 'login' ? 'Sign in to access your tickets and bookings' : 'Join thousands of event-goers across South Africa'}
          </p>
        </div>

        {/* Tab toggle */}
        <div style={{ display: 'flex', background: isDark ? '#1c1c1c' : '#ebebeb', borderRadius: 12, padding: 3, marginBottom: 24, position: 'relative' }}>
          {(['login', 'register'] as Mode[]).map((m) => (
            <button key={m} onClick={() => switchMode(m)} style={{
              flex: 1, padding: '9px 0', border: 'none', cursor: 'pointer', borderRadius: 10, fontSize: 14, fontWeight: mode === m ? 700 : 500,
              background: mode === m ? (isDark ? '#2c2c2c' : '#fff') : 'transparent',
              color: mode === m ? colors.text : colors.subText,
              boxShadow: mode === m ? '0 1px 4px rgba(0,0,0,0.12)' : 'none',
              transition: 'all 0.2s',
            }}>
              {m === 'login' ? 'Log in' : 'Sign up'}
            </button>
          ))}
        </div>

        {/* Global error */}
        {globalErr && (
          <div style={{ background: 'rgba(255,60,110,0.1)', border: '1px solid rgba(255,60,110,0.3)', borderRadius: 10, padding: '10px 14px', marginBottom: 16, display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontSize: 15 }}>⚠️</span>
            <span style={{ color: '#ff3c6e', fontSize: 13, fontWeight: 600 }}>{globalErr}</span>
          </div>
        )}

        {/* LOGIN FORM */}
        {mode === 'login' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <Field placeholder="Email address" value={loginEmail} onChange={(v) => { setLoginEmail(v); clearErr('loginEmail'); }} type="email" error={errors.loginEmail} autoComplete="email" />
            <Field placeholder="Password" value={loginPassword} onChange={(v) => { setLoginPassword(v); clearErr('loginPassword'); }} type="password" error={errors.loginPassword} autoComplete="current-password" />
            <button onClick={handleLogin} disabled={loading} style={{
              background: isDark ? '#fff' : '#0a0a0a', color: isDark ? '#0a0a0a' : '#fff',
              border: 'none', borderRadius: 12, padding: '14px', fontSize: 15, fontWeight: 800,
              cursor: loading ? 'not-allowed' : 'pointer', opacity: loading ? 0.6 : 1, marginTop: 4,
            }}>
              {loading ? 'Logging in…' : 'Log in'}
            </button>
            <p style={{ textAlign: 'center', fontSize: 13, color: colors.subText, margin: 0 }}>
              <span onClick={() => router.push('/reset-password')} style={{ color: '#ff2d55', fontWeight: 700, cursor: 'pointer' }}>Forgot password?</span>
            </p>
            <p style={{ textAlign: 'center', fontSize: 13, color: colors.subText, margin: 0 }}>
              Don&apos;t have an account?{' '}
              <span onClick={() => switchMode('register')} style={{ color: '#ff2d55', fontWeight: 700, cursor: 'pointer' }}>Sign up</span>
            </p>
          </div>
        )}

        {/* REGISTER FORM */}
        {mode === 'register' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <Field placeholder="Email address" value={regEmail} onChange={(v) => { setRegEmail(v); clearErr('regEmail'); }} type="email" error={errors.regEmail} autoComplete="email" />
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
              <Field placeholder="First name" value={firstName} onChange={(v) => { setFirstName(v); clearErr('firstName'); }} error={errors.firstName} autoComplete="given-name" />
              <Field placeholder="Last name" value={lastName} onChange={(v) => { setLastName(v); clearErr('lastName'); }} error={errors.lastName} autoComplete="family-name" />
            </div>
            <Field placeholder="Password" value={regPassword} onChange={(v) => { setRegPassword(v); clearErr('regPassword'); }} type="password" error={errors.regPassword} autoComplete="new-password" />
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, padding: '0 4px', marginTop: -2, marginBottom: 4 }}>
              {[
                { label: 'At least 8 characters',             ok: regPassword.length >= 8 },
                { label: 'One capital letter (A\u2013Z)',        ok: /[A-Z]/.test(regPassword) },
                { label: 'One number (0\u20139)',                ok: /[0-9]/.test(regPassword) },
                { label: 'One special character (e.g. !@#$)', ok: /[^A-Za-z0-9\s]/.test(regPassword) },
              ].map((c) => (
                <div key={c.label} style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
                  <CheckCircle2 size={15} color={c.ok ? '#22c55e' : (isDark ? 'rgba(255,255,255,0.28)' : 'rgba(10,10,10,0.25)')} />
                  <span style={{ fontSize: 12.5, color: c.ok ? '#22c55e' : colors.subText }}>{c.label}</span>
                </div>
              ))}
            </div>
            <Field placeholder="Confirm password" value={confirmPass} onChange={(v) => { setConfirmPass(v); clearErr('confirmPass'); }} type="password" error={errors.confirmPass} autoComplete="new-password" />
            <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
              <div style={{ display: 'flex', alignItems: 'center', background: isDark ? '#1c1c1c' : '#f5f5f5', border: `1.5px solid ${isDark ? '#333' : '#e0e0e0'}`, borderRadius: '10px 0 0 10px', padding: '12px 12px', whiteSpace: 'nowrap', fontSize: 14, color: colors.text }}>
                🇿🇦 +27
              </div>
              <div style={{ flex: 1 }}>
                <Field placeholder="Phone number" value={phone} onChange={(v) => { setPhone(formatPhoneInput(v)); clearErr('phone'); }} type="tel" error={errors.phone} autoComplete="tel" />
              </div>
            </div>
            <DobField value={dob} onChange={(v) => { setDob(v); clearErr('dob'); }} error={errors.dob} />
            {/* Terms */}
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10, marginTop: 4 }}>
              <div onClick={() => { setAgreed((a) => !a); clearErr('agreed'); }} style={{
                width: 20, height: 20, minWidth: 20, borderRadius: 5, border: `2px solid ${agreed ? '#ff2d55' : (isDark ? '#444' : '#ccc')}`,
                background: agreed ? '#ff2d55' : 'transparent', display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', marginTop: 2,
              }}>
                {agreed && <span style={{ color: '#fff', fontSize: 12, fontWeight: 900 }}>✓</span>}
              </div>
              <p style={{ margin: 0, fontSize: 12, color: colors.subText, lineHeight: 1.5 }}>
                By checking this box, you agree to our{' '}
                <span style={{ color: '#ff2d55', fontWeight: 600 }}>Terms of Use</span> and acknowledge having read our{' '}
                <span style={{ color: '#ff2d55', fontWeight: 600 }}>Privacy Policy</span>.
              </p>
            </div>
            {errors.agreed && <span style={{ color: '#ff3c6e', fontSize: 12, fontWeight: 600 }}>{errors.agreed}</span>}
            <button onClick={handleRegister} disabled={loading} style={{
              background: isDark ? '#fff' : '#0a0a0a', color: isDark ? '#0a0a0a' : '#fff',
              border: 'none', borderRadius: 12, padding: '14px', fontSize: 15, fontWeight: 800,
              cursor: loading ? 'not-allowed' : 'pointer', opacity: loading ? 0.6 : 1, marginTop: 4,
            }}>
              {loading ? 'Creating account…' : 'Sign up'}
            </button>
            <p style={{ textAlign: 'center', fontSize: 13, color: colors.subText, margin: 0 }}>
              Already have an account?{' '}
              <span onClick={() => switchMode('login')} style={{ color: '#ff2d55', fontWeight: 700, cursor: 'pointer' }}>Log in</span>
            </p>
          </div>
        )}
      </div>
    </div>
  );
}