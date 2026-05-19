'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/theme/colors';

export default function NavBar() {
  const router = useRouter();
  const { user, logout } = useAuthStore();
  const { colors, isDark } = useAppTheme();

  const handleLogout = async () => {
    await logout();
    router.push('/auth/login');
  };

  return (
    <header
      style={{
        backgroundColor: colors.headerBg,
        borderBottom: `1px solid ${colors.dividerColor}`,
      }}
      className="sticky top-0 z-50 flex items-center justify-between px-8 h-16"
    >
      {/* Left — logo + nav links */}
      <nav className="flex items-center gap-8">
        <Link href="/" className="text-lg font-black tracking-wide" style={{ color: colors.text }}>
          EVENTS<span style={{ color: '#E8472A' }}>APP</span>
        </Link>
        <Link href="/"         className="text-sm font-medium" style={{ color: colors.text }}>Home</Link>
        <Link href="/merch"    className="text-sm font-medium" style={{ color: colors.text }}>Merch</Link>
        <Link href="/search"   className="text-sm font-medium" style={{ color: colors.text }}>Search</Link>
        <Link href="/tickets"  className="text-sm font-medium" style={{ color: colors.text }}>Tickets</Link>
        <Link href="/profile"  className="text-sm font-medium" style={{ color: colors.text }}>Profile</Link>
      </nav>

      {/* Right — auth */}
      <div className="flex items-center gap-4">
        {user ? (
          <>
            <span className="text-sm" style={{ color: colors.subText }}>{user.name}</span>
            <button
              onClick={handleLogout}
              className="px-5 py-2 rounded-md text-sm font-bold"
              style={{ backgroundColor: colors.ctaBg, color: colors.ctaText }}
            >
              Sign out
            </button>
          </>
        ) : (
          <Link
            href="/auth/login"
            className="px-5 py-2 rounded-md text-sm font-bold"
            style={{ backgroundColor: colors.text, color: colors.background }}
          >
            Sign in
          </Link>
        )}
      </div>
    </header>
  );
}