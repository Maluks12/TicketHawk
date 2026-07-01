// ─────────────────────────────────────────────────────────────────────────────
// Offline tickets — local cache so a ticket works with no signal.
//
// Two things are cached:
//   1. The bookings LIST, so the Tickets tab still renders when offline.
//   2. Per-booking OFFLINE TOKEN BUNDLES — self-contained signed tokens the
//      RotatingQR component falls back to when it can't reach the network.
//
// The tokens are minted server-side (GET /v1/bookings/:id/offline and the guest
// equivalent) and are the offline analogue of the live rotating QR: static,
// bound to the event's admission window, verified by the same scanner path.
//
// Storage uses AsyncStorage (plain JSON, survives restarts). The tokens are not
// account credentials — they are the ticket itself, like a boarding pass — so
// they don't need Keychain-grade secrecy, and AsyncStorage sidesteps the small
// value-size limit SecureStore imposes on multi-ticket bookings.
// ─────────────────────────────────────────────────────────────────────────────
import AsyncStorage from '@react-native-async-storage/async-storage';
import { api } from './api';

// ── Keys ──────────────────────────────────────────────────────────────────────
const K_BOOKINGS = 'th:offline:bookings'; // cached list for the Tickets tab
const K_BUNDLES = 'th:offline:bundles'; // { [bookingId]: CachedBundle }

// ── Types ─────────────────────────────────────────────────────────────────────
export interface OfflineAssignment {
  assignment_id: string;
  slot_number: number;
  token: string;
}

export interface OfflineBundle {
  booking_id: string;
  generated_at: string;
  expires_at: string;
  pool_token: string | null;
  pool_remaining: number;
  assignments: OfflineAssignment[];
}

// Cached form adds a couple of client-side fields the server doesn't send, so
// guest lookups (which key on booking number) and staleness checks work offline.
export interface CachedBundle extends OfflineBundle {
  booking_number?: string;
  cached_at: string;
}

type BundleMap = Record<string, CachedBundle>;

// ── Low-level helpers ──────────────────────────────────────────────────────────
async function readBundles(): Promise<BundleMap> {
  try {
    const raw = await AsyncStorage.getItem(K_BUNDLES);
    return raw ? (JSON.parse(raw) as BundleMap) : {};
  } catch {
    return {};
  }
}

async function writeBundles(map: BundleMap): Promise<void> {
  try {
    await AsyncStorage.setItem(K_BUNDLES, JSON.stringify(map));
  } catch {
    // Best-effort: a failed cache write should never break the online path.
  }
}

// ── Bookings list cache ────────────────────────────────────────────────────────
export async function cacheBookingsList<T>(list: T[]): Promise<void> {
  try {
    await AsyncStorage.setItem(K_BOOKINGS, JSON.stringify(list ?? []));
  } catch {
    /* best-effort */
  }
}

export async function getCachedBookingsList<T>(): Promise<T[]> {
  try {
    const raw = await AsyncStorage.getItem(K_BOOKINGS);
    return raw ? (JSON.parse(raw) as T[]) : [];
  } catch {
    return [];
  }
}

// ── Bundle sync ────────────────────────────────────────────────────────────────

interface GuestAuth {
  bookingNumber: string;
  email: string;
}

/**
 * Fetch and cache the offline token bundle for one booking. Call this right
 * after a booking is confirmed / whenever it's viewed online, so the tokens are
 * on the device before the holder ever loses signal. Silently no-ops on failure
 * (e.g. already offline) — the caller keeps working from whatever is cached.
 *
 * Authed buyer:  syncBookingOffline(bookingId)
 * Guest buyer:   syncBookingOffline(bookingId, { bookingNumber, email })
 */
export async function syncBookingOffline(
  bookingId: string,
  guest?: GuestAuth,
): Promise<CachedBundle | null> {
  try {
    const res = guest
      ? await api.get<OfflineBundle>(
          `/v1/guest/bookings/${encodeURIComponent(guest.bookingNumber)}/offline`,
          { params: { email: guest.email } },
        )
      : await api.get<OfflineBundle>(`/v1/bookings/${bookingId}/offline`);

    const bundle: CachedBundle = {
      ...res.data,
      booking_number: guest?.bookingNumber,
      cached_at: new Date().toISOString(),
    };

    // Key on the server-authoritative booking_id (guest responses include it too).
    const key = bundle.booking_id || bookingId;
    const map = await readBundles();
    map[key] = bundle;
    await writeBundles(map);
    return bundle;
  } catch {
    return null;
  }
}

// ── Offline token lookups (used by RotatingQR fallback) ─────────────────────────

function isFresh(b: CachedBundle): boolean {
  // Keep a little grace so a token that just ticked past expiry at the gate is
  // still offered — the scanner makes the final call.
  const exp = new Date(b.expires_at).getTime();
  return Number.isFinite(exp) ? Date.now() < exp + 10 * 60 * 1000 : true;
}

/** Cached offline token for a specific individual assignment, or null. */
export async function getOfflineIndividualToken(
  assignmentId: string,
): Promise<string | null> {
  const map = await readBundles();
  for (const b of Object.values(map)) {
    if (!isFresh(b)) continue;
    const hit = b.assignments?.find((a) => a.assignment_id === assignmentId);
    if (hit?.token) return hit.token;
  }
  return null;
}

/** Cached offline POOL token for a booking (by id or, for guests, by number). */
export async function getOfflinePoolToken(opts: {
  bookingId?: string;
  bookingNumber?: string;
}): Promise<string | null> {
  const map = await readBundles();

  if (opts.bookingId) {
    const b = map[opts.bookingId];
    if (b && isFresh(b) && b.pool_token) return b.pool_token;
  }
  if (opts.bookingNumber) {
    for (const b of Object.values(map)) {
      if (!isFresh(b)) continue;
      if (b.booking_number === opts.bookingNumber && b.pool_token) return b.pool_token;
    }
  }
  return null;
}

/** Best-effort: cache offline bundles for a batch of confirmed bookings. */
export async function syncManyOffline(
  bookings: Array<{ id: string; booking_status?: string; is_guest?: boolean; booking_number?: string; attendee_email?: string }>,
): Promise<void> {
  const confirmed = bookings.filter((b) => b.booking_status === 'confirmed');
  // Sequential to stay gentle on the qr-token rate-limit bucket.
  for (const b of confirmed) {
    if (b.is_guest && b.booking_number && b.attendee_email) {
      await syncBookingOffline(b.id, { bookingNumber: b.booking_number, email: b.attendee_email });
    } else if (!b.is_guest) {
      await syncBookingOffline(b.id);
    }
  }
}