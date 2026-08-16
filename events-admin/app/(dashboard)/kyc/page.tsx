'use client';
import { useEffect, useState, useCallback } from 'react';
import { api } from '../../../lib/api';
import { fmtDateTime } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useStaff, AccessDenied } from '../../../lib/permissions';
import { useConfirm } from '../../../lib/confirm';

// ─────────────────────────────────────────────────────────────────────────────
// KYC review
//
// THE DOCUMENT RULE, which this screen exists to make legible:
//
//   before the first completed review   the reviewer sees the ID photo, because
//                                       you cannot verify an identity you are
//                                       not allowed to look at
//   after  the first completed review   master admin only, permanently
//
// The console does NOT decide that. It asks. Every submission comes back with
// can_view_document computed for this caller by the same function the document
// endpoint's gate uses, so the panel drawn here and the answer the server would
// give cannot disagree.
//
// The break-glass path for everyone else is a grant: an editor or admin asks, a
// master admin approves, and the grant is scoped to one organiser and expires
// on its own.
// ─────────────────────────────────────────────────────────────────────────────

// ── Types ────────────────────────────────────────────────────────────────────
interface QueueRow {
  organizer_id: string;
  business_name: string;
  business_email: string;
  status: string;
  method?: string;
  legal_first_name?: string;
  legal_last_name?: string;
  submitted_at?: string;
  has_registry_check: boolean;
}

interface KYCDetail {
  organizer_id: string;
  business_name: string;
  business_email: string;
  status: string;
  method?: string;
  legal_first_name?: string;
  legal_last_name?: string;
  sa_id_number?: string;
  date_of_birth?: string;
  submitted_at?: string;
  verified_at?: string;
  kyc_first_verified_at?: string;
  rejection_reason?: string;
  has_id_document: boolean;
  has_proof_of_address: boolean;
  bank_account_name?: string;
  name_match: 'match' | 'partial' | 'none' | 'unknown';
  // Server-computed, per caller. See the header.
  document_locked: boolean;
  can_view_document: boolean;
  document_access_via_grant?: boolean;
  document_grant_expires_at?: string;
  document_lock_reason?: string;
}

interface Grant {
  id: string;
  organizer_id: string;
  business_name?: string | null;
  requested_by: string;
  reason: string;
  requested_at: string;
  approved_at?: string | null;
  expires_at?: string | null;
  denied_at?: string | null;
}

interface AccessLogEntry {
  at: string;
  actor_email: string;
  action: string;
  reason: string;
  organizer_id?: string | null;
  business_name?: string | null;
  ip?: string | null;
}

const TABS = [
  { key: 'pending_review', label: 'Pending review' },
  { key: 'verified', label: 'Verified' },
  { key: 'rejected', label: 'Rejected' },
] as const;

const MATCH_STYLES: Record<string, { color: string; label: string; hint: string }> = {
  match:   { color: 'var(--success-fg)', label: 'Full match',    hint: 'Bank account name matches the legal name on the ID.' },
  partial: { color: 'var(--warning-fg)', label: 'Partial match', hint: 'Surname matches — initials or a business account. Judgement call.' },
  none:    { color: 'var(--danger-fg)', label: 'No match',      hint: 'Bank account name does not contain the legal surname. Investigate before approving.' },
  unknown: { color: 'var(--text-3)', label: 'No bank details', hint: 'Organizer has not saved payout bank details yet.' },
};

const REASON_CATEGORIES = [
  { key: 'dispute',             label: 'Dispute' },
  { key: 'regulator_request',   label: 'Regulator request' },
  { key: 'fraud_investigation', label: 'Fraud investigation' },
  { key: 'retention_audit',     label: 'Retention audit' },
  { key: 'other',               label: 'Other' },
];

function StatusPill({ status }: { status: string }) {
  const map: Record<string, string> = {
    pending_review: 'var(--warning-fg)', verified: 'var(--success-fg)', rejected: 'var(--danger-fg)', none: 'var(--text-3)',
  };
  const color = map[status] || 'var(--text-3)';
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center',
      background: color + '18', border: `1px solid ${color}40`,
      color, borderRadius: 99, padding: '2px 9px',
      fontSize: 12, fontWeight: 500, lineHeight: 1.5,
    }}>{status.replace('_', ' ')}</span>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', gap: 12, padding: '9px 0', borderBottom: '1px solid var(--border)' }}>
      <div style={{ width: 150, flexShrink: 0, fontSize: 12, fontWeight: 600, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '.04em', paddingTop: 2 }}>{label}</div>
      <div style={{ fontSize: 14, color: 'var(--text)', flex: 1, wordBreak: 'break-word' }}>{children}</div>
    </div>
  );
}

function LockIcon({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="3" y="11" width="18" height="11" rx="2.5" />
      <path d="M7 11V7a5 5 0 0110 0v4" />
    </svg>
  );
}

// ── Page ─────────────────────────────────────────────────────────────────────
export default function KYCPage() {
  const toast = useToast();
  const confirm = useConfirm();

  const capsLoaded    = useStaff((st) => st.loaded);
  const can           = useStaff((st) => st.can);
  const loadCaps      = useStaff((st) => st.load);
  const isMasterAdmin = useStaff((st) => st.isMasterAdmin);

  useEffect(() => { loadCaps(); }, [loadCaps]);

  const [tab, setTab] = useState<string>('pending_review');
  const [rows, setRows] = useState<QueueRow[]>([]);
  const [loading, setLoading] = useState(true);

  const [openID, setOpenID] = useState<string | null>(null);
  const [detail, setDetail] = useState<KYCDetail | null>(null);
  const [registryRaw, setRegistryRaw] = useState<string | null>(null);
  const [docURL, setDocURL] = useState<string | null>(null);
  const [docLoading, setDocLoading] = useState(false);
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);

  // Unlock form, shown to a master admin on a locked submission.
  const [unlockReason, setUnlockReason] = useState('');
  const [unlockCategory, setUnlockCategory] = useState('dispute');

  // Grant request, shown to editors and admins on a locked submission.
  const [grantReason, setGrantReason] = useState('');

  // Master admin side panels.
  const [panel, setPanel] = useState<'queue' | 'grants' | 'log'>('queue');
  const [grants, setGrants] = useState<Grant[]>([]);
  const [logEntries, setLogEntries] = useState<AccessLogEntry[]>([]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.get(`/admin/kyc?status=${tab}`);
      setRows(data.submissions || []);
    } catch (e) {
      const status = (e as { response?: { status?: number } })?.response?.status;
      if (status !== 403) toast('Failed to load KYC submissions', 'error');
      setRows([]);
    } finally {
      setLoading(false);
    }
  }, [tab, toast]);

  useEffect(() => { if (panel === 'queue') load(); }, [load, panel]);

  const loadGrants = useCallback(async () => {
    try {
      const { data } = await api.get('/admin/kyc/grants');
      setGrants(data.grants || []);
    } catch { toast('Could not load access requests', 'error'); }
  }, [toast]);

  const loadLog = useCallback(async () => {
    try {
      const { data } = await api.get('/admin/kyc/access-log');
      setLogEntries(data.entries || []);
    } catch { toast('Could not load the access log', 'error'); }
  }, [toast]);

  useEffect(() => {
    if (panel === 'grants') loadGrants();
    if (panel === 'log') loadLog();
  }, [panel, loadGrants, loadLog]);

  // ── Detail modal ───────────────────────────────────────────────────────────
  const openDetail = async (organizerId: string) => {
    setOpenID(organizerId);
    setDetail(null);
    setRegistryRaw(null);
    setReason('');
    setUnlockReason('');
    setGrantReason('');
    if (docURL) { URL.revokeObjectURL(docURL); setDocURL(null); }
    try {
      const { data } = await api.get(`/admin/kyc/${organizerId}`);
      const d: KYCDetail = data.detail;
      setDetail(d);
      setRegistryRaw(data.registry_raw || null);

      // Auto-fetch ONLY inside the review window, where no reason is required.
      //
      // A locked document is never fetched on open, even for a master admin.
      // Opening a submission is not the same as deciding to look at someone's
      // ID, and every read after the review window is written to the access
      // log permanently — so it has to be a deliberate click with a stated
      // purpose, not a side effect of navigation.
      if (d?.has_id_document && d.can_view_document && !d.document_locked) {
        void fetchDocument(organizerId, null, null);
      }
    } catch (e) {
      const status = (e as { response?: { status?: number } })?.response?.status;
      toast(status === 403
        ? 'You do not have permission to view this submission'
        : 'Failed to load submission detail', 'error');
      setOpenID(null);
    }
  };

  // The ID photo leaves the database ONLY through this POST. Fetched as a blob
  // and rendered from an object URL, so no public URL ever exists and the image
  // is revoked when the modal closes.
  const fetchDocument = async (
    organizerId: string, why: string | null, category: string | null,
  ) => {
    setDocLoading(true);
    try {
      const res = await api.post(
        `/admin/kyc/${organizerId}/document`,
        why ? { reason: why, category } : {},
        { responseType: 'blob' },
      );

      // An exported document comes back as JSON holding an object-store key,
      // not bytes. axios still types it as a blob, so it is read as text and
      // parsed — rendering it as an image would show a broken thumbnail.
      const ct = String(res.headers?.['content-type'] ?? '');
      if (ct.includes('application/json')) {
        const text = await (res.data as Blob).text();
        const body = JSON.parse(text);
        toast('Document is in the secure store — use the key with a signed URL', 'info');
        console.info('KYC document key:', body.key, 'store:', body.store);
        return;
      }

      if (docURL) URL.revokeObjectURL(docURL);
      setDocURL(URL.createObjectURL(res.data as Blob));
      if (why) toast('Access recorded in the document access log', 'info');
    } catch (e) {
      // The error body is a blob too, because responseType was set. Read it
      // back so the server's actual reason reaches the user instead of a
      // generic failure.
      let msg = 'Could not load the document';
      const data = (e as { response?: { data?: unknown } })?.response?.data;
      if (data instanceof Blob) {
        try { msg = JSON.parse(await data.text())?.error || msg; } catch { /* keep default */ }
      }
      toast(msg, 'error');
    } finally {
      setDocLoading(false);
    }
  };

  const unlockDocument = () => {
    if (!openID) return;
    if (unlockReason.trim().length < 10) {
      toast('A reason of at least 10 characters is required — it is recorded permanently', 'error');
      return;
    }
    confirm(
      'Open this identity document?',
      'This is recorded permanently in the document access log against your name, with the reason you gave. Continue?',
      () => fetchDocument(openID, unlockReason.trim(), unlockCategory),
    );
  };

  const requestGrant = async () => {
    if (!openID) return;
    if (grantReason.trim().length < 10) {
      toast('Explain why you need this — a master admin has to judge it', 'error');
      return;
    }
    setBusy(true);
    try {
      await api.post(`/admin/kyc/${openID}/request-access`, { reason: grantReason.trim() });
      toast('Request sent — a master admin must approve it', 'success');
      setGrantReason('');
    } catch (e) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      toast(msg ?? 'Could not send the request', 'error');
    } finally {
      setBusy(false);
    }
  };

  const actOnGrant = async (g: Grant, action: 'approve' | 'deny') => {
    try {
      await api.post(`/admin/kyc/grants/${g.id}/${action}`,
        action === 'approve' ? { hours: 24 } : {});
      toast(action === 'approve' ? 'Access granted for 24 hours' : 'Request denied', 'success');
      loadGrants();
    } catch (e) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      toast(msg ?? 'Could not update the request', 'error');
    }
  };

  const closeDetail = () => {
    setOpenID(null);
    setDetail(null);
    setRegistryRaw(null);
    if (docURL) { URL.revokeObjectURL(docURL); setDocURL(null); }
  };

  // Object URLs survive a page navigation, so the blob would stay resident
  // after the modal is gone without this.
  useEffect(() => () => { if (docURL) URL.revokeObjectURL(docURL); }, [docURL]);

  const openProofOfAddress = async () => {
    if (!openID || !detail) return;
    if (detail.document_locked && !detail.can_view_document) {
      toast(detail.document_lock_reason || 'You cannot view this document', 'error');
      return;
    }
    try {
      const body = detail.document_locked
        ? { reason: unlockReason.trim() || 'Proof of address review', category: unlockCategory }
        : {};
      const res = await api.post(`/admin/kyc/${openID}/proof-of-address`, body, { responseType: 'blob' });
      const url = URL.createObjectURL(res.data as Blob);
      window.open(url, '_blank');
      setTimeout(() => URL.revokeObjectURL(url), 60_000);
    } catch (e) {
      let msg = 'Could not open proof of address';
      const data = (e as { response?: { data?: unknown } })?.response?.data;
      if (data instanceof Blob) {
        try { msg = JSON.parse(await data.text())?.error || msg; } catch { /* keep default */ }
      }
      toast(msg, 'error');
    }
  };

  const decide = (approve: boolean) => {
    if (!openID || !detail) return;
    if (!approve && !reason.trim()) {
      toast('A rejection reason is required — the organizer sees it', 'error');
      return;
    }
    const title = approve ? 'Verify identity?' : 'Reject submission?';
    const msg = approve
      ? `Mark ${detail.legal_first_name || ''} ${detail.legal_last_name || ''} (${detail.business_name}) as identity-verified? They will be able to publish paid events immediately.\n\nThis also CLOSES their identity document: after this, only a master admin can open it.`
      : `Reject this submission? The organizer will see your reason and can resubmit.`;
    confirm(title, msg, async () => {
      setBusy(true);
      try {
        await api.post(`/admin/kyc/${openID}/decision`, { approve, reason: reason.trim() });
        toast(approve ? 'Identity verified — document now master-admin only' : 'Submission rejected', 'success');
        closeDetail();
        load();
      } catch (e) {
        const msg2 = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
        toast(msg2 || 'Decision failed', 'error');
      } finally {
        setBusy(false);
      }
    });
  };

  const runRegistryCheck = async () => {
    if (!openID) return;
    setBusy(true);
    try {
      const { data } = await api.post(`/admin/kyc/${openID}/registry-check`);
      setRegistryRaw(data.raw || null);
      if (data.full_match) {
        toast('DHA registry: full match — auto-verified', 'success');
        closeDetail();
        load();
      } else if (data.conclusive) {
        toast(`DHA registry returned: ${data.match_type}`, 'error');
      } else {
        toast('Registry check inconclusive — review manually', 'error');
      }
    } catch (e) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      toast(msg || 'Registry check failed (is DIDIT_API_KEY set?)', 'error');
    } finally {
      setBusy(false);
    }
  };

  const match = detail ? MATCH_STYLES[detail.name_match] || MATCH_STYLES.unknown : null;

  if (!capsLoaded) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }
  // kyc.status is the floor for this screen: the detail response carries the
  // decrypted SA ID number, so read_only has no business here.
  if (!can('kyc.status')) return <AccessDenied need="kyc.status" />;

  const canDecide = can('kyc.verify');

  return (
    <div style={{ padding: '24px 28px 48px', maxWidth: 1440, margin: '0 auto' }}>
      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: 18, gap: 16, flexWrap: 'wrap' }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>KYC review</h1>
        <span style={{ fontSize: 13, color: 'var(--text-3)' }}>
          Identity checks for organizers publishing paid events
        </span>
      </div>

      {/* Master admin panels. Hidden entirely otherwise — a greyed-out tab
          advertises that a document access log exists and invites probing. */}
      {isMasterAdmin && (
        <div className="tabs" style={{ marginBottom: 14 }}>
          <button className={`tab${panel === 'queue' ? ' active' : ''}`} onClick={() => setPanel('queue')}>Submissions</button>
          <button className={`tab${panel === 'grants' ? ' active' : ''}`} onClick={() => setPanel('grants')}>Access requests</button>
          <button className={`tab${panel === 'log' ? ' active' : ''}`} onClick={() => setPanel('log')}>Document access log</button>
        </div>
      )}

      {panel === 'grants' && (
        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 12, overflow: 'hidden' }}>
          {grants.length === 0 ? (
            <div style={{ padding: 32, textAlign: 'center', color: 'var(--text-3)', fontSize: 14 }}>
              No one has asked to view an identity document.
            </div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
              <thead>
                <tr style={{ background: 'var(--surface-2)', textAlign: 'left' }}>
                  {['Requested by', 'Organiser', 'Reason', 'Asked', 'State', ''].map(h => (
                    <th key={h} style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '.04em' }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {grants.map(g => {
                  const live = g.approved_at && !g.denied_at
                    && g.expires_at && new Date(g.expires_at) > new Date();
                  const pending = !g.approved_at && !g.denied_at;
                  return (
                    <tr key={g.id} style={{ borderTop: '1px solid var(--border)' }}>
                      <td style={{ padding: '11px 14px' }}>{g.requested_by}</td>
                      <td style={{ padding: '11px 14px' }}>{g.business_name || g.organizer_id}</td>
                      <td style={{ padding: '11px 14px', color: 'var(--text-2)', maxWidth: 320 }}>{g.reason}</td>
                      <td style={{ padding: '11px 14px', color: 'var(--text-2)' }}>{fmtDateTime(g.requested_at)}</td>
                      <td style={{ padding: '11px 14px' }}>
                        {g.denied_at ? <StatusPill status="rejected" />
                          : live ? <span style={{ fontSize: 12, color: 'var(--success-fg)' }}>Active until {fmtDateTime(g.expires_at!)}</span>
                          : g.approved_at ? <span style={{ fontSize: 12, color: 'var(--text-3)' }}>Expired</span>
                          : <StatusPill status="pending_review" />}
                      </td>
                      <td style={{ padding: '11px 14px', textAlign: 'right', whiteSpace: 'nowrap' }}>
                        {pending && (
                          <>
                            <button onClick={() => actOnGrant(g, 'approve')} style={{
                              padding: '6px 12px', borderRadius: 8, border: '1px solid var(--success-fg)',
                              background: 'var(--surface)', color: 'var(--success-fg)', fontSize: 13, fontWeight: 600, cursor: 'pointer', marginRight: 6,
                            }}>Approve 24h</button>
                            <button onClick={() => actOnGrant(g, 'deny')} style={{
                              padding: '6px 12px', borderRadius: 8, border: '1px solid var(--danger-fg)',
                              background: 'var(--surface)', color: 'var(--danger-fg)', fontSize: 13, fontWeight: 600, cursor: 'pointer',
                            }}>Deny</button>
                          </>
                        )}
                        {live && (
                          <button onClick={() => actOnGrant(g, 'deny')} style={{
                            padding: '6px 12px', borderRadius: 8, border: '1px solid var(--danger-fg)',
                            background: 'var(--surface)', color: 'var(--danger-fg)', fontSize: 13, fontWeight: 600, cursor: 'pointer',
                          }}>Revoke</button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>
      )}

      {panel === 'log' && (
        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 12, overflow: 'hidden' }}>
          <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--border)', fontSize: 12.5, color: 'var(--text-2)' }}>
            Every access to an identity document, permitted or refused. Append-only —
            it cannot be edited or deleted, including by you.
          </div>
          {logEntries.length === 0 ? (
            <div style={{ padding: 32, textAlign: 'center', color: 'var(--text-3)', fontSize: 14 }}>Nothing recorded yet.</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13.5 }}>
              <thead>
                <tr style={{ background: 'var(--surface-2)', textAlign: 'left' }}>
                  {['When', 'Who', 'Action', 'Organiser', 'Reason', 'IP'].map(h => (
                    <th key={h} style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '.04em' }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {logEntries.map((e, i) => (
                  <tr key={i} style={{ borderTop: '1px solid var(--border)' }}>
                    <td style={{ padding: '10px 14px', color: 'var(--text-2)', whiteSpace: 'nowrap' }}>{fmtDateTime(e.at)}</td>
                    <td style={{ padding: '10px 14px' }}>{e.actor_email}</td>
                    <td style={{ padding: '10px 14px' }}>
                      <span style={{
                        fontSize: 11.5, fontWeight: 700, padding: '2px 8px', borderRadius: 10,
                        background: e.action.includes('denied') ? 'var(--danger-bg)' : 'var(--success-bg)',
                        color: e.action.includes('denied') ? 'var(--danger-fg)' : 'var(--success-fg)',
                      }}>{e.action}</span>
                    </td>
                    <td style={{ padding: '10px 14px', color: 'var(--text-2)' }}>{e.business_name || '—'}</td>
                    <td style={{ padding: '10px 14px', color: 'var(--text-2)', maxWidth: 360 }}>{e.reason}</td>
                    <td style={{ padding: '10px 14px', color: 'var(--text-3)', fontFamily: 'ui-monospace, monospace', fontSize: 12 }}>{e.ip || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {panel === 'queue' && (
        <>
          {/* Tabs */}
          <div style={{ display: 'flex', gap: 6, marginBottom: 16 }}>
            {TABS.map(t => (
              <button key={t.key} onClick={() => setTab(t.key)} style={{
                padding: '7px 14px', borderRadius: 8, fontSize: 13, fontWeight: 600, cursor: 'pointer',
                border: '1px solid ' + (tab === t.key ? 'var(--accent)' : 'var(--border)'),
                background: tab === t.key ? 'var(--accent-soft)' : 'var(--surface)',
                color: tab === t.key ? 'var(--accent)' : 'var(--text-2)',
              }}>{t.label}</button>
            ))}
          </div>

          {/* Queue */}
          <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 12, overflow: 'hidden' }}>
            {loading ? (
              <div style={{ padding: 32, textAlign: 'center', color: 'var(--text-3)', fontSize: 14 }}>Loading…</div>
            ) : rows.length === 0 ? (
              <div style={{ padding: 32, textAlign: 'center', color: 'var(--text-3)', fontSize: 14 }}>
                {tab === 'pending_review' ? 'No submissions waiting — the queue is clear.' : 'Nothing here.'}
              </div>
            ) : (
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
                <thead>
                  <tr style={{ background: 'var(--surface-2)', textAlign: 'left' }}>
                    {['Business', 'Legal name', 'Status', 'Method', 'Submitted', 'Registry', ''].map(h => (
                      <th key={h} style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '.04em' }}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.map(r => (
                    <tr key={r.organizer_id} style={{ borderTop: '1px solid var(--border)' }}>
                      <td style={{ padding: '11px 14px' }}>
                        <div style={{ fontWeight: 600 }}>{r.business_name}</div>
                        <div style={{ fontSize: 12, color: 'var(--text-3)' }}>{r.business_email}</div>
                      </td>
                      <td style={{ padding: '11px 14px' }}>{[r.legal_first_name, r.legal_last_name].filter(Boolean).join(' ') || '—'}</td>
                      <td style={{ padding: '11px 14px' }}><StatusPill status={r.status} /></td>
                      <td style={{ padding: '11px 14px', color: 'var(--text-2)' }}>{r.method || '—'}</td>
                      <td style={{ padding: '11px 14px', color: 'var(--text-2)' }}>{r.submitted_at ? fmtDateTime(r.submitted_at) : '—'}</td>
                      <td style={{ padding: '11px 14px', color: 'var(--text-2)' }}>{r.has_registry_check ? 'Yes' : '—'}</td>
                      <td style={{ padding: '11px 14px', textAlign: 'right' }}>
                        <button onClick={() => openDetail(r.organizer_id)} style={{
                          padding: '6px 12px', borderRadius: 8, border: '1px solid var(--accent)',
                          background: 'var(--surface)', color: 'var(--accent)', fontSize: 13, fontWeight: 600, cursor: 'pointer',
                        }}>Review</button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </>
      )}

      {/* Detail modal */}
      {openID && (
        <div onClick={closeDetail} style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,.45)', zIndex: 50,
          display: 'flex', alignItems: 'flex-start', justifyContent: 'center', padding: '5vh 16px', overflowY: 'auto',
        }}>
          <div onClick={e => e.stopPropagation()} style={{
            background: 'var(--surface)', borderRadius: 14, width: '100%', maxWidth: 860,
            padding: 22, boxShadow: '0 12px 40px rgba(0,0,0,.18)',
          }}>
            {!detail ? (
              <div style={{ padding: 40, textAlign: 'center', color: 'var(--text-3)' }}>Loading…</div>
            ) : (
              <>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 8 }}>
                  <div>
                    <h2 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>{detail.business_name}</h2>
                    <div style={{ fontSize: 13, color: 'var(--text-3)' }}>{detail.business_email}</div>
                  </div>
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    <StatusPill status={detail.status} />
                    <button onClick={closeDetail} style={{ border: 'none', background: 'none', fontSize: 20, cursor: 'pointer', color: 'var(--text-3)' }}>✕</button>
                  </div>
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 320px', gap: 22 }}>
                  {/* Left: facts */}
                  <div>
                    <Row label="Legal name">{[detail.legal_first_name, detail.legal_last_name].filter(Boolean).join(' ') || '—'}</Row>
                    <Row label="SA ID number">
                      <span style={{ fontFamily: 'ui-monospace, monospace', letterSpacing: '.05em' }}>{detail.sa_id_number || '—'}</span>
                    </Row>
                    <Row label="Date of birth">{detail.date_of_birth || '—'}</Row>
                    <Row label="Submitted">{detail.submitted_at ? fmtDateTime(detail.submitted_at) : '—'}</Row>
                    {detail.verified_at && <Row label="Verified">{fmtDateTime(detail.verified_at)}</Row>}
                    {detail.rejection_reason && <Row label="Rejection reason">{detail.rejection_reason}</Row>}
                    <Row label="Bank account name">{detail.bank_account_name || '—'}</Row>
                    {match && (
                      <Row label="Name match">
                        <span style={{
                          display: 'inline-flex', background: match.color + '18', border: `1px solid ${match.color}40`,
                          color: match.color, borderRadius: 99, padding: '2px 9px', fontSize: 12, fontWeight: 600,
                        }}>{match.label}</span>
                        <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 4 }}>{match.hint}</div>
                      </Row>
                    )}
                    {registryRaw && (
                      <Row label="DHA registry result">
                        <pre style={{
                          margin: 0, fontSize: 11, background: 'var(--surface-2)', border: '1px solid var(--surface-3)',
                          borderRadius: 8, padding: 10, maxHeight: 160, overflow: 'auto', whiteSpace: 'pre-wrap',
                        }}>{registryRaw}</pre>
                      </Row>
                    )}
                    {detail.has_proof_of_address && (
                      <Row label="Proof of address">
                        <button onClick={openProofOfAddress} disabled={!detail.can_view_document} style={{
                          padding: '5px 11px', borderRadius: 8, border: '1px solid var(--border)',
                          background: 'var(--surface)', fontSize: 13,
                          cursor: detail.can_view_document ? 'pointer' : 'not-allowed',
                          opacity: detail.can_view_document ? 1 : .5,
                        }}>Open in new tab</button>
                      </Row>
                    )}
                  </div>

                  {/* Right: the document, or why not */}
                  <div>
                    <div style={{
                      fontSize: 12, fontWeight: 600, color: 'var(--text-3)',
                      textTransform: 'uppercase', letterSpacing: '.04em', marginBottom: 8,
                      display: 'flex', alignItems: 'center', gap: 6,
                    }}>
                      {detail.document_locked && <LockIcon size={13} />}
                      {detail.can_view_document ? 'ID document' : 'Verification basis'}
                    </div>

                    {/* Case 1 — visible: inside the review window, or a master
                        admin / grant holder who has already unlocked it. */}
                    {detail.can_view_document && (docURL || docLoading || !detail.document_locked) ? (
                      <div style={{
                        border: '1px solid var(--border)', borderRadius: 10, minHeight: 200,
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        background: 'var(--surface-2)', overflow: 'hidden',
                      }}>
                        {docLoading ? (
                          <span style={{ color: 'var(--text-3)', fontSize: 13 }}>Loading photo…</span>
                        ) : docURL ? (
                          // eslint-disable-next-line @next/next/no-img-element
                          <img src={docURL} alt="ID document" style={{ width: '100%', height: 'auto', display: 'block' }} />
                        ) : (
                          <span style={{ color: 'var(--text-3)', fontSize: 13 }}>
                            {detail.has_id_document ? 'Could not load' : 'No document'}
                          </span>
                        )}
                      </div>
                    ) : detail.can_view_document && detail.document_locked ? (
                      // Case 2 — permitted but LOCKED. A master admin, or an
                      // editor on a live grant. Deliberately not auto-fetched:
                      // opening a submission is not the same as deciding to
                      // look at someone's ID, and this read is permanent.
                      <div style={{
                        border: '1px solid var(--warning-border, var(--border))', borderRadius: 10,
                        padding: 16, background: 'var(--warning-bg, var(--surface-2))',
                      }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8, color: 'var(--warning-fg)' }}>
                          <LockIcon size={16} />
                          <span style={{ fontSize: 13.5, fontWeight: 700 }}>
                            {detail.document_access_via_grant ? 'Temporary access' : 'Break-glass access'}
                          </span>
                        </div>
                        <p style={{ fontSize: 12.5, color: 'var(--text-2)', margin: '0 0 12px', lineHeight: 1.6 }}>
                          KYC completed{detail.kyc_first_verified_at ? ` on ${fmtDateTime(detail.kyc_first_verified_at)}` : ''}.
                          {detail.document_access_via_grant && detail.document_grant_expires_at
                            ? ` Your grant expires ${fmtDateTime(detail.document_grant_expires_at)}.`
                            : ' Opening it now is recorded permanently against your name.'}
                        </p>

                        <select value={unlockCategory} onChange={e => setUnlockCategory(e.target.value)}
                          style={{
                            width: '100%', padding: '7px 9px', borderRadius: 8, fontSize: 13,
                            border: '1px solid var(--border)', background: 'var(--surface)',
                            color: 'var(--text)', marginBottom: 8, boxSizing: 'border-box',
                          }}>
                          {REASON_CATEGORIES.map(rc => <option key={rc.key} value={rc.key}>{rc.label}</option>)}
                        </select>

                        <textarea
                          value={unlockReason}
                          onChange={e => setUnlockReason(e.target.value)}
                          placeholder="Why do you need to see this? (min 10 characters — recorded permanently)"
                          style={{
                            width: '100%', minHeight: 62, borderRadius: 8, border: '1px solid var(--border)',
                            padding: 9, fontSize: 12.5, resize: 'vertical', boxSizing: 'border-box',
                            background: 'var(--surface)', color: 'var(--text)',
                          }}
                        />
                        <button onClick={unlockDocument} disabled={docLoading || unlockReason.trim().length < 10}
                          style={{
                            width: '100%', marginTop: 8, padding: '9px 12px', borderRadius: 8,
                            border: 'none', background: 'var(--warning-fg)', color: '#fff',
                            fontSize: 13.5, fontWeight: 700,
                            cursor: unlockReason.trim().length < 10 ? 'not-allowed' : 'pointer',
                            opacity: unlockReason.trim().length < 10 ? .55 : 1,
                          }}>
                          {docLoading ? 'Opening…' : 'Open identity document'}
                        </button>
                      </div>
                    ) : (
                      // Case 3 — not permitted. Explains the absence rather than
                      // leaving a blank panel: a reviewer who expects a photo
                      // and sees nothing assumes the upload failed and rejects
                      // a valid submission.
                      <div style={{
                        border: '1px solid var(--border)', borderRadius: 10, padding: 18,
                        background: 'var(--surface-2)',
                      }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 9, marginBottom: 10 }}>
                          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="var(--success)"
                            strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
                            <path d="M9 12l2 2 4-4" />
                          </svg>
                          <span style={{ fontSize: 13.5, fontWeight: 700, color: 'var(--text)' }}>
                            Verified against the Home Affairs registry
                          </span>
                        </div>
                        <p style={{ fontSize: 12.5, color: 'var(--text-2)', margin: 0, lineHeight: 1.65 }}>
                          Decide on the DHA result and the name match on the left.
                          {detail.has_id_document
                            ? ' An ID document is held securely, but you cannot open it.'
                            : ' No ID document was uploaded.'}
                        </p>
                        {detail.document_lock_reason && (
                          <p style={{ fontSize: 12, color: 'var(--text-3)', margin: '10px 0 0', lineHeight: 1.6 }}>
                            {detail.document_lock_reason}
                          </p>
                        )}

                        {/* Editors and admins may ASK. read_only and access
                            cannot hold a grant, so they are not offered one —
                            a button that can never succeed is worse than none. */}
                        {detail.document_locked && can('request.create') && (
                          <div style={{ marginTop: 12, paddingTop: 12, borderTop: '1px solid var(--border)' }}>
                            <textarea
                              value={grantReason}
                              onChange={e => setGrantReason(e.target.value)}
                              placeholder="Why do you need access? A master admin reviews this."
                              style={{
                                width: '100%', minHeight: 56, borderRadius: 8, border: '1px solid var(--border)',
                                padding: 9, fontSize: 12.5, resize: 'vertical', boxSizing: 'border-box',
                                background: 'var(--surface)', color: 'var(--text)',
                              }}
                            />
                            <button onClick={requestGrant} disabled={busy || grantReason.trim().length < 10}
                              style={{
                                width: '100%', marginTop: 8, padding: '8px 12px', borderRadius: 8,
                                border: '1px solid var(--accent)', background: 'var(--surface)',
                                color: 'var(--accent)', fontSize: 13, fontWeight: 600,
                                cursor: grantReason.trim().length < 10 ? 'not-allowed' : 'pointer',
                                opacity: grantReason.trim().length < 10 ? .55 : 1,
                              }}>
                              Request access
                            </button>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </div>

                {/* Actions */}
                {detail.status === 'pending_review' && canDecide && (
                  <div style={{ marginTop: 18, paddingTop: 16, borderTop: '1px solid var(--border)' }}>
                    <textarea
                      value={reason}
                      onChange={e => setReason(e.target.value)}
                      placeholder="Rejection reason (required to reject — the organizer sees this)"
                      style={{
                        width: '100%', minHeight: 56, borderRadius: 10, border: '1px solid var(--border)',
                        padding: 10, fontSize: 13, resize: 'vertical', boxSizing: 'border-box',
                        background: 'var(--surface)', color: 'var(--text)',
                      }}
                    />
                    <div style={{
                      fontSize: 12, color: 'var(--text-3)', margin: '8px 0 0',
                      display: 'flex', alignItems: 'center', gap: 6,
                    }}>
                      <LockIcon size={12} />
                      Verifying closes the ID document — after this only a master admin can open it.
                    </div>
                    <div style={{ display: 'flex', gap: 10, marginTop: 10, justifyContent: 'flex-end' }}>
                      <button onClick={runRegistryCheck} disabled={busy} style={{
                        padding: '9px 16px', borderRadius: 10, border: '1px solid var(--border)',
                        background: 'var(--surface)', color: 'var(--text-2)', fontSize: 14, fontWeight: 600, cursor: 'pointer',
                      }}>Run DHA registry check</button>
                      <button onClick={() => decide(false)} disabled={busy} style={{
                        padding: '9px 16px', borderRadius: 10, border: '1px solid var(--danger-fg)',
                        background: 'var(--surface)', color: 'var(--danger-fg)', fontSize: 14, fontWeight: 600, cursor: 'pointer',
                      }}>Reject</button>
                      <button onClick={() => decide(true)} disabled={busy} style={{
                        padding: '9px 18px', borderRadius: 10, border: 'none',
                        background: 'var(--success-fg)', color: '#fff', fontSize: 14, fontWeight: 600, cursor: 'pointer',
                      }}>{busy ? 'Working…' : 'Verify identity'}</button>
                    </div>
                  </div>
                )}
                {detail.status === 'pending_review' && !canDecide && (
                  <div style={{
                    marginTop: 18, paddingTop: 16, borderTop: '1px solid var(--border)',
                    fontSize: 13, color: 'var(--text-3)',
                  }}>
                    Your role can review this submission but not decide it.
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}