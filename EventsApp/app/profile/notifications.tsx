// ─────────────────────────────────────────────────────────────────────────────
// Notifications inbox
//
// The read half of the notification system. /v1/notifications, /unread,
// /:id/read and /read-all have existed since migration 000013 and nothing in
// the app called any of them — which meant `is_read` was never set, and the
// iOS badge (driven by the server's unread count) only ever counted upward.
// A user could not clear it by any means available to them.
//
// Reading a notification happens on TAP, not on scroll-into-view. Marking
// everything read just because it rendered would make the badge meaningless —
// the point of the count is "things you haven't dealt with", not "things that
// have been on screen".
//
// Tap routing goes through routeForNotification() in lib/push.ts — the SAME
// function the push-tap handler uses. That's deliberate: a notification tapped
// from the tray and the same notification tapped in this list must land on the
// same screen, and one shared function is the only way to guarantee that stays
// true as new notification types are added.
// ─────────────────────────────────────────────────────────────────────────────

import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  View, Text, TouchableOpacity, ActivityIndicator,
  RefreshControl, FlatList, StyleSheet,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Ionicons, Feather } from '@expo/vector-icons';
import { useRouter, useFocusEffect } from 'expo-router';
import { useAuthStore } from '../../store/authStore';
import { useAppTheme } from '../../theme/colors';
import { api } from '../../lib/api';
import { routeForNotification, syncBadgeFromServer, type PushPayload } from '../../lib/push';

// ─── Types ───────────────────────────────────────────────────────────────────

interface NotificationItem {
  id: string;
  type: string;
  title: string;
  body: string;
  data?: PushPayload | null;
  is_read: boolean;
  created_at: string;
}

interface NotificationsResponse {
  data: NotificationItem[] | null;
  next_cursor: string | null;
}

// ─── Presentation ────────────────────────────────────────────────────────────

// Icon + accent per type. Kept in one place so a new notification type is a
// single-line change here rather than a switch buried in the row component.
// The fallback matters: an unrecognised type from a newer backend still renders
// sensibly instead of showing a blank circle.
const TYPE_STYLE: Record<string, { icon: keyof typeof Ionicons.glyphMap; color: string }> = {
  booking_confirmed:    { icon: 'ticket',            color: '#00e5a0' },
  refund_processed:     { icon: 'cash-outline',      color: '#00e5a0' },
  event_reminder:       { icon: 'alarm-outline',     color: '#29c5f6' },
  tickets_selling_fast: { icon: 'flame',             color: '#ffb830' },
  event_sold_out:       { icon: 'lock-closed',       color: '#ff6b6b' },
  event_cancelled:      { icon: 'close-circle',      color: '#ff6b6b' },
  kyc_approved:         { icon: 'shield-checkmark',  color: '#b57bee' },
};
const FALLBACK_STYLE = { icon: 'notifications' as const, color: '#ff2d55' };

/** "3m", "2h", "5d", then an absolute date once it stops being useful as a delta. */
function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '';
  const secs = Math.floor((Date.now() - then) / 1000);

  if (secs < 60)     return 'now';
  if (secs < 3600)   return `${Math.floor(secs / 60)}m`;
  if (secs < 86400)  return `${Math.floor(secs / 3600)}h`;
  if (secs < 604800) return `${Math.floor(secs / 86400)}d`;

  return new Date(then).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
}

// ─── Row ─────────────────────────────────────────────────────────────────────

function NotificationRow({
  item, colors, isDark, onPress,
}: {
  item: NotificationItem;
  colors: any;
  isDark: boolean;
  onPress: () => void;
}) {
  const style = TYPE_STYLE[item.type] ?? FALLBACK_STYLE;

  return (
    <TouchableOpacity activeOpacity={0.7} onPress={onPress}>
      <View
        style={[
          rs.row,
          {
            // Unread rows get a tint rather than bold text alone — it survives
            // being glanced at, and works for anyone who can't easily tell
            // font weights apart.
            backgroundColor: item.is_read
              ? 'transparent'
              : isDark ? 'rgba(255,45,85,0.06)' : 'rgba(255,45,85,0.04)',
          },
        ]}
      >
        <View style={[rs.iconWrap, { backgroundColor: `${style.color}1f` }]}>
          <Ionicons name={style.icon} size={18} color={style.color} />
        </View>

        <View style={rs.body}>
          <View style={rs.titleLine}>
            <Text
              style={[
                rs.title,
                { color: colors.text, fontWeight: item.is_read ? '500' : '700' },
              ]}
              numberOfLines={1}
            >
              {item.title}
            </Text>
            <Text style={[rs.time, { color: colors.subText }]}>
              {relativeTime(item.created_at)}
            </Text>
          </View>

          <Text style={[rs.text, { color: colors.subText }]} numberOfLines={2}>
            {item.body}
          </Text>
        </View>

        {!item.is_read && <View style={rs.dot} />}
      </View>
    </TouchableOpacity>
  );
}

// ─── Screen ──────────────────────────────────────────────────────────────────

export default function NotificationsScreen() {
  const router   = useRouter();
  const insets   = useSafeAreaInsets();
  const { user } = useAuthStore();
  const { colors, isDark } = useAppTheme();

  const [items,       setItems]       = useState<NotificationItem[]>([]);
  const [loading,     setLoading]     = useState(true);
  const [refreshing,  setRefreshing]  = useState(false);
  const [cursor,      setCursor]      = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [markingAll,  setMarkingAll]  = useState(false);

  const unreadCount = items.filter((n) => !n.is_read).length;

  // FlatList fires onEndReached several times before a state update lands, so
  // the in-flight guard has to be a ref. A `loadingMore` state check reads
  // stale and lets two identical page requests through.
  const inFlight = useRef(false);

  const load = useCallback(async (replace = false) => {
    if (!user) { setLoading(false); return; }
    if (inFlight.current) return;
    inFlight.current = true;

    try {
      replace ? setLoading(true) : setLoadingMore(true);

      const params: Record<string, string> = { limit: '20' };
      if (!replace && cursor) params.cursor = cursor;

      const { data } = await api.get<NotificationsResponse>('/v1/notifications', { params });
      const page = data.data ?? [];

      setItems((prev) => {
        if (replace) return page;
        // Merge by id. Belt-and-braces: the server's keyset pagination should
        // never hand back a row twice, but a duplicate key is a hard React
        // error that blanks the list, and dropping a repeat is free. Cheap
        // insurance against a paging bug ever taking the screen down again.
        const seen = new Set(prev.map((n) => n.id));
        return [...prev, ...page.filter((n) => !seen.has(n.id))];
      });
      setCursor(data.next_cursor);
    } catch {
      // Leave whatever is already on screen. A failed page-2 fetch shouldn't
      // blank out page 1.
    } finally {
      inFlight.current = false;
      setLoading(false);
      setLoadingMore(false);
      setRefreshing(false);
    }
  }, [user, cursor]);

  useEffect(() => { void load(true); }, [user?.id]);

  // Re-pull on focus so the list reflects anything that arrived by push while
  // the user was on another screen.
  const didMount = useRef(false);
  useFocusEffect(
    useCallback(() => {
      // Skip the first focus — the mount effect above already loaded. Without
      // this the screen fires two identical requests on open.
      if (!didMount.current) { didMount.current = true; return; }
      setCursor(null);
      void load(true);
    }, [user?.id])
  );

  const onRefresh = useCallback(() => {
    setRefreshing(true);
    setCursor(null);
    void load(true);
  }, [load]);

  const handlePress = useCallback(async (item: NotificationItem) => {
    // Optimistic: flip it locally first so the row responds instantly. If the
    // PATCH fails the next refresh corrects it — a notification briefly shown
    // as read when it isn't is a much smaller problem than a laggy tap.
    if (!item.is_read) {
      setItems((prev) => prev.map((n) => (n.id === item.id ? { ...n, is_read: true } : n)));
      api.patch(`/v1/notifications/${item.id}/read`)
        .then(() => syncBadgeFromServer())
        .catch(() => {});
    }

    const route = routeForNotification(item.data ?? undefined);
    if (route) router.push(route as any);
  }, [router]);

  const handleMarkAll = useCallback(async () => {
    if (markingAll || unreadCount === 0) return;
    setMarkingAll(true);
    setItems((prev) => prev.map((n) => ({ ...n, is_read: true })));
    try {
      await api.post('/v1/notifications/read-all');
      await syncBadgeFromServer();
    } catch {
      void load(true); // roll back to server truth
    } finally {
      setMarkingAll(false);
    }
  }, [markingAll, unreadCount, load]);

  // ── Content ────────────────────────────────────────────────────────────────

  const renderContent = () => {
    if (!user) {
      return (
        <View style={ss.empty}>
          <Ionicons name="notifications-off-outline" size={44} color={colors.iconColor} />
          <Text style={[ss.emptyTitle, { color: colors.text }]}>Sign in to see notifications</Text>
          <TouchableOpacity
            style={ss.signInBtn}
            onPress={() => router.push('/auth/login' as any)}
            activeOpacity={0.85}
          >
            <Text style={ss.signInText}>Sign in</Text>
          </TouchableOpacity>
        </View>
      );
    }

    if (loading) {
      return (
        <View style={ss.empty}>
          <ActivityIndicator color="#ff2d55" />
        </View>
      );
    }

    if (items.length === 0) {
      return (
        <View style={ss.empty}>
          <Ionicons name="notifications-outline" size={44} color={colors.iconColor} />
          <Text style={[ss.emptyTitle, { color: colors.text }]}>Nothing yet</Text>
          <Text style={[ss.emptyBody, { color: colors.subText }]}>
            Booking confirmations and reminders for events you follow will show up here.
          </Text>
        </View>
      );
    }

    return (
      <FlatList
        data={items}
        keyExtractor={(n) => n.id}
        renderItem={({ item }) => (
          <NotificationRow
            item={item}
            colors={colors}
            isDark={isDark}
            onPress={() => handlePress(item)}
          />
        )}
        ItemSeparatorComponent={() => (
          <View style={[ss.sep, { backgroundColor: colors.dividerColor }]} />
        )}
        contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor="#ff2d55" />
        }
        onEndReachedThreshold={0.4}
        onEndReached={() => {
          if (cursor && !loadingMore) void load(false);
        }}
        ListFooterComponent={
          loadingMore ? (
            <View style={ss.footer}><ActivityIndicator color="#ff2d55" /></View>
          ) : null
        }
        showsVerticalScrollIndicator={false}
      />
    );
  };

  return (
    <View style={[ss.root, { backgroundColor: colors.background }]}>
      <View style={[ss.header, { paddingTop: insets.top + 12, backgroundColor: colors.background }]}>
        <TouchableOpacity style={ss.backBtn} onPress={() => router.back()} activeOpacity={0.7}>
          <Feather name="arrow-left" size={20} color={colors.text} />
        </TouchableOpacity>

        <View style={ss.headerCenter}>
          <Text style={[ss.headerTitle, { color: colors.text }]}>Notifications</Text>
          {unreadCount > 0 && (
            <Text style={[ss.headerCount, { color: colors.subText }]}>
              {unreadCount} unread
            </Text>
          )}
        </View>

        {/* Fixed-width slot keeps the title optically centred whether or not
            the action is showing. */}
        <View style={ss.headerAction}>
          {unreadCount > 0 && (
            <TouchableOpacity onPress={handleMarkAll} activeOpacity={0.7} disabled={markingAll}>
              <Text style={[ss.markAll, { opacity: markingAll ? 0.4 : 1 }]}>Read all</Text>
            </TouchableOpacity>
          )}
        </View>
      </View>

      <View style={[ss.headerDivider, { backgroundColor: colors.dividerColor }]} />

      {renderContent()}
    </View>
  );
}

// ─── Styles ──────────────────────────────────────────────────────────────────

const ss = StyleSheet.create({
  root: { flex: 1 },
  header: {
    flexDirection: 'row', alignItems: 'center',
    paddingHorizontal: 16, paddingBottom: 12,
  },
  backBtn: { width: 40, height: 40, alignItems: 'flex-start', justifyContent: 'center' },
  headerCenter: { flex: 1, alignItems: 'center' },
  headerTitle: { fontSize: 17, fontWeight: '700' },
  headerCount: { fontSize: 12, marginTop: 2 },
  headerAction: { width: 64, alignItems: 'flex-end' },
  markAll: { fontSize: 13, fontWeight: '600', color: '#ff2d55' },
  headerDivider: { height: StyleSheet.hairlineWidth, width: '100%' },
  sep: { height: StyleSheet.hairlineWidth, marginLeft: 66 },
  footer: { paddingVertical: 20 },
  empty: { flex: 1, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 40, gap: 10 },
  emptyTitle: { fontSize: 16, fontWeight: '700', marginTop: 6 },
  emptyBody: { fontSize: 13, textAlign: 'center', lineHeight: 19 },
  signInBtn: {
    marginTop: 14, backgroundColor: '#ff2d55',
    paddingHorizontal: 26, paddingVertical: 11, borderRadius: 22,
  },
  signInText: { color: '#fff', fontWeight: '700', fontSize: 14 },
});

const rs = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'flex-start', paddingHorizontal: 16, paddingVertical: 14 },
  iconWrap: {
    width: 38, height: 38, borderRadius: 19,
    alignItems: 'center', justifyContent: 'center', marginRight: 12,
  },
  body: { flex: 1 },
  titleLine: { flexDirection: 'row', alignItems: 'center', marginBottom: 3 },
  title: { flex: 1, fontSize: 14.5, marginRight: 8 },
  time: { fontSize: 11.5 },
  text: { fontSize: 13, lineHeight: 18 },
  dot: {
    width: 8, height: 8, borderRadius: 4,
    backgroundColor: '#ff2d55', marginLeft: 10, marginTop: 6,
  },
});