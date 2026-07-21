// ─────────────────────────────────────────────────────────────────────────────
// Scanner sync engine.
//
// prepareEvent(): the pre-doors step — mint the device's scoped credential and
// pull the FULL manifest into SQLite. After this, the gate works with zero
// signal for the whole admission window.
//
// startSyncLoop(): the background loop — every ~20s (and on connectivity
// regained / app foreground) it PUSHES the unsynced scan log up (idempotent by
// scan id; retries are free) and PULLS the manifest delta down, which is how
// other gates' admissions reach this device. The scan path itself never
// touches the network.
// ─────────────────────────────────────────────────────────────────────────────
import NetInfo from '@react-native-community/netinfo';
import { AppState } from 'react-native';
import { api } from '../api';
import {
  metaGet, metaSet, wipeManifest, upsertAssignments, upsertPools,
  unsyncedScans, markSynced, queuedCount, counts,
} from './db';

export interface SyncState {
  online: boolean;
  queued: number;
  lastSyncAt: string | null;
  total: number;
  scanned: number;
}

export interface ServerException {
  id: string;
  verdict: string;
  name: string;
  slot_number?: number;
  admit_count: number;
  scanned_by: string;
  device_id: string;
  scanned_at: string;
}

// UUID v4 for device + scan ids. Prefer the platform CSPRNG when present.
export function uuid(): string {
  const g: any = globalThis as any;
  if (g.crypto?.randomUUID) return g.crypto.randomUUID();
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}

export async function getDeviceId(): Promise<string> {
  let id = await metaGet('device_id');
  if (!id) {
    id = uuid();
    await metaSet('device_id', id);
  }
  return id;
}

// ── Pre-doors preparation ─────────────────────────────────────────────────────

export async function prepareEvent(eventId: string): Promise<{
  role: string; credentialExp: string; total: number;
}> {
  const deviceId = await getDeviceId();

  // 1. Scoped offline credential (also refreshes if event dates moved).
  const credRes = await api.post(`/v1/staff/events/${eventId}/scanner-credential`, {
    device_id: deviceId,
  });
  await metaSet('credential', credRes.data.credential);
  await metaSet('credential_exp', credRes.data.expires_at);
  await metaSet('credential_role', credRes.data.role);

  // 2. Full manifest. Switching events wipes the previous one.
  const active = await metaGet('active_event');
  if (active !== eventId) {
    await wipeManifest();
    await metaSet('active_event', eventId);
    await metaSet('cursor', '');
  }
  const mRes = await api.get(`/v1/staff/events/${eventId}/manifest`);
  await upsertAssignments(mRes.data.assignments ?? []);
  await upsertPools(mRes.data.pools ?? []);
  await metaSet('cursor', mRes.data.cursor);

  const c = await counts();
  return { role: credRes.data.role, credentialExp: credRes.data.expires_at, total: c.total };
}

/** True while the cached credential covers "now" — the device's local gate. */
export async function credentialValid(eventId: string): Promise<boolean> {
  const active = await metaGet('active_event');
  if (active !== eventId) return false;
  const exp = await metaGet('credential_exp');
  return !!exp && new Date(exp).getTime() > Date.now();
}

// ── One sync cycle ────────────────────────────────────────────────────────────

async function pushOnce(eventId: string): Promise<Record<string, string>> {
  const pending = await unsyncedScans(eventId);
  if (!pending.length) return {};
  const deviceId = await getDeviceId();
  const res = await api.post(`/v1/staff/events/${eventId}/scans`, {
    device_id: deviceId,
    scans: pending.map((s) => ({
      id: s.id,
      kind: s.kind,
      assignment_id: s.assignment_id ?? undefined,
      booking_id: s.booking_id ?? undefined,
      admit_count: s.admit_count,
      mode: 'offline',
      scanned_at: s.scanned_at,
    })),
  });
  const verdicts: Record<string, string> = {};
  for (const r of res.data.results ?? []) verdicts[r.id] = r.verdict;
  await markSynced(pending.map((s) => s.id), verdicts);
  return verdicts;
}

async function pullOnce(eventId: string): Promise<void> {
  const cursor = await metaGet('cursor');
  const res = await api.get(`/v1/staff/events/${eventId}/manifest`, {
    params: cursor ? { since: cursor } : {},
  });
  await upsertAssignments(res.data.assignments ?? []);
  await upsertPools(res.data.pools ?? []);
  await metaSet('cursor', res.data.cursor);
}

/** Manager devices poll the exceptions delta so collisions surface as toasts. */
async function pullExceptions(eventId: string): Promise<ServerException[]> {
  const role = (await metaGet('credential_role')) ?? 'scanner';
  if (role === 'scanner') return [];
  const exCursor = await metaGet('ex_cursor');
  const res = await api.get(`/v1/events/${eventId}/scan-exceptions`, {
    params: exCursor ? { since: exCursor } : {},
  });
  await metaSet('ex_cursor', res.data.cursor);
  // First poll returns history; only surface deltas (when we had a cursor).
  return exCursor ? (res.data.exceptions ?? []) : [];
}

// ── The loop ──────────────────────────────────────────────────────────────────

export function startSyncLoop(
  eventId: string,
  onState: (s: SyncState) => void,
  onCollisions?: (ex: ServerException[]) => void,
): () => void {
  let stopped = false;
  let online = true;
  let syncing = false;

  const report = async () => {
    const c = await counts();
    onState({
      online,
      queued: await queuedCount(eventId),
      lastSyncAt: await metaGet('last_sync'),
      total: c.total,
      scanned: c.scanned,
    });
  };

  const cycle = async () => {
    if (stopped || syncing) return;
    syncing = true;
    try {
      const verdicts = await pushOnce(eventId);
      await pullOnce(eventId);
      online = true;
      await metaSet('last_sync', new Date().toISOString());

      // Server-upgraded verdicts on OUR scans (e.g. another gate won) plus the
      // manager exceptions delta both feed the collision callback.
      const mine = Object.entries(verdicts)
        .filter(([, v]) => v === 'collision')
        .map(([id]) => ({
          id, verdict: 'collision', name: '', admit_count: 1,
          scanned_by: 'this device', device_id: 'this device',
          scanned_at: new Date().toISOString(),
        } as ServerException));
      let others: ServerException[] = [];
      try { others = await pullExceptions(eventId); } catch { /* manager-only; ignore */ }
      const all = [...mine, ...others];
      if (all.length && onCollisions) onCollisions(all);
    } catch {
      online = false; // silent retry next cycle — the gate never depends on this
    } finally {
      syncing = false;
      if (!stopped) await report();
    }
  };

  const interval = setInterval(cycle, 20_000);
  const net = NetInfo.addEventListener((state) => {
    if (state.isConnected) cycle();
  });
  const app = AppState.addEventListener('change', (st) => {
    if (st === 'active') cycle();
  });

  cycle(); // immediate first pass
  report();

  return () => {
    stopped = true;
    clearInterval(interval);
    net();
    app.remove();
  };
}