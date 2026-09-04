import React, { useEffect } from 'react';
import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { View } from 'react-native';
import { useAuthStore } from '../store/authStore';
import { useThemeStore } from '../store/themeStore';
import { useStudioTheme } from '../components/studio/theme';

// ─────────────────────────────────────────────────────────────────────────────
// TicketHawk Studio — root layout.
//
// Two things are restored once on launch, both before the first real screen:
//   • the session (offline-tolerant: authStore falls back to the cached
//     profile when there's no signal, which is exactly the situation at a
//     venue door), and
//   • the appearance preference.
//
// The Studio keeps its own identity — indigo accent, its own screens, no
// consumer tab bar — but it now honours light and dark like everything else on
// the phone. `auto` follows the device; app.json sets userInterfaceStyle to
// "automatic" so the OS actually reports the real scheme instead of pinning it
// to dark.
// ─────────────────────────────────────────────────────────────────────────────
export default function RootLayout() {
  const loadUser = useAuthStore((s) => s.loadUser);
  const loadMode = useThemeStore((s) => s.loadMode);
  const { S, isDark } = useStudioTheme();

  useEffect(() => {
    loadUser();
    loadMode();
  }, []);

  return (
    <View style={{ flex: 1, backgroundColor: S.bg }}>
      <StatusBar style={isDark ? 'light' : 'dark'} />
      <Stack
        screenOptions={{
          headerShown: false,
          contentStyle: { backgroundColor: S.bg },
          animation: 'slide_from_right',
        }}
      >
        {/* Auth gate — decides where a launch lands. */}
        <Stack.Screen name="index" options={{ animation: 'fade' }} />

        <Stack.Screen name="auth/login" options={{ animation: 'fade' }} />

        {/* Account creation. Studio needs its own sign-up so App Review (and
            any organiser who installs Studio first) can get in without being
            sent to another app. */}
        <Stack.Screen name="auth/signup" options={{ animation: 'slide_from_right' }} />

        {/* Password reset request. Studio is the app people install at a venue
            an hour before doors — the worst possible moment to discover the
            only way back into a forgotten account is another app. */}
        <Stack.Screen name="auth/forgot" options={{ animation: 'slide_from_right' }} />

        {/* Organiser Studio — dashboard, event drill-down, scanner, staff. */}
        <Stack.Screen name="organiser" options={{ animation: 'fade' }} />

        {/* Staff home — join-code redemption + "events I can scan". */}
        <Stack.Screen name="staff/join" options={{ animation: 'fade' }} />
      </Stack>
    </View>
  );
}