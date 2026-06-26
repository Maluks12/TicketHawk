import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  View,
  Text,
  TouchableOpacity,
  StyleSheet,
  Animated,
  Easing,
  Linking,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { CameraView, useCameraPermissions } from 'expo-camera';
import { Feather, Ionicons } from '@expo/vector-icons';
import { STUDIO } from '../../components/studio/theme';
import { scanToken, newScanKey, fmtTime } from '../../lib/organiser';

type Phase =
  | 'ready'
  | 'processing'
  | 'admitted'
  | 'already_used'
  | 'invalid'
  | 'pool_headcount';

interface Result {
  phase: 'admitted' | 'already_used' | 'invalid' | 'pool_headcount';
  slotNumber?: number;
  firstScanAt?: string;
  message?: string;
  admitted?: number;   // pool admit: how many just went in
  remaining?: number;  // pool: how many remain after
  token?: string;      // pool headcount: the raw token to admit against
}

const FINDER = 244;

export default function StudioScan() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { title } = useLocalSearchParams<{ eventId?: string; title?: string }>();

  const [permission, requestPermission] = useCameraPermissions();
  const [phase, setPhase] = useState<Phase>('ready');
  const [result, setResult] = useState<Result | null>(null);
  const [admitted, setAdmitted] = useState(0);
  const [headcount, setHeadcount] = useState(1); // selected count in the picker

  // Ignore repeated decodes of the same code while a result is on screen.
  const cooldownRef = useRef<number>(0);
  const scanLine = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    if (!permission) requestPermission();
  }, [permission, requestPermission]);

  // Scan-line animation
  useEffect(() => {
    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(scanLine, {
          toValue: 1,
          duration: 1600,
          easing: Easing.inOut(Easing.ease),
          useNativeDriver: true,
        }),
        Animated.timing(scanLine, {
          toValue: 0,
          duration: 1600,
          easing: Easing.inOut(Easing.ease),
          useNativeDriver: true,
        }),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [scanLine]);

  const scheduleReset = useCallback(() => {
    setTimeout(() => {
      setResult(null);
      setPhase('ready');
      cooldownRef.current = 0;
    }, 3500);
  }, []);

  const handleScan = useCallback(
    async (raw: string) => {
      if (Date.now() < cooldownRef.current) return;
      cooldownRef.current = Date.now() + 5000;

      setPhase('processing');
      try {
        // No count: admits an individual ticket, or PEEKs a pool.
        const res = await scanToken(raw);

        if (res.type === 'pool' && res.peek) {
          const remaining = res.remaining ?? 0;
          if (remaining <= 0) {
            setResult({ phase: 'already_used', message: 'Everyone on this pass is already in' });
            setPhase('already_used');
            scheduleReset();
            return;
          }
          // Default the picker to admitting the whole group.
          setHeadcount(remaining);
          setResult({ phase: 'pool_headcount', remaining, token: raw });
          setPhase('pool_headcount');
          return; // wait for the staff to confirm a headcount — no auto-reset
        }

        // Individual ticket admitted.
        setResult({ phase: 'admitted', slotNumber: res.slot_number, admitted: res.admitted ?? 1 });
        setPhase('admitted');
        setAdmitted((n) => n + (res.admitted ?? 1));
        scheduleReset();
      } catch (err: unknown) {
        handleScanError(err);
        scheduleReset();
      }
    },
    [scheduleReset],
  );

  // Confirm a pool admission for the selected headcount.
  const confirmAdmit = useCallback(
    async (token: string, count: number) => {
      cooldownRef.current = Date.now() + 5000;
      setPhase('processing');
      try {
        const res = await scanToken(token, { count, idempotencyKey: newScanKey() });
        const justAdmitted = res.admitted ?? count;
        setResult({ phase: 'admitted', admitted: justAdmitted, remaining: res.remaining });
        setPhase('admitted');
        setAdmitted((n) => n + justAdmitted);
      } catch (err: unknown) {
        handleScanError(err);
      }
      scheduleReset();
    },
    [scheduleReset],
  );

  const cancelHeadcount = useCallback(() => {
    setResult(null);
    setPhase('ready');
    cooldownRef.current = 0;
  }, []);

  // Shared error → result mapping.
  const handleScanError = useCallback((err: unknown) => {
    const ax = err as {
      response?: { status?: number; data?: { error?: string; first_scan_at?: string } };
    };
    const status = ax.response?.status;
    const data = ax.response?.data;
    if (status === 409 && data?.error === 'Ticket already scanned') {
      setResult({ phase: 'already_used', firstScanAt: data.first_scan_at });
      setPhase('already_used');
    } else if (status === 409) {
      // e.g. pool fully admitted, outside admission window, booking invalid
      setResult({ phase: 'already_used', message: data?.error });
      setPhase('already_used');
    } else {
      setResult({ phase: 'invalid', message: data?.error ?? 'Ticket could not be verified' });
      setPhase('invalid');
    }
  }, []);

  const showingResult =
    phase === 'admitted' || phase === 'already_used' || phase === 'invalid';
  const showingHeadcount = phase === 'pool_headcount';
  // Pause decoding while a result, the headcount picker, or a verify is on screen.
  const cameraIdle = showingResult || showingHeadcount || phase === 'processing';

  // ── Permission gates ────────────────────────────────────────────────────────
  if (!permission) {
    return <View style={styles.root} />;
  }

  if (!permission.granted) {
    return (
      <View style={[styles.root, styles.permWrap, { paddingTop: insets.top }]}>
        <Header title={title} admitted={admitted} onBack={() => router.back()} />
        <View style={styles.permBody}>
          <View style={styles.permIcon}>
            <Feather name="camera-off" size={30} color={STUDIO.accent} />
          </View>
          <Text style={styles.permTitle}>Camera access needed</Text>
          <Text style={styles.permDesc}>
            The scanner uses your camera to read ticket QR codes at the door.
          </Text>
          {permission.canAskAgain ? (
            <TouchableOpacity style={styles.permBtn} onPress={requestPermission}>
              <Text style={styles.permBtnTxt}>Enable camera</Text>
            </TouchableOpacity>
          ) : (
            <TouchableOpacity style={styles.permBtn} onPress={() => Linking.openSettings()}>
              <Text style={styles.permBtnTxt}>Open settings</Text>
            </TouchableOpacity>
          )}
        </View>
      </View>
    );
  }

  // ── Scanner ─────────────────────────────────────────────────────────────────
  const translateY = scanLine.interpolate({
    inputRange: [0, 1],
    outputRange: [6, FINDER - 8],
  });

  return (
    <View style={styles.root}>
      <CameraView
        style={StyleSheet.absoluteFill}
        facing="back"
        barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
        onBarcodeScanned={
          cameraIdle ? undefined : ({ data }) => handleScan(data)
        }
      />

      {/* Dim overlay */}
      <View style={styles.dim} pointerEvents="none" />

      {/* Header */}
      <View style={{ paddingTop: insets.top }}>
        <Header title={title} admitted={admitted} onBack={() => router.back()} />
      </View>

      {/* Viewfinder */}
      {!showingResult && !showingHeadcount && (
        <View style={styles.finderWrap} pointerEvents="none">
          <View style={styles.finder}>
            {CORNERS.map((c, i) => (
              <View key={i} style={[styles.corner, c]} />
            ))}
            {phase !== 'processing' && (
              <Animated.View style={[styles.scanLine, { transform: [{ translateY }] }]} />
            )}
          </View>
          <Text style={styles.finderHint}>
            {phase === 'processing' ? 'Verifying…' : 'Align the ticket QR within the frame'}
          </Text>
        </View>
      )}

      {/* Result card */}
      {showingResult && result && (
        <View style={styles.resultWrap} pointerEvents="none">
          <View style={styles.resultCard}>
            {result.phase === 'admitted' && (
              <>
                <Badge color={STUDIO.green} bg={STUDIO.greenSoft} icon="check" />
                <Text style={[styles.resultTitle, { color: STUDIO.green }]}>
                  {result.admitted && result.admitted > 1 ? `Admitted ${result.admitted}` : 'Admitted'}
                </Text>
                {result.remaining != null && result.remaining > 0 ? (
                  <Text style={styles.resultSub}>{result.remaining} more left on this pass</Text>
                ) : result.slotNumber != null ? (
                  <Text style={styles.resultSub}>Ticket slot #{result.slotNumber}</Text>
                ) : null}
              </>
            )}
            {result.phase === 'already_used' && (
              <>
                <Badge color={STUDIO.amber} bg={STUDIO.amberSoft} icon="alert-circle" />
                <Text style={[styles.resultTitle, { color: STUDIO.amber }]}>Already used</Text>
                {result.firstScanAt && (
                  <Text style={styles.resultSub}>
                    First scanned at {fmtTime(result.firstScanAt)}
                  </Text>
                )}
              </>
            )}
            {result.phase === 'invalid' && (
              <>
                <Badge color={STUDIO.red} bg={STUDIO.redSoft} icon="x" />
                <Text style={[styles.resultTitle, { color: STUDIO.red }]}>Invalid</Text>
                <Text style={styles.resultSub}>
                  {result.message ?? 'Ticket could not be verified'}
                </Text>
              </>
            )}
            <Text style={styles.resumeTxt}>Resuming scanner…</Text>
          </View>
        </View>
      )}

      {/* Pool headcount picker — interactive, no auto-reset */}
      {showingHeadcount && result?.phase === 'pool_headcount' && (
        <View style={styles.resultWrap}>
          <View style={styles.resultCard}>
            <Badge color={STUDIO.accent} bg={STUDIO.accentSoft} icon="users" />
            <Text style={[styles.resultTitle, { color: '#fff', fontSize: 22 }]}>Group pass</Text>
            <Text style={styles.resultSub}>
              {result.remaining} admission{result.remaining === 1 ? '' : 's'} remaining
            </Text>

            <View style={styles.stepper}>
              <TouchableOpacity
                style={[styles.stepBtn, headcount <= 1 && styles.stepBtnDisabled]}
                onPress={() => setHeadcount((c) => Math.max(1, c - 1))}
                disabled={headcount <= 1}
                hitSlop={8}
              >
                <Feather name="minus" size={22} color="#fff" />
              </TouchableOpacity>
              <Text style={styles.stepValue}>{headcount}</Text>
              <TouchableOpacity
                style={[styles.stepBtn, headcount >= (result.remaining ?? 1) && styles.stepBtnDisabled]}
                onPress={() => setHeadcount((c) => Math.min(result.remaining ?? 1, c + 1))}
                disabled={headcount >= (result.remaining ?? 1)}
                hitSlop={8}
              >
                <Feather name="plus" size={22} color="#fff" />
              </TouchableOpacity>
            </View>

            <Text style={styles.stepHint}>
              {headcount === result.remaining
                ? 'Admitting the whole group'
                : 'Tap − / + if fewer are entering now'}
            </Text>

            <TouchableOpacity
              style={styles.admitBtn}
              onPress={() => confirmAdmit(result.token!, headcount)}
              activeOpacity={0.85}
            >
              <Text style={styles.admitBtnTxt}>Admit {headcount}</Text>
            </TouchableOpacity>
            <TouchableOpacity style={styles.cancelBtn} onPress={cancelHeadcount} hitSlop={8}>
              <Text style={styles.cancelBtnTxt}>Cancel</Text>
            </TouchableOpacity>
          </View>
        </View>
      )}
    </View>
  );
}

// ── Header ────────────────────────────────────────────────────────────────────
function Header({
  title,
  admitted,
  onBack,
}: {
  title?: string;
  admitted: number;
  onBack: () => void;
}) {
  return (
    <View style={styles.header}>
      <TouchableOpacity onPress={onBack} style={styles.hBtn} hitSlop={10}>
        <Feather name="arrow-left" size={20} color="#fff" />
      </TouchableOpacity>
      <View style={{ flex: 1, alignItems: 'center' }}>
        <Text style={styles.hTitle} numberOfLines={1}>
          Scan tickets
        </Text>
        <Text style={styles.hSub} numberOfLines={1}>
          {title ? title : 'All your events'}
        </Text>
      </View>
      <View style={styles.countPill}>
        <Ionicons name="checkmark-circle" size={14} color={STUDIO.green} />
        <Text style={styles.countTxt}>{admitted}</Text>
      </View>
    </View>
  );
}

function Badge({ color, bg, icon }: { color: string; bg: string; icon: keyof typeof Feather.glyphMap }) {
  return (
    <View style={[styles.resultBadge, { backgroundColor: bg }]}>
      <Feather name={icon} size={32} color={color} />
    </View>
  );
}

const CORNERS = [
  { top: -2, left: -2, borderRightWidth: 0, borderBottomWidth: 0, borderTopLeftRadius: 12 },
  { top: -2, right: -2, borderLeftWidth: 0, borderBottomWidth: 0, borderTopRightRadius: 12 },
  { bottom: -2, left: -2, borderRightWidth: 0, borderTopWidth: 0, borderBottomLeftRadius: 12 },
  { bottom: -2, right: -2, borderLeftWidth: 0, borderTopWidth: 0, borderBottomRightRadius: 12 },
];

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: '#000' },
  dim: { ...StyleSheet.absoluteFillObject, backgroundColor: 'rgba(0,0,0,0.35)' },

  header: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 12,
    paddingVertical: 10,
    gap: 8,
  },
  hBtn: {
    width: 38,
    height: 38,
    borderRadius: 19,
    backgroundColor: 'rgba(0,0,0,0.5)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  hTitle: { color: '#fff', fontSize: 16, fontWeight: '800' },
  hSub: { color: 'rgba(255,255,255,0.65)', fontSize: 12, marginTop: 1 },
  countPill: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    backgroundColor: 'rgba(0,0,0,0.5)',
    borderRadius: 999,
    paddingHorizontal: 11,
    paddingVertical: 7,
    minWidth: 52,
    justifyContent: 'center',
  },
  countTxt: { color: '#fff', fontSize: 14, fontWeight: '800' },

  finderWrap: { flex: 1, alignItems: 'center', justifyContent: 'center', marginTop: -40 },
  finder: {
    width: FINDER,
    height: FINDER,
    borderRadius: 14,
  },
  corner: {
    position: 'absolute',
    width: 26,
    height: 26,
    borderWidth: 3,
    borderColor: STUDIO.accent,
  },
  scanLine: {
    position: 'absolute',
    left: 6,
    right: 6,
    height: 2,
    borderRadius: 2,
    backgroundColor: STUDIO.accent,
    shadowColor: STUDIO.accent,
    shadowOpacity: 0.9,
    shadowRadius: 6,
    shadowOffset: { width: 0, height: 0 },
  },
  finderHint: {
    marginTop: 24,
    color: 'rgba(255,255,255,0.85)',
    fontSize: 14,
    fontWeight: '600',
    backgroundColor: 'rgba(0,0,0,0.5)',
    paddingHorizontal: 14,
    paddingVertical: 7,
    borderRadius: 999,
    overflow: 'hidden',
  },

  resultWrap: {
    ...StyleSheet.absoluteFillObject,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: 'rgba(0,0,0,0.6)',
  },
  resultCard: {
    width: '82%',
    maxWidth: 320,
    backgroundColor: STUDIO.bgElevated,
    borderRadius: 22,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: STUDIO.border,
    paddingVertical: 32,
    paddingHorizontal: 28,
    alignItems: 'center',
  },
  resultBadge: {
    width: 70,
    height: 70,
    borderRadius: 35,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 16,
  },
  resultTitle: { fontSize: 26, fontWeight: '800', letterSpacing: -0.5 },
  resultSub: {
    color: STUDIO.sub,
    fontSize: 13.5,
    marginTop: 8,
    textAlign: 'center',
    lineHeight: 19,
  },
  resumeTxt: { color: STUDIO.faint, fontSize: 12, marginTop: 18 },

  // ── Pool headcount picker ───────────────────────────────────────────────────
  stepper: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 22,
    marginTop: 20,
  },
  stepBtn: {
    width: 52,
    height: 52,
    borderRadius: 26,
    backgroundColor: 'rgba(255,255,255,0.08)',
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: STUDIO.border,
    alignItems: 'center',
    justifyContent: 'center',
  },
  stepBtnDisabled: { opacity: 0.3 },
  stepValue: {
    color: '#fff',
    fontSize: 40,
    fontWeight: '800',
    minWidth: 54,
    textAlign: 'center',
  },
  stepHint: {
    color: STUDIO.faint,
    fontSize: 12.5,
    marginTop: 12,
    textAlign: 'center',
  },
  admitBtn: {
    marginTop: 22,
    backgroundColor: STUDIO.green,
    borderRadius: 14,
    paddingVertical: 15,
    width: '100%',
    alignItems: 'center',
  },
  admitBtnTxt: { color: '#03281c', fontSize: 16, fontWeight: '800' },
  cancelBtn: { marginTop: 12, paddingVertical: 6 },
  cancelBtnTxt: { color: STUDIO.sub, fontSize: 14, fontWeight: '600' },

  // Permission
  permWrap: { backgroundColor: STUDIO.bg },
  permBody: { flex: 1, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 36, gap: 10 },
  permIcon: {
    width: 76,
    height: 76,
    borderRadius: 24,
    backgroundColor: STUDIO.accentSoft,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 6,
  },
  permTitle: { color: STUDIO.text, fontSize: 19, fontWeight: '800' },
  permDesc: { color: STUDIO.muted, fontSize: 14, textAlign: 'center', lineHeight: 20 },
  permBtn: {
    marginTop: 12,
    backgroundColor: STUDIO.accentDeep,
    borderRadius: 14,
    paddingHorizontal: 28,
    paddingVertical: 14,
  },
  permBtnTxt: { color: '#fff', fontSize: 15, fontWeight: '800' },
});