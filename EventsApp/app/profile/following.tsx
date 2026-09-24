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
  type FollowedOrganizer,
  fmtFollowDate,
  fmtFollowPrice,
  followAvailPct,
} from '../../lib/api';
import { CheckCircle } from 'phosphor-react-native';

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
            style={StyleSheet.absoluteFill}
            contentFit="cover"
            transition={180}
          />
        ) : (
          <LinearGradient
            colors={[pal.top, pal.bottom]}
            start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }}
            style={StyleSheet.absoluteFill}
          />
        )}
        <LinearGradient
          colors={['rgba(0,0,0,0.06)', 'rgba(0,0,0,0.62)', 'rgba(0,0,0,0.92)']}
          start={{ x: 0, y: 0 }} end={{ x: 0, y: 1 }}
          style={StyleSheet.absoluteFill}
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
    // No shadow.
    //
    // These cards are already dark artwork with a gradient scrim, so a
    // shadowOpacity of 0.5 at radius 14 was casting a heavy dark halo onto a
    // light background — it read as a rendering artefact rather than depth,
    // and stacked between adjacent cards into a grey band.
    //
    // The card has its own contrast; it does not need lifting off the page.
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

// ─── Organiser row ────────────────────────────────────────────────────────────
//
// Deliberately NOT the same 188pt artwork card the events use. An organiser has
// no cover art of their own worth that much height, and a list of six identical
// dark rectangles with a logo in the corner would be harder to scan than a
// compact row. The count of what they have on sale is the useful part.

function OrganiserRow({ item, onPress, onUnfollow, unfollowing, colors }: {
  item: FollowedOrganizer;
  onPress: () => void;
  onUnfollow: () => void;
  unfollowing: boolean;
  colors: any;
}) {
  return (
    <TouchableOpacity
      activeOpacity={0.75}
      onPress={onPress}
      style={[os.row, { borderBottomColor: colors.dividerColor }]}
    >
      <View style={[os.logo, { backgroundColor: colors.sectionCardBg }]}>
        {item.logo_url ? (
          <Image
            source={{ uri: item.logo_url }}
            style={StyleSheet.absoluteFill}
            contentFit="cover"
            cachePolicy="memory-disk"
            recyclingKey={item.id}
            transition={140}
          />
        ) : (
          <Ionicons name="business-outline" size={20} color={colors.iconColor} />
        )}
      </View>

      <View style={{ flex: 1 }}>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 5 }}>
          <Text style={[os.name, { color: colors.text }]} numberOfLines={1}>
            {item.business_name}
          </Text>
          {item.verified && <CheckCircle size={15} color="#2f7cf6" weight="fill" />}
        </View>
        <Text style={[os.meta, { color: colors.subText }]} numberOfLines={1}>
          {item.upcoming_events > 0
            ? `${item.upcoming_events} event${item.upcoming_events === 1 ? '' : 's'} on sale`
            : 'No events on sale'}
        </Text>
      </View>

      <TouchableOpacity
        onPress={onUnfollow}
        disabled={unfollowing}
        hitSlop={10}
        style={[os.unfollow, { borderColor: colors.dividerColor }]}
      >
        {unfollowing
          ? <ActivityIndicator size="small" color={colors.subText} />
          : <Text style={[os.unfollowText, { color: colors.subText }]}>FOLLOWING</Text>}
      </TouchableOpacity>
    </TouchableOpacity>
  );
}

const os = StyleSheet.create({
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 14,
    paddingHorizontal: 16,
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  logo: {
    width: 50, height: 50, borderRadius: 14,
    overflow: 'hidden', alignItems: 'center', justifyContent: 'center',
  },
  name: { fontSize: 15.5, fontWeight: '700', letterSpacing: -0.1, flexShrink: 1 },
  meta: { fontSize: 12.5, marginTop: 3 },
  unfollow: {
    borderWidth: 1, borderRadius: 20,
    paddingHorizontal: 12, paddingVertical: 7,
    minWidth: 92, alignItems: 'center',
  },
  unfollowText: { fontSize: 10.5, fontWeight: '800', letterSpacing: 0.9 },
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

  // Which list is showing. Events first — it is the older feature, the larger
  // list for most people, and the one the profile row has always led to.
  const [tab, setTab] = useState<'events' | 'organisers'>('events');
  const [organisers, setOrganisers] = useState<FollowedOrganizer[]>([]);
  const [orgLoading, setOrgLoading] = useState(true);

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

  // Organisers load ONCE on mount, alongside the events, rather than lazily
  // when the tab is first opened.
  //
  // The list is unpaged and small — a user following more than a few dozen
  // organisers does not exist yet — so fetching it up front costs one request
  // and makes the tab switch instant. Deferring it would put a spinner behind
  // a tap that should feel like nothing happened.
  const loadOrganisers = useCallback(async () => {
    if (!user) { setOrgLoading(false); return; }
    try {
      const res = await api.get<{ data: FollowedOrganizer[]; count: number }>(
        '/v1/user/following-organisers',
      );
      setOrganisers(res.data?.data ?? []);
    } catch {
      setOrganisers([]);
    } finally {
      setOrgLoading(false);
    }
  }, [user]);

  useEffect(() => { load(true); loadOrganisers(); }, []);

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

  const handleUnfollowOrganiser = useCallback(async (org: FollowedOrganizer) => {
    setUnfollowing(prev => new Set(prev).add(org.id));
    try {
      // Slug, not id — the route resolves either, but the slug is what every
      // other organiser call in the app uses and keeps the logs readable.
      await api.post(`/v1/organisers/${org.slug}/follow`);
      setOrganisers(prev => prev.filter(o => o.id !== org.id));
    } catch {
      Alert.alert('Error', 'Could not unfollow. Please try again.');
    } finally {
      setUnfollowing(prev => { const n = new Set(prev); n.delete(org.id); return n; });
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
          <Text style={[ss.emptyTitle, { color: colors.text }]}>Sign in to see followed events</Text>
          <Text style={[ss.emptySub, { color: colors.subText }]}>
            Follow events you're excited about and we'll remind you as the date gets close.
          </Text>
        </View>
      );
    }

    // ── Organisers ─────────────────────────────────────────────────────────
    if (tab === 'organisers') {
      if (orgLoading) {
        return <View style={ss.centred}><ActivityIndicator color="#ff2d55" size="large" /></View>;
      }
      if (organisers.length === 0) {
        return (
          <View style={ss.centred}>
            <View style={ss.emptyIcon}>
              <Ionicons name="business-outline" size={30} color={colors.subText} />
            </View>
            <Text style={[ss.emptyTitle, { color: colors.text }]}>No followed organisers yet</Text>
            <Text style={[ss.emptySub, { color: colors.subText }]}>
              Follow an organiser and you&apos;ll hear about their new events as soon as they go on sale.
            </Text>
          </View>
        );
      }
      return (
        <FlatList
          data={organisers}
          keyExtractor={o => o.id}
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingTop: 4, paddingBottom: insets.bottom + 32 }}
          renderItem={({ item }) => (
            <OrganiserRow
              item={item}
              colors={colors}
              onPress={() => router.push(`/organiser/${item.slug}` as any)}
              onUnfollow={() => handleUnfollowOrganiser(item)}
              unfollowing={unfollowing.has(item.id)}
            />
          )}
        />
      );
    }

    // ── Events ─────────────────────────────────────────────────────────────
    if (loading) {
      return <View style={ss.centred}><ActivityIndicator color="#ff2d55" size="large" /></View>;
    }

    if (events.length === 0) {
      return (
        <View style={ss.centred}>
          <View style={ss.emptyIcon}>
            <Ionicons name="heart-outline" size={30} color="rgba(255,255,255,0.2)" />
          </View>
          <Text style={[ss.emptyTitle, { color: colors.text }]}>No followed events yet</Text>
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
          {/* Count follows the visible tab. A header reading "12 events" above
              a list of organisers is worse than no count at all. */}
          {tab === 'events' && events.length > 0 && !loading && (
            <Text style={[ss.headerCount, { color: colors.subText }]}>
              {events.length} event{events.length !== 1 ? 's' : ''}
            </Text>
          )}
          {tab === 'organisers' && organisers.length > 0 && !orgLoading && (
            <Text style={[ss.headerCount, { color: colors.subText }]}>
              {organisers.length} organiser{organisers.length !== 1 ? 's' : ''}
            </Text>
          )}
        </View>
        {/* Spacer to keep title centered */}
        <View style={{ width: 40 }} />
      </View>

      {/* ── Tabs ──────────────────────────────────────────────────────────── */}
      <View style={[ss.tabRow, { borderBottomColor: colors.dividerColor }]}>
        {([['events', 'EVENTS'], ['organisers', 'ORGANISERS']] as const).map(([key, label]) => (
          <TouchableOpacity
            key={key}
            onPress={() => setTab(key)}
            activeOpacity={0.8}
            style={[ss.tab, tab === key && { borderBottomColor: '#ff2d55' }]}
          >
            <Text style={[ss.tabText, { color: tab === key ? colors.text : colors.subText }]}>
              {label}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

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
  tabRow: {
    flexDirection: 'row',
    gap: 24,
    paddingHorizontal: 16,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  tab: {
    paddingVertical: 12,
    borderBottomWidth: 2,
    borderBottomColor: 'transparent',
  },
  tabText: {
    fontSize: 12,
    fontWeight: '800',
    letterSpacing: 1.1,
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
    // Was hardcoded '#ffffff', which is invisible on the light theme — the
    // screenshot of this screen in light mode had a blank line where the
    // heading should be. Every call site now passes colors.text.
    textAlign: 'center',
    letterSpacing: -0.2,
  },
  emptySub: {
    fontSize: 13,
    textAlign: 'center',
    lineHeight: 20,
  },
});