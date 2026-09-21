import React, { useEffect, useState } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { StatusBar } from 'expo-status-bar';
import { usePathname } from 'expo-router';
import Svg, { Path } from 'react-native-svg';
import { SafeAreaInsetsContext, useSafeAreaInsets } from 'react-native-safe-area-context';
import { useStudioTheme, type StudioPalette } from './theme';
import { useStatusStore, startStatusMonitor, probeNow, type ServiceState } from '../../store/statusStore';

// ─────────────────────────────────────────────────────────────────────────────
// Studio's service banner — the same notice the consumer app shows, in the
// Studio's own palette and written for organisers and door staff.
//
// Wraps the root <Stack> in app/_layout.tsx. Why it wraps rather than sits on
// top is explained in EventsApp/components/ServiceBanner.tsx: while the banner
// owns the notch, the navigator is given insets with top: 0 so no screen pads
// the notch a second time. Every Studio screen reads useSafeAreaInsets(), so
// that one override covers all of them.
//
// ── Not on the scanner ───────────────────────────────────────────────────────
//
// The scan screen already carries its own online/offline pill, driven by the
// sync loop that actually matters at a gate. A second connectivity notice over
// a live camera feed would cost viewfinder height at the worst possible moment
// and tell the operator nothing the pill does not. Scanning is offline-first by
// design (lib/scanner/sync.ts: the scan path never touches the network), so an
// outage is not news on that screen. Everywhere else, it is.
// ─────────────────────────────────────────────────────────────────────────────

const HIDDEN_ON = new Set(['/organiser/scan']);

type Tone = { dot: string; tint: string; border: string };

function toneFor(key: Exclude<ServiceState, 'ok'> | 'recovered', S: StudioPalette): Tone {
  switch (key) {
    case 'maintenance':
    case 'degraded':  return { dot: S.amber, tint: S.amberSoft,     border: S.amberSoft };
    case 'down':      return { dot: S.red,   tint: S.redSoft,       border: S.redSoft   };
    case 'recovered': return { dot: S.green, tint: S.greenSoft,     border: S.greenSoft };
    default:          return { dot: S.muted, tint: S.surfaceStrong, border: S.border    };
  }
}

/** "Until 02:00." — rendered against the SERVER's clock, not the phone's. */
function endsAtLabel(iso: string | null): string {
  if (!iso) return '';
  const t = new Date(iso);
  if (Number.isNaN(t.getTime()) || t.getTime() < Date.now()) return '';
  const hh = String(t.getHours()).padStart(2, '0');
  const mm = String(t.getMinutes()).padStart(2, '0');
  return `Until ${hh}:${mm}.`;
}

export default function ServiceBanner({ children }: { children: React.ReactNode }) {
  const insets = useSafeAreaInsets();
  const { S, isDark } = useStudioTheme();
  const pathname = usePathname();

  const state     = useStatusStore((s) => s.state);
  const message   = useStatusStore((s) => s.message);
  const endsAt    = useStatusStore((s) => s.endsAt);
  const revision  = useStatusStore((s) => s.revision);
  const checking  = useStatusStore((s) => s.checking);
  const recovered = useStatusStore((s) => s.recovered);

  // Keyed to the NOTICE: dismissing today's maintenance notice must not also
  // silence tomorrow's outage.
  const [dismissed, setDismissed] = useState<string | null>(null);

  useEffect(() => { startStatusMonitor(); }, []);

  const showRecovered = state === 'ok' && recovered;
  const key = `${state}:${revision}`;
  const visible =
    !HIDDEN_ON.has(pathname) &&
    (showRecovered || (state !== 'ok' && dismissed !== key));

  let banner: React.ReactNode = null;

  if (visible) {
    const tone = toneFor(showRecovered ? 'recovered' : (state as Exclude<ServiceState, 'ok'>), S);

    let text: string;
    let detail = '';

    if (showRecovered) {
      text = 'Back online.';
    } else if (state === 'maintenance') {
      text = message || "TicketHawk is under maintenance. Some actions won't work until it's done.";
      detail = endsAtLabel(endsAt);
    } else if (state === 'degraded') {
      text = "Some parts of TicketHawk aren't loading. We're working on it.";
    } else if (state === 'offline') {
      // Accurate, not soothing: scans queue in the local SQLite db and the
      // sync loop pushes them (idempotently) once there is a connection.
      text = "You're offline. Scans are saved on this phone and sync when you reconnect.";
    } else {
      text = "Can't reach TicketHawk right now. Scans are saved on this phone and sync when it's back.";
    }

    const canRetry = state === 'down' || state === 'offline' || state === 'degraded';

    banner = (
      <View
        accessibilityLiveRegion="polite"
        style={[
          styles.wrap,
          { paddingTop: insets.top + 8, backgroundColor: S.bg, borderBottomColor: tone.border },
        ]}
      >
        <StatusBar style={isDark ? 'light' : 'dark'} />

        <View pointerEvents="none" style={[StyleSheet.absoluteFill, { backgroundColor: tone.tint }]} />

        <View style={styles.row}>
          <View style={[styles.dot, { backgroundColor: tone.dot }]} />

          <Text style={[styles.text, { color: S.text }]}>
            {text}
            {detail ? <Text style={{ color: S.sub }}> {detail}</Text> : null}
          </Text>

          {canRetry && (
            <Pressable
              onPress={() => { void probeNow(); }}
              disabled={checking}
              hitSlop={8}
              accessibilityRole="button"
              accessibilityLabel="Retry connection"
              style={[
                styles.retry,
                { backgroundColor: S.btnSecondaryBg, borderColor: S.border, opacity: checking ? 0.6 : 1 },
              ]}
            >
              <Text style={[styles.retryText, { color: S.text }]}>
                {checking ? 'Checking…' : 'Retry'}
              </Text>
            </Pressable>
          )}

          {/* No Dismiss on "Back online" — it removes itself in a few seconds. */}
          {!showRecovered && (
            <Pressable
              onPress={() => setDismissed(key)}
              hitSlop={12}
              accessibilityRole="button"
              accessibilityLabel="Dismiss"
              style={styles.close}
            >
              <Svg width={14} height={14} viewBox="0 0 24 24" fill="none">
                <Path d="M18 6 6 18M6 6l12 12" stroke={S.sub} strokeWidth={2.2} strokeLinecap="round" />
              </Svg>
            </Pressable>
          )}
        </View>
      </View>
    );
  }

  return (
    <View style={styles.root}>
      {banner}
      <SafeAreaInsetsContext.Provider value={visible ? { ...insets, top: 0 } : insets}>
        {children}
      </SafeAreaInsetsContext.Provider>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1 },
  wrap: {
    paddingBottom: 9,
    paddingHorizontal: 16,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  row: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  dot: { width: 8, height: 8, borderRadius: 4 },
  text: { flex: 1, fontSize: 13, lineHeight: 18 },
  retry: { paddingHorizontal: 12, paddingVertical: 5, borderRadius: 99, borderWidth: StyleSheet.hairlineWidth },
  retryText: { fontSize: 12, fontWeight: '700', letterSpacing: 0.3 },
  close: { padding: 2 },
});