'use client';

import Link from 'next/link';
import { useRouter, usePathname } from 'next/navigation';
import { useAuthStore } from '@/store/authStore';
import { useFilterStore } from '@/store/filterStore';
import { useAppTheme } from '@/themes/colors';
import {
  ShoppingCart, MapPin, ChevronDown,
  Home, ShoppingBag, Search, Ticket, User,
} from 'lucide-react';
import { useState, useEffect, useCallback } from 'react';
import { getCart, getCachedSessionId } from '@/lib/api';

// ── Cart badge button ──────────────────────────────────────────────────────────
function CartButton() {
  const router     = useRouter();
  const { colors } = useAppTheme();
  const [count, setCount] = useState(0);

  const refresh = useCallback(async () => {
    try {
      const sid  = await getCachedSessionId();
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
        <span className="absolute -top-0.5 -right-0.5 min-w-[16px] h-[16px] rounded-full bg-[#FF2D55] text-white text-[9px] font-black flex items-center justify-center border border-black/30 px-0.5">
          {count > 9 ? '9+' : count}
        </span>
      )}
    </button>
  );
}

// ── Nav links config ───────────────────────────────────────────────────────────
const NAV_LINKS = [
  { href: '/',        label: 'Home',    Icon: Home        },
  { href: '/merch',   label: 'Drops',   Icon: ShoppingBag },
  { href: '/search',  label: 'Search',  Icon: Search      },
  { href: '/tickets', label: 'Tickets', Icon: Ticket      },
  { href: '/profile', label: 'Profile', Icon: User        },
];

// ── Mobile bottom tab bar ──────────────────────────────────────────────────────
// FIX: This was the missing piece. layout.tsx always reserved pb-[60px] for a
// bottom tab bar, but the bar itself was never rendered — only a hamburger
// overlay existed. On mobile there was no way to navigate between pages.
function BottomTabBar() {
  const pathname   = usePathname();
  const { colors } = useAppTheme();

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-50 flex md:hidden h-[60px] items-center"
      style={{
        backgroundColor: colors.headerBg,
        borderTop: `1px solid ${colors.dividerColor}`,
        // Pad below the tabs on notched/home-indicator devices.
        // Requires viewport-fit=cover (set in layout.tsx).
        paddingBottom: 'env(safe-area-inset-bottom)',
      }}
    >
      {NAV_LINKS.map(({ href, label, Icon }) => {
        const active = pathname === href || (href !== '/' && pathname.startsWith(href));
        return (
          <Link
            key={href}
            href={href}
            className="relative flex-1 flex flex-col items-center justify-center gap-[3px] h-full"
          >
            {/* Active top-edge indicator */}
            <div
              className="absolute top-0 h-[2px] w-[28px] rounded-full transition-opacity duration-200"
              style={{ backgroundColor: '#FF2D55', opacity: active ? 1 : 0 }}
            />
            <Icon size={22} color={active ? '#FF2D55' : colors.iconColor} />
            <span
              className="text-[9px] font-bold tracking-[0.5px]"
              style={{ color: active ? '#FF2D55' : colors.iconColor }}
            >
              {label.toUpperCase()}
            </span>
          </Link>
        );
      })}
    </nav>
  );
}

// ── NavBar ─────────────────────────────────────────────────────────────────────
export default function NavBar() {
  const pathname = usePathname();
  const { colors } = useAppTheme();

  const { user, logout, loadUser } = useAuthStore();
  const { locationLabel, dateLabelText, setFilterOpen } = useFilterStore();

  const router = useRouter();

  useEffect(() => { loadUser(); }, [loadUser]);

  const handleLogout = async () => {
    await logout();
    router.push('/auth/login');
  };

  return (
    <>
      {/* ── Header ─────────────────────────────────────────────────────────── */}
      <header
        className="sticky top-0 z-50 h-16 flex items-center px-6 gap-4"
        style={{
          backgroundColor: colors.headerBg,
          borderBottom: `1px solid ${colors.dividerColor}`,
        }}
      >
        {/* LEFT: brand logo ─────────────────────────────────────────────── */}
        <div className="flex-none">
          <Link href="/" className="text-[17px] font-black tracking-wide" style={{ color: colors.text }}>
            EVENTS<span style={{ color: '#FF2D55' }}>APP</span>
          </Link>
        </div>

        {/* CENTER: nav icons — desktop only ────────────────────────────── */}
        <nav className="hidden md:flex flex-1 items-center justify-center gap-1">
          {NAV_LINKS.map(({ href, label, Icon }) => {
            const active = pathname === href || (href !== '/' && pathname.startsWith(href));
            return (
              <Link
                key={href}
                href={href}
                className="flex flex-col items-center gap-[3px] px-5 py-[7px] rounded-lg transition-colors duration-150"
                style={{ backgroundColor: active ? 'rgba(255,255,255,0.07)' : 'transparent' }}
              >
                <Icon size={21} color={active ? colors.text : colors.iconColor} />
                <span
                  className="text-[10px] font-bold tracking-[0.7px]"
                  style={{ color: active ? colors.text : colors.iconColor }}
                >
                  {label.toUpperCase()}
                </span>
                <div
                  className="h-[3px] w-[3px] rounded-full transition-opacity duration-200"
                  style={{ backgroundColor: '#FF2D55', opacity: active ? 1 : 0 }}
                />
              </Link>
            );
          })}
        </nav>

        {/* RIGHT: cart + filter pill ───────────────────────────────────── */}
        <div className="flex-none flex items-center gap-3 ml-auto md:ml-0">

          {/* Filter pill — desktop only */}
          <button
            onClick={() => setFilterOpen(true)}
            className="hidden md:flex items-center gap-[7px] h-9 px-[14px] rounded-full text-[12px] font-semibold transition-opacity duration-150 hover:opacity-75"
            style={{
              backgroundColor: colors.sectionCardBg,
              border: `1px solid ${colors.dividerColor}`,
            }}
          >
            <MapPin size={13} color={colors.subText} />
            <span style={{ color: colors.text }}>{locationLabel}</span>
            <span className="opacity-30" style={{ color: colors.text }}>·</span>
            <span style={{ color: colors.subText }}>{dateLabelText}</span>
            <ChevronDown size={12} color={colors.subText} />
          </button>

          {/* Filter pill — mobile only (compact, in top header) */}
          <button
            onClick={() => setFilterOpen(true)}
            className="md:hidden flex items-center gap-[5px] h-8 px-[10px] rounded-full text-[11px] font-semibold"
            style={{
              backgroundColor: colors.sectionCardBg,
              border: `1px solid ${colors.dividerColor}`,
            }}
          >
            <MapPin size={11} color={colors.subText} />
            <span style={{ color: colors.subText }}>{locationLabel}</span>
          </button>

          <CartButton />

          {/* Desktop: auth actions */}
          <div className="hidden md:flex items-center gap-2">
            {user ? (
              <button
                onClick={handleLogout}
                className="px-4 py-2 text-[12px] font-bold rounded-[6px]"
                style={{ backgroundColor: colors.sectionCardBg, color: colors.text }}
              >
                Sign out
              </button>
            ) : (
              <Link
                href="/auth/login"
                className="px-4 py-2 text-[12px] font-bold rounded-[6px]"
                style={{ backgroundColor: '#FF2D55', color: '#fff' }}
              >
                Sign in
              </Link>
            )}
          </div>
        </div>
      </header>

      {/* ── Mobile bottom tab bar ─────────────────────────────────────────────── */}
      <BottomTabBar />
    </>
  );
}