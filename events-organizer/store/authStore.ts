import { create } from 'zustand';
import { api } from '../lib/api';

interface OrgUser { id: string; email: string; role: string; }
interface OrgAuthState {
  user: OrgUser | null;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => void;
  hydrate: () => void;
}

export const useOrgAuth = create<OrgAuthState>((set) => ({
  user: null,
  isLoading: true,

  hydrate: () => {
    if (typeof window === 'undefined') return;
    const token = localStorage.getItem('evt_token');
    const raw = localStorage.getItem('org_user');
    if (token && raw) {
      try {
        const user = JSON.parse(raw) as OrgUser;
        if (user.role !== 'organizer') { localStorage.removeItem('org_user'); set({ user: null, isLoading: false }); return; }
        set({ user, isLoading: false });
      } catch { set({ user: null, isLoading: false }); }
    } else { set({ user: null, isLoading: false }); }
  },

  login: async (email: string, password: string) => {
    const { data } = await api.post('/auth/login', { email, password });
    const token = data.token || data.access_token;
    if (!token) throw new Error('No token returned');
    const payload = JSON.parse(atob(token.split('.')[1]));
    if (payload.role !== 'organizer') throw new Error('This account does not have organizer access.');
    const user = { id: payload.user_id, email: payload.email, role: payload.role };
    localStorage.setItem('evt_token', token);
    localStorage.setItem('org_user', JSON.stringify(user));
    set({ user });
  },

  logout: () => {
    localStorage.removeItem('evt_token');
    localStorage.removeItem('org_user');
    set({ user: null });
  },
}));