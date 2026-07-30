import '../global.css';
import React, { useEffect } from 'react';
import { Stack } from 'expo-router';
import { useThemeStore } from '../store/themeStore';
import { useAuthStore } from '../store/authStore';
import { usePushNotifications } from '../lib/usepush';

export default function RootLayout() {
  const loadMode = useThemeStore((s) => s.loadMode);
  const loadUser = useAuthStore((s) => s.loadUser);

  // Push notifications: permission, token registration and tap routing.
  // Mounted here (not in a tab) so a notification tapped from a killed app is
  // handled during the very first render, before any screen exists.
  usePushNotifications();

  useEffect(() => {
    loadMode();
    // Validate the stored access token on every app start.
    // If it has expired, loadUser calls GET /v1/user/me → 401 → axios
    // interceptor refreshes the token automatically.
    // Without this, an expired token is silently ignored by OptionalAuth
    // on /v1/home (no 401 returned), so "For You" never appears and the
    // token is never refreshed until the user manually logs in again.
    loadUser();
  }, []);

  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen name="(tabs)" />

      <Stack.Screen name="event/[id]" />

      <Stack.Screen
        name="category/[name]"
        options={{ animation: 'slide_from_right' }}
      />

      <Stack.Screen
        name="event/purchase"
        options={{ animation: 'slide_from_right' }}
      />

      {/* Payment WebView screen — registered so expo-router knows about it */}
      <Stack.Screen
        name="payment/[id]"
        options={{
          animation: 'slide_from_bottom',
          // Prevent swipe-to-dismiss mid-payment
          gestureEnabled: false,
        }}
      />

      <Stack.Screen
        name="auth/login"
        options={{
          presentation: 'modal',
          animation: 'slide_from_bottom',
        }}
      />

      <Stack.Screen
        name="auth/forgot"
        options={{ animation: 'slide_from_right' }}
      />

      <Stack.Screen
        name="booking/[id]"
        options={{ animation: 'slide_from_right' }}
      />
    </Stack>
  );
}