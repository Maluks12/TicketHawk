import type { Metadata, Viewport } from 'next';
import './globals.css';
import { ThemeProvider, THEME_SCRIPT } from '../lib/theme';
import ServiceBanner from '../components/serviceBanner';

// Text unchanged on purpose — the brief was colour scheme only.
export const metadata: Metadata = {
  title: 'Admin — TicketHawk',
  description: 'TicketHawk admin dashboard',
  // There was no favicon, so the tab fell back to a generic globe. Next serves
  // app/icon.png and app/apple-icon.png automatically; declaring them gives the
  // shortcut and apple slots a definite answer instead of a guess.
  icons: {
    icon: [{ url: '/icon.png', type: 'image/png' }],
    shortcut: ['/icon.png'],
    apple: ['/apple-icon.png'],
  },
  // An internal console must never be indexed. Unlike the organiser app there
  // is no public landing page here, so this is safe to set at the root and
  // applies to every route.
  robots: { index: false, follow: false, nocache: true },
};

export const viewport: Viewport = {
  themeColor: [
    { media: '(prefers-color-scheme: dark)', color: '#08080C' },
    { media: '(prefers-color-scheme: light)', color: '#F4F5F9' },
  ],
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    // THEME_SCRIPT stamps data-theme before first paint, so the server HTML and
    // the pre-hydration DOM differ by that one attribute on purpose;
    // suppressHydrationWarning scopes the mismatch to <html>.
    <html lang="en-ZA" data-theme="dark" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_SCRIPT }} />
      </head>
      <body>
        <ThemeProvider>
          {/* Maintenance / outage notice, on every route — dashboard, login
              and landing page alike. Renders nothing when healthy. It
              publishes its height as --service-banner-h, which the
              one-viewport-tall dashboard shell subtracts from 100dvh. */}
          <ServiceBanner />
          {children}
        </ThemeProvider>
      </body>
    </html>
  );
}