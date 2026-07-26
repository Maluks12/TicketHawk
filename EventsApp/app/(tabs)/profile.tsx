import React, { useCallback, useState } from 'react';
import {
  View, Text, ScrollView, TouchableOpacity, Alert, Linking, StyleSheet,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Feather, Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { useAuthStore } from '../../store/authStore';
import { openStudioApp } from '../../lib/studioLink';
import { openLegal } from '../../lib/legalLinks';
import { s, sheet } from '../../styles/profile';
import { useThemeStore, ThemeMode } from '../../store/themeStore';
import { useAppTheme } from '../../theme/colors';

const APP_VERSION = '1.0.0';

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
      <Text className={s.rowLabel} style={colors && !destructive ? { color: colors.text } : undefined}>{label}</Text>
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

const Avatar = ({ name, email }: { name: string; email: string }) => {
  const initials = name.split(' ').map(w => w[0]).join('').toUpperCase().slice(0, 2);
  return (
    <View className={s.avatarSection}>
      <View className={s.avatarCircle}>
        <Text className={s.avatarInitials}>{initials || '?'}</Text>
      </View>
      <View>
        <Text className={s.avatarName}>{name}</Text>
        <Text className={s.avatarEmail}>{email}</Text>
      </View>
    </View>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function ProfileScreen() {
  const insets             = useSafeAreaInsets();
  const router             = useRouter();
  const { user, logout }   = useAuthStore();
  const { mode, setMode }  = useThemeStore();
  const { isDark, colors } = useAppTheme();

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
        {user && <Avatar name={fullName} email={user.email} />}

        {/* ── Organiser Studio entry — opens the read-only dashboard ──────── */}
        {isOrganiser && (
          <View className={s.section}>
            <Text className={s.sectionTitle} style={{ color: colors.subText }}>ORGANISER</Text>
            <TouchableOpacity
              activeOpacity={0.85}
              onPress={() => openStudioApp()}
              style={{
                flexDirection: 'row',
                alignItems: 'center',
                gap: 14,
                padding: 16,
                borderRadius: 16,
                backgroundColor: isDark ? 'rgba(110,123,255,0.10)' : 'rgba(79,70,229,0.07)',
                borderWidth: StyleSheet.hairlineWidth,
                borderColor: 'rgba(110,123,255,0.30)',
              }}
            >
              <View
                style={{
                  width: 46,
                  height: 46,
                  borderRadius: 13,
                  backgroundColor: '#4F46E5',
                  alignItems: 'center',
                  justifyContent: 'center',
                }}
              >
                <Feather name="bar-chart-2" size={22} color="#fff" />
              </View>
              <View style={{ flex: 1, minWidth: 0 }}>
                <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                  <Text style={{ fontSize: 16, fontWeight: '800', color: colors.text }}>
                    Organiser Dashboard
                  </Text>
                  <View
                    style={{
                      backgroundColor: 'rgba(110,123,255,0.16)',
                      borderRadius: 5,
                      paddingHorizontal: 6,
                      paddingVertical: 2,
                    }}
                  >
                    <Text style={{ fontSize: 8.5, fontWeight: '800', letterSpacing: 1, color: '#7C8CFF' }}>
                      STUDIO
                    </Text>
                  </View>
                </View>
                <Text style={{ fontSize: 12.5, color: colors.subText, marginTop: 2 }}>
                  Opens the TicketHawk Studio app
                </Text>
              </View>
              <Feather name="arrow-up-right" size={18} color="#7C8CFF" />
            </TouchableOpacity>
          </View>
        )}

        {user && (
          <>
            <Section title="ACCOUNT" colors={colors}>
              <Row
                icon={<Feather name="user" size={16} color={colors.iconColor} />}
                label="Edit profile"
                onPress={() => {}}
                colors={colors}
              />
              <View style={[sheet.divider, { backgroundColor: colors.dividerColor }]} />
              <Row
                icon={<Ionicons name="notifications-outline" size={16} color={colors.iconColor} />}
                label="Notifications"
                onPress={() => {}}
                colors={colors}
              />
              <View style={[sheet.divider, { backgroundColor: colors.dividerColor }]} />
              <Row
                icon={<Feather name="lock" size={16} color={colors.iconColor} />}
                label="Change password"
                onPress={() => {}}
                colors={colors}
              />
              <View style={[sheet.divider, { backgroundColor: colors.dividerColor }]} />
              {/* ── Following Events ──────────────────────────────────────── */}
              <Row
                icon={<Ionicons name="heart-outline" size={16} color={colors.iconColor} />}
                label="Following Events"
                onPress={() => router.push('/profile/following' as any)}
                colors={colors}
              />
              <View style={[sheet.divider, { backgroundColor: colors.dividerColor }]} />
              {/* Event staff: redeem a join code / scan events you work */}
              <Row
                icon={<Ionicons name="qr-code-outline" size={16} color={colors.iconColor} />}
                label="Event staff"
                onPress={() => openStudioApp()}
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
          <Section>
            <Row
              icon={<Feather name="log-out" size={16} color="#ff4d4d" />}
              label="Log out"
              onPress={handleLogout}
              destructive
              hideChevron
            />
          </Section>
        )}

        <Text className={s.version} style={{ color: colors.subText }}>v{APP_VERSION}</Text>
      </ScrollView>
    </View>
  );
}