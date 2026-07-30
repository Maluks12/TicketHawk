// ─────────────────────────────────────────────────────────────────────────────
// TicketHawk — out-of-app (push) notifications
//
// Everything device-side for push lives here: permissions, Android channels,
// Expo token registration with the backend, and tap routing.
//
// HOW THE WHOLE THING FITS TOGETHER
//
//   backend  repository.CreateNotification()   ← every notification producer
//              ├─ writes the notifications row (durable, shown in-app)
//              └─ fires the push dispatcher
//                   └─ services.PushService batches → Expo → APNs/FCM
//                                                            │
//   device   ◄────────────────────────────────────────────────┘
//              ├─ app in FOREGROUND  → notificationListener  (banner via handler)
//              ├─ app BACKGROUNDED   → OS shows it; tap → responseListener
//              └─ app KILLED         → OS shows it; tap → cold start, picked up
//                                       by getLastNotificationResponseAsync()
//
// THE FEED IS THE SOURCE OF TRUTH, NOT THE PUSH. A push can be dropped by the
// OS, the device can be off, the token can be stale. The notifications row on
// the server always exists, so nothing is ever lost — it just isn't announced.
// Never put information in a push that isn't also in the feed row.
//
// ── BUILD REQUIREMENT ────────────────────────────────────────────────────────
// Remote push does NOT work in Expo Go on Android (removed in SDK 53) and is
// unreliable in Expo Go on iOS. You need a development build:
//
//     npx expo install expo-notifications expo-device
//     eas build --profile development --platform android
//
// You also need push credentials, which EAS generates on first build:
//   • Android — an FCM V1 service-account JSON uploaded to the Expo project
//     (expo.dev → project → Credentials → Android → FCM V1). Without this,
//     tokens are issued but nothing is ever delivered, silently.
//   • iOS — an APNs key, which `eas credentials` creates automatically.
//
// The functions below degrade quietly when push is unavailable (simulator,
// Expo Go, permission denied) rather than throwing, so the app is fully usable
// without it.
// ─────────────────────────────────────────────────────────────────────────────

import { Platform } from 'react-native';
import * as Notifications from 'expo-notifications';
import * as Device from 'expo-device';
import Constants, { ExecutionEnvironment } from 'expo-constants';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { api } from './api';

// Mirrors NOTIFS_KEY in app/(tabs)/profile.tsx — the user-facing Settings
// toggle. Exported so the settings screen and this module can never drift onto
// different keys.
export const PUSH_PREF_KEY = 'prefs.notifications.enabled';

// Last token we successfully registered with the backend. Used to avoid a
// pointless POST on every cold start when nothing has changed, and to know
// which token to unregister on logout (by which point getExpoPushTokenAsync
// may no longer be callable).
const PUSH_TOKEN_KEY = 'push.token.registered';

// ─── Notification handler ────────────────────────────────────────────────────
//
// Controls what happens when a push arrives while the app is OPEN. Without
// this, foreground notifications are delivered to JS but never shown — which
// looks exactly like "push is broken" during testing.
//
// NOTE: shouldShowBanner / shouldShowList replaced the single shouldShowAlert
// field in SDK 53. Using the old name compiles but silently does nothing.
export function configureNotificationHandler() {
  Notifications.setNotificationHandler({
    handleNotification: async () => ({
      shouldShowBanner: true,
      shouldShowList: true,
      shouldPlaySound: true,
      shouldSetBadge: true,
    }),
  });
}

// ─── Android channels ────────────────────────────────────────────────────────
//
// Android 8+ requires every notification to name a channel, and the CHANNEL —
// not the app — owns importance, sound and vibration. Once created, a channel's
// settings belong to the user: changing importance in code afterwards is
// ignored. So get these right the first time; the only way to change them later
// is a new channel id.
//
// Splitting them out means someone who finds "selling fast" alerts noisy can
// mute that one channel in system settings instead of turning TicketHawk off
// entirely. These ids MUST match models.PushChannel() on the backend.
export async function ensureAndroidChannels() {
  if (Platform.OS !== 'android') return;

  await Notifications.setNotificationChannelAsync('default', {
    name: 'General',
    importance: Notifications.AndroidImportance.DEFAULT,
    vibrationPattern: [0, 250, 250, 250],
    lightColor: '#ff2d55',
  });

  // Transactional. The user paid money or was waiting on an outcome — these
  // should always arrive with sound, and users should be discouraged from
  // muting them.
  await Notifications.setNotificationChannelAsync('tickets', {
    name: 'Tickets & payments',
    description: 'Booking confirmations, refunds and ticket delivery.',
    importance: Notifications.AndroidImportance.HIGH,
    vibrationPattern: [0, 250, 250, 250],
    lightColor: '#ff2d55',
  });

  await Notifications.setNotificationChannelAsync('reminders', {
    name: 'Event reminders',
    description: 'Reminders for events you follow, 7 days and 1 day before.',
    importance: Notifications.AndroidImportance.DEFAULT,
    vibrationPattern: [0, 250],
    lightColor: '#ff2d55',
  });

  await Notifications.setNotificationChannelAsync('alerts', {
    name: 'Ticket alerts',
    description: 'Selling-fast, sold-out and cancellation alerts.',
    importance: Notifications.AndroidImportance.DEFAULT,
    lightColor: '#ff2d55',
  });
}

// ─── Capability check ────────────────────────────────────────────────────────

export type PushUnavailableReason =
  | 'simulator'
  | 'expo-go'
  | 'web'
  | null;

/**
 * Why push can't work here, or null if it can.
 *
 * Checked before touching the permission APIs so we never show a permission
 * prompt on a device that could not receive a notification anyway — a denied
 * prompt is sticky, and burning it on a simulator means the real prompt never
 * appears on that install.
 */
export function pushUnavailableReason(): PushUnavailableReason {
  if (Platform.OS === 'web') return 'web';
  if (!Device.isDevice) return 'simulator';
  // Expo Go dropped remote push on Android in SDK 53 and it is unreliable on
  // iOS. executionEnvironment is the reliable check; Constants.appOwnership is
  // deprecated and returns null in dev builds.
  if (Constants.executionEnvironment === ExecutionEnvironment.StoreClient) {
    return 'expo-go';
  }
  return null;
}

function getProjectId(): string | undefined {
  // EAS project id, needed by getExpoPushTokenAsync in SDK 49+. It lives in
  // app.json under extra.eas.projectId; the fallbacks cover older manifest
  // shapes so this doesn't break on an OTA update from an older runtime.
  return (
    Constants.expoConfig?.extra?.eas?.projectId ??
    (Constants as any).easConfig?.projectId ??
    (Constants as any).manifest2?.extra?.eas?.projectId
  );
}

// ─── Preference ──────────────────────────────────────────────────────────────

export async function isPushPreferenceEnabled(): Promise<boolean> {
  try {
    const v = await AsyncStorage.getItem(PUSH_PREF_KEY);
    return v === null ? true : v === '1'; // default ON, matching the Settings UI
  } catch {
    return true;
  }
}

export async function setPushPreference(enabled: boolean) {
  try {
    await AsyncStorage.setItem(PUSH_PREF_KEY, enabled ? '1' : '0');
  } catch {
    /* a failed pref write must not break the toggle */
  }
}

// ─── Registration ────────────────────────────────────────────────────────────

export type RegisterResult =
  | { ok: true; token: string }
  | { ok: false; reason: 'unavailable' | 'denied' | 'no-project-id' | 'error'; detail?: string };

/**
 * Obtain an Expo push token and register it with the backend.
 *
 * @param promptIfNeeded  Whether to show the OS permission dialog when
 *                        permission has not yet been asked for. Pass false for
 *                        silent refreshes on cold start; pass true only when
 *                        the user has done something that implies they want
 *                        notifications (signed in, flipped the toggle on).
 *
 * Safe to call repeatedly. The backend upsert is idempotent, and this skips the
 * network call entirely when the token is unchanged since last registration.
 */
export async function registerForPushNotifications(
  promptIfNeeded = false
): Promise<RegisterResult> {
  const unavailable = pushUnavailableReason();
  if (unavailable) {
    return { ok: false, reason: 'unavailable', detail: unavailable };
  }

  try {
    await ensureAndroidChannels();

    const existing = await Notifications.getPermissionsAsync();
    let status = existing.status;

    if (status !== 'granted') {
      // `canAskAgain === false` means the user has hard-denied; requesting
      // again is a no-op that returns 'denied' immediately. Nothing to do but
      // send them to system settings, which the caller handles.
      if (!promptIfNeeded || !existing.canAskAgain) {
        return { ok: false, reason: 'denied' };
      }
      const requested = await Notifications.requestPermissionsAsync();
      status = requested.status;
    }

    if (status !== 'granted') {
      return { ok: false, reason: 'denied' };
    }

    const projectId = getProjectId();
    if (!projectId) {
      // Without this the token call throws an unhelpful error. Surfacing it as
      // its own reason makes a misconfigured app.json obvious instead of
      // looking like a permissions problem.
      return { ok: false, reason: 'no-project-id' };
    }

    const { data: token } = await Notifications.getExpoPushTokenAsync({ projectId });

    // Re-register on every cold start even when unchanged? No — but DO
    // re-register whenever it differs, because Expo rotates tokens silently
    // (reinstall, some OS updates, FCM's own rotation). A stale server-side
    // token means notifications that just quietly stop arriving.
    const lastRegistered = await AsyncStorage.getItem(PUSH_TOKEN_KEY);
    if (lastRegistered !== token) {
      await api.post('/v1/notifications/push-token', {
        token,
        platform: Platform.OS === 'ios' ? 'ios' : 'android',
        app: 'consumer',
        device_name: [Device.manufacturer, Device.modelName].filter(Boolean).join(' ').slice(0, 120),
      });
      await AsyncStorage.setItem(PUSH_TOKEN_KEY, token);
    }

    return { ok: true, token };
  } catch (e: any) {
    // Never let a push failure surface to the user or break app start. Push is
    // an enhancement; the in-app feed carries the same information.
    return { ok: false, reason: 'error', detail: e?.message ?? String(e) };
  }
}

/**
 * Remove this device from the backend registry.
 *
 * Called on logout and when the Settings toggle goes off. The local cache is
 * cleared FIRST so that even if the network call fails, the next
 * registerForPushNotifications() will re-POST rather than assume the server
 * already knows — the cheap failure (a redundant POST) is preferable to the
 * expensive one (a phone that never re-registers).
 */
export async function unregisterPushNotifications() {
  let token: string | null = null;
  try {
    token = await AsyncStorage.getItem(PUSH_TOKEN_KEY);
    await AsyncStorage.removeItem(PUSH_TOKEN_KEY);
  } catch {
    /* ignore */
  }

  if (!token) return;

  try {
    // axios sends a DELETE body under `data`.
    await api.delete('/v1/notifications/push-token', { data: { token } });
  } catch {
    /* Best-effort. If it fails the server keeps a token that will either be
       re-pointed at the next user to sign in on this handset, or pruned when
       Expo reports DeviceNotRegistered. */
  }
}

/** Clears the iOS app-icon badge. Android badges are launcher-dependent. */
export async function clearBadge() {
  try {
    await Notifications.setBadgeCountAsync(0);
  } catch {
    /* ignore */
  }
}

/** Syncs the app-icon badge to the server's unread count. */
export async function syncBadgeFromServer() {
  try {
    const { data } = await api.get<{ unread_count: number }>('/v1/notifications/unread');
    await Notifications.setBadgeCountAsync(data.unread_count ?? 0);
  } catch {
    /* ignore */
  }
}

// ─── Tap routing ─────────────────────────────────────────────────────────────

export interface PushPayload {
  type?: string;
  event_id?: string;
  event_slug?: string;
  booking_id?: string;
  booking_number?: string;
  days_until?: number;
  test?: boolean;
  [key: string]: unknown;
}

/**
 * Map a notification payload to an in-app route.
 *
 * The payload keys come from the `data` map each backend producer passes to
 * CreateNotification, plus `type` which the push service always injects. Keep
 * this in sync with those producers — a notification whose data shape changes
 * server-side silently falls through to the tickets tab here rather than
 * erroring, which is safe but not useful.
 *
 * Returns null when there is nowhere sensible to go, in which case the tap just
 * opens the app.
 */
export function routeForNotification(data: PushPayload | undefined | null): string | null {
  if (!data) return null;

  switch (data.type) {
    case 'booking_confirmed':
    case 'refund_processed':
      // Prefer the specific booking; fall back to the tickets tab so the tap
      // still lands somewhere relevant if the id is missing.
      return data.booking_id ? `/booking/${data.booking_id}` : '/(tabs)/tickets';

    case 'event_reminder':
    case 'tickets_selling_fast':
    case 'event_sold_out':
      return data.event_id ? `/event/${data.event_id}` : null;

    case 'event_cancelled':
      // Deliberately the tickets tab, not the event page: a cancelled event's
      // page is a dead end, and what the user actually needs is their booking
      // and the refund state.
      return data.booking_id ? `/booking/${data.booking_id}` : '/(tabs)/tickets';

    case 'kyc_approved':
      // Organiser-facing; the consumer app has no dashboard. Land on profile.
      return '/(tabs)/profile';

    default:
      return data.event_id ? `/event/${data.event_id}` : null;
  }
}

function payloadOf(response: Notifications.NotificationResponse): PushPayload | undefined {
  return response?.notification?.request?.content?.data as PushPayload | undefined;
}

/**
 * Wire up notification listeners.
 *
 * @param navigate  Called with a route path when a notification is TAPPED.
 * @param onReceive Optional: called when one arrives while the app is open —
 *                  use it to refresh the notification feed / unread badge.
 * @returns cleanup function; call it on unmount.
 *
 * Handles three delivery states. The cold-start one is the easy one to miss:
 * when the app is fully killed, tapping a notification launches it, and by the
 * time listeners attach the response event has already been dispatched. Only
 * getLastNotificationResponseAsync() recovers it — without that call, tapping a
 * notification on a killed app just opens the home screen, which reads as a
 * broken deep link.
 */
export function attachNotificationListeners(
  navigate: (path: string) => void,
  onReceive?: (payload: PushPayload | undefined) => void
): () => void {
  let handledColdStart = false;

  const handle = (response: Notifications.NotificationResponse | null) => {
    if (!response) return;
    const route = routeForNotification(payloadOf(response));
    if (route) navigate(route);
  };

  // Cold start.
  Notifications.getLastNotificationResponseAsync()
    .then((response) => {
      if (handledColdStart) return;
      handledColdStart = true;
      handle(response);
    })
    .catch(() => {});

  const receivedSub = Notifications.addNotificationReceivedListener((notification) => {
    onReceive?.(notification.request.content.data as PushPayload | undefined);
  });

  const responseSub = Notifications.addNotificationResponseReceivedListener((response) => {
    handledColdStart = true; // don't double-handle with the cold-start path
    handle(response);
  });

  return () => {
    receivedSub.remove();
    responseSub.remove();
  };
}