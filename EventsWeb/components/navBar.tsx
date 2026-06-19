'use client';

import Link from 'next/link';
import { useRouter, usePathname } from 'next/navigation';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';
import { ShoppingCart, Menu, X } from 'lucide-react';
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
  // ── CHANGE 1: hamburger menu toggle state ─────────────────────────────────
  const [menuOpen, setMenuOpen] = useState(false);

  useEffect(() => {
    loadUser();
  }, [loadUser]);

  // Close mobile menu on route change
  useEffect(() => { setMenuOpen(false); }, [pathname]);

  // Prevent body scroll when menu is open
  useEffect(() => {
    document.body.style.overflow = menuOpen ? 'hidden' : '';
    return () => { document.body.style.overflow = ''; };
  }, [menuOpen]);

  const handleLogout = async () => {
    await logout();
    router.push('/auth/login');
  };

  return (
    <>
      <header
        style={{ backgroundColor: colors.headerBg, borderBottom: `1px solid ${colors.dividerColor}` }}
        className="sticky top-0 z-50 flex items-center justify-between px-8 h-16"
      >
        {/* Left — logo + desktop nav */}
        <nav className="flex items-center gap-8">
          <Link href="/" className="text-lg font-black tracking-wide" style={{ color: colors.text }}>
            EVENTS<span style={{ color: '#FF2D55' }}>APP</span>
          </Link>
          {/* ── CHANGE 2: Desktop nav — hidden on mobile, visible md+ ────── */}
          <div className="hidden md:flex items-center gap-8">
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
          </div>
        </nav>

        {/* Right — cart + auth + hamburger */}
        <div className="flex items-center gap-4">
          <CartButton />
          {/* ── CHANGE 3: Auth buttons — hidden on mobile ─────────────────── */}
          <div className="hidden md:flex items-center gap-4">
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
          {/* ── CHANGE 4: Hamburger button — visible only on mobile ──────── */}
          <button
            onClick={() => setMenuOpen((v) => !v)}
            className="md:hidden p-2 rounded-sm flex items-center justify-center"
            aria-label="Toggle menu"
          >
            {menuOpen
              ? <X size={24} color={colors.text} />
              : <Menu size={24} color={colors.text} />
            }
          </button>
        </div>
      </header>

      {/* ── CHANGE 5: Mobile full-screen menu overlay ────────────────────────── */}
      {menuOpen && (
        <div
          className="fixed inset-0 top-16 z-40 flex flex-col md:hidden"
          style={{ backgroundColor: colors.headerBg }}
        >
          {/* Nav links */}
          <nav className="flex-1 flex flex-col pt-6 px-8 overflow-y-auto">
            {NAV_LINKS.map(({ href, label, Icon }) => {
              const active = pathname === href || (href !== '/' && pathname.startsWith(href));
              return (
                <Link
                  key={href}
                  href={href}
                  className="flex items-center gap-5 py-5 border-b"
                  style={{ borderColor: colors.dividerColor }}
                >
                  <Icon size={26} color={active ? '#FF2D55' : colors.iconColor} />
                  <span
                    className="text-[22px] font-bold"
                    style={{ color: active ? '#FF2D55' : colors.text }}
                  >
                    {label}
                  </span>
                  {active && <div className="ml-auto w-2 h-2 rounded-full bg-[#FF2D55]" />}
                </Link>
              );
            })}
          </nav>
          {/* Auth section at bottom */}
          <div className="px-8 py-6 border-t" style={{ borderColor: colors.dividerColor }}>
            {user ? (
              <div className="flex items-center justify-between">
                <span className="text-base font-semibold" style={{ color: colors.text }}>
                  {user.first_name ?? user.name}
                </span>
                <button
                  onClick={handleLogout}
                  className="px-5 py-2.5 text-sm font-bold rounded-sm"
                  style={{ backgroundColor: colors.sectionCardBg, color: colors.text }}
                >
                  Sign out
                </button>
              </div>
            ) : (
              <Link
                href="/auth/login"
                className="block w-full text-center py-3.5 text-base font-bold rounded-sm"
                style={{ backgroundColor: '#FF2D55', color: '#ffffff' }}
              >
                Sign in
              </Link>
            )}
          </div>
        </div>
      )}
    </>
  );
}