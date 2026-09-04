// ─────────────────────────────────────────────────────────────────────────────
// Studio appearance store.
//
// Three modes, matching every other app on the phone:
//   auto  — follow the device (the default)
//   light — force light
//   dark  — force dark
//
// Persisted in SecureStore next to the auth tokens, because that is the only
// key-value store this app already ships (no AsyncStorage dependency). The
// value is a three-character string, not a secret; SecureStore is simply the
// storage we have.
//
// loadMode() runs once from app/_layout.tsx before the first paint. Until it
// resolves the store reports 'auto', which is also the fallback, so a slow read
// never flashes the wrong theme at someone whose choice IS auto — and for the
// other two it settles within the splash.
// ─────────────────────────────────────────────────────────────────────────────
import { create } from 'zustand';
import * as SecureStore from 'expo-secure-store';

export type ThemeMode = 'auto' | 'light' | 'dark';

const KEY = 'studio_theme_mode';

/** Order the toggle button cycles through. */
export const THEME_CYCLE: ThemeMode[] = ['auto', 'light', 'dark'];

function isMode(v: unknown): v is ThemeMode {
  return v === 'auto' || v === 'light' || v === 'dark';
}

interface ThemeState {
  mode: ThemeMode;
  /** True once loadMode() has finished, whether or not anything was stored. */
  hydrated: boolean;
  setMode: (mode: ThemeMode) => Promise<void>;
  cycleMode: () => Promise<void>;
  loadMode: () => Promise<void>;
}

export const useThemeStore = create<ThemeState>((set, get) => ({
  mode: 'auto',
  hydrated: false,

  setMode: async (mode) => {
    // Set first, persist second: the UI must not wait on disk to repaint.
    set({ mode });
    try {
      await SecureStore.setItemAsync(KEY, mode);
    } catch {
      /* best-effort — a failed write only costs the choice on next launch */
    }
  },

  cycleMode: async () => {
    const i = THEME_CYCLE.indexOf(get().mode);
    await get().setMode(THEME_CYCLE[(i + 1) % THEME_CYCLE.length]);
  },

  loadMode: async () => {
    try {
      const saved = await SecureStore.getItemAsync(KEY);
      if (isMode(saved)) set({ mode: saved });
    } catch {
      /* fall through to 'auto' */
    } finally {
      set({ hydrated: true });
    }
  },
}));