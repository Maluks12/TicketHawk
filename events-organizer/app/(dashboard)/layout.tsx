'use client';
import { useEffect } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Link from 'next/link';
import { useOrgAuth } from '../../store/authStore';
import { ToastProvider } from '../../lib/toast';
import { ConfirmProvider } from '../../lib/confirm';

// Launch gate: merch is built but NOT launching — the tab and the event-page
// merch sections are hidden, not deleted. Flip to true to bring merch back.
const MERCH_ENABLED = false;
// Ads/campaigns are disabled for launch — feed placement is organic. Flip to
// re-surface the Campaigns section when paid promotion returns.
const ADS_ENABLED = false;

const NAV: Array<
  | { type: 'item'; icon: React.ReactNode; label: string; href: string; exact?: boolean }
  | { type: 'divider' }
> = [
  {
    type: 'item', href: '/dashboard', exact: true, label: 'Overview',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M2 11l8-8 8 8v7a1 1 0 01-1 1H3a1 1 0 01-1-1v-7z"/></svg>,
  },
  {
    type: 'item', href: '/events', label: 'My Events',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M6 2a1 1 0 00-1 1v1H4a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V6a2 2 0 00-2-2h-1V3a1 1 0 10-2 0v1H7V3a1 1 0 00-1-1zm0 5a1 1 0 000 2h8a1 1 0 100-2H6z" clipRule="evenodd"/></svg>,
  },
  { type: 'divider' },
  ...(ADS_ENABLED ? [{
    type: 'item' as const, href: '/campaigns', label: 'Campaigns',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M2 11a1 1 0 011-1h2a1 1 0 011 1v5a1 1 0 01-1 1H3a1 1 0 01-1-1v-5zM8 7a1 1 0 011-1h2a1 1 0 011 1v9a1 1 0 01-1 1H9a1 1 0 01-1-1V7zM14 4a1 1 0 011-1h2a1 1 0 011 1v12a1 1 0 01-1 1h-2a1 1 0 01-1-1V4z"/></svg>,
  }, { type: 'divider' as const }] : []),
  ...(MERCH_ENABLED ? [{
    // Collections removed from sidebar — accessible as a tab inside Merch
    type: 'item' as const, href: '/merch', label: 'Merch',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M10 2a4 4 0 00-4 4v1H5a1 1 0 00-.994.89l-1 9A1 1 0 004 18h12a1 1 0 00.994-1.11l-1-9A1 1 0 0015 7h-1V6a4 4 0 00-4-4zm2 5V6a2 2 0 10-4 0v1h4zm-6 3a1 1 0 112 0 1 1 0 01-2 0zm7-1a1 1 0 100 2 1 1 0 000-2z" clipRule="evenodd"/></svg>,
  }, { type: 'divider' as const }] : []),
  ...(ADS_ENABLED ? [{
    type: 'item' as const, href: '/payouts', label: 'Billing & Payments',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M4 4a2 2 0 00-2 2v1h16V6a2 2 0 00-2-2H4z"/><path fillRule="evenodd" d="M18 9H2v5a2 2 0 002 2h12a2 2 0 002-2V9zM4 13a1 1 0 011-1h1a1 1 0 110 2H5a1 1 0 01-1-1zm5-1a1 1 0 100 2h1a1 1 0 100-2H9z" clipRule="evenodd"/></svg>,
  }] : []),
];

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, hydrate, isLoading, logout } = useOrgAuth();

  useEffect(() => { hydrate(); }, [hydrate]);
  useEffect(() => {
    if (!isLoading && !user) router.replace('/login');
  }, [user, isLoading, router]);

  if (isLoading || !user) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', background: '#f0f2f5' }}>
        <div className="spinner spinner-lg" />
      </div>
    );
  }

  function isActive(href: string, exact?: boolean) {
    if (exact) return pathname === href;
    return pathname === href || pathname.startsWith(href + '/');
  }

  const initials = (user.email ?? '??').slice(0, 2).toUpperCase();

  return (
    <ToastProvider>
      <ConfirmProvider>
        <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: '#f0f2f5' }}>
          <aside style={{
            width: 244, flexShrink: 0, background: '#fff',
            borderRight: '1px solid #dddfe2',
            display: 'flex', flexDirection: 'column', overflow: 'hidden',
          }}>
            {/* Logo */}
            <div style={{ padding: '16px 16px 12px', borderBottom: '1px solid #f0f2f5', flexShrink: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <div style={{ width: 32, height: 32, borderRadius: 8, background: '#1877f2', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                  <svg viewBox="0 0 20 20" fill="white" width="16" height="16">
                    <path fillRule="evenodd" d="M6 2a1 1 0 00-1 1v1H4a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V6a2 2 0 00-2-2h-1V3a1 1 0 10-2 0v1H7V3a1 1 0 00-1-1zm0 5a1 1 0 000 2h8a1 1 0 100-2H6z" clipRule="evenodd"/>
                  </svg>
                </div>
                <div>
                  <div style={{ fontSize: 15, fontWeight: 800, letterSpacing: 0.4, color: '#1c1e21', lineHeight: 1.2 }}>TICKET<span style={{ color: '#FF2D55' }}>HAWK</span></div>
                  <div style={{ fontSize: 11, color: '#606770', lineHeight: 1 }}>Organiser</div>
                </div>
              </div>
            </div>

            {/* Nav */}
            <nav style={{ flex: 1, padding: '8px 0', overflowY: 'auto' }}>
              {NAV.map((item, i) => {
                if (item.type === 'divider') {
                  return <div key={i} style={{ height: 1, background: '#f0f2f5', margin: '4px 0' }} />;
                }
                const active = isActive(item.href, item.exact);
                return (
                  <Link key={item.href} href={item.href} style={{
                    display: 'flex', alignItems: 'center', gap: 10,
                    padding: '8px 12px', margin: '1px 8px', borderRadius: 6,
                    textDecoration: 'none',
                    color: active ? '#1877f2' : '#1c1e21',
                    background: active ? '#e7f3ff' : 'transparent',
                    fontWeight: active ? 600 : 500, fontSize: 14,
                  }}>
                    <span style={{ color: active ? '#1877f2' : '#606770', flexShrink: 0 }}>{item.icon}</span>
                    {item.label}
                  </Link>
                );
              })}
            </nav>

            {/* User strip */}
            <div style={{ padding: '12px 16px', borderTop: '1px solid #f0f2f5', flexShrink: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <div style={{ width: 32, height: 32, borderRadius: '50%', background: '#1877f2', color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 12, fontWeight: 700, flexShrink: 0 }}>
                  {initials}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 13, fontWeight: 600, color: '#1c1e21', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{user.email}</div>
                  <div style={{ fontSize: 11, color: '#606770' }}>Organiser</div>
                </div>
                <button onClick={async () => { await logout(); router.replace('/login'); }} title="Sign out"
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#606770', padding: 4, borderRadius: 4, display: 'flex', alignItems: 'center' }}>
                  <svg viewBox="0 0 20 20" fill="currentColor" width="16" height="16">
                    <path fillRule="evenodd" d="M3 3a1 1 0 00-1 1v12a1 1 0 001 1h12a1 1 0 001-1V4a1 1 0 00-1-1H3zm10.293 9.293a1 1 0 001.414 1.414l3-3a1 1 0 000-1.414l-3-3a1 1 0 10-1.414 1.414L14.586 9H7a1 1 0 100 2h7.586l-1.293 1.293z" clipRule="evenodd"/>
                  </svg>
                </button>
              </div>
            </div>
          </aside>

          <main style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
            {children}
          </main>
        </div>
      </ConfirmProvider>
    </ToastProvider>
  );
}