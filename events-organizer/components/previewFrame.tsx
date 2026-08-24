'use client';
import { useCallback, useEffect, useState } from 'react';
import { ArrowSquareOut } from '@phosphor-icons/react';
import { createEventPreview } from '../lib/eventContent';

/**
 * Device-framed preview of the REAL page.
 *
 * ── Why an iframe and not a React replica ───────────────────────────────────
 *
 * Rebuilding the event page as components inside the dashboard ships faster
 * and is wrong within a month: the replica drifts from the page it imitates
 * the moment either side changes, and a preview that lies is worse than no
 * preview, because the organiser makes decisions on it.
 *
 * Framing the actual page means any change to the public site shows up here
 * for free, and there is exactly one implementation to keep correct.
 *
 * ── The token ───────────────────────────────────────────────────────────────
 *
 * A draft is not publicly readable, so the frame carries a short-lived signed
 * token — ten minutes, scoped to one event, minted only after the server has
 * verified the caller owns it. That means the preview URL is not something to
 * leave lying around in a shared doc, and it means a tab left open overnight
 * needs re-minting rather than silently showing stale content.
 */
export default function PreviewFrame({
  eventId,
  organiserPath,
  webBase,
}: {
  /** Event preview — mints a token. Omit for an organiser-page preview. */
  eventId?: string;
  /** Organiser page path, e.g. /organiser/wilderness-beach. Public, no token. */
  organiserPath?: string;
  /** Public site origin. Falls back to the API-derived value. */
  webBase?: string;
}) {
  const [device, setDevice] = useState<'mobile' | 'desktop'>('mobile');
  const [url, setUrl] = useState<string | null>(null);
  const [expiresAt, setExpiresAt] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [nonce, setNonce] = useState(0);

  const base = webBase
    ?? process.env.NEXT_PUBLIC_WEB_URL
    // Last resort: strip the /v1 off the API URL. Right in production, where
    // api.ticket-hawk.com and ticket-hawk.com are siblings; wrong locally,
    // which is why NEXT_PUBLIC_WEB_URL exists.
    ?? (process.env.NEXT_PUBLIC_API_URL ?? '').replace(/\/v1\/?$/, '').replace(/^https:\/\/api\./, 'https://');

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      if (eventId) {
        const link = await createEventPreview(eventId);
        setUrl(link.web_url);
        setExpiresAt(Date.parse(link.expires_at));
      } else if (organiserPath) {
        setUrl(`${base}${organiserPath}`);
        setExpiresAt(null);
      }
    } catch {
      setError('Could not create a preview link. Try again in a moment.');
    } finally {
      setLoading(false);
    }
  }, [eventId, organiserPath, base]);

  useEffect(() => { load(); }, [load, nonce]);

  // Countdown, so an organiser knows the frame is going stale before it does
  // rather than after — a preview that quietly expired looks like a bug.
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    if (!expiresAt) return;
    const t = setInterval(() => setNow(Date.now()), 15_000);
    return () => clearInterval(t);
  }, [expiresAt]);

  const secondsLeft = expiresAt ? Math.max(0, Math.floor((expiresAt - now) / 1000)) : null;
  const expired = secondsLeft !== null && secondsLeft <= 0;

  const frameW = device === 'mobile' ? 390 : 1180;

  return (
    <div>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10,
        marginBottom: 14, flexWrap: 'wrap',
      }}>
        <div style={{ display: 'flex', gap: 4, background: 'var(--surface-2)', padding: 4, borderRadius: 'var(--r-lg)' }}>
          <button type="button" className={`btn btn-sm${device === 'mobile' ? ' btn-primary' : ''}`}
            onClick={() => setDevice('mobile')}>
            Mobile
          </button>
          <button type="button" className={`btn btn-sm${device === 'desktop' ? ' btn-primary' : ''}`}
            onClick={() => setDevice('desktop')}>
            Desktop
          </button>
        </div>

        <button type="button" className="btn btn-sm" onClick={() => setNonce(n => n + 1)}>
          ↻ Refresh
        </button>

        {url && !expired && (
          <a className="btn btn-sm btn-ghost" href={url} target="_blank" rel="noreferrer">
            <ArrowSquareOut size={13} weight="bold" /> Open in a tab
          </a>
        )}

        {secondsLeft !== null && (
          <span style={{ fontSize: 11.5, color: expired ? 'var(--accent)' : 'var(--text-3)', marginLeft: 'auto' }}>
            {expired
              ? 'Preview link expired'
              : `Link expires in ${Math.ceil(secondsLeft / 60)} min`}
          </span>
        )}
      </div>

      {error && <div className="alert alert-danger" style={{ marginBottom: 12 }}>{error}</div>}

      {expired && (
        <div className="alert alert-warning" style={{ marginBottom: 12 }}>
          This preview link has expired. Refresh to get a new one — the page
          itself has not changed.
        </div>
      )}

      <div style={{
        background: 'var(--surface-2)', borderRadius: 'var(--r-xl)',
        border: '1px solid var(--border)', padding: 20,
        display: 'flex', justifyContent: 'center', overflowX: 'auto',
      }}>
        {loading ? (
          <div style={{ padding: 60 }}><div className="spinner spinner-lg" /></div>
        ) : url && !expired ? (
          <div style={{
            width: frameW, maxWidth: '100%', height: device === 'mobile' ? 780 : 720,
            borderRadius: device === 'mobile' ? 28 : 10,
            overflow: 'hidden', background: '#000',
            border: '1px solid var(--border)', flexShrink: 0,
          }}>
            <iframe
              key={`${url}-${device}-${nonce}`}
              src={url}
              title="Preview"
              style={{ width: '100%', height: '100%', border: 'none', background: '#fff' }}
              // The preview renders our own site, but it is still a document we
              // do not want navigating the dashboard or opening dialogs.
              sandbox="allow-scripts allow-same-origin"
            />
          </div>
        ) : (
          <div style={{ padding: 60, color: 'var(--text-3)', fontSize: 13 }}>
            Nothing to preview yet.
          </div>
        )}
      </div>
    </div>
  );
}