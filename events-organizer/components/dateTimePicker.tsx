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
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { CSSProperties, RefObject } from 'react';
import { CalendarBlank, Clock, CaretLeft, CaretRight } from '@phosphor-icons/react';

// ── Popover placement ────────────────────────────────────────────────────────
//
// Both popovers here used to be hardcoded to open DOWNWARD, which is fine in
// the middle of a form and wrong at the bottom of one. "Doors open" is the last
// row of the information editor, so its popover opened into the space below the
// card and the hour and minute columns were simply unreachable — you could see
// COMMON TIMES and nothing under it.
//
// Note this is only half the fix. The other half is that `.card` in globals.css
// sets `overflow: hidden`, which CLIPS an absolutely positioned child no matter
// where it is placed. The create wizard hit the same thing and solved it by
// dropping that rule from FormCard; the event page's Information and Line-up
// cards now do the same. Flipping without unclipping just moves the cut edge.
//
// Measured rather than assumed, and re-measured while open, so the same control
// behaves correctly in a tall card and a short one.

/** Breathing room between the trigger and the popover. */
const POPOVER_GAP = 6;
/** Keep the popover off the very edge of the viewport. */
const VIEWPORT_EDGE = 10;
/** Under this much room, a side counts as "no room" and the other side wins. */
const POPOVER_MIN_H = 240;

// useLayoutEffect warns when React renders on the server, and every screen in
// this dashboard is server-rendered before it hydrates. The measurement can
// only run in a browser anyway, so pick the effect that suits where we are.
const useIsoLayoutEffect = typeof window === 'undefined' ? useEffect : useLayoutEffect;

function usePlacement(open: boolean, anchorRef: RefObject<HTMLElement | null>) {
  const [place, setPlace] = useState<{ up: boolean; maxHeight: number }>(
    { up: false, maxHeight: 0 },
  );

  useIsoLayoutEffect(() => {
    if (!open) return;
    const measure = () => {
      const el = anchorRef.current;
      if (!el) return;
      const r = el.getBoundingClientRect();
      const below = window.innerHeight - r.bottom - POPOVER_GAP - VIEWPORT_EDGE;
      const above = r.top - POPOVER_GAP - VIEWPORT_EDGE;
      // Prefer downward — that is where a dropdown is expected. Flip only when
      // down genuinely cannot hold the popover AND up has more room.
      const up = below < POPOVER_MIN_H && above > below;
      setPlace({ up, maxHeight: Math.max(160, up ? above : below) });
    };
    measure();
    window.addEventListener('resize', measure);
    // capture: true — the scroll that moves this trigger is .page-content's,
    // not the window's, and scroll events do not bubble.
    window.addEventListener('scroll', measure, true);
    return () => {
      window.removeEventListener('resize', measure);
      window.removeEventListener('scroll', measure, true);
    };
  }, [open, anchorRef]);

  return place;
}

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
/** '8 Aug 2026' — no time, for dateOnly mode. */
function toDisplayDate(d: Date): string {
  return `${d.getDate()} ${MONTHS_SHORT[d.getMonth()]} ${d.getFullYear()}`;
}

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
  /** Upper bound, same format. Later days are shown but not selectable. */
  max?: string;
  /** Marks the trigger with an error outline (form validation). */
  invalid?: boolean;
  /**
   * Date-only: hides the time field and emits 'YYYY-MM-DD'.
   *
   * Used for date-of-birth on sign-up, which has no meaningful time component —
   * and where the native <input type="date"> was the last un-themed control
   * left in the product, rendering Chrome's own light-on-dark calendar.
   */
  dateOnly?: boolean;
}

// NOTE: every label drawn on an --accent-deep fill uses --on-accent, never
// '#fff'. The accent is near-black in the light theme and near-white in
// dark, so a hardcoded white label made the selected date, the AM/PM
// toggle and the Done button white-on-white and invisible in dark mode.
// The token flips with the theme; the literal could not.

export default function DateTimePicker({
  value,
  onChange,
  min,
  max,
  placeholder,
  style,
  invalid = false,
  dateOnly = false,
}: Props) {
  // In dateOnly mode the value is a bare 'YYYY-MM-DD'; fromValue expects a time
  // part, so one is appended for parsing and stripped again on emit.
  const selected = useMemo(
    () => fromValue(dateOnly && value && !value.includes('T') ? `${value}T00:00` : value),
    [value, dateOnly],
  );
  const minDate = useMemo(
    () => (min ? fromValue(min.includes('T') ? min : `${min}T00:00`) : null),
    [min],
  );
  const maxDate = useMemo(
    () => (max ? fromValue(max.includes('T') ? max : `${max}T23:59`) : null),
    [max],
  );
  const ph = placeholder ?? (dateOnly ? 'Select a date' : 'Select date and time');

  const [open, setOpen] = useState(false);
  // The month being browsed, which is independent of the selected day —
  // paging to March shouldn't change the value until a day is clicked.
  const [view, setView] = useState<Date>(() => selected ?? new Date());

  const boxRef = useRef<HTMLDivElement | null>(null);
  const placement = usePlacement(open, boxRef);

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
  const maxDay = maxDate ? startOfDay(maxDate) : null;

  /** Emit in the format the caller expects: date-only or date+time. */
  const emit = (d: Date) => onChange(dateOnly ? toValue(d).slice(0, 10) : toValue(d));

  /** Apply a new date while preserving the chosen time (default 19:00). */
  const pickDay = (day: Date) => {
    const base = selected ?? new Date(day.getFullYear(), day.getMonth(), day.getDate(), 19, 0);
    const next = new Date(
      day.getFullYear(), day.getMonth(), day.getDate(),
      dateOnly ? 0 : base.getHours(), dateOnly ? 0 : base.getMinutes(),
    );
    // Selecting the boundary day can land before `min` if the time is earlier;
    // clamp rather than emitting a value the form will just reject.
    if (!dateOnly && minDate && next < minDate) {
      emit(minDate);
      if (dateOnly) setOpen(false);
      return;
    }
    emit(next);
    // Date-only has no second step, so the popover has nothing left to do.
    if (dateOnly) setOpen(false);
  };

  /** Apply a new time while preserving the chosen day (default today). */
  const pickTime = (hours: number, minutes: number) => {
    const base = selected ?? new Date();
    const next = new Date(
      base.getFullYear(), base.getMonth(), base.getDate(), hours, minutes,
    );
    if (minDate && next < minDate) {
      emit(minDate);
      return;
    }
    emit(next);
  };

  const shiftMonth = (delta: number) =>
    setView(v => new Date(v.getFullYear(), v.getMonth() + delta, 1));

  /**
   * Years offered in the jump dropdown.
   *
   * Bounded by min/max when the caller gives them, so a date-of-birth field
   * capped at today can't offer 2030, and an event picker can't offer 1920.
   * Newest first: both use cases start from the recent end.
   */
  const yearRange = useMemo(() => {
    const now = new Date().getFullYear();
    const lo = minDate ? minDate.getFullYear() : now - 100;
    const hi = maxDate ? maxDate.getFullYear() : now + 10;
    const out: number[] = [];
    for (let y = hi; y >= lo; y--) out.push(y);
    // The current view might sit outside the bound range (a restored draft, or
    // a value that predates a later-added min). Keep it selectable rather than
    // letting the select fall back to a year nobody chose.
    const vy = view.getFullYear();
    if (!out.includes(vy)) out.push(vy);
    return out;
  }, [minDate, maxDate, view]);

  const jumpSelect: CSSProperties = {
    padding: '4px 6px',
    border: '1px solid var(--border)',
    borderRadius: 7,
    // Opaque, not the alpha surface: Chrome derives the native option list's
    // background from the select's own, and alpha there composites over white.
    background: 'var(--surface-2)',
    color: 'var(--text)',
    fontSize: 13, fontWeight: 600, fontFamily: 'inherit',
    cursor: 'pointer', outline: 'none',
  };

  // ── Styles (inline: this component is used inside .field blocks that already
  // style bare inputs, and a stylesheet entry would be one more file to keep in
  // sync between the organiser and admin dashboards) ─────────────────────────
  const trigger: CSSProperties = {
    width: '100%',
    padding: '9px 12px',
    border: `1px solid ${invalid ? 'var(--danger)' : 'var(--border-2)'}`,
    borderRadius: 'var(--r)',
    background: 'var(--surface-alpha)',
    color: selected ? 'var(--text)' : 'var(--text-faint)',
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
    ...(placement.up
      ? { bottom: `calc(100% + ${POPOVER_GAP}px)` }
      : { top: `calc(100% + ${POPOVER_GAP}px)` }),
    left: 0,
    background: 'var(--bg-elevated)',
    border: '1px solid var(--border-2)',
    borderRadius: 'var(--r-lg)',
    boxShadow: 'var(--shadow-pop)',
    padding: 12,
    width: 300,
    // A last resort, not the plan: on a short window even the better side may
    // not fit the whole month grid, and a scrollable popover beats a truncated
    // one. maxHeight is 0 until the first measurement, so it is only applied
    // once there is a real number.
    ...(placement.maxHeight ? { maxHeight: placement.maxHeight, overflowY: 'auto' as const } : {}),
  };

  const navBtn: CSSProperties = {
    width: 28, height: 28,
    display: 'grid', placeItems: 'center',
    border: '1px solid var(--border)',
    borderRadius: 'var(--r)',
    background: 'var(--surface-alpha)',
    color: 'var(--text-2)',
    cursor: 'pointer',
    fontSize: 14, lineHeight: 1,
  };



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
          {selected
            ? (dateOnly ? toDisplayDate(selected) : toDisplay(selected))
            : ph}
        </span>
        <CalendarBlank size={16} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
      </button>

      {open && (
        <div style={popover} role="dialog" aria-label={dateOnly ? 'Choose a date' : 'Choose date and time'}>
          {/* Month navigation */}
          <div style={{
            display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            marginBottom: 10,
          }}>
            <button type="button" onClick={() => shiftMonth(-1)} style={navBtn} aria-label="Previous month">
              <CaretLeft size={14} weight="bold" />
            </button>
            {/* Month and year are SELECTS, not just a label.
                Paging one month at a time is fine for an event three weeks out
                and unusable for a date of birth — reaching 1994 from today
                would be nearly 400 clicks on the arrow. The arrows stay for
                fine adjustment; the dropdowns make any date two clicks away. */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <select
                aria-label="Month"
                value={view.getMonth()}
                onChange={e => setView(v => new Date(v.getFullYear(), Number(e.target.value), 1))}
                style={jumpSelect}
              >
                {MONTHS.map((m, i) => <option key={m} value={i}>{m}</option>)}
              </select>
              <select
                aria-label="Year"
                value={view.getFullYear()}
                onChange={e => setView(v => new Date(Number(e.target.value), v.getMonth(), 1))}
                style={{ ...jumpSelect, width: 76 }}
              >
                {yearRange.map(y => <option key={y} value={y}>{y}</option>)}
              </select>
            </div>
            <button type="button" onClick={() => shiftMonth(1)} style={navBtn} aria-label="Next month">
              <CaretRight size={14} weight="bold" />
            </button>
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
              const dayStart = startOfDay(day);
              const disabled = (minDay ? dayStart < minDay : false)
                            || (maxDay ? dayStart > maxDay : false);
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
                    border: isToday && !isSel ? '1px solid var(--accent)' : '1px solid transparent',
                    borderRadius: 'var(--r)',
                    background: isSel ? 'var(--accent-deep)' : 'transparent',
                    color: disabled
                      ? 'var(--text-faint)'
                      : isSel
                        ? 'var(--on-accent)'
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
                background: 'var(--accent-deep)', color: 'var(--on-accent)',
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

      {/* Time — custom, NOT <input type="time">.
          The native control renders as two unlabelled scrolling number columns
          in Chrome, which is impossible to tell apart at a glance: the hour and
          minute lists look identical and neither says which is which. This is a
          labelled 12-hour picker with an AM/PM toggle and quick presets, so
          choosing 7pm takes one tap instead of two scrolls. */}
      {!dateOnly && (
        <TimeField
          selected={selected}
          invalid={invalid}
          onPick={pickTime}
        />
      )}
    </div>
  );
}

// ── TimeField ────────────────────────────────────────────────────────────────
// A labelled replacement for <input type="time">.
//
// Three deliberate choices:
//
//   1. 12-hour with an AM/PM toggle. Every organiser writing "doors at 8" means
//      20:00, and the 24-hour native control makes them do the conversion. The
//      value emitted is still 24-hour — this is display only.
//   2. Minutes in 5-minute steps. Nobody schedules a gig for 19:07, and 60
//      individual minute rows is exactly the haystack that makes the native
//      control slow to use. An exact minute is still reachable by typing.
//   3. Presets first. Most events start on the hour in the evening, so the
//      common answers are one tap away above the columns.
function TimeField({
  selected, invalid, onPick, disabled = false, onClear, wrapStyle, align = 'right',
}: {
  selected: Date | null;
  invalid: boolean;
  onPick: (hours: number, minutes: number) => void;
  /** Read-only. Used by review-locked information fields on a live event. */
  disabled?: boolean;
  /** When given, the popover offers Clear. A standalone time may be blank;
   *  the time half of a date+time pair may not, which is why DateTimePicker
   *  does not pass this and keeps its own Clear on the date side. */
  onClear?: () => void;
  /** Overrides the fixed 118px column so TimePicker can fill its .field. */
  wrapStyle?: CSSProperties;
  /**
   * Which edge of the trigger the popover hangs from.
   *
   * 'right' suits the narrow 118px column beside the date button, where the
   * popover is wider than its trigger. A full-width TimePicker is the other
   * way round: right-aligning there parks a 244px popover at the far end of a
   * 900px field, nowhere near the value it belongs to.
   */
  align?: 'left' | 'right';
}) {
  const [open, setOpen] = useState(false);
  const boxRef = useRef<HTMLDivElement | null>(null);
  const hourColRef = useRef<HTMLDivElement | null>(null);
  const placement = usePlacement(open, boxRef);

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

  // Scroll the current hour into view when the popover opens, so an 8pm event
  // doesn't open showing midnight and require a scroll to find itself.
  useEffect(() => {
    if (!open || !hourColRef.current) return;
    const el = hourColRef.current.querySelector('[data-on="1"]') as HTMLElement | null;
    if (el) hourColRef.current.scrollTop = el.offsetTop - 60;
  }, [open]);

  const h24 = selected ? selected.getHours() : 19;
  const mins = selected ? selected.getMinutes() : 0;
  const isPM = h24 >= 12;
  const h12 = h24 % 12 === 0 ? 12 : h24 % 12;

  const label = selected
    ? `${h12}:${pad(mins)} ${isPM ? 'PM' : 'AM'}`
    : 'Time';

  /** 12-hour + meridiem back to a 24-hour hour. */
  const to24 = (hour12: number, pm: boolean) => {
    if (hour12 === 12) return pm ? 12 : 0;
    return pm ? hour12 + 12 : hour12;
  };

  const PRESETS: Array<[string, number, number]> = [
    ['09:00', 9, 0], ['12:00', 12, 0], ['14:00', 14, 0],
    ['18:00', 18, 0], ['19:00', 19, 0], ['20:00', 20, 0],
    ['21:00', 21, 0], ['22:00', 22, 0],
  ];

  const colStyle: CSSProperties = {
    maxHeight: 168, overflowY: 'auto', display: 'flex',
    flexDirection: 'column', gap: 2, paddingRight: 4,
  };

  const cell = (on: boolean): CSSProperties => ({
    padding: '7px 0', border: 'none', borderRadius: 8,
    background: on ? 'var(--accent-deep)' : 'transparent',
    color: on ? 'var(--on-accent)' : 'var(--text-2)',
    fontSize: 13, fontWeight: on ? 700 : 500,
    fontFamily: 'inherit', cursor: 'pointer',
    fontVariantNumeric: 'tabular-nums',
  });

  const colHead: CSSProperties = {
    fontSize: 9.5, fontWeight: 800, letterSpacing: '.1em',
    textTransform: 'uppercase', color: 'var(--text-3)',
    textAlign: 'center', marginBottom: 6,
  };

  return (
    <div ref={boxRef} style={{ position: 'relative', width: 118, flexShrink: 0, ...wrapStyle }}>
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        disabled={disabled}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label="Time"
        style={{
          width: '100%', padding: '9px 12px',
          border: `1px solid ${invalid ? 'var(--danger)' : 'var(--border-2)'}`,
          borderRadius: 'var(--r)',
          background: 'var(--surface-alpha)',
          color: selected ? 'var(--text)' : 'var(--text-faint)',
          fontSize: 14, fontFamily: 'inherit',
          cursor: disabled ? 'not-allowed' : 'pointer',
          opacity: disabled ? 0.6 : 1,
          display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 6,
          fontVariantNumeric: 'tabular-nums',
        }}
      >
        <span>{label}</span>
        <Clock size={15} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
      </button>

      {open && (
        <div
          role="dialog"
          aria-label="Choose time"
          style={{
            position: 'absolute', zIndex: 60,
            ...(placement.up
              ? { bottom: `calc(100% + ${POPOVER_GAP}px)` }
              : { top: `calc(100% + ${POPOVER_GAP}px)` }),
            ...(align === 'left' ? { left: 0 } : { right: 0 }),
            background: 'var(--bg-elevated)', border: '1px solid var(--border-2)',
            borderRadius: 'var(--r-lg)', boxShadow: 'var(--shadow-pop)',
            padding: 12, width: 244,
            ...(placement.maxHeight
              ? { maxHeight: placement.maxHeight, overflowY: 'auto' as const }
              : {}),
          }}
        >
          <div style={colHead}>Common times</div>
          <div style={{
            display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 5,
            marginBottom: 12, paddingBottom: 12, borderBottom: '1px solid var(--border)',
          }}>
            {PRESETS.map(([lbl, ph, pm]) => {
              const on = h24 === ph && mins === pm;
              return (
                <button key={lbl} type="button"
                  onClick={() => { onPick(ph, pm); setOpen(false); }}
                  style={{
                    padding: '6px 0', borderRadius: 7, fontSize: 11.5, fontWeight: 600,
                    border: `1px solid ${on ? 'var(--accent)' : 'var(--border)'}`,
                    background: on ? 'var(--accent-soft)' : 'var(--surface-alpha)',
                    color: on ? 'var(--accent)' : 'var(--text-2)',
                    cursor: 'pointer', fontFamily: 'inherit',
                    fontVariantNumeric: 'tabular-nums',
                  }}>
                  {lbl}
                </button>
              );
            })}
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 58px', gap: 8 }}>
            {/* Hour */}
            <div>
              <div style={colHead}>Hour</div>
              <div ref={hourColRef} style={colStyle}>
                {Array.from({ length: 12 }, (_, i) => i + 1).map(hh => {
                  const on = hh === h12;
                  return (
                    <button key={hh} type="button" data-on={on ? '1' : '0'}
                      onClick={() => onPick(to24(hh, isPM), mins)}
                      style={cell(on)}>
                      {hh}
                    </button>
                  );
                })}
              </div>
            </div>

            {/* Minute */}
            <div>
              <div style={colHead}>Min</div>
              <div style={colStyle}>
                {Array.from({ length: 12 }, (_, i) => i * 5).map(mm => {
                  const on = mm === mins;
                  return (
                    <button key={mm} type="button"
                      onClick={() => onPick(h24, mm)}
                      style={cell(on)}>
                      {pad(mm)}
                    </button>
                  );
                })}
              </div>
            </div>

            {/* Meridiem */}
            <div>
              <div style={colHead}>&nbsp;</div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                {([['AM', false], ['PM', true]] as const).map(([lbl, pm]) => {
                  const on = isPM === pm;
                  return (
                    <button key={lbl} type="button"
                      onClick={() => onPick(to24(h12, pm), mins)}
                      style={{ ...cell(on), padding: '9px 0' }}>
                      {lbl}
                    </button>
                  );
                })}
              </div>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginTop: 12 }}>
            {onClear && (
              <button type="button" onClick={() => { onClear(); setOpen(false); }}
                style={{
                  background: 'none', border: 'none', cursor: 'pointer',
                  color: 'var(--text-3)', fontSize: 12.5, textDecoration: 'underline',
                  fontFamily: 'inherit', padding: 0, flexShrink: 0,
                }}>
                Clear
              </button>
            )}
            <button type="button" onClick={() => setOpen(false)}
              style={{
                flex: 1, padding: '7px 0',
                border: 'none', borderRadius: 'var(--r)',
                background: 'var(--accent-deep)', color: 'var(--on-accent)',
                fontSize: 13, fontWeight: 700, cursor: 'pointer', fontFamily: 'inherit',
              }}>
              Done
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
// ── TimePicker ───────────────────────────────────────────────────────────────
// The TIME half of DateTimePicker, on its own, for the fields that are a
// wall-clock time with no date of their own: "Doors open" in the information
// catalogue, and a line-up act's set time.
//
// Both of those were bare <input type="time"> — the browser's two unlabelled
// scrolling columns, which is exactly what TimeField was written to replace.
// So the same form carried the good picker on start/end time and Chrome's on
// every other time in it. This is that gap closed, not a new control: the
// popover below is the same component the event's start time opens.
//
// Contract matches the native input it replaces, so no caller had to change
// how it stores anything:
//   value / onChange: 'HH:mm', 24-hour, no date. '' means unset.
// A stored 'HH:mm:ss' still parses; it is normalised to 'HH:mm' on the next
// edit. Date arithmetic stays with the caller — LineupFields still decides
// which DAY a 01:00 closing set belongs to.
export function TimePicker({
  value, onChange, invalid = false, disabled = false, style,
}: {
  value: string;
  onChange: (next: string) => void;
  invalid?: boolean;
  disabled?: boolean;
  style?: CSSProperties;
}) {
  // A scratch Date that never leaves this component — TimeField is written
  // against one, and only its hours and minutes are read back out on emit.
  const selected = useMemo(() => {
    const m = /^(\d{1,2}):(\d{2})/.exec(value ?? '');
    if (!m) return null;
    const h = Number(m[1]);
    const mi = Number(m[2]);
    if (h > 23 || mi > 59) return null;
    const d = new Date();
    d.setHours(h, mi, 0, 0);
    return d;
  }, [value]);

  return (
    <TimeField
      selected={selected}
      invalid={invalid}
      disabled={disabled}
      onPick={(h, mi) => onChange(`${pad(h)}:${pad(mi)}`)}
      onClear={() => onChange('')}
      align="left"
      wrapStyle={{ width: '100%', flexShrink: 1, ...style }}
    />
  );
}