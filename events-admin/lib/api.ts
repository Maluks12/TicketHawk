import axios from 'axios';

export const api = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL,
  withCredentials: true,
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

function refreshAccessToken(): Promise<string> {
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
    if (error.response?.status === 401 && original && !original._retry && typeof window !== 'undefined') {
      original._retry = true;
      try {
        const token = await refreshAccessToken();
        original.headers.Authorization = `Bearer ${token}`;
        return api(original);
      } catch {
        clearSession();
        window.location.href = '/login';
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