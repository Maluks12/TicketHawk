'use client';
import { useEffect, useState, useCallback } from 'react';
import { api } from '../../../lib/api';
import { fmtMoney, fmtDate } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';

// ── Types ────────────────────────────────────────────────────────────────────
interface PendingEvent {
  id: string;
  title: string;
  slug: string;
  status: string;
  start_time: string;
  end_time: string;
  price: number;
  currency: string;
  venue_name: string;
  city: string;
  organizer_id: string;
  created_at: string;
}

interface EventDetail {
  id: string;
  title: string;
  description: string;
  slug: string;
  status: string;
  category_id: number | null;
  timezone: string;
  start_time: string;
  end_time: string;
  expires_at: string;
  price: number;
  currency: string;
  max_capacity: number | null;
  allow_waitlist: boolean;
  max_tickets_per_user: number | null;
  venue_type: string | null;
  tickets_sold: number;
  venue_name: string;
  address: string;
  city: string;
  cover_image_url: string | null;
  banner_image_url: string | null;
  gallery_images: string[] | null;
  tags: string[] | null;
  performers: string[] | null;
  teams: string[] | null;
  refund_policy: string | null;
  terms_and_conditions: string | null;
  organizer_id: string | null;
  created_at: string;
  admin_promoted?: boolean;
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function timeAgo(iso: string) {
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return 'just now';
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  if (d < 7) return `${d}d ago`;
  return fmtDate(iso);
}

function Pill({ label, color = '#1877f2' }: { label: string; color?: string }) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center',
      background: color + '18', border: `1px solid ${color}40`,
      color, borderRadius: 99, padding: '2px 9px',
      fontSize: 12, fontWeight: 500, lineHeight: 1.5,
    }}>{label}</span>
  );
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', gap: 12, padding: '10px 0', borderBottom: '1px solid #f0f2f5' }}>
      <div style={{ width: 140, flexShrink: 0, fontSize: 12, fontWeight: 600, color: '#8a8d91', textTransform: 'uppercase', letterSpacing: '.04em', paddingTop: 2 }}>{label}</div>
      <div style={{ flex: 1, fontSize: 14, color: '#1c1e21', lineHeight: 1.6 }}>{children}</div>
    </div>
  );
}

// ── Empty state ───────────────────────────────────────────────────────────────
function EmptyQueue() {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', flex: 1, gap: 12, padding: 40, textAlign: 'center' }}>
      <div style={{ fontSize: 48, opacity: .25 }}>✓</div>
      <div style={{ fontSize: 18, fontWeight: 700, color: '#1c1e21' }}>All clear!</div>
      <div style={{ fontSize: 14, color: '#606770', maxWidth: 280 }}>No organiser events are waiting for review. Check back later.</div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────
export default function ApprovalsPage() {
  const toast = useToast();
  const [queue, setQueue] = useState<PendingEvent[]>([]);
  const [queueLoading, setQueueLoading] = useState(true);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detail, setDetail] = useState<EventDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [rejectOpen, setRejectOpen] = useState(false);
  const [rejectReason, setRejectReason] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [activeImageTab, setActiveImageTab] = useState<'cover' | 'banner'>('cover');

  const loadQueue = useCallback(async () => {
    setQueueLoading(true);
    try {
      const { data } = await api.get<{ events: PendingEvent[] }>('/admin/events/pending');
      const list = data.events || [];
      setQueue(list);
      // Auto-select first if nothing selected
      if (list.length > 0 && !selectedId) {
        setSelectedId(list[0].id);
      }
    } catch {
      toast('Failed to load review queue', 'error');
    } finally {
      setQueueLoading(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [toast]);

  useEffect(() => { loadQueue(); }, [loadQueue]);

  // Load full detail when selection changes
  useEffect(() => {
    if (!selectedId) { setDetail(null); return; }
    setDetailLoading(true);
    setDetail(null);
    api.get<EventDetail>(`/admin/events/${selectedId}`)
      .then(res => setDetail(res.data))
      .catch(() => toast('Failed to load event details', 'error'))
      .finally(() => setDetailLoading(false));
  }, [selectedId, toast]);

  // ── Actions ────────────────────────────────────────────────────────────────
  async function approve(id: string) {
    setSubmitting(true);
    try {
      await api.put(`/admin/events/${id}/approve`);
      toast('Event approved and published!', 'success');
      const next = queue.find(e => e.id !== id);
      await loadQueue();
      setSelectedId(next?.id ?? null);
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'Failed to approve event';
      toast(msg, 'error');
    } finally { setSubmitting(false); }
  }

  // Approve + place on the home page as a free (admin-granted) sponsored card.
  async function approveAndPromote(id: string) {
    setSubmitting(true);
    try {
      await api.put(`/admin/events/${id}/promote`);
      toast('Event published and added to the home page!', 'success');
      const next = queue.find(e => e.id !== id);
      await loadQueue();
      setSelectedId(next?.id ?? null);
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'Failed to add event to the home page';
      toast(msg, 'error');
    } finally { setSubmitting(false); }
  }

  async function submitReject() {
    if (rejectReason.trim().length < 10) { toast('Reason must be at least 10 characters', 'error'); return; }
    setSubmitting(true);
    try {
      await api.put(`/admin/events/${selectedId}/reject`, { reason: rejectReason.trim() });
      toast('Event denied. Organiser has been notified.', 'success');
      const next = queue.find(e => e.id !== selectedId);
      setRejectOpen(false);
      await loadQueue();
      setSelectedId(next?.id ?? null);
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'Failed to reject event';
      toast(msg, 'error');
    } finally { setSubmitting(false); }
  }

  // ── Render ─────────────────────────────────────────────────────────────────
  return (
    <div className="page">
      {/* Header */}
      <div className="page-header">
        <div>
          <div className="page-title">Event Approvals</div>
          <div className="page-sub">Review and approve events submitted by organisers</div>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          {!queueLoading && (
            <span style={{ fontSize: 13, color: '#606770' }}>
              {queue.length === 0 ? 'No events pending' : `${queue.length} event${queue.length !== 1 ? 's' : ''} pending review`}
            </span>
          )}
          <button className="btn btn-sm" onClick={loadQueue} disabled={queueLoading}>↺ Refresh</button>
        </div>
      </div>

      {/* Body */}
      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>

        {/* ── Left: Queue list ── */}
        <div style={{
          width: 320, flexShrink: 0, borderRight: '1px solid #dddfe2',
          display: 'flex', flexDirection: 'column', background: '#f7f8fa', overflowY: 'auto',
        }}>
          {queueLoading ? (
            <div style={{ padding: 24, display: 'flex', alignItems: 'center', gap: 10, color: '#606770', fontSize: 14 }}>
              <div className="spinner" style={{ width: 16, height: 16 }} /> Loading…
            </div>
          ) : queue.length === 0 ? (
            <div style={{ padding: 24, color: '#8a8d91', fontSize: 14, textAlign: 'center', marginTop: 24 }}>
              No events pending review
            </div>
          ) : (
            queue.map(ev => {
              const selected = ev.id === selectedId;
              return (
                <button
                  key={ev.id}
                  onClick={() => setSelectedId(ev.id)}
                  style={{
                    display: 'block', width: '100%', textAlign: 'left', border: 'none',
                    borderBottom: '1px solid #dddfe2', padding: '12px 14px',
                    background: selected ? '#fff' : 'transparent',
                    cursor: 'pointer', borderLeft: selected ? '3px solid #1877f2' : '3px solid transparent',
                    transition: 'background .12s',
                  }}
                >
                  <div style={{ fontWeight: 600, fontSize: 13, color: '#1c1e21', marginBottom: 4, lineHeight: 1.3, overflow: 'hidden', textOverflow: 'ellipsis', display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' }}>
                    {ev.title}
                  </div>
                  <div style={{ fontSize: 12, color: '#606770', marginBottom: 3, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {[ev.venue_name, ev.city].filter(Boolean).join(', ') || 'No location'}
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 11, color: '#8a8d91' }}>
                    <span>{ev.start_time ? fmtDate(ev.start_time) : '—'}</span>
                    <span>·</span>
                    <span>{fmtMoney(ev.price, ev.currency)}</span>
                    <span style={{ marginLeft: 'auto' }}>{timeAgo(ev.created_at)}</span>
                  </div>
                </button>
              );
            })
          )}
        </div>

        {/* ── Right: Detail panel ── */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          {!selectedId ? (
            <EmptyQueue />
          ) : detailLoading ? (
            <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 12, color: '#606770' }}>
              <div className="spinner" /> Loading event details…
            </div>
          ) : !detail ? (
            <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#8a8d91' }}>
              Could not load event details
            </div>
          ) : (
            <>
              {/* Detail scrollable area */}
              <div style={{ flex: 1, overflowY: 'auto', padding: '24px 28px' }}>

                {/* Title + status */}
                <div style={{ marginBottom: 20 }}>
                  <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10, flexWrap: 'wrap', marginBottom: 6 }}>
                    <h2 style={{ fontSize: 22, fontWeight: 700, color: '#1c1e21', lineHeight: 1.3, flex: 1, minWidth: 0 }}>{detail.title}</h2>
                    <span style={{ background: '#fff3e0', color: '#e65100', border: '1px solid #fcd34d', borderRadius: 99, padding: '3px 10px', fontSize: 12, fontWeight: 700, flexShrink: 0 }}>Pending Review</span>
                  </div>
                  <div style={{ fontSize: 13, color: '#8a8d91' }}>Submitted {timeAgo(detail.created_at)}</div>
                </div>

                {/* Images */}
                {(detail.cover_image_url || detail.banner_image_url) && (
                  <div style={{ marginBottom: 24 }}>
                    <div style={{ display: 'flex', gap: 6, marginBottom: 8 }}>
                      {detail.cover_image_url && (
                        <button
                          onClick={() => setActiveImageTab('cover')}
                          style={{ fontSize: 12, fontWeight: 600, padding: '4px 10px', borderRadius: 6, border: '1px solid', cursor: 'pointer', background: activeImageTab === 'cover' ? '#1877f2' : '#fff', borderColor: activeImageTab === 'cover' ? '#1877f2' : '#dddfe2', color: activeImageTab === 'cover' ? '#fff' : '#606770' }}
                        >Cover</button>
                      )}
                      {detail.banner_image_url && (
                        <button
                          onClick={() => setActiveImageTab('banner')}
                          style={{ fontSize: 12, fontWeight: 600, padding: '4px 10px', borderRadius: 6, border: '1px solid', cursor: 'pointer', background: activeImageTab === 'banner' ? '#1877f2' : '#fff', borderColor: activeImageTab === 'banner' ? '#1877f2' : '#dddfe2', color: activeImageTab === 'banner' ? '#fff' : '#606770' }}
                        >Banner</button>
                      )}
                    </div>
                    {activeImageTab === 'cover' && detail.cover_image_url && (
                      <img
                        src={detail.cover_image_url}
                        alt="Cover"
                        style={{ width: '100%', maxHeight: 260, objectFit: 'cover', borderRadius: 8, border: '1px solid #dddfe2', display: 'block' }}
                        onError={e => { (e.target as HTMLImageElement).style.display = 'none'; }}
                      />
                    )}
                    {activeImageTab === 'banner' && detail.banner_image_url && (
                      <img
                        src={detail.banner_image_url}
                        alt="Banner"
                        style={{ width: '100%', maxHeight: 260, objectFit: 'cover', borderRadius: 8, border: '1px solid #dddfe2', display: 'block' }}
                        onError={e => { (e.target as HTMLImageElement).style.display = 'none'; }}
                      />
                    )}
                  </div>
                )}

                {/* Gallery */}
                {detail.gallery_images && detail.gallery_images.length > 0 && (
                  <div style={{ marginBottom: 20 }}>
                    <div style={{ fontSize: 12, fontWeight: 600, color: '#8a8d91', textTransform: 'uppercase', letterSpacing: '.04em', marginBottom: 8 }}>Gallery</div>
                    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                      {detail.gallery_images.map((url, i) => (
                        <img key={i} src={url} alt={`Gallery ${i + 1}`}
                          style={{ width: 80, height: 80, objectFit: 'cover', borderRadius: 6, border: '1px solid #dddfe2' }}
                          onError={e => { (e.target as HTMLImageElement).style.display = 'none'; }}
                        />
                      ))}
                    </div>
                  </div>
                )}

                {/* Core details */}
                <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, padding: '0 16px', marginBottom: 20 }}>
                  <DetailRow label="Description">
                    <div style={{ whiteSpace: 'pre-wrap', color: '#3c4043' }}>{detail.description || '—'}</div>
                  </DetailRow>
                  <DetailRow label="Venue">
                    <div>{detail.venue_name || '—'}</div>
                    {detail.address && <div style={{ color: '#606770', fontSize: 13 }}>{detail.address}</div>}
                    {detail.city && <div style={{ color: '#606770', fontSize: 13 }}>{detail.city}</div>}
                  </DetailRow>
                  <DetailRow label="Date & Time">
                    <div>
                      <span style={{ fontWeight: 500 }}>Start:</span>{' '}
                      {detail.start_time ? new Date(detail.start_time).toLocaleString('en-ZA', { dateStyle: 'full', timeStyle: 'short' }) : '—'}
                    </div>
                    {detail.end_time && (
                      <div style={{ marginTop: 2 }}>
                        <span style={{ fontWeight: 500 }}>End:</span>{' '}
                        {new Date(detail.end_time).toLocaleString('en-ZA', { dateStyle: 'full', timeStyle: 'short' })}
                      </div>
                    )}
                    {detail.timezone && <div style={{ color: '#8a8d91', fontSize: 12, marginTop: 2 }}>Timezone: {detail.timezone}</div>}
                  </DetailRow>
                  <DetailRow label="Pricing">
                    <span style={{ fontWeight: 600, fontSize: 15 }}>{fmtMoney(detail.price, detail.currency)}</span>
                    {detail.max_capacity && (
                      <span style={{ marginLeft: 12, fontSize: 13, color: '#606770' }}>· Capacity: {detail.max_capacity.toLocaleString()}</span>
                    )}
                    {detail.allow_waitlist && (
                      <span style={{ marginLeft: 8, fontSize: 12, color: '#42b72a', fontWeight: 600 }}>· Waitlist enabled</span>
                    )}
                  </DetailRow>
                  {(detail.tags && detail.tags.length > 0) && (
                    <DetailRow label="Tags">
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                        {detail.tags.map((t, i) => <Pill key={i} label={t} color="#606770" />)}
                      </div>
                    </DetailRow>
                  )}
                  {(detail.performers && detail.performers.length > 0) && (
                    <DetailRow label="Performers">
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                        {detail.performers.map((p, i) => <Pill key={i} label={p} color="#1877f2" />)}
                      </div>
                    </DetailRow>
                  )}
                  {(detail.teams && detail.teams.length > 0) && (
                    <DetailRow label="Teams">
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                        {detail.teams.map((t, i) => <Pill key={i} label={t} color="#9333ea" />)}
                      </div>
                    </DetailRow>
                  )}
                  {detail.refund_policy && (
                    <DetailRow label="Refund Policy">
                      <div style={{ color: '#3c4043', whiteSpace: 'pre-wrap' }}>{detail.refund_policy}</div>
                    </DetailRow>
                  )}
                  {detail.terms_and_conditions && (
                    <DetailRow label="T&Cs">
                      <div style={{ color: '#3c4043', whiteSpace: 'pre-wrap' }}>{detail.terms_and_conditions}</div>
                    </DetailRow>
                  )}
                  <DetailRow label="Organiser ID">
                    <span style={{ fontFamily: 'monospace', fontSize: 12, color: '#606770' }}>{detail.organizer_id || 'N/A'}</span>
                  </DetailRow>
                </div>
              </div>

              {/* ── Sticky action bar ── */}
              <div style={{
                flexShrink: 0, padding: '14px 28px', borderTop: '1px solid #dddfe2',
                background: '#fff', display: 'flex', alignItems: 'center', gap: 10,
              }}>
                <div style={{ flex: 1, fontSize: 13, color: '#8a8d91' }}>
                  Review this event carefully before approving.
                </div>
                <button
                  className="btn"
                  onClick={() => { setRejectOpen(true); setRejectReason(''); }}
                  disabled={submitting}
                  style={{ borderColor: '#fa3e3e', color: '#fa3e3e', fontWeight: 600 }}
                >✕ Deny</button>
                <button
                  className="btn"
                  onClick={() => approveAndPromote(detail.id)}
                  disabled={submitting}
                  title="Approve and feature this event on the home page as a sponsored card — no ad spend required."
                  style={{ background: '#1877f2', borderColor: '#1877f2', color: '#fff', fontWeight: 600 }}
                >
                  {submitting ? <><div className="spinner" style={{ width: 14, height: 14 }} /> Processing…</> : '★ Approve & Add to Home'}
                </button>
                <button
                  className="btn"
                  onClick={() => approve(detail.id)}
                  disabled={submitting}
                  style={{ background: '#42b72a', borderColor: '#42b72a', color: '#fff', fontWeight: 600 }}
                >
                  {submitting ? <><div className="spinner" style={{ width: 14, height: 14 }} /> Processing…</> : '✓ Approve & Publish'}
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      {/* ── Deny Modal ── */}
      {rejectOpen && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setRejectOpen(false); }}>
          <div className="modal modal-sm">
            <div className="modal-header">
              <div className="modal-title">Deny Event</div>
              <button className="modal-close" onClick={() => setRejectOpen(false)}>✕</button>
            </div>
            <div className="modal-body">
              <div className="alert alert-warning">
                The organiser will receive your reason and can revise and resubmit the event.
              </div>
              {detail && (
                <div style={{ padding: '8px 12px', background: '#f7f8fa', borderRadius: 6, marginBottom: 14, fontSize: 13, color: '#606770', fontWeight: 500 }}>
                  "{detail.title}"
                </div>
              )}
              <div className="field">
                <label>Reason for Denial <small style={{ color: 'var(--text-3)', fontWeight: 300 }}>(min 10 characters)</small></label>
                <textarea
                  rows={4}
                  value={rejectReason}
                  onChange={e => setRejectReason(e.target.value)}
                  placeholder="e.g. Missing venue details, description too vague, start date conflicts with local regulations…"
                  autoFocus
                />
                <div style={{ fontSize: 12, color: rejectReason.trim().length >= 10 ? '#42b72a' : '#8a8d91', marginTop: 4 }}>
                  {rejectReason.trim().length} characters {rejectReason.trim().length < 10 ? `(${10 - rejectReason.trim().length} more needed)` : '✓'}
                </div>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setRejectOpen(false)} disabled={submitting}>Back</button>
              <button
                className="btn"
                onClick={submitReject}
                disabled={rejectReason.trim().length < 10 || submitting}
                style={{ background: '#fa3e3e', borderColor: '#fa3e3e', color: '#fff', fontWeight: 600 }}
              >
                {submitting ? 'Denying…' : 'Deny Event'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}