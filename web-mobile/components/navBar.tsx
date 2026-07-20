'use client';

import Link from 'next/link';
import { useRouter, usePathname } from 'next/navigation';
import { useAuthStore } from '@/store/authStore';
import { useFilterStore } from '@/store/filterStore';
import { useAppTheme } from '@/themes/colors';
import {
  ShoppingCart, Menu, X, MapPin, ChevronDown,
  Home, Search, Ticket, User,
} from 'lucide-react';
import { useState, useEffect, useCallback } from 'react';
import { getCart, getCachedSessionId } from '@/lib/api';

// ── Cart badge button ──────────────────────────────────────────────────────────
function CartButton() {
  // Cart is hidden: TicketHawk buys tickets directly (POST /v1/bookings →
  // payment), with no cart step, matching the app. Merch/Drops is removed, so
  // nothing reaches the cart. Flip SHOW_CART to true to re-enable everything
  // below (component, badge polling, and imports all stay wired).
  const SHOW_CART = false;

  const router     = useRouter();
  const { colors } = useAppTheme();
  const [count, setCount] = useState(0);

  const refresh = useCallback(async () => {
    if (!SHOW_CART) return;
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

  if (!SHOW_CART) return null;

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
  { href: '/search',  label: 'Search',  Icon: Search      },
  { href: '/tickets', label: 'Tickets', Icon: Ticket      },
  { href: '/profile', label: 'Profile', Icon: User        },
];

// ── NavBar ─────────────────────────────────────────────────────────────────────
export default function NavBar() {
  const router   = useRouter();
  const pathname = usePathname();
  const { user, logout, loadUser } = useAuthStore();
  const { colors }                 = useAppTheme();

  // shared filter state — written by page.tsx, read here for display
  const { locationLabel, dateLabelText, setFilterOpen } = useFilterStore();

  const [menuOpen, setMenuOpen] = useState(false);

  useEffect(() => { loadUser(); }, [loadUser]);

  // Close mobile overlay on route change
  useEffect(() => { setMenuOpen(false); }, [pathname]);

  // Lock body scroll while mobile menu is open
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
            <img src="/brand-logo_v2.png" alt="" style={{ width: 58, height: 58, verticalAlign: 'middle', marginRight: 8, display: 'inline-block' }} />TICKET<span style={{ color: '#FF2D55' }}>HAWK</span>
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
                {/* active indicator dot */}
                <div
                  className="h-[3px] w-[3px] rounded-full transition-opacity duration-200"
                  style={{
                    backgroundColor: '#FF2D55',
                    opacity: active ? 1 : 0,
                  }}
                />
              </Link>
            );
          })}
        </nav>

        {/* RIGHT: filter pill + cart + mobile hamburger ────────────────── */}
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

          <CartButton />

          {/* Hamburger — mobile only */}
          <button
            onClick={() => setMenuOpen((v) => !v)}
            className="md:hidden p-1.5 rounded-sm"
            aria-label="Toggle menu"
          >
            {menuOpen
              ? <X    size={23} color={colors.text} />
              : <Menu size={23} color={colors.text} />
            }
          </button>
        </div>
      </header>

      {/* ── Mobile full-screen overlay ─────────────────────────────────────── */}
      {menuOpen && (
        <div
          className="fixed inset-0 z-[45] flex flex-col md:hidden pt-16"
          style={{ backgroundColor: colors.background }}
        >
          <nav className="flex-1 flex flex-col pt-4 px-8 overflow-y-auto">

            {/* Filter row at top of mobile menu */}
            <button
              onClick={() => { setMenuOpen(false); setFilterOpen(true); }}
              className="flex items-center gap-4 py-5 border-b"
              style={{ borderColor: colors.dividerColor }}
            >
              <MapPin size={24} color={colors.iconColor} />
              <div className="text-left">
                <p className="text-[17px] font-bold leading-tight" style={{ color: colors.text }}>
                  {locationLabel}
                </p>
                <p className="text-[12px] mt-0.5" style={{ color: colors.subText }}>
                  {dateLabelText}
                </p>
              </div>
              <ChevronDown size={16} color={colors.subText} className="ml-auto" />
            </button>

            {/* Nav links */}
            {NAV_LINKS.map(({ href, label }) => {
              const active = pathname === href || (href !== '/' && pathname.startsWith(href));
              return (
                <Link
                  key={href}
                  href={href}
                  className="flex items-center py-5 border-b"
                  style={{ borderColor: colors.dividerColor }}
                >
                  <span
                    className="text-[21px] font-bold"
                    style={{ color: active ? '#FF2D55' : colors.text }}
                  >
                    {label}
                  </span>
                  {active && (
                    <div className="ml-auto w-[7px] h-[7px] rounded-full bg-[#FF2D55]" />
                  )}
                </Link>
              );
            })}
          </nav>

          {/* Auth row at bottom */}
          <div className="px-8 py-6 border-t" style={{ borderColor: colors.dividerColor }}>
            {user ? (
              <div className="flex items-center justify-between">
                <span className="text-[15px] font-semibold" style={{ color: colors.text }}>
                  {user.first_name ?? user.name}
                </span>
                <button
                  onClick={handleLogout}
                  className="px-5 py-2.5 text-[13px] font-bold rounded-[6px]"
                  style={{ backgroundColor: colors.sectionCardBg, color: colors.text }}
                >
                  Sign out
                </button>
              </div>
            ) : (
              <Link
                href="/auth/login"
                className="block w-full text-center py-3.5 text-[15px] font-bold rounded-[6px]"
                style={{ backgroundColor: '#FF2D55', color: '#fff' }}
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