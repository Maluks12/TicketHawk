'use client';
import { useEffect, useMemo, useState } from 'react';
import { Info, ShieldCheck, X } from '@phosphor-icons/react';
import { useToast } from '../lib/toast';
import {
  getInfoCatalogue, getEventInfo, replaceEventInfo,
  type InfoCatalogueEntry, type InfoItem,
} from '../lib/eventContent';

/**
 * Event information block.
 *
 * The organiser picks which fields to fill in. Ones they leave blank get NO
 * ROW server-side, which is what makes it impossible for the event page to
 * render a heading with nothing under it — the bug visible on competitors'
 * pages, where PARKING and PROHIBITED ITEMS appear as empty headings on events
 * where nobody answered.
 *
 * ── The review lock ─────────────────────────────────────────────────────────
 *
 * On a LIVE event, most fields save immediately: parking, gates, transport and
 * prohibited items all change in the days before an event, and routing "the
 * north gate is now closed" through an admin queue means the accurate version
 * arrives after the event.
 *
 * The exception is any field the catalogue marks
 * review_required_when_published — today only age_requirement. Someone who
 * bought a ticket to an "all ages" event and arrives with a 15-year-old at an
 * 18+ door has a refund dispute, and an organiser should not be able to create
 * that silently after the sale.
 *
 * Those fields are shown READ-ONLY with an explanation rather than hidden.
 * Hiding them would make the organiser think the information was lost.
 */
export default function EventInfoEditor({
  eventId,
  onDirtyChange,
}: {
  eventId: string;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const toast = useToast();

  const [catalogue, setCatalogue] = useState<InfoCatalogueEntry[]>([]);
  const [values, setValues] = useState<Record<string, string>>({});
  const [original, setOriginal] = useState<Record<string, string>>({});
  const [chosen, setChosen] = useState<string[]>([]);
  const [isPublished, setIsPublished] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [adding, setAdding] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [cat, info] = await Promise.all([
          getInfoCatalogue(),
          getEventInfo(eventId),
        ]);
        if (cancelled) return;
        setCatalogue(cat);
        setIsPublished(info.is_published);

        const v: Record<string, string> = {};
        info.items.forEach(i => { v[i.key] = i.value; });
        setValues(v);
        setOriginal(v);
        // Chosen order comes from the event's own sort_order, so an organiser
        // who arranged their block keeps that arrangement.
        setChosen(info.items.map(i => i.key));
      } catch {
        if (!cancelled) toast('Could not load event information', 'error');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [eventId, toast]);

  const byKey = useMemo(() => {
    const m: Record<string, InfoCatalogueEntry> = {};
    catalogue.forEach(c => { m[c.key] = c; });
    return m;
  }, [catalogue]);

  const dirty = useMemo(() => {
    const keys = new Set([...Object.keys(values), ...Object.keys(original)]);
    for (const k of keys) {
      if ((values[k] ?? '').trim() !== (original[k] ?? '').trim()) return true;
    }
    return false;
  }, [values, original]);

  useEffect(() => { onDirtyChange?.(dirty); }, [dirty, onDirtyChange]);

  // Fields not yet on this event, offered in the picker. Inactive catalogue
  // entries are excluded — an admin retired them — but a RETIRED field the
  // event already uses stays rendered below, so nothing silently vanishes.
  const available = catalogue.filter(c => c.is_active && !chosen.includes(c.key));

  function locked(key: string) {
    return isPublished && !!byKey[key]?.review_required_when_published;
  }

  function setValue(key: string, val: string) {
    setValues(v => ({ ...v, [key]: val }));
  }

  function addField(key: string) {
    setChosen(c => [...c, key]);
    setValues(v => ({ ...v, [key]: '' }));
    setAdding(false);
  }

  function removeField(key: string) {
    setChosen(c => c.filter(k => k !== key));
    setValues(v => { const n = { ...v }; delete n[key]; return n; });
  }

  async function save() {
    setSaving(true);
    try {
      // Send everything held. Blank values are dropped by the server, which is
      // the mechanism that keeps unchosen fields row-less.
      const items = chosen.map(key => ({ key, value: values[key] ?? '' }));
      const res = await replaceEventInfo(eventId, items);

      const v: Record<string, string> = {};
      res.items.forEach(i => { v[i.key] = i.value; });
      setValues(v);
      setOriginal(v);
      setChosen(res.items.map(i => i.key));
      toast('Information saved', 'success');
    } catch (e: unknown) {
      const err = e as { response?: { status?: number; data?: { error?: string; code?: string } } };
      if (err.response?.status === 409 && err.response.data?.code === 'review_required') {
        // Not a validation error. The organiser may make this change, just not
        // here — so say where, rather than showing a red field they cannot fix.
        toast(
          `${err.response.data.error ?? 'This field needs review'} — submit it as an edit request instead.`,
          'error',
        );
      } else {
        toast(err.response?.data?.error ?? 'Could not save information', 'error');
      }
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return <div style={{ padding: 24 }}><div className="spinner" /></div>;
  }

  return (
    <div>
      <div style={{
        display: 'flex', gap: 10, alignItems: 'flex-start',
        background: 'var(--accent-soft)', borderRadius: 'var(--r-lg)',
        padding: '12px 14px', marginBottom: 18,
      }}>
        <Info size={17} weight="bold" style={{ color: 'var(--accent)', flexShrink: 0, marginTop: 1 }} />
        <div style={{ fontSize: 12.5, color: 'var(--text-2)', lineHeight: 1.6 }}>
          These answer the questions attendees ask before buying. Anything you
          leave out simply does not appear on the event page — there are no
          empty headings.
          {isPublished && ' This event is live, so most changes here go out immediately.'}
        </div>
      </div>

      {chosen.length === 0 && (
        <div style={{
          textAlign: 'center', padding: '28px 20px',
          color: 'var(--text-3)', fontSize: 13,
        }}>
          No information added yet. Attendees will see only your description,
          venue and times.
        </div>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
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
                    title="Changing this on a live event needs admin review"
                    style={{
                      display: 'inline-flex', alignItems: 'center', gap: 4,
                      fontSize: 10.5, fontWeight: 700, letterSpacing: '.04em',
                      color: 'var(--accent)', textTransform: 'uppercase',
                    }}
                  >
                    <ShieldCheck size={11} weight="bold" /> review required
                  </span>
                )}
                {!isLocked && (
                  <button
                    type="button"
                    onClick={() => removeField(key)}
                    title="Remove this field"
                    style={{
                      marginLeft: 'auto', background: 'none', border: 'none',
                      cursor: 'pointer', color: 'var(--text-3)', padding: 2,
                      display: 'grid', placeItems: 'center',
                    }}
                  >
                    <X size={13} weight="bold" />
                  </button>
                )}
              </label>

              {entry.value_type === 'enum' ? (
                <select
                  value={values[key] ?? ''}
                  disabled={isLocked}
                  onChange={e => setValue(key, e.target.value)}
                >
                  <option value="">Not specified</option>
                  {entry.enum_options?.map(o => <option key={o} value={o}>{o}</option>)}
                </select>
              ) : entry.value_type === 'time' ? (
                <input
                  type="time"
                  value={values[key] ?? ''}
                  disabled={isLocked}
                  onChange={e => setValue(key, e.target.value)}
                />
              ) : (
                <textarea
                  rows={2}
                  maxLength={600}
                  value={values[key] ?? ''}
                  disabled={isLocked}
                  placeholder={entry.hint}
                  onChange={e => setValue(key, e.target.value)}
                />
              )}

              {entry.hint && entry.value_type !== 'text' && (
                <small className="field-hint">{entry.hint}</small>
              )}
              {isLocked && (
                <small className="field-hint">
                  Tickets are on sale, so this can only change through an edit
                  request an admin approves. Everything else here saves
                  immediately.
                </small>
              )}
            </div>
          );
        })}
      </div>

      {/* ── Add ─────────────────────────────────────────────────────────── */}
      <div style={{ marginTop: 18 }}>
        {adding ? (
          <div style={{
            border: '1px solid var(--border)', borderRadius: 'var(--r-lg)',
            padding: 12, maxHeight: 260, overflowY: 'auto',
          }}>
            {available.length === 0 ? (
              <div style={{ fontSize: 12.5, color: 'var(--text-3)', padding: 6 }}>
                Every available field is already on this event.
              </div>
            ) : available.map(c => (
              <button
                key={c.key}
                type="button"
                onClick={() => addField(c.key)}
                style={{
                  display: 'block', width: '100%', textAlign: 'left',
                  background: 'none', border: 'none', cursor: 'pointer',
                  padding: '9px 8px', borderRadius: 'var(--r-md)',
                }}
              >
                <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)' }}>{c.label}</div>
                {c.hint && (
                  <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 2 }}>{c.hint}</div>
                )}
              </button>
            ))}
            <button type="button" className="btn btn-sm" style={{ marginTop: 8 }}
              onClick={() => setAdding(false)}>Cancel</button>
          </div>
        ) : (
          <button type="button" className="btn btn-sm" onClick={() => setAdding(true)}>
            <span style={{ fontWeight: 700 }}>+</span> Add information field
          </button>
        )}
      </div>

      <div style={{ marginTop: 22, display: 'flex', gap: 10, alignItems: 'center' }}>
        <button
          type="button"
          className="btn btn-primary"
          disabled={!dirty || saving}
          onClick={save}
        >
          {saving ? 'Saving…' : 'Save information'}
        </button>
        {dirty && !saving && (
          <span style={{ fontSize: 12, color: 'var(--text-3)' }}>Unsaved changes</span>
        )}
      </div>
    </div>
  );
}