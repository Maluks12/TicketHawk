'use client';

import LegalDoc, { type Section } from '@/components/legal-doc';
import { COMPANY } from '@/lib/legal';

// ─────────────────────────────────────────────────────────────────────────────
// /support — the address given to Apple and Google as the app's Support URL.
//
// Both stores require one and both actually open it during review; a 404 or a
// bare homepage is a rejection under App Store Guideline 1.5. The bar is that a
// user who has a problem can work out how to get help, so the page leads with a
// working email address and answers the questions support would otherwise field
// one at a time.
//
// Reuses LegalDoc rather than hand-rolling a layout. Not laziness — this page
// sits alongside /terms, /privacy and /legal in the footer, and a support page
// that looks nothing like them reads as an afterthought bolted on for the
// stores, which is exactly the impression to avoid in front of a reviewer.
//
// The answers below must stay true to what the apps actually do. Every one of
// them describes a flow a reviewer can follow and check, so a stale answer here
// is worse than no answer at all:
//   • refunds        — sliding scale, requested per booking
//   • deletion       — Profile > Delete account (App Store Guideline 5.1.1(v))
//   • ticket sharing — per-slot assignment by email
//   • organisers     — identity verification gates payouts, not sign-up
// ─────────────────────────────────────────────────────────────────────────────

const sections: Section[] = [
  {
    heading: 'Contact us',
    blocks: [
      {
        kind: 'p',
        text:
          `The fastest way to reach us is email. Write to **${COMPANY.supportEmail}** and we will come back to you within one business day.`,
      },
      {
        kind: 'p',
        text:
          'If your event is today and you cannot get in, put **URGENT** and the event name in the subject line so we see it first.',
      },
      {
        kind: 'p',
        text:
          'It helps enormously if you include your booking reference (it starts with BK-), the email address you booked with, and the name of the event.',
      },
    ],
  },
  {
    heading: 'Tickets and entry',
    blocks: [
      {
        kind: 'callout',
        title: 'Where are my tickets?',
        blocks: [
          {
            kind: 'p',
            text:
              'Open the **Tickets** tab in the TicketHawk app. Every ticket you hold is there, whether you bought it or someone sent it to you.',
          },
          {
            kind: 'p',
            text:
              'We also email your tickets to the address on the booking. If you cannot find that email, check your spam or promotions folder before contacting us — it is the most common cause.',
          },
        ],
      },
      {
        kind: 'p',
        text:
          '**Why does my QR code keep changing?** That is deliberate. Your pass is a live code that refreshes every few seconds, which makes a screenshot useless to anyone else. Just show the screen at the gate.',
      },
      {
        kind: 'p',
        text:
          '**What if there is no signal at the venue?** Your pass is stored on your phone when you first open it, so it still scans without a connection. Open your ticket once before you travel and you are covered.',
      },
      {
        kind: 'p',
        text:
          '**Can I send tickets to friends?** Yes. Open the booking and assign a ticket to each person by email. They claim it on their own phone and can walk in separately — nobody has to wait at the gate for the person who paid.',
      },
    ],
  },
  {
    heading: 'Payments and refunds',
    blocks: [
      {
        kind: 'p',
        text:
          '**How do I get a refund?** Open the booking in the app and choose **Request refund**. What you get back depends on how close the event is — the full sliding scale is set out in the Terms of Use.',
      },
      {
        kind: 'p',
        text:
          '**The event was cancelled.** You do not need to do anything. Cancellations are refunded automatically and we email you when it is processed.',
      },
      {
        kind: 'p',
        text:
          '**I was charged but got no ticket.** Email us with the booking reference and the date. This is usually a payment that confirmed after a dropped connection, and we can resolve it from our side.',
      },
      {
        kind: 'p',
        text:
          'Payments are processed by Paystack. We never see or store your full card number.',
      },
    ],
  },
  {
    heading: 'Your account',
    blocks: [
      {
        kind: 'p',
        text:
          '**I cannot sign in.** Use **Forgot password** on the sign-in screen. If the reset email does not arrive within a few minutes, check your spam folder, then email us.',
      },
      {
        kind: 'callout',
        title: 'How do I delete my account?',
        blocks: [
          {
            kind: 'p',
            text:
              'In the app, go to **Profile > Delete account**. You will be asked for your password to confirm. You can also delete your account on the web at **ticket-hawk.com/account/delete** — you do not need to reinstall the app.',
          },
          {
            kind: 'p',
            text:
              'Your name, email address and phone number are permanently removed. We keep a record of past ticket purchases with your personal details stripped out, because South African tax and consumer protection law requires us to be able to prove a sale took place, and because event organisers need it for their own settlement records.',
          },
          {
            kind: 'p',
            text:
              'If you still hold tickets to an event that has not happened yet, request a refund first — deleting your account does not refund them.',
          },
        ],
      },
      {
        kind: 'p',
        text:
          '**How do I change my email address?** Profile > Edit profile. We will ask for your current password, and we notify both your old and new address once the change is made.',
      },
    ],
  },
  {
    heading: 'Running an event',
    blocks: [
      {
        kind: 'p',
        text:
          'Download **TicketHawk Studio**, our free companion app for organisers and door staff, or create an organiser account on the web.',
      },
      {
        kind: 'p',
        text:
          'Anyone can sign up. Before we can pay out ticket revenue we verify your South African ID against the Department of Home Affairs and confirm the bank account you nominate belongs to you — this is required under FICA and applies to every organiser.',
      },
      {
        kind: 'p',
        text:
          `Questions about listing an event, fees or payouts: **${COMPANY.supportEmail}**.`,
      },
      {
        kind: 'p',
        text:
          '**Working the door?** You do not need an organiser account. Ask the organiser for a join code, enter it in TicketHawk Studio, and you can scan tickets for that event.',
      },
    ],
  },
  {
    heading: 'Reporting a problem',
    blocks: [
      {
        kind: 'p',
        text:
          `**Security issue?** Please report it to **${COMPANY.legalEmail}** rather than posting publicly. Our Vulnerability Disclosure Policy is on the Legal page.`,
      },
      {
        kind: 'p',
        text:
          `**Someone is using your content without permission?** Send a take-down notice to **${COMPANY.takedownEmail}**. The procedure is set out on the Legal page.`,
      },
    ],
  },
  {
    heading: 'Company details',
    blocks: [
      {
        kind: 'p',
        text:
          `${COMPANY.tradingName} is operated by **${COMPANY.legalName}**, registration number ${COMPANY.registrationNumber}.`,
      },
      {
        kind: 'p',
        text: `Registered address: ${COMPANY.physicalAddress}, South Africa.`,
      },
    ],
  },
];

export default function SupportPage() {
  return (
    <LegalDoc
      title="Support"
      intro="Help with tickets, refunds, your account and running an event on TicketHawk."
      sections={sections}
      seeAlso={[
        { href: '/terms', label: 'Terms of Use' },
        { href: '/privacy', label: 'Privacy Policy' },
        { href: '/legal', label: 'Legal' },
      ]}
    />
  );
}