'use client';
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

// A native <select> popup is drawn by the OS, so its list can never match the
// trigger. This renders the list ourselves.
//
// The list goes in a PORTAL on document.body, positioned from the trigger's
// bounding rect. An absolutely-positioned list is clipped by any ancestor with
// overflow:hidden — .card has it, so the dropdown was being cut off inside the
// analytics toolbar. A portal cannot be clipped by an ancestor because it has
// none, and it sidesteps z-index stacking contexts at the same time.
//
// Keyboard behaviour matches a native select: arrows move, Enter/Space commit,
// Escape cancels, typing jumps to a match.

export interface SelectOption {
  value: string;
  label: string;
  /** Renders under the label — used for the "needs full verification" hints. */
  hint?: string;
  disabled?: boolean;
  group?: string;
}

export default function Select({
  value, options, onChange, placeholder = 'Select…', disabled = false,
  style, ariaLabel, fullWidth = true,
}: {
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  style?: React.CSSProperties;
  ariaLabel?: string;
  fullWidth?: boolean;
}) {
  const [open, setOpen] = useState(false);
  // -1 means "not moved yet": the highlight falls back to the selected option,
  // so opening never needs an effect to sync it.
  const [active, setActive] = useState(-1);
  const [query, setQuery] = useState('');
  const boxRef = useRef<HTMLDivElement | null>(null);
  const listRef = useRef<HTMLDivElement | null>(null);
  const queryTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [pos, setPos] = useState<{ top: number; left: number; width: number; flip: boolean } | null>(null);

  const measure = useCallback(() => {
    const el = boxRef.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const below = window.innerHeight - r.bottom;
    // Flip above only when there genuinely isn't room below AND there is more
    // room above — otherwise a trigger near the bottom of a short window would
    // flip into even less space.
    const flip = below < 240 && r.top > below;
    setPos({
      top: flip ? r.top - 6 : r.bottom + 6,
      left: r.left,
      width: r.width,
      flip,
    });
  }, []);

  useLayoutEffect(() => { if (open) measure(); }, [open, measure]);

  useEffect(() => {
    if (!open) return;
    // capture:true so scrolling an ancestor container moves the list too, not
    // just scrolling the window.
    const onMove = () => measure();
    window.addEventListener('scroll', onMove, true);
    window.addEventListener('resize', onMove);
    return () => {
      window.removeEventListener('scroll', onMove, true);
      window.removeEventListener('resize', onMove);
    };
  }, [open, measure]);

  const selected = options.find(o => o.value === value) ?? null;

  const grouped = useMemo(() => {
    const out: Array<{ group?: string; items: SelectOption[] }> = [];
    for (const o of options) {
      const last = out[out.length - 1];
      if (last && last.group === o.group) last.items.push(o);
      else out.push({ group: o.group, items: [o] });
    }
    return out;
  }, [options]);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      const t = e.target as Node;
      if (boxRef.current?.contains(t)) return;
      if (listRef.current?.contains(t)) return;
      setOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [open]);

  const selectedIndex = options.findIndex(o => o.value === value);
  // Open on the current value so a long list doesn't start at the top.
  const activeIndex = active >= 0 ? active : (selectedIndex >= 0 ? selectedIndex : 0);

  useEffect(() => {
    if (!open || !listRef.current) return;
    const el = listRef.current.querySelector(`[data-i="${activeIndex}"]`) as HTMLElement | null;
    el?.scrollIntoView({ block: 'nearest' });
  }, [activeIndex, open]);

  function commit(i: number) {
    const o = options[i];
    if (!o || o.disabled) return;
    onChange(o.value);
    setOpen(false);
    setActive(-1);
  }

  function step(delta: number) {
    let i = activeIndex;
    for (let n = 0; n < options.length; n++) {
      i = (i + delta + options.length) % options.length;
      if (!options[i].disabled) break;
    }
    setActive(i);
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (disabled) return;
    if (!open && (e.key === 'Enter' || e.key === ' ' || e.key === 'ArrowDown')) {
      e.preventDefault(); setOpen(true); return;
    }
    if (!open) return;

    if (e.key === 'Escape') { e.preventDefault(); setOpen(false); return; }
    if (e.key === 'ArrowDown') { e.preventDefault(); step(1); return; }
    if (e.key === 'ArrowUp') { e.preventDefault(); step(-1); return; }
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); commit(activeIndex); return; }
    if (e.key === 'Home') { e.preventDefault(); setActive(0); return; }
    if (e.key === 'End') { e.preventDefault(); setActive(options.length - 1); return; }

    // Type-ahead, reset after a pause like a native select.
    if (e.key.length === 1) {
      const next = query + e.key.toLowerCase();
      setQuery(next);
      if (queryTimer.current) clearTimeout(queryTimer.current);
      queryTimer.current = setTimeout(() => setQuery(''), 600);
      const i = options.findIndex(o => !o.disabled && o.label.toLowerCase().startsWith(next));
      if (i >= 0) setActive(i);
    }
  }

  return (
    <div
      ref={boxRef}
      style={{ position: 'relative', width: fullWidth ? '100%' : undefined }}
    >
      <button
        type="button"
        disabled={disabled}
        onClick={() => !disabled && setOpen(o => !o)}
        onKeyDown={onKeyDown}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={ariaLabel}
        style={{
          width: '100%', display: 'flex', alignItems: 'center', gap: 10,
          justifyContent: 'space-between',
          padding: '11px 13px',
          border: `1px solid ${open ? 'var(--accent)' : 'var(--border-2)'}`,
          borderRadius: 'var(--r)',
          background: 'var(--surface-alpha)',
          color: selected ? 'var(--text)' : 'var(--text-faint)',
          fontSize: 14, fontFamily: 'inherit', textAlign: 'left',
          cursor: disabled ? 'not-allowed' : 'pointer',
          opacity: disabled ? 0.6 : 1,
          boxShadow: open ? '0 0 0 3px var(--accent-ring)' : 'none',
          transition: 'border-color .15s ease, box-shadow .15s ease',
          ...style,
        }}
      >
        <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {selected ? selected.label : placeholder}
        </span>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
          strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"
          style={{
            flexShrink: 0, color: 'var(--text-3)',
            transform: open ? 'rotate(180deg)' : 'none', transition: 'transform .15s ease',
          }}>
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>

      {open && pos && createPortal(
        <div
          ref={listRef}
          role="listbox"
          style={{
            position: 'fixed',
            top: pos.flip ? undefined : pos.top,
            bottom: pos.flip ? window.innerHeight - pos.top : undefined,
            left: pos.left,
            width: pos.width,
            zIndex: 3000,
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-2)',
            borderRadius: 'var(--r-lg)',
            boxShadow: 'var(--shadow-pop)',
            padding: 5, maxHeight: 300, overflowY: 'auto',
          }}
        >
          {grouped.map((g, gi) => (
            <div key={gi}>
              {g.group && (
                <div style={{
                  fontSize: 10, fontWeight: 800, letterSpacing: '.1em',
                  textTransform: 'uppercase', color: 'var(--text-3)',
                  padding: '9px 10px 5px',
                }}>
                  {g.group}
                </div>
              )}
              {g.items.map((o) => {
                const i = options.indexOf(o);
                const on = o.value === value;
                const hot = i === activeIndex;
                return (
                  <div
                    key={o.value}
                    data-i={i}
                    role="option"
                    aria-selected={on}
                    onMouseEnter={() => !o.disabled && setActive(i)}
                    onClick={() => commit(i)}
                    style={{
                      display: 'flex', alignItems: 'flex-start', gap: 9,
                      padding: '9px 10px', borderRadius: 8,
                      cursor: o.disabled ? 'not-allowed' : 'pointer',
                      background: hot && !o.disabled ? 'var(--surface-strong)' : 'transparent',
                      color: o.disabled ? 'var(--text-faint)'
                        : on ? 'var(--accent)' : 'var(--text)',
                      opacity: o.disabled ? 0.55 : 1,
                    }}
                  >
                    <span style={{
                      width: 14, flexShrink: 0, marginTop: 2,
                      color: on ? 'var(--accent)' : 'transparent',
                    }}>
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                        strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M20 6L9 17l-5-5" />
                      </svg>
                    </span>
                    <span style={{ minWidth: 0 }}>
                      <div style={{ fontSize: 13.5, fontWeight: on ? 700 : 500 }}>{o.label}</div>
                      {o.hint && (
                        <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 1 }}>
                          {o.hint}
                        </div>
                      )}
                    </span>
                  </div>
                );
              })}
            </div>
          ))}
        </div>,
        document.body,
      )}
    </div>
  );
}