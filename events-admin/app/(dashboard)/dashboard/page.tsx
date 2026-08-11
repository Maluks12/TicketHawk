'use client';
import { useCallback, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { fmtMoney } from '../../../lib/utils';

// ─────────────────────────────────────────────────────────────────────────────
// Dashboard
//
// The old one showed eight equal-weight stat cards and then a single wide table
// of recent events, which made every number look equally important and buried
// the one thing that needs acting on — events waiting for approval.
//
// Now: two rows of stats that answer two different questions, then a split
// section with the five most recent events on the left and the five waiting for
// approval on the right. Everything is clickable through to where you'd act.
//
// GMV VS REVENUE
//
// The old "Total Revenue R47 110" was SUM(subtotal) — the face value of tickets
// sold, almost all of which belongs to organisers. This separates the two:
// volume (GMV) sits in one row, what TicketHawk actually keeps in the next.
// ─────────────────────────────────────────────────────────────────────────────

interface GMV {
  ticket_gmv: number; merch_gmv: number; total_gmv: number; buyer_paid: number;
  booking_fees: number; commission: number; merch_revenue: number;
  our_revenue: number; take_rate: number | null; merch_revenue_known: boolean;
  organiser_psr: number;
  tickets_sold: number; tickets_free: number; tickets_paid: number;
  events_public_free: number; events_public_paid: number;
  events_private_free: number; events_private_paid: number;
  events_pending_count: number;
  total_users: number; active_users: number; organisers: number;
}

interface DashEvent {
  id: string; title: string; status: string; start_time: string;
  city: string | null; price: number; tickets_sold: number;
  revenue: number; submitted_at: string;
}

export default function DashboardPage() {
  const router = useRouter();
  const toast = useToast();

  const [gmv, setGmv] = useState<GMV | null>(null);
  const [recent, setRecent] = useState<DashEvent[]>([]);
  const [pending, setPending] = useState<DashEvent[]>([]);
  // `loading` starts true and is only ever set FALSE. Flipping it back to true
  // at the top of load() would be a synchronous setState inside the mount
  // effect — a cascading render, and the thing the lint rule is pointing at.
  // The Refresh button owns `refreshing` instead, because a button click is an
  // event handler, not an effect.
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async () => {
    try {
      // Settled individually: a failure in one list must not blank the stats,
      // and vice versa. Promise.all would discard a good response because its
      // sibling failed.
      const [g, l] = await Promise.allSettled([
        api.get('/admin/stats/gmv'),
        api.get('/admin/dashboard/lists'),
      ]);
      if (g.status === 'fulfilled') setGmv(g.value.data);
      if (l.status === 'fulfilled') {
        setRecent(l.value.data?.recent ?? []);
        setPending(l.value.data?.pending ?? []);
      }
      if (g.status === 'rejected' && l.status === 'rejected') {
        toast('Could not load dashboard', 'error');
      }
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [toast]);

  useEffect(() => { load(); }, [load]);

  function refresh() {
    setRefreshing(true);
    load();
  }

  if (loading && !gmv) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }

  const pct = (n: number | null) => (n === null ? '—' : `${(n * 100).toFixed(1)}%`);

  return (
    <div style={{ padding: '24px 28px 48px', maxWidth: 1600, margin: '0 auto' }}>
      <div style={{
        display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between',
        gap: 16, marginBottom: 22,
      }}>
        <div>
          <div className="page-title">Dashboard</div>
          <div className="page-sub">Platform overview and metrics</div>
        </div>
        <button className="btn btn-sm" onClick={refresh} disabled={refreshing}>
          {refreshing ? 'Refreshing…' : 'Refresh'}
        </button>
      </div>

      {/* ── Row 1: volume ──────────────────────────────────────────────────
          What moved through the platform. None of this is our money. */}
      <SectionLabel>Volume — value transacted</SectionLabel>
      <div className="stats-grid">
        <Stat
          label="Total GMV" accent="var(--accent)"
          value={fmtMoney(gmv?.total_gmv ?? 0)}
          sub="Tickets + merch at face value"
        />
        <Stat
          label="Ticket GMV" accent="var(--info)"
          value={fmtMoney(gmv?.ticket_gmv ?? 0)}
          sub={`${(gmv?.tickets_sold ?? 0).toLocaleString('en-ZA')} tickets sold`}
        />
        <Stat
          label="Merch GMV" accent="var(--warning)"
          value={fmtMoney(gmv?.merch_gmv ?? 0)}
          sub="Fulfilled orders only"
        />
        <Stat
          label="Buyer paid" accent="var(--text-3)"
          value={fmtMoney(gmv?.buyer_paid ?? 0)}
          sub="Cash through the platform"
        />
      </div>

      {/* ── Row 2: what we keep ────────────────────────────────────────────── */}
      <SectionLabel>Revenue — what TicketHawk keeps</SectionLabel>
      <div className="stats-grid">
        <Stat
          label="Our revenue" accent="var(--success)" emphasis
          value={fmtMoney(gmv?.our_revenue ?? 0)}
          sub={`${pct(gmv?.take_rate ?? null)} take rate`}
        />
        <Stat
          label="Booking fees" accent="var(--success)"
          value={fmtMoney(gmv?.booking_fees ?? 0)}
          sub="Buyer-paid service fee"
        />
        <Stat
          label="Commission" accent="var(--success)"
          value={fmtMoney(gmv?.commission ?? 0)}
          sub="6% of ticket face value"
        />
        <Stat
          label="Merch revenue" accent="var(--text-3)"
          value={gmv?.merch_revenue_known ? fmtMoney(gmv.merch_revenue) : '—'}
          // Says WHY rather than showing a confident zero. A zero here would
          // read as "merch earns us nothing", which is a different and wrong
          // claim from "we don't store the rate".
          sub={gmv?.merch_revenue_known ? 'Merch commission' : 'No commission rate stored'}
        />
      </div>

      {/* ── Split section ──────────────────────────────────────────────────── */}
      <div style={{
        display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(440px, 1fr))',
        gap: 18, marginTop: 26, alignItems: 'start',
      }}>
        <EventList
          title="Recent events"
          sub="Newest first"
          events={recent}
          emptyText="No events yet."
          onSeeAll={() => router.push('/events')}
          onPick={(e) => router.push(`/events?focus=${e.id}`)}
        />

        <EventList
          title="Waiting for approval"
          // Oldest first is the server's ordering and it matters: the event
          // that has waited longest is closest to its own start date.
          sub={pending.length > 0 ? 'Longest wait first' : 'Nothing pending'}
          events={pending}
          emptyText="Nothing waiting — the queue is clear."
          badge={gmv?.events_pending_count ?? pending.length}
          onSeeAll={() => router.push('/approvals')}
          onPick={() => router.push('/approvals')}
          urgent
        />
      </div>

      {/* ── Composition ────────────────────────────────────────────────────
          Deliberately last and deliberately small. These are context, not
          things to act on, and putting them in the stat rows was a big part of
          why the top of the page felt like noise. */}
      <div className="card" style={{ marginTop: 18 }}>
        <div className="card-header">
          <div className="card-title">Composition</div>
        </div>
        <div className="card-body" style={{
          display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 20,
        }}>
          <MiniGroup title="Tickets" rows={[
            ['Paid', (gmv?.tickets_paid ?? 0).toLocaleString('en-ZA')],
            ['Free', (gmv?.tickets_free ?? 0).toLocaleString('en-ZA')],
          ]} />
          <MiniGroup title="Public events" rows={[
            ['Paid', String(gmv?.events_public_paid ?? 0)],
            ['Free', String(gmv?.events_public_free ?? 0)],
          ]} />
          <MiniGroup title="Private events" rows={[
            ['Paid', String(gmv?.events_private_paid ?? 0)],
            ['Free', String(gmv?.events_private_free ?? 0)],
          ]} />
          <MiniGroup title="People" rows={[
            ['Users', (gmv?.total_users ?? 0).toLocaleString('en-ZA')],
            ['Active (90d)', (gmv?.active_users ?? 0).toLocaleString('en-ZA')],
            ['Organisers', String(gmv?.organisers ?? 0)],
          ]} />
          <MiniGroup title="Organiser side" rows={[
            ['Paid out (PSR)', fmtMoney(gmv?.organiser_psr ?? 0)],
          ]} />
        </div>
      </div>
    </div>
  );
}

// ── Pieces ───────────────────────────────────────────────────────────────────

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div style={{
      fontSize: 10.5, fontWeight: 800, letterSpacing: '.12em',
      textTransform: 'uppercase', color: 'var(--text-3)', margin: '0 0 10px',
    }}>
      {children}
    </div>
  );
}

function Stat({
  label, value, sub, accent, emphasis = false,
}: {
  label: string; value: string; sub?: string; accent: string; emphasis?: boolean;
}) {
  return (
    <div className="stat-card" style={{
      boxShadow: `inset 3px 0 0 ${accent}`,
      // The headline number gets a tint so the eye lands on it first. One
      // emphasised card per row — more than that is the same as none.
      background: emphasis ? 'var(--success-bg)' : 'var(--surface)',
    }}>
      <div className="stat-label">{label}</div>
      <div className="stat-value" style={emphasis ? { color: 'var(--success-fg)' } : undefined}>
        {value}
      </div>
      {sub && <div className="stat-sub">{sub}</div>}
    </div>
  );
}

function MiniGroup({ title, rows }: { title: string; rows: [string, string][] }) {
  return (
    <div>
      <div style={{
        fontSize: 10, fontWeight: 800, letterSpacing: '.1em', textTransform: 'uppercase',
        color: 'var(--text-3)', marginBottom: 8,
      }}>
        {title}
      </div>
      {rows.map(([k, v]) => (
        <div key={k} style={{
          display: 'flex', justifyContent: 'space-between', gap: 12,
          fontSize: 13, padding: '3px 0',
        }}>
          <span style={{ color: 'var(--text-2)' }}>{k}</span>
          <span style={{ fontWeight: 700, fontVariantNumeric: 'tabular-nums' }}>{v}</span>
        </div>
      ))}
    </div>
  );
}

function EventList({
  title, sub, events, emptyText, badge, onSeeAll, onPick, urgent = false,
}: {
  title: string;
  sub: string;
  events: DashEvent[];
  emptyText: string;
  badge?: number;
  onSeeAll: () => void;
  onPick: (e: DashEvent) => void;
  urgent?: boolean;
}) {
  return (
    <div className="card">
      <div className="card-header">
        <div>
          <div className="card-title" style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {title}
            {badge !== undefined && badge > 0 && (
              <span className={`badge badge-${urgent ? 'pending' : 'draft'}`}>{badge}</span>
            )}
          </div>
          <div className="section-sub">{sub}</div>
        </div>
        <button className="btn btn-sm" onClick={onSeeAll}>See all</button>
      </div>

      <div className="table-wrap">
        <table className="meta-table">
          <tbody>
            {events.length === 0 && (
              <tr>
                <td style={{ textAlign: 'center', color: 'var(--text-3)', padding: 28 }}>
                  {emptyText}
                </td>
              </tr>
            )}
            {events.map((e) => (
              // The whole row is the target, not a link buried in the title.
              // These lists exist to be clicked through from.
              <tr
                key={e.id}
                onClick={() => onPick(e)}
                style={{ cursor: 'pointer' }}
                title={urgent ? 'Review this event' : 'Open in Events'}
              >
                <td>
                  <div style={{ fontWeight: 600, color: 'var(--text)' }}>{e.title}</div>
                  <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 2 }}>
                    {new Date(e.start_time).toLocaleDateString('en-ZA', {
                      day: 'numeric', month: 'short', year: 'numeric',
                    })}
                    {e.city ? ` · ${e.city}` : ''}
                  </div>
                </td>
                <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                  {urgent ? (
                    <span style={{ fontSize: 12, color: 'var(--warning-fg)', fontWeight: 600 }}>
                      {waitingFor(e.submitted_at)}
                    </span>
                  ) : (
                    <>
                      <div style={{ fontWeight: 700, fontVariantNumeric: 'tabular-nums' }}>
                        {fmtMoney(e.revenue)}
                      </div>
                      <div style={{ fontSize: 11.5, color: 'var(--text-3)' }}>
                        {e.tickets_sold} sold
                      </div>
                    </>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

/**
 * How long an event has been waiting.
 *
 * A relative age is the actionable form here — "3 days" tells you the queue is
 * slipping, where a submission timestamp makes you do the subtraction yourself.
 */
function waitingFor(since: string): string {
  const mins = Math.floor((Date.now() - new Date(since).getTime()) / 60000);
  if (mins < 60) return `${Math.max(1, mins)}m waiting`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h waiting`;
  return `${Math.floor(hours / 24)}d waiting`;
}