import axios from 'axios';
import * as SecureStore from 'expo-secure-store';
import Constants from 'expo-constants';
import { fmtRand, fmtPrice } from './money';

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
  // 10s was too tight for SA mobile data. A timeout is indistinguishable from
  // a failure to the code above it, and the old refresh path treated a failure
  // as "your session is invalid" — so a slow tunnel logged people out.
  timeout: 20000,
  headers: { 'Content-Type': 'application/json' },
});

// ─── Token store ────────────────────────────────────────────────────────────
// SecureStore is the durable home for the tokens, but it is NOT read on every
// request any more. Two reasons:
//
//   1. Speed. A Keychain/Keystore round-trip per request is measurable, and the
//      app fires several in parallel on most screens.
//   2. Correctness. On iOS a Keychain item written with the default
//      accessibility (kSecAttrAccessibleWhenUnlocked) cannot be read or written
//      while the device is locked. Anything that touched the API from the
//      background — a push arriving, a foreground refresh racing the lock
//      screen — got `null` back and looked exactly like "signed out". Worse, a
//      failed WRITE during token rotation threw away the new refresh token
//      while the server had already revoked the old one, which is a guaranteed
//      logout on the next call.
//
// So: memory is the working copy, SecureStore is the backing store, writes are
// retried, and the Keychain item is created with AFTER_FIRST_UNLOCK so it stays
// readable whenever the device has been unlocked once since boot.
const ACCESS_KEY  = 'access_token';
const REFRESH_KEY = 'refresh_token';

const SECURE_OPTS: SecureStore.SecureStoreOptions = {
  keychainAccessible: SecureStore.AFTER_FIRST_UNLOCK,
};

let memAccess:  string | null | undefined = undefined; // undefined = not yet loaded
let memRefresh: string | null | undefined = undefined;

// Bumped every time a new access token lands. A request that was already in
// flight when a refresh completed will come back 401 through no fault of the
// session; comparing generations tells us to simply retry it with the new
// token instead of kicking off a second (token-rotating) refresh.
let tokenGeneration = 0;

async function secureGet(key: string): Promise<string | null> {
  try {
    return await SecureStore.getItemAsync(key, SECURE_OPTS);
  } catch {
    // Fall back to the default accessibility in case the item predates
    // SECURE_OPTS — a read failure must never be read as "signed out".
    try { return await SecureStore.getItemAsync(key); } catch { return null; }
  }
}

async function secureSet(key: string, value: string): Promise<boolean> {
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      await SecureStore.setItemAsync(key, value, SECURE_OPTS);
      return true;
    } catch {
      await new Promise((r) => setTimeout(r, 150 * (attempt + 1)));
    }
  }
  return false;
}

async function secureDelete(key: string): Promise<void> {
  try { await SecureStore.deleteItemAsync(key, SECURE_OPTS); } catch { /* ignore */ }
  try { await SecureStore.deleteItemAsync(key); } catch { /* ignore */ }
}

export async function getAccessToken(): Promise<string | null> {
  if (memAccess === undefined) memAccess = await secureGet(ACCESS_KEY);
  return memAccess;
}

export async function getRefreshToken(): Promise<string | null> {
  if (memRefresh === undefined) memRefresh = await secureGet(REFRESH_KEY);
  return memRefresh;
}

/**
 * Commits a freshly-issued token pair.
 *
 * Memory is updated FIRST and synchronously. If the Keychain write then fails,
 * the session still works for the rest of this app run (and the write is
 * retried on the next rotation) instead of being destroyed — which is what used
 * to happen, because the server had already revoked the token we just dropped.
 */
export async function saveTokens(access: string, refresh: string): Promise<void> {
  memAccess  = access;
  memRefresh = refresh;
  tokenGeneration++;
  // Refresh token first: it is the one that cannot be re-derived.
  await secureSet(REFRESH_KEY, refresh);
  await secureSet(ACCESS_KEY, access);
}

export async function clearTokens(): Promise<void> {
  memAccess  = null;
  memRefresh = null;
  tokenGeneration++;
  await secureDelete(ACCESS_KEY);
  await secureDelete(REFRESH_KEY);
}

// ─── Session-ended notification ─────────────────────────────────────────────
// Only ever fired for a DEFINITIVE rejection (see below). authStore subscribes
// so the UI drops to signed-out in the one case where that is genuinely true.
type SessionEndedListener = () => void;
const sessionEndedListeners = new Set<SessionEndedListener>();

export function onSessionEnded(fn: SessionEndedListener): () => void {
  sessionEndedListeners.add(fn);
  return () => { sessionEndedListeners.delete(fn); };
}

async function endSession(): Promise<void> {
  await clearTokens();
  sessionEndedListeners.forEach((fn) => { try { fn(); } catch { /* ignore */ } });
}

// ─── Refresh ────────────────────────────────────────────────────────────────
//
// THE RULE THIS FILE EXISTS TO ENFORCE: a session is ended by the SERVER
// saying so, or by the user tapping Log out. Nothing else.
//
// The previous version cleared the tokens in a bare `catch` around the refresh
// call, so every one of these ended with the user signed out:
//   • request timed out on a slow connection
//   • phone was offline / switching between wifi and LTE
//   • Cloudflare Tunnel returned 502 for a few seconds
//   • a Keychain write failed
// None of those mean the session is invalid. They are now retried, and if they
// still fail the tokens are left exactly where they are so the very next
// request can succeed.

/** Thrown when the server has definitively rejected the refresh token. */
class SessionExpiredError extends Error {
  constructor(msg = 'session expired') { super(msg); this.name = 'SessionExpiredError'; }
}

// A refresh gets a longer budget than a normal request: it is the one call we
// really do not want to give up on.
const REFRESH_TIMEOUT_MS  = 30000;
const REFRESH_MAX_ATTEMPTS = 4;

/**
 * A response that means "this refresh token will never work again".
 *
 * 401/403 — the server looked it up and rejected it (expired, revoked, reused).
 * 400     — malformed body; retrying sends the identical body, so it is final.
 *
 * Everything else (no response at all, 408, 429, 5xx, 502/503/504 from the
 * tunnel) is transient by definition and must not cost the user their session.
 */
function isDefinitiveAuthRejection(err: any): boolean {
  const status = err?.response?.status;
  return status === 400 || status === 401 || status === 403;
}

let refreshInFlight: Promise<string> | null = null;

async function doRefresh(): Promise<string> {
  const refresh = await getRefreshToken();
  if (!refresh) throw new SessionExpiredError('no refresh token');

  let lastErr: any = null;

  for (let attempt = 0; attempt < REFRESH_MAX_ATTEMPTS; attempt++) {
    try {
      const { data } = await axios.post(
        `${API_BASE_URL}/v1/auth/refresh`,
        { refresh_token: refresh },
        { timeout: REFRESH_TIMEOUT_MS, headers: { 'Content-Type': 'application/json' } },
      );
      if (!data?.access_token || !data?.refresh_token) {
        throw new Error('malformed refresh response');
      }
      await saveTokens(data.access_token, data.refresh_token);
      return data.access_token as string;
    } catch (err: any) {
      lastErr = err;
      if (isDefinitiveAuthRejection(err)) {
        throw new SessionExpiredError('refresh token rejected by server');
      }
      // Transient. Back off and present the SAME token again — the server's
      // rotation grace window exists precisely so that a retry after a lost
      // response is treated as a race, not as a replay.
      if (attempt < REFRESH_MAX_ATTEMPTS - 1) {
        await new Promise((r) => setTimeout(r, 600 * Math.pow(2, attempt)));
      }
    }
  }

  // Out of attempts, still nothing definitive. Keep the tokens.
  throw lastErr ?? new Error('refresh failed');
}

function refreshAccessToken(): Promise<string> {
  if (!refreshInFlight) {
    refreshInFlight = doRefresh().finally(() => { refreshInFlight = null; });
  }
  return refreshInFlight;
}

// ─── Proactive refresh ──────────────────────────────────────────────────────
// Access tokens live 15 minutes, so an app left open (or backgrounded and
// reopened) is nearly always holding an expired one. Renewing it BEFORE it is
// used turns the common case into a single quiet call instead of a burst of
// 401s from whatever screen happened to mount first.

// Minimal base64 decoder. Hermes exposes a global atob on current RN versions,
// but not on every engine/config this app can be built with, and Buffer is a
// Node global that only exists in React Native if something has polyfilled it.
// Neither is worth a hard dependency for reading one integer claim, and a
// throw here would surface as a spurious "session expired".
const B64_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

function decodeBase64(input: string): string {
  if (typeof atob === 'function') return atob(input);
  const clean = input.replace(/=+$/, '');
  let out = '';
  let bits = 0;
  let acc = 0;
  for (let i = 0; i < clean.length; i++) {
    const idx = B64_ALPHABET.indexOf(clean[i]);
    if (idx === -1) continue;
    acc = (acc << 6) | idx;
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      out += String.fromCharCode((acc >> bits) & 0xff);
    }
  }
  return out;
}

function jwtExpiryMs(token: string): number | null {
  try {
    const payload = token.split('.')[1];
    if (!payload) return null;
    const b64 = payload.replace(/-/g, '+').replace(/_/g, '/');
    const padded = b64 + '='.repeat((4 - (b64.length % 4)) % 4);
    const claims = JSON.parse(decodeBase64(padded));
    return typeof claims?.exp === 'number' ? claims.exp * 1000 : null;
  } catch {
    return null;
  }
}

const EXPIRY_SKEW_MS = 120000; // renew when under 2 minutes remain

/**
 * Renews the access token if it is expired or close to it. Safe to call often
 * and from anywhere — it shares the single-flight promise, so ten callers cause
 * at most one network call and one token rotation.
 *
 * Never throws and never signs anyone out. Returns true if there is a usable
 * session afterwards.
 */
export async function ensureFreshSession(): Promise<boolean> {
  try {
    const refresh = await getRefreshToken();
    if (!refresh) return false;

    const access = await getAccessToken();
    if (access) {
      const exp = jwtExpiryMs(access);
      if (exp != null && exp - Date.now() > EXPIRY_SKEW_MS) return true;
    }

    await refreshAccessToken();
    return true;
  } catch (err) {
    if (err instanceof SessionExpiredError) {
      await endSession();
      return false;
    }
    // Transient — we still have tokens, so the session is not over.
    return true;
  }
}

// ─── Interceptors ───────────────────────────────────────────────────────────

api.interceptors.request.use(async (config) => {
  const token = await getAccessToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  (config as any)._tokenGen = tokenGeneration;
  return config;
});

const MAX_AUTH_RETRIES = 2;

api.interceptors.response.use(
  (res) => res,
  async (error) => {
    const original: any = error.config;

    if (error.response?.status !== 401 || !original) throw error;

    original._authRetries = (original._authRetries ?? 0) + 1;
    if (original._authRetries > MAX_AUTH_RETRIES) throw error;

    // Raced a refresh that has already finished — the token in the header was
    // simply stale by the time the server saw it. Retry, don't rotate.
    if (original._tokenGen !== undefined && original._tokenGen !== tokenGeneration) {
      original._tokenGen = tokenGeneration;
      const current = await getAccessToken();
      if (current) {
        original.headers = original.headers ?? {};
        original.headers.Authorization = `Bearer ${current}`;
        return api(original);
      }
    }

    try {
      const newToken = await refreshAccessToken();
      original.headers = original.headers ?? {};
      original.headers.Authorization = `Bearer ${newToken}`;
      return api(original);
    } catch (refreshErr) {
      if (refreshErr instanceof SessionExpiredError) {
        // The ONLY path that signs someone out.
        await endSession();
      }
      // Transient failure: tokens are untouched, the user stays signed in, and
      // the caller gets the original error to handle (screens fall back to
      // their offline cache).
      throw error;
    }
  },
);

// ── Session ID cache ───────────────────────────────────────────────────────
let _cachedSessionId: string | undefined = undefined;

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
  max_tickets_per_user?: number | null;
  venue_type?: 'stadium' | 'auditorium' | 'none' | null;
  organizer?:  PublicOrganizer;
  info_items?: EventInfoItem[];
  lineup?:     LineupEntry[];
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

export const getHomeSections = (city?: string) =>
  api.get<HomeResponse>('/v1/home', city ? { params: { city } } : undefined);

export async function getEventsByIds(ids: string[]): Promise<Event[]> {
  if (!ids.length) return [];
  const res = await api.get<{ data: Event[] }>('/v1/events/by-ids', {
    params: { ids: ids.join(',') },
  });
  return res.data.data ?? [];
}

export const getFeaturedEvents   = (limit = 1)                      => api.get<EventPage>(`/v1/events/featured?limit=${limit}`);
export const getTrendingEvents   = (limit = 10)                     => api.get<EventPage>(`/v1/events/trending?limit=${limit}`);
export const getActiveEvents     = (limit = 10)                     => api.get<EventPage>(`/v1/events/active?limit=${limit}`);
export const getEventsByCategory = (categoryId: number, limit = 5)  => api.get<EventPage>(`/v1/events/category/${categoryId}?limit=${limit}`);
export const filterEvents        = (params: Record<string, string>) => api.get<EventPage>(`/v1/events/filter`, { params });

const SECTION_ENDPOINT: Record<string, string> = {
  trending:        '/v1/events/trending',
  featured:        '/v1/events/featured',
  happening_soon:  '/v1/events/active',
  this_weekend:    '/v1/events/active',
  recently_added:  '/v1/events',
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
  collection_cover_image_url?: string;
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
  fulfillment_mode?:  'delivery' | 'pickup' | 'both';
  pickup_location?:   string | null;
  delivery_fee?:      number;
}

export interface DropsResponse {
  available: MerchandiseItem[];
  upcoming:  MerchandiseItem[];
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
}

// ── Auth API calls ─────────────────────────────────────────────────────────

export const authLogin    = (email: string, password: string) =>
  api.post<TokenResponse>('/v1/auth/login', { email, password });

export const authRegister = (dto: RegisterDTO) =>
  api.post('/v1/auth/register', dto);

// Send our refresh token so the server revokes ONLY this device's session —
// logging out of the app no longer logs you out of the web (and vice versa).
// Older backend builds ignore the body, so this is backwards-compatible.
export interface SocialSignInBody {
  provider: 'google' | 'apple';
  id_token: string;
  nonce?: string;
  first_name?: string;
  last_name?: string;
}

export interface SocialSignInResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  created: boolean;   // brand-new account provisioned
  linked: boolean;    // provider attached to an existing password account
  needs_info: boolean; // phone / DOB still blank — checkout will ask
  user: { id: string; email: string; first_name?: string; last_name?: string; role: string };
}

/**
 * Exchanges a provider ID token for a TicketHawk session.
 *
 * One endpoint covers signup AND login: only the server can know which it is,
 * because only the server can see whether that provider identity (or email)
 * already maps to an account.
 */
export const authSocial = (body: SocialSignInBody) =>
  api.post<SocialSignInResponse>('/v1/auth/social', body);

export const getLinkedIdentities = () =>
  api.get<{ data: Array<{ provider: string; email?: string; created_at: string; last_login_at?: string }> }>(
    '/v1/auth/identities',
  );

export const unlinkIdentity = (provider: 'google' | 'apple') =>
  api.delete(`/v1/auth/identities/${provider}`);

export const authLogout = async () =>
  api.post('/v1/auth/logout', {
    refresh_token: (await getRefreshToken()) ?? '',
  });

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
  available_tickets?: number;
  is_sold_out?: boolean;
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
  fulfillment_method: 'pickup' | 'delivery';
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

export async function updateCartItemAddress(
  itemId: string,
  address: { street: string; suburb?: string; city: string; province: string; postal_code?: string },
  sessionId?: string | null,
): Promise<void> {
  const headers = makeSessionHeaders(sessionId ?? null);
  await api.patch(`/v1/cart/items/${itemId}`, { delivery_address: address }, { headers });
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

// Delegates to lib/money. This was `Math.round(...)`, which rounds to nearest
// rather than truncating — a R3 ticket plus its 60c service fee printed as
// "R4", quoting the buyer 40c more than the payment sheet then charged. Kept
// as a named export so every existing import site is untouched.
export function fmtZAR(amount: number): string {
  return fmtRand(amount);
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

// ── Booking detail & ticket assignments ───────────────────────────────────

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

// ── Rotating QR ────────────────────────────────────────────────────────────

export interface RotatingQRToken {
  token:      string;
  expires_at: string;   // ISO-8601
  window_sec: number;   // 30 — rotation window in seconds
  issued_at:  string;   // ISO-8601
}

/**
 * Fetches a fresh rotating HMAC QR token for a claimed ticket assignment.
 * The token rotates every `window_sec` seconds and is accepted by
 * POST /v1/tickets/:assignmentId/scan on the organiser scanner.
 */
export async function getRotatingQRToken(assignmentId: string): Promise<RotatingQRToken> {
  const res = await api.get<RotatingQRToken>(`/v1/tickets/${assignmentId}/qr`);
  return res.data;
}

// ── Event Follow ───────────────────────────────────────────────────────────

export interface FollowedEvent {
  follow_id:        string;
  followed_at:      string;
  id:               string;
  slug:             string;
  title:            string;
  cover_image_url?: string | null;
  start_time:       string;
  end_time:         string;
  venue_name:       string;
  city:             string;
  price:            number;
  currency:         string;
  status:           string;
  category:         string;
  tickets_sold:     number;
  max_capacity?:    number | null;
  is_sold_out:      boolean;
  available_tickets: number;
}

export interface FollowingResponse {
  data:        FollowedEvent[];
  next_cursor: string | null;
}

export interface FollowStatusResponse {
  following:    boolean;
  follow_count: number;
  event_id:     string;
}

export function fmtFollowDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-ZA', {
    weekday: 'short',
    day:     'numeric',
    month:   'short',
  });
}

export function fmtFollowPrice(price: number): string {
  return fmtPrice(price);
}

export function followAvailPct(
  sold: number,
  max: number | null | undefined,
): number | null {
  if (!max) return null;
  return Math.round((sold / max) * 100);
}

export async function toggleFollow(eventId: string): Promise<FollowStatusResponse> {
  const res = await api.post<FollowStatusResponse>(`/v1/events/${eventId}/follow`);
  return res.data;
}

export async function getFollowStatus(eventId: string): Promise<FollowStatusResponse> {
  const res = await api.get<FollowStatusResponse>(`/v1/events/${eventId}/follow`);
  return res.data;
}

// ═══════════════════════════════════════════════════════════════════════════
// APPEND TO: EventsApp/lib/api.ts
//
// Everything below is additive. Two edits are needed to the EXISTING `Event`
// interface — marked at the bottom of this file.
// ═══════════════════════════════════════════════════════════════════════════


// ─── Organiser ────────────────────────────────────────────────────────────────

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
   * Absent when the request carried no valid token — NOT `false`.
   *
   * The server omits the key for a logged-out visitor on purpose, so the
   * follow button can render an unauthenticated state rather than claiming
   * "you are not following", which is a different thing.
   */
  is_following?: boolean;
  links?: OrganizerLink[];
}

export interface OrganizerFollowResponse {
  following:      boolean;
  follower_count: number;
  organizer_id:   string;
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

export async function getOrganizerFollowStatus(slugOrId: string) {
  const res = await api.get<{ following: boolean; follower_count: number; organizer_id: string }>(
    `/v1/organisers/${slugOrId}/follow`
  );
  return res.data;
}


// ─── Event information ────────────────────────────────────────────────────────

export interface EventInfoItem {
  key: string;
  label: string;
  value: string;
  value_type: 'text' | 'enum' | 'time';
  sort_order: number;
}


// ─── Line-up ──────────────────────────────────────────────────────────────────

export interface LineupEntry {
  id: string;
  kind: 'performer' | 'team';
  name: string;
  image_url?: string;
  subtitle?: string;
  stage?: string;
  set_time?: string;
  external_url?: string;
  sort_order: number;
}

export async function searchOrganizers(query: string, limit = 8): Promise<PublicOrganizer[]> {
  const q = query.trim();
  if (q.length < 2) return [];
  try {
    const res = await api.get<{ data: PublicOrganizer[]; count: number }>(
      '/v1/organisers/search',
      { params: { q, limit } },
    );
    return res.data?.data ?? [];
  } catch {
    return [];
  }
}

export interface FollowedOrganizer {
  id: string;
  slug: string;
  business_name: string;
  logo_url?: string;
  verified: boolean;
  followed_at: string;
  upcoming_events: number;
}