import React from 'react';
import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { View } from 'react-native';
import { STUDIO } from '../../components/studio/theme';

// The Organiser Studio runs inside its own stack so it feels like a separate
// app: a fixed dark surface, its own screens, and no consumer tab bar. The
// section is mounted from the root layout as a full-screen modal.
export default function StudioLayout() {
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
        <Stack.Screen name="index" options={{ animation: 'fade' }} />
        <Stack.Screen name="event/[id]" />
        <Stack.Screen
          name="scan"
          options={{ animation: 'slide_from_bottom', gestureEnabled: false }}
        />
      </Stack>
    </View>
  );
}