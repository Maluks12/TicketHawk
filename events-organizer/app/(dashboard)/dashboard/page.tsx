'use client';
import { useState, useEffect } from 'react';
import Link from 'next/link';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { fmtMoney, fmtDate } from '../../../lib/utils';
import { getNotifications, markRead, NOTIF_KYC_APPROVED, Notification } from '../../../lib/notifications';
import { StatusPill } from '@/components/ui';
import { OverviewSummaryRows } from '@/components/overviewRows';

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
            Create event
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

        {/* ── Summary rows ─────────────────────────────────────────────
            Two shallow rows: the event coming up next, and the average across
            every event already held so the first row has something to be read
            against. Deliberately six metrics each and no controls — the deep
            cuts live on /analytics. Self-fetching and silent on failure, so an
            analytics outage costs two rows rather than the whole screen. */}
        <OverviewSummaryRows />

        {/* ── Stat cards — 4 across like Meta's account overview ──────── */}
        <div className="metric-row">
          <div className="metric">
            <div className="metric-label">Published events</div>
            <div className="metric-value">{stats?.published_events ?? 0}</div>
            <div className="metric-sub">of {stats?.total_events ?? 0} total</div>
          </div>
          <div className="metric">
            <div className="metric-label">Net earnings</div>
            <div className="metric-value">{fmtMoney(stats?.total_net_earnings ?? 0)}</div>
            <div className="metric-sub">
              {fmtMoney(stats?.gross_ticket_revenue ?? 0)} gross · {fmtMoney(stats?.platform_fees_paid ?? 0)} fees
            </div>
          </div>
          <div className="metric">
            <div className="metric-label">Tickets sold</div>
            <div className="metric-value">{(stats?.total_tickets_sold ?? 0).toLocaleString()}</div>
            <div className="metric-sub">
              across {(stats?.total_bookings ?? 0).toLocaleString()} confirmed bookings
            </div>
          </div>
          <div className="metric">
            <div className="metric-label">Today</div>
            <div className="metric-value">{fmtMoney(stats?.revenue_today ?? 0)}</div>
            <div className="metric-sub">
              {(stats?.bookings_today ?? 0).toLocaleString()} bookings since midnight
            </div>
          </div>
        </div>

        {/* ── Two-column lower section ─────────────────────────────────── */}
        {/* marginTop separates the stats band from Recent events / Quick
            actions. .page-content is plain block flow with no row gap, so the
            spacing has to live here. Not set on .metric-row itself, because
            that class has a `+ .metric-row { margin-top: -1px }` rule for
            stacking two bands into one seam — a bottom margin would split it. */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginTop: 28 }}>

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