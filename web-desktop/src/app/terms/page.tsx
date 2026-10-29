'use client';

import LegalDoc, { type Section } from '@/components/legal-doc';
import { COMPANY, val } from '@/lib/legal';
import { REFUND_SECTIONS, DELIVERY_SECTIONS, asClauseBlocks } from '@/lib/policies';

// ─────────────────────────────────────────────────────────────────────────────
// /terms — Terms of Service, EULA, Refund & Cancellation Policy, Delivery
// Policy, Acceptable Use. The refund and delivery clauses render from
// lib/policies.ts, which also feeds the standalone /refund-policy and
// /delivery-policy pages.
//
// These four documents are merged onto one page on purpose. They are all the
// consumer-facing half of the contract, the app's checkout links here, and the
// refund rules are meaningless without the purchase terms they sit inside.
// Organiser-side and regulatory documents live at /legal.
//
// The risk/admission/liability terms render inside a bordered callout because
// CPA Section 49 requires terms of that kind to be brought to the consumer's
// attention conspicuously rather than set in ordinary body copy.
// ─────────────────────────────────────────────────────────────────────────────

const sections: Section[] = [
  {
    heading: 'Who we are',
    blocks: [
      {
        kind: 'p',
        text:
          'In compliance with Section 43 of the Electronic Communications and Transactions Act (ECTA), we disclose the following:',
      },
      {
        kind: 'list',
        items: [
          `**Supplier:** ${COMPANY.legalName}`,
          `**Registration number:** ${val(COMPANY.registrationNumber)}`,
          `**Physical address:** ${val(COMPANY.physicalAddress)}`,
          `**Country of domicile:** ${COMPANY.countryOfDomicile}`,
          `**Contact email:** ${COMPANY.supportEmail}`,
          `**Website:** ${COMPANY.website}`,
        ],
      },
    ],
  },
  {
    heading: '1. Licence to use TicketHawk',
    blocks: [
      {
        kind: 'p',
        text:
          'We grant you a limited, non-exclusive, non-transferable licence to use the Platform for personal ticket purchasing and event discovery.',
      },
      {
        kind: 'p',
        text:
          '**Automated access.** You may not use any robot, spider, scraper, automated script or other automated means to access the Platform, bypass queues, or purchase tickets. Tickets purchased using automated software will be cancelled without notice and without refund.',
      },
      {
        kind: 'p',
        text:
          '**Reverse engineering.** You may not reverse-engineer, decompile or disassemble the Platform’s software to discover source code or bypass security measures.',
      },
    ],
  },
  {
    heading: '2. Purchases, pricing and fees',
    blocks: [
      {
        kind: 'p',
        text:
          `**Currency.** All prices on the Platform are shown in ${COMPANY.currencyName} (${COMPANY.currencyCode}), and every transaction — payments, fees and refunds — is processed in ${COMPANY.currencyName}.`,
      },
      {
        kind: 'p',
        text:
          '**Pricing and VAT.** All prices shown on the Platform are inclusive of Value-Added Tax where applicable. Where our booking or processing fee is charged separately from the ticket face value, it is disclosed to you before you complete your purchase.',
      },
      {
        kind: 'p',
        text:
          '**Service fee.** We charge a service fee of up to R10 per ticket, capped at 20% of the ticket’s face value. The exact fee is shown to you at checkout before payment.',
      },
      {
        kind: 'p',
        text:
          '**Payment.** Card payments are processed by Paystack, a Level 1 PCI-DSS compliant payment gateway. We do not store your card number.',
      },
    ],
  },
  {
    heading: '3. Ticket transfers and resale',
    blocks: [
      {
        kind: 'p',
        text:
          '**Personal use.** Tickets are issued for personal use. You may not resell, or attempt to resell, a ticket above its original face value. We may cancel any ticket identified as being offered for resale above face value on a secondary market, without a refund. We do not guarantee the validity of tickets bought from unauthorised third parties.',
      },
      {
        kind: 'p',
        text:
          '**Permitted transfers.** This does not stop you giving or transferring a ticket you no longer need to another person at or below face value, using the Platform’s transfer feature where available. Transfers made outside that feature are at the sender’s and recipient’s own risk, and we cannot guarantee the validity of a ticket transferred that way.',
      },
    ],
  },
  {
    heading: '4. Lost, stolen or duplicated tickets',
    blocks: [
      {
        kind: 'p',
        text: `If your ticket, account or payment method is lost, stolen or compromised, contact ${COMPANY.supportEmail} immediately. We may reissue a ticket to the verified original purchaser, cancel and reissue affected tickets, or refer the matter to the Organizer’s on-the-day verification process, at our discretion. We are not liable for tickets used by a third party as a result of your failure to secure your account or your ticket.`,
      },
    ],
  },
  {
    heading: '5. Risk, admission and liability',
    blocks: [
      {
        kind: 'callout',
        title: 'Important — this section affects your legal rights',
        blocks: [
          {
            kind: 'p',
            text:
              'In accordance with Section 49 of the Consumer Protection Act, we specifically draw the following to your attention.',
          },
          {
            kind: 'p',
            text:
              '**(a) Right of admission reserved.** The Organizer and venue management reserve the right of admission. Attendees who are intoxicated, abusive, carrying prohibited items or posing a genuine security risk may be refused entry or removed at any time, and are not entitled to a refund. This right will not be exercised in a way that unfairly discriminates against any person on a ground prohibited by the Constitution or by PEPUDA.',
          },
          {
            kind: 'p',
            text:
              '**(b) Assumption of risk.** Attending an event carries inherent risks, which may include personal injury, illness or death, arising from crowd conditions, venue conditions, weather, the conduct of other attendees, or circumstances outside our control. By buying a ticket and attending, you acknowledge and accept those risks. This does not affect any right you may have against the Organizer or venue for their own negligence.',
          },
          {
            kind: 'p',
            text:
              '**(c) Age-restricted events.** You must be 18 or older to create an account. By buying a ticket to an age-restricted event you warrant that you meet the minimum age. The venue may ask for a valid South African ID or passport on entry. No refund is issued to an attendee refused entry for failing to prove their age.',
          },
        ],
      },
    ],
  },
  {
    heading: '6. Our role, and the limits of our liability',
    blocks: [
      {
        kind: 'p',
        text:
          'TicketHawk provides software that lets third-party Organizers sell tickets. We are not the creator, host or owner of the events listed. To the maximum extent permitted by South African law, we are not liable for direct, indirect, incidental or consequential damages arising from your attendance at an event, or for personal injury, death or property damage sustained at an event.',
      },
      {
        kind: 'p',
        text:
          'Nothing in these terms excludes or limits liability that cannot lawfully be excluded or limited, including liability for gross negligence under Section 51 of the Consumer Protection Act.',
      },
    ],
  },
  {
    heading: '7. Refund and cancellation policy',
    blocks: [
      {
        kind: 'p',
        text:
          'This policy is also published on its own page at ticket-hawk.com/refund-policy. Both say the same thing.',
      },
      ...asClauseBlocks(REFUND_SECTIONS),
    ],
  },
  {
    heading: '8. Delivery policy',
    blocks: [
      {
        kind: 'p',
        text:
          'This policy is also published on its own page at ticket-hawk.com/delivery-policy. Both say the same thing.',
      },
      ...asClauseBlocks(DELIVERY_SECTIONS),
    ],
  },
  {
    heading: '9. Acceptable use',
    blocks: [
      {
        kind: 'p',
        text: 'You and any Organizer using the Platform may not:',
      },
      {
        kind: 'list',
        items: [
          'Resell or attempt to resell a ticket above face value.',
          'Publish or sell tickets to events involving illegal gambling, the unlicensed sale of alcohol or firearms, hate speech or violence, or any activity that breaks South African law.',
          'Create fraudulent events or use "bait-and-switch" marketing. Where we determine an event is fraudulent we may freeze ticket sales immediately, refund attendees from the pending balance, and ban the Organizer.',
          'Use the Platform to commit cyber fraud, unauthorised access or data interference, contrary to the Cybercrimes Act 19 of 2020.',
        ],
      },
      {
        kind: 'p',
        text:
          '**Reporting cybercrime.** Where we suspect cybercrime we will suspend the accounts involved, report the matter to the South African Police Service within 72 hours, and hand over relevant evidence. To the extent we cooperate with a lawful investigation in good faith and in accordance with applicable law, you agree not to hold us liable for that cooperation. This does not waive any right you may have in respect of our own unlawful or grossly negligent conduct.',
      },
      {
        kind: 'p',
        text:
          'Security researchers should follow our Vulnerability Disclosure Policy, published with our other legal documents.',
      },
    ],
  },
  {
    heading: '10. Events outside anyone’s control',
    blocks: [
      {
        kind: 'p',
        text:
          'Neither we nor the Organizer is liable for a failure or delay caused by circumstances beyond reasonable control, including load-shedding or power disruption, extreme weather, civil unrest, strikes, pandemic or public health measures, or acts of government. Where an event is affected by such circumstances, the cancellation and postponement provisions above apply.',
      },
    ],
  },
  {
    heading: '11. Intellectual property',
    blocks: [
      {
        kind: 'p',
        text: `The TicketHawk name, logo, software and related content are the property of ${COMPANY.legalName} or its licensors and may not be used without prior written permission. Event names, descriptions, artwork and media displayed on the Platform belong to the relevant Organizer and are used under licence.`,
      },
    ],
  },
  {
    heading: '12. Resolving a dispute',
    blocks: [
      {
        kind: 'p',
        text:
          'Before going to court, a consumer dispute arising from this agreement may be referred to the Consumer Goods and Services Ombud (CGSO), an independent dispute resolution body established under the Consumer Protection Act, free of charge to consumers. If the CGSO does not resolve it, the matter may go to the National Consumer Commission and then the National Consumer Tribunal.',
      },
      {
        kind: 'p',
        text:
          'This does not limit your right to approach a court directly, or to approach the Information Regulator on a POPIA matter.',
      },
    ],
  },
  {
    heading: '13. Governing law',
    blocks: [
      {
        kind: 'p',
        text:
          `These terms are governed by the laws of the Republic of South Africa. ${COMPANY.legalName} is domiciled in ${COMPANY.countryOfDomicile}. Subject to the section above, disputes not resolved through the CGSO, NCC or Tribunal are subject to the exclusive jurisdiction of the South African courts.`,
      },
    ],
  },
  {
    heading: '14. General',
    blocks: [
      {
        kind: 'list',
        items: [
          '**Entire agreement.** These terms, together with the documents they expressly incorporate, are the entire agreement between us on their subject matter and supersede all prior discussions and representations.',
          '**Severability.** If any provision is found unlawful, void or unenforceable, it is severed and the rest remains in force.',
          '**No waiver.** A failure or delay in exercising a right under this agreement is not a waiver of that right.',
          '**Assignment.** You may not cede, assign or transfer your rights or obligations without our prior written consent. We may assign this agreement in connection with a merger, acquisition or sale of assets, or by operation of law.',
          `**Notices.** Legal notices to us must be sent to ${COMPANY.legalEmail}. We may give notice to you at the email address or phone number on your account, or by a prominent notice on the Platform.`,
          '**Changes.** We may update this agreement to reflect changes in our services or in the law. Where changes are material we will give reasonable notice — by email or an in-app notice — before they take effect. Continuing to use the Platform after that notice means you accept the updated terms.',
        ],
      },
    ],
  },
];

export default function TermsPage() {
  return (
    <LegalDoc
      title="Terms of Use"
      intro={`Welcome to TicketHawk, operated by ${COMPANY.legalName}. By using our website, downloading our app, or buying a ticket, you agree to these terms. They cover your purchase, our refund and delivery policies, and what we are and are not responsible for.`}
      sections={sections}
      seeAlso={[
        { href: '/privacy', label: 'Privacy Policy' },
        { href: '/refund-policy', label: 'Refund Policy' },
        { href: '/delivery-policy', label: 'Delivery Policy' },
        { href: '/about', label: 'About us' },
        { href: '/legal', label: 'Legal & compliance' },
      ]}
    />
  );
}