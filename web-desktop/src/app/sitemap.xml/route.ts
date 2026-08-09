// ── /sitemap.xml ──────────────────────────────────────────────────────────────
// Explicit Route Handler for the same reason as robots.txt: the `app/sitemap.ts`
// metadata convention is what broke the Next 16 production build in
// events-organizer, and web-desktop is on the same major.
//
// Unlike the organiser app's sitemap — which lists one static page, because
// everything else there is behind auth — this one is DYNAMIC. The valuable URLs
// on the public site are the events, they're created by other people daily, and
// they expire. Without them in a sitemap, Google finds a new listing by
// stumbling across a link, which for a short-notice event means finding it after
// it has already happened.

// Revalidate hourly rather than force-static: events go on sale and sell out
// fast enough that a build-time snapshot is stale before anyone reads it.
export const revalidate = 3600;

const SITE_URL = 'https://ticket-hawk.com';
const API = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080';

interface ApiEvent {
  id: string;
  slug?: string | null;
  start_time: string;
  updated_at?: string | null;
  status?: string | null;
}

/**
 * Pulls published events.
 *
 * Failure is non-fatal — an empty list still yields a valid sitemap of the
 * static routes. A sitemap that 500s is worse than a short one: Google backs
 * off from a URL that errors, and you lose the static pages too.
 */
async function fetchEvents(): Promise<ApiEvent[]> {
  try {
    // 1000 is a deliberate ceiling. The sitemap spec allows 50 000 URLs, but a
    // single huge response is a slow path crawlers time out on; past this size
    // the right answer is a sitemap index, not a bigger page.
    const res = await fetch(`${API}/v1/events?limit=1000`, {
      next: { revalidate: 3600 },
    });
    if (!res.ok) return [];
    const json = await res.json();
    const list = json?.data ?? json?.events ?? json;
    return Array.isArray(list) ? list : [];
  } catch {
    return [];
  }
}

function urlTag(
  loc: string, lastmod: string, changefreq: string, priority: string,
): string {
  return `  <url>
    <loc>${escapeXml(loc)}</loc>
    <lastmod>${lastmod}</lastmod>
    <changefreq>${changefreq}</changefreq>
    <priority>${priority}</priority>
  </url>`;
}

// Slugs are organiser-supplied, so they reach this file as untrusted input. An
// unescaped & or < produces a malformed document and Google rejects the WHOLE
// sitemap, not just the offending entry.
function escapeXml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

export async function GET() {
  const now = new Date();
  const iso = now.toISOString();

  const staticUrls = [
    urlTag(SITE_URL,                 iso, 'hourly',  '1.0'),
    urlTag(`${SITE_URL}/search`,     iso, 'hourly',  '0.8'),
    urlTag(`${SITE_URL}/category`,   iso, 'daily',   '0.7'),
    urlTag(`${SITE_URL}/merch`,      iso, 'daily',   '0.5'),
    urlTag(`${SITE_URL}/support`,    iso, 'monthly', '0.3'),
    urlTag(`${SITE_URL}/terms`,      iso, 'yearly',  '0.2'),
    urlTag(`${SITE_URL}/privacy`,    iso, 'yearly',  '0.2'),
    urlTag(`${SITE_URL}/legal`,      iso, 'yearly',  '0.2'),
  ];

  const events = await fetchEvents();
  const eventUrls = events
    .filter(e => !e.status || e.status === 'published')
    .map(e => {
      // Slug when present — the same URL the event layout declares canonical.
      // Listing the UUID here while canonicalising the slug hands Google two
      // contradictory signals about one page.
      const path = e.slug || e.id;
      const past = new Date(e.start_time) < now;
      const lastmod = e.updated_at ? new Date(e.updated_at).toISOString() : iso;
      return urlTag(
        `${SITE_URL}/events/${path}`,
        lastmod,
        // An upcoming event's page changes as tiers sell out; a past one is
        // frozen. Asking Google to keep re-crawling finished events spends
        // crawl budget on the ones that can no longer earn a sale.
        past ? 'yearly' : 'daily',
        past ? '0.3' : '0.9',
      );
    });

  const body = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${[...staticUrls, ...eventUrls].join('\n')}
</urlset>
`;

  return new Response(body, {
    headers: {
      'Content-Type': 'application/xml; charset=utf-8',
      'Cache-Control': 'public, max-age=3600',
    },
  });
}