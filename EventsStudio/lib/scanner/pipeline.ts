// ─────────────────────────────────────────────────────────────────────────────
// Scan pipeline — entirely local, sub-10ms, never touches the network.
//
// Ticket validity offline is MANIFEST-authoritative: a token is valid iff its
// assignment (or booking, for pools) appears in the synced manifest with an
// admissible state. The device never verifies HMAC signatures — that would
// require the master secret on staff phones. Offline "forgery" would need a
// valid assignment UUID, obtainable only from a real ticket's QR, which makes
// it identical to replay — and replay is what the dedupe layer handles.
// ─────────────────────────────────────────────────────────────────────────────
import {
  findAssignment, findPool, admitIndividual, admitPool as dbAdmitPool,
  admitProvisional, findTag,
} from './db';
import { uuid } from './sync';
import { Buffer } from 'buffer';

// Must match QRWindowSeconds in rotating_ar_service.go.
const QR_WINDOW_SECONDS = 30;

// How stale a ROTATING token may be and still scan at an offline gate, in
// windows. 20 x 30s = 10 minutes, in either direction — a future-dated window
// means clock skew or a crafted payload and is refused the same way.
//
// The window used to be ignored offline entirely, which meant a screenshot of
// a live QR taken at any point earlier admitted successfully. The server does
// not check the window on ingest either — IngestScans resolves the assignment
// id and admits if unscanned — so nothing downstream caught it. The 30-second
// rotation bought nothing the moment a gate went offline.
//
// It is still not enforced to the second, because a legitimate holder may be
// showing a code a little older than the server itself would accept. But they
// should not be showing a ROTATING code at all with no signal: RotatingQR.tsx
// falls back to the self-contained OFFLINE token as soon as a refresh fails. A
// rotating code ten minutes stale is a replay, not a straggler.
const ROTATING_STALE_WINDOWS = 20;

// Token payload shape shared by rotating, pool, and offline token families
// (see rotating_ar_service.go). Base64url JSON before the last '.'.
interface TokenPayload {
  t?: string;    // 'p' = pool
  aid?: string;  // assignment id (individual)
  bid?: string;  // booking id
  eid?: string;  // event id
  sn?: number;   // slot
  w?: number;    // rotating window
  off?: boolean; // offline token
  exp?: number;  // absolute expiry (offline tokens)
  v?: number;
}

export type ScanOutcome =
  | { kind: 'admitted'; name: string; tier: string; slot: number }
  | { kind: 'duplicate'; name: string; firstScanAt: string | null }
  | { kind: 'invalid'; reason: string }
  // `tier` so the door can see GA vs VIP BEFORE admitting a group, not only
  // after. Comes from the manifest, so it is available offline too.
  | { kind: 'pool_prompt'; bookingId: string; buyer: string; bookingNumber: string; remaining: number; tier: string }
  | { kind: 'unknown_prompt'; assignmentId: string | null; bookingId: string | null; isPool: boolean };

export function decodeToken(raw: string): TokenPayload | null {
  try {
    const dot = raw.lastIndexOf('.');
    const b64 = dot > 0 ? raw.slice(0, dot) : raw;
    // base64url → base64 with padding
    let s = b64.replace(/-/g, '+').replace(/_/g, '/');
    while (s.length % 4) s += '=';
    const json = typeof atob === 'function'
      ? atob(s)
      : Buffer.from(s, 'base64').toString('utf8');
    const p = JSON.parse(json);
    return typeof p === 'object' && p ? (p as TokenPayload) : null;
  } catch {
    return null;
  }
}

/** Process one decoded QR against the local manifest. Pool tokens return a
 *  prompt (the gate picks a headcount); unknown refs return a prompt too
 *  (manager-confirmable provisional). Everything else resolves immediately. */
export async function processScan(eventId: string, raw: string): Promise<ScanOutcome> {
  const p = decodeToken(raw);
  if (!p || (!p.aid && !p.bid)) {
    return { kind: 'invalid', reason: 'Not a TicketHawk ticket' };
  }
  if (p.eid && p.eid !== eventId) {
    return { kind: 'invalid', reason: 'Ticket is for a different event' };
  }
  // Offline tokens carry an absolute expiry; honour it.
  if (p.off && p.exp && Date.now() / 1000 > p.exp + 300) {
    return { kind: 'invalid', reason: 'Ticket expired' };
  }
  // Rotating tokens carry a 30s window. See ROTATING_STALE_WINDOWS above for
  // why this is checked loosely rather than not at all.
  if (!p.off && typeof p.w === 'number') {
    const currentWindow = Math.floor(Date.now() / 1000 / QR_WINDOW_SECONDS);
    if (Math.abs(currentWindow - p.w) > ROTATING_STALE_WINDOWS) {
      return { kind: 'invalid', reason: 'QR is out of date — ask them to refresh their ticket' };
    }
  }

  // ── Pool pass ───────────────────────────────────────────────────────────────
  if (p.t === 'p') {
    const pool = p.bid ? await findPool(p.bid) : null;
    if (!pool) {
      return { kind: 'unknown_prompt', assignmentId: null, bookingId: p.bid ?? null, isPool: true };
    }
    if (!pool.booking_ok) {
      return { kind: 'invalid', reason: 'Booking was cancelled or refunded' };
    }
    const remaining = pool.pool_total - pool.pool_admitted;
    if (remaining <= 0) {
      return { kind: 'duplicate', name: pool.buyer, firstScanAt: null };
    }
    return {
      kind: 'pool_prompt',
      bookingId: pool.booking_id,
      buyer: pool.buyer,
      tier: pool.tier || 'General Admission',
      bookingNumber: pool.booking_number,
      remaining,
    };
  }

  // ── Individual ticket ───────────────────────────────────────────────────────
  const a = p.aid ? await findAssignment(p.aid) : null;
  if (!a) {
    return { kind: 'unknown_prompt', assignmentId: p.aid ?? null, bookingId: p.bid ?? null, isPool: false };
  }
  if (!a.booking_ok) {
    return { kind: 'invalid', reason: 'Booking was cancelled or refunded' };
  }
  if (a.scanned) {
    return { kind: 'duplicate', name: a.name || `Slot ${a.slot}`, firstScanAt: a.scanned_at };
  }

  await admitIndividual(uuid(), eventId, a);
  return { kind: 'admitted', name: a.name || `Slot ${a.slot}`, tier: a.tier, slot: a.slot };
}

/** Gate picked a headcount for a pool pass. */
export async function confirmPoolAdmit(
  eventId: string, bookingId: string, count: number,
): Promise<void> {
  await dbAdmitPool(uuid(), eventId, bookingId, count);
}

// ─────────────────────────────────────────────────────────────────────────────
// NFC tap — offline path.
//
// Same manifest-authoritative rule as QR above, one step longer: resolve the
// band's UID to an assignment through the synced tag table, then run the exact
// checks processScan already runs. Sharing the outcome type is deliberate —
// the screen must not need to know which medium admitted somebody, or the two
// paths will drift and door staff will get different answers depending on
// whether they tapped or scanned.
//
// The device does NOT verify the band's cryptogram here, and cannot: that would
// need the event's tag key on the handset, and possessing a verification key
// for symmetric crypto means possessing a forgery key. Offline, a cloned band
// therefore gets through this gate. What contains it is that the server
// re-checks the mapping at sync (ClassifyTagScan) and refuses anything revoked
// or re-bound since this handset last synced — so a clone cannot survive
// reconciliation, and lands in the organiser's exceptions report.
//
// Online the screen should prefer tapOnline() below, which forwards the SDM
// cryptogram for real server-side verification.
// ─────────────────────────────────────────────────────────────────────────────

export async function processTap(eventId: string, uid: string): Promise<ScanOutcome> {
  const tag = await findTag(uid);

  if (!tag) {
    // Not in the manifest. Could be a band encoded after our last sync, or a
    // band from another event, or junk. The device cannot tell — same position
    // as an unknown QR ref, so the same manager-confirmable prompt. The server
    // resolves it as a provisional at sync.
    return { kind: 'unknown_prompt', assignmentId: null, bookingId: null, isPool: false };
  }

  if (tag.status !== 'active') {
    // Revoked bands are shipped in the manifest rather than filtered out
    // precisely so this branch can exist: the gate says WHY, instead of
    // treating a band reported lost ten minutes ago as an unknown one.
    return { kind: 'invalid', reason: reasonForStatus(tag.status) };
  }

  if (!tag.assignment_id) {
    // Pool-bound or unbound. Refused offline for the same reason TapTagHandler
    // refuses it online — one band admitting several people is not a product
    // that has been designed, and the two paths must agree.
    return { kind: 'invalid', reason: 'This band is not linked to a ticket' };
  }

  const a = await findAssignment(tag.assignment_id);
  if (!a) {
    return { kind: 'unknown_prompt', assignmentId: tag.assignment_id, bookingId: null, isPool: false };
  }
  if (!a.booking_ok) {
    return { kind: 'invalid', reason: 'Booking was cancelled or refunded' };
  }
  if (a.scanned) {
    // Covers a band tapped twice AND a ticket already admitted by QR — both
    // land on the same assignment row, which is what makes the two media share
    // one single-entry guard instead of each having their own.
    return { kind: 'duplicate', name: a.name || `Slot ${a.slot}`, firstScanAt: a.scanned_at };
  }

  await admitIndividual(uuid(), eventId, a, 'nfc', uid);
  return { kind: 'admitted', name: a.name || `Slot ${a.slot}`, tier: a.tier, slot: a.slot };
}

function reasonForStatus(status: string): string {
  switch (status) {
    case 'lost':        return 'This band was reported lost — do not admit';
    case 'deactivated': return 'This band has been deactivated';
    case 'destroyed':   return 'This band was written off';
    case 'provisioned':
    case 'encoded':     return 'This band was never issued to anyone';
    default:            return 'This band is not active';
  }
}

/** Manager confirmed a provisional from a TAP rather than a QR, so the tag UID
 *  is carried through to the ledger. Without it the exceptions report shows an
 *  unexplained provisional with nothing to investigate. */
export async function confirmTapProvisional(
  eventId: string, uid: string, assignmentId: string | null,
): Promise<void> {
  await admitProvisional(uuid(), eventId, 'individual', assignmentId, null, 'nfc', uid);
}

/** Manager confirmed a provisional (unknown ref — likely sold after last sync).
 *  Logged and reconciled server-side; unresolved ones surface in exceptions. */
export async function confirmProvisional(
  eventId: string, assignmentId: string | null, bookingId: string | null, isPool: boolean,
): Promise<void> {
  await admitProvisional(uuid(), eventId, isPool ? 'pool' : 'individual', assignmentId, bookingId);
}