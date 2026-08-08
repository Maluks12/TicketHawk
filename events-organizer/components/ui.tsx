'use client';
// ── Shared dashboard UI primitives ───────────────────────────────────────────
// Small pieces that were previously re-implemented inline on every page, each
// time with slightly different hardcoded hex values. Centralising them fixes
// two visible problems at once:
//
//   1. Status rendering was inconsistent. The pages used
//      `className={'badge badge-' + status}`, but globals.css only defines
//      variants for draft/published/cancelled/pending. Anything else —
//      'completed', 'published_private', 'pending_review', 'rejected' — got a
//      class that doesn't exist and rendered as bare text next to properly
//      styled pills. StatusPill below covers every status the API can emit, so
//      a new one can't silently fall through to unstyled.
//
//   2. Raw enum values leaked to organisers. 'published_private' is a database
//      value, not a phrase anyone should read in a UI. Labels are mapped.
//
// Restyled to the Studio language (see components/studio/ui.tsx in
// EventsStudio): chip with a leading status dot, uppercase micro-label on stat
// cards, tinted icon tile, hairline borders. Every colour is a custom property
// from globals.css — no literal hex — so both themes come free and a future
// palette change still lands in one place.
import type { CSSProperties, ReactNode } from 'react';

// ── Status ───────────────────────────────────────────────────────────────────

type Tone = 'green' | 'grey' | 'amber' | 'red' | 'blue';

const TONES: Record<Tone, { bg: string; fg: string; dot: string }> = {
  green: { bg: 'var(--success-bg)', fg: 'var(--success-fg)', dot: 'var(--success)' },
  grey:  { bg: 'var(--surface-strong)', fg: 'var(--text-2)', dot: 'var(--text-3)' },
  amber: { bg: 'var(--warning-bg)', fg: 'var(--warning-fg)', dot: 'var(--warning)' },
  red:   { bg: 'var(--danger-bg)', fg: 'var(--danger-fg)', dot: 'var(--danger)' },
  blue:  { bg: 'var(--info-bg)', fg: 'var(--info-fg)', dot: 'var(--info)' },
};

/**
 * Every event status the backend can return, with the words an organiser
 * should actually see. Unknown values fall back to a de-underscored,
 * capitalised version rather than rendering the raw enum.
 */
const STATUS: Record<string, { label: string; tone: Tone; hint?: string }> = {
  draft:            { label: 'Draft',        tone: 'grey',  hint: 'Not visible to anyone yet' },
  pending_review:   { label: 'In review',    tone: 'amber', hint: 'Waiting on admin approval' },
  published:        { label: 'Live',         tone: 'green', hint: 'Visible in the TicketHawk feed' },
  published_private:{ label: 'Private link', tone: 'blue',  hint: 'Only reachable via the share link' },
  rejected:         { label: 'Rejected',     tone: 'red',   hint: 'Admin declined this event' },
  cancelled:        { label: 'Cancelled',    tone: 'red',   hint: 'Called off; buyers refunded in full' },
  completed:        { label: 'Completed',    tone: 'grey',  hint: 'Event has finished' },
};

function humanise(raw: string): string {
  const s = raw.replace(/_/g, ' ').trim();
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export function statusLabel(status: string): string {
  return STATUS[status]?.label ?? humanise(status);
}

export function statusHint(status: string): string | undefined {
  return STATUS[status]?.hint;
}

export function StatusPill({ status, style }: { status: string; style?: CSSProperties }) {
  const meta = STATUS[status];
  const tone = TONES[meta?.tone ?? 'grey'];
  return (
    <span
      title={meta?.hint}
      style={{
        display: 'inline-flex', alignItems: 'center', gap: 6,
        padding: '4px 10px', borderRadius: 999,
        background: tone.bg, color: tone.fg,
        fontSize: 11.5, fontWeight: 700, whiteSpace: 'nowrap',
        letterSpacing: '.01em',
        ...style,
      }}
    >
      <span style={{
        width: 6, height: 6, borderRadius: '50%',
        background: tone.dot, flexShrink: 0,
      }} />
      {meta?.label ?? humanise(status)}
    </span>
  );
}

// ── Stat card ────────────────────────────────────────────────────────────────

export function StatCard({
  label, value, sub, accent = 'blue', icon,
}: {
  label: string;
  value: string | number;
  sub?: string;
  accent?: 'blue' | 'green' | 'orange' | 'grey';
  icon?: ReactNode;
}) {
  const accents = {
    blue:   { line: 'var(--accent)',  bg: 'var(--accent-soft)',  fg: 'var(--accent)' },
    green:  { line: 'var(--success)', bg: 'var(--success-bg)',   fg: 'var(--success-fg)' },
    orange: { line: 'var(--warning)', bg: 'var(--warning-bg)',   fg: 'var(--warning-fg)' },
    grey:   { line: 'var(--text-3)',  bg: 'var(--surface-strong)', fg: 'var(--text-2)' },
  }[accent];

  return (
    <div style={{
      background: 'var(--surface)',
      border: '1px solid var(--border)',
      // Studio marks accent on the leading edge rather than the top edge — a
      // left rail reads as a category tab instead of a stray underline.
      boxShadow: `inset 3px 0 0 ${accents.line}`,
      borderRadius: 'var(--r-lg)',
      padding: 17,
    }}>
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        marginBottom: 11, gap: 8,
      }}>
        <div style={{
          fontSize: 10, fontWeight: 800, color: 'var(--text-3)',
          textTransform: 'uppercase', letterSpacing: '.1em',
        }}>
          {label}
        </div>
        {icon && (
          <div style={{
            width: 30, height: 30, borderRadius: 10,
            background: accents.bg, color: accents.fg,
            display: 'grid', placeItems: 'center', flexShrink: 0,
          }}>
            {icon}
          </div>
        )}
      </div>
      <div style={{
        fontSize: 27, fontWeight: 800, color: 'var(--text)',
        lineHeight: 1, letterSpacing: '-.03em', fontVariantNumeric: 'tabular-nums',
      }}>
        {value}
      </div>
      {sub && (
        <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 6 }}>{sub}</div>
      )}
    </div>
  );
}

// ── Panel ────────────────────────────────────────────────────────────────────

export function Panel({
  title, action, children, bodyPadding = 0,
}: {
  title: string;
  action?: ReactNode;
  children: ReactNode;
  bodyPadding?: number;
}) {
  return (
    <div style={{
      background: 'var(--surface)',
      border: '1px solid var(--border)',
      borderRadius: 'var(--r-xl)',
      overflow: 'hidden',
      display: 'flex',
      flexDirection: 'column',
    }}>
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '15px 18px', borderBottom: '1px solid var(--border)',
      }}>
        <div style={{ fontSize: 14.5, fontWeight: 800, color: 'var(--text)', letterSpacing: '-.01em' }}>
          {title}
        </div>
        {action}
      </div>
      <div style={{ padding: bodyPadding, flex: 1 }}>{children}</div>
    </div>
  );
}

// ── Empty state ──────────────────────────────────────────────────────────────

export function EmptyState({
  title, body, action,
}: {
  title: string;
  body?: string;
  action?: ReactNode;
}) {
  return (
    <div style={{ padding: '34px 24px', textAlign: 'center' }}>
      <div style={{ fontSize: 14, fontWeight: 700, color: 'var(--text-2)' }}>{title}</div>
      {body && (
        <div style={{
          fontSize: 12.5, color: 'var(--text-3)', marginTop: 5,
          maxWidth: 340, marginLeft: 'auto', marginRight: 'auto', lineHeight: 1.6,
        }}>
          {body}
        </div>
      )}
      {action && <div style={{ marginTop: 14 }}>{action}</div>}
    </div>
  );
}