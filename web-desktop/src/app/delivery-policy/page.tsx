import type { Metadata } from 'next';
import LegalDoc from '@/components/legal-doc';
import { DELIVERY_SECTIONS } from '@/lib/policies';

// ─────────────────────────────────────────────────────────────────────────────
// /delivery-policy — Delivery Policy, as a standalone page.
//
// The same content is clause 8 of /terms. Both render from DELIVERY_SECTIONS
// in lib/policies.ts — edit it there, never here.
// ─────────────────────────────────────────────────────────────────────────────

export const metadata: Metadata = {
  title: 'Delivery Policy | TicketHawk',
  description: 'How TicketHawk delivers tickets and event merchandise.',
};

export default function DeliveryPolicyPage() {
  return (
    <LegalDoc
      title="Delivery Policy"
      intro="How you receive what you buy on TicketHawk: tickets are delivered electronically, and merchandise is delivered by courier within South Africa or collected in person. This policy forms part of our Terms of Use (clause 8)."
      sections={DELIVERY_SECTIONS}
      seeAlso={[
        { href: '/terms', label: 'Terms of Use' },
        { href: '/refund-policy', label: 'Refund Policy' },
        { href: '/support', label: 'Support' },
      ]}
    />
  );
}