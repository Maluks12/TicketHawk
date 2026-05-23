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
  first_name?: string;
  last_name?: string;
  avatar_url?: string;
}

export interface RegisterDTO {
  email: string;
  password: string;
  first_name: string;
  last_name: string;
  phone_number: string;
  date_of_birth: string;
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
  items: CartItem[];
}

export interface CartItem {
  id: string;
  item_type: 'ticket' | 'merchandise';
  event_id?: string;
  merchandise_id?: string;
  merchandise_variant_id?: string;
  quantity: number;
  unit_price: number;
  subtotal: number;
  fulfillment_method?: string;
  delivery_address?: string;
  event?: { title: string; start_time: string; venue_name: string; city: string; cover_image_url?: string };
  merchandise?: { name: string; category?: string; images?: string };
  variant?: { attributes: string; sku: string };
}

export interface CartEventGroup {
  event: CartItem['event'];
  event_id: string;
  items: CartItem[];
}

export interface TicketType {
  id: string;
  event_id: string;
  name: string;
  description: string | null;
  price: number;
  capacity: number | null;
  tickets_sold: number;
  reserved_count: number;
  sort_order: number;
  is_visible: boolean;
  sale_starts_at: string | null;
  sale_ends_at: string | null;
  available?: number | null;
  created_at: string;
  updated_at: string;
}

export interface MerchandiseItem {
  id: string;
  event_id?: string;
  event_name?: string;
  collection_id?: string;
  collection_name?: string;
  collection_cover_image_url?: string;
  name: string;
  category?: string;
  description?: string;
  images?: string;
  price: number;
  effective_price: number;
  sale_status: 'live' | 'upcoming' | 'ended';
  sale_start?: string;
  sale_end?: string;
  seconds_remain?: number;
  seconds_until?: number;
  display_order: number;
  variants?: Array<{
    id: string;
    sku: string;
    attributes: string;
    price_override?: number;
    stock_quantity: number;
  }>;
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

export const getTrendingEvents = (limit = 50) =>
  api.get<{ data: Event[] }>('/v1/events/trending', { params: { limit } });

export const getActiveEvents = (limit = 50) =>
  api.get<{ data: Event[] }>('/v1/events/active', { params: { limit } });

export const getEventsByCategory = (category: string, limit = 50) =>
  api.get<{ data: Event[] }>('/v1/events', { params: { category, limit } });

export const searchEvents = async (query: string): Promise<Event[]> => {
  try {
    const res = await api.get<{ data: Event[] }>('/v1/events/search', { params: { q: query, limit: 30 } });
    return res.data.data ?? [];
  } catch {
    return [];
  }
};

export const getSuggestionEvents = async (key: 'popular' | 'just_announced'): Promise<Event[]> => {
  try {
    const endpoint = key === 'popular' ? '/v1/events/trending' : '/v1/events';
    const params = key === 'just_announced' ? { sort: 'created_at', order: 'desc', limit: 30 } : { limit: 30 };
    const res = await api.get<{ data: Event[] }>(endpoint, { params });
    return res.data.data ?? [];
  } catch {
    return [];
  }
};

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

// ── Follow endpoints ───────────────────────────────────────────────────────────
export const toggleFollow = async (eventId: string): Promise<{ following: boolean }> => {
  const res = await api.post<{ following: boolean }>(`/v1/events/${eventId}/follow`);
  return res.data;
};

export const getFollowStatus = async (eventId: string): Promise<{ following: boolean }> => {
  const res = await api.get<{ following: boolean }>(`/v1/events/${eventId}/follow`);
  return res.data;
};

export const getFollowedEvents = async (): Promise<Event[]> => {
  try {
    const res = await api.get<{ data: Event[] }>('/v1/events/following');
    return res.data.data ?? [];
  } catch {
    return [];
  }
};

// ── Ticket types & booking ─────────────────────────────────────────────────────
export const getTicketTypes = async (eventId: string): Promise<TicketType[]> => {
  const res = await api.get<{ data: TicketType[] }>(`/v1/events/${eventId}/ticket-types`);
  return res.data.data ?? [];
};

export const createBooking = async (payload: {
  event_id: string;
  ticket_type_id: string;
  quantity: number;
  attendee_first_name?: string;
  attendee_last_name?: string;
  attendee_email?: string;
}) => {
  const res = await api.post<{ data: { id: string; booking_number: string } }>('/v1/bookings', payload);
  return res.data;
};

// ── Cart endpoints ─────────────────────────────────────────────────────────────
export const getCart = async (sessionId?: string | null): Promise<CartSummary> => {
  const params = sessionId ? { session_id: sessionId } : {};
  const res = await api.get<CartSummary>('/v1/cart', { params });
  return res.data;
};

export const addMerchToCart = async (payload: {
  item_type: 'merchandise';
  event_id?: string;
  merchandise_variant_id?: string;
  quantity: number;
  fulfillment_method?: string;
}, sessionId?: string) => {
  const params = sessionId ? { session_id: sessionId } : {};
  const res = await api.post('/v1/cart/items', payload, { params });
  return res.data;
};

export const updateCartItem = async (itemId: string, quantity: number, sessionId?: string | null) => {
  const params = sessionId ? { session_id: sessionId } : {};
  return api.patch(`/v1/cart/items/${itemId}`, { quantity }, { params });
};

export const updateCartItemAddress = async (itemId: string, address: {
  street: string; suburb?: string; city: string; province: string; postal_code?: string;
}, sessionId?: string | null) => {
  const params = sessionId ? { session_id: sessionId } : {};
  return api.patch(`/v1/cart/items/${itemId}/address`, { delivery_address: address }, { params });
};

export const removeCartItem = async (itemId: string, sessionId?: string | null) => {
  const params = sessionId ? { session_id: sessionId } : {};
  return api.delete(`/v1/cart/items/${itemId}`, { params });
};

export const clearCart = async (sessionId?: string | null) => {
  const params = sessionId ? { session_id: sessionId } : {};
  return api.delete('/v1/cart', { params });
};

// ── Merch/Drops endpoints ──────────────────────────────────────────────────────
export const getDropsFeed = async (): Promise<{ data: { available: MerchandiseItem[]; upcoming: MerchandiseItem[] } }> => {
  const res = await api.get<{ data: { available: MerchandiseItem[]; upcoming: MerchandiseItem[] } }>('/v1/merchandise/drops');
  return res;
};

// ── Cart helpers ───────────────────────────────────────────────────────────────
export function fmtZAR(n: number) {
  return `R${Math.round(n).toLocaleString('en-ZA')}`;
}

export function cartItemLabel(item: CartItem): string {
  if (item.item_type === 'ticket') return item.event?.title ?? 'Ticket';
  if (item.merchandise) return item.merchandise.name;
  return 'Item';
}

export function cartItemSubtitle(item: CartItem): string {
  if (item.item_type === 'ticket' && item.event?.start_time) {
    return new Date(item.event.start_time).toLocaleDateString('en-ZA', {
      day: 'numeric', month: 'short', year: 'numeric',
    });
  }
  if (item.variant) {
    try {
      const attr = JSON.parse(item.variant.attributes) as Record<string, string>;
      return Object.values(attr).join(' / ');
    } catch {
      return item.variant.sku;
    }
  }
  return item.fulfillment_method === 'delivery' ? 'Delivery' : 'Collection';
}