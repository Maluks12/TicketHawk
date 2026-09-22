'use client';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../../../lib/api';
import { fmtMoney } from '../../../lib/utils';
import { useStaff, AccessDenied } from '../../../lib/permissions';
import { LineChart, BarChart, Legend, Series } from '../../../components/charts';

// ─────────────────────────────────────────────────────────────────────────────
// Advanced analytics
//
// Everything the dashboard shows, plus the shape of it over time. The dashboard
// answers "where are we"; this answers "which way are we moving", and those are
// different enough that cramming both onto one screen is what made the
// dashboard unreadable in the first place.
//
// Money charts keep GMV and revenue on SEPARATE charts rather than plotting
// both on one axis. At a ~7% take rate the revenue line would be a flat smear
// along the bottom of a GMV chart — technically present, practically invisible,
// and the number that actually matters is the invisible one.
// ─────────────────────────────────────────────────────────────────────────────

interface Point {
  bucket: string;
  ticket_gmv: number; merch_gmv: number; our_revenue: number;
  bookings: number; tickets_sold: number; tickets_free: number; tickets_paid: number;
  signups: number; events_created: number;
}

interface GMV {
  ticket_gmv: number; merch_gmv: number; total_gmv: number;
  booking_fees: number; commission: number; our_revenue: number;
  take_rate: number | null; merch_revenue_known: boolean; organiser_psr: number;
  tickets_sold: number; tickets_free: number; tickets_paid: number;
  events_public_free: number; events_public_paid: number;
  events_private_free: number; events_private_paid: number;
  total_users: number; booked_90d: number; organisers: number;
}

interface Presence {
  available: boolean;
  active_now: number;
  active_1h: number;
  active_24h: number;
  as_of: string;
}

type Gran = 'day' | 'hour';

const RANGES: Record<Gran, Array<{ id: string; label: string; hours: number }>> = {
  hour: [
    { id: '24h', label: 'Last 24 hours', hours: 24 },
    { id: '7d', label: 'Last 7 days', hours: 24 * 7 },
    { id: '14d', label: 'Last 14 days', hours: 24 * 14 },
  ],
  day: [
    { id: '30d', label: 'Last 30 days', hours: 24 * 30 },
    { id: '90d', label: 'Last 90 days', hours: 24 * 90 },
    { id: '365d', label: 'Last 12 months', hours: 24 * 365 },
  ],
};

export default function AdminAnalyticsPage() {
  const capsLoaded = useStaff((s) => s.loaded);
  const can = useStaff((s) => s.can);
  const loadCaps = useStaff((s) => s.load);

  const [gran, setGran] = useState<Gran>('day');
  const [rangeId, setRangeId] = useState('30d');
  const [points, setPoints] = useState<Point[]>([]);
  const [gmv, setGmv] = useState<GMV | null>(null);
  const [clampNote, setClampNote] = useState<string | null>(null);
  const [presence, setPresence] = useState<Presence | null>(null);

  // Only ever set FALSE — see the dashboard for why a synchronous setState at
  // the top of a fetch called from an effect is a cascading render.
  const [loading, setLoading] = useState(true);

  useEffect(() => { loadCaps(); }, [loadCaps]);

  // Presence polls on its own 60-second interval, independent of the charts.
  // Separate because it is a cheap Redis read while the series is four
  // aggregate queries — tying them together would mean either a stale number
  // or re-running the heavy work every minute.
  useEffect(() => {
    let alive = true;
    const tick = () => {
      api.get('/admin/stats/presence')
        .then(({ data }) => { if (alive) setPresence(data); })
        .catch(() => { if (alive) setPresence(null); });
    };
    tick();
    const id = setInterval(tick, 60_000);
    return () => { alive = false; clearInterval(id); };
  }, []);

  // Ranges are grain-specific; reconciling happens in the click handler rather
  // than an effect, so changing grain is one render and one request instead of
  // two of each.
  function chooseGran(g: Gran) {
    setGran(g);
    if (!RANGES[g].some((r) => r.id === rangeId)) setRangeId(RANGES[g][0].id);
  }

  const queryKey = `${gran}|${rangeId}`;
  const [loadedKey, setLoadedKey] = useState<string | null>(null);
  const busy = loadedKey !== queryKey;

  // The fetch lives IN the effect and every setState sits in a promise
  // continuation. Calling an async, state-setting helper from an effect is what
  // the cascading-render rule points at; putting the awaits inline keeps all the
  // writes off the synchronous path.
  //
  // `alive` guards against a slow request for an old grain landing after a fast
  // one for the new grain and overwriting it.
  useEffect(() => {
    let alive = true;

    const range = RANGES[gran].find((r) => r.id === rangeId) ?? RANGES[gran][0];
    const to = new Date();
    const from = new Date(to.getTime() - range.hours * 3600 * 1000);
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone ?? 'Africa/Johannesburg';

    Promise.allSettled([
      api.get('/admin/analytics/series', {
        params: { granularity: gran, from: from.toISOString(), to: to.toISOString(), tz },
      }),
      api.get('/admin/stats/gmv'),
    ]).then(([s, g]) => {
      if (!alive) return;
      if (s.status === 'fulfilled') {
        setPoints(s.value.data?.points ?? []);
        setClampNote(s.value.data?.clamped ? s.value.data?.clamp_note ?? null : null);
      } else {
        setPoints([]);
      }
      if (g.status === 'fulfilled') setGmv(g.value.data);
      setLoadedKey(queryKey);
      setLoading(false);
    });

    return () => { alive = false; };
  }, [gran, rangeId, queryKey]);

  const labels = useMemo(
    () => points.map((p) => {
      const d = new Date(p.bucket);
      return gran === 'day'
        ? d.toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' })
        : d.toLocaleString('en-ZA', { day: 'numeric', month: 'short', hour: '2-digit' });
    }),
    [points, gran],
  );

  if (!capsLoaded) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }
  // Aggregates only — no personal data — so the weakest staff capability is the
  // right gate. Anything narrower would keep read_only staff from seeing
  // numbers that contain nobody's information.
  if (!can('user.list')) return <AccessDenied need="user.list" />;

  if (loading) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }

  const v = (f: (p: Point) => number) => points.map(f);

  return (
    <div style={{ padding: '24px 28px 48px', maxWidth: 1600, margin: '0 auto' }}>
      <div style={{ marginBottom: 18 }}>
        <div className="page-title">Advanced analytics</div>
        <div className="page-sub">Volume, revenue and growth over time</div>
      </div>

      {/* ── Controls ────────────────────────────────────────────────────── */}
      <div className="card" style={{ marginBottom: 18 }}>
        <div className="table-toolbar" style={{ padding: '12px 18px', marginBottom: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 14, flexWrap: 'wrap' }}>
            <Group label="Resolution">
              {(['day', 'hour'] as Gran[]).map((g) => (
                <button key={g} className={`pill${gran === g ? ' active' : ''}`}
                  onClick={() => chooseGran(g)}>
                  {g === 'day' ? 'By day' : 'By hour'}
                </button>
              ))}
            </Group>
            <Group label="Window">
              {RANGES[gran].map((r) => (
                <button key={r.id} className={`pill${rangeId === r.id ? ' active' : ''}`}
                  onClick={() => setRangeId(r.id)}>
                  {r.label}
                </button>
              ))}
            </Group>
          </div>
        </div>
        {clampNote && (
          <div style={{ padding: '0 18px 14px' }}>
            <div className="alert alert-info" style={{ marginBottom: 0 }}>{clampNote}</div>
          </div>
        )}
      </div>

      {/* ── Headline ─────────────────────────────────────────────────────── */}
      <div className="stats-grid">
        <Tile label="Total GMV" value={fmtMoney(gmv?.total_gmv ?? 0)} accent="var(--accent)"
          sub="Value transacted" />
        <Tile label="Our revenue" value={fmtMoney(gmv?.our_revenue ?? 0)} accent="var(--success)"
          sub={gmv?.take_rate != null ? `${(gmv.take_rate * 100).toFixed(1)}% take rate` : '—'} />
        {/* Live, not a cohort. The old tile showed 90-day bookers under the
            label "Active users", which is why the number looked wrong — it was
            answering a different question. */}
        <Tile
          label="Active now"
          value={presence?.available ? presence.active_now.toLocaleString('en-ZA') : '—'}
          accent="var(--info)"
          sub={presence?.available
            ? `in the last 5 min · ${presence.active_24h.toLocaleString('en-ZA')} today`
            : 'Presence tracking unavailable'}
          live={presence?.available}
        />
        <Tile label="Organisers" value={String(gmv?.organisers ?? 0)} accent="var(--warning)"
          sub="With a profile" />
      </div>

      {/* ── Charts ───────────────────────────────────────────────────────── */}
      <Chart
        title="GMV over time" sub="Ticket and merch value transacted"
        kind="bar" labels={labels} busy={busy} money
        series={[
          { key: 'tgmv', label: 'Ticket GMV', color: 'var(--accent)', values: v((p) => p.ticket_gmv) },
          { key: 'mgmv', label: 'Merch GMV', color: 'var(--warning)', values: v((p) => p.merch_gmv) },
        ]}
      />

      {/* Own chart, own axis. Plotted against GMV this line would be a flat
          smear at the bottom — and it is the number that matters most. */}
      <Chart
        title="Our revenue over time" sub="Booking fees + commission"
        kind="line" labels={labels} busy={busy} money
        series={[
          { key: 'rev', label: 'Our revenue', color: 'var(--success)', values: v((p) => p.our_revenue) },
        ]}
      />

      <Chart
        title="Tickets sold" sub="Free versus paid"
        kind="bar" labels={labels} busy={busy}
        series={[
          { key: 'paid', label: 'Paid', color: 'var(--accent)', values: v((p) => p.tickets_paid) },
          { key: 'free', label: 'Free', color: 'var(--text-3)', values: v((p) => p.tickets_free) },
        ]}
      />

      <Chart
        title="Growth" sub="New signups and events created"
        kind="line" labels={labels} busy={busy}
        series={[
          { key: 'su', label: 'Signups', color: 'var(--info)', values: v((p) => p.signups) },
          { key: 'ev', label: 'Events created', color: 'var(--accent)', values: v((p) => p.events_created) },
        ]}
      />

      {/* ── Breakdowns ───────────────────────────────────────────────────── */}
      <div style={{
        display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(340px, 1fr))',
        gap: 18, marginTop: 18,
      }}>
        <Breakdown
          title="Events by type"
          sub="Visibility crossed with pricing"
          rows={[
            ['Public · paid', gmv?.events_public_paid ?? 0],
            ['Public · free', gmv?.events_public_free ?? 0],
            ['Private · paid', gmv?.events_private_paid ?? 0],
            ['Private · free', gmv?.events_private_free ?? 0],
          ]}
        />
        <Breakdown
          title="Tickets"
          sub="Counted per booking by unit price"
          rows={[
            ['Paid', gmv?.tickets_paid ?? 0],
            ['Free', gmv?.tickets_free ?? 0],
          ]}
        />
        <Breakdown
          title="People"
          sub="Registered, and how many still book"
          rows={[
            ['Registered', gmv?.total_users ?? 0],
            ['Booked in 90 days', gmv?.booked_90d ?? 0],
            ['Organisers', gmv?.organisers ?? 0],
          ]}
        />
        <Breakdown
          title="Revenue split"
          sub="Where our cut comes from"
          money
          rows={[
            ['Booking fees', gmv?.booking_fees ?? 0],
            ['Commission', gmv?.commission ?? 0],
          ]}
        />
      </div>

      <Definitions />
    </div>
  );
}

// ── Pieces ───────────────────────────────────────────────────────────────────

function Group({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <span style={{
        fontSize: 10, fontWeight: 800, letterSpacing: '.1em',
        textTransform: 'uppercase', color: 'var(--text-3)',
      }}>{label}</span>
      <div style={{ display: 'flex', gap: 6 }}>{children}</div>
    </div>
  );
}

function Tile({ label, value, sub, accent, live = false }: {
  label: string; value: string; sub?: string; accent: string; live?: boolean;
}) {
  return (
    <div className="stat-card" style={{ boxShadow: `inset 3px 0 0 ${accent}` }}>
      <div className="stat-label" style={{
        display: 'flex', alignItems: 'center', gap: 6,
      }}>
        {label}
        {/* A live number that looks identical to a static one invites the
            reader to assume it is stale. The dot says it is moving. */}
        {live && (
          <span
            title="Updates every minute"
            style={{
              width: 6, height: 6, borderRadius: '50%',
              background: 'var(--success)', flexShrink: 0,
              animation: 'presencePulse 2s ease-in-out infinite',
            }}
          />
        )}
      </div>
      <div className="stat-value">{value}</div>
      {sub && <div className="stat-sub">{sub}</div>}
      {live && (
        <style>{`@keyframes presencePulse{0%,100%{opacity:1}50%{opacity:.25}}`}</style>
      )}
    </div>
  );
}

function Chart({
  title, sub, series, labels, kind, busy, money = false,
}: {
  title: string; sub: string; series: Series[]; labels: string[];
  kind: 'line' | 'bar'; busy: boolean; money?: boolean;
}) {
  const fmt = (n: number) => (money
    ? (Math.abs(n) >= 10000 ? `R${(n / 1000).toFixed(0)}k` : `R${Math.round(n)}`)
    : String(Math.round(n)));

  return (
    <div className="card" style={{ marginTop: 18 }}>
      <div className="card-header">
        <div>
          <div className="card-title">{title}</div>
          <div className="section-sub">{sub}</div>
        </div>
        <Legend series={series} />
      </div>
      <div className="card-body" style={{ opacity: busy ? 0.45 : 1, transition: 'opacity .15s' }}>
        {kind === 'line'
          ? <LineChart series={series} labels={labels} formatValue={fmt} />
          : <BarChart series={series} labels={labels} formatValue={fmt} />}
      </div>
    </div>
  );
}

/**
 * A labelled set of counts with proportional bars.
 *
 * Bars are relative to the largest row, not to the total — the question these
 * answer is "which of these is biggest", and normalising to a total makes small
 * categories vanish when one dominates.
 */
function Breakdown({ title, sub, rows, money = false }: {
  title: string; sub: string; rows: [string, number][]; money?: boolean;
}) {
  const max = Math.max(1, ...rows.map(([, n]) => n));
  return (
    <div className="card">
      <div className="card-header">
        <div>
          <div className="card-title">{title}</div>
          <div className="section-sub">{sub}</div>
        </div>
      </div>
      <div className="card-body" style={{ display: 'grid', gap: 14 }}>
        {rows.map(([label, n]) => (
          <div key={label}>
            <div style={{
              display: 'flex', justifyContent: 'space-between', gap: 10,
              fontSize: 13, marginBottom: 5,
            }}>
              <span style={{ color: 'var(--text-2)', fontWeight: 600 }}>{label}</span>
              <span style={{ fontWeight: 800, fontVariantNumeric: 'tabular-nums' }}>
                {money ? fmtMoney(n) : n.toLocaleString('en-ZA')}
              </span>
            </div>
            <div style={{ height: 8, borderRadius: 999, background: 'var(--surface-strong)' }}>
              <div style={{
                width: `${Math.max(2, (n / max) * 100)}%`, height: '100%',
                borderRadius: 999, background: 'var(--accent)',
                transition: 'width .3s ease',
              }} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

/**
 * Definitions on the page, not in a runbook.
 *
 * GMV-vs-revenue in particular has to be stated here: the previous dashboard
 * labelled ticket face value as "revenue", and anyone who saw that number needs
 * to know why this page shows a much smaller one.
 */
function Definitions() {
  const items: [string, string][] = [
    ['GMV', 'Gross merchandise value — the total value transacted. Almost all of it belongs to organisers, not to us.'],
    ['Our revenue', 'What TicketHawk keeps: the buyer-paid booking fee plus the organiser commission (5% standard, lower for organisers on custom terms). This is the business\u2019s actual income.'],
    ['Take rate', 'Our revenue divided by ticket GMV. The single number that says whether the model works.'],
    ['Organiser PSR', 'Post-split revenue — what organisers keep after our commission.'],
    ['Free vs paid tickets', 'Decided per booking by its unit price, not by the event\u2019s headline price. An event with a free tier and a VIP tier counts in both.'],
    ['Active now', 'Distinct users who made an authenticated request in the last 5 minutes. Updates every minute. Measured in Redis on the tail of each request, so it reflects real use rather than sign-ins.'],
    ['Booked (90d)', 'Distinct users who confirmed a booking in the last 90 days — a retention cohort, not a liveness measure. Both are useful; they answer different questions.'],
    ['Merch revenue', 'Not shown: merch_orders stores no commission split, so there is nothing in the database that says what we earn on a merch sale.'],
  ];
  return (
    <div className="card" style={{ marginTop: 18 }}>
      <div className="card-header"><div className="card-title">What these mean</div></div>
      <div className="card-body" style={{ display: 'grid', gap: 14 }}>
        {items.map(([t, d]) => (
          <div key={t}>
            <div style={{ fontSize: 12.5, fontWeight: 800, color: 'var(--text)' }}>{t}</div>
            <div style={{ fontSize: 12.5, color: 'var(--text-2)', marginTop: 3, lineHeight: 1.6 }}>{d}</div>
          </div>
        ))}
      </div>
    </div>
  );
}