'use client';
import { useState, useEffect, useCallback } from 'react';
import { useParams, useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { api } from '../../../../lib/api';
import { useToast } from '../../../../lib/toast';
import TicketTiersPanel from '../../../../components/TicketTiersPanel';
import EventInfoEditor from '../../../../components/eventInfoEditor';
import LineupBuilder from '../../../../components/lineupBuilder';
import { fmtMoney, fmtDate, fmtDateTime } from '../../../../lib/utils';

// 'preview' is gone. The framed preview loaded the public event page in an
// iframe, and it never worked reliably — see the note in the manifest. An
// organiser can open their own event on the site directly, which is the same
// page without a component in between that can lie about it.
type Tab = 'overview' | 'info' | 'lineup' | 'bookings' | 'merch' | 'scan';

// Launch gate: merch is built but NOT launching — the tab and its section
// are hidden, not deleted. Keep in sync with MERCH_ENABLED in layout.tsx.
const MERCH_ENABLED = false;

// Mirrors what /organiser/dashboard/events/:id/stats returns for a tier.
//
// reserved_count, available_tickets and is_sold_out were all absent here even
// though the API has always sent them, which is why the read-only table could
// not show held tickets or a sold-out state. TicketTiersPanel needs them, and
// `description` is widened from optional to nullable-required so the shapes
// line up exactly rather than merely overlapping.
interface TicketTypeRow {
  id: string; name: string; description: string | null;
  price: number; capacity: number | null; tickets_sold: number;
  reserved_count: number;
  available_tickets?: number;
  is_sold_out?: boolean;
  sort_order: number; is_visible: boolean;
  sale_starts_at?: string | null; sale_ends_at?: string | null;
}

interface EventStats {
  event: {
    id: string; slug?: string; visibility?: string; title: string; status: string;
    description?: string;
    start_time: string; end_time: string; expires_at?: string; timezone?: string;
    city: string; venue_name: string; address?: string; venue_type?: string | null;
    latitude?: number | null; longitude?: number | null;
    category?: string; category_id?: number;
    price: number; currency: string;
    tickets_sold: number; max_capacity: number | null;
    max_tickets_per_user?: number | null; allow_waitlist?: boolean;
    views: number;
    cover_image_url: string | null;
    banner_image_url: string | null;
    banner_video_url?: string | null;
    gallery_images?: string[];
    scan_open_offset_min?: number | null;
    scan_close_offset_min?: number | null;
    refund_policy?: string | null;
    terms_and_conditions?: string | null;
    tags?: string[]; performers?: string[]; teams?: string[];
    ticket_types?: TicketTypeRow[];
    listing_fee_paid?: boolean; listing_fee_amount?: number;
    is_featured?: boolean; admin_promoted?: boolean;
    rights_declared_at?: string | null;
    published_at?: string | null;
    rejection_reason?: string | null;
    cancelled_at?: string | null;
    cancellation_reason?: string | null;
    created_at?: string; updated_at?: string;
  };
  total_revenue: number;
  total_bookings: number;
  confirmed_bookings: number;
  pending_bookings: number;
  cancelled_bookings?: number;
  refunded_bookings?: number;
  gross_ticket_revenue?: number;
  platform_fees_paid?: number;
  net_ticket_earnings?: number;
  total_refunded?: number;
}

// Mirrors repository.OrganizerBookingRow in the backend. The previous shape
// (`user` object + `total_amount`) never existed on the wire, so every row
// fell through to "Guest" with a blank amount.
//
// Attendee email is deliberately absent: the backend tags it `json:"-"`
// because it is attendee PII that organisers are not given access to.
interface Booking {
  id: string; booking_number: string; booking_status: string;
  created_at: string;
  ticket_quantity: number;
  unit_price: number;
  subtotal: number;
  platform_fee: number;
  net_earning: number;
  currency: string;
  is_guest: boolean;
  attendee_name: string;
}

interface MerchItem {
  id: string; name: string; status: string; price: number; currency: string;
  stock_quantity: number | null;
}

export default function EventDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const toast = useToast();

  // ── Opening tab comes from the URL ─────────────────────────────────────
  //
  // The create wizard finishes at /event/<id>?tab=info&created=1 so a brand new
  // event lands on its Information block rather than on Overview, where the
  // Information and Line-up tabs are easy to miss entirely.
  //
  // Validated against the known set rather than cast: ?tab=nonsense should show
  // Overview, not an empty pane.
  const search = useSearchParams();
  const initialTab: Tab = (() => {
    const t = search.get('tab');
    return (['overview', 'info', 'lineup', 'bookings'] as const)
      .includes(t as never) ? (t as Tab) : 'overview';
  })();
  const justCreated = search.get('created') === '1';

  const [tab, setTab] = useState<Tab>(initialTab);
  const [stats, setStats] = useState<EventStats | null>(null);
  const [bookings, setBookings] = useState<Booking[]>([]);
  const [merch, setMerch] = useState<MerchItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [coverUploading, setCoverUploading] = useState(false);
  const [bannerUploading, setBannerUploading] = useState(false);
  // "Copied!" flash on the share-link button. This MUST be declared here with
  // every other hook, above the `if (loading)` / `if (!stats)` early returns.
  // It used to sit below them: on the first render `loading` was true, the
  // component bailed early and this useState never ran; once stats arrived the
  // render continued past the returns and the hook appeared, so React saw more
  // hooks than the previous render, threw "Rendered more hooks than during the
  // previous render", and — with no error boundary on the route — took the
  // whole render tree down. That is the browser's "This page couldn't load".
  const [copied, setCopied] = useState(false);

  const fetchStats = useCallback(async () => {
    try {
      const res = await api.get(`/organiser/dashboard/events/${id}/stats`);
      setStats(res.data);
    } catch { toast('Failed to load event stats', 'error'); }
    finally { setLoading(false); }
  }, [id]);

  const fetchBookings = useCallback(async () => {
    try {
      const res = await api.get(`/organiser/dashboard/events/${id}/bookings`);
      setBookings(res.data.bookings ?? []);
    } catch { toast('Failed to load bookings', 'error'); }
  }, [id]);

  const fetchMerch = useCallback(async () => {
    try {
      const res = await api.get(`/organiser/dashboard/events/${id}/merch`);
      setMerch(res.data.merch ?? []);
    } catch { toast('Failed to load merch', 'error'); }
  }, [id]);

  useEffect(() => { fetchStats(); }, [fetchStats]);

  useEffect(() => {
    if (tab === 'bookings') fetchBookings();
    if (MERCH_ENABLED && tab === 'merch') fetchMerch();
  }, [tab, fetchBookings, fetchMerch]);

  async function handleCoverUpload(file: File) {
    setCoverUploading(true);
    try {
      const fd = new FormData();
      fd.append('image', file);
      const res = await api.post(`/organiser/events/${id}/media/cover`, fd);
      setStats(prev => prev ? {
        ...prev,
        event: { ...prev.event, cover_image_url: res.data.cover_image_url },
      } : prev);
      toast('Cover image updated', 'success');
    } catch { toast('Failed to upload cover image', 'error'); }
    finally { setCoverUploading(false); }
  }

  async function handleBannerUpload(file: File) {
    setBannerUploading(true);
    try {
      const fd = new FormData();
      fd.append('image', file);
      const res = await api.post(`/organiser/events/${id}/media/banner`, fd);
      setStats(prev => prev ? {
        ...prev,
        event: { ...prev.event, banner_image_url: res.data.banner_image_url },
      } : prev);
      toast('Banner image updated', 'success');
    } catch { toast('Failed to upload banner image', 'error'); }
    finally { setBannerUploading(false); }
  }

  if (loading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
        <div className="spinner spinner-lg" />
      </div>
    );
  }

  if (!stats) {
    return (
      <div className="empty-state">
        <div className="empty-title">Event not found</div>
        <Link href="/events" className="btn">← Back to events</Link>
      </div>
    );
  }

  const ev = stats.event;

  // ── Shareable event link ──────────────────────────────────────────────────
  // Only meaningful once the event is live (published / published_private).
  // Uses the public web base (NEXT_PUBLIC_SITE_URL) and the event slug, falling
  // back to the id and to the production domain if the env var isn't set.
  // NOTE: `copied` state lives with the other hooks at the top of the
  // component — it must NOT be declared here, below the early returns.
  const isLive = ev.status === 'published' || ev.status === 'published_private';
  const siteBase = process.env.NEXT_PUBLIC_SITE_URL || 'https://ticket-hawk.com';
  const shareUrl = `${siteBase}/events/${ev.slug || ev.id}`;
  const copyShare = async () => {
    try {
      await navigator.clipboard.writeText(shareUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast('Could not copy — long-press the link to copy it manually', 'error');
    }
  };
  const capacityPct = ev.max_capacity
    ? Math.min((ev.tickets_sold / ev.max_capacity) * 100, 100)
    : null;

  return (
    <div className="page">

      {/* ── Page header with breadcrumb ───────────────────────────────── */}
      <div className="page-header">
        <div>
          <div style={{ fontSize: 12, color: 'var(--text-2)', marginBottom: 4 }}>
            <Link href="/events" style={{ color: 'var(--text-2)' }}>My Events</Link>
            {' → '}
            <span>{ev.title}</span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <div className="page-title">{ev.title}</div>
            <span className={`badge badge-${ev.status}`}>
              {ev.status.charAt(0).toUpperCase() + ev.status.slice(1)}
            </span>
          </div>
          <div style={{ fontSize: 13, color: 'var(--text-2)', marginTop: 2 }}>
            {fmtDate(ev.start_time)} · {ev.venue_name}, {ev.city}
          </div>

          {isLive && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 10, flexWrap: 'wrap' }}>
              <div style={{
                display: 'flex', alignItems: 'center', gap: 6, maxWidth: 360,
                background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 6,
                padding: '6px 10px', fontSize: 12.5, color: 'var(--text)',
                overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis',
              }}>
                <svg viewBox="0 0 20 20" fill="var(--text-2)" width="14" height="14" style={{ flexShrink: 0 }}>
                  <path d="M11 3a1 1 0 100 2h2.586l-6.293 6.293a1 1 0 101.414 1.414L15 6.414V9a1 1 0 102 0V4a1 1 0 00-1-1h-5z"/>
                  <path d="M5 5a2 2 0 00-2 2v8a2 2 0 002 2h8a2 2 0 002-2v-3a1 1 0 10-2 0v3H5V7h3a1 1 0 000-2H5z"/>
                </svg>
                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{shareUrl}</span>
              </div>
              <button className="btn btn-sm" onClick={copyShare}>
                {copied ? '✓ Copied' : 'Copy link'}
              </button>
              <a className="btn btn-sm" href={shareUrl} target="_blank" rel="noopener noreferrer">Open</a>
            </div>
          )}
        </div>
        {/* Boost/campaign CTA removed — ads disabled for launch */}
      </div>

      {/* ── Tab bar ──────────────────────────────────────────────────── */}
      <div className="tab-bar">
        {([
          { id: 'overview' as Tab, label: 'Overview' },
          // Content tabs sit next to Overview, before Bookings: they are what
          // an organiser edits while preparing an event, and Bookings is what
          // they watch after it is live.
          { id: 'info' as Tab, label: 'Information' },
          { id: 'lineup' as Tab, label: 'Line-up' },
          { id: 'bookings' as Tab, label: `Bookings (${stats.total_bookings})` },
          ...(MERCH_ENABLED ? [{ id: 'merch' as Tab, label: 'Merch' }] : []),
        ]).map(t => (
          <button key={t.id} className={`tab-item${tab === t.id ? ' active' : ''}`}
            onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}

        {/* Scan Tickets removed. Door scanning happens in the EventsStudio
            app, not here: the app holds the offline manifest, the scanner
            credential and the local scan ledger, so a browser tab could only
            ever scan while online — the one condition a door cannot rely on.
            Offering it here implied a parity that does not exist.

            The /event/[id]/scan route still exists and still works if opened
            directly; only the entry point is gone. Delete the route too when
            you are ready to drop the web scanner entirely. */}
      </div>

      {/* ── Tab content ──────────────────────────────────────────────── */}
      <div className="page-content">

        {tab === 'info' && (
          <div className="card"><div className="card-body">
            {justCreated && (
              <div className="alert alert-success" style={{ marginBottom: 18 }}>
                <div>
                  <strong>Event created.</strong> It is a draft pending review.
                  While you wait, fill in the information attendees ask about
                  and add your line-up — both are tabs above, and both can be
                  changed later.
                </div>
              </div>
            )}
            <EventInfoEditor eventId={id} />
          </div></div>
        )}

        {tab === 'lineup' && (
          <div className="card"><div className="card-body">
            <LineupBuilder
              eventId={id}
              eventStart={ev.start_time}
              isSport={ev.category?.toLowerCase() === 'sports'}
            />
          </div></div>
        )}

        {tab === 'overview' && (
          <div>
            {/* ── Event images ──────────────────────────────────────────── */}
            <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, overflow: 'hidden', marginBottom: 16 }}>
              <div style={{ padding: '14px 16px', borderBottom: '1px solid var(--border)', fontWeight: 700, fontSize: 15 }}>
                Event images
              </div>
              <div style={{ padding: 16, display: 'flex', gap: 24, flexWrap: 'wrap' }}>

                {/* Cover image */}
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-2)', marginBottom: 8, textTransform: 'uppercase', letterSpacing: '.04em' }}>Cover image</div>
                  <div style={{ position: 'relative', width: 160, height: 100 }}>
                    {ev.cover_image_url ? (
                      <>
                        <img
                          src={ev.cover_image_url}
                          alt="Cover"
                          style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: 6, border: '1px solid var(--border)', display: 'block' }}
                          onError={e => { (e.target as HTMLImageElement).style.opacity = '0.3'; }}
                        />
                        <label
                          title="Replace cover image"
                          style={{
                            position: 'absolute', bottom: 4, right: 4,
                            background: 'rgba(0,0,0,.55)', color: '#fff',
                            borderRadius: 4, padding: '3px 7px', fontSize: 11,
                            cursor: coverUploading ? 'wait' : 'pointer',
                            fontWeight: 600,
                          }}>
                          <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={coverUploading}
                            onChange={e => { const f = e.target.files?.[0]; if (f) { handleCoverUpload(f); e.target.value = ''; } }} />
                          {coverUploading ? '…' : '✎ Replace'}
                        </label>
                      </>
                    ) : (
                      <label style={{
                        width: '100%', height: '100%', border: '1.5px dashed var(--border-2)', borderRadius: 6,
                        display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
                        cursor: coverUploading ? 'wait' : 'pointer', color: 'var(--text-3)',
                        fontSize: 11, gap: 4, background: 'var(--surface-2)',
                      }}>
                        <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={coverUploading}
                          onChange={e => { const f = e.target.files?.[0]; if (f) { handleCoverUpload(f); e.target.value = ''; } }} />
                        {coverUploading
                          ? <div className="spinner" style={{ width: 18, height: 18 }} />
                          : <><span style={{ fontSize: '1.4rem', lineHeight: 1 }}>+</span><span>Upload cover</span></>}
                      </label>
                    )}
                  </div>
                  <div style={{ fontSize: 11, color: 'var(--text-3)', marginTop: 6 }}>JPEG · PNG · WebP · max 5 MB</div>
                </div>

                {/* Banner image */}
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-2)', marginBottom: 8, textTransform: 'uppercase', letterSpacing: '.04em' }}>Banner image</div>
                  <div style={{ position: 'relative', width: 240, height: 100 }}>
                    {ev.banner_image_url ? (
                      <>
                        <img
                          src={ev.banner_image_url}
                          alt="Banner"
                          style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: 6, border: '1px solid var(--border)', display: 'block' }}
                          onError={e => { (e.target as HTMLImageElement).style.opacity = '0.3'; }}
                        />
                        <label
                          title="Replace banner image"
                          style={{
                            position: 'absolute', bottom: 4, right: 4,
                            background: 'rgba(0,0,0,.55)', color: '#fff',
                            borderRadius: 4, padding: '3px 7px', fontSize: 11,
                            cursor: bannerUploading ? 'wait' : 'pointer',
                            fontWeight: 600,
                          }}>
                          <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={bannerUploading}
                            onChange={e => { const f = e.target.files?.[0]; if (f) { handleBannerUpload(f); e.target.value = ''; } }} />
                          {bannerUploading ? '…' : '✎ Replace'}
                        </label>
                      </>
                    ) : (
                      <label style={{
                        width: '100%', height: '100%', border: '1.5px dashed var(--border-2)', borderRadius: 6,
                        display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
                        cursor: bannerUploading ? 'wait' : 'pointer', color: 'var(--text-3)',
                        fontSize: 11, gap: 4, background: 'var(--surface-2)',
                      }}>
                        <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={bannerUploading}
                          onChange={e => { const f = e.target.files?.[0]; if (f) { handleBannerUpload(f); e.target.value = ''; } }} />
                        {bannerUploading
                          ? <div className="spinner" style={{ width: 18, height: 18 }} />
                          : <><span style={{ fontSize: '1.4rem', lineHeight: 1 }}>+</span><span>Upload banner</span></>}
                      </label>
                    )}
                  </div>
                  <div style={{ fontSize: 11, color: 'var(--text-3)', marginTop: 6 }}>Wide hero image · JPEG · PNG · WebP · max 8 MB</div>
                </div>

              </div>
            </div>

            {/* Stat row */}
            <div className="stat-grid" style={{ gridTemplateColumns: 'repeat(4, 1fr)' }}>
              {[
                { label: 'Revenue', value: fmtMoney(stats.total_revenue, ev.currency), sub: 'from ticket sales', accent: 'green' as const },
                { label: 'Tickets sold', value: ev.tickets_sold.toLocaleString(), sub: ev.max_capacity ? `of ${ev.max_capacity.toLocaleString()} capacity` : 'no cap', accent: 'blue' as const },
                { label: 'Confirmed', value: stats.confirmed_bookings.toLocaleString(), sub: `${stats.pending_bookings} pending`, accent: 'blue' as const },
                { label: 'Page views', value: ev.views.toLocaleString(), sub: 'total event views', accent: 'orange' as const },
              ].map(item => (
                <div key={item.label} className={`stat-card ${item.accent}`}>
                  <div className="stat-label">{item.label}</div>
                  <div className="stat-value">{item.value}</div>
                  <div className="stat-sub">{item.sub}</div>
                </div>
              ))}
            </div>

            {/* Capacity fill bar */}
            {capacityPct !== null && (
              <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: '16px 20px', marginBottom: 16 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                  <span style={{ fontSize: 14, fontWeight: 600 }}>Capacity</span>
                  <span style={{ fontSize: 14, color: 'var(--text-2)' }}>
                    {ev.tickets_sold} / {ev.max_capacity} ({capacityPct.toFixed(0)}%)
                  </span>
                </div>
                <div className="progress-bar" style={{ height: 8 }}>
                  <div className="progress-fill" style={{
                    width: `${capacityPct}%`,
                    background: capacityPct >= 90 ? 'var(--danger)' : capacityPct >= 70 ? 'var(--warning)' : 'var(--accent)',
                  }} />
                </div>
                {capacityPct >= 90 && (
                  <div style={{ fontSize: 12, color: 'var(--danger)', marginTop: 6, fontWeight: 600 }}>
                    ⚠ Almost sold out
                  </div>
                )}
              </div>
            )}

            {/* ── Status notices ─────────────────────────────────────────── */}
            {ev.rejection_reason && (
              <Panel title="Rejected by review">
                <div style={{ color: 'var(--danger-fg)', fontSize: 13.5, lineHeight: 1.6 }}>{ev.rejection_reason}</div>
              </Panel>
            )}
            {ev.cancelled_at && (
              <Panel title="Cancelled">
                <Row label="Cancelled at" value={fmtDateTime(ev.cancelled_at)} />
                <Row label="Reason" value={ev.cancellation_reason || '—'} />
              </Panel>
            )}

            {/* ── Description ────────────────────────────────────────────── */}
            <Panel title="Description">
              <div style={{ fontSize: 13.5, lineHeight: 1.65, color: 'var(--text)', whiteSpace: 'pre-wrap' }}>
                {ev.description?.trim() || <span style={{ color: 'var(--text-3)' }}>No description</span>}
              </div>
            </Panel>

            {/* ── Schedule ───────────────────────────────────────────────── */}
            <Panel title="Schedule">
              <Row label="Starts"           value={fmtDateTime(ev.start_time)} />
              <Row label="Ends"             value={fmtDateTime(ev.end_time)} />
              <Row label="Bookings close"   value={ev.expires_at ? fmtDateTime(ev.expires_at) : '—'} />
              <Row label="Timezone"         value={ev.timezone || '—'} />
              <Row label="Scanning opens"
                   value={ev.scan_open_offset_min != null
                     ? `${ev.scan_open_offset_min} min before start`
                     : 'Default window'} />
              <Row label="Scanning closes"
                   value={ev.scan_close_offset_min != null
                     ? `${ev.scan_close_offset_min} min after end`
                     : 'Default window'} />
            </Panel>

            {/* ── Location ───────────────────────────────────────────────── */}
            <Panel title="Location">
              <Row label="Venue"      value={ev.venue_name} />
              <Row label="Address"    value={ev.address || '—'} />
              <Row label="City"       value={ev.city} />
              <Row label="Venue type" value={ev.venue_type || 'Auto'} />
              <Row label="Coordinates"
                   value={ev.latitude != null && ev.longitude != null
                     ? `${ev.latitude}, ${ev.longitude}`
                     : '—'} />
            </Panel>

            {/* ── Tickets & pricing ──────────────────────────────────────── */}
            <Panel title="Tickets & pricing">
              <Row label="Base price"        value={ev.price > 0 ? fmtMoney(ev.price, ev.currency) : 'Free'} />
              <Row label="Currency"          value={ev.currency} />
              <Row label="Capacity"          value={ev.max_capacity ? ev.max_capacity.toLocaleString() : 'Unlimited'} />
              <Row label="Max per attendee"  value={ev.max_tickets_per_user ? String(ev.max_tickets_per_user) : 'No limit'} />
              <Row label="Waitlist"          value={ev.allow_waitlist ? 'Enabled' : 'Disabled'} />

              {/* Was a read-only table. The backend has always exposed full tier
                  CRUD; the dashboard just never called it, so a typo’d price or a
                  tier that needed pulling off sale meant a support request. */}
              <TicketTiersPanel
                eventId={id}
                tiers={ev.ticket_types ?? []}
                currency={ev.currency}
                onChanged={fetchStats}
              />
            </Panel>

            {/* ── Classification ─────────────────────────────────────────── */}
            <Panel title="Classification">
              <Row label="Category"   value={ev.category || '—'} />
              <Row label="Visibility" value={ev.visibility === 'private_link' ? 'Private (link only)' : 'Public'} />
              <ChipRow label="Tags"       items={ev.tags} />
              <ChipRow label="Performers" items={ev.performers} />
              <ChipRow label="Teams"      items={ev.teams} />
            </Panel>

            {/* ── Policies ───────────────────────────────────────────────── */}
            <Panel title="Policies">
              <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--text-2)', marginBottom: 4 }}>Refund policy</div>
              <div style={{ fontSize: 13.5, lineHeight: 1.6, whiteSpace: 'pre-wrap', marginBottom: 14 }}>
                {ev.refund_policy?.trim() || <span style={{ color: 'var(--text-3)' }}>Not set</span>}
              </div>
              <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--text-2)', marginBottom: 4 }}>Terms &amp; conditions</div>
              <div style={{ fontSize: 13.5, lineHeight: 1.6, whiteSpace: 'pre-wrap' }}>
                {ev.terms_and_conditions?.trim() || <span style={{ color: 'var(--text-3)' }}>Not set</span>}
              </div>
            </Panel>

            {/* ── Financial breakdown ────────────────────────────────────── */}
            <Panel title="Financial breakdown">
              <Row label="Gross ticket revenue" value={fmtMoney(stats.gross_ticket_revenue ?? 0, ev.currency)} />
              <Row label="Platform commission"  value={`− ${fmtMoney(stats.platform_fees_paid ?? 0, ev.currency)}`} />
              <Row label="Net ticket earnings"  value={fmtMoney(stats.net_ticket_earnings ?? 0, ev.currency)} />
              <Row label="Refunded"             value={fmtMoney(stats.total_refunded ?? 0, ev.currency)} />
              <Row label="Confirmed bookings"   value={String(stats.confirmed_bookings)} />
              <Row label="Pending bookings"     value={String(stats.pending_bookings)} />
              <Row label="Cancelled bookings"   value={String(stats.cancelled_bookings ?? 0)} />
              <Row label="Refunded bookings"    value={String(stats.refunded_bookings ?? 0)} />
            </Panel>

            {/* ── Gallery ────────────────────────────────────────────────── */}
            {ev.gallery_images && ev.gallery_images.length > 0 && (
              <Panel title={`Gallery (${ev.gallery_images.length})`}>
                <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
                  {ev.gallery_images.map((url, i) => (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img key={i} src={url} alt={`Gallery ${i + 1}`}
                      style={{ width: 120, height: 80, objectFit: 'cover', borderRadius: 6, border: '1px solid var(--border)' }} />
                  ))}
                </div>
              </Panel>
            )}

            {/* ── Record ─────────────────────────────────────────────────── */}
            <Panel title="Record">
              <Row label="Event ID"    value={ev.id} mono />
              <Row label="Slug"        value={ev.slug || '—'} mono />
              <Row label="Status"      value={ev.status} />
              <Row label="Published"   value={ev.published_at ? fmtDateTime(ev.published_at) : 'Not published'} />
              <Row label="Rights declared"
                   value={ev.rights_declared_at ? fmtDateTime(ev.rights_declared_at) : 'Not declared'} />
              <Row label="Featured"        value={ev.is_featured ? 'Yes' : 'No'} />
              <Row label="Admin promoted"  value={ev.admin_promoted ? 'Yes' : 'No'} />
              <Row label="Created"         value={ev.created_at ? fmtDateTime(ev.created_at) : '—'} />
              <Row label="Last updated"    value={ev.updated_at ? fmtDateTime(ev.updated_at) : '—'} />
            </Panel>

          </div>
        )}

        {tab === 'bookings' && (
          <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, overflow: 'hidden' }}>
            {bookings.length === 0 ? (
              <div className="empty-state">
                <div className="empty-title">No bookings yet</div>
              </div>
            ) : (
              <table className="meta-table">
                <thead>
                  <tr>
                    <th>Booking #</th>
                    <th>Attendee</th>
                    <th>Status</th>
                    <th>Amount</th>
                    <th>Date</th>
                  </tr>
                </thead>
                <tbody>
                  {bookings.map(b => (
                    <tr key={b.id}>
                      <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{b.booking_number}</td>
                      <td>
                        {b.attendee_name && b.attendee_name !== 'Guest' ? (
                          <div>
                            <div style={{ fontWeight: 600 }}>{b.attendee_name}</div>
                            {b.is_guest && (
                              <div style={{ fontSize: 12, color: 'var(--text-3)' }}>Guest checkout</div>
                            )}
                          </div>
                        ) : (
                          <span style={{ color: 'var(--text-3)' }}>Guest</span>
                        )}
                      </td>
                      <td>
                        <span className={`badge badge-${b.booking_status === 'confirmed' ? 'active' : b.booking_status === 'cancelled' ? 'cancelled' : 'pending'}`}>
                          {b.booking_status.charAt(0).toUpperCase() + b.booking_status.slice(1)}
                        </span>
                      </td>
                      <td style={{ fontWeight: 600 }}>
                        {fmtMoney(b.subtotal, b.currency)}
                        {b.ticket_quantity > 1 && (
                          <div style={{ fontSize: 11, fontWeight: 500, color: 'var(--text-3)' }}>
                            {b.ticket_quantity} x {fmtMoney(b.unit_price, b.currency)}
                          </div>
                        )}
                      </td>
                      <td style={{ color: 'var(--text-2)', fontSize: 13 }}>{fmtDateTime(b.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        )}

        {MERCH_ENABLED && tab === 'merch' && (
          <div>
            {merch.length === 0 ? (
              <div className="empty-state">
                <div className="empty-title">No merch for this event</div>
                <div className="empty-desc">Add merch items to sell alongside tickets.</div>
              </div>
            ) : (
              <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, overflow: 'hidden' }}>
                <table className="meta-table">
                  <thead>
                    <tr>
                      <th>Item</th>
                      <th>Status</th>
                      <th>Price</th>
                      <th>Stock</th>
                    </tr>
                  </thead>
                  <tbody>
                    {merch.map(m => (
                      <tr key={m.id}>
                        <td style={{ fontWeight: 600 }}>{m.name}</td>
                        <td>
                          <span className={`badge badge-${m.status === 'active' ? 'active' : 'draft'}`}>
                            {m.status}
                          </span>
                        </td>
                        <td>{fmtMoney(m.price, m.currency)}</td>
                        <td>{m.stock_quantity ?? 'Unlimited'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
// ── Detail presentation helpers ───────────────────────────────────────────────
// Plain presentational components — no hooks, so they can safely live at module
// scope and be used anywhere in the tree.

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{
      background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8,
      overflow: 'hidden', marginBottom: 16,
    }}>
      <div style={{
        padding: '14px 16px', borderBottom: '1px solid var(--border)',
        fontWeight: 700, fontSize: 15,
      }}>
        {title}
      </div>
      <div style={{ padding: 16 }}>{children}</div>
    </div>
  );
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div style={{
      display: 'flex', gap: 16, padding: '7px 0',
      borderBottom: '1px solid var(--surface-2)', fontSize: 13.5,
    }}>
      <div style={{ width: 180, flexShrink: 0, color: 'var(--text-2)' }}>{label}</div>
      <div style={{
        flex: 1, color: 'var(--text)', wordBreak: 'break-word',
        fontFamily: mono ? 'ui-monospace, SFMono-Regular, Menlo, monospace' : 'inherit',
        fontSize: mono ? 12.5 : 13.5,
      }}>
        {value}
      </div>
    </div>
  );
}

function ChipRow({ label, items }: { label: string; items?: string[] }) {
  return (
    <div style={{
      display: 'flex', gap: 16, padding: '7px 0',
      borderBottom: '1px solid var(--surface-2)', fontSize: 13.5,
    }}>
      <div style={{ width: 180, flexShrink: 0, color: 'var(--text-2)' }}>{label}</div>
      <div style={{ flex: 1, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {items && items.length > 0
          ? items.map((t, i) => (
              <span key={i} style={{
                background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 999,
                padding: '2px 10px', fontSize: 12.5, color: 'var(--text)',
              }}>
                {t}
              </span>
            ))
          : <span style={{ color: 'var(--text-3)' }}>None</span>}
      </div>
    </div>
  );
}