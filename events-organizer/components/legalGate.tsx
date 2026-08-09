'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Legal acceptance gate
//
// New accounts accept the agreements at sign-up. Organisers who already had an
// account never saw that checkbox, so this is how they're told — and how their
// acceptance is recorded against the same version.
//
// It also handles the ongoing case: bumping CURRENT_LEGAL_VERSION re-prompts
// everyone, which is the only way a material change (a different commission
// split, say) can be re-consented rather than silently imposed.
//
// ── FAIL OPEN ────────────────────────────────────────────────────────────────
//
// If the status endpoint errors, 404s, or the backend simply hasn't shipped
// yet, the gate does NOT appear and nobody is blocked. That direction is
// deliberate. A consent gate that fails closed would lock every organiser out
// of their own dashboard the moment the endpoint had a bad minute — during
// which they can't check door scans at a live event. The cost of failing open
// is that a prompt is missed for a while; the cost of failing closed is that
// the product stops working.
//
// ── NOT DISMISSIBLE ──────────────────────────────────────────────────────────
//
// There's no X and no Escape handler. This is the one modal in the product
// where that's right: it isn't a notification, it's the record of an agreement,
// and a dismissible consent prompt produces organisers who have "seen" terms
// they never accepted. They can always sign out instead.
// ─────────────────────────────────────────────────────────────────────────────
import { useEffect, useState } from 'react';
import { ShieldCheck, ArrowSquareOut } from '@phosphor-icons/react';
import { api } from '../lib/api';
import {
  ORGANISER_LEGAL_DOCS, legalLinkProps, CURRENT_LEGAL_VERSION,
} from '../lib/legalLinks';

export default function LegalGate({ onLogout }: { onLogout: () => void }) {
  const [needed, setNeeded] = useState(false);
  const [checked, setChecked] = useState(false);
  const [accepted, setAccepted] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // ── Status ───────────────────────────────────────────────────────────────
  useEffect(() => {
    let alive = true;
    api.get('/organiser/legal')
      .then(({ data }) => {
        if (!alive) return;
        // Anything older than the current version counts as not accepted —
        // that comparison is the whole point of storing a version rather than
        // a boolean.
        const v = data?.accepted_version as string | undefined;
        setNeeded(v !== CURRENT_LEGAL_VERSION);
        setChecked(true);
      })
      .catch(() => {
        // Fail open — see the header comment.
        if (alive) { setNeeded(false); setChecked(true); }
      });
    return () => { alive = false; };
  }, []);

  async function accept() {
    setBusy(true);
    setError(null);
    try {
      await api.post('/organiser/legal/accept', {
        version: CURRENT_LEGAL_VERSION,
        accepted_at: new Date().toISOString(),
      });
      setNeeded(false);
    } catch {
      // Failing to RECORD acceptance is different from failing to read status:
      // here the organiser has acted, and pretending it worked would leave no
      // record of an agreement they believe they made. So this one surfaces.
      setError('We could not record your acceptance. Please try again.');
    } finally {
      setBusy(false);
    }
  }

  if (!checked || !needed) return null;

  return (
    <div
      className="modal-overlay"
      // No click-outside handler: dismissing by missing the dialog is not a
      // decision about a legal agreement.
      style={{ alignItems: 'center', zIndex: 2000 }}
    >
      <div className="modal modal-sm" role="dialog" aria-modal="true"
        aria-labelledby="legal-gate-title"
        style={{ maxWidth: 520 }}>
        <div className="modal-body" style={{ padding: 26 }}>
          <div style={{
            width: 44, height: 44, borderRadius: 14, marginBottom: 16,
            background: 'var(--accent-soft)', color: 'var(--accent)',
            display: 'grid', placeItems: 'center',
          }}>
            <ShieldCheck size={24} weight="bold" />
          </div>

          <h2 id="legal-gate-title" style={{
            fontSize: 19, fontWeight: 800, letterSpacing: '-.02em', margin: '0 0 8px',
          }}>
            Our organiser terms have been updated
          </h2>

          <p style={{
            fontSize: 13.5, color: 'var(--text-2)', lineHeight: 1.65, margin: '0 0 18px',
          }}>
            Before you carry on, please review and accept the agreements that
            govern selling tickets on TicketHawk. They cover the commission
            split, when settlement runs, who carries a chargeback, and the
            refund policy your buyers are entitled to.
          </p>

          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 18 }}>
            {ORGANISER_LEGAL_DOCS.map(d => (
              <a key={d.key} {...legalLinkProps(d.key)} className="btn btn-sm"
                style={{ textDecoration: 'none' }}>
                {d.label}
                <ArrowSquareOut size={13} />
              </a>
            ))}
          </div>

          <label style={{
            display: 'flex', alignItems: 'flex-start', gap: 10,
            padding: '12px 14px', marginBottom: 16, cursor: 'pointer',
            background: 'var(--surface-alpha)',
            border: `1px solid ${accepted ? 'var(--accent-border)' : 'var(--border)'}`,
            borderRadius: 'var(--r-lg)',
            transition: 'border-color .15s ease',
          }}>
            <input
              type="checkbox"
              checked={accepted}
              onChange={e => setAccepted(e.target.checked)}
              style={{
                width: 16, height: 16, marginTop: 2,
                accentColor: 'var(--accent-deep)', cursor: 'pointer', flexShrink: 0,
              }}
            />
            <span style={{ fontSize: 12.5, lineHeight: 1.6, color: 'var(--text-2)' }}>
              I have read and agree to the Terms of Service, Privacy Policy,
              Organiser Agreement and Legal Notices.
            </span>
          </label>

          {error && (
            <div className="alert alert-danger" style={{ fontSize: 13 }}>{error}</div>
          )}

          <button
            type="button"
            className="btn btn-primary btn-full"
            disabled={!accepted || busy}
            onClick={accept}
          >
            {busy ? 'Saving…' : 'Accept and continue'}
          </button>

          {/* The only other way out. Offered explicitly so the dialog isn't a
              trap for someone who wants to read the documents properly first. */}
          <button
            type="button"
            onClick={onLogout}
            style={{
              display: 'block', width: '100%', marginTop: 12,
              background: 'none', border: 'none', cursor: 'pointer',
              fontSize: 12.5, color: 'var(--text-3)', fontFamily: 'inherit',
            }}
          >
            Sign out instead
          </button>
        </div>
      </div>
    </div>
  );
}