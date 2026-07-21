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
  admitProvisional,
} from './db';
import { uuid } from './sync';
import { Buffer } from 'buffer';

// Token payload shape shared by rotating, pool, and offline token families
// (see rotating_ar_service.go). Base64url JSON before the last '.'.
interface TokenPayload {
  t?: string;    // 'p' = pool
  aid?: string;  // assignment id (individual)
  bid?: string;  // booking id
  eid?: string;  // event id
  sn?: number;   // slot
  w?: number;    // rotating window (ignored offline)
  off?: boolean; // offline token
  exp?: number;  // absolute expiry (offline tokens)
  v?: number;
}

export type ScanOutcome =
  | { kind: 'admitted'; name: string; tier: string; slot: number }
  | { kind: 'duplicate'; name: string; firstScanAt: string | null }
  | { kind: 'invalid'; reason: string }
  | { kind: 'pool_prompt'; bookingId: string; buyer: string; bookingNumber: string; remaining: number }
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
  // Offline tokens carry an absolute expiry; honour it. Rotating tokens carry
  // a 30s window we deliberately SKIP offline — refusing a legitimate attendee
  // whose app happens to show the live QR would be the worse failure.
  if (p.off && p.exp && Date.now() / 1000 > p.exp + 300) {
    return { kind: 'invalid', reason: 'Ticket expired' };
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

/** Manager confirmed a provisional (unknown ref — likely sold after last sync).
 *  Logged and reconciled server-side; unresolved ones surface in exceptions. */
export async function confirmProvisional(
  eventId: string, assignmentId: string | null, bookingId: string | null, isPool: boolean,
): Promise<void> {
  await admitProvisional(uuid(), eventId, isPool ? 'pool' : 'individual', assignmentId, bookingId);
}