import { create } from 'zustand';
import * as SecureStore from 'expo-secure-store';

export type ThemeMode = 'auto' | 'light' | 'dark';

interface ThemeState {
  mode: ThemeMode;
  setMode: (mode: ThemeMode) => Promise<void>;
  loadMode: () => Promise<void>;
}

export const useThemeStore = create<ThemeState>((set) => ({
  mode: 'auto',
  setMode: async (mode) => {
    set({ mode });
    await SecureStore.setItemAsync('theme_mode', mode);
  },
  loadMode: async () => {
    const saved = await SecureStore.getItemAsync('theme_mode');
    if (saved === 'light' || saved === 'dark' || saved === 'auto') {
      set({ mode: saved as ThemeMode });
    }
  },
}));