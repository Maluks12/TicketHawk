import React, { useCallback, useEffect, useState } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, ScrollView,
  ActivityIndicator, Alert, StyleSheet, Share,
} from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { Ionicons } from '@expo/vector-icons';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import QRCode from 'react-native-qrcode-svg';
import { makeStyles, useStudioTheme } from '../../../components/studio/theme';
import {
  listEventStaff, addEventStaff, revokeEventStaff,
  createStaffCode, listStaffCodes, revokeStaffCode,
  type StaffMember, type StaffCode,
} from '../../../lib/staff';

// ─────────────────────────────────────────────────────────────────────────────
// Manage door staff — owner / manager view for one event.
//
// Two ways in for staff:
//   * Add by email — for known collaborators with existing accounts.
//   * Join code    — for door staff at the venue: generate, show big + as QR,
//                    they redeem in "Join event staff" on their own phone.
// Revocation is immediate: the backend checks membership on every scan.
// ─────────────────────────────────────────────────────────────────────────────

export default function ManageEventStaff() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { eventId, title } = useLocalSearchParams<{ eventId: string; title?: string }>();
  const { S } = useStudioTheme();
  const styles = useStyles();

  const [staff, setStaff] = useState<StaffMember[]>([]);
  const [codes, setCodes] = useState<StaffCode[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [email, setEmail] = useState('');
  const [adding, setAdding] = useState(false);
  const [creatingCode, setCreatingCode] = useState(false);
  const [activeCode, setActiveCode] = useState<StaffCode | null>(null);

  const load = useCallback(async () => {
    if (!eventId) return;
    try {
      const [s, c] = await Promise.all([listEventStaff(eventId), listStaffCodes(eventId)]);
      setStaff(s.data.staff ?? []);
      setCodes(c.data.codes ?? []);
      setError(null);
    } catch (e: any) {
      setError(e?.response?.data?.error ?? 'Could not load staff. Check your connection.');
    } finally {
      setLoading(false);
    }
  }, [eventId]);

  useEffect(() => { load(); }, [load]);

  // ── Actions ────────────────────────────────────────────────────────────────

  const add = async () => {
    const em = email.trim();
    if (!em || !eventId) return;
    setAdding(true);
    try {
      await addEventStaff(eventId, em, 'scanner');
      setEmail('');
      load();
    } catch (e: any) {
      Alert.alert('Could not add', e?.response?.data?.error ?? 'Something went wrong.');
    } finally {
      setAdding(false);
    }
  };

  const revoke = (m: StaffMember) => {
    Alert.alert(
      'Remove scanner access?',
      `${m.first_name || m.email} will be blocked from their very next scan.`,
      [
        { text: 'Cancel', style: 'cancel' },
        {
          text: 'Remove', style: 'destructive',
          onPress: async () => {
            try { await revokeEventStaff(eventId!, m.user_id); load(); }
            catch (e: any) { Alert.alert('Failed', e?.response?.data?.error ?? 'Could not revoke.'); }
          },
        },
      ],
    );
  };

  const makeCode = async () => {
    if (!eventId) return;
    setCreatingCode(true);
    try {
      const { data } = await createStaffCode(eventId, { role: 'scanner', max_uses: 10 });
      setActiveCode(data.code);
      load();
    } catch (e: any) {
      Alert.alert('Could not create code', e?.response?.data?.error ?? 'Something went wrong.');
    } finally {
      setCreatingCode(false);
    }
  };

  const killCode = async (c: StaffCode) => {
    try {
      await revokeStaffCode(eventId!, c.id);
      if (activeCode?.id === c.id) setActiveCode(null);
      load();
    } catch (e: any) {
      Alert.alert('Failed', e?.response?.data?.error ?? 'Could not revoke the code.');
    }
  };

  const shareCode = (c: StaffCode) =>
    Share.share({
      message: `Join code for scanning tickets at “${title ?? 'the event'}”: ${c.code}\n\nOpen TicketHawk → Join event staff → enter this code.`,
    });

  const activeStaff = staff.filter((m) => m.status === 'active');
  const liveCodes = codes.filter((c) => !c.revoked_at && new Date(c.expires_at) > new Date());

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <View style={[styles.root, { paddingTop: insets.top + 8 }]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} hitSlop={12} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={22} color={S.text} />
        </TouchableOpacity>
        <View style={{ flex: 1, alignItems: 'center' }}>
          <Text style={styles.title}>Door staff</Text>
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
        <ScrollView contentContainerStyle={{ paddingBottom: insets.bottom + 32 }} showsVerticalScrollIndicator={false}>

          {/* Join code */}
          <View style={styles.card}>
            <Text style={styles.cardTitle}>Join code</Text>
            <Text style={styles.cardSub}>
              Fastest way to onboard staff at the venue: they enter this code under
              “Join event staff” on their own phone. Codes are use-limited and expire
              with the event.
            </Text>

            {activeCode ? (
              <View style={styles.codeBox}>
                <View style={styles.qrWrap}>
                  <QRCode value={activeCode.code} size={110} color="#0a0a0a" backgroundColor="#ffffff" quietZone={5} />
                </View>
                <Text style={styles.codeBig}>{activeCode.code}</Text>
                <Text style={styles.codeMeta}>
                  {activeCode.use_count}/{activeCode.max_uses} used · expires{' '}
                  {new Date(activeCode.expires_at).toLocaleString()}
                </Text>
                <View style={styles.row}>
                  <TouchableOpacity style={styles.secondaryBtn} onPress={() => shareCode(activeCode)}>
                    <Ionicons name="share-outline" size={15} color={S.text} />
                    <Text style={styles.secondaryBtnTxt}>Share</Text>
                  </TouchableOpacity>
                  <TouchableOpacity
                    style={[styles.secondaryBtn, { borderColor: S.red }]}
                    onPress={() => killCode(activeCode)}
                  >
                    <Ionicons name="close-circle-outline" size={15} color={S.red} />
                    <Text style={[styles.secondaryBtnTxt, { color: S.red }]}>Revoke</Text>
                  </TouchableOpacity>
                </View>
              </View>
            ) : (
              <TouchableOpacity style={styles.primaryBtn} onPress={makeCode} disabled={creatingCode} activeOpacity={0.85}>
                {creatingCode
                  ? <ActivityIndicator color={S.onAccent} size="small" />
                  : <Text style={styles.primaryBtnTxt}>Generate join code</Text>}
              </TouchableOpacity>
            )}

            {liveCodes.filter((c) => c.id !== activeCode?.id).map((c) => (
              <TouchableOpacity key={c.id} style={styles.codeRow} onPress={() => setActiveCode(c)}>
                <Text style={styles.codeRowTxt}>{c.code}</Text>
                <Text style={styles.codeRowMeta}>{c.use_count}/{c.max_uses} used</Text>
                <Ionicons name="chevron-forward" size={15} color={S.muted} />
              </TouchableOpacity>
            ))}
          </View>

          {/* Add by email */}
          <View style={styles.card}>
            <Text style={styles.cardTitle}>Add by email</Text>
            <Text style={styles.cardSub}>
              For someone you already work with — grants scanner access to their existing
              TicketHawk account immediately.
            </Text>
            <View style={styles.row}>
              <TextInput
                style={styles.input}
                value={email}
                onChangeText={setEmail}
                placeholder="their@email.com"
                placeholderTextColor={S.muted}
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="email-address"
                onSubmitEditing={add}
                returnKeyType="done"
              />
              <TouchableOpacity
                style={[styles.primaryBtn, { paddingHorizontal: 18 }, (!email.trim() || adding) && { opacity: 0.5 }]}
                onPress={add}
                disabled={!email.trim() || adding}
              >
                {adding ? <ActivityIndicator color={S.onAccent} size="small" /> : <Text style={styles.primaryBtnTxt}>Add</Text>}
              </TouchableOpacity>
            </View>
          </View>

          {/* Staff list */}
          <Text style={styles.sectionLabel}>ACTIVE STAFF ({activeStaff.length})</Text>
          {activeStaff.length === 0 ? (
            <Text style={styles.empty}>No staff yet. Only you can scan this event right now.</Text>
          ) : (
            activeStaff.map((m) => (
              <View key={m.user_id} style={styles.staffRow}>
                <View style={{ flex: 1 }}>
                  <Text style={styles.staffName} numberOfLines={1}>
                    {(m.first_name || m.last_name) ? `${m.first_name} ${m.last_name}`.trim() : m.email}
                  </Text>
                  <Text style={styles.staffMeta} numberOfLines={1}>
                    {m.email} · {m.role}{m.via_code ? ' · via code' : ''}
                  </Text>
                </View>
                <TouchableOpacity onPress={() => revoke(m)} hitSlop={10} style={styles.removeBtn}>
                  <Ionicons name="person-remove-outline" size={17} color={S.red} />
                </TouchableOpacity>
              </View>
            ))
          )}
        </ScrollView>
      )}
    </View>
  );
}

const useStyles = makeStyles((S) => ({
  root: { flex: 1, backgroundColor: S.bg, paddingHorizontal: 16 },
  header: { flexDirection: 'row', alignItems: 'center', marginBottom: 14 },
  backBtn: {
    width: 34, height: 34, borderRadius: 17, alignItems: 'center', justifyContent: 'center',
    backgroundColor: S.surface,
  },
  title: { color: S.text, fontSize: 17, fontWeight: '700' },
  subtitle: { color: S.sub, fontSize: 11.5, marginTop: 2, maxWidth: 220 },

  card: {
    backgroundColor: S.surface, borderRadius: 16, padding: 16,
    borderWidth: 1, borderColor: S.border, marginBottom: 18,
  },
  cardTitle: { color: S.text, fontSize: 15, fontWeight: '700', marginBottom: 4 },
  cardSub: { color: S.sub, fontSize: 12.5, lineHeight: 18, marginBottom: 14 },
  row: { flexDirection: 'row', gap: 10, marginTop: 4 },

  input: {
    flex: 1, backgroundColor: S.field, borderRadius: 10, borderWidth: 1,
    borderColor: S.borderStrong, color: S.text, paddingHorizontal: 14,
    paddingVertical: 12, fontSize: 14.5,
  },
  primaryBtn: {
    backgroundColor: S.accentDeep, borderRadius: 10, paddingVertical: 13,
    alignItems: 'center', justifyContent: 'center',
  },
  primaryBtnTxt: { color: S.onAccent, fontWeight: '700', fontSize: 14.5 },
  secondaryBtn: {
    flexDirection: 'row', alignItems: 'center', gap: 6, borderWidth: 1,
    borderColor: S.borderStrong, borderRadius: 9, paddingHorizontal: 14, paddingVertical: 9,
  },
  secondaryBtnTxt: { color: S.text, fontSize: 13, fontWeight: '600' },

  codeBox: { alignItems: 'center', gap: 10, paddingVertical: 6 },
  qrWrap: { backgroundColor: '#fff', borderRadius: 12, padding: 8 },
  codeBig: { color: S.text, fontSize: 28, fontWeight: '800', letterSpacing: 4 },
  codeMeta: { color: S.sub, fontSize: 12 },
  codeRow: {
    flexDirection: 'row', alignItems: 'center', gap: 10, marginTop: 12,
    borderTopWidth: 1, borderTopColor: S.border, paddingTop: 12,
  },
  codeRowTxt: { color: S.text, fontSize: 14, fontWeight: '700', letterSpacing: 1.5, flex: 1 },
  codeRowMeta: { color: S.muted, fontSize: 11.5 },

  sectionLabel: { color: S.muted, fontSize: 11, fontWeight: '700', letterSpacing: 1.2, marginBottom: 10 },
  empty: { color: S.sub, fontSize: 13, lineHeight: 19 },

  staffRow: {
    flexDirection: 'row', alignItems: 'center', backgroundColor: S.surface,
    borderRadius: 14, borderWidth: 1, borderColor: S.border,
    paddingHorizontal: 14, paddingVertical: 12, marginBottom: 10,
  },
  staffName: { color: S.text, fontSize: 14.5, fontWeight: '600' },
  staffMeta: { color: S.sub, fontSize: 12, marginTop: 3 },
  removeBtn: {
    width: 32, height: 32, borderRadius: 16, alignItems: 'center', justifyContent: 'center',
    backgroundColor: S.redSoft, marginLeft: 10,
  },
}));