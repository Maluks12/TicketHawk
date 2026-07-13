import React, { useState } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, ScrollView,
  KeyboardAvoidingView, Platform, ActivityIndicator,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Feather, Ionicons } from '@expo/vector-icons';
import { EnvelopeSimple } from 'phosphor-react-native';
import { useRouter, useLocalSearchParams } from 'expo-router';
import { api } from '../../lib/api';
import { s, sheet, fieldSheet } from '../../styles/auth';
import { useAppTheme } from '../../theme/colors';

/**
 * /auth/forgot — request a password-reset link.
 *
 * POSTs to /v1/auth/reset_password_request. The backend always answers the
 * same way whether or not the account exists (enumeration-safe), so this
 * screen shows the same "check your email" state either way. The user
 * completes the reset on the web page (/reset-password?token=…) that the
 * email links to; the link expires in 30 minutes.
 */

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export default function ForgotPasswordScreen() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { isDark, colors } = useAppTheme();
  const params = useLocalSearchParams<{ email?: string }>();

  const [email, setEmail]         = useState(params.email ?? '');
  const [focused, setFocused]     = useState(false);
  const [loading, setLoading]     = useState(false);
  const [fieldErr, setFieldErr]   = useState<string | null>(null);
  const [globalErr, setGlobalErr] = useState<string | null>(null);
  const [sent, setSent]           = useState(false);

  const valid = EMAIL_RE.test(email.trim());

  const handleSend = async () => {
    const trimmed = email.trim().toLowerCase();
    if (!EMAIL_RE.test(trimmed)) {
      setFieldErr('Enter a valid email address');
      return;
    }
    setFieldErr(null);
    setGlobalErr(null);
    setLoading(true);
    try {
      await api.post('/v1/auth/reset_password_request', { email: trimmed });
      setSent(true);
    } catch (err: any) {
      // 3-per-IP rate limit on this endpoint
      setGlobalErr(err?.response?.status === 429
        ? 'Too many requests. Please wait a few minutes and try again.'
        : 'Something went wrong. Check your connection and try again.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <View style={{ flex: 1, backgroundColor: colors.background }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <ScrollView
          className={s.scroll}
          contentContainerStyle={{ paddingTop: insets.top + 16, paddingBottom: insets.bottom + 24, flexGrow: 1 }}
          keyboardShouldPersistTaps="handled"
        >
          {/* Back */}
          <TouchableOpacity
            onPress={() => router.back()}
            hitSlop={{ top: 10, bottom: 10, left: 10, right: 10 }}
            style={{ alignSelf: 'flex-start', marginBottom: 8 }}
          >
            <Ionicons name="chevron-back" size={26} color={colors.text} />
          </TouchableOpacity>

          {sent ? (
            /* ── Success ─────────────────────────────────────────────── */
            <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', gap: 14, paddingHorizontal: 8 }}>
              <View style={{
                width: 76, height: 76, borderRadius: 24, alignItems: 'center', justifyContent: 'center',
                backgroundColor: isDark ? 'rgba(46,204,113,0.12)' : 'rgba(46,204,113,0.10)',
              }}>
                <EnvelopeSimple size={36} color="#2ecc71" weight="bold" />
              </View>
              <Text className={s.brandTitle} style={{ color: colors.text, textAlign: 'center' }}>
                Check your email
              </Text>
              <Text className={s.brandSub} style={{ color: colors.subText, textAlign: 'center' }}>
                If an account exists for {email.trim()}, we've sent a link to reset your password. The link opens in your browser and expires in 30 minutes.
              </Text>
              <TouchableOpacity
                style={[sheet.cta, { backgroundColor: isDark ? '#ffffff' : '#0a0a0a', alignSelf: 'stretch', marginTop: 10 }]}
                onPress={() => router.back()} activeOpacity={0.85}
              >
                <Text className={s.ctaText} style={{ color: isDark ? '#0a0a0a' : '#ffffff' }}>Back to login</Text>
              </TouchableOpacity>
            </View>
          ) : (
            /* ── Request form ────────────────────────────────────────── */
            <View>
              <View className={s.brand}>
                <Text className={s.brandTitle} style={{ color: colors.text }}>Reset password</Text>
                <Text className={s.brandSub} style={{ color: colors.subText }}>
                  Enter the email for your account and we'll send you a link to reset your password.
                </Text>
              </View>

              {globalErr && (
                <View className={s.errBanner}>
                  <Ionicons name="alert-circle-outline" size={15} color="#ff3c6e" />
                  <Text className={s.errBannerText}>{globalErr}</Text>
                </View>
              )}

              <View className={s.form}>
                <View style={fieldSheet.wrap}>
                  <View style={[
                    fieldSheet.box,
                    isDark && { backgroundColor: '#1c1c1c', borderColor: '#333' },
                    focused && (isDark ? { borderColor: '#666' } : fieldSheet.boxFocused),
                    !!fieldErr && fieldSheet.boxError,
                  ]}>
                    <TextInput
                      style={[fieldSheet.input, isDark && { color: '#ffffff' }]}
                      value={email}
                      onChangeText={(v) => { setEmail(v); setFieldErr(null); }}
                      placeholder="Email address"
                      placeholderTextColor={isDark ? 'rgba(255,255,255,0.32)' : 'rgba(10,10,10,0.32)'}
                      keyboardType="email-address"
                      autoCapitalize="none"
                      autoCorrect={false}
                      autoFocus={!params.email}
                      returnKeyType="done"
                      onFocus={() => setFocused(true)}
                      onBlur={() => setFocused(false)}
                      onSubmitEditing={handleSend}
                    />
                    <Feather name="mail" size={19} color={isDark ? 'rgba(255,255,255,0.4)' : 'rgba(10,10,10,0.4)'} />
                  </View>
                  {fieldErr ? <Text style={fieldSheet.err}>{fieldErr}</Text> : null}
                </View>

                <TouchableOpacity
                  style={[
                    sheet.cta,
                    loading && sheet.ctaDisabled,
                    { backgroundColor: valid ? (isDark ? '#ffffff' : '#0a0a0a') : (isDark ? '#232326' : '#e2e2e6') },
                  ]}
                  onPress={handleSend} activeOpacity={0.85} disabled={loading || !valid}
                >
                  {loading
                    ? <ActivityIndicator color={isDark ? '#000' : '#fff'} size="small" />
                    : <Text className={s.ctaText} style={{ color: valid ? (isDark ? '#0a0a0a' : '#ffffff') : (isDark ? 'rgba(255,255,255,0.35)' : 'rgba(10,10,10,0.35)') }}>Send reset link</Text>}
                </TouchableOpacity>

                <TouchableOpacity className={s.switchRow} onPress={() => router.back()}>
                  <Text className={s.switchText} style={{ color: colors.subText }}>
                    Remembered it?{'  '}<Text className={s.switchLink}>Log in</Text>
                  </Text>
                </TouchableOpacity>
              </View>
            </View>
          )}
        </ScrollView>
      </KeyboardAvoidingView>
    </View>
  );
}