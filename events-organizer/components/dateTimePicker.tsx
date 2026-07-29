'use client';
// ── Date & time picker ───────────────────────────────────────────────────────
// A drop-in replacement for <input type="datetime-local">.
//
// The native control was the weak point of the create-event form: its calendar
// is rendered by the browser, so it looks different in every one of them, can't
// be styled to match the dashboard, and on Chrome puts the date grid and two
// scrolling number columns side by side in a popup that's easy to mis-tap.
//
// It renders as TWO fields, not one. The first attempt put the calendar and the
// time selectors together in a single popover, which failed twice over: the
// popover was tall enough to be clipped by the enclosing card, and because the
// time controls sat below the calendar grid they were the part that got cut —
// so the time was literally unreachable. A date button beside a plain time
// input keeps the popover short and keeps time always visible.
//
// The contract here is deliberately identical to the native input —
//   value:    'YYYY-MM-DDTHH:mm'  (local wall-clock, no timezone suffix)
//   onChange: (next: string) => void
// — so swapping it in required no change to form state, validation, or the
// `new Date(startTime).toISOString()` calls on submit.
//
// TIMEZONE NOTE: like the native control, every value here is local wall-clock
// time with no offset. All the arithmetic below uses getFullYear/getMonth/
// getDate rather than anything UTC-based, because building the string via
// toISOString() would silently shift the date by a day for anyone east or west
// of UTC — which in SAST (UTC+2) would turn a 00:30 event into the previous
// day. The event's actual timezone is chosen separately in the form's Timezone
// field and applied server-side.
import { useEffect, useMemo, useRef, useState } from 'react';
import type { CSSProperties } from 'react';

const DAYS = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'];
const MONTHS = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
];
const MONTHS_SHORT = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

// ── Value helpers ────────────────────────────────────────────────────────────

const pad = (n: number) => String(n).padStart(2, '0');

/** Date → 'YYYY-MM-DDTHH:mm', matching what datetime-local emits. */
function toValue(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** 'YYYY-MM-DDTHH:mm' → Date, or null if absent/unparseable. */
function fromValue(v: string): Date | null {
  if (!v) return null;
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(v);
  if (!m) return null;
  const d = new Date(
    Number(m[1]), Number(m[2]) - 1, Number(m[3]),
    Number(m[4]), Number(m[5]), 0, 0,
  );
  return Number.isNaN(d.getTime()) ? null : d;
}

/** Label for the date button, e.g. "Wed 29 Jul 2026". No time — that has its
 *  own field, and repeating it here just made the button overflow. */
function toDisplay(d: Date): string {
  return `${DAYS[d.getDay()]} ${d.getDate()} ${MONTHS_SHORT[d.getMonth()]} ${d.getFullYear()}`;
}

/** Midnight on the same day — used for date-only comparisons. */
function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear()
    && a.getMonth() === b.getMonth()
    && a.getDate() === b.getDate();
}

/** The 42 cells of a month grid: leading/trailing days included for alignment. */
function monthGrid(year: number, month: number): Date[] {
  const first = new Date(year, month, 1);
  const start = new Date(year, month, 1 - first.getDay());
  return Array.from({ length: 42 }, (_, i) =>
    new Date(start.getFullYear(), start.getMonth(), start.getDate() + i));
}

// ── Component ────────────────────────────────────────────────────────────────

interface Props {
  value: string;
  onChange: (next: string) => void;
  /** Lower bound, same format. Earlier days are shown but not selectable. */
  min?: string;
  /** Shown when empty. */
  placeholder?: string;
  style?: CSSProperties;
  /** Marks the trigger with an error outline (form validation). */
  invalid?: boolean;
}

export default function DateTimePicker({
  value,
  onChange,
  min,
  placeholder = 'Select date and time',
  style,
  invalid = false,
}: Props) {
  const selected = useMemo(() => fromValue(value), [value]);
  const minDate = useMemo(() => (min ? fromValue(min) : null), [min]);

  const [open, setOpen] = useState(false);
  // The month being browsed, which is independent of the selected day —
  // paging to March shouldn't change the value until a day is clicked.
  const [view, setView] = useState<Date>(() => selected ?? new Date());

  const boxRef = useRef<HTMLDivElement | null>(null);

  // Re-centre the calendar when the value changes from outside (draft restore,
  // or end-time being nudged forward after start-time moves).
  useEffect(() => {
    if (selected) setView(selected);
  }, [selected]);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('mousedown', onDoc);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDoc);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  const cells = useMemo(() => monthGrid(view.getFullYear(), view.getMonth()), [view]);
  const today = useMemo(() => startOfDay(new Date()), []);
  const minDay = minDate ? startOfDay(minDate) : null;

  /** Apply a new date while preserving the chosen time (default 19:00). */
  const pickDay = (day: Date) => {
    const base = selected ?? new Date(day.getFullYear(), day.getMonth(), day.getDate(), 19, 0);
    const next = new Date(
      day.getFullYear(), day.getMonth(), day.getDate(),
      base.getHours(), base.getMinutes(),
    );
    // Selecting the boundary day can land before `min` if the time is earlier;
    // clamp rather than emitting a value the form will just reject.
    if (minDate && next < minDate) {
      onChange(toValue(minDate));
      return;
    }
    onChange(toValue(next));
  };

  /** Apply a new time while preserving the chosen day (default today). */
  const pickTime = (hours: number, minutes: number) => {
    const base = selected ?? new Date();
    const next = new Date(
      base.getFullYear(), base.getMonth(), base.getDate(), hours, minutes,
    );
    if (minDate && next < minDate) {
      onChange(toValue(minDate));
      return;
    }
    onChange(toValue(next));
  };

  const shiftMonth = (delta: number) =>
    setView(v => new Date(v.getFullYear(), v.getMonth() + delta, 1));

  // ── Styles (inline: this component is used inside .field blocks that already
  // style bare inputs, and a stylesheet entry would be one more file to keep in
  // sync between the organiser and admin dashboards) ─────────────────────────
  const trigger: CSSProperties = {
    width: '100%',
    padding: '9px 12px',
    border: `1px solid ${invalid ? 'var(--danger)' : 'var(--border-2)'}`,
    borderRadius: 'var(--r)',
    background: 'var(--surface)',
    color: selected ? 'var(--text)' : 'var(--text-3)',
    fontSize: 14,
    fontFamily: 'inherit',
    textAlign: 'left',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 8,
    ...style,
  };

  const popover: CSSProperties = {
    position: 'absolute',
    zIndex: 50,
    top: 'calc(100% + 6px)',
    left: 0,
    background: 'var(--surface)',
    border: '1px solid var(--border-2)',
    borderRadius: 'var(--r-lg)',
    boxShadow: 'var(--shadow)',
    padding: 12,
    width: 300,
  };

  const navBtn: CSSProperties = {
    width: 28, height: 28,
    display: 'grid', placeItems: 'center',
    border: '1px solid var(--border)',
    borderRadius: 'var(--r)',
    background: 'var(--surface)',
    color: 'var(--text-2)',
    cursor: 'pointer',
    fontSize: 14, lineHeight: 1,
  };



  const timeValue = selected ? `${pad(selected.getHours())}:${pad(selected.getMinutes())}` : '';

  return (
    <div style={{ display: 'flex', gap: 8, alignItems: 'stretch' }}>
      {/* Date — custom, because a month grid is the part worth styling */}
      <div ref={boxRef} style={{ position: 'relative', flex: 1, minWidth: 0 }}>
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        style={trigger}
        aria-haspopup="dialog"
        aria-expanded={open}
      >
        <span style={{
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>
          {selected ? toDisplay(selected) : placeholder}
        </span>
        <span aria-hidden style={{ color: 'var(--text-3)', fontSize: 15, flexShrink: 0 }}>📅</span>
      </button>

      {open && (
        <div style={popover} role="dialog" aria-label="Choose date and time">
          {/* Month navigation */}
          <div style={{
            display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            marginBottom: 10,
          }}>
            <button type="button" onClick={() => shiftMonth(-1)} style={navBtn} aria-label="Previous month">‹</button>
            <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text)' }}>
              {MONTHS[view.getMonth()]} {view.getFullYear()}
            </div>
            <button type="button" onClick={() => shiftMonth(1)} style={navBtn} aria-label="Next month">›</button>
          </div>

          {/* Weekday header */}
          <div style={{
            display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: 2, marginBottom: 4,
          }}>
            {DAYS.map(d => (
              <div key={d} style={{
                textAlign: 'center', fontSize: 11, fontWeight: 600,
                color: 'var(--text-3)', padding: '4px 0',
              }}>{d}</div>
            ))}
          </div>

          {/* Day grid */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: 2 }}>
            {cells.map((day, i) => {
              const inMonth = day.getMonth() === view.getMonth();
              const isSel = selected ? sameDay(day, selected) : false;
              const isToday = sameDay(day, today);
              const disabled = minDay ? startOfDay(day) < minDay : false;
              return (
                <button
                  key={i}
                  type="button"
                  disabled={disabled}
                  onClick={() => pickDay(day)}
                  style={{
                    padding: '7px 0',
                    fontSize: 13,
                    fontFamily: 'inherit',
                    border: isToday && !isSel ? '1px solid var(--blue)' : '1px solid transparent',
                    borderRadius: 'var(--r)',
                    background: isSel ? 'var(--blue)' : 'transparent',
                    color: disabled
                      ? 'var(--border-2)'
                      : isSel
                        ? 'var(--text-inv)'
                        : inMonth ? 'var(--text)' : 'var(--text-3)',
                    fontWeight: isSel ? 700 : inMonth ? 500 : 400,
                    cursor: disabled ? 'not-allowed' : 'pointer',
                  }}
                >
                  {day.getDate()}
                </button>
              );
            })}
          </div>

          {/* Footer */}
          <div style={{
            display: 'flex', justifyContent: 'space-between', alignItems: 'center',
            marginTop: 10, paddingTop: 10, borderTop: '1px solid var(--border)',
          }}>
            <button
              type="button"
              onClick={() => { onChange(''); setOpen(false); }}
              style={{
                background: 'none', border: 'none', cursor: 'pointer',
                color: 'var(--text-3)', fontSize: 12.5, textDecoration: 'underline',
                fontFamily: 'inherit', padding: 0,
              }}
            >
              Clear
            </button>
            <button
              type="button"
              onClick={() => setOpen(false)}
              style={{
                padding: '6px 14px',
                border: 'none', borderRadius: 'var(--r)',
                background: 'var(--blue)', color: 'var(--text-inv)',
                fontSize: 13, fontWeight: 600, cursor: 'pointer',
                fontFamily: 'inherit',
              }}
            >
              Done
            </button>
          </div>
        </div>
      )}
      </div>

      {/* Time — the native control. Small, precise to the minute, keyboard
          friendly, and with no popover of its own it cannot be clipped. */}
      <input
        type="time"
        value={timeValue}
        onChange={e => {
          const [h, m] = e.target.value.split(':');
          if (h === undefined || m === undefined || e.target.value === '') return;
          pickTime(Number(h), Number(m));
        }}
        aria-label="Time"
        style={{
          width: 116, flexShrink: 0,
          padding: '9px 10px',
          border: `1px solid ${invalid ? 'var(--danger)' : 'var(--border-2)'}`,
          borderRadius: 'var(--r)',
          background: 'var(--surface)',
          color: selected ? 'var(--text)' : 'var(--text-3)',
          fontSize: 14,
          fontFamily: 'inherit',
        }}
      />
    </div>
  );
}