import { create } from 'zustand';
import { authLogin, authRegister, authLogout, getMe, saveTokens, clearTokens, type AuthUser, type RegisterDTO } from '../lib/api';

interface AuthState {
  user: AuthUser | null;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (dto: RegisterDTO) => Promise<void>;
  logout: () => Promise<void>;
  loadUser: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isLoading: false,

  login: async (email, password) => {
    set({ isLoading: true });
    try {
      const { data: tokens } = await authLogin(email, password);
      saveTokens(tokens.access_token, tokens.refresh_token);
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
      await authRegister(dto);
      const { data: tokens } = await authLogin(dto.email, dto.password);
      saveTokens(tokens.access_token, tokens.refresh_token);
      const { data: user } = await getMe();
      set({ user, isLoading: false });
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
    if (!token) return;
    try {
      const { data: user } = await getMe();
      set({ user });
    } catch {
      clearTokens();
      set({ user: null });
    }
  },
}));