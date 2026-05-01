import axios from 'axios';
import * as SecureStore from 'expo-secure-store';

export const API_BASE_URL = 'http://192.168.1.129:8080';

export const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000,
  headers: { 'Content-Type': 'application/json' },
});

api.interceptors.request.use(async (config) => {
  const token = await SecureStore.getItemAsync('access_token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

api.interceptors.response.use(
  (res) => res,
  async (error) => {
    const original = error.config;
    if (error.response?.status === 401 && !original._retry) {
      original._retry = true;
      const refresh = await SecureStore.getItemAsync('refresh_token');
      if (!refresh) throw error;
      try {
        // FIX: path must include /v1 prefix
        const { data } = await axios.post(`${API_BASE_URL}/v1/auth/refresh`, {
          refresh_token: refresh,
        });
        // FIX: backend key is "access_token" (was "access_tokens" typo, now fixed in backend too)
        const newToken = data.access_token;
        await SecureStore.setItemAsync('access_token', newToken);
        await SecureStore.setItemAsync('refresh_token', data.refresh_token);
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

// ── Types ──────────────────────────────────────────────────────────────────

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

interface EventPage {
  data: Event[];
  next_cursor: string;
}

export interface PaginatedEvents {
  data: Event[];
  total: number;
  page: number;
  page_size: number;
  next_cursor?: string;
}

// ── API functions ──────────────────────────────────────────────────────────

export const getHomeSections     = ()                               => api.get<HomeResponse>('/v1/home');
export const getFeaturedEvents   = (limit = 1)                      => api.get<EventPage>(`/v1/events/featured?limit=${limit}`);
export const getTrendingEvents   = (limit = 10)                     => api.get<EventPage>(`/v1/events/trending?limit=${limit}`);
export const getActiveEvents     = (limit = 10)                     => api.get<EventPage>(`/v1/events/active?limit=${limit}`);
export const getEventsByCategory = (categoryId: number, limit = 5)  => api.get<EventPage>(`/v1/events/category/${categoryId}?limit=${limit}`);
export const filterEvents        = (params: Record<string, string>) => api.get<EventPage>(`/v1/events/filter`, { params });

const SECTION_ENDPOINT: Record<string, string> = {
  trending:        '/v1/events/trending',
  featured:        '/v1/events/featured',
  happening_soon:  '/v1/events/active',
};

export async function loadMoreSectionEvents(
  sectionId: string,
  cursor: string,
  limit = 10,
): Promise<{ data: Event[]; next_cursor: string } | null> {
  const endpoint = SECTION_ENDPOINT[sectionId];
  if (!endpoint || !cursor) return null;
  try {
    const res = await api.get<EventPage>(endpoint, { params: { cursor, limit } });
    return { data: res.data.data ?? [], next_cursor: res.data.next_cursor ?? '' };
  } catch {
    return null;
  }
}

// ── Merch / Drops types ────────────────────────────────────────────────────

export interface MerchandiseVariant {
  id: string;
  merchandise_id: string;
  sku: string;
  attributes: string;
  stock_quantity: number;
  price_override?: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface MerchandiseItem {
  id: string;
  event_id: string;
  event_name: string;
  name: string;
  description?: string;
  base_price: number;
  category?: string;
  display_order: number;
  is_active: boolean;
  images?: string;
  sale_start?: string;
  sale_end?: string;
  created_at: string;
  updated_at: string;
  variants?: MerchandiseVariant[];
  sale_status: string;
  seconds_until?: number;
  seconds_remain?: number;
  effective_price: number;
}

export interface DropsResponse {
  available: MerchandiseItem[];
  upcoming: MerchandiseItem[];
}

export const getDropsFeed = () => api.get<DropsResponse>('/v1/drops');
export const getMerchItem  = (id: string) => api.get<MerchandiseItem>(`/v1/merch/${id}`);

// ── Search ─────────────────────────────────────────────────────────────────

export async function searchEvents(query: string, limit = 20): Promise<Event[]> {
  const q = query.trim();
  if (!q) return [];
  try {
    const res = await api.get<{ data: Event[]; results?: Event[] }>('/v1/events/search', {
      params: { q, limit },
    });
    const payload = res.data;
    return payload.data ?? payload.results ?? [];
  } catch {
    try {
      const res = await api.get<{ data: Event[] }>('/v1/events/filter', {
        params: { search: q, limit },
      });
      return res.data.data ?? [];
    } catch {
      return [];
    }
  }
}

export async function getSuggestionEvents(
  type: 'popular' | 'just_announced',
  limit = 20,
): Promise<Event[]> {
  try {
    const sort = type === 'popular' ? 'popular' : 'newest';
    const res = await api.get<{ data: Event[] }>('/v1/events/filter', {
      params: { sort, limit },
    });
    return res.data.data ?? [];
  } catch {
    return [];
  }
}

// ── Auth types ─────────────────────────────────────────────────────────────

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
  date_of_birth: string; // *string in Go — accepted as plain string, not parsed as time.Time
}

// ── Auth API calls ─────────────────────────────────────────────────────────
// All routes: POST /v1/auth/login, /v1/auth/register, /v1/auth/logout
// Profile:    GET  /v1/user/me

export const authLogin    = (email: string, password: string) =>
  api.post<TokenResponse>('/v1/auth/login', { email, password });

export const authRegister = (dto: RegisterDTO) =>
  api.post('/v1/auth/register', dto);

export const authLogout   = () =>
  api.post('/v1/auth/logout');

export const getMe = () =>
  api.get<AuthUser>('/v1/user/me');

// ─────────────────────────────────────────────────────────────────────────────
// ── Ticket Types ─────────────────────────────────────────────────────────────
// ─────────────────────────────────────────────────────────────────────────────
 
export interface TicketType {
  id: string;
  event_id: string;
  name: string;
  description?: string | null;
  price: number;
  capacity?: number | null;
  tickets_sold: number;
  reserved_count: number;
  /** Computed server-side: capacity - tickets_sold - reserved_count (null = unlimited) */
  available?: number | null;
  sort_order: number;
  is_visible: boolean;
  sale_starts_at?: string | null;
  sale_ends_at?: string | null;
  created_at: string;
  updated_at: string;
}
 
/**
 * GET /v1/events/:eventId/ticket-types
 * Returns only visible, on-sale ticket types for the public purchase flow.
 */
export async function getTicketTypes(eventId: string): Promise<TicketType[]> {
  const res = await api.get<{ ticket_types: TicketType[]; count: number }>(
    `/v1/events/${eventId}/ticket-types`
  );
  return res.data.ticket_types ?? [];
}
 
// ─────────────────────────────────────────────────────────────────────────────
// ── Cart ─────────────────────────────────────────────────────────────────────
// ─────────────────────────────────────────────────────────────────────────────
 
export type CartItemType = 'ticket' | 'merch';
 
export interface CartItem {
  id: string;
  cart_id: string;
  item_type: CartItemType;
  event_id?: string | null;
  ticket_type_id?: string | null;
  ticket_type_name?: string | null;
  merchandise_variant_id?: string | null;
  quantity: number;
  unit_price: number;
  delivery_fee?: number;
  fulfillment_method?: string | null;
  event_title?: string;
}
 
export interface CartSummary {
  id: string;
  items: CartItem[];
  total: number;
  item_count: number;
}
 
export interface AddTicketToCartDTO {
  item_type: 'ticket';
  event_id: string;
  ticket_type_id?: string;
  quantity: number;
}
 
/**
 * POST /v1/cart/items
 * Adds a ticket item to the cart. Pass sessionId for guest (unauthenticated) carts.
 */
export async function addTicketToCart(
  dto: AddTicketToCartDTO,
  sessionId?: string | null,
): Promise<CartItem> {
  const headers: Record<string, string> = {};
  if (sessionId) headers['X-Session-ID'] = sessionId;
  const res = await api.post<CartItem>('/v1/cart/items', dto, { headers });
  return res.data;
}
 
/**
 * GET /v1/cart
 */
export async function getCart(sessionId?: string | null): Promise<CartSummary> {
  const headers: Record<string, string> = {};
  if (sessionId) headers['X-Session-ID'] = sessionId;
  const res = await api.get<CartSummary>('/v1/cart', { headers });
  return res.data;
}
 
/**
 * DELETE /v1/cart
 * Clears all items from the cart.
 */
export async function clearCart(sessionId?: string | null): Promise<void> {
  const headers: Record<string, string> = {};
  if (sessionId) headers['X-Session-ID'] = sessionId;
  await api.delete('/v1/cart', { headers });
}
 
/**
 * PATCH /v1/cart/items/:id
 * Updates quantity of a single cart item.
 */
export async function updateCartItem(
  itemId: string,
  quantity: number,
  sessionId?: string | null,
): Promise<CartItem> {
  const headers: Record<string, string> = {};
  if (sessionId) headers['X-Session-ID'] = sessionId;
  const res = await api.patch<CartItem>(`/v1/cart/items/${itemId}`, { quantity }, { headers });
  return res.data;
}
 
/**
 * DELETE /v1/cart/items/:id
 * Removes one item from the cart.
 */
export async function removeCartItem(
  itemId: string,
  sessionId?: string | null,
): Promise<void> {
  const headers: Record<string, string> = {};
  if (sessionId) headers['X-Session-ID'] = sessionId;
  await api.delete(`/v1/cart/items/${itemId}`, { headers });
}
 
// ─────────────────────────────────────────────────────────────────────────────
// ── Checkout ─────────────────────────────────────────────────────────────────
// ─────────────────────────────────────────────────────────────────────────────
 
export interface ReservationSummary {
  reservation_id: string;
  event_id: string;
  event_title: string;
  quantity: number;
  seconds_left: number;
}
 
export interface CheckoutBeginResponse {
  reservations: ReservationSummary[];
  /** Shortest TTL across all reservations — use this for the countdown timer */
  expires_in_seconds: number;
  cart_total: number;
}
 
/**
 * POST /v1/checkout/begin
 * Creates 10-minute ticket reservations for all ticket items in the cart.
 * Call this before sending the user to the Stitch payment page.
 */
export async function checkoutBegin(
  sessionId?: string | null,
): Promise<CheckoutBeginResponse> {
  const headers: Record<string, string> = {};
  if (sessionId) headers['X-Session-ID'] = sessionId;
  const res = await api.post<CheckoutBeginResponse>('/v1/checkout/begin', {}, { headers });
  return res.data;
}
 
/**
 * POST /v1/checkout/heartbeat
 * Extends the reservation TTL. Call every ~60 s while the user is on the
 * payment page to prevent the reservation from expiring mid-payment.
 */
export async function checkoutHeartbeat(sessionId?: string | null): Promise<void> {
  const headers: Record<string, string> = {};
  if (sessionId) headers['X-Session-ID'] = sessionId;
  await api.post('/v1/checkout/heartbeat', {}, { headers });
}
 
/**
 * POST /v1/checkout/abandon
 * Releases all active reservations and clears the cart.
 * Call when the user navigates away from payment without completing it.
 */
export async function checkoutAbandon(sessionId?: string | null): Promise<void> {
  const headers: Record<string, string> = {};
  if (sessionId) headers['X-Session-ID'] = sessionId;
  await api.post('/v1/checkout/abandon', {}, { headers });
}