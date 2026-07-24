'use client';

import { useState, useCallback } from 'react';
import { CheckCircle2 } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';

type Mode = 'login' | 'register';

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

export default function AuthPage() {
  const router = useRouter();
  const { login, register } = useAuthStore();
  const { isDark, colors } = useAppTheme();

  const [mode, setMode] = useState<Mode>('login');
  const [loading, setLoading] = useState(false);
  const [globalErr, setGlobalErr] = useState<string | null>(null);
  const [agreed, setAgreed] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

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

  const switchMode = (m: Mode) => { setMode(m); setGlobalErr(null); setErrors({}); };

  const parseDobToISO = (f: string) => {
    const [dd, mm, yyyy] = f.split('/');
    if (!dd || !mm || !yyyy || yyyy.length < 4) return null;
    const d = new Date(`${yyyy}-${mm}-${dd}`);
    return isNaN(d.getTime()) ? null : d.toISOString();
  };

  const handleLogin = useCallback(async () => {
    const errs: Record<string, string> = {};
    if (!loginEmail.trim()) errs.loginEmail = 'Please enter your email';
    if (!loginPassword.trim()) errs.loginPassword = 'Please enter your password';
    if (Object.keys(errs).length) { setErrors(errs); return; }
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
    else if (phone.replace(/\D/g, '').length < 10) errs.phone = 'Enter a valid number';
    if (!dob) errs.dob = 'Required';
    else if (!parseDobToISO(dob)) errs.dob = 'Use DD/MM/YYYY';
    if (!regPassword) errs.regPassword = 'Required';
    else if (!(regPassword.length >= 8 && /[A-Z]/.test(regPassword) && /[0-9]/.test(regPassword) && /[^A-Za-z0-9\s]/.test(regPassword))) errs.regPassword = 'Password does not meet all requirements';
    if (confirmPass !== regPassword) errs.confirmPass = "Passwords don't match";
    if (!agreed) errs.agreed = 'You must agree to continue';
    if (Object.keys(errs).length) { setErrors(errs); return; }
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
                <Field placeholder="Phone number" value={phone} onChange={(v) => { setPhone(v); clearErr('phone'); }} type="tel" error={errors.phone} autoComplete="tel" />
              </div>
            </div>
            <Field placeholder="Date of birth (DD/MM/YYYY)" value={dob} onChange={(v) => { setDob(v); clearErr('dob'); }} error={errors.dob} />
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