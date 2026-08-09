'use client';

// ─────────────────────────────────────────────────────────────────────────────
// CoverImage — img wrapper that loads the small card thumbnail and falls back
// to the full image if the thumbnail is missing or errors. Use on list/feed
// cards (home sections, category grids, search results). The event detail
// page should keep using the full image directly, not this.
//
// Ported from EventsApp/components/CoverImage.tsx.
// ─────────────────────────────────────────────────────────────────────────────

import { useState } from 'react';
import { thumbUrl } from '@/lib/img';

interface Props {
  uri?: string | null;
  alt: string;
  className?: string;
}

export function CoverImage({ uri, alt, className }: Props) {
  // Track the URI whose thumbnail failed, so changing items retries cleanly.
  const [erroredUri, setErroredUri] = useState<string | null>(null);
  // Track a full-image failure too: render nothing (the card's background
  // shows) instead of the browser's broken-image icon.
  const [deadUri, setDeadUri] = useState<string | null>(null);

  const thumb = thumbUrl(uri);
  const useFull = !thumb || erroredUri === uri;
  const src = useFull ? uri : thumb;
  if (!src || deadUri === uri) return null;

  return (
    <img
      src={src}
      alt={alt}
      loading="lazy"
      className={className}
      onError={() => {
        if (!uri) return;
        if (!useFull && erroredUri !== uri) setErroredUri(uri); // thumb failed → try full
        else if (useFull && deadUri !== uri) setDeadUri(uri);   // full failed → give up quietly
      }}
    />
  );
}