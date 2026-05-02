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
        name="event/purchase"
        options={{
          animation: 'slide_from_right',
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
        options={{
          animation: 'slide_from_right',
        }}
      />
    </Stack>
  );
}