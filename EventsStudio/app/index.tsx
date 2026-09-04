import React, { useEffect } from 'react';
import { View, ActivityIndicator } from 'react-native';
import { useRouter } from 'expo-router';
import { useAuthStore } from '../store/authStore';
import { useStudioTheme } from '../components/studio/theme';

// ─────────────────────────────────────────────────────────────────────────────
// Auth gate.
//
// Waits for the initial session restore (authReady) so we never flash the login
// screen at a signed-in user, then routes by role:
//
//   organiser / admin → /organiser        (full Studio dashboard)
//   any other user    → /staff/join       (redeem a join code, pick a scan event)
//   no session        → /auth/login
//
// replace() is used everywhere so the gate never sits in the back stack.
// ─────────────────────────────────────────────────────────────────────────────
export default function Gate() {
  const router = useRouter();
  const user = useAuthStore((s) => s.user);
  const authReady = useAuthStore((s) => s.authReady);
  const { S } = useStudioTheme();

  useEffect(() => {
    if (!authReady) return;
    if (!user) {
      router.replace('/auth/login' as any);
    } else if (user.role === 'organizer' || user.role === 'admin') {
      router.replace('/organiser' as any);
    } else {
      router.replace('/staff/join' as any);
    }
  }, [authReady, user]);

  return (
    <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: S.bg }}>
      <ActivityIndicator color={S.accent} />
    </View>
  );
}