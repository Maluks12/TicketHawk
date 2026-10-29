// ── Event collections: display helpers ───────────────────────────────────────
//
// The home feed folds a collection's dates into ONE event that carries a
// `collection` summary (see Event.collection in lib/api.ts). These helpers let
// every existing card component render that event as the collection without
// a separate card type: same layout, the collection's cover / title / price,
// a date RANGE instead of one date, and a badge saying it is a collection.
//
// Same helpers as lib/eventCollections.ts in web-desktop and web-mobile.
import type { Event, EventCollectionRef } from './api';

/** Where a collection card leads. */
export const collectionHref = (c: Pick<EventCollectionRef, 'slug'>) =>
  `/collection/${encodeURIComponent(c.slug)}`;

/**
 * The event as a card should show it. For a collection: its cover (falling
 * back to the standing-in date's own cover), its title, the cheapest date's
 * price, and the cities in place of a venue. Anything else passes through.
 */
export function asCardEvent(e: Event): Event {
  const c = e.collection;
  if (!c) return e;
  return {
    ...e,
    title: c.title,
    cover_image_url: c.cover_image_url || e.cover_image_url,
    price: c.min_price,
    start_time: c.first_start,
    venue_name: collectionCitiesLine(c),
    city: '',
  };
}

const short = (iso: string, withYear: boolean) =>
  new Date(iso).toLocaleDateString('en-ZA', {
    day: 'numeric', month: 'short', ...(withYear ? { year: 'numeric' } : {}),
  });

/** "21 Nov – 23 Dec 2026", or one date when they coincide. */
export function collectionDateRange(c: Pick<EventCollectionRef, 'first_start' | 'last_start'>): string {
  if (!c.first_start) return '';
  const a = new Date(c.first_start);
  const b = new Date(c.last_start || c.first_start);
  if (a.toDateString() === b.toDateString()) return short(c.first_start, true);
  const sameYear = a.getFullYear() === b.getFullYear();
  return `${short(c.first_start, !sameYear)} – ${short(c.last_start, true)}`;
}

/** "East London · Cape Town · +4 more" — at most `max` names. */
export function collectionCitiesLine(c: Pick<EventCollectionRef, 'cities'>, max = 2): string {
  const cities = (c.cities ?? []).filter(Boolean);
  if (cities.length <= max) return cities.join(' · ');
  return `${cities.slice(0, max).join(' · ')} · +${cities.length - max} more`;
}

/** Badge text: "12 DATES". */
export const collectionCountLabel = (c: Pick<EventCollectionRef, 'event_count'>) =>
  `${c.event_count} ${c.event_count === 1 ? 'DATE' : 'DATES'}`;