'use client';
import { useEffect, useState, useCallback } from 'react';
import { api } from '../../../lib/api';
import { fmtMoney, fmtDateTime } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';
import { useStaff, AccessDenied } from '../../../lib/permissions';

// ── Types ────────────────────────────────────────────────────────────────────
interface Eligibility {
  completed_paid_events: number;
  required_events: number;
  gross_processed: number;
  required_gross: number;
  account_age_days: number;
  required_age_days: number;
  open_refund_requests: number;
  has_verified_bank: boolean;
  eligible: boolean;
}

interface OrganizerRow {
  organizer_id: string;
  business_name: string;
  business_email: string;
  status: string;
  payout_tier: 'held' | 'instant';
  payout_enabled: boolean;
  has_subaccount: boolean;
  subaccount_code?: string | null;
  settlement_bank_name?: string | null;
  bank_account_name?: string | null;
  bank_account_last_4?: string | null;
  tier_upgraded_at?: string | null;
  eligibility?: Eligibility;
}

interface Payout {
  id: string;
  organizer_id: string;
  event_id: string;
  event_title: string;
  kind: 'event_settlement' | 'reserve_release';
  amount: number;
  currency: string;
  status: 'pending' | 'processing' | 'paid' | 'failed';
  failure_reason?: string | null;
  paystack_transfer_code?: string | null;
  created_at: string;
  paid_at?: string | null;
}

interface Bank { name: string; code: string; }

// ── Small UI helpers ─────────────────────────────────────────────────────────
function TierBadge({ tier }: { tier: string }) {
  const instant = tier === 'instant';
  return (
    <span style={{
      fontSize: 11, fontWeight: 700, padding: '2px 8px', borderRadius: 10,
      background: instant ? 'var(--accent-soft)' : 'var(--bg)',
      color: instant ? 'var(--accent)' : 'var(--text-2)',
    }}>
      {instant ? '⚡ Instant' : 'Held (post-event)'}
    </span>
  );
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, [string, string]> = {
    pending:    ['var(--warning-bg)', 'var(--warning-fg)'],
    processing: ['var(--accent-soft)', 'var(--accent)'],
    paid:       ['var(--success-bg)', 'var(--success-fg)'],
    failed:     ['var(--danger-bg)', 'var(--danger-fg)'],
  };
  const [bg, fg] = colors[status] ?? ['var(--bg)', 'var(--text-2)'];
  return (
    <span style={{ fontSize: 11, fontWeight: 700, padding: '2px 8px', borderRadius: 10, background: bg, color: fg }}>
      {status}
    </span>
  );
}

function Check({ ok, children }: { ok: boolean; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: ok ? 'var(--success-fg)' : 'var(--text-2)' }}>
      <span style={{ fontWeight: 700 }}>{ok ? '✓' : '○'}</span> {children}
    </div>
  );
}

// ── Page ─────────────────────────────────────────────────────────────────────
export default function OrganizersPage() {
  const toast = useToast();
  const confirm = useConfirm();

  // This screen shows bank account names and tails, so bank.view_tail is the
  // floor for seeing it at all — read_only has no business here.
  //
  // Everything that WRITES on this screen moves money: adding payout details
  // redirects where ticket revenue lands, tier decides whether it settles
  // instantly, and a retry pushes a transfer. All three sit at request.act,
  // which only an admin holds. An editor who needs one files a staff request
  // and a second admin approves it — that four-eyes path exists precisely
  // because these actions are the dangerous ones.
  const capsLoaded = useStaff((st) => st.loaded);
  const can = useStaff((st) => st.can);
  const loadCaps = useStaff((st) => st.load);
  useEffect(() => { loadCaps(); }, [loadCaps]);
  const canAct = can('request.act');

  const [tab, setTab] = useState<'organizers' | 'payouts'>('organizers');
  const [organizers, setOrganizers] = useState<OrganizerRow[]>([]);
  const [payouts, setPayouts] = useState<Payout[]>([]);
  const [payoutFilter, setPayoutFilter] = useState('');
  const [loading, setLoading] = useState(true);
  const [expanded, setExpanded] = useState<string | null>(null);

  // Bank details form state
  const [bankFormFor, setBankFormFor] = useState<OrganizerRow | null>(null);
  const [banks, setBanks] = useState<Bank[]>([]);
  const [bankForm, setBankForm] = useState({ bank_code: '', account_number: '', account_name: '' });
  const [saving, setSaving] = useState(false);
  const [retrying, setRetrying] = useState<string | null>(null);

  const loadOrganizers = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.get('/admin/organizers/payouts');
      setOrganizers(data.organizers || []);
    } catch {
      toast('Failed to load organizers', 'error');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  const loadPayouts = useCallback(async (status: string) => {
    setLoading(true);
    try {
      const { data } = await api.get('/admin/payouts' + (status ? `?status=${status}` : ''));
      setPayouts(data.payouts || []);
    } catch {
      toast('Failed to load payouts', 'error');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => { loadOrganizers(); }, [loadOrganizers]);
  useEffect(() => { if (tab === 'payouts') loadPayouts(payoutFilter); }, [tab, payoutFilter, loadPayouts]);

  async function openBankForm(org: OrganizerRow) {
    setBankFormFor(org);
    setBankForm({
      bank_code: '',
      account_number: '',
      account_name: org.bank_account_name || org.business_name,
    });
    if (banks.length === 0) {
      try {
        const { data } = await api.get('/admin/payouts/banks');
        setBanks(data.banks || []);
      } catch {
        toast('Failed to load bank list', 'error');
      }
    }
  }

  async function saveBankDetails() {
    if (!bankFormFor) return;
    const bank = banks.find(b => b.code === bankForm.bank_code);
    if (!bank || !bankForm.account_number || !bankForm.account_name) {
      toast('Fill in bank, account number and account name', 'error');
      return;
    }
    setSaving(true);
    try {
      await api.post(`/admin/organizers/${bankFormFor.organizer_id}/payout-details`, {
        bank_code: bank.code,
        bank_name: bank.name,
        account_number: bankForm.account_number.trim(),
        account_name: bankForm.account_name.trim(),
      });
      toast('Payout details saved — subaccount created', 'success');
      setBankFormFor(null);
      loadOrganizers();
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } };
      toast(err.response?.data?.error || 'Payment provider rejected these details', 'error');
    } finally {
      setSaving(false);
    }
  }

  function setTier(org: OrganizerRow, tier: 'held' | 'instant') {
    const promoting = tier === 'instant';
    const warn = promoting && org.eligibility && !org.eligibility.eligible
      ? ' ⚠️ This organizer does NOT meet the eligibility checklist — this is an override.'
      : '';
    confirm(
      promoting ? 'Promote to Instant settlement?' : 'Demote to Held settlement?',
      promoting
        ? `${org.business_name} will be settled directly by Paystack as tickets sell (minus fees + 10% reserve).${warn}`
        : `${org.business_name} will go back to post-event payouts. Future sales are platform-collected; already-split transactions are unaffected.`,
      async () => {
        try {
          await api.put(`/admin/organizers/${org.organizer_id}/tier`, { tier });
          toast(`Tier updated to ${tier}`, 'success');
          loadOrganizers();
        } catch (e: unknown) {
          const err = e as { response?: { data?: { error?: string } } };
          toast(err.response?.data?.error || 'Failed to update tier', 'error');
        }
      },
    );
  }

  async function retryPayout(p: Payout) {
    setRetrying(p.id);
    try {
      const { data } = await api.post(`/admin/payouts/${p.id}/retry`);
      const after = data.payout?.status;
      toast(after === 'paid' ? 'Payout paid ✓' : `Retry triggered — status: ${after ?? 'processing'}`,
        after === 'paid' ? 'success' : 'info');
      loadPayouts(payoutFilter);
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } };
      toast(err.response?.data?.error || 'Retry failed', 'error');
    } finally {
      setRetrying(null);
    }
  }

  // Matched to .meta-table in globals.css: sticky uppercase micro-headers,
  // hairline rules, tabular figures. Kept as inline objects rather than moved
  // to classes because this table is built with raw <table> markup and
  // rewriting it would be a functional change, not a visual one.
  const th: React.CSSProperties = {
    textAlign: 'left', padding: '11px 14px',
    fontSize: 10.5, fontWeight: 800, color: 'var(--text-3)',
    textTransform: 'uppercase', letterSpacing: '.09em',
    borderBottom: '1px solid var(--border)', whiteSpace: 'nowrap',
  };
  const td: React.CSSProperties = {
    padding: '13px 14px', fontSize: 13.5, color: 'var(--text)',
    borderBottom: '1px solid var(--border)', verticalAlign: 'top',
  };
  const btn: React.CSSProperties = {
    fontSize: 12.5, fontWeight: 600, padding: '7px 13px',
    borderRadius: 'var(--r)', border: '1px solid var(--border-2)',
    background: 'var(--surface-alpha)', color: 'var(--text-2)',
    cursor: 'pointer', fontFamily: 'inherit',
  };
  const btnPrimary: React.CSSProperties = {
    ...btn, background: 'var(--accent-deep)',
    borderColor: 'var(--accent-deep)', color: '#fff',
  };

  if (!capsLoaded) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }
  if (!can('bank.view_tail')) return <AccessDenied need="bank.view_tail" />;

  return (
    <div style={{ padding: '24px 28px 48px', maxWidth: 1600, margin: '0 auto' }}>
      <div style={{
        display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between',
        gap: 16, marginBottom: 18, flexWrap: 'wrap',
      }}>
        <div>
          <div className="page-title">Organizers &amp; Payouts</div>
          <div className="page-sub">Payout tiers, bank details, and the settlement ledger.</div>
        </div>
      </div>

      {/* Tabs moved out of the header and onto the shared .tabs control, so
          this page switches views the same way People and Staff Requests do.
          Same two buttons, same handlers — only the presentation changed. */}
      <div className="tabs">
        <button className={`tab${tab === 'organizers' ? ' active' : ''}`}
          onClick={() => setTab('organizers')}>Organizers</button>
        <button className={`tab${tab === 'payouts' ? ' active' : ''}`}
          onClick={() => setTab('payouts')}>Payout ledger</button>
      </div>

      {loading && <div className="spinner spinner-lg" style={{ margin: '48px auto' }} />}

      {/* ── Organizers tab ─────────────────────────────────────────────────── */}
      {!loading && tab === 'organizers' && (
        <div className="card">
          <div className="table-wrap">
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead><tr>
              <th style={th}>Business</th>
              <th style={th}>Payout tier</th>
              <th style={th}>Bank</th>
              <th style={th}>Eligibility</th>
              <th style={th}></th>
            </tr></thead>
            <tbody>
              {organizers.map(org => {
                const e = org.eligibility;
                const open = expanded === org.organizer_id;
                return (
                  <tr key={org.organizer_id}>
                    <td style={td}>
                      <div style={{ fontWeight: 600 }}>{org.business_name}</div>
                      <div style={{ fontSize: 12, color: 'var(--text-2)' }}>{org.business_email}</div>
                      <div style={{ fontSize: 11, color: 'var(--text-2)' }}>status: {org.status}</div>
                    </td>
                    <td style={td}>
                      <TierBadge tier={org.payout_tier} />
                      {org.tier_upgraded_at && org.payout_tier === 'instant' && (
                        <div style={{ fontSize: 11, color: 'var(--text-2)', marginTop: 4 }}>since {fmtDateTime(org.tier_upgraded_at)}</div>
                      )}
                    </td>
                    <td style={td}>
                      {org.has_subaccount ? (
                        <>
                          <div>{org.settlement_bank_name} ····{org.bank_account_last_4}</div>
                          <div style={{ fontSize: 11, color: 'var(--text-2)' }}>{org.bank_account_name}</div>
                          <div style={{ fontSize: 11, color: 'var(--text-2)', fontFamily: 'monospace' }}>{org.subaccount_code}</div>
                        </>
                      ) : (
                        <span style={{ fontSize: 12, color: 'var(--warning-fg)', fontWeight: 600 }}>No bank details</span>
                      )}
                    </td>
                    <td style={td}>
                      {e ? (
                        <>
                          <div style={{ fontSize: 12, fontWeight: 700, color: e.eligible ? 'var(--success-fg)' : 'var(--text-2)', cursor: 'pointer' }}
                               onClick={() => setExpanded(open ? null : org.organizer_id)}>
                            {e.eligible ? '✓ Eligible for Instant' : 'Not yet eligible'} {open ? '▾' : '▸'}
                          </div>
                          {open && (
                            <div style={{ marginTop: 6, display: 'flex', flexDirection: 'column', gap: 3 }}>
                              <Check ok={e.completed_paid_events >= e.required_events}>
                                {e.completed_paid_events}/{e.required_events} delivered paid events
                              </Check>
                              <Check ok={e.gross_processed >= e.required_gross}>
                                {fmtMoney(e.gross_processed)} / {fmtMoney(e.required_gross)} processed
                              </Check>
                              <Check ok={e.account_age_days >= e.required_age_days}>
                                {e.account_age_days}/{e.required_age_days} days on platform
                              </Check>
                              <Check ok={e.open_refund_requests === 0}>
                                {e.open_refund_requests} open refund requests
                              </Check>
                              <Check ok={e.has_verified_bank}>Bank details on file</Check>
                            </div>
                          )}
                        </>
                      ) : <span style={{ fontSize: 12, color: 'var(--text-2)' }}>—</span>}
                    </td>
                    <td style={{ ...td, whiteSpace: 'nowrap' }}>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 6, alignItems: 'flex-end' }}>
                        {canAct ? (
                          <>
                            <button style={btn} onClick={() => openBankForm(org)}>
                              {org.has_subaccount ? 'Edit bank details' : 'Add bank details'}
                            </button>
                            {org.payout_tier === 'held' ? (
                              <button
                                style={org.has_subaccount ? btnPrimary : { ...btn, opacity: 0.5, cursor: 'not-allowed' }}
                                disabled={!org.has_subaccount}
                                title={org.has_subaccount ? '' : 'Bank details required first'}
                                onClick={() => setTier(org, 'instant')}>
                                Promote to Instant
                              </button>
                            ) : (
                              <button style={btn} onClick={() => setTier(org, 'held')}>Demote to Held</button>
                            )}
                          </>
                        ) : (
                          // Hidden rather than disabled: a permanently greyed
                          // row of buttons is noise. The line says what to do
                          // instead, which is the useful part.
                          <span style={{ fontSize: 11.5, color: 'var(--text-3)', textAlign: 'right', maxWidth: 150 }}>
                            Payout changes need an admin
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
              {organizers.length === 0 && (
                <tr><td style={{ ...td, textAlign: 'center', color: 'var(--text-2)' }} colSpan={5}>No organizers yet</td></tr>
              )}
            </tbody>
          </table>
          </div>
        </div>
      )}

      {/* ── Payout ledger tab ──────────────────────────────────────────────── */}
      {!loading && tab === 'payouts' && (
        <>
          <div className="table-toolbar">
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
              {['', 'pending', 'processing', 'paid', 'failed'].map(s => (
                <button key={s || 'all'}
                  className={`pill${payoutFilter === s ? ' active' : ''}`}
                  onClick={() => setPayoutFilter(s)}>
                  {s ? s.charAt(0).toUpperCase() + s.slice(1) : 'All'}
                </button>
              ))}
            </div>
          </div>
          <div className="card">
            <div className="table-wrap">
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead><tr>
                <th style={th}>Event</th>
                <th style={th}>Type</th>
                <th style={th}>Amount</th>
                <th style={th}>Status</th>
                <th style={th}>Created</th>
                <th style={th}></th>
              </tr></thead>
              <tbody>
                {payouts.map(p => (
                  <tr key={p.id}>
                    <td style={td}>
                      <div style={{ fontWeight: 600 }}>{p.event_title || p.event_id}</div>
                      {p.paystack_transfer_code && (
                        <div style={{ fontSize: 11, color: 'var(--text-2)', fontFamily: 'monospace' }}>{p.paystack_transfer_code}</div>
                      )}
                      {p.failure_reason && (
                        <div style={{ fontSize: 11, color: 'var(--danger-fg)', marginTop: 2 }}>{p.failure_reason}</div>
                      )}
                    </td>
                    <td style={td}>{p.kind === 'event_settlement' ? 'Event settlement' : 'Reserve release'}</td>
                    <td style={{ ...td, fontWeight: 700 }}>{fmtMoney(p.amount, p.currency)}</td>
                    <td style={td}><StatusBadge status={p.status} /></td>
                    <td style={td}>
                      {fmtDateTime(p.created_at)}
                      {p.paid_at && <div style={{ fontSize: 11, color: 'var(--success-fg)' }}>paid {fmtDateTime(p.paid_at)}</div>}
                    </td>
                    <td style={{ ...td, textAlign: 'right' }}>
                      {canAct && (p.status === 'failed' || p.status === 'pending' || p.status === 'processing') && (
                        <button style={btn} disabled={retrying === p.id} onClick={() => retryPayout(p)}>
                          {retrying === p.id ? 'Retrying…' : p.status === 'failed' ? 'Retry' : 'Run now'}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
                {payouts.length === 0 && (
                  <tr><td style={{ ...td, textAlign: 'center', color: 'var(--text-2)' }} colSpan={6}>No payouts{payoutFilter ? ` with status '${payoutFilter}'` : ' yet'}</td></tr>
                )}
              </tbody>
            </table>
            </div>
          </div>
        </>
      )}

      {/* ── Bank details modal ─────────────────────────────────────────────── */}
      {bankFormFor && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.45)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50 }}
             onClick={() => !saving && setBankFormFor(null)}>
          <div style={{ background: 'var(--surface)', borderRadius: 10, padding: 20, width: 420, maxWidth: '90vw' }} onClick={e => e.stopPropagation()}>
            <h2 style={{ fontSize: 16, fontWeight: 800, margin: '0 0 4px' }}>Payout details — {bankFormFor.business_name}</h2>
            <p style={{ fontSize: 12, color: 'var(--text-2)', margin: '0 0 14px' }}>
              Creates the Paystack subaccount + transfer recipient. Only the last 4 digits are stored on our side.
            </p>

            <label style={{ fontSize: 12, fontWeight: 700, color: 'var(--text-2)', display: 'block', marginBottom: 4 }}>Bank</label>
            <select value={bankForm.bank_code}
                    onChange={e => setBankForm(f => ({ ...f, bank_code: e.target.value }))}
                    style={{ width: '100%', padding: '8px 10px', borderRadius: 6, border: '1px solid var(--border)', fontSize: 13, marginBottom: 12 }}>
              <option value="">{banks.length ? 'Select a bank…' : 'Loading banks…'}</option>
              {banks.map(b => <option key={b.code} value={b.code}>{b.name}</option>)}
            </select>

            <label style={{ fontSize: 12, fontWeight: 700, color: 'var(--text-2)', display: 'block', marginBottom: 4 }}>Account number</label>
            <input value={bankForm.account_number}
                   onChange={e => setBankForm(f => ({ ...f, account_number: e.target.value.replace(/\D/g, '') }))}
                   placeholder="e.g. 1234567890" inputMode="numeric"
                   style={{ width: '100%', padding: '8px 10px', borderRadius: 6, border: '1px solid var(--border)', fontSize: 13, marginBottom: 12, boxSizing: 'border-box' }} />

            <label style={{ fontSize: 12, fontWeight: 700, color: 'var(--text-2)', display: 'block', marginBottom: 4 }}>Account holder name</label>
            <input value={bankForm.account_name}
                   onChange={e => setBankForm(f => ({ ...f, account_name: e.target.value }))}
                   style={{ width: '100%', padding: '8px 10px', borderRadius: 6, border: '1px solid var(--border)', fontSize: 13, marginBottom: 6, boxSizing: 'border-box' }} />
            <p style={{ fontSize: 11, color: 'var(--warning-fg)', margin: '0 0 14px' }}>
              ⚠️ Paystack does not verify ZA account-holder names — double-check the account number. Money sent to a wrong account is not recoverable by us.
            </p>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              <button style={btn} disabled={saving} onClick={() => setBankFormFor(null)}>Cancel</button>
              <button style={btnPrimary} disabled={saving} onClick={saveBankDetails}>
                {saving ? 'Verifying with Paystack…' : 'Save details'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}