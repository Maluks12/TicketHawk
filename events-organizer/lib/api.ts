import axios from 'axios';

// ── baseURL must be set, and we say so loudly if it is not ──────────────────
//
// NEXT_PUBLIC_API_URL is only ever supplied as a Docker BUILD ARG (Dockerfile
// line 23), so it is present in every deployed environment and absent in every
// local one unless a .env.local exists. When it is missing, axios silently
// falls back to resolving paths against the page origin — /organiser/kyc
// becomes http://localhost:3000/organiser/kyc — and the failure surfaces
// several layers away as a dashboard that never stops loading.
//
// A console error at startup turns twenty minutes of spinner into one line.
if (typeof window !== 'undefined' && !process.env.NEXT_PUBLIC_API_URL) {
  // eslint-disable-next-line no-console
  console.error(
    '[api] NEXT_PUBLIC_API_URL is not set. Every API call will hit the Next ' +
    'dev server instead of the backend. Create .env.local with:\n' +
    '  NEXT_PUBLIC_API_URL=http://127.0.0.1:8080/v1',
  );
}

export const api = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL,
  withCredentials: true,

  // ── A timeout, because axios has none by default ───────────────────────
  //
  // axios defaults to timeout: 0, meaning wait forever. Any request that
  // stalls — wrong host, API down, a connection dropped rather than refused —
  // leaves its promise unsettled for the life of the page.
  //
  // That is not merely slow, it is a permanent UI state: the dashboard layout
  // gates rendering on `kycChecked`, which is only set in getKycStatus()'s
  // .then or .catch. Neither runs if the promise never settles, so the
  // spinner is forever and no error is ever logged.
  //
  // 20s is long enough for a cold Go container plus a slow mobile connection,
  // and short enough that a person notices something is wrong rather than
  // assuming the app is thinking.
  timeout: 20_000,
});

api.interceptors.request.use((config) => {
  if (typeof window !== 'undefined') {
    const token = localStorage.getItem('evt_token');
    if (token) config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// ── Single-flight token refresh ────────────────────────────────────────────
// Parity with the main web apps. The backend issues a 15-minute access token
// plus a rotating refresh token; the old client stored ONLY the access token,
// which forced a hard re-login every 15 minutes. It also refreshed per failed
// request, and because the backend rotates refresh tokens and treats a
// replayed one as theft, parallel 401s could revoke every session the user
// had. One in-flight refresh promise, shared by all concurrent 401s.
let refreshInFlight: Promise<string> | null = null;

// Exported: after basic KYC promotes attendee → organizer, the current access
// token still carries the OLD role; callers force a refresh to pick it up.
export function refreshAccessToken(): Promise<string> {
  if (!refreshInFlight) {
    refreshInFlight = (async () => {
      const refresh = typeof window !== 'undefined' ? localStorage.getItem('evt_refresh') : null;
      if (!refresh) throw new Error('no refresh token');
      const { data } = await axios.post(
        `${process.env.NEXT_PUBLIC_API_URL}/auth/refresh`,
        { refresh_token: refresh },
      );
      localStorage.setItem('evt_token', data.access_token);
      localStorage.setItem('evt_refresh', data.refresh_token);
      return data.access_token as string;
    })().finally(() => {
      refreshInFlight = null;
    });
  }
  return refreshInFlight;
}

api.interceptors.response.use(
  (res) => res,
  async (error) => {
    const original = error.config;
    const url: string = original?.url ?? '';
    const isAuthCall = url.includes('/auth/login')
      || url.includes('/auth/register')
      || url.includes('/auth/refresh');

    if (error.response?.status === 401 && original
        && !isAuthCall && typeof window !== 'undefined') {
      const token = localStorage.getItem('evt_token');

      // ── Race retry ─────────────────────────────────────────────────────
      // The request went out WITHOUT an Authorization header even though we
      // now hold a token. This is the long-standing post-login propagation
      // race: a gate fires its first authenticated call in the same tick the
      // token is written, the request interceptor read localStorage a moment
      // too early, and the backend rejected the bare request with 401
      // "authorization header required". The access token is perfectly valid
      // — it just wasn't on the request. Re-attach it and retry ONCE.
      //
      // This is the fix for the "login just reloads the page" loop: the old
      // code funnelled this race into the refresh-or-reload path below, and a
      // failed refresh ran window.location + clearSession, bouncing the user
      // back to a wiped login screen. A plain retry spends no refresh token,
      // rotates nothing, and never reloads.
      if (token && !original.headers?.Authorization && !original._authRetry) {
        original._authRetry = true;
        original.headers = original.headers ?? {};
        original.headers.Authorization = `Bearer ${token}`;
        return api(original);
      }

      // ── Refresh retry ──────────────────────────────────────────────────
      // A token WAS sent and still came back 401 → genuinely expired. Swap it
      // via the single-flight refresh and replay the request.
      if (!original._retry) {
        original._retry = true;

        // No refresh token = no session to restore. Reject quietly and let
        // the caller decide; do NOT hard-redirect/reload the page.
        if (!localStorage.getItem('evt_refresh')) {
          return Promise.reject(error);
        }

        try {
          const fresh = await refreshAccessToken();
          original.headers = original.headers ?? {};
          original.headers.Authorization = `Bearer ${fresh}`;
          return api(original);
        } catch {
          // A real refresh failure = genuinely expired/revoked session.
          clearSession();
          window.location.href = '/login';
        }
      }
    }
    return Promise.reject(error);
  }
);

export function clearSession() {
  localStorage.removeItem('evt_token');
  localStorage.removeItem('evt_refresh');
  localStorage.removeItem('org_user');
  localStorage.removeItem('admin_user');
}

// Tells the server to revoke ONLY this session's refresh token, so logging
// out of the dashboard doesn't kill the user's app/web sessions.
export async function serverLogout() {
  try {
    const refresh = localStorage.getItem('evt_refresh') ?? '';
    await api.post('/auth/logout', { refresh_token: refresh });
  } catch {
    // Best effort — local session is cleared regardless.
  }
}