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

// ── Role source of truth ──────────────────────────────────────────────────────
// The ACCESS TOKEN is authoritative for the role, never the cached org_user
// blob. This matters right after basic KYC: the promotion happens server-side,
// refreshAccessToken() mints a token carrying the new `organizer` role, but the
// org_user written at login still says `attendee`. Reading the token means the
// promotion is picked up on the very next hydrate, with no re-login.
//
// Backend role VALUES (models.UserRole): "attendee" | "organizer" | "admin".
// US spelling on the wire — only UI copy uses the British "organiser".
function decodeToken(token: string): { role: string; userId: string } | null {
  try {
    const seg = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
    const pad = seg.length % 4 ? '='.repeat(4 - (seg.length % 4)) : '';
    const payload = JSON.parse(atob(seg + pad));
    return {
      role:   typeof payload.role === 'string' && payload.role ? payload.role : 'attendee',
      userId: typeof payload.user_id === 'string' ? payload.user_id : '',
    };
  } catch {
    return null;
  }
}

export const useOrgAuth = create<OrgAuthState>((set) => ({
  user: null,
  isLoading: true,

  // hydrate RESTORES a session. It does not authorize one.
  //
  // It used to call clearSession() whenever the stored role wasn't organizer or
  // admin. That destroyed the login: a plain `attendee` — exactly the account
  // /onboarding exists to promote — logged in fine, got routed to /onboarding,
  // and the first thing that page did on mount was hydrate(), which wiped the
  // freshly-issued tokens and nulled the user. /onboarding's own guard then saw
  // no user and bounced to /login. The whole loop ran client-side, so the API
  // log showed a 200 login followed by dead silence, and the screen appeared to
  // "just reload" back to the login form.
  //
  // Authorization is the route guards' job: /onboarding admits anyone signed
  // in, the dashboard layout gates on KYC level. hydrate only answers "is
  // someone signed in, and as whom?".
  hydrate: () => {
    if (typeof window === 'undefined') return;

    const token = localStorage.getItem('evt_token');
    if (!token) { set({ user: null, isLoading: false }); return; }

    let stored: Partial<OrgUser> | null = null;
    try {
      const raw = localStorage.getItem('org_user');
      stored = raw ? (JSON.parse(raw) as Partial<OrgUser>) : null;
    } catch { stored = null; }

    // Token wins; fall back to the cached blob only if the token won't decode.
    const claims = decodeToken(token);
    const user: OrgUser = {
      id:    claims?.userId || stored?.id    || '',
      email: stored?.email  || '',
      role:  claims?.role   || stored?.role  || 'attendee',
    };

    // Keep the cache in step so a later decode failure degrades to the right
    // role instead of an outdated one.
    localStorage.setItem('org_user', JSON.stringify(user));
    set({ user, isLoading: false });
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
    // Always OVERWRITE the refresh token — and if this login didn't return one,
    // drop any leftover from a previous session. Replaying a stale/rotated
    // refresh token makes the backend treat it as token reuse (theft) and
    // revoke the whole session family, which 401s the fresh session and bounces
    // the user back to a wiped login screen.
    if (data.refresh_token) localStorage.setItem('evt_refresh', data.refresh_token);
    else localStorage.removeItem('evt_refresh');

    // A bad decode is not fatal: we still hold a working session and simply
    // fall back to the lowest role, so the gate routes to onboarding — the safe
    // default, and recoverable, rather than a dead end.
    const claims = decodeToken(token);
    const user: OrgUser = {
      id: claims?.userId ?? '',
      email,
      role: claims?.role ?? 'attendee',
    };

    localStorage.setItem('org_user', JSON.stringify(user));
    set({ user, isLoading: false });
    return { role: user.role };
  },

  logout: async () => {
    await serverLogout(); // revoke this session server-side (scoped)
    clearSession();
    set({ user: null, isLoading: false });
  },
}));