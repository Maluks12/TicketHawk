import React, { useState, useCallback, useMemo } from 'react';
import {
  View, Text, ScrollView, TextInput, TouchableOpacity,
  ActivityIndicator, Alert, KeyboardAvoidingView, Platform, StyleSheet,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Feather } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { api } from '../../lib/api';
import { useAuthStore } from '../../store/authStore';
import { useAppTheme } from '../../theme/colors';
import {
  signInWithApple, signInWithGoogle,
  SocialAuthCancelled, SocialAuthProviderError,
  type SocialCredential,
} from '../../lib/socialAuth';

// ── Edit profile ─────────────────────────────────────────────────────────────
//
// Backed by PATCH /v1/user/me, which is a SPARSE patch: only fields the user
// actually changed are sent, and anything omitted is left untouched server-side.
// That matters because a blanket PUT from a form with a blank optional field
// would clear stored data the user never intended to touch.
//
// ── On changing the email address ────────────────────────────────────────────
//
// Email is the login identifier, so it is treated differently from the other
// fields. The server resets email_verified to FALSE on change, which means the
// user must confirm the new address. We warn before submitting rather than
// after, because a typo here locks someone out of their own account.
//
// Changing it also requires the current password. A valid access token is not
// authority enough to move the login identifier: without re-authentication,
// anyone holding a stolen session could point the account at their own address
// and then use "forgot password" to take it over outright. The password field
// below appears ONLY when the email is actually being changed — demanding it to
// fix a typo in a surname is friction with no threat behind it, and prompts that
// cry wolf get dismissed on the day they matter.

const SA_NATIONAL_DIGITS = 9;

/** Digits only, dropping a +27 / 27 / leading-0 prefix. Mirrors auth/login.tsx. */
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

export default function EditProfileScreen() {
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();
  const { user, applyUser } = useAuthStore();

  const [firstName, setFirstName] = useState(user?.first_name ?? '');
  const [lastName,  setLastName]  = useState(user?.last_name ?? '');
  const [phone,     setPhone]     = useState(user?.phone_number ?? '');
  const [email,     setEmail]     = useState(user?.email ?? '');
  const [password,  setPassword]  = useState('');
  const [saving,    setSaving]    = useState(false);
  const [errors,    setErrors]    = useState<Record<string, string>>({});

  // ── Password vs social accounts ────────────────────────────────────────────
  //
  // Since migration 000075 an account can exist with no password at all — one
  // created through Google or Apple. Two things on this screen assumed every
  // account had one, and both were unclearable dead ends for those users:
  //
  //   • Phone validation ran unconditionally. A social account starts with
  //     phone_number NULL, so the field initialised blank, failed validation,
  //     and blocked EVERY save — including simply correcting a first name —
  //     with "Enter a valid number" on a field the user was never asked to
  //     fill and had no idea was required.
  //
  //   • An email change demanded the current password. There isn't one. The
  //     server now accepts a fresh provider token as equivalent proof (see
  //     handlers/reauth.go); this screen has to offer that instead of a field
  //     the user can never satisfy.
  //
  // has_password and linked_providers come from GET /v1/user/me.
  const hasPassword = (user as any)?.has_password !== false;
  const linkedProviders: string[] = (user as any)?.linked_providers ?? [];
  const socialProvider: 'apple' | 'google' | null =
    linkedProviders.includes('apple') ? 'apple'
    : linkedProviders.includes('google') ? 'google'
    : null;

  // Only changed fields are sent. Comparing against the loaded user also means
  // an untouched form submits nothing and the server replies "No changes".
  const patch = useMemo(() => {
    const p: Record<string, string> = {};
    if (firstName.trim() !== (user?.first_name ?? '')) p.first_name = firstName.trim();
    if (lastName.trim()  !== (user?.last_name  ?? '')) p.last_name  = lastName.trim();
    if (phoneToLocal(phone) !== (user?.phone_number ?? '')) p.phone_number = phoneToLocal(phone);
    if (email.trim().toLowerCase() !== (user?.email ?? '').toLowerCase()) p.email = email.trim().toLowerCase();
    return p;
  }, [firstName, lastName, phone, email, user]);

  const dirty = Object.keys(patch).length > 0;
  const emailChanging = 'email' in patch;

  const validate = useCallback(() => {
    const e: Record<string, string> = {};
    if (!firstName.trim()) e.first_name = 'Required';
    if (!lastName.trim())  e.last_name  = 'Required';
    // Validate the phone number only if there IS one, or the user is entering
    // one now. A blank field on an account that never had a number is not an
    // error — it is the normal state of a social account, and treating it as
    // one prevented those users from saving anything at all.
    if (phone.trim() && !phoneIsValid(phone)) e.phone_number = 'Enter a valid number';
    if (!emailIsValid(email)) e.email = 'Enter a valid email address';
    // Only demanded when the identifier itself is moving, AND only from
    // accounts that actually have a password. Social accounts confirm with
    // their provider instead — see confirmWithProvider below.
    if (emailChanging && hasPassword && !password) e.current_password = 'Enter your current password';
    setErrors(e);
    return Object.keys(e).length === 0;
  }, [firstName, lastName, phone, email, emailChanging, password, hasPassword]);

  // Re-runs the linked provider's sign-in sheet to prove the person at the
  // device still controls the account. This is the social equivalent of typing
  // your password, and the server rejects the token unless its subject is
  // already linked to this account.
  const confirmWithProvider = useCallback(async (): Promise<SocialCredential> => {
    if (socialProvider === 'apple') return signInWithApple();
    if (socialProvider === 'google') return signInWithGoogle();
    throw new SocialAuthProviderError(
      'This account has no password and no connected sign-in method. Use "Forgot password" to set one.',
    );
  }, [socialProvider]);

  const save = useCallback(async () => {
    if (!validate() || !dirty) return;
    setSaving(true);
    try {
      // Build the step-up credential for an email change. Password accounts
      // send current_password; social accounts re-run their provider flow and
      // send the fresh ID token, which the server checks is linked to THIS
      // account before accepting it.
      let body: Record<string, unknown> = { ...patch };
      if (emailChanging) {
        if (hasPassword) {
          body.current_password = password;
        } else {
          const cred = await confirmWithProvider();
          body.provider = cred.provider;
          body.id_token = cred.idToken;
          if (cred.nonce) body.nonce = cred.nonce;
        }
      }
      const { data } = await api.patch('/v1/user/me', body);
      if (data?.user) applyUser(data.user);
      setPassword('');

      if (data?.email_changed) {
        Alert.alert(
          'Check your email',
          'Your email address has changed. We\u2019ve sent a confirmation link to the new address — until you confirm it, some features may be limited.',
          [{ text: 'OK', onPress: () => router.back() }],
        );
      } else {
        router.back();
      }
    } catch (err: any) {
      // The user backed out of the Apple/Google sheet. Not an error, and an
      // alert here would be noise.
      if (err instanceof SocialAuthCancelled) { setSaving(false); return; }
      if (err instanceof SocialAuthProviderError) {
        Alert.alert('Could not confirm', err.message);
        setSaving(false);
        return;
      }
      const msg = err?.response?.data?.error ?? 'Could not save your changes. Please try again.';
      // 409 is the "email already in use" case and belongs on the field rather
      // than in an alert the user has to dismiss before they can fix it.
      const code = err?.response?.data?.code;
      if (err?.response?.status === 409) setErrors((p) => ({ ...p, email: msg }));
      else if (code === 'password_incorrect' || code === 'password_required') {
        // Belongs on the field, not in an alert the user must dismiss before
        // they can retype it.
        setErrors((p) => ({ ...p, current_password: msg }));
      } else if (code === 'reauth_wrong_account') {
        // Verified token, wrong person — they signed in with a different
        // Google/Apple account than the one linked here.
        Alert.alert('Wrong account', msg);
      } else Alert.alert('Something went wrong', msg);
    } finally {
      setSaving(false);
    }
  }, [validate, dirty, patch, emailChanging, password, hasPassword, confirmWithProvider, applyUser, router]);

  const onSavePress = useCallback(() => {
    if (!emailChanging) { save(); return; }
    // Warn BEFORE the change, not after. A typo in an email field is the one
    // mistake on this screen that can cost someone their account.
    Alert.alert(
      'Change your email address?',
      `You\u2019ll sign in with ${email.trim().toLowerCase()} from now on, and you\u2019ll need to confirm it before your account is fully verified.`,
      [
        { text: 'Cancel', style: 'cancel' },
        { text: 'Change it', style: 'destructive', onPress: save },
      ],
    );
  }, [emailChanging, email, save]);

  const field = (
    label: string,
    value: string,
    onChange: (v: string) => void,
    key: string,
    opts: Partial<React.ComponentProps<typeof TextInput>> = {},
    hint?: string,
  ) => (
    <View style={{ marginBottom: 18 }}>
      <Text style={{ fontSize: 12, fontWeight: '700', color: colors.subText, marginBottom: 6, letterSpacing: 0.4 }}>
        {label.toUpperCase()}
      </Text>
      <TextInput
        value={value}
        onChangeText={(v) => { onChange(v); setErrors((p) => ({ ...p, [key]: '' })); }}
        placeholderTextColor={colors.subText}
        style={{
          backgroundColor: isDark ? '#141418' : '#f2f2f4',
          borderWidth: StyleSheet.hairlineWidth,
          borderColor: errors[key] ? '#ff3c6e' : (isDark ? '#2a2a31' : '#dcdce1'),
          borderRadius: 12,
          paddingHorizontal: 14,
          paddingVertical: 13,
          fontSize: 15,
          color: colors.text,
        }}
        {...opts}
      />
      {!!errors[key] && (
        <Text style={{ color: '#ff3c6e', fontSize: 12.5, marginTop: 5 }}>{errors[key]}</Text>
      )}
      {!errors[key] && !!hint && (
        <Text style={{ color: colors.subText, fontSize: 12, marginTop: 5 }}>{hint}</Text>
      )}
    </View>
  );

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      style={{ flex: 1, backgroundColor: colors.background }}
    >
      <View style={{
        flexDirection: 'row', alignItems: 'center', gap: 12,
        paddingTop: insets.top + 8, paddingHorizontal: 16, paddingBottom: 12,
      }}>
        <TouchableOpacity onPress={() => router.back()} hitSlop={10}>
          <Feather name="arrow-left" size={22} color={colors.text} />
        </TouchableOpacity>
        <Text style={{ fontSize: 20, fontWeight: '800', color: colors.text }}>Edit profile</Text>
      </View>

      <ScrollView
        contentContainerStyle={{ padding: 16, paddingBottom: 40 }}
        keyboardShouldPersistTaps="handled"
      >
        {field('First name', firstName, setFirstName, 'first_name', {
          autoCapitalize: 'words', textContentType: 'givenName',
        })}
        {field('Last name', lastName, setLastName, 'last_name', {
          autoCapitalize: 'words', textContentType: 'familyName',
        })}
        {field('Phone number', phone, setPhone, 'phone_number', {
          keyboardType: 'phone-pad', textContentType: 'telephoneNumber',
        }, 'South African mobile — with or without the leading 0')}
        {field('Email', email, setEmail, 'email', {
          keyboardType: 'email-address', autoCapitalize: 'none',
          autoCorrect: false, textContentType: 'emailAddress',
        }, emailChanging ? 'You\u2019ll need to confirm this new address' : 'You sign in with this address')}

        {/* Password accounts confirm here. Social accounts have no password to
            type, so they get an explanation instead and confirm through their
            provider's sheet when they hit Save. Rendering an unfillable
            password box for them is the dead end this replaces. */}
        {emailChanging && hasPassword && field('Current password', password, setPassword, 'current_password', {
          secureTextEntry: true, autoCapitalize: 'none', autoCorrect: false,
          textContentType: 'password',
        }, 'Required to change the address you sign in with')}

        {emailChanging && !hasPassword && (
          <View style={{
            flexDirection: 'row', alignItems: 'center', gap: 10,
            backgroundColor: isDark ? 'rgba(255,255,255,0.05)' : 'rgba(0,0,0,0.04)',
            borderRadius: 12, padding: 14, marginBottom: 16,
          }}>
            <Feather name="shield" size={17} color={colors.subText} />
            <Text style={{ color: colors.subText, fontSize: 13, flex: 1, lineHeight: 18 }}>
              {socialProvider === 'apple'
                ? 'You\u2019ll confirm with Apple when you save.'
                : socialProvider === 'google'
                ? 'You\u2019ll confirm with Google when you save.'
                : 'This account has no password set. Use \u201cForgot password\u201d to set one first.'}
            </Text>
          </View>
        )}

        <TouchableOpacity
          onPress={onSavePress}
          disabled={!dirty || saving}
          style={{
            backgroundColor: '#ff2d55',
            borderRadius: 14,
            paddingVertical: 15,
            alignItems: 'center',
            opacity: !dirty || saving ? 0.5 : 1,
            marginTop: 6,
          }}
        >
          {saving
            ? <ActivityIndicator color="#fff" />
            : <Text style={{ color: '#fff', fontSize: 15, fontWeight: '800' }}>
                {dirty ? 'Save changes' : 'No changes'}
              </Text>}
        </TouchableOpacity>
      </ScrollView>
    </KeyboardAvoidingView>
  );
}