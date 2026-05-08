'use client';
import { useEffect, useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Link from 'next/link';
import { useAdminAuth } from '../../store/authStore';
import { ToastProvider } from '../../lib/toast';
import { ConfirmProvider } from '../../lib/confirm';

const NAV = [
  { section: 'Overview' },
  { href: '/dashboard', label: 'Dashboard', icon: '▦' },
  { section: 'Content' },
  { href: '/events',      label: 'Events',         icon: '◈' },
  { href: '/merch',       label: 'Merchandise',    icon: '◇' },
  { href: '/collections', label: 'Collections',    icon: '⬡' },
  { section: 'Users' },
  { href: '/users',       label: 'All Users',      icon: '◉' },
  { href: '/allmerch',    label: 'All Merchandise', icon: '◈' },
  { section: 'Alerts' },
  { href: '/oversold',    label: 'Oversold Orders', icon: '⚠', badge: true },
];

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, hydrate, isLoading, logout } = useAdminAuth();
  const [oversoldCount, setOversoldCount] = useState(0);

  useEffect(() => { hydrate(); }, [hydrate]);
  useEffect(() => { if (!isLoading && !user) router.replace('/login'); }, [user, isLoading, router]);

  // Poll oversold badge
  useEffect(() => {
    if (!user) return;
    const token = localStorage.getItem('evt_token');
    const base = localStorage.getItem('evt_base') || process.env.NEXT_PUBLIC_API_URL?.replace('/v1','') || 'http://localhost:8080';
    fetch(`${base}/v1/admin/merch/oversold`, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json()).then(d => setOversoldCount((d.data || []).length)).catch(() => {});
  }, [user]);

  if (isLoading || !user) {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--bg)' }}>
        <div className="spinner" />
      </div>
    );
  }

  function handleLogout() { logout(); router.replace('/login'); }

  const initials = user.email.slice(0, 2).toUpperCase();

  return (
    <ToastProvider>
      <ConfirmProvider>
        <div style={{ display: 'grid', gridTemplateColumns: '220px 1fr', height: '100vh', overflow: 'hidden' }}>
          {/* Sidebar */}
          <aside style={{ background: 'var(--surface)', borderRight: '1px solid var(--border)', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            <div style={{ padding: '1.25rem 1rem', fontFamily: 'var(--font-mono)', fontSize: '.95rem', fontWeight: 600, color: 'var(--accent)', borderBottom: '1px solid var(--border)', letterSpacing: '.04em', flexShrink: 0 }}>
              ⬡ events
              <em style={{ color: 'var(--text-3)', fontStyle: 'normal', fontWeight: 400, fontSize: '.7rem', display: 'block', marginTop: '.1rem', letterSpacing: '.06em', textTransform: 'uppercase' }}>
                admin panel
              </em>
            </div>

            <nav style={{ flex: 1, padding: '.75rem 0', overflowY: 'auto' }}>
              {NAV.map((item, i) => {
                if ('section' in item) {
                  return (
                    <div key={i} style={{ fontFamily: 'var(--font-mono)', fontSize: '.6rem', textTransform: 'uppercase', letterSpacing: '.1em', color: 'var(--text-3)', padding: '.5rem 1rem .2rem', marginTop: '.5rem' }}>
                      {item.section}
                    </div>
                  );
                }
                const active = pathname === item.href || (item.href !== '/dashboard' && pathname.startsWith(item.href!));
                return (
                  <Link key={item.href} href={item.href!} style={{
                    display: 'flex', alignItems: 'center', gap: '.65rem', padding: '.55rem 1rem',
                    color: active ? 'var(--accent)' : 'var(--text-2)',
                    borderLeft: `2px solid ${active ? 'var(--accent)' : 'transparent'}`,
                    background: active ? 'var(--accent-d)' : 'transparent',
                    fontSize: '.88rem', textDecoration: 'none', marginBottom: '.1rem', transition: 'all .15s',
                  }}>
                    <span style={{ width: '16px', textAlign: 'center', fontSize: '13px', flexShrink: 0 }}>{item.icon}</span>
                    {item.label}
                    {item.badge && oversoldCount > 0 && (
                      <span style={{ marginLeft: 'auto', background: 'var(--danger)', color: '#fff', fontSize: '.65rem', fontFamily: 'var(--font-mono)', borderRadius: '99px', padding: '1px 6px', minWidth: '18px', textAlign: 'center' }}>
                        {oversoldCount}
                      </span>
                    )}
                  </Link>
                );
              })}
            </nav>

            <div style={{ padding: '.75rem 1rem', borderTop: '1px solid var(--border)', flexShrink: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '.5rem', fontSize: '.8rem' }}>
                <div style={{ width: '28px', height: '28px', borderRadius: '50%', background: 'var(--accent-d)', border: '1px solid var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontFamily: 'var(--font-mono)', fontSize: '.65rem', color: 'var(--accent)', flexShrink: 0 }}>
                  {initials}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ color: 'var(--text-2)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{user.email}</div>
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.6rem', color: 'var(--accent)', textTransform: 'uppercase' }}>administrator</div>
                </div>
                <button className="btn-icon" onClick={handleLogout} title="Logout" style={{ fontSize: '1rem' }}>⇥</button>
              </div>
            </div>
          </aside>

          {/* Content */}
          <main style={{ overflowY: 'auto', display: 'flex', flexDirection: 'column' }}>
            {children}
          </main>
        </div>
      </ConfirmProvider>
    </ToastProvider>
  );
}