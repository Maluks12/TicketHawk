// ─────────────────────────────────────────────────────────────────────────────
// Studio password reset request.
//
// POSTs to /v1/auth/reset_password_request, the same endpoint the consumer app
// uses. Two things about that endpoint shape this screen:
//
//   1. It is enumeration-safe — it answers 200 with the same message whether
//      or not the account exists. So the success state must NOT claim a mail
//      was sent; it says "if an account exists". Anything stronger would leak
//      which emails are registered, and would also lie to the 5% of people who
//      typed the wrong address.
//   2. It is rate limited to 3 per IP per hour. A 429 is therefore an ordinary
//      outcome worth its own message, not a generic failure.
//
// The reset itself is completed on the web page the email links to, which is
// why there is no token/new-password step here. The link expires in 30 minutes
// (utils.GenerateResetToken + a 30 minute expiry in RequestPasswordResetHandler).
// ─────────────────────────────────────────────────────────────────────────────

import React, { useState } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, ScrollView, StyleSheet,
  KeyboardAvoidingView, Platform, ActivityIndicator,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter, useLocalSearchParams } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { api } from '../../lib/api';
import { makeStyles, useStudioTheme } from '../../components/studio/theme';

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export default function StudioForgotPassword() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { S } = useStudioTheme();
  const styles = useStyles();
  const params = useLocalSearchParams<{ email?: string }>();

  const [email, setEmail] = useState(params.email ?? '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);

  const valid = EMAIL_RE.test(email.trim());

  // Deep-linking straight here (or arriving after a replace()) leaves nothing
  // in the back stack, so fall through to the login screen rather than a
  // dead button.
  const goBack = () => {
    if (router.canGoBack()) router.back();
    else router.replace('/auth/login' as any);
  };

  const submit = async () => {
    const trimmed = email.trim().toLowerCase();
    if (!EMAIL_RE.test(trimmed)) {
      setError('Enter a valid email address.');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.post('/v1/auth/reset_password_request', { email: trimmed });
      setSent(true);
    } catch (err: any) {
      setError(
        err?.response?.status === 429
          ? 'Too many reset requests from this connection. Wait a few minutes and try again.'
          : 'Something went wrong. Check your connection and try again.',
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <View style={[styles.root, { paddingTop: insets.top }]}>
      <ScrollView
        contentContainerStyle={{ paddingBottom: insets.bottom + 40, flexGrow: 1 }}
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        automaticallyAdjustKeyboardInsets
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.headerRow}>
          <TouchableOpacity onPress={goBack} hitSlop={10} style={styles.back}>
            <Ionicons name="arrow-back" size={20} color={S.text} />
          </TouchableOpacity>
          <Text style={styles.headerTitle}>Reset password</Text>
          <View style={{ width: 40 }} />
        </View>

        <KeyboardAvoidingView
          behavior={Platform.OS === 'ios' ? 'padding' : undefined}
          style={{ flex: 1, justifyContent: 'center' }}
        >
          {sent ? (
            /* ── Sent ──────────────────────────────────────────────────── */
            <View style={styles.card}>
              <View style={styles.okIcon}>
                <Ionicons name="mail-outline" size={30} color={S.green} />
              </View>
              <Text style={styles.okTitle}>Check your email</Text>
              <Text style={styles.okBody}>
                If an account exists for {email.trim()}, we've sent it a link to set
                a new password. The link opens in your browser and expires in 30
                minutes.
              </Text>
              <Text style={styles.okNote}>
                Nothing arrived? Check spam, and make sure that's the address on the
                account — for security we give the same answer either way.
              </Text>

              <TouchableOpacity style={styles.btn} onPress={goBack} activeOpacity={0.85}>
                <Text style={styles.btnTxt}>Back to sign in</Text>
              </TouchableOpacity>

              <TouchableOpacity
                onPress={() => { setSent(false); setError(null); }}
                activeOpacity={0.7}
                style={{ marginTop: 16 }}
              >
                <Text style={styles.hint}>
                  Wrong address? <Text style={styles.hintLink}>Try another</Text>
                </Text>
              </TouchableOpacity>
            </View>
          ) : (
            /* ── Request ───────────────────────────────────────────────── */
            <View style={styles.card}>
              <Text style={styles.tagline}>
                Enter the email on your TicketHawk account and we'll send a link to
                set a new password. It works for organiser and door-staff accounts
                alike — there is only one account system.
              </Text>

              <Text style={styles.label}>EMAIL</Text>
              <TextInput
                style={styles.input}
                value={email}
                onChangeText={(v) => { setEmail(v); setError(null); }}
                placeholder="you@example.com"
                placeholderTextColor={S.muted}
                autoCapitalize="none"
                autoCorrect={false}
                autoFocus={!params.email}
                keyboardType="email-address"
                textContentType="emailAddress"
                returnKeyType="go"
                onSubmitEditing={submit}
              />

              {error && <Text style={styles.error}>{error}</Text>}

              <TouchableOpacity
                style={[styles.btn, (!valid || busy) && { opacity: 0.45 }]}
                onPress={submit}
                disabled={!valid || busy}
                activeOpacity={0.85}
              >
                {busy
                  ? <ActivityIndicator color={S.onAccent} />
                  : <Text style={styles.btnTxt}>Send reset link</Text>}
              </TouchableOpacity>

              <TouchableOpacity onPress={goBack} activeOpacity={0.7} style={{ marginTop: 16 }}>
                <Text style={styles.hint}>
                  Remembered it? <Text style={styles.hintLink}>Sign in</Text>
                </Text>
              </TouchableOpacity>
            </View>
          )}
        </KeyboardAvoidingView>
      </ScrollView>
    </View>
  );
}

const useStyles = makeStyles((S) => ({
  root: { flex: 1, backgroundColor: S.bg, paddingHorizontal: 22 },
  headerRow: { flexDirection: 'row', alignItems: 'center', marginBottom: 6, paddingTop: 8 },
  back: { width: 40, height: 44, alignItems: 'flex-start', justifyContent: 'center' },
  headerTitle: { flex: 1, color: S.text, fontSize: 20, fontWeight: '800', textAlign: 'center' },

  card: {
    backgroundColor: S.bgElevated,
    borderColor: S.border,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 20,
    padding: 20,
  },
  tagline: { color: S.sub, fontSize: 13.5, lineHeight: 19, marginBottom: 20 },

  label: { color: S.muted, fontSize: 11, fontWeight: '800', letterSpacing: 1.2, marginBottom: 8 },
  input: {
    backgroundColor: S.field,
    borderColor: S.border,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 12,
    color: S.text,
    paddingHorizontal: 14,
    paddingVertical: Platform.OS === 'ios' ? 13 : 10,
    fontSize: 15,
  },
  error: { color: S.red, fontSize: 13, marginTop: 12, lineHeight: 18 },

  btn: {
    backgroundColor: S.accentDeep,
    borderRadius: 13,
    alignItems: 'center',
    paddingVertical: 14,
    marginTop: 18,
  },
  btnTxt: { color: S.onAccent, fontSize: 15.5, fontWeight: '800' },
  hint: { color: S.muted, fontSize: 12.5, textAlign: 'center', lineHeight: 18 },
  hintLink: { color: S.accent, fontWeight: '700' },

  okIcon: {
    width: 64, height: 64, borderRadius: 20, alignSelf: 'center',
    alignItems: 'center', justifyContent: 'center',
    backgroundColor: S.greenSoft, marginBottom: 14,
  },
  okTitle: { color: S.text, fontSize: 19, fontWeight: '800', textAlign: 'center' },
  okBody: {
    color: S.sub, fontSize: 13.5, lineHeight: 20,
    textAlign: 'center', marginTop: 10,
  },
  okNote: {
    color: S.muted, fontSize: 12.5, lineHeight: 18,
    textAlign: 'center', marginTop: 12,
  },
}));