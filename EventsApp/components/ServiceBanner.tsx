import React, { useEffect, useState } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { StatusBar } from 'expo-status-bar';
import Svg, { Path } from 'react-native-svg';
import { SafeAreaInsetsContext, useSafeAreaInsets } from 'react-native-safe-area-context';
import { useAppTheme } from '../theme/colors';
import { useStatusStore, startStatusMonitor, probeNow, type ServiceState } from '../store/statusStore';

// ─────────────────────────────────────────────────────────────────────────────
// The banner every screen shows when TicketHawk is not itself.
//
// Used in app/_layout.tsx as a WRAPPER around the root <Stack>, not as a
// sibling dropped on top of it, because of the notch.
//
// ── The notch problem, and why this wraps the navigator ──────────────────────
//
// Every screen in this app pads its own top edge with useSafeAreaInsets().top.
// A banner placed above the navigator has to sit in that unsafe area itself —
// and then every screen below it would STILL pad by the full notch height,
// leaving a ~50px empty strip between the banner and the content.
//
// useSafeAreaInsets() reads from SafeAreaInsetsContext. So while the banner is
// showing, the navigator is rendered inside a provider that reports top: 0 —
// the banner has already consumed the notch — and every screen lays out
// correctly without a single one of them being edited. When the banner goes
// away, the real insets flow through again. The two screens that use the
// native <SafeAreaView> (search, merch) need no help: that component measures
// its own position, and no longer touches the notch while the banner is up.
//
// Renders only its children (plus one View) when the platform is healthy.
// ─────────────────────────────────────────────────────────────────────────────

type Tone = { dot: string; bg: string; border: string };

// themes/colors.ts has no semantic status colours (the palette is black, white
// and the pink CTA), so the tones live here. Alpha tints composite correctly on
// both the black and the white background; text comes from the theme. Pink is
// deliberately not the error colour — it is the "buy" colour everywhere else.
const TONES: Record<Exclude<ServiceState, 'ok'> | 'recovered', Tone> = {
  maintenance: { dot: '#FFB020', bg: 'rgba(255,176,32,0.13)', border: 'rgba(255,176,32,0.32)' },
  degraded:    { dot: '#FFB020', bg: 'rgba(255,176,32,0.13)', border: 'rgba(255,176,32,0.32)' },
  down:        { dot: '#FF453A', bg: 'rgba(255,69,58,0.12)',  border: 'rgba(255,69,58,0.30)'  },
  offline:     { dot: '#8E8E93', bg: 'rgba(142,142,147,0.15)', border: 'rgba(142,142,147,0.30)' },
  recovered:   { dot: '#30D158', bg: 'rgba(48,209,88,0.13)',  border: 'rgba(48,209,88,0.30)'  },
};

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
  const { colors, isDark } = useAppTheme();

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
  const visible = showRecovered || (state !== 'ok' && dismissed !== key);

  let banner: React.ReactNode = null;

  if (visible) {
    const tone = TONES[showRecovered ? 'recovered' : (state as Exclude<ServiceState, 'ok'>)];

    let text: string;
    let detail = '';

    if (showRecovered) {
      text = 'Back online.';
    } else if (state === 'maintenance') {
      text = message || "TicketHawk is under maintenance. Some things won't work until it's done.";
      detail = endsAtLabel(endsAt);
    } else if (state === 'degraded') {
      text = "Some parts of TicketHawk aren't loading. We're working on it.";
    } else if (state === 'offline') {
      // True, not reassurance for its own sake: lib/offline.ts caches every
      // ticket that has been opened online, QR tokens included.
      text = "You're offline. Tickets you've opened before still work.";
    } else {
      text = "Can't reach TicketHawk right now. Tickets you've opened before still work.";
    }

    const canRetry = state === 'down' || state === 'offline' || state === 'degraded';

    banner = (
      <View
        accessibilityLiveRegion="polite"
        style={[
          styles.wrap,
          {
            paddingTop: insets.top + 8,
            backgroundColor: colors.background,
            borderBottomColor: tone.border,
          },
        ]}
      >
        {/* The banner now owns the strip under the clock, so it also owns the
            status-bar icon colour while it is up. */}
        <StatusBar style={isDark ? 'light' : 'dark'} />

        {/* Tint layered over the solid theme background, so it reads the same
            whatever screen is underneath. */}
        <View pointerEvents="none" style={[StyleSheet.absoluteFill, { backgroundColor: tone.bg }]} />

        <View style={styles.row}>
          <View style={[styles.dot, { backgroundColor: tone.dot }]} />

          <Text style={[styles.text, { color: colors.text }]}>
            {text}
            {detail ? <Text style={{ color: colors.subText }}> {detail}</Text> : null}
          </Text>

          {canRetry && (
            <Pressable
              onPress={() => { void probeNow(); }}
              disabled={checking}
              hitSlop={8}
              accessibilityRole="button"
              accessibilityLabel="Retry connection"
              style={[styles.retry, { backgroundColor: colors.sectionCardBg, opacity: checking ? 0.6 : 1 }]}
            >
              <Text style={[styles.retryText, { color: colors.text }]}>
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
                <Path d="M18 6 6 18M6 6l12 12" stroke={colors.iconColor} strokeWidth={2.2} strokeLinecap="round" />
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
  retry: { paddingHorizontal: 12, paddingVertical: 5, borderRadius: 99 },
  retryText: { fontSize: 12, fontWeight: '700', letterSpacing: 0.3 },
  close: { padding: 2 },
});