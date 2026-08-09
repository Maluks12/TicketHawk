// NOTE: byte-identical to the web-desktop copy, on purpose.
//
// web-desktop and web-mobile are two separate Next apps serving ONE domain,
// split by device upstream. Which app answers a given request depends on the
// user agent — and Googlebot crawls mobile-first, so the crawler that matters
// most is the one routed HERE. Shipping these only to web-desktop would leave
// them unreachable to the exact bot they exist for.
//
// Same content either way, so whichever app answers, Google gets the same
// answer. Keep the two copies in sync.

import type { Metadata } from 'next';
import type { ReactNode } from 'react';

// ─────────────────────────────────────────────────────────────────────────────
// Per-event SEO
//
// THE PROBLEM THIS SOLVES
//
// events/[id]/page.tsx is a client component that fetches in useEffect. Next
// still server-renders the shell, but the DATA arrives after hydration — so the
// HTML a crawler receives has no event title, no date, no venue and no
// description. Every event page was, to Google, an empty page called
// "TicketHawk".
//
// A LAYOUT is the fix that doesn't require rewriting the page. Layouts are
// server components even when the page inside them is a client component, so
// this file can fetch the event, emit real <title>/<meta> tags and inject
// structured data — while page.tsx carries on exactly as it is.
//
// WHY Event STRUCTURED DATA MATTERS MOST HERE
//
// For a ticketing site this is the single biggest lever. schema.org/Event is
// what puts an event into Google's event experience — the date/venue/ticket
// cards in search and on Google Maps — and that surface is where people
// actually look for things to go to. Title tags help you rank for your own
// name; Event markup gets you in front of someone searching "things to do in
// Johannesburg this weekend".
// ─────────────────────────────────────────────────────────────────────────────

const API = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080';
const SITE = 'https://ticket-hawk.com';

interface ApiEvent {
  id: string;
  slug?: string | null;
  title: string;
  description?: string | null;
  start_time: string;
  end_time?: string | null;
  venue_name?: string | null;
  address?: string | null;
  city?: string | null;
  cover_image_url?: string | null;
  banner_image_url?: string | null;
  price?: number | null;
  currency?: string | null;
  status?: string | null;
  is_free?: boolean | null;
  organizer_name?: string | null;
}

/**
 * Fetches an event for metadata purposes.
 *
 * Deliberately forgiving: metadata generation must never throw. A failure here
 * would 500 a page that would otherwise render fine for a human, trading a
 * working page for a missing title. Returns null and the caller falls back to
 * generic metadata.
 *
 * `next.revalidate` caches for 5 minutes. Event details change rarely, and
 * without it every crawl of a popular event hits the API.
 */
async function fetchEvent(id: string): Promise<ApiEvent | null> {
  try {
    const res = await fetch(`${API}/v1/events/${encodeURIComponent(id)}`, {
      next: { revalidate: 300 },
    });
    if (!res.ok) return null;
    const json = await res.json();
    return (json?.data ?? json) as ApiEvent;
  } catch {
    return null;
  }
}

/** Trims to a length search engines actually display, without cutting mid-word. */
function clamp(text: string, max: number): string {
  const flat = text.replace(/\s+/g, ' ').trim();
  if (flat.length <= max) return flat;
  const cut = flat.slice(0, max);
  const lastSpace = cut.lastIndexOf(' ');
  return `${cut.slice(0, lastSpace > 40 ? lastSpace : max)}…`;
}

export async function generateMetadata(
  { params }: { params: Promise<{ id: string }> },
): Promise<Metadata> {
  const { id } = await params;
  const event = await fetchEvent(id);

  if (!event) {
    return { title: 'Event · TicketHawk' };
  }

  const when = new Date(event.start_time).toLocaleDateString('en-ZA', {
    weekday: 'long', day: 'numeric', month: 'long', year: 'numeric',
    timeZone: 'Africa/Johannesburg',
  });
  const where = [event.venue_name, event.city].filter(Boolean).join(', ');

  // Title carries the two things someone scanning results needs: when and
  // where. "Summer Festival · TicketHawk" tells them nothing they can act on.
  const title = where
    ? `${event.title} — ${where} | ${when}`
    : `${event.title} | ${when}`;

  const description = clamp(
    event.description?.trim()
      ? event.description
      : `${event.title}${where ? ` at ${where}` : ''} on ${when}. Book tickets on TicketHawk.`,
    160,
  );

  const image = event.banner_image_url || event.cover_image_url || '/icon.png';
  // Canonical to the SLUG when there is one. Without this, /events/<uuid> and
  // /events/<slug> are two URLs serving identical content, and Google splits
  // the ranking signal between them.
  const canonical = `${SITE}/events/${event.slug || event.id}`;

  return {
    title,
    description,
    alternates: { canonical },
    openGraph: {
      type: 'website',
      siteName: 'TicketHawk',
      title,
      description,
      url: canonical,
      images: [{ url: image }],
    },
    twitter: {
      card: 'summary_large_image',
      title,
      description,
      images: [image],
    },
    // Draft, cancelled and pending events must not be indexed — a cancelled
    // event ranking for its own name sends people to a dead page.
    robots: event.status === 'published'
      ? undefined
      : { index: false, follow: false },
  };
}

export default async function EventLayout(
  { children, params }: { children: ReactNode; params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const event = await fetchEvent(id);

  // Only published events get structured data. Marking up a private or
  // cancelled event invites Google to surface something the organiser didn't
  // publish.
  const jsonLd = event && event.status === 'published' ? {
    '@context': 'https://schema.org',
    '@type': 'Event',
    name: event.title,
    description: event.description?.replace(/\s+/g, ' ').trim() || undefined,
    startDate: event.start_time,
    endDate: event.end_time || undefined,
    eventStatus: 'https://schema.org/EventScheduled',
    eventAttendanceMode: 'https://schema.org/OfflineEventAttendanceMode',
    image: [event.banner_image_url, event.cover_image_url].filter(Boolean),
    url: `${SITE}/events/${event.slug || event.id}`,
    location: event.venue_name ? {
      '@type': 'Place',
      name: event.venue_name,
      address: {
        '@type': 'PostalAddress',
        streetAddress: event.address || undefined,
        addressLocality: event.city || undefined,
        addressCountry: 'ZA',
      },
    } : undefined,
    organizer: event.organizer_name ? {
      '@type': 'Organization',
      name: event.organizer_name,
    } : undefined,
    // `offers` is what makes the result show a price and a Buy path. Google
    // treats an Event without offers as materially less complete.
    offers: {
      '@type': 'Offer',
      url: `${SITE}/events/${event.slug || event.id}`,
      price: event.is_free ? 0 : (event.price ?? 0),
      priceCurrency: event.currency || 'ZAR',
      availability: 'https://schema.org/InStock',
      validFrom: undefined,
    },
  } : null;

  return (
    <>
      {jsonLd && (
        <script
          type="application/ld+json"
          // Structured data has to be in the server HTML to be seen — a crawler
          // that doesn't execute JS never sees anything added at runtime.
          dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
        />
      )}
      {children}
    </>
  );
}