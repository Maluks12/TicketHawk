'use client';

import { passwordChecks } from '../lib/validation';

// ─────────────────────────────────────────────────────────────────────────────
// Live password requirements.
//
// Shown under the password field on BOTH screens that set a password: signup
// (/login?mode=register) and reset (/reset-password?token=…). They hit two
// different endpoints — /auth/register and /auth/reset_password — but the
// backend binds both to the same `strong_password` tag, so showing two
// different rule lists would be a lie on one of them.
//
// The rows come from lib/validation.passwordChecks(), which validatePassword()
// is also built on. One array, so a tick can never mean something the form
// will then reject.
// ─────────────────────────────────────────────────────────────────────────────

export default function PasswordChecklist({
  password,
  style,
}: {
  password: string;
  /** Escape hatch for spacing — the two screens sit in different layouts. */
  style?: React.CSSProperties;
}) {
  const checks = passwordChecks(password);
  const allMet = checks.every(c => c.ok);
  // Until they type, everything stays grey and neutral. A wall of failed marks
  // on an empty field reads as an error before they've done anything wrong.
  const touched = password.length > 0;

  return (
    <div style={{
      padding: '11px 13px', marginBottom: 16,
      background: 'var(--surface-alpha)',
      border: `1px solid ${touched && allMet ? 'var(--success-border)' : 'var(--border)'}`,
      borderRadius: 'var(--r-lg)',
      transition: 'border-color .15s ease',
      ...style,
    }}>
      <div style={{
        fontSize: 11.5, fontWeight: 700, letterSpacing: .4, textTransform: 'uppercase',
        color: 'var(--text-3)', marginBottom: 8,
      }}>
        Password must contain
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        {checks.map(c => (
          <div key={c.label} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <CheckIcon met={touched && c.ok} />
            <span style={{
              fontSize: 12.5, lineHeight: 1.35,
              color: touched && c.ok ? 'var(--success-fg)' : 'var(--text-2)',
              transition: 'color .15s ease',
            }}>
              {c.label}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

// Inline rather than an icon-package import, matching the EyeIcon on both auth
// screens. Met is a filled check; unmet is a hollow ring, not a red cross — an
// unfinished requirement is not an error yet.
function CheckIcon({ met }: { met: boolean }) {
  return met ? (
    <svg viewBox="0 0 20 20" fill="var(--success)" width="15" height="15"
      style={{ flexShrink: 0 }} aria-hidden="true">
      <path fillRule="evenodd" clipRule="evenodd"
        d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" />
    </svg>
  ) : (
    <svg viewBox="0 0 20 20" fill="none" width="15" height="15"
      style={{ flexShrink: 0 }} aria-hidden="true">
      <circle cx="10" cy="10" r="7" stroke="var(--text-faint)" strokeWidth="1.5" />
    </svg>
  );
}