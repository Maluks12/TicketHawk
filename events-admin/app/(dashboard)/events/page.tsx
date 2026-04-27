'use client';
import { useEffect, useState, useRef, useCallback } from 'react';
import { api } from '../../../lib/api';
import { fmtMoney, fmtDate, toLocalInput, Badge } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';

interface EventRow { id: string; title: string; slug: string; status: string; start_time: string; price: number; currency: string; tickets_sold: number; max_capacity: number | null; ticket_revenue: number; city: string; }
interface TicketTypeRow { name: string; price: number; capacity: string; sort_order: number; description: string; is_visible: boolean; }

const TABS = ['Basic Info', 'Location', 'Ticket Types', 'Media & Policy'];

function TagInput({ tags, onChange }: { tags: string[]; onChange: (t: string[]) => void }) {
  const [val, setVal] = useState('');
  function add() {
    const v = val.trim().replace(/,$/, '');
    if (v && !tags.includes(v)) onChange([...tags, v]);
    setVal('');
  }
  return (
    <div className="tag-input-wrap" onClick={(e) => (e.currentTarget.querySelector('input') as HTMLInputElement)?.focus()}>
      {tags.map((t, i) => (
        <span key={i} className="tag-pill">{t} <button type="button" onClick={() => onChange(tags.filter((_, j) => j !== i))}>✕</button></span>
      ))}
      <input className="tag-bare-input" value={val} onChange={e => setVal(e.target.value)}
        placeholder="Add…"
        onKeyDown={e => { if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); add(); } else if (e.key === 'Backspace' && !val) onChange(tags.slice(0, -1)); }} />
    </div>
  );
}

function TicketTypeBuilder({ rows, onChange }: { rows: TicketTypeRow[]; onChange: (r: TicketTypeRow[]) => void }) {
  function add() { onChange([...rows, { name: '', price: 0, capacity: '', sort_order: rows.length, description: '', is_visible: true }]); }
  function update(i: number, patch: Partial<TicketTypeRow>) { onChange(rows.map((r, j) => j === i ? { ...r, ...patch } : r)); }
  function remove(i: number) { onChange(rows.filter((_, j) => j !== i)); }
  return (
    <>
      <div className="alert alert-info" style={{ marginBottom: '.75rem' }}>ℹ Ticket types define pricing tiers (e.g. General, VIP). Leave empty for flat pricing.</div>
      {rows.map((r, i) => (
        <div key={i} className="sub-section" style={{ marginBottom: '.5rem' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '.6rem' }}>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.72rem', color: 'var(--text-3)' }}>TICKET TYPE {i + 1}</span>
            <button className="btn-icon" onClick={() => remove(i)} style={{ fontSize: '.9rem', color: 'var(--danger)' }}>✕</button>
          </div>
          <div className="field-row">
            <div className="field" style={{ margin: 0 }}><label>Name *</label><input type="text" value={r.name} onChange={e => update(i, { name: e.target.value })} placeholder="e.g. General Admission" /></div>
            <div className="field" style={{ margin: 0 }}><label>Price (0 = free)</label><input type="number" min="0" step="0.01" value={r.price} onChange={e => update(i, { price: parseFloat(e.target.value) || 0 })} /></div>
          </div>
          <div className="field-row" style={{ marginTop: '.5rem' }}>
            <div className="field" style={{ margin: 0 }}><label>Capacity (blank = unlimited)</label><input type="number" min="1" value={r.capacity} onChange={e => update(i, { capacity: e.target.value })} /></div>
            <div className="field" style={{ margin: 0 }}><label>Sort Order</label><input type="number" value={r.sort_order} onChange={e => update(i, { sort_order: parseInt(e.target.value) || 0 })} /></div>
          </div>
          <div className="field" style={{ marginTop: '.5rem', marginBottom: '.25rem' }}><label>Description</label><input type="text" value={r.description} onChange={e => update(i, { description: e.target.value })} placeholder="Optional" /></div>
          <label className="check-row" style={{ marginTop: '.25rem' }}>
            <input type="checkbox" checked={r.is_visible} onChange={e => update(i, { is_visible: e.target.checked })} />
            <span style={{ fontSize: '.88rem', color: 'var(--text)' }}>Visible to buyers</span>
          </label>
        </div>
      ))}
      <button className="btn btn-sm" onClick={add}>+ Add Ticket Type</button>
    </>
  );
}

const EMPTY_FORM = () => ({
  title: '', description: '', category_id: '', timezone: 'Africa/Johannesburg',
  start_time: '', end_time: '', expires_at: '', price: '', currency: 'ZAR',
  max_capacity: '', allow_waitlist: false,
  venue_name: '', address: '', city: '', latitude: '', longitude: '',
  cover_image_url: '', banner_image_url: '', banner_video_url: '', gallery_images: '',
  refund_policy: '', terms_and_conditions: '',
  tags: [] as string[], performers: [] as string[], teams: [] as string[],
  ticket_types: [] as TicketTypeRow[],
});

type FormState = ReturnType<typeof EMPTY_FORM>;

export default function EventsPage() {
  const toast = useToast();
  const confirm = useConfirm();
  const [events, setEvents] = useState<EventRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM());
  const [activeTab, setActiveTab] = useState(0);
  const [submitting, setSubmitting] = useState(false);
  const [cancelModalOpen, setCancelModalOpen] = useState(false);
  const [cancelId, setCancelId] = useState<string | null>(null);
  const [cancelReason, setCancelReason] = useState('');
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const loadEvents = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({ limit: '50' });
      if (statusFilter) params.set('status', statusFilter);
      if (search) params.set('search', search);
      const { data } = await api.get<{ data: EventRow[] }>(`/admin/events?${params}`);
      setEvents(data.data || []);
    } catch { toast('Failed to load events', 'error'); }
    finally { setLoading(false); }
  }, [search, statusFilter, toast]);

  useEffect(() => { loadEvents(); }, [loadEvents]);

  function debounceSearch(v: string) {
    setSearch(v);
    if (searchTimer.current) clearTimeout(searchTimer.current);
    searchTimer.current = setTimeout(loadEvents, 400);
  }

  function setF(patch: Partial<FormState>) { setForm(p => ({ ...p, ...patch })); }

  function openCreate() {
    setEditingId(null); setForm(EMPTY_FORM()); setActiveTab(0); setModalOpen(true);
  }

  async function openEdit(id: string) {
    setEditingId(id); setActiveTab(0); setModalOpen(true);
    try {
      const { data: e } = await api.get<Record<string, unknown>>(`/admin/events/${id}`);
      setForm({
        title: String(e.title || ''), description: String(e.description || ''),
        category_id: String(e.category_id || ''), timezone: String(e.timezone || 'Africa/Johannesburg'),
        start_time: toLocalInput(e.start_time as string), end_time: toLocalInput(e.end_time as string),
        expires_at: toLocalInput(e.expires_at as string),
        price: String(e.price ?? ''), currency: String(e.currency || 'ZAR'),
        max_capacity: String(e.max_capacity || ''), allow_waitlist: Boolean(e.allow_waitlist),
        venue_name: String(e.venue_name || ''), address: String(e.address || ''),
        city: String(e.city || ''), latitude: String(e.latitude || ''), longitude: String(e.longitude || ''),
        cover_image_url: String(e.cover_image_url || ''), banner_image_url: String(e.banner_image_url || ''),
        banner_video_url: String(e.banner_video_url || ''),
        gallery_images: ((e.gallery_images as string[]) || []).join(', '),
        refund_policy: String(e.refund_policy || ''), terms_and_conditions: String(e.terms_and_conditions || ''),
        tags: (e.tags as string[]) || [], performers: (e.performers as string[]) || [],
        teams: (e.teams as string[]) || [], ticket_types: [],
      });
    } catch { toast('Failed to load event', 'error'); }
  }

  async function submitForm() {
    if (!form.title || form.title.length < 5) { toast('Title must be at least 5 characters', 'error'); setActiveTab(0); return; }
    if (!form.description || form.description.length < 50) { toast('Description must be at least 50 characters', 'error'); setActiveTab(0); return; }
    if (!form.venue_name || !form.address || !form.city) { toast('Venue name, address and city are required', 'error'); setActiveTab(1); return; }
    setSubmitting(true);
    try {
      const toISO = (v: string) => v ? new Date(v).toISOString() : null;
      const numOrNull = (v: string) => v === '' ? null : Number(v);
      const body: Record<string, unknown> = {
        title: form.title.trim(), description: form.description.trim(),
        category_id: form.category_id ? parseInt(form.category_id) : null,
        timezone: form.timezone,
        start_time: toISO(form.start_time), end_time: toISO(form.end_time), expires_at: toISO(form.expires_at),
        price: parseFloat(form.price) || 0, currency: form.currency,
        allow_waitlist: form.allow_waitlist,
        venue_name: form.venue_name.trim(), address: form.address.trim(), city: form.city.trim(),
        latitude: numOrNull(form.latitude), longitude: numOrNull(form.longitude),
        refund_policy: form.refund_policy.trim() || null,
        terms_and_conditions: form.terms_and_conditions.trim() || null,
        tags: form.tags, performers: form.performers, teams: form.teams,
      };
      if (form.max_capacity) body.max_capicity = parseInt(form.max_capacity);
      if (form.cover_image_url) body.cover_image_url = form.cover_image_url;
      if (form.banner_image_url) body.banner_image_url = form.banner_image_url;
      if (form.banner_video_url) body.banner_video_url = form.banner_video_url;
      if (form.gallery_images) body.gallery_images = form.gallery_images.split(',').map(u => u.trim()).filter(Boolean);
      if (form.ticket_types.length) {
        body.ticket_types = form.ticket_types.filter(t => t.name).map(t => ({
          name: t.name, price: t.price, sort_order: t.sort_order, is_visible: t.is_visible,
          ...(t.capacity ? { capacity: parseInt(t.capacity) } : {}),
          ...(t.description ? { description: t.description } : {}),
        }));
      }
      if (editingId) { await api.put(`/admin/events/${editingId}`, body); toast('Event updated', 'success'); }
      else { await api.post('/admin/events', body); toast('Event created as draft', 'success'); }
      setModalOpen(false); loadEvents();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Save failed';
      toast(msg, 'error');
    } finally { setSubmitting(false); }
  }

  async function publishEvent(id: string) {
    if (!confirm) return;
    confirm('Publish Event', 'This event will become visible to all users.', async () => {
      await api.post(`/organiser/events/${id}/publish`);
      toast('Event published!', 'success'); loadEvents();
    });
  }

  function openCancel(id: string) { setCancelId(id); setCancelReason(''); setCancelModalOpen(true); }

  async function submitCancel() {
    if (cancelReason.length < 10) { toast('Reason must be at least 10 characters', 'error'); return; }
    try {
      await api.post(`/organiser/events/${cancelId}/cancel`, { reason: cancelReason });
      toast('Event cancelled', 'success'); setCancelModalOpen(false); loadEvents();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Cancel failed';
      toast(msg, 'error');
    }
  }

  function deleteEvent(id: string, title: string) {
    confirm('Delete Event', `Permanently delete "${title}"? This cannot be undone.`, async () => {
      await api.delete(`/admin/events/${id}`);
      toast('Event deleted', 'success'); loadEvents();
    });
  }

  return (
    <div style={{ padding: '1.5rem', maxWidth: '1400px', display: 'flex', flexDirection: 'column', flex: 1 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600 }}>Events</div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: '.15rem' }}>Create and manage all platform events</div>
        </div>
        <button className="btn btn-primary" onClick={openCreate}>+ New Event</button>
      </div>

      <div className="table-wrap">
        <div className="table-toolbar">
          <input className="search-input" type="text" placeholder="Search events…" value={search} onChange={e => debounceSearch(e.target.value)} />
          <select className="filter-select" value={statusFilter} onChange={e => setStatusFilter(e.target.value)}>
            <option value="">All Statuses</option>
            <option value="draft">Draft</option>
            <option value="published">Published</option>
            <option value="cancelled">Cancelled</option>
            <option value="completed">Completed</option>
          </select>
          <button className="btn btn-sm" onClick={loadEvents}>↺</button>
          <div style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: '.72rem', color: 'var(--text-3)' }}>{events.length} events</div>
        </div>
        <div style={{ overflowX: 'auto', flex: 1 }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '.85rem', tableLayout: 'fixed' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                {['Title', 'Status', 'Start Date', 'Price', 'Sold / Cap', 'Revenue', 'Actions'].map((h, i) => (
                  <th key={h} style={{ padding: '.65rem 1rem', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '.65rem', textTransform: 'uppercase', letterSpacing: '.07em', color: 'var(--text-3)', fontWeight: 500, width: ['30%','10%','14%','10%','10%','12%','14%'][i] }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={7} style={{ padding: '2rem', textAlign: 'center' }}><div className="spinner" style={{ margin: 'auto' }} /></td></tr>
              ) : events.length === 0 ? (
                <tr><td colSpan={7} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-3)' }}>No events found.</td></tr>
              ) : events.map(e => (
                <tr key={e.id} style={{ borderBottom: '1px solid rgba(45,45,61,.5)' }}>
                  <td style={{ padding: '.65rem 1rem', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={e.title}>{e.title}</td>
                  <td style={{ padding: '.65rem 1rem' }}><Badge status={e.status} /></td>
                  <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)', fontSize: '.77rem', color: 'var(--text-2)' }}>{fmtDate(e.start_time)}</td>
                  <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{fmtMoney(e.price, e.currency)}</td>
                  <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{e.tickets_sold}{e.max_capacity ? ` / ${e.max_capacity}` : ''}</td>
                  <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{fmtMoney(e.ticket_revenue)}</td>
                  <td style={{ padding: '.65rem 1rem', overflow: 'visible', whiteSpace: 'nowrap' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '.3rem' }}>
                      <button className="btn-icon" onClick={() => openEdit(e.id)} title="Edit">✎</button>
                      {e.status === 'draft' && <button className="btn btn-success btn-sm" onClick={() => publishEvent(e.id)}>Publish</button>}
                      {e.status === 'published' && <button className="btn btn-sm" onClick={() => openCancel(e.id)} style={{ borderColor: 'var(--warning)', color: 'var(--warning)' }}>Cancel</button>}
                      <button className="btn-icon" onClick={() => deleteEvent(e.id, e.title)} style={{ color: 'var(--danger)' }} title="Delete">✕</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Event Modal */}
      {modalOpen && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setModalOpen(false); }}>
          <div className="modal modal-lg">
            <div className="modal-header">
              <div className="modal-title">{editingId ? 'Edit Event' : 'Create Event'}</div>
              <button className="modal-close" onClick={() => setModalOpen(false)}>✕</button>
            </div>
            <div className="modal-body">
              <div className="tabs">
                {TABS.map((t, i) => <button key={t} className={`tab${activeTab === i ? ' active' : ''}`} onClick={() => setActiveTab(i)}>{t}</button>)}
              </div>

              {/* Basic Info */}
              {activeTab === 0 && (
                <>
                  <div className="field"><label>Title *</label><input type="text" value={form.title} onChange={e => setF({ title: e.target.value })} placeholder="e.g. Summer Music Festival 2025" minLength={5} maxLength={255} /></div>
                  <div className="field"><label>Description * <small style={{ color: 'var(--text-3)', fontWeight: 300 }}>(min 50 chars)</small></label><textarea rows={4} value={form.description} onChange={e => setF({ description: e.target.value })} placeholder="Describe your event in detail…" /></div>
                  <div className="field-row">
                    <div className="field"><label>Category *</label>
                      <select value={form.category_id} onChange={e => setF({ category_id: e.target.value })}>
                        <option value="">Select category</option>
                        {[['1','Music'],['2','Sports'],['3','Arts & Culture'],['4','Food & Drink'],['5','Technology'],['6','Business'],['7','Health & Wellness'],['8','Education'],['9','Comedy'],['10','Other']].map(([v,l]) => <option key={v} value={v}>{l}</option>)}
                      </select>
                    </div>
                    <div className="field"><label>Timezone *</label>
                      <select value={form.timezone} onChange={e => setF({ timezone: e.target.value })}>
                        <option value="Africa/Johannesburg">Africa/Johannesburg (SAST)</option>
                        <option value="Africa/Cape_Town">Africa/Cape_Town</option>
                        <option value="UTC">UTC</option>
                        <option value="Europe/London">Europe/London</option>
                        <option value="America/New_York">America/New_York</option>
                        <option value="America/Los_Angeles">America/Los_Angeles</option>
                        <option value="Asia/Dubai">Asia/Dubai</option>
                      </select>
                    </div>
                  </div>
                  <div className="field-row-3">
                    <div className="field"><label>Start Time *</label><input type="datetime-local" value={form.start_time} onChange={e => setF({ start_time: e.target.value })} /></div>
                    <div className="field"><label>End Time *</label><input type="datetime-local" value={form.end_time} onChange={e => setF({ end_time: e.target.value })} /></div>
                    <div className="field"><label>Expires At *</label><input type="datetime-local" value={form.expires_at} onChange={e => setF({ expires_at: e.target.value })} /></div>
                  </div>
                  <div className="field-row">
                    <div className="field"><label>Base Price *</label><input type="number" min="0" step="0.01" value={form.price} onChange={e => setF({ price: e.target.value })} placeholder="0.00" /></div>
                    <div className="field"><label>Currency *</label>
                      <select value={form.currency} onChange={e => setF({ currency: e.target.value })}>
                        <option value="ZAR">ZAR — South African Rand</option>
                        <option value="USD">USD — US Dollar</option>
                        <option value="EUR">EUR — Euro</option>
                        <option value="GBP">GBP — British Pound</option>
                        <option value="KES">KES — Kenyan Shilling</option>
                        <option value="NGN">NGN — Nigerian Naira</option>
                      </select>
                    </div>
                  </div>
                  <div className="field-row">
                    <div className="field"><label>Max Capacity <small style={{ color: 'var(--text-3)' }}>(leave blank for unlimited)</small></label><input type="number" min="1" value={form.max_capacity} onChange={e => setF({ max_capacity: e.target.value })} placeholder="Unlimited" /></div>
                    <div className="field" style={{ display: 'flex', alignItems: 'flex-end', paddingBottom: '.1rem' }}>
                      <label className="check-row"><input type="checkbox" checked={form.allow_waitlist} onChange={e => setF({ allow_waitlist: e.target.checked })} /><span style={{ fontSize: '.88rem', color: 'var(--text)' }}>Allow Waitlist when sold out</span></label>
                    </div>
                  </div>
                  <div className="section-sep">Tags & Performers</div>
                  <div className="field"><label>Tags <small style={{ color: 'var(--text-3)' }}>(press Enter to add)</small></label><TagInput tags={form.tags} onChange={t => setF({ tags: t })} /></div>
                  <div className="field"><label>Performers</label><TagInput tags={form.performers} onChange={t => setF({ performers: t })} /></div>
                  <div className="field"><label>Teams</label><TagInput tags={form.teams} onChange={t => setF({ teams: t })} /></div>
                </>
              )}

              {/* Location */}
              {activeTab === 1 && (
                <>
                  <div className="field"><label>Venue Name *</label><input type="text" value={form.venue_name} onChange={e => setF({ venue_name: e.target.value })} placeholder="e.g. FNB Stadium" /></div>
                  <div className="field"><label>Street Address *</label><input type="text" value={form.address} onChange={e => setF({ address: e.target.value })} placeholder="e.g. 1 Soccer City Ave" /></div>
                  <div className="field-row">
                    <div className="field"><label>City *</label><input type="text" value={form.city} onChange={e => setF({ city: e.target.value })} placeholder="e.g. Johannesburg" /></div>
                    <div className="field" />
                  </div>
                  <div className="field-row">
                    <div className="field"><label>Latitude <small style={{ color: 'var(--text-3)' }}>(optional)</small></label><input type="number" step="0.000001" value={form.latitude} onChange={e => setF({ latitude: e.target.value })} placeholder="-26.2041" /></div>
                    <div className="field"><label>Longitude <small style={{ color: 'var(--text-3)' }}>(optional)</small></label><input type="number" step="0.000001" value={form.longitude} onChange={e => setF({ longitude: e.target.value })} placeholder="28.0473" /></div>
                  </div>
                </>
              )}

              {/* Ticket Types */}
              {activeTab === 2 && <TicketTypeBuilder rows={form.ticket_types} onChange={t => setF({ ticket_types: t })} />}

              {/* Media & Policy */}
              {activeTab === 3 && (
                <>
                  <div className="field"><label>Cover Image URL</label><input type="url" value={form.cover_image_url} onChange={e => setF({ cover_image_url: e.target.value })} placeholder="https://…/cover.jpg" /></div>
                  <div className="field"><label>Banner Image URL</label><input type="url" value={form.banner_image_url} onChange={e => setF({ banner_image_url: e.target.value })} placeholder="https://…/banner.jpg" /></div>
                  <div className="field"><label>Banner Video URL</label><input type="url" value={form.banner_video_url} onChange={e => setF({ banner_video_url: e.target.value })} placeholder="https://…/banner.mp4" /></div>
                  <div className="field"><label>Gallery Images <small style={{ color: 'var(--text-3)' }}>(comma-separated URLs)</small></label><textarea rows={2} value={form.gallery_images} onChange={e => setF({ gallery_images: e.target.value })} placeholder="https://…/img1.jpg, https://…/img2.jpg" /></div>
                  <div className="section-sep">Legal</div>
                  <div className="field"><label>Refund Policy</label><textarea rows={3} value={form.refund_policy} onChange={e => setF({ refund_policy: e.target.value })} placeholder="Describe the refund policy…" /></div>
                  <div className="field"><label>Terms & Conditions</label><textarea rows={3} value={form.terms_and_conditions} onChange={e => setF({ terms_and_conditions: e.target.value })} placeholder="Terms and conditions for attendees…" /></div>
                </>
              )}
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setModalOpen(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={submitForm} disabled={submitting}>{submitting ? 'Saving…' : editingId ? 'Save Changes' : 'Create Event'}</button>
            </div>
          </div>
        </div>
      )}

      {/* Cancel Modal */}
      {cancelModalOpen && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setCancelModalOpen(false); }}>
          <div className="modal modal-sm">
            <div className="modal-header">
              <div className="modal-title">Cancel Event</div>
              <button className="modal-close" onClick={() => setCancelModalOpen(false)}>✕</button>
            </div>
            <div className="modal-body">
              <div className="alert alert-warning" style={{ marginBottom: '.75rem' }}>This will cancel the event. All attendees will be automatically refunded.</div>
              <div className="field"><label>Cancellation Reason * <small style={{ color: 'var(--text-3)' }}>(min 10 chars)</small></label><textarea rows={3} value={cancelReason} onChange={e => setCancelReason(e.target.value)} placeholder="Please provide a reason for cancellation…" /></div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setCancelModalOpen(false)}>Back</button>
              <button className="btn btn-danger" onClick={submitCancel}>Confirm Cancellation</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}