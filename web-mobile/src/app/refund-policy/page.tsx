import type { Metadata } from 'next';
import LegalDoc from '@/components/legal-doc';
import { REFUND_SECTIONS } from '@/lib/policies';

// ─────────────────────────────────────────────────────────────────────────────
// /refund-policy — Refund and Cancellation Policy, as a standalone page.
//
// The same content is clause 7 of /terms. Both render from REFUND_SECTIONS in
// lib/policies.ts — edit it there, never here.
// ─────────────────────────────────────────────────────────────────────────────

export const metadata: Metadata = {
  title: 'Refund and Cancellation Policy | TicketHawk',
  description: 'How ticket and merchandise refunds, cancellations and returns work on TicketHawk.',
};

export default function RefundPolicyPage() {
  return (
    <LegalDoc
      title="Refund and Cancellation Policy"
      intro="How cancellations, refunds and returns work for tickets and merchandise bought on TicketHawk. This policy forms part of our Terms of Use (clause 7), and nothing in it limits your rights under the Consumer Protection Act or the Electronic Communications and Transactions Act."
      sections={REFUND_SECTIONS}
      seeAlso={[
        { href: '/terms', label: 'Terms of Use' },
        { href: '/delivery-policy', label: 'Delivery Policy' },
        { href: '/support', label: 'Support' },
      ]}
    />
  );
}