'use client';

import Link from 'next/link';
import { useRouter, usePathname } from 'next/navigation';
import { useAuthStore } from '@/store/authStore';
import { useFilterStore } from '@/store/filterStore';
import { useAppTheme } from '@/themes/colors';
import { Menu, X, MapPin, ChevronDown } from 'lucide-react';
import { useState, useEffect } from 'react';

// ── Nav links config ───────────────────────────────────────────────────────────
const NAV_LINKS = [
  { href: '/',        label: 'Home'    },
  { href: '/merch',   label: 'Drops'   },
  { href: '/search',  label: 'Search'  },
  { href: '/tickets', label: 'Tickets' },
  { href: '/profile', label: 'Profile' },
];

// ── Nav pill button (desktop) ───────────────────────────────────────────────────
function NavPill({ href, label, active }: { href: string; label: string; active: boolean }) {
  const { colors } = useAppTheme();
  const [hovered, setHovered] = useState(false);

  return (
    <Link
      href={href}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="px-4 py-[9px] rounded-full text-[12.5px] font-bold tracking-[0.6px] transition-all duration-200 ease-out"
      style={{
        backgroundColor: active ? '#FF2D55' : hovered ? colors.sectionCardBg : 'transparent',
        color: active ? '#ffffff' : hovered ? colors.text : colors.iconColor,
        boxShadow: active ? '0 4px 14px rgba(255,45,85,0.35)' : 'none',
        transform: hovered && !active ? 'translateY(-1px)' : 'translateY(0)',
      }}
    >
      {label.toUpperCase()}
    </Link>
  );
}

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
        {/* LEFT: brand logo + nav links ───────────────────────────────────── */}
        <div className="flex-none flex items-center gap-8">
          <Link href="/" className="text-[17px] font-black tracking-wide" style={{ color: colors.text }}>
            EVENTS<span style={{ color: '#FF2D55' }}>APP</span>
          </Link>

          {/* nav links — desktop only */}
          <nav className="hidden md:flex items-center gap-1.5">
            {NAV_LINKS.map(({ href, label }) => {
              const active = pathname === href || (href !== '/' && pathname.startsWith(href));
              return <NavPill key={href} href={href} label={label} active={active} />;
            })}
          </nav>
        </div>

        {/* RIGHT: filter pill (homepage only) + mobile hamburger ─────────── */}
        <div className="flex-none flex items-center gap-3 ml-auto">

          {/* Filter pill — desktop only, homepage only */}
          {pathname === '/' && (
            <button
              onClick={() => setFilterOpen(true)}
              className="hidden md:flex items-center gap-2 h-11 px-4 rounded-full text-[13px] font-semibold transition-all duration-150 hover:opacity-80 hover:scale-[1.02]"
              style={{
                backgroundColor: colors.sectionCardBg,
                border: `1px solid ${colors.dividerColor}`,
              }}
            >
              <MapPin size={18} color="#FF2D55" />
              <span style={{ color: colors.text }}>{locationLabel}</span>
              <span className="opacity-30" style={{ color: colors.text }}>·</span>
              <span style={{ color: colors.subText }}>{dateLabelText}</span>
              <ChevronDown size={15} color={colors.subText} />
            </button>
          )}

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
          className="fixed inset-0 top-16 z-40 flex flex-col md:hidden"
          style={{ backgroundColor: colors.headerBg }}
        >
          <nav className="flex-1 flex flex-col pt-4 px-8 overflow-y-auto">

            {/* Filter row at top of mobile menu — homepage only */}
            {pathname === '/' && (
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
            )}

            {/* Nav links */}
            {NAV_LINKS.map(({ href, label }) => {
              const active = pathname === href || (href !== '/' && pathname.startsWith(href));
              return (
                <Link
                  key={href}
                  href={href}
                  className="flex items-center gap-3 py-4 px-4 my-1 rounded-2xl transition-colors duration-150"
                  style={{
                    backgroundColor: active ? '#FF2D55' : 'transparent',
                  }}
                >
                  <span
                    className="text-[19px] font-bold"
                    style={{ color: active ? '#ffffff' : colors.text }}
                  >
                    {label}
                  </span>
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