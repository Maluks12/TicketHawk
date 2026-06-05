'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { pollPaymentStatus } from '@/lib/api';

type Phase = 'loading' | 'paying' | 'confirming' | 'done' | 'failed';

export default function PaymentPage({ params }: { params: { id: string } }) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const bookingId = params.id;
  const clientToken = searchParams.get('token') ?? '';
  const amount = searchParams.get('amount') ?? '0';
  const currency = searchParams.get('currency') ?? 'ZAR';

  const [phase, setPhase] = useState<Phase>('loading');
  const [message, setMessage] = useState('Loading payment…');
  const pollingRef = useRef(false);
  const iframeRef = useRef<HTMLIFrameElement>(null);

  const apiBase = process.env.NEXT_PUBLIC_API_URL ?? '';
  const paymentPageUrl = `${apiBase}/v1/payments/${bookingId}/page?client_token=${encodeURIComponent(clientToken)}`;

  const handleSuccess = useCallback(async () => {
    if (pollingRef.current) return;
    pollingRef.current = true;
    setPhase('confirming');
    setMessage('Confirming your payment…');
    try {
      const status = await pollPaymentStatus(bookingId, 2000, 120_000);
      if (status.payment_status === 'completed') {
        setPhase('done');
        setMessage('Payment confirmed! 🎟');
        setTimeout(() => router.replace('/tickets'), 1500);
      } else {
        setPhase('failed');
        setMessage('Payment could not be confirmed. Please check your tickets or try again.');
      }
    } catch {
      setPhase('failed');
      setMessage('Timed out waiting for payment confirmation. Your ticket may still arrive — check the Tickets tab.');
    }
  }, [bookingId, router]);

  // Listen for postMessage from the iframe (Stitch redirects)
  useEffect(() => {
    const handler = (event: MessageEvent) => {
      const url = event.data?.url ?? event.data ?? '';
      if (typeof url === 'string') {
        if (url.includes('payment/success')) handleSuccess();
        if (url.includes('payment/cancel')) { alert('Payment cancelled. You can retry from the checkout screen.'); router.back(); }
      }
    };
    window.addEventListener('message', handler);
    return () => window.removeEventListener('message', handler);
  }, [handleSuccess, router]);

  const fmtAmount = () => `${currency} ${Math.round(parseFloat(amount)).toLocaleString('en-ZA')}`;

  const overlayVisible = phase !== 'paying';

  return (
    <div style={{ minHeight: '100vh', background: '#fff', display: 'flex', flexDirection: 'column' }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', padding: '12px 16px', borderBottom: '1px solid #e5e5e5', background: '#fff' }}>
        {(phase === 'paying' || phase === 'loading') && (
          <button onClick={() => { if (confirm('Leave payment? Your reservation will be held for a few minutes.')) router.back(); }} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#ff3c6e', fontSize: 15, marginRight: 'auto' }}>
            Cancel
          </button>
        )}
        <span style={{ flex: 1, textAlign: 'center', fontSize: 16, fontWeight: 600, color: '#0a0a0a' }}>Secure Payment</span>
        <span style={{ fontSize: 14, color: '#666', marginLeft: 'auto' }}>{fmtAmount()}</span>
      </div>

      {/* iframe */}
      <div style={{ flex: 1, position: 'relative' }}>
        <iframe
          ref={iframeRef}
          src={paymentPageUrl}
          style={{ width: '100%', height: '100%', border: 'none', position: 'absolute', inset: 0, opacity: overlayVisible ? 0 : 1, pointerEvents: overlayVisible ? 'none' : 'auto' }}
          onLoad={() => { if (phase === 'loading') setPhase('paying'); }}
          onError={() => { setPhase('failed'); setMessage('Failed to load payment page. Check your connection and try again.'); }}
          allow="payment"
          title="Secure Payment"
        />

        {/* Overlay */}
        {overlayVisible && (
          <div style={{ position: 'absolute', inset: 0, background: '#fff', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 16, padding: 32, zIndex: 10 }}>
            {phase !== 'failed' && (
              <div style={{ width: 40, height: 40, border: `3px solid ${phase === 'done' ? '#00c853' : '#ff3c6e'}`, borderTopColor: 'transparent', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
            )}
            {phase === 'failed' && <span style={{ fontSize: 40 }}>⚠️</span>}
            <p style={{ margin: 0, fontSize: 16, color: phase === 'done' ? '#00c853' : '#333', textAlign: 'center', lineHeight: 1.5, fontWeight: phase === 'done' ? 700 : 400 }}>{message}</p>
            {phase === 'failed' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 10, width: '100%', maxWidth: 280 }}>
                <button onClick={() => router.back()} style={{ background: '#ff3c6e', color: '#fff', border: 'none', borderRadius: 12, padding: '12px 28px', fontWeight: 700, fontSize: 15, cursor: 'pointer' }}>Go back</button>
                <button onClick={() => router.replace('/tickets')} style={{ background: '#1a1a2e', color: '#fff', border: 'none', borderRadius: 12, padding: '12px 28px', fontWeight: 700, fontSize: 15, cursor: 'pointer' }}>Check Tickets tab</button>
              </div>
            )}
          </div>
        )}
      </div>

      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  );
}