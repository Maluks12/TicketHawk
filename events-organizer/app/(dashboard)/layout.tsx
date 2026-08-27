'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Dashboard shell — Studio look, desktop layout
//
// Visual change only. The auth gate, the KYC/onboarding bounce, the launch
// flags and every route in NAV behave exactly as before; what changed is the
// chrome around them:
//
//   • Studio palette via globals.css tokens instead of hardcoded Meta hex, so
//     the shell follows light/dark with everything else.
//   • Feather-style stroke icons (components/icons.tsx) replacing the solid
//     20-unit glyphs, matching the mobile Studio's icon language.
//   • A real desktop frame: fixed full-height sidebar + top bar with the
//     current section, a theme switch and the scan shortcut, and a single
//     scroll region for page content.
// ─────────────────────────────────────────────────────────────────────────────
import { useEffect, useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Link from 'next/link';
import { useOrgAuth } from '../../store/authStore';
import { ToastProvider } from '../../lib/toast';
import { ConfirmProvider } from '../../lib/confirm';
import { ThemeToggle } from '../../lib/theme';
import { getKycStatus } from '../../lib/kyc';
import { Icon, IconName } from '../../components/icons';
import LegalGate from '../../components/legalGate';

// Launch gate: merch is built but NOT launching — the tab and the event-page
// merch sections are hidden, not deleted. Flip to true to bring merch back.
const MERCH_ENABLED = false;
// Ads/campaigns are disabled for launch — feed placement is organic. Flip to
// re-surface the Campaigns section when paid promotion returns.
const ADS_ENABLED = false;

type NavEntry =
  | { type: 'item'; icon: IconName; label: string; href: string; exact?: boolean }
  | { type: 'section'; label: string }
  | { type: 'divider' };

const NAV: NavEntry[] = [
  { type: 'section', label: 'Dashboard' },
  { type: 'item', href: '/dashboard', exact: true, label: 'Overview', icon: 'grid' },
  { type: 'item', href: '/events', label: 'My Events', icon: 'ticket' },
  // Deep analytics — reach, retention, PSR and door numbers at minute/hour/day
  // resolution. Kept OFF the Overview and behind its own route so the
  // dashboard's first screen stays a summary rather than a control panel.
  { type: 'item', href: '/analytics', label: 'Analytics', icon: 'activity' },
  // The organiser's own public page — what attendees reach from the ORGANISED
  // BY row on any of their events, and from search. Sits under Dashboard
  // rather than Account because it is a thing they BUILD, like an event, not a
  // setting they configure once.
  { type: 'item', href: '/organiser-page', label: 'Public Page', icon: 'user' },
  { type: 'divider' },
  { type: 'section', label: 'Account' },
  {
    // Verification + payout bank details. Unconditional: this is the only
    // reachable route to the bank form now that /payouts is retired behind
    // ADS_ENABLED, and PublishEvent hard-fails every paid event without it.
    type: 'item', href: '/kyc', label: 'Verification & Payouts', icon: 'shield-check',
  },
  // Business name, contact email, phone. Read-only until now — the model had
  // an update DTO with no repository function and no route behind it, so a
  // name mistyped at sign-up could not be corrected.
  { type: 'item', href: '/settings', label: 'Account Settings', icon: 'settings' },
  ...(ADS_ENABLED
    ? [{ type: 'item' as const, href: '/campaigns', label: 'Campaigns', icon: 'bar-chart' as IconName }]
    : []),
  ...(MERCH_ENABLED
    // Collections removed from sidebar — accessible as a tab inside Merch
    ? [{ type: 'item' as const, href: '/merch', label: 'Merch', icon: 'shopping-bag' as IconName }]
    : []),
  ...(ADS_ENABLED
    ? [{ type: 'item' as const, href: '/payouts', label: 'Billing & Payments', icon: 'credit-card' as IconName }]
    : []),
];

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, hydrate, isLoading, logout } = useOrgAuth();

  useEffect(() => { hydrate(); }, [hydrate]);
  useEffect(() => {
    if (!isLoading && !user) router.replace('/login');
  }, [user, isLoading, router]);

  // ── Onboarding gate ──────────────────────────────────────────────────────
  // Tier 0 (no organiser profile / no basic identity) cannot use the
  // dashboard: everything here needs the organiser role, which basic KYC is
  // what grants. Bounce them to /onboarding. Admin accounts are exempt — they
  // may legitimately have no organizer profile.
  const [kycChecked, setKycChecked] = useState(false);
  useEffect(() => {
    if (!user) return;
    if ((user as { role?: string }).role === 'admin') { setKycChecked(true); return; }
    if (typeof window !== 'undefined' && !localStorage.getItem('evt_token')) return; // token not ready
    getKycStatus()
      .then((s) => {
        if (s.kyc_level === 0) router.replace('/onboarding');
        else setKycChecked(true);
      })
      .catch(() => setKycChecked(true)); // network hiccup: don't lock them out
  }, [user, router]);

  if (isLoading || !user || !kycChecked) {
    return (
      <div className="page-loader">
        <div className="spinner spinner-lg" />
      </div>
    );
  }

  function isActive(href: string, exact?: boolean) {
    if (exact) return pathname === href;
    return pathname === href || pathname.startsWith(href + '/');
  }

  const initials = (user.email ?? '??').slice(0, 2).toUpperCase();

  // Label for the top bar. Longest match wins so /event/[id] still reads as
  // "My Events" rather than falling through to the default.
  const current = NAV.filter((n): n is Extract<NavEntry, { type: 'item' }> => n.type === 'item')
    .filter((n) => isActive(n.href, n.exact))
    .sort((a, b) => b.href.length - a.href.length)[0];
  const sectionLabel =
    current?.label ?? (pathname.startsWith('/event/') ? 'My Events' : 'Studio');

  return (
    <ToastProvider>
      <ConfirmProvider>
        {/* Existing organisers never saw the sign-up consent checkbox, so this
            is where they're told. Fails open — if the status endpoint is down
            or not yet deployed, nobody is blocked out of their dashboard. */}
        <LegalGate onLogout={async () => { await logout(); router.replace('/login'); }} />

        <div className="app-shell">
          {/* ── Sidebar ──────────────────────────────────────────────────── */}
          <aside className="sidebar">
            <div className="sidebar-brand">
              {/* The real mark, not a calendar glyph standing in for one.
                  eslint-disable below: next/image wants width/height and a
                  loader config for a 36px static asset that's already in
                  /public — a plain <img> is the right tool at this size. */}
              {/* .brand-mark still supplies the 36px box and the flex-shrink,
                  but its indigo plate and accent glow are both switched off —
                  the glow was the halo visible around the circle. Left as an
                  inline override so the class stays reusable for a future
                  square mark. */}
              <div className="brand-mark" style={{
                background: 'transparent', boxShadow: 'none', padding: 0,
              }}>
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src="/brand-logo.png"
                  alt=""
                  width={36}
                  height={36}
                  style={{ display: 'block', width: '100%', height: '100%', objectFit: 'cover' }}
                />
              </div>
              <div className="brand-text" style={{ minWidth: 0 }}>
                <div className="brand-word">
                  TICKET<span style={{ color: 'var(--brand)' }}>HAWK</span>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 3 }}>
                  <span className="brand-tag">STUDIO</span>
                </div>
              </div>
            </div>

            <nav className="nav-scroll">
              {NAV.map((item, i) => {
                if (item.type === 'divider') return <div key={`d${i}`} className="nav-divider" />;
                if (item.type === 'section') {
                  return <div key={`s${i}`} className="nav-section">{item.label}</div>;
                }
                const active = isActive(item.href, item.exact);
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={`nav-item${active ? ' active' : ''}`}
                    aria-current={active ? 'page' : undefined}
                    title={item.label}
                  >
                    <span className="nav-icon"><Icon name={item.icon} size={18} /></span>
                    <span className="nav-label">{item.label}</span>
                  </Link>
                );
              })}
            </nav>

            {/* User strip */}
            <div className="sidebar-foot">
              <div className="user-strip">
                <div className="avatar">{initials}</div>
                <div className="user-text" style={{ flex: 1, minWidth: 0 }}>
                  <div style={{
                    fontSize: 12.5, fontWeight: 700, color: 'var(--text)',
                    overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                  }}>
                    {user.email}
                  </div>
                  <div style={{ fontSize: 11, color: 'var(--text-3)' }}>Organiser</div>
                </div>
                <button
                  className="btn-icon"
                  onClick={async () => { await logout(); router.replace('/login'); }}
                  title="Sign out"
                  aria-label="Sign out"
                >
                  <Icon name="log-out" size={16} />
                </button>
              </div>
            </div>
          </aside>

          {/* ── Main column ──────────────────────────────────────────────── */}
          <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            <header className="topbar">
              <div className="crumb">
                <Icon name="layers" size={15} />
                <span>Studio</span>
                <Icon name="chevron-right" size={13} />
                <strong>{sectionLabel}</strong>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <ThemeToggle />
              </div>
            </header>

            {/* overflow:auto, NOT hidden. The shell is height:100vh, so anything
                taller than the viewport was previously clipped with no scrollbar —
                the KYC/payout form was unreachable below the fold. Pages that run
                their own scroller (e.g. /events uses flex:1 + overflow:auto) are
                unaffected: main is a definite-height flex column, so those children
                still resolve to a bounded height and scroll internally as before;
                this only adds a fallback scrollbar for pages that don't. */}
            <main style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
              {children}
            </main>
          </div>
        </div>
      </ConfirmProvider>
    </ToastProvider>
  );
}