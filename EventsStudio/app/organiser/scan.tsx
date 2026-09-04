import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  View, Text, TouchableOpacity, StyleSheet, ActivityIndicator, FlatList,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { CameraView, useCameraPermissions } from 'expo-camera';
import { Ionicons } from '@expo/vector-icons';
import { makeStyles, useStudioTheme } from '../../components/studio/theme';
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
//
// ── Why the non-camera phases now have a header ─────────────────────────────
// preparing / prepare_failed / camera-denied used to render as a bare centred
// column with no way out but the OS back gesture, which this stack disables
// (gestureEnabled: false on the scan route, so a half-swipe can't dismiss the
// camera mid-scan). A closed admission window therefore produced a screen with
// exactly one button — Retry — that could never succeed: the server is right,
// the window HAS ended, and retrying re-asks the same question. Every one of
// those phases now carries a back button.
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
  const { S, isDark } = useStudioTheme();
  const styles = useStyles();

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

  // ── Background sync loop ────────────────────────────────────────────────────
  //
  // Tied to the EVENT, not to the scan phase.
  //
  // This effect used to list `phase` in its dependencies. phase changes TWICE
  // per scan (ready -> result -> ready), and every change ran the cleanup and
  // then re-ran the effect — and startSyncLoop fires a full cycle() the moment
  // it starts. A gate scanning continuously was therefore doing a push + pull +
  // exceptions round-trip per ticket instead of one every 20 seconds, on the one
  // subsystem whose entire purpose is to keep the network out of the scan path.
  // On venue wifi that is exactly when it hurts most.
  //
  // It also lost data: the teardown sets stopped = true inside the loop, so any
  // cycle still in flight skipped its report() and threw away the pushOnce
  // verdicts — the collision toast for that batch never fired. Collisions were
  // being dropped silently during rapid scanning, which is the one moment they
  // matter.
  //
  // eventId alone is correct: the loop should live exactly as long as the screen
  // is pointed at one event.
  useEffect(() => {
    if (!eventId) return;

    const stop = startSyncLoop(eventId, setSync, (ex) => {
      const first = ex[0];
      setCollisionNote(
        ex.length === 1
          ? `Duplicate entry detected${first.name ? `: ${first.name}` : ''}`
          : `${ex.length} scan exceptions detected`,
      );
      // Replace rather than stack: back-to-back collisions used to leave
      // orphaned timers, so the second banner was cleared by the first one's
      // timeout and vanished early.
      if (collisionTimer.current) clearTimeout(collisionTimer.current);
      collisionTimer.current = setTimeout(() => setCollisionNote(null), 6000);
    });

    return () => {
      stop();
      if (collisionTimer.current) {
        clearTimeout(collisionTimer.current);
        collisionTimer.current = null;
      }
    };
  }, [eventId]);

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
    // Was hardcoded 'Group', which told the door nothing about what the group
    // had actually bought. outcome.tier comes from the manifest.
    showResult({
      kind: 'admitted',
      name: `${outcome.buyer} +${headcount}`,
      tier: `${outcome.tier} · ${headcount} admitted`,
      slot: 0,
    });
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
        <Header title="Choose event" onBack={goBack} />
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
              <Ionicons name="qr-code-outline" size={20} color={S.accent} />
            </TouchableOpacity>
          )}
        />
      </View>
    );
  }

  if (phase === 'preparing' || phase === 'prepare_failed') {
    return (
      <View style={[styles.root, { paddingTop: insets.top + 8 }]}>
        <Header title={title} onBack={goBack} />
        <View style={styles.centerFill}>
          {phase === 'preparing' ? (
            <>
              <ActivityIndicator color={S.accent} size="large" />
              <Text style={styles.prepTitle}>Preparing for offline…</Text>
              <Text style={[styles.sub, { textAlign: 'center' }]}>
                Downloading the attendee list so scanning works with no signal.
              </Text>
            </>
          ) : (
            <>
              <Ionicons name="cloud-offline-outline" size={40} color={S.red} />
              <Text style={styles.prepTitle}>Couldn't prepare</Text>
              <Text style={[styles.sub, { textAlign: 'center' }]}>{prepareErr}</Text>
              <View style={styles.failActions}>
                <TouchableOpacity style={styles.secondaryBtn} onPress={goBack} activeOpacity={0.85}>
                  <Text style={styles.secondaryBtnTxt}>
                    {cameFromPicker && eventId ? 'Choose another event' : 'Go back'}
                  </Text>
                </TouchableOpacity>
                <TouchableOpacity
                  style={styles.primaryBtn}
                  onPress={() => eventId && prepare(eventId)}
                  activeOpacity={0.85}
                >
                  <Text style={styles.primaryBtnTxt}>Retry</Text>
                </TouchableOpacity>
              </View>
            </>
          )}
        </View>
      </View>
    );
  }

  if (!permission?.granted) {
    return (
      <View style={[styles.root, { paddingTop: insets.top + 8 }]}>
        <Header title={title} onBack={goBack} />
        <View style={styles.centerFill}>
          <Ionicons name="camera-outline" size={40} color={S.sub} />
          <Text style={styles.prepTitle}>Camera access needed</Text>
          <Text style={[styles.sub, { textAlign: 'center' }]}>
            Scanning reads the ticket QR through the camera. Nothing is recorded.
          </Text>
          <TouchableOpacity style={styles.primaryBtn} onPress={requestPermission} activeOpacity={0.85}>
            <Text style={styles.primaryBtnTxt}>Allow camera</Text>
          </TouchableOpacity>
        </View>
      </View>
    );
  }

  const pill = sync
    ? sync.online
      ? `Live · ${sync.scanned}/${sync.total} in`
      : `Offline · ${sync.queued} queued`
    : '…';

  // From here down the screen is a camera viewfinder, so the overlay chrome
  // stays light-on-dark in BOTH themes — it sits on a live image, not on the
  // app surface. Only the floating cards adopt the palette, and they use the
  // opaque `onCamera` surface rather than the translucent one, which would be
  // unreadable over a moving picture.
  return (
    <View style={styles.camRoot}>
      <CameraView
        style={StyleSheet.absoluteFill}
        facing="back"
        barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
        onBarcodeScanned={phase === 'ready' ? onBarcode : undefined}
      />

      {/* Header overlay */}
      <View style={[styles.overlayHeader, { paddingTop: insets.top + 8 }]}>
        <TouchableOpacity onPress={goBack} hitSlop={12} style={styles.hBtn}>
          <Ionicons name="chevron-back" size={22} color="#fff" />
        </TouchableOpacity>
        <View style={{ flex: 1, alignItems: 'center' }}>
          <Text style={styles.hTitle} numberOfLines={1}>{title}</Text>
          <View style={[styles.pill, { backgroundColor: sync?.online ? 'rgba(0,229,160,0.18)' : 'rgba(255,184,48,0.18)' }]}>
            <View style={[styles.pillDot, { backgroundColor: sync?.online ? '#00e5a0' : '#ffb830' }]} />
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
            outcome.kind === 'admitted' && { borderColor: S.green },
            outcome.kind === 'duplicate' && { borderColor: S.amber },
            outcome.kind === 'invalid' && { borderColor: S.red },
          ]}>
            <Ionicons
              name={outcome.kind === 'admitted' ? 'checkmark-circle' : outcome.kind === 'duplicate' ? 'time' : 'close-circle'}
              size={42}
              color={outcome.kind === 'admitted' ? S.green : outcome.kind === 'duplicate' ? S.amber : S.red}
            />
            {outcome.kind === 'admitted' && (
              <>
                <Text style={styles.resultName}>{outcome.name}</Text>
                <Text style={styles.resultMeta}>{outcome.tier}{outcome.slot ? ` · Slot ${outcome.slot}` : ''}</Text>
                <Text style={[styles.resultVerdict, { color: S.green }]}>ADMITTED</Text>
              </>
            )}
            {outcome.kind === 'duplicate' && (
              <>
                <Text style={styles.resultName}>{outcome.name}</Text>
                {outcome.firstScanAt && (
                  <Text style={styles.resultMeta}>First entry {new Date(outcome.firstScanAt).toLocaleTimeString()}</Text>
                )}
                <Text style={[styles.resultVerdict, { color: S.amber }]}>ALREADY USED</Text>
              </>
            )}
            {outcome.kind === 'invalid' && (
              <>
                <Text style={styles.resultName}>{outcome.reason}</Text>
                <Text style={[styles.resultVerdict, { color: S.red }]}>NOT VALID</Text>
              </>
            )}
          </View>
        </TouchableOpacity>
      )}

      {/* Pool headcount picker */}
      {phase === 'pool_headcount' && outcome?.kind === 'pool_prompt' && (
        <View style={styles.resultWrap}>
          <View style={[styles.resultCard, { borderColor: S.accent }]}>
            <Text style={styles.resultName}>{outcome.buyer}</Text>
            <Text style={styles.resultMeta}>{outcome.bookingNumber} · {outcome.remaining} remaining</Text>
            <View style={styles.countRow}>
              <TouchableOpacity style={styles.countBtn} onPress={() => setHeadcount(Math.max(1, headcount - 1))}>
                <Ionicons name="remove" size={22} color={S.text} />
              </TouchableOpacity>
              <Text style={styles.countTxt}>{headcount}</Text>
              <TouchableOpacity style={styles.countBtn} onPress={() => setHeadcount(Math.min(outcome.remaining, headcount + 1))}>
                <Ionicons name="add" size={22} color={S.text} />
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
          <View style={[styles.resultCard, { borderColor: S.amber }]}>
            <Ionicons name="help-circle" size={40} color={S.amber} />
            <Text style={styles.resultName}>Unknown ticket</Text>
            <Text style={[styles.resultMeta, { textAlign: 'center' }]}>
              Not in this device's attendee list — possibly sold after your last sync.
              Admitting will flag it for review.
            </Text>
            {/* The label colour has to follow the palette: the dark amber is a
                bright yellow (#FFB830) that needs near-black on it, the light
                amber is a dark ochre (#A65B00) that needs white. A fixed #111
                was unreadable in one of the two. */}
            <TouchableOpacity style={[styles.primaryBtn, { backgroundColor: S.amber }]} onPress={doProvisional}>
              <Text style={[styles.primaryBtnTxt, { color: isDark ? '#111' : '#fff' }]}>Admit provisionally</Text>
            </TouchableOpacity>
            <TouchableOpacity onPress={resume}><Text style={styles.cancelTxt}>Deny</Text></TouchableOpacity>
          </View>
        </View>
      )}
    </View>
  );
}

function Header({ title, onBack }: { title: string; onBack: () => void }) {
  const { S } = useStudioTheme();
  const styles = useStyles();
  return (
    <View style={styles.header}>
      <TouchableOpacity onPress={onBack} hitSlop={12} style={styles.backBtn} accessibilityRole="button" accessibilityLabel="Go back">
        <Ionicons name="chevron-back" size={22} color={S.text} />
      </TouchableOpacity>
      <Text style={styles.title} numberOfLines={1}>{title}</Text>
      <View style={{ width: 34 }} />
    </View>
  );
}

const useStyles = makeStyles((S) => ({
  root: { flex: 1, backgroundColor: S.bg, paddingHorizontal: 16 },
  camRoot: { flex: 1, backgroundColor: '#000' },
  centerFill: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 12, paddingBottom: 60 },
  header: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginBottom: 14 },
  backBtn: {
    width: 34, height: 34, borderRadius: 17, alignItems: 'center', justifyContent: 'center',
    backgroundColor: S.surfaceStrong,
  },
  title: { color: S.text, fontSize: 17, fontWeight: '700', flex: 1, textAlign: 'center', marginHorizontal: 8 },
  sub: { color: S.sub, fontSize: 13, lineHeight: 19, marginBottom: 10 },
  prepTitle: { color: S.text, fontSize: 16, fontWeight: '700', marginTop: 6 },
  failActions: { flexDirection: 'row', gap: 10, marginTop: 10, flexWrap: 'wrap', justifyContent: 'center' },

  pickRow: {
    flexDirection: 'row', alignItems: 'center', backgroundColor: S.surface,
    borderRadius: 14, borderWidth: 1, borderColor: S.border,
    paddingHorizontal: 14, paddingVertical: 13, marginBottom: 10,
  },
  pickTitle: { color: S.text, fontSize: 14.5, fontWeight: '600' },
  pickMeta: { color: S.sub, fontSize: 12, marginTop: 3 },

  // ── Camera chrome. Fixed light-on-dark in both themes: it floats on a live
  // image, and a light scrim over a bright venue shot reads as nothing at all.
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

  resultWrap: { ...StyleSheet.absoluteFillObject, alignItems: 'center', justifyContent: 'center', backgroundColor: S.scrim, padding: 24 },
  // onCamera, not surface: `surface` is a translucent tint meant to sit on the
  // app background. Over a camera feed it let the picture through and the
  // verdict text became unreadable at exactly the wrong moment.
  resultCard: {
    width: '100%', maxWidth: 340, backgroundColor: S.onCamera, borderRadius: 20,
    borderWidth: 2, borderColor: S.border, alignItems: 'center', padding: 22, gap: 8,
  },
  resultName: { color: S.text, fontSize: 22, fontWeight: '800', textAlign: 'center' },
  resultMeta: { color: S.sub, fontSize: 13 },
  resultVerdict: { fontSize: 13, fontWeight: '800', letterSpacing: 1.5, marginTop: 2 },

  countRow: { flexDirection: 'row', alignItems: 'center', gap: 22, marginVertical: 8 },
  countBtn: {
    width: 44, height: 44, borderRadius: 22, backgroundColor: S.btnSecondaryBg,
    alignItems: 'center', justifyContent: 'center',
  },
  countTxt: { color: S.text, fontSize: 30, fontWeight: '800', minWidth: 44, textAlign: 'center' },

  primaryBtn: { backgroundColor: S.accentDeep, borderRadius: 12, paddingVertical: 13, paddingHorizontal: 26, alignItems: 'center', marginTop: 6 },
  primaryBtnTxt: { color: S.onAccent, fontWeight: '700', fontSize: 15 },
  secondaryBtn: {
    backgroundColor: S.btnSecondaryBg, borderRadius: 12, paddingVertical: 13, paddingHorizontal: 22,
    alignItems: 'center', marginTop: 6, borderWidth: StyleSheet.hairlineWidth, borderColor: S.border,
  },
  secondaryBtnTxt: { color: S.text, fontWeight: '700', fontSize: 15 },
  cancelTxt: { color: S.sub, fontSize: 13, fontWeight: '600', marginTop: 10 },
}));