'use client';
import { useState, useEffect, useCallback } from 'react';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { fmtMoney, fmtDate, fmtDateTime } from '../../../lib/utils';

type SubPage = 'accounts' | 'payment-activity';

interface AdAccount {
  id: string; name: string; status: string;
  stitch_mandate_status: string; has_payment_method: boolean;
  ad_payment_email: string | null; total_spent: number;
  active_campaigns: number; currency: string; created_at: string;
}

interface BillingRecord {
  id: string; billing_date: string; amount: number;
  status: string; failure_reason: string | null; completed_at: string | null;
}

export default function PayoutsPage() {
  const toast = useToast();
  const [subPage, setSubPage] = useState<SubPage>('accounts');
  const [accounts, setAccounts] = useState<AdAccount[]>([]);
  const [billing, setBilling] = useState<BillingRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedAccount, setSelectedAccount] = useState<AdAccount | null>(null);
  const [showPaymentModal, setShowPaymentModal] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [accRes, billRes] = await Promise.all([
        api.get('/organiser/ad-accounts'),
        api.get('/organiser/ad-billing'),
      ]);
      const accs = accRes.data.accounts ?? [];
      setAccounts(accs);
      setBilling(billRes.data.records ?? []);
      if (accs.length > 0 && !selectedAccount) setSelectedAccount(accs[0]);
    } catch {
      toast('Failed to load billing data', 'error');
    } finally { setLoading(false); }
  }, []);

  useEffect(() => { fetchData(); }, [fetchData]);

  const totalSpent = accounts.reduce((s, a) => s + a.total_spent, 0);
  const hasAnyPayment = accounts.some(a => a.has_payment_method);

  return (
    <div className="page">

      {/* ── Page header — "Billing & payments" title style ─────────────── */}
      <div className="page-header">
        <div className="page-title">Billing &amp; payments</div>
      </div>

      {/* ── Body: page-level sub-nav + content ───────────────────────── */}
      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>

        {/* Sub-nav — mirrors Meta's Accounts | Payment methods | Payment activity | Credit lines */}
        <div style={{
          width: 220, flexShrink: 0, background: '#fff',
          borderRight: '1px solid #dddfe2', padding: '12px 0', overflow: 'auto',
        }}>
          {([
            {
              id: 'accounts' as SubPage, label: 'Accounts',
              icon: (
                <svg viewBox="0 0 20 20" fill="currentColor" width="16" height="16">
                  <path d="M4 4a2 2 0 00-2 2v1h16V6a2 2 0 00-2-2H4z"/>
                  <path fillRule="evenodd" d="M18 9H2v5a2 2 0 002 2h12a2 2 0 002-2V9zM4 13a1 1 0 011-1h1a1 1 0 110 2H5a1 1 0 01-1-1zm5-1a1 1 0 100 2h1a1 1 0 100-2H9z" clipRule="evenodd"/>
                </svg>
              ),
            },
            {
              id: 'payment-activity' as SubPage, label: 'Payment activity',
              icon: (
                <svg viewBox="0 0 20 20" fill="currentColor" width="16" height="16">
                  <path fillRule="evenodd" d="M3 3a1 1 0 000 2v8a2 2 0 002 2h2.586l-1.293 1.293a1 1 0 101.414 1.414L10 15.414l2.293 2.293a1 1 0 001.414-1.414L12.414 15H15a2 2 0 002-2V5a1 1 0 100-2H3zm11 4a1 1 0 10-2 0v4a1 1 0 102 0V7zm-3 1a1 1 0 10-2 0v3a1 1 0 102 0V8zM8 9a1 1 0 00-2 0v2a1 1 0 102 0V9z" clipRule="evenodd"/>
                </svg>
              ),
            },
          ] as const).map(item => (
            <button
              key={item.id}
              onClick={() => setSubPage(item.id)}
              style={{
                display: 'flex', alignItems: 'center', gap: 10,
                width: '100%', padding: '9px 16px', border: 'none',
                background: subPage === item.id ? '#e7f3ff' : 'transparent',
                color: subPage === item.id ? '#1877f2' : '#1c1e21',
                fontWeight: subPage === item.id ? 700 : 500,
                fontSize: 14, cursor: 'pointer', textAlign: 'left',
                borderLeft: `3px solid ${subPage === item.id ? '#1877f2' : 'transparent'}`,
              }}
            >
              <span style={{ color: subPage === item.id ? '#1877f2' : '#606770' }}>{item.icon}</span>
              {item.label}
            </button>
          ))}
        </div>

        {/* Main content area */}
        <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
          {loading ? (
            <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 60 }}>
              <div className="spinner spinner-lg" />
            </div>
          ) : subPage === 'accounts' ? (
            <AccountsSubPage
              accounts={accounts}
              selectedAccount={selectedAccount}
              onSelectAccount={setSelectedAccount}
              onAddPayment={(id) => setShowPaymentModal(id)}
              totalSpent={totalSpent}
              hasAnyPayment={hasAnyPayment}
              onRefresh={fetchData}
            />
          ) : (
            <PaymentActivitySubPage records={billing} />
          )}
        </div>

        {/* Right: help sidebar — same as Meta's "Payment history / Help center" panel */}
        <div style={{
          width: 220, flexShrink: 0, padding: 20,
          borderLeft: '1px solid #dddfe2', background: '#fff',
        }}>
          <div style={{ fontSize: 14, fontWeight: 700, color: '#1c1e21', marginBottom: 12 }}>
            Payment history
          </div>
          <div style={{ fontSize: 13, color: '#1877f2', cursor: 'pointer', marginBottom: 20 }}>
            View transaction history
          </div>
          <div style={{ height: 1, background: '#ebedf0', marginBottom: 16 }}/>
          <div style={{ fontSize: 14, fontWeight: 700, color: '#1c1e21', marginBottom: 12 }}>
            Help center
          </div>
          {[
            'How post-pay billing works',
            'What to do if a payment fails',
            'Managing ad accounts',
            'Stitch mandate setup',
          ].map(link => (
            <div key={link} style={{
              fontSize: 13, color: '#1877f2', marginBottom: 10, cursor: 'pointer',
              display: 'flex', alignItems: 'center', gap: 6,
            }}>
              <svg viewBox="0 0 20 20" fill="currentColor" width="12" height="12">
                <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd"/>
              </svg>
              {link}
            </div>
          ))}
        </div>
      </div>

      {/* Payment method modal */}
      {showPaymentModal && (
        <RegisterPaymentModal
          accountId={showPaymentModal}
          onClose={() => setShowPaymentModal(null)}
          onSaved={() => { setShowPaymentModal(null); fetchData(); toast('Payment method saved', 'success'); }}
        />
      )}
    </div>
  );
}

// ── Accounts sub-page — mirrors Meta's Billing > Accounts layout ───────────────
function AccountsSubPage({
  accounts, selectedAccount, onSelectAccount, onAddPayment,
  totalSpent, hasAnyPayment, onRefresh,
}: {
  accounts: AdAccount[];
  selectedAccount: AdAccount | null;
  onSelectAccount: (a: AdAccount) => void;
  onAddPayment: (id: string) => void;
  totalSpent: number;
  hasAnyPayment: boolean;
  onRefresh: () => void;
}) {
  if (accounts.length === 0) {
    return (
      <div>
        {/* Setup prompt card — same as "Set up your account" in Meta */}
        <div style={{
          background: '#fff', border: '1px solid #dddfe2', borderRadius: 8,
          padding: 20, marginBottom: 16,
        }}>
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10 }}>
            <svg viewBox="0 0 20 20" fill="#1877f2" width="18" height="18" style={{ flexShrink: 0, marginTop: 1 }}>
              <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd"/>
            </svg>
            <div>
              <div style={{ fontWeight: 700, fontSize: 15 }}>Set up your account</div>
              <div style={{ fontSize: 13, color: '#606770', marginTop: 4 }}>
                Create an ad account and add a payment method to start running campaigns.
              </div>
            </div>
          </div>
        </div>
        <div className="empty-state">
          <div className="empty-icon">💳</div>
          <div className="empty-title">No ad accounts</div>
          <div className="empty-desc">Go to Campaigns → Ad Accounts to create one.</div>
        </div>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', gap: 20 }}>
      {/* Account list */}
      <div style={{ flex: 1, minWidth: 0 }}>

        {!hasAnyPayment && (
          <div style={{
            background: '#fff', border: '1px solid #dddfe2', borderRadius: 8,
            padding: '14px 16px', marginBottom: 16,
            display: 'flex', alignItems: 'flex-start', gap: 10,
          }}>
            <svg viewBox="0 0 20 20" fill="#1877f2" width="16" height="16" style={{ flexShrink: 0, marginTop: 1 }}>
              <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd"/>
            </svg>
            <div style={{ flex: 1 }}>
              <div style={{ fontWeight: 700, fontSize: 14 }}>Set up your account</div>
              <div style={{ fontSize: 13, color: '#606770', marginTop: 2 }}>
                Add a Stitch debit mandate to start running ad campaigns.
              </div>
            </div>
            {selectedAccount && (
              <button className="btn btn-primary btn-sm" onClick={() => onAddPayment(selectedAccount.id)}>
                Get started
              </button>
            )}
          </div>
        )}

        {/* Current balance summary */}
        <div style={{
          background: '#fff', border: '1px solid #dddfe2', borderRadius: 8,
          padding: '16px 20px', marginBottom: 12,
        }}>
          <div style={{ fontSize: 14, fontWeight: 600, color: '#1c1e21', marginBottom: 6 }}>
            Current balance
          </div>
          <div style={{ fontSize: 28, fontWeight: 700, color: '#1c1e21' }}>
            {fmtMoney(totalSpent)}
          </div>
          <div style={{ fontSize: 12, color: '#8a8d91', marginTop: 2 }}>
            total spent across all accounts
          </div>
        </div>

        {/* Ad accounts table — same as Meta's accounts list */}
        <div style={{
          background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden',
        }}>
          {/* Table header tabs */}
          <div style={{
            display: 'flex', borderBottom: '1px solid #dddfe2',
            padding: '0 16px',
          }}>
            <div style={{
              fontSize: 13, fontWeight: 700, color: '#1877f2',
              padding: '10px 4px', borderBottom: '2px solid #1877f2',
            }}>
              Ad accounts
            </div>
          </div>
          <table className="meta-table">
            <thead>
              <tr>
                <th>Account</th>
                <th>Status</th>
                <th>How you'll pay</th>
                <th>Total spent</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {accounts.map(a => (
                <tr key={a.id}
                  onClick={() => onSelectAccount(a)}
                  style={{ cursor: 'pointer', background: selectedAccount?.id === a.id ? '#f0f2f5' : undefined }}
                >
                  <td>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <div style={{
                        width: 32, height: 32, borderRadius: 6,
                        background: '#e7f3ff', display: 'flex', alignItems: 'center',
                        justifyContent: 'center', flexShrink: 0,
                      }}>
                        <svg viewBox="0 0 20 20" fill="#1877f2" width="14" height="14">
                          <path d="M4 4a2 2 0 00-2 2v1h16V6a2 2 0 00-2-2H4z"/>
                          <path fillRule="evenodd" d="M18 9H2v5a2 2 0 002 2h12a2 2 0 002-2V9z" clipRule="evenodd"/>
                        </svg>
                      </div>
                      <div>
                        <div style={{ fontWeight: 600, color: '#1877f2', fontSize: 14 }}>{a.name}</div>
                        <div style={{ fontSize: 11, color: '#8a8d91' }}>ID: {a.id.slice(0, 12)}…</div>
                      </div>
                    </div>
                  </td>
                  <td>
                    <span className={`badge badge-${a.status}`}>
                      {a.status === 'active' ? 'Active' : 'Disabled'}
                    </span>
                  </td>
                  <td>
                    {a.has_payment_method
                      ? <span style={{ fontSize: 13, color: '#606770' }}>Stitch debit mandate</span>
                      : <span style={{ fontSize: 13, color: '#8a8d91' }}>No payment method</span>
                    }
                  </td>
                  <td style={{ fontWeight: 600 }}>
                    {fmtMoney(a.total_spent, a.currency)}
                  </td>
                  <td>
                    <button
                      className="btn btn-sm"
                      onClick={e => { e.stopPropagation(); onSelectAccount(a); }}
                    >
                      View details
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Selected account detail panel — right column */}
      {selectedAccount && (
        <div style={{ width: 300, flexShrink: 0 }}>

          {/* Back + account name */}
          <div style={{
            display: 'flex', alignItems: 'center', gap: 8,
            marginBottom: 16,
          }}>
            <button className="btn-icon btn-sm" onClick={() => onSelectAccount(accounts[0])}>
              ←
            </button>
            <div style={{ fontSize: 18, fontWeight: 700 }}>{selectedAccount.name}</div>
          </div>

          {/* Payment methods card */}
          <div style={{
            background: '#fff', border: '1px solid #dddfe2', borderRadius: 8,
            overflow: 'hidden', marginBottom: 12,
          }}>
            <div style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '12px 16px', borderBottom: '1px solid #ebedf0',
            }}>
              <span style={{ fontSize: 14, fontWeight: 600 }}>Payment methods</span>
              {!selectedAccount.has_payment_method && (
                <button className="btn btn-sm" onClick={() => onAddPayment(selectedAccount.id)}>
                  Add payment method
                </button>
              )}
            </div>
            <div style={{ padding: '14px 16px' }}>
              {selectedAccount.has_payment_method ? (
                <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                  <div style={{
                    width: 40, height: 28, background: '#1877f2', borderRadius: 4,
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                  }}>
                    <svg viewBox="0 0 20 20" fill="white" width="14" height="14">
                      <path d="M4 4a2 2 0 00-2 2v1h16V6a2 2 0 00-2-2H4z"/>
                      <path fillRule="evenodd" d="M18 9H2v5a2 2 0 002 2h12a2 2 0 002-2V9z" clipRule="evenodd"/>
                    </svg>
                  </div>
                  <div>
                    <div style={{ fontSize: 13, fontWeight: 600 }}>Stitch debit mandate</div>
                    {selectedAccount.ad_payment_email && (
                      <div style={{ fontSize: 12, color: '#606770' }}>{selectedAccount.ad_payment_email}</div>
                    )}
                  </div>
                </div>
              ) : (
                <div style={{ fontSize: 13, color: '#8a8d91' }}>
                  You haven't added any payment methods.
                </div>
              )}
            </div>
          </div>

          {/* Campaign activity */}
          <div style={{
            background: '#fff', border: '1px solid #dddfe2', borderRadius: 8,
            overflow: 'hidden', marginBottom: 12,
          }}>
            <div style={{ padding: '12px 16px', borderBottom: '1px solid #ebedf0' }}>
              <span style={{ fontSize: 14, fontWeight: 600 }}>Campaign activity</span>
            </div>
            <div style={{ padding: '14px 16px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                <span style={{ fontSize: 13, color: '#606770' }}>Active campaigns</span>
                <span style={{ fontSize: 13, fontWeight: 600 }}>{selectedAccount.active_campaigns}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ fontSize: 13, color: '#606770' }}>Total spent</span>
                <span style={{ fontSize: 13, fontWeight: 600 }}>{fmtMoney(selectedAccount.total_spent, selectedAccount.currency)}</span>
              </div>
            </div>
          </div>

          {/* Business info */}
          <div style={{
            background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden',
          }}>
            <div style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '12px 16px', borderBottom: '1px solid #ebedf0',
            }}>
              <span style={{ fontSize: 14, fontWeight: 600 }}>Account info</span>
            </div>
            <div style={{ padding: '14px 16px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                <span style={{ fontSize: 13, color: '#606770' }}>Created</span>
                <span style={{ fontSize: 13 }}>{fmtDate(selectedAccount.created_at)}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ fontSize: 13, color: '#606770' }}>Currency</span>
                <span style={{ fontSize: 13 }}>{selectedAccount.currency}</span>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ── Payment Activity sub-page ─────────────────────────────────────────────────
function PaymentActivitySubPage({ records }: { records: BillingRecord[] }) {
  if (records.length === 0) {
    return (
      <div>
        <div style={{
          background: '#fff', border: '1px solid #dddfe2', borderRadius: 8,
          padding: '14px 16px', marginBottom: 16,
        }}>
          <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 4 }}>Payment activity</div>
          <div style={{ fontSize: 13, color: '#606770' }}>
            You have no recent spending.{' '}
            <a href="/campaigns/create" style={{ color: '#1877f2' }}>Create a campaign</a>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div style={{ fontSize: 16, fontWeight: 700, marginBottom: 16 }}>Payment activity</div>
      <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden' }}>
        <table className="meta-table">
          <thead>
            <tr>
              <th>Date</th>
              <th>Amount</th>
              <th>Status</th>
              <th>Completed</th>
            </tr>
          </thead>
          <tbody>
            {records.map(r => (
              <tr key={r.id}>
                <td>{fmtDate(r.billing_date)}</td>
                <td style={{ fontWeight: 600 }}>{fmtMoney(r.amount)}</td>
                <td>
                  <span className={`badge badge-${
                    r.status === 'completed' ? 'active'
                    : r.status === 'failed' ? 'cancelled'
                    : 'pending'
                  }`}>
                    {r.status.charAt(0).toUpperCase() + r.status.slice(1)}
                  </span>
                </td>
                <td style={{ color: '#606770' }}>
                  {r.completed_at ? fmtDateTime(r.completed_at) : '—'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ── Register Payment Method Modal ─────────────────────────────────────────────
function RegisterPaymentModal({ accountId, onClose, onSaved }: {
  accountId: string; onClose: () => void; onSaved: () => void;
}) {
  const [mandateId, setMandateId] = useState('');
  const [email, setEmail] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const toast = useToast();

  async function handleSave() {
    if (!mandateId.trim() || !email.trim()) { setError('All fields are required'); return; }
    setSubmitting(true); setError('');
    try {
      await api.post(`/organiser/ad-accounts/${accountId}/payment-method`, {
        stitch_mandate_id: mandateId.trim(),
        email: email.trim(),
      });
      onSaved();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      setError(msg ?? 'Failed to save');
    } finally { setSubmitting(false); }
  }

  return (
    <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="modal modal-sm">
        <div className="modal-header">
          <div className="modal-title">Add payment method</div>
          <button className="btn-icon" onClick={onClose}>✕</button>
        </div>
        <div className="modal-body">
          <div className="alert alert-info">
            Enter your Stitch debit mandate ID. You must complete the mandate authorisation in Stitch before adding it here.
          </div>
          <div className="field">
            <label>Stitch mandate ID</label>
            <input value={mandateId} onChange={e => setMandateId(e.target.value)} placeholder="man_abc123…" autoFocus />
          </div>
          <div className="field" style={{ marginBottom: 0 }}>
            <label>Billing email</label>
            <input type="email" value={email} onChange={e => setEmail(e.target.value)} placeholder="billing@example.com" />
          </div>
          {error && <div style={{ color: '#c41c1c', fontSize: 13, marginTop: 8 }}>{error}</div>}
        </div>
        <div className="modal-footer">
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" onClick={handleSave} disabled={submitting}>
            {submitting ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  );
}