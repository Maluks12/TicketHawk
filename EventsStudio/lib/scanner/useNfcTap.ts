// ─────────────────────────────────────────────────────────────────────────────
// useNfcTap — the whole NFC read loop behind one hook, so scan.tsx needs a few
// lines rather than a rewrite.
//
// Mirrors how the camera scanner is already driven: arm it, get outcomes back,
// disarm on blur. The ScanOutcome type is shared with processScan, so the
// screen renders a tap result through exactly the same branch it renders a QR
// result through.
//
// ONLINE VS OFFLINE. Online it posts the SDM cryptogram to /tags/tap and the
// SERVER verifies it — real anti-cloning, because the keys live there. Offline
// it falls back to processTap and the synced manifest. The fallback is silent
// and automatic: a gate whose signal drops mid-queue must keep working, and a
// staffer should not have to know which mode they are in.
//
// The consequence, stated plainly because it is the design and not an
// oversight: offline, a cloned band gets through. The server refuses it at sync
// (ClassifyTagScan) and it lands in the exceptions report, but it does not stop
// somebody walking in during the outage. That is the same exposure the offline
// QR token already carries, and it is why Phase 3's wallet does NOT reuse this
// trust model for money.
// ─────────────────────────────────────────────────────────────────────────────
import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../api';
import { initNfc, isNfcPlatform, readTag, cancelRead, isNfcEnabled } from './nfc';
import { processTap, type ScanOutcome } from './pipeline';
import { getDeviceId } from './sync';

export interface NfcState {
  supported: boolean;
  enabled: boolean;
  scanning: boolean;
}

export function useNfcTap(
  eventId: string,
  onOutcome: (o: ScanOutcome) => void,
  online: boolean,
) {
  const [state, setState] = useState<NfcState>({
    supported: false, enabled: false, scanning: false,
  });

  // Ref rather than state: the read loop reads this on every iteration and must
  // see the current value, not the one captured when the loop started.
  const activeRef = useRef(false);
  const onlineRef = useRef(online);
  onlineRef.current = online;

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const supported = await initNfc();
      const enabled = supported ? await isNfcEnabled() : false;
      if (!cancelled) setState((s) => ({ ...s, supported, enabled }));
    })();
    return () => { cancelled = true; };
  }, []);

  const loop = useCallback(async () => {
    while (activeRef.current) {
      try {
        const read = await readTag();

        let outcome: ScanOutcome;
        if (onlineRef.current && read.sdm) {
          try {
            outcome = await tapOnline(eventId, read.sdm);
          } catch {
            // Server unreachable or slow. Falling back rather than failing is
            // the point of the offline design — a queue does not wait for a
            // timeout.
            outcome = await processTap(eventId, read.uid);
          }
        } else {
          outcome = await processTap(eventId, read.uid);
        }
        onOutcome(outcome);
      } catch (e: any) {
        // A cancelled session (screen blurred, user backed out) is the normal
        // way this loop ends, not an error worth showing.
        if (activeRef.current) {
          onOutcome({ kind: 'invalid', reason: 'Could not read the band — try again' });
        }
      }
    }
  }, [eventId, onOutcome]);

  const start = useCallback(() => {
    if (!isNfcPlatform() || activeRef.current) return;
    activeRef.current = true;
    setState((s) => ({ ...s, scanning: true }));
    loop();
  }, [loop]);

  const stop = useCallback(async () => {
    activeRef.current = false;
    setState((s) => ({ ...s, scanning: false }));
    // Essential. An abandoned technology request keeps the NFC adapter and the
    // next read silently never fires — the classic "scanner stopped working"
    // Android NFC bug.
    await cancelRead();
  }, []);

  useEffect(() => () => { activeRef.current = false; cancelRead(); }, []);

  return { ...state, start, stop };
}

/**
 * Online tap: the server verifies the chip's cryptogram with keys the handset
 * never holds, so a cloned UID is refused outright rather than merely flagged
 * at reconciliation.
 */
async function tapOnline(
  eventId: string,
  sdm: { picc_data: string; cmac: string; enc_file_data?: string },
): Promise<ScanOutcome> {
  const deviceId = await getDeviceId();
  try {
    const res = await api.post(`/v1/staff/events/${eventId}/tags/tap`, {
      device_id: deviceId,
      chip_type: 'ntag424',
      sdm,
    });
    return {
      kind: 'admitted',
      name: res.data.attendee || `Slot ${res.data.slot_number ?? ''}`.trim(),
      tier: res.data.tier || 'General Admission',
      slot: res.data.slot_number ?? 0,
    };
  } catch (e: any) {
    const status = e?.response?.status;
    const body = e?.response?.data ?? {};

    // 409 covers already-scanned, band not active, and replayed tap. Each is a
    // different sentence at the door, so they are not collapsed.
    if (status === 409) {
      if (typeof body.error === 'string' && body.error.includes('already scanned')) {
        return { kind: 'duplicate', name: '', firstScanAt: body.first_scan_at ?? null };
      }
      return { kind: 'invalid', reason: body.error || 'This band cannot be admitted' };
    }
    if (status === 404) {
      return { kind: 'unknown_prompt', assignmentId: null, bookingId: null, isPool: false };
    }
    if (status === 422) {
      // Verification failed: cloned band, wrong event, or a real encoding
      // problem. Refusing is correct either way.
      return { kind: 'invalid', reason: 'This band could not be verified' };
    }
    throw e; // anything else: let the caller fall back to the offline path
  }
}