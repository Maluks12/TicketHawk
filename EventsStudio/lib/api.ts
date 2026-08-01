// ─────────────────────────────────────────────────────────────────────────────
// TicketHawk Studio — API client
//
// A trimmed copy of the consumer app's lib/api.ts. The Studio app only needs:
//   • the shared axios instance (`api`) with bearer auth + single-flight refresh
//   • the auth endpoints (login / logout / me) used by store/authStore.ts
// Everything else (events, cart, checkout, bookings, merch) lives only in the
// consumer app. lib/organiser.ts, lib/staff.ts and lib/scanner/* all import
// `api` from here.
//
// Unlike the consumer app, this file is committed — it contains no secrets.
// The base URL comes from EXPO_PUBLIC_API_URL (set in eas.json per profile),
// falling back to the Metro host in dev.
// ─────────────────────────────────────────────────────────────────────────────
import axios from 'axios';
import * as SecureStore from 'expo-secure-store';
import Constants from 'expo-constants';

function getApiBaseUrl(): string {
  if (process.env.EXPO_PUBLIC_API_URL) {
    return process.env.EXPO_PUBLIC_API_URL;
  }
  const debuggerHost =
    Constants.expoConfig?.hostUri ??
    (Constants as any).manifest?.debuggerHost ??
    (Constants as any).manifest2?.extra?.expoGo?.debuggerHost;

  if (debuggerHost) {
    const ip = debuggerHost.split(':')[0];
    return `http://${ip}:8080`;
  }
  return 'http://localhost:8080';
}

export const API_BASE_URL = getApiBaseUrl();

export const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 20000,
  headers: { 'Content-Type': 'application/json' },
});

api.interceptors.request.use(async (config) => {
  const token = await SecureStore.getItemAsync('access_token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

// ── Single-flight token refresh ───────────────────────────────────────────────
// The backend ROTATES refresh tokens: every /v1/auth/refresh call revokes the
// presented token and issues a new one. All concurrent 401s share ONE in-flight
// refresh promise, so the app makes exactly one refresh call per expiry.
let refreshInFlight: Promise<string> | null = null;

function refreshAccessToken(): Promise<string> {
  if (!refreshInFlight) {
    refreshInFlight = (async () => {
      const refresh = await SecureStore.getItemAsync('refresh_token');
      if (!refresh) throw new Error('no refresh token');
      const { data } = await axios.post(`${API_BASE_URL}/v1/auth/refresh`, {
        refresh_token: refresh,
      });
      await SecureStore.setItemAsync('access_token', data.access_token);
      await SecureStore.setItemAsync('refresh_token', data.refresh_token);
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
    if (error.response?.status === 401 && !original._retry) {
      original._retry = true;
      try {
        const newToken = await refreshAccessToken();
        original.headers.Authorization = `Bearer ${newToken}`;
        return api(original);
      } catch {
        await SecureStore.deleteItemAsync('access_token');
        await SecureStore.deleteItemAsync('refresh_token');
        throw error;
      }
    }
    throw error;
  }
);

// ── Auth types ────────────────────────────────────────────────────────────────

export interface AuthUser {
  id: string;
  email: string;
  role: string;
  first_name?: string;
  last_name?: string;
  phone_number?: string;
}

export interface TokenResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

export interface RegisterDTO {
  email: string;
  password: string;
  first_name: string;
  last_name: string;
  phone_number: string;
  date_of_birth: string;
}

// ── Auth API calls ────────────────────────────────────────────────────────────

export const authLogin = (email: string, password: string) =>
  api.post<TokenResponse>('/v1/auth/login', { email, password });

// The Studio app has no sign-up flow — organisers and staff create accounts in
// the consumer app or on the web. authRegister exists only so store/authStore.ts
// compiles unchanged; calling it works, but no screen links to it.
export const authRegister = (dto: RegisterDTO) =>
  api.post('/v1/auth/register', dto);

// Send our refresh token so the server revokes ONLY this device's session —
// logging out of Studio doesn't log you out of the consumer app or web.
export const authLogout = async () =>
  api.post('/v1/auth/logout', {
    refresh_token: (await SecureStore.getItemAsync('refresh_token')) ?? '',
  });

export const getMe = () => api.get<AuthUser>('/v1/user/me');

// Applies the signed-in account for an organiser profile. Creates a PENDING
// profile only — passing KYC is what actually grants the organiser role
// (see RegisterAsOrganizerHandler / PromoteOrganizerAfterKYC on the backend),
// so a fresh signup lands on the staff screen until verification completes.
// Body is optional; the backend derives the business name from the account.
export const applyAsOrganiser = (body?: { business_name?: string; business_phone?: string }) =>
  api.post('/v1/register-organizer', body ?? {});