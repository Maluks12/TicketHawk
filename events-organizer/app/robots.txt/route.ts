import { SITE_URL } from '../site';

// ── /robots.txt ───────────────────────────────────────────────────────────────
// Written as an explicit Route Handler rather than Next's `app/robots.ts`
// metadata convention. The metadata version crashed the production build on
// Next 16.2.6 + Turbopack ("Cannot read properties of undefined (reading
// 'userAgent')") while prerendering /robots.txt. Emitting the text ourselves
// removes that dependency on Next's internal serializer entirely, and the
// output is identical as far as crawlers are concerned.
//
// force-static: prerendered once at build time into a static file, so there's
// no per-request work and it caches like any other asset.
export const dynamic = 'force-static';

export function GET() {
  const body = [
    'User-agent: *',
    'Allow: /',
    // Signed-in areas: gated, no SEO value, and we don't want them in an index.
    'Disallow: /dashboard',
    'Disallow: /onboarding',
    'Disallow: /auth',
    'Disallow: /api',
    'Disallow: /kyc',
    'Disallow: /events',
    'Disallow: /event',
    'Disallow: /campaigns',
    'Disallow: /merch',
    'Disallow: /payouts',
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