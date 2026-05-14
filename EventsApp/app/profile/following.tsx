import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  View, Text, TouchableOpacity, ActivityIndicator,
  RefreshControl, FlatList, StyleSheet, Alert,
} from 'react-native';
import { Image } from 'expo-image';
import { LinearGradient } from 'expo-linear-gradient';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Ionicons, Feather } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { useAuthStore } from '../../store/authStore';
import { useAppTheme } from '../../theme/colors';
import {
  api,
  type FollowedEvent,
  type FollowingResponse,
  fmtFollowDate,
  fmtFollowPrice,
  followAvailPct,
} from '../../lib/api';

// ─── Card colour palettes ─────────────────────────────────────────────────────

const PALETTES = [
  { top: '#0d1f37', bottom: '#092e20', accent: '#00e5a0' },
  { top: '#1a0a2e', bottom: '#200a3d', accent: '#b57bee' },
  { top: '#1a1000', bottom: '#3a2400', accent: '#ffb830' },
  { top: '#001828', bottom: '#003050', accent: '#29c5f6' },
  { top: '#1a0008', bottom: '#3a0012', accent: '#ff4d6d' },
  { top: '#0a1a00', bottom: '#163800', accent: '#8fff00' },
];

// ─── Event card ───────────────────────────────────────────────────────────────

interface CardProps {
  item: FollowedEvent;
  index: number;
  onPress: () => void;
  onUnfollow: () => void;
  unfollowing: boolean;
}

function EventCard({ item, index, onPress, onUnfollow, unfollowing }: CardProps) {
  const pal = PALETTES[index % PALETTES.length];
  const pct = followAvailPct(item.tickets_sold, item.max_capacity);
  const isSellingFast = pct !== null && pct >= 80 && !item.is_sold_out;

  const pill = item.is_sold_out
    ? { bg: 'rgba(255,77,77,0.2)',   border: 'rgba(255,77,77,0.38)',   color: '#ff6b6b', label: 'SOLD OUT' }
    : isSellingFast
      ? { bg: 'rgba(255,184,48,0.16)', border: 'rgba(255,184,48,0.32)', color: '#ffb830', label: '🔥 SELLING FAST' }
      : { bg: 'rgba(0,229,160,0.1)',   border: 'rgba(0,229,160,0.22)',  color: '#00e5a0', label: item.category?.toUpperCase() || 'EVENT' };

  return (
    <TouchableOpacity activeOpacity={0.92} onPress={onPress} style={cs.touchable}>
      <View style={cs.card}>
        {item.cover_image_url ? (
          <Image
            source={{ uri: item.cover_image_url }}
            style={StyleSheet.absoluteFillObject}
            contentFit="cover"
            transition={180}
          />
        ) : (
          <LinearGradient
            colors={[pal.top, pal.bottom]}
            start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }}
            style={StyleSheet.absoluteFillObject}
          />
        )}
        <LinearGradient
          colors={['rgba(0,0,0,0.06)', 'rgba(0,0,0,0.62)', 'rgba(0,0,0,0.92)']}
          start={{ x: 0, y: 0 }} end={{ x: 0, y: 1 }}
          style={StyleSheet.absoluteFillObject}
        />

        {/* Pill + unfollow heart */}
        <View style={cs.topRow}>
          <View style={[cs.pill, { backgroundColor: pill.bg, borderColor: pill.border }]}>
            <Text style={[cs.pillText, { color: pill.color }]}>{pill.label}</Text>
          </View>
          <TouchableOpacity
            style={cs.heartBtn}
            onPress={(e) => { e.stopPropagation(); onUnfollow(); }}
            activeOpacity={0.7}
            hitSlop={{ top: 10, bottom: 10, left: 10, right: 10 }}
          >
            {unfollowing
              ? <ActivityIndicator size="small" color="#ff2d55" />
              : <Ionicons name="heart" size={17} color="#ff2d55" />}
          </TouchableOpacity>
        </View>

        {/* Info */}
        <View style={cs.bottom}>
          <View style={cs.dateRow}>
            <Ionicons name="calendar-outline" size={10} color="rgba(255,255,255,0.5)" />
            <Text style={cs.dateTxt}>{fmtFollowDate(item.start_time)}</Text>
          </View>
          <Text style={cs.title} numberOfLines={2}>{item.title}</Text>
          <View style={cs.metaRow}>
            <Ionicons name="location-outline" size={10} color="rgba(255,255,255,0.4)" />
            <Text style={cs.metaTxt} numberOfLines={1}>{item.venue_name}, {item.city}</Text>
            <Text style={[cs.price, { color: pal.accent }]}>{fmtFollowPrice(item.price)}</Text>
          </View>
        </View>
      </View>
    </TouchableOpacity>
  );
}

const cs = StyleSheet.create({
  touchable: {
    marginHorizontal: 16,
    marginBottom: 12,
    borderRadius: 18,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 8 },
    shadowOpacity: 0.5,
    shadowRadius: 14,
    elevation: 12,
  },
  card: {
    height: 188,
    borderRadius: 18,
    overflow: 'hidden',
    padding: 14,
    justifyContent: 'space-between',
  },
  topRow: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
  },
  pill: {
    paddingHorizontal: 8,
    paddingVertical: 4,
    borderRadius: 6,
    borderWidth: 1,
  },
  pillText: {
    fontSize: 9,
    fontWeight: '800',
    letterSpacing: 1.2,
  },
  heartBtn: {
    width: 34,
    height: 34,
    borderRadius: 17,
    backgroundColor: 'rgba(0,0,0,0.45)',
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.1)',
  },
  bottom: { gap: 4 },
  dateRow: { flexDirection: 'row', alignItems: 'center', gap: 5 },
  dateTxt: { fontSize: 11, fontWeight: '600', color: 'rgba(255,255,255,0.55)', letterSpacing: 0.2 },
  title: { fontSize: 20, fontWeight: '800', color: '#fff', letterSpacing: -0.3, lineHeight: 24 },
  metaRow: { flexDirection: 'row', alignItems: 'center', gap: 5 },
  metaTxt: { fontSize: 11, color: 'rgba(255,255,255,0.42)', fontWeight: '500', flex: 1 },
  price: { fontSize: 12, fontWeight: '800' },
});

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function FollowingScreen() {
  const router   = useRouter();
  const insets   = useSafeAreaInsets();
  const { user } = useAuthStore();
  const { colors } = useAppTheme();

  const [events,      setEvents]      = useState<FollowedEvent[]>([]);
  const [loading,     setLoading]     = useState(true);
  const [refreshing,  setRefreshing]  = useState(false);
  const [cursor,      setCursor]      = useState<string | null>(null);
  const [hasMore,     setHasMore]     = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [unfollowing, setUnfollowing] = useState<Set<string>>(new Set());

  const load = useCallback(async (replace = false) => {
    if (!user) { setLoading(false); return; }
    try {
      replace ? setLoading(true) : setLoadingMore(true);
      const params: Record<string, string> = { limit: '20' };
      if (!replace && cursor) params.cursor = cursor;
      const res = await api.get<FollowingResponse>('/v1/user/following', { params });
      const { data: items, next_cursor } = res.data;
      setEvents(prev => replace ? items : [...prev, ...items]);
      setCursor(next_cursor ?? null);
      setHasMore(!!next_cursor);
    } catch { /* show empty state */ }
    finally {
      setLoading(false);
      setLoadingMore(false);
      setRefreshing(false);
    }
  }, [user, cursor]);

  useEffect(() => { load(true); }, []);

  const handleRefresh = useCallback(() => {
    setRefreshing(true);
    setCursor(null);
    load(true);
  }, [load]);

  const handleUnfollow = useCallback(async (eventId: string) => {
    setUnfollowing(prev => new Set(prev).add(eventId));
    try {
      await api.post(`/v1/events/${eventId}/follow`);
      setEvents(prev => prev.filter(e => e.id !== eventId));
    } catch {
      Alert.alert('Error', 'Could not unfollow. Please try again.');
    } finally {
      setUnfollowing(prev => { const n = new Set(prev); n.delete(eventId); return n; });
    }
  }, []);

  // ── Render ──────────────────────────────────────────────────────────────────

  const renderContent = () => {
    if (!user) {
      return (
        <View style={ss.centred}>
          <View style={ss.emptyIcon}>
            <Ionicons name="heart-outline" size={30} color="rgba(255,255,255,0.2)" />
          </View>
          <Text style={ss.emptyTitle}>Sign in to see followed events</Text>
          <Text style={[ss.emptySub, { color: colors.subText }]}>
            Follow events you're excited about and we'll remind you as the date gets close.
          </Text>
        </View>
      );
    }

    if (loading) {
      return <View style={ss.centred}><ActivityIndicator color="#ff2d55" size="large" /></View>;
    }

    if (events.length === 0) {
      return (
        <View style={ss.centred}>
          <View style={ss.emptyIcon}>
            <Ionicons name="heart-outline" size={30} color="rgba(255,255,255,0.2)" />
          </View>
          <Text style={ss.emptyTitle}>No followed events yet</Text>
          <Text style={[ss.emptySub, { color: colors.subText }]}>
            Tap Follow on any event to track it. You'll get reminders as the date approaches and alerts when tickets sell out.
          </Text>
        </View>
      );
    }

    return (
      <FlatList
        data={events}
        keyExtractor={item => item.follow_id}
        showsVerticalScrollIndicator={false}
        contentContainerStyle={{ paddingTop: 8, paddingBottom: insets.bottom + 32 }}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={handleRefresh} tintColor="rgba(255,255,255,0.35)" />
        }
        onEndReached={() => { if (hasMore && !loadingMore) load(false); }}
        onEndReachedThreshold={0.3}
        ListFooterComponent={
          loadingMore
            ? <View style={{ paddingVertical: 20 }}><ActivityIndicator color="rgba(255,255,255,0.25)" /></View>
            : null
        }
        renderItem={({ item, index }) => (
          <EventCard
            item={item}
            index={index}
            onPress={() => router.push(`/event/${item.slug || item.id}` as any)}
            onUnfollow={() => handleUnfollow(item.id)}
            unfollowing={unfollowing.has(item.id)}
          />
        )}
      />
    );
  };

  return (
    <View style={[ss.root, { backgroundColor: colors.background }]}>

      {/* ── Header ────────────────────────────────────────────────────────── */}
      <View style={[ss.header, { paddingTop: insets.top + 12, backgroundColor: colors.background }]}>
        <TouchableOpacity style={ss.backBtn} onPress={() => router.back()} activeOpacity={0.7}>
          <Feather name="arrow-left" size={20} color={colors.text} />
        </TouchableOpacity>
        <View style={ss.headerCenter}>
          <Text style={[ss.headerTitle, { color: colors.text }]}>Following</Text>
          {events.length > 0 && !loading && (
            <Text style={[ss.headerCount, { color: colors.subText }]}>
              {events.length} event{events.length !== 1 ? 's' : ''}
            </Text>
          )}
        </View>
        {/* Spacer to keep title centered */}
        <View style={{ width: 40 }} />
      </View>

      {/* Subtle separator */}
      <View style={[ss.headerDivider, { backgroundColor: colors.dividerColor }]} />

      {renderContent()}
    </View>
  );
}

const ss = StyleSheet.create({
  root: {
    flex: 1,
  },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingBottom: 14,
  },
  backBtn: {
    width: 40,
    height: 40,
    borderRadius: 20,
    alignItems: 'center',
    justifyContent: 'center',
  },
  headerCenter: {
    flex: 1,
    alignItems: 'center',
  },
  headerTitle: {
    fontSize: 18,
    fontWeight: '800',
    letterSpacing: -0.2,
  },
  headerCount: {
    fontSize: 12,
    fontWeight: '500',
    marginTop: 1,
  },
  headerDivider: {
    height: StyleSheet.hairlineWidth,
    marginBottom: 4,
  },
  centred: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 44,
    gap: 12,
  },
  emptyIcon: {
    width: 80,
    height: 80,
    borderRadius: 24,
    backgroundColor: 'rgba(255,255,255,0.04)',
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 4,
  },
  emptyTitle: {
    fontSize: 20,
    fontWeight: '800',
    color: '#ffffff',
    textAlign: 'center',
    letterSpacing: -0.2,
  },
  emptySub: {
    fontSize: 13,
    textAlign: 'center',
    lineHeight: 20,
  },
});