'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '../../../lib/api';
import { fmtMoney, fmtDate, Badge } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';

interface Stats {
  ticket_revenue: number;
  merch_revenue: number;
  total_users: number;
  total_events: number;
  active_events: number;
  oversold_merch_orders: number;
}
interface AdminEvent {
  id: string; title: string; status: string; start_time: string;
  tickets_sold: number; ticket_revenue: number; city: string;
}
interface MerchRow    { id: string; is_active: boolean; }
interface Collection  { id: string; is_active: boolean; }

const th: React.CSSProperties = {
  padding: '.65rem 1rem', textAlign: 'left', fontFamily: 'var(--font-mono)',
  fontSize: '.65rem', textTransform: 'uppercase' as const, letterSpacing: '.07em',
  color: 'var(--text-3)', fontWeight: 500, borderBottom: '1px solid var(--border)',
};

export default function DashboardPage() {
  const toast  = useToast();
  const router = useRouter();

  const [stats,       setStats]       = useState<Stats | null>(null);
  const [events,      setEvents]      = useState<AdminEvent[]>([]);
  const [merchCount,  setMerchCount]  = useState<{ total: number; active: number } | null>(null);
  const [collCount,   setCollCount]   = useState<{ total: number; active: number } | null>(null);

  useEffect(() => {
    Promise.all([
      api.get<Stats>('/admin/stats'),
      api.get<{ data: AdminEvent[] }>('/admin/events?limit=8'),
      api.get<{ data: MerchRow[] }>('/admin/merch'),
      api.get<{ data: Collection[] }>('/admin/collections'),
    ]).then(([s, e, m, c]) => {
      setStats(s.data);
      setEvents(e.data.data || []);
      const merch = m.data.data || [];
      setMerchCount({ total: merch.length, active: merch.filter(i => i.is_active).length });
      const colls = c.data.data || [];
      setCollCount({ total: colls.length, active: colls.filter(i => i.is_active).length });
    }).catch(() => toast('Dashboard load failed', 'error'));
  }, [toast]);

  const totalRevenue = stats ? (stats.ticket_revenue || 0) + (stats.merch_revenue || 0) : 0;
  const oc = stats?.oversold_merch_orders || 0;

  return (
    <div style={{ padding: '1.5rem', maxWidth: '1400px' }}>

      {/* ── Header ── */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600, color: 'var(--text)' }}>Dashboard</div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: '.15rem' }}>Platform overview and metrics</div>
        </div>
        <button className="btn" onClick={() => window.location.reload()}>↺ Refresh</button>
      </div>

      {/* ── Revenue row ── */}
      <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.6rem', textTransform: 'uppercase', letterSpacing: '.1em', color: 'var(--text-3)', marginBottom: '.6rem' }}>Revenue</div>
      <div className="stats-grid" style={{ marginBottom: '1rem' }}>
        <div className="stat-card accent">
          <div className="stat-label">Total Revenue</div>
          <div className="stat-value">{stats ? fmtMoney(totalRevenue) : '—'}</div>
          <div className="stat-sub">Tickets + Merch combined</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Ticket Revenue</div>
          <div className="stat-value">{stats ? fmtMoney(stats.ticket_revenue) : '—'}</div>
          <div className="stat-sub">All confirmed bookings</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Merch Revenue</div>
          <div className="stat-value">{stats ? fmtMoney(stats.merch_revenue) : '—'}</div>
          <div className="stat-sub">Fulfilled orders</div>
        </div>
        <div className={`stat-card${oc > 0 ? ' danger' : ''}`}>
          <div className="stat-label">Oversold Orders</div>
          <div className="stat-value">{stats ? String(oc) : '—'}</div>
          <div className="stat-sub">{oc > 0 ? 'Requires attention' : 'All clear'}</div>
        </div>
      </div>

      {/* ── Platform row ── */}
      <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.6rem', textTransform: 'uppercase', letterSpacing: '.1em', color: 'var(--text-3)', marginBottom: '.6rem' }}>Platform</div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '1rem', marginBottom: '1.5rem' }}>
        {/* Users */}
        <div className="stat-card" style={{ cursor: 'pointer' }} onClick={() => router.push('/users')}>
          <div className="stat-label">Total Users</div>
          <div className="stat-value">{stats ? stats.total_users.toLocaleString() : '—'}</div>
          <div className="stat-sub">Registered accounts</div>
        </div>
        {/* Events */}
        <div className="stat-card" style={{ cursor: 'pointer' }} onClick={() => router.push('/events')}>
          <div className="stat-label">Events</div>
          <div className="stat-value">
            {stats ? (
              <span>
                {stats.active_events}
                <span style={{ fontSize: '.9rem', color: 'var(--text-3)', fontWeight: 400 }}> / {stats.total_events}</span>
              </span>
            ) : '—'}
          </div>
          <div className="stat-sub">Active / total</div>
        </div>
        {/* Merch items */}
        <div className="stat-card" style={{ cursor: 'pointer' }} onClick={() => router.push('/merch')}>
          <div className="stat-label">Merch Items</div>
          <div className="stat-value">
            {merchCount ? (
              <span>
                {merchCount.active}
                <span style={{ fontSize: '.9rem', color: 'var(--text-3)', fontWeight: 400 }}> / {merchCount.total}</span>
              </span>
            ) : '—'}
          </div>
          <div className="stat-sub">Active / total</div>
        </div>
        {/* Collections */}
        <div className="stat-card" style={{ cursor: 'pointer' }} onClick={() => router.push('/merch')}>
          <div className="stat-label">Collections</div>
          <div className="stat-value">
            {collCount ? (
              <span>
                {collCount.active}
                <span style={{ fontSize: '.9rem', color: 'var(--text-3)', fontWeight: 400 }}> / {collCount.total}</span>
              </span>
            ) : '—'}
          </div>
          <div className="stat-sub">Active / total</div>
        </div>
      </div>

      {/* ── Oversold alert ── */}
      {oc > 0 && (
        <div className="alert alert-warning" style={{ marginBottom: '1.5rem' }}>
          ⚠ {oc} oversold merch order{oc !== 1 ? 's' : ''} require attention.{' '}
          <a href="/oversold" style={{ color: 'var(--warning)', textDecoration: 'underline' }}>View oversold →</a>
        </div>
      )}

      {/* ── Recent events table ── */}
      <div className="table-wrap" style={{ flex: 'none' }}>
        <div className="table-toolbar" style={{ justifyContent: 'space-between' }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.78rem', color: 'var(--text-2)' }}>RECENT EVENTS</span>
          <button className="btn btn-sm" onClick={() => router.push('/events')}>View All →</button>
        </div>
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '.85rem', tableLayout: 'fixed' }}>
            <thead>
              <tr>
                <th style={{ ...th, width: '36%' }}>Title</th>
                <th style={{ ...th, width: '12%' }}>Status</th>
                <th style={{ ...th, width: '16%' }}>Date</th>
                <th style={{ ...th, width: '13%' }}>Tickets Sold</th>
                <th style={{ ...th, width: '12%' }}>Revenue</th>
                <th style={{ ...th, width: '11%' }}>City</th>
              </tr>
            </thead>
            <tbody>
              {events.length === 0 ? (
                <tr>
                  <td colSpan={6} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-3)' }}>
                    {stats === null
                      ? <span className="spinner" style={{ margin: 'auto', display: 'block' }} />
                      : 'No events found.'}
                  </td>
                </tr>
              ) : events.map(e => (
                <tr
                  key={e.id}
                  style={{ borderBottom: '1px solid rgba(45,45,61,.5)', cursor: 'pointer' }}
                  onClick={() => router.push('/events')}
                >
                  <td style={{ padding: '.65rem 1rem', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{e.title}</td>
                  <td style={{ padding: '.65rem 1rem' }}><Badge status={e.status} /></td>
                  <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)', fontSize: '.78rem', color: 'var(--text-2)' }}>{fmtDate(e.start_time)}</td>
                  <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{e.tickets_sold.toLocaleString()}</td>
                  <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{fmtMoney(e.ticket_revenue)}</td>
                  <td style={{ padding: '.65rem 1rem', color: 'var(--text-2)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{e.city}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* ── Quick links ── */}
      <div style={{ display: 'flex', gap: '.75rem', marginTop: '1.25rem', flexWrap: 'wrap' }}>
        {[
          { label: '◇ Merch & Collections', href: '/merch' },
          { label: '◉ Users',               href: '/users' },
          { label: '⚠ Oversold Orders',     href: '/oversold' },
        ].map(l => (
          <button key={l.href} className="btn btn-sm" onClick={() => router.push(l.href)}>{l.label}</button>
        ))}
      </div>
    </div>
  );
}