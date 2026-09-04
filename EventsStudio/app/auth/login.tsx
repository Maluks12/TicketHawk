import React, { useState } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, StyleSheet,
  KeyboardAvoidingView, Platform, ActivityIndicator, Image,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { useAuthStore } from '../../store/authStore';
import { makeStyles, useStudioTheme } from '../../components/studio/theme';
import { ThemeToggle } from '../../components/studio/ui';

// ─────────────────────────────────────────────────────────────────────────────
// Studio login.
//
// One account system across TicketHawk — organisers and door staff sign in with
// the same credentials they use in the consumer app, the web dashboard or here.
//
// Studio used to be login-only, on the reasoning that accounts get made
// elsewhere. That fails App Review (a reviewer who can't create an account
// can't evaluate the app) and quietly lost every organiser who found Studio
// first, so app/auth/signup.tsx now exists and is linked below.
//
// The same reasoning applies to a FORGOTTEN password, and that gap was worse:
// sign-up at least had a workaround (make the account elsewhere), while a
// staffer locked out at a door had none — no reset link anywhere in the app,
// and the reset page is a web page they'd have to find on their own. The link
// under the button now goes to /auth/forgot.
//
// After login the auth gate (app/index.tsx) routes by role.
// ─────────────────────────────────────────────────────────────────────────────
export default function StudioLogin() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const login = useAuthStore((s) => s.login);
  const { S } = useStudioTheme();
  const styles = useStyles();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPw, setShowPw] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    const e = email.trim();
    if (!e || !password) {
      setError('Enter your email and password.');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await login(e, password);
      // Back through the gate so role routing stays in ONE place.
      router.replace('/' as any);
    } catch (err: any) {
      setError(err?.response?.data?.error ?? 'Sign in failed. Check your details and try again.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <View style={[styles.root, { paddingTop: insets.top }]}>
      {/* Appearance toggle. Reachable before sign-in on purpose: door staff
          set this phone up outdoors, and a reviewer should be able to see both
          modes without an account. */}
      <View style={styles.topRow}>
        <ThemeToggle />
      </View>

      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        style={{ flex: 1, justifyContent: 'center' }}
      >
        {/* Brand */}
        <View style={styles.brand}>
          {/* brand-logo, NOT icon.png. icon.png is the launcher icon: it has an
              opaque #08080C square baked into it because the OS needs a filled
              square to mask into a rounded rect. Rendering it inside the app
              therefore draws a visible dark tile behind the hawk, which is what
              the black box on this screen was. brand-logo.png is the same mark
              on transparency, so it sits directly on the screen background —
              which is now sometimes light, making this doubly important. */}
          <Image source={require('../../assets/brand-logo.png')} style={styles.logo} />
          <View style={styles.brandLine}>
            <Text style={styles.brandWord}>TicketHawk</Text>
            <View style={styles.pill}>
              <Text style={styles.pillTxt}>STUDIO</Text>
            </View>
          </View>
          <Text style={styles.tagline}>
            Door staff and organisers sign in with the same TicketHawk account.
            Got a join code? Sign in and enter it on the next screen.
          </Text>
        </View>

        {/* Form */}
        <View style={styles.card}>
          <Text style={styles.label}>EMAIL</Text>
          <TextInput
            style={styles.input}
            value={email}
            onChangeText={setEmail}
            placeholder="you@example.com"
            placeholderTextColor={S.muted}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="email-address"
            textContentType="emailAddress"
          />

          <View style={styles.labelRow}>
            <Text style={styles.label}>PASSWORD</Text>
            {/* Carries the typed email across so they don't retype it. */}
            <TouchableOpacity
              onPress={() =>
                router.push({
                  pathname: '/auth/forgot',
                  params: email.trim() ? { email: email.trim() } : {},
                } as any)
              }
              hitSlop={8}
              activeOpacity={0.7}
            >
              <Text style={styles.forgotTxt}>Forgot password?</Text>
            </TouchableOpacity>
          </View>
          <View style={styles.pwRow}>
            <TextInput
              style={[styles.input, { flex: 1 }]}
              value={password}
              onChangeText={setPassword}
              placeholder="Your password"
              placeholderTextColor={S.muted}
              secureTextEntry={!showPw}
              autoCapitalize="none"
              textContentType="password"
              onSubmitEditing={submit}
              returnKeyType="go"
            />
            <TouchableOpacity onPress={() => setShowPw((v) => !v)} hitSlop={10} style={styles.eye}>
              <Ionicons name={showPw ? 'eye-off-outline' : 'eye-outline'} size={20} color={S.sub} />
            </TouchableOpacity>
          </View>

          {error && <Text style={styles.error}>{error}</Text>}

          <TouchableOpacity style={styles.btn} onPress={submit} disabled={busy} activeOpacity={0.85}>
            {busy ? (
              <ActivityIndicator color={S.onAccent} />
            ) : (
              <Text style={styles.btnTxt}>Sign in</Text>
            )}
          </TouchableOpacity>

          <TouchableOpacity
            onPress={() => router.push('/auth/signup' as any)}
            activeOpacity={0.7}
            style={{ marginTop: 16 }}
          >
            <Text style={styles.hint}>
              No account? <Text style={styles.hintLink}>Create one</Text>
            </Text>
          </TouchableOpacity>
        </View>
      </KeyboardAvoidingView>
    </View>
  );
}

const useStyles = makeStyles((S) => ({
  root: { flex: 1, backgroundColor: S.bg, paddingHorizontal: 22 },
  topRow: { flexDirection: 'row', justifyContent: 'flex-end', paddingTop: 6 },
  brand: { alignItems: 'center', marginBottom: 30 },
  // No borderRadius: there is no longer a square to round off, and rounding a
  // transparent logo just clips the hawk's outer feathers.
  logo: { width: 72, height: 72, marginBottom: 14, resizeMode: 'contain' },
  brandLine: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  brandWord: { color: S.text, fontSize: 24, fontWeight: '800', letterSpacing: 0.3 },
  pill: {
    backgroundColor: S.accentSoft,
    borderColor: S.accentBorder,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 6,
    paddingHorizontal: 7,
    paddingVertical: 3,
  },
  pillTxt: { color: S.accent, fontSize: 10, fontWeight: '800', letterSpacing: 1.4 },
  tagline: {
    color: S.sub, fontSize: 13.5, textAlign: 'center',
    marginTop: 10, lineHeight: 19, maxWidth: 300,
  },
  card: {
    backgroundColor: S.bgElevated,
    borderColor: S.border,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 20,
    padding: 20,
  },
  label: { color: S.muted, fontSize: 11, fontWeight: '800', letterSpacing: 1.2, marginBottom: 8 },
  labelRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginTop: 16,
  },
  forgotTxt: { color: S.accent, fontSize: 12, fontWeight: '700', marginBottom: 8 },
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
  pwRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  eye: {
    width: 44, height: 44, borderRadius: 12, alignItems: 'center', justifyContent: 'center',
    backgroundColor: S.field,
    borderColor: S.border, borderWidth: StyleSheet.hairlineWidth,
  },
  error: { color: S.red, fontSize: 13, marginTop: 12 },
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
}));