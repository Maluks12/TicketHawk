import React, { useCallback, useEffect, useState } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, FlatList,
  ActivityIndicator, StyleSheet, KeyboardAvoidingView, Platform, Alert,
} from 'react-native';
import { useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { STUDIO } from '../../components/studio/theme';
import { redeemStaffCode, listMyScanEvents, type ScannableEvent } from '../../lib/staff';
import { useAuthStore } from '../../store/authStore';

// ─────────────────────────────────────────────────────────────────────────────
// Join event staff — the staffer's single entry point.
//
// A door staffer redeems the join code the organiser shows them, and every
// event they can scan is listed below with a direct route into the scanner.
// Deliberately minimal: this is a tool screen, not a consumer surface.
// ─────────────────────────────────────────────────────────────────────────────

export default function JoinEventStaff() {
  const insets = useSafeAreaInsets();
  const router = useRouter();

  const [code, setCode] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [feedback, setFeedback] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null);

  const [events, setEvents] = useState<ScannableEvent[]>([]);
  const [loadingEvents, setLoadingEvents] = useState(true);

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

  const redeem = async () => {
    const trimmed = code.trim();
    if (!trimmed) return;
    setSubmitting(true);
    setFeedback(null);
    try {
      const { data } = await redeemStaffCode(trimmed);
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
        {/* Join by code */}
        <View style={styles.card}>
          <Text style={styles.cardTitle}>Join with a code</Text>
          <Text style={styles.cardSub}>
            Ask the event organiser for a join code, then enter it here to start scanning tickets.
          </Text>
          <View style={styles.row}>
            <TextInput
              style={styles.input}
              value={code}
              onChangeText={setCode}
              placeholder="e.g. 7KQ4-M2XR"
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
            Nothing yet. Redeem a join code above, and events you're staffing will appear here.
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
  row: { flexDirection: 'row', gap: 10 },
  input: {
    flex: 1, backgroundColor: STUDIO.bgElevated, borderRadius: 10, borderWidth: 1,
    borderColor: STUDIO.borderStrong, color: STUDIO.text, paddingHorizontal: 14,
    paddingVertical: 12, fontSize: 16, letterSpacing: 2, fontWeight: '600',
  },
  joinBtn: {
    backgroundColor: STUDIO.accentDeep, borderRadius: 10, paddingHorizontal: 20,
    alignItems: 'center', justifyContent: 'center',
  },
  joinBtnTxt: { color: '#fff', fontWeight: '700', fontSize: 15 },
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
});