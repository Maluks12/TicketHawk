'use client';
import { useState, useEffect } from 'react';
import Link from 'next/link';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { fmtMoney, fmtDate } from '../../../lib/utils';

interface DashStats {
  total_events: number;
  published_events: number;
  total_bookings: number;
  total_revenue: number;
  active_campaigns: number;
  total_impressions: number;
  total_clicks: number;
  total_ad_spend: number;
}

interface RecentEvent {
  id: string;
  title: string;
  status: string;
  start_time: string;
  tickets_sold: number;
  max_capacity: number | null;
  price: number;
}

interface RecentCampaign {
  id: string;
  status: string;
  daily_budget: number;
  today_spent: number;
  total_impressions: number;
  total_clicks: number;
  event?: { title: string };
}

export default function DashboardPage() {
  const toast = useToast();
  const [stats, setStats] = useState<DashStats | null>(null);
  const [events, setEvents] = useState<RecentEvent[]>([]);
  const [campaigns, setCampaigns] = useState<RecentCampaign[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      api.get('/organiser/dashboard/stats'),
      api.get('/organiser/dashboard/events'),
      api.get('/organiser/campaigns'),
    ]).then(([statsRes, evRes, campRes]) => {
      setStats(statsRes.data);
      setEvents((evRes.data.events ?? []).slice(0, 5));
      setCampaigns((campRes.data.campaigns ?? []).filter((c: RecentCampaign) => c.status === 'active').slice(0, 4));
    }).catch(() => toast('Failed to load dashboard', 'error'))
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
        <div className="spinner spinner-lg" />
      </div>
    );
  }

  return (
    <div className="page">

      {/* ── Page header ───────────────────────────────────────────────── */}
      <div className="page-header">
        <div>
          <div className="page-title">Overview</div>
          <div style={{ fontSize: 13, color: '#606770', marginTop: 2 }}>
            {new Date().toLocaleDateString('en-ZA', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Link href="/campaigns/create" className="btn btn-primary">
            + Create campaign
          </Link>
        </div>
      </div>

      <div className="page-content">

        {/* ── Stat cards — 4 across like Meta's account overview ──────── */}
        <div className="stat-grid">
          <StatCard
            label="Published events"
            value={stats?.published_events ?? 0}
            sub={`of ${stats?.total_events ?? 0} total`}
            accent="blue"
            icon={
              <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
                <path fillRule="evenodd" d="M6 2a1 1 0 00-1 1v1H4a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V6a2 2 0 00-2-2h-1V3a1 1 0 10-2 0v1H7V3a1 1 0 00-1-1zm0 5a1 1 0 000 2h8a1 1 0 100-2H6z" clipRule="evenodd"/>
              </svg>
            }
          />
          <StatCard
            label="Total revenue"
            value={fmtMoney(stats?.total_revenue ?? 0)}
            sub="from ticket sales"
            accent="green"
            icon={
              <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
                <path d="M8.433 7.418c.155-.103.346-.196.567-.267v1.698a2.305 2.305 0 01-.567-.267C8.07 8.34 8 8.114 8 8c0-.114.07-.34.433-.582zM11 12.849v-1.698c.22.071.412.164.567.267.364.243.433.468.433.582 0 .114-.07.34-.433.582a2.305 2.305 0 01-.567.267z"/>
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-13a1 1 0 10-2 0v.092a4.535 4.535 0 00-1.676.662C6.602 6.234 6 7.009 6 8c0 .99.602 1.765 1.324 2.246.48.32 1.054.545 1.676.662v1.941c-.391-.127-.68-.317-.843-.504a1 1 0 10-1.51 1.31c.562.649 1.413 1.076 2.353 1.253V15a1 1 0 102 0v-.092a4.535 4.535 0 001.676-.662C13.398 13.766 14 12.991 14 12c0-.99-.602-1.765-1.324-2.246A4.535 4.535 0 0011 9.092V7.151c.391.127.68.317.843.504a1 1 0 101.511-1.31c-.563-.649-1.413-1.076-2.354-1.253V5z" clipRule="evenodd"/>
              </svg>
            }
          />
          <StatCard
            label="Total bookings"
            value={(stats?.total_bookings ?? 0).toLocaleString()}
            sub="confirmed tickets"
            accent="orange"
            icon={
              <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
                <path d="M2 6a2 2 0 012-2h12a2 2 0 012 2v2a2 2 0 100 4v2a2 2 0 01-2 2H4a2 2 0 01-2-2v-2a2 2 0 100-4V6z"/>
              </svg>
            }
          />
          <StatCard
            label="Active campaigns"
            value={stats?.active_campaigns ?? 0}
            sub={`${(stats?.total_impressions ?? 0).toLocaleString()} total impressions`}
            accent="blue"
            icon={
              <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
                <path d="M2 11a1 1 0 011-1h2a1 1 0 011 1v5a1 1 0 01-1 1H3a1 1 0 01-1-1v-5zM8 7a1 1 0 011-1h2a1 1 0 011 1v9a1 1 0 01-1 1H9a1 1 0 01-1-1V7zM14 4a1 1 0 011-1h2a1 1 0 011 1v12a1 1 0 01-1 1h-2a1 1 0 01-1-1V4z"/>
              </svg>
            }
          />
        </div>

        {/* ── Two-column lower section ─────────────────────────────────── */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>

          {/* Recent Events */}
          <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden' }}>
            <div style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '14px 16px', borderBottom: '1px solid #ebedf0',
            }}>
              <div style={{ fontSize: 15, fontWeight: 700 }}>Recent events</div>
              <Link href="/events" style={{ fontSize: 13, color: '#1877f2' }}>See all</Link>
            </div>
            {events.length === 0 ? (
              <div style={{ padding: 24, textAlign: 'center', color: '#8a8d91', fontSize: 13 }}>
                No events yet.{' '}
                <Link href="/events" style={{ color: '#1877f2' }}>Create one</Link>
              </div>
            ) : (
              <div>
                {events.map((ev, i) => (
                  <div key={ev.id} style={{
                    display: 'flex', alignItems: 'center', gap: 12,
                    padding: '10px 16px',
                    borderBottom: i < events.length - 1 ? '1px solid #f0f2f5' : 'none',
                  }}>
                    <div style={{
                      width: 36, height: 36, borderRadius: 6, background: '#e7f3ff',
                      display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
                    }}>
                      <svg viewBox="0 0 20 20" fill="#1877f2" width="16" height="16">
                        <path fillRule="evenodd" d="M6 2a1 1 0 00-1 1v1H4a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V6a2 2 0 00-2-2h-1V3a1 1 0 10-2 0v1H7V3a1 1 0 00-1-1zm0 5a1 1 0 000 2h8a1 1 0 100-2H6z" clipRule="evenodd"/>
                      </svg>
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontSize: 14, fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {ev.title}
                      </div>
                      <div style={{ fontSize: 12, color: '#8a8d91', marginTop: 1 }}>
                        {fmtDate(ev.start_time)} · {ev.tickets_sold} tickets sold
                      </div>
                    </div>
                    <span className={`badge badge-${ev.status}`} style={{ flexShrink: 0 }}>
                      {ev.status}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Active Campaigns */}
          <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden' }}>
            <div style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '14px 16px', borderBottom: '1px solid #ebedf0',
            }}>
              <div style={{ fontSize: 15, fontWeight: 700 }}>Active campaigns</div>
              <Link href="/campaigns" style={{ fontSize: 13, color: '#1877f2' }}>Manage</Link>
            </div>
            {campaigns.length === 0 ? (
              <div style={{ padding: 24, textAlign: 'center' }}>
                <div style={{ fontSize: 13, color: '#8a8d91', marginBottom: 12 }}>
                  No active campaigns. Boost an event to reach more people.
                </div>
                <Link href="/campaigns/create" className="btn btn-primary btn-sm">
                  Create campaign
                </Link>
              </div>
            ) : (
              <div>
                {campaigns.map((c, i) => {
                  const ctr = c.total_impressions > 0
                    ? ((c.total_clicks / c.total_impressions) * 100).toFixed(2)
                    : '0.00';
                  const budgetUsed = c.daily_budget > 0
                    ? Math.min((c.today_spent / c.daily_budget) * 100, 100)
                    : 0;
                  return (
                    <div key={c.id} style={{
                      padding: '12px 16px',
                      borderBottom: i < campaigns.length - 1 ? '1px solid #f0f2f5' : 'none',
                    }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                        <div style={{ fontSize: 14, fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1, marginRight: 8 }}>
                          {c.event?.title ?? 'Unknown'}
                        </div>
                        <span className="badge badge-active" style={{ flexShrink: 0 }}>Active</span>
                      </div>
                      <div style={{ display: 'flex', gap: 16, fontSize: 12, color: '#606770', marginBottom: 8 }}>
                        <span>Today: {fmtMoney(c.today_spent)}</span>
                        <span>Budget: {fmtMoney(c.daily_budget)}/day</span>
                        <span>CTR: {ctr}%</span>
                      </div>
                      {/* Budget progress bar */}
                      <div className="progress-bar">
                        <div className="progress-fill" style={{ width: `${budgetUsed}%` }} />
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        {/* ── Ad performance summary row ───────────────────────────────── */}
        {(stats?.active_campaigns ?? 0) > 0 && (
          <div style={{
            marginTop: 16, background: '#fff', border: '1px solid #dddfe2',
            borderRadius: 8, padding: '16px 20px',
          }}>
            <div style={{ fontSize: 15, fontWeight: 700, marginBottom: 12 }}>
              Ad performance summary
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 0 }}>
              {[
                { label: 'Total impressions', value: (stats?.total_impressions ?? 0).toLocaleString() },
                { label: 'Total clicks', value: (stats?.total_clicks ?? 0).toLocaleString() },
                {
                  label: 'Average CTR',
                  value: stats && stats.total_impressions > 0
                    ? `${((stats.total_clicks / stats.total_impressions) * 100).toFixed(2)}%`
                    : '0.00%',
                },
                { label: 'Total ad spend', value: fmtMoney(stats?.total_ad_spend ?? 0) },
              ].map((item, i, arr) => (
                <div key={item.label} style={{
                  padding: '0 20px',
                  borderRight: i < arr.length - 1 ? '1px solid #ebedf0' : 'none',
                }}>
                  <div style={{ fontSize: 12, color: '#606770', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '.04em', marginBottom: 4 }}>
                    {item.label}
                  </div>
                  <div style={{ fontSize: 22, fontWeight: 700, color: '#1c1e21' }}>{item.value}</div>
                </div>
              ))}
            </div>
          </div>
        )}

      </div>
    </div>
  );
}

// ── Stat Card ─────────────────────────────────────────────────────────────────
function StatCard({
  label, value, sub, accent, icon,
}: {
  label: string;
  value: string | number;
  sub: string;
  accent: 'blue' | 'green' | 'orange' | 'red';
  icon: React.ReactNode;
}) {
  const colors = { blue: '#1877f2', green: '#1a7f37', orange: '#e65c00', red: '#c41c1c' };
  const bgs    = { blue: '#e7f3ff', green: '#f0fff4', orange: '#fff8f0', red: '#fff5f5' };
  const color  = colors[accent];
  const bg     = bgs[accent];

  return (
    <div style={{
      background: '#fff', border: '1px solid #dddfe2', borderRadius: 8,
      padding: '16px', borderTop: `3px solid ${color}`,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
        <div style={{ fontSize: 12, fontWeight: 600, color: '#606770', textTransform: 'uppercase', letterSpacing: '.04em' }}>
          {label}
        </div>
        <div style={{ width: 32, height: 32, borderRadius: 6, background: bg, display: 'flex', alignItems: 'center', justifyContent: 'center', color }}>
          {icon}
        </div>
      </div>
      <div style={{ fontSize: 26, fontWeight: 700, color: '#1c1e21', lineHeight: 1 }}>{value}</div>
      <div style={{ fontSize: 12, color: '#8a8d91', marginTop: 4 }}>{sub}</div>
    </div>
  );
}