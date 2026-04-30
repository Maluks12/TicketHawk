import React, { useCallback } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Alert,
  Linking,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Feather, Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { useAuthStore } from '../../store/authStore';

// ─── App version ──────────────────────────────────────────────────────────────
const APP_VERSION = '1.0.0';

// ─── Row ─────────────────────────────────────────────────────────────────────

interface RowProps {
  icon: React.ReactNode;
  label: string;
  value?: string;
  valueColor?: string;
  onPress?: () => void;
  destructive?: boolean;
  hideChevron?: boolean;
}

const Row = ({ icon, label, value, valueColor, onPress, destructive, hideChevron }: RowProps) => (
  <TouchableOpacity
    style={styles.row}
    onPress={onPress}
    activeOpacity={onPress ? 0.65 : 1}
    disabled={!onPress}
  >
    <View style={styles.rowLeft}>
      <View style={styles.rowIconWrap}>{icon}</View>
      <Text style={[styles.rowLabel, destructive && styles.rowLabelDestructive]}>{label}</Text>
    </View>
    <View style={styles.rowRight}>
      {value ? (
        <Text style={[styles.rowValue, valueColor ? { color: valueColor } : undefined]}>{value}</Text>
      ) : null}
      {!hideChevron && onPress && (
        <Feather name="chevron-right" size={16} color="rgba(255,255,255,0.25)" />
      )}
    </View>
  </TouchableOpacity>
);

// ─── Section ─────────────────────────────────────────────────────────────────

const Section = ({ title, children }: { title?: string; children: React.ReactNode }) => (
  <View style={styles.section}>
    {title && <Text style={styles.sectionTitle}>{title}</Text>}
    <View style={styles.sectionCard}>{children}</View>
  </View>
);

// ─── Avatar ──────────────────────────────────────────────────────────────────

const Avatar = ({ name, email }: { name: string; email: string }) => {
  const initials = name
    .split(' ')
    .map(w => w[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);

  return (
    <View style={styles.avatarSection}>
      <View style={styles.avatarCircle}>
        <Text style={styles.avatarInitials}>{initials || '?'}</Text>
      </View>
      <View>
        <Text style={styles.avatarName}>{name}</Text>
        <Text style={styles.avatarEmail}>{email}</Text>
      </View>
    </View>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function ProfileScreen() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { user, logout } = useAuthStore();

  const handleLogout = useCallback(() => {
    Alert.alert(
      'Log out',
      'Are you sure you want to log out?',
      [
        { text: 'Cancel', style: 'cancel' },
        {
          text: 'Log out',
          style: 'destructive',
          onPress: async () => {
            try {
              await logout();
            } catch {}
          },
        },
      ],
    );
  }, [logout]);

  const fullName = user?.first_name ?? user?.email?.split('@')[0] ?? 'Guest';

  return (
    <View style={styles.root}>
      {/* Header */}
      <View style={[styles.header, { paddingTop: insets.top + 10 }]}>
        <Text style={styles.headerTitle}>Profile</Text>
      </View>

      <ScrollView
        contentContainerStyle={[styles.scroll, { paddingBottom: insets.bottom + 32 }]}
        showsVerticalScrollIndicator={false}
      >
        {/* ── Not logged in ── */}
        {!user && (
          <Section>
            <TouchableOpacity
              style={styles.signInRow}
              activeOpacity={0.7}
              onPress={() => router.push('/auth/login')}
            >
              <View style={styles.signInAvatar}>
                <Feather name="user" size={26} color="rgba(255,255,255,0.4)" />
              </View>
              <View style={{ flex: 1 }}>
                <Text style={styles.signInTitle}>Sign up or log in</Text>
                <Text style={styles.signInSub}>Access your tickets and bookings</Text>
              </View>
              <Feather name="chevron-right" size={16} color="rgba(255,255,255,0.25)" />
            </TouchableOpacity>
          </Section>
        )}

        {/* ── Logged in: avatar ── */}
        {user && (
          <Avatar name={fullName} email={user.email} />
        )}

        {/* ── Account ── */}
        {user && (
          <Section title="ACCOUNT">
            <Row
              icon={<Feather name="user" size={16} color="rgba(255,255,255,0.55)" />}
              label="Edit profile"
              onPress={() => {}}
            />
            <View style={styles.divider} />
            <Row
              icon={<Ionicons name="notifications-outline" size={16} color="rgba(255,255,255,0.55)" />}
              label="Notifications"
              onPress={() => {}}
            />
            <View style={styles.divider} />
            <Row
              icon={<Feather name="lock" size={16} color="rgba(255,255,255,0.55)" />}
              label="Change password"
              onPress={() => {}}
            />
          </Section>
        )}

        {/* ── Support ── */}
        <Section title="SUPPORT">
          <Row
            icon={<Feather name="help-circle" size={16} color="rgba(255,255,255,0.55)" />}
            label="Help & support"
            onPress={() => Linking.openURL('mailto:support@example.com')}
          />
          <View style={styles.divider} />
          <Row
            icon={<Feather name="file-text" size={16} color="rgba(255,255,255,0.55)" />}
            label="Terms of use"
            onPress={() => {}}
          />
          <View style={styles.divider} />
          <Row
            icon={<Feather name="shield" size={16} color="rgba(255,255,255,0.55)" />}
            label="Privacy policy"
            onPress={() => {}}
          />
        </Section>

        {/* ── Log out ── */}
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

        {/* ── Version ── */}
        <Text style={styles.version}>v{APP_VERSION}</Text>
      </ScrollView>
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: '#0a0a0a' },

  header: {
    paddingHorizontal: 24,
    paddingBottom: 18,
  },
  headerTitle: {
    fontSize: 28,
    fontWeight: '800',
    color: '#fff',
    letterSpacing: -0.5,
  },

  scroll: {
    paddingHorizontal: 16,
    paddingTop: 4,
    gap: 24,
  },

  // Avatar block
  avatarSection: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 16,
    paddingHorizontal: 4,
    paddingBottom: 4,
  },
  avatarCircle: {
    width: 60,
    height: 60,
    borderRadius: 30,
    backgroundColor: '#ff3c6e',
    alignItems: 'center',
    justifyContent: 'center',
  },
  avatarInitials: {
    fontSize: 22,
    fontWeight: '800',
    color: '#fff',
    letterSpacing: 1,
  },
  avatarName: {
    fontSize: 18,
    fontWeight: '800',
    color: '#fff',
    letterSpacing: -0.2,
  },
  avatarEmail: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.4)',
    marginTop: 2,
  },

  // Sign in row
  signInRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 14,
    padding: 16,
  },
  signInAvatar: {
    width: 50,
    height: 50,
    borderRadius: 25,
    backgroundColor: 'rgba(255,255,255,0.07)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  signInTitle: {
    fontSize: 16,
    fontWeight: '700',
    color: '#fff',
  },
  signInSub: {
    fontSize: 12,
    color: 'rgba(255,255,255,0.35)',
    marginTop: 2,
  },

  // Section
  section: {
    gap: 8,
  },
  sectionTitle: {
    fontSize: 10,
    fontWeight: '800',
    letterSpacing: 2.5,
    color: 'rgba(255,255,255,0.28)',
    marginLeft: 4,
  },
  sectionCard: {
    backgroundColor: 'rgba(255,255,255,0.05)',
    borderRadius: 16,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: 'rgba(255,255,255,0.08)',
    overflow: 'hidden',
  },

  // Row
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 16,
    paddingVertical: 15,
  },
  rowLeft: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 13,
    flex: 1,
  },
  rowIconWrap: {
    width: 22,
    alignItems: 'center',
  },
  rowLabel: {
    fontSize: 15,
    fontWeight: '500',
    color: '#fff',
  },
  rowLabelDestructive: {
    color: '#ff4d4d',
  },
  rowRight: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  rowValue: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.4)',
  },

  divider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: 'rgba(255,255,255,0.07)',
    marginLeft: 51,
  },

  version: {
    textAlign: 'center',
    fontSize: 12,
    color: 'rgba(255,255,255,0.18)',
    marginTop: 4,
  },
});