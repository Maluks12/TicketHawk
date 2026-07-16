import { create } from 'zustand';
import { api, clearSession, serverLogout } from '../lib/api';

interface AdminUser { id: string; email: string; role: string; }
interface AdminAuthState {
  user: AdminUser | null;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  hydrate: () => void;
}

export const useAdminAuth = create<AdminAuthState>((set) => ({
  user: null,
  isLoading: true,

  hydrate: () => {
    if (typeof window === 'undefined') return;
    const token = localStorage.getItem('evt_token');
    const raw = localStorage.getItem('admin_user');
    if (token && raw) {
      try {
        const user = JSON.parse(raw) as AdminUser;
        if (user.role !== 'admin') { clearSession(); set({ user: null, isLoading: false }); return; }
        set({ user, isLoading: false });
      } catch { set({ user: null, isLoading: false }); }
    } else { set({ user: null, isLoading: false }); }
  },

  login: async (email: string, password: string) => {
    const { data } = await api.post('/auth/login', { email, password });
    const token = data.token || data.access_token;
    if (!token) throw new Error('No token returned');
    const payload = JSON.parse(atob(token.split('.')[1]));
    if (payload.role !== 'admin') throw new Error('This account does not have admin access.');

    // Persist the refresh token too — without it the session died with the
    // 15-minute access token and forced a re-login every quarter hour.
    if (data.refresh_token) localStorage.setItem('evt_refresh', data.refresh_token);

    // The JWT does not carry an email claim (the old code read
    // payload.email and stored `undefined`) — use the address they typed.
    const user = { id: payload.user_id, email, role: payload.role };
    localStorage.setItem('evt_token', token);
    localStorage.setItem('admin_user', JSON.stringify(user));
    set({ user });
  },

  logout: async () => {
    await serverLogout(); // revoke this session server-side (scoped)
    clearSession();
    set({ user: null });
  },
}));