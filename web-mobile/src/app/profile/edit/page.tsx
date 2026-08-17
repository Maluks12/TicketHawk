'use client';

// ─────────────────────────────────────────────────────────────────────────────
// /profile/edit
//
// Backed by PATCH /v1/user/me, which is a SPARSE patch: only fields the user
// actually changed are sent, and anything omitted is left untouched
// server-side. That matters because a blanket submit of every field —
// including ones the user never touched — could clobber data with a stale
// copy if two tabs are open, or just be needless traffic.
//
// ── On changing the email address ────────────────────────────────────────────
//
// Email is the login identifier, so it's treated differently from the other
// fields. The server resets email_verified to FALSE on change, so the user
// must confirm the new address — we warn before submitting rather than
// after, since a typo here can lock someone out of their own account.
//
// Changing it also requires the current password. A valid access token isn't
// authority enough to move the login identifier: without re-authentication,
// anyone holding a stolen session could point the account at their own
// address and take it over via "forgot password". The password field below
// only appears once the email is actually being changed — demanding it to
// fix a typo in a surname is friction with no threat behind it.
//
// Mirrors EventsApp/app/profile/edit.tsx so the web and native flows share
// the same rules against the same endpoint.
// ─────────────────────────────────────────────────────────────────────────────

import { useCallback, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { AlertCircle, Check } from 'lucide-react';
import { updateMyProfile } from '@/lib/api';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';

const SA_NATIONAL_DIGITS = 9;

/** Digits only, dropping a +27 / 27 / leading-0 prefix. Mirrors auth/login. */
function phoneNationalDigits(raw: string): string {
  let d = raw.replace(/\D/g, '');
  if (d.startsWith('27') && d.length > SA_NATIONAL_DIGITS) d = d.slice(2);
  if (d.startsWith('0')) d = d.slice(1);
  return d;
}

const phoneIsValid = (raw: string) => phoneNationalDigits(raw).length === SA_NATIONAL_DIGITS;
const phoneToLocal = (raw: string) => {
  const d = phoneNationalDigits(raw);
  return d ? `0${d}` : '';
};

const emailIsValid = (v: string) => /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(v.trim());

export default function EditProfilePage() {
  const router = useRouter();
  const { user, applyUser } = useAuthStore();
  const { colors } = useAppTheme();

  const [firstName, setFirstName] = useState(user?.first_name ?? '');
  const [lastName, setLastName] = useState(user?.last_name ?? '');
  const [phone, setPhone] = useState(user?.phone_number ?? '');

  // ── Password vs social accounts ────────────────────────────────────────────
  //
  // Since migration 000075 an account can exist with no password — one created
  // through Google or Apple. Two things on this screen assumed otherwise, and
  // both were unclearable dead ends for those users:
  //
  //   • Phone validation ran unconditionally. A social account starts with
  //     phone_number NULL, so the field initialised blank, failed validation,
  //     and blocked EVERY save — including simply correcting a first name —
  //     with "Enter a valid number" on a field they were never asked to fill.
  //
  //   • An email change demanded the current password. There isn't one.
  //
  // On the web the provider flow lives on the login page, so rather than
  // re-running it inline here, an account with no password is told to set one
  // first via password reset — which works fine, because a NULL password does
  // not block a reset link. Fewer moving parts than embedding Apple's JS SDK
  // on the profile screen for a rare action.
  const hasPassword = (user as any)?.has_password !== false;
  const [email, setEmail] = useState(user?.email ?? '');
  const [password, setPassword] = useState('');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [confirmingEmail, setConfirmingEmail] = useState(false);

  // Only changed fields are sent. Comparing against the loaded user also
  // means an untouched form submits nothing.
  const patch = useMemo(() => {
    const p: Record<string, string> = {};
    if (firstName.trim() !== (user?.first_name ?? '')) p.first_name = firstName.trim();
    if (lastName.trim() !== (user?.last_name ?? '')) p.last_name = lastName.trim();
    if (phoneToLocal(phone) !== (user?.phone_number ?? '')) p.phone_number = phoneToLocal(phone);
    if (email.trim().toLowerCase() !== (user?.email ?? '').toLowerCase()) p.email = email.trim().toLowerCase();
    return p;
  }, [firstName, lastName, phone, email, user]);

  const dirty = Object.keys(patch).length > 0;
  const emailChanging = 'email' in patch;

  const validate = useCallback(() => {
    const e: Record<string, string> = {};
    if (!firstName.trim()) e.first_name = 'Required';
    if (!lastName.trim()) e.last_name = 'Required';
    // Only validate a phone number if there IS one, or the user is entering one
    // now. A blank field on an account that never had a number is the normal
    // state of a social account, not an error.
    if (phone.trim() && !phoneIsValid(phone)) e.phone_number = 'Enter a valid number';
    if (!emailIsValid(email)) e.email = 'Enter a valid email address';
    // Only demanded when the identifier itself is moving.
    // Only demanded from accounts that actually have a password.
    if (emailChanging && hasPassword && !password) e.current_password = 'Enter your current password';
    if (emailChanging && !hasPassword) {
      e.current_password =
        'Set a password before changing your email address \u2014 use "Forgot password" on the sign-in page.';
    }
    setErrors(e);
    return Object.keys(e).length === 0;
  }, [firstName, lastName, phone, email, emailChanging, password, hasPassword]);

  const save = useCallback(async () => {
    if (!validate() || !dirty) return;
    setSaving(true);
    try {
      const body = emailChanging && hasPassword
        ? { ...patch, current_password: password }
        : patch;
      const { data } = await updateMyProfile(body);
      if (data?.user) applyUser(data.user);
      setPassword('');
      setConfirmingEmail(false);

      if (data?.email_changed) {
        window.alert(
          "We\u2019ve sent a confirmation link to your new address. Until you confirm it, some features may be limited.",
        );
      }
      setSaved(true);
      setTimeout(() => router.push('/profile'), 700);
    } catch (err: any) {
      const status = err?.response?.status;
      const code = err?.response?.data?.code;
      const raw = err?.response?.data;
      // Every error path on the server returns { error: string }. If this is
      // undefined here, the response never carried that shape at all — most
      // often because the request never reached our handler (backend not
      // rebuilt after this route was added, backend not running, or a proxy/
      // 404 in between), not because of anything the user typed. Logging the
      // raw response makes that distinction visible in devtools instead of
      // always showing the same generic banner.
      if (process.env.NODE_ENV !== 'production') {
        // eslint-disable-next-line no-console
        console.error('[edit-profile] PATCH /v1/user/me failed', { status, data: raw, err });
      }
      const msg = (raw && typeof raw === 'object' ? raw.error : undefined)
        ?? 'Could not save your changes. Please try again.';
      // 409 is the "email already in use" case and belongs on the field
      // rather than in a banner the user has to parse.
      if (status === 409) setErrors((p) => ({ ...p, email: msg }));
      else if (code === 'password_incorrect' || code === 'password_required'
               || code === 'reauth_social_required') {
        setErrors((p) => ({ ...p, current_password: msg }));
      } else setErrors((p) => ({ ...p, form: msg }));
    } finally {
      setSaving(false);
    }
  }, [validate, dirty, patch, emailChanging, password, hasPassword, applyUser, router]);

  const onSavePress = useCallback(() => {
    if (!emailChanging) { void save(); return; }
    // Warn BEFORE the change, not after — a typo in an email field is the
    // one mistake on this screen that can cost someone their account.
    setConfirmingEmail(true);
  }, [emailChanging, save]);

  const input: React.CSSProperties = {
    backgroundColor: colors.background,
    borderColor: colors.dividerColor,
    color: colors.text,
  };

  const field = (
    label: string,
    value: string,
    onChange: (v: string) => void,
    key: string,
    opts: Partial<React.InputHTMLAttributes<HTMLInputElement>> = {},
    hint?: string,
  ) => (
    <div className="mb-5">
      <label className="block text-[10px] font-black tracking-[1.5px] mb-2" style={{ color: colors.subText }}>
        {label.toUpperCase()}
      </label>
      <input
        value={value}
        onChange={(e) => { onChange(e.target.value); setErrors((p) => ({ ...p, [key]: '' })); }}
        className="w-full rounded-[12px] border px-4 py-3 text-sm outline-none"
        style={{ ...input, borderColor: errors[key] ? '#ff4d4d' : colors.dividerColor }}
        {...opts}
      />
      {!!errors[key] && (
        <p className="text-[12.5px] mt-1.5" style={{ color: '#ff4d4d' }}>{errors[key]}</p>
      )}
      {!errors[key] && !!hint && (
        <p className="text-xs mt-1.5" style={{ color: colors.subText }}>{hint}</p>
      )}
    </div>
  );

  if (!user) {
    return (
      <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>
        <div className="max-w-[560px] mx-auto px-5 py-20 text-center">
          <h1 className="text-xl font-extrabold mb-3" style={{ color: colors.text }}>
            Sign in to edit your profile
          </h1>
          <button
            onClick={() => router.push('/auth/login?next=/profile/edit')}
            className="px-6 py-3 rounded-[14px] text-sm font-bold text-white"
            style={{ backgroundColor: '#ff2d55' }}
          >
            Sign in
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen" style={{ backgroundColor: colors.background }}>
      {/* Header */}
      <div
        className="sticky top-0 z-10 flex items-center px-4 py-3 border-b"
        style={{ backgroundColor: colors.background, borderColor: colors.dividerColor }}
      >
        <button onClick={() => router.back()} className="w-10 h-10 flex items-center justify-center rounded-full">
          <span style={{ color: colors.text }}>←</span>
        </button>
        <div className="flex-1 text-center">
          <p className="font-extrabold text-lg" style={{ color: colors.text }}>Edit profile</p>
        </div>
        <div className="w-10" />
      </div>

      <div className="max-w-[560px] mx-auto px-5 py-8">
        {field('First name', firstName, setFirstName, 'first_name', {
          autoComplete: 'given-name',
        })}
        {field('Last name', lastName, setLastName, 'last_name', {
          autoComplete: 'family-name',
        })}
        {field('Phone number', phone, setPhone, 'phone_number', {
          type: 'tel', autoComplete: 'tel',
        }, 'South African mobile — with or without the leading 0')}
        {field('Email', email, setEmail, 'email', {
          type: 'email', autoComplete: 'email',
        }, emailChanging ? "You'll need to confirm this new address" : 'You sign in with this address')}

        {/* Only rendered for accounts that have a password. A social account
            gets the validation message instead, which points at password reset
            — rendering an unfillable box for them was the dead end this
            replaces. */}
        {emailChanging && hasPassword && field('Current password', password, setPassword, 'current_password', {
          type: 'password', autoComplete: 'current-password',
        }, 'Required to change the address you sign in with')}

        {confirmingEmail && emailChanging && (
          <div
            className="rounded-[14px] border p-4 mb-5"
            style={{ backgroundColor: 'rgba(255,184,48,0.10)', borderColor: 'rgba(255,184,48,0.35)' }}
          >
            <p className="text-sm font-bold mb-1" style={{ color: colors.text }}>
              Change your email address?
            </p>
            <p className="text-[13px] leading-5 mb-3" style={{ color: colors.subText }}>
              {`You\u2019ll sign in with ${email.trim().toLowerCase()} from now on, and you\u2019ll need to confirm it before your account is fully verified.`}
            </p>
            <div className="flex gap-2">
              <button
                onClick={() => setConfirmingEmail(false)}
                className="px-4 py-2 rounded-[10px] text-sm font-semibold"
                style={{ color: colors.subText, border: `1px solid ${colors.dividerColor}` }}
              >
                Cancel
              </button>
              <button
                onClick={() => void save()}
                disabled={saving}
                className="px-4 py-2 rounded-[10px] text-sm font-bold text-white disabled:opacity-60"
                style={{ backgroundColor: '#ff2d55' }}
              >
                {saving ? 'Changing…' : 'Change it'}
              </button>
            </div>
          </div>
        )}

        {errors.form && (
          <div className="flex gap-2 items-center mb-4">
            <AlertCircle size={14} color="#ff4d4d" className="shrink-0" />
            <span className="text-[13px]" style={{ color: '#ff4d4d' }}>{errors.form}</span>
          </div>
        )}

        <button
          onClick={onSavePress}
          disabled={!dirty || saving}
          className="w-full py-3.5 rounded-[14px] text-[15px] font-extrabold text-white flex items-center justify-center gap-2 disabled:opacity-45"
          style={{ backgroundColor: saved ? '#2ecc71' : '#ff2d55' }}
        >
          {saved ? (<><Check size={17} /> Saved</>) : saving ? 'Saving…' : dirty ? 'Save changes' : 'No changes'}
        </button>

        <button
          onClick={() => router.push('/profile')}
          className="w-full mt-3 py-2 text-sm font-semibold"
          style={{ color: colors.subText }}
        >
          Cancel
        </button>
      </div>
    </div>
  );
}