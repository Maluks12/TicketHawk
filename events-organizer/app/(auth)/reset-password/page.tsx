'use client';

import { Suspense, useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { api } from '../../../lib/api';
import { validatePasswordMatch, firstError } from '../../../lib/validation';

/**
 * /reset-password — organiser dashboard
 *
 * This page is the target of TWO email flows:
 *   1. The organiser HANDOVER email (admin-entered events, Path 4): new
 *      organisers get a "Set Your Password" button pointing at
 *      {OrganizerDashboardURL}/reset-password?token=… — until this page
 *      existed, that button was a 404 and handed-over accounts could never
 *      be activated.
 *   2. Ordinary password resets requested by organisers/admins.
 *
 * Backend: POST /auth/reset_password { token, new_password }
 *   (token must be a bare uuid4; 400 with details on weak passwords)
 */

// Mirrors the backend's password policy so users get instant feedback
// instead of a round-trip rejection.
function passwordProblem(pw: string): string | null {
  if (!pw) return 'Password is required';
  if (pw.length < 8) return 'Password must be at least 8 characters';
  if (!/[A-Z]/.test(pw)) return 'Password needs an uppercase letter';
  if (!/[0-9]/.test(pw)) return 'Password needs a digit';
  if (!/[^A-Za-z0-9]/.test(pw)) return 'Password needs a special character';
  return null;
}

function ResetPasswordInner() {
  const router = useRouter();
  const params = useSearchParams();
  const token = useMemo(() => params.get('token')?.trim() ?? '', [params]);

  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [showPass, setShowPass] = useState(false);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [done, setDone] = useState(false);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const err = firstError({
      password: passwordProblem(password),
      confirm: validatePasswordMatch(password, confirm),
    });
    if (err) { setError(err); return; }

    setError(''); setSubmitting(true);
    try {
      await api.post('/auth/reset_password', { token, new_password: password });
      setDone(true);
    } catch (e: unknown) {
      const data = (e as { response?: { data?: { error?: string; details?: Record<string, string> } } })
        ?.response?.data;
      if (data?.details?.new_password) setError(data.details.new_password);
      else if (data?.error && data.error !== 'Validation failed') setError(data.error);
      else setError('This link is invalid or has expired. Request a new reset email and try again.');
    } finally { setSubmitting(false); }
  }

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--bg)', padding: 20,
      fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
    }}>
      <style>{`
        .org-input {
          width: 100%; padding: 10px 12px; font-size: 14px;
          border: 1px solid var(--border); border-radius: 6px; outline: none;
          box-sizing: border-box; font-family: inherit; color: var(--text);
        }
        .org-input:focus { border-color: var(--brand); box-shadow: 0 0 0 2px rgba(255,45,85,0.15); }
      `}</style>

      <div style={{ width: '100%', maxWidth: 400 }}>

        {/* Brand */}
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <div style={{
            width: 60, height: 60, borderRadius: 14, background: 'var(--accent-deep)',
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center', marginBottom: 12,
          }}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="/brand-logo.png" alt="TicketHawk" width={44} height={44} />
          </div>
          <div style={{ fontSize: 24, fontWeight: 800, letterSpacing: 0.5, color: 'var(--text)' }}>
            TICKET<span style={{ color: 'var(--brand)' }}>HAWK</span>
          </div>
          <div style={{ fontSize: 14, color: 'var(--text-2)', marginTop: 4 }}>Organiser Dashboard</div>
        </div>

        {/* Card */}
        <div style={{
          background: 'var(--surface)', borderRadius: 8, border: '1px solid var(--border)',
          padding: '24px', overflow: 'hidden',
        }}>
          {done ? (
            <div style={{ textAlign: 'center' }}>
              <div style={{
                width: 48, height: 48, borderRadius: 24, background: 'var(--success-bg)',
                display: 'inline-flex', alignItems: 'center', justifyContent: 'center', marginBottom: 12,
              }}>
                <svg viewBox="0 0 20 20" fill="var(--success)" width="24" height="24">
                  <path fillRule="evenodd" d="M16.7 5.3a1 1 0 010 1.4l-8 8a1 1 0 01-1.4 0l-4-4a1 1 0 111.4-1.4L8 12.6l7.3-7.3a1 1 0 011.4 0z" clipRule="evenodd" />
                </svg>
              </div>
              <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--text)', marginBottom: 6 }}>
                Password set
              </div>
              <div style={{ fontSize: 14, color: 'var(--text-2)', marginBottom: 20, lineHeight: 1.5 }}>
                Your password has been updated and any other sessions were signed out.
                You can now log in to your organiser dashboard.
              </div>
              <button onClick={() => router.replace('/login')} style={{
                width: '100%', padding: '11px', fontSize: 15, fontWeight: 700,
                background: 'var(--brand)', color: '#fff', border: 'none',
                borderRadius: 6, cursor: 'pointer', fontFamily: 'inherit',
              }}>
                Go to login
              </button>
            </div>
          ) : !token ? (
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--text)', marginBottom: 6 }}>
                Invalid link
              </div>
              <div style={{ fontSize: 14, color: 'var(--text-2)', lineHeight: 1.5 }}>
                This password link is missing its token. Please open the exact link
                from your email — or request a new one from the login page.
              </div>
            </div>
          ) : (
            <>
              <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--text)', marginBottom: 4 }}>
                Set your password
              </div>
              <div style={{ fontSize: 13, color: 'var(--text-2)', marginBottom: 16, lineHeight: 1.5 }}>
                Choose a password for your TicketHawk organiser account. It needs at
                least 8 characters, an uppercase letter, a digit, and a special character.
              </div>

              {error && (
                <div style={{
                  background: 'var(--danger-bg)', border: '1px solid var(--danger-border)', borderRadius: 6,
                  padding: '10px 12px', color: 'var(--danger-fg)', fontSize: 13, marginBottom: 16,
                }}>
                  {error}
                </div>
              )}

              <form onSubmit={handleSubmit} noValidate>
                <div style={{ marginBottom: 12 }}>
                  <label style={{ display: 'block', fontSize: 13, fontWeight: 600, color: 'var(--text)', marginBottom: 5 }}>
                    New password
                  </label>
                  <div style={{ position: 'relative' }}>
                    <input className="org-input" type={showPass ? 'text' : 'password'}
                      value={password} autoFocus autoComplete="new-password"
                      onChange={e => setPassword(e.target.value)} placeholder="••••••••" />
                    <button type="button" onClick={() => setShowPass(p => !p)}
                      aria-label={showPass ? 'Hide password' : 'Show password'}
                      style={{
                        position: 'absolute', right: 0, top: 0, bottom: 0, width: 40,
                        background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-3)',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                      }}>
                      <svg viewBox="0 0 20 20" fill="currentColor" width="16" height="16">
                        {showPass
                          ? <path fillRule="evenodd" d="M3.7 2.3a1 1 0 00-1.4 1.4l14 14a1 1 0 001.4-1.4l-1.5-1.5A10.1 10.1 0 0019.5 10S16 3.5 10 3.5c-1.2 0-2.3.25-3.3.67L3.7 2.3zM10 6.5a3.5 3.5 0 013.4 4.3L9.2 6.6c.26-.06.53-.1.8-.1zM.5 10s1.3-2.4 3.6-4.2l2.6 2.6a3.5 3.5 0 004.4 4.4l2 2A9.4 9.4 0 0110 16.5C4 16.5.5 10 .5 10z" clipRule="evenodd" />
                          : <path fillRule="evenodd" d="M10 3.5C4 3.5.5 10 .5 10S4 16.5 10 16.5 19.5 10 19.5 10 16 3.5 10 3.5zm0 10a3.5 3.5 0 110-7 3.5 3.5 0 010 7z" clipRule="evenodd" />}
                      </svg>
                    </button>
                  </div>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ display: 'block', fontSize: 13, fontWeight: 600, color: 'var(--text)', marginBottom: 5 }}>
                    Confirm password
                  </label>
                  <input className="org-input" type={showPass ? 'text' : 'password'}
                    value={confirm} autoComplete="new-password"
                    onChange={e => setConfirm(e.target.value)} placeholder="••••••••" />
                </div>

                <button type="submit" disabled={submitting} style={{
                  width: '100%', padding: '11px', fontSize: 15, fontWeight: 700,
                  background: 'var(--brand)', color: '#fff', border: 'none',
                  borderRadius: 6, cursor: submitting ? 'not-allowed' : 'pointer',
                  opacity: submitting ? 0.7 : 1, fontFamily: 'inherit',
                }}>
                  {submitting ? 'Setting password…' : 'Set password'}
                </button>
              </form>
            </>
          )}
        </div>
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