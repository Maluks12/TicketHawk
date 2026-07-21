// ─────────────────────────────────────────────────────────────────────────────
// Organiser Studio — shared UI primitives
// Small, presentational building blocks reused across the Studio screens.
// ─────────────────────────────────────────────────────────────────────────────
import React from 'react';
import { View, Text, ActivityIndicator, StyleSheet } from 'react-native';
import { STUDIO, ACCENTS, AccentKey, statusChip } from './theme';

// ── Status chip ───────────────────────────────────────────────────────────────
export function Chip({ status, label }: { status: string; label?: string }) {
  const c = statusChip(status);
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
  color = STUDIO.accent,
  height = 6,
}: {
  pct: number;
  color?: string;
  height?: number;
}) {
  const clamped = Math.max(0, Math.min(100, pct));
  return (
    <View style={[ui.track, { height, borderRadius: height }]}>
      <View
        style={{
          width: `${clamped}%`,
          height: '100%',
          backgroundColor: color,
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
  const a = ACCENTS[accent];
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
  return (
    <View style={ui.center}>
      <ActivityIndicator color={STUDIO.accent} />
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
  return (
    <View style={ui.empty}>
      {icon ? <View style={ui.emptyIcon}>{icon}</View> : null}
      <Text style={ui.emptyTitle}>{title}</Text>
      {desc ? <Text style={ui.emptyDesc}>{desc}</Text> : null}
    </View>
  );
}

const ui = StyleSheet.create({
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
    backgroundColor: 'rgba(255,255,255,0.08)',
    overflow: 'hidden',
  },

  statCard: {
    flex: 1,
    minWidth: 0,
    backgroundColor: STUDIO.surface,
    borderRadius: 16,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: STUDIO.border,
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
    color: STUDIO.muted,
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
    color: STUDIO.text,
    letterSpacing: -0.5,
  },
  statSub: { fontSize: 11.5, color: STUDIO.muted, marginTop: 3 },

  card: {
    backgroundColor: STUDIO.surface,
    borderRadius: 18,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: STUDIO.border,
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
  cardTitle: { fontSize: 15, fontWeight: '800', color: STUDIO.text },
  cardBody: { paddingHorizontal: 16, paddingBottom: 14 },

  center: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    paddingVertical: 60,
    gap: 12,
  },
  centerSub: { fontSize: 13, color: STUDIO.muted },

  empty: { alignItems: 'center', paddingVertical: 44, paddingHorizontal: 32, gap: 8 },
  emptyIcon: {
    width: 64,
    height: 64,
    borderRadius: 20,
    backgroundColor: STUDIO.surface,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: STUDIO.border,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 4,
  },
  emptyTitle: {
    fontSize: 16,
    fontWeight: '800',
    color: STUDIO.text,
    textAlign: 'center',
  },
  emptyDesc: {
    fontSize: 13,
    color: STUDIO.muted,
    textAlign: 'center',
    lineHeight: 19,
  },
});