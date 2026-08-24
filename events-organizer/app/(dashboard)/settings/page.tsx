'use client';
import { useCallback, useEffect, useState } from 'react';
import { Warning, Info } from '@phosphor-icons/react';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';

/**
 * Account settings.
 *
 * Business name, contact email, phone and website — the ACCOUNT, as opposed to
 * the Public Page, which holds the description, cover, socials and visibility.
 *
 * ── Why they are two screens and not one ────────────────────────────────────
 *
 * They are two resources on the server (/organiser/profile and
 * /organiser/public-profile) because they answer different questions. The
 * account is who you are for invoicing, payouts and support contact — the
 * email here is where we reach you, not something attendees see. The public
 * page is what attendees see. Merging them would put a payout contact address
 * on the same form as a bio, and make it unclear which fields are published.
 *
 * None of this was editable before today. UpdateOrganizerDTO existed in the
 * model with no repository function and no route behind it, so an organiser
 * who mistyped their business name at sign-up had no way to correct it.
 */

interface OrganizerAccount {
  id: string;
  slug: string;
  business_name: string;
  business_email: string;
  business_phone: string;
  website_url?: string | null;
  verified: boolean;
  status: string;
  kyc_level?: number;
  created_at: string;
}

export default function AccountSettingsPage() {
  const toast = useToast();

  const [account, setAccount] = useState<OrganizerAccount | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [website, setWebsite] = useState('');

  const load = useCallback(async () => {
    try {
      const { data } = await api.get<OrganizerAccount>('/organiser/profile');
      setAccount(data);
      setName(data.business_name ?? '');
      setEmail(data.business_email ?? '');
      setPhone(data.business_phone ?? '');
      setWebsite(data.website_url ?? '');
    } catch {
      toast('Could not load your account', 'error');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => { load(); }, [load]);

  const dirty = !!account && (
    name !== (account.business_name ?? '') ||
    email !== (account.business_email ?? '') ||
    phone !== (account.business_phone ?? '') ||
    website !== (account.website_url ?? '')
  );

  const nameChanged = !!account && name.trim() !== (account.business_name ?? '');

  async function save() {
    if (name.trim().length < 2) { toast('Business name is too short', 'error'); return; }

    setSaving(true);
    try {
      // Sparse patch — only what actually changed. Sending everything would
      // overwrite a field edited in another tab with whatever this form was
      // holding when it loaded.
      const patch: Record<string, string> = {};
      if (name !== account?.business_name)     patch.business_name  = name.trim();
      if (email !== account?.business_email)   patch.business_email = email.trim();
      if (phone !== account?.business_phone)   patch.business_phone = phone.trim();
      if (website !== (account?.website_url ?? '')) patch.website_url = website.trim();

      const { data } = await api.patch<OrganizerAccount>('/organiser/profile', patch);
      setAccount(data);
      toast('Account updated', 'success');
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } };
      toast(err.response?.data?.error ?? 'Could not save', 'error');
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <div className="page-loader"><div className="spinner spinner-lg" /></div>;

  if (!account) {
    return (
      <div style={{ padding: 24 }}>
        <div className="alert alert-danger">Could not load your account. Refresh to try again.</div>
      </div>
    );
  }

  return (
    <div style={{ padding: '1.5rem', maxWidth: 720 }}>
      <div style={{ marginBottom: 20 }}>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600 }}>
          Account settings
        </div>
        <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: 2, maxWidth: 560 }}>
          Your business details. None of this appears on your public page —
          that is under Public Page.
        </div>
      </div>

      <section className="card" style={{ marginBottom: 18 }}>
        <div className="card-header"><div className="card-title">Business details</div></div>
        <div className="card-body">

          <div className="field">
            <label>Business name</label>
            <input value={name} maxLength={255} onChange={e => setName(e.target.value)} />
            {nameChanged ? (
              <div className="alert alert-info" style={{ marginTop: 10 }}>
                <Info size={15} weight="fill" />
                <div>
                  Your page address stays <code>/organiser/{account.slug}</code>.
                  It is set once when you register and does not follow a rename,
                  so links you have already shared keep working.
                </div>
              </div>
            ) : (
              <small className="field-hint">
                Shown to attendees as the organiser of your events.
              </small>
            )}
          </div>

          <div className="field">
            <label>Contact email</label>
            <input type="email" value={email} onChange={e => setEmail(e.target.value)} />
            <small className="field-hint">
              Where we reach you about payouts, reviews and support. Not shown
              to attendees.
            </small>
          </div>

          <div className="field">
            <label>Contact phone</label>
            <input value={phone} maxLength={32} onChange={e => setPhone(e.target.value)} />
            <small className="field-hint">Not shown to attendees.</small>
          </div>

          <div className="field">
            <label>Website <small style={{ fontWeight: 400, color: 'var(--text-3)' }}>(optional)</small></label>
            <input value={website} maxLength={500} placeholder="yourdomain.co.za"
              onChange={e => setWebsite(e.target.value)} />
            <small className="field-hint">
              To show a website link on your public page, add it under Public
              Page → Links instead. This one is for our records.
            </small>
          </div>

          <div style={{ display: 'flex', gap: 10, alignItems: 'center', marginTop: 6 }}>
            <button type="button" className="btn btn-primary" disabled={!dirty || saving} onClick={save}>
              {saving ? 'Saving…' : 'Save changes'}
            </button>
            {dirty && !saving && (
              <span style={{ fontSize: 12, color: 'var(--text-3)' }}>Unsaved changes</span>
            )}
          </div>
        </div>
      </section>

      {/* ── Read-only ────────────────────────────────────────────────────── */}
      <section className="card">
        <div className="card-header"><div className="card-title">Set by us</div></div>
        <div className="card-body">
          <div className="field-hint" style={{ marginBottom: 14 }}>
            These are not editable here. Verification status comes from identity
            review, and your page address is fixed so shared links keep working.
          </div>
          <table className="meta-table">
            <tbody>
              <tr>
                <th>Page address</th>
                <td><code>/organiser/{account.slug}</code></td>
              </tr>
              <tr>
                <th>Verification</th>
                <td>
                  {account.verified
                    ? 'Verified'
                    : <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                        <Warning size={13} weight="fill" style={{ color: 'var(--accent)' }} />
                        Not verified — see Verification &amp; Payouts
                      </span>}
                </td>
              </tr>
              <tr>
                <th>Account status</th>
                <td>{account.status}</td>
              </tr>
              <tr>
                <th>Organiser since</th>
                <td>{new Date(account.created_at).getFullYear()}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}