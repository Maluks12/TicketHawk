'use client';

import { useEffect } from 'react';
import { useAppTheme } from '@/themes/colors';

// ─────────────────────────────────────────────────────────────────────────────
// Keeps <html>/<body> and the browser chrome in step with the ACTIVE theme.
//
// ── The bug this fixes ───────────────────────────────────────────────────────
//
// global.css sets the body background from `prefers-color-scheme` — a pure CSS
// fallback, so it paints before any JS runs and there is no white flash on load.
// That is worth keeping. But it tracks the OPERATING SYSTEM, while everything
// inside <main> tracks the theme the user actually chose in the app.
//
// Those two agree only while the app is on "auto". A user whose phone is in dark
// mode but who has picked light in TicketHawk gets a black <body> under white
// content, and black shows through anywhere the themed elements do not reach:
//
//   • the strip between the end of <main> and the footer;
//   • any area below the footer on a short page;
//   • the rubber-band overscroll region at the top and bottom in Safari;
//   • Safari's own toolbar, which samples the page background to tint itself —
//     which is why the browser chrome went black under a white page.
//
// ThemedMain already paints <main>. This does the same for the document itself,
// so there is no uncovered surface left to disagree.
//
// ── Why theme-color is set here and not in layout metadata ───────────────────
//
// Next's metadata `themeColor` is static, resolved on the server, and cannot
// know a preference stored in the client's localStorage. The meta tag has to be
// written after the theme resolves, which means client-side.
// ─────────────────────────────────────────────────────────────────────────────

export default function ThemeSync() {
  const { colors, isDark } = useAppTheme();

  useEffect(() => {
    const bg = colors.background;

    // Both elements: body alone leaves the overscroll region using html's
    // background on iOS.
    document.documentElement.style.backgroundColor = bg;
    document.body.style.backgroundColor = bg;

    // Lets the browser render native controls (form fields, scrollbars) for the
    // right scheme, and stops Safari inverting things it thinks are on a light
    // page when they are not.
    document.documentElement.style.colorScheme = isDark ? 'dark' : 'light';

    // Tints Safari's toolbar and Chrome's address bar to match.
    let meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
    if (!meta) {
      meta = document.createElement('meta');
      meta.name = 'theme-color';
      document.head.appendChild(meta);
    }
    meta.content = bg;
  }, [colors.background, isDark]);

  return null;
}