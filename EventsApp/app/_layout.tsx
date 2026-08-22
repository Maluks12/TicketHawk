import '../global.css';
import React, { useEffect, useRef } from 'react';
import { Stack } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { useThemeStore } from '../store/themeStore';
import { useAuthStore } from '../store/authStore';
import { usePushNotifications } from '../lib/usepush';

// ─── Splash control ──────────────────────────────────────────────────────────
// Called at MODULE scope, not inside the component. By the time a component
// effect runs, expo has already auto-hidden the splash — which is why the
// splash used to flash past in a few hundred milliseconds and hand over to a
// half-empty home feed. preventAutoHideAsync() has to win the race with the
// first frame, and module scope is the only place that reliably does.
SplashScreen.preventAutoHideAsync().catch(() => {});

// Cross-fade out instead of cutting. 400ms is long enough to read as a
// deliberate transition and short enough not to feel like extra waiting.
SplashScreen.setOptions({ duration: 400, fade: true });

// Floor on how long the splash is shown, regardless of how fast the data
// arrives. Two purposes: it gives the home feed's first fetch time to land so
// the app opens onto content rather than skeletons, and it stops the splash
// strobing on a warm start.
const MIN_SPLASH_MS = 2000;
// Ceiling. If session restore wedges (dead tunnel, captive portal), the user
// gets the app rather than an indefinite logo.
const MAX_SPLASH_MS = 5000;

// Measured from module load rather than from component mount, so the 2s floor
// covers the JS bundle parse too instead of starting after it.
const SPLASH_START = Date.now();

export default function RootLayout() {
  const loadMode  = useThemeStore((s) => s.loadMode);
  const loadUser  = useAuthStore((s) => s.loadUser);
  const authReady = useAuthStore((s) => s.authReady);

  const hidden = useRef(false);

  const hideSplash = React.useCallback(() => {
    if (hidden.current) return;
    hidden.current = true;
    SplashScreen.hideAsync().catch(() => {});
  }, []);

  // Hide once BOTH conditions hold: the minimum time has elapsed, and session
  // restore has settled. authReady is the right signal to wait on — it is what
  // the home feed itself gates its single startup fetch on, so hiding before
  // it flips would show a feed that is guaranteed to still be empty.
  useEffect(() => {
    if (!authReady) return;
    const elapsed   = Date.now() - SPLASH_START;
    const remaining = Math.max(0, MIN_SPLASH_MS - elapsed);
    const t = setTimeout(hideSplash, remaining);
    return () => clearTimeout(t);
  }, [authReady, hideSplash]);

  // Failsafe.
  useEffect(() => {
    const t = setTimeout(hideSplash, MAX_SPLASH_MS);
    return () => clearTimeout(t);
  }, [hideSplash]);

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

      {/* Organiser public page. Registered explicitly for the same reason
          profile/delete-account is: an unlisted route gets expo-router's
          default presentation rather than the slide-from-right every other
          pushed screen in this app uses, so it arrives feeling like a
          different app. */}
      <Stack.Screen
        name="organiser/[slug]"
        options={{ animation: 'slide_from_right' }}
      />

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

      {/* Account deletion (Apple 5.1.1(v)). Registered explicitly so it gets a
          normal push animation rather than expo-router's default for an
          unlisted route. */}
      <Stack.Screen
        name="profile/delete-account"
        options={{ animation: 'slide_from_right' }}
      />
    </Stack>
  );
}