'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Dashboard summary rows
//
// Two rows, deliberately shallow:
//
//   Row 1  The event coming up next.
//   Row 2  The average across every event already held — the benchmark row 1
//          should be read against.
//
// The brief for the overview was "don't clutter it", so this is a strict
// budget: six metrics per row, one line each, no charts, no time controls, no
// drill-down. Anything an organiser might want to slice, compare or zoom lives
// on /analytics and is reached by the one link at the end of each row.
//
// Both rows share one component because they answer the same question at
// different scopes, and rendering them from one place is what guarantees the
// columns line up — the whole point of putting an average directly beneath a
// current figure is that the eye can compare them vertically without reading
// the labels twice.
// ─────────────────────────────────────────────────────────────────────────────
import { useEffect, useState } from 'react';
import Link from 'next/link';
import { Icon, IconName } from './icons';
import {
  getAnalyticsOverview, AnalyticsOverview,
  fmtCompact, fmtMoneyCompact, fmtDuration, fmtPct, DASH,
} from '../lib/analytics';

interface Chip {
  label: string;
  value: string;
  icon: IconName;
  hint?: string;
}

export function OverviewSummaryRows() {
  const [data, setData] = useState<AnalyticsOverview | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let alive = true;
    getAnalyticsOverview()
      .then((d) => { if (alive) setData(d); })
      // Silent on failure. These rows are a summary of information available in
      // full elsewhere, so a broken analytics endpoint should cost the
      // organiser two rows, not the whole Overview screen behind an error
      // toast.
      .catch(() => { if (alive) setFailed(true); });
    return () => { alive = false; };
  }, []);

  if (failed) return null;

  // Reserve the space while loading rather than letting the stat cards jump up
  // and then back down when the rows arrive.
  if (!data) {
    return (
      <div style={{ display: 'grid', gap: 12, marginBottom: 18 }}>
        <SkeletonRow />
        <SkeletonRow />
      </div>
    );
  }

  const up = data.upcoming;
  const avg = data.averages;

  // Nothing to summarise at all — a brand-new organiser with no live event and
  // no history. Showing two rows of em dashes would be worse than showing
  // nothing, so the rows simply don't appear until there's something in them.
  if (!up && !avg) return null;

  const upcomingChips: Chip[] = up ? [
    { label: 'Impressions', value: fmtCompact(up.metrics.impressions), icon: 'layers' },
    { label: 'Views', value: fmtCompact(up.metrics.views), icon: 'eye' },
    { label: 'Retention', value: fmtDuration(up.metrics.retention_seconds), icon: 'clock' },
    { label: 'Tickets sold', value: fmtCompact(up.metrics.tickets_sold), icon: 'ticket' },
    { label: 'PSR', value: fmtMoneyCompact(up.metrics.psr, up.currency), icon: 'dollar' },
    { label: 'Scanned', value: fmtCompact(up.metrics.scanned), icon: 'qr' },
  ] : [];

  const averageChips: Chip[] = avg ? [
    { label: 'Impressions', value: fmtCompact(avg.avg_impressions), icon: 'layers' },
    { label: 'Views', value: fmtCompact(avg.avg_views), icon: 'eye' },
    { label: 'Retention', value: fmtDuration(avg.retention_seconds), icon: 'clock' },
    { label: 'Tickets sold', value: fmtCompact(avg.avg_tickets_sold), icon: 'ticket' },
    { label: 'PSR', value: fmtMoneyCompact(avg.avg_psr, avg.currency), icon: 'dollar' },
    {
      label: 'No-shows',
      value: avg.avg_no_shows != null ? fmtCompact(avg.avg_no_shows) : DASH,
      icon: 'user',
    },
  ] : [];

  return (
    <div style={{ display: 'grid', gap: 12, marginBottom: 18 }}>
      {up && (
        <SummaryRow
          eyebrow="Coming up next"
          title={up.title}
          meta={daysToGoLabel(up.days_to_go)}
          accent="var(--accent)"
          chips={upcomingChips}
          href={`/analytics?event=${up.event_id}`}
        />
      )}

      {avg && (
        <SummaryRow
          eyebrow="Your average"
          title={`Across ${avg.events_counted} ${avg.events_counted === 1 ? 'event' : 'events'} held`}
          meta={
            avg.attendance_rate != null
              ? `${fmtPct(avg.attendance_rate, 0)} attendance`
              : undefined
          }
          accent="var(--text-3)"
          chips={averageChips}
          href="/analytics"
        />
      )}
    </div>
  );
}

// ── Row ──────────────────────────────────────────────────────────────────────

function SummaryRow({
  eyebrow, title, meta, chips, accent, href,
}: {
  eyebrow: string;
  title: string;
  meta?: string;
  chips: Chip[];
  accent: string;
  href: string;
}) {
  return (
    <div
      style={{
        background: 'var(--surface)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--r-lg)',
        boxShadow: `inset 3px 0 0 ${accent}`,
        padding: '13px 16px',
        display: 'flex',
        alignItems: 'center',
        gap: 18,
        flexWrap: 'wrap',
      }}
    >
      {/* Identity — fixed minimum so the chip columns start at the same x on
          both rows and stay vertically comparable. */}
      <div style={{ minWidth: 210, flex: '0 1 260px' }}>
        <div style={{
          fontSize: 9.5, fontWeight: 800, letterSpacing: '.12em',
          textTransform: 'uppercase', color: 'var(--text-3)',
        }}>
          {eyebrow}
        </div>
        <div style={{
          fontSize: 14.5, fontWeight: 800, color: 'var(--text)',
          letterSpacing: '-.01em', marginTop: 3,
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>
          {title}
        </div>
        {meta && (
          <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 2 }}>{meta}</div>
        )}
      </div>

      {/* Metrics */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 22,
        flex: 1, flexWrap: 'wrap', minWidth: 0,
      }}>
        {chips.map((c) => (
          <div key={c.label} style={{ minWidth: 78 }}>
            <div style={{
              display: 'flex', alignItems: 'center', gap: 5,
              fontSize: 9.5, fontWeight: 800, letterSpacing: '.08em',
              textTransform: 'uppercase', color: 'var(--text-3)',
            }}>
              <Icon name={c.icon} size={11} />
              {c.label}
            </div>
            <div style={{
              fontSize: 17, fontWeight: 800, color: 'var(--text)',
              letterSpacing: '-.02em', marginTop: 3,
              fontVariantNumeric: 'tabular-nums',
            }}>
              {c.value}
            </div>
          </div>
        ))}
      </div>

      <Link
        href={href}
        className="btn btn-sm"
        style={{ flexShrink: 0, textDecoration: 'none' }}
      >
        Details
        <Icon name="chevron-right" size={13} />
      </Link>
    </div>
  );
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function daysToGoLabel(days: number): string {
  if (days < 0) return 'Happening now';
  if (days < 1) {
    const hours = Math.max(1, Math.round(days * 24));
    return `In ${hours} ${hours === 1 ? 'hour' : 'hours'}`;
  }
  const d = Math.round(days);
  return `In ${d} ${d === 1 ? 'day' : 'days'}`;
}

function SkeletonRow() {
  return (
    <div style={{
      background: 'var(--surface)',
      border: '1px solid var(--border)',
      borderRadius: 'var(--r-lg)',
      height: 74,
    }} />
  );
}