import React, { useCallback, useEffect, useState } from 'react';
import {
  View, Text, TouchableOpacity, FlatList, ActivityIndicator, RefreshControl, StyleSheet,
} from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { STUDIO } from '../../../components/studio/theme';
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

  return (
    <View style={[styles.root, { paddingTop: insets.top + 8 }]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} hitSlop={12} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={22} color={STUDIO.text} />
        </TouchableOpacity>
        <View style={{ flex: 1, alignItems: 'center' }}>
          <Text style={styles.title}>Scan exceptions</Text>
          {!!title && <Text style={styles.subtitle} numberOfLines={1}>{title}</Text>}
        </View>
        <View style={{ width: 34 }} />
      </View>

      {loading ? (
        <ActivityIndicator color={STUDIO.sub} style={{ marginTop: 40 }} />
      ) : error ? (
        <TouchableOpacity onPress={() => { setLoading(true); load(); }}>
          <Text style={[styles.empty, { color: STUDIO.red }]}>{error} Tap to retry.</Text>
        </TouchableOpacity>
      ) : (
        <FlatList
          data={items}
          keyExtractor={(e) => e.id}
          contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}
          refreshControl={
            <RefreshControl refreshing={refreshing} tintColor={STUDIO.sub}
              onRefresh={() => { setRefreshing(true); load(); }} />
          }
          ListEmptyComponent={
            <View style={{ alignItems: 'center', marginTop: 48, gap: 10 }}>
              <Ionicons name="shield-checkmark-outline" size={36} color={STUDIO.green} />
              <Text style={styles.empty}>No exceptions — every admission checks out.</Text>
            </View>
          }
          renderItem={({ item }) => (
            <View style={[styles.row, { borderLeftColor: item.verdict === 'collision' ? STUDIO.red : '#ffb830' }]}>
              <View style={styles.badge(item.verdict)}>
                <Text style={styles.badgeTxt(item.verdict)}>
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

const styles = {
  ...StyleSheet.create({
    root: { flex: 1, backgroundColor: STUDIO.bg, paddingHorizontal: 16 },
    header: { flexDirection: 'row', alignItems: 'center', marginBottom: 14 },
    backBtn: { width: 34, height: 34, borderRadius: 17, alignItems: 'center', justifyContent: 'center', backgroundColor: STUDIO.surface },
    title: { color: STUDIO.text, fontSize: 17, fontWeight: '700' },
    subtitle: { color: STUDIO.sub, fontSize: 11.5, marginTop: 2, maxWidth: 220 },
    empty: { color: STUDIO.sub, fontSize: 13, lineHeight: 19, textAlign: 'center' },
    row: {
      backgroundColor: STUDIO.surface, borderRadius: 14, borderWidth: 1, borderColor: STUDIO.border,
      borderLeftWidth: 3, padding: 14, marginBottom: 10, gap: 5,
    },
    name: { color: STUDIO.text, fontSize: 14.5, fontWeight: '700' },
    meta: { color: STUDIO.sub, fontSize: 12, lineHeight: 17 },
  }),
  badge: (v: string) => ({
    alignSelf: 'flex-start' as const,
    backgroundColor: v === 'collision' ? STUDIO.redSoft : 'rgba(255,184,48,0.15)',
    borderRadius: 6, paddingHorizontal: 7, paddingVertical: 3,
  }),
  badgeTxt: (v: string) => ({
    color: v === 'collision' ? STUDIO.red : '#ffb830',
    fontSize: 10, fontWeight: '800' as const, letterSpacing: 0.6,
  }),
};