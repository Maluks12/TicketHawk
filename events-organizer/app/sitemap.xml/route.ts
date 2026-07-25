import { SITE_URL } from '../site';

// ── /sitemap.xml ──────────────────────────────────────────────────────────────
// Explicit Route Handler for the same reason as robots.txt above: the
// `app/sitemap.ts` metadata convention is what broke the Next 16 production
// build, so both are hand-emitted for predictability.
//
// Only the public landing page belongs here — everything else in this app sits
// behind auth and is disallowed in robots.txt.
export const dynamic = 'force-static';

export function GET() {
  const lastmod = new Date().toISOString();
  const body = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>${SITE_URL}</loc>
    <lastmod>${lastmod}</lastmod>
    <changefreq>weekly</changefreq>
    <priority>1.0</priority>
  </url>
</urlset>
`;

  return new Response(body, {
    headers: {
      'Content-Type': 'application/xml; charset=utf-8',
      'Cache-Control': 'public, max-age=3600',
    },
  });
}