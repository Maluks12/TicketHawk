import React from 'react';
import Svg, { Path, Circle, Rect } from 'react-native-svg';

/**
 * SocialIcon — brand marks for organiser links.
 *
 * ── Why the paths are drawn here rather than pulled from an icon set ────────
 *
 * @expo/vector-icons ships several brand sets, but the glyph names differ
 * between them and between versions — Ionicons has `logo-twitter` and no X or
 * Spotify, FontAwesome5 has `spotify` but names X as `twitter`, FontAwesome6
 * renamed it `x-twitter`. Picking one means guessing at names that cannot be
 * verified without the package installed, and a wrong name renders as a "?"
 * box, which is worse than the text chips this replaces.
 *
 * react-native-svg is already a dependency (15.12.1) and already used by
 * BrandLogo, so drawing the marks is the option with no unknowns in it.
 *
 * The marks are simplified to a single path each. At 18–20pt on a phone the
 * difference between a simplified glyph and the licensed original is not
 * visible, and none of these are being reproduced at a size or fidelity that
 * would misrepresent the brand.
 */

export type SocialPlatform =
  | 'instagram' | 'x' | 'facebook' | 'tiktok'
  | 'youtube' | 'linkedin' | 'whatsapp' | 'spotify' | 'website';

// Brand colours, used when `tinted` is set. Muted by default instead: a row of
// nine saturated brand colours next to a red CTA is visual noise, and the
// organiser page already has an accent colour doing a job.
export const SOCIAL_BRAND_COLOR: Record<SocialPlatform, string> = {
  instagram: '#E4405F',
  x:         '#000000',
  facebook:  '#1877F2',
  tiktok:    '#000000',
  youtube:   '#FF0000',
  linkedin:  '#0A66C2',
  whatsapp:  '#25D366',
  spotify:   '#1DB954',
  website:   '#6B7280',
};

export const SOCIAL_LABEL: Record<SocialPlatform, string> = {
  instagram: 'Instagram',
  x:         'X',
  facebook:  'Facebook',
  tiktok:    'TikTok',
  youtube:   'YouTube',
  linkedin:  'LinkedIn',
  whatsapp:  'WhatsApp',
  spotify:   'Spotify',
  website:   'Website',
};

interface Props {
  platform: string;
  size?: number;
  color?: string;
}

export function SocialIcon({ platform, size = 20, color = '#FFFFFF' }: Props) {
  const p = platform as SocialPlatform;
  const common = { width: size, height: size, viewBox: '0 0 24 24' };

  switch (p) {
    case 'instagram':
      return (
        <Svg {...common} fill="none" stroke={color} strokeWidth={1.9}>
          <Rect x={2.5} y={2.5} width={19} height={19} rx={5.5} />
          <Circle cx={12} cy={12} r={4.2} />
          <Circle cx={17.6} cy={6.4} r={1.15} fill={color} stroke="none" />
        </Svg>
      );

    case 'x':
      return (
        <Svg {...common} fill={color}>
          <Path d="M18.9 2.2h3.05l-6.66 7.62L23.1 21.8h-6.13l-4.8-6.28-5.5 6.28H3.62l7.13-8.15L2.5 2.2h6.28l4.34 5.74 5.78-5.74Zm-1.07 17.78h1.69L8.24 3.92H6.43l11.4 16.06Z" />
        </Svg>
      );

    case 'facebook':
      return (
        <Svg {...common} fill={color}>
          <Path d="M22 12.06C22 6.5 17.52 2 12 2S2 6.5 2 12.06c0 5.02 3.66 9.18 8.44 9.94v-7.03H7.9v-2.91h2.54V9.85c0-2.52 1.49-3.91 3.77-3.91 1.09 0 2.24.2 2.24.2v2.46h-1.26c-1.24 0-1.63.78-1.63 1.57v1.89h2.78l-.45 2.91h-2.33V22c4.78-.76 8.44-4.92 8.44-9.94Z" />
        </Svg>
      );

    case 'tiktok':
      return (
        <Svg {...common} fill={color}>
          <Path d="M16.6 2h-3.1v13.4a2.72 2.72 0 1 1-2.1-2.65V9.6a5.9 5.9 0 1 0 5.2 5.85V9.28a7.1 7.1 0 0 0 4.15 1.33V7.5A4.1 4.1 0 0 1 16.6 2Z" />
        </Svg>
      );

    case 'youtube':
      return (
        <Svg {...common} fill={color}>
          <Path d="M21.6 7.2a2.5 2.5 0 0 0-1.76-1.77C18.25 5 12 5 12 5s-6.25 0-7.84.43A2.5 2.5 0 0 0 2.4 7.2 26 26 0 0 0 2 12a26 26 0 0 0 .4 4.8 2.5 2.5 0 0 0 1.76 1.77C5.75 19 12 19 12 19s6.25 0 7.84-.43a2.5 2.5 0 0 0 1.76-1.77A26 26 0 0 0 22 12a26 26 0 0 0-.4-4.8ZM10 15.06V8.94L15.2 12 10 15.06Z" />
        </Svg>
      );

    case 'linkedin':
      return (
        <Svg {...common} fill={color}>
          <Path d="M20.45 20.45h-3.55v-5.57c0-1.33-.02-3.04-1.85-3.04-1.85 0-2.14 1.45-2.14 2.94v5.67H9.35V9h3.41v1.56h.05a3.74 3.74 0 0 1 3.36-1.85c3.6 0 4.27 2.37 4.27 5.45v6.29ZM5.34 7.43a2.06 2.06 0 1 1 0-4.12 2.06 2.06 0 0 1 0 4.12Zm1.78 13.02H3.55V9h3.57v11.45ZM22.22 0H1.77C.79 0 0 .77 0 1.72v20.56C0 23.23.79 24 1.77 24h20.45c.98 0 1.78-.77 1.78-1.72V1.72C24 .77 23.2 0 22.22 0Z" />
        </Svg>
      );

    case 'whatsapp':
      return (
        <Svg {...common} fill={color}>
          <Path d="M17.47 14.38c-.3-.15-1.75-.86-2.02-.96-.27-.1-.47-.15-.67.15-.2.3-.77.96-.94 1.16-.17.2-.35.22-.64.07a8.1 8.1 0 0 1-2.39-1.47 8.96 8.96 0 0 1-1.65-2.06c-.17-.3-.02-.46.13-.6.14-.14.3-.35.45-.53.15-.18.2-.3.3-.5.1-.2.05-.38-.02-.53-.08-.15-.67-1.6-.92-2.2-.24-.58-.49-.5-.67-.51h-.57c-.2 0-.52.07-.79.37-.27.3-1.04 1.01-1.04 2.47s1.06 2.87 1.21 3.07c.15.2 2.1 3.2 5.08 4.49.71.3 1.26.49 1.7.63.71.22 1.36.19 1.87.12.57-.09 1.75-.72 2-1.41.25-.7.25-1.28.17-1.41-.07-.13-.27-.2-.57-.35ZM12.05 21.8h-.01a9.8 9.8 0 0 1-4.99-1.37l-.36-.21-3.71.97.99-3.62-.23-.37a9.78 9.78 0 0 1-1.5-5.22c0-5.4 4.4-9.8 9.81-9.8a9.75 9.75 0 0 1 9.8 9.81c0 5.4-4.4 9.8-9.8 9.8ZM20.52 3.49A11.68 11.68 0 0 0 12.05 0C5.6 0 .35 5.25.35 11.7c0 2.06.54 4.07 1.56 5.85L.25 24l6.6-1.73a11.68 11.68 0 0 0 5.2 1.24h.01c6.45 0 11.7-5.25 11.7-11.7 0-3.13-1.22-6.06-3.43-8.27Z" />
        </Svg>
      );

    case 'spotify':
      return (
        <Svg {...common} fill={color}>
          <Path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20Zm4.59 14.42a.62.62 0 0 1-.86.21c-2.35-1.44-5.31-1.76-8.8-.96a.62.62 0 1 1-.28-1.22c3.82-.87 7.1-.5 9.73 1.11.3.18.39.57.21.86Zm1.22-2.72a.78.78 0 0 1-1.07.26c-2.69-1.66-6.79-2.14-9.97-1.17a.78.78 0 1 1-.45-1.49c3.63-1.1 8.15-.57 11.24 1.33.36.23.48.7.25 1.07Zm.11-2.84C14.7 8.94 9.4 8.77 6.32 9.7a.94.94 0 1 1-.54-1.8c3.54-1.07 9.39-.87 13.1 1.33a.94.94 0 0 1-.96 1.62Z" />
        </Svg>
      );

    case 'website':
    default:
      return (
        <Svg {...common} fill="none" stroke={color} strokeWidth={1.9}>
          <Circle cx={12} cy={12} r={9.2} />
          <Path d="M2.8 12h18.4M12 2.8c2.6 3.1 2.6 15.3 0 18.4M12 2.8c-2.6 3.1-2.6 15.3 0 18.4" />
        </Svg>
      );
  }
}