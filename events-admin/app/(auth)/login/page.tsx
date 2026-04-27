'use client';
import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useAdminAuth } from '../../../store/authStore';

export default function LoginPage() {
  const router = useRouter();
  const { login, user, hydrate, isLoading } = useAdminAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => { hydrate(); }, [hydrate]);
  useEffect(() => { if (!isLoading && user) router.replace('/dashboard'); }, [user, isLoading, router]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!email || !password) { setError('Email and password required'); return; }
    setError(''); setSubmitting(true);
    try {
      await login(email, password);
      router.replace('/dashboard');
    } catch (err: unknown) {
      const apiMsg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error;
      const msg = err instanceof Error ? err.message : 'Login failed.';
      setError(apiMsg || msg);
    } finally { setSubmitting(false); }
  }

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--bg)', position: 'relative', overflow: 'hidden',
    }}>
      <div style={{ position: 'absolute', inset: 0, background: 'radial-gradient(ellipse 60% 50% at 50% 0%,rgba(245,158,11,.08) 0%,transparent 70%)' }} />
      <div style={{
        background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)',
        padding: '2.5rem 2rem', width: '100%', maxWidth: '420px', position: 'relative', zIndex: 1,
      }}>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600, color: 'var(--accent)', letterSpacing: '.05em', marginBottom: '2rem', display: 'flex', alignItems: 'center', gap: '.5rem' }}>
          ⬡ events<span style={{ color: 'var(--text-2)', fontWeight: 400 }}>/admin</span>
        </div>
        <h1 style={{ fontSize: '1.4rem', fontWeight: 600, marginBottom: '.25rem', color: 'var(--text)' }}>Admin Dashboard</h1>
        <p style={{ color: 'var(--text-2)', marginBottom: '1.75rem', fontSize: '.9rem' }}>
          Sign in with your admin credentials to manage events, merch, and users.
        </p>
        <form onSubmit={handleSubmit}>
          <div className="field">
            <label>Email</label>
            <input type="email" value={email} onChange={e => setEmail(e.target.value)} placeholder="admin@example.com" required />
          </div>
          <div className="field">
            <label>Password</label>
            <input type="password" value={password} onChange={e => setPassword(e.target.value)} placeholder="••••••••" required
              onKeyDown={e => e.key === 'Enter' && handleSubmit(e as unknown as React.FormEvent)} />
          </div>
          {error && (
            <div style={{ background: 'rgba(239,68,68,.1)', border: '1px solid rgba(239,68,68,.3)', borderRadius: 'var(--r)', padding: '.6rem .75rem', color: '#fca5a5', fontSize: '.85rem', marginBottom: '1rem' }}>
              {error}
            </div>
          )}
          <button type="submit" className="btn btn-primary btn-full" disabled={submitting} style={{ marginTop: '.5rem' }}>
            {submitting ? 'Signing in…' : 'Sign In'}
          </button>
        </form>
        <p style={{ marginTop: '.75rem', fontSize: '.75rem', color: 'var(--text-3)', textAlign: 'center' }}>
          Requires admin role — contact your system administrator if access is denied.
        </p>
      </div>
    </div>
  );
}