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
// Everything uses the CSS custom properties from globals.css rather than
// literal hex, so a future palette change lands in one place.
import type { CSSProperties, ReactNode } from 'react';

// ── Status ───────────────────────────────────────────────────────────────────

type Tone = 'green' | 'grey' | 'amber' | 'red' | 'blue';

const TONES: Record<Tone, { bg: string; fg: string; dot: string }> = {
  green: { bg: '#e6f4ea', fg: '#1e7e34', dot: 'var(--success)' },
  grey:  { bg: 'var(--surface-3)', fg: 'var(--text-2)', dot: 'var(--border-2)' },
  amber: { bg: 'var(--warning-bg)', fg: '#92400e', dot: 'var(--warning)' },
  red:   { bg: 'var(--danger-bg)', fg: 'var(--danger)', dot: 'var(--danger)' },
  blue:  { bg: 'var(--blue-light)', fg: 'var(--blue)', dot: 'var(--blue)' },
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
        padding: '3px 9px', borderRadius: 999,
        background: tone.bg, color: tone.fg,
        fontSize: 12, fontWeight: 600, whiteSpace: 'nowrap',
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
    blue:   { line: 'var(--blue)', bg: 'var(--blue-light)', fg: 'var(--blue)' },
    green:  { line: '#1a7f37', bg: '#f0fff4', fg: '#1a7f37' },
    orange: { line: '#e65c00', bg: '#fff8f0', fg: '#e65c00' },
    grey:   { line: 'var(--border-2)', bg: 'var(--surface-3)', fg: 'var(--text-2)' },
  }[accent];

  return (
    <div style={{
      background: 'var(--surface)',
      border: '1px solid var(--border)',
      borderTop: `3px solid ${accents.line}`,
      borderRadius: 'var(--r-lg)',
      padding: 16,
    }}>
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        marginBottom: 10, gap: 8,
      }}>
        <div style={{
          fontSize: 12, fontWeight: 600, color: 'var(--text-2)',
          textTransform: 'uppercase', letterSpacing: '.04em',
        }}>
          {label}
        </div>
        {icon && (
          <div style={{
            width: 32, height: 32, borderRadius: 'var(--r)',
            background: accents.bg, color: accents.fg,
            display: 'grid', placeItems: 'center', flexShrink: 0,
          }}>
            {icon}
          </div>
        )}
      </div>
      <div style={{ fontSize: 26, fontWeight: 700, color: 'var(--text)', lineHeight: 1 }}>
        {value}
      </div>
      {sub && (
        <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 4 }}>{sub}</div>
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
      borderRadius: 'var(--r-lg)',
      overflow: 'hidden',
      display: 'flex',
      flexDirection: 'column',
    }}>
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '14px 16px', borderBottom: '1px solid var(--surface-3)',
      }}>
        <div style={{ fontSize: 15, fontWeight: 700, color: 'var(--text)' }}>{title}</div>
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
    <div style={{ padding: '28px 24px', textAlign: 'center' }}>
      <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-2)' }}>{title}</div>
      {body && (
        <div style={{
          fontSize: 12.5, color: 'var(--text-3)', marginTop: 4,
          maxWidth: 320, marginLeft: 'auto', marginRight: 'auto', lineHeight: 1.5,
        }}>
          {body}
        </div>
      )}
      {action && <div style={{ marginTop: 12 }}>{action}</div>}
    </div>
  );
}