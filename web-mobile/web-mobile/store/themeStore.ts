import { create } from 'zustand';

export type ThemeMode = 'auto' | 'light' | 'dark';

interface ThemeState {
  mode: ThemeMode;
  setMode: (mode: ThemeMode) => void;
  loadMode: () => void;
}

export const useThemeStore = create<ThemeState>((set) => ({
  mode: 'auto',

  setMode: (mode) => {
    set({ mode });
    if (typeof window !== 'undefined') {
      localStorage.setItem('theme_mode', mode);
    }
  },

  loadMode: () => {
    if (typeof window === 'undefined') return;
    const saved = localStorage.getItem('theme_mode');
    if (saved === 'light' || saved === 'dark' || saved === 'auto') {
      set({ mode: saved as ThemeMode });
    }
  },
}));