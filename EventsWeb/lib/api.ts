import axios from 'axios';

export const api = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL,
  withCredentials: true,
});

// ── Token helpers ────────────────────────────────────────────────────────────
export function getAccessToken() {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem('access_token');
}
export function saveTokens(access: string, refresh: string) {
  localStorage.setItem('access_token', access);
  localStorage.setItem('refresh_token', refresh);
}
export function clearTokens() {
  localStorage.removeItem('access_token');
  localStorage.removeItem('refresh_token');
}

// ── Request interceptor — attach Bearer token ────────────────────────────────
api.interceptors.request.use((config) => {
  const token = getAccessToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

// ── Response interceptor — handle 401 ───────────────────────────────────────
api.interceptors.response.use(
  (res) => res,
  (error) => {
    if (error.response?.status === 401 && typeof window !== 'undefined') {
      clearTokens();
      window.location.href = '/auth/login';
    }
    return Promise.reject(error);
  }
);

// ── Types (keep in sync with EventsApp/lib/api) ──────────────────────────────
export interface AuthUser {
  id: string;
  email: string;
  name: string;
  avatar_url?: string;
}

export interface RegisterDTO {
  email: string;
  password: string;
  name: string;
}

export interface Event {
  id: string;
  title: string;
  slug: string;
  description?: string;
  image_url?: string;
  date: string;
  venue?: string;
  city?: string;
  category?: string;
  price?: number;
  is_free?: boolean;
}

export interface Category {
  id: string;
  name: string;
  slug: string;
  image_url?: string;
}

// ── Auth endpoints ────────────────────────────────────────────────────────────
export const authLogin = (email: string, password: string) =>
  api.post<{ access_token: string; refresh_token: string }>('/auth/login', { email, password });

export const authRegister = (dto: RegisterDTO) =>
  api.post('/auth/register', dto);

export const authLogout = () =>
  api.post('/auth/logout');

export const getMe = () =>
  api.get<AuthUser>('/users/me');