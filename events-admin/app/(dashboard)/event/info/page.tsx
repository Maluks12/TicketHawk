'use client';
import { useEffect, useState, useMemo } from 'react';
import { api } from '../../../../lib/api';
import { useToast } from '../../../../lib/toast';
import { useConfirm } from '../../../../lib/confirm';
import { useStaff, AccessDenied } from '../../../../lib/permissions';

/**
 * Event information catalogue.
 *
 * The fields organisers can choose from when filling in an event's Information
 * block. Adding, relabelling, reordering and retiring them used to require SQL.
 *
 * Two things this screen is careful about, because both silently break
 * organiser forms:
 *
 *  - Retiring is is_active = false, never a delete. Every event already using
 *    a retired field keeps its value and keeps rendering it. The usage count
 *    beside each row is what turns "retire this" from a guess into a decision.
 *
 *  - Narrowing an enum is refused by the API when events still hold a value
 *    being removed. Dropping "16+" from age_requirement would leave those
 *    organisers unable to save their own form, rejected on a field they never
 *    touched. The error names the orphaned values.
 */

/**
 * Three keys the apps render as SUMMARY TILES rather than ordinary rows: they
 * appear together at the top of the Information block, and a field the
 * organiser skipped shows N/A instead of vanishing.
 *
 * That makes retiring one of them different from retiring anything else, and
 * this screen has to say so. Retiring is normally safe — the field stops being
 * offered and existing values keep rendering. Retire a summary key and the tile
 * does NOT disappear: it keeps its slot reading N/A on every event, and no
 * organiser can fill it because it is gone from the picker.
 *
 * Hardcoded because the clients hardcode it. If that ever moves to a catalogue
 * column, this list should read from the API instead of duplicating it.
 */
const SUMMARY_TILE_KEYS = ['age_requirement', 'doors_open', 'id_required'];

type ValueType = 'text' | 'enum' | 'time';

interface CatalogueField {
  key: string;
  label: string;
  hint?: string;
  value_type: ValueType;
  enum_options?: string[];
  display_order: number;
  review_required_when_published: boolean;
  is_active: boolean;
}

const EMPTY_FORM = {
  key: '',
  label: '',
  hint: '',
  value_type: 'text' as ValueType,
  enum_options: '',
  display_order: '500',
  review_required_when_published: false,
};

export default function EventInfoCataloguePage() {
  const toast = useToast();
  const confirm = useConfirm();

  // Read at event.view, written at event.edit — matching how merch and
  // collections are gated. Rendering the edit controls to a read_only user
  // would give them buttons that each produce a 403, which reads as a broken
  // console rather than a permission boundary.
  const capsLoaded = useStaff(st => st.loaded);
  const can = useStaff(st => st.can);
  const loadCaps = useStaff(st => st.load);
  useEffect(() => { loadCaps(); }, [loadCaps]);
  const canEdit = can('event.edit');

  const [fields, setFields] = useState<CatalogueField[]>([]);
  const [usage, setUsage] = useState<Record<string, number>>({});
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<'all' | 'active' | 'retired'>('all');
  const [search, setSearch] = useState('');

  const [formModal, setFormModal] = useState(false);
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [form, setForm] = useState({ ...EMPTY_FORM });
  const [submitting, setSubmitting] = useState(false);
  const [busyKey, setBusyKey] = useState<string | null>(null);

  useEffect(() => { load(); }, []);

  async function load() {
    setLoading(true);
    try {
      const { data } = await api.get<{ catalogue: CatalogueField[]; usage: Record<string, number> }>(
        '/admin/event-info/catalogue'
      );
      setFields(data.catalogue || []);
      setUsage(data.usage || {});
    } catch { toast('Failed to load catalogue', 'error'); }
    finally { setLoading(false); }
  }

  const filtered = useMemo(() => {
    const q = search.toLowerCase();
    return fields.filter(f => {
      if (filter === 'active' && !f.is_active) return false;
      if (filter === 'retired' && f.is_active) return false;
      if (!q) return true;
      return f.key.includes(q) || f.label.toLowerCase().includes(q);
    });
  }, [fields, filter, search]);

  const activeCount = fields.filter(f => f.is_active).length;

  function setF(patch: Partial<typeof EMPTY_FORM>) { setForm(p => ({ ...p, ...patch })); }

  function openCreate() {
    setEditingKey(null);
    setForm({ ...EMPTY_FORM });
    setFormModal(true);
  }

  function openEdit(f: CatalogueField) {
    setEditingKey(f.key);
    setForm({
      key: f.key,
      label: f.label,
      hint: f.hint || '',
      value_type: f.value_type,
      enum_options: (f.enum_options || []).join('\n'),
      display_order: String(f.display_order),
      review_required_when_published: f.review_required_when_published,
    });
    setFormModal(true);
  }

  async function submitForm() {
    if (!form.label.trim()) { toast('Label is required', 'error'); return; }

    const options = form.enum_options.split('\n').map(s => s.trim()).filter(Boolean);
    if (form.value_type === 'enum' && options.length < 2) {
      toast('An enum field needs at least two options', 'error');
      return;
    }

    setSubmitting(true);
    try {
      if (editingKey) {
        // key and value_type are absent on purpose — neither is editable.
        // Renaming a key would orphan every event using it; changing a type
        // would leave existing values failing validation in forms nobody
        // touched.
        await api.patch(`/admin/event-info/catalogue/${editingKey}`, {
          label: form.label.trim(),
          hint: form.hint.trim(),
          display_order: parseInt(form.display_order) || 500,
          review_required_when_published: form.review_required_when_published,
          ...(form.value_type === 'enum' ? { enum_options: options } : {}),
        });
        toast('Field updated', 'success');
      } else {
        await api.post('/admin/event-info/catalogue', {
          key: form.key.trim().toLowerCase(),
          label: form.label.trim(),
          hint: form.hint.trim() || undefined,
          value_type: form.value_type,
          enum_options: form.value_type === 'enum' ? options : undefined,
          display_order: parseInt(form.display_order) || 500,
          review_required_when_published: form.review_required_when_published,
        });
        toast('Field created', 'success');
      }
      setFormModal(false);
      load();
    } catch (e: any) {
      // The API returns actionable errors here — orphaned enum values by name,
      // a reserved key, a duplicate. Surfacing the server message rather than
      // a generic one is the difference between an admin fixing it and an
      // admin filing a bug.
      toast(e?.response?.data?.error || 'Failed to save field', 'error');
    } finally { setSubmitting(false); }
  }

  // confirm(title, msg, cb) — callback-based, not promise-returning.
  function retire(f: CatalogueField) {
    const used = usage[f.key] || 0;
    const isSummary = SUMMARY_TILE_KEYS.includes(f.key);
    const base = used > 0
      ? `${used} event${used === 1 ? '' : 's'} already use this field. They keep their values and keep showing them — it just stops appearing for new events. Nothing is deleted.`
      : 'It will stop appearing when organisers add information to an event. Nothing is deleted.';
    confirm(
      `Retire "${f.label}"`,
      // Leads with the consequence, because for these three the usual
      // reassurance above is the wrong mental model.
      isSummary
        ? `This is one of the three summary tiles on the event page. Retiring it does NOT remove the tile — it will read N/A on every event from now on, and no organiser will be able to fill it in. ${base}`
        : base,
      async () => {
        try {
          await api.put(`/admin/event-info/catalogue/${f.key}/active`, { is_active: false });
          toast('Field retired', 'success');
          load();
        } catch (e: any) {
          toast(e?.response?.data?.error || 'Failed to retire field', 'error');
        }
      }
    );
  }

  // No confirmation — restoring is additive and trivially undone by retiring
  // again, so a modal would be friction with nothing behind it.
  async function restore(f: CatalogueField) {
    setBusyKey(f.key);
    try {
      await api.put(`/admin/event-info/catalogue/${f.key}/active`, { is_active: true });
      toast('Field restored', 'success');
      load();
    } catch (e: any) {
      toast(e?.response?.data?.error || 'Failed to restore field', 'error');
    } finally { setBusyKey(null); }
  }

  async function move(f: CatalogueField, direction: -1 | 1) {
    const ordered = [...fields].sort((a, b) => a.display_order - b.display_order);
    const idx = ordered.findIndex(x => x.key === f.key);
    const target = idx + direction;
    if (target < 0 || target >= ordered.length) return;

    [ordered[idx], ordered[target]] = [ordered[target], ordered[idx]];

    setBusyKey(f.key);
    try {
      // Whole list in one call. Reordering by swap moves two fields, and
      // applying that as two requests would leave the catalogue visibly
      // half-sorted if one failed.
      await api.put('/admin/event-info/catalogue/order', { keys: ordered.map(x => x.key) });
      setFields(ordered.map((x, i) => ({ ...x, display_order: (i + 1) * 10 })));
    } catch { toast('Failed to reorder', 'error'); }
    finally { setBusyKey(null); }
  }

  if (!capsLoaded) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }
  if (!can('event.view')) return <AccessDenied need="event.view" />;

  return (
    <div style={{ padding: '1.5rem', maxWidth: '1200px' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600 }}>Event Info Fields</div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: '.15rem', maxWidth: '600px' }}>
            What organisers can fill in on an event&apos;s Information block. Fields left blank never appear on the event page.
          </div>
        </div>
        {canEdit && <button className="btn btn-primary btn-sm" onClick={openCreate}>+ New Field</button>}
      </div>

      <div style={{ display: 'flex', gap: '1rem', marginBottom: '1rem', flexWrap: 'wrap' }}>
        {[
          { label: 'Total', value: fields.length, accent: false },
          { label: 'Active', value: activeCount, accent: true },
          { label: 'Retired', value: fields.length - activeCount, accent: false },
        ].map(s => (
          <div key={s.label} style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', padding: '.6rem 1rem', minWidth: '90px' }}>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.3rem', fontWeight: 600, color: s.accent ? 'var(--accent)' : 'var(--text-1)' }}>{s.value}</div>
            <div style={{ fontSize: '.72rem', color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '.06em' }}>{s.label}</div>
          </div>
        ))}
      </div>

      <div className="table-toolbar" style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', marginBottom: '1rem', flexWrap: 'wrap' }}>
        <input className="filter-select" type="text" placeholder="Search key or label…" value={search} onChange={e => setSearch(e.target.value)} style={{ minWidth: '180px' }} />
        {(['all', 'active', 'retired'] as const).map(f => (
          <button key={f} className={`btn btn-sm${filter === f ? ' btn-primary' : ''}`} onClick={() => setFilter(f)}>
            {f.charAt(0).toUpperCase() + f.slice(1)}
          </button>
        ))}
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: '2rem' }}><div className="spinner" style={{ margin: 'auto' }} /></div>
      ) : filtered.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-3)', fontSize: '.9rem' }}>
          {fields.length === 0 ? 'No fields yet.' : 'No fields match your filter.'}
        </div>
      ) : (
        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', overflow: 'hidden' }}>
          <table className="table" style={{ width: '100%' }}>
            <thead>
              <tr>
                {canEdit && <th style={{ width: '48px' }} />}
                <th>Field</th>
                <th>Hint</th>
                <th style={{ textAlign: 'center' }}>Type</th>
                <th style={{ textAlign: 'center' }}>Used by</th>
                <th style={{ textAlign: 'center' }}>Status</th>
                <th style={{ textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map(f => (
                <tr key={f.key} style={f.is_active ? undefined : { opacity: .55 }}>
                  {canEdit && (
                    <td>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                        <button className="btn btn-sm" style={{ padding: '0 5px', fontSize: '.7rem', lineHeight: 1.4 }} disabled={busyKey === f.key} onClick={() => move(f, -1)}>↑</button>
                        <button className="btn btn-sm" style={{ padding: '0 5px', fontSize: '.7rem', lineHeight: 1.4 }} disabled={busyKey === f.key} onClick={() => move(f, 1)}>↓</button>
                      </div>
                    </td>
                  )}

                  <td>
                    <div style={{ fontWeight: 500, fontSize: '.87rem' }}>{f.label}</div>
                    <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.72rem', color: 'var(--text-3)', marginTop: '.1rem' }}>{f.key}</div>
                    {f.value_type === 'enum' && f.enum_options && (
                      <div style={{ fontSize: '.72rem', color: 'var(--text-3)', marginTop: '.2rem' }}>{f.enum_options.join(' · ')}</div>
                    )}
                    {SUMMARY_TILE_KEYS.includes(f.key) && (
                      <div
                        style={{ fontSize: '.68rem', color: 'var(--accent)', marginTop: '.2rem' }}
                        title="Shown as a tile at the top of the event page's Information block, alongside the other two. Reads N/A if the organiser leaves it blank."
                      >
                        summary tile
                      </div>
                    )}
                  </td>

                  <td style={{ fontSize: '.78rem', color: 'var(--text-2)', maxWidth: '280px' }}>{f.hint || '—'}</td>

                  <td style={{ textAlign: 'center' }}>
                    <span className="badge">{f.value_type}</span>
                    {f.review_required_when_published && (
                      <div
                        style={{ fontSize: '.68rem', color: 'var(--accent)', marginTop: '.25rem', whiteSpace: 'nowrap' }}
                        title="Organisers cannot change this on a live event without admin review"
                      >
                        review required
                      </div>
                    )}
                  </td>

                  <td style={{ textAlign: 'center', fontFamily: 'var(--font-mono)', fontSize: '.85rem' }}>
                    {usage[f.key] || 0}
                  </td>

                  <td style={{ textAlign: 'center' }}>
                    <span className={`badge ${f.is_active ? 'badge-active' : 'badge-inactive'}`}>
                      {f.is_active ? 'active' : 'retired'}
                    </span>
                  </td>

                  <td style={{ textAlign: 'right' }}>
                    {canEdit && (
                      <div style={{ display: 'inline-flex', gap: '.35rem' }}>
                        <button className="btn btn-sm" onClick={() => openEdit(f)}>✎ Edit</button>
                        {f.is_active
                          ? <button className="btn btn-sm btn-danger" onClick={() => retire(f)}>Retire</button>
                          : <button className="btn btn-sm" disabled={busyKey === f.key} onClick={() => restore(f)}>Restore</button>}
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Create / Edit Modal */}
      {formModal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setFormModal(false); }}>
          <div className="modal modal-sm">
            <div className="modal-header">
              <div className="modal-title">{editingKey ? `Edit "${form.label}"` : 'New Information Field'}</div>
              <button className="modal-close" onClick={() => setFormModal(false)}>✕</button>
            </div>

            <div className="modal-body">
              <div className="field">
                <label>Key *</label>
                <input
                  type="text"
                  maxLength={48}
                  value={form.key}
                  disabled={!!editingKey}
                  onChange={e => setF({ key: e.target.value })}
                  placeholder="e.g. shuttle_service"
                />
                <small style={{ color: 'var(--text-3)', fontSize: '.74rem' }}>
                  {editingKey
                    ? 'Keys cannot be changed — every event using this field points at it.'
                    : 'Lowercase letters, digits and underscores. Permanent once created.'}
                </small>
              </div>

              <div className="field">
                <label>Label *</label>
                <input type="text" maxLength={80} value={form.label} onChange={e => setF({ label: e.target.value })} placeholder="e.g. Shuttle service" />
              </div>

              <div className="field">
                <label>Hint <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>(optional)</small></label>
                <textarea rows={2} maxLength={240} value={form.hint} onChange={e => setF({ hint: e.target.value })} placeholder="Shown under the field in the organiser form" />
                <small style={{ color: 'var(--text-3)', fontSize: '.74rem' }}>
                  A good hint asks the question. &quot;Where should people park, and does it cost anything?&quot; gets a better answer than &quot;Parking&quot;.
                </small>
              </div>

              <div className="field">
                <label>Value type *</label>
                <select value={form.value_type} disabled={!!editingKey} onChange={e => setF({ value_type: e.target.value as ValueType })}>
                  <option value="text">Text — free prose</option>
                  <option value="enum">Enum — one of a fixed list</option>
                  <option value="time">Time — HH:MM</option>
                </select>
                {editingKey && (
                  <small style={{ color: 'var(--text-3)', fontSize: '.74rem' }}>
                    Type cannot be changed. Existing values would stop validating and organisers could not save forms they had not touched.
                  </small>
                )}
              </div>

              {form.value_type === 'enum' && (
                <div className="field">
                  <label>Options — one per line *</label>
                  <textarea rows={5} value={form.enum_options} onChange={e => setF({ enum_options: e.target.value })} placeholder={'All ages\n16+\n18+'} style={{ fontFamily: 'var(--font-mono)', fontSize: '.82rem' }} />
                  <small style={{ color: 'var(--text-3)', fontSize: '.74rem' }}>
                    Adding options is always safe. Removing one that events already use will be refused — those organisers could no longer save their own form.
                  </small>
                </div>
              )}

              <div className="field">
                <label>Display Order</label>
                <input type="number" min="0" value={form.display_order} onChange={e => setF({ display_order: e.target.value })} />
                <small style={{ color: 'var(--text-3)', fontSize: '.74rem' }}>Lower = shown first in the Information block</small>
              </div>

              <div className="field">
                <label style={{ display: 'flex', alignItems: 'flex-start', gap: '.5rem', cursor: 'pointer', fontWeight: 400 }}>
                  <input
                    type="checkbox"
                    checked={form.review_required_when_published}
                    onChange={e => setF({ review_required_when_published: e.target.checked })}
                    style={{ width: 'auto', marginTop: '.15rem' }}
                  />
                  <span>
                    Require review on live events
                    <small style={{ display: 'block', color: 'var(--text-3)', fontSize: '.74rem', marginTop: '.2rem' }}>
                      Organisers cannot change this after tickets go on sale without admin approval. Use it only where a change alters
                      what a buyer already agreed to — the age requirement is the one field that qualifies today. Turning it on for
                      something like parking would push last-minute corrections into a queue and leave the page wrong on the day.
                    </small>
                  </span>
                </label>
              </div>
            </div>

            <div className="modal-footer">
              <button className="btn" onClick={() => setFormModal(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={submitForm} disabled={submitting}>
                {submitting ? 'Saving…' : editingKey ? 'Save Changes' : 'Create Field'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}