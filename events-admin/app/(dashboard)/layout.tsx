'use client';
import { useEffect, useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Link from 'next/link';
import { ThemeToggle } from '../../lib/theme';
import { useAdminAuth } from '../../store/authStore';
import { ToastProvider } from '../../lib/toast';
import { ConfirmProvider } from '../../lib/confirm';
import { useStaff, ROLE_LABEL, Capability } from '../../lib/permissions';

// `need` is the capability that makes a link WORTH SHOWING.
//
// A link to a screen that will answer "Access denied" is worse than no link:
// it advertises a section the person cannot use and turns the sidebar into a
// list of things to ask for. Hidden, not disabled, for the same reason the
// Staff tab is hidden without staff.view.
//
// This is presentation only. Typing the URL directly still reaches the page,
// which still gates itself, which still calls an endpoint the server gates.
// Three layers, and only the last one is the control.
const NAV: Array<
  | { type: 'item'; icon: React.ReactNode; label: string; href: string; badge?: boolean | string; exact?: boolean; need?: Capability }
  | { type: 'divider' }
> = [
  {
    type: 'item', href: '/dashboard', need: 'user.list' as Capability, exact: true, label: 'Dashboard',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M2 11l8-8 8 8v7a1 1 0 01-1 1H3a1 1 0 01-1-1v-7z"/></svg>,
  },
  {
    // Sits directly under Dashboard, above the divider: the dashboard answers
    // "where are we" and this answers "which way are we moving" — they're read
    // together, so they belong in the same group.
    type: 'item', href: '/analytics', need: 'user.list' as Capability, label: 'Analytics',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M2 13h3v5H2v-5zm5.5-5h3v10h-3V8zM13 3h3v15h-3V3z"/></svg>,
  },
  { type: 'divider' },
  {
    type: 'item', href: '/events', need: 'event.view' as Capability, label: 'Events',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M6 2a1 1 0 00-1 1v1H4a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V6a2 2 0 00-2-2h-1V3a1 1 0 10-2 0v1H7V3a1 1 0 00-1-1zm0 5a1 1 0 000 2h8a1 1 0 100-2H6z" clipRule="evenodd"/></svg>,
  },
  {
    type: 'item', href: '/approvals', need: 'event.approve' as Capability, label: 'Event Approvals', badge: 'approval',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd"/></svg>,
  },
  {
    // Sits with Event Approvals: both are "things waiting on a decision", and
    // an approvals queue nobody navigates to is an approvals queue nobody
    // clears.
    type: 'item', href: '/requests', need: 'request.create' as Capability, label: 'Staff Requests',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M10 1a4.5 4.5 0 00-4.5 4.5V9H5a2 2 0 00-2 2v6a2 2 0 002 2h10a2 2 0 002-2v-6a2 2 0 00-2-2h-.5V5.5A4.5 4.5 0 0010 1zm3 8V5.5a3 3 0 10-6 0V9h6z" clipRule="evenodd"/></svg>,
  },
  {
    type: 'item', href: '/kyc', need: 'kyc.status' as Capability, label: 'KYC Review',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M4 4a2 2 0 00-2 2v8a2 2 0 002 2h12a2 2 0 002-2V6a2 2 0 00-2-2H4zm3 3a2 2 0 100 4 2 2 0 000-4zm-2.5 6.5a3.5 3.5 0 015 0 .75.75 0 01-.61 1.19H5.11a.75.75 0 01-.61-1.19zM12 8a1 1 0 100 2h3a1 1 0 100-2h-3zm0 3.5a1 1 0 100 2h3a1 1 0 100-2h-3z" clipRule="evenodd"/></svg>,
  },
  {
    type: 'item', href: '/organizers', need: 'bank.view_tail' as Capability, label: 'Organizers & Payouts',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M8.433 7.418c.155-.103.346-.196.567-.267v1.698a2.305 2.305 0 01-.567-.267C8.07 8.34 8 8.114 8 8c0-.114.07-.34.433-.582zM11 12.849v-1.698c.22.071.412.164.567.267.364.243.433.468.433.582 0 .114-.07.34-.433.582a2.305 2.305 0 01-.567.267z"/><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-14a1 1 0 10-2 0v.092a4.535 4.535 0 00-1.676.662C6.602 5.234 6 6.09 6 7c0 .91.602 1.766 1.324 2.246.48.32 1.054.545 1.676.662v1.941c-.391-.127-.68-.317-.843-.504a1 1 0 10-1.51 1.31c.562.649 1.413 1.076 2.353 1.253V14a1 1 0 102 0v-.092a4.535 4.535 0 001.676-.662C13.398 12.766 14 11.91 14 11c0-.91-.602-1.766-1.324-2.246A4.535 4.535 0 0011 8.092V6.151c.391.127.68.317.843.504a1 1 0 101.511-1.31c-.563-.649-1.413-1.076-2.354-1.253V4z" clipRule="evenodd"/></svg>,
  },
  {
    type: 'item', href: '/merch', need: 'event.view' as Capability, label: 'Merch & Collections',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M10 2a4 4 0 00-4 4v1H5a1 1 0 00-.994.89l-1 9A1 1 0 004 18h12a1 1 0 00.994-1.11l-1-9A1 1 0 0015 7h-1V6a4 4 0 00-4-4zm2 5V6a2 2 0 10-4 0v1h4zm-6 3a1 1 0 112 0 1 1 0 01-2 0zm7-1a1 1 0 100 2 1 1 0 000-2z" clipRule="evenodd"/></svg>,
  },
  { type: 'divider' },
  {
    type: 'item', href: '/users', need: 'user.list' as Capability, label: 'All Users',
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M9 6a3 3 0 11-6 0 3 3 0 016 0zM17 6a3 3 0 11-6 0 3 3 0 016 0zM12.93 17c.046-.327.07-.66.07-1a6.97 6.97 0 00-1.5-4.33A5 5 0 0119 16v1h-6.07zM6 11a5 5 0 015 5v1H1v-1a5 5 0 015-5z"/></svg>,
  },
  { type: 'divider' },
  {
    type: 'item', href: '/oversold', need: 'event.view' as Capability, label: 'Oversold Orders', badge: true,
    icon: <svg viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd"/></svg>,
  },
];

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, hydrate, isLoading, logout } = useAdminAuth();
  const [oversoldCount, setOversoldCount] = useState(0);
  const [approvalCount, setApprovalCount] = useState(0);

  const capsLoaded  = useStaff((s) => s.loaded);
  const can         = useStaff((s) => s.can);
  const loadCaps    = useStaff((s) => s.load);
  const staffRole   = useStaff((s) => s.role);
  const legacyAdmin = useStaff((s) => s.legacyAdmin);

  useEffect(() => { hydrate(); }, [hydrate]);
  useEffect(() => { if (!isLoading && !user) router.replace('/login'); }, [user, isLoading, router]);

  // Loaded once here, in the shell, rather than per page. Every gated screen
  // still calls load() defensively, but the store is single-flight so those are
  // free once this has resolved — and the sidebar can render the right links on
  // first paint instead of flashing the full set and then removing half.
  useEffect(() => { if (user) loadCaps(); }, [user, loadCaps]);

  useEffect(() => {
    if (!user || !capsLoaded) return;
    const token = localStorage.getItem('evt_token');
    const base = localStorage.getItem('evt_base') || process.env.NEXT_PUBLIC_API_URL?.replace('/v1', '') || 'http://localhost:8080';
    const h = { Authorization: `Bearer ${token}` };
    // Badge counts are fetched only when the badge is reachable. Firing these
    // for a read_only user produced two guaranteed 403s on every page load,
    // each one written to staff_audit_log as a denied attempt — which is meant
    // to be the signal of a compromised account, and was instead the sound of
    // the console loading normally.
    if (can('event.view')) {
      fetch(`${base}/v1/admin/merch/oversold`, { headers: h })
        .then(r => r.ok ? r.json() : { data: [] })
        .then(d => setOversoldCount((d.data || []).length)).catch(() => {});
    }
    if (can('event.approve')) {
      fetch(`${base}/v1/admin/events/pending`, { headers: h })
        .then(r => r.ok ? r.json() : { events: [] })
        .then(d => setApprovalCount((d.events || []).length)).catch(() => {});
    }
  }, [user, capsLoaded, can]);

  if (isLoading || !user) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', background: 'var(--bg)' }}>
        <div className="spinner spinner-lg" />
      </div>
    );
  }

  function isActive(href: string, exact?: boolean) {
    if (exact) return pathname === href;
    return pathname === href || pathname.startsWith(href + '/');
  }

  // Until capabilities have loaded, show nothing gated rather than everything.
  // The other order flashes the full sidebar to a read_only user for as long as
  // the request takes, which is exactly the set of links they may not use.
  const permitted = NAV.filter(
    (item) => item.type === 'divider' || !item.need || (capsLoaded && can(item.need)),
  );
  // Dropping items can strand a divider at either end or leave two adjacent,
  // which reads as a rendering fault rather than a permission boundary.
  const visibleNav = permitted.filter((item, i, arr) => {
    if (item.type !== 'divider') return true;
    const prev = arr.slice(0, i).reverse().find((x) => x.type !== 'divider');
    const next = arr.slice(i + 1).find((x) => x.type !== 'divider');
    if (!prev || !next) return false;
    return arr[i - 1]?.type !== 'divider';
  });

  async function handleLogout() { await logout(); router.replace('/login'); }
  const initials = (user.email ?? '??').slice(0, 2).toUpperCase();

  return (
    <ToastProvider>
      <ConfirmProvider>
        <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>

          {/* Sidebar */}
          <aside style={{
            width: 244, flexShrink: 0,
            background: 'var(--surface)',
            borderRight: '1px solid var(--border)',
            display: 'flex', flexDirection: 'column', overflow: 'hidden',
          }}>
            {/* Logo */}
            <div style={{ padding: '16px 16px 12px', borderBottom: '1px solid var(--border)', flexShrink: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                {/* Was a generic house glyph on a blue square. Same mark the
                    organiser dashboard and the phone apps use, rendered without
                    a plate — the artwork carries its own circular shape. */}
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src="/brand-logo.png"
                  alt=""
                  width={32}
                  height={32}
                  style={{ display: 'block', width: 32, height: 32, flexShrink: 0 }}
                />
                <div>
                  <div style={{ fontSize: 15, fontWeight: 800, letterSpacing: 0.4, color: 'var(--text)', lineHeight: 1.2 }}>TICKET<span style={{ color: 'var(--brand)' }}>HAWK</span></div>
                  <div style={{ fontSize: 11, color: 'var(--text-2)', lineHeight: 1 }}>Admin Panel</div>
                </div>
              </div>
            </div>

            {/* Nav */}
            <nav style={{ flex: 1, padding: '8px 0', overflowY: 'auto' }}>
              {visibleNav.map((item, i) => {
                if (item.type === 'divider') {
                  return <div key={`d${i}`} style={{ height: 1, background: 'var(--bg)', margin: '4px 0' }} />;
                }
                const active = isActive(item.href, item.exact);
                return (
                  <Link key={item.href} href={item.href} style={{
                    display: 'flex', alignItems: 'center', gap: 10,
                    padding: '8px 12px', margin: '1px 8px', borderRadius: 6,
                    textDecoration: 'none',
                    color: active ? 'var(--accent)' : 'var(--text)',
                    background: active ? 'var(--accent-soft)' : 'transparent',
                    fontWeight: active ? 600 : 500, fontSize: 14,
                    transition: 'background .12s, color .12s',
                  }}>
                    <span style={{ color: active ? 'var(--accent)' : 'var(--text-2)', flexShrink: 0 }}>{item.icon}</span>
                    <span style={{ flex: 1 }}>{item.label}</span>
                    {item.badge === true && oversoldCount > 0 && (
                      <span style={{
                        background: 'var(--danger)', color: 'var(--surface)', fontSize: 11, fontWeight: 700,
                        borderRadius: '99px', padding: '1px 7px', minWidth: 20, textAlign: 'center',
                      }}>
                        {oversoldCount}
                      </span>
                    )}
                    {item.badge === 'approval' && approvalCount > 0 && (
                      <span style={{
                        background: 'var(--warning)', color: 'var(--surface)', fontSize: 11, fontWeight: 700,
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
            <div style={{ padding: '12px 16px', borderTop: '1px solid var(--border)', flexShrink: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <div style={{
                  width: 32, height: 32, borderRadius: '50%',
                  background: 'var(--accent)', color: 'var(--surface)',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  fontSize: 12, fontWeight: 700, flexShrink: 0,
                }}>
                  {initials}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{user.email}</div>
                  {/* The real role, not the word "Administrator" for everyone.
                      Someone who cannot approve an event should not be told
                      they are an administrator — they will file a bug when the
                      button is missing. */}
                  <div style={{ fontSize: 11, color: 'var(--text-2)' }}>
                    {legacyAdmin
                      ? 'Admin (no role set)'
                      : staffRole ? ROLE_LABEL[staffRole] : 'Administrator'}
                  </div>
                </div>
                <button onClick={handleLogout} title="Sign out"
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-3)', padding: 4, borderRadius: 4, display: 'flex', alignItems: 'center' }}>
                  <svg viewBox="0 0 20 20" fill="currentColor" width="16" height="16">
                    <path fillRule="evenodd" d="M3 3a1 1 0 00-1 1v12a1 1 0 001 1h4a1 1 0 100-2H4V5h3a1 1 0 100-2H3zm10.293 9.293a1 1 0 001.414 1.414l3-3a1 1 0 000-1.414l-3-3a1 1 0 10-1.414 1.414L14.586 9H7a1 1 0 100 2h7.586l-1.293 1.293z" clipRule="evenodd"/>
                  </svg>
                </button>
              </div>

              {/* Theme switch lives in the sidebar foot rather than a top bar,
                  because this shell has no top bar to put it in. */}
              <div style={{ display: 'flex', justifyContent: 'center', marginTop: 10 }}>
                <ThemeToggle />
              </div>
            </div>
          </aside>

          {/* Main */}
          {/* overflow:auto, not hidden — the shell is height:100vh, so anything
              taller than the viewport was clipped with no scrollbar at all.
              display:flex is gone too: as a flex item, a page wrapper with
              `margin: 0 auto` loses align-items:stretch and shrinks to its
              CONTENT width, which is why the dashboard sat in a narrow column
              with empty space either side. As a block container, the same
              wrapper fills the width and caps at its max-width. */}
          <main style={{ flex: 1, minWidth: 0, overflow: 'auto' }}>
            {children}
          </main>
        </div>
      </ConfirmProvider>
    </ToastProvider>
  );
}