'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Organiser fees — MASTER ADMIN ONLY
//
// Per-organiser overrides of the organiser commission and the buyer service fee
// (migration 000093). An organiser with no override pays the platform standard,
// and follows it automatically if the standard ever changes.
//
// The server is the authority on everything here: who may see the screen
// (is_master_admin), the limits (echoed as `defaults`), and the validation. The
// limits below are read from the response, never hard-coded, so the screen can
// never offer a value the API will refuse.
//
// A change prices bookings made AFTER it. Existing bookings keep the fees they
// were created with, and so do their payouts.
// ─────────────────────────────────────────────────────────────────────────────
import { useEffect, useMemo, useState } from 'react';
import { api } from '../../../lib/api';
import { fmtDateTime } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';
import { useStaff, AccessDenied } from '../../../lib/permissions';

// ── Types ────────────────────────────────────────────────────────────────────
interface FeeDefaults {
  commission_pct: number;
  service_fee: number;
  max_service_fee_rate: number;
  min_commission_pct: number;
  max_commission_pct: number;
  min_service_fee: number;
  max_service_fee: number;
}

interface OrganizerFeeRow {
  organizer_id: string;
  business_name: string;
  business_email: string;
  status: string;
  custom_commission_pct: number | null;
  custom_service_fee: number | null;
  effective_commission_pct: number;
  effective_service_fee: number;
  note: string | null;
  updated_at: string | null;
  updated_by_email: string | null;
}

// Paystack's local-card price as commonly published for SA merchants:
// 2.9% + R1 per successful charge, excl. VAT. Used ONLY for the margin preview
// below, never for money. Confirm against the Paystack dashboard — volume
// pricing, international cards and EFT differ.
const PAYSTACK_PCT = 0.029;
const PAYSTACK_FLAT = 1.0;
const VAT = 0.15;

const round2 = (v: number) => Math.round(v * 100) / 100;
const rand = (v: number) =>
  new Intl.NumberFormat('en-ZA', { style: 'currency', currency: 'ZAR', minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(v);
const pct = (v: number) => `${Number.isInteger(v) ? v : v.toFixed(2).replace(/0$/, '')}%`;
const apiError = (e: unknown, fallback: string) =>
  (e as { response?: { data?: { error?: string } } })?.response?.data?.error || fallback;

// ── Page ─────────────────────────────────────────────────────────────────────
export default function OrganiserFeesPage() {
  const toast = useToast();
  const confirm = useConfirm();

  const capsLoaded = useStaff((s) => s.loaded);
  const isMaster = useStaff((s) => s.isMasterAdmin);
  const loadCaps = useStaff((s) => s.load);
  useEffect(() => { loadCaps(); }, [loadCaps]);

  const [defaults, setDefaults] = useState<FeeDefaults | null>(null);
  const [rows, setRows] = useState<OrganizerFeeRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [onlyCustom, setOnlyCustom] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // Bumped by "Try again". State is only set in the request's callbacks, never
  // synchronously in the effect body (react-hooks/set-state-in-effect).
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    if (!capsLoaded || !isMaster) return;
    let cancelled = false;
    api.get('/admin/fees/organizers')
      .then(({ data }) => {
        if (cancelled) return;
        setDefaults(data.defaults);
        setRows(data.organizers || []);
        setLoadError(null);
      })
      .catch((e) => { if (!cancelled) setLoadError(apiError(e, 'Could not load organiser fees.')); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [capsLoaded, isMaster, reloadKey]);

  function retry() {
    setLoading(true);
    setReloadKey((k) => k + 1);
  }

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((r) => {
      if (onlyCustom && r.custom_commission_pct == null && r.custom_service_fee == null) return false;
      if (!q) return true;
      return r.business_name.toLowerCase().includes(q) || r.business_email.toLowerCase().includes(q);
    });
  }, [rows, search, onlyCustom]);

  const customCount = rows.filter((r) => r.custom_commission_pct != null || r.custom_service_fee != null).length;
  const selected = rows.find((r) => r.organizer_id === selectedId) ?? null;

  if (!capsLoaded) {
    return <div className="page-loader"><div className="spinner spinner-lg" /></div>;
  }
  // Presentational gate. The API refuses a non-master caller regardless.
  if (!isMaster) return <AccessDenied />;

  return (
    <div style={{ padding: '24px 28px 48px', maxWidth: 1400, margin: '0 auto' }}>
      <div style={{ marginBottom: 18 }}>
        <div className="page-title">Organiser fees</div>
        <div className="page-sub">
          {defaults
            ? <>Standard terms: {pct(defaults.commission_pct)} organiser commission and {rand(defaults.service_fee)} service fee per ticket. </>
            : null}
          Changes apply to new bookings only. Existing bookings and their payouts keep the fees they were sold with.
        </div>
      </div>

      {loading && <div className="spinner spinner-lg" style={{ margin: '48px auto' }} />}

      {!loading && loadError && (
        <div className="alert alert-danger" style={{ padding: '12px 14px', borderRadius: 'var(--r)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
          <span>{loadError}</span>
          <button className="btn btn-sm" onClick={retry}>Try again</button>
        </div>
      )}

      {!loading && !loadError && defaults && (
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(280px, 400px) minmax(0, 1fr)', gap: 18, alignItems: 'start' }}
             className="fees-grid">
          <style>{`@media (max-width: 900px) { .fees-grid { grid-template-columns: 1fr !important; } }`}</style>

          {/* ── Organiser picker ───────────────────────────────────────── */}
          <div className="card" style={{ overflow: 'hidden' }}>
            <div className="card-header" style={{ flexDirection: 'column', alignItems: 'stretch', gap: 10 }}>
              <input
                className="search-input"
                style={{ minWidth: 0, width: '100%' }}
                placeholder="Search by business name or email"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                aria-label="Search organisers"
              />
              <div style={{ display: 'flex', gap: 6 }}>
                <button className={`pill${!onlyCustom ? ' active' : ''}`} onClick={() => setOnlyCustom(false)}>
                  All ({rows.length})
                </button>
                <button className={`pill${onlyCustom ? ' active' : ''}`} onClick={() => setOnlyCustom(true)}>
                  Custom terms ({customCount})
                </button>
              </div>
            </div>

            <div style={{ maxHeight: 'calc(100dvh - 260px)', overflowY: 'auto' }}>
              {filtered.length === 0 && (
                <div style={{ padding: 24, textAlign: 'center', fontSize: 13, color: 'var(--text-2)' }}>
                  {onlyCustom ? 'No organiser has custom terms yet.' : 'No organiser matches that search.'}
                </div>
              )}
              {filtered.map((r) => {
                const active = r.organizer_id === selectedId;
                const custom = r.custom_commission_pct != null || r.custom_service_fee != null;
                return (
                  <button
                    key={r.organizer_id}
                    onClick={() => setSelectedId(r.organizer_id)}
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
                        {r.business_name}
                      </span>
                      <span style={{ display: 'block', fontSize: 12, color: 'var(--text-2)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {r.business_email}
                      </span>
                    </span>
                    <span style={{ textAlign: 'right', flexShrink: 0, fontVariantNumeric: 'tabular-nums' }}>
                      <span style={{ display: 'block', fontSize: 13, fontWeight: 700, color: custom ? 'var(--accent)' : 'var(--text-2)' }}>
                        {pct(r.effective_commission_pct)} · {rand(r.effective_service_fee)}
                      </span>
                      <span style={{ display: 'block', fontSize: 11, color: 'var(--text-3)' }}>
                        {custom ? 'Custom' : 'Standard'}
                      </span>
                    </span>
                  </button>
                );
              })}
            </div>
          </div>

          {/* ── Editor ─────────────────────────────────────────────────── */}
          {selected ? (
            <FeeEditor
              key={selected.organizer_id + (selected.updated_at ?? '')}
              row={selected}
              defaults={defaults}
              onSaved={(updated) => {
                setRows((prev) => prev.map((r) => (r.organizer_id === updated.organizer_id ? updated : r)));
              }}
              toast={toast}
              confirm={confirm}
            />
          ) : (
            <div className="card">
              <div className="card-body" style={{ padding: '48px 24px', textAlign: 'center', color: 'var(--text-2)', fontSize: 13.5 }}>
                Choose an organiser to see or change their commission and service fee.
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ── Editor panel ─────────────────────────────────────────────────────────────
function FeeEditor({
  row, defaults, onSaved, toast, confirm,
}: {
  row: OrganizerFeeRow;
  defaults: FeeDefaults;
  onSaved: (r: OrganizerFeeRow) => void;
  toast: (msg: string, type?: 'success' | 'error' | 'info') => void;
  confirm: (title: string, msg: string, cb: () => Promise<void>) => void;
}) {
  const [useCustomCommission, setUseCustomCommission] = useState(row.custom_commission_pct != null);
  const [useCustomService, setUseCustomService] = useState(row.custom_service_fee != null);
  const [commission, setCommission] = useState(String(row.custom_commission_pct ?? defaults.commission_pct));
  const [serviceFee, setServiceFee] = useState(String(row.custom_service_fee ?? defaults.service_fee));
  const [note, setNote] = useState(row.note ?? '');
  const [samplePrice, setSamplePrice] = useState('150');
  const [saving, setSaving] = useState(false);

  const commissionNum = Number(commission);
  const serviceNum = Number(serviceFee);

  const commissionError = useCustomCommission && (
    commission.trim() === '' || !Number.isFinite(commissionNum) ||
    commissionNum < defaults.min_commission_pct || commissionNum > defaults.max_commission_pct
  ) ? `Enter ${pct(defaults.min_commission_pct)} to ${pct(defaults.max_commission_pct)}.` : null;

  const serviceError = useCustomService && (
    serviceFee.trim() === '' || !Number.isFinite(serviceNum) ||
    serviceNum < defaults.min_service_fee || serviceNum > defaults.max_service_fee
  ) ? `Enter ${rand(defaults.min_service_fee)} to ${rand(defaults.max_service_fee)}.` : null;

  const hasCustom = useCustomCommission || useCustomService;
  const noteError = hasCustom && note.trim().length < 3 ? 'Say what was agreed, so the arrangement can be traced later.' : null;

  const effCommission = useCustomCommission && !commissionError ? round2(commissionNum) : defaults.commission_pct;
  const effService = useCustomService && !serviceError ? round2(serviceNum) : defaults.service_fee;

  const unchanged =
    (useCustomCommission ? round2(commissionNum) : null) === row.custom_commission_pct &&
    (useCustomService ? round2(serviceNum) : null) === row.custom_service_fee &&
    (hasCustom ? note.trim() : '') === (row.note ?? '').trim();

  const canSave = !commissionError && !serviceError && !noteError && !unchanged && !saving;

  // Preview of ONE ticket at the sample price, priced exactly as the backend's
  // CalculateFees does: per-ticket service fee capped at max_service_fee_rate
  // of face value, commission on the subtotal.
  const price = Math.max(0, Number(samplePrice) || 0);
  const svc = price > 0 ? round2(Math.min(effService, defaults.max_service_fee_rate * price)) : 0;
  const comm = round2((effCommission / 100) * price);
  const buyerPays = round2(price + svc);
  const organiserGets = round2(price - comm);
  const platformGross = round2(svc + comm);
  const paystack = price > 0 ? round2((PAYSTACK_PCT * buyerPays + PAYSTACK_FLAT) * (1 + VAT)) : 0;
  const margin = round2(platformGross - paystack);

  function save(resetToStandard = false) {
    const body = resetToStandard
      ? { commission_pct: null, service_fee: null, note: '' }
      : {
          commission_pct: useCustomCommission ? round2(commissionNum) : null,
          service_fee: useCustomService ? round2(serviceNum) : null,
          note: hasCustom ? note.trim() : '',
        };

    const summary = resetToStandard
      ? `${row.business_name} goes back to the standard ${pct(defaults.commission_pct)} commission and ${rand(defaults.service_fee)} service fee.`
      : `${row.business_name} will pay ${pct(body.commission_pct ?? defaults.commission_pct)} commission, and their buyers ${rand(body.service_fee ?? defaults.service_fee)} service fee per ticket.`;

    confirm(
      resetToStandard ? 'Reset to standard terms?' : 'Save new fee terms?',
      `${summary} This applies to bookings made from now on. Existing bookings and payouts are not changed.`,
      async () => {
        setSaving(true);
        try {
          const { data } = await api.put(`/admin/fees/organizers/${row.organizer_id}`, body);
          onSaved(data.organizer as OrganizerFeeRow);
          toast(resetToStandard ? 'Reset to standard terms' : 'Fee terms saved', 'success');
        } catch (e) {
          toast(apiError(e, 'Could not save the fee terms.'), 'error');
        } finally {
          setSaving(false);
        }
      },
    );
  }

  const rowCustom = row.custom_commission_pct != null || row.custom_service_fee != null;

  return (
    <div className="card">
      <div className="card-header">
        <div style={{ minWidth: 0 }}>
          <div className="card-title">{row.business_name}</div>
          <div style={{ fontSize: 12, color: 'var(--text-2)', marginTop: 2 }}>
            {row.business_email} · account {row.status}
          </div>
        </div>
        {rowCustom && (
          <button className="btn btn-sm" disabled={saving} onClick={() => save(true)}>
            Reset to standard
          </button>
        )}
      </div>

      <div className="card-body">
        {row.updated_at && (
          <div style={{ fontSize: 12, color: 'var(--text-3)', marginBottom: 16 }}>
            Last changed {fmtDateTime(row.updated_at)}{row.updated_by_email ? ` by ${row.updated_by_email}` : ''}
          </div>
        )}

        <div className="field-row">
          <FeeField
            label="Organiser commission"
            custom={useCustomCommission}
            onCustomChange={setUseCustomCommission}
            standardText={`Standard ${pct(defaults.commission_pct)}`}
            value={commission}
            onChange={setCommission}
            suffix="%"
            step="0.25"
            min={defaults.min_commission_pct}
            max={defaults.max_commission_pct}
            hint={`Taken from the organiser's payout. ${pct(defaults.min_commission_pct)} to ${pct(defaults.max_commission_pct)}.`}
            error={commissionError}
          />
          <FeeField
            label="Service fee per ticket"
            custom={useCustomService}
            onCustomChange={setUseCustomService}
            standardText={`Standard ${rand(defaults.service_fee)}`}
            value={serviceFee}
            onChange={setServiceFee}
            prefix="R"
            step="0.50"
            min={defaults.min_service_fee}
            max={defaults.max_service_fee}
            hint={`Paid by the buyer, never more than ${Math.round(defaults.max_service_fee_rate * 100)}% of the ticket price. ${rand(defaults.min_service_fee)} to ${rand(defaults.max_service_fee)}.`}
            error={serviceError}
          />
        </div>

        {hasCustom && (
          <div className={`field${noteError && note.length > 0 ? ' field-error' : ''}`}>
            <label htmlFor="fee-note">What was agreed</label>
            <textarea
              id="fee-note"
              value={note}
              maxLength={500}
              onChange={(e) => setNote(e.target.value)}
              placeholder="e.g. Launch deal for their 2026 festival series, agreed by email on 20 Sep"
            />
            {noteError && note.length > 0
              ? <div className="field-error-msg">{noteError}</div>
              : <div className="field-hint">Kept with the organiser and in the audit log.</div>}
          </div>
        )}

        {/* ── Split preview ── */}
        <div style={{
          border: '1px solid var(--border)', borderRadius: 'var(--r-lg)',
          padding: 16, marginBottom: 18, background: 'var(--surface-alpha)',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', marginBottom: 12 }}>
            <div style={{ fontSize: 13.5, fontWeight: 700 }}>One ticket at</div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <span style={{ fontSize: 13, color: 'var(--text-2)' }}>R</span>
              <input
                type="number" min={0} step="10" inputMode="decimal"
                value={samplePrice}
                onChange={(e) => setSamplePrice(e.target.value)}
                aria-label="Sample ticket price in rand"
                className="search-input"
                style={{ minWidth: 0, width: 110, textAlign: 'right' }}
              />
            </div>
          </div>

          <SplitLine label="Buyer pays" value={rand(buyerPays)} sub={`ticket ${rand(price)} + service fee ${rand(svc)}`} />
          <SplitLine label="Organiser receives" value={rand(organiserGets)} sub={`after ${pct(effCommission)} commission (${rand(comm)})`} />
          <SplitLine label="TicketHawk earns" value={rand(platformGross)} sub="service fee + commission" />
          <SplitLine label="Paystack cost (estimate)" value={`−${rand(paystack)}`} sub="2.9% + R1 + VAT on what the buyer pays" />
          <div style={{ borderTop: '1px solid var(--border)', marginTop: 8, paddingTop: 8 }}>
            <SplitLine
              label="Margin"
              value={rand(margin)}
              strong
              tone={margin < 0 ? 'danger' : margin < 1 ? 'warning' : 'success'}
            />
          </div>
          {price > 0 && margin < 0 && (
            <div className="alert alert-danger" style={{ marginTop: 10, padding: '9px 12px', borderRadius: 'var(--r)', fontSize: 12.5 }}>
              At this price TicketHawk loses money on every ticket with these terms.
            </div>
          )}
          <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 10, lineHeight: 1.5 }}>
            The Paystack figure is the published local-card rate. Check your Paystack dashboard for your actual pricing; international cards cost more.
          </div>
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button className="btn btn-primary" disabled={!canSave} onClick={() => save(false)}>
            {saving ? 'Saving…' : 'Save fee terms'}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Small pieces ─────────────────────────────────────────────────────────────
function FeeField({
  label, custom, onCustomChange, standardText, value, onChange,
  prefix, suffix, step, min, max, hint, error,
}: {
  label: string;
  custom: boolean;
  onCustomChange: (v: boolean) => void;
  standardText: string;
  value: string;
  onChange: (v: string) => void;
  prefix?: string;
  suffix?: string;
  step: string;
  min: number;
  max: number;
  hint: string;
  error: string | null;
}) {
  const id = label.toLowerCase().replace(/\W+/g, '-');
  return (
    <div className={`field${error ? ' field-error' : ''}`}>
      <label htmlFor={id}>{label}</label>
      <div style={{ display: 'flex', gap: 6, marginBottom: 8 }}>
        <button type="button" className={`pill${!custom ? ' active' : ''}`} onClick={() => onCustomChange(false)}>
          {standardText}
        </button>
        <button type="button" className={`pill${custom ? ' active' : ''}`} onClick={() => onCustomChange(true)}>
          Custom
        </button>
      </div>
      {custom && (
        <div style={{ position: 'relative' }}>
          {prefix && (
            <span style={{ position: 'absolute', left: 13, top: '50%', transform: 'translateY(-50%)', color: 'var(--text-2)', fontSize: 14 }}>
              {prefix}
            </span>
          )}
          <input
            id={id}
            type="number"
            inputMode="decimal"
            step={step}
            min={min}
            max={max}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            style={{ paddingLeft: prefix ? 28 : undefined, paddingRight: suffix ? 30 : undefined, fontVariantNumeric: 'tabular-nums' }}
          />
          {suffix && (
            <span style={{ position: 'absolute', right: 13, top: '50%', transform: 'translateY(-50%)', color: 'var(--text-2)', fontSize: 14 }}>
              {suffix}
            </span>
          )}
        </div>
      )}
      {error ? <div className="field-error-msg">{error}</div> : <div className="field-hint">{hint}</div>}
    </div>
  );
}

function SplitLine({
  label, value, sub, strong, tone,
}: {
  label: string;
  value: string;
  sub?: string;
  strong?: boolean;
  tone?: 'danger' | 'warning' | 'success';
}) {
  const color = tone === 'danger' ? 'var(--danger-fg)' : tone === 'warning' ? 'var(--warning-fg)' : tone === 'success' ? 'var(--success-fg)' : 'var(--text)';
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', gap: 12, padding: '4px 0' }}>
      <div style={{ minWidth: 0 }}>
        <div style={{ fontSize: 13, fontWeight: strong ? 700 : 500 }}>{label}</div>
        {sub && <div style={{ fontSize: 11.5, color: 'var(--text-3)' }}>{sub}</div>}
      </div>
      <div style={{ fontSize: strong ? 15 : 13.5, fontWeight: strong ? 800 : 600, color, fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' }}>
        {value}
      </div>
    </div>
  );
}