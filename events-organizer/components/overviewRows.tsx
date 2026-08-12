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
  fmtCompact, fmtMoneyCompact, fmtDuration,
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
      <div style={{ marginBottom: 18 }}>
        <SkeletonRow />
      </div>
    );
  }

  const up = data.upcoming;

  if (!up) return null;

  const upcomingChips: Chip[] = up ? [
    { label: 'Impressions', value: fmtCompact(up.metrics.impressions), icon: 'layers' },
    { label: 'Views', value: fmtCompact(up.metrics.views), icon: 'eye' },
    { label: 'Retention', value: fmtDuration(up.metrics.retention_seconds), icon: 'clock' },
    { label: 'Tickets sold', value: fmtCompact(up.metrics.tickets_sold), icon: 'ticket' },
    { label: 'PSR', value: fmtMoneyCompact(up.metrics.psr, up.currency), icon: 'dollar' },
    { label: 'Scanned', value: fmtCompact(up.metrics.scanned), icon: 'qr' },
  ] : [];

  return (
    <div style={{ display: 'grid', gap: 12, marginBottom: 18 }}>
      <SummaryRow
        eyebrow="Coming up next"
        title={up.title}
        meta={daysToGoLabel(up.days_to_go)}
        accent="var(--accent)"
        chips={upcomingChips}
        href={`/analytics?event=${up.event_id}`}
      />
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
    <div style={{
      background: 'var(--surface)',
      border: '1px solid var(--border)',
      borderRadius: 'var(--r-xl)',
      display: 'flex',
      alignItems: 'stretch',
      flexWrap: 'wrap',
      overflow: 'hidden',
    }}>
      <div style={{
        padding: '16px 20px',
        borderRight: '1px solid var(--border)',
        minWidth: 230, flex: '0 1 280px',
        display: 'flex', flexDirection: 'column', justifyContent: 'center',
        boxShadow: `inset 3px 0 0 ${accent}`,
      }}>
        <div style={{
          fontSize: 10, fontWeight: 800, letterSpacing: '.12em',
          textTransform: 'uppercase', color: 'var(--text-3)',
        }}>
          {eyebrow}
        </div>
        <div style={{
          fontSize: 16, fontWeight: 700, color: 'var(--text)',
          letterSpacing: '-.02em', marginTop: 4,
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>
          {title}
        </div>
        {meta && (
          <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 3 }}>{meta}</div>
        )}
      </div>

      {/* Same hairline-divided treatment as the analytics metric row, so the
          Overview and Analytics read as one system. */}
      {chips.map((c) => (
        <div key={c.label} style={{
          padding: '16px 18px',
          borderRight: '1px solid var(--border)',
          flex: '1 1 110px', minWidth: 108,
        }}>
          <div style={{
            display: 'flex', alignItems: 'center', gap: 5,
            fontSize: 10.5, fontWeight: 700, letterSpacing: '.04em',
            color: 'var(--text-3)',
          }}>
            <Icon name={c.icon} size={11} />
            {c.label}
          </div>
          <div style={{
            fontSize: 20, fontWeight: 700, color: 'var(--text)',
            letterSpacing: '-.025em', marginTop: 6,
            fontVariantNumeric: 'tabular-nums',
          }}>
            {c.value}
          </div>
        </div>
      ))}

      <Link
        href={href}
        className="btn btn-sm"
        style={{
          alignSelf: 'center', margin: '0 16px',
          textDecoration: 'none', flexShrink: 0,
        }}
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