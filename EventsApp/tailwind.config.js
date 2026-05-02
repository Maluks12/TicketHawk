/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './app/**/*.{js,jsx,ts,tsx}',
    './components/**/*.{js,jsx,ts,tsx}',
    './screens/**/*.{js,jsx,ts,tsx}',
    './styles/**/*.{js,jsx,ts,tsx}',
  ],
  presets: [require('nativewind/preset')],
  theme: {
    extend: {
      colors: {
        // ── Core brand ──────────────────────────────────────────────────────
        'brand-pink':   '#FF2D55',
        'brand-gold':   '#c8962a',
        // ── Dark surface palette (booking, tickets, profile) ────────────────
        'dark-base':    '#0a0a0a',
        'dark-card':    'rgba(255,255,255,0.05)',
        'dark-border':  'rgba(255,255,255,0.08)',
        'dark-muted':   'rgba(255,255,255,0.3)',
        'dark-sub':     'rgba(255,255,255,0.45)',
        // ── Status ──────────────────────────────────────────────────────────
        'status-confirmed': '#00e5a0',
        'status-pending':   '#ffb830',
        'status-cancelled': '#ff4d4d',
        // ── Legacy aliases kept for existing screens ─────────────────────────
        primary: '#4F46E5',
        accent:  '#B46EF5',
        surface: '#0A0A0A',
        card:    '#1A1A1A',
        border:  '#2A2A2A',
        navy:    '#002047',
        gold:    '#f5c842',
      },
    },
  },
  plugins: [],
};