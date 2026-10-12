'use client';
import { useState, useEffect, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import {
  Article, CalendarBlank, MapPin, Ticket, Tag, ImageSquare,
  Confetti,
  Trash, Plus, X, Check, CheckCircle, Circle, PencilSimple,
  FloppyDisk, NotePencil, LockKey, UploadSimple, Info, SealCheck, EyeSlash, Warning,
  type Icon as PhosphorIcon,
} from '@phosphor-icons/react';
import { api } from '../../../../lib/api';
import { useToast } from '../../../../lib/toast';
import { useConfirm } from '../../../../lib/confirm';
import { validateRequired, validateMinLength, firstError } from '../../../../lib/validation';
import { getKycStatus } from '../../../../lib/kyc';
import PlacesAutocomplete, { PlaceResult } from '@/components/placesAutocomplete';
import DateTimePicker from '@/components/dateTimePicker';
import ImageCropper, { CROP_COVER } from '@/components/imageCropper';
import TicketTypeModal from '@/components/ticketTypeModal';
import Select from '@/components/select';
import LineupFields from '@/components/lineupFields';
import EventInfoFields from '@/components/eventInfoFields';
import {
  getInfoCatalogue, replaceEventInfo, replaceEventLineup,
  type InfoCatalogueEntry, type LineupEntry,
} from '../../../../lib/eventContent';

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
  // id is present only for tiers loaded from an existing event. A row without
  // one is new, which is how the backend tells "update this tier" apart from
  // "create this tier".
  id?: string;
  name: string; price: number; capacity: string;
  sort_order: number; description: string; is_visible: boolean;
  // Sold + reserved on an existing tier: capacity may not be set below this.
  min_capacity?: number;
}

interface EventDraft {
  savedAt: string;
  title: string; description: string; categoryId: string;
  timezone: string; startTime: string; endTime: string; expiresAt: string;
  // Optional: drafts saved before this fix won't have it, and must fall back
  // to 'private_free' on restore rather than throw. Without saving this, a
  // draft always came back as private_free regardless of what was chosen —
  // silently turning a "Public event" into a private one on restore.
  eventType?: 'private_free' | 'private_paid' | 'public';
  isFree: boolean; price: string; currency: string;
  maxCapacity: string; maxTicketsPerUser: string; allowWaitlist: boolean;
  venueName: string; address: string; city: string;
  // Google Places selection saved with the draft, so restoring a draft keeps
  // the venue pinned rather than silently falling back to a text search.
  // Optional: drafts saved before this feature existed won't have them.
  placeId?: string; placeLabel?: string;
  latitude?: number | null; longitude?: number | null;
  // The venue/address/city the pin was captured WITH. Without this a refresh
  // resets the staleness check and a drifted pin goes quiet again.
  pinnedAt?: { venueName: string; address: string; city: string } | null;
  venueType: 'auto' | 'stadium' | 'auditorium' | 'none';
  tags: string[]; performers: string[]; teams: string[];
  // Optional: drafts saved before line-up and information existed will not have
  // them, and restoring one of those must not throw.
  lineup?: LineupEntry[];
  infoChosen?: string[];
  infoValues?: Record<string, string>;
  ticketTypes: TicketTypeRow[];
  // Retained in the draft shape even though the form no longer shows them, so
  // a draft saved by the previous version still round-trips without throwing.
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
// The list is now read-only summaries; composing and editing happen in
// TicketTypeModal. A tier only enters this array once it has passed validation,
// so the array can no longer hold a half-filled row that the submit handler
// silently drops.
function TicketTypeBuilder({ rows, onChange }: { rows: TicketTypeRow[]; onChange: (r: TicketTypeRow[]) => void }) {
  const [modalOpen, setModalOpen] = useState(false);
  // null = adding; a number = editing that index.
  const [editing, setEditing] = useState<number | null>(null);

  function openAdd()  { setEditing(null); setModalOpen(true); }
  function openEdit(i: number) { setEditing(i); setModalOpen(true); }

  function confirm(t: TicketTypeRow) {
    if (editing === null) onChange([...rows, t]);
    else onChange(rows.map((r, j) => (j === editing ? t : r)));
    setModalOpen(false);
    setEditing(null);
  }

  function del(i: number) { onChange(rows.filter((_, j) => j !== i)); }

  return (
    <>
      <div className="alert alert-info" style={{ marginBottom: 14 }}>
        <Info size={17} weight="fill" style={{ flexShrink: 0, marginTop: 1 }} />
        <span>Ticket types define pricing tiers (e.g. General, VIP). Leave empty for flat pricing.</span>
      </div>

      {rows.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 14 }}>
          {rows.map((r, i) => (
            <div key={i} style={{
              display: 'flex', alignItems: 'center', gap: 14,
              padding: '13px 16px',
              background: 'var(--surface-alpha)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--r-lg)',
            }}>
              <span style={{
                width: 30, height: 30, borderRadius: 9, flexShrink: 0,
                background: 'var(--accent-soft)', color: 'var(--accent)',
                display: 'grid', placeItems: 'center',
                fontSize: 12, fontWeight: 800, fontVariantNumeric: 'tabular-nums',
              }}>
                {r.sort_order}
              </span>

              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{
                  display: 'flex', alignItems: 'center', gap: 8,
                  fontSize: 14, fontWeight: 700, color: 'var(--text)',
                }}>
                  <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {r.name}
                  </span>
                  {!r.is_visible && (
                    <span title="Hidden from buyers" style={{
                      display: 'inline-flex', alignItems: 'center', gap: 4,
                      fontSize: 10.5, fontWeight: 700, letterSpacing: '.04em',
                      textTransform: 'uppercase', color: 'var(--text-3)',
                      background: 'var(--surface-strong)', borderRadius: 999,
                      padding: '2px 8px', flexShrink: 0,
                    }}>
                      <EyeSlash size={11} weight="bold" /> Hidden
                    </span>
                  )}
                </div>
                {r.description && (
                  <div style={{
                    fontSize: 12.5, color: 'var(--text-3)', marginTop: 2,
                    overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                  }}>
                    {r.description}
                  </div>
                )}
              </div>

              <div style={{ textAlign: 'right', flexShrink: 0 }}>
                <div style={{
                  fontSize: 14.5, fontWeight: 800, color: 'var(--text)',
                  fontVariantNumeric: 'tabular-nums',
                }}>
                  {r.price > 0
                    ? `R${r.price.toLocaleString('en-ZA', { minimumFractionDigits: 2 })}`
                    : 'Free'}
                </div>
                <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 1 }}>
                  {r.capacity ? `${r.capacity} available` : 'Unlimited'}
                </div>
              </div>

              <div style={{ display: 'flex', gap: 2, flexShrink: 0 }}>
                <button type="button" className="btn-icon" onClick={() => openEdit(i)}
                  aria-label={`Edit ${r.name}`}>
                  <PencilSimple size={15} />
                </button>
                <button type="button" className="btn-icon" onClick={() => del(i)}
                  aria-label={`Remove ${r.name}`} style={{ color: 'var(--danger-fg)' }}>
                  <Trash size={15} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      <button type="button" className="btn btn-sm" onClick={openAdd}>
        <Plus size={14} weight="bold" />
        Add ticket type
      </button>

      <TicketTypeModal
        open={modalOpen}
        initial={editing === null ? null : rows[editing]}
        index={editing === null ? rows.length : rows[editing].sort_order}
        // The tier being edited is excluded, otherwise saving it unchanged
        // would fail the duplicate-name check against itself.
        existingNames={rows.filter((_, j) => j !== editing).map(r => r.name)}
        onCancel={() => { setModalOpen(false); setEditing(null); }}
        onConfirm={confirm}
      />
    </>
  );
}

// ── ImageUploader ─────────────────────────────────────────────────────────────
// Accepts a click OR a dropped file.
//
// The drop target is the whole tile and it reports its own state: `isOver`
// switches the border and tint the moment a file is dragged over it, because a
// drop zone that looks identical whether or not it will accept the file is a
// guess, not an affordance.
function ImageUploader({
  label, imageUrl, uploading, onUpload, onRemove, hint, aspect,
}: {
  label: string; imageUrl: string | null; uploading: boolean;
  onUpload: (f: File) => void; onRemove: () => void; hint?: string;
  /** When set, the file goes through ImageCropper at this ratio before it is
   *  uploaded. Omitted uploads exactly what was picked, which is what the
   *  banner does — its framing is decided per surface at display time. */
  aspect?: number | null;
}) {
  const [isOver, setIsOver] = useState(false);
  const [rejected, setRejected] = useState<string | null>(null);
  // Held between picking and confirming a crop. Nothing is sent until the crop
  // is accepted, so cancelling costs no bandwidth and leaves no orphan in R2.
  const [cropping, setCropping] = useState<File | null>(null);

  const ACCEPTED = ['image/jpeg', 'image/png', 'image/webp'];
  const MAX_BYTES = 10 * 1024 * 1024;

  // Checked here as well as server-side. Not a security boundary — the point is
  // that a 40 MB PNG fails instantly with a reason, instead of after a long
  // upload that ends in a generic error.
  function accept(f: File): boolean {
    if (!ACCEPTED.includes(f.type)) {
      setRejected('That file type isn\u2019t supported — use JPEG, PNG or WebP');
      return false;
    }
    if (f.size > MAX_BYTES) {
      setRejected(`That file is ${(f.size / 1024 / 1024).toFixed(1)} MB — the limit is 10 MB`);
      return false;
    }
    setRejected(null);
    return true;
  }

  // One route in for both a click and a drop: validate, crop if this slot
  // crops, then upload. Two copies of that decision is how a drag-and-drop
  // path quietly skips a step the click path takes.
  function take(f: File | undefined | null) {
    if (!f || !accept(f)) return;
    if (aspect) setCropping(f);
    else onUpload(f);
  }

  function onDrop(e: React.DragEvent) {
    e.preventDefault();
    setIsOver(false);
    if (uploading) return;
    take(e.dataTransfer.files?.[0]);
  }

  return (
    <div>
      {cropping && aspect && (
        <ImageCropper
          file={cropping}
          aspect={aspect}
          title={`Crop ${label.toLowerCase()}`}
          onCancel={() => setCropping(null)}
          onConfirm={f => { setCropping(null); onUpload(f); }}
        />
      )}

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
        // Full-width 16:9 drop zone rather than a 120x80 thumbnail — it matches
        // the shape of the image being asked for, so what you drop is what you
        // get.
        <label
          onDragOver={e => { e.preventDefault(); if (!uploading) setIsOver(true); }}
          onDragEnter={e => { e.preventDefault(); if (!uploading) setIsOver(true); }}
          // relatedTarget is null when the pointer leaves the window entirely;
          // without this the highlight sticks on after dragging away.
          onDragLeave={e => { if (!e.currentTarget.contains(e.relatedTarget as Node)) setIsOver(false); }}
          onDrop={onDrop}
          style={{
            width: '100%', aspectRatio: '16 / 9',
            border: `1.5px dashed ${isOver ? 'var(--accent)' : 'var(--border-2)'}`,
            borderRadius: 'var(--r-lg)',
            display: 'flex', flexDirection: 'column', alignItems: 'center',
            justifyContent: 'center', gap: 8,
            cursor: uploading ? 'wait' : 'pointer',
            color: isOver ? 'var(--accent)' : 'var(--text-3)',
            background: isOver ? 'var(--accent-soft)' : 'var(--surface-alpha)',
            transition: 'border-color .15s ease, color .15s ease, background .15s ease',
          }}>
          <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={uploading}
            onChange={e => { take(e.target.files?.[0]); e.target.value = ''; }} />
          {uploading
            ? <div className="spinner" />
            : <>
                <UploadSimple size={26} weight={isOver ? 'bold' : 'regular'} />
                <span style={{ fontSize: 12.5, fontWeight: 600 }}>
                  {isOver ? 'Drop to upload' : 'Drag an image here, or click to browse'}
                </span>
              </>}
        </label>
      )}

      {rejected
        ? <div style={{ fontSize: 12, color: 'var(--danger-fg)', marginTop: 8, fontWeight: 500 }}>{rejected}</div>
        : <div className="field-hint" style={{ marginTop: 8 }}>{hint ?? 'JPEG \u00b7 PNG \u00b7 WebP'}</div>}
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
// LockedFieldset greys out and disables everything inside it when a field is
// frozen. A native <fieldset disabled> is used rather than adding `disabled` to
// each input because it covers every control in the subtree — including the
// ones inside PlacesAutocomplete and the tier builder — and cannot be missed
// when a new field is added later.
//
// The fields stay VISIBLE. An organiser needs to see the venue and date they
// committed to; hiding them would read as data loss rather than a restriction.
function LockedFieldset({
  locked, note, children,
}: { locked: boolean; note?: string; children: React.ReactNode }) {
  return (
    <fieldset
      disabled={locked}
      style={{
        border: 'none', padding: 0, margin: 0, minWidth: 0,
        opacity: locked ? 0.55 : 1,
        cursor: locked ? 'not-allowed' : undefined,
      }}>
      {locked && note && (
        <div style={{
          display: 'flex', alignItems: 'flex-start', gap: 7,
          fontSize: 12, color: 'var(--text-3)', marginBottom: 10,
        }}>
          <LockKey size={14} weight="fill" style={{ flexShrink: 0, marginTop: 1 }} />
          <span>{note}</span>
        </div>
      )}
      {children}
    </fieldset>
  );
}

export default function CreateEventPage() {
  const router  = useRouter();
  const toast   = useToast();
  const confirm = useConfirm();

  // ── Edit mode ───────────────────────────────────────────────────────────
  //
  // The page doubles as the editor for an existing event, entered via
  // /events/create?edit=<id>.
  //
  //   mode 'create'    — blank form, posts to /organiser/events/create
  //   mode 'draft'     — hydrated, saves straight to PUT /organiser/events/:id
  //   mode 'published' — hydrated, files an edit request for admin review;
  //                      fields that cannot change are visibly locked
  //
  // The id is read from window.location rather than useSearchParams because
  // useSearchParams forces the route into a Suspense boundary at build time;
  // this page is already client-only, so reading location in an effect keeps
  // the change local instead of restructuring the route.
  const [editId,          setEditId]          = useState<string | null>(null);
  const [mode,            setMode]            = useState<'create' | 'draft' | 'published'>('create');
  const [hydrating,       setHydrating]       = useState(false);
  const [editableFields,  setEditableFields]  = useState<Set<string>>(new Set());
  const [editReason,      setEditReason]      = useState('');
  // Start date is pinned on a published event — only the clock time may move.
  // Kept so submit can prove the date is unchanged before sending.
  const [originalStartDate, setOriginalStartDate] = useState<string | null>(null);

  // locked() answers "is this field frozen right now?" — false in create and
  // draft mode, true on a published event unless the backend listed the field
  // as editable. The list comes FROM the API so the two cannot drift apart.
  const locked = useCallback((field: string) => {
    if (mode !== 'published') return false;
    return !editableFields.has(field);
  }, [mode, editableFields]);

  const [categories, setCategories]   = useState<{ id: number; name: string; slug: string }[]>([]);
  // false when idle; otherwise which action is in flight, so "Save draft" and
  // "Submit for review" can each show their own spinner instead of a generic
  // one appearing on whichever button the organiser did not click.
  const [submitting, setSubmitting]   = useState<false | 'save' | 'publish'>(false);
  const [pendingDraft, setPendingDraft] = useState<EventDraft | null>(null);

  // ── Why a ref and not state ─────────────────────────────────────────────
  //
  // Once an event has been created, this form must never write its draft
  // again. The autosave already guards on `submitting`, but that is not
  // enough: the finally block sets submitting back to false, and because
  // router.push does not unmount the page synchronously, the debounced
  // autosave then fires ~800ms later with the form still fully populated and
  // rewrites the draft that was just deleted.
  //
  // That is the "Unsaved draft — saved just now" banner on My Events after a
  // successful create, and it is worse than untidy: Resume draft reloads a
  // form for an event that already exists, so submitting it again creates a
  // duplicate.
  //
  // A ref, not state, because the guard must be visible to the already-
  // scheduled effect closure without triggering another render.
  const submittedRef = useRef(false);

  // Local 'YYYY-MM-DDTHH:mm' for right now, used as the floor on every date
  // field. Local rather than ISO because DateTimePicker parses it as wall-clock
  // time — an ISO string would shift the boundary by the UTC offset and, in
  // SAST, make the first two hours of today unselectable.
  //
  // useState with an initialiser rather than useMemo: this must be stable for
  // the life of the form. A value that advances while somebody is filling it in
  // could invalidate a start time they already chose.
  const [nowLocal] = useState(() => {
    const d = new Date();
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
      + `T${pad(d.getHours())}:${pad(d.getMinutes())}`;
  });
  const [draftSavedAt, setDraftSavedAt] = useState<string | null>(null);

  // ── Text fields ───────────────────────────────────────────────────────────
  const [title,               setTitle]               = useState('');
  const [description,         setDescription]         = useState('');
  const [categoryId,          setCategoryId]          = useState('');
  const [timezone,            setTimezone]            = useState('Africa/Johannesburg');
  const [startTime,           setStartTime]           = useState('');
  const [endTime,             setEndTime]             = useState('');
  const [expiresAt,           setExpiresAt]           = useState('');
  // Scan (admission) window. Both offsets are measured from START time.
  // 'default' = opens 2h before start, closes 2h after start or 30 min before
  // the end, whichever comes first. 'custom' exposes the minute offsets for
  // all-day or tight-door events, and a custom close is NOT capped.
  const [scanWindowMode, setScanWindowMode] = useState<'default' | 'custom'>('default');
  const [scanOpenMin,  setScanOpenMin]  = useState('120');   // minutes before start
  const [scanCloseMin, setScanCloseMin] = useState('120');   // minutes after start
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
  // The venue/address/city the map pin was captured with.
  //
  // A pin is only meaningful while it still matches the text beside it. These
  // three fields are free-text inputs, so an organiser fixing a typo'd street
  // address leaves place_id and lat/lng pointing at the ORIGINAL place — and
  // every surface that opens a map prefers coordinates over text, by design,
  // because a text search once sent buyers to the wrong FNB Stadium.
  //
  // The result: the event reads correctly everywhere and navigates to the old
  // location. iOS hides it (Apple Maps shows the label you pass alongside `ll`,
  // so the NEW address is displayed over the OLD pin); Android's geo: URI shows
  // what is actually at those coordinates, so it looks broken there first.
  // Android is right. Nothing is wrong in the app — the record is contradictory.
  const [pinnedAt, setPinnedAt] = useState<
    { venueName: string; address: string; city: string } | null
  >(null);
  const [placeId,             setPlaceId]             = useState('');
  const [latitude,            setLatitude]            = useState<number | null>(null);
  const [longitude,           setLongitude]           = useState<number | null>(null);
  const [placeLabel,          setPlaceLabel]          = useState('');

  // Has the address text moved away from the place the pin was dropped on?
  //
  // Compared trimmed, because trailing whitespace is not a relocation. Only
  // meaningful while a pin exists — with no pin there is nothing to contradict,
  // and every map surface falls back to searching the text, which is the safe
  // degradation rather than a silently wrong one.
  const pinStale = !!placeId && !!pinnedAt && (
    venueName.trim() !== pinnedAt.venueName.trim() ||
    address.trim()   !== pinnedAt.address.trim()   ||
    city.trim()      !== pinnedAt.city.trim()
  );
  // Fixed at 'none' while seating maps are disabled; see the note in the
  // Location card where the picker used to be.
  const [venueType,           setVenueType]           = useState<'auto'|'stadium'|'auditorium'|'none'>('none');
  const [tags,                setTags]                = useState<string[]>([]);

  // ── Line-up and information, held locally until the event exists ─────────
  //
  // Neither can be saved during creation: both are keyed on an event id, and
  // the event does not exist until this form is submitted. So they are
  // collected here and written straight after the create call succeeds.
  //
  // That is a follow-up request rather than part of the create payload,
  // because CreateEventDTO has no line-up or information fields — adding them
  // would mean a migration to the DTO, the handler and the repository to save
  // one round-trip on a form somebody fills in once.
  const [lineup,     setLineup]     = useState<LineupEntry[]>([]);
  const [infoChosen, setInfoChosen] = useState<string[]>([]);
  const [infoValues, setInfoValues] = useState<Record<string, string>>({});
  const [infoCatalogue, setInfoCatalogue] = useState<InfoCatalogueEntry[]>([]);
  const [performers,          setPerformers]          = useState<string[]>([]);
  const [teams,               setTeams]               = useState<string[]>([]);
  const [ticketTypes,         setTicketTypes]         = useState<TicketTypeRow[]>([]);

  // Mirrors what the backend stores: events.price follows the cheapest VISIBLE
  // tier, so the locked field shows the real number rather than the stale one
  // typed before any tier was added.
  //
  // Declared here, after both `price` and `ticketTypes` — a const referencing
  // either before its useState line is a temporal dead zone error that throws
  // on every render, not a lint warning.
  //
  // The visible filter can empty the array (every tier staged hidden), and
  // Math.min() of nothing is Infinity, so fall back to the typed price.
  const visibleTierPrices = ticketTypes.filter(t => t.is_visible).map(t => t.price);
  const lowestTierPrice = visibleTierPrices.length
    ? String(Math.min(...visibleTierPrices))
    : price;

  // The price the event is ACTUALLY sold at, and the single source of truth for
  // the payload, the checklist and the locked field alike.
  //
  // Locking the base-price input stopped `price` being typed into, but the
  // submit payload and the readiness checklist both still read that raw state.
  // The result was an event that displayed R110 on screen and was created at
  // R0, with "Set ticket pricing" stuck unticked because nothing had been typed
  // into a field the organiser was no longer allowed to type into.
  const effectivePrice = ticketTypes.length > 0 ? lowestTierPrice : price;
  // ── refundPolicy / termsAndConditions removed ───────────────────────────
  //
  // Both were write-only: state that was submitted but never rendered by any
  // field on this form, so every event created here sent them as null anyway.
  //
  // More to the point, a per-event refund policy was never real. Refunds are
  // decided entirely by the sliding scale in the backend's
  // models/refund_policy.go — 100% at 30+ days, down to nothing inside 7 —
  // and that code never reads events.refund_policy. Anything an organiser
  // typed was display prose that the actual refund would contradict.
  //
  // Terms are platform-wide for the same reason: they are the terms of the
  // sale we process, not of the organiser's event.

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
    // Never offer the local scratch draft while editing a saved event — it
    // belongs to an unfinished CREATE and would silently overwrite the real
    // record's fields with someone's abandoned draft.
    if (typeof window !== 'undefined' && window.location.search.includes('edit=')) return;
    try {
      const raw = localStorage.getItem(DRAFT_KEY);
      if (!raw) return;
      const d = JSON.parse(raw) as EventDraft;
      if (d.title || d.description || d.venueName) setPendingDraft(d);
    } catch { /* corrupt data — ignore */ }
  }, []);

  // ── Hydrate from an existing event when ?edit=<id> is present ─────────────
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const id = new URLSearchParams(window.location.search).get('edit');
    if (!id) return;
    setEditId(id);
    setHydrating(true);

    (async () => {
      try {
        const res = await api.get(`/organiser/events/${id}/edit`);
        const ev = res.data.event;
        const tiers = res.data.ticket_types ?? [];

        setMode(res.data.requires_review ? 'published' : 'draft');
        const fields: string[] = res.data.editable_fields ?? [];
        setEditableFields(new Set(fields));

        // datetime-local wants 'YYYY-MM-DDTHH:mm' in LOCAL time; an ISO string
        // sliced naively would shift the displayed time by the UTC offset.
        const toLocalInput = (iso: string) => {
          const d = new Date(iso);
          const pad = (n: number) => String(n).padStart(2, '0');
          return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
            + `T${pad(d.getHours())}:${pad(d.getMinutes())}`;
        };

        setTitle(ev.title ?? '');
        setDescription(ev.description ?? '');
        setCategoryId(ev.category_id ? String(ev.category_id) : '');
        setTimezone(ev.timezone ?? 'Africa/Johannesburg');
        setStartTime(ev.start_time ? toLocalInput(ev.start_time) : '');
        setEndTime(ev.end_time ? toLocalInput(ev.end_time) : '');
        // These three were never restored, so opening Edit silently blanked
        // them — and because the form then submits what it holds, saving wrote
        // the blanks back over real values. The booking deadline is validated
        // as required, so the organiser at least saw an error; the refund
        // policy and terms would have been erased without a word.
        //
        // The API did not return them either. Both halves are fixed:
        // organiser_event_edit_handler.go now selects expires_at,
        // refund_policy and terms_and_conditions.
        setExpiresAt(ev.expires_at ? toLocalInput(ev.expires_at) : '');
        setOriginalStartDate(ev.start_time ? toLocalInput(ev.start_time).slice(0, 10) : null);
        // eventType was never restored here, so the selector always showed
        // its initial default — "Private free event" — no matter what the
        // event actually was. Editing a public draft and saving (mode
        // 'draft' PUTs `visibility: eventType === 'public' ? ... :
        // 'private_link'`, see below) then silently flipped it to private,
        // even though nothing on screen looked touched. Rebuild it from the
        // same two signals the event was saved with: visibility, and whether
        // anything about it is priced.
        const paidTier = tiers.some((t: Record<string, unknown>) => Number(t.price ?? 0) > 0);
        const freeEvent = (!ev.price || Number(ev.price) === 0) && !paidTier;
        setEventType(ev.visibility === 'public' ? 'public' : freeEvent ? 'private_free' : 'private_paid');
        setIsFree(!ev.price || Number(ev.price) === 0);
        setPrice(ev.price != null ? String(ev.price) : '0');
        setCurrency(ev.currency ?? 'ZAR');
        setMaxCapacity(ev.max_capacity != null ? String(ev.max_capacity) : '');
        setMaxTicketsPerUser(ev.max_tickets_per_user != null ? String(ev.max_tickets_per_user) : '');
        setAllowWaitlist(!!ev.allow_waitlist);
        setVenueName(ev.venue_name ?? '');
        setAddress(ev.address ?? '');
        setCity(ev.city ?? '');
        setPlaceId(ev.place_id ?? '');
        setLatitude(ev.latitude ?? null);
        setLongitude(ev.longitude ?? null);
        // Saved text and saved pin are taken to have agreed at save time, so
        // simply opening an event never reports drift. Editing any of the three
        // fields from here does.
        setPinnedAt(ev.place_id
          ? {
              venueName: String(ev.venue_name ?? ''),
              address:   String(ev.address ?? ''),
              city:      String(ev.city ?? ''),
            }
          : null);
        setTags(ev.tags ?? []);
        setPerformers(ev.performers ?? []);
        setTeams(ev.teams ?? []);
        setCoverImageUrl(ev.cover_image_url ?? null);
        setBannerImageUrl(ev.banner_image_url ?? null);
        setTicketTypes(tiers.map((t: Record<string, unknown>, i: number) => ({
          ...(t.id ? { id: String(t.id) } : {}),
          name:        String(t.name ?? ''),
          description: t.description ? String(t.description) : '',
          price:       Number(t.price ?? 0),
          capacity:    t.capacity != null ? String(t.capacity) : '',
          sort_order:  Number(t.sort_order ?? i),
          is_visible:  t.is_visible !== false,
          ...(t.min_capacity != null ? { min_capacity: Number(t.min_capacity) } : {}),
        })));
      } catch (e: unknown) {
        const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
        toast(msg ?? 'Could not load this event', 'error');
        router.push('/events');
      } finally {
        setHydrating(false);
      }
    })();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ── Auto-save draft (debounced 800 ms) ────────────────────────────────────
  useEffect(() => {
    if (submitting) return;
    // Set on a successful create and never cleared — see submittedRef.
    if (submittedRef.current) return;
    // Editing a saved event: the server is the source of truth, so there is
    // nothing to keep in localStorage and keeping it would resurface these
    // values the next time someone starts a brand-new event.
    if (editId) return;
    if (!title && !description && !venueName) return; // nothing worth saving
    const t = setTimeout(() => {
      const draft: EventDraft = {
        savedAt: new Date().toISOString(),
        title, description, categoryId, timezone, startTime, endTime, expiresAt,
        eventType,
        isFree, price, currency, maxCapacity, maxTicketsPerUser, allowWaitlist,
        venueName, address, city, placeId, latitude, longitude, placeLabel, pinnedAt, venueType, tags, performers, teams, ticketTypes,
        coverImageUrl, bannerImageUrl,
        // Included so a refresh mid-form does not lose a line-up somebody has
        // just typed out — the most tedious part of this page to re-enter.
        lineup, infoChosen, infoValues,
      };
      try {
        localStorage.setItem(DRAFT_KEY, JSON.stringify(draft));
        setDraftSavedAt(draft.savedAt);
      } catch { /* storage quota */ }
    }, 800);
    return () => clearTimeout(t);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [title, description, categoryId, timezone, startTime, endTime, expiresAt,
      eventType,
      isFree, price, currency, maxCapacity, maxTicketsPerUser, allowWaitlist,
      venueName, address, city, placeId, latitude, longitude, placeLabel, pinnedAt, venueType, tags, performers, teams, ticketTypes,
      coverImageUrl, bannerImageUrl,
      lineup, infoChosen, infoValues, submitting]);

  // The information catalogue is server-owned, so it is fetched rather than
  // hardcoded — an admin retiring a field or adding one takes effect here
  // without a dashboard deploy. A failure leaves the section empty rather than
  // blocking event creation, which must not depend on it.
  useEffect(() => {
    getInfoCatalogue().then(setInfoCatalogue).catch(() => setInfoCatalogue([]));
  }, []);

  // ── Restore draft ─────────────────────────────────────────────────────────
  function restoreDraft(d: EventDraft) {
    setTitle(d.title ?? '');
    setDescription(d.description ?? '');
    setCategoryId(d.categoryId ?? '');
    setTimezone(d.timezone ?? 'Africa/Johannesburg');
    setStartTime(d.startTime ?? '');
    setEndTime(d.endTime ?? '');
    setExpiresAt(d.expiresAt ?? '');
    // Older drafts predate this field and default to private_free, same as a
    // brand-new form — but a draft saved after this fix carries forward
    // whatever the organiser actually picked (including 'public'), instead of
    // silently reverting to private on restore.
    setEventType(d.eventType ?? 'private_free');
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
    // Older drafts predate the field; fall back to their own saved text, which
    // is the same assumption edit mode makes.
    setPinnedAt(d.pinnedAt ?? (d.placeId
      ? { venueName: d.venueName ?? '', address: d.address ?? '', city: d.city ?? '' }
      : null));
    setVenueType(d.venueType ?? 'none');
    setTags(d.tags ?? []);
    setLineup((d as { lineup?: LineupEntry[] }).lineup ?? []);
    setInfoChosen((d as { infoChosen?: string[] }).infoChosen ?? []);
    setInfoValues((d as { infoValues?: Record<string, string> }).infoValues ?? {});
    setPerformers(d.performers ?? []);
    setTeams(d.teams ?? []);
    setTicketTypes(d.ticketTypes ?? []);
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

  async function handleSubmit(e: React.SyntheticEvent, intent: 'save' | 'publish') {
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
    // Published-event guards. The backend enforces both of these too; doing it
    // here as well means the organiser is told before a round-trip.
    if (mode === 'published') {
      if (editReason.trim().length < 10) {
        toast('Add a short reason for the change — an admin has to review it', 'error');
        return;
      }
      if (originalStartDate && startTime.slice(0, 10) !== originalStartDate) {
        toast('The event date cannot be changed here — only the time of day', 'error');
        return;
      }
    }
    // Capacity on an existing tier cannot fall below what is already sold.
    const shrunk = ticketTypes.find(t =>
      t.min_capacity != null && t.capacity !== '' && parseInt(t.capacity) < t.min_capacity);
    if (shrunk) {
      toast(`"${shrunk.name}" already has ${shrunk.min_capacity} tickets sold or held — capacity can't go below that`, 'error');
      return;
    }

    // Shared by the draft-save and create paths below. The published/
    // edit-request branch builds its own `changes` straight from component
    // state instead — visibility, price and the like are not editable on a
    // live event, so most of this body would not apply there anyway.
    function buildBody(): Record<string, unknown> {
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
        price:        isFree ? 0 : parseFloat(effectivePrice) || 0,
        currency,
        allow_waitlist:       allowWaitlist,
        tags,
        performers,
        teams,
        visibility:           eventType === 'public' ? 'public' : 'private_link',
      };
      if (scanWindowMode === 'custom') {
        // A blank or unparseable field means "not chosen" — omit it, so the
        // column stays NULL and the default rule applies. It must NOT coerce
        // to 0 the way it used to: now that the close is anchored to the
        // START, 0 means "shut the doors the moment the event begins", which
        // is not a window anyone clears a field to ask for. An organiser who
        // actually types 0 still gets 0.
        const openMin  = parseInt(scanOpenMin, 10);
        const closeMin = parseInt(scanCloseMin, 10);
        if (Number.isFinite(openMin))  body.scan_open_offset_min  = openMin;
        if (Number.isFinite(closeMin)) body.scan_close_offset_min = closeMin;
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
      return body;
    }

    if (mode === 'published' && editId) {
      // Published: nothing is written to the event now. Only the fields the
      // backend marked editable are sent; it strips anything else anyway,
      // but filing a diff an admin cannot action just wastes their time.
      // There is no draft/publish split here — a live event only ever files
      // an edit-request — so `intent` does not apply to this branch.
      setSubmitting('publish');
      try {
        const changes: Record<string, unknown> = {};
        if (editableFields.has('title'))            changes.title            = title.trim();
        if (editableFields.has('description'))      changes.description      = description.trim();
        if (editableFields.has('start_time'))       changes.start_time       = new Date(startTime).toISOString();
        if (editableFields.has('end_time'))         changes.end_time         = new Date(endTime).toISOString();
        if (editableFields.has('tags'))             changes.tags             = tags;
        if (editableFields.has('cover_image_url')  && coverImageUrl)  changes.cover_image_url  = coverImageUrl;
        if (editableFields.has('banner_image_url') && bannerImageUrl) changes.banner_image_url = bannerImageUrl;
        if (editableFields.has('ticket_types') && ticketTypes.length) {
          changes.ticket_types = ticketTypes.filter(t => t.name).map(t => ({
            ...(t.id ? { id: t.id } : {}),
            name: t.name, price: t.price, sort_order: t.sort_order, is_visible: t.is_visible,
            ...(t.capacity    ? { capacity:    parseInt(t.capacity) } : {}),
            ...(t.description ? { description: t.description }       : {}),
          }));
        }

        const res = await api.post(`/organiser/events/${editId}/edit-request`, {
          reason: editReason.trim(),
          changes,
        });
        toast(res.data?.message ?? 'Sent for review', 'success');
        router.push('/events');
      } catch (e: unknown) {
        const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
          ?? 'Failed to create event';
        toast(msg, 'error');
      } finally { setSubmitting(false); }
      return;
    }

    // ── Draft (brand new, or an existing one being edited) ──────────────────
    //
    // Saving always happens. Publishing is a second, optional step gated
    // behind an explicit confirmation below — never bundled silently into the
    // same click as saving, and never the thing that requires a *second*
    // click on a *different* page to actually take effect.
    const doSave = async (alsoPublish: boolean) => {
      setSubmitting(alsoPublish ? 'publish' : 'save');
      let resultId: string | undefined;
      try {
        const body = buildBody();

        if (mode === 'draft' && editId) {
          // Draft: saved straight through. Tiers go to the existing replace
          // route, which already refuses to drop a tier that has sales.
          const { ticket_types, ...eventBody } = body;
          await api.put(`/organiser/events/${editId}`, eventBody);
          if (Array.isArray(ticket_types) && ticket_types.length) {
            await api.put(`/organiser/events/${editId}/ticket-types`, { types: ticket_types });
          }
          resultId = editId;

        } else {
          // The create endpoint always comes back as a draft — see the note
          // above OrgCreateEventHandler on the backend — so on its own this is
          // really just "save"; alsoPublish below is what actually submits it.
          const res = await api.post('/organiser/events/create', body);

          // BEFORE anything that can throw or await. If the line-up write
          // below fails, the event still exists and its local draft must
          // still be gone.
          submittedRef.current = true;
          localStorage.removeItem(DRAFT_KEY);
          setPendingDraft(null);

          resultId = (res.data as { event?: { id?: string } })?.event?.id;

          if (resultId) {
            // ── Line-up and information, written now the event has an id ──
            //
            // Sequential rather than parallel: both hit the same event row
            // and failing one should not obscure which. Each is caught
            // separately so a line-up failure does not lose the information
            // block too. Neither failure undoes the event — it exists, and
            // both are editable on its own page — so the honest outcome is a
            // warning plus a route to where they can be fixed, not an error
            // implying nothing was saved.
            const cleanedLineup = lineup
              .map(l => ({ ...l, name: l.name.trim() }))
              .filter(l => l.name.length > 0)
              .map(({ id: _id, ...rest }, i) => ({ ...rest, sort_order: i }));

            if (cleanedLineup.length > 0) {
              try {
                await replaceEventLineup(resultId, cleanedLineup);
              } catch {
                toast('Event saved, but the line-up did not save — add it on the event page', 'error');
              }
            }

            const infoItems = infoChosen
              .map(key => ({ key, value: (infoValues[key] ?? '').trim() }))
              .filter(i => i.value.length > 0);

            if (infoItems.length > 0) {
              try {
                await replaceEventInfo(resultId, infoItems);
              } catch {
                toast('Event saved, but the information did not save — add it on the event page', 'error');
              }
            }
          }
        }
      } catch (e: unknown) {
        // The save itself failed, so nothing (or nothing new) was written —
        // stay on the form rather than routing away from data the organiser
        // has not actually managed to save yet.
        const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
          ?? 'Failed to save event';
        toast(msg, 'error');
        setSubmitting(false);
        return;
      }

      if (!alsoPublish) {
        toast('Draft saved', 'success');
        setSubmitting(false);
        router.push('/events');
        return;
      }
      if (!resultId) {
        // Defensive only: the try block above either set resultId or threw,
        // which already returned above. Kept so publish is never called
        // with an empty id if that ever stops being true.
        toast('Draft saved', 'success');
        setSubmitting(false);
        router.push('/events');
        return;
      }

      // The save above already succeeded, so the event safely exists either
      // way from here on — a failure now must not be reported as if nothing
      // was saved, and must not route back through 'create', which would
      // create a duplicate event.
      try {
        const res = await api.post(`/organiser/events/${resultId}/publish`, {
          rights_declaration_accepted: true,
        });
        // Private events go live instantly; public ones go to admin review.
        // The backend already words this correctly — surface its message
        // rather than always claiming "submitted for review".
        toast(res.data?.message ?? 'Event published', 'success');
      } catch (e: unknown) {
        const detail = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
        toast(
          detail
            ? `Saved as a draft, but couldn't submit it — ${detail}`
            : "Saved as a draft, but couldn't submit it for review",
          'error',
        );
      } finally {
        setSubmitting(false);
      }
      router.push('/events');
    };

    if (intent === 'publish') {
      // A rights declaration is a legal attestation, so it must be an
      // explicit act by the organiser — never a hardcoded `true` posted on
      // their behalf, and never folded into the same click as saving.
      // Confirming BEFORE anything is written also means Cancel leaves the
      // form exactly as it was: nothing saved, nothing sent.
      confirm(
        'Confirm you hold the rights',
        'You confirm that you hold the rights to host this event and to sell '
        + 'tickets for it, and that the details you have provided are accurate. '
        + 'TicketHawk may remove events and withhold payouts where these rights '
        + 'cannot be evidenced.',
        () => doSave(true),
      );
      return;
    }

    await doSave(false);
  }

  // ── Checklist ─────────────────────────────────────────────────────────────
  const checklist = [
    { done: title.length >= 5,               label: 'Add a title (5+ chars)' },
    { done: description.length >= 50,        label: 'Write a description (50+ chars)' },
    { done: !!categoryId,                    label: 'Choose a category' },
    { done: !!venueName && !!address && !!city,  label: 'Set venue and city' },
    { done: !!startTime && !!endTime,        label: 'Set start and end times' },
    { done: !!expiresAt,                     label: 'Set a booking deadline' },
    // Tiers ARE the pricing once any exist, and a R0 tier (e.g. "Super Early
    // Bird — Free") is a legitimate choice. effectivePrice follows the cheapest
    // visible tier, so testing it for > 0 left this stuck unticked whenever the
    // cheapest tier was free, however many paid tiers sat alongside it.
    { done: isFree || ticketTypes.length > 0 || parseFloat(price) > 0, label: 'Set ticket pricing' },
    { done: !!coverImageUrl,                 label: 'Upload a cover image' },
  ];

  // ─────────────────────────────────────────────────────────────────────────
  return (
    // The <form> IS the page column, rather than sitting inside it.
    //
    // Previously the action bar was `position: sticky` inside the scrolling
    // content, which is why it floated mid-page: a sticky element is trapped
    // inside its containing block, so it drifted with the form's own box
    // instead of pinning to the viewport. Making the form a flex column with
    // the scroller in the middle means the bar is simply the LAST FLEX CHILD —
    // it sits at the bottom because there is nowhere else for it to be, with no
    // sticky maths involved.
    <form onSubmit={(e) => handleSubmit(e, 'save')} noValidate className="page">
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
          <div className="page-title">
            {mode === 'draft'     ? 'Edit draft'
             : mode === 'published' ? 'Request changes'
             : 'Create a new event'}
          </div>
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

          <div>
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
                  subtitle="All times are South African Standard Time (SAST)">
                  {/* Three across — these are the shortest fields on the page and
                      stacking them wasted the most space. */}
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 16 }}>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Start time <Required /></label>
                      {/* min={nowLocal} — the picker already disables days
                          before `min`; it simply was not being given one, so
                          an event could be created in the past. Computed once
                          on mount rather than per render, so the calendar does
                          not re-key itself every keystroke elsewhere in the
                          form. */}
                      <DateTimePicker value={startTime} onChange={setStartTime}
                        min={nowLocal} placeholder="When doors open" style={inp} />
                    </div>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>End time <Required /></label>
                      {/* min={startTime} stops the "end before start" error being
                          reachable at all, rather than catching it on submit. */}
                      {/* Falls back to now when no start is chosen yet —
                          otherwise this field is unconstrained until the user
                          happens to fill the one before it. */}
                      <DateTimePicker value={endTime} onChange={setEndTime}
                        min={startTime || nowLocal} placeholder="When it wraps up" style={inp} />
                    </div>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Booking deadline <Required /></label>
                      {/* max={endTime}: selling tickets after the event has
                          finished is never intended, and the deadline is
                          commonly meant to be the start rather than the end —
                          but some events do sell at the door, so the end is
                          the honest ceiling rather than the start. */}
                      <DateTimePicker value={expiresAt} onChange={setExpiresAt}
                        min={nowLocal} max={endTime || undefined}
                        placeholder="Last moment to buy" style={inp} />
                      <div className="field-hint">Tickets stop selling after this</div>
                    </div>
                  </div>

                  {/* The timezone picker is hidden. `timezone` state still
                      exists, still round-trips through the draft and is still
                      sent on submit — it just isn't a decision the organiser is
                      asked to make. Every event is local, and offering seven
                      zones invited someone to pick the wrong one and shift
                      their own event by hours. */}
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginTop: 20 }}>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Ticket scanning window</label>
                      <Select
                        ariaLabel="Ticket scanning window"
                        value={scanWindowMode}
                        onChange={(v) => setScanWindowMode(v as 'default' | 'custom')}
                        options={[
                          { value: 'default', label: 'Default', hint: '2h before start \u2192 2h after start' },
                          { value: 'custom', label: 'Custom', hint: 'Set my own window' },
                        ]}
                      />
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
                        <label>Closes after start (minutes)</label>
                        <input type="number" min={0} max={10080} value={scanCloseMin}
                          onChange={e => setScanCloseMin(e.target.value)} style={inp}
                          placeholder="120" />
                        <div className="field-hint">Measured from the START, e.g. 840 = 14 hours (all-day)</div>
                      </div>
                    </div>
                  )}
                </FormCard>

                {/* Location */}
                <FormCard title="Location" icon={MapPin}
                  subtitle="Search to pin the exact venue on a map">
                  <LockedFieldset locked={locked('venue_name')} note="Venue and city are fixed once an event is live — ticket holders chose to attend at this place. Cancel and re-list to move it.">
                  {/* Search first — picking a suggestion fills the three fields
                      below AND captures place_id + lat/lng, which is what lets the
                      app and website open the exact venue on a map instead of a
                      fuzzy text search. Manual entry still works if the venue
                      isn't listed. */}
                  <PlacesAutocomplete
                    style={inp}
                    initialValue={placeLabel}
                    onSelect={(p: PlaceResult) => {
                      // Record what the pin was captured with, using the values
                      // actually applied — a Place can come back without a
                      // street number, and those fields keep their old text.
                      const nextVenue = p.venueName || venueName;
                      const nextAddr  = p.address   || address;
                      const nextCity  = p.city      || city;
                      if (p.venueName) setVenueName(nextVenue);
                      if (p.address)   setAddress(nextAddr);
                      if (p.city)      setCity(nextCity);
                      setPlaceId(p.placeId);
                      setLatitude(p.latitude);
                      setLongitude(p.longitude);
                      setPlaceLabel(p.formatted);
                      setPinnedAt({ venueName: nextVenue, address: nextAddr, city: nextCity });
                    }}
                  />
                  {placeId && (
                    <div style={{
                      display: 'flex', alignItems: 'center', gap: 8, margin: '-4px 0 18px',
                      fontSize: 12.5,
                      color: pinStale ? 'var(--warning-fg)' : 'var(--success-fg)',
                    }}>
                      {pinStale
                        ? <Warning size={15} weight="fill" />
                        : <SealCheck size={15} weight="fill" />}
                      <span style={{ flex: 1 }}>
                        {pinStale ? (
                          <>
                            Still pinned to <strong>{placeLabel || 'the place you picked'}</strong>,
                            which no longer matches the address below. Maps open the PIN,
                            not the text — search again to move it, or remove it and
                            let maps search the address.
                          </>
                        ) : (
                          <>Pinned to <strong>{placeLabel || 'the selected place'}</strong></>
                        )}
                      </span>
                      <button type="button" onClick={() => {
                        setPlaceId(''); setLatitude(null); setLongitude(null);
                        setPlaceLabel(''); setPinnedAt(null);
                      }} style={{
                        background: 'none', border: 'none', cursor: 'pointer',
                        color: 'var(--text-3)', fontSize: 12, textDecoration: 'underline',
                        flexShrink: 0, alignSelf: 'flex-start',
                      }}>{pinStale ? 'Remove pin' : 'Clear'}</button>
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

                  {/* Venue type picker removed — every event uses 'none'
                      (straight to checkout) until the stadium / auditorium
                      seating SVGs are reactivated. `venueType` state stays
                      'none' and is omitted from the create payload, so the
                      backend default applies. Restore the picker here to
                      re-enable seat maps. */}

                  </LockedFieldset>
                </FormCard>

                {/* Tickets & pricing */}
                <FormCard title="Tickets & pricing" icon={Ticket}
                  subtitle="Category, price and capacity">
                  <LockedFieldset locked={locked('price')} note="Category, price and capacity are locked on a live event — people have already paid against these terms. Ticket tiers below can still be changed.">
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
                    <div className="field" style={{ marginBottom: 0 }}>
                      <label>Category <Required /></label>
                      <Select
                        ariaLabel="Category"
                        value={categoryId}
                        onChange={setCategoryId}
                        placeholder="Select a category"
                        options={(categories.length > 0
                          ? categories.map(c => ({ value: String(c.id), label: c.name }))
                          : [['1','Music'],['2','Sports'],['3','Arts & Culture'],['4','Food & Drink'],
                             ['5','Technology'],['6','Business'],['7','Health & Wellness'],
                             ['8','Education'],['9','Comedy'],['10','Other'],
                            ].map(([v, l]) => ({ value: v, label: l }))
                        )}
                      />
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
                    <div style={{ marginTop: 18 }}>
                      {/* Base price is SUPERSEDED once a tier exists.
                          Tiers are the alternative to flat pricing, not an
                          addition to it: the backend sets events.price from the
                          tiers and buyers only ever choose between tiers. The
                          field used to stay editable, so entering R100 here and
                          then adding a R150 tier looked like two tickets and
                          produced one — the R100 was silently discarded.
                          Locking it says so before it happens rather than after. */}
                      <LockedFieldset
                        locked={ticketTypes.length > 0}
                        note={`Base price is set by your ticket tiers. ${ticketTypes.length === 1
                          ? 'Remove the tier below'
                          : 'Remove all tiers below'} to price this event with a single flat price instead.`}>
                        <div className="field" style={{ marginBottom: 0, maxWidth: 320 }}>
                          <label>Base price <Required /></label>
                          <div style={{ position: 'relative' }}>
                            <span style={{
                              position: 'absolute', left: 13, top: '50%', transform: 'translateY(-50%)',
                              color: 'var(--text-3)', fontWeight: 700, fontSize: 14, pointerEvents: 'none',
                            }}>R</span>
                            <input type="number" min="0" step="0.01"
                              value={effectivePrice}
                              onChange={e => setPrice(e.target.value)}
                              style={{ ...inp, paddingLeft: 30 }} />
                          </div>
                          {/* Currency selector removed — ZAR only for launch. The
                              `currency` state below stays fixed at 'ZAR' and is
                              still sent in the create payload, so re-adding a
                              picker later needs no backend change. */}
                          <div className="field-hint">
                            {ticketTypes.length > 0
                              ? `Taken from your lowest tier — buyers choose between ${ticketTypes.length} tier${ticketTypes.length === 1 ? '' : 's'} at checkout`
                              : 'Priced in ZAR (South African Rand)'}
                          </div>
                        </div>
                      </LockedFieldset>
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

                  </LockedFieldset>

                  <div style={{ marginTop: 26 }}>
                    <div className="section-sep" style={{ marginTop: 0 }}>Ticket types</div>
                    <TicketTypeBuilder rows={ticketTypes} onChange={setTicketTypes} />
                  </div>
                </FormCard>

                {/* Tags & people */}
                {/* Review reason — published edits only. staff_requests
                    rejects a blank reason at the DB level, and an approver
                    reading a diff with no stated purpose has nothing to
                    evaluate. */}
                {mode === 'published' && (
                  <FormCard title="Reason for this change" icon={NotePencil}
                    subtitle="An admin reads this alongside your changes">
                    <div className="field" style={{ marginBottom: 0 }}>
                      <textarea
                        value={editReason}
                        onChange={e => setEditReason(e.target.value)}
                        rows={3}
                        placeholder="e.g. Doors now open 30 minutes later at the venue's request"
                        style={{ ...inp, resize: 'vertical', minHeight: 78 }} />
                      <div className="field-hint">
                        {editReason.trim().length < 10
                          ? `${10 - editReason.trim().length} more characters needed`
                          : 'Your event stays live and on sale while this is reviewed.'}
                      </div>
                    </div>
                  </FormCard>
                )}

                {/* ── Search keywords ──────────────────────────────────────
                    Renamed from "Tags & people", and PERFORMERS / TEAMS are
                    gone from here.

                    Tags were never shown on the event page — the
                    events_search_trigger in migration 000004 folds them into
                    search_vector at weight B, the same weight as the
                    description. They are invisible search keywords, and
                    labelling them "Tags" beside two fields that DO render made
                    organisers think they appeared on the page.

                    Performers and teams moved to the Line-up tab on the event
                    itself, where each act gets a photo, a stage, a set time
                    and a link. Leaving the flat text versions here would mean
                    two places to edit the same thing, silently disagreeing —
                    the backend writes both from the line-up rows now. */}
                <FormCard title="Search keywords" icon={Tag}
                  subtitle="Helps people find your event — not shown on the page">
                  <div className="field" style={{ marginBottom: 0 }}>
                    <label>Keywords</label>
                    <TagInput tags={tags} onChange={setTags} />
                    <div className="field-hint">
                      Words someone might type when looking for an event like
                      yours — a genre, a vibe, a neighbourhood. These are
                      matched by search but never displayed, so there is no
                      cost to being generous.
                    </div>
                  </div>
                </FormCard>

                {/* ── Line-up ──────────────────────────────────────────────
                    In the wizard, not edit-only.

                    My first pass left this to the event page on the reasoning
                    that an organiser has a date and a venue before a confirmed
                    headliner. That is true, and it is still optional here —
                    but it made the line-up invisible at the moment somebody is
                    actually thinking about their event, which is worse than
                    asking for something they can skip. */}
                <FormCard title="Line-up" icon={Tag}
                  subtitle="Who is playing — optional, and editable any time">
                  <LineupFields
                    entries={lineup}
                    onChange={setLineup}
                    eventStart={startTime || undefined}
                    isSport={false}
                  />
                </FormCard>

                {/* ── Information ──────────────────────────────────────────
                    Parking, age limits, what to bring. Fields left blank get
                    no row at all, so the event page cannot end up showing a
                    heading with nothing under it. */}
                <FormCard title="Information" icon={Info}
                  subtitle="The questions attendees ask before buying">
                  <EventInfoFields
                    catalogue={infoCatalogue}
                    chosen={infoChosen}
                    values={infoValues}
                    onChange={next => { setInfoChosen(next.chosen); setInfoValues(next.values); }}
                  />
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
                      aspect={CROP_COVER}
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
                  <Select
                    ariaLabel="Event type"
                    value={eventType}
                    onChange={(v) => {
                      if (v !== 'private_free' && kycLevel < 2) { router.push('/kyc'); return; }
                      setEventType(v as typeof eventType);
                    }}
                    options={[
                      { value: 'private_free', label: 'Private free event' },
                      {
                        value: 'private_paid', label: 'Private paid event',
                        hint: kycLevel < 2 ? 'Needs full verification' : undefined,
                        disabled: kycLevel < 2,
                      },
                      {
                        value: 'public', label: 'Public event',
                        hint: kycLevel < 2 ? 'Needs full verification' : undefined,
                        disabled: kycLevel < 2,
                      },
                    ]}
                  />
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

          </div>
        </div>
      </div>

      {/* ── Action bar ───────────────────────────────────────────────────
          Outside .page-content, so it is never scrolled and never overlaps a
          field. flex-shrink:0 stops it being squeezed when the form is tall. */}
      <div style={{
        flexShrink: 0,
        borderTop: '1px solid var(--border)',
        background: 'var(--bg-elevated)',
        padding: '14px 28px',
      }}>
        <div style={{
          maxWidth: 1440, margin: '0 auto',
          display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap',
        }}>
          {mode === 'published' ? (
            <button type="button" className="btn btn-primary" disabled={!!submitting}
              onClick={(e) => handleSubmit(e, 'publish')}>
              {submitting
                ? <><span className="spinner" style={{ width: 14, height: 14, borderWidth: 2 }} /> Submitting…</>
                : <><NotePencil size={15} weight="bold" /> Send for review</>}
            </button>
          ) : (
            <>
              {/* Saving never asks for the rights-declaration confirmation —
                  only Submit does, below — so this always completes in one
                  click with nothing further to confirm. */}
              <button type="button" className="btn" disabled={!!submitting}
                onClick={(e) => handleSubmit(e, 'save')}>
                {submitting === 'save'
                  ? <><span className="spinner" style={{ width: 14, height: 14, borderWidth: 2 }} /> Saving…</>
                  : <><FloppyDisk size={15} weight="bold" /> Save draft</>}
              </button>
              <button type="button" className="btn btn-primary" disabled={!!submitting}
                onClick={(e) => handleSubmit(e, 'publish')}>
                {submitting === 'publish'
                  ? <><span className="spinner" style={{ width: 14, height: 14, borderWidth: 2 }} /> Submitting…</>
                  : <><Confetti size={15} weight="bold" /> {eventType === 'public' ? 'Submit for review' : 'Publish event'}</>}
              </button>
            </>
          )}
          <button type="button" className="btn" disabled={!!submitting}
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
      </div>
    </form>
  );
}