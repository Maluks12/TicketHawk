'use client';
import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';
import { fmtMoney, fmtDate, Badge } from '../../../lib/utils';

// ── Types ─────────────────────────────────────────────────────────────────────
type TabId = 'campaigns' | 'ad-accounts';
type CampaignStatus = 'active' | 'paused' | 'exhausted' | 'ended' | 'payment_failed';
type AccountStatus = 'active' | 'disabled';

interface Campaign {
  id: string;
  event_id: string;
  ad_account_id: string | null;
  daily_budget: number;
  total_spent: number;
  today_spent: number;
  total_impressions: number;
  today_impressions: number;
  total_clicks: number;
  today_clicks: number;
  start_date: string;
  end_date: string | null;
  status: CampaignStatus;
  created_at: string;
  event?: { title: string };
}

interface AdAccount {
  id: string;
  name: string;
  status: AccountStatus;
  stitch_mandate_status: string;
  has_payment_method: boolean;
  total_spent: number;
  active_campaigns: number;
  currency: string;
  created_at: string;
}

// ── Page ──────────────────────────────────────────────────────────────────────
export default function CampaignsPage() {
  const router = useRouter();
  const toast = useToast();
  const confirm = useConfirm();

  const [tab, setTab] = useState<TabId>('campaigns');
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [accounts, setAccounts] = useState<AdAccount[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedCampaigns, setSelectedCampaigns] = useState<Set<string>>(new Set());
  const [selectedAccounts, setSelectedAccounts] = useState<Set<string>>(new Set());
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [search, setSearch] = useState('');
  const [showCreateAccountModal, setShowCreateAccountModal] = useState(false);
  const [showPaymentModal, setShowPaymentModal] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [campRes, accRes] = await Promise.all([
        api.get('/organiser/campaigns'),
        api.get('/organiser/ad-accounts'),
      ]);
      setCampaigns(campRes.data.campaigns ?? []);
      setAccounts(accRes.data.accounts ?? []);
    } catch {
      toast('Failed to load data', 'error');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchData(); }, [fetchData]);

  // ── Filtering ───────────────────────────────────────────────────────────────
  const filteredCampaigns = campaigns.filter(c => {
    if (statusFilter !== 'all' && c.status !== statusFilter) return false;
    if (search && !c.event?.title?.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  const filteredAccounts = accounts.filter(a => {
    if (statusFilter !== 'all' && a.status !== statusFilter) return false;
    if (search && !a.name.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  // ── Selection helpers ───────────────────────────────────────────────────────
  function toggleCampaign(id: string) {
    setSelectedCampaigns(prev => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }

  function toggleAccount(id: string) {
    setSelectedAccounts(prev => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }

  function toggleAllCampaigns() {
    if (selectedCampaigns.size === filteredCampaigns.length) {
      setSelectedCampaigns(new Set());
    } else {
      setSelectedCampaigns(new Set(filteredCampaigns.map(c => c.id)));
    }
  }

  // ── Campaign actions ────────────────────────────────────────────────────────
  async function pauseCampaign(id: string) {
    try {
      await api.patch(`/organiser/campaigns/${id}`, { status: 'paused' });
      toast('Campaign paused', 'success');
      fetchData();
    } catch { toast('Failed to pause campaign', 'error'); }
  }

  async function resumeCampaign(id: string) {
    try {
      await api.patch(`/organiser/campaigns/${id}`, { status: 'active' });
      toast('Campaign resumed', 'success');
      fetchData();
    } catch { toast('Failed to resume campaign', 'error'); }
  }

  async function endCampaign(id: string) {
    confirm('End campaign', 'This will permanently end the campaign. This cannot be undone.', async () => {
      await api.delete(`/organiser/campaigns/${id}`);
      toast('Campaign ended', 'success');
      fetchData();
    });
  }

  // ── Account actions ─────────────────────────────────────────────────────────
  async function toggleAccountStatus(account: AdAccount) {
    const newStatus = account.status === 'active' ? 'disabled' : 'active';
    try {
      await api.patch(`/organiser/ad-accounts/${account.id}`, { status: newStatus });
      toast(`Account ${newStatus === 'active' ? 'enabled' : 'disabled'}`, 'success');
      fetchData();
    } catch { toast('Failed to update account', 'error'); }
  }

  // ── Delivery status helpers ─────────────────────────────────────────────────
  function deliveryLabel(status: CampaignStatus) {
    const map: Record<CampaignStatus, string> = {
      active: 'Active',
      paused: 'Paused',
      exhausted: 'Budget reached',
      ended: 'Ended',
      payment_failed: 'Payment failed',
    };
    return map[status] ?? status;
  }

  // ── Render ───────────────────────────────────────────────────────────────────
  return (
    <div className="page">

      {/* ── Page header ──────────────────────────────────────────────────── */}
      <div className="page-header">
        <div>
          <div className="page-title">Campaigns</div>
          <div style={{ fontSize: 13, color: '#606770', marginTop: 2 }}>
            Manage your event ad campaigns and ad accounts
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {tab === 'ad-accounts' && (
            <button className="btn btn-primary" onClick={() => setShowCreateAccountModal(true)}>
              + Add ad account
            </button>
          )}
          {tab === 'campaigns' && (
            <Link href="/campaigns/create" className="btn btn-primary">
              + Create campaign
            </Link>
          )}
        </div>
      </div>

      {/* ── Filter pill strip — mirrors "Actions | Had delivery | See more" ── */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 6,
        padding: '8px 16px', borderBottom: '1px solid #dddfe2',
        background: '#fff', flexWrap: 'wrap',
      }}>
        {(['all', 'active', 'paused', 'ended'] as const).map(s => (
          <button
            key={s}
            onClick={() => setStatusFilter(s)}
            style={{
              padding: '5px 12px', borderRadius: 16, fontSize: 13, fontWeight: 600,
              border: `1px solid ${statusFilter === s ? '#1877f2' : '#dddfe2'}`,
              background: statusFilter === s ? '#e7f3ff' : '#fff',
              color: statusFilter === s ? '#1877f2' : '#606770',
              cursor: 'pointer',
            }}
          >
            {s === 'all' ? 'All' : s.charAt(0).toUpperCase() + s.slice(1)}
          </button>
        ))}
      </div>

      {/* ── Tab bar: Campaigns | Ad Accounts ─────────────────────────────── */}
      <div className="tab-bar">
        <button
          className={`tab-item${tab === 'campaigns' ? ' active' : ''}`}
          onClick={() => { setTab('campaigns'); setSelectedCampaigns(new Set()); }}
        >
          Campaigns
          {campaigns.length > 0 && (
            <span style={{
              marginLeft: 6, background: tab === 'campaigns' ? '#1877f2' : '#606770',
              color: '#fff', borderRadius: 99, padding: '1px 6px', fontSize: 11,
            }}>
              {campaigns.length}
            </span>
          )}
        </button>
        <button
          className={`tab-item${tab === 'ad-accounts' ? ' active' : ''}`}
          onClick={() => { setTab('ad-accounts'); setSelectedAccounts(new Set()); }}
        >
          Ad Accounts
          {accounts.length > 0 && (
            <span style={{
              marginLeft: 6, background: tab === 'ad-accounts' ? '#1877f2' : '#606770',
              color: '#fff', borderRadius: 99, padding: '1px 6px', fontSize: 11,
            }}>
              {accounts.length}
            </span>
          )}
        </button>

        {/* Date / search pushed to right */}
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 8, alignItems: 'center', paddingBottom: 2 }}>
          <div className="search-box" style={{ minWidth: 180 }}>
            <svg viewBox="0 0 20 20" fill="#606770" width="14" height="14">
              <path fillRule="evenodd" d="M8 4a4 4 0 100 8 4 4 0 000-8zM2 8a6 6 0 1110.89 3.476l4.817 4.817a1 1 0 01-1.414 1.414l-4.816-4.816A6 6 0 012 8z" clipRule="evenodd"/>
            </svg>
            <input
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder={tab === 'campaigns' ? 'Search campaigns…' : 'Search accounts…'}
            />
          </div>
        </div>
      </div>

      {/* ── Action toolbar — appears when rows are selected ───────────────── */}
      {(tab === 'campaigns' ? selectedCampaigns.size : selectedAccounts.size) > 0 && (
        <div className="toolbar" style={{ background: '#e7f3ff', borderBottom: '1px solid #b8d9ff' }}>
          <span style={{ fontSize: 13, fontWeight: 600, color: '#1877f2' }}>
            {tab === 'campaigns' ? selectedCampaigns.size : selectedAccounts.size} selected
          </span>
          {tab === 'campaigns' && (
            <>
              <button className="btn btn-sm">
                <svg viewBox="0 0 20 20" fill="currentColor" width="14" height="14"><path d="M13.586 3.586a2 2 0 112.828 2.828l-.793.793-2.828-2.828.793-.793zM11.379 5.793L3 14.172V17h2.828l8.38-8.379-2.83-2.828z"/></svg>
                Edit
              </button>
              <button className="btn btn-sm btn-danger" onClick={() => {
                selectedCampaigns.forEach(id => endCampaign(id));
              }}>
                End selected
              </button>
            </>
          )}
          {tab === 'ad-accounts' && (
            <button className="btn btn-sm">Edit</button>
          )}
          <button
            className="btn btn-sm"
            style={{ marginLeft: 'auto' }}
            onClick={() => {
              setSelectedCampaigns(new Set());
              setSelectedAccounts(new Set());
            }}
          >
            Cancel
          </button>
        </div>
      )}

      {/* ── Table area ───────────────────────────────────────────────────── */}
      <div style={{ flex: 1, overflow: 'auto', background: '#fff' }}>
        {loading ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 200 }}>
            <div className="spinner" />
          </div>
        ) : tab === 'campaigns' ? (
          <CampaignTable
            campaigns={filteredCampaigns}
            selected={selectedCampaigns}
            onToggle={toggleCampaign}
            onToggleAll={toggleAllCampaigns}
            onPause={pauseCampaign}
            onResume={resumeCampaign}
            onEnd={endCampaign}
            deliveryLabel={deliveryLabel}
          />
        ) : (
          <AdAccountTable
            accounts={filteredAccounts}
            selected={selectedAccounts}
            onToggle={toggleAccount}
            onToggleStatus={toggleAccountStatus}
            onAddPayment={(id) => setShowPaymentModal(id)}
          />
        )}
      </div>

      {/* ── Modals ───────────────────────────────────────────────────────── */}
      {showCreateAccountModal && (
        <CreateAdAccountModal
          onClose={() => setShowCreateAccountModal(false)}
          onCreated={() => { setShowCreateAccountModal(false); fetchData(); toast('Ad account created', 'success'); }}
        />
      )}

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

// ── Campaign Table ─────────────────────────────────────────────────────────────
function CampaignTable({
  campaigns, selected, onToggle, onToggleAll, onPause, onResume, onEnd, deliveryLabel,
}: {
  campaigns: Campaign[];
  selected: Set<string>;
  onToggle: (id: string) => void;
  onToggleAll: () => void;
  onPause: (id: string) => void;
  onResume: (id: string) => void;
  onEnd: (id: string) => void;
  deliveryLabel: (s: CampaignStatus) => string;
}) {
  if (campaigns.length === 0) {
    return (
      <div className="empty-state">
        <div className="empty-icon">🔍</div>
        <div className="empty-title">No campaigns yet</div>
        <div className="empty-desc">
          Create a campaign to start boosting your events to more people.
        </div>
        <Link href="/campaigns/create" className="btn btn-primary" style={{ marginTop: 8 }}>
          Create your first campaign
        </Link>
      </div>
    );
  }

  const allSelected = selected.size === campaigns.length && campaigns.length > 0;

  return (
    <table className="meta-table">
      <thead>
        <tr>
          <th className="col-check">
            <input type="checkbox" checked={allSelected} onChange={onToggleAll}
              style={{ accentColor: '#1877f2', cursor: 'pointer' }} />
          </th>
          <th className="sortable">Campaign</th>
          <th className="sortable">Delivery</th>
          <th className="sortable">Budget / day</th>
          <th className="sortable">Amount spent</th>
          <th className="sortable">Today's spend</th>
          <th className="sortable">Impressions</th>
          <th className="sortable">Clicks</th>
          <th className="sortable">Start date</th>
          <th className="col-actions">Actions</th>
        </tr>
      </thead>
      <tbody>
        {campaigns.map(c => (
          <tr key={c.id} style={{ background: selected.has(c.id) ? '#e7f3ff' : undefined }}>
            <td>
              <input type="checkbox" checked={selected.has(c.id)} onChange={() => onToggle(c.id)}
                style={{ accentColor: '#1877f2', cursor: 'pointer' }} />
            </td>
            <td>
              <div style={{ fontWeight: 600, color: '#1c1e21', fontSize: 14 }}>
                {c.event?.title ?? 'Unknown Event'}
              </div>
              <div style={{ fontSize: 12, color: '#606770', marginTop: 2 }}>
                ID: {c.id.slice(0, 8)}…
              </div>
            </td>
            <td>
              <span className={`badge badge-${c.status === 'payment_failed' ? 'cancelled' : c.status}`}>
                {deliveryLabel(c.status)}
              </span>
            </td>
            <td style={{ fontWeight: 600 }}>{fmtMoney(c.daily_budget)}</td>
            <td>{fmtMoney(c.total_spent)}</td>
            <td style={{ color: c.today_spent > 0 ? '#1c1e21' : '#8a8d91' }}>
              {fmtMoney(c.today_spent)}
            </td>
            <td>{c.total_impressions.toLocaleString()}</td>
            <td>{c.total_clicks.toLocaleString()}</td>
            <td style={{ color: '#606770' }}>{fmtDate(c.start_date)}</td>
            <td>
              <div style={{ display: 'flex', gap: 4, justifyContent: 'flex-end' }}>
                {c.status === 'active' && (
                  <button className="btn btn-xs" onClick={() => onPause(c.id)}>Pause</button>
                )}
                {c.status === 'paused' && (
                  <button className="btn btn-xs btn-primary" onClick={() => onResume(c.id)}>Resume</button>
                )}
                {c.status !== 'ended' && (
                  <button className="btn btn-xs btn-danger" onClick={() => onEnd(c.id)}>End</button>
                )}
              </div>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// ── Ad Account Table ───────────────────────────────────────────────────────────
function AdAccountTable({
  accounts, selected, onToggle, onToggleStatus, onAddPayment,
}: {
  accounts: AdAccount[];
  selected: Set<string>;
  onToggle: (id: string) => void;
  onToggleStatus: (a: AdAccount) => void;
  onAddPayment: (id: string) => void;
}) {
  if (accounts.length === 0) {
    return (
      <div className="empty-state">
        <div className="empty-icon">💳</div>
        <div className="empty-title">No ad accounts yet</div>
        <div className="empty-desc">
          Ad accounts hold your payment method and campaigns. You can have one per brand or event portfolio.
        </div>
      </div>
    );
  }

  return (
    <table className="meta-table">
      <thead>
        <tr>
          <th className="col-check">
            <input type="checkbox" style={{ accentColor: '#1877f2' }} />
          </th>
          <th>Account</th>
          <th>Status</th>
          <th>Payment method</th>
          <th className="sortable">Active campaigns</th>
          <th className="sortable">Total spent</th>
          <th className="sortable">Created</th>
          <th className="col-actions">Actions</th>
        </tr>
      </thead>
      <tbody>
        {accounts.map(a => (
          <tr key={a.id} style={{ background: selected.has(a.id) ? '#e7f3ff' : undefined }}>
            <td>
              <input type="checkbox" checked={selected.has(a.id)} onChange={() => onToggle(a.id)}
                style={{ accentColor: '#1877f2', cursor: 'pointer' }} />
            </td>
            <td>
              <div style={{ fontWeight: 600, fontSize: 14 }}>{a.name}</div>
              <div style={{ fontSize: 12, color: '#606770', marginTop: 2 }}>ID: {a.id.slice(0, 8)}…</div>
            </td>
            <td>
              <span className={`badge badge-${a.status}`}>
                {a.status === 'active' ? 'Active' : 'Disabled'}
              </span>
            </td>
            <td>
              {a.has_payment_method ? (
                <span style={{ display: 'flex', alignItems: 'center', gap: 6, color: '#1a7f37', fontSize: 13, fontWeight: 600 }}>
                  <svg viewBox="0 0 20 20" fill="currentColor" width="14" height="14">
                    <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd"/>
                  </svg>
                  Debit mandate active
                </span>
              ) : (
                <button className="btn btn-sm" onClick={() => onAddPayment(a.id)}>
                  Add payment method
                </button>
              )}
            </td>
            <td>{a.active_campaigns}</td>
            <td>{fmtMoney(a.total_spent, a.currency)}</td>
            <td style={{ color: '#606770' }}>{fmtDate(a.created_at)}</td>
            <td>
              <div style={{ display: 'flex', gap: 4, justifyContent: 'flex-end' }}>
                <button className="btn btn-xs" onClick={() => onToggleStatus(a)}>
                  {a.status === 'active' ? 'Disable' : 'Enable'}
                </button>
              </div>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// ── Create Ad Account Modal — mirrors "Add an ad account" Meta modal ───────────
function CreateAdAccountModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  async function handleSubmit() {
    if (!name.trim() || name.trim().length < 2) { setError('Name must be at least 2 characters'); return; }
    setSubmitting(true); setError('');
    try {
      await api.post('/organiser/ad-accounts', { name: name.trim() });
      onCreated();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      setError(msg ?? 'Failed to create account');
    } finally { setSubmitting(false); }
  }

  return (
    <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="modal modal-sm">
        <div className="modal-header">
          <div className="modal-title">Add an ad account</div>
          <button className="btn-icon" onClick={onClose}>✕</button>
        </div>
        <div className="modal-body">
          <p style={{ fontSize: 14, color: '#606770', marginBottom: 20 }}>
            Ad accounts can't be removed once created. Each account holds its own payment method and campaigns.
          </p>

          {/* Three option rows — same pattern as Meta's modal */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 20 }}>
            <label style={{
              display: 'flex', alignItems: 'flex-start', gap: 14,
              padding: '14px 16px', border: '2px solid #1877f2',
              borderRadius: 8, cursor: 'pointer', background: '#e7f3ff',
            }}>
              <span style={{ fontSize: 22, marginTop: 2 }}>＋</span>
              <div>
                <div style={{ fontWeight: 700, fontSize: 15, color: '#1c1e21' }}>Create a new ad account</div>
                <div style={{ fontSize: 13, color: '#606770', marginTop: 4 }}>
                  Manage campaigns for your events under a single billing account.
                </div>
              </div>
            </label>
          </div>

          <div className="field">
            <label>Account name</label>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="e.g. Thetsongaboys Main Account"
              onKeyDown={e => e.key === 'Enter' && handleSubmit()}
              autoFocus
            />
            {error && <div className="field-error-msg" style={{ color: '#c41c1c' }}>{error}</div>}
          </div>
        </div>
        <div className="modal-footer">
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" onClick={handleSubmit} disabled={submitting}>
            {submitting ? 'Creating…' : 'Create account'}
          </button>
        </div>
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
          <div className="alert alert-info" style={{ marginBottom: 16 }}>
            Enter your Stitch debit mandate ID. Complete the mandate setup in Stitch first if you haven't already.
          </div>
          <div className="field">
            <label>Stitch mandate ID</label>
            <input value={mandateId} onChange={e => setMandateId(e.target.value)} placeholder="man_abc123…" />
          </div>
          <div className="field">
            <label>Billing email</label>
            <input type="email" value={email} onChange={e => setEmail(e.target.value)} placeholder="billing@example.com" />
          </div>
          {error && <div style={{ color: '#c41c1c', fontSize: 13 }}>{error}</div>}
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