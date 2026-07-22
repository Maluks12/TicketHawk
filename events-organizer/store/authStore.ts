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

    // Persist tokens FIRST — before any decode that could throw. Previously the
    // JWT was decoded before storing the access token, so a malformed payload
    // (or any exception) left localStorage empty and every later request went
    // out unauthenticated. Storage must never be gated behind the decode.
    localStorage.setItem('evt_token', token);
    if (data.refresh_token) localStorage.setItem('evt_refresh', data.refresh_token);

    // Decode defensively. A base64url JWT segment needs padding fixups before
    // atob; if anything is off we still have a working session and just fall
    // back to a minimal user (role unknown → treated as non-organiser → the
    // gate routes to onboarding, which is the safe default).
    let role = 'user';
    let userId = '';
    try {
      const seg = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
      const pad = seg.length % 4 ? '='.repeat(4 - (seg.length % 4)) : '';
      const payload = JSON.parse(atob(seg + pad));
      role = payload.role ?? 'user';
      userId = payload.user_id ?? '';
    } catch {
      /* keep the safe defaults */
    }

    const user = { id: userId, email, role };
    localStorage.setItem('org_user', JSON.stringify(user));
    set({ user });
    return { role };
  },

  logout: async () => {
    await serverLogout(); // revoke this session server-side (scoped)
    clearSession();
    set({ user: null });
  },
}));