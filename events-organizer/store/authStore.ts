import { create } from 'zustand';
import { api, clearSession, serverLogout } from '../lib/api';

interface OrgUser { id: string; email: string; role: string; }
interface OrgAuthState {
  user: OrgUser | null;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<{ role: string }>;
  logout: () => Promise<void>;
  hydrate: () => void;
}

export const useOrgAuth = create<OrgAuthState>((set) => ({
  user: null,
  isLoading: true,

  hydrate: () => {
    if (typeof window === 'undefined') return;
    const token = localStorage.getItem('evt_token');
    const raw   = localStorage.getItem('org_user');
    if (token && raw) {
      try {
        const user = JSON.parse(raw) as OrgUser;
        // Allow organiser AND admin through — reject plain attendees who
        // somehow ended up with a stored session without completing upgrade.
        // NOTE: 'organizer' (US spelling) is the backend role VALUE — do not
        // change it; only UI copy uses the British spelling.
        if (user.role !== 'organizer' && user.role !== 'admin') {
          clearSession();
          set({ user: null, isLoading: false });
          return;
        }
        set({ user, isLoading: false });
      } catch { set({ user: null, isLoading: false }); }
    } else { set({ user: null, isLoading: false }); }
  },

  // login accepts ANY valid user and returns the role so the login page can
  // decide whether to show the organiser-upgrade step.
  login: async (email: string, password: string) => {
    const { data } = await api.post('/auth/login', { email, password });
    const token = data.token || data.access_token;
    if (!token) throw new Error('No token returned');

    // Persist the refresh token too — without it the session died with the
    // 15-minute access token and forced a re-login every quarter hour.
    if (data.refresh_token) localStorage.setItem('evt_refresh', data.refresh_token);

    const payload = JSON.parse(atob(token.split('.')[1]));
    // The JWT does not carry an email claim — use the address they typed.
    const user = { id: payload.user_id, email, role: payload.role };
    localStorage.setItem('evt_token', token);
    localStorage.setItem('org_user', JSON.stringify(user));
    set({ user });
    return { role: payload.role };
  },

  logout: async () => {
    await serverLogout(); // revoke this session server-side (scoped)
    clearSession();
    set({ user: null });
  },
}));