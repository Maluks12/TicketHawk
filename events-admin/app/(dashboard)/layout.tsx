'use client';
import { useEffect, useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Link from 'next/link';
import { useAdminAuth } from '../../store/authStore';
import { ToastProvider } from '../../lib/toast';
import { ConfirmProvider } from '../../lib/confirm';

const NAV: Array<
  | { type: 'item'; icon: React.ReactNode; label: string; href: string; badge?: boolean | string; exact?: boolean }
  | { type: 'divider' }
> = [
  {
    type: 'item', href: '/dashboard', exact: true, label: 'Dashboard',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M2 11l8-8 8 8v7a1 1 0 01-1 1H3a1 1 0 01-1-1v-7z"/></svg>,
  },
  { type: 'divider' },
  {
    type: 'item', href: '/events', label: 'Events',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M6 2a1 1 0 00-1 1v1H4a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V6a2 2 0 00-2-2h-1V3a1 1 0 10-2 0v1H7V3a1 1 0 00-1-1zm0 5a1 1 0 000 2h8a1 1 0 100-2H6z" clipRule="evenodd"/></svg>,
  },
  {
    type: 'item', href: '/approvals', label: 'Event Approvals', badge: 'approval',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd"/></svg>,
  },
  {
    type: 'item', href: '/organizers', label: 'Organizers & Payouts',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M8.433 7.418c.155-.103.346-.196.567-.267v1.698a2.305 2.305 0 01-.567-.267C8.07 8.34 8 8.114 8 8c0-.114.07-.34.433-.582zM11 12.849v-1.698c.22.071.412.164.567.267.364.243.433.468.433.582 0 .114-.07.34-.433.582a2.305 2.305 0 01-.567.267z"/><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-14a1 1 0 10-2 0v.092a4.535 4.535 0 00-1.676.662C6.602 5.234 6 6.09 6 7c0 .91.602 1.766 1.324 2.246.48.32 1.054.545 1.676.662v1.941c-.391-.127-.68-.317-.843-.504a1 1 0 10-1.51 1.31c.562.649 1.413 1.076 2.353 1.253V14a1 1 0 102 0v-.092a4.535 4.535 0 001.676-.662C13.398 12.766 14 11.91 14 11c0-.91-.602-1.766-1.324-2.246A4.535 4.535 0 0011 8.092V6.151c.391.127.68.317.843.504a1 1 0 101.511-1.31c-.563-.649-1.413-1.076-2.354-1.253V4z" clipRule="evenodd"/></svg>,
  },
  {
    type: 'item', href: '/merch', label: 'Merch & Collections',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M10 2a4 4 0 00-4 4v1H5a1 1 0 00-.994.89l-1 9A1 1 0 004 18h12a1 1 0 00.994-1.11l-1-9A1 1 0 0015 7h-1V6a4 4 0 00-4-4zm2 5V6a2 2 0 10-4 0v1h4zm-6 3a1 1 0 112 0 1 1 0 01-2 0zm7-1a1 1 0 100 2 1 1 0 000-2z" clipRule="evenodd"/></svg>,
  },
  { type: 'divider' },
  {
    type: 'item', href: '/users', label: 'All Users',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M9 6a3 3 0 11-6 0 3 3 0 016 0zM17 6a3 3 0 11-6 0 3 3 0 016 0zM12.93 17c.046-.327.07-.66.07-1a6.97 6.97 0 00-1.5-4.33A5 5 0 0119 16v1h-6.07zM6 11a5 5 0 015 5v1H1v-1a5 5 0 015-5z"/></svg>,
  },
  { type: 'divider' },
  {
    type: 'item', href: '/oversold', label: 'Oversold Orders', badge: true,
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd"/></svg>,
  },
];

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, hydrate, isLoading, logout } = useAdminAuth();
  const [oversoldCount, setOversoldCount] = useState(0);
  const [approvalCount, setApprovalCount] = useState(0);

  useEffect(() => { hydrate(); }, [hydrate]);
  useEffect(() => { if (!isLoading && !user) router.replace('/login'); }, [user, isLoading, router]);

  useEffect(() => {
    if (!user) return;
    const token = localStorage.getItem('evt_token');
    const base = localStorage.getItem('evt_base') || process.env.NEXT_PUBLIC_API_URL?.replace('/v1', '') || 'http://localhost:8080';
    const h = { Authorization: `Bearer ${token}` };
    fetch(`${base}/v1/admin/merch/oversold`, { headers: h })
      .then(r => r.json()).then(d => setOversoldCount((d.data || []).length)).catch(() => {});
    fetch(`${base}/v1/admin/events/pending`, { headers: h })
      .then(r => r.json()).then(d => setApprovalCount((d.events || []).length)).catch(() => {});
  }, [user]);

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

  async function handleLogout() { await logout(); router.replace('/login'); }
  const initials = (user.email ?? '??').slice(0, 2).toUpperCase();

  return (
    <ToastProvider>
      <ConfirmProvider>
        <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: '#f0f2f5' }}>

          {/* Sidebar */}
          <aside style={{
            width: 244, flexShrink: 0,
            background: '#ffffff',
            borderRight: '1px solid #dddfe2',
            display: 'flex', flexDirection: 'column', overflow: 'hidden',
          }}>
            {/* Logo */}
            <div style={{ padding: '16px 16px 12px', borderBottom: '1px solid #f0f2f5', flexShrink: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <div style={{
                  width: 32, height: 32, borderRadius: 8,
                  background: '#1877f2',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                }}>
                  <svg viewBox="0 0 20 20" fill="white" width="16" height="16">
                    <path d="M2 11l8-8 8 8v7a1 1 0 01-1 1H3a1 1 0 01-1-1v-7z"/>
                  </svg>
                </div>
                <div>
                  <div style={{ fontSize: 15, fontWeight: 800, letterSpacing: 0.4, color: '#1c1e21', lineHeight: 1.2 }}>TICKET<span style={{ color: '#FF2D55' }}>HAWK</span></div>
                  <div style={{ fontSize: 11, color: '#606770', lineHeight: 1 }}>Admin Panel</div>
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
                    transition: 'background .12s, color .12s',
                  }}>
                    <span style={{ color: active ? '#1877f2' : '#606770', flexShrink: 0 }}>{item.icon}</span>
                    <span style={{ flex: 1 }}>{item.label}</span>
                    {item.badge === true && oversoldCount > 0 && (
                      <span style={{
                        background: '#fa3e3e', color: '#fff', fontSize: 11, fontWeight: 700,
                        borderRadius: '99px', padding: '1px 7px', minWidth: 20, textAlign: 'center',
                      }}>
                        {oversoldCount}
                      </span>
                    )}
                    {item.badge === 'approval' && approvalCount > 0 && (
                      <span style={{
                        background: '#f59e0b', color: '#fff', fontSize: 11, fontWeight: 700,
                        borderRadius: '99px', padding: '1px 7px', minWidth: 20, textAlign: 'center',
                      }}>
                        {approvalCount}
                      </span>
                    )}
                  </Link>
                );
              })}
            </nav>

            {/* User strip */}
            <div style={{ padding: '12px 16px', borderTop: '1px solid #f0f2f5', flexShrink: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <div style={{
                  width: 32, height: 32, borderRadius: '50%',
                  background: '#1877f2', color: '#fff',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  fontSize: 12, fontWeight: 700, flexShrink: 0,
                }}>
                  {initials}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 13, fontWeight: 600, color: '#1c1e21', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{user.email}</div>
                  <div style={{ fontSize: 11, color: '#606770' }}>Administrator</div>
                </div>
                <button onClick={handleLogout} title="Sign out"
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#8a8d91', padding: 4, borderRadius: 4, display: 'flex', alignItems: 'center' }}>
                  <svg viewBox="0 0 20 20" fill="currentColor" width="16" height="16">
                    <path fillRule="evenodd" d="M3 3a1 1 0 00-1 1v12a1 1 0 001 1h4a1 1 0 100-2H4V5h3a1 1 0 100-2H3zm10.293 9.293a1 1 0 001.414 1.414l3-3a1 1 0 000-1.414l-3-3a1 1 0 10-1.414 1.414L14.586 9H7a1 1 0 100 2h7.586l-1.293 1.293z" clipRule="evenodd"/>
                  </svg>
                </button>
              </div>
            </div>
          </aside>

          {/* Main */}
          <main style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
            {children}
          </main>
        </div>
      </ConfirmProvider>
    </ToastProvider>
  );
}