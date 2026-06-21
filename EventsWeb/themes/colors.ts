'use client';

import { useEffect, useState } from 'react';
import { useThemeStore } from '../store/themeStore';

export const lightColors = {
  background: '#ffffff', text: '#0a0a0a', subText: 'rgba(0,0,0,0.4)',
  headerBg: '#ffffff', cardBg: '#ffffff', sectionCardBg: 'rgba(0,0,0,0.05)',
  dividerColor: 'rgba(0,0,0,0.07)', iconColor: 'rgba(0,0,0,0.55)',
  ctaBg: '#ff2d55', ctaText: '#ffffff', secondaryBg: '#1c2e61', secondaryText: '#ffffff',
};

export const darkColors: typeof lightColors = {
  background: '#000000', text: '#ffffff', subText: 'rgba(255,255,255,0.4)',
  headerBg: '#000000', cardBg: '#1a1a1a', sectionCardBg: 'rgba(255,255,255,0.05)',
  dividerColor: 'rgba(255,255,255,0.07)', iconColor: 'rgba(255,255,255,0.55)',
  ctaBg: '#ff2d55', ctaText: '#ffffff', secondaryBg: '#1c2e61', secondaryText: '#ffffff',
};

export function useAppTheme() {
  const { mode, loadMode } = useThemeStore();

  const [systemDark, setSystemDark] = useState(false);

  // FIX: loadMode() existed in the store but was never called anywhere,
  // so a saved 'light' or 'dark' choice was silently dropped on every
  // refresh/navigation and the app fell back to 'auto' (system) instead.
  useEffect(() => {
    loadMode();
  }, [loadMode]);

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    setSystemDark(mq.matches);
    const handler = (e: MediaQueryListEvent) => setSystemDark(e.matches);
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  }, []);

  const isDark =
    mode === 'dark' ||
    (mode === 'auto' && systemDark);

  return { isDark, colors: isDark ? darkColors : lightColors, mode };
}