import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, FlatList,
  ActivityIndicator, StyleSheet, KeyboardAvoidingView, Platform, Alert,
} from 'react-native';
import { useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { CameraView, useCameraPermissions } from 'expo-camera';
import { STUDIO } from '../../components/studio/theme';
import { redeemStaffCode, listMyScanEvents, type ScannableEvent } from '../../lib/staff';
import { useAuthStore } from '../../store/authStore';

// ─────────────────────────────────────────────────────────────────────────────
// Join event staff — the staffer's single entry point.
//
// Two equal ways in, side by side: type the join code the organiser reads out,
// or scan the QR the organiser shows (the QR encodes the same code, so both
// paths hit the same /v1/staff/redeem endpoint). Every event the staffer can
// scan is listed below with a direct route into the scanner. Deliberately
// minimal: this is a tool screen, not a consumer surface — staff see nothing
// but their events and a way to add more.
// ─────────────────────────────────────────────────────────────────────────────

// Join codes are Crockford base32 shown as XXXX-XXXX. Accept with or without
// the dash, any case, and ignore surrounding junk (a QR might be scanned with
// stray whitespace).
function normaliseCode(raw: string): string | null {
  const cleaned = raw.trim().toUpperCase().replace(/\s+/g, '');
  return /^[0-9A-Z]{4}-?[0-9A-Z]{4}$/.test(cleaned) ? cleaned : null;
}

export default function JoinEventStaff() {
  const insets = useSafeAreaInsets();
  const router = useRouter();

  const [code, setCode] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [feedback, setFeedback] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null);

  const [events, setEvents] = useState<ScannableEvent[]>([]);
  const [loadingEvents, setLoadingEvents] = useState(true);

  // QR scan state. scannedRef debounces the camera callback, which fires many
  // times per second while a code is in frame.
  const [scanning, setScanning] = useState(false);
  const scannedRef = useRef(false);
  const [permission, requestPermission] = useCameraPermissions();

  const loadEvents = useCallback(async () => {
    try {
      const { data } = await listMyScanEvents();
      setEvents(data.events ?? []);
    } catch {
      /* offline or error — keep whatever is shown */
    } finally {
      setLoadingEvents(false);
    }
  }, []);

  useEffect(() => { loadEvents(); }, [loadEvents]);

  // Shared redemption path for BOTH entry methods.
  const redeemValue = async (value: string) => {
    setSubmitting(true);
    setFeedback(null);
    try {
      const { data } = await redeemStaffCode(value);
      setFeedback({ kind: 'ok', text: `You can now scan tickets for “${data.event.title}”.` });
      setCode('');
      loadEvents();
    } catch (e: any) {
      setFeedback({
        kind: 'err',
        text: e?.response?.data?.error ?? 'Could not redeem the code. Check your connection and try again.',
      });
    } finally {
      setSubmitting(false);
    }
  };

  const redeem = () => {
    const trimmed = code.trim();
    if (trimmed) redeemValue(trimmed);
  };

  const startScan = async () => {
    if (!permission?.granted) {
      const res = await requestPermission();
      if (!res.granted) {
        setFeedback({
          kind: 'err',
          text: 'Camera access is needed to scan a join QR. You can still type the code instead.',
        });
        return;
      }
    }
    scannedRef.current = false;
    setFeedback(null);
    setScanning(true);
  };

  const onBarcodeScanned = ({ data }: { data: string }) => {
    if (scannedRef.current) return;
    const normalised = normaliseCode(data);
    if (!normalised) return; // not a join code (someone scanned a random QR) — keep looking
    scannedRef.current = true;
    setScanning(false);
    setCode(normalised);
    redeemValue(normalised);
  };

  const openScanner = (ev: ScannableEvent) =>
    router.push({ pathname: '/organiser/scan', params: { eventId: ev.id, title: ev.title } } as any);

  // For organisers arriving from the dashboard this is a plain back. For door
  // staff this screen IS home (nothing to go back to), so the same button
  // becomes sign-out.
  const { logout } = useAuthStore();
  const leave = () => {
    if (router.canGoBack()) {
      router.back();
      return;
    }
    Alert.alert('Sign out', 'Sign out of TicketHawk Studio on this device?', [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'Sign out',
        style: 'destructive',
        onPress: async () => {
          try {
            await logout();
          } finally {
            router.replace('/auth/login' as any);
          }
        },
      },
    ]);
  };

  const roleBadge = (role: ScannableEvent['role']) =>
    role === 'owner' ? 'OWNER' : role === 'manager' ? 'MANAGER' : 'SCANNER';

  // ── Full-screen QR capture ──────────────────────────────────────────────────
  if (scanning) {
    return (
      <View style={styles.camRoot}>
        <CameraView
          style={StyleSheet.absoluteFill}
          facing="back"
          barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
          onBarcodeScanned={onBarcodeScanned}
        />
        <View style={[styles.camTopBar, { paddingTop: insets.top + 8 }]}>
          <TouchableOpacity onPress={() => setScanning(false)} hitSlop={12} style={styles.camClose}>
            <Ionicons name="close" size={22} color="#fff" />
          </TouchableOpacity>
          <Text style={styles.camTitle}>Scan join code</Text>
          <View style={{ width: 38 }} />
        </View>
        <View style={styles.camFrameWrap} pointerEvents="none">
          <View style={styles.camFrame} />
          <Text style={styles.camHint}>
            Point at the QR the organiser shows you
          </Text>
        </View>
      </View>
    );
  }

  return (
    <View style={[styles.root, { paddingTop: insets.top + 8 }]}>
      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity onPress={leave} hitSlop={12} style={styles.backBtn}>
          <Ionicons
            name={router.canGoBack() ? 'chevron-back' : 'log-out-outline'}
            size={22}
            color={STUDIO.text}
          />
        </TouchableOpacity>
        <Text style={styles.title}>Event staff</Text>
        <View style={{ width: 34 }} />
      </View>

      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1 }}>
        {/* Join an event — two equal paths, side by side */}
        <View style={styles.card}>
          <Text style={styles.cardTitle}>Join an event</Text>
          <Text style={styles.cardSub}>
            Enter the organiser's join code, or scan the QR they show you.
          </Text>

          <View style={styles.joinRow}>
            {/* Left: typed code */}
            <View style={{ flex: 1 }}>
              <TextInput
                style={styles.input}
                value={code}
                onChangeText={setCode}
                placeholder="7KQ4-M2XR"
                placeholderTextColor={STUDIO.muted}
                autoCapitalize="characters"
                autoCorrect={false}
                maxLength={12}
                onSubmitEditing={redeem}
                returnKeyType="go"
              />
              <TouchableOpacity
                style={[styles.joinBtn, (!code.trim() || submitting) && { opacity: 0.5 }]}
                onPress={redeem}
                disabled={!code.trim() || submitting}
                activeOpacity={0.85}
              >
                {submitting
                  ? <ActivityIndicator color="#fff" size="small" />
                  : <Text style={styles.joinBtnTxt}>Join</Text>}
              </TouchableOpacity>
            </View>

            {/* Divider */}
            <View style={styles.orCol}>
              <View style={styles.orLine} />
              <Text style={styles.orTxt}>OR</Text>
              <View style={styles.orLine} />
            </View>

            {/* Right: scan the organiser's QR */}
            <TouchableOpacity
              style={[styles.scanQrBtn, submitting && { opacity: 0.5 }]}
              onPress={startScan}
              disabled={submitting}
              activeOpacity={0.85}
            >
              <Ionicons name="qr-code-outline" size={30} color={STUDIO.accent} />
              <Text style={styles.scanQrTxt}>Scan QR</Text>
            </TouchableOpacity>
          </View>

          {feedback && (
            <Text style={[styles.feedback, { color: feedback.kind === 'ok' ? STUDIO.green : STUDIO.red }]}>
              {feedback.text}
            </Text>
          )}
        </View>

        {/* Events I can scan */}
        <Text style={styles.sectionLabel}>EVENTS YOU CAN SCAN</Text>
        {loadingEvents ? (
          <ActivityIndicator color={STUDIO.sub} style={{ marginTop: 24 }} />
        ) : events.length === 0 ? (
          <Text style={styles.empty}>
            Nothing yet. Enter or scan a join code above, and events you're staffing will appear here.
          </Text>
        ) : (
          <FlatList
            data={events}
            keyExtractor={(e) => e.id}
            contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}
            renderItem={({ item }) => (
              <TouchableOpacity style={styles.eventRow} activeOpacity={0.8} onPress={() => openScanner(item)}>
                <View style={{ flex: 1 }}>
                  <Text style={styles.eventTitle} numberOfLines={1}>{item.title}</Text>
                  <Text style={styles.eventMeta} numberOfLines={1}>
                    {new Date(item.start_time).toLocaleString()}
                    {item.venue_name ? ` · ${item.venue_name}` : ''}
                  </Text>
                </View>
                <View style={styles.badge}>
                  <Text style={styles.badgeTxt}>{roleBadge(item.role)}</Text>
                </View>
                <Ionicons name="qr-code-outline" size={20} color={STUDIO.accent} style={{ marginLeft: 10 }} />
              </TouchableOpacity>
            )}
          />
        )}
      </KeyboardAvoidingView>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: STUDIO.bg, paddingHorizontal: 16 },
  header: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginBottom: 14 },
  backBtn: {
    width: 34, height: 34, borderRadius: 17, alignItems: 'center', justifyContent: 'center',
    backgroundColor: STUDIO.surface,
  },
  title: { color: STUDIO.text, fontSize: 17, fontWeight: '700' },

  card: {
    backgroundColor: STUDIO.surface, borderRadius: 16, padding: 16,
    borderWidth: 1, borderColor: STUDIO.border, marginBottom: 22,
  },
  cardTitle: { color: STUDIO.text, fontSize: 15, fontWeight: '700', marginBottom: 4 },
  cardSub: { color: STUDIO.sub, fontSize: 12.5, lineHeight: 18, marginBottom: 14 },

  joinRow: { flexDirection: 'row', alignItems: 'stretch', gap: 10 },
  input: {
    backgroundColor: STUDIO.bgElevated, borderRadius: 10, borderWidth: 1,
    borderColor: STUDIO.borderStrong, color: STUDIO.text, paddingHorizontal: 14,
    paddingVertical: 12, fontSize: 15, letterSpacing: 1.5, fontWeight: '600',
  },
  joinBtn: {
    backgroundColor: STUDIO.accentDeep, borderRadius: 10, paddingVertical: 11,
    alignItems: 'center', justifyContent: 'center', marginTop: 8,
  },
  joinBtnTxt: { color: '#fff', fontWeight: '700', fontSize: 15 },

  orCol: { alignItems: 'center', justifyContent: 'center', gap: 6 },
  orLine: { flex: 1, width: 1, backgroundColor: STUDIO.border },
  orTxt: { color: STUDIO.muted, fontSize: 10, fontWeight: '800', letterSpacing: 1 },

  scanQrBtn: {
    width: 104, borderRadius: 10, borderWidth: 1, borderColor: STUDIO.accentBorder,
    backgroundColor: STUDIO.accentSoft, alignItems: 'center', justifyContent: 'center', gap: 6,
  },
  scanQrTxt: { color: STUDIO.accent, fontWeight: '700', fontSize: 13 },

  feedback: { marginTop: 12, fontSize: 12.5, lineHeight: 18 },

  sectionLabel: { color: STUDIO.muted, fontSize: 11, fontWeight: '700', letterSpacing: 1.2, marginBottom: 10 },
  empty: { color: STUDIO.sub, fontSize: 13, lineHeight: 19, marginTop: 6 },

  eventRow: {
    flexDirection: 'row', alignItems: 'center', backgroundColor: STUDIO.surface,
    borderRadius: 14, borderWidth: 1, borderColor: STUDIO.border,
    paddingHorizontal: 14, paddingVertical: 13, marginBottom: 10,
  },
  eventTitle: { color: STUDIO.text, fontSize: 14.5, fontWeight: '600' },
  eventMeta: { color: STUDIO.sub, fontSize: 12, marginTop: 3 },
  badge: {
    backgroundColor: STUDIO.accentSoft, borderColor: STUDIO.accentBorder, borderWidth: 1,
    borderRadius: 6, paddingHorizontal: 7, paddingVertical: 3, marginLeft: 8,
  },
  badgeTxt: { color: STUDIO.accent, fontSize: 10, fontWeight: '800', letterSpacing: 0.6 },

  // Camera capture
  camRoot: { flex: 1, backgroundColor: '#000' },
  camTopBar: {
    position: 'absolute', top: 0, left: 0, right: 0, zIndex: 2,
    flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between',
    paddingHorizontal: 16, paddingBottom: 10, backgroundColor: 'rgba(0,0,0,0.45)',
  },
  camClose: {
    width: 38, height: 38, borderRadius: 19, alignItems: 'center', justifyContent: 'center',
    backgroundColor: 'rgba(255,255,255,0.14)',
  },
  camTitle: { color: '#fff', fontSize: 16, fontWeight: '700' },
  camFrameWrap: { flex: 1, alignItems: 'center', justifyContent: 'center' },
  camFrame: {
    width: 230, height: 230, borderRadius: 22, borderWidth: 2.5,
    borderColor: 'rgba(124,140,255,0.95)',
  },
  camHint: {
    color: 'rgba(255,255,255,0.85)', fontSize: 13.5, marginTop: 18,
    textAlign: 'center', paddingHorizontal: 40,
  },
});