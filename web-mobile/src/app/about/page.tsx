import type { Metadata } from 'next';
import LegalDoc, { type Section } from '@/components/legal-doc';
import { COMPANY, val } from '@/lib/legal';

// ─────────────────────────────────────────────────────────────────────────────
// /about — About Us.
//
// Required by the acquiring bank's e-commerce checklist, alongside the Terms,
// refund, delivery and privacy policies. A bank reviewer reads this page to
// answer two questions: is this a real South African business, and what does
// it actually sell? So it says both plainly, and carries the company details
// (legal name, registration number, country of domicile, currency) in one
// place instead of leaving them scattered through the Terms.
//
// A server component, unlike the other legal pages, only so it can export its
// own <title>. LegalDoc is a client component and the sections are plain data,
// so nothing else about rendering changes.
// ─────────────────────────────────────────────────────────────────────────────

export const metadata: Metadata = {
  title: 'About us | TicketHawk',
  description: `TicketHawk is a South African event ticketing platform operated by ${COMPANY.legalName}.`,
};

const sections: Section[] = [
  {
    heading: 'What we do',
    blocks: [
      {
        kind: 'p',
        text:
          'TicketHawk is an online ticketing platform for live events in South Africa — concerts, festivals, sports, nightlife, comedy and more. We bring together the people who put on events and the people who go to them.',
      },
      {
        kind: 'list',
        items: [
          '**For fans:** discover events, buy tickets securely, keep every ticket in one place, send tickets to friends, and buy official event merchandise.',
          '**For organisers:** list an event, sell tickets and merchandise, manage entry at the door, and get paid once identity verification is complete.',
        ],
      },
    ],
  },
  {
    heading: 'How we keep it safe',
    blocks: [
      {
        kind: 'list',
        items: [
          '**Secure payments.** Card payments are processed by Paystack, a Level 1 PCI-DSS compliant payment gateway. We never see or store your card number.',
          '**Verified organisers.** Every organiser must verify their identity and bank account before they can be paid, as required by the Financial Intelligence Centre Act.',
          '**Tickets that cannot be copied.** Your ticket carries a live QR code that refreshes every few seconds, so a screenshot is useless to anyone else.',
          '**Fair resale.** Tickets may not be resold above face value on TicketHawk.',
          '**Your data stays local.** Our platform is hosted in Amazon Web Services’ Cape Town region, and we protect your information in line with POPIA.',
        ],
      },
    ],
  },
  {
    heading: 'Company details',
    blocks: [
      {
        kind: 'list',
        items: [
          `**Trading name:** ${COMPANY.tradingName}`,
          `**Legal name:** ${COMPANY.legalName}`,
          `**Registration number:** ${val(COMPANY.registrationNumber)}`,
          `**Country of domicile:** ${COMPANY.countryOfDomicile}`,
          `**Physical address:** ${val(COMPANY.physicalAddress)}`,
          `**Transaction currency:** ${COMPANY.currencyName} (${COMPANY.currencyCode})`,
          '**Governing law:** the laws of the Republic of South Africa',
          `**Website:** ${COMPANY.website}`,
        ],
      },
    ],
  },
  {
    heading: 'Contact us',
    blocks: [
      {
        kind: 'list',
        items: [
          `**Customer support:** ${COMPANY.supportEmail}`,
          `**Privacy and personal information:** ${COMPANY.informationOfficerEmail}`,
          `**Legal notices:** ${COMPANY.legalEmail}`,
        ],
      },
      {
        kind: 'p',
        text: 'We reply to support email within one business day.',
      },
    ],
  },
];

export default function AboutPage() {
  return (
    <LegalDoc
      title="About us"
      intro={`TicketHawk is a South African event ticketing platform, owned and operated by ${COMPANY.legalName}, a private company registered in ${COMPANY.countryOfDomicile}.`}
      sections={sections}
      seeAlso={[
        { href: '/terms', label: 'Terms of Use' },
        { href: '/refund-policy', label: 'Refund Policy' },
        { href: '/delivery-policy', label: 'Delivery Policy' },
        { href: '/privacy', label: 'Privacy Policy' },
        { href: '/support', label: 'Support' },
      ]}
    />
  );
}