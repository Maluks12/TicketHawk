import { create } from 'zustand';
import { authLogin, authRegister, authLogout, getMe, saveTokens, clearTokens, type AuthUser, type RegisterDTO } from '../lib/api';

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

  loadUser: async () => {
    if (typeof window === 'undefined') return;
    const token = localStorage.getItem('access_token');
    if (!token) {
      set({ authReady: true });
      return;
    }
    try {
      const { data: user } = await getMe();
      set({ user });
    } catch {
      clearTokens();
      set({ user: null });
    } finally {
      // Mark auth as settled in every path (no token, success, or failure) so
      // pages waiting on authReady always proceed.
      set({ authReady: true });
    }
  },
}));