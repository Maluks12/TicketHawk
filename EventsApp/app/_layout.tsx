import '../global.css';
import React, { useEffect } from 'react';
import { Stack } from 'expo-router';
import { useThemeStore } from '../store/themeStore';

export default function RootLayout() {
  const loadMode = useThemeStore((s) => s.loadMode);
  useEffect(() => { loadMode(); }, []);

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
        name="booking/[id]"
        options={{ animation: 'slide_from_right' }}
      />
    </Stack>
  );
}