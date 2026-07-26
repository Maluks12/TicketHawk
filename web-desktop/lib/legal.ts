// ─────────────────────────────────────────────────────────────────────────────
// Legal content — single source of truth.
//
// Every legal page on the site renders from this file. Nothing here is styled
// or laid out; that is components/legal-doc.tsx. Keeping the words separate
// from the markup means an attorney can review and edit this file without
// touching React, and means a change to the company address updates the Terms,
// the Privacy Policy and the PAIA Manual at once instead of in three places.
//
// ── THIS FILE IS DUPLICATED ──────────────────────────────────────────────────
// web-desktop/lib/legal.ts and web-mobile/lib/legal.ts must stay in sync. The
// two Next apps share no package, the same way lib/paymentMethods.ts is
// triplicated. Publishing different terms on desktop and mobile is not a
// cosmetic inconsistency — it is two different contracts with your buyers.
//
// ── BEFORE THIS GOES LIVE ────────────────────────────────────────────────────
// Source document: "SA-Compliant Legal Master File". Its own opening paragraph
// says it is a template, not a finished legal document, and that it needs
// sign-off from a South African admitted attorney with consumer protection and
// data privacy experience before any part of it is published or relied on.
// That has not happened yet. The internal DRAFTING NOTES from the source have
// been stripped here (they are working notes, not terms), but stripping them
// does not resolve the questions they raised.
//
// Search this file for TODO. Each one is a value that is legally required to be
// disclosed and is not yet known.
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Company and contact details.
 *
 * ECTA Section 43 requires a supplier transacting electronically to disclose
 * its legal name, registration number, physical address and contact details.
 * These are not optional fields — an incomplete disclosure is a compliance
 * gap, and it is also exactly what URL-categorisation reviewers and payment
 * partners look for when deciding whether a site is a real business.
 */
export const COMPANY = {
  tradingName: 'TicketHawk',
  legalName: 'Self Hawk Tech (Pty) Ltd',

  // TODO: CIPC registration number, in the form 2024/123456/07.
  // Required by ECTA s43(1)(b). Leave as null and the pages will render a
  // visible placeholder rather than silently omitting a required disclosure.
  registrationNumber: '2026/449825/07',

  // TODO: registered physical address. Required by ECTA s43(1)(c).
  // A PO box does not satisfy this — it must be a physical address.
  physicalAddress: '8 Orion Avenue Sundowner Randburg 2161',

  supportEmail: 'support@ticket-hawk.com',
  legalEmail: 'legal@ticket-hawk.com',
  takedownEmail: 'takedown@ticket-hawk.com',
  securityEmail: 'security@ticket-hawk.com',

  // TODO: Information Officer name and email.
  // POPIA requires the Information Officer to be REGISTERED with the
  // Information Regulator before they may take up the role — appointing
  // someone internally is not sufficient on its own. Registration is free at
  // inforegulator.org.za. By default the responsible person is the CEO/head of
  // the company unless someone else is designated in writing.
  informationOfficerName: null as string | null,
  informationOfficerEmail: 'privacy@ticket-hawk.com',

  website: 'https://ticket-hawk.com',
} as const;

/** Rendered wherever a TODO value is still unset, so gaps are visible rather than silent. */
export const MISSING = '[to be confirmed]';

export const val = (v: string | null): string => v ?? MISSING;

/** Shown as "Last updated" on every document. Bump when the text changes. */
export const LAST_UPDATED = '26 July 2026';

// ─────────────────────────────────────────────────────────────────────────────
// Refund schedule
//
// ⚠ THIS MUST MATCH events-db/internal/models/refund_policy.go.
//
// The bands below mirror `RefundSchedule` in the Go backend, which is what
// actually prices and pays a refund. The published policy and the code that
// executes it must agree — if they diverge, the buyer is quoted one figure by
// the API and promised another by this page, and the published policy is the
// one that binds you.
//
// NOTE — the source legal document differs from the code here. It lists
// "14 to 29 days: 50% refund", which merges two bands. The code has a distinct
// 21–29 day band at 75%, so a buyer cancelling 25 days out is paid 75% by the
// API while the document promises 50%. The code's schedule is reproduced below
// because it is what runs today and it is the more generous of the two, which
// is the safer direction under the CPA. If you decide the document is correct
// instead, change BOTH this array and RefundSchedule in refund_policy.go —
// changing one alone recreates the mismatch.
// ─────────────────────────────────────────────────────────────────────────────

export interface RefundBand {
  window: string;
  refund: string;
  note?: string;
}

export const REFUND_BANDS: RefundBand[] = [
  { window: '30 days or more before the event', refund: '100% of the ticket price' },
  { window: '21 to 29 days before the event', refund: '75% of the ticket price' },
  { window: '14 to 20 days before the event', refund: '50% of the ticket price' },
  { window: '7 to 13 days before the event', refund: '25% of the ticket price' },
  { window: 'Less than 7 days before the event', refund: 'No refund', note: 'Refunds close entirely inside 7 days.' },
];