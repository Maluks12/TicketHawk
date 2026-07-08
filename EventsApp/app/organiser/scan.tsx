import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  View, Text, TouchableOpacity, StyleSheet, ActivityIndicator, FlatList,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { CameraView, useCameraPermissions } from 'expo-camera';
import { Ionicons } from '@expo/vector-icons';
import { STUDIO } from '../../components/studio/theme';
import { listMyScanEvents, type ScannableEvent } from '../../lib/staff';
import { prepareEvent, credentialValid, startSyncLoop, type SyncState } from '../../lib/scanner/sync';
import { processScan, confirmPoolAdmit, confirmProvisional, type ScanOutcome } from '../../lib/scanner/pipeline';

// ─────────────────────────────────────────────────────────────────────────────
// Local-first door scanner.
//
// Every scan resolves against the on-device manifest in milliseconds — the
// network is never in the scan path. A background loop pushes this device's
// admissions up and pulls other gates' admissions down; the pill in the header
// shows which regime you're in ("Live" vs "Offline · N queued").
// ─────────────────────────────────────────────────────────────────────────────

type Phase =
  | 'pick_event'   // no eventId param: choose from my scannable events
  | 'preparing'    // credential + manifest pre-load
  | 'prepare_failed'
  | 'ready'
  | 'result'       // admitted / duplicate / invalid card showing
  | 'pool_headcount'
  | 'unknown_confirm';

const RESULT_MS = 2200;

export default function StudioScan() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const params = useLocalSearchParams<{ eventId?: string; title?: string }>();

  const [permission, requestPermission] = useCameraPermissions();
  const [eventId, setEventId] = useState<string | null>(params.eventId ?? null);
  const [title, setTitle] = useState<string>(params.title ?? 'Scan tickets');
  const [phase, setPhase] = useState<Phase>(params.eventId ? 'preparing' : 'pick_event');

  const [myEvents, setMyEvents] = useState<ScannableEvent[]>([]);
  const [outcome, setOutcome] = useState<ScanOutcome | null>(null);
  const [headcount, setHeadcount] = useState(1);
  const [sync, setSync] = useState<SyncState | null>(null);
  const [collisionNote, setCollisionNote] = useState<string | null>(null);
  const [prepareErr, setPrepareErr] = useState<string>('');

  const busyRef = useRef(false);
  const resultTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stopLoop = useRef<(() => void) | null>(null);

  useEffect(() => {
    if (!permission) requestPermission();
  }, [permission, requestPermission]);

  // ── Event picker (entry without params, e.g. from Join event staff) ─────────
  useEffect(() => {
    if (phase !== 'pick_event') return;
    listMyScanEvents()
      .then(({ data }) => setMyEvents(data.events ?? []))
      .catch(() => setMyEvents([]));
  }, [phase]);

  // ── Prepare: credential + full manifest, then start the sync loop ───────────
  const prepare = useCallback(async (evId: string) => {
    setPhase('preparing');
    try {
      // If a valid credential + manifest for this event is already on-device,
      // preparing again just refreshes both — cheap, and handles date changes.
      await prepareEvent(evId);
      setPhase('ready');
    } catch (e: any) {
      // Offline at open: if a previous prepare covered this event, scan anyway.
      if (await credentialValid(evId)) {
        setPhase('ready');
      } else {
        setPrepareErr(e?.response?.data?.error ?? 'Could not prepare. Connect to the internet once for this event.');
        setPhase('prepare_failed');
      }
    }
  }, []);

  useEffect(() => {
    if (eventId) prepare(eventId);
  }, [eventId, prepare]);

  useEffect(() => {
    if (!eventId || (phase !== 'ready' && phase !== 'result' && phase !== 'pool_headcount' && phase !== 'unknown_confirm')) return;
    if (stopLoop.current) return;
    stopLoop.current = startSyncLoop(eventId, setSync, (ex) => {
      const first = ex[0];
      setCollisionNote(
        ex.length === 1
          ? `Duplicate entry detected${first.name ? `: ${first.name}` : ''}`
          : `${ex.length} scan exceptions detected`,
      );
      setTimeout(() => setCollisionNote(null), 6000);
    });
    return () => { stopLoop.current?.(); stopLoop.current = null; };
  }, [eventId, phase]);

  useEffect(() => () => { if (resultTimer.current) clearTimeout(resultTimer.current); }, []);

  // ── Scan handling ────────────────────────────────────────────────────────────
  const showResult = (o: ScanOutcome) => {
    setOutcome(o);
    if (o.kind === 'pool_prompt') {
      setHeadcount(1);
      setPhase('pool_headcount');
      return;
    }
    if (o.kind === 'unknown_prompt') {
      setPhase('unknown_confirm');
      return;
    }
    setPhase('result');
    if (resultTimer.current) clearTimeout(resultTimer.current);
    resultTimer.current = setTimeout(() => {
      setPhase('ready');
      setOutcome(null);
      busyRef.current = false;
    }, RESULT_MS);
  };

  const onBarcode = useCallback(async ({ data }: { data: string }) => {
    if (busyRef.current || phase !== 'ready' || !eventId) return;
    busyRef.current = true;
    const o = await processScan(eventId, data);
    showResult(o);
    if (o.kind !== 'pool_prompt' && o.kind !== 'unknown_prompt') {
      // busyRef released by the result timer
    }
  }, [phase, eventId]);

  const resume = () => { setPhase('ready'); setOutcome(null); busyRef.current = false; };

  const doPoolAdmit = async () => {
    if (!eventId || !outcome || outcome.kind !== 'pool_prompt') return;
    await confirmPoolAdmit(eventId, outcome.bookingId, headcount);
    showResult({ kind: 'admitted', name: `${outcome.buyer} +${headcount}`, tier: 'Group', slot: 0 });
  };

  const doProvisional = async () => {
    if (!eventId || !outcome || outcome.kind !== 'unknown_prompt') return;
    await confirmProvisional(eventId, outcome.assignmentId, outcome.bookingId, outcome.isPool);
    showResult({ kind: 'admitted', name: 'Provisional entry', tier: 'Flagged for review', slot: 0 });
  };

  // ── Renders ──────────────────────────────────────────────────────────────────

  if (phase === 'pick_event') {
    return (
      <View style={[styles.root, { paddingTop: insets.top + 8 }]}>
        <Header title="Choose event" router={router} />
        <Text style={styles.sub}>Events you can scan:</Text>
        <FlatList
          data={myEvents}
          keyExtractor={(e) => e.id}
          contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}
          ListEmptyComponent={<Text style={styles.sub}>Nothing yet — join an event's staff first.</Text>}
          renderItem={({ item }) => (
            <TouchableOpacity
              style={styles.pickRow}
              onPress={() => { setTitle(item.title); setEventId(item.id); }}
            >
              <View style={{ flex: 1 }}>
                <Text style={styles.pickTitle} numberOfLines={1}>{item.title}</Text>
                <Text style={styles.pickMeta}>{new Date(item.start_time).toLocaleString()} · {item.role}</Text>
              </View>
              <Ionicons name="qr-code-outline" size={20} color={STUDIO.accent} />
            </TouchableOpacity>
          )}
        />
      </View>
    );
  }

  if (phase === 'preparing' || phase === 'prepare_failed') {
    return (
      <View style={[styles.root, styles.center, { paddingTop: insets.top }]}>
        {phase === 'preparing' ? (
          <>
            <ActivityIndicator color={STUDIO.accent} size="large" />
            <Text style={styles.prepTitle}>Preparing for offline…</Text>
            <Text style={styles.sub}>Downloading the attendee list so scanning works with no signal.</Text>
          </>
        ) : (
          <>
            <Ionicons name="cloud-offline-outline" size={40} color={STUDIO.red} />
            <Text style={styles.prepTitle}>Couldn't prepare</Text>
            <Text style={[styles.sub, { textAlign: 'center' }]}>{prepareErr}</Text>
            <TouchableOpacity style={styles.primaryBtn} onPress={() => eventId && prepare(eventId)}>
              <Text style={styles.primaryBtnTxt}>Retry</Text>
            </TouchableOpacity>
          </>
        )}
      </View>
    );
  }

  if (!permission?.granted) {
    return (
      <View style={[styles.root, styles.center, { paddingTop: insets.top }]}>
        <Ionicons name="camera-outline" size={40} color={STUDIO.sub} />
        <Text style={styles.prepTitle}>Camera access needed</Text>
        <TouchableOpacity style={styles.primaryBtn} onPress={requestPermission}>
          <Text style={styles.primaryBtnTxt}>Allow camera</Text>
        </TouchableOpacity>
      </View>
    );
  }

  const pill = sync
    ? sync.online
      ? `Live · ${sync.scanned}/${sync.total} in`
      : `Offline · ${sync.queued} queued`
    : '…';

  return (
    <View style={styles.root}>
      <CameraView
        style={StyleSheet.absoluteFill}
        facing="back"
        barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
        onBarcodeScanned={phase === 'ready' ? onBarcode : undefined}
      />

      {/* Header overlay */}
      <View style={[styles.overlayHeader, { paddingTop: insets.top + 8 }]}>
        <TouchableOpacity onPress={() => router.back()} hitSlop={12} style={styles.hBtn}>
          <Ionicons name="chevron-back" size={22} color="#fff" />
        </TouchableOpacity>
        <View style={{ flex: 1, alignItems: 'center' }}>
          <Text style={styles.hTitle} numberOfLines={1}>{title}</Text>
          <View style={[styles.pill, { backgroundColor: sync?.online ? 'rgba(0,229,160,0.18)' : 'rgba(255,184,48,0.18)' }]}>
            <View style={[styles.pillDot, { backgroundColor: sync?.online ? STUDIO.green : '#ffb830' }]} />
            <Text style={styles.pillTxt}>{pill}</Text>
          </View>
        </View>
        <TouchableOpacity
          onPress={() => router.push({ pathname: '/organiser/exceptions/[eventId]', params: { eventId, title } } as any)}
          hitSlop={12} style={styles.hBtn}
        >
          <Ionicons name="alert-circle-outline" size={20} color="#fff" />
        </TouchableOpacity>
      </View>

      {collisionNote && (
        <View style={[styles.collisionBanner, { top: insets.top + 64 }]}>
          <Ionicons name="warning-outline" size={15} color="#111" />
          <Text style={styles.collisionTxt}>{collisionNote}</Text>
        </View>
      )}

      {/* Finder */}
      {phase === 'ready' && (
        <View style={styles.finderWrap} pointerEvents="none">
          <View style={styles.finder} />
          <Text style={styles.finderHint}>Point at the ticket QR</Text>
        </View>
      )}

      {/* Result card */}
      {phase === 'result' && outcome && (
        <TouchableOpacity style={styles.resultWrap} activeOpacity={1} onPress={resume}>
          <View style={[
            styles.resultCard,
            outcome.kind === 'admitted' && { borderColor: STUDIO.green },
            outcome.kind === 'duplicate' && { borderColor: '#ffb830' },
            outcome.kind === 'invalid' && { borderColor: STUDIO.red },
          ]}>
            <Ionicons
              name={outcome.kind === 'admitted' ? 'checkmark-circle' : outcome.kind === 'duplicate' ? 'time' : 'close-circle'}
              size={42}
              color={outcome.kind === 'admitted' ? STUDIO.green : outcome.kind === 'duplicate' ? '#ffb830' : STUDIO.red}
            />
            {outcome.kind === 'admitted' && (
              <>
                <Text style={styles.resultName}>{outcome.name}</Text>
                <Text style={styles.resultMeta}>{outcome.tier}{outcome.slot ? ` · Slot ${outcome.slot}` : ''}</Text>
                <Text style={[styles.resultVerdict, { color: STUDIO.green }]}>ADMITTED</Text>
              </>
            )}
            {outcome.kind === 'duplicate' && (
              <>
                <Text style={styles.resultName}>{outcome.name}</Text>
                {outcome.firstScanAt && (
                  <Text style={styles.resultMeta}>First entry {new Date(outcome.firstScanAt).toLocaleTimeString()}</Text>
                )}
                <Text style={[styles.resultVerdict, { color: '#ffb830' }]}>ALREADY USED</Text>
              </>
            )}
            {outcome.kind === 'invalid' && (
              <>
                <Text style={styles.resultName}>{outcome.reason}</Text>
                <Text style={[styles.resultVerdict, { color: STUDIO.red }]}>NOT VALID</Text>
              </>
            )}
          </View>
        </TouchableOpacity>
      )}

      {/* Pool headcount picker */}
      {phase === 'pool_headcount' && outcome?.kind === 'pool_prompt' && (
        <View style={styles.resultWrap}>
          <View style={[styles.resultCard, { borderColor: STUDIO.accent }]}>
            <Text style={styles.resultName}>{outcome.buyer}</Text>
            <Text style={styles.resultMeta}>{outcome.bookingNumber} · {outcome.remaining} remaining</Text>
            <View style={styles.countRow}>
              <TouchableOpacity style={styles.countBtn} onPress={() => setHeadcount(Math.max(1, headcount - 1))}>
                <Ionicons name="remove" size={22} color="#fff" />
              </TouchableOpacity>
              <Text style={styles.countTxt}>{headcount}</Text>
              <TouchableOpacity style={styles.countBtn} onPress={() => setHeadcount(Math.min(outcome.remaining, headcount + 1))}>
                <Ionicons name="add" size={22} color="#fff" />
              </TouchableOpacity>
            </View>
            <TouchableOpacity style={styles.primaryBtn} onPress={doPoolAdmit}>
              <Text style={styles.primaryBtnTxt}>Admit {headcount}</Text>
            </TouchableOpacity>
            <TouchableOpacity onPress={resume}><Text style={styles.cancelTxt}>Cancel</Text></TouchableOpacity>
          </View>
        </View>
      )}

      {/* Unknown ticket → provisional confirm */}
      {phase === 'unknown_confirm' && outcome?.kind === 'unknown_prompt' && (
        <View style={styles.resultWrap}>
          <View style={[styles.resultCard, { borderColor: '#ffb830' }]}>
            <Ionicons name="help-circle" size={40} color="#ffb830" />
            <Text style={styles.resultName}>Unknown ticket</Text>
            <Text style={[styles.resultMeta, { textAlign: 'center' }]}>
              Not in this device's attendee list — possibly sold after your last sync.
              Admitting will flag it for review.
            </Text>
            <TouchableOpacity style={[styles.primaryBtn, { backgroundColor: '#ffb830' }]} onPress={doProvisional}>
              <Text style={[styles.primaryBtnTxt, { color: '#111' }]}>Admit provisionally</Text>
            </TouchableOpacity>
            <TouchableOpacity onPress={resume}><Text style={styles.cancelTxt}>Deny</Text></TouchableOpacity>
          </View>
        </View>
      )}
    </View>
  );
}

function Header({ title, router }: { title: string; router: any }) {
  return (
    <View style={styles.header}>
      <TouchableOpacity onPress={() => router.back()} hitSlop={12} style={styles.backBtn}>
        <Ionicons name="chevron-back" size={22} color={STUDIO.text} />
      </TouchableOpacity>
      <Text style={styles.title}>{title}</Text>
      <View style={{ width: 34 }} />
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: STUDIO.bg, paddingHorizontal: 16 },
  center: { alignItems: 'center', justifyContent: 'center', gap: 12 },
  header: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginBottom: 14 },
  backBtn: { width: 34, height: 34, borderRadius: 17, alignItems: 'center', justifyContent: 'center', backgroundColor: STUDIO.surface },
  title: { color: STUDIO.text, fontSize: 17, fontWeight: '700' },
  sub: { color: STUDIO.sub, fontSize: 13, lineHeight: 19, marginBottom: 10 },
  prepTitle: { color: STUDIO.text, fontSize: 16, fontWeight: '700', marginTop: 6 },

  pickRow: {
    flexDirection: 'row', alignItems: 'center', backgroundColor: STUDIO.surface,
    borderRadius: 14, borderWidth: 1, borderColor: STUDIO.border,
    paddingHorizontal: 14, paddingVertical: 13, marginBottom: 10,
  },
  pickTitle: { color: STUDIO.text, fontSize: 14.5, fontWeight: '600' },
  pickMeta: { color: STUDIO.sub, fontSize: 12, marginTop: 3 },

  overlayHeader: {
    position: 'absolute', top: 0, left: 0, right: 0, zIndex: 5,
    flexDirection: 'row', alignItems: 'center', paddingHorizontal: 14, paddingBottom: 10,
    backgroundColor: 'rgba(0,0,0,0.45)',
  },
  hBtn: { width: 34, height: 34, borderRadius: 17, alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(255,255,255,0.14)' },
  hTitle: { color: '#fff', fontSize: 15, fontWeight: '700', maxWidth: 220 },
  pill: { flexDirection: 'row', alignItems: 'center', gap: 5, borderRadius: 99, paddingHorizontal: 9, paddingVertical: 3, marginTop: 4 },
  pillDot: { width: 6, height: 6, borderRadius: 3 },
  pillTxt: { color: '#fff', fontSize: 10.5, fontWeight: '700' },

  collisionBanner: {
    position: 'absolute', left: 16, right: 16, zIndex: 6,
    flexDirection: 'row', alignItems: 'center', gap: 7,
    backgroundColor: '#ffb830', borderRadius: 10, paddingHorizontal: 12, paddingVertical: 9,
  },
  collisionTxt: { color: '#111', fontSize: 12.5, fontWeight: '700', flex: 1 },

  finderWrap: { ...StyleSheet.absoluteFillObject, alignItems: 'center', justifyContent: 'center' },
  finder: { width: 244, height: 244, borderRadius: 24, borderWidth: 2, borderColor: 'rgba(255,255,255,0.85)' },
  finderHint: { color: 'rgba(255,255,255,0.8)', fontSize: 13, marginTop: 14, fontWeight: '600' },

  resultWrap: { ...StyleSheet.absoluteFillObject, alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(0,0,0,0.55)', padding: 24 },
  resultCard: {
    width: '100%', maxWidth: 340, backgroundColor: STUDIO.surface, borderRadius: 20,
    borderWidth: 2, borderColor: STUDIO.border, alignItems: 'center', padding: 22, gap: 8,
  },
  resultName: { color: STUDIO.text, fontSize: 22, fontWeight: '800', textAlign: 'center' },
  resultMeta: { color: STUDIO.sub, fontSize: 13 },
  resultVerdict: { fontSize: 13, fontWeight: '800', letterSpacing: 1.5, marginTop: 2 },

  countRow: { flexDirection: 'row', alignItems: 'center', gap: 22, marginVertical: 8 },
  countBtn: { width: 44, height: 44, borderRadius: 22, backgroundColor: 'rgba(255,255,255,0.12)', alignItems: 'center', justifyContent: 'center' },
  countTxt: { color: STUDIO.text, fontSize: 30, fontWeight: '800', minWidth: 44, textAlign: 'center' },

  primaryBtn: { backgroundColor: STUDIO.accentDeep, borderRadius: 12, paddingVertical: 13, paddingHorizontal: 26, alignItems: 'center', marginTop: 6 },
  primaryBtnTxt: { color: '#fff', fontWeight: '700', fontSize: 15 },
  cancelTxt: { color: STUDIO.sub, fontSize: 13, fontWeight: '600', marginTop: 10 },
});