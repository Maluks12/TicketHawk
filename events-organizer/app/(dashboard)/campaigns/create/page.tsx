'use client';
import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '../../../../lib/api';
import { useToast } from '../../../../lib/toast';

interface AdAccount { id: string; name: string; status: string; has_payment_method: boolean; }
interface OrgEvent  { id: string; title: string; status: string; }

// Step ids
type Step = 'details' | 'budget';

export default function CreateCampaignPage() {
  const router = useRouter();
  const toast = useToast();

  const [step, setStep] = useState<Step>('details');
  const [accounts, setAccounts] = useState<AdAccount[]>([]);
  const [events, setEvents] = useState<OrgEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  // Form state
  const [selectedEvent, setSelectedEvent] = useState('');
  const [selectedAccount, setSelectedAccount] = useState('');
  const [campaignName, setCampaignName] = useState('');
  const [dailyBudget, setDailyBudget] = useState('200');
  const [startDate, setStartDate] = useState('');
  const [endDate, setEndDate] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    Promise.all([
      api.get('/organiser/ad-accounts'),
      api.get('/organiser/events'),
    ]).then(([accRes, evRes]) => {
      setAccounts(accRes.data.accounts ?? []);
      const published = (evRes.data.events ?? []).filter((e: OrgEvent) => e.status === 'published');
      setEvents(published);
    }).catch(() => toast('Failed to load data', 'error'))
      .finally(() => setLoading(false));
  }, []);

  // Auto-set name when event selected
  useEffect(() => {
    const ev = events.find(e => e.id === selectedEvent);
    if (ev) setCampaignName(`Boost: ${ev.title}`);
  }, [selectedEvent, events]);

  // ── Validation ──────────────────────────────────────────────────────────────
  function validateDetails() {
    const errs: Record<string, string> = {};
    if (!selectedEvent) errs.event = 'Select an event to boost';
    if (!selectedAccount) errs.account = 'Select an ad account';
    return errs;
  }

  function validateBudget() {
    const errs: Record<string, string> = {};
    const b = parseFloat(dailyBudget);
    if (isNaN(b) || b < 200) errs.budget = 'Minimum daily budget is R200';
    return errs;
  }

  // ── Navigation ──────────────────────────────────────────────────────────────
  function handleNext() {
    if (step === 'details') {
      const errs = validateDetails();
      if (Object.keys(errs).length) { setErrors(errs); return; }
      setErrors({});
      setStep('budget');
    }
  }

  function handleBack() {
    if (step === 'budget') setStep('details');
    else router.push('/campaigns');
  }

  // ── Submit ──────────────────────────────────────────────────────────────────
  async function handleSubmit() {
    const errs = validateBudget();
    if (Object.keys(errs).length) { setErrors(errs); return; }
    setSubmitting(true);
    try {
      await api.post(`/organiser/events/${selectedEvent}/campaigns`, {
        ad_account_id: selectedAccount,
        daily_budget: parseFloat(dailyBudget),
        start_date: startDate || undefined,
        end_date: endDate || undefined,
      });
      toast('Campaign created and is now active!', 'success');
      router.push('/campaigns');
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      toast(msg ?? 'Failed to create campaign', 'error');
    } finally { setSubmitting(false); }
  }

  // ── Score ───────────────────────────────────────────────────────────────────
  const score = selectedEvent && selectedAccount && parseFloat(dailyBudget) >= 200 ? 100
    : selectedEvent && selectedAccount ? 60
    : selectedEvent ? 30 : 0;

  const scoreColor = score === 100 ? '#1a7f37' : score >= 60 ? '#1877f2' : '#e65c00';

  const selectedEventTitle = events.find(e => e.id === selectedEvent)?.title ?? '';
  const selectedAccountName = accounts.find(a => a.id === selectedAccount)?.name ?? '';

  if (loading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
        <div className="spinner spinner-lg" />
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', height: '100%', background: '#f0f2f5', overflow: 'hidden' }}>

      {/* ── Left: Hierarchy tree panel ─────────────────────────────────── */}
      <div style={{
        width: 280, flexShrink: 0, background: '#fff',
        borderRight: '1px solid #dddfe2',
        display: 'flex', flexDirection: 'column', overflow: 'hidden',
      }}>
        {/* Panel header with close */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '12px 16px', borderBottom: '1px solid #dddfe2',
        }}>
          <div style={{ fontSize: 14, fontWeight: 600, color: '#606770' }}>New Campaign</div>
          <button className="btn-icon" onClick={() => router.push('/campaigns')}>✕</button>
        </div>

        {/* Hierarchy tree — Campaign → (future: Ad Set → Ad) */}
        <div style={{ flex: 1, overflow: 'auto', padding: 8 }}>
          <div style={{
            display: 'flex', alignItems: 'center', gap: 8,
            padding: '8px 10px', borderRadius: 6,
            background: '#e7f3ff', marginBottom: 2,
          }}>
            <svg viewBox="0 0 20 20" fill="#1877f2" width="16" height="16">
              <path d="M2 11a1 1 0 011-1h2a1 1 0 011 1v5a1 1 0 01-1 1H3a1 1 0 01-1-1v-5zM8 7a1 1 0 011-1h2a1 1 0 011 1v9a1 1 0 01-1 1H9a1 1 0 01-1-1V7zM14 4a1 1 0 011-1h2a1 1 0 011 1v12a1 1 0 01-1 1h-2a1 1 0 01-1-1V4z"/>
            </svg>
            <div>
              <div style={{ fontSize: 13, fontWeight: 600, color: '#1877f2' }}>
                {selectedEventTitle || 'New Campaign'}
              </div>
              <div style={{ fontSize: 11, color: '#606770' }}>Campaign</div>
            </div>
          </div>
        </div>
      </div>

      {/* ── Center: Scrollable form ─────────────────────────────────────── */}
      <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>

        {/* Breadcrumb topbar */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '10px 20px', background: '#fff', borderBottom: '1px solid #dddfe2',
          flexShrink: 0,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: '#606770' }}>
            <span style={{ fontWeight: 600, color: '#1877f2' }}>
              {selectedEventTitle || 'New Campaign'}
            </span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{
              display: 'flex', alignItems: 'center', gap: 5,
              fontSize: 12, color: '#606770', background: '#f0f2f5',
              padding: '3px 10px', borderRadius: 99,
            }}>
              <span style={{ width: 8, height: 8, borderRadius: '50%', background: '#ccd0d5', display: 'inline-block' }} />
              In draft
            </span>
          </div>
        </div>

        {/* Step tabs */}
        <div style={{
          display: 'flex', gap: 0, padding: '0 20px',
          background: '#fff', borderBottom: '1px solid #dddfe2', flexShrink: 0,
        }}>
          {([
            { id: 'details' as Step, label: 'Campaign details' },
            { id: 'budget'  as Step, label: 'Budget' },
          ] as const).map(s => (
            <button
              key={s.id}
              onClick={() => {
                if (s.id === 'budget' && Object.keys(validateDetails()).length === 0) setStep(s.id);
                if (s.id === 'details') setStep(s.id);
              }}
              style={{
                padding: '11px 16px', fontSize: 13, fontWeight: 600,
                border: 'none', background: 'none', cursor: 'pointer',
                borderBottom: `2px solid ${step === s.id ? '#1877f2' : 'transparent'}`,
                color: step === s.id ? '#1877f2' : '#606770',
                marginBottom: -1,
              }}
            >
              {s.label}
            </button>
          ))}
        </div>

        {/* Form content */}
        <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
          {step === 'details' && (
            <DetailsStep
              events={events}
              accounts={accounts}
              selectedEvent={selectedEvent}
              selectedAccount={selectedAccount}
              campaignName={campaignName}
              onSelectEvent={setSelectedEvent}
              onSelectAccount={setSelectedAccount}
              onNameChange={setCampaignName}
              errors={errors}
            />
          )}
          {step === 'budget' && (
            <BudgetStep
              dailyBudget={dailyBudget}
              startDate={startDate}
              endDate={endDate}
              onBudgetChange={setDailyBudget}
              onStartChange={setStartDate}
              onEndChange={setEndDate}
              errors={errors}
            />
          )}
        </div>

        {/* Footer: Close | Back | Next/Publish */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '12px 20px', background: '#fff', borderTop: '1px solid #dddfe2',
          flexShrink: 0,
        }}>
          <button className="btn" onClick={handleBack}>
            {step === 'details' ? 'Close' : '← Back'}
          </button>
          <div style={{ display: 'flex', gap: 8 }}>
            {step !== 'budget' ? (
              <button className="btn btn-primary" onClick={handleNext}>
                Next →
              </button>
            ) : (
              <button className="btn btn-primary" onClick={handleSubmit} disabled={submitting}>
                {submitting ? 'Creating…' : 'Publish campaign'}
              </button>
            )}
          </div>
        </div>
      </div>

      {/* ── Right: Score + recommendations panel ───────────────────────── */}
      <div style={{
        width: 280, flexShrink: 0, background: '#fff',
        borderLeft: '1px solid #dddfe2', overflow: 'auto', padding: 16,
      }}>
        {/* Score circle — same as Meta's campaign score widget */}
        <div style={{ marginBottom: 20 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 8 }}>
            <div style={{ position: 'relative', width: 52, height: 52 }}>
              <svg viewBox="0 0 36 36" width="52" height="52" style={{ transform: 'rotate(-90deg)' }}>
                <circle cx="18" cy="18" r="15.9" fill="none" stroke="#ebedf0" strokeWidth="3"/>
                <circle cx="18" cy="18" r="15.9" fill="none" stroke={scoreColor}
                  strokeWidth="3" strokeDasharray={`${score} 100`}
                  strokeLinecap="round"/>
              </svg>
              <div style={{
                position: 'absolute', inset: 0, display: 'flex', alignItems: 'center',
                justifyContent: 'center', fontSize: 13, fontWeight: 700, color: scoreColor,
              }}>{score}</div>
            </div>
            <div>
              <div style={{ fontSize: 14, fontWeight: 700, color: '#1c1e21' }}>Campaign score</div>
              <div style={{ fontSize: 12, color: '#606770' }}>
                {score === 100 ? "You're using the recommended setup." : 'Complete setup to improve score.'}
              </div>
            </div>
          </div>
        </div>

        <div style={{ height: 1, background: '#ebedf0', margin: '12px 0' }}/>

        {/* Recommendations */}
        <div style={{ fontSize: 13, fontWeight: 700, color: '#1c1e21', marginBottom: 12 }}>
          Recommendations
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          <RecommendationItem
            done={!!selectedEvent}
            label="Select an event to boost"
            desc="Choose a published event to promote."
          />
          <RecommendationItem
            done={!!selectedAccount}
            label="Link an ad account"
            desc="Ad account determines your billing method."
          />
          <RecommendationItem
            done={parseFloat(dailyBudget) >= 200}
            label="Set daily budget (min R200)"
            desc="Higher budgets reach more people faster."
          />
        </div>

        {selectedAccountName && (
          <>
            <div style={{ height: 1, background: '#ebedf0', margin: '16px 0' }}/>
            <div style={{ fontSize: 12, color: '#606770' }}>
              <div style={{ fontWeight: 600, marginBottom: 4 }}>Billing account</div>
              <div>{selectedAccountName}</div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

// ── Step 1: Campaign Details ───────────────────────────────────────────────────
function DetailsStep({
  events, accounts, selectedEvent, selectedAccount, campaignName,
  onSelectEvent, onSelectAccount, onNameChange, errors,
}: {
  events: OrgEvent[]; accounts: AdAccount[];
  selectedEvent: string; selectedAccount: string; campaignName: string;
  onSelectEvent: (id: string) => void;
  onSelectAccount: (id: string) => void;
  onNameChange: (v: string) => void;
  errors: Record<string, string>;
}) {
  return (
    <div style={{ maxWidth: 600, display: 'flex', flexDirection: 'column', gap: 16 }}>

      {/* Campaign name card */}
      <FormCard
        icon="✓"
        iconColor="#1a7f37"
        title="Campaign name"
        complete={!!campaignName}
      >
        <div className="field" style={{ marginBottom: 0 }}>
          <input
            value={campaignName}
            onChange={e => onNameChange(e.target.value)}
            placeholder="e.g. Boost: Summer Concert"
          />
        </div>
      </FormCard>

      {/* Campaign details card */}
      <FormCard
        icon={selectedEvent && selectedAccount ? '✓' : '○'}
        iconColor={selectedEvent && selectedAccount ? '#1a7f37' : '#606770'}
        title="Campaign details"
        complete={!!selectedEvent && !!selectedAccount}
      >
        <div className={`field${errors.event ? ' field-error' : ''}`}>
          <label>Event to boost</label>
          <select value={selectedEvent} onChange={e => onSelectEvent(e.target.value)}>
            <option value="">— Select a published event —</option>
            {events.map(e => (
              <option key={e.id} value={e.id}>{e.title}</option>
            ))}
          </select>
          {errors.event && <div className="field-error-msg" style={{ color: '#c41c1c' }}>{errors.event}</div>}
          {events.length === 0 && (
            <div className="field-hint">No published events found. Publish an event first.</div>
          )}
        </div>

        <div className={`field${errors.account ? ' field-error' : ''}`} style={{ marginBottom: 0 }}>
          <label>Ad account</label>
          <select value={selectedAccount} onChange={e => onSelectAccount(e.target.value)}>
            <option value="">— Select an ad account —</option>
            {accounts.filter(a => a.status === 'active').map(a => (
              <option key={a.id} value={a.id} disabled={!a.has_payment_method}>
                {a.name}{!a.has_payment_method ? ' (no payment method)' : ''}
              </option>
            ))}
          </select>
          {errors.account && <div className="field-error-msg" style={{ color: '#c41c1c' }}>{errors.account}</div>}
          {accounts.length === 0 && (
            <div className="field-hint">
              No ad accounts yet. Create one first under Campaigns → Ad Accounts.
            </div>
          )}
        </div>
      </FormCard>
    </div>
  );
}

// ── Step 2: Budget ─────────────────────────────────────────────────────────────
function BudgetStep({
  dailyBudget, startDate, endDate,
  onBudgetChange, onStartChange, onEndChange, errors,
}: {
  dailyBudget: string; startDate: string; endDate: string;
  onBudgetChange: (v: string) => void;
  onStartChange: (v: string) => void;
  onEndChange: (v: string) => void;
  errors: Record<string, string>;
}) {
  const budget = parseFloat(dailyBudget) || 0;
  const impressions = budget > 0 ? Math.floor((budget / 20) * 1000) : 0;

  return (
    <div style={{ maxWidth: 600, display: 'flex', flexDirection: 'column', gap: 16 }}>
      <FormCard icon="✓" iconColor="#1a7f37" title="Budget" complete={budget >= 200}>

        <div style={{ marginBottom: 16 }}>
          <div style={{ fontSize: 13, fontWeight: 600, color: '#1c1e21', marginBottom: 8 }}>
            Budget strategy
          </div>
          <label className="check-row" style={{ padding: '10px 12px', border: '2px solid #1877f2', borderRadius: 6, background: '#e7f3ff' }}>
            <input type="radio" name="budget_type" defaultChecked style={{ accentColor: '#1877f2' }} />
            <div>
              <div style={{ fontWeight: 600, fontSize: 14 }}>Daily budget</div>
              <div style={{ fontSize: 12, color: '#606770' }}>Set a fixed amount to spend per day on this campaign.</div>
            </div>
          </label>
        </div>

        <div className={`field${errors.budget ? ' field-error' : ''}`}>
          <label>Daily budget (ZAR)</label>
          <div style={{ display: 'flex', alignItems: 'center', gap: 0, position: 'relative' }}>
            <span style={{
              position: 'absolute', left: 12, fontSize: 14, fontWeight: 600,
              color: '#606770', pointerEvents: 'none',
            }}>R</span>
            <input
              type="number"
              value={dailyBudget}
              onChange={e => onBudgetChange(e.target.value)}
              min="200"
              style={{ paddingLeft: 28 }}
            />
          </div>
          {errors.budget
            ? <div className="field-error-msg" style={{ color: '#c41c1c' }}>{errors.budget}</div>
            : budget >= 200 && (
              <div className="field-hint">
                Estimated {impressions.toLocaleString()} impressions/day at R20 CPM
              </div>
            )
          }
        </div>

        <div className="field-row">
          <div className="field" style={{ marginBottom: 0 }}>
            <label>Start date (optional)</label>
            <input type="date" value={startDate} onChange={e => onStartChange(e.target.value)} />
          </div>
          <div className="field" style={{ marginBottom: 0 }}>
            <label>End date (optional)</label>
            <input type="date" value={endDate} onChange={e => onEndChange(e.target.value)} />
          </div>
        </div>
      </FormCard>

      {/* Billing model explanation */}
      <div className="alert alert-info">
        <svg viewBox="0 0 20 20" fill="currentColor" width="16" height="16" style={{ flexShrink: 0, marginTop: 1 }}>
          <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd"/>
        </svg>
        <div>
          <strong>Post-pay billing.</strong> Your campaign starts immediately with no upfront payment.
          Yesterday's actual spend is debited each morning via your Stitch mandate.
        </div>
      </div>
    </div>
  );
}

// ── FormCard — matches Meta's card sections with a green checkmark header ──────
function FormCard({
  icon, iconColor, title, complete, children,
}: {
  icon: string; iconColor: string; title: string; complete: boolean; children: React.ReactNode;
}) {
  return (
    <div style={{
      background: '#fff', border: '1px solid #dddfe2', borderRadius: 8, overflow: 'hidden',
    }}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8,
        padding: '12px 16px', borderBottom: '1px solid #ebedf0',
        background: complete ? '#f0fff4' : '#fff',
      }}>
        <span style={{ fontSize: 16, color: iconColor }}>{icon}</span>
        <span style={{ fontSize: 15, fontWeight: 700, color: '#1c1e21' }}>{title}</span>
      </div>
      <div style={{ padding: 16 }}>{children}</div>
    </div>
  );
}

// ── Recommendation item ────────────────────────────────────────────────────────
function RecommendationItem({ done, label, desc }: { done: boolean; label: string; desc: string }) {
  return (
    <div style={{ display: 'flex', gap: 8, alignItems: 'flex-start' }}>
      <span style={{
        width: 18, height: 18, borderRadius: '50%', flexShrink: 0, marginTop: 1,
        background: done ? '#1a7f37' : '#ebedf0',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontSize: 10, color: done ? '#fff' : '#8a8d91',
      }}>
        {done ? '✓' : '·'}
      </span>
      <div>
        <div style={{ fontSize: 13, fontWeight: 600, color: done ? '#606770' : '#1c1e21', textDecoration: done ? 'line-through' : 'none' }}>{label}</div>
        {!done && <div style={{ fontSize: 11, color: '#8a8d91', marginTop: 1 }}>{desc}</div>}
      </div>
    </div>
  );
}