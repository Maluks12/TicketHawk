'use client';
import { useCallback, useEffect, useState } from 'react';
import { isBackdropPress } from '../../../lib/backdrop';
import { Warning, Info } from '@phosphor-icons/react';
import { api, clearSession } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { fmtMoney } from '../../../lib/utils';

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

      <DeleteAccountSection />
    </div>
  );
}

// ── Delete account ────────────────────────────────────────────────────────────
//
// Uses the same self-service endpoints as the TicketHawk app and web-mobile
// (handlers/account_deletion_handler.go):
//
//   GET    /user/me/deletion-preview  → can_delete, organiser_hold, counts
//   DELETE /user/me                   → { password, acknowledge_forfeit, reason }
//
// The backend owns every rule. It refuses while the organiser has live events
// or unsettled payouts (409 organiser_hold), asks for a second confirmation
// when the account holds upcoming tickets (409 tickets_will_be_lost), and
// re-checks the password (401 password_incorrect). This screen only explains
// those answers — it never decides deletability itself, so the dashboard and
// the apps cannot disagree about whether an account can go.
//
// It is ONE TicketHawk account: deleting it here also removes the attendee
// side (tickets, app sign-in), which is why the copy says so plainly.

interface DeletionPreview {
  can_delete: boolean;
  organiser_hold: boolean;
  upcoming_tickets: number;
  upcoming_value: number;
  live_events: number;
  unsettled_payouts: number;
}

type ApiErr = {
  response?: {
    status?: number;
    data?: {
      error?: string; code?: string;
      upcoming_tickets?: number; upcoming_value?: number;
      live_events?: number; unsettled_payouts?: number;
    };
  };
};

function DeleteAccountSection() {
  const [preview, setPreview] = useState<DeletionPreview | null>(null);
  const [previewFailed, setPreviewFailed] = useState(false);
  const [open, setOpen] = useState(false);

  const [password, setPassword] = useState('');
  const [reason, setReason] = useState('');
  const [ackTickets, setAckTickets] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadPreview = useCallback(async () => {
    try {
      const { data } = await api.get<DeletionPreview>('/user/me/deletion-preview');
      setPreview(data);
      setPreviewFailed(false);
    } catch {
      setPreviewFailed(true);
    }
  }, []);

  useEffect(() => { loadPreview(); }, [loadPreview]);

  function openDialog() {
    setPassword(''); setReason(''); setAckTickets(false); setError(null);
    setOpen(true);
  }

  const hasTickets = (preview?.upcoming_tickets ?? 0) > 0;
  const canSubmit = password.length > 0 && (!hasTickets || ackTickets) && !busy;

  async function confirmDelete() {
    if (!canSubmit) return;
    setBusy(true);
    setError(null);
    try {
      await api.delete('/user/me', {
        data: {
          password,
          acknowledge_forfeit: ackTickets,
          reason: reason.trim() || undefined,
        },
      });
      // The server has already revoked every refresh token. Drop the local
      // session and hard-navigate so no in-memory state survives the account.
      clearSession();
      window.location.replace('/login');
    } catch (e: unknown) {
      const res = (e as ApiErr).response;
      const body = res?.data ?? {};
      if (res?.status === 409 && body.code === 'tickets_will_be_lost') {
        // Tickets appeared since the preview loaded — refresh it so the
        // acknowledgement checkbox shows, and ask again.
        setPreview(p => p ? {
          ...p,
          upcoming_tickets: body.upcoming_tickets ?? p.upcoming_tickets,
          upcoming_value: body.upcoming_value ?? p.upcoming_value,
        } : p);
        setAckTickets(false);
        setError('You still have tickets to upcoming events. Tick the box below to confirm you understand they will be lost.');
      } else if (res?.status === 409 && body.code === 'organiser_hold') {
        setOpen(false);
        loadPreview();
      } else if (body.code === 'reauth_social_required') {
        setError('This account signs in with Google or Apple. Delete it from the TicketHawk app under Profile → Delete account.');
      } else {
        setError(body.error ?? 'Could not delete your account. Try again.');
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="card" style={{ marginTop: 18, borderColor: 'var(--danger-border)' }}>
      <div className="card-header">
        <div className="card-title" style={{ color: 'var(--danger-fg)' }}>Delete account</div>
      </div>
      <div className="card-body">
        <p style={{ fontSize: 13.5, color: 'var(--text-2)', lineHeight: 1.6, marginBottom: 14 }}>
          Permanently deletes your TicketHawk account — this organiser dashboard
          <strong> and </strong>your sign-in on the TicketHawk app and website,
          including any tickets you hold. Your personal details are erased;
          financial records (bookings, payments, payouts) are kept but no
          longer linked to you. This cannot be undone.
        </p>

        {previewFailed && (
          <div className="alert alert-warning">
            <span>Could not check whether your account can be deleted right now. Refresh to try again.</span>
          </div>
        )}

        {preview?.organiser_hold && (
          <div className="alert alert-warning">
            <Warning size={16} weight="fill" style={{ flexShrink: 0, marginTop: 2 }} />
            <div>
              <strong>Not yet — your organiser account is still active.</strong>{' '}
              {preview.live_events > 0 && (
                <>You have {preview.live_events} live {preview.live_events === 1 ? 'event' : 'events'}; cancel or complete {preview.live_events === 1 ? 'it' : 'them'}. </>
              )}
              {preview.unsettled_payouts > 0 && (
                <>{preview.unsettled_payouts} {preview.unsettled_payouts === 1 ? 'payout is' : 'payouts are'} still settling; wait for {preview.unsettled_payouts === 1 ? 'it' : 'them'} to clear. </>
              )}
              Buyers hold tickets to your events and money may still be owed, so
              the account has to stay until those are finished.
            </div>
          </div>
        )}

        <button type="button" className="btn btn-danger"
          disabled={!preview || preview.organiser_hold}
          onClick={openDialog}>
          Delete my account
        </button>
      </div>

      {open && preview && (
        <div className="modal-overlay" onMouseDown={(e) => {
          if (isBackdropPress(e) && !busy) setOpen(false);
        }}>
          <div className="modal modal-sm" role="dialog" aria-modal="true" aria-labelledby="del-title">
            <div className="modal-header">
              <div className="modal-title" id="del-title">Delete your account?</div>
            </div>
            <div className="modal-body">
              <div className="alert alert-danger" style={{ fontSize: 13 }}>
                <span>
                  This removes your organiser dashboard and your TicketHawk app
                  sign-in for good. It cannot be undone.
                </span>
              </div>

              {hasTickets && (
                <label className="check-row" style={{ alignItems: 'flex-start', marginBottom: 16 }}>
                  <input type="checkbox" checked={ackTickets}
                    onChange={(e) => setAckTickets(e.target.checked)} style={{ marginTop: 2 }} />
                  <span style={{ fontSize: 13, lineHeight: 1.5 }}>
                    I understand my {preview.upcoming_tickets} upcoming{' '}
                    {preview.upcoming_tickets === 1 ? 'ticket' : 'tickets'}
                    {preview.upcoming_value > 0 ? ` (worth ${fmtMoney(preview.upcoming_value)})` : ''}{' '}
                    will be lost.
                  </span>
                </label>
              )}

              <div className="field">
                <label htmlFor="del-password">Your password</label>
                <input id="del-password" type="password" autoComplete="current-password"
                  value={password} onChange={(e) => setPassword(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Enter') confirmDelete(); }} />
                <div className="field-hint">Confirms it is really you.</div>
              </div>

              <div className="field" style={{ marginBottom: 0 }}>
                <label htmlFor="del-reason">Why are you leaving? <span style={{ textTransform: 'none', fontWeight: 400 }}>(optional)</span></label>
                <textarea id="del-reason" rows={2} maxLength={500}
                  value={reason} onChange={(e) => setReason(e.target.value)} />
              </div>

              {error && (
                <div className="alert alert-danger" style={{ marginTop: 14, marginBottom: 0, fontSize: 13 }}>
                  <span>{error}</span>
                </div>
              )}
            </div>
            <div className="modal-footer">
              <button type="button" className="btn" disabled={busy} onClick={() => setOpen(false)}>
                Keep my account
              </button>
              <button type="button" className="btn btn-danger" disabled={!canSubmit} onClick={confirmDelete}>
                {busy ? 'Deleting…' : 'Delete permanently'}
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}