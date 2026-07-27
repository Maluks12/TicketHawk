'use client';

import { useEffect } from 'react';
import { usePathname } from 'next/navigation';
import { rememberReturnPath } from '@/lib/returnPath';

// ── ReturnPathTracker ────────────────────────────────────────────────────────
// Mounted once in the root layout. Records every page the user visits that is
// safe to return to, so the login page knows where to send them afterwards
// without router.back() guessing.
//
// Centralising it here is the point: the ~10 places that push to /auth/login
// need no changes, and — more importantly — no future call site can forget to
// pass a return path and silently reintroduce the reset-password loop.
//
// Auth-flow routes are filtered out inside rememberReturnPath, not here, so the
// rule lives next to the definition of what counts as safe.
export default function ReturnPathTracker() {
  const pathname = usePathname();

  useEffect(() => {
    if (pathname) rememberReturnPath(pathname);
  }, [pathname]);

  return null;
}