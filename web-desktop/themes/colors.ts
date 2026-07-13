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
  const mode     = useThemeStore((s) => s.mode);
  const loadMode = useThemeStore((s) => s.loadMode);

  const [systemDark, setSystemDark] = useState(false);

  // Empty deps: loadMode only needs to run once on mount to read localStorage.
  // [loadMode] caused an infinite loop: loadMode calls set() which triggers
  // a re-render, which re-runs the effect, which calls set() again forever.
  useEffect(() => {
    loadMode();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

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