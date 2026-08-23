import React, { useCallback, useEffect, useState } from 'react';
import { View, Text, TouchableOpacity, ActivityIndicator, Platform, Alert } from 'react-native';
import { Ionicons } from '@expo/vector-icons';
import * as AppleAuthentication from 'expo-apple-authentication';
import { useAuthStore } from '../store/authStore';
import {
  isAppleAvailable, isGoogleSignInAvailable, signInWithApple, signInWithGoogle,
  SocialAuthCancelled, SocialAuthProviderError, type SocialCredential,
} from '../lib/socialAuth';
import { useAppTheme } from '../theme/colors';

interface Props {
  /** Called after a successful sign-in, with whether the account is brand new. */
  onSuccess?: (info: { created: boolean; needsInfo: boolean }) => void;
  /** Label above the divider — "or sign in with" vs "or sign up with". */
  dividerLabel?: string;
  disabled?: boolean;
}

/**
 * Google + Apple sign-in buttons.
 *
 * ── On button order and appearance ──────────────────────────────────────────
 *
 * Apple's Human Interface Guidelines require the Sign in with Apple button to
 * be at least as prominent as other sign-in options, which in practice means
 * showing it first on iOS and not making it visually smaller. Reviewers do
 * check this. AppleAuthenticationButton is used rather than a hand-rolled
 * button because it renders Apple's exact approved artwork and localises the
 * label automatically — a custom Apple button is a rejection risk for no gain.
 *
 * Apple is hidden entirely on Android (the native module doesn't exist there);
 * Google shows on both.
 */
export default function SocialAuthButtons({
  onSuccess,
  dividerLabel = 'or continue with',
  disabled,
}: Props) {
  const { isDark, colors } = useAppTheme();
  const socialLogin = useAuthStore((s) => s.socialLogin);

  const [appleReady, setAppleReady] = useState(false);
  const [busy, setBusy] = useState<null | 'google' | 'apple'>(null);

  useEffect(() => { isAppleAvailable().then(setAppleReady); }, []);

  const run = useCallback(async (
    provider: 'google' | 'apple',
    getCredential: () => Promise<SocialCredential>,
  ) => {
    if (busy) return;
    setBusy(provider);
    try {
      const cred = await getCredential();
      const result = await socialLogin({
        provider: cred.provider,
        id_token: cred.idToken,
        nonce: cred.nonce,
        first_name: cred.firstName,
        last_name: cred.lastName,
      });
      onSuccess?.(result);
    } catch (err: any) {
      // A cancel is a normal outcome, not an error. Alerting on it is the
      // single most irritating thing a social login can do.
      if (err instanceof SocialAuthCancelled) return;

      // A provider-side failure that never reached our API. Its message is the
      // provider's own, and on a production build this alert is the only place
      // it will ever be visible — so it must NOT be collapsed into the generic
      // "no connection" case below, which is what hid a Google config error
      // behind a misleading "check your connection".
      if (err instanceof SocialAuthProviderError) {
        Alert.alert('Sign-in failed', err.message);
        return;
      }

      const status = err?.response?.status;
      const serverMessage = err?.response?.data?.error;

      if (status === 403 && serverMessage) {
        // Deleted account — the message is specific and actionable.
        Alert.alert('Account unavailable', serverMessage);
      } else if (status === 501) {
        Alert.alert(
          'Unavailable',
          'That sign-in method isn\u2019t available right now. Please use your email and password.',
        );
      } else if (!err?.response) {
        Alert.alert('No connection', 'Check your connection and try again.');
      } else {
        Alert.alert('Sign-in failed', serverMessage ?? 'Something went wrong. Please try again.');
      }
    } finally {
      setBusy(null);
    }
  }, [busy, socialLogin, onSuccess]);

  const isDisabled = !!disabled || !!busy;
  const showApple = Platform.OS === 'ios' && appleReady;

  // False in Expo Go, where the RNGoogleSignin native module is not in the
  // binary. Hidden rather than shown-and-failing: a sign-in button that throws
  // on tap is worse than one that is absent, because the user cannot tell it
  // from their account being broken.
  //
  // Computed once at render rather than in state — isGoogleSignInAvailable()
  // is a cached require() after the first call, so there is nothing to await
  // and nothing to re-check.
  const showGoogle = isGoogleSignInAvailable();

  return (
    <View style={{ width: '100%' }}>
      {/* Divider */}
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 12, marginVertical: 20 }}>
        <View style={{ flex: 1, height: 1, backgroundColor: colors.dividerColor }} />
        <Text style={{ color: colors.subText, fontSize: 13 }}>{dividerLabel}</Text>
        <View style={{ flex: 1, height: 1, backgroundColor: colors.dividerColor }} />
      </View>

      {/* Apple first on iOS — HIG prominence requirement. */}
      {showApple && (
        <View style={{ marginBottom: 12 }}>
          {busy === 'apple' ? (
            <View style={{ height: 50, alignItems: 'center', justifyContent: 'center' }}>
              <ActivityIndicator color={colors.ctaBg} />
            </View>
          ) : (
            <AppleAuthentication.AppleAuthenticationButton
              buttonType={AppleAuthentication.AppleAuthenticationButtonType.CONTINUE}
              buttonStyle={
                isDark
                  ? AppleAuthentication.AppleAuthenticationButtonStyle.WHITE
                  : AppleAuthentication.AppleAuthenticationButtonStyle.BLACK
              }
              cornerRadius={12}
              style={{ width: '100%', height: 50 }}
              onPress={() => { if (!isDisabled) void run('apple', signInWithApple); }}
            />
          )}
        </View>
      )}

      {showGoogle && (
      <TouchableOpacity
        accessibilityRole="button"
        accessibilityLabel="Continue with Google"
        disabled={isDisabled}
        onPress={() => void run('google', signInWithGoogle)}
        style={{
          height: 50,
          borderRadius: 12,
          flexDirection: 'row',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 10,
          backgroundColor: colors.cardBg,
          borderWidth: 1,
          borderColor: colors.dividerColor,
          opacity: isDisabled ? 0.6 : 1,
        }}
      >
        {busy === 'google' ? (
          <ActivityIndicator color={colors.ctaBg} />
        ) : (
          <>
            <Ionicons name="logo-google" size={19} color={colors.text} />
            <Text style={{ color: colors.text, fontSize: 16, fontWeight: '600' }}>
              Continue with Google
            </Text>
          </>
        )}
      </TouchableOpacity>
      )}

      {/* Neither provider available — Expo Go on Android, for instance. The
          divider above would otherwise sit over nothing. */}
      {!showApple && !showGoogle && (
        <Text style={{ color: colors.subText, fontSize: 12, textAlign: 'center' }}>
          Social sign-in needs a development build. Use your email and password.
        </Text>
      )}
    </View>
  );
}