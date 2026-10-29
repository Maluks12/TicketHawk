import type { Block, Section } from '@/components/legal-doc';
import { COMPANY, MERCH_DELIVERY, REFUND_BANDS } from '@/lib/legal';

// ─────────────────────────────────────────────────────────────────────────────
// Refund & cancellation policy, and delivery policy — single source of truth.
//
// Each policy is published twice: as its own page (/refund-policy,
// /delivery-policy) and as a clause inside the Terms of Use. The acquiring
// bank's e-commerce checklist looks for each one as a clearly labelled page,
// while the Terms are the contract the checkout links to and must contain them
// too. Both renderings read from the arrays below, so the two can never say
// different things — publishing two versions of a refund policy is publishing
// two contracts.
//
// ── THIS FILE IS DUPLICATED ──────────────────────────────────────────────────
// web-desktop/lib/policies.ts and web-mobile/lib/policies.ts must stay in sync,
// for the same reason as lib/legal.ts.
// ─────────────────────────────────────────────────────────────────────────────

export const REFUND_SECTIONS: Section[] = [
  {
    heading: 'Tickets: if you cancel',
    blocks: [
      {
        kind: 'p',
        text:
          'Under Section 17 of the Consumer Protection Act you may cancel an advance booking. A reasonable cancellation fee applies, based on how much notice you give:',
      },
      {
        kind: 'table',
        head: ['Notice given', 'You receive back'],
        rows: REFUND_BANDS.map((b) => [b.window, b.refund] as [string, string]),
      },
      {
        kind: 'p',
        text:
          'These percentages reflect our assessment of a reasonable cancellation fee, having regard to the notice given, the likelihood of resale, and standard practice for comparable events. The amount you will receive is shown to you before you confirm a refund request.',
      },
      {
        kind: 'p',
        text:
          '**Service fees on your own cancellation.** Where you cancel your own booking, or do not attend, our booking and processing fees are not refunded. Our service — facilitating your purchase and issuing your ticket — is fully rendered once your ticket is issued.',
      },
      {
        kind: 'p',
        text:
          '**Hospitalisation or death.** In accordance with Section 17 of the CPA, no cancellation fee applies if you cannot honour the booking because of hospitalisation or death. We may ask for supporting documentation.',
      },
      {
        kind: 'p',
        text:
          '**How to cancel.** Open the booking under **Tickets** on the website or in the app and choose **Request refund**. If you bought as a guest, or cannot use that option, email ' +
          `${COMPANY.supportEmail} with your booking reference (it starts with BK-).`,
      },
    ],
  },
  {
    heading: 'Tickets: if the event is cancelled or changed',
    blocks: [
      {
        kind: 'p',
        text:
          'If an event is cancelled entirely, you receive a full refund of the ticket face value **and** the service fee. You do not need to apply — cancellations are refunded automatically and we email you when the refund is processed.',
      },
      {
        kind: 'p',
        text:
          'If an event is postponed or materially changed (for example a new date, a new venue, or the headline act no longer appearing), you may keep your ticket or request that same full refund.',
      },
    ],
  },
  {
    heading: 'Tickets: cooling-off',
    blocks: [
      {
        kind: 'p',
        text:
          'In accordance with Section 42(2)(j) of the Electronic Communications and Transactions Act (ECTA), the 7-day cooling-off period for online transactions does not apply to event tickets, as these are leisure services provided on a specific date. Your rights to cancel a ticket are those set out above.',
      },
    ],
  },
  {
    heading: 'Merchandise: changing your mind',
    blocks: [
      {
        kind: 'p',
        text:
          'Under Section 44 of ECTA you may cancel a merchandise order, without giving a reason and without penalty, within 7 days after you receive the goods. The only charge to you is the direct cost of returning the goods.',
      },
      {
        kind: 'p',
        text:
          'Goods must be returned unused, in their original condition and packaging. This right does not apply to items made to your specifications or clearly personalised for you, as provided in Section 42(2)(f) of ECTA.',
      },
    ],
  },
  {
    heading: 'Merchandise: faulty, damaged or wrong items',
    blocks: [
      {
        kind: 'p',
        text:
          'Under Sections 55 and 56 of the Consumer Protection Act, if an item is defective, unsafe or not what you ordered, you may return it within six months of delivery, at our risk and expense, and choose a repair, a replacement or a full refund. This applies in addition to any warranty offered with the item.',
      },
      {
        kind: 'p',
        text:
          'If an item arrives damaged, please tell us as soon as you can — ideally within 48 hours — with a photo of the item and its packaging. This helps us resolve the claim with the courier quickly. It does not reduce your rights under the CPA.',
      },
      {
        kind: 'p',
        text:
          '**Out of stock after payment.** If an item sells out between your payment and your order being confirmed, we refund you in full, including any delivery fee, and tell you by email.',
      },
    ],
  },
  {
    heading: 'Merchandise: how to request a return or refund',
    blocks: [
      {
        kind: 'p',
        text: `Email ${COMPANY.supportEmail} with your order or booking reference, the item, and the reason for the return. We will confirm the return address or collection arrangement. Please do not send goods back before we have confirmed where they should go.`,
      },
    ],
  },
  {
    heading: 'How refunds are paid',
    blocks: [
      {
        kind: 'list',
        items: [
          `Refunds are paid in ${COMPANY.currencyName} (${COMPANY.currencyCode}) to the card or account you paid with. We cannot pay a refund to a different card, account or person.`,
          'Once a refund is approved we process it through our payment provider, Paystack. Your bank may take several further business days to show it on your statement.',
          'Where a refund is due under ECTA after a merchandise cancellation, we pay it within 30 days of the cancellation, as Section 44(3) requires.',
        ],
      },
    ],
  },
];

export const DELIVERY_SECTIONS: Section[] = [
  {
    heading: 'Tickets are delivered electronically',
    blocks: [
      {
        kind: 'p',
        text:
          'TicketHawk tickets are digital. We do not post or courier physical tickets, and there is no delivery fee on a ticket.',
      },
      {
        kind: 'list',
        items: [
          'Your tickets are issued as soon as your payment is confirmed, normally within a few minutes.',
          'They appear under **Tickets** on the website and in the TicketHawk app, and we email them to the address on your booking.',
          'If you assign a ticket to someone else, we email them a link to claim it on their own phone.',
          'At the venue, show the live QR code on your ticket. It refreshes every few seconds, so a screenshot will not scan.',
        ],
      },
      {
        kind: 'p',
        text: `If you have not received your tickets within one hour of paying, check your spam or promotions folder, then email ${COMPANY.supportEmail} with your booking reference.`,
      },
    ],
  },
  {
    heading: 'Merchandise: delivery or collection',
    blocks: [
      {
        kind: 'p',
        text:
          'Event merchandise is supplied by the Organizer of the related event, or by us where we list an item ourselves. Each item shows which options are available before you buy:',
      },
      {
        kind: 'list',
        items: [
          '**Delivery** — sent by courier to the address you enter at checkout.',
          '**Collection** — collected in person at the location shown on the item, which is often the event itself.',
        ],
      },
    ],
  },
  {
    heading: 'Where we deliver',
    blocks: [
      {
        kind: 'p',
        text: `We deliver to street addresses in ${COMPANY.countryOfDomicile} only. We do not ship internationally.`,
      },
    ],
  },
  {
    heading: 'Delivery fees',
    blocks: [
      {
        kind: 'p',
        text: `Where a delivery fee applies, it is shown in ${COMPANY.currencyName} on the item and in your cart, and included in the total before you pay. There are no further charges on delivery.`,
      },
    ],
  },
  {
    heading: 'Delivery times',
    blocks: [
      {
        kind: 'list',
        items: [
          `Orders are dispatched within ${MERCH_DELIVERY.dispatchWithin} of payment, unless the item says it will be released on a later date, in which case it is dispatched from that date.`,
          `Courier delivery then normally takes ${MERCH_DELIVERY.courierTransit}, depending on your location.`,
          `If your order has not been delivered within ${MERCH_DELIVERY.outerLimitDays} days of payment (or of the stated release date), you may cancel it and receive a full refund, including the delivery fee, as provided in Section 46 of ECTA.`,
        ],
      },
    ],
  },
  {
    heading: 'Receiving your order',
    blocks: [
      {
        kind: 'list',
        items: [
          'Please make sure your delivery address is complete and correct. If a parcel cannot be delivered because the address was wrong or incomplete, a further delivery fee may apply to send it again.',
          'Ownership and risk in the goods pass to you on delivery.',
          'Check your parcel when it arrives. If anything is damaged or missing, see our Refund and Cancellation Policy.',
        ],
      },
    ],
  },
  {
    heading: 'Collecting your order',
    blocks: [
      {
        kind: 'p',
        text:
          'Bring your order confirmation (on your phone is fine) to the collection point at the time stated on the item. The person collecting may be asked for ID matching the name on the order. If you cannot collect at the stated time, email us before the collection date so we can try to arrange an alternative with the Organizer.',
      },
    ],
  },
];

/**
 * Flattens a policy's sections into the blocks of a single Terms clause, each
 * sub-heading becoming a bold lead-in line, so the policy reads as one
 * numbered clause inside the Terms rather than adding a dozen top-level
 * headings to it.
 */
export function asClauseBlocks(sections: Section[]): Block[] {
  return sections.flatMap((s) => [{ kind: 'p', text: `**${s.heading}**` } as Block, ...s.blocks]);
}