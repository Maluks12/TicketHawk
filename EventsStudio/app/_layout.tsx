import React, { useEffect } from 'react';
import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { View } from 'react-native';
import { useAuthStore } from '../store/authStore';
import { STUDIO } from '../components/studio/theme';

// ─────────────────────────────────────────────────────────────────────────────
// TicketHawk Studio — root layout.
//
// The whole app is the dark Studio surface; there is no consumer theme, no tab
// bar and no light mode. Auth is restored once on launch (offline-tolerant:
// authStore falls back to the cached profile when there's no signal, which is
// exactly the situation at a venue door).
// ─────────────────────────────────────────────────────────────────────────────
export default function RootLayout() {
  const loadUser = useAuthStore((s) => s.loadUser);

  useEffect(() => {
    loadUser();
  }, []);

  return (
    <View style={{ flex: 1, backgroundColor: STUDIO.bg }}>
      <StatusBar style="light" />
      <Stack
        screenOptions={{
          headerShown: false,
          contentStyle: { backgroundColor: STUDIO.bg },
          animation: 'slide_from_right',
        }}
      >
        {/* Auth gate — decides where a launch lands. */}
        <Stack.Screen name="index" options={{ animation: 'fade' }} />

        <Stack.Screen name="auth/login" options={{ animation: 'fade' }} />

        {/* Organiser Studio — dashboard, event drill-down, scanner, staff. */}
        <Stack.Screen name="organiser" options={{ animation: 'fade' }} />

        {/* Staff home — join-code redemption + "events I can scan". */}
        <Stack.Screen name="staff/join" options={{ animation: 'fade' }} />
      </Stack>
    </View>
  );
}
