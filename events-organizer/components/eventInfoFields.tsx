'use client';
import { useMemo, useState } from 'react';
import { Info, ShieldCheck, X } from '@phosphor-icons/react';
import type { InfoCatalogueEntry } from '../lib/eventContent';

/**
 * Information fields — CONTROLLED. Same split, same reason, as LineupFields:
 * the create wizard has no event id to load or save against.
 *
 * `chosen` is the ordered list of keys on this event, `values` maps key to
 * value. Two structures rather than one array of objects because order is
 * edited independently of content, and a single array would mean rebuilding it
 * on every keystroke.
 *
 * Every input is inside a `.field` wrapper — globals.css only styles
 * `.field input` / `select` / `textarea`, so a bare input renders unstyled.
 */
export default function EventInfoFields({
  catalogue,
  chosen,
  values,
  onChange,
  lockedKeys,
}: {
  catalogue: InfoCatalogueEntry[];
  chosen: string[];
  values: Record<string, string>;
  onChange: (next: { chosen: string[]; values: Record<string, string> }) => void;
  /**
   * Keys to show read-only. The event edit screen passes the review-locked ones
   * on a published event; the wizard passes none, because a draft has no buyers
   * and nothing to protect.
   */
  lockedKeys?: string[];
}) {
  const [adding, setAdding] = useState(false);

  const byKey = useMemo(() => {
    const m: Record<string, InfoCatalogueEntry> = {};
    catalogue.forEach(c => { m[c.key] = c; });
    return m;
  }, [catalogue]);

  const locked = (k: string) => !!lockedKeys?.includes(k);

  // Inactive entries are excluded from the picker — an admin retired them — but
  // a retired field the event ALREADY uses stays rendered, so nothing an
  // organiser wrote silently disappears.
  const available = catalogue.filter(c => c.is_active && !chosen.includes(c.key));

  function setValue(key: string, val: string) {
    onChange({ chosen, values: { ...values, [key]: val } });
  }

  function addField(key: string) {
    onChange({ chosen: [...chosen, key], values: { ...values, [key]: '' } });
    setAdding(false);
  }

  function removeField(key: string) {
    const next = { ...values };
    delete next[key];
    onChange({ chosen: chosen.filter(k => k !== key), values: next });
  }

  return (
    <div>
      <div className="alert alert-info" style={{ marginBottom: 18 }}>
        <Info size={16} weight="fill" />
        <div>
          These answer the questions attendees ask before buying. Anything you
          leave out simply does not appear on the event page — there are no
          empty headings.
        </div>
      </div>

      {chosen.length === 0 && (
        <div className="empty-state" style={{ padding: '26px 20px' }}>
          <div className="empty-desc">
            Nothing added yet. Attendees will see only your description, venue
            and times.
          </div>
        </div>
      )}

      {chosen.map(key => {
        const entry = byKey[key];
        if (!entry) return null;
        const isLocked = locked(key);

        return (
          <div key={key} className="field">
            <label style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span>{entry.label}</span>
              {isLocked && (
                <span
                  className="pill"
                  title="Changing this on a live event needs admin review"
                  style={{ color: 'var(--accent)' }}
                >
                  <ShieldCheck size={11} weight="bold" /> review required
                </span>
              )}
              {!isLocked && (
                <button
                  type="button"
                  className="btn btn-xs"
                  style={{ marginLeft: 'auto', padding: '2px 7px' }}
                  onClick={() => removeField(key)}
                  title="Remove this field"
                >
                  <X size={11} weight="bold" />
                </button>
              )}
            </label>

            {entry.value_type === 'enum' ? (
              <select value={values[key] ?? ''} disabled={isLocked}
                onChange={e => setValue(key, e.target.value)}>
                <option value="">Not specified</option>
                {entry.enum_options?.map(o => <option key={o} value={o}>{o}</option>)}
              </select>
            ) : entry.value_type === 'time' ? (
              <input type="time" value={values[key] ?? ''} disabled={isLocked}
                onChange={e => setValue(key, e.target.value)} />
            ) : (
              <textarea rows={2} maxLength={600} value={values[key] ?? ''} disabled={isLocked}
                placeholder={entry.hint} onChange={e => setValue(key, e.target.value)}
                style={{ minHeight: 62 }} />
            )}

            {entry.hint && entry.value_type !== 'text' && (
              <div className="field-hint">{entry.hint}</div>
            )}
            {isLocked && (
              <div className="field-hint">
                Tickets are on sale, so this can only change through an edit
                request an admin approves. Everything else here saves
                immediately.
              </div>
            )}
          </div>
        );
      })}

      {adding ? (
        <div style={{
          border: '1px solid var(--border)', borderRadius: 'var(--r-lg)',
          background: 'var(--surface-alpha)', padding: 12,
          maxHeight: 280, overflowY: 'auto',
        }}>
          {available.length === 0 ? (
            <div className="field-hint" style={{ padding: 6, marginTop: 0 }}>
              Every available field is already on this event.
            </div>
          ) : available.map(c => (
            <button key={c.key} type="button" onClick={() => addField(c.key)}
              style={{
                display: 'block', width: '100%', textAlign: 'left',
                background: 'none', border: 'none', cursor: 'pointer',
                padding: '10px 9px', borderRadius: 'var(--r)',
                color: 'var(--text)',
              }}>
              <div style={{ fontSize: 13.5, fontWeight: 600 }}>{c.label}</div>
              {c.hint && (
                <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 3 }}>{c.hint}</div>
              )}
            </button>
          ))}
          <button type="button" className="btn btn-sm" style={{ marginTop: 8 }}
            onClick={() => setAdding(false)}>Cancel</button>
        </div>
      ) : (
        <button type="button" className="btn btn-sm" onClick={() => setAdding(true)}>
          + Add information field
        </button>
      )}
    </div>
  );
}