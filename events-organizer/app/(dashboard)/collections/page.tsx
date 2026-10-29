'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Collections
//
// Group a tour's dates (or any set of your events) into ONE card on the
// TicketHawk home feed. Tapping the card opens a page listing every date.
// Search, category pages and the map still show each date on its own, so a
// visitor looking for "Cape Town" still finds the Cape Town stop.
//
// Rules that come from the backend (events-db, migration 000096) and are only
// EXPLAINED here, never re-decided:
//   • an event belongs to at most one collection — adding it here moves it;
//   • the feed shows the card only once 2+ dates are live and public
//     (live_event_count); private, draft and finished dates can be members but
//     never appear;
//   • no review step — a collection only ever shows events already approved.
// ─────────────────────────────────────────────────────────────────────────────
import { useCallback, useEffect, useMemo, useState } from 'react';
import { isBackdropPress } from '../../../lib/backdrop';
import Link from 'next/link';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';
import { fmtDate } from '../../../lib/utils';
import { StatusPill } from '@/components/ui';
import { Icon } from '@/components/icons';
import ImageCropper, { CROP_COVER } from '@/components/imageCropper';
import {
  EventCollection, MIN_LIVE_FOR_CARD,
  listEventCollections, createEventCollection, updateEventCollection,
  setEventCollectionEvents, deleteEventCollection,
  uploadEventCollectionCover, removeEventCollectionCover,
} from '../../../lib/eventCollections';

interface OrgEvent {
  id: string;
  title: string;
  status: string;
  start_time: string;
  end_time?: string;
  city: string;
  venue_name: string;
}

const SITE = process.env.NEXT_PUBLIC_SITE_URL || 'https://ticket-hawk.com';

/** Would the public see this date on the feed right now? */
function isLivePublic(e: OrgEvent, now: number): boolean {
  const end = e.end_time ? new Date(e.end_time).getTime() : new Date(e.start_time).getTime();
  return e.status === 'published' && end >= now;
}

function memberNote(e: OrgEvent, now: number): string | null {
  if (isLivePublic(e, now)) return null;
  if (e.status === 'published_private') return 'Private — never shown on the feed';
  if (e.status === 'published' || e.status === 'completed') return 'Finished — no longer shown';
  if (e.status === 'cancelled') return 'Cancelled — not shown';
  return 'Not live yet — shows once published';
}

export default function CollectionsPage() {
  const toast = useToast();
  const confirm = useConfirm();

  const [collections, setCollections] = useState<EventCollection[]>([]);
  const [events, setEvents] = useState<OrgEvent[]>([]);
  const [loading, setLoading] = useState(true);
  // null = closed; 'new' = creating; otherwise the collection being edited.
  const [editing, setEditing] = useState<EventCollection | 'new' | null>(null);
  const [helpOpen, setHelpOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      const [cols, evRes] = await Promise.all([
        listEventCollections(),
        api.get<{ events: OrgEvent[] }>('/organiser/events'),
      ]);
      setCollections(cols);
      setEvents(evRes.data.events ?? []);
    } catch {
      toast('Could not load your collections', 'error');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => { load(); }, [load]);

  function remove(c: EventCollection) {
    confirm(
      'Delete this collection?',
      `"${c.title}" will be removed. Its ${c.event_count} ${c.event_count === 1 ? 'event goes' : 'events go'} `
      + 'back to appearing individually on the feed — the events themselves are not changed.',
      async () => {
        try {
          await deleteEventCollection(c.id);
          setCollections(prev => prev.filter(x => x.id !== c.id));
          toast('Collection deleted', 'success');
        } catch (e: unknown) {
          const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
          toast(msg ?? 'Could not delete the collection', 'error');
        }
      },
    );
  }

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <div className="page-title">Collections</div>
          <div style={{ fontSize: 13, color: 'var(--text-2)', marginTop: 2 }}>
            Show a tour&apos;s dates as one card on the TicketHawk feed
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
          <button type="button" className="btn" onClick={() => setHelpOpen(true)}>
            <Icon name="info" size={15} />
            How collections work
          </button>
          <button type="button" className="btn btn-primary" onClick={() => setEditing('new')}>
            New collection
          </button>
        </div>
      </div>

      <div className="page-content">
        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 40 }}>
            <div className="spinner" />
          </div>
        ) : collections.length === 0 ? (
          <div className="empty-state">
            <div className="empty-icon"><Icon name="layers" size={26} /></div>
            <div className="empty-title">No collections yet</div>
            <div className="empty-desc">
              Create one for your next tour, then add its dates.
            </div>
            <button type="button" className="btn btn-primary" style={{ marginTop: 6 }}
              onClick={() => setEditing('new')}>
              New collection
            </button>
          </div>
        ) : (
          <div style={{
            display: 'grid', gap: 16,
            gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 300px), 1fr))',
          }}>
            {collections.map(c => (
              <CollectionCard key={c.id} c={c}
                onEdit={() => setEditing(c)} onDelete={() => remove(c)} />
            ))}
          </div>
        )}
      </div>

      {helpOpen && <CollectionsHelp onClose={() => setHelpOpen(false)} />}

      {editing && (
        <CollectionEditor
          initial={editing === 'new' ? null : editing}
          events={events}
          collections={collections}
          onClose={() => setEditing(null)}
          onSaved={(saved) => {
            setCollections(prev => {
              const rest = prev.filter(x => x.id !== saved.id);
              // Membership moves between collections, so refresh the others'
              // counts too rather than trusting the stale copies.
              return [saved, ...rest];
            });
            setEditing(null);
            load();
          }}
        />
      )}
    </div>
  );
}

// ── Help ──────────────────────────────────────────────────────────────────────

function CollectionsHelp({ onClose }: { onClose: () => void }) {
  return (
    <div className="modal-overlay" onMouseDown={e => { if (isBackdropPress(e)) onClose(); }}>
      <div className="modal modal-sm" role="dialog" aria-modal="true" aria-labelledby="col-help-title">
        <div className="modal-header">
          <div className="modal-title" id="col-help-title">How collections work</div>
          <button type="button" className="btn-icon" onClick={onClose} aria-label="Close">
            <Icon name="x" size={16} />
          </button>
        </div>
        <div className="modal-body" style={{ display: 'grid', gap: 12, fontSize: 14, lineHeight: 1.6, color: 'var(--text-2)' }}>
          <p style={{ margin: 0 }}>
            Hosting the same event in several cities or on several dates? Group those
            events into a collection.
          </p>
          <p style={{ margin: 0 }}>
            On the home feed, a collection appears as <strong style={{ color: 'var(--text)' }}>one</strong> card
            instead of one card per date. Tapping the card opens a page that lists every date.
          </p>
          <p style={{ margin: 0 }}>
            A collection only appears as a single card while it has at least two upcoming,
            published dates. With fewer, the event shows on its own as usual.
          </p>
          <p style={{ margin: 0 }}>
            Search results and category pages still show each event separately.
          </p>
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-primary" onClick={onClose}>Got it</button>
        </div>
      </div>
    </div>
  );
}

// ── Card ──────────────────────────────────────────────────────────────────────

function CollectionCard({ c, onEdit, onDelete }: {
  c: EventCollection; onEdit: () => void; onDelete: () => void;
}) {
  const onFeed = c.live_event_count >= MIN_LIVE_FOR_CARD;
  return (
    <div className="card" style={{ display: 'flex', flexDirection: 'column' }}>
      {/* Fixed 16:9 frame. The image is absolutely positioned so a cover of
          any shape (a tall poster, say) is cropped to the frame instead of
          stretching it — an in-flow <img> made the box as tall as the image. */}
      <div style={{ position: 'relative', aspectRatio: '16 / 9', overflow: 'hidden', background: 'var(--surface-2)' }}>
        {c.cover_image_url ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={c.cover_image_url} alt="" style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', objectFit: 'cover', display: 'block' }} />
        ) : (
          <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', color: 'var(--text-3)', fontSize: 12.5 }}>
            No cover yet
          </div>
        )}
        <span style={{
          position: 'absolute', top: 10, right: 10,
          display: 'inline-flex', alignItems: 'center', gap: 5,
          background: 'rgba(0,0,0,.62)', color: '#fff', borderRadius: 999,
          padding: '4px 10px', fontSize: 11.5, fontWeight: 700,
        }}>
          <Icon name="layers" size={12} /> {c.event_count} {c.event_count === 1 ? 'event' : 'events'}
        </span>
      </div>
      <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 8, flex: 1 }}>
        <div style={{ fontSize: 15.5, fontWeight: 800, overflowWrap: 'anywhere' }}>{c.title}</div>
        <div style={{
          display: 'flex', alignItems: 'flex-start', gap: 6, fontSize: 12.5,
          color: onFeed ? 'var(--success-fg)' : 'var(--warning-fg)',
        }}>
          <Icon name={onFeed ? 'check-circle' : 'alert-circle'} size={14} />
          <span>
            {onFeed
              ? `On the feed as one card · ${c.live_event_count} live dates`
              : `Not on the feed yet — needs ${MIN_LIVE_FOR_CARD}+ live public dates (has ${c.live_event_count})`}
          </span>
        </div>
        <div style={{ display: 'flex', gap: 8, marginTop: 'auto', paddingTop: 6, flexWrap: 'wrap' }}>
          <button type="button" className="btn btn-sm btn-primary" onClick={onEdit}>Edit</button>
          {onFeed && (
            <a className="btn btn-sm" href={`${SITE}/collections/${c.slug}`} target="_blank" rel="noopener noreferrer">
              View <Icon name="external-link" size={12} />
            </a>
          )}
          <button type="button" className="btn btn-sm btn-danger" onClick={onDelete} style={{ marginLeft: 'auto' }}>
            Delete
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Editor ────────────────────────────────────────────────────────────────────

function CollectionEditor({ initial, events, collections, onClose, onSaved }: {
  initial: EventCollection | null;
  events: OrgEvent[];
  collections: EventCollection[];
  onClose: () => void;
  onSaved: (c: EventCollection) => void;
}) {
  const toast = useToast();
  const [title, setTitle] = useState(initial?.title ?? '');
  const [description, setDescription] = useState(initial?.description ?? '');
  const [selected, setSelected] = useState<Set<string>>(() => new Set(initial?.event_ids ?? []));
  const [query, setQuery] = useState('');

  // Cover: the saved URL, a picked-and-cropped file waiting to upload, or
  // removed. Nothing is uploaded until Save, so cancelling costs nothing.
  const [coverUrl, setCoverUrl] = useState<string | null>(initial?.cover_image_url ?? null);
  const [coverFile, setCoverFile] = useState<File | null>(null);
  const [coverPreview, setCoverPreview] = useState<string | null>(null);
  const [cropping, setCropping] = useState<File | null>(null);
  const [saving, setSaving] = useState(false);
  // One clock reading for the editor's lifetime — render stays pure, and
  // "upcoming" cannot flicker between renders.
  const [now] = useState(() => Date.now());

  useEffect(() => () => { if (coverPreview) URL.revokeObjectURL(coverPreview); }, [coverPreview]);

  // Which OTHER collection each event is currently in — so the list can say
  // "will move from …" before it happens.
  const otherHome = useMemo(() => {
    const m = new Map<string, string>();
    for (const c of collections) {
      if (initial && c.id === initial.id) continue;
      for (const id of c.event_ids) m.set(id, c.title);
    }
    return m;
  }, [collections, initial]);

  // Upcoming first (soonest at top), then past, newest first.
  const sorted = useMemo(() => {
    const upcoming = events.filter(e => new Date(e.start_time).getTime() >= now)
      .sort((a, b) => +new Date(a.start_time) - +new Date(b.start_time));
    const past = events.filter(e => new Date(e.start_time).getTime() < now)
      .sort((a, b) => +new Date(b.start_time) - +new Date(a.start_time));
    const q = query.trim().toLowerCase();
    return [...upcoming, ...past].filter(e =>
      !q || `${e.title} ${e.city} ${e.venue_name}`.toLowerCase().includes(q));
  }, [events, query, now]);

  const liveSelected = events.filter(e => selected.has(e.id) && isLivePublic(e, now)).length;
  const moving = [...selected].filter(id => otherHome.has(id)).length;

  function toggle(id: string) {
    setSelected(prev => {
      const n = new Set(prev);
      if (n.has(id)) n.delete(id); else n.add(id);
      return n;
    });
  }

  function pickCover(f: File | undefined) {
    if (!f) return;
    if (!['image/jpeg', 'image/png', 'image/webp'].includes(f.type)) {
      toast('Use a JPEG, PNG or WebP image', 'error'); return;
    }
    if (f.size > 10 * 1024 * 1024) { toast('That image is over 10 MB', 'error'); return; }
    setCropping(f);
  }

  async function save() {
    const t = title.trim();
    if (t.length < 2) { toast('Give the collection a title', 'error'); return; }
    setSaving(true);
    const ids = [...selected];
    try {
      let saved: EventCollection;
      if (!initial) {
        saved = await createEventCollection({ title: t, description: description.trim(), event_ids: ids });
      } else {
        saved = initial;
        if (t !== initial.title || description.trim() !== (initial.description ?? '')) {
          saved = await updateEventCollection(initial.id, { title: t, description: description.trim() });
        }
        const before = [...initial.event_ids].sort().join(',');
        if ([...ids].sort().join(',') !== before) {
          saved = await setEventCollectionEvents(initial.id, ids);
        }
        if (!coverFile && !coverUrl && initial.cover_image_url) {
          await removeEventCollectionCover(initial.id);
          saved = { ...saved, cover_image_url: null };
        }
      }
      if (coverFile) {
        try {
          const url = await uploadEventCollectionCover(saved.id, coverFile);
          saved = { ...saved, cover_image_url: url };
        } catch (e: unknown) {
          // The collection itself saved; only the image did not. Say so
          // precisely rather than implying nothing was saved.
          toast(`Saved, but the cover did not upload: ${e instanceof Error ? e.message : 'try again'}`, 'error');
          onSaved(saved);
          return;
        }
      }
      toast(initial ? 'Collection updated' : 'Collection created', 'success');
      onSaved(saved);
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      toast(msg ?? 'Could not save the collection', 'error');
    } finally {
      setSaving(false);
    }
  }

  const shownCover = coverPreview ?? coverUrl;

  return (
    <div className="modal-overlay" onMouseDown={e => { if (isBackdropPress(e) && !saving) onClose(); }}>
      {cropping && (
        <ImageCropper
          file={cropping}
          aspect={CROP_COVER}
          title="Crop collection cover"
          hint="This is the card people see on the home feed."
          onCancel={() => setCropping(null)}
          onConfirm={f => {
            setCropping(null);
            if (coverPreview) URL.revokeObjectURL(coverPreview);
            setCoverFile(f);
            setCoverPreview(URL.createObjectURL(f));
          }}
        />
      )}

      <div className="modal modal-lg" role="dialog" aria-modal="true" aria-labelledby="col-editor-title">
        <div className="modal-header">
          <div className="modal-title" id="col-editor-title">
            {initial ? 'Edit collection' : 'New collection'}
          </div>
          <button type="button" className="btn-icon" onClick={onClose} disabled={saving} aria-label="Close">
            <Icon name="x" size={16} />
          </button>
        </div>

        <div className="modal-body">
          <div className="rgrid" style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)', gap: 20 }}>
            <div>
              <div className="field">
                <label htmlFor="col-title">Title</label>
                <input id="col-title" value={title} maxLength={120}
                  placeholder="e.g. Afrochild Collab Pop-Up Tour"
                  onChange={e => setTitle(e.target.value)} />
              </div>
              <div className="field" style={{ marginBottom: 0 }}>
                <label htmlFor="col-desc">Description <span style={{ textTransform: 'none', fontWeight: 400 }}>(optional)</span></label>
                <textarea id="col-desc" rows={5} maxLength={2000} value={description}
                  placeholder="What the tour is about. Shown at the top of the collection page."
                  onChange={e => setDescription(e.target.value)} />
              </div>
            </div>

            <div className="field" style={{ marginBottom: 0 }}>
              <label>Cover</label>
              <div style={{
                position: 'relative', aspectRatio: '16 / 9', borderRadius: 'var(--r-lg)',
                border: `1.5px ${shownCover ? 'solid' : 'dashed'} var(--border-2)`,
                background: 'var(--surface-alpha)', overflow: 'hidden',
              }}>
                {shownCover ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={shownCover} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }} />
                ) : (
                  <label style={{
                    position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column',
                    alignItems: 'center', justifyContent: 'center', gap: 6, cursor: 'pointer',
                    color: 'var(--text-3)', fontSize: 12.5, fontWeight: 600, textTransform: 'none', letterSpacing: 0,
                  }}>
                    <Icon name="upload" size={22} />
                    Choose a cover image
                    <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }}
                      onChange={e => { pickCover(e.target.files?.[0]); e.target.value = ''; }} />
                  </label>
                )}
              </div>
              {shownCover && (
                <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                  <label className="btn btn-sm" style={{ cursor: 'pointer', textTransform: 'none', letterSpacing: 0, color: 'var(--text)', fontSize: 12.5, marginBottom: 0 }}>
                    Replace
                    <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }}
                      onChange={e => { pickCover(e.target.files?.[0]); e.target.value = ''; }} />
                  </label>
                  <button type="button" className="btn btn-sm" onClick={() => {
                    if (coverPreview) URL.revokeObjectURL(coverPreview);
                    setCoverPreview(null); setCoverFile(null); setCoverUrl(null);
                  }}>
                    Remove
                  </button>
                </div>
              )}
              <div className="field-hint">
                Shown as the card on the home feed — your tour poster works well.
                Without one, the first date&apos;s cover is used.
              </div>
            </div>
          </div>

          <div className="section-sep">Events in this collection</div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap', marginBottom: 10 }}>
            <div className="search-box" style={{ flex: '1 1 220px', minWidth: 0 }}>
              <Icon name="search" size={14} />
              <input value={query} onChange={e => setQuery(e.target.value)} placeholder="Search your events…" />
            </div>
            <span style={{
              fontSize: 12.5, fontWeight: 600,
              color: liveSelected >= MIN_LIVE_FOR_CARD ? 'var(--success-fg)' : 'var(--warning-fg)',
            }}>
              {selected.size} selected · {liveSelected} live
              {liveSelected < MIN_LIVE_FOR_CARD && ` — ${MIN_LIVE_FOR_CARD}+ live needed for the feed card`}
            </span>
          </div>

          {moving > 0 && (
            <div className="alert alert-warning" style={{ fontSize: 13 }}>
              <Icon name="alert-triangle" size={15} />
              <span>
                {moving} selected {moving === 1 ? 'event is' : 'events are'} in another collection
                and will move here — an event can only be in one collection.
              </span>
            </div>
          )}

          <div style={{
            border: '1px solid var(--border)', borderRadius: 'var(--r-lg)',
            maxHeight: 340, overflowY: 'auto',
          }}>
            {sorted.length === 0 ? (
              <div style={{ padding: 20, textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>
                {events.length === 0
                  ? <>You have no events yet. <Link href="/events/create">Create one</Link> first.</>
                  : 'No events match your search.'}
              </div>
            ) : sorted.map((e, i) => {
              const on = selected.has(e.id);
              const note = memberNote(e, now);
              const from = otherHome.get(e.id);
              return (
                <label key={e.id} style={{
                  display: 'flex', alignItems: 'flex-start', gap: 12, padding: '11px 14px',
                  borderTop: i === 0 ? 'none' : '1px solid var(--border)', cursor: 'pointer',
                  background: on ? 'var(--accent-softer)' : 'transparent',
                }}>
                  <input type="checkbox" checked={on} onChange={() => toggle(e.id)}
                    style={{ width: 16, height: 16, marginTop: 2, flexShrink: 0, accentColor: 'var(--accent-deep)' }} />
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 13.5, fontWeight: 600, overflowWrap: 'anywhere' }}>{e.title}</div>
                    <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 2 }}>
                      {fmtDate(e.start_time)} · {e.city || e.venue_name || 'No location'}
                    </div>
                    {(note || from) && (
                      <div style={{ fontSize: 11.5, color: 'var(--warning-fg)', marginTop: 3 }}>
                        {[note, from && `In "${from}"${on ? ' — will move here' : ''}`].filter(Boolean).join(' · ')}
                      </div>
                    )}
                  </div>
                  <StatusPill status={e.status} style={{ flexShrink: 0 }} />
                </label>
              );
            })}
          </div>
        </div>

        <div className="modal-footer">
          <button type="button" className="btn" onClick={onClose} disabled={saving}>Cancel</button>
          <button type="button" className="btn btn-primary" onClick={save} disabled={saving}>
            {saving ? 'Saving…' : initial ? 'Save changes' : 'Create collection'}
          </button>
        </div>
      </div>
    </div>
  );
}