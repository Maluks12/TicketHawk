'use client';

import LegalDoc, { type Section } from '@/components/legal-doc';
import { COMPANY, val } from '@/lib/legal';

// ─────────────────────────────────────────────────────────────────────────────
// /legal — the documents that are not consumer purchase terms.
//
// Organizer Agreement, POPIA operator terms, ECTA take-down procedure, PAIA
// manual and the Vulnerability Disclosure Policy. These have real audiences —
// a rights holder sending a take-down, a researcher reporting a bug, someone
// exercising a PAIA request — but none of them is the person buying a ticket,
// which is why they are not on /terms.
//
// Two of these are also load-bearing for the business: the ECTA s77 take-down
// procedure is what supports a hosting safe-harbour claim, and the PAIA manual
// is a statutory publication requirement.
// ─────────────────────────────────────────────────────────────────────────────

const sections: Section[] = [
  {
    heading: '1. Organizer Agreement',
    blocks: [
      {
        kind: 'p',
        text:
          'These terms apply in addition to the Terms of Use if you list an event and sell tickets through TicketHawk.',
      },
      {
        kind: 'p',
        text:
          '**Identity verification.** Before you can receive payouts you must complete identity verification. We verify your South African ID against the Department of Home Affairs, and verify that the bank account you nominate belongs to you. This is required by the Financial Intelligence Centre Act (FICA). We may withhold payouts until verification is complete.',
      },
      {
        kind: 'p',
        text:
          '**We are not your agent.** TicketHawk provides software that lets you sell tickets. We are not the promoter, host or co-organiser of your event, and we do not assume any of your obligations to attendees or to any authority.',
      },
      {
        kind: 'p',
        text:
          '**Safety, permits and liability.** You are responsible for obtaining every permit, licence and approval your event requires, and for complying with the Safety at Sports and Recreational Events Act (SASREA) where it applies, including event categorisation and the associated safety planning. You must be able to produce that documentation before a high-capacity listing goes live. You remain solely liable for the safety of attendees at your event.',
      },
      {
        kind: 'p',
        text:
          '**Financial liability and chargebacks.** You are liable for chargebacks, refunds and disputes arising from your events. Where an event is cancelled or found to be fraudulent, we may freeze sales, refund attendees from your pending balance, and recover any shortfall from you.',
      },
      {
        kind: 'p',
        text:
          '**Commission.** We deduct a commission of 6% from your payout, calculated on gross ticket sales. This is separate from the service fee paid by the buyer.',
      },
      {
        kind: 'p',
        text:
          '**Attendee data.** Personal information we share with you about attendees is shared for the purpose of running your event. You are a responsible party in your own right for what you then do with it, and you must comply with POPIA — including not using attendee data for direct marketing without a lawful basis.',
      },
      {
        kind: 'p',
        text:
          '**Your content.** You grant us a non-exclusive licence to display your event name, description, artwork and media on the Platform and in Platform marketing, for as long as your event is listed.',
      },
      {
        kind: 'p',
        text:
          '**Indemnity.** You indemnify us against claims, losses and costs arising from your event, your content, or your breach of these terms, except to the extent caused by our own unlawful or grossly negligent conduct.',
      },
    ],
  },
  {
    heading: '2. Take-down notice procedure',
    blocks: [
      {
        kind: 'p',
        text:
          'Prepared in compliance with Section 77 of the Electronic Communications and Transactions Act. If you believe content on the Platform infringes your copyright or trademark, or is otherwise unlawful, you may send a take-down notice to our designated agent.',
      },
      {
        kind: 'list',
        items: [
          `**Email:** ${COMPANY.takedownEmail}`,
          `**Postal address:** ${val(COMPANY.physicalAddress)}`,
        ],
      },
      { kind: 'p', text: 'Under Section 77(1), a valid notice must be in writing and include:' },
      {
        kind: 'list',
        items: [
          'Your full name and physical address.',
          'Your written or electronic signature.',
          'The specific right you say has been infringed.',
          'The exact URL or other identification of the material.',
          'The remedial action you are asking for.',
          'Your contact details.',
          'A statement that the notice is made in good faith.',
          'A statement that the information in it is true and correct.',
        ],
      },
      {
        kind: 'callout',
        title: 'Warning',
        blocks: [
          {
            kind: 'p',
            text:
              'Under Section 77(2) of ECTA, a person who lodges a notification knowing that it materially misrepresents the facts is liable for damages for wrongful take-down.',
          },
        ],
      },
    ],
  },
  {
    heading: '3. PAIA manual',
    blocks: [
      {
        kind: 'p',
        text:
          'Prepared in terms of Section 51 of the Promotion of Access to Information Act 2 of 2000.',
      },
      {
        kind: 'list',
        items: [
          `**Legal name:** ${COMPANY.legalName}`,
          `**Registration number:** ${val(COMPANY.registrationNumber)}`,
          `**Information Officer:** ${val(COMPANY.informationOfficerName)}`,
          `**Email:** ${COMPANY.informationOfficerEmail}`,
          `**Physical address:** ${val(COMPANY.physicalAddress)}`,
        ],
      },
      {
        kind: 'p',
        text:
          'The Information Regulator publishes an official guide on how to use PAIA at inforegulator.org.za.',
      },
      { kind: 'p', text: '**Records we hold:**' },
      {
        kind: 'list',
        items: [
          '**Freely available:** website content and public event listings.',
          '**Available on formal request:** internal financial records, user and organiser databases, commercial contracts, and security protocols.',
        ],
      },
      {
        kind: 'p',
        text:
          'To request access to a record, complete the prescribed PAIA Form 2, send it to our Information Officer, and pay the prescribed statutory fee. We will respond within 30 days of a valid request, and will notify you in writing if an extension is needed.',
      },
    ],
  },
  {
    heading: '4. Vulnerability disclosure policy',
    blocks: [
      {
        kind: 'p',
        text:
          'We value the security community and encourage responsible reporting of vulnerabilities affecting TicketHawk.',
      },
      { kind: 'p', text: '**Rules of engagement.** If you find a vulnerability, please:' },
      {
        kind: 'list',
        items: [
          `Report it privately to ${COMPANY.securityEmail} before disclosing it publicly.`,
          'Do not exploit it to access, modify or delete user data.',
          'Do not run denial-of-service attacks or spam the Platform.',
          'Do not test our sub-processors — such as Paystack or Amazon Web Services — directly.',
        ],
      },
      {
        kind: 'p',
        text:
          '**Safe harbour.** If you research in good faith and follow this policy, we will not take legal action against you under the Cybercrimes Act or otherwise, and we will work with you to fix the issue promptly. This is a commitment by us not to pursue action against you. It does not bind third parties, which is why testing our sub-processors directly is out of scope above.',
      },
    ],
  },
];

export default function LegalPage() {
  return (
    <LegalDoc
      title="Legal & compliance"
      intro={`Documents that sit alongside our Terms of Use and Privacy Policy: the agreement that applies to event Organizers, how to send us a take-down notice, our PAIA manual, and how to report a security vulnerability.`}
      sections={sections}
      seeAlso={[
        { href: '/terms', label: 'Terms of Use' },
        { href: '/privacy', label: 'Privacy Policy' },
      ]}
    />
  );
}