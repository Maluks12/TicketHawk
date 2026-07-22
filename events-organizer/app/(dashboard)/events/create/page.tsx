'use client';
import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '../../../../lib/api';
import { useToast } from '../../../../lib/toast';
import { validateRequired, validateMinLength, firstError } from '../../../../lib/validation';
import { getKycStatus } from '../../../../lib/kyc';

// ── Draft ─────────────────────────────────────────────────────────────────────
export const DRAFT_KEY = 'org_event_draft';

interface TicketTypeRow {
  name: string; price: number; capacity: string;
  sort_order: number; description: string; is_visible: boolean;
}

interface EventDraft {
  savedAt: string;
  title: string; description: string; categoryId: string;
  timezone: string; startTime: string; endTime: string; expiresAt: string;
  isFree: boolean; price: string; currency: string;
  maxCapacity: string; maxTicketsPerUser: string; allowWaitlist: boolean;
  venueName: string; address: string; city: string;
  venueType: 'auto' | 'stadium' | 'auditorium' | 'none';
  tags: string[]; performers: string[]; teams: string[];
  ticketTypes: TicketTypeRow[];
  refundPolicy: string; termsAndConditions: string;
  coverImageUrl: string | null; bannerImageUrl: string | null;
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60_000);
  const h = Math.floor(diff / 3_600_000);
  if (m < 1)  return 'just now';
  if (m < 60) return `${m}m ago`;
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

// ── TagInput ──────────────────────────────────────────────────────────────────
function TagInput({ tags, onChange }: { tags: string[]; onChange: (t: string[]) => void }) {
  const [val, setVal] = useState('');
  function commit() {
    const v = val.trim().replace(/,$/, '');
    if (v && !tags.includes(v)) onChange([...tags, v]);
    setVal('');
  }
  return (
    <div className="tag-input-wrap"
      onClick={e => (e.currentTarget.querySelector('input') as HTMLInputElement)?.focus()}>
      {tags.map((t, i) => (
        <span key={i} className="tag-pill">
          {t} <button type="button" onClick={() => onChange(tags.filter((_, j) => j !== i))}>✕</button>
        </span>
      ))}
      <input className="tag-bare-input" value={val} placeholder="Add…"
        onChange={e => setVal(e.target.value)}
        onBlur={commit}
        onKeyDown={e => {
          if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); commit(); }
          else if (e.key === 'Backspace' && !val) onChange(tags.slice(0, -1));
        }} />
    </div>
  );
}

// ── TicketTypeBuilder ─────────────────────────────────────────────────────────
function TicketTypeBuilder({ rows, onChange }: { rows: TicketTypeRow[]; onChange: (r: TicketTypeRow[]) => void }) {
  function add() {
    onChange([...rows, { name: '', price: 0, capacity: '', sort_order: rows.length, description: '', is_visible: true }]);
  }
  function upd(i: number, p: Partial<TicketTypeRow>) { onChange(rows.map((r, j) => j === i ? { ...r, ...p } : r)); }
  function del(i: number) { onChange(rows.filter((_, j) => j !== i)); }

  return (
    <>
      <div className="alert alert-info" style={{ marginBottom: '.75rem' }}>
        ℹ Ticket types define pricing tiers (e.g. General, VIP). Leave empty for flat pricing.
      </div>
      {rows.map((r, i) => (
        <div key={i} className="sub-section">
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '.6rem' }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '.04em' }}>
              Ticket Type {i + 1}
            </span>
            <button type="button" className="btn-icon" onClick={() => del(i)}
              style={{ fontSize: '.9rem', color: 'var(--danger, #c41c1c)' }}>✕</button>
          </div>
          <div className="field-row">
            <div className="field" style={{ margin: 0 }}>
              <label>Name *</label>
              <input type="text" value={r.name} style={inp}
                onChange={e => upd(i, { name: e.target.value })} placeholder="e.g. General Admission" />
            </div>
            <div className="field" style={{ margin: 0 }}>
              <label>Price (0 = free)</label>
              <input type="number" min="0" step="0.01" value={r.price} style={inp}
                onChange={e => upd(i, { price: parseFloat(e.target.value) || 0 })} />
            </div>
          </div>
          <div className="field-row" style={{ marginTop: '.5rem' }}>
            <div className="field" style={{ margin: 0 }}>
              <label>Capacity (blank = unlimited)</label>
              <input type="number" min="1" value={r.capacity} style={inp}
                onChange={e => upd(i, { capacity: e.target.value })} />
            </div>
            <div className="field" style={{ margin: 0 }}>
              <label>Sort Order</label>
              <input type="number" value={r.sort_order} style={inp}
                onChange={e => upd(i, { sort_order: parseInt(e.target.value) || 0 })} />
            </div>
          </div>
          <div className="field" style={{ marginTop: '.5rem', marginBottom: '.25rem' }}>
            <label>Description</label>
            <input type="text" value={r.description} style={inp} placeholder="Optional"
              onChange={e => upd(i, { description: e.target.value })} />
          </div>
          <label className="check-row" style={{ marginTop: '.25rem' }}>
            <input type="checkbox" checked={r.is_visible}
              onChange={e => upd(i, { is_visible: e.target.checked })} />
            <span style={{ fontSize: '.88rem' }}>Visible to buyers</span>
          </label>
        </div>
      ))}
      <button type="button" className="btn btn-sm" onClick={add}>+ Add Ticket Type</button>
    </>
  );
}

// ── ImageUploader ─────────────────────────────────────────────────────────────
function ImageUploader({
  label, imageUrl, uploading, onUpload, onRemove, hint,
}: {
  label: string; imageUrl: string | null; uploading: boolean;
  onUpload: (f: File) => void; onRemove: () => void; hint?: string;
}) {
  return (
    <div className="field">
      <label>{label}</label>
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: '.75rem' }}>
        {imageUrl ? (
          <div style={{ position: 'relative', width: 120, height: 80, flexShrink: 0 }}>
            <img src={imageUrl} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: 'var(--r)', border: '1px solid var(--border)', display: 'block' }}
              onError={e => { (e.target as HTMLImageElement).style.opacity = '0.3'; }} />
            <button type="button" onClick={onRemove}
              style={{ position: 'absolute', top: -6, right: -6, width: 20, height: 20, borderRadius: '50%', background: '#c41c1c', color: '#fff', border: 'none', cursor: 'pointer', fontSize: 10, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>✕</button>
          </div>
        ) : (
          <label style={{ width: 120, height: 80, border: '1px dashed var(--border-2)', borderRadius: 'var(--r)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', cursor: uploading ? 'wait' : 'pointer', color: 'var(--text-3)', fontSize: '.68rem', gap: '.25rem', background: 'var(--surface-2)', flexShrink: 0 }}>
            <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={uploading}
              onChange={e => { const f = e.target.files?.[0]; if (f) { onUpload(f); e.target.value = ''; } }} />
            {uploading
              ? <div className="spinner" style={{ width: 18, height: 18 }} />
              : <><span style={{ fontSize: '1.5rem', lineHeight: 1 }}>+</span><span>Upload</span></>}
          </label>
        )}
        <small style={{ color: 'var(--text-3)', fontSize: '.72rem', marginTop: '.25rem' }}>
          {hint ?? 'JPEG · PNG · WebP · uploaded to R2 immediately'}
        </small>
      </div>
    </div>
  );
}

// ── FormCard + helpers ────────────────────────────────────────────────────────
const inp: React.CSSProperties = {
  width: '100%', padding: '9px 12px', border: '1px solid var(--border-2)',
  borderRadius: 'var(--r)', fontSize: 14, color: 'var(--text)', background: 'var(--surface)',
  outline: 'none', fontFamily: 'inherit', boxSizing: 'border-box',
};

function Required() { return <span style={{ color: '#c41c1c', marginLeft: 2 }}>*</span>; }

function FormCard({ title, icon, children }: { title: string; icon: string; children: React.ReactNode }) {
  return (
    <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', overflow: 'hidden' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '12px 16px', borderBottom: '1px solid var(--border)', background: 'var(--surface-2)' }}>
        <span style={{ fontSize: 16 }}>{icon}</span>
        <span style={{ fontSize: 15, fontWeight: 700 }}>{title}</span>
      </div>
      <div style={{ padding: 16 }}>{children}</div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────
export default function CreateEventPage() {
  const router  = useRouter();
  const toast   = useToast();

  const [categories, setCategories]   = useState<{ id: number; name: string; slug: string }[]>([]);
  const [submitting, setSubmitting]   = useState(false);
  const [pendingDraft, setPendingDraft] = useState<EventDraft | null>(null);
  const [draftSavedAt, setDraftSavedAt] = useState<string | null>(null);

  // ── Text fields ───────────────────────────────────────────────────────────
  const [title,               setTitle]               = useState('');
  const [description,         setDescription]         = useState('');
  const [categoryId,          setCategoryId]          = useState('');
  const [timezone,            setTimezone]            = useState('Africa/Johannesburg');
  const [startTime,           setStartTime]           = useState('');
  const [endTime,             setEndTime]             = useState('');
  const [expiresAt,           setExpiresAt]           = useState('');
  // Scan (admission) window. 'default' keeps the 2h-before / 2h-after grace.
  // 'custom' exposes minute offsets for all-day or tight-door events.
  const [scanWindowMode, setScanWindowMode] = useState<'default' | 'custom'>('default');
  const [scanOpenMin,  setScanOpenMin]  = useState('120');   // minutes before start
  const [scanCloseMin, setScanCloseMin] = useState('120');   // minutes after end
  const [isFree,              setIsFree]              = useState(true);
  // Event class — drives visibility + what verification tier is needed.
  // private_free is the only class every organiser (tier 1) can publish.
  const [eventType, setEventType] = useState<'private_free' | 'private_paid' | 'public'>('private_free');
  const [kycLevel, setKycLevel] = useState<number>(1);
  const [price,               setPrice]               = useState('0');
  const [currency,            setCurrency]            = useState('ZAR');
  const [maxCapacity,         setMaxCapacity]         = useState('');
  const [maxTicketsPerUser,   setMaxTicketsPerUser]   = useState('');
  const [allowWaitlist,       setAllowWaitlist]       = useState(false);
  const [venueName,           setVenueName]           = useState('');
  const [address,             setAddress]             = useState('');
  const [city,                setCity]                = useState('');
  const [venueType,           setVenueType]           = useState<'auto'|'stadium'|'auditorium'|'none'>('auto');
  const [tags,                setTags]                = useState<string[]>([]);
  const [performers,          setPerformers]          = useState<string[]>([]);
  const [teams,               setTeams]               = useState<string[]>([]);
  const [ticketTypes,         setTicketTypes]         = useState<TicketTypeRow[]>([]);
  const [refundPolicy,        setRefundPolicy]        = useState('');
  const [termsAndConditions,  setTermsAndConditions]  = useState('');

  // ── Image state ───────────────────────────────────────────────────────────
  const [coverImageUrl,   setCoverImageUrl]   = useState<string | null>(null);
  const [bannerImageUrl,  setBannerImageUrl]  = useState<string | null>(null);
  const [coverUploading,  setCoverUploading]  = useState(false);
  const [bannerUploading, setBannerUploading] = useState(false);

  // ── Load categories ───────────────────────────────────────────────────────
  useEffect(() => {
    api.get('/categories')
      .then(r => setCategories(r.data.categories ?? r.data ?? []))
      .catch(() => {});
  }, []);

  // ── Check for saved draft on mount ────────────────────────────────────────
  useEffect(() => {
    try {
      const raw = localStorage.getItem(DRAFT_KEY);
      if (!raw) return;
      const d = JSON.parse(raw) as EventDraft;
      if (d.title || d.description || d.venueName) setPendingDraft(d);
    } catch { /* corrupt data — ignore */ }
  }, []);

  // ── Auto-save draft (debounced 800 ms) ────────────────────────────────────
  useEffect(() => {
    if (submitting) return;
    if (!title && !description && !venueName) return; // nothing worth saving
    const t = setTimeout(() => {
      const draft: EventDraft = {
        savedAt: new Date().toISOString(),
        title, description, categoryId, timezone, startTime, endTime, expiresAt,
        isFree, price, currency, maxCapacity, maxTicketsPerUser, allowWaitlist,
        venueName, address, city, venueType, tags, performers, teams, ticketTypes,
        refundPolicy, termsAndConditions, coverImageUrl, bannerImageUrl,
      };
      try {
        localStorage.setItem(DRAFT_KEY, JSON.stringify(draft));
        setDraftSavedAt(draft.savedAt);
      } catch { /* storage quota */ }
    }, 800);
    return () => clearTimeout(t);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [title, description, categoryId, timezone, startTime, endTime, expiresAt,
      isFree, price, currency, maxCapacity, maxTicketsPerUser, allowWaitlist,
      venueName, address, city, venueType, tags, performers, teams, ticketTypes,
      refundPolicy, termsAndConditions, coverImageUrl, bannerImageUrl, submitting]);

  // ── Restore draft ─────────────────────────────────────────────────────────
  function restoreDraft(d: EventDraft) {
    setTitle(d.title ?? '');
    setDescription(d.description ?? '');
    setCategoryId(d.categoryId ?? '');
    setTimezone(d.timezone ?? 'Africa/Johannesburg');
    setStartTime(d.startTime ?? '');
    setEndTime(d.endTime ?? '');
    setExpiresAt(d.expiresAt ?? '');
    setIsFree(d.isFree ?? true);
    setPrice(d.price ?? '0');
    setCurrency(d.currency ?? 'ZAR');
    setMaxCapacity(d.maxCapacity ?? '');
    setMaxTicketsPerUser(d.maxTicketsPerUser ?? '');
    setAllowWaitlist(d.allowWaitlist ?? false);
    setVenueName(d.venueName ?? '');
    setAddress(d.address ?? '');
    setCity(d.city ?? '');
    setVenueType(d.venueType ?? 'auto');
    setTags(d.tags ?? []);
    setPerformers(d.performers ?? []);
    setTeams(d.teams ?? []);
    setTicketTypes(d.ticketTypes ?? []);
    setRefundPolicy(d.refundPolicy ?? '');
    setTermsAndConditions(d.termsAndConditions ?? '');
    setCoverImageUrl(d.coverImageUrl ?? null);
    setBannerImageUrl(d.bannerImageUrl ?? null);
    setDraftSavedAt(d.savedAt);
    setPendingDraft(null);
  }

  function discardDraft() {
    localStorage.removeItem(DRAFT_KEY);
    setPendingDraft(null);
  }

  // ── Image upload — calls Go backend directly (same pattern as admin) ─────
  async function uploadImage(
    file: File,
    setUploading: (v: boolean) => void,
    onSuccess: (url: string) => void,
  ) {
    setUploading(true);
    try {
      const fd    = new FormData();
      fd.append('image', file);
      const token   = typeof window !== 'undefined' ? localStorage.getItem('evt_token') : null;
      const baseURL = (process.env.NEXT_PUBLIC_API_URL || '').replace(/\/$/, '');
      const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};
      const res  = await fetch(`${baseURL}/organiser/upload`, { method: 'POST', headers, body: fd });
      const json = await res.json().catch(() => null) as { image_url?: string; error?: string } | null;
      if (!res.ok) throw new Error(json?.error ?? `Upload failed (HTTP ${res.status})`);
      const url = json?.image_url ?? null;
      if (url) { onSuccess(url); toast('Image uploaded', 'success'); }
      else toast('Upload succeeded but no URL returned', 'error');
    } catch (e: unknown) {
      toast((e instanceof Error ? e.message : null) ?? 'Image upload failed', 'error');
    } finally { setUploading(false); }
  }

  // ── Validation ────────────────────────────────────────────────────────────
  function validate() {
    return firstError({
      title:       validateMinLength(title, 5, 'Title'),
      description: validateMinLength(description, 50, 'Description'),
      category:    validateRequired(categoryId, 'Category'),
      venueName:   validateRequired(venueName, 'Venue name'),
      address:     validateRequired(address, 'Address'),
      city:        validateRequired(city, 'City'),
      startTime:   validateRequired(startTime, 'Start time'),
      endTime:     validateRequired(endTime, 'End time'),
      expiresAt:   validateRequired(expiresAt, 'Booking deadline'),
    });
  }

  // ── Submit ────────────────────────────────────────────────────────────────
  useEffect(() => {
    getKycStatus().then((k) => setKycLevel(k.kyc_level)).catch(() => {});
  }, []);

  // Choosing "private free" locks pricing to free — the backend enforces this
  // at publish, the UI just keeps the two controls from contradicting.
  useEffect(() => {
    if (eventType === 'private_free') setIsFree(true);
  }, [eventType]);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const err = validate();
    if (err) { toast(err, 'error'); return; }
    if (new Date(endTime) <= new Date(startTime)) {
      toast('End time must be after start time', 'error'); return;
    }
    const hasPaidTier = ticketTypes.some(t => t.name && Number(t.price) > 0);
    if (eventType === 'private_free' && (!isFree || hasPaidTier)) {
      toast('Private free events can\'t have paid tickets — switch the event type or make tickets free', 'error'); return;
    }
    if (eventType !== 'private_free' && kycLevel < 2) {
      toast('Paid and public events need full verification first', 'error');
      router.push('/kyc'); return;
    }
    setSubmitting(true);
    try {
      const body: Record<string, unknown> = {
        title:        title.trim(),
        description:  description.trim(),
        category_id:  parseInt(categoryId),
        venue_name:   venueName.trim(),
        address:      address.trim(),
        city:         city.trim(),
        timezone,
        start_time:   new Date(startTime).toISOString(),
        end_time:     new Date(endTime).toISOString(),
        expires_at:   new Date(expiresAt).toISOString(),
        price:        isFree ? 0 : parseFloat(price) || 0,
        currency,
        allow_waitlist:       allowWaitlist,
        tags,
        performers,
        teams,
        refund_policy:        refundPolicy.trim()       || null,
        terms_and_conditions: termsAndConditions.trim() || null,
        visibility:           eventType === 'public' ? 'public' : 'private_link',
      };
      if (scanWindowMode === 'custom') {
        body.scan_open_offset_min  = parseInt(scanOpenMin, 10)  || 0;
        body.scan_close_offset_min = parseInt(scanCloseMin, 10) || 0;
      }
      // Note: backend DTO spells this "max_capicity" (typo kept intentionally to match backend)
      if (maxCapacity)       body.max_capicity          = parseInt(maxCapacity);
      if (maxTicketsPerUser) body.max_tickets_per_user  = parseInt(maxTicketsPerUser);
      if (venueType !== 'auto') body.venue_type         = venueType;
      if (coverImageUrl)     body.cover_image_url       = coverImageUrl;
      if (bannerImageUrl)    body.banner_image_url      = bannerImageUrl;
      if (ticketTypes.length) {
        body.ticket_types = ticketTypes.filter(t => t.name).map(t => ({
          name: t.name, price: t.price, sort_order: t.sort_order, is_visible: t.is_visible,
          ...(t.capacity    ? { capacity:    parseInt(t.capacity) } : {}),
          ...(t.description ? { description: t.description }       : {}),
        }));
      }

      await api.post('/organiser/events/create', body);
      localStorage.removeItem(DRAFT_KEY);
      toast('Event submitted for review', 'success');
      router.push('/events');
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
        ?? 'Failed to create event';
      toast(msg, 'error');
    } finally { setSubmitting(false); }
  }

  // ── Checklist ─────────────────────────────────────────────────────────────
  const checklist = [
    { done: title.length >= 5,               label: 'Add a title (5+ chars)' },
    { done: description.length >= 50,        label: 'Write a description (50+ chars)' },
    { done: !!categoryId,                    label: 'Choose a category' },
    { done: !!venueName && !!address && !!city,  label: 'Set venue and city' },
    { done: !!startTime && !!endTime,        label: 'Set start and end times' },
    { done: !!expiresAt,                     label: 'Set a booking deadline' },
    { done: isFree || parseFloat(price) > 0, label: 'Set ticket pricing' },
    { done: !!coverImageUrl,                 label: 'Upload a cover image' },
  ];

  // ─────────────────────────────────────────────────────────────────────────
  return (
    <div className="page">
      <div className="page-header">
        <div>
          <div style={{ fontSize: 12, color: 'var(--text-2)', marginBottom: 4 }}>
            <span style={{ cursor: 'pointer' }} onClick={() => router.push('/events')}>My Events</span>
            {' → '}Create event
          </div>
          <div className="page-title">Create a new event</div>
          <div style={{ fontSize: 13, color: 'var(--text-2)', marginTop: 2 }}>
            Events are created as drafts and require admin approval before going live.
          </div>
        </div>
      </div>

      <div className="page-content">

        {/* ── Draft restore banner ─────────────────────────────────────── */}
        {pendingDraft && (
          <div style={{
            display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap',
            padding: '12px 16px', marginBottom: 16,
            background: '#fff8e1', border: '1px solid #fcd34d',
            borderRadius: 'var(--r-lg)', color: '#92400e',
          }}>
            <span style={{ fontSize: 18 }}>📝</span>
            <div style={{ flex: 1, minWidth: 180 }}>
              <strong style={{ fontSize: 14 }}>
                {pendingDraft.title ? `"${pendingDraft.title}"` : 'Untitled draft'}
              </strong>
              <span style={{ fontSize: 13, marginLeft: 8 }}>
                — saved {relativeTime(pendingDraft.savedAt)}
              </span>
            </div>
            <div style={{ display: 'flex', gap: 8 }}>
              <button className="btn btn-sm btn-primary"
                onClick={() => restoreDraft(pendingDraft)}>
                Restore draft
              </button>
              <button className="btn btn-sm" onClick={discardDraft}>
                Start fresh
              </button>
            </div>
          </div>
        )}

        <form onSubmit={handleSubmit} noValidate>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 300px', gap: 20, maxWidth: 980 }}>

            {/* ── Left column ───────────────────────────────────────────── */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>

              {/* Event details */}
              <FormCard title="Event details" icon="📋">
                <div className="field">
                  <label>Event title <Required /></label>
                  <input value={title} onChange={e => setTitle(e.target.value)}
                    placeholder="e.g. Summer Music Festival 2025" style={inp} />
                  <div className="field-hint">{title.length}/255 · minimum 5 characters</div>
                </div>
                <div className="field" style={{ marginBottom: 0 }}>
                  <label>Description <Required /></label>
                  <textarea value={description} onChange={e => setDescription(e.target.value)}
                    placeholder="Describe your event in detail — what attendees can expect, lineup, dress code, etc. (minimum 50 characters)"
                    rows={5} style={{ ...inp, resize: 'vertical' }} />
                  <div className="field-hint">{description.length}/∞ · minimum 50 characters</div>
                </div>
              </FormCard>

              {/* Date & time */}
              <FormCard title="Date & time" icon="📅">
                <div className="field">
                  <label>Timezone</label>
                  <select value={timezone} onChange={e => setTimezone(e.target.value)} style={inp}>
                    <option value="Africa/Johannesburg">Africa/Johannesburg (SAST)</option>
                    <option value="Africa/Cape_Town">Africa/Cape_Town</option>
                    <option value="UTC">UTC</option>
                    <option value="Europe/London">Europe/London</option>
                    <option value="America/New_York">America/New_York</option>
                    <option value="America/Los_Angeles">America/Los_Angeles</option>
                    <option value="Asia/Dubai">Asia/Dubai</option>
                  </select>
                </div>
                <div className="field-row">
                  <div className="field">
                    <label>Start time <Required /></label>
                    <input type="datetime-local" value={startTime}
                      onChange={e => setStartTime(e.target.value)} style={inp} />
                  </div>
                  <div className="field">
                    <label>End time <Required /></label>
                    <input type="datetime-local" value={endTime}
                      onChange={e => setEndTime(e.target.value)} style={inp} />
                  </div>
                </div>
                <div className="field" style={{ marginBottom: 0 }}>
                  <label>Booking deadline <Required /></label>
                  <input type="datetime-local" value={expiresAt}
                    onChange={e => setExpiresAt(e.target.value)} style={inp} />
                  <div className="field-hint">Tickets stop selling after this date</div>
                </div>

                <div style={{ marginTop: 16 }}>
                  <label>Ticket scanning window</label>
                  <select value={scanWindowMode}
                    onChange={e => setScanWindowMode(e.target.value as 'default' | 'custom')}
                    style={inp}>
                    <option value="default">Default — opens 2 hours before, closes 2 hours after</option>
                    <option value="custom">Custom — set my own window</option>
                  </select>
                  <div className="field-hint">
                    When staff can scan tickets at the door. All-day events may want a wider window.
                  </div>

                  {scanWindowMode === 'custom' && (
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 10 }}>
                      <div>
                        <label style={{ fontSize: 12.5 }}>Opens before start (minutes)</label>
                        <input type="number" min={0} max={10080} value={scanOpenMin}
                          onChange={e => setScanOpenMin(e.target.value)} style={inp}
                          placeholder="120" />
                        <div className="field-hint">e.g. 120 = 2 hours</div>
                      </div>
                      <div>
                        <label style={{ fontSize: 12.5 }}>Closes after end (minutes)</label>
                        <input type="number" min={0} max={10080} value={scanCloseMin}
                          onChange={e => setScanCloseMin(e.target.value)} style={inp}
                          placeholder="120" />
                        <div className="field-hint">e.g. 720 = 12 hours (all-day)</div>
                      </div>
                    </div>
                  )}
                </div>
              </FormCard>

              {/* Location */}
              <FormCard title="Location" icon="📍">
                <div className="field">
                  <label>Venue name <Required /></label>
                  <input value={venueName} onChange={e => setVenueName(e.target.value)}
                    placeholder="e.g. FNB Stadium" style={inp} />
                </div>
                <div className="field">
                  <label>Street address <Required /></label>
                  <input value={address} onChange={e => setAddress(e.target.value)}
                    placeholder="e.g. 1 Soccer City Ave" style={inp} />
                </div>
                <div className="field">
                  <label>City <Required /></label>
                  <input value={city} onChange={e => setCity(e.target.value)}
                    placeholder="e.g. Johannesburg" style={inp} />
                </div>
                <div className="field" style={{ marginBottom: 0 }}>
                  <label>Venue type <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>(controls seating map in app)</small></label>
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 6 }}>
                    {([
                      { value: 'auto',       emoji: '⚡', label: 'Auto',          hint: 'Guess from category & name' },
                      { value: 'stadium',    emoji: '🏟️', label: 'Stadium',        hint: 'Stadium section map' },
                      { value: 'auditorium', emoji: '🎭', label: 'Hall / Theatre', hint: 'Auditorium seating map' },
                      { value: 'none',       emoji: '🎫', label: 'None',           hint: 'No map — straight to checkout' },
                    ] as const).map(o => (
                      <button key={o.value} type="button" title={o.hint}
                        onClick={() => setVenueType(o.value)}
                        style={{
                          padding: '8px 14px', borderRadius: 'var(--r)', cursor: 'pointer', fontSize: 13,
                          fontWeight: venueType === o.value ? 600 : 400,
                          border: venueType === o.value ? '2px solid var(--blue)' : '1px solid var(--border)',
                          background: venueType === o.value ? 'var(--blue-light)' : 'var(--surface)',
                          color: venueType === o.value ? 'var(--blue)' : 'var(--text-2)',
                          transition: 'all .15s',
                        }}>
                        {o.emoji} {o.label}
                      </button>
                    ))}
                  </div>
                </div>
              </FormCard>

              {/* Tickets & pricing */}
              <FormCard title="Tickets & pricing" icon="🎟">
                <div className="field">
                  <label>Category <Required /></label>
                  <select value={categoryId} onChange={e => setCategoryId(e.target.value)} style={inp}>
                    <option value="">— Select a category —</option>
                    {categories.length > 0
                      ? categories.map(c => <option key={c.id} value={c.id}>{c.name}</option>)
                      : [['1','Music'],['2','Sports'],['3','Arts & Culture'],['4','Food & Drink'],
                         ['5','Technology'],['6','Business'],['7','Health & Wellness'],
                         ['8','Education'],['9','Comedy'],['10','Other'],
                        ].map(([v, l]) => <option key={v} value={v}>{l}</option>)
                    }
                  </select>
                </div>

                <div style={{ display: 'flex', gap: 12, marginBottom: 16 }}>
                  {[true, false].map(free => (
                    <label key={String(free)} style={{
                      display: 'flex', alignItems: 'center', gap: 8, padding: '10px 14px',
                      border: `2px solid ${isFree === free ? 'var(--blue)' : 'var(--border)'}`,
                      borderRadius: 'var(--r)',
                      background: isFree === free ? 'var(--blue-light)' : 'var(--surface)',
                      cursor: 'pointer', flex: 1,
                    }}>
                      <input type="radio" name="pricing" checked={isFree === free}
                        onChange={() => setIsFree(free)} style={{ accentColor: 'var(--blue)' }} />
                      <div>
                        <div style={{ fontWeight: 600, fontSize: 14 }}>{free ? 'Free event' : 'Paid event'}</div>
                        <div style={{ fontSize: 12, color: 'var(--text-2)' }}>
                          {free ? 'No ticket charge' : 'Set a ticket price'}
                        </div>
                      </div>
                    </label>
                  ))}
                </div>

                {!isFree && (
                  <div className="field-row">
                    <div className="field">
                      <label>Base price <Required /></label>
                      <div style={{ position: 'relative' }}>
                        <span style={{ position: 'absolute', left: 12, top: '50%', transform: 'translateY(-50%)', color: 'var(--text-2)', fontWeight: 600 }}>R</span>
                        <input type="number" min="0" step="0.01" value={price}
                          onChange={e => setPrice(e.target.value)}
                          style={{ ...inp, paddingLeft: 28 }} />
                      </div>
                    </div>
                    <div className="field">
                      <label>Currency</label>
                      <select value={currency} onChange={e => setCurrency(e.target.value)} style={inp}>
                        <option value="ZAR">ZAR — South African Rand</option>
                        <option value="USD">USD — US Dollar</option>
                        <option value="EUR">EUR — Euro</option>
                        <option value="GBP">GBP — British Pound</option>
                        <option value="KES">KES — Kenyan Shilling</option>
                        <option value="NGN">NGN — Nigerian Naira</option>
                      </select>
                    </div>
                  </div>
                )}

                <div className="field-row">
                  <div className="field" style={{ marginBottom: 0 }}>
                    <label>Max capacity <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>(blank = unlimited)</small></label>
                    <input type="number" min="1" value={maxCapacity}
                      onChange={e => setMaxCapacity(e.target.value)} placeholder="Unlimited" style={inp} />
                  </div>
                  <div className="field" style={{ marginBottom: 0 }}>
                    <label>Max per user <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>(blank = no limit)</small></label>
                    <input type="number" min="1" max="20" value={maxTicketsPerUser}
                      onChange={e => setMaxTicketsPerUser(e.target.value)} placeholder="No limit" style={inp} />
                  </div>
                </div>

                <label className="check-row" style={{ marginTop: 12, marginBottom: 0 }}>
                  <input type="checkbox" checked={allowWaitlist}
                    onChange={e => setAllowWaitlist(e.target.checked)} />
                  <span style={{ fontSize: '.88rem' }}>Allow waitlist when sold out</span>
                </label>

                <div style={{ marginTop: 20 }}>
                  <div className="section-sep" style={{ marginTop: 0 }}>Ticket Types</div>
                  <TicketTypeBuilder rows={ticketTypes} onChange={setTicketTypes} />
                </div>
              </FormCard>

              {/* Tags & people */}
              <FormCard title="Tags & people" icon="🏷">
                <div className="field">
                  <label>Tags <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>(press Enter to add)</small></label>
                  <TagInput tags={tags} onChange={setTags} />
                </div>
                <div className="field">
                  <label>Performers</label>
                  <TagInput tags={performers} onChange={setPerformers} />
                </div>
                <div className="field" style={{ marginBottom: 0 }}>
                  <label>Teams</label>
                  <TagInput tags={teams} onChange={setTeams} />
                </div>
              </FormCard>

              {/* Media & policy */}
              <FormCard title="Media & policy" icon="🖼">
                <ImageUploader
                  label="Cover image"
                  imageUrl={coverImageUrl}
                  uploading={coverUploading}
                  onUpload={f => uploadImage(f, setCoverUploading, setCoverImageUrl)}
                  onRemove={() => setCoverImageUrl(null)}
                  hint="Event card thumbnail · JPEG · PNG · WebP · max 10 MB"
                />
                <ImageUploader
                  label="Banner image"
                  imageUrl={bannerImageUrl}
                  uploading={bannerUploading}
                  onUpload={f => uploadImage(f, setBannerUploading, setBannerImageUrl)}
                  onRemove={() => setBannerImageUrl(null)}
                  hint="Wide hero image at the top of the event page · JPEG · PNG · WebP · max 10 MB"
                />
                <div className="section-sep">Legal</div>
                <div className="field">
                  <label>Refund policy</label>
                  <textarea rows={3} value={refundPolicy}
                    onChange={e => setRefundPolicy(e.target.value)}
                    placeholder="Describe the refund policy…"
                    style={{ ...inp, resize: 'vertical' }} />
                </div>
                <div className="field" style={{ marginBottom: 0 }}>
                  <label>Terms &amp; conditions</label>
                  <textarea rows={3} value={termsAndConditions}
                    onChange={e => setTermsAndConditions(e.target.value)}
                    placeholder="Terms and conditions for attendees…"
                    style={{ ...inp, resize: 'vertical' }} />
                </div>
              </FormCard>

            </div>{/* end left column */}

            {/* ── Right column: sticky checklist ────────────────────────── */}
            <div>
              <div style={{
                background: 'var(--surface)', border: '1px solid var(--border)',
                borderRadius: 'var(--r-lg)', padding: 16, position: 'sticky', top: 16,
              }}>
                <div style={{ fontSize: 14, fontWeight: 700, marginBottom: 8 }}>Event type</div>
                <select className="input" value={eventType}
                  onChange={(e) => {
                    const v = e.target.value as typeof eventType;
                    if (v !== 'private_free' && kycLevel < 2) { router.push('/kyc'); return; }
                    setEventType(v);
                  }}
                  style={{ width: '100%', padding: '8px 10px', border: '1px solid var(--border)', borderRadius: 6, fontSize: 13.5, marginBottom: 8 }}>
                  <option value="private_free">Private free event</option>
                  <option value="private_paid" disabled={kycLevel < 2}>
                    Private paid event{kycLevel < 2 ? ' — needs full verification' : ''}
                  </option>
                  <option value="public" disabled={kycLevel < 2}>
                    Public event{kycLevel < 2 ? ' — needs full verification' : ''}
                  </option>
                </select>
                <p style={{ fontSize: 12, color: 'var(--text-3)', margin: '0 0 8px', lineHeight: 1.5 }}>
                  {eventType === 'private_free' && 'Invite-only via a shareable link. Free tickets only. Goes live immediately — no admin review.'}
                  {eventType === 'private_paid' && 'Invite-only via a shareable link, with paid tickets. Goes live immediately — no admin review. Requires full verification and payout bank details.'}
                  {eventType === 'public' && 'Listed in the TicketHawk feed and search for everyone. Requires full verification and admin review before going live.'}
                </p>
                {kycLevel < 2 && (
                  <button type="button" className="btn btn-sm" style={{ width: '100%', marginBottom: 12 }}
                    onClick={() => router.push('/kyc')}>
                    🔒 Unlock paid &amp; public — verify your ID
                  </button>
                )}
                <div style={{ height: 1, background: 'var(--border)', margin: '4px 0 12px' }} />
                <div style={{ fontSize: 14, fontWeight: 700, marginBottom: 12 }}>Before you submit</div>
                {checklist.map(item => (
                  <div key={item.label} style={{ display: 'flex', gap: 8, alignItems: 'flex-start', marginBottom: 8 }}>
                    <span style={{
                      width: 16, height: 16, borderRadius: '50%', flexShrink: 0, marginTop: 1,
                      background: item.done ? '#1a7f37' : 'var(--surface-3)',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                      fontSize: 10, color: item.done ? '#fff' : 'var(--text-3)',
                    }}>
                      {item.done ? '✓' : '·'}
                    </span>
                    <span style={{
                      fontSize: 13,
                      color: item.done ? 'var(--text-3)' : 'var(--text)',
                      textDecoration: item.done ? 'line-through' : 'none',
                    }}>
                      {item.label}
                    </span>
                  </div>
                ))}
                <div style={{ height: 1, background: 'var(--border)', margin: '12px 0' }} />
                <div className="alert alert-info" style={{ fontSize: 12, padding: '8px 10px', marginBottom: 0 }}>
                  <svg viewBox="0 0 20 20" fill="currentColor" width="14" height="14" style={{ flexShrink: 0 }}>
                    <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd"/>
                  </svg>
                  <span>{eventType === 'public'
                    ? 'Public events need admin approval before going live. You\u2019ll be notified once approved.'
                    : 'Private events go live immediately after you publish \u2014 no admin review.'}</span>
                </div>
              </div>
            </div>

          </div>{/* end grid */}

          {/* ── Footer ───────────────────────────────────────────────────── */}
          <div style={{
            display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap',
            marginTop: 20, paddingTop: 20, borderTop: '1px solid var(--border)', maxWidth: 640,
          }}>
            <button type="button" className="btn" disabled={submitting}
              onClick={() => router.push('/events')}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={submitting}>
              {submitting ? 'Submitting…' : eventType === 'public' ? 'Submit for review' : 'Create event'}
            </button>
            {draftSavedAt && !submitting && (
              <span style={{ fontSize: 12, color: 'var(--text-3)', marginLeft: 4 }}>
                💾 Draft saved {relativeTime(draftSavedAt)}
              </span>
            )}
          </div>

        </form>
      </div>
    </div>
  );
}