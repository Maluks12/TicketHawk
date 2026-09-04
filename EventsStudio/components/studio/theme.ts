// ─────────────────────────────────────────────────────────────────────────────
// Organiser Studio — design tokens
//
// The Studio keeps its own identity (indigo accent, not the consumer pink
// #FF2D55) but no longer forces a single surface. Both palettes carry the SAME
// keys, so a screen only ever reads `S.<token>` and never asks which mode it is
// in. Anything that has to differ beyond colour gets the `isDark` flag.
//
// ── Why every screen changed ────────────────────────────────────────────────
// StyleSheet.create() at module scope runs ONCE, at import, so a stylesheet
// built from a fixed palette can never repaint. Screens therefore build their
// styles through makeStyles() below, which is a hook: it returns the sheet for
// the palette in force and caches one sheet per scheme, so switching theme is
// a re-render, not a rebuild.
// ─────────────────────────────────────────────────────────────────────────────
import { StyleSheet, useColorScheme } from 'react-native';
import type { ImageStyle, TextStyle, ViewStyle } from 'react-native';
import { useThemeStore, type ThemeMode } from '../../store/themeStore';

export type { ThemeMode };

export interface StudioPalette {
  scheme: 'dark' | 'light';

  // Surfaces
  bg: string;            // page background
  bgElevated: string;    // raised panel (login card, hero, dropdowns)
  surface: string;       // cards, chips, icon buttons
  surfaceStrong: string; // a card resting on another card
  field: string;         // text input background
  border: string;
  borderStrong: string;
  overlay: string;       // sticky footer / bottom nav backdrop
  onCamera: string;      // opaque card floating over the camera feed
  scrim: string;         // dim behind that card
  track: string;         // progress bar groove

  // Text
  text: string;
  sub: string;
  muted: string;
  faint: string;

  // Brand / accent
  accent: string;
  accentDeep: string;    // filled button background
  accentSoft: string;
  accentBorder: string;
  onAccent: string;      // text/icons ON accentDeep

  // Neutral filled button (the secondary action next to a primary one)
  btnSecondaryBg: string;

  // Status / semantic
  green: string; greenSoft: string;
  amber: string; amberSoft: string;
  red: string;   redSoft: string;
  blue: string;  blueSoft: string;
}

export const STUDIO_DARK: StudioPalette = {
  scheme: 'dark',

  bg: '#08080C',
  bgElevated: '#0E0E15',
  surface: 'rgba(255,255,255,0.045)',
  surfaceStrong: 'rgba(255,255,255,0.07)',
  field: 'rgba(255,255,255,0.05)',
  border: 'rgba(255,255,255,0.08)',
  borderStrong: 'rgba(255,255,255,0.14)',
  overlay: 'rgba(8,8,12,0.96)',
  onCamera: '#14141C',
  scrim: 'rgba(0,0,0,0.55)',
  track: 'rgba(255,255,255,0.08)',

  text: '#FFFFFF',
  sub: 'rgba(255,255,255,0.55)',
  muted: 'rgba(255,255,255,0.34)',
  faint: 'rgba(255,255,255,0.20)',

  accent: '#7C8CFF',
  accentDeep: '#4F46E5',
  accentSoft: 'rgba(124,140,255,0.14)',
  accentBorder: 'rgba(124,140,255,0.30)',
  onAccent: '#FFFFFF',

  btnSecondaryBg: 'rgba(255,255,255,0.09)',

  green: '#00E5A0', greenSoft: 'rgba(0,229,160,0.13)',
  amber: '#FFB830', amberSoft: 'rgba(255,184,48,0.13)',
  red: '#FF5A5A',   redSoft: 'rgba(255,90,90,0.13)',
  blue: '#4FA3FF',  blueSoft: 'rgba(79,163,255,0.13)',
};

// The status colours are NOT the dark ones reused. #00E5A0 and #FFB830 are
// mint and yellow on a white card — legible as decoration, not as text. These
// are the same hues pulled down to something that actually reads at 11px.
export const STUDIO_LIGHT: StudioPalette = {
  scheme: 'light',

  bg: '#EFF1F5',
  bgElevated: '#FFFFFF',
  surface: '#FFFFFF',
  surfaceStrong: '#F3F4F8',
  field: '#F3F4F8',
  border: 'rgba(9,10,20,0.10)',
  borderStrong: 'rgba(9,10,20,0.20)',
  overlay: 'rgba(239,241,245,0.97)',
  onCamera: '#FFFFFF',
  scrim: 'rgba(0,0,0,0.55)',
  track: 'rgba(9,10,20,0.10)',

  text: '#0B0C14',
  sub: 'rgba(11,12,20,0.62)',
  muted: 'rgba(11,12,20,0.46)',
  faint: 'rgba(11,12,20,0.30)',

  accent: '#4F46E5',
  accentDeep: '#4F46E5',
  accentSoft: 'rgba(79,70,229,0.10)',
  accentBorder: 'rgba(79,70,229,0.28)',
  onAccent: '#FFFFFF',

  btnSecondaryBg: 'rgba(9,10,20,0.06)',

  green: '#00875A', greenSoft: 'rgba(0,135,90,0.12)',
  amber: '#A65B00', amberSoft: 'rgba(166,91,0,0.12)',
  red: '#D42F2F',   redSoft: 'rgba(212,47,47,0.10)',
  blue: '#1B69C4',  blueSoft: 'rgba(27,105,196,0.11)',
};

// ── The hook every screen starts with ────────────────────────────────────────
export interface StudioTheme {
  S: StudioPalette;
  isDark: boolean;
  /** The stored preference — 'auto' means "whatever the device says". */
  mode: ThemeMode;
}

export function useStudioTheme(): StudioTheme {
  const mode = useThemeStore((s) => s.mode);
  const system = useColorScheme();
  // A null system scheme (some Android launchers during cold start) falls back
  // to dark, which is the Studio's house style and the one it shipped with.
  const isDark = mode === 'dark' || (mode === 'auto' && system !== 'light');
  return { S: isDark ? STUDIO_DARK : STUDIO_LIGHT, isDark, mode };
}

// ── Themed stylesheets ───────────────────────────────────────────────────────
type NamedStyles<T> = { [P in keyof T]: ViewStyle | TextStyle | ImageStyle };

/**
 * makeStyles(factory) → a hook returning the stylesheet for the active palette.
 *
 *   const useStyles = makeStyles((S) => ({ root: { backgroundColor: S.bg } }));
 *   function Screen() { const styles = useStyles(); … }
 *
 * The sheet is built at most twice per screen (once per scheme) and cached, so
 * this costs the same as the module-scope StyleSheet.create it replaces.
 */
export function makeStyles<T extends NamedStyles<T>>(
  factory: (S: StudioPalette, isDark: boolean) => T,
) {
  const cache = new Map<string, T>();
  return function useStudioStyles(): T {
    const { S, isDark } = useStudioTheme();
    const key = S.scheme;
    let sheet = cache.get(key);
    if (!sheet) {
      sheet = StyleSheet.create(factory(S, isDark) as any) as T;
      cache.set(key, sheet);
    }
    return sheet;
  };
}

// ── Accent pairs ─────────────────────────────────────────────────────────────
export type AccentKey = 'accent' | 'green' | 'amber' | 'blue' | 'red';

export function accentPair(S: StudioPalette, key: AccentKey): { fg: string; soft: string } {
  switch (key) {
    case 'green': return { fg: S.green, soft: S.greenSoft };
    case 'amber': return { fg: S.amber, soft: S.amberSoft };
    case 'blue':  return { fg: S.blue,  soft: S.blueSoft };
    case 'red':   return { fg: S.red,   soft: S.redSoft };
    default:      return { fg: S.accent, soft: S.accentSoft };
  }
}

// Map a backend status string to a chip colour set.
export function statusChip(
  status: string,
  S: StudioPalette,
): { fg: string; bg: string; label: string } {
  const s = (status ?? '').toLowerCase();
  const label = s.charAt(0).toUpperCase() + s.slice(1).replace(/_/g, ' ');
  switch (s) {
    case 'published':
    case 'active':
    case 'confirmed':
      return { fg: S.green, bg: S.greenSoft, label };
    case 'pending':
    case 'pending_review':
    case 'paused':
      return { fg: S.amber, bg: S.amberSoft, label };
    case 'cancelled':
    case 'rejected':
    case 'ended':
    case 'payment_failed':
    case 'exhausted':
      return { fg: S.red, bg: S.redSoft, label };
    case 'completed':
      return { fg: S.blue, bg: S.blueSoft, label };
    default:
      return { fg: S.sub, bg: S.surfaceStrong, label };
  }
}