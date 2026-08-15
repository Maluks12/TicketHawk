import { create } from 'zustand';
import { authLogin, authRegister, authLogout, getMe, saveTokens, clearTokens, ensureFreshSession, type AuthUser, type RegisterDTO } from '../lib/api';

interface AuthState {
  user: AuthUser | null;
  isLoading: boolean;
  // authReady flips true once the initial session-restore attempt (loadUser)
  // has finished — whether or not a user was found. Pages use it to wait for
  // a settled auth state before their first data fetch, so they don't load as
  // a guest and then immediately re-load as a signed-in user.
  authReady: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (dto: RegisterDTO) => Promise<void>;
  logout: () => Promise<void>;
  loadUser: () => Promise<void>;
  // Merges a fresh user object (e.g. from PATCH /v1/user/me) into the store
  // without a network round-trip. Used by the edit-profile screen so a save
  // reflects immediately instead of waiting on a re-fetch.
  applyUser: (user: AuthUser) => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isLoading: false,
  authReady: false,

  login: async (email, password) => {
    set({ isLoading: true });
    try {
      const { data: tokens } = await authLogin(email, password);
      saveTokens(tokens.access_token, tokens.refresh_token);
      const { data: user } = await getMe();
      set({ user, isLoading: false, authReady: true });
    } catch (e) {
      set({ isLoading: false });
      throw e;
    }
  },

  register: async (dto) => {
    set({ isLoading: true });
    try {
      await authRegister(dto);
      const { data: tokens } = await authLogin(dto.email, dto.password);
      saveTokens(tokens.access_token, tokens.refresh_token);
      const { data: user } = await getMe();
      set({ user, isLoading: false, authReady: true });
    } catch (e) {
      set({ isLoading: false });
      throw e;
    }
  },

  logout: async () => {
    try { await authLogout(); } catch { /* clear locally regardless */ }
    clearTokens();
    set({ user: null });
  },

  applyUser: (user) => set({ user }),

  loadUser: async () => {
    if (typeof window === 'undefined') return;
    // The REFRESH token decides whether a session exists, not the access
    // token. Access tokens live 15 minutes, so on any visit more than a
    // quarter of an hour after the last one it is EXPECTED to be expired —
    // reading that as "signed out" (which checking access_token here did)
    // logged people out for simply leaving the tab alone.
    const refresh = localStorage.getItem('refresh_token');
    if (!refresh) {
      set({ authReady: true });
      return;
    }
    try {
      // Renew up front so the first page's fetches go out with a live token
      // rather than each discovering the expiry for itself.
      await ensureFreshSession();
      const { data: user } = await getMe();
      set({ user });
    } catch (err: unknown) {
      // Only a rejection that SURVIVED the refresh attempt counts. By the time
      // an error reaches here the interceptor has already tried to refresh; if
      // that refresh was definitively rejected it cleared the tokens itself,
      // so re-check rather than assuming. Offline, a timeout or a 502 from the
      // tunnel must leave the session intact.
      const stillHaveSession = !!localStorage.getItem('refresh_token');
      if (!stillHaveSession) {
        clearTokens();
        set({ user: null });
      }
    } finally {
      // Mark auth as settled in every path (no token, success, or failure) so
      // pages waiting on authReady always proceed.
      set({ authReady: true });
    }
  },
}));