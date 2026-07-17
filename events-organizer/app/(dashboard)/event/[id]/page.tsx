'use client';
import { useState, useEffect, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import { api } from '../../../../lib/api';
import { useToast } from '../../../../lib/toast';
import { fmtMoney, fmtDate, fmtDateTime } from '../../../../lib/utils';

type Tab = 'overview' | 'bookings' | 'merch' | 'scan';

// Launch gate: merch is built but NOT launching — the tab and its section
// are hidden, not deleted. Keep in sync with MERCH_ENABLED in layout.tsx.
const MERCH_ENABLED = false;

interface EventStats {
  event: {
    id: string; title: string; status: string; start_time: string; end_time: string;
    city: string; venue_name: string; price: number; currency: string;
    tickets_sold: number; max_capacity: number | null; views: number;
    cover_image_url: string | null;
    banner_image_url: string | null;
  };
  total_revenue: number;
  total_bookings: number;
  confirmed_bookings: number;
  pending_bookings: number;
  active_campaign: {
    id: string; daily_budget: number; today_spent: number;
    total_impressions: number; total_clicks: number; status: string;
  } | null;
}

interface Booking {
  id: string; booking_number: string; booking_status: string;
  created_at: string; total_amount: number; currency: string;
  user?: { email: string; first_name: string; last_name: string };
}

interface MerchItem {
  id: string; name: string; status: string; price: number; currency: string;
  stock_quantity: number | null;
}

export default function EventDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const toast = useToast();

  const [tab, setTab] = useState<Tab>('overview');
  const [stats, setStats] = useState<EventStats | null>(null);
  const [bookings, setBookings] = useState<Booking[]>([]);
  const [merch, setMerch] = useState<MerchItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [coverUploading, setCoverUploading] = useState(false);
  const [bannerUploading, setBannerUploading] = useState(false);

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
  const capacityPct = ev.max_capacity
    ? Math.min((ev.tickets_sold / ev.max_capacity) * 100, 100)
    : null;

  return (
    <div className="page">

      {/* ── Page header with breadcrumb ───────────────────────────────── */}
      <div className="page-header">
        <div>
          <div style={{ fontSize: 12, color: '#606770', marginBottom: 4 }}>
            <Link href="/events" style={{ color: '#606770' }}>My Events</Link>
            {' → '}
            <span>{ev.title}</span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <div className="page-title">{ev.title}</div>
            <span className={`badge badge-${ev.status}`}>
              {ev.status.charAt(0).toUpperCase() + ev.status.slice(1)}
            </span>
          </div>
          <div style={{ fontSize: 13, color: '#606770', marginTop: 2 }}>
            {fmtDate(ev.start_time)} · {ev.venue_name}, {ev.city}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {ev.status === 'published' && !stats.active_campaign && (
            <Link href="/campaigns/create" className="btn btn-primary btn-sm">
              Boost this event
            </Link>
          )}
        </div>
      </div>

      {/* ── Campaign banner if active ─────────────────────────────────── */}
      {stats.active_campaign && (
        <div style={{
          background: '#e7f3ff', borderBottom: '1px solid #b8d9ff',
          padding: '10px 20px', display: 'flex', alignItems: 'center', gap: 12,
          fontSize: 13,
        }}>
          <svg viewBox="0 0 20 20" fill="#1877f2" width="16" height="16">
            <path d="M2 11a1 1 0 011-1h2a1 1 0 011 1v5a1 1 0 01-1 1H3a1 1 0 01-1-1v-5zM8 7a1 1 0 011-1h2a1 1 0 011 1v9a1 1 0 01-1 1H9a1 1 0 01-1-1V7zM14 4a1 1 0 011-1h2a1 1 0 011 1v12a1 1 0 01-1 1h-2a1 1 0 01-1-1V4z"/>
          </svg>
          <span style={{ fontWeight: 600, color: '#1877f2' }}>Active campaign</span>
          <span style={{ color: '#606770' }}>·</span>
          <span>{fmtMoney(stats.active_campaign.today_spent)} spent today</span>
          <span style={{ color: '#606770' }}>·</span>
          <span>{stats.active_campaign.total_impressions.toLocaleString()} impressions</span>
          <Link href="/campaigns" style={{ marginLeft: 'auto', color: '#1877f2', fontWeight: 600 }}>
            Manage →
          </Link>
        </div>
      )}

      {/* ── Tab bar ──────────────────────────────────────────────────── */}
      <div className="tab-bar">
        {([
          { id: 'overview' as Tab, label: 'Overview' },
          { id: 'bookings' as Tab, label: `Bookings (${stats.total_bookings})` },
          ...(MERCH_ENABLED ? [{ id: 'merch' as Tab, label: 'Merch' }] : []),
        ]).map(t => (
          <button key={t.id} className={`tab-item${tab === t.id ? ' active' : ''}`}
            onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}

        {/* Scan Tickets — opens full-screen scanner */}
        <Link
          href={`/event/${id}/scan`}
          style={{
            marginLeft: 'auto', display: 'inline-flex', alignItems: 'center', gap: 6,
            padding: '7px 14px', borderRadius: 6,
            background: '#1877f2', color: '#fff',
            fontSize: 13, fontWeight: 700, textDecoration: 'none',
            alignSelf: 'center',
          }}
        >
          <svg viewBox="0 0 20 20" fill="currentColor" width="14" height="14">
            <path fillRule="evenodd" d="M3 4a1 1 0 011-1h3a1 1 0 011 1v3a1 1 0 01-1 1H4a1 1 0 01-1-1V4zm2 2V5h1v1H5zM3 13a1 1 0 011-1h3a1 1 0 011 1v3a1 1 0 01-1 1H4a1 1 0 01-1-1v-3zm2 2v-1h1v1H5zM13 3a1 1 0 00-1 1v3a1 1 0 001 1h3a1 1 0 001-1V4a1 1 0 00-1-1h-3zm1 2v1h1V5h-1z" clipRule="evenodd"/>
            <path d="M11 4a1 1 0 10-2 0v1a1 1 0 002 0V4zM10 7a1 1 0 011 1v1h2a1 1 0 110 2h-3a1 1 0 01-1-1V8a1 1 0 011-1zM16 9a1 1 0 100 2 1 1 0 000-2zM9 13a1 1 0 011-1h1a1 1 0 110 2v2a1 1 0 11-2 0v-3zM7 11a1 1 0 100-2H4a1 1 0 100 2h3zM17 13a1 1 0 01-1 1h-2a1 1 0 110-2h2a1 1 0 011 1zM16 17a1 1 0 100-2h-3a1 1 0 100 2h3z"/>
          </svg>
          Scan Tickets
        </Link>
      </div>

      {/* ── Tab content ──────────────────────────────────────────────── */}
      <div className="page-content">

        {tab === 'overview' && (
          <div>
            {/* ── Event images ──────────────────────────────────────────── */}
            <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden', marginBottom: 16 }}>
              <div style={{ padding: '14px 16px', borderBottom: '1px solid #ebedf0', fontWeight: 700, fontSize: 15 }}>
                Event images
              </div>
              <div style={{ padding: 16, display: 'flex', gap: 24, flexWrap: 'wrap' }}>

                {/* Cover image */}
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600, color: '#606770', marginBottom: 8, textTransform: 'uppercase', letterSpacing: '.04em' }}>Cover image</div>
                  <div style={{ position: 'relative', width: 160, height: 100 }}>
                    {ev.cover_image_url ? (
                      <>
                        <img
                          src={ev.cover_image_url}
                          alt="Cover"
                          style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: 6, border: '1px solid #dddfe2', display: 'block' }}
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
                        width: '100%', height: '100%', border: '1.5px dashed #ccd0d5', borderRadius: 6,
                        display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
                        cursor: coverUploading ? 'wait' : 'pointer', color: '#8a8d91',
                        fontSize: 11, gap: 4, background: '#f7f8fa',
                      }}>
                        <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={coverUploading}
                          onChange={e => { const f = e.target.files?.[0]; if (f) { handleCoverUpload(f); e.target.value = ''; } }} />
                        {coverUploading
                          ? <div className="spinner" style={{ width: 18, height: 18 }} />
                          : <><span style={{ fontSize: '1.4rem', lineHeight: 1 }}>+</span><span>Upload cover</span></>}
                      </label>
                    )}
                  </div>
                  <div style={{ fontSize: 11, color: '#8a8d91', marginTop: 6 }}>JPEG · PNG · WebP · max 5 MB</div>
                </div>

                {/* Banner image */}
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600, color: '#606770', marginBottom: 8, textTransform: 'uppercase', letterSpacing: '.04em' }}>Banner image</div>
                  <div style={{ position: 'relative', width: 240, height: 100 }}>
                    {ev.banner_image_url ? (
                      <>
                        <img
                          src={ev.banner_image_url}
                          alt="Banner"
                          style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: 6, border: '1px solid #dddfe2', display: 'block' }}
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
                        width: '100%', height: '100%', border: '1.5px dashed #ccd0d5', borderRadius: 6,
                        display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
                        cursor: bannerUploading ? 'wait' : 'pointer', color: '#8a8d91',
                        fontSize: 11, gap: 4, background: '#f7f8fa',
                      }}>
                        <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={bannerUploading}
                          onChange={e => { const f = e.target.files?.[0]; if (f) { handleBannerUpload(f); e.target.value = ''; } }} />
                        {bannerUploading
                          ? <div className="spinner" style={{ width: 18, height: 18 }} />
                          : <><span style={{ fontSize: '1.4rem', lineHeight: 1 }}>+</span><span>Upload banner</span></>}
                      </label>
                    )}
                  </div>
                  <div style={{ fontSize: 11, color: '#8a8d91', marginTop: 6 }}>Wide hero image · JPEG · PNG · WebP · max 8 MB</div>
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
              <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, padding: '16px 20px', marginBottom: 16 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                  <span style={{ fontSize: 14, fontWeight: 600 }}>Capacity</span>
                  <span style={{ fontSize: 14, color: '#606770' }}>
                    {ev.tickets_sold} / {ev.max_capacity} ({capacityPct.toFixed(0)}%)
                  </span>
                </div>
                <div className="progress-bar" style={{ height: 8 }}>
                  <div className="progress-fill" style={{
                    width: `${capacityPct}%`,
                    background: capacityPct >= 90 ? '#c41c1c' : capacityPct >= 70 ? '#e65c00' : '#1877f2',
                  }} />
                </div>
                {capacityPct >= 90 && (
                  <div style={{ fontSize: 12, color: '#c41c1c', marginTop: 6, fontWeight: 600 }}>
                    ⚠ Almost sold out
                  </div>
                )}
              </div>
            )}

            {/* Campaign performance */}
            {stats.active_campaign && (
              <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden' }}>
                <div style={{ padding: '14px 16px', borderBottom: '1px solid #ebedf0', fontWeight: 700, fontSize: 15 }}>
                  Ad campaign performance
                </div>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', padding: '16px' }}>
                  {[
                    { label: 'Today spend', value: fmtMoney(stats.active_campaign.today_spent) },
                    { label: 'Daily budget', value: fmtMoney(stats.active_campaign.daily_budget) },
                    { label: 'Impressions', value: stats.active_campaign.total_impressions.toLocaleString() },
                    {
                      label: 'CTR',
                      value: stats.active_campaign.total_impressions > 0
                        ? `${((stats.active_campaign.total_clicks / stats.active_campaign.total_impressions) * 100).toFixed(2)}%`
                        : '0.00%',
                    },
                  ].map((item, i, arr) => (
                    <div key={item.label} style={{
                      padding: '0 16px',
                      borderRight: i < arr.length - 1 ? '1px solid #ebedf0' : 'none',
                    }}>
                      <div style={{ fontSize: 11, fontWeight: 600, color: '#606770', textTransform: 'uppercase', letterSpacing: '.04em', marginBottom: 4 }}>{item.label}</div>
                      <div style={{ fontSize: 20, fontWeight: 700 }}>{item.value}</div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {tab === 'bookings' && (
          <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden' }}>
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
                        {b.user ? (
                          <div>
                            <div style={{ fontWeight: 600 }}>{b.user.first_name} {b.user.last_name}</div>
                            <div style={{ fontSize: 12, color: '#8a8d91' }}>{b.user.email}</div>
                          </div>
                        ) : <span style={{ color: '#8a8d91' }}>Guest</span>}
                      </td>
                      <td>
                        <span className={`badge badge-${b.booking_status === 'confirmed' ? 'active' : b.booking_status === 'cancelled' ? 'cancelled' : 'pending'}`}>
                          {b.booking_status.charAt(0).toUpperCase() + b.booking_status.slice(1)}
                        </span>
                      </td>
                      <td style={{ fontWeight: 600 }}>{fmtMoney(b.total_amount, b.currency)}</td>
                      <td style={{ color: '#606770', fontSize: 13 }}>{fmtDateTime(b.created_at)}</td>
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
              <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden' }}>
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