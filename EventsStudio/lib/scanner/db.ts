// ─────────────────────────────────────────────────────────────────────────────
// Scanner local store (SQLite).
//
// Holds one event's manifest (attendees + pools) and the append-only local
// scan log. SQLite over AsyncStorage because manifests reach thousands of rows
// and every scan does indexed lookups; admits are transactional so a scan is
// never admitted-but-unlogged (an unrecorded admission is the worse failure —
// if the log write fails, the admit fails).
// ─────────────────────────────────────────────────────────────────────────────
import * as SQLite from 'expo-sqlite';

export interface LocalAssignment {
  assignment_id: string;
  booking_id: string;
  slot: number;
  name: string;
  tier: string;
  status: string;
  booking_ok: number; // 1 confirmed, 0 refunded/cancelled
  scanned: number;    // 1 admitted (by ANY gate, once deltas apply)
  scanned_at: string | null;
}

export interface LocalPool {
  booking_id: string;
  booking_number: string;
  buyer: string;
  // Tier the whole booking was bought under. Individual assignments have always
  // carried this; pools did not, so a group pass showed "Group" at the door and
  // staff could not tell a VIP table from a GA block.
  tier: string;
  pool_total: number;
  pool_admitted: number;
  booking_ok: number;
}

export interface LocalScan {
  id: string;
  event_id: string;
  kind: 'individual' | 'pool';
  assignment_id: string | null;
  booking_id: string | null;
  admit_count: number;
  scanned_at: string;
  synced: number;
  verdict: string; // local verdict; server may upgrade to collision on sync
}

let dbPromise: Promise<SQLite.SQLiteDatabase> | null = null;

export function getDb(): Promise<SQLite.SQLiteDatabase> {
  if (!dbPromise) {
    dbPromise = (async () => {
      const db = await SQLite.openDatabaseAsync('tickethawk_scanner.db');
      await db.execAsync(`
        PRAGMA journal_mode = WAL;
        CREATE TABLE IF NOT EXISTS meta (
          k TEXT PRIMARY KEY, v TEXT
        );
        CREATE TABLE IF NOT EXISTS assignments (
          assignment_id TEXT PRIMARY KEY,
          booking_id TEXT NOT NULL,
          slot INTEGER NOT NULL,
          name TEXT NOT NULL DEFAULT '',
          tier TEXT NOT NULL DEFAULT 'General',
          status TEXT NOT NULL,
          booking_ok INTEGER NOT NULL DEFAULT 1,
          scanned INTEGER NOT NULL DEFAULT 0,
          scanned_at TEXT
        );
        CREATE INDEX IF NOT EXISTS idx_assign_booking ON assignments (booking_id);
        CREATE TABLE IF NOT EXISTS pools (
          booking_id TEXT PRIMARY KEY,
          booking_number TEXT NOT NULL,
          buyer TEXT NOT NULL DEFAULT '',
          tier TEXT NOT NULL DEFAULT 'General Admission',
          pool_total INTEGER NOT NULL,
          pool_admitted INTEGER NOT NULL,
          booking_ok INTEGER NOT NULL DEFAULT 1
        );
        CREATE TABLE IF NOT EXISTS scan_log (
          id TEXT PRIMARY KEY,
          event_id TEXT NOT NULL,
          kind TEXT NOT NULL,
          assignment_id TEXT,
          booking_id TEXT,
          admit_count INTEGER NOT NULL DEFAULT 1,
          scanned_at TEXT NOT NULL,
          synced INTEGER NOT NULL DEFAULT 0,
          verdict TEXT NOT NULL DEFAULT 'admitted'
        );
        CREATE INDEX IF NOT EXISTS idx_scanlog_unsynced ON scan_log (event_id, synced);
      `);

      // ── Migrations ───────────────────────────────────────────────────────
      //
      // CREATE TABLE IF NOT EXISTS does nothing to a device that already has
      // the table, so a new column has to be added explicitly. Scanner
      // databases live on the device across app updates — without this, an
      // existing install would keep its old `pools` table and every manifest
      // sync would fail with "no such column: tier", at the gate, offline.
      //
      // ALTER TABLE ADD COLUMN throws if the column is already there and
      // SQLite has no IF NOT EXISTS for it, so the error is swallowed. That is
      // the intended control flow here, not a silenced bug.
      try {
        await db.execAsync(`ALTER TABLE pools ADD COLUMN tier TEXT NOT NULL DEFAULT 'General Admission'`);
      } catch {
        // already present
      }
      return db;
    })();
  }
  return dbPromise;
}

// ── meta ──────────────────────────────────────────────────────────────────────

export async function metaGet(k: string): Promise<string | null> {
  const db = await getDb();
  const row = await db.getFirstAsync<{ v: string }>('SELECT v FROM meta WHERE k = ?', k);
  return row?.v ?? null;
}

export async function metaSet(k: string, v: string): Promise<void> {
  const db = await getDb();
  await db.runAsync('INSERT INTO meta (k, v) VALUES (?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v', k, v);
}

// ── manifest load / delta upsert ─────────────────────────────────────────────

/** Wipe manifest tables — called when preparing a DIFFERENT event. */
export async function wipeManifest(): Promise<void> {
  const db = await getDb();
  await db.execAsync('DELETE FROM assignments; DELETE FROM pools;');
}

export async function upsertAssignments(rows: Array<{
  assignment_id: string; booking_id: string; slot_number: number; name: string;
  tier: string; status: string; booking_ok: boolean; scanned_at: string | null;
}>): Promise<void> {
  if (!rows.length) return;
  const db = await getDb();
  await db.withExclusiveTransactionAsync(async (txn) => {
    for (const r of rows) {
      await txn.runAsync(
        `INSERT INTO assignments (assignment_id, booking_id, slot, name, tier, status, booking_ok, scanned, scanned_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(assignment_id) DO UPDATE SET
           booking_id = excluded.booking_id, slot = excluded.slot, name = excluded.name,
           tier = excluded.tier, status = excluded.status, booking_ok = excluded.booking_ok,
           -- scanned only ratchets upward: a delta can mark a row scanned (another
           -- gate admitted it) but never un-scan one this device already admitted.
           scanned = MAX(assignments.scanned, excluded.scanned),
           scanned_at = COALESCE(assignments.scanned_at, excluded.scanned_at)`,
        r.assignment_id, r.booking_id, r.slot_number, r.name, r.tier, r.status,
        r.booking_ok ? 1 : 0, r.scanned_at ? 1 : 0, r.scanned_at,
      );
    }
  });
}

export async function upsertPools(rows: Array<{
  booking_id: string; booking_number: string; buyer_name: string;
  // Optional so a manifest from an older backend still type-checks; the insert
  // below falls back to General Admission when it is absent.
  tier?: string;
  pool_total: number; pool_admitted: number; booking_ok: boolean;
}>): Promise<void> {
  if (!rows.length) return;
  const db = await getDb();
  await db.withExclusiveTransactionAsync(async (txn) => {
    for (const r of rows) {
      await txn.runAsync(
        `INSERT INTO pools (booking_id, booking_number, buyer, tier, pool_total, pool_admitted, booking_ok)
         VALUES (?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(booking_id) DO UPDATE SET
           booking_number = excluded.booking_number, buyer = excluded.buyer,
           tier = excluded.tier,
           pool_total = excluded.pool_total,
           -- admitted ratchets upward for the same reason as scanned above
           pool_admitted = MAX(pools.pool_admitted, excluded.pool_admitted),
           booking_ok = excluded.booking_ok`,
        r.booking_id, r.booking_number, r.buyer_name, r.tier || 'General Admission',
        r.pool_total, r.pool_admitted,
        r.booking_ok ? 1 : 0,
      );
    }
  });
}

// ── lookups ───────────────────────────────────────────────────────────────────

export async function findAssignment(assignmentId: string): Promise<LocalAssignment | null> {
  const db = await getDb();
  return (await db.getFirstAsync<LocalAssignment>(
    'SELECT * FROM assignments WHERE assignment_id = ?', assignmentId)) ?? null;
}

export async function findPool(bookingId: string): Promise<LocalPool | null> {
  const db = await getDb();
  return (await db.getFirstAsync<LocalPool>(
    'SELECT * FROM pools WHERE booking_id = ?', bookingId)) ?? null;
}

export async function counts(): Promise<{ total: number; scanned: number }> {
  const db = await getDb();
  const a = await db.getFirstAsync<{ t: number; s: number }>(
    'SELECT COUNT(*) AS t, SUM(scanned) AS s FROM assignments');
  return { total: a?.t ?? 0, scanned: a?.s ?? 0 };
}

// ── admits (transactional: mark + log together) ───────────────────────────────

export async function admitIndividual(scanId: string, eventId: string, a: LocalAssignment): Promise<void> {
  const db = await getDb();
  const now = new Date().toISOString();
  await db.withExclusiveTransactionAsync(async (txn) => {
    await txn.runAsync(
      'UPDATE assignments SET scanned = 1, scanned_at = ? WHERE assignment_id = ?',
      now, a.assignment_id);
    await txn.runAsync(
      `INSERT INTO scan_log (id, event_id, kind, assignment_id, booking_id, admit_count, scanned_at, synced, verdict)
       VALUES (?, ?, 'individual', ?, ?, 1, ?, 0, 'admitted')`,
      scanId, eventId, a.assignment_id, a.booking_id, now);
  });
}

export async function admitPool(scanId: string, eventId: string, bookingId: string, count: number): Promise<void> {
  const db = await getDb();
  const now = new Date().toISOString();
  await db.withExclusiveTransactionAsync(async (txn) => {
    await txn.runAsync(
      'UPDATE pools SET pool_admitted = pool_admitted + ? WHERE booking_id = ?',
      count, bookingId);
    await txn.runAsync(
      `INSERT INTO scan_log (id, event_id, kind, assignment_id, booking_id, admit_count, scanned_at, synced, verdict)
       VALUES (?, ?, 'pool', NULL, ?, ?, ?, 0, 'admitted')`,
      scanId, eventId, bookingId, count, now);
  });
}

/** Provisional: token parsed + event matched but the ref isn't in the manifest
 *  (usually sold after last sync). Logged for server reconciliation. */
export async function admitProvisional(
  scanId: string, eventId: string,
  kind: 'individual' | 'pool', assignmentId: string | null, bookingId: string | null,
): Promise<void> {
  const db = await getDb();
  const now = new Date().toISOString();
  await db.runAsync(
    `INSERT INTO scan_log (id, event_id, kind, assignment_id, booking_id, admit_count, scanned_at, synced, verdict)
     VALUES (?, ?, ?, ?, ?, 1, ?, 0, 'provisional')`,
    scanId, eventId, kind, assignmentId, bookingId, now);
}

// ── sync queue ────────────────────────────────────────────────────────────────

export async function unsyncedScans(eventId: string, limit = 200): Promise<LocalScan[]> {
  const db = await getDb();
  return db.getAllAsync<LocalScan>(
    'SELECT * FROM scan_log WHERE event_id = ? AND synced = 0 ORDER BY scanned_at LIMIT ?',
    eventId, limit);
}

export async function markSynced(ids: string[], verdicts: Record<string, string>): Promise<void> {
  if (!ids.length) return;
  const db = await getDb();
  await db.withExclusiveTransactionAsync(async (txn) => {
    for (const id of ids) {
      await txn.runAsync(
        'UPDATE scan_log SET synced = 1, verdict = COALESCE(?, verdict) WHERE id = ?',
        verdicts[id] ?? null, id);
    }
  });
}

export async function queuedCount(eventId: string): Promise<number> {
  const db = await getDb();
  const r = await db.getFirstAsync<{ n: number }>(
    'SELECT COUNT(*) AS n FROM scan_log WHERE event_id = ? AND synced = 0', eventId);
  return r?.n ?? 0;
}