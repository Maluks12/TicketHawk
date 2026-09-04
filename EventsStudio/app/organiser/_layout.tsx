import React from 'react';
import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { View } from 'react-native';
import { useStudioTheme } from '../../components/studio/theme';

// The Organiser Studio runs inside its own stack so it feels like a separate
// app: its own screens and no consumer tab bar. The surface follows the
// appearance preference (auto / light / dark) rather than being pinned dark.
export default function StudioLayout() {
  const { S, isDark } = useStudioTheme();
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