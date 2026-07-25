// ── Public URL of THIS organiser dashboard app ───────────────────────────────
// Used for the landing page's <link rel="canonical">, Open Graph URL,
// robots.txt and sitemap.xml.
//
// ⚠️ This is deliberately NOT `NEXT_PUBLIC_SITE_URL`. That variable already
// exists and means something different in this app: it's the CONSUMER site base
// used to build shareable event links (`${SITE_URL}/events/<slug>`) on the event
// detail screen. If the organiser dashboard lives on its own subdomain and you
// reused that var, every share link would start pointing at the dashboard
// instead of the public site. So this gets its own name.
//
// Set it to the address you'd type in a browser to reach this dashboard's login
// screen — no trailing slash. Examples:
//   NEXT_PUBLIC_ORGANISER_URL=https://organiser.ticket-hawk.com
//   NEXT_PUBLIC_ORGANISER_URL=https://ticket-hawk.com/organiser
//
// NOTE: NEXT_PUBLIC_* vars are inlined by Next at BUILD time, so this must be
// passed as a Docker build arg (see Dockerfile), not just a runtime env var.
export const SITE_URL =
  process.env.NEXT_PUBLIC_ORGANISER_URL?.replace(/\/$/, '') || 'https://ticket-hawk.com';