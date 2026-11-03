'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useEffect, useRef, useState } from 'react';
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
// acknowledgement links to clause 5, and the notice above the button links to
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

// ── Contact ──────────────────────────────────────────────────────────────────
//
// Contact used to be a bare mailto: link. A browser hands mailto: to the
// computer's default mail APP — and most desktop visitors don't have one set
// up; they use Gmail or Outlook in a browser tab. For them the click did
// nothing at all, which reads as a broken link.
//
// So Contact opens a small menu instead: the address itself (with Copy, which
// always works), compose links for the two webmail services most people use,
// and the original mailto: for anyone who does have a mail app.
const CONTACT_SUBJECT = 'TicketHawk support';

function contactLinks(email: string) {
  const to = encodeURIComponent(email);
  const subject = encodeURIComponent(CONTACT_SUBJECT);
  return [
    { label: 'Open in Gmail', href: `https://mail.google.com/mail/?view=cm&fs=1&to=${to}&su=${subject}`, newTab: true },
    { label: 'Open in Outlook.com', href: `https://outlook.live.com/mail/0/deeplink/compose?to=${to}&subject=${subject}`, newTab: true },
    { label: 'Use my email app', href: `mailto:${email}?subject=${subject}`, newTab: false },
  ];
}

export default function Footer() {
  const { colors } = useAppTheme();
  const pathname = usePathname() || '';

  // Hooks run before the early return below, on every render.
  const [contactOpen, setContactOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const contactRef = useRef<HTMLDivElement>(null);
  const addressRef = useRef<HTMLSpanElement>(null);

  // Close on Escape or on a click anywhere outside the menu.
  useEffect(() => {
    if (!contactOpen) return;
    const onPointerDown = (e: MouseEvent) => {
      if (contactRef.current && !contactRef.current.contains(e.target as Node)) {
        setContactOpen(false);
      }
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setContactOpen(false);
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [contactOpen]);

  // "Copied" reverts after a moment so a second copy still gets feedback.
  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(t);
  }, [copied]);

  const copyAddress = async () => {
    try {
      await navigator.clipboard.writeText(COMPANY.supportEmail);
      setCopied(true);
    } catch {
      // Clipboard API refused (older browser, or not a secure context):
      // select the address so Ctrl+C works instead.
      const node = addressRef.current;
      const selection = typeof window !== 'undefined' ? window.getSelection() : null;
      if (node && selection) {
        const range = document.createRange();
        range.selectNodeContents(node);
        selection.removeAllRanges();
        selection.addRange(range);
      }
    }
  };

  // Normalise a trailing slash so /cart/ and /cart both match.
  const path = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname;
  if (HIDE_FOOTER_PATTERNS.some((re) => re.test(path))) return null;

  // About, Terms, Refund, Delivery and Privacy are each required, as a page
  // reachable from the live site, by the acquiring bank's e-commerce
  // checklist — keep all five here.
  const LEGAL_LINKS = [
    { href: '/about',           label: 'About Us' },
    { href: '/terms',           label: 'Terms of Use' },
    { href: '/refund-policy',   label: 'Refund Policy' },
    { href: '/delivery-policy', label: 'Delivery Policy' },
    { href: '/privacy',         label: 'Privacy Policy' },
    { href: '/legal',           label: 'Legal' },
    // Listed here as well as being the Support URL given to Apple and Google.
    // Both stores open that URL during review, and a support page reachable
    // only by typing it in reads as one made purely to satisfy the checklist.
    { href: '/support', label: 'Support' },
  ];

  // ── Organiser dashboard link ───────────────────────────────────────────────
  //
  // A real <a href>, and that is the entire point.
  //
  // The only other route from this site to the organiser dashboard is a
  // window.open() inside a click handler on the authenticated profile page.
  // Crawlers do not run click handlers and never reach an authed page, so
  // organiser.ticket-hawk.com had NO crawlable inbound link from anywhere —
  // which is why site:organiser.ticket-hawk.com returned nothing at all.
  //
  // Being in the footer means it appears on every public page here, so it is
  // both a crawl path onto the subdomain and a way for this domain's authority
  // to reach it.
  //
  // Deliberately NOT rel="nofollow" and NOT target="_blank" with
  // rel="noopener noreferrer" alone: nofollow would withhold the very signal
  // this link exists to pass. It is a property we own, not untrusted content.
  //
  // Uses <a>, not next/link: next/link is for in-app routes and prefetches
  // them. This is a different origin, so a plain anchor is both correct and
  // what a crawler reads most directly.
  const ORGANISER_URL =
    process.env.NEXT_PUBLIC_ORGANISER_URL ?? 'https://organiser.ticket-hawk.com';

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

        <div ref={contactRef} className="relative inline-block">
          <button
            type="button"
            onClick={() => setContactOpen((open) => !open)}
            aria-haspopup="dialog"
            aria-expanded={contactOpen}
            aria-controls="footer-contact-menu"
            className="text-[12.5px] transition-opacity hover:opacity-100 cursor-pointer"
            style={{
              color: colors.text,
              opacity: contactOpen ? 1 : 0.65,
              background: 'none',
              border: 'none',
              padding: 0,
              font: 'inherit',
              fontSize: '12.5px',
            }}
          >
            Contact
          </button>

          {contactOpen && (
            <div
              id="footer-contact-menu"
              role="dialog"
              aria-label="Contact TicketHawk support"
              className="absolute left-1/2 -translate-x-1/2 bottom-full mb-2 w-[260px] rounded-xl border p-3 shadow-lg z-50 text-left"
              style={{ backgroundColor: colors.cardBg, borderColor: colors.dividerColor }}
            >
              <p className="text-[11px] mb-1" style={{ color: colors.subText }}>
                Email us at
              </p>
              <div className="flex items-center justify-between gap-2 mb-3">
                <span
                  ref={addressRef}
                  className="text-[13px] font-semibold select-all break-all"
                  style={{ color: colors.text }}
                >
                  {COMPANY.supportEmail}
                </span>
                <button
                  type="button"
                  onClick={copyAddress}
                  className="shrink-0 rounded-md px-2 py-1 text-[11px] font-semibold cursor-pointer"
                  style={{ backgroundColor: colors.sectionCardBg, color: copied ? colors.ctaBg : colors.text, border: 'none' }}
                >
                  {copied ? 'Copied' : 'Copy'}
                </button>
              </div>

              <div className="border-t pt-2 flex flex-col" style={{ borderColor: colors.dividerColor }}>
                {contactLinks(COMPANY.supportEmail).map((link) => (
                  <a
                    key={link.label}
                    href={link.href}
                    {...(link.newTab ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
                    onClick={() => setContactOpen(false)}
                    className="rounded-md px-2 py-1.5 text-[12.5px] no-underline transition-opacity hover:opacity-80"
                    style={{ color: colors.text }}
                  >
                    {link.label}
                  </a>
                ))}
              </div>
            </div>
          )}
        </div>

        <a
          href={ORGANISER_URL}
          className="text-[12.5px] no-underline transition-opacity hover:opacity-100"
          style={{ color: colors.text, opacity: 0.65 }}
        >
          Sell tickets
        </a>
      </nav>

      <p className="text-[12px] text-center" style={{ color: colors.subText }}>
        TicketHawk is a product of {COMPANY.legalName}
      </p>
      <p className="text-[11px] text-center mt-1" style={{ color: colors.subText }}>
        Registered in {COMPANY.countryOfDomicile}
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