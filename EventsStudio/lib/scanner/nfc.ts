// ─────────────────────────────────────────────────────────────────────────────
// NFC reader.
//
// Android only. iOS Core NFC cannot do the DESFire authentication or the tag
// writes Phase 4's offline accumulator needs, and a half-working iOS path at a
// gate is worse than an honest QR-only one — so isNfcSupported() reports false
// on iOS and the screen keeps the camera scanner.
//
// WHAT THIS READS, AND WHY IT DIFFERS ONLINE VS OFFLINE.
//
// Offline the gate is manifest-authoritative, exactly as it is for QR (see
// pipeline.ts): the band is valid iff its UID is in the synced tag list bound
// to an admissible assignment. That needs nothing but the UID, which every
// chip emits on contact — no keys on the handset, which is the property
// ScannerCredentialService deliberately preserves.
//
// Online we can do better for free. An NTAG 424 DNA also emits a fresh signed
// SDM message on every tap, and the SERVER holds the keys to verify it, so the
// online path forwards that message and gets real anti-cloning. The cryptogram
// is carried in the tag's NDEF URL as picc_data and cmac query parameters.
//
// So: always read the UID; read NDEF as well and pass it along when present.
// The caller decides which path applies based on connectivity.
// ─────────────────────────────────────────────────────────────────────────────
import { Platform } from 'react-native';
import NfcManager, { NfcTech, Ndef } from 'react-native-nfc-manager';

export interface TagRead {
  /** Uppercase hex, no separators — matches services.NormaliseUID on the server. */
  uid: string;
  /** SDM cryptogram, present on a correctly encoded NTAG 424 DNA. */
  sdm?: { picc_data: string; cmac: string; enc_file_data?: string };
}

let started = false;
let starting: Promise<boolean> | null = null;

export function isNfcPlatform(): boolean {
  return Platform.OS === 'android';
}

/** Idempotent. Safe to call on every screen focus. */
export async function initNfc(): Promise<boolean> {
  if (!isNfcPlatform()) return false;
  if (started) return true;
  if (starting) return starting;

  starting = (async () => {
    try {
      const supported = await NfcManager.isSupported();
      if (!supported) return false;
      await NfcManager.start();
      started = true;
      return true;
    } catch {
      // A handset without NFC hardware, or NFC disabled at the OS level. Not an
      // error worth surfacing — the gate falls back to the camera.
      return false;
    } finally {
      starting = null;
    }
  })();
  return starting;
}

export async function isNfcEnabled(): Promise<boolean> {
  if (!started) return false;
  try {
    return await NfcManager.isEnabled();
  } catch {
    return false;
  }
}

/**
 * Read one tag. Resolves when a band is presented, rejects on cancel or timeout.
 *
 * Always call cancelRead() when leaving the screen: an abandoned technology
 * request holds the NFC adapter and the next read silently never fires.
 */
export async function readTag(): Promise<TagRead> {
  if (!isNfcPlatform()) throw new Error('NFC is Android-only');

  try {
    // IsoDep covers DESFire; NfcA covers NTAG. Requesting Ndef alongside them
    // lets one session return both the UID and the SDM message, rather than
    // making the attendee tap twice.
    await NfcManager.requestTechnology([NfcTech.Ndef, NfcTech.NfcA, NfcTech.IsoDep]);
    const tag = await NfcManager.getTag();
    if (!tag?.id) throw new Error('Could not read the band');

    const uid = normaliseUid(tag.id);
    const sdm = extractSdm(tag);
    return sdm ? { uid, sdm } : { uid };
  } finally {
    // Always, even on the error path. A leaked session is the single most
    // common cause of "the scanner stopped working" on Android NFC.
    try { await NfcManager.cancelTechnologyRequest(); } catch { /* already gone */ }
  }
}

export async function cancelRead(): Promise<void> {
  try { await NfcManager.cancelTechnologyRequest(); } catch { /* nothing open */ }
}

// ── Parsing ──────────────────────────────────────────────────────────────────

/**
 * Canonicalise a UID to uppercase hex with no separators.
 *
 * react-native-nfc-manager returns tag.id as a lowercase hex string on Android,
 * but the shape has varied across versions and devices — some return colon
 * separators, some a byte array. Normalising here means one representation
 * reaches the manifest lookup and the server, and it matches what
 * services.NormaliseUID does in Go. A UID that differs only in formatting would
 * otherwise miss the local tag table and be admitted as an unknown provisional.
 */
export function normaliseUid(raw: string | number[]): string {
  if (Array.isArray(raw)) {
    return raw.map((b) => (b & 0xff).toString(16).padStart(2, '0')).join('').toUpperCase();
  }
  return String(raw).replace(/[:\-\s]/g, '').toUpperCase();
}

/**
 * Pull picc_data and cmac out of the tag's NDEF URI record.
 *
 * An NTAG 424 encoded for SDM carries a URL whose query string the chip
 * rewrites on every tap. Returns undefined for any tag without one, which
 * includes every DESFire and UID-only band — those verify by other means (or,
 * for uid_only, not at all).
 */
export function extractSdm(tag: any): TagRead['sdm'] {
  try {
    const records = tag?.ndefMessage;
    if (!Array.isArray(records) || !records.length) return undefined;

    for (const rec of records) {
      let text: string | null = null;
      try {
        text = Ndef.uri.decodePayload(new Uint8Array(rec.payload));
      } catch {
        text = null;
      }
      if (!text) continue;

      const q = text.indexOf('?');
      if (q < 0) continue;
      const params = new URLSearchParams(text.slice(q + 1));

      // Parameter names follow the encoding profile. Both the conventional
      // NXP names and the shorter forms some encoders emit are accepted, so a
      // profile change at the encoder does not silently drop the cryptogram
      // and quietly downgrade every tap to UID-only.
      const picc = params.get('picc_data') ?? params.get('e') ?? params.get('d');
      const cmac = params.get('cmac') ?? params.get('c') ?? params.get('m');
      if (!picc || !cmac) continue;

      const enc = params.get('enc_file_data') ?? params.get('f') ?? undefined;
      return {
        picc_data: picc.toUpperCase(),
        cmac: cmac.toUpperCase(),
        ...(enc ? { enc_file_data: enc.toUpperCase() } : {}),
      };
    }
    return undefined;
  } catch {
    return undefined;
  }
}