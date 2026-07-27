'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useAppTheme } from '@/themes/colors';
import { COMPANY } from '@/lib/legal';

// ── Footer ───────────────────────────────────────────────────────────────────
// Rendered once in the root layout, so it appears on every page — except the
// ones listed below.
//
// ── Why some routes hide it ──────────────────────────────────────────────────
// Event, cart and checkout screens end in a `position: fixed` CTA bar pinned to the
// bottom of the viewport. A footer in normal document flow scrolls up behind
// that bar, so the copyright line ends up rendering *through* the Pay Now
// button and the legal links sit in a dead strip just above it. It reads as a
// layout bug, and on a phone it actively competes with the primary action at
// the exact moment the buyer is deciding to pay.
//
// Those routes carry their own links to the legal documents (the s49
// acknowledgement links to clause 7, and the notice above the button links to
// the Terms and Privacy Policy), so hiding the footer costs no reachability.
// Matched as anchored patterns, not substrings. The first attempt used
// endsWith/startsWith on path fragments, which both missed /events/[id] (its
// CTA is declared inside a template-literal <style> block, so it does not look
// like the others) and would have matched any future route that merely
// contained one of those words.
const HIDE_FOOTER_PATTERNS: RegExp[] = [
  /^\/events\/[^/]+$/,            // event detail — "Get Tickets" bar
  /^\/events\/[^/]+\/purchase$/,  // ticket selection → summary → pay
  /^\/cart$/,                     // cart — "Checkout" bar
  /^\/cart\/checkout$/,           // reserve → pay
  /^\/payment\/[^/]+$/,           // hosted gateway return — transient
];

export default function Footer() {
  const { colors } = useAppTheme();
  const pathname = usePathname() || '';

  // Normalise a trailing slash so /cart/ and /cart both match.
  const path = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname;
  if (HIDE_FOOTER_PATTERNS.some((re) => re.test(path))) return null;

  const LEGAL_LINKS = [
    { href: '/terms',   label: 'Terms of Use' },
    { href: '/privacy', label: 'Privacy Policy' },
    { href: '/legal',   label: 'Legal' },
  ];

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