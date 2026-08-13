'use client';
import { useCallback, useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { fmtMoney } from '../../../lib/utils';
import { useStaff, AccessDenied } from '../../../lib/permissions';

// ─────────────────────────────────────────────────────────────────────────────
// Event approvals — a review record, not a preview
//
// The old screen showed a hero image and a five-row summary, then two buttons.
// Approving on that basis means nobody ever looks at the ticket tiers, which is
// where the mistakes actually are: a "VIP" tier priced at R0, a tier with
// unlimited capacity at a 200-capacity venue, a hidden tier nobody will see.
//
// So this is laid out as a RECORD. Labelled field rows, monospace for ids,
// sections you can scan in a fixed order every time — the useful half of how a
// registry office presents a file, without the beige. Every field the event
// has is on screen, because "approve" is a statement that you looked.
//
// "Approve & Add to Home" is gone. It set is_featured with no end date, which
// is how a stale event sits on the homepage for months after it happened.
// Featuring is now a checkbox that opens a scheduler, and the window is
// required.
// ─────────────────────────────────────────────────────────────────────────────

interface Tier {
  name: string; price: number; capacity: number | null;
  description: string | null; is_visible: boolean; sort_order: number;
}

interface ReviewEvent {
  id: string; title: string; description: string | null; status: string;
  start_time: string; end_time: string; expires_at: string | null;
  timezone: string | null;
  venue_name: string | null; address: string | null; city: string | null;
  price: number; currency: string;
  max_capacity: number | null; max_tickets_per_user: number | null;
  cover_image_url: string | null; banner_image_url: string | null;
  category_name: string | null;
  organiser_id: string; organiser_name: string | null;
  organiser_email: string | null; organiser_kyc_level: number | null;
  is_featured: boolean; featured_from: string | null; featured_until: string | null;
  refund_policy: string | null; terms_and_conditions: string | null;
  created_at: string; tickets_sold: number;
}

interface QueueItem {
  id: string; title: string; city: string | null;
  start_time: string; submitted_at: string; price: number;
}

export default function ApprovalsPage() {
  const toast = useToast();
  const capsLoaded = useStaff((s) => s.loaded);
  const can = useStaff((s) => s.can);
  const loadCaps = useStaff((s) => s.load);

  const [queue, setQueue] = useState<QueueItem[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [detail, setDetail] = useState<ReviewEvent | null>(null);
  const [tiers, setTiers] = useState<Tier[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);

  const [feature, setFeature] = useState(false);
  const [showScheduler, setShowScheduler] = useState(false);
  const [featureFrom, setFeatureFrom] = useState('');
  const [featureDays, setFeatureDays] = useState(7);

  const [denyOpen, setDenyOpen] = useState(false);
  const [denyReason, setDenyReason] = useState('');

  useEffect(() => { loadCaps(); }, [loadCaps]);

  // Every setState sits in a promise continuation. Calling an async,
  // state-setting helper straight from an effect is the cascading render the
  // lint rule points at.
  const loadQueue = useCallback(() => {
    return api.get('/admin/dashboard/lists')
      .then(({ data }) => {
        const pending: QueueItem[] = data?.pending ?? [];
        setQueue(pending);
        setSelected((cur) => cur ?? pending[0]?.id ?? null);
      })
      .catch(() => { toast('Could not load the queue', 'error'); })
      .finally(() => { setLoading(false); });
  }, [toast]);

  useEffect(() => { loadQueue(); }, [loadQueue]);

  // Detail loads on selection. Fetched separately from the queue because the
  // queue is five short rows and the detail is everything — loading all of it
  // upfront would make opening the page slow to serve a screen showing one.
  useEffect(() => {
    // No synchronous clear on the empty branch — `detail` is DERIVED from
    // `selected` at render time instead (see `shown` below), so there is no
    // state to reset and nothing to cascade.
    if (!selected) return;
    let alive = true;
    api.get(`/admin/events/${selected}/review`)
      .then(({ data }) => {
        if (!alive) return;
        setDetail(data?.event ?? null);
        setTiers(data?.ticket_types ?? []);
        // Reset the featuring choice per event — carrying it across would
        // silently feature the next one you open.
        setFeature(false);
        setShowScheduler(false);
        setFeatureDays(7);
        setFeatureFrom('');
      })
      .catch(() => { if (alive) toast('Could not load event', 'error'); });
    return () => { alive = false; };
  }, [selected, toast]);

  async function approve() {
    if (!detail) return;
    setBusy(true);
    try {
      await api.put(`/admin/events/${detail.id}/approve-with-feature`, {
        feature,
        featured_from: featureFrom ? new Date(featureFrom).toISOString() : null,
        featured_days: feature ? featureDays : 0,
      });
      toast(feature ? 'Approved and featured' : 'Approved and published', 'success');
      setSelected(null);
      setDetail(null);
      loadQueue();
    } catch {
      toast('Could not approve', 'error');
    } finally {
      setBusy(false);
    }
  }

  async function deny() {
    if (!detail || denyReason.trim().length < 10) return;
    setBusy(true);
    try {
      await api.put(`/admin/events/${detail.id}/reject`, { reason: denyReason.trim() });
      toast('Event denied — the organiser has been told why', 'success');
      setDenyOpen(false);
      setDenyReason('');
      setSelected(null);
      setDetail(null);
      loadQueue();
    } catch {
      toast('Could not deny', 'error');
    } finally {
      setBusy(false);
    }
  }

  // Guards the window between picking a new event and its detail arriving:
  // without it the previous event's record would show under the new title.
  const shown = detail && detail.id === selected ? detail : null;
  const shownTiers = shown ? tiers : [];

  if (!capsLoaded || loading) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }
  if (!can('event.approve')) return <AccessDenied need="event.approve" />;

  return (
    <div style={{ padding: '24px 28px 48px', maxWidth: 1600, margin: '0 auto' }}>
      <div style={{
        display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start',
        gap: 16, marginBottom: 18,
      }}>
        <div>
          <div className="page-title">Event approvals</div>
          <div className="page-sub">Review and approve events submitted by organisers</div>
        </div>
        <span style={{ fontSize: 13, color: 'var(--text-3)' }}>
          {queue.length} pending
        </span>
      </div>

      <div style={{
        display: 'grid', gridTemplateColumns: 'minmax(0, 300px) minmax(0, 1fr)',
        gap: 18, alignItems: 'start',
      }}>
        {/* ── Queue ─────────────────────────────────────────────────────── */}
        <div className="card">
          <div className="card-header"><div className="card-title">Queue</div></div>
          <div>
            {queue.length === 0 && (
              <div style={{ padding: 26, textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>
                Nothing waiting — the queue is clear.
              </div>
            )}
            {queue.map((q) => (
              <button
                key={q.id}
                onClick={() => setSelected(q.id)}
                style={{
                  display: 'block', width: '100%', textAlign: 'left',
                  padding: '13px 16px', border: 'none', cursor: 'pointer',
                  borderLeft: `3px solid ${selected === q.id ? 'var(--accent)' : 'transparent'}`,
                  background: selected === q.id ? 'var(--accent-soft)' : 'transparent',
                  borderBottom: '1px solid var(--border)',
                  fontFamily: 'inherit',
                }}
              >
                <div style={{
                  fontSize: 13.5, fontWeight: 700,
                  color: selected === q.id ? 'var(--accent)' : 'var(--text)',
                }}>
                  {q.title}
                </div>
                <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 3 }}>
                  {q.city ?? 'No city'} · {waiting(q.submitted_at)}
                </div>
              </button>
            ))}
          </div>
        </div>

        {/* ── Record ────────────────────────────────────────────────────── */}
        {!shown ? (
          <div className="card">
            <div className="card-body" style={{
              padding: 60, textAlign: 'center', color: 'var(--text-3)', fontSize: 13.5,
            }}>
              Select an event from the queue to review it.
            </div>
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            {/* Record header */}
            <div className="card">
              <div style={{ padding: '18px 22px', borderBottom: '1px solid var(--border)' }}>
                <div style={{
                  display: 'flex', justifyContent: 'space-between',
                  alignItems: 'flex-start', gap: 16, flexWrap: 'wrap',
                }}>
                  <div style={{ minWidth: 0 }}>
                    <div style={{
                      fontSize: 10, fontWeight: 800, letterSpacing: '.12em',
                      textTransform: 'uppercase', color: 'var(--text-3)',
                    }}>
                      Event record
                    </div>
                    <div style={{
                      fontSize: 21, fontWeight: 800, letterSpacing: '-.02em',
                      marginTop: 3, color: 'var(--text)',
                    }}>
                      {shown.title}
                    </div>
                    {/* Monospace id, selectable. Reviewers quote these in
                        support threads and Slack; a proportional font makes
                        them misread and unselectable makes them retyped. */}
                    <div style={{
                      fontFamily: 'var(--font-mono)', fontSize: 11.5,
                      color: 'var(--text-3)', marginTop: 5, userSelect: 'all',
                    }}>
                      {shown.id}
                    </div>
                  </div>
                  <span className="badge badge-pending">{shown.status.replace(/_/g, ' ')}</span>
                </div>
              </div>

              {(shown.banner_image_url || shown.cover_image_url) ? (
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: shown.banner_image_url && shown.cover_image_url
                    ? 'minmax(0, 2fr) minmax(0, 1fr)' : '1fr',
                  gap: 1, background: 'var(--border)',
                }}>
                  {shown.banner_image_url && (
                    <Figure label="Banner" hint="Wide hero at the top of the event page"
                      src={shown.banner_image_url} ratio="16/7" />
                  )}
                  {shown.cover_image_url && (
                    <Figure label="Cover" hint="Card thumbnail in the feed"
                      src={shown.cover_image_url} ratio="16/9" />
                  )}
                </div>
              ) : (
                <div style={{
                  padding: 26, textAlign: 'center',
                  color: 'var(--warning-fg)', fontSize: 13, background: 'var(--warning-bg)',
                }}>
                  No images uploaded — the event will show a placeholder in the feed.
                </div>
              )}
            </div>

            <Record title="Organiser">
              <Field label="Business" value={shown.organiser_name} />
              <Field label="Email" value={shown.organiser_email} mono />
              <Field
                label="KYC level"
                value={shown.organiser_kyc_level != null ? `Level ${shown.organiser_kyc_level}` : null}
                // Level 2 is required to publish a paid or public event. A paid
                // event from a level-1 organiser should never reach this queue,
                // so flagging it here catches a gate that has failed upstream.
                warn={shown.price > 0 && (shown.organiser_kyc_level ?? 0) < 2
                  ? 'Paid event from an organiser below level 2' : undefined}
              />
              <Field label="Organiser ID" value={shown.organiser_id} mono />
            </Record>

            <Record title="Event details">
              <Field label="Category" value={shown.category_name} />
              <Field label="Description" value={shown.description} wide />
              <Field label="Submitted" value={dt(shown.created_at)} />
            </Record>

            <Record title="When">
              <Field label="Starts" value={dt(shown.start_time)} />
              <Field label="Ends" value={dt(shown.end_time)} />
              <Field label="Sales close" value={shown.expires_at ? dt(shown.expires_at) : null}
                warn={shown.expires_at && new Date(shown.expires_at) > new Date(shown.start_time)
                  ? 'Sales close after the event starts' : undefined} />
              <Field label="Timezone" value={shown.timezone} />
            </Record>

            <Record title="Where">
              <Field label="Venue" value={shown.venue_name} />
              <Field label="Address" value={shown.address} />
              <Field label="City" value={shown.city} />
            </Record>

            <Record title="Pricing & capacity">
              <Field label="Base price" value={shown.price > 0 ? fmtMoney(shown.price, shown.currency) : 'Free'} />
              <Field label="Capacity" value={shown.max_capacity ? String(shown.max_capacity) : 'Unlimited'} />
              <Field label="Max per buyer" value={shown.max_tickets_per_user ? String(shown.max_tickets_per_user) : 'No limit'} />
              <Field label="Sold so far" value={String(shown.tickets_sold)} />
            </Record>

            {/* Tiers — the part the old summary left out entirely. */}
            <div className="card">
              <div className="card-header">
                <div className="card-title">Ticket types</div>
                <span style={{ fontSize: 12, color: 'var(--text-3)' }}>
                  {shownTiers.length === 0 ? 'Flat pricing' : `${shownTiers.length} tiers`}
                </span>
              </div>
              {shownTiers.length > 0 && (
                <div className="table-wrap">
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
                      {shownTiers.map((t, i) => (
                        <tr key={i}>
                          <td>
                            <div style={{ fontWeight: 600 }}>{t.name}</div>
                            {t.description && (
                              <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 2 }}>
                                {t.description}
                              </div>
                            )}
                          </td>
                          <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>
                            {t.price > 0 ? fmtMoney(t.price, shown.currency) : 'Free'}
                          </td>
                          <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>
                            {t.capacity ?? '∞'}
                          </td>
                          <td>
                            {t.is_visible
                              ? <span className="badge badge-active">Visible</span>
                              : <span className="badge badge-draft">Hidden</span>}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>

            {(shown.refund_policy || shown.terms_and_conditions) && (
              <Record title="Legal">
                <Field label="Refund policy" value={shown.refund_policy} wide />
                <Field label="Terms" value={shown.terms_and_conditions} wide />
              </Record>
            )}

            {/* ── Verdict ─────────────────────────────────────────────── */}
            <div className="card" style={{ position: 'sticky', bottom: 0, zIndex: 10 }}>
              <div className="card-body" style={{ display: 'grid', gap: 14 }}>
                <label className="check-row">
                  <input
                    type="checkbox"
                    checked={feature}
                    onChange={(e) => {
                      setFeature(e.target.checked);
                      // Opening the scheduler on tick is what makes the window
                      // required rather than optional — the old button featured
                      // with no end date and nobody ever went back to unfeature.
                      if (e.target.checked) setShowScheduler(true);
                    }}
                  />
                  <span style={{ fontSize: 13.5 }}>
                    Make this a featured event
                    {feature && featureDays > 0 && (
                      <span style={{ color: 'var(--text-3)', marginLeft: 8, fontSize: 12.5 }}>
                        — {featureDays} days from {featureFrom ? d(featureFrom) : 'approval'}
                        {' · '}
                        <button type="button" onClick={() => setShowScheduler(true)}
                          style={{
                            background: 'none', border: 'none', padding: 0, cursor: 'pointer',
                            color: 'var(--accent)', fontSize: 12.5, textDecoration: 'underline',
                            fontFamily: 'inherit',
                          }}>
                          change
                        </button>
                      </span>
                    )}
                  </span>
                </label>

                <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', alignItems: 'center' }}>
                  <button className="btn btn-primary" disabled={busy} onClick={approve}>
                    {busy ? 'Working…' : feature ? 'Approve & feature' : 'Approve & publish'}
                  </button>
                  <button className="btn btn-danger" disabled={busy} onClick={() => setDenyOpen(true)}>
                    Deny
                  </button>
                  <span style={{ flex: 1 }} />
                  <span style={{ fontSize: 12, color: 'var(--text-3)' }}>
                    Review every section before approving.
                  </span>
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* ── Featuring scheduler ──────────────────────────────────────────── */}
      {showScheduler && (
        <div className="modal-overlay" onMouseDown={(e) => {
          if (e.target === e.currentTarget) { setShowScheduler(false); if (!featureDays) setFeature(false); }
        }}>
          <div className="modal modal-sm" role="dialog" aria-modal="true">
            <div className="modal-header">
              <div className="modal-title">Featuring window</div>
              <button className="modal-close" onClick={() => setShowScheduler(false)}>×</button>
            </div>
            <div className="modal-body">
              <p style={{ fontSize: 13, color: 'var(--text-2)', margin: '0 0 16px', lineHeight: 1.6 }}>
                Featured events appear at the top of the home feed. The window is
                required — a feature with no end date stays on the homepage long
                after the event is over.
              </p>

              <div className="field">
                <label>Start</label>
                <input
                  type="date"
                  value={featureFrom}
                  min={new Date().toISOString().slice(0, 10)}
                  onChange={(e) => setFeatureFrom(e.target.value)}
                  style={inputStyle}
                />
                <div className="field-hint">Leave blank to start as soon as it&apos;s approved</div>
              </div>

              <div className="field" style={{ marginBottom: 0 }}>
                <label>Duration</label>
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  {[3, 7, 14, 30].map((d2) => (
                    <button key={d2} type="button"
                      className={`pill${featureDays === d2 ? ' active' : ''}`}
                      onClick={() => setFeatureDays(d2)}>
                      {d2} days
                    </button>
                  ))}
                </div>
                <input
                  type="number" min={1} max={90} value={featureDays}
                  onChange={(e) => setFeatureDays(Math.min(90, Math.max(1, Number(e.target.value) || 1)))}
                  style={{ ...inputStyle, marginTop: 10 }}
                />
                <div className="field-hint">1–90 days</div>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => { setShowScheduler(false); setFeature(false); }}>
                Don&apos;t feature
              </button>
              <button className="btn btn-primary" onClick={() => { setFeature(true); setShowScheduler(false); }}>
                Set window
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Deny ─────────────────────────────────────────────────────────── */}
      {denyOpen && (
        <div className="modal-overlay" onMouseDown={(e) => { if (e.target === e.currentTarget) setDenyOpen(false); }}>
          <div className="modal modal-sm" role="dialog" aria-modal="true">
            <div className="modal-header">
              <div className="modal-title">Deny this event</div>
              <button className="modal-close" onClick={() => setDenyOpen(false)}>×</button>
            </div>
            <div className="modal-body">
              <div className="field" style={{ marginBottom: 0 }}>
                <label>Reason</label>
                <textarea
                  value={denyReason}
                  onChange={(e) => setDenyReason(e.target.value)}
                  rows={4}
                  placeholder="The organiser sees this, so say what needs fixing."
                  style={{ ...inputStyle, resize: 'vertical' }}
                />
                <div className="field-hint">
                  {denyReason.trim().length < 10
                    ? `${10 - denyReason.trim().length} more characters needed`
                    : 'The organiser can revise and resubmit'}
                </div>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setDenyOpen(false)}>Cancel</button>
              <button className="btn btn-danger" disabled={busy || denyReason.trim().length < 10} onClick={deny}>
                Deny event
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function Figure({ label, hint, src, ratio }: {
  label: string; hint: string; src: string; ratio: string;
}) {
  return (
    <figure style={{ margin: 0, background: 'var(--surface)' }}>
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img src={src} alt={label}
        style={{ width: '100%', aspectRatio: ratio, objectFit: 'cover', display: 'block' }} />
      <figcaption style={{ padding: '9px 14px' }}>
        <div style={{
          fontSize: 10.5, fontWeight: 800, letterSpacing: '.08em',
          textTransform: 'uppercase', color: 'var(--text-3)',
        }}>
          {label}
        </div>
        <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 2 }}>{hint}</div>
      </figcaption>
    </figure>
  );
}

// ── Record primitives ────────────────────────────────────────────────────────

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '10px 12px',
  border: '1px solid var(--border-2)', borderRadius: 'var(--r)',
  background: 'var(--surface-2)', color: 'var(--text)',
  fontSize: 14, fontFamily: 'inherit', outline: 'none', boxSizing: 'border-box',
};

function Record({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="card">
      <div className="card-header"><div className="card-title">{title}</div></div>
      <div className="card-body" style={{ display: 'grid', gap: 0 }}>{children}</div>
    </div>
  );
}

/**
 * One labelled row of the record.
 *
 * Label left, value right, hairline between — the layout that makes a file
 * scannable in a fixed order. An absent value renders "Not provided" in muted
 * text rather than collapsing: a missing field is information when you're
 * deciding whether to approve, and a row that vanishes is a field nobody
 * noticed was empty.
 */
function Field({
  label, value, mono = false, wide = false, warn,
}: {
  label: string; value: string | null | undefined;
  mono?: boolean; wide?: boolean; warn?: string;
}) {
  return (
    <div style={{
      display: wide ? 'block' : 'grid',
      gridTemplateColumns: wide ? undefined : '160px minmax(0, 1fr)',
      gap: wide ? 6 : 16,
      padding: '11px 0',
      borderBottom: '1px solid var(--border)',
    }}>
      <div style={{
        fontSize: 11, fontWeight: 700, letterSpacing: '.05em',
        textTransform: 'uppercase', color: 'var(--text-3)',
        paddingTop: wide ? 0 : 1,
      }}>
        {label}
      </div>
      <div style={{
        fontSize: 13.5,
        color: value ? 'var(--text)' : 'var(--text-faint)',
        fontFamily: mono ? 'var(--font-mono)' : undefined,
        lineHeight: 1.6,
        whiteSpace: wide ? 'pre-wrap' : undefined,
        userSelect: mono ? 'all' : undefined,
      }}>
        {value || 'Not provided'}
        {warn && (
          <div style={{
            marginTop: 6, fontSize: 12, fontWeight: 600,
            color: 'var(--warning-fg)',
          }}>
            ⚠ {warn}
          </div>
        )}
      </div>
    </div>
  );
}

function dt(iso: string): string {
  return new Date(iso).toLocaleString('en-ZA', {
    weekday: 'short', day: 'numeric', month: 'short', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  });
}

function d(date: string): string {
  return new Date(date).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' });
}

function waiting(since: string): string {
  const mins = Math.floor((Date.now() - new Date(since).getTime()) / 60000);
  if (mins < 60) return `${Math.max(1, mins)}m waiting`;
  const h = Math.floor(mins / 60);
  if (h < 24) return `${h}h waiting`;
  return `${Math.floor(h / 24)}d waiting`;
}