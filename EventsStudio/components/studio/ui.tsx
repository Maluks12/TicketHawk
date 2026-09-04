// ─────────────────────────────────────────────────────────────────────────────
// Organiser Studio — shared UI primitives
// Small, presentational building blocks reused across the Studio screens.
// Every one of them reads the active palette through makeStyles(), so they
// repaint with the theme instead of baking in the dark surface.
// ─────────────────────────────────────────────────────────────────────────────
import React from 'react';
import { View, Text, ActivityIndicator, StyleSheet, TouchableOpacity } from 'react-native';
import { Ionicons } from '@expo/vector-icons';
import {
  makeStyles,
  useStudioTheme,
  accentPair,
  statusChip,
  type AccentKey,
} from './theme';
import { useThemeStore } from '../../store/themeStore';

// ── Status chip ───────────────────────────────────────────────────────────────
export function Chip({ status, label }: { status: string; label?: string }) {
  const { S } = useStudioTheme();
  const ui = useUi();
  const c = statusChip(status, S);
  return (
    <View style={[ui.chip, { backgroundColor: c.bg }]}>
      <View style={[ui.chipDot, { backgroundColor: c.fg }]} />
      <Text style={[ui.chipText, { color: c.fg }]}>{label ?? c.label}</Text>
    </View>
  );
}

// ── Progress bar ──────────────────────────────────────────────────────────────
export function ProgressBar({
  pct,
  color,
  height = 6,
}: {
  pct: number;
  color?: string;
  height?: number;
}) {
  const { S } = useStudioTheme();
  const ui = useUi();
  const clamped = Math.max(0, Math.min(100, pct));
  return (
    <View style={[ui.track, { height, borderRadius: height }]}>
      <View
        style={{
          width: `${clamped}%`,
          height: '100%',
          backgroundColor: color ?? S.accent,
          borderRadius: height,
        }}
      />
    </View>
  );
}

// ── Stat card ─────────────────────────────────────────────────────────────────
export function StatCard({
  label,
  value,
  sub,
  accent = 'accent',
  icon,
}: {
  label: string;
  value: string | number;
  sub?: string;
  accent?: AccentKey;
  icon?: React.ReactNode;
}) {
  const { S } = useStudioTheme();
  const ui = useUi();
  const a = accentPair(S, accent);
  return (
    <View style={ui.statCard}>
      <View style={ui.statTop}>
        <Text style={ui.statLabel} numberOfLines={1}>
          {label.toUpperCase()}
        </Text>
        {icon ? (
          <View style={[ui.statIcon, { backgroundColor: a.soft }]}>{icon}</View>
        ) : null}
      </View>
      <Text style={ui.statValue} numberOfLines={1}>
        {value}
      </Text>
      {sub ? (
        <Text style={ui.statSub} numberOfLines={1}>
          {sub}
        </Text>
      ) : null}
    </View>
  );
}

// ── Card shell with optional header ───────────────────────────────────────────
export function Card({
  title,
  action,
  children,
  noPad,
}: {
  title?: string;
  action?: React.ReactNode;
  children: React.ReactNode;
  noPad?: boolean;
}) {
  const ui = useUi();
  return (
    <View style={ui.card}>
      {(title || action) && (
        <View style={ui.cardHeader}>
          {title ? <Text style={ui.cardTitle}>{title}</Text> : <View />}
          {action}
        </View>
      )}
      <View style={noPad ? undefined : ui.cardBody}>{children}</View>
    </View>
  );
}

// ── Loading / empty ───────────────────────────────────────────────────────────
export function Loader({ label }: { label?: string }) {
  const { S } = useStudioTheme();
  const ui = useUi();
  return (
    <View style={ui.center}>
      <ActivityIndicator color={S.accent} />
      {label ? <Text style={ui.centerSub}>{label}</Text> : null}
    </View>
  );
}

export function Empty({
  icon,
  title,
  desc,
}: {
  icon?: React.ReactNode;
  title: string;
  desc?: string;
}) {
  const ui = useUi();
  return (
    <View style={ui.empty}>
      {icon ? <View style={ui.emptyIcon}>{icon}</View> : null}
      <Text style={ui.emptyTitle}>{title}</Text>
      {desc ? <Text style={ui.emptyDesc}>{desc}</Text> : null}
    </View>
  );
}

// ── Appearance toggle ─────────────────────────────────────────────────────────
//
// One tap cycles auto → light → dark → auto. The icon states which mode is
// active rather than which one the tap will bring, and the accessibility label
// says both, because an icon alone is a guess.
export function ThemeToggle({ size = 36 }: { size?: number }) {
  const { S, mode, isDark } = useStudioTheme();
  const cycleMode = useThemeStore((s) => s.cycleMode);
  const ui = useUi();

  const icon =
    mode === 'auto' ? 'phone-portrait-outline'
    : mode === 'light' ? 'sunny-outline'
    : 'moon-outline';

  const next = mode === 'auto' ? 'light' : mode === 'light' ? 'dark' : 'automatic';

  return (
    <TouchableOpacity
      onPress={cycleMode}
      hitSlop={10}
      activeOpacity={0.7}
      accessibilityRole="button"
      accessibilityLabel={`Appearance: ${mode === 'auto' ? `automatic (${isDark ? 'dark' : 'light'})` : mode}. Tap for ${next}.`}
      style={[ui.toggle, { width: size, height: size, borderRadius: size / 2 }]}
    >
      <Ionicons name={icon} size={Math.round(size * 0.5)} color={S.text} />
    </TouchableOpacity>
  );
}

const useUi = makeStyles((S) => ({
  chip: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    paddingHorizontal: 9,
    paddingVertical: 4,
    borderRadius: 999,
    alignSelf: 'flex-start',
  },
  chipDot: { width: 6, height: 6, borderRadius: 3 },
  chipText: { fontSize: 11, fontWeight: '700', letterSpacing: 0.2 },

  track: {
    width: '100%',
    backgroundColor: S.track,
    overflow: 'hidden',
  },

  statCard: {
    flex: 1,
    minWidth: 0,
    backgroundColor: S.surface,
    borderRadius: 16,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: S.border,
    padding: 14,
  },
  statTop: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: 10,
  },
  statLabel: {
    fontSize: 10,
    fontWeight: '800',
    letterSpacing: 1,
    color: S.muted,
    flex: 1,
    marginRight: 6,
  },
  statIcon: {
    width: 26,
    height: 26,
    borderRadius: 8,
    alignItems: 'center',
    justifyContent: 'center',
  },
  statValue: {
    fontSize: 23,
    fontWeight: '800',
    color: S.text,
    letterSpacing: -0.5,
  },
  statSub: { fontSize: 11.5, color: S.muted, marginTop: 3 },

  card: {
    backgroundColor: S.surface,
    borderRadius: 18,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: S.border,
    overflow: 'hidden',
  },
  cardHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 16,
    paddingTop: 14,
    paddingBottom: 12,
  },
  cardTitle: { fontSize: 15, fontWeight: '800', color: S.text },
  cardBody: { paddingHorizontal: 16, paddingBottom: 14 },

  center: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    paddingVertical: 60,
    gap: 12,
  },
  centerSub: { fontSize: 13, color: S.muted },

  empty: { alignItems: 'center', paddingVertical: 44, paddingHorizontal: 32, gap: 8 },
  emptyIcon: {
    width: 64,
    height: 64,
    borderRadius: 20,
    backgroundColor: S.surfaceStrong,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: S.border,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 4,
  },
  emptyTitle: {
    fontSize: 16,
    fontWeight: '800',
    color: S.text,
    textAlign: 'center',
  },
  emptyDesc: {
    fontSize: 13,
    color: S.muted,
    textAlign: 'center',
    lineHeight: 19,
  },

  toggle: {
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: S.surfaceStrong,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: S.border,
  },
}));