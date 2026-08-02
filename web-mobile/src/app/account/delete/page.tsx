'use client';

// ─────────────────────────────────────────────────────────────────────────────
// /account/delete
//
// The URL given to Google Play as the app's account-deletion link.
//
// Play requires that a user who can create an account can also DELETE it, and
// specifically that they can start that request from the WEB — without having
// to reinstall the app they have already removed from their phone. That last
// part is why a static "here's how to do it in the app" page is not enough and
// this one actually performs the deletion.
//
// It calls exactly the same endpoint as the in-app screen
// (DELETE /v1/user/me), so there is one deletion path, one set of rules, and no
// possibility of the two drifting apart:
//
//   • password re-authentication  — a live session is not authority enough to
//     destroy an account; a stolen one must not be
//   • organiser hold (409)        — live events or unsettled payouts block it
//   • upcoming tickets (409)      — warn once, proceed on explicit confirmation
//
// Apple's equivalent requirement (Guideline 5.1.1(v)) is satisfied by the
// in-app screen; this page is the Play-facing counterpart and a courtesy to
// anyone who prefers a keyboard.
// ─────────────────────────────────────────────────────────────────────────────

import { useCallback, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { AlertCircle, Trash2, FileText, XCircle, Ticket } from 'lucide-react';
import { api } from '@/lib/api';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';

const DANGER = '#ff4d4d';

interface DeletionPreview {
  can_delete: boolean;
  organiser_hold: boolean;
  upcoming_tickets: number;
  upcoming_value: number;
  live_events: number;
  unsettled_payouts: number;
}

const rand = (n: number) => `R${Math.round(n).toLocaleString('en-ZA')}`;

export default function DeleteAccountPage() {
  const router = useRouter();
  const { user, logout } = useAuthStore();
  const { colors } = useAppTheme();

  const [preview, setPreview] = useState<DeletionPreview | null>(null);
  const [loading, setLoading] = useState(true);
  const [password, setPassword] = useState('');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  // ── Preview ────────────────────────────────────────────────────────────────
  useEffect(() => {
    if (!user) { setLoading(false); return; }
    let cancelled = false;
    (async () => {
      try {
        const { data } = await api.get<DeletionPreview>('/v1/user/me/deletion-preview');
        if (!cancelled) setPreview(data);
      } catch {
        // A failed preview must never block deletion. The server re-checks
        // everything on the DELETE, so the worst case is a generic warning
        // instead of a specific one.
        if (!cancelled) setPreview(null);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [user]);

  // ── Delete ─────────────────────────────────────────────────────────────────
  const doDelete = useCallback(async (acknowledgeForfeit: boolean) => {
    setBusy(true);
    setError(null);
    try {
      await api.delete('/v1/user/me', {
        data: {
          password,
          acknowledge_forfeit: acknowledgeForfeit,
          reason: reason.trim() || undefined,
        },
      });
      await logout().catch(() => {});
      setDone(true);
    } catch (e: any) {
      const status = e?.response?.status;
      const body = e?.response?.data ?? {};

      if (status === 409 && body.code === 'tickets_will_be_lost') {
        setBusy(false);
        const ok = window.confirm(
          `You have ${body.upcoming_tickets} ticket${body.upcoming_tickets === 1 ? '' : 's'}` +
          `${body.upcoming_value ? ` worth ${rand(body.upcoming_value)}` : ''} for events that ` +
          `haven't happened yet.\n\nDeleting your account means you lose access to them and they ` +
          `will not be refunded. If you want your money back, cancel the bookings first.\n\n` +
          `Delete anyway?`,
        );
        if (ok) void doDelete(true);
        return;
      }

      if (status === 401 && body.code === 'password_incorrect') {
        setError('That password is incorrect.');
      } else if (status === 401) {
        setError('Your session has expired. Please sign in again.');
      } else if (status === 409 && body.code === 'organiser_hold') {
        setError(body.error ?? 'Your organiser account is still active.');
      } else {
        setError(body.error ?? 'Could not delete your account. Please try again.');
      }
      setBusy(false);
    }
  }, [password, reason, logout]);

  const confirmThenDelete = useCallback(() => {
    if (!password) { setError('Enter your password to continue.'); return; }
    const ok = window.confirm(
      'Delete your account?\n\nThis cannot be undone. Your name, email address and ' +
      'phone number will be permanently removed, and you will be signed out on every device.',
    );
    if (ok) void doDelete(false);
  }, [password, doDelete]);

  const card: React.CSSProperties = {
    backgroundColor: colors.sectionCardBg,
    borderColor: colors.dividerColor,
  };
  const input: React.CSSProperties = {
    backgroundColor: colors.background,
    borderColor: colors.dividerColor,
    color: colors.text,
  };

  // ── Done ───────────────────────────────────────────────────────────────────
  if (done) {
    return (
      <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>
        <div className="max-w-[560px] mx-auto px-5 py-20 text-center">
          <h1 className="text-xl font-extrabold mb-3" style={{ color: colors.text }}>
            Your account has been deleted
          </h1>
          <p className="text-sm leading-6 mb-8" style={{ color: colors.subText }}>
            Your name, email address and phone number have been permanently removed.
            Thank you for using TicketHawk.
          </p>
          <button
            onClick={() => router.push('/')}
            className="px-6 py-3 rounded-[14px] text-sm font-bold text-white"
            style={{ backgroundColor: '#ff2d55' }}
          >
            Back to TicketHawk
          </button>
        </div>
      </div>
    );
  }

  // ── Signed out ─────────────────────────────────────────────────────────────
  if (!loading && !user) {
    return (
      <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>
        <div className="max-w-[560px] mx-auto px-5 py-20 text-center">
          <h1 className="text-xl font-extrabold mb-3" style={{ color: colors.text }}>
            Sign in to delete your account
          </h1>
          <p className="text-sm leading-6 mb-8" style={{ color: colors.subText }}>
            We need to confirm it's really you before removing your account.
            You don't need to reinstall the app — you can do this here.
          </p>
          <button
            onClick={() => router.push('/auth/login?next=/account/delete')}
            className="px-6 py-3 rounded-[14px] text-sm font-bold text-white"
            style={{ backgroundColor: '#ff2d55' }}
          >
            Sign in
          </button>
          <p className="text-xs mt-8" style={{ color: colors.subText }}>
            Can't sign in? Email{' '}
            <a href="mailto:support@ticket-hawk.com" style={{ color: '#ff2d55', fontWeight: 700 }}>
              support@ticket-hawk.com
            </a>{' '}
            from your account's address and we'll delete it for you.
          </p>
        </div>
      </div>
    );
  }

  const held = preview?.organiser_hold === true;

  return (
    <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>
      <div className="max-w-[560px] mx-auto px-5 py-10">
        <h1 className="text-2xl font-extrabold mb-2" style={{ color: colors.text }}>
          Delete account
        </h1>
        <p className="text-sm mb-8" style={{ color: colors.subText }}>
          Signed in as {user?.email}
        </p>

        {loading ? (
          <p className="text-sm" style={{ color: colors.subText }}>Loading…</p>
        ) : (
          <>
            {/* What happens */}
            <div className="rounded-[14px] border p-5 mb-5" style={card}>
              <p className="text-sm font-extrabold mb-3" style={{ color: colors.text }}>
                What gets deleted
              </p>
              {[
                'Your name, email address and phone number',
                'Your saved events and notification history',
                'Every signed-in device is signed out',
              ].map((line) => (
                <div key={line} className="flex gap-2.5 items-start mb-2">
                  <XCircle size={15} color={DANGER} className="shrink-0 mt-0.5" />
                  <span className="text-[13px] leading-5" style={{ color: colors.subText }}>{line}</span>
                </div>
              ))}

              <p className="text-sm font-extrabold mt-5 mb-3" style={{ color: colors.text }}>
                What we keep
              </p>
              <div className="flex gap-2.5 items-start">
                <FileText size={15} color={colors.subText} className="shrink-0 mt-0.5" />
                <span className="text-[13px] leading-5" style={{ color: colors.subText }}>
                  Records of past ticket purchases, with your personal details removed.
                  South African tax and consumer protection law requires us to keep proof
                  of sale, and event organisers need it for their own settlement records.
                </span>
              </div>
            </div>

            {/* Organiser hold */}
            {held && (
              <div
                className="rounded-[14px] border p-4 mb-5 flex gap-3"
                style={{ backgroundColor: 'rgba(255,184,48,0.10)', borderColor: 'rgba(255,184,48,0.35)' }}
              >
                <AlertCircle size={18} color="#ffb830" className="shrink-0 mt-0.5" />
                <div>
                  <p className="text-sm font-bold mb-1" style={{ color: colors.text }}>
                    Organiser account active
                  </p>
                  <p className="text-[13px] leading-5" style={{ color: colors.subText }}>
                    {preview!.live_events > 0 &&
                      `You have ${preview!.live_events} live event${preview!.live_events === 1 ? '' : 's'} with ticket holders. `}
                    {preview!.unsettled_payouts > 0 &&
                      `You have ${preview!.unsettled_payouts} payout${preview!.unsettled_payouts === 1 ? '' : 's'} still settling. `}
                    Finish or cancel these first — we can't remove the organiser behind an
                    event that people have bought tickets to.{' '}
                    <a href="mailto:support@ticket-hawk.com" style={{ color: '#ffb830', fontWeight: 700 }}>
                      support@ticket-hawk.com
                    </a>
                  </p>
                </div>
              </div>
            )}

            {/* Upcoming tickets */}
            {!held && (preview?.upcoming_tickets ?? 0) > 0 && (
              <div
                className="rounded-[14px] border p-4 mb-5 flex gap-3"
                style={{ backgroundColor: 'rgba(255,77,77,0.09)', borderColor: 'rgba(255,77,77,0.32)' }}
              >
                <Ticket size={18} color={DANGER} className="shrink-0 mt-0.5" />
                <div>
                  <p className="text-sm font-bold mb-1" style={{ color: colors.text }}>
                    You have {preview!.upcoming_tickets} upcoming ticket
                    {preview!.upcoming_tickets === 1 ? '' : 's'}
                  </p>
                  <p className="text-[13px] leading-5" style={{ color: colors.subText }}>
                    Worth {rand(preview!.upcoming_value)}. Deleting your account means losing
                    access to them, and they will not be refunded. Cancel the bookings from
                    your Tickets page first if you want your money back.
                  </p>
                </div>
              </div>
            )}

            {!held && (
              <>
                <label className="block text-[10px] font-black tracking-[1.5px] mb-2" style={{ color: colors.subText }}>
                  CONFIRM YOUR PASSWORD
                </label>
                <input
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => { setPassword(e.target.value); setError(null); }}
                  placeholder="Your password"
                  className="w-full rounded-[12px] border px-4 py-3 text-sm mb-5 outline-none"
                  style={{ ...input, borderColor: error ? DANGER : colors.dividerColor }}
                />

                <label className="block text-[10px] font-black tracking-[1.5px] mb-2" style={{ color: colors.subText }}>
                  WHY ARE YOU LEAVING? (OPTIONAL)
                </label>
                <textarea
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  maxLength={500}
                  rows={3}
                  placeholder="This helps us improve TicketHawk"
                  className="w-full rounded-[12px] border px-4 py-3 text-sm outline-none resize-none"
                  style={input}
                />

                {error && (
                  <div className="flex gap-2 items-center mt-4">
                    <AlertCircle size={14} color={DANGER} className="shrink-0" />
                    <span className="text-[13px]" style={{ color: DANGER }}>{error}</span>
                  </div>
                )}

                <button
                  onClick={confirmThenDelete}
                  disabled={busy || !password}
                  className="w-full mt-6 py-3.5 rounded-[14px] text-[15px] font-extrabold text-white flex items-center justify-center gap-2 disabled:opacity-45"
                  style={{ backgroundColor: DANGER }}
                >
                  <Trash2 size={17} />
                  {busy ? 'Deleting…' : 'Delete my account'}
                </button>

                <button
                  onClick={() => router.push('/profile')}
                  className="w-full mt-3 py-2 text-sm font-semibold"
                  style={{ color: colors.subText }}
                >
                  Keep my account
                </button>
              </>
            )}

            {held && (
              <button
                onClick={() => router.push('/profile')}
                className="w-full mt-2 py-2 text-sm font-semibold"
                style={{ color: colors.subText }}
              >
                Go back
              </button>
            )}
          </>
        )}
      </div>
    </div>
  );
}