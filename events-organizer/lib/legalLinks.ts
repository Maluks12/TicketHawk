// ─────────────────────────────────────────────────────────────────────────────
// Legal document links
//
// Mirrors EventsApp/lib/legalLinks.ts, and for the same reason: the documents
// live on the website (web-desktop / web-mobile: src/app/terms, /privacy,
// /legal) rather than being duplicated per surface. Legal text changes on legal
// advice, not on a release cycle — one edit on the website updates every
// surface, and the version someone agreed to is the version actually published.
//
// The organiser agreement is the one document that is organiser-specific: it
// covers the commission split, settlement timing, chargeback liability and the
// refund policy an organiser is bound by. Buyers never see it.
// ─────────────────────────────────────────────────────────────────────────────

const WEB_ORIGIN = 'https://ticket-hawk.com';

export const LEGAL_URLS = {
  terms:             `${WEB_ORIGIN}/terms`,
  privacy:           `${WEB_ORIGIN}/privacy`,
  legal:             `${WEB_ORIGIN}/legal`,
  organiserAgreement: `${WEB_ORIGIN}/legal/organiser-agreement`,
} as const;

export type LegalDoc = keyof typeof LEGAL_URLS;

/**
 * Version of the agreement set currently in force.
 *
 * Bumping this is what re-prompts every organiser: the acceptance stored
 * against an account records the version accepted, and anything older than
 * CURRENT_LEGAL_VERSION means they have not seen the current terms. Without a
 * version, "accepted" would be permanent and a material change to the
 * commission split could never be re-consented.
 *
 * Format is a date so it is obvious at a glance which revision an organiser
 * agreed to. Bump it only when the change is material enough to warrant asking
 * again — typo fixes don't need re-consent, a change to the fee model does.
 */
export const CURRENT_LEGAL_VERSION = '2026-08-01';

/** The documents an organiser is asked to accept, in display order. */
export const ORGANISER_LEGAL_DOCS: Array<{ key: LegalDoc; label: string }> = [
  { key: 'terms',              label: 'Terms of Service' },
  { key: 'privacy',            label: 'Privacy Policy' },
  { key: 'organiserAgreement', label: 'Organiser Agreement' },
  { key: 'legal',              label: 'Legal Notices' },
];

/**
 * Renders a legal document link.
 *
 * Always opens in a new tab. An organiser reading the agreement mid-signup or
 * mid-KYC has a half-filled form behind them, and navigating away in the same
 * tab would lose it.
 */
export function legalLinkProps(doc: LegalDoc) {
  return {
    href: LEGAL_URLS[doc],
    target: '_blank' as const,
    rel: 'noopener noreferrer',
  };
}