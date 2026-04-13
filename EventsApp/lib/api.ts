import axios from 'axios';
import * as SecureStore from 'expo-secure-store';

// ── Find your IP: run `ipconfig` (Windows) and look for Wi-Fi adapter IPv4
// Your phone and PC must be on the same WiFi network
export const API_BASE_URL = 'http://192.168.1.129:8080';

export const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000,
  headers: { 'Content-Type': 'application/json' },
});

// Attach access token to every request
api.interceptors.request.use(async (config) => {
  const token = await SecureStore.getItemAsync('access_token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

// Auto-refresh on 401
api.interceptors.response.use(
  (res) => res,
  async (error) => {
    const original = error.config;
    if (error.response?.status === 401 && !original._retry) {
      original._retry = true;
      const refresh = await SecureStore.getItemAsync('refresh_token');
      if (!refresh) throw error;
      try {
        const { data } = await axios.post(`${API_BASE_URL}/auth/refresh`,
          { refresh_token: refresh }
        );
        await SecureStore.setItemAsync('access_token', data.access_token);
        await SecureStore.setItemAsync('refresh_token', data.refresh_token);
        original.headers.Authorization = `Bearer ${data.access_token}`;
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

// ── Types ──────────────────────────────────────────────────────────────────

export interface Event {
  id: string;
  slug: string;
  title: string;
  description: string;
  category: string;
  venue_name: string;
  city: string;
  cover_image_url?: string;
  banner_image_url?: string;  // wide cinematic image — used for hero + sports banner
  is_featured: boolean;
  is_sponsored?: boolean;     // injected by ad system at feed assembly time
  status: string;
  start_time: string;
  end_time: string;
  price: number;
  currency: string;
  views: number;
  is_sold_out?: boolean;
  available_tickets?: number;
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

interface EventPage {
  data: Event[];
  next_cursor: string;
}

// ── API functions ──────────────────────────────────────────────────────────

export const getHomeSections    = ()                          => api.get<HomeResponse>('/v1/home');
export const getFeaturedEvents  = (limit = 1)                 => api.get<EventPage>(`/v1/events/featured?limit=${limit}`);
export const getTrendingEvents  = (limit = 10)                => api.get<EventPage>(`/v1/events/trending?limit=${limit}`);
export const getActiveEvents    = (limit = 10)                => api.get<EventPage>(`/v1/events/active?limit=${limit}`);
export const getEventsByCategory = (categoryId: number, limit = 5) =>
  api.get<EventPage>(`/v1/events/category/${categoryId}?limit=${limit}`);
export const filterEvents       = (params: Record<string, string>) =>
  api.get<EventPage>(`/v1/events/filter`, { params });

// ── Merch / Drops types ────────────────────────────────────────────────────

export interface MerchandiseVariant {
  id: string;
  merchandise_id: string;
  sku: string;
  attributes: string;       // JSON string e.g. '{"size":"L","colour":"red"}'
  stock_quantity: number;
  price_override?: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface MerchandiseItem {
  id: string;
  event_id: string;
  name: string;
  description?: string;
  base_price: number;
  category?: string;
  display_order: number;
  is_active: boolean;
  images?: string;          // JSON string array e.g. '["https://...","https://..."]'
  sale_start?: string;      // ISO timestamp
  sale_end?: string;        // ISO timestamp
  created_at: string;
  updated_at: string;
  variants?: MerchandiseVariant[];
  // Computed fields added by the drops handler
  sale_status: string;      // "live" | "scheduled" | "ended"
  seconds_until?: number;   // countdown to drop going live (upcoming only)
  seconds_remain?: number;  // countdown to expiry (available only)
  effective_price: number;  // lowest variant price, or base_price
}

export interface DropsResponse {
  available: MerchandiseItem[];
  upcoming: MerchandiseItem[];
}

export const getDropsFeed = () => api.get<DropsResponse>('/v1/drops');
export const getMerchItem  = (id: string) => api.get<MerchandiseItem>(`/v1/merch/${id}`);