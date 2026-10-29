'use client';

import LegalDoc, { type Section } from '@/components/legal-doc';
import { COMPANY, val } from '@/lib/legal';

// ─────────────────────────────────────────────────────────────────────────────
// /privacy — Privacy Policy (POPIA) and Cookie & Tracking Policy.
//
// Merged onto one page because a cookie policy is a description of one category
// of collection, and splitting it across two URLs makes a reader hunt for half
// the answer. The app's checkout links here.
//
// This is the document that most needs to be accurate rather than generic: the
// KYC flow processes SA ID numbers and bank account details, which are the
// higher-risk end of what POPIA governs.
// ─────────────────────────────────────────────────────────────────────────────

const sections: Section[] = [
  {
    heading: '1. Who is responsible for your data',
    blocks: [
      {
        kind: 'list',
        items: [
          `**Responsible party:** ${COMPANY.legalName}`,
          `**Registration number:** ${val(COMPANY.registrationNumber)}`,
          `**Physical address:** ${val(COMPANY.physicalAddress)}`,
          `**Information Officer:** ${val(COMPANY.informationOfficerName)}`,
          `**Contact for privacy matters:** ${COMPANY.informationOfficerEmail}`,
        ],
      },
    ],
  },
  {
    heading: '2. What we collect',
    blocks: [
      { kind: 'p', text: '**You give us directly:**' },
      {
        kind: 'list',
        items: [
          'Your name, email address and phone number when you create an account or buy a ticket as a guest.',
          'The name and email of anyone you assign a ticket to, so we can send them their claim link.',
          'Your delivery address, if you order merchandise for delivery.',
          'Your South African ID number, and supporting identity information, if you register as an event Organizer and complete identity verification.',
          'Your bank account details, if you are an Organizer receiving payouts.',
        ],
      },
      { kind: 'p', text: '**We collect automatically:**' },
      {
        kind: 'list',
        items: [
          'Your IP address, device identifier and general location, used for fraud prevention and rate limiting.',
          'How you browse the Platform — which events you view and search for — used to improve recommendations and to catch abuse.',
        ],
      },
      {
        kind: 'p',
        text:
          '**We never see your card number.** Card payments are handled entirely by Paystack, a Level 1 PCI-DSS compliant payment gateway. Your full card details are entered on Paystack’s systems and are never stored on ours.',
      },
    ],
  },
  {
    heading: '3. Why we process it',
    blocks: [
      {
        kind: 'list',
        items: [
          '**To issue and manage your tickets** — including generating your entry QR code and letting you transfer a ticket to someone else.',
          '**To take payment securely** through Paystack, and to process refunds.',
          '**To deliver merchandise** you order, or arrange its collection.',
          '**To verify Organizer identity**, as required by the Financial Intelligence Centre Act (FICA), before an Organizer can receive payouts.',
          '**To let Organizers manage entry** at their own events.',
          '**To prevent fraud** and detect abuse of the Platform.',
          '**To meet our legal obligations**, including records we are required to keep.',
        ],
      },
    ],
  },
  {
    heading: '4. Who we share it with',
    blocks: [
      {
        kind: 'list',
        items: [
          '**The event Organizer.** When you buy a ticket, we share your name, email and ticket details with the Organizer of that event, so they can admit you and contact you about the event.',
          '**Merchandise fulfilment.** If you order merchandise, we share your name, contact details and delivery address with the Organizer supplying it and with the courier delivering it, only so they can fulfil your order.',
          '**Our service providers.** We use sub-processors — including Amazon Web Services for hosting and Paystack for payments — who are bound by agreements requiring POPIA-level protection.',
          '**Law enforcement.** We will disclose data to the South African Police Service or the Financial Intelligence Centre where required by law, or where we suspect cyber fraud or money laundering.',
        ],
      },
      {
        kind: 'p',
        text: 'We do not sell your personal information, and we do not share it with advertisers.',
      },
    ],
  },
  {
    heading: '5. Where your data is stored',
    blocks: [
      {
        kind: 'p',
        text:
          'Your data is hosted in Amazon Web Services’ Cape Town region, so it stays in South Africa by default. Some sub-processors may store or process information outside South Africa. Where that happens, we do it on a basis permitted by Section 72 of POPIA — for example, because the recipient is bound by a written agreement imposing protections substantially similar to POPIA. You can request further detail on the specific safeguards from our Information Officer.',
      },
    ],
  },
  {
    heading: '6. Children',
    blocks: [
      {
        kind: 'p',
        text:
          'You must be 18 or older to create an account. If you buy a ticket on behalf of a minor, or an event is open to attendees under 18, we process the minor’s personal information only as far as necessary to issue and administer their ticket, in accordance with Sections 34 and 35 of POPIA. We do not knowingly use a minor’s data for direct marketing.',
      },
    ],
  },
  {
    heading: '7. Marketing',
    blocks: [
      {
        kind: 'p',
        text:
          'Marketing from us is subject to POPIA and the Consumer Protection Act Amendment Regulations, 2026. You will only receive marketing if you explicitly opt in.',
      },
      {
        kind: 'list',
        items: [
          `**Opting out of us.** You may withdraw consent at any time, free of charge, using the unsubscribe link in any marketing message or by contacting ${COMPANY.supportEmail}. We action this directly — you do not need to use the registry below.`,
          '**The NCC Opt-Out Registry.** You also have a statutory right to register a pre-emptive block against direct marketing industry-wide, or against a specific marketer, on the National Consumer Commission’s Opt-Out Registry.',
        ],
      },
      {
        kind: 'p',
        text:
          'Transactional messages — your ticket, your booking confirmation, a refund notice, an event cancellation — are not marketing, and you receive those regardless, because they are how we deliver what you bought.',
      },
    ],
  },
  {
    heading: '8. How long we keep it',
    blocks: [
      {
        kind: 'p',
        text:
          'We keep personal information only as long as necessary for the purpose it was collected for. The main exception is FICA, which requires us to keep financial and identity-verification records for five years after the end of the business relationship.',
      },
    ],
  },
  {
    heading: '9. Your rights',
    blocks: [
      { kind: 'p', text: 'Under POPIA you may:' },
      {
        kind: 'list',
        items: [
          'Ask what personal information we hold about you.',
          'Ask us to correct information that is inaccurate or out of date.',
          'Ask us to delete information we no longer have a lawful basis to keep.',
          'Object to processing in certain circumstances.',
          'Withdraw marketing consent at any time.',
          'Complain to the Information Regulator at inforegulator.org.za.',
        ],
      },
      {
        kind: 'p',
        text: `To exercise any of these, contact our Information Officer at ${COMPANY.informationOfficerEmail}. We will respond to a valid request within 30 days. We may extend that period where reasonably necessary, and if we do we will tell you and explain why.`,
      },
    ],
  },
  {
    heading: '10. Cookies and tracking',
    blocks: [
      {
        kind: 'p',
        text:
          'We use cookies, local storage and similar technologies to make TicketHawk work. They fall into three categories:',
      },
      {
        kind: 'list',
        items: [
          '**Strictly necessary** — login sessions, your cart, and checkout timers. The Platform cannot work without these and you cannot opt out of them.',
          '**Performance and analytics** — anonymised measurement of how the Platform performs, used to find and fix bugs.',
          '**Marketing and targeting** — third-party pixels that Organizers may use to measure ad conversions on their own event pages.',
        ],
      },
      {
        kind: 'p',
        text:
          'You can control non-essential cookies through your browser settings, or on mobile through your device’s privacy settings such as iOS App Tracking Transparency. Blocking strictly necessary cookies will stop checkout from working.',
      },
    ],
  },
  {
    heading: '11. If something goes wrong',
    blocks: [
      {
        kind: 'p',
        text:
          'If personal information under our control is accessed or acquired by an unauthorised person, we will notify the Information Regulator and any affected users as soon as reasonably possible, as required by Section 22 of POPIA.',
      },
    ],
  },
];

export default function PrivacyPage() {
  return (
    <LegalDoc
      title="Privacy Policy"
      intro={`${COMPANY.legalName} operates TicketHawk. This policy explains what personal information we collect, why, who we share it with, and the rights you have under the Protection of Personal Information Act (POPIA).`}
      sections={sections}
      seeAlso={[
        { href: '/terms', label: 'Terms of Use' },
        { href: '/legal', label: 'Legal & compliance' },
      ]}
    />
  );
}