'use client';

import Link from 'next/link';
import { useAppTheme } from '@/themes/colors';
import { COMPANY } from '@/lib/legal';

// ── Footer ───────────────────────────────────────────────────────────────────
// Rendered once in the root layout so it appears on every page site-wide.
//
// The legal links are here rather than in the header for the ordinary reason
// that footers are where people look for them, but also because the app's
// checkout screen links to /terms and /privacy directly: those routes have to
// exist and be reachable, and a buyer who taps through from the payment screen
// should be able to find their way back to the rest.
//
// next/link, not <a>: these are internal routes, and client-side navigation
// avoids a full document reload from a page the user may be mid-checkout on.

const LEGAL_LINKS = [
  { href: '/terms',   label: 'Terms of Use' },
  { href: '/privacy', label: 'Privacy Policy' },
  { href: '/legal',   label: 'Legal' },
];

export default function Footer() {
  const { colors } = useAppTheme();

  return (
    <footer
      className="px-6 py-8 border-t"
      style={{ borderColor: colors.dividerColor, backgroundColor: colors.background }}
    >
      <nav
        className="flex flex-wrap items-center justify-center gap-x-6 gap-y-2 mb-5"
        aria-label="Legal"
      >
        {LEGAL_LINKS.map((link) => (
          <Link
            key={link.href}
            href={link.href}
            className="text-[12.5px] no-underline transition-opacity hover:opacity-100"
            style={{ color: colors.text, opacity: 0.65 }}
          >
            {link.label}
          </Link>
        ))}
        <a
          href={`mailto:${COMPANY.supportEmail}`}
          className="text-[12.5px] no-underline transition-opacity hover:opacity-100"
          style={{ color: colors.text, opacity: 0.65 }}
        >
          Contact
        </a>
      </nav>

      <p className="text-[12px] text-center" style={{ color: colors.subText }}>
        TicketHawk is a product of {COMPANY.legalName}
      </p>
      <p
        className="text-[11px] text-center mt-1"
        style={{ color: colors.subText, opacity: 0.7 }}
      >
        &copy; {new Date().getFullYear()} {COMPANY.legalName}. All rights reserved.
      </p>
    </footer>
  );
}