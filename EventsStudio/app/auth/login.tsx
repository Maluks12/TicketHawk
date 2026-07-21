import React, { useState } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, StyleSheet,
  KeyboardAvoidingView, Platform, ActivityIndicator, Image,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { useAuthStore } from '../../store/authStore';
import { STUDIO } from '../../components/studio/theme';

// ─────────────────────────────────────────────────────────────────────────────
// Studio login.
//
// One account system across TicketHawk — organisers and door staff sign in with
// the same credentials they use in the consumer app. There is deliberately no
// sign-up here: accounts are created in the consumer app or on the web, then
// this app is where they *work*. After login the auth gate (app/index.tsx)
// routes by role.
// ─────────────────────────────────────────────────────────────────────────────
export default function StudioLogin() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const login = useAuthStore((s) => s.login);

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
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        style={{ flex: 1, justifyContent: 'center' }}
      >
        {/* Brand */}
        <View style={styles.brand}>
          <Image source={require('../../assets/icon.png')} style={styles.logo} />
          <View style={styles.brandLine}>
            <Text style={styles.brandWord}>TicketHawk</Text>
            <View style={styles.pill}>
              <Text style={styles.pillTxt}>STUDIO</Text>
            </View>
          </View>
          <Text style={styles.tagline}>
            Scan tickets at the door and track your events — sign in with your TicketHawk account.
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
            placeholderTextColor={STUDIO.muted}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="email-address"
            textContentType="emailAddress"
          />

          <Text style={[styles.label, { marginTop: 16 }]}>PASSWORD</Text>
          <View style={styles.pwRow}>
            <TextInput
              style={[styles.input, { flex: 1 }]}
              value={password}
              onChangeText={setPassword}
              placeholder="Your password"
              placeholderTextColor={STUDIO.muted}
              secureTextEntry={!showPw}
              autoCapitalize="none"
              textContentType="password"
              onSubmitEditing={submit}
              returnKeyType="go"
            />
            <TouchableOpacity onPress={() => setShowPw((v) => !v)} hitSlop={10} style={styles.eye}>
              <Ionicons name={showPw ? 'eye-off-outline' : 'eye-outline'} size={20} color={STUDIO.sub} />
            </TouchableOpacity>
          </View>

          {error && <Text style={styles.error}>{error}</Text>}

          <TouchableOpacity style={styles.btn} onPress={submit} disabled={busy} activeOpacity={0.85}>
            {busy ? (
              <ActivityIndicator color="#fff" />
            ) : (
              <Text style={styles.btnTxt}>Sign in</Text>
            )}
          </TouchableOpacity>

          <Text style={styles.hint}>
            No account? Create one in the TicketHawk app, then come back here to scan.
          </Text>
        </View>
      </KeyboardAvoidingView>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: STUDIO.bg, paddingHorizontal: 22 },
  brand: { alignItems: 'center', marginBottom: 30 },
  logo: { width: 64, height: 64, borderRadius: 16, marginBottom: 14 },
  brandLine: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  brandWord: { color: STUDIO.text, fontSize: 24, fontWeight: '800', letterSpacing: 0.3 },
  pill: {
    backgroundColor: STUDIO.accentSoft,
    borderColor: STUDIO.accentBorder,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 6,
    paddingHorizontal: 7,
    paddingVertical: 3,
  },
  pillTxt: { color: STUDIO.accent, fontSize: 10, fontWeight: '800', letterSpacing: 1.4 },
  tagline: {
    color: STUDIO.sub, fontSize: 13.5, textAlign: 'center',
    marginTop: 10, lineHeight: 19, maxWidth: 300,
  },
  card: {
    backgroundColor: STUDIO.bgElevated,
    borderColor: STUDIO.border,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 20,
    padding: 20,
  },
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
  pwRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  eye: {
    width: 44, height: 44, borderRadius: 12, alignItems: 'center', justifyContent: 'center',
    backgroundColor: STUDIO.surface,
    borderColor: STUDIO.border, borderWidth: StyleSheet.hairlineWidth,
  },
  error: { color: STUDIO.red, fontSize: 13, marginTop: 12 },
  btn: {
    backgroundColor: STUDIO.accentDeep,
    borderRadius: 13,
    alignItems: 'center',
    paddingVertical: 14,
    marginTop: 18,
  },
  btnTxt: { color: '#fff', fontSize: 15.5, fontWeight: '800' },
  hint: { color: STUDIO.muted, fontSize: 12.5, textAlign: 'center', marginTop: 16, lineHeight: 18 },
});
