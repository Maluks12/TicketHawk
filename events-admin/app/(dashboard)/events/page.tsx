'use client';
import { useEffect, useState, useRef, useCallback } from 'react';
import { api } from '../../../lib/api';
import { fmtMoney, fmtDate, fmtDateTime, toLocalInput, Badge } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useStaff } from '../../../lib/permissions';
import { useConfirm } from '../../../lib/confirm';

// ── Types ───────────────────────────────────────────────────────────────────
interface EventRow { id: string; title: string; slug: string; status: string; start_time: string; price: number; currency: string; tickets_sold: number; max_capacity: number | null; ticket_revenue: number; city: string; }
interface TicketTypeRow { name: string; price: number; capacity: string; sort_order: number; description: string; is_visible: boolean; }

interface LocalDraft {
  id: string;
  savedAt: string;   // ISO timestamp
  form: FormState;
  coverImageUrl: string | null;
  bannerImageUrl: string | null;
  activeTab: number;
}

const DRAFTS_KEY = 'evt_admin_event_drafts';
const TABS = ['Basic Info', 'Location', 'Ticket Types', 'Media & Policy'];

// ── Draft helpers ────────────────────────────────────────────────────────────
function loadDrafts(): LocalDraft[] {
  if (typeof window === 'undefined') return [];
  try { return JSON.parse(localStorage.getItem(DRAFTS_KEY) || '[]'); } catch { return []; }
}
function saveDrafts(drafts: LocalDraft[]) {
  if (typeof window === 'undefined') return;
  localStorage.setItem(DRAFTS_KEY, JSON.stringify(drafts));
}
function upsertDraft(draft: LocalDraft) {
  const drafts = loadDrafts();
  const idx = drafts.findIndex(d => d.id === draft.id);
  if (idx >= 0) drafts[idx] = draft; else drafts.unshift(draft);
  saveDrafts(drafts);
}
function removeDraft(id: string) {
  saveDrafts(loadDrafts().filter(d => d.id !== id));
}
function isDraftEmpty(form: FormState) {
  return !form.title && !form.description && !form.venue_name && !form.city;
}

// ── TagInput ─────────────────────────────────────────────────────────────────
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
        onBlur={add}
        onKeyDown={e => { if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); add(); } else if (e.key === 'Backspace' && !val) onChange(tags.slice(0, -1)); }} />
    </div>
  );
}

// ── TicketTypeBuilder ────────────────────────────────────────────────────────
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
            <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '.04em' }}>Ticket Type {i + 1}</span>
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

// ── EMPTY_FORM ───────────────────────────────────────────────────────────────
const EMPTY_FORM = () => ({
  title: '', description: '', category_id: '', timezone: 'Africa/Johannesburg',
  start_time: '', end_time: '', expires_at: '', price: '', currency: 'ZAR',
  max_capacity: '', allow_waitlist: false,
  max_tickets_per_user: '',
  venue_name: '', address: '', city: '',
  venue_type: 'auto' as 'auto' | 'stadium' | 'auditorium' | 'none',
  refund_policy: '', terms_and_conditions: '',
  tags: [] as string[], performers: [] as string[], teams: [] as string[],
  ticket_types: [] as TicketTypeRow[],
});
type FormState = ReturnType<typeof EMPTY_FORM>;

// ── ImageUploader ────────────────────────────────────────────────────────────
interface ImageUploaderProps {
  label: string;
  imageUrl: string | null;
  uploading: boolean;
  onUpload: (file: File) => void;
  onRemove: () => void;
  hint?: string;
}
function ImageUploader({ label, imageUrl, uploading, onUpload, onRemove, hint }: ImageUploaderProps) {
  return (
    <div className="field">
      <label>{label}</label>
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: '.75rem' }}>
        {imageUrl ? (
          <div style={{ position: 'relative', width: '120px', height: '80px', flexShrink: 0 }}>
            <img src={imageUrl} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: 'var(--r)', border: '1px solid var(--border)', display: 'block' }}
              onError={e => { (e.target as HTMLImageElement).style.opacity = '0.3'; }} />
            <button type="button" onClick={onRemove}
              style={{ position: 'absolute', top: '-6px', right: '-6px', width: '20px', height: '20px', borderRadius: '50%', background: 'var(--danger)', color: '#fff', border: 'none', cursor: 'pointer', fontSize: '10px', display: 'flex', alignItems: 'center', justifyContent: 'center', lineHeight: 1 }}>✕</button>
          </div>
        ) : (
          <label style={{ width: '120px', height: '80px', border: '1px dashed var(--border)', borderRadius: 'var(--r)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', cursor: uploading ? 'wait' : 'pointer', color: 'var(--text-3)', fontSize: '.68rem', gap: '.25rem', background: 'var(--surface-2)', flexShrink: 0 }}>
            <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={uploading}
              onChange={e => { const f = e.target.files?.[0]; if (f) { onUpload(f); e.target.value = ''; } }} />
            {uploading ? <div className="spinner" style={{ width: '18px', height: '18px' }} /> : <><span style={{ fontSize: '1.5rem', lineHeight: 1 }}>+</span><span>Upload</span></>}
          </label>
        )}
        <small style={{ color: 'var(--text-3)', fontSize: '.72rem', marginTop: '.25rem' }}>{hint ?? 'JPEG · PNG · WebP'}</small>
      </div>
    </div>
  );
}


// ── DraftCard ────────────────────────────────────────────────────────────────
function DraftCard({ draft, onResume, onDelete }: { draft: LocalDraft; onResume: () => void; onDelete: () => void }) {
  const title = draft.form.title?.trim() || 'Untitled Draft';
  const subtitle = [draft.form.city, draft.form.venue_name].filter(Boolean).join(' · ') || 'No location yet';
  const tab = TABS[draft.activeTab] ?? 'Basic Info';
  const ago = (() => {
    const diff = Date.now() - new Date(draft.savedAt).getTime();
    const m = Math.floor(diff / 60000);
    if (m < 1) return 'just now';
    if (m < 60) return `${m}m ago`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h}h ago`;
    return `${Math.floor(h / 24)}d ago`;
  })();
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', padding: '.75rem 1rem', background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 'var(--r)', minWidth: 0 }}>
      <div style={{ width: '36px', height: '36px', borderRadius: 'var(--r)', background: 'var(--accent-soft)', border: '1px solid var(--accent-border)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
        <span style={{ fontSize: '1rem' }}>📝</span>
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontWeight: 500, fontSize: '.88rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{title}</div>
        <div style={{ fontSize: '.73rem', color: 'var(--text-3)', marginTop: '.1rem', display: 'flex', gap: '.5rem', flexWrap: 'wrap' }}>
          <span>{subtitle}</span>
          <span style={{ opacity: .5 }}>·</span>
          <span>Last edited: {tab}</span>
          <span style={{ opacity: .5 }}>·</span>
          <span>{ago}</span>
        </div>
      </div>
      <div style={{ display: 'flex', gap: '.4rem', flexShrink: 0 }}>
        <button className="btn btn-sm btn-primary" onClick={onResume} style={{ fontSize: '.78rem', padding: '.3rem .7rem' }}>Resume</button>
        <button className="btn-icon" onClick={onDelete} style={{ color: 'var(--danger)', fontSize: '.85rem' }} title="Delete draft">✕</button>
      </div>
    </div>
  );
}

// ── Page component ───────────────────────────────────────────────────────────
export default function EventsPage() {
  const toast = useToast();
  const confirm = useConfirm();
  const [events, setEvents] = useState<EventRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [viewingId, setViewingId] = useState<string | null>(null);
  // Editing a live event changes what buyers already paid for, so it stays with
  // admins; everyone else reads the record.
  const canEdit = useStaff((st) => st.can('event.approve'));
  const loadStaffCaps = useStaff((st) => st.load);
  const [form, setForm] = useState<FormState>(EMPTY_FORM());
  const [activeTab, setActiveTab] = useState(0);
  const [submitting, setSubmitting] = useState(false);

  const [cancelModalOpen, setCancelModalOpen] = useState(false);
  const [cancelId, setCancelId] = useState<string | null>(null);
  const [cancelReason, setCancelReason] = useState('');
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ── Image upload state ───────────────────────────────────────────────────
  const [coverImageUrl, setCoverImageUrl] = useState<string | null>(null);
  const [bannerImageUrl, setBannerImageUrl] = useState<string | null>(null);
  const [coverUploading, setCoverUploading] = useState(false);
  const [bannerUploading, setBannerUploading] = useState(false);

  // ── Draft state ──────────────────────────────────────────────────────────
  const [drafts, setDrafts] = useState<LocalDraft[]>([]);
  const [currentDraftId, setCurrentDraftId] = useState<string | null>(null);
  // Refs so the close handler always sees fresh values
  const formRef = useRef(form);
  const activeTabRef = useRef(activeTab);
  const coverRef = useRef(coverImageUrl);
  const bannerRef = useRef(bannerImageUrl);
  const currentDraftIdRef = useRef(currentDraftId);
  const editingIdRef = useRef(editingId);

  useEffect(() => { loadStaffCaps(); }, [loadStaffCaps]);
  useEffect(() => { formRef.current = form; }, [form]);
  useEffect(() => { activeTabRef.current = activeTab; }, [activeTab]);
  useEffect(() => { coverRef.current = coverImageUrl; }, [coverImageUrl]);
  useEffect(() => { bannerRef.current = bannerImageUrl; }, [bannerImageUrl]);
  useEffect(() => { currentDraftIdRef.current = currentDraftId; }, [currentDraftId]);
  useEffect(() => { editingIdRef.current = editingId; }, [editingId]);

  // Load drafts from localStorage on mount
  useEffect(() => { setDrafts(loadDrafts()); }, []);

  const refreshDrafts = useCallback(() => { setDrafts(loadDrafts()); }, []);

  // ── Save-as-draft on modal close (new events only) ───────────────────────
  const closeModal = useCallback((didSubmit = false) => {
    // Only auto-draft when creating a new event (not editing an existing one)
    if (!editingIdRef.current && !didSubmit) {
      const f = formRef.current;
      if (!isDraftEmpty(f)) {
        const draftId = currentDraftIdRef.current ?? crypto.randomUUID();
        const draft: LocalDraft = {
          id: draftId,
          savedAt: new Date().toISOString(),
          form: f,
          coverImageUrl: coverRef.current,
          bannerImageUrl: bannerRef.current,
          activeTab: activeTabRef.current,
        };
        upsertDraft(draft);
        refreshDrafts();
        toast('Draft saved', 'success');
      }
    }
    setModalOpen(false);
  }, [refreshDrafts, toast]);

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

  // ── Create / resume ──────────────────────────────────────────────────────
  function openCreate() {
    setEditingId(null);
    setForm(EMPTY_FORM());
    setActiveTab(0);
    setCoverImageUrl(null); setBannerImageUrl(null);
    setCurrentDraftId(null);
    setModalOpen(true);
  }

  function resumeDraft(draft: LocalDraft) {
    setEditingId(null);
    setForm(draft.form);
    setActiveTab(draft.activeTab);
    setCoverImageUrl(draft.coverImageUrl);
    setBannerImageUrl(draft.bannerImageUrl);
    setCurrentDraftId(draft.id);
    setModalOpen(true);
  }

  function deleteDraft(id: string) {
    removeDraft(id);
    refreshDrafts();
    if (currentDraftId === id) setCurrentDraftId(null);
    toast('Draft deleted', 'success');
  }

  // ── Image upload ─────────────────────────────────────────────────────────
  async function uploadImage(file: File, setUploading: (v: boolean) => void, onSuccess: (url: string) => void) {
    setUploading(true);
    try {
      const fd = new FormData();
      fd.append('image', file);
      const token = typeof window !== 'undefined' ? localStorage.getItem('evt_token') : null;
      const baseURL = (process.env.NEXT_PUBLIC_API_URL || '').replace(/\/$/, '');
      const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};
      const res = await fetch(`${baseURL}/admin/upload`, { method: 'POST', headers, body: fd });
      const json = await res.json().catch(() => null) as { image_url?: string; error?: string } | null;
      if (!res.ok) throw new Error(json?.error ?? `Upload failed (HTTP ${res.status})`);
      const imageUrl = json?.image_url ?? null;
      if (imageUrl) { onSuccess(imageUrl); toast('Image uploaded', 'success'); }
      else toast('Upload succeeded but no URL was returned', 'error');
    } catch (e: unknown) {
      toast((e instanceof Error ? e.message : null) ?? 'Image upload failed', 'error');
    } finally { setUploading(false); }
  }

  // ── Open edit ────────────────────────────────────────────────────────────
  async function openEdit(id: string) {
    setEditingId(id); setActiveTab(0); setCurrentDraftId(null); setModalOpen(true);
    try {
      const { data: e } = await api.get<Record<string, unknown>>(`/admin/events/${id}`);
      setForm({
        title: String(e.title || ''), description: String(e.description || ''),
        category_id: String(e.category_id || ''), timezone: String(e.timezone || 'Africa/Johannesburg'),
        start_time: toLocalInput(e.start_time as string), end_time: toLocalInput(e.end_time as string),
        expires_at: toLocalInput(e.expires_at as string),
        price: String(e.price ?? ''), currency: String(e.currency || 'ZAR'),
        max_capacity: String(e.max_capacity || ''), allow_waitlist: Boolean(e.allow_waitlist),
        max_tickets_per_user: String(e.max_tickets_per_user || ''),
        venue_name: String(e.venue_name || ''), address: String(e.address || ''),
        city: String(e.city || ''),
        venue_type: (['stadium','auditorium','none'].includes(e.venue_type as string) ? e.venue_type as 'stadium'|'auditorium'|'none' : 'auto'),
        refund_policy: String(e.refund_policy || ''), terms_and_conditions: String(e.terms_and_conditions || ''),
        tags: (e.tags as string[]) || [], performers: (e.performers as string[]) || [],
        teams: (e.teams as string[]) || [], ticket_types: [],
      });
      setCoverImageUrl(e.cover_image_url ? String(e.cover_image_url) : null);
      setBannerImageUrl(e.banner_image_url ? String(e.banner_image_url) : null);
    } catch { toast('Failed to load event', 'error'); }
  }

  // ── Submit ───────────────────────────────────────────────────────────────
  async function submitForm() {
    if (!form.title || form.title.length < 5) { toast('Title must be at least 5 characters', 'error'); setActiveTab(0); return; }
    if (!form.description || form.description.length < 50) { toast('Description must be at least 50 characters', 'error'); setActiveTab(0); return; }
    if (!form.venue_name || !form.address || !form.city) { toast('Venue name, address and city are required', 'error'); setActiveTab(1); return; }
    setSubmitting(true);
    try {
      const toISO = (v: string) => v ? new Date(v).toISOString() : null;
      const body: Record<string, unknown> = {
        title: form.title.trim(), description: form.description.trim(),
        category_id: form.category_id ? parseInt(form.category_id) : null,
        timezone: form.timezone,
        start_time: toISO(form.start_time), end_time: toISO(form.end_time), expires_at: toISO(form.expires_at),
        price: parseFloat(form.price) || 0, currency: form.currency,
        allow_waitlist: form.allow_waitlist,
        venue_name: form.venue_name.trim(), address: form.address.trim(), city: form.city.trim(),
        refund_policy: form.refund_policy.trim() || null,
        terms_and_conditions: form.terms_and_conditions.trim() || null,
        tags: form.tags, performers: form.performers, teams: form.teams,
      };
      if (form.max_capacity) body.max_capicity = parseInt(form.max_capacity);
      if (form.max_tickets_per_user) body.max_tickets_per_user = parseInt(form.max_tickets_per_user);
      body.venue_type = form.venue_type !== 'auto' ? form.venue_type : null;
      if (coverImageUrl) body.cover_image_url = coverImageUrl;
      if (bannerImageUrl) body.banner_image_url = bannerImageUrl;
      if (form.ticket_types.length) {
        body.ticket_types = form.ticket_types.filter(t => t.name).map(t => ({
          name: t.name, price: t.price, sort_order: t.sort_order, is_visible: t.is_visible,
          ...(t.capacity ? { capacity: parseInt(t.capacity) } : {}),
          ...(t.description ? { description: t.description } : {}),
        }));
      }
      if (editingId) { await api.put(`/admin/events/${editingId}`, body); toast('Event updated', 'success'); }
      else { await api.post('/admin/events', body); toast('Event created — pending review', 'success'); }
      // On success, remove local draft if one was active
      if (currentDraftId) { removeDraft(currentDraftId); refreshDrafts(); setCurrentDraftId(null); }
      closeModal(true);
      loadEvents();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Save failed';
      toast(msg, 'error');
    } finally { setSubmitting(false); }
  }

  async function publishEvent(id: string) {
    if (!confirm) return;
    confirm('Publish Event', 'This event will be publicly visible to all users.', async () => {
      try {
        await api.put(`/admin/events/${id}/approve`);
        toast('Event published!', 'success');
        loadEvents();
      } catch (e: unknown) {
        const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Failed to publish event';
        toast(msg, 'error');
      }
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
    try {
      await api.delete(`/admin/events/${id}`);
      toast('Event deleted', 'success');
      loadEvents();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Delete failed';
      toast(msg, 'error');
    }
  });
}
  return (
    <div className="page">
      <div className="page-header">
        <div>
          <div className="page-title">Events</div>
          <div className="page-sub">Create and manage all platform events</div>
        </div>
        <button className="btn btn-primary" onClick={openCreate}>+ New Event</button>
      </div>

      {/* ── Local Drafts ── */}
      {drafts.length > 0 && (
        <div style={{ padding: '16px 20px 0', flexShrink: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
            <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--text-2)', textTransform: 'uppercase', letterSpacing: '.04em' }}>Local Drafts</span>
            <span style={{ background: 'var(--accent-soft)', color: 'var(--accent)', borderRadius: 99, padding: '1px 8px', fontSize: 12, fontWeight: 700 }}>{drafts.length}</span>
            <span style={{ fontSize: 13, color: 'var(--text-3)', marginLeft: 2 }}>Unsaved forms are auto-saved when you close the modal</span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '.5rem' }}>
            {drafts.map(d => (
              <DraftCard
                key={d.id}
                draft={d}
                onResume={() => resumeDraft(d)}
                onDelete={() => deleteDraft(d.id)}
              />
            ))}
          </div>
        </div>
      )}


      {/* ── Events Table ── */}
      <div style={{ flex: 1, padding: '16px 20px', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div className="table-wrap">
        <div className="table-toolbar">
          <input className="search-input" type="text" placeholder="Search events…" value={search} onChange={e => debounceSearch(e.target.value)} />
          <select className="filter-select" value={statusFilter} onChange={e => setStatusFilter(e.target.value)}>
            <option value="">All Statuses</option>
            <option value="draft">Draft</option>
            <option value="pending_review">Pending Review</option>
            <option value="published">Published</option>
            <option value="cancelled">Cancelled</option>
            <option value="completed">Completed</option>
            <option value="rejected">Rejected</option>
          </select>
          <button className="btn btn-sm" onClick={loadEvents}>↺</button>
          <div style={{ marginLeft: 'auto', fontSize: 13, color: 'var(--text-3)' }}>{events.length} events</div>
        </div>
        <div style={{ overflowX: 'auto', flex: 1 }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '.85rem', tableLayout: 'fixed' }}>
            <thead>
              <tr>
                {['Title', 'Status', 'Start Date', 'Price', 'Sold / Cap', 'Revenue', 'Actions'].map((h, i) => (
                  <th key={h} style={{ padding: '8px 12px', textAlign: 'left', fontSize: 12, fontWeight: 600, color: 'var(--text-2)', width: ['30%','10%','14%','10%','10%','12%','14%'][i] }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={7} style={{ padding: '2rem', textAlign: 'center' }}><div className="spinner" style={{ margin: 'auto' }} /></td></tr>
              ) : events.length === 0 ? (
                <tr><td colSpan={7} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-3)' }}>No events found.</td></tr>
              ) : events.map(e => (
                <tr key={e.id} style={{ borderBottom: '1px solid var(--border)' }}>
                  <td style={{ padding: '10px 12px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={e.title}>
                    <button type="button" onClick={() => setViewingId(e.id)}
                      style={{
                        background: 'none', border: 'none', padding: 0, cursor: 'pointer',
                        font: 'inherit', fontWeight: 600, color: 'var(--accent)', textAlign: 'left',
                      }}>
                      {e.title}
                    </button>
                  </td>
                  <td style={{ padding: '10px 12px' }}><Badge status={e.status} /></td>
                  <td style={{ padding: '10px 12px', fontSize: 13, color: 'var(--text-2)' }}>{fmtDate(e.start_time)}</td>
                  <td style={{ padding: '10px 12px' }}>{fmtMoney(e.price, e.currency)}</td>
                  <td style={{ padding: '10px 12px' }}>{e.tickets_sold}{e.max_capacity ? ` / ${e.max_capacity}` : ''}</td>
                  <td style={{ padding: '10px 12px' }}>{fmtMoney(e.ticket_revenue)}</td>
                  <td style={{ padding: '10px 12px', overflow: 'visible', whiteSpace: 'nowrap' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '.3rem' }}>
                      <button className="btn-icon" onClick={() => setViewingId(e.id)} title="View details">👁</button>
                      {canEdit && (
                        <button className="btn-icon" onClick={() => openEdit(e.id)} title="Edit">✎</button>
                      )}
                      {(e.status === 'draft' || e.status === 'pending_review') && (
                        <button className="btn btn-success btn-sm" onClick={() => publishEvent(e.id)}>Publish</button>
                      )}
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
      </div>

      {/* ── Event Modal ── */}
      {modalOpen && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) closeModal(); }}>
          <div className="modal modal-lg">
            <div className="modal-header">
              <div style={{ display: 'flex', alignItems: 'center', gap: '.6rem' }}>
                <div className="modal-title">{editingId ? 'Edit Event' : currentDraftId ? 'Resume Draft' : 'Create Event'}</div>
                {currentDraftId && (
                  <span style={{ fontSize: 12, fontWeight: 700, background: 'var(--accent-soft)', color: 'var(--accent)', borderRadius: 99, padding: '2px 8px' }}>DRAFT</span>
                )}
              </div>
              <button className="modal-close" onClick={() => closeModal()}>✕</button>
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
                    <div className="field"><label>Max Tickets Per User <small style={{ color: 'var(--text-3)' }}>(blank = no limit)</small></label><input type="number" min="1" max="20" value={form.max_tickets_per_user} onChange={e => setF({ max_tickets_per_user: e.target.value })} placeholder="No limit" /></div>
                  </div>
                  <div className="field" style={{ marginBottom: '16px' }}>
                    <label className="check-row"><input type="checkbox" checked={form.allow_waitlist} onChange={e => setF({ allow_waitlist: e.target.checked })} /><span style={{ fontSize: '.88rem', color: 'var(--text)' }}>Allow Waitlist when sold out</span></label>
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
                  <div className="section-sep">Seating</div>
                  <div className="field">
                    <label>Venue Type <small style={{ color: 'var(--text-3)', fontWeight: 300 }}>(controls seating map shown in app)</small></label>
                    <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', marginTop: '6px' }}>
                      {([
                        { value: 'auto',       emoji: '⚡', label: 'Auto-detect',   hint: 'Guess from category & venue name' },
                        { value: 'stadium',    emoji: '🏟️', label: 'Stadium',        hint: 'Full stadium section map' },
                        { value: 'auditorium', emoji: '🎭', label: 'Hall / Theatre', hint: 'Auditorium seating map' },
                        { value: 'none',       emoji: '🎫', label: 'None',           hint: 'Skip seating map — go straight to checkout' },
                      ] as const).map(opt => (
                        <button key={opt.value} type="button" title={opt.hint}
                          onClick={() => setF({ venue_type: opt.value })}
                          style={{
                            padding: '8px 14px', borderRadius: 'var(--r)', cursor: 'pointer', fontSize: '13px',
                            fontWeight: form.venue_type === opt.value ? 600 : 400,
                            border: form.venue_type === opt.value ? '2px solid var(--blue)' : '1px solid var(--border)',
                            background: form.venue_type === opt.value ? 'var(--blue-light)' : 'var(--surface)',
                            color: form.venue_type === opt.value ? 'var(--blue)' : 'var(--text-2)',
                            transition: 'all .15s',
                          }}>
                          {opt.emoji} {opt.label}
                        </button>
                      ))}
                    </div>
                    <small style={{ color: 'var(--text-3)', fontSize: '12px', marginTop: '6px', display: 'block' }}>
                      <strong>None</strong> skips seat selection entirely — buyers pick a quantity and proceed straight to checkout.
                    </small>
                  </div>
                </>
              )}

              {/* Ticket Types */}
              {activeTab === 2 && <TicketTypeBuilder rows={form.ticket_types} onChange={t => setF({ ticket_types: t })} />}

              {/* Media & Policy */}
              {activeTab === 3 && (
                <>
                  <ImageUploader
                    label="Cover Image"
                    imageUrl={coverImageUrl}
                    uploading={coverUploading}
                    onUpload={f => uploadImage(f, setCoverUploading, setCoverImageUrl)}
                    onRemove={() => setCoverImageUrl(null)}
                    hint="Displayed as the event card thumbnail · JPEG · PNG · WebP"
                  />
                  <ImageUploader
                    label="Banner Image"
                    imageUrl={bannerImageUrl}
                    uploading={bannerUploading}
                    onUpload={f => uploadImage(f, setBannerUploading, setBannerImageUrl)}
                    onRemove={() => setBannerImageUrl(null)}
                    hint="Wide hero image shown at the top of the event page · JPEG · PNG · WebP"
                  />
                  <div className="section-sep">Legal</div>
                  <div className="field"><label>Refund Policy</label><textarea rows={3} value={form.refund_policy} onChange={e => setF({ refund_policy: e.target.value })} placeholder="Describe the refund policy…" /></div>
                  <div className="field"><label>Terms & Conditions</label><textarea rows={3} value={form.terms_and_conditions} onChange={e => setF({ terms_and_conditions: e.target.value })} placeholder="Terms and conditions for attendees…" /></div>
                </>
              )}
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => closeModal()}>Cancel</button>
              <button className="btn btn-primary" onClick={submitForm} disabled={submitting}>{submitting ? 'Saving…' : editingId ? 'Save Changes' : 'Create Event'}</button>
            </div>
          </div>
        </div>
      )}


      {/* ── Cancel Modal ── */}
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

      {viewingId && (
        <EventDetailModal id={viewingId} onClose={() => setViewingId(null)} />
      )}
    </div>
  );
}

interface ReviewTier {
  name: string; price: number; capacity: number | null; is_visible: boolean;
}
interface ReviewEventDetail {
  id: string; title: string; status: string;
  description: string | null; start_time: string; end_time: string | null;
  expires_at: string | null; venue_name: string | null; address: string | null;
  city: string | null; price: number; currency: string;
  max_capacity: number | null; max_tickets_per_user: number | null;
  cover_image_url: string | null; banner_image_url: string | null;
  category_name: string | null; organiser_name: string | null;
  organiser_email: string | null; is_featured: boolean; tickets_sold: number;
}

function EventDetailModal({ id, onClose }: { id: string; onClose: () => void }) {
  const [data, setData] = useState<{ event: ReviewEventDetail; ticket_types: ReviewTier[] } | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let alive = true;
    api.get(`/admin/events/${id}/review`)
      .then(({ data: d }) => { if (alive) setData(d); })
      .catch(() => { if (alive) setFailed(true); });
    return () => { alive = false; };
  }, [id]);

  const ev = data?.event;
  const tiers = data?.ticket_types ?? [];

  return (
    <div className="modal-overlay" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="modal modal-lg" role="dialog" aria-modal="true">
        <div className="modal-header">
          <div>
            <div style={{
              fontSize: 10, fontWeight: 800, letterSpacing: '.12em',
              textTransform: 'uppercase', color: 'var(--text-3)',
            }}>
              Event record
            </div>
            <div className="modal-title">{ev?.title ?? 'Loading…'}</div>
          </div>
          <button className="modal-close" onClick={onClose}>×</button>
        </div>

        <div className="modal-body">
          {failed && <div className="alert alert-danger">Could not load this event.</div>}
          {!data && !failed && (
            <div style={{ textAlign: 'center', padding: 30 }}><div className="spinner spinner-lg" style={{ margin: '0 auto' }} /></div>
          )}

          {ev && (
            <>
              {(ev.banner_image_url || ev.cover_image_url) && (
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: ev.banner_image_url && ev.cover_image_url ? '2fr 1fr' : '1fr',
                  gap: 10, marginBottom: 18,
                }}>
                  {ev.banner_image_url && (
                    <div>
                      {/* eslint-disable-next-line @next/next/no-img-element */}
                      <img src={ev.banner_image_url} alt="Banner"
                        style={{ width: '100%', aspectRatio: '16/7', objectFit: 'cover', borderRadius: 'var(--r-lg)', display: 'block' }} />
                      <div style={{ fontSize: 11, color: 'var(--text-3)', marginTop: 4 }}>Banner</div>
                    </div>
                  )}
                  {ev.cover_image_url && (
                    <div>
                      {/* eslint-disable-next-line @next/next/no-img-element */}
                      <img src={ev.cover_image_url} alt="Cover"
                        style={{ width: '100%', aspectRatio: '16/9', objectFit: 'cover', borderRadius: 'var(--r-lg)', display: 'block' }} />
                      <div style={{ fontSize: 11, color: 'var(--text-3)', marginTop: 4 }}>Cover</div>
                    </div>
                  )}
                </div>
              )}

              <DRow label="Status" value={String(ev.status ?? '').replace(/_/g, ' ')} />
              <DRow label="Organiser" value={ev.organiser_name} />
              <DRow label="Organiser email" value={ev.organiser_email} />
              <DRow label="Category" value={ev.category_name} />
              <DRow label="Description" value={ev.description} wide />
              <DRow label="Starts" value={ev.start_time ? fmtDateTime(ev.start_time) : null} />
              <DRow label="Ends" value={ev.end_time ? fmtDateTime(ev.end_time) : null} />
              <DRow label="Sales close" value={ev.expires_at ? fmtDateTime(ev.expires_at) : null} />
              <DRow label="Venue" value={ev.venue_name} />
              <DRow label="Address" value={ev.address} />
              <DRow label="City" value={ev.city} />
              <DRow label="Price" value={ev.price > 0 ? fmtMoney(ev.price, ev.currency) : 'Free'} />
              <DRow label="Capacity" value={ev.max_capacity ? String(ev.max_capacity) : 'Unlimited'} />
              <DRow label="Max per buyer" value={ev.max_tickets_per_user ? String(ev.max_tickets_per_user) : 'No limit'} />
              <DRow label="Tickets sold" value={String(ev.tickets_sold ?? 0)} />
              <DRow label="Featured" value={ev.is_featured ? 'Yes' : 'No'} />
              <DRow label="Event ID" value={String(ev.id ?? '')} mono />

              <div style={{ marginTop: 18 }}>
                <div style={{
                  fontSize: 10.5, fontWeight: 800, letterSpacing: '.09em',
                  textTransform: 'uppercase', color: 'var(--text-3)', marginBottom: 8,
                }}>
                  Ticket types
                </div>
                {tiers.length === 0 ? (
                  <div style={{ fontSize: 13, color: 'var(--text-3)' }}>Flat pricing — no tiers.</div>
                ) : (
                  <table className="meta-table">
                    <thead>
                      <tr>
                        <th>Name</th>
                        <th style={{ textAlign: 'right' }}>Price</th>
                        <th style={{ textAlign: 'right' }}>Capacity</th>
                        <th>Visible</th>
                      </tr>
                    </thead>
                    <tbody>
                      {tiers.map((t, i) => (
                        <tr key={i}>
                          <td>{t.name}</td>
                          <td style={{ textAlign: 'right' }}>{t.price > 0 ? fmtMoney(t.price, ev.currency) : 'Free'}</td>
                          <td style={{ textAlign: 'right' }}>{t.capacity ?? '∞'}</td>
                          <td>{t.is_visible ? 'Yes' : 'Hidden'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            </>
          )}
        </div>

        <div className="modal-footer">
          <button className="btn" onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  );
}

function DRow({ label, value, mono = false, wide = false }: {
  label: string; value: unknown; mono?: boolean; wide?: boolean;
}) {
  const text = value == null || value === '' ? null : String(value);
  return (
    <div style={{
      display: wide ? 'block' : 'grid',
      gridTemplateColumns: wide ? undefined : '150px minmax(0, 1fr)',
      gap: wide ? 4 : 14, padding: '10px 0',
      borderBottom: '1px solid var(--border)',
    }}>
      <div style={{
        fontSize: 11, fontWeight: 700, letterSpacing: '.05em',
        textTransform: 'uppercase', color: 'var(--text-3)',
      }}>
        {label}
      </div>
      <div style={{
        fontSize: 13.5, lineHeight: 1.6,
        color: text ? 'var(--text)' : 'var(--text-faint)',
        fontFamily: mono ? 'var(--font-mono)' : undefined,
        whiteSpace: wide ? 'pre-wrap' : undefined,
        userSelect: mono ? 'all' : undefined,
      }}>
        {text ?? 'Not provided'}
      </div>
    </div>
  );
}