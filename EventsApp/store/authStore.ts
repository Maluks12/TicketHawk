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
}

async function saveTokens(access: string, refresh: string) {
  await SecureStore.setItemAsync('access_token', access);
  await SecureStore.setItemAsync('refresh_token', refresh);
}

async function clearTokens() {
  await SecureStore.deleteItemAsync('access_token');
  await SecureStore.deleteItemAsync('refresh_token');
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
      set({ user, isLoading: false, authReady: true });
    } catch (e) {
      set({ isLoading: false });
      throw e;
    }
  },

  logout: async () => {
    try { await authLogout(); } catch { /* clear locally regardless */ }
    await clearTokens();
    set({ user: null });
  },

  loadUser: async () => {
    try {
      const token = await SecureStore.getItemAsync('access_token');
      if (!token) return;
      const { data: user } = await getMe();
      set({ user });
    } catch {
      await clearTokens();
      set({ user: null });
    } finally {
      // Mark auth as settled in every path (no token, success, or failure) so
      // screens waiting on authReady always proceed.
      set({ authReady: true });
    }
  },
}));