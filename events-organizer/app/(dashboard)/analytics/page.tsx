'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Advanced analytics
//
// This is where the depth lives. The dashboard overview stays two summary rows;
// everything granular — per-minute/hour/day series, the funnel, per-event
// breakdown, averages in detail — is here, behind a deliberate click.
//
// The organiser controls three things and the page reacts to all three:
//   • Grain     minute | hour | day
//   • Window    presets sized so the point count never trips the server cap
//   • Scope     all events, or one event
//
// Every number rendered here comes from the server already computed. Nothing on
// this page divides one metric by another — if a rate is shown, the API sent
// that rate, so it can't disagree with the same rate elsewhere in the product.
// ─────────────────────────────────────────────────────────────────────────────
import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { Icon } from '@/components/icons';
import { LineChart, BarChart, Funnel, Legend, Series } from '@/components/charts';
import { StatusPill } from '@/components/ui';
import {
  getAnalyticsOverview, listEventAnalytics, getSeries,
  AnalyticsOverview, EventAnalyticsRow, SeriesResult, Granularity,
  RANGES, rangeToDates,
  fmtNum, fmtCompact, fmtMoney, fmtMoneyCompact, fmtPct, fmtDuration, fmtBucket,
  coverageNote, DASH,
} from '@/lib/analytics';

const GRAINS: Array<{ id: Granularity; label: string }> = [
  { id: 'minute', label: 'By minute' },
  { id: 'hour', label: 'By hour' },
  { id: 'day', label: 'By day' },
];

export default function AnalyticsPage() {
  const [grain, setGrain] = useState<Granularity>('day');
  const [rangeId, setRangeId] = useState<string>('30d');
  const [scope, setScope] = useState<string>('all'); // 'all' | event id

  const [overview, setOverview] = useState<AnalyticsOverview | null>(null);
  const [events, setEvents] = useState<EventAnalyticsRow[]>([]);
  const [series, setSeries] = useState<SeriesResult | null>(null);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Ranges are grain-specific — a minute grain has no "last 12 months" option.
  // Reconciling the two happens in the click handler below rather than in an
  // effect: an effect would render once with an invalid rangeId, fire a fetch
  // for it, then re-render and fire a second fetch for the corrected one.
  // Setting both together batches into a single render and a single request.
  const ranges = RANGES[grain];

  function chooseGrain(g: Granularity) {
    setGrain(g);
    const valid = RANGES[g];
    if (!valid.some((r) => r.id === rangeId)) setRangeId(valid[0].id);
  }

  // Identifies the query the controls currently describe. Loading is DERIVED
  // from whether the loaded data matches it, rather than tracked in its own
  // state — a separate isLoading flag has to be set synchronously at the top of
  // the effect, which is both a cascading render and a thing that can get out
  // of sync with reality if a fetch throws on a path that forgets to reset it.
  const queryKey = `${grain}|${rangeId}|${scope}`;
  const [loadedKey, setLoadedKey] = useState<string | null>(null);
  const seriesLoading = loadedKey !== queryKey;

  // ── Static data: overview + event list ──────────────────────────────────
  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const [ov, evs] = await Promise.all([
          getAnalyticsOverview(),
          listEventAnalytics(),
        ]);
        if (!alive) return;
        setOverview(ov);
        setEvents(evs);
      } catch {
        if (alive) setError('Could not load analytics.');
      } finally {
        if (alive) setLoading(false);
      }
    })();
    return () => { alive = false; };
  }, []);

  // ── Series: refetches on every control change ───────────────────────────
  // `reloadNonce` exists only so the Refresh button can re-run this effect
  // without any control having changed.
  const [reloadNonce, setReloadNonce] = useState(0);

  useEffect(() => {
    let alive = true;
    const range = RANGES[grain].find((r) => r.id === rangeId) ?? RANGES[grain][0];
    const { from, to } = rangeToDates(range.hours);

    getSeries({
      granularity: grain,
      eventIds: scope === 'all' ? undefined : [scope],
      from, to,
    })
      .then((res) => {
        if (!alive) return;
        setSeries(res);
        setLoadedKey(queryKey);
      })
      .catch(() => {
        if (!alive) return;
        setSeries(null);
        // Mark the key loaded even on failure, otherwise seriesLoading stays
        // true forever and the charts sit dimmed with no explanation.
        setLoadedKey(queryKey);
      });

    // Guards against a slow request for an old grain landing after a fast one
    // for the new grain and overwriting it.
    return () => { alive = false; };
  }, [grain, rangeId, scope, queryKey, reloadNonce]);

  const selectedEvent = useMemo(
    () => events.find((e) => e.event_id === scope) ?? null,
    [events, scope],
  );

  // Scope drives which metric set the summary tiles and funnel describe:
  // one event ⇒ that event's lifetime totals; all events ⇒ the window totals.
  const focus = selectedEvent?.metrics ?? series?.totals ?? null;

  const labels = useMemo(
    () => (series?.points ?? []).map((p) => fmtBucket(p.bucket, series!.granularity)),
    [series],
  );

  if (loading) {
    return (
      <div className="page">
        <div className="page-loader"><div className="spinner spinner-lg" /></div>
      </div>
    );
  }

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <div className="page-title">Advanced analytics</div>
          <div className="section-sub">
            Reach, engagement, revenue and door data at minute, hour or day resolution
          </div>
        </div>
        <button className="btn btn-sm" onClick={() => setReloadNonce((n) => n + 1)} disabled={seriesLoading}>
          <Icon name="refresh" size={14} />
          Refresh
        </button>
      </div>

      <div className="page-content">
        <div className="container" style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>

          {error && <div className="alert alert-danger">{error}</div>}

          {/* ── Controls ──────────────────────────────────────────────── */}
          <div className="card">
            <div className="toolbar" style={{ borderBottom: 'none' }}>
              <div className="toolbar-left" style={{ flexWrap: 'wrap' }}>
                <ControlGroup label="Resolution">
                  {GRAINS.map((g) => (
                    <button
                      key={g.id}
                      className={`pill${grain === g.id ? ' active' : ''}`}
                      onClick={() => chooseGrain(g.id)}
                    >
                      {g.label}
                    </button>
                  ))}
                </ControlGroup>

                <ControlGroup label="Window">
                  {ranges.map((r) => (
                    <button
                      key={r.id}
                      className={`pill${rangeId === r.id ? ' active' : ''}`}
                      onClick={() => setRangeId(r.id)}
                    >
                      {r.label}
                    </button>
                  ))}
                </ControlGroup>
              </div>

              <div className="toolbar-right">
                <select
                  className="filter-select"
                  value={scope}
                  onChange={(e) => setScope(e.target.value)}
                  style={{ minWidth: 220 }}
                  aria-label="Event scope"
                >
                  <option value="all">All events</option>
                  {events.map((e) => (
                    <option key={e.event_id} value={e.event_id}>{e.title}</option>
                  ))}
                </select>
              </div>
            </div>

            {series?.clamped && series.clamp_note && (
              <div style={{ padding: '0 20px 14px' }}>
                <div className="alert alert-info" style={{ marginBottom: 0 }}>
                  <Icon name="info" size={16} />
                  <span>{series.clamp_note}</span>
                </div>
              </div>
            )}
          </div>

          {/* ── Headline tiles ────────────────────────────────────────── */}
          <div>
            <div className="section-header">
              <div>
                <div className="section-title">
                  {selectedEvent ? selectedEvent.title : 'All events'}
                </div>
                <div className="section-sub">
                  {selectedEvent
                    ? 'Lifetime totals for this event'
                    : 'Totals across the selected window'}
                </div>
              </div>
              {selectedEvent && <StatusPill status={selectedEvent.status} />}
            </div>

            <div className="stat-grid" style={{ marginBottom: 14 }}>
              <MetricTile
                label="Impressions" icon="layers" accent="var(--accent)"
                value={fmtCompact(focus?.impressions)}
                sub="Times your card appeared in a feed"
              />
              <MetricTile
                label="Views" icon="eye" accent="var(--info)"
                value={fmtCompact(focus?.views)}
                sub={focus?.ctr !== undefined ? `${fmtPct(focus?.ctr ?? null)} of impressions` : undefined}
              />
              <MetricTile
                label="Retention" icon="clock" accent="var(--warning)"
                value={fmtDuration(focus?.retention_seconds ?? null)}
                sub={focus ? (coverageNote(focus) ?? 'Average time on your event page') : undefined}
              />
              <MetricTile
                label="PSR" icon="dollar" accent="var(--success)"
                value={fmtMoneyCompact(focus?.psr ?? null, selectedEvent?.currency)}
                sub="Revenue after the platform cut"
              />
            </div>

            <div className="stat-grid">
              <MetricTile
                label="Tickets sold" icon="ticket" accent="var(--accent)"
                value={fmtNum(focus?.tickets_sold)}
                sub={`${fmtNum(focus?.bookings)} bookings`}
              />
              <MetricTile
                label="Conversion" icon="trending-up" accent="var(--info)"
                value={fmtPct(focus?.conversion ?? null)}
                sub="Tickets sold per view"
              />
              <MetricTile
                label="Scanned in" icon="qr" accent="var(--success)"
                value={fmtNum(focus?.scanned)}
                sub={
                  focus?.attendance_rate != null
                    ? `${fmtPct(focus.attendance_rate)} of tickets sold`
                    : 'No scans recorded yet'
                }
              />
              <MetricTile
                label="No-shows" icon="user" accent="var(--danger)"
                value={focus?.no_shows != null ? fmtNum(focus.no_shows) : DASH}
                sub={
                  focus?.no_shows != null
                    ? 'Tickets never scanned at the door'
                    : 'Calculated once the event has ended'
                }
              />
            </div>
          </div>

          {/* ── Charts ────────────────────────────────────────────────── */}
          <ChartCard
            title="Reach and interest"
            sub="Impressions versus views over time"
            loading={seriesLoading}
            series={[
              { key: 'impressions', label: 'Impressions', color: 'var(--accent)', values: (series?.points ?? []).map((p) => p.impressions) },
              { key: 'views', label: 'Views', color: 'var(--info)', values: (series?.points ?? []).map((p) => p.views) },
            ]}
            labels={labels}
            kind="line"
            format={(n) => fmtCompact(n)}
          />

          <ChartCard
            title="Revenue"
            sub="Gross versus post-split revenue"
            loading={seriesLoading}
            series={[
              { key: 'gross', label: 'Gross', color: 'var(--text-3)', values: (series?.points ?? []).map((p) => p.gross_revenue) },
              { key: 'psr', label: 'PSR', color: 'var(--success)', values: (series?.points ?? []).map((p) => p.psr) },
            ]}
            labels={labels}
            kind="bar"
            format={(n) => fmtMoneyCompact(n)}
          />

          <ChartCard
            title="Tickets and door"
            sub="Sold versus scanned in"
            loading={seriesLoading}
            series={[
              { key: 'sold', label: 'Tickets sold', color: 'var(--accent)', values: (series?.points ?? []).map((p) => p.tickets_sold) },
              { key: 'scanned', label: 'Scanned', color: 'var(--warning)', values: (series?.points ?? []).map((p) => p.scanned) },
            ]}
            labels={labels}
            kind="bar"
            format={(n) => fmtNum(n)}
          />

          {/* ── Funnel ────────────────────────────────────────────────── */}
          {focus && (
            <div className="card">
              <div className="card-header">
                <div>
                  <div className="card-title">Conversion funnel</div>
                  <div className="section-sub">
                    Where people drop off between seeing your event and walking through the door
                  </div>
                </div>
              </div>
              <div className="card-body">
                <Funnel
                  steps={[
                    { label: 'Impressions', value: focus.impressions, color: 'var(--accent)', hint: 'Appeared in someone\u2019s feed' },
                    { label: 'Views', value: focus.views, color: 'var(--info)', hint: 'Opened your event page' },
                    { label: 'Tickets sold', value: focus.tickets_sold, color: 'var(--success)', hint: 'Completed a booking' },
                    { label: 'Scanned in', value: focus.scanned, color: 'var(--warning)', hint: 'Arrived and were admitted' },
                  ]}
                />
              </div>
            </div>
          )}

          {/* ── Averages in detail ────────────────────────────────────── */}
          {overview?.averages && (
            <div className="card">
              <div className="card-header">
                <div>
                  <div className="card-title">Your averages, in detail</div>
                  <div className="section-sub">
                    Across {overview.averages.events_counted}{' '}
                    {overview.averages.events_counted === 1 ? 'event' : 'events'} you have held.
                    Rates are pooled across all of them, not averaged per event.
                  </div>
                </div>
              </div>
              <div className="table-wrap">
                <table className="meta-table">
                  <thead>
                    <tr>
                      <th>Metric</th>
                      <th style={{ textAlign: 'right' }}>Per event</th>
                      <th style={{ textAlign: 'right' }}>Rate</th>
                    </tr>
                  </thead>
                  <tbody>
                    <AvgRow label="Impressions" per={fmtNum(overview.averages.avg_impressions)} rate={DASH} />
                    <AvgRow label="Views" per={fmtNum(overview.averages.avg_views)} rate={`${fmtPct(overview.averages.ctr)} CTR`} />
                    <AvgRow label="Retention" per={DASH} rate={fmtDuration(overview.averages.retention_seconds)} />
                    <AvgRow label="Tickets sold" per={fmtNum(overview.averages.avg_tickets_sold)} rate={`${fmtPct(overview.averages.conversion)} conversion`} />
                    <AvgRow label="Gross revenue" per={fmtMoney(overview.averages.avg_gross_revenue, overview.averages.currency)} rate={DASH} />
                    <AvgRow label="PSR" per={fmtMoney(overview.averages.avg_psr, overview.averages.currency)} rate={DASH} />
                    <AvgRow label="Scanned in" per={fmtNum(overview.averages.avg_scanned)} rate={`${fmtPct(overview.averages.attendance_rate)} attendance`} />
                    <AvgRow label="No-shows" per={fmtNum(overview.averages.avg_no_shows)} rate={DASH} />
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* ── Per-event breakdown ───────────────────────────────────── */}
          <div className="card">
            <div className="card-header">
              <div className="card-title">Every event</div>
              <span style={{ fontSize: 12, color: 'var(--text-3)' }}>
                {events.length} total
              </span>
            </div>
            <div className="table-wrap">
              <table className="meta-table">
                <thead>
                  <tr>
                    <th>Event</th>
                    <th style={{ textAlign: 'right' }}>Impressions</th>
                    <th style={{ textAlign: 'right' }}>Views</th>
                    <th style={{ textAlign: 'right' }}>CTR</th>
                    <th style={{ textAlign: 'right' }}>Retention</th>
                    <th style={{ textAlign: 'right' }}>Sold</th>
                    <th style={{ textAlign: 'right' }}>PSR</th>
                    <th style={{ textAlign: 'right' }}>Scanned</th>
                    <th style={{ textAlign: 'right' }}>No-shows</th>
                  </tr>
                </thead>
                <tbody>
                  {events.length === 0 && (
                    <tr>
                      <td colSpan={9} style={{ textAlign: 'center', color: 'var(--text-3)', padding: 28 }}>
                        No events yet.
                      </td>
                    </tr>
                  )}
                  {events.map((e) => (
                    <tr key={e.event_id}>
                      <td>
                        <Link
                          href={`/event/${e.event_id}`}
                          style={{ fontWeight: 600, color: 'var(--text)' }}
                        >
                          {e.title}
                        </Link>
                        <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 2 }}>
                          {new Date(e.start_time).toLocaleDateString('en-ZA', {
                            day: 'numeric', month: 'short', year: 'numeric',
                          })}
                        </div>
                      </td>
                      <Num>{fmtNum(e.metrics.impressions)}</Num>
                      <Num>{fmtNum(e.metrics.views)}</Num>
                      <Num>{fmtPct(e.metrics.ctr)}</Num>
                      <Num>{fmtDuration(e.metrics.retention_seconds)}</Num>
                      <Num>{fmtNum(e.metrics.tickets_sold)}</Num>
                      <Num>{fmtMoney(e.metrics.psr, e.currency)}</Num>
                      <Num>{fmtNum(e.metrics.scanned)}</Num>
                      <Num>{e.metrics.no_shows != null ? fmtNum(e.metrics.no_shows) : DASH}</Num>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <MetricGlossary />
        </div>
      </div>
    </div>
  );
}

// ── Small pieces ─────────────────────────────────────────────────────────────

function ControlGroup({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <span style={{
        fontSize: 10, fontWeight: 800, letterSpacing: '.1em',
        textTransform: 'uppercase', color: 'var(--text-3)',
      }}>
        {label}
      </span>
      <div style={{ display: 'flex', gap: 6 }}>{children}</div>
    </div>
  );
}

function MetricTile({
  label, value, sub, icon, accent,
}: {
  label: string; value: string; sub?: string; icon: React.ComponentProps<typeof Icon>['name']; accent: string;
}) {
  return (
    <div className="stat-card" style={{ boxShadow: `inset 3px 0 0 ${accent}` }}>
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        gap: 8, marginBottom: 11,
      }}>
        <div className="stat-label" style={{ marginBottom: 0 }}>{label}</div>
        <div style={{
          width: 30, height: 30, borderRadius: 10, color: accent,
          background: 'var(--surface-strong)', display: 'grid', placeItems: 'center',
        }}>
          <Icon name={icon} size={15} />
        </div>
      </div>
      <div className="stat-value">{value}</div>
      {sub && <div className="stat-sub">{sub}</div>}
    </div>
  );
}

function ChartCard({
  title, sub, series, labels, kind, format, loading,
}: {
  title: string;
  sub: string;
  series: Series[];
  labels: string[];
  kind: 'line' | 'bar';
  format: (n: number) => string;
  loading: boolean;
}) {
  return (
    <div className="card">
      <div className="card-header">
        <div>
          <div className="card-title">{title}</div>
          <div className="section-sub">{sub}</div>
        </div>
        <Legend series={series} />
      </div>
      <div className="card-body" style={{ opacity: loading ? 0.45 : 1, transition: 'opacity .15s' }}>
        {kind === 'line'
          ? <LineChart series={series} labels={labels} formatValue={format} />
          : <BarChart series={series} labels={labels} formatValue={format} />}
      </div>
    </div>
  );
}

function AvgRow({ label, per, rate }: { label: string; per: string; rate: string }) {
  return (
    <tr>
      <td style={{ fontWeight: 600 }}>{label}</td>
      <td style={{ textAlign: 'right', fontWeight: 700 }}>{per}</td>
      <td style={{ textAlign: 'right', color: 'var(--text-2)' }}>{rate}</td>
    </tr>
  );
}

function Num({ children }: { children: React.ReactNode }) {
  return <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{children}</td>;
}

/**
 * Definitions, on the page rather than in a support doc.
 *
 * Reach and retention in particular are easy to misread — "impressions" means
 * something different on every platform an organiser has used, and an
 * unexplained number invites the wrong conclusion.
 */
function MetricGlossary() {
  const items: Array<[string, string]> = [
    ['Impressions', 'How many people your event reached — times your card was returned in a feed, search or category listing. Counted once per person per feed load, even if you appear in several sections of it.'],
    ['Views', 'How many people clicked through and opened your event page.'],
    ['CTR', 'Views divided by impressions. How compelling your card is in the feed.'],
    ['Retention', 'Total time spent on your event page divided by views. Reported by the app when someone closes the page, so it covers a sample of views rather than all of them — the coverage figure tells you how large that sample is.'],
    ['Conversion', 'Tickets sold divided by views. How persuasive your event page is once someone is on it.'],
    ['PSR', 'Post-split revenue: what you keep after the TicketHawk platform fee.'],
    ['Scanned in', 'How many people entered your event — one count per ticket admitted at the door.'],
    ['No-shows', 'Tickets that were sold but never scanned. Calculated after the event ends, since before then an unscanned ticket is just someone who has not arrived yet.'],
    ['Attendance', 'Scanned in divided by tickets sold.'],
  ];
  return (
    <div className="card">
      <div className="card-header">
        <div className="card-title">What these mean</div>
      </div>
      <div className="card-body" style={{ display: 'grid', gap: 14 }}>
        {items.map(([term, def]) => (
          <div key={term}>
            <div style={{ fontSize: 12.5, fontWeight: 800, color: 'var(--text)' }}>{term}</div>
            <div style={{ fontSize: 12.5, color: 'var(--text-2)', marginTop: 3, lineHeight: 1.6 }}>{def}</div>
          </div>
        ))}
      </div>
    </div>
  );
}