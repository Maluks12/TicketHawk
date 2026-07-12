import React from 'react';
import { Image } from 'expo-image';

// TicketHawk brand mark. Rendered from a pre-rasterized 1024px PNG of the
// vector-traced logo rather than inline SVG: react-native-svg mishandled the
// traced paths' winding rules (dark layers rendered inverted/white), while a
// PNG is renderer-proof and pixel-identical to the master artwork.
// 1024px source stays crisp at any in-app size; transparent background works
// on both light and dark surfaces (the badge carries its own gold circle).
export default function BrandLogo({ size = 84 }: { size?: number }) {
  return (
    <Image
      source={require('../assets/brand-logo.png')}
      style={{ width: size, height: size }}
      contentFit="contain"
      cachePolicy="memory-disk"
    />
  );
}