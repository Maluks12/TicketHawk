// ─────────────────────────────────────────────────────────────────────────────
// usePushNotifications — lifecycle glue between auth state, the router and
// lib/push.ts. Mounted once, in app/_layout.tsx.
//
// Registration is deliberately tied to the AUTH session rather than to app
// start. A push token is meaningless without a user to attach it to: the
// backend keys push_tokens on user_id, so registering before sign-in would have
// nothing to write. The sequence that matters:
//
//   authReady && user   → register (prompt on the first sign-in of an install)
//   user → null         → unregister, clear badge
//
// It also re-registers on every foreground. Expo rotates tokens without
// notice — reinstalls, some OS updates, FCM's own rotation — and a rotated
// token that the server never learns about is the classic "notifications just
// stopped working for one user" bug. The check is local and cheap: nothing is
// sent unless the token actually differs from the one last registered.
// ─────────────────────────────────────────────────────────────────────────────

import { useEffect, useRef } from 'react';
import { AppState, AppStateStatus } from 'react-native';
import { useRouter } from 'expo-router';
import { useAuthStore } from '../store/authStore';
import {
  attachNotificationListeners,
  clearBadge,
  configureNotificationHandler,
  isPushPreferenceEnabled,
  registerForPushNotifications,
  syncBadgeFromServer,
  unregisterPushNotifications,
} from './push';

// Set the foreground presentation behaviour once, at module load, before any
// notification can arrive. Doing this inside a component effect races with a
// notification that lands during the first render.
configureNotificationHandler();

export function usePushNotifications() {
  const router = useRouter();
  const user = useAuthStore((s) => s.user);
  const authReady = useAuthStore((s) => s.authReady);

  // Tracks whether we've already prompted this session, so a foreground
  // re-registration never re-triggers the OS permission dialog.
  const promptedRef = useRef(false);
  const lastUserIdRef = useRef<string | null>(null);

  // ── Tap routing ────────────────────────────────────────────────────────────
  useEffect(() => {
    const cleanup = attachNotificationListeners(
      (path) => {
        // expo-router types push() narrowly; these paths are built from a
        // closed set of notification types in routeForNotification().
        router.push(path as any);
      },
      () => {
        // Arrived while the app was open — the OS won't touch the badge for a
        // foreground notification, so pull the authoritative count.
        void syncBadgeFromServer();
      }
    );
    return cleanup;
  }, [router]);

  // ── Register / unregister on auth transitions ──────────────────────────────
  useEffect(() => {
    if (!authReady) return; // wait for session restore to settle

    const currentId = user?.id ?? null;
    const previousId = lastUserIdRef.current;
    lastUserIdRef.current = currentId;

    // Signed out (or switched accounts) — drop this device from the previous
    // account before doing anything else, so notifications can't leak to a
    // handset whose owner has signed out.
    if (previousId && previousId !== currentId) {
      void unregisterPushNotifications();
      void clearBadge();
    }

    if (!currentId) return;

    void (async () => {
      if (!(await isPushPreferenceEnabled())) {
        // Toggle is off. Make sure the server agrees — covers the case where
        // the toggle was flipped off while offline and the DELETE never landed.
        await unregisterPushNotifications();
        return;
      }

      // Prompt only on the first registration attempt of this app session.
      // Signing in is a reasonable moment to ask: the user has just committed
      // to an account, and every notification the platform sends is about
      // something they own — their bookings, their followed events.
      const shouldPrompt = !promptedRef.current;
      promptedRef.current = true;

      await registerForPushNotifications(shouldPrompt);
      await syncBadgeFromServer();
    })();
  }, [authReady, user?.id]);

  // ── Refresh on foreground ──────────────────────────────────────────────────
  useEffect(() => {
    const sub = AppState.addEventListener('change', (state: AppStateStatus) => {
      if (state !== 'active') return;
      if (!useAuthStore.getState().user) return;

      void (async () => {
        if (!(await isPushPreferenceEnabled())) return;
        // promptIfNeeded = false: never surprise someone with a permission
        // dialog just for opening the app.
        await registerForPushNotifications(false);
        await syncBadgeFromServer();
      })();
    });
    return () => sub.remove();
  }, []);
}