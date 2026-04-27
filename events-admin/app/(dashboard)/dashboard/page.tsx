'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '../../../lib/api';
import { fmtMoney, fmtDate, Badge } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';

interface Stats { ticket_revenue: number; merch_revenue: number; total_users: number; total_events: number; active_events: number; oversold_merch_orders: number; }
interface AdminEvent { id: string; title: string; status: string; start_time: string; tickets_sold: number; ticket_revenue: number; city: string; }

const th: React.CSSProperties = {
  padding: '.65rem 1rem', textAlign: 'left', fontFamily: 'var(--font-mono)',
  fontSize: '.65rem', textTransform: 'uppercase' as const, letterSpacing: '.07em',
  color: 'var(--text-3)', fontWeight: 500, borderBottom: '1px solid var(--border)',
};

export default function DashboardPage() {
  const toast = useToast();
  const router = useRouter();
  const [stats, setStats] = useState<Stats | null>(null);
  const [events, setEvents] = useState<AdminEvent[]>([]);

  useEffect(() => {
    Promise.all([
      api.get<Stats>('/admin/stats'),
      api.get<{ data: AdminEvent[] }>('/admin/events?limit=8'),
    ]).then(([s, e]) => {
      setStats(s.data);
      setEvents(e.data.data || []);
    }).catch(() => toast('Dashboard load failed', 'error'));
  }, [toast]);

  const totalRevenue = stats ? (stats.ticket_revenue || 0) + (stats.merch_revenue || 0) : 0;
  const oc = stats?.oversold_merch_orders || 0;

  return (
    <div style={{ padding: '1.5rem', maxWidth: '1400px' }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600, color: 'var(--text)' }}>Dashboard</div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: '.15rem' }}>Platform overview and metrics</div>
        </div>
        <button className="btn" onClick={() => window.location.reload()}>↺ Refresh</button>
      </div>

      {/* Stats grid */}
      <div className="stats-grid">
        {/* Total Revenue */}
        <div className="stat-card">
          <div className="stat-label">Total Revenue</div>
          <div className="stat-value">{stats ? fmtMoney(totalRevenue) : '—'}</div>
          <div className="stat-sub">Tickets + Merch</div>
        </div>
        {/* Ticket Revenue — accent */}
        <div className="stat-card accent">
          <div className="stat-label">Ticket Revenue</div>
          <div className="stat-value">{stats ? fmtMoney(stats.ticket_revenue) : '—'}</div>
          <div className="stat-sub">All confirmed bookings</div>
        </div>
        {/* Merch Revenue */}
        <div className="stat-card">
          <div className="stat-label">Merch Revenue</div>
          <div className="stat-value">{stats ? fmtMoney(stats.merch_revenue) : '—'}</div>
          <div className="stat-sub">Fulfilled orders</div>
        </div>
        {/* Oversold — danger if > 0 */}
        <div className={`stat-card${oc > 0 ? ' danger' : ''}`}>
          <div className="stat-label">Oversold Orders</div>
          <div className="stat-value">{stats ? String(oc) : '—'}</div>
          <div className="stat-sub">Requires attention</div>
        </div>
      </div>

      {/* Oversold alert */}
      {oc > 0 && (
        <div className="alert alert-warning" style={{ marginBottom: '1.5rem' }}>
          ⚠ {oc} oversold merch order{oc !== 1 ? 's' : ''} require attention.{' '}
          <a href="/oversold" style={{ color: 'var(--warning)', textDecoration: 'underline' }}>View →</a>
        </div>
      )}

      {/* Recent events table */}
      <div className="table-wrap" style={{ flex: 'none' }}>
        <div className="table-toolbar" style={{ justifyContent: 'space-between' }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.78rem', color: 'var(--text-2)' }}>RECENT EVENTS</span>
          <button className="btn btn-sm" onClick={() => router.push('/events')}>View All →</button>
        </div>
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '.85rem', tableLayout: 'fixed' }}>
            <thead>
              <tr>
                <th style={{ ...th, width: '35%' }}>Title</th>
                <th style={{ ...th, width: '12%' }}>Status</th>
                <th style={{ ...th, width: '15%' }}>Date</th>
                <th style={{ ...th, width: '14%' }}>Tickets Sold</th>
                <th style={{ ...th, width: '12%' }}>Revenue</th>
                <th style={{ ...th, width: '12%' }}>City</th>
              </tr>
            </thead>
            <tbody>
              {events.length === 0 ? (
                <tr>
                  <td colSpan={6} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-3)' }}>
                    {stats === null ? <span className="spinner" style={{ margin: 'auto', display: 'block' }} /> : 'No events found.'}
                  </td>
                </tr>
              ) : events.map(e => (
                <tr key={e.id} style={{ borderBottom: '1px solid rgba(45,45,61,.5)', cursor: 'pointer' }} onClick={() => router.push('/events')}>
                  <td style={{ padding: '.65rem 1rem', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{e.title}</td>
                  <td style={{ padding: '.65rem 1rem' }}><Badge status={e.status} /></td>
                  <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)', fontSize: '.78rem', color: 'var(--text-2)' }}>{fmtDate(e.start_time)}</td>
                  <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{e.tickets_sold}</td>
                  <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{fmtMoney(e.ticket_revenue)}</td>
                  <td style={{ padding: '.65rem 1rem', color: 'var(--text-2)' }}>{e.city}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}