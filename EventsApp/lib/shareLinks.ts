// ─────────────────────────────────────────────────────────────────────────────
// Share links.
//
// A share sheet needs a URL that works for EVERYONE the link is sent to, not
// just people who already have the app. So the canonical share target is always
// the WEB url:
//
//     https://ticket-hawk.com/events/<id>
//
//   • No app installed  → the web event page opens in the browser.
//   • App installed     → iOS Universal Links / Android App Links hand the URL
//                         to the app, which routes it to app/events/[id].tsx,
//                         which redirects to the native event screen.
//
// That is why nothing here emits the `eventsapp://` custom scheme. A custom
// scheme link is dead text in WhatsApp, Instagram or email for anyone without
// the app, which is the majority of the people a ticket link gets forwarded to.
//
// ── Server-side prerequisites for the app to intercept the link ──────────────
// The OS only hands a https:// URL to the app if the domain vouches for it:
//
//   iOS      https://ticket-hawk.com/.well-known/apple-app-site-association
//   Android  https://ticket-hawk.com/.well-known/assetlinks.json
//
// Both must be served over https, with no redirect, as application/json.
// Until they exist the links still work — they just always open the website.
// See app.json (`ios.associatedDomains`, `android.intentFilters`).
// ─────────────────────────────────────────────────────────────────────────────

/** Canonical public web origin. Matches web-mobile/web-desktop metadataBase. */
export const WEB_ORIGIN = 'https://ticket-hawk.com';

interface ShareableEvent {
  id?: string | null;
  slug?: string | null;
  title?: string | null;
  venue_name?: string | null;
  city?: string | null;
  start_time?: string | null;
}

/**
 * Public URL for an event page.
 *
 * Prefers the slug when the backend supplies one — GET /v1/events/:id resolves
 * either, and a slug reads better when the link is pasted into a chat. Falls
 * back to the UUID.
 */
export function eventUrl(event: ShareableEvent | null | undefined, fallbackId?: string): string {
  const key =
    (event?.slug && String(event.slug).trim()) ||
    (event?.id && String(event.id).trim()) ||
    (fallbackId && String(fallbackId).trim()) ||
    '';
  return key ? `${WEB_ORIGIN}/events/${encodeURIComponent(key)}` : WEB_ORIGIN;
}

function fmtShareDate(iso?: string | null): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (isNaN(d.getTime())) return null;
  return d.toLocaleDateString('en-ZA', {
    weekday: 'short', day: 'numeric', month: 'long', year: 'numeric',
  });
}

/**
 * Builds the payload for React Native's `Share.share`.
 *
 * The two platforms treat the arguments differently, which is the whole reason
 * this returns a shaped object instead of a single string:
 *
 *   iOS     honours `url` as a first-class activity item, so the receiving app
 *           renders a rich link preview. The URL must NOT also be inside
 *           `message` or it gets shared twice.
 *   Android ignores `url` entirely — anything not in `message` is dropped, so
 *           the URL has to be appended to the message text there.
 */
export function buildEventShare(
  event: ShareableEvent | null | undefined,
  platformIsIOS: boolean,
  fallbackId?: string,
): { message: string; url?: string; title?: string } {
  const url   = eventUrl(event, fallbackId);
  const title = event?.title?.trim() || 'this event';
  const where = [event?.venue_name, event?.city].filter(Boolean).join(', ');
  const when  = fmtShareDate(event?.start_time);

  const lines = [
    `${title} on TicketHawk`,
    [when, where].filter(Boolean).join(' · '),
  ].filter(Boolean);

  const body = lines.join('\n');

  return platformIsIOS
    ? { message: body, url, title }
    : { message: `${body}\n${url}`, title };
}