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
import { extractAssignmentId, scanTicket, fmtTime } from '../../lib/organiser';

type Phase =
  | 'ready'
  | 'processing'
  | 'admitted'
  | 'already_used'
  | 'invalid';

interface Result {
  phase: 'admitted' | 'already_used' | 'invalid';
  slotNumber?: number;
  firstScanAt?: string;
  message?: string;
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

      let assignmentId: string;
      try {
        assignmentId = extractAssignmentId(raw);
      } catch {
        setResult({ phase: 'invalid', message: 'Not a valid ticket QR code' });
        setPhase('invalid');
        scheduleReset();
        return;
      }

      setPhase('processing');
      try {
        const res = await scanTicket(assignmentId, raw);
        setResult({ phase: 'admitted', slotNumber: res.slot_number });
        setPhase('admitted');
        setAdmitted((n) => n + 1);
      } catch (err: unknown) {
        const ax = err as {
          response?: { status?: number; data?: { error?: string; first_scan_at?: string } };
        };
        const status = ax.response?.status;
        const data = ax.response?.data;
        if (status === 409 && data?.error === 'Ticket already scanned') {
          setResult({ phase: 'already_used', firstScanAt: data.first_scan_at });
          setPhase('already_used');
        } else {
          setResult({
            phase: 'invalid',
            message: data?.error ?? 'Ticket could not be verified',
          });
          setPhase('invalid');
        }
      }
      scheduleReset();
    },
    [scheduleReset],
  );

  const showingResult =
    phase === 'admitted' || phase === 'already_used' || phase === 'invalid';

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
          showingResult ? undefined : ({ data }) => handleScan(data)
        }
      />

      {/* Dim overlay */}
      <View style={styles.dim} pointerEvents="none" />

      {/* Header */}
      <View style={{ paddingTop: insets.top }}>
        <Header title={title} admitted={admitted} onBack={() => router.back()} />
      </View>

      {/* Viewfinder */}
      {!showingResult && (
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
                <Text style={[styles.resultTitle, { color: STUDIO.green }]}>Admitted</Text>
                {result.slotNumber != null && (
                  <Text style={styles.resultSub}>Ticket slot #{result.slotNumber}</Text>
                )}
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