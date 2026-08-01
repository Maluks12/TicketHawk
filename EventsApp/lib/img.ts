// ─────────────────────────────────────────────────────────────────────────────
// Image speed helpers (thumbnail variants)
//
// The backend stores a card-sized thumbnail next to each cover/banner at a
// deterministic path: the same key with the extension replaced by "_thumb.jpg".
// We derive that URL on the client from the existing cover/banner URL — no new
// API fields or queries needed. Cards request the small thumbnail; the event
// detail screen keeps the full-resolution image.
//
// If a thumbnail doesn't exist yet (e.g. a rare WebP upload, or images not yet
// backfilled), <CoverImage> falls back to the full URL on error, so nothing
// ever shows as broken.
// ─────────────────────────────────────────────────────────────────────────────
import { Image } from 'expo-image';

// Neutral blur-up placeholder so cards never flash blank while loading.
export const IMG_BLURHASH = 'L6PZfSi_.AyE_3t7t7R**0o#DgR4';

// IMPORTANT: expo-image resolves a *string* placeholder as `{ uri: string }`.
// Passing IMG_BLURHASH directly (as this app did everywhere) therefore asked it
// to fetch a URL called "L6PZfSi_…", which fails silently — which is why cards
// sat as flat grey rectangles instead of blurring up. A blurhash has to be
// handed over in the object form below. Always use IMG_PLACEHOLDER in the
// `placeholder` prop; IMG_BLURHASH stays exported only as the raw hash.
export const IMG_PLACEHOLDER = { blurhash: IMG_BLURHASH } as const;

function isRemote(u: string): boolean {
  return /^https?:\/\//i.test(u);
}

/**
 * Derive the card thumbnail URL from a full image URL.
 * `https://host/events/<id>/covers/<uuid>.jpg`
 *   → `https://host/events/<id>/covers/<uuid>_thumb.jpg`
 * Returns undefined when no thumbnail can be derived (non-remote / no extension).
 */
export function thumbUrl(url?: string | null): string | undefined {
  if (!url || !isRemote(url)) return undefined;
  const [path] = url.split(/[?#]/);
  const dot = path.lastIndexOf('.');
  const slash = path.lastIndexOf('/');
  if (dot <= slash) return undefined; // no real extension to replace
  return `${path.slice(0, dot)}_thumb.jpg`;
}

/**
 * Warm the disk cache for thumbnails about to scroll into view, so they render
 * instantly instead of starting to download on first sight. Best-effort.
 */
export function prefetchImages(urls: (string | null | undefined)[]): Promise<void> {
  const seen = new Set<string>();
  const list = urls
    .map((u) => thumbUrl(u) ?? (u || undefined))
    .filter((u): u is string => {
      if (!u || seen.has(u)) return false;
      seen.add(u);
      return true;
    });
  if (list.length === 0) return Promise.resolve();
  // Returned (rather than fire-and-forget) so callers that want to hold a
  // loading state until the first screenful is warm can await it. Existing
  // fire-and-forget callers are unaffected — an un-awaited promise with a
  // catch attached behaves exactly as before.
  return Image.prefetch(list, 'memory-disk').then(() => {}).catch(() => {});
}

/**
 * Warm the disk cache for a FULL-resolution image (hero / banner artwork), as
 * opposed to prefetchImages() above which deliberately warms the small card
 * thumbnail. Use this when the screen you're heading to renders the full URL --
 * prefetching the thumbnail would leave that render a cache miss.
 */
export function prefetchFullImages(urls: (string | null | undefined)[]): void {
  const list = urls.filter((u): u is string => !!u && isRemote(u));
  if (list.length === 0) return;
  Image.prefetch(list, 'memory-disk').catch(() => {});
}