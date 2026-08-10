'use client';
import { useEffect, useState, useCallback } from 'react';
import { api } from '../../../lib/api';
import { fmtDateTime } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';

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
  rejection_reason?: string;
  has_id_document: boolean;
  has_proof_of_address: boolean;
  bank_account_name?: string;
  name_match: 'match' | 'partial' | 'none' | 'unknown';
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

// ── Page ─────────────────────────────────────────────────────────────────────
export default function KYCPage() {
  const toast = useToast();
  const confirm = useConfirm();

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

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.get(`/admin/kyc?status=${tab}`);
      setRows(data.submissions || []);
    } catch {
      toast('Failed to load KYC submissions', 'error');
    } finally {
      setLoading(false);
    }
  }, [tab, toast]);

  useEffect(() => { load(); }, [load]);

  // ── Detail modal ───────────────────────────────────────────────────────────
  const openDetail = async (organizerId: string) => {
    setOpenID(organizerId);
    setDetail(null);
    setRegistryRaw(null);
    setReason('');
    if (docURL) { URL.revokeObjectURL(docURL); setDocURL(null); }
    try {
      const { data } = await api.get(`/admin/kyc/${organizerId}`);
      setDetail(data.detail);
      setRegistryRaw(data.registry_raw || null);
      if (data.detail?.has_id_document) {
        setDocLoading(true);
        // The ID photo only leaves the DB via this authenticated call — fetch
        // as a blob and render locally; no public URL ever exists.
        const res = await api.get(`/admin/kyc/${organizerId}/document`, { responseType: 'blob' });
        setDocURL(URL.createObjectURL(res.data));
      }
    } catch {
      toast('Failed to load submission detail', 'error');
      setOpenID(null);
    } finally {
      setDocLoading(false);
    }
  };

  const closeDetail = () => {
    setOpenID(null);
    setDetail(null);
    setRegistryRaw(null);
    if (docURL) { URL.revokeObjectURL(docURL); setDocURL(null); }
  };

  const openProofOfAddress = async () => {
    if (!openID) return;
    try {
      const res = await api.get(`/admin/kyc/${openID}/proof-of-address`, { responseType: 'blob' });
      const url = URL.createObjectURL(res.data);
      window.open(url, '_blank');
      setTimeout(() => URL.revokeObjectURL(url), 60_000);
    } catch {
      toast('Could not open proof of address', 'error');
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
      ? `Mark ${detail.legal_first_name || ''} ${detail.legal_last_name || ''} (${detail.business_name}) as identity-verified? They will be able to publish paid events immediately.`
      : `Reject this submission? The organizer will see your reason and can resubmit.`;
    confirm(title, msg, async () => {
      setBusy(true);
      try {
        await api.post(`/admin/kyc/${openID}/decision`, { approve, reason: reason.trim() });
        toast(approve ? 'Identity verified' : 'Submission rejected', 'success');
        closeDetail();
        load();
      } catch (e: any) {
        toast(e?.response?.data?.error || 'Decision failed', 'error');
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
    } catch (e: any) {
      toast(e?.response?.data?.error || 'Registry check failed (is DIDIT_API_KEY set?)', 'error');
    } finally {
      setBusy(false);
    }
  };

  const match = detail ? MATCH_STYLES[detail.name_match] || MATCH_STYLES.unknown : null;

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: 18 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>KYC review</h1>
        <span style={{ fontSize: 13, color: 'var(--text-3)' }}>
          Identity checks for organizers publishing paid events
        </span>
      </div>

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
                        <button onClick={openProofOfAddress} style={{
                          padding: '5px 11px', borderRadius: 8, border: '1px solid var(--border)',
                          background: 'var(--surface)', fontSize: 13, cursor: 'pointer',
                        }}>Open in new tab</button>
                      </Row>
                    )}
                  </div>

                  {/* Right: ID photo */}
                  <div>
                    <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '.04em', marginBottom: 8 }}>ID document</div>
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
                        <span style={{ color: 'var(--text-3)', fontSize: 13 }}>No document</span>
                      )}
                    </div>
                    <div style={{ fontSize: 11, color: 'var(--text-3)', marginTop: 6 }}>
                      Check: photo is a real SA ID, the number matches the one on the left, name matches, face is visible.
                    </div>
                  </div>
                </div>

                {/* Actions */}
                {detail.status === 'pending_review' && (
                  <div style={{ marginTop: 18, paddingTop: 16, borderTop: '1px solid var(--border)' }}>
                    <textarea
                      value={reason}
                      onChange={e => setReason(e.target.value)}
                      placeholder="Rejection reason (required to reject — the organizer sees this)"
                      style={{
                        width: '100%', minHeight: 56, borderRadius: 10, border: '1px solid var(--border)',
                        padding: 10, fontSize: 13, resize: 'vertical', boxSizing: 'border-box',
                      }}
                    />
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
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}