// ─────────────────────────────────────────────────────────────────────────────
// Organiser Studio — design tokens
//
// The Studio runs on its own fixed palette rather than the consumer app's
// light/dark theme. This is deliberate: stepping into the Studio should feel
// like opening a separate tool (think YouTube → YouTube Studio), so it keeps a
// consistent deep-charcoal analytics surface and an indigo accent that
// contrasts the consumer pink (#FF2D55).
// ─────────────────────────────────────────────────────────────────────────────

export const STUDIO = {
  // Surfaces
  bg: '#08080C',
  bgElevated: '#0E0E15',
  surface: 'rgba(255,255,255,0.045)',
  surfaceStrong: 'rgba(255,255,255,0.07)',
  border: 'rgba(255,255,255,0.08)',
  borderStrong: 'rgba(255,255,255,0.14)',

  // Text
  text: '#FFFFFF',
  sub: 'rgba(255,255,255,0.55)',
  muted: 'rgba(255,255,255,0.34)',
  faint: 'rgba(255,255,255,0.20)',

  // Brand / accent (Studio identity)
  accent: '#7C8CFF',
  accentDeep: '#4F46E5',
  accentSoft: 'rgba(124,140,255,0.14)',
  accentBorder: 'rgba(124,140,255,0.30)',

  // Status / semantic
  green: '#00E5A0',
  greenSoft: 'rgba(0,229,160,0.13)',
  amber: '#FFB830',
  amberSoft: 'rgba(255,184,48,0.13)',
  red: '#FF5A5A',
  redSoft: 'rgba(255,90,90,0.13)',
  blue: '#4FA3FF',
  blueSoft: 'rgba(79,163,255,0.13)',
} as const;

export type AccentKey = 'accent' | 'green' | 'amber' | 'blue' | 'red';

export const ACCENTS: Record<AccentKey, { fg: string; soft: string }> = {
  accent: { fg: STUDIO.accent, soft: STUDIO.accentSoft },
  green: { fg: STUDIO.green, soft: STUDIO.greenSoft },
  amber: { fg: STUDIO.amber, soft: STUDIO.amberSoft },
  blue: { fg: STUDIO.blue, soft: STUDIO.blueSoft },
  red: { fg: STUDIO.red, soft: STUDIO.redSoft },
};

// Map a backend status string to a chip colour set.
export function statusChip(status: string): { fg: string; bg: string; label: string } {
  const s = status.toLowerCase();
  const label = s.charAt(0).toUpperCase() + s.slice(1).replace(/_/g, ' ');
  switch (s) {
    case 'published':
    case 'active':
    case 'confirmed':
      return { fg: STUDIO.green, bg: STUDIO.greenSoft, label };
    case 'pending':
    case 'pending_review':
    case 'paused':
      return { fg: STUDIO.amber, bg: STUDIO.amberSoft, label };
    case 'cancelled':
    case 'rejected':
    case 'ended':
    case 'payment_failed':
    case 'exhausted':
      return { fg: STUDIO.red, bg: STUDIO.redSoft, label };
    case 'completed':
      return { fg: STUDIO.blue, bg: STUDIO.blueSoft, label };
    default:
      return { fg: STUDIO.sub, bg: STUDIO.surface, label };
  }
}