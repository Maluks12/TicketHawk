'use client';
import { useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { fmtMoney, fmtDateTime, Badge } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useStaff, AccessDenied } from '../../../lib/permissions';

interface OversoldOrder { id: string; merch_name?: string; quantity: number; total_amount?: number; amount?: number; variant_sku?: string; fulfillment_status: string; created_at: string; }

export default function OversoldPage() {
  const toast = useToast();
  const [orders, setOrders] = useState<OversoldOrder[]>([]);
  const [loading, setLoading] = useState(true);

  // Oversold orders are customer purchase records — amounts, SKUs, order ids.
  // event.view is the floor, matching the endpoint's own gate.
  const capsLoaded = useStaff((st) => st.loaded);
  const can = useStaff((st) => st.can);
  const loadCaps = useStaff((st) => st.load);
  useEffect(() => { loadCaps(); }, [loadCaps]);

  async function load() {
    setLoading(true);
    try {
      const { data } = await api.get<{ data: OversoldOrder[] }>('/admin/merch/oversold');
      setOrders(data.data || []);
    } catch { toast('Failed to load oversold orders', 'error'); }
    finally { setLoading(false); }
  }

  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  if (!capsLoaded) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }
  if (!can('event.view')) return <AccessDenied need="event.view" />;

  return (
    <div style={{ padding: '1.5rem', maxWidth: '1400px' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600 }}>Oversold Orders</div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: '.15rem' }}>Orders where payment captured but stock exhausted — requires manual resolution</div>
        </div>
        <button className="btn btn-sm" onClick={load}>↺ Refresh</button>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: '2rem' }}><div className="spinner" style={{ margin: 'auto' }} /></div>
      ) : orders.length === 0 ? (
        <div className="alert alert-info">✓ No oversold orders — all clear!</div>
      ) : (
        <>
          <div className="alert alert-warning" style={{ marginBottom: '1.5rem' }}>
            ⚠ {orders.length} oversold order{orders.length !== 1 ? 's' : ''} require manual resolution — issue a refund via Stitch or source additional stock.
          </div>
          {orders.map(o => (
            <div key={o.id} className="oversold-card">
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '.4rem' }}>
                <div style={{ fontWeight: 500 }}>{o.merch_name || 'Merchandise Item'}</div>
                <div style={{ fontFamily: 'var(--font-mono)', color: 'var(--danger)', fontSize: '1rem', fontWeight: 600 }}>{fmtMoney(o.total_amount ?? o.amount)}</div>
              </div>
              <div style={{ fontSize: '.8rem', color: 'var(--text-2)', display: 'flex', gap: '1rem', flexWrap: 'wrap' }}>
                <span>Order: <code style={{ fontFamily: 'var(--font-mono)', fontSize: '.75rem' }}>{o.id}</code></span>
                <span>Qty: {o.quantity || 1}</span>
                <span>SKU: {o.variant_sku || '—'}</span>
                <span>Status: <Badge status={o.fulfillment_status || 'oversold'} /></span>
                <span>Date: {fmtDateTime(o.created_at)}</span>
              </div>
              <div style={{ marginTop: '.4rem', fontSize: '.78rem', color: 'var(--text-3)' }}>Issue a refund via Stitch or source additional stock and mark as fulfilled.</div>
            </div>
          ))}
        </>
      )}
    </div>
  );
}