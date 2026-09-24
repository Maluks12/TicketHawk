'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Account controls — MASTER ADMIN ONLY
//
// Disable an account for a number of days, ban it, or block single features:
// buying tickets, creating and publishing events, changing bank details, and
// payouts (migration 000094).
//
// The server is the authority on everything here: who may see the screen
// (is_master_admin), who may be restricted (can_restrict), the limits (echoed
// as `limits`), and whether each restriction is still in force (`state` and
// `days_remaining`, worked out by the database clock). Nothing below decides
// policy; it draws what it is told.
//
// Enforcement does not live in the console. A disable or ban signs the account
// out everywhere the moment it is saved, and every app refuses it from its next
// request.
// ─────────────────────────────────────────────────────────────────────────────
import { useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { fmtDateTime } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';
import { useStaff, AccessDenied } from '../../../lib/permissions';

// ── Types (mirror repository.RestrictableAccount / RestrictionRecord) ───────
type Kind = 'disable' | 'ban' | 'buy_tickets' | 'create_events' | 'bank_details' | 'payouts';
type Feature = Exclude<Kind, 'disable' | 'ban'>;
type AccountState = 'active' | 'limited' | 'disabled' | 'banned';
type Action = 'block' | 'disable' | 'ban';

interface Restriction {
  id: string;
  user_id: string;
  kind: Kind;
  label: string;
  reason: string;
  created_at: string;
  created_by_email: string | null;
  ends_at: string | null;
  lifted_at: string | null;
  lifted_by_email: string | null;
  lift_reason: string | null;
  state: 'active' | 'expired' | 'lifted';
  days_remaining: number | null;
}

interface Account {
  user_id: string;
  display_name: string;
  email: string;
  role: string;
  staff_role: string | null;
  is_master_admin: boolean;
  business_name: string | null;
  created_at: string | null;
  state: AccountState;
  active: Restriction[];
}

interface Limits {
  max_days: number;
  min_reason: number;
  max_reason: number;
  features: { key: Feature; label: string }[];
}

interface Detail {
  account: Account;
  history: Restriction[];
  can_restrict: boolean;
  cannot_reason: string;
  limits: Limits;
}

type ToastFn = (msg: string, type?: 'success' | 'error' | 'info') => void;
type ConfirmFn = (title: string, msg: string, cb: () => Promise<void>) => void;

// ── Copy ─────────────────────────────────────────────────────────────────────
// What each block does, shown beside its checkbox so the choice is informed.
const FEATURE_HELP: Record<Feature, string> = {
  buy_tickets: 'No ticket purchases in the app or on the web, including guest checkout with this email address.',
  create_events: 'No new events and no publishing of drafts. Events that are already live stay on sale.',
  bank_details: 'They cannot change the bank account their payouts go to.',
  payouts: 'Every payout waits. New sales settle to TicketHawk instead of their Paystack split until the hold ends.',
};

// What lifting a block gives back.
const LIFT_COPY: Record<Feature, (name: string) => string> = {
  buy_tickets: (n) => `${n} can buy tickets again straight away.`,
  create_events: (n) => `${n} can create and publish events again straight away.`,
  bank_details: (n) => `${n} can change their bank details again straight away.`,
  payouts: (n) => `Payouts to ${n} resume on the next payout run.`,
};

const ACTIONS: { key: Action; label: string }[] = [
  { key: 'block', label: 'Block features' },
  { key: 'disable', label: 'Disable account' },
  { key: 'ban', label: 'Ban account' },
];

const STATE_BADGE: Record<AccountState, { cls: string; label: string }> = {
  active: { cls: 'badge-active', label: 'Active' },
  limited: { cls: 'badge-pending', label: 'Limited' },
  disabled: { cls: 'badge-cancelled', label: 'Disabled' },
  banned: { cls: 'badge-cancelled', label: 'Banned' },
};

const apiError = (e: unknown, fallback: string) =>
  (e as { response?: { data?: { error?: string } } })?.response?.data?.error || fallback;

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

const isLockout = (k: Kind) => k === 'disable' || k === 'ban';

function nameOf(a: Account): string {
  return a.display_name || a.business_name || a.email;
}

function untilText(r: Restriction): string {
  if (!r.ends_at) return 'Until lifted';
  const left = r.days_remaining != null ? ` (${plural(r.days_remaining, 'day', 'days')} left)` : '';
  return `Until ${fmtDateTime(r.ends_at)}${left}`;
}

// One line under each account in the picker.
function summary(a: Account): string {
  const ban = a.active.find((r) => r.kind === 'ban');
  if (ban) return 'Until lifted';
  const disable = a.active.find((r) => r.kind === 'disable');
  if (disable) {
    return disable.days_remaining != null ? `${plural(disable.days_remaining, 'day', 'days')} left` : 'Disabled';
  }
  const blocks = a.active.filter((r) => !isLockout(r.kind)).length;
  if (blocks > 0) return `${plural(blocks, 'feature', 'features')} blocked`;
  if (a.business_name) return 'Organiser';
  return a.role === 'admin' ? 'Admin' : 'Attendee';
}

const sectionTitle: React.CSSProperties = { fontSize: 13.5, fontWeight: 800, color: 'var(--text)', marginBottom: 8 };
const groupLabel: React.CSSProperties = {
  fontSize: 11, fontWeight: 700, letterSpacing: '.07em', textTransform: 'uppercase',
  color: 'var(--text-3)', marginBottom: 7,
};

// ── Page ─────────────────────────────────────────────────────────────────────
export default function AccountControlsPage() {
  const toast = useToast();
  const confirm = useConfirm();

  const capsLoaded = useStaff((s) => s.loaded);
  const isMaster = useStaff((s) => s.isMasterAdmin);
  const loadCaps = useStaff((s) => s.load);
  useEffect(() => { loadCaps(); }, [loadCaps]);

  const [query, setQuery] = useState('');
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);
  const [listKey, setListKey] = useState(0);

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [detailKey, setDetailKey] = useState(0);

  const searching = query.trim().length > 0;

  // Restricted accounts when the search is empty, matches otherwise. Debounced,
  // and state is only set in the request's callbacks, never synchronously in
  // the effect body (react-hooks/set-state-in-effect).
  useEffect(() => {
    if (!capsLoaded || !isMaster) return;
    let cancelled = false;
    const q = query.trim();
    const t = setTimeout(() => {
      api.get('/admin/restrictions/users', { params: { q } })
        .then(({ data }) => {
          if (cancelled) return;
          setAccounts((data?.accounts ?? []) as Account[]);
          setListError(null);
        })
        .catch((e) => { if (!cancelled) setListError(apiError(e, 'Could not load accounts.')); })
        .finally(() => { if (!cancelled) setListLoading(false); });
    }, q ? 300 : 0);
    return () => { cancelled = true; clearTimeout(t); };
  }, [capsLoaded, isMaster, query, listKey]);

  useEffect(() => {
    if (!selectedId) return;
    let cancelled = false;
    api.get(`/admin/restrictions/users/${selectedId}`)
      .then(({ data }) => {
        if (cancelled) return;
        setDetail(data as Detail);
        setDetailError(null);
      })
      .catch((e) => { if (!cancelled) setDetailError(apiError(e, 'Could not load this account.')); })
      .finally(() => { if (!cancelled) setDetailLoading(false); });
    return () => { cancelled = true; };
  }, [selectedId, detailKey]);

  function onSearch(value: string) {
    setQuery(value);
    setListLoading(true);
  }

  function select(id: string) {
    if (id === selectedId) return;
    setSelectedId(id);
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
  }

  // The server answers every change with the fresh detail. The list re-queries
  // so an account that has just been restricted, or fully cleared, moves.
  function onChanged(next: Detail) {
    setDetail(next);
    setListKey((k) => k + 1);
  }

  if (!capsLoaded) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }
  // Presentational gate. The API refuses a non-master caller regardless.
  if (!isMaster) return <AccessDenied />;

  return (
    <div style={{ padding: '24px 28px 48px', maxWidth: 1400, margin: '0 auto' }}>
      <div style={{ marginBottom: 18 }}>
        <div className="page-title">Account controls</div>
        <div className="page-sub">
          Disable an account for a set number of days, ban it, or block buying tickets, creating
          events, changing bank details or payouts. Disabled and banned accounts are signed out
          everywhere as soon as you save.
        </div>
      </div>

      <div
        className="controls-grid"
        style={{ display: 'grid', gridTemplateColumns: 'minmax(280px, 400px) minmax(0, 1fr)', gap: 18, alignItems: 'start' }}
      >
        <style>{`@media (max-width: 900px) { .controls-grid { grid-template-columns: 1fr !important; } }`}</style>

        {/* ── Account picker ───────────────────────────────────────────── */}
        <div className="card" style={{ overflow: 'hidden' }}>
          <div className="card-header" style={{ flexDirection: 'column', alignItems: 'stretch', gap: 8 }}>
            <input
              className="search-input"
              style={{ minWidth: 0, width: '100%' }}
              placeholder="Search by name, email or business"
              value={query}
              onChange={(e) => onSearch(e.target.value)}
              aria-label="Search accounts"
            />
            <div style={{ fontSize: 12, color: 'var(--text-3)' }}>
              {searching ? 'Search results' : 'Accounts with a restriction in force'}
            </div>
          </div>

          <div style={{ maxHeight: 'calc(100dvh - 260px)', overflowY: 'auto' }}>
            {listLoading && accounts.length === 0 && !listError && (
              <div className="spinner" style={{ margin: '28px auto' }} />
            )}
            {listError && (
              <div className="alert alert-danger" style={{ margin: 12, justifyContent: 'space-between', alignItems: 'center' }}>
                <span>{listError}</span>
                <button className="btn btn-sm" onClick={() => { setListLoading(true); setListKey((k) => k + 1); }}>
                  Try again
                </button>
              </div>
            )}
            {!listLoading && !listError && accounts.length === 0 && (
              <div style={{ padding: 24, textAlign: 'center', fontSize: 13, color: 'var(--text-2)', lineHeight: 1.6 }}>
                {searching
                  ? 'No account matches that search.'
                  : 'No account is restricted right now. Search above to find one.'}
              </div>
            )}
            {accounts.map((a) => {
              const active = a.user_id === selectedId;
              const badge = STATE_BADGE[a.state];
              return (
                <button
                  key={a.user_id}
                  onClick={() => select(a.user_id)}
                  aria-pressed={active}
                  style={{
                    display: 'flex', width: '100%', textAlign: 'left', gap: 12,
                    alignItems: 'center', justifyContent: 'space-between',
                    padding: '12px 16px', border: 'none', cursor: 'pointer',
                    borderBottom: '1px solid var(--border)', fontFamily: 'inherit',
                    background: active ? 'var(--accent-soft)' : 'transparent',
                    color: 'var(--text)',
                  }}
                >
                  <span style={{ minWidth: 0 }}>
                    <span style={{ display: 'block', fontWeight: 600, fontSize: 13.5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {nameOf(a)}
                    </span>
                    <span style={{ display: 'block', fontSize: 12, color: 'var(--text-2)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {a.email}
                    </span>
                  </span>
                  <span style={{ textAlign: 'right', flexShrink: 0 }}>
                    {a.state !== 'active' && <span className={`badge ${badge.cls}`}>{badge.label}</span>}
                    <span style={{ display: 'block', fontSize: 11.5, color: 'var(--text-3)', marginTop: a.state !== 'active' ? 4 : 0 }}>
                      {summary(a)}
                    </span>
                  </span>
                </button>
              );
            })}
          </div>
        </div>

        {/* ── Detail ───────────────────────────────────────────────────── */}
        {!selectedId && (
          <div className="card">
            <div className="card-body" style={{ padding: '48px 24px', textAlign: 'center', color: 'var(--text-2)', fontSize: 13.5 }}>
              Choose an account to see its restrictions or add one.
            </div>
          </div>
        )}
        {selectedId && detailLoading && !detail && (
          <div className="card">
            <div className="card-body"><div className="spinner spinner-lg" style={{ margin: '40px auto' }} /></div>
          </div>
        )}
        {selectedId && detailError && !detail && (
          <div className="alert alert-danger" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
            <span>{detailError}</span>
            <button className="btn btn-sm" onClick={() => { setDetailLoading(true); setDetailKey((k) => k + 1); }}>
              Try again
            </button>
          </div>
        )}
        {selectedId && detail && (
          <RestrictionPanel
            key={detail.account.user_id}
            detail={detail}
            onChanged={onChanged}
            toast={toast}
            confirm={confirm}
          />
        )}
      </div>
    </div>
  );
}

// ── Detail panel ─────────────────────────────────────────────────────────────
function RestrictionPanel({
  detail, onChanged, toast, confirm,
}: {
  detail: Detail;
  onChanged: (d: Detail) => void;
  toast: ToastFn;
  confirm: ConfirmFn;
}) {
  const { account, history, can_restrict, cannot_reason, limits } = detail;
  const name = nameOf(account);
  const isOrganiser = account.business_name != null;

  const [action, setAction] = useState<Action>('block');
  const [features, setFeatures] = useState<Feature[]>([]);
  const [indefinite, setIndefinite] = useState(true);
  const [days, setDays] = useState('7');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);

  const lock = account.active.find((r) => r.kind === 'ban') ?? account.active.find((r) => r.kind === 'disable') ?? null;
  const past = history.filter((r) => r.state !== 'active');

  const needsDays = action === 'disable' || (action === 'block' && !indefinite);
  const daysNum = Number(days);
  const daysError = needsDays && (!Number.isInteger(daysNum) || daysNum < 1 || daysNum > limits.max_days)
    ? `Enter a whole number of days from 1 to ${limits.max_days}.`
    : null;
  const reasonLen = Array.from(reason.trim()).length;
  const reasonError = reasonLen < limits.min_reason
    ? `Write at least ${limits.min_reason} characters.`
    : reasonLen > limits.max_reason
      ? `Keep it to ${limits.max_reason} characters.`
      : null;
  const featuresError = action === 'block' && features.length === 0;
  const canSubmit = can_restrict && !daysError && !reasonError && !featuresError && !busy;

  function toggleFeature(f: Feature) {
    setFeatures((prev) => (prev.includes(f) ? prev.filter((x) => x !== f) : [...prev, f]));
  }

  function submit() {
    const n = needsDays ? daysNum : null;
    const labels = limits.features.filter((f) => features.includes(f.key)).map((f) => f.label);
    const replacing = lock && action !== 'block'
      ? ` This replaces the current ${lock.kind === 'ban' ? 'ban' : 'disable'}.`
      : '';
    const money = isOrganiser
      ? ` Their payouts are held ${action === 'ban' ? 'until the ban is lifted' : 'until it ends'}. Their live events stay on sale.`
      : '';

    let title: string;
    let msg: string;
    if (action === 'disable') {
      title = 'Disable this account?';
      msg = `${name} will be signed out everywhere and won't be able to sign in for ${plural(n ?? 0, 'day', 'days')}.${money}${replacing}`;
    } else if (action === 'ban') {
      title = 'Ban this account?';
      msg = `${name} will be signed out everywhere and won't be able to sign in until you lift the ban.${money}${replacing}`;
    } else {
      title = labels.length === 1 ? 'Block this feature?' : 'Block these features?';
      msg = `${labels.join(', ')} will be blocked for ${name} ${n ? `for ${plural(n, 'day', 'days')}` : 'until you lift it'}. They can still sign in and use the rest of their account.`;
    }

    confirm(title, msg, async () => {
      setBusy(true);
      try {
        const { data } = await api.post(`/admin/restrictions/users/${account.user_id}`, {
          action,
          days: n,
          features: action === 'block' ? features : [],
          reason: reason.trim(),
        });
        if (data?.account) onChanged(data as Detail);
        toast(
          action === 'disable' ? 'Account disabled'
            : action === 'ban' ? 'Account banned'
              : labels.length === 1 ? 'Feature blocked' : 'Features blocked',
          'success',
        );
        setReason('');
        setFeatures([]);
      } catch (e) {
        toast(apiError(e, 'Could not apply the restriction.'), 'error');
      } finally {
        setBusy(false);
      }
    });
  }

  function lift(r: Restriction) {
    const what = r.kind === 'ban' ? 'ban' : r.kind === 'disable' ? 'disable' : 'block';
    const msg = isLockout(r.kind)
      ? `${name} can sign in again straight away. Sessions that were ended stay ended, so they sign in fresh.`
      : LIFT_COPY[r.kind as Feature](name);
    confirm(`Lift this ${what}?`, msg, async () => {
      try {
        const { data } = await api.post(`/admin/restrictions/lift/${r.id}`, {});
        if (data?.account) onChanged(data as Detail);
        toast('Restriction lifted', 'success');
      } catch (e) {
        toast(apiError(e, 'Could not lift the restriction.'), 'error');
      }
    });
  }

  const badge = STATE_BADGE[account.state];
  const submitLabel = action === 'disable' ? 'Disable account'
    : action === 'ban' ? 'Ban account'
      : features.length === 1 ? 'Block feature' : 'Block features';

  return (
    <div className="card">
      <div className="card-header">
        <div style={{ minWidth: 0 }}>
          <div className="card-title">{name}</div>
          <div style={{ fontSize: 12, color: 'var(--text-2)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis' }}>
            {account.email}
            {account.business_name && account.business_name !== name ? `, ${account.business_name}` : ''}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
          {isOrganiser && <span className="badge badge-organizer">Organiser</span>}
          {account.staff_role && <span className="badge badge-admin">Staff</span>}
          {account.is_master_admin && <span className="badge badge-admin">Master admin</span>}
          <span className={`badge ${badge.cls}`}>{badge.label}</span>
        </div>
      </div>

      <div className="card-body">
        {!can_restrict && (
          <div className="alert alert-info"><span>{cannot_reason}</span></div>
        )}

        {/* ── In force ── */}
        <div style={{ marginBottom: 22 }}>
          <div style={sectionTitle}>In force</div>
          {account.active.length === 0 ? (
            <div style={{ fontSize: 13, color: 'var(--text-2)' }}>No restrictions in force.</div>
          ) : (
            account.active.map((r) => <RestrictionRow key={r.id} r={r} onLift={() => lift(r)} />)
          )}
        </div>

        {/* ── Add ── */}
        {can_restrict && (
          <div style={{ marginBottom: 22 }}>
            <div style={sectionTitle}>Add a restriction</div>

            <div role="group" aria-label="Type of restriction" style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 16 }}>
              {ACTIONS.map((a) => (
                <button
                  key={a.key}
                  type="button"
                  className={`pill${action === a.key ? ' active' : ''}`}
                  aria-pressed={action === a.key}
                  onClick={() => setAction(a.key)}
                >
                  {a.label}
                </button>
              ))}
            </div>

            {action === 'block' && (
              <>
                <div style={{ marginBottom: 16 }}>
                  <div style={groupLabel}>Features</div>
                  <div style={{ display: 'grid', gap: 12 }}>
                    {limits.features.map((f) => {
                      const current = account.active.some((r) => r.kind === f.key);
                      return (
                        <label key={f.key} className="check-row" style={{ alignItems: 'flex-start' }}>
                          <input
                            type="checkbox"
                            checked={features.includes(f.key)}
                            onChange={() => toggleFeature(f.key)}
                            style={{ marginTop: 2, flexShrink: 0 }}
                          />
                          <span>
                            <span style={{ display: 'block', fontSize: 13.5, fontWeight: 600, color: 'var(--text)' }}>{f.label}</span>
                            <span style={{ display: 'block', fontSize: 12, color: 'var(--text-3)', marginTop: 2, lineHeight: 1.5 }}>
                              {FEATURE_HELP[f.key]}
                              {current ? ' Already blocked: choosing it again replaces that block.' : ''}
                            </span>
                          </span>
                        </label>
                      );
                    })}
                  </div>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <div style={groupLabel}>How long</div>
                  <div role="group" aria-label="How long" style={{ display: 'flex', gap: 6 }}>
                    <button type="button" className={`pill${indefinite ? ' active' : ''}`} aria-pressed={indefinite} onClick={() => setIndefinite(true)}>
                      Until lifted
                    </button>
                    <button type="button" className={`pill${!indefinite ? ' active' : ''}`} aria-pressed={!indefinite} onClick={() => setIndefinite(false)}>
                      A number of days
                    </button>
                  </div>
                </div>
              </>
            )}

            {action !== 'block' && (
              <div className="alert alert-warning" style={{ fontSize: 13 }}>
                <span>
                  {action === 'disable'
                    ? `${name} is signed out everywhere and can't sign in until the period ends. It lifts itself when it does.`
                    : `${name} is signed out everywhere and can't sign in until you lift the ban.`}
                  {isOrganiser ? ' Their payouts are held for the same time. Their live events stay on sale.' : ''}
                </span>
              </div>
            )}

            {needsDays && (
              <div className={`field${daysError ? ' field-error' : ''}`} style={{ maxWidth: 220 }}>
                <label htmlFor="restrict-days">Days</label>
                <input
                  id="restrict-days"
                  type="number"
                  inputMode="numeric"
                  min={1}
                  max={limits.max_days}
                  step={1}
                  value={days}
                  onChange={(e) => setDays(e.target.value)}
                  style={{ fontVariantNumeric: 'tabular-nums' }}
                />
                {daysError
                  ? <div className="field-error-msg">{daysError}</div>
                  : <div className="field-hint">{`Counted from when you save. Up to ${limits.max_days} days.`}</div>}
              </div>
            )}

            <div className={`field${reason.length > 0 && reasonError ? ' field-error' : ''}`}>
              <label htmlFor="restrict-reason">Reason</label>
              <textarea
                id="restrict-reason"
                value={reason}
                maxLength={limits.max_reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder="e.g. Chargebacks on three events in September, confirmed with Paystack"
              />
              {reason.length > 0 && reasonError
                ? <div className="field-error-msg">{reasonError}</div>
                : <div className="field-hint">For staff only. Kept with the restriction and in the audit log; the account holder never sees it.</div>}
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button
                className={`btn ${action === 'block' ? 'btn-primary' : 'btn-danger'}`}
                disabled={!canSubmit}
                onClick={submit}
              >
                {busy ? 'Saving…' : submitLabel}
              </button>
            </div>
          </div>
        )}

        {/* ── History ── */}
        {past.length > 0 && (
          <div>
            <div style={sectionTitle}>History</div>
            {past.map((r) => <RestrictionRow key={r.id} r={r} />)}
          </div>
        )}
      </div>
    </div>
  );
}

// ── One restriction ──────────────────────────────────────────────────────────
function RestrictionRow({ r, onLift }: { r: Restriction; onLift?: () => void }) {
  let ended = '';
  if (r.state === 'lifted' && r.lifted_at) {
    ended = `Lifted ${fmtDateTime(r.lifted_at)}${r.lifted_by_email ? ` by ${r.lifted_by_email}` : ''}${r.lift_reason ? `: ${r.lift_reason}` : ''}`;
  } else if (r.state === 'expired' && r.ends_at) {
    ended = `Ended ${fmtDateTime(r.ends_at)}`;
  }
  return (
    <div style={{
      display: 'flex', gap: 12, justifyContent: 'space-between', alignItems: 'flex-start',
      padding: '10px 0', borderBottom: '1px solid var(--border)',
    }}>
      <div style={{ minWidth: 0 }}>
        <div style={{ fontSize: 13.5, fontWeight: 700, color: 'var(--text)' }}>
          {r.label}
          {r.state === 'active' && (
            <span style={{ fontWeight: 500, color: 'var(--text-2)' }}>{`, ${untilText(r).charAt(0).toLowerCase()}${untilText(r).slice(1)}`}</span>
          )}
        </div>
        <div style={{ fontSize: 12.5, color: 'var(--text-2)', marginTop: 3, lineHeight: 1.5, overflowWrap: 'anywhere' }}>
          {r.reason}
        </div>
        <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 4 }}>
          {`Set ${fmtDateTime(r.created_at)}${r.created_by_email ? ` by ${r.created_by_email}` : ''}`}
          {ended ? `. ${ended}` : ''}
        </div>
      </div>
      {onLift && (
        <button className="btn btn-sm" onClick={onLift} style={{ flexShrink: 0 }}>Lift</button>
      )}
    </div>
  );
}