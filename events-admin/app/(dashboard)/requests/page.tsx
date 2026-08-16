'use client';
import { useCallback, useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { useStaff, AccessDenied } from '../../../lib/permissions';

// ─────────────────────────────────────────────────────────────────────────────
// Staff requests
//
// Where dangerous actions live between being asked for and happening. An editor
// logs one, a DIFFERENT admin approves it, the affected organiser confirms by
// OTP where required, a cooling-off elapses, and only then can it run.
//
// The screen's job is to make the gates legible. A request that cannot proceed
// shows WHY and WHAT IS MISSING, because "Execute is greyed out" with no reason
// is how someone concludes the tool is broken and asks for the action to be
// done by hand.
//
// Own-request approval is blocked here as well as in the API and in a database
// CHECK. The button is disabled with the reason on it — not hidden — because
// this is a temporary state tied to who is looking, not a permission the user
// lacks.
// ─────────────────────────────────────────────────────────────────────────────

interface StaffRequest {
  id: string;
  kind: string;
  status: string;
  reason: string;
  requested_at: string;
  expires_at: string;
  execute_not_before: string | null;
  subject_required: boolean;
  subject_confirmed_at: string | null;
  requested_by_email: string;
  approved_by_email: string | null;
  target_user_id: string | null;
  target_event_id: string | null;
  payload?: { changes?: Record<string, unknown> } | null;
  /** Server-computed: whose move it is, in words. */
  waiting_on?: string;
  /** Bank kinds — a plain admin may file but not approve. */
  needs_master_admin?: boolean;
}

const KIND_LABEL: Record<string, string> = {
  bank_details_delete: 'Remove bank details',
  bank_details_add: 'Add bank details',
  event_takedown: 'Take event down',
  role_change: 'Change platform role',
  credential_reset: 'Force credential reset',
  account_delete: 'Delete account',
  event_edit: 'Change event details',
};

/** What each action actually does, shown to the approver at the decision. */
const KIND_EFFECT: Record<string, string> = {
  bank_details_delete: 'Payouts will fail until new details are added. Settlement is held for 48 hours.',
  bank_details_add: 'Money will be sent to the new account. Requires the organiser to confirm.',
  event_takedown: 'The event is unpublished and leaves the feed. Ticket holders keep their tickets.',
  role_change: 'Changes what this account can do on the platform.',
  credential_reset: 'Signs the account out everywhere. Use when an account is compromised.',
  account_delete: 'Permanently anonymises the account. Cannot be undone.',
  event_edit: 'Rewrites a live event\u2019s details. Ticket holders have already bought against the current terms.',
};

const STATUS_TONE: Record<string, string> = {
  pending_approval: 'pending',
  awaiting_subject: 'pending',
  ready: 'active',
  executed: 'completed',
  rejected: 'cancelled',
  cancelled: 'draft',
  expired: 'draft',
};

export default function RequestsPage() {
  const toast = useToast();
  const capsLoaded = useStaff((s) => s.loaded);
  const can = useStaff((s) => s.can);
  const loadCaps = useStaff((s) => s.load);
  const masterAdmin = useStaff((s) => s.isMasterAdmin);

  const [requests, setRequests] = useState<StaffRequest[]>([]);
  const [filter, setFilter] = useState<string>('pending_approval');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [me, setMe] = useState<string | null>(null);

  const [rejectFor, setRejectFor] = useState<StaffRequest | null>(null);
  const [rejectReason, setRejectReason] = useState('');

  useEffect(() => { loadCaps(); }, [loadCaps]);

  // The console needs to know who it is, to spot the caller's own requests.
  useEffect(() => {
    let alive = true;
    api.get('/admin/me/capabilities')
      .then(({ data }) => { if (alive) setMe(data?.email ?? null); })
      .catch(() => {});
    return () => { alive = false; };
  }, []);

  const load = useCallback((status: string) => {
    return api.get('/admin/requests', { params: { status } })
      .then(({ data }) => { setRequests(data?.requests ?? []); })
      .catch(() => { toast('Could not load requests', 'error'); })
      .finally(() => { setLoading(false); });
  }, [toast]);

  useEffect(() => { load(filter); }, [filter, load]);

  async function act(r: StaffRequest, path: string, ok: string) {
    setBusy(r.id);
    try {
      await api.post(`/admin/requests/${r.id}/${path}`, {});
      toast(ok, 'success');
      load(filter);
    } catch (e) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      toast(msg ?? 'Action failed', 'error');
    } finally {
      setBusy(null);
    }
  }

  async function reject() {
    if (!rejectFor || rejectReason.trim().length < 5) return;
    setBusy(rejectFor.id);
    try {
      await api.post(`/admin/requests/${rejectFor.id}/reject`, { reason: rejectReason.trim() });
      toast('Request rejected', 'success');
      setRejectFor(null);
      setRejectReason('');
      load(filter);
    } catch {
      toast('Could not reject', 'error');
    } finally {
      setBusy(null);
    }
  }

  if (!capsLoaded || loading) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }
  // Editors can see the queue — they log requests and need to know what
  // happened to them. Only admins get the buttons.
  if (!can('request.create')) return <AccessDenied need="request.create" />;

  const canApprove = can('request.approve');
  const isMasterAdmin = masterAdmin;

  return (
    <div style={{ padding: '24px 28px 48px', maxWidth: 1440, margin: '0 auto' }}>
      <div style={{ marginBottom: 18 }}>
        <div className="page-title">Staff requests</div>
        <div className="page-sub">
          Dangerous actions need a second admin, and sometimes the affected
          organiser, before they run
        </div>
      </div>

      <div className="tabs">
        {[
          ['pending_approval', 'Awaiting approval'],
          ['awaiting_subject', 'Awaiting organiser'],
          ['ready', 'Ready to run'],
          ['executed', 'Done'],
          ['rejected', 'Rejected'],
        ].map(([id, label]) => (
          <button key={id} className={`tab${filter === id ? ' active' : ''}`}
            onClick={() => setFilter(id)}>
            {label}
          </button>
        ))}
      </div>

      {requests.length === 0 ? (
        <div className="card">
          <div className="card-body" style={{
            padding: 50, textAlign: 'center', color: 'var(--text-3)', fontSize: 13.5,
          }}>
            Nothing here.
          </div>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {requests.map((r) => {
            const mine = me != null && r.requested_by_email === me;
            const coolingOff = r.execute_not_before
              ? new Date(r.execute_not_before) > new Date()
              : false;

            return (
              <div key={r.id} className="card">
                <div className="card-body" style={{ display: 'grid', gap: 14 }}>
                  <div style={{
                    display: 'flex', justifyContent: 'space-between',
                    alignItems: 'flex-start', gap: 14, flexWrap: 'wrap',
                  }}>
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontSize: 15, fontWeight: 800, color: 'var(--text)' }}>
                        {KIND_LABEL[r.kind] ?? r.kind}
                      </div>
                      <div style={{ fontSize: 12.5, color: 'var(--text-3)', marginTop: 3 }}>
                        Asked by {r.requested_by_email} · {ago(r.requested_at)}
                      </div>
                    </div>
                    <span className={`badge badge-${STATUS_TONE[r.status] ?? 'draft'}`}>
                      {r.status.replace(/_/g, ' ')}
                    </span>
                  </div>

                  {/* The consequence, at the decision. An approver reading only
                      a kind name is approving a label, not an action. */}
                  <div className="alert alert-warning" style={{ marginBottom: 0, fontSize: 12.5 }}>
                    <span>{KIND_EFFECT[r.kind] ?? 'This action cannot be undone.'}</span>
                  </div>

                  <div>
                    <div style={{
                      fontSize: 10.5, fontWeight: 800, letterSpacing: '.08em',
                      textTransform: 'uppercase', color: 'var(--text-3)', marginBottom: 4,
                    }}>
                      Stated reason
                    </div>
                    <div style={{ fontSize: 13.5, color: 'var(--text)', lineHeight: 1.6 }}>
                      {r.reason}
                    </div>
                  </div>

                  {/* Whose move it is. A queue that only shows a status leaves
                      everyone assuming someone else has it — which is how a
                      filed request sits untouched and the system looks broken. */}
                  {r.waiting_on && (
                    <div style={{
                      fontSize: 13, fontWeight: 700,
                      color: r.needs_master_admin && !isMasterAdmin
                        ? 'var(--warning-fg)' : 'var(--text)',
                    }}>
                      {r.waiting_on}
                    </div>
                  )}

                  {/* The field diff, for an event edit. An approver reading only
                      a kind name is approving a label, not a change. */}
                  {r.kind === 'event_edit' && r.payload?.changes && (
                    <div>
                      <div style={{
                        fontSize: 10.5, fontWeight: 800, letterSpacing: '.08em',
                        textTransform: 'uppercase', color: 'var(--text-3)', marginBottom: 6,
                      }}>
                        Proposed changes
                      </div>
                      <div style={{
                        border: '1px solid var(--border)', borderRadius: 8,
                        overflow: 'hidden', fontSize: 12.5,
                      }}>
                        {Object.entries(r.payload.changes).map(([field, val], i) => (
                          <div key={field} style={{
                            display: 'flex', gap: 12, padding: '7px 11px',
                            borderTop: i === 0 ? 'none' : '1px solid var(--border)',
                          }}>
                            <span style={{
                              width: 130, flexShrink: 0, color: 'var(--text-3)',
                              fontWeight: 600,
                            }}>{field.replace(/_/g, ' ')}</span>
                            <span style={{ color: 'var(--text)', wordBreak: 'break-word' }}>
                              {typeof val === 'string' || typeof val === 'number'
                                ? String(val)
                                : JSON.stringify(val)}
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Gate checklist — what has happened and what has not. */}
                  <div style={{ display: 'flex', gap: 18, flexWrap: 'wrap' }}>
                    <Gate done label="Requested" detail={r.requested_by_email} />
                    <Gate
                      done={!!r.approved_by_email}
                      label="Second admin"
                      detail={r.approved_by_email ?? 'Waiting'}
                    />
                    {r.subject_required && (
                      <Gate
                        done={!!r.subject_confirmed_at}
                        label="Organiser confirmed"
                        detail={r.subject_confirmed_at ? ago(r.subject_confirmed_at) : 'Waiting on OTP'}
                      />
                    )}
                    {r.execute_not_before && (
                      <Gate
                        done={!coolingOff}
                        label="Cooling-off"
                        detail={coolingOff ? `Until ${time(r.execute_not_before)}` : 'Elapsed'}
                      />
                    )}
                  </div>

                  {canApprove && (
                    <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', alignItems: 'center' }}>
                      {r.status === 'pending_approval' && (
                        <>
                          <button
                            className="btn btn-primary"
                            disabled={mine || busy === r.id
                              || (!!r.needs_master_admin && !isMasterAdmin)}
                            // Disabled with the reason ON the button, not
                            // hidden: this is about who is looking, not about
                            // what they're allowed to do.
                            title={
                              mine ? 'You cannot approve your own request'
                                : r.needs_master_admin && !isMasterAdmin
                                  ? 'Bank detail changes must be approved by a master admin'
                                  : undefined
                            }
                            onClick={() => act(r, 'approve', 'Approved')}
                          >
                            {mine ? 'Your own request'
                              : r.needs_master_admin && !isMasterAdmin ? 'Needs master admin'
                              : 'Approve'}
                          </button>
                          <button className="btn btn-danger" disabled={busy === r.id}
                            onClick={() => setRejectFor(r)}>
                            Reject
                          </button>
                        </>
                      )}

                      {r.status === 'awaiting_subject' && (
                        <button className="btn" disabled={busy === r.id}
                          onClick={() => act(r, 'send-code', 'Confirmation code sent')}>
                          Resend confirmation code
                        </button>
                      )}

                      {r.status === 'ready' && (
                        <button
                          className="btn btn-primary"
                          disabled={coolingOff || busy === r.id}
                          title={coolingOff ? `Cooling-off until ${time(r.execute_not_before!)}` : undefined}
                          onClick={() => act(r, 'execute', 'Action performed')}
                        >
                          {coolingOff ? `Runs after ${time(r.execute_not_before!)}` : 'Execute now'}
                        </button>
                      )}

                      <span style={{ flex: 1 }} />
                      <span style={{ fontSize: 12, color: 'var(--text-3)' }}>
                        Expires {ago(r.expires_at, true)}
                      </span>
                    </div>
                  )}

                  {mine && r.status === 'pending_approval' && (
                    <div style={{ fontSize: 12.5, color: 'var(--text-3)' }}>
                      You asked for this, so someone else has to approve it. That
                      is the point of the rule, not a bug.
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {rejectFor && (
        <div className="modal-overlay" onMouseDown={(e) => {
          if (e.target === e.currentTarget) setRejectFor(null);
        }}>
          <div className="modal modal-sm" role="dialog" aria-modal="true">
            <div className="modal-header">
              <div className="modal-title">Reject request</div>
              <button className="modal-close" onClick={() => setRejectFor(null)}>×</button>
            </div>
            <div className="modal-body">
              <div className="field" style={{ marginBottom: 0 }}>
                <label>Reason</label>
                <textarea
                  rows={4}
                  value={rejectReason}
                  onChange={(e) => setRejectReason(e.target.value)}
                  placeholder="The person who asked sees this."
                  style={{
                    width: '100%', padding: '10px 12px', resize: 'vertical',
                    border: '1px solid var(--border-2)', borderRadius: 'var(--r)',
                    background: 'var(--surface-2)', color: 'var(--text)',
                    fontSize: 14, fontFamily: 'inherit', outline: 'none',
                    boxSizing: 'border-box',
                  }}
                />
                <div className="field-hint">Minimum 5 characters</div>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setRejectFor(null)}>Cancel</button>
              <button className="btn btn-danger"
                disabled={rejectReason.trim().length < 5 || busy === rejectFor.id}
                onClick={reject}>
                Reject
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/** One gate in the chain: satisfied or not, and by what. */
function Gate({ done, label, detail }: { done: boolean; label: string; detail: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8 }}>
      <span style={{
        width: 16, height: 16, borderRadius: '50%', flexShrink: 0, marginTop: 1,
        display: 'grid', placeItems: 'center',
        background: done ? 'var(--success-bg)' : 'var(--surface-strong)',
        color: done ? 'var(--success)' : 'var(--text-faint)',
        fontSize: 10, fontWeight: 800,
      }}>
        {done ? '✓' : '·'}
      </span>
      <div>
        <div style={{
          fontSize: 11, fontWeight: 700, letterSpacing: '.04em',
          textTransform: 'uppercase',
          color: done ? 'var(--text-2)' : 'var(--text-3)',
        }}>
          {label}
        </div>
        <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 1 }}>{detail}</div>
      </div>
    </div>
  );
}

function ago(iso: string, future = false): string {
  const diff = Math.abs(Date.now() - new Date(iso).getTime());
  const mins = Math.floor(diff / 60000);
  const suffix = future ? '' : ' ago';
  const prefix = future ? 'in ' : '';
  if (mins < 60) return `${prefix}${Math.max(1, mins)}m${suffix}`;
  const h = Math.floor(mins / 60);
  if (h < 24) return `${prefix}${h}h${suffix}`;
  return `${prefix}${Math.floor(h / 24)}d${suffix}`;
}

function time(iso: string): string {
  return new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}