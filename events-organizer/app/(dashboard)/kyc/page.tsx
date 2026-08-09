'use client';
// ── Full verification (KYC tier 2) + payout details ───────────────────────────
// Publishing a PAID or PUBLIC event is gated on TWO server-side checks, and
// this is the one screen that satisfies both:
//
//   1. Identity — tier-2 KYC. Legal name + SA ID + a photo of the ID document.
//      The backend runs a VerifyNow Home Affairs check; a full name match
//      verifies INSTANTLY, anything else goes to admin review (~1 business day).
//
//   2. Bank details — PublishEvent returns ErrPayoutDetailsRequired for any
//      PAID event when payout_enabled is false, independent of KYC. The bank
//      form used to live on /payouts, which was retired with the ads surface
//      and is hidden from the sidebar, so there was no reachable way to enter
//      it: verification "passed" and paid events still refused to publish.
//      It belongs here — the create-event screen sends organisers to /kyc the
//      moment they pick a paid or public event class.
//
// Identity fields are PREFILLED from the tier-1 onboarding record. The legal
// name stays editable (a tier-1 typo must be fixable, or the DHA check can
// never pass), but nobody retypes what they already gave us. The ID number
// can't be prefilled — it's stored AES-GCM encrypted and only the last 4
// digits come back to the client — so it's shown as "on file" and the field
// is omitted from the submission entirely; the server reuses the stored value.
import { useCallback, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useToast } from '../../../lib/toast';
import { refreshAccessToken } from '../../../lib/api';
import { getKycStatus, readKycState, submitFullKyc, looksLikeSaId, KycProfile } from '../../../lib/kyc';
import {
  getBanks, getPayoutProfile, setPayoutDetails, looksLikeAccountNumber,
  Bank, PayoutProfile,
} from '../../../lib/payouts';
import FileDropZone from '@/components/fileDropZone';
import { ShieldCheck, ArrowSquareOut } from '@phosphor-icons/react';
import { ORGANISER_LEGAL_DOCS, legalLinkProps } from '../../../lib/legalLinks';

const ACCOUNT_TYPES = ['Savings', 'Current', 'Transmission'];

export default function FullKycPage() {
  const router = useRouter();
  const toast = useToast();

  const [tier, setTier] = useState<number | null>(null);
  const [state, setState] = useState<string>('none');
  const [profile, setProfile] = useState<KycProfile | null>(null);
  const [loading, setLoading] = useState(true);

  // ── Identity form ──────────────────────────────────────────────────────────
  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [idNumber, setIdNumber] = useState('');
  const [overrideId, setOverrideId] = useState(false); // "use a different ID"
  const [idDoc, setIdDoc] = useState<File | null>(null);
  const [proof, setProof] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // ── Payout form ────────────────────────────────────────────────────────────
  const [payout, setPayout] = useState<PayoutProfile | null>(null);
  const [banks, setBanks] = useState<Bank[]>([]);
  const [banksError, setBanksError] = useState<string | null>(null);
  const [editingBank, setEditingBank] = useState(false);
  const [bankCode, setBankCode] = useState('');
  const [accountNumber, setAccountNumber] = useState('');
  const [accountName, setAccountName] = useState('');
  const [accountType, setAccountType] = useState('Savings');
  const [bankBusy, setBankBusy] = useState(false);
  const [bankError, setBankError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const s = await getKycStatus();
      setTier(s.kyc_level);
      setState(readKycState(s));
      setProfile(s.kyc ?? null);

      // Prefill the identity fields from what tier-1 already captured.
      const first = s.kyc?.legal_first_name ?? '';
      const last = s.kyc?.legal_last_name ?? '';
      setFirstName((v) => v || first);
      setLastName((v) => v || last);
      // Bank account holder name defaults to the legal name — VerifyNow's
      // ownership check compares exactly these two, so seeding it cuts the
      // most common mismatch before it happens.
      setAccountName((v) => v || [first, last].filter(Boolean).join(' '));
    } catch {
      /* leave the form usable */
    }

    const p = await getPayoutProfile();
    setPayout(p);
    if (p?.bank_account_name) setAccountName((v) => v || p.bank_account_name!);
    setEditingBank(!p?.payout_enabled);

    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  // Bank list is only needed once the organiser is actually editing.
  useEffect(() => {
    if (!editingBank || banks.length > 0) return;
    getBanks()
      .then((b) => { setBanks(b); setBanksError(null); })
      .catch(() => setBanksError('Could not load the bank list. Refresh the page and try again.'));
  }, [editingBank, banks.length]);

  const hasIdOnFile = Boolean(profile?.id_last_4);
  const askForId = overrideId || !hasIdOnFile;

  // ── Identity submit ────────────────────────────────────────────────────────
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (firstName.trim().length < 2 || lastName.trim().length < 2) {
      setError('Enter your legal first and last name as they appear on your ID.');
      return;
    }
    if (askForId && !looksLikeSaId(idNumber)) {
      setError('Your SA ID number should be exactly 13 digits.');
      return;
    }
    if (!idDoc) {
      setError('A photo of your ID document is required (JPEG/PNG/WebP, max 5MB).');
      return;
    }
    setBusy(true);
    try {
      const form = new FormData();
      form.append('legal_first_name', firstName.trim());
      form.append('legal_last_name', lastName.trim());
      // Omitted entirely when it's already on file — the server decrypts and
      // reuses the stored ID rather than making the organiser retype it.
      if (askForId) form.append('sa_id_number', idNumber.trim());
      form.append('id_document', idDoc);
      if (proof) form.append('proof_of_address', proof);

      const { data } = await submitFullKyc(form);
      const newState = String(data.kyc_status ?? data.status ?? 'pending_review');
      setState(newState);
      if (newState === 'verified') {
        setTier(2);
        try { await refreshAccessToken(); } catch { /* non-fatal */ }
        toast('Identity verified — paid and public events are unlocked', 'success');
      } else {
        toast('Submitted for review', 'success');
      }
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error;
      setError(msg ?? 'Submission failed — check your details and try again.');
    } finally {
      setBusy(false);
    }
  }

  // ── Bank submit ────────────────────────────────────────────────────────────
  async function submitBank(e: React.FormEvent) {
    e.preventDefault();
    setBankError(null);
    const bank = banks.find((b) => b.code === bankCode);
    if (!bank) {
      setBankError('Choose the bank your account is held at.');
      return;
    }
    if (!looksLikeAccountNumber(accountNumber)) {
      setBankError('Enter your account number — digits only, between 6 and 20 of them.');
      return;
    }
    if (accountName.trim().length < 2) {
      setBankError('Enter the account holder name exactly as your bank has it.');
      return;
    }
    setBankBusy(true);
    try {
      const { data } = await setPayoutDetails({
        bank_code: bank.code,
        bank_name: bank.name,
        account_number: accountNumber.trim(),
        account_name: accountName.trim(),
        account_type: accountType,
      });
      setPayout(data.profile ?? null);
      setEditingBank(false);
      setAccountNumber('');
      toast('Payout details saved', 'success');
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error;
      setBankError(msg ?? 'Could not save these bank details. Check the account number and bank, then try again.');
    } finally {
      setBankBusy(false);
    }
  }

  if (loading) {
    return <div style={{ padding: 40, textAlign: 'center' }}><div className="spinner spinner-lg" /></div>;
  }

  const verified = tier === 2 || state === 'verified';
  const pending = state === 'pending_review';
  const bankReady = Boolean(payout?.payout_enabled);
  const readyForPaid = verified && bankReady;

  return (
    // Was maxWidth: 640 — a single 640px column on a 1900px monitor, with the
    // two steps stacked so the bank form sat a full screen below the identity
    // form. They're independent and both required, so they now sit side by side
    // and are visible at the same time.
    <div style={{ padding: '24px 28px 48px', maxWidth: 1440, margin: '0 auto' }}>
      <h1 style={{ fontSize: 22, fontWeight: 700, margin: '0 0 4px' }}>Verification &amp; payouts</h1>
      <p style={{ fontSize: 13.5, color: 'var(--text-2)', margin: '0 0 18px', lineHeight: 1.5 }}>
        Both steps below are required before you can publish a <strong>paid</strong> or{' '}
        <strong>public</strong> event. Your ID is checked against the Home Affairs
        registry — a match verifies you instantly; otherwise our team reviews it,
        usually within 1 business day.
      </p>

      {/* ── Progress ──────────────────────────────────────────────────────── */}
      <div style={{ display: 'flex', gap: 10, marginBottom: 18 }}>
        <StepPill n={1} label="Identity"
          done={verified} pending={!verified && pending} />
        <StepPill n={2} label="Payout bank details" done={bankReady} pending={false} />
      </div>

      {readyForPaid && (
        <div className="alert alert-success" style={{ marginBottom: 16 }}>
          ✓ You&apos;re fully set up. Paid and public events are unlocked.
          <button className="btn btn-sm" style={{ marginLeft: 10 }}
            onClick={() => router.push('/events/create')}>Create an event</button>
        </div>
      )}

      {/* Two independent, both-required steps. Side by side rather than
          stacked, so an organiser can see what is still outstanding without
          scrolling — and so the 640px column stops wasting two thirds of a
          desktop. alignItems:start keeps each card its natural height instead
          of stretching the shorter one. */}
      <div style={{
        display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(440px, 1fr))',
        gap: 18, alignItems: 'start',
      }}>
      {/* ── Step 1: identity ──────────────────────────────────────────────── */}
      <Section title="1. Identity verification"
        badge={verified ? 'Verified' : pending ? 'Under review' : 'Required'}
        tone={verified ? 'good' : pending ? 'wait' : 'todo'}>

        {verified && (
          <div style={{ fontSize: 13.5, color: 'var(--success-fg)' }}>
            Your identity is verified
            {profile?.id_last_4 ? ` against SA ID ending ${profile.id_last_4}` : ''}.
          </div>
        )}

        {!verified && pending && (
          <div style={{ fontSize: 13.5, color: 'var(--text-2)', lineHeight: 1.5 }}>
            Your submission is under review — usually within 1 business day. You can
            keep creating private free events in the meantime.
          </div>
        )}

        {!verified && !pending && (
          <>
            {state === 'rejected' && (
              <div className="alert alert-danger" style={{ marginBottom: 14 }}>
                Your previous submission was rejected
                {profile?.rejection_reason ? `: ${profile.rejection_reason}` : '.'}{' '}
                Check your details and resubmit.
              </div>
            )}
            <form onSubmit={submit}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginBottom: 4 }}>
                <div>
                  <label style={lbl}>Legal first name</label>
                  <input className="input" style={inp} value={firstName}
                    onChange={(e) => setFirstName(e.target.value)}
                    placeholder="As on your ID" autoComplete="given-name" />
                </div>
                <div>
                  <label style={lbl}>Legal last name</label>
                  <input className="input" style={inp} value={lastName}
                    onChange={(e) => setLastName(e.target.value)}
                    placeholder="As on your ID" autoComplete="family-name" />
                </div>
              </div>
              {(profile?.legal_first_name || profile?.legal_last_name) && (
                <div style={{ ...hint, marginBottom: 12 }}>
                  Filled in from the details you gave when you set up your profile.
                  Correct them here if they don&apos;t match your ID exactly.
                </div>
              )}

              <div style={{ marginBottom: 12 }}>
                <label style={lbl}>SA ID number</label>
                {!askForId ? (
                  <div style={{
                    display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                    gap: 10, padding: '9px 11px', border: '1px solid var(--border)',
                    borderRadius: 6, background: 'var(--surface-2)', fontSize: 14,
                  }}>
                    <span>Using the ID on your profile — ending <strong>{profile?.id_last_4}</strong></span>
                    <button type="button" className="btn btn-xs"
                      onClick={() => { setOverrideId(true); setIdNumber(''); }}>
                      Use a different ID
                    </button>
                  </div>
                ) : (
                  <>
                    <input className="input" style={inp} value={idNumber}
                      onChange={(e) => setIdNumber(e.target.value.replace(/\D/g, '').slice(0, 13))}
                      placeholder="13 digits" inputMode="numeric" />
                    {hasIdOnFile && (
                      <button type="button" className="btn btn-xs" style={{ marginTop: 6 }}
                        onClick={() => { setOverrideId(false); setIdNumber(''); }}>
                        Cancel — use the ID on my profile
                      </button>
                    )}
                  </>
                )}
              </div>

              <FileDropZone
                label="ID document photo"
                required
                file={idDoc}
                onPick={setIdDoc}
                accept={['image/jpeg', 'image/png', 'image/webp']}
                maxMB={5}
                hint="All four corners visible, no glare over the ID number"
              />
              {/* PDF is allowed here and not above: proof of address is
                  normally a downloaded bank statement or municipal bill, which
                  arrives as a PDF, whereas an ID is always a photo. */}
              <FileDropZone
                label="Proof of address (optional)"
                file={proof}
                onPick={setProof}
                accept={['image/jpeg', 'image/png', 'image/webp', 'application/pdf']}
                maxMB={5}
                hint="Bank statement or municipal bill, less than 3 months old"
              />

              {error && <div className="alert alert-danger" style={{ fontSize: 13, marginBottom: 12 }}>{error}</div>}

              <button type="submit" className="btn btn-primary" disabled={busy}>
                {busy ? 'Verifying…' : 'Submit for verification'}
              </button>
            </form>
          </>
        )}
      </Section>

      {/* ── Step 2: payout bank details ───────────────────────────────────── */}
      <Section title="2. Payout bank details"
        badge={bankReady ? 'On file' : 'Required for paid events'}
        tone={bankReady ? 'good' : 'todo'}>

        <p style={{ fontSize: 13, color: 'var(--text-2)', margin: '0 0 12px', lineHeight: 1.5 }}>
          Where we send your ticket money. Settlement runs 48 hours after each
          event completes, minus the 6% organiser commission. The account must
          be in your own name — we check it against your verified ID.
        </p>

        {bankReady && !editingBank && (
          <div style={{
            border: '1px solid var(--border)', borderRadius: 8, padding: 12,
            background: 'var(--surface-2)', fontSize: 13.5, lineHeight: 1.6,
          }}>
            <div><strong>{payout?.settlement_bank_name ?? 'Bank on file'}</strong></div>
            <div style={{ color: 'var(--text-2)' }}>
              {payout?.bank_account_name}
              {payout?.bank_account_last_4 ? ` · account ending ${payout.bank_account_last_4}` : ''}
            </div>
            <button type="button" className="btn btn-sm" style={{ marginTop: 10 }}
              onClick={() => setEditingBank(true)}>
              Change bank details
            </button>
          </div>
        )}

        {editingBank && (
          <form onSubmit={submitBank}>
            {banksError && <div className="alert alert-danger" style={{ fontSize: 13, marginBottom: 12 }}>{banksError}</div>}

            <div style={{ marginBottom: 12 }}>
              <label style={lbl}>Bank</label>
              <select style={inp} value={bankCode} onChange={(e) => setBankCode(e.target.value)}>
                <option value="">{banks.length ? 'Select your bank…' : 'Loading banks…'}</option>
                {banks.map((b) => (
                  <option key={b.code} value={b.code}>{b.name}</option>
                ))}
              </select>
            </div>

            <div style={{ marginBottom: 12 }}>
              <label style={lbl}>Account number</label>
              <input className="input" style={inp} value={accountNumber}
                onChange={(e) => setAccountNumber(e.target.value.replace(/\D/g, '').slice(0, 20))}
                placeholder="Digits only" inputMode="numeric" autoComplete="off" />
              <div style={hint}>
                We store only the last 4 digits — the full number goes straight to
                our payment provider and is never kept here.
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 10, marginBottom: 4 }}>
              <div>
                <label style={lbl}>Account holder name</label>
                <input className="input" style={inp} value={accountName}
                  onChange={(e) => setAccountName(e.target.value)}
                  placeholder="As your bank has it" />
              </div>
              <div>
                <label style={lbl}>Account type</label>
                <select style={inp} value={accountType} onChange={(e) => setAccountType(e.target.value)}>
                  {ACCOUNT_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
                </select>
              </div>
            </div>
            <div style={{ ...hint, marginBottom: 14 }}>
              Prefilled from your legal name. Change it only if your bank holds the
              account under a different spelling.
            </div>

            {bankError && <div className="alert alert-danger" style={{ fontSize: 13, marginBottom: 12 }}>{bankError}</div>}

            <div style={{ display: 'flex', gap: 8 }}>
              <button type="submit" className="btn btn-primary" disabled={bankBusy}>
                {bankBusy ? 'Saving…' : 'Save payout details'}
              </button>
              {bankReady && (
                <button type="button" className="btn" disabled={bankBusy}
                  onClick={() => { setEditingBank(false); setBankError(null); }}>
                  Cancel
                </button>
              )}
            </div>
          </form>
        )}
      </Section>
      </div>

      {/* ── Legal ───────────────────────────────────────────────────────────
          Reference copy, not a consent point. The acceptance itself happens at
          sign-up (new accounts) or through the gate in the dashboard shell
          (existing accounts); duplicating a checkbox here would create a second
          record of the same consent that could disagree with the first. */}
      <div style={{
        background: 'var(--surface)', border: '1px solid var(--border)',
        borderRadius: 'var(--r-xl)', padding: 20, marginTop: 18,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 }}>
          <span style={{
            width: 30, height: 30, borderRadius: 9, flexShrink: 0,
            background: 'var(--accent-soft)', color: 'var(--accent)',
            display: 'grid', placeItems: 'center',
          }}>
            <ShieldCheck size={17} weight="bold" />
          </span>
          <div style={{ fontSize: 15, fontWeight: 700 }}>Your agreements</div>
        </div>
        <p style={{ fontSize: 13, color: 'var(--text-2)', lineHeight: 1.6, margin: '0 0 12px' }}>
          Selling tickets on TicketHawk is governed by the documents below. They
          cover the commission split, when settlement runs, who carries a
          chargeback, and the refund policy buyers are entitled to.
        </p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          {ORGANISER_LEGAL_DOCS.map(d => (
            <a
              key={d.key}
              {...legalLinkProps(d.key)}
              className="btn btn-sm"
              style={{ textDecoration: 'none' }}
            >
              {d.label}
              <ArrowSquareOut size={13} />
            </a>
          ))}
        </div>
      </div>
    </div>
  );
}

// ── Presentational bits ───────────────────────────────────────────────────────

function Section({
  title, badge, tone, children,
}: {
  title: string;
  badge: string;
  tone: 'good' | 'wait' | 'todo';
  children: React.ReactNode;
}) {
  const tones = {
    good: { bg: 'var(--success-bg)', fg: 'var(--success-fg)' },
    wait: { bg: 'var(--warning-bg)', fg: 'var(--warning-fg)' },
    todo: { bg: 'var(--bg)', fg: 'var(--text-2)' },
  }[tone];

  return (
    <div style={{
      background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 10,
      padding: 20, marginBottom: 16,
    }}>
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        gap: 10, marginBottom: 14,
      }}>
        <div style={{ fontSize: 15.5, fontWeight: 700 }}>{title}</div>
        <span style={{
          fontSize: 11.5, fontWeight: 600, padding: '3px 9px', borderRadius: 20,
          background: tones.bg, color: tones.fg, whiteSpace: 'nowrap',
        }}>
          {badge}
        </span>
      </div>
      {children}
    </div>
  );
}

function StepPill({ n, label, done, pending }: { n: number; label: string; done: boolean; pending: boolean }) {
  const color = done ? 'var(--success-fg)' : pending ? 'var(--warning-fg)' : 'var(--text-3)';
  const bg = done ? 'var(--success-bg)' : pending ? 'var(--warning-bg)' : 'var(--bg)';
  return (
    <div style={{
      flex: 1, display: 'flex', alignItems: 'center', gap: 8,
      padding: '8px 12px', borderRadius: 8, background: bg,
      fontSize: 13, fontWeight: 600, color,
    }}>
      <span style={{
        width: 20, height: 20, borderRadius: '50%', background: color, color: '#fff',
        display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
        fontSize: 11.5, flexShrink: 0,
      }}>
        {done ? '✓' : n}
      </span>
      {label}
    </div>
  );
}

const lbl: React.CSSProperties = {
  display: 'block', fontSize: 12.5, fontWeight: 600, marginBottom: 4,
  color: 'var(--text)',
};

// `color` was missing here, which is why the bank and account-type values
// rendered in the browser's default near-black on a dark card. An unset color
// inherits from the UA stylesheet for form controls, NOT from the page — so it
// stayed dark-on-dark however the theme changed.
const inp: React.CSSProperties = {
  width: '100%', padding: '10px 12px',
  border: '1px solid var(--border-2)', borderRadius: 'var(--r)',
  fontSize: 14,
  color: 'var(--text)',
  // Opaque, not the alpha surface: Chrome paints a native <select> popup using
  // the element's own background-color, and alpha there composites over white.
  background: 'var(--surface-2)',
  fontFamily: 'inherit', outline: 'none', boxSizing: 'border-box',
};
const hint: React.CSSProperties = { fontSize: 12, color: 'var(--text-2)', marginTop: 4 };