'use client';
import { useState, useEffect } from 'react';
import Link from 'next/link';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { fmtMoney, fmtDate } from '../../../lib/utils';
import { getNotifications, markRead, NOTIF_KYC_APPROVED, Notification } from '../../../lib/notifications';
import { StatCard, StatusPill } from '@/components/ui';

// Mirrors repository.OrganizerStats as serialised by
// GET /organiser/dashboard/stats. The names matter: this endpoint returns the
// struct directly, so a mismatched key doesn't error — it silently reads as
// undefined and renders as zero. `total_revenue` was exactly that bug; the
// field is called total_net_earnings here. (A different handler does expose
// `total_revenue`, which is presumably where the wrong name came from.)
interface DashStats {
  total_events: number;
  published_events: number;
  total_bookings: number;
  total_tickets_sold: number;
  total_net_earnings: number;
  gross_ticket_revenue: number;
  platform_fees_paid: number;
  bookings_today: number;
  revenue_today: number;
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

export default function DashboardPage() {
  const toast = useToast();
  const [stats, setStats] = useState<DashStats | null>(null);
  const [events, setEvents] = useState<RecentEvent[]>([]);
  const [loading, setLoading] = useState(true);

  // Unread "your verification was approved" notification, if there is one.
  // Kept out of the Promise.all above on purpose: a notifications hiccup must
  // never take the whole Overview screen down to an error toast.
  const [kycNotice, setKycNotice] = useState<Notification | null>(null);

  useEffect(() => {
    Promise.all([
      api.get('/organiser/dashboard/stats'),
      api.get('/organiser/dashboard/events'),
    ]).then(([statsRes, evRes]) => {
      setStats(statsRes.data);
      setEvents((evRes.data.events ?? []).slice(0, 5));
    }).catch(() => toast('Failed to load dashboard', 'error'))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    getNotifications(20).then((list) => {
      setKycNotice(list.find((n) => n.type === NOTIF_KYC_APPROVED && !n.is_read) ?? null);
    });
  }, []);

  // Dismiss = mark read server-side, so it doesn't reappear on the next login.
  function dismissKycNotice() {
    if (kycNotice) markRead(kycNotice.id);
    setKycNotice(null);
  }

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
          <div style={{ fontSize: 13, color: 'var(--text-2)', marginTop: 2 }}>
            {new Date().toLocaleDateString('en-ZA', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Link href="/events/create" className="btn btn-primary">
            + Create event
          </Link>
        </div>
      </div>

      <div className="page-content">

        {/* ── KYC approval notice ──────────────────────────────────────────
            Emitted once by the backend the moment full verification passes,
            whichever path got them there. Sits above the stats because an
            organiser who has been waiting on review shouldn't have to go
            hunting through /kyc to find out they were approved. */}
        {kycNotice && (
          <div style={{
            display: 'flex', alignItems: 'flex-start', gap: 12,
            background: 'var(--success-bg)', border: '1px solid var(--success-border)', borderRadius: 8,
            padding: '14px 16px', marginBottom: 16,
          }}>
            <div style={{
              width: 28, height: 28, borderRadius: '50%', background: 'var(--success-fg)',
              color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 15, fontWeight: 700, flexShrink: 0,
            }}>
              ✓
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: 14.5, fontWeight: 700, color: 'var(--success-fg)' }}>
                {kycNotice.title}
              </div>
              <div style={{ fontSize: 13, color: 'var(--success-fg)', marginTop: 3, lineHeight: 1.5 }}>
                {kycNotice.body}
              </div>
              <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
                <Link href="/events/create" className="btn btn-sm btn-primary">
                  Create a paid event
                </Link>
                <Link href="/kyc" className="btn btn-sm">
                  Payout details
                </Link>
              </div>
            </div>
            <button type="button" onClick={dismissKycNotice} aria-label="Dismiss"
              style={{
                background: 'none', border: 'none', cursor: 'pointer',
                color: 'var(--success-fg)', fontSize: 18, lineHeight: 1, padding: 2, flexShrink: 0,
              }}>
              ×
            </button>
          </div>
        )}

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
            label="Net earnings"
            value={fmtMoney(stats?.total_net_earnings ?? 0)}
            sub={`${fmtMoney(stats?.gross_ticket_revenue ?? 0)} gross · ${fmtMoney(stats?.platform_fees_paid ?? 0)} fees`}
            accent="green"
            icon={
              <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
                <path d="M8.433 7.418c.155-.103.346-.196.567-.267v1.698a2.305 2.305 0 01-.567-.267C8.07 8.34 8 8.114 8 8c0-.114.07-.34.433-.582zM11 12.849v-1.698c.22.071.412.164.567.267.364.243.433.468.433.582 0 .114-.07.34-.433.582a2.305 2.305 0 01-.567.267z"/>
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-13a1 1 0 10-2 0v.092a4.535 4.535 0 00-1.676.662C6.602 6.234 6 7.009 6 8c0 .99.602 1.765 1.324 2.246.48.32 1.054.545 1.676.662v1.941c-.391-.127-.68-.317-.843-.504a1 1 0 10-1.51 1.31c.562.649 1.413 1.076 2.353 1.253V15a1 1 0 102 0v-.092a4.535 4.535 0 001.676-.662C13.398 13.766 14 12.991 14 12c0-.99-.602-1.765-1.324-2.246A4.535 4.535 0 0011 9.092V7.151c.391.127.68.317.843.504a1 1 0 101.511-1.31c-.563-.649-1.413-1.076-2.354-1.253V5z" clipRule="evenodd"/>
              </svg>
            }
          />
          <StatCard
            label="Tickets sold"
            value={(stats?.total_tickets_sold ?? 0).toLocaleString()}
            sub={`across ${(stats?.total_bookings ?? 0).toLocaleString()} confirmed bookings`}
            accent="orange"
            icon={
              <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
                <path d="M2 6a2 2 0 012-2h12a2 2 0 012 2v2a2 2 0 100 4v2a2 2 0 01-2 2H4a2 2 0 01-2-2v-2a2 2 0 100-4V6z"/>
              </svg>
            }
          />
          <StatCard
            label="Today"
            value={fmtMoney(stats?.revenue_today ?? 0)}
            sub={`${(stats?.bookings_today ?? 0).toLocaleString()} bookings since midnight`}
            accent="grey"
            icon={
              <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-12a1 1 0 10-2 0v4a1 1 0 00.293.707l2.5 2.5a1 1 0 001.414-1.414L11 9.586V6z" clipRule="evenodd"/>
              </svg>
            }
          />
        </div>

        {/* ── Two-column lower section ─────────────────────────────────── */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>

          {/* Recent Events */}
          <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, overflow: 'hidden' }}>
            <div style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '14px 16px', borderBottom: '1px solid var(--border)',
            }}>
              <div style={{ fontSize: 15, fontWeight: 700 }}>Recent events</div>
              <Link href="/events" style={{ fontSize: 13, color: 'var(--accent)' }}>See all</Link>
            </div>
            {events.length === 0 ? (
              <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>
                No events yet.{' '}
                <Link href="/events" style={{ color: 'var(--accent)' }}>Create one</Link>
              </div>
            ) : (
              <div>
                {events.map((ev, i) => (
                  <div key={ev.id} style={{
                    display: 'flex', alignItems: 'center', gap: 12,
                    padding: '10px 16px',
                    borderBottom: i < events.length - 1 ? '1px solid var(--border)' : 'none',
                  }}>
                    <div style={{
                      width: 36, height: 36, borderRadius: 6, background: 'var(--accent-soft)',
                      display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
                    }}>
                      <svg viewBox="0 0 20 20" fill="var(--accent)" width="16" height="16">
                        <path fillRule="evenodd" d="M6 2a1 1 0 00-1 1v1H4a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V6a2 2 0 00-2-2h-1V3a1 1 0 10-2 0v1H7V3a1 1 0 00-1-1zm0 5a1 1 0 000 2h8a1 1 0 100-2H6z" clipRule="evenodd"/>
                      </svg>
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontSize: 14, fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {ev.title}
                      </div>
                      <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 1 }}>
                        {fmtDate(ev.start_time)} · {ev.tickets_sold} tickets sold
                      </div>
                    </div>
                    <StatusPill status={ev.status} style={{ flexShrink: 0 }} />
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Quick actions — campaigns removed for launch; ads may return later */}
          <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, overflow: 'hidden' }}>
            <div style={{ padding: '14px 16px', borderBottom: '1px solid var(--border)' }}>
              <div style={{ fontSize: 15, fontWeight: 700 }}>Quick actions</div>
            </div>
            <div style={{ padding: 8 }}>
              {[
                { href: '/events/create', title: 'Create an event', desc: 'Publish free or paid events — they appear in the TicketHawk feed organically.' },
                { href: '/events', title: 'Manage events', desc: 'Edit details, track sales, and manage door staff per event.' },
                { href: '/kyc', title: 'Verification & payouts', desc: 'Confirm your identity and bank details so ticket money can reach you.' },
              ].map((a) => (
                <Link key={a.href} href={a.href} style={{
                  display: 'block', padding: '12px 10px', borderRadius: 6,
                  textDecoration: 'none', color: 'inherit',
                }}>
                  <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--accent)' }}>{a.title}</div>
                  <div style={{ fontSize: 12, color: 'var(--text-2)', marginTop: 2 }}>{a.desc}</div>
                </Link>
              ))}
            </div>
          </div>
        </div>

      </div>
    </div>
  );
}