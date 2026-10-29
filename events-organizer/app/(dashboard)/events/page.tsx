'use client';
import { useState, useEffect, useCallback } from 'react';
import { isBackdropPress } from '../../../lib/backdrop';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';
import { fmtMoney, fmtDate, fmtDateTime } from '../../../lib/utils';
import { StatusPill } from '@/components/ui';

const DRAFT_KEY = 'org_event_draft';

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60_000);
  const h = Math.floor(diff / 3_600_000);
  if (m < 1)  return 'just now';
  if (m < 60) return `${m}m ago`;
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

interface OrgEvent {
  id: string;
  title: string;
  status: string;
  start_time: string;
  end_time: string;
  city: string;
  venue_name: string;
  address: string;
  price: number;
  currency: string;
  tickets_sold?: number;   // optional — backend may omit
  max_capacity: number | null;
  views?: number;          // optional — backend may omit
  created_at: string;
}

type StatusFilter = 'all' | 'published' | 'draft' | 'cancelled' | 'completed';
type SortKey = 'title' | 'start_time' | 'tickets_sold' | 'views' | 'created_at';

export default function EventsPage() {
  const toast = useToast();
  const confirm = useConfirm();
  const router = useRouter();

  const [events, setEvents] = useState<OrgEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [search, setSearch] = useState('');
  const [sortKey, setSortKey] = useState<SortKey>('start_time');
  const [sortAsc, setSortAsc] = useState(true);
  const [draftInfo, setDraftInfo] = useState<{ savedAt: string; title: string } | null>(null);

  const fetchEvents = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.get('/organiser/events');
      setEvents(res.data.events ?? []);
    } catch {
      toast('Failed to load events', 'error');
    } finally { setLoading(false); }
  }, []);

  useEffect(() => { fetchEvents(); }, [fetchEvents]);

  // Check for saved draft on mount
  useEffect(() => {
    try {
      const raw = localStorage.getItem(DRAFT_KEY);
      if (!raw) return;
      const d = JSON.parse(raw) as { savedAt: string; title: string };
      if (d.savedAt) setDraftInfo({ savedAt: d.savedAt, title: d.title ?? '' });
    } catch { /* ignore */ }
  }, []);

  // ── Filter + sort ───────────────────────────────────────────────────────────
  const filtered = events
    .filter(e => {
      if (statusFilter !== 'all' && e.status !== statusFilter) return false;
      // Guarded: city was absent from the API payload entirely, so typing in
      // the search box called .toLowerCase() on undefined and blew up the
      // whole list. The backend now sends it, but the guard stays — one
      // missing optional field should not take the page down.
      if (search) {
        const q = search.toLowerCase();
        const hay = `${e.title ?? ''} ${e.city ?? ''} ${e.venue_name ?? ''}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    })
    .sort((a, b) => {
      let valA: string | number = a[sortKey] as string | number ?? 0;
      let valB: string | number = b[sortKey] as string | number ?? 0;
      if (typeof valA === 'string') valA = valA.toLowerCase();
      if (typeof valB === 'string') valB = valB.toLowerCase();
      if (valA < valB) return sortAsc ? -1 : 1;
      if (valA > valB) return sortAsc ? 1 : -1;
      return 0;
    });

  function handleSort(key: SortKey) {
    if (sortKey === key) setSortAsc(p => !p);
    else { setSortKey(key); setSortAsc(true); }
  }

  function SortIcon({ k }: { k: SortKey }) {
    if (sortKey !== k) return <span style={{ color: 'var(--border-2)', fontSize: 10 }}>↕</span>;
    return <span style={{ color: 'var(--accent)', fontSize: 10 }}>{sortAsc ? '↑' : '↓'}</span>;
  }

  // ── Selection ───────────────────────────────────────────────────────────────
  function toggle(id: string) {
    setSelected(prev => { const n = new Set(prev); n.has(id) ? n.delete(id) : n.add(id); return n; });
  }
  function toggleAll() {
    setSelected(selected.size === filtered.length ? new Set() : new Set(filtered.map(e => e.id)));
  }
  const allSelected = selected.size === filtered.length && filtered.length > 0;

  // ── Counts by status ────────────────────────────────────────────────────────
  const counts = {
    all: events.length,
    published: events.filter(e => e.status === 'published').length,
    draft: events.filter(e => e.status === 'draft').length,
    cancelled: events.filter(e => e.status === 'cancelled').length,
    completed: events.filter(e => e.status === 'completed').length,
  };

  // ── Actions ─────────────────────────────────────────────────────────────────
  // The API requires a reason of at least 10 characters and rejects the request
  // without one, so this prompts rather than posting an empty body.
  // ── Delete a draft ──────────────────────────────────────────────────────────
  // Hard delete. The backend refuses anything that is not status='draft', and
  // bookings.event_id is ON DELETE RESTRICT, so a draft that somehow carries a
  // booking is rejected by Postgres rather than silently cascading away.
  async function deleteDraft(id: string, title: string) {
    confirm(
      'Delete this draft?',
      `"${title}" will be permanently deleted, along with any ticket tiers and `
      + 'images set up on it. This cannot be undone.',
      async () => {
        try {
          await api.delete(`/organiser/events/${id}`);
          toast('Draft deleted', 'success');
          setEvents(prev => prev.filter(ev => ev.id !== id));
          setSelected(prev => { const n = new Set(prev); n.delete(id); return n; });
        } catch (e: unknown) {
          const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
          toast(msg ?? 'Failed to delete draft', 'error');
        }
      },
    );
  }

  const [cancelId, setCancelId] = useState<string | null>(null);
  const [cancelReason, setCancelReason] = useState('');
  const [cancelBusy, setCancelBusy] = useState(false);

  async function submitCancel() {
    if (!cancelId || cancelReason.trim().length < 10) return;
    setCancelBusy(true);
    try {
      await api.post(`/organiser/events/${cancelId}/cancel`, { reason: cancelReason.trim() });
      toast('Event cancelled — attendees are being refunded', 'success');
      setCancelId(null);
      setCancelReason('');
      fetchEvents();
    } catch (e) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      toast(msg ?? 'Could not cancel the event', 'error');
    } finally {
      setCancelBusy(false);
    }
  }

  function cancelEvent(id: string) {
    setCancelReason('');
    setCancelId(id);
  }

  // Publishing requires the rights declaration. The backend
  // (models.RequirementsFor) sets RightsDeclaration: true for EVERY event
  // class, and PublishEvent rejects with ErrDeclarationRequired →
  // 400 "Please confirm you hold the rights…" unless events.rights_declared_at
  // is already stamped or the request body carries the flag. Neither the
  // create form nor this call ever sent it, so publishing was a hard dead end
  // for every event on the platform.
  //
  // A rights declaration is a legal attestation, so it must be an explicit act
  // by the organiser — never a hardcoded `true` posted on their behalf. We put
  // the wording in front of them and only send the flag once they accept.
  async function publishEvent(id: string) {
    confirm(
      'Confirm you hold the rights',
      'You confirm that you hold the rights to host this event and to sell '
      + 'tickets for it, and that the details you have provided are accurate. '
      + 'TicketHawk may remove events and withhold payouts where these rights '
      + 'cannot be evidenced.',
      async () => {
        try {
          const res = await api.post(`/organiser/events/${id}/publish`, {
            rights_declaration_accepted: true,
          });
          // Private events go live instantly; public ones go to admin review.
          // The backend already words this correctly — surface its message
          // rather than always claiming "submitted for review".
          toast(res.data?.message ?? 'Event published', 'success');
          fetchEvents();
        } catch (e: unknown) {
          const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
          toast(msg ?? 'Failed to publish', 'error');
        }
      },
    );
  }

  // ── Capacity fill % ─────────────────────────────────────────────────────────
  function capacityPct(e: OrgEvent) {
    if (!e.max_capacity) return null;
    return Math.min(((e.tickets_sold ?? 0) / e.max_capacity) * 100, 100);
  }

  return (
    <div className="page">

      {/* ── Page header ───────────────────────────────────────────────── */}
      <div className="page-header">
        <div>
          <div className="page-title">My Events</div>
          <div style={{ fontSize: 13, color: 'var(--text-2)', marginTop: 2 }}>
            {counts.published} published · {counts.draft} draft
          </div>
        </div>
        <Link href="/events/create" className="btn btn-primary">Create event</Link>
      </div>

      {/* ── Filter pill strip ────────────────────────────────────────── */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 6,
        padding: '8px 16px', borderBottom: '1px solid var(--border)',
        background: 'var(--surface)', flexWrap: 'wrap',
      }}>
        {(Object.entries(counts) as [StatusFilter, number][]).map(([s, count]) => (
          <button
            key={s}
            onClick={() => { setStatusFilter(s); setSelected(new Set()); }}
            style={{
              padding: '5px 12px', borderRadius: 16, fontSize: 13, fontWeight: 600,
              border: `1px solid ${statusFilter === s ? 'var(--accent)' : 'var(--border)'}`,
              background: statusFilter === s ? 'var(--accent-soft)' : 'var(--surface)',
              color: statusFilter === s ? 'var(--accent)' : 'var(--text-2)',
              cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6,
            }}
          >
            {s === 'all' ? 'All' : s.charAt(0).toUpperCase() + s.slice(1)}
            {count > 0 && (
              <span style={{
                background: statusFilter === s ? 'var(--accent)' : 'var(--surface-3)',
                color: statusFilter === s ? 'var(--surface)' : 'var(--text-2)',
                borderRadius: 99, padding: '0 5px', fontSize: 11,
              }}>
                {count}
              </span>
            )}
          </button>
        ))}

        <div className="filter-search" style={{ marginLeft: 'auto' }}>
          <div className="search-box">
            <svg viewBox="0 0 20 20" fill="var(--text-2)" width="14" height="14">
              <path fillRule="evenodd" d="M8 4a4 4 0 100 8 4 4 0 000-8zM2 8a6 6 0 1110.89 3.476l4.817 4.817a1 1 0 01-1.414 1.414l-4.816-4.816A6 6 0 012 8z" clipRule="evenodd"/>
            </svg>
            <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search events…" />
          </div>
        </div>
      </div>

      {/* ── Draft banner ─────────────────────────────────────────────── */}
      {draftInfo && (
        <div style={{
          display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap',
          padding: '10px 16px', background: 'var(--warning-bg)',
          borderBottom: '1px solid var(--warning-border)', color: 'var(--warning-fg)',
        }}>
          <span style={{ fontSize: 16 }}>📝</span>
          <span style={{ fontSize: 13, flex: 1, minWidth: 140 }}>
            <strong>Unsaved draft</strong>
            {draftInfo.title ? ` — "${draftInfo.title}"` : ''}
            <span style={{ fontWeight: 400 }}> · saved {relativeTime(draftInfo.savedAt)}</span>
          </span>
          <div style={{ display: 'flex', gap: 8 }}>
            <button className="btn btn-sm btn-primary" onClick={() => router.push('/events/create')}>
              Resume draft
            </button>
            <button className="btn btn-sm" onClick={() => {
              localStorage.removeItem(DRAFT_KEY);
              setDraftInfo(null);
            }}>
              Discard
            </button>
          </div>
        </div>
      )}

      {/* ── Action toolbar — shows when rows are selected ─────────────── */}
      {selected.size > 0 && (
        <div className="toolbar" style={{ background: 'var(--accent-soft)', borderBottom: '1px solid var(--accent-border)' }}>
          <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--accent)' }}>
            {selected.size} selected
          </span>
          <button className="btn btn-sm" onClick={() => {
            const id = [...selected][0];
            if (id) router.push(`/event/${id}`);
          }}>
            View details
          </button>
          <button className="btn btn-sm btn-danger"
            onClick={() => { selected.forEach(id => cancelEvent(id)); }}>
            Cancel selected
          </button>
          <button className="btn btn-sm" style={{ marginLeft: 'auto' }}
            onClick={() => setSelected(new Set())}>
            Cancel
          </button>
        </div>
      )}

      {/* ── Table ────────────────────────────────────────────────────── */}
      <div style={{ flex: 1, overflow: 'auto', background: 'var(--surface)' }}>
        {loading ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 200 }}>
            <div className="spinner" />
          </div>
        ) : filtered.length === 0 ? (
          <div className="empty-state">
            <div className="empty-icon">📅</div>
            <div className="empty-title">
              {search || statusFilter !== 'all' ? 'No events match your filters' : 'No events yet'}
            </div>
            <div className="empty-desc">
              {search || statusFilter !== 'all'
                ? 'Try adjusting your search or filter.'
                : 'Events you create will appear here once an admin sets them up for your organizer account.'}
            </div>
          </div>
        ) : (
          <table className="meta-table mobile-cards">
            <thead>
              <tr>
                <th className="col-check">
                  <input type="checkbox" checked={allSelected} onChange={toggleAll}
                    style={{ accentColor: 'var(--accent)', cursor: 'pointer' }} />
                </th>
                <th className="sortable" onClick={() => handleSort('title')}>
                  Event <SortIcon k="title" />
                </th>
                <th>Status</th>
                <th className="sortable" onClick={() => handleSort('start_time')}>
                  Date <SortIcon k="start_time" />
                </th>
                <th>Location</th>
                <th className="sortable" onClick={() => handleSort('tickets_sold')}>
                  Tickets sold <SortIcon k="tickets_sold" />
                </th>
                <th className="sortable" onClick={() => handleSort('views')}>
                  Views <SortIcon k="views" />
                </th>
                <th>Price</th>
                <th className="col-actions">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map(e => {
                const pct = capacityPct(e);
                const ticketsSold = e.tickets_sold ?? 0;
                const views = e.views ?? 0;
                return (
                  <tr key={e.id} style={{ background: selected.has(e.id) ? 'var(--accent-soft)' : undefined }}>
                    <td className="cell-hide-mobile">
                      <input type="checkbox" checked={selected.has(e.id)} onChange={() => toggle(e.id)}
                        style={{ accentColor: 'var(--accent)', cursor: 'pointer' }} />
                    </td>
                    <td className="cell-primary">
                      <div style={{ fontWeight: 600, fontSize: 14, color: 'var(--accent)', cursor: 'pointer' }}
                        onClick={() => router.push(`/event/${e.id}`)}>
                        {e.title}
                      </div>
                      <div style={{ fontSize: 11, color: 'var(--text-3)', marginTop: 1 }}>
                        Created {fmtDateTime(e.created_at)}
                      </div>
                    </td>
                    <td data-label="Status">
                      <StatusPill status={e.status} />
                    </td>
                    <td data-label="Date" style={{ color: 'var(--text-2)' }}>
                      <div>{fmtDateTime(e.start_time)}</div>
                      <div style={{ fontSize: 11, color: 'var(--text-3)' }}>
                        ends {fmtDateTime(e.end_time)}
                      </div>
                    </td>
                    <td data-label="Location" style={{ color: 'var(--text-2)' }}>
                      {e.venue_name || e.address || e.city ? (
                        <>
                          {/* Venue name, else the street address, else the city
                              on its own — an event with an address but no venue
                              name is located, just unnamed. */}
                          <div>{e.venue_name || e.address || e.city}</div>
                          {(e.venue_name || e.address) && e.city && (
                            <div style={{ fontSize: 11, color: 'var(--text-3)' }}>{e.city}</div>
                          )}
                        </>
                      ) : (
                        <Link href={`/event/${e.id}`} style={{
                          fontSize: 12, color: 'var(--warning)', fontWeight: 600,
                        }}>
                          Not set — add
                        </Link>
                      )}
                    </td>
                    <td data-label="Tickets sold">
                      <div style={{ fontWeight: 600 }}>
                        {ticketsSold.toLocaleString()}
                        {e.max_capacity && (
                          <span style={{ fontSize: 11, color: 'var(--text-3)', fontWeight: 400 }}>
                            {' '}/ {e.max_capacity.toLocaleString()}
                          </span>
                        )}
                      </div>
                      {pct !== null && (
                        <div style={{ marginTop: 4, width: 80 }}>
                          <div className="progress-bar">
                            <div className="progress-fill" style={{
                              width: `${pct}%`,
                              background: pct >= 90 ? 'var(--danger)' : pct >= 70 ? 'var(--warning)' : 'var(--accent)',
                            }} />
                          </div>
                        </div>
                      )}
                    </td>
                    <td data-label="Views" style={{ color: 'var(--text-2)' }}>{views.toLocaleString()}</td>
                    <td data-label="Price" style={{ fontWeight: 600 }}>
                      {e.price === 0 ? (
                        <span style={{ color: 'var(--success-fg)', fontSize: 13 }}>Free</span>
                      ) : fmtMoney(e.price, e.currency)}
                    </td>
                    <td className="cell-actions">
                      <div style={{ display: 'flex', gap: 4, justifyContent: 'flex-end' }}>
                        {/* A draft opens the create form in edit mode; anything
                            live goes to the read-only details page. */}
                        <Link
                          href={e.status === 'draft' ? `/events/create?edit=${e.id}` : `/event/${e.id}`}
                          className="btn btn-xs">
                          {e.status === 'draft' ? 'Edit' : 'Details'}
                        </Link>

                        {e.status === 'draft' && (
                          <button className="btn btn-xs btn-primary" onClick={() => publishEvent(e.id)}>
                            Submit
                          </button>
                        )}

                        {/* Drafts are deleted, not cancelled. Nothing has been
                            sold and the event was never discoverable, so there
                            is no audience to notify — "Cancel" implied a
                            standing commitment that a draft never made. */}
                        {e.status === 'draft' ? (
                          <button className="btn btn-xs btn-danger" onClick={() => deleteDraft(e.id, e.title)}>
                            Delete
                          </button>
                        ) : !['cancelled', 'completed'].includes(e.status) && (
                          <button className="btn btn-xs btn-danger" onClick={() => cancelEvent(e.id)}>
                            Cancel
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {cancelId && (
        <div className="modal-overlay" onMouseDown={(e) => {
          if (isBackdropPress(e)) setCancelId(null);
        }}>
          <div className="modal modal-sm" role="dialog" aria-modal="true">
            <div className="modal-header">
              <div className="modal-title">Cancel this event</div>
            </div>
            <div className="modal-body">
              <div className="alert alert-warning" style={{ fontSize: 13 }}>
                <span>
                  Every confirmed ticket is refunded in full and attendees are
                  notified. This cannot be undone.
                </span>
              </div>
              <div className="field" style={{ marginBottom: 0 }}>
                <label>Why are you cancelling?</label>
                <textarea
                  rows={3}
                  value={cancelReason}
                  onChange={(e) => setCancelReason(e.target.value)}
                  placeholder="Attendees see this in their refund email."
                  style={{
                    width: '100%', padding: '10px 12px', resize: 'vertical',
                    border: '1px solid var(--border-2)', borderRadius: 'var(--r)',
                    background: 'var(--surface-alpha)', color: 'var(--text)',
                    fontSize: 14, fontFamily: 'inherit', outline: 'none',
                    boxSizing: 'border-box',
                  }}
                />
                <div className="field-hint">
                  {cancelReason.trim().length < 10
                    ? `${10 - cancelReason.trim().length} more characters needed`
                    : 'Attendees will see this'}
                </div>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setCancelId(null)}>Keep event</button>
              <button className="btn btn-danger"
                disabled={cancelBusy || cancelReason.trim().length < 10}
                onClick={submitCancel}>
                {cancelBusy ? 'Cancelling…' : 'Cancel event'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}