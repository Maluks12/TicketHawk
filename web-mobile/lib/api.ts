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

// ── Response interceptor ──────────────────────────────────────────────────────
//
// THE RULE: a session ends when the SERVER rejects the refresh token, or when
// the user clicks Log out. Nothing else.
//
// The previous version wrapped the refresh in a bare `catch` that called
// clearTokens() and bounced to /auth/login. That fired on a dropped
// connection, a request timeout, a 502 from the Cloudflare Tunnel, any 5xx —
// none of which mean the session is invalid. Transient failures are now
// retried, and if they still fail the tokens are left alone so the next
// request can succeed.
//
// SINGLE-FLIGHT REFRESH is retained: the backend rotates refresh tokens and
// treats a replayed one as theft. A page like home fires several requests in
// parallel; if the access token has expired they all 401 together, and one
// refresh call per request meant the losers presented the just-rotated token
// and tripped reuse detection, nuking every session the user had.

/** Thrown only when the server has definitively rejected the refresh token. */
class SessionExpiredError extends Error {
  constructor(msg = 'session expired') { super(msg); this.name = 'SessionExpiredError'; }
}

const REFRESH_TIMEOUT_MS   = 30000;
const REFRESH_MAX_ATTEMPTS = 4;

/**
 * 401/403 — the server looked the token up and rejected it (expired, revoked,
 *           reused). Final.
 * 400     — malformed body; a retry sends the identical body. Final.
 * Anything else — no response at all, 408, 429, 5xx, tunnel 502/503/504 — is
 * transient and must not cost the user their session.
 */
function isDefinitiveAuthRejection(err: any): boolean {
  const status = err?.response?.status;
  return status === 400 || status === 401 || status === 403;
}

// Bumped whenever a new access token lands, so a request that was already in
// flight when a refresh completed can just be retried with the current token
// instead of kicking off a second (token-rotating) refresh.
let tokenGeneration = 0;

let refreshInFlight: Promise<string> | null = null;

async function doRefresh(): Promise<string> {
  const refresh = typeof window !== 'undefined' ? localStorage.getItem('refresh_token') : null;
  if (!refresh) throw new SessionExpiredError('no refresh token');

  let lastErr: any = null;

  for (let attempt = 0; attempt < REFRESH_MAX_ATTEMPTS; attempt++) {
    try {
      const { data } = await axios.post(
        `${API_BASE_URL}/v1/auth/refresh`,
        { refresh_token: refresh },
        { timeout: REFRESH_TIMEOUT_MS },
      );
      if (!data?.access_token || !data?.refresh_token) throw new Error('malformed refresh response');
      saveTokens(data.access_token, data.refresh_token);
      tokenGeneration++;
      return data.access_token as string;
    } catch (err: any) {
      lastErr = err;
      if (isDefinitiveAuthRejection(err)) {
        throw new SessionExpiredError('refresh token rejected by server');
      }
      // Transient. Back off and present the SAME token again — the server's
      // rotation grace window exists precisely so a retry after a lost
      // response reads as a race, not a replay.
      if (attempt < REFRESH_MAX_ATTEMPTS - 1) {
        await new Promise((r) => setTimeout(r, 600 * Math.pow(2, attempt)));
      }
    }
  }

  throw lastErr ?? new Error('refresh failed');
}

function refreshAccessToken(): Promise<string> {
  if (!refreshInFlight) {
    refreshInFlight = doRefresh().finally(() => { refreshInFlight = null; });
  }
  return refreshInFlight;
}

api.interceptors.request.use((config) => {
  const token = getAccessToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  (config as any)._tokenGen = tokenGeneration;
  return config;
});

const MAX_AUTH_RETRIES = 2;

api.interceptors.response.use(
  (res) => res,
  async (error) => {
    const original: any = error.config;
    if (error.response?.status !== 401 || !original) return Promise.reject(error);

    original._authRetries = (original._authRetries ?? 0) + 1;
    if (original._authRetries > MAX_AUTH_RETRIES) return Promise.reject(error);

    // Raced a refresh that has already finished — the header was simply stale
    // by the time the server saw it. Retry, don't rotate.
    if (original._tokenGen !== undefined && original._tokenGen !== tokenGeneration) {
      original._tokenGen = tokenGeneration;
      const current = getAccessToken();
      if (current) {
        original.headers = original.headers ?? {};
        original.headers.Authorization = `Bearer ${current}`;
        return api(original);
      }
    }

    try {
      const accessToken = await refreshAccessToken();
      original.headers = original.headers ?? {};
      original.headers.Authorization = `Bearer ${accessToken}`;
      return api(original);
    } catch (refreshErr) {
      if (refreshErr instanceof SessionExpiredError) {
        // The ONLY path that signs someone out.
        clearTokens();
        // Don't hard-redirect from public pages — anonymous browsing (home,
        // event pages) works without a session. Only bounce to login if the
        // user is somewhere that requires one.
        if (typeof window !== 'undefined') {
          const p = window.location.pathname;
          const needsAuth = p.startsWith('/profile') || p.startsWith('/tickets') || p.startsWith('/booking');
          if (needsAuth) window.location.href = '/auth/login';
        }
      }
      // Transient failure: tokens untouched, user stays signed in, caller gets
      // the original error to handle.
      return Promise.reject(error);
    }
  }
);

/**
 * Renews the access token if it is expired or nearly so. Safe to call often —
 * callers share the single-flight promise, so N callers cause at most one
 * network call and one rotation. Never throws. Returns true if a usable
 * session remains afterwards.
 */
function jwtExpiryMs(token: string): number | null {
  try {
    const payload = token.split('.')[1];
    if (!payload) return null;
    const b64 = payload.replace(/-/g, '+').replace(/_/g, '/');
    const json = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4));
    const claims = JSON.parse(json);
    return typeof claims?.exp === 'number' ? claims.exp * 1000 : null;
  } catch {
    return null;
  }
}

const EXPIRY_SKEW_MS = 120000;

export async function ensureFreshSession(): Promise<boolean> {
  if (typeof window === 'undefined') return false;
  try {
    if (!localStorage.getItem('refresh_token')) return false;
    const access = getAccessToken();
    if (access) {
      const exp = jwtExpiryMs(access);
      if (exp != null && exp - Date.now() > EXPIRY_SKEW_MS) return true;
    }
    await refreshAccessToken();
    return true;
  } catch (err) {
    if (err instanceof SessionExpiredError) {
      clearTokens();
      return false;
    }
    return true; // transient — tokens intact, session not over
  }
}

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
}

export interface LineupEntry {
  id: string;
  kind: 'performer' | 'team';
  name: string;
  image_url?: string;
  subtitle?: string;
  stage?: string;
  // RFC3339. A full timestamp rather than a clock string because festivals
  // cross midnight — render the time part against the event's own date.
  set_time?: string;
  external_url?: string;
  sort_order: number;
}

export interface EventInfoItem {
  key: string;
  label: string;
  value: string;
  // 'text' | 'enum' | 'time'. Present so a client can treat typed values
  // differently from prose without hardcoding which keys are which.
  value_type?: string;
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
  // Organiser-supplied event information (parking, accessibility, age limit and
  // so on). Each item is one row the organiser chose to fill in — a field they
  // skipped has NO entry, so absence is meaningful and must not be rendered as
  // an empty value. Keys and labels come from the server-side catalogue, which
  // is why label ships with every item rather than being mapped client-side.
  info_items?: EventInfoItem[];
  // Structured line-up: names with artwork, stage and set time. The legacy
  // teams/performers arrays above are still written by the backend and are the
  // fallback for events created before this existed — not dead code.
  lineup?: LineupEntry[];
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
  /**
   * Server-computed, from models.TicketType.ComputeAvailable().
   * -1 is the UnlimitedTickets sentinel (no capacity set).
   *
   * `available` (without the suffix) was declared here previously and is NOT a
   * field the API has ever sent, so every read of it was undefined. Do not
   * reintroduce it — use these two.
   */
  available_tickets?: number;
  is_sold_out?: boolean;
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

// Sparse patch: only send fields that actually changed. current_password is
// only required by the server when email is present — see
// events-db/internal/handlers/user_profile_handler.go.
export interface UpdateProfileDTO {
  first_name?: string;
  last_name?: string;
  phone_number?: string;
  email?: string;
  current_password?: string;
  // Social accounts have no password — a fresh provider token is the
  // equivalent proof. See events-db/internal/handlers/reauth.go.
  provider?: 'google' | 'apple';
  id_token?: string;
  nonce?: string;
}

export interface UpdateProfileResponse {
  user: AuthUser;
  // True when the email address changed — the client should tell the user
  // to go confirm the new address.
  email_changed: boolean;
}

export const updateMyProfile = (dto: UpdateProfileDTO) =>
  api.patch<UpdateProfileResponse>('/v1/user/me', dto);

// ── Event endpoints ────────────────────────────────────────────────────────────
export const getHomeSections = (city?: string) =>
  api.get<HomeResponse>('/v1/home', city ? { params: { city } } : undefined);

export const getCategories = () =>
  api.get<{ categories: Category[] }>('/v1/categories');

export const getTrendingEvents = (limit = 50) =>
  api.get<{ data: Event[] }>('/v1/events/trending', { params: { limit } });

export const getActiveEvents = (limit = 50) =>
  api.get<{ data: Event[] }>('/v1/events/active', { params: { limit } });

export const getEventsByCategory = (category: string | number, limit = 50) =>
  api.get<{ data: Event[] }>('/v1/events', { params: { category, limit } });

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

export type SuggestionKey = 'popular' | 'just_announced' | 'featured';

export const getSuggestionEvents = async (key: SuggestionKey): Promise<Event[]> => {
  try {
    // 'featured' has its own endpoint. Without this branch it fell through to
    // the generic /v1/events list, which would have returned ordinary results
    // under an "Editor's Picks" heading.
    const endpoint =
      key === 'popular'  ? '/v1/events/trending' :
      key === 'featured' ? '/v1/events/featured' :
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
  if (item.fulfillment_method === 'delivery') parts.push('Delivery');
  else if (item.fulfillment_method === 'pickup') {
    parts.push(item.pickup_location ? `Pickup · ${item.pickup_location}` : 'Pickup');
  }
  return parts.join(' · ') || null;
}

// This is a plain string helper — it can't embed a React icon component, so
// the fulfillment glyph that used to live inline in cartItemSubtitle above
// (📦 / 🏪) now comes from here instead. Call sites render the matching
// Phosphor icon (Package / Storefront) next to the subtitle text.
export function cartItemFulfillmentIcon(item: CartItem): 'delivery' | 'pickup' | null {
  if (item.item_type !== 'merchandise') return null;
  if (item.fulfillment_method === 'delivery') return 'delivery';
  if (item.fulfillment_method === 'pickup') return 'pickup';
  return null;
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
  const res = await api.get<PaymentStatusResponse>(`/v1/bookings/${bookingId}/payment`);
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
  // channel=web tags the payment so PAYMENT_CALLBACK_URL comes back as the
  // self-closing web page. Without it the return 302s to eventsapp://, which
  // mobile Safari rejects as an invalid address. web-desktop already sends it.
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


// ─── Organiser ────────────────────────────────────────────────────────────────
// Public organiser profile. Same endpoints the mobile app uses — nothing here
// is new server-side, the web simply never called them.

export interface OrganizerLink {
  platform: 'instagram' | 'x' | 'facebook' | 'tiktok' | 'youtube'
          | 'linkedin' | 'whatsapp' | 'spotify' | 'website';
  url: string;
  sort_order: number;
}

export interface PublicOrganizer {
  id: string;
  slug: string;
  business_name: string;
  description?: string;
  logo_url?: string;
  cover_image_url?: string;
  verified: boolean;
  member_since: number;
  events_hosted: number;
  follower_count: number;
  /**
   * ABSENT when the request carried no valid token — not `false`. The server
   * omits the key for a logged-out visitor on purpose, so the button can show
   * an unauthenticated state rather than claiming "you are not following",
   * which is a different thing. Compare with === true, never truthiness.
   */
  is_following?: boolean;
  links?: OrganizerLink[];
}

export interface OrganizerFollowResponse {
  following: boolean;
  follower_count: number;
  organizer_id: string;
  organizer_slug: string;
}

/** Slug or UUID — the route accepts either, matching the event detail route. */
export const getOrganizer = (slugOrId: string) =>
  api.get<PublicOrganizer>(`/v1/organisers/${slugOrId}`);

export const getOrganizerEvents = (
  slugOrId: string,
  scope: 'upcoming' | 'past' = 'upcoming',
  cursor?: string,
) =>
  api.get<{ data: Event[]; scope: string; next_cursor?: string }>(
    `/v1/organisers/${slugOrId}/events?scope=${scope}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`
  );

export async function toggleOrganizerFollow(slugOrId: string): Promise<OrganizerFollowResponse> {
  const res = await api.post<OrganizerFollowResponse>(`/v1/organisers/${slugOrId}/follow`);
  return res.data;
}

/** Followed organisers, for the profile Following screen. */
export const getFollowedOrganizers = async (): Promise<PublicOrganizer[]> => {
  const { data } = await api.get<{ data: PublicOrganizer[] }>('/v1/user/following-organisers');
  return data.data ?? [];
};