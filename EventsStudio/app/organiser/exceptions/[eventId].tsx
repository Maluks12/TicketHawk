import React, { useCallback, useEffect, useState } from 'react';
import {
  View, Text, TouchableOpacity, FlatList, ActivityIndicator, RefreshControl, StyleSheet,
} from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { makeStyles, useStudioTheme } from '../../../components/studio/theme';
import { api } from '../../../lib/api';

// ─────────────────────────────────────────────────────────────────────────────
// Scan exceptions — the multi-gate report.
//
// Collisions (same ticket admitted at two gates / pool over-admission) and
// unresolved provisionals (tickets admitted at the gate that weren't in the
// device's manifest). By the time a collision is known both people are inside;
// the point is that the organiser FINDS OUT — names, gates, timestamps.
// ─────────────────────────────────────────────────────────────────────────────

interface ScanException {
  id: string;
  verdict: 'collision' | 'provisional';
  name: string;
  slot_number?: number;
  admit_count: number;
  scanned_by: string;
  device_id: string;
  scanned_at: string;
  received_at: string;
  first_scan_at?: string;
  raw_ref?: string;
}

export default function ScanExceptions() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { eventId, title } = useLocalSearchParams<{ eventId: string; title?: string }>();
  const { S } = useStudioTheme();
  const styles = useStyles();

  const [items, setItems] = useState<ScanException[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!eventId) return;
    try {
      const { data } = await api.get(`/v1/events/${eventId}/scan-exceptions`);
      setItems(data.exceptions ?? []);
      setError(null);
    } catch (e: any) {
      setError(e?.response?.data?.error ?? 'Could not load exceptions.');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [eventId]);

  useEffect(() => { load(); }, [load]);

  const fmt = (s: string) => new Date(s).toLocaleTimeString();

  // The two verdicts have their own colour. Resolved from the palette rather
  // than the hardcoded '#ffb830' this file used to carry — that amber is a
  // highlighter pen on a white card.
  const verdictColor = (v: string) => (v === 'collision' ? S.red : S.amber);
  const verdictSoft = (v: string) => (v === 'collision' ? S.redSoft : S.amberSoft);

  return (
    <View style={[styles.root, { paddingTop: insets.top + 8 }]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} hitSlop={12} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={22} color={S.text} />
        </TouchableOpacity>
        <View style={{ flex: 1, alignItems: 'center' }}>
          <Text style={styles.title}>Scan exceptions</Text>
          {!!title && <Text style={styles.subtitle} numberOfLines={1}>{title}</Text>}
        </View>
        <View style={{ width: 34 }} />
      </View>

      {loading ? (
        <ActivityIndicator color={S.sub} style={{ marginTop: 40 }} />
      ) : error ? (
        <TouchableOpacity onPress={() => { setLoading(true); load(); }}>
          <Text style={[styles.empty, { color: S.red }]}>{error} Tap to retry.</Text>
        </TouchableOpacity>
      ) : (
        <FlatList
          data={items}
          keyExtractor={(e) => e.id}
          contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}
          refreshControl={
            <RefreshControl refreshing={refreshing} tintColor={S.sub}
              onRefresh={() => { setRefreshing(true); load(); }} />
          }
          ListEmptyComponent={
            <View style={{ alignItems: 'center', marginTop: 48, gap: 10 }}>
              <Ionicons name="shield-checkmark-outline" size={36} color={S.green} />
              <Text style={styles.empty}>No exceptions — every admission checks out.</Text>
            </View>
          }
          renderItem={({ item }) => (
            <View style={[styles.row, { borderLeftColor: verdictColor(item.verdict) }]}>
              <View style={[styles.badge, { backgroundColor: verdictSoft(item.verdict) }]}>
                <Text style={[styles.badgeTxt, { color: verdictColor(item.verdict) }]}>
                  {item.verdict === 'collision' ? 'DUPLICATE ENTRY' : 'PROVISIONAL'}
                </Text>
              </View>
              <Text style={styles.name}>
                {item.name || (item.raw_ref ? `Ref ${item.raw_ref.slice(0, 8)}…` : 'Unknown ticket')}
                {item.slot_number ? `  · Slot ${item.slot_number}` : ''}
                {item.admit_count > 1 ? `  · ${item.admit_count} heads` : ''}
              </Text>
              <Text style={styles.meta}>
                Scanned by {item.scanned_by} · device {item.device_id.slice(0, 8)} · {fmt(item.scanned_at)}
              </Text>
              {item.verdict === 'collision' && item.first_scan_at && (
                <Text style={styles.meta}>First admitted at {fmt(item.first_scan_at)} — this scan was second.</Text>
              )}
              {item.verdict === 'provisional' && (
                <Text style={styles.meta}>Admitted at the gate but not found in the attendee list at sync time.</Text>
              )}
            </View>
          )}
        />
      )}
    </View>
  );
}

const useStyles = makeStyles((S) => ({
  root: { flex: 1, backgroundColor: S.bg, paddingHorizontal: 16 },
  header: { flexDirection: 'row', alignItems: 'center', marginBottom: 14 },
  backBtn: {
    width: 34, height: 34, borderRadius: 17, alignItems: 'center', justifyContent: 'center',
    backgroundColor: S.surfaceStrong,
  },
  title: { color: S.text, fontSize: 17, fontWeight: '700' },
  subtitle: { color: S.sub, fontSize: 11.5, marginTop: 2, maxWidth: 220 },
  empty: { color: S.sub, fontSize: 13, lineHeight: 19, textAlign: 'center' },
  row: {
    backgroundColor: S.surface, borderRadius: 14, borderWidth: 1, borderColor: S.border,
    borderLeftWidth: 3, padding: 14, marginBottom: 10, gap: 5,
  },
  name: { color: S.text, fontSize: 14.5, fontWeight: '700' },
  meta: { color: S.sub, fontSize: 12, lineHeight: 17 },
  badge: {
    alignSelf: 'flex-start',
    borderRadius: 6, paddingHorizontal: 7, paddingVertical: 3,
  },
  badgeTxt: { fontSize: 10, fontWeight: '800', letterSpacing: 0.6 },
}));