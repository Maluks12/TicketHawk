import React, { useEffect, useCallback, useState } from 'react';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { View, Text, ScrollView, TouchableOpacity, Alert, Linking, StyleSheet, Switch } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Feather, Ionicons } from '@expo/vector-icons';
import { useRouter, useFocusEffect } from 'expo-router';
import { useAuthStore } from '../../store/authStore';
import { openLegal } from '../../lib/legalLinks';
import { api } from '../../lib/api';
import {
  PUSH_PREF_KEY,
  pushUnavailableReason,
  registerForPushNotifications,
  setPushPreference,
  unregisterPushNotifications,
} from '../../lib/push';
import { s, sheet } from '../../styles/profile';
import { useThemeStore, ThemeMode } from '../../store/themeStore';
import { useAppTheme } from '../../theme/colors';

const APP_VERSION = '1.0.3';

// ─── Row ──────────────────────────────────────────────────────────────────────

interface RowProps {
  icon: React.ReactNode;
  label: string;
  value?: string;
  valueColor?: string;
  onPress?: () => void;
  destructive?: boolean;
  hideChevron?: boolean;
  colors?: { text: string; subText: string };
}

const Row = ({ icon, label, value, valueColor, onPress, destructive, hideChevron, colors }: RowProps) => (
  <TouchableOpacity className={s.row} onPress={onPress} activeOpacity={onPress ? 0.65 : 1} disabled={!onPress}>
    <View className={s.rowLeft}>
      <View className={s.rowIconWrap}>{icon}</View>
      {/* Destructive rows must set their colour explicitly. The s.rowLabel
          class hardcodes text-white, so falling through to it (as this did)
          renders white-on-white in light mode — the Log out row was invisible.
          Red also matches the icon and the iOS destructive convention. */}
      <Text
        className={s.rowLabel}
        style={destructive ? { color: '#ff4d4d' } : colors ? { color: colors.text } : undefined}
      >
        {label}
      </Text>
    </View>
    <View className={s.rowRight}>
      {value ? (
        <Text className={s.rowValue} style={valueColor ? { color: valueColor } : colors ? { color: colors.subText } : undefined}>
          {value}
        </Text>
      ) : null}
      {!hideChevron && onPress && (
        <Feather name="chevron-right" size={16} color={colors ? colors.subText : 'rgba(255,255,255,0.25)'} />
      )}
    </View>
  </TouchableOpacity>
);

// ─── Section ──────────────────────────────────────────────────────────────────

const Section = ({
  title, children, colors,
}: {
  title?: string;
  children: React.ReactNode;
  colors?: { subText: string; sectionCardBg: string; dividerColor: string };
}) => (
  <View className={s.section}>
    {title && <Text className={s.sectionTitle} style={colors ? { color: colors.subText } : undefined}>{title}</Text>}
    <View style={[sheet.sectionCard, colors ? { backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor } : undefined]}>
      {children}
    </View>
  </View>
);

// ─── Avatar ───────────────────────────────────────────────────────────────────

// s.avatarName and s.avatarEmail hardcode text-white / text-white/40, which is
// invisible on the light theme's white background. Same root cause as the
// Log out row: a NativeWind class pins the colour and nothing overrides it.
// The initials stay white — they sit on the solid pink circle, not the page.
const Avatar = ({ name, email, colors }: { name: string; email: string; colors?: { text: string; subText: string } }) => {
  const initials = name.split(' ').map(w => w[0]).join('').toUpperCase().slice(0, 2);
  return (
    <View className={s.avatarSection}>
      <View className={s.avatarCircle}>
        <Text className={s.avatarInitials}>{initials || '?'}</Text>
      </View>
      <View>
        <Text className={s.avatarName} style={colors ? { color: colors.text } : undefined}>{name}</Text>
        <Text className={s.avatarEmail} style={colors ? { color: colors.subText } : undefined}>{email}</Text>
      </View>
    </View>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

// ── Notifications preference ─────────────────────────────────────────────────
//
// This switch is now live: it controls whether this DEVICE is registered for
// push with the backend.
//
//   ON  → request OS permission if needed, obtain an Expo token, POST it to
//         /v1/notifications/push-token
//   OFF → DELETE /v1/notifications/push-token, so the server stops targeting
//         this device
//
// Turning it off does NOT stop notifications being generated — the in-app feed
// (/v1/notifications) keeps filling up exactly as before, and unread counts
// still work. It stops the phone buzzing. That's the honest reading of a
// "Notifications" switch in a ticketing app: you still want to find your
// booking confirmation in the app, you just don't want to be interrupted.
//
// The one case the switch cannot handle is a hard OS-level denial. Once someone
// taps "Don't allow", iOS and Android will not show the prompt again, and
// requestPermissionsAsync returns 'denied' instantly. There is nothing the app
// can do but send them to system settings, which is what the alert below does.
const NOTIFS_KEY = PUSH_PREF_KEY;

function ToggleRow({
  icon, label, value, onValueChange, colors, disabled,
}: {
  icon: React.ReactNode;
  label: string;
  value: boolean | null;   // null = still hydrating
  onValueChange: (v: boolean) => void;
  colors: any;
  // Set while a permission prompt / network round-trip is in flight. Without
  // it, rapid toggling fires overlapping register and unregister calls whose
  // completion order decides the final server state.
  disabled?: boolean;
}) {
  return (
    <View
      style={{
        flexDirection: 'row',
        alignItems: 'center',
        gap: 14,
        paddingHorizontal: 16,
        paddingVertical: 14,
      }}
    >
      {icon}
      <Text style={{ flex: 1, fontSize: 15, color: colors.text }}>{label}</Text>
      {value === null ? (
        // Placeholder holding the Switch's exact footprint (iOS 51x31) so the
        // row doesn't shift when the real control mounts. Rendering nothing
        // here would trade a flashing toggle for a jumping layout.
        <View style={{ width: 51, height: 31 }} />
      ) : (
        <Switch
          value={value}
          onValueChange={onValueChange}
          disabled={disabled}
          trackColor={{ false: colors.dividerColor, true: '#ff2d55' }}
          thumbColor="#fff"
          ios_backgroundColor={colors.dividerColor}
        />
      )}
    </View>
  );
}

export default function ProfileScreen() {
  // null until AsyncStorage has been read. Starting at `true` meant the Switch
  // mounted ON and then animated itself OFF a frame later for anyone who had
  // disabled notifications — a toggle that appears to flip on its own every
  // time you open the screen. The Switch does not animate on its FIRST render,
  // so the fix is to not mount it until the stored value is known.
  const [notifsOn, setNotifsOn]   = useState<boolean | null>(null);
  const [notifsBusy, setNotifsBusy] = useState(false);

  useEffect(() => {
    AsyncStorage.getItem(NOTIFS_KEY)
      .then((v) => setNotifsOn(v === null ? true : v === '1'))  // unset = on, matching lib/push
      .catch(() => setNotifsOn(true));
  }, []);

  const setNotifications = useCallback(async (v: boolean) => {
    if (notifsOn === null) return;  // not hydrated yet — ignore stray taps
    // Optimistic: the switch moves immediately. If registration turns out to be
    // impossible we roll it back below rather than leaving the UI claiming a
    // state the device isn't actually in.
    setNotifsOn(v);
    setNotifsBusy(true);
    await setPushPreference(v);

    try {
      if (!v) {
        await unregisterPushNotifications();
        return;
      }

      // Turning it ON is the one moment where prompting for OS permission is
      // unambiguously wanted — the user just asked for notifications.
      const result = await registerForPushNotifications(true);
      if (result.ok) return;

      // Every failure path below rolls the switch back, because leaving it ON
      // when no token was registered is the exact dishonesty this replaced.
      setNotifsOn(false);
      await setPushPreference(false);

      if (result.reason === 'denied') {
        Alert.alert(
          'Notifications are blocked',
          "Your phone is blocking notifications for TicketHawk. You'll need to allow them in system settings — we can't re-ask from inside the app.",
          [
            { text: 'Not now', style: 'cancel' },
            { text: 'Open settings', onPress: () => Linking.openSettings() },
          ],
        );
        return;
      }

      if (result.reason === 'unavailable') {
        const why =
          pushUnavailableReason() === 'simulator'
            ? 'Push notifications only work on a physical device.'
            : 'Push notifications need a development or production build — they no longer work in Expo Go.';
        Alert.alert('Not available here', why);
        return;
      }

      Alert.alert(
        "Couldn't turn on notifications",
        'Something went wrong setting this up. Please try again in a moment.',
      );
    } finally {
      setNotifsBusy(false);
    }
    // notifsOn is in the deps because the hydration guard above reads it. With
    // an empty array this closure would capture the initial null forever and
    // silently swallow every tap.
  }, [notifsOn]);

  const insets             = useSafeAreaInsets();
  const router             = useRouter();
  const { user, logout }   = useAuthStore();
  const { mode, setMode }  = useThemeStore();
  const { isDark, colors } = useAppTheme();

  // Unread badge on the bell. Refreshed whenever this screen regains focus —
  // which covers returning from the inbox after marking things read, and
  // coming back into the app after tapping a push.
  const [unreadCount, setUnreadCount] = useState(0);

  useFocusEffect(
    useCallback(() => {
      if (!user) { setUnreadCount(0); return; }
      let cancelled = false;
      api.get<{ unread_count: number }>('/v1/notifications/unread')
        .then(({ data }) => { if (!cancelled) setUnreadCount(data.unread_count ?? 0); })
        .catch(() => {}); // a missing badge is not worth an error state
      return () => { cancelled = true; };
    }, [user?.id])
  );

  const themeModeLabel: Record<ThemeMode, string> = { auto: 'Auto', light: 'Light', dark: 'Dark' };
  const cycleTheme = () => {
    const order: ThemeMode[] = ['auto', 'light', 'dark'];
    setMode(order[(order.indexOf(mode) + 1) % order.length]);
  };

  const handleLogout = useCallback(() => {
    Alert.alert('Log out', 'Are you sure you want to log out?', [
      { text: 'Cancel', style: 'cancel' },
      { text: 'Log out', style: 'destructive', onPress: async () => { try { await logout(); } catch {} } },
    ]);
  }, [logout]);

  const fullName = user?.first_name ?? user?.email?.split('@')[0] ?? 'Guest';
  const isOrganiser = user?.role === 'organizer' || user?.role === 'admin';

  return (
    <View className={s.root} style={{ backgroundColor: colors.background }}>

      {/* ── Header ────────────────────────────────────────────────────────── */}
      <View
        className={s.header}
        style={{ paddingTop: insets.top + 10, backgroundColor: colors.background }}
      >
        <Text className={s.headerTitle} style={{ color: colors.text }}>Profile</Text>

        {/* Notifications bell — only shown when signed in, since the inbox is
            per-user and there is nothing to show a logged-out visitor. The
            unread count is fetched on focus rather than polled: notifications
            arrive by push, and a push already updates the app-icon badge. */}
        {user && (
          <TouchableOpacity
            style={ph.bell}
            activeOpacity={0.7}
            onPress={() => router.push('/profile/notifications' as any)}
            accessibilityLabel={
              unreadCount > 0 ? `Notifications, ${unreadCount} unread` : 'Notifications'
            }
          >
            <Ionicons name="notifications-outline" size={22} color={colors.text} />
            {unreadCount > 0 && (
              <View style={ph.badge}>
                <Text style={ph.badgeText}>{unreadCount > 99 ? '99+' : unreadCount}</Text>
              </View>
            )}
          </TouchableOpacity>
        )}
      </View>

      <ScrollView
        className={s.scroll}
        contentContainerStyle={{ paddingBottom: insets.bottom + 32 }}
        showsVerticalScrollIndicator={false}
      >

        {/* ── Logged-out ─────────────────────────────────────────────────── */}
        {!user && (
          <>
            <Section colors={colors}>
              <TouchableOpacity
                className={s.signInRow}
                activeOpacity={0.7}
                onPress={() => router.push('/auth/login' as any)}
              >
                <View
                  className={s.signInAvatar}
                  style={{ backgroundColor: isDark ? 'rgba(255,255,255,0.07)' : 'rgba(0,0,0,0.06)' }}
                >
                  <Feather name="user" size={26} color={colors.iconColor} />
                </View>
                <View style={{ flex: 1 }}>
                  <Text className={s.signInTitle} style={{ color: colors.text }}>Sign up or log in</Text>
                  <Text className={s.signInSub} style={{ color: colors.subText }}>Access your tickets and bookings</Text>
                </View>
                <Feather name="chevron-right" size={16} color={colors.iconColor} />
              </TouchableOpacity>
            </Section>

            <Section title="APPEARANCE" colors={colors}>
              <Row
                icon={<Feather name={isDark ? 'moon' : 'sun'} size={16} color={colors.iconColor} />}
                label="Theme"
                value={themeModeLabel[mode]}
                onPress={cycleTheme}
                colors={colors}
              />
            </Section>
          </>
        )}

        {/* ── Logged-in ──────────────────────────────────────────────────── */}
        {user && <Avatar name={fullName} email={user.email} colors={colors} />}

        {/* ── Organiser Studio entry — opens the read-only dashboard ──────── */}
        {user && (
          <>
            <Section title="ACCOUNT" colors={colors}>
              <Row
                icon={<Feather name="user" size={16} color={colors.iconColor} />}
                label="Edit profile"
                onPress={() => router.push('/profile/edit' as any)}
                colors={colors}
              />
              <View style={[sheet.divider, { backgroundColor: colors.dividerColor }]} />
              <ToggleRow
                icon={<Ionicons name="notifications-outline" size={16} color={colors.iconColor} />}
                label="Notifications"
                value={notifsOn}
                onValueChange={setNotifications}
                disabled={notifsBusy}
                colors={colors}
              />
              <View style={[sheet.divider, { backgroundColor: colors.dividerColor }]} />
              {/* ── Following ─────────────────────────────────────────────────
                  "Following", not "Following Events" — that screen now has two
                  tabs, Events and Organisers, so naming it after one of them
                  would misdescribe half of it. */}
              <Row
                icon={<Ionicons name="heart-outline" size={16} color={colors.iconColor} />}
                label="Following"
                onPress={() => router.push('/profile/following' as any)}
                colors={colors}
              />
            </Section>

            <Section title="APPEARANCE" colors={colors}>
              <Row
                icon={<Feather name={isDark ? 'moon' : 'sun'} size={16} color={colors.iconColor} />}
                label="Theme"
                value={themeModeLabel[mode]}
                onPress={cycleTheme}
                colors={colors}
              />
            </Section>
          </>
        )}

        <Section title="SUPPORT" colors={colors}>
          <Row
            icon={<Feather name="help-circle" size={16} color={colors.iconColor} />}
            label="Help & support"
            onPress={() => Linking.openURL('mailto:support@ticket-hawk.com')}
            colors={colors}
          />
          <View style={[sheet.divider, { backgroundColor: colors.dividerColor }]} />
          <Row
            icon={<Feather name="file-text" size={16} color={colors.iconColor} />}
            label="Terms of use"
            onPress={() => openLegal('terms')}
            colors={colors}
          />
          <View style={[sheet.divider, { backgroundColor: colors.dividerColor }]} />
          <Row
            icon={<Feather name="shield" size={16} color={colors.iconColor} />}
            label="Privacy policy"
            onPress={() => openLegal('privacy')}
            colors={colors}
          />
        </Section>

        {user && (
          <Section colors={colors}>
            <Row
              icon={<Feather name="log-out" size={16} color="#ff4d4d" />}
              label="Log out"
              onPress={handleLogout}
              destructive
              hideChevron
            />
            {/* Account deletion must be reachable from inside the app — Apple
                App Store Guideline 5.1.1(v). It sits below Log out, in the same
                destructive group, because that is where people look for it.
                The confirmation, password re-entry and the warning about
                upcoming tickets all live on the screen itself rather than in an
                alert here, so nothing irreversible can happen from one tap. */}
            <View style={[sheet.divider, { backgroundColor: colors.dividerColor }]} />
            <Row
              icon={<Feather name="trash-2" size={16} color="#ff4d4d" />}
              label="Delete account"
              onPress={() => router.push('/profile/delete-account' as any)}
              destructive
              colors={colors}
            />
          </Section>
        )}

        <Text className={s.version} style={{ color: colors.subText }}>v{APP_VERSION}</Text>
      </ScrollView>
    </View>
  );
}

// Header bell + unread badge. Absolute positioning keeps the "Profile" title
// in its original place rather than shifting it when the badge appears.
const ph = StyleSheet.create({
  bell: { position: 'absolute', right: 18, bottom: 18, padding: 6 },
  badge: {
    position: 'absolute', top: 0, right: 0,
    minWidth: 17, height: 17, borderRadius: 8.5,
    backgroundColor: '#ff2d55',
    alignItems: 'center', justifyContent: 'center',
    paddingHorizontal: 4,
  },
  badgeText: { color: '#fff', fontSize: 10, fontWeight: '700' },
});