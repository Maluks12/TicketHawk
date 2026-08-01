// ─────────────────────────────────────────────────────────────────────────────
// CoverImage — expo-image wrapper that loads the small card thumbnail and falls
// back to the full image if the thumbnail is missing. Use on list/feed cards.
// (The event detail screen should use the full image directly, not this.)
// ─────────────────────────────────────────────────────────────────────────────
import React, { useState } from 'react';
import { StyleProp } from 'react-native';
import { Image, ImageStyle } from 'expo-image';
import { thumbUrl, IMG_PLACEHOLDER } from '../lib/img';

type Fit = 'cover' | 'contain' | 'fill' | 'none' | 'scale-down';

interface Props {
  uri?: string | null;
  style?: StyleProp<ImageStyle>;
  contentFit?: Fit;
  priority?: 'low' | 'normal' | 'high';
  recyclingKey?: string | null;
  transition?: number;
}

export function CoverImage({
  uri,
  style,
  contentFit = 'cover',
  priority = 'normal',
  recyclingKey,
  transition = 160,
}: Props) {
  // Track the URI whose thumbnail failed, so recycling to a new item retries.
  const [erroredUri, setErroredUri] = useState<string | null>(null);

  const thumb = thumbUrl(uri);
  const useFull = !thumb || erroredUri === uri;
  const src = useFull ? uri : thumb;
  if (!src) return null;

  return (
    <Image
      source={{ uri: src }}
      style={style}
      contentFit={contentFit}
      cachePolicy="memory-disk"
      // Object form — see the note in lib/img.ts. A bare string here is read as
      // a URI and quietly renders nothing.
      placeholder={IMG_PLACEHOLDER}
      placeholderContentFit="cover"
      priority={priority}
      transition={transition}
      // Decode at the size actually drawn rather than at the source's native
      // resolution. Card thumbs are ~118×68pt; decoding a 1200px cover into
      // that box is wasted CPU on the UI thread and is a large part of why a
      // list of covers feels slow to appear on mid-range Android.
      allowDownscaling
      recyclingKey={recyclingKey ?? uri ?? undefined}
      onError={() => {
        if (uri && erroredUri !== uri) setErroredUri(uri);
      }}
    />
  );
}