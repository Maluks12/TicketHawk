import { create } from 'zustand';
import * as SecureStore from 'expo-secure-store';
import { authLogin, authRegister, authLogout, getMe, type AuthUser, type RegisterDTO } from '../lib/api';

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

async function saveTokens(access: string, refresh: string) {
  await SecureStore.setItemAsync('access_token', access);
  await SecureStore.setItemAsync('refresh_token', refresh);
}

async function clearTokens() {
  await SecureStore.deleteItemAsync('access_token');
  await SecureStore.deleteItemAsync('refresh_token');
}

// ── Cached user profile ───────────────────────────────────────────────────────
// We keep the last-known profile on-device so the session can be restored with
// NO network (e.g. arriving at a venue with no signal). It's only display data —
// every real API call still authenticates against the server when back online.
const USER_KEY = 'auth_user';

async function saveCachedUser(user: AuthUser) {
  try {
    await SecureStore.setItemAsync(USER_KEY, JSON.stringify(user));
  } catch {
    /* best-effort — a failed cache write must not break login */
  }
}

async function getCachedUser(): Promise<AuthUser | null> {
  try {
    const raw = await SecureStore.getItemAsync(USER_KEY);
    return raw ? (JSON.parse(raw) as AuthUser) : null;
  } catch {
    return null;
  }
}

async function clearCachedUser() {
  try {
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
    try { await authLogout(); } catch { /* clear locally regardless */ }
    await clearTokens();
    await clearCachedUser();
    set({ user: null });
  },

  loadUser: async () => {
    try {
      const token = await SecureStore.getItemAsync('access_token');
      if (!token) {
        // No token → genuinely signed out. Make sure no stale profile lingers.
        await clearCachedUser();
        set({ user: null });
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
        if (status === 401 || status === 403) {
          // Reached the server and it rejected us (token truly invalid / refresh
          // also failed) → real sign-out.
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