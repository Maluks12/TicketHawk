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
// Checkout and cart screens end in a `position: fixed` CTA bar pinned to the
// bottom of the viewport. A footer in normal document flow scrolls up behind
// that bar, so the copyright line ends up rendering *through* the Pay Now
// button and the legal links sit in a dead strip just above it. It reads as a
// layout bug, and on a phone it actively competes with the primary action at
// the exact moment the buyer is deciding to pay.
//
// Those routes carry their own links to the legal documents (the s49
// acknowledgement links to clause 7, and the notice above the button links to
// the Terms and Privacy Policy), so hiding the footer costs no reachability.
const HIDE_FOOTER_ON = [
  '/purchase',       // /events/[id]/purchase
  '/cart',           // /cart and /cart/checkout
  '/payment',        // /payment/[id] — hosted gateway return
];

export default function Footer() {
  const { colors } = useAppTheme();
  const pathname = usePathname() || '';

  // endsWith for /purchase so it matches the nested route without also
  // matching a future /purchases listing page.
  const hidden = HIDE_FOOTER_ON.some(
    (seg) => pathname.endsWith(seg) || pathname.startsWith(seg + '/') || pathname === seg,
  );
  if (hidden) return null;

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