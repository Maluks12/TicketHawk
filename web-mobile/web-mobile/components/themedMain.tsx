'use client';

import type { ReactNode } from 'react';
import { useAppTheme } from '@/themes/colors';

// ── ThemedMain ───────────────────────────────────────────────────────────────
// Wraps <main> with an explicit background from the app's actual selected
// theme. Without this, unstyled space inside <main> falls back to <body>'s
// CSS-only background, which tracks the OS's prefers-color-scheme rather
// than the theme the user/app actually picked — causing mismatched
// black/white bars.
//
// The previous pb-[60px] (mobile-only) bottom padding has been removed: it
// was reserved for a bottom tab bar that isn't actually rendered anywhere in
// navBar.tsx, so it was just empty space. Add it back (with a real bottom
// nav component) if that UI gets built later.
export default function ThemedMain({ children }: { children: ReactNode }) {
  const { colors } = useAppTheme();

  return (
    <main style={{ backgroundColor: colors.background }}>
      {children}
    </main>
  );
}