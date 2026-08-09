'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import { Broadcast } from '@phosphor-icons/react';
import { api } from '@/lib/api';

interface RotatingQRToken {
  token:      string;
  expires_at: string;
  window_sec: number;
  remaining?: number; // pool tokens only — admissions left in the pool
}

interface Props {
  // Individual-assignment mode. Optional so pool mode (endpoint-only) can omit it.
  assignmentId?: string;
  size?: number;
  // Override the token endpoint — used for the group pool QR (`…/pool-qr`).
  endpoint?: string;
  // Pool mode only: reports admissions remaining on each refresh.
  onRemaining?: (n: number) => void;
}

export function RotatingQR({ assignmentId, size = 200, endpoint, onRemaining }: Props) {
  const [token,    setToken]    = useState<string | null>(null);
  const [windowSec, setWindowSec] = useState(30);
  const [secsLeft, setSecsLeft] = useState(30);
  const [loading,  setLoading]  = useState(true);
  const [error,    setError]    = useState(false);
  const [progress, setProgress] = useState(1); // 1 = full, 0 = empty

  const refreshTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const countdownTimer = useRef<ReturnType<typeof setInterval> | null>(null);
  const progressTimer  = useRef<ReturnType<typeof setInterval> | null>(null);
  const expiresAtRef   = useRef<number>(0);
  const mounted        = useRef(true);

  const clearTimers = useCallback(() => {
    if (refreshTimer.current)   clearTimeout(refreshTimer.current);
    if (countdownTimer.current) clearInterval(countdownTimer.current);
    if (progressTimer.current)  clearInterval(progressTimer.current);
  }, []);

  const fetchToken = useCallback(async () => {
    try {
      const res = await api.get<RotatingQRToken>(endpoint ?? `/v1/tickets/${assignmentId}/qr`);
      if (!mounted.current) return;

      const { token, expires_at, window_sec, remaining } = res.data;
      if (typeof remaining === 'number') onRemaining?.(remaining);
      const exp    = new Date(expires_at).getTime();
      const now    = Date.now();
      const msLeft = Math.max(exp - now, 0);

      expiresAtRef.current = exp;
      setToken(token);
      setWindowSec(window_sec);
      setSecsLeft(Math.ceil(msLeft / 1000));
      setProgress(msLeft / (window_sec * 1000));
      setError(false);
      setLoading(false);

      // Tick countdown
      if (countdownTimer.current) clearInterval(countdownTimer.current);
      countdownTimer.current = setInterval(() => {
        if (!mounted.current) return;
        setSecsLeft(Math.max(0, Math.ceil((expiresAtRef.current - Date.now()) / 1000)));
      }, 500);

      // Smooth progress bar
      if (progressTimer.current) clearInterval(progressTimer.current);
      const total = window_sec * 1000;
      progressTimer.current = setInterval(() => {
        if (!mounted.current) return;
        const left = Math.max(0, expiresAtRef.current - Date.now());
        setProgress(left / total);
      }, 200);

      // Refresh 2s before expiry
      if (refreshTimer.current) clearTimeout(refreshTimer.current);
      refreshTimer.current = setTimeout(fetchToken, Math.max(msLeft - 2000, 1000));
    } catch {
      if (!mounted.current) return;
      setError(true);
      setLoading(false);
      refreshTimer.current = setTimeout(fetchToken, 5000);
    }
  }, [assignmentId, endpoint, onRemaining]);

  useEffect(() => {
    mounted.current = true;
    fetchToken();
    // Refresh on tab visibility
    const onVisible = () => {
      if (document.visibilityState === 'visible' && Date.now() >= expiresAtRef.current - 1000) {
        clearTimers();
        fetchToken();
      }
    };
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      mounted.current = false;
      clearTimers();
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [fetchToken, clearTimers]);

  const urgency = secsLeft <= 4 ? 'crit' : secsLeft <= 9 ? 'warn' : 'ok';
  const barColor = urgency === 'crit' ? '#ff4d4d' : urgency === 'warn' ? '#ffb830' : '#00e5a0';

  if (loading) return (
    <div style={{ width: size, height: size, borderRadius: 12, background: 'rgba(255,255,255,0.04)', border: '1px solid rgba(255,255,255,0.06)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8 }}>
      <div style={{ width: 24, height: 24, border: '3px solid rgba(255,255,255,0.2)', borderTopColor: '#fff', borderRadius: '50%', animation: 'qr-spin 0.8s linear infinite' }} />
      <style>{`@keyframes qr-spin { to { transform: rotate(360deg); } }`}</style>
      <p style={{ margin: 0, fontSize: 12, color: 'rgba(255,255,255,0.25)' }}>Loading ticket…</p>
    </div>
  );

  if (error || !token) return (
    <div
      onClick={() => { setLoading(true); setError(false); fetchToken(); }}
      style={{ width: size, height: size, borderRadius: 12, background: 'rgba(255,255,255,0.04)', border: '1px solid rgba(255,255,255,0.06)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, cursor: 'pointer' }}
    >
      <Broadcast size={28} weight="regular" style={{ opacity: 0.3 }} />
      <p style={{ margin: 0, fontSize: 12, color: 'rgba(255,255,255,0.25)' }}>Could not load QR</p>
      <p style={{ margin: 0, fontSize: 11, color: 'rgba(255,255,255,0.15)' }}>Tap to retry</p>
    </div>
  );

  // Generate QR as SVG via a public API (no npm dep needed)
  const qrUrl = `https://api.qrserver.com/v1/create-qr-code/?size=${size - 20}x${size - 20}&data=${encodeURIComponent(token)}&format=svg&bgcolor=ffffff&color=0a0a0a&margin=4`;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10 }}>
      {/* White QR box */}
      <div style={{ width: size, height: size, background: '#fff', borderRadius: 12, display: 'flex', alignItems: 'center', justifyContent: 'center', overflow: 'hidden', boxShadow: '0 2px 12px rgba(0,0,0,0.4)' }}>
        <img src={qrUrl} alt="Live ticket QR" width={size - 20} height={size - 20} style={{ display: 'block' }} />
      </div>

      {/* Progress bar */}
      <div style={{ width: size, height: 3, borderRadius: 99, background: 'rgba(255,255,255,0.08)', overflow: 'hidden' }}>
        <div style={{ height: '100%', borderRadius: 99, background: barColor, width: `${Math.round(progress * 100)}%`, transition: 'width 0.2s linear, background 0.3s' }} />
      </div>

      {/* Status row */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <div style={{ width: 6, height: 6, borderRadius: 3, background: barColor }} />
        <p style={{ margin: 0, fontSize: 11, fontWeight: 600, color: barColor, letterSpacing: 0.2 }}>
          {urgency === 'crit' ? 'Refreshing…' : `Live ticket · refreshes in ${secsLeft}s`}
        </p>
      </div>
    </div>
  );
}