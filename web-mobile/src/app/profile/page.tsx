'use client';

import { useCallback } from 'react';
import { useRouter } from 'next/navigation';
import {
  User, ChevronRight, Heart, Sun, Moon,
  LogOut, Lock, Trash2,
} from 'lucide-react';
import { useAuthStore } from '@/store/authStore';
import { useThemeStore, type ThemeMode } from '@/store/themeStore';
import { useAppTheme } from '@/themes/colors';

const APP_VERSION = '1.0.0';

// ── Row ───────────────────────────────────────────────────────────────────────
interface RowProps {
  icon: React.ReactNode;
  label: string;
  value?: string;
  valueColor?: string;
  onClick?: () => void;
  destructive?: boolean;
  hideChevron?: boolean;
}

function Row({ icon, label, value, valueColor, onClick, destructive, hideChevron }: RowProps) {
  const { colors } = useAppTheme();
  return (
    <button
      onClick={onClick}
      disabled={!onClick}
      className="flex items-center w-full px-4 py-3.5 gap-3 text-left transition-opacity hover:opacity-70 disabled:hover:opacity-100"
    >
      <div
        className="w-8 h-8 rounded-[8px] flex items-center justify-center shrink-0"
        style={{ backgroundColor: colors.sectionCardBg }}
      >
        {icon}
      </div>
      <span
        className="flex-1 text-sm font-semibold"
        style={{ color: destructive ? '#ff4d4d' : colors.text }}
      >
        {label}
      </span>
      {value && (
        <span className="text-sm font-medium mr-1" style={{ color: valueColor ?? colors.subText }}>
          {value}
        </span>
      )}
      {!hideChevron && onClick && (
        <ChevronRight size={16} color={colors.subText} />
      )}
    </button>
  );
}

// ── Divider ───────────────────────────────────────────────────────────────────
function Divider() {
  const { colors } = useAppTheme();
  return <div className="mx-4 h-px" style={{ backgroundColor: colors.dividerColor }} />;
}

// ── Section ───────────────────────────────────────────────────────────────────
function Section({ title, children }: { title?: string; children: React.ReactNode }) {
  const { colors } = useAppTheme();
  return (
    <div className="mb-6 px-4">
      {title && (
        <p className="text-[10px] font-black tracking-[1.5px] mb-2 px-0.5" style={{ color: colors.subText }}>
          {title}
        </p>
      )}
      <div
        className="rounded-[14px] overflow-hidden border"
        style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }}
      >
        {children}
      </div>
    </div>
  );
}

// ── Avatar ────────────────────────────────────────────────────────────────────
function Avatar({ name, email }: { name: string; email: string }) {
  const { colors } = useAppTheme();
  const initials = name.split(' ').map((w) => w[0]).join('').toUpperCase().slice(0, 2);
  return (
    <div className="flex items-center gap-4 px-4 py-6 mb-2">
      <div
        className="w-16 h-16 rounded-full flex items-center justify-center shrink-0"
        style={{ backgroundColor: '#ff2d55' }}
      >
        <span className="text-xl font-black text-white">{initials || '?'}</span>
      </div>
      <div>
        <p className="text-lg font-black" style={{ color: colors.text }}>{name}</p>
        <p className="text-sm" style={{ color: colors.subText }}>{email}</p>
      </div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────
export default function ProfilePage() {
  const router = useRouter();
  const { user, logout }  = useAuthStore();
  const { mode, setMode } = useThemeStore();
  const { isDark, colors } = useAppTheme();

  const themeModeLabel: Record<ThemeMode, string> = { auto: 'Auto', light: 'Light', dark: 'Dark' };
  const cycleTheme = () => {
    const order: ThemeMode[] = ['auto', 'light', 'dark'];
    setMode(order[(order.indexOf(mode) + 1) % order.length]);
  };

  const handleLogout = useCallback(async () => {
    if (!window.confirm('Are you sure you want to sign out?')) return;
    await logout();
    router.push('/');
  }, [logout, router]);

  const fullName = user
    ? `${user.first_name ?? ''} ${(user as any).last_name ?? ''}`.trim() || user.email?.split('@')[0] || 'User'
    : '';

  const ThemeIcon = mode === 'dark' ? Moon : mode === 'light' ? Sun : isDark ? Moon : Sun;

  return (
    <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>
      {/* Header */}
      <div
        className="sticky top-0 md:top-16 z-40 px-5 py-4"
        style={{ backgroundColor: colors.background, borderBottom: `1px solid ${colors.dividerColor}` }}
      >
        <h1 className="text-2xl font-black" style={{ color: colors.text }}>Profile</h1>
      </div>

      <div className="pt-2 pb-10">
        {/* ── Logged-out ────────────────────────────────────────────────── */}
        {!user && (
          <>
            <Section>
              <Row
                icon={<User size={16} color={colors.iconColor} />}
                label="Sign up or log in"
                value="Access your tickets"
                onClick={() => router.push('/auth/login')}
              />
            </Section>

            <Section title="APPEARANCE">
              <Row
                icon={<ThemeIcon size={16} color={colors.iconColor} />}
                label="Theme"
                value={themeModeLabel[mode]}
                onClick={cycleTheme}
                hideChevron
              />
            </Section>
          </>
        )}

        {/* ── Logged-in ─────────────────────────────────────────────────── */}
        {user && (
          <>
            <Avatar name={fullName} email={user.email} />

            <Section title="ACCOUNT">
              <Row
                icon={<User size={16} color={colors.iconColor} />}
                label="Edit profile"
                onClick={() => router.push('/profile/edit')}
              />
              <Divider />
              <Row
                icon={<Lock size={16} color={colors.iconColor} />}
                label="Change password"
                onClick={() => router.push(`/reset-password?email=${encodeURIComponent(user.email)}`)}
              />
              <Divider />
              <Row
                icon={<Heart size={16} color={colors.iconColor} />}
                label="Following Events"
                onClick={() => router.push('/profile/following')}
              />
            </Section>

            <Section title="APPEARANCE">
              <Row
                icon={<ThemeIcon size={16} color={colors.iconColor} />}
                label="Theme"
                value={themeModeLabel[mode]}
                onClick={cycleTheme}
                hideChevron
              />
            </Section>
          </>
        )}

        {user && (
          <Section>
            <Row
              icon={<LogOut size={16} color="#ff4d4d" />}
              label="Log out"
              onClick={handleLogout}
              destructive
              hideChevron
            />
            {/* Deletion must be reachable from the WEB, not only from inside
                the app: Google Play requires that someone who has already
                uninstalled can still delete their account without
                reinstalling. Same endpoint and rules as the in-app screen. */}
            <Row
              icon={<Trash2 size={16} color="#ff4d4d" />}
              label="Delete account"
              onClick={() => router.push('/account/delete')}
              destructive
            />
          </Section>
        )}

        <p className="text-center text-[11px] mt-4" style={{ color: colors.subText }}>
          v{APP_VERSION}
        </p>
      </div>
    </div>
  );
}