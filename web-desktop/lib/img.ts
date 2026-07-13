// ─────────────────────────────────────────────────────────────────────────────
// Image speed helpers (thumbnail variants)
//
// Ported from EventsApp/lib/img.ts. The backend stores a card-sized thumbnail
// next to each cover/banner at a deterministic path: the same key with the
// extension replaced by "_thumb.jpg". We derive that URL on the client from
// the existing cover/banner URL — no new API fields or queries needed. Cards
// request the small thumbnail; the event detail page keeps the full-res image.
//
// If a thumbnail doesn't exist yet (e.g. a rare WebP upload, or images not
// yet backfilled), <CoverImage> falls back to the full URL on error, so
// nothing ever shows as broken.
// ─────────────────────────────────────────────────────────────────────────────

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