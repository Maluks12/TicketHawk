// ─────────────────────────────────────────────────────────────────────────────
// Studio sign-up.
//
// Studio used to be login-only, on the reasoning that accounts are made in the
// consumer app or on the web. That reasoning is fine for real users and fatal
// for App Review: a reviewer who cannot create an account cannot evaluate the
// app, and "download our other app first" is not an acceptable answer. It also
// quietly lost every organiser who found Studio first.
//
// One account system, so this creates an ordinary TicketHawk account through
// the same /v1/auth/register endpoint the consumer app uses. The only Studio
// addition is the optional organiser application on top.
//
// ── What a new account can actually do ──────────────────────────────────────
//
// Registration alone does NOT grant the organiser role. Since the KYC gating
// overhaul, /v1/register-organizer creates a PENDING organiser profile and the
// role is granted only once identity verification passes. So a fresh sign-up
// lands on the staff screen, which is the honest outcome: they can redeem a
// join code and scan at a door immediately, and their organiser dashboard
// unlocks when verification completes.
//
// The screen says this in plain language BEFORE the button rather than
// surfacing it as a surprise afterwards.
// ─────────────────────────────────────────────────────────────────────────────

import React, { useMemo, useState } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, StyleSheet, ScrollView,
  KeyboardAvoidingView, Platform, ActivityIndicator, Linking,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { useAuthStore } from '../../store/authStore';
import { applyAsOrganiser } from '../../lib/api';
import { STUDIO } from '../../components/studio/theme';

const TERMS_URL   = 'https://ticket-hawk.com/legal/terms';
const PRIVACY_URL = 'https://ticket-hawk.com/legal/privacy';

// Mirrors the backend's `strong_password` binding. Validated here so the user
// finds out while typing rather than after a round trip.
function passwordProblems(pw: string): string[] {
  const out: string[] = [];
  if (pw.length < 8) out.push('8+ characters');
  if (!/[A-Z]/.test(pw)) out.push('an uppercase letter');
  if (!/[a-z]/.test(pw)) out.push('a lowercase letter');
  if (!/[0-9]/.test(pw)) out.push('a number');
  return out;
}

// SA mobile numbers, normalised the same way the backend does: strip +27 / 27 /
// leading 0 down to the 9 national digits, then store as 0XXXXXXXXX.
function normalisePhone(raw: string): string | null {
  let d = raw.replace(/\D/g, '');
  if (d.startsWith('27')) d = d.slice(2);
  if (d.startsWith('0')) d = d.slice(1);
  return d.length === 9 ? `0${d}` : null;
}

const emailOk = (v: string) => /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(v);

export default function StudioSignup() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const register = useAuthStore((s) => s.register);

  const [firstName, setFirstName] = useState('');
  const [lastName,  setLastName]  = useState('');
  const [email,     setEmail]     = useState('');
  const [phone,     setPhone]     = useState('');
  const [password,  setPassword]  = useState('');
  const [showPw,    setShowPw]    = useState(false);
  const [wantsOrganiser, setWantsOrganiser] = useState(true);
  const [agreed,    setAgreed]    = useState(false);

  const [busy,  setBusy]  = useState(false);
  const [error, setError] = useState<string | null>(null);

  const pwProblems = useMemo(() => passwordProblems(password), [password]);
  const phoneOk    = useMemo(() => normalisePhone(phone) !== null, [phone]);

  const valid =
    firstName.trim().length > 0 &&
    lastName.trim().length > 0 &&
    emailOk(email.trim()) &&
    phoneOk &&
    pwProblems.length === 0 &&
    agreed;

  const submit = async () => {
    if (!valid || busy) return;
    setBusy(true);
    setError(null);

    try {
      // register() creates the account, logs in and loads the profile — the
      // store already does all three, so nothing extra is needed here.
      await register({
        email: email.trim().toLowerCase(),
        password,
        first_name: firstName.trim(),
        last_name: lastName.trim(),
        phone_number: normalisePhone(phone)!,
        // The backend accepts this optional; Studio doesn't ask for a birthday
        // because nothing in the organiser or scanning flow needs one, and
        // collecting a date of birth we never use is data we then have to
        // protect for no reason.
        date_of_birth: '',
      });

      // Organiser application is best-effort and deliberately non-fatal. The
      // account exists and works either way; failing the whole sign-up because
      // an optional follow-up call timed out would be the wrong trade.
      if (wantsOrganiser) {
        try { await applyAsOrganiser(); } catch { /* they can re-apply later */ }
      }

      // Back through the gate so role routing stays in ONE place (app/index.tsx).
      router.replace('/' as any);
    } catch (err: any) {
      const msg = err?.response?.data?.error;
      setError(
        msg === 'Email already registered'
          ? 'That email already has a TicketHawk account. Sign in instead.'
          : msg ?? 'Could not create your account. Check your details and try again.',
      );
      setBusy(false);
    }
  };

  return (
    <View style={[styles.root, { paddingTop: insets.top }]}>
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        style={{ flex: 1 }}
      >
        <ScrollView
          contentContainerStyle={{ paddingBottom: insets.bottom + 40, paddingTop: 8 }}
          keyboardShouldPersistTaps="handled"
          showsVerticalScrollIndicator={false}
        >
          {/* Header */}
          <View style={styles.headerRow}>
            <TouchableOpacity onPress={() => router.back()} hitSlop={10} style={styles.back}>
              <Ionicons name="arrow-back" size={20} color={STUDIO.text} />
            </TouchableOpacity>
            <Text style={styles.headerTitle}>Create account</Text>
            <View style={{ width: 40 }} />
          </View>

          <Text style={styles.tagline}>
            One TicketHawk account works everywhere — Studio, the app and the web
            dashboard.
          </Text>

          <View style={styles.card}>
            <View style={styles.row2}>
              <View style={{ flex: 1 }}>
                <Text style={styles.label}>FIRST NAME</Text>
                <TextInput
                  style={styles.input}
                  value={firstName}
                  onChangeText={setFirstName}
                  placeholder="Thabo"
                  placeholderTextColor={STUDIO.muted}
                  autoCapitalize="words"
                  textContentType="givenName"
                />
              </View>
              <View style={{ flex: 1 }}>
                <Text style={styles.label}>SURNAME</Text>
                <TextInput
                  style={styles.input}
                  value={lastName}
                  onChangeText={setLastName}
                  placeholder="Mokoena"
                  placeholderTextColor={STUDIO.muted}
                  autoCapitalize="words"
                  textContentType="familyName"
                />
              </View>
            </View>

            <Text style={[styles.label, { marginTop: 16 }]}>EMAIL</Text>
            <TextInput
              style={styles.input}
              value={email}
              onChangeText={(v) => { setEmail(v); setError(null); }}
              placeholder="you@example.com"
              placeholderTextColor={STUDIO.muted}
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="email-address"
              textContentType="emailAddress"
            />

            <Text style={[styles.label, { marginTop: 16 }]}>MOBILE NUMBER</Text>
            <TextInput
              style={styles.input}
              value={phone}
              onChangeText={setPhone}
              placeholder="082 123 4567"
              placeholderTextColor={STUDIO.muted}
              keyboardType="phone-pad"
              textContentType="telephoneNumber"
            />
            {phone.length > 0 && !phoneOk && (
              <Text style={styles.fieldHint}>Enter a valid South African mobile number.</Text>
            )}

            <Text style={[styles.label, { marginTop: 16 }]}>PASSWORD</Text>
            <View style={styles.pwRow}>
              <TextInput
                style={[styles.input, { flex: 1 }]}
                value={password}
                onChangeText={setPassword}
                placeholder="Choose a strong password"
                placeholderTextColor={STUDIO.muted}
                secureTextEntry={!showPw}
                autoCapitalize="none"
                textContentType="newPassword"
              />
              <TouchableOpacity onPress={() => setShowPw((v) => !v)} hitSlop={10} style={styles.eye}>
                <Ionicons name={showPw ? 'eye-off-outline' : 'eye-outline'} size={20} color={STUDIO.sub} />
              </TouchableOpacity>
            </View>
            {password.length > 0 && pwProblems.length > 0 && (
              <Text style={styles.fieldHint}>Needs {pwProblems.join(', ')}.</Text>
            )}

            {/* Organiser opt-in */}
            <TouchableOpacity
              style={styles.checkRow}
              onPress={() => setWantsOrganiser((v) => !v)}
              activeOpacity={0.7}
            >
              <View style={[styles.check, wantsOrganiser && styles.checkOn]}>
                {wantsOrganiser && <Ionicons name="checkmark" size={13} color="#fff" />}
              </View>
              <View style={{ flex: 1 }}>
                <Text style={styles.checkLabel}>I want to sell tickets to my own events</Text>
                <Text style={styles.checkSub}>
                  We'll start your organiser application. You'll need to complete
                  identity verification before your dashboard unlocks — until then you
                  can still scan tickets with a join code.
                </Text>
              </View>
            </TouchableOpacity>

            {/* Terms */}
            <TouchableOpacity
              style={[styles.checkRow, { marginTop: 4 }]}
              onPress={() => setAgreed((v) => !v)}
              activeOpacity={0.7}
            >
              <View style={[styles.check, agreed && styles.checkOn]}>
                {agreed && <Ionicons name="checkmark" size={13} color="#fff" />}
              </View>
              <Text style={[styles.checkLabel, { flex: 1 }]}>
                I agree to the{' '}
                <Text style={styles.link} onPress={() => Linking.openURL(TERMS_URL)}>Terms of Use</Text>
                {' '}and{' '}
                <Text style={styles.link} onPress={() => Linking.openURL(PRIVACY_URL)}>Privacy Policy</Text>.
              </Text>
            </TouchableOpacity>

            {error && <Text style={styles.error}>{error}</Text>}

            <TouchableOpacity
              style={[styles.btn, (!valid || busy) && { opacity: 0.45 }]}
              onPress={submit}
              disabled={!valid || busy}
              activeOpacity={0.85}
            >
              {busy
                ? <ActivityIndicator color="#fff" />
                : <Text style={styles.btnTxt}>Create account</Text>}
            </TouchableOpacity>

            <TouchableOpacity onPress={() => router.back()} activeOpacity={0.7} style={{ marginTop: 16 }}>
              <Text style={styles.hint}>
                Already have an account? <Text style={styles.link}>Sign in</Text>
              </Text>
            </TouchableOpacity>
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: STUDIO.bg, paddingHorizontal: 22 },
  headerRow: { flexDirection: 'row', alignItems: 'center', marginBottom: 6 },
  back: { width: 40, height: 44, alignItems: 'flex-start', justifyContent: 'center' },
  headerTitle: { flex: 1, color: STUDIO.text, fontSize: 20, fontWeight: '800', textAlign: 'center' },
  tagline: {
    color: STUDIO.sub, fontSize: 13.5, textAlign: 'center',
    marginBottom: 20, lineHeight: 19, paddingHorizontal: 10,
  },
  card: {
    backgroundColor: STUDIO.bgElevated,
    borderColor: STUDIO.border,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 20,
    padding: 20,
  },
  row2: { flexDirection: 'row', gap: 12 },
  label: { color: STUDIO.muted, fontSize: 11, fontWeight: '800', letterSpacing: 1.2, marginBottom: 8 },
  input: {
    backgroundColor: STUDIO.surface,
    borderColor: STUDIO.border,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 12,
    color: STUDIO.text,
    paddingHorizontal: 14,
    paddingVertical: Platform.OS === 'ios' ? 13 : 10,
    fontSize: 15,
  },
  fieldHint: { color: STUDIO.amber, fontSize: 12, marginTop: 7, lineHeight: 17 },
  pwRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  eye: {
    width: 44, height: 44, borderRadius: 12, alignItems: 'center', justifyContent: 'center',
    backgroundColor: STUDIO.surface,
    borderColor: STUDIO.border, borderWidth: StyleSheet.hairlineWidth,
  },
  checkRow: { flexDirection: 'row', gap: 11, alignItems: 'flex-start', marginTop: 20 },
  check: {
    width: 20, height: 20, borderRadius: 6, marginTop: 1,
    borderWidth: StyleSheet.hairlineWidth, borderColor: STUDIO.borderStrong,
    alignItems: 'center', justifyContent: 'center',
  },
  checkOn: { backgroundColor: STUDIO.accentDeep, borderColor: STUDIO.accentDeep },
  checkLabel: { color: STUDIO.text, fontSize: 13.5, lineHeight: 19 },
  checkSub: { color: STUDIO.sub, fontSize: 12.5, lineHeight: 18, marginTop: 5 },
  link: { color: STUDIO.accent, fontWeight: '700' },
  error: { color: STUDIO.red, fontSize: 13, marginTop: 16, lineHeight: 18 },
  btn: {
    backgroundColor: STUDIO.accentDeep,
    borderRadius: 13,
    alignItems: 'center',
    paddingVertical: 14,
    marginTop: 20,
  },
  btnTxt: { color: '#fff', fontSize: 15.5, fontWeight: '800' },
  hint: { color: STUDIO.muted, fontSize: 12.5, textAlign: 'center', lineHeight: 18 },
});