// ─────────────────────────────────────────────────────────────────────────────
// Legal document links.
//
// The documents themselves live on the website (web-desktop / web-mobile,
// src/app/terms, /privacy, /legal) rather than being duplicated into the app.
// That is deliberate: legal text changes on legal advice, not on a release
// cycle, and a copy compiled into a shipped binary cannot be corrected without
// an App Store review. Pointing at the canonical web copy means one edit
// updates every surface at once, and the version a buyer agreed to is the
// version actually published.
//
// ── Why WebBrowser and not Linking.openURL ───────────────────────────────────
//
// Linking.openURL hands the user to Safari or Chrome and leaves the app. From
// the checkout screen that is genuinely risky: the buyer taps "Terms of Use"
// to check the refund policy before paying, gets dropped into another app, and
// has to find their way back to a half-finished purchase. iOS may also evict
// the app from memory while it is backgrounded, losing the guest contact
// details they just typed in.
//
// openBrowserAsync presents the page in a dismissible sheet ON TOP of the app.
// The purchase screen stays mounted underneath, and dismissing returns the
// buyer exactly where they were. The same mechanism already drives the hosted
// payment step in lib/payments.ts.
// ─────────────────────────────────────────────────────────────────────────────

import * as WebBrowser from 'expo-web-browser';
import { Linking } from 'react-native';

const WEB_ORIGIN = 'https://ticket-hawk.com';

export const LEGAL_URLS = {
  terms:   `${WEB_ORIGIN}/terms`,
  privacy: `${WEB_ORIGIN}/privacy`,
  legal:   `${WEB_ORIGIN}/legal`,
} as const;

export type LegalDoc = keyof typeof LEGAL_URLS;

/**
 * Opens a legal document in an in-app browser sheet.
 *
 * Falls back to the system browser if the in-app browser is unavailable —
 * failing to open a document a buyer is being asked to agree to is worse than
 * leaving the app to show it.
 */
export async function openLegal(doc: LegalDoc): Promise<void> {
  const url = LEGAL_URLS[doc];
  try {
    await WebBrowser.openBrowserAsync(url, {
      // Match the app's brand rather than the OS default chrome, so the sheet
      // reads as part of TicketHawk instead of an abrupt handoff.
      toolbarColor: '#0e0e12',
      controlsColor: '#FF2D55',
      enableBarCollapsing: true,
    });
  } catch {
    Linking.openURL(url).catch(() => {});
  }
}