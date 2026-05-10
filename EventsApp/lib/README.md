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
        const { data } = await axios.post(`${API_BASE_URL}/v1/auth/refresh`, {
          refresh_token: refresh,
        });
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

// ── Session ID cache ───────────────────────────────────────────────────────
// Reads SecureStore once and keeps the result in memory. Auto-generates a
// UUID if no session exists so guest carts always have an X-Session-ID.

let _cachedSessionId: string | undefined = undefined; // never null — always generates

function generateUUID(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = Math.random() * 16 | 0;
    const v = c === 'x' ? r : (r & 0x3 | 0x8);
    return v.toString(16);
  });
}

export async function getCachedSessionId(): Promise<string> {
  if (_cachedSessionId) return _cachedSessionId;
  try {
    let sid = await SecureStore.getItemAsync('session_id');
    if (!sid) {
      sid = generateUUID();
      await SecureStore.setItemAsync('session_id', sid);
    }
    _cachedSessionId = sid;
    return sid;
  } catch {
    // SecureStore unavailable — use in-memory UUID for this session
    if (!_cachedSessionId) _cachedSessionId = generateUUID();
    return _cachedSessionId;
  }
}

export function makeSessionHeaders(sid: string | null): Record<string, string> {
  return sid ? { 'X-Session-ID': sid } : {};
}

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

export interface Category {
  id: number;
  name: string;
  slug?: string;
}

export const getCategories = () => api.get<{ categories: Category[] }>('/v1/categories');

// ── API functions ──────────────────────────────────────────────────────────

export const getHomeSections     = (city?: string)                  => api.get<HomeResponse>('/v1/home', city ? { params: { city } } : undefined);
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
  collection_id?: string;
  collection_name?: string;
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
  // ── Fulfillment — returned by backend, required when adding to cart ──────
  fulfillment_mode?:  'delivery' | 'pickup' | 'both';
  pickup_location?:   string | null;
  delivery_fee?:      number;
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
  date_of_birth: string;
}

// ── Auth API calls ─────────────────────────────────────────────────────────

export const authLogin    = (email: string, password: string) =>
  api.post<TokenResponse>('/v1/auth/login', { email, password });

export const authRegister = (dto: RegisterDTO) =>
  api.post('/v1/auth/register', dto);

export const authLogout   = () =>
  api.post('/v1/auth/logout');

export const getMe = () =>
  api.get<AuthUser>('/v1/user/me');

// ── Ticket Types ───────────────────────────────────────────────────────────

export interface TicketType {
  id: string;
  event_id: string;
  name: string;
  description?: string | null;
  price: number;
  capacity?: number | null;
  tickets_sold: number;
  reserved_count: number;
  available?: number | null;
  sort_order: number;
  is_visible: boolean;
  sale_starts_at?: string | null;
  sale_ends_at?: string | null;
  created_at: string;
  updated_at: string;
}

export async function getTicketTypes(eventId: string): Promise<TicketType[]> {
  const res = await api.get<{ ticket_types: TicketType[]; count: number }>(
    `/v1/events/${eventId}/ticket-types`
  );
  return res.data.ticket_types ?? [];
}

// ── Cart ───────────────────────────────────────────────────────────────────

export interface CartItem {
  id: string;
  cart_id: string;
  item_type: 'ticket' | 'merchandise';
  event_id: string;
  ticket_type_id?: string | null;
  ticket_type_name?: string | null;
  merchandise_variant_id?: string | null;
  quantity: number;
  unit_price_snapshot: number;
  delivery_fee_snapshot: number;
  fulfillment_method?: string | null;
  delivery_address?: string | null;
  event_title?: string | null;
  variant_sku?: string | null;
  merch_name?: string | null;
  variant_attributes?: string | null;
  pickup_location?: string | null;
  created_at: string;
}

export interface CartEventGroup {
  event_id: string;
  event_title: string;
  items: CartItem[];
  subtotal: number;
}

export interface CartSummary {
  total_items: number;
  total_amount: number;
  currency: string;
  items_by_event: CartEventGroup[];
}

export interface AddTicketToCartDTO {
  item_type: 'ticket';
  event_id: string;
  ticket_type_id?: string;
  quantity: number;
}

export interface AddMerchToCartDTO {
  item_type: 'merchandise';
  event_id: string;
  merchandise_variant_id?: string;
  quantity: number;
  // Required by the backend — must always be sent
  fulfillment_method: 'pickup' | 'delivery';
  // Required when fulfillment_method === 'delivery'
  delivery_address?: {
    street:       string;
    suburb?:      string;
    city:         string;
    province:     string;
    postal_code?: string;
  };
}

export async function getCart(sessionId?: string | null): Promise<CartSummary> {
  const headers = makeSessionHeaders(sessionId ?? null);
  const res = await api.get<CartSummary>('/v1/cart', { headers });
  return res.data;
}

export async function addTicketToCart(
  dto: AddTicketToCartDTO,
  sessionId?: string | null,
): Promise<CartItem> {
  const headers = makeSessionHeaders(sessionId ?? null);
  const res = await api.post<CartItem>('/v1/cart/items', dto, { headers });
  return res.data;
}

export async function addMerchToCart(
  dto: AddMerchToCartDTO,
  sessionId?: string | null,
): Promise<CartItem> {
  const headers = makeSessionHeaders(sessionId ?? null);
  const res = await api.post<CartItem>('/v1/cart/items', dto, { headers });
  return res.data;
}

export async function updateCartItem(
  itemId: string,
  quantity: number,
  sessionId?: string | null,
): Promise<void> {
  const headers = makeSessionHeaders(sessionId ?? null);
  await api.patch(`/v1/cart/items/${itemId}`, { quantity }, { headers });
}

export async function removeCartItem(
  itemId: string,
  sessionId?: string | null,
): Promise<void> {
  const headers = makeSessionHeaders(sessionId ?? null);
  await api.delete(`/v1/cart/items/${itemId}`, { headers });
}

export async function clearCart(sessionId?: string | null): Promise<void> {
  const headers = makeSessionHeaders(sessionId ?? null);
  await api.delete('/v1/cart', { headers });
}

// ── Cart UI helpers ────────────────────────────────────────────────────────

export function cartItemLabel(
  item: Pick<CartItem, 'item_type' | 'ticket_type_name' | 'merch_name'>
): string {
  if (item.item_type === 'ticket') return item.ticket_type_name ?? 'General Admission';
  return item.merch_name ?? 'Merchandise';
}

export function cartItemSubtitle(item: CartItem): string | null {
  if (item.item_type !== 'merchandise') return null;
  const parts: string[] = [];
  if (item.variant_sku) parts.push(item.variant_sku);
  if (item.variant_attributes) {
    try {
      const attrs = JSON.parse(item.variant_attributes);
      Object.entries(attrs).forEach(([k, v]) => parts.push(`${k}: ${v}`));
    } catch {
      parts.push(item.variant_attributes);
    }
  }
  if (item.fulfillment_method === 'delivery') parts.push('📦 Delivery');
  else if (item.fulfillment_method === 'pickup') {
    parts.push(item.pickup_location ? `🏪 Pickup · ${item.pickup_location}` : '🏪 Pickup');
  }
  return parts.join(' · ') || null;
}

export function fmtZAR(amount: number): string {
  return `R${Math.round(amount).toLocaleString('en-ZA')}`;
}

// ── Checkout ───────────────────────────────────────────────────────────────

export interface ReservationSummary {
  reservation_id: string;
  event_id: string;
  event_title: string;
  quantity: number;
  seconds_left: number;
}

export interface CheckoutBeginResponse {
  reservations: ReservationSummary[];
  seconds_left: number;
  cart_total: number;
  currency: string;
}

export interface CheckoutConfirmResponse {
  client_token: string;
  payment_request_id: string;
  total_amount: number;
  currency: string;
  primary_booking_id: string;
  booking_ids: string[];
}

export async function checkoutBegin(sessionId?: string | null): Promise<CheckoutBeginResponse> {
  const headers = makeSessionHeaders(sessionId ?? null);
  const res = await api.post<CheckoutBeginResponse>('/v1/checkout/begin', {}, { headers });
  return res.data;
}

export async function checkoutConfirm(sessionId?: string | null): Promise<CheckoutConfirmResponse> {
  const headers = makeSessionHeaders(sessionId ?? null);
  const res = await api.post<CheckoutConfirmResponse>('/v1/checkout/confirm', {}, { headers });
  return res.data;
}

export async function checkoutHeartbeat(sessionId?: string | null): Promise<void> {
  const headers = makeSessionHeaders(sessionId ?? null);
  await api.post('/v1/checkout/heartbeat', {}, { headers });
}

export async function checkoutAbandon(sessionId?: string | null): Promise<void> {
  const headers = makeSessionHeaders(sessionId ?? null);
  await api.post('/v1/checkout/abandon', {}, { headers });
}

// ── Bookings ───────────────────────────────────────────────────────────────

export interface CreateBookingDTO {
  event_id: string;
  ticket_type_id?: string;
  ticket_quantity: number;
  attendee_first_name?: string;
  attendee_last_name?: string;
  attendee_email?: string;
}

export interface BookingResponse {
  booking_id: string;
  booking_number: string;
  total_amount: number;
  currency: string;
  client_token: string;
  payment_request_id: string;
  expires_at: string;
}

export interface PaymentStatusResponse {
  booking_id: string;
  booking_number: string;
  payment_status: 'pending' | 'processing' | 'completed' | 'failed' | 'refunded';
  payment_method?: string;
  amount: number;
  currency: string;
  stitch_payment_url?: string;
  stitch_payment_request_id?: string;
  transaction_id?: string;
  completed_at?: string;
  webhook_received: boolean;
}

export async function createBooking(dto: CreateBookingDTO): Promise<BookingResponse> {
  const res = await api.post<BookingResponse>('/v1/bookings', dto);
  return res.data;
}

export async function getPaymentStatus(bookingId: string): Promise<PaymentStatusResponse> {
  const res = await api.get<PaymentStatusResponse>(`/v1/payments/${bookingId}/status`);
  return res.data;
}

export async function pollPaymentStatus(
  bookingId: string,
  intervalMs = 2000,
  timeoutMs = 120_000,
): Promise<PaymentStatusResponse> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const status = await getPaymentStatus(bookingId);
    if (status.payment_status === 'completed' || status.payment_status === 'failed') {
      return status;
    }
    await new Promise(r => setTimeout(r, intervalMs));
  }
  throw new Error('Payment confirmation timed out');
}