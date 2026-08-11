'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Charts
//
// Hand-rolled SVG rather than a charting library, for two reasons:
//
//   1. No new dependency. recharts/chart.js would be ~150 kB for what amounts
//      to a polyline and some rects, and adding one contradicts the
//      "don't change anything else" brief.
//   2. Theming. Every colour here is a CSS custom property from globals.css, so
//      the charts follow light/dark with the rest of the dashboard for free.
//      Chart libraries want concrete hex at render time, which would mean
//      re-rendering on every theme flip and keeping a second palette in sync.
//
// All components are presentational and take pre-computed values. No fetching,
// no aggregation, no rate maths — the server owns every number.
// ─────────────────────────────────────────────────────────────────────────────
import { useId, useMemo, useState } from 'react';

// ── Shared geometry ──────────────────────────────────────────────────────────

const PAD = { top: 14, right: 12, bottom: 26, left: 46 };

/** Nice round axis ceiling: 87 → 100, 1240 → 1500. */
function niceMax(max: number): number {
  if (max <= 0) return 1;
  const mag = Math.pow(10, Math.floor(Math.log10(max)));
  const norm = max / mag;
  const step = norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 5 ? 5 : 10;
  return step * mag;
}

function ticks(max: number, count = 4): number[] {
  const out: number[] = [];
  for (let i = 0; i <= count; i++) out.push((max / count) * i);
  return out;
}

export interface Series {
  key: string;
  label: string;
  /** Any CSS colour — pass a var(--…) token so it follows the theme. */
  color: string;
  values: number[];
}

// ── Line chart ───────────────────────────────────────────────────────────────

export function LineChart({
  series, labels, height = 220, formatValue = (n: number) => String(Math.round(n)),
}: {
  series: Series[];
  labels: string[];
  height?: number;
  formatValue?: (n: number) => string;
}) {
  const uid = useId().replace(/[:]/g, '');
  const [hover, setHover] = useState<number | null>(null);
  const width = 900; // viewBox units; the SVG scales to its container

  const n = labels.length;
  const max = useMemo(
    () => niceMax(Math.max(1, ...series.flatMap((s) => s.values))),
    [series],
  );

  const plotW = width - PAD.left - PAD.right;
  const plotH = height - PAD.top - PAD.bottom;
  // n === 1 would divide by zero; a single point sits at the left edge.
  const stepX = n > 1 ? plotW / (n - 1) : 0;

  const x = (i: number) => PAD.left + i * stepX;
  const y = (v: number) => PAD.top + plotH - (v / max) * plotH;

  // Show at most ~7 x labels so they never overlap, whatever the point count.
  const labelEvery = Math.max(1, Math.ceil(n / 7));

  if (n === 0) {
    return <ChartEmpty height={height} />;
  }

  return (
    <div style={{ position: 'relative', width: '100%' }}>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        style={{ width: '100%', height, display: 'block', overflow: 'visible' }}
        onMouseLeave={() => setHover(null)}
        role="img"
        aria-label={series.map((s) => s.label).join(', ')}
      >
        {/* Horizontal grid + y axis */}
        {ticks(max).map((t, i) => (
          <g key={i}>
            <line
              x1={PAD.left} x2={width - PAD.right} y1={y(t)} y2={y(t)}
              stroke="var(--border)" strokeWidth={1}
            />
            <text
              x={PAD.left - 8} y={y(t) + 4} textAnchor="end"
              fontSize={10} fill="var(--text-3)" fontWeight={600}
            >
              {formatValue(t)}
            </text>
          </g>
        ))}

        {/* Series */}
        {series.map((s) => {
          const pts = s.values.map((v, i) => `${x(i)},${y(v)}`).join(' ');
          const area = `${PAD.left},${y(0)} ${pts} ${x(n - 1)},${y(0)}`;
          return (
            <g key={s.key}>
              <defs>
                <linearGradient id={`${uid}-${s.key}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={s.color} stopOpacity={0.22} />
                  <stop offset="100%" stopColor={s.color} stopOpacity={0} />
                </linearGradient>
              </defs>
              {n > 1 && <polygon points={area} fill={`url(#${uid}-${s.key})`} />}
              <polyline
                points={pts} fill="none" stroke={s.color}
                strokeWidth={2} strokeLinejoin="round" strokeLinecap="round"
              />
              {/* A single point has no line to see, so it gets a dot. */}
              {n === 1 && <circle cx={x(0)} cy={y(s.values[0])} r={3.5} fill={s.color} />}
              {hover !== null && (
                <circle
                  cx={x(hover)} cy={y(s.values[hover] ?? 0)} r={4}
                  fill="var(--surface)" stroke={s.color} strokeWidth={2}
                />
              )}
            </g>
          );
        })}

        {/* Hover guide */}
        {hover !== null && (
          <line
            x1={x(hover)} x2={x(hover)} y1={PAD.top} y2={PAD.top + plotH}
            stroke="var(--border-2)" strokeWidth={1} strokeDasharray="3 3"
          />
        )}

        {/* X labels */}
        {labels.map((l, i) =>
          i % labelEvery === 0 ? (
            <text
              key={i} x={x(i)} y={height - 8} textAnchor="middle"
              fontSize={10} fill="var(--text-3)" fontWeight={600}
            >
              {l}
            </text>
          ) : null,
        )}

        {/* Invisible hit strips — one per point, full height, so hovering
            anywhere in a column works rather than requiring pixel-accuracy on
            the line itself. */}
        {labels.map((_, i) => (
          <rect
            key={i}
            x={x(i) - stepX / 2} y={PAD.top}
            width={Math.max(stepX, 6)} height={plotH}
            fill="transparent"
            onMouseEnter={() => setHover(i)}
          />
        ))}
      </svg>

      {hover !== null && (
        <Tooltip
          label={labels[hover]}
          rows={series.map((s) => ({
            label: s.label, color: s.color, value: formatValue(s.values[hover] ?? 0),
          }))}
          leftPct={(x(hover) / width) * 100}
        />
      )}
    </div>
  );
}

// ── Bar chart ────────────────────────────────────────────────────────────────

export function BarChart({
  series, labels, height = 220, formatValue = (n: number) => String(Math.round(n)),
}: {
  series: Series[];
  labels: string[];
  height?: number;
  formatValue?: (n: number) => string;
}) {
  const [hover, setHover] = useState<number | null>(null);
  const width = 900;
  const n = labels.length;
  const max = useMemo(
    () => niceMax(Math.max(1, ...series.flatMap((s) => s.values))),
    [series],
  );

  const plotW = width - PAD.left - PAD.right;
  const plotH = height - PAD.top - PAD.bottom;
  const slot = n > 0 ? plotW / n : plotW;
  // Bars get thin fast at high point counts; a floor of 1 unit keeps them
  // visible rather than collapsing to nothing at 300+ buckets.
  const barW = Math.max(1, (slot / series.length) * 0.62);

  const y = (v: number) => PAD.top + plotH - (v / max) * plotH;
  const labelEvery = Math.max(1, Math.ceil(n / 7));

  if (n === 0) return <ChartEmpty height={height} />;

  return (
    <div style={{ position: 'relative', width: '100%' }}>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        style={{ width: '100%', height, display: 'block', overflow: 'visible' }}
        onMouseLeave={() => setHover(null)}
        role="img"
        aria-label={series.map((s) => s.label).join(', ')}
      >
        {ticks(max).map((t, i) => (
          <g key={i}>
            <line
              x1={PAD.left} x2={width - PAD.right} y1={y(t)} y2={y(t)}
              stroke="var(--border)" strokeWidth={1}
            />
            <text
              x={PAD.left - 8} y={y(t) + 4} textAnchor="end"
              fontSize={10} fill="var(--text-3)" fontWeight={600}
            >
              {formatValue(t)}
            </text>
          </g>
        ))}

        {labels.map((_, i) => (
          <g key={i}>
            {hover === i && (
              <rect
                x={PAD.left + i * slot} y={PAD.top}
                width={slot} height={plotH}
                fill="var(--surface-hover)"
              />
            )}
            {series.map((s, si) => {
              const v = s.values[i] ?? 0;
              const h = Math.max(0, plotH - (y(v) - PAD.top));
              const groupW = barW * series.length;
              const x0 = PAD.left + i * slot + (slot - groupW) / 2 + si * barW;
              return (
                <rect
                  key={s.key}
                  x={x0} y={y(v)} width={barW} height={h}
                  fill={s.color} rx={Math.min(2, barW / 2)}
                  opacity={hover === null || hover === i ? 1 : 0.45}
                />
              );
            })}
            <rect
              x={PAD.left + i * slot} y={PAD.top}
              width={slot} height={plotH}
              fill="transparent"
              onMouseEnter={() => setHover(i)}
            />
          </g>
        ))}

        {labels.map((l, i) =>
          i % labelEvery === 0 ? (
            <text
              key={i} x={PAD.left + i * slot + slot / 2} y={height - 8}
              textAnchor="middle" fontSize={10} fill="var(--text-3)" fontWeight={600}
            >
              {l}
            </text>
          ) : null,
        )}
      </svg>

      {hover !== null && (
        <Tooltip
          label={labels[hover]}
          rows={series.map((s) => ({
            label: s.label, color: s.color, value: formatValue(s.values[hover] ?? 0),
          }))}
          leftPct={((PAD.left + hover * slot + slot / 2) / width) * 100}
        />
      )}
    </div>
  );
}

// ── Funnel ───────────────────────────────────────────────────────────────────

/**
 * Impressions → views → tickets → scanned, as proportional bars.
 *
 * Each step shows its share of the FIRST step (so the shape reads as a funnel)
 * and its conversion from the step immediately before it (which is the number
 * that tells the organiser where they're losing people).
 */
export function Funnel({
  steps,
}: {
  steps: Array<{ label: string; value: number; color: string; hint?: string }>;
}) {
  const top = steps[0]?.value ?? 0;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      {steps.map((s, i) => {
        const prev = i > 0 ? steps[i - 1].value : null;
        const widthPct = top > 0 ? Math.max(1.5, (s.value / top) * 100) : 1.5;
        const fromPrev = prev && prev > 0 ? s.value / prev : null;
        return (
          <div key={s.label}>
            <div style={{
              display: 'flex', alignItems: 'baseline', justifyContent: 'space-between',
              gap: 12, marginBottom: 5,
            }}>
              <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--text-2)' }}>
                {s.label}
              </span>
              <span style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
                <span style={{
                  fontSize: 15, fontWeight: 800, color: 'var(--text)',
                  fontVariantNumeric: 'tabular-nums',
                }}>
                  {new Intl.NumberFormat('en-ZA').format(s.value)}
                </span>
                {fromPrev !== null && (
                  <span style={{ fontSize: 11.5, color: 'var(--text-3)', fontWeight: 600 }}>
                    {(fromPrev * 100).toFixed(1)}% of previous
                  </span>
                )}
              </span>
            </div>
            <div style={{
              height: 10, borderRadius: 999,
              background: 'var(--surface-strong)', overflow: 'hidden',
            }}>
              <div style={{
                width: `${widthPct}%`, height: '100%',
                background: s.color, borderRadius: 999,
                transition: 'width .35s ease',
              }} />
            </div>
            {s.hint && (
              <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 4 }}>
                {s.hint}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

// ── Shared bits ──────────────────────────────────────────────────────────────

function Tooltip({
  label, rows, leftPct,
}: {
  label: string;
  rows: Array<{ label: string; color: string; value: string }>;
  leftPct: number;
}) {
  // Flip the anchor past the midpoint so the tooltip never runs off the right
  // edge of the card.
  const flip = leftPct > 60;
  return (
    <div
      style={{
        position: 'absolute', top: 4,
        left: `${Math.min(Math.max(leftPct, 2), 98)}%`,
        transform: flip ? 'translateX(-100%) translateX(-10px)' : 'translateX(10px)',
        background: 'var(--bg-elevated)',
        border: '1px solid var(--border-2)',
        borderRadius: 10, padding: '9px 11px',
        boxShadow: 'var(--shadow-pop)',
        pointerEvents: 'none', zIndex: 5, minWidth: 132,
      }}
    >
      <div style={{
        fontSize: 10.5, fontWeight: 800, letterSpacing: '.06em',
        textTransform: 'uppercase', color: 'var(--text-3)', marginBottom: 6,
      }}>
        {label}
      </div>
      {rows.map((r) => (
        <div key={r.label} style={{
          display: 'flex', alignItems: 'center', gap: 7,
          fontSize: 12, marginTop: 3,
        }}>
          <span style={{
            width: 7, height: 7, borderRadius: 2,
            background: r.color, flexShrink: 0,
          }} />
          <span style={{ color: 'var(--text-2)', flex: 1 }}>{r.label}</span>
          <span style={{
            color: 'var(--text)', fontWeight: 700,
            fontVariantNumeric: 'tabular-nums',
          }}>
            {r.value}
          </span>
        </div>
      ))}
    </div>
  );
}

function ChartEmpty({ height }: { height: number }) {
  return (
    <div style={{
      height, display: 'grid', placeItems: 'center',
      color: 'var(--text-3)', fontSize: 13,
    }}>
      Nothing recorded in this window
    </div>
  );
}

export function Legend({ series }: { series: Series[] }) {
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 14 }}>
      {series.map((s) => (
        <span key={s.key} style={{
          display: 'inline-flex', alignItems: 'center', gap: 6,
          fontSize: 12, color: 'var(--text-2)', fontWeight: 600,
        }}>
          <span style={{
            width: 9, height: 9, borderRadius: 3,
            background: s.color, flexShrink: 0,
          }} />
          {s.label}
        </span>
      ))}
    </div>
  );
}