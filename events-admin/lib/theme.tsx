'use client';
// ─────────────────────────────────────────────────────────────────────────────
// Theme (light / dark)
//
// The mobile Studio is dark-only. On the web dashboard organisers work in
// daylight offices, so both themes exist and the choice is remembered per
// device.
//
// Resolution order: saved choice → OS preference → dark. "system" stays live:
// if the organiser never picks explicitly, flipping the OS to light flips the
// dashboard with it.
//
// No-flash: THEME_SCRIPT runs before first paint in <head> and stamps
// data-theme on <html>. Without it the page renders dark for a frame before
// React hydrates and a light-mode user gets a black flash on every navigation.
// ─────────────────────────────────────────────────────────────────────────────
import {
  createContext, useCallback, useContext, useEffect, useState, ReactNode,
} from 'react';

export type ThemeChoice = 'light' | 'dark' | 'system';
export type ResolvedTheme = 'light' | 'dark';

const STORAGE_KEY = 'evt_admin_theme';

// Injected verbatim into <head>. Kept dependency-free and wrapped in try/catch:
// Safari private mode throws on localStorage access, and a throw here would
// block the rest of the document.
export const THEME_SCRIPT = `(function(){try{
var s=localStorage.getItem('${STORAGE_KEY}');
var m=window.matchMedia('(prefers-color-scheme: light)').matches?'light':'dark';
var t=(s==='light'||s==='dark')?s:m;
document.documentElement.setAttribute('data-theme',t);
document.documentElement.style.colorScheme=t;
}catch(e){document.documentElement.setAttribute('data-theme','dark');}})();`;

interface ThemeCtx {
  theme: ResolvedTheme;   // what is actually painted
  choice: ThemeChoice;    // what the organiser selected
  setChoice: (c: ThemeChoice) => void;
  toggle: () => void;
}

const Ctx = createContext<ThemeCtx>({
  theme: 'dark',
  choice: 'system',
  setChoice: () => {},
  toggle: () => {},
});

export const useTheme = () => useContext(Ctx);

function systemTheme(): ResolvedTheme {
  if (typeof window === 'undefined') return 'dark';
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
}

function apply(t: ResolvedTheme) {
  const el = document.documentElement;
  el.setAttribute('data-theme', t);
  el.style.colorScheme = t;
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  // Start at 'system' on both server and client so the first client render
  // matches the server HTML; the effect below reconciles with what
  // THEME_SCRIPT already painted.
  const [choice, setChoiceState] = useState<ThemeChoice>('system');
  const [theme, setTheme] = useState<ResolvedTheme>('dark');

  useEffect(() => {
    let saved: ThemeChoice = 'system';
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (raw === 'light' || raw === 'dark') saved = raw;
    } catch { /* storage blocked — fall back to system */ }
    setChoiceState(saved);
    const resolved = saved === 'system' ? systemTheme() : saved;
    setTheme(resolved);
    apply(resolved);
  }, []);

  // Follow the OS only while the organiser hasn't pinned a theme.
  useEffect(() => {
    if (choice !== 'system' || typeof window === 'undefined') return;
    const mq = window.matchMedia('(prefers-color-scheme: light)');
    const onChange = () => {
      const t: ResolvedTheme = mq.matches ? 'light' : 'dark';
      setTheme(t);
      apply(t);
    };
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, [choice]);

  const setChoice = useCallback((c: ThemeChoice) => {
    setChoiceState(c);
    const resolved = c === 'system' ? systemTheme() : c;
    setTheme(resolved);
    apply(resolved);
    try {
      if (c === 'system') localStorage.removeItem(STORAGE_KEY);
      else localStorage.setItem(STORAGE_KEY, c);
    } catch { /* non-fatal: theme just won't persist */ }
  }, []);

  const toggle = useCallback(() => {
    setChoice(theme === 'dark' ? 'light' : 'dark');
  }, [theme, setChoice]);

  return (
    <Ctx.Provider value={{ theme, choice, setChoice, toggle }}>
      {children}
    </Ctx.Provider>
  );
}

// ── Toggle control ───────────────────────────────────────────────────────────
export function ThemeToggle({ compact = false }: { compact?: boolean }) {
  const { theme, setChoice } = useTheme();

  if (compact) {
    return (
      <button
        type="button"
        className="btn-icon"
        aria-label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
        title={theme === 'dark' ? 'Light mode' : 'Dark mode'}
        onClick={() => setChoice(theme === 'dark' ? 'light' : 'dark')}
      >
        {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
      </button>
    );
  }

  return (
    <div className="theme-toggle" role="group" aria-label="Colour theme">
      <button
        type="button"
        className={theme === 'light' ? 'on' : ''}
        aria-pressed={theme === 'light'}
        aria-label="Light mode"
        title="Light"
        onClick={() => setChoice('light')}
      >
        <SunIcon />
      </button>
      <button
        type="button"
        className={theme === 'dark' ? 'on' : ''}
        aria-pressed={theme === 'dark'}
        aria-label="Dark mode"
        title="Dark"
        onClick={() => setChoice('dark')}
      >
        <MoonIcon />
      </button>
    </div>
  );
}

function SunIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M21 12.79A9 9 0 1111.21 3a7 7 0 009.79 9.79z" />
    </svg>
  );
}