import axios from 'axios';

export const api = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080',
  withCredentials: true,
});

export const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080';

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

export function makeSessionHeaders(sid: string | null): Record<string, string> {
  return sid ? { 'X-Session-ID': sid } : {};
}

// ── Request interceptor ───────────────────────────────────────────────────────
api.interceptors.request.use((config) => {
  const token = getAccessToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

// ── Response interceptor ──────────────────────────────────────────────────────
// SINGLE-FLIGHT REFRESH: the backend rotates refresh tokens and treats a
// replayed (already-rotated) token as theft, revoking EVERY session for that
// user. A page like home fires several requests in parallel; if the access
// token has expired they all 401 together, and the old interceptor launched
// one refresh call PER request. The first one won and rotated the token — the
// rest then presented the just-rotated token and tripped reuse detection,
// nuking the user's brand-new session and leaving the page unable to load.
// Now all concurrent 401s share ONE in-flight refresh promise.
let refreshInFlight: Promise<string> | null = null;

function refreshAccessToken(): Promise<string> {
  if (!refreshInFlight) {
    refreshInFlight = (async () => {
      const refresh = typeof window !== 'undefined' ? localStorage.getItem('refresh_token') : null;
      if (!refresh) throw new Error('no refresh token');
      const { data } = await axios.post(`${API_BASE_URL}/v1/auth/refresh`, { refresh_token: refresh });
      saveTokens(data.access_token, data.refresh_token);
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
        const accessToken = await refreshAccessToken();
        original.headers.Authorization = `Bearer ${accessToken}`;
        return api(original);
      } catch {
        clearTokens();
        // Don't hard-redirect from public pages — anonymous browsing (home,
        // event pages) works without a session. Only bounce to login if the
        // user is somewhere that requires one.
        if (typeof window !== 'undefined') {
          const p = window.location.pathname;
          const needsAuth = p.startsWith('/profile') || p.startsWith('/tickets') || p.startsWith('/booking');
          if (needsAuth) window.location.href = '/auth/login';
        }
        return Promise.reject(error);
      }
    }
    return Promise.reject(error);
  }
);

// ── Types ─────────────────────────────────────────────────────────────────────
export interface AuthUser {
  id: string;
  email: string;
  role: string;
  name: string;
  first_name?: string;
  last_name?: string;
  phone_number?: string;
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
  // Populated from the organiser's Google Places selection. Clients link
  // with place_id (or lat/lng) to open the EXACT venue rather than a fuzzy
  // text search. Optional: events created before this feature have none.
  latitude?: number | null;
  longitude?: number | null;
  place_id?: string | null;
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
  max_tickets_per_user?: number | null;
  venue_type?: 'stadium' | 'auditorium' | 'none' | null;
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
  total_amount: number;
  currency: string;
  items_by_event: CartEventGroup[];
}

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
  event_id?: string;
  event_name?: string;
  collection_id?: string;
  collection_name?: string;
  collection_cover_image_url?: string;
  name: string;
  category?: string;
  description?: string;
  images?: string;
  base_price: number;
  effective_price: number;
  sale_status: 'live' | 'upcoming' | 'ended';
  sale_start?: string;
  sale_end?: string;
  seconds_remain?: number;
  seconds_until?: number;
  display_order: number;
  is_active: boolean;
  fulfillment_mode?: 'delivery' | 'pickup' | 'both';
  pickup_location?: string | null;
  delivery_fee?: number;
  variants?: MerchandiseVariant[];
  created_at: string;
  updated_at: string;
}

export interface DropsResponse {
  available: MerchandiseItem[];
  upcoming: MerchandiseItem[];
}

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

// ── Auth endpoints ─────────────────────────────────────────────────────────────
export const authLogin = (email: string, password: string) =>
  api.post<{ access_token: string; refresh_token: string }>('/v1/auth/login', { email, password });

export const authRegister = (dto: RegisterDTO) =>
  api.post('/v1/auth/register', dto);

// Send our refresh token so the server revokes ONLY this session — logging
// out here no longer kills your other devices and tabs.
export const authLogout = () =>
  api.post('/v1/auth/logout', {
    refresh_token: typeof window !== 'undefined' ? localStorage.getItem('refresh_token') ?? '' : '',
  });

export const getMe = () =>
  api.get<AuthUser>('/v1/user/me');

// ── Event endpoints ────────────────────────────────────────────────────────────
export const getHomeSections = (city?: string) =>
  api.get<HomeResponse>('/v1/home', city ? { params: { city } } : undefined);

export const getCategories = () =>
  api.get<{ categories: Category[] }>('/v1/categories');

export const getTrendingEvents = (limit = 50) =>
  api.get<{ data: Event[] }>('/v1/events/trending', { params: { limit } });

export const getActiveEvents = (limit = 50) =>
  api.get<{ data: Event[] }>('/v1/events/active', { params: { limit } });

export const getEventsByCategory = (categoryId: number, limit = 50) =>
  api.get<{ data: Event[] }>(`/v1/events/category/${categoryId}`, { params: { limit } });

export async function getEventsByIds(ids: string[]): Promise<Event[]> {
  if (!ids.length) return [];
  const res = await api.get<{ data: Event[] }>('/v1/events/by-ids', {
    params: { ids: ids.join(',') },
  });
  return res.data.data ?? [];
}

export const searchEvents = async (query: string): Promise<Event[]> => {
  try {
    const res = await api.get<{ data: Event[] }>('/v1/events/search', { params: { q: query, limit: 30 } });
    return res.data.data ?? [];
  } catch {
    return [];
  }
};

export const getSuggestionEvents = async (key: 'popular' | 'just_announced' | 'featured'): Promise<Event[]> => {
  try {
    const endpoint =
      key === 'popular'  ? '/v1/events/trending' :
      key === 'featured' ? '/v1/events/featured'  :
      '/v1/events';
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

// ── Ticket types ───────────────────────────────────────────────────────────────
export const getTicketTypes = async (eventId: string): Promise<TicketType[]> => {
  const res = await api.get<{ ticket_types: TicketType[]; count: number }>(`/v1/events/${eventId}/ticket-types`);
  return res.data.ticket_types ?? [];
};

// ── Bookings ───────────────────────────────────────────────────────────────────
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

export interface BookingDetail {
  id: string;
  booking_number: string;
  event_id: string;
  ticket_quantity: number;
  unit_price?: number;
  subtotal?: number;
  platform_fee?: number;
  organizer_fee?: number;
  total_amount: number;
  discount_amount?: number;
  currency: string;
  booking_status: string;
  payment_status: string;
  qr_code_url?: string | null;
  tickets_sent?: boolean;
  ticket_type_name?: string | null;
  attendee_first_name?: string | null;
  attendee_last_name?: string | null;
  attendee_email?: string | null;
  created_at: string;
  confirmed_at?: string | null;
  refunded_at?: string | null;
  refunded_reason?: string | null;
  refunded_amount?: number | null;
  event?: {
    title: string;
    start_time: string;
    venue_name: string;
    city: string;
    cover_image_url?: string | null;
    address?: string;
  };
}

export interface TicketAssignment {
  id: string;
  slot_number: number;
  status: 'unassigned' | 'pending' | 'claimed' | 'revoked' | 'cancelled';
  assignee_name?: string | null;
  assignee_email?: string | null;
  qr_code_url?: string | null;
}

export const createBooking = async (dto: CreateBookingDTO): Promise<BookingResponse> => {
  const res = await api.post<BookingResponse>('/v1/bookings', dto);
  return res.data;
};

export async function getBookingDetail(bookingId: string): Promise<BookingDetail> {
  const res = await api.get<BookingDetail>(`/v1/bookings/${bookingId}`);
  return res.data;
}

export async function getTicketAssignments(bookingId: string): Promise<TicketAssignment[]> {
  const res = await api.get<{ data: TicketAssignment[] }>(`/v1/bookings/${bookingId}/assignments`);
  return res.data?.data ?? [];
}

export async function assignTicket(
  bookingId: string,
  assignmentId: string,
  assignee_name: string,
  assignee_email: string,
): Promise<void> {
  await api.post(`/v1/bookings/${bookingId}/assignments/${assignmentId}`, {
    assignee_name,
    assignee_email,
  });
}

export async function revokeAssignment(bookingId: string, assignmentId: string): Promise<void> {
  await api.delete(`/v1/bookings/${bookingId}/assignments/${assignmentId}`);
}

// ── Cart endpoints ─────────────────────────────────────────────────────────────
export const getCart = async (sessionId?: string | null): Promise<CartSummary> => {
  const headers = makeSessionHeaders(sessionId ?? null);
  const res = await api.get<CartSummary>('/v1/cart', { headers });
  return res.data;
};

export interface AddTicketToCartDTO {
  item_type: 'ticket';
  event_id: string;
  ticket_type_id: string;
  quantity: number;
}

export async function addTicketToCart(
  payload: AddTicketToCartDTO,
  sessionId?: string | null,
): Promise<void> {
  const headers = makeSessionHeaders(sessionId ?? null);
  await api.post('/v1/cart/items', payload, { headers });
}

export interface AddMerchToCartDTO {
  item_type: 'merchandise';
  event_id?: string;
  merchandise_variant_id?: string;
  quantity: number;
  fulfillment_method?: 'pickup' | 'delivery';
  delivery_address?: {
    street: string;
    suburb?: string;
    city: string;
    province: string;
    postal_code?: string;
  };
}

export const addMerchToCart = async (
  payload: AddMerchToCartDTO,
  sessionId?: string | null,
): Promise<CartItem> => {
  const headers = makeSessionHeaders(sessionId ?? null);
  const res = await api.post<CartItem>('/v1/cart/items', payload, { headers });
  return res.data;
};

export const updateCartItem = async (itemId: string, quantity: number, sessionId?: string | null) => {
  const headers = makeSessionHeaders(sessionId ?? null);
  return api.patch(`/v1/cart/items/${itemId}`, { quantity }, { headers });
};

export const updateCartItemAddress = async (itemId: string, address: {
  street: string; suburb?: string; city: string; province: string; postal_code?: string;
}, sessionId?: string | null) => {
  const headers = makeSessionHeaders(sessionId ?? null);
  return api.patch(`/v1/cart/items/${itemId}`, { delivery_address: address }, { headers });
};

export const removeCartItem = async (itemId: string, sessionId?: string | null) => {
  const headers = makeSessionHeaders(sessionId ?? null);
  return api.delete(`/v1/cart/items/${itemId}`, { headers });
};

export const clearCart = async (sessionId?: string | null) => {
  const headers = makeSessionHeaders(sessionId ?? null);
  return api.delete('/v1/cart', { headers });
};

// ── Merch/Drops endpoints ──────────────────────────────────────────────────────
export const getDropsFeed = async (): Promise<DropsResponse> => {
  const res = await api.get<DropsResponse>('/v1/drops');
  return res.data;
};

export const getMerchItem = (id: string) =>
  api.get<MerchandiseItem>(`/v1/merch/${id}`);

// ── Cart helpers ───────────────────────────────────────────────────────────────
export function fmtZAR(n: number) {
  return `R${Math.round(n).toLocaleString('en-ZA')}`;
}

export function cartItemLabel(item: CartItem): string {
  if (item.item_type === 'ticket') return item.ticket_type_name ?? 'General Admission';
  return item.merch_name ?? 'Merchandise';
}

export function cartItemSubtitle(item: CartItem): string | null {
  if (item.item_type !== 'merchandise') return null;
  const parts: string[] = [];
  if (item.variant_sku) parts.push(item.variant_sku);
  if (item.variant_attributes) {
    try {
      const attrs = JSON.parse(item.variant_attributes) as Record<string, string>;
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

// ── Payments ───────────────────────────────────────────────────────────────────
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
    await new Promise((r) => setTimeout(r, intervalMs));
  }
  throw new Error('Payment polling timed out');
}

// ── Checkout ───────────────────────────────────────────────────────────────────
export async function checkoutBegin(sessionId?: string | null): Promise<CheckoutBeginResponse> {
  const headers = makeSessionHeaders(sessionId ?? null);
  const res = await api.post<CheckoutBeginResponse>('/v1/checkout/begin', {}, { headers });
  return res.data;
}

export async function checkoutConfirm(sessionId?: string | null): Promise<CheckoutConfirmResponse> {
  const headers = makeSessionHeaders(sessionId ?? null);
  // channel=web tags the payment so the gateway return page is web-friendly.
  const res = await api.post<CheckoutConfirmResponse>('/v1/checkout/confirm', {}, { headers, params: { channel: 'web' } });
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

// ── Follow endpoints ───────────────────────────────────────────────────────────
export interface FollowStatusResponse {
  following: boolean;
  follow_count: number;
  event_id: string;
}

export const toggleFollow = async (eventId: string): Promise<FollowStatusResponse> => {
  const res = await api.post<FollowStatusResponse>(`/v1/events/${eventId}/follow`);
  return res.data;
};

export const getFollowStatus = async (eventId: string): Promise<FollowStatusResponse> => {
  const res = await api.get<FollowStatusResponse>(`/v1/events/${eventId}/follow`);
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

// ── Following feed types & helpers ─────────────────────────────────────────────
export interface FollowedEvent {
  follow_id: string;
  followed_at: string;
  id: string;
  slug: string;
  title: string;
  start_time: string;
  end_time: string;
  venue_name: string;
  city: string;
  cover_image_url?: string | null;
  price: number;
  currency: string;
  status: string;
  category: string;
  tickets_sold: number;
  max_capacity?: number | null;
  is_sold_out: boolean;
  available_tickets: number;
}

export interface FollowingResponse {
  data: FollowedEvent[];
  next_cursor: string | null;
}

export function fmtFollowDate(iso?: string | null): string {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-ZA', {
    weekday: 'short', day: 'numeric', month: 'short',
  });
}

export function fmtFollowPrice(price: number): string {
  if (!price || price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

export function followAvailPct(sold?: number, capacity?: number | null): number | null {
  if (sold == null || capacity == null || capacity === 0) return null;
  return Math.round((sold / capacity) * 100);
}

// ── Rotating QR ────────────────────────────────────────────────────────────────
export interface RotatingQRToken {
  token: string;
  expires_at: string;
  window_sec: number;
  issued_at: string;
}

export async function getRotatingQRToken(assignmentId: string): Promise<RotatingQRToken> {
  const res = await api.get<RotatingQRToken>(`/v1/tickets/${assignmentId}/qr`);
  return res.data;
}