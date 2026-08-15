import { create } from 'zustand';
import * as SecureStore from 'expo-secure-store';
import { AppState, type AppStateStatus } from 'react-native';
import {
  authLogin, authRegister, authLogout, getMe,
  saveTokens as persistTokens,
  clearTokens as wipeTokens,
  getAccessToken, getRefreshToken,
  ensureFreshSession, onSessionEnded,
  type AuthUser, type RegisterDTO,
} from '../lib/api';
import { unregisterPushNotifications } from '../lib/push';

interface AuthState {
  user: AuthUser | null;
  isLoading: boolean;
  // authReady flips true once the initial session-restore attempt (loadUser)
  // has finished — whether or not a user was found. Screens use it to wait for
  // a settled auth state before their first data fetch, so they don't load as a
  // guest and then immediately re-load as a signed-in user.
  authReady: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (dto: RegisterDTO) => Promise<void>;
  logout: () => Promise<void>;
  loadUser: () => Promise<void>;
  // Applies a freshly-updated profile (e.g. after PATCH /v1/user/me) to
  // both the store and the on-device cache, so the change survives a
  // restart without waiting for the next loadUser round-trip.
  applyUser: (user: AuthUser) => void;
}

// Both delegate to lib/api so there is exactly ONE token store. They used to
// be separate SecureStore calls here, which meant the api layer's in-memory
// copy went stale after a login and the very first authed request went out
// with the previous user's (or no) token.
const saveTokens  = persistTokens;
const clearTokens = wipeTokens;

// ── Cached user profile ───────────────────────────────────────────────────────
// We keep the last-known profile on-device so the session can be restored with
// NO network (e.g. arriving at a venue with no signal). It's only display data —
// every real API call still authenticates against the server when back online.
const USER_KEY = 'auth_user';

// Same accessibility as the tokens (see lib/api.ts): readable whenever the
// device has been unlocked once since boot, so a locked-screen read doesn't
// come back empty and look like a signed-out user.
const SECURE_OPTS: SecureStore.SecureStoreOptions = {
  keychainAccessible: SecureStore.AFTER_FIRST_UNLOCK,
};

async function saveCachedUser(user: AuthUser) {
  try {
    await SecureStore.setItemAsync(USER_KEY, JSON.stringify(user), SECURE_OPTS);
  } catch {
    /* best-effort — a failed cache write must not break login */
  }
}

async function getCachedUser(): Promise<AuthUser | null> {
  try {
    const raw = (await SecureStore.getItemAsync(USER_KEY, SECURE_OPTS))
      ?? (await SecureStore.getItemAsync(USER_KEY));
    return raw ? (JSON.parse(raw) as AuthUser) : null;
  } catch {
    return null;
  }
}

async function clearCachedUser() {
  try {
    await SecureStore.deleteItemAsync(USER_KEY, SECURE_OPTS);
    await SecureStore.deleteItemAsync(USER_KEY);
  } catch {
    /* ignore */
  }
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isLoading: false,
  authReady: false,

  login: async (email, password) => {
    set({ isLoading: true });
    try {
      // 1. Get tokens
      const { data: tokens } = await authLogin(email, password);
      await saveTokens(tokens.access_token, tokens.refresh_token);

      // 2. Fetch profile (interceptor will attach the token automatically)
      const { data: user } = await getMe();
      await saveCachedUser(user); // cache for offline session restore
      set({ user, isLoading: false, authReady: true });
    } catch (e) {
      set({ isLoading: false });
      throw e;
    }
  },

  register: async (dto) => {
    set({ isLoading: true });
    try {
      // 1. Create account
      await authRegister(dto);

      // 2. Log in to get tokens
      const { data: tokens } = await authLogin(dto.email, dto.password);
      await saveTokens(tokens.access_token, tokens.refresh_token);

      // 3. Fetch full profile
      const { data: user } = await getMe();
      await saveCachedUser(user); // cache for offline session restore
      set({ user, isLoading: false, authReady: true });
    } catch (e) {
      set({ isLoading: false });
      throw e;
    }
  },

  logout: async () => {
    // ORDER MATTERS: unregister the push token FIRST, while the access token
    // still exists. DELETE /v1/notifications/push-token is an authed endpoint —
    // called after clearTokens() it 401s, the interceptor finds no refresh
    // token, and the device stays registered on the server. The visible symptom
    // is the worst kind: the next person to use this handset keeps getting the
    // previous user's booking confirmations.
    try { await unregisterPushNotifications(); } catch { /* best-effort */ }

    try { await authLogout(); } catch { /* clear locally regardless */ }
    await clearTokens();
    await clearCachedUser();
    set({ user: null });
  },

  loadUser: async () => {
    try {
      // The REFRESH token is what decides whether there is a session, not the
      // access token. An access token only lives 15 minutes, so on any launch
      // more than a quarter of an hour after the last one it is expected to be
      // expired — treating that as "signed out" (as this used to, by reading
      // access_token) logged people out roughly every time they left the app
      // alone for a while.
      const refresh = await getRefreshToken();
      if (!refresh) {
        // No refresh token → genuinely signed out. Clear any stale profile.
        await clearCachedUser();
        set({ user: null });
        return;
      }

      // Renew up front so the first screen's fetches go out with a live token
      // rather than each discovering the expiry for itself. Never throws and
      // never signs anyone out on its own.
      await ensureFreshSession();
      const token = await getAccessToken();
      if (!token) {
        // Offline with an expired access token but a valid refresh token still
        // on the device: the session is intact, we just can't prove it right
        // now. Show the cached profile and carry on.
        const offlineUser = await getCachedUser();
        if (offlineUser) set({ user: offlineUser });
        return;
      }

      // Optimistically restore the cached profile FIRST, so the app is usable
      // offline (venue with no signal) before we even try to reach the server.
      const cached = await getCachedUser();
      if (cached) set({ user: cached });

      // Then validate against the server. The key rule: only an explicit auth
      // rejection (server reachable, says the token is bad) logs the user out.
      // A network failure (offline) or server hiccup must NOT drop the session —
      // otherwise a dead-signal cold-start would wipe tokens and lock the user
      // out of their own cached tickets.
      try {
        const { data: user } = await getMe();
        await saveCachedUser(user);
        set({ user });
      } catch (err: any) {
        const status = err?.response?.status;
        // Only an explicit rejection that SURVIVED the refresh attempt counts.
        // By the time an error reaches here the interceptor has already tried
        // to refresh; if that refresh was rejected outright it has cleared the
        // tokens itself, so re-check rather than assuming.
        const stillHaveSession = !!(await getRefreshToken());
        if ((status === 401 || status === 403) && !stillHaveSession) {
          // Reached the server and it rejected us (token truly invalid /
          // refresh also failed) → real sign-out.
          await clearTokens();
          await clearCachedUser();
          set({ user: null });
        }
        // Otherwise: offline / network error / 5xx → keep the cached session.
        // (If there was no cached user, we simply remain signed-out this launch.)
      }
    } catch {
      // Unexpected local failure (e.g. SecureStore read) — leave state as-is
      // rather than forcing a logout.
    } finally {
      // Mark auth as settled in every path (no token, success, or failure) so
      // screens waiting on authReady always proceed.
      set({ authReady: true });
    }
  },
  applyUser: (user) => {
    set({ user });
    saveCachedUser(user).catch(() => {});
  },
}));

// ─── Session-ended bridge ─────────────────────────────────────────────────────
// lib/api fires this from the ONE place a session can legitimately end without
// the user asking: the server definitively rejecting the refresh token
// (expired, revoked, or reuse detected). Everything else — offline, timeout,
// 502 from the tunnel, a failed Keychain write — leaves the session alone.
onSessionEnded(() => {
  clearCachedUser().catch(() => {});
  useAuthStore.setState({ user: null, authReady: true });
});

// ─── Keep the session warm across foregrounds ────────────────────────────────
// A phone can sit in a pocket for days. Coming back to the app, the access
// token is always expired; renewing it here means the first tap works instead
// of firing a 401 into whatever screen is on top.
let lastForegroundCheck = 0;
const FOREGROUND_MIN_GAP_MS = 30000;

AppState.addEventListener('change', (state: AppStateStatus) => {
  if (state !== 'active') return;
  if (!useAuthStore.getState().user) return;
  const now = Date.now();
  if (now - lastForegroundCheck < FOREGROUND_MIN_GAP_MS) return;
  lastForegroundCheck = now;
  void ensureFreshSession();
});