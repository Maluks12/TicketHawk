import { create } from 'zustand';
import * as SecureStore from 'expo-secure-store';
import { authLogin, authRegister, authLogout, getMe, type AuthUser, type RegisterDTO } from '../lib/api';

interface AuthState {
  user: AuthUser | null;
  isLoading: boolean;
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

  login: async (email, password) => {
    set({ isLoading: true });
    try {
      // 1. Get tokens
      const { data: tokens } = await authLogin(email, password);
      await saveTokens(tokens.access_token, tokens.refresh_token);

      // 2. Fetch profile (interceptor will attach the token automatically)
      const { data: user } = await getMe();
      set({ user, isLoading: false });
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
      set({ user, isLoading: false });
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
    const token = await SecureStore.getItemAsync('access_token');
    if (!token) return;
    try {
      const { data: user } = await getMe();
      set({ user });
    } catch {
      await clearTokens();
      set({ user: null });
    }
  },
}));