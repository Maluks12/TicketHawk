// ── /robots.txt ───────────────────────────────────────────────────────────────
// There was no robots.txt and no sitemap on the public site at all. That is the
// most likely single reason it's hard to find: Google had no map of the site and
// was left to discover pages by following whatever links it happened to land on.
//
// Written as an explicit Route Handler rather than Next's `app/robots.ts`
// metadata convention — deliberately matching events-organizer, where that
// convention crashed the production build on Next 16 + Turbopack ("Cannot read
// properties of undefined (reading 'userAgent')"). web-desktop is on the same
// major, so the same trap applies. Emitting the text ourselves removes the
// dependency on Next's serializer, and crawlers can't tell the difference.
//
// force-static: prerendered once at build time, cached like any other asset.
export const dynamic = 'force-static';

const SITE_URL = 'https://ticket-hawk.com';

export function GET() {
  const body = [
    'User-agent: *',
    'Allow: /',
    '',
    // Nothing here is secret. These are pages with no search value that would
    // burn crawl budget and can surface as thin results:
    //   /account /profile /tickets  — signed-in only, empty to a crawler
    //   /cart /booking /payment     — transient checkout state
    //   /auth /reset-password       — auth plumbing
    //   /claim-ticket               — one-time token links
    'Disallow: /account',
    'Disallow: /profile',
    'Disallow: /tickets',
    'Disallow: /cart',
    'Disallow: /booking',
    'Disallow: /payment',
    'Disallow: /auth',
    'Disallow: /reset-password',
    'Disallow: /claim-ticket',
    'Disallow: /api',
    '',
    `Sitemap: ${SITE_URL}/sitemap.xml`,
    '',
  ].join('\n');

  return new Response(body, {
    headers: {
      'Content-Type': 'text/plain; charset=utf-8',
      'Cache-Control': 'public, max-age=3600',
    },
  });
}