'use client';
// ── Full verification (KYC tier 2) ────────────────────────────────────────────
// Unlocks paid events and public listings. The submission is the legal name +
// SA ID (same identity as basic) plus a photo of the ID document; the backend
// runs a VerifyNow Home Affairs check — a full name match verifies INSTANTLY,
// anything else goes to admin review (usually 1 business day).
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useToast } from '../../../lib/toast';
import { refreshAccessToken } from '../../../lib/api';
import { getKycStatus, readKycState, submitFullKyc, looksLikeSaId } from '../../../lib/kyc';

export default function FullKycPage() {
  const router = useRouter();
  const toast = useToast();

  const [tier, setTier] = useState<number | null>(null);
  const [state, setState] = useState<string>('none');
  const [loading, setLoading] = useState(true);

  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [idNumber, setIdNumber] = useState('');
  const [idDoc, setIdDoc] = useState<File | null>(null);
  const [proof, setProof] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = () => getKycStatus().then((s) => {
    setTier(s.kyc_level);
    setState(readKycState(s));
  }).catch(() => {}).finally(() => setLoading(false));

  useEffect(() => { load(); }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (firstName.trim().length < 2 || lastName.trim().length < 2) {
      setError('Enter your legal first and last name as they appear on your ID.');
      return;
    }
    if (!looksLikeSaId(idNumber)) {
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
      form.append('sa_id_number', idNumber.trim());
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

  if (loading) {
    return <div style={{ padding: 40, textAlign: 'center' }}><div className="spinner spinner-lg" /></div>;
  }

  const verified = tier === 2 || state === 'verified';
  const pending = state === 'pending_review';

  return (
    <div style={{ padding: 24, maxWidth: 640 }}>
      <h1 style={{ fontSize: 22, fontWeight: 700, margin: '0 0 4px' }}>Full verification</h1>
      <p style={{ fontSize: 13.5, color: 'var(--text-3, #606770)', margin: '0 0 20px', lineHeight: 1.5 }}>
        Required for <strong>paid events</strong> and <strong>public listings</strong>.
        Your ID is checked against the Home Affairs registry — a match verifies you
        instantly; otherwise our team reviews it, usually within 1 business day.
      </p>

      {verified && (
        <div className="alert alert-success" style={{ marginBottom: 16 }}>
          ✓ Your identity is verified. Paid and public events are unlocked.
          <button className="btn btn-sm" style={{ marginLeft: 10 }}
            onClick={() => router.push('/events/create')}>Create an event</button>
        </div>
      )}

      {!verified && pending && (
        <div className="alert alert-info" style={{ marginBottom: 16 }}>
          Your submission is under review — usually within 1 business day. You can
          keep creating private free events in the meantime.
        </div>
      )}

      {!verified && !pending && (
        <div style={{ background: '#fff', border: '1px solid #dddfe2', borderRadius: 10, padding: 20 }}>
          {state === 'rejected' && (
            <div className="alert alert-error" style={{ marginBottom: 14 }}>
              Your previous submission was rejected. Check your details and resubmit.
            </div>
          )}
          <form onSubmit={submit}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginBottom: 12 }}>
              <div>
                <label style={lbl}>Legal first name</label>
                <input className="input" style={inp} value={firstName}
                  onChange={(e) => setFirstName(e.target.value)} placeholder="As on your ID" />
              </div>
              <div>
                <label style={lbl}>Legal last name</label>
                <input className="input" style={inp} value={lastName}
                  onChange={(e) => setLastName(e.target.value)} placeholder="As on your ID" />
              </div>
            </div>
            <div style={{ marginBottom: 12 }}>
              <label style={lbl}>SA ID number</label>
              <input className="input" style={inp} value={idNumber}
                onChange={(e) => setIdNumber(e.target.value.replace(/\D/g, '').slice(0, 13))}
                placeholder="13 digits" inputMode="numeric" />
            </div>
            <div style={{ marginBottom: 12 }}>
              <label style={lbl}>ID document photo (required)</label>
              <input type="file" accept="image/jpeg,image/png,image/webp"
                onChange={(e) => setIdDoc(e.target.files?.[0] ?? null)} />
              <div style={hint}>JPEG, PNG or WebP · max 5MB · all corners visible</div>
            </div>
            <div style={{ marginBottom: 14 }}>
              <label style={lbl}>Proof of address (optional)</label>
              <input type="file" accept="image/jpeg,image/png,image/webp,application/pdf"
                onChange={(e) => setProof(e.target.files?.[0] ?? null)} />
            </div>

            {error && <div className="alert alert-error" style={{ fontSize: 13, marginBottom: 12 }}>{error}</div>}

            <button type="submit" className="btn btn-primary" disabled={busy}>
              {busy ? 'Verifying…' : 'Submit for verification'}
            </button>
          </form>
        </div>
      )}
    </div>
  );
}

const lbl: React.CSSProperties = { display: 'block', fontSize: 12.5, fontWeight: 600, marginBottom: 4 };
const inp: React.CSSProperties = { width: '100%', padding: '9px 11px', border: '1px solid #dddfe2', borderRadius: 6, fontSize: 14 };
const hint: React.CSSProperties = { fontSize: 12, color: '#606770', marginTop: 4 };