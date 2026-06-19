'use client';

import Link from 'next/link';
import { useRouter, usePathname } from 'next/navigation';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';
import { ShoppingCart } from 'lucide-react';
import { useState, useEffect, useCallback } from 'react';
import { getCart, getCachedSessionId } from '@/lib/api';
import { Home, ShoppingBag, Search, Ticket, User } from 'lucide-react';

function CartButton() {
  const router = useRouter();
  const { colors } = useAppTheme();
  const [count, setCount] = useState(0);

  const refresh = useCallback(async () => {
    try {
      const sid = await getCachedSessionId();
      const data = await getCart(sid);
      setCount(data?.total_items ?? 0);
    } catch {}
  }, []);

  useEffect(() => {
    refresh();
    const onVisible = () => { if (document.visibilityState === 'visible') refresh(); };
    document.addEventListener('visibilitychange', onVisible);
    return () => document.removeEventListener('visibilitychange', onVisible);
  }, [refresh]);

  return (
    <button onClick={() => router.push('/cart')} className="relative p-1">
      <ShoppingCart size={22} color={colors.text} />
      {count > 0 && (
        <span className="absolute -top-0.5 -right-0.5 min-w-[16px] h-[16px] rounded-full bg-[#ff2d55] text-white text-[9px] font-black flex items-center justify-center px-0.5">
          {count > 9 ? '9+' : count}
        </span>
      )}
    </button>
  );
}

const NAV_LINKS = [
  { href: '/',        label: 'Home',    Icon: Home        },
  { href: '/merch',   label: 'Drops',   Icon: ShoppingBag },
  { href: '/search',  label: 'Search',  Icon: Search      },
  { href: '/tickets', label: 'Tickets', Icon: Ticket      },
  { href: '/profile', label: 'Profile', Icon: User        },
];

export default function NavBar() {
  const router   = useRouter();
  const pathname = usePathname();
  const { user, logout, loadUser } = useAuthStore();
  const { colors }          = useAppTheme();

  useEffect(() => {
    // Validate the stored access token on every page load.
    // Without this, the web app never restores the session after a refresh —
    // `user` resets to null in the Zustand store even though a valid token
    // sits in localStorage, so the UI shows "signed out" until the person
    // logs in again manually.
    loadUser();
  }, [loadUser]);

  const handleLogout = async () => {
    await logout();
    router.push('/auth/login');
  };

  return (
    <header
      style={{ backgroundColor: colors.headerBg, borderBottom: `1px solid ${colors.dividerColor}` }}
      className="sticky top-0 z-50 flex items-center justify-between px-8 h-16"
    >
      {/* Left — logo + nav */}
      <nav className="flex items-center gap-8">
        <Link href="/" className="text-lg font-black tracking-wide" style={{ color: colors.text }}>
          EVENTS<span style={{ color: '#FF2D55' }}>APP</span>
        </Link>
        {NAV_LINKS.map(({ href, label, Icon }) => {
        const active = pathname === href || (href !== '/' && pathname.startsWith(href));
        return (
          <Link key={href} href={href} className="flex flex-col items-center gap-0.5 px-3">
            <Icon size={24} color={active ? colors.text : colors.iconColor} />
            <span className="text-[10px] font-semibold tracking-wide"
                  style={{ color: active ? colors.text : colors.iconColor }}>
              {label.toUpperCase()}
            </span>
            {active && <div className="w-1.5 h-1.5 rounded-full bg-[#ff3c6e]" />}
          </Link>
        );
      })}
      </nav>

      {/* Right — cart + auth */}
      <div className="flex items-center gap-4">
        <CartButton />
        {user ? (
          <>
            <span className="text-sm" style={{ color: colors.subText }}>
              {user.first_name ?? user.name}
            </span>
            <button
              onClick={handleLogout}
              className="px-4 py-2 text-sm font-bold rounded-sm"
              style={{ backgroundColor: colors.sectionCardBg, color: colors.text }}
            >
              Sign out
            </button>
          </>
        ) : (
          <Link
            href="/auth/login"
            className="px-4 py-2 text-sm font-bold rounded-sm"
            style={{ backgroundColor: colors.text, color: colors.background }}
          >
            Sign in
          </Link>
        )}
      </div>
    </header>
  );
}