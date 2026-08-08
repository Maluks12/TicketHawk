'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Ticket type modal
//
// Adding a tier used to append a blank inline row you filled in place, which
// had two problems: a half-filled row was already part of the form (the submit
// handler filtered on `t.name`, so an abandoned row vanished silently rather
// than telling you), and four rows of inline fields buried the rest of the
// section.
//
// Now a tier is composed in a dialog and only joins the list once it validates.
// Cancel discards it. The same dialog edits an existing tier, so there is one
// place where a tier's rules live rather than two that can drift.
// ─────────────────────────────────────────────────────────────────────────────
import { useEffect, useRef, useState } from 'react';
import { X, Warning, Ticket, Check } from '@phosphor-icons/react';

export interface TicketTypeRow {
  name: string; price: number; capacity: string;
  sort_order: number; description: string; is_visible: boolean;
}

// ── Preset names ─────────────────────────────────────────────────────────────
// Grouped so the list is scannable rather than 25 flat options. These are the
// tiers that actually appear on South African and international event
// listings — Golden Circle in particular is standard at SA stadium concerts and
// its absence would send organisers straight to "Custom".
const PRESET_GROUPS: Array<{ group: string; options: string[] }> = [
  {
    group: 'Release phases',
    options: [
      'Early Bird',
      'Super Early Bird',
      'Presale',
      'Phase 1',
      'Phase 2',
      'Phase 3',
      'Final Release',
      'Door Sales',
    ],
  },
  {
    group: 'Standard admission',
    options: [
      'General Admission',
      'Standing',
      'Seated',
      'Floor',
      'Balcony',
      'Grandstand',
      'Golden Circle',
      'Pitside',
    ],
  },
  {
    group: 'Premium',
    options: [
      'VIP',
      'VVIP',
      'Platinum',
      'Gold',
      'Silver',
      'Backstage Pass',
      'Meet & Greet',
      'Hospitality',
      'Corporate Suite',
    ],
  },
  {
    group: 'Groups & tables',
    options: [
      'Table (2)',
      'Table (4)',
      'Table (6)',
      'Table (8)',
      'Table (10)',
      'Booth',
      'Group Booking',
      'Bundle',
    ],
  },
  {
    group: 'Passes',
    options: [
      'Day Pass',
      'Weekend Pass',
      'Season Pass',
      'Multi-Day Pass',
    ],
  },
  {
    group: 'Concessions',
    options: [
      'Student',
      'Child',
      'Senior',
      'Pensioner',
      'Complimentary',
      'Guest List',
    ],
  },
];

const CUSTOM = '__custom__';

const ALL_PRESETS = PRESET_GROUPS.flatMap(g => g.options);

// ── Validation ───────────────────────────────────────────────────────────────

export interface TicketErrors {
  name?: string;
  price?: string;
  capacity?: string;
  sort_order?: string;
}

/**
 * Mirrors what the backend will accept. Returning a field-keyed object rather
 * than a single string lets the dialog mark the offending input instead of
 * making the organiser guess which of five fields the toast meant.
 */
export function validateTicket(
  t: TicketTypeRow,
  existingNames: string[],
): TicketErrors {
  const e: TicketErrors = {};

  const name = t.name.trim();
  if (!name) e.name = 'Give this tier a name';
  else if (name.length < 2) e.name = 'Name is too short';
  else if (name.length > 60) e.name = 'Keep the name under 60 characters';
  else if (existingNames.some(n => n.toLowerCase() === name.toLowerCase())) {
    // Two tiers with the same name are indistinguishable at checkout, and the
    // buyer has no way to tell which one they bought.
    e.name = 'You already have a tier with this name';
  }

  if (Number.isNaN(t.price)) e.price = 'Enter a number';
  else if (t.price < 0) e.price = 'Price cannot be negative';
  else if (t.price > 1_000_000) e.price = 'That price looks wrong';

  if (t.capacity.trim() !== '') {
    const c = Number(t.capacity);
    if (!Number.isInteger(c) || c < 1) e.capacity = 'Capacity must be a whole number above 0';
  }

  if (!Number.isInteger(t.sort_order)) e.sort_order = 'Must be a whole number';

  return e;
}

// ── Modal ────────────────────────────────────────────────────────────────────

interface ModalProps {
  open: boolean;
  /** Editing an existing tier, or null when adding a new one. */
  initial: TicketTypeRow | null;
  /** Sort order to pre-fill for a new tier. */
  index: number;
  /** Names already in use, EXCLUDING the tier being edited. */
  existingNames: string[];
  onCancel: () => void;
  onConfirm: (t: TicketTypeRow) => void;
}

export default function TicketTypeModal(props: ModalProps) {
  // Unmounted entirely when closed, and remounted with a fresh key each time it
  // opens. That is what resets the form — not an effect that syncs props into
  // state, which would both cascade an extra render and leave stale values
  // behind the moment a dependency was missed.
  if (!props.open) return null;
  return <TicketTypeForm key={props.initial ? `edit-${props.index}` : `add-${props.index}`} {...props} />;
}

function TicketTypeForm({
  initial, index, existingNames, onCancel, onConfirm,
}: ModalProps) {
  const [row, setRow] = useState<TicketTypeRow>(() => initial ?? {
    name: '', price: 0, capacity: '', sort_order: index, description: '', is_visible: true,
  });
  const [preset, setPreset] = useState<string>(() => {
    const n = initial?.name ?? '';
    if (n === '') return '';
    return ALL_PRESETS.includes(n) ? n : CUSTOM;
  });
  const [errors, setErrors] = useState<TicketErrors>({});
  // Errors only appear after a submit attempt. Marking fields red while someone
  // is still typing the first character is noise, not help.
  const [touched, setTouched] = useState(false);

  const firstRef = useRef<HTMLSelectElement | null>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onCancel(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onCancel]);

  useEffect(() => { firstRef.current?.focus(); }, []);

  const upd = (p: Partial<TicketTypeRow>) => {
    setRow(r => {
      const next = { ...r, ...p };
      if (touched) setErrors(validateTicket(next, existingNames));
      return next;
    });
  };

  function submit() {
    setTouched(true);
    const errs = validateTicket(row, existingNames);
    setErrors(errs);
    if (Object.keys(errs).length > 0) return;
    onConfirm({ ...row, name: row.name.trim(), description: row.description.trim() });
  }

  const err = (k: keyof TicketErrors) => (touched ? errors[k] : undefined);

  const inputStyle = (bad?: string): React.CSSProperties => ({
    width: '100%', padding: '11px 13px',
    border: `1px solid ${bad ? 'var(--danger)' : 'var(--border-2)'}`,
    borderRadius: 'var(--r)', fontSize: 14, color: 'var(--text)',
    background: 'var(--surface-alpha)', outline: 'none',
    fontFamily: 'inherit', boxSizing: 'border-box',
  });

  // Opaque variant for the <select>. Chrome paints the native dropdown list
  // using the select's computed background-color; a translucent one composites
  // over white, which is what turned the option list and its group headers
  // white in dark mode.
  const selectStyle = (bad?: string): React.CSSProperties => ({
    ...inputStyle(bad), background: 'var(--surface-2)',
  });

  return (
    <div
      className="modal-overlay"
      onMouseDown={e => { if (e.target === e.currentTarget) onCancel(); }}
    >
      <div className="modal" role="dialog" aria-modal="true" aria-label="Ticket type">
        <div className="modal-header">
          <div style={{ display: 'flex', alignItems: 'center', gap: 11 }}>
            <span style={{
              width: 32, height: 32, borderRadius: 10, flexShrink: 0,
              background: 'var(--accent-soft)', color: 'var(--accent)',
              display: 'grid', placeItems: 'center',
            }}>
              <Ticket size={17} weight="bold" />
            </span>
            <div>
              <div className="modal-title">
                {initial ? 'Edit ticket type' : 'Add ticket type'}
              </div>
              <div style={{ fontSize: 12.5, color: 'var(--text-3)', marginTop: 1 }}>
                A pricing tier buyers choose between at checkout
              </div>
            </div>
          </div>
          <button type="button" className="btn-icon" onClick={onCancel} aria-label="Close">
            <X size={16} />
          </button>
        </div>

        <div className="modal-body">
          {/* Name — preset or custom */}
          <div className="field">
            <label htmlFor="tt-preset">Ticket name <span style={{ color: 'var(--danger)' }}>*</span></label>
            <select
              id="tt-preset"
              ref={firstRef}
              value={preset}
              onChange={e => {
                const v = e.target.value;
                setPreset(v);
                // Switching to Custom clears the box so it's obviously waiting
                // for input rather than pre-filled with the last preset.
                upd({ name: v === CUSTOM ? '' : v === '' ? '' : v });
              }}
              style={selectStyle(err('name'))}
            >
              <option value="">— Choose a ticket type —</option>
              {PRESET_GROUPS.map(g => (
                <optgroup key={g.group} label={g.group}>
                  {g.options.map(o => <option key={o} value={o}>{o}</option>)}
                </optgroup>
              ))}
              <option value={CUSTOM}>Custom — type your own…</option>
            </select>

            {preset === CUSTOM && (
              <input
                type="text"
                value={row.name}
                onChange={e => upd({ name: e.target.value })}
                placeholder="e.g. Sunrise Session"
                maxLength={60}
                autoFocus
                style={{ ...inputStyle(err('name')), marginTop: 8 }}
              />
            )}

            {err('name')
              ? <FieldError msg={errors.name!} />
              : <div className="field-hint">Shown to buyers on the checkout screen</div>}
          </div>

          {/* Price / capacity / order */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 14 }}>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tt-price">Price</label>
              <div style={{ position: 'relative' }}>
                <span style={{
                  position: 'absolute', left: 13, top: '50%', transform: 'translateY(-50%)',
                  color: 'var(--text-3)', fontWeight: 700, fontSize: 14, pointerEvents: 'none',
                }}>R</span>
                <input
                  id="tt-price" type="number" min="0" step="0.01"
                  value={row.price}
                  onChange={e => upd({ price: parseFloat(e.target.value) || 0 })}
                  style={{ ...inputStyle(err('price')), paddingLeft: 30 }}
                />
              </div>
              {err('price')
                ? <FieldError msg={errors.price!} />
                : <div className="field-hint">0 = free</div>}
            </div>

            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tt-cap">Capacity</label>
              <input
                id="tt-cap" type="number" min="1"
                value={row.capacity}
                onChange={e => upd({ capacity: e.target.value })}
                placeholder="Unlimited"
                style={inputStyle(err('capacity'))}
              />
              {err('capacity')
                ? <FieldError msg={errors.capacity!} />
                : <div className="field-hint">Blank = unlimited</div>}
            </div>

            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tt-sort">Sort order</label>
              <input
                id="tt-sort" type="number"
                value={row.sort_order}
                onChange={e => upd({ sort_order: parseInt(e.target.value, 10) || 0 })}
                style={inputStyle(err('sort_order'))}
              />
              {err('sort_order')
                ? <FieldError msg={errors.sort_order!} />
                : <div className="field-hint">Lower shows first</div>}
            </div>
          </div>

          <div className="field" style={{ marginTop: 18, marginBottom: 0 }}>
            <label htmlFor="tt-desc">Description</label>
            <input
              id="tt-desc" type="text"
              value={row.description}
              onChange={e => upd({ description: e.target.value })}
              placeholder="Optional — what this tier includes"
              style={inputStyle()}
            />
          </div>

          <label className="check-row" style={{ marginTop: 16 }}>
            <input
              type="checkbox"
              checked={row.is_visible}
              onChange={e => upd({ is_visible: e.target.checked })}
            />
            <span style={{ fontSize: 13.5 }}>
              Visible to buyers
              <span style={{ color: 'var(--text-3)', marginLeft: 6, fontSize: 12.5 }}>
                — uncheck to stage a tier without releasing it
              </span>
            </span>
          </label>
        </div>

        <div className="modal-footer">
          <button type="button" className="btn" onClick={onCancel}>Cancel</button>
          <button type="button" className="btn btn-primary" onClick={submit}>
            <Check size={15} weight="bold" />
            {initial ? 'Save changes' : 'Add ticket type'}
          </button>
        </div>
      </div>
    </div>
  );
}

function FieldError({ msg }: { msg: string }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 5,
      fontSize: 12, color: 'var(--danger-fg)', marginTop: 5, fontWeight: 500,
    }}>
      <Warning size={13} weight="fill" />
      {msg}
    </div>
  );
}