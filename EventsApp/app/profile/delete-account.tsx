// ─────────────────────────────────────────────────────────────────────────────
// Delete account
//
// Required by Apple App Store Guideline 5.1.1(v): an app that offers account
// creation must offer account deletion from inside the app. Not a link to a
// website, not an email to support — here.
//
// The screen is built around one idea: nobody should be able to delete an
// account without first seeing, in their own numbers, what they are destroying.
// So it loads a preview before showing the button, and the copy is specific
// ("3 tickets worth R1,480") rather than generic ("are you sure?"). Generic
// warnings get tapped through; specific ones get read.
//
// Three outcomes are handled:
//   • organiser hold      — live events or unsettled payouts; explain and stop
//   • upcoming tickets    — second confirmation naming the loss, then proceed
//   • clean               — confirm identity, confirm intent, gone
//
// ── Confirming identity without a password ───────────────────────────────────
//
// A valid access token is deliberately not enough to delete an account: that
// would make a stolen unlocked phone sufficient to destroy someone's tickets.
// So this screen demands step-up proof.
//
// Since migration 000075 an account can exist with password IS NULL — created
// through Google or Apple. This screen used to render a password field
// unconditionally, which for those users was an unfillable box in front of the
// one action Guideline 5.1.1(v) says must always be available. They could not
// delete their account at all.
//
// The social equivalent is a FRESH ID token from a provider already linked to
// the account. The server checks the token's subject is linked here before
// accepting it (see handlers/reauth.go), so this is genuine re-authentication
// rather than a way around it.
// ─────────────────────────────────────────────────────────────────────────────

import React, { useCallback, useEffect, useState } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, ActivityIndicator,
  ScrollView, StyleSheet, Alert, Platform, Linking,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Feather, Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { api } from '../../lib/api';
import { useAuthStore } from '../../store/authStore';
import { useAppTheme } from '../../theme/colors';
import { fmtRand } from '../../lib/money';
import {
  signInWithApple, signInWithGoogle,
  SocialAuthCancelled, SocialAuthProviderError,
  type SocialCredential,
} from '../../lib/socialAuth';

const DANGER = '#ff453a';

interface DeletionPreview {
  can_delete: boolean;
  organiser_hold: boolean;
  upcoming_tickets: number;
  upcoming_value: number;
  live_events: number;
  unsettled_payouts: number;
}

// lib/money. This one prints refund amounts, where cents are the whole point.
const rand = (n: number) => fmtRand(n);

export default function DeleteAccountScreen() {
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();
  const logout = useAuthStore((s) => s.logout);
  const user = useAuthStore((s) => s.user);

  const [preview, setPreview] = useState<DeletionPreview | null>(null);
  const [loading, setLoading] = useState(true);
  const [password, setPassword] = useState('');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // How this account proves who it is. Both fields come from GET /v1/user/me.
  // `!== false` rather than `=== true`: an older cached user object predating
  // the field should behave as a password account, which is the safe default —
  // it shows a field the user can fill rather than a provider sheet they may
  // have no identity for.
  const hasPassword = (user as any)?.has_password !== false;
  const linkedProviders: string[] = (user as any)?.linked_providers ?? [];
  const socialProvider: 'apple' | 'google' | null =
    linkedProviders.includes('apple') ? 'apple'
    : linkedProviders.includes('google') ? 'google'
    : null;
  const providerLabel = socialProvider === 'apple' ? 'Apple' : 'Google';

  // ── Preview ────────────────────────────────────────────────────────────────
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const { data } = await api.get<DeletionPreview>('/v1/user/me/deletion-preview');
        if (!cancelled) setPreview(data);
      } catch {
        // A failed preview must not block deletion — the server re-checks
        // everything on the DELETE anyway, so the worst case is that the user
        // sees the generic warning instead of the specific one.
        if (!cancelled) setPreview(null);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  // Re-runs the linked provider's sign-in sheet. The token it returns is the
  // social equivalent of typing a password, and the server rejects it unless
  // its subject is already linked to this account.
  const confirmWithProvider = useCallback(async (): Promise<SocialCredential> => {
    if (socialProvider === 'apple') return signInWithApple();
    if (socialProvider === 'google') return signInWithGoogle();
    throw new SocialAuthProviderError(
      'This account has no password and no connected sign-in method. Use “Forgot password” on the sign-in screen to set one, then try again.',
    );
  }, [socialProvider]);

  // ── Delete ─────────────────────────────────────────────────────────────────
  const doDelete = useCallback(async (acknowledgeForfeit: boolean) => {
    setBusy(true);
    setError(null);
    try {
      // Build the step-up credential. ReauthCredentials is embedded in the
      // server's DTO, so these flatten to the top level of the body — meaning
      // the plain `password` shape that existing clients send still works
      // unchanged.
      const credential: Record<string, unknown> = {};
      if (hasPassword) {
        credential.password = password;
      } else {
        const cred = await confirmWithProvider();
        credential.provider = cred.provider;
        credential.id_token = cred.idToken;
        if (cred.nonce) credential.nonce = cred.nonce;
      }

      await api.delete('/v1/user/me', {
        data: {
          ...credential,
          acknowledge_forfeit: acknowledgeForfeit,
          reason: reason.trim() || undefined,
        },
      });

      // Clear the local session before navigating; otherwise the tab layout
      // re-renders with a user object whose account no longer exists.
      await logout().catch(() => {});
      router.replace('/(tabs)' as any);
      // Deliberately after the navigation, so it lands on the home screen
      // rather than on a screen that is about to unmount.
      setTimeout(() => {
        Alert.alert('Account deleted', 'Your account and personal details have been removed.');
      }, 400);
    } catch (e: any) {
      // Backing out of the Apple/Google sheet is a normal outcome, not a
      // failure — silently return to the screen.
      if (e instanceof SocialAuthCancelled) { setBusy(false); return; }
      if (e instanceof SocialAuthProviderError) {
        setError(e.message);
        setBusy(false);
        return;
      }

      const status = e?.response?.status;
      const body = e?.response?.data ?? {};

      if (status === 409 && body.code === 'tickets_will_be_lost') {
        setBusy(false);
        Alert.alert(
          'You still have tickets',
          `You have ${body.upcoming_tickets} ticket${body.upcoming_tickets === 1 ? '' : 's'}` +
          `${body.upcoming_value ? ` worth ${rand(body.upcoming_value)}` : ''} for events that ` +
          `haven't happened yet. Deleting your account means you lose access to them and they ` +
          `will not be refunded.\n\nIf you want your money back, cancel the bookings first — ` +
          `refunds are handled from each ticket.`,
          [
            { text: 'Keep my account', style: 'cancel' },
            {
              text: 'Delete anyway',
              style: 'destructive',
              onPress: () => { void doDelete(true); },
            },
          ],
        );
        return;
      }

      if (status === 409 && body.code === 'organiser_hold') {
        setError(body.error ?? 'Your organiser account is still active.');
      } else if (body.code === 'reauth_social_required') {
        // The server says this account has no password. Reaching here means the
        // cached user object was stale — the next attempt will take the social
        // branch above.
        setError(`This account signs in with ${providerLabel}. Tap “Delete my account” again to confirm.`);
      } else if (body.code === 'reauth_wrong_account') {
        setError(`That ${providerLabel} account isn’t the one connected to this profile.`);
      } else if (status === 401) {
        setError(hasPassword ? 'That password is incorrect.' : 'Could not confirm your identity.');
      } else {
        setError(body.error ?? 'Could not delete your account. Please try again.');
      }
      setBusy(false);
    }
  }, [password, reason, logout, router, hasPassword, confirmWithProvider, providerLabel]);

  const confirmThenDelete = useCallback(() => {
    if (hasPassword && !password) { setError('Enter your password to continue.'); return; }
    Alert.alert(
      'Delete your account?',
      'This cannot be undone. Your name, email address and phone number will be ' +
      'permanently removed, and you will be signed out on every device.',
      [
        { text: 'Cancel', style: 'cancel' },
        { text: 'Delete account', style: 'destructive', onPress: () => { void doDelete(false); } },
      ],
    );
  }, [hasPassword, password, doDelete]);

  const held = preview?.organiser_hold === true;
  const cardBg = isDark ? 'rgba(255,255,255,0.04)' : 'rgba(10,10,10,0.03)';
  // A social account has nothing to type, so the button is enabled from the
  // start — gating it on `password` would leave it permanently dead for them.
  const canSubmit = !busy && (!hasPassword || !!password);

  return (
    <View style={{ flex: 1, backgroundColor: colors.background }}>
      {/* Header */}
      <View style={[ss.header, { paddingTop: insets.top + 12 }]}>
        <TouchableOpacity onPress={() => router.back()} style={ss.backBtn} activeOpacity={0.7}>
          <Feather name="arrow-left" size={20} color={colors.text} />
        </TouchableOpacity>
        <Text style={[ss.headerTitle, { color: colors.text }]}>Delete account</Text>
        <View style={{ width: 40 }} />
      </View>
      <View style={{ height: StyleSheet.hairlineWidth, backgroundColor: colors.dividerColor }} />

      {/* No KeyboardAvoidingView. Wrapping this ScrollView in one with
          behavior="padding" is what produced the blank slab above the keyboard:
          the KAV shrank the ScrollView by the keyboard's height, so the parent
          View's background showed through the vacated strip, and the fields
          below the fold (reason, delete button) were pushed out of the
          shortened viewport entirely rather than scrolled to.

          automaticallyAdjustKeyboardInsets does the right thing instead -- it
          adds the keyboard as a content inset, so the ScrollView keeps its full
          height, nothing is vacated, and the focused field is scrolled above
          the keyboard. Android needs neither: Expo builds ship
          windowSoftInputMode=adjustResize and the OS resizes the window. */}
      <ScrollView
        contentContainerStyle={{ padding: 20, paddingBottom: insets.bottom + 40 }}
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        automaticallyAdjustKeyboardInsets
        showsVerticalScrollIndicator={false}
      >
          {loading ? (
            <View style={{ paddingTop: 60, alignItems: 'center' }}>
              <ActivityIndicator color={colors.subText} />
            </View>
          ) : (
            <>
              {/* What happens */}
              <View style={[ss.card, { backgroundColor: cardBg, borderColor: colors.dividerColor }]}>
                <Text style={[ss.cardTitle, { color: colors.text }]}>What gets deleted</Text>
                {[
                  'Your name, email address and phone number',
                  'Your saved events and notification history',
                  'Every signed-in device is signed out',
                ].map((line) => (
                  <View key={line} style={ss.bulletRow}>
                    <Ionicons name="close-circle" size={15} color={DANGER} />
                    <Text style={[ss.bulletText, { color: colors.subText }]}>{line}</Text>
                  </View>
                ))}

                <Text style={[ss.cardTitle, { color: colors.text, marginTop: 18 }]}>What we keep</Text>
                <View style={ss.bulletRow}>
                  <Ionicons name="document-text-outline" size={15} color={colors.subText} />
                  <Text style={[ss.bulletText, { color: colors.subText }]}>
                    Records of past ticket purchases, with your personal details removed.
                    South African tax and consumer-protection law requires us to keep proof
                    of sale, and event organisers need it for their own settlement records.
                  </Text>
                </View>
              </View>

              {/* Organiser hold */}
              {held && (
                <View style={[ss.notice, { backgroundColor: 'rgba(255,184,48,0.10)', borderColor: 'rgba(255,184,48,0.35)' }]}>
                  <Ionicons name="warning-outline" size={18} color="#ffb830" />
                  <View style={{ flex: 1 }}>
                    <Text style={[ss.noticeTitle, { color: colors.text }]}>Organiser account active</Text>
                    <Text style={[ss.noticeBody, { color: colors.subText }]}>
                      {preview!.live_events > 0
                        ? `You have ${preview!.live_events} live event${preview!.live_events === 1 ? '' : 's'} with ticket holders. `
                        : ''}
                      {preview!.unsettled_payouts > 0
                        ? `You have ${preview!.unsettled_payouts} payout${preview!.unsettled_payouts === 1 ? '' : 's'} still settling. `
                        : ''}
                      Finish or cancel these first — we can't remove the organiser behind an
                      event that people have bought tickets to.
                    </Text>
                    <TouchableOpacity
                      onPress={() => Linking.openURL('mailto:support@ticket-hawk.com')}
                      activeOpacity={0.7}
                    >
                      <Text style={[ss.noticeLink]}>support@ticket-hawk.com</Text>
                    </TouchableOpacity>
                  </View>
                </View>
              )}

              {/* Upcoming tickets */}
              {!held && (preview?.upcoming_tickets ?? 0) > 0 && (
                <View style={[ss.notice, { backgroundColor: 'rgba(255,69,58,0.09)', borderColor: 'rgba(255,69,58,0.32)' }]}>
                  <Ionicons name="ticket-outline" size={18} color={DANGER} />
                  <View style={{ flex: 1 }}>
                    <Text style={[ss.noticeTitle, { color: colors.text }]}>
                      You have {preview!.upcoming_tickets} upcoming ticket
                      {preview!.upcoming_tickets === 1 ? '' : 's'}
                    </Text>
                    <Text style={[ss.noticeBody, { color: colors.subText }]}>
                      Worth {rand(preview!.upcoming_value)}. Deleting your account means losing
                      access to them, and they will not be refunded. If you want your money back,
                      cancel the bookings from your Tickets tab first.
                    </Text>
                  </View>
                </View>
              )}

              {/* Identity confirmation */}
              {!held && (
                <>
                  {hasPassword ? (
                    <>
                      <Text style={[ss.label, { color: colors.subText }]}>
                        CONFIRM YOUR PASSWORD
                      </Text>
                      <TextInput
                        value={password}
                        onChangeText={(v) => { setPassword(v); setError(null); }}
                        placeholder="Your password"
                        placeholderTextColor={colors.iconColor}
                        secureTextEntry
                        autoCapitalize="none"
                        textContentType="password"
                        style={[ss.input, {
                          color: colors.text,
                          backgroundColor: cardBg,
                          borderColor: error ? DANGER : colors.dividerColor,
                        }]}
                      />
                    </>
                  ) : (
                    // No password to type. Rendering the field anyway is what
                    // made deletion impossible for these accounts — an
                    // unfillable box in front of a mandatory action.
                    <>
                      <Text style={[ss.label, { color: colors.subText }]}>
                        CONFIRM YOUR IDENTITY
                      </Text>
                      <View style={[ss.card, {
                        backgroundColor: cardBg,
                        borderColor: colors.dividerColor,
                        marginBottom: 0,
                        flexDirection: 'row',
                        gap: 11,
                        alignItems: 'flex-start',
                      }]}>
                        <Ionicons name="shield-checkmark-outline" size={17} color={colors.subText} />
                        <Text style={[ss.bulletText, { color: colors.subText, marginBottom: 0 }]}>
                          {socialProvider
                            ? `You sign in with ${providerLabel}. You’ll be asked to confirm with ${providerLabel} before your account is deleted.`
                            : 'This account has no password set. Use “Forgot password” on the sign-in screen to set one, then come back.'}
                        </Text>
                      </View>
                    </>
                  )}

                  <Text style={[ss.label, { color: colors.subText, marginTop: 18 }]}>
                    WHY ARE YOU LEAVING? (OPTIONAL)
                  </Text>
                  <TextInput
                    value={reason}
                    onChangeText={setReason}
                    placeholder="This helps us improve TicketHawk"
                    placeholderTextColor={colors.iconColor}
                    multiline
                    maxLength={500}
                    style={[ss.input, {
                      color: colors.text,
                      backgroundColor: cardBg,
                      borderColor: colors.dividerColor,
                      height: 88,
                      textAlignVertical: 'top',
                      paddingTop: 12,
                    }]}
                  />

                  {error && (
                    <View style={ss.errorRow}>
                      <Ionicons name="alert-circle" size={14} color={DANGER} />
                      <Text style={ss.errorText}>{error}</Text>
                    </View>
                  )}

                  <TouchableOpacity
                    onPress={confirmThenDelete}
                    disabled={!canSubmit}
                    activeOpacity={0.85}
                    style={[ss.deleteBtn, !canSubmit && { opacity: 0.45 }]}
                  >
                    {busy
                      ? <ActivityIndicator color="#fff" size="small" />
                      : <Text style={ss.deleteBtnText}>Delete my account</Text>}
                  </TouchableOpacity>

                  <TouchableOpacity onPress={() => router.back()} activeOpacity={0.7} style={{ marginTop: 14 }}>
                    <Text style={[ss.keepText, { color: colors.subText }]}>Keep my account</Text>
                  </TouchableOpacity>
                </>
              )}

              {held && (
                <TouchableOpacity onPress={() => router.back()} activeOpacity={0.7} style={{ marginTop: 8 }}>
                  <Text style={[ss.keepText, { color: colors.subText }]}>Go back</Text>
                </TouchableOpacity>
              )}

              <Text style={[ss.signedIn, { color: colors.iconColor }]}>
                Signed in as {user?.email ?? '—'}
              </Text>
            </>
          )}
      </ScrollView>
    </View>
  );
}

const ss = StyleSheet.create({
  header: {
    flexDirection: 'row', alignItems: 'center',
    paddingHorizontal: 16, paddingBottom: 12,
  },
  backBtn: { width: 40, height: 40, alignItems: 'flex-start', justifyContent: 'center' },
  headerTitle: { flex: 1, fontSize: 17, fontWeight: '700', textAlign: 'center' },

  card: { borderRadius: 16, borderWidth: StyleSheet.hairlineWidth, padding: 16, marginBottom: 16 },
  cardTitle: { fontSize: 14, fontWeight: '800', marginBottom: 10 },
  bulletRow: { flexDirection: 'row', gap: 9, alignItems: 'flex-start', marginBottom: 8 },
  bulletText: { flex: 1, fontSize: 13, lineHeight: 19 },

  notice: {
    flexDirection: 'row', gap: 11, alignItems: 'flex-start',
    borderRadius: 14, borderWidth: StyleSheet.hairlineWidth, padding: 14, marginBottom: 16,
  },
  noticeTitle: { fontSize: 14, fontWeight: '700', marginBottom: 4 },
  noticeBody: { fontSize: 13, lineHeight: 19 },
  noticeLink: { fontSize: 13, fontWeight: '700', color: '#ffb830', marginTop: 8 },

  label: { fontSize: 11, fontWeight: '800', letterSpacing: 1.1, marginBottom: 8 },
  input: {
    borderRadius: 12, borderWidth: StyleSheet.hairlineWidth,
    paddingHorizontal: 14, paddingVertical: Platform.OS === 'ios' ? 13 : 10, fontSize: 15,
  },
  errorRow: { flexDirection: 'row', alignItems: 'center', gap: 6, marginTop: 12 },
  errorText: { flex: 1, color: DANGER, fontSize: 13, lineHeight: 18 },

  deleteBtn: {
    backgroundColor: DANGER, borderRadius: 14,
    alignItems: 'center', paddingVertical: 15, marginTop: 22,
  },
  deleteBtnText: { color: '#fff', fontSize: 15.5, fontWeight: '800' },
  keepText: { fontSize: 14, fontWeight: '600', textAlign: 'center' },
  signedIn: { fontSize: 12, textAlign: 'center', marginTop: 26 },
});