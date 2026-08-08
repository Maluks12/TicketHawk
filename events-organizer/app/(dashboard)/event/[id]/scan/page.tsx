'use client';
import { useState, useEffect, useRef, useCallback } from 'react';
import type { CSSProperties } from 'react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { api } from '../../../../../lib/api';

// ── Types ─────────────────────────────────────────────────────────────────────

type ScanPhase =
  | 'idle'          // camera initialising
  | 'ready'         // camera live, waiting for QR
  | 'processing'    // API call in-flight
  | 'admitted'      // success
  | 'already_used'  // 409 with first_scan_at
  | 'invalid'       // bad token / business rule failure
  | 'cam_error';    // no camera permission

interface ScanResult {
  phase: 'admitted' | 'already_used' | 'invalid';
  slotNumber?: number;
  firstScanAt?: string;
  message?: string;
}

// ── Helpers ───────────────────────────────────────────────────────────────────

/**
 * Extracts the assignmentId ("aid") from a rotating QR token.
 * Token format: base64url(json_payload).<hmac_sig>
 * The Go service uses RawURL encoding (no padding, - and _ instead of + and /).
 */
function extractAssignmentId(token: string): string {
  const lastDot = token.lastIndexOf('.');
  if (lastDot < 0) throw new Error('Malformed token');

  const b64raw = token.substring(0, lastDot);
  const b64 = b64raw.replace(/-/g, '+').replace(/_/g, '/');
  const padded = b64 + '='.repeat((4 - (b64.length % 4)) % 4);

  let payload: Record<string, unknown>;
  try {
    payload = JSON.parse(atob(padded));
  } catch {
    throw new Error('Token payload is not valid JSON');
  }

  if (typeof payload.aid !== 'string' || !payload.aid) {
    throw new Error('Token missing assignment ID');
  }
  return payload.aid;
}

function fmtTime(iso: string) {
  return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function ScanPage() {
  const { id: eventId } = useParams<{ id: string }>();

  const videoRef    = useRef<HTMLVideoElement>(null);
  const controlsRef = useRef<{ stop: () => void } | null>(null);
  const cooldownRef = useRef<number>(0); // timestamp: don't process a new scan before this

  const [eventTitle, setEventTitle] = useState('');
  const [phase, setPhase]           = useState<ScanPhase>('idle');
  const [result, setResult]         = useState<ScanResult | null>(null);
  const [admitted, setAdmitted]     = useState(0);

  // ── Fetch event name for the header ────────────────────────────────────────
  useEffect(() => {
    api.get(`/organiser/dashboard/events/${eventId}/stats`)
      .then(res => setEventTitle(res.data?.event?.title ?? ''))
      .catch(() => {});
  }, [eventId]);

  // ── Reset back to ready after showing a result ─────────────────────────────
  const scheduleReset = useCallback(() => {
    setTimeout(() => {
      setResult(null);
      setPhase('ready');
      // Re-arm the scanner immediately — the cooldown was held during the card
      // display (3.5 s), so we reset it here so the next code scans instantly.
      cooldownRef.current = 0;
    }, 3500);
  }, []);

  // ── Process a decoded QR string ───────────────────────────────────────────
  const handleScan = useCallback(async (rawToken: string) => {
    // Cooldown: ignore repeated decodes of the same code while result is displayed
    if (Date.now() < cooldownRef.current) return;
    cooldownRef.current = Date.now() + 5000; // 5 s window

    let assignmentId: string;
    try {
      assignmentId = extractAssignmentId(rawToken);
    } catch {
      setResult({ phase: 'invalid', message: 'Not a valid ticket QR code' });
      setPhase('invalid');
      scheduleReset();
      return;
    }

    setPhase('processing');

    try {
      const res = await api.post(`/tickets/${assignmentId}/scan`, { token: rawToken });
      setResult({ phase: 'admitted', slotNumber: res.data?.slot_number });
      setPhase('admitted');
      setAdmitted(n => n + 1);
    } catch (err: unknown) {
      const axiosErr = err as { response?: { status?: number; data?: { error?: string; first_scan_at?: string } } };
      const status   = axiosErr.response?.status;
      const data     = axiosErr.response?.data;

      if (status === 409 && data?.error === 'Ticket already scanned') {
        // first_scan_at is present when the backend could retrieve the timestamp;
        // it may be absent if the row was deleted in a race — handle both cases.
        setResult({ phase: 'already_used', firstScanAt: data.first_scan_at });
        setPhase('already_used');
      } else {
        // 422 = invalid/expired QR token  |  403 = wrong organizer
        // 404 = ticket not found          |  409 (other) = outside window / status
        // All surface as 'invalid' with the backend's error message.
        setResult({ phase: 'invalid', message: data?.error ?? 'Ticket could not be verified' });
        setPhase('invalid');
      }
    }

    scheduleReset();
  }, [scheduleReset]);

  // ── Start camera + continuous QR scanner ──────────────────────────────────
  useEffect(() => {
    let stopped = false;

    (async () => {
      try {
        // Dynamically import to avoid SSR issues — @zxing/browser is browser-only
        const { BrowserQRCodeReader } = await import('@zxing/browser');
        const { DecodeHintType }      = await import('@zxing/library');
        if (stopped || !videoRef.current) return;

        // TRY_HARDER makes ZXing work hard on every frame — essential for
        // real-world scanning (angled phones, screen glare, partial framing).
        // Without it the default decoder demands a nearly-perfect QR alignment.
        const hints = new Map<typeof DecodeHintType[keyof typeof DecodeHintType], unknown>();
        hints.set(DecodeHintType.TRY_HARDER, true);

        const reader = new BrowserQRCodeReader(hints);
        const controls = await reader.decodeFromConstraints(
          {
            video: {
              facingMode: 'environment',
              width:  { ideal: 1280 },
              height: { ideal: 720 },
            },
          },
          videoRef.current,
          // @zxing/browser 0.2.x calls this on every frame:
          // result is populated on a successful decode, error on a missed frame.
          (result, _err) => { if (result) handleScan(result.getText()); },
        );

        if (stopped) {
          controls.stop();
          return;
        }

        controlsRef.current = controls;
        setPhase('ready');
      } catch {
        if (!stopped) setPhase('cam_error');
      }
    })();

    return () => {
      stopped = true;
      controlsRef.current?.stop();
    };
  }, [handleScan]);

  // ── Derived booleans ──────────────────────────────────────────────────────
  const showingResult = phase === 'admitted' || phase === 'already_used' || phase === 'invalid';
  const showViewfinder = (phase === 'ready' || phase === 'processing') && !showingResult;

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      background: '#0a0a0a', overflow: 'hidden',
    }}>

      {/* ── Header ─────────────────────────────────────────────────────────── */}
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '10px 16px', background: 'rgba(0,0,0,.75)',
        backdropFilter: 'blur(8px)', flexShrink: 0,
        borderBottom: '1px solid rgba(255,255,255,.08)',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <Link href={`/event/${eventId}`} style={{
            display: 'flex', alignItems: 'center', gap: 5,
            color: 'rgba(255,255,255,.65)', textDecoration: 'none',
            fontSize: 13, fontWeight: 500,
          }}>
            <svg viewBox="0 0 20 20" fill="currentColor" width="15" height="15">
              <path fillRule="evenodd" d="M9.707 14.707a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414l4-4a1 1 0 011.414 1.414L7.414 9H15a1 1 0 110 2H7.414l2.293 2.293a1 1 0 010 1.414z" clipRule="evenodd"/>
            </svg>
            Back
          </Link>
          <span style={{ color: 'rgba(255,255,255,.2)' }}>·</span>
          <span style={{ color: '#fff', fontWeight: 700, fontSize: 14 }}>
            {eventTitle || 'Scan Tickets'}
          </span>
        </div>

        {/* Admission counter */}
        <div style={{
          display: 'flex', alignItems: 'center', gap: 6,
          background: admitted > 0 ? 'rgba(66,183,42,.18)' : 'rgba(255,255,255,.06)',
          border: `1px solid ${admitted > 0 ? 'rgba(66,183,42,.35)' : 'rgba(255,255,255,.1)'}`,
          borderRadius: 99, padding: '4px 12px', transition: 'all .3s',
        }}>
          <div style={{
            width: 7, height: 7, borderRadius: '50%',
            background: admitted > 0 ? 'var(--success)' : 'rgba(255,255,255,.3)',
          }} />
          <span style={{
            fontWeight: 700, fontSize: 13,
            color: admitted > 0 ? 'var(--success)' : 'rgba(255,255,255,.45)',
          }}>
            {admitted} admitted
          </span>
        </div>
      </div>

      {/* ── Camera area ────────────────────────────────────────────────────── */}
      <div style={{ flex: 1, position: 'relative', overflow: 'hidden' }}>

        {/* Live video feed */}
        <video
          ref={videoRef}
          autoPlay playsInline muted
          style={{
            width: '100%', height: '100%',
            objectFit: 'cover', display: 'block',
          }}
        />

        {/* ── Initialising overlay ─────────────────────────────────────────── */}
        {phase === 'idle' && (
          <div style={{
            position: 'absolute', inset: 0, background: '#0a0a0a',
            display: 'flex', flexDirection: 'column',
            alignItems: 'center', justifyContent: 'center', gap: 14,
          }}>
            <div className="spinner spinner-lg" style={{
              borderColor: 'rgba(255,255,255,.15)',
              borderTopColor: 'rgba(255,255,255,.8)',
            }} />
            <span style={{ color: 'rgba(255,255,255,.4)', fontSize: 14 }}>
              Starting camera…
            </span>
          </div>
        )}

        {/* ── Camera error overlay ─────────────────────────────────────────── */}
        {phase === 'cam_error' && (
          <div style={{
            position: 'absolute', inset: 0, background: '#0a0a0a',
            display: 'flex', flexDirection: 'column',
            alignItems: 'center', justifyContent: 'center',
            gap: 14, padding: 32, textAlign: 'center',
          }}>
            <div style={{ fontSize: 44, lineHeight: 1 }}>📷</div>
            <div style={{ color: '#fff', fontSize: 18, fontWeight: 700 }}>
              Camera unavailable
            </div>
            <div style={{ color: 'rgba(255,255,255,.45)', fontSize: 14, maxWidth: 280 }}>
              Grant camera permission in your browser and reload the page, or use a
              device with a rear-facing camera for best results.
            </div>
            <button
              onClick={() => window.location.reload()}
              className="btn btn-primary"
              style={{ marginTop: 8 }}
            >
              Reload
            </button>
          </div>
        )}

        {/* ── Viewfinder + scan line ────────────────────────────────────────── */}
        {showViewfinder && (
          <div style={{
            position: 'absolute', inset: 0,
            display: 'flex', flexDirection: 'column',
            alignItems: 'center', justifyContent: 'center',
            pointerEvents: 'none',
          }}>
            {/* Vignette */}
            <div style={{
              position: 'absolute', inset: 0,
              background: 'radial-gradient(ellipse 65% 65% at center, transparent 0%, rgba(0,0,0,.6) 100%)',
            }} />

            {/* Finder box */}
            <div style={{
              position: 'relative', width: 248, height: 248,
              borderRadius: 14,
              boxShadow: '0 0 0 9999px rgba(0,0,0,.45)',
            }}>
              {/* Corner brackets */}
              {([
                { top: 0,    left: 0,    borderRight: 'none', borderBottom: 'none', borderRadius: '10px 0 0 0' },
                { top: 0,    right: 0,   borderLeft: 'none',  borderBottom: 'none', borderRadius: '0 10px 0 0' },
                { bottom: 0, left: 0,    borderRight: 'none', borderTop: 'none',    borderRadius: '0 0 0 10px' },
                { bottom: 0, right: 0,   borderLeft: 'none',  borderTop: 'none',    borderRadius: '0 0 10px 0' },
              ] as CSSProperties[]).map((style, i) => (
                <div key={i} style={{
                  position: 'absolute', width: 22, height: 22,
                  border: '3px solid var(--accent)',
                  ...style,
                }} />
              ))}

              {/* Animated scan line */}
              <div style={{
                position: 'absolute', left: 4, right: 4, height: 2,
                background: 'linear-gradient(90deg, transparent 0%, var(--accent) 40%, var(--info) 50%, var(--accent) 60%, transparent 100%)',
                borderRadius: 2,
                animation: 'qrScanLine 1.8s ease-in-out infinite',
                opacity: phase === 'processing' ? 0 : 1,
              }} />

              {/* Processing spinner in centre */}
              {phase === 'processing' && (
                <div style={{
                  position: 'absolute', inset: 0,
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                }}>
                  <div className="spinner spinner-lg" style={{
                    borderColor: 'rgba(255,255,255,.2)',
                    borderTopColor: 'var(--accent)',
                  }} />
                </div>
              )}
            </div>

            <div style={{
              marginTop: 22, color: 'rgba(255,255,255,.75)',
              fontSize: 14, fontWeight: 500,
              background: 'rgba(0,0,0,.5)', padding: '6px 14px',
              borderRadius: 99, backdropFilter: 'blur(4px)',
            }}>
              {phase === 'processing' ? 'Verifying…' : 'Align QR code within the frame'}
            </div>
          </div>
        )}

        {/* ── Result card overlay ───────────────────────────────────────────── */}
        {showingResult && result && (
          <div style={{
            position: 'absolute', inset: 0,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            background: 'rgba(0,0,0,.72)', backdropFilter: 'blur(6px)',
          }}>
            <div style={{
              background: 'var(--bg-elevated)', borderRadius: 18,
              padding: '32px 36px', maxWidth: 320, width: '90%',
              textAlign: 'center',
              boxShadow: '0 24px 80px rgba(0,0,0,.5)',
              animation: 'qrCardIn .2s cubic-bezier(.34,1.56,.64,1)',
            }}>

              {/* ── Admitted ── */}
              {result.phase === 'admitted' && (
                <>
                  <div style={{
                    width: 68, height: 68, borderRadius: '50%',
                    background: 'var(--success-bg)',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    margin: '0 auto 18px',
                  }}>
                    <svg viewBox="0 0 24 24" width="34" height="34" fill="none"
                      stroke="var(--success-fg)" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                      <polyline points="20 6 9 17 4 12"/>
                    </svg>
                  </div>
                  <div style={{ fontSize: 26, fontWeight: 800, color: 'var(--success-fg)', letterSpacing: '-.5px' }}>
                    Admitted
                  </div>
                  {result.slotNumber != null && (
                    <div style={{ fontSize: 14, color: 'var(--text-2)', marginTop: 6 }}>
                      Ticket slot #{result.slotNumber}
                    </div>
                  )}
                  <div style={{
                    marginTop: 18, fontSize: 12, color: 'var(--text-3)',
                    display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
                  }}>
                    <div style={{
                      width: 6, height: 6, borderRadius: '50%', background: 'var(--success)',
                      animation: 'pulse 1.2s ease infinite',
                    }} />
                    Resuming scanner…
                  </div>
                </>
              )}

              {/* ── Already used ── */}
              {result.phase === 'already_used' && (
                <>
                  <div style={{
                    width: 68, height: 68, borderRadius: '50%',
                    background: 'var(--warning-bg)',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    margin: '0 auto 18px',
                  }}>
                    <svg viewBox="0 0 24 24" width="34" height="34" fill="none"
                      stroke="var(--warning-fg)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <circle cx="12" cy="12" r="10"/>
                      <line x1="12" y1="8" x2="12" y2="12"/>
                      <line x1="12" y1="16" x2="12.01" y2="16"/>
                    </svg>
                  </div>
                  <div style={{ fontSize: 26, fontWeight: 800, color: 'var(--warning-fg)', letterSpacing: '-.5px' }}>
                    Already Used
                  </div>
                  {result.firstScanAt && (
                    <div style={{
                      marginTop: 8, fontSize: 13, color: 'var(--warning-fg)',
                      background: 'var(--warning-bg)', borderRadius: 8, padding: '6px 12px', display: 'inline-block',
                    }}>
                      First scanned at {fmtTime(result.firstScanAt)}
                    </div>
                  )}
                  <div style={{ marginTop: 14, fontSize: 12, color: 'var(--text-3)' }}>Resuming scanner…</div>
                </>
              )}

              {/* ── Invalid ── */}
              {result.phase === 'invalid' && (
                <>
                  <div style={{
                    width: 68, height: 68, borderRadius: '50%',
                    background: 'var(--danger-bg)',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    margin: '0 auto 18px',
                  }}>
                    <svg viewBox="0 0 24 24" width="34" height="34" fill="none"
                      stroke="var(--danger)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <circle cx="12" cy="12" r="10"/>
                      <line x1="15" y1="9" x2="9" y2="15"/>
                      <line x1="9" y1="9" x2="15" y2="15"/>
                    </svg>
                  </div>
                  <div style={{ fontSize: 26, fontWeight: 800, color: 'var(--danger)', letterSpacing: '-.5px' }}>
                    Invalid
                  </div>
                  <div style={{ fontSize: 13, color: 'var(--text-2)', marginTop: 6 }}>
                    {result.message ?? 'Ticket could not be verified'}
                  </div>
                  <div style={{ marginTop: 14, fontSize: 12, color: 'var(--text-3)' }}>Resuming scanner…</div>
                </>
              )}
            </div>
          </div>
        )}
      </div>

      {/* ── Animations ────────────────────────────────────────────────────────── */}
      <style>{`
        @keyframes qrScanLine {
          0%   { top: 6px;   opacity: 1; }
          48%  { opacity: 1; }
          50%  { top: calc(100% - 8px); opacity: .6; }
          52%  { opacity: 1; }
          100% { top: 6px;   opacity: 1; }
        }
        @keyframes qrCardIn {
          from { opacity: 0; transform: scale(.85); }
          to   { opacity: 1; transform: scale(1); }
        }
        @keyframes pulse {
          0%, 100% { opacity: 1; transform: scale(1); }
          50%       { opacity: .4; transform: scale(.7); }
        }
      `}</style>
    </div>
  );
}