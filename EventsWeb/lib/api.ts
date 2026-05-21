import axios from 'axios';

export const api = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL,
  withCredentials: true,
});

// ── Token helpers ─────────────────────────────────────────────────────────────
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

// ── Session ID ────────────────────────────────────────────────────────────────
let _cachedSessionId: string | undefined;
function generateUUID() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}
export async function getCachedSessionId(): Promise<string> {
  if (_cachedSessionId) return _cachedSessionId;
  let sid = localStorage.getItem('session_id');
  if (!sid) { sid = generateUUID(); localStorage.setItem('session_id', sid); }
  _cachedSessionId = sid;
  return sid;
}

// ── Request interceptor ───────────────────────────────────────────────────────
api.interceptors.request.use((config) => {
  const token = getAccessToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

// ── Response interceptor ──────────────────────────────────────────────────────
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

// ── Types ─────────────────────────────────────────────────────────────────────
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
  slug: string;
  title: string;
  description: string;
  category: string;
  venue_name: string;
  city: string;
  address?: string;
  cover_image_url?: string;
  banner_image_url?: string;
  is_featured: boolean;
  is_sponsored?: boolean;
  status: string;
  start_time: string;
  end_time: string;
  price: number;
  currency: string;
  views: number;
  is_sold_out?: boolean;
  available_tickets?: number;
  tickets_sold?: number;
  teams?: string[];
  performers?: string[];
  max_capacity?: number;
}

export interface HomeSection {
  id: string;
  title: string;
  events: Event[];
  next_cursor: string;
}

export interface HomeResponse {
  sections: HomeSection[];
}

export interface Category {
  id: number;
  name: string;
  slug?: string;
}

export interface CartSummary {
  total_items: number;
  total_price: number;
  items: any[];
}

// ── Auth endpoints ─────────────────────────────────────────────────────────────
export const authLogin = (email: string, password: string) =>
  api.post<{ access_token: string; refresh_token: string }>('/auth/login', { email, password });

export const authRegister = (dto: RegisterDTO) =>
  api.post('/auth/register', dto);

export const authLogout = () =>
  api.post('/auth/logout');

export const getMe = () =>
  api.get<AuthUser>('/users/me');

// ── Event endpoints ────────────────────────────────────────────────────────────
export const getHomeSections = (city?: string) =>
  api.get<HomeResponse>('/v1/home', city ? { params: { city } } : undefined);

export const getCategories = () =>
  api.get<{ categories: Category[] }>('/v1/categories');

const SECTION_ENDPOINT: Record<string, string> = {
  trending:       '/v1/events/trending',
  featured:       '/v1/events/featured',
  happening_soon: '/v1/events/active',
  this_weekend:   '/v1/events/active',
  recently_added: '/v1/events',
};

export async function loadMoreSectionEvents(
  sectionId: string,
  cursor: string,
  limit = 10,
): Promise<{ data: Event[]; next_cursor: string } | null> {
  const endpoint = SECTION_ENDPOINT[sectionId];
  if (!endpoint || !cursor) return null;
  try {
    const res = await api.get<{ data: Event[]; next_cursor: string }>(endpoint, {
      params: { cursor, limit },
    });
    return { data: res.data.data ?? [], next_cursor: res.data.next_cursor ?? '' };
  } catch {
    return null;
  }
}

export async function getCart(sessionId?: string | null): Promise<CartSummary> {
  const params = sessionId ? { session_id: sessionId } : {};
  const res = await api.get<CartSummary>('/v1/cart', { params });
  return res.data;
}