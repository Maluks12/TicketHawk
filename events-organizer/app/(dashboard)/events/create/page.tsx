'use client';
import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import {
  Article, CalendarBlank, MapPin, Ticket, Tag, ImageSquare,
  MagicWand, Vault, MaskHappy, Confetti,
  Trash, Plus, X, Check, CheckCircle, Circle,
  FloppyDisk, NotePencil, LockKey, UploadSimple, Info, SealCheck,
  type Icon as PhosphorIcon,
} from '@phosphor-icons/react';
import { api } from '../../../../lib/api';
import { useToast } from '../../../../lib/toast';
import { validateRequired, validateMinLength, firstError } from '../../../../lib/validation';
import { getKycStatus } from '../../../../lib/kyc';
import PlacesAutocomplete, { PlaceResult } from '@/components/placesAutocomplete';
import DateTimePicker from '@/components/dateTimePicker';

// ─────────────────────────────────────────────────────────────────────────────
// UI-ONLY REWRITE
//
// Every hook, handler, validation rule and request body below the render is
// byte-identical to the previous version. What changed is presentation:
//
//   • Emoji swapped for Phosphor icons (MIT licensed, and the same family
//     EventsApp already uses via phosphor-react-native — so the dashboard and
//     the phone apps finally draw from one icon set).
//   • The form was capped at `maxWidth: 980` with a 300px sidebar, leaving the
//     inputs ~660px wide on a 1900px monitor. It now runs to 1440 with a 360px
//     rail, and short fields sit two- and three-up instead of stacking into one
//     tall column of full-width boxes.
//   • Inputs were painted `var(--surface)` — the exact colour of the card
//     behind them — so every field was an invisible rectangle you had to guess
//     at. They now use the recessed alpha surface with a focus ring.
//   • The Legal section (refund policy, T&Cs) is gone from the form, as asked.
//     The STATE and the submit body are untouched, so nothing else breaks and
//     restoring the section is a UI-only change. See README for the caveat.
// ─────────────────────────────────────────────────────────────────────────────

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
  // Google Places selection saved with the draft, so restoring a draft keeps
  // the venue pinned rather than silently falling back to a text search.
  // Optional: drafts saved before this feature existed won't have them.
  placeId?: string; placeLabel?: string;
  latitude?: number | null; longitude?: number | null;
  venueType: 'auto' | 'stadium' | 'auditorium' | 'none';
  tags: string[]; performers: string[]; teams: string[];
  ticketTypes: TicketTypeRow[];
  // Retained in the draft shape even though the form no longer shows them, so
  // a draft saved by the previous version still round-trips without throwing.
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

// ── Shared field styling ──────────────────────────────────────────────────────
// Exported as an object rather than a class because DateTimePicker and
// PlacesAutocomplete both take a `style` prop — they can't read a className.
const inp: React.CSSProperties = {
  width: '100%', padding: '11px 13px',
  border: '1px solid var(--border-2)', borderRadius: 'var(--r)',
  fontSize: 14, color: 'var(--text)',
  // Recessed, NOT var(--surface). On the old page the input background matched
  // the card exactly, so fields read as empty space with a hairline round them.
  background: 'var(--surface-alpha)',
  outline: 'none', fontFamily: 'inherit', boxSizing: 'border-box',
  transition: 'border-color .15s ease, box-shadow .15s ease, background .15s ease',
};

function Required() {
  return <span style={{ color: 'var(--danger)', marginLeft: 3 }} aria-hidden>*</span>;
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
          {t}
          <button type="button" aria-label={`Remove ${t}`}
            onClick={() => onChange(tags.filter((_, j) => j !== i))}>
            <X size={10} weight="bold" />
          </button>
        </span>
      ))}
      <input className="tag-bare-input" value={val} placeholder="Type and press Enter…"
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
      <div className="alert alert-info" style={{ marginBottom: 14 }}>
        <Info size={17} weight="fill" style={{ flexShrink: 0, marginTop: 1 }} />
        <span>Ticket types define pricing tiers (e.g. General, VIP). Leave empty for flat pricing.</span>
      </div>

      {rows.map((r, i) => (
        <div key={i} className="sub-section">
          <div style={{
            display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            marginBottom: 14,
          }}>
            <span style={{
              display: 'inline-flex', alignItems: 'center', gap: 7,
              fontSize: 10.5, fontWeight: 800, color: 'var(--text-3)',
              textTransform: 'uppercase', letterSpacing: '.1em',
            }}>
              <Ticket size={14} weight="bold" />
              Ticket type {i + 1}
            </span>
            <button type="button" className="btn-icon" onClick={() => del(i)}
              aria-label={`Remove ticket type ${i + 1}`}
              style={{ color: 'var(--danger-fg)' }}>
              <Trash size={15} />
            </button>
          </div>

          {/* Four short fields across, rather than two rows of two. The old
              660px column couldn't fit them; the wider one can. */}
          <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr 1fr', gap: 12 }}>
            <div className="field" style={{ margin: 0 }}>
              <label>Name <Required /></label>
              <input type="text" value={r.name} style={inp}
                onChange={e => upd(i, { name: e.target.value })} placeholder="e.g. General Admission" />
            </div>
            <div className="field" style={{ margin: 0 }}>
              <label>Price</label>
              <input type="number" min="0" step="0.01" value={r.price} style={inp}
                onChange={e => upd(i, { price: parseFloat(e.target.value) || 0 })} />
              <div className="field-hint">0 = free</div>
            </div>
            <div className="field" style={{ margin: 0 }}>
              <label>Capacity</label>
              <input type="number" min="1" value={r.capacity} style={inp}
                onChange={e => upd(i, { capacity: e.target.value })} placeholder="∞" />
              <div className="field-hint">Blank = unlimited</div>
            </div>
            <div className="field" style={{ margin: 0 }}>
              <label>Sort order</label>
              <input type="number" value={r.sort_order} style={inp}
                onChange={e => upd(i, { sort_order: parseInt(e.target.value) || 0 })} />
            </div>
          </div>

          <div style={{
            display: 'grid', gridTemplateColumns: 'minmax(0,1fr) auto',
            gap: 16, alignItems: 'end', marginTop: 12,
          }}>
            <div className="field" style={{ margin: 0 }}>
              <label>Description</label>
              <input type="text" value={r.description} style={inp} placeholder="Optional — what this tier includes"
                onChange={e => upd(i, { description: e.target.value })} />
            </div>
            <label className="check-row" style={{ paddingBottom: 11, whiteSpace: 'nowrap' }}>
              <input type="checkbox" checked={r.is_visible}
                onChange={e => upd(i, { is_visible: e.target.checked })} />
              <span style={{ fontSize: 13 }}>Visible to buyers</span>
            </label>
          </div>
        </div>
      ))}

      <button type="button" className="btn btn-sm" onClick={add}>
        <Plus size={14} weight="bold" />
        Add ticket type
      </button>
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
    <div>
      <label style={{
        display: 'block', fontSize: 11, fontWeight: 700,
        letterSpacing: '.07em', textTransform: 'uppercase',
        color: 'var(--text-3)', marginBottom: 8,
      }}>
        {label}
      </label>

      {imageUrl ? (
        <div style={{ position: 'relative' }}>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src={imageUrl} alt=""
            style={{
              width: '100%', aspectRatio: '16 / 9', objectFit: 'cover',
              borderRadius: 'var(--r-lg)', border: '1px solid var(--border)', display: 'block',
            }}
            onError={e => { (e.target as HTMLImageElement).style.opacity = '0.3'; }} />
          <button type="button" onClick={onRemove} aria-label={`Remove ${label}`}
            style={{
              position: 'absolute', top: 8, right: 8,
              width: 26, height: 26, borderRadius: '50%',
              background: 'var(--danger)', color: '#fff', border: 'none',
              cursor: 'pointer', display: 'flex', alignItems: 'center',
              justifyContent: 'center', boxShadow: 'var(--shadow-sm)',
            }}>
            <X size={13} weight="bold" />
          </button>
        </div>
      ) : (
        // Full-width 16:9 drop zone rather than the old 120×80 thumbnail — it
        // matches the shape of the image being asked for, so what you drop is
        // what you get.
        <label style={{
          width: '100%', aspectRatio: '16 / 9',
          border: '1.5px dashed var(--border-2)', borderRadius: 'var(--r-lg)',
          display: 'flex', flexDirection: 'column', alignItems: 'center',
          justifyContent: 'center', gap: 8,
          cursor: uploading ? 'wait' : 'pointer',
          color: 'var(--text-3)', background: 'var(--surface-alpha)',
          transition: 'border-color .15s ease, color .15s ease',
        }}>
          <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={uploading}
            onChange={e => { const f = e.target.files?.[0]; if (f) { onUpload(f); e.target.value = ''; } }} />
          {uploading
            ? <div className="spinner" />
            : <>
                <UploadSimple size={26} />
                <span style={{ fontSize: 12.5, fontWeight: 600 }}>Click to upload</span>
              </>}
        </label>
      )}

      <div className="field-hint" style={{ marginTop: 8 }}>
        {hint ?? 'JPEG · PNG · WebP'}
      </div>
    </div>
  );
}

// ── FormCard ──────────────────────────────────────────────────────────────────
function FormCard({
  title, icon: IconCmp, subtitle, children,
}: {
  title: string; icon: PhosphorIcon; subtitle?: string; children: React.ReactNode;
}) {
  return (
    // No `overflow: hidden` here. It was clipping every popover raised from
    // inside a card — the date calendar was sliced off mid-month, and the venue
    // autocomplete dropdown had the same problem. The header rounds its own top
    // corners instead, which is all the overflow rule was really achieving.
    <section style={{
      background: 'var(--surface)', border: '1px solid var(--border)',
      borderRadius: 'var(--r-xl)',
    }}>
      <header style={{
        display: 'flex', alignItems: 'center', gap: 12,
        padding: '16px 22px', borderBottom: '1px solid var(--border)',
      }}>
        <span style={{
          width: 34, height: 34, borderRadius: 11, flexShrink: 0,
          background: 'var(--accent-soft)', color: 'var(--accent)',
          display: 'grid', placeItems: 'center',
        }}>
          <IconCmp size={18} weight="bold" />
        </span>
        <div style={{ minWidth: 0 }}>
          <div style={{ fontSize: 15, fontWeight: 800, letterSpacing: '-.01em' }}>{title}</div>
          {subtitle && (
            <div style={{ fontSize: 12.5, color: 'var(--text-3)', marginTop: 1 }}>{subtitle}</div>
          )}
        </div>
      </header>
      <div style={{ padding: 22 }}>{children}</div>
    </section>
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
  // Captured by the Places autocomplete alongside the three text fields, so the
  // clients can build an EXACT map link instead of a fuzzy text search.
  const [placeId,             setPlaceId]             = useState('');
  const [latitude,            setLatitude]            = useState<number | null>(null);
  const [longitude,           setLongitude]           = useState<number | null>(null);
  const [placeLabel,          setPlaceLabel]          = useState('');
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
        venueName, address, city, placeId, latitude, longitude, placeLabel, venueType, tags, performers, teams, ticketTypes,
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
      venueName, address, city, placeId, latitude, longitude, placeLabel, venueType, tags, performers, teams, ticketTypes,
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
    setPlaceId(d.placeId ?? '');
    setLatitude(typeof d.latitude === 'number' ? d.latitude : null);
    setLongitude(typeof d.longitude === 'number' ? d.longitude : null);
    setPlaceLabel(d.placeLabel ?? '');
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
        // Only sent when the organiser actually picked a suggestion — a
        // hand-typed address stays text-only rather than shipping a stale
        // place_id from a previous selection.
        ...(placeId   ? { place_id: placeId } : {}),
        ...(latitude  !== null ? { latitude }  : {}),
        ...(longitude !== null ? { longitude } : {}),
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
        <div style={{ minWidth: 0 }}>
          <button type="button" onClick={() => router.push('/events')}
            style={{
              display: 'inline-flex', alignItems: 'center', gap: 5,
              background: 'none', border: 'none', padding: 0, cursor: 'pointer',
              fontSize: 12, color: 'var(--text-3)', marginBottom: 5,
            }}>
            My Events
          </button>
          <div className="page-title">Create a new event</div>
          <div style={{ fontSize: 13, color: 'var(--text-3)', marginTop: 3 }}>
            {eventType === 'public'
              ? 'Public events are reviewed by an admin before going live.'
              : 'Private events go live the moment you publish them.'}
          </div>
        </div>

        {draftSavedAt && !submitting && (
          <span style={{
            display: 'inline-flex', alignItems: 'center', gap: 7,
            fontSize: 12, color: 'var(--text-3)', flexShrink: 0,
          }}>
            <FloppyDisk size={14} />
            Draft saved {relativeTime(draftSavedAt)}
          </span>
        )}
      </div>

      <div className="page-content">
        <div style={{ maxWidth: 1440, margin: '0 auto' }}>

          {/* ── Draft restore banner ─────────────────────────────────────── */}
          {pendingDraft && (
            <div style={{
              display: 'flex', alignItems: 'center', gap: 14, flexWrap: 'wrap',
              padding: '14px 18px', marginBottom: 20,
              background: 'var(--warning-bg)', border: '1px solid var(--warning-border)',
              borderRadius: 'var(--r-lg)', color: 'var(--warning-fg)',
            }}>
              <NotePencil size={20} weight="bold" style={{ flexShrink: 0 }} />
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
            {/* Was `1fr 300px` capped at 980, which left the inputs ~660px wide
                on any real monitor. minmax(0,…) on the main column is what stops
                a long venue name blowing the grid out instead of ellipsing. */}
            <div style={{
              display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) 360px',
              gap: 24, alignItems: 'start',
            }}>

              {/* ── Left column ───────────────────────────────────────────── */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 18, minWidth: 0 }}>

                {/* Event details */}
                <FormCard title="Event details" icon={Article}
                  subtitle="What people see first in the feed">
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
                      rows={6} style={{ ...inp, resize: 'vertical', lineHeight: 1.6 }} />
                    <div className="field-hint">{description.length} characters · minimum 50</div>
                  </div>
                </FormCard>

                {/* Date & time */}
                <FormCard title="Date & time" icon={CalendarBlank}
                  subtitle="Shown in the event's own timezone">
                  {/* Three across — these are the shortest fields on the page and
                      stacking them wasted the most space. */}
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 16 }}>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Start time <Required /></label>
                      <DateTimePicker value={startTime} onChange={setStartTime}
                        placeholder="When doors open" style={inp} />
                    </div>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>End time <Required /></label>
                      {/* min={startTime} stops the "end before start" error being
                          reachable at all, rather than catching it on submit. */}
                      <DateTimePicker value={endTime} onChange={setEndTime}
                        min={startTime} placeholder="When it wraps up" style={inp} />
                    </div>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Booking deadline <Required /></label>
                      <DateTimePicker value={expiresAt} onChange={setExpiresAt}
                        placeholder="Last moment to buy" style={inp} />
                      <div className="field-hint">Tickets stop selling after this</div>
                    </div>
                  </div>

                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginTop: 20 }}>
                    <div className="field" style={{ marginBottom: 0 }}>
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
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Ticket scanning window</label>
                      <select value={scanWindowMode}
                        onChange={e => setScanWindowMode(e.target.value as 'default' | 'custom')}
                        style={inp}>
                        <option value="default">Default — 2h before, 2h after</option>
                        <option value="custom">Custom — set my own window</option>
                      </select>
                      <div className="field-hint">When staff can scan at the door</div>
                    </div>
                  </div>

                  {scanWindowMode === 'custom' && (
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginTop: 16 }}>
                      <div className="field" style={{ marginBottom: 0 }}>
                        <label>Opens before start (minutes)</label>
                        <input type="number" min={0} max={10080} value={scanOpenMin}
                          onChange={e => setScanOpenMin(e.target.value)} style={inp}
                          placeholder="120" />
                        <div className="field-hint">e.g. 120 = 2 hours</div>
                      </div>
                      <div className="field" style={{ marginBottom: 0 }}>
                        <label>Closes after end (minutes)</label>
                        <input type="number" min={0} max={10080} value={scanCloseMin}
                          onChange={e => setScanCloseMin(e.target.value)} style={inp}
                          placeholder="120" />
                        <div className="field-hint">e.g. 720 = 12 hours (all-day)</div>
                      </div>
                    </div>
                  )}
                </FormCard>

                {/* Location */}
                <FormCard title="Location" icon={MapPin}
                  subtitle="Search to pin the exact venue on a map">
                  {/* Search first — picking a suggestion fills the three fields
                      below AND captures place_id + lat/lng, which is what lets the
                      app and website open the exact venue on a map instead of a
                      fuzzy text search. Manual entry still works if the venue
                      isn't listed. */}
                  <PlacesAutocomplete
                    style={inp}
                    initialValue={placeLabel}
                    onSelect={(p: PlaceResult) => {
                      if (p.venueName) setVenueName(p.venueName);
                      if (p.address)   setAddress(p.address);
                      if (p.city)      setCity(p.city);
                      setPlaceId(p.placeId);
                      setLatitude(p.latitude);
                      setLongitude(p.longitude);
                      setPlaceLabel(p.formatted);
                    }}
                  />
                  {placeId && (
                    <div style={{
                      display: 'flex', alignItems: 'center', gap: 8, margin: '-4px 0 18px',
                      fontSize: 12.5, color: 'var(--success-fg)',
                    }}>
                      <SealCheck size={15} weight="fill" />
                      <span style={{ flex: 1 }}>Pinned to <strong>{placeLabel || 'the selected place'}</strong></span>
                      <button type="button" onClick={() => {
                        setPlaceId(''); setLatitude(null); setLongitude(null); setPlaceLabel('');
                      }} style={{
                        background: 'none', border: 'none', cursor: 'pointer',
                        color: 'var(--text-3)', fontSize: 12, textDecoration: 'underline',
                      }}>Clear</button>
                    </div>
                  )}

                  <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1.4fr 1fr', gap: 16 }}>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Venue name <Required /></label>
                      <input value={venueName} onChange={e => setVenueName(e.target.value)}
                        placeholder="e.g. FNB Stadium" style={inp} />
                    </div>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Street address <Required /></label>
                      <input value={address} onChange={e => setAddress(e.target.value)}
                        placeholder="e.g. 1 Soccer City Ave" style={inp} />
                    </div>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>City <Required /></label>
                      <input value={city} onChange={e => setCity(e.target.value)}
                        placeholder="e.g. Johannesburg" style={inp} />
                    </div>
                  </div>

                  <div style={{ marginTop: 22 }}>
                    <label style={{
                      display: 'block', fontSize: 11, fontWeight: 700,
                      letterSpacing: '.07em', textTransform: 'uppercase',
                      color: 'var(--text-3)', marginBottom: 4,
                    }}>
                      Venue type
                    </label>
                    <div className="field-hint" style={{ marginTop: 0, marginBottom: 10 }}>
                      Controls which seating map the app shows at checkout
                    </div>
                    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 10 }}>
                      {([
                        { value: 'auto',       Ico: MagicWand, label: 'Auto',          hint: 'Guess from category & name' },
                        { value: 'stadium',    Ico: Vault,     label: 'Stadium',       hint: 'Stadium section map' },
                        { value: 'auditorium', Ico: MaskHappy, label: 'Hall / Theatre', hint: 'Auditorium seating map' },
                        { value: 'none',       Ico: Ticket,    label: 'None',          hint: 'Straight to checkout' },
                      ] as const).map(o => {
                        const on = venueType === o.value;
                        return (
                          <button key={o.value} type="button" title={o.hint}
                            onClick={() => setVenueType(o.value)}
                            style={{
                              display: 'flex', flexDirection: 'column', alignItems: 'flex-start',
                              gap: 7, padding: '13px 14px', borderRadius: 'var(--r-lg)',
                              cursor: 'pointer', textAlign: 'left',
                              border: `1px solid ${on ? 'var(--accent)' : 'var(--border)'}`,
                              background: on ? 'var(--accent-soft)' : 'var(--surface-alpha)',
                              color: on ? 'var(--accent)' : 'var(--text-2)',
                              transition: 'all .15s ease',
                            }}>
                            <o.Ico size={19} weight={on ? 'fill' : 'regular'} />
                            <span style={{ fontSize: 13, fontWeight: on ? 700 : 600 }}>{o.label}</span>
                            <span style={{ fontSize: 11, color: 'var(--text-3)', lineHeight: 1.35 }}>{o.hint}</span>
                          </button>
                        );
                      })}
                    </div>
                  </div>
                </FormCard>

                {/* Tickets & pricing */}
                <FormCard title="Tickets & pricing" icon={Ticket}
                  subtitle="Category, price and capacity">
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
                    <div className="field" style={{ marginBottom: 0 }}>
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
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Pricing</label>
                      <div style={{ display: 'flex', gap: 10 }}>
                        {[true, false].map(free => {
                          const on = isFree === free;
                          return (
                            <label key={String(free)} style={{
                              display: 'flex', alignItems: 'center', gap: 9,
                              padding: '10px 13px', flex: 1, cursor: 'pointer',
                              border: `1px solid ${on ? 'var(--accent)' : 'var(--border)'}`,
                              borderRadius: 'var(--r)',
                              background: on ? 'var(--accent-soft)' : 'var(--surface-alpha)',
                              transition: 'all .15s ease',
                            }}>
                              <input type="radio" name="pricing" checked={on}
                                onChange={() => setIsFree(free)}
                                style={{ accentColor: 'var(--accent-deep)' }} />
                              <span style={{
                                fontWeight: on ? 700 : 600, fontSize: 13.5,
                                color: on ? 'var(--accent)' : 'var(--text-2)',
                              }}>
                                {free ? 'Free' : 'Paid'}
                              </span>
                            </label>
                          );
                        })}
                      </div>
                    </div>
                  </div>

                  {!isFree && (
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginTop: 18 }}>
                      <div className="field" style={{ marginBottom: 0 }}>
                        <label>Base price <Required /></label>
                        <div style={{ position: 'relative' }}>
                          <span style={{
                            position: 'absolute', left: 13, top: '50%', transform: 'translateY(-50%)',
                            color: 'var(--text-3)', fontWeight: 700, fontSize: 14, pointerEvents: 'none',
                          }}>R</span>
                          <input type="number" min="0" step="0.01" value={price}
                            onChange={e => setPrice(e.target.value)}
                            style={{ ...inp, paddingLeft: 30 }} />
                        </div>
                      </div>
                      <div className="field" style={{ marginBottom: 0 }}>
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

                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 16, marginTop: 18, alignItems: 'end' }}>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Max capacity</label>
                      <input type="number" min="1" value={maxCapacity}
                        onChange={e => setMaxCapacity(e.target.value)} placeholder="Unlimited" style={inp} />
                      <div className="field-hint">Blank = unlimited</div>
                    </div>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Max per user</label>
                      <input type="number" min="1" max="20" value={maxTicketsPerUser}
                        onChange={e => setMaxTicketsPerUser(e.target.value)} placeholder="No limit" style={inp} />
                      <div className="field-hint">Blank = no limit</div>
                    </div>
                    <label className="check-row" style={{ paddingBottom: 11 }}>
                      <input type="checkbox" checked={allowWaitlist}
                        onChange={e => setAllowWaitlist(e.target.checked)} />
                      <span style={{ fontSize: 13 }}>Allow waitlist when sold out</span>
                    </label>
                  </div>

                  <div style={{ marginTop: 26 }}>
                    <div className="section-sep" style={{ marginTop: 0 }}>Ticket types</div>
                    <TicketTypeBuilder rows={ticketTypes} onChange={setTicketTypes} />
                  </div>
                </FormCard>

                {/* Tags & people */}
                <FormCard title="Tags & people" icon={Tag}
                  subtitle="Helps the right people find your event">
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 16 }}>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Tags</label>
                      <TagInput tags={tags} onChange={setTags} />
                    </div>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Performers</label>
                      <TagInput tags={performers} onChange={setPerformers} />
                    </div>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Teams</label>
                      <TagInput tags={teams} onChange={setTeams} />
                    </div>
                  </div>
                </FormCard>

                {/* Media */}
                <FormCard title="Media" icon={ImageSquare}
                  subtitle="Cover art for the feed, banner for the event page">
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 20 }}>
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
                      hint="Wide hero at the top of the event page · max 10 MB"
                    />
                  </div>
                </FormCard>

              </div>{/* end left column */}

              {/* ── Right rail ────────────────────────────────────────────── */}
              <aside style={{ position: 'sticky', top: 0, display: 'flex', flexDirection: 'column', gap: 16 }}>

                {/* Event type */}
                <section style={{
                  background: 'var(--surface)', border: '1px solid var(--border)',
                  borderRadius: 'var(--r-xl)', padding: 20,
                }}>
                  <div style={{ fontSize: 14.5, fontWeight: 800, marginBottom: 12 }}>Event type</div>
                  <select value={eventType}
                    onChange={(e) => {
                      const v = e.target.value as typeof eventType;
                      if (v !== 'private_free' && kycLevel < 2) { router.push('/kyc'); return; }
                      setEventType(v);
                    }}
                    style={inp}>
                    <option value="private_free">Private free event</option>
                    <option value="private_paid" disabled={kycLevel < 2}>
                      Private paid event{kycLevel < 2 ? ' — needs full verification' : ''}
                    </option>
                    <option value="public" disabled={kycLevel < 2}>
                      Public event{kycLevel < 2 ? ' — needs full verification' : ''}
                    </option>
                  </select>
                  <p style={{ fontSize: 12.5, color: 'var(--text-3)', margin: '10px 0 0', lineHeight: 1.6 }}>
                    {eventType === 'private_free' && 'Invite-only via a shareable link. Free tickets only. Goes live immediately — no admin review.'}
                    {eventType === 'private_paid' && 'Invite-only via a shareable link, with paid tickets. Goes live immediately — no admin review. Requires full verification and payout bank details.'}
                    {eventType === 'public' && 'Listed in the TicketHawk feed and search for everyone. Requires full verification and admin review before going live.'}
                  </p>
                  {kycLevel < 2 && (
                    <button type="button" className="btn btn-sm" style={{ width: '100%', marginTop: 14 }}
                      onClick={() => router.push('/kyc')}>
                      <LockKey size={14} weight="bold" />
                      Unlock paid &amp; public — verify your ID
                    </button>
                  )}
                </section>

                {/* Checklist */}
                <section style={{
                  background: 'var(--surface)', border: '1px solid var(--border)',
                  borderRadius: 'var(--r-xl)', padding: 20,
                }}>
                  <div style={{
                    display: 'flex', alignItems: 'baseline', justifyContent: 'space-between',
                    gap: 10, marginBottom: 14,
                  }}>
                    <span style={{ fontSize: 14.5, fontWeight: 800 }}>Before you submit</span>
                    <span style={{
                      fontSize: 12, fontWeight: 700, color: 'var(--text-3)',
                      fontVariantNumeric: 'tabular-nums',
                    }}>
                      {checklist.filter(i => i.done).length}/{checklist.length}
                    </span>
                  </div>

                  <div className="progress-bar" style={{ marginBottom: 16 }}>
                    <div className="progress-fill" style={{
                      width: `${(checklist.filter(i => i.done).length / checklist.length) * 100}%`,
                    }} />
                  </div>

                  {checklist.map(item => (
                    <div key={item.label} style={{
                      display: 'flex', gap: 9, alignItems: 'flex-start', marginBottom: 10,
                    }}>
                      <span style={{
                        flexShrink: 0, marginTop: 1,
                        color: item.done ? 'var(--success)' : 'var(--text-faint)',
                        display: 'flex',
                      }}>
                        {item.done
                          ? <CheckCircle size={16} weight="fill" />
                          : <Circle size={16} />}
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

                  <div className="alert alert-info" style={{ fontSize: 12, marginTop: 16, marginBottom: 0 }}>
                    <Info size={15} weight="fill" style={{ flexShrink: 0, marginTop: 1 }} />
                    <span>{eventType === 'public'
                      ? 'Public events need admin approval before going live. You\u2019ll be notified once approved.'
                      : 'Private events go live immediately after you publish \u2014 no admin review.'}</span>
                  </div>
                </section>
              </aside>

            </div>{/* end grid */}

            {/* ── Sticky action bar ────────────────────────────────────────
                The form is long enough that a footer at the bottom means
                scrolling past everything to submit. Sticking it to the bottom of
                the scroll container keeps Submit reachable from any point. */}
            <div style={{
              position: 'sticky', bottom: 0, zIndex: 20,
              display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap',
              marginTop: 24, padding: '14px 18px',
              background: 'var(--bg-elevated)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--r-lg)',
              boxShadow: 'var(--shadow)',
            }}>
              <button type="submit" className="btn btn-primary" disabled={submitting}>
                {submitting
                  ? <><span className="spinner" style={{ width: 14, height: 14, borderWidth: 2 }} /> Submitting…</>
                  : <><Confetti size={15} weight="bold" /> {eventType === 'public' ? 'Submit for review' : 'Create event'}</>}
              </button>
              <button type="button" className="btn" disabled={submitting}
                onClick={() => router.push('/events')}>Cancel</button>

              <span style={{ flex: 1 }} />

              <span style={{
                display: 'inline-flex', alignItems: 'center', gap: 7,
                fontSize: 12.5, color: 'var(--text-3)',
              }}>
                <Check size={14} weight="bold" />
                {checklist.filter(i => i.done).length} of {checklist.length} steps done
              </span>
            </div>

          </form>
        </div>
      </div>
    </div>
  );
}