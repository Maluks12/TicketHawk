'use client';

import { useAppTheme } from '@/themes/colors';

// ── Footer ───────────────────────────────────────────────────────────────────
// Rendered once in the root layout so it appears on every page site-wide.
export default function Footer() {
  const { colors } = useAppTheme();

  return (
    <footer
      className="px-6 py-6 text-center border-t"
      style={{ borderColor: colors.dividerColor, backgroundColor: colors.background }}
    >
      <p className="text-[12px]" style={{ color: colors.subText }}>
        TicketHawk is a product of Self Hawk Tech Pty Ltd
      </p>
      <p className="text-[11px] mt-1" style={{ color: colors.subText, opacity: 0.7 }}>
        &copy; {new Date().getFullYear()} Self Hawk Tech Pty Ltd. All rights reserved.
      </p>
    </footer>
  );
}